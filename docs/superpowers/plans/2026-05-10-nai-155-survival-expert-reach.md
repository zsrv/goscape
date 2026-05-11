# NAI-155 — Survival Expert second-contact reach (investigation) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Attribute and fix the Tutorial Island Survival Expert (#943) second-OPNPC1 reach regression surfaced by NAI-154 smoke. Frame B already shows `branch_pre=0 && branch_post=0` on both failing ticks ⇒ `tryInteract` guard trips on `!CanAccess()`. Plan dispatches Bundle 1 (TS-fidelity gate parity audit) and Bundle 2 (CanAccess residue root-cause audit) in parallel, then conditionally lands the Stage 2 gate-parity fix.

**Architecture:** Investigation sub-spec per `investigation_subspec_cadence`. Stage 1: two parallel Sonnet audit subagents emit findings docs (no code). Decision gate. Stage 2 (likely): align `processInteraction` to TS Player.ts:1210/1232/1244 canAccess gates via TDD. Stage 3: conditional — close NAI-155 PRIMARY on smoke green, route Bundle 2 residue to NAI-156, OR escalate residue to in-scope if smoke fails.

**Tech Stack:** Go 1.26+. No new deps. Stage 2 touches `modules/world/interaction.go` (`processInteraction` + relax inner guard in `tryInteract`). Test fixtures in `modules/world/interaction_debug_test.go` already exercise the branch-id pinning surface; new TDD tests will live alongside.

---

## Spec Reference

Per `docs/superpowers/specs/2026-05-10-nai-155-survival-expert-reach-design.md`.

**Frame B pin** (from spec §1, must be preserved across all audit subagent contexts):

```
tick=222 player_uid=2232170497 target_kind=Npc target_type_id=943
  target_x=3105 target_z=3095 player_x=3105 player_z=3096 cheb_dist=1
  op_trigger=true ap_trigger=false ap_range=10 waypoint_idx=-1
  branch_pre=0 branch_post=0 interacted=false interaction_fired=false
  steps_taken=1 repathed=false target_still_set=true
tick=223 (same coords) branch_pre=0 branch_post=0 interacted=false
  interaction_fired=false steps_taken=0 repathed=false
  target_still_set=false
```

**Bundle 0 result (already resolved):** Both ticks guard-trip on `!CanAccess()` at `interaction.go:387` (HasInteraction true: target non-nil + not followOp).

## File Structure

- **Create:** `docs/superpowers/audits/2026-05-10-nai-155-bundle1-gate-parity.md` — Bundle 1 findings doc emitted by Task 1 subagent
- **Create:** `docs/superpowers/audits/2026-05-10-nai-155-bundle2-canaccess-residue.md` — Bundle 2 findings doc emitted by Task 2 subagent
- **Modify (Task 3 if Bundle 1 RED):** `modules/world/interaction.go` — gate-parity patch at `processInteraction` (L245-274) + relax `tryInteract` inner guard (L387)
- **Create (Task 3 tests):** `modules/world/interaction_canaccess_gate_test.go` — 5 TDD test pins

## Controller pre-flight (1 minute, before Task 1 dispatch)

Per `controller_preflight` memory. Verify the audit premises against HEAD.

- [ ] **Step 0.1:** Confirm `interaction.go:245-274` line ranges match the spec.

Run: `sed -n '240,280p' /home/owner/Code/github.com/zsrv/goscape/modules/world/interaction.go`

Expected: pre-step arm at ~L244-249, post-step at L267-273, exact shape:
```go
// Pre-step interact arm (TS L1209-1224).
if !followOp {
    p.processWalktrigger()
}
p.interactCallSlot = 0
interacted = p.tryInteract(false)
```
and
```go
if p.target != nil && !followOp {
    p.interactCallSlot = 1
    interacted = p.tryInteract(p.stepsTaken == 0)
    if !interacted && !p.hasWaypoints() && p.stepsTaken == 0 {
        p.MessageGame("I can't reach that!")
        p.ClearInteraction()
    }
}
```

If lines have drifted (post-NAI-154 commits), update Task 3 line targets accordingly.

- [ ] **Step 0.2:** Confirm `tryInteract` inner guard at `interaction.go:387` shape.

Run: `sed -n '378,395p' /home/owner/Code/github.com/zsrv/goscape/modules/world/interaction.go`

Expected: `if p.target == nil || !p.HasInteraction() || !p.CanAccess() { recordTryInteractBranch(p, 0); return false }`. If different, Task 3 patch shape needs adjusting.

- [ ] **Step 0.3:** Confirm `CanAccess` shape at HEAD.

Run: `sed -n '320,340p' /home/owner/Code/github.com/zsrv/goscape/modules/world/player_script.go`

Expected: 3-clause `if p.delayed || modalState&(Main|Chat)!=0 || protectedScriptActive()` returns false; else true.

