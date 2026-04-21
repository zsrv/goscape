# S6m — OpLocT + OpLocU Handler Design

> **Sub-spec context:** Thirteenth sub-spec in the runescript-s* series. Closes S6j-D5 (last major unclosed S6j deviation) by wiring the OpLocT (spell-on-loc) and OpLocU (item-on-loc) sibling opcodes to fire single-trigger APLOCT/OPLOCT and APLOCU/OPLOCU scripts.

> **TS-faithfulness gate:** User requires "true to TS." All behavioral claims cite TS line numbers in `/home/owner/Code/github.com/LostCityRS/Engine-TS`. Four new documented deviations (S6m-D1 through S6m-D4), all for validation gates that require pre-existing infrastructure goscape hasn't built yet (component registry, members-config, keyed InvListener map).

> **Scope:** Approach 1 (scoped validation). Core TS gates (delayed, viewport, loc-exists, locType-exists) wired; complex validation (component visibility, members-only items, inventory-listener lookup) deferred with explicit deviation documentation.

## 1. Goal

Wire the two remaining OPLOC click-opcode variants to fire their respective triggers:

- **OpLocT** (opcode 9, 8-byte payload): spell-on-loc. Player drags a spell-book interface icon onto a loc; fires `[aploct,<locType>]` / `[oploct,<locType>]` scripts.
- **OpLocU** (opcode 75, 12-byte payload): item-on-loc. Player drags an inventory item onto a loc (e.g., axe on tree); fires `[aplocu,<locType>]` / `[oplocu,<locType>]` scripts.

After this sub-spec, the full OPLOC click surface is wired:

| Click type | Opcode | Trigger | Shipped in |
|---|---|---|---|
| OpLoc1..5 | 245, 172, 96, 97, 116 | APLOC1..5 / OPLOC1..5 | S6j / S6k / S6l |
| **OpLocT** | 9 | APLOCT / OPLOCT | **S6m** |
| **OpLocU** | 75 | APLOCU / OPLOCU | **S6m** |

## 2. Architecture

Three phases mirroring S6j/S6l's shape, with one design inversion: T/U variants use single triggers (APLOCT/OPLOCT, APLOCU/OPLOCU) instead of 5-op variants.

**Phase A — Two click handlers** (`handler_oploc.go` expansion):
- `handleOpLocT(p, payload)` — decode 8-byte payload (x, z, loc, spellCom), validate gates, `SetInteraction(InteractionEngine, loc, targetOpLocT, spellCom)`
- `handleOpLocU(p, payload)` — decode 12-byte payload (x, z, loc, useObj, useSlot, useCom), validate gates, set `p.lastUseItem` / `p.lastUseSlot`, `SetInteraction(InteractionEngine, loc, targetOpLocU, -1)`

**Phase B — `SetInteraction` signature change**. Current signature `SetInteraction(kind, target, op)` grows to `SetInteraction(kind, target, op, com int)`. `targetSubject.com` resurrected (was removed in S6j). All 19 existing call sites pass `-1` for `com`.

**Phase C — Fire dispatch extension**. `fireApTriggerLoc` and `fireOpTriggerLoc` use a new `apLocTriggerForOp(op) (trigger, ok)` helper that switches on `p.targetOp`:
- `1..5` → `TriggerApLoc1 + (op-1)` (existing)
- `6` (targetOpLocT) → `TriggerApLocT` (single)
- `7` (targetOpLocU) → `TriggerApLocU` (single)
- else → `ok = false`, caller silent-clears

Sentinels `targetOpLocT = 6` / `targetOpLocU = 7` live in `interaction.go` near `InteractionEngine`.

### Data flow

