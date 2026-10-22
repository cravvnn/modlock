// Package vpktest writes small Valve archives for tests.
package vpktest

import (
	"bytes"
	"testing/fstest"

	"github.com/paralin/modlock/source2/vpk"
)

// Archive packs an archive of paths, each holding three bytes.
func Archive(paths ...string) []byte {
	contents := make(map[string][]byte, len(paths))
	for _, resource := range paths {
		contents[resource] = []byte("abc")
	}
	return Files(contents)
}

// Files packs contents into an archive with vpk.Write.
func Files(contents map[string][]byte) []byte {
	// Place the contents in a tree, then pack it.
	tree := fstest.MapFS{}
	for name, body := range contents {
		tree[name] = &fstest.MapFile{Data: body}
	}
	var packed bytes.Buffer
	if err := vpk.Write(&packed, tree); err != nil {
		panic(err)
	}
	return packed.Bytes()
}

// Map encodes the archive of the map name: its compiled map and the extra
// paths of the map's own files.
func Map(name string, extra ...string) []byte {
	return Archive(append([]string{"maps/" + name + ".vmap_c"}, extra...)...)
}
