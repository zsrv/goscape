# NAI-35 — deferred opcode stubs (NPC_PARAM, MAP_PLAYERCOUNT, NPC_HUNTALL, HUNTALL, HUNTNEXT, MAP_FINDSQUARE)

## Motivation

Five script-VM opcodes are declared in `pkg/script/opcode.go` but have no handler registered in `pkg/script/handlers.go` at HEAD `afdc28b`. Classic `protocol_stub_not_completed.md` shape: tests pass against the missing dispatch entries because no test exercises the script-VM dispatch path for those opcode numbers.

The named four from NAI-34 follow-up #4 (`From NAI-34 / Items deferred to future sub-specs / item 4 — "the other 4 NAI-33-deferred stubs"`):

- `NPC_PARAM` (opcode 2529, declared at opcode.go:266)
- `MAP_PLAYERCOUNT` (opcode 1015, declared at opcode.go:89)
- `HUNTALL` (opcode 2031, declared at opcode.go:131; player-context variant)
- `MAP_FINDSQUARE` (opcode 1009, declared at opcode.go:83)

Bundle 0 surfaced two structurally-paired sibling stubs that fold in cleanly per `audit_full_method_against_ts.md` and `dead_api_polish.md`:

- `NPC_HUNTALL` (opcode 2526, declared at opcode.go:263) — NPC-context sibling of HUNTALL; reuses NAI-33's `npcIterator` state field via Path-A extension (activates the NAI-33-D1-deferred `huntvis` field that was kept "for retirement readiness").
- `HUNTNEXT` (opcode 2032, declared at opcode.go:132) — consumer of the new `playerIterator` state field that HUNTALL sets. Without HUNTNEXT, HUNTALL ships a fully-functional iterator with zero in-VM consumers — the canonical `dead_api_polish.md` foot-gun.

Pre-NAI-35 behavior: 6 opcodes abort with `Aborted` execution state and log `script %q: no handler for %s (opcode %d) at pc=%d` (runner.go:71) whenever a content script reaches them. Surveyed call-site counts in `LostCityRS/Content/scripts/` (verified by grep at HEAD): `npc_param`=202, `map_findsquare`=77, `huntall`=35, `npc_huntall`=18, `npc_param`=202, `map_playercount`=4 (test/debug only).

Post-NAI-35 behavior: all 6 opcodes execute TS-faithfully; aggressive-NPC behavior, imp wandering, fishing-NPC `npc_param`-driven dialog (where applicable), and the rest of the affected content layer come online. NAI-33-D1 (`huntvis` dead-field deferral) closes via Task 3.

## Tech stack

- **Go 1.26+** (per `go_version.md` memory; use modern Go syntax via the `use-modern-go` skill).
- TS source: `Engine-TS` only per `ts_source_canonical_path.md`.
  - `src/engine/script/handlers/NpcOps.ts:132-141` (NPC_PARAM)
  - `src/engine/script/handlers/ServerOps.ts:27-45` (MAP_PLAYERCOUNT)
  - `src/engine/script/handlers/NpcOps.ts:325-333` (NPC_HUNTALL)
  - `src/engine/script/handlers/PlayerOps.ts:1215-1223` (HUNTALL)
  - `src/engine/script/handlers/PlayerOps.ts:1226-1233` (HUNTNEXT)
  - `src/engine/script/handlers/ServerOps.ts:254-374` (MAP_FINDSQUARE)
