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
	// stock citadel path, or just inside SearchPaths when that path does not
	// start its own line.
	insertion int
	// roles are the Mod and Write entries a block names because its paths
	// would otherwise become the first Game path and take them.
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

	// Walk its entries for the first Game path, the stock one, and whether
	// any entry names the Mod and Write paths.
	var first string
	firstIndex, stockIndex := -1, -1
	var mod, write bool
	for index := open + 1; index < len(found); {
		key := found[index].value
		if key == "}" {
			return placement(source, found, open, first, firstIndex, stockIndex, mod, write), nil
		}
		if index+1 >= len(found) || key == "{" || found[index+1].value == "}" || found[index+1].value == "{" {
			break
		}
		kinds := strings.Split(strings.ToLower(key), "+")
		value := found[index+1].value
		if slices.Contains(kinds, "game") && first == "" {
			first, firstIndex = value, index
		}
		if slices.Contains(kinds, "game") && stockIndex < 0 && stock(value) {
			stockIndex = index
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

// placement puts the block on its own line before the stock path when that
// path starts a line, and names the roles the block would otherwise take.
func placement(source []byte, found []token, open int, first string, firstIndex, stockIndex int, mod, write bool) search {
	// Go before the stock path when it starts its own line.
	out := search{insertion: found[open].end}
	leading := true
	if stockIndex >= 0 {
		if at, ok := lineEndBefore(source, found[stockIndex].start); ok && at > found[open].end {
			out.insertion = at
			leading = stockIndex == firstIndex
		}
	}

	// A block ahead of every Game path names the roles the first path held.
	if leading && first != "" && !mod {
		out.roles = append(out.roles, [2]string{"Mod", first})
	}
	if leading && first != "" && !write {
		out.roles = append(out.roles, [2]string{"Write", first})
	}
	return out
}

// stock reports whether a Game path names the game's own citadel content.
func stock(value string) bool {
	value = strings.TrimSuffix(strings.ToLower(strings.ReplaceAll(value, `\`, "/")), "/")
	return value == "citadel" || value == "|gameinfo_path|."
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
