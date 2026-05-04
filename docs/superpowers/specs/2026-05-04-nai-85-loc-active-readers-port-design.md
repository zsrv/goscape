# NAI-85: Port LOC_PARAM, LOC_NAME, LOC_TYPE, LOC_SHAPE handlers

**Date**: 2026-05-04
**Cadence**: combined spec + plan, single end-of-impl review (per
`compressed_cadence.md`, ≤100 production-LOC band; ~80 production LOC).
**Predecessor**: NAI-84 (HEAD `469d07b` — REBUILD_GETMAPS byte-source fix +
rebuildZones port; door-click cache freeze silenced).
**Smoke binding (post-NAI-83)**: door-click smoke at HEAD `469d07b` confirmed
LOC_ANGLE silenced; new cascade-blocker surfaced at the same site as a
`no handler for LOC_PARAM (opcode 3011)` script error.
**Successor**: TBD (drained by next NAI-N+1 user-driven door-click smoke).

## 1. Problem

Tutorial Island door-click smoke at HEAD `469d07b` reveals the
`[oploc1, newbie_door1]` script aborts at pc=14:

```
WARN script="[oploc1,newbie_door1]" err="no handler for LOC_PARAM (opcode 3011) at pc=14"
```

The player walks to the adjacent tile (`interacted=true` at tick 130/132/etc.)
but the script's player-side path never completes — door stays shut.

Pattern: `protocol_stub_not_completed` (per `nai_followups.md` and the
NAI-83 §8 routing rule). Family of 4 active-loc reader opcodes declared
in `pkg/script/opcode.go` but with no dispatch wiring:

| Opcode | Constant decl | Name decl | Handler |
|---|---|---|---|
| LOC_NAME (3010) | `pkg/script/opcode.go:299` | `pkg/script/opcode.go:989-990` | **missing** |
| LOC_PARAM (3011) | `pkg/script/opcode.go:300` | `pkg/script/opcode.go:991-992` | **missing** |
| LOC_SHAPE (3012) | `pkg/script/opcode.go:301` | `pkg/script/opcode.go:993-994` | **missing** |
| LOC_TYPE (3013) | `pkg/script/opcode.go:302` | `pkg/script/opcode.go:995-996` | **missing** |

LOC_PARAM is the cascade-blocker; the other three share the same
`checkedHandler(ActiveLoc, …)` shape and live in the same TS file. Per
`dead_api_polish.md` and Q1 brainstorm (option B confirmed; option C
retracted — `LC_TYPE`/`LC_SHAPE` do not exist in TS), the bundle drains
all four together rather than ship LOC_PARAM solo and re-open the file
on each subsequent surfacing.

## 2. TS reference

`LostCityRS/Engine-TS/src/engine/script/handlers/LocOps.ts:114-135`:

```ts
[ScriptOpcode.LOC_PARAM]: checkedHandler(ActiveLoc, state => {
    const paramType: ParamType = check(state.popInt(), ParamTypeValid);

    const locType: LocType = check(state.activeLoc.type, LocTypeValid);
    if (paramType.isString()) {
        state.pushString(ParamHelper.getStringParam(paramType.id, locType, paramType.defaultString));
    } else {
        state.pushInt(ParamHelper.getIntParam(paramType.id, locType, paramType.defaultInt));
    }
}),

[ScriptOpcode.LOC_TYPE]: checkedHandler(ActiveLoc, state => {
    state.pushInt(check(state.activeLoc.type, LocTypeValid).id);
}),

[ScriptOpcode.LOC_NAME]: checkedHandler(ActiveLoc, state => {
    state.pushString(check(state.activeLoc.type, LocTypeValid).name ?? 'null');
}),

[ScriptOpcode.LOC_SHAPE]: checkedHandler(ActiveLoc, state => {
    state.pushInt(check(state.activeLoc.shape, LocShapeValid));
}),
```

Validators (`ScriptValidators.ts`):

