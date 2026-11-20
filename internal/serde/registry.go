package serde

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bufbuild/protocompile"
	"github.com/hamba/avro/v2"
	"github.com/twmb/franz-go/pkg/sr"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Schema-backed payload formats. Schema is the request format meaning "use
// the registry"; the others are what a decoded payload reports.
const (
	Schema     Format = "schema"
	Avro       Format = "avro"
	Protobuf   Format = "protobuf"
	JSONSchema Format = "jsonschema"
)

// SchemaRef picks the schema used to encode a produced key or value.
type SchemaRef struct {
	Subject string `json:"subject"`
	Version int    `json:"version"` // 0 or -1 = latest
	// Message is the fully qualified protobuf message name; empty = first message.
	Message string `json:"message,omitempty"`
}

// failedLookupTTL caches failed schema lookups so a 100k-record scan over a
// topic with a missing schema doesn't call the registry 100k times.
const failedLookupTTL = 30 * time.Second

type compiled struct {
	id    int
	typ   sr.SchemaType
	avro  avro.Schema
	proto protoreflect.FileDescriptor
	err   error
	at    time.Time
}

// Registry decodes and encodes Confluent wire-format payloads.
type Registry struct {
	cl *sr.Client

	mu   sync.Mutex
	byID map[int]*compiled
}

func NewRegistry(cl *sr.Client) *Registry {
	return &Registry{cl: cl, byID: map[int]*compiled{}}
}

func (r *Registry) schemaByID(ctx context.Context, id int) *compiled {
	r.mu.Lock()
	c, ok := r.byID[id]
	r.mu.Unlock()
	if ok && (c.err == nil || time.Since(c.at) < failedLookupTTL) {
		return c
	}
	c = &compiled{id: id, at: time.Now()}
	s, err := r.cl.SchemaByID(ctx, id)
	if err == nil {
		c.typ = s.Type
		err = r.compile(ctx, c, s)
	}
	c.err = err
	// Don't cache a lookup that failed only because the caller's context ended.
	if err == nil || ctx.Err() == nil {
		r.mu.Lock()
		r.byID[id] = c
		r.mu.Unlock()
	}
	return c
}

func (r *Registry) compile(ctx context.Context, c *compiled, s sr.Schema) error {
	switch s.Type {
	case sr.TypeAvro:
		cache := &avro.SchemaCache{}
		if err := r.avroRefs(ctx, cache, s.References, 0); err != nil {
			return err
		}
		as, err := avro.ParseWithCache(s.Schema, "", cache)
		if err != nil {
			return fmt.Errorf("parse avro schema: %w", err)
		}
		c.avro = as
	case sr.TypeProtobuf:
		files := map[string]string{}
		if err := r.protoRefs(ctx, files, s.References, 0); err != nil {
			return err
		}
		name := fmt.Sprintf("schema-%d.proto", c.id)
		files[name] = s.Schema
		comp := protocompile.Compiler{Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			Accessor: protocompile.SourceAccessorFromMap(files),
		})}
		fds, err := comp.Compile(ctx, name)
		if err != nil {
			return fmt.Errorf("compile protobuf schema: %w", err)
		}
		c.proto = fds[0]
	case sr.TypeJSON:
	default:
		return fmt.Errorf("unsupported schema type %s", s.Type)
	}
	return nil
}

const maxRefDepth = 10

func (r *Registry) avroRefs(ctx context.Context, cache *avro.SchemaCache, refs []sr.SchemaReference, depth int) error {
	if depth > maxRefDepth {
		return errors.New("schema references nest too deep")
	}
	for _, ref := range refs {
		ss, err := r.cl.SchemaByVersion(ctx, ref.Subject, ref.Version)
		if err != nil {
			return fmt.Errorf("reference %s: %w", ref.Name, err)
		}
		if err := r.avroRefs(ctx, cache, ss.References, depth+1); err != nil {
			return err
		}
		if _, err := avro.ParseWithCache(ss.Schema.Schema, "", cache); err != nil {
			return fmt.Errorf("reference %s: %w", ref.Name, err)
		}
	}
	return nil
}

