# NAI-136 — Run-weight propagation (calculateRunWeight + UpdateRunWeight wire)

**Status:** spec
**Date:** 2026-05-09
**Predecessors:** NAI-135 (run-mode visible-effect wiring) closed at `c3f078f`. NAI-135 carryover queue named this candidate verbatim: "weight not propagated from inventory."
**Tech stack:** Go 1.26+
**Cadence:** Compressed (combined spec+plan, single-dispatch implementer + final Sonnet code-reviewer per `compressed_cadence`).

## 1. Goal

Port TS `Player.calculateRunWeight` + the `NetworkPlayer.updateInvs` runweight-tracking branch + the `UpdateRunWeight` wire packet so that:

1. Equipping or stowing a non-stackable weighted item updates `p.runweight` to the correct sum of `ObjType.Weight × Count` over all `InvType.RunWeight=true` invs.
2. The "Weight Carried" UI reflects the current `runweight / 1000` kg, on the same tick as the inventory change.
3. The drain branch of `(*Player).updateEnergy` (NAI-135 `modules/world/player_run.go:38`) reads a meaningful `p.runweight` instead of always-zero, restoring TS-faithful run-energy drain rates.

Closes the NAI-135 PRIMARY-met / SECONDARY-residual: `p.runweight` is only ever assigned in tests at HEAD `c3f078f`. The drain-formula tests pin behavior at fixed `runweight` values, but the production path never recomputes `p.runweight` after an inventory change, so the UI shows 0 kg and drain stays at the lowest-weight rate.

## 2. TS source — anchored

- **`Player.calculateRunWeight`**: `Engine-TS/src/engine/entity/Player.ts:598-627`.
- **`NetworkPlayer.updateInvs` runweight branch**: `Engine-TS/src/engine/entity/NetworkPlayer.ts:337-394` (lines 338-339 add the two locals; 372-381 set the per-player tracking; 385-389 recompute + skip-on-no-change; 391-393 emit).
- **`UpdateRunWeight` model**: `Engine-TS/src/network/game/server/model/UpdateRunWeight.ts` — single field `kg: number`.
- **`UpdateRunWeightEncoder`**: `Engine-TS/src/network/game/server/codec/UpdateRunWeightEncoder.ts` — `buf.p2(message.kg)`.
- **Wire opcode**: `Engine-TS/src/network/game/server/ServerGameProt.ts:55` — `UPDATE_RUNWEIGHT = new ServerGameProt(22, 2)` (opcode 22, 2-byte payload).
- **Player field**: `Engine-TS/src/engine/entity/Player.ts:291` — `runweight: number = 0`.
- **InvType flag**: `Engine-TS/src/cache/config/InvType.ts:81` — `runweight = false; // inv contributes to weight`.

## 3. Non-goals

- No `RUNWEIGHT` script opcode handler. TS `PlayerOps.ts:1181` defines `state.pushInt(state.activePlayer.runweight)`; goscape's `OpRunWeight` opcode-id existence and dispatch wiring are out of scope. (If a `runweight` script consumer surfaces in smoke, route to NAI-137+.)
- No retire of `dispatch_correct_reach_blocked` SECONDARY pieces from earlier NAI sub-specs (firemaking ashes, LOWMEM trace, P_TELEJUMP, weapon-equip rendering, combat-init cascade) — all remain queued.
- No retire of NAI-115-D1/D2 deviations.
- No engine-side broadcasting / weight events / weight-bucketing — single per-tick `UpdateRunWeight` per player only when the runweight value (or first-seen condition) changes.
- No script-side `update_weight` proc — TS does NOT have a content `[proc,update_weight]`; the carryover note's mention of one was a misframing. The recompute is purely engine-side, triggered by inv-listener-update tick events.
- Smoke harness work is user-driven per `smoke_test_server_handoff`.

## 4. Architecture

NAI-136 adds three things across two existing files plus one new file:

