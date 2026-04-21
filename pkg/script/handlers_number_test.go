package script

import (
	"testing"
)

// runSingleOp builds a one-instruction script, pushes `inputs` onto the
// int stack bottom-to-top, executes the op + return, and returns the
// top of the int stack.
func runSingleOp(t *testing.T, op Opcode, inputs []int) int {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + op.String(),
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
		t.Fatalf("%s: unexpected error: %v", op.String(), err)
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
	cases := []struct {
		name        string
		op          Opcode
		lhs, rhs    int
		branchTaken int // 0 = fell through, 1 = branched
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
					OpReturn,          // 4
					OpPushConstantInt, // 5: push 1 (branch taken)
					OpReturn,          // 6
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
