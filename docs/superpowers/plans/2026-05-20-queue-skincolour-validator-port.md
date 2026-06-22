# Queue + SkinColour validator port — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Always invoke the `use-modern-go` skill at the start of every implementer dispatch.

**Goal:** Port the two remaining `ScriptInputRangeValidator` entries from TS `ScriptValidators.ts` (`QueueValid [0, 19]`, `SkinColourValid [0, 7]`) into `checkQueue` / `checkSkinColour` free-functions in `pkg/script/`. Wire at `NPC_QUEUE`, `NPC_WALKTRIGGER`, `SETSKINCOLOUR`. Queue receives a deliberate TS-literal range shift from goscape's pre-existing `[1, 20]` to `[0, 19]`, inheriting the upstream TS fencepost bug (pinned by `NAI-QUEUE-D-TS-FENCEPOST-INHERITED`).

**Architecture:** Bare-number free-function checkers in the existing `pkg/script/handlers_npc.go` / `pkg/script/handlers_player.go` sibling style (analog: `checkHuntVis`, `checkNotNull`, `checkHitType`). No new `pkg/objtype/` files — TS has no named enum constants for either validator (only string labels `'AIQueue'` / `'SkinColour'`). Three handler call-site wraps; five doc-comment refreshes (production + test); three existing-test rewrites; six new tests.

**Tech Stack:** Go 1.26. Project conventions per `CLAUDE.md`: prefix Go commands with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`; PATH set via `unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"` if needed; commits use `git commit --no-gpg-sign`; stage explicitly (the working tree has standing noise — `config.yaml`, untracked dotfiles, `RUNESCRIPT.md` — **never stage these**). Spec: `docs/superpowers/specs/2026-05-20-queue-skincolour-validator-design.md` (commit `3750ac6e`).

---

## File Structure

| File | Status | Purpose |
|---|---|---|
| `pkg/script/handlers_npc.go` | **MODIFY** | Add `checkQueue` (insert after `checkHitType` at L104, before `checkHuntVis` at L109). Replace inline range check in `handleNpcQueue` (L491-494). Replace inline range check in `handleNpcWalkTrigger` (L562-565). Refresh doc comments at L475-481 + L550-556. |
| `pkg/script/handlers_npc_test.go` | **MODIFY** | Add `TestCheckQueue_Range` (insert near existing `TestCheckHitType`). Rewrite `TestHandleNpcQueueInvalidQueueIDErrors` (L979). Rewrite `TestNpcWalkTrigger_QueueIDBelowOne_Errors` (L3109) + `TestNpcWalkTrigger_QueueIDAboveTwenty_Errors` (L3128). Add 4 new boundary tests: `TestHandleNpcQueueAcceptsZeroEdge`, `TestHandleNpcQueueRejectsTwenty`, `TestHandleNpcWalkTriggerAcceptsZeroEdge`, `TestHandleNpcWalkTriggerRejectsTwenty`. Refresh doc comments on `TestHandleNpcQueueEnqueues` (L917) + `TestHandleNpcQueueNullDelayRejected` (L1630). |
| `pkg/script/handlers_player.go` | **MODIFY** | Add `checkSkinColour` (insert after `checkNotNull` at L89). Replace inline range check in `handleSetSkinColour` (L1661-1664). Append validator-citation sentence to doc comment at L1648-1656. |
| `pkg/script/handlers_player_test.go` | **MODIFY** | Add `TestCheckSkinColour_Range` (near `checkNotNull`-adjacent test if present, else near `TestHandleSetSkinColour_RejectsOutOfRange` at L5101). |

No files created or deleted. No `pkg/objtype/` changes.

---

## Pre-flight

- [ ] **Step 0: Verify clean working state**

Run:
```bash
cd $HOME/Code/github.com/zsrv/goscape
git log --oneline -2
git status
```

Expected: HEAD shows `3750ac6e docs(spec): Queue + SkinColour validator port (TS-literal range shift)` on top of `7965eda1 chore(close): HitType validator port + NpcStat read-path validator coverage`. `git status` shows only `config.yaml` modified plus standing untracked noise (`.bash_profile`, `.bashrc`, `.claude/`, `.gitconfig`, `.gitmodules`, `.mcp.json`, `.profile`, `.ripgreprc`, `.vscode`, `.zprofile`, `.zshrc`, `RUNESCRIPT.md`). **Do not stage or modify any of that noise.**

- [ ] **Step 1: Establish baseline gate**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/script/... 2>&1 | tail -5
```

Expected: all packages OK. If anything fails BEFORE you start, stop and report — that's not your fault to fix.

- [ ] **Step 2: Pre-commit safety reminder**

Before EVERY commit in this plan, run `git status` first to confirm only your intended files are staged, and `git show --stat HEAD` after to confirm the commit landed cleanly. See memory `[[git-pre-commit-status-check]]`: concurrent shell activity can stage things between session-start and `git commit`; the safest recovery for an accidental stage is `git reset --mixed HEAD~1`, never `--amend`.

---

## Task 1: `checkQueue` validator + unit test

**Files:**
- Modify: `pkg/script/handlers_npc.go` (insert new function after `checkHitType`, ~L104)
- Modify: `pkg/script/handlers_npc_test.go` (insert new test near existing `TestCheckHitType`)

- [ ] **Step 1: Locate the sibling-validator test cluster**

Run:
```bash
grep -n "TestCheckHitType\|TestCheckHuntVis\|TestCheckCategoryType\|TestCheckNpcStatID" pkg/script/handlers_npc_test.go | head -10
```

Note the line numbers; the new `TestCheckQueue_Range` goes immediately AFTER `TestCheckHitType` (which lives near the other `TestCheck*` validators).

- [ ] **Step 2: Write the failing test**

Insert the following into `pkg/script/handlers_npc_test.go` immediately after the closing `}` of `TestCheckHitType`:

```go
// TestCheckQueue_Range pins the [0, 19] inclusive range check. Mirrors
// TS QueueValid (ScriptValidators.ts:114) —
// ScriptInputRangeValidator(0, 19, 'AIQueue').
func TestCheckQueue_Range(t *testing.T) {
	for _, v := range []int{0, 1, 10, 19} {
		if err := checkQueue(v, "TEST_OP"); err != nil {
			t.Errorf("checkQueue(%d): unexpected error %v", v, err)
		}
	}
	for _, v := range []int{-1, 20, 21, math.MinInt, math.MaxInt} {
		err := checkQueue(v, "TEST_OP")
		if err == nil {
			t.Errorf("checkQueue(%d): want error, got nil", v)
			continue
		}
		if !strings.Contains(err.Error(), "TEST_OP") {
			t.Errorf("checkQueue(%d): error %q missing op name TEST_OP", v, err)
		}
	}
}
```

If `"math"` is not already imported in this file, add it to the import block. (`"strings"` is almost certainly already imported; verify with `grep -n "\"strings\"\|\"math\"" pkg/script/handlers_npc_test.go | head -5`.)

- [ ] **Step 3: Run test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestCheckQueue_Range -count=1 2>&1 | tail -10
```

