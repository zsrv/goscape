# Rebuild async worker + fsnotify auto-rebuild Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `::rebuild` async via a background worker goroutine, and add an opt-in fsnotify auto-rebuild watcher — both feeding one coordination plane (depth-1 chans + busy/pending state) so `Reload` always runs on the tick goroutine.

**Architecture:** A long-lived `rebuildWorker` goroutine owns PackAll. A `contentWatcher` goroutine owns fsnotify with 1s debounce. Both dispatch via a non-blocking send on a depth-1 `rebuildReq` chan; the worker posts a `rebuildResult` back on a depth-1 chan that the tick loop drains at top-of-body to run `Reload(true)` and broadcast outcome to invoker + all online staff (modlvl≥4). A `rebuildPending` flag under `rebuildMu` closes the busy-but-chan-was-just-drained race.

**Tech Stack:** Go 1.26+; new dep `github.com/fsnotify/fsnotify` (cross-platform recursive file-watch); existing `pkg/packall`, `internal/dskit/services`, `modules/world` patterns.

**Spec:** `docs/superpowers/specs/2026-05-18-rebuild-async-fsnotify-design.md`.

**Pre-flight (run once before Task 1):**
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```
Expected: PASS. This is the green baseline.

---

## Task 1: Add `ContentWatch` config field and flag

**Files:**
- Modify: `modules/world/config.go`

- [ ] **Step 1: Add `ContentWatch bool` field to `Config` struct**

In `modules/world/config.go`, add the field near the other bool fields (e.g., right after `NodeProduction`):

```go
// ContentWatch enables fsnotify auto-rebuild when ContentPath is set.
// Default false; mirrors TS DevThread's dev-only posture.
ContentWatch bool `yaml:"content_watch"`
```

- [ ] **Step 2: Register the flag in `RegisterFlagsAndApplyDefaults`**

Add this line in `RegisterFlagsAndApplyDefaults`, right after the existing `ContentPath` flag registration (around L84):

```go
f.BoolVar(&c.ContentWatch, "world.content-watch", false, "Watch ContentPath subdirs and auto-trigger ::rebuild on changes (debounced 1s). Requires --world.content-path.")
```

- [ ] **Step 3: Verify the build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: builds with no errors.

- [ ] **Step 4: Verify the flag appears in help**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run ./cmd/goscape -h 2>&1 | grep content-watch`
Expected: one line with `-world.content-watch` and the description.

- [ ] **Step 5: Commit**

```bash
git add modules/world/config.go
git commit --no-gpg-sign -m "config(world): add --world.content-watch flag for auto-rebuild"
```

---

## Task 2: Add fsnotify dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the dependency**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go get github.com/fsnotify/fsnotify@latest`
Expected: command succeeds; `go.mod` shows `github.com/fsnotify/fsnotify v1.x.y` in the `require` block.

- [ ] **Step 2: Verify the build still works**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: builds with no errors.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit --no-gpg-sign -m "deps: add github.com/fsnotify/fsnotify for content auto-rebuild watcher"
```

---

## Task 3: Add `Server` fields + chan/packFn initialization

**Files:**
- Modify: `modules/world/server.go`

- [ ] **Step 1: Add the new fields to the `Server` struct**

In `modules/world/server.go`, append this block at the bottom of the `Server` struct definition (just before the closing `}` at L178; place after the last existing field — wherever that is, but inside the struct body):

```go
// --- rebuild async worker plane (spec 2026-05-18-rebuild-async-fsnotify) ---

// rebuildReq carries pack-and-reload requests from the ::rebuild handler
// and the contentWatcher to runRebuildWorker. Depth 1 with non-blocking
// send: a queued request coalesces all concurrent senders into one pack.
rebuildReq chan struct{}

// rebuildResult carries completion events back to the tick goroutine,
// drained non-blocking at the top of runTickLoopWithRate's for-body.
// Depth 1; worker waits for in-flight result to be drained before
// accepting the next request, so a second concurrent enqueue is
// impossible.
rebuildResult chan rebuildResult

// rebuildMu guards rebuildBusy + rebuildPending + rebuildManualInvoker.
// Held only across state transitions; never across packFn.
rebuildMu sync.Mutex

// rebuildBusy is true while packFn is running on the worker.
rebuildBusy bool

// rebuildPending is set by dispatchRebuildRequest. Worker re-queues
// itself on completion if pending is true (closes the race where a
// request arrives during the brief window between worker drain and
// busy=false). Mirrors TS DevThread.processNextQueue.
rebuildPending bool

// rebuildManualInvoker holds the *Player that triggered the in-flight
// rebuild via ::rebuild. nil for fsnotify-triggered. Cleared when the
// worker posts the result.
rebuildManualInvoker *Player

// packFn is the function the worker invokes. Defaults to
// packall.PackAll; test code overrides to avoid 7s real-content packs.
packFn func(srcDir, outDir, dataPackDir string) error

// reloadFn is the function the tick-drain invokes on success. Defaults
// to s.Reload; test code overrides to record invocations / inject errors.
reloadFn func(clearInvs bool) error
```

- [ ] **Step 2: Initialize the chans + functions in `NewServer`**

In `NewServer` (starts at L187 in current head), inside the `s := &Server{...}` literal, add the chans:

```go
rebuildReq:    make(chan struct{}, 1),
rebuildResult: make(chan rebuildResult, 1),
```

Immediately after the struct literal closes (so just after L217), add:

```go
s.packFn = packall.PackAll
s.reloadFn = s.Reload
```

Add the packall import at the top of `server.go` if not already imported (grep first):

```go
"github.com/zsrv/goscape/pkg/packall"
```

- [ ] **Step 3: Initialize the chans + functions in `newTestServer`**

In `modules/world/server_test.go` (`newTestServer` at L311), inside the `s := &Server{...}` literal, add:

```go
rebuildReq:    make(chan struct{}, 1),
rebuildResult: make(chan rebuildResult, 1),
```

After the literal (and after the existing `s.locOps = ...` line), add:

```go
s.reloadFn = s.Reload
// packFn intentionally left nil; tests that exercise the worker set it explicitly.
```

- [ ] **Step 4: Verify the build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: build fails — `rebuildResult` type undefined. That's fine; Task 4 declares it.

If a different error appears, fix it. The expected error is the `rebuildResult` undefined-type one only.

- [ ] **Step 5: Hold the commit**

Do NOT commit yet. The build is broken. Task 4 declares `rebuildResult` and Tasks 5+6 finish the wiring; commit at end of Task 6.

---

## Task 4: Declare `rebuildResult` type + worker file skeleton

**Files:**
- Create: `modules/world/rebuild_worker.go`

- [ ] **Step 1: Create the file with type + helpers**

Create `modules/world/rebuild_worker.go` with this exact content:

```go
package world

