# S7f — NPC_FIND Family Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the closest-single-NPC cluster of the TS NPC_FIND family — `NPC_FIND` (2513), `NPC_FINDCAT` (2517), and `NPC_FINDEXACT` (2518) — along with a reusable script→world `NpcLookup` bridge and three new validators, to unblock `[proc,set_hint_newbie_basics_instructor]` past `pc=8`.

**Architecture:** New validators (`checkCoord`, `checkNpcType`, `checkHuntVis`, `checkCategoryType`) live next to `checkNotNull` in `pkg/script/handlers_npc.go`. A new `NpcLookup` interface is added to `ScriptState` (parallel to `InvLookup`/`PlayerLookup`), implemented world-side as `serverNpcLookup` doing linear iteration over `s.npcs` with type/coord/distance filters. Each handler validates all inputs, consults the bridge, and writes the found NPC to either `ActiveNpc` (IntOperand=0) or `OtherActiveNpc` (IntOperand=1) via a shared `setActiveNpcSlot` helper.

**Tech Stack:** Go 1.26+. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` for verification. `git commit --no-gpg-sign` for commits. Branch: `main` (project convention — no worktree needed per established S-series workflow). Spec source: `docs/superpowers/specs/2026-04-24-runescript-s7f-npc-find-family-design.md` (commit `63219b0`).

---

## Task 1: pkg/script foundation — validators + NpcLookup bridge + mock

**Files:**
- Modify: `pkg/script/handlers_npc.go` (add 4 validators + `setActiveNpcSlot` helper at top of file)
- Modify: `pkg/script/state.go` (add `NpcLookup` interface + `Npcs NpcLookup` field on `ScriptState`)
- Modify: `pkg/script/runner_test.go` (add `mockNpcLookup` type)
- Modify: `pkg/script/handlers_npc_test.go` (add validator unit tests)

### Step 1.1: Write failing validator unit tests

- [ ] **Step 1.1.1: Write failing validator unit tests**

Add to `pkg/script/handlers_npc_test.go` (place at top of file, after package/import):

```go
// --- S7f: validator unit tests -----------------------------------------

func TestCheckCoord(t *testing.T) {
    cases := []struct {
        name    string
        in      int
        wantErr bool
        wantL   int
        wantX   int
        wantZ   int
    }{
        {"zero", 0, false, 0, 0, 0},
        {"valid packed", (2 << 28) | (3200 << 14) | 3300, false, 2, 3200, 3300},
        {"max valid", 2147483647, false, 0, 0, 0x3fff},
        {"negative", -1, true, 0, 0, 0},
        {"beyond max", -2147483648, true, 0, 0, 0}, // int overflow wrap
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            level, x, z, err := checkCoord(tc.in, "TEST")
            if tc.wantErr {
                if err == nil {
                    t.Fatalf("checkCoord(%d): want error, got nil", tc.in)
                }
                if !strings.Contains(err.Error(), "TEST:") {
                    t.Errorf("error should carry op prefix: %v", err)
                }
                return
            }
            if err != nil {
                t.Fatalf("checkCoord(%d): unexpected error: %v", tc.in, err)
            }
            if level != tc.wantL || x != tc.wantX || z != tc.wantZ {
                t.Errorf("checkCoord(%d) = (%d, %d, %d), want (%d, %d, %d)",
                    tc.in, level, x, z, tc.wantL, tc.wantX, tc.wantZ)
            }
        })
    }
}

func TestCheckNpcType(t *testing.T) {
    // Build a minimal ScriptState with a Configs that reports NpcType 7 as present.
    s := &ScriptState{Configs: newTestConfigsWithNpcTypes(map[int]bool{7: true})}

    if err := checkNpcType(s, 7, "TEST"); err != nil {
        t.Errorf("checkNpcType(7) with loaded type: unexpected error %v", err)
    }
    if err := checkNpcType(s, 8, "TEST"); err == nil {
        t.Errorf("checkNpcType(8) with unloaded type: want error")
    } else if !strings.Contains(err.Error(), "TEST:") || !strings.Contains(err.Error(), "8") {
        t.Errorf("error should carry op prefix and offending id: %v", err)
    }
    if err := checkNpcType(s, -1, "TEST"); err == nil {
        t.Errorf("checkNpcType(-1): want error")
    }

    // Nil Configs: always errors.
    s2 := &ScriptState{}
    if err := checkNpcType(s2, 7, "TEST"); err == nil {
        t.Errorf("checkNpcType with nil Configs: want error")
    }
}

func TestCheckHuntVis(t *testing.T) {
    for _, v := range []int{0, 1, 2} {
        if err := checkHuntVis(v, "TEST"); err != nil {
            t.Errorf("checkHuntVis(%d): unexpected error %v", v, err)
        }
    }
    for _, v := range []int{-1, 3, 99} {
        if err := checkHuntVis(v, "TEST"); err == nil {
            t.Errorf("checkHuntVis(%d): want error", v)
        }
    }
}

func TestCheckCategoryType(t *testing.T) {
    // Partial validator: only -1 rejected (S7f-D3).
    if err := checkCategoryType(-1, "TEST"); err == nil {
        t.Errorf("checkCategoryType(-1): want error (null sentinel)")
    }
    for _, v := range []int{0, 1, 100, 999999} {
        if err := checkCategoryType(v, "TEST"); err != nil {
            t.Errorf("checkCategoryType(%d): partial validator should accept; got %v", v, err)
        }
    }
}
```

Ensure `strings` is in the imports and that `newTestConfigsWithNpcTypes` exists in the test file (next step adds it if missing).

- [ ] **Step 1.1.2: Add `newTestConfigsWithNpcTypes` test helper if absent**

Grep `newTestConfigsWithNpcTypes` in `pkg/script/`. If missing, add to `pkg/script/handlers_npc_test.go` (or the nearest existing test-helpers location):

```go
// testConfigsNpc is a minimal Configs impl for NpcType lookup tests.
type testConfigsNpc struct {
    present map[int]bool
}

func (c *testConfigsNpc) NpcType(id int) *objtype.NpcType {
    if c == nil || !c.present[id] {
        return nil
    }
    return &objtype.NpcType{ID: id}
}

// Delegate everything else to returning nil — tests don't exercise other configs.
func (c *testConfigsNpc) InvType(int) *objtype.InvType   { return nil }
func (c *testConfigsNpc) LocType(int) *objtype.LocType   { return nil }
func (c *testConfigsNpc) ObjType(int) *objtype.ObjType   { return nil }
func (c *testConfigsNpc) SeqType(int) *objtype.SeqType   { return nil }
func (c *testConfigsNpc) EnumType(int) *objtype.EnumType { return nil }
func (c *testConfigsNpc) StructType(int) *objtype.StructType { return nil }
func (c *testConfigsNpc) VarPlayerType(int) *objtype.VarPlayerType { return nil }
func (c *testConfigsNpc) ParamType(int) *objtype.ParamType { return nil }
func (c *testConfigsNpc) DbTableType(int) *objtype.DbTableType { return nil }
func (c *testConfigsNpc) DbRowType(int) *objtype.DbRowType { return nil }

func newTestConfigsWithNpcTypes(present map[int]bool) Configs {
    return &testConfigsNpc{present: present}
}
```

**Before writing this**, grep `pkg/script/` for an existing `testConfigs*` type (likely exists for S7c's `checkInvType`). If one exists, add an `NpcType` entry to its map and skip this helper. The minimal interface above must match `pkg/script/configs.go`'s `Configs` surface — re-read `configs.go` and mirror exactly. If the surface has more methods than listed here, add no-op returns for each.

- [ ] **Step 1.1.3: Run validator tests, expect compile failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestCheck -count=1
```

Expected: FAIL with "undefined: checkCoord" / "undefined: checkHuntVis" / "undefined: checkCategoryType" (and possibly "undefined: checkNpcType" if not previously defined).