Expected: FAIL with compile error mentioning undefined `checkQueue`. If you instead see "undefined: math.MinInt" or "math imported and not used", fix the imports first and re-run; the compile error MUST be about `checkQueue` being undefined.

- [ ] **Step 4: Add the validator**

Open `pkg/script/handlers_npc.go`. Find `checkHitType` (around L99-104). Insert the following IMMEDIATELY after its closing `}` (i.e., after the line `}` at L104, before the blank line preceding `checkHuntVis`):

```go

// checkQueue validates an AI-queue identifier. Mirrors TS QueueValid
// (ScriptValidators.ts:114) — ScriptInputRangeValidator(0, 19, 'AIQueue'),
// inclusive range [0, 19]. Note: the call-site arithmetic at NPC_QUEUE /
// NPC_WALKTRIGGER then subtracts 1 to index TriggerAiQueue1..20, which
// means queueId=0 produces TriggerAiQueue1-1 (a garbage trigger one
// before AI_QUEUE1). That fencepost is inherited from upstream TS and is
// not exercised by any LostCityRS/Content script (audit: real callers
// push 1..12); the inherited bug is pinned by
// NAI-QUEUE-D-TS-FENCEPOST-INHERITED.
func checkQueue(v int, op string) error {
	if v < 0 || v > 19 {
		return fmt.Errorf("%s: queue id out of range (%d)", op, v)
	}
	return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestCheckQueue_Range -count=1 -v 2>&1 | tail -10
```

Expected: `--- PASS: TestCheckQueue_Range` + `ok  github.com/zsrv/goscape/pkg/script`.

- [ ] **Step 6: Verify no broader regression**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1 2>&1 | tail -5
```

Expected: `ok  github.com/zsrv/goscape/pkg/script` with NO failures. The existing `TestHandleNpcQueueInvalidQueueIDErrors`, `TestNpcWalkTrigger_QueueIDBelowOne_Errors`, and `TestNpcWalkTrigger_QueueIDAboveTwenty_Errors` must STILL be green — production code hasn't changed yet, only the new validator function was added.

- [ ] **Step 7: Commit**

```bash
git status
git add pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go
git diff --cached --stat
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): add checkQueue validator (T1)

New checkQueue(v, op) error free-function in pkg/script/handlers_npc.go
alongside checkHitType / checkHuntVis. Validates inclusive range [0, 19]
mirroring TS QueueValid (ScriptValidators.ts:114). Doc comment pins
NAI-QUEUE-D-TS-FENCEPOST-INHERITED for the inherited upstream-TS
queueId=0 → garbage-trigger fencepost (unused by LostCityRS/Content).

Wired by T3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

Expected: `2 files changed`, ~30 insertions.

---

## Task 2: `checkSkinColour` validator + unit test

**Files:**
- Modify: `pkg/script/handlers_player.go` (insert new function after `checkNotNull` at L89)
- Modify: `pkg/script/handlers_player_test.go` (insert new test near the existing skincolour test cluster around L5101 or near any `TestCheckNotNull` if present)

- [ ] **Step 1: Locate insertion point for the test**

Run:
```bash
grep -n "TestCheckNotNull\|TestHandleSetSkinColour" pkg/script/handlers_player_test.go | head -5
```

If `TestCheckNotNull` exists, place `TestCheckSkinColour_Range` immediately after it. Otherwise place it immediately BEFORE `TestHandleSetSkinColour_WritesColors4` (at L5061 per the spec baseline).

- [ ] **Step 2: Write the failing test**

Insert the following into `pkg/script/handlers_player_test.go` at the chosen location:

```go
// TestCheckSkinColour_Range pins the [0, 7] inclusive range check.
// Mirrors TS SkinColourValid (ScriptValidators.ts:137) —
// ScriptInputRangeValidator(0, 7, 'SkinColour').
func TestCheckSkinColour_Range(t *testing.T) {
	for _, v := range []int{0, 1, 4, 7} {
		if err := checkSkinColour(v, "TEST_OP"); err != nil {
			t.Errorf("checkSkinColour(%d): unexpected error %v", v, err)
		}
	}
	for _, v := range []int{-1, 8, 100, math.MinInt} {
		err := checkSkinColour(v, "TEST_OP")
		if err == nil {
			t.Errorf("checkSkinColour(%d): want error, got nil", v)
			continue
		}
		if !strings.Contains(err.Error(), "TEST_OP") {
			t.Errorf("checkSkinColour(%d): error %q missing op name TEST_OP", v, err)
		}
	}
}
```

If `"math"` is not already imported in this file, add it to the import block. (`"strings"` is almost certainly already imported.)