```
pkg/io/protocol/game/server/prot.go       OpUpdateRunWeight constant (new line)
modules/world/stat_update.go              sendUpdateRunWeight helper (new func)
modules/world/player_runweight.go         (*Player).calculateRunWeight (NEW FILE)
modules/world/player.go                   updateInvs extended (~15 LOC delta @ :778-815)
```

**No new struct types, no new interfaces, no new ScriptState fields, no script-handler ports.** All TS prerequisites have goscape analogs already:

| TS surface | Goscape surface | Source |
|---|---|---|
| `InvType.runweight` | `objtype.InvType.RunWeight` | `pkg/objtype/invtype.go:27` |
| `ObjType.weight` (grams) | `objtype.ObjType.Weight` (grams) | `pkg/objtype/objtype.go:131` |
| `Player.runweight` | `Player.runweight` | `modules/world/player.go:203` |
| `Player.invs` map | `Player.invs map[int]*inventory.Inventory` | `modules/world/player.go:293` |
| `InvListener.firstSeen` | `InventoryListener.FirstSeen` | (existing; used at `player.go:801`, `:1013`) |
| `World.getInventory` lazy-alloc | `srv.invLookup.Get` (NAI-118) | `modules/world/server_invs.go:15-47` |
| `UpdateRunEnergy` packet | `OpUpdateRunEnergy` + `sendUpdateRunEnergy` | `prot.go:57`, `stat_update.go:20` |

**Tick-ordering**: `Player.updateInvs` is called from `processOut` (post-NAI-93). The new `sendUpdateRunWeight` rides the same per-tick wire batch as the inv-update packets that triggered it — landing in the client at the same wire frame as the `UpdateInvFull` deltas that caused the recompute. No re-ordering of existing packets needed.

```
Per-tick wire-emission sequence (per player, in updateInvs)
──────────────────────────────
for listener in invListeners:
  if Source == -1:                          // SCOPE_SHARED — TS world-inv branch
    UpdateInvFull(com, inv) if inv.Update or FirstSeen
    // (no runweight tracking; TS NetworkPlayer.ts:354-357)
  else:                                     // per-player listener
    UpdateInvFull(com, inv) if inv.Update or FirstSeen
    if emitted:
      if FirstSeen: firstSeen = true        // forces UpdateRunWeight emit
      if InvType.RunWeight: runWeightChanged = true
clear inv.Update for all observed
if runWeightChanged:
  before := p.runweight
  p.calculateRunWeight()
  runWeightChanged = before != p.runweight  // skip-on-no-change
if runWeightChanged or firstSeen:
  sendUpdateRunWeight(p, p.runweight / 1000)
```

## 5. Implementation — code blocks

### 5.1 Wire layer

**`pkg/io/protocol/game/server/prot.go`** — add immediately after `OpUpdateRunEnergy` (line 57):

```go
// Per-player run-weight (kg). Emitted from NetworkPlayer.updateInvs when an
// inv with RunWeight=true is dirtied or first-seen. Mirrors TS
// ServerGameProt.UPDATE_RUNWEIGHT (opcode 22, 2-byte payload).
OpUpdateRunWeight = Op{Opcode: 22, PayloadSize: 2}
```

**`modules/world/stat_update.go`** — add immediately below `sendUpdateRunEnergy` (line 20):

```go
// sendUpdateRunWeight writes one UpdateRunWeight packet (kg field is
// the truncated runweight/1000). Mirrors TS UpdateRunWeightEncoder
// (`buf.p2(kg)`). Negative kg is signed-16-bit-encoded for parity with
// TS p2 (which is signed); in practice kg is always >= 0 since
// calculateRunWeight sums non-negative ObjType.Weight values.
//
// NAI-136.
func sendUpdateRunWeight(p *Player, kg int) {
    buf := packet.NewPacket(nil)
    buf.P2(uint16(int16(kg)))
    p.writeOut(gameserver.OpUpdateRunWeight, buf.Bytes())
}
```

### 5.2 calculateRunWeight (new file)

**`modules/world/player_runweight.go`** — new:

