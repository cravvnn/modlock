package panorama

import (
	"io/fs"
	"strings"
	"testing/fstest"

	"github.com/pkg/errors"
)

// CompileTree compiles the addon content directory tree, such as
// content/citadel_addons/<addon>, into the addon's game files: each layout and
// script under panorama/ becomes its compiled resource, and every other file
// is kept as it is, so already compiled resources ship beside them. A style
// sheet is refused: its compiled form also carries a source map and an image
// table that this package does not write yet, so compile it with Valve's
// resource compiler.
func CompileTree(addon string, tree fs.FS) (fstest.MapFS, error) {
	out := fstest.MapFS{}
	err := fs.WalkDir(tree, ".", func(name string, entry fs.DirEntry, err error) error {
		// Read each regular file.
		if err != nil || !entry.Type().IsRegular() {
			return err
		}
		data, err := fs.ReadFile(tree, name)
		if err != nil {
			return err
		}

		// Compile Panorama sources and keep everything else.
		inPanorama := strings.HasPrefix(name, "panorama/")
		compiled, ok := Compiled(name)
		switch {
		case inPanorama && strings.HasSuffix(name, ".css"):
			return errors.Errorf("%s: style sheets need Valve's resource compiler for now; ship its .vcss_c instead", name)
		case inPanorama && ok:
			if data, err = Compile(addon, name, data); err != nil {
				return err
			}
			name = compiled
		}
		if _, exists := out[name]; exists {
			return errors.Errorf("%s is both compiled from source and present as a file", name)
		}
		out[name] = &fstest.MapFile{Data: data, Mode: 0o644}
		return nil
	})
	return out, err
}