- [ ] **Step 3: Run test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestCheckSkinColour_Range -count=1 2>&1 | tail -10
```

Expected: FAIL with compile error mentioning undefined `checkSkinColour`.

- [ ] **Step 4: Add the validator**

Open `pkg/script/handlers_player.go`. Find `checkNotNull` (L80-89). Insert the following IMMEDIATELY after the closing `}` of `checkNotNull` (i.e., after L89, before the blank line preceding `checkLocAngle`):

```go

// checkSkinColour validates a player skin-colour wire value. Mirrors TS
// SkinColourValid (ScriptValidators.ts:137) —
// ScriptInputRangeValidator(0, 7, 'SkinColour'), inclusive range [0, 7].
func checkSkinColour(v int, op string) error {
	if v < 0 || v > 7 {
		return fmt.Errorf("%s: skin colour out of range (%d)", op, v)
	}
	return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestCheckSkinColour_Range -count=1 -v 2>&1 | tail -10
```

Expected: `--- PASS: TestCheckSkinColour_Range`.

- [ ] **Step 6: Verify no broader regression**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1 2>&1 | tail -5
```

Expected: `ok  github.com/zsrv/goscape/pkg/script`. Production code hasn't changed yet; existing tests still green.

- [ ] **Step 7: Commit**

```bash
git status
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go
git diff --cached --stat
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): add checkSkinColour validator (T2)

New checkSkinColour(v, op) error free-function in
pkg/script/handlers_player.go alongside checkNotNull. Validates inclusive
range [0, 7] mirroring TS SkinColourValid (ScriptValidators.ts:137).

Wired by T4.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

Expected: `2 files changed`, ~25 insertions.

---

## Task 3: Wire `checkQueue` at NPC_QUEUE + NPC_WALKTRIGGER + test rewrites + doc refresh

This task contains the **range-shift behavior change** for both Queue call sites. The existing tests fail under the new range; we update tests FIRST (RED) then production (GREEN) per TDD. Two call sites are bundled because they share `checkQueue` and the changes are mechanically symmetric.

**Files:**
- Modify: `pkg/script/handlers_npc.go` — replace inline checks at L491-494 (`handleNpcQueue`) and L562-565 (`handleNpcWalkTrigger`); refresh doc comments at L475-481 + L550-556
- Modify: `pkg/script/handlers_npc_test.go` — rewrite 3 existing tests, add 4 new boundary tests, refresh 2 doc comments

### 3A. NPC_QUEUE site

- [ ] **Step 1: Rewrite the existing invalid-id test for the new range**

Find `TestHandleNpcQueueInvalidQueueIDErrors` at `pkg/script/handlers_npc_test.go:979-1015`. Replace the entire function body with:

```go
// TestHandleNpcQueueOutOfRangeErrors — queueID out of [0, 19]. Pins the
// TS-literal range shift (was [1, 20] pre-NAI-QUEUE-D port).
func TestHandleNpcQueueOutOfRangeErrors(t *testing.T) {
	cases := []struct {
		name    string
		queueID int32
		wantErr string
	}{
		{"negative", -1, "NPC_QUEUE: queue id out of range (-1)"},
		{"twenty", 20, "NPC_QUEUE: queue id out of range (20)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			npc := &mockNpc{}
			sf := &ScriptFile{
				Name: "npc_queue_invalid_id",
				Opcodes: []Opcode{
					OpPushConstantInt, // queueID
					OpPushConstantInt, // arg
					OpPushConstantInt, // delay
					OpNpcQueue,
					OpReturn,
				},
				IntOperands: []int32{tc.queueID, 0, 0, 0, 0},
			}
			state := Init(sf, nil, false, nil, nil)
			state.ActiveNpc = npc
			state.Pointers |= PtrActiveNpc

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error, got nil")
			}
			if got := err.Error(); !strings.Contains(got, tc.wantErr) {
				t.Errorf("error: got %q, want substring %q", got, tc.wantErr)
			}
			if len(npc.enqueueCalls) != 0 {
				t.Errorf("enqueueCalls: got %d, want 0 (must not enqueue on rejection)",
					len(npc.enqueueCalls))
			}
		})
	}
}
```

(Note: the function rename `TestHandleNpcQueueInvalidQueueIDErrors → TestHandleNpcQueueOutOfRangeErrors` is intentional and reflects the new semantics.)

- [ ] **Step 2: Run the rewritten test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleNpcQueueOutOfRangeErrors -count=1 -v 2>&1 | tail -30
```

Expected: FAIL for BOTH subtests. The "negative" subtest fails because queueID=-1 is currently NOT rejected by the inline `if queueID < 1 || queueID > 20` (it IS rejected, so this should actually pass…) — wait. Re-check: inline currently rejects `queueID < 1` which includes -1, so -1 IS rejected, but the error string `"NPC_QUEUE: invalid queueId -1 (want 1..20)"` differs from the new expected `"NPC_QUEUE: queue id out of range (-1)"`. So the "negative" subtest fails on **error-string mismatch**.

The "twenty" subtest fails because queueID=20 is currently ACCEPTED by the inline `if queueID < 1 || queueID > 20` (20 is not > 20), so `Execute` returns nil — the test expects an error.

Both subtests fail in different ways. Confirm both fail before proceeding.

- [ ] **Step 3: Replace the inline check at `handleNpcQueue`**

Open `pkg/script/handlers_npc.go`. Find `handleNpcQueue` (~L482-498). The relevant lines to replace are L491-494:

```go
	queueID := s.PopInt()
	if queueID < 1 || queueID > 20 {
		return fmt.Errorf("NPC_QUEUE: invalid queueId %d (want 1..20)", queueID)
	}
```

Replace with:

```go
	queueID := s.PopInt()
	if err := checkQueue(queueID, "NPC_QUEUE"); err != nil {
		return err
	}
```

