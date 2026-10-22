package kv3

import (
	"encoding/binary"
	"maps"
	"math"
	"slices"

	"github.com/pkg/errors"
)

// Object is a KeyValues3 object whose members keep their order. Encode also
// takes a map[string]any, whose members it writes in key order.
type Object []Member

// Member is one key and value of an Object.
type Member struct {
	// Key names the member.
	Key string
	// Value is the member's value.
	Value any
}

// The generic KeyValues3 format GUID 7412167c-06e9-4698-aff2-e63eb59037e7,
// in the byte order the header stores it.
var genericFormat = [16]byte{
	0x7c, 0x16, 0x12, 0x74, 0xe9, 0x06, 0x98, 0x46,
	0xaf, 0xf2, 0xe6, 0x3e, 0xb5, 0x90, 0x37, 0xe7,
}

// frameSize is the LZ4 frame size the header names.
const frameSize = 16384

// Encode writes value as binary KeyValues3 version 5 with LZ4 compression,
// the form the resource compiler writes. It takes nil, bool, every integer
// type, float32, float64, string, []any, Object and map[string]any. An
// integer is written in the narrowest of int32, uint32, int64 and uint64 that
// holds it.
func Encode(value any) ([]byte, error) {
	// Walk the value into the typed lanes.
	e := &encoder{strings: make(map[string]int32)}
	if err := e.walk(value, 0); err != nil {
		return nil, err
	}

	// The first buffer holds the strings, then their count.
	var first []byte
	for _, text := range e.order {
		first = append(append(first, text...), 0)
	}
	stringBytes := len(first)
	first = pad(first, 4)
	first = binary.LittleEndian.AppendUint32(first, uint32(len(e.order)))

	// The second holds the object sizes, then each lane at its alignment,
	// then the types and the end marker.
	second := slices.Concat(e.lengths, e.bytes4)
	if len(e.bytes8) != 0 {
		second = append(pad(second, 8), e.bytes8...)
	}
	second = append(second, e.types...)
	second = binary.LittleEndian.AppendUint32(second, end)

	// Write the format, the compression and its frame size, then the sizes
	// and counts in the order version 5 declares them: no blobs, no one- or
	// two-byte values, and one compressed block per buffer.
	packedFirst, packedSecond := lz4Literals(first), lz4Literals(second)
	out := binary.LittleEndian.AppendUint32(nil, magic)
	out = append(out, genericFormat[:]...)
	out = binary.LittleEndian.AppendUint32(out, compressionLZ4)
	out = binary.LittleEndian.AppendUint16(out, 0)
	out = binary.LittleEndian.AppendUint16(out, frameSize)
	for _, field := range []int{stringBytes, 1, 0, len(e.types)} {
		out = binary.LittleEndian.AppendUint32(out, uint32(field))
	}
	out = binary.LittleEndian.AppendUint16(out, uint16(e.objects))
	out = binary.LittleEndian.AppendUint16(out, uint16(e.arrays))
	for _, field := range []int{
		len(first) + len(second), len(packedFirst) + len(packedSecond),
		0, 0, 0, 0,
		len(first), len(packedFirst), len(second), len(packedSecond),
		0, 0, len(e.bytes4) / 4, len(e.bytes8) / 8,
		0, e.objects, e.arrays, 0,
	} {
		out = binary.LittleEndian.AppendUint32(out, uint32(field))
	}

	// The compressed buffers follow the header.
	return slices.Concat(out, packedFirst, packedSecond), nil
}

// encoder accumulates the lanes of one value walk.
type encoder struct {
	// strings numbers each distinct string by its first use.
	strings map[string]int32
	// order lists the strings by number.
	order []string
	// types is the type of each value in walk order.
	types []byte
	// lengths is the member count of each object.
	lengths []byte
	// bytes4 and bytes8 are the four- and eight-byte values.
	bytes4, bytes8 []byte
	// objects and arrays count the containers.
	objects, arrays int
}

