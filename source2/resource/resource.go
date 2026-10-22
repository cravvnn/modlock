// Package resource reads and writes the container of a Source 2 compiled
// resource, such as a .vxml_c layout or a .vents_c entity lump: a header and
// a table of tagged blocks, each holding one part of the resource.
package resource

import (
	"encoding/binary"

	"github.com/pkg/errors"
)

// headerVersion is the container header version every resource this package
// writes carries.
const headerVersion = 12

// Block is one tagged part of a resource, such as its DATA block.
type Block struct {
	// Tag is the block's four-letter tag, such as "DATA" or "RED2".
	Tag string
	// Data is the block's contents.
	Data []byte
}

// errTruncated reports a resource whose table or blocks run past its end.
var errTruncated = errors.New("The resource is truncated")

// Read returns the blocks of a resource in table order. Each block's Data
// aliases resource.
func Read(resource []byte) ([]Block, error) {
	// The block table follows its offset field, and each entry counts its
	// block's offset from the entry's own offset field.
	if len(resource) < 16 {
		return nil, errTruncated
	}
	table := 8 + int(binary.LittleEndian.Uint32(resource[8:]))
	count := int(binary.LittleEndian.Uint32(resource[12:]))
	if count > len(resource)/12 {
		return nil, errTruncated
	}
	blocks := make([]Block, 0, count)
	for index := range count {
		at := table + index*12
		if at < 0 || at+12 > len(resource) {
			return nil, errTruncated
		}
		start := at + 4 + int(binary.LittleEndian.Uint32(resource[at+4:]))
		size := int(binary.LittleEndian.Uint32(resource[at+8:]))
		if start < 0 || size < 0 || start+size > len(resource) {
			return nil, errTruncated
		}
		blocks = append(blocks, Block{Tag: string(resource[at : at+4]), Data: resource[start : start+size]})
	}
	return blocks, nil
}

// Find returns the contents of the first block tagged tag, and whether there
// is one.
func Find(blocks []Block, tag string) ([]byte, bool) {
	for _, block := range blocks {
		if block.Tag == tag {
			return block.Data, true
		}
	}
	return nil, false
}

// ReadBlock reads the contents of the block tagged tag from a resource.
func ReadBlock(resource []byte, tag string) ([]byte, error) {
	blocks, err := Read(resource)
	if err != nil {
		return nil, err
	}
	data, ok := Find(blocks, tag)
	if !ok {
		return nil, errors.Errorf("The resource has no %s block", tag)
	}
	return data, nil
}

// Write encodes blocks as a resource of the given type version, each block on
// a 16-byte boundary after the table, as the resource compiler places them.
func Write(version uint16, blocks []Block) ([]byte, error) {
	// Place each block after the table.
	align := func(offset int) int { return (offset + 15) / 16 * 16 }
	offsets := make([]int, len(blocks))
	end := align(16 + len(blocks)*12)
	size := 16
	for i, block := range blocks {
		if len(block.Tag) != 4 {
			return nil, errors.Errorf("Block tag %q is not four letters", block.Tag)
		}
		offsets[i] = end
		size = end + len(block.Data)
		end = align(size)
	}

	// Write the header: the size, the header and type versions, and where the
	// block table starts.
	out := make([]byte, 16, size)
	binary.LittleEndian.PutUint32(out[0:], uint32(size))
	binary.LittleEndian.PutUint16(out[4:], headerVersion)
	binary.LittleEndian.PutUint16(out[6:], version)
	binary.LittleEndian.PutUint32(out[8:], 8)
	binary.LittleEndian.PutUint32(out[12:], uint32(len(blocks)))

	// Write the table, whose offsets count from each offset field, then the
	// blocks.
	for i, block := range blocks {
		at := len(out) + 4
		out = append(out, block.Tag...)
		out = binary.LittleEndian.AppendUint32(out, uint32(offsets[i]-at))
		out = binary.LittleEndian.AppendUint32(out, uint32(len(block.Data)))
	}
	for i, block := range blocks {
		out = append(out, make([]byte, offsets[i]-len(out))...)
		out = append(out, block.Data...)
	}
	return out, nil
}