```go
package world

// calculateRunWeight recomputes p.runweight from all RunWeight=true
// invs that the player owns, summing ObjType.Weight * Count over
// non-stackable items only. Stackable items contribute 0 (TS Player.ts:620-622).
//
// Mirrors TS Engine-TS/src/engine/entity/Player.ts:598-627
// line-for-line. Called from (*Player).updateInvs when a
// runweight-flagged listener fires; also safe to call directly for
// tests / forced recompute.
//
// Defensive nil-server guard (goscape defensive; TS uses static
// InvType/ObjType imports).
//
// NAI-136.
func (p *Player) calculateRunWeight() {
    p.runweight = 0
    if p.client == nil || p.client.server == nil {
        return
    }
    srv := p.client.server
    if srv.invTypes == nil || srv.objTypes == nil {
        return
    }
    invConfigs := srv.invTypes.Configs
    objConfigs := srv.objTypes.Configs
    for _, inv := range p.invs {
        if inv == nil {
            continue
        }
        if inv.Type < 0 || inv.Type >= len(invConfigs) {
            continue
        }
        invType := invConfigs[inv.Type]
        if invType == nil || !invType.RunWeight {
            continue
        }
        for slot := 0; slot < inv.Capacity; slot++ {
            item := inv.Get(slot)
            if item == nil {
                continue
            }
            if item.Id < 0 || item.Id >= len(objConfigs) {
                continue
            }
            objType := objConfigs[item.Id]
            if objType == nil || objType.Stackable {
                continue
            }
            p.runweight += objType.Weight * item.Count
        }
    }
}
```

**Style notes:**
- Direct slice access via `srv.invTypes.Configs[inv.Type]` matches the existing in-Player convention at `player.go:998-1004` (`invListenOnCom`). The `serverConfigsView.InvType(id)` getter is the script-side adapter; not used from `*Player` methods today.
- `inv.Get(slot)` is the TS-mirror call shape (TS `inv.get(slot)`). Direct `inv.Items[slot]` would be one-fewer indirection but loses the bounds check and the TS line-for-line parity.

### 5.3 updateInvs extension

**`modules/world/player.go`** — replace the body of `updateInvs` (currently lines 778-815). Diff against HEAD `c3f078f`:

```go
func (p *Player) updateInvs() {
    if p.client == nil || p.client.server == nil {
        return
    }
    srv := p.client.server
    observed := make([]*inventory.Inventory, 0, len(p.invListeners))
    runWeightChanged := false                            // NEW
    firstSeen := false                                   // NEW
    for com, l := range p.invListeners {
        var self script.ActivePlayer
        if l.Source == -1 {
            self = p
        } else {
            self = srv.LookupPlayerByUID(l.Source)
            if self == nil {
                continue
            }
        }
        inv := srv.invLookup.Get(self, l.Type)
        if inv == nil {
            continue
        }
        if inv.Update || l.FirstSeen {
            sendUpdateInvFullCom(p, l.Com, inv)
            // NEW: per-player branch only — runweight + firstSeen tracking.
            // TS NetworkPlayer.ts:372-381. SCOPE_SHARED (Source==-1) branch
            // does NOT count toward runWeightChanged or firstSeen — TS
            // line 354-357 falls through after the world-inv emit.
            if l.Source != -1 {
                if l.FirstSeen {
                    firstSeen = true
                }
                if l.Type >= 0 && l.Type < len(srv.invTypes.Configs) {
                    if invType := srv.invTypes.Configs[l.Type]; invType != nil && invType.RunWeight {
                        runWeightChanged = true
                    }
                }
            }
            if l.FirstSeen {
                l.FirstSeen = false
                p.invListeners[com] = l
            }
        }
        observed = append(observed, inv)
    }
    for _, inv := range observed {
        inv.Update = false
    }
    // NEW: TS NetworkPlayer.ts:385-393 — recompute, skip-on-no-change, emit.
    if runWeightChanged {
        before := p.runweight
        p.calculateRunWeight()
        runWeightChanged = before != p.runweight
    }
    if runWeightChanged || firstSeen {
        sendUpdateRunWeight(p, p.runweight/1000)
    }
}
```

