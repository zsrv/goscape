# S6o — OpNpcT + OpNpcU Handler Design

> **Sub-spec context:** Fifteenth sub-spec in the runescript-s* series. Closes S6n-D1 by wiring OpNpcT (spell-on-NPC) and OpNpcU (item-on-NPC) sibling opcodes end-to-end. NPC parallel to S6m's OpLocT/OpLocU work. Bundles the `fireOpTriggerNpc` helper refactor (using `apNpcTriggerForOp + 7`) as Task 1's natural pair since T/U dispatch requires it.

> **TS-faithfulness gate:** User requires "true to TS." Four new documented deviations (S6o-D1..D4) all mirror S6m's Loc-side deviations at the same validation gates — infra-dependency follow-ups bundle with the S6m parallels.

> **Scope:** Approach 1 (full bundle with distinct NPC sentinels 8/9). Defers DRY refactor of the 4 fire-helpers (now all matured to identical 7-step shape — natural S6p candidate).

## 1. Goal

Wire the two remaining NPC click-opcode variants to fire their respective triggers:

- **OpNpcT** (opcode 134, 4-byte payload): spell-on-NPC. Player drags a spell icon onto an NPC; fires `[apnpct,<npcType>]` / `[opnpct,<npcType>]` scripts.
- **OpNpcU** (opcode 202, 8-byte payload): item-on-NPC. Player drags an inventory item onto an NPC; fires `[apnpcu,<npcType>]` / `[opnpcu,<npcType>]` scripts.

After this sub-spec, the full NPC click surface matches the Loc click surface end-to-end:

| Click type | Opcode | Trigger (AP/OP pair) | Shipped in |
|---|---|---|---|
| OpNpc1..5 | 194, 8, 27, 113, 100 | APNPC1..5 / OPNPC1..5 | S6b (OP), S6n (AP) |
| **OpNpcT** | 134 | APNPCT / OPNPCT | **S6o** |
| **OpNpcU** | 202 | APNPCU / OPNPCU | **S6o** |

## 2. Architecture

Three phases parallel to S6m's OpLoc T/U shape with NPC-specific simplifications (slot-indexed, no in-place mutation risk, simpler handlers).

**Phase A — Sentinel + helper + fire-helper refactor (Task 1):**
- Add `targetOpNpcT = 8`, `targetOpNpcU = 9` constants to `modules/world/interaction.go` sentinel block
- Extend `apNpcTriggerForOp(op)` from 1..5-only to 1..5 + T + U (mirrors `apLocTriggerForOp` shape)
- Refactor `fireOpTriggerNpc` to use `apNpcTriggerForOp + 7` — byte-equivalent for 1..5, gains T/U dispatch for free
- Remove stale `S6n-D1` deviation note from `fireApTriggerNpc` docstring

**Phase B — Handler implementations (Task 2):**
- `handleOpNpcT`: 4-byte payload (slot + spellCom); 5 validation gates; SetInteraction with `targetOpNpcT` + spellCom
- `handleOpNpcU`: 8-byte payload (slot + useObj + useSlot + useCom); 5 validation gates; store useObj/useSlot in `p.lastUseItem`/`lastUseSlot`; SetInteraction with `targetOpNpcU` + -1; useCom discarded

**Phase C — Opcode wiring (Task 2):**
- `gameHandlers[134] = handleOpNpcT` // OPNPCT
- `gameHandlers[202] = handleOpNpcU` // OPNPCU

### Data flow

```
Spell-on-NPC click (spellCom=7777, slot=0):
wire → handleOpNpcT decodes (slot=0, spellCom=7777)
     → 5 gates pass → SetInteraction(Engine, npc, 8, 7777)
     → p.targetSubject.com = 7777
tick N: within npc.typ.AttackRange → tryFireApTrigger → *Npc → fireApTriggerNpc
     → apNpcTriggerForOp(8) → TriggerApNpcT (9)
     → script fires with ActiveNpc=npc; p.TargetSubjectCom() returns 7777
     → Execution Finished → ClearInteraction (no apRangeCalled persistence)

Item-on-NPC click (useObj=1511, useSlot=3, slot=0):
wire → handleOpNpcU decodes → gates pass
     → p.lastUseItem=1511, p.lastUseSlot=3
     → SetInteraction(Engine, npc, 9, -1)
tick N: within attackrange → fireApTriggerNpc
     → apNpcTriggerForOp(9) → TriggerApNpcU (8)
     → script reads item via p.LastUseItem()/LastUseSlot()
     → Finished → ClearInteraction
```

