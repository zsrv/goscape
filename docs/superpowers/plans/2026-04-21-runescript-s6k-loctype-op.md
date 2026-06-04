# LocType.Op + LOC_OP Script Opcode Implementation Plan (S6k)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close S6j-D1 (handler op-gate) and S6j-D7 (defaultOp message) by adding `LocType.Op []string` config field, the `loc_op` script opcode, and the two `modules/world` wiring sites those enable.

**Architecture:** Four coordinated changes across three package boundaries. Task 1 adds the `Op` field + cache decoder in `pkg/objtype` (pure additive). Task 2 expands the `ActiveLoc` interface with `LocType() int`, adds the matching method on `*entity.Loc`, registers `OpLocOp = 3014`, and wires `handleLocOp` in `pkg/script`. Task 3 restores the OPLOC per-op validation gate in `modules/world/handler_oploc.go` and adds the defaultOp `MessageGame` call in `fireOpTriggerLoc`.

**Tech Stack:** Go 1.26 (stdlib only). Tests use existing fixtures (`makeOpLocFixture`, `buildLocDat`, OPNPC-style test-LocType-register pattern).

**Spec reference:** `docs/superpowers/specs/2026-04-21-runescript-s6k-loctype-op-design.md` (commit `d21154c`).

**Build commands (per CLAUDE.md):**
- Build: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
- Test all: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
- Test one: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run TestName -v`
- Vet: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

**Commit policy (per CLAUDE.md):** All commits use `git commit --no-gpg-sign`.

---

## File Structure

| File | Created/Modified | Responsibility | Task |
|---|---|---|---|
| `pkg/objtype/loctype.go` | Modify | Add `Op []string` field; add decoder cases 30-34 (lazy 5-slot init + `"hidden"` → `""` coercion) | 1 |
| `pkg/objtype/loctype_test.go` | Modify | Extend `locEntry` struct with `op []string`; extend `buildLocDat` to emit codes 30-34; add 3 decoder tests | 1 |
| `pkg/entity/loc.go` | Modify | Add `Loc.LocType() int` method (alias for `Type()`) satisfying `pkg/script.ActiveLoc` | 2 |
| `pkg/script/active.go` | Modify | Expand `ActiveLoc` from `interface{}` to `interface { LocType() int }` | 2 |
| `pkg/script/opcode.go` | Modify | Register `OpLocOp Opcode = 3014` | 2 |
| `pkg/script/handlers.go` | Modify | Wire `OpLocOp → handleLocOp` in dispatch table | 2 |
| `pkg/script/handlers_loc.go` | Modify | Add `requireActiveLoc` + `handleLocOp` helpers | 2 |
| `pkg/script/handlers_loc_test.go` | Create | 5 `handleLocOp` tests | 2 |
| `modules/world/handler_oploc.go` | Modify | Restore S6j-D1 op-validation gate; update deviation comment | 3 |
| `modules/world/handler_oploc_test.go` | Modify | Extend `makeOpLocFixture` LocType with `Op`; add 2 new gate tests | 3 |
| `modules/world/interaction_trigger.go` | Modify | Insert `p.MessageGame(...)` before no-script clear in `fireOpTriggerLoc` | 3 |
| `modules/world/interaction_trigger_test.go` | Modify | Update `TestTryFireOpTriggerLocNoScript` to assert MessageGame sent | 3 |

**Existing infrastructure already in place (no changes needed):**
- `Player.MessageGame(msg string)` — `modules/world/message_game.go`
- `NpcType.Op` decoder template — `pkg/objtype/npctype.go:124-132`
- `handleNpcHasOp` (closest sibling, bool return) — `pkg/script/handlers_npc.go:87`
- `Configs.LocType(id int) *objtype.LocType` — `pkg/script/configs.go:11`
- `OpLocAdd..OpLocType = 3000..3013` — `pkg/script/opcode.go:289-302`; `OpLocOp = 3014` is next free
- `ScriptState.ActiveLoc` field — `pkg/script/state.go` (S6j Task 1)
- `makeOpLocFixture` — `modules/world/handler_oploc_test.go`
- `buildLocDat` + `locEntry` — `pkg/objtype/loctype_test.go`

---

## Task 1: LocType.Op Field + Cache Decoder

**Goal:** Pure additive addition in `pkg/objtype`. After this task, `LocType.Op []string` exists and `loc.dat` cache reads with codes 30-34 populate it correctly. No consumer changes yet.

**Files:**
- Modify: `pkg/objtype/loctype.go`
- Modify: `pkg/objtype/loctype_test.go`

### Step-by-step

- [ ] **Step 1.1: Write failing decoder test for a single Op entry**

In `pkg/objtype/loctype_test.go`, first extend the `locEntry` struct (top of file, near line 10) to include an `op` field:

```go
type locEntry struct {
	debugName string
	desc      string
	category  int
	width     int
	length    int
	intParams map[uint32]uint32
	op        []string // NEW — S6k: op-name slots (codes 30-34)
}
```

Then extend `buildLocDat` to emit op codes. Find the per-entry write block (around line 27) and append BEFORE the `pkt.P1(0)` terminator:

```go
		// Op entries (codes 30-34). S6k: emit one code per non-empty slot.
		for i, name := range e.op {
			if name == "" {
				continue
			}
			pkt.P1(uint8(30 + i))
			pkt.PJStrLF(name)
		}
