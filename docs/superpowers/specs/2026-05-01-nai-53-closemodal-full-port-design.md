# NAI-53 — Full CloseModal port

**Date:** 2026-05-01
**Tech stack:** Go 1.26+
**Closes follow-ups:** `NAI-52-F1`

## Problem

`(*Player).CloseModal` at `modules/world/player_script.go:573-579` is a flat slot
reset — it zeroes `modalMain`/`modalChat`/`modalSide`, sets
`modalState = modalStateNone`, and flags `refreshModalClose`. TS
`Player.closeModal` (`Player.ts:741-794`) does substantially more:

- Optionally clears the weak queue (`weakQueue.clear()`, gated on
  `clearWeakQueue` parameter, default `true`).
- Clears the player's protect flag when not delayed
  (`if (!this.delayed) this.protect = false`).
- Early-returns when `modalState === ModalState.NONE`.
- Sets `modalState = ModalState.NONE`.
- Nulls `activeScript` if its execution is `COUNTDIALOG` or `PAUSEBUTTON`
  (this is `NAI-52-F1`).
- For each open slot (Main → Chat → Side), looks up an `IF_CLOSE`
  trigger script keyed on the slot's com ID, executes it via
  `executeScript(ScriptRunner.init(closeTrigger, this), false)`, then
  calls `clearComListeners(slotCom)`, then resets the slot to `-1`.

The current goscape body skips all of this. Visible behavioral consequences:

- Closing a chatbox dialog (`CLOSE_MODAL` packet, STRONG-queue trigger,
  `ClearPendingAction`, or any `IF_CLOSE` opcode) does NOT cancel a
  suspended `COUNTDIALOG` / `PAUSEBUTTON` script; the player remains
  pinned in the protected state per `(*Player).protectedScriptActive`'s
  lens (`NAI-52-F1`).
- Per-slot `IF_CLOSE` trigger RuneScripts never fire, so content authors
  cannot react to modal closure.
- Weak-queue entries persist across modal close — TS clears them
  proactively to prevent stale weak-queued scripts after a UI flow ends.
- `protect` is not cleared on a non-delayed CloseModal, so an existing
  `activeScript` keeps its `Protect=true` state after the player closes
  a modal that wasn't a `COUNTDIALOG` / `PAUSEBUTTON` suspend.

Port the full TS body, modulo the one structural piece goscape lacks
(per-slot `Component.rootLayer` lookup), tracked as a single new
deviation.

## Scope

Single bundle, 5 tasks. CloseModal's body and signature both change;
all 3 existing callers update to pass `clearWeakQueue=true` (the TS
default).

### In scope

- New helper `(*Player).clearWeakQueue()` that drops `script.QueueWeak`
  entries from `p.queue`, preserving relative order of remaining
  entries.
- Signature change: `CloseModal()` → `CloseModal(clearWeakQueue bool)`.
  3 callers updated to pass `true`.
- Full TS body port:
  1. Conditional `clearWeakQueue` invocation.
  2. `if !p.delayed && p.activeScript != nil { p.activeScript.Protect = false }`
     — applies the NAI-52 convergence (TS `this.protect` ↔ goscape
     `activeScript.Protect`). When `activeScript == nil`, protect is
     already implicitly `false` so nothing to clear.
  3. Early-return on `p.modalState == modalStateNone`.
  4. `p.modalState = modalStateNone`.
  5. COUNTDIALOG/PAUSEBUTTON → `p.activeScript = nil` (closes
     `NAI-52-F1`).
  6. Per-slot dispatch (Main → Chat → Side, in TS order):
     - `s.scriptProvider.GetByTriggerSpecific(script.TriggerIfClose, slotCom, -1)`.
     - If non-nil and `p.client.server` reachable: `s.runScript(sf, p, nil, false, nil, nil)`.
     - Slot reset to `-1`.
- Preserve existing post-condition: `refreshModalClose = true` set
  unconditionally at the end (so the wire-IF_CLOSE still fires).

### Out of scope (explicitly deferred)

- **Per-slot `clearComListeners(root)` faithful to TS**: requires
  unported Component config registry with `rootLayer` field. Goscape's
  `encodeOut` already clears all `invListeners` globally when
  `refreshModalClose` is set (see `modal_close_test.go:30`); the new
  CloseModal body relies on that global clear. Tracked as
  `NAI-53-D-CLEARCOMLISTENERS-PER-SLOT`.
