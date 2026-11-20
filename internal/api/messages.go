package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
	"github.com/huseyinbabal/kafka-ui/internal/kafka"
	"github.com/huseyinbabal/kafka-ui/internal/serde"
)

type sseSink struct{ sse *sseWriter }

func (s sseSink) Message(m kafka.Message) error   { return s.sse.send("message", m) }
func (s sseSink) Progress(p kafka.Progress)       { _ = s.sse.send("progress", p) }
func (s sseSink) done(r kafka.BrowseResult) error { return s.sse.send("done", r) }

// parseBrowse reads the query of GET .../messages/stream.
func parseBrowse(r *http.Request) (kafka.BrowseRequest, error) {
	q := r.URL.Query()
	req := kafka.BrowseRequest{Topic: r.PathValue("topic"), Mode: kafka.SeekMode(q.Get("mode"))}
	if req.Mode == "" {
		req.Mode = kafka.SeekNewest
	}
	var err error
	intParam := func(name string) int64 {
		v := q.Get(name)
		if v == "" || err != nil {
			return 0
		}
		n, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil {
			err = apierr.BadRequest("invalid "+name, perr)
		}
		return n
	}
	req.Limit = int(intParam("limit"))
	req.Offset = intParam("offset")
	req.Timestamp = intParam("timestamp")
	if err != nil {
		return req, err
	}
	if req.Mode == kafka.SeekTimestamp && q.Get("timestamp") == "" {
		return req, apierr.BadRequest("timestamp is required for mode=timestamp", nil)
	}
	if v := q.Get("partitions"); v != "" {
		if req.Partitions, err = kafka.ParsePartitions(strings.Split(v, ",")); err != nil {
			return req, err
		}
	}
	if req.Filter, err = kafka.NewFilter(q.Get("qField"), q.Get("q"), q.Get("regex") == "true"); err != nil {
		return req, err
	}
	if req.KeyFormat, err = serde.ParseFormat(q.Get("keyFormat")); err != nil {
		return req, apierr.BadRequest(err.Error(), err)
	}
	if req.ValueFormat, err = serde.ParseFormat(q.Get("valueFormat")); err != nil {
		return req, apierr.BadRequest(err.Error(), err)
	}
	return req, nil
}

func (s *Server) streamMessages(w http.ResponseWriter, r *http.Request) {
	req, err := parseBrowse(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	c, _, cancel, ok := s.withCluster(w, r)
	if !ok {
		return
	}
	cancel() // the scan has its own time cap; it ends when the client disconnects

	sse := newSSE(w)
	done := make(chan struct{})
	defer close(done)
	go sse.heartbeat(15*time.Second, done)

	req.Decoder = c.Decoder()
	sink := sseSink{sse}
	res, err := kafka.Browse(r.Context(), c.NewClient, c.Admin, req, sink)
	if err != nil {
		e := apierr.From(mapKafkaError(err))
		if e.Status >= 500 {
			s.log.ErrorContext(r.Context(), "message scan failed", "cluster", c.Config.ID, "topic", req.Topic, "err", err)
		}
		msg := e.Message
		if e.Status >= 500 && e.Err != nil && e.Code != "INTERNAL" {
			msg = e.Error()
		}
		_ = sse.send("error", map[string]string{"code": e.Code, "message": msg})
		return
	}
	_ = sink.done(res)
}

func (s *Server) getMessage(w http.ResponseWriter, r *http.Request) {
	part, err := strconv.ParseInt(r.PathValue("partition"), 10, 32)
	if err != nil || part < 0 {
		writeError(w, r, apierr.BadRequest("invalid partition", err))
		return
	}
	off, err := strconv.ParseInt(r.PathValue("offset"), 10, 64)
	if err != nil || off < 0 {
		writeError(w, r, apierr.BadRequest("invalid offset", err))
		return
	}
	kf, err := serde.ParseFormat(r.URL.Query().Get("keyFormat"))
	if err != nil {
		writeError(w, r, apierr.BadRequest(err.Error(), err))
		return
	}
	vf, err := serde.ParseFormat(r.URL.Query().Get("valueFormat"))
	if err != nil {
		writeError(w, r, apierr.BadRequest(err.Error(), err))
		return
	}
	c, ctx, cancel, ok := s.withCluster(w, r)
	if !ok {
		return
	}
	defer cancel()
	m, err := kafka.GetMessage(ctx, c.NewClient, c.Admin, c.Decoder(), r.PathValue("topic"), int32(part), off, kf, vf)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) produceMessage(w http.ResponseWriter, r *http.Request) {
	c, err := s.clusters.Writable(r.PathValue("cluster"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req kafka.ProduceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if _, ctx, cancel, ok := s.withCluster(w, r); ok {
		defer cancel()
		res, err := kafka.Produce(ctx, c.Client, c.Admin, c.Serde, r.PathValue("topic"), req)
		if err != nil {
			writeError(w, r, err)
			return
		}
		p, _ := principalFrom(r.Context())
		s.log.InfoContext(context.WithoutCancel(ctx), "message produced", "user", p.Username, "cluster", c.Config.ID,
			"topic", r.PathValue("topic"), "partition", res.Partition, "offset", res.Offset)
		writeJSON(w, http.StatusCreated, res)
	}
}
