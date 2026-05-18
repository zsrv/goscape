package world

import (
	"errors"
	"io/fs"
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

// TestContentWatcher_BackoffDoubles pins the exponential-backoff curve.
// With base=1ms and max=16ms, the 6 inter-call deltas for 7 calls are
// [1, 2, 4, 8, 16, 16] ms. Cap kicks in at attempt 5.
func TestContentWatcher_BackoffDoubles(t *testing.T) {
	s := newTestServer(t)
	withFastBackoff(t)

	const wantCalls = 7
	var mu sync.Mutex
	var timestamps []time.Time
	stubEntered := make(chan int, wantCalls+1)

	s.watchSessionFn = func() bool {
		mu.Lock()
		timestamps = append(timestamps, time.Now())
		n := len(timestamps)
		mu.Unlock()
		stubEntered <- n
		if n >= wantCalls {
			// Final call: block until quit so the supervisor doesn't
			// sleep+enter an 8th call before we close s.quit.
			<-s.quit
			return false
		}
		return true
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runContentWatcher()
	}()

	deadline := time.After(2 * time.Second)
	for i := 0; i < wantCalls; i++ {
		select {
		case <-stubEntered:
		case <-deadline:
			mu.Lock()
			got := len(timestamps)
			mu.Unlock()
			t.Fatalf("only %d/%d stub entries within 2s", got, wantCalls)
		}
	}

	close(s.quit)
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(timestamps) != wantCalls {
		t.Fatalf("got %d timestamps, want %d", len(timestamps), wantCalls)
	}

	wantDeltas := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		4 * time.Millisecond,
		8 * time.Millisecond,
		16 * time.Millisecond,
		16 * time.Millisecond, // capped
	}
	for i, want := range wantDeltas {
		got := timestamps[i+1].Sub(timestamps[i])
		// Lower bound: at least 80% of want (allow small overshoot of
		// the timer firing slightly early; in practice time.After never
		// undershoots but we leave slack for clock granularity).
		// Upper bound: want + 25ms scheduler-jitter budget under -race.
		lo := want * 4 / 5
		hi := want + 25*time.Millisecond
		if got < lo || got > hi {
			t.Errorf("delta[%d] = %v, want in [%v, %v]", i, got, lo, hi)
		}
	}
}

// TestContentWatcher_QuitDuringBackoff_ExitsCleanly pins that closing
// s.quit while the supervisor is asleep in its inter-restart backoff
// causes prompt exit, not wait-out-the-full-delay. With base=100ms
// (max), one restart sleep is 100ms; we close quit ~10ms in and
// assert exit within 50ms — well under the 100ms full delay.
func TestContentWatcher_QuitDuringBackoff_ExitsCleanly(t *testing.T) {
	s := newTestServer(t)

	// Force a measurable but short backoff. Use larger base so we can
	// reliably close quit mid-sleep without racing the wakeup.
	oldBase, oldMax, oldReset := watcherBackoffBase, watcherBackoffMax, watcherBackoffResetWindow
	watcherBackoffBase = 100 * time.Millisecond
	watcherBackoffMax = 100 * time.Millisecond
	watcherBackoffResetWindow = 10 * time.Second
	t.Cleanup(func() {
		watcherBackoffBase = oldBase
		watcherBackoffMax = oldMax
		watcherBackoffResetWindow = oldReset
	})

	stubEntered := make(chan struct{}, 4)
	s.watchSessionFn = func() bool {
		stubEntered <- struct{}{}
		return true // always request restart; quit-during-sleep is the exit
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runContentWatcher()
	}()

	// Wait for the first session call.
	select {
	case <-stubEntered:
	case <-time.After(1 * time.Second):
		t.Fatal("supervisor did not enter watchSessionFn within 1s")
	}

	// Stub returned true; supervisor is now sleeping in time.After(100ms).
	// Close s.quit early in that window.
	time.Sleep(10 * time.Millisecond)
	closeStart := time.Now()
	close(s.quit)

	select {
	case <-done:
		// Total budget from quit close to goroutine exit: under the
		// remaining ~90ms sleep. Allow scheduler jitter up to 50ms.
		elapsed := time.Since(closeStart)
		if elapsed > 50*time.Millisecond {
			t.Errorf("supervisor took %v to exit after quit close; want < 50ms (sleep was 100ms)", elapsed)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("supervisor did not exit within 200ms after s.quit close")
	}
}

// TestReadPackStamp pins the four return cases: present-valid, missing,
// corrupt-non-numeric, empty. Permission-denied is omitted as it is not
// portable to test reliably (root, container, FS).
func TestReadPackStamp(t *testing.T) {
	dir := t.TempDir()

	// Case 1: missing file → (zero, false, nil).
	if ts, ok, err := readPackStamp(filepath.Join(dir, "absent.stamp")); err != nil || ok || !ts.IsZero() {
		t.Errorf("missing: got (%v, %v, %v); want (zero, false, nil)", ts, ok, err)
	}

	// Case 2: valid stamp → (parsed, true, nil).
	valid := filepath.Join(dir, "valid.stamp")
	want := time.Unix(0, 1747569600123456789)
	if err := os.WriteFile(valid, []byte("1747569600123456789\n"), 0o644); err != nil {
		t.Fatalf("write valid: %v", err)
	}
	if ts, ok, err := readPackStamp(valid); err != nil || !ok || !ts.Equal(want) {
		t.Errorf("valid: got (%v, %v, %v); want (%v, true, nil)", ts, ok, err, want)
	}

	// Case 3: corrupt non-numeric → (zero, false, err).
	corrupt := filepath.Join(dir, "corrupt.stamp")
	if err := os.WriteFile(corrupt, []byte("not-a-number\n"), 0o644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	ts, ok, err := readPackStamp(corrupt)
	if err == nil || ok || !ts.IsZero() {
		t.Errorf("corrupt: got (%v, %v, %v); want (zero, false, err)", ts, ok, err)
	}

	// Case 4: empty file → (zero, false, err).
	empty := filepath.Join(dir, "empty.stamp")
	if err := os.WriteFile(empty, []byte(""), 0o644); err != nil {
		t.Fatalf("write empty: %v", err)
	}
	ts, ok, err = readPackStamp(empty)
	if err == nil || ok || !ts.IsZero() {
		t.Errorf("empty: got (%v, %v, %v); want (zero, false, err)", ts, ok, err)
	}
}

// TestWritePackStamp_Roundtrip pins UnixNano precision: a time written
// via writePackStamp then read via readPackStamp equals the original at
// nanosecond granularity.
func TestWritePackStamp_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.stamp")

	want := time.Unix(0, 1747569600123456789)
	if err := writePackStamp(path, want); err != nil {
		t.Fatalf("writePackStamp: %v", err)
	}

	got, ok, err := readPackStamp(path)
	if err != nil || !ok {
		t.Fatalf("readPackStamp: got (%v, %v, %v); want (..., true, nil)", got, ok, err)
	}
	if !got.Equal(want) {
		t.Errorf("roundtrip: got %v (unix=%d), want %v (unix=%d)",
			got, got.UnixNano(), want, want.UnixNano())
	}
}

// TestWritePackStamp_AtomicNoTmpLeak pins that no `.tmp` sibling lingers
// on the happy path. (Crash-mid-rename is not directly testable; absence
// of the tmp file proves the rename completed.)
func TestWritePackStamp_AtomicNoTmpLeak(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.stamp")
	if err := writePackStamp(path, time.Unix(0, 42)); err != nil {
		t.Fatalf("writePackStamp: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected %s.tmp to be absent (got err=%v)", path, err)
	}
}

// TestContentWatcher_BackoffResetsAfterSteadyRun pins reset semantics:
// a session that runs >= resetWindow before ending resets the attempt
// counter, so the next backoff is base (1x) not 2x. Uses a local
// 20ms base so the reset/no-reset discriminator (20ms vs 40ms) has
// 10ms headroom each side against Linux timer granularity
// (CONFIG_HZ=250 → ~4ms tick) plus -race scheduling overhead.
// Sequence:
//   call 1: returns true immediately → delay = 20ms
//   call 2: blocks 200ms (> resetWindow), returns true → attempt
//           resets to 0 then increments to 1 → delay = 20ms
//   call 3: captures elapsed-since-call-2-return; asserts ≈ 20ms (NOT 40ms)
func TestContentWatcher_BackoffResetsAfterSteadyRun(t *testing.T) {
	s := newTestServer(t)

	oldBase, oldMax, oldReset := watcherBackoffBase, watcherBackoffMax, watcherBackoffResetWindow
	watcherBackoffBase = 20 * time.Millisecond
	watcherBackoffMax = 320 * time.Millisecond
	watcherBackoffResetWindow = 100 * time.Millisecond
	t.Cleanup(func() {
		watcherBackoffBase = oldBase
		watcherBackoffMax = oldMax
		watcherBackoffResetWindow = oldReset
	})

	var mu sync.Mutex
	var (
		call2Return  time.Time
		call3Enter   time.Time
		callCount    int
		thirdEntered = make(chan struct{})
	)

	s.watchSessionFn = func() bool {
		mu.Lock()
		callCount++
		n := callCount
		mu.Unlock()

		switch n {
		case 1:
			return true // immediate restart
		case 2:
			// Block longer than resetWindow.
			time.Sleep(200 * time.Millisecond)
			mu.Lock()
			call2Return = time.Now()
			mu.Unlock()
			return true
		case 3:
			mu.Lock()
			call3Enter = time.Now()
			mu.Unlock()
			close(thirdEntered)
			<-s.quit
			return false
		}
		<-s.quit
		return false
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runContentWatcher()
	}()

	select {
	case <-thirdEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("third stub call did not happen within 2s")
	}

	mu.Lock()
	pre, post := call2Return, call3Enter
	mu.Unlock()
	delta := post.Sub(pre)

	close(s.quit)
	<-done

	// Expect ≈ 20ms (base). Without reset, after call 2 the supervisor
	// would have attempt=2 and sleep ≈ 40ms. The 30ms upper bound is
	// the discriminator that makes this test fail when reset is broken
	// (with 10ms headroom against ~4ms Linux timer tick under -race
	// plus scheduling overhead).
	// Lower bound 10ms guards against a "delay never applied" pathology
	// (e.g., a future refactor accidentally dropping the sleep).
	if delta < 10*time.Millisecond {
		t.Errorf("call-2-return → call-3-enter = %v, expected ≈ 20ms (base); too short — sleep dropped?", delta)
	}
	if delta > 30*time.Millisecond {
		t.Errorf("call-2-return → call-3-enter = %v, expected ≈ 20ms (base, reset). Without reset would be ≈ 40ms; >30ms means reset broken (or scheduler stall — rerun)", delta)
	}
}
