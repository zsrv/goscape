# Sub-spec RuneScript S6b: OPNPC Trigger Routing + NPC_SAY — Design

**Status:** Draft → ready for plan
**Scope:** Connect the player-clicks-NPC chain to script dispatch. When a player reaches interaction range of an anchored NPC target, fire the `[opnpc<N>,<npcType>]` script with `state.Self = player` and `state.ActiveNpc = npc`. Ships alongside `NPC_SAY` — the single most-used NPC mutating op — so the dispatch is demonstrable end-to-end.
**Out of scope:** APNPC approach triggers, OPNPCT / OPNPCU (spell / use-item on NPC), NPC_FIND\* lookups, all other NPC mutating ops (`NPC_ANIM`, `NPC_FACESQUARE`, `NPC_DAMAGE`, `NPC_SETMODE`, etc.), OPLOC / OPOBJ routing (different sub-specs).

---

## Goal

After S6b:

- A Java client that left-clicks an NPC sends `OPNPC1..5`. The player walks to the NPC. On arrival, a registered `[opnpc<N>,<typeID>]` script runs with `ActiveNpc` and `Self` bound.
- Scripts can call `NPC_SAY "text"` and the speech bubble appears on the wire in the NPC info block for nearby players.
- Suspension ops (P_DELAY, P_PAUSEBUTTON, P_COUNTDIALOG) inside an OPNPC script work as in S5g — the interaction anchor stays put until the script finishes or aborts.
- No script registered (and no category/global fallback) is a silent no-op — interaction clears, no client message.

## Architecture

```
modules/world/
├── interaction.go              (modify) — call tryFireOpTrigger from processInteraction;
│                                           reset Player.interactionFired in SetInteraction /
│                                           ClearInteraction; add field
├── interaction_trigger.go      (NEW)    — tryFireOpTrigger(p) + dispatch helper
├── interaction_trigger_test.go (NEW)    — unit tests, 10 cases
├── script.go                   (untouched)
└── script_test.go              (modify) — E2E: click → walk → arrive → NPC_SAY on wire

pkg/script/
├── active.go                   (modify) — ActiveNpc gains Say([]byte)
├── handlers_npc.go             (modify) — + handleNpcSay
├── handlers.go                 (modify) — register OpNpcSay
└── handlers_npc_test.go        (modify) — mockNpc gains Say; 2 new tests
```

Two separate slices merge into one sub-spec: **(1) dispatch plumbing** (the bulk), **(2) NPC_SAY** (~48 LOC of glue). They ship together because NPC_SAY is what makes the dispatch observable end-to-end.

## Components

### 1. `Player.interactionFired` — one-shot dispatch gate

Field addition in `modules/world/player.go`:

```go
// interactionFired is true once tryFireOpTrigger has run for the current
// interaction anchor. Prevents re-dispatching the same trigger every tick
// while the player remains in range. Reset by SetInteraction (new click)
// and ClearInteraction (logout / script-driven clear).
interactionFired bool
```

Reset sites in `modules/world/interaction.go`:

- `SetInteraction` — add `p.interactionFired = false` at the end (new anchor = fresh dispatch).
- `ClearInteraction` — add `p.interactionFired = false` (idle = next anchor starts fresh).

### 2. Dispatch helper — `modules/world/interaction_trigger.go` (NEW)

```go
package world

import (
    "github.com/zsrv/goscape/pkg/script"
)

// tryFireOpTrigger fires the [opnpc<op>,<npcType>] trigger for the player's
// anchored NPC target when the player has just reached interaction range.
//
// Matches TS Player.tryInteract() for the NPC branch:
//   - Dead NPC / bad op / missing script: clear interaction silently.
//   - Script suspends: preserve interaction anchor across suspension.
//   - Script finishes / aborts: clear interaction.
//
// Caller (processInteraction) guarantees:
//   - p.interacted == true (reach succeeded this tick)
//   - p.interactionKind == InteractionEngine
//   - p.target != nil
//   - p.interactionFired == false
func tryFireOpTrigger(p *Player) {
    srv := p.client.server

    // S6c will add loc/obj branches here. For S6b, only NPCs dispatch.
    npc, ok := p.target.(*Npc)
    if !ok {
        p.interactionFired = true
        return
    }

    // Player became delayed between reach and dispatch (e.g. concurrent
    // queued script P_DELAYed them). Try again next tick.
    if p.delayed && srv.currentTick < p.delayedUntil {
        return
    }

    if npc.dead {
        p.ClearInteraction()
        p.interactionFired = true
        return
    }

    op := p.targetOp
    if op < 1 || op > 5 {
        p.ClearInteraction()
        p.interactionFired = true
        return
    }

    trigger := script.TriggerOpNpc1 + script.ServerTriggerType(op-1)
    category := 0
    if npc.typ != nil {
        category = npc.typ.Category
    }

    sf := srv.scriptProvider.GetByTrigger(trigger, npc.typeId, category)
    if sf == nil {
        p.ClearInteraction()
        p.interactionFired = true
        return
    }

    state := script.Init(sf, p, false, nil, nil)
    state.ActiveNpc = npc
    state.Pointers |= script.PtrActiveNpc
    state.Provider = srv.scriptProvider
    state.World = srv.worldVars
    state.Configs = srv.configsView
    state.Inv = srv.invLookup

    srv.resumeOrFinish(state, p)

    p.interactionFired = true
    if state.Execution == script.Finished || state.Execution == script.Aborted {
        p.ClearInteraction()
    }
}
```

