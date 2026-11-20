package kafka

import (
	"testing"

	"github.com/twmb/franz-go/pkg/kadm"
)

func TestSumLagSkipsUncommitted(t *testing.T) {
	ps := map[int32]kadm.GroupMemberLag{
		0: {Commit: kadm.Offset{At: 2}, Lag: 1},
		1: {Commit: kadm.Offset{At: -1}, Lag: 5}, // no commit: excluded like the Kafka CLI
		2: {Commit: kadm.Offset{At: 7}, Lag: -1}, // error marker
		3: {Commit: kadm.Offset{At: 0}, Lag: 4},
	}
	if got := sumLag(ps); got != 5 {
		t.Fatalf("sumLag = %d, want 5", got)
	}
}
