package project

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/paralin/modlock/internal/npm"
	"github.com/paralin/modlock/js"
	"github.com/paralin/modlock/proto/modlock/wasm"
	"github.com/pkg/errors"
)

// ScriptEntry is the bundled JavaScript's file name in a built mod.
const ScriptEntry = "mod.js"

// libraryDir is where a script project's copy of the modlock library's
// declarations lives, so the type checker and editors resolve imports of
// modlock. The compilers resolve modlock to the built library instead.
var libraryDir = filepath.Join("node_modules", "modlock")

// libraryPackage is the declarations' package.json.
const libraryPackage = `{
  "name": "modlock",
  "type": "module",
  "types": "index.d.ts",
  "sideEffects": false
}
`

// scriptMain returns the source entry of a TypeScript or JavaScript project.
func scriptMain(language wasm.Manifest_Language) string {
	if language == wasm.Manifest_LANGUAGE_JAVASCRIPT {
		return "main.js"
	}
	return "main.ts"
}

// buildScript type checks a TypeScript or JavaScript project. A release build
// compiles it with scriptc; a dev build, or a mod scriptc cannot compile,
// bundles it with the library into one script for QuickJS.
func (p *Project) buildScript(ctx context.Context, output io.Writer, mode Mode) error {
	// Install the library's declarations and the type checker.
	library, err := js.Library(ctx)
	if err != nil {
		return err
	}
	if err := installLibrary(library, p.Dir); err != nil {
		return err
	}
	tsgo, err := npm.Tsgo(ctx)
	if err != nil {
		return err
	}

	// Check the types, locating each error.
	var printed bytes.Buffer
	check := exec.CommandContext(ctx, tsgo, "--project", p.Dir, "--pretty", "false")
	check.Dir = p.Dir
	check.Stdout = io.MultiWriter(output, &printed)
	check.Stderr = check.Stdout
	if err := check.Run(); err != nil {
		return &CheckError{
			Err:         errors.New("the mod has type errors"),
			Diagnostics: tsgoDiagnostics(p.Dir, printed.Bytes()),
		}
	}

	// Compile a release build with scriptc when it can.
	if mode == Release {
		module, err := p.compileScriptc(ctx, library, output)
		if err == nil {
			return p.writeBuild(module, wasm.Manifest_RUNTIME_WASM, Entry, ScriptEntry)
		}
		_, _ = fmt.Fprintf(output, "%v; the mod runs on QuickJS\n", err)
	}

	// Bundle the sources and the library into one script.
	result := api.Build(api.BuildOptions{
		EntryPoints:     []string{scriptMain(p.Manifest.GetLanguage())},
		AbsWorkingDir:   p.Dir,
		Bundle:          true,
		Format:          api.FormatIIFE,
		Platform:        api.PlatformNeutral,
		MainFields:      []string{"module", "main"},
		Target:          api.ES2023,
		JSX:             api.JSXAutomatic,
		JSXImportSource: "modlock",
		Alias:           map[string]string{"modlock": library},
		LogLevel:        api.LogLevelSilent,
	})

	// Print and locate each bundling error.
	messages := api.FormatMessages(result.Errors, api.FormatMessagesOptions{Kind: api.ErrorMessage})
	if len(messages) != 0 {
		_, _ = io.WriteString(output, strings.Join(messages, ""))
		return &CheckError{
			Err:         errors.New("the mod does not bundle"),
			Diagnostics: esbuildDiagnostics(p.Dir, result.Errors),
		}
	}

	return p.writeBuild(result.OutputFiles[0].Contents, wasm.Manifest_RUNTIME_QUICKJS, ScriptEntry, Entry)
}

// writeBuild replaces the built mod's entry with content, then the manifest
// that names it, and removes the entry of the other runtime, stale, a
// script mod builds for.
func (p *Project) writeBuild(content []byte, runtime wasm.Manifest_Runtime, entry, stale string) error {
	// Stage the entry so a failed write leaves the previous build in place.
	if err := os.MkdirAll(p.Output(), 0o755); err != nil {
		return errors.Wrap(err, "create the build directory")
	}
	staged := filepath.Join(p.Output(), entry+".tmp")
	if err := os.WriteFile(staged, content, 0o644); err != nil {
		return errors.Wrap(err, "write the build")
	}
	if err := os.Rename(staged, filepath.Join(p.Output(), entry)); err != nil {
		return errors.Wrap(err, "write the build")
	}

	// Name it in the manifest, then drop the entry the manifest stopped naming.
	if err := p.writeBuiltManifest(runtime, entry); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(p.Output(), stale)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errors.Wrap(err, "remove the previous build")
	}
	return nil
}

// installLibrary installs the declarations of the built library in library
// into the script project in dir, removing any other file there.
func installLibrary(library, dir string) error {
	// Gather the declarations and the package that names them.
	files := map[string][]byte{"package.json": []byte(libraryPackage)}
	for _, name := range js.Declarations {
		content, err := os.ReadFile(filepath.Join(library, name))
		if err != nil {
			return errors.Wrap(err, "install the modlock library")
		}
		files[name] = content
	}

	// Write each file that differs, and remove the rest.
	installed := filepath.Join(dir, libraryDir)
	if err := os.MkdirAll(installed, 0o755); err != nil {
		return errors.Wrap(err, "install the modlock library")
	}
	for name, content := range files {
		path := filepath.Join(installed, name)
		if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, content) {
			continue
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return errors.Wrap(err, "install the modlock library")
		}
	}
	entries, err := os.ReadDir(installed)
	if err != nil {
		return errors.Wrap(err, "install the modlock library")
	}
	for _, entry := range entries {
		if _, ok := files[entry.Name()]; !ok {
			if err := os.RemoveAll(filepath.Join(installed, entry.Name())); err != nil {
				return errors.Wrap(err, "install the modlock library")
			}
		}
	}
	return nil
}
