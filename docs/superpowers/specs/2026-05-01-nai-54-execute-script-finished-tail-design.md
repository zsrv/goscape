# NAI-54 — executeScript Finished/Aborted tail port

**Date:** 2026-05-01
**Tech stack:** Go 1.26+
**Closes follow-ups:** `NAI-53-F1`, `NAI-53-F2`

## Problem

The post-Execute Finished/Aborted tail of TS
`Player.executeScript` (`Player.ts:2143-2148`) and
`Npc.executeScript` (`Npc.ts:226-228`) is partially ported in goscape's
`(*Server).resumeOrFinish` (`modules/world/script.go:113-114`) and
`(*Server).resumeOrFinishNpc` (`modules/world/npc_script.go:304-305`).

**TS Player.ts:2143-2148:**
```ts
} else if (script === this.activeScript) {
    this.activeScript = null;

    if ((this.modalState & ModalState.MAIN) === ModalState.NONE) {
        // close chat dialogues automatically and leave main modals alone
        this.closeModal(false);
    }
}
```

**TS Npc.ts:226-228:**
```ts
} else if (script === this.activeScript) {
    this.activeScript = null;
}
```

**Current goscape player path (script.go:113-114):**
```go
case script.Finished, script.Aborted:
    self.ClearActiveScript()
```

**Current goscape npc path (npc_script.go:304-305):**
```go
case script.Finished, script.Aborted:
    npc.ClearActiveScript()
```

Both paths drop the `script === this.activeScript` identity guard and
the player path additionally drops the gated `closeModal(false)` call.

### Wire-observable consequences

1. **Player Suspended-clobber:** A fresh script Y fired on a Player who
   already holds a Suspended / PauseButton / CountDialog `activeScript` X
   (e.g. via timer, queue, or button handler) will, on Y's
   Finished/Aborted, unconditionally null `p.activeScript`. X is wiped.
   TS preserves X because `Y !== this.activeScript`.

2. **Player chat-dialogue lingering:** When a fresh chat-dialogue script
   Finished/Aborted while no MAIN modal is open, TS auto-closes the
   chat dialogue (firing per-slot `IF_CLOSE` triggers via
   `closeModal(false)`). Goscape leaves the chat dialogue open until
   the next explicit `CLOSE_MODAL` packet or `IF_CLOSE` opcode. **This
   is `NAI-53-F1`** — NAI-53 added the `clearWeakQueue` parameter to
   the `CloseModal` API but no caller wires the `false`-arg path.

3. **Npc Suspended-clobber:** Symmetric to (1). A fresh NPC script
   Finished/Aborted will null an unrelated NpcSuspended-stored
   activeScript on the same NPC.

### Test-coverage gap (NAI-53-F2)

The NAI-53 T5 close noted that no test exercises the combined
COUNTDIALOG/PAUSEBUTTON-null branch *and* the per-slot `IF_CLOSE`
dispatch in the same fixture: the T5 null-tests use `newTestPlayer`
without a server (so per-slot dispatch is a no-op), and the dispatch
tests use a fresh `ScriptState` left at the zero-value `Running`
execution (so the null branch never fires). The two branches are
independently green; their interaction is unpinned.

## Scope

Single sub-spec, single bundle, full cadence per
`runescript_cadence.md`. Estimated ~30 production LOC + ~80 test LOC.

**In scope:**

- Add `OnScriptFinishedOrAborted(state *ScriptState)` to `script.ActivePlayer`
  and `script.ActiveNpc` interfaces (`pkg/script/active.go`).
- Implement on `*Player` (`modules/world/player_script.go`) and `*Npc`
  (`modules/world/npc.go`) with full TS-faithful tail logic.
- Swap the Finished/Aborted-arm `ClearActiveScript()` call in
  `resumeOrFinish` and `resumeOrFinishNpc` to the new method.
- Update every test mock that implements `ActivePlayer` or `ActiveNpc`
  with a stub `OnScriptFinishedOrAborted` (per `enumerate_all_sites.md`).
- Pin both the guard-passes-and-clears branch AND the guard-mismatch-and-preserves
  branch with unit tests (`ts_asymmetry_dual_pin.md`).
- Pin the player-path `(modalState & MAIN) == NONE` modal-close-fired
  branch AND the MAIN-set modal-close-skipped branch.
- Integration tests via `resumeOrFinish` and `resumeOrFinishNpc` to pin
  the wire-observable Suspended-preservation behavior end-to-end.
- `NAI-53-F2` combined-fixture test: a single `*_test.go` fixture that
  exercises CloseModal's PauseButton-state-null branch AND a per-slot
  `IF_CLOSE` dispatch in the same call (`modal_close_test.go`).

