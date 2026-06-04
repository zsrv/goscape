# NAI-53 — Full CloseModal Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the full body of TS `Player.closeModal` (Player.ts:741-794) into goscape's `(*Player).CloseModal`, including weak-queue clearing, NAI-52 protect-convergence application, modalState-NONE early-return, COUNTDIALOG/PAUSEBUTTON activeScript-null (closes NAI-52-F1), and per-slot IF_CLOSE trigger-script dispatch.

**Architecture:** Single bundle, 5 implementation tasks + 1 close task. Each task is TDD (failing test → minimal impl → pass → commit). Body changes are additive on top of T2's signature change.

**Tech Stack:** Go 1.26+

**Spec:** `docs/superpowers/specs/2026-05-01-nai-53-closemodal-full-port-design.md`

---

## File Map

**Modify:**
- `pkg/script/active.go` — `script.ActivePlayer` interface: `CloseModal()` → `CloseModal(clearWeakQueue bool)`.
- `pkg/script/runner_test.go` — `mockPlayer.CloseModal` signature update.
- `pkg/script/handlers_interface.go` — `handleIfClose` updates call site.
- `modules/world/player_script.go` — implementation: signature change, body port, new helpers `clearWeakQueue` + `runIfCloseTrigger`.
- `modules/world/tick.go` — refreshModalClose path call site.
- `modules/world/modal_close_test.go` — new tests for body behavior (T3, T4, T5).
- `modules/world/player_script_test.go` — new tests for `clearWeakQueue` helper (T1).

---

## Task 1: `(*Player).clearWeakQueue` helper

**Files:**
- Modify: `modules/world/player_script.go` (add helper near existing queue ops, ~line 80).
- Test: `modules/world/player_script_test.go` (extend).

- [ ] **Step 1.1: Write the failing test**

Add to `modules/world/player_script_test.go`:

```go
// TestClearWeakQueueRemovesOnlyWeakEntries pins (*Player).clearWeakQueue:
// drops every QueueWeak entry from p.queue, preserves relative order
// of remaining entries. Mirrors TS Player.weakQueue.clear() (Player.ts:743).
func TestClearWeakQueueRemovesOnlyWeakEntries(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueStrong},
		{Script: sf, Type: script.QueueWeak},
		{Script: sf, Type: script.QueueNormal},
		{Script: sf, Type: script.QueueWeak},
		{Script: sf, Type: script.QueueLong},
	}

	p.clearWeakQueue()

	if got, want := len(p.queue), 3; got != want {
		t.Fatalf("queue len after clearWeakQueue: got %d, want %d", got, want)
	}
	wantTypes := []script.PlayerQueueType{
		script.QueueStrong, script.QueueNormal, script.QueueLong,
	}
	for i, want := range wantTypes {
		if got := p.queue[i].Type; got != want {
			t.Errorf("queue[%d].Type: got %v, want %v (order must be preserved)", i, got, want)
		}
	}
}

// TestClearWeakQueueEmptyQueueNoOp pins clearWeakQueue is safe on empty queue.
func TestClearWeakQueueEmptyQueueNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.queue = nil

	p.clearWeakQueue()

	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0", len(p.queue))
	}
}

// TestClearWeakQueueAllWeakEntries pins clearWeakQueue empties a queue
// of all-weak entries.
func TestClearWeakQueueAllWeakEntries(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueWeak},
		{Script: sf, Type: script.QueueWeak},
	}

	p.clearWeakQueue()

	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 (all weak entries should be removed)", len(p.queue))
	}
}

// TestClearWeakQueueIdempotent pins repeated clearWeakQueue is a no-op
// after the first call.
func TestClearWeakQueueIdempotent(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueStrong},
		{Script: sf, Type: script.QueueWeak},
	}

	p.clearWeakQueue()
	p.clearWeakQueue()

	if got, want := len(p.queue), 1; got != want {
		t.Errorf("queue len after 2× clearWeakQueue: got %d, want %d", got, want)
	}
	if p.queue[0].Type != script.QueueStrong {
		t.Errorf("queue[0].Type: got %v, want QueueStrong", p.queue[0].Type)
	}
}
```

