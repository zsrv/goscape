# NAI-111 Stage 2 — Minimal: delete CloseModal protect over-clear

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore `script.pointers&PAP` semantics on resumed protected scripts by deleting the 3-line over-clear in `(*Player).CloseModal` and updating the surrounding NAI-52 doc-comments to a narrowed convergence.

**Architecture:** Single TDD bundle. Production diff is ~3 LOC delete + 3 doc-comment revisions. Test diff retires 3 broken-behavior pins, updates 1 outdated workaround note, and adds 2 regression tests. Cadence per `compressed_cadence` (≤~15 LOC threshold satisfied) — combined spec+plan in this doc, single Sonnet implementer + Sonnet code-reviewer per `superpowers_code_reviewer_model`.

**Tech Stack:** Go 1.26+. Standard library only. Tests use existing `newTestPlayer(t)` helper from `modules/world/player_test.go:17`.

---

## §A Background (compressed spec)

**Symptom:** Tutorial Island terminal teleport (`[label,tutorial_complete]` → `tut_close; if_close; p_telejump`) aborts with `"P_TELEJUMP: script not protected"` after the player picks "Yes, go to mainland" at the Magic Instructor sub-quest.

**Root cause** (Stage 1 §G3 confirmed):
`modules/world/player_script.go:728-730` strips `PtrProtectedActivePlayer` from `p.activeScript.Pointers` whenever `(*Player).CloseModal` is called and the player is not delayed. During a **resumed** script, `p.activeScript` is the same `*ScriptState` struct as the in-flight `s` passed to handlers. Stripping the pointer mid-flight breaks every subsequent `requireProtectedActivePlayer`-gated opcode in the same script (35 broken handler call sites enumerated in Stage 1 G3, all sharing this single root).

**TS reference** (`Engine-TS/src/engine/entity/Player.ts`):
- `closeModal` (L741-794) clears `this.protect = false` (L746) but contains **zero** calls to `pointerRemove` / `pointerAdd` / `pointerGet` on `ProtectedActivePlayer`. The `script.pointers&PAP` bitmask is **never touched by closeModal** in TS.
- The TS `this.protect` Player-level bool is read only by `canAccess` (L810), `processWalktrigger` (L1062), and the `runScript` reentry early-return (L2095) — all **external** consumers.

**Fix** (Stage 1 §scope recommendation):
Delete the 3 LOC at `player_script.go:728-730`. The narrowed NAI-52 convergence holds because:
- `script.Pointers&PtrProtectedActivePlayer` maps cleanly to TS `script.pointers&PAP` (in-flight script-state flag).
- Goscape's external "is a protected script suspended?" question is answered by `protectedScriptActive()`, which reads the suspended `*ScriptState`'s preserved Pointers. Goscape's `StoreActiveScript` (`player_script.go:138-140`) preserves `Pointers` across suspensions, so the flag stays set across the suspend boundary — net observable behavior identical to TS `this.protect = protect` restore at `Player.ts:2141`.

**DEVIATION-NAI-111-D1** (to be added in code as doc-comment, per `defensive_gate_doc_comment_label`):
NAI-52 convergence narrowed. `script.Pointers&PtrProtectedActivePlayer` no longer maps to TS `this.protect` (Player-level bool). It maps **only** to TS `script.pointers&PAP` (script-state bitmask). The forward-looking "is a protected script suspended" external question is answered by `protectedScriptActive`, which preserves correctness because the pointer persists on `*ScriptState` across suspensions (TS strips and re-adds at every runScript boundary; goscape preserves; observable behavior identical for `canAccess` + `processWalktrigger` gates). Retire condition: port Scenario B if a future TS change to `runScript` reentry semantics requires per-call lifecycle modelling.

---

## §B Pre-flight notes (controller-verified against HEAD)

These line refs were resolved against HEAD before plan-author dispatch. The implementer should still re-verify before each Edit if any have shifted (none expected, but `controller_preflight` hygiene applies).

- **Production target (delete):** `modules/world/player_script.go:728-730`
  ```go
  if !p.delayed && p.activeScript != nil {
      p.activeScript.Pointers &^= script.PtrProtectedActivePlayer
  }
  ```
