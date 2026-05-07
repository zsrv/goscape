# NAI-119 — LOC_FIND iterator family port (LOC_FINDALLZONE + LOC_FINDNEXT)

**Cadence:** Standard 15-100 LOC band per `compressed_cadence` — separate spec + plan; combined single review at end (no per-task two-stage).

**Tech stack:** Go 1.26+ (`pkg/script`).

---

## §1. Background

Surfaced by NAI-118 smoke (2026-05-06): leaving the Mining Instructor area to the combat area triggers `[proc,tut_open_mining_gate]` (`LostCityRS/Content/scripts/tutorial/scripts/tut_doors_and_gates.rs2:131`), which calls:

```
[proc,tut_open_mining_gate]
loc_findallzone(coord);
while(loc_findnext = true) {
    if(loc_category = tut_mining_exit) {
        ...
        loc_del(3);
        if(loc_type = newbiedoor4l) { loc_add(...); ... }
        loc_add($central_coord, inviswall, $orig_angle, loc_shape, 3);
    }
}
sound_synth(door_open, 0, 0);
```

`loc_findallzone` (opcode 3008) and `loc_findnext` (opcode 3009) are unwired in goscape (only `OpLocFind` 3007 has a dispatch entry at `pkg/script/handlers.go:138`, and even that is a stub at `handlers_loc.go:31-36` that always pushes 0). The script aborts at pc=1 with `script: no handler for LOC_FINDALLZONE`.

The downstream `loc_category`, `loc_coord`, `loc_angle`, `loc_type`, `loc_shape`, `loc_del`, and `loc_add` handlers all already exist. Wiring the iterator pair is the entire fix for the gate proc.

---

## §2. TS reference (verified at HEAD)

### §2.1 `Engine-TS/src/engine/script/handlers/LocOps.ts:96-112`

```typescript
[ScriptOpcode.LOC_FINDALLZONE]: state => {
    const coord: CoordGrid = check(state.popInt(), CoordValid);
    state.locIterator = new LocIterator(World.currentTick, coord.level, coord.x, coord.z);
},

[ScriptOpcode.LOC_FINDNEXT]: state => {
    const result = state.locIterator?.next();
    if (!result || result.done) {
        state.pushInt(0);
        return;
    }
    state.activeLoc = result.value;
    state.pointerAdd(ActiveLoc[state.intOperand]);
    state.pushInt(1);
},
```

### §2.2 `Engine-TS/src/engine/script/ScriptIterators.ts:365-385`

```typescript
export class LocIterator extends ScriptIterator<Loc> {
    private readonly level: number;
    private readonly x: number;
    private readonly z: number;

    constructor(tick: number, level: number, x: number, z: number) {
        super(tick);
        this.level = level;
        this.x = x;
        this.z = z;
    }

    protected *generator(): IterableIterator<Loc> {
        for (const loc of World.gameMap.getZone(this.x, this.z, this.level).getAllLocsSafe(true)) {
            if (World.currentTick > this.tick) {
                throw new Error('[LocIterator] tried to use an old iterator. Create a new iterator instead.');
            }
            yield loc;
        }
    }
}
```

Single-zone, no filtering, stale-check on each yield. Much simpler than the NPC family (no DISTANCE/HuntAll modes, no per-NPC filter chain).

---

## §3. Goscape mapping

### §3.1 Existing infra (verified at HEAD `9de73a7`)

- **`pkg/script/loc_ops.go:25`** — `LocOps.AllLocsInZone(level, x, z) []ActiveLoc`. Already exists; consumed by NAI-114 MAP_LOCADDUNSAFE. Implementation at `modules/world/script_loc_ops.go:85-92` — returns the zone's `Locs` slice copy, no per-tile filtering. Drop-in snapshot source.
- **`pkg/script/handlers_npc.go:64-83`** — `setActiveNpcSlot` reference template for the dual-slot helper.
- **`pkg/script/handlers_npc.go:704-719`** — `handleNpcFindAllZone` reference template.
- **`pkg/script/handlers_npc.go:778-795`** — `handleNpcFindNext` reference template.
- **`pkg/script/npc_iterator.go:32-64, 66-74, 184-199`** — `NpcIterator` ZONE-mode reference template.
- **`pkg/script/pointer.go:12-13`** — `PtrActiveLoc` and `PtrActiveLoc2` flags already exist (the latter unused at HEAD).
- **`pkg/script/state.go:234`** — `ActiveLoc ActiveLoc` field exists; `OtherActiveLoc` does NOT.
- **`pkg/script/opcode.go:296-298`** — `OpLocFind`, `OpLocFindAllZone`, `OpLocFindNext` constants already declared.