### Step 1.2: Implement validators

- [ ] **Step 1.2.1: Add validators to handlers_npc.go**

Insert at the top of `pkg/script/handlers_npc.go` (after package and imports, before the first existing function). Import `"fmt"` if not already imported:

```go
// checkCoord mirrors TS CoordValid (ScriptValidators.ts:109) — validates
// the packed int is in [0, 2147483647] and unpacks to (level, x, z).
// Uses the package-local unpackCoord helper at handlers_player.go:18.
func checkCoord(v int, op string) (level, x, z int, err error) {
    if v < 0 || v > 2147483647 {
        return 0, 0, 0, fmt.Errorf("%s: coord out of range (%d)", op, v)
    }
    level, x, z = unpackCoord(v)
    return
}

// checkNpcType mirrors TS NpcTypeValid (ScriptValidators.ts:111) — range
// + registry presence check, collapsed into a single Configs.NpcType(id)
// nil check per the S7c checkInvType pattern at handlers_player.go:75.
func checkNpcType(s *ScriptState, id int, op string) error {
    if s.Configs == nil || s.Configs.NpcType(id) == nil {
        return fmt.Errorf("%s: no NpcType with value (%d) found", op, id)
    }
    return nil
}

// checkHuntVis mirrors TS HuntVisValid (ScriptValidators.ts:125) — range
// [HuntVisOff=0, HuntVisLineOfWalk=2]. Constants live in
// pkg/objtype/hunttype.go:22-26 and match TS values.
func checkHuntVis(v int, op string) error {
    if v < 0 || v > 2 {
        return fmt.Errorf("%s: huntvis out of range (%d)", op, v)
    }
    return nil
}

// checkCategoryType partially mirrors TS CategoryTypeValid
// (ScriptValidators.ts:123). Goscape has no CategoryType config loader,
// so the count-bound check is absent — only null-sentinel rejection
// survives. Deviation S7f-D3. Follow-up: count-bound check when the
// CategoryType loader lands.
func checkCategoryType(v int, op string) error {
    if v == -1 {
        return fmt.Errorf("%s: category null(-1)", op)
    }
    return nil
}
```

- [ ] **Step 1.2.2: Run validator tests, expect pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestCheck -count=1 -v
```

Expected: `--- PASS: TestCheckCoord`, `--- PASS: TestCheckNpcType`, `--- PASS: TestCheckHuntVis`, `--- PASS: TestCheckCategoryType`.

### Step 1.3: Implement `setActiveNpcSlot` helper

- [ ] **Step 1.3.1: Write failing test**

Add to `pkg/script/handlers_npc_test.go`:

```go
func TestSetActiveNpcSlot_OperandZero(t *testing.T) {
    s := &ScriptState{
        Script: &ScriptFile{IntOperands: []int32{0}},
        PC:     0,
    }
    npc := &mockNpc{typeID: 42}
    setActiveNpcSlot(s, npc)
    if s.ActiveNpc != npc {
        t.Errorf("ActiveNpc: got %v, want %v", s.ActiveNpc, npc)
    }
    if s.OtherActiveNpc != nil {
        t.Errorf("OtherActiveNpc: got %v, want nil", s.OtherActiveNpc)
    }
    if s.Pointers&PtrActiveNpc == 0 {
        t.Error("PtrActiveNpc should be set")
    }
    if s.Pointers&PtrActiveNpc2 != 0 {
        t.Error("PtrActiveNpc2 should NOT be set")
    }
}

func TestSetActiveNpcSlot_OperandOne(t *testing.T) {
    s := &ScriptState{
        Script: &ScriptFile{IntOperands: []int32{1}},
        PC:     0,
    }
    npc := &mockNpc{typeID: 42}
    setActiveNpcSlot(s, npc)
    if s.OtherActiveNpc != npc {
        t.Errorf("OtherActiveNpc: got %v, want %v", s.OtherActiveNpc, npc)
    }
    if s.ActiveNpc != nil {
        t.Errorf("ActiveNpc: got %v, want nil", s.ActiveNpc)
    }
    if s.Pointers&PtrActiveNpc2 == 0 {
        t.Error("PtrActiveNpc2 should be set")
    }
    if s.Pointers&PtrActiveNpc != 0 {
        t.Error("PtrActiveNpc should NOT be set")
    }
}

func TestSetActiveNpcSlot_InvalidOperand(t *testing.T) {
    s := &ScriptState{
        Script: &ScriptFile{IntOperands: []int32{2}},
        PC:     0,
    }
    defer func() {
        if r := recover(); r == nil {
            t.Error("setActiveNpcSlot with operand=2 should panic")
        }
    }()
    setActiveNpcSlot(s, &mockNpc{typeID: 42})
}
```

Requires `mockNpc` type to exist. Grep `type mockNpc struct` in `pkg/script/`. If it exists, reuse. If not, the existing handler tests likely rely on an `ActiveNpc` mock — find it by grepping `implements ActiveNpc` or `NpcType() int` in `_test.go` files.

- [ ] **Step 1.3.2: Run, expect compile failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestSetActiveNpcSlot -count=1
```

Expected: FAIL with "undefined: setActiveNpcSlot".

- [ ] **Step 1.3.3: Add helper to handlers_npc.go**

Insert after the validator block:

```go
// setActiveNpcSlot writes the found NPC to either ActiveNpc (primary) or
// OtherActiveNpc (secondary) based on the handler's IntOperand and sets
// the corresponding Pointer flag. Mirrors TS
// state.pointerAdd(ActiveNpc[state.intOperand]) at NpcOps.ts:365, 398, 105.
// IntOperand==0 → ActiveNpc/PtrActiveNpc (.npc syntax).
// IntOperand==1 → OtherActiveNpc/PtrActiveNpc2 (.npc2 syntax).
// Any other value panics (compiler invariant — bytecode only emits 0/1).
func setActiveNpcSlot(s *ScriptState, npc ActiveNpc) {
    operand := s.Script.IntOperands[s.PC]
    switch operand {
    case 0:
        s.ActiveNpc = npc
        s.Pointers |= PtrActiveNpc
    case 1:
        s.OtherActiveNpc = npc
        s.Pointers |= PtrActiveNpc2
    default:
        panic(fmt.Sprintf("setActiveNpcSlot: invalid IntOperand %d", operand))
    }
}
```

- [ ] **Step 1.3.4: Run slot helper tests, expect pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestSetActiveNpcSlot -count=1 -v
```

Expected: all three sub-tests PASS.

### Step 1.4: Add `NpcLookup` interface + `Npcs` field to `ScriptState`

- [ ] **Step 1.4.1: Write failing test that references `state.Npcs`**

Add to `pkg/script/handlers_npc_test.go`:

```go
// TestNpcLookupInterfaceShape is a compile-time assertion that the
// NpcLookup interface has the three expected methods. If this test
// compiles, the interface is correctly defined.
func TestNpcLookupInterfaceShape(t *testing.T) {
    var _ NpcLookup = (*mockNpcLookup)(nil)
    s := &ScriptState{}
    s.Npcs = &mockNpcLookup{}
    _ = s.Npcs
}
```

- [ ] **Step 1.4.2: Run, expect compile failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcLookupInterfaceShape -count=1
```

Expected: FAIL with "undefined: NpcLookup" and "undefined field Npcs on ScriptState" and "undefined: mockNpcLookup".

- [ ] **Step 1.4.3: Add `NpcLookup` interface to state.go**

Insert in `pkg/script/state.go` immediately after the `InvLookup` interface (around line 56, after the closing brace of `InvLookup`):

