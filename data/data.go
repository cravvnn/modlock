// Package data holds the game data file modlock-host reads at start, so a
// distribution can ship it apart from the host executable.
package data

import _ "embed"

// GameSignatures is the text-format GameSignatureFile in game_signatures.txtpb:
// the byte patterns and virtual slots for the current game build.
//
//go:embed game_signatures.txtpb
var GameSignatures []byte