### §3.2 What's missing

1. `LocIterator` type (no analogue exists for Loc).
2. `OtherActiveLoc ActiveLoc` field on `ScriptState`.
3. `locIterator *LocIterator` field on `ScriptState`.
4. `setActiveLocSlot` helper.
5. `handleLocFindAllZone` (opcode 3008) handler + dispatch.
6. `handleLocFindNext` (opcode 3009) handler + dispatch.
7. Tests: iterator-level (in `loc_iterator_test.go`) + handler-level (in `handlers_loc_test.go`).

---

## §4. Architecture

### §4.1 `pkg/script/loc_iterator.go` (new file)

```go
package script

// LocIterator is the script-VM iterator state for the LOC_FIND iterator
// family (currently LOC_FINDALLZONE only — the LOC iterator family is
// single-mode unlike NpcIterator's DISTANCE/ZONE/HuntAll). Mirrors TS
// LocIterator at ScriptIterators.ts:365-385.
//
// Lifetime: single-tick. Created by LOC_FINDALLZONE; consumed by
// LOC_FINDNEXT. Stale() check at FINDNEXT compares creationTick to
// World.CurrentTick(); on mismatch, handler returns an error mirroring
// the NPC family pattern (npc_script.go:167-172 catches and clears the
// active script).
//
// Snapshot strategy: lazy on first Next() call via
// LocOps.AllLocsInZone(level, x, z). Subsequent calls drain the
// snapshot. TS uses a generator over `getZone(...).getAllLocsSafe(true)`
// — equivalent because both produce a single point-in-time slice that
// the iterator drains independent of subsequent zone mutation.
//
// Ownership: held by ScriptState.locIterator. Nil = no active iterator.
type LocIterator struct {
    creationTick int
    ops          LocOps
    level, x, z  int
    locs         []ActiveLoc
    idx          int
    started      bool
}

// NewZoneLocIterator constructs a single-zone iterator for the zone
// containing (level, x, z). Mirrors TS LocIterator constructor at
// ScriptIterators.ts:370-374. The snapshot is deferred to first Next();
// the constructor only stores center coords and tick.
func NewZoneLocIterator(ops LocOps, tick, level, x, z int) *LocIterator {
    return &LocIterator{
        creationTick: tick,
        ops:          ops,
        level:        level,
        x:            x,
        z:            z,
    }
}

// Stale reports whether the iterator was created in a prior tick. The
// FINDNEXT handler MUST check this before calling Next when single-tick
// lifetime matters. Mirrors TS strict-greater-than at
// ScriptIterators.ts:379 (World.currentTick > this.tick).
func (it *LocIterator) Stale(currentTick int) bool {
    return currentTick > it.creationTick
}

// Next returns the next loc in the zone snapshot, or (nil, false) on
// exhaustion. Lazy-initializes the snapshot on first call.
//
// Nil-ops degrades to immediate exhaustion (test stub or pre-wiring) —
// mirrors NpcIterator.Next nil-lookup handling
// (npc_iterator.go:238-240).
func (it *LocIterator) Next() (ActiveLoc, bool) {
    if !it.started {
        it.started = true
        if it.ops != nil {
            it.locs = it.ops.AllLocsInZone(it.level, it.x, it.z)
        }
    }
    if it.idx >= len(it.locs) {
        return nil, false
    }
    loc := it.locs[it.idx]
    it.idx++
    return loc, true
}
```

### §4.2 `pkg/script/state.go` — field adds

Add immediately after `ActiveLoc ActiveLoc` (line 234):