- Existing helpers (verified at HEAD afdc28b):
  - `paramLookup` at `pkg/script/handlers_config.go:17` (delegates type-aware push for `nc_param`, `lc_param`, `oc_param`)
  - `requireActiveNpc` at `pkg/script/handlers_npc.go:72`
  - `checkHuntVis` (referenced in state.go:64-66 doc-comments; verify exact location at plan-write)
  - `coordgrid.Unpack` / coord-rect math in `pkg/coordgrid/`
  - `Zone.PlayersSafe` at `pkg/zone/zone.go:422` (mirror of TS `Zone.getAllPlayersSafe`)
  - `GameMap.IsFreeToPlay` at `pkg/gamemap/multimap.go:23`
  - `LineRouteFinder.LineOfWalk` at `pkg/pathfinder/routefinder/lineroutefinder.go:31`
  - `NpcIterator` template at `pkg/script/npc_iterator.go:30` (NAI-33 reference impl); `huntvis int` field at line 44 ALREADY DECLARED, deferred per NAI-33-D1
- Reference shape for iterator family: `iterator_state_pattern.md` template (10 elements, NAI-33 reference impl)

## Scope

**In scope:**

1. `handleNpcParam` — handler for NPC_PARAM (active-npc paramLookup), reuses existing `paramLookup`.
2. `handleMapPlayerCount` — handler for MAP_PLAYERCOUNT, iterates zones in coord-rect via existing `Zone.PlayersSafe`; adds `PlayerLookup.ZonePlayers` interface method.
3. `handleNpcHuntAll` — handler for NPC_HUNTALL; extends `NpcIterator` with HuntAll mode + new `NewHuntAllNpcIterator` constructor; **activates** the deferred `huntvis` filter in `passesFilter` (closes NAI-33-D1).
4. `handleHuntAll` — handler for HUNTALL (player variant); adds `playerIterator *PlayerIterator` field to ScriptState; new `pkg/script/player_iterator.go` mirroring NpcIterator template (HuntAll-only constructor — Distance/Zone modes deferred per NAI-35-D2).
5. `handleHuntNext` — handler for HUNTNEXT; consumes `playerIterator`. Mirrors `handleNpcFindNext` (handlers_npc.go:605) shape.
6. `handleMapFindSquare` — handler for MAP_FINDSQUARE; literal port of TS ServerOps.ts:254-374 (six structural branches: NONE/LINEOFWALK/LINEOFSIGHT × random-50 / west-bias). Adds `IsMapBlocked` and (if absent at HEAD) `IsLineOfSight` wrappers on `pkg/pathfinder/collision/`.
7. NAI-33-D1 retirement (Task 7 close polish) — drop deferred-comment annotations on NpcIterator + update `nai_followups.md` retired-deviations section.

**Out of scope (with rationale):**

- `OpNpcParam` STRING-typed return path edge cases beyond what TS handles (TS dispatches on `paramType.isString()`; goscape mirrors via `paramLookup`'s existing branching).
- PLAYER_FINDALL / PLAYER_FINDALLANY / PLAYER_FINDALLZONE / PLAYER_FINDNEXT family — does not exist in TS at HEAD. PlayerIterator's HuntAll-only constructor (NAI-35-D2) avoids speculative dead-API construction per `dead_api_polish.md`.
- `OpLineOfSight` / `OpLineOfWalk` / `OpMapBlocked` (TS-declared ServerOps opcodes 1004/1005/1006 per ScriptOpcode.ts:47-49) — separate, not in NAI-34 follow-up #4. May surface during plan-author preflight; if confirmed-stub, file as future follow-up.
- Members/F2P-world-config wiring (full feature) — tracked as conditional NAI-35-D3 to be resolved at plan-write.
- NPC_WALK (opcode 2542) — already in `nai_followups.md` from NAI-34. Independent.

## Architecture

NAI-35 is a multi-task feature-port sub-spec, full cadence per `runescript_cadence.md` (size > 100 LOC, beyond `compressed_cadence.md` threshold). Cross-package surface: `pkg/script` ↔ `modules/world` ↔ `pkg/pathfinder/collision` ↔ `pkg/zone`. Two new interface methods (`PlayerLookup.ZonePlayers`, conditional `IsLineOfSight` wrapper). One new file (`pkg/script/player_iterator.go`).

NAI-33's `NpcIterator` extends in-place via Path A (chosen over Path B interface-broadening); the NAI-33-D1 `huntvis` field was reserved with this exact extension hook in mind ("kept for retirement readiness — when LoS/LoW filtering lands, passesFilter only needs to start reading huntvis; no constructor surface change", per the field's existing doc-comment at npc_iterator.go:39-43).