- [ ] **Step 1.2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestClearWeakQueue -v`
Expected: FAIL with `p.clearWeakQueue undefined (type *Player has no field or method clearWeakQueue)` compile error.

- [ ] **Step 1.3: Add the helper**

Insert into `modules/world/player_script.go` near the existing queue ops (after `EnqueueScriptFile`, before `SetDelayed` — pick a stable location near other queue helpers; suggested: directly after the `playerQueueRequest` struct definition's closing brace, before `SetDelayed`):

```go
// clearWeakQueue removes every QueueWeak entry from p.queue, preserving
// relative order of remaining entries. Mirrors TS
// Player.weakQueue.clear() (Player.ts:743). Goscape unifies all queue
// types into p.queue with a Type discriminator, so "clear weak queue"
// becomes a filter on the Type field.
func (p *Player) clearWeakQueue() {
	out := p.queue[:0]
	for _, req := range p.queue {
		if req.Type != script.QueueWeak {
			out = append(out, req)
		}
	}
	p.queue = out
}
```

- [ ] **Step 1.4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestClearWeakQueue -v`
Expected: 4 PASS.

- [ ] **Step 1.5: Run full module tests for regression check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ ./pkg/script/`
Expected: all PASS.

- [ ] **Step 1.6: Commit**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-53 T1 — add (*Player).clearWeakQueue helper

Filters QueueWeak entries from p.queue, preserving relative order.
Pure additive; no consumers yet (added in T2).

Spec: docs/superpowers/specs/2026-05-01-nai-53-closemodal-full-port-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `CloseModal(clearWeakQueue bool)` signature + invocation

**Files:**
- Modify: `pkg/script/active.go:131-133` (interface declaration).
- Modify: `pkg/script/runner_test.go:408` (mockPlayer impl).
- Modify: `pkg/script/handlers_interface.go:19` (handleIfClose call site).
- Modify: `modules/world/player_script.go:573-579` (CloseModal signature + body).
- Modify: `modules/world/player_script.go:652` (ClearPendingAction call site).
- Modify: `modules/world/tick.go:245` (refreshModalClose path call site).
- Test: `modules/world/modal_close_test.go` (extend).

- [ ] **Step 2.1: Write the failing test**

Add to `modules/world/modal_close_test.go`:

```go
// TestCloseModalClearsWeakQueueWhenTrue pins CloseModal(true) drops weak
// queue entries. Mirrors TS Player.closeModal default arg path
// (Player.ts:742-744).
func TestCloseModalClearsWeakQueueWhenTrue(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueStrong},
		{Script: sf, Type: script.QueueWeak},
	}

	p.CloseModal(true)

	if got, want := len(p.queue), 1; got != want {
		t.Fatalf("queue len: got %d, want %d (weak should be dropped)", got, want)
	}
	if p.queue[0].Type != script.QueueStrong {
		t.Errorf("queue[0].Type: got %v, want QueueStrong", p.queue[0].Type)
	}
}

// TestCloseModalPreservesWeakQueueWhenFalse pins CloseModal(false)
// preserves weak queue entries. Mirrors TS Player.closeModal(false)
// path (Player.ts:2148 caller).
func TestCloseModalPreservesWeakQueueWhenFalse(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueStrong},
		{Script: sf, Type: script.QueueWeak},
	}

	p.CloseModal(false)

	if got, want := len(p.queue), 2; got != want {
		t.Fatalf("queue len: got %d, want %d (weak should be preserved)", got, want)
	}
}
```

- [ ] **Step 2.2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestCloseModalClearsWeakQueue -v`
Expected: FAIL with compile error `too many arguments in call to p.CloseModal`.

- [ ] **Step 2.3: Update interface declaration**

In `pkg/script/active.go`, change:

```go
	// S5f: interface / modal control.

	// CloseModal closes any currently open main/chat/side interface and
	// flags the client to refresh modal state.
	CloseModal()
```

to:

```go
	// S5f: interface / modal control.

	// CloseModal closes any currently open main/chat/side interface and
	// flags the client to refresh modal state. clearWeakQueue=true (TS
	// default) drops weak-queue entries before processing; false
	// preserves them. Mirrors TS Player.closeModal(clearWeakQueue).
	CloseModal(clearWeakQueue bool)
```

- [ ] **Step 2.4: Update mockPlayer in runner_test.go**

In `pkg/script/runner_test.go:408`, change:

```go
func (m *mockPlayer) CloseModal()      { m.lastCloseModalCalls++ }
```

to:

```go
func (m *mockPlayer) CloseModal(bool)  { m.lastCloseModalCalls++ }
```

- [ ] **Step 2.5: Update handleIfClose call site**

In `pkg/script/handlers_interface.go:19`, change:

```go
	s.Self.CloseModal()
```

to:

```go
	s.Self.CloseModal(true)
```

- [ ] **Step 2.6: Update CloseModal signature and body**