### Key simplifications vs S6m Loc T/U

| Gate / Behavior | Loc T/U (S6m) | NPC T/U (S6o) |
|---|---|---|
| Payload size | 8 / 12 bytes | 4 / 8 bytes |
| Coordinate decode | x, z, locId | slot only |
| Viewport 52-tile check | Required | Skipped — slot-indexed |
| `Server.GetLoc` lookup | Required | Skipped — direct `s.npcs[slot]` |
| `targetSubject.{typ,x,z,level}` snapshot | Required (lifecycle gate) | Skipped — NPCs have `npc.dead` flag |
| Per-op validation gate (`locType.Op[N]`) | S6k required for 1..5 | Skipped for T/U per TS |

NPC handlers are ~50% smaller than Loc counterparts because NPCs don't need coord validation, zone lookup, or lifecycle snapshot.

## 3. File Map

| File | Action | Purpose |
|---|---|---|
| `modules/world/interaction.go` | Modify | Add `targetOpNpcT=8`, `targetOpNpcU=9` in sentinels block |
| `modules/world/interaction_trigger.go` | Modify | Extend `apNpcTriggerForOp` with T/U cases; refactor `fireOpTriggerNpc` to use helper+7; remove S6n-D1 note from `fireApTriggerNpc` |
| `modules/world/interaction_trigger_test.go` | Modify | Extend `apNpcTriggerForOp` table tests with T/U; add 4 fire-dispatch tests (AP+OP × T+U) |
| `modules/world/handler_opnpc.go` | Modify | Add `handleOpNpcT` + `handleOpNpcU` |
| `modules/world/handler_opnpc_test.go` | Modify | 12 validation tests (6 per handler) |
| `modules/world/handlers_game.go` | Modify | Wire `gameHandlers[134]=handleOpNpcT`, `gameHandlers[202]=handleOpNpcU` |

**Existing infrastructure leveraged (no changes needed):**
- OPNPCT=134 (4 bytes), OPNPCU=202 (8 bytes) — `pkg/io/protocol/game/client/prot.go:65-66`
- TriggerApNpcT=9, TriggerApNpcU=8, TriggerOpNpcT=16, TriggerOpNpcU=15 — `pkg/script/trigger.go:17-25`
- `Player.lastUseItem`/`lastUseSlot` fields — `player.go:175` (S6m consumer)
- `targetSubject.com` field + `TargetSubjectCom()` accessor — S6m plumbing
- `SetInteraction(kind, target, op, com int)` — S6m signature (unchanged)
- `apNpcTriggerForOp` + `fireApTriggerNpc` (S6n) — apNpcTriggerForOp extended; fireApTriggerNpc unchanged (gets T/U for free)
- `fireOpTriggerNpc` (S6j) — refactored to use helper+7

## 4. TS Reference Map

- **OpNpcTHandler:** `src/network/game/client/handler/OpNpcTHandler.ts` — single APNPCT trigger; stores spellComId via `setInteraction(Engine, npc, APNPCT, spellComId)`
- **OpNpcUHandler:** `src/network/game/client/handler/OpNpcUHandler.ts` — single APNPCU trigger; stores useObj/useSlot on player
- **Payloads:** `OpNpcTDecoder.ts` (4 bytes: slot G2, spellCom G2), `OpNpcUDecoder.ts` (8 bytes: +useObj, useSlot, useCom — all G2)
- **Trigger offset:** APNPC+7=OPNPC verified numerically (9→16, 8→15)

## 5. Component Details

### 5.1 Sentinel constants

In `modules/world/interaction.go:24-27`, extend existing block:

```go
// Sentinel targetOp values for non-op-numbered T/U interaction variants.
// OpLoc1..5/OpNpc1..5 use op = 1..5 (the op slot clicked); T/U variants
// use these sentinels so fireXxxTriggerYyy can dispatch to the correct
// single-trigger (e.g. APLOCT, OPNPCU). The targetOp interpretation is
// per-entity-type: tryFireXxxTrigger type-switches on p.target first,
// then each branch reads targetOp independently.
const (
	targetOpLocT = 6 // APLOCT / OPLOCT dispatch marker
	targetOpLocU = 7 // APLOCU / OPLOCU dispatch marker
	targetOpNpcT = 8 // APNPCT / OPNPCT dispatch marker (S6o)
	targetOpNpcU = 9 // APNPCU / OPNPCU dispatch marker (S6o)
)
```

### 5.2 `apNpcTriggerForOp` T/U extension

In `modules/world/interaction_trigger.go` around line 199, replace:

```go
func apNpcTriggerForOp(op int) (script.ServerTriggerType, bool) {
	if op >= 1 && op <= 5 {
		return script.TriggerApNpc1 + script.ServerTriggerType(op-1), true
	}
	return 0, false
}
```

With:

```go
// apNpcTriggerForOp returns the APNPC trigger for the player's
// targetOp. fireOpTriggerNpc derives the OPNPC trigger by adding 7
// (TS Player.ts:~997 offset convention):
//
//	APNPC1..5 (3..7) + 7 → OPNPC1..5 (10..14)
//	APNPCT    (9)    + 7 → OPNPCT    (16)
//	APNPCU    (8)    + 7 → OPNPCU    (15)
//
// NPC variant of apLocTriggerForOp. Parallel shape after S6o: 1..5
// ops + T/U sentinels. Returns ok=false for invalid op.
func apNpcTriggerForOp(op int) (script.ServerTriggerType, bool) {
	switch {
	case op >= 1 && op <= 5:
		return script.TriggerApNpc1 + script.ServerTriggerType(op-1), true
	case op == targetOpNpcT:
		return script.TriggerApNpcT, true
	case op == targetOpNpcU:
		return script.TriggerApNpcU, true
	default:
		return 0, false
	}
}
```

### 5.3 `fireOpTriggerNpc` refactor

Current at `interaction_trigger.go:49-94` has this block:

```go
	op := p.targetOp
	if op < 1 || op > 5 {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	trigger := script.TriggerOpNpc1 + script.ServerTriggerType(op-1)
```

Becomes:

```go
	apTrigger, ok := apNpcTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}
	trigger := apTrigger + 7 // APNPC→OPNPC offset per TS Player.ts:~997
```

Byte-equivalent for 1..5 (`TriggerApNpc1 + (op-1) + 7 = TriggerOpNpc1 + (op-1)`); gains T/U dispatch. Everything else in `fireOpTriggerNpc` unchanged (npc.dead gate, category lookup, script init, resumeOrFinish, terminal ClearInteraction).

### 5.4 `fireApTriggerNpc` comment cleanup

Remove the stale S6n-D1 deviation block from `fireApTriggerNpc`'s docstring:

```
// DEVIATION S6n-D1: APNPC T/U sentinels not wired. OpNpcT/OpNpcU
// handlers don't exist in goscape yet; when they land,
// apNpcTriggerForOp gains matching cases and this fire function
// needs a sentinel-aware op-range gate update.
```

Function body unchanged — `apNpcTriggerForOp` handles T/U internally after Task 1.

### 5.5 `handleOpNpcT`

Append to `modules/world/handler_opnpc.go`:

```go
// handleOpNpcT is the handler for OPNPCT (opcode 134, 4-byte payload).
// Spell-on-NPC: player drags a spell icon onto an NPC.
// Payload = (slot:G2, spellCom:G2).
//
// Validation gates (mirrors TS OpNpcTHandler.ts):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. slot out of range → UnsetMapFlag
//  4. NPC nil or dead → UnsetMapFlag
//  5. NpcType nil → UnsetMapFlag
//
// DEVIATION S6o-D1: TS validates spellCom references a component
// with ComActionTarget.NPC flag AND that the component is visible in
// the player's interface stack. Skipped here because goscape has no
// component registry yet. Effective risk: client can forge spellCom
// values; scripts reading p.TargetSubjectCom() get raw wire values.
// Follow-up: "component registry + ComActionTarget validation"
// sub-spec (bundle with S6m-D1).
//
// Unlike OpNpc1..5 (handler_opnpc.go:40-44), there is NO per-op
// validation gate — T/U variants don't index into NpcType.Op.
//
// No targetSubject.{typ,x,z,level} snapshot — NPCs have no in-place
// mutation risk (unlike Loc's packed Info bitfield). npc.dead is the
// lifecycle gate, checked at fire time (S6n fireApTriggerNpc).
//
// On success: ClearPendingAction → SetInteraction(Engine, npc,
// targetOpNpcT, spellCom).
func handleOpNpcT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 4 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	slot := int(r.G2())
	spellCom := int(r.G2())

	if slot < 0 || slot >= len(s.npcs) {
		sendUnsetMapFlag(p)
		return nil
	}
	npc := s.npcs[slot]
	if npc == nil || npc.dead {
		sendUnsetMapFlag(p)
		return nil
	}
	if npc.typ == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, npc, targetOpNpcT, spellCom)
	return nil
}
```

### 5.6 `handleOpNpcU`

Append to `modules/world/handler_opnpc.go`:

```go
// handleOpNpcU is the handler for OPNPCU (opcode 202, 8-byte payload).
// Item-on-NPC: player drags an inventory item onto an NPC (e.g., feed
// pet, give gift, sacrifice item).
// Payload = (slot:G2, useObj:G2, useSlot:G2, useCom:G2).
//
// Validation gates (subset of TS OpNpcUHandler.ts):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. slot out of range → UnsetMapFlag
//  4. NPC nil or dead → UnsetMapFlag
//  5. NpcType nil → UnsetMapFlag
//
// DEVIATION S6o-D2: TS validates useCom references a usable, visible
// interface component. Skipped — no component registry. (Mirrors S6m-D2.)
//
// DEVIATION S6o-D3: TS does an inventory-listener lookup by useCom +
// slot-bounds + item-at-slot-matches-useObj validation. Goscape's
// invListeners is a slice not keyed map, so this lookup shape doesn't
// translate. Skip; scripts reading p.LastUseItem()/p.LastUseSlot() get
// raw wire values. (Mirrors S6m-D3.)
//
// DEVIATION S6o-D4: TS checks members-only items against NODE_MEMBERS
// config. Skipped — no members-config surface. (Mirrors S6m-D4.)
//
// On success: set p.lastUseItem=useObj, p.lastUseSlot=useSlot →
// ClearPendingAction → SetInteraction(Engine, npc, targetOpNpcU, -1).
func handleOpNpcU(p *Player, payload []byte) error {
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
	slot := int(r.G2())
	useObj := int(r.G2())
	useSlot := int(r.G2())
	_ = int(r.G2()) // useCom — deliberately discarded (S6o-D2/D3)

	if slot < 0 || slot >= len(s.npcs) {
		sendUnsetMapFlag(p)
		return nil
	}
	npc := s.npcs[slot]
	if npc == nil || npc.dead {
		sendUnsetMapFlag(p)
		return nil
	}
	if npc.typ == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.lastUseItem = useObj
	p.lastUseSlot = useSlot

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, npc, targetOpNpcU, -1)
	return nil
}
```

### 5.7 Opcode wiring

Append to `modules/world/handlers_game.go` near the existing OPNPC1..5 block:

```go
gameHandlers[134] = handleOpNpcT // OPNPCT
gameHandlers[202] = handleOpNpcU // OPNPCU
```

## 6. Test Plan

### 6.1 Task 1 tests (~6 new/modified tests)

**`modules/world/interaction_trigger_test.go`:**