- **Doc-comments to revise:**
  - `modules/world/player_script.go:712-723` (`CloseModal` doc — line 715-716 currently asserts the convergence + clear).
  - `modules/world/player_script.go:268-282` (`CanAccess` doc — lines 276-282 assert the broad NAI-52 equivalence).
  - `modules/world/player_script.go:296-301` (`protectedScriptActive` doc — line 300 cross-references CanAccess).
- **Test cohort:**
  - `TestCloseModalClearsActiveScriptProtectWhenNotDelayed` — `modal_close_test.go:100-124` (incl. doc-comment at 100-104). **Retire.** Pins broken behavior.
  - `TestCloseModalPreservesActiveScriptProtectWhenDelayed` — `modal_close_test.go:126-145`. **Retire.** Pins the `delayed`-branch of L728-730; the entire block is gone post-fix.
  - `TestCloseModalNoneEarlyReturnStillRunsClearWeakQueueAndProtect` — `modal_close_test.go:213-238`. **Retire and replace** with a narrower test that pins only the weak-queue half of the early-return ordering. The protect-clear half no longer exists.
  - `TestCloseModalPreservesActiveScriptOnSuspended` — `modal_close_test.go:277-297`. **Update only:** drop the `p.delayed = true` workaround at line 282 (and its trailing comment) — the workaround was needed solely because L728-730 would have stripped PAP otherwise. Test body remains valid.
- **StoreActiveScript verification:** `modules/world/player_script.go:138-140` is `p.activeScript = state`. No PAP clear. Confirms Stage 1's drift-tolerant verdict on `protectedScriptActive`.
- **Audit minor mislabel:** Stage 1 row "`pkg/script/handlers_inv.go:460` (INV_DROPSLOT)" actually references `INV_DELSLOT` (`handleInvDelSlot` is at `handlers_inv.go:448`; `handleInvDropSlot` is at `:771`). The mislabel does not affect this plan — no `handlers_inv.go` line numbers appear in the production diff.

---

## §C Task: Delete the over-clear and pin the new invariants

**Files:**
- Modify: `modules/world/player_script.go:712-723` (CloseModal doc-comment).
- Modify: `modules/world/player_script.go:728-730` (delete the 3-line block).
- Modify: `modules/world/player_script.go:268-282` (CanAccess doc-comment).
- Modify: `modules/world/player_script.go:296-301` (protectedScriptActive doc-comment).
- Modify: `modules/world/modal_close_test.go` (retire 3 tests, narrow 1, update 1 setup, add 2 regressions).

### Step 1: Update `modal_close_test.go` — retire broken-behavior pins, add regression pins (failing red phase)

Make these test-file edits in a single pass. After this step, `go test ./modules/world/...` should fail because (a) two of the new regression tests pin behavior the unfixed production code violates, and (b) the retired tests are gone but production still over-clears. The failures are intentional — the implementation step removes the over-clear.

- [ ] **Step 1a: Delete `TestCloseModalClearsActiveScriptProtectWhenNotDelayed`**

Delete `modal_close_test.go:100-124` (the function plus its 4-line doc-comment, contiguous block).

- [ ] **Step 1b: Delete `TestCloseModalPreservesActiveScriptProtectWhenDelayed`**

Delete `modal_close_test.go:126-145` (the function plus its 3-line doc-comment, contiguous block).

- [ ] **Step 1c: Replace `TestCloseModalNoneEarlyReturnStillRunsClearWeakQueueAndProtect` with a narrower weak-queue-only variant**

The test name and assertion about protect-clear are obsolete. Rename + retain the weak-queue-on-NONE-early-return half:

```go
// TestCloseModalNoneEarlyReturnStillRunsClearWeakQueue pins the
// weak-queue clearing runs BEFORE the modalState == NONE early-return
// (TS Player.ts:742-744 — clearWeakQueue runs before the modalState
// check).
func TestCloseModalNoneEarlyReturnStillRunsClearWeakQueue(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueWeak},
	}
	p.modalState = modalStateNone

	p.CloseModal(true)

	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 (weak should be cleared even on NONE early-return)", len(p.queue))
	}
}
```

Implementation: locate the existing `TestCloseModalNoneEarlyReturnStillRunsClearWeakQueueAndProtect` function (currently at `modal_close_test.go:213-238`, including its 4-line doc-comment at 213-216) and replace the whole block with the snippet above.

- [ ] **Step 1d: Update `TestCloseModalPreservesActiveScriptOnSuspended` to drop the workaround**

