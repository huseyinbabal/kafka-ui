//go:build integration

package itest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
	"github.com/huseyinbabal/kafka-ui/internal/kafka"
	"github.com/huseyinbabal/kafka-ui/internal/serde"
)

type sink struct {
	mu   sync.Mutex
	msgs []kafka.Message
}

func (s *sink) Message(m kafka.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, m)
	return nil
}
func (s *sink) Progress(kafka.Progress) {}

func strp(s string) *string { return &s }

func TestTopicLifecycle(t *testing.T) {
	c := newCluster(t)
	ctx := ctxT(t)
	name := uniq("it-topic")

	if _, err := kafka.CreateTopic(ctx, c.Admin, kafka.CreateTopicRequest{Name: name, Partitions: 2, ReplicationFactor: 1, ValidateOnly: true}); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if _, err := kafka.GetTopic(ctx, c.Admin, name); err == nil {
		t.Fatal("validateOnly must not create the topic")
	}
	created, err := kafka.CreateTopic(ctx, c.Admin, kafka.CreateTopicRequest{Name: name, Partitions: 2, ReplicationFactor: 1,
		Configs: map[string]*string{"retention.ms": strp("3600000")}})
	if err != nil || created.Partitions != 2 {
		t.Fatalf("create: %+v %v", created, err)
	}
	if _, err := kafka.CreateTopic(ctx, c.Admin, kafka.CreateTopicRequest{Name: name, Partitions: 1}); err == nil {
		t.Fatal("duplicate create must fail")
	}

	if err := kafka.AlterTopicConfigs(ctx, c.Admin, name, []kafka.ConfigChange{{Name: "cleanup.policy", Value: strp("bogus")}}); err == nil || kafka.BrokerMessage(err) == "" {
		t.Fatalf("invalid config must fail with the broker's message: %v", err)
	}
	if err := kafka.AlterTopicConfigs(ctx, c.Admin, name, []kafka.ConfigChange{{Name: "retention.ms", Value: strp("7200000")}}); err != nil {
		t.Fatal(err)
	}
	if err := kafka.SetPartitionCount(ctx, c.Admin, name, 1); err == nil {
		t.Fatal("shrinking must fail")
	}
	if err := kafka.SetPartitionCount(ctx, c.Admin, name, 3); err != nil {
		t.Fatal(err)
	}
	d, err := kafka.GetTopic(ctx, c.Admin, name)
	if err != nil || d.Partitions != 3 {
		t.Fatalf("describe: %+v %v", d, err)
	}
	for _, cfg := range d.Configs {
		if cfg.Name == "retention.ms" && (cfg.Value == nil || *cfg.Value != "7200000") {
			t.Fatalf("retention.ms = %v", cfg.Value)
		}
	}
	list, err := kafka.ListTopics(ctx, c.Admin)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ts := range list {
		found = found || ts.Name == name
	}
	if !found {
		t.Fatal("topic missing from list")
	}

	if err := kafka.DeleteTopic(ctx, c.Admin, "__consumer_offsets"); err == nil {
		t.Fatal("internal topics must be protected")
	}
	if err := kafka.DeleteTopic(ctx, c.Admin, name); err != nil {
		t.Fatal(err)
	}
}

