package scriptc

import (
	"bytes"

	"github.com/pkg/errors"
)

// The section ids the fit rewrites.
const (
	// sectionCustom holds named data, such as debug names.
	sectionCustom = 0
	// sectionType lists the function types.
	sectionType = 1
	// sectionImport lists the imports.
	sectionImport = 2
	// sectionFunction lists each defined function's type.
	sectionFunction = 3
	// sectionGlobal lists the defined globals.
	sectionGlobal = 6
	// sectionExport lists the exports.
	sectionExport = 7
	// sectionStart names the start function.
	sectionStart = 8
	// sectionElement lists the table element segments.
	sectionElement = 9
	// sectionCode holds the function bodies.
	sectionCode = 10
)

// module carries what the fit learns from each section to the sections after
// it. A module's sections come in a fixed order, so every fact is known by
// the time a later section needs it.
type module struct {
	// types is the number of function types; the fit adds two after them.
	types uint32
	// importedFunctions is the number of imported functions before the fit.
	importedFunctions uint32
	// importedGlobals is the number of imported globals.
	importedGlobals uint32
	// definedFunctions is the number of functions the module defines.
	definedFunctions uint32
	// panic is the index of the scriptc.panic import the fit removes.
	panic uint32
	// panicType is the type of scriptc.panic.
	panicType uint32
	// hasPanic reports whether the module imports scriptc.panic.
	hasPanic bool
	// hostCall is the index of the host_call import.
	hostCall uint32
	// hasGlobals reports whether the global section was rewritten.
	hasGlobals bool
	// slots is the index of the global the fit adds, which holds the address
	// of the event export's two out-slots.
	slots uint32
	// exports maps each exported function's name to its index before the fit.
	exports map[string]uint32
	// hasCode reports whether the code section was rewritten.
	hasCode bool
}

// rewrite returns the fitted payload of s, and false for a section the fit
// drops.
func (m *module) rewrite(s section) ([]byte, bool, error) {
	// Rewrite the section by its kind.
	r := &reader{data: s.payload}
	var out []byte
	var err error
	switch s.id {
	case sectionCustom:
		// Debug names index functions by their old numbers.
		if r.name() == "name" {
			return nil, false, nil
		}
		return s.payload, true, nil
	case sectionType:
		out = m.typeSection(r)
	case sectionImport:
		out, err = m.imports(r)
	case sectionFunction:
		out = m.functions(r)
	case sectionGlobal:
		out = m.globals(r)
	case sectionExport:
		out, err = m.exportSection(r)
	case sectionStart:
		out = appendU32(nil, m.function(r.u32()))
	case sectionElement:
		out = m.elements(r)
	case sectionCode:
		out = m.code(r)
	default:
		return s.payload, true, nil
	}

	// Refuse a section that failed to read or held more than it declared.
	if err == nil && r.err == nil && !r.done() {
		r.fail(errors.New("the section is longer than its contents"))
	}
	if err == nil {
		err = r.err
	}
	if err != nil {
		return nil, false, errors.Wrapf(err, "fit section %d", s.id)
	}
	return out, true, nil
}

// typeSection appends the two types the added functions need: the event
// wrapper's (i32) -> i64 at m.types and the init wrapper's () -> () after it.
func (m *module) typeSection(r *reader) []byte {
	m.types = r.u32()
	out := appendU32(nil, m.types+2)
	out = append(out, r.bytes(len(r.data)-r.pos)...)
	return append(out, 0x60, 1, 0x7f, 1, 0x7e, 0x60, 0, 0)
}

// imports renames the host callbacks' module to modlock and removes the
// panic import, which the fit defines instead.
func (m *module) imports(r *reader) ([]byte, error) {
	count := r.u32()
	var out []byte
	for range count {
		// Read one import's names and skip its description.
		module, name := r.name(), r.name()
		described := r.pos
		kind := r.byte()
		var function uint32
		switch kind {
		case 0x00:
			function = r.u32()
		case 0x01:
			r.byte()
			r.limits()
		case 0x02:
			r.limits()
		case 0x03:
			r.bytes(2)
			m.importedGlobals++
		default:
			return nil, errors.Errorf("the module imports %s.%s of kind %d", module, name, kind)
		}
		description := r.data[described:r.pos]

		// Keep WASI, rename the host callbacks and drop the panic sink.
		switch {
		case module != "scriptc":
		case name == "panic" && kind == 0x00:
			m.panic, m.panicType, m.hasPanic = m.importedFunctions, function, true
		case name == "host_call" || name == "host_read":
			if name == "host_call" {
				m.hostCall = m.importedFunctions
			}
			module = "modlock"
		default:
			return nil, errors.Errorf("the module imports scriptc.%s, which the host does not offer", name)
		}
		if kind == 0x00 {
			m.importedFunctions++
		}
		if module == "scriptc" {
			continue
		}
		out = append(appendName(appendName(out, module), name), description...)
	}
	return append(appendU32(nil, count-1), out...), nil
}

