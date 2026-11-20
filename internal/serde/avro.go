package serde

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"time"

	"github.com/hamba/avro/v2"
)

// avroToJSON renders a value decoded by hamba/avro as JSON, keeping record
// field order from the schema. Bytes and fixed render as base64 strings.
func avroToJSON(s avro.Schema, v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeAvroJSON(&buf, s, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeAvroJSON(buf *bytes.Buffer, s avro.Schema, v any) error {
	if v == nil {
		buf.WriteString("null")
		return nil
	}
	switch sch := s.(type) {
	case *avro.RecordSchema:
		m, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("record %s: got %T", sch.FullName(), v)
		}
		buf.WriteByte('{')
		for i, f := range sch.Fields() {
			if i > 0 {
				buf.WriteByte(',')
			}
			k, _ := json.Marshal(f.Name())
			buf.Write(k)
			buf.WriteByte(':')
			if err := writeAvroJSON(buf, f.Type(), m[f.Name()]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
		return nil
	case *avro.ArraySchema:
		items, ok := v.([]any)
		if !ok {
			return plainJSON(buf, v)
		}
		buf.WriteByte('[')
		for i, it := range items {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeAvroJSON(buf, sch.Items(), it); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	case *avro.MapSchema:
		m, ok := v.(map[string]any)
		if !ok {
			return plainJSON(buf, v)
		}
		buf.WriteByte('{')
		first := true
		for _, k := range sortedKeys(m) {
			if !first {
				buf.WriteByte(',')
			}
			first = false
			kb, _ := json.Marshal(k)
			buf.Write(kb)
			buf.WriteByte(':')
			if err := writeAvroJSON(buf, sch.Values(), m[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
		return nil
	case *avro.UnionSchema:
		// hamba returns union values unwrapped; pick the branch that fits the Go value.
		for _, t := range sch.Types() {
			if t.Type() != avro.Null && fitsAvro(t, v) {
				return writeAvroJSON(buf, t, v)
			}
		}
		return plainJSON(buf, v)
	}
	return plainJSON(buf, v)
}

func fitsAvro(s avro.Schema, v any) bool {
	switch s.Type() {
	case avro.Record:
		_, ok := v.(map[string]any)
		return ok
	case avro.Map:
		_, ok := v.(map[string]any)
		return ok
	case avro.Array:
		_, ok := v.([]any)
		return ok
	case avro.String, avro.Enum:
		_, ok := v.(string)
		return ok
	case avro.Bytes, avro.Fixed:
		switch v.(type) {
		case []byte, *big.Rat:
			return true
		}
		return isByteArray(v)
	case avro.Boolean:
		_, ok := v.(bool)
		return ok
	case avro.Int, avro.Long:
		switch v.(type) {
		case int, int32, int64, time.Time, time.Duration:
			return true
		}
	case avro.Float, avro.Double:
		switch v.(type) {
		case float32, float64:
			return true
		}
	}
	return false
}

func isByteArray(v any) bool {
	t := reflect.TypeOf(v)
	return t != nil && t.Kind() == reflect.Array && t.Elem().Kind() == reflect.Uint8
}

func plainJSON(buf *bytes.Buffer, v any) error {
	if isByteArray(v) {
		rv := reflect.ValueOf(v)
		b := make([]byte, rv.Len())
		reflect.Copy(reflect.ValueOf(b), rv)
		v = b
	}
	switch x := v.(type) {
	case []byte:
		v = base64.StdEncoding.EncodeToString(x)
	case *big.Rat:
		v = json.Number(x.FloatString(10))
	case time.Time:
		v = x.UTC().Format(time.RFC3339Nano)
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			v = strconv.FormatFloat(x, 'g', -1, 64)
		}
	case float32:
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			v = strconv.FormatFloat(float64(x), 'g', -1, 32)
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	buf.Write(b)
	return nil
}

// avroFromJSON converts JSON (decoded with UseNumber) into Go values hamba/avro
// can encode for schema s. Union values may be bare or wrapped as {"type": value}.
func avroFromJSON(s avro.Schema, v any) (any, error) {
	switch sch := s.(type) {
	case *avro.RecordSchema:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: expected an object", sch.FullName())
		}
		out := make(map[string]any, len(sch.Fields()))
		for _, f := range sch.Fields() {
			fv, present := m[f.Name()]
			if !present {
				if f.HasDefault() {
					fv = f.Default()
					if fv == nil {
						out[f.Name()] = nil
						continue
					}
				} else {
					return nil, fmt.Errorf("%s: missing field %q", sch.FullName(), f.Name())
				}
			}
			c, err := avroFromJSON(f.Type(), fv)
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", sch.Name(), f.Name(), err)
			}
			out[f.Name()] = c
		}
		return out, nil
	case *avro.ArraySchema:
		items, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("expected an array")
		}
		out := make([]any, len(items))
		for i, it := range items {
			c, err := avroFromJSON(sch.Items(), it)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out[i] = c
		}
		return out, nil
	case *avro.MapSchema:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected an object")
		}
		out := make(map[string]any, len(m))
		for k, mv := range m {
			c, err := avroFromJSON(sch.Values(), mv)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			out[k] = c
		}
		return out, nil
	case *avro.UnionSchema:
		if v == nil {
			if sch.Nullable() {
				return nil, nil
			}
			return nil, fmt.Errorf("null is not allowed in this union")
		}
		if m, ok := v.(map[string]any); ok && len(m) == 1 {
			for k, inner := range m {
				for _, t := range sch.Types() {
					if unionName(t) == k {
						c, err := avroFromJSON(t, inner)
						if err != nil {
							return nil, err
						}
						return map[string]any{k: c}, nil
					}
				}
			}
		}
		for _, t := range sch.Types() {
			if t.Type() == avro.Null {
				continue
			}
			if c, err := avroFromJSON(t, v); err == nil {
				return map[string]any{unionName(t): c}, nil
			}
		}
		return nil, fmt.Errorf("value matches no branch of the union")
	case *avro.EnumSchema:
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected an enum symbol")
		}
		for _, sym := range sch.Symbols() {
			if sym == str {
				return str, nil
			}
		}
		return nil, fmt.Errorf("unknown enum symbol %q", str)
	case *avro.FixedSchema:
		b, err := jsonBytes(v)
		if err != nil {
			return nil, err
		}
		if len(b) != sch.Size() {
			return nil, fmt.Errorf("fixed %s needs %d bytes, got %d", sch.Name(), sch.Size(), len(b))
		}
		arr := make([]byte, sch.Size())
		copy(arr, b)
		return avroFixed(arr, sch.Size()), nil
	}

	switch s.Type() {
	case avro.Null:
		if v != nil {
			return nil, fmt.Errorf("expected null")
		}
		return nil, nil
	case avro.Boolean:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("expected a boolean")
		}
		return b, nil
	case avro.String:
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected a string")
		}
		return str, nil
	case avro.Bytes:
		return jsonBytes(v)
	case avro.Int:
		n, err := jsonInt(v)
		if err != nil {
			return nil, err
		}
		if n < math.MinInt32 || n > math.MaxInt32 {
			return nil, fmt.Errorf("%d overflows int", n)
		}
		return int(n), nil
	case avro.Long:
		return jsonInt(v)
	case avro.Float:
		f, err := jsonFloat(v)
		return float32(f), err
	case avro.Double:
		return jsonFloat(v)
	}
	return nil, fmt.Errorf("unsupported schema type %s", s.Type())
}

