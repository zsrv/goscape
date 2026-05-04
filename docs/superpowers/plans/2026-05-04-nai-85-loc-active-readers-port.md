# NAI-85: Port LOC_PARAM/NAME/TYPE/SHAPE Active-Loc Readers — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire four active-loc reader opcodes — LOC_PARAM (3011), LOC_NAME (3010), LOC_TYPE (3013), LOC_SHAPE (3012) — end-to-end: extend the `ActiveLoc` interface with `Shape()`, add a `checkLocShape` range validator, implement four `handleLoc*` handlers, register them in the dispatch map, and update the two test mocks plus `fakeConfigs`. Cascade-blocker: `[oploc1, newbie_door1]` no-handler-for-LOC_PARAM error from NAI-83 close smoke at HEAD `469d07b`.

**Architecture:** Stub-not-completed accessor port (4 opcodes from same TS file `LocOps.ts:114-135`). The opcode constants and name table already exist; this plan adds four handlers + dispatch entries + one interface accessor (`Shape()`) + one range validator following the NAI-83 LOC_ANGLE pattern. `pkg/entity.Loc.Shape()` already exists as the producer-side accessor (`(l.Info >> 14) & 0x1F` at `pkg/entity/loc.go:31`); no producer-side change. `LocType.Name` and `LocType.Params` are populated by the existing cache decoder (`objtype/loctype.go:72, :153`); no decoder change. `paramLookup` (`handlers_config.go:17-49`) is reused for LOC_PARAM via the established NPC_PARAM template (`handlers_config.go:297-311`).

**Tech Stack:** Go 1.26+ (per `go_version.md`).

**Spec:** `docs/superpowers/specs/2026-05-04-nai-85-loc-active-readers-port-design.md` (committed `080355f`).

**Cadence:** spec + plan + single combined review at end (per `compressed_cadence.md` 15–100 LOC band; ~80 production LOC). No per-task two-stage review. One implementer subagent owns T1+T2+T3; reviewer subagent runs once at end.

---

## File Manifest

| File | Action | Responsibility |
|---|---|---|
| `pkg/script/active.go:698-702` | Modify | Add `Shape() int` to `ActiveLoc` interface |
| `pkg/script/handlers_player.go` (after `checkLocAngle`) | Modify | Add `checkLocShape(v int) error` validator |
| `pkg/script/handlers_loc.go` (after `handleLocAngle` at line 100) | Modify | Add four handlers: `handleLocType`, `handleLocName`, `handleLocShape`, `handleLocParam` |
| `pkg/script/handlers.go:122-127` | Modify | Add four dispatch entries: `OpLocName`, `OpLocParam`, `OpLocShape`, `OpLocType` |
| `pkg/script/handlers_loc_test.go:11-19` | Modify | Extend `fakeActiveLoc` with `shape` field + `Shape()` method |
| `pkg/script/handlers_loc_test.go:23-25` | Modify | Extend `fakeConfigs` with `params` field + `ParamType` lookup |
| `pkg/script/handlers_player_test.go:15-23` | Modify | Extend `mockActiveLoc` with `shape` field + `Shape()` method |
| `pkg/script/handlers_loc_test.go` (after `TestHandleLocAngleRequiresActiveLoc`) | Modify | Add 11 test functions for the 4 new handlers |

---

## Task 1: Red — failing tests + compile-broken interface

**Goal:** Land the interface extension, both mock updates, the `fakeConfigs` extension, and 11 test functions in one commit. Compile fails until the four handlers exist.

**Files:**
- Modify: `pkg/script/active.go:698-702`
- Modify: `pkg/script/handlers_loc_test.go:11-19` (mock) + `:23-25` (configs) + append (tests)
- Modify: `pkg/script/handlers_player_test.go:15-23` (mock)

### Step 1.1: Extend `ActiveLoc` interface

- [ ] Edit `pkg/script/active.go`. Replace the current interface block (lines 695-702):

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

with:

```go
// ActiveLoc is the surface that LOC_* opcodes use to read the loc
// bound to the script's execution. Set by OPLOC trigger routing
// (S6j fireOpTriggerLoc) and LOC_FIND (future).
type ActiveLoc interface {
	LocType() int              // returns the LocType ID (from packed Loc.Info bitfield)
	Coords() (x, z, level int) // world position; consumed by LOC_COORD
	Angle() int                // rotation (0=west, 1=north, 2=east, 3=south); consumed by LOC_ANGLE
	Shape() int                // shape (0..22 valid range); consumed by LOC_SHAPE
}
```

