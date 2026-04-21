# RuneScript S5a: Pure VM Opcode Expansion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Register 49 new opcode handlers across NumberOps, StringOps, DebugOps, and the Core array/switch/comparison-branch bits so cache scripts doing math/string/bit/compare logic run without `unknown opcode` errors.

**Architecture:** Split handlers across four new files (`handlers_{number,string,debug,array}.go`) by TS category, keeping the single `handlers` map in `handlers.go` as source of truth. Extend `ScriptState` with `Arrays [][]int32`, extend `ActivePlayer` with `Playtime() int`. No new server state.

**Tech Stack:** Go 1.22+, existing `pkg/script/` VM, existing opcode constants (all 49 already declared in `pkg/script/opcode.go` from S1 scaffolding).

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s5a-pure-vm-opcodes-design.md`](../specs/2026-04-21-runescript-s5a-pure-vm-opcodes-design.md)

---

## Task 1: Extend `ActivePlayer` with `Playtime()` + Player impl

**Files:**
- Modify: `pkg/script/active.go`
- Modify: `modules/world/player_script.go`

- [ ] **Step 1: Add `Playtime()` to `ActivePlayer`**

In `pkg/script/active.go`, append to the interface:

```go
	// Playtime returns the number of ticks the player has been online
	// this session, used by the TIMESPENT / GETTIMESPENT opcodes.
	Playtime() int
```

- [ ] **Step 2: Implement `Playtime()` on `*Player`**

In `modules/world/player_script.go`, append:

```go
// Playtime implements script.ActivePlayer.Playtime. The playtime field is
// incremented in processIn each tick.
func (p *Player) Playtime() int { return int(p.playtime) }
```

- [ ] **Step 3: Verify the full build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: success. The compile-time assertion `var _ script.ActivePlayer = (*Player)(nil)` in `message_game.go` catches the new interface method.

- [ ] **Step 4: Commit**

```bash
git add pkg/script/active.go modules/world/player_script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): ActivePlayer.Playtime for TIMESPENT opcode support

Adds Playtime() int to ActivePlayer; *Player returns int(p.playtime).
Prepares S5a's DebugOps handlers.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `Arrays [][]int32` to `ScriptState`

**Files:**
- Modify: `pkg/script/state.go`

- [ ] **Step 1: Add the field**

In `pkg/script/state.go`, inside the `ScriptState` struct, add after the `Protect bool` line:

```go
	// Arrays holds script-local int[] arrays defined via DEFINE_ARRAY.
	// Index = array slot (0..4); length set at DEFINE_ARRAY, fixed thereafter.
	Arrays [5][]int32
```

Using a fixed `[5][]int32` rather than a slice — TS caps at 5 slots and this makes bounds-checking free. The nil-slice zero-value means "undefined; OOB access returns 0".

- [ ] **Step 2: Verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add pkg/script/state.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): ScriptState.Arrays for DEFINE_ARRAY opcode support

[5][]int32 layout matches TS 5-slot cap; nil slots read as zero on OOB.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: NumberOps — 28 handlers (arithmetic + bitwise + random + 4 comparison branches)

**Files:**
- Create: `pkg/script/handlers_number.go`
- Create: `pkg/script/handlers_number_test.go`
- Modify: `pkg/script/handlers.go` (register in map)

- [ ] **Step 1: Create `pkg/script/handlers_number.go`**

