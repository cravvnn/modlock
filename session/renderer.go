package session

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing/fstest"

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
	// hyperlineMount names the Hyperline desktop's block, whose startup
	// package also shadows the news layout.
	hyperlineMount = "HYPERLINE PANORAMA"
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

// errHyperlineMounted reports that the Hyperline desktop's game content is
// mounted, so the renderer package stands aside rather than shadow the same
// stock layout.
var errHyperlineMounted = errors.New("the Hyperline desktop's game content is mounted; close the desktop and restart the game to see mod interfaces in it")

// mountRenderer mounts a package in the Deadlock installation at gameDir that
// draws the mods' interfaces in the player's game, and returns the call that
// unmounts it. renderer is the built renderer script. The package shadows the
// stock news layout, read from the installed game so it follows game updates,
// with one script added. A game already running loads it at its next start.
// A block left by a session that did not end cleanly is replaced.
func mountRenderer(gameDir string, renderer []byte) (func() error, error) {
	// Leave the news layout to the Hyperline desktop when it is mounted.
	citadel := filepath.Join(gameDir, "game", "citadel")
	info := filepath.Join(citadel, "gameinfo.gi")
	source, err := os.ReadFile(info)
	if err != nil {
		return nil, err
	}
	if gameinfo.Mounted(source, hyperlineMount) {
		return nil, errHyperlineMounted
	}

	// Build the package from the installed news layout.
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
	files, err := s2panorama.CompileTree("modlock_dev", fstest.MapFS{
		newsLayout:                       {Data: []byte(news)},
		bootScript:                       {Data: []byte(bootSource)},
		"panorama/layout/modlock/ui.xml": {Data: panorama.Layout},
		"panorama/scripts/modlock/ui.js": {Data: renderer},
	})
	if err != nil {
		return nil, err
	}

	// Write the package, then mount it.
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
