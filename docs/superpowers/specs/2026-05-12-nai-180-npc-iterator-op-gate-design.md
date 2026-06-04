# NAI-180 — NpcIterator HuntAll `op[1]` operability gate port

**Date:** 2026-05-12
**Status:** Design (combined spec + plan per `compressed_cadence.md`)
**Tracker:** Retires `NAI-35-T3-D1` deviation at `pkg/script/npc_iterator.go:93-97`
**Predecessor:** NAI-35 T3 (HuntAll iterator wiring; left D1 deferred pending content audit)
**HEAD at design:** main (post-NAI-179 close `530e74f`)

## 1. Problem

`pkg/script/npc_iterator.go::passesFilter` HuntAll-mode branch does not port
the TS `NpcType.op[1]==""` reject filter that lives in TS
`Engine-TS/src/engine/script/ScriptIterators.ts:274-280`:

```ts
const npcType: NpcType = NpcType.get(npc.type);
if (!npcType.op) {
    continue;
}
if (!npcType.op[1]) {
    continue;
}
if (CoordGrid.distanceToSW(...) > this.distance) {
    continue;
}
```

Semantics: the HuntAll iterator (used by `NPC_HUNTALL` opcode 2520 and
`NPC_HUNT` opcode 2525) skips NPCs whose 2nd op-slot is empty. The filter
exists because `npc_huntall` was designed for aggression-target hunting;
NPCs with no attackable op label have no reason to be returned.

Goscape skipped this in NAI-35 T3 because plumbing `Configs` onto
`NpcIterator` was out of scope. The deviation tag `NAI-35-T3-D1` has been
parked since then.

## 2. Content audit (Stage 1)

Surveyed all 13 production `npc_huntall(...)` sites in
`LostCityRS/Content/scripts/`:

| Script | Self-filters by | Relies on op[1]? |
|---|---|---|
| `kolodion.rs2` mage arena | exact `npc_type` | No |
| `beer_barrels.rs2` barbarian villagers | `npc_category` + `npc_getmode` | No |
| `stealing.rs2`, `quest_zombiequeen.rs2`, `kalrag.rs2`, `quest_ball.rs2`, `og.rs2`, `grew.rs2`, `legends_boulder.rs2`, `spade.rs2`, `quest_ikov.rs2` | various downstream filters | No |
| `_test/debug_npc_huntall.rs2` | debug-only iteration | No |

**No content site depends on the op[1] reject filter.** Goscape's wider
yield is harmless because every surveyed consumer self-filters by
`npc_type`, `npc_category`, or `npc_getmode` immediately after `npc_findnext`.

Despite the absence of smoke-binding evidence, the user elected to port the
filter for TS-fidelity completeness per `true_to_ts_gate.md`. The audit
itself is the spec input — Stage 2 (this port) ships.

## 3. Op-index mapping (verified)

TS `op: (string | null)[]` is allocated as `new Array(5).fill(null)` and
populated by `op[code - 30] = dat.gjstr()` for codes 30..34 (`NpcType.ts`).
Goscape `NpcType.Op []string` is populated identically by
`t.Op[code-30] = dat.GJStrLF()` (`pkg/objtype/npctype.go:212`) with
`"hidden"` collapsing to `""` per existing loader convention.

Therefore TS `npcType.op[1]` (2nd element of 0-indexed array) maps to
goscape `npcType.Op[1]` (2nd element of 0-indexed slice). 1-for-1.

Both arrays are 5-slot; the `len(Op) <= 1` guard is defensive against
minimally-decoded `NewNpcType()` test fixtures where `Op` may be nil or
shorter.

## 4. Architecture

### 4.1 `NpcIterator` struct addition

Add `configs Configs` field:

```go
type NpcIterator struct {
    // ... existing fields ...
    configs Configs
}
```

`Configs` is the existing interface from `pkg/script/active.go` (the same
interface `ScriptState.Configs` uses). HuntAll-mode is the only consumer.

### 4.2 `passesFilter` HuntAll branch

Insert the op-gate **BEFORE** the existing distance check, mirroring TS
order at `ScriptIterators.ts:274-280`:

```go
func (it *NpcIterator) passesFilter(npc ActiveNpc) bool {
    if it.mode == NpcIteratorZone {
        return true // ZONE mode: no per-NPC filtering per TS line 329-335
    }
    // NAI-180: HuntAll-mode op[1] reject (TS ScriptIterators.ts:274-280).
    // Runs BEFORE distance check per TS order. Retires NAI-35-T3-D1.
    if it.mode == NpcIteratorHuntAll {
        if it.configs != nil {
            npcType := it.configs.NpcType(npc.NpcType())
            if npcType == nil || len(npcType.Op) <= 1 || npcType.Op[1] == "" {
                return false
            }
        }
        // (goscape defensive; TS throws on missing NpcType) — test fixtures
        // may omit Configs; pessimistically allow when configs is nil, matching
        // the lineValidator==nil convention at npcVisibleViaLineOfSight.
    }
    if coordgrid.DistanceToSW(it.x, it.z, npc.NpcX(), npc.NpcZ()) > it.distance {
        return false
    }
    if it.mode == NpcIteratorHuntAll {
        switch it.huntvis {
        case objtype.HuntVisOff:
        case objtype.HuntVisLineOfSight:
            if !it.npcVisibleViaLineOfSight(npc) {
                return false
            }
        case objtype.HuntVisLineOfWalk:
            if !it.npcVisibleViaLineOfWalk(npc) {
                return false
            }
        }
    }
    if it.typeID >= 0 && npc.NpcType() != it.typeID {
        return false
    }
    return true
}
```

