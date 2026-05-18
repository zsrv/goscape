package world

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// startWatcher seeds s.cfg.ContentPath = root, pre-creates the requested
// canonical subdirs, spawns runContentWatcher in a goroutine, and
// returns a done-chan + cleanup that closes s.quit and waits for exit.
func startWatcher(t *testing.T, s *Server, root string, subs ...string) <-chan struct{} {
	t.Helper()
	s.cfg.ContentPath = root
	for _, sub := range subs {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s/%s: %v", root, sub, err)
		}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runContentWatcher()
	}()
	t.Cleanup(func() {
		close(s.quit)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("contentWatcher did not exit within 2s after s.quit close")
		}
	})
	return done
}

// TestContentWatcher_FileWrite_TriggersRebuildAfterDebounce pins the
// single-file-edit path: write one file under scripts/ → exactly one
// rebuildReq arrives within ~debounce + slack.
func TestContentWatcher_FileWrite_TriggersRebuildAfterDebounce(t *testing.T) {
	root := t.TempDir()
	s := newTestServer(t)
	_ = startWatcher(t, s, root, "scripts")

	// Give the watcher a moment to register watches before we write.
	// The watcher's first Events read happens after addWatchesRecursive
	// returns; on Linux inotify is synchronous, but we sleep briefly to
	// avoid a race on slower CI.
	time.Sleep(100 * time.Millisecond)

	target := filepath.Join(root, "scripts", "foo.rs2")
	if err := os.WriteFile(target, []byte("[proc,foo]\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case <-s.rebuildReq:
		// good
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive on rebuildReq within 3s")
	}
}

// TestContentWatcher_BurstCoalesces pins that a rapid burst of file
// writes collapses into a single rebuildReq via the 1s debounce.
func TestContentWatcher_BurstCoalesces(t *testing.T) {
	root := t.TempDir()
	s := newTestServer(t)
	_ = startWatcher(t, s, root, "scripts")
	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 10; i++ {
		target := filepath.Join(root, "scripts", "foo.rs2")
		if err := os.WriteFile(target, []byte{byte(i)}, 0o644); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		time.Sleep(20 * time.Millisecond) // well inside the 1s window
	}

	// Drain the first rebuildReq within debounce + slack.
	select {
	case <-s.rebuildReq:
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive first rebuildReq")
	}

	// Window after the burst: no further rebuildReqs should fire (chan
	// stays empty for the slack window).
	select {
	case <-s.rebuildReq:
		t.Errorf("burst coalesce broken — second rebuildReq fired")
	case <-time.After(500 * time.Millisecond):
		// good
	}
}

// TestContentWatcher_NewSubdir_AddedToWatch pins that subdirs created
// after the watcher started are also watched: write into a freshly-
// created subdir triggers a rebuildReq.
func TestContentWatcher_NewSubdir_AddedToWatch(t *testing.T) {
	root := t.TempDir()
	s := newTestServer(t)
	_ = startWatcher(t, s, root, "scripts")
	time.Sleep(100 * time.Millisecond)

	newDir := filepath.Join(root, "scripts", "newdir")
	if err := os.Mkdir(newDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Drain the rebuildReq the mkdir itself triggered (it's an event on
	// the parent scripts/ dir).
	select {
	case <-s.rebuildReq:
	case <-time.After(3 * time.Second):
		t.Fatal("no rebuildReq after mkdir")
	}

	// Give the watcher a window to add the new dir to its watch list
	// (the dynamic-add happens inside the same select branch that reset
	// debounceC; the add is synchronous so this sleep is conservative).
	time.Sleep(100 * time.Millisecond)

	target := filepath.Join(newDir, "x.rs2")
	if err := os.WriteFile(target, []byte("[proc,x]\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case <-s.rebuildReq:
		// good — write inside newly-watched subdir triggered the debounce
	case <-time.After(3 * time.Second):
		t.Fatal("write under newly-created subdir did not trigger rebuildReq; dynamic add broken")
	}
}
