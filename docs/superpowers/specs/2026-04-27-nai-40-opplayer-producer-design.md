# NAI-40 — OPPLAYER trigger producer (player→player op-click dispatch)

**Date:** 2026-04-27
**Status:** Spec
**Predecessor:** NAI-39 (HINT_PL/HINT_COORD/HINT_STOP + activePlayer2 substrate, HEAD `28e9e83`)
**Tech stack:** Go 1.26+

## Summary

Port the player→player op-click trigger dispatch path so that an `OpPlayer<N>` (or `OpPlayerT` / `OpPlayerU`) client packet results in the target player's `[opplayer<N>,_]` (or `[applayer<N>,_]`) trigger script running with `Self` = target and `Self2` = clicker. Closes `NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER` by giving NAI-39's `Self2` substrate its production producer.

## Closes / opens

- **Closes:** `NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER`
- **Opens (track-only):** `NAI-40-D-OPCALLED-MISSING` — TS sets `player.opcalled = true` after a successful op-click handler exit; goscape uses the existing `interactionFired` gate (set by trigger fire) instead. Pre-existing S6a-era convention; cross-cutting refactor deferred to `NAI-40-SB1`.
- **Conditional open** (verified at plan-write): `NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK` — if `Player.ts:1170-1185` calls `setFaceEntity(other)` on op-click against a player target and goscape does not, tag at the new `processInteraction` Player-arm.

## Scope

### In scope

- Three new client-message handlers: `OpPlayer` (parametric over ops 1..4), `OpPlayerT` (use spell on player), `OpPlayerU` (use item on player).
- Per-handler gate sequence mirroring TS `OpPlayerHandler.ts` / `OpPlayerTHandler.ts` / `OpPlayerUHandler.ts`.
- New `Server.LookupPlayerBySlot(slot int) *Player` server method.
- Two new sentinels in `interaction.go`: `targetOpPlayerT = 10`, `targetOpPlayerU = 11`.
- Extension of `processInteraction` with a `*Player` target arm (Chebyshev-1 contact, AP→OP sequencing; no `SetFaceEntity` unless TS does it).
- New file `modules/world/player_interaction_trigger.go` mirroring `npc_interaction_trigger.go`: `opPlayerTriggerForOp` / `apPlayerTriggerForOp` + `fireOpTriggerPlayer` / `fireApTriggerPlayer`.
- Extension of `tryFireOpTrigger` / `tryFireApTrigger` (in `modules/world/interaction_trigger.go`) with a `case *Player:` arm.
- `Self2` binding via existing `buildPlayerScriptState` `case script.ActivePlayer:` arm at `modules/world/script.go:54-57` — no infra change; the new fire functions pass the clicker as the `ActivePlayer`-typed second arg.
- Removal of the deviation comments at `pkg/script/state.go:194-196` and `modules/world/script.go:30-33` in the close commit.

### Out of scope (scope-boundary tags)

- `NAI-40-SB1-OPCALLED-FLAG-CONVERGENCE` — adopt `opcalled` flag broadly (Loc + Npc + Player); cross-cutting refactor.
- `NAI-40-SB2-FINDHERO-BOTH-HEROPOINTS` — script-syntax `_activePlayer2` producer pair (TS `PlayerOps.ts:1138-1170`); needs HeroPoints damage-table infra.
- `NAI-40-SB3-AI-OPPLAYER-SETS-SELF2` — verify NPC-AI `OPPLAYER<N>` trigger dispatch sets `Self2`; grep at NAI-40 close.
- `NAI-40-SB4-SLOT-REUSE-DETECTION` — distinguish "target logged out" from "different player at reused slot" mid-walk.
- `OPPLAYER5` — not surfaced by the real client (only `OPPLAYER1..4` in `prot.go`); the trigger constant exists for the NPC-AI side only.
- The `P_OPPLAYER` script opcode (`PlayerOps.ts:1009`) — script-syntax `~p_opplayer` invocation; sibling sub-spec since it consumes `Self2` rather than producing it.

