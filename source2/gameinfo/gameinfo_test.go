package gameinfo

import (
	"bytes"
	"strings"
	"testing"
)

// stockFile is the shape of Deadlock's own gameinfo.gi, with its CRLF line
// endings.
var stockFile = strings.ReplaceAll(`"GameInfo"
{
	FileSystem
	{
		SearchPaths
		{
			Game_Language		citadel_*LANGUAGE*
			Game				citadel
			Game				core
		}
	}
}
`, "\n", "\r\n")

// TestMount mounts and unmounts a block beside the stock paths and beside
// another tool's block.
func TestMount(t *testing.T) {
	// A block before the stock path names the roles it would take, in the
	// file's line endings, and its removal restores every byte.
	source := []byte(stockFile)
	mounted, err := Mount(source, "MODLOCK DEV", "citadel/modlock_dev")
	if err != nil {
		t.Fatal(err)
	}
	want := "\t\t\tGame_Language\t\tcitadel_*LANGUAGE*\r\n\t\t\t// MODLOCK DEV BEGIN\r\n\t\t\tGame\t\t\tcitadel/modlock_dev\r\n\t\t\tMod\t\t\t\"citadel\"\r\n\t\t\tWrite\t\t\t\"citadel\"\r\n\t\t\t// MODLOCK DEV END\r\n\t\t\tGame\t\t\t\tcitadel\r\n"
	if !bytes.Contains(mounted, []byte(want)) || !Mounted(mounted, "MODLOCK DEV") {
		t.Fatalf("mounted:\n%s", mounted)
	}
	if again, err := Mount(mounted, "MODLOCK DEV", "citadel/modlock_dev"); err != nil || !bytes.Equal(again, mounted) {
		t.Fatalf("a second mount changed the file: %v\n%s", err, again)
	}
	if restored, err := Unmount(mounted, "MODLOCK DEV"); err != nil || !bytes.Equal(restored, source) {
		t.Fatalf("unmount left:\n%s\n%v", restored, err)
	}

	// Beside another tool's block, which already names the roles, a block
	// goes after it and before the stock path, and each removal keeps the
	// other block.
	other, err := Mount(source, "HYPERLINE PANORAMA", "citadel/hyperline/ab")
	if err != nil {
		t.Fatal(err)
	}
	both, err := Mount(other, "MODLOCK DEV", "citadel/modlock_dev")
	if err != nil {
		t.Fatal(err)
	}
	hyperline, modlock := bytes.Index(both, []byte("citadel/hyperline/ab")), bytes.Index(both, []byte("citadel/modlock_dev"))
	if hyperline < 0 || modlock < hyperline || bytes.Count(both, []byte("Mod\t")) != 1 {
		t.Fatalf("both:\n%s", both)
	}
	if restored, err := Unmount(both, "MODLOCK DEV"); err != nil || !bytes.Equal(restored, other) {
		t.Fatalf("unmount left:\n%s\n%v", restored, err)
	}
}
