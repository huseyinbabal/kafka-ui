//go:build integration

package itest

import (
	"errors"
	"testing"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
	"github.com/huseyinbabal/kafka-ui/internal/kafka"
)

func TestACLs(t *testing.T) {
	c := newClusterFor(t, env.aclBootstrap, false)
	ctx := ctxT(t)
	a := kafka.ACL{Principal: "User:alice", Host: "*", ResourceType: "TOPIC", ResourceName: "orders-",
		PatternType: "PREFIXED", Operation: "READ", Permission: "ALLOW"}
	if err := kafka.CreateACL(ctx, c.Admin, a); err != nil {
		t.Fatal(err)
	}
	if err := kafka.CreateACL(ctx, c.Admin, kafka.ACL{Principal: "User:bob", ResourceType: "CLUSTER",
		PatternType: "LITERAL", Operation: "ALTER", Permission: "DENY"}); err != nil {
		t.Fatal(err)
	}
	acls, err := kafka.ListACLs(ctx, c.Admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(acls) != 2 || acls[1] != a {
		t.Fatalf("listed %+v", acls)
	}
	n, err := kafka.DeleteACL(ctx, c.Admin, a)
	if err != nil || n != 1 {
		t.Fatalf("delete: %d %v", n, err)
	}
	if acls, _ := kafka.ListACLs(ctx, c.Admin); len(acls) != 1 {
		t.Fatalf("after delete: %+v", acls)
	}

	// Without an authorizer, the API reports SECURITY_DISABLED (mapped in the HTTP layer).
	plain := newClusterFor(t, env.bootstrap, false)
	_, err = kafka.ListACLs(ctx, plain.Admin)
	var ae *apierr.Error
	if err == nil || errors.As(err, &ae) {
		t.Fatalf("plain cluster must return the raw Kafka error, got %v", err)
	}
}
