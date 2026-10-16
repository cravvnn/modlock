package scriptc

import "github.com/pkg/errors"

// rewriteExpr copies one expression from r to out through its final end,
// replacing each function index an instruction names with function(index).
// It decodes the instructions scriptc emits: the core set with sign
// extension, saturating truncation, bulk memory, reference types and tail
// calls. Any other instruction fails the read, so no index is left stale.
func rewriteExpr(r *reader, out []byte, function func(uint32) uint32) []byte {
	for depth := 0; !r.done(); {
		// Copy the instruction up to an index it names, then the new index.
		start := r.pos
		op := r.byte()
		switch {
		case op == 0x10 || op == 0x12 || op == 0xd2: // call, return_call, ref.func
			// The linker may pad an index, so copy only the bytes before it.
			at := r.pos
			index := r.u32()
			out = append(out, r.data[start:at]...)
			out = appendU32(out, function(index))
			continue
		case op == 0x02 || op == 0x03 || op == 0x04: // block, loop, if
			depth++
			r.blockType()
		case op == 0x0b: // end
			if depth == 0 {
				return append(out, r.data[start:r.pos]...)
			}
			depth--
		case op == 0x0c || op == 0x0d || op == 0x20 || op == 0x21 || op == 0x22 ||
			op == 0x23 || op == 0x24 || op == 0x25 || op == 0x26 || op == 0x3f ||
			op == 0x40 || op == 0xd0:
			// br, br_if, local and global access, table.get and table.set,
			// memory.size, memory.grow and ref.null take one index.
			r.u32()
		case op == 0x0e: // br_table
			for range r.u32() + 1 {
				r.u32()
			}
		case op == 0x11 || op == 0x13: // call_indirect, return_call_indirect
			r.u32()
			r.u32()
		case op == 0x1c: // select with types
			r.bytes(int(r.u32()))
		case op >= 0x28 && op <= 0x3e: // loads and stores
			r.memarg()
		case op == 0x41 || op == 0x42: // i32.const, i64.const
			r.sleb()
		case op == 0x43: // f32.const
			r.bytes(4)
		case op == 0x44: // f64.const
			r.bytes(8)
		case op == 0xfc:
			r.prefixed()
		case op <= 0x01 || op == 0x05 || op == 0x0f || op == 0x1a || op == 0x1b ||
			(op >= 0x45 && op <= 0xc4) || op == 0xd1:
			// unreachable, nop, else, return, drop, select, the numeric
			// instructions and ref.is_null take no immediates.
		default:
			r.fail(errors.Errorf("the module uses instruction 0x%02x, which the fit does not decode", op))
		}
		out = append(out, r.data[start:r.pos]...)
	}
	r.fail(errTruncated)
	return out
}

// blockType skips a block's type: empty, one value type, or a type index.
func (r *reader) blockType() {
	if b := r.peek(); b == 0x40 || (b >= 0x6f && b <= 0x7f) {
		r.byte()
		return
	}
	r.sleb()
}

// memarg skips a load's or a store's alignment, memory and offset.
func (r *reader) memarg() {
	if r.u32()&0x40 != 0 {
		r.u32()
	}
	r.u32()
}

// prefixed skips the immediates of an instruction under the 0xfc prefix:
// saturating truncation, bulk memory and table operations.
func (r *reader) prefixed() {
	switch op := r.u32(); {
	case op <= 7: // saturating truncation
	case op == 8 || op == 10 || op == 12 || op == 14: // memory.init, memory.copy, table.init, table.copy
		r.u32()
		r.u32()
	case op <= 17: // data.drop, memory.fill, elem.drop, table.grow, table.size, table.fill
		r.u32()
	default:
		r.fail(errors.Errorf("the module uses instruction 0xfc %d, which the fit does not decode", op))
	}
}
