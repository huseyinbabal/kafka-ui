// Package config loads and validates the kafka-ui configuration file.
package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Auth      AuthConfig      `yaml:"auth"`
	Storage   StorageConfig   `yaml:"storage"`
	Collector CollectorConfig `yaml:"collector"`
	Clusters  []ClusterConfig `yaml:"clusters"`
}

type ServerConfig struct {
	Addr          string `yaml:"addr"`
	SecureCookies *bool  `yaml:"secureCookies"`
}

// CookiesSecure reports whether session cookies get the Secure flag (default true).
func (s ServerConfig) CookiesSecure() bool { return s.SecureCookies == nil || *s.SecureCookies }

type AuthConfig struct {
	SessionIdleTimeout time.Duration `yaml:"sessionIdleTimeout"`
	Users              []UserConfig  `yaml:"users"`
}

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleViewer Role = "viewer"
)

type UserConfig struct {
	Username string `yaml:"username"`
	// PasswordHash is a bcrypt hash. Generate with `kafka-ui hash-password`.
	PasswordHash string `yaml:"passwordHash"`
	Role         Role   `yaml:"role"`
}

type StorageConfig struct {
	Path      string        `yaml:"path"`
	Retention time.Duration `yaml:"retention"`
}

type CollectorConfig struct {
	Interval     time.Duration `yaml:"interval"`
	LiveInterval time.Duration `yaml:"liveInterval"`
}

type ClusterConfig struct {
	ID             string                `yaml:"id"`
	Name           string                `yaml:"name"`
	Bootstrap      []string              `yaml:"bootstrap"`
	ReadOnly       bool                  `yaml:"readOnly"`
	SASL           *SASLConfig           `yaml:"sasl"`
	TLS            *TLSConfig            `yaml:"tls"`
	SchemaRegistry *SchemaRegistryConfig `yaml:"schemaRegistry"`
}

type SASLConfig struct {
	Mechanism string `yaml:"mechanism"` // PLAIN, SCRAM-SHA-256, SCRAM-SHA-512
	Username  string `yaml:"username"`
	Password  Secret `yaml:"password"`
}

type TLSConfig struct {
	Enabled            bool   `yaml:"enabled"`
	CAFile             string `yaml:"caFile"`
	CertFile           string `yaml:"certFile"`
	KeyFile            string `yaml:"keyFile"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
}

type SchemaRegistryConfig struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	Password Secret `yaml:"password"`
}

// Secret is a string that can be given inline, as ${ENV_VAR} or as file:/path.
// It never prints its value.
type Secret string

func (s Secret) String() string { return "[redacted]" }

// Value resolves the secret.
func (s Secret) Value() (string, error) {
	v := string(s)
	switch {
	case strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}"):
		name := v[2 : len(v)-1]
		val, ok := os.LookupEnv(name)
		if !ok {
			return "", fmt.Errorf("env var %s not set", name)
		}
		return val, nil
	case strings.HasPrefix(v, "file:"):
		b, err := os.ReadFile(strings.TrimPrefix(v, "file:"))
		if err != nil {
			return "", fmt.Errorf("read secret file: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	default:
		return v, nil
	}
}

var clusterIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// Load reads the YAML file at path, applies defaults and env overrides, and validates it.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return Parse(b)
}

// Parse parses YAML bytes into a validated Config.
func Parse(b []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.applyEnv()
	c.applyDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyEnv() {
	if v := os.Getenv("KAFKAUI_SERVER_ADDR"); v != "" {
		c.Server.Addr = v
	}
	if v := os.Getenv("KAFKAUI_STORAGE_PATH"); v != "" {
		c.Storage.Path = v
	}
}

func (c *Config) applyDefaults() {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.Auth.SessionIdleTimeout == 0 {
		c.Auth.SessionIdleTimeout = 12 * time.Hour
	}
	if c.Storage.Path == "" {
		c.Storage.Path = "kafka-ui.db"
	}
	if c.Storage.Retention == 0 {
		c.Storage.Retention = 7 * 24 * time.Hour
	}
	if c.Collector.Interval == 0 {
		c.Collector.Interval = 5 * time.Second
	}
	if c.Collector.LiveInterval == 0 {
		c.Collector.LiveInterval = 2 * time.Second
	}
	for i := range c.Clusters {
		if c.Clusters[i].Name == "" {
			c.Clusters[i].Name = c.Clusters[i].ID
		}
	}
}

// Validate checks semantic correctness.
func (c *Config) Validate() error {
	var errs []error
	if len(c.Auth.Users) == 0 {
		errs = append(errs, errors.New("auth.users: at least one user is required"))
	}
	seenUser := map[string]bool{}
	for i, u := range c.Auth.Users {
		switch {
		case u.Username == "":
			errs = append(errs, fmt.Errorf("auth.users[%d]: username required", i))
		case seenUser[u.Username]:
			errs = append(errs, fmt.Errorf("auth.users[%d]: duplicate username %q", i, u.Username))
		}
		seenUser[u.Username] = true
		if !strings.HasPrefix(u.PasswordHash, "$2") {
			errs = append(errs, fmt.Errorf("auth.users[%d]: passwordHash must be a bcrypt hash", i))
		}
		if u.Role != RoleAdmin && u.Role != RoleViewer {
			errs = append(errs, fmt.Errorf("auth.users[%d]: role must be admin or viewer", i))
		}
	}
	if len(c.Clusters) == 0 {
		errs = append(errs, errors.New("clusters: at least one cluster is required"))
	}
	seen := map[string]bool{}
	for i, cl := range c.Clusters {
		if !clusterIDRe.MatchString(cl.ID) {
			errs = append(errs, fmt.Errorf("clusters[%d]: id %q must match %s", i, cl.ID, clusterIDRe))
		}
		if seen[cl.ID] {
			errs = append(errs, fmt.Errorf("clusters[%d]: duplicate id %q", i, cl.ID))
		}
		seen[cl.ID] = true
		if len(cl.Bootstrap) == 0 {
			errs = append(errs, fmt.Errorf("clusters[%d]: bootstrap required", i))
		}
		if cl.SASL != nil {
			switch cl.SASL.Mechanism {
			case "PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512":
			default:
				errs = append(errs, fmt.Errorf("clusters[%d]: unsupported sasl mechanism %q", i, cl.SASL.Mechanism))
			}
		}
	}
	return errors.Join(errs...)
}