```
Spell-on-loc:
click → handleOpLocT decodes (x, z, locId, spellCom)
      → validates → SetInteraction(..., 6, spellCom)
      → targetSubject.{typ, x, z, level, com=spellCom}
tick N: within apRange → fireApTriggerLoc
                       → apLocTriggerForOp(6) → TriggerApLocT
                       → script fires with ActiveLoc, reads spell via p.TargetSubjectCom()
                       → apRangeCalled persistence applies per S6l

Item-on-loc:
click → handleOpLocU decodes (x, z, locId, useObj, useSlot, useCom)
      → validates → sets p.lastUseItem=useObj, p.lastUseSlot=useSlot
      → SetInteraction(..., 7, -1)
tick N: within apRange → fireApTriggerLoc
                       → apLocTriggerForOp(7) → TriggerApLocU
                       → script reads item via p.LastUseItem() / p.LastUseSlot()
```

## 3. File Map

| File | Action | Purpose |
|---|---|---|
| `modules/world/interaction.go` | Modify | `SetInteraction` gains `com int` param; add `targetOpLocT = 6`, `targetOpLocU = 7` constants |
| `modules/world/player.go` | Modify | `targetSubject` struct regains `com int` field |
| `modules/world/player_script.go` | Modify | Add `*Player.TargetSubjectCom() int` method |
| `pkg/script/active.go` | Modify | `ActivePlayer` interface gains `TargetSubjectCom() int` |
| `modules/world/handler_oploc.go` | Modify | Add `handleOpLocT` + `handleOpLocU` |
| `modules/world/handler_oploc_test.go` | Modify | 12 validation tests |
| `modules/world/handlers_game.go` | Modify | Wire `gameHandlers[9] = handleOpLocT`, `gameHandlers[75] = handleOpLocU` |
| `modules/world/interaction_trigger.go` | Modify | Add `apLocTriggerForOp` helper; extend `fireApTriggerLoc` + `fireOpTriggerLoc` to use it |
| `modules/world/interaction_trigger_test.go` | Modify | 6 fire tests |
| 19 call sites of `SetInteraction` | Modify | Mechanical `, -1` argument addition (see §7 Task 1) |

**Existing infrastructure leveraged (no changes needed):**
- Opcodes `OPLOCT=9` (8 bytes), `OPLOCU=75` (12 bytes) — `pkg/io/protocol/game/client/prot.go:73-74` (pre-registered)
- Triggers `TriggerApLocT=65`, `TriggerOpLocT=72`, `TriggerApLocU=64`, `TriggerOpLocU=71` — `pkg/script/trigger.go:65-76` (pre-enumerated)
- `Player.lastUseItem int`, `Player.lastUseSlot int` — `modules/world/player.go:173` (fields + accessor methods exist)
- `locStillValid` — S6j lifecycle helper (used as-is for T/U)
- `inApproachDistance` / `inOperableDistance` — S6l/S6j helpers (unchanged)
- `fireApTriggerLoc` persistence contract with `apRangeCalled` — S6l (applies to T/U scripts too)

## 4. TS Reference Map

- **OpLocTHandler:** `src/network/game/client/handler/OpLocTHandler.ts:~49` — single APLOCT trigger; stores `spellComId` via `setInteraction(Engine, loc, APLOCT, spellComId)`
- **OpLocUHandler:** `src/network/game/client/handler/OpLocUHandler.ts:~79` — single APLOCU trigger; stores `useObj`/`useSlot` on player
- **Trigger offset:** `src/engine/entity/Player.ts:~997` — `getOpTrigger()` returns `targetOp + 7` (APLOC→OPLOC). Verified: APLOCT 65 + 7 = OPLOCT 72; APLOCU 64 + 7 = OPLOCU 71.
- **Payload decoders:** `src/network/game/client/decode/OpLocTDecoder.ts` (x, z, loc, spellCom — all G2, 8 bytes total), `OpLocUDecoder.ts` (x, z, loc, useObj, useSlot, useCom — all G2, 12 bytes total)

## 5. Component Details

### 5.1 `SetInteraction` signature change

**Current** (`modules/world/interaction.go:24`):
```go
func (p *Player) SetInteraction(kind InteractionKind, target entity, op int) {
    p.target = target
    p.targetOp = op
    p.interactionKind = kind
    p.apRange = 10
    p.apRangeCalled = false
    p.interacted = false
    p.repathed = false
    p.interactionFired = false
}
```

