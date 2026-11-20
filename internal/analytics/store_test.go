package analytics

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/huseyinbabal/kafka-ui/internal/storage"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewStore(db, 7*24*time.Hour)
}

var t0 = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

// fill writes n ticks 5s apart: "orders" grows 10 msg/s (1000 B/s), "idle" never
// moves, and group "ledger" consumes 8 msg/s of orders so its lag grows 2 msg/s.
func fill(t *testing.T, s *Store, n int) {
	t.Helper()
	for i := range n {
		at := t0.Add(time.Duration(i) * 5 * time.Second)
		end := int64(1000 + i*50)
		snap := Snapshot{Cluster: "c", At: at,
			Topics: []TopicSample{
				{Topic: "orders", EndSum: end, StartSum: 0, SizeBytes: end * 100},
				{Topic: "idle", EndSum: 7, SizeBytes: 70},
				{Topic: "__consumer_offsets", EndSum: int64(i * 1000), SizeBytes: 1},
			},
			Groups:  []GroupSample{{Group: "ledger", Topic: "orders", CommittedSum: 900 + int64(i*40), Lag: end - (900 + int64(i*40))}},
			Overall: ClusterSample{Partitions: 3},
		}
		if i == 0 {
			// A group seen only at the start (deleted later) must not show up as current.
			snap.Groups = append(snap.Groups, GroupSample{Group: "gone", Topic: "orders", CommittedSum: 1, Lag: 999})
		}
		if err := s.Write(context.Background(), snap); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOverviewRatesAndCatchUp(t *testing.T) {
	s := newStore(t)
	fill(t, s, 120) // 10 minutes
	w := Window{From: t0.Add(-time.Minute), To: t0.Add(10 * time.Minute), Step: 5 * time.Second}
	o, err := s.Overview(context.Background(), "c", w)
	if err != nil {
		t.Fatal(err)
	}
	if o.CollectedSince != t0.UnixMilli() {
		t.Fatalf("collectedSince %d", o.CollectedSince)
	}
	last := o.Throughput[len(o.Throughput)-1]
	if last.Messages != 10 {
		t.Fatalf("throughput %+v (internal topics must be excluded)", last)
	}
	if o.BytesStep != time.Minute.Milliseconds() || o.Bytes[len(o.Bytes)-1].Bytes != 1000 {
		t.Fatalf("bytes must use minute buckets: step %d, %+v", o.BytesStep, o.Bytes[len(o.Bytes)-1])
	}
	if o.TopTopics[0].Topic != "orders" || o.TopTopics[0].MsgRate != 10 || o.TopTopics[0].Messages != 119*50 {
		t.Fatalf("top topics %+v", o.TopTopics)
	}
	if len(o.Groups) != 1 || o.Groups[0].Group != "ledger" {
		t.Fatalf("groups %+v (stale group must be dropped)", o.Groups)
	}
	g := o.Groups[0]
	if g.ConsumeRate != 8 || g.IngressRate != 10 || g.CatchUpSeconds != nil {
		t.Fatalf("ledger is falling behind, no catch-up expected: %+v", g)
	}
	if len(o.Lag) == 0 || o.Lag[len(o.Lag)-1].Lag != g.Lag {
		t.Fatalf("lag series end %v vs %d", o.Lag[len(o.Lag)-1], g.Lag)
	}
}

func TestCatchUpEstimate(t *testing.T) {
	s := newStore(t)
	for i := range 80 {
		// consumes 20 msg/s, ingress 10 msg/s, so lag shrinks 10 msg/s from 5000
		if err := s.Write(context.Background(), Snapshot{Cluster: "c", At: t0.Add(time.Duration(i) * 5 * time.Second),
			Topics: []TopicSample{{Topic: "t", EndSum: int64(5000 + i*50)}},
			Groups: []GroupSample{{Group: "g", Topic: "t", CommittedSum: int64(3000 + i*100), Lag: int64(5000 - i*50)}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	o, err := s.Overview(context.Background(), "c", Window{From: t0, To: t0.Add(time.Hour), Step: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	g := o.Groups[0]
	// lag 1050 at net 10 msg/s → 105s
	if g.Lag != 1050 || g.CatchUpSeconds == nil || *g.CatchUpSeconds != 105 {
		t.Fatalf("expected a 105s estimate: %+v", g)
	}
}

func TestCompactAndRetention(t *testing.T) {
	s := newStore(t)
	fill(t, s, 120)
	// Pretend 7 hours passed: everything is older than RawFor and gets rolled up.
	s.now = func() time.Time { return t0.Add(7 * time.Hour) }
	if err := s.Compact(context.Background()); err != nil {
		t.Fatal(err)
	}
	var raw, rolled int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM topic_samples_raw`).Scan(&raw)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM topic_samples_1m WHERE topic = 'orders'`).Scan(&rolled)
	if raw != 0 || rolled != 10 {
		t.Fatalf("raw=%d rolled=%d, want 0 and 10 minute buckets", raw, rolled)
	}
	// Rates still come out right from minute rollups.
	w := Window{From: t0, To: t0.Add(time.Hour), Step: time.Minute}
	pts, err := s.TopicSeries(context.Background(), "c", "orders", w)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 10 || pts[5].MsgRate != 10 {
		t.Fatalf("rollup series %+v", pts)
	}
	// Compacting twice must not duplicate or lose data.
	if err := s.Compact(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM topic_samples_1m WHERE topic = 'orders'`).Scan(&rolled)
	if rolled != 10 {
		t.Fatalf("second compact changed rollups: %d", rolled)
	}
	// After the retention period everything is gone.
	s.now = func() time.Time { return t0.Add(8 * 24 * time.Hour) }
	if err := s.Compact(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM topic_samples_1m`).Scan(&rolled)
	if rolled != 0 {
		t.Fatalf("retention left %d rows", rolled)
	}
}

func TestGroupSeriesAndIdleTopics(t *testing.T) {
	s := newStore(t)
	fill(t, s, 24)
	w := Window{From: t0, To: t0.Add(2 * time.Minute), Step: 10 * time.Second}
	pts, err := s.GroupSeries(context.Background(), "c", "ledger", w)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 12 || pts[3].ConsumeRate != 8 || pts[3].ByTopic["orders"] != pts[3].Lag {
		t.Fatalf("group series %+v", pts[:4])
	}
	idle, err := s.IdleTopics(context.Background(), "c", w)
	if err != nil {
		t.Fatal(err)
	}
	if len(idle) != 1 || idle[0] != "idle" {
		t.Fatalf("idle %v", idle)
	}
}

func TestNewWindow(t *testing.T) {
	w, err := NewWindow("15m", t0, 5*time.Second)
	if err != nil || w.Step != 5*time.Second {
		t.Fatalf("15m: %v %v", w.Step, err)
	}
	w, _ = NewWindow("7d", t0, 5*time.Second)
	if w.Step != 42*time.Minute {
		t.Fatalf("7d step %v", w.Step)
	}
	w, _ = NewWindow("24h", t0, 5*time.Second)
	if w.Step < time.Minute {
		t.Fatalf("24h must use at least minute buckets: %v", w.Step)
	}
	if _, err := NewWindow("2y", t0, time.Second); err == nil {
		t.Fatal("unknown range must fail")
	}
}

func TestPartialLastBucketRate(t *testing.T) {
	s := newStore(t)
	// 10 msg/s, sampled every 5s for 70s: the last 60s bucket holds only 2 samples.
	for i := range 15 {
		if err := s.Write(context.Background(), Snapshot{Cluster: "c", At: t0.Add(time.Duration(i) * 5 * time.Second),
			Topics: []TopicSample{{Topic: "t", EndSum: int64(i * 50)}}}); err != nil {
			t.Fatal(err)
		}
	}
	o, err := s.Overview(context.Background(), "c", Window{From: t0, To: t0.Add(2 * time.Minute), Step: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if got := o.Throughput[len(o.Throughput)-1].Messages; got != 10 {
		t.Fatalf("partial bucket rate %v, want 10", got)
	}
}
