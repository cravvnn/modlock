package panorama

import (
	"strconv"
	"unicode/utf8"
)

// AppendPackedLine appends one binary frame as the line the relay page
// carries into a Panorama page title: "~<sequence>:<packed>\n". The relay
// splits the packed text into acknowledged title parts without reading it,
// and src/packed.ts reads the bytes back.
func AppendPackedLine(line []byte, sequence uint64, frame []byte) []byte {
	line = append(line, '~')
	line = strconv.AppendUint(line, sequence, 10)
	line = append(line, ':')
	line = AppendPacked(line, frame)
	return append(line, '\n')
}

// AppendPacked appends data one character per byte. Printable ASCII other
// than the backslash stands for itself, and every other byte for U+0100 plus
// the byte, so a page title carries it without whitespace, control or escape
// characters the browser could fold. A byte takes at most two UTF-8 bytes.
func AppendPacked(line, data []byte) []byte {
	for _, b := range data {
		if b > ' ' && b < 0x7f && b != '\\' {
			line = append(line, b)
			continue
		}
		line = utf8.AppendRune(line, 0x100+rune(b))
	}
	return line
}