- **Wiring `CloseModal(false)` into `(*Server).resumeOrFinish`** — TS
  `Player.ts:2148` invokes `closeModal(false)` after a script suspends
  with `(modalState & MAIN) == NONE`. Goscape's `resumeOrFinish`
  doesn't call CloseModal at all in that path. NAI-53 adds the `false`
  arg to the API surface but does not wire the new caller. Tracked as
  follow-up `NAI-53-F1`, not a formal deviation (pre-existing gap).
- **No reshape of unrelated CloseModal-adjacent code**: `OpenMain`,
  `OpenChat`, `OpenSide`, `OpenMainSide` (player_script.go:583-619) are
  out of scope.

## Goals

1. Close `NAI-52-F1` (CloseModal nulls `activeScript` on
   COUNTDIALOG / PAUSEBUTTON).
2. Land per-slot `IF_CLOSE` trigger-script dispatch faithful to TS
   order (Main → Chat → Side) and lookup key (`TriggerIfClose` keyed on
   slot com ID).
3. Apply NAI-52 convergence to the `!delayed → protect=false` rule.
4. Add `clearWeakQueue` parameter without breaking existing 3 callers.
5. Land exactly one new tracked deviation
   (`NAI-53-D-CLEARCOMLISTENERS-PER-SLOT`).

## Non-goals

- Component-config / rootLayer port.
- Wiring the `CloseModal(false)` caller from `resumeOrFinish`.
- Any change to `invListeners` storage or the `encodeOut` global-clear
  path.
- Renames or refactors of unrelated modal helpers.

## TS reference

`Engine-TS/src/engine/entity/Player.ts:741-794`:

```ts
closeModal(clearWeakQueue: boolean = true) {
    if (clearWeakQueue) {
        this.weakQueue.clear();
    }
    if (!this.delayed) {
        this.protect = false;
    }

    if (this.modalState === ModalState.NONE) {
        return;
    }

    this.modalState = ModalState.NONE;

    // close any input dialogue suspended scripts.
    if (this.activeScript?.execution === ScriptState.COUNTDIALOG || this.activeScript?.execution === ScriptState.PAUSEBUTTON) {
        this.activeScript = null;
    }

    // close any main viewport interface
    if (this.modalMain !== -1) {
        const closeTrigger = ScriptProvider.getByTrigger(ServerTriggerType.IF_CLOSE, this.modalMain);
        if (closeTrigger) {
            this.executeScript(ScriptRunner.init(closeTrigger, this), false);
        }

        this.clearComListeners(this.modalMain);
        this.modalMain = -1;
    }

    // close any chatbox interface
    if (this.modalChat !== -1) {
        const closeTrigger = ScriptProvider.getByTrigger(ServerTriggerType.IF_CLOSE, this.modalChat);
        if (closeTrigger) {
            this.executeScript(ScriptRunner.init(closeTrigger, this), false);
        }

        this.clearComListeners(this.modalChat);
        this.modalChat = -1;
    }

    // close any sidebar tabs interface
    if (this.modalSide !== -1) {
        const closeTrigger = ScriptProvider.getByTrigger(ServerTriggerType.IF_CLOSE, this.modalSide);
        if (closeTrigger) {
            this.executeScript(ScriptRunner.init(closeTrigger, this), false);
        }

        this.clearComListeners(this.modalSide);
        this.modalSide = -1;
    }
}
```

## Architecture

### Helper

```go
// clearWeakQueue removes every QueueWeak entry from p.queue,
// preserving relative order of the remaining entries. Mirrors TS
// Player.weakQueue.clear() (the weak queue is unified into p.queue
// with a Type discriminator at this engine level).
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

### CloseModal body

```go
// CloseModal mirrors TS Player.closeModal (Player.ts:741-794). When
// clearWeakQueue is true (TS default), drops every QueueWeak entry
// from p.queue. Clears activeScript.Protect when not delayed (TS
// `this.protect = false`, applied via the NAI-52 convergence).
// Early-returns if no modal is currently open. Otherwise, nulls
// activeScript on COUNTDIALOG/PAUSEBUTTON, dispatches a per-slot
// IF_CLOSE trigger script (Main → Chat → Side, TS order), and
// resets all slots to -1.
//
// DEVIATION NAI-53-D-CLEARCOMLISTENERS-PER-SLOT: TS calls
// clearComListeners(slotCom) per-slot using Component.rootLayer
// to filter invListeners. Goscape clears all invListeners globally
// in encodeOut when refreshModalClose is set; the unported
// Component-config rootLayer registry blocks per-slot filtering.
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

    if p.activeScript != nil &&
        (p.activeScript.Execution == script.CountDialog ||
            p.activeScript.Execution == script.PauseButton) {
        p.activeScript = nil
    }

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
        // No server (test path with mock Player) — still reset slots.
        p.modalMain = -1
        p.modalChat = -1
        p.modalSide = -1
    }

    p.refreshModalClose = true
}