**Design notes:**

- Built inline rather than extending `runScript`: we need to write `ActiveNpc` and `PtrActiveNpc` between `Init` and `resumeOrFinish`. Extending `runScript` to take an optional NPC would force every existing caller through a new signature for one branch. Deliberate duplication of the five `state.Provider/World/Configs/Inv/Pointers` lines is cheaper than a new overload.
- `resumeOrFinish` already handles Suspended/CountDialog/PauseButton → `StoreActiveScript`, and Finished/Aborted → `ClearActiveScript`. We don't replicate that.
- After `resumeOrFinish`, `state.Execution` reflects the final disposition; we inspect it to decide whether to clear the interaction anchor.

### 3. Hook into `processInteraction`

In `modules/world/interaction.go`, at the point where reach succeeds and `p.interacted = true` is set, add:

```go
if !p.interactionFired {
    tryFireOpTrigger(p)
}
```

The exact insertion point is just after the existing face-entity and `p.interacted = true` assignments. No other modifications.

### 4. `ActiveNpc.Say` — interface extension

In `pkg/script/active.go`, append to the `ActiveNpc` interface:

```go
// Say buffers text as the NPC's speech bubble for the current tick,
// flagging NpcMaskSay so the NPC-info encoder emits it. Empty text is
// allowed (produces an empty bubble that clears itself next tick via
// ResetMasks).
Say(text []byte)
```

The `*Npc` concrete type already has `Say(msg []byte)` in `modules/world/npc_masks.go:11-14`. No new production code in `modules/world/`. The compile-time `var _ script.ActiveNpc = (*Npc)(nil)` check from S6a will catch any shape mismatch at build time.

### 5. `handleNpcSay` — script handler

In `pkg/script/handlers_npc.go`, append:

```go
// handleNpcSay pops a string and sets it as the active NPC's speech
// bubble for this tick. Silent no-op semantics handled by *Npc.Say —
// empty strings are legal.
func handleNpcSay(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_SAY"); err != nil {
        return err
    }
    text := s.PopString()
    s.ActiveNpc.Say([]byte(text))
    return nil
}
```

In `pkg/script/handlers.go`, add one entry to the handler map (next to the S6a NPC read block):

```go
// S6b: NPC mutating ops.
OpNpcSay: handleNpcSay,
```

### 6. mockNpc extension — `pkg/script/handlers_npc_test.go`

Extend the S6a fixture:

```go
type mockNpc struct {
    // ... existing S6a fields ...
    sayCalls []string
}

func (m *mockNpc) Say(text []byte) { m.sayCalls = append(m.sayCalls, string(text)) }
```

## Data flow (one full happy-path tick)

1. **Client → Server (tick N):** `OPNPC1` packet arrives. `handler_opnpc.go` validates and calls `p.SetInteraction(InteractionEngine, npc, 1)`. Side effect: `p.interactionFired = false`.
2. **Tick N → N+K walking:** `processInteraction` runs each tick. `inOperableDistance()` is false; player steps toward NPC.
3. **Tick N+K arrival:** `inOperableDistance()` returns true. `p.interacted = true`, player faces NPC. Because `!p.interactionFired`, `tryFireOpTrigger(p)` runs:
   - Type-assert target to `*Npc`: OK.
   - Dead check: false.
   - Op range: 1.
   - Trigger lookup: `TriggerOpNpc1 + 0 = 10`, typeID=7, category=3. `scriptProvider.GetByTrigger(10, 7, 3)` returns the `[opnpc1,chicken]` script.
   - `state := script.Init(...)`, wire pointers/providers, set `state.ActiveNpc = npc`, `state.Pointers |= PtrActiveNpc`.
   - `srv.resumeOrFinish(state, p)` runs the script to completion. Inside: `NPC_SAY "cluck cluck"` calls `mockNpc.Say` / `*Npc.Say` → sets `sayText` + `NpcMaskSay`.
   - `state.Execution == Finished` → `p.ClearInteraction()`.
   - `p.interactionFired = true` (guards against double-fire if reach were re-tested this tick).