```go
// OtherActiveLoc is the secondary Loc slot, parallel to OtherActiveNpc
// (NAI-11). Set by LOC_FINDNEXT when the bytecode IntOperand is 1
// (.loc2 syntax). NAI-119.
//
// NAI-119-D-NO-DOWNSTREAM-LOC2-CONSUMERS: no existing LOC_* read
// handler reads from this slot at HEAD — they all read s.ActiveLoc
// only. Tracked deviation; closure when a `.loc2` content-script
// consumer surfaces.
OtherActiveLoc ActiveLoc
```

Add immediately after `npcIterator *NpcIterator` (line 250):

```go
// locIterator holds the active LOC_FIND iterator state. Set by
// LOC_FINDALLZONE; consumed by LOC_FINDNEXT. Lifetime is single-tick
// — Stale() check enforced at FINDNEXT against s.World.CurrentTick().
// Nil = no active iterator. Mirrors TS ScriptState.locIterator. NAI-119.
locIterator *LocIterator
```

### §4.3 `pkg/script/handlers_loc.go` — helpers + handlers

Add at the top of the file (after `requireActiveLoc`):

```go
// setActiveLocSlot writes the loc to either ActiveLoc (primary) or
// OtherActiveLoc (secondary) based on the handler's IntOperand and sets
// the corresponding Pointer flag. Mirrors TS
// state.pointerAdd(ActiveLoc[state.intOperand]) at LocOps.ts:110, and
// the parallel setActiveNpcSlot at handlers_npc.go:64-83.
//
// IntOperand==0 → ActiveLoc/PtrActiveLoc (.loc syntax).
// IntOperand==1 → OtherActiveLoc/PtrActiveLoc2 (.loc2 syntax).
// Any other value panics (compiler invariant — bytecode only emits 0/1).
func setActiveLocSlot(s *ScriptState, loc ActiveLoc) {
    operand := s.Script.IntOperands[s.PC]
    switch operand {
    case 0:
        s.ActiveLoc = loc
        s.Pointers |= PtrActiveLoc
    case 1:
        s.OtherActiveLoc = loc
        s.Pointers |= PtrActiveLoc2
    default:
        panic(fmt.Sprintf("setActiveLocSlot: invalid IntOperand %d", operand))
    }
}
```

Add the two new handlers (placement: alphabetical or near `handleLocFind`):

```go
// handleLocFindAllZone (LOC_FINDALLZONE, opcode 3008) pops a coord,
// validates, and stores a single-zone LocIterator targeting the zone
// containing that coord. Mirrors TS LocOps.ts:96-100.
//
// Nil-LocOps degrades silently (matches NPC_FINDALLZONE convention at
// handlers_npc.go:714-716).
func handleLocFindAllZone(s *ScriptState) error {
    coord := s.PopInt()
    level, x, z, err := checkCoord(coord, "LOC_FINDALLZONE")
    if err != nil {
        return err
    }
    if s.LocOps == nil {
        return nil
    }
    s.locIterator = NewZoneLocIterator(s.LocOps, s.World.CurrentTick(), level, x, z)
    return nil
}

// handleLocFindNext (LOC_FINDNEXT, opcode 3009) advances the active
// LocIterator and either sets active_loc + pushes 1 on hit, or pushes 0
// on miss / nil-iterator. Mirrors TS LocOps.ts:102-112.
//
// Stale-iterator semantics: mirror NPC_FINDNEXT (handlers_npc.go:778-795)
// — return error on stale; existing script_loc_ops.go runtime path
// catches and ClearActiveScripts (parallel to npc_script.go:167-172).
//
// Pointer-set: setActiveLocSlot threads IntOperand 0/1 to choose
// primary/secondary slot per TS state.pointerAdd(ActiveLoc[intOperand]).
func handleLocFindNext(s *ScriptState) error {
    it := s.locIterator
    if it == nil {
        s.PushInt(0)
        return nil
    }
    if it.Stale(s.World.CurrentTick()) {
        return fmt.Errorf("LOC_FINDNEXT: tried to use an old iterator. Create a new iterator instead.")
    }
    loc, ok := it.Next()
    if !ok {
        s.PushInt(0)
        return nil
    }
    setActiveLocSlot(s, loc)
    s.PushInt(1)
    return nil
}
```

### §4.4 `pkg/script/handlers.go` — dispatch entries

Add to the existing handler map near `OpLocFind: handleLocFind` (line 138):

