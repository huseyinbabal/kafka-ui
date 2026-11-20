// Package auth implements UI users, password checks and server-side sessions.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/huseyinbabal/kafka-ui/internal/config"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

// Principal is an authenticated user.
type Principal struct {
	Username string      `json:"username"`
	Role     config.Role `json:"role"`
}

func (p Principal) CanWrite() bool { return p.Role == config.RoleAdmin }

// dummyHash is compared against when the user does not exist, to keep timing uniform.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

type Service struct {
	db          *sql.DB
	users       map[string]config.UserConfig
	idleTimeout time.Duration
	now         func() time.Time
}

func NewService(db *sql.DB, cfg config.AuthConfig) *Service {
	users := make(map[string]config.UserConfig, len(cfg.Users))
	for _, u := range cfg.Users {
		users[u.Username] = u
	}
	return &Service{db: db, users: users, idleTimeout: cfg.SessionIdleTimeout, now: time.Now}
}

// Login verifies credentials and creates a session, returning the raw token.
func (s *Service) Login(ctx context.Context, username, password string) (string, Principal, error) {
	u, ok := s.users[username]
	hash := dummyHash
	if ok {
		hash = []byte(u.PasswordHash)
	}
	if err := bcrypt.CompareHashAndPassword(hash, []byte(password)); err != nil || !ok {
		return "", Principal{}, ErrInvalidCredentials
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", Principal{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := s.now().Unix()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions(token_hash, username, role, created_at, last_seen) VALUES (?, ?, ?, ?, ?)`,
		hashToken(token), u.Username, string(u.Role), now, now)
	if err != nil {
		return "", Principal{}, fmt.Errorf("store session: %w", err)
	}
	return token, Principal{Username: u.Username, Role: u.Role}, nil
}

// Authenticate resolves a session token, sliding its idle expiry.
// The role is re-read from config so role changes apply to existing sessions.
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrInvalidCredentials
	}
	h := hashToken(token)
	var username string
	var lastSeen int64
	err := s.db.QueryRowContext(ctx, `SELECT username, last_seen FROM sessions WHERE token_hash = ?`, h).Scan(&username, &lastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, ErrInvalidCredentials
	}
	if err != nil {
		return Principal{}, err
	}
	now := s.now()
	u, ok := s.users[username]
	if !ok || now.Sub(time.Unix(lastSeen, 0)) > s.idleTimeout {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, h)
		return Principal{}, ErrInvalidCredentials
	}
	// Only touch last_seen at most once a minute to limit writes.
	if now.Unix()-lastSeen > 60 {
		_, _ = s.db.ExecContext(ctx, `UPDATE sessions SET last_seen = ? WHERE token_hash = ?`, now.Unix(), h)
	}
	return Principal{Username: u.Username, Role: u.Role}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	return err
}

// PurgeExpired deletes idle sessions.
func (s *Service) PurgeExpired(ctx context.Context) error {
	cutoff := s.now().Add(-s.idleTimeout).Unix()
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE last_seen < ?`, cutoff)
	return err
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// HashPassword returns a bcrypt hash for config files.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}
