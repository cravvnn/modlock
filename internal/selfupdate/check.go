package selfupdate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/paralin/modlock/internal/fetch"
)

// checkEvery is how long a cached answer to Latest stays fresh.
const checkEvery = 24 * time.Hour

// checkTimeout bounds one refresh, so a slow network never holds a command.
const checkTimeout = 5 * time.Second

// Check calls found with the newest release when it is later than current. It
// answers at once from the answer cached in the user cache, which holds the
// newest tag and, as its modification time, when it was learned. When that is
// older than a day it also asks GitHub in the background and calls found
// again if the fresh answer is later still. found runs at most twice, on any
// goroutine, and never after ctx ends. Every failure is dropped: the check
// must never slow or fail the command that runs it.
func Check(ctx context.Context, current string, found func(latest string)) {
	// Answer from the cache.
	path, err := fetch.Cache("latest")
	if err != nil || !Released(current) {
		return
	}
	cached := ""
	if data, err := os.ReadFile(path); err == nil {
		cached = strings.TrimSpace(string(data))
	}
	if Newer(cached, current) {
		found(cached)
	}

	// Refresh a stale answer in the background.
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < checkEvery {
		return
	}
	go func() {
		// Ask for the newest release, and keep the answer for a day.
		refreshCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		defer cancel()
		latest, err := Latest(refreshCtx)
		if err != nil {
			return
		}
		Remember(latest)

		// Report it when it is news.
		if ctx.Err() == nil && Newer(latest, current) && Newer(latest, cached) {
			found(latest)
		}
	}()
}

// Remember caches latest as the newest release, learned now.
func Remember(latest string) {
	path, err := fetch.Cache("latest")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(latest+"\n"), 0o644)
}
