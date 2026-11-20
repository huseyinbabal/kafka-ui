package kafka

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
	"github.com/huseyinbabal/kafka-ui/internal/serde"
)

// NewClientFunc creates a short-lived client with the cluster's connection settings.
type NewClientFunc func(opts ...kgo.Opt) (*kgo.Client, error)

type SeekMode string

const (
	SeekOldest    SeekMode = "oldest"
	SeekNewest    SeekMode = "newest"
	SeekOffset    SeekMode = "offset"
	SeekTimestamp SeekMode = "timestamp"
	SeekLive      SeekMode = "live"
)

// Scan limits (SPEC F5).
const (
	DefaultLimit   = 100
	MaxLimit       = 500
	DefaultMaxScan = 100_000
	// StreamMaxText caps each payload's text in a stream; the single-message
	// endpoint returns full content.
	StreamMaxText = 64 << 10

	boundedScanTimeout = 60 * time.Second
	liveScanTimeout    = 15 * time.Minute
	progressEvery      = 250 * time.Millisecond
	// quietPolls ends a bounded read when no records arrive for this many polls,
	// covering ranges whose last offsets are control records or compacted away.
	quietPolls  = 2
	pollTimeout = time.Second
)

// Filter matches records server-side. Field is any|key|value|header.
type Filter struct {
	Field string
	re    *regexp.Regexp
}

// NewFilter builds a filter. Substring matches are case-insensitive; regex is
// RE2 (linear time), so user input cannot cause catastrophic backtracking.
func NewFilter(field, query string, isRegex bool) (*Filter, error) {
	if query == "" {
		return nil, nil
	}
	switch field {
	case "", "any":
		field = "any"
	case "key", "value", "header":
	default:
		return nil, apierr.BadRequest("filter field must be any, key, value or header", nil)
	}
	expr := "(?i)" + regexp.QuoteMeta(query)
	if isRegex {
		expr = query
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, apierr.BadRequest("invalid regex: "+err.Error(), err)
	}
	return &Filter{Field: field, re: re}, nil
}

func (f *Filter) match(r *kgo.Record) bool {
	if f == nil {
		return true
	}
	all := f.Field == "any"
	if (all || f.Field == "key") && f.re.Match(r.Key) {
		return true
	}
	if (all || f.Field == "value") && f.re.Match(r.Value) {
		return true
	}
	if all || f.Field == "header" {
		for _, h := range r.Headers {
			if f.re.MatchString(h.Key) || f.re.Match(h.Value) {
				return true
			}
		}
	}
	return false
}

type BrowseRequest struct {
	Topic       string
	Partitions  []int32 // empty = all
	Mode        SeekMode
	Offset      int64 // SeekOffset: start offset in every selected partition
	Timestamp   int64 // SeekTimestamp: unix millis
	Limit       int
	MaxScan     int
	Filter      *Filter
	KeyFormat   serde.Format
	ValueFormat serde.Format
	// Decoder renders payloads; set Decoder.Registry to decode schema-registry formats.
	Decoder serde.Decoder
}

type Header struct {
	Key   string         `json:"key"`
	Value *serde.Payload `json:"value"`
}

type Message struct {
	Partition int32          `json:"partition"`
	Offset    int64          `json:"offset"`
	Timestamp int64          `json:"timestamp"`
	Key       *serde.Payload `json:"key"`
	Value     *serde.Payload `json:"value"`
	Headers   []Header       `json:"headers"`
	Size      int            `json:"size"`
}

type Progress struct {
	Scanned int `json:"scanned"`
	Matched int `json:"matched"`
}

// StopReason says why a scan ended.
type StopReason string

const (
	StopEnd       StopReason = "end"
	StopLimit     StopReason = "limit"
	StopScanLimit StopReason = "scanLimit"
	StopTimeout   StopReason = "timeout"
	StopCancelled StopReason = "cancelled"
)

type BrowseResult struct {
	Progress
	Reason StopReason `json:"reason"`
}