- `LocTypeValid` — `ScriptInputValidator<LocType>` rejecting nil/missing
  resolved LocType objects (TS `state.activeLoc.type` returns the LocType
  object directly).
- `LocShapeValid` — `ScriptInputRangeValidator(LocShape.WALL_STRAIGHT=0,
  LocShape.GROUND_DECOR=22, 'LocShape')`. Rejects values outside `[0, 22]`.
- `ParamTypeValid` — non-null check on resolved ParamType. Already handled
  inside goscape's `paramLookup` (handlers_config.go:22-24).

Pointer manifest (`ScriptOpcodePointers.ts`, by analogy with LOC_ANGLE /
LOC_COORD): `require: ['active_loc']`.

## 3. Existing goscape surface

| Concern | Location | Status |
|---|---|---|
| `ActiveLoc` interface | `pkg/script/active.go:698-702` | has `LocType`, `Coords`, `Angle`; **needs `Shape()`** |
| `Loc.Shape()` accessor | `pkg/entity/loc.go:31` (`(l.Info >> 14) & 0x1F`) | exists — no producer change |
| `LocType.Name` field | `pkg/objtype/loctype.go:25` (decoded at `:72` via code 2) | populated server-side |
| `LocType.Params` field | `pkg/objtype/loctype.go:58` (`ParamMap`) | populated via `DecodeParams` (code 249) |
| `paramLookup` helper | `pkg/script/handlers_config.go:17-49` | accepts `ParamMap`; handles ParamType nil + push-by-type |
| `requireActiveLoc` gate | `pkg/script/handlers_loc.go:12-17` | available — established |
| `requireConfigs` gate | `pkg/script/handlers_npc.go` (sibling) | available — used by NPC_PARAM |
| Hand-roll validator precedent | `pkg/script/handlers_player.go:71` (`checkNotNull`), `checkLocAngle` from NAI-83 (lives in `handlers_player.go` per NAI-83 §4.2) | established |
| `fakeActiveLoc` mock | `pkg/script/handlers_loc_test.go:11-19` | **needs `shape` field + `Shape() int`** |
| `mockActiveLoc` mock | `pkg/script/handlers_player_test.go:15-23` | **needs `shape` field + `Shape() int`** |
| Sibling pattern (active-entity param) | `handleNpcParam` (`handlers_config.go:297-311`) | direct mirror for LOC_PARAM |
| Sibling pattern (locID-arg LocOps) | `handleLcParam` (`handlers_config.go:163-175`), `handleLcName` (`handlers_config.go:143-161`) | informs goscape conventions |

## 4. Design

### 4.1 Interface extension

`pkg/script/active.go:698-702` — add `Shape()` accessor:

```go
type ActiveLoc interface {
    LocType() int              // returns the LocType ID (from packed Loc.Info bitfield)
    Coords() (x, z, level int) // world position; consumed by LOC_COORD
    Angle() int                // rotation (0=west, 1=north, 2=east, 3=south); consumed by LOC_ANGLE
    Shape() int                // shape (0..22 valid range); consumed by LOC_SHAPE
}
```

`pkg/entity.Loc.Shape()` already implements this (loc.go:31) — no
producer-side change required.

### 4.2 Range validator

`pkg/script/handlers_player.go` (alongside `checkLocAngle` from NAI-83
§4.2 and `checkNotNull` at :71) — add a sibling helper:

```go
// checkLocShape mirrors TS LocShapeValid (ScriptValidators.ts) — a
// ScriptInputRangeValidator over [LocShape.WALL_STRAIGHT=0,
// LocShape.GROUND_DECOR=22]. Rejects values outside that range.
//
// Note: pkg/entity.Loc.Shape() returns (l.Info >> 14) & 0x1F which
// covers [0,31]; production may surface values in (22,31] for unrecognized
// shapes. Caller wraps the error with "LOC_SHAPE: %w" so the script
// abort message names the opcode.
func checkLocShape(v int) error {
    if v < 0 || v > 22 {
        return fmt.Errorf("LocShape out of range: %d", v)
    }
    return nil
}
```