In `modules/world/player_script.go:573-579`, change:

```go
// CloseModal clears every modal slot and flags the client to emit
// IF_CLOSE on the next encodeOut pass.
func (p *Player) CloseModal() {
	p.modalMain = -1
	p.modalChat = -1
	p.modalSide = -1
	p.modalState = modalStateNone
	p.refreshModalClose = true
}
```

to:

```go
// CloseModal clears every modal slot and flags the client to emit
// IF_CLOSE on the next encodeOut pass. When clearWeakQueue is true
// (TS default), drops every QueueWeak entry from p.queue before
// processing.
//
// Body is incrementally ported across NAI-53 tasks; this commit
// adds only the clearWeakQueue invocation. T3 ports protect-clear,
// T4 ports NONE early-return + slot-reset gating, T5 ports
// activeScript-null + per-slot IF_CLOSE dispatch.
func (p *Player) CloseModal(clearWeakQueue bool) {
	if clearWeakQueue {
		p.clearWeakQueue()
	}
	p.modalMain = -1
	p.modalChat = -1
	p.modalSide = -1
	p.modalState = modalStateNone
	p.refreshModalClose = true
}
```

- [ ] **Step 2.7: Update ClearPendingAction call site**

In `modules/world/player_script.go:652`, change:

```go
	p.CloseModal()
```

to:

```go
	p.CloseModal(true)
```

- [ ] **Step 2.8: Update tick.go call site**

In `modules/world/tick.go:245`, change:

```go
		p.CloseModal()
```

to:

```go
		p.CloseModal(true)
```

- [ ] **Step 2.9: Run new tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestCloseModal -v`
Expected: 2 new tests PASS; no existing test failures.

- [ ] **Step 2.10: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all PASS.

- [ ] **Step 2.11: Commit**

```bash
git add pkg/script/active.go pkg/script/runner_test.go pkg/script/handlers_interface.go modules/world/player_script.go modules/world/tick.go modules/world/modal_close_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-53 T2 — CloseModal(clearWeakQueue bool) signature

Adds clearWeakQueue parameter (TS default true). When true, invokes
p.clearWeakQueue() before slot processing. Updates 3 callers
(handleIfClose, ClearPendingAction, tick refreshModalClose path) +
script.ActivePlayer interface + mockPlayer to pass true.

Spec: docs/superpowers/specs/2026-05-01-nai-53-closemodal-full-port-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `!p.delayed → activeScript.Protect = false` (NAI-52 convergence)

**Files:**
- Modify: `modules/world/player_script.go` (CloseModal body — add protect-clear block before slot resets).
- Test: `modules/world/modal_close_test.go` (extend).

- [ ] **Step 3.1: Write the failing tests**

Add to `modules/world/modal_close_test.go`:

```go
// TestCloseModalClearsActiveScriptProtectWhenNotDelayed pins
// !delayed && activeScript != nil → activeScript.Protect = false.
// Mirrors TS Player.closeModal !delayed → protect=false branch
// (Player.ts:745-747), applied via NAI-52 convergence (TS this.protect ↔
// goscape activeScript.Protect).
func TestCloseModalClearsActiveScriptProtectWhenNotDelayed(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = false
	p.activeScript = &script.ScriptState{
		Script:  &script.ScriptFile{Name: "running"},
		Protect: true,
	}

	p.CloseModal(true)

	if p.activeScript == nil {
		t.Fatal("activeScript: got nil, want preserved (Suspended/Running scripts not nulled)")
	}
	if p.activeScript.Protect {
		t.Errorf("activeScript.Protect: got true, want false (!delayed should clear)")
	}
	if p.protectedScriptActive() {
		t.Errorf("protectedScriptActive(): got true, want false (NAI-52 convergence)")
	}
}

// TestCloseModalPreservesActiveScriptProtectWhenDelayed pins
// delayed → activeScript.Protect preserved.
// Mirrors TS Player.closeModal `if (!this.delayed)` guard.
func TestCloseModalPreservesActiveScriptProtectWhenDelayed(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = true
	p.activeScript = &script.ScriptState{
		Script:  &script.ScriptFile{Name: "running"},
		Protect: true,
	}

	p.CloseModal(true)

	if p.activeScript == nil {
		t.Fatal("activeScript: got nil, want preserved")
	}
	if !p.activeScript.Protect {
		t.Errorf("activeScript.Protect: got false, want true (delayed should preserve)")
	}
}

// TestCloseModalNilActiveScriptNoPanic pins !delayed + nil activeScript
// is a no-op (no panic). Mirrors TS where `this.protect = false` is a
// no-op when no script is suspended.
func TestCloseModalNilActiveScriptNoPanic(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = false
	p.activeScript = nil

	// Should not panic.
	p.CloseModal(true)

	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil")
	}
}
```