**Three TS-fidelity callouts encoded above:**
1. **SCOPE_SHARED skip** (TS line 354-357): the world-inv emit block does not touch the two locals. Goscape ports this with the `if l.Source != -1 { ... }` guard.
2. **firstSeen forces emit even when weight unchanged** (TS line 391): a fresh per-player listener triggers an `UpdateRunWeight` regardless of `runWeightChanged`. This is intentional TS behavior — the client needs the initial weight on first listener registration.
3. **Skip-on-no-change** (TS line 388): after `calculateRunWeight()`, `runWeightChanged` is reset to `before != p.runweight`. Avoids spamming `UpdateRunWeight` every tick when an irrelevant runweight-flagged listener happens to fire but its inv contents didn't actually change weight.

## 6. Tracked deviations

- **DEVIATION-NAI-136-D1** — defensive nil-server / nil-config-table guards in `calculateRunWeight`. Goscape-defensive; TS `InvType.get(...)` / `ObjType.get(...)` are static imports that throw on missing data. Labeled per `defensive_gate_doc_comment_label`.
- **DEVIATION-NAI-136-D2** — bounds-check on `inv.Type` and `item.Id` in `calculateRunWeight` and on `l.Type` in `updateInvs`. Goscape-defensive; TS array-index access on missing config returns `undefined` and is caught by the `if (!invType || !invType.runweight)` / `if (!type || type.stackable)` guards. Goscape's bounds-check is structurally equivalent.

No behavioral deviations from TS for in-bounds, well-formed configs.

## 7. Risks / pre-flight verified at spec-write

| Risk | Verified? | Anchor |
|---|---|---|
| `p.invs` shape | ✅ `map[int]*inventory.Inventory` | `modules/world/player.go:293` |
| `srv.invTypes.Configs` slice access | ✅ pre-existing pattern | `modules/world/player.go:998-1004` (`invListenOnCom`); `modules/world/server_invs.go:19-22` |
| `Inventory.Capacity` field, `Get(slot)` method | ✅ field + method | `pkg/inventory/inventory.go:22, :62-67` |
| `Item.Id` (lowercase d), `Item.Count` | ✅ confirmed | `pkg/inventory/inventory.go:15-18` |
| `InvType.RunWeight bool` | ✅ exists | `pkg/objtype/invtype.go:27` |
| `ObjType.Weight int (grams)`, `ObjType.Stackable bool` | ✅ exists | `pkg/objtype/objtype.go:105, :131` |
| `InventoryListener.FirstSeen` field + read-modify-write update via map-index assignment | ✅ established pattern | `modules/world/player.go:801-806`, `:1013` |
| `OpUpdateRunEnergy` is the template for the new opcode + helper | ✅ symmetric shape | `pkg/io/protocol/game/server/prot.go:57`, `modules/world/stat_update.go:18-23` |
| TS `Math.trunc(x/1000)` ↔ Go `int / int` semantics | ✅ both truncate toward zero for negative values | (language spec; tested via R5 below) |
| Tick-ordering: `updateInvs` called after content-script tick (so equip ops have already mutated invs); `UpdateRunWeight` lands in the same wire frame as the triggering `UpdateInvFull` | ✅ `processOut` invokes `updateInvs` post-tick | `modules/world/player.go:830`+ |

**R5 — Math.trunc parity for negative runweight**: Go integer division truncates toward zero, matching TS `Math.trunc`. Pinned by `TestSendUpdateRunWeight_NegativeRoundsTowardZero` (negative case is theoretical — `calculateRunWeight` only sums non-negative `Weight × Count`, but the truncation parity is worth pinning since `int / int` and `Math.trunc` coincide for both signs).

**R6 — `inv.Type` is `int` not `int32`**: `Inventory.Type int` per `pkg/inventory/inventory.go:21` — direct comparison to `len(invConfigs)` is type-clean.

## 8. Test plan

### 8.1 New: `modules/world/player_runweight_test.go`

