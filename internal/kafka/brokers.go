// Package kafka contains domain services over the Kafka admin/client APIs.
// Nothing in here knows about HTTP.
package kafka

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

type Broker struct {
	ID           int32  `json:"id"`
	Host         string `json:"host"`
	Port         int32  `json:"port"`
	Rack         string `json:"rack,omitempty"`
	IsController bool   `json:"isController"`
	LeaderCount  int    `json:"leaderCount"`
	ReplicaCount int    `json:"replicaCount"`
	SizeBytes    int64  `json:"sizeBytes"`
	// DiskTotalBytes/DiskUsableBytes are -1 when unreported.
	DiskTotalBytes  int64 `json:"diskTotalBytes"`
	DiskUsableBytes int64 `json:"diskUsableBytes"`
}

type Overview struct {
	ClusterID          string   `json:"clusterId"`
	ControllerID       int32    `json:"controllerId"`
	Version            string   `json:"version"`
	Brokers            []Broker `json:"brokers"`
	TopicCount         int      `json:"topicCount"`
	InternalTopicCount int      `json:"internalTopicCount"`
	PartitionCount     int      `json:"partitionCount"`
	URPCount           int      `json:"underReplicatedPartitions"`
	OfflineCount       int      `json:"offlinePartitions"`
	TotalSizeBytes     int64    `json:"totalSizeBytes"`
}

// GetOverview builds the cluster summary including per-broker stats.
func GetOverview(ctx context.Context, cl *kgo.Client, adm *kadm.Client) (*Overview, error) {
	md, err := adm.Metadata(ctx)
	if err != nil {
		return nil, err
	}
	// In KRaft, Metadata returns a random broker as "controller" (KIP-590), so ask
	// the metadata quorum for the real active controller and fall back if unsupported.
	controller := md.Controller
	if id, ok := quorumLeader(ctx, cl); ok {
		controller = id
	}
	md.Controller = controller
	o := &Overview{ClusterID: md.Cluster, ControllerID: controller, Version: guessVersion(ctx, adm)}

	brokers := map[int32]*Broker{}
	for _, b := range md.Brokers {
		br := &Broker{ID: b.NodeID, Host: b.Host, Port: b.Port, IsController: b.NodeID == md.Controller,
			DiskTotalBytes: -1, DiskUsableBytes: -1}
		if b.Rack != nil {
			br.Rack = *b.Rack
		}
		brokers[b.NodeID] = br
	}
	for _, t := range md.Topics {
		if t.Err != nil {
			continue
		}
		if t.IsInternal {
			o.InternalTopicCount++
		}
		o.TopicCount++
		for _, p := range t.Partitions {
			o.PartitionCount++
			if p.Leader < 0 {
				o.OfflineCount++
			}
			if len(p.ISR) < len(p.Replicas) {
				o.URPCount++
			}
			if b, ok := brokers[p.Leader]; ok {
				b.LeaderCount++
			}
			for _, r := range p.Replicas {
				if b, ok := brokers[r]; ok {
					b.ReplicaCount++
				}
			}
		}
	}

	if dirs, err := adm.DescribeAllLogDirs(ctx, nil); err == nil || isPartial(err) {
		dirs.Each(func(d kadm.DescribedLogDir) {
			b, ok := brokers[d.Broker]
			if !ok || d.Err != nil {
				return
			}
			size := d.Size()
			b.SizeBytes += size
			o.TotalSizeBytes += size
			if d.TotalBytes >= 0 {
				b.DiskTotalBytes = max(b.DiskTotalBytes, 0) + d.TotalBytes
				b.DiskUsableBytes = max(b.DiskUsableBytes, 0) + d.UsableBytes
			}
		})
	}

	for _, b := range brokers {
		o.Brokers = append(o.Brokers, *b)
	}
	sort.Slice(o.Brokers, func(i, j int) bool { return o.Brokers[i].ID < o.Brokers[j].ID })
	return o, nil
}

// GetBrokerConfigs returns the configs of one broker.
func GetBrokerConfigs(ctx context.Context, adm *kadm.Client, id int32) ([]ConfigEntry, error) {
	rcs, err := adm.DescribeBrokerConfigs(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(rcs) == 0 {
		return nil, nil
	}
	if rcs[0].Err != nil {
		return nil, rcs[0].Err
	}
	return toConfigEntries(rcs[0].Configs), nil
}

func guessVersion(ctx context.Context, adm *kadm.Client) string {
	vs, err := adm.ApiVersions(ctx)
	if err != nil {
		return ""
	}
	for _, v := range vs.Sorted() {
		if v.Err == nil {
			g := v.VersionGuess()
			// kversion reports e.g. "v4.1" or "at least v4.1"; keep it short.
			return strings.TrimPrefix(g, "at least ")
		}
	}
	return ""
}

// ActiveController returns the active controller: the KRaft quorum leader, or
// the Metadata controller on clusters without DescribeQuorum (ZooKeeper mode).
func ActiveController(ctx context.Context, cl *kgo.Client, fallback int32) int32 {
	if id, ok := quorumLeader(ctx, cl); ok {
		return id
	}
	return fallback
}

// quorumLeader returns the active KRaft controller via DescribeQuorum (KIP-595/836).
func quorumLeader(ctx context.Context, cl *kgo.Client) (int32, bool) {
	req := kmsg.NewPtrDescribeQuorumRequest()
	t := kmsg.NewDescribeQuorumRequestTopic()
	t.Topic = "__cluster_metadata"
	p := kmsg.NewDescribeQuorumRequestTopicPartition()
	p.Partition = 0
	t.Partitions = append(t.Partitions, p)
	req.Topics = append(req.Topics, t)
	resp, err := req.RequestWith(ctx, cl)
	if err != nil || resp.ErrorCode != 0 || len(resp.Topics) == 0 || len(resp.Topics[0].Partitions) == 0 {
		return -1, false
	}
	part := resp.Topics[0].Partitions[0]
	if part.ErrorCode != 0 || part.LeaderID < 0 {
		return -1, false
	}
	return part.LeaderID, true
}

func isPartial(err error) bool {
	var se *kadm.ShardErrors
	return errors.As(err, &se) && !se.AllFailed
}
