package analytics

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"

	"github.com/huseyinbabal/kafka-ui/internal/cluster"
)

const (
	sizeEvery    = time.Minute // log dir sizes are expensive; sample less often
	rateWindow   = time.Minute // partition rates for skew use this much history
	compactEvery = 5 * time.Minute
)

type tp struct {
	topic     string
	partition int32
}

type endsAt struct {
	at   time.Time
	ends map[tp]int64
}

// clusterState is the collector's per-cluster memory between ticks.
type clusterState struct {
	sizes    map[string]int64
	sizesAt  time.Time
	history  []endsAt // oldest first, covers about rateWindow
	lastLive *Live
}

// PartitionRate is one partition's ingress over the last minute.
type PartitionRate struct {
	Topic     string  `json:"topic"`
	Partition int32   `json:"partition"`
	Leader    int32   `json:"leader"`
	Rate      float64 `json:"rate"` // msg/s
}

// BrokerLoad counts leaders and replicas per broker.
type BrokerLoad struct {
	Broker   int32   `json:"broker"`
	Leaders  int     `json:"leaders"`
	Replicas int     `json:"replicas"`
	Ingress  float64 `json:"ingress"` // msg/s into partitions this broker leads
}

// Live is the latest in-memory view, used for skew (no history needed).
type Live struct {
	At         time.Time       `json:"at"`
	Partitions []PartitionRate `json:"partitions"`
	Brokers    []BrokerLoad    `json:"brokers"`
}

// Collector samples every cluster on an interval and writes to the Store.
type Collector struct {
	reg      *cluster.Registry
	store    *Store
	interval time.Duration
	log      *slog.Logger

	mu    sync.RWMutex
	state map[string]*clusterState
}

func NewCollector(reg *cluster.Registry, store *Store, interval time.Duration, log *slog.Logger) *Collector {
	return &Collector{reg: reg, store: store, interval: interval, log: log, state: map[string]*clusterState{}}
}

// Interval is the sampling interval (the finest chart resolution).
func (c *Collector) Interval() time.Duration { return c.interval }

// Live returns the latest skew view of a cluster, or nil before the first tick.
func (c *Collector) Live(clusterID string) *Live {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if st, ok := c.state[clusterID]; ok {
		return st.lastLive
	}
	return nil
}

