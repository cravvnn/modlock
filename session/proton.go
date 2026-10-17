package session

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// requireTool matches the runtime app a tool's toolmanifest.vdf requires.
var requireTool = regexp.MustCompile(`"require_tool_appid"\s+"(\d+)"`)

// installDir matches an app's installation directory in its appmanifest.
var installDir = regexp.MustCompile(`"installdir"\s+"([^"\\/]+)"`)

// Proton is one Steam Proton installation with the prefix modlock-host runs
// in. The prefix is the command line's own: the host must not share the
// game's prefix, because Steam runs the client under its own Proton, whose
// wineserver would wait for this one to exit.
type Proton struct {
	// Dir is the Proton installation directory.
	Dir string
	// Prefix is the compatibility data directory of the host's prefix.
	Prefix string
	// Runtime is the run script of the Steam Linux Runtime the Proton
	// requires, or empty for a Proton that runs on the host.
	Runtime string
	// steam is the Steam installation Proton runs under.
	steam *Steam
}

// FindProton returns MODLOCK_PROTON when set, else the newest numbered Proton
// in the Steam libraries, else Proton Experimental.
func FindProton(steam *Steam) (*Proton, error) {
	// Place the prefix in the user cache.
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, errors.Wrap(err, "locate the cache directory")
	}
	proton := &Proton{Prefix: filepath.Join(cache, "modlock", "proton"), steam: steam}

	// Take the configured Proton, else the newest installed, and the runtime
	// it requires.
	proton.Dir = os.Getenv("MODLOCK_PROTON")
	if proton.Dir == "" {
		if proton.Dir, err = newestProton(steam); err != nil {
			return nil, err
		}
	}
	if proton.Runtime, err = proton.requiredRuntime(); err != nil {
		return nil, err
	}
	return proton, nil
}

// newestProton returns the newest numbered Proton in the Steam libraries,
// else Proton Experimental.
func newestProton(steam *Steam) (string, error) {

	// Rank the installed Protons by version.
	type candidate struct {
		dir     string
		version []int
	}
	var candidates []candidate
	for _, library := range steam.Libraries {
		matches, _ := filepath.Glob(filepath.Join(library, "steamapps", "common", "Proton *", "proton"))
		for _, match := range matches {
			name := strings.TrimPrefix(filepath.Base(filepath.Dir(match)), "Proton ")
			version, ok := protonVersion(name)
			if !ok && name != "- Experimental" {
				continue
			}
			candidates = append(candidates, candidate{dir: filepath.Dir(match), version: version})
		}
	}

	// Take the newest. Experimental has no version, so any numbered release
	// outranks it.
	if len(candidates) == 0 {
		return "", errors.New("cannot find Proton; install Proton in Steam or set MODLOCK_PROTON")
	}
	best := slices.MaxFunc(candidates, func(a, b candidate) int {
		return slices.Compare(a.version, b.version)
	})
	return best.dir, nil
}

// requiredRuntime finds the run script of the Steam Linux Runtime that the
// Proton's tool manifest requires, in any Steam library, as Steam does before
// it starts the tool. A Proton whose manifest names none runs on the host.
func (p *Proton) requiredRuntime() (string, error) {
	// Read the required runtime app.
	manifest, err := os.ReadFile(filepath.Join(p.Dir, "toolmanifest.vdf"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", errors.Wrap(err, "read Proton's tool manifest")
	}
	app := requireTool.FindSubmatch(manifest)
	if app == nil {
		return "", nil
	}

	// Find its installation through its app manifest.
	for _, library := range p.steam.Libraries {
		installed, err := os.ReadFile(filepath.Join(library, "steamapps", "appmanifest_"+string(app[1])+".acf"))
		if err != nil {
			continue
		}
		name := installDir.FindSubmatch(installed)
		if name == nil {
			continue
		}
		run := filepath.Join(library, "steamapps", "common", string(name[1]), "run")
		if info, err := os.Stat(run); err == nil && info.Mode().IsRegular() {
			return run, nil
		}
	}
	return "", errors.Errorf("cannot find the Steam Linux Runtime Proton needs (app %s); launch Deadlock through Steam once", app[1])
}

// protonVersion parses a release name such as "11.0", ignoring a suffix such
// as " (Beta)".
func protonVersion(name string) ([]int, bool) {
	name, _, _ = strings.Cut(name, " ")
	var version []int
	for part := range strings.SplitSeq(name, ".") {
		number, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		version = append(version, number)
	}
	return version, true
}

// Prepare creates the prefix when it does not exist yet. Proton sets a prefix
// up for every verb except runinprefix; getcompatpath does so and exits.
func (p *Proton) Prepare(ctx context.Context) error {
	// Keep a prefix that exists.
	if _, err := os.Stat(filepath.Join(p.Prefix, "pfx")); err == nil {
		return nil
	}

	// Create it with getcompatpath.
	if err := os.MkdirAll(p.Prefix, 0o755); err != nil {
		return errors.Wrap(err, "create the Proton prefix")
	}
	cmd := p.script(ctx, "getcompatpath", "/")
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "create the Proton prefix: %s", output)
	}
	return nil
}

// Command returns a command that runs the Windows program at path in the
// prefix. runinprefix starts it directly; run would wrap it in Proton's
// steam.exe, which leaves its console output silent.
func (p *Proton) Command(ctx context.Context, path string, arguments ...string) *exec.Cmd {
	return p.script(ctx, append([]string{"runinprefix", path}, arguments...)...)
}

// Kill ends every process in the prefix.
func (p *Proton) Kill() {
	cmd := exec.Command(filepath.Join(p.Dir, "files", "bin", "wineserver"), "-k")
	cmd.Env = append(os.Environ(), "WINEPREFIX="+filepath.Join(p.Prefix, "pfx"))
	_ = cmd.Run()
}

// script returns a command that runs the proton script with arguments, inside
// the runtime it requires, whose container supplies the libraries its Wine
// loads, such as the 32-bit loader a host may lack.
func (p *Proton) script(ctx context.Context, arguments ...string) *exec.Cmd {
	// Run the script in its runtime.
	command := append([]string{filepath.Join(p.Dir, "proton")}, arguments...)
	if p.Runtime != "" {
		command = append([]string{p.Runtime, "--"}, command...)
	}

	// Name the game and the prefix as Steam does.
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Env = append(os.Environ(),
		"STEAM_COMPAT_CLIENT_INSTALL_PATH="+p.steam.Root,
		"STEAM_COMPAT_DATA_PATH="+p.Prefix,
		"STEAM_COMPAT_APP_ID="+DeadlockAppID,
		"SteamAppId="+DeadlockAppID,
		"SteamGameId="+DeadlockAppID,
		"WINEDEBUG=-all",
	)
	return cmd
}

// WindowsPath names an absolute Linux path as Wine's Z: drive sees it.
func WindowsPath(path string) string {
	return `Z:` + strings.ReplaceAll(path, "/", `\`)
}