```go
package script

import (
	"errors"
	"math/bits"
	"math/rand/v2"
)

// floorDiv returns floor(a / b), matching TS's Math.floor(a/b). Panics
// on zero divisor; callers must pre-check and return an error.
func floorDiv(a, b int) int {
	q := a / b
	// Go truncates toward zero; floor division rounds toward -inf. Adjust
	// when the quotient is negative and there is a non-zero remainder.
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// posMod returns a mathematical modulus that is always non-negative when
// b > 0, matching TS's ((a%b)+b)%b idiom.
func posMod(a, b int) int {
	r := a % b
	if r < 0 && b > 0 {
		r += b
	} else if r > 0 && b < 0 {
		r += b
	}
	return r
}

// -- Comparison branches --

func handleBranchLessThan(s *ScriptState) error {
	rhs := s.PopInt()
	lhs := s.PopInt()
	if lhs < rhs {
		s.PC += int(s.Script.IntOperands[s.PC])
	}
	return nil
}

func handleBranchGreaterThan(s *ScriptState) error {
	rhs := s.PopInt()
	lhs := s.PopInt()
	if lhs > rhs {
		s.PC += int(s.Script.IntOperands[s.PC])
	}
	return nil
}

func handleBranchLessThanOrEquals(s *ScriptState) error {
	rhs := s.PopInt()
	lhs := s.PopInt()
	if lhs <= rhs {
		s.PC += int(s.Script.IntOperands[s.PC])
	}
	return nil
}

func handleBranchGreaterThanOrEquals(s *ScriptState) error {
	rhs := s.PopInt()
	lhs := s.PopInt()
	if lhs >= rhs {
		s.PC += int(s.Script.IntOperands[s.PC])
	}
	return nil
}

// -- Arithmetic --

func handleMultiply(s *ScriptState) error {
	rhs := s.PopInt()
	lhs := s.PopInt()
	// 32-bit wraparound to match TS Math.imul semantics.
	s.PushInt(int(int32(lhs) * int32(rhs)))
	return nil
}

func handleDivide(s *ScriptState) error {
	rhs := s.PopInt()
	lhs := s.PopInt()
	if rhs == 0 {
		return errors.New("DIVIDE: division by zero")
	}
	s.PushInt(floorDiv(lhs, rhs))
	return nil
}

func handleModulo(s *ScriptState) error {
	rhs := s.PopInt()
	lhs := s.PopInt()
	if rhs == 0 {
		return errors.New("MODULO: division by zero")
	}
	s.PushInt(posMod(lhs, rhs))
	return nil
}

func handleAbs(s *ScriptState) error {
	x := s.PopInt()
	if x < 0 {
		x = -x
	}
	s.PushInt(x)
	return nil
}

func handleAddPercent(s *ScriptState) error {
	rhs := s.PopInt()
	lhs := s.PopInt()
	s.PushInt(lhs + (lhs*rhs)/100)
	return nil
}

func handleScale(s *ScriptState) error {
	c := s.PopInt()
	b := s.PopInt()
	a := s.PopInt()
	if c == 0 {
		return errors.New("SCALE: division by zero")
	}
	s.PushInt(floorDiv(a*b, c))
	return nil
}

func handleMin(s *ScriptState) error {
	rhs := s.PopInt()
	lhs := s.PopInt()
	s.PushInt(min(lhs, rhs))
	return nil
}

func handleMax(s *ScriptState) error {
	rhs := s.PopInt()
	lhs := s.PopInt()
	s.PushInt(max(lhs, rhs))
	return nil
}

func handlePow(s *ScriptState) error {
	exp := s.PopInt()
	base := s.PopInt()
	if exp < 0 {
		s.PushInt(0)
		return nil
	}
	result := int32(1)
	b32 := int32(base)
	for range exp {
		result *= b32
	}
	s.PushInt(int(result))
	return nil
}

func handleInvPow(s *ScriptState) error {
	// invpow(value, base) = floor(log_base(value)). Zero/negative value
	// returns 0; base <= 1 returns 0.
	base := s.PopInt()
	value := s.PopInt()
	if value <= 0 || base <= 1 {
		s.PushInt(0)
		return nil
	}
	n := 0
	for value >= base {
		value /= base
		n++
	}
	s.PushInt(n)
	return nil
}

// -- Bitwise --

func handleAnd(s *ScriptState) error {
	rhs := s.PopInt()
	lhs := s.PopInt()
	s.PushInt(lhs & rhs)
	return nil
}

func handleOr(s *ScriptState) error {
	rhs := s.PopInt()
	lhs := s.PopInt()
	s.PushInt(lhs | rhs)
	return nil
}

func handleBitCount(s *ScriptState) error {
	x := uint32(s.PopInt())
	s.PushInt(bits.OnesCount32(x))
	return nil
}

func handleTestBit(s *ScriptState) error {
	bit := s.PopInt()
	value := s.PopInt()
	s.PushInt((value >> bit) & 1)
	return nil
}

func handleSetBit(s *ScriptState) error {
	bit := s.PopInt()
	value := s.PopInt()
	s.PushInt(value | (1 << bit))
	return nil
}

func handleClearBit(s *ScriptState) error {
	bit := s.PopInt()
	value := s.PopInt()
	s.PushInt(value &^ (1 << bit))
	return nil
}

func handleToggleBit(s *ScriptState) error {
	bit := s.PopInt()
	value := s.PopInt()
	s.PushInt(value ^ (1 << bit))
	return nil
}

// bitMask returns a mask covering bits [start..end] inclusive.
func bitMask(start, end int) int {
	width := end - start + 1
	if width <= 0 {
		return 0
	}
	return ((1 << width) - 1) << start
}

func handleGetBitRange(s *ScriptState) error {
	end := s.PopInt()
	start := s.PopInt()
	value := s.PopInt()
	width := end - start + 1
	if width <= 0 {
		s.PushInt(0)
		return nil
	}
	s.PushInt((value >> start) & ((1 << width) - 1))
	return nil
}

func handleSetBitRange(s *ScriptState) error {
	end := s.PopInt()
	start := s.PopInt()
	value := s.PopInt()
	s.PushInt(value | bitMask(start, end))
	return nil
}

func handleClearBitRange(s *ScriptState) error {
	end := s.PopInt()
	start := s.PopInt()
	value := s.PopInt()
	s.PushInt(value &^ bitMask(start, end))
	return nil
}

func handleSetBitRangeToInt(s *ScriptState) error {
	end := s.PopInt()
	start := s.PopInt()
	bitsVal := s.PopInt()
	value := s.PopInt()
	mask := bitMask(start, end)
	width := end - start + 1
	if width <= 0 {
		s.PushInt(value)
		return nil
	}
	low := bitsVal & ((1 << width) - 1)
	s.PushInt((value &^ mask) | (low << start))
	return nil
}

// -- Random --

func handleRandom(s *ScriptState) error {
	n := s.PopInt()
	if n <= 0 {
		s.PushInt(0)
		return nil
	}
	s.PushInt(rand.IntN(n))
	return nil
}

func handleRandomInc(s *ScriptState) error {
	n := s.PopInt()
	if n < 0 {
		s.PushInt(0)
		return nil
	}
	s.PushInt(rand.IntN(n + 1))
	return nil
}
```