The test currently uses `p.delayed = true` at line 282 with the trailing comment `// delayed so the protect-clear block doesn't fire`. After the fix, the workaround is unnecessary — the test should run with `delayed=false` (the default) so it pins the post-fix invariant directly: a Suspended-execution script with PAP set is preserved on CloseModal regardless of delayed.

Locate the existing function at `modal_close_test.go:277-297` and replace it with:

```go
// TestCloseModalPreservesActiveScriptOnSuspended pins Suspended (non-dialog)
// activeScript is preserved on CloseModal. Mirrors TS exclusion of
// non-COUNTDIALOG/PAUSEBUTTON execution states from the null branch.
func TestCloseModalPreservesActiveScriptOnSuspended(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateChat
	p.modalChat = 7
	state := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "suspended"},
		Execution: script.Suspended,
		Pointers:  script.PtrProtectedActivePlayer,
	}
	p.activeScript = state

	p.CloseModal(true)

	if p.activeScript != state {
		t.Errorf("activeScript: got %v, want preserved %v (Suspended must NOT be cleared)", p.activeScript, state)
	}
}
```

(Diff vs. current: removed `p.delayed = true` line and its trailing comment.)

- [ ] **Step 1e: Add `TestCloseModalPreservesInFlightProtectOnResumedScript`**

Append this test at the end of the protect-related test block (immediately after the updated `TestCloseModalPreservesActiveScriptOnSuspended`). It pins the post-fix invariant: `CloseModal` does NOT clear `PtrProtectedActivePlayer` from a Running activeScript when the player is not delayed. This is the regression test for the NAI-111 root cause.

```go
// TestCloseModalPreservesInFlightProtectOnResumedScript pins NAI-111:
// CloseModal must NOT strip PtrProtectedActivePlayer from p.activeScript.
// During a resumed script's in-flight execution (Execution=Running),
// p.activeScript IS the in-flight ScriptState — handlers downstream of
// tut_close/if_close (e.g. p_telejump) read s.Pointers&PAP via
// requireProtectedActivePlayer. TS Player.closeModal (Player.ts:741-794)
// touches no script pointer. Regression for NAI-53 T3's incorrect
// NAI-52-convergence over-clear.
func TestCloseModalPreservesInFlightProtectOnResumedScript(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = false
	state := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "tutorial_complete"},
		Execution: script.Running,
		Pointers:  script.PtrActivePlayer | script.PtrProtectedActivePlayer,
	}
	p.activeScript = state

	p.CloseModal(true)

	if p.activeScript != state {
		t.Fatalf("activeScript: got %v, want preserved %v (Running must NOT be cleared)", p.activeScript, state)
	}
	if p.activeScript.Pointers&script.PtrProtectedActivePlayer == 0 {
		t.Errorf("activeScript.PtrProtectedActivePlayer: got clear, want set (CloseModal must not strip mid-flight protect)")
	}
	if p.activeScript.Pointers&script.PtrActivePlayer == 0 {
		t.Errorf("activeScript.PtrActivePlayer: got clear, want set (CloseModal must not touch any script pointer)")
	}
}
```

- [ ] **Step 1f: Add `TestCloseModal_NotDelayed_ProtectedScriptActiveStaysTrue`**

Append immediately after the previous test. This test pins the externally-observable post-fix behavior via `protectedScriptActive()` — the same gate consumed by `CanAccess` and `processWalktrigger`. It is a complementary view of the same invariant from the external-consumer angle (Stage 1 G3 column "drift-tolerant" verdict for these consumers depends on this invariant holding).

```go
// TestCloseModal_NotDelayed_ProtectedScriptActiveStaysTrue pins NAI-111:
// after CloseModal on a !delayed player whose activeScript is a Running
// protected script, protectedScriptActive() must still return true.
// Externally observable form of the same invariant covered by
// TestCloseModalPreservesInFlightProtectOnResumedScript; this view
// matches the gate consumed by CanAccess and processWalktrigger.
func TestCloseModal_NotDelayed_ProtectedScriptActiveStaysTrue(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = false
	p.activeScript = &script.ScriptState{
		Script:    &script.ScriptFile{Name: "tutorial_complete"},
		Execution: script.Running,
		Pointers:  script.PtrActivePlayer | script.PtrProtectedActivePlayer,
	}

	p.CloseModal(true)

	if !p.protectedScriptActive() {
		t.Errorf("protectedScriptActive(): got false, want true (CloseModal must not strip in-flight protect)")
	}
}
```

