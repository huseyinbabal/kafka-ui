package api

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/huseyinbabal/kafka-ui/internal/analytics"
	"github.com/huseyinbabal/kafka-ui/internal/apierr"
	"github.com/huseyinbabal/kafka-ui/internal/auth"
	"github.com/huseyinbabal/kafka-ui/internal/cluster"
	"github.com/huseyinbabal/kafka-ui/internal/config"
	"github.com/huseyinbabal/kafka-ui/internal/kafka"
	"github.com/huseyinbabal/kafka-ui/internal/live"
)

// kafkaTimeout bounds every Kafka admin call made on behalf of a request.
const kafkaTimeout = 15 * time.Second

type Server struct {
	cfg       *config.Config
	auth      *auth.Service
	clusters  *cluster.Registry
	loginRate *auth.RateLimiter
	log       *slog.Logger
	static    fs.FS
	store     *analytics.Store
	collector *analytics.Collector
	live      *live.Hub
}

func NewServer(cfg *config.Config, a *auth.Service, reg *cluster.Registry, static fs.FS, log *slog.Logger) *Server {
	return &Server{cfg: cfg, auth: a, clusters: reg, loginRate: auth.NewRateLimiter(5, time.Minute), log: log, static: static}
}

// Handler builds the full HTTP handler tree.
func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /api/v1/auth/me", s.me)
	api.HandleFunc("POST /api/v1/auth/logout", s.logout)

	api.HandleFunc("GET /api/v1/clusters", s.listClusters)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/overview", s.overview)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/brokers/{id}/configs", s.brokerConfigs)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/topics", s.listTopics)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/topics/{topic}", s.getTopic)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/topics/{topic}/consumers", s.topicConsumers)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/topics/{topic}/messages/stream", s.streamMessages)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/topics/{topic}/messages/{partition}/{offset}", s.getMessage)
	api.HandleFunc("POST /api/v1/clusters/{cluster}/topics/{topic}/messages", admin(s.produceMessage))
	api.HandleFunc("POST /api/v1/clusters/{cluster}/topics", admin(s.createTopic))
	api.HandleFunc("DELETE /api/v1/clusters/{cluster}/topics/{topic}", admin(s.deleteTopic))
	api.HandleFunc("PATCH /api/v1/clusters/{cluster}/topics/{topic}/configs", admin(s.alterTopicConfigs))
	api.HandleFunc("PUT /api/v1/clusters/{cluster}/topics/{topic}/partitions", admin(s.setPartitions))
	api.HandleFunc("POST /api/v1/clusters/{cluster}/topics/{topic}/purge", admin(s.purgeTopic))
	api.HandleFunc("GET /api/v1/clusters/{cluster}/groups", s.listGroups)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/groups/{group}", s.getGroup)
	api.HandleFunc("POST /api/v1/clusters/{cluster}/groups/{group}/reset-offsets", admin(s.resetOffsets))
	api.HandleFunc("DELETE /api/v1/clusters/{cluster}/groups/{group}", admin(s.deleteGroup))
	api.HandleFunc("GET /api/v1/clusters/{cluster}/schemas/subjects", s.listSubjects)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/schemas/subjects/{subject}", s.getSubject)
	api.HandleFunc("POST /api/v1/clusters/{cluster}/schemas/subjects/{subject}/compatibility-check", s.checkCompatibility)
	api.HandleFunc("POST /api/v1/clusters/{cluster}/schemas/subjects/{subject}/versions", admin(s.registerSchema))
	api.HandleFunc("PUT /api/v1/clusters/{cluster}/schemas/subjects/{subject}/compatibility", admin(s.setCompatibility))
	api.HandleFunc("DELETE /api/v1/clusters/{cluster}/schemas/subjects/{subject}", admin(s.deleteSubject))
	api.HandleFunc("GET /api/v1/clusters/{cluster}/analytics/overview", s.analyticsOverview)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/analytics/topics/{topic}", s.analyticsTopic)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/analytics/groups/{group}", s.analyticsGroup)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/analytics/skew", s.analyticsSkew)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/analytics/hygiene", s.analyticsHygiene)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/live", s.liveStream)
	api.HandleFunc("GET /api/v1/clusters/{cluster}/acls", s.listACLs)
	api.HandleFunc("POST /api/v1/clusters/{cluster}/acls", admin(s.createACL))
	api.HandleFunc("POST /api/v1/clusters/{cluster}/acls/delete", admin(s.deleteACL))

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	root.HandleFunc("POST /api/v1/auth/login", s.login)
	root.Handle("/api/", s.requireAuth(api))
	root.Handle("/", spaHandler(s.static))

	return recoverer(s.log, logging(s.log, securityHeaders(root)))
}