Leave the `lastIntArg := s.PopInt()` line above and the `trigger := TriggerAiQueue1 + ServerTriggerType(queueID-1)` arithmetic below UNCHANGED.

- [ ] **Step 4: Refresh the `handleNpcQueue` doc comment**

In the same file, find the comment block above `handleNpcQueue` (L475-481). The text to replace (from the third line onward):

```go
// (bottom). queueId ∈ [1, 20] maps to TriggerAiQueue1..20 via
// arithmetic: trigger = TriggerAiQueue1 + queueId - 1. Mirrors TS
// NpcOps.ts:144-150, including the NumberNotNull check on delay
// (closed in NAI-20). The Go-side queueId 1..20 range check
// corresponds to TS QueueValid; the arg pop is unwrapped per TS.
```

Replace with:

```go
// (bottom). queueId ∈ [0, 19] (TS QueueValid) maps to TriggerAiQueue1..20
// via arithmetic: trigger = TriggerAiQueue1 + queueId - 1. Mirrors TS
// NpcOps.ts:144-150, including the NumberNotNull check on delay
// (closed in NAI-20). Validated via checkQueue (TS-literal [0, 19]
// inclusive); the arg pop is unwrapped per TS.
```

Leave the first two doc lines (`// handleNpcQueue (NPC_QUEUE, opcode 2530) enqueues an ai_queueN` / `// dispatch on the active NPC. Pop order: delay (top), arg, queueId`) UNCHANGED.

- [ ] **Step 5: Run rewritten test to verify it now passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleNpcQueueOutOfRangeErrors -count=1 -v 2>&1 | tail -15
```

Expected: PASS for both `negative` and `twenty` subtests.

- [ ] **Step 6: Add the NPC_QUEUE boundary-pin tests**

Insert the following two new test functions into `pkg/script/handlers_npc_test.go` immediately AFTER the rewritten `TestHandleNpcQueueOutOfRangeErrors`:

```go
// TestHandleNpcQueueAcceptsZeroEdge pins the TS-faithful fencepost:
// queueID=0 now passes checkQueue and produces TriggerAiQueue1 - 1 (a
// garbage trigger one before AI_QUEUE1). Inherited from upstream TS;
// see NAI-QUEUE-D-TS-FENCEPOST-INHERITED. No LostCityRS/Content script
// exercises this path. This test exists so a future contributor doesn't
// "fix" the validator back to [1, 20] without recognizing the deliberate
// divergence call.
func TestHandleNpcQueueAcceptsZeroEdge(t *testing.T) {
	npc := &mockNpc{}
	sf := &ScriptFile{
		Name: "npc_queue_zero_edge",
		Opcodes: []Opcode{
			OpPushConstantInt, // queueID = 0
			OpPushConstantInt, // arg = 0
			OpPushConstantInt, // delay = 0
			OpNpcQueue,
			OpReturn,
		},
		IntOperands: []int32{0, 0, 0, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: unexpected error %v (queueID=0 should pass TS-literal [0, 19])", err)
	}
	if len(npc.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(npc.enqueueCalls))
	}
	// queueID=0 → trigger = TriggerAiQueue1 + (0-1) = TriggerAiQueue1 - 1
	wantTrigger := TriggerAiQueue1 - 1
	if got := npc.enqueueCalls[0].trigger; got != wantTrigger {
		t.Errorf("trigger: got %d, want %d (TriggerAiQueue1 - 1, the inherited TS fencepost)",
			got, wantTrigger)
	}
}