// runIfCloseTrigger looks up TriggerIfClose for slotCom and runs it
// if found. Mirrors TS executeScript(ScriptRunner.init(closeTrigger,
// this), false) per slot. Nil-safe on scriptProvider.
func (p *Player) runIfCloseTrigger(s *Server, slotCom int) {
    if s.scriptProvider == nil {
        return
    }
    sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerIfClose, slotCom, -1)
    if sf == nil {
        return
    }
    s.runScript(sf, p, nil, false, nil, nil)
}
```

**`refreshModalClose` placement note:** The current body sets it
unconditionally. The TS body has no analogue (TS uses a different
wire-emission path). NAI-53 sets `refreshModalClose = true` at the
end of the new body — but the modalState-NONE early-return path
leaves `refreshModalClose` untouched.

**Observable wire-behavior delta:** before NAI-53, every `CloseModal`
call sets `refreshModalClose=true`, so a redundant CloseModal (when
no modal is open) emits IF_CLOSE on the next encodeOut. After NAI-53,
the early-return path skips that wire-emit. This is more faithful to
TS (which early-returns on NONE without writing anything) and reduces
unnecessary client chatter. Pin this in test coverage: a CloseModal
call with `modalState=modalStateNone` and `refreshModalClose=false`
preserves both fields.

### Caller updates

Three call sites to update — all pass `true` (TS default):

| Site | Current | After |
|---|---|---|
| `pkg/script/handlers_interface.go:19` (handleIfClose) | `s.Self.CloseModal()` | `s.Self.CloseModal(true)` |
| `modules/world/player_script.go:652` (ClearPendingAction) | `p.CloseModal()` | `p.CloseModal(true)` |
| `modules/world/tick.go:245` (refreshModalClose path) | `p.CloseModal()` | `p.CloseModal(true)` |

The `script.ActivePlayer` interface (`pkg/script/active.go:131-133`)
must update its method signature: `CloseModal()` → `CloseModal(clearWeakQueue bool)`.
Mock player in `pkg/script/runner_test.go:408` updates accordingly.

## Test strategy

All new tests live in `modules/world/modal_close_test.go` (extend),
`modules/world/player_script_test.go` (extend for `clearWeakQueue`
helper), and `pkg/script/handlers_interface_test.go` (existing
test updates for new signature).

### `clearWeakQueue` helper (T1, `player_script_test.go`)

- Empty queue → empty queue (no panic).
- Queue with one Weak entry → empty queue.
- Queue with one Strong entry → unchanged.
- Queue with `[Strong, Weak, Normal, Weak, Long]` → `[Strong, Normal, Long]`
  (relative order preserved).
- Queue with all Weak entries → empty queue.
- Repeat clearWeakQueue is idempotent.

### Signature update (T2)

- All existing CloseModal tests pass with `true` arg threaded through.
- `script.ActivePlayer` interface satisfaction recompiles cleanly.

### Protect-clear block (T3, `modal_close_test.go`)

- `delayed=true, activeScript={Protect=true}` → `Protect=true` preserved.
- `delayed=false, activeScript=nil` → no panic, no state change.
- `delayed=false, activeScript={Protect=true}` → `Protect=false` after.
- `delayed=false, activeScript={Protect=false}` → `Protect=false` (no-op).
- After T3, `(*Player).protectedScriptActive()` returns `false` for the
  cleared cases (NAI-52 convergence pin).

### NONE early-return + slot reset (T4, `modal_close_test.go`)

- `modalState=modalStateNone, modalMain=-1` → no field changes,
  `refreshModalClose` unchanged.
- `modalState=modalStateMain, modalMain=42` (no IF_CLOSE script
  registered) → `modalMain=-1`, `modalState=modalStateNone`,
  `refreshModalClose=true`.
- All-three-slots-set, no IF_CLOSE registered → all slots reset to
  `-1`, `modalState=modalStateNone`, `refreshModalClose=true`.

### activeScript null + IF_CLOSE dispatch (T5, `modal_close_test.go`)

- COUNTDIALOG: pre-condition `activeScript={Execution=CountDialog}` +
  modal open → post: `activeScript=nil`.
- PAUSEBUTTON: same with `Execution=PauseButton` → post: `activeScript=nil`.
- Suspended: `activeScript={Execution=Suspended}` + modal open → post:
  `activeScript` preserved (NOT nulled).
- Nil activeScript + modal open → no panic, slot reset proceeds.
- Per-slot IF_CLOSE dispatch: register a TriggerIfClose script for com
  `42`, set `modalMain=42`, call CloseModal — verify `runScript` was
  invoked with the registered script. Repeat for Chat and Side.
- Dispatch order pin: register IF_CLOSE for Main(7) + Chat(8) + Side(9)
  recording invocation order; verify Main fired first, Chat second,
  Side third (TS order).
- Nil scriptProvider → no panic, slot reset still proceeds.
- Nil `p.client.server` → no panic, slot reset still proceeds (test
  path).

## Deviations introduced

### `NAI-53-D-CLEARCOMLISTENERS-PER-SLOT`

**Where:** `modules/world/player_script.go::(*Player).CloseModal`.

**TS:** `Player.ts:767, 778, 789` calls `clearComListeners(slotCom)`
per-slot, which iterates `invListeners` and removes those whose
`Component.get(com).rootLayer === slotCom`.

**Goscape:** No per-slot call. Goscape's `encodeOut` clears ALL
`invListeners` globally when `refreshModalClose` is set
(`modal_close_test.go:30` pins this behavior).

**Why:** Goscape has no Component config registry — `Component.get`
and `rootLayer` are unported. Per-slot rootLayer-filtered clearing
requires the registry. The global clear in `encodeOut` is broader (it
removes listeners for slots that weren't open) but doesn't leak stale
listeners and is safe.

**Closure:** future Component-config sub-spec (will add the registry,
add `(*Player).clearComListeners(root int)` per-slot, and remove the
global `encodeOut` clear).

**Tag:** add `// DEVIATION NAI-53-D-CLEARCOMLISTENERS-PER-SLOT` comment
at the top of the slot-dispatch block in CloseModal.

