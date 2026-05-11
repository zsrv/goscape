# NAI-155 Stage 3B — WorldSuspended+PAP residue root-cause audit

**Date:** 2026-05-11
**Scope:** Read-only TS-fidelity audit. No code is modified here.
**Smoke input:** firelighting → talk to Survival Expert NPC #943 on Tutorial Island.
**Observed frame:** `modal_state=0 active_script_exec=7 protected_active=true branch_pre=0 branch_post=0 target_still_set=true`
**Decoded:** `modalState=None`, `Execution=WorldSuspended` (value 7 from `pkg/script/execution.go:16`), PAP bit set on the lingering `activeScript`.

---

## 1. Verdict

**Confirmed.** The provisional hypothesis is correct.

goscape keeps `p.activeScript` pointing at the now-world-queued `ScriptState` throughout the
world-queue wait window (`modules/world/script.go:132-146`). The NAI-44 divergence was
intentional (comment at `script.go:139-144`). As a result, when the firelighting script calls
`WORLD_DELAY` and enters the world queue:

- `p.activeScript` is non-nil and has `Pointers & PtrProtectedActivePlayer != 0`.
- `p.protectedScriptActive()` (`player_script.go:346-347`) returns `true`.
- `p.CanAccess()` (`player_script.go:324-335`) returns `false`.
- Every interaction attempt — including talk to Survival Expert — is blocked for the entire
  world-queue wait window.

TS does NOT do this. At `WORLD_SUSPENDED`, TS leaves `this.activeScript` pointing at whatever
was there before (it simply does nothing to it in the `WORLD_SUSPENDED` arm), while
simultaneously `this.protect` is NOT updated in that arm, so it retains the value set at the
previous player-side suspension. The key structural difference: TS `this.protect` is a
**separate boolean** from `this.activeScript`; clearing one does not affect the other. Goscape
collapsed them into a single pointer predicate (`protectedScriptActive = pointer + PAP bit`),
which introduced the coupling that caused this bug.

Specifically: in TS `executeScript` (Player.ts:2134-2150), the `WORLD_SUSPENDED` arm (L2135-2136)
enqueues to the world and falls through to the implicit `else` — it does NOT assign
`script.activePlayer.activeScript = script`, and it does NOT update `this.protect`. The effect is
that `this.activeScript` is left as whatever it was from the prior player-side state (could be
null if the script ran fresh from a queue entry), and `this.protect` is **also left from the prior
state** — meaning for a fresh queue-dispatched run, `this.protect` is still `false` at
`WORLD_SUSPENDED` because no `protect=true` arm was entered. In goscape, however, the NAI-44
divergence explicitly holds `p.activeScript = state` through the `WorldSuspended` arm
(`script.go:145-146`), setting up the false PAP positive.

---

## 2. TS canonical behavior

### Player.ts:2134-2150 — `executeScript` WORLD_SUSPENDED arm

```
2134:        if (state !== ScriptState.FINISHED && state !== ScriptState.ABORTED) {
2135:            if (state === ScriptState.WORLD_SUSPENDED) {
2136:                World.enqueueScript(script, script.popInt());
2137:            } else if (state === ScriptState.NPC_SUSPENDED) {
2138:                script.activeNpc.activeScript = script;
2139:            } else {
2140:                script.activePlayer.activeScript = script;
2141:                script.activePlayer.protect = protect; // preserve protected access when delayed
2142:            }
2143:        } else if (script === this.activeScript) {
2144:            this.activeScript = null;
2145:            ...
2150:        }
```

Key observation: `WORLD_SUSPENDED` is the FIRST `if` arm inside the "not finished/aborted" block.
It does `World.enqueueScript(...)` only. There is no assignment to `script.activePlayer.activeScript`
and no assignment to `script.activePlayer.protect`. Both are left unchanged from whatever they were.
For a world-queue-dispatched protect=true script, once the script hits `WORLD_DELAY` again this is
reached from `processWorld` (not from `executeScript`) so `this.protect` is not updated at all.

### Player.ts:741-747 — `closeModal` protect-clear

```
741:    closeModal(clearWeakQueue: boolean = true) {
742:        if (clearWeakQueue) {
743:            this.weakQueue.clear();
744:        }
745:        if (!this.delayed) {
746:            this.protect = false;
747:        }
```