// functions declares the three added functions' types after the module's own.
func (m *module) functions(r *reader) []byte {
	// Keep the module's own declarations, then add the panic sink's, the init
	// wrapper's and the event wrapper's.
	m.definedFunctions = r.u32()
	out := appendU32(nil, m.definedFunctions+3)
	out = append(out, r.bytes(len(r.data)-r.pos)...)
	out = appendU32(out, m.panicType)
	out = appendU32(out, m.types+1)
	return appendU32(out, m.types)
}

// globals rewrites the defined globals, or none when r is nil, and adds the
// mutable i32 that holds the event export's out-slots.
func (m *module) globals(r *reader) []byte {
	// Copy each global's type and rewrite its initializer.
	m.hasGlobals = true
	var count uint32
	var out []byte
	if r != nil {
		count = r.u32()
		for range count {
			out = append(out, r.bytes(2)...)
			out = rewriteExpr(r, out, m.function)
		}
	}

	// Add the slots global, set by the init wrapper.
	m.slots = m.importedGlobals + count
	out = append(out, 0x7f, 0x01, 0x41, 0x00, 0x0b)
	return append(appendU32(nil, count+1), out...)
}

// exportSection records the exported functions and replaces the exports with
// the boundary's: memory, _initialize and modlock_event.
func (m *module) exportSection(r *reader) ([]byte, error) {
	// Read every export, keeping the memory's.
	m.exports = make(map[string]uint32)
	var memory []byte
	for range r.u32() {
		name, kind, index := r.name(), r.byte(), r.u32()
		switch {
		case kind == 0x00:
			m.exports[name] = index
		case kind == 0x02 && name == "memory":
			memory = append(appendName(nil, name), kind)
			memory = appendU32(memory, index)
		}
	}
	if memory == nil {
		return nil, errors.New("the module does not export memory")
	}
	for _, name := range []string{"_initialize", "scriptc_alloc", InitExport, EventExport} {
		if _, ok := m.exports[name]; !ok {
			return nil, errors.Errorf("the module does not export %s", name)
		}
	}

	// Export the added init and event wrappers.
	out := append(appendU32(nil, 3), memory...)
	out = appendU32(append(appendName(out, "_initialize"), 0x00), m.added(1))
	return appendU32(append(appendName(out, "modlock_event"), 0x00), m.added(2)), nil
}

// elements rewrites the function indices in each element segment.
func (m *module) elements(r *reader) []byte {
	count := r.u32()
	out := appendU32(nil, count)
	for range count {
		// Copy the segment's mode: its table and offset when it is active.
		start := r.pos
		flags := r.u32()
		out = append(out, r.data[start:r.pos]...)
		if flags&1 == 0 {
			if flags&2 != 0 {
				start = r.pos
				r.u32()
				out = append(out, r.data[start:r.pos]...)
			}
			out = rewriteExpr(r, out, m.function)
		}
		if flags&3 != 0 {
			out = append(out, r.byte())
		}

		// Rewrite its function indices or its initializer expressions.
		items := r.u32()
		out = appendU32(out, items)
		for range items {
			if flags&4 == 0 {
				out = appendU32(out, m.function(r.u32()))
				continue
			}
			out = rewriteExpr(r, out, m.function)
		}
	}
	return out
}

// code rewrites each function body's calls and appends the added functions.
func (m *module) code(r *reader) []byte {
	// Rewrite each body: its locals stay, its instructions are rewritten.
	m.hasCode = true
	count := r.u32()
	out := appendU32(nil, count+3)
	for range count {
		body := &reader{data: r.bytes(int(r.u32()))}
		start := body.pos
		for range body.u32() {
			body.u32()
			body.byte()
		}
		rewritten := rewriteExpr(body, bytes.Clone(body.data[start:body.pos]), m.function)
		if body.err == nil && !body.done() {
			body.fail(errors.New("a function body is longer than its code"))
		}
		r.fail(body.err)
		out = append(appendU32(out, uint32(len(rewritten))), rewritten...)
	}

	// Add the panic sink and the init and event wrappers.
	out = panicBody(m.function(m.hostCall)).encode(out)
	out = initBody(
		m.function(m.exports["_initialize"]),
		m.function(m.exports[InitExport]),
		m.function(m.exports["scriptc_alloc"]),
		m.slots,
	).encode(out)
	return eventBody(m.function(m.exports[EventExport]), m.slots).encode(out)
}

// function maps a function index before the fit to its index after it.
// Removing the panic import moves every later function down by one, and
// calls to the panic import go to the panic sink the fit adds.
func (m *module) function(index uint32) uint32 {
	switch {
	case index < m.panic:
		return index
	case index == m.panic:
		return m.added(0)
	default:
		return index - 1
	}
}

// added returns the index of the n-th function the fit adds: the panic sink,
// the init wrapper and the event wrapper.
func (m *module) added(n uint32) uint32 {
	return m.importedFunctions - 1 + m.definedFunctions + n
}