**New:**
```go
// SetInteraction anchors the interaction state machine. For OpLocT the
// com parameter carries the spellCom; for OpLocU pass -1 (item tracking
// uses lastUseItem/lastUseSlot instead). For OpLoc1..5 and OpNpc1..5,
// callers pass -1.
func (p *Player) SetInteraction(kind InteractionKind, target entity, op, com int) {
    p.target = target
    p.targetOp = op
    p.targetSubject.com = com
    p.interactionKind = kind
    p.apRange = 10
    p.apRangeCalled = false
    p.interacted = false
    p.repathed = false
    p.interactionFired = false
}
```

Sentinel constants appended near `InteractionEngine`:

```go
// Sentinel targetOp values for non-op-numbered Loc interaction types.
// OpLoc1..5 use op = 1..5 (the op slot clicked); T and U variants use
// these sentinels so fireXxxTriggerLoc can dispatch to the correct
// single-trigger (APLOCT/OPLOCT or APLOCU/OPLOCU).
const (
    targetOpLocT = 6 // APLOCT / OPLOCT dispatch marker
    targetOpLocU = 7 // APLOCU / OPLOCU dispatch marker
)
```

### 5.2 `targetSubject.com` field

In `modules/world/player.go` (current line 86-92 after S6l):

```go
targetSubject struct{ typ, x, z, level int }
```

Becomes:

```go
// targetSubject snapshots the identity of the interaction target at
// click time. Components:
//   typ, x, z, level — loc identity for tryFireXxxTriggerLoc's
//     lifecycle gate (set by OpLoc handlers after SetInteraction).
//   com — spell-component ID for OpLocT; -1 for OpLoc1..5 and OpLocU.
//     Scripts read via ActivePlayer.TargetSubjectCom() (S6m).
// S6m: com field resurrected from S6j shrink to carry spellCom.
targetSubject struct{ typ, x, z, level, com int }
```

### 5.3 `handleOpLocT` — spell-on-loc handler

Append to `modules/world/handler_oploc.go`:

```go
// handleOpLocT is the handler for OPLOCT (opcode 9, 8-byte payload).
// Spell-on-loc: player drags a spell icon from the magic-book interface
// onto a loc. Payload = (x:G2, z:G2, locId:G2, spellCom:G2).
//
// Validation gates (mirrors TS OpLocTHandler.ts:~49):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. coords outside viewport (52-tile half-extent) → UnsetMapFlag
//  4. Server.GetLoc returns nil → UnsetMapFlag
//  5. LocType not registered → UnsetMapFlag
//
// DEVIATION (S6m-D1): TS also validates spellCom references a component
// with ComActionTarget.LOC flag AND that the component is visible in the
// player's interface stack (OpLocTHandler.ts:~25-35). Skipped here
// because goscape has no component registry yet. Effective risk:
// client can forge spellCom values; scripts reading
// p.TargetSubjectCom() get raw wire values. Follow-up: "component
// registry + ComActionTarget validation" sub-spec.
//
// On success: ClearPendingAction → SetInteraction(Engine, loc,
// targetOpLocT, spellCom) → targetSubject snapshot.
func handleOpLocT(p *Player, payload []byte) error {
    if p.client == nil || p.client.server == nil {
        return nil
    }
    s := p.client.server

    if p.delayed && s.currentTick < p.delayedUntil {
        sendUnsetMapFlag(p)
        return nil
    }

    if len(payload) < 8 {
        sendUnsetMapFlag(p)
        return nil
    }

    r := packet.NewPacket(payload)
    x := int(r.G2())
    z := int(r.G2())
    locId := int(r.G2())
    spellCom := int(r.G2())

    dx := x - p.originX
    if dx < 0 {
        dx = -dx
    }
    dz := z - p.originZ
    if dz < 0 {
        dz = -dz
    }
    if dx > 52 || dz > 52 {
        sendUnsetMapFlag(p)
        return nil
    }

    loc := s.GetLoc(p.level, x, z, locId)
    if loc == nil {
        sendUnsetMapFlag(p)
        return nil
    }

    locType := s.locTypes.Configs[locId]
    if locType == nil {
        sendUnsetMapFlag(p)
        return nil
    }

    p.ClearPendingAction()
    p.SetInteraction(InteractionEngine, loc, targetOpLocT, spellCom)
    p.targetSubject.typ = loc.Type()
    p.targetSubject.x = loc.X
    p.targetSubject.z = loc.Z
    p.targetSubject.level = loc.Level
    return nil
}
```

