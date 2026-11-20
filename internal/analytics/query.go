package analytics

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Ranges the UI offers (SPEC F9).
var Ranges = map[string]time.Duration{
	"15m": 15 * time.Minute,
	"1h":  time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
}

// targetPoints is roughly how many points a chart gets per range.
const targetPoints = 240

// Window is a resolved time range and bucket size.
type Window struct {
	From, To time.Time
	Step     time.Duration
}

// NewWindow resolves a range name; minStep is the collector interval.
func NewWindow(name string, now time.Time, minStep time.Duration) (Window, error) {
	d, ok := Ranges[name]
	if !ok {
		return Window{}, fmt.Errorf("range must be one of 15m, 1h, 6h, 24h, 7d")
	}
	step := max(d/targetPoints, minStep)
	if d > 6*time.Hour {
		step = max(step, time.Minute) // older data only exists as minute rollups
	}
	step = step.Round(time.Second)
	return Window{From: now.Add(-d), To: now, Step: step}, nil
}

type bucket struct {
	ts   int64 // bucket start, used as the x value
	last int64 // time of the latest sample in the bucket, used for rates
	a, b int64 // meaning depends on the query
}

// series fetches (key, bucket, max(colA), max(colB)) over raw ∪ 1m tables.
// Raw and rollup never overlap: Compact moves rows, it doesn't copy them.
func (s *Store) series(ctx context.Context, table, keyExpr, colA, colB, where string, w Window, args ...any) (map[string][]bucket, error) {
	step := w.Step.Milliseconds()
	q := fmt.Sprintf(`SELECT k, (ts/%[1]d)*%[1]d AS b, MAX(ts), MAX(a), MAX(bb) FROM (
		SELECT %[2]s AS k, ts, %[3]s AS a, %[4]s AS bb FROM %[5]s_1m WHERE cluster = ? AND ts >= ? %[6]s
		UNION ALL
		SELECT %[2]s AS k, ts, %[3]s AS a, %[4]s AS bb FROM %[5]s_raw WHERE cluster = ? AND ts >= ? %[6]s
	) GROUP BY k, b ORDER BY k, b`, step, keyExpr, colA, colB, table, where)
	from := w.From.UnixMilli()
	full := append(append([]any{args[0], from}, args[1:]...), append([]any{args[0], from}, args[1:]...)...)
	rows, err := s.db.QueryContext(ctx, q, full...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string][]bucket{}
	for rows.Next() {
		var k string
		var bk bucket
		if err := rows.Scan(&k, &bk.ts, &bk.last, &bk.a, &bk.b); err != nil {
			return nil, err
		}
		out[k] = append(out[k], bk)
	}
	return out, rows.Err()
}

// rates turns counter samples into per-second rates at each bucket (from the
// previous bucket), timed by the actual samples so a partly filled last bucket
// isn't under-reported. A counter going backwards (topic recreated) counts as 0.
func rates(bs []bucket, val func(bucket) int64) map[int64]float64 {
	out := make(map[int64]float64, len(bs))
	for i := 1; i < len(bs); i++ {
		dt := float64(bs[i].last-bs[i-1].last) / 1000
		if dt <= 0 {
			continue
		}
		d := val(bs[i]) - val(bs[i-1])
		if d < 0 || val(bs[i-1]) < 0 {
			d = 0
		}
		out[bs[i].ts] = float64(d) / dt
	}
	return out
}

type ThroughputPoint struct {
	TS       int64   `json:"ts"`
	Messages float64 `json:"messages"` // msg/s
}

// BytesPoint is estimated from log size growth, so it uses buckets of at
// least bytesMinStep (sizes are sampled less often than offsets).
type BytesPoint struct {
	TS    int64   `json:"ts"`
	Bytes float64 `json:"bytes"` // bytes/s
}

const bytesMinStep = time.Minute

type LagPoint struct {
	TS  int64 `json:"ts"`
	Lag int64 `json:"lag"`
}

type TopicStat struct {
	Topic     string  `json:"topic"`
	MsgRate   float64 `json:"msgRate"`  // average over the window
	Messages  int64   `json:"messages"` // produced during the window
	SizeBytes int64   `json:"sizeBytes"`
	Growth    int64   `json:"growthBytes"` // size change during the window
}

type GroupStat struct {
	Group       string  `json:"group"`
	Lag         int64   `json:"lag"`
	ConsumeRate float64 `json:"consumeRate"` // msg/s over the trailing 5 minutes
	IngressRate float64 `json:"ingressRate"` // msg/s into the group's topics, same window
	// CatchUpSeconds is lag ÷ (consume − ingress); nil when the group is not catching up.
	CatchUpSeconds *float64 `json:"catchUpSeconds"`
}

type Overview struct {
	Step           int64             `json:"step"` // ms
	From           int64             `json:"from"`
	To             int64             `json:"to"`
	CollectedSince int64             `json:"collectedSince"` // first sample, 0 = none yet
	Throughput     []ThroughputPoint `json:"throughput"`
	Bytes          []BytesPoint      `json:"bytes"`
	BytesStep      int64             `json:"bytesStep"` // ms
	Lag            []LagPoint        `json:"lag"`
	TopTopics      []TopicStat       `json:"topTopics"`
	Groups         []GroupStat       `json:"groups"`
}

