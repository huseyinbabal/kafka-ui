package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// sseWriter writes Server-Sent Events. It is safe for concurrent use so a
// heartbeat can run next to the producer of events.
type sseWriter struct {
	mu sync.Mutex
	w  http.ResponseWriter
	rc *http.ResponseController
}

func newSSE(w http.ResponseWriter) *sseWriter {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	s := &sseWriter{w: w, rc: http.NewResponseController(w)}
	_ = s.rc.Flush()
	return s
}

func (s *sseWriter) send(event string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, b); err != nil {
		return err
	}
	return s.rc.Flush()
}

// heartbeat sends a comment line periodically so idle streams survive proxies.
// It stops when done is closed.
func (s *sseWriter) heartbeat(every time.Duration, done <-chan struct{}) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			s.mu.Lock()
			_, err := fmt.Fprint(s.w, ": ping\n\n")
			if err == nil {
				err = s.rc.Flush()
			}
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}