(Lives in `handlers_player.go` per the established convention from
NAI-83 §4.2.)

### 4.3 Handlers

`pkg/script/handlers_loc.go` — append after `handleLocAngle` (line 100).
All four handlers follow the established `requireActiveLoc` early-return
pattern. No new imports: `fmt` is already imported (handlers_loc.go:4),
and field access through `*objtype.LocType` returned by
`s.Configs.LocType(...)` does not require an `objtype` import (Go field
access via a returned pointer only requires the package to be transitively
importable, not directly imported). Confirmed via the existing
`handleLocOp:55-65` precedent which does `cfg.Op[idx]` without an
`objtype` import.

```go
// handleLocType pushes the ActiveLoc's resolved LocType ID. Mirrors TS
// LOC_TYPE: pushInt(check(activeLoc.type, LocTypeValid).id).
//
// Goscape ActiveLoc.LocType() returns the int ID directly; the TS
// LocTypeValid non-null check translates to a Configs lookup nil-check
// (matches handleNpcParam pattern at handlers_config.go:307-308).
func handleLocType(s *ScriptState) error {
    if err := requireConfigs(s, "LOC_TYPE"); err != nil {
        return err
    }
    if err := requireActiveLoc(s, "LOC_TYPE"); err != nil {
        return err
    }
    id := s.ActiveLoc.LocType()
    lt := s.Configs.LocType(id)
    if lt == nil {
        return fmt.Errorf("LOC_TYPE: unknown loc id %d", id)
    }
    s.PushInt(id)
    return nil
}

// handleLocName pushes the ActiveLoc's LocType name, with "null" fallback
// when the name is empty. Mirrors TS LOC_NAME: pushString(check(activeLoc.type,
// LocTypeValid).name ?? 'null').
//
// Note: TS active-loc LOC_NAME does NOT fall back to debugname (only the
// locID-arg LC_NAME does). LC_NAME itself currently uses DebugName with a
// stale comment claiming Name is unset server-side; that's a tracked
// follow-up (NAI-N+1 — fix LC_NAME to use Name → DebugName → "null"
// per TS LocConfigOps.ts:12). LOC_NAME ships TS-correct from the start.
func handleLocName(s *ScriptState) error {
    if err := requireConfigs(s, "LOC_NAME"); err != nil {
        return err
    }
    if err := requireActiveLoc(s, "LOC_NAME"); err != nil {
        return err
    }
    id := s.ActiveLoc.LocType()
    lt := s.Configs.LocType(id)
    if lt == nil {
        return fmt.Errorf("LOC_NAME: unknown loc id %d", id)
    }
    if lt.Name != "" {
        s.PushString(lt.Name)
    } else {
        s.PushString("null")
    }
    return nil
}

// handleLocShape pushes the ActiveLoc's shape, validated through the
// [0,22] LocShape range. TS:
//
//  pushInt(check(activeLoc.shape, LocShapeValid));
//
// Requires an ActiveLoc; returns "LOC_SHAPE: no active loc" otherwise.
// Range-validates because Loc.Shape()'s mask is [0,31] — wider than
// the LocShape valid range.
func handleLocShape(s *ScriptState) error {
    if err := requireActiveLoc(s, "LOC_SHAPE"); err != nil {
        return err
    }
    shape := s.ActiveLoc.Shape()
    if err := checkLocShape(shape); err != nil {
        return fmt.Errorf("LOC_SHAPE: %w", err)
    }
    s.PushInt(shape)
    return nil
}

// handleLocParam pops paramID, resolves the ActiveLoc's LocType, and
// delegates to paramLookup. Mirrors TS LOC_PARAM (LocOps.ts:114-123) —
// the active-loc-bound counterpart of LC_PARAM (handlers_config.go:163).
func handleLocParam(s *ScriptState) error {
    if err := requireConfigs(s, "LOC_PARAM"); err != nil {
        return err
    }
    if err := requireActiveLoc(s, "LOC_PARAM"); err != nil {
        return err
    }
    paramID := s.PopInt()
    id := s.ActiveLoc.LocType()
    lt := s.Configs.LocType(id)
    if lt == nil {
        return fmt.Errorf("LOC_PARAM: unknown loc id %d", id)
    }
    return paramLookup(s, lt.Params, paramID)
}
```