## Architecture

### Wire-to-script-state data flow

```
client packet (164/53/185/206/177/248)
    ↓ pkg/io/protocol/game/client decoder
new model decoders: OpPlayer{PlayerSlot,Op}, OpPlayerT{PlayerSlot,SpellCom},
                    OpPlayerU{PlayerSlot,UseObj,UseSlot,UseCom}
    ↓
modules/world/handler_op_player.go (new) — three handlers:
  handleOpPlayer  (parametric ops 1..4)
  handleOpPlayerT (use spell on player)
  handleOpPlayerU (use item on player)
    ↓
shared gate sequence (TS-faithful):
  if p.delayed && currentTick < delayedUntil → sendUnsetMapFlag, return
  T-only:  Component(spellCom) actionTarget&PLAYER + isComponentVisible
  U-only:  Component(useCom) usable + isComponentVisible
           + invListenerForCom + invValidSlot + invHasAt + members-check
  other := server.LookupPlayerBySlot(playerSlot)  // new
  if other == nil → sendUnsetMapFlag, return
  if !rsbuf.HasPlayer(p.slot, other.slot) → sendUnsetMapFlag, return
  p.ClearPendingAction()
  U-only: p.lastUseItem = useObj; p.lastUseSlot = useSlot
  p.SetInteraction(InteractionEngine, other, op_or_sentinel, com_or_-1)
     1..4: op = 1..4,                         com = -1
     T:    op = targetOpPlayerT (10),         com = spellCom
     U:    op = targetOpPlayerU (11),         com = -1   (TS quirk: useCom not snapshotted)
    ↓
modules/world/interaction.go (mod) — processInteraction Player-arm:
  same Chebyshev-1 contact gate as Npc/Loc; no SetFaceEntity (TS-pending verification);
  same AP→OP sequencing
    ↓
modules/world/player_interaction_trigger.go (new) — mirror of npc_interaction_trigger.go:
  opPlayerTriggerForOp(op) → ServerTriggerType  (1..4 / T / U → corresponding TriggerOpPlayer*)
  apPlayerTriggerForOp(op) → symmetric
  fireOpTriggerPlayer(p, target):
     trigger := opPlayerTriggerForOp(p.targetOp)
     sf := server.findScriptForTrigger(trigger)
     if sf == nil → return
     server.runScript(sf, target /* Self */, p /* clicker, threaded as ActivePlayer for Self2 */, true, nil, nil)
     p.interactionFired = true
  fireApTriggerPlayer(p, target):
     trigger := apPlayerTriggerForOp(p.targetOp)
     sf := server.findScriptForTrigger(trigger)
     if sf == nil → p.apRange = -1; return
     server.runScript(sf, target, p, true, nil, nil)
     p.interactionFired = true
    ↓
modules/world/interaction_trigger.go (mod) — tryFireOpTrigger / tryFireApTrigger:
  extend type-switch with case *Player → fireXTriggerPlayer(p, target)
    ↓
modules/world/script.go (mod) — buildPlayerScriptState ActivePlayer-case
  already sets state.Self2 = t; this is the binding pin closing NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER.
```

### Files

| File | New / mod | Approx LOC |
|---|---|---|
| `pkg/io/protocol/game/client/op_player.go` (or extend existing decoder file) | new | ~70 |
| `pkg/io/protocol/game/client/op_player_test.go` | new | ~80 |
| `modules/world/handler_op_player.go` | new | ~200 |
| `modules/world/handler_op_player_test.go` | new | ~150 |
| `modules/world/interaction.go` (sentinels + processInteraction Player arm) | mod | ~25 |
| `modules/world/interaction_trigger.go` (extend tryFire{Op,Ap}Trigger type-switch) | mod | ~20 |
| `modules/world/player_interaction_trigger.go` | new | ~140 |
| `modules/world/player_interaction_trigger_test.go` | new | ~100 |
| `modules/world/script.go` (no change to buildPlayerScriptState; close-commit comment removal) | mod | ~5 |
| `pkg/script/state.go` (close-commit deviation-comment removal) | mod | ~5 |
| `modules/world/server.go` (`LookupPlayerBySlot`) | mod | ~15 |
| `modules/world/script_test.go` (E2E smoke) | mod | ~80 |
| **Production total** | | **~480 LOC** |
| **Including tests** | | **~890 LOC** |