| # | Test | Asserts |
|---|---|---|
| 1 | `TestApNpcTriggerForOpValidValues` (extended) | Add rows `{targetOpNpcT, TriggerApNpcT}`, `{targetOpNpcU, TriggerApNpcU}`. Existing 1..5 preserved. |
| 2 | `TestApNpcTriggerForOpInvalidValues` (modified) | Remove `8` from invalid list (now valid via T); keep `{0, 6, 7, -1, 100, -100}` |
| 3 | `TestFireOpTriggerNpcFiresOpNpcTTrigger` | `targetOp=targetOpNpcT` + OPNPCT script → fires, ActiveNpc bound, ClearInteraction after Finished |
| 4 | `TestFireOpTriggerNpcFiresOpNpcUTrigger` | `targetOp=targetOpNpcU` + OPNPCU script → fires |
| 5 | `TestFireApTriggerNpcFiresApNpcTTrigger` | `targetOp=targetOpNpcT` at approach distance + APNPCT → fires |
| 6 | `TestFireApTriggerNpcFiresApNpcUTrigger` | `targetOp=targetOpNpcU` at approach + APNPCU → fires |

Regression: existing S6j `TestTryFireOpTrigger_HappyPath` and S6n `TestFireApTriggerNpcScriptFires` must continue passing (byte-equivalent refactor for 1..5).

### 6.2 Task 2 tests (12 new tests)

**`modules/world/handler_opnpc_test.go`:**

**OpNpcT (6 tests):**

| # | Test | Asserts |
|---|---|---|
| 7 | `TestHandleOpNpcTSetsInteraction` | valid → `p.target==npc`, `p.targetOp==targetOpNpcT`, `p.targetSubject.com==spellCom` |
| 8 | `TestHandleOpNpcTDelayedPlayerRejected` | delayed → UnsetMapFlag, no state change |
| 9 | `TestHandleOpNpcTShortPayloadRejected` | < 4 bytes → UnsetMapFlag |
| 10 | `TestHandleOpNpcTInvalidSlotRejected` | slot OOB → UnsetMapFlag |
| 11 | `TestHandleOpNpcTDeadNpcRejected` | `npc.dead=true` → UnsetMapFlag |
| 12 | `TestHandleOpNpcTMissingNpcTypeRejected` | `npc.typ=nil` → UnsetMapFlag |

**OpNpcU (6 tests):**

| # | Test | Asserts |
|---|---|---|
| 13 | `TestHandleOpNpcUSetsInteraction` | valid → `p.target==npc`, `p.targetOp==targetOpNpcU`, `p.lastUseItem==useObj`, `p.lastUseSlot==useSlot`, `p.targetSubject.com==-1` |
| 14 | `TestHandleOpNpcUDelayedPlayerRejected` | delayed → UnsetMapFlag + `lastUseItem` unchanged (leak-prevention) |
| 15 | `TestHandleOpNpcUShortPayloadRejected` | < 8 bytes → UnsetMapFlag |
| 16 | `TestHandleOpNpcUInvalidSlotRejected` | slot OOB → UnsetMapFlag |
| 17 | `TestHandleOpNpcUDeadNpcRejected` | dead NPC → UnsetMapFlag |
| 18 | `TestHandleOpNpcUMissingNpcTypeRejected` | `npc.typ=nil` → UnsetMapFlag |

**Total: ~18 new/modified tests.**

## 7. Task Split

### Task 1 — Sentinels + `apNpcTriggerForOp` T/U + `fireOpTriggerNpc` refactor + comment cleanup

- `modules/world/interaction.go` — 2 new constants
- `modules/world/interaction_trigger.go` — extended helper + refactor + comment cleanup
- `modules/world/interaction_trigger_test.go` — 2 test modifications + 4 fire-dispatch tests

Build green throughout. 1..5 paths byte-equivalent; T/U dispatch works in principle (no handler sets those sentinels yet).

Commit: `feat(world): apNpcTriggerForOp T/U extension + fireOpTriggerNpc refactor (S6o-1)`

### Task 2 — `handleOpNpcT` + `handleOpNpcU` + opcode wiring