- [ ] **Step 2: Register in `pkg/script/handlers.go`**

Extend the `handlers` map literal with all 28 new entries (kept alphabetical within each group for diffability):

```go
	OpBranchLessThan:            handleBranchLessThan,
	OpBranchGreaterThan:         handleBranchGreaterThan,
	OpBranchLessThanOrEquals:    handleBranchLessThanOrEquals,
	OpBranchGreaterThanOrEquals: handleBranchGreaterThanOrEquals,

	OpMultiply:         handleMultiply,
	OpDivide:           handleDivide,
	OpModulo:           handleModulo,
	OpAbs:              handleAbs,
	OpAddPercent:       handleAddPercent,
	OpScale:            handleScale,
	OpMin:              handleMin,
	OpMax:              handleMax,
	OpPow:              handlePow,
	OpInvPow:           handleInvPow,

	OpAnd:              handleAnd,
	OpOr:               handleOr,
	OpBitCount:         handleBitCount,
	OpTestBit:          handleTestBit,
	OpSetBit:           handleSetBit,
	OpClearBit:         handleClearBit,
	OpToggleBit:        handleToggleBit,
	OpGetBitRange:      handleGetBitRange,
	OpSetBitRange:      handleSetBitRange,
	OpClearBitRange:    handleClearBitRange,
	OpSetBitRangeToInt: handleSetBitRangeToInt,

	OpRandom:    handleRandom,
	OpRandomInc: handleRandomInc,
```

- [ ] **Step 3: Create `pkg/script/handlers_number_test.go` — table-driven tests**

