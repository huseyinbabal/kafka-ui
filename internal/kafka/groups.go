package kafka

import (
	"context"
	"sort"

	"github.com/twmb/franz-go/pkg/kadm"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
)

type GroupSummary struct {
	ID           string   `json:"id"`
	State        string   `json:"state"`
	ProtocolType string   `json:"protocolType"`
	Protocol     string   `json:"protocol"`
	Coordinator  int32    `json:"coordinator"`
	Members      int      `json:"members"`
	Topics       []string `json:"topics"`
	TotalLag     int64    `json:"totalLag"`
	Error        string   `json:"error,omitempty"`
}

type GroupMember struct {
	MemberID   string           `json:"memberId"`
	InstanceID string           `json:"instanceId,omitempty"`
	ClientID   string           `json:"clientId"`
	ClientHost string           `json:"clientHost"`
	Assigned   []TopicPartitons `json:"assigned"`
}

type TopicPartitons struct {
	Topic      string  `json:"topic"`
	Partitions []int32 `json:"partitions"`
}

type PartitionLag struct {
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	// Committed is -1 when the group has no commit for this partition.
	Committed   int64  `json:"committed"`
	StartOffset int64  `json:"startOffset"`
	EndOffset   int64  `json:"endOffset"`
	Lag         int64  `json:"lag"`
	MemberID    string `json:"memberId,omitempty"`
	ClientID    string `json:"clientId,omitempty"`
	ClientHost  string `json:"clientHost,omitempty"`
	Error       string `json:"error,omitempty"`
}

type GroupDetail struct {
	GroupSummary
	MemberList []GroupMember  `json:"memberList"`
	Partitions []PartitionLag `json:"partitions"`
}

// ListGroups returns all groups with lag totals.
func ListGroups(ctx context.Context, adm *kadm.Client) ([]GroupSummary, error) {
	listed, err := adm.ListGroups(ctx)
	if err != nil && !isPartial(err) {
		return nil, err
	}
	if len(listed) == 0 {
		return []GroupSummary{}, nil
	}
	lags, err := adm.Lag(ctx, listed.Groups()...)
	if err != nil {
		return nil, err
	}
	out := make([]GroupSummary, 0, len(lags))
	for _, l := range lags.Sorted() {
		out = append(out, summarize(l))
	}
	return out, nil
}

// GetGroup returns members and per-partition lag of one group.
func GetGroup(ctx context.Context, adm *kadm.Client, id string) (*GroupDetail, error) {
	lags, err := adm.Lag(ctx, id)
	if err != nil {
		return nil, err
	}
	l, ok := lags[id]
	if !ok || l.State == "Dead" {
		return nil, apierr.NotFound("GROUP_NOT_FOUND", "consumer group not found")
	}
	d := &GroupDetail{GroupSummary: summarize(l), MemberList: []GroupMember{}, Partitions: []PartitionLag{}}
	for _, m := range l.Members {
		gm := GroupMember{MemberID: m.MemberID, ClientID: m.ClientID, ClientHost: m.ClientHost, Assigned: []TopicPartitons{}}
		if m.InstanceID != nil {
			gm.InstanceID = *m.InstanceID
		}
		if c, ok := m.Assigned.AsConsumer(); ok {
			for _, t := range c.Topics {
				ps := append([]int32(nil), t.Partitions...)
				sort.Slice(ps, func(i, j int) bool { return ps[i] < ps[j] })
				gm.Assigned = append(gm.Assigned, TopicPartitons{Topic: t.Topic, Partitions: ps})
			}
		}
		d.MemberList = append(d.MemberList, gm)
	}
	for _, ml := range l.Lag.Sorted() {
		pl := PartitionLag{
			Topic: ml.Topic, Partition: ml.Partition,
			Committed: ml.Commit.At, StartOffset: ml.Start.Offset, EndOffset: ml.End.Offset, Lag: ml.Lag,
		}
		if ml.Member != nil {
			pl.MemberID, pl.ClientID, pl.ClientHost = ml.Member.MemberID, ml.Member.ClientID, ml.Member.ClientHost
		}
		if ml.Err != nil {
			pl.Error = ml.Err.Error()
		}
		d.Partitions = append(d.Partitions, pl)
	}
	return d, nil
}

// GroupsForTopic returns group summaries that consume (or have commits on) a topic,
// with lag restricted to that topic.
func GroupsForTopic(ctx context.Context, adm *kadm.Client, topic string) ([]GroupSummary, error) {
	listed, err := adm.ListGroups(ctx)
	if err != nil && !isPartial(err) {
		return nil, err
	}
	if len(listed) == 0 {
		return []GroupSummary{}, nil
	}
	lags, err := adm.Lag(ctx, listed.Groups()...)
	if err != nil {
		return nil, err
	}
	out := []GroupSummary{}
	for _, l := range lags.Sorted() {
		ps, ok := l.Lag[topic]
		if !ok {
			continue
		}
		s := summarize(l)
		s.TotalLag = sumLag(ps)
		out = append(out, s)
	}
	return out, nil
}

func summarize(l kadm.DescribedGroupLag) GroupSummary {
	s := GroupSummary{
		ID: l.Group, State: l.State, ProtocolType: l.ProtocolType, Protocol: l.Protocol,
		Coordinator: l.Coordinator.NodeID, Members: len(l.Members), Topics: []string{},
	}
	if err := l.Error(); err != nil {
		s.Error = err.Error()
	}
	for t, ps := range l.Lag {
		s.Topics = append(s.Topics, t)
		s.TotalLag += sumLag(ps)
	}
	sort.Strings(s.Topics)
	return s
}

// sumLag totals lag over partitions that have a committed offset, matching
// kafka-consumer-groups.sh (uncommitted partitions show "-" there, not lag).
func sumLag(ps map[int32]kadm.GroupMemberLag) int64 {
	var total int64
	for _, p := range ps {
		if p.Commit.At >= 0 && p.Lag > 0 {
			total += p.Lag
		}
	}
	return total
}