// Sink receives scan output. Message returning an error aborts the scan
// (e.g. the HTTP client went away).
type Sink interface {
	Message(Message) error
	Progress(Progress)
}

type span struct{ from, to int64 } // to < 0: unbounded (live)

// Browse scans a topic and streams matching messages to sink.
func Browse(ctx context.Context, newClient NewClientFunc, adm *kadm.Client, req BrowseRequest, sink Sink) (BrowseResult, error) {
	if req.Limit <= 0 {
		req.Limit = DefaultLimit
	}
	req.Limit = min(req.Limit, MaxLimit)
	if req.MaxScan <= 0 {
		req.MaxScan = DefaultMaxScan
	}
	timeout := boundedScanTimeout
	if req.Mode == SeekLive {
		timeout = liveScanTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req.Decoder.Ctx = ctx

	bounds, err := partitionBounds(ctx, adm, req.Topic, req.Partitions)
	if err != nil {
		return BrowseResult{}, err
	}

	s := &scan{req: req, sink: sink, lastProgress: time.Now()}
	switch req.Mode {
	case SeekNewest:
		err = s.newest(ctx, newClient, bounds)
	case SeekOldest, SeekOffset, SeekTimestamp, SeekLive:
		var spans map[int32]span
		spans, err = forwardSpans(ctx, adm, req, bounds)
		if err == nil {
			err = s.forward(ctx, newClient, spans)
		}
	default:
		return BrowseResult{}, apierr.BadRequest("mode must be oldest, newest, offset, timestamp or live", nil)
	}

	res := BrowseResult{Progress: s.progress(), Reason: s.reason}
	switch {
	case err == nil:
		if res.Reason == "" {
			res.Reason = StopEnd
		}
		return res, nil
	case errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil:
		res.Reason = StopTimeout
		return res, nil
	case errors.Is(err, context.Canceled):
		res.Reason = StopCancelled
		return res, nil
	default:
		return res, err
	}
}

type scan struct {
	req          BrowseRequest
	sink         Sink
	scanned      int
	matched      int
	reason       StopReason
	lastProgress time.Time
}

func (s *scan) progress() Progress { return Progress{Scanned: s.scanned, Matched: s.matched} }

func (s *scan) tick() {
	if time.Since(s.lastProgress) >= progressEvery {
		s.lastProgress = time.Now()
		s.sink.Progress(s.progress())
	}
}

// visit counts r and reports whether it matches. It sets s.reason when the scan limit is hit.
func (s *scan) visit(r *kgo.Record) bool {
	s.scanned++
	ok := s.req.Filter.match(r)
	if s.scanned >= s.req.MaxScan && s.reason == "" {
		s.reason = StopScanLimit
	}
	s.tick()
	return ok
}

func (s *scan) emit(r *kgo.Record) error {
	s.matched++
	if err := s.sink.Message(ToMessage(s.req.Decoder, r, s.req.KeyFormat, s.req.ValueFormat, StreamMaxText)); err != nil {
		return err
	}
	if s.matched >= s.req.Limit {
		s.reason = StopLimit
	}
	return nil
}

var errStop = errors.New("stop")

func (s *scan) forward(ctx context.Context, newClient NewClientFunc, spans map[int32]span) error {
	err := readSpans(ctx, newClient, s.req.Topic, spans, func(r *kgo.Record) error {
		if s.visit(r) {
			if err := s.emit(r); err != nil {
				return err
			}
		}
		if s.reason != "" {
			return errStop
		}
		return nil
	})
	if errors.Is(err, errStop) {
		return nil
	}
	return err
}

// newest reads backwards in growing windows per partition and emits each
// window's matches newest-first, until Limit matches or the start is reached.
func (s *scan) newest(ctx context.Context, newClient NewClientFunc, bounds map[int32]span) error {
	hi := map[int32]int64{}
	for p, b := range bounds {
		hi[p] = b.to
	}
	window := int64(max(s.req.Limit, 200))
	for {
		spans := map[int32]span{}
		for p, b := range bounds {
			lo := max(b.from, hi[p]-window)
			if lo < hi[p] {
				spans[p] = span{lo, hi[p]}
				hi[p] = lo
			}
		}
		if len(spans) == 0 {
			return nil
		}
		var batch []*kgo.Record
		err := readSpans(ctx, newClient, s.req.Topic, spans, func(r *kgo.Record) error {
			if s.visit(r) {
				batch = append(batch, r)
			}
			if s.reason != "" {
				return errStop
			}
			return nil
		})
		if err != nil && !errors.Is(err, errStop) {
			return err
		}
		sort.SliceStable(batch, func(i, j int) bool {
			if !batch[i].Timestamp.Equal(batch[j].Timestamp) {
				return batch[i].Timestamp.After(batch[j].Timestamp)
			}
			return batch[i].Offset > batch[j].Offset
		})
		for _, r := range batch {
			if err := s.emit(r); err != nil {
				return err
			}
			if s.reason == StopLimit {
				return nil
			}
		}
		if s.reason != "" {
			return nil
		}
		window = min(window*2, 20_000)
	}
}

// partitionBounds returns [start, end) offsets of the selected partitions.
func partitionBounds(ctx context.Context, adm *kadm.Client, topic string, want []int32) (map[int32]span, error) {
	starts, err := adm.ListStartOffsets(ctx, topic)
	if err != nil {
		return nil, err
	}
	ends, err := adm.ListEndOffsets(ctx, topic)
	if err != nil {
		return nil, err
	}
	if err := starts.Error(); err != nil {
		return nil, err
	}
	if err := ends.Error(); err != nil {
		return nil, err
	}
	all := map[int32]span{}
	starts.Each(func(o kadm.ListedOffset) {
		if e, ok := ends.Lookup(topic, o.Partition); ok {
			all[o.Partition] = span{o.Offset, e.Offset}
		}
	})
	if len(all) == 0 {
		return nil, apierr.NotFound("TOPIC_NOT_FOUND", "topic not found")
	}
	if len(want) == 0 {
		return all, nil
	}
	out := map[int32]span{}
	for _, p := range want {
		b, ok := all[p]
		if !ok {
			return nil, apierr.BadRequest(fmt.Sprintf("partition %d does not exist", p), nil)
		}
		out[p] = b
	}
	return out, nil
}

func forwardSpans(ctx context.Context, adm *kadm.Client, req BrowseRequest, bounds map[int32]span) (map[int32]span, error) {
	out := map[int32]span{}
	switch req.Mode {
	case SeekOldest:
		maps.Copy(out, bounds)
	case SeekOffset:
		if req.Offset < 0 {
			return nil, apierr.BadRequest("offset must be >= 0", nil)
		}
		for p, b := range bounds {
			out[p] = span{min(max(req.Offset, b.from), b.to), b.to}
		}
	case SeekTimestamp:
		listed, err := adm.ListOffsetsAfterMilli(ctx, req.Timestamp, req.Topic)
		if err != nil {
			return nil, err
		}
		for p, b := range bounds {
			from := b.to
			if o, ok := listed.Lookup(req.Topic, p); ok && o.Err == nil && o.Offset >= 0 {
				from = min(max(o.Offset, b.from), b.to)
			}
			out[p] = span{from, b.to}
		}
	case SeekLive:
		for p, b := range bounds {
			out[p] = span{b.to, -1}
		}
	}
	return out, nil
}

// readSpans consumes [from, to) of each partition, calling fn per record in
// partition order. It returns when all bounded spans are done, fn returns an
// error, or ctx ends. Unbounded spans (to < 0) only end with ctx or fn.
func readSpans(ctx context.Context, newClient NewClientFunc, topic string, spans map[int32]span, fn func(*kgo.Record) error) error {
	offsets := map[int32]kgo.Offset{}
	pending := map[int32]bool{}
	bounded := false
	for p, sp := range spans {
		if sp.to >= 0 && sp.from >= sp.to {
			continue
		}
		offsets[p] = kgo.NewOffset().At(sp.from)
		if sp.to >= 0 {
			pending[p], bounded = true, true
		}
	}
	if len(offsets) == 0 {
		return nil
	}
	cl, err := newClient(
		kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{topic: offsets}),
		kgo.FetchMaxWait(500*time.Millisecond),
		kgo.FetchMaxPartitionBytes(4<<20),
		kgo.ClientID("kafka-ui-browser"),
	)
	if err != nil {
		return err
	}
	defer cl.Close()

	quiet := 0
	for {
		if bounded && len(pending) == 0 {
			return nil
		}
		pctx, cancel := context.WithTimeout(ctx, pollTimeout)
		fetches := cl.PollFetches(pctx)
		cancel()
		if err := ctx.Err(); err != nil {
			return err
		}
		var fetchErr error
		fetches.EachError(func(_ string, _ int32, err error) {
			if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) && fetchErr == nil {
				fetchErr = err
			}
		})
		if fetchErr != nil {
			return fetchErr
		}

		got := false
		var stop error
		fetches.EachPartition(func(fp kgo.FetchTopicPartition) {
			if stop != nil {
				return
			}
			sp := spans[fp.Partition]
			for _, r := range fp.Records {
				if sp.to >= 0 && r.Offset >= sp.to {
					delete(pending, fp.Partition)
					break
				}
				got = true
				if err := fn(r); err != nil {
					stop = err
					return
				}
				if sp.to >= 0 && r.Offset >= sp.to-1 {
					delete(pending, fp.Partition)
				}
			}
		})
		if stop != nil {
			return stop
		}
		if got {
			quiet = 0
		} else if bounded {
			if quiet++; quiet >= quietPolls {
				return nil
			}
		}
	}
}

