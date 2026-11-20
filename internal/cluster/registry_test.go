package cluster

import (
	"io"
	"log/slog"
	"testing"

	"github.com/huseyinbabal/kafka-ui/internal/config"
)

func TestMisconfiguredClusterStaysOffline(t *testing.T) {
	cfgs := []config.ClusterConfig{
		{ID: "ok", Bootstrap: []string{"127.0.0.1:1"}},
		{ID: "bad", Bootstrap: []string{"127.0.0.1:1"}, SASL: &config.SASLConfig{
			Mechanism: "SCRAM-SHA-512", Username: "u", Password: "${KAFKAUI_TEST_UNSET_PASSWORD}",
		}},
	}
	r, err := NewRegistry(cfgs, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("registry must not fail because of one cluster: %v", err)
	}
	defer r.Close()

	if got := len(r.All()); got != 2 {
		t.Fatalf("both clusters must be listed, got %d", got)
	}
	bad, _ := r.Get("bad")
	if h := bad.Health(); h.Status != StatusOffline || h.LastError == "" {
		t.Fatalf("bad cluster must be offline with a reason: %+v", h)
	}
	if _, err := bad.NewClient(); err == nil {
		t.Fatal("NewClient on a broken cluster must fail")
	}
	ok, _ := r.Get("ok")
	if ok.Client == nil || ok.Admin == nil {
		t.Fatal("healthy cluster must have clients")
	}
}
