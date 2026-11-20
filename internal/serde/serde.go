// Package serde turns raw record bytes into displayable payloads and back.
// Schema Registry formats (avro/protobuf/jsonschema) are added in M4.
package serde

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"unicode"
	"unicode/utf8"
)

// Format names a (de)serializer.
type Format string

const (
	Auto   Format = "auto"
	String Format = "string"
	JSON   Format = "json"
	Base64 Format = "base64"
	Hex    Format = "hex"
)

// ParseFormat validates a format name; empty means Auto.
func ParseFormat(s string) (Format, error) {
	switch f := Format(s); f {
	case "":
		return Auto, nil
	case Auto, String, JSON, Base64, Hex, Schema:
		return f, nil
	default:
		return "", fmt.Errorf("unknown format %q", s)
	}
}

// Payload is a decoded key, value or header value.
type Payload struct {
	Format Format `json:"format"`
	Text   string `json:"text"`
	// Size is the raw byte length.
	Size int `json:"size"`
	// Truncated is set when Text was cut to the caller's limit.
	Truncated bool `json:"truncated,omitempty"`
	// SchemaID is set when the bytes look like Confluent wire format (magic 0 + id).
	SchemaID *int32 `json:"schemaId,omitempty"`
	// Error explains why schema-based decoding failed; Text then holds a raw rendering.
	Error string `json:"error,omitempty"`
}

// Decoder renders record bytes, using the schema registry when one is configured.
type Decoder struct {
	Registry *Registry // nil when the cluster has no schema registry
	Ctx      context.Context
}

// Decode is like the package-level Decode, but Auto and Schema try the
// registry for payloads in Confluent wire format.
func (d Decoder) Decode(b []byte, f Format, maxText int) *Payload {
	if b == nil {
		return nil
	}
	_, wire := wireSchemaID(b)
	if f != Schema && (f != Auto || !wire || d.Registry == nil) {
		return Decode(b, f, maxText)
	}
	reason := "no schema registry is configured for this cluster"
	if d.Registry != nil {
		ctx := d.Ctx
		if ctx == nil {
			ctx = context.Background()
		}
		p, err := d.Registry.Decode(ctx, b)
		if err == nil {
			truncate(p, maxText)
			return p
		}
		reason = err.Error()
	}
	p := Decode(b, Auto, maxText)
	p.Error = reason
	return p
}

// Decode renders b with format f. A nil b yields nil (Kafka null).
// maxText limits the rendered text (0 = unlimited); the cut never splits a rune.
func Decode(b []byte, f Format, maxText int) *Payload {
	if b == nil {
		return nil
	}
	if f == Auto {
		f = detect(b)
	}
	p := &Payload{Format: f, Size: len(b)}
	switch f {
	case String:
		p.Text = toText(b)
	case JSON:
		if !json.Valid(b) {
			// Manual override on non-JSON: fall back so the user still sees something.
			p.Format = String
			p.Text = toText(b)
			break
		}
		p.Text = string(b)
	case Hex:
		p.Text = hex.EncodeToString(b)
	default:
		p.Format = Base64
		p.Text = base64.StdEncoding.EncodeToString(b)
	}
	if id, ok := wireSchemaID(b); ok {
		p.SchemaID = &id
	}
	truncate(p, maxText)
	return p
}

// truncate cuts p.Text to maxText bytes (0 = no limit) without splitting a rune.
func truncate(p *Payload, maxText int) {
	if maxText > 0 && len(p.Text) > maxText {
		cut := maxText
		for cut > 0 && !utf8.RuneStart(p.Text[cut]) {
			cut--
		}
		p.Text, p.Truncated = p.Text[:cut], true
	}
}

func detect(b []byte) Format {
	if len(b) == 0 {
		return String
	}
	if !printable(b) {
		return Base64
	}
	t := bytes.TrimSpace(b)
	if len(t) > 0 && (t[0] == '{' || t[0] == '[') && json.Valid(t) {
		return JSON
	}
	return String
}

// printable reports whether b is valid UTF-8 without control characters
// other than common whitespace.
func printable(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for _, r := range string(b) {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

// toText converts b to a string, replacing invalid UTF-8 so JSON encoding is lossless to read.
func toText(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	return string(bytes.ToValidUTF8(b, []byte("�")))
}

func wireSchemaID(b []byte) (int32, bool) {
	if len(b) < 6 || b[0] != 0 {
		return 0, false
	}
	id := int32(uint32(b[1])<<24 | uint32(b[2])<<16 | uint32(b[3])<<8 | uint32(b[4]))
	return id, id > 0
}

// Encode turns user input into record bytes. JSON input must be valid and is
// sent as typed; base64/hex are decoded. Auto behaves like String.
func Encode(text string, f Format) ([]byte, error) {
	switch f {
	case JSON:
		if !json.Valid([]byte(text)) {
			return nil, fmt.Errorf("value is not valid JSON")
		}
		return []byte(text), nil
	case Base64:
		b, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			return nil, fmt.Errorf("invalid base64: %w", err)
		}
		return b, nil
	case Hex:
		b, err := hex.DecodeString(text)
		if err != nil {
			return nil, fmt.Errorf("invalid hex: %w", err)
		}
		return b, nil
	default:
		return []byte(text), nil
	}
}