- [ ] **Step 3.2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestCloseModalClearsActiveScriptProtect -v`
Expected: FAIL — `activeScript.Protect: got true, want false` (the body has no protect-clear branch yet).

The other two tests may pass already (delayed-preserve case is currently true because we never touch Protect; nil case has no panic in current body). That's fine — write all three together, run all three, fix the failing one.

- [ ] **Step 3.3: Add the protect-clear block**

In `modules/world/player_script.go`, update CloseModal body:

```go
func (p *Player) CloseModal(clearWeakQueue bool) {
	if clearWeakQueue {
		p.clearWeakQueue()
	}
	if !p.delayed && p.activeScript != nil {
		p.activeScript.Protect = false
	}
	p.modalMain = -1
	p.modalChat = -1
	p.modalSide = -1
	p.modalState = modalStateNone
	p.refreshModalClose = true
}
```

- [ ] **Step 3.4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestCloseModal -v`
Expected: all PASS.

- [ ] **Step 3.5: Run full module tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ ./pkg/script/`
Expected: all PASS.

- [ ] **Step 3.6: Commit**

```bash
git add modules/world/player_script.go modules/world/modal_close_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-53 T3 — CloseModal clears activeScript.Protect when not delayed

Ports TS Player.closeModal !delayed → protect=false branch
(Player.ts:745-747) via NAI-52 convergence: TS this.protect ↔ goscape
activeScript.Protect. When p.delayed is false and p.activeScript is
non-nil, sets p.activeScript.Protect = false.

Spec: docs/superpowers/specs/2026-05-01-nai-53-closemodal-full-port-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `modalState == NONE` early-return + gated slot reset

**Files:**
- Modify: `modules/world/player_script.go` (CloseModal body — wrap slot resets in non-NONE branch, add early-return).
- Test: `modules/world/modal_close_test.go` (extend).

- [ ] **Step 4.1: Write the failing tests**

Add to `modules/world/modal_close_test.go`:

```go
// TestCloseModalNoneEarlyReturnPreservesRefreshModalClose pins
// modalState == NONE early-return. When no modal is open, CloseModal
// must NOT touch refreshModalClose (avoids redundant wire IF_CLOSE).
// Mirrors TS Player.closeModal `if (modalState === NONE) return`
// (Player.ts:749-751).
func TestCloseModalNoneEarlyReturnPreservesRefreshModalClose(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateNone
	p.modalMain = -1
	p.modalChat = -1
	p.modalSide = -1
	p.refreshModalClose = false

	p.CloseModal(true)

	if p.refreshModalClose {
		t.Errorf("refreshModalClose: got true, want false (NONE state must early-return)")
	}
}

// TestCloseModalNonNoneResetsAllSlots pins that with any modal open,
// all three slots are reset to -1, modalState becomes NONE, and
// refreshModalClose is set true.
func TestCloseModalNonNoneResetsAllSlots(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateMain
	p.modalMain = 42
	p.modalChat = 88
	p.modalSide = 99
	p.refreshModalClose = false

	p.CloseModal(true)

	if p.modalMain != -1 {
		t.Errorf("modalMain: got %d, want -1", p.modalMain)
	}
	if p.modalChat != -1 {
		t.Errorf("modalChat: got %d, want -1", p.modalChat)
	}
	if p.modalSide != -1 {
		t.Errorf("modalSide: got %d, want -1", p.modalSide)
	}
	if p.modalState != modalStateNone {
		t.Errorf("modalState: got %#x, want %#x (NONE)", p.modalState, modalStateNone)
	}
	if !p.refreshModalClose {
		t.Errorf("refreshModalClose: got false, want true (modal was open)")
	}
}

// TestCloseModalNoneEarlyReturnStillRunsClearWeakQueueAndProtect pins
// the early-return is positioned AFTER weak-queue clearing and the
// !delayed protect-clear (TS Player.ts:742-748 — both run before the
// modalState check).
func TestCloseModalNoneEarlyReturnStillRunsClearWeakQueueAndProtect(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueWeak},
	}
	p.delayed = false
	p.activeScript = &script.ScriptState{
		Script:  &script.ScriptFile{Name: "running"},
		Protect: true,
	}
	p.modalState = modalStateNone

	p.CloseModal(true)

	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 (weak should be cleared even on NONE early-return)", len(p.queue))
	}
	if p.activeScript == nil || p.activeScript.Protect {
		t.Errorf("activeScript.Protect should be cleared even on NONE early-return")
	}
}
```