// walk appends value and everything it holds.
func (e *encoder) walk(value any, depth int) error {
	if depth > maxDepth {
		return errors.New("KeyValues3 value nests too deeply")
	}
	switch v := value.(type) {
	case nil:
		e.types = append(e.types, 1)
	case bool:
		kind := byte(14)
		if v {
			kind = 13
		}
		e.types = append(e.types, kind)
	case string:
		e.types = append(e.types, 6)
		e.bytes4 = binary.LittleEndian.AppendUint32(e.bytes4, uint32(e.text(v)))
	case int:
		e.integer(int64(v))
	case int8:
		e.integer(int64(v))
	case int16:
		e.integer(int64(v))
	case int32:
		e.integer(int64(v))
	case int64:
		e.integer(v)
	case uint8:
		e.integer(int64(v))
	case uint16:
		e.integer(int64(v))
	case uint32:
		e.integer(int64(v))
	case uint:
		e.unsigned(uint64(v))
	case uint64:
		e.unsigned(v)
	case float32:
		e.double(float64(v))
	case float64:
		e.double(v)
	case []any:
		e.types = append(e.types, 8)
		e.arrays++
		e.bytes4 = binary.LittleEndian.AppendUint32(e.bytes4, uint32(len(v)))
		for _, item := range v {
			if err := e.walk(item, depth+1); err != nil {
				return err
			}
		}
	case map[string]any:
		object := make(Object, 0, len(v))
		for _, key := range slices.Sorted(maps.Keys(v)) {
			object = append(object, Member{Key: key, Value: v[key]})
		}
		return e.walk(object, depth)
	case Object:
		e.types = append(e.types, 9)
		e.objects++
		e.lengths = binary.LittleEndian.AppendUint32(e.lengths, uint32(len(v)))
		for _, member := range v {
			e.bytes4 = binary.LittleEndian.AppendUint32(e.bytes4, uint32(e.text(member.Key)))
			if err := e.walk(member.Value, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.Errorf("KeyValues3 cannot hold a %T", value)
	}
	return nil
}

// integer appends a signed integer as int32, uint32 or int64.
func (e *encoder) integer(v int64) {
	switch {
	case v >= math.MinInt32 && v <= math.MaxInt32:
		e.types = append(e.types, 11)
		e.bytes4 = binary.LittleEndian.AppendUint32(e.bytes4, uint32(int32(v)))
	case v >= 0 && v <= math.MaxUint32:
		e.types = append(e.types, 12)
		e.bytes4 = binary.LittleEndian.AppendUint32(e.bytes4, uint32(v))
	default:
		e.types = append(e.types, 3)
		e.bytes8 = binary.LittleEndian.AppendUint64(e.bytes8, uint64(v))
	}
}

// unsigned appends an unsigned integer, as uint64 when no signed type holds
// it.
func (e *encoder) unsigned(v uint64) {
	if v <= math.MaxInt64 {
		e.integer(int64(v))
		return
	}
	e.types = append(e.types, 4)
	e.bytes8 = binary.LittleEndian.AppendUint64(e.bytes8, v)
}

// double appends a float64.
func (e *encoder) double(v float64) {
	e.types = append(e.types, 5)
	e.bytes8 = binary.LittleEndian.AppendUint64(e.bytes8, math.Float64bits(v))
}

// text returns the id of a string, numbering it on first use; the empty
// string is -1.
func (e *encoder) text(s string) int32 {
	if s == "" {
		return -1
	}
	id, ok := e.strings[s]
	if !ok {
		id = int32(len(e.order))
		e.strings[s] = id
		e.order = append(e.order, s)
	}
	return id
}

// lz4Literals encodes data as one LZ4 block of literals. Every LZ4 decoder
// reads it, and the small resources this package writes gain little from
// matches.
func lz4Literals(data []byte) []byte {
	// The token holds the literal length up to 15; longer lengths continue in
	// bytes of 255 and a remainder.
	out := []byte{byte(min(len(data), 15) << 4)}
	if len(data) >= 15 {
		rest := len(data) - 15
		for ; rest >= 255; rest -= 255 {
			out = append(out, 255)
		}
		out = append(out, byte(rest))
	}
	return append(out, data...)
}

// pad appends zeros to b up to a multiple of alignment.
func pad(b []byte, alignment int) []byte {
	for len(b)%alignment != 0 {
		b = append(b, 0)
	}
	return b
}
