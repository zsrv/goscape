package world

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

// runContentWatcher is the body of the fsnotify watcher goroutine.
// Spawned by world.go startingFn iff ContentPath != "" && ContentWatch.
// Exits when s.quit is closed. Spec §4.6.
func (s *Server) runContentWatcher() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		s.log.Error("contentWatcher: fsnotify init failed", "err", err)
		return
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
			return

		case ev, ok := <-w.Events:
			if !ok {
				s.log.Warn("contentWatcher: Events chan closed")
				return
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
				return
			}
			s.log.Warn("contentWatcher: fsnotify error", "err", err)

		case <-debounceC:
			debounceC = nil
			s.dispatchRebuildRequest()
		}
	}
}