// ToMessage converts a record for the API. maxText 0 means no truncation.
func ToMessage(dec serde.Decoder, r *kgo.Record, keyFmt, valFmt serde.Format, maxText int) Message {
	m := Message{
		Partition: r.Partition, Offset: r.Offset, Timestamp: r.Timestamp.UnixMilli(),
		Key:     dec.Decode(r.Key, keyFmt, maxText),
		Value:   dec.Decode(r.Value, valFmt, maxText),
		Headers: make([]Header, 0, len(r.Headers)),
		Size:    len(r.Key) + len(r.Value),
	}
	for _, h := range r.Headers {
		m.Size += len(h.Key) + len(h.Value)
		m.Headers = append(m.Headers, Header{Key: h.Key, Value: serde.Decode(h.Value, serde.Auto, maxText)})
	}
	return m
}

// GetMessage reads one record with full (untruncated) content.
func GetMessage(ctx context.Context, newClient NewClientFunc, adm *kadm.Client, dec serde.Decoder, topic string, partition int32, offset int64, keyFmt, valFmt serde.Format) (*Message, error) {
	dec.Ctx = ctx
	bounds, err := partitionBounds(ctx, adm, topic, []int32{partition})
	if err != nil {
		return nil, err
	}
	b := bounds[partition]
	notFound := apierr.NotFound("MESSAGE_NOT_FOUND", "no message at this offset")
	if offset < b.from || offset >= b.to {
		return nil, notFound
	}
	var found *Message
	err = readSpans(ctx, newClient, topic, map[int32]span{partition: {offset, offset + 1}}, func(r *kgo.Record) error {
		if r.Offset == offset {
			m := ToMessage(dec, r, keyFmt, valFmt, 0)
			found = &m
		}
		return errStop
	})
	if err != nil && !errors.Is(err, errStop) {
		return nil, err
	}
	if found == nil {
		return nil, notFound
	}
	return found, nil
}