```go
package script

import (
	"testing"
)

// runSingleOp builds a one-instruction script that runs `op` with the
// given int stack pre-pushed (bottom-to-top), executes it, and returns
// the top of the int stack.
func runSingleOp(t *testing.T, op Opcode, inputs []int) int {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + opcodeName(op),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	for _, v := range inputs {
		state.PushInt(v)
	}
	if err := Execute(state); err != nil {
		t.Fatalf("%s: unexpected error: %v", opcodeName(op), err)
	}
	return state.PopInt()
}

func TestNumberHandlers(t *testing.T) {
	cases := []struct {
		name   string
		op     Opcode
		inputs []int
		expect int
	}{
		// Arithmetic
		{"multiply pos", OpMultiply, []int{6, 7}, 42},
		{"multiply neg", OpMultiply, []int{-3, 5}, -15},
		{"divide pos", OpDivide, []int{10, 3}, 3},
		{"divide floor neg", OpDivide, []int{-7, 2}, -4},
		{"modulo pos", OpModulo, []int{10, 3}, 1},
		{"modulo neg", OpModulo, []int{-7, 3}, 2},
		{"abs neg", OpAbs, []int{-9}, 9},
		{"abs pos", OpAbs, []int{7}, 7},
		{"addpercent +50%", OpAddPercent, []int{100, 50}, 150},
		{"scale 3/4 of 200", OpScale, []int{200, 3, 4}, 150},
		{"min", OpMin, []int{5, 3}, 3},
		{"max", OpMax, []int{5, 3}, 5},
		{"pow 2^10", OpPow, []int{2, 10}, 1024},
		{"pow 0 exp", OpPow, []int{5, 0}, 1},
		{"pow neg exp", OpPow, []int{5, -1}, 0},
		{"invpow 1024 base 2", OpInvPow, []int{1024, 2}, 10},
		{"invpow zero value", OpInvPow, []int{0, 2}, 0},

		// Bitwise
		{"and", OpAnd, []int{0b1100, 0b1010}, 0b1000},
		{"or", OpOr, []int{0b1100, 0b1010}, 0b1110},
		{"bitcount 0xFF", OpBitCount, []int{0xFF}, 8},
		{"bitcount 0", OpBitCount, []int{0}, 0},
		{"testbit set", OpTestBit, []int{0b1010, 1}, 1},
		{"testbit clear", OpTestBit, []int{0b1010, 0}, 0},
		{"setbit", OpSetBit, []int{0, 3}, 0b1000},
		{"clearbit", OpClearBit, []int{0b1111, 2}, 0b1011},
		{"togglebit on", OpToggleBit, []int{0, 0}, 1},
		{"togglebit off", OpToggleBit, []int{1, 0}, 0},
		{"getbitrange [2..4]", OpGetBitRange, []int{0b11100, 2, 4}, 0b111},
		{"setbitrange [1..3]", OpSetBitRange, []int{0, 1, 3}, 0b1110},
		{"clearbitrange [1..3]", OpClearBitRange, []int{0b1111, 1, 3}, 0b0001},
		{"setbitrangetoint [1..3]=0b101", OpSetBitRangeToInt, []int{0, 0b101, 1, 3}, 0b1010},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runSingleOp(t, tc.op, tc.inputs)
			if got != tc.expect {
				t.Errorf("%s: got %d, want %d", tc.name, got, tc.expect)
			}
		})
	}
}

func TestDivideByZeroAborts(t *testing.T) {
	sf := &ScriptFile{
		Name:             "divzero",
		Opcodes:          []Opcode{OpDivide, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.PushInt(10)
	state.PushInt(0)
	if err := Execute(state); err == nil {
		t.Fatal("Execute: want error on div by zero")
	}
	if state.Execution != Aborted {
		t.Errorf("Execution: got %v, want Aborted", state.Execution)
	}
}

func TestRandomInRange(t *testing.T) {
	for range 100 {
		got := runSingleOp(t, OpRandom, []int{10})
		if got < 0 || got >= 10 {
			t.Errorf("OpRandom(10): got %d, want [0..9]", got)
		}
	}
}

func TestRandomIncInRange(t *testing.T) {
	for range 100 {
		got := runSingleOp(t, OpRandomInc, []int{10})
		if got < 0 || got > 10 {
			t.Errorf("OpRandomInc(10): got %d, want [0..10]", got)
		}
	}
}

func TestComparisonBranches(t *testing.T) {
	// Script: [PUSH lhs, PUSH rhs, BRANCH_X +1, PUSH 0, RETURN, PUSH 1, RETURN]
	// Verifies branch-taken skips to the PUSH 1 + RETURN path.
	cases := []struct {
		name        string
		op          Opcode
		lhs, rhs    int
		branchTaken int // expected value after execution (0 = fell through, 1 = branched)
	}{
		{"lt true", OpBranchLessThan, 1, 2, 1},
		{"lt false", OpBranchLessThan, 2, 1, 0},
		{"gt true", OpBranchGreaterThan, 2, 1, 1},
		{"gt false", OpBranchGreaterThan, 1, 2, 0},
		{"lte true eq", OpBranchLessThanOrEquals, 2, 2, 1},
		{"lte false", OpBranchLessThanOrEquals, 3, 2, 0},
		{"gte true eq", OpBranchGreaterThanOrEquals, 2, 2, 1},
		{"gte false", OpBranchGreaterThanOrEquals, 1, 2, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sf := &ScriptFile{
				Name: "branch_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, // 0: push lhs
					OpPushConstantInt, // 1: push rhs
					tc.op,             // 2: branch +2 → jumps over [3, 4]
					OpPushConstantInt, // 3: push 0 (fell through)
					OpReturn,          // 4: return
					OpPushConstantInt, // 5: push 1 (branch taken)
					OpReturn,          // 6: return
				},
				IntOperands:      []int32{int32(tc.lhs), int32(tc.rhs), 2, 0, 0, 1, 0},
				StringOperands:   []string{"", "", "", "", "", "", ""},
				InstructionCount: 7,
			}
			state := Init(sf, nil, false, nil, nil)
			if err := Execute(state); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got := state.PopInt(); got != tc.branchTaken {
				t.Errorf("%s: got %d, want %d", tc.name, got, tc.branchTaken)
			}
		})
	}
}
```

Where `opcodeName` is the existing disassembly helper in `opcode.go`.

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestNumberHandlers|TestDivideByZero|TestRandom|TestComparisonBranches' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/handlers_number.go pkg/script/handlers_number_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5a NumberOps + comparison branches (28 handlers)

Arithmetic: multiply/divide/modulo/abs/addpercent/scale/min/max/pow/
invpow with TS-matching floor-div and positive-modulo semantics.
Bitwise: and/or/bitcount/testbit/setbit/clearbit/togglebit plus the
four bit-range variants.
Random: RANDOM (exclusive) and RANDOMINC (inclusive) via math/rand/v2.
Comparison branches: BRANCH_LESS_THAN / _GREATER_THAN /
_LESS_THAN_OR_EQUALS / _GREATER_THAN_OR_EQUALS mirror the existing
BRANCH_EQUALS / BRANCH_NOT pattern.