(Estimates may shift up to ±15% during plan-write once exact field paths are read.)

### Closes / opens deviations (final)

| Tag | Action at NAI-40 |
|---|---|
| `NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER` | **CLOSE** — comments removed at `pkg/script/state.go:194-196` and `modules/world/script.go:30-33` |
| `NAI-40-D-OPCALLED-MISSING` | **OPEN** (track-only) — comment at handler exit |
| `NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK` | **conditional OPEN** at plan-write |

## Data shapes & interfaces

### Client message structs (`pkg/io/protocol/game/client/`)

```go
// OpPlayer covers ops 1..4. Wire body = u2 PlayerSlot. Op carried in
// dispatcher (one of 4 opcodes 164/53/185/206) → stored in model so
// the handler does TriggerApPlayer1 + (Op-1) arithmetic.
type OpPlayer struct {
    PlayerSlot int
    Op         int // 1..4
}

// OpPlayerT — opcode 177, size 4. Wire body = u2 PlayerSlot, u2 SpellCom.
type OpPlayerT struct {
    PlayerSlot int
    SpellCom   int
}

// OpPlayerU — opcode 248, size 8. Wire body = u2 PlayerSlot, u2 UseObj,
// u2 UseSlot, u2 UseCom.
type OpPlayerU struct {
    PlayerSlot int
    UseObj     int
    UseSlot    int
    UseCom     int
}
```

### Sentinels (new in `modules/world/interaction.go`)

```go
const (
    targetOpLocT    = 6
    targetOpLocU    = 7
    targetOpNpcT    = 8
    targetOpNpcU    = 9
    targetOpPlayerT = 10  // new
    targetOpPlayerU = 11  // new
)
```

### Server lookup (new in `modules/world/server.go`)

```go
// LookupPlayerBySlot returns the logged-in player at the given slot
// index, or nil. Mirrors TS World.getPlayer(slot). Used by OpPlayer
// handlers to resolve message PlayerSlot to a target Player.
func (s *Server) LookupPlayerBySlot(slot int) *Player {
    // exact field path resolved at plan-write (s.players /
    // s.playersBySlot / iteration); pin in plan code block
}
```

### `targetSubject` snapshot — Loc/Npc-only (decision)

Player target identity flows through `p.target.(*Player)` at every consumer site. No `targetPlayer` field added to `targetSubject`. Stale-target / slot-reuse detection deferred to `NAI-40-SB4`.

For OpPlayerT: `targetSubject.com = spellCom` (mirrors OpNpcT).
For OpPlayerU: `targetSubject.com = -1` (TS quirk; `useCom` not snapshotted); `lastUseItem` / `lastUseSlot` snapshot (mirrors OpNpcU).

### `script.ActivePlayer` interface — no extension

No script opcode in this sub-spec calls "set interaction against a player from a script." Future `P_OPPLAYER` opcode (`NAI-40-SB`-class follow-up) would add `SetInteractionScriptPlayer` mirroring `SetInteractionScriptLoc` / `SetInteractionScriptNpc`. Keeping the interface untouched here keeps the diff focused.

### Trigger-fire signatures (`modules/world/player_interaction_trigger.go`)

