package serde

import (
	"testing"
)

func TestDecodeAutoDetect(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want Format
		text string
	}{
		{"json object", []byte(`{"a":1}`), JSON, `{"a":1}`},
		{"json array with space", []byte(" [1,2]\n"), JSON, " [1,2]\n"},
		{"plain string", []byte("hello world"), String, "hello world"},
		{"number is string", []byte("42"), String, "42"},
		{"broken json is string", []byte(`{"a":`), String, `{"a":`},
		{"empty", []byte{}, String, ""},
		{"binary", []byte{0xff, 0x00, 0x10}, Base64, "/wAQ"},
		{"control chars", []byte("a\x01b"), Base64, "YQFi"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := Decode(c.in, Auto, 0)
			if p.Format != c.want || p.Text != c.text || p.Size != len(c.in) {
				t.Fatalf("got %+v, want format=%s text=%q", p, c.want, c.text)
			}
		})
	}
}

func TestDecodeNilIsNull(t *testing.T) {
	if Decode(nil, Auto, 0) != nil {
		t.Fatal("nil bytes must decode to nil payload")
	}
}

func TestDecodeOverrides(t *testing.T) {
	if p := Decode([]byte("hi"), Hex, 0); p.Text != "6869" || p.Format != Hex {
		t.Fatalf("hex: %+v", p)
	}
	if p := Decode([]byte("not json"), JSON, 0); p.Format != String {
		t.Fatalf("json override on non-json must fall back to string: %+v", p)
	}
	if p := Decode([]byte{0xff}, String, 0); p.Text != "�" {
		t.Fatalf("invalid utf8 must be replaced: %q", p.Text)
	}
}

func TestDecodeTruncateKeepsRunes(t *testing.T) {
	p := Decode([]byte("aé"), String, 2) // "é" is 2 bytes, cutting at 2 would split it
	if p.Text != "a" || !p.Truncated {
		t.Fatalf("got %+v", p)
	}
}

func TestWireSchemaID(t *testing.T) {
	p := Decode([]byte{0, 0, 0, 0, 7, 'x'}, Auto, 0)
	if p.SchemaID == nil || *p.SchemaID != 7 {
		t.Fatalf("schema id not detected: %+v", p)
	}
}

func TestEncode(t *testing.T) {
	if _, err := Encode("{", JSON); err == nil {
		t.Fatal("invalid json must fail")
	}
	if b, _ := Encode("6869", Hex); string(b) != "hi" {
		t.Fatalf("hex decode: %q", b)
	}
	if b, _ := Encode("aGk=", Base64); string(b) != "hi" {
		t.Fatalf("base64 decode: %q", b)
	}
	if _, err := Encode("zz", Hex); err == nil {
		t.Fatal("bad hex must fail")
	}
}

func TestParseFormat(t *testing.T) {
	if f, err := ParseFormat(""); err != nil || f != Auto {
		t.Fatal("empty must be auto")
	}
	if _, err := ParseFormat("xml"); err == nil {
		t.Fatal("unknown must fail")
	}
}