// ---- produce ----

type HeaderInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ProduceRequest struct {
	Partition   *int32        `json:"partition"`
	Key         *string       `json:"key"`
	Value       *string       `json:"value"`
	KeyFormat   string        `json:"keyFormat"`
	ValueFormat string        `json:"valueFormat"`
	Headers     []HeaderInput `json:"headers"`
	// KeySchema/ValueSchema are required when the matching format is "schema".
	KeySchema   *serde.SchemaRef `json:"keySchema"`
	ValueSchema *serde.SchemaRef `json:"valueSchema"`
}

type ProduceResult struct {
	Partition int32 `json:"partition"`
	Offset    int64 `json:"offset"`
	Timestamp int64 `json:"timestamp"`
}

// Produce writes one record. cl must use kgo.ManualPartitioner; when no
// partition is given it is chosen like the Java client (murmur2 of the key).
// reg may be nil when the cluster has no schema registry.
func Produce(ctx context.Context, cl *kgo.Client, adm *kadm.Client, reg *serde.Registry, topic string, req ProduceRequest) (*ProduceResult, error) {
	rec := &kgo.Record{Topic: topic}
	var err error
	if rec.Key, err = encodeOptional(ctx, reg, req.Key, req.KeyFormat, req.KeySchema, "key"); err != nil {
		return nil, err
	}
	if rec.Value, err = encodeOptional(ctx, reg, req.Value, req.ValueFormat, req.ValueSchema, "value"); err != nil {
		return nil, err
	}
	for _, h := range req.Headers {
		if h.Key == "" {
			return nil, apierr.BadRequest("header key must not be empty", nil)
		}
		rec.Headers = append(rec.Headers, kgo.RecordHeader{Key: h.Key, Value: []byte(h.Value)})
	}

	// Check existence first so producing never auto-creates a topic.
	md, err := adm.ListTopics(ctx, topic)
	if err != nil {
		return nil, err
	}
	td, ok := md[topic]
	if !ok || td.Err != nil {
		return nil, apierr.NotFound("TOPIC_NOT_FOUND", "topic not found")
	}
	n := len(td.Partitions)
	if req.Partition != nil {
		if *req.Partition < 0 || int(*req.Partition) >= n {
			return nil, apierr.BadRequest(fmt.Sprintf("partition must be between 0 and %d", n-1), nil)
		}
		rec.Partition = *req.Partition
	} else {
		rec.Partition = int32(kgo.StickyKeyPartitioner(nil).ForTopic(topic).Partition(rec, n))
	}

	res := cl.ProduceSync(ctx, rec)
	if err := res.FirstErr(); err != nil {
		return nil, err
	}
	r := res[0].Record
	return &ProduceResult{Partition: r.Partition, Offset: r.Offset, Timestamp: r.Timestamp.UnixMilli()}, nil
}