```go
func opPlayerTriggerForOp(op int) script.ServerTriggerType {
    switch {
    case op >= 1 && op <= 4:
        return script.TriggerOpPlayer1 + script.ServerTriggerType(op-1)
    case op == targetOpPlayerT:
        return script.TriggerOpPlayerT
    case op == targetOpPlayerU:
        return script.TriggerOpPlayerU
    }
    return 0
}

func apPlayerTriggerForOp(op int) script.ServerTriggerType { /* symmetric over TriggerApPlayer* */ }

func fireOpTriggerPlayer(p *Player, target *Player) { /* see data-flow above */ }
func fireApTriggerPlayer(p *Player, target *Player) { /* AP-side; sets p.apRange = -1 on miss */ }
```

### `Server.RunPlayerTriggerWithSecondary` — inlined

The trigger fire functions inline the `runScript` call directly (matches Loc/Npc precedent). No new typed entry point on Server.

### Script-registry lookup for player triggers

Players have no type — registry lookup is `findScriptForTrigger(trigger)` (single arg) versus `findScriptForTrigger(trigger, typeId)` for typed targets. Plan-time pre-flight resolves whether goscape's existing registry has a no-type-key variant or whether a sentinel typeId is used.

## Test strategy — TDD layer-by-layer

### Layer 1 — Decoder unit tests (`pkg/io/protocol/game/client/op_player_test.go`)

- `TestOpPlayer_DecodeOp1` … `_DecodeOp4` — opcode→Op mapping (164→1, 53→2, 185→3, 206→4) + PlayerSlot read
- `TestOpPlayerT_Decode` — (PlayerSlot, SpellCom) byte order
- `TestOpPlayerU_Decode` — (PlayerSlot, UseObj, UseSlot, UseCom) byte order
- `TestOpPlayer_TruncatedBody` — short-read fails cleanly

### Layer 2 — Trigger-map unit tests (`modules/world/player_interaction_trigger_test.go`)

- `TestOpPlayerTriggerForOp` — table: 1→TriggerOpPlayer1, 2→2, 3→3, 4→4, targetOpPlayerT→TriggerOpPlayerT, targetOpPlayerU→TriggerOpPlayerU, 0→0, 5→0, -1→0
- `TestApPlayerTriggerForOp` — symmetric

### Layer 3 — Handler integration tests (`modules/world/handler_op_player_test.go`)

- `TestHandleOpPlayer_HappyPath` — for each op 1..4: targetOp = op, target = other, targetSubject.com = -1, kind = InteractionEngine
- `TestHandleOpPlayer_DelayedSendsUnsetMapFlag`
- `TestHandleOpPlayer_TargetNotLoggedIn` — `LookupPlayerBySlot` returns nil → UnsetMapFlag
- `TestHandleOpPlayer_NotVisibleViaRsbuf` — `rsbuf.HasPlayer` false → UnsetMapFlag
- `TestHandleOpPlayerT_HappyPath` — targetOp = targetOpPlayerT, targetSubject.com = spellCom
- `TestHandleOpPlayerT_ComponentNotForPlayer` — actionTarget&PLAYER == 0 → UnsetMapFlag
- `TestHandleOpPlayerT_ComponentNotVisible` — UnsetMapFlag
- `TestHandleOpPlayerU_HappyPath` — targetOp = targetOpPlayerU, targetSubject.com = -1, lastUseItem = useObj, lastUseSlot = useSlot
- `TestHandleOpPlayerU_ComponentNotUsable` — UnsetMapFlag
- `TestHandleOpPlayerU_InvListenerMissing` — UnsetMapFlag
- `TestHandleOpPlayerU_InvalidSlotOrItemMismatch` — two cases, UnsetMapFlag
- `TestHandleOpPlayerU_MembersOnNonMembersServer` — messageGame + UnsetMapFlag

### Layer 4 — processInteraction Player-arm tests (`modules/world/interaction_test.go` / `script_test.go`)