### File layout

| Path | Change | Net production lines |
|---|---|---|
| `pkg/script/handlers_config.go` | + `handleNpcParam` (alongside `handleNcParam:280`) | +10 |
| `pkg/script/handlers_npc.go` | + `handleNpcHuntAll` | +20 |
| `pkg/script/handlers_player.go` (or new) | + `handleHuntAll`, `handleHuntNext` | +50 |
| `pkg/script/handlers_server.go` (or new `handlers_map.go`) | + `handleMapPlayerCount`, `handleMapFindSquare` | +130 |
| `pkg/script/handlers.go` | + 6 dispatch entries (`OpNpcParam`, `OpMapPlayerCount`, `OpNpcHuntAll`, `OpHuntAll`, `OpHuntNext`, `OpMapFindSquare`) | +6 |
| `pkg/script/npc_iterator.go` | + `NpcIteratorHuntAll` mode constant + `NewHuntAllNpcIterator` constructor; activate huntvis branch in `passesFilter`; drop NAI-33-D1 deferred-comments | ±25 |
| `pkg/script/player_iterator.go` (new) | + `PlayerIterator` struct + `Stale` + `passesFilter` + `Next` + `advanceZone` + `NewHuntAllPlayerIterator` constructor | +120 |
| `pkg/script/state.go` | + `playerIterator *PlayerIterator` field; extend `PlayerLookup` interface with `ZonePlayers` method | +5 |
| `pkg/pathfinder/collision/` (existing) | + `IsMapBlocked` wrapper; + `IsLineOfSight` wrapper if absent at HEAD | +15 |
| `modules/world/player_script_lookup.go` | + `ZonePlayers` impl delegating to existing `Zone.PlayersSafe` | +15 |
| `modules/world/npc_script_lookup.go` | unchanged for HuntAll (existing `ZoneNpcs` sufficient — `passesFilter` does the distance + huntvis filtering) | 0 |
| `pkg/script/handlers_*_test.go` (multiple) | + handler tests | +120 |
| `pkg/script/npc_iterator_test.go` | + huntvis-active `passesFilter` pins; + HuntAll-mode iterator end-to-end | +30 |
| `pkg/script/player_iterator_test.go` (new) | + 7-test mirror of `npc_iterator_test.go` | +50 |

**Aggregate**: ~395 net production + ~200 test = medium-to-large multi-task feature-port (well within precedent of NAI-29 and NAI-30 multi-task sub-specs).

## Tasks

Ordering: complexity-ascending + dependency-respecting. Each task is a separate red→green→commit per `superpowers:test-driven-development`.

### Task 1 — NPC_PARAM (OpNpcParam=2529)

**Pop order**: `paramID` (int).

**Body** (~10 LOC): `requireActiveNpc(s, OpNpcParam)`; `paramID := s.PopInt()`; `typeID := s.ActiveNpc.NpcType()`; resolve `npcType` via `s.Configs.NpcType(typeID)`; delegate to `paramLookup(s, npcType.Params, paramID)`.

**Tests** (~10 LOC, in `handlers_config_test.go` alongside `TestHandleNcParam_*`):
- (a) int param push from active-npc-type
- (b) string param push (using paramType.isString-equivalent fixture)
- (c) `requireActiveNpc` returns tagged error when ActiveNpc nil
- (d) `Configs == nil` returns "config nil" error mirroring handleNcParam

**Deviations**: none expected.

### Task 2 — MAP_PLAYERCOUNT (OpMapPlayerCount=1015)