- [ ] **Step 4.2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestCloseModalNone -v`
Expected: FAIL — at least `TestCloseModalNoneEarlyReturnPreservesRefreshModalClose` fails (current body unconditionally sets `refreshModalClose=true`).

- [ ] **Step 4.3: Update CloseModal body**

In `modules/world/player_script.go`, replace CloseModal body with:

```go
func (p *Player) CloseModal(clearWeakQueue bool) {
	if clearWeakQueue {
		p.clearWeakQueue()
	}
	if !p.delayed && p.activeScript != nil {
		p.activeScript.Protect = false
	}

	if p.modalState == modalStateNone {
		return
	}

	p.modalState = modalStateNone

	p.modalMain = -1
	p.modalChat = -1
	p.modalSide = -1
	p.refreshModalClose = true
}
```

- [ ] **Step 4.4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestCloseModal -v`
Expected: all PASS.

- [ ] **Step 4.5: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all PASS. (Pay attention: tests like `TestModalCloseEmitsStopTransmit` set `refreshModalClose=true` directly without going through CloseModal — they should still pass. If a test fails because it relied on the old "CloseModal always sets refreshModalClose=true even on NONE" behavior, it must be updated to set `modalState=modalStateMain` (or similar non-NONE) before calling CloseModal, matching TS semantics.)

- [ ] **Step 4.6: Commit**

```bash
git add modules/world/player_script.go modules/world/modal_close_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-53 T4 — CloseModal NONE-state early-return + gated slot reset

Ports TS Player.closeModal `if (modalState === NONE) return`
(Player.ts:749-751). Slot resets and refreshModalClose=true now gated
on non-NONE state. clearWeakQueue + protect-clear still run
unconditionally before the early-return (matches TS dispatch order
Player.ts:742-748).

Observable wire-behavior delta: redundant CloseModal calls (no modal
open) no longer emit IF_CLOSE on next encodeOut. More faithful to TS.

Spec: docs/superpowers/specs/2026-05-01-nai-53-closemodal-full-port-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: COUNTDIALOG/PAUSEBUTTON activeScript-null + per-slot IF_CLOSE dispatch

**Files:**
- Modify: `modules/world/player_script.go` (CloseModal body — add activeScript-null branch, add per-slot dispatch helper).
- Test: `modules/world/modal_close_test.go` (extend).

- [ ] **Step 5.1: Write the failing tests**

Add to `modules/world/modal_close_test.go`:

```go
// TestCloseModalNullsActiveScriptOnCountDialog pins COUNTDIALOG-suspended
// activeScript is nulled on CloseModal. Closes NAI-52-F1.
// Mirrors TS Player.closeModal Player.ts:756-758.
func TestCloseModalNullsActiveScriptOnCountDialog(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateChat
	p.modalChat = 7
	p.activeScript = &script.ScriptState{
		Script:    &script.ScriptFile{Name: "dialog"},
		Execution: script.CountDialog,
	}

	p.CloseModal(true)

	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (COUNTDIALOG must be cleared)")
	}
}

// TestCloseModalNullsActiveScriptOnPauseButton pins PAUSEBUTTON-suspended
// activeScript is nulled on CloseModal. Closes NAI-52-F1.
func TestCloseModalNullsActiveScriptOnPauseButton(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateChat
	p.modalChat = 7
	p.activeScript = &script.ScriptState{
		Script:    &script.ScriptFile{Name: "pause"},
		Execution: script.PauseButton,
	}

	p.CloseModal(true)

	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (PAUSEBUTTON must be cleared)")
	}
}

// TestCloseModalPreservesActiveScriptOnSuspended pins Suspended (non-dialog)
// activeScript is preserved on CloseModal. Mirrors TS exclusion of
// non-COUNTDIALOG/PAUSEBUTTON execution states from the null branch.
func TestCloseModalPreservesActiveScriptOnSuspended(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = true // delayed so the protect-clear block doesn't fire
	p.modalState = modalStateChat
	p.modalChat = 7
	state := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "suspended"},
		Execution: script.Suspended,
		Protect:   true,
	}
	p.activeScript = state

	p.CloseModal(true)

	if p.activeScript != state {
		t.Errorf("activeScript: got %v, want preserved %v (Suspended must NOT be cleared)", p.activeScript, state)
	}
}