const trailing = 5 * time.Minute

func internalTopic(t string) bool {
	return strings.HasPrefix(t, "__") || strings.HasPrefix(t, "_schemas")
}

// CollectedSince returns the time of the oldest stored sample (unix ms), or 0.
func (s *Store) CollectedSince(ctx context.Context, cluster string) (int64, error) {
	var ts int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MIN(ts), 0) FROM (
		SELECT MIN(ts) AS ts FROM cluster_samples_1m WHERE cluster = ?
		UNION ALL SELECT MIN(ts) FROM cluster_samples_raw WHERE cluster = ?)`, cluster, cluster).Scan(&ts)
	return ts, err
}

// Overview builds the cluster dashboard for a window.
func (s *Store) Overview(ctx context.Context, cluster string, w Window) (*Overview, error) {
	o := &Overview{Step: w.Step.Milliseconds(), From: w.From.UnixMilli(), To: w.To.UnixMilli(),
		Throughput: []ThroughputPoint{}, Bytes: []BytesPoint{}, Lag: []LagPoint{}, TopTopics: []TopicStat{}, Groups: []GroupStat{}}
	var err error
	if o.CollectedSince, err = s.CollectedSince(ctx, cluster); err != nil {
		return nil, err
	}

	topics, err := s.series(ctx, "topic_samples", "topic", "end_sum", "size_bytes", "", w, cluster)
	if err != nil {
		return nil, err
	}
	msgs := map[int64]float64{}
	topicIngress := map[string][]bucket{} // for group catch-up
	for topic, bs := range topics {
		if internalTopic(topic) {
			continue
		}
		topicIngress[topic] = bs
		for ts, r := range rates(bs, func(b bucket) int64 { return b.a }) {
			msgs[ts] += r
		}
		if len(bs) > 0 {
			first, last := bs[0], bs[len(bs)-1]
			st := TopicStat{Topic: topic, Messages: max(last.a-first.a, 0), SizeBytes: last.b}
			if dt := float64(last.last-first.last) / 1000; dt > 0 {
				st.MsgRate = float64(st.Messages) / dt
			}
			if first.b >= 0 && last.b >= 0 {
				st.Growth = last.b - first.b
			}
			o.TopTopics = append(o.TopTopics, st)
		}
	}
	for _, ts := range sortedTS(msgs) {
		o.Throughput = append(o.Throughput, ThroughputPoint{TS: ts, Messages: round2(msgs[ts])})
	}
	bw := w
	bw.Step = max(w.Step, bytesMinStep)
	o.BytesStep = bw.Step.Milliseconds()
	sizes, err := s.series(ctx, "topic_samples", "topic", "size_bytes", "0", "", bw, cluster)
	if err != nil {
		return nil, err
	}
	bytes := map[int64]float64{}
	for topic, bs := range sizes {
		if internalTopic(topic) {
			continue
		}
		for ts, r := range rates(bs, func(b bucket) int64 { return b.a }) {
			bytes[ts] += r
		}
	}
	for _, ts := range sortedTS(bytes) {
		o.Bytes = append(o.Bytes, BytesPoint{TS: ts, Bytes: round2(bytes[ts])})
	}
	sort.Slice(o.TopTopics, func(i, j int) bool {
		if o.TopTopics[i].MsgRate != o.TopTopics[j].MsgRate {
			return o.TopTopics[i].MsgRate > o.TopTopics[j].MsgRate
		}
		return o.TopTopics[i].SizeBytes > o.TopTopics[j].SizeBytes
	})

	groups, err := s.series(ctx, "group_samples", "grp || char(0) || topic", "committed_sum", "lag", "", w, cluster)
	if err != nil {
		return nil, err
	}
	lagAt := map[int64]int64{}
	type acc struct {
		lag               int64
		lastTS            int64
		consumed, ingress float64
		windowSecs        float64
	}
	perGroup := map[string]*acc{}
	for key, bs := range groups {
		grp, topic, _ := strings.Cut(key, "\x00")
		for _, b := range bs {
			lagAt[b.ts] += b.b
		}
		a := perGroup[grp]
		if a == nil {
			a = &acc{}
			perGroup[grp] = a
		}
		if len(bs) == 0 {
			continue
		}
		last := bs[len(bs)-1]
		// Latest lag per topic, summed; topics whose last sample is older belong to the past.
		if last.ts > a.lastTS {
			a.lastTS = last.ts
		}
		a.lag += last.b
		c, secs := trailingDelta(bs, func(b bucket) int64 { return b.a })
		a.consumed += c
		a.windowSecs = max(a.windowSecs, secs)
		if ti, ok := topicIngress[topic]; ok {
			in, _ := trailingDelta(ti, func(b bucket) int64 { return b.a })
			a.ingress += in
		}
	}
	for _, ts := range sortedTS(lagAt) {
		o.Lag = append(o.Lag, LagPoint{TS: ts, Lag: lagAt[ts]})
	}
	var newest int64
	for _, a := range perGroup {
		newest = max(newest, a.lastTS)
	}
	for grp, a := range perGroup {
		// A group not sampled recently was deleted (or lost all commits): leave it out.
		if a.lastTS < newest-2*o.Step {
			continue
		}
		g := GroupStat{Group: grp, Lag: a.lag}
		if a.windowSecs > 0 {
			g.ConsumeRate = round2(a.consumed / a.windowSecs)
			g.IngressRate = round2(a.ingress / a.windowSecs)
		}
		if net := g.ConsumeRate - g.IngressRate; a.lag > 0 && net > 0 {
			secs := math.Round(float64(a.lag) / net)
			g.CatchUpSeconds = &secs
		}
		o.Groups = append(o.Groups, g)
	}
	sort.Slice(o.Groups, func(i, j int) bool {
		if o.Groups[i].Lag != o.Groups[j].Lag {
			return o.Groups[i].Lag > o.Groups[j].Lag
		}
		return o.Groups[i].Group < o.Groups[j].Group
	})
	return o, nil
}

// trailingDelta returns the counter increase over the last `trailing` of bs and its span in seconds.
func trailingDelta(bs []bucket, val func(bucket) int64) (float64, float64) {
	if len(bs) < 2 {
		return 0, 0
	}
	last := bs[len(bs)-1]
	i := len(bs) - 2
	for i > 0 && last.last-bs[i].last < trailing.Milliseconds() {
		i--
	}
	first := bs[i]
	d := val(last) - val(first)
	if d < 0 {
		d = 0
	}
	return float64(d), float64(last.last-first.last) / 1000
}

type TopicPoint struct {
	TS        int64   `json:"ts"`
	MsgRate   float64 `json:"msgRate"`
	BytesRate float64 `json:"bytesRate"`
	SizeBytes int64   `json:"sizeBytes"`
	Messages  int64   `json:"messages"` // retained = end − start
}

// TopicSeries returns one topic's throughput and size over a window.
func (s *Store) TopicSeries(ctx context.Context, cluster, topic string, w Window) ([]TopicPoint, error) {
	ends, err := s.series(ctx, "topic_samples", "topic", "end_sum", "size_bytes", "AND topic = ?", w, cluster, topic)
	if err != nil {
		return nil, err
	}
	starts, err := s.series(ctx, "topic_samples", "topic", "start_sum", "0", "AND topic = ?", w, cluster, topic)
	if err != nil {
		return nil, err
	}
	bs := ends[topic]
	startAt := map[int64]int64{}
	for _, b := range starts[topic] {
		startAt[b.ts] = b.a
	}
	mr := rates(bs, func(b bucket) int64 { return b.a })
	br := rates(bs, func(b bucket) int64 { return b.b })
	out := make([]TopicPoint, 0, len(bs))
	for _, b := range bs {
		out = append(out, TopicPoint{TS: b.ts, MsgRate: round2(mr[b.ts]), BytesRate: round2(br[b.ts]), SizeBytes: b.b,
			Messages: b.a - startAt[b.ts]})
	}
	return out, nil
}

type GroupPoint struct {
	TS          int64            `json:"ts"`
	Lag         int64            `json:"lag"`
	ConsumeRate float64          `json:"consumeRate"`
	ByTopic     map[string]int64 `json:"byTopic"` // lag per topic
}

// GroupSeries returns one group's lag (total and per topic) and consume rate over a window.
func (s *Store) GroupSeries(ctx context.Context, cluster, group string, w Window) ([]GroupPoint, error) {
	byTopic, err := s.series(ctx, "group_samples", "topic", "committed_sum", "lag", "AND grp = ?", w, cluster, group)
	if err != nil {
		return nil, err
	}
	points := map[int64]*GroupPoint{}
	for topic, bs := range byTopic {
		r := rates(bs, func(b bucket) int64 { return b.a })
		for _, b := range bs {
			p := points[b.ts]
			if p == nil {
				p = &GroupPoint{TS: b.ts, ByTopic: map[string]int64{}}
				points[b.ts] = p
			}
			p.Lag += b.b
			p.ByTopic[topic] = b.b
			p.ConsumeRate = round2(p.ConsumeRate + r[b.ts])
		}
	}
	out := make([]GroupPoint, 0, len(points))
	for _, p := range points {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TS < out[j].TS })
	return out, nil
}

// IdleTopics returns topics whose end offsets did not move within w, among topics
// sampled at least twice (so a just-created topic isn't reported).
func (s *Store) IdleTopics(ctx context.Context, cluster string, w Window) ([]string, error) {
	topics, err := s.series(ctx, "topic_samples", "topic", "end_sum", "0", "", w, cluster)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for topic, bs := range topics {
		if len(bs) >= 2 && !internalTopic(topic) && bs[len(bs)-1].a == bs[0].a {
			out = append(out, topic)
		}
	}
	sort.Strings(out)
	return out, nil
}

func sortedTS[V any](m map[int64]V) []int64 {
	out := make([]int64, 0, len(m))
	for ts := range m {
		out = append(out, ts)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }
