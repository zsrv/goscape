package world

import (
	"context"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// newRunningFriendsDispatcher constructs a friendsMutationDispatcher and
// starts its worker goroutine against a cancellable context. Cleanup
// cancels the context and waits (bounded) for run to return, so no test
// leaks a worker goroutine into the next test.
func newRunningFriendsDispatcher(t *testing.T) *friendsMutationDispatcher {
	t.Helper()
	d := newFriendsMutationDispatcher(discardLogger())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("friendsMutationDispatcher worker did not exit after cancel")
		}
	})
	return d
}

// TestFriendsDispatchPreservesLoginLogoutOrder pins arch-29.13: two
// friends-server mutations enqueued in login-then-logout order must be
// OBSERVED by the friends-server (here, by the closures standing in for
// the PlayerLogin/PlayerLogout RPCs) in that same order — the TS
// friendThread.postMessage FIFO guarantee.
//
// Mechanism (deterministic — channels, not sleeps): loginAction blocks
// until EITHER logoutSeen closes (meaning logout already ran — the WRONG
// order) OR its own per-call ctx times out (meaning logout has NOT run
// yet — the worker could not have started it, because a single worker
// cannot begin the next queued action until this one returns). d.callTimeout
// is shrunk to a few tens of ms so the correct-order path (which always
// waits out that timeout) resolves quickly.
//
// Under the OLD per-call goroutine fan-out (proven — see this task's
// report for the captured RED run), logoutAction has nothing gating it,
// so it runs to completion in microseconds and closes logoutSeen almost
// immediately; loginAction's select then takes the logoutSeen branch and
// records itself AFTER logout. Observed order becomes [logout, login] —
// a deterministic failure, not a flaky race, because logoutAction is
// never blocked and will essentially always win that select before the
// (much longer) ctx timeout fires.
//
// Under the single-worker FIFO dispatcher, the worker dequeues
// loginAction first and cannot dequeue logoutAction until loginAction
// returns. loginAction's ctx.Done() case is therefore the only way for
// it to return (logoutSeen cannot close first — logoutAction hasn't
// run), so it always fires via the timeout path and records "login"
// first; only then does the worker dequeue and run logoutAction.
func TestFriendsDispatchPreservesLoginLogoutOrder(t *testing.T) {
	d := newRunningFriendsDispatcher(t)
	d.callTimeout = 30 * time.Millisecond

	var mu sync.Mutex
	var observed []string
	logoutSeen := make(chan struct{})

	loginAction := func(ctx context.Context) {
		select {
		case <-logoutSeen:
		case <-ctx.Done():
		}
		mu.Lock()
		observed = append(observed, "login")
		mu.Unlock()
	}
	logoutAction := func(ctx context.Context) {
		mu.Lock()
		observed = append(observed, "logout")
		mu.Unlock()
		close(logoutSeen)
	}

	d.enqueue(loginAction)
	d.enqueue(logoutAction)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(observed)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	got := append([]string(nil), observed...)
	mu.Unlock()
	want := []string{"login", "logout"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("observed order: got %v, want %v (TS FIFO: login must be observed strictly before logout)", got, want)
	}
}

// TestFriendsDispatchAddThenDeleteOrder is the same shape as
// TestFriendsDispatchPreservesLoginLogoutOrder for the FriendlistAdd /
// FriendlistDel pair (the other ordering-sensitive pair named in the
// review finding — an add-then-delete that applies as delete-then-add
// leaves a friend-list row that should have been removed).
func TestFriendsDispatchAddThenDeleteOrder(t *testing.T) {
	d := newRunningFriendsDispatcher(t)
	d.callTimeout = 30 * time.Millisecond

	var mu sync.Mutex
	var observed []string
	delSeen := make(chan struct{})

	addAction := func(ctx context.Context) {
		select {
		case <-delSeen:
		case <-ctx.Done():
		}
		mu.Lock()
		observed = append(observed, "add")
		mu.Unlock()
	}
	delAction := func(ctx context.Context) {
		mu.Lock()
		observed = append(observed, "del")
		mu.Unlock()
		close(delSeen)
	}

	d.enqueue(addAction)
	d.enqueue(delAction)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(observed)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	got := append([]string(nil), observed...)
	mu.Unlock()
	want := []string{"add", "del"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("observed order: got %v, want %v (TS FIFO: add must be observed strictly before del)", got, want)
	}
}

// TestFriendsMutationDispatcher_WorkerExitsPromptlyOnCancel_EnqueueAfterCancelNoOp
// pins the dispatcher's shutdown contract: cancelling run's ctx makes the
// worker return promptly (join bounded — the real Server.bridgeWg.Wait()
// join after bridgesCancel relies on exactly this), and an enqueue call
// AFTER the worker has exited is a harmless no-op — no panic, no
// goroutine leak, and (since no worker remains) the enqueued action never
// runs.
func TestFriendsMutationDispatcher_WorkerExitsPromptlyOnCancel_EnqueueAfterCancelNoOp(t *testing.T) {
	d := newFriendsMutationDispatcher(discardLogger())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.run(ctx)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not exit promptly after ctx cancel")
	}

	ran := make(chan struct{})
	d.enqueue(func(context.Context) { close(ran) })

	select {
	case <-ran:
		t.Fatal("action enqueued after worker exit must not run")
	case <-time.After(50 * time.Millisecond):
		// expected: nothing runs it — the enqueued closure just sits in
		// the queue forever, harmlessly.
	}
}

// TestFriendsMutationDispatcher_DepthWarnThreshold_LogsOncePerThreshold
// pins the depth-warn contract: with the worker permanently blocked (so
// the queue only grows), enqueuing past each threshold in
// friendsDispatchWarnThresholds logs exactly ONE Warn per threshold, not
// one per enqueue call past it.
func TestFriendsMutationDispatcher_DepthWarnThreshold_LogsOncePerThreshold(t *testing.T) {
	buf := &syncBuffer{}
	log := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	d := newFriendsMutationDispatcher(log)

	// No worker started: the queue only grows, exercising enqueue's
	// depth-warn logic in isolation without any dequeue racing it.
	const wantThreshold = 256
	for i := 0; i < wantThreshold; i++ {
		d.enqueue(func(context.Context) {})
	}

	got := strings.Count(buf.String(), "friends dispatch queue depth crossed threshold")
	if got != 1 {
		t.Fatalf("Warn count after reaching depth %d: got %d, want 1\nlog:\n%s", wantThreshold, got, buf.String())
	}
	if !strings.Contains(buf.String(), "threshold=256") {
		t.Errorf("expected log to name threshold=256; got %q", buf.String())
	}

	// Enqueue past the threshold again (still below the next one, 1024) —
	// must NOT log a second time for the same threshold.
	for i := 0; i < 10; i++ {
		d.enqueue(func(context.Context) {})
	}
	got = strings.Count(buf.String(), "friends dispatch queue depth crossed threshold")
	if got != 1 {
		t.Fatalf("Warn count after further enqueues below next threshold: got %d, want 1 (no re-warn)\nlog:\n%s", got, buf.String())
	}
}