```go
// NpcLookup is the script→world bridge for NPC_FIND family opcodes. All
// methods return the matching NPC as script.ActiveNpc or nil when no
// match. Implementations iterate the world NPC registry; see
// serverNpcLookup (modules/world/npc_script_lookup.go) for the
// production impl.
//
// huntvis accepts HuntVisOff/LineOfSight/LineOfWalk (pkg/objtype.HuntVis*)
// but the current impl does not filter on it (deviation S7f-D1).
// Callers must still validate via checkHuntVis.
type NpcLookup interface {
    // FindClosestNpcByType: NPC_FIND semantics. Square-bounded by dist
    // from (level, x, z); filter by typeID; closest by euclidean-squared
    // with later-match-wins on ties.
    FindClosestNpcByType(level, x, z, dist, typeID, huntvis int) ActiveNpc

    // FindClosestNpcByCategory: NPC_FINDCAT semantics. Same shape as
    // FindClosestNpcByType but filter via NpcType.Category == cat.
    FindClosestNpcByCategory(level, x, z, dist, cat, huntvis int) ActiveNpc

    // FindNpcAtExactCoord: NPC_FINDEXACT semantics. Returns the first
    // NPC at exactly (level, x, z) whose type matches typeID, or nil.
    FindNpcAtExactCoord(level, x, z, typeID int) ActiveNpc
}
```

- [ ] **Step 1.4.4: Add `Npcs` field to `ScriptState`**

Find the existing `PlayerLookup PlayerLookup` field declaration on `ScriptState` (around line 84). Add immediately after it (following its doc-comment pattern):

```go
    // Npcs is the NPC-lookup surface for NPC_FIND family opcodes.
    // Callers set this after Init if the script uses find opcodes.
    // Nil disables (handlers treat a nil surface as "no match", push 0).
    Npcs NpcLookup
```

- [ ] **Step 1.4.5: Add `mockNpcLookup` to runner_test.go**

Insert in `pkg/script/runner_test.go` after the existing `mockPlayer` and associated mocks:

```go
// mockNpcLookup is a test double for script.NpcLookup. Tests set the
// per-method return fields and assert call-capture afterwards. Mirrors
// the mockPlayer "value + counter" pattern (runner_test.go:224-228,
// S7e precedent). lastArgs captures the most recent call's args as an
// []int so handler tests can cross-check arg ordering.
type mockNpcLookup struct {
    byType     ActiveNpc
    byCategory ActiveNpc
    atCoord    ActiveNpc

    byTypeCalls     int
    byCategoryCalls int
    atCoordCalls    int

    lastArgs []int
}

func (m *mockNpcLookup) FindClosestNpcByType(level, x, z, dist, typeID, huntvis int) ActiveNpc {
    m.byTypeCalls++
    m.lastArgs = []int{level, x, z, dist, typeID, huntvis}
    return m.byType
}

func (m *mockNpcLookup) FindClosestNpcByCategory(level, x, z, dist, cat, huntvis int) ActiveNpc {
    m.byCategoryCalls++
    m.lastArgs = []int{level, x, z, dist, cat, huntvis}
    return m.byCategory
}

func (m *mockNpcLookup) FindNpcAtExactCoord(level, x, z, typeID int) ActiveNpc {
    m.atCoordCalls++
    m.lastArgs = []int{level, x, z, typeID}
    return m.atCoord
}
```

- [ ] **Step 1.4.6: Run NpcLookup shape test, expect pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcLookupInterfaceShape -count=1 -v
```

Expected: PASS.

### Step 1.5: Full Task 1 verification and commit

- [ ] **Step 1.5.1: Run all pkg/script tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1
```

Expected: `ok github.com/zsrv/goscape/pkg/script`. All pre-existing tests must still pass.

- [ ] **Step 1.5.2: Run go vet**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/script/
```

Expected: no output.

- [ ] **Step 1.5.3: Verify no modules/world regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: no output (clean build across all packages — the new interface + field must not break any existing wire-up).

- [ ] **Step 1.5.4: Commit Task 1**

```bash
git add pkg/script/handlers_npc.go pkg/script/state.go pkg/script/runner_test.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7f Task 1 — validators + NpcLookup interface + mock

Four new validators (checkCoord / checkNpcType / checkHuntVis /
checkCategoryType), setActiveNpcSlot pointer helper, NpcLookup
interface on ScriptState, and mockNpcLookup for handler tests.
Handlers land in Task 2. Three deviations tracked per spec §7 —
checkCategoryType is a partial port (S7f-D3).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 1.5.5: Report to controller**

Report:
- Status: DONE
- Commit SHA
- Test counts (validator tests + slot helper tests + interface shape test)
- Files changed with LOC deltas
- Any divergences from spec §3.1 / §3.2 / §3.5 / §5.1

---

## Task 2: All three handlers + handler tests + registry

**Files:**
- Modify: `pkg/script/handlers_npc.go` (add 3 handler functions)
- Modify: `pkg/script/handlers.go` (register 3 opcodes under new section)
- Modify: `pkg/script/handlers_npc_test.go` (add 17 handler test cases)

### Step 2.1: Write failing tests for NPC_FIND

- [ ] **Step 2.1.1: Add `TestNpcFind_*` tests**

Append to `pkg/script/handlers_npc_test.go`:

```go
// --- S7f Task 2: NPC_FIND handler tests --------------------------------

// newNpcFindState constructs a ScriptState with IntOperands[PC]=operand,
// pushes (coord, npcTypeID, distance, huntvis) onto the int stack in
// the order the handler expects, wires a Configs that treats every id
// in loaded as a valid NpcType, and binds a mockNpcLookup as state.Npcs.
func newNpcFindState(t *testing.T, operand int32, coord, npcTypeID, distance, huntvis int, loaded map[int]bool, lookup *mockNpcLookup) *ScriptState {
    t.Helper()
    s := &ScriptState{
        Script:   &ScriptFile{IntOperands: []int32{operand}},
        PC:       0,
        Configs:  newTestConfigsWithNpcTypes(loaded),
        Npcs:     lookup,
        Pointers: 0,
    }
    // Push in the order: coord, npcType, distance, huntvis (matches
    // TS popInts(4) = [coord, npc, distance, checkVis] where coord
    // was pushed first).
    s.PushInt(coord)
    s.PushInt(npcTypeID)
    s.PushInt(distance)
    s.PushInt(huntvis)
    return s
}

func TestNpcFind_SingleMatch(t *testing.T) {
    foundNpc := &mockNpc{typeID: 7}
    lookup := &mockNpcLookup{byType: foundNpc}
    coord := (2 << 28) | (3200 << 14) | 3300
    s := newNpcFindState(t, 0, coord, 7, 10, 0, map[int]bool{7: true}, lookup)

    if err := handleNpcFind(s); err != nil {
        t.Fatalf("handleNpcFind: %v", err)
    }
    if got := s.PopInt(); got != 1 {
        t.Errorf("push: got %d, want 1", got)
    }
    if s.ActiveNpc != foundNpc {
        t.Errorf("ActiveNpc: got %v, want %v", s.ActiveNpc, foundNpc)
    }
    if s.Pointers&PtrActiveNpc == 0 {
        t.Error("PtrActiveNpc should be set")
    }
    if lookup.byTypeCalls != 1 {
        t.Errorf("byTypeCalls: got %d, want 1", lookup.byTypeCalls)
    }
    // Cross-check: handler passed (level, x, z, dist, typeID, huntvis).
    wantArgs := []int{2, 3200, 3300, 10, 7, 0}
    if !intSliceEqual(lookup.lastArgs, wantArgs) {
        t.Errorf("lastArgs: got %v, want %v", lookup.lastArgs, wantArgs)
    }
}

func TestNpcFind_NoMatch(t *testing.T) {
    lookup := &mockNpcLookup{byType: nil} // no match
    coord := (0 << 28) | (50 << 14) | 50
    s := newNpcFindState(t, 0, coord, 7, 10, 0, map[int]bool{7: true}, lookup)

    if err := handleNpcFind(s); err != nil {
        t.Fatalf("handleNpcFind: %v", err)
    }
    if got := s.PopInt(); got != 0 {
        t.Errorf("push: got %d, want 0", got)
    }
    if s.ActiveNpc != nil {
        t.Errorf("ActiveNpc should be nil, got %v", s.ActiveNpc)
    }
    if s.Pointers&PtrActiveNpc != 0 {
        t.Error("PtrActiveNpc should NOT be set on miss")
    }
}

func TestNpcFind_NilNpcLookup(t *testing.T) {
    coord := (0 << 28) | (50 << 14) | 50
    s := newNpcFindState(t, 0, coord, 7, 10, 0, map[int]bool{7: true}, nil)
    s.Npcs = nil // explicit

    if err := handleNpcFind(s); err != nil {
        t.Fatalf("handleNpcFind with nil Npcs: %v", err)
    }
    if got := s.PopInt(); got != 0 {
        t.Errorf("nil Npcs should degrade to not-found (push 0); got %d", got)
    }
    if s.Pointers&PtrActiveNpc != 0 {
        t.Error("PtrActiveNpc should NOT be set when Npcs is nil")
    }
}

func TestNpcFind_IntOperandZero(t *testing.T) {
    foundNpc := &mockNpc{typeID: 7}
    lookup := &mockNpcLookup{byType: foundNpc}
    s := newNpcFindState(t, 0, 0, 7, 10, 0, map[int]bool{7: true}, lookup)

    if err := handleNpcFind(s); err != nil {
        t.Fatal(err)
    }
    if s.ActiveNpc != foundNpc {
        t.Errorf("operand=0 should set ActiveNpc, got %v", s.ActiveNpc)
    }
    if s.OtherActiveNpc != nil {
        t.Errorf("operand=0 should leave OtherActiveNpc nil, got %v", s.OtherActiveNpc)
    }
}

func TestNpcFind_IntOperandOne(t *testing.T) {
    foundNpc := &mockNpc{typeID: 7}
    lookup := &mockNpcLookup{byType: foundNpc}
    s := newNpcFindState(t, 1, 0, 7, 10, 0, map[int]bool{7: true}, lookup)

    if err := handleNpcFind(s); err != nil {
        t.Fatal(err)
    }
    if s.OtherActiveNpc != foundNpc {
        t.Errorf("operand=1 should set OtherActiveNpc, got %v", s.OtherActiveNpc)
    }
    if s.ActiveNpc != nil {
        t.Errorf("operand=1 should leave ActiveNpc nil, got %v", s.ActiveNpc)
    }
    if s.Pointers&PtrActiveNpc2 == 0 {
        t.Error("operand=1 should set PtrActiveNpc2")
    }
    if s.Pointers&PtrActiveNpc != 0 {
        t.Error("operand=1 should NOT set PtrActiveNpc")
    }
}

func TestNpcFind_InvalidCoord(t *testing.T) {
    lookup := &mockNpcLookup{}
    s := newNpcFindState(t, 0, -1, 7, 10, 0, map[int]bool{7: true}, lookup)
    if err := handleNpcFind(s); err == nil {
        t.Fatal("expected error for coord=-1")
    } else if !strings.Contains(err.Error(), "NPC_FIND: coord out of range") {
        t.Errorf("wrong error: %v", err)
    }
    if lookup.byTypeCalls != 0 {
        t.Errorf("lookup should NOT be called on validator failure; calls=%d", lookup.byTypeCalls)
    }
}

func TestNpcFind_InvalidNpcType(t *testing.T) {
    lookup := &mockNpcLookup{}
    s := newNpcFindState(t, 0, 0, 999, 10, 0, map[int]bool{7: true}, lookup) // 999 not loaded
    if err := handleNpcFind(s); err == nil {
        t.Fatal("expected error for unloaded npcType")
    } else if !strings.Contains(err.Error(), "NPC_FIND: no NpcType") {
        t.Errorf("wrong error: %v", err)
    }
    if lookup.byTypeCalls != 0 {
        t.Errorf("lookup should NOT be called; calls=%d", lookup.byTypeCalls)
    }
}

func TestNpcFind_NullDistance(t *testing.T) {
    lookup := &mockNpcLookup{}
    s := newNpcFindState(t, 0, 0, 7, -1, 0, map[int]bool{7: true}, lookup)
    if err := handleNpcFind(s); err == nil {
        t.Fatal("expected error for distance=-1 (NumberNotNull)")
    } else if !strings.Contains(err.Error(), "NPC_FIND") {
        t.Errorf("error should carry op prefix: %v", err)
    }
    if lookup.byTypeCalls != 0 {
        t.Errorf("lookup should NOT be called; calls=%d", lookup.byTypeCalls)
    }
}

func TestNpcFind_InvalidHuntVis(t *testing.T) {
    lookup := &mockNpcLookup{}
    s := newNpcFindState(t, 0, 0, 7, 10, 3, map[int]bool{7: true}, lookup) // 3 out of range
    if err := handleNpcFind(s); err == nil {
        t.Fatal("expected error for huntvis=3")
    } else if !strings.Contains(err.Error(), "NPC_FIND: huntvis out of range") {
        t.Errorf("wrong error: %v", err)
    }
    if lookup.byTypeCalls != 0 {
        t.Errorf("lookup should NOT be called; calls=%d", lookup.byTypeCalls)
    }
}

// intSliceEqual is a test helper for comparing []int.
func intSliceEqual(a, b []int) bool {
    if len(a) != len(b) {
        return false
    }
    for i := range a {
        if a[i] != b[i] {
            return false
        }
    }
    return true
}
```

- [ ] **Step 2.1.2: Run, expect compile failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcFind_ -count=1
```

Expected: FAIL with "undefined: handleNpcFind".

### Step 2.2: Implement handleNpcFind

- [ ] **Step 2.2.1: Add `handleNpcFind` to handlers_npc.go**

Insert after `setActiveNpcSlot`:

```go
// handleNpcFind (NPC_FIND, opcode 2513) pops (coord, npc, distance,
// huntvis), validates each, asks NpcLookup for the closest NPC of that
// type within square-bounded distance, and either sets the active NPC
// slot + pushes 1 or pushes 0. Mirrors TS NpcOps.ts:336-367. Gate:
// none (ActivePlayer-agnostic — the opcode only depends on the world).
// Pointer-set is conditional on hit (TS ScriptOpcodePointers.ts:579).
func handleNpcFind(s *ScriptState) error {
    checkVis := s.PopInt()
    distance := s.PopInt()
    npcTypeID := s.PopInt()
    coord := s.PopInt()

    level, x, z, err := checkCoord(coord, "NPC_FIND")
    if err != nil {
        return err
    }
    if err := checkNpcType(s, npcTypeID, "NPC_FIND"); err != nil {
        return err
    }
    if err := checkNotNull(distance, "NPC_FIND"); err != nil {
        return err
    }
    if err := checkHuntVis(checkVis, "NPC_FIND"); err != nil {
        return err
    }

    var npc ActiveNpc
    if s.Npcs != nil {
        npc = s.Npcs.FindClosestNpcByType(level, x, z, distance, npcTypeID, checkVis)
    }
    if npc == nil {
        s.PushInt(0)
        return nil
    }
    setActiveNpcSlot(s, npc)
    s.PushInt(1)
    return nil
}
```

- [ ] **Step 2.2.2: Run NPC_FIND tests, expect pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcFind_ -count=1 -v
```

Expected: all 9 sub-tests PASS.

### Step 2.3: Write failing tests for NPC_FINDCAT

- [ ] **Step 2.3.1: Add `TestNpcFindCat_*` tests**

Append:

```go
// --- S7f Task 2: NPC_FINDCAT handler tests -----------------------------

// newNpcFindCatState is the NPC_FINDCAT analogue of newNpcFindState.
// Pushes (coord, category, distance, huntvis). Loaded is the NpcType map
// — NPC_FINDCAT does NOT validate NpcType (it validates CategoryType)
// but the ScriptState still needs a Configs field.
func newNpcFindCatState(t *testing.T, operand int32, coord, category, distance, huntvis int, loaded map[int]bool, lookup *mockNpcLookup) *ScriptState {
    t.Helper()
    s := &ScriptState{
        Script:   &ScriptFile{IntOperands: []int32{operand}},
        PC:       0,
        Configs:  newTestConfigsWithNpcTypes(loaded),
        Npcs:     lookup,
        Pointers: 0,
    }
    s.PushInt(coord)
    s.PushInt(category)
    s.PushInt(distance)
    s.PushInt(huntvis)
    return s
}

func TestNpcFindCat_SingleMatch(t *testing.T) {
    foundNpc := &mockNpc{typeID: 12}
    lookup := &mockNpcLookup{byCategory: foundNpc}
    coord := (1 << 28) | (1000 << 14) | 1000
    s := newNpcFindCatState(t, 0, coord, 5, 15, 1, nil, lookup)

    if err := handleNpcFindCat(s); err != nil {
        t.Fatalf("handleNpcFindCat: %v", err)
    }
    if got := s.PopInt(); got != 1 {
        t.Errorf("push: got %d, want 1", got)
    }
    if s.ActiveNpc != foundNpc {
        t.Error("ActiveNpc should be the found NPC")
    }
    if lookup.byCategoryCalls != 1 {
        t.Errorf("byCategoryCalls: got %d, want 1", lookup.byCategoryCalls)
    }
    wantArgs := []int{1, 1000, 1000, 15, 5, 1} // level, x, z, dist, cat, huntvis
    if !intSliceEqual(lookup.lastArgs, wantArgs) {
        t.Errorf("lastArgs: got %v, want %v", lookup.lastArgs, wantArgs)
    }
}

func TestNpcFindCat_NoMatch(t *testing.T) {
    lookup := &mockNpcLookup{byCategory: nil}
    s := newNpcFindCatState(t, 0, 0, 5, 10, 0, nil, lookup)

    if err := handleNpcFindCat(s); err != nil {
        t.Fatal(err)
    }
    if got := s.PopInt(); got != 0 {
        t.Errorf("push: got %d, want 0", got)
    }
}

func TestNpcFindCat_NullCategory(t *testing.T) {
    lookup := &mockNpcLookup{}
    s := newNpcFindCatState(t, 0, 0, -1, 10, 0, nil, lookup)

    if err := handleNpcFindCat(s); err == nil {
        t.Fatal("expected error for category=-1")
    } else if !strings.Contains(err.Error(), "NPC_FINDCAT: category null(-1)") {
        t.Errorf("wrong error: %v", err)
    }
    if lookup.byCategoryCalls != 0 {
        t.Errorf("lookup should NOT be called; calls=%d", lookup.byCategoryCalls)
    }
}

// TestNpcFindCat_PartialValidatorAcceptsNonNegative pins S7f-D3:
// checkCategoryType accepts any non-(-1) value even if no CategoryType
// count is loaded. The handler MUST call the lookup with the raw cat.
func TestNpcFindCat_PartialValidatorAcceptsNonNegative(t *testing.T) {
    foundNpc := &mockNpc{typeID: 12}
    lookup := &mockNpcLookup{byCategory: foundNpc}
    s := newNpcFindCatState(t, 0, 0, 999999, 10, 0, nil, lookup)

    if err := handleNpcFindCat(s); err != nil {
        t.Fatalf("partial validator should accept 999999 (S7f-D3): %v", err)
    }
    if lookup.byCategoryCalls != 1 {
        t.Errorf("byCategoryCalls: got %d, want 1", lookup.byCategoryCalls)
    }
}
```

- [ ] **Step 2.3.2: Run, expect compile failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcFindCat_ -count=1
```

Expected: FAIL with "undefined: handleNpcFindCat".

### Step 2.4: Implement handleNpcFindCat

- [ ] **Step 2.4.1: Add `handleNpcFindCat` to handlers_npc.go**

Insert after `handleNpcFind`:

```go
// handleNpcFindCat (NPC_FINDCAT, opcode 2517) pops (coord, category,
// distance, huntvis). Same spine as handleNpcFind but filter is by
// NpcType.Category == category (handled in the world-side impl).
// checkCategoryType is partial (S7f-D3). Mirrors TS NpcOps.ts:369-400.
func handleNpcFindCat(s *ScriptState) error {
    checkVis := s.PopInt()
    distance := s.PopInt()
    category := s.PopInt()
    coord := s.PopInt()

    level, x, z, err := checkCoord(coord, "NPC_FINDCAT")
    if err != nil {
        return err
    }
    if err := checkCategoryType(category, "NPC_FINDCAT"); err != nil {
        return err
    }
    if err := checkNotNull(distance, "NPC_FINDCAT"); err != nil {
        return err
    }
    if err := checkHuntVis(checkVis, "NPC_FINDCAT"); err != nil {
        return err
    }

    var npc ActiveNpc
    if s.Npcs != nil {
        npc = s.Npcs.FindClosestNpcByCategory(level, x, z, distance, category, checkVis)
    }
    if npc == nil {
        s.PushInt(0)
        return nil
    }
    setActiveNpcSlot(s, npc)
    s.PushInt(1)
    return nil
}
```

- [ ] **Step 2.4.2: Run NPC_FINDCAT tests, expect pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcFindCat_ -count=1 -v
```

Expected: all 4 sub-tests PASS.

### Step 2.5: Write failing tests for NPC_FINDEXACT

- [ ] **Step 2.5.1: Add `TestNpcFindExact_*` tests**

Append:

```go
// --- S7f Task 2: NPC_FINDEXACT handler tests ---------------------------

// newNpcFindExactState pushes (coord, npcTypeID) — only 2 args.
func newNpcFindExactState(t *testing.T, operand int32, coord, npcTypeID int, loaded map[int]bool, lookup *mockNpcLookup) *ScriptState {
    t.Helper()
    s := &ScriptState{
        Script:   &ScriptFile{IntOperands: []int32{operand}},
        PC:       0,
        Configs:  newTestConfigsWithNpcTypes(loaded),
        Npcs:     lookup,
        Pointers: 0,
    }
    s.PushInt(coord)
    s.PushInt(npcTypeID)
    return s
}

func TestNpcFindExact_Match(t *testing.T) {
    foundNpc := &mockNpc{typeID: 7}
    lookup := &mockNpcLookup{atCoord: foundNpc}
    coord := (0 << 28) | (3200 << 14) | 3300
    s := newNpcFindExactState(t, 0, coord, 7, map[int]bool{7: true}, lookup)

    if err := handleNpcFindExact(s); err != nil {
        t.Fatalf("handleNpcFindExact: %v", err)
    }
    if got := s.PopInt(); got != 1 {
        t.Errorf("push: got %d, want 1", got)
    }
    if s.ActiveNpc != foundNpc {
        t.Error("ActiveNpc should be the found NPC")
    }
    wantArgs := []int{0, 3200, 3300, 7}
    if !intSliceEqual(lookup.lastArgs, wantArgs) {
        t.Errorf("lastArgs: got %v, want %v", lookup.lastArgs, wantArgs)
    }
}

func TestNpcFindExact_NoNpcAtCoord(t *testing.T) {
    lookup := &mockNpcLookup{atCoord: nil}
    s := newNpcFindExactState(t, 0, 0, 7, map[int]bool{7: true}, lookup)

    if err := handleNpcFindExact(s); err != nil {
        t.Fatal(err)
    }
    if got := s.PopInt(); got != 0 {
        t.Errorf("push: got %d, want 0", got)
    }
}

func TestNpcFindExact_InvalidCoord(t *testing.T) {
    lookup := &mockNpcLookup{}
    s := newNpcFindExactState(t, 0, -1, 7, map[int]bool{7: true}, lookup)

    if err := handleNpcFindExact(s); err == nil {
        t.Fatal("expected error for coord=-1")
    } else if !strings.Contains(err.Error(), "NPC_FINDEXACT: coord out of range") {
        t.Errorf("wrong error: %v", err)
    }
    if lookup.atCoordCalls != 0 {
        t.Errorf("lookup should NOT be called; calls=%d", lookup.atCoordCalls)
    }
}

func TestNpcFindExact_InvalidNpcType(t *testing.T) {
    lookup := &mockNpcLookup{}
    s := newNpcFindExactState(t, 0, 0, 999, map[int]bool{7: true}, lookup)

    if err := handleNpcFindExact(s); err == nil {
        t.Fatal("expected error for unloaded npcType")
    } else if !strings.Contains(err.Error(), "NPC_FINDEXACT: no NpcType") {
        t.Errorf("wrong error: %v", err)
    }
    if lookup.atCoordCalls != 0 {
        t.Errorf("lookup should NOT be called; calls=%d", lookup.atCoordCalls)
    }
}
```