Table-driven tests cover 30+ arithmetic/bitwise cases; per-branch
taken/fall-through matrix; div-by-zero abort; random range assertions
over 100 iterations.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: StringOps — 11 active + 5 SPLIT stubs (16 handlers)

**Files:**
- Create: `pkg/script/handlers_string.go`
- Create: `pkg/script/handlers_string_test.go`
- Modify: `pkg/script/handlers.go` (register)

- [ ] **Step 1: Create `pkg/script/handlers_string.go`**

```go
package script

import (
	"log/slog"
	"strconv"
	"strings"
)

func handleAppend(s *ScriptState) error {
	suffix := s.PopString()
	base := s.PopString()
	s.PushString(base + suffix)
	return nil
}

func handleAppendNum(s *ScriptState) error {
	n := s.PopInt()
	base := s.PopString()
	s.PushString(base + strconv.Itoa(n))
	return nil
}

func handleAppendChar(s *ScriptState) error {
	ch := s.PopInt()
	base := s.PopString()
	s.PushString(base + string(rune(ch)))
	return nil
}

func handleAppendSignNum(s *ScriptState) error {
	n := s.PopInt()
	base := s.PopString()
	// TS appendsignnum: negative keeps -, positive has no + prefix,
	// zero prints as "0".
	s.PushString(base + strconv.Itoa(n))
	return nil
}

func handleLowercase(s *ScriptState) error {
	s.PushString(strings.ToLower(s.PopString()))
	return nil
}

func handleCompare(s *ScriptState) error {
	rhs := s.PopString()
	lhs := s.PopString()
	s.PushInt(strings.Compare(lhs, rhs))
	return nil
}

func handleStringLength(s *ScriptState) error {
	s.PushInt(len(s.PopString()))
	return nil
}

func handleSubstring(s *ScriptState) error {
	end := s.PopInt()
	start := s.PopInt()
	src := s.PopString()
	if start < 0 {
		start = 0
	}
	if end > len(src) {
		end = len(src)
	}
	if start > end {
		start = end
	}
	s.PushString(src[start:end])
	return nil
}

func handleStringIndexOfChar(s *ScriptState) error {
	ch := s.PopInt()
	src := s.PopString()
	s.PushInt(strings.IndexRune(src, rune(ch)))
	return nil
}

func handleStringIndexOfString(s *ScriptState) error {
	needle := s.PopString()
	haystack := s.PopString()
	s.PushInt(strings.Index(haystack, needle))
	return nil
}

// handleTextSwitch is a stub until we decode string-switch tables in
// ScriptFile. TS branches based on a string key; for S5a we just
// pop the key and fall through.
func handleTextSwitch(s *ScriptState) error {
	_ = s.PopString()
	slog.Debug("TEXT_SWITCH not implemented; falling through",
		"script", s.Script.Name, "pc", s.PC)
	return nil
}

// -- SPLIT_* stubs (dialog pagination — deferred to later sub-spec) --

func handleSplitInit(s *ScriptState) error {
	// Pops (text, fontId, maxWidth) per TS; we don't keep them.
	_ = s.PopInt()
	_ = s.PopInt()
	_ = s.PopString()
	slog.Debug("SPLIT_INIT stub invoked", "script", s.Script.Name)
	return nil
}

func handleSplitGet(s *ScriptState) error {
	// Pops (page, line) per TS.
	_ = s.PopInt()
	_ = s.PopInt()
	s.PushString("")
	return nil
}

func handleSplitGetAnim(s *ScriptState) error {
	_ = s.PopInt()
	_ = s.PopInt()
	s.PushInt(-1)
	return nil
}

func handleSplitLineCount(s *ScriptState) error {
	_ = s.PopInt()
	s.PushInt(0)
	return nil
}

func handleSplitPageCount(s *ScriptState) error {
	s.PushInt(0)
	return nil
}
```

- [ ] **Step 2: Register 16 handlers in `pkg/script/handlers.go`**

```go
	OpAppend:              handleAppend,
	OpAppendNum:           handleAppendNum,
	OpAppendChar:          handleAppendChar,
	OpAppendSignNum:       handleAppendSignNum,
	OpLowercase:           handleLowercase,
	OpCompare:             handleCompare,
	OpStringLength:        handleStringLength,
	OpSubstring:           handleSubstring,
	OpStringIndexOfChar:   handleStringIndexOfChar,
	OpStringIndexOfString: handleStringIndexOfString,
	OpTextSwitch:          handleTextSwitch,

	OpSplitInit:      handleSplitInit,
	OpSplitGet:       handleSplitGet,
	OpSplitGetAnim:   handleSplitGetAnim,
	OpSplitLineCount: handleSplitLineCount,
	OpSplitPageCount: handleSplitPageCount,
```

- [ ] **Step 3: Create `pkg/script/handlers_string_test.go`**