// TestCloseModalIfCloseDispatchMain pins per-slot IF_CLOSE dispatch
// for modalMain. Mirrors TS Player.closeModal:761-769.
func TestCloseModalIfCloseDispatchMain(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	ifCloseScript := &script.ScriptFile{
		Name:        "[if_close,42]",
		LookupKey:   script.LookupKeyForType(script.TriggerIfClose, 42),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(ifCloseScript)
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.modalState = modalStateMain
	p.modalMain = 42

	p.CloseModal(true)

	if p.modalMain != -1 {
		t.Errorf("modalMain: got %d, want -1", p.modalMain)
	}
	// Script is registered, so dispatch path was taken; OpReturn finishes
	// immediately so activeScript is nil.
	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (IF_CLOSE script returned)")
	}
}

// TestCloseModalIfCloseDispatchChat pins per-slot IF_CLOSE dispatch
// for modalChat (slot lookup uses modalChat com ID).
func TestCloseModalIfCloseDispatchChat(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	ifCloseScript := &script.ScriptFile{
		Name:        "[if_close,88]",
		LookupKey:   script.LookupKeyForType(script.TriggerIfClose, 88),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(ifCloseScript)
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.modalState = modalStateChat
	p.modalChat = 88

	p.CloseModal(true)

	if p.modalChat != -1 {
		t.Errorf("modalChat: got %d, want -1", p.modalChat)
	}
}

// TestCloseModalIfCloseDispatchSide pins per-slot IF_CLOSE dispatch
// for modalSide.
func TestCloseModalIfCloseDispatchSide(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	ifCloseScript := &script.ScriptFile{
		Name:        "[if_close,99]",
		LookupKey:   script.LookupKeyForType(script.TriggerIfClose, 99),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(ifCloseScript)
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.modalState = modalStateSide
	p.modalSide = 99

	p.CloseModal(true)

	if p.modalSide != -1 {
		t.Errorf("modalSide: got %d, want -1", p.modalSide)
	}
}

// TestCloseModalIfCloseMissingScriptNoOp pins that an open slot with no
// registered IF_CLOSE script is a silent no-op (slot still resets, no
// panic). Mirrors TS where `if (closeTrigger)` guards the executeScript.
func TestCloseModalIfCloseMissingScriptNoOp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.modalState = modalStateMain
	p.modalMain = 42

	// Should not panic.
	p.CloseModal(true)

	if p.modalMain != -1 {
		t.Errorf("modalMain: got %d, want -1", p.modalMain)
	}
}

// TestCloseModalNilScriptProviderNoOp pins that nil scriptProvider is
// a silent no-op (slots still reset). Defensive — covers test paths
// that don't seed scriptProvider.
func TestCloseModalNilScriptProviderNoOp(t *testing.T) {
	s := newTestServer(t)
	// s.scriptProvider intentionally nil.
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.modalState = modalStateMain
	p.modalMain = 42

	// Should not panic.
	p.CloseModal(true)

	if p.modalMain != -1 {
		t.Errorf("modalMain: got %d, want -1", p.modalMain)
	}
}
```

- [ ] **Step 5.2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestCloseModal -v`
Expected: at least the COUNTDIALOG/PAUSEBUTTON null tests FAIL (body has no activeScript-null branch yet); IF_CLOSE dispatch tests are slot-reset-only, may pass on slot-reset alone but must validate that the new dispatch helper exists once added.

- [ ] **Step 5.3: Add the per-slot dispatch helper**

In `modules/world/player_script.go`, add the helper (suggested location: directly after `CloseModal`):

```go
// runIfCloseTrigger looks up TriggerIfClose for slotCom and runs it
// if found. Mirrors TS Player.closeModal per-slot
// `executeScript(ScriptRunner.init(closeTrigger, this), false)`
// (Player.ts:761-769, 772-780, 783-791).
//
// Nil-safe on s.scriptProvider; runScript is itself nil-safe on the
// returned ScriptFile.
func (p *Player) runIfCloseTrigger(s *Server, slotCom int) {
	if s.scriptProvider == nil {
		return
	}
	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerIfClose, slotCom, -1)
	s.runScript(sf, p, nil, false, nil, nil)
}
```

- [ ] **Step 5.4: Update CloseModal body to add activeScript-null + per-slot dispatch**

In `modules/world/player_script.go`, replace CloseModal body with:

```go
func (p *Player) CloseModal(clearWeakQueue bool) {
	if clearWeakQueue {
		p.clearWeakQueue()
	}
	if !p.delayed && p.activeScript != nil {
		p.activeScript.Protect = false
	}

	if p.modalState == modalStateNone {
		return
	}

	p.modalState = modalStateNone

	// Close any input-dialogue suspended scripts. NAI-52-F1.
	if p.activeScript != nil &&
		(p.activeScript.Execution == script.CountDialog ||
			p.activeScript.Execution == script.PauseButton) {
		p.activeScript = nil
	}

	// Per-slot IF_CLOSE dispatch (Main → Chat → Side, TS order).
	//
	// DEVIATION NAI-53-D-CLEARCOMLISTENERS-PER-SLOT: TS calls
	// clearComListeners(slotCom) per-slot, filtering invListeners by
	// Component.rootLayer. Goscape's encodeOut clears ALL invListeners
	// globally when refreshModalClose is set; per-slot rootLayer
	// filtering blocked on unported Component config registry.
	if p.client != nil && p.client.server != nil {
		s := p.client.server
		if p.modalMain != -1 {
			p.runIfCloseTrigger(s, p.modalMain)
			p.modalMain = -1
		}
		if p.modalChat != -1 {
			p.runIfCloseTrigger(s, p.modalChat)
			p.modalChat = -1
		}
		if p.modalSide != -1 {
			p.runIfCloseTrigger(s, p.modalSide)
			p.modalSide = -1
		}
	} else {
		// No server (test path with no Server bound) — still reset slots.
		p.modalMain = -1
		p.modalChat = -1
		p.modalSide = -1
	}

	p.refreshModalClose = true
}
```

- [ ] **Step 5.5: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestCloseModal -v`
Expected: all PASS.

- [ ] **Step 5.6: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all PASS.

- [ ] **Step 5.7: Verify implementer commit content matches stated diff**

Per memory `implementer_commit_content_verify.md`:
Run: `git status` and `git diff --stat HEAD`
Expected: changes only in `modules/world/player_script.go` (CloseModal body + new `runIfCloseTrigger` helper) and `modules/world/modal_close_test.go` (new tests). No stray edits.

- [ ] **Step 5.8: Commit**

```bash
git add modules/world/player_script.go modules/world/modal_close_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-53 T5 — CloseModal nulls activeScript on dialog suspends + per-slot IF_CLOSE dispatch

Ports remaining TS Player.closeModal body (Player.ts:756-794):

- COUNTDIALOG/PAUSEBUTTON activeScript → nil (closes NAI-52-F1).
- Per-slot IF_CLOSE trigger-script dispatch via new
  (*Player).runIfCloseTrigger helper. Order: Main → Chat → Side
  (TS-faithful).

Tags DEVIATION NAI-53-D-CLEARCOMLISTENERS-PER-SLOT: TS per-slot
clearComListeners(rootLayer-filtered) replaced by goscape's existing
global invListener clear in encodeOut; faithful per-slot port blocks
on unported Component config registry.

Spec: docs/superpowers/specs/2026-05-01-nai-53-closemodal-full-port-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Bundle close — memory updates + tally

**Files:**
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (close NAI-52-F1; add NAI-53 close section).

- [ ] **Step 6.1: Verify all earlier tasks landed**

Run: `git log --oneline -7`
Expected: 5 commits since `07490ed` (NAI-52 close), one per T1–T5.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all PASS.

- [ ] **Step 6.2: Update nai_followups.md**

Append to `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`:

```markdown

## NAI-53 — CLOSED 2026-05-01

**Scope:** Full port of TS `Player.closeModal` (Player.ts:741-794) into goscape's `(*Player).CloseModal`. Adds `clearWeakQueue` parameter, weak-queue clearing, NAI-52 protect-convergence application (`!delayed → activeScript.Protect=false`), `modalState==NONE` early-return, COUNTDIALOG/PAUSEBUTTON activeScript-null, and per-slot IF_CLOSE trigger-script dispatch (Main → Chat → Side).

**Cadence:** Full sub-spec, single bundle, 5 implementation tasks.

**Close commit:** `<close-sha>` (T1: `<t1-sha>`, T2: `<t2-sha>`, T3: `<t3-sha>`, T4: `<t4-sha>`, T5: `<t5-sha>`).

**Spec:** `docs/superpowers/specs/2026-05-01-nai-53-closemodal-full-port-design.md`.
**Plan:** `docs/superpowers/plans/2026-05-01-nai-53-closemodal-full-port.md`.

**Follow-ups closed:**
- NAI-52-F1 (CloseModal nulls activeScript on COUNTDIALOG/PAUSEBUTTON).

**Deviations opened:**
- NAI-53-D-CLEARCOMLISTENERS-PER-SLOT — `(*Player).CloseModal` does NOT call a per-slot rootLayer-filtered `clearComListeners`. Goscape relies on `encodeOut`'s global `invListeners` clear when `refreshModalClose` is set. Per-slot port requires unported Component config registry. Closure: future Component-config sub-spec.

**Deviations closed:** none.

**Deviation tally:** 20 → 21 (+1).

**Follow-up candidates:**
- **NAI-53-F1** — `(*Server).resumeOrFinish` (`modules/world/script.go:100+`) does NOT call `CloseModal(false)` on Suspended/non-MAIN-modal completion (TS `Player.ts:2148`). NAI-53 added the `false` arg to the API but no caller wires it. **Closure:** future executeScript-completion sub-spec.

**Observable wire-behavior delta from NAI-53:** redundant CloseModal calls (modalState already NONE) no longer set `refreshModalClose=true`, so no spurious wire IF_CLOSE on next encodeOut. More faithful to TS.
```

(Substitute actual commit SHAs from `git log --format=%H -5` after T5 lands.)

- [ ] **Step 6.3: Update Follow-up entry on NAI-52-F1**

In the existing `## NAI-52 — CLOSED 2026-05-01` block of `nai_followups.md`, change the NAI-52-F1 entry's "Closure:" line from `future modal-close fidelity sub-spec` to `Closed by NAI-53.`

- [ ] **Step 6.4: Verify memory file integrity**

Read the appended NAI-53 close section back; confirm headers, list-bullet syntax, and SHA placeholders are all replaced.

- [ ] **Step 6.5: Final verification**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all PASS.

Run: `git status`
Expected: clean working tree (memory file lives outside repo).

- [ ] **Step 6.6: Bundle close commit**

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-53 — full CloseModal port; closes NAI-52-F1

5 commits land the full TS Player.closeModal port (Player.ts:741-794):
clearWeakQueue helper + signature + protect-convergence + NONE
early-return + activeScript-null + per-slot IF_CLOSE dispatch.

Closes follow-up: NAI-52-F1 (CloseModal nulls activeScript on
COUNTDIALOG/PAUSEBUTTON).

Opens deviation: NAI-53-D-CLEARCOMLISTENERS-PER-SLOT (per-slot
rootLayer filtering blocks on unported Component config; goscape's
existing global encodeOut clear of invListeners covers the broader
effect).

Defers follow-up: NAI-53-F1 (resumeOrFinish does not yet call
CloseModal(false) on non-MAIN suspends; TS Player.ts:2148 does).

Tally: 20 → 21 (+1).

Spec: docs/superpowers/specs/2026-05-01-nai-53-closemodal-full-port-design.md
Plan: docs/superpowers/plans/2026-05-01-nai-53-closemodal-full-port.md

Closes memory: NAI-53 close section in nai_followups.md.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review checklist

After completing all tasks, the controller should verify:

1. **Spec coverage**: every spec In-scope item maps to a task (T1: clearWeakQueue helper; T2: signature + invocation + 5 sites; T3: protect-clear; T4: NONE early-return + slot reset gating; T5: activeScript-null + per-slot IF_CLOSE).
2. **Deviation tag**: `// DEVIATION NAI-53-D-CLEARCOMLISTENERS-PER-SLOT` comment lives in CloseModal body (T5).
3. **No stale CloseModal()-without-arg call sites**: `grep -rn "\.CloseModal()" --include='*.go'` returns zero hits.
4. **Mock + interface in lockstep**: `pkg/script/active.go:CloseModal` signature matches `pkg/script/runner_test.go:mockPlayer.CloseModal`.
5. **NAI-52 convergence preserved**: `(*Player).protectedScriptActive` still returns the AND of `activeScript != nil && activeScript.Protect`. T3 mutates the inner Protect field, so the predicate still gives the right answer.
6. **Dispatch order is Main → Chat → Side** (TS order, Player.ts:761/772/783): code review confirms the three `if p.modalXxx != -1` blocks in CloseModal appear in that exact order. (Not pinned by an explicit runtime test because the existing test infrastructure has no easy way to record per-script invocation order without elaborate bytecode fixtures; the per-slot tests + code-review pin together cover it.)
7. **Memory tally arithmetic**: 20 → 21 documented in nai_followups.md NAI-53 close section.
