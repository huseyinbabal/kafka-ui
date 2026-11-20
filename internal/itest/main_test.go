//go:build integration

// Package itest runs end-to-end tests against real Kafka and Schema Registry
// containers (testcontainers-go). Run with: make test-integration
package itest

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/huseyinbabal/kafka-ui/internal/cluster"
	"github.com/huseyinbabal/kafka-ui/internal/config"
)

const (
	kafkaImage = "confluentinc/confluent-local:7.5.0"
	srImage    = "confluentinc/cp-schema-registry:7.5.0"
)

// env is shared by all tests; containers start once in TestMain.
var env struct {
	bootstrap    []string // plain cluster
	aclBootstrap []string // cluster with StandardAuthorizer
	srURL        string
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "integration setup:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	ctx := context.Background()
	nw, err := network.New(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = nw.Remove(ctx) }()

	k, err := tckafka.Run(ctx, kafkaImage, tckafka.WithClusterID("itest"), network.WithNetwork([]string{"kafka"}, nw))
	defer func() { _ = testcontainers.TerminateContainer(k) }()
	if err != nil {
		return 0, fmt.Errorf("kafka: %w", err)
	}
	if env.bootstrap, err = k.Brokers(ctx); err != nil {
		return 0, err
	}

	// The authorizer cluster talks PLAINTEXT, so every client is User:ANONYMOUS;
	// making it a super user lets the tests manage ACLs.
	acl, err := tckafka.Run(ctx, kafkaImage, tckafka.WithClusterID("itest-acl"), testcontainers.WithEnv(map[string]string{
		"KAFKA_AUTHORIZER_CLASS_NAME": "org.apache.kafka.metadata.authorizer.StandardAuthorizer",
		"KAFKA_SUPER_USERS":           "User:ANONYMOUS",
	}))
	defer func() { _ = testcontainers.TerminateContainer(acl) }()
	if err != nil {
		return 0, fmt.Errorf("acl kafka: %w", err)
	}
	if env.aclBootstrap, err = acl.Brokers(ctx); err != nil {
		return 0, err
	}

	// Schema Registry reaches Kafka over the shared network via its BROKER listener.
	inspect, err := k.Inspect(ctx)
	if err != nil {
		return 0, err
	}
	sr, err := testcontainers.Run(ctx, srImage,
		network.WithNetwork([]string{"schema-registry"}, nw),
		testcontainers.WithExposedPorts("8081/tcp"),
		testcontainers.WithEnv(map[string]string{
			"SCHEMA_REGISTRY_HOST_NAME":                    "schema-registry",
			"SCHEMA_REGISTRY_LISTENERS":                    "http://0.0.0.0:8081",
			"SCHEMA_REGISTRY_KAFKASTORE_BOOTSTRAP_SERVERS": "PLAINTEXT://" + inspect.Config.Hostname + ":9092",
		}),
		testcontainers.WithWaitStrategyAndDeadline(3*time.Minute, wait.ForHTTP("/subjects").WithPort("8081/tcp")),
	)
	defer func() { _ = testcontainers.TerminateContainer(sr) }()
	if err != nil {
		return 0, fmt.Errorf("schema registry: %w", err)
	}
	ep, err := sr.PortEndpoint(ctx, "8081/tcp", "http")
	if err != nil {
		return 0, err
	}
	env.srURL = ep
	return m.Run(), nil
}

// newCluster returns a connected cluster for the plain broker (with Schema Registry).
func newCluster(t *testing.T) *cluster.Cluster {
	t.Helper()
	return newClusterFor(t, env.bootstrap, true)
}

func newClusterFor(t *testing.T, bootstrap []string, withSR bool) *cluster.Cluster {
	t.Helper()
	cfg := config.ClusterConfig{ID: "it", Name: "it", Bootstrap: bootstrap}
	if withSR {
		cfg.SchemaRegistry = &config.SchemaRegistryConfig{URL: env.srURL}
	}
	reg, err := cluster.NewRegistry([]config.ClusterConfig{cfg}, quiet)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reg.Close)
	c, _ := reg.Get("it")
	return c
}

func ctxT(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// produce writes n records "k-i"/"v-i" round-robin over partitions.
func produce(t *testing.T, c *cluster.Cluster, topic string, partitions int32, n int) {
	t.Helper()
	recs := make([]*kgo.Record, 0, n)
	for i := range n {
		recs = append(recs, &kgo.Record{Topic: topic, Partition: int32(i) % partitions,
			Key: fmt.Appendf(nil, "k-%d", i), Value: fmt.Appendf(nil, "v-%d", i)})
	}
	if err := c.Client.ProduceSync(ctxT(t), recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}
}

// uniq gives each test its own topic/group names.
func uniq(prefix string) string { return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()) }
