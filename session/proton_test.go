package session

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRequiredRuntime checks that a Proton runs in the runtime its tool
// manifest requires, found in any library, and on the host without one.
func TestRequiredRuntime(t *testing.T) {
	// Make two libraries and a writer of their files.
	root, library := t.TempDir(), t.TempDir()
	write := func(path, data string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Lay out a Proton that requires the runtime in one library.
	dir := filepath.Join(root, "steamapps", "common", "Proton - Experimental")
	write(filepath.Join(dir, "proton"), "")
	write(filepath.Join(dir, "toolmanifest.vdf"), "\"manifest\"\n{\n  \"commandline\" \"/proton %verb%\"\n  \"require_tool_appid\" \"4183110\"\n}\n")

	// Install the runtime in the other.
	write(filepath.Join(library, "steamapps", "appmanifest_4183110.acf"), "\"AppState\"\n{\n\t\"appid\"\t\t\"4183110\"\n\t\"installdir\"\t\t\"SteamLinuxRuntime_4\"\n}\n")
	run := filepath.Join(library, "steamapps", "common", "SteamLinuxRuntime_4", "run")
	write(run, "")
	steam := &Steam{Root: root, Libraries: []string{root, library}}

	// Find the runtime.
	proton := &Proton{Dir: dir, steam: steam}
	if got, err := proton.requiredRuntime(); err != nil || got != run {
		t.Fatalf("runtime %q, %v; want %q", got, err, run)
	}

	// Run on the host without a manifest.
	if err := os.Remove(filepath.Join(dir, "toolmanifest.vdf")); err != nil {
		t.Fatal(err)
	}
	if got, err := proton.requiredRuntime(); err != nil || got != "" {
		t.Fatalf("runtime %q, %v; want none", got, err)
	}
}