```

Now append this test function to `loctype_test.go`:

```go
func TestLocTypeDecodeOpSingleEntry(t *testing.T) {
	dat := buildLocDat([]locEntry{
		{debugName: "tree", op: []string{"Chop", "", "", "", ""}},
	})
	pkt := packet2.NewPacket(dat)

	cfgs, err := parseLocTypes(pkt)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}
	if got := len(cfgs.Configs); got != 1 {
		t.Fatalf("Configs len: got %d, want 1", got)
	}

	tree := cfgs.Configs[0]
	if tree.Op == nil {
		t.Fatal("Op: got nil, want 5-slot slice")
	}
	if got := tree.Op[0]; got != "Chop" {
		t.Errorf("Op[0]: got %q, want \"Chop\"", got)
	}
	for i := 1; i < 5; i++ {
		if tree.Op[i] != "" {
			t.Errorf("Op[%d]: got %q, want \"\"", i, tree.Op[i])
		}
	}
}
```

- [ ] **Step 1.2: Run the test and confirm compile failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run TestLocTypeDecodeOpSingleEntry -v`

Expected: compile failure — `tree.Op undefined (type *LocType has no field or method Op)` and/or `unknown field 'op' in struct literal`.

- [ ] **Step 1.3: Add the `Op` field to the `LocType` struct**

In `pkg/objtype/loctype.go`, modify the `LocType` struct (lines 18-25). Insert `Op []string` after `Length`:

```go
type LocType struct {
	ConfigType
	Category int
	Desc     string
	Width    int
	Length   int
	Op       []string // S6k: 5 click-option names, nil until decoded
	Params   ParamMap
}
```

- [ ] **Step 1.4: Run the test and confirm a different failure (decoder case missing)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run TestLocTypeDecodeOpSingleEntry -v`

Expected: FAIL at runtime with `unrecognized loc config code 30`. The compile error from Step 1.2 is now fixed, but the decoder still rejects code 30.

- [ ] **Step 1.5: Add decoder cases 30-34 to `LocType.Decode`**

In `pkg/objtype/loctype.go`, find the `Decode` method (lines 27-45). Insert the new cases BEFORE the `default:` branch:

```go
func (lt *LocType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 3:
		lt.Desc = dat.GJStrLF()
	case 14:
		lt.Width = int(dat.G1())
	case 15:
		lt.Length = int(dat.G1())
	case 30, 31, 32, 33, 34:
		// S6k: op-name slots. Lazy 5-slot init mirrors NpcType.Op
		// (npctype.go:124-132). TS LocType.ts:152-157 uses
		// `code >= 30 && < 35`. The "hidden" keyword in the cache
		// marks a disabled op slot; we coerce to "" here so the
		// handler gate in modules/world/handler_oploc.go can do a
		// single empty-string check at runtime.
		if lt.Op == nil {
			lt.Op = make([]string, 5)
		}
		lt.Op[code-30] = dat.GJStrLF()
		if lt.Op[code-30] == "hidden" {
			lt.Op[code-30] = ""
		}
	case 61:
		lt.Category = int(dat.G2())
	case 249:
		lt.Params = DecodeParams(dat)
	case 250:
		lt.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized loc config code %d", code)
	}
	return nil
}
```

- [ ] **Step 1.6: Run the test and confirm PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run TestLocTypeDecodeOpSingleEntry -v`

Expected: PASS.

- [ ] **Step 1.7: Add test for all 5 op slots decoded together**

Append to `loctype_test.go`:

```go
func TestLocTypeDecodeOpAllFive(t *testing.T) {
	dat := buildLocDat([]locEntry{
		{debugName: "multi", op: []string{"op0", "op1", "op2", "op3", "op4"}},
	})
	pkt := packet2.NewPacket(dat)

	cfgs, err := parseLocTypes(pkt)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}

	multi := cfgs.Configs[0]
	want := []string{"op0", "op1", "op2", "op3", "op4"}
	for i, w := range want {
		if got := multi.Op[i]; got != w {
			t.Errorf("Op[%d]: got %q, want %q", i, got, w)
		}
	}
}
```