4. **Same tick later — processZones/NPC info encoder:** Encoder sees `NpcMaskSay` on the NPC, writes the text + `0x0A` terminator per the mask block in `pkg/rsbuf/npc_mask_payload.go:22-27`.
5. **Tick N+K+1:** `Npc.ResetMasks()` clears `sayText` and the mask bit. The speech bubble is a one-tick event.

## Edge cases

| # | Case | Expected behaviour |
|---|---|---|
| 1 | Happy path (above) | Script fires once, NPC says, interaction clears. |
| 2 | No script registered (type, category, and global all miss) | Silent: `GetByTrigger` returns nil → `ClearInteraction`, `interactionFired = true`. No client message. |
| 3 | NPC dead at dispatch | Silent: `ClearInteraction`, `interactionFired = true`. |
| 4 | Hidden op (`npc.typ.Op[op-1] == "hidden"`) | Rejected at click time by existing `handler_opnpc.go`; never reaches dispatch. |
| 5 | Target not `*Npc` (OPLOC/OPOBJ land later) | `interactionFired = true`, no-op. Target preserved for that sub-spec's branch. |
| 6 | `targetOp` out of `[1,5]` (corruption / future OPNPCT bleed) | `ClearInteraction`, silent. |
| 7 | Script suspends (P_DELAY / P_PAUSEBUTTON / P_COUNTDIALOG) | Interaction **preserved** (dialog still anchors the NPC). `interactionFired = true` so processInteraction doesn't re-dispatch during the suspension. `resumeOrFinish` stored the state via `StoreActiveScript`; S4's tick-resume path handles it from here. |
| 8 | Player delayed at dispatch time (from concurrent queue) | Skip dispatch, leave `interactionFired = false`. Next tick retries once delay expires. |
| 9 | Re-click mid-walk on new NPC | `SetInteraction` resets `interactionFired = false` and swaps target. New arrival fires for new target. |
| 10 | Category fallback / global fallback | `GetByTrigger`'s 3-level fallback handles these. A script at `(trigger, category, categoryID)` fires if no type-specific hit; `(trigger)` fires if neither. |

**Non-obvious rules:**

- **Suspension keeps the anchor** — the resumed script needs `ActiveNpc` to still make sense. Clearing interaction on suspension would break mid-dialog `NPC_COORD` reads. TS matches this.
- **Delayed player retries next tick** — the gate returns without setting `interactionFired`, so `processInteraction` tries again when the delay expires. Belt-and-suspenders against the tick-boundary race where a queued script delays the player right before dispatch.

## Testing strategy

### Unit tests — `modules/world/interaction_trigger_test.go` (NEW)

Fixture `newTriggerTestFixture(t)` builds a `*Server` with seeded `scriptProvider`, a `*Player` wired via `client.server`, and a `*Npc` (typeID=7, category=3). Tests use a minimal `[NPC_SAY "hello" + RETURN]` script at `(TriggerOpNpc1, 7, 3)`.

Ten cases mirroring the edge-case matrix:

| Test | Setup | Assertion |
|---|---|---|
| `TestTryFireOpTrigger_HappyPath` | NPC alive, script registered | `npc.sayText == "hello"`, `p.target == nil`, `p.interactionFired == true` |
| `TestTryFireOpTrigger_NoScript` | no script registered at any of the 3 lookup tiers | target cleared, `interactionFired == true`, no panic |
| `TestTryFireOpTrigger_DeadNpc` | `npc.dead = true` | target cleared, `interactionFired == true`, no script side effects |
| `TestTryFireOpTrigger_WrongTargetType` | `p.target = &Loc{}` stub | `interactionFired == true`, target preserved |
| `TestTryFireOpTrigger_BadOp` | `p.targetOp = 99` | target cleared, `interactionFired == true` |
| `TestTryFireOpTrigger_ScriptSuspends` | script = `[P_DELAY 5 + RETURN]` | target preserved, `interactionFired == true`, `p.activeScript != nil` |
| `TestTryFireOpTrigger_PlayerDelayed` | `p.delayed = true, p.delayedUntil = currentTick+3` | no dispatch, target preserved, `interactionFired == false` |
| `TestTryFireOpTrigger_ReClickResetsFired` | dispatch, then `SetInteraction(newNpc)` | `interactionFired == false` after SetInteraction |
| `TestTryFireOpTrigger_CategoryFallback` | no type-specific script; category script at `(trigger, category=3)` | category script fires |
| `TestTryFireOpTrigger_GlobalFallback` | neither type nor category; global script at trigger | global script fires |

