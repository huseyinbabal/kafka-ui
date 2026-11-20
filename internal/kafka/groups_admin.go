package kafka

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"

	"github.com/twmb/franz-go/pkg/kadm"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
)

type ResetStrategy string

const (
	ResetEarliest  ResetStrategy = "earliest"
	ResetLatest    ResetStrategy = "latest"
	ResetOffset    ResetStrategy = "offset"
	ResetTimestamp ResetStrategy = "timestamp"
	ResetShift     ResetStrategy = "shift"
)

type ResetRequest struct {
	Topic      string        `json:"topic"`
	Partitions []int32       `json:"partitions"` // empty = all
	Strategy   ResetStrategy `json:"strategy"`
	// Value is the offset (offset), unix millis (timestamp) or delta (shift).
	Value  int64 `json:"value"`
	DryRun bool  `json:"dryRun"`
}

type ResetPlanItem struct {
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	// Current is -1 when the group has no commit for the partition.
	Current int64  `json:"current"`
	Target  int64  `json:"target"`
	Error   string `json:"error,omitempty"`
}

type ResetResult struct {
	Applied bool            `json:"applied"`
	Items   []ResetPlanItem `json:"items"`
}

// resettable reports whether offsets may be changed: only when no consumer is
// active ("Empty"), or the group does not exist yet ("Dead").
func resettable(state string) bool { return state == "Empty" || state == "Dead" }

func groupState(ctx context.Context, adm *kadm.Client, group string) (string, error) {
	ds, err := adm.DescribeGroups(ctx, group)
	if err != nil {
		return "", err
	}
	d, ok := ds[group]
	if !ok {
		return "Dead", nil
	}
	if d.Err != nil {
		return "", d.Err
	}
	return d.State, nil
}

func errGroupActive(state string) error {
	return &apierr.Error{Status: http.StatusConflict, Code: "GROUP_NOT_EMPTY",
		Message: fmt.Sprintf("group is %s; stop all its consumers before changing offsets", state)}
}

// ResetOffsets computes (and unless DryRun, commits) new offsets for one topic.
func ResetOffsets(ctx context.Context, adm *kadm.Client, group string, req ResetRequest) (*ResetResult, error) {
	if req.Topic == "" {
		return nil, apierr.BadRequest("topic is required", nil)
	}
	state, err := groupState(ctx, adm, group)
	if err != nil {
		return nil, err
	}
	if !resettable(state) {
		return nil, errGroupActive(state)
	}
	bounds, err := partitionBounds(ctx, adm, req.Topic, req.Partitions)
	if err != nil {
		return nil, err
	}
	commits, err := adm.FetchOffsets(ctx, group)
	if err != nil {
		return nil, err
	}

	var byTime kadm.ListedOffsets
	switch req.Strategy {
	case ResetEarliest, ResetLatest, ResetShift:
	case ResetOffset:
		if req.Value < 0 {
			return nil, apierr.BadRequest("offset must be >= 0", nil)
		}
	case ResetTimestamp:
		if byTime, err = adm.ListOffsetsAfterMilli(ctx, req.Value, req.Topic); err != nil {
			return nil, err
		}
	default:
		return nil, apierr.BadRequest("strategy must be earliest, latest, offset, timestamp or shift", nil)
	}

	res := &ResetResult{Items: []ResetPlanItem{}}
	toCommit := kadm.Offsets{}
	for _, p := range sortedPartitions(bounds) {
		b := bounds[p]
		item := ResetPlanItem{Topic: req.Topic, Partition: p, Current: -1}
		if c, ok := commits.Lookup(req.Topic, p); ok && c.Err == nil {
			item.Current = c.At
		}
		clamp := func(o int64) int64 { return min(max(o, b.from), b.to) }
		switch req.Strategy {
		case ResetEarliest:
			item.Target = b.from
		case ResetLatest:
			item.Target = b.to
		case ResetOffset:
			item.Target = clamp(req.Value)
		case ResetTimestamp:
			item.Target = b.to
			if o, ok := byTime.Lookup(req.Topic, p); ok && o.Err == nil && o.Offset >= 0 {
				item.Target = clamp(o.Offset)
			}
		case ResetShift:
			if item.Current < 0 {
				item.Target, item.Error = -1, "no committed offset to shift from"
				res.Items = append(res.Items, item)
				continue
			}
			item.Target = clamp(item.Current + req.Value)
		}
		res.Items = append(res.Items, item)
		toCommit.Add(kadm.Offset{Topic: req.Topic, Partition: p, At: item.Target, LeaderEpoch: -1})
	}
	if req.DryRun || len(toCommit) == 0 {
		return res, nil
	}

	// Re-check right before committing to narrow the race with a consumer joining.
	if state, err = groupState(ctx, adm, group); err != nil {
		return nil, err
	}
	if !resettable(state) {
		return nil, errGroupActive(state)
	}
	resp, err := adm.CommitOffsets(ctx, group, toCommit)
	if err != nil {
		return nil, err
	}
	for i := range res.Items {
		it := &res.Items[i]
		if r, ok := resp.Lookup(it.Topic, it.Partition); ok && r.Err != nil {
			it.Error = r.Err.Error()
		}
	}
	res.Applied = true
	return res, nil
}

// DeleteGroup deletes an inactive group and its committed offsets.
func DeleteGroup(ctx context.Context, adm *kadm.Client, group string) error {
	state, err := groupState(ctx, adm, group)
	if err != nil {
		return err
	}
	if state == "Dead" {
		return apierr.NotFound("GROUP_NOT_FOUND", "consumer group not found")
	}
	if state != "Empty" {
		return &apierr.Error{Status: http.StatusConflict, Code: "GROUP_NOT_EMPTY",
			Message: fmt.Sprintf("group is %s; only groups without active members can be deleted", state)}
	}
	resp, err := adm.DeleteGroup(ctx, group)
	if err != nil {
		return err
	}
	return withMessage(resp.Err, resp.ErrMessage)
}

func sortedPartitions(m map[int32]span) []int32 { return slices.Sorted(maps.Keys(m)) }