### Step 2: Run tests to verify they fail (red)

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestCloseModal' -v`

Expected: `TestCloseModalPreservesInFlightProtectOnResumedScript` and `TestCloseModal_NotDelayed_ProtectedScriptActiveStaysTrue` both **FAIL** with assertion errors about `PtrProtectedActivePlayer` being cleared / `protectedScriptActive()` being false. Other tests in the cohort (Suspended-preserve, weak-queue-only, dispatch tests, etc.) **PASS**.

If a different test fails (e.g. compile error, unrelated regression), stop and inspect.

### Step 3: Delete the over-clear (green phase, production diff)

- [ ] **Step 3a: Edit `player_script.go:728-730`** — delete the 3-line block.

The function body should go from:

```go
func (p *Player) CloseModal(clearWeakQueue bool) {
	if clearWeakQueue {
		p.clearWeakQueue()
	}
	if !p.delayed && p.activeScript != nil {
		p.activeScript.Pointers &^= script.PtrProtectedActivePlayer
	}

	if p.modalState == modalStateNone {
```

to:

```go
func (p *Player) CloseModal(clearWeakQueue bool) {
	if clearWeakQueue {
		p.clearWeakQueue()
	}

	if p.modalState == modalStateNone {
```

(Remove the entire `if !p.delayed && p.activeScript != nil { … }` block including its trailing blank line collapse.)

- [ ] **Step 3b: Replace the CloseModal doc-comment** at `player_script.go:712-723`.

Old (current HEAD, lines 712-723):

```go
// CloseModal clears every modal slot and flags the client to emit
// IF_CLOSE on the next encodeOut pass. When clearWeakQueue is true
// (TS default), drops every QueueWeak entry from p.queue before
// processing. When the player is not delayed, clears any active
// script's Protect flag (NAI-52 convergence). Early-returns if no
// modal is currently open. Otherwise: nulls activeScript on
// COUNTDIALOG/PAUSEBUTTON suspends (closes NAI-52-F1) and dispatches
// a per-slot IF_CLOSE trigger script (Main → Chat → Side, TS order).
//
// Mirrors TS Player.closeModal (Player.ts:741-794). Body fully
// landed across NAI-53 T1-T5; per-slot clearComListeners wired in
// NAI-64 (TS Player.ts:728-739, 767, 778, 789).
```

New:

```go
// CloseModal clears every modal slot and flags the client to emit
// IF_CLOSE on the next encodeOut pass. When clearWeakQueue is true
// (TS default), drops every QueueWeak entry from p.queue before
// processing. Early-returns if no modal is currently open. Otherwise:
// nulls activeScript on COUNTDIALOG/PAUSEBUTTON suspends (closes
// NAI-52-F1) and dispatches a per-slot IF_CLOSE trigger script
// (Main → Chat → Side, TS order).
//
// Mirrors TS Player.closeModal (Player.ts:741-794). Body fully
// landed across NAI-53 T1-T5; per-slot clearComListeners wired in
// NAI-64 (TS Player.ts:728-739, 767, 778, 789).
//
// DEVIATION-NAI-111-D1: NAI-52 convergence narrowed. CloseModal does
// NOT touch p.activeScript.Pointers — TS Player.closeModal clears
// this.protect (Player-level bool) but contains zero pointer
// operations on script.pointers&PAP. Goscape maps PAP only to the
// script-state bitmask; the external "is a protected script
// suspended?" question is answered by protectedScriptActive, which
// reads the preserved pointer on the stored *ScriptState across
// suspensions (StoreActiveScript at player_script.go:138-140
// preserves Pointers). NAI-53 T3's earlier convergence claim that
// CloseModal must clear PAP was incorrect — it stripped the gate
// from in-flight resumed scripts (e.g. tut_close inside
// [label,tutorial_complete] caused P_TELEJUMP to abort).
```

- [ ] **Step 3c: Replace the CanAccess doc-comment** at `player_script.go:268-282`.

Old (current HEAD, lines 268-282):