func (r *Registry) protoRefs(ctx context.Context, files map[string]string, refs []sr.SchemaReference, depth int) error {
	if depth > maxRefDepth {
		return errors.New("schema references nest too deep")
	}
	for _, ref := range refs {
		if _, done := files[ref.Name]; done {
			continue
		}
		ss, err := r.cl.SchemaByVersion(ctx, ref.Subject, ref.Version)
		if err != nil {
			return fmt.Errorf("reference %s: %w", ref.Name, err)
		}
		files[ref.Name] = ss.Schema.Schema
		if err := r.protoRefs(ctx, files, ss.References, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// Decode renders a wire-format payload (magic byte, schema id, body) as JSON.
func (r *Registry) Decode(ctx context.Context, b []byte) (*Payload, error) {
	id, ok := wireSchemaID(b)
	if !ok {
		return nil, errors.New("not in schema registry wire format")
	}
	c := r.schemaByID(ctx, int(id))
	if c.err != nil {
		return nil, fmt.Errorf("schema %d: %w", id, c.err)
	}
	body := b[5:]
	sid := id
	p := &Payload{Size: len(b), SchemaID: &sid}
	switch c.typ {
	case sr.TypeAvro:
		var v any
		if err := avro.Unmarshal(c.avro, body, &v); err != nil {
			return nil, fmt.Errorf("avro decode: %w", err)
		}
		out, err := avroToJSON(c.avro, v)
		if err != nil {
			return nil, err
		}
		p.Format, p.Text = Avro, string(out)
	case sr.TypeProtobuf:
		idx, rest, err := readMessageIndexes(body)
		if err != nil {
			return nil, err
		}
		md, err := messageByIndexes(c.proto, idx)
		if err != nil {
			return nil, err
		}
		msg := dynamicpb.NewMessage(md)
		if err := proto.Unmarshal(rest, msg); err != nil {
			return nil, fmt.Errorf("protobuf decode: %w", err)
		}
		out, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
		if err != nil {
			return nil, err
		}
		// protojson randomizes whitespace on purpose; normalize it.
		var buf bytes.Buffer
		if err := json.Compact(&buf, out); err != nil {
			return nil, err
		}
		p.Format, p.Text = Protobuf, buf.String()
	case sr.TypeJSON:
		if !json.Valid(body) {
			return nil, errors.New("json schema payload is not valid JSON")
		}
		p.Format, p.Text = JSONSchema, string(body)
	}
	return p, nil
}

// Encode serializes JSON text with the referenced schema into wire format.
func (r *Registry) Encode(ctx context.Context, ref SchemaRef, text string) ([]byte, error) {
	if ref.Subject == "" {
		return nil, errors.New("schema subject is required")
	}
	version := ref.Version
	if version == 0 {
		version = -1
	}
	ss, err := r.cl.SchemaByVersion(ctx, ref.Subject, version)
	if err != nil {
		return nil, fmt.Errorf("schema %s: %w", ref.Subject, err)
	}
	c := r.schemaByID(ctx, ss.ID)
	if c.err != nil {
		return nil, fmt.Errorf("schema %d: %w", ss.ID, c.err)
	}
	out := []byte{0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(out[1:], uint32(ss.ID)) //nolint:gosec // registry ids are positive int32

	switch c.typ {
	case sr.TypeAvro:
		dec := json.NewDecoder(bytes.NewReader([]byte(text)))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			return nil, fmt.Errorf("value is not valid JSON: %w", err)
		}
		native, err := avroFromJSON(c.avro, v)
		if err != nil {
			return nil, err
		}
		body, err := avro.Marshal(c.avro, native)
		if err != nil {
			return nil, fmt.Errorf("avro encode: %w", err)
		}
		return append(out, body...), nil
	case sr.TypeProtobuf:
		md, idx, err := messageByName(c.proto, ref.Message)
		if err != nil {
			return nil, err
		}
		msg := dynamicpb.NewMessage(md)
		if err := protojson.Unmarshal([]byte(text), msg); err != nil {
			return nil, fmt.Errorf("value does not match %s: %w", md.FullName(), err)
		}
		body, err := proto.Marshal(msg)
		if err != nil {
			return nil, err
		}
		return append(appendMessageIndexes(out, idx), body...), nil
	case sr.TypeJSON:
		if !json.Valid([]byte(text)) {
			return nil, errors.New("value is not valid JSON")
		}
		return append(out, text...), nil
	}
	return nil, fmt.Errorf("unsupported schema type %s", c.typ)
}

// readMessageIndexes parses the protobuf message-index prefix: a zigzag varint
// count followed by that many indexes; a lone 0 means [0].
func readMessageIndexes(b []byte) ([]int, []byte, error) {
	n, read := binary.Varint(b)
	if read <= 0 || n < 0 || n > 100 {
		return nil, nil, errors.New("invalid protobuf message indexes")
	}
	b = b[read:]
	if n == 0 {
		return []int{0}, b, nil
	}
	idx := make([]int, n)
	for i := range idx {
		v, read := binary.Varint(b)
		if read <= 0 || v < 0 {
			return nil, nil, errors.New("invalid protobuf message indexes")
		}
		idx[i] = int(v)
		b = b[read:]
	}
	return idx, b, nil
}

func appendMessageIndexes(b []byte, idx []int) []byte {
	if len(idx) == 1 && idx[0] == 0 {
		return append(b, 0)
	}
	b = binary.AppendVarint(b, int64(len(idx)))
	for _, i := range idx {
		b = binary.AppendVarint(b, int64(i))
	}
	return b
}

func messageByIndexes(fd protoreflect.FileDescriptor, idx []int) (protoreflect.MessageDescriptor, error) {
	msgs := fd.Messages()
	var md protoreflect.MessageDescriptor
	for _, i := range idx {
		if i >= msgs.Len() {
			return nil, fmt.Errorf("message index %v not in schema", idx)
		}
		md = msgs.Get(i)
		msgs = md.Messages()
	}
	if md == nil {
		return nil, errors.New("schema has no messages")
	}
	return md, nil
}

// messageByName finds a message (first top-level one when name is empty) and its index path.
func messageByName(fd protoreflect.FileDescriptor, name string) (protoreflect.MessageDescriptor, []int, error) {
	if name == "" {
		if fd.Messages().Len() == 0 {
			return nil, nil, errors.New("schema has no messages")
		}
		return fd.Messages().Get(0), []int{0}, nil
	}
	var walk func(msgs protoreflect.MessageDescriptors, path []int) (protoreflect.MessageDescriptor, []int)
	walk = func(msgs protoreflect.MessageDescriptors, path []int) (protoreflect.MessageDescriptor, []int) {
		for i := range msgs.Len() {
			md := msgs.Get(i)
			p := append(append([]int(nil), path...), i)
			if string(md.FullName()) == name || string(md.Name()) == name {
				return md, p
			}
			if found, fp := walk(md.Messages(), p); found != nil {
				return found, fp
			}
		}
		return nil, nil
	}
	if md, p := walk(fd.Messages(), nil); md != nil {
		return md, p, nil
	}
	return nil, nil, fmt.Errorf("message %q not in schema", name)
}
