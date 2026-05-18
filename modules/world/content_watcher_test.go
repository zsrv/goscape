package world

import (
	"os"
	"path/filepath"
	"sync"
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

// TestContentWatcher_NonWatchedDirIgnored pins that writes outside the
// 12 canonical subdirs do NOT trigger rebuildReq.
func TestContentWatcher_NonWatchedDirIgnored(t *testing.T) {
	root := t.TempDir()
	s := newTestServer(t)
	_ = startWatcher(t, s, root, "scripts") // only scripts watched
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	target := filepath.Join(root, "node_modules", "foo")
	if err := os.WriteFile(target, []byte("ignored"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case <-s.rebuildReq:
		t.Errorf("rebuildReq fired for non-watched node_modules/")
	case <-time.After(2 * time.Second):
		// good — debounce window elapsed with no event
	}
}

// TestContentWatcher_QuitClosesCleanly pins that closing s.quit makes
// runContentWatcher return promptly (within 1s).
func TestContentWatcher_QuitClosesCleanly(t *testing.T) {
	root := t.TempDir()
	s := newTestServer(t)
	s.cfg.ContentPath = root
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runContentWatcher()
	}()
	time.Sleep(100 * time.Millisecond)

	close(s.quit)
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("watcher did not exit within 1s after s.quit close")
	}
}

// TestNextWatcherBackoff_DoublesThenCaps pins the backoff curve:
// attempt 1 → base, doubling each step, capped at max.
func TestNextWatcherBackoff_DoublesThenCaps(t *testing.T) {
	oldBase, oldMax := watcherBackoffBase, watcherBackoffMax
	watcherBackoffBase = 1 * time.Millisecond
	watcherBackoffMax = 16 * time.Millisecond
	t.Cleanup(func() {
		watcherBackoffBase = oldBase
		watcherBackoffMax = oldMax
	})

	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 1 * time.Millisecond},
		{2, 2 * time.Millisecond},
		{3, 4 * time.Millisecond},
		{4, 8 * time.Millisecond},
		{5, 16 * time.Millisecond}, // cap kicks in
		{6, 16 * time.Millisecond},
		{100, 16 * time.Millisecond},
		{0, 1 * time.Millisecond}, // clamp to attempt >= 1
		{-5, 1 * time.Millisecond},
	}
	for _, tc := range cases {
		got := nextWatcherBackoff(tc.attempt)
		if got != tc.want {
			t.Errorf("nextWatcherBackoff(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}

// withFastBackoff rescales the watcher-backoff vars to milliseconds so
// supervisor tests finish in tens of ms instead of tens of seconds.
// Restored via t.Cleanup; tests MUST NOT use t.Parallel together with
// this helper because the vars are package-level.
func withFastBackoff(t *testing.T) {
	t.Helper()
	oldBase, oldMax, oldReset := watcherBackoffBase, watcherBackoffMax, watcherBackoffResetWindow
	watcherBackoffBase = 1 * time.Millisecond
	watcherBackoffMax = 16 * time.Millisecond
	watcherBackoffResetWindow = 100 * time.Millisecond
	t.Cleanup(func() {
		watcherBackoffBase = oldBase
		watcherBackoffMax = oldMax
		watcherBackoffResetWindow = oldReset
	})
}

// TestContentWatcher_SessionExitsRestart_RetriesUntilQuit pins the
// supervisor's core loop: each watchSessionFn return value of `true`
// causes another invocation; a return of `false` causes the supervisor
// to exit. With s.quit closure, the supervisor exits within slack.
func TestContentWatcher_SessionExitsRestart_RetriesUntilQuit(t *testing.T) {
	s := newTestServer(t)
	withFastBackoff(t)

	var mu sync.Mutex
	count := 0
	const wantRestarts = 3 // session returns true this many times
	stubEntered := make(chan int, wantRestarts+2)

	s.watchSessionFn = func() bool {
		mu.Lock()
		count++
		n := count
		mu.Unlock()
		stubEntered <- n
		if n <= wantRestarts {
			return true // request restart
		}
		// (wantRestarts+1)th call: block until quit, then signal exit.
		<-s.quit
		return false
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runContentWatcher()
	}()

	// Wait for stub to be entered (wantRestarts+1) times in total.
	deadline := time.After(2 * time.Second)
	for i := 0; i < wantRestarts+1; i++ {
		select {
		case <-stubEntered:
			// good
		case <-deadline:
			mu.Lock()
			got := count
			mu.Unlock()
			t.Fatalf("only %d/%d stub entries within 2s", got, wantRestarts+1)
		}
	}

	close(s.quit)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not exit within 2s after s.quit close")
	}

	mu.Lock()
	final := count
	mu.Unlock()
	if final != wantRestarts+1 {
		t.Errorf("watchSessionFn call count = %d, want %d", final, wantRestarts+1)
	}
}