### 5.4 `handleOpLocU` — item-on-loc handler

Append to `modules/world/handler_oploc.go`:

```go
// handleOpLocU is the handler for OPLOCU (opcode 75, 12-byte payload).
// Item-on-loc: player drags an inventory item onto a loc (e.g., axe on
// tree, tinderbox on logs, seed on patch).
// Payload = (x:G2, z:G2, locId:G2, useObj:G2, useSlot:G2, useCom:G2).
//
// Validation gates (subset of TS OpLocUHandler.ts:~79):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. coords outside viewport → UnsetMapFlag
//  4. Server.GetLoc returns nil → UnsetMapFlag
//  5. LocType not registered → UnsetMapFlag
//
// DEVIATION (S6m-D2): TS validates useCom references a usable,
// visible interface component (OpLocUHandler.ts:~25-35). Skipped —
// see S6m-D1 rationale (no component registry).
//
// DEVIATION (S6m-D3): TS does an inventory-listener lookup by useCom
// to verify the player has an inv listening at that interface, plus
// slot-bounds + item-at-slot-matches-useObj validation
// (OpLocUHandler.ts:~45-70). Goscape's invListeners is a slice, not a
// keyed map, so this lookup shape doesn't translate directly. Skip;
// scripts reading p.LastUseItem() / p.LastUseSlot() get raw wire
// values. Security risk: client can claim any item/slot. Real
// scripts defensively re-check via inv_getobj-style opcodes.
// Follow-up: "InvListener keyed-map refactor + OpLocU item
// validation" sub-spec.
//
// DEVIATION (S6m-D4): TS checks members-only items against
// NODE_MEMBERS server config (OpLocUHandler.ts:~71-77). Skipped
// because goscape has no members-config surface yet. Follow-up:
// "members-config + item-gating" sub-spec.
//
// On success: set p.lastUseItem = useObj, p.lastUseSlot = useSlot →
// ClearPendingAction → SetInteraction(Engine, loc, targetOpLocU, -1)
// → targetSubject snapshot.
func handleOpLocU(p *Player, payload []byte) error {
    if p.client == nil || p.client.server == nil {
        return nil
    }
    s := p.client.server

    if p.delayed && s.currentTick < p.delayedUntil {
        sendUnsetMapFlag(p)
        return nil
    }

    if len(payload) < 12 {
        sendUnsetMapFlag(p)
        return nil
    }

    r := packet.NewPacket(payload)
    x := int(r.G2())
    z := int(r.G2())
    locId := int(r.G2())
    useObj := int(r.G2())
    useSlot := int(r.G2())
    _ = int(r.G2()) // useCom — deliberately discarded (S6m-D2/D3)

    dx := x - p.originX
    if dx < 0 {
        dx = -dx
    }
    dz := z - p.originZ
    if dz < 0 {
        dz = -dz
    }
    if dx > 52 || dz > 52 {
        sendUnsetMapFlag(p)
        return nil
    }

    loc := s.GetLoc(p.level, x, z, locId)
    if loc == nil {
        sendUnsetMapFlag(p)
        return nil
    }

    locType := s.locTypes.Configs[locId]
    if locType == nil {
        sendUnsetMapFlag(p)
        return nil
    }

    p.lastUseItem = useObj
    p.lastUseSlot = useSlot

    p.ClearPendingAction()
    p.SetInteraction(InteractionEngine, loc, targetOpLocU, -1)
    p.targetSubject.typ = loc.Type()
    p.targetSubject.x = loc.X
    p.targetSubject.z = loc.Z
    p.targetSubject.level = loc.Level
    return nil
}
```