### Step 1.2: Extend `fakeActiveLoc`

- [ ] Edit `pkg/script/handlers_loc_test.go`. Replace lines 10-19:

```go
// fakeActiveLoc is a minimal ActiveLoc implementation for handler tests.
type fakeActiveLoc struct {
	id          int
	x, z, level int
	angle       int
}

func (f fakeActiveLoc) LocType() int              { return f.id }
func (f fakeActiveLoc) Coords() (x, z, level int) { return f.x, f.z, f.level }
func (f fakeActiveLoc) Angle() int                { return f.angle }
```

with:

```go
// fakeActiveLoc is a minimal ActiveLoc implementation for handler tests.
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

Existing call sites at `:48`, `:150`, `:167`, `:202` construct `fakeActiveLoc{id: X, …}`; Go zero-values for the new `shape` field preserve current behaviour (zero is in the [0,22] LocShape range). **Do not modify those call sites.**

### Step 1.3: Extend `fakeConfigs`

- [ ] Edit `pkg/script/handlers_loc_test.go`. Replace lines 23-25:

```go
// fakeConfigs implements the Configs interface with just the LocType path
// wired for these tests; other methods return nil.
type fakeConfigs struct {
	locs map[int]*objtype.LocType
}
```

with:

```go
// fakeConfigs implements the Configs interface with the LocType and
// ParamType paths wired for these tests; other methods return nil.
type fakeConfigs struct {
	locs   map[int]*objtype.LocType
	params map[int]*objtype.ParamType
}
```

Then update the `ParamType` method at line 32. Replace:

```go
func (f *fakeConfigs) ParamType(id int) *objtype.ParamType          { return nil }
```

with:

```go
func (f *fakeConfigs) ParamType(id int) *objtype.ParamType          { return f.params[id] }
```

The `params` map is nil-safe to read from (Go map zero-value reads return the zero value of the value type — `nil` for `*objtype.ParamType`), so existing call sites that construct `&fakeConfigs{locs: ...}` without `params` continue to work. **Do not modify the existing `&fakeConfigs{locs: ...}` call sites at `:49`, `:151`.**

### Step 1.4: Extend `mockActiveLoc`

- [ ] Edit `pkg/script/handlers_player_test.go`. Replace lines 15-23:

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

with:

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

Existing call sites at `:952`, `:1010`, `:1091`, `:2369` construct `&mockActiveLoc{locType: 42}`; zero-values for the new `shape` field preserve behaviour. **Do not modify those call sites.**

### Step 1.5: Add the 11 test functions

- [ ] Append to `pkg/script/handlers_loc_test.go` (after `TestHandleLocAngleRequiresActiveLoc` at line 230):

```go

// -- LOC_TYPE tests --

func TestHandleLocTypeHappyPath(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42},
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}},
	}

	if err := handleLocType(s); err != nil {
		t.Fatalf("handleLocType: %v", err)
	}

	if s.ISP != 1 {
		t.Fatalf("ISP: got %d, want 1", s.ISP)
	}
	if got := s.IntStack[0]; got != 42 {
		t.Errorf("top of int stack: got %d, want 42", got)
	}
}

func TestHandleLocTypeRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{}},
	}

	err := handleLocType(s)
	if err == nil {
		t.Fatal("handleLocType: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_TYPE: no active loc" {
		t.Errorf("error: got %q, want \"LOC_TYPE: no active loc\"", got)
	}
}

func TestHandleLocTypeUnknownID(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 999},
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{}},
	}

	err := handleLocType(s)
	if err == nil {
		t.Fatal("handleLocType: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_TYPE: unknown loc id 999" {
		t.Errorf("error: got %q, want \"LOC_TYPE: unknown loc id 999\"", got)
	}
}

// -- LOC_NAME tests --

func TestHandleLocNameHappyPath(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Name:       "door",
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42},
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}},
	}

	if err := handleLocName(s); err != nil {
		t.Fatalf("handleLocName: %v", err)
	}

	if s.SSP != 1 {
		t.Fatalf("SSP: got %d, want 1", s.SSP)
	}
	if got := s.StringStack[0]; got != "door" {
		t.Errorf("top of string stack: got %q, want \"door\"", got)
	}
}

func TestHandleLocNameNullFallback(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		// Name left empty — verifies "null" fallback per TS `name ?? 'null'`.
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42},
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}},
	}

	if err := handleLocName(s); err != nil {
		t.Fatalf("handleLocName: %v", err)
	}

	if got := s.StringStack[0]; got != "null" {
		t.Errorf("top of string stack: got %q, want \"null\"", got)
	}
}

func TestHandleLocNameRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{}},
	}

	err := handleLocName(s)
	if err == nil {
		t.Fatal("handleLocName: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_NAME: no active loc" {
		t.Errorf("error: got %q, want \"LOC_NAME: no active loc\"", got)
	}
}

// -- LOC_SHAPE tests --

func TestHandleLocShapeHappyPath(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42, shape: 10},
	}

	if err := handleLocShape(s); err != nil {
		t.Fatalf("handleLocShape: %v", err)
	}

	if s.ISP != 1 {
		t.Fatalf("ISP: got %d, want 1", s.ISP)
	}
	if got := s.IntStack[0]; got != 10 {
		t.Errorf("top of int stack: got %d, want 10", got)
	}
}

func TestHandleLocShapeRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}

	err := handleLocShape(s)
	if err == nil {
		t.Fatal("handleLocShape: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_SHAPE: no active loc" {
		t.Errorf("error: got %q, want \"LOC_SHAPE: no active loc\"", got)
	}
}

// -- LOC_PARAM tests --

func TestHandleLocParamHappyPathInt(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Params:     objtype.ParamMap{1: uint32(7)},
	}
	pInt := objtype.NewParamType(1)
	pInt.Type = objtype.ScriptVarTypeInt
	pInt.DefaultInt = 0

	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42},
		Configs: &fakeConfigs{
			locs:   map[int]*objtype.LocType{42: lt},
			params: map[int]*objtype.ParamType{1: pInt},
		},
	}
	s.PushInt(1) // paramID

	if err := handleLocParam(s); err != nil {
		t.Fatalf("handleLocParam: %v", err)
	}

	if s.ISP != 1 {
		t.Fatalf("ISP: got %d, want 1", s.ISP)
	}
	if got := s.IntStack[0]; got != 7 {
		t.Errorf("top of int stack: got %d, want 7", got)
	}
}

func TestHandleLocParamHappyPathString(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Params:     objtype.ParamMap{2: "hello"},
	}
	pStr := objtype.NewParamType(2)
	pStr.Type = objtype.ScriptVarTypeString
	pStr.DefaultString = ""

	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42},
		Configs: &fakeConfigs{
			locs:   map[int]*objtype.LocType{42: lt},
			params: map[int]*objtype.ParamType{2: pStr},
		},
	}
	s.PushInt(2) // paramID

	if err := handleLocParam(s); err != nil {
		t.Fatalf("handleLocParam: %v", err)
	}

	if s.SSP != 1 {
		t.Fatalf("SSP: got %d, want 1", s.SSP)
	}
	if got := s.StringStack[0]; got != "hello" {
		t.Errorf("top of string stack: got %q, want \"hello\"", got)
	}
}

func TestHandleLocParamRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{}},
	}
	s.PushInt(1) // paramID — present so the no-active-loc gate fires before any pop

	err := handleLocParam(s)
	if err == nil {
		t.Fatal("handleLocParam: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_PARAM: no active loc" {
		t.Errorf("error: got %q, want \"LOC_PARAM: no active loc\"", got)
	}
}
```

### Step 1.6: Verify red — compile fails

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: build fails in `pkg/script/handlers_loc_test.go` with "undefined" errors for `handleLocType`, `handleLocName`, `handleLocShape`, `handleLocParam`. Compilation OK in non-test packages — the interface extension compiles cleanly because `pkg/entity.Loc.Shape()` already exists at `loc.go:31`, and the only other concrete `ActiveLoc` implementers are `fakeActiveLoc` / `mockActiveLoc` (already extended above).

If the build passes (i.e., something already satisfies all four handlers), STOP — investigate before proceeding.

### Step 1.7: Commit T1

- [ ] Run:

```bash
git add pkg/script/active.go pkg/script/handlers_loc_test.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(script): NAI-85 T1 — LOC_PARAM/NAME/TYPE/SHAPE failing tests + Shape() on ActiveLoc

Adds Shape() to the ActiveLoc interface, extends fakeActiveLoc and
mockActiveLoc to satisfy it, extends fakeConfigs with a params field +
ParamType lookup, and lands 11 test functions covering happy paths,
no-active-loc gates, unknown-id errors, and LOC_PARAM int/string
dispatch. Compile fails on undefined handleLocType/Name/Shape/Param
until T2.
EOF
)"
```

---

## Task 2: Green — validator + 4 handlers + dispatch

**Goal:** Land `checkLocShape`, four handlers, and four dispatch entries in one commit. All 11 T1 tests pass after this.

**Files:**
- Modify: `pkg/script/handlers_player.go` (after `checkLocAngle`)
- Modify: `pkg/script/handlers_loc.go` (append after `handleLocAngle` at line 100)
- Modify: `pkg/script/handlers.go:122-127`

### Step 2.1: Add `checkLocShape`

- [ ] Edit `pkg/script/handlers_player.go`. Insert immediately after the closing `}` of `checkLocAngle` (function spans lines 87-92 post-NAI-83; insert at line 93, before `checkStringNotNull` at line 100):

```go