- [ ] **Step 0.4:** Enumerate all `tryInteract` callers (Risk R1).

Run: `rg -n "\\.tryInteract\\(" modules/world/ pkg/ --type go`

Expected: exactly two production call sites in `modules/world/interaction.go` (L249 pre-step, L269 post-step); test sites in `modules/world/interaction_debug_test.go`. If a third production caller exists, the Bundle 1 inner-guard relaxation (Task 3) must preserve their gate semantics.

---

## Task 1 — Bundle 1 audit: TS-fidelity gate parity at `processInteraction`

**Files:**
- Create: `docs/superpowers/audits/2026-05-10-nai-155-bundle1-gate-parity.md`

**Dispatch:** Single Sonnet subagent (`general-purpose`). Read-only — must NOT modify code. Output is a findings doc.

- [ ] **Step 1.1:** Dispatch the audit subagent with the prompt below.

```
Subagent: general-purpose, model=sonnet, run_in_background=false
Description: NAI-155 Bundle 1 TS-fidelity gate parity audit
```

**Prompt (self-contained — subagent has no conversation context):**

> You are auditing a TS→Go port for a specific TS-fidelity divergence. Read-only — do NOT edit code. Emit a findings document only.
>
> **Context:** goscape (Go) ports LostCityRS Engine-TS (TypeScript). Files:
> - TS source: `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/Player.ts`, `processInteraction` at L1200-1268, `canAccess` at L805-812.
> - Goscape source: `/home/owner/Code/github.com/zsrv/goscape/modules/world/interaction.go`, `processInteraction` at L189-309, `tryInteract` at L378-454; `CanAccess` at `modules/world/player_script.go:324-335`; `HasInteraction` at `player_script.go:1080-1088`.
>
> **The bug:** Frame B for the failing tick shows `branch_pre=0 && branch_post=0` with `target_still_set=true`, `target_kind=Npc`, `op_trigger=true`, `cheb_dist=1`. The `tryInteract` guard at `interaction.go:387` (`!p.HasInteraction() || !p.CanAccess()`) trips on `!CanAccess()`.
>
> **Hypothesis to audit:** TS gates the post-step interact block (including the user-visible `"I can't reach that!"` message at L1249 and `clearInteraction()` at L1250) on `this.target && this.canAccess() && !followOp` (L1244). Goscape's equivalent at `interaction.go:267` gates only on `p.target != nil && !followOp` — MISSING the canAccess gate. TS also gates the pre-step arm at L1210 and the post-step walktrigger at L1232 on canAccess(); goscape does not.
>
> **Audit tasks:**
> 1. Line-by-line diff `processInteraction` (TS L1200-1268 vs goscape L189-309). Identify every canAccess-related gate divergence. Report each as a row in a table: TS line | TS gate | goscape line | goscape gate | divergent? (Y/N).
> 2. For each divergence, determine whether the goscape shape preserves OR destroys an interaction that TS would preserve when canAccess()=false. Specifically: does the missing gate let goscape reach a `ClearInteraction()` call (`interaction.go:262, 272, 290`) that TS would skip?
> 3. Enumerate all production `tryInteract` callers (goscape) via `rg -n "\\.tryInteract\\(" modules/world/ pkg/`. For each, confirm whether moving the canAccess check from the inner guard (L387) up to the call site (per TS shape) preserves correctness.
> 4. Check Risk R2 from spec §9: TS L1212 calls `validateTarget()` inside the canAccess-gated pre-step arm; goscape has a level-mismatch subset at `interaction.go:230-240` BEFORE the pre-step arm (runs unconditionally). Confirm this ordering doesn't conflict with a new pre-step canAccess gate.
> 5. Check Risk R3: TS L1237-1239 followOp + waypoint-exhaustion clear runs UNGATED on canAccess. Goscape `interaction.go:261-263` matches. Confirm.
> 6. Check Risk R4: goscape `processInteraction` entry guard at L196-202 (`if p.delayed && s.currentTick < p.delayedUntil { return }`) is a stricter early-return than CanAccess (which is just `p.delayed` without tick math). Does adding call-site canAccess gates make this entry guard redundant, or is it still load-bearing? (Hint: entry guard returns BEFORE Frame B emit.)
> 7. Emit the patch shape for `processInteraction` matching TS L1209/1232/1244 gates. Also emit the relaxation for `tryInteract` inner guard at L387.
>
> **Output format:** Write to `/home/owner/Code/github.com/zsrv/goscape/docs/superpowers/audits/2026-05-10-nai-155-bundle1-gate-parity.md`. Sections:
> 1. Verdict: GREEN (no divergence) / RED (divergence found) / YELLOW (partial).
> 2. Gate-divergence table (one row per canAccess-related gate site).
> 3. Risk audits R1-R4 with verdict + evidence per memory `audit_full_method_against_ts`.
> 4. Concrete patch shape for `processInteraction` (full function body if needed).
> 5. Concrete patch shape for `tryInteract` inner guard.
> 6. List of `tryInteract` production call sites + each one's gate-shift impact.
>
> Cite line numbers as `path:line`. Do NOT modify any code. Length budget: ~600 words.