// Run samples all clusters until ctx ends. One goroutine per cluster keeps a
// slow cluster from delaying the others.
func (c *Collector) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, cl := range c.reg.All() {
		c.mu.Lock()
		c.state[cl.Config.ID] = &clusterState{sizes: map[string]int64{}}
		c.mu.Unlock()
		wg.Go(func() { c.loop(ctx, cl) })
	}
	wg.Go(func() {
		t := time.NewTicker(compactEvery)
		defer t.Stop()
		for {
			if err := c.store.Compact(ctx); err != nil && ctx.Err() == nil {
				c.log.Error("analytics compaction failed", "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	})
	wg.Wait()
}

func (c *Collector) loop(ctx context.Context, cl *cluster.Cluster) {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	failing := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if cl.Admin == nil || cl.Health().Status == cluster.StatusOffline {
			continue
		}
		tctx, cancel := context.WithTimeout(ctx, max(c.interval, 5*time.Second))
		err := c.tick(tctx, cl)
		cancel()
		switch {
		case err != nil && ctx.Err() == nil && !failing:
			c.log.Warn("analytics sample failed", "cluster", cl.Config.ID, "err", err)
			failing = true
		case err == nil && failing:
			c.log.Info("analytics sampling recovered", "cluster", cl.Config.ID)
			failing = false
		}
	}
}

func (c *Collector) tick(ctx context.Context, cl *cluster.Cluster) error {
	c.mu.RLock()
	st := c.state[cl.Config.ID]
	c.mu.RUnlock()
	now := time.Now()
	adm := cl.Admin

	md, err := adm.Metadata(ctx)
	if err != nil {
		return err
	}
	ends, err := adm.ListEndOffsets(ctx)
	if err != nil && !partial(err) {
		return err
	}
	starts, err := adm.ListStartOffsets(ctx)
	if err != nil && !partial(err) {
		return err
	}
	if now.Sub(st.sizesAt) >= sizeEvery {
		if sizes, err := topicSizes(ctx, adm, md); err == nil {
			st.sizes, st.sizesAt = sizes, now
		}
	}

	snap := Snapshot{Cluster: cl.Config.ID, At: now}
	cur := map[tp]int64{}
	leaders := map[tp]int32{}
	loads := map[int32]*BrokerLoad{}
	for _, b := range md.Brokers {
		loads[b.NodeID] = &BrokerLoad{Broker: b.NodeID}
	}
	for _, t := range md.Topics {
		if t.Err != nil {
			continue
		}
		ts := TopicSample{Topic: t.Topic, SizeBytes: -1}
		if size, ok := st.sizes[t.Topic]; ok {
			ts.SizeBytes = size
		}
		for _, p := range t.Partitions {
			snap.Overall.Partitions++
			if p.Leader < 0 {
				snap.Overall.Offline++
			}
			if len(p.ISR) < len(p.Replicas) {
				snap.Overall.URP++
			}
			if l, ok := loads[p.Leader]; ok {
				l.Leaders++
			}
			for _, r := range p.Replicas {
				if l, ok := loads[r]; ok {
					l.Replicas++
				}
			}
			key := tp{t.Topic, p.Partition}
			leaders[key] = p.Leader
			if o, ok := ends.Lookup(t.Topic, p.Partition); ok && o.Err == nil {
				ts.EndSum += o.Offset
				cur[key] = o.Offset
			}
			if o, ok := starts.Lookup(t.Topic, p.Partition); ok && o.Err == nil {
				ts.StartSum += o.Offset
			}
		}
		snap.Topics = append(snap.Topics, ts)
	}

	if groups, err := groupSamples(ctx, adm); err == nil {
		snap.Groups = groups
	} else if !errors.Is(err, context.DeadlineExceeded) {
		c.log.Debug("analytics group sample failed", "cluster", cl.Config.ID, "err", err)
	}

	if err := c.store.Write(ctx, snap); err != nil {
		return err
	}

	// Partition rates over about a minute, for skew.
	st.history = append(st.history, endsAt{at: now, ends: cur})
	for len(st.history) > 2 && now.Sub(st.history[1].at) >= rateWindow {
		st.history = st.history[1:]
	}
	live := &Live{At: now, Partitions: []PartitionRate{}, Brokers: []BrokerLoad{}}
	if old := st.history[0]; len(st.history) > 1 {
		dt := now.Sub(old.at).Seconds()
		for key, end := range cur {
			prev, ok := old.ends[key]
			if !ok || dt <= 0 {
				continue
			}
			r := float64(max(end-prev, 0)) / dt
			live.Partitions = append(live.Partitions, PartitionRate{Topic: key.topic, Partition: key.partition, Leader: leaders[key], Rate: round2(r)})
			if l, ok := loads[leaders[key]]; ok {
				l.Ingress += r
			}
		}
	}
	sort.Slice(live.Partitions, func(i, j int) bool {
		a, b := live.Partitions[i], live.Partitions[j]
		if a.Topic != b.Topic {
			return a.Topic < b.Topic
		}
		return a.Partition < b.Partition
	})
	for _, l := range loads {
		l.Ingress = round2(l.Ingress)
		live.Brokers = append(live.Brokers, *l)
	}
	sort.Slice(live.Brokers, func(i, j int) bool { return live.Brokers[i].Broker < live.Brokers[j].Broker })
	c.mu.Lock()
	st.lastLive = live
	c.mu.Unlock()
	return nil
}

// topicSizes sums each partition's leader-replica size per topic.
func topicSizes(ctx context.Context, adm *kadm.Client, md kadm.Metadata) (map[string]int64, error) {
	dirs, err := adm.DescribeAllLogDirs(ctx, nil)
	if err != nil && !partial(err) {
		return nil, err
	}
	leader := map[tp]int32{}
	for _, t := range md.Topics {
		for _, p := range t.Partitions {
			leader[tp{t.Topic, p.Partition}] = p.Leader
		}
	}
	out := map[string]int64{}
	dirs.Each(func(d kadm.DescribedLogDir) {
		if d.Err != nil {
			return
		}
		d.Topics.Each(func(p kadm.DescribedLogDirPartition) {
			if leader[tp{p.Topic, p.Partition}] == d.Broker {
				out[p.Topic] += p.Size
			}
		})
	})
	return out, nil
}

// groupSamples sums committed offsets and lag per group and topic. Like
// kafka-consumer-groups.sh, partitions without a commit add no lag.
func groupSamples(ctx context.Context, adm *kadm.Client) ([]GroupSample, error) {
	listed, err := adm.ListGroups(ctx)
	if err != nil && !partial(err) {
		return nil, err
	}
	if len(listed) == 0 {
		return nil, nil
	}
	lags, err := adm.Lag(ctx, listed.Groups()...)
	if err != nil {
		return nil, err
	}
	var out []GroupSample
	for _, l := range lags.Sorted() {
		for topic, ps := range l.Lag {
			gs := GroupSample{Group: l.Group, Topic: topic}
			committed := false
			for _, p := range ps {
				if p.Commit.At < 0 {
					continue
				}
				committed = true
				gs.CommittedSum += p.Commit.At
				if p.Lag > 0 {
					gs.Lag += p.Lag
				}
			}
			if committed {
				out = append(out, gs)
			}
		}
	}
	return out, nil
}

func partial(err error) bool {
	var se *kadm.ShardErrors
	return errors.As(err, &se) && !se.AllFailed
}

// PartitionRates returns the last minute's ingress per topic and partition (msg/s).
func (c *Collector) PartitionRates(clusterID string) map[string]map[int32]float64 {
	live := c.Live(clusterID)
	if live == nil {
		return nil
	}
	out := map[string]map[int32]float64{}
	for _, p := range live.Partitions {
		if out[p.Topic] == nil {
			out[p.Topic] = map[int32]float64{}
		}
		out[p.Topic][p.Partition] = p.Rate
	}
	return out
}