**Out of scope:**

- `NAI-53-D-CLEARCOMLISTENERS-PER-SLOT` — still blocks on unported
  Component config registry. No new deviation, no work in this sub-spec.
- Reshape of any other `ClearActiveScript()` call site
  (Execute-error arms at `script.go:109` / `npc_script.go:300`,
  default warn-clear arms at `script.go:136` / `npc_script.go:323`,
  logout cleanup, `pkg/script/handlers_npc.go:767`,
  `pkg/script/npc_iterator.go:26`). All TS-faithful as-is.
- The `unsetMapFlag()`-style processInteraction tail polish
  (different surface; covered by NAI-44 polish commit `c22449d`).
- Any ScriptRunner / ScriptState reshape.

## Design

### Interface additions

**`pkg/script/active.go` — `ActivePlayer` (after `ClearActiveScript`):**

```go
// OnScriptFinishedOrAborted is the post-Execute tail for the Finished
// or Aborted execution states. If state matches the player's currently
// stored activeScript, nulls activeScript; additionally calls
// CloseModal(false) when no MAIN modal is open. Mirrors TS
// Player.executeScript tail (Player.ts:2143-2148). Player-only modal
// clause; the symmetric ActiveNpc method has no modal handling.
OnScriptFinishedOrAborted(state *ScriptState)
```

**`pkg/script/active.go` — `ActiveNpc` (after `ClearActiveScript`):**

```go
// OnScriptFinishedOrAborted is the post-Execute tail for the Finished
// or Aborted execution states. Nulls activeScript only if state matches
// the npc's currently stored value. Mirrors TS Npc.executeScript tail
// (Npc.ts:226-228). NPCs have no modals.
OnScriptFinishedOrAborted(state *ScriptState)
```

### Player implementation

**`modules/world/player_script.go`:**

```go
// OnScriptFinishedOrAborted handles the Finished/Aborted post-Execute
// tail for a player-anchored script. If state is the player's current
// activeScript, nulls it; and if no MAIN modal is open, fires
// CloseModal(false) to auto-close any open chat / side dialogue.
//
// Mirrors TS Player.executeScript Finished/Aborted tail
// (Player.ts:2143-2148). The match-guard preserves a Suspended /
// PauseButton / CountDialog activeScript when a different fresh
// script Finishes on the same player in the same tick. The
// MAIN-bit gate on CloseModal preserves any open main modal while
// dropping chat / side dialogues — TS comment: "close chat
// dialogues automatically and leave main modals alone".
func (p *Player) OnScriptFinishedOrAborted(state *script.ScriptState) {
    if p.activeScript != state {
        return
    }
    p.activeScript = nil
    if p.modalState&modalStateMain == modalStateNone {
        p.CloseModal(false)
    }
}
```

`modalStateNone == 0x0`, so `p.modalState&modalStateMain == modalStateNone`
is bitwise-identity-with-zero. Form chosen for visual parity with TS
`(this.modalState & ModalState.MAIN) === ModalState.NONE`.

### Npc implementation

**`modules/world/npc.go`:**

```go
// OnScriptFinishedOrAborted handles the Finished/Aborted post-Execute
// tail for an npc-anchored script. If state matches the npc's
// activeScript, nulls it; otherwise no-op. Mirrors TS
// Npc.executeScript tail (Npc.ts:226-228). The match-guard preserves
// an NpcSuspended-stored activeScript when a different fresh script
// Finishes on the same npc in the same tick.
func (n *Npc) OnScriptFinishedOrAborted(state *script.ScriptState) {
    if n.activeScript != state {
        return
    }
    n.activeScript = nil
}
```

### Call-site swaps

**`modules/world/script.go:113-114`:**
```go
// before:
case script.Finished, script.Aborted:
    self.ClearActiveScript()

// after:
case script.Finished, script.Aborted:
    self.OnScriptFinishedOrAborted(state)
```

**`modules/world/npc_script.go:304-305`:**
```go
// before:
case script.Finished, script.Aborted:
    npc.ClearActiveScript()

// after:
case script.Finished, script.Aborted:
    npc.OnScriptFinishedOrAborted(state)
```

### Mocks (interface-change blast radius)

The plan must enumerate every concrete impl of `ActivePlayer` and
`ActiveNpc` and add `OnScriptFinishedOrAborted` to each. Per
`enumerate_all_sites.md`, the controller pre-flight grep target is:

```
rg -n 'ActivePlayer\b' modules/ pkg/
rg -n 'ActiveNpc\b' modules/ pkg/
```