**Pop order**: 2 ints. TS `state.popInts(2)` returns `[c1, c2]` where `c1` was BELOW `c2` on the stack (i.e. `c2` was top-of-stack at pop time). In goscape's individual-pop convention, the first `s.PopInt()` returns top-of-stack, so: `c2 := s.PopInt(); c1 := s.PopInt()`. Pin via test (rare edge cases of script-author-error reverse one direction). Matches `iterator_state_pattern.md` element 6 ("popInts(N) reverse: top of stack first").

**Body** (~30 LOC): unpack both coords; iterate zones in coord-rect via:

```
for zx := from.X >> 3; zx <= (to.X+7) >> 3; zx++ {
    for zz := from.Z >> 3; zz <= (to.Z+7) >> 3; zz++ {
        for p := range s.PlayerLookup.ZonePlayers(from.Level, zx, zz) {
            if p.X() >= from.X && p.X() <= to.X && p.Z() >= from.Z && p.Z() <= to.Z {
                count++
            }
        }
    }
}
s.PushInt(count)
```

**Prerequisites**: extend `PlayerLookup` (state.go:113) with `ZonePlayers(level, zoneX, zoneZ int) []ActivePlayer`. World-side impl in `modules/world/player_script_lookup.go` delegates to `Zone.PlayersSafe(false)`.

**Tests** (~20 LOC):
- (a) empty rect → 0
- (b) single player in single zone → 1
- (c) player at boundary (inclusive `from.x`/`to.x`)
- (d) players spanning multiple zones counted once each
- (e) ceil-math: rect of width <8 still iterates the spanning zone
- (f) cross-level rect: TS uses `from.level` only — pin

**Deviations**: NAI-35-D1 — TS uses `from.level` for inner getZone with no `to.level` validation; cross-level rect silently iterates only `from.level` zones. goscape mirrors. Pinned by test (f).

### Task 3 — NPC_HUNTALL (OpNpcHuntAll=2526) + NAI-33-D1 retirement

**Pop order**: 3 ints — `[coord, distance, huntvis]`. Pop reverse: `huntvis := s.PopInt(); distance := s.PopInt(); coord := s.PopInt()`.

**Body** (~20 LOC handler):
- Validate: `checkCoordValid(coord)`, `checkNumberNotNull(distance)`, `checkHuntVis(huntvis)`.
- Construct: `s.npcIterator = NewHuntAllNpcIterator(s.Npcs, s.World.CurrentTick(), level, x, z, distance, huntvis)`.

**Extension to NpcIterator** (npc_iterator.go):
- Add `NpcIteratorHuntAll NpcIteratorMode = 2` constant alongside Distance/Zone.
- Add `NewHuntAllNpcIterator(lookup NpcLookup, tick, level, x, z, distance, huntvis int) *NpcIterator` — same shape as `NewDistanceNpcIterator` but mode = HuntAll, typeID = -1.
- **Activate huntvis filter in `passesFilter`** (npc_iterator.go:72). Replace line 79 (`// huntvis filter intentionally omitted — NAI-33-D1 carryover`) with active filtering branched by huntvis value: `HuntVisOff` no filter; `HuntVisLineOfSight` calls collision wrapper; `HuntVisLineOfWalk` calls existing `LineRouteFinder.LineOfWalk`.

**Prerequisites**: existing `ZoneNpcs` (NpcLookup:81) already returns the per-zone snapshot — no new NpcLookup methods needed.

**Tests** (~30 LOC):
- Layer 1 — iterator mechanics (huntvis-active passesFilter pin):
  - (a) HuntVisOff admits in-range NPC
  - (b) HuntVisLineOfSight rejects LoS-blocked NPC (collision-flag fixture)
  - (c) HuntVisLineOfWalk admits LoW-clear path
  - (d) NPC outside distance rejected regardless of huntvis
- Layer 2 — handler:
  - (e) pop order
  - (f) iterator state field set with correct fields
- Layer 4 — integration (NPC_HUNTALL → NPC_FINDNEXT loop returns expected count for a fixture zone)

