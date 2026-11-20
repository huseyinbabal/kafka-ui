// Package live builds topology snapshots (brokers, topics, groups, consumers)
// and streams them to subscribers.
package live

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"

	"github.com/huseyinbabal/kafka-ui/internal/cluster"
	"github.com/huseyinbabal/kafka-ui/internal/kafka"
)

type NodeKind string

const (
	KindBroker NodeKind = "broker"
	KindTopic  NodeKind = "topic"
	KindGroup  NodeKind = "group"
	KindMember NodeKind = "member"
)

// Node is one vertex. IDs are "<kind>:<name>" so they are stable across snapshots.
type Node struct {
	ID    string   `json:"id"`
	Kind  NodeKind `json:"kind"`
	Label string   `json:"label"`
	// Rate is msg/s: ingress for topics and brokers (as leader), consume rate for groups/members.
	Rate float64 `json:"rate"`

	Controller bool   `json:"controller,omitempty"` // broker
	Host       string `json:"host,omitempty"`       // broker, member
	Leaders    int    `json:"leaders,omitempty"`    // broker

	Partitions int  `json:"partitions,omitempty"` // topic: count; member: assigned
	Internal   bool `json:"internal,omitempty"`   // topic

	State   string `json:"state,omitempty"`   // group
	Lag     int64  `json:"lag,omitempty"`     // group, member (sum over assigned partitions)
	Members int    `json:"members,omitempty"` // group

	Group    string `json:"group,omitempty"`    // member
	ClientID string `json:"clientId,omitempty"` // member
}

type LinkKind string

const (
	LinkLeader   LinkKind = "leader"   // topic → broker leading some of its partitions
	LinkMember   LinkKind = "member"   // group → member
	LinkConsumes LinkKind = "consumes" // member → topic
	LinkOffsets  LinkKind = "offsets"  // group → topic it has committed offsets on
)

type Link struct {
	Source     string   `json:"source"`
	Target     string   `json:"target"`
	Kind       LinkKind `json:"kind"`
	Partitions int      `json:"partitions"`
	Rate       float64  `json:"rate"` // msg/s flowing along the link
}

type Snapshot struct {
	At    int64  `json:"at"`
	Nodes []Node `json:"nodes"`
	Links []Link `json:"links"`
}

// RateSource provides rates measured elsewhere (the analytics collector), so a
// freshly opened view shows real numbers instead of waiting for its own history.
// Either method may return nil when nothing is known yet.
type RateSource interface {
	// PartitionRates is ingress per topic and partition (msg/s).
	PartitionRates(cluster string) map[string]map[int32]float64
	// ConsumeRates is consumption per group, topic and partition (msg/s).
	ConsumeRates(cluster string) map[string]map[string]map[int32]float64
}

type noRates struct{}

func (noRates) PartitionRates(string) map[string]map[int32]float64 { return nil }
func (noRates) ConsumeRates(string) map[string]map[string]map[int32]float64 {
	return nil
}

func nodeID(kind NodeKind, name string) string { return string(kind) + ":" + name }

// consumeWindow smooths consume rates: consumers that commit in bursts (every
// few seconds) would otherwise flicker between 0 and a spike.
const consumeWindow = 30 * time.Second

type commitSample struct {
	at        time.Time
	committed map[string]map[string]map[int32]int64 // group → topic → partition → offset
}

// consumeTracker remembers recent committed offsets to derive consume rates.
type consumeTracker struct {
	history []commitSample // oldest first, about consumeWindow long
}

func (t *consumeTracker) rates(now time.Time, lags kadm.DescribedGroupLags) map[string]map[string]map[int32]float64 {
	cur := map[string]map[string]map[int32]int64{}
	for g, l := range lags {
		cur[g] = map[string]map[int32]int64{}
		for topic, ps := range l.Lag {
			cur[g][topic] = map[int32]int64{}
			for p, pl := range ps {
				if pl.Commit.At >= 0 {
					cur[g][topic][p] = pl.Commit.At
				}
			}
		}
	}
	out := map[string]map[string]map[int32]float64{}
	if len(t.history) > 0 {
		old := t.history[0]
		dt := now.Sub(old.at).Seconds()
		for g, topics := range cur {
			out[g] = map[string]map[int32]float64{}
			for topic, ps := range topics {
				out[g][topic] = map[int32]float64{}
				for p, at := range ps {
					if prev, ok := old.committed[g][topic][p]; ok && dt > 0 && at >= prev {
						out[g][topic][p] = float64(at-prev) / dt
					}
				}
			}
		}
	}
	t.history = append(t.history, commitSample{at: now, committed: cur})
	for len(t.history) > 2 && now.Sub(t.history[1].at) >= consumeWindow {
		t.history = t.history[1:]
	}
	return out
}

