package api

import (
	"net/http"
	"time"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
	"github.com/huseyinbabal/kafka-ui/internal/live"
)

// WithLive enables the live topology stream.
func (s *Server) WithLive(h *live.Hub) *Server {
	s.live = h
	return s
}

// liveStream pushes topology snapshots over SSE until the client leaves.
func (s *Server) liveStream(w http.ResponseWriter, r *http.Request) {
	if s.live == nil {
		writeError(w, r, apierr.New(http.StatusServiceUnavailable, "LIVE_DISABLED", "live view is not enabled"))
		return
	}
	c, _, cancel, ok := s.withCluster(w, r)
	if !ok {
		return
	}
	cancel()

	events, stop := s.live.Subscribe(c)
	defer stop()
	sse := newSSE(w)
	done := make(chan struct{})
	defer close(done)
	go sse.heartbeat(15*time.Second, done)
	_ = sse.send("hello", map[string]any{"interval": s.live.Interval().Milliseconds()})

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-events:
			var err error
			if ev.Err != nil {
				e := apierr.From(mapKafkaError(ev.Err))
				err = sse.send("error", map[string]string{"code": e.Code, "message": ev.Err.Error()})
			} else {
				err = sse.send("snapshot", ev.Snapshot)
			}
			if err != nil {
				return
			}
		}
	}
}
