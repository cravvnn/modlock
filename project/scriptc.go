package project

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/paralin/modlock/internal/npm"
	"github.com/paralin/modlock/js"
	"github.com/paralin/modlock/scriptc"
	"github.com/pkg/errors"
)

// scriptcDir is where a script project's scriptc build keeps its entry, its
// profile and its output, beside the library it imports.
var scriptcDir = filepath.Join("node_modules", ".scriptc")

// scriptcEntry is the build's entry. It loads the mod's main module, which
// registers the mod's handlers, and exports the library's event dispatch,
// since scriptc exports only functions the entry declares. The host
// callbacks bind only in the entry, so it hands them to the library, each in
// a closure: scriptc 0.3.4 traps on a callback passed as a bare value.
const scriptcEntry = `import '../../%s'
import { event as dispatch } from 'modlock/scriptc'

declare function host_call(data: Uint8Array): number
declare function host_read(data: Uint8Array): void

export function event(size: number): Uint8Array {
  return dispatch(
    size,
    (data) => host_call(data),
    (data) => host_read(data),
  )
}
`

// scriptcConfig is the TypeScript configuration of the scriptc build. It
// extends the project's, and maps the library and the protobuf runtime to
// their TypeScript source, which scriptc compiles with the mod in place of
// their bundles and declarations.
type scriptcConfig struct {
	Extends         string `json:"extends,omitempty"`
	CompilerOptions struct {
		// NoUncheckedIndexedAccess is off: the one program spans the
		// protobuf runtime, written without it.
		NoUncheckedIndexedAccess bool                `json:"noUncheckedIndexedAccess"`
		Paths                    map[string][]string `json:"paths"`
	} `json:"compilerOptions"`
}

// writeScriptcConfig writes the scriptc build's TypeScript configuration into
// dir, mapping the library to its source in library.
func (p *Project) writeScriptcConfig(dir, library string) error {
	var config scriptcConfig
	if _, err := os.Stat(filepath.Join(p.Dir, "tsconfig.json")); err == nil {
		config.Extends = "../../tsconfig.json"
	}
	source := filepath.Join(library, js.Scriptc)
	index := filepath.Join(source, "js", "src", "index.ts")
	protobuf := filepath.Join(source, js.ScriptcProtobuf)
	config.CompilerOptions.Paths = map[string][]string{
		"modlock":                   {index},
		"modlock/jsx-runtime":       {index},
		"modlock/entities":          {filepath.Join(source, "js", "src", "entities.ts")},
		"modlock/console":           {filepath.Join(source, "js", "src", "console.ts")},
		"modlock/scriptc":           {filepath.Join(source, "js", "src", "scriptc.ts")},
		"@aptre/protobuf-es-lite":   {filepath.Join(protobuf, "index.ts")},
		"@aptre/protobuf-es-lite/*": {filepath.Join(protobuf, "*.ts")},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "tsconfig.json"), data, 0o644)
}

// compileScriptc compiles the project with scriptc into a module fitted to
// the mod boundary. The compiler's diagnostics go to output; they list what
// keeps a mod from compiling.
func (p *Project) compileScriptc(ctx context.Context, library string, output io.Writer) ([]byte, error) {
	// Install the compiler, then write the entry and the profile.
	compiler, runtimePack, err := npm.Scriptc(ctx)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(p.Dir, scriptcDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, errors.Wrap(err, "write the scriptc entry")
	}
	entry := fmt.Appendf(nil, scriptcEntry, scriptMain(p.Manifest.GetLanguage()))
	if err := os.WriteFile(filepath.Join(dir, "entry.ts"), entry, 0o644); err != nil {
		return nil, errors.Wrap(err, "write the scriptc entry")
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.json"), scriptc.Profile("entry.ts"), 0o644); err != nil {
		return nil, errors.Wrap(err, "write the scriptc profile")
	}
	if err := p.writeScriptcConfig(dir, library); err != nil {
		return nil, errors.Wrap(err, "write the scriptc configuration")
	}

	// scriptc 0.3.4 judges a bare import as an npm package whenever
	// node_modules holds its declarations, even when the configuration maps it
	// to source, and refuses a package that ships no runtime. Hide the
	// library's declarations, which the type checker has already read, while
	// it compiles; the next build reinstalls them if putting them back fails.
	installed := filepath.Join(p.Dir, libraryDir)
	hidden := filepath.Join(dir, "modlock")
	if err := os.RemoveAll(hidden); err != nil {
		return nil, errors.Wrap(err, "hide the library declarations")
	}
	if err := os.Rename(installed, hidden); err != nil {
		return nil, errors.Wrap(err, "hide the library declarations")
	}
	defer func() { _ = os.Rename(hidden, installed) }()

	// Compile a WASI reactor, printing what keeps the mod from compiling.
	build := exec.CommandContext(ctx, compiler, "build", "--lib", "--profile", "profile.json", "-o", Entry)
	build.Dir = dir
	build.Env = append(os.Environ(), "SCRIPTC_TARGET=wasm32-wasi", "SCRIPTC_RUNTIME_PACK="+runtimePack)
	build.Stdout = io.Discard
	build.Stderr = output
	if err := build.Run(); err != nil {
		return nil, errors.Wrap(err, "scriptc cannot compile the mod")
	}

	// Fit the reactor to the mod boundary.
	built, err := os.ReadFile(filepath.Join(dir, Entry))
	if err != nil {
		return nil, errors.Wrap(err, "read the scriptc build")
	}
	return scriptc.Fit(built)
}