// TestHandleNpcQueueRejectsTwenty pins the upper-bound shift: queueID=20
// was accepted under goscape's pre-port [1, 20] range, now rejected
// under TS-literal [0, 19].
func TestHandleNpcQueueRejectsTwenty(t *testing.T) {
	npc := &mockNpc{}
	sf := &ScriptFile{
		Name: "npc_queue_twenty",
		Opcodes: []Opcode{
			OpPushConstantInt, // queueID = 20
			OpPushConstantInt, // arg = 0
			OpPushConstantInt, // delay = 0
			OpNpcQueue,
			OpReturn,
		},
		IntOperands: []int32{20, 0, 0, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for queueID=20 (now out of [0, 19]), got nil")
	}
	if !strings.Contains(err.Error(), "NPC_QUEUE: queue id out of range (20)") {
		t.Errorf("error: got %q, want substring %q", err.Error(),
			"NPC_QUEUE: queue id out of range (20)")
	}
	if len(npc.enqueueCalls) != 0 {
		t.Errorf("enqueueCalls: got %d, want 0 (must not enqueue on rejection)",
			len(npc.enqueueCalls))
	}
}
```

**Inspect the `mockNpc.enqueueCalls` shape first.** Run:
```bash
grep -nE "enqueueCalls|EnqueueScriptForTrigger" pkg/script/handlers_npc_test.go | head -10
```

If the recorded struct field is NOT named `trigger` (e.g., the mock might record `triggerType` or unpack arguments differently), adapt the field access in `TestHandleNpcQueueAcceptsZeroEdge` to match the actual field. The semantic check `got != TriggerAiQueue1 - 1` must remain.

- [ ] **Step 7: Run new boundary tests to verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHandleNpcQueueAcceptsZeroEdge|TestHandleNpcQueueRejectsTwenty' -count=1 -v 2>&1 | tail -15
```

Expected: both PASS.

### 3B. NPC_WALKTRIGGER site

- [ ] **Step 8: Rewrite the two existing walktrigger boundary tests**

Find `TestNpcWalkTrigger_QueueIDBelowOne_Errors` (`handlers_npc_test.go:3109-3126`). Replace with:

```go
func TestNpcWalkTrigger_QueueIDBelowZero_Errors(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Push order: queueID (first → bottom), arg (second → top).
	s.PushInt(-1) // queueID = -1 → invalid under [0, 19]
	s.PushInt(5)  // arg
	if err := handleNpcWalkTrigger(s); err == nil {
		t.Fatalf("expected error for queueID=-1")
	}
	if len(npc.walkTriggerCalls) != 0 {
		t.Errorf("walkTriggerCalls: got %d writes, want 0 on validation failure",
			len(npc.walkTriggerCalls))
	}
}
```

Find `TestNpcWalkTrigger_QueueIDAboveTwenty_Errors` (`handlers_npc_test.go:3128-3141`). Replace with:

```go
func TestNpcWalkTrigger_QueueIDAboveNineteen_Errors(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Push order: queueID (first → bottom), arg (second → top).
	s.PushInt(20) // queueID = 20 → invalid under [0, 19]
	s.PushInt(5)  // arg
	if err := handleNpcWalkTrigger(s); err == nil {
		t.Fatalf("expected error for queueID=20")
	}
}
```

- [ ] **Step 9: Run rewritten walktrigger tests to verify they fail**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestNpcWalkTrigger_QueueIDBelowZero_Errors|TestNpcWalkTrigger_QueueIDAboveNineteen_Errors' -count=1 -v 2>&1 | tail -20
```

Expected: `TestNpcWalkTrigger_QueueIDBelowZero_Errors` PASSes (queueID=-1 was already rejected by the inline `< 1` check; it remains rejected). `TestNpcWalkTrigger_QueueIDAboveNineteen_Errors` FAILs because queueID=20 is currently accepted (inline check is `> 20`).

If `TestNpcWalkTrigger_QueueIDBelowZero_Errors` unexpectedly fails too, investigate — but the rename + value change should be safe.

- [ ] **Step 10: Replace the inline check at `handleNpcWalkTrigger`**

Open `pkg/script/handlers_npc.go`. Find `handleNpcWalkTrigger` (~L557-569). The relevant lines to replace are L562-565:

```go
	arg := s.PopInt()
	queueID := s.PopInt()
	if queueID < 1 || queueID > 20 {
		return fmt.Errorf("NPC_WALKTRIGGER: invalid queueId %d (want 1..20)", queueID)
	}
```

Replace with:

```go
	arg := s.PopInt()
	queueID := s.PopInt()
	if err := checkQueue(queueID, "NPC_WALKTRIGGER"); err != nil {
		return err
	}
```

Leave the `s.ActiveNpc.SetWalkTrigger(queueID - 1)` and `s.ActiveNpc.SetWalkTriggerArg(arg)` lines below UNCHANGED.

- [ ] **Step 11: Refresh the `handleNpcWalkTrigger` doc comment**

In the same file, find the comment block above `handleNpcWalkTrigger` (L550-556). The text to replace:

```go
// (bottom). queueID ∈ [1, 20] mirrors TS QueueValid range, transformed
// to [0, 19] via queueID-1 to match TS NpcOps.ts:488 storage. Mirrors
// TS NpcOps.ts:483-490. The walktrigger consumer fires from
// (*Npc).updateMovement (modules/world/npc_interaction.go, NAI-51 T2.1).
```

Replace with:

```go
// (bottom). queueId ∈ [0, 19] (TS QueueValid); then queueId-1 mirrors
// TS NpcOps.ts:488 storage (walktrigger = queueId - 1). Validated via
// checkQueue. Mirrors TS NpcOps.ts:483-490. The walktrigger consumer
// fires from (*Npc).updateMovement (modules/world/npc_interaction.go,
// NAI-51 T2.1).
```

Leave the first three doc lines (`// handleNpcWalkTrigger (NPC_WALKTRIGGER, opcode 2545) sets a deferred` / `// AI-queue trigger and arg on the active NPC; the trigger fires when` / `// the NPC completes a walk step. Pop order: arg (top), queueID`) UNCHANGED.

- [ ] **Step 12: Run rewritten walktrigger tests to verify they now pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestNpcWalkTrigger_QueueIDBelowZero_Errors|TestNpcWalkTrigger_QueueIDAboveNineteen_Errors' -count=1 -v 2>&1 | tail -15
```

Expected: both PASS.

- [ ] **Step 13: Add the NPC_WALKTRIGGER boundary-pin tests**

Insert the following two new test functions into `pkg/script/handlers_npc_test.go` immediately AFTER `TestNpcWalkTrigger_QueueIDAboveNineteen_Errors`:

```go
// TestHandleNpcWalkTriggerAcceptsZeroEdge pins the TS-faithful fencepost:
// queueID=0 now passes checkQueue and stores walktrigger = -1
// (queueID - 1). See NAI-QUEUE-D-TS-FENCEPOST-INHERITED.
func TestHandleNpcWalkTriggerAcceptsZeroEdge(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0) // queueID = 0 (was rejected, now accepted under TS-literal)
	s.PushInt(5) // arg
	if err := handleNpcWalkTrigger(s); err != nil {
		t.Fatalf("unexpected error: %v (queueID=0 should pass TS-literal [0, 19])", err)
	}
	if want := []int{-1}; !equalIntSlice(npc.walkTriggerCalls, want) {
		t.Errorf("walkTriggerCalls: got %v, want %v (queueID-1 = -1, inherited TS fencepost)",
			npc.walkTriggerCalls, want)
	}
}

// TestHandleNpcWalkTriggerRejectsTwenty pins the upper-bound shift:
// queueID=20 was accepted under [1, 20], now rejected under [0, 19].
func TestHandleNpcWalkTriggerRejectsTwenty(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(20) // queueID = 20 (was accepted, now rejected)
	s.PushInt(5)  // arg
	err := handleNpcWalkTrigger(s)
	if err == nil {
		t.Fatalf("expected error for queueID=20")
	}
	if !strings.Contains(err.Error(), "NPC_WALKTRIGGER: queue id out of range (20)") {
		t.Errorf("error: got %q, want substring %q", err.Error(),
			"NPC_WALKTRIGGER: queue id out of range (20)")
	}
	if len(npc.walkTriggerCalls) != 0 {
		t.Errorf("walkTriggerCalls: got %d writes, want 0 on validation failure",
			len(npc.walkTriggerCalls))
	}
}
```

- [ ] **Step 14: Run new walktrigger boundary tests to verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHandleNpcWalkTriggerAcceptsZeroEdge|TestHandleNpcWalkTriggerRejectsTwenty' -count=1 -v 2>&1 | tail -15
```

