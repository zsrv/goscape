# NAI-33 — NPC iterator family port (FINDALL / FINDALLANY / FINDALLZONE / FINDNEXT)

- **Sub-spec**: NAI-33
- **Date**: 2026-04-26
- **Scope label**: One-bundle feature-port sub-spec — ports the four `NPC_FIND*` iterator-family opcodes (NPC_FINDALL=2514, NPC_FINDALLANY=2515, NPC_FINDALLZONE=2516, NPC_FINDNEXT=2520) currently declared but unhandled in goscape's script VM. Adds a new `*NpcIterator` state field on `ScriptState`, a new lazy zone-walking iterator type in `pkg/script`, one new world-side primitive (`NpcLookup.ZoneNpcs`) for per-zone NPC enumeration, and four new handler funcs. Closes the runtime WARN `no handler for NPC_FINDALLANY (opcode 2515)` triggered by `[proc,check_fishing_spot_empty]` in `skill_fishing/scripts/fishing_movement.rs2`. Smoke gate: server restart + observe fishing NPCs visibly relocate between spots over time (the proc's purpose); pre-existing `move_fishing_spot` chain executes end-to-end with no opcode-not-found WARNs.
- **Predecessors**: NAI-32 (renderer dual-cache CHAT port + rs-server-225 citation sweep + 4-stage Bundle 3 layered-bug investigation) — last on `main` as `a0852e0`
- **Source root**:
  - `LostCityRS/Engine-TS` (TS canonical for `pkg/script` per `ts_source_canonical_path.md`)
    - `src/engine/script/handlers/NpcOps.ts:403-441` (the four handler bodies)
    - `src/engine/script/ScriptIterators.ts:297-363` (the `NpcIterator` class + DISTANCE/ZONE generator)
    - `src/engine/script/ScriptState.ts:125` (`npcIterator` field declaration)
    - `src/engine/script/ScriptOpcodePointers.ts:586-600` (pointer-set/require contract)
  - `LostCityRS/Content/scripts/skill_fishing/scripts/fishing_movement.rs2:30-37` (the proc that triggers the WARN — driver, not port source)

## Motivation

Server log emits `level=WARN source=…/npc_script.go:169 msg="npc script execute error" script=[proc,check_fishing_spot_empty] err="script \"[proc,check_fishing_spot_empty]\": no handler for NPC_FINDALLANY (opcode 2515) at pc=3"` whenever an NPC's `move_fishing_spot` AI timer fires. The `check_fishing_spot_empty` proc body is:

```
[proc,check_fishing_spot_empty](coord $rand_coord)(boolean)
npc_findallany($rand_coord, 0, 0);          // opcode 2515
while (.npc_findnext = true) {              // opcode 2520
    if (.npc_coord = $rand_coord & ...) {
        return (false);
    }
}
return (true);
```

Both opcodes are declared in `pkg/script/opcode.go` (`OpNpcFindAllAny=2515` line 252, `OpNpcFindNext=2520` line 257) and named in the `String()` toString switch at lines 904 and 914 — but neither is wired to a handler. Result: every fishing-NPC AI-timer tick emits the WARN and the proc returns whatever the engine's "no handler" abort path produces. Effect: fishing NPCs never relocate between spots; the `fishing_movement` content layer is broken end-to-end despite the engine declaring it should work.

Same shape as `protocol_stub_not_completed.md` (NAI-32 Bundle 3 Stage 5/6) but at the script-VM layer: declared protocol surface, no production wiring, tests pass against the declarations because no test crosses the integration boundary that would surface "missing handler". Distinguishing feature here: opcodes 2514 (FINDALL) and 2516 (FINDALLZONE) are also declared-but-unhandled — silently-broken siblings. Bundling all four in one sub-spec amortizes the iterator-state plumbing (single new `*NpcIterator` field on `ScriptState`, single new `ZoneNpcs` method on `NpcLookup`, single new iterator type) and establishes the template that the parallel-shaped LOC iterator family (`OpLocFindAllZone=3008` at `opcode.go:297`, also stubbed) will copy when ported in a future sub-spec.

The choice of custom `*NpcIterator` struct (vs `iter.Pull` over `iter.Seq[ActiveNpc]` vs eager-snapshot) was decided in brainstorm: avoids the goroutine-lifecycle invariant `iter.Pull` would impose on the script-VM's multiple termination paths (`ClearActiveScript`, error-return, NPC-suspend) and leaves the field nil-or-non-nil for clean mocking. Lazy per-zone snapshot inside the iterator preserves TS's "single-tick, throws if reused" semantic via a one-line `Stale(currentTick)` check in the FINDNEXT handler.

## Tech stack

