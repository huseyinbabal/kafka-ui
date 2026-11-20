package api

import (
	"context"
	"net/http"

	"github.com/huseyinbabal/kafka-ui/internal/cluster"
	"github.com/huseyinbabal/kafka-ui/internal/kafka"
)

// withWritable is withCluster for mutations: it also rejects read-only clusters.
func (s *Server) withWritable(w http.ResponseWriter, r *http.Request) (*cluster.Cluster, context.Context, context.CancelFunc, bool) {
	if _, err := s.clusters.Writable(r.PathValue("cluster")); err != nil {
		writeError(w, r, err)
		return nil, nil, nil, false
	}
	return s.withCluster(w, r)
}

// audit logs a successful mutation with the acting user.
func (s *Server) audit(r *http.Request, action string, attrs ...any) {
	p, _ := principalFrom(r.Context())
	base := []any{"action", action, "user", p.Username, "cluster", r.PathValue("cluster")}
	s.log.InfoContext(r.Context(), "audit", append(base, attrs...)...)
}

// mutate runs a JSON mutation handler: decode body (if v != nil), resolve a writable cluster, call fn.
func mutate[T any](s *Server, w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, c *cluster.Cluster, body T) (any, error)) {
	var body T
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, r, err)
			return
		}
	}
	c, ctx, cancel, ok := s.withWritable(w, r)
	if !ok {
		return
	}
	defer cancel()
	out, err := fn(ctx, c, body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if out == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type empty struct{}

func (s *Server) createTopic(w http.ResponseWriter, r *http.Request) {
	mutate(s, w, r, func(ctx context.Context, c *cluster.Cluster, req kafka.CreateTopicRequest) (any, error) {
		t, err := kafka.CreateTopic(ctx, c.Admin, req)
		if err == nil && !req.ValidateOnly {
			s.audit(r, "topic.create", "topic", req.Name, "partitions", t.Partitions, "rf", t.ReplicationFactor)
		}
		return t, err
	})
}

func (s *Server) deleteTopic(w http.ResponseWriter, r *http.Request) {
	mutate(s, w, r, func(ctx context.Context, c *cluster.Cluster, _ empty) (any, error) {
		topic := r.PathValue("topic")
		err := kafka.DeleteTopic(ctx, c.Admin, topic)
		if err == nil {
			s.audit(r, "topic.delete", "topic", topic)
		}
		return nil, err
	})
}

type alterConfigsBody struct {
	Changes []kafka.ConfigChange `json:"changes"`
}

func (s *Server) alterTopicConfigs(w http.ResponseWriter, r *http.Request) {
	mutate(s, w, r, func(ctx context.Context, c *cluster.Cluster, b alterConfigsBody) (any, error) {
		topic := r.PathValue("topic")
		err := kafka.AlterTopicConfigs(ctx, c.Admin, topic, b.Changes)
		if err == nil {
			names := make([]string, 0, len(b.Changes))
			for _, ch := range b.Changes {
				names = append(names, ch.Name)
			}
			s.audit(r, "topic.configs", "topic", topic, "configs", names)
		}
		return nil, err
	})
}

type partitionsBody struct {
	Count int `json:"count"`
}

func (s *Server) setPartitions(w http.ResponseWriter, r *http.Request) {
	mutate(s, w, r, func(ctx context.Context, c *cluster.Cluster, b partitionsBody) (any, error) {
		topic := r.PathValue("topic")
		err := kafka.SetPartitionCount(ctx, c.Admin, topic, b.Count)
		if err == nil {
			s.audit(r, "topic.partitions", "topic", topic, "count", b.Count)
		}
		return nil, err
	})
}

type purgeBody struct {
	Partitions []int32 `json:"partitions"`
}

func (s *Server) purgeTopic(w http.ResponseWriter, r *http.Request) {
	mutate(s, w, r, func(ctx context.Context, c *cluster.Cluster, b purgeBody) (any, error) {
		topic := r.PathValue("topic")
		res, err := kafka.PurgeTopic(ctx, c.Admin, topic, b.Partitions)
		if err == nil {
			s.audit(r, "topic.purge", "topic", topic, "partitions", b.Partitions)
		}
		return listBody[kafka.PurgeResult]{Items: res}, err
	})
}

func (s *Server) resetOffsets(w http.ResponseWriter, r *http.Request) {
	mutate(s, w, r, func(ctx context.Context, c *cluster.Cluster, req kafka.ResetRequest) (any, error) {
		group := r.PathValue("group")
		res, err := kafka.ResetOffsets(ctx, c.Admin, group, req)
		if err == nil && res.Applied {
			s.audit(r, "group.reset", "group", group, "topic", req.Topic, "strategy", req.Strategy, "value", req.Value)
		}
		return res, err
	})
}

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	mutate(s, w, r, func(ctx context.Context, c *cluster.Cluster, _ empty) (any, error) {
		group := r.PathValue("group")
		err := kafka.DeleteGroup(ctx, c.Admin, group)
		if err == nil {
			s.audit(r, "group.delete", "group", group)
		}
		return nil, err
	})
}

func (s *Server) listACLs(w http.ResponseWriter, r *http.Request) {
	c, ctx, cancel, ok := s.withCluster(w, r)
	if !ok {
		return
	}
	defer cancel()
	acls, err := kafka.ListACLs(ctx, c.Admin)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeList(w, acls)
}

func (s *Server) createACL(w http.ResponseWriter, r *http.Request) {
	mutate(s, w, r, func(ctx context.Context, c *cluster.Cluster, a kafka.ACL) (any, error) {
		err := kafka.CreateACL(ctx, c.Admin, a)
		if err == nil {
			s.audit(r, "acl.create", "acl", a)
		}
		return nil, err
	})
}

func (s *Server) deleteACL(w http.ResponseWriter, r *http.Request) {
	mutate(s, w, r, func(ctx context.Context, c *cluster.Cluster, a kafka.ACL) (any, error) {
		n, err := kafka.DeleteACL(ctx, c.Admin, a)
		if err == nil {
			s.audit(r, "acl.delete", "acl", a, "deleted", n)
		}
		return map[string]int{"deleted": n}, err
	})
}
