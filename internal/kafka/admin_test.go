package kafka

import (
	"testing"
)

func TestValidateTopicName(t *testing.T) {
	for _, ok := range []string{"orders", "e2e-a.b_c", "A1"} {
		if err := ValidateTopicName(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "..", "has space", "slash/x", string(make([]byte, 250))} {
		if err := ValidateTopicName(bad); err == nil {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestProtectedTopics(t *testing.T) {
	if !isProtected("__consumer_offsets") || isProtected("orders") {
		t.Fatal("only __-prefixed topics are protected")
	}
}

func TestResettable(t *testing.T) {
	for state, want := range map[string]bool{"Empty": true, "Dead": true, "Stable": false, "PreparingRebalance": false, "Assigning": false} {
		if resettable(state) != want {
			t.Errorf("%s: want %v", state, want)
		}
	}
}

func TestACLBuilder(t *testing.T) {
	good := ACL{Principal: "User:alice", ResourceType: "TOPIC", ResourceName: "orders", PatternType: "LITERAL", Operation: "READ", Permission: "ALLOW"}
	b, err := good.builder()
	if err != nil {
		t.Fatal(err)
	}
	if err := b.ValidateCreate(); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := b.ValidateDelete(); err != nil {
		t.Fatalf("delete: %v", err)
	}
	cluster := ACL{Principal: "User:ops", ResourceType: "cluster", PatternType: "literal", Operation: "describe_configs", Permission: "deny"}
	if _, err := cluster.builder(); err != nil {
		t.Fatalf("cluster acl (lowercase enums) should parse: %v", err)
	}
	bads := []ACL{
		{Principal: "alice", ResourceType: "TOPIC", ResourceName: "x", PatternType: "LITERAL", Operation: "READ", Permission: "ALLOW"},
		{Principal: "User:a", ResourceType: "TOPIC", ResourceName: "", PatternType: "LITERAL", Operation: "READ", Permission: "ALLOW"},
		{Principal: "User:a", ResourceType: "TOPIC", ResourceName: "x", PatternType: "ANY", Operation: "READ", Permission: "ALLOW"},
		{Principal: "User:a", ResourceType: "TOPIC", ResourceName: "x", PatternType: "LITERAL", Operation: "ANY", Permission: "ALLOW"},
		{Principal: "User:a", ResourceType: "TOPIC", ResourceName: "x", PatternType: "LITERAL", Operation: "READ", Permission: "MAYBE"},
	}
	for _, a := range bads {
		if _, err := a.builder(); err == nil {
			t.Errorf("%+v should be rejected", a)
		}
	}
}

func TestListAllFilterIsValid(t *testing.T) {
	if err := listAllFilter().ValidateDescribe(); err != nil {
		t.Fatal(err)
	}
}