```go
// CanAccess implements script.ActivePlayer.CanAccess — the P_FINDUID
// protected-binding gate. False when delayed, when a modal main/chat
// is open, or when a suspended protected script is stored. Mirrors TS
// Player.canAccess at Engine-TS/src/engine/entity/Player.ts:805-812.
//
// The World-shutdown early-return from TS is omitted — goscape has
// no global shutdown flag to consult and rejects lookups uniformly.
//
// The third branch derives what TS expresses as a single Player.protect
// bool from activeScript.Pointers&PtrProtectedActivePlayer. They are equivalent: TS persists the
// flag onto the player at script suspension (Player.ts:2141) and clears
// it at script completion (:2103-2114), so "is the player in a stored
// protected script?" and "is the player-level protect flag set?" are
// the same condition — goscape just reads it from the stored state
// instead of a redundant bool field.
```

New:

```go
// CanAccess implements script.ActivePlayer.CanAccess — the P_FINDUID
// protected-binding gate. False when delayed, when a modal main/chat
// is open, or when a protected script is stored on activeScript.
// Mirrors TS Player.canAccess at Engine-TS/src/engine/entity/Player.ts:805-812.
//
// The World-shutdown early-return from TS is omitted — goscape has
// no global shutdown flag to consult and rejects lookups uniformly.
//
// The third branch reads activeScript.Pointers&PtrProtectedActivePlayer
// to answer TS's `!this.protect` gate. The mapping holds because
// goscape's StoreActiveScript (player_script.go:138-140) preserves
// Pointers across suspensions — so "is a protected script stored on
// the player?" and TS's persisted `this.protect` bool produce
// identical observable behavior across the canAccess + walktrigger
// gates. See DEVIATION-NAI-111-D1 on CloseModal for the full
// narrowed-convergence rationale.
```

- [ ] **Step 3d: Replace the protectedScriptActive doc-comment** at `player_script.go:296-301`.

Old (current HEAD, lines 296-301):

```go
// protectedScriptActive reports whether the player currently owns a
// suspended protected script — goscape's mapping of TS Player.protect.
// Used by CanAccess (above) and processWalktrigger to gate operations
// that TS guards with !this.protect. See the CanAccess doc-comment for
// the activeScript.Pointers&PtrProtectedActivePlayer ↔ TS Player.protect equivalence rationale.
// NAI-52.
```

New:

```go
// protectedScriptActive reports whether the player currently has a
// protected script stored on activeScript — goscape's mapping of
// TS Player.protect for external (non-handler) consumers. Used by
// CanAccess (above) and processWalktrigger to gate operations that
// TS guards with !this.protect. See CanAccess + DEVIATION-NAI-111-D1
// on CloseModal for the narrowed-convergence rationale: the
// activeScript-pointer reading produces identical observable behavior
// to TS Player.protect because StoreActiveScript preserves Pointers
// across suspensions. NAI-52, narrowed in NAI-111.
```

### Step 4: Run tests to verify they pass (green)

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestCloseModal' -v`

Expected: every test in the `TestCloseModal*` cohort **PASSES**, including the two new regressions.

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: full test suite passes. No existing test outside `modal_close_test.go` should break — Stage 1 G3 inventoried every PAP consumer and verified behavior is preserved (4 drift-tolerant external consumers, all in-flight handler gates restored to correct semantics).

If `pkg/script/handlers_player_test.go`, `pkg/script/handlers_inv_test.go`, `pkg/script/handlers_vars_test.go`, or `modules/world/interaction_test.go` regresses, stop and inspect — Stage 1's "no changes needed" verdict on those cohorts may have missed a test that exercised a handler from a non-resumed path where the over-clear coincidentally produced the expected outcome.

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`

Expected: passes. The change introduces no concurrency surface (it only deletes existing read+write).

### Step 5: Commit

- [ ] Run:

```bash
git add modules/world/player_script.go modules/world/modal_close_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(nai-111): Stage 2 — delete CloseModal protect over-clear (PRIMARY)

Restores TS Player.closeModal semantics: closeModal does NOT touch
script.pointers&PAP. Deletes the 3-LOC NAI-52-convergence-driven clear
at player_script.go:728-730 that stripped PtrProtectedActivePlayer from
in-flight resumed scripts (tut_close/if_close inside a protected
script broke the next requireProtectedActivePlayer-gated opcode, e.g.
P_TELEJUMP at [label,tutorial_complete]).

DEVIATION-NAI-111-D1 documents the narrowed convergence:
script.Pointers&PAP maps only to TS script.pointers&PAP (script-state);
external "is a protected script stored?" gate is answered by
protectedScriptActive reading the preserved pointer on the stored
*ScriptState (StoreActiveScript preserves Pointers across suspensions).

