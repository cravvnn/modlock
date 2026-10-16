package scriptc

import "github.com/pkg/errors"

// errTruncated reports a module that ends inside a section or an entry.
var errTruncated = errors.New("the module ends early")

// reader reads WebAssembly binary fields from data. The first failure sticks:
// later reads return zero values, and err reports it.
type reader struct {
	// data holds the bytes being read.
	data []byte
	// pos is the offset of the next unread byte.
	pos int
	// err is the first read failure.
	err error
}

// done reports whether every byte was read or a read failed.
func (r *reader) done() bool {
	return r.err != nil || r.pos >= len(r.data)
}

// byte reads one byte.
func (r *reader) byte() byte {
	if r.err != nil || r.pos >= len(r.data) {
		r.fail(errTruncated)
		return 0
	}
	b := r.data[r.pos]
	r.pos++
	return b
}

// peek returns the next byte without reading it, or zero at the end.
func (r *reader) peek() byte {
	if r.pos >= len(r.data) {
		return 0
	}
	return r.data[r.pos]
}

// bytes reads the next n bytes without copying them.
func (r *reader) bytes(n int) []byte {
	if r.err != nil || n < 0 || len(r.data)-r.pos < n {
		r.fail(errTruncated)
		return nil
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b
}

// u32 reads an unsigned LEB128 value of at most 32 bits.
func (r *reader) u32() uint32 {
	var v uint32
	for shift := 0; shift < 35; shift += 7 {
		b := r.byte()
		v |= uint32(b&0x7f) << shift
		if b&0x80 == 0 {
			return v
		}
	}
	r.fail(errors.New("the module has an integer longer than 32 bits"))
	return 0
}

// sleb skips a signed LEB128 value of at most 64 bits.
func (r *reader) sleb() {
	for range 10 {
		if r.byte()&0x80 == 0 {
			return
		}
	}
	r.fail(errors.New("the module has an integer longer than 64 bits"))
}

// name reads a length-prefixed UTF-8 name.
func (r *reader) name() string {
	return string(r.bytes(int(r.u32())))
}

// limits skips a table's or a memory's limits.
func (r *reader) limits() {
	hasMax := r.byte()&1 != 0
	r.u32()
	if hasMax {
		r.u32()
	}
}

// fail records err unless a read already failed.
func (r *reader) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

// appendU32 appends v as an unsigned LEB128 value.
func appendU32(b []byte, v uint32) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

// appendS32 appends v as a signed LEB128 value.
func appendS32(b []byte, v int32) []byte {
	for {
		c := byte(v & 0x7f)
		v >>= 7
		if (v == 0 && c&0x40 == 0) || (v == -1 && c&0x40 != 0) {
			return append(b, c)
		}
		b = append(b, c|0x80)
	}
}

// appendName appends name with its length prefix.
func appendName(b []byte, name string) []byte {
	return append(appendU32(b, uint32(len(name))), name...)
}