### 4.3 Constructor signature change

`NewHuntAllNpcIterator` gains a `configs Configs` parameter:

```go
func NewHuntAllNpcIterator(
    lookup NpcLookup,
    lv LineValidator,
    configs Configs,            // NEW
    tick, level, x, z, distance, huntvis int,
) *NpcIterator
```

### 4.4 Call sites

`pkg/script/handlers_npc.go:879` (`handleNpcHuntAll`) and
`pkg/script/handlers_npc.go:924` (`handleNpcHunt`): pass `s.Configs` as the
new argument.

### 4.5 Test fixtures

All 10 existing `NewHuntAllNpcIterator` call sites in
`pkg/script/npc_iterator_test.go` (lines 264, 293, 304, 314, 323, 332,
336, 348, 373, 387) get `nil` inserted for the new `configs` parameter.
The new test pins use a populated stub (likely `mockConfigs` already
defined at `pkg/script/handlers_config_test.go:11` per
`Configs interface` precedent).

## 5. Tests

Three new test pins in `pkg/script/npc_iterator_test.go`:

### 5.1 `TestPassesFilter_HuntAll_OpEmpty_Rejects`

Sets up an NPC with NpcType where `Op[1] = ""`. Calls `passesFilter` (or
asserts via iterator yield) — expects rejection.

### 5.2 `TestPassesFilter_HuntAll_OpNonEmpty_Allows`

Sets up an NPC with NpcType where `Op[1] = "Attack"`. Calls `passesFilter`
— expects acceptance (assuming distance + LoS pass).

### 5.3 `TestPassesFilter_HuntAll_NilConfigs_Allows`

Iterator constructed with `configs = nil`. Calls `passesFilter` — expects
acceptance (defensive pessimistic-allow).

### 5.4 Existing-test no-regression

The 4 existing HuntAll-iterator tests at `npc_iterator_test.go:260+` must
still pass after threading `nil` Configs through their constructors. Their
existing assertions (Stale check, distance bound, LineValidator stub
behavior) are unchanged.

## 6. Implementation plan

### Bundle 0 — RED

T1: Add `configs Configs` field to `NpcIterator`. Add `configs Configs`
parameter to `NewHuntAllNpcIterator`. Thread `nil` through the 4 existing
test fixture calls (compile-only change to keep tests green at this stage).
Verify `go build ./...` passes.

T2: Add the 3 new test pins. Expect them to **FAIL** with the existing
`passesFilter` (no op-gate yet). Verify failure modes:
- `OpEmpty_Rejects` fails (allows instead of rejects)
- `OpNonEmpty_Allows` passes incidentally (no filter to break it)
- `NilConfigs_Allows` passes incidentally (no filter to break it)

### Bundle 1 — GREEN

T3: Insert the op-gate in `passesFilter` HuntAll-mode branch per §4.2.
Wire `s.Configs` argument at the 2 production call sites in
`handlers_npc.go`. Verify all 3 new tests pass + existing tests still
green.

### Bundle 2 — CLOSE

T4: Retire `NAI-35-T3-D1` doc-comment in `passesFilter` (replace with
"NAI-180 closes NAI-35-T3-D1" marker per `retire_deviation_grep_all_comments`).
Update `pkg/script/npc_iterator.go` constructor doc-comment.
Update tracker entries at `nai_followups.md` line 2484 (primary
deviation listing), 2542, 2606 (carry-forward duplicates).
Close commit with `Closes memory: NAI-35-T3-D1` trailer.

## 7. Risk register

- **R1 (low):** Production NPC configs may have `Op[1] = ""` for legitimate
  aggression-targets if OSRS uses non-standard slot conventions. Audit
  evidence: all 13 surveyed `npc_huntall` consumers self-filter by
  `npc_type`/`npc_category`, so a behaviorally-wrong reject is invisible
  to current content. Mitigation: pre-flight grep for any NpcType whose
  Op[1] is empty AND that appears in HuntAll content; flag for re-audit
  if surprising.
- **R2 (low):** Nil-Configs path in tests. Mitigation: pessimistic-allow
  matches the `lineValidator == nil` precedent at
  `npcVisibleViaLineOfSight` (`npc_iterator.go:123-128`).
- **R3 (very low):** TS-faithful order moves the op-gate BEFORE distance.
  No behavior change since both checks are AND'd; only perf differs (skips
  distance calc on op-rejected NPCs). Order matches TS line 274-284.

## 8. Out of scope

- Porting the filter to `NpcIteratorDistance` mode. Distance mode keeps
  the existing deferred posture per `NAI-33-D1 / S7f-D1` doc-comment.
  Distance is the FINDALL family (`NPC_FINDALL`, `NPC_FINDNEXT`), not
  HuntAll.
- ZONE mode: TS line 329-335 explicitly skips filtering — already correct.
- A Configs nil-DENY policy: out per §4.2 pessimistic-allow rationale.

## 9. Closure criteria

- All 3 new test pins green.
- Existing 4 HuntAll-iterator tests still green.
- `go vet ./... && go test ./...` green.
- `rg NAI-35-T3-D1 pkg/ modules/ cmd/` returns 0 production hits.
- Tracker entry annotated RETIRED.
- Close commit trailer cites memory closure.

## 10. Memory notes

This sub-spec is the first to plumb the `Configs` interface onto a
script-level iterator. Pattern is reusable if FINDALL-family
(`NewDistanceNpcIterator`) ever ports its own per-NPC type filter (likely
not needed; the content audit for HuntAll showed no consumer reliance,
and Distance-mode consumers (FINDALL) already self-filter by typeID at
construction).