Tests: retire 3 broken-behavior pins (NAI-53 T3 cohort), narrow 1 to
weak-queue-only, drop 1 obsolete delayed=true workaround, add 2
regression pins (in-flight Pointers preserved + protectedScriptActive
stays true post-CloseModal).

Closes memory: nai_111_protect_over_clear

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] Run `git status` to verify the commit succeeded and no stray untracked files were missed.

Expected: clean working tree on `main` (modulo pre-existing untracked `.claude/` and `test_typed_nil.go`).

### Step 6: Code review (Sonnet)

After the implementer commits, dispatch a Sonnet code-reviewer subagent per `superpowers_code_reviewer_model` (NEVER Opus). Reviewer scope:
- Verify the production diff matches §B exactly: 3 LOC deleted, 3 doc-comments revised, no other code changes in `player_script.go`.
- Verify the test diff matches §C Step 1: 2 retired, 1 narrowed, 1 setup-cleaned, 2 added.
- Sanity-check: full `go test ./...` passes locally (re-run in reviewer).
- Sanity-check: no `_test.go` outside `modal_close_test.go` was touched.
- Confirm DEVIATION-NAI-111-D1 doc-comment block lives in `CloseModal` per `defensive_gate_doc_comment_label`.

If reviewer flags blockers, fix in a follow-up commit; non-blocking nits at controller's discretion.

---

## §D Smoke handoff (post-merge, user-launched per `smoke_test_server_handoff`)

The sandbox cannot reach the host's Java client. Controller asks the user to run the goscape server (`CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml`) and connect with the Java client.

**Path:**
1. Login fresh character on Tutorial Island.
2. Progress to Magic Instructor (Terrova).
3. Cast Wind Strike on a chicken successfully.
4. Re-talk to Magic Instructor; the dialog "Do you want to go to the mainland?" appears.
5. Pick "Yes."

**Pre-fix expected log:** `script "[label,tutorial_complete]": P_TELEJUMP: script not protected at pc=N` immediately after `if_close;`.

**Post-fix expected log:** `tut_close`, `if_close`, `p_telejump` execute without abort. Java client renders the player at world coord (3222, 3222) level 0 — the `0_50_50_22_22` packed-coord (`level=0, square=50,50, local=22,22`) Lumbridge spawn destination.

**PRIMARY closes** on smoke-bind per `cascade_theory_smoke_binding`. Adjacent residuals downstream of `tutorial_complete` (`inv_clear`, `inv_add`×16, `~stat_reset_all`, `~initalltabs` at `tutorial.rs2:303-327`) route to NAI-N+1 per `smoke_surfaces_adjacent_divergences` — the P_TELEJUMP gate restoration is the bound symptom regardless of downstream content gates (`dispatch_correct_reach_blocked`).

---

## §E Self-review checklist

Run after writing the plan:

- [x] **Spec coverage:** Spec §3 Scenario A specifies (a) delete L728-730, (b) revise doc-comments at 712-723/270-282/296-301, (c) retire 3 modal_close_test.go tests, (d) add 1-2 regression tests. All four mapped to §C Steps 1-3.
- [x] **Placeholder scan:** No "TBD", "implement later", "add appropriate error handling" — every Edit shows the exact old/new content.
- [x] **Type consistency:** All test fixtures use `script.PtrActivePlayer`, `script.PtrProtectedActivePlayer`, `script.Running`, `script.Suspended`, `script.PauseButton`, `script.CountDialog`, `script.QueueWeak` — names match `pkg/script` exports as used by current `modal_close_test.go`. `script.ScriptState{Script, Execution, Pointers}` field names match Stage 1's verified usage.
- [x] **Smoke binding:** §D specifies the user-launched smoke and pre/post-fix log expectations.
- [x] **DEVIATION tag:** D1 lives in CloseModal doc-comment per `defensive_gate_doc_comment_label`; cross-referenced from CanAccess + protectedScriptActive docs.
- [x] **Cadence:** `compressed_cadence` (~3 LOC + test churn well under 15 LOC threshold) → single combined spec+plan doc, single TDD bundle.
- [x] **Reviewer model:** §C Step 6 requires Sonnet per `superpowers_code_reviewer_model`.
- [x] **Post-merge git status check:** §C Step 5 includes `git status` per `feedback_subagent_wt_path`.
