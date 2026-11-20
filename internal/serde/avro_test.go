package serde

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/hamba/avro/v2"
)

const richSchema = `{"type":"record","name":"Rich","fields":[
 {"name":"flag","type":"boolean"},
 {"name":"small","type":"int"},
 {"name":"ratio","type":"float"},
 {"name":"attrs","type":{"type":"map","values":"long"}},
 {"name":"hash","type":{"type":"fixed","name":"H4","size":4}},
 {"name":"choice","type":["null","int","string"]},
 {"name":"items","type":{"type":"array","items":{"type":"record","name":"Item","fields":[{"name":"sku","type":"string"}]}}},
 {"name":"nothing","type":"null"}
]}`

func jsonAny(t *testing.T, s string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestAvroRichRoundTrip(t *testing.T) {
	s := avro.MustParse(richSchema)
	in := `{"flag":true,"small":7,"ratio":1.5,"attrs":{"b":2,"a":1},"hash":"AQIDBA==","choice":{"string":"x"},"items":[{"sku":"A"}],"nothing":null}`
	native, err := avroFromJSON(s, jsonAny(t, in))
	if err != nil {
		t.Fatal(err)
	}
	b, err := avro.Marshal(s, native)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := avro.Unmarshal(s, b, &out); err != nil {
		t.Fatal(err)
	}
	got, err := avroToJSON(s, out)
	if err != nil {
		t.Fatal(err)
	}
	// Field order follows the schema, map keys are sorted, fixed renders as base64, unions unwrap.
	want := `{"flag":true,"small":7,"ratio":1.5,"attrs":{"a":1,"b":2},"hash":"AQIDBA==","choice":"x","items":[{"sku":"A"}],"nothing":null}`
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	// A bare union value picks the first branch that fits.
	bare := strings.Replace(in, `{"string":"x"}`, `42`, 1)
	if _, err := avroFromJSON(s, jsonAny(t, bare)); err != nil {
		t.Fatalf("bare union value: %v", err)
	}
}

func TestAvroFromJSONErrors(t *testing.T) {
	s := avro.MustParse(richSchema)
	base := `{"flag":true,"small":7,"ratio":1.5,"attrs":{},"hash":"AQIDBA==","choice":null,"items":[],"nothing":null}`
	cases := map[string][2]string{
		"overflow int":     {`"small":7`, `"small":3000000000`},
		"fraction in int":  {`"small":7`, `"small":1.5`},
		"string for bool":  {`"flag":true`, `"flag":"yes"`},
		"string for float": {`"ratio":1.5`, `"ratio":"x"`},
		"bad fixed size":   {`"hash":"AQIDBA=="`, `"hash":"AQI="`},
		"bad base64":       {`"hash":"AQIDBA=="`, `"hash":"%%"`},
		"union no match":   {`"choice":null`, `"choice":true`},
		"null mismatch":    {`"nothing":null`, `"nothing":1`},
		"map not object":   {`"attrs":{}`, `"attrs":[]`},
		"array not array":  {`"items":[]`, `"items":{}`},
		"bad item":         {`"items":[]`, `"items":[{"sku":1}]`},
		"long as string":   {`"attrs":{}`, `"attrs":{"a":"1"}`},
	}
	for name, c := range cases {
		in := strings.Replace(base, c[0], c[1], 1)
		if _, err := avroFromJSON(s, jsonAny(t, in)); err == nil {
			t.Errorf("%s: expected an error for %s", name, in)
		}
	}
	if _, err := avroFromJSON(s, "not an object"); err == nil {
		t.Error("record from a string must fail")
	}
	nonNull := avro.MustParse(`["int","string"]`)
	if _, err := avroFromJSON(nonNull, nil); err == nil {
		t.Error("null in a non-nullable union must fail")
	}
}

func TestPlainJSONSpecialValues(t *testing.T) {
	var buf bytes.Buffer
	for _, v := range []any{math.NaN(), float32(math.Inf(1)), time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), [2]byte{1, 2}} {
		buf.Reset()
		if err := plainJSON(&buf, v); err != nil {
			t.Fatalf("%v: %v", v, err)
		}
		if !json.Valid(buf.Bytes()) {
			t.Fatalf("%v rendered invalid JSON %s", v, buf.String())
		}
	}
	if !fitsAvro(avro.MustParse(`"long"`), time.Duration(1)) || fitsAvro(avro.MustParse(`"double"`), "x") {
		t.Fatal("fitsAvro type checks")
	}
}