Expected: both PASS.

### 3C. Test-side doc-comment sweep (spec §5.5)

- [ ] **Step 15: Refresh stale `[1, 20]` / `1..20` references**

Find `TestHandleNpcQueueEnqueues` (~`handlers_npc_test.go:917`). The doc comment currently reads:

```go
// TestHandleNpcQueueEnqueues — NPC_QUEUE pops (delay, arg, queueID)
// in that order (top of stack = delay) and maps queueID (1-20) to
// TriggerAiQueue1 + queueID - 1.
```

Replace with:

```go
// TestHandleNpcQueueEnqueues — NPC_QUEUE pops (delay, arg, queueID)
// in that order (top of stack = delay) and maps queueID (0-19, TS
// QueueValid) to TriggerAiQueue1 + queueID - 1.
```

Find `TestHandleNpcQueueNullDelayRejected` (~`handlers_npc_test.go:1630-1632`). The doc comment currently reads:

```go
// TestHandleNpcQueueNullDelayRejected pins NAI-20 Task 4: NPC_QUEUE
// rejects delay=-1 via checkNotNull. The queueId 1..20 range check is
// orthogonal (covered by TestHandleNpcQueueInvalidQueueIDErrors).
```

Replace with:

```go
// TestHandleNpcQueueNullDelayRejected pins NAI-20 Task 4: NPC_QUEUE
// rejects delay=-1 via checkNotNull. The queueId [0, 19] range check is
// orthogonal (covered by TestHandleNpcQueueOutOfRangeErrors).
```

(Note the test-name reference also updates: `TestHandleNpcQueueInvalidQueueIDErrors → TestHandleNpcQueueOutOfRangeErrors`.)

- [ ] **Step 16: Audit-grep sweep for any remaining stale references**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache rg -n "want 1\.\.20|1\.\.20|1-20" pkg/script/ 2>&1 | head -20
```

Expected: ZERO hits. If anything turns up, inspect each match and refresh per the same `[1, 20] → [0, 19]` / `1..20 → 0..19` pattern. The walktrigger test block header at L3093 may need updating if it cites "1..20" anywhere — re-read the lines around L3093-3094 to confirm.

```bash
sed -n '3091,3094p' pkg/script/handlers_npc_test.go
```

If the block header references "[1,20]" or "1..20", update to "[0, 19]" / "0..19".

### 3D. Full gate + commit

- [ ] **Step 17: Run all touched + related tests**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/script/... 2>&1 | tail -10
```

Expected: `ok  github.com/zsrv/goscape/pkg/script` clean. Specifically:
- New: `TestCheckQueue_Range`, `TestHandleNpcQueueAcceptsZeroEdge`, `TestHandleNpcQueueRejectsTwenty`, `TestHandleNpcWalkTriggerAcceptsZeroEdge`, `TestHandleNpcWalkTriggerRejectsTwenty` → PASS
- Rewritten: `TestHandleNpcQueueOutOfRangeErrors`, `TestNpcWalkTrigger_QueueIDBelowZero_Errors`, `TestNpcWalkTrigger_QueueIDAboveNineteen_Errors` → PASS
- Unchanged-but-validated: `TestHandleNpcQueueEnqueues`, `TestHandleNpcQueueNullDelayRejected`, `TestNpcWalkTrigger_PopOrderAndTransform`, `TestNpcWalkTrigger_BoundaryQueueIDs`, `TestNpcWalkTrigger_NoActiveNpc_Errors` → PASS

If `TestNpcWalkTrigger_BoundaryQueueIDs` exists and exercises queueID=20 (the upper boundary under the old range), it will now FAIL. Read the test (`handlers_npc_test.go:3165+`) and adjust: if it tests `queueID=1` and `queueID=20`, change `queueID=20` to `queueID=19` (the new upper boundary). The semantic check (boundary acceptance) is preserved; only the value shifts.

- [ ] **Step 18: Commit**

```bash
git status
git add pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go
git diff --cached --stat
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): wire checkQueue at NPC_QUEUE + NPC_WALKTRIGGER (T3)

Range shift from goscape [1, 20] to TS-literal [0, 19] per
NAI-QUEUE-D-TS-FENCEPOST-INHERITED. Trigger arithmetic
(TriggerAiQueue1 + queueID - 1; SetWalkTrigger(queueID - 1)) unchanged
at both sites. Inherits the upstream-TS fencepost (queueID=0 produces
TriggerAiQueue1 - 1 garbage trigger); not exercised by any
LostCityRS/Content script.

Existing invalid-id tests rewritten for new boundaries (rename:
TestHandleNpcQueueInvalidQueueIDErrors → TestHandleNpcQueueOutOfRangeErrors;
TestNpcWalkTrigger_QueueIDBelowOne_Errors → ...BelowZero_Errors;
TestNpcWalkTrigger_QueueIDAboveTwenty_Errors → ...AboveNineteen_Errors).
Four new boundary pins: TestHandleNpcQueue{AcceptsZeroEdge,RejectsTwenty},
TestHandleNpcWalkTrigger{AcceptsZeroEdge,RejectsTwenty}.

Doc-comment refresh: handleNpcQueue, handleNpcWalkTrigger,
TestHandleNpcQueueEnqueues, TestHandleNpcQueueNullDelayRejected (+
NAI-37 walktrigger block header if applicable).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

Expected: `2 files changed`. ~120 insertions, ~50 deletions (rough).

---

## Task 4: Wire `checkSkinColour` at SETSKINCOLOUR + doc touch

Smaller than T3 — only one call site, no range shift (TS and goscape already agree on `[0, 7]`), no behavior change. Existing `TestHandleSetSkinColour_RejectsOutOfRange` uses `strings.Contains(err, "SETSKINCOLOUR")` so it survives the new error message unmodified.

**Files:**
- Modify: `pkg/script/handlers_player.go` — replace inline check at L1661-1664 (`handleSetSkinColour`); append validator citation to doc comment at L1648-1656

- [ ] **Step 1: Replace the inline check at `handleSetSkinColour`**

Open `pkg/script/handlers_player.go`. Find `handleSetSkinColour` (~L1657-1667). The relevant lines to replace are L1661-1664:

```go
	skin := s.PopInt()
	if skin < 0 || skin > 7 {
		return fmt.Errorf("SETSKINCOLOUR: invalid skin colour %d (range 0..7)", skin)
	}