- [ ] **Step 1.2:** Read the emitted findings doc.

Run: `cat /home/owner/Code/github.com/zsrv/goscape/docs/superpowers/audits/2026-05-10-nai-155-bundle1-gate-parity.md`

Expected: RED verdict (hypothesis confirmed). If GREEN, the bug is NOT gate parity — re-evaluate Bundle 2 priority and consult user before Task 3.

- [ ] **Step 1.3:** Commit the findings doc.

```bash
git add docs/superpowers/audits/2026-05-10-nai-155-bundle1-gate-parity.md
git commit --no-gpg-sign -m "docs(nai-155): Bundle 1 audit — TS-fidelity gate parity findings"
```

---

## Task 2 — Bundle 2 audit: WHY `CanAccess()` is false on the second contact

**Files:**
- Create: `docs/superpowers/audits/2026-05-10-nai-155-bundle2-canaccess-residue.md`

**Dispatch:** Single Sonnet subagent (`general-purpose`). Parallel with Task 1 (no dependency between them — send both Agent calls in a single message). Read-only.

- [ ] **Step 2.1:** Dispatch the audit subagent with the prompt below.

```
Subagent: general-purpose, model=sonnet, run_in_background=false
Description: NAI-155 Bundle 2 CanAccess residue root-cause audit
```

**Prompt (self-contained):**

