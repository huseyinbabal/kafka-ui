package live

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/huseyinbabal/kafka-ui/internal/cluster"
)

// Event is what subscribers receive: a snapshot, or the error of a failed tick.
type Event struct {
	Snapshot *Snapshot
	Err      error
}

type feed struct {
	subs   map[chan Event]struct{}
	last   *Event
	cancel context.CancelFunc
}

// Hub runs one snapshot builder per watched cluster and fans its output out
// to all subscribers. A builder only runs while someone is watching.
type Hub struct {
	interval time.Duration
	log      *slog.Logger
	build    func(ctx context.Context, cl *cluster.Cluster, tr *consumeTracker) (*Snapshot, error)

	mu    sync.Mutex
	root  context.Context
	feeds map[string]*feed
}

func NewHub(root context.Context, interval time.Duration, rates RateSource, log *slog.Logger) *Hub {
	if rates == nil {
		rates = func(string) map[string]map[int32]float64 { return nil }
	}
	return &Hub{
		interval: interval, log: log, root: root, feeds: map[string]*feed{},
		build: func(ctx context.Context, cl *cluster.Cluster, tr *consumeTracker) (*Snapshot, error) {
			return build(ctx, cl, rates, tr)
		},
	}
}

// Interval is how often snapshots are built.
func (h *Hub) Interval() time.Duration { return h.interval }

// Subscribe starts receiving events for cl. The latest snapshot, if any, is
// delivered first. Call the returned func to stop.
func (h *Hub) Subscribe(cl *cluster.Cluster) (<-chan Event, func()) {
	ch := make(chan Event, 1)
	h.mu.Lock()
	id := cl.Config.ID
	f := h.feeds[id]
	if f == nil {
		ctx, cancel := context.WithCancel(h.root)
		f = &feed{subs: map[chan Event]struct{}{}, cancel: cancel}
		h.feeds[id] = f
		go h.run(ctx, cl, f)
	} else if f.last != nil {
		ch <- *f.last
	}
	f.subs[ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			delete(f.subs, ch)
			if len(f.subs) == 0 {
				f.cancel()
				if h.feeds[id] == f {
					delete(h.feeds, id)
				}
			}
		})
	}
}

func (h *Hub) run(ctx context.Context, cl *cluster.Cluster, f *feed) {
	tr := &consumeTracker{}
	t := time.NewTicker(h.interval)
	defer t.Stop()
	for {
		tctx, cancel := context.WithTimeout(ctx, max(h.interval*3, 5*time.Second))
		snap, err := h.build(tctx, cl, tr)
		cancel()
		if ctx.Err() != nil {
			return
		}
		ev := Event{Snapshot: snap, Err: err}
		h.mu.Lock()
		f.last = &ev
		for ch := range f.subs {
			// Keep only the newest event for slow subscribers.
			select {
			case ch <- ev:
			default:
				select {
				case <-ch:
				default:
				}
				ch <- ev
			}
		}
		h.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Watching reports how many clusters currently have an active builder (for tests and metrics).
func (h *Hub) Watching() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.feeds)
}
