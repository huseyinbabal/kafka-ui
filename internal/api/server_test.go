package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/huseyinbabal/kafka-ui/internal/auth"
	"github.com/huseyinbabal/kafka-ui/internal/cluster"
	"github.com/huseyinbabal/kafka-ui/internal/config"
	"github.com/huseyinbabal/kafka-ui/internal/storage"
)

func newTestServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	h, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	insecure := false
	cfg := &config.Config{
		Server: config.ServerConfig{SecureCookies: &insecure},
		Auth: config.AuthConfig{SessionIdleTimeout: time.Hour, Users: []config.UserConfig{
			{Username: "admin", PasswordHash: string(h), Role: config.RoleAdmin},
			{Username: "viewer", PasswordHash: string(h), Role: config.RoleViewer},
		}},
		Clusters: []config.ClusterConfig{{ID: "dev", Name: "Dev", Bootstrap: []string{"127.0.0.1:1"}, ReadOnly: true}},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg, err := cluster.NewRegistry(cfg.Clusters, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reg.Close)
	static := fstest.MapFS{"index.html": {Data: []byte("<html>spa</html>")}}
	s := NewServer(cfg, auth.NewService(db, cfg.Auth), reg, static, log)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, s
}

func client(t *testing.T) *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func login(t *testing.T, ts *httptest.Server, c *http.Client, user string) {
	t.Helper()
	resp, err := c.Post(ts.URL+"/api/v1/auth/login", "application/json",
		strings.NewReader(`{"username":"`+user+`","password":"pw"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status %d", resp.StatusCode)
	}
}

func TestAuthRequired(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/clusters")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", resp.StatusCode)
	}
}

func TestLoginListClustersAndLogout(t *testing.T) {
	ts, _ := newTestServer(t)
	c := client(t)
	login(t, ts, c, "viewer")

	resp, err := c.Get(ts.URL + "/api/v1/clusters")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"id":"dev"`) || !strings.Contains(string(body), `"readOnly":true`) {
		t.Fatalf("clusters: %d %s", resp.StatusCode, body)
	}

	// Logout without CSRF header is rejected.
	resp, _ = c.Post(ts.URL+"/api/v1/auth/logout", "application/json", nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("csrf: status %d", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/logout", nil)
	req.Header.Set("X-Requested-With", "kafka-ui")
	resp, _ = c.Do(req)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: status %d", resp.StatusCode)
	}
	resp, _ = c.Get(ts.URL + "/api/v1/auth/me")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after logout: status %d", resp.StatusCode)
	}
}

func TestAdminWrapper(t *testing.T) {
	h := admin(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	for role, want := range map[config.Role]int{config.RoleAdmin: http.StatusTeapot, config.RoleViewer: http.StatusForbidden} {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r = r.WithContext(context.WithValue(r.Context(), principalKey, auth.Principal{Username: "x", Role: role}))
		w := httptest.NewRecorder()
		h(w, r)
		if w.Code != want {
			t.Errorf("role %s: got %d want %d", role, w.Code, want)
		}
	}
}

func TestUnknownClusterAndSPAFallback(t *testing.T) {
	ts, _ := newTestServer(t)
	c := client(t)
	login(t, ts, c, "admin")
	resp, _ := c.Get(ts.URL + "/api/v1/clusters/nope/topics")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown cluster: %d", resp.StatusCode)
	}
	resp, _ = c.Get(ts.URL + "/clusters/dev/topics")
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "spa") {
		t.Fatalf("spa fallback: %s", body)
	}
}

func TestLoginRateLimit(t *testing.T) {
	ts, _ := newTestServer(t)
	var last int
	for range 6 {
		resp, _ := http.Post(ts.URL+"/api/v1/auth/login", "application/json", strings.NewReader(`{"username":"admin","password":"bad"}`))
		_ = resp.Body.Close()
		last = resp.StatusCode
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("6th attempt status %d", last)
	}
}

func TestProduceGuards(t *testing.T) {
	ts, _ := newTestServer(t)
	post := func(user string) (int, string) {
		c := client(t)
		login(t, ts, c, user)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/dev/topics/x/messages", strings.NewReader(`{"value":"v"}`))
		req.Header.Set("X-Requested-With", "kafka-ui")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode, string(body)
	}
	if code, _ := post("viewer"); code != http.StatusForbidden {
		t.Fatalf("viewer produce: %d", code)
	}
	// The test cluster is read-only, so even admin is refused before touching Kafka.
	if code, body := post("admin"); code != http.StatusForbidden || !strings.Contains(body, "CLUSTER_READ_ONLY") {
		t.Fatalf("read-only produce: %d %s", code, body)
	}
}

func TestStreamValidation(t *testing.T) {
	ts, _ := newTestServer(t)
	c := client(t)
	login(t, ts, c, "viewer")
	for _, q := range []string{"mode=bogus&limit=x", "regex=true&q=(", "mode=timestamp", "partitions=a", "valueFormat=xml"} {
		resp, err := c.Get(ts.URL + "/api/v1/clusters/dev/topics/x/messages/stream?" + q)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: got %d", q, resp.StatusCode)
		}
	}
}

func TestMutationGuards(t *testing.T) {
	ts, _ := newTestServer(t)
	routes := []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/clusters/dev/topics", `{"name":"x"}`},
		{http.MethodDelete, "/api/v1/clusters/dev/topics/x", ``},
		{http.MethodPatch, "/api/v1/clusters/dev/topics/x/configs", `{"changes":[]}`},
		{http.MethodPut, "/api/v1/clusters/dev/topics/x/partitions", `{"count":3}`},
		{http.MethodPost, "/api/v1/clusters/dev/topics/x/purge", `{}`},
		{http.MethodPost, "/api/v1/clusters/dev/groups/g/reset-offsets", `{"topic":"x","strategy":"earliest","dryRun":true}`},
		{http.MethodDelete, "/api/v1/clusters/dev/groups/g", ``},
		{http.MethodPost, "/api/v1/clusters/dev/acls", `{}`},
		{http.MethodPost, "/api/v1/clusters/dev/acls/delete", `{}`},
	}
	for _, user := range []string{"viewer", "admin"} {
		c := client(t)
		login(t, ts, c, user)
		for _, rt := range routes {
			req, _ := http.NewRequest(rt.method, ts.URL+rt.path, strings.NewReader(rt.body))
			req.Header.Set("X-Requested-With", "kafka-ui")
			resp, err := c.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			// viewer: FORBIDDEN by role; admin: CLUSTER_READ_ONLY (the test cluster is read-only).
			want := map[string]string{"viewer": `"FORBIDDEN"`, "admin": `"CLUSTER_READ_ONLY"`}[user]
			if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(body), want) {
				t.Errorf("%s %s %s: %d %s", user, rt.method, rt.path, resp.StatusCode, body)
			}
		}
	}
}