```

Replace with:

```go
	skin := s.PopInt()
	if err := checkSkinColour(skin, "SETSKINCOLOUR"); err != nil {
		return err
	}
```

Leave the `requireActivePlayer` guard above and the `s.Self.SetColorPart(4, skin)` call below UNCHANGED.

- [ ] **Step 2: Append validator-citation to the doc comment**

In the same file, find the comment block above `handleSetSkinColour` (L1648-1656). The current text ends with:

```go
// The active-player guard is goscape defensive (TS skips this check;
// see defensive_gate_doc_comment_label).
```

Insert ONE additional line immediately before that "active-player guard" sentence:

```go
// Validated via checkSkinColour (TS SkinColourValid, inclusive [0, 7]).
// The active-player guard is goscape defensive (TS skips this check;
// see defensive_gate_doc_comment_label).
```

(So the final doc-comment block has the new line as the second-to-last sentence.)

- [ ] **Step 3: Verify existing `TestHandleSetSkinColour_*` tests stay green**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHandleSetSkinColour|TestCheckSkinColour' -count=1 -v 2>&1 | tail -30
```

Expected: ALL pass — `TestHandleSetSkinColour_WritesColors4`, `TestHandleSetSkinColour_RejectsOutOfRange` (with new error message — `strings.Contains` assertion still satisfied), `TestHandleSetSkinColour_RequiresActivePlayer`, `TestCheckSkinColour_Range`.

If `TestHandleSetSkinColour_RejectsOutOfRange` unexpectedly fails, inspect its error assertion: if it pins the OLD literal `"invalid skin colour"` or `"range 0..7"` instead of `strings.Contains(err.Error(), "SETSKINCOLOUR")`, update those assertions to a `strings.Contains(err.Error(), "SETSKINCOLOUR")` or `"out of range"` pattern that matches the new wording.

- [ ] **Step 4: Audit-grep sweep**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache rg -n "range 0\.\.7" pkg/script/ 2>&1 | head -10
```

Expected: ZERO hits (the only known reference was the production-side error string, now removed). If any test still references "range 0..7" in doc-comment text, refresh that too.

- [ ] **Step 5: Commit**

```bash
git status
git add pkg/script/handlers_player.go
git diff --cached --stat
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): wire checkSkinColour at SETSKINCOLOUR (T4)

Replace inline [0, 7] range check at handleSetSkinColour with
checkSkinColour. Direct TS mirror (TS and goscape already agreed on
[0, 7]); zero behavior change. Doc-comment cites the validator.

TestHandleSetSkinColour_RejectsOutOfRange green unmodified — assertion
uses strings.Contains(err, "SETSKINCOLOUR") which survives new error
message wording.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

Expected: `1 file changed`, ~5 insertions, ~3 deletions.

---

## Task 5: Carry-forward grep sweep + final gate

Per carry-forward `[[hit-type-validator-slice-close]]` finding #2: predecessor slice's reviewer caught stale `"stays raw"` test-side doc comments that the spec hadn't named. This task does the equivalent sweep for THIS slice's keywords across the full `pkg/script/` tree.

- [ ] **Step 1: Final stale-comment audit grep**

Run each of:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache rg -n "want 1\.\.20|1\.\.20|1-20|range 0\.\.7" pkg/script/
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache rg -n "TestHandleNpcQueueInvalidQueueIDErrors|TestNpcWalkTrigger_QueueIDBelowOne|TestNpcWalkTrigger_QueueIDAboveTwenty" pkg/script/
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache rg -n "invalid queueId.*want 1" pkg/script/
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache rg -n "stays raw" pkg/script/
```

Expected: ZERO hits across all four greps. If any turn up:
- `want 1..20` / `1..20` / `1-20` / `range 0..7` → update doc/error text to match the new ranges
- Old test names → update cross-references (test docs that say "covered by TestHandleNpcQueueInvalidQueueIDErrors" must become "...OutOfRangeErrors")
- `stays raw` → carry-forward leftover; inspect and refresh per the same `[[hit-type-validator-slice-close]]` finding pattern (not specific to this slice but always worth a sweep)

If any updates are needed, make them, then RE-RUN all four greps to confirm zero hits.

- [ ] **Step 2: Full -race gate across all packages**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... 2>&1 | tail -20
```

Expected: every package `ok`. No FAIL anywhere. This typically takes 2-3 minutes on a warm cache.

If a non-`pkg/script` package fails: the change does not touch any other production code, so a failure indicates either (a) a flaky test unrelated to this slice, or (b) the test imports `pkg/script` and depends on its public API. The validators are unexported (`checkQueue`, `checkSkinColour`) and the wired handlers preserve their public contract, so (b) is unlikely. Investigate and report rather than papering over.

