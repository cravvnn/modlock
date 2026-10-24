package session

import (
	"bytes"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing/fstest"

	"github.com/paralin/modlock/check"
	"github.com/paralin/modlock/panorama"
	"github.com/paralin/modlock/source2/gameinfo"
	s2panorama "github.com/paralin/modlock/source2/panorama"
	"github.com/paralin/modlock/source2/vpk"
	"github.com/pkg/errors"
)

const (
	// rendererMount names the session's block in gameinfo.gi.
	rendererMount = "MODLOCK DEV"
	// rendererDir is the game-relative directory of the renderer package.
	rendererDir = "citadel/modlock_dev"
	// newsLayout is the stock layout the package shadows. The game loads it
	// at startup inside the hideout HUD and keeps it for the whole process,
	// so a script it includes can draw over the HUD in every match.
	newsLayout = "panorama/layout/latest_news.xml"
	// bootScript loads the renderer's layout from the news layout.
	bootScript = "panorama/scripts/modlock/boot.js"
)

// bootSource creates a pass-through panel in the news layout and loads the
// renderer into it.
const bootSource = `(function () {
  var host = $.CreatePanel('Panel', $.GetContextPanel(), 'ModlockUiHost')
  host.hittest = false
  host.BLoadLayout('file://{resources}/layout/modlock/ui.xml', false, false)
})()
`

// Renderer is the built scripts that draw the mods' interfaces in the game.
type Renderer struct {
	// Script draws each mod's tree; it installs as scripts/modlock/ui.js.
	Script []byte
	// Layout is the library a mod's layout includes to read its data and
	// routes; it installs as scripts/modlock/layout.js.
	Layout []byte
}

// mountRenderer mounts a package in the Deadlock installation at gameDir that
// draws the mods' interfaces in the player's game and carries each mod's
// content addon, and returns the call that unmounts it. renderer is the built
// renderer, or nil to leave the interfaces out, and mods are the built mods. The renderer shadows the stock news layout, read from the installed
// game so it follows game updates, with one script added. The package
// outranks every other mounted package while the session runs, and the server
// and the game load it only when they start. A block left by a session that
// did not end cleanly is replaced.
func mountRenderer(gameDir string, renderer *Renderer, mods []string) (func() error, error) {
	// Build the package from the renderer and the mods' content.
	citadel := filepath.Join(gameDir, "game", "citadel")
	files := fstest.MapFS{}
	if renderer != nil {
		compiled, err := rendererFiles(citadel, renderer)
		if err != nil {
			return nil, err
		}
		maps.Copy(files, compiled)
	}
	for _, mod := range mods {
		if err := addContent(files, mod); err != nil {
			return nil, err
		}
	}
	if len(files) == 0 {
		return func() error { return nil }, nil
	}

	// Write the package, then mount it.
	info := filepath.Join(citadel, "gameinfo.gi")
	source, err := os.ReadFile(info)
	if err != nil {
		return nil, err
	}
	var archive bytes.Buffer
	if err := vpk.Write(&archive, files); err != nil {
		return nil, err
	}
	dir := filepath.Join(gameDir, "game", filepath.FromSlash(rendererDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "pak01_dir.vpk"), archive.Bytes(), 0o644); err != nil {
		return nil, err
	}
	mounted, err := gameinfo.Mount(source, rendererMount, rendererDir)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(info, mounted, 0o644); err != nil {
		return nil, err
	}
	return func() error { return unmountRenderer(info) }, nil
}

// rendererFiles compiles the renderer's files: the installed news layout
// with the boot script added, the boot script, and the renderer's layout and
// scripts.
func rendererFiles(citadel string, renderer *Renderer) (fstest.MapFS, error) {
	stock, err := vpk.OpenArchive(filepath.Join(citadel, "pak01_dir.vpk"))
	if err != nil {
		return nil, err
	}
	compiled, err := stock.ReadFile(strings.TrimSuffix(newsLayout, ".xml") + ".vxml_c")
	if err != nil {
		return nil, errors.Wrap(err, "read the game's news layout")
	}
	news, err := s2panorama.Decompile(compiled)
	if err != nil {
		return nil, errors.Wrap(err, "read the game's news layout")
	}
	news, err = withScript(news, "file://{resources}/scripts/modlock/boot.js")
	if err != nil {
		return nil, err
	}
	return s2panorama.CompileTree("modlock_dev", fstest.MapFS{
		newsLayout:                           {Data: []byte(news)},
		bootScript:                           {Data: []byte(bootSource)},
		"panorama/layout/modlock/ui.xml":     {Data: panorama.Layout},
		"panorama/scripts/modlock/ui.js":     {Data: renderer.Script},
		"panorama/scripts/modlock/layout.js": {Data: renderer.Layout},
	})
}

// addContent adds the content addon of the mod built at dir to files: its
// compiled Panorama content and assets, each already under the addon's name.
func addContent(files fstest.MapFS, dir string) error {
	built := os.DirFS(dir)
	for _, kind := range check.AddonDirs() {
		err := fs.WalkDir(built, kind, func(name string, entry fs.DirEntry, err error) error {
			switch {
			case name == kind && errors.Is(err, fs.ErrNotExist):
				return nil
			case err != nil || !entry.Type().IsRegular():
				return err
			}
			data, err := fs.ReadFile(built, name)
			if err != nil {
				return err
			}
			files[name] = &fstest.MapFile{Data: data, Mode: 0o644}
			return nil
		})
		if err != nil {
			return errors.Wrap(err, "read the mod's content")
		}
	}
	return nil
}

// unmountRenderer removes the session's block from the gameinfo.gi at info,
// as it reads now, keeping every change another program made since.
func unmountRenderer(info string) error {
	source, err := os.ReadFile(info)
	if err != nil {
		return err
	}
	unmounted, err := gameinfo.Unmount(source, rendererMount)
	if err != nil || bytes.Equal(unmounted, source) {
		return err
	}
	return os.WriteFile(info, unmounted, 0o644)
}

// withScript adds an include of the script at src to a layout's scripts.
func withScript(layout, src string) (string, error) {
	include := `<include src="` + src + `" />`
	switch {
	case strings.Contains(layout, "<scripts>"):
		return strings.Replace(layout, "<scripts>", "<scripts>\n"+include, 1), nil
	case strings.Contains(layout, "<root>"):
		return strings.Replace(layout, "<root>", "<root>\n<scripts>"+include+"</scripts>", 1), nil
	default:
		return "", errors.New("the game's news layout has no root to add the renderer to")
	}
}
