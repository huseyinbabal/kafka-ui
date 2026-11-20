package live

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"

	"github.com/huseyinbabal/kafka-ui/internal/cluster"
	"github.com/huseyinbabal/kafka-ui/internal/config"
)

func TestConsumeTrackerRates(t *testing.T) {
	tr := &consumeTracker{}
	t0 := time.Now()
	lag := func(commit int64) kadm.DescribedGroupLags {
		return kadm.DescribedGroupLags{"g": {Group: "g", Lag: kadm.GroupLag{"t": {
			0: {Commit: kadm.Offset{At: commit}},
			1: {Commit: kadm.Offset{At: -1}},
		}}}}
	}
	if r := tr.rates(t0, lag(100)); r["g"]["t"][0] != 0 {
		t.Fatalf("first sample has no rate: %v", r)
	}
	r := tr.rates(t0.Add(2*time.Second), lag(140))
	if r["g"]["t"][0] != 20 {
		t.Fatalf("rate %v, want 20 msg/s", r["g"]["t"][0])
	}
	if _, ok := r["g"]["t"][1]; ok {
		t.Fatal("uncommitted partition must have no rate")
	}
	// A bursty consumer: no new commit for a few ticks keeps a smoothed, non-zero rate.
	r = tr.rates(t0.Add(4*time.Second), lag(140))
	if r["g"]["t"][0] != 10 {
		t.Fatalf("smoothed rate %v, want 10 (40 msgs over 4s)", r["g"]["t"][0])
	}
	// The window slides: after 30s+ the oldest samples drop out.
	for i := 3; i <= 20; i++ {
		tr.rates(t0.Add(time.Duration(i*2)*time.Second), lag(140))
	}
	if r := tr.rates(t0.Add(42*time.Second), lag(140)); r["g"]["t"][0] != 0 {
		t.Fatalf("idle for longer than the window must be 0: %v", r)
	}
	// Offsets reset backwards: no negative rate.
	if r := tr.rates(t0.Add(44*time.Second), lag(10)); r["g"]["t"][0] != 0 {
		t.Fatalf("reset must not produce a rate: %v", r)
	}
}

func testHub(t *testing.T, builds *atomic.Int32) (*Hub, *cluster.Cluster) {
	t.Helper()
	reg, err := cluster.NewRegistry([]config.ClusterConfig{{ID: "c", Bootstrap: []string{"127.0.0.1:1"}}},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reg.Close)
	cl, _ := reg.Get("c")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h := NewHub(ctx, 20*time.Millisecond, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.build = func(context.Context, *cluster.Cluster, *consumeTracker) (*Snapshot, error) {
		n := builds.Add(1)
		return &Snapshot{At: int64(n)}, nil
	}
	return h, cl
}

func TestHubFanOutAndStop(t *testing.T) {
	var builds atomic.Int32
	h, cl := testHub(t, &builds)
	a, stopA := h.Subscribe(cl)
	first := <-a
	if first.Snapshot == nil {
		t.Fatal("expected a snapshot")
	}
	// A second subscriber shares the builder and gets the latest snapshot right away.
	b, stopB := h.Subscribe(cl)
	select {
	case ev := <-b:
		if ev.Snapshot == nil {
			t.Fatal("expected a snapshot")
		}
	case <-time.After(time.Second):
		t.Fatal("second subscriber got nothing")
	}
	if h.Watching() != 1 {
		t.Fatalf("one builder per cluster, got %d", h.Watching())
	}
	stopA()
	stopA() // idempotent
	if h.Watching() != 1 {
		t.Fatal("builder must keep running while someone watches")
	}
	stopB()
	if h.Watching() != 0 {
		t.Fatal("builder must stop with the last subscriber")
	}
	n := builds.Load()
	time.Sleep(100 * time.Millisecond)
	if builds.Load() > n+1 {
		t.Fatalf("builder kept running after stop: %d → %d", n, builds.Load())
	}
}

func TestHubSlowSubscriberGetsNewest(t *testing.T) {
	var builds atomic.Int32
	h, cl := testHub(t, &builds)
	ch, stop := h.Subscribe(cl)
	defer stop()
	time.Sleep(150 * time.Millisecond) // several ticks without reading
	ev := <-ch
	if ev.Snapshot.At < 3 {
		t.Fatalf("expected a recent snapshot, got #%d", ev.Snapshot.At)
	}
}