- [ ] **Step 2.5.2: Run, expect compile failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcFindExact_ -count=1
```

Expected: FAIL with "undefined: handleNpcFindExact".

### Step 2.6: Implement handleNpcFindExact

- [ ] **Step 2.6.1: Add `handleNpcFindExact` to handlers_npc.go**

Insert after `handleNpcFindCat`:

```go
// handleNpcFindExact (NPC_FINDEXACT, opcode 2518) pops (coord, npcType).
// Iterates NPCs at exactly (level, x, z) of the popped coord whose type
// matches. Mirrors TS NpcOps.ts:94-112. Pointer-set conditional on hit.
func handleNpcFindExact(s *ScriptState) error {
    npcTypeID := s.PopInt()
    coord := s.PopInt()

    level, x, z, err := checkCoord(coord, "NPC_FINDEXACT")
    if err != nil {
        return err
    }
    if err := checkNpcType(s, npcTypeID, "NPC_FINDEXACT"); err != nil {
        return err
    }

    var npc ActiveNpc
    if s.Npcs != nil {
        npc = s.Npcs.FindNpcAtExactCoord(level, x, z, npcTypeID)
    }
    if npc == nil {
        s.PushInt(0)
        return nil
    }
    setActiveNpcSlot(s, npc)
    s.PushInt(1)
    return nil
}
```

- [ ] **Step 2.6.2: Run NPC_FINDEXACT tests, expect pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcFindExact_ -count=1 -v
```

Expected: all 4 sub-tests PASS.

### Step 2.7: Register all three opcodes

- [ ] **Step 2.7.1: Add registry entries to handlers.go**

Edit `pkg/script/handlers.go`. Find the block of NPC mutator registrations ending at line ~331 (`OpNpcDelay`). Insert immediately after, with a blank line separator:

```go
// NPC find (S7f) — closest-single cluster.
OpNpcFind:      handleNpcFind,
OpNpcFindCat:   handleNpcFindCat,
OpNpcFindExact: handleNpcFindExact,
```

Also, as part of the bundled polish carry-forward from S7e, update the comment style on line 348 from `// Player flag setters.` to `// S7e: character-design flag setter.` for consistency with adjacent `// S7b: ...` / `// S7c: ...` blocks.

- [ ] **Step 2.7.2: Apply the S7e runner_test.go polish**

Edit `pkg/script/runner_test.go` around lines 430-432: the `SetAllowDesign` method currently carries the same 3-line docstring as the struct-site field at 224-226. Trim the method-site docstring to one line:

```go
// SetAllowDesign — see mockPlayer struct (runner_test.go:224) for field semantics.
func (m *mockPlayer) SetAllowDesign(v bool) {
    m.allowDesignValue = v
    m.allowDesignCalls++
}
```

Keep the struct-site comment at 224-226 intact.

### Step 2.8: Full Task 2 verification and commit

- [ ] **Step 2.8.1: Run all handler tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestNpcFind" -count=1 -v
```

Expected: all 17 test functions pass. Count via `grep -c "^--- PASS: TestNpcFind"` of the output.

- [ ] **Step 2.8.2: Run full pkg/script test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1
```

Expected: `ok github.com/zsrv/goscape/pkg/script`.

