package api

import (
	"context"
	"net/http"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
	"github.com/huseyinbabal/kafka-ui/internal/cluster"
	"github.com/huseyinbabal/kafka-ui/internal/schemaregistry"
)

var errNoRegistry = apierr.NotFound("SCHEMA_REGISTRY_NOT_CONFIGURED", "no schema registry is configured for this cluster")

// withSchemas resolves the cluster's registry client. It is not gated on Kafka
// health: the registry is a separate service.
func (s *Server) withSchemas(w http.ResponseWriter, r *http.Request, write bool) (*cluster.Cluster, context.Context, context.CancelFunc, bool) {
	get := s.clusters.Get
	if write {
		get = s.clusters.Writable
	}
	c, err := get(r.PathValue("cluster"))
	if err == nil && c.Schemas == nil {
		err = errNoRegistry
	}
	if err != nil {
		writeError(w, r, err)
		return nil, nil, nil, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), kafkaTimeout)
	return c, ctx, cancel, true
}

func (s *Server) listSubjects(w http.ResponseWriter, r *http.Request) {
	c, ctx, cancel, ok := s.withSchemas(w, r, false)
	if !ok {
		return
	}
	defer cancel()
	subs, err := c.Schemas.ListSubjects(ctx)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeList(w, subs)
}

func (s *Server) getSubject(w http.ResponseWriter, r *http.Request) {
	c, ctx, cancel, ok := s.withSchemas(w, r, false)
	if !ok {
		return
	}
	defer cancel()
	d, err := c.Schemas.GetSubject(ctx, r.PathValue("subject"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) checkCompatibility(w http.ResponseWriter, r *http.Request) {
	var req schemaregistry.RegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	c, ctx, cancel, ok := s.withSchemas(w, r, false)
	if !ok {
		return
	}
	defer cancel()
	res, err := c.Schemas.CheckCompatibility(ctx, r.PathValue("subject"), req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) registerSchema(w http.ResponseWriter, r *http.Request) {
	var req schemaregistry.RegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	c, ctx, cancel, ok := s.withSchemas(w, r, true)
	if !ok {
		return
	}
	defer cancel()
	subject := r.PathValue("subject")
	v, err := c.Schemas.Register(ctx, subject, req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	s.audit(r, "schema.register", "subject", subject, "version", v.Version, "id", v.ID)
	writeJSON(w, http.StatusOK, v)
}

type compatibilityBody struct {
	Level string `json:"level"` // empty = inherit the global level
}

func (s *Server) setCompatibility(w http.ResponseWriter, r *http.Request) {
	var b compatibilityBody
	if err := decodeJSON(r, &b); err != nil {
		writeError(w, r, err)
		return
	}
	c, ctx, cancel, ok := s.withSchemas(w, r, true)
	if !ok {
		return
	}
	defer cancel()
	subject := r.PathValue("subject")
	if err := c.Schemas.SetCompatibility(ctx, subject, b.Level); err != nil {
		writeError(w, r, err)
		return
	}
	s.audit(r, "schema.compatibility", "subject", subject, "level", b.Level)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteSubject(w http.ResponseWriter, r *http.Request) {
	c, ctx, cancel, ok := s.withSchemas(w, r, true)
	if !ok {
		return
	}
	defer cancel()
	subject := r.PathValue("subject")
	permanent := r.URL.Query().Get("permanent") == "true"
	versions, err := c.Schemas.DeleteSubject(ctx, subject, permanent)
	if err != nil {
		writeError(w, r, err)
		return
	}
	s.audit(r, "schema.delete", "subject", subject, "permanent", permanent)
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
}