- `modules/world/handler_opnpc.go` — 2 new handlers (~100 LOC total)
- `modules/world/handlers_game.go` — 2 opcode wires
- `modules/world/handler_opnpc_test.go` — 12 validation tests
- After commit: spell-on-NPC / item-on-NPC clicks route + APNPCT/OPNPCT/APNPCU/OPNPCU scripts fire end-to-end

Commit: `feat(world): handleOpNpcT + handleOpNpcU + 12 validation tests (S6o-2)`

## 8. Deviations from TS — Complete Summary

### New deviations in S6o (all mirror S6m's Loc T/U deviations)

| ID | TS behavior | goscape S6o | Reason | Follow-up |
|---|---|---|---|---|
| **S6o-D1** | `OpNpcTHandler.ts` validates spellCom → visible `ComActionTarget.NPC` component | Skip — accept any spellCom | No component registry | "component registry + ComActionTarget validation" sub-spec (bundle with S6m-D1/D2) |
| **S6o-D2** | `OpNpcUHandler.ts` validates useCom → usable, visible component | Skip — discard useCom | Same as D1 | Same follow-up |
| **S6o-D3** | `OpNpcUHandler.ts` listener-lookup + slot-bounds + item-at-slot match | Skip — raw wire values trusted | `invListeners` is slice not keyed map | "InvListener keyed-map refactor" sub-spec (bundle with S6m-D3) |
| **S6o-D4** | Members-only items against NODE_MEMBERS config | Skip — no members gate | No members-config surface | "members-config + item-gating" sub-spec (bundle with S6m-D4) |

### S6n deviation status after S6o

| ID | Status | Notes |
|---|---|---|
| **S6n-D1** | ✅ **CLOSED in S6o** | APNPC T/U sentinels wired |

After S6o, S6n has 0 open deviations.

### Fire-helper DRY refactor — natural S6p candidate

After S6o, all 4 fire-helpers support full 1..5 + T + U:

| Helper | Handles | Uses |
|---|---|---|
| `fireOpTriggerLoc` | 1..5 + T + U | apLocTriggerForOp + 7 (since S6m) |
| `fireOpTriggerNpc` | 1..5 + T + U | apNpcTriggerForOp + 7 (from S6o) |
| `fireApTriggerLoc` | 1..5 + T + U | apLocTriggerForOp (since S6m) |
| `fireApTriggerNpc` | 1..5 + T + U | apNpcTriggerForOp (from S6o) |

All four helpers have the same 7-step skeleton: delayed → lifecycle gate → op-gate → category lookup → trigger lookup → script init → resumeOrFinish → terminal handling. Divergences (lifecycle gate, category source, ActiveXxx binding, persistence contract) are now crystallized — extraction target is clear for S6p.

## 9. Scope Estimate

- **Implementation:** ~140 LOC across 4 files
- **Tests:** ~230 LOC (18 new/modified tests)
- **Commits:** 2 (one per task)
- **Build/test green:** at every commit
- **End-to-end gain:** spell-on-NPC and item-on-NPC clicks route + APNPCT/OPNPCT/APNPCU/OPNPCU scripts fire

## 10. Out-of-Scope Reminders

Explicitly NOT in S6o:

- Component registry + ComActionTarget validation (S6m-D1/D2 + S6o-D1/D2 — bundled)
- InvListener keyed-map refactor + item-in-slot validation (S6m-D3 + S6o-D3 — bundled)
- Members-config surface (S6m-D4 + S6o-D4 — bundled)
- **4 fire-helpers DRY refactor** — all matured to identical 7-step shape after S6o; natural S6p candidate
- `@spellcom` script opcode (consumer of `TargetSubjectCom()`)
- `p_op*` / `nextTarget` re-anchor opcodes (S6l-D5)
- S6l-D1/D3/D4 (small optimizations + infra deferrals)
- S6j-D6 focus/camera half
- Retire Npc.ShowHit symmetry cleanup
- Lookup-key bit-packing helpers
- Large multi-sub-spec scopes (Combat-level / save-file / NPC AI / session-log)