import (
	"fmt"
	"time"
)

// rebuildResult is the completion event posted by runRebuildWorker and
// drained on the tick goroutine. Spec 2026-05-18-rebuild-async-fsnotify §3.
type rebuildResult struct {
	err      error
	duration time.Duration
	invoker  *Player // nil for fsnotify-triggered rebuilds
}

// dispatchRebuildRequest is the single non-blocking-send helper used by
// both the ::rebuild handler and the contentWatcher. Sets rebuildPending
// under rebuildMu, then tries a non-blocking send on rebuildReq. The
// pending flag + chan together implement TS DevThread's
// active/processNextQueue/processNextTimeout coalescing.
func (s *Server) dispatchRebuildRequest() {
	s.rebuildMu.Lock()
	s.rebuildPending = true
	s.rebuildMu.Unlock()

	select {
	case s.rebuildReq <- struct{}{}:
	default:
	}
}

// runRebuildWorker is the body of the long-lived pack worker goroutine.
// Exits when s.quit is closed. Mirrors TS DevThread.processChangedFiles.
func (s *Server) runRebuildWorker() {
	for {
		select {
		case <-s.quit:
			return
		case <-s.rebuildReq:
		}

		s.rebuildMu.Lock()
		s.rebuildBusy = true
		s.rebuildPending = false
		invoker := s.rebuildManualInvoker
		s.rebuildMu.Unlock()

		start := time.Now()
		err := s.packFn(s.cfg.ContentPath, s.cfg.CachePath, s.cfg.CachePath)
		elapsed := time.Since(start)

		// Blocking send + s.quit select: back-pressure ensures tick
		// drains and runs Reload before worker accepts the next
		// request. Worst-case wait ≈ 1 tick.
		select {
		case s.rebuildResult <- rebuildResult{err: err, duration: elapsed, invoker: invoker}:
		case <-s.quit:
			return
		}

		s.rebuildMu.Lock()
		s.rebuildBusy = false
		s.rebuildManualInvoker = nil
		pending := s.rebuildPending
		s.rebuildPending = false
		s.rebuildMu.Unlock()

		if pending {
			select {
			case s.rebuildReq <- struct{}{}:
			default:
			}
		}
	}
}

// handleRebuildResult runs on the tick goroutine. Calls reloadFn on
// success and broadcasts the outcome to invoker (if any) + all online
// staff (modlvl >= 4) via broadcastRebuildStaff.
func (s *Server) handleRebuildResult(r rebuildResult) {
	if r.err != nil {
		s.log.Error("rebuild: PackAll failed", "err", r.err)
		s.broadcastRebuildStaff(r.invoker, "Rebuild failed: "+r.err.Error())
		return
	}
	if err := s.reloadFn(true); err != nil {
		s.log.Error("rebuild: Reload failed", "err", err)
		s.broadcastRebuildStaff(r.invoker, "Rebuild failed: reload returned error (see server log).")
		return
	}
	msg := fmt.Sprintf("Rebuilt: %s.", r.duration.Round(time.Millisecond))
	s.broadcastRebuildStaff(r.invoker, msg)
}