- [ ] **Step 2.8.3: Run go vet**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/script/
```

Expected: no output.

- [ ] **Step 2.8.4: Verify no other-package regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: no output.

- [ ] **Step 2.8.5: Commit Task 2**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers.go pkg/script/handlers_npc_test.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7f Task 2 — NPC_FIND / NPC_FINDCAT / NPC_FINDEXACT handlers

All three closest-single-NPC handlers implemented against mockNpcLookup.
17 handler tests (9 for NPC_FIND, 4 each for NPC_FINDCAT / NPC_FINDEXACT)
cover: hit/miss, nil-Npcs degradation, IntOperand 0/1 slot selection,
each validator failure path. World-side bridge lands in Task 3.

Bundled S7e review polish: handlers.go:348 comment style and
runner_test.go SetAllowDesign docstring trim.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 2.8.6: Report to controller**

Report:
- Status: DONE
- Commit SHA
- Handler test counts (9 + 4 + 4 = 17)
- LOC deltas
- Any divergences from spec §3.4 / §5.2 / §5.3 / §5.4 / §3.6

---

## Task 3: World-side bridge + integration tests + wire-up + close

**Files:**
- Create: `modules/world/npc_script_lookup.go`
- Create: `modules/world/npc_script_lookup_test.go`
- Modify: `modules/world/server.go` (lines 77, 198)
- Modify: `modules/world/interaction_trigger.go` (lines 85, 157, 318, 387)
- Modify: `modules/world/npc_script.go` (line 110)
- Modify: `modules/world/script_test.go` (lines 618, 663, 696, 743, 806)

### Step 3.1: Pre-flight re-enumeration

- [ ] **Step 3.1.1: Re-grep all wire-up sites**

Per `enumerate_all_sites` memory, re-grep to catch any sites that landed after the spec was written:

```bash
grep -n "state\.Inv = srv\.invLookup\|state\.Inv = s\.invLookup" modules/world/*.go
grep -n "s\.invLookup = invLookupView" modules/world/*.go
```

Expected output (compare to spec §3.3's 12-site table):
- `modules/world/interaction_trigger.go:85`, `:157`, `:318`, `:387` (production wire-up)
- `modules/world/npc_script.go:110` (production wire-up)
- `modules/world/script_test.go:618`, `:663`, `:696`, `:743`, `:806` (test fixtures)
- `modules/world/server.go:198` (init — via the `s.invLookup = invLookupView{s: s}` grep)

If the line numbers have drifted but the sites are the same, proceed with the drifted line numbers. If a NEW site has appeared that isn't in the spec's list, add it to the parallel-edit set and note it in the close commit.

### Step 3.2: Create `serverNpcLookup` with failing integration tests

- [ ] **Step 3.2.1: Write failing integration tests**

Create `modules/world/npc_script_lookup_test.go`:

```go
package world

import (
    "testing"

    "github.com/zsrv/goscape/pkg/objtype"
    "github.com/zsrv/goscape/pkg/script"
)

// setupLookupServer returns a Server with npcLookup bound and an
// NpcType 7 ("Hans") registered. Mirrors the fixture patterns at
// player_npc_test.go:33 and script_test.go:929+.
func setupLookupServer(t *testing.T) *Server {
    t.Helper()
    s := newTestServer(t) // existing helper; grep pkg/world tests
    s.npcLookup = serverNpcLookup{s: s}
    // Ensure the NpcType 7 is registered so checkNpcType would pass.
    s.npcTypes = &objtype.NpcTypeConfigs{Configs: make([]*objtype.NpcType, 100)}
    s.npcTypes.Configs[7] = &objtype.NpcType{ID: 7, Name: "Hans", Category: 5}
    s.npcTypes.Configs[8] = &objtype.NpcType{ID: 8, Name: "Other", Category: 9}
    return s
}

// setupNpc — if the existing helper from player_npc_test.go:33 is
// reachable, reuse it. Otherwise define a local one.

func TestServerNpcLookup_FindClosestByType(t *testing.T) {
    s := setupLookupServer(t)
    // Place 3 NPCs: near target type at (50, 50), far target type at
    // (60, 50), wrong type at (51, 50). Target coord (50, 50).
    near := setupNpc(t, s, 50, 50, 0)
    near.typeId = 7
    far := setupNpc(t, s, 60, 50, 0)
    far.typeId = 7
    wrong := setupNpc(t, s, 51, 50, 0)
    wrong.typeId = 8

    var _ = far
    var _ = wrong

    lookup := s.npcLookup
    got := lookup.FindClosestNpcByType(0, 50, 50, 30, 7, 0)
    if got == nil {
        t.Fatal("expected to find an NPC, got nil")
    }
    // Cast back to *Npc to compare identity.
    gotNpc, ok := got.(*Npc)
    if !ok {
        t.Fatalf("got type %T, want *Npc", got)
    }
    if gotNpc != near {
        t.Errorf("expected closest NPC %v, got %v", near, gotNpc)
    }
}

func TestServerNpcLookup_FindClosestByCategory(t *testing.T) {
    s := setupLookupServer(t)
    // Place 2 NPCs with matching category (NpcType.Category == 5), 1 non-match.
    catMatch := setupNpc(t, s, 50, 50, 0)
    catMatch.typeId = 7 // Category 5
    catFar := setupNpc(t, s, 60, 50, 0)
    catFar.typeId = 7
    catMiss := setupNpc(t, s, 51, 50, 0)
    catMiss.typeId = 8 // Category 9

    var _ = catFar
    var _ = catMiss

    lookup := s.npcLookup
    got := lookup.FindClosestNpcByCategory(0, 50, 50, 30, 5, 0)
    if got == nil {
        t.Fatal("expected to find an NPC with category 5, got nil")
    }
    gotNpc, ok := got.(*Npc)
    if !ok {
        t.Fatalf("got type %T, want *Npc", got)
    }
    if gotNpc != catMatch {
        t.Errorf("expected closer category-match NPC %v, got %v", catMatch, gotNpc)
    }
}

func TestServerNpcLookup_FindAtExactCoord(t *testing.T) {
    s := setupLookupServer(t)
    exact := setupNpc(t, s, 50, 50, 0)
    exact.typeId = 7

    lookup := s.npcLookup

    // Hit.
    got := lookup.FindNpcAtExactCoord(0, 50, 50, 7)
    if got == nil {
        t.Fatal("exact coord lookup should find the NPC")
    }

    // Off-by-one in x, z, level, and type — all miss.
    for _, tc := range []struct {
        name     string
        l, x, z  int
        typeID   int
    }{
        {"off by one x", 0, 51, 50, 7},
        {"off by one z", 0, 50, 51, 7},
        {"wrong level", 1, 50, 50, 7},
        {"wrong type", 0, 50, 50, 8},
    } {
        t.Run(tc.name, func(t *testing.T) {
            if got := lookup.FindNpcAtExactCoord(tc.l, tc.x, tc.z, tc.typeID); got != nil {
                t.Errorf("%s: expected nil, got %v", tc.name, got)
            }
        })
    }

    // Satisfy script.NpcLookup type assertion at compile time.
    var _ script.NpcLookup = s.npcLookup
}
```

**Note:** the exact shapes of `newTestServer`, `setupNpc`, and the `s.npcTypes`/`s.npcs` registry depend on the existing test infrastructure at `modules/world/script_test.go:929+` and `player_npc_test.go:33+`. Re-read those before writing the test file; adapt the fixture bootstrap to match.

- [ ] **Step 3.2.2: Run, expect compile failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestServerNpcLookup -count=1
```

Expected: FAIL with "undefined: serverNpcLookup" and "s.npcLookup undefined".

### Step 3.3: Add `npcLookup` field + init to Server

- [ ] **Step 3.3.1: Add field to Server struct**

Edit `modules/world/server.go` line 77. Insert after `invLookup invLookupView`:

```go
    invLookup    invLookupView
    npcLookup    serverNpcLookup
```

- [ ] **Step 3.3.2: Add init in server constructor**

Edit `modules/world/server.go` line 198. Insert after `s.invLookup = invLookupView{s: s}`:

```go
    s.invLookup = invLookupView{s: s}
    s.npcLookup = serverNpcLookup{s: s}
```

### Step 3.4: Implement `serverNpcLookup`

- [ ] **Step 3.4.1: Create `modules/world/npc_script_lookup.go`**

```go
package world

import (
    "github.com/zsrv/goscape/pkg/script"
)

// serverNpcLookup implements script.NpcLookup by linearly iterating
// s.npcs. See S7f spec §3.3 and deviations S7f-D1 (huntvis validated-
// only at the handler, not filtered at lookup) and S7f-D2 (linear
// iteration — future: route via s.grid.NearbyNpcs).
type serverNpcLookup struct{ s *Server }

// FindClosestNpcByType returns the NPC of typeID closest to (level, x, z)
// within square-bounded dist. Picks the later-iterated NPC on ties
// (TS NpcOps.ts:353 uses `<=`).
func (l serverNpcLookup) FindClosestNpcByType(level, x, z, dist, typeID, _ int) script.ActiveNpc {
    var closest *Npc
    bestDist := 1<<31 - 1 // max int32
    for _, n := range l.s.npcs {
        if n == nil || n.level != level || n.typeId != typeID {
            continue
        }
        dx := n.x - x
        dz := n.z - z
        if dx < 0 {
            dx = -dx
        }
        if dz < 0 {
            dz = -dz
        }
        if dx > dist || dz > dist {
            continue
        }
        d := (n.x-x)*(n.x-x) + (n.z-z)*(n.z-z)
        if d <= bestDist {
            closest = n
            bestDist = d
        }
    }
    if closest == nil {
        return nil
    }
    return closest
}

// FindClosestNpcByCategory — same shape as FindClosestNpcByType but
// filter via NpcType.Category == cat (look up via s.npcTypes.Configs).
func (l serverNpcLookup) FindClosestNpcByCategory(level, x, z, dist, cat, _ int) script.ActiveNpc {
    var closest *Npc
    bestDist := 1<<31 - 1
    if l.s.npcTypes == nil {
        return nil
    }
    for _, n := range l.s.npcs {
        if n == nil || n.level != level {
            continue
        }
        if n.typeId < 0 || n.typeId >= len(l.s.npcTypes.Configs) {
            continue
        }
        nt := l.s.npcTypes.Configs[n.typeId]
        if nt == nil || nt.Category != cat {
            continue
        }
        dx := n.x - x
        dz := n.z - z
        if dx < 0 {
            dx = -dx
        }
        if dz < 0 {
            dz = -dz
        }
        if dx > dist || dz > dist {
            continue
        }
        d := (n.x-x)*(n.x-x) + (n.z-z)*(n.z-z)
        if d <= bestDist {
            closest = n
            bestDist = d
        }
    }
    if closest == nil {
        return nil
    }
    return closest
}

// FindNpcAtExactCoord returns the first NPC at exactly (level, x, z)
// whose type matches typeID, or nil.
func (l serverNpcLookup) FindNpcAtExactCoord(level, x, z, typeID int) script.ActiveNpc {
    for _, n := range l.s.npcs {
        if n == nil {
            continue
        }
        if n.level == level && n.x == x && n.z == z && n.typeId == typeID {
            return n
        }
    }
    return nil
}
```

**Note:** `*Npc` must already implement `script.ActiveNpc` for this to compile. Grep `func (n \*Npc).*NpcType\|NpcX\|NpcZ\|NpcLevel` — if the bindings exist, this will compile. If not, that's a separate bridge gap predating S7f and should be flagged as BLOCKED before proceeding.

- [ ] **Step 3.4.2: Wire `state.Npcs = srv.npcLookup` at all 5 production sites**

Edit each site found in Step 3.1.1. For each `state.Inv = srv.invLookup` or `state.Inv = s.invLookup` line, add the parallel `state.Npcs = srv.npcLookup` or `state.Npcs = s.npcLookup` on the next line. Sites (line numbers may have drifted):

1. `modules/world/interaction_trigger.go:85` → add `state.Npcs = srv.npcLookup`
2. `modules/world/interaction_trigger.go:157` → same
3. `modules/world/interaction_trigger.go:318` → same
4. `modules/world/interaction_trigger.go:387` → same
5. `modules/world/npc_script.go:110` → add `state.Npcs = s.npcLookup`

- [ ] **Step 3.4.3: Wire `s.npcLookup = serverNpcLookup{s: s}` at all 5 test-fixture sites**

Edit each site found in Step 3.1.1. For each `s.invLookup = invLookupView{s: s}` line, add the parallel init immediately after:

1. `modules/world/script_test.go:618`
2. `modules/world/script_test.go:663`
3. `modules/world/script_test.go:696`
4. `modules/world/script_test.go:743`
5. `modules/world/script_test.go:806`

Pattern at each site:
```go
    s.invLookup = invLookupView{s: s}
    s.npcLookup = serverNpcLookup{s: s}
```

- [ ] **Step 3.4.4: Run integration tests, expect pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestServerNpcLookup -count=1 -v
```

Expected: all 3 (+ 4 sub-tests in FindAtExactCoord) PASS.

- [ ] **Step 3.4.5: Full modules/world test run**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: `ok github.com/zsrv/goscape/modules/world`. If any pre-existing test fails, it indicates a test fixture wasn't updated — re-check §3.4.3.

### Step 3.5: Full regression + close commit

- [ ] **Step 3.5.1: Full repo build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: no output.

- [ ] **Step 3.5.2: Full repo tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...
```

Expected: all packages green.

- [ ] **Step 3.5.3: go vet**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: no output.

- [ ] **Step 3.5.4: Close commit**

```bash
git add modules/world/npc_script_lookup.go modules/world/npc_script_lookup_test.go modules/world/server.go modules/world/interaction_trigger.go modules/world/npc_script.go modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(script): S7f closed — NPC_FIND / FINDCAT / FINDEXACT + NpcLookup bridge

World-side serverNpcLookup implements the three lookup methods via
linear iteration over s.npcs; wired at all 5 production sites and all
5 test-fixture sites. 3 integration tests pin the iteration semantics
against real *Npc fixtures.

Unblocks: [proc,set_hint_newbie_basics_instructor] past pc=8.
Deviations added per spec §7: S7f-D1 (huntvis validated-only),
S7f-D2 (linear iteration), S7f-D3 (CategoryType count-bound absent).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3.5.5: Report to controller**

Report:
- Status: DONE
- Three commit SHAs (Task 1, Task 2, Task 3)
- File touch counts and LOC deltas
- Integration test counts passing
- Full test suite pass confirmation
- Any wire-up-site drift noted during Step 3.1.1

---

## Self-review notes (pre-handoff)

Cross-checked this plan against `docs/superpowers/specs/2026-04-24-runescript-s7f-npc-find-family-design.md`:

- **Spec §3.1 validators:** Task 1.2.1 implements all four; Task 1.1.1 tests all four. ✓
- **Spec §3.2 NpcLookup + Npcs field:** Task 1.4.3 adds interface; Task 1.4.4 adds field; Task 1.4.5 adds mock. ✓
- **Spec §3.3 serverNpcLookup + 12 wire-up sites:** Task 3.4.1 creates impl; Tasks 3.3/3.4.2/3.4.3 cover all 12 sites with explicit line-number references. Task 3.1.1 re-greps pre-commit per `enumerate_all_sites` memory. ✓
- **Spec §3.4 three handlers:** Task 2.2.1, 2.4.1, 2.6.1 implement each; Tasks 2.1, 2.3, 2.5 test each. ✓
- **Spec §3.5 setActiveNpcSlot:** Task 1.3.3 implements; Task 1.3.1 tests both operand branches + panic. ✓
- **Spec §3.6 registry:** Task 2.7.1 adds all three entries. ✓
- **Spec §5.1 validator unit tests (4 functions):** Task 1.1.1. ✓
- **Spec §5.2 NPC_FIND tests (9 cases):** Task 2.1.1 — SingleMatch, NoMatch, NilNpcLookup, IntOperandZero, IntOperandOne, InvalidCoord, InvalidNpcType, NullDistance, InvalidHuntVis. Spec mentions ClosestWinsOnTies as case #2 — plan covers via integration test in §5.5 (Task 3.2.1 `TestServerNpcLookup_FindClosestByType` pins the later-iterated-wins invariant using real world state). Acceptable — the handler-level test with a mock returning a pre-chosen NPC cannot test the iteration logic itself, only the handler-to-lookup arg-passing, which IS tested in `TestNpcFind_SingleMatch` via `lastArgs` cross-check. ✓
- **Spec §5.3 NPC_FINDCAT tests (4 cases):** Task 2.3.1 — SingleMatch, NoMatch, NullCategory, PartialValidatorAcceptsNonNegative. ✓
- **Spec §5.4 NPC_FINDEXACT tests (4 cases):** Task 2.5.1 — Match, NoNpcAtCoord, InvalidCoord, InvalidNpcType. The spec's "TypeMismatchAtCoord" (spec §5.4 #2) is implicitly covered by the off-by-one x/z/level tests in the world-side integration `TestServerNpcLookup_FindAtExactCoord` (Task 3.2.1's `wrong type` sub-case specifically). ✓
- **Spec §5.5 integration tests (3 tests):** Task 3.2.1 — FindClosestByType, FindClosestByCategory, FindAtExactCoord. ✓
- **Spec §5.6 mockNpcLookup:** Task 1.4.5 — fields + methods matching spec shape. ✓
- **Spec §6 task split:** Three tasks per user's collapse. ✓
- **Spec §8 bundled S7e polish:** Task 2.7.1 (handlers.go:348 comment) + Task 2.7.2 (runner_test.go docstring trim). ✓

**Placeholder scan:** None. Every code block is complete; every file path is exact; every command is runnable with expected-output lines.

**Type consistency:** `NpcLookup` interface methods have the same signatures in Task 1 (declaration), Task 1.4.5 (mock), Task 2 (handler consumers), and Task 3 (production impl). `setActiveNpcSlot` signature and semantics consistent across declaration (Task 1.3.3), test (Task 1.3.1), and handler callers (Task 2). Wire-up field name `npcLookup` consistent across server.go declaration, production init, interaction_trigger.go wire-ups, and script_test.go test-fixture wire-ups.

**Open risks flagged for implementer:**
- Task 1.1.2: `testConfigs*` helper may already exist; grep first and extend rather than create.
- Task 3.2.1: `newTestServer` + `setupNpc` fixture APIs may differ from what the test code assumes — read `script_test.go:929+` and `player_npc_test.go:33+` before writing the integration tests.
- Task 3.4.1: `*Npc` must implement `script.ActiveNpc`. If it doesn't, implementer should report BLOCKED rather than modify `*Npc`'s method set as part of S7f (would be a separate sub-spec).