### 5.5 `handlers_game.go` wiring

Append near existing OpLoc wires:

```go
gameHandlers[9] = handleOpLocT  // OPLOCT
gameHandlers[75] = handleOpLocU // OPLOCU
```

### 5.6 `ActivePlayer.TargetSubjectCom` accessor

**`pkg/script/active.go`** — append to `ActivePlayer` interface:

```go
// TargetSubjectCom returns the com-component value stored at click
// time by OpLocT / OpLocT-style handlers. For OpLocT it's spellCom;
// for OpLoc1..5 and OpLocU it's -1. Allows APLOCT scripts to read
// which spell the player cast via @spellcom-style script variables.
TargetSubjectCom() int
```

**`modules/world/player_script.go`** — append:

```go
// TargetSubjectCom implements script.ActivePlayer.TargetSubjectCom.
func (p *Player) TargetSubjectCom() int { return p.targetSubject.com }
```

No script opcode is wired in S6m for reading this; future sub-spec wires `@spellcom` when RuneScript infrastructure catches up.

### 5.7 Fire dispatch helper

Append to `modules/world/interaction_trigger.go`:

```go
// apLocTriggerForOp returns the APLOC trigger for the player's
// targetOp sentinel. Returns ok=false if op is neither 1..5 nor a T/U
// sentinel (6, 7). fireOpTriggerLoc derives the OPLOC trigger by
// adding 7 to the returned APLOC (TS Player.ts:~997 offset convention).
func apLocTriggerForOp(op int) (script.ServerTriggerType, bool) {
    switch {
    case op >= 1 && op <= 5:
        return script.TriggerApLoc1 + script.ServerTriggerType(op-1), true
    case op == targetOpLocT:
        return script.TriggerApLocT, true
    case op == targetOpLocU:
        return script.TriggerApLocU, true
    default:
        return 0, false
    }
}
```

### 5.8 `fireApTriggerLoc` + `fireOpTriggerLoc` extension

Both functions currently compute `trigger := script.TriggerXxxLoc1 + ServerTriggerType(op-1)` after checking `op < 1 || op > 5`. Replace both sites:

**`fireApTriggerLoc`** (existing, at `~line 240-250`):
```go
op := p.targetOp
if op < 1 || op > 5 {
    p.ClearInteraction()
    p.interactionFired = true
    return
}

trigger := script.TriggerApLoc1 + script.ServerTriggerType(op-1)
```

→

```go
trigger, ok := apLocTriggerForOp(p.targetOp)
if !ok {
    p.ClearInteraction()
    p.interactionFired = true
    return
}
```

**`fireOpTriggerLoc`** (existing, at `~line 120-130`):
```go
op := p.targetOp
if op < 1 || op > 5 {
    p.ClearInteraction()
    p.interactionFired = true
    return
}

trigger := script.TriggerOpLoc1 + script.ServerTriggerType(op-1)
```

→

```go
apTrigger, ok := apLocTriggerForOp(p.targetOp)
if !ok {
    p.ClearInteraction()
    p.interactionFired = true
    return
}
trigger := apTrigger + 7 // APLOC→OPLOC offset per TS Player.ts:~997
```

Everything downstream (lifecycle gate, category lookup, script dispatch, persistence contract) is unchanged.

## 6. Test Plan

### 6.1 `modules/world/interaction_test.go` — SetInteraction (2 tests)

| # | Test | Asserts |
|---|---|---|
| 1 | `TestSetInteractionStoresComField` | `SetInteraction(..., spellCom)` writes `p.targetSubject.com == spellCom` |
| 2 | `TestSetInteractionPassesMinusOneForNonComOps` | Backwards-compat: existing caller pattern `-1` → `p.targetSubject.com == -1` |