```go
package script

import (
	"testing"
)

func runStringOp(t *testing.T, op Opcode, intInputs []int, stringInputs []string) (topInt int, topStr string) {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_str_" + opcodeName(op),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	for _, v := range stringInputs {
		state.PushString(v)
	}
	for _, v := range intInputs {
		state.PushInt(v)
	}
	if err := Execute(state); err != nil {
		t.Fatalf("%s: unexpected error: %v", opcodeName(op), err)
	}
	return state.PopInt(), state.PopString()
}

func TestStringAppend(t *testing.T) {
	_, got := runStringOp(t, OpAppend, nil, []string{"foo", "bar"})
	if got != "foobar" {
		t.Errorf("Append: got %q, want %q", got, "foobar")
	}
}

func TestStringAppendNum(t *testing.T) {
	_, got := runStringOp(t, OpAppendNum, []int{42}, []string{"n="})
	if got != "n=42" {
		t.Errorf("AppendNum: got %q, want %q", got, "n=42")
	}
}

func TestStringAppendChar(t *testing.T) {
	_, got := runStringOp(t, OpAppendChar, []int{'!'}, []string{"hi"})
	if got != "hi!" {
		t.Errorf("AppendChar: got %q, want %q", got, "hi!")
	}
}

func TestStringLowercase(t *testing.T) {
	_, got := runStringOp(t, OpLowercase, nil, []string{"HeLLo"})
	if got != "hello" {
		t.Errorf("Lowercase: got %q, want %q", got, "hello")
	}
}

func TestStringCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"a", "b", -1},
		{"b", "a", 1},
		{"foo", "foo", 0},
	}
	for _, tc := range cases {
		t.Run(tc.a+"vs"+tc.b, func(t *testing.T) {
			got, _ := runStringOp(t, OpCompare, nil, []string{tc.a, tc.b})
			if got != tc.want {
				t.Errorf("Compare(%q,%q): got %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestStringLength(t *testing.T) {
	got, _ := runStringOp(t, OpStringLength, nil, []string{"hello"})
	if got != 5 {
		t.Errorf("StringLength: got %d, want 5", got)
	}
}

func TestStringSubstring(t *testing.T) {
	_, got := runStringOp(t, OpSubstring, []int{1, 4}, []string{"hello"})
	if got != "ell" {
		t.Errorf("Substring: got %q, want %q", got, "ell")
	}
}

func TestStringIndexOfChar(t *testing.T) {
	got, _ := runStringOp(t, OpStringIndexOfChar, []int{'l'}, []string{"hello"})
	if got != 2 {
		t.Errorf("IndexOfChar: got %d, want 2", got)
	}
}

func TestStringIndexOfString(t *testing.T) {
	got, _ := runStringOp(t, OpStringIndexOfString, nil, []string{"hello world", "world"})
	if got != 6 {
		t.Errorf("IndexOfString: got %d, want 6", got)
	}
}

func TestSplitStubsReturnZeroes(t *testing.T) {
	got, _ := runStringOp(t, OpSplitLineCount, []int{0}, nil)
	if got != 0 {
		t.Errorf("SplitLineCount stub: got %d, want 0", got)
	}
	got, _ = runStringOp(t, OpSplitPageCount, nil, nil)
	if got != 0 {
		t.Errorf("SplitPageCount stub: got %d, want 0", got)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestString -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/handlers_string.go pkg/script/handlers_string_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5a StringOps (11 active + 5 SPLIT stubs)

Active: APPEND, APPEND_NUM, APPEND_CHAR, APPEND_SIGNNUM, LOWERCASE,
COMPARE, STRING_LENGTH, SUBSTRING, STRING_INDEXOF_CHAR,
STRING_INDEXOF_STRING. TEXT_SWITCH pops + falls through with a Debug
log (string-switch tables are not yet in the file decoder).

Stubs: SPLIT_INIT / SPLIT_GET / SPLIT_GETANIM / SPLIT_LINECOUNT /
SPLIT_PAGECOUNT all pop their TS argument lists and push empty/zero
sentinels plus a Debug log so we can see which scripts want dialog
pagination before building that subsystem.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: DebugOps — 3 handlers (ERROR, GETTIMESPENT, TIMESPENT)

**Files:**
- Create: `pkg/script/handlers_debug.go`
- Create: `pkg/script/handlers_debug_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Create `pkg/script/handlers_debug.go`**

```go
package script

import (
	"errors"
	"fmt"
)

// handleError aborts the script with a scripted error message.
func handleError(s *ScriptState) error {
	msg := s.PopString()
	return fmt.Errorf("ERROR: %s", msg)
}

// handleGetTimeSpent / handleTimeSpent return the active player's playtime.
// TS exposes both names; they have identical behavior.
func handleGetTimeSpent(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("GETTIMESPENT: no active player")
	}
	s.PushInt(s.Self.Playtime())
	return nil
}

func handleTimeSpent(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("TIMESPENT: no active player")
	}
	s.PushInt(s.Self.Playtime())
	return nil
}
```