| Test | Scenario | Asserts |
|---|---|---|
| `TestCalculateRunWeight_EmptyInvs` | Player with no invs | `p.runweight == 0` |
| `TestCalculateRunWeight_SkipsNonRunWeightInv` | `RunWeight=false` inv with weighted item | `runweight == 0` |
| `TestCalculateRunWeight_SkipsStackableObj` | RunWeight inv, stackable obj × 100 | `runweight == 0` |
| `TestCalculateRunWeight_SkipsNilInvOrItem` | RunWeight inv with nil slot + nil entries in `p.invs` | `runweight == 0` (defensive) |
| `TestCalculateRunWeight_SumsWeightTimesCount` | RunWeight inv, 4 non-stack × Weight=32g + 1 non-stack × Weight=100g | `runweight == 228` |
| `TestCalculateRunWeight_MultipleInvs` | Two RunWeight invs with different weighted contents | sums correctly across both |
| `TestCalculateRunWeight_OutOfBoundsTypeIDsSkipped` | Inv with `Type` outside `len(invConfigs)`; item with `Id` outside `len(objConfigs)` | `runweight == 0` (no panic) |
| `TestCalculateRunWeight_NilServerNoOp` | `p.client == nil` | `runweight == 0` (no panic) |

### 8.2 Extend: `modules/world/stat_update_test.go`

| Test | Scenario | Asserts |
|---|---|---|
| `TestSendUpdateRunWeightWireFormat` | `kg=42` | wire = `opcode(1) + P2(42)(2)` = 3 bytes; opcode applies ISAAC offset; payload `[0x00, 0x2a]` |
| `TestSendUpdateRunWeight_LargeKg` | `kg=64` (max from clamp on the consumer side, but encoder is unclamped) | round-trips through P2 |
| `TestSendUpdateRunWeight_NegativeRoundsTowardZero` | (unit-test of the divisor at the call site, not the helper itself) — verify `int(-500)/1000 == 0` | sanity-pin Go's truncate-toward-zero for negative integer division |

### 8.3 Extend: `modules/world/player_inv_test.go`

| Test | Scenario | Asserts |
|---|---|---|
| `TestUpdateInvs_RunWeightChangedEmitsPacket` | Per-player listener on RunWeight=true inv; tick `updateInvs` after equipping non-stack obj | captures one `UpdateRunWeight(kg)` packet on the wire batch |
| `TestUpdateInvs_FirstSeenEmitsEvenIfWeightZero` | Fresh listener (`FirstSeen=true`), inv empty | captures one `UpdateRunWeight(0)` even though `runWeightChanged=false` |
| `TestUpdateInvs_NoChangeNoEmitOnSecondTick` | Tick 1 emits `UpdateRunWeight(N)`; tick 2 with no inv mutations | tick 2 emits no `UpdateRunWeight` |
| `TestUpdateInvs_SharedInvDoesNotCountToRunWeight` | SCOPE_SHARED listener (`Source == -1`) on a RunWeight inv | `runWeightChanged` stays false; no `UpdateRunWeight` emitted |
| `TestUpdateInvs_SkipOnNoNetWeightChange` | RunWeight listener fires (`inv.Update=true`) but inv contents have unchanged total weight (e.g. swap two equal-weight items) | no `UpdateRunWeight` emitted (skip-on-no-change branch) |

### 8.4 Smoke target (user-driven)

Equip a weighted non-stackable item from inventory (e.g., bronze axe 2275g, or body armor) → "Weight Carried" UI text updates from "0kg" to the equipped item's weight in kg, on the same tick as the equip animation. Drop the item → UI reverts to 0kg. Equip multiple items → kg is the sum.

## 9. Order of operations

**Single implementer task** (compressed cadence — Bundle 0 short-circuit per `bundle0_short_circuits_stage1_audit`; spec §5 already provides binding TS-source line ranges and code blocks).

