//go:build integration

package itest

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/huseyinbabal/kafka-ui/internal/api"
	"github.com/huseyinbabal/kafka-ui/internal/auth"
	"github.com/huseyinbabal/kafka-ui/internal/cluster"
	"github.com/huseyinbabal/kafka-ui/internal/config"
	"github.com/huseyinbabal/kafka-ui/internal/storage"
)

// TestHTTPEndToEnd drives the real HTTP API against Kafka: login, overview,
// create topic, produce, stream it back over SSE.
func TestHTTPEndToEnd(t *testing.T) {
	ctx := ctxT(t)
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	h, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	insecure := false
	cfg := &config.Config{
		Server:   config.ServerConfig{SecureCookies: &insecure},
		Auth:     config.AuthConfig{SessionIdleTimeout: time.Hour, Users: []config.UserConfig{{Username: "admin", PasswordHash: string(h), Role: config.RoleAdmin}}},
		Clusters: []config.ClusterConfig{{ID: "it", Name: "it", Bootstrap: env.bootstrap, SchemaRegistry: &config.SchemaRegistryConfig{URL: env.srURL}}},
	}
	reg, err := cluster.NewRegistry(cfg.Clusters, quiet)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()
	srv := httptest.NewServer(api.NewServer(cfg, auth.NewService(db, cfg.Auth), reg, fstest.MapFS{"index.html": {Data: []byte("spa")}}, quiet).Handler())
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	cl := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	do := func(method, path, body string) (int, string) {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, method, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "kafka-ui")
		resp, err := cl.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	if code, body := do("POST", "/api/v1/auth/login", `{"username":"admin","password":"pw"}`); code != 200 {
		t.Fatalf("login %d %s", code, body)
	}
	if code, body := do("GET", "/api/v1/clusters/it/overview", ""); code != 200 || !strings.Contains(body, `"brokers"`) {
		t.Fatalf("overview %d %s", code, body)
	}
	topic := uniq("it-http")
	if code, body := do("POST", "/api/v1/clusters/it/topics", `{"name":"`+topic+`","partitions":1,"replicationFactor":1}`); code != 200 {
		t.Fatalf("create %d %s", code, body)
	}
	if code, body := do("POST", "/api/v1/clusters/it/topics/"+topic+"/messages", `{"key":"k","value":"hello http"}`); code != 201 {
		t.Fatalf("produce %d %s", code, body)
	}
	if code, body := do("GET", "/api/v1/clusters/it/schemas/subjects", ""); code != 200 {
		t.Fatalf("subjects %d %s", code, body)
	}

	sctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(sctx, "GET", srv.URL+"/api/v1/clusters/it/topics/"+topic+"/messages/stream?mode=oldest", nil)
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type %q", resp.Header.Get("Content-Type"))
	}
	var events []string
	var msg struct {
		Value struct{ Text string } `json:"value"`
	}
	sc := bufio.NewScanner(resp.Body)
	event := ""
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
			events = append(events, event)
		case strings.HasPrefix(line, "data: ") && event == "message":
			_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &msg)
		}
		if event == "done" {
			break
		}
	}
	if msg.Value.Text != "hello http" || events[len(events)-1] != "done" {
		t.Fatalf("stream events %v, message %+v", events, msg)
	}
}