Plan-author MUST list every match before dispatch (production +
test mocks). Per `mock_recorder_field_naming_check.md`, plan-author
MUST Read each mock's struct fields before authoring impl bodies (no
inferred field names).

## Edge cases & invariants

1. **Fresh-fire-completes while activeScript is nil.** Common case.
   `p.activeScript != state` (nil vs new state pointer), guard returns
   early. CloseModal NOT fired. TS-faithful: `script === null` is `false`.

2. **Fresh-fire-completes while activeScript holds X (Suspended /
   PauseButton / CountDialog).** Y just ran. `p.activeScript == X != Y`,
   guard returns early. X preserved. **Closes the silent Suspended-clobber
   bug.** Symmetric on Npc.

3. **Resume-completes (state IS the activeScript).** `state == p.activeScript`,
   guard passes. Player nulls activeScript and gates CloseModal on MAIN bit.
   Npc nulls activeScript.

4. **Resume-completes with MAIN modal open.** Guard passes, activeScript
   nulled, CloseModal NOT fired. Main modal preserved. TS L2146 gate.

5. **Resume-completes with chat-only modal (no MAIN).** Guard passes,
   activeScript nulled, `CloseModal(false)` fires:
   - `clearWeakQueue=false` skips `p.clearWeakQueue()` (TS L2148).
   - `!p.delayed && p.activeScript != nil` is now FALSE
     (just nulled), so the NAI-52 protect-clear no-ops. Matches TS:
     TS clears `this.activeScript` first, then calls
     `closeModal(false)`, where `script.activePlayer.protect = false`
     also no-ops on the now-detached reference.
   - `p.modalState != modalStateNone` (chat / side bits set), so
     CloseModal does NOT early-return at the `modalState == NONE` check.
   - Resets modalState to NONE, runs per-slot IF_CLOSE for Chat (and
     Side if open), sets `refreshModalClose`.