- [ ] **Step 2: Register 3 handlers in `pkg/script/handlers.go`**

```go
	OpError:        handleError,
	OpGetTimeSpent: handleGetTimeSpent,
	OpTimeSpent:    handleTimeSpent,
```

- [ ] **Step 3: Create `pkg/script/handlers_debug_test.go`**

```go
package script

import (
	"strings"
	"testing"
)

func TestErrorAborts(t *testing.T) {
	sf := &ScriptFile{
		Name:             "err",
		Opcodes:          []Opcode{OpPushConstantString, OpError, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"bad thing", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error")
	}
	if !strings.Contains(err.Error(), "bad thing") {
		t.Errorf("err msg: got %v, want containing 'bad thing'", err)
	}
	if state.Execution != Aborted {
		t.Errorf("Execution: got %v, want Aborted", state.Execution)
	}
}

func TestTimeSpentReturnsPlaytime(t *testing.T) {
	sf := &ScriptFile{
		Name:             "ts",
		Opcodes:          []Opcode{OpTimeSpent, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{username: "x"}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// mockPlayer doesn't track playtime; assert it returned 0 without error.
	if got := state.PopInt(); got != 0 {
		t.Errorf("TimeSpent: got %d, want 0", got)
	}
}
```

- [ ] **Step 4: Extend `mockPlayer` to implement `Playtime()`**

In `pkg/script/runner_test.go`, add to the existing `mockPlayer`:

```go
func (m *mockPlayer) Playtime() int { return m.playtime }
```

And add a `playtime int` field to the `mockPlayer` struct.

- [ ] **Step 5: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestError|TestTimeSpent' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_debug.go pkg/script/handlers_debug_test.go pkg/script/handlers.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5a DebugOps (ERROR + TIMESPENT + GETTIMESPENT)

ERROR aborts via a scripted error message that propagates up to the
runScript caller for logging. TIMESPENT/GETTIMESPENT return the active
player's tick-granularity playtime via ActivePlayer.Playtime. mockPlayer
gains a playtime field for unit tests.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Array ops + SWITCH (4 handlers)

**Files:**
- Create: `pkg/script/handlers_array.go`
- Create: `pkg/script/handlers_array_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Create `pkg/script/handlers_array.go`**

```go
package script

import "errors"

// ArrayCap is the maximum number of arrays per script (matches TS's
// compile-time limit).
const ArrayCap = 5

// handleDefineArray allocates a fresh []int32 of popped length at the
// operand-indexed slot. Re-defining overwrites.
func handleDefineArray(s *ScriptState) error {
	length := s.PopInt()
	slot := int(s.Script.IntOperands[s.PC])
	if slot < 0 || slot >= ArrayCap {
		return errors.New("DEFINE_ARRAY: slot out of range")
	}
	if length < 0 {
		length = 0
	}
	s.Arrays[slot] = make([]int32, length)
	return nil
}

// handlePushArrayInt reads Arrays[slot][idx]. Pushes 0 on OOB.
func handlePushArrayInt(s *ScriptState) error {
	idx := s.PopInt()
	slot := int(s.Script.IntOperands[s.PC])
	if slot < 0 || slot >= ArrayCap || idx < 0 || idx >= len(s.Arrays[slot]) {
		s.PushInt(0)
		return nil
	}
	s.PushInt(int(s.Arrays[slot][idx]))
	return nil
}

// handlePopArrayInt writes Arrays[slot][idx] = value. Silently drops on OOB.
func handlePopArrayInt(s *ScriptState) error {
	val := s.PopInt()
	idx := s.PopInt()
	slot := int(s.Script.IntOperands[s.PC])
	if slot < 0 || slot >= ArrayCap || idx < 0 || idx >= len(s.Arrays[slot]) {
		return nil
	}
	s.Arrays[slot][idx] = int32(val)
	return nil
}

// handleSwitch looks up the popped key in the per-instruction switch
// table and advances PC by the table's offset on hit. Falls through on
// miss.
func handleSwitch(s *ScriptState) error {
	key := int32(s.PopInt())
	tableIdx := int(s.Script.IntOperands[s.PC])
	if tableIdx < 0 || tableIdx >= len(s.Script.SwitchTables) {
		return nil // fall through
	}
	table := s.Script.SwitchTables[tableIdx]
	if offset, ok := table[key]; ok {
		s.PC += int(offset)
	}
	return nil
}
```

- [ ] **Step 2: Register 4 handlers in `pkg/script/handlers.go`**

```go
	OpDefineArray:  handleDefineArray,
	OpPushArrayInt: handlePushArrayInt,
	OpPopArrayInt:  handlePopArrayInt,
	OpSwitch:       handleSwitch,
```

- [ ] **Step 3: Create `pkg/script/handlers_array_test.go`**

```go
package script

import "testing"