// withCluster resolves the {cluster} path value and a request-scoped timeout.
func (s *Server) withCluster(w http.ResponseWriter, r *http.Request) (*cluster.Cluster, context.Context, context.CancelFunc, bool) {
	c, err := s.clusters.Get(r.PathValue("cluster"))
	if err != nil {
		writeError(w, r, err)
		return nil, nil, nil, false
	}
	// Fail fast instead of waiting for a Kafka timeout; the health loop flips it back.
	if h := c.Health(); h.Status == cluster.StatusOffline {
		writeError(w, r, &apierr.Error{Status: apierr.ErrOffline.Status, Code: apierr.ErrOffline.Code,
			Message: "cluster is unreachable: " + h.LastError})
		return nil, nil, nil, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), kafkaTimeout)
	return c, ctx, cancel, true
}

// ---- auth ----

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.loginRate.Allow(clientIP(r)) {
		writeError(w, r, apierr.New(http.StatusTooManyRequests, "RATE_LIMITED", "too many login attempts, try again in a minute"))
		return
	}
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	token, p, err := s.auth.Login(r.Context(), req.Username, req.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		writeError(w, r, apierr.New(http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid username or password"))
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: s.cfg.Server.CookiesSecure(), SameSite: http.SameSiteStrictMode,
		MaxAge: int(s.cfg.Auth.SessionIdleTimeout.Seconds()),
	})
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.auth.Logout(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: s.cfg.Server.CookiesSecure(), SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	writeJSON(w, http.StatusOK, p)
}

// ---- clusters ----

type clusterDTO struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	ReadOnly          bool   `json:"readOnly"`
	HasSchemaRegistry bool   `json:"hasSchemaRegistry"`
	cluster.Health
}

func (s *Server) listClusters(w http.ResponseWriter, _ *http.Request) {
	all := s.clusters.All()
	out := make([]clusterDTO, 0, len(all))
	for _, c := range all {
		out = append(out, clusterDTO{
			ID: c.Config.ID, Name: c.Config.Name, ReadOnly: c.Config.ReadOnly,
			HasSchemaRegistry: c.Config.SchemaRegistry != nil, Health: c.Health(),
		})
	}
	writeList(w, out)
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	c, ctx, cancel, ok := s.withCluster(w, r)
	if !ok {
		return
	}
	defer cancel()
	o, err := kafka.GetOverview(ctx, c.Client, c.Admin)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) brokerConfigs(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 32)
	if err != nil {
		writeError(w, r, apierr.BadRequest("invalid broker id", err))
		return
	}
	c, ctx, cancel, ok := s.withCluster(w, r)
	if !ok {
		return
	}
	defer cancel()
	cfgs, err := kafka.GetBrokerConfigs(ctx, c.Admin, int32(id))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeList(w, cfgs)
}

// ---- topics ----

func (s *Server) listTopics(w http.ResponseWriter, r *http.Request) {
	c, ctx, cancel, ok := s.withCluster(w, r)
	if !ok {
		return
	}
	defer cancel()
	ts, err := kafka.ListTopics(ctx, c.Admin)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeList(w, ts)
}

func (s *Server) getTopic(w http.ResponseWriter, r *http.Request) {
	c, ctx, cancel, ok := s.withCluster(w, r)
	if !ok {
		return
	}
	defer cancel()
	t, err := kafka.GetTopic(ctx, c.Admin, r.PathValue("topic"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) topicConsumers(w http.ResponseWriter, r *http.Request) {
	c, ctx, cancel, ok := s.withCluster(w, r)
	if !ok {
		return
	}
	defer cancel()
	gs, err := kafka.GroupsForTopic(ctx, c.Admin, r.PathValue("topic"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeList(w, gs)
}

// ---- groups ----

func (s *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	c, ctx, cancel, ok := s.withCluster(w, r)
	if !ok {
		return
	}
	defer cancel()
	gs, err := kafka.ListGroups(ctx, c.Admin)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeList(w, gs)
}

func (s *Server) getGroup(w http.ResponseWriter, r *http.Request) {
	c, ctx, cancel, ok := s.withCluster(w, r)
	if !ok {
		return
	}
	defer cancel()
	g, err := kafka.GetGroup(ctx, c.Admin, r.PathValue("group"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}
