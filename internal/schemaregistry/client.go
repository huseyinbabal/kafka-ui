// Package schemaregistry wraps a Confluent-compatible Schema Registry for the API.
package schemaregistry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/sr"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
	"github.com/huseyinbabal/kafka-ui/internal/config"
)

type Client struct {
	SR *sr.Client
}

func New(cfg *config.SchemaRegistryConfig) (*Client, error) {
	opts := []sr.ClientOpt{
		sr.URLs(cfg.URL),
		sr.UserAgent("kafka-ui"),
		sr.HTTPClient(&http.Client{Timeout: 10 * time.Second}),
	}
	if cfg.Username != "" {
		pw, err := cfg.Password.Value()
		if err != nil {
			return nil, fmt.Errorf("schema registry password: %w", err)
		}
		opts = append(opts, sr.BasicAuth(cfg.Username, pw))
	}
	cl, err := sr.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return &Client{SR: cl}, nil
}

type SubjectSummary struct {
	Subject       string `json:"subject"`
	LatestVersion int    `json:"latestVersion"`
	SchemaID      int    `json:"schemaId"`
	SchemaType    string `json:"schemaType"`
	Versions      int    `json:"versions"`
}

type SchemaVersion struct {
	Subject    string               `json:"subject"`
	Version    int                  `json:"version"`
	ID         int                  `json:"id"`
	SchemaType string               `json:"schemaType"`
	Schema     string               `json:"schema"`
	References []sr.SchemaReference `json:"references"`
}

type SubjectDetail struct {
	Subject  string          `json:"subject"`
	Versions []SchemaVersion `json:"versions"`
	// Compatibility is the subject's own level; empty means it inherits Global.
	Compatibility string `json:"compatibility"`
	Global        string `json:"globalCompatibility"`
}

// ListSubjects returns every subject with its latest version, in one request.
func (c *Client) ListSubjects(ctx context.Context) ([]SubjectSummary, error) {
	all, err := c.SR.AllSchemas(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	bySubject := map[string]*SubjectSummary{}
	for _, s := range all {
		cur, ok := bySubject[s.Subject]
		if !ok {
			cur = &SubjectSummary{Subject: s.Subject}
			bySubject[s.Subject] = cur
		}
		cur.Versions++
		if s.Version > cur.LatestVersion {
			cur.LatestVersion, cur.SchemaID, cur.SchemaType = s.Version, s.ID, s.Type.String()
		}
	}
	out := make([]SubjectSummary, 0, len(bySubject))
	for _, s := range bySubject {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Subject < out[j].Subject })
	return out, nil
}

func (c *Client) GetSubject(ctx context.Context, subject string) (*SubjectDetail, error) {
	schemas, err := c.SR.Schemas(ctx, subject)
	if err != nil {
		return nil, mapErr(err)
	}
	d := &SubjectDetail{Subject: subject, Versions: make([]SchemaVersion, 0, len(schemas))}
	for _, s := range schemas {
		d.Versions = append(d.Versions, toVersion(s))
	}
	sort.Slice(d.Versions, func(i, j int) bool { return d.Versions[i].Version > d.Versions[j].Version })
	d.Compatibility, d.Global, err = c.compatibility(ctx, subject)
	return d, err
}

func toVersion(s sr.SubjectSchema) SchemaVersion {
	refs := s.References
	if refs == nil {
		refs = []sr.SchemaReference{}
	}
	return SchemaVersion{Subject: s.Subject, Version: s.Version, ID: s.ID, SchemaType: s.Type.String(), Schema: s.Schema.Schema, References: refs}
}

// compatibility returns the subject's own level ("" when inherited) and the global level.
func (c *Client) compatibility(ctx context.Context, subject string) (string, string, error) {
	rs := c.SR.Compatibility(ctx, "", subject)
	var own, global string
	for _, r := range rs {
		if r.Err != nil {
			// 40408: the subject has no own config and inherits the global one.
			if se := schemaErr(r.Err); se != nil && se.ErrorCode == 40408 {
				continue
			}
			return "", "", mapErr(r.Err)
		}
		if r.Subject == "" {
			global = r.Level.String()
		} else {
			own = r.Level.String()
		}
	}
	return own, global, nil
}

type RegisterRequest struct {
	Schema     string               `json:"schema"`
	SchemaType string               `json:"schemaType"` // AVRO (default), PROTOBUF, JSON
	References []sr.SchemaReference `json:"references"`
}

