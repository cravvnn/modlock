package check

import (
	"encoding/xml"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/paralin/modlock/proto/modlock/wasm"
	"github.com/paralin/modlock/source2/panorama"
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

// Scripted returns the first file of dist's client content that runs code in
// a player's game, or "" when none does. A script runs code, and so does a
// layout that loads a script, holds one inline or handles an event. A layout
// this package cannot read counts as running code, so a publishing service
// can hold scripted content for creators it trusts.
func Scripted(dist fs.FS) (string, error) {
	var found string
	err := fs.WalkDir(dist, ContentDir, func(name string, entry fs.DirEntry, err error) error {
		// A mod without client content has no directory.
		if name == ContentDir && errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil || entry.IsDir() {
			return err
		}

		// Judge each script and layout; other resources hold data.
		switch path.Ext(name) {
		case ".vjs_c", ".vts_c", ".js", ".ts":
			found = name
		case ".vxml_c":
			compiled, err := fs.ReadFile(dist, name)
			if err != nil {
				return err
			}
			if layoutScripted(compiled) {
				found = name
			}
		}
		if found != "" {
			return fs.SkipAll
		}
		return nil
	})
	return found, err
}

// layoutScripted reports whether a compiled layout loads a script, holds one
// inline or handles an event, or cannot be read.
func layoutScripted(compiled []byte) bool {
	source, err := panorama.Decompile(compiled)
	if err != nil {
		return true
	}
	decoder := xml.NewDecoder(strings.NewReader(source))
	decoder.Strict = false
	depth, scripts := 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return false
		}
		if err != nil {
			return true
		}
		switch token := token.(type) {
		case xml.StartElement:
			// Anything inside <scripts> loads a script.
			depth++
			switch {
			case scripts != 0, strings.EqualFold(token.Name.Local, "script"):
				return true
			case strings.EqualFold(token.Name.Local, "scripts"):
				scripts = depth
			}
			for _, attr := range token.Attr {
				if len(attr.Name.Local) > 2 && strings.EqualFold(attr.Name.Local[:2], "on") {
					return true
				}
			}
		case xml.EndElement:
			if depth == scripts {
				scripts = 0
			}
			depth--
		}
	}
}