- [ ] **Step 1.8: Run the new test and confirm PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run TestLocTypeDecodeOpAllFive -v`

Expected: PASS.

- [ ] **Step 1.9: Add test for "hidden" keyword coercion**

Append to `loctype_test.go`:

```go
func TestLocTypeDecodeOpHiddenCoercedToEmpty(t *testing.T) {
	dat := buildLocDat([]locEntry{
		{debugName: "hidden_test", op: []string{"visible", "hidden", "", "", ""}},
	})
	pkt := packet2.NewPacket(dat)

	cfgs, err := parseLocTypes(pkt)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}

	entry := cfgs.Configs[0]
	if got := entry.Op[0]; got != "visible" {
		t.Errorf("Op[0]: got %q, want \"visible\"", got)
	}
	if got := entry.Op[1]; got != "" {
		t.Errorf("Op[1] (hidden-coerced): got %q, want \"\"", got)
	}
}
```

- [ ] **Step 1.10: Run the hidden-coercion test and confirm PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run TestLocTypeDecodeOpHiddenCoercedToEmpty -v`

Expected: PASS.

- [ ] **Step 1.11: Run the full test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all existing tests still pass; 3 new tests pass.

- [ ] **Step 1.12: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no warnings.

- [ ] **Step 1.13: Commit Task 1**

```bash
git add pkg/objtype/loctype.go pkg/objtype/loctype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): LocType.Op field + cache decoder (S6k-1)

Adds the LocType.Op []string field (5 click-option name slots) and
the matching cache-decoder cases for codes 30-34. Mirrors NpcType.Op
(npctype.go:124-132) verbatim: lazy 5-slot init and the "hidden"
keyword is coerced to "" at decode time so the runtime handler gate
needs only a single empty-string check.

Per TS cache/config/LocType.ts:152-157 (code >= 30 && < 35).

Pure additive — no consumer reads Op yet. S6k Tasks 2 and 3 wire the
script handler (LOC_OP) and the handler-side op-validation gate that
closes S6j-D1.

3 decoder tests: single-entry, all-five, and "hidden" coercion.

Spec: docs/superpowers/specs/2026-04-21-runescript-s6k-loctype-op-design.md
Plan: docs/superpowers/plans/2026-04-21-runescript-s6k-loctype-op.md (Task 1)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: ActiveLoc Interface + Loc.LocType + handleLocOp

**Goal:** Script side. After this task, scripts can call `loc_op(op)` (OpLocOp = 3014) to read the Op-slot string for the currently bound ActiveLoc.

**Files:**
- Modify: `pkg/entity/loc.go`
- Modify: `pkg/script/active.go`
- Modify: `pkg/script/opcode.go`
- Modify: `pkg/script/handlers.go`
- Modify: `pkg/script/handlers_loc.go`
- Create: `pkg/script/handlers_loc_test.go`

### Step-by-step

- [ ] **Step 2.1: Add `Loc.LocType() int` method**

In `pkg/entity/loc.go`, append after the existing `Angle()` method (around line 34, after Task 1 shipped — `Angle` is unchanged):

```go
// LocType returns the LocType ID for this loc. Satisfies the
// pkg/script.ActiveLoc interface. Alias for Type() with a
// less-ambiguous name when the loc is bound to script state.
func (l *Loc) LocType() int { return l.Type() }
```

- [ ] **Step 2.2: Expand the ActiveLoc interface**

In `pkg/script/active.go`, find line 303 (the stub `type ActiveLoc interface{}`) and replace with:

```go
// ActiveLoc is the surface that LOC_* opcodes use to read the loc
// bound to the script's execution. Set by OPLOC trigger routing
// (S6j fireOpTriggerLoc) and LOC_FIND (future).
type ActiveLoc interface {
	LocType() int // returns the LocType ID (from packed Loc.Info bitfield)
}
```

- [ ] **Step 2.3: Run build to confirm no pre-existing consumer breaks**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: builds successfully. The only `state.ActiveLoc = loc` setter in goscape is in `fireOpTriggerLoc` (from S6j), and `*entity.Loc` gained `LocType()` in Step 2.1 — so the interface is satisfied.

- [ ] **Step 2.4: Write failing test for `handleLocOp` happy path**

Create `pkg/script/handlers_loc_test.go`:

```go
package script

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// fakeActiveLoc is a minimal ActiveLoc implementation for handler tests.
type fakeActiveLoc struct{ id int }

func (f fakeActiveLoc) LocType() int { return f.id }

// fakeConfigs implements the Configs interface with just the LocType path
// wired for these tests; other methods return nil.
type fakeConfigs struct {
	locs map[int]*objtype.LocType
}

