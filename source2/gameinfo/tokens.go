package gameinfo

import (
	"strings"

	"github.com/pkg/errors"
)

// token is one KeyValues token with its offsets in the source, so an edit
// keeps every other byte of the file as it was.
type token struct {
	// value is the unquoted token.
	value string
	// start is the offset of the token's first byte.
	start int
	// end is the offset just past the token's last byte.
	end int
}

// tokens splits Valve's KeyValues text into tokens, skipping whitespace and
// comments. Quoted values unescape only \\ and \", as the engine does.
func tokens(source string) ([]token, error) {
	var out []token
	for index := 0; index < len(source); {
		// Skip whitespace and comments.
		if source[index] <= ' ' {
			index++
			continue
		}
		if strings.HasPrefix(source[index:], "//") {
			end := strings.IndexByte(source[index:], '\n')
			if end < 0 {
				break
			}
			index += end + 1
			continue
		}
		if strings.HasPrefix(source[index:], "/*") {
			end := strings.Index(source[index+2:], "*/")
			if end < 0 {
				return nil, errors.New("gameinfo.gi has an unfinished comment")
			}
			index += end + 4
			continue
		}

		// Braces are structure only outside quotes.
		start := index
		if source[index] == '{' || source[index] == '}' {
			index++
			out = append(out, token{source[start:index], start, index})
			continue
		}
		if source[index] == '"' {
			index++
			var value strings.Builder
			for index < len(source) && source[index] != '"' {
				if source[index] == '\\' && index+1 < len(source) && (source[index+1] == '\\' || source[index+1] == '"') {
					index++
				}
				value.WriteByte(source[index])
				index++
			}
			if index == len(source) {
				return nil, errors.New("gameinfo.gi has an unfinished quoted value")
			}
			index++
			out = append(out, token{value.String(), start, index})
			continue
		}

		// An unquoted value may hold slashes, wildcards and |path| names.
		for index < len(source) && source[index] > ' ' && !strings.ContainsRune("{}\"", rune(source[index])) {
			if strings.HasPrefix(source[index:], "//") || strings.HasPrefix(source[index:], "/*") {
				break
			}
			index++
		}
		out = append(out, token{source[start:index], start, index})
	}
	return out, nil
}