- `TestProcessInteractionPlayer_AdjacentFiresOp`
- `TestProcessInteractionPlayer_OutOfRangePathsToTarget`
- `TestProcessInteractionPlayer_DifferentLevelClears`
- `TestProcessInteractionPlayer_NoSetFaceEntity` (per the conditional `NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK` outcome)

### Layer 5 — Trigger-fire + Self2 binding (`modules/world/player_interaction_trigger_test.go`)

- `TestFireOpTriggerPlayer_BindsSelf2ToClicker` — register `[opplayer1,_]` → `~hint_pl(active_player2)` → assert hint-arrow mask on target points at clicker
- `TestFireOpTriggerPlayer_NoScriptRegistered` — silent no-op
- `TestFireApTriggerPlayer_NoScriptSetsApRangeMinusOne`

### Layer 6 — End-to-end smoke (`modules/world/script_test.go`)

- `TestOpPlayer1_E2E_HintPlOnTarget` — full path: client packet → handler → tick processInteraction → trigger fires → `[opplayer1,_]` script `~hint_pl(active_player2)` → assert hint-arrow mask. **Deviation-closure pin per `true_to_ts_gate.md`.**

### Test fixtures

- `mockPlayer` already has Slot/UID/HintPlayer plumbing (NAI-39 tests).
- Two-Player Server fixtures precedented at `modules/world/script_test.go:1292-1335`.
- Use real `Server` + spawned Players for E2E (no mocks at the smoke layer).

## Task sequencing

| # | Task | LOC est | Layer |
|---|---|---|---|
| T1 | OpPlayer / OpPlayerT / OpPlayerU client-message structs + decoders | ~70 | 1 |
| T2 | `Server.LookupPlayerBySlot` + `targetOpPlayerT/U` sentinels | ~30 | infra |
| T3 | `handleOpPlayer` (ops 1..4) — gate sequence + SetInteraction | ~50 | 3 |
| T4 | `handleOpPlayerT` — spell-component validation + spellCom snapshot | ~60 | 3 |
| T5 | `handleOpPlayerU` — full validation chain + `lastUse{Item,Slot}` snapshot | ~90 | 3 |
| T6 | `player_interaction_trigger.go` (trigger maps + fire functions + tryFire dispatch wiring + processInteraction Player-arm) | ~140 | 2+4+5 |
| T7 | E2E smoke (`TestOpPlayer1_E2E_HintPlOnTarget`) + close commit (deviation comments removed) | ~50 | 6 |

Total: **~490 LOC production + ~250 LOC tests = ~740 LOC across 7 tasks.**

## Cadence

- Standard cadence per `compressed_cadence.md` (>100 LOC): full spec + plan + per-task two-stage review (spec-compliance opus → code-quality opus) + final whole-impl review.
- Pre-flight before each task dispatch per `controller_preflight.md`.
- Plan-time grep-and-list every existing `tryFireOpTrigger` / `tryFireApTrigger` call site per `enumerate_all_sites.md`.
- TS source reads required at plan-write: `Player.ts:1100-1210` interaction tick body (for `setFaceEntity` on player target), `OpPlayerHandler.ts` / `OpPlayerTHandler.ts` / `OpPlayerUHandler.ts` (already done in spec-write).

## References

- TS source (canonical, per `ts_source_canonical_path.md`): `LostCityRS/Engine-TS`
  - `src/network/game/client/handler/OpPlayerHandler.ts`
  - `src/network/game/client/handler/OpPlayerTHandler.ts`
  - `src/network/game/client/handler/OpPlayerUHandler.ts`
  - `src/engine/script/ScriptState.ts:215-243` (activePlayer/activePlayer2 getters/setters)
  - `src/engine/script/ScriptRunner.ts:78-92` (target-binding switch)
  - `src/engine/entity/Player.ts:1100-1210` (interaction tick)
- goscape predecessor: NAI-39 (`28e9e83`)
- NPC-side analog: `modules/world/npc_interaction_trigger.go`, `modules/world/handler_opnpc*.go`
