package kafka

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/twmb/franz-go/pkg/kadm"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
)

var topicNameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,249}$`)

// ValidateTopicName applies Kafka's topic name rules.
func ValidateTopicName(name string) error {
	if !topicNameRe.MatchString(name) || name == "." || name == ".." {
		return apierr.BadRequest("topic name must be 1-249 characters of [a-zA-Z0-9._-] and not '.' or '..'", nil)
	}
	return nil
}

// isProtected reports internal topics that the UI never mutates.
func isProtected(topic string) bool { return strings.HasPrefix(topic, "__") }

func errProtected(topic string) error {
	return apierr.New(403, "TOPIC_PROTECTED", fmt.Sprintf("%s is an internal topic and cannot be changed here", topic))
}

type CreateTopicRequest struct {
	Name              string             `json:"name"`
	Partitions        int32              `json:"partitions"`
	ReplicationFactor int16              `json:"replicationFactor"`
	Configs           map[string]*string `json:"configs"`
	// ValidateOnly asks the broker to check the request without creating it.
	ValidateOnly bool `json:"validateOnly"`
}

// CreateTopic creates a topic. -1 partitions/RF use the broker defaults.
func CreateTopic(ctx context.Context, adm *kadm.Client, req CreateTopicRequest) (*TopicSummary, error) {
	if err := ValidateTopicName(req.Name); err != nil {
		return nil, err
	}
	if isProtected(req.Name) {
		return nil, errProtected(req.Name)
	}
	if req.Partitions == 0 {
		req.Partitions = -1
	}
	if req.ReplicationFactor == 0 {
		req.ReplicationFactor = -1
	}
	if req.Partitions < -1 || req.ReplicationFactor < -1 || req.Partitions > 10_000 {
		return nil, apierr.BadRequest("partitions and replicationFactor must be positive (or -1 for the broker default)", nil)
	}
	var (
		resp kadm.CreateTopicResponse
		err  error
	)
	if req.ValidateOnly {
		var rs kadm.CreateTopicResponses
		rs, err = adm.ValidateCreateTopics(ctx, req.Partitions, req.ReplicationFactor, req.Configs, req.Name)
		if err == nil {
			resp = rs[req.Name]
		}
	} else {
		resp, err = adm.CreateTopic(ctx, req.Partitions, req.ReplicationFactor, req.Configs, req.Name)
	}
	if err != nil {
		return nil, err
	}
	if resp.Err != nil {
		return nil, withMessage(resp.Err, resp.ErrMessage)
	}
	return &TopicSummary{Name: req.Name, Partitions: int(resp.NumPartitions), ReplicationFactor: int(resp.ReplicationFactor)}, nil
}

func DeleteTopic(ctx context.Context, adm *kadm.Client, topic string) error {
	if isProtected(topic) {
		return errProtected(topic)
	}
	resp, err := adm.DeleteTopic(ctx, topic)
	if err != nil {
		return err
	}
	return withMessage(resp.Err, resp.ErrMessage)
}

// ConfigChange sets a topic config, or resets it to the default when Value is nil.
type ConfigChange struct {
	Name  string  `json:"name"`
	Value *string `json:"value"`
}

func AlterTopicConfigs(ctx context.Context, adm *kadm.Client, topic string, changes []ConfigChange) error {
	if isProtected(topic) {
		return errProtected(topic)
	}
	if len(changes) == 0 {
		return apierr.BadRequest("no config changes given", nil)
	}
	alters := make([]kadm.AlterConfig, 0, len(changes))
	for _, c := range changes {
		if c.Name == "" {
			return apierr.BadRequest("config name must not be empty", nil)
		}
		op := kadm.SetConfig
		if c.Value == nil {
			op = kadm.DeleteConfig
		}
		alters = append(alters, kadm.AlterConfig{Op: op, Name: c.Name, Value: c.Value})
	}
	rs, err := adm.AlterTopicConfigs(ctx, alters, topic)
	if err != nil {
		return err
	}
	for _, r := range rs {
		if r.Err != nil {
			return withMessage(r.Err, r.ErrMessage)
		}
	}
	return nil
}

// SetPartitionCount grows a topic to total partitions (Kafka cannot shrink).
func SetPartitionCount(ctx context.Context, adm *kadm.Client, topic string, total int) error {
	if isProtected(topic) {
		return errProtected(topic)
	}
	if total < 1 || total > 10_000 {
		return apierr.BadRequest("partition count must be between 1 and 10000", nil)
	}
	rs, err := adm.UpdatePartitions(ctx, total, topic)
	if err != nil {
		return err
	}
	if r, ok := rs[topic]; ok && r.Err != nil {
		return withMessage(r.Err, r.ErrMessage)
	}
	return nil
}

type PurgeResult struct {
	Partition    int32  `json:"partition"`
	LowWatermark int64  `json:"lowWatermark"`
	Error        string `json:"error,omitempty"`
}

// PurgeTopic deletes all records (up to the current end offset) in the given
// partitions, or all partitions when none are given.
func PurgeTopic(ctx context.Context, adm *kadm.Client, topic string, partitions []int32) ([]PurgeResult, error) {
	if isProtected(topic) {
		return nil, errProtected(topic)
	}
	bounds, err := partitionBounds(ctx, adm, topic, partitions)
	if err != nil {
		return nil, err
	}
	os := kadm.Offsets{}
	for p, b := range bounds {
		os.Add(kadm.Offset{Topic: topic, Partition: p, At: b.to, LeaderEpoch: -1})
	}
	rs, err := adm.DeleteRecords(ctx, os)
	if err != nil {
		return nil, err
	}
	out := []PurgeResult{}
	rs.Each(func(r kadm.DeleteRecordsResponse) {
		pr := PurgeResult{Partition: r.Partition, LowWatermark: r.LowWatermark}
		if r.Err != nil {
			pr.Error = r.Err.Error()
		}
		out = append(out, pr)
	})
	slices.SortFunc(out, func(a, b PurgeResult) int { return int(a.Partition - b.Partition) })
	return out, nil
}

// withMessage keeps the broker's extra error message (e.g. why a config is invalid).
func withMessage(err error, msg string) error {
	if err == nil {
		return nil
	}
	if msg == "" {
		return err
	}
	return &brokerError{err: err, msg: msg}
}

type brokerError struct {
	err error
	msg string
}

func (e *brokerError) Error() string { return e.err.Error() + ": " + e.msg }
func (e *brokerError) Unwrap() error { return e.err }

// BrokerMessage returns the broker-provided detail of err, if any.
func BrokerMessage(err error) string {
	var be *brokerError
	if errors.As(err, &be) {
		return be.msg
	}
	return ""
}
