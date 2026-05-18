package world

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// canonicalContentSubdirs mirrors TS DevThread.ts:100-111 verbatim. The
// watcher recursively adds every existing subdir under each entry.
var canonicalContentSubdirs = []string{
	"maps", "songs", "jingles", "binary", "fonts", "title",
	"scripts", "sprites", "models", "textures", "synth", "wordenc",
}

// debounceWindow is the gap of fs-quiescence required before a rebuild
// fires. Mirrors TS DevThread setTimeout(processChangedFiles, 1000).
const debounceWindow = 1 * time.Second

// Watcher restart backoff parameters. Exposed as var (not const) so
// tests can rescale to ms via t.Cleanup-restored helpers. Production
// code MUST NOT mutate these. Spec §3.1.
var (
	// watcherBackoffBase is the initial restart delay after a session
	// ends (NewWatcher fail, Events/Errors close).
	watcherBackoffBase = 1 * time.Second
	// watcherBackoffMax caps the exponential growth.
	watcherBackoffMax = 30 * time.Second
	// watcherBackoffResetWindow: a session that ran at least this long
	// before ending resets the attempt counter, so the next restart
	// starts from watcherBackoffBase instead of continuing to grow.
	watcherBackoffResetWindow = 60 * time.Second
)

// nextWatcherBackoff returns watcherBackoffBase * 2^(attempt-1), clamped
// to [watcherBackoffBase, watcherBackoffMax]. attempt < 1 is treated as
// attempt == 1.
func nextWatcherBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	// Guard the shift: attempt > 62 would overflow int64 nanoseconds
	// even with sub-second base. Short-circuit to max.
	if attempt > 30 {
		return watcherBackoffMax
	}
	d := watcherBackoffBase << uint(attempt-1)
	if d <= 0 || d > watcherBackoffMax {
		return watcherBackoffMax
	}
	return d
}

// addWatchesRecursive registers every directory under root with the
// fsnotify watcher. Missing root is OK (subset of content not present);
// only logs+returns nil. Other walk errors propagate.
func addWatchesRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipDir
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}
		return w.Add(path)
	})
}

// runContentWatcher is the goroutine entry-point for content-directory
// auto-rebuild. Spawned by world.go startingFn iff ContentPath != ""
// && ContentWatch. Supervises one or more s.watchSessionFn invocations
// (default: s.runWatchSession), restarting on `true` returns with
// exponential backoff (1s → 30s, reset to base after a session runs
// ≥ watcherBackoffResetWindow), and exiting on `false` (s.quit fired).
// Forever-retry; persistent failure is signalled via per-restart WARN
// logs. Spec §3.1.
func (s *Server) runContentWatcher() {
	attempt := 0
	for {
		sessionStart := time.Now()
		if !s.watchSessionFn() {
			return
		}
		ran := time.Since(sessionStart)
		if ran >= watcherBackoffResetWindow {
			attempt = 0
		}
		attempt++
		delay := nextWatcherBackoff(attempt)
		s.log.Warn("contentWatcher: session ended, restarting",
			"attempt", attempt, "delay", delay, "ran", ran)
		select {
		case <-time.After(delay):
		case <-s.quit:
			return
		}
	}
}

// runWatchSession runs one fsnotify.Watcher from init to close. Returns
// true if the session ended due to a transient failure (NewWatcher
// error, Events/Errors close) and the supervisor should restart; false
// if s.quit fired and the supervisor should exit. Spec §3.2.
func (s *Server) runWatchSession() bool {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		s.log.Error("contentWatcher: fsnotify init failed", "err", err)
		return true
	}
	defer w.Close()

	for _, sub := range canonicalContentSubdirs {
		root := filepath.Join(s.cfg.ContentPath, sub)
		if err := addWatchesRecursive(w, root); err != nil {
			s.log.Warn("contentWatcher: add-watches failed", "root", root, "err", err)
			// continue with partial coverage
		}
	}

	var debounceC <-chan time.Time

	for {
		select {
		case <-s.quit:
			return false

		case ev, ok := <-w.Events:
			if !ok {
				s.log.Warn("contentWatcher: Events chan closed")
				return true
			}
			// Dynamic add-on-CREATE-dir: subdirs created mid-session
			// get watched too. Mirrors TS DevThread.trackDir recursion.
			if ev.Op&fsnotify.Create != 0 {
				if info, statErr := os.Stat(ev.Name); statErr == nil && info.IsDir() {
					_ = w.Add(ev.Name)
				}
			}
			debounceC = time.After(debounceWindow)

		case err, ok := <-w.Errors:
			if !ok {
				s.log.Warn("contentWatcher: Errors chan closed")
				return true
			}
			s.log.Warn("contentWatcher: fsnotify error", "err", err)

		case <-debounceC:
			debounceC = nil
			s.dispatchRebuildRequest()
		}
	}
}

// readPackStamp loads the pack-stamp file at path. Returns
// (zero, false, nil) if the file does not exist. Returns
// (zero, false, err) on any other I/O or parse error. Returns
// (parsed, true, nil) on success.
//
// Stamp format: a single line containing the decimal time.UnixNano() of
// the pack-start time, optionally followed by a trailing newline.
func readPackStamp(path string) (time.Time, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return time.Time{}, false, fmt.Errorf("readPackStamp: empty stamp at %s", path)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("readPackStamp: parse %q: %w", path, err)
	}
	return time.Unix(0, n), true, nil
}
