// Package gameinfo edits the search paths in a Source 2 game's gameinfo.gi,
// which several programs share: Steam, mod managers and tools that mount
// content for a while. Each tool's paths sit in a block between its own
// marker comments, and a tool removes exactly the lines it wrote, keeping
// every other byte of the file.
package gameinfo

import (
	"bytes"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

// markers returns the comment lines that open and close the block of the
// tool named name.
func markers(name string) (begin, end string) {
	return "// " + name + " BEGIN", "// " + name + " END"
}

// Mount returns source with the block of the tool named name holding paths as
// Game search paths, replacing the tool's earlier block. The block goes on its
// own line ahead of every other Game path and other tools' blocks, so its
// files win while it is mounted. When no entry names the engine's Mod and
// Write paths, which the engine would otherwise take from the first Game
// path, the block names the path that came first before it.
func Mount(source []byte, name string, paths ...string) ([]byte, error) {
	// Start from the file without the tool's earlier block.
	source, err := Unmount(source, name)
	if err != nil {
		return nil, err
	}
	search, err := readSearch(source)
	if err != nil {
		return nil, err
	}

	// Write the block in the file's own line endings.
	newline := "\n"
	if bytes.Contains(source, []byte("\r\n")) {
		newline = "\r\n"
	}
	begin, end := markers(name)
	var block strings.Builder
	block.WriteString(newline + "\t\t\t" + begin)
	for _, path := range paths {
		block.WriteString(newline + "\t\t\tGame\t\t\t" + path)
	}
	for _, role := range search.roles {
		block.WriteString(newline + "\t\t\t" + role[0] + "\t\t\t\"" + role[1] + "\"")
	}
	block.WriteString(newline + "\t\t\t" + end)
	at := search.insertion
	if at < len(source) && source[at] != '\r' && source[at] != '\n' {
		block.WriteString(newline)
	}
	return slices.Concat(source[:at], []byte(block.String()), source[at:]), nil
}

// Unmount returns source without the block of the tool named name: its
// marker lines and the Game, Mod and Write lines between them. Any other line
// another program put inside the block stays.
func Unmount(source []byte, name string) ([]byte, error) {
	// Find the block, if the file has one.
	text := string(source)
	beginMarker, endMarker := markers(name)
	if !strings.Contains(text, beginMarker) && !strings.Contains(text, endMarker) {
		return source, nil
	}
	if strings.Count(text, beginMarker) != 1 || strings.Count(text, endMarker) != 1 {
		return nil, errors.Errorf("gameinfo.gi has more or fewer than one %s block; it was left unchanged", name)
	}
	begin, end := strings.Index(text, beginMarker), strings.Index(text, endMarker)
	if end < begin {
		return nil, errors.Errorf("gameinfo.gi's %s markers are out of order; it was left unchanged", name)
	}

	// Collect the block's own lines: the markers and each search path entry.
	found, err := tokens(text[begin:end])
	if err != nil {
		return nil, err
	}
	starts := []int{begin, end}
	for index := 0; index+1 < len(found); index += 2 {
		switch strings.ToLower(found[index].value) {
		case "game", "mod", "write":
			starts = append(starts, begin+found[index].start)
		}
	}
	slices.Sort(starts)

	// Remove each line with the newline before it, the reverse of Mount.
	for _, at := range slices.Backward(starts) {
		lineStart := strings.LastIndexByte(text[:at], '\n') + 1
		lineEnd := len(text)
		if newline := strings.IndexByte(text[at:], '\n'); newline >= 0 {
			lineEnd = at + newline
			if text[lineEnd-1] == '\r' {
				lineEnd--
			}
		}
		if lineStart > 0 {
			lineStart--
			if lineStart > 0 && text[lineStart-1] == '\r' {
				lineStart--
			}
		}
		text = text[:lineStart] + text[lineEnd:]
	}
	return []byte(text), nil
}