**Deviations**: NAI-33-D1 retired (move from "deferred" to "closed by NAI-35-T3" in deviation registry; drop "carryover" comments). No new deviations expected.

### Task 4 — HUNTALL (OpHuntAll=2031, player variant)

**Pop order**: same 3 ints — `[coord, distance, huntvis]`.

**Body** (~20 LOC handler): same validation + `s.playerIterator = NewHuntAllPlayerIterator(s.PlayerLookup, s.World.CurrentTick(), level, x, z, distance, huntvis)`.

**New file `pkg/script/player_iterator.go`** (~120 LOC): mirror of `npc_iterator.go` shape (same field set: mode, creationTick, lookup, level, x, z, distance, huntvis, zone-cursor, intra-zone snapshot). Single `NewHuntAllPlayerIterator` constructor — Distance/Zone modes deferred per NAI-35-D2.

**ScriptState extension** (state.go): + `playerIterator *PlayerIterator` package-private field alongside `npcIterator` at line 173.

**PlayerLookup extension**: already adds `ZonePlayers` in Task 2; this task uses it.

**Tests** (~40 LOC): 7-test mirror of NpcIterator suite (Layer 1 mechanics + Layer 2 handler).

**Deviations**: NAI-35-D2 — PlayerIterator ships HuntAll-only constructor; Distance/Zone modes deferred until PLAYER_FINDALL family has consumers (currently absent in TS too). Tracked for `dead_api_polish.md` discipline.

### Task 5 — HUNTNEXT (OpHuntNext=2032)

**Pop order**: none (no args).

**Body** (~30 LOC): mirror `handleNpcFindNext` (handlers_npc.go:605):
- Nil-iterator → push 0 (boolean false; verify against existing NpcFindNext convention at plan-write per preflight item 8)
- `s.playerIterator.Stale(s.World.CurrentTick())` → return tagged error (existing log-warn path runs via `npc_script.go:167-172` equivalent for player scripts)
- `it.Next()` hit → set `s.Self = player`; push 1 (or UID — confirm against TS PlayerOps.ts:1226-1233 + handleNpcFindNext shape)
- `it.Next()` done → push 0; iterator NOT cleared (matches `iterator_state_pattern.md` element 7)

**Tests** (~20 LOC):
- (a) nil-iterator pushes 0
- (b) Stale returns error (suspended-cross-tick scenario)
- (c) hit pushes 1 + sets Self (or UID per preflight resolution)
- (d) done pushes 0
- (e) exhaustion does NOT clear iterator — repeat-call pushes 0 again

**Deviations**: none expected; mirrors NAI-33 NPC_FINDNEXT shape exactly.

### Task 6 — MAP_FINDSQUARE (OpMapFindSquare=1009)

**Pop order**: 4 ints — `[coord, minRadius, maxRadius, type]`.

**Body** (~100 LOC): literal port of TS ServerOps.ts:254-374. Six structural branches:

| `maxRadius` | `type` | Algorithm |
|---|---|---|
| `< 10` | NONE | Random-sample 50 attempts in `[-maxRadius, +maxRadius]` × `[-maxRadius, +maxRadius]`; chebyshev distance; return first non-blocked + (free-world & F2P-tile-OK) |
| `< 10` | LINEOFWALK | Same + LoW check from candidate to origin |
| `< 10` | LINEOFSIGHT | Same + LoS check from candidate to origin |
| `>= 10` | NONE | West-bias: outer `x` ascending (-maxRadius to +maxRadius), inner `distZ` random; `!isWithinDistanceSW` exclusion |
| `>= 10` | LINEOFWALK | Same + LoW |
| `>= 10` | LINEOFSIGHT | Same + LoS |

Push found packed coord on hit (`CoordGrid.PackCoord(level, x, z)`); push original `coord` on fall-through (TS line 373).