### Unit tests — `pkg/script/handlers_npc_test.go` (diff)

```go
// Standard NPC_SAY.
func TestNpcSay(t *testing.T) { /* mockNpc.sayCalls == []string{"hello"} */ }
// Interface guard.
func TestNpcSayRequiresActiveNpc(t *testing.T) { /* expect "NPC_SAY: no active npc" */ }
```

### E2E — `modules/world/script_test.go` (diff)

```go
func TestOpNpc1FiresScriptAndEmitsSay(t *testing.T) {
    // Build server + player + NPC type 7 at (3222, 3222).
    // Register [opnpc1,type7] = [push_string "cluck cluck" + NPC_SAY + RETURN].
    // Invoke handler_opnpc.go with OPNPC1 payload pointing at npc slot 0.
    // Drive ticks until inOperableDistance() true.
    // Assert npc.sayText == []byte("cluck cluck") and NpcMaskSay bit set.
    // Assert p.target == nil.
    // Optionally: flush outbound, decode NPC info block, assert "cluck cluck\n"
    //   appears in the mask payload.
}
```

Hermetic — drives the packet parser directly, same pattern as `TestPushesOutKeyPlayerInfo` and the S4/S5g suspension tests. No TCP loopback.

## LOC estimate

| File | LOC |
|---|---|
| `modules/world/interaction.go` (diff) | +8 |
| `modules/world/interaction_trigger.go` (new) | ~120 |
| `modules/world/interaction_trigger_test.go` (new) | ~220 |
| `modules/world/script_test.go` (diff) | +80 |
| `modules/world/player.go` (diff — one field) | +2 |
| `pkg/script/active.go` (diff) | +6 |
| `pkg/script/handlers_npc.go` (diff) | +9 |
| `pkg/script/handlers.go` (diff) | +3 |
| `pkg/script/handlers_npc_test.go` (diff) | +30 |
| **Total** | **~478** |

## Key design calls

- **Inline state construction in `tryFireOpTrigger` rather than extending `runScript`.** Adding a second required parameter to `runScript` to cover one branch would force every existing caller through a new signature. The five-line duplication here is deliberate.
- **One-shot `interactionFired` gate.** `p.interacted` reflects "in range" and stays true as long as the player stands adjacent. Without a separate gate, dispatch would fire every tick. The gate lives on `Player` (not a method-local closure) because `SetInteraction` / `ClearInteraction` need to reset it across the whole lifecycle of an interaction.
- **Suspension preserves the anchor.** The resumed script's `ActiveNpc` read must still make sense; clearing on suspension would break dialogs. TS agrees.
- **Silent no-script fail.** Matches TS and matches every other trigger-lookup site in goscape (queued scripts, timers, login). No fallback "Nothing interesting happens" message.
- **NPC_SAY only.** `NPC_ANIM`, `NPC_FACESQUARE`, `NPC_DAMAGE`, `NPC_SETMODE` etc. are one-liner additions over the existing mask infrastructure but shipping them here would double the test surface; they live in S6c.
- **Op 1..5 stored raw on `targetOp`, translated to trigger id at dispatch time.** `TriggerOpNpc1 + op - 1` is cheap. TS's `targetOp + 7` offset is baked into store-time; we avoid the offset by not conflating `targetOp` with AP trigger IDs. When APNPC arrives in S6c, that sub-spec can choose its own representation.

## Gotchas

- **`Server` field is `scriptProvider`, not `scripts`.** The tick tests show the canonical name.
- **`script.Init` is the state constructor, not `NewScriptState`.** Seeds `Self` and `PtrActivePlayer` when `self != nil`.
- **`resumeOrFinish` already calls `StoreActiveScript` / `ClearActiveScript`.** `tryFireOpTrigger` only calls `ClearInteraction` (a different concept from `ClearActiveScript`).
- **`PtrActiveNpc` already exists** (`pkg/script/pointer.go:10`, value `1 << 2`). No new pointer constant needed.
- **`requireActiveNpc` helper already exists** in `handlers_npc.go` from S6a. `handleNpcSay` reuses it.
- **`Npc.dead` is the liveness flag** (not `isValid` or similar). Confirmed in `handler_opnpc.go` usage.
- **`Npc.typ.Category`** is the access path for category; nil-guard `npc.typ` before the access.
