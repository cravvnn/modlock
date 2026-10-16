// Package npm downloads pinned npm packages into the user cache: the
// TypeScript checker, the scriptc compiler and the runtime the TypeScript
// library imports.
package npm

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/paralin/modlock/internal/fetch"
	"github.com/pkg/errors"
)

// TsgoVersion is the TypeScript native preview that checks script mods and
// declares the library.
const TsgoVersion = "7.0.0-dev.20260707.2"

// ScriptcVersion is the scriptc compiler that builds script mods to
// WebAssembly.
const ScriptcVersion = "0.3.4"

// Package returns the directory holding the npm package name at version,
// downloading it from the registry into the user cache the first time.
func Package(ctx context.Context, name, version string) (string, error) {
	// Download and unpack the package the first time it is asked for.
	dir, err := fetch.Cache("npm", name+"@"+version)
	if err != nil {
		return "", err
	}
	err = fetch.Install(dir, func(staging string) error {
		// Fetch the registry's archive beside the cache, then unpack it.
		archive := dir + ".tgz"
		defer os.Remove(archive)
		url := "https://registry.npmjs.org/" + name + "/-/" + path.Base(name) + "-" + version + ".tgz"
		if err := fetch.File(ctx, url, archive); err != nil {
			return err
		}
		return extract(archive, staging)
	})
	if err != nil {
		return "", errors.Wrapf(err, "install %s", name)
	}
	return dir, nil
}

// Tsgo returns the TypeScript checker for this system.
func Tsgo(ctx context.Context) (string, error) {
	dir, err := Package(ctx, "@typescript/native-preview-"+host(), TsgoVersion)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lib", executable("tsgo")), nil
}

// Scriptc returns the scriptc compiler for this system and the runtime pack
// it links WebAssembly modules against.
func Scriptc(ctx context.Context) (compiler, runtimePack string, err error) {
	// Name this system's compiler package as scriptc's installer does.
	system := host()
	switch runtime.GOOS {
	case "windows":
		system += "-msvc"
	case "linux":
		system += "-gnu"
	}

	// Install the compiler and the runtime pack.
	dir, err := Package(ctx, "@scriptc/cli-"+system, ScriptcVersion)
	if err != nil {
		return "", "", err
	}
	runtimePack, err = Package(ctx, "@scriptc/runtime-wasm32-wasi", ScriptcVersion)
	if err != nil {
		return "", "", err
	}

	// npm's archive drops the execute bits of the compiler and the tools it
	// runs, which scriptc's installer restores from its toolchain manifest.
	compiler = filepath.Join(dir, "dist", "bin", executable("scriptc"))
	if runtime.GOOS == "windows" {
		return compiler, runtimePack, nil
	}
	for _, tool := range []string{"bin/scriptc", "lib/typescript/lib/tsc", "lib/scriptc-comptime", "lib/llvm/bin/scriptc-llvm-codegen"} {
		if err := os.Chmod(filepath.Join(dir, "dist", filepath.FromSlash(tool)), 0o755); err != nil {
			return "", "", errors.Wrap(err, "install scriptc")
		}
	}
	return compiler, runtimePack, nil
}

// host names this system's platform and architecture as npm does.
func host() string {
	// Map Go's names to npm's where they differ.
	system := map[string]string{"windows": "win32"}[runtime.GOOS]
	if system == "" {
		system = runtime.GOOS
	}
	arch := map[string]string{"amd64": "x64"}[runtime.GOARCH]
	if arch == "" {
		arch = runtime.GOARCH
	}
	return system + "-" + arch
}

// executable returns the file name of the command name on this system.
func executable(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// extract unpacks the regular files under package/ in the npm archive at
// archive into dir, keeping their paths and modes.
func extract(archive, dir string) error {
	return fetch.WalkTar(archive, func(name string, mode fs.FileMode, body io.Reader) error {
		name, ok := strings.CutPrefix(name, "package/")
		if !ok || !filepath.IsLocal(name) {
			return nil
		}
		return write(filepath.Join(dir, filepath.FromSlash(name)), body, mode|0o644)
	})
}

// write copies source into a new file at target.
func write(target string, source io.Reader, mode os.FileMode) error {
	// Create the file and its directory, then copy into it.
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(file, source)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}