func (f *fakeConfigs) ObjType(id int) *objtype.ObjType       { return nil }
func (f *fakeConfigs) NpcType(id int) *objtype.NpcType       { return nil }
func (f *fakeConfigs) LocType(id int) *objtype.LocType       { return f.locs[id] }
func (f *fakeConfigs) EnumType(id int) *objtype.EnumType     { return nil }
func (f *fakeConfigs) StructType(id int) *objtype.StructType { return nil }
func (f *fakeConfigs) ParamType(id int) *objtype.ParamType   { return nil }
func (f *fakeConfigs) InvType(id int) *objtype.InvType       { return nil }

// newLocOpState builds a ScriptState with ActiveLoc bound, Configs wired,
// and a single int on the stack (the op index).
func newLocOpState(locID, op int, locType *objtype.LocType) *ScriptState {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: locID},
		Configs: &fakeConfigs{
			locs: map[int]*objtype.LocType{locID: locType},
		},
	}
	s.PushInt(op)
	return s
}

// TestHandleLocOpHappyPath verifies a valid op index returns the configured
// Op-slot string.
func TestHandleLocOpHappyPath(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Op:         []string{"Chop", "Examine", "", "", ""},
	}
	s := newLocOpState(42, 1, lt)

	if err := handleLocOp(s); err != nil {
		t.Fatalf("handleLocOp: %v", err)
	}

	if s.SSP != 1 {
		t.Fatalf("SSP: got %d, want 1", s.SSP)
	}
	if got := s.StringStack[0]; got != "Chop" {
		t.Errorf("top of string stack: got %q, want \"Chop\"", got)
	}
}
```

- [ ] **Step 2.5: Run the test and confirm compile failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestHandleLocOpHappyPath -v`

Expected: compile failure — `handleLocOp undefined`.

- [ ] **Step 2.6: Implement `requireActiveLoc` and `handleLocOp`**

In `pkg/script/handlers_loc.go`, replace the existing file contents (20-line stub) with:

```go
package script

import "fmt"

// requireActiveLoc returns an error tagged with the opcode name if the
// script has no ActiveLoc bound. All LOC_* read handlers start with
// this check to mirror TS `checkedHandler(ActiveLoc, ...)`.
func requireActiveLoc(s *ScriptState, op string) error {
	if s.ActiveLoc == nil {
		return fmt.Errorf("%s: no active loc", op)
	}
	return nil
}

// handleLocFind is a stub for the LOC_FIND opcode. TS:
//
//	const [coord, locId] = state.popInts(2);
//	loc = World.getLoc(coord.x, coord.z, coord.level, locType.id);
//	if loc: activeLoc = loc; pushInt(1); else: pushInt(0);
//
// MVP stub: pop both args, push 0 (not found). Scripts that branch
// on "found" take the else-branch, which is almost always the safe
// path (e.g. check_chest_macro_gas proc early-returns on LOC_FIND=0).
// Real implementation needs world-wide loc iteration + ActiveLoc
// setup; ships with a later S6 sub-spec.
func handleLocFind(s *ScriptState) error {
	_ = s.PopInt() // locId (type)
	_ = s.PopInt() // coord (packed)
	s.PushInt(0)
	return nil
}

// handleLocOp pops a 1-indexed op slot and pushes the ActiveLoc's
// LocType.Op[op-1] string. Pushes "" if:
//   - Configs is nil (test-only defensive guard)
//   - LocType is not loaded for the ActiveLoc's type ID
//   - op is out of [1, len(Op)] range
//
// Mirrors handleNpcHasOp (handlers_npc.go:87) in structure — same read
// path through Configs → LocType.Op — but returns the string rather
// than a bool (NPC_HASOP's boolean form).
func handleLocOp(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_OP"); err != nil {
		return err
	}
	op := s.PopInt()
	if s.Configs == nil {
		s.PushString("")
		return nil
	}
	cfg := s.Configs.LocType(s.ActiveLoc.LocType())
	if cfg == nil {
		s.PushString("")
		return nil
	}
	idx := op - 1
	if idx < 0 || idx >= len(cfg.Op) {
		s.PushString("")
		return nil
	}
	s.PushString(cfg.Op[idx])
	return nil
}
```

