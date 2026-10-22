package check

import (
	"io/fs"
	"regexp"
	"strings"

	"github.com/paralin/modlock/proto/modlock/wasm"
	"github.com/pkg/errors"
)

// ContentDir is the built mod's directory of client content: the panorama
// directory of the mod's content addon, which a player's game mounts while it
// plays the mod.
const ContentDir = "panorama"

// versionPattern matches a version that can name a directory.
var versionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+-]{0,63}$`)

// Version reports whether version is a valid mod version.
func Version(version string) error {
	switch {
	case version == "":
		return errors.New("mod.json names no version")
	case !versionPattern.MatchString(version):
		return errors.Errorf("mod.json's version %q must be at most 64 letters, digits, '.', '+' and '-', starting with a letter or digit", version)
	}
	return nil
}

// Addon returns the name of the content addon a release of the mod ships: its
// slug and version. Panorama keeps each layout it loads for the life of the
// game, so every release names its own, and a slug belongs to one mod.
func Addon(manifest *wasm.Manifest) string {
	return manifest.GetSlug() + "-" + manifest.GetVersion()
}

// Content checks that every file in dist's content directory lies in the
// addon's directory of its kind, panorama/<kind>/<addon>/, so one mod's
// content never replaces another's or the game's.
func Content(dist fs.FS, addon string) error {
	return fs.WalkDir(dist, ContentDir, func(name string, entry fs.DirEntry, err error) error {
		// A mod without client content has no directory.
		if name == ContentDir && errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil || entry.IsDir() {
			return err
		}

		// Each file sits below its kind and the addon's name.
		parts := strings.SplitN(name, "/", 4)
		if len(parts) < 4 || parts[2] != addon {
			return errors.Errorf("%s is outside the addon's directories; client content goes in %s/<kind>/%s/", name, ContentDir, addon)
		}
		return nil
	})
}
