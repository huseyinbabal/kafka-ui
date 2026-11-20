//go:build integration

package itest

import (
	"errors"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
	"github.com/huseyinbabal/kafka-ui/internal/kafka"
)

// consume joins group, reads n records, commits, and returns the still-joined client.
func consume(t *testing.T, bootstrap []string, group, topic string, n int) *kgo.Client {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(bootstrap...), kgo.ConsumerGroup(group), kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), kgo.DisableAutoCommit())
	if err != nil {
		t.Fatal(err)
	}
	ctx := ctxT(t)
	got := 0
	for got < n {
		fs := cl.PollRecords(ctx, n-got)
		if err := fs.Err0(); err != nil {
			t.Fatal(err)
		}
		got += fs.NumRecords()
	}
	if err := cl.CommitUncommittedOffsets(ctx); err != nil {
		t.Fatal(err)
	}
	return cl
}

func TestGroupLagResetAndDelete(t *testing.T) {
	c := newCluster(t)
	ctx := ctxT(t)
	topic, group := uniq("it-grp-topic"), uniq("it-grp")
	if _, err := kafka.CreateTopic(ctx, c.Admin, kafka.CreateTopicRequest{Name: topic, Partitions: 2, ReplicationFactor: 1}); err != nil {
		t.Fatal(err)
	}
	produce(t, c, topic, 2, 100)
	member := consume(t, env.bootstrap, group, topic, 60)

	g, err := kafka.GetGroup(ctx, c.Admin, group)
	if err != nil {
		t.Fatal(err)
	}
	if g.State != "Stable" || len(g.MemberList) != 1 || g.TotalLag != 40 {
		t.Fatalf("active group: state %s members %d lag %d", g.State, len(g.MemberList), g.TotalLag)
	}
	consumers, err := kafka.GroupsForTopic(ctx, c.Admin, topic)
	if err != nil || len(consumers) != 1 || consumers[0].ID != group {
		t.Fatalf("groups for topic: %+v %v", consumers, err)
	}

	var ae *apierr.Error
	if _, err := kafka.ResetOffsets(ctx, c.Admin, group, kafka.ResetRequest{Topic: topic, Strategy: kafka.ResetEarliest, DryRun: true}); !errors.As(err, &ae) || ae.Code != "GROUP_NOT_EMPTY" {
		t.Fatalf("reset of an active group must be refused: %v", err)
	}
	if err := kafka.DeleteGroup(ctx, c.Admin, group); !errors.As(err, &ae) || ae.Code != "GROUP_NOT_EMPTY" {
		t.Fatalf("delete of an active group must be refused: %v", err)
	}

	member.Close()
	waitFor(t, 30*time.Second, func() bool {
		g, err := kafka.GetGroup(ctx, c.Admin, group)
		return err == nil && g.State == "Empty"
	})

	plan, err := kafka.ResetOffsets(ctx, c.Admin, group, kafka.ResetRequest{Topic: topic, Strategy: kafka.ResetShift, Value: -10, DryRun: true})
	if err != nil || plan.Applied || len(plan.Items) != 2 {
		t.Fatalf("dry run: %+v %v", plan, err)
	}
	for _, it := range plan.Items {
		if it.Target != it.Current-10 {
			t.Fatalf("shift plan %+v", it)
		}
	}
	g, _ = kafka.GetGroup(ctx, c.Admin, group)
	if g.TotalLag != 40 {
		t.Fatalf("dry run must not commit; lag %d", g.TotalLag)
	}

	applied, err := kafka.ResetOffsets(ctx, c.Admin, group, kafka.ResetRequest{Topic: topic, Strategy: kafka.ResetLatest})
	if err != nil || !applied.Applied {
		t.Fatalf("apply: %+v %v", applied, err)
	}
	g, _ = kafka.GetGroup(ctx, c.Admin, group)
	if g.TotalLag != 0 {
		t.Fatalf("after reset to latest lag = %d", g.TotalLag)
	}
	if _, err := kafka.ResetOffsets(ctx, c.Admin, group, kafka.ResetRequest{Topic: topic, Strategy: kafka.ResetOffset, Value: 5, Partitions: []int32{0}}); err != nil {
		t.Fatal(err)
	}
	g, _ = kafka.GetGroup(ctx, c.Admin, group)
	for _, p := range g.Partitions {
		if p.Partition == 0 && p.Committed != 5 {
			t.Fatalf("partition 0 committed %d, want 5", p.Committed)
		}
	}

	if err := kafka.DeleteGroup(ctx, c.Admin, group); err != nil {
		t.Fatal(err)
	}
	if _, err := kafka.GetGroup(ctx, c.Admin, group); !errors.As(err, &ae) || ae.Code != "GROUP_NOT_FOUND" {
		t.Fatalf("deleted group: %v", err)
	}
}

func waitFor(t *testing.T, d time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}
