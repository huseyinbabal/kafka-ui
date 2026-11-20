// Package cluster manages one Kafka client per configured cluster and tracks health.
package cluster

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
	"github.com/huseyinbabal/kafka-ui/internal/config"
)

type Status string

const (
	StatusUnknown Status = "unknown"
	StatusOnline  Status = "online"
	StatusOffline Status = "offline"
)

// Cluster is a connected Kafka cluster.
type Cluster struct {
	Config config.ClusterConfig
	Client *kgo.Client
	Admin  *kadm.Client

	opts []kgo.Opt

	mu        sync.RWMutex
	status    Status
	lastErr   string
	lastCheck time.Time
}

// NewClient creates a short-lived client with the cluster's connection settings,
// e.g. a direct-partition consumer for a message scan. The caller must Close it.
func (c *Cluster) NewClient(extra ...kgo.Opt) (*kgo.Client, error) {
	opts := append(append([]kgo.Opt(nil), c.opts...), extra...)
	return kgo.NewClient(opts...)
}

// Health is a snapshot of a cluster's connectivity.
type Health struct {
	Status    Status    `json:"status"`
	LastError string    `json:"lastError,omitempty"`
	LastCheck time.Time `json:"lastCheck"`
}

func (c *Cluster) Health() Health {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Health{Status: c.status, LastError: c.lastErr, LastCheck: c.lastCheck}
}

func (c *Cluster) setHealth(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastCheck = time.Now()
	if err != nil {
		c.status, c.lastErr = StatusOffline, err.Error()
		return
	}
	c.status, c.lastErr = StatusOnline, ""
}

// Registry holds all clusters keyed by ID, preserving config order.
type Registry struct {
	order    []string
	clusters map[string]*Cluster
	log      *slog.Logger
}

func NewRegistry(cfgs []config.ClusterConfig, log *slog.Logger) (*Registry, error) {
	r := &Registry{clusters: map[string]*Cluster{}, log: log}
	for _, cfg := range cfgs {
		opts, err := clientOpts(cfg)
		if err != nil {
			r.Close()
			return nil, fmt.Errorf("cluster %s: %w", cfg.ID, err)
		}
		// The shared client produces with explicit partitions; see kafka.Produce.
		cl, err := kgo.NewClient(append(opts, kgo.RecordPartitioner(kgo.ManualPartitioner()))...)
		if err != nil {
			r.Close()
			return nil, fmt.Errorf("cluster %s: %w", cfg.ID, err)
		}
		r.order = append(r.order, cfg.ID)
		r.clusters[cfg.ID] = &Cluster{Config: cfg, Client: cl, Admin: kadm.NewClient(cl), opts: opts, status: StatusUnknown}
	}
	return r, nil
}

func clientOpts(cfg config.ClusterConfig) ([]kgo.Opt, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Bootstrap...),
		kgo.ClientID("kafka-ui"),
		kgo.DialTimeout(5 * time.Second),
		kgo.RequestTimeoutOverhead(5 * time.Second),
		kgo.MetadataMinAge(2 * time.Second),
	}
	if cfg.TLS != nil && cfg.TLS.Enabled {
		tc, err := tlsConfig(cfg.TLS)
		if err != nil {
			return nil, err
		}
		opts = append(opts, kgo.DialTLSConfig(tc))
	}
	if cfg.SASL != nil {
		pw, err := cfg.SASL.Password.Value()
		if err != nil {
			return nil, fmt.Errorf("sasl password: %w", err)
		}
		switch cfg.SASL.Mechanism {
		case "PLAIN":
			opts = append(opts, kgo.SASL(plain.Auth{User: cfg.SASL.Username, Pass: pw}.AsMechanism()))
		case "SCRAM-SHA-256":
			opts = append(opts, kgo.SASL(scram.Auth{User: cfg.SASL.Username, Pass: pw}.AsSha256Mechanism()))
		case "SCRAM-SHA-512":
			opts = append(opts, kgo.SASL(scram.Auth{User: cfg.SASL.Username, Pass: pw}.AsSha512Mechanism()))
		}
	}
	return opts, nil
}

func tlsConfig(c *config.TLSConfig) (*tls.Config, error) {
	tc := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: c.InsecureSkipVerify} //nolint:gosec // opt-in by config
	if c.CAFile != "" {
		pem, err := os.ReadFile(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read ca: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certs in %s", c.CAFile)
		}
		tc.RootCAs = pool
	}
	if c.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client cert: %w", err)
		}
		tc.Certificates = []tls.Certificate{cert}
	}
	return tc, nil
}

// Get returns a cluster for reads.
func (r *Registry) Get(id string) (*Cluster, error) {
	c, ok := r.clusters[id]
	if !ok {
		return nil, apierr.ErrClusterNF
	}
	return c, nil
}

// Writable returns a cluster for mutations, rejecting read-only clusters.
func (r *Registry) Writable(id string) (*Cluster, error) {
	c, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	if c.Config.ReadOnly {
		return nil, apierr.ErrReadOnly
	}
	return c, nil
}

// All returns clusters in config order.
func (r *Registry) All() []*Cluster {
	out := make([]*Cluster, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.clusters[id])
	}
	return out
}

// RunHealthChecks probes every cluster periodically until ctx is done.
func (r *Registry) RunHealthChecks(ctx context.Context, every time.Duration) {
	check := func() {
		var wg sync.WaitGroup
		for _, c := range r.All() {
			wg.Go(func() {
				cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
				defer cancel()
				_, err := c.Admin.BrokerMetadata(cctx)
				prev := c.Health().Status
				c.setHealth(err)
				if now := c.Health().Status; now != prev {
					r.log.Info("cluster status changed", "cluster", c.Config.ID, "status", now, "err", err)
				}
			})
		}
		wg.Wait()
	}
	check()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			check()
		}
	}
}

func (r *Registry) Close() {
	for _, c := range r.clusters {
		c.Client.Close()
	}
}