**Prerequisites**:
- Existing `GameMap.IsFreeToPlay` (multimap.go:23) — for `freeWorld` F2P-zone check.
- Existing `LineRouteFinder.LineOfWalk` (lineroutefinder.go:31) — wrap as package-level `IsLineOfWalk(level, x1, z1, x2, z2 int) bool`.
- New `IsMapBlocked(level, x, z int) bool` wrapper on `pkg/pathfinder/collision/flagmap` (returns true if BLOCK_WALK flag set).
- New `IsLineOfSight(level, x1, z1, x2, z2 int) bool` if absent at HEAD (uses `pkg/pathfinder/collision/strategies.go` `TypeLineOfSight` branch). Plan-author preflight item 3.
- New `MapFindSquareType` constants (NONE=0, LINEOFWALK=1, LINEOFSIGHT=2).
- Members/F2P world-config flag — see NAI-35-D3.

**Tests** (~50 LOC):
- (a) maxRadius<10 + NONE finds a free square (deterministic via seeded `rand.New(rand.NewSource(...))`)
- (b) maxRadius<10 + LINEOFWALK requires LoW
- (c) maxRadius<10 + LINEOFSIGHT requires LoS (collision-flag fixture)
- (d) maxRadius>=10 west-bias finds expected x-direction-first
- (e) all-50-attempts-fail → returns origin coord
- (f) all-blocked + members-world rejects F2P-tile
- (g) `FindSquareValid` validation (type ∈ {0,1,2})
- (h) `NumberPositive` validation (minRadius/maxRadius > 0)

**Deviations**:
- NAI-35-D3 (conditional) — F2P/members-world flag wiring if `cmd/goscape/app/config.go` doesn't expose it. Plan-author resolves at preflight item 4.
- NAI-35-D4 — Goscape uses `math/rand`; TS uses `Math.random`. Behaviorally equivalent (non-deterministic-per-call). Tests seed deterministically. Per `true_to_ts_gate.md`, this is an instrumentation note, not a behavioral divergence.

### Task 7 — Close polish + NAI-33-D1 retirement

**Scope**:
- Update `nai_followups.md` retired-deviations section: NAI-33-D1 closed by NAI-35-T3.
- Drop "huntvis filter intentionally omitted" comment block from `passesFilter` body (npc_iterator.go:79) — now active for HuntAll mode.
- Drop "kept for D1 retirement readiness" comment from NpcIterator.huntvis declaration (npc_iterator.go:39-43) — now consumed.
- Per `retire_deviation_grep_all_comments.md`: `rg "S7f-D1|NAI-33-D1" pkg/ modules/` at task-start to enumerate all stale comment sites; update each.
- Per `dead_api_polish.md`: scan for helpers shipped without consumers across Tasks 1-6; remove or document.
- Apply final-review polish flagged across Tasks 1-6.

**Tests**: minimal — any test asserting "huntvis omitted" semantics needs updating (none expected at HEAD).

**Deviations**: none.

## Test strategy

- **Per-task TDD per `superpowers:test-driven-development`**: red → green → commit per task. No batched green-runs.
- **4-layer suite for iterator-family (Tasks 3, 4, 5)** per `iterator_state_pattern.md` template.
- **Plan-author preflight per `plan_test_coverage_crosscheck.md`**: every test enumerated above must appear verbatim in the plan doc's task code block.
- **Plan-test-runnable check per `plan_runnable_test_fixtures.md`**: each plan-codified test fixture mentally compiled before dispatch.
- **Cross-package green required per `verify_implementer_claims.md`**: each task's commit must pass `go test ./...`.

## Deviations registry