// checkLocShape mirrors TS LocShapeValid (ScriptValidators.ts) — a
// ScriptInputRangeValidator over [LocShape.WALL_STRAIGHT=0,
// LocShape.GROUND_DECOR=22]. Rejects values outside that range.
//
// Note: pkg/entity.Loc.Shape() returns (l.Info >> 14) & 0x1F which
// covers [0,31] — wider than the LocShape valid range. Caller wraps
// the error with "LOC_SHAPE: %w" so the script abort message names
// the opcode.
func checkLocShape(v int) error {
	if v < 0 || v > 22 {
		return fmt.Errorf("LocShape out of range: %d", v)
	}
	return nil
}
```

`fmt` is already imported in `handlers_player.go`. No import changes.

### Step 2.2: Add four handlers

- [ ] Edit `pkg/script/handlers_loc.go`. Append after the closing `}` of `handleLocAngle` (current line 100):

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
//	pushInt(check(activeLoc.shape, LocShapeValid));
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

`fmt` is already imported in `handlers_loc.go` (line 4). The Configs interface is in the `script` package itself; field access through `*objtype.LocType` returned by `s.Configs.LocType(...)` does not require an `objtype` import (sibling `handleLocOp:55-65` does the same `cfg.Op[idx]` access without one). No import changes.

### Step 2.3: Wire into dispatch

- [ ] Edit `pkg/script/handlers.go`. Replace lines 122-127:

```go
	// LOC lookup — stub (always "not found"). Real impl ships with S6.
	OpLocCoord: handleLocCoord,
	OpLocFind:  handleLocFind,
	// LOC active-loc reads.
	OpLocAngle: handleLocAngle,
	OpLocOp:    handleLocOp,
```

with:

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

(New entries `OpLocName`, `OpLocParam`, `OpLocShape`, `OpLocType` slot in lexically under the existing "LOC active-loc reads" sub-comment alongside `OpLocAngle` and `OpLocOp`. Pre-existing labeling for `OpLocCoord` / `OpLocFind` stays untouched per spec §4.4 — out of scope.)

### Step 2.4: Verify green — pkg/script tests pass

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestHandleLoc(Type|Name|Shape|Param)' -v`

Expected:
```
=== RUN   TestHandleLocTypeHappyPath
--- PASS: TestHandleLocTypeHappyPath (0.00s)
=== RUN   TestHandleLocTypeRequiresActiveLoc
--- PASS: TestHandleLocTypeRequiresActiveLoc (0.00s)
=== RUN   TestHandleLocTypeUnknownID
--- PASS: TestHandleLocTypeUnknownID (0.00s)
=== RUN   TestHandleLocNameHappyPath
--- PASS: TestHandleLocNameHappyPath (0.00s)
=== RUN   TestHandleLocNameNullFallback
--- PASS: TestHandleLocNameNullFallback (0.00s)
=== RUN   TestHandleLocNameRequiresActiveLoc
--- PASS: TestHandleLocNameRequiresActiveLoc (0.00s)
=== RUN   TestHandleLocShapeHappyPath
--- PASS: TestHandleLocShapeHappyPath (0.00s)
=== RUN   TestHandleLocShapeRequiresActiveLoc
--- PASS: TestHandleLocShapeRequiresActiveLoc (0.00s)
=== RUN   TestHandleLocParamHappyPathInt
--- PASS: TestHandleLocParamHappyPathInt (0.00s)
=== RUN   TestHandleLocParamHappyPathString
--- PASS: TestHandleLocParamHappyPathString (0.00s)
=== RUN   TestHandleLocParamRequiresActiveLoc
--- PASS: TestHandleLocParamRequiresActiveLoc (0.00s)
PASS
```

If any fail, STOP and diagnose against the spec §4 / §5 code blocks.

### Step 2.5: Commit T2