// build assembles one snapshot.
func build(ctx context.Context, cl *cluster.Cluster, rates RateSource, tr *consumeTracker) (*Snapshot, error) {
	adm := cl.Admin
	if adm == nil {
		return nil, errors.New("cluster is not connected")
	}
	md, err := adm.Metadata(ctx)
	if err != nil {
		return nil, err
	}
	controller := kafka.ActiveController(ctx, cl.Client, md.Controller)
	listed, err := adm.ListGroups(ctx)
	if err != nil && !partial(err) {
		return nil, err
	}
	var lags kadm.DescribedGroupLags
	if len(listed) > 0 {
		if lags, err = adm.Lag(ctx, listed.Groups()...); err != nil {
			return nil, fmt.Errorf("describe groups: %w", err)
		}
	}
	now := time.Now()
	in := rates.PartitionRates(cl.Config.ID)
	consumed := tr.rates(now, lags)
	// Prefer the collector's minute-long view: the local window starts empty
	// when a view opens, and batch committers would read as idle until it fills.
	for g, topics := range rates.ConsumeRates(cl.Config.ID) {
		if consumed[g] == nil {
			consumed[g] = map[string]map[int32]float64{}
		}
		for t, ps := range topics {
			consumed[g][t] = ps
		}
	}

	snap := &Snapshot{At: now.UnixMilli(), Nodes: []Node{}, Links: []Link{}}
	brokers := map[int32]*Node{}
	for _, b := range md.Brokers {
		n := &Node{ID: nodeID(KindBroker, fmt.Sprint(b.NodeID)), Kind: KindBroker, Label: fmt.Sprintf("broker %d", b.NodeID),
			Controller: b.NodeID == controller, Host: fmt.Sprintf("%s:%d", b.Host, b.Port)}
		brokers[b.NodeID] = n
	}

	for _, t := range md.Topics.Sorted() {
		if t.Err != nil {
			continue
		}
		tn := Node{ID: nodeID(KindTopic, t.Topic), Kind: KindTopic, Label: t.Topic, Partitions: len(t.Partitions),
			Internal: t.IsInternal || strings.HasPrefix(t.Topic, "__") || t.Topic == "_schemas"}
		byLeader := map[int32]*Link{}
		for _, p := range t.Partitions {
			r := in[t.Topic][p.Partition]
			tn.Rate += r
			b, ok := brokers[p.Leader]
			if !ok {
				continue
			}
			b.Leaders++
			b.Rate += r
			l := byLeader[p.Leader]
			if l == nil {
				l = &Link{Source: tn.ID, Target: b.ID, Kind: LinkLeader}
				byLeader[p.Leader] = l
			}
			l.Partitions++
			l.Rate += r
		}
		snap.Nodes = append(snap.Nodes, tn)
		for _, l := range byLeader {
			snap.Links = append(snap.Links, *l)
		}
	}
	for _, b := range brokers {
		b.Rate = round2(b.Rate)
		snap.Nodes = append(snap.Nodes, *b)
	}

	for _, l := range lags.Sorted() {
		if l.State == "Dead" {
			continue
		}
		gn := Node{ID: nodeID(KindGroup, l.Group), Kind: KindGroup, Label: l.Group, State: l.State, Members: len(l.Members)}
		for topic, ps := range l.Lag {
			for p, pl := range ps {
				if pl.Commit.At >= 0 && pl.Lag > 0 {
					gn.Lag += pl.Lag
				}
				gn.Rate += consumed[l.Group][topic][p]
			}
		}
		gn.Rate = round2(gn.Rate)
		snap.Nodes = append(snap.Nodes, gn)
		// Tie the group to its topics so groups without members stay next to them.
		for topic, ps := range l.Lag {
			if _, ok := md.Topics[topic]; !ok {
				continue
			}
			link := Link{Source: gn.ID, Target: nodeID(KindTopic, topic), Kind: LinkOffsets, Partitions: len(ps)}
			for p := range ps {
				link.Rate += consumed[l.Group][topic][p]
			}
			snap.Links = append(snap.Links, link)
		}

		for _, m := range l.Members {
			mn := Node{ID: nodeID(KindMember, l.Group+"/"+m.MemberID), Kind: KindMember, Group: l.Group,
				ClientID: m.ClientID, Host: strings.TrimPrefix(m.ClientHost, "/"),
				Label: fmt.Sprintf("%s@%s", m.ClientID, strings.TrimPrefix(m.ClientHost, "/"))}
			if c, ok := m.Assigned.AsConsumer(); ok {
				for _, t := range c.Topics {
					link := Link{Source: mn.ID, Target: nodeID(KindTopic, t.Topic), Kind: LinkConsumes, Partitions: len(t.Partitions)}
					for _, p := range t.Partitions {
						r := consumed[l.Group][t.Topic][p]
						link.Rate += r
						mn.Rate += r
						if pl, ok := l.Lag[t.Topic][p]; ok && pl.Commit.At >= 0 && pl.Lag > 0 {
							mn.Lag += pl.Lag
						}
					}
					link.Rate = round2(link.Rate)
					mn.Partitions += len(t.Partitions)
					snap.Links = append(snap.Links, link)
				}
			}
			mn.Rate = round2(mn.Rate)
			snap.Nodes = append(snap.Nodes, mn)
			snap.Links = append(snap.Links, Link{Source: gn.ID, Target: mn.ID, Kind: LinkMember, Partitions: mn.Partitions, Rate: mn.Rate})
		}
	}
	for i := range snap.Nodes {
		snap.Nodes[i].Rate = round2(snap.Nodes[i].Rate)
	}
	for i := range snap.Links {
		snap.Links[i].Rate = round2(snap.Links[i].Rate)
	}
	sort.Slice(snap.Nodes, func(i, j int) bool { return snap.Nodes[i].ID < snap.Nodes[j].ID })
	sort.Slice(snap.Links, func(i, j int) bool {
		a, b := snap.Links[i], snap.Links[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.Target < b.Target
	})
	return snap, nil
}

func partial(err error) bool {
	var se *kadm.ShardErrors
	return errors.As(err, &se) && !se.AllFailed
}

func round2(f float64) float64 { return float64(int64(f*100+0.5)) / 100 }