### 4.4 Dispatch wiring

`pkg/script/handlers.go:122-127` — add four entries under the existing
`// LOC active-loc reads.` sub-comment in lexical order:

```go
// LOC lookup — stub (always "not found"). Real impl ships with S6.
OpLocCoord: handleLocCoord,
OpLocFind:  handleLocFind,
// LOC active-loc reads.
OpLocAngle: handleLocAngle,
OpLocName:  handleLocName,
OpLocOp:    handleLocOp,
OpLocParam: handleLocParam,
OpLocShape: handleLocShape,
OpLocType:  handleLocType,
```

Minimal-touch: do not regroup the existing `OpLocCoord` / `OpLocFind`
entries (pre-existing labeling question from NAI-81/NAI-83; out of scope).

### 4.5 Mock updates

**`pkg/script/handlers_loc_test.go:11-19`** — extend `fakeActiveLoc`:

```go
type fakeActiveLoc struct {
    id          int
    x, z, level int
    angle       int
    shape       int
}

func (f fakeActiveLoc) LocType() int              { return f.id }
func (f fakeActiveLoc) Coords() (x, z, level int) { return f.x, f.z, f.level }
func (f fakeActiveLoc) Angle() int                { return f.angle }
func (f fakeActiveLoc) Shape() int                { return f.shape }
```

Existing call sites at `:48`, `:150`, `:167`, `:202` construct
`fakeActiveLoc{id: X, …}`; Go zero-values for the new `shape` field
preserve current behaviour (zero is in-range).

**`pkg/script/handlers_player_test.go:15-23`** — extend `mockActiveLoc`:

```go
type mockActiveLoc struct {
    locType     int
    x, z, level int
    angle       int
    shape       int
}

func (m *mockActiveLoc) LocType() int              { return m.locType }
func (m *mockActiveLoc) Coords() (x, z, level int) { return m.x, m.z, m.level }
func (m *mockActiveLoc) Angle() int                { return m.angle }
func (m *mockActiveLoc) Shape() int                { return m.shape }
```

Existing call sites at `:952, :1010, :1091, :2369` construct
`&mockActiveLoc{locType: 42}`; zero-values for the new field preserve
behaviour.

## 5. Tests

New test functions in `pkg/script/handlers_loc_test.go`. Pattern mirrors
the existing LOC_OP / LOC_COORD / LOC_ANGLE clusters. Adds `params` and
`locs` fields to `fakeConfigs` (currently `ParamType` returns nil; tests
need a populated `params` map for LOC_PARAM string/int dispatch).

Tests added (11 total):

1. **`TestHandleLocTypeHappyPath`** — ActiveLoc + Configs with LocType{ID:42}; expect ISP=1, IntStack[0]==42.
2. **`TestHandleLocTypeRequiresActiveLoc`** — no ActiveLoc; expect `"LOC_TYPE: no active loc"`.
3. **`TestHandleLocTypeUnknownID`** — ActiveLoc with id=999, fakeConfigs empty; expect `"LOC_TYPE: unknown loc id 999"`.
4. **`TestHandleLocNameHappyPath`** — LocType{Name:"door"}; expect SSP=1, StringStack[0]=="door".
5. **`TestHandleLocNameNullFallback`** — LocType{Name:""}; expect StringStack[0]=="null".
6. **`TestHandleLocNameRequiresActiveLoc`** — expect `"LOC_NAME: no active loc"`.
7. **`TestHandleLocShapeHappyPath`** — ActiveLoc{shape:10}; expect IntStack[0]==10.
8. **`TestHandleLocShapeRequiresActiveLoc`** — expect `"LOC_SHAPE: no active loc"`.
9. **`TestHandleLocParamHappyPathInt`** — ActiveLoc{id:42}; LocType{Params: {1: uint32(7)}}; ParamType{Id:1, Type:Int}; pop paramID=1; expect IntStack[0]==7.
10. **`TestHandleLocParamHappyPathString`** — same setup with ParamType{Type:String}; expect StringStack[0]=="hello".
11. **`TestHandleLocParamRequiresActiveLoc`** — expect `"LOC_PARAM: no active loc"`.