### 6.2 `modules/world/handler_oploc_test.go` — OpLocT + OpLocU (12 tests)

**OpLocT (6 tests):**

| # | Test | Asserts |
|---|---|---|
| 3 | `TestHandleOpLocTSetsInteraction` | valid payload → `p.target==loc`, `p.targetOp==6`, `p.targetSubject.com==spellCom`, typ/x/z/level snapshot correct |
| 4 | `TestHandleOpLocTDelayedPlayerRejected` | delayed → UnsetMapFlag, no state change |
| 5 | `TestHandleOpLocTShortPayloadRejected` | < 8 bytes → UnsetMapFlag |
| 6 | `TestHandleOpLocTOutOfViewportRejected` | dx > 52 → UnsetMapFlag |
| 7 | `TestHandleOpLocTMissingLocRejected` | Server.GetLoc returns nil → UnsetMapFlag |
| 8 | `TestHandleOpLocTMissingLocTypeRejected` | locType unregistered → UnsetMapFlag |

**OpLocU (6 tests):**

| # | Test | Asserts |
|---|---|---|
| 9 | `TestHandleOpLocUSetsInteraction` | valid payload → `p.target==loc`, `p.targetOp==7`, `p.lastUseItem==useObj`, `p.lastUseSlot==useSlot`, `p.targetSubject.com==-1` |
| 10 | `TestHandleOpLocUDelayedPlayerRejected` | delayed → UnsetMapFlag, `lastUseItem` unchanged |
| 11 | `TestHandleOpLocUShortPayloadRejected` | < 12 bytes → UnsetMapFlag |
| 12 | `TestHandleOpLocUOutOfViewportRejected` | dx > 52 → UnsetMapFlag |
| 13 | `TestHandleOpLocUMissingLocRejected` | nil loc → UnsetMapFlag |
| 14 | `TestHandleOpLocUMissingLocTypeRejected` | nil locType → UnsetMapFlag |

### 6.3 `modules/world/interaction_trigger_test.go` — Fire dispatch (6 tests)

| # | Test | Asserts |
|---|---|---|
| 15 | `TestApLocTriggerForOpValidValues` | table-test: 1..5 → TriggerApLoc1..5; 6 → TriggerApLocT; 7 → TriggerApLocU |
| 16 | `TestApLocTriggerForOpInvalidValues` | op=0, 8, -1 → ok==false |
| 17 | `TestFireOpTriggerLocFiresOpLocTTrigger` | targetOp=6 + OPLOCT registered → script fires, ActiveLoc set |
| 18 | `TestFireOpTriggerLocFiresOpLocUTrigger` | targetOp=7 + OPLOCU registered → script fires |
| 19 | `TestFireApTriggerLocFiresApLocTTrigger` | targetOp=6, in apRange, APLOCT registered → script fires |
| 20 | `TestFireApTriggerLocFiresApLocUTrigger` | targetOp=7, in apRange, APLOCU registered → script fires |

**Total: 20 new tests.** Plus mechanical `, -1` argument migration across ~19 existing test call-sites.

## 7. Task Split

### Task 1 — `SetInteraction` signature + `targetSubject.com` + `TargetSubjectCom` + sentinels

Foundational. Mechanical across many files.

- `modules/world/interaction.go` — `SetInteraction` gains `com int`; sentinels `targetOpLocT=6` / `targetOpLocU=7`
- `modules/world/player.go` — `targetSubject` re-expands with `com int`
- `modules/world/player_script.go` — `*Player.TargetSubjectCom()`
- `pkg/script/active.go` — `ActivePlayer.TargetSubjectCom()`
- **19 `SetInteraction` call sites** — mechanical `, -1` argument addition
- 2 new tests

Build green at commit. Commit: `feat(world): SetInteraction com param + targetSubject.com + sentinels (S6m-1)`

### Task 2 — `handleOpLocT` + `handleOpLocU` + opcode wiring