- [ ] **Step 2.7: Run the happy-path test and confirm PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestHandleLocOpHappyPath -v`

Expected: PASS.

- [ ] **Step 2.8: Add the remaining 4 handler tests**

Append to `pkg/script/handlers_loc_test.go`:

```go
// TestHandleLocOpRequiresActiveLoc verifies a nil ActiveLoc returns an
// error tagged "LOC_OP".
func TestHandleLocOpRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(1)

	err := handleLocOp(s)
	if err == nil {
		t.Fatal("handleLocOp: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_OP: no active loc" {
		t.Errorf("error: got %q, want \"LOC_OP: no active loc\"", got)
	}
}

// TestHandleLocOpOutOfRangeLow verifies op=0 (below 1) pushes "".
func TestHandleLocOpOutOfRangeLow(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Op:         []string{"Chop", "", "", "", ""},
	}
	s := newLocOpState(42, 0, lt)

	if err := handleLocOp(s); err != nil {
		t.Fatalf("handleLocOp: %v", err)
	}
	if got := s.StringStack[0]; got != "" {
		t.Errorf("got %q, want \"\" for op=0", got)
	}
}

// TestHandleLocOpOutOfRangeHigh verifies op=6 (above 5) pushes "".
func TestHandleLocOpOutOfRangeHigh(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Op:         []string{"Chop", "", "", "", ""},
	}
	s := newLocOpState(42, 6, lt)

	if err := handleLocOp(s); err != nil {
		t.Fatalf("handleLocOp: %v", err)
	}
	if got := s.StringStack[0]; got != "" {
		t.Errorf("got %q, want \"\" for op=6", got)
	}
}

// TestHandleLocOpEmptySlot verifies an in-range op with an empty Op
// slot pushes "" (this is the common post-"hidden"-coercion case).
func TestHandleLocOpEmptySlot(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Op:         []string{"Chop", "", "", "", ""},
	}
	s := newLocOpState(42, 2, lt) // Op[1] == ""

	if err := handleLocOp(s); err != nil {
		t.Fatalf("handleLocOp: %v", err)
	}
	if got := s.StringStack[0]; got != "" {
		t.Errorf("got %q, want \"\" for empty slot", got)
	}
}

// TestHandleLocOpLocTypeNotLoaded verifies a nil LocType lookup pushes "".
func TestHandleLocOpLocTypeNotLoaded(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 999}, // id not in fakeConfigs
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{}},
	}
	s.PushInt(1)

	if err := handleLocOp(s); err != nil {
		t.Fatalf("handleLocOp: %v", err)
	}
	if got := s.StringStack[0]; got != "" {
		t.Errorf("got %q, want \"\" for missing LocType", got)
	}
}
```

Note: this is 5 tests total (1 from Step 2.4 plus 4 new). The "empty slot" test replaces one of the spec's listed tests with a semantically-identical case; the spec's §6.2 "5 tests" count is preserved.

- [ ] **Step 2.9: Run all handleLocOp tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestHandleLocOp -v`

Expected: 5 tests PASS.

- [ ] **Step 2.10: Register the `OpLocOp` opcode**

In `pkg/script/opcode.go`, find the `OpLocType Opcode = 3013` line (around line 302) and append immediately after:

```go
	OpLocOp          Opcode = 3014
```

- [ ] **Step 2.11: Wire `handleLocOp` in the dispatch table**

In `pkg/script/handlers.go`, find the block that registers LOC_* handlers (search for `handleLocFind` to locate). Add an entry alongside the existing LOC_* entries:

```go
	handlers[OpLocOp] = handleLocOp
```

The exact location of the `handlers[...] = ...` assignment in `handlers.go` varies — follow the file's existing pattern. If handlers are registered inside an `init()` function, add the line there; if inside a `map[Opcode]Handler{...}` literal, add the entry there.

- [ ] **Step 2.12: Run the full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests pass.

- [ ] **Step 2.13: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no warnings.

- [ ] **Step 2.14: Commit Task 2**