- [ ] Run:

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_loc.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-85 T2 — handleLoc{Type,Name,Shape,Param} + checkLocShape + dispatch

Greens T1's 11 failing tests. checkLocShape mirrors TS LocShapeValid
(range [0,22]); handleLocType/Name/Shape pushes via active-loc accessors
with Configs nil-check standing in for TS LocTypeValid; handleLocParam
reuses paramLookup via the established NPC_PARAM template. Dispatch
entries slot under the existing "LOC active-loc reads" sub-comment in
lexical order. LOC_NAME ships TS-correct (Name → "null"); LC_NAME
follow-up (NAI-N+1) tracked separately.
EOF
)"
```

---

## Task 3: Verify — full-repo regression scan

**Goal:** Confirm the additive interface change broke nothing elsewhere and the new code passes `go vet`.

**Files:** none modified.

### Step 3.1: Full test suite

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all packages green. The `ActiveLoc` interface extension is purely additive — `pkg/entity.Loc` already implements `Shape()` (loc.go:31), and the only other concrete implementers are `fakeActiveLoc` / `mockActiveLoc` (already extended in T1).

If any package fails, the most likely cause is an additional `ActiveLoc` implementer outside `pkg/script` test files. Grep first:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./... 2>&1 | grep -i "does not implement ActiveLoc\|missing Shape"
```

Add `Shape() int` to any concrete type the grep surfaces (returning the underlying shape field — for an `*entity.Loc` wrapper, delegate to `Loc.Shape()`).

### Step 3.2: Vet clean

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no output (clean).

### Step 3.3: No commit needed for T3

T3 is verification-only. No file changes. Proceed to combined review.

---

## Combined Review (single, end-of-impl)

**Per `compressed_cadence.md` 15–100 LOC band:** dispatch ONE reviewer subagent against the cumulative diff `080355f..HEAD` (spec→impl range), not per-task reviewers.

**Reviewer prompt template (subagent):**

> Review the implementation of NAI-85 (LOC_PARAM/NAME/TYPE/SHAPE active-loc reader port). Spec at `docs/superpowers/specs/2026-05-04-nai-85-loc-active-readers-port-design.md` (commit `080355f`). Plan at `docs/superpowers/plans/2026-05-04-nai-85-loc-active-readers-port.md`. Cumulative diff: `git diff 080355f..HEAD`.
>
> TS reference: `LostCityRS/Engine-TS/src/engine/script/handlers/LocOps.ts:114-135` and `ScriptValidators.ts` (`LocTypeValid`, `LocShapeValid`, `ParamTypeValid`).
>
> Verify:
> 1. **TS-fidelity per opcode**:
>    - `handleLocType`: pushes `id` (matches `pushInt(check(activeLoc.type, LocTypeValid).id)`); Configs nil-check is the LocTypeValid equivalent.
>    - `handleLocName`: pushes `lt.Name` or `"null"` (matches `name ?? 'null'`); does NOT fall back to DebugName (LC_NAME divergence is a separate tracked follow-up).
>    - `handleLocShape`: pushes shape iff `checkLocShape` passes (matches `pushInt(check(activeLoc.shape, LocShapeValid))`).
>    - `handleLocParam`: pops paramID, resolves LocType, delegates to `paramLookup` (matches TS `ParamHelper.getIntParam/getStringParam` dispatch via ParamType.isString()).
> 2. **Validator shape**: `checkLocShape` rejects v<0 or v>22, accepts 0…22.
> 3. **Interface additive**: `ActiveLoc.Shape()` is the only signature change; existing implementers (`*entity.Loc`, `fakeActiveLoc`, `mockActiveLoc`) all implement it; no other production types broken.
> 4. **Dispatch wiring**: `OpLocName: handleLocName`, `OpLocParam: handleLocParam`, `OpLocShape: handleLocShape`, `OpLocType: handleLocType` registered exactly once each under "LOC active-loc reads" sub-comment, in lexical order alongside existing `OpLocAngle`/`OpLocOp`.
> 5. **fakeConfigs extension**: `params` field added; `ParamType(id)` reads from it; existing `&fakeConfigs{locs: ...}` literals at `:49`, `:151` still compile (zero-value nil map is read-safe).
> 6. **Test coverage**: 11 tests added — happy paths push correct values; missing-active-loc returns the spec'd error literals (`"LOC_X: no active loc"`); LOC_TYPE unknown-id error has correct format (`"LOC_TYPE: unknown loc id 999"`); LOC_PARAM dispatches int vs string by ParamType.Type. No synthetic out-of-range LOC_SHAPE test (correct per spec §5).
> 7. **No scope creep**: spec §4.4 minimal-touch dispatch wiring honored — no regrouping of `OpLocCoord` / `OpLocFind` labeling. No additional opcodes wired beyond the four. LC_NAME divergence flagged but NOT modified (deferred to NAI-N+1).
> 8. **`go test ./...`** and **`go vet ./...`** both clean against working tree.
> 9. **`git show <commit-SHA> --stat`** for T1 and T2 — verify implementer commits' file lists match the plan File Manifest (per `implementer_commit_content_verify.md`):
>    - T1: `pkg/script/active.go`, `pkg/script/handlers_loc_test.go`, `pkg/script/handlers_player_test.go`.
>    - T2: `pkg/script/handlers_player.go`, `pkg/script/handlers_loc.go`, `pkg/script/handlers.go`.
>
> Report any deviations or missed items. Sonnet model only (per `superpowers_code_reviewer_model.md`).