func encodeOptional(ctx context.Context, reg *serde.Registry, text *string, format string, ref *serde.SchemaRef, what string) ([]byte, error) {
	if text == nil {
		return nil, nil
	}
	f, err := serde.ParseFormat(format)
	if err != nil {
		return nil, apierr.BadRequest(what+": "+err.Error(), err)
	}
	var b []byte
	if f == serde.Schema {
		switch {
		case reg == nil:
			err = errors.New("no schema registry is configured for this cluster")
		case ref == nil:
			err = errors.New("a schema subject is required")
		default:
			b, err = reg.Encode(ctx, *ref, *text)
		}
	} else {
		b, err = serde.Encode(*text, f)
	}
	if err != nil {
		return nil, &apierr.Error{Status: http.StatusBadRequest, Code: "BAD_REQUEST", Message: what + ": " + err.Error(), Err: err}
	}
	if b == nil {
		b = []byte{} // empty, not null
	}
	return b, nil
}

// ParsePartitions parses "0,2,5" (empty = all).
func ParsePartitions(s []string) ([]int32, error) {
	var out []int32
	for _, v := range s {
		var p int32
		if _, err := fmt.Sscan(v, &p); err != nil || p < 0 {
			return nil, apierr.BadRequest("invalid partition "+v, err)
		}
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out, nil
}
