//go:build integration

package itest

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/huseyinbabal/kafka-ui/internal/analytics"
	"github.com/huseyinbabal/kafka-ui/internal/cluster"
	"github.com/huseyinbabal/kafka-ui/internal/config"
	"github.com/huseyinbabal/kafka-ui/internal/kafka"
	"github.com/huseyinbabal/kafka-ui/internal/live"
	"github.com/huseyinbabal/kafka-ui/internal/storage"
)

func TestCollectorAndLiveSnapshot(t *testing.T) {
	ctx := ctxT(t)
	reg, err := cluster.NewRegistry([]config.ClusterConfig{{ID: "it", Bootstrap: env.bootstrap}}, quiet)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reg.Close)
	c, _ := reg.Get("it")
	topic, group := uniq("it-an"), uniq("it-an-grp")
	if _, err := kafka.CreateTopic(ctx, c.Admin, kafka.CreateTopicRequest{Name: topic, Partitions: 2, ReplicationFactor: 1}); err != nil {
		t.Fatal(err)
	}
	produce(t, c, topic, 2, 50)
	member := consume(t, env.bootstrap, group, topic, 20)
	defer member.Close()

	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	store := analytics.NewStore(db, 24*time.Hour)
	col := analytics.NewCollector(reg, store, time.Second, quiet)
	runCtx, stop := context.WithCancel(ctx)
	go col.Run(runCtx)
	defer stop()

	// Keep producing while the collector runs until the store shows ingress for
	// the topic (1s buckets, so samples never collapse into a single bucket).
	var o *analytics.Overview
	var ts *analytics.TopicStat
	waitFor(t, 30*time.Second, func() bool {
		produce(t, c, topic, 2, 20)
		w := analytics.Window{From: time.Now().Add(-5 * time.Minute), To: time.Now(), Step: time.Second}
		var err error
		if o, err = store.Overview(ctx, "it", w); err != nil {
			t.Fatal(err)
		}
		ts = nil
		for i := range o.TopTopics {
			if o.TopTopics[i].Topic == topic {
				ts = &o.TopTopics[i]
			}
		}
		return ts != nil && ts.Messages > 0 && ts.MsgRate > 0 && len(col.PartitionRates("it")[topic]) == 2
	})
	w := analytics.Window{From: time.Now().Add(-5 * time.Minute), To: time.Now(), Step: time.Second}
	var gs *analytics.GroupStat
	for i := range o.Groups {
		if o.Groups[i].Group == group {
			gs = &o.Groups[i]
		}
	}
	if gs == nil || gs.Lag < 30 {
		t.Fatalf("group stats %+v (expected lag ≥ 30)", gs)
	}
	pts, err := store.TopicSeries(ctx, "it", topic, w)
	if err != nil || len(pts) < 2 {
		t.Fatalf("topic series %d %v", len(pts), err)
	}

	// Live snapshot over the same cluster: broker, topic, group, member and the links between them.
	hub := live.NewHub(ctx, 500*time.Millisecond, col.PartitionRates, quiet)
	events, unsubscribe := hub.Subscribe(c)
	defer unsubscribe()
	var snap *live.Snapshot
	waitFor(t, 15*time.Second, func() bool {
		select {
		case ev := <-events:
			if ev.Err != nil {
				t.Fatal(ev.Err)
			}
			snap = ev.Snapshot
			return snap != nil
		default:
			return false
		}
	})
	kinds := map[live.NodeKind]int{}
	for _, n := range snap.Nodes {
		kinds[n.Kind]++
	}
	links := map[string]bool{}
	for _, l := range snap.Links {
		links[string(l.Kind)+" "+l.Source+" "+l.Target] = true
	}
	if kinds[live.KindBroker] != 1 || kinds[live.KindMember] < 1 || !links["offsets group:"+group+" topic:"+topic] {
		t.Fatalf("snapshot kinds %v links %v", kinds, links)
	}
	controller := false
	for _, n := range snap.Nodes {
		controller = controller || (n.Kind == live.KindBroker && n.Controller)
	}
	if !controller {
		t.Fatal("broker must be marked as controller")
	}

	hygiene, err := kafka.Hygiene(ctx, c.Admin)
	if err != nil {
		t.Fatal(err)
	}
	lowRF := false
	for _, i := range hygiene {
		lowRF = lowRF || (i.Kind == "low-rf" && i.Target == topic)
	}
	if !lowRF {
		t.Fatalf("RF 1 topic must be flagged: %+v", hygiene)
	}
}
