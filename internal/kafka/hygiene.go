package kafka

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/twmb/franz-go/pkg/kadm"
)

// HygieneIssue is one finding about a topic or group configuration.
type HygieneIssue struct {
	Kind     string `json:"kind"`     // low-rf, minisr-too-high, minisr-weak, under-replicated, empty-group
	Severity string `json:"severity"` // warning | info
	Target   string `json:"target"`   // topic or group name
	Message  string `json:"message"`
}

// Hygiene checks replication settings of every non-internal topic and lists empty groups.
func Hygiene(ctx context.Context, adm *kadm.Client) ([]HygieneIssue, error) {
	md, err := adm.Metadata(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, t := range md.Topics {
		if t.Err == nil && !t.IsInternal && !isProtected(t.Topic) {
			names = append(names, t.Topic)
		}
	}
	minISR := map[string]int{}
	if len(names) > 0 {
		rcs, err := adm.DescribeTopicConfigs(ctx, names...)
		if err != nil && !isPartial(err) {
			return nil, err
		}
		for _, rc := range rcs {
			for _, c := range rc.Configs {
				if c.Key == "min.insync.replicas" && c.Value != nil {
					if n, err := strconv.Atoi(*c.Value); err == nil {
						minISR[rc.Name] = n
					}
				}
			}
		}
	}

	out := []HygieneIssue{}
	for _, name := range names {
		t := md.Topics[name]
		if len(t.Partitions) == 0 {
			continue
		}
		rf := len(t.Partitions.Sorted()[0].Replicas)
		urp := 0
		for _, p := range t.Partitions {
			if len(p.ISR) < len(p.Replicas) {
				urp++
			}
		}
		out = append(out, topicIssues(name, rf, minISR[name], urp)...)
	}

	listed, err := adm.ListGroups(ctx)
	if err != nil && !isPartial(err) {
		return nil, err
	}
	for _, g := range listed.Sorted() {
		if g.State == "Empty" {
			out = append(out, HygieneIssue{Kind: "empty-group", Severity: "info", Target: g.Group,
				Message: "no active members; its committed offsets remain until they expire or the group is deleted"})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Severity == "warning" && out[j].Severity != "warning" })
	return out, nil
}

// topicIssues holds the pure rules so they can be unit tested.
func topicIssues(topic string, rf, minISR, urp int) []HygieneIssue {
	var out []HygieneIssue
	warn := func(kind, msg string) {
		out = append(out, HygieneIssue{Kind: kind, Severity: "warning", Target: topic, Message: msg})
	}
	if rf < 2 {
		warn("low-rf", fmt.Sprintf("replication factor %d: losing one broker loses data", rf))
	}
	switch {
	case minISR > rf:
		warn("minisr-too-high", fmt.Sprintf("min.insync.replicas=%d > RF %d: acks=all producers always fail", minISR, rf))
	case minISR == rf && rf > 1:
		warn("minisr-too-high", fmt.Sprintf("min.insync.replicas=%d = RF: one broker down blocks acks=all producers", minISR))
	case minISR == 1 && rf >= 3:
		out = append(out, HygieneIssue{Kind: "minisr-weak", Severity: "info", Target: topic,
			Message: fmt.Sprintf("min.insync.replicas=1 with RF %d: acks=all writes can be acknowledged by a single replica", rf)})
	}
	if urp > 0 {
		warn("under-replicated", fmt.Sprintf("%d partitions are under-replicated", urp))
	}
	return out
}