## Deviations retired

None. NAI-52-F1 is a follow-up entry, not a tracked deviation.

## Net deviation tally

20 (post-NAI-52) → 21 (introduce
`NAI-53-D-CLEARCOMLISTENERS-PER-SLOT`).

Net change: **+1**.

## Bundle ordering

Single bundle, 5 tasks ordered T1 → T5. Per `runescript_cadence.md`:
formal whole-impl review at bundle close (not compressed cadence — the
production delta exceeds the ~15 LOC threshold and the IF_CLOSE
dispatch path is semantically novel).

## Open audit items for plan-write phase

Per `controller_preflight.md` and `spec_followup_tracker_freshness.md`,
plan-author should re-grep + Read at HEAD:

1. **Caller list freshness**: re-grep `\.CloseModal()` and `\.CloseModal\b`
   across `pkg/` and `modules/` to confirm exactly 3 callers (the spec
   names `handlers_interface.go:19`, `player_script.go:652`,
   `tick.go:245`). Any new caller in NAI-50/51/52 must be added.
2. **`script.ActivePlayer` interface signature**: re-Read
   `pkg/script/active.go:131-133` to confirm the method name, comment
   wording, and any peers that must update in lockstep
   (`runner_test.go:408` mock).
3. **`scriptProvider.GetByTriggerSpecific` signature** at HEAD: confirm
   the (trigger, type, category) arity via `pkg/script/provider.go`;
   plan code blocks should match exactly.
4. **`p.queue` slice type**: re-Read `playerQueueRequest` definition
   (`modules/world/player_script.go:~30`) to confirm field name `Type`
   and the `script.QueueWeak` constant; plan code block for
   `clearWeakQueue` helper depends on both.
5. **`(*Server).runScript` signature**: re-Read
   `modules/world/script.go:86` for argument order
   `(sf, p, npc, isFresh, intArgs, stringArgs)` and zero-arg form to
   confirm `s.runScript(sf, p, nil, false, nil, nil)` is correct.
6. **`refreshModalClose` semantics**: re-Read `tick.go:243-246` to
   verify the deferred-clear pattern still uses
   `requestModalClose → CloseModal` and that `refreshModalClose` is
   the wire-flag, not the deferred-action flag (these are distinct in
   goscape's modal lifecycle).
7. **`activeScript.Execution` enum names**: confirm `CountDialog` and
   `PauseButton` constants in `pkg/script/execution.go` at HEAD (no
   rename to e.g. `ExecutionCountDialog` since spec write).

## Follow-ups

- **NAI-53-F1** — `(*Server).resumeOrFinish` (`modules/world/script.go:100+`)
  does NOT invoke `CloseModal(false)` on Suspended/non-MAIN-modal
  completion; TS `Player.ts:2148` does. NAI-53 adds the `false` arg
  support to the API but the new caller is unwired. **Closure:**
  future executeScript-completion sub-spec.