// broadcastRebuildStaff sends msg privately to invoker (if non-nil)
// AND to every online player with staffModLevel >= 4. Deduplicates.
// Spec §4.5.
func (s *Server) broadcastRebuildStaff(invoker *Player, msg string) {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	delivered := false
	for _, p := range players {
		if p.staffModLevel < 4 && p != invoker {
			continue
		}
		p.MessageGame(msg)
		if p == invoker {
			delivered = true
		}
	}
	if invoker != nil && !delivered {
		// Invoker not in playerLoop (e.g., test scaffolding) — deliver
		// directly so per-invoker messages still pin.
		invoker.MessageGame(msg)
	}
}
```

- [ ] **Step 2: Verify the build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: builds with no errors. (Task 3's fields now have a referenced type.)

- [ ] **Step 3: Hold the commit**

Still combined with Task 3 + 5 + 6. Commit at end of Task 6.

---

## Task 5: Add top-of-tick result drain

**Files:**
- Modify: `modules/world/tick.go`

- [ ] **Step 1: Insert the drain at the top of the for-body**

In `modules/world/tick.go`, find `runTickLoopWithRate`. Locate the `for {` at L39. Insert the following block immediately inside the `for {` (so it becomes the very first statement of the loop body, BEFORE the NAI-182 shutdown gate at L44):

```go
		// NAI-REBUILD-ASYNC — drain at top-of-body so Reload runs before
		// any per-tick work observes mid-swap state. Mirrors
		// processShutdown's top-of-body placement.
		select {
		case r := <-s.rebuildResult:
			s.handleRebuildResult(r)
		default:
		}

```

(Note the trailing blank line — preserve the existing visual gap before the NAI-182 comment.)

- [ ] **Step 2: Verify the build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: builds with no errors.

- [ ] **Step 3: Hold the commit (still wiring up; commit at Task 6).**

---

## Task 6: Replace inline sync `::rebuild` handler with async dispatch

**Files:**
- Modify: `modules/world/handlers_game.go`

- [ ] **Step 1: Replace the existing `case "rebuild":` block**

In `modules/world/handlers_game.go`, the existing block runs L656–L681 (verified at session start). Replace it entirely with:

```go
		case "rebuild":
			// NAI-REBUILD-ASYNC (spec 2026-05-18-rebuild-async-fsnotify).
			// Async dispatch via rebuildWorker. Mirrors TS
			// ClientCheatHandler.ts:151-153 → World.rebuild() →
			// DevThread.processChangedFiles. Handler returns
			// immediately; result is drained at top-of-tick.
			if p.client.server.cfg.ContentPath == "" {
				p.MessageGame("Rebuild failed: --world.content-path is not configured.")
				return nil
			}
			p.client.server.rebuildMu.Lock()
			if p.client.server.rebuildManualInvoker == nil {
				p.client.server.rebuildManualInvoker = p
			}
			p.client.server.rebuildMu.Unlock()
			p.client.server.broadcastRebuildStaff(p, "Rebuilding scripts...")
			p.client.server.dispatchRebuildRequest()
			return nil
```

- [ ] **Step 2: Update or remove the stale NAI-189-D1 carryforward comment**

In `handlers_game.go` around L541 (verified at session start), the block comment refers to the now-shipped sync `::rebuild`. Replace lines L541–L544 (or whichever surrounding lines reference the inline sync implementation) with a one-line pointer to the async wiring:

```go
		// NAI-189-D1 / NAI-REBUILD-ASYNC: the ::rebuild cheat dispatches
		// asynchronously via rebuildWorker; see rebuild_worker.go and the
		// case "rebuild": arm below. Spec
		// docs/superpowers/specs/2026-05-18-rebuild-async-fsnotify-design.md.
```

(If the surrounding comment block has additional content unrelated to rebuild, leave that content; only the rebuild-specific lines change.)

- [ ] **Step 3: Drop the unused `packall` import from handlers_game.go**

Run: `grep -n "packall" modules/world/handlers_game.go`
If `packall` is no longer referenced in the file (the inline call was the only use), remove the import line. If `goimports` is preferred, run it manually:
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
If it fails complaining about unused `packall` import, remove that import.
Expected: builds clean after this step.

- [ ] **Step 4: Verify the build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: builds with no errors.

- [ ] **Step 5: Verify existing-tests state — they SHOULD fail now**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleClientCheat_Rebuild_ -v 2>&1 | tail -30`
Expected: `TestHandleClientCheat_Rebuild_Dispatches` and `TestHandleClientCheat_Rebuild_PackAllFailure_PrivateError` FAIL (they expected synchronous "Rebuilt:" / "Rebuild failed:" on the same drainConn cycle; now those messages arrive only after the tick drains). `TestHandleClientCheat_Rebuild_NoContentPath_PrivateError` PASSES (no async path).

This is the expected red-phase state. Tasks 7+ rewrite these tests around the async semantics; commit the broken-state in this group so the next worker has a self-consistent starting point.

- [ ] **Step 6: Commit Tasks 3–6 together**

```bash
git add modules/world/server.go modules/world/server_test.go modules/world/rebuild_worker.go modules/world/tick.go modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: async ::rebuild worker scaffolding + tick result drain

Adds rebuildReq/rebuildResult chans, rebuildMu/busy/pending state,
packFn/reloadFn test seams, and the rebuildWorker goroutine body to
Server. Tick loop drains rebuildResult at top-of-body and runs Reload
via reloadFn. ::rebuild handler now non-blocking-sends a request and
returns immediately. Worker is not yet spawned in startingFn (Task 8)
and the rebuild tests are red until Task 11 rewrites them.

Spec: docs/superpowers/specs/2026-05-18-rebuild-async-fsnotify-design.md
EOF
)"
```

---

## Task 7: Worker test — single request runs pack and posts result

**Files:**
- Create: `modules/world/rebuild_worker_test.go`

- [ ] **Step 1: Write the failing test**

Create `modules/world/rebuild_worker_test.go` with this exact content:

```go
package world

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestRebuildWorker_Request_RunsPackAndPostsResult pins the happy path:
// dispatchRebuildRequest → worker reads, runs packFn, posts success result.
func TestRebuildWorker_Request_RunsPackAndPostsResult(t *testing.T) {
	s := newTestServer(t)
	s.cfg.ContentPath = "/content"
	s.cfg.CachePath = "/cache"

	var calls atomic.Int32
	var gotSrc, gotOut, gotPack string
	s.packFn = func(srcDir, outDir, dataPackDir string) error {
		calls.Add(1)
		gotSrc, gotOut, gotPack = srcDir, outDir, dataPackDir
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runRebuildWorker()
	}()
	t.Cleanup(func() {
		close(s.quit)
		<-done
	})

	s.dispatchRebuildRequest()

	select {
	case r := <-s.rebuildResult:
		if r.err != nil {
			t.Fatalf("expected nil err, got %v", r.err)
		}
		if r.duration <= 0 {
			t.Errorf("expected duration > 0, got %v", r.duration)
		}
		if r.invoker != nil {
			t.Errorf("expected nil invoker (no manual sender), got %v", r.invoker)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rebuildResult")
	}

	if c := calls.Load(); c != 1 {
		t.Errorf("expected packFn called once, got %d", c)
	}
	if gotSrc != "/content" || gotOut != "/cache" || gotPack != "/cache" {
		t.Errorf("packFn args: src=%q out=%q pack=%q; want /content,/cache,/cache",
			gotSrc, gotOut, gotPack)
	}
}
```

- [ ] **Step 2: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestRebuildWorker_Request_RunsPackAndPostsResult -v`
Expected: PASS. (Task 4's worker body satisfies this; this test pins the contract.)

- [ ] **Step 3: Run with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestRebuildWorker_Request_RunsPackAndPostsResult -v`
Expected: PASS, no race warnings.

- [ ] **Step 4: Commit**

```bash
git add modules/world/rebuild_worker_test.go
git commit --no-gpg-sign -m "test(world): pin rebuildWorker happy-path pack+post-result"
```

---

## Task 8: Worker test — pack error posts error result

**Files:**
- Modify: `modules/world/rebuild_worker_test.go`

- [ ] **Step 1: Append the test**

Append to `modules/world/rebuild_worker_test.go`:

```go
// TestRebuildWorker_PackError_PostsErrorResult pins that packFn errors
// propagate to rebuildResult.err verbatim. handleRebuildResult (tested
// elsewhere) is responsible for not calling reloadFn on error.
func TestRebuildWorker_PackError_PostsErrorResult(t *testing.T) {
	s := newTestServer(t)
	wantErr := errors.New("boom")
	s.packFn = func(srcDir, outDir, dataPackDir string) error { return wantErr }

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runRebuildWorker()
	}()
	t.Cleanup(func() {
		close(s.quit)
		<-done
	})

	s.dispatchRebuildRequest()

	select {
	case r := <-s.rebuildResult:
		if !errors.Is(r.err, wantErr) {
			t.Errorf("expected errors.Is err == wantErr; got %v", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rebuildResult")
	}
}
```

- [ ] **Step 2: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestRebuildWorker_PackError -v`
Expected: PASS, no race.

- [ ] **Step 3: Commit**

```bash
git add modules/world/rebuild_worker_test.go
git commit --no-gpg-sign -m "test(world): pin rebuildWorker propagates packFn error to result"
```

---

## Task 9: Worker test — busy coalesce (pending triggers second run)

**Files:**
- Modify: `modules/world/rebuild_worker_test.go`

- [ ] **Step 1: Append the test**

Append:

```go
// TestRebuildWorker_BusyCoalesce_PendingTriggersSecondRun pins the
// busy-state coalesce: a second dispatch while busy sets pending; on
// pack completion, worker re-queues itself and runs packFn exactly one
// more time. A third dispatch arriving during the first pack is
// coalesced into the same "pending" slot — not a third run.
func TestRebuildWorker_BusyCoalesce_PendingTriggersSecondRun(t *testing.T) {
	s := newTestServer(t)

	release1 := make(chan struct{})
	release2 := make(chan struct{})
	entered := make(chan struct{}, 2)
	var calls atomic.Int32

	s.packFn = func(srcDir, outDir, dataPackDir string) error {
		n := calls.Add(1)
		entered <- struct{}{}
		switch n {
		case 1:
			<-release1
		case 2:
			<-release2
		default:
			t.Errorf("unexpected third packFn invocation")
		}
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runRebuildWorker()
	}()
	t.Cleanup(func() {
		close(s.quit)
		<-done
	})

	// Dispatch req1; wait until packFn entered (so worker is busy).
	s.dispatchRebuildRequest()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first packFn entry")
	}

	// Dispatch req2 + req3 while busy. Both should coalesce into a
	// single pending slot (one re-run after release1).
	s.dispatchRebuildRequest()
	s.dispatchRebuildRequest()

	// Release packFn #1; drain result1; expect packFn #2 to enter.
	close(release1)
	select {
	case <-s.rebuildResult:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for result #1")
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second packFn entry (coalesced pending)")
	}

	// Release packFn #2; drain result2.
	close(release2)
	select {
	case <-s.rebuildResult:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for result #2")
	}

	// Give the worker a window to spuriously re-run. It must not.
	select {
	case <-entered:
		t.Errorf("packFn ran a third time; coalesce broken (calls=%d)", calls.Load())
	case <-time.After(200 * time.Millisecond):
	}

	if c := calls.Load(); c != 2 {
		t.Errorf("expected exactly 2 packFn calls, got %d", c)
	}
}
```

- [ ] **Step 2: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestRebuildWorker_BusyCoalesce -v`
Expected: PASS, no race.

- [ ] **Step 3: Commit**

```bash
git add modules/world/rebuild_worker_test.go
git commit --no-gpg-sign -m "test(world): pin rebuildWorker coalesce semantics (busy + pending → one re-run)"
```

---

## Task 10: Worker test — quit mid-idle exits cleanly

**Files:**
- Modify: `modules/world/rebuild_worker_test.go`

- [ ] **Step 1: Append the test**

Append:

```go
// TestRebuildWorker_QuitMidIdle_ExitsCleanly pins that closing s.quit
// causes runRebuildWorker to return promptly when idle.
func TestRebuildWorker_QuitMidIdle_ExitsCleanly(t *testing.T) {
	s := newTestServer(t)
	s.packFn = func(srcDir, outDir, dataPackDir string) error { return nil }

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runRebuildWorker()
	}()

	close(s.quit)

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("worker did not exit within 1s after s.quit close")
	}
}
```

- [ ] **Step 2: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestRebuildWorker_QuitMidIdle -v`
Expected: PASS, no race.

- [ ] **Step 3: Commit**

```bash
git add modules/world/rebuild_worker_test.go
git commit --no-gpg-sign -m "test(world): pin rebuildWorker exits on s.quit close while idle"
```

---

## Task 11: Rewrite handler tests for async dispatch

**Files:**
- Modify: `modules/world/handler_cheat_rebuild_test.go`

- [ ] **Step 1: Replace the entire file content**

Overwrite `modules/world/handler_cheat_rebuild_test.go` with:

```go
package world

import (
	"bytes"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

// TestHandleClientCheat_Rebuild_NoContentPath_PrivateError pins that
// the handler short-circuits when ContentPath is empty: private message
// to invoker only, no broadcast, no worker engagement.
func TestHandleClientCheat_Rebuild_NoContentPath_PrivateError(t *testing.T) {
	p, conn := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.cfg.NodeProduction = false
	s.cfg.ContentPath = "" // unconfigured
	p.staffModLevel = 4
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, conn)
	dispatchCheat(t, p, "rebuild")
	p.client.flushWrite()
	out := <-received

	if !bytes.Contains(out, []byte("Rebuild failed")) {
		t.Errorf("missing 'Rebuild failed' message; got %q", out)
	}
	if !bytes.Contains(out, []byte("content-path")) {
		t.Errorf("error message should mention content-path; got %q", out)
	}
	if bytes.Contains(out, []byte("Rebuilding scripts...")) {
		t.Errorf("must not emit 'Rebuilding scripts...' when content-path is empty; got %q", out)
	}
}

// TestHandleClientCheat_Rebuild_DispatchesAndBroadcastsInProgress pins
// that the handler returns immediately, enqueues a request to the
// worker (rebuildBusy true while packFn blocks), and emits "Rebuilding
// scripts..." to the invoker.
func TestHandleClientCheat_Rebuild_DispatchesAndBroadcastsInProgress(t *testing.T) {
	p, conn := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.cfg.NodeProduction = false
	s.cfg.ContentPath = "/content"
	s.cfg.CachePath = "/cache"
	p.staffModLevel = 4
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Block packFn so the worker stays busy across the assertion.
	release := make(chan struct{})
	entered := make(chan struct{})
	s.packFn = func(srcDir, outDir, dataPackDir string) error {
		close(entered)
		<-release
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runRebuildWorker()
	}()
	t.Cleanup(func() {
		close(release)
		<-s.rebuildResult // drain so worker can complete
		close(s.quit)
		<-done
	})

	received := drainConn(t, conn)
	dispatchCheat(t, p, "rebuild")
	p.client.flushWrite()
	out := <-received

	if !bytes.Contains(out, []byte("Rebuilding scripts...")) {
		t.Errorf("missing 'Rebuilding scripts...' start message; got %q", out)
	}
	if bytes.Contains(out, []byte("Rebuilt:")) {
		t.Errorf("must not emit 'Rebuilt:' before tick drains result; got %q", out)
	}

	// Confirm worker is in fact busy (i.e., dispatchRebuildRequest fed it).
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("packFn never entered — dispatch did not reach worker")
	}

	s.rebuildMu.Lock()
	busy := s.rebuildBusy
	invoker := s.rebuildManualInvoker
	s.rebuildMu.Unlock()
	if !busy {
		t.Errorf("expected rebuildBusy=true while packFn blocks")
	}
	if invoker != p {
		t.Errorf("expected rebuildManualInvoker == p; got %v", invoker)
	}
}