- Go 1.26+
- Existing packages **read** from at brainstorm time:
  - `pkg/script/opcode.go` lines 250-260 (opcode constants — all four already declared) and lines 901-914 (toString switch — all four already named)
  - `pkg/script/state.go` lines 30-46 (`WorldVars` interface — `CurrentTick()` at line 41 already exposed) and lines 58-80 (`NpcLookup` interface — `FindClosestNpcByType`, `FindClosestNpcByCategory`, `FindNpcAtExactCoord` already defined; deviation S7f-D1 documented at lines 64-66 noting huntvis is validated but not filtered)
  - `pkg/script/handlers_npc.go` lines 377-479 (existing FIND-family handlers — `handleNpcFind`, `handleNpcFindCat`, `handleNpcFindExact` — for pop-order/validation/`setActiveNpcSlot` patterns)
  - `pkg/script/handlers_npc.go` `setActiveNpcSlot` definition (helper used by all FIND handlers — exact location to verify at plan-time)
  - `pkg/script/handlers_npc.go` validation helpers `checkCoord`, `checkNotNull`, `checkHuntVis`, `checkNpcType` (locations to verify at plan-time)
  - `pkg/script/handlers_npc_test.go` lines ~1037-1396 (existing FIND-family tests — for mock `NpcLookup` fixture name and shape to reuse per `mock_recorder_field_naming_check.md`)
  - `pkg/zone/zone.go` line 38 (`Zone` struct), line 54 (`npcs DoublyLinkList[NpcLike]`), line 439 (`NpcsSafe(reverse bool) iter.Seq[NpcLike]`), line 458 (`NpcsCount() int`)
  - `pkg/coordgrid/coordgrid.go` line 131 (`DistanceToSW(posX, posZ, otherX, otherZ int) int`)
  - `modules/world/npc_script_lookup.go` (`serverNpcLookup` impl of `NpcLookup` — extension site for `ZoneNpcs` method)
  - `modules/world/npc_script.go:167-186` (`resumeOrFinishNpc` termination paths — verified non-impact: ScriptState drop on Aborted/Finished, persistence on NpcSuspended; both tolerate iterator field zero-cleanup)
  - `LostCityRS/Engine-TS/src/engine/script/handlers/NpcOps.ts:403-441` (TS handler bodies for the four opcodes)
  - `LostCityRS/Engine-TS/src/engine/script/ScriptIterators.ts:297-363` (TS `NpcIterator` class — DISTANCE/ZONE generators, bounding-box math, per-NPC filter chain, stale-tick throw)
  - `LostCityRS/Engine-TS/src/engine/script/ScriptState.ts:125` (TS `npcIterator: IterableIterator<Npc> | null = null` field declaration)
  - `LostCityRS/Engine-TS/src/engine/script/ScriptOpcodePointers.ts:586-600` (pointer-set/require contract — informs nil-check semantics on FINDNEXT)
- New files:
  - `pkg/script/npc_iterator.go` — `NpcIteratorMode` enum, `NpcIterator` struct, `NewDistanceNpcIterator` + `NewZoneNpcIterator` constructors, `Stale(currentTick) bool`, `Next() (ActiveNpc, bool)`
  - `pkg/script/npc_iterator_test.go` — Layer 1 tests (bounds math, cursor order, distance filter, type filter, ZONE mode, stale-check)
- Modified files in `pkg/script/`:
  - `state.go` — add `npcIterator *NpcIterator` field to `ScriptState` (lowercase, package-private; alongside existing `ActiveNpc` field — exact insertion line to verify at plan-time); add `ZoneNpcs(level, zoneX, zoneZ int) []ActiveNpc` method to `NpcLookup` interface (around line 80 in the interface body)
  - `handlers_npc.go` — append four new handler funcs (`handleNpcFindAll`, `handleNpcFindAllAny`, `handleNpcFindAllZone`, `handleNpcFindNext`) after `handleNpcFindExact` (~line 480); register the four in the dispatch table (location to verify at plan-time — likely an `init()` block or a switch in `pkg/script/executor.go`)
  - `handlers_npc_test.go` — append Layer 2 tests (~20 tests across the four handlers); reuse existing mock-`NpcLookup` fixture and add stub `ZoneNpcs` method to it
- Modified files outside `pkg/script/`:
  - `modules/world/npc_script_lookup.go` — add `ZoneNpcs(level, zoneX, zoneZ int) []ActiveNpc` method on `serverNpcLookup` (consumes `pkg/zone.Zone.NpcsSafe(true)` per-zone)
  - `modules/world/npc_script_lookup_test.go` (new file or extension of existing) — Layer 3 tests for `ZoneNpcs`
- Test files modified or created: see four-layer breakdown in § Testing below.

## Scope

In scope:
- Implement four NPC iterator-family handlers: `handleNpcFindAll` (2514), `handleNpcFindAllAny` (2515), `handleNpcFindAllZone` (2516), `handleNpcFindNext` (2520).
- New `*NpcIterator` type in `pkg/script` with DISTANCE and ZONE modes, lazy per-zone snapshot via `NpcLookup.ZoneNpcs`, single-tick lifetime via `Stale(currentTick) bool`.
- New `npcIterator *NpcIterator` field on `ScriptState`. Set by FINDALL*; consumed by FINDNEXT. Nil-or-non-nil; no termination-path cleanup needed (verified — see Architecture § Lifecycle invariants).
- New `ZoneNpcs(level, zoneX, zoneZ int) []ActiveNpc` method on `NpcLookup` interface. World-side impl in `serverNpcLookup` consumes existing `pkg/zone.Zone.NpcsSafe(true)`.
- Stale-tick check in `FINDNEXT` returning error → existing `npc_script.go:169` log-warn → `ClearActiveScript` path. Mirrors TS throw-on-stale faithfully without new cleanup wiring.
- Preserve TS bounding-box math exactly: `centerX = x>>3`, `radius = 1 + distance/8`, zone walk `maxX→minX`, `maxZ→minZ`. Ordering pinned with a dual-pin test per `ts_asymmetry_dual_pin.md`.
- Preserve TS pop order exactly: FINDALL pops 4 (checkVis, distance, npcTypeID, coord), FINDALLANY pops 3 (checkVis, distance, coord), FINDALLZONE pops 1 (coord), FINDNEXT pops 0.
- Preserve TS validation chain exactly per opcode (CoordValid + NumberNotNull + NpcTypeValid + HuntVisValid as applicable; FINDALLZONE validates coord only; FINDNEXT validates nothing pre-iterator).
- Preserve TS exhaustion semantics: FINDNEXT does NOT clear `state.npcIterator` after exhaustion (matches TS `state.npcIterator?.next()` returning `{done:true}` without nulling the field). Subsequent FINDNEXT calls continue to push 0.
- Smoke gate: user-launched server, fishing NPC near a `move_fishing_spot`-eligible spawn, observe (a) original WARN gone, (b) no new WARN at FINDNEXT, (c) NPCs visibly relocate over time.

