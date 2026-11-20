package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/huseyinbabal/kafka-ui/internal/config"
	"github.com/huseyinbabal/kafka-ui/internal/storage"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	h, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	return NewService(db, config.AuthConfig{
		SessionIdleTimeout: time.Hour,
		Users: []config.UserConfig{
			{Username: "alice", PasswordHash: string(h), Role: config.RoleAdmin},
			{Username: "bob", PasswordHash: string(h), Role: config.RoleViewer},
		},
	})
}

func TestLoginAndAuthenticate(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	if _, _, err := s.Login(ctx, "alice", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, _, err := s.Login(ctx, "nobody", "pw"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown user: %v", err)
	}

	tok, p, err := s.Login(ctx, "bob", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if p.CanWrite() {
		t.Error("viewer must not write")
	}
	got, err := s.Authenticate(ctx, tok)
	if err != nil || got.Username != "bob" {
		t.Fatalf("authenticate: %+v %v", got, err)
	}

	if err := s.Logout(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, tok); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("after logout: %v", err)
	}
}

func TestSessionIdleExpiry(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	now := time.Now()
	s.now = func() time.Time { return now }
	tok, _, err := s.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, err := s.Authenticate(ctx, tok); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected expiry, got %v", err)
	}
}

func TestRateLimiter(t *testing.T) {
	r := NewRateLimiter(2, time.Minute)
	now := time.Now()
	r.now = func() time.Time { return now }
	for i := range 2 {
		if !r.Allow("ip") {
			t.Fatalf("call %d must pass", i+1)
		}
	}
	if r.Allow("ip") {
		t.Fatal("third must be limited")
	}
	if !r.Allow("other") {
		t.Fatal("other key independent")
	}
	r.now = func() time.Time { return now.Add(time.Minute) }
	if !r.Allow("ip") {
		t.Fatal("window reset")
	}
}
