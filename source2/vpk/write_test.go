package vpk

import (
	"bytes"
	"maps"
	"testing"
	"testing/fstest"
)

// TestWrite packs a tree with nested, root, extensionless, empty and large
// files, reads it back with Files, and packs it again to the same bytes.
func TestWrite(t *testing.T) {
	// Build a tree that crosses extension and directory groups.
	large := bytes.Repeat([]byte("world"), 1<<16)
	files := map[string][]byte{
		"maps/hl_arena.vmap_c":                        []byte("map"),
		"maps/hl_arena/entities/default_ents.vents_c": []byte("ents"),
		"maps/hl_arena/world_visibility.vvis_c":       large,
		"maps/hl_arena/lightmaps/a.vtex_c":            []byte("a"),
		"maps/hl_arena/lightmaps/b.vtex_c":            {},
		"materials/mymod/wall.vtex_c":                 []byte("wall"),
		"readme":                                      []byte("root"),
		"scripts/hl.v1.vjs_c":                         []byte("dots"),
	}
	tree := fstest.MapFS{}
	for name, body := range files {
		tree[name] = &fstest.MapFile{Data: body}
	}

	// Pack the tree and read every file back.
	var packed bytes.Buffer
	if err := Write(&packed, tree); err != nil {
		t.Fatal(err)
	}
	read, err := Files(packed.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !maps.EqualFunc(read, files, bytes.Equal) {
		t.Fatalf("read %d files back, want %d", len(read), len(files))
	}

	// Pack the unpacked files again; the bytes must not change.
	again := fstest.MapFS{}
	for name, body := range read {
		again[name] = &fstest.MapFile{Data: body}
	}
	var repacked bytes.Buffer
	if err := Write(&repacked, again); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(packed.Bytes(), repacked.Bytes()) {
		t.Fatal("repacking the same files changed the archive")
	}
}
