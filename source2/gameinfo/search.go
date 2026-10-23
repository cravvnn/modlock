package gameinfo

import (
	"bytes"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

// search is where a block goes in the first SearchPaths section.
type search struct {
	// insertion is the offset a block goes at: the end of the line before the
	// first Game path and the comments right above it, or just inside
	// SearchPaths when that path does not start its own line.
	insertion int
	// roles are the Mod and Write entries a block names because its paths
	// become the first Game path, which would otherwise take them.
	roles [][2]string
}

// readSearch finds where a block goes in source. The engine reads only the
// first SearchPaths section, so later ones are ignored here too.
func readSearch(source []byte) (search, error) {
	// Find the first SearchPaths section.
	found, err := tokens(string(source))
	if err != nil {
		return search{}, err
	}
	open := slices.IndexFunc(found, func(t token) bool { return strings.EqualFold(t.value, "SearchPaths") })
	if open < 0 || open+1 >= len(found) || found[open+1].value != "{" {
		return search{}, errors.New("gameinfo.gi has no SearchPaths section")
	}
	open++

	// Walk its entries for the first Game path and whether any entry names
	// the Mod and Write paths.
	first := -1
	var mod, write bool
	for index := open + 1; index < len(found); {
		key := found[index].value
		if key == "}" {
			return placement(source, found, open, first, mod, write), nil
		}
		if index+1 >= len(found) || key == "{" || found[index+1].value == "}" || found[index+1].value == "{" {
			break
		}
		kinds := strings.Split(strings.ToLower(key), "+")
		if slices.Contains(kinds, "game") && first < 0 {
			first = index
		}
		mod = mod || slices.Contains(kinds, "mod")
		write = write || slices.Contains(kinds, "write")

		// Skip a trailing [condition].
		index += 2
		if index < len(found) && strings.HasPrefix(found[index].value, "[") {
			index++
		}
	}
	return search{}, errors.New("gameinfo.gi's SearchPaths section is incomplete")
}

// placement puts the block on its own line ahead of the first Game path and
// any other tool's marker comment above it, and names the roles that path
// held.
func placement(source []byte, found []token, open, first int, mod, write bool) search {
	// Go before the first Game path when it starts its own line.
	out := search{insertion: found[open].end}
	if first < 0 {
		return out
	}
	if at, ok := lineEndBefore(source, found[first].start); ok && at > found[open].end {
		out.insertion = commentsAbove(source, at, found[open].end)
	}

	// The block's paths come first, so it names the roles the first path held.
	if !mod {
		out.roles = append(out.roles, [2]string{"Mod", found[first+1].value})
	}
	if !write {
		out.roles = append(out.roles, [2]string{"Write", found[first+1].value})
	}
	return out
}

// commentsAbove moves the line end at back over the comment lines directly
// above it, such as another tool's opening marker, stopping at floor.
func commentsAbove(source []byte, at, floor int) int {
	for {
		start := bytes.LastIndexByte(source[:at], '\n') + 1
		line := bytes.TrimSpace(source[start:at])
		previous, ok := lineEndBefore(source, start)
		if !bytes.HasPrefix(line, []byte("//")) || !ok || previous < floor {
			return at
		}
		at = previous
	}
}

// lineEndBefore returns the end of the line before offset, before any
// carriage return, when only whitespace precedes offset on its line.
func lineEndBefore(source []byte, offset int) (int, bool) {
	newline := bytes.LastIndexByte(source[:offset], '\n')
	if newline < 0 || len(bytes.TrimSpace(source[newline+1:offset])) != 0 {
		return 0, false
	}
	if newline > 0 && source[newline-1] == '\r' {
		newline--
	}
	return newline, true
}