```go
OpLocFind:        handleLocFind,
OpLocFindAllZone: handleLocFindAllZone,
OpLocFindNext:    handleLocFindNext,
```

---

## §5. Tests

### §5.1 `pkg/script/loc_iterator_test.go` (new file)

| Test | Pin |
|---|---|
| `TestLocIteratorStaleAtSameTick` | Stale returns false when currentTick == creationTick (per TS strict `>`). |
| `TestLocIteratorStaleNextTick` | Stale returns true when currentTick > creationTick. |
| `TestLocIteratorYieldsAllZoneLocs` | fakeLocOps seeded with 3 locs in target zone; Next returns all 3 in slice order, then (nil, false). |
| `TestLocIteratorEmptyZone` | fakeLocOps returns empty slice; first Next returns (nil, false). |
| `TestLocIteratorExhaustionDoesNotClear` | After exhaustion, repeat Next() calls keep returning (nil, false) without panic; iterator state preserved (no nilling). |
| `TestLocIteratorNilOpsDegrades` | ops=nil; first Next returns (nil, false). |
| `TestNewZoneLocIteratorStoresFields` | Constructor pins level/x/z/creationTick exactly. |

Use the existing `fakeLocOps` at `pkg/script/loc_ops_test.go:6-61` (verified to have `AllLocsInZone` method). Extend with a per-test seeder helper if needed.

### §5.2 `pkg/script/handlers_loc_test.go` additions

| Test | Pin |
|---|---|
| `TestLocFindAllZoneStoresIterator` | Run `[push coord; LOC_FINDALLZONE]`; assert `s.locIterator != nil`, `creationTick == s.World.CurrentTick()`, level/x/z match the popped coord. |
| `TestLocFindAllZoneNilLocOpsDegrades` | Same script; `s.LocOps = nil`. Asserts no panic, `s.locIterator == nil`. |
| `TestLocFindAllZoneCoordValid` | Coord = -1; handler returns the `checkCoord` error. |
| `TestLocFindNextNoIterator` | s.locIterator nil; FINDNEXT pushes 0; no error. |
| `TestLocFindNextHitPrimarySlot` | fakeLocOps with one loc; iterator created; IntOperand=0; FINDNEXT pushes 1, sets `s.ActiveLoc`, sets `PtrActiveLoc`. |
| `TestLocFindNextHitSecondarySlot` | Same but `IntOperands[PC] = 1`; FINDNEXT sets `s.OtherActiveLoc`, sets `PtrActiveLoc2`. Pins NAI-119-B (dual-slot). |
| `TestLocFindNextExhaustionPushesZero` | Iterator drained; FINDNEXT pushes 0, does NOT touch ActiveLoc/OtherActiveLoc. |
| `TestLocFindNextStaleErrors` | Iterator created at tick=0; World.CurrentTick advanced to 1; FINDNEXT returns the "tried to use an old iterator" error matching the NPC wording. |

Test-fixture pattern: cribbed from existing `handlers_npc_test.go` FINDNEXT tests.

---

## §6. Deviations

- **`NAI-119-D-NO-DOWNSTREAM-LOC2-CONSUMERS`** — `OtherActiveLoc` field added but no LOC_* read handler at HEAD reads from it (LOC_TYPE / LOC_COORD / LOC_CATEGORY / etc. all read `s.ActiveLoc` only). TS read handlers also operate on `state.activeLoc` (primary) — but TS's pointer-set lets a `.loc2` script branch keep the secondary populated for cross-handler use. Goscape's read handlers haven't been parameterized over the slot. Tracked; closure when a `.loc2` content-script consumer surfaces. No current Tutorial Island scope consumer; verified via grep.

---

## §7. Verification

