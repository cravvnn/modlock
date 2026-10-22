package kv3

import (
	"math"
	"reflect"
	"testing"
)

// TestEncode encodes every kind of value, decodes it back, and refuses every
// truncation of the encoding.
func TestEncode(t *testing.T) {
	// Encode one value of each kind, nested.
	value := Object{
		{Key: "name", Value: "panel"},
		{Key: "empty", Value: ""},
		{Key: "nothing", Value: nil},
		{Key: "on", Value: true},
		{Key: "off", Value: false},
		{Key: "small", Value: -7},
		{Key: "unsigned", Value: uint32(math.MaxUint32)},
		{Key: "large", Value: int64(math.MinInt64)},
		{Key: "huge", Value: uint64(math.MaxUint64)},
		{Key: "half", Value: 0.5},
		{Key: "list", Value: []any{"panel", 1, []any{}, map[string]any{"b": 2, "a": "x"}}},
	}
	data, err := Encode(value)
	if err != nil {
		t.Fatal(err)
	}

	// Decode it back to the decoder's types.
	decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"name": "panel", "empty": "", "nothing": nil, "on": true, "off": false,
		"small": int64(-7), "unsigned": int64(math.MaxUint32), "large": int64(math.MinInt64),
		"huge": uint64(math.MaxUint64), "half": 0.5,
		"list": []any{"panel", int64(1), []any{}, map[string]any{"a": "x", "b": int64(2)}},
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("decoded %#v", decoded)
	}

	// A cut encoding fails instead of panicking.
	for size := range len(data) {
		if _, err := Decode(data[:size]); err == nil {
			t.Fatalf("Decode accepted the encoding cut to %d bytes", size)
		}
	}
}