func TestDefineAndReadArray(t *testing.T) {
	// Script: DEFINE_ARRAY slot=0 len=5, push 42 at idx 2, read idx 2, return
	sf := &ScriptFile{
		Name: "arr",
		Opcodes: []Opcode{
			OpPushConstantInt, // push length 5
			OpDefineArray,     // slot 0 = new [5]int32
			OpPushConstantInt, // push idx 2
			OpPushConstantInt, // push val 42
			OpPopArrayInt,     // arrays[0][2] = 42
			OpPushConstantInt, // push idx 2
			OpPushArrayInt,    // push arrays[0][2]
			OpReturn,
		},
		IntOperands:      []int32{5, 0, 2, 42, 0, 2, 0, 0},
		StringOperands:   []string{"", "", "", "", "", "", "", ""},
		InstructionCount: 8,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 42 {
		t.Errorf("arr[2]: got %d, want 42", got)
	}
}

func TestPushArrayIntOutOfBoundsReturnsZero(t *testing.T) {
	sf := &ScriptFile{
		Name: "oob",
		Opcodes: []Opcode{
			OpPushConstantInt, // push length 3
			OpDefineArray,     // slot 0
			OpPushConstantInt, // push idx 99 (OOB)
			OpPushArrayInt,
			OpReturn,
		},
		IntOperands:      []int32{3, 0, 99, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("oob read: got %d, want 0", got)
	}
}

func TestSwitchHitAndMiss(t *testing.T) {
	sf := &ScriptFile{
		Name: "sw",
		Opcodes: []Opcode{
			OpPushConstantInt, // 0: push key
			OpSwitch,          // 1: switch table[operand]
			OpPushConstantInt, // 2: push 111 (fell through)
			OpReturn,          // 3
			OpPushConstantInt, // 4: push 222 (branch taken target)
			OpReturn,          // 5
		},
		IntOperands:      []int32{7, 0, 111, 0, 222, 0},
		StringOperands:   []string{"", "", "", "", "", ""},
		InstructionCount: 6,
		SwitchTables: []SwitchTable{
			{7: 3}, // key 7 -> PC += 3 -> land at instr 4 after PC++
		},
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 222 {
		t.Errorf("switch hit: got %d, want 222", got)
	}

	// Miss case: change pushed key to a value not in the table.
	sf.IntOperands[0] = 99
	state2 := Init(sf, nil, false, nil, nil)
	if err := Execute(state2); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state2.PopInt(); got != 111 {
		t.Errorf("switch miss: got %d, want 111", got)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestDefine|TestPushArray|TestSwitch' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/handlers_array.go pkg/script/handlers_array_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5a array ops + SWITCH

DEFINE_ARRAY allocates a []int32 at operand-indexed slot (cap 5 per TS).
PUSH_ARRAY_INT / POP_ARRAY_INT read/write; OOB reads return 0, writes
are dropped. SWITCH looks up the popped key in Script.SwitchTables
[IntOperands[PC]] and advances PC on hit; falls through on miss.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: End-to-end Playtime integration test

**Files:**
- Modify: `modules/world/script_test.go`

- [ ] **Step 1: Append a Playtime test**

```go
func TestPlaytimeViaScriptMessageGame(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.playtime = 42

	// Script:
	//   pushstr "n="
	//   timespent       (pushes 42)
	//   append_num      (pops 42 + "n=", pushes "n=42")
	//   mes             (sends "n=42" on the wire)
	//   return
	sf := &script.ScriptFile{
		Name: "[timespent,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpTimeSpent,
			script.OpAppendNum,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0, 0, 0},
		StringOperands:   []string{"n=", "", "", "", ""},
		InstructionCount: 5,
	}

	received := drainConn(t, cc)
	s.runScript(sf, p, false, nil, nil)
	p.client.flushWrite()
	got := <-received

	// Expect MessageGame "n=42\n" payload (5 bytes) → wire = opcode + len + 5 = 7 bytes.
	if len(got) != 7 {
		t.Fatalf("wire: got %d bytes, want 7", len(got))
	}
	if string(got[2:6]) != "n=42" || got[6] != 0x0a {
		t.Errorf("payload: got %q, want 'n=42\\n'", got[2:])
	}
}
```

- [ ] **Step 2: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlaytime -v`
Expected: PASS.

- [ ] **Step 3: Full test + race + vet**

Run in order:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: all green.

- [ ] **Step 4: Commit**

```bash
git add modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): end-to-end S5a Playtime via AppendNum + Mes

Chains TIMESPENT → APPEND_NUM → MES to verify the cross-package wiring
from ActivePlayer.Playtime through the new NumberOps/StringOps handlers
and out to the wire as a MessageGame packet.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Checklist

After completing all tasks:

- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` — clean build
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` — all tests pass
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` — no race warnings
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` — no vet issues
- [ ] Count the `handlers` map entries and confirm the delta is exactly 49 (23 pre-S5a + 2 S4 + 49 S5a = 74 total).
