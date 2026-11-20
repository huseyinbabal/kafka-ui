package api

import (
	"net/http"
	"time"

	"github.com/huseyinbabal/kafka-ui/internal/analytics"
	"github.com/huseyinbabal/kafka-ui/internal/apierr"
	"github.com/huseyinbabal/kafka-ui/internal/kafka"
)

// WithAnalytics enables the analytics endpoints.
func (s *Server) WithAnalytics(store *analytics.Store, col *analytics.Collector) *Server {
	s.store, s.collector = store, col
	return s
}

var errNoAnalytics = apierr.New(http.StatusServiceUnavailable, "ANALYTICS_DISABLED", "analytics are not enabled")

// analyticsWindow resolves ?range= (default def) after checking the cluster exists.
func (s *Server) analyticsWindow(w http.ResponseWriter, r *http.Request, def string) (analytics.Window, bool) {
	if s.store == nil {
		writeError(w, r, errNoAnalytics)
		return analytics.Window{}, false
	}
	if _, err := s.clusters.Get(r.PathValue("cluster")); err != nil {
		writeError(w, r, err)
		return analytics.Window{}, false
	}
	name := r.URL.Query().Get("range")
	if name == "" {
		name = def
	}
	win, err := analytics.NewWindow(name, time.Now(), s.collector.Interval())
	if err != nil {
		writeError(w, r, apierr.BadRequest(err.Error(), err))
		return analytics.Window{}, false
	}
	return win, true
}

func (s *Server) analyticsOverview(w http.ResponseWriter, r *http.Request) {
	win, ok := s.analyticsWindow(w, r, "1h")
	if !ok {
		return
	}
	o, err := s.store.Overview(r.Context(), r.PathValue("cluster"), win)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) analyticsTopic(w http.ResponseWriter, r *http.Request) {
	win, ok := s.analyticsWindow(w, r, "1h")
	if !ok {
		return
	}
	pts, err := s.store.TopicSeries(r.Context(), r.PathValue("cluster"), r.PathValue("topic"), win)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"step": win.Step.Milliseconds(), "points": pts})
}

func (s *Server) analyticsGroup(w http.ResponseWriter, r *http.Request) {
	win, ok := s.analyticsWindow(w, r, "1h")
	if !ok {
		return
	}
	pts, err := s.store.GroupSeries(r.Context(), r.PathValue("cluster"), r.PathValue("group"), win)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"step": win.Step.Milliseconds(), "points": pts})
}

func (s *Server) analyticsSkew(w http.ResponseWriter, r *http.Request) {
	if s.collector == nil {
		writeError(w, r, errNoAnalytics)
		return
	}
	if _, err := s.clusters.Get(r.PathValue("cluster")); err != nil {
		writeError(w, r, err)
		return
	}
	live := s.collector.Live(r.PathValue("cluster"))
	if live == nil {
		live = &analytics.Live{Partitions: []analytics.PartitionRate{}, Brokers: []analytics.BrokerLoad{}}
	}
	writeJSON(w, http.StatusOK, live)
}

type hygieneResponse struct {
	Issues     []kafka.HygieneIssue `json:"issues"`
	IdleTopics []string             `json:"idleTopics"`
	// IdleSince is where the idle check's evidence starts (unix ms): the range
	// start, or the first sample if collection began later.
	IdleSince int64 `json:"idleSince"`
}

func (s *Server) analyticsHygiene(w http.ResponseWriter, r *http.Request) {
	win, ok := s.analyticsWindow(w, r, "24h")
	if !ok {
		return
	}
	c, ctx, cancel, ok := s.withCluster(w, r)
	if !ok {
		return
	}
	defer cancel()
	issues, err := kafka.Hygiene(ctx, c.Admin)
	if err != nil {
		writeError(w, r, err)
		return
	}
	idle, err := s.store.IdleTopics(ctx, c.Config.ID, win)
	if err != nil {
		writeError(w, r, err)
		return
	}
	since, err := s.store.CollectedSince(ctx, c.Config.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, hygieneResponse{Issues: issues, IdleTopics: idle, IdleSince: max(since, win.From.UnixMilli())})
}