// TestHandleClientCheat_Rebuild_Async_HappyPath_DrainsAndReloads pins
// the full async-success path: dispatch → worker runs packFn → result
// drained on tick → reloadFn invoked → "Rebuilt: ..." emitted to invoker.
func TestHandleClientCheat_Rebuild_Async_HappyPath_DrainsAndReloads(t *testing.T) {
	p, conn := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.cfg.NodeProduction = false
	s.cfg.ContentPath = "/content"
	s.cfg.CachePath = "/cache"
	p.staffModLevel = 4
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s.packFn = func(srcDir, outDir, dataPackDir string) error { return nil }

	var reloadCalls atomic.Int32
	var reloadArg atomic.Bool
	s.reloadFn = func(clearInvs bool) error {
		reloadCalls.Add(1)
		reloadArg.Store(clearInvs)
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runRebuildWorker()
	}()
	t.Cleanup(func() {
		close(s.quit)
		<-done
	})

	received := drainConn(t, conn)
	dispatchCheat(t, p, "rebuild")

	// Wait for the worker to post a result, then run the tick drain
	// directly (mirrors the top-of-tick select in runTickLoopWithRate).
	select {
	case r := <-s.rebuildResult:
		s.handleRebuildResult(r)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rebuildResult")
	}

	p.client.flushWrite()
	out := <-received

	if !bytes.Contains(out, []byte("Rebuilding scripts...")) {
		t.Errorf("missing 'Rebuilding scripts...'; got %q", out)
	}
	if !bytes.Contains(out, []byte("Rebuilt:")) {
		t.Errorf("missing 'Rebuilt:' success message; got %q", out)
	}
	if bytes.Contains(out, []byte("Rebuild failed")) {
		t.Errorf("happy path must not emit 'Rebuild failed'; got %q", out)
	}
	if c := reloadCalls.Load(); c != 1 {
		t.Errorf("expected reloadFn called once; got %d", c)
	}
	if !reloadArg.Load() {
		t.Errorf("expected reloadFn(true); got reloadFn(false)")
	}
}