Out of scope:
- LineOfSight / LineOfWalk filtering inside the iterator. Goscape's existing single-find handlers carry deviation S7f-D1 (huntvis validated but not used as filter). NAI-33 carries this forward as **NAI-33-D1** to keep the FIND* family semantically symmetric. Retiring S7f-D1 + NAI-33-D1 together is a separate sub-spec that touches all FIND handlers + the `routefinder.LineValidator` integration surface.
- LOC iterator family port (`OpLocFindAllZone=3008` and any sibling `LOC_FINDALL*` opcodes). Same shape, separate sub-spec; NAI-33 establishes the template for it.
- Pointer-set/require enforcement at the VM level. None of goscape's existing FIND handlers enforce TS's `set ['active_npc']` / `require ['find_npc']` machinery; NAI-33 follows that convention. The nil-check on `s.npcIterator` in FINDNEXT is the practical equivalent of TS's `require ['find_npc']` (returns push-0 on missing iterator). Adding full VM-level pointer machinery is a separate cross-cutting refactor.
- `iter.Seq[ActiveNpc]` exposure on `NpcLookup`. The slice-returning `ZoneNpcs` is the minimal world-side surface; promoting it to `iter.Seq` adds plumbing for negligible gain at this scale (per-zone NPC count typically ≤10).
- NPC_FINDHERO (2519), NPC_FINDUID (2521), NPC_HUNTALL (2526) and other non-iterator FIND* opcodes. Different shapes (FINDHERO requires active_npc + sets active_player; FINDUID is keyed lookup; HUNTALL is a different iterator subtype `NpcHuntAllCommandIterator` per `ScriptIterators.ts`). Out of scope.

## Bundle structure