- `modules/world/handler_oploc.go` — both handlers
- `modules/world/handlers_game.go` — opcode wires
- `modules/world/handler_oploc_test.go` — 12 validation tests
- Depends on Task 1 for the new `SetInteraction` signature + sentinels

Commit: `feat(world): handleOpLocT + handleOpLocU + 12 validation tests (S6m-2)`

### Task 3 — Fire dispatch helper + `fireApTriggerLoc` / `fireOpTriggerLoc` extension

- `modules/world/interaction_trigger.go` — `apLocTriggerForOp` helper; swap inline switches in both fire functions
- `modules/world/interaction_trigger_test.go` — 6 fire tests
- After commit: APLOCT/OPLOCT/APLOCU/OPLOCU scripts fire end-to-end

Commit: `feat(world): fireXxxTriggerLoc dispatch for T/U variants (S6m-3)`

## 8. Deviations from TS — Complete Summary

### New deviations introduced by S6m

| ID | TS behavior | goscape S6m | Reason | Follow-up |
|---|---|---|---|---|
| **S6m-D1** | `OpLocTHandler.ts:~25-35` validates spellCom → visible `ComActionTarget.LOC` component | Skip — accept any spellCom | No component registry yet | "component registry + ComActionTarget validation" sub-spec |
| **S6m-D2** | `OpLocUHandler.ts:~25-35` validates useCom → usable, visible component | Skip — discard useCom | Same as D1 | Same follow-up |
| **S6m-D3** | `OpLocUHandler.ts:~45-70` listener-lookup by useCom + slot-bounds + item-at-slot match | Skip — raw wire values trusted | `invListeners` is slice not keyed map | "InvListener keyed-map refactor + OpLocU item validation" sub-spec |
| **S6m-D4** | `OpLocUHandler.ts:~71-77` members-only items against NODE_MEMBERS config | Skip — no members gate | No members-config surface yet | "members-config + item-gating" sub-spec |

### S6j deviation status after S6m — milestone

| ID | Status | Notes |
|---|---|---|
| S6j-D1 | ✅ CLOSED in S6k | Per-op validation gate |
| S6j-D2 | ✅ CLOSED for Loc in S6l | APLOC path wired |
| S6j-D3 | Still open (convention) | `targetOp` 1..5/6/7 sentinel; no follow-up planned |
| S6j-D4 | Still open (defensive) | `locStillValid` zone check; no follow-up needed |
| **S6j-D5** | ✅ **CLOSED in S6m** | OpLocT + OpLocU handlers shipped |
| S6j-D6 | ✅ PARTIAL: apRange closed S6l; focus/camera still deferred | |
| S6j-D7 | ✅ CLOSED in S6k | defaultOp message |

**After S6m: all 7 S6j deviations are either closed, storage-convention defensives, or documented-with-infra-dependency follow-ups.** The OPLOC click surface is TS-faithful end-to-end.

## 9. Scope Estimate

- **Implementation:** ~310 LOC across 7 files
- **Tests:** ~300 LOC (20 new tests + 19 mechanical call-site migrations)
- **Commits:** 3 (one per task)
- **Build/test green:** at every commit (Task 1's signature change batches with all 19 call-site updates)
- **End-to-end gain:** spell-on-loc and item-on-loc clicks route correctly; APLOCT/OPLOCT/APLOCU/OPLOCU scripts fire

## 10. Out-of-Scope Reminders

Explicitly NOT in S6m (each tracked in §8 as a future sub-spec):

- Component registry + ComActionTarget validation (S6m-D1, S6m-D2)
- InvListener keyed-map refactor + OpLocU item-match validation (S6m-D3)
- Members-config + members-item gating (S6m-D4)
- `@spellcom` / script-side reading of targetSubject.com (interface ready, opcode not yet wired)
- OpNpcT sibling (NPC version of OpLocT) — future sub-spec
- APNPC approach-range gating (S6l-D2, still open)
- `p_op*` / nextTarget re-anchor opcodes (S6l-D5)