1. Add `OpUpdateRunWeight` constant in `prot.go` (§5.1).
2. Add `sendUpdateRunWeight` helper in `stat_update.go` (§5.1).
3. Create `player_runweight.go` with `calculateRunWeight` (§5.2).
4. Replace `updateInvs` body in `player.go` with the extended version (§5.3).
5. Write 8 tests in `player_runweight_test.go` (§8.1).
6. Write 3 tests in `stat_update_test.go` extension (§8.2).
7. Write 5 tests in `player_inv_test.go` extension (§8.3).
8. Run `go test ./... -count=1 -race` + `go vet ./...` + `go build ./...` for verification.

**Order rationale:**
- §5.1 + §5.2 are pure-add (no risk to existing tests).
- §5.3 modifies a method touched by NAI-118 (`updateInvs`) — risk of test regressions in `player_inv_test.go`; running existing tests after §5.3 (before adding new tests) catches any structural break early.
- §8.3's tests depend on §5.3 implementation; §8.1's tests depend on §5.2; §8.2's tests depend on §5.1.

**Final review**: Sonnet `superpowers:code-reviewer` agent (per `superpowers_code_reviewer_model`) — TS fidelity line-by-line vs `Player.ts:598-627` + `NetworkPlayer.ts:337-394` + `UpdateRunWeightEncoder.ts`; defensive-deviation labeling; modern-Go usage (`min`/`max` not needed here); no YAGNI.

## 10. Pattern memories applied

- `compressed_cadence` — single combined spec+plan doc; one implementer dispatch; one Sonnet code-reviewer at end.
- `bundle0_short_circuits_stage1_audit` — spec §5 already line-level binding to TS source; no Stage 1 audit subagent.
- `controller_preflight` — spec §7 enumerates every premise verified at HEAD `c3f078f`; controller will re-grep before dispatch.
- `verify_implementer_claims` — fresh `go test ./... -count=1 -race` post-commit.
- `defensive_gate_doc_comment_label` — D1/D2 labeled in handler doc-comments + commit body.
- `superpowers_code_reviewer_model` — final reviewer Sonnet, never Opus.
- `flat_arg_signature_for_cross_lang_parity` — preserves TS line-for-line shape in `calculateRunWeight`; no struct refactor.
- `true_to_ts_gate` — every behavioral divergence tracked as D1/D2 with rationale + bounds equivalence note.
- `audit_full_method_against_ts` — `updateInvs` extension audited line-by-line vs TS NetworkPlayer.ts:337-394, including the SCOPE_SHARED skip and skip-on-no-change branches.
- `mock_recorder_field_naming_check` — controller pre-flight will verify `mockPlayer` / test-helper field names against actual struct shapes before dispatch.
- `smoke_test_server_handoff` — smoke handoff to user; controller waits for binding.
- `close_commit_memory_trailer` — `Closes memory:` trailer applied on close commit.

## 11. Cross-references

- **Predecessor:** NAI-135 close `c3f078f`. NAI-135 shipped the `updateEnergy` drain branch that reads `p.runweight`; NAI-136 makes that read produce a meaningful value.
- **Adjacent:** NAI-118 close `a52b9b6` last touched `Player.updateInvs` (lazy-alloc fix). NAI-136 extends the same method body — the NAI-118 contract (route through `srv.invLookup.Get`) is preserved.
- **Sibling:** NAI-117 close shipped `OpUpdateRunEnergy` + `sendUpdateRunEnergy` (the exact template for §5.1).
- **Carry-forward (NAI-137+ brainstorm queue, in priority order):**
  - NAI-135 carryover (still queued): run-toggle UI varp binding investigation.
  - NAI-119 carryover: weapon-equip rendering.
  - NAI-122 candidate (NAI-119 carryover residual #2): combat-init `[proc,player_in_combat_check]` cascade.
  - NAI-115 carryover P1: firemaking ashes-no-drop after fire despawn.
  - NAI-115 carryover P2: LOWMEM byte-alignment trace.
  - NAI-111 (still queued): P_TELEJUMP `[label,tutorial_complete]` investigation.
  - NAI-115 D1/D2 deviations (parked).
  - NAI-119-D-NO-DOWNSTREAM-LOC2-CONSUMERS (parked).