| Bundle | Surface | Cadence | Reviews | Commits |
|---|---|---|---|---|
| 0 | Controller pre-flight per `controller_preflight.md`: re-grep + Read every plan-time-deferred site at HEAD `a0852e0` (handler-dispatch registration site, `setActiveNpcSlot` location, validation-helper locations, `ScriptState` struct insertion line, `serverNpcLookup` zone-registry accessor name, mock-NpcLookup field names in tests). Verify `pkg/zone.Zone.NpcsSafe(true)` returns `iter.Seq[NpcLike]` and that `*Npc` satisfies both `NpcLike` and `script.ActiveNpc`. Read full TS `ScriptIterators.ts:297-363` body (already read once at brainstorm; re-read at plan-time to catch any iteration-order detail missed). | n/a (no commits, no subagent) | n/a | 0 |
| 1 | Iterator type + state field + four handlers + dispatch registration + `ZoneNpcs` interface method + `serverNpcLookup.ZoneNpcs` impl + 4-layer test suite. Single bundle (no surface natural to split: state field, iterator type, and FINDNEXT handler are mutually entangled). | Full TDD (red→green→commit) per `runescript_cadence.md`; subagent-driven-development per `execution_mode_default.md` | Two-stage (spec-compliance + code-quality) per `runescript_cadence.md` | 1-3 (one main feature commit; possible follow-up if the dispatch-table-registration site requires a separate touch, or if Layer 4 integration test surfaces a wiring gap) |
| Smoke | User-launched server (`smoke_test_server_handoff.md`); fishing NPC near a spawn-eligible coord; observe WARN absence + NPC relocation. | Binding feature-correctness gate | n/a | 0 (or 1 follow-up if Bundle 2 needed) |
| 2 (conditional) | Smoke-failure investigation + fix per `investigation_subspec_cadence.md`. Materialized only if smoke surfaces a layered bug (e.g., dispatch-table miss, type-assertion panic, S7f-D1-driven LoS issue we didn't anticipate). | Stage 1 audit → Stage 2 fix per `investigation_subspec_cadence.md` | Per audit shape | Conditional |
| Close | Standard close commit per `close_commit_memory_trailer.md`. New memory entries likely: an entry on the iterator-state pattern (template for LOC iterator family); possibly an entry on the cross-package `NpcLike → ActiveNpc` assertion if it surfaces a non-trivial gotcha. | n/a | n/a | 1 |

## Bundle 1 — NPC iterator family port

### Architecture

**Three-layer addition with one minimal interface boundary.**

```
pkg/script/                              modules/world/
┌─────────────────────────────────┐     ┌─────────────────────────────────┐
│ NpcIterator (new type)          │     │ serverNpcLookup                 │
│  - mode: Distance|Zone          │     │  - existing: FindClosest…       │
│  - cursor: zoneX/Z + intra-zone │◄────┤  - NEW: ZoneNpcs(lvl, zX, zZ)   │
│  - filters: dist + typeID       │     │           ↳ z := s.zones.<look> │
│  - lookup: NpcLookup (new mthd) │     │             for n := range      │
│  - creationTick                 │     │               z.NpcsSafe(true): │
│  - Next() (ActiveNpc, bool)     │     │                 out = append…   │
│  - Stale(tick) bool             │     │             return out          │
└─────────────────────────────────┘     └─────────────────────────────────┘
        ▲                       ▲
        │ owned by              │ extends
        │                       │
┌───────┴─────────────────┐  ┌──┴──────────────────────────┐
│ ScriptState (existing)  │  │ NpcLookup (existing iface)  │
│  - npcIterator (NEW)    │  │  - existing 3 methods       │
│  - existing fields…     │  │  - NEW: ZoneNpcs            │
└─────────────────────────┘  └─────────────────────────────┘
        ▲
        │ consumed by
        │
┌───────┴───────────────────────────────────────────────────┐
│ handlers_npc.go (NEW handlers — append after line 480)    │
│  handleNpcFindAll      (2514) — DISTANCE iter, typeID set │
│  handleNpcFindAllAny   (2515) — DISTANCE iter, typeID=-1  │
│  handleNpcFindAllZone  (2516) — ZONE iter                 │
│  handleNpcFindNext     (2520) — Stale-check + Next; sets  │
│                                  active_npc + push 1/0    │
└───────────────────────────────────────────────────────────┘
```

### `NpcIterator` data model and lifecycle

```go
package script

type NpcIteratorMode int

const (
    NpcIteratorDistance NpcIteratorMode = iota
    NpcIteratorZone
)

// NpcIterator is the script-VM iterator state for the NPC_FIND iterator
// family (NPC_FINDALL / NPC_FINDALLANY / NPC_FINDALLZONE). Mirrors TS
// NpcIterator at ScriptIterators.ts:297-363.
//
// Semantics: single-tick lifetime. Created by FINDALL*; consumed by
// FINDNEXT. Stale check in FINDNEXT compares creationTick to
// World.CurrentTick(); on miss, handler returns error → existing
// npc_script.go:167-172 log-warn + ClearActiveScript path runs (matches
// TS throw-on-stale).
type NpcIterator struct {
    mode         NpcIteratorMode
    creationTick int
    lookup       NpcLookup

    // Center + filter config
    level    int
    x, z     int
    distance int // DISTANCE mode only; 0 for ZONE
    huntvis  int // validated at handler; not used as filter (NAI-33-D1)
    typeID   int // -1 = no filter; else exact match on npc.Type()

    // Zone cursor (DISTANCE mode)
    minZoneX, maxZoneX int
    minZoneZ, maxZoneZ int
    curZoneX, curZoneZ int
    started            bool

    // Intra-zone snapshot (lazy: filled on zone-entry)
    zoneNpcs []ActiveNpc
    zoneIdx  int
}

func NewDistanceNpcIterator(lookup NpcLookup, tick, level, x, z, distance, huntvis, typeID int) *NpcIterator {
    centerX := x >> 3                 // CoordGrid.zone(x) per TS
    centerZ := z >> 3
    radius := 1 + distance/8          // (1 + distance / 8) | 0 per TS line 314
    return &NpcIterator{
        mode:         NpcIteratorDistance,
        creationTick: tick,
        lookup:       lookup,
        level:        level,
        x:            x,
        z:            z,
        distance:     distance,
        huntvis:      huntvis,
        typeID:       typeID,
        minZoneX:     centerX - radius,
        maxZoneX:     centerX + radius,
        minZoneZ:     centerZ - radius,
        maxZoneZ:     centerZ + radius,
        curZoneX:     centerX + radius, // start at maxX per TS line 337
        curZoneZ:     centerZ + radius, // start at maxZ per TS line 339
    }
}

func NewZoneNpcIterator(lookup NpcLookup, tick, level, x, z int) *NpcIterator {
    return &NpcIterator{
        mode:         NpcIteratorZone,
        creationTick: tick,
        lookup:       lookup,
        level:        level,
        x:            x,
        z:            z,
        typeID:       -1, // not used in ZONE mode
    }
}

func (it *NpcIterator) Stale(currentTick int) bool {
    return currentTick != it.creationTick
}

// Next advances and returns the next NPC. Returns (nil, false) on
// exhaustion. Caller is responsible for calling Stale() first if the
// single-tick lifetime invariant matters (FINDNEXT handler does this).
func (it *NpcIterator) Next() (ActiveNpc, bool) {
    for {
        // Drain current intra-zone snapshot
        for it.zoneIdx < len(it.zoneNpcs) {
            npc := it.zoneNpcs[it.zoneIdx]
            it.zoneIdx++
            if it.passesFilter(npc) {
                return npc, true
            }
        }
        // Advance zone cursor (DISTANCE only) or terminate (ZONE)
        if !it.advanceZone() {
            return nil, false
        }
        it.zoneNpcs = it.lookup.ZoneNpcs(it.level, it.curZoneX*8, it.curZoneZ*8)
        it.zoneIdx = 0
    }
}

// passesFilter applies the per-NPC filter chain in TS line 345-356 order.
// huntvis filtering is deferred (NAI-33-D1 / S7f-D1 carryover).
// Accessor names match pkg/script/active.go:400-408 ActiveNpc interface
// (NpcX/NpcZ/NpcType, not X/Z/Type).
func (it *NpcIterator) passesFilter(npc ActiveNpc) bool {
    if it.mode == NpcIteratorZone {
        return true // ZONE mode: no per-NPC filtering per TS line 329-335
    }
    if coordgrid.DistanceToSW(it.x, it.z, npc.NpcX(), npc.NpcZ()) > it.distance {
        return false
    }
    // huntvis filter intentionally omitted — NAI-33-D1 carryover
    if it.typeID >= 0 && npc.NpcType() != it.typeID {
        return false
    }
    return true
}

// advanceZone returns false when the zone cursor has been exhausted.
// For ZONE mode, returns true exactly once (the initial single-zone visit).
// For DISTANCE mode, walks (curZoneX, curZoneZ) from (max, max) toward
// (min, min) — outer X descending, inner Z descending — per TS line 337-340.
func (it *NpcIterator) advanceZone() bool {
    if it.mode == NpcIteratorZone {
        if it.started {
            return false
        }
        it.started = true
        // First (and only) ZONE-mode fetch: target the zone containing the
        // center coord. Set curZoneX/Z to zone-indices; Next() multiplies
        // by 8 before passing to ZoneNpcs (which expects coord-grid coords).
        it.curZoneX = it.x >> 3
        it.curZoneZ = it.z >> 3
        return true
    }
    // DISTANCE mode
    if !it.started {
        it.started = true
        return true // initial cursor already at (max, max) from constructor
    }
    // inner z--; if z < minZ, reset z to maxZ and outer x--; if x < minX, done
    it.curZoneZ--
    if it.curZoneZ < it.minZoneZ {
        it.curZoneZ = it.maxZoneZ
        it.curZoneX--
        if it.curZoneX < it.minZoneX {
            return false
        }
    }
    return true
}
```

**Plan-author note (per `plan_var_name_collision.md`)**: mentally compile each function body — `it.curZoneX*8` operands, no `:=` redeclaration of any field name. ZONE-mode `advanceZone` mutates `curZoneX/Z` on first call; this is the only state mutation in the ZONE path beyond `started=true`. Pre-flight grep test should confirm `curZoneX`/`curZoneZ` are not referenced before `advanceZone()` runs.

### Handler dispatch (the four new handlers)

All four append to `pkg/script/handlers_npc.go` after `handleNpcFindExact` (~line 480), following the existing checked-pop / push-on-conditional-set pattern.

**`handleNpcFindAllAny` (NPC_FINDALLANY, opcode 2515)** — the proximate fix.
```go
// pops (coord, distance, checkVis); pop order matches TS popInts(3) reverse
func handleNpcFindAllAny(s *ScriptState) error {
    checkVis := s.PopInt()
    distance := s.PopInt()
    coord := s.PopInt()

    level, x, z, err := checkCoord(coord, "NPC_FINDALLANY")
    if err != nil {
        return err
    }
    if err := checkNotNull(distance, "NPC_FINDALLANY"); err != nil {
        return err
    }
    if err := checkHuntVis(checkVis, "NPC_FINDALLANY"); err != nil {
        return err
    }

    s.npcIterator = NewDistanceNpcIterator(
        s.Npcs, s.World.CurrentTick(),
        level, x, z, distance, checkVis, /*typeID=*/ -1,
    )
    return nil
}
```
No push (TS doesn't push either; pointer-set is `set ['find_npc']` only).

**`handleNpcFindAll` (NPC_FINDALL, opcode 2514)** — same shape plus the npcType filter.
```go
func handleNpcFindAll(s *ScriptState) error {
    checkVis := s.PopInt()
    distance := s.PopInt()
    npcTypeID := s.PopInt()
    coord := s.PopInt()

    level, x, z, err := checkCoord(coord, "NPC_FINDALL")
    if err != nil {
        return err
    }
    if err := checkNotNull(distance, "NPC_FINDALL"); err != nil {
        return err
    }
    if err := checkNpcType(s, npcTypeID, "NPC_FINDALL"); err != nil {
        return err
    }
    if err := checkHuntVis(checkVis, "NPC_FINDALL"); err != nil {
        return err
    }

    s.npcIterator = NewDistanceNpcIterator(
        s.Npcs, s.World.CurrentTick(),
        level, x, z, distance, checkVis, npcTypeID,
    )
    return nil
}
```

**`handleNpcFindAllZone` (NPC_FINDALLZONE, opcode 2516)** — single-zone setup.
```go
func handleNpcFindAllZone(s *ScriptState) error {
    coord := s.PopInt()
    level, x, z, err := checkCoord(coord, "NPC_FINDALLZONE")
    if err != nil {
        return err
    }
    s.npcIterator = NewZoneNpcIterator(s.Npcs, s.World.CurrentTick(), level, x, z)
    return nil
}
```
TS validates only `coord` (no distance, no huntvis, no type). Mirror exactly.

**`handleNpcFindNext` (NPC_FINDNEXT, opcode 2520)** — the consumer.
```go
func handleNpcFindNext(s *ScriptState) error {
    it := s.npcIterator
    if it == nil {
        s.PushInt(0)
        return nil
    }
    if it.Stale(s.World.CurrentTick()) {
        return fmt.Errorf("NPC_FINDNEXT: tried to use an old iterator. Create a new iterator instead.")
    }
    npc, ok := it.Next()
    if !ok {
        s.PushInt(0)
        return nil
    }
    setActiveNpcSlot(s, npc)
    s.PushInt(1)
    return nil
}
```

**Quirks to pin in tests** (per `ts_asymmetry_dual_pin.md` and `test_passes_for_wrong_reason.md`):
- FINDALLANY pops 3, FINDALL pops 4, FINDALLZONE pops 1, FINDNEXT pops 0 — pin per-handler.
- Pop ORDER (checkVis on top) — pin via mock-state with distinguishable values (e.g., coord=10001, distance=10002, checkVis=10003) so a swapped pop produces wrong validation behavior.
- TS error message verbatim: "tried to use an old iterator. Create a new iterator instead." — pin substring.
- FINDNEXT exhaustion does NOT clear `s.npcIterator` — pin via assertion that the field remains non-nil after a push-0 exhaustion result.
- Iterator zone-cursor walks `(maxX, maxZ) → (maxX, minZ) → (maxX-1, maxZ) → … → (minX, minZ)` — outer X descending, inner Z descending per TS line 337-340. Pin order with a mock-NpcLookup recording the call sequence.

### `ZoneNpcs` world-side implementation

**Single new method on `script.NpcLookup`** (extends interface at `pkg/script/state.go:67-80`):
```go
// ZoneNpcs returns all NPCs subscribed to the zone at (level, zoneX, zoneZ),
// filtered by IsValid. Mirrors TS Zone.getAllNpcsSafe(true) consumed by
// NpcIterator.generator. zoneX/zoneZ are coord-grid coords (not zone
// indices); the impl converts via z.x/z.z = zoneX & ~7. Empty slice
// (or nil) on miss. No error path.
ZoneNpcs(level, zoneX, zoneZ int) []ActiveNpc
```

**`modules/world/npc_script_lookup.go` adds** (verify zone-registry accessor name at plan-time per `controller_preflight.md`):
```go
func (l serverNpcLookup) ZoneNpcs(level, zoneX, zoneZ int) []ActiveNpc {
    z := l.s.zones.Lookup(level, zoneX, zoneZ) // PLACEHOLDER: verify accessor name
    if z == nil {
        return nil
    }
    out := make([]ActiveNpc, 0, z.NpcsCount())
    for n := range z.NpcsSafe(true) {
        out = append(out, n.(ActiveNpc)) // *Npc satisfies both NpcLike and ActiveNpc
    }
    return out
}
```

**Plan-time verifications** (per `controller_preflight.md`, must run before plan dispatch):
1. Exact accessor name on `s.zones` for "fetch zone at coord-grid (level, x, z)" — likely `Lookup`, `At`, `Get`, or `ZoneAt`. Grep `s.s.zones\.` in `modules/world/*.go`.
2. Confirm `*Npc` satisfies both `pkg/zone.NpcLike` AND `pkg/script.ActiveNpc` at HEAD `a0852e0` — grep for `var _ pkg/zone.NpcLike = (*Npc)(nil)` and `var _ pkg/script.ActiveNpc = (*Npc)(nil)` (or equivalent).
3. Confirm `pkg/zone.Zone.NpcsSafe(reverse bool)` returns `iter.Seq[NpcLike]` (verified at brainstorm at `zone.go:439`; re-grep at HEAD).
4. Locate handler-dispatch registration site for the 4 new opcodes. Likely an `init()` block populating a `map[Opcode]Handler` in `pkg/script/handlers_npc.go` or a central switch in `pkg/script/executor.go`. Grep `OpNpcFind\b` at HEAD to find where the existing 3 are registered.
5. Locate `setActiveNpcSlot` definition. Likely in `pkg/script/handlers_npc.go` near line 410 (used by `handleNpcFind` at line 411). Confirm signature: `setActiveNpcSlot(s *ScriptState, npc ActiveNpc)`.

### `ScriptState` integration

**One new field, no termination-path cleanup needed.**

```go
// in pkg/script/state.go ScriptState struct (alongside existing ActiveNpc field)
//
// npcIterator holds the active NPC_FIND iterator state. Set by
// FINDALL/FINDALLANY/FINDALLZONE; consumed by FINDNEXT. Lifetime is
// single-tick — Stale() check enforced at FINDNEXT against
// s.World.CurrentTick(). Nil = no active iterator. Mirrors TS
// ScriptState.npcIterator (ScriptState.ts:125).
npcIterator *NpcIterator
```

Lowercase (package-private). Handlers in same package access directly; no exported getter.

**Lifecycle invariants (verified non-impact):**
- `Aborted` / `Finished` (npc_script.go:175): ScriptState is dropped → iterator GC'd. Nothing to add.
- `NpcSuspended` (npc_script.go:177): state persists; on next-tick resume, `Stale()` guarantees the iterator can't be misused. Nothing to add.
- `script.Execute` returns error (npc_script.go:169): `ClearActiveScript()` already runs; state dropped. Nothing to add.

**`script.Execute` is unchanged.** Only the opcode-table registration grows by 4 entries.

**Plan-time verification**: how does goscape currently zero `ScriptState`? If via zero-value (struct literal without explicit field assignments), the `npcIterator *NpcIterator` field zero-initializes to nil — no explicit nil-set needed. If via an explicit `Init()` constructor, add `s.npcIterator = nil` defensively.

### Testing

**Layer 1 — `NpcIterator` mechanics (`pkg/script/npc_iterator_test.go`, NEW file)**

| Test | Asserts |
|---|---|
| `TestNpcIterator_DistanceMode_BoundsMath` | constructor sets `min/maxZoneX/Z` per `(x>>3) ± (1 + distance/8)` for `(x, z, distance)` triples covering boundary cases: `distance=0` (radius 1), `distance=8` (radius 2), `distance=15` (radius 2), `distance=16` (radius 3) |
| `TestNpcIterator_DistanceMode_CursorOrder` | with mock `NpcLookup` recording `(level, zoneX, zoneZ)` calls, iterator visits zones in `(maxX, maxZ) → (maxX, maxZ-1) → … → (maxX, minZ) → (maxX-1, maxZ) → … → (minX, minZ)` order. Outer X descending, inner Z descending. Per TS line 337-340. Dual-pin per `ts_asymmetry_dual_pin.md` |
| `TestNpcIterator_DistanceMode_DistanceFilter` | NPCs with `DistanceToSW > distance` skipped; `<= distance` yielded. Synthetic NPCs at exact-distance + 1-over + 1-under |
| `TestNpcIterator_DistanceMode_TypeFilter` | with `typeID=42`, only NPCs with `Type()==42` yielded. With `typeID=-1`, all NPCs yielded. Both positive AND negative branch covered per `test_passes_for_wrong_reason.md` |
| `TestNpcIterator_ZoneMode_SingleZone` | only zone at `(x>>3, z>>3, level)` queried; no distance/type filter applied; all NPCs yielded |
| `TestNpcIterator_ZoneMode_TerminatesAfterOneZone` | second `Next()` after exhausting the single zone returns `(nil, false)` |
| `TestNpcIterator_StaleCheck` | `Stale(currentTick)` returns true iff `currentTick != creationTick` |

**Layer 2 — Handler tests (`pkg/script/handlers_npc_test.go`, append to existing file)**

For each of the 4 handlers:
- Pop-order test using distinguishable values (coord=10001, distance=10002, checkVis=10003, npcTypeID=10004 for FINDALL).
- Validation-error tests: invalid coord, null distance, invalid huntvis, invalid npcType (where applicable). Each error message substring pinned (e.g., `"NPC_FINDALLANY: coord out of range"`).
- Side-effect test: `s.npcIterator` is non-nil after FINDALL*; correct mode (Distance vs Zone); typeID is `-1` for FINDALLANY/FINDALLZONE, set for FINDALL; `creationTick == World.CurrentTick()`.

FINDNEXT-specific tests (pin all 4 termination branches):
- Nil iterator → push 0, no error.
- Stale iterator → error with substring `"tried to use an old iterator"`.
- Hit → `setActiveNpcSlot` called with returned NPC, push 1, active_npc set.
- Exhaustion → push 0, `s.npcIterator` NOT cleared (matches TS).

Pre-flight per `mock_recorder_field_naming_check.md`: grep existing mock `NpcLookup` field names in `handlers_npc_test.go` before authoring; reuse rather than invent. Add `ZoneNpcs` stub method to mock with field `ZoneNpcsCalls []zoneNpcsCall` + `ZoneNpcsReturn map[zoneKey][]ActiveNpc` (or reuse existing mock-recorder convention if one exists).

**Layer 3 — World-side `serverNpcLookup.ZoneNpcs` test (`modules/world/npc_script_lookup_test.go`)**

| Test | Asserts |
|---|---|
| `TestServerNpcLookup_ZoneNpcs_Empty` | empty zone → empty/nil slice |
| `TestServerNpcLookup_ZoneNpcs_Single` | zone with 1 NPC → slice of length 1 |
| `TestServerNpcLookup_ZoneNpcs_MultipleNpcsReverseOrder` | zone with N NPCs → slice in `NpcsSafe(true)` order (reverse) |
| `TestServerNpcLookup_ZoneNpcs_FiltersInvalid` | zone with mix of valid + invalid NPCs (post-`removeNpc` but pre-cleanup) → only IsValid NPCs returned |
| `TestServerNpcLookup_ZoneNpcs_OffGrid` | zone coord outside the world grid → nil/empty (no panic) |
| `TestServerNpcLookup_ZoneNpcs_OnlyRequestedZone` | populate two adjacent zones; query one; only that zone's NPCs returned |

**Layer 4 — Integration sanity test (`pkg/script/handlers_npc_test.go`, append)**

`TestIteratorFamily_Integration_FullLoop`: handcrafted bytecode runs PUSH coord / PUSH 0 / PUSH 0 / FINDALLANY / loop {FINDNEXT → if 0 break; assert active_npc set; ...}. Verifies the full state-binding path works across opcode dispatch boundaries — catches "handler runs but `state.npcIterator` isn't visible to FINDNEXT" failures (per `protocol_stub_not_completed.md`, declared-but-unwired bugs survive single-handler-unit tests).

**Plan-time test-fixture pre-flight per `plan_runnable_test_fixtures.md`**: mentally execute (or `go test -count=1` dry-run) every test fixture in this spec before plan-author dispatch; especially the bounds-math triples, the cursor-order expected sequence, and the integration-test bytecode push order.

**Smoke test (binding gate)** per `smoke_test_server_handoff.md`:
- User restarts the dev server.
- Find a fishing-NPC that's idle on a `move_fishing_spot` AI timer (any NPC of category freshfish/saltfish/rarefish/memberfish or NPC type `0_45_152_lavafish`).
- Wait at least one full `npc_settimer(calc(280 + random(250)))` interval (~3-9 minutes).
- Confirm:
  1. Original WARN (`no handler for NPC_FINDALLANY (opcode 2515)`) is gone.
  2. No new WARN at NPC_FINDNEXT (opcode 2520).
  3. Fishing NPCs visibly relocate between spots over time.

Per `smoke_test_server_handoff.md`, Claude cannot launch the server itself (sandboxed); user must run it.

### Deviations introduced

- **NAI-33-D1**: `huntvis` validated but not used as filter (LineOfSight / LineOfWalk skipped) inside the iterator's `passesFilter`. Carryover of the existing **S7f-D1** posture across the new iterator family. Rationale: keep iterator + single-find ops semantically symmetric until LoS/LoW is wired across the entire FIND* family in one sweep. Retire alongside S7f-D1.

**No deviations introduced for:**
- Stale-tick semantics (mirrors TS throw via existing error → log-warn → clear-script path).
- Pop order (matches TS `popInts(N)` reverse order).
- Zone iteration order (matches TS line 337-340 outer-X-desc / inner-Z-desc).
- Type-filter sentinel `-1` (goscape's existing convention for "no filter" — see `serverNpcLookup.FindClosestNpcByType`).
- FINDNEXT-doesn't-clear-iterator-on-exhaustion (matches TS `state.npcIterator?.next()` returning `{done:true}` without nulling).
- Lazy per-zone snapshot vs TS lazy generator (snapshot is functionally equivalent within a single tick; the stale-tick check guarantees no cross-tick semantic divergence).
- Slice vs `iter.Seq` for `ZoneNpcs` return (architectural choice, not behavioral; documented in Scope/Out-of-scope).

**Net deviation count: 14 → 14** (NAI-33-D1 carries existing S7f-D1 posture; introduces zero new behavioral divergence beyond what S7f already established).

## Plan-author pre-flight checklist (Bundle 0 surface)

Per `controller_preflight.md` and `spec_followup_tracker_freshness.md` — every plan-time-deferred verification gets a concrete grep target. Each must return a known-good result before plan dispatch:

1. `s.zones.<accessor>(level, zoneX, zoneZ)` — exact method name. Run `rg 's\.zones\.' modules/world/`.
2. `setActiveNpcSlot` — confirm location + signature. Run `rg 'func setActiveNpcSlot' pkg/script/`.
3. Validation helpers (`checkCoord`, `checkNotNull`, `checkHuntVis`, `checkNpcType`) — confirm locations + signatures. Run `rg 'func check(Coord|NotNull|HuntVis|NpcType)' pkg/script/`.
4. Handler dispatch registration site for opcodes 2513-2518 — find where the existing 3 FIND handlers are registered, append the 4 new ones in the same place. Run `rg 'OpNpcFind\b|handleNpcFind\b' pkg/script/`.
5. `ScriptState` struct definition + insertion line for `npcIterator` field (alongside existing `ActiveNpc`). Run `rg 'ActiveNpc\s+ActiveNpc' pkg/script/state.go`.
6. `*Npc` satisfies `pkg/script.ActiveNpc` AND `pkg/zone.NpcLike` at HEAD — re-grep `var _.*=.*\(\*Npc\)\(nil\)` in `modules/world/`.
7. `pkg/zone.Zone.NpcsSafe(true)` returns `iter.Seq[NpcLike]` at HEAD — re-grep `func.*NpcsSafe` in `pkg/zone/zone.go`.
8. Existing mock `NpcLookup` field names in `handlers_npc_test.go` — to extend rather than invent. Run `rg 'mockNpcLookup|fakeNpcLookup' pkg/script/handlers_npc_test.go`.
9. `pkg/coordgrid.DistanceToSW` signature unchanged from `(posX, posZ, otherX, otherZ int) int` — re-grep `func DistanceToSW` in `pkg/coordgrid/`.
10. Existing FIND handlers' import block in `handlers_npc.go` — confirm `fmt` already imported (FINDNEXT's stale-tick error needs it); confirm `pkg/coordgrid` import path for `passesFilter`'s `DistanceToSW` call (or place that call in the world-side impl instead — decision deferred to plan-time).
11. `pkg/script.ActiveNpc` interface accessor names at HEAD `a0852e0` — confirmed at brainstorm to be `NpcType()`, `NpcX()`, `NpcZ()` (NOT `Type()`/`X()`/`Z()`) per `pkg/script/active.go:400-408`. Re-grep at plan-time and ensure the iterator's `passesFilter` body and any Layer 1/2 test fixtures use the prefixed names. Also confirm `*Npc` (in `modules/world`) implements all three accessors — likely yes since existing FIND handlers already consume them, but the iterator is a new caller path.

## Close criteria

- All 4 layer-1-2-3-4 test suites pass: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...`
- Race-detector clean: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/script/... ./modules/world/...`
- Fishing-spot smoke (above) confirms WARN absence + visible NPC relocation.
- Two-stage review per `runescript_cadence.md` — spec-compliance pass + code-quality pass.
- Close commit with `Closes memory:` trailer per `close_commit_memory_trailer.md` if any new memory entries are added (likely: an entry on the iterator-state pattern as the LOC-iterator-family template).