TS clears `this.protect` inside `closeModal` unless the player is delayed. This is a **boolean
field clear** — it does not interact with `this.activeScript` at all. Goscape's
DEVIATION-NAI-111-D1 deliberately does not mirror this (CloseModal does not strip PAP).

### Player.ts:805-812 — `canAccess`

```
805:    canAccess() {
806:        if (World.shutdown) {
807:            return true;
808:        } else {
809:            return !this.protect && !this.busy();
810:        }
811:    }
```

TS checks `this.protect` directly — a standalone boolean. It does **not** check
`this.activeScript != null`. The two concepts are structurally independent in TS.

### World.ts:530-560 — `processWorld` world-queue dispatch

```
530:    private processWorld(): void {
534:        for (const request of this.queue.all()) {
536:            const delay = request.delay--;
537:            if (delay > 0) { continue; }
540:            const script: ScriptState = request.script;
542:                const state: number = ScriptRunner.execute(script);
545:                request.unlink();
547:                if (state === ScriptState.SUSPENDED) {
548:                    // suspend to player (probably not needed)
549:                    script.activePlayer.activeScript = script;
550:                } else if (state === ScriptState.NPC_SUSPENDED) {
551:                    // suspend to npc (probably not needed)
552:                    script.activeNpc.activeScript = script;
553:                } else if (state === ScriptState.WORLD_SUSPENDED) {
554:                    // suspend to world again
555:                    this.enqueueScript(script, script.popInt());
556:                }
```

On `FINISHED`/`ABORTED`, none of the `if` arms fire — the world queue entry was already removed
(`request.unlink()` at L545), and `script.activePlayer.activeScript` is NOT touched. TS does not
null the player's `activeScript` here. This means TS is relying on `executeScript`'s
FINISHED/ABORTED arm (Player.ts:2143-2144) to null `this.activeScript` — but that arm is only
reached when `executeScript` is the caller, not when `processWorld` is the caller.

Implication: when a world-queued script Finishes, TS `this.activeScript` on the player is
whatever it was before (often null if the script ran fresh, never having been stored there).
goscape's `OnScriptFinishedOrAborted` is only called from `resumeOrFinish` (`script.go:125-129`),
not from `resumeOrFinishWorld` (`script.go:176-178`) — this is correct per TS.

---

## 3. Remediation choice

**Fix B is more TS-faithful.** Null `p.activeScript` at the `WorldSuspended` transition in
`resumeOrFinish` (`modules/world/script.go:132-146`), retiring the NAI-44 divergence.

**Rationale:**

Fix A (gate `protectedScriptActive()` on `Execution != WorldSuspended`) is a symptom patch. It
papers over the false positive without restoring the structural invariant. It also requires
updating every `protectedScriptActive()` call site and would leave `p.activeScript` pointing at
a world-owned state, which is a latent bug waiting for any future caller that reads
`p.activeScript` directly.

Fix B re-establishes the TS invariant: `p.activeScript` is nil during the world-queue window.
The NAI-44 rationale was "safe because resume gates on `Execution == Suspended`"
(`script.go:143-144`). This is correct — `processActiveScripts` (`tick.go:281-285`) gates resume
on `p.activeScript.Execution == script.Suspended`. A nil `p.activeScript` would simply skip that
check (`p.activeScript != nil` is the first guard at `tick.go:281`). There is no resume-re-fire
risk because the resume loop is doubly gated: non-nil pointer AND `Execution == Suspended`.

Fix B also eliminates the concern about `p.activeScript` pointing at a Finished state between
world-queue completion and the next `runScript` call (task 7 below).

TS structural proof: TS canAccess checks `this.protect` (boolean), not `this.activeScript`.
During a world-queue wait, TS `this.protect` is left unchanged from whatever it was before the
`WORLD_DELAY` instruction, which is typically `false` for protect-off runs. For a protect=true
run, `this.protect` would remain true — but TS scripts on Tutorial Island use `WORLD_DELAY` from
non-protected contexts (the queue-fired entry point had `protect=false` or the protect was already
cleared). The real invariant in TS: the world-queue wait window is NOT a player-protection window.
goscape's NAI-44 divergence incorrectly made it one.

---

## 4. Patch diff

File: `modules/world/script.go`, lines 132-146.