(Out-of-range LOC_SHAPE test omitted per `test_passes_for_wrong_reason.md`
guidance — Loc.Shape() may produce values >22 in production but the
production reach for that branch is unverified; adding a synthetic
`fakeActiveLoc{shape: 99}` test would risk asserting a non-production
code path. The validator exists for TS-fidelity and bit-mask-bypass
producers (e.g. future LOC_FIND with external sources); the validator
itself is well-defined unit logic.)

### 5.1 Pre-flight verification (against HEAD `469d07b`)

- `pkg/script/active.go:698-702` — `ActiveLoc` has `LocType`, `Coords`,
  `Angle`; appending `Shape()` is purely additive.
- `pkg/entity/loc.go:31` — `(l *Loc).Shape() int` returns
  `(l.Info >> 14) & 0x1F`; signature matches new interface method.
- `pkg/script/handlers.go:122-127` — current LOC dispatch entries are
  `OpLocCoord, OpLocFind, OpLocAngle, OpLocOp` (with one sub-comment
  between `OpLocFind` and `OpLocAngle`); plan task adds `OpLocName`,
  `OpLocParam`, `OpLocShape`, `OpLocType` to the active-loc-reads
  cluster, sorted lexically.
- `pkg/script/handlers_loc.go:1-100` — `requireActiveLoc` gate at line
  12 is the established pattern for new active-loc handlers.
- `pkg/script/handlers_loc_test.go:10-19` — `fakeActiveLoc` is value
  receiver; new `Shape()` method must match.
- `pkg/script/handlers_player_test.go:15-23` — `mockActiveLoc` is
  pointer receiver; new `Shape()` method must match.
- `pkg/script/handlers_config.go:17-49` — `paramLookup(s, params,
  paramID)` accepts `objtype.ParamMap`; nil-checks ParamType internally.
- `pkg/objtype/loctype.go:25` (Name), `:58` (Params) — both fields
  present and decoded.
- `pkg/script/handlers_config.go:297-311` (`handleNpcParam`) — direct
  template for LOC_PARAM (active-entity + paramLookup).
- `pkg/script/handlers_loc.go` does NOT currently import `objtype` —
  field accesses through `*objtype.LocType` returned by the Configs
  interface don't require importing the type's package (sibling
  `handleLocOp:55-65` uses `cfg.Op[idx]` without an `objtype` import).

## 6. TS-fidelity ledger

| TS construct | goscape mapping | Divergence? |
|---|---|---|
| `checkedHandler(ActiveLoc, ...)` | `requireActiveLoc(s, op)` | No — established sibling pattern |
| `state.activeLoc.type` returns LocType obj | `s.Configs.LocType(s.ActiveLoc.LocType())` two-step | No — `Configs.LocType(id) == nil` check is the `LocTypeValid` equivalent |
| `state.activeLoc.shape` int | `s.ActiveLoc.Shape()` | No — accessor wrap of `(l.Info >> 14) & 0x1F` |
| `LocShapeValid` `[0,22]` range | `checkLocShape` `[0,22]` range | No — equivalent semantics |
| `ParamHelper.getIntParam`/`getStringParam` | `paramLookup` | No — semantically equivalent (NPC_PARAM precedent) |
| `name ?? 'null'` (LOC_NAME) | `if lt.Name != "" pushString(lt.Name) else pushString("null")` | No — equivalent (Go zero-value vs JS nullish) |
| Pointer manifest `require: ['active_loc']` | `requireActiveLoc` early-return | No |