func unionName(s avro.Schema) string {
	if n, ok := s.(avro.NamedSchema); ok {
		return n.FullName()
	}
	return string(s.Type())
}

func jsonInt(v any) (int64, error) {
	switch n := v.(type) {
	case json.Number:
		return n.Int64()
	case float64:
		if n != math.Trunc(n) {
			return 0, fmt.Errorf("expected an integer")
		}
		return int64(n), nil
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	}
	return 0, fmt.Errorf("expected an integer")
}

func jsonFloat(v any) (float64, error) {
	switch n := v.(type) {
	case json.Number:
		return n.Float64()
	case float64:
		return n, nil
	}
	return 0, fmt.Errorf("expected a number")
}

// jsonBytes accepts base64 (what the browser shows for bytes fields).
func jsonBytes(v any) ([]byte, error) {
	str, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("expected a base64 string")
	}
	b, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return nil, fmt.Errorf("invalid base64: %w", err)
	}
	return b, nil
}

// avroFixed builds the [N]byte array hamba expects for fixed types.
func avroFixed(b []byte, n int) any {
	arr := reflect.New(reflect.ArrayOf(n, reflect.TypeFor[byte]())).Elem()
	reflect.Copy(arr, reflect.ValueOf(b))
	return arr.Interface()
}

func sortedKeys(m map[string]any) []string { return slices.Sorted(maps.Keys(m)) }
