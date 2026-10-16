package project

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/paralin/modlock/internal/npm"
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

// compileScriptc compiles the project with scriptc into a module fitted to
// the mod boundary. The compiler's diagnostics go to output; they list what
// keeps a mod from compiling.
func (p *Project) compileScriptc(ctx context.Context, output io.Writer) ([]byte, error) {
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