func (r RegisterRequest) toSchema() (sr.Schema, error) {
	var t sr.SchemaType
	if r.SchemaType == "" {
		r.SchemaType = "AVRO"
	}
	if err := t.UnmarshalText([]byte(strings.ToUpper(r.SchemaType))); err != nil {
		return sr.Schema{}, apierr.BadRequest("schemaType must be AVRO, PROTOBUF or JSON", err)
	}
	if strings.TrimSpace(r.Schema) == "" {
		return sr.Schema{}, apierr.BadRequest("schema must not be empty", nil)
	}
	return sr.Schema{Schema: r.Schema, Type: t, References: r.References}, nil
}

// Register adds a new version (or returns the existing one for an identical schema).
func (c *Client) Register(ctx context.Context, subject string, req RegisterRequest) (*SchemaVersion, error) {
	s, err := req.toSchema()
	if err != nil {
		return nil, err
	}
	ss, err := c.SR.CreateSchema(ctx, subject, s)
	if err != nil {
		return nil, mapErr(err)
	}
	v := toVersion(ss)
	return &v, nil
}

type CompatibilityCheck struct {
	Compatible bool     `json:"compatible"`
	Messages   []string `json:"messages"`
}

// CheckCompatibility tests a schema against the latest version of subject.
func (c *Client) CheckCompatibility(ctx context.Context, subject string, req RegisterRequest) (*CompatibilityCheck, error) {
	s, err := req.toSchema()
	if err != nil {
		return nil, err
	}
	res, err := c.SR.CheckCompatibility(sr.WithParams(ctx, sr.Verbose), subject, -1, s)
	if err != nil {
		// A subject without versions accepts anything.
		if se := schemaErr(err); se != nil && (se.ErrorCode == 40401 || se.ErrorCode == 40402) {
			return &CompatibilityCheck{Compatible: true, Messages: []string{}}, nil
		}
		return nil, mapErr(err)
	}
	msgs := res.Messages
	if msgs == nil {
		msgs = []string{}
	}
	return &CompatibilityCheck{Compatible: res.Is, Messages: msgs}, nil
}

// SetCompatibility sets the subject's level; an empty level resets it to inherit the global one.
func (c *Client) SetCompatibility(ctx context.Context, subject, level string) error {
	var rs []sr.CompatibilityResult
	if level == "" {
		rs = c.SR.ResetCompatibility(ctx, subject)
	} else {
		var l sr.CompatibilityLevel
		if err := l.UnmarshalText([]byte(strings.ToUpper(level))); err != nil {
			return apierr.BadRequest("unknown compatibility level", err)
		}
		rs = c.SR.SetCompatibility(ctx, sr.SetCompatibility{Level: l}, subject)
	}
	for _, r := range rs {
		if r.Err != nil {
			if se := schemaErr(r.Err); level == "" && se != nil && se.ErrorCode == 40408 {
				continue // already inheriting
			}
			return mapErr(r.Err)
		}
	}
	return nil
}

// DeleteSubject soft-deletes a subject; permanent also hard-deletes it afterwards.
func (c *Client) DeleteSubject(ctx context.Context, subject string, permanent bool) ([]int, error) {
	versions, err := c.SR.DeleteSubject(ctx, subject, sr.SoftDelete)
	if err != nil {
		// Already soft-deleted: allow going straight to the hard delete.
		alreadySoftDeleted := schemaErr(err) != nil && schemaErr(err).ErrorCode == 40404
		if !permanent || !alreadySoftDeleted {
			return nil, mapErr(err)
		}
	}
	if permanent {
		if v, err := c.SR.DeleteSubject(ctx, subject, sr.HardDelete); err != nil {
			return nil, mapErr(err)
		} else if versions == nil {
			versions = v
		}
	}
	return versions, nil
}

func schemaErr(err error) *sr.ResponseError {
	var re *sr.ResponseError
	if errors.As(err, &re) {
		return re
	}
	return nil
}

// mapErr turns registry errors into API errors with the registry's message.
func mapErr(err error) error {
	re := schemaErr(err)
	if re == nil {
		return &apierr.Error{Status: http.StatusBadGateway, Code: "SCHEMA_REGISTRY_ERROR", Message: "schema registry request failed: " + err.Error(), Err: err}
	}
	status := re.StatusCode
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		status = http.StatusBadGateway // the registry rejected *our* credentials
	case status >= 500 || status == 0:
		status = http.StatusBadGateway
	}
	code := "SCHEMA_REGISTRY_ERROR"
	switch re.ErrorCode {
	case 40401:
		code = "SUBJECT_NOT_FOUND"
	case 40402, 40403:
		code = "SCHEMA_NOT_FOUND"
	case 409:
		code = "SCHEMA_INCOMPATIBLE"
	case 42201:
		code = "SCHEMA_INVALID"
	}
	msg := re.Message
	if msg == "" {
		msg = re.Error()
	}
	return &apierr.Error{Status: status, Code: code, Message: msg, Err: err}
}