| ID | Task | Behavior | Status |
|---|---|---|---|
| NAI-35-D1 | T2 (MAP_PLAYERCOUNT) | TS uses `from.level` for inner getZone with no `to.level` validation; cross-level rect silently iterates only `from.level` zones. goscape mirrors. | Active; pinned by test |
| NAI-35-D2 | T4 (HUNTALL) | PlayerIterator ships HuntAll-only constructor. Distance/Zone modes deferred until PLAYER_FINDALL family has consumers (currently absent in TS). | Active; per `dead_api_polish.md` discipline |
| NAI-35-D3 | T6 (MAP_FINDSQUARE) | Members/F2P world flag wiring if `cmd/goscape/app/config.go` doesn't expose it. Conditional — plan-author resolves at preflight. | Conditional; resolve at plan-write |
| NAI-35-D4 | T6 (MAP_FINDSQUARE) | Goscape uses `math/rand`; TS uses `Math.random`. Behaviorally equivalent. Tests seed via `rand.New(rand.NewSource(...))`. | Instrumentation, not behavioral |
| NAI-33-D1 | T3 (NPC_HUNTALL) | **Retired** by NAI-35-T3 — `huntvis` field becomes live consumer. | Closed |

## Smoke gate

End-of-spec, user-driven smoke per `smoke_test_server_handoff.md`. User runs the server with the post-NAI-35 binary; Claude's sandboxed binary is unreachable from the host Java client.

Smoke checklist:

1. **NPC_PARAM smoke** — Login + `::tele 3222 3219` (Lumbridge area). Confirm no `no handler for opcode 2529` WARN in server log on subsequent NPC interactions that use `npc_param`.
2. **HUNTALL smoke** — Walk to Al-Kharid. Confirm aggressive warriors (`Content/scripts/areas/area_alkharid/scripts/al_kharid_warrior.rs2`) attack/approach the player when in range. Pre-NAI-35: silent. Post-NAI-35: aggression fires.
3. **NPC_HUNTALL smoke** — Walk to Barbarian Village beer barrels (`Content/scripts/areas/area_barbarian_village/scripts/beer_barrels.rs2:3` — `npc_huntall(coord, 10, ^vis_lineofsight)`). Confirm intended interaction visible.
4. **MAP_FINDSQUARE smoke** — Walk to Wizards' Tower (`Content/scripts/areas/area_wizard_tower/scripts/wizard_grayzag.rs2:19`) where the imp NPC uses `map_findsquare`. Confirm imp visibly wanders to nearby squares.
5. **MAP_PLAYERCOUNT smoke** — Optional; only used in test/debug content. Validate via `[mes,debug_map_playercount]` if available.
6. **No regressions** — NAI-33 fishing-spot relocate (NAI-33's smoke gate) still works; NpcIterator extension is additive to non-HuntAll modes.

If any smoke fails, surface as a Bundle 3 conditional follow-up per `investigation_subspec_cadence.md`.

## Plan-author preflight checklist

Plan-author MUST verify each item against HEAD before dispatching subagents (per `controller_preflight.md`):

1. **Configs.NpcType API shape** — Task 1 references `s.Configs.NpcType(typeID).Params`. Grep the Configs interface in `pkg/script/state.go` and existing handleNcParam to confirm exact accessor.
2. **PopInt convention** — Task 2's pop-order claim (top-of-stack first → c2 popped before c1). Verify against `iterator_state_pattern.md` element 6 + actual `s.PopInt()` semantics.
3. **`IsLineOfSight` wrapper status** — Task 6 assumes wrapper either exists or is trivial to add. Grep `pkg/pathfinder/` for `LineOfSight` (case-insensitive). If no top-level wrapper, plan a small one alongside `IsMapBlocked`.
4. **Members/F2P world flag** — Task 6 D3 conditional. Grep `cmd/goscape/app/config.go` and `modules/world/` for `Members` / `F2P`. Resolve D3 to "wire it" or "stub-as-members" before dispatch.
5. **Existing NAI-33-D1 / S7f-D1 comment sites** per `retire_deviation_grep_all_comments.md` — `rg "NAI-33-D1|S7f-D1" pkg/ modules/` and enumerate every stale doc-comment that Task 7 must update.
6. **PlayerLookup interface body** — Task 4 extends with `ZonePlayers`. Read state.go:113-117 for exact shape; grep `modules/world/` for existing implementor.
7. **Compiled-bytecode call-site inventory per opcode** — confirms Bundle 0 content-script counts (npc_param=202, etc.) by sampling compiled scripts in `data/pack/server/script.dat`.
8. **HUNTNEXT push-shape** — Task 5 says "push 1" or "push UID". Read TS PlayerOps.ts:1226-1233 + `handleNpcFindNext` (handlers_npc.go:605) to pin the exact convention.
9. **`checkNumberNotNull` + `checkHuntVis` helper locations** — Tasks 3, 4 reference both. Grep `pkg/script/handlers_*.go` to confirm existing helpers exist (likely `checkNumberNotNull` exists alongside `checkCoordValid`; `checkHuntVis` referenced in state.go:64-66 doc-comments). If `checkHuntVis` is documented but not implemented, plan a small wrapper alongside Task 3.
10. **`Configs.NpcType` accessor returning a `Params` map vs `params` lowercase field** — Task 1 references `npcType.Params`. Goscape's NpcType-config struct may use either case; verify existing handleNcParam / handleLcParam / handleOcParam usage and mirror exactly.

Per `enumerate_all_sites.md`: the plan-author should pre-grep all 4 NAI-35-D entries (proposed) for any pre-existing collision with prior deviation IDs.

## Follow-ups expected at NAI-35 close

- **NAI-35-D2 retirement** when PLAYER_FINDALL family lands → PlayerIterator Distance/Zone modes activate.
- **NAI-35-D3 retirement** if F2P flag was stubbed-as-members at NAI-35-T6 → wire properly when world-config-members flag becomes a real feature.
- Sibling **LINEOFSIGHT / LINEOFWALK / MAP_BLOCKED** opcodes 1005 / 1006 / 1007 (ScriptOpcode.ts:47-49 — derived from auto-increment from `COORDX = 1000` at line 42) — TS exposes top-level opcodes for these. If still stubbed at NAI-35 close, file as `nai_followups.md` entry. Plan-author preflight item 3 may surface this incidentally.
- Per `close_commit_memory_trailer.md`: NAI-35 close commit gets `Closes memory:` trailer for grep-discoverable provenance (NAI-33-D1 retirement, NAI-34 follow-up #4 closure).

## Memory cross-references

- `runescript_cadence.md` — established sub-spec workflow (brainstorm → spec → plan → subagent-driven TDD with two-stage review)
- `execution_mode_default.md` — dispatch via `superpowers:subagent-driven-development`
- `iterator_state_pattern.md` — 10-element NPC_FIND-family template, reused by Tasks 3-5
- `dead_api_polish.md` — discipline applied to NAI-35-D2 (PlayerIterator HuntAll-only) and HUNTNEXT inclusion (preventing dead HUNTALL)
- `audit_full_method_against_ts.md` — drove NPC_HUNTALL + HUNTNEXT scope expansion beyond the named 4 stubs
- `protocol_stub_not_completed.md` — the foundational pattern this sub-spec resolves (6 instances)
- `controller_preflight.md` — plan-author preflight checklist above
- `plan_test_coverage_crosscheck.md` — every test enumerated above must appear in plan code blocks
- `plan_runnable_test_fixtures.md` — fixtures must be mentally compiled before dispatch
- `verify_implementer_claims.md` — cross-package green required at every commit
- `retire_deviation_grep_all_comments.md` — Task 7 must enumerate all NAI-33-D1 / S7f-D1 stale comment sites
- `smoke_test_server_handoff.md` — smoke is the binding feature gate; user-launched
- `true_to_ts_gate.md` — every behavioral divergence has a tracked deviation entry
- `close_commit_memory_trailer.md` — NAI-35 close commit gets `Closes memory:` trailer
- `go_version.md` — Go 1.26+
- `ts_source_canonical_path.md` — only `LostCityRS/Engine-TS` is the porting reference