// TestHandleClientCheat_Rebuild_Async_PackError_BroadcastsFailureAndSkipsReload
// pins that packFn errors arrive at the tick drain as a failure message
// and reloadFn is NOT invoked.
func TestHandleClientCheat_Rebuild_Async_PackError_BroadcastsFailureAndSkipsReload(t *testing.T) {
	p, conn := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.cfg.NodeProduction = false
	s.cfg.ContentPath = "/content"
	s.cfg.CachePath = "/cache"
	p.staffModLevel = 4
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s.packFn = func(srcDir, outDir, dataPackDir string) error {
		return errors.New("pack boom")
	}

	var reloadCalls atomic.Int32
	s.reloadFn = func(clearInvs bool) error {
		reloadCalls.Add(1)
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runRebuildWorker()
	}()
	t.Cleanup(func() {
		close(s.quit)
		<-done
	})

	received := drainConn(t, conn)
	dispatchCheat(t, p, "rebuild")

	select {
	case r := <-s.rebuildResult:
		s.handleRebuildResult(r)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rebuildResult")
	}

	p.client.flushWrite()
	out := <-received

	if !bytes.Contains(out, []byte("Rebuilding scripts...")) {
		t.Errorf("missing 'Rebuilding scripts...'; got %q", out)
	}
	if !bytes.Contains(out, []byte("Rebuild failed: pack boom")) {
		t.Errorf("missing 'Rebuild failed: pack boom'; got %q", out)
	}
	if bytes.Contains(out, []byte("Rebuilt:")) {
		t.Errorf("must not emit 'Rebuilt:' on packFn error; got %q", out)
	}
	if c := reloadCalls.Load(); c != 0 {
		t.Errorf("expected reloadFn NOT called on pack error; got %d", c)
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestHandleClientCheat_Rebuild_ -v 2>&1 | tail -30`
Expected: all four tests PASS, no race.

- [ ] **Step 3: Commit**

```bash
git add modules/world/handler_cheat_rebuild_test.go
git commit --no-gpg-sign -m "test(world): rewrite ::rebuild handler tests for async dispatch"
```

---

## Task 12: Broadcast helper test — staff broadcast + non-staff filter

**Files:**
- Create: `modules/world/rebuild_broadcast_test.go`

- [ ] **Step 1: Write the failing test**

Create `modules/world/rebuild_broadcast_test.go`:

```go
package world

import (
	"bytes"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

// TestBroadcastRebuildStaff_DeliversToInvokerAndStaffOnly pins that a
// rebuild outcome reaches invoker + every online staff (modlvl>=4) but
// NOT non-staff players. Mirrors spec §4.5.
func TestBroadcastRebuildStaff_DeliversToInvokerAndStaffOnly(t *testing.T) {
	s := newTestServer(t)

	invoker, invokerConn := newTestPlayer(t)
	invoker.client.server = s
	invoker.staffModLevel = 4
	invoker.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	staff, staffConn := newTestPlayer(t)
	staff.client.server = s
	staff.staffModLevel = 4
	staff.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	peasant, peasantConn := newTestPlayer(t)
	peasant.client.server = s
	peasant.staffModLevel = 0
	peasant.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s.playersMu.Lock()
	s.playerLoop = []*Player{invoker, staff, peasant}
	s.playersMu.Unlock()

	invokerRcv := drainConn(t, invokerConn)
	staffRcv := drainConn(t, staffConn)
	peasantRcv := drainConn(t, peasantConn)

	s.broadcastRebuildStaff(invoker, "Rebuilt: 42ms.")

	invoker.client.flushWrite()
	staff.client.flushWrite()
	peasant.client.flushWrite()

	if got := <-invokerRcv; !bytes.Contains(got, []byte("Rebuilt: 42ms.")) {
		t.Errorf("invoker missing message; got %q", got)
	}
	if got := <-staffRcv; !bytes.Contains(got, []byte("Rebuilt: 42ms.")) {
		t.Errorf("staff missing message; got %q", got)
	}
	if got := <-peasantRcv; bytes.Contains(got, []byte("Rebuilt: 42ms.")) {
		t.Errorf("non-staff must NOT receive rebuild message; got %q", got)
	}
}

// TestBroadcastRebuildStaff_FsnotifyTriggered_NoInvoker pins the
// auto-rebuild path (invoker == nil): only staff receive; non-staff
// don't.
func TestBroadcastRebuildStaff_FsnotifyTriggered_NoInvoker(t *testing.T) {
	s := newTestServer(t)

	staff, staffConn := newTestPlayer(t)
	staff.client.server = s
	staff.staffModLevel = 4
	staff.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	peasant, peasantConn := newTestPlayer(t)
	peasant.client.server = s
	peasant.staffModLevel = 0
	peasant.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s.playersMu.Lock()
	s.playerLoop = []*Player{staff, peasant}
	s.playersMu.Unlock()

	staffRcv := drainConn(t, staffConn)
	peasantRcv := drainConn(t, peasantConn)

	s.broadcastRebuildStaff(nil, "Rebuilding scripts...")

	staff.client.flushWrite()
	peasant.client.flushWrite()

	if got := <-staffRcv; !bytes.Contains(got, []byte("Rebuilding scripts...")) {
		t.Errorf("staff missing message; got %q", got)
	}
	if got := <-peasantRcv; bytes.Contains(got, []byte("Rebuilding scripts...")) {
		t.Errorf("non-staff must NOT receive auto-rebuild message; got %q", got)
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestBroadcastRebuildStaff -v`
Expected: PASS, no race.

If you see "invoker not in playerLoop" code path being exercised (the helper's `delivered` fallback), it means the first test's `invoker` is in `playerLoop` so the fallback should NOT fire. The test still passes because invoker IS in `playerLoop` for the first test. Verify by adding a temporary `t.Logf` if uncertain, then revert.

- [ ] **Step 3: Commit**

```bash
git add modules/world/rebuild_broadcast_test.go
git commit --no-gpg-sign -m "test(world): pin broadcastRebuildStaff filters non-staff and handles nil invoker"
```

---

## Task 13: Wire `runRebuildWorker` into world service startingFn

**Files:**
- Modify: `modules/world/world.go`

- [ ] **Step 1: Spawn the worker in `startingFn`**

In `modules/world/world.go`, locate `startingFn` at L82. Add the worker spawn after the existing `cache.PreloadClient`/`cache.MakeCRCs`/`lc.WorldStartup` block, before the `return nil` at L90. The result should look like:

```go
	startingFn := func(ctx context.Context) error {
		if err := cache.PreloadClient("data/pack/client"); err != nil {
			return fmt.Errorf("world: preload client assets: %w", err)
		}
		cache.MakeCRCs()
		if lc != nil {
			lc.WorldStartup(ctx, int32(serv.cfg.NodeID), serv.cfg.NodeProfile)
		}
		// NAI-REBUILD-ASYNC: spawn the long-lived pack worker if
		// ContentPath is configured. Exits when serv.quit closes (via
		// stoppingFn → Shutdown).
		if serv.cfg.ContentPath != "" {
			go serv.runRebuildWorker()
		} else if serv.cfg.ContentWatch {
			serv.log.Warn("world: --world.content-watch is set but --world.content-path is empty; auto-rebuild disabled")
		}
		return nil
	}
```

- [ ] **Step 2: Verify the build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: builds clean.

- [ ] **Step 3: Verify the whole modules/world suite still passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... 2>&1 | tail -5`
Expected: PASS, no race.

- [ ] **Step 4: Commit**

```bash
git add modules/world/world.go
git commit --no-gpg-sign -m "world: spawn runRebuildWorker in service startingFn when ContentPath is set"
```

---

## Task 14: Create content watcher skeleton

**Files:**
- Create: `modules/world/content_watcher.go`

- [ ] **Step 1: Create the file**

Create `modules/world/content_watcher.go` with:

```go
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
```

- [ ] **Step 2: Verify the build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: builds clean (fsnotify dep from Task 2 resolves).

- [ ] **Step 3: Commit**

```bash
git add modules/world/content_watcher.go
git commit --no-gpg-sign -m "world: add contentWatcher (fsnotify + 1s debounce) skeleton"
```

---

## Task 15: Watcher test — file write triggers rebuild after debounce

**Files:**
- Create: `modules/world/content_watcher_test.go`

- [ ] **Step 1: Write the test**

Create `modules/world/content_watcher_test.go`:

```go
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
```

- [ ] **Step 2: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestContentWatcher_FileWrite -v`
Expected: PASS, no race.

If it fails with "did not receive on rebuildReq": (a) verify `dispatchRebuildRequest`'s pending flag did set (read `s.rebuildPending` under mu) — if not, the watcher debounce branch isn't firing; check `time.After` semantics. (b) On WSL or other filesystems where inotify is sluggish, bump the initial sleep to 250ms.

- [ ] **Step 3: Commit**

```bash
git add modules/world/content_watcher_test.go
git commit --no-gpg-sign -m "test(world): pin contentWatcher single-file write triggers rebuildReq"
```

---

## Task 16: Watcher test — burst coalesces to one rebuildReq

**Files:**
- Modify: `modules/world/content_watcher_test.go`

- [ ] **Step 1: Append the test**

Append:

```go
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
```

- [ ] **Step 2: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestContentWatcher_BurstCoalesces -v`
Expected: PASS, no race.

- [ ] **Step 3: Commit**

```bash
git add modules/world/content_watcher_test.go
git commit --no-gpg-sign -m "test(world): pin contentWatcher burst coalesces to one rebuildReq"
```

---

## Task 17: Watcher test — new subdir auto-added to watch

**Files:**
- Modify: `modules/world/content_watcher_test.go`

- [ ] **Step 1: Append the test**

Append:

```go
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
```

- [ ] **Step 2: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestContentWatcher_NewSubdir -v`
Expected: PASS, no race.

- [ ] **Step 3: Commit**

```bash
git add modules/world/content_watcher_test.go
git commit --no-gpg-sign -m "test(world): pin contentWatcher dynamically adds new subdirs to watch"
```

---

## Task 18: Watcher test — non-watched dir ignored

**Files:**
- Modify: `modules/world/content_watcher_test.go`

- [ ] **Step 1: Append the test**

Append:

```go
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
```

- [ ] **Step 2: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestContentWatcher_NonWatchedDirIgnored -v`
Expected: PASS, no race.

- [ ] **Step 3: Commit**

```bash
git add modules/world/content_watcher_test.go
git commit --no-gpg-sign -m "test(world): pin contentWatcher ignores non-canonical dirs"
```

---

## Task 19: Watcher test — quit closes cleanly

**Files:**
- Modify: `modules/world/content_watcher_test.go`

- [ ] **Step 1: Append the test**

Append:

```go
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
```

- [ ] **Step 2: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestContentWatcher_QuitClosesCleanly -v`
Expected: PASS, no race.

- [ ] **Step 3: Commit**

```bash
git add modules/world/content_watcher_test.go
git commit --no-gpg-sign -m "test(world): pin contentWatcher exits on s.quit close"
```

---

## Task 20: Wire `runContentWatcher` into service startingFn

**Files:**
- Modify: `modules/world/world.go`

- [ ] **Step 1: Add the watcher spawn**

In `modules/world/world.go`, locate the rebuild-worker spawn added in Task 13. Replace it with:

```go
		// NAI-REBUILD-ASYNC: spawn long-lived pack worker + optional
		// fsnotify watcher when ContentPath is configured. Both exit
		// when serv.quit closes (via stoppingFn → Shutdown).
		if serv.cfg.ContentPath != "" {
			go serv.runRebuildWorker()
			if serv.cfg.ContentWatch {
				go serv.runContentWatcher()
			}
		} else if serv.cfg.ContentWatch {
			serv.log.Warn("world: --world.content-watch is set but --world.content-path is empty; auto-rebuild disabled")
		}
```

- [ ] **Step 2: Verify the build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: builds clean.

- [ ] **Step 3: Verify the modules/world suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... 2>&1 | tail -5`
Expected: PASS, no race.

- [ ] **Step 4: Commit**

```bash
git add modules/world/world.go
git commit --no-gpg-sign -m "world: spawn contentWatcher when ContentPath+ContentWatch both set"
```

---

## Task 21: Full-suite + smoke-pack baseline verification

**Files:** (no code changes)

- [ ] **Step 1: Run the full project test suite under `-race`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... 2>&1 | tail -20`
Expected: PASS, no race warnings.

If any test outside `modules/world/` fails (unlikely — this slice touches only `modules/world/` + `go.mod`), inspect and fix at this task before proceeding.

- [ ] **Step 2: Run smoke-pack baseline check**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content 2>&1 | tail -20
```
Expected: 12 OK / 0 ERR / 0 SKIP. (Per `[[smoke_pack_worldmap_stage_wiring]]`, this is the established baseline.)

If smoke-pack diverges from the baseline, something in this slice has accidentally affected pack output — should not happen because no `pkg/pack*` files were touched, but verify by `git diff main..HEAD --stat` and confirm only `modules/world/` + `go.mod` + `go.sum` + `docs/` changed.

- [ ] **Step 3: Verify `goscape --help` shows the new flag**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run ./cmd/goscape -h 2>&1 | grep -E "content-(path|watch)"
```
Expected: two lines — `-world.content-path` and `-world.content-watch`.

- [ ] **Step 4: No commit (verification only)**

If everything passes, the implementation is complete. Proceed to Task 22.

If any check fails, identify the responsible task, revert or amend, and re-run.

---

## Task 22: Update spec back-references and memory

**Files:**
- Modify: `docs/superpowers/specs/2026-05-17-rebuild-cheat-design.md` (the predecessor spec) — optional, only if a "follow-ups" section exists there.
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/rebuild_cheat_close.md` (the predecessor memory).
- Create: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/rebuild_async_fsnotify_close.md`.
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`.

- [ ] **Step 1: Update `rebuild_cheat_close.md` to retire the two follow-ups**

Open the memory file. In the "Open follow-ups:" section, retire the two bullets that say "Async-via-worker (deferred)" and "fsnotify auto-rebuild (deferred)". Replace both with a single back-link line:

```markdown
**Retires:** Async-via-worker and fsnotify auto-rebuild follow-ups closed by [[rebuild_async_fsnotify_close]] on 2026-05-18.
```

Leave the third bullet (RunServerCompiler pointer-check error on real Content) — it's unrelated.

- [ ] **Step 2: Write the new close memory**

Create `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/rebuild_async_fsnotify_close.md`:

```markdown
---
name: rebuild-async-fsnotify-close
description: "::rebuild async worker + fsnotify auto-rebuild shipped 2026-05-18; replaces sync on-tick PackAll with a long-lived rebuildWorker goroutine; new --world.content-watch flag enables fsnotify-debounced auto-rebuild; mirrors TS DevThread.processChangedFiles + active/processNextQueue coalescing"
metadata:
  type: project
---

# `::rebuild` async + fsnotify auto-rebuild close (2026-05-18)

Closes the two deferred follow-ups from [[rebuild_cheat_close]]. Spec: `docs/superpowers/specs/2026-05-18-rebuild-async-fsnotify-design.md`. Plan: `docs/superpowers/plans/2026-05-18-rebuild-async-fsnotify.md`.

**Architecture (changes from prior sync ::rebuild):**
- Long-lived `rebuildWorker` goroutine on Server, spawned in world.go startingFn when ContentPath != "". Reads `rebuildReq` (chan struct{} cap 1, non-blocking-send coalescing), runs `packFn` (default `packall.PackAll`, overridable for tests), posts `rebuildResult` on a cap-1 chan back to the tick goroutine.
- Tick loop drains `rebuildResult` non-blocking at top-of-body (before the NAI-182 shutdown gate) and calls `reloadFn(true)` on success. Reload still runs on the tick goroutine — single-tick-goroutine invariant preserved.
- `rebuildMu` guards `rebuildBusy` + `rebuildPending` + `rebuildManualInvoker`. Mutex never held across PackAll. Pending-flag closes the race between "worker drained chan" and "worker cleared busy". Mirrors TS DevThread.active + processNextQueue + processNextTimeout coalescing semantics.
- New `contentWatcher` goroutine: fsnotify over the 12 TS-canonical subdirs (`maps,songs,jingles,binary,fonts,title,scripts,sprites,models,textures,synth,wordenc`), 1s debounce, dynamic add-on-CREATE-dir for subdirs created mid-session. Opt-in via `--world.content-watch` (yaml `content_watch`), default false.
- Broadcast model: manual `::rebuild` → "Rebuilding scripts..." sent to invoker + all online staff (modlvl≥4) immediately; "Rebuilt: Ns." or "Rebuild failed: …" sent on drain. fsnotify-triggered → no invoker; staff get both messages.
- Replaces inline PackAll/Reload in `handlers_game.go`'s `case "rebuild":`. The handler now non-blocking-sends and returns immediately; tick continues normally during ~7s pack.

**Test seams:** `Server.packFn` (override packall.PackAll), `Server.reloadFn` (override s.Reload). Both set in `newTestServer` (reloadFn defaults to s.Reload; packFn left nil — tests that exercise worker set it explicitly).

**Tests landed:**
- 4 worker tests (`rebuild_worker_test.go`): happy path, pack-error propagates, busy-coalesce (one re-run despite 2 queued), quit-mid-idle.
- 5 watcher tests (`content_watcher_test.go`): file-write triggers, burst coalesces, new-subdir-added, non-watched-dir ignored, quit-closes.
- 4 handler tests rewritten (`handler_cheat_rebuild_test.go`): no-content-path, dispatches+broadcasts-in-progress, async happy-path-drains-and-reloads, async pack-error-broadcasts-failure-skips-reload.
- 2 broadcast tests (`rebuild_broadcast_test.go`): staff-only filter, nil-invoker (fsnotify-triggered) path.

**Deferrals (still open):**
- PackAll context cancellation — worker can't interrupt mid-pack on s.quit. Pack stages have no ctx threading; would need a separate large slice.
- fsnotify watcher auto-restart after Events/Errors chan closes. Manual `::rebuild` continues to work; auto-rebuild silently stops until process restart.
- Stale invoker on logout — if invoker disconnects between dispatch and drain, MessageGame to a logged-off player is currently a no-op (verified during exec). No special-case needed.

**Dep added:** `github.com/fsnotify/fsnotify` (MIT, cross-platform).

**Smoke-pack invariant:** 12 OK / 0 ERR baseline unchanged post-merge.

**Retires:** Two open follow-ups in [[rebuild_cheat_close]] (async-via-worker, fsnotify auto-rebuild).
```

- [ ] **Step 3: Add MEMORY.md index line**

Prepend (or insert in date order at top of the list) in `MEMORY.md`:

```markdown
- [rebuild async + fsnotify close](rebuild_async_fsnotify_close.md) — `::rebuild` async worker + opt-in fsnotify auto-rebuild shipped 2026-05-18; replaces sync on-tick PackAll; new --world.content-watch flag; retires [[rebuild_cheat_close]]'s two open follow-ups
```

- [ ] **Step 4: Commit (memory files only — not tracked by git, so skipped in repo). The repo-side spec back-reference is OK to skip since we already have the close memory linking back to the spec.**

The repo does not version the user's memory dir. No git commit needed for Task 22's memory updates. If you find you do want to record the closure on the repo side, add a one-line entry to a CHANGELOG-equivalent — but this project doesn't have one, so skip.

---

## Self-Review Checklist (read after writing — do NOT execute as a task)

**1. Spec coverage:**
- §1 scope items: ✅ ContentWatch flag (T1), worker (T4 + T13), watcher (T14 + T20), coordination chans (T3), broadcast helper (T4 + T12), top-of-tick drain (T5), handler replacement (T6).
- §2 layout files: all 11 files in the spec table → covered across T1–T20.
- §3 data types: rebuildResult + Server fields covered in T3 + T4.
- §4 components: worker (T4), watcher (T14), handler (T6), tick drain (T5), lifecycle (T13 + T20), broadcast (T4 + T12).
- §5 failure modes: T8 (pack error), T11 (handler-side coverage of all paths), T19 (quit mid-watcher).
- §6 tests: 4 worker + 5 watcher + 4 handler + 2 broadcast = 15 tests covered.
- §7 deferrals: surfaced in T22 close memory.
- §8 acceptance: T21 verifies build + race + smoke-pack + flag visibility.

**2. Placeholders:** none — every step has concrete code and exact commands.

**3. Type consistency:** `rebuildResult` declared T4, referenced T3 + T7+8+9. `packFn` / `reloadFn` shapes match between Server-field decl (T3) and all test overrides (T7+8+9+11). `canonicalContentSubdirs` declared T14, no re-declaration. `debounceWindow` declared T14, used implicitly via `time.After(debounceWindow)` only in the same file.

**4. Build-state continuity:** T3 intentionally leaves the build broken (rebuildResult undefined); T4 declares it and restores green. The Task 6 commit is the first green commit after that group — clearly noted in T3 step 5 and T4 step 3.

**5. Test coverage matrix:**
| Spec §6 test | Plan task |
|---|---|
| Worker.Request_RunsPackAndPostsResult | T7 |
| Worker.PackError_PostsErrorResult | T8 |
| Worker.BusyCoalesce | T9 |
| Worker.QuitMidIdle | T10 |
| Watcher.FileWrite_Triggers | T15 |
| Watcher.BurstCoalesces | T16 |
| Watcher.NewSubdir_Added | T17 |
| Watcher.NonWatched_Ignored | T18 |
| Watcher.QuitClosesCleanly | T19 |
| Handler.NoContentPath | T11 |
| Handler.DispatchesAndBroadcasts | T11 |
| Handler.Async_HappyPath | T11 |
| Handler.Async_PackError | T11 |
| Broadcast.InvokerAndStaff | T12 |
| Broadcast.FsnotifyTriggered_NoInvoker | T12 |

All 15 spec tests covered.