1. Both new test files (`loc_iterator_test.go`) and additions (`handlers_loc_test.go`) fail at HEAD `9de73a7`.
2. Apply §4 production diffs.
3. New tests pass.
4. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` clean.
5. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean.

---

## §8. Smoke (user-launched, post-merge)

Per `smoke_test_server_handoff`: ask user to launch the server and verify the gate proc binds.

**Smoke target:** Mining Instructor → combat area mining gate (NAI-118 SECONDARY residual).

**Procedure:**
1. Java client login.
2. Tutorial Island, past Mining Instructor smithing step.
3. Approach the mining-area exit (the gate that calls `tut_open_mining_gate`).

**PRIMARY bind:**
- No `script: no handler for LOC_FINDALLZONE` error in server log.
- No `script: no handler for LOC_FINDNEXT` error.
- Gate visually opens (door4l locs replaced with inviswall + neighboring locs per the script body).
- `door_open` sound plays.
- Player can walk through to the combat area.

**Secondary check:** No regression in any existing LOC_* handler (LOC_FIND stub, LOC_ADD, LOC_DEL, LOC_CHANGE, LOC_ANIM, LOC_PARAM, etc. — they all read `s.ActiveLoc` directly and are unaffected by the new `OtherActiveLoc` field).

---

## §9. Out of scope

- **`LOC_FIND` (opcode 3007) real implementation.** Currently stubbed at `handlers_loc.go:31-36`. Not consumed by `tut_open_mining_gate`; deferred to a separate sub-spec when a content consumer needs it. The stub-vs-real divergence isn't a regression — scripts that read its return value already take the "not found" branch correctly.
- **`OBJ_FIND` family (`OBJ_FINDALLZONE` 3010, `OBJ_FINDNEXT` 3011).** TS has parallel `ObjIterator` at `ScriptIterators.ts:387-407`. No current content consumer surfaced; future sub-spec when needed.
- **LOC_* read-handler parameterization over slot.** Currently every LOC read reads `s.ActiveLoc`; making them honor `IntOperand` to read primary vs. secondary is the closure for `NAI-119-D-NO-DOWNSTREAM-LOC2-CONSUMERS`. Deferred until a `.loc2` consumer surfaces.
- **HuntAll/Distance modes for LOC.** TS LocIterator has only single-zone mode; no scope expansion needed.

---

## §10. Plan cadence

Cadence B (per `compressed_cadence`): spec + plan, single combined review at end.

The plan doc will lay out three sequential tasks dispatched as Sonnet implementer subagents:

- **Task 1** — Iterator infra. New file `pkg/script/loc_iterator.go` (type + constructor + Stale + Next per §4.1) + `pkg/script/state.go` field adds (per §4.2) + new file `pkg/script/loc_iterator_test.go` (the 7 tests per §5.1). TDD red→green→commit.
- **Task 2** — Handler dispatch. `pkg/script/handlers_loc.go` additions (per §4.3) + `pkg/script/handlers.go` dispatch wire-up (per §4.4) + `pkg/script/handlers_loc_test.go` additions (the 8 tests per §5.2). TDD red→green→commit. Depends on Task 1 (uses `s.locIterator`, `s.OtherActiveLoc`, `LocIterator`, `NewZoneLocIterator`).
- **Task 3** — Combined final review across both commits + fresh `go test ./...` + `go vet ./...`. Single review subagent (NOT per-task two-stage). Reports DONE / DONE_WITH_CONCERNS / BLOCKED.

After review approves: handoff to user for §8 smoke. On smoke bind: close commit per `close_commit_memory_trailer` with `Closes memory:` trailer.

---

## §11. Pattern memories applied

- `iterator_state_pattern` — single-tick iterator template (custom struct + state field + Lookup-style snapshot + Stale check), reused for LOC family. NAI-33 reference impl.
- `controller_preflight` — Bundle 0 verified all anchor lines/symbols at HEAD `9de73a7` (NPC iterator template, LocOps interface, OtherActiveNpc/PtrActiveLoc2 existence, opcode constants, state field offsets).
- `audit_full_method_against_ts` — TS LocIterator + LOC_FINDALLZONE + LOC_FINDNEXT all read end-to-end from primary sources (ScriptIterators.ts:365-385, LocOps.ts:96-112).
- `plan_helper_coverage` — `setActiveLocSlot` mirrors `setActiveNpcSlot` exactly; both threading the same `s.Script.IntOperands[s.PC]` + same panic-on-other-operand invariant.
- `compressed_cadence` — Cadence B (single combined review, not per-task two-stage) chosen for ~100 LOC well-templated work.
- `smoke_test_server_handoff` — user-launched smoke binds.
- `close_commit_memory_trailer` — apply on close commit.
