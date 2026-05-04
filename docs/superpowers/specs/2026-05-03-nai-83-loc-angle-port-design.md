# NAI-83: Port LOC_ANGLE opcode handler

**Date**: 2026-05-03
**Cadence**: spec + plan + single combined review (per `compressed_cadence.md`,
15–100 production-LOC band; ~22 production LOC)
**Predecessor**: NAI-82 (HEAD `959c9da` — P_ARRIVEDELAY ported; door-click smoke
on Tutorial Island surfaced LOC_ANGLE as next stub-not-completed consumer)
**Successor**: TBD (drained by next NAI-N+1 user-driven door-click smoke)

## 1. Problem

`LOC_ANGLE` (opcode 3001) is declared at `pkg/script/opcode.go:290` and named
at `:971-972` but has no dispatch wiring. NAI-82's close smoke (HEAD `959c9da`,
Tutorial Island door click) surfaced one consumer:

- `[oploc1, newbie_door1]` pc=10 — `no handler for LOC_ANGLE (opcode 3001)`

Pattern: `protocol_stub_not_completed`. Same shape as NAI-81 (LOC_COORD): an
`ActiveLoc` accessor with no producer-side work — the underlying entity
already exposes the field via mask arithmetic.

## 2. TS reference

`LostCityRS/Engine-TS/src/engine/script/handlers/LocOps.ts:45-47`:

```ts
[ScriptOpcode.LOC_ANGLE]: checkedHandler(ActiveLoc, state => {
    state.pushInt(check(state.activeLoc.angle, LocAngleValid));
}),
```

`LocAngleValid` (`ScriptValidators.ts:106`):

```ts
export const LocAngleValid: ScriptValidator<number, LocAngle> =
    new ScriptInputRangeValidator(LocAngle.WEST, LocAngle.SOUTH, 'LocAngle');
```

`LocAngle.WEST = 0, NORTH = 1, EAST = 2, SOUTH = 3` — i.e. range `[0, 3]`.
`ScriptInputRangeValidator` rejects values outside `[min, max]` with
`"LocAngle out of range: <v>"`-style errors.

Pointer manifest (`ScriptOpcodePointers.ts`, by analogy with LOC_COORD):
`require: ['active_loc']`.

Behaviour: pop nothing, push the active loc's `angle` field through the
range validator. `ActiveLoc` is required; absence is a hard error.

## 3. Existing goscape surface

| Concern | Location | Status |
|---|---|---|
| Opcode constant | `pkg/script/opcode.go:290` (`OpLocAngle = 3001`) | declared |
| Opcode → name | `pkg/script/opcode.go:971-972` (`"LOC_ANGLE"`) | declared |
| Dispatch map | `pkg/script/handlers.go:122-126` LOC cluster | **missing** |
| `ActiveLoc` interface | `pkg/script/active.go:698-701` | **needs `Angle()`** |
| Loc-side accessor | `pkg/entity/loc.go:34` (`(l *Loc).Angle()`) | already implements `(l.Info >> 19) & 0x3` |
| Range validator | (new) `checkLocAngle` in `pkg/script/handlers_player.go` | **missing** |
| Active-loc gate | `pkg/script.requireActiveLoc(s, op)` (`handlers_loc.go:12`) | available |
| Test mocks | `fakeActiveLoc` (`handlers_loc_test.go:11`), `mockActiveLoc` (`handlers_player_test.go:15`) | **need `Angle()`** |

## 4. Design

### 4.1 Interface extension

`pkg/script/active.go:698-701` — add an `Angle()` accessor mirroring the
established `Coords()` precedent (NAI-81):

```go
// ActiveLoc is the surface that LOC_* opcodes use to read the loc
// bound to the script's execution. Set by OPLOC trigger routing
// (S6j fireOpTriggerLoc) and LOC_FIND (future).
type ActiveLoc interface {
    LocType() int              // returns the LocType ID (from packed Loc.Info bitfield)
    Coords() (x, z, level int) // world position; consumed by LOC_COORD
    Angle() int                // rotation (0=west, 1=north, 2=east, 3=south); consumed by LOC_ANGLE
}
```

`pkg/entity.Loc.Angle()` already implements this (loc.go:34) — no
producer-side change required.

### 4.2 Range validator

`pkg/script/handlers_player.go` (alongside `checkNotNull` at :71) — add a
sibling helper following the same shape and error-message convention:

```go
// checkLocAngle mirrors TS LocAngleValid (ScriptValidators.ts:106) — a
// ScriptInputRangeValidator over [LocAngle.WEST=0, LocAngle.SOUTH=3].
// Rejects values outside that range.
//
// Note: pkg/entity.Loc.Angle() is mask-bounded to [0,3] by construction
// ((l.Info >> 19) & 0x3 at loc.go:34), so this validator is unreachable
// when fed from the entity layer. Retained for TS-fidelity parity per
// true_to_ts_gate.md — future ActiveLoc producers (e.g. LOC_FIND results
// from external sources) may bypass the bit mask.
func checkLocAngle(v int) error {
    if v < 0 || v > 3 {
        return fmt.Errorf("LocAngle out of range: %d", v)
    }
    return nil
}
```