- [ ] **Step 3: Smoke-pack**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/packall/... 2>&1 | tail -20
```

Expected: `12 OK / 0 ERR / 0 SKIP` in the output (or whatever the existing pass count is — the count must MATCH the baseline). If the smoke-pack normally runs via a different command in this project, use:

```bash
grep -rn "smoke-pack\|smoke_pack" Makefile pkg/packall/ 2>&1 | head -5
```

…and follow the invocation it shows. The HitType close memory reports `12 OK / 0 ERR / 0 SKIP (8.48s)`, so that's the expected shape.

- [ ] **Step 4: Commit (only if Step 1 produced edits)**

If Step 1 found and fixed stale references, commit them:

```bash
git status
git add pkg/script/
git diff --cached --stat
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(script): carry-forward sweep for Queue/SkinColour validator port (T5)

Audit-grep sweep per [[hit-type-validator-slice-close]] finding #2.
Refreshes stale "1..20" / "range 0..7" / old-test-name references in
test doc comments that the per-task spec scoping missed.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

If Step 1 produced no edits, skip this step entirely — no empty commit. Note that in the close commit summary at the chore(close) stage.

---

## Close commit

After all impl tasks SHIP per their per-task review, the dispatcher runs the close commit. This is NOT a sonnet implementer task — it's a controller-side wrap.

- [ ] **Step 1: Final gate re-run (sanity check before close)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... 2>&1 | tail -10
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./pkg/packall/... 2>&1 | tail -20
```

Expected: both green, smoke-pack `12 OK / 0 ERR / 0 SKIP`.

- [ ] **Step 2: Verify git log**

```bash
git log --oneline -7
```

Expected sequence (newest first): close(chore) → T5 (if produced an edit) → T4 → T3 → T2 → T1 → `3750ac6e docs(spec): ...` → `7965eda1 chore(close): HitType ...`.

- [ ] **Step 3: Write close commit**

This is an empty-diff `chore(close)` commit summarizing the slice. Use `--allow-empty`:

```bash
git status
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): Queue + SkinColour validator port

Two new ScriptInputRangeValidator family entries ported from TS:
- checkQueue [0, 19] @ NPC_QUEUE + NPC_WALKTRIGGER (TS-literal range
  shift from goscape [1, 20]; opens NAI-QUEUE-D-TS-FENCEPOST-INHERITED
  pinning the inherited upstream-TS queueId=0 garbage-trigger fencepost
  unexercised by LostCityRS/Content)
- checkSkinColour [0, 7] @ SETSKINCOLOUR (direct mirror, no behavior
  change)

3 existing tests rewritten (boundary value shifts + rename);
6 new tests added (2 validator unit + 4 handler boundary pins);
5 doc comments refreshed (production + test);
1 new deviation pin opened (NAI-QUEUE-D-TS-FENCEPOST-INHERITED).

Closes the "simple enum-range validator family" port from TS
ScriptValidators.ts — predecessor [[hit-type-validator-slice-close]]
§9 retired (extraction was framed as cosmetic; Queue actually carried
a fencepost-correction opportunity, now consciously inherited).

Spec: docs/superpowers/specs/2026-05-20-queue-skincolour-validator-design.md
Plan: docs/superpowers/plans/2026-05-20-queue-skincolour-validator-port.md

-race ./... clean. Smoke-pack 12 OK / 0 ERR / 0 SKIP.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

Expected: empty commit lands successfully.

- [ ] **Step 4: Write the close memory**

Update `MEMORY.md` index + create `hit_type_validator_slice_close.md`-style topic file under `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/` capturing: commit range, retired/opened pins, gate posture, key non-obvious findings, carry-forward grep keywords (the `1..20` / `range 0..7` set plus the predecessor's `stays raw`). Reference predecessor `[[hit-type-validator-slice-close]]` via wikilink.

---

## Acceptance criteria (mirrors spec §10)

1. ✅ `checkQueue` added to `pkg/script/handlers_npc.go` (T1). `checkSkinColour` added to `pkg/script/handlers_player.go` (T2). Both bare-number, no objtype dep.
2. ✅ Three handler call sites wire their validator: NPC_QUEUE (T3 Step 3), NPC_WALKTRIGGER (T3 Step 10), SETSKINCOLOUR (T4 Step 1). Arithmetic preserved at all three.
3. ✅ Doc comments refreshed: handleNpcQueue (T3 Step 4), handleNpcWalkTrigger (T3 Step 11), handleSetSkinColour (T4 Step 2).
4. ✅ Stale `1..20` / `range 0..7` sweep — both production (T3 Steps 3/10, T4 Step 1) and test side (T3 Step 15, T5 Step 1). Final grep returns empty.
5. ✅ Existing tests rewritten: `TestHandleNpcQueueInvalidQueueIDErrors → ...OutOfRangeErrors` (T3 Step 1), `TestNpcWalkTrigger_QueueIDBelowOne_Errors → ...BelowZero_Errors` (T3 Step 8), `TestNpcWalkTrigger_QueueIDAboveTwenty_Errors → ...AboveNineteen_Errors` (T3 Step 8). `TestHandleSetSkinColour_RejectsOutOfRange` verified green unmodified (T4 Step 3).
6. ✅ Six new tests added: `TestCheckQueue_Range` (T1), `TestCheckSkinColour_Range` (T2), `TestHandleNpcQueueAcceptsZeroEdge` + `TestHandleNpcQueueRejectsTwenty` (T3 Step 6), `TestHandleNpcWalkTriggerAcceptsZeroEdge` + `TestHandleNpcWalkTriggerRejectsTwenty` (T3 Step 13).
7. ✅ One new deviation pin `NAI-QUEUE-D-TS-FENCEPOST-INHERITED` opened in `checkQueue` doc comment (T1 Step 4).
8. ✅ `-race ./...` clean (T5 Step 2).
9. ✅ Smoke-pack `12 OK / 0 ERR / 0 SKIP` (T5 Step 3).
