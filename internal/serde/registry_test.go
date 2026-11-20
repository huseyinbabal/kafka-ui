package serde

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/twmb/franz-go/pkg/sr"
)

const orderAvro = `{"type":"record","name":"Order","namespace":"shop","fields":[
 {"name":"id","type":"long"},
 {"name":"status","type":{"type":"enum","name":"Status","symbols":["NEW","PAID"]}},
 {"name":"note","type":["null","string"],"default":null},
 {"name":"amount","type":"double"},
 {"name":"tags","type":{"type":"array","items":"string"}},
 {"name":"raw","type":"bytes"}]}`

const orderProto = `syntax = "proto3";
package shop;
message Envelope { string trace = 1; }
message Order {
  int64 id = 1;
  string status = 2;
  message Line { string sku = 1; int32 qty = 2; }
  repeated Line lines = 3;
}`

// fakeRegistry serves just enough of the Schema Registry API for the serde.
func fakeRegistry(t *testing.T, lookups *atomic.Int32) *Registry {
	t.Helper()
	schemas := map[int]sr.SubjectSchema{
		1: {Subject: "orders-value", Version: 1, ID: 1, Schema: sr.Schema{Schema: orderAvro}},
		2: {Subject: "orders-proto-value", Version: 1, ID: 2, Schema: sr.Schema{Schema: orderProto, Type: sr.TypeProtobuf}},
		3: {Subject: "orders-json-value", Version: 1, ID: 3, Schema: sr.Schema{Schema: `{"type":"object"}`, Type: sr.TypeJSON}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.schemaregistry.v1+json")
		path := r.URL.Path
		switch {
		case strings.HasPrefix(path, "/schemas/ids/"):
			lookups.Add(1)
			var id int
			_, _ = sscanInt(strings.TrimPrefix(path, "/schemas/ids/"), &id)
			s, ok := schemas[id]
			if !ok {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"error_code":40403,"message":"Schema not found"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(s.Schema)
		case strings.HasPrefix(path, "/subjects/") && strings.HasSuffix(path, "/versions/latest"):
			subject := strings.TrimSuffix(strings.TrimPrefix(path, "/subjects/"), "/versions/latest")
			for _, s := range schemas {
				if s.Subject == subject {
					_ = json.NewEncoder(w).Encode(s)
					return
				}
			}
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error_code":40401,"message":"Subject not found"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	cl, err := sr.NewClient(sr.URLs(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	return NewRegistry(cl)
}

func sscanInt(s string, v *int) (int, error) { return 1, json.Unmarshal([]byte(s), v) }

func TestAvroRoundTripKeepsFieldOrder(t *testing.T) {
	reg := fakeRegistry(t, &atomic.Int32{})
	ctx := context.Background()
	in := `{"id": 12345678901234, "status": "PAID", "note": {"string": "rush"}, "amount": 12.5, "tags": ["a","b"], "raw": "AQI="}`
	b, err := reg.Encode(ctx, SchemaRef{Subject: "orders-value"}, in)
	if err != nil {
		t.Fatal(err)
	}
	if b[0] != 0 || b[4] != 1 {
		t.Fatalf("missing wire header: %v", b[:5])
	}
	p, err := reg.Decode(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":12345678901234,"status":"PAID","note":"rush","amount":12.5,"tags":["a","b"],"raw":"AQI="}`
	if p.Format != Avro || p.Text != want || *p.SchemaID != 1 {
		t.Fatalf("got %s %s", p.Format, p.Text)
	}
	// Bare union value and a defaulted missing field are accepted too.
	if _, err := reg.Encode(ctx, SchemaRef{Subject: "orders-value"}, `{"id":1,"status":"NEW","amount":1,"tags":[],"raw":""}`); err != nil {
		t.Fatalf("defaults: %v", err)
	}
}

func TestAvroEncodeErrors(t *testing.T) {
	reg := fakeRegistry(t, &atomic.Int32{})
	ctx := context.Background()
	for _, in := range []string{
		`{"id":1.5,"status":"NEW","amount":1,"tags":[],"raw":""}`, // long must be integral
		`{"id":1,"status":"LOST","amount":1,"tags":[],"raw":""}`,  // unknown enum symbol
		`{"status":"NEW","amount":1,"tags":[],"raw":""}`,          // missing required field
		`not json`,
	} {
		if _, err := reg.Encode(ctx, SchemaRef{Subject: "orders-value"}, in); err == nil {
			t.Errorf("%s: expected an error", in)
		}
	}
	if _, err := reg.Encode(ctx, SchemaRef{Subject: "missing"}, `{}`); err == nil {
		t.Error("unknown subject must fail")
	}
}

func TestProtobufNestedMessageRoundTrip(t *testing.T) {
	reg := fakeRegistry(t, &atomic.Int32{})
	ctx := context.Background()
	b, err := reg.Encode(ctx, SchemaRef{Subject: "orders-proto-value", Message: "shop.Order.Line"}, `{"sku":"X-1","qty":3}`)
	if err != nil {
		t.Fatal(err)
	}
	// header + indexes [1,0] => count 2, then 1, 0 as zigzag varints
	if got := b[5:8]; got[0] != 4 || got[1] != 2 || got[2] != 0 {
		t.Fatalf("message indexes: %v", got)
	}
	p, err := reg.Decode(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if p.Format != Protobuf || p.Text != `{"sku":"X-1","qty":3}` {
		t.Fatalf("got %s %s", p.Format, p.Text)
	}
	// Default message is the first one, encoded with the single-zero shortcut.
	b, err = reg.Encode(ctx, SchemaRef{Subject: "orders-proto-value"}, `{"trace":"t1"}`)
	if err != nil || b[5] != 0 {
		t.Fatalf("first message: %v %v", err, b)
	}
	if _, err := reg.Encode(ctx, SchemaRef{Subject: "orders-proto-value", Message: "Nope"}, `{}`); err == nil {
		t.Fatal("unknown message must fail")
	}
}

func TestJSONSchemaAndDecoderFallback(t *testing.T) {
	var lookups atomic.Int32
	reg := fakeRegistry(t, &lookups)
	ctx := context.Background()
	b, err := reg.Encode(ctx, SchemaRef{Subject: "orders-json-value"}, `{"a":1}`)
	if err != nil {
		t.Fatal(err)
	}
	dec := Decoder{Registry: reg, Ctx: ctx}
	if p := dec.Decode(b, Auto, 0); p.Format != JSONSchema || p.Text != `{"a":1}` {
		t.Fatalf("auto decode: %+v", p)
	}
	// Unknown schema id: raw rendering plus an error, and the failure is cached.
	unknown := []byte{0, 0, 0, 0, 99, 'x'}
	for range 3 {
		p := dec.Decode(unknown, Auto, 0)
		if p.Error == "" || p.Format != Base64 {
			t.Fatalf("fallback: %+v", p)
		}
	}
	before := lookups.Load()
	dec.Decode(unknown, Auto, 0)
	if lookups.Load() != before {
		t.Fatal("failed lookups must be cached")
	}
	// Plain JSON never goes to the registry.
	if p := dec.Decode([]byte(`{"plain":true}`), Auto, 0); p.Format != JSON {
		t.Fatalf("plain json: %+v", p)
	}
	// Explicit schema format without a registry explains why.
	if p := (Decoder{}).Decode([]byte("x"), Schema, 0); p.Error == "" {
		t.Fatal("schema format without registry must report an error")
	}
}

func TestMessageIndexes(t *testing.T) {
	for _, idx := range [][]int{{0}, {1}, {2, 0, 5}} {
		b := appendMessageIndexes(nil, idx)
		got, rest, err := readMessageIndexes(append(b, 0xAA))
		if err != nil || len(rest) != 1 || len(got) != len(idx) {
			t.Fatalf("%v: %v %v %v", idx, got, rest, err)
		}
		for i := range idx {
			if got[i] != idx[i] {
				t.Fatalf("%v: got %v", idx, got)
			}
		}
	}
}