(Lives in `handlers_player.go` rather than `handlers_loc.go` to match the
existing convention: `checkNotNull` and `checkStringNotNull` both live in
`handlers_player.go` despite being domain-agnostic. No new file needed.)

### 4.3 Handler

`pkg/script/handlers_loc.go` — append after `handleLocCoord` (line 82):

```go
// handleLocAngle pushes the ActiveLoc's rotation onto the int stack,
// validated through the [0,3] LocAngle range. TS:
//
//	pushInt(check(activeLoc.angle, LocAngleValid));
//
// Requires an ActiveLoc; returns "LOC_ANGLE: no active loc" otherwise.
func handleLocAngle(s *ScriptState) error {
    if err := requireActiveLoc(s, "LOC_ANGLE"); err != nil {
        return err
    }
    angle := s.ActiveLoc.Angle()
    if err := checkLocAngle(angle); err != nil {
        return fmt.Errorf("LOC_ANGLE: %w", err)
    }
    s.PushInt(angle)
    return nil
}
```

No new imports — `fmt` is already imported in this file (handlers_loc.go:4).

### 4.4 Dispatch wiring

`pkg/script/handlers.go:122-126` — add the entry under the existing
`// LOC active-loc reads.` sub-comment in lexical order:

```go
// LOC lookup — stub (always "not found"). Real impl ships with S6.
OpLocCoord: handleLocCoord,
OpLocFind:  handleLocFind,
// LOC active-loc reads.
OpLocAngle: handleLocAngle,
OpLocOp:    handleLocOp,
```

Minimal-touch: do not regroup the existing `OpLocCoord` / `OpLocFind`
entries. (Whether `OpLocCoord` belongs under "lookup" or "active-loc reads"
is a pre-existing labeling question from NAI-81; out of scope here.)

### 4.5 Mock updates

**`pkg/script/handlers_loc_test.go:11-17`** — extend `fakeActiveLoc`:

```go
type fakeActiveLoc struct {
    id          int
    x, z, level int
    angle       int
}

func (f fakeActiveLoc) LocType() int              { return f.id }
func (f fakeActiveLoc) Coords() (x, z, level int) { return f.x, f.z, f.level }
func (f fakeActiveLoc) Angle() int                { return f.angle }
```

Existing call sites at `:46`, `:152` construct `fakeActiveLoc{id: locID}`
or `fakeActiveLoc{id: 42, x: 3200, z: 3200, level: 0}`; Go zero-values for
the new `angle` field preserve current behaviour.

**`pkg/script/handlers_player_test.go:15-21`** — extend `mockActiveLoc`:

```go
type mockActiveLoc struct {
    locType     int
    x, z, level int
    angle       int
}

func (m *mockActiveLoc) LocType() int              { return m.locType }
func (m *mockActiveLoc) Coords() (x, z, level int) { return m.x, m.z, m.level }
func (m *mockActiveLoc) Angle() int                { return m.angle }
```

Existing call sites at `:950, :1008, :1089, :2367` construct
`&mockActiveLoc{locType: 42}`; zero-values for the new field preserve
behaviour.

## 5. Tests

Two new test functions in `pkg/script/handlers_loc_test.go` (matching the
NAI-81 LOC_COORD pattern). No table-driven expansion — the validator's
out-of-range branch is unreachable when fed from `entity.Loc.Angle()` and
adding a synthetic `fakeActiveLoc{angle: 99}` test would risk the
"test passes for wrong reason" anti-pattern (`test_passes_for_wrong_reason.md`)
since production cannot reach that branch.

### 5.1 `TestHandleLocAngleHappyPath`

```go
func TestHandleLocAngleHappyPath(t *testing.T) {
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
        ActiveLoc:   fakeActiveLoc{id: 42, angle: 2},
    }

    if err := handleLocAngle(s); err != nil {
        t.Fatalf("handleLocAngle: %v", err)
    }

    if s.ISP != 1 {
        t.Fatalf("ISP: got %d, want 1", s.ISP)
    }
    if got := s.IntStack[0]; got != 2 {
        t.Errorf("top of int stack: got %d, want 2", got)
    }
}
```

### 5.2 `TestHandleLocAngleRequiresActiveLoc`

```go
func TestHandleLocAngleRequiresActiveLoc(t *testing.T) {
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
    }

    err := handleLocAngle(s)
    if err == nil {
        t.Fatal("handleLocAngle: expected error, got nil")
    }
    if got := err.Error(); got != "LOC_ANGLE: no active loc" {
        t.Errorf("error: got %q, want \"LOC_ANGLE: no active loc\"", got)
    }
}
```