```bash
git add pkg/entity/loc.go pkg/script/active.go pkg/script/opcode.go pkg/script/handlers.go pkg/script/handlers_loc.go pkg/script/handlers_loc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): handleLocOp + ActiveLoc.LocType binding (S6k-2)

Wires the loc_op script opcode (OpLocOp = 3014) so scripts bound to
an ActiveLoc can read its LocType.Op slot strings.

- ActiveLoc interface expands from empty stub to { LocType() int }.
  Mirrors ActiveNpc's LocType()-equivalent accessor (NpcType()).
- *entity.Loc.LocType() aliases Type() to satisfy the interface; no
  behavior change to existing Loc.Info bitfield reads.
- requireActiveLoc + handleLocOp mirror the handlers_npc.go pattern
  (handleNpcHasOp:87) — same Configs.LocType lookup path, pushes the
  Op-slot string instead of a bool.
- OpLocOp registered at 3014 (next free after OpLocType = 3013).

5 handler tests: happy path, nil ActiveLoc error, op-out-of-range
low/high, empty slot, LocType not loaded.

Spec: docs/superpowers/specs/2026-04-21-runescript-s6k-loctype-op-design.md
Plan: docs/superpowers/plans/2026-04-21-runescript-s6k-loctype-op.md (Task 2)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Handler Op-Gate Restore + defaultOp Message

**Goal:** Close S6j-D1 (per-op validation gate in `handleOpLoc`) and S6j-D7 (defaultOp message in `fireOpTriggerLoc`). After this task, OPLOC routing is TS-faithful for the populated-Op case and emits "Nothing interesting happens." when the player reaches a loc with no registered trigger.

**Files:**
- Modify: `modules/world/handler_oploc.go`
- Modify: `modules/world/handler_oploc_test.go`
- Modify: `modules/world/interaction_trigger.go`
- Modify: `modules/world/interaction_trigger_test.go`

### Step-by-step

- [ ] **Step 3.1: Extend `makeOpLocFixture` LocType with a populated Op field**

In `modules/world/handler_oploc_test.go`, find `makeOpLocFixture` (around line 25-47). The existing block is:

```go
s.locTypes.Configs[42] = &objtype.LocType{
	ConfigType: objtype.ConfigType{ID: 42, DebugName: "test_loc"},
	Category:   7,
}
```

Replace with:

```go
s.locTypes.Configs[42] = &objtype.LocType{
	ConfigType: objtype.ConfigType{ID: 42, DebugName: "test_loc"},
	Category:   7,
	Op:         []string{"op1", "op2", "op3", "op4", "op5"},
}
```

This ensures every existing `TestHandleOpLoc*` test continues passing once the op-gate lands in Step 3.4 — all 5 op slots have non-empty names.

- [ ] **Step 3.2: Run all existing OPLOC handler tests to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpLoc -v`