**No new tracked divergences from NAI-85's own production.** One
**inherited** divergence flagged for follow-up:

- **LC_NAME field choice (NAI-N+1 follow-up)**: `handleLcName`
  (handlers_config.go:143-161) uses `lt.DebugName` instead of TS's
  `name ?? debugname ?? 'null'` chain. Stale comment at line 154 claims
  Name is unset server-side; loctype.go:72 disproves this. NAI-85 ships
  LOC_NAME TS-correct (Name → "null") to avoid propagating the
  divergence; the LC_NAME fix is tracked as a separate sub-spec target.

## 7. Out of scope

- **`LOC_FIND`** — remains stubbed (`handlers_loc.go:30-35`); needs
  world-wide loc iteration. Separate sub-spec.
- **`LOC_CATEGORY`** — declared but unwired; defer until smoke surfaces
  it (no current dependency).
- **`LOC_ADD`, `LOC_DEL`, `LOC_CHANGE`, `LOC_ANIM`** — mutating opcodes
  requiring world-state plumbing; out of scope for the accessor-port line.
- **`LC_OP`** — declared in TS (`ScriptOpcode.ts:296`) and goscape
  (`opcode.go:341`), `protocol_stub_not_completed` on **both sides** (TS
  has no implementation in `LocConfigOps.ts`). Per `true_to_ts_gate.md`,
  do not invent semantics for an unimplemented TS opcode.
- **`LC_TYPE` / `LC_SHAPE`** — do not exist in TS or goscape (verified
  against `LocConfigOps.ts` and `opcode.go:336-343`). Bundle C from the
  brainstorm was retracted on this finding.
- **LC_NAME field-choice fix** — tracked as NAI-N+1 follow-up; not
  blocking the door-click cascade.
- **Out-of-range LOC_SHAPE production test** — unverified production
  reach for `shape > 22`; per `test_passes_for_wrong_reason.md` we
  decline to add a synthetic test that doesn't reflect a known
  production path.

## 8. Smoke / cascade routing

Stub-not-completed port. Cadence-blockers:

1. `go test ./...` green
2. `go vet ./...` clean

Cascade attribution ("LOC_PARAM silenced") closes at the next NAI-N+1
user-driven Tutorial Island door-click smoke if the same content path
runs without a `no handler for LOC_PARAM` error. Subsequent no-handler
errors at the same `[oploc1, newbie_door1]` script (or any `LOC_PARAM`-
adjacent content) seed the next sub-spec target per
`protocol_stub_not_completed.md`.

If the door **opens** on smoke (cascade-bound at the door), NAI-85
closes and the broader Tutorial-Island tutorial-flow cascade either
unblocks or reveals the next adjacent divergence.

## 9. Tech stack

Go 1.26+ (per `go_version.md`).

## 10. Plan handoff

Implementation plan ships as a separate doc
(`docs/superpowers/plans/2026-05-04-nai-85-loc-active-readers-port.md`)
per the spec+plan cadence. Plan structure:

- **T1**: Red — extend `ActiveLoc` interface with `Shape()`, extend
  `fakeActiveLoc` + `mockActiveLoc` mocks, extend `fakeConfigs` with
  `params` + populated `locs` map, add 11 test functions (compile fail
  until handlers exist).
- **T2**: Green — `checkLocShape` validator + four handlers
  (`handleLocType`/`handleLocName`/`handleLocShape`/`handleLocParam`)
  + four dispatch entries.
- **T3**: Verify — `go test ./pkg/script/...`, `go test ./...`,
  `go vet ./...`.
- **Single combined review** at end (per `compressed_cadence.md` 15–100
  LOC band) — no per-task two-stage review.
- **Close commit** with `Closes memory: nai85_loc_active_readers.md`
  trailer per `close_commit_memory_trailer.md`. Memory entry seeds the
  LC_NAME follow-up.
