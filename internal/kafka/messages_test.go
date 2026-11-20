package kafka

import (
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestFilterMatch(t *testing.T) {
	rec := &kgo.Record{
		Key:     []byte("order-42"),
		Value:   []byte(`{"status":"PAID"}`),
		Headers: []kgo.RecordHeader{{Key: "trace-id", Value: []byte("abc123")}},
	}
	cases := []struct {
		field, q string
		regex    bool
		want     bool
	}{
		{"any", "paid", false, true}, // case-insensitive substring
		{"key", "paid", false, false},
		{"value", "PAID", false, true},
		{"header", "abc", false, true},
		{"header", "trace", false, true}, // header key
		{"key", `^order-\d+$`, true, true},
		{"key", `^order-\d$`, true, false},
		{"value", "a.c", false, false}, // substring treats dots literally
	}
	for _, c := range cases {
		f, err := NewFilter(c.field, c.q, c.regex)
		if err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
		if got := f.match(rec); got != c.want {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}

func TestFilterValidation(t *testing.T) {
	if f, err := NewFilter("any", "", false); f != nil || err != nil {
		t.Fatal("empty query must mean no filter")
	}
	if _, err := NewFilter("body", "x", false); err == nil {
		t.Fatal("unknown field must fail")
	}
	if _, err := NewFilter("any", "(", true); err == nil {
		t.Fatal("bad regex must fail")
	}
	var nilFilter *Filter
	if !nilFilter.match(&kgo.Record{}) {
		t.Fatal("nil filter matches everything")
	}
}

func TestParsePartitions(t *testing.T) {
	got, err := ParsePartitions([]string{"2", "0", "2"})
	if err != nil || len(got) != 2 || got[0] != 2 || got[1] != 0 {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := ParsePartitions([]string{"-1"}); err == nil {
		t.Fatal("negative must fail")
	}
	if _, err := ParsePartitions([]string{"x"}); err == nil {
		t.Fatal("non-number must fail")
	}
}
