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
	// ASYNC-1 (Arc 18): cap consecutive restart attempts so a persistent
	// fs/permission failure can't spin the supervisor goroutine forever.
	// Once exceeded, the supervisor logs and exits; the server keeps
	// running with content-hot-reload disabled. Dev-only path; production
	// has ContentWatch=false by default.
	watcherMaxAttempts = 100
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
		// ASYNC-1: bail out if persistent failures exhaust the attempt
		// ceiling. Server keeps running; only content-hot-reload disabled.
		if attempt > watcherMaxAttempts {
			s.logContent.Error("contentWatcher: max restart attempts exceeded, giving up",
				"attempts", attempt-1, "ran", ran)
			return
		}
		delay := nextWatcherBackoff(attempt)
		s.logContent.Warn("contentWatcher: session ended, restarting",
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
		s.logContent.Error("contentWatcher: fsnotify init failed", "err", err)
		return true
	}
	defer w.Close()

	for _, sub := range canonicalContentSubdirs {
		root := filepath.Join(s.cfg.ContentPath, sub)
		if err := addWatchesRecursive(w, root); err != nil {
			s.logContent.Warn("contentWatcher: add-watches failed", "root", root, "err", err)
			// continue with partial coverage
		}
	}

	s.maybeReplayDispatch()

	var debounceC <-chan time.Time

	for {
		select {
		case <-s.quit:
			return false

		case ev, ok := <-w.Events:
			if !ok {
				s.logContent.Warn("contentWatcher: Events chan closed")
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
				s.logContent.Warn("contentWatcher: Errors chan closed")
				return true
			}
			s.logContent.Warn("contentWatcher: fsnotify error", "err", err)

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

// writePackStamp atomically writes t.UnixNano() to path via a sibling
// `.tmp` file + os.Rename. Crash mid-write leaves the previous stamp (or
// no stamp) intact; the orphaned `.tmp` will be overwritten on the next
// successful write.
func writePackStamp(path string, t time.Time) error {
	tmp := path + ".tmp"
	data := []byte(strconv.FormatInt(t.UnixNano(), 10) + "\n")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writePackStamp: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("writePackStamp: rename: %w", err)
	}
	return nil
}

// maybeReplayDispatch reads CachePath/.pack-stamp and scans the content
// tree against it. If the stamp is missing, unreadable, or corrupt, or
// if any content file has mtime newer than the stamp, calls
// dispatchRebuildRequest exactly once. Returns true iff dispatch fired.
//
// Caller invariant: must be called from inside runWatchSession AFTER
// addWatchesRecursive has succeeded, so any edit that races the scan is
// also caught by the active fsnotify watcher (dispatchRebuildRequest's
// pending-flag coalescing dedupes). Scan walk errors result in no
// dispatch — we don't trigger a rebuild we can't justify. Stamp read
// errors result in dispatch — unknown cache state is safer to rebuild.
func (s *Server) maybeReplayDispatch() bool {
	stampPath := filepath.Join(s.cfg.CachePath, ".pack-stamp")
	ref, ok, err := readPackStamp(stampPath)
	if err != nil {
		s.logContent.Warn("contentWatcher: pack stamp unreadable, treating as stale",
			"path", stampPath, "err", err)
		s.dispatchRebuildRequest()
		return true
	}
	if !ok {
		s.logContent.Info("contentWatcher: no pack stamp, triggering replay rebuild",
			"path", stampPath)
		s.dispatchRebuildRequest()
		return true
	}
	newer, err := scanContentNewerThan(s.cfg.ContentPath, canonicalContentSubdirs, ref)
	if err != nil {
		s.logContent.Warn("contentWatcher: replay scan failed, skipping replay",
			"err", err)
		return false
	}
	if newer {
		s.logContent.Info("contentWatcher: detected post-stamp edits, triggering replay rebuild",
			"stamp", ref)
		s.dispatchRebuildRequest()
		return true
	}
	return false
}

// scanContentNewerThan walks each existing subdir under root and returns
// true on the first regular file whose ModTime() is strictly after ref.
// Missing subdirs are skipped (mirrors addWatchesRecursive's
// fs.ErrNotExist tolerance). Walk errors other than fs.ErrNotExist
// propagate as a returned error. Returns false if no qualifying file is
// found.
//
// Used by maybeReplayDispatch to detect edits made outside an active
// fsnotify session (cold boot or supervisor down-window).
func scanContentNewerThan(root string, subdirs []string, ref time.Time) (bool, error) {
	for _, sub := range subdirs {
		var newer bool
		err := filepath.WalkDir(filepath.Join(root, sub), func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if errors.Is(walkErr, fs.ErrNotExist) {
					return fs.SkipDir
				}
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.ModTime().After(ref) {
				newer = true
				return fs.SkipAll
			}
			return nil
		})
		if err != nil {
			return false, err
		}
		if newer {
			return true, nil
		}
	}
	return false, nil
}