---

## Close Commit

After reviewer subagent reports clean (or after addressing any review feedback), run a close commit:

- [ ] Run:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
close: NAI-85 — LOC_PARAM/NAME/TYPE/SHAPE active-loc readers ported

Cascade: door-click smoke at NAI-83 close (HEAD 469d07b) surfaced
[oploc1, newbie_door1] no-handler-for-LOC_PARAM error at pc=14. Now
wired: ActiveLoc.Shape() accessor, checkLocShape range validator, four
handlers (LOC_TYPE/NAME/SHAPE/PARAM), four dispatch entries. LOC_NAME
ships TS-correct; LC_NAME field-choice fix tracked as NAI-N+1
follow-up. Cascade attribution closes at the next user-driven Tutorial
Island door-click smoke.

Closes memory: nai85_loc_active_readers.md
EOF
)"
```

(Empty close commit per `close_commit_memory_trailer.md` — gives the memory ledger a discoverable provenance entry from `git log --grep`.)

---

## Self-Review

**Spec coverage:**
- §4.1 interface extension (Shape) → T1 Step 1.1 ✓
- §4.2 validator (checkLocShape) → T2 Step 2.1 ✓
- §4.3 four handlers → T2 Step 2.2 ✓
- §4.4 dispatch wiring (4 entries) → T2 Step 2.3 ✓
- §4.5 mock updates (fakeActiveLoc + mockActiveLoc) → T1 Steps 1.2, 1.4 ✓
- §3 fakeConfigs extension (params map) → T1 Step 1.3 ✓
- §5 11 test functions → T1 Step 1.5 ✓
- §5.1 pre-flight verification (HEAD 469d07b) → File Manifest line numbers ✓
- §6 TS-fidelity ledger → reviewer prompt items 1–4 ✓
- §6 LC_NAME inherited divergence → close commit body + reviewer prompt item 7 ✓
- §8 smoke / cascade routing → close commit body ✓
- §10 plan handoff (T1 red / T2 green / T3 verify / single review / close commit) → all sections ✓

**Placeholder scan:** no TBDs, no "appropriate error handling", no "TODO". All code blocks complete with concrete identifiers, test inputs, and expected outputs.

**Type consistency:**
- `Shape() int` on `ActiveLoc` interface (Step 1.1) matches `(f fakeActiveLoc) Shape() int` (Step 1.2), `(m *mockActiveLoc) Shape() int` (Step 1.4, pointer receiver), and `(l *Loc) Shape() int` (pre-existing at `pkg/entity/loc.go:31`).
- `checkLocShape(v int) error` signature (Step 2.1) matches the call site in `handleLocShape` (Step 2.2).
- `handleLocType(s *ScriptState) error`, `handleLocName(s *ScriptState) error`, `handleLocShape(s *ScriptState) error`, `handleLocParam(s *ScriptState) error` signatures (Step 2.2) match the test call sites (Step 1.5) and the dispatch map entries (Step 2.3).
- `fakeConfigs.params` field (Step 1.3, type `map[int]*objtype.ParamType`) matches the read site `f.params[id]` and the write site in tests `params: map[int]*objtype.ParamType{...}` (Step 1.5).
- Error literals in tests (Step 1.5) match the `fmt.Errorf` formats in handlers (Step 2.2): `"LOC_TYPE: no active loc"`, `"LOC_TYPE: unknown loc id 999"`, `"LOC_NAME: no active loc"`, `"LOC_SHAPE: no active loc"`, `"LOC_PARAM: no active loc"`.

**Spec-to-plan exit clean.**