6. **CloseModal recursion safety.** CloseModal's `Execution == CountDialog
   || PauseButton` activeScript-null branch sees `p.activeScript == nil`
   (just cleared); no-ops. The `runIfCloseTrigger` per-slot dispatch
   re-enters `runScript` → `resumeOrFinish` for the IF_CLOSE script;
   that script's Finished/Aborted again calls
   `OnScriptFinishedOrAborted` but the IF_CLOSE state is NOT
   `p.activeScript` (which is nil), so guard returns early. No
   re-entry into CloseModal.

7. **Execute-error path unchanged.** `script.go:109` and `npc_script.go:300`
   continue to call `ClearActiveScript()` directly on Execute returning
   non-nil error. TS-faithful: a thrown ScriptRunner.execute error
   doesn't reach the Finished/Aborted tail.

8. **Default warn-clear arm unchanged.** `script.go:136` and
   `npc_script.go:323` continue to call `ClearActiveScript()` on
   unsupported / unexpected execution states. Defensive, fine.

## Wire-observable behavioral delta summary

| Pre-NAI-54 | Post-NAI-54 | TS-faithful? |
|---|---|---|
| Fresh fire-and-finish wipes existing Suspended `p.activeScript` | Preserved | ✓ closes silent bug |
| Chat dialogue lingers after script Finished (no MAIN modal) | Closes via per-slot IF_CLOSE | ✓ closes NAI-53-F1 |
| Chat dialogue persists when MAIN modal open | Same (preserved) | ✓ unchanged |
| Same-tick Npc fresh-fire wipes Suspended `n.activeScript` | Preserved | ✓ closes silent twin |

## Test strategy

### `*Player` matrix — `(*Player).OnScriptFinishedOrAborted`

Four cases (`ts_asymmetry_dual_pin.md` form — adjacent positive +
negative pins):

| Case | Setup | Expect |
|---|---|---|
| `match-no-MAIN` | `p.activeScript = X`; `modalState = chat`; `modalChat = 100` | `p.activeScript == nil`; `modalState == NONE`; `refreshModalClose == true`; `modalChat == -1` |
| `match-with-MAIN` | `p.activeScript = X`; `modalState = main\|chat`; `modalMain = 200`; `modalChat = 100` | `p.activeScript == nil`; `modalState == main\|chat` (UNCHANGED); `refreshModalClose == false`; slots unchanged |
| `mismatch` | `p.activeScript = X`; `modalState = chat`; call with `Y != X` | `p.activeScript == X` (preserved); `modalState == chat` (unchanged) |
| `nil-active` | `p.activeScript = nil`; `modalState = chat`; call with `Y` | `p.activeScript == nil`; `modalState == chat` (unchanged); no panic |

The `match-no-MAIN` case requires the player to have a `*Server` bound
so `runIfCloseTrigger` can dispatch (or stub the dispatch sentinel).
Plan-author: design fixture so dispatch is observable but no real
script runs (provider returns nil ScriptFile so `runIfCloseTrigger`'s
`s.scriptProvider.GetByTrigger` lookup miss path is exercised).

### `*Npc` matrix — `(*Npc).OnScriptFinishedOrAborted`

Two cases:

| Case | Setup | Expect |
|---|---|---|
| `match` | `n.activeScript = X`; call with `X` | `n.activeScript == nil` |
| `mismatch` | `n.activeScript = X`; call with `Y != X` | `n.activeScript == X` |

### Integration tests via `resumeOrFinish`

End-to-end pin of the **Suspended-preservation bug fix**:

- `script_test.go` — `TestResumeOrFinishPreservesUnrelatedSuspendedScript`:
  pre-store X (Suspended) on `p.activeScript`. Build fresh ScriptState
  Y bound to a no-op script that finishes immediately. Call
  `s.resumeOrFinish(Y, p)`. Assert `p.activeScript == X`.

- `npc_script_test.go` — `TestResumeOrFinishNpcPreservesUnrelatedSuspendedScript`:
  symmetric on `*Npc` via `s.resumeOrFinishNpc(Y, n)`.

### `*` NAI-53-F2 combined fixture — `modal_close_test.go`

`TestCloseModalCombinedPauseButtonNullAndPerSlotDispatch`:

- Build `*Player` with a real `*Server` bound.
- Stub `s.scriptProvider` to return a recorded sentinel `*ScriptFile`
  for `LookupTrigger(IfClose, slotCom)`.
- Set `p.activeScript = state` with `state.Execution = script.PauseButton`.
- Set `modalState = chat`, `modalChat = someComId`.
- Call `p.CloseModal(true)`.
- Assert all of:
  - `p.activeScript == nil` (NAI-53 T5 PauseButton-null branch fired).
  - The recorded sentinel ScriptFile was dispatched via `runScript`
    (per-slot IF_CLOSE branch fired).
  - `modalChat == -1`.
  - `refreshModalClose == true`.

Per `plan_runnable_test_fixtures.md`, plan-author must mentally
execute each fixture to verify it compiles and reaches the asserted
branches before dispatch.

## TDD task ordering

T1 — Author `*Player` matrix tests (RED: method does not exist).
T2 — Implement `(*Player).OnScriptFinishedOrAborted` (GREEN).
T3 — Author `*Npc` matrix tests (RED).
T4 — Implement `(*Npc).OnScriptFinishedOrAborted` (GREEN).
T5 — Add interface methods to `pkg/script/active.go`; update every
     `ActivePlayer` / `ActiveNpc` mock; swap call sites in
     `resumeOrFinish` and `resumeOrFinishNpc`; author integration
     tests for the Suspended-preservation flow. Build must remain
     GREEN throughout.
T6 — Author NAI-53-F2 combined-fixture test in `modal_close_test.go`.
     Independent of T5; can land any time after T2.
T7 — Close commit: `closes memory:` trailer per
     `close_commit_memory_trailer.md`; deviation tally update; no
     net deviation change (no new deviations opened, F1 + F2 closed
     as follow-ups).

## Deviations

**Opened:** none.

**Closed:** none.

**Net deviation tally:** 21 → 21 (no change).

## Memory entries

Apply at brainstorm / plan / impl time:

- `ts_source_canonical_path.md` — `LostCityRS/Engine-TS` only.
- `runescript_cadence.md` — full sub-spec cadence selected.
- `audit_full_method_against_ts.md` — both divergences in same TS
  block; port both, not just F1's named one.
- `enumerate_all_sites.md` — interface-change blast radius requires
  pre-flight grep of all mocks.
- `mock_recorder_field_naming_check.md` — plan-author Reads each
  mock's struct shape before authoring stub bodies.
- `ts_asymmetry_dual_pin.md` — adjacent positive + negative test
  cases for both the activeScript-match guard and the MAIN-bit
  modal-close gate.
- `plan_runnable_test_fixtures.md` — fixtures mentally executable
  before dispatch.
- `controller_preflight.md` — re-grep mocks at HEAD before each
  implementer dispatch.
- `close_commit_memory_trailer.md` — close commit trailer.
- `true_to_ts_gate.md` — every divergence tracked with rationale +
  follow-up; this sub-spec opens zero new deviations.

## Follow-up candidates after NAI-54 close

To be enumerated at close time. Anticipated none (this sub-spec is
itself a follow-up; the underlying TS divergences in the
`Player.executeScript` / `Npc.executeScript` Finished/Aborted tail are
fully ported and no upstream blockers remain on this surface).