Expected: all 9 existing tests still pass (no behavior change yet — the gate isn't in place).

- [ ] **Step 3.3: Write failing test for empty-op-slot rejection**

Append to `modules/world/handler_oploc_test.go`:

```go
// TestHandleOpLocRejectsEmptyOpSlot verifies that clicking an op slot
// whose Op string is "" emits UnsetMapFlag and leaves state untouched.
// Closes S6j-D1 coverage.
func TestHandleOpLocRejectsEmptyOpSlot(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)
	// Clear Op[0] so op=1 should reject.
	s.locTypes.Configs[42].Op[0] = ""

	_ = handleOpLoc1(p, p2x3Payload(100, 100, 42))
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for empty Op slot, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil when Op slot is empty")
	}
}
```

- [ ] **Step 3.4: Run the test and confirm FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpLocRejectsEmptyOpSlot -v`

Expected: FAIL — the current handler accepts the click and sets state (no op-gate yet; `p.target != nil` after the call).

- [ ] **Step 3.5: Restore the per-op validation gate in `handleOpLoc`**

In `modules/world/handler_oploc.go`, find the LocType-exists check (around line 71, the `if s.locTypes.Configs[locId] == nil` block). Replace the current structure:

```go
	if s.locTypes.Configs[locId] == nil {
		sendUnsetMapFlag(p)
		return nil
	}
```

With:

```go
	locType := s.locTypes.Configs[locId]
	if locType == nil {
		sendUnsetMapFlag(p)
		return nil
	}
	// S6j-D1 closed in S6k: per-op validation gate. TS OpLocHandler.ts:38-42
	// rejects clicks where locType.op is nil, too short, or the slot
	// is empty. The decoder coerces "hidden" to "" at load time
	// (pkg/objtype/loctype.go cases 30-34), so the runtime check is
	// just `== ""`.
	if len(locType.Op) < op || locType.Op[op-1] == "" {
		sendUnsetMapFlag(p)
		return nil
	}
```

Also update the top-of-function `DEVIATION (S6j-D1)` comment block (lines 18-23 currently). Find this block:

```go
// DEVIATION (S6j-D1): TS gate 6 — `locType.op[op-1] != null && != "hidden"`
// (OpLocHandler.ts:38-42) — is skipped here because LocType.Op []string is
// not yet a field on LocType. Effective behavior: trigger registration
// absence becomes the gate (no trigger → silent no-op on next tick instead
// of TS's UnsetMapFlag at click time). Follow-up: "LocType.Op + loc_op
// script opcode" sub-spec.
```

Replace with:

```go
// S6j-D1 closed in S6k: per-op validation gate (locType.Op[op-1])
// restored below, mirroring handler_opnpc.go:38-44 for consistency.
```

- [ ] **Step 3.6: Run the empty-op test and confirm PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpLocRejectsEmptyOpSlot -v`

Expected: PASS.

- [ ] **Step 3.7: Add the positive-case test**

Append to `modules/world/handler_oploc_test.go`:

```go
// TestHandleOpLocAcceptsPopulatedOpSlot verifies that clicking a
// populated Op slot proceeds through the handler normally. Provides
// positive coverage for the S6k gate.
func TestHandleOpLocAcceptsPopulatedOpSlot(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)
	s.locTypes.Configs[42].Op[0] = "Chop"

	if err := handleOpLoc1(p, p2x3Payload(100, 100, 42)); err != nil {
		t.Fatalf("handleOpLoc1: %v", err)
	}

	if p.target != loc {
		t.Errorf("target: got %v, want loc", p.target)
	}
	if p.targetOp != 1 {
		t.Errorf("targetOp: got %d, want 1", p.targetOp)
	}
}
```

- [ ] **Step 3.8: Run the positive-case test and confirm PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpLocAcceptsPopulatedOpSlot -v`

Expected: PASS.

- [ ] **Step 3.9: Run all OPLOC handler tests to confirm no collateral failures**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpLoc -v`

Expected: all 11 tests PASS (9 pre-existing + 2 new from this task).

- [ ] **Step 3.10: Update `TestTryFireOpTriggerLocNoScript` to expect the defaultOp message**

In `modules/world/interaction_trigger_test.go`, find `TestTryFireOpTriggerLocNoScript`. The current test setup builds a fixture, calls `tryFireOpTrigger(p)`, and asserts `p.target == nil` + `p.interactionFired == true`.

The test needs to (a) drain the player's connection beforehand, and (b) assert a `MessageGame("Nothing interesting happens.")` packet landed. The current `makeOpLocTriggerFixture` does NOT return the connection — check its signature. If it doesn't, you have two options:

**Option A: Extend `makeOpLocTriggerFixture` to return the connection.** Find the helper (it wraps `makeOpLocFixture`) and update it to return the 4th value:

```go
func makeOpLocTriggerFixture(t *testing.T) (*Server, *Player, *entitypkg.Loc, net.Conn) {
	t.Helper()
	s, p, loc, cc := makeOpLocFixture(t)
	p.SetInteraction(InteractionEngine, loc, 1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	return s, p, loc, cc
}
```

Then update existing callers in the same file (anywhere `makeOpLocTriggerFixture(t)` appears) to capture or discard the new 4th return.

**Option B: Drain the connection inline in the test.** If Option A causes too much churn, use `drainConn(t, p.client.conn)` directly in each test that needs to inspect writes. Verify `p.client.conn` is the correct accessor.

Use Option A — it's cleaner for future tests.

Now update `TestTryFireOpTriggerLocNoScript`. The current shape:

```go
func TestTryFireOpTriggerLocNoScript(t *testing.T) {
	_, p, _ := makeOpLocTriggerFixture(t)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after silent clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after no-script clear")
	}
}
```

Becomes:

```go
func TestTryFireOpTriggerLocNoScript(t *testing.T) {
	_, p, _, cc := makeOpLocTriggerFixture(t)

	received := drainConn(t, cc)
	tryFireOpTrigger(p)
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected MessageGame packet for defaultOp, got nothing")
	}
	if p.target != nil {
		t.Errorf("target: got %v, want nil after default-op clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after default-op clear")
	}
	// Assert the message text appears in the drained bytes. The
	// wire format is [opcode][length][text+10]; we check the text
	// substring to stay robust to framing details.
	if !bytesContainString(got, "Nothing interesting happens.") {
		t.Errorf("drained bytes: expected \"Nothing interesting happens.\" substring, got %x", got)
	}
}

// bytesContainString is a test-only helper: returns true if the raw
// wire bytes contain the given string as a contiguous substring.
func bytesContainString(haystack []byte, needle string) bool {
	nb := []byte(needle)
	for i := 0; i+len(nb) <= len(haystack); i++ {
		match := true
		for j := range nb {
			if haystack[i+j] != nb[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
```

(If `bytes.Contains` is already available / imported in this test file, prefer it over the hand-rolled helper.)

- [ ] **Step 3.11: Run the updated test and confirm FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestTryFireOpTriggerLocNoScript -v`

Expected: FAIL — no MessageGame is sent yet. The text-substring assertion fails.

- [ ] **Step 3.12: Add the defaultOp MessageGame call in `fireOpTriggerLoc`**

In `modules/world/interaction_trigger.go`, find `fireOpTriggerLoc`. Locate the "no script found" branch:

```go
sf := srv.scriptProvider.GetByTrigger(trigger, loc.Type(), category)
if sf == nil {
	p.ClearInteraction()
	p.interactionFired = true
	return
}
```

Insert `MessageGame` immediately before the `ClearInteraction`:

```go
sf := srv.scriptProvider.GetByTrigger(trigger, loc.Type(), category)
if sf == nil {
	// S6j-D7 closed in S6k: defaultOp fallback. TS Player.ts:~1095
	// fires "Nothing interesting happens." when the player reaches
	// contact range and no op-trigger is registered for this loc.
	// Message infra was already in place (Player.MessageGame at
	// modules/world/message_game.go); S6j's "needs message infra"
	// concern was spurious.
	p.MessageGame("Nothing interesting happens.")
	p.ClearInteraction()
	p.interactionFired = true
	return
}
```

- [ ] **Step 3.13: Run the updated test and confirm PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestTryFireOpTriggerLocNoScript -v`

Expected: PASS.

- [ ] **Step 3.14: Run all interaction_trigger tests to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestTryFireOpTrigger -v`

Expected: all 10 Npc-branch + 6 Loc-branch tests pass (the other 5 Loc tests don't care about MessageGame bytes because they don't drain the connection).

- [ ] **Step 3.15: Run the full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests pass.

- [ ] **Step 3.16: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no warnings.

- [ ] **Step 3.17: Run the race detector on modules/world**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`

Expected: no races.

- [ ] **Step 3.18: Commit Task 3**

```bash
git add modules/world/handler_oploc.go modules/world/handler_oploc_test.go modules/world/interaction_trigger.go modules/world/interaction_trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): restore OPLOC op-gate + defaultOp (S6k-3)

Closes the two S6j deviations that were waiting on LocType.Op:

1. S6j-D1 — per-op validation gate restored in handleOpLoc.
   Rejects clicks where locType.Op is too short or locType.Op[op-1]
   is "" (the decoder coerces "hidden" to "" at load time, so the
   runtime check is single-condition). Mirrors handler_opnpc.go:38-44.

2. S6j-D7 — defaultOp message in fireOpTriggerLoc. Calls
   Player.MessageGame("Nothing interesting happens.") immediately
   before ClearInteraction on the no-script path. TS Player.ts:~1095.

Fixture update: makeOpLocFixture now populates LocType.Op with
5 non-empty slots so existing TestHandleOpLoc* tests continue passing
under the new gate.

2 new handler tests (empty-slot rejection + populated-slot positive
path) + TestTryFireOpTriggerLocNoScript updated to assert the
defaultOp MessageGame packet.

End-to-end: OPLOC routing is now TS-faithful for populated Ops and
emits the defaultOp message on no-script locs.

Spec: docs/superpowers/specs/2026-04-21-runescript-s6k-loctype-op-design.md
Plan: docs/superpowers/plans/2026-04-21-runescript-s6k-loctype-op.md (Task 3)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Notes (for plan-author use)

**1. Spec coverage:**
- §1 Goal — Tasks 1+2+3 collectively achieve "LocType.Op + loc_op + gate + defaultOp." ✅
- §2 Architecture — Task 1 (objtype), Task 2 (entity + script), Task 3 (world) — all three layers covered. ✅
- §3 File map — every modified/created file appears in task headers. ✅
- §5.1 Field + decoder — Task 1 Steps 1.3-1.5. ✅
- §5.2 ActiveLoc + Loc.LocType — Task 2 Steps 2.1-2.2. ✅
- §5.3 handleLocOp + opcode + dispatch — Task 2 Steps 2.6, 2.10, 2.11. ✅
- §5.4 Handler gate restore — Task 3 Step 3.5. ✅
- §5.5 defaultOp message — Task 3 Step 3.12. ✅
- §6 Test plan — 3 decoder (Task 1) + 5 handleLocOp (Task 2) + 2 handler gate (Task 3) + 1 defaultOp update (Task 3) = 11 new tests. ✅
- §7 Task split — 3 tasks as planned. ✅
- §8 Deviations — S6j-D1 and S6j-D7 close; remaining deviations untouched. ✅

**2. Type consistency:**
- `LocType.Op []string` consistent across decoder, handler gate, and handleLocOp. ✅
- `ActiveLoc.LocType() int` method signature consistent across interface definition, `*entity.Loc` impl, and handler reader. ✅
- `OpLocOp Opcode = 3014` consistent across opcode.go, handlers.go wiring, and handler naming. ✅
- `requireActiveLoc` tag `"LOC_OP"` consistent between handler impl and the error-text assertion in `TestHandleLocOpRequiresActiveLoc`. ✅

**3. Placeholder scan:** No "TBD" / "TODO" / "implement later" / "add validation". The Step 3.10 "Option A vs Option B" branch is explicitly resolved ("Use Option A") — not a placeholder, a documented decision.

**4. Scope:** Three tasks, each independently committable. Build green at every commit. End-to-end `[oploc<n>,<locType>]` with populated Op routes TS-faithfully after Task 3; no-script locs emit defaultOp.