func TestProduceBrowseAndPurge(t *testing.T) {
	c := newCluster(t)
	ctx := ctxT(t)
	topic := uniq("it-msgs")
	if _, err := kafka.CreateTopic(ctx, c.Admin, kafka.CreateTopicRequest{Name: topic, Partitions: 3, ReplicationFactor: 1}); err != nil {
		t.Fatal(err)
	}
	produce(t, c, topic, 3, 300)

	// Produce through the API path: partition chosen from the key, JSON validated, headers kept.
	res, err := kafka.Produce(ctx, c.Client, c.Admin, c.Serde, topic, kafka.ProduceRequest{
		Key: strp("order-1"), Value: strp(`{"status":"PAID"}`), ValueFormat: "json",
		Headers: []kafka.HeaderInput{{Key: "trace", Value: "abc"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kafka.Produce(ctx, c.Client, c.Admin, c.Serde, "it-no-such-topic", kafka.ProduceRequest{Value: strp("x")}); err == nil {
		t.Fatal("producing must not auto-create topics")
	}

	browse := func(req kafka.BrowseRequest) (kafka.BrowseResult, []kafka.Message) {
		t.Helper()
		req.Topic = topic
		s := &sink{}
		r, err := kafka.Browse(ctx, c.NewClient, c.Admin, req, s)
		if err != nil {
			t.Fatalf("%+v: %v", req, err)
		}
		return r, s.msgs
	}

	r, msgs := browse(kafka.BrowseRequest{Mode: kafka.SeekOldest, Limit: 500})
	if r.Reason != kafka.StopEnd || len(msgs) != 301 {
		t.Fatalf("oldest: %+v, %d msgs", r, len(msgs))
	}
	r, msgs = browse(kafka.BrowseRequest{Mode: kafka.SeekOldest, Limit: 10})
	if r.Reason != kafka.StopLimit || len(msgs) != 10 {
		t.Fatalf("limit: %+v", r)
	}
	_, msgs = browse(kafka.BrowseRequest{Mode: kafka.SeekNewest, Limit: 1})
	if len(msgs) != 1 || msgs[0].Key.Text != "order-1" || msgs[0].Value.Format != serde.JSON || msgs[0].Headers[0].Key != "trace" {
		t.Fatalf("newest: key=%+v value=%+v headers=%d", msgs[0].Key, msgs[0].Value, len(msgs[0].Headers))
	}
	f, _ := kafka.NewFilter("value", `^v-1\d$`, true)
	_, msgs = browse(kafka.BrowseRequest{Mode: kafka.SeekNewest, Limit: 100, Filter: f})
	if len(msgs) != 10 {
		t.Fatalf("regex filter matched %d, want 10 (v-10..v-19)", len(msgs))
	}
	_, msgs = browse(kafka.BrowseRequest{Mode: kafka.SeekOffset, Offset: 95, Partitions: []int32{1}, Limit: 500})
	want := 5 // offsets 95..99 of the 100 round-robin records in partition 1
	if res.Partition == 1 {
		want++ // plus the keyed record if it hashed to partition 1
	}
	if len(msgs) != want || msgs[0].Offset != 95 || msgs[0].Partition != 1 {
		t.Fatalf("offset seek: %d msgs (want %d), first offset %d", len(msgs), want, msgs[0].Offset)
	}
	_, msgs = browse(kafka.BrowseRequest{Mode: kafka.SeekTimestamp, Timestamp: time.Now().Add(time.Hour).UnixMilli(), Limit: 10})
	if len(msgs) != 0 {
		t.Fatalf("future timestamp must find nothing, got %d", len(msgs))
	}
	if _, err := kafka.Browse(ctx, c.NewClient, c.Admin, kafka.BrowseRequest{Topic: topic, Mode: kafka.SeekOldest, Partitions: []int32{9}}, &sink{}); err == nil {
		t.Fatal("unknown partition must fail")
	}

	m, err := kafka.GetMessage(ctx, c.NewClient, c.Admin, c.Decoder(), topic, res.Partition, res.Offset, serde.Auto, serde.Auto)
	if err != nil || m.Value.Text != `{"status":"PAID"}` {
		t.Fatalf("get message: %+v %v", m, err)
	}
	var ae *apierr.Error
	if _, err := kafka.GetMessage(ctx, c.NewClient, c.Admin, c.Decoder(), topic, 0, 99_999, serde.Auto, serde.Auto); !errors.As(err, &ae) || ae.Code != "MESSAGE_NOT_FOUND" {
		t.Fatalf("missing offset: %v", err)
	}

	purged, err := kafka.PurgeTopic(ctx, c.Admin, topic, nil)
	if err != nil || len(purged) != 3 {
		t.Fatalf("purge: %+v %v", purged, err)
	}
	_, msgs = browse(kafka.BrowseRequest{Mode: kafka.SeekOldest, Limit: 500})
	if len(msgs) != 0 {
		t.Fatalf("after purge: %d msgs", len(msgs))
	}
}

func TestLiveTailCancels(t *testing.T) {
	c := newCluster(t)
	topic := uniq("it-tail")
	if _, err := kafka.CreateTopic(ctxT(t), c.Admin, kafka.CreateTopicRequest{Name: topic, Partitions: 1, ReplicationFactor: 1}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &sink{}
	done := make(chan kafka.BrowseResult, 1)
	go func() {
		r, _ := kafka.Browse(ctx, c.NewClient, c.Admin, kafka.BrowseRequest{Topic: topic, Mode: kafka.SeekLive, Limit: 500}, s)
		done <- r
	}()
	time.Sleep(2 * time.Second)
	produce(t, c, topic, 1, 3)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		n := len(s.msgs)
		s.mu.Unlock()
		if n == 3 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	cancel()
	select {
	case r := <-done:
		if r.Matched != 3 || r.Reason != kafka.StopCancelled {
			t.Fatalf("live tail: %+v", r)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("live tail did not stop on cancel")
	}
}