**Current:**
```go
	case script.WorldSuspended:
		// NAI-37 T10: player-bound script suspended to world queue.
		// Pop the wakeup-tick (which the script's bytecode pushed
		// before WORLD_DELAY — see handlers_server.go:87-108) and
		// enqueue. The player no longer owns this script's execution;
		// it now belongs to the world queue. Mirrors TS Player.ts:2135-2136.
		//
		// NAI-44: TS Player.executeScript (L2143-2150) only nulls
		// this.activeScript on FINISHED/ABORTED. Goscape's previous
		// defensive clear here was untracked-divergent; processActiveScripts
		// gates resume on Execution == Suspended (tick.go:213-214), so
		// holding the pointer is safe — the player's resume loop will
		// not re-fire a WorldSuspended state.
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
```

**Replace with:**
```go
	case script.WorldSuspended:
		// NAI-37 T10 / NAI-155: player-bound script suspended to world
		// queue. Pop the wakeup-tick (pushed before WORLD_DELAY — see
		// handlers_server.go:87-108) and enqueue.
		//
		// Clear p.activeScript (via self.ClearActiveScript) BEFORE
		// enqueue. TS Player.executeScript:2135-2136 does NOT assign
		// script.activePlayer.activeScript in the WORLD_SUSPENDED arm —
		// neither does it set this.protect — so the player's protect
		// boolean remains false during the world-queue wait window.
		// Goscape's NAI-44 divergence (hold the pointer for "safe
		// resume") was incorrect: it made protectedScriptActive() return
		// true for the entire wait window, blocking CanAccess and all
		// interactions. The resume gate (tick.go:281) is doubly guarded
		// (non-nil AND Execution==Suspended), so a nil activeScript
		// produces no false-resume. Retiring NAI-44-D-WORLDSUSPENDED-HOLD.
		self.ClearActiveScript()
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
```

No other files need editing for the core fix. The comment in `player_script.go:337-348`
(protectedScriptActive doc) should note that WorldSuspended states are excluded by construction
(activeScript is nil during world-queue windows), but that is a doc-only follow-up.

---

## 5. Test pins

All new tests belong in `modules/world/player_script_test.go` or a new
`modules/world/worldsuspended_pap_test.go`.

**T1 — protectedScriptActive returns false when activeScript is nil (regression guard)**
- Setup: `p.activeScript = nil`
- Assert: `p.protectedScriptActive() == false`
- (Already implicitly tested by existing `player_test.go:736`, but add an explicit WorldSuspended
  label.)

**T2 — CanAccess returns true after WORLD_SUSPENDED transition**
- Setup: build a `*ScriptState` with `PtrProtectedActivePlayer` set and
  `Execution = script.WorldSuspended`.
