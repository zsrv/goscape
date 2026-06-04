# NAI-81: Port LOC_COORD opcode handler

**Date**: 2026-05-03
**Cadence**: compressed (combined spec + plan; per `compressed_cadence.md`, ≤~15 production LOC)
**Predecessor**: NAI-80 (smoke at HEAD `7692715` advanced cascade to `script_dispatch`; LOC_COORD identified as next stub-not-completed)
**Successor**: NAI-82 (P_ARRIVEDELAY, opcode 2068)

## 1. Problem

`LOC_COORD` (opcode 3005) is declared at `pkg/script/opcode.go:294` but has no
dispatch wiring. NAI-80's user-driven smoke surfaced 3 distinct script consumers
hitting `no handler for LOC_COORD` in one session:

- `[oploc1, _drawer]` pc=4
- `[oploc1, _chest_closed]` pc=4
- `[oploc1, newbie_door1]` pc=9

Pattern: `protocol_stub_not_completed`.

## 2. TS reference

`LostCityRS/Engine-TS/src/engine/script/handlers/LocOps.ts:69-72`:

```ts
[ScriptOpcode.LOC_COORD]: checkedHandler(ActiveLoc, state => {
    const coord: CoordGrid = state.activeLoc;
    state.pushInt(CoordGrid.packCoord(coord.level, coord.x, coord.z));
}),
```

Pointer manifest (`ScriptOpcodePointers.ts:740-743`):
```ts
[ScriptOpcode.LOC_COORD]: {
    require: ['active_loc'],
    require2: ['active_loc2']
}
```

Behaviour: pop nothing, push the active loc's packed (level, x, z) coord using
the standard `(z & 0x3fff) | ((x & 0x3fff) << 14) | ((level & 0x3) << 28)`
encoding. Requires `ActiveLoc` pointer; absence is a hard error.

## 3. Existing goscape surface

| Concern | Location | Status |
|---|---|---|
| Opcode constant | `pkg/script/opcode.go:294` (`OpLocCoord = 3005`) | declared |
| Opcode → name | `pkg/script/opcode.go:979-980` | declared |
| Dispatch map | `pkg/script/handlers.go:123-125` (sibling LOC entries) | **missing** |
| `ActiveLoc` interface | `pkg/script/active.go:686-688` (currently `LocType() int` only) | **needs `Coords()`** |
| Coord packer | `pkg/coordgrid.PackCoord(level, x, z)` (coordgrid.go:158) | available |
| Loc producer | `pkg/entity.Loc.Coords() (x, z, level int)` (entity/loc.go:49) | already implements |
| Active-loc gate | `pkg/script.requireActiveLoc(s, op string)` (handlers_loc.go:8) | available |
| Test mocks | `fakeActiveLoc` (handlers_loc_test.go:10), `mockActiveLoc` (handlers_player_test.go:15) | **need `Coords()`** |

## 4. Design

### 4.1 Interface extension

`pkg/script/active.go:686-688` — add a `Coords()` accessor mirroring the
established `ActiveObj` precedent at `active.go:693-696`:

```go
// ActiveLoc is the surface that LOC_* opcodes use to read the loc
// bound to the script's execution. Set by OPLOC trigger routing
// (S6j fireOpTriggerLoc) and LOC_FIND (future).
type ActiveLoc interface {
    LocType() int             // returns the LocType ID (from packed Loc.Info bitfield)
    Coords() (x, z, level int) // world position; consumed by LOC_COORD
}
```

`pkg/entity.Loc` already implements `Coords()` (entity/loc.go:49) — no producer
changes required.

### 4.2 Handler

`pkg/script/handlers_loc.go` — append:

```go
// handleLocCoord pushes the ActiveLoc's packed (level, x, z) coord onto
// the int stack. TS:
//
//	pushInt(CoordGrid.packCoord(activeLoc.level, activeLoc.x, activeLoc.z));
//
// Requires an ActiveLoc; returns "LOC_COORD: no active loc" otherwise.
func handleLocCoord(s *ScriptState) error {
    if err := requireActiveLoc(s, "LOC_COORD"); err != nil {
        return err
    }
    x, z, level := s.ActiveLoc.Coords()
    s.PushInt(coordgrid.PackCoord(level, x, z))
    return nil
}
```

Add `"github.com/zsrv/goscape/pkg/coordgrid"` to the file's import block.

### 4.3 Dispatch wiring

`pkg/script/handlers.go` — at the existing LOC dispatch cluster (lines 123-125):

```go
OpLocCoord: handleLocCoord,
OpLocFind:  handleLocFind,
OpLocOp:    handleLocOp,
```

(Lexical order preserved among LOC entries.)

### 4.4 Mock updates

**`pkg/script/handlers_loc_test.go:10-12`** — extend `fakeActiveLoc`:
```go
type fakeActiveLoc struct {
    id            int
    x, z, level   int
}

func (f fakeActiveLoc) LocType() int                 { return f.id }
func (f fakeActiveLoc) Coords() (x, z, level int)    { return f.x, f.z, f.level }
```
Existing call sites construct `fakeActiveLoc{id: locID}` — Go zero-values for
the new fields preserve current behaviour.

