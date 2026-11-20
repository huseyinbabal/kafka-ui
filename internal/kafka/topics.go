package kafka

import (
	"context"
	"errors"
	"sort"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
)

type TopicSummary struct {
	Name              string `json:"name"`
	Internal          bool   `json:"internal"`
	Partitions        int    `json:"partitions"`
	ReplicationFactor int    `json:"replicationFactor"`
	URPCount          int    `json:"underReplicatedPartitions"`
	MessageCount      int64  `json:"messageCount"`
	SizeBytes         int64  `json:"sizeBytes"`
}

type Partition struct {
	ID              int32   `json:"id"`
	Leader          int32   `json:"leader"`
	Replicas        []int32 `json:"replicas"`
	ISR             []int32 `json:"isr"`
	OfflineReplicas []int32 `json:"offlineReplicas"`
	StartOffset     int64   `json:"startOffset"`
	EndOffset       int64   `json:"endOffset"`
	SizeBytes       int64   `json:"sizeBytes"`
}

type TopicDetail struct {
	TopicSummary
	ID              string        `json:"id"`
	PartitionDetail []Partition   `json:"partitionDetails"`
	Configs         []ConfigEntry `json:"configs"`
}

// topicData is everything needed to render topic summaries/details.
type topicData struct {
	md     kadm.Metadata
	starts kadm.ListedOffsets
	ends   kadm.ListedOffsets
	sizes  map[sizeKey]int64
}

type sizeKey struct {
	topic     string
	partition int32
	broker    int32
}

func loadTopicData(ctx context.Context, adm *kadm.Client, topics ...string) (*topicData, error) {
	md, err := adm.Metadata(ctx, topics...)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(md.Topics))
	for n, t := range md.Topics {
		if t.Err == nil {
			names = append(names, n)
		}
	}
	d := &topicData{md: md, sizes: map[sizeKey]int64{}}
	if len(names) == 0 {
		return d, nil
	}
	// Offsets/sizes are best-effort; a partial failure should still render the list.
	d.starts, err = adm.ListStartOffsets(ctx, names...)
	if err != nil && !isPartial(err) {
		return nil, err
	}
	d.ends, err = adm.ListEndOffsets(ctx, names...)
	if err != nil && !isPartial(err) {
		return nil, err
	}
	set := kadm.TopicsSet{}
	for _, n := range names {
		set.Add(n, md.Topics[n].Partitions.Numbers()...)
	}
	if dirs, err := adm.DescribeAllLogDirs(ctx, set); err == nil || isPartial(err) {
		dirs.Each(func(dir kadm.DescribedLogDir) {
			dir.Topics.Each(func(p kadm.DescribedLogDirPartition) {
				d.sizes[sizeKey{p.Topic, p.Partition, p.Broker}] += p.Size
			})
		})
	}
	return d, nil
}

func (d *topicData) offsets(t string, p int32) (start, end int64) {
	start, end = -1, -1
	if o, ok := d.starts.Lookup(t, p); ok && o.Err == nil {
		start = o.Offset
	}
	if o, ok := d.ends.Lookup(t, p); ok && o.Err == nil {
		end = o.Offset
	}
	return start, end
}

func (d *topicData) summary(t kadm.TopicDetail) TopicSummary {
	s := TopicSummary{
		Name:              t.Topic,
		Internal:          t.IsInternal,
		Partitions:        len(t.Partitions),
		ReplicationFactor: t.Partitions.NumReplicas(),
	}
	for _, p := range t.Partitions {
		if len(p.ISR) < len(p.Replicas) {
			s.URPCount++
		}
		if start, end := d.offsets(t.Topic, p.Partition); start >= 0 && end >= start {
			s.MessageCount += end - start
		}
		s.SizeBytes += d.sizes[sizeKey{t.Topic, p.Partition, p.Leader}]
	}
	return s
}

// ListTopics returns summaries of every topic (internal included; the UI filters).
func ListTopics(ctx context.Context, adm *kadm.Client) ([]TopicSummary, error) {
	d, err := loadTopicData(ctx, adm)
	if err != nil {
		return nil, err
	}
	out := make([]TopicSummary, 0, len(d.md.Topics))
	for _, t := range d.md.Topics {
		if t.Err != nil {
			continue
		}
		out = append(out, d.summary(t))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetTopic returns a single topic with partitions and configs.
func GetTopic(ctx context.Context, adm *kadm.Client, name string) (*TopicDetail, error) {
	d, err := loadTopicData(ctx, adm, name)
	if err != nil {
		return nil, err
	}
	t, ok := d.md.Topics[name]
	if !ok || errors.Is(t.Err, kerr.UnknownTopicOrPartition) {
		return nil, apierr.NotFound("TOPIC_NOT_FOUND", "topic not found")
	}
	if t.Err != nil {
		return nil, t.Err
	}
	out := &TopicDetail{TopicSummary: d.summary(t), ID: t.ID.String()}
	for _, p := range t.Partitions.Sorted() {
		start, end := d.offsets(name, p.Partition)
		out.PartitionDetail = append(out.PartitionDetail, Partition{
			ID: p.Partition, Leader: p.Leader,
			Replicas: nonNil(p.Replicas), ISR: nonNil(p.ISR), OfflineReplicas: nonNil(p.OfflineReplicas),
			StartOffset: start, EndOffset: end,
			SizeBytes: d.sizes[sizeKey{name, p.Partition, p.Leader}],
		})
	}
	rcs, err := adm.DescribeTopicConfigs(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(rcs) > 0 && rcs[0].Err == nil {
		out.Configs = toConfigEntries(rcs[0].Configs)
	}
	return out, nil
}

func nonNil(s []int32) []int32 {
	if s == nil {
		return []int32{}
	}
	return s
}