- Simulate: call `p.ClearActiveScript()` (the fix's effect).
- Assert: `p.CanAccess() == true` (no delay, no modal, activeScript nil).

**T3 — processWalktrigger is NOT blocked after WORLD_SUSPENDED transition**
- Variation of T2 for the walktrigger gate (`interaction.go:340`).

**T4 — Resume loop does NOT re-fire a nil activeScript**
- Setup: `p.activeScript = nil`, `p.delayed = false`.
- Simulate one `processActiveScripts` call (or its equivalent logic).
- Assert: no panic; `p.activeScript` remains nil; `resumeOrFinish` is NOT called.
- Pins the "resume re-fire" concern from NAI-44.

**T5 — Resume loop correctly resumes a Suspended (non-WorldSuspended) script unchanged**
- Regression pin to confirm that valid Suspended scripts still resume after the fix.
- Setup: `p.activeScript = state` with `Execution = script.Suspended`, `p.delayed = false`.
- Assert: `resumeOrFinish` is called (via mock or observable side-effect).

---

## 6. Risk audit

**Callers of `protectedScriptActive()`** (`rg -n "protectedScriptActive\(\)" modules/world/`):

| Location | Role |
|---|---|
| `player_script.go:331` | `CanAccess()` — third gate |
| `interaction.go:340` | `processWalktrigger` guard |
| `interaction.go:785` | `processInteraction` post-step guard |
| `interaction_debug.go:71` | probe field (read-only diagnostic) |
| `player_test.go:736` | test harness |
| `modal_close_test.go:291-292` | test — CloseModal must not strip in-flight protect |

Only the first three are production gates. The diagnostic use at `interaction_debug.go:71` will
correctly show `protected_active=false` during world-queue windows after the fix — this is the
correct observable value and what the smoke probe should show going forward.

**What the fix unblocks (correctly):**
- All interactions during the world-queue wait window are unblocked: talk to NPC, use object,
  player→player interactions, anything gated by `CanAccess()` or `processWalktrigger`.
- This matches TS: `this.protect` is false during a world-queue wait for non-protected fire paths.

**What the fix must NOT break:**
- A script that is Suspended (player-paused, e.g. `P_PAUSEBUTTON`/`P_COUNTDIALOG`) with PAP set
  must still block CanAccess. This is unaffected: those states remain in the `script.Suspended`
  arm of `resumeOrFinish`, which calls `self.StoreActiveScript(state)`. `p.activeScript` is still
  set for those states.
- The `modal_close_test.go:276-292` case (CloseModal must not strip in-flight protect for Suspended
  scripts) is also unaffected — that case has `Execution = Suspended`, not `WorldSuspended`.

**Behaviour unchanged:**
- Suspended (P_DELAY player-side scripts): stored, resumed correctly.
- Finished/Aborted: `OnScriptFinishedOrAborted` path unchanged.
- CountDialog/PauseButton: stored, cleared by CloseModal unchanged.
- NpcSuspended: unchanged.

---

## 7. `resumeOrFinishWorld` Finished arm — separate fix needed?

`modules/world/script.go:176-178`: the Finished/Aborted arm is a silent no-op. After the world
queue processes a Finished script, `state.Self.activeScript` still points at that Finished state
(if the player's `activeScript` had been set by a prior SUSPENDED→StoreActiveScript transition
within the same world-queue run — the `Suspended` arm at L182-192 calls `StoreActiveScript`).

However, this is NOT a new bug introduced by fix B. Under fix B:

1. `resumeOrFinish` (player path) nulls `p.activeScript` at `WorldSuspended` (the fix).
2. The world-queue run of the same state proceeds via `resumeOrFinishWorld`.
3. If that world-queue run finishes (`Finished`/`Aborted`), `resumeOrFinishWorld:176-178` does
   nothing. But `p.activeScript` was already nulled in step 1 — so it is nil at this point.

The only risky scenario is the `Suspended` arm at `resumeOrFinishWorld:182-192`: if the
world-queue run hits `P_DELAY` again (player-side suspend from within world-queue context), it
calls `state.Self.StoreActiveScript(state)`, setting `p.activeScript` to that state. Then the
next tick's `processActiveScripts` would resume it. If that subsequent resume Finishes, it goes
through `resumeOrFinish`'s Finished arm → `OnScriptFinishedOrAborted` → null. No residue.

**Conclusion:** The `resumeOrFinishWorld` Finished arm does not need a separate fix. Under fix B,
`p.activeScript` is nil before the world-queue run begins (cleared at WorldSuspended). The only
way it gets set again is via the `Suspended` arm, which is handled correctly downstream. No
Finished-arm residue is possible.

---

## Citations

- `modules/world/script.go:132-146` — NAI-44 WorldSuspended arm (site of fix)
- `modules/world/script.go:170-211` — `resumeOrFinishWorld`
- `modules/world/player_script.go:324-335` — `CanAccess`
- `modules/world/player_script.go:346-347` — `protectedScriptActive`
- `modules/world/player_script.go:144-148` — `StoreActiveScript`
- `modules/world/player_script.go:150-154` — `ClearActiveScript`
- `modules/world/player_script.go:170-178` — `OnScriptFinishedOrAborted`
- `modules/world/interaction.go:339-340` — `processWalktrigger` PAP gate
- `modules/world/interaction.go:785` — `processInteraction` PAP gate
- `modules/world/tick.go:281-286` — `processActiveScripts` resume gate (nil+Suspended double gate)
- `pkg/script/execution.go:16` — `WorldSuspended = 7`
- `LostCityRS/Engine-TS/src/engine/entity/Player.ts:2134-2150` — `executeScript` state dispatch
- `LostCityRS/Engine-TS/src/engine/entity/Player.ts:741-747` — `closeModal` protect clear
- `LostCityRS/Engine-TS/src/engine/entity/Player.ts:805-812` — `canAccess`
- `LostCityRS/Engine-TS/src/engine/World.ts:530-560` — `processWorld` world-queue loop