### 5.3 Pre-flight verification (confirmed against HEAD `959c9da`)

- `pkg/script/active.go:698-701` — `ActiveLoc` interface has `LocType()` and
  `Coords()`; appending `Angle()` is purely additive.
- `pkg/entity/loc.go:34` — `(l *Loc).Angle()` implements `(l.Info >> 19) & 0x3`;
  signature `func (l *Loc) Angle() int` matches new interface method.
- `pkg/script/handlers.go:122-126` — current LOC dispatch entries are
  `OpLocCoord, OpLocFind, OpLocOp` (with one sub-comment between `OpLocFind`
  and `OpLocOp`); plan task adds `OpLocAngle` and consolidates the cluster.
- `pkg/script/handlers_loc_test.go:10-17` — `fakeActiveLoc` is a value
  receiver; new `Angle()` method must match.
- `pkg/script/handlers_player_test.go:15-21` — `mockActiveLoc` is a pointer
  receiver; new `Angle()` method must match.
- `pkg/script/handlers_player.go:71` — `checkNotNull` shape (single int arg,
  op-name parameter dropped); `checkLocAngle` follows the same single-int
  signature but **drops the `op` parameter** since the error message names
  `LocAngle` directly (matching TS, where the validator's error names the
  validator class). Caller wraps with `fmt.Errorf("LOC_ANGLE: %w", err)`.
- `s.ISP` is the int-stack pointer field used in NAI-81's tests at
  `handlers_loc_test.go:171` — same idiom.

## 6. TS-fidelity ledger

| TS construct | goscape mapping | Divergence? |
|---|---|---|
| `checkedHandler(ActiveLoc, ...)` | `requireActiveLoc(s, "LOC_ANGLE")` | No — established sibling pattern |
| `state.activeLoc.angle` | `s.ActiveLoc.Angle()` | No — accessor wrap of `(l.Info >> 19) & 0x3` |
| `LocAngleValid` `[0,3]` range | `checkLocAngle` `[0,3]` range | No — equivalent semantics |
| Validator error literal | `"LocAngle out of range: <v>"` | Cosmetic — TS message format is engine-internal, not script-observable |
| Pointer manifest `require: ['active_loc']` | `requireActiveLoc` early-return | No |
| Push int via `pushInt(check(...))` | Push int **after** validator returns nil | No — same observable behaviour: push happens iff validator passes |

**No divergences to track.** No follow-ups deferred.

## 7. Out of scope

- **`LOC_FIND`** — remains stubbed (`handlers_loc.go:30-35`); needs world-wide
  loc iteration. Separate sub-spec.
- **`LOC_SHAPE`, `LOC_TYPE`, `LOC_PARAM`, `LOC_NAME`, `LOC_CATEGORY`** — also
  declared but unwired; `LOC_SHAPE` would be the next analogous accessor port
  if a smoke surfaces it. Not addressed here.
- **`LOC_ADD`, `LOC_DEL`, `LOC_CHANGE`, `LOC_ANIM`** — mutating opcodes
  requiring world-state plumbing; out of scope for the accessor-port line.
- **`LocAngle` named constant set in goscape** — TS exposes `LocAngle.WEST`
  etc. through `@2004scape/rsmod-pathfinder`; goscape uses raw ints `[0,3]`.
  No script-side consumer needs the named constants today; if one surfaces
  later, define a `LocAngle` constant block then.

## 8. Smoke / cascade routing

Stub-not-completed port. Cadence-blocker is `go test ./...` green +
`go vet ./...` clean. Cascade attribution ("LOC_ANGLE silenced") closes at
the next NAI-N+1 user-driven Tutorial Island door-click smoke if the same
content path runs without a `no handler for LOC_ANGLE` error. Subsequent
no-handler errors at the same `[oploc1, newbie_door1]` click would seed the
next sub-spec target per `protocol_stub_not_completed.md`.

## 9. Tech stack

Go 1.26+ (per `go_version.md`).

## 10. Plan handoff

Implementation plan ships as a separate doc
(`docs/superpowers/plans/2026-05-03-nai-83-loc-angle-port.md`) per the
spec+plan cadence. Plan structure:

- **T1**: Red — add interface method + validator + test fixtures (compile
  fail until handler exists; mock structs need `Angle()` to satisfy the
  extended interface).
- **T2**: Green — `handleLocAngle` body + dispatch wiring + mock `Angle()`
  methods.
- **T3**: Verify — `go test ./pkg/script/...`, `go test ./...`, `go vet ./...`.
- **Single combined review** at end (per `compressed_cadence.md` 15–100 LOC
  band) — no per-task two-stage review.
- **Close commit** with `Closes memory: nai83_seed_loc_angle.md` trailer
  per `close_commit_memory_trailer.md`.
