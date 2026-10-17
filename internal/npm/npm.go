// Package npm downloads pinned npm packages into the user cache: the
// TypeScript checker and the runtime the TypeScript library imports.
package npm

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/paralin/modlock/internal/fetch"
	"github.com/pkg/errors"
)

// TsgoVersion is the TypeScript native preview that checks script mods and
// declares the library.
const TsgoVersion = "7.0.0-dev.20260707.2"

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
		return fetch.UntarTree(archive, "package/", staging)
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
