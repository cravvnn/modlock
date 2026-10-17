package scriptc

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/paralin/modlock/internal/fetch"
	"github.com/pkg/errors"
)

// Version is the scriptc release that builds script mods. It comes from the
// paralin/scriptc fork, which carries compiler fixes upstream has not
// released yet.
const Version = "0.3.6-paralin.1"

// asset is one file of the release and its SHA-256.
type asset struct{ name, sum string }

// compilers maps each system to the release asset holding its standalone
// compiler.
var compilers = map[string]asset{
	"darwin/arm64":  {"scriptc-" + Version + "-darwin-arm64.tar.gz", "014222007bb6ee2aaf2907b9cdf9f6a67382298f026be58556db5b8944072a86"},
	"linux/amd64":   {"scriptc-" + Version + "-linux-x64-gnu.tar.gz", "33e315b972c3dd78e123ba3df08c7473dda710b385da26a7fce242d9c5646667"},
	"linux/arm64":   {"scriptc-" + Version + "-linux-arm64-gnu.tar.gz", "ebfbec68dd82fbbd76180e394a19c5ed494546e94a4117d223f3ca9cb6ef57ae"},
	"windows/amd64": {"scriptc-" + Version + "-win32-x64-msvc.tar.gz", "1dcb09e02429f5ee13c61d1974f008dd507cb4a6534e3a5e9b0ec6552f3f4443"},
}

// runtimePack is the release asset holding the wasm32-wasi runtime pack, an
// npm package archive.
var runtimePack = asset{"scriptc-runtime-wasm32-wasi-" + Version + ".tgz", "392a041e601a42c7f3c0b71e08c8ebbe5b8864a4c9600a769b1537537d1d8c23"}

// Compiler returns the scriptc compiler for this system and the runtime pack
// it links WebAssembly modules against.
func Compiler(ctx context.Context) (compiler, pack string, err error) {
	// Install this system's compiler and the runtime pack.
	release, ok := compilers[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		return "", "", errors.Errorf("scriptc publishes no compiler for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	dir, err := install(ctx, "compiler", release, "")
	if err != nil {
		return "", "", err
	}
	pack, err = install(ctx, "runtime-wasm32-wasi", runtimePack, "package/")
	if err != nil {
		return "", "", err
	}
	executable := "scriptc"
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	return filepath.Join(dir, "bin", executable), pack, nil
}

// install returns the directory holding the files under prefix in the
// release asset, downloading it into the user cache the first time.
func install(ctx context.Context, name string, release asset, prefix string) (string, error) {
	dir, err := fetch.Cache("scriptc", Version, name)
	if err != nil {
		return "", err
	}
	err = fetch.Install(dir, func(staging string) error {
		archive := dir + ".tar.gz"
		defer os.Remove(archive)
		url := "https://github.com/paralin/scriptc/releases/download/v" + Version + "/" + release.name
		if err := fetch.File(ctx, url, archive); err != nil {
			return err
		}
		if err := fetch.Verify(archive, release.sum); err != nil {
			return err
		}
		return fetch.UntarTree(archive, prefix, staging)
	})
	if err != nil {
		return "", errors.Wrapf(err, "install scriptc %s", name)
	}
	return dir, nil
}
