package config

import (
	"strings"
	"testing"
	"time"
)

const validYAML = `
auth:
  users:
    - username: admin
      passwordHash: "$2a$10$abcdefghijklmnopqrstuv"
      role: admin
clusters:
  - id: orders
    bootstrap: ["localhost:9092"]
`

func TestParseDefaults(t *testing.T) {
	c, err := Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Server.Addr != ":8080" {
		t.Errorf("addr = %q", c.Server.Addr)
	}
	if c.Auth.SessionIdleTimeout != 12*time.Hour {
		t.Errorf("idle timeout = %v", c.Auth.SessionIdleTimeout)
	}
	if c.Clusters[0].Name != "orders" {
		t.Errorf("name default = %q", c.Clusters[0].Name)
	}
	if !c.Server.CookiesSecure() {
		t.Error("secure cookies should default to true")
	}
}

func TestValidateErrors(t *testing.T) {
	tests := []struct {
		name, yaml, want string
	}{
		{"no users", "clusters: [{id: a, bootstrap: [x]}]", "at least one user"},
		{"bad hash", `
auth: {users: [{username: a, passwordHash: plain, role: admin}]}
clusters: [{id: a, bootstrap: [x]}]`, "bcrypt"},
		{"bad role", `
auth: {users: [{username: a, passwordHash: "$2a$x", role: root}]}
clusters: [{id: a, bootstrap: [x]}]`, "role must be"},
		{"bad cluster id", `
auth: {users: [{username: a, passwordHash: "$2a$x", role: admin}]}
clusters: [{id: "Bad ID", bootstrap: [x]}]`, "must match"},
		{"dup cluster", `
auth: {users: [{username: a, passwordHash: "$2a$x", role: admin}]}
clusters: [{id: a, bootstrap: [x]}, {id: a, bootstrap: [y]}]`, "duplicate id"},
		{"bad sasl", `
auth: {users: [{username: a, passwordHash: "$2a$x", role: admin}]}
clusters: [{id: a, bootstrap: [x], sasl: {mechanism: GSSAPI}}]`, "unsupported sasl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestSecret(t *testing.T) {
	t.Setenv("KUI_TEST_SECRET", "s3cret")
	v, err := Secret("${KUI_TEST_SECRET}").Value()
	if err != nil || v != "s3cret" {
		t.Fatalf("got %q, %v", v, err)
	}
	if _, err := Secret("${KUI_MISSING_XYZ}").Value(); err == nil {
		t.Fatal("expected error for missing env")
	}
	if Secret("x").String() != "[redacted]" {
		t.Fatal("secret must not print")
	}
}

// The documented example must stay loadable.
func TestExampleConfigLoads(t *testing.T) {
	t.Setenv("SECURE_KAFKA_PASSWORD", "x")
	cfg, err := Load("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Clusters) != 3 || cfg.Clusters[2].SASL == nil || cfg.Clusters[2].TLS == nil || cfg.Collector.LiveInterval == 0 {
		t.Fatalf("unexpected example config: %+v", cfg)
	}
}