> You are root-causing a state-residue bug in a TS→Go port. Read-only — do NOT edit code. Emit a findings document only.
>
> **Context:** goscape's `(*Player).CanAccess()` at `/home/owner/Code/github.com/zsrv/goscape/modules/world/player_script.go:324-335` returns false when ANY of:
> 1. `p.delayed`
> 2. `p.modalState & (modalStateMain | modalStateChat) != 0`
> 3. `p.protectedScriptActive()` (i.e. `p.activeScript != nil && p.activeScript.Pointers & script.PtrProtectedActivePlayer != 0`)
>
> **The bug:** the Tutorial Island Survival Expert (NPC #943) chatnpc opens a dialog on first OPNPC1. After dialog dismiss, a second OPNPC1 click on the same NPC produces `CanAccess()=false` (Frame B `branch_pre=0 && branch_post=0` across two ticks). Smoke packet log shows opcode `[31 76 43 44]` arriving 383ms after the second OPNPC1 — candidate dialog-close packet.
>
> **Audit tasks:** Identify which of the three fields is residually true on tick 222 of the failing trace, and find the missing clear-site OR root cause of the residue.
>
> 1. **`p.delayed` lifecycle.** Setter: `player_script.go:53-54` (`p.delayed = true; p.delayedUntil = currentTick+1+ticks`). Sole clearer: `tick.go:278` (`p.delayed = false`). Read the clearer context (50 lines around tick.go:278). Is the clear unconditional once `currentTick >= delayedUntil`, or gated? Could a chatnpc set delayed and have it dangle? Cross-reference TS counterpart at Engine-TS Player.ts where TS's `this.delayed` is cleared.
>
> 2. **`modalState & Chat` lifecycle.** Sole setter: `player_script.go:867` (`p.modalState = modalStateChat`) — this is the chatnpc opener. Sole clearer: `player_script.go:799` (`p.modalState = modalStateNone`) inside `CloseModal`. Audit:
>    - Which decode handler / packet calls `CloseModal`? `rg -n "CloseModal\\(" modules/world/ pkg/ --type go`.
>    - Goscape opcode 31 (visible in smoke packet log): identify which handler. `rg -n "case 31\\b|Opcode.*= ?31\\b|0x1f" modules/world/ pkg/io/`. Likely `OpResumePauseButton` or similar.
>    - TS counterpart: search `Engine-TS/src/engine/network/incoming/` for the equivalent close-modal packet handler. Does TS clear modal on this opcode?
>    - Could packet processing ordering let modal STAY set when `processInteractions` runs on tick 222 (the second OPNPC1 click tick)?
>
> 3. **`protectedScriptActive()` lifecycle.** Per memory `nai_111_protect_over_clear.md`: "TS Player.protect bool ≠ goscape script.Pointers&PAP for in-flight handlers; CloseModal must NOT strip PAP". Audit:
>    - When does `p.activeScript` get cleared after a chatnpc completes? Search `rg -n "activeScript ?= ?nil|ClearActiveScript|p\\.activeScript ?=" modules/world/ pkg/script/`.
>    - Could a chatnpc that suspends mid-execution with `PtrProtectedActivePlayer` set fail to clear `activeScript` on resume-to-end? Read `StoreActiveScript` (`player_script.go:138-140` per the CanAccess doc-comment).
>    - TS counterpart `Player.protect` at Engine-TS Player.ts: when is `this.protect` set/cleared in the chatnpc lifecycle?
>
> 4. **Triangulate the residual field.** Given:
>    - The packet log: second OPNPC1 at 21:09:51.833 (start of tick 222); 4-byte packet at 21:09:52.216 (between ticks 222 and 223, possibly dialog-close); tick 223 frame at 21:09:52.433.
>    - Frame B emits at the END of `processInteraction` (interaction.go:306-308), AFTER both tryInteract calls would have run.
>    - The Frame B record does NOT include `delayed`, `modalState`, or `activeScript` fields. Recommend the SMALLEST instrumentation extension (one extra slog field per candidate) to disambiguate in a follow-up smoke if static audit is inconclusive.
>
> 5. **Identify the missing clear-site (if any).** If a TS clear-site exists for the residual field that goscape lacks, name the file:line in TS and the file:line in goscape where the clear should be added.
>
> **Output format:** Write to `/home/owner/Code/github.com/zsrv/goscape/docs/superpowers/audits/2026-05-10-nai-155-bundle2-canaccess-residue.md`. Sections:
> 1. Verdict: identify which field is residually true (or "INCONCLUSIVE — instrumentation needed").
> 2. Evidence per field (1, 2, 3 above).
> 3. Root cause: missing clear-site OR packet-ordering bug OR something else.
> 4. Proposed instrumentation extension to Frame B (1-3 extra slog fields) for disambiguation if static audit is inconclusive.
> 5. Routing recommendation: if Bundle 1 fix alone clears smoke, can Bundle 2 wait for NAI-156? Or is Bundle 2 load-bearing?
>
> Cite line numbers as `path:line`. Do NOT modify any code. Length budget: ~600 words.

- [ ] **Step 2.2:** Read the emitted findings doc.

Run: `cat /home/owner/Code/github.com/zsrv/goscape/docs/superpowers/audits/2026-05-10-nai-155-bundle2-canaccess-residue.md`

- [ ] **Step 2.3:** Commit the findings doc.

```bash
git add docs/superpowers/audits/2026-05-10-nai-155-bundle2-canaccess-residue.md
git commit --no-gpg-sign -m "docs(nai-155): Bundle 2 audit — CanAccess residue root-cause findings"
```

---

## Decision Gate (between Task 2 and Task 3)

After both audit docs are committed, review verdicts and route:

| Bundle 1 | Bundle 2 | Action |
|---|---|---|
| RED | RED / INCONCLUSIVE | Proceed to Task 3 (Stage 2 gate-parity fix). Route Bundle 2 to NAI-156 if smoke green, else escalate as Stage 3. |
| RED | GREEN | Proceed to Task 3. Bundle 2 GREEN means the residue is intentional/expected; gate-parity fix suffices. |
| GREEN | RED | **Stop and consult user.** Bundle 1 was the prime hypothesis; if disproven, brainstorm Bundle 2-only fix shape. |
| GREEN | GREEN | **Stop and consult user.** Both audits clear → unknown root cause. Re-derive from primary sources. |

Expected path: RED + RED (or RED + INCONCLUSIVE). Proceed to Task 3.

---

## Task 3 — Stage 2: gate-parity fix (TDD)

**Conditional on Bundle 1 RED.** If GREEN, skip and consult user.

**Files:**
- Create: `modules/world/interaction_canaccess_gate_test.go`
- Modify: `modules/world/interaction.go` (`processInteraction` body + `tryInteract` inner guard)

### Step 3.1: Read the existing test fixture surface

- [ ] Confirm test-helper conventions before authoring new tests.

Run: `rg -n "func newTestPlayer|func newTestServer|func newTestNpc" modules/world/*_test.go | head -10`

Read the matching helper definitions. Goal: use the same fixture idioms as existing interaction tests (per memory `test_fixture_view_parity`).

Run: `sed -n '1,60p' modules/world/interaction_debug_test.go`

Note the package, imports, and Player construction pattern.

### Step 3.2: Write the 5 failing test pins

- [ ] Create `modules/world/interaction_canaccess_gate_test.go` with the tests below.

**File contents** (full file):

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// TestProcessInteraction_CanAccessGate_ModalChat_PreservesInteraction pins
// TS Player.ts:1244 fidelity: when CanAccess()=false due to modalState&Chat,
// the post-step interact block (including "I can't reach!" + ClearInteraction)
// must NOT fire. The interaction is preserved across the tick.
func TestProcessInteraction_CanAccessGate_ModalChat_PreservesInteraction(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayerOn(s, 3105, 3096, 0)
	n := newTestNpc(s, 943, 3105, 3095, 0)

	p.SetInteraction(InteractionEngine, n, 1, -1)
	p.modalState = modalStateChat // residue from prior chatnpc dialog

	mailbox := captureMessages(p)
	p.processInteraction()

	if p.target == nil {
		t.Fatalf("interaction destroyed under modalChat residue; TS L1244 preserves")
	}
	for _, m := range mailbox.collected() {
		if m == "I can't reach that!" {
			t.Fatalf("'I can't reach' fired under canAccess=false; TS L1244 gates the message")
		}
	}
	if p.interactionFired {
		t.Fatalf("interactionFired set true under canAccess=false; tryInteract guard should block")
	}
}

// TestProcessInteraction_CanAccessGate_ProtectedScript_PreservesInteraction pins
// the same invariant for protectedScriptActive()=true (TS Player.protect path,
// memory protect_over_clear / NAI-111).
func TestProcessInteraction_CanAccessGate_ProtectedScript_PreservesInteraction(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayerOn(s, 3105, 3096, 0)
	n := newTestNpc(s, 943, 3105, 3095, 0)

	p.SetInteraction(InteractionEngine, n, 1, -1)
	p.activeScript = &script.ScriptState{Pointers: script.PtrProtectedActivePlayer}

	mailbox := captureMessages(p)
	p.processInteraction()

	if p.target == nil {
		t.Fatalf("interaction destroyed under protected-script residue; TS L1244 preserves")
	}
	for _, m := range mailbox.collected() {
		if m == "I can't reach that!" {
			t.Fatalf("'I can't reach' fired under canAccess=false (protected); TS L1244 gates the message")
		}
	}
}

// TestProcessInteraction_CanAccessGate_HappyPath_OpFires regression-pins the
// success case: CanAccess()=true, target at cheb=1, opTrigger present →
// Branch 1 fires opnpc1 trigger, interactionFired flips true.
func TestProcessInteraction_CanAccessGate_HappyPath_OpFires(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayerOn(s, 3105, 3096, 0)
	n := newTestNpc(s, 943, 3105, 3095, 0)
	registerOpNpc1Trigger(t, s, 943) // shared helper installs an empty opnpc1 script

	p.SetInteraction(InteractionEngine, n, 1, -1)
	p.processInteraction()

	if !p.interactionFired {
		t.Fatalf("happy-path OPNPC1 did not fire; interactionFired=false")
	}
	if p.lastInteractBranchPre != 1 {
		t.Fatalf("expected pre-step branch 1 (OP fire); got %d", p.lastInteractBranchPre)
	}
}

// TestProcessInteraction_CanAccessGate_Delayed_EarlyReturnsBeforePathing pins
// the existing early-return at interaction.go:200-202: when delayed and within
// the delay window, processInteraction returns BEFORE the new canAccess gates
// would have fired. (Risk R4 in spec.)
func TestProcessInteraction_CanAccessGate_Delayed_EarlyReturnsBeforePathing(t *testing.T) {
	s := newTestServer(t)
	s.currentTick = 100
	p := newTestPlayerOn(s, 3105, 3096, 0)
	n := newTestNpc(s, 943, 3105, 3095, 0)

	p.SetInteraction(InteractionEngine, n, 1, -1)
	p.delayed = true
	p.delayedUntil = 105 // currentTick < delayedUntil
	p.lastInteractBranchPre = 99 // sentinel — should remain 99 (Frame B reset skipped)

	p.processInteraction()

	if p.lastInteractBranchPre != 99 {
		t.Fatalf("entry guard at L200-202 not hit; Frame B reset clobbered sentinel (got %d, want 99)", p.lastInteractBranchPre)
	}
	if p.target == nil {
		t.Fatalf("entry-guard early-return cleared interaction; should preserve")
	}
}

// TestProcessInteraction_CanAccessGate_NilTarget_PostStepSkipped negatively pins
// the post-step block: when target is nil (e.g., cleared by a mid-tick path),
// the post-step interact does not run. (TS L1244 gates on this.target.)
func TestProcessInteraction_CanAccessGate_NilTarget_PostStepSkipped(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayerOn(s, 3105, 3096, 0)
	n := newTestNpc(s, 943, 3105, 3095, 0)

	p.SetInteraction(InteractionEngine, n, 1, -1)
	// Simulate a mid-tick target clear (entry guard already passed since
	// p.target was set at entry; pre-step would have nulled via some path).
	// Direct shortcut: enter with target=nil already; entry guard at L190-192
	// short-circuits before any branch logic.
	p.target = nil

	p.processInteraction()

	if p.lastInteractBranchPre != 0 || p.lastInteractBranchPost != 0 {
		t.Fatalf("post-step ran with nil target; pre=%d post=%d", p.lastInteractBranchPre, p.lastInteractBranchPost)
	}
}
```

**Pre-author preflight per memory `mock_recorder_field_naming_check` + `plan_runnable_test_fixtures`:**

Before authoring, the implementer MUST verify these helpers exist with these names:
- `newTestServer(t)` — search `rg -n "func newTestServer" modules/world/*_test.go`
- `newTestPlayerOn(s, x, z, level)` — search `rg -n "func newTestPlayer" modules/world/*_test.go`. If only `newTestPlayer` exists without coord params, the implementer adapts via field-set after construction (e.g., `p.x = 3105; p.z = 3096`). Document the substitution as a deviation in the commit message.
- `newTestNpc(s, typeId, x, z, level)` — search `rg -n "func newTestNpc" modules/world/*_test.go`. Similar adapt.
- `captureMessages(p)` — if not present, implementer creates a minimal helper inline that snapshots `p.outgoingMessages` or the slog stream. The pattern in `interaction_debug_test.go` may already cover this.
- `registerOpNpc1Trigger(t, s, npcTypeId)` — if not present, implementer creates a minimal helper that installs an empty `script.ScriptFile` keyed on `TriggerOpNpc1 + npcTypeId` into `s.scriptProvider`. Pattern: see existing trigger-registration helpers in `interaction_debug_test.go` or `npc_interaction_test.go`.

**If helpers don't match, do NOT invent — adapt to actual fixtures and document each substitution in the commit message.**

### Step 3.3: Run failing tests

- [ ] **Step 3.3:** Run all 5 new tests; confirm they fail.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestProcessInteraction_CanAccessGate ./modules/world/ -v`

Expected:
- `_ModalChat_PreservesInteraction` FAIL: `p.target == nil` (interaction destroyed) OR "I can't reach!" in mailbox.
- `_ProtectedScript_PreservesInteraction` FAIL: same.
- `_HappyPath_OpFires` PASS (regression baseline — no fix needed, existing happy path works).
- `_Delayed_EarlyReturnsBeforePathing` PASS (current entry guard already handles this).
- `_NilTarget_PostStepSkipped` PASS (current `target != nil` gate at L267 handles this).

If `_HappyPath` fails: stop, fix the fixture/helper mismatch, do not proceed.

### Step 3.4: Apply the gate-parity patch

- [ ] **Step 3.4:** Replace `processInteraction` body in `modules/world/interaction.go` to mirror TS L1209/1232/1244 canAccess gates.

The implementer reads the Bundle 1 audit doc's "Concrete patch shape for `processInteraction`" section and applies it. The expected diff shape (from spec §6):

**Replace** the pre-step arm at `interaction.go:244-249`:
```go
	// Pre-step interact arm (TS L1209-1224).
	if !followOp {
		p.processWalktrigger()
	}
	p.interactCallSlot = 0
	interacted = p.tryInteract(false)
```
**With:**
```go
	// Pre-step interact arm (TS L1209-1224). Gated on CanAccess so a
	// modal/protected/delayed state preserves the interaction across the
	// tick (TS skips both walktrigger and tryInteract; goscape mirrors).
	if p.target != nil && p.CanAccess() {
		if !followOp {
			p.processWalktrigger()
		}
		p.interactCallSlot = 0
		interacted = p.tryInteract(false)
	}
```

**Replace** the walktrigger inside the post-step block at `interaction.go:256-258`:
```go
		if p.hasWaypoints() {
			p.processWalktrigger()
		}
```
**With:**
```go
		// TS L1232 — walktrigger gated on canAccess.
		if p.hasWaypoints() && p.CanAccess() {
			p.processWalktrigger()
		}
```

**Replace** the post-step interact arm at `interaction.go:267-274`:
```go
		// Post-step interact (TS L1244-1252). Skipped when followOp
		// (the chase keeps interaction anchored across steps).
		if p.target != nil && !followOp {
			p.interactCallSlot = 1
			interacted = p.tryInteract(p.stepsTaken == 0)
			if !interacted && !p.hasWaypoints() && p.stepsTaken == 0 {
				p.MessageGame("I can't reach that!")
				p.ClearInteraction()
			}
		}
```
**With:**
```go
		// Post-step interact (TS L1244-1252). Gated on CanAccess so a
		// modal/protected/delayed state preserves the interaction
		// (including the "I can't reach!" message + ClearInteraction)
		// across the tick. Pre-NAI-155 goscape lacked this gate; second-
		// contact OPNPC1 on Tutorial Island Survival Expert destroyed
		// the anchor when a residual modalChat or PAP tripped CanAccess.
		if p.target != nil && p.CanAccess() && !followOp {
			p.interactCallSlot = 1
			interacted = p.tryInteract(p.stepsTaken == 0)
			if !interacted && !p.hasWaypoints() && p.stepsTaken == 0 {
				p.MessageGame("I can't reach that!")
				p.ClearInteraction()
			}
		}
```

**Relax** the `tryInteract` inner guard at `interaction.go:387`:
```go
	if p.target == nil || !p.HasInteraction() || !p.CanAccess() {
		recordTryInteractBranch(p, 0) // NAI-79 Stage 1 (combined early-return)
		return false
	}
```
**To:**
```go
	// NAI-155: CanAccess gate lifted to processInteraction call sites
	// (TS L1210/1244 parity). Inner guard now matches TS Player.ts:1114
	// shape: target presence + HasInteraction (follow-op filter). Callers
	// that don't go through processInteraction (none in current tree per
	// NAI-155 controller-preflight Step 0.4) must gate canAccess at their
	// site.
	if p.target == nil || !p.HasInteraction() {
		recordTryInteractBranch(p, 0) // NAI-79 Stage 1 (combined early-return)
		return false
	}
```

Use the Edit tool with these old/new pairs. **If the Bundle 1 audit doc prescribes a different shape** (e.g., the audit found an additional gate divergence at validateTarget, R3, or elsewhere), defer to the audit doc.

### Step 3.5: Re-run the failing tests; confirm they pass

- [ ] **Step 3.5:** Run all 5 new tests; expect ALL pass.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestProcessInteraction_CanAccessGate ./modules/world/ -v`

Expected: all 5 PASS.

### Step 3.6: Run the full world-package test suite for regressions

- [ ] **Step 3.6:** Verify no regression in adjacent interaction tests.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: PASS. Note: existing `interaction_debug_test.go` tests pin `tryInteract` branch IDs — if any of those tests rely on the inner guard's `!CanAccess()` half (e.g., they set `p.delayed=true` or `p.modalState=Chat` and expect branch 0), they may now reach a different branch. Investigate any failures: either the test relied on guard semantics that have moved up to the call site (acceptable — update the test to assert the call-site behavior), OR the implementer over-relaxed the guard (revert and adjust).

### Step 3.7: Run the full repo test suite for cross-package regressions

- [ ] **Step 3.7:** Run all tests.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS. If a `pkg/script` test fails, the relaxation interacted with a script-side caller — re-read the controller-preflight Step 0.4 enumeration.

### Step 3.8: Commit the fix

- [ ] **Step 3.8:** Commit.

```bash
git add modules/world/interaction.go modules/world/interaction_canaccess_gate_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(world): NAI-155 T1 — CanAccess gate parity at processInteraction (TS L1244)

TS Player.ts:1210/1232/1244 gate pre-step interact, walktrigger, and
post-step interact (including the "I can't reach!" message + clearInteraction)
on canAccess(). Goscape's processInteraction lacked these site-gates and
relied on a stricter inner guard inside tryInteract that returned false
unconditionally on !CanAccess() — but the post-step block then fired
"I can't reach!" + ClearInteraction, destroying an interaction TS would
preserve across the canAccess-false tick.

NAI-154 Java-client smoke pinned the regression: Tutorial Island Survival
Expert (NPC #943) second OPNPC1 at cheb=1 with op_trigger=true failed to
fire (Frame B branch_pre=0 && branch_post=0 across two ticks; tryInteract
guard tripped on !CanAccess()).

Fix shape (per spec §6):
- Add `p.target != nil && p.CanAccess()` gate at the pre-step arm.
- Add `p.CanAccess()` gate to the in-block walktrigger.
- Add `p.CanAccess()` gate to the post-step arm (covers the "I can't
  reach!" message and ClearInteraction per TS L1244-1252).
- Relax tryInteract inner guard to drop the `!CanAccess()` half; gate
  has moved up to the two production call sites.

5 TDD pins added: ModalChat + ProtectedScript preserve interaction (the
load-bearing fix); HappyPath OP fires (regression baseline); Delayed
early-return at entry guard preserved; nil-target post-step skipped.
EOF
)"
```

---

## Task 4 — Smoke handoff (user-driven)

Per memory `smoke_test_server_handoff`: Java-client-driven smokes require the user to run the server (sandbox can't reach host network).

- [ ] **Step 4.1:** Emit the resume prompt for the user and STOP.

Per memory `post_task_handoff` + `superpowers_clear_between_spec_and_impl`: after the Stage 2 commit, the controller emits a paste-ready resume prompt and stops. The user runs the smoke, captures Frame B, pastes the result, then continues.

**Resume prompt template** (the controller emits this verbatim):

```
NAI-155 Stage 2 gate-parity fix landed (commit <SHA>). Please run the
Tutorial Island Survival Expert second-talk smoke:

1. CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml
2. Log in. Walk to Survival Expert (#943).
3. Right-click → Talk-to. Complete the dialog.
4. Right-click Survival Expert again. Talk-to.
5. PASS: dialog reopens.
   FAIL: "I can't reach that!" or no response.

Capture the "interaction tick" slog lines for:
- The first-talk success tick (baseline)
- The post-dismiss tick (after step 3 dialog close)
- The second-talk tick (step 4)

Paste all three Frame B records. Expected post-fix: second-talk tick
shows branch_pre=1 or branch_post=1 (OP fire), interaction_fired=true,
target_still_set=true (until auto-clear).

If second-talk Frame B shows branch_pre=0 && branch_post=0 across many
ticks, Bundle 2's CanAccess residue is load-bearing — escalate to
in-scope Stage 3 per spec §8.
```

---

## Task 5 — Close routing (conditional, after smoke)

- [ ] **Step 5.1:** Branch on smoke result.

**Smoke PASS (expected):**
- Open `MEMORY.md` and add a Closes entry pointing at the new audit + fix.
- Append a `Closes memory:` trailer to the Task 3.8 commit OR a fresh close commit per memory `close_commit_memory_trailer`.
- Write a NAI-156 seed entry in `nai_followups.md` IF Bundle 2 found a real residue that future scenarios will surface (per Bundle 2 doc's "Routing recommendation"). NAI-156 scope: clear the residual field on the appropriate lifecycle boundary.
- Emit close commit:
  ```bash
  git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
  chore(close): NAI-155 — Survival Expert second-talk gate parity landed

  Bundle 1 RED: TS-fidelity gate parity missing at processInteraction
  L1244. Stage 2 fix lifted CanAccess gate from tryInteract inner guard
  to three call sites (pre-step L1210, walktrigger L1232, post-step L1244).
  Java-client smoke green: second-talk dialog reopens cleanly.

  Bundle 2 routing: <copy verdict from audit doc; e.g., "modalState&Chat
  not cleared on opcode-31 dismiss; route to NAI-156 — residue is masked
  by gate-parity fix but persists, future protected-modal scenarios will
  re-surface."> OR "<INCONCLUSIVE — instrumentation extension queued for
  NAI-156>".

  Closes memory: docs/superpowers/audits/2026-05-10-nai-155-bundle1-gate-parity.md
  Closes memory: docs/superpowers/audits/2026-05-10-nai-155-bundle2-canaccess-residue.md
  EOF
  )"
  ```

**Smoke FAIL (branch_pre=0 && branch_post=0 persists across many ticks):**
- Bundle 2 residue is load-bearing. STOP and consult user to scope Stage 3.
- Stage 3 prompt: "Smoke shows CanAccess permanently false on second-contact — Bundle 2 root cause must be patched in-scope. The Bundle 2 audit identified <field>. Brainstorm the clear-site fix as Stage 3?"
- Do NOT close NAI-155 PRIMARY until both Bundle 1 fix + Bundle 2 fix are smoke-green.

**Smoke surfaces adjacent divergence (e.g., NPC wander mid-tick obstructs second contact at cheb>1):**
- ≤30 LOC: in-scope stretch within NAI-155.
- >30 LOC: route to NAI-156 per `smoke_surfaces_adjacent_divergences` memory.

- [ ] **Step 5.2:** Update memory.

Append to `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` IF Bundle 1 audit revealed a non-obvious TS-fidelity pattern (e.g., "interaction-preservation gates must mirror TS canAccess call-site gates, not inner-tryInteract guards"). One-line index entry per memory format.

---

## Self-Review

**Spec coverage:**
- Spec §5 Bundle 1 → Task 1 (audit dispatch). ✓
- Spec §5 Bundle 2 → Task 2 (audit dispatch). ✓
- Spec §6 Stage 2 fix → Task 3 (5 TDD pins + gate-parity patch + inner-guard relax). ✓
- Spec §7 Smoke → Task 4 (user handoff). ✓
- Spec §8 Stage 3 conditional → Task 5 (smoke-result routing). ✓
- Spec §9 Risks R1-R5 → Step 0.4 (R1), Step 1.1 audit prompt (R2-R4), Bundle 2 prompt covers R5. ✓
- Spec §10 Acceptance → Task 5 routes PRIMARY close + NAI-156 seed. ✓
- Spec §11 Memory hits → woven through Task prompts + Step 5.2. ✓

**Placeholder scan:** None. All commit messages, code blocks, test bodies, and audit prompts are inline. Conditional branches at Task 3 deferring to audit doc are bounded by explicit fallback shapes.

**Type consistency:** Test helper names (`newTestServer`, `newTestPlayerOn`, `newTestNpc`, `captureMessages`, `registerOpNpc1Trigger`) called out in Step 3.2 with explicit "verify before authoring" preflight per memory `mock_recorder_field_naming_check`. `modalState`/`modalStateChat`/`script.PtrProtectedActivePlayer`/`script.ScriptState`/`Pointers` names checked against grep results in earlier exploration.