**`pkg/script/handlers_player_test.go:15-19`** — extend `mockActiveLoc`:
```go
type mockActiveLoc struct {
    locType     int
    x, z, level int
}

func (m *mockActiveLoc) LocType() int              { return m.locType }
func (m *mockActiveLoc) Coords() (x, z, level int) { return m.x, m.z, m.level }
```
Existing call sites at :948, :1006, :1087 construct `&mockActiveLoc{locType: 42}`
— zero-values for the new fields preserve behaviour.

## 5. Tests

Two new test functions in `pkg/script/handlers_loc_test.go`. No table-driven
expansion — only two distinct paths.

### 5.1 `TestHandleLocCoordHappyPath`
```go
func TestHandleLocCoordHappyPath(t *testing.T) {
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
        ActiveLoc:   fakeActiveLoc{id: 42, x: 3200, z: 3200, level: 0},
    }

    if err := handleLocCoord(s); err != nil {
        t.Fatalf("handleLocCoord: %v", err)
    }

    if s.ISP != 1 {
        t.Fatalf("ISP: got %d, want 1", s.ISP)
    }
    want := coordgrid.PackCoord(0, 3200, 3200)
    if got := s.IntStack[0]; got != want {
        t.Errorf("top of int stack: got %d, want %d", got, want)
    }
}
```

### 5.2 `TestHandleLocCoordRequiresActiveLoc`
```go
func TestHandleLocCoordRequiresActiveLoc(t *testing.T) {
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
    }

    err := handleLocCoord(s)
    if err == nil {
        t.Fatal("handleLocCoord: expected error, got nil")
    }
    if got := err.Error(); got != "LOC_COORD: no active loc" {
        t.Errorf("error: got %q, want \"LOC_COORD: no active loc\"", got)
    }
}
```

Add `"github.com/zsrv/goscape/pkg/coordgrid"` to the test file's import block.

### 5.3 Pre-flight verification (confirmed against HEAD `7692715`)
- `pkg/coordgrid.PackCoord(level, x, z)` exported (coordgrid.go:158).
- `pkg/script.ScriptState.ISP` is the int-stack pointer field
  (state.go:175-177); idiom `s.ISP != 1 || s.IntStack[0] != want` is used
  throughout `handlers_db_test.go` (e.g. line 164).
- `fakeActiveLoc` is a value receiver (handlers_loc_test.go:12);
  `mockActiveLoc` is a pointer receiver (handlers_player_test.go:19).
  The new `Coords()` method must match each struct's existing receiver kind.

## 6. TS-fidelity ledger

| TS construct | goscape mapping | Divergence? |
|---|---|---|
| `checkedHandler(ActiveLoc, ...)` | `requireActiveLoc(s, "LOC_COORD")` | No — established sibling pattern |
| `coord.level/x/z` (CoordGrid base) | `s.ActiveLoc.Coords()` returns `(x, z, level)` | No — argument order resolved by named binding at the `PackCoord` call site |
| `CoordGrid.packCoord(level, x, z)` | `coordgrid.PackCoord(level, x, z)` | No — identical arithmetic |
| Pointer manifest `require: ['active_loc']` | `requireActiveLoc` early-return | No |

No deviations to track. No follow-ups deferred.

## 7. Out of scope

- **`LOC_FIND`** — remains stubbed (handlers_loc.go:26); needs world-wide loc
  iteration. Separate sub-spec.
- **`P_ARRIVEDELAY`** (opcode 2068, single consumer `[oploc1, _bookcase]`) —
  next ticket (NAI-82).
- **Other LOC_* opcodes** declared in `opcode.go` but unwired (LOC_ADD,
  LOC_ANGLE, LOC_ANIM, LOC_CATEGORY, LOC_CHANGE, LOC_DEL, LOC_FINDALLZONE,
  LOC_FINDNEXT, LOC_NAME, LOC_PARAM, LOC_SHAPE, LOC_TYPE) — surface only when
  a smoke or test exercises them; not addressed here.

## 8. Plan (compressed, single-task)

**Task 1** — single TDD red-then-green cycle with one commit:

1. **Red**: extend `ActiveLoc` interface + add the two failing tests in
   `handlers_loc_test.go`. Compile fails on `Coords()` not implemented by
   `fakeActiveLoc`/`mockActiveLoc`; once mocks updated, tests fail because
   `handleLocCoord` doesn't exist.
2. **Green**: add `handleLocCoord` body + register in `handlers.go` dispatch
   map. Update both mock structs (`fakeActiveLoc`, `mockActiveLoc`) with
   `Coords()` method.
3. **Verify**:
   - `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`
     green.
   - `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` green
     (regression scan; no other consumers of the `ActiveLoc` interface should
     break since the addition is additive and `entity.Loc` already implements
     `Coords()`).
   - `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean.
4. **Commit** with `--no-gpg-sign` and the trailer:
   `Closes memory: nai81_seed_loc_coord_p_arrivedelay.md`
   (per `close_commit_memory_trailer.md`).

## 9. Tech stack
Go 1.26+ (per `go_version.md`).

## 10. Smoke / cascade routing

This is a stub-not-completed port. No user-driven smoke required to close —
the cadence-blocker is `go test ./...` green. The cascade attribution
("LOC_COORD silenced") will be confirmed at the next NAI-N+1 smoke run if
the user re-exercises the same Tutorial Island content path.
