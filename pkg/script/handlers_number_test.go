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
		// M15: DIVIDE truncates toward zero (TS pushInt(a/b) → toInt32), not floor.
		{"divide trunc neg", OpDivide, []int{-7, 2}, -3},
		{"modulo pos", OpModulo, []int{10, 3}, 1},
		// M16: MODULO is the truncated remainder (sign of dividend), TS `n1 % n2`.
		{"modulo neg dividend", OpModulo, []int{-7, 3}, -1},
		{"modulo neg divisor", OpModulo, []int{7, -3}, 1},
		{"abs neg", OpAbs, []int{-9}, 9},
		{"abs pos", OpAbs, []int{7}, 7},
		{"addpercent +50%", OpAddPercent, []int{100, 50}, 150},
		{"scale (100,100,1) smoke trace", OpScale, []int{100, 100, 1}, 1},
		{"scale value*newMax/max", OpScale, []int{200, 4, 3}, 150},
		{"min", OpMin, []int{5, 3}, 3},
		{"max", OpMax, []int{5, 3}, 5},
		{"pow 2^10", OpPow, []int{2, 10}, 1024},
		{"pow 0 exp", OpPow, []int{5, 0}, 1},
		{"pow neg exp", OpPow, []int{5, -1}, 0}, // Math.pow(5,-1)=0.2 → toInt32 0
		// L25: POW/MULTIPLY computed in float64 then narrowed by toInt32 (JS `| 0`),
		// matching TS Math.pow + pushInt. Base ±1 with a negative exponent yields
		// 1/-1 — the old `exp<0 → 0` shortcut produced 0; results above int32 wrap
		// via toInt32 exactly as JS does.
		{"pow 1^-5 base-1 identity", OpPow, []int{1, -5}, 1},
		{"pow -1^-2", OpPow, []int{-1, -2}, 1},
		{"pow -1^-3", OpPow, []int{-1, -3}, -1},
		{"pow 2^31 toInt32 wrap", OpPow, []int{2, 31}, -2147483648},
		{"pow 2^32 toInt32 zero", OpPow, []int{2, 32}, 0},
		{"multiply 1e5*1e5 toInt32", OpMultiply, []int{100000, 100000}, 1410065408},
		// INVPOW(n1, n2) = floor(n1 ^ (1/n2)) — the n2-th root of n1,
		// matching TS NumberOps.ts:79-100 (sqrt at n2==2, cbrt at n2==3,
		// 4th-root at n2==4, general pow(n1, 1/n2) otherwise). NOT a
		// logarithm.
		{"invpow sqrt 1024", OpInvPow, []int{1024, 2}, 32},
		{"invpow sqrt 100", OpInvPow, []int{100, 2}, 10},
		{"invpow sqrt 1000 truncates", OpInvPow, []int{1000, 2}, 31},
		{"invpow cbrt 27", OpInvPow, []int{27, 3}, 3},
		{"invpow cbrt neg 8", OpInvPow, []int{-8, 3}, -2},
		{"invpow 4th root 16", OpInvPow, []int{16, 4}, 2},
		{"invpow base 1 identity", OpInvPow, []int{50, 1}, 50},
		{"invpow general 5th root", OpInvPow, []int{100, 5}, 2},
		{"invpow zero value", OpInvPow, []int{0, 2}, 0},
		{"invpow zero exponent", OpInvPow, []int{8, 0}, 0},
		{"invpow sqrt neg is zero", OpInvPow, []int{-9, 2}, 0},

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

func TestScaleDivideByZeroAborts(t *testing.T) {
	sf := &ScriptFile{
		Name:             "scalezero",
		Opcodes:          []Opcode{OpScale, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	// scale(value=100, max=0, newMax=5) — divisor is max (the second-popped operand).
	state.PushInt(100)
	state.PushInt(0)
	state.PushInt(5)
	if err := Execute(state); err == nil {
		t.Fatal("Execute: want error on SCALE divide by zero")
	}
	if state.Execution != Aborted {
		t.Errorf("Execution: got %v, want Aborted", state.Execution)
	}
}

// runSingleOpExpectAbort builds a one-instruction script like runSingleOp
// but expects Execute to fail and the script to abort.
func runSingleOpExpectAbort(t *testing.T, op Opcode, inputs []int) {
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
	if err := Execute(state); err == nil {
		t.Fatalf("%s(%v): want error, got nil", op.String(), inputs)
	}
	if state.Execution != Aborted {
		t.Errorf("%s(%v): Execution: got %v, want Aborted", op.String(), inputs, state.Execution)
	}
}

// TestRandomNegativeBoundTruncatesTowardZero — the 254 pin moves RANDOM
// off nextInt onto nextDouble (TS NumberOps.ts:31-34 @2e3bcf43):
// pushInt(nextDouble() * n) with toInt32 truncation. For n < 0 the
// product lies in (n, 0] and truncates toward zero, so the result is
// uniform over {n+1, ..., 0} — no RangeError (the 43e02957-era
// nextInt bound guard no longer runs).
func TestRandomNegativeBoundTruncatesTowardZero(t *testing.T) {
	for range 100 {
		got := runSingleOp(t, OpRandom, []int{-5})
		if got < -4 || got > 0 {
			t.Errorf("OpRandom(-5): got %d, want [-4..0] (nextDouble()*-5 truncated toward zero)", got)
		}
	}
	if got := runSingleOp(t, OpRandom, []int{-1}); got != 0 {
		t.Errorf("OpRandom(-1): got %d, want 0 (nextDouble()*-1 ∈ (-1,0] truncates to 0)", got)
	}
}

// TestRandomZeroBoundPushesZero — random(0) → nextDouble()*0 → 0.
func TestRandomZeroBoundPushesZero(t *testing.T) {
	if got := runSingleOp(t, OpRandom, []int{0}); got != 0 {
		t.Errorf("OpRandom(0): got %d, want 0 (nextDouble()*0)", got)
	}
}

// TestRandomIncNegativeBoundTruncatesTowardZero — randominc(n) pushes
// toInt32(nextDouble() * (n+1)) at the 254 pin (TS NumberOps.ts:36-39
// @2e3bcf43); n <= -2 yields a negative bound, uniform over {n+2..0}.
func TestRandomIncNegativeBoundTruncatesTowardZero(t *testing.T) {
	for range 100 {
		got := runSingleOp(t, OpRandomInc, []int{-6})
		if got < -4 || got > 0 {
			t.Errorf("OpRandomInc(-6): got %d, want [-4..0] (nextDouble()*-5 truncated)", got)
		}
	}
	if got := runSingleOp(t, OpRandomInc, []int{-2}); got != 0 {
		t.Errorf("OpRandomInc(-2): got %d, want 0 (nextDouble()*-1 truncates to 0)", got)
	}
}

// TestRandomIncBoundaryPushesZero — randominc(-1) → nextDouble()*0 → 0;
// randominc(0) → toInt32(nextDouble()*1) → 0.
func TestRandomIncBoundaryPushesZero(t *testing.T) {
	if got := runSingleOp(t, OpRandomInc, []int{-1}); got != 0 {
		t.Errorf("OpRandomInc(-1): got %d, want 0 (nextDouble()*0)", got)
	}
	if got := runSingleOp(t, OpRandomInc, []int{0}); got != 0 {
		t.Errorf("OpRandomInc(0): got %d, want 0 (toInt32(nextDouble()*1))", got)
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

// -- S5j: trig + INTERPOLATE --

func TestSinDegZero(t *testing.T) {
	got := runSingleOp(t, OpSinDeg, []int{0})
	if got != 0 {
		t.Errorf("SIN_DEG(0): got %d, want 0", got)
	}
}

func TestSinDegQuarter(t *testing.T) {
	// 90° in 16384-units = 4096; sin(90°)*16384 = 16384.
	got := runSingleOp(t, OpSinDeg, []int{4096})
	if got != 16384 {
		t.Errorf("SIN_DEG(4096): got %d, want 16384", got)
	}
}

func TestCosDegZero(t *testing.T) {
	got := runSingleOp(t, OpCosDeg, []int{0})
	if got != 16384 {
		t.Errorf("COS_DEG(0): got %d, want 16384", got)
	}
}

func TestAtan2DegRight(t *testing.T) {
	// atan2(0, 1) = 0 → 0 (pointing along +x axis).
	got := runSingleOp(t, OpAtan2Deg, []int{0, 1}) // y=0 (bottom), x=1 (top)
	if got != 0 {
		t.Errorf("ATAN2(0,1): got %d, want 0", got)
	}
}

func TestAtan2DegUp(t *testing.T) {
	// atan2(1, 0) = π/2 → 4096
	got := runSingleOp(t, OpAtan2Deg, []int{1, 0})
	if got != 4096 {
		t.Errorf("ATAN2(1,0): got %d, want 4096", got)
	}
}

func TestInterpolateLinear(t *testing.T) {
	// y0=0, y1=10, x0=0, x1=10, x=5 → 5
	got := runSingleOp(t, OpInterpolate, []int{0, 10, 0, 10, 5})
	if got != 5 {
		t.Errorf("INTERPOLATE(0,10,0,10,5): got %d, want 5", got)
	}
}

func TestInterpolateAtEnd(t *testing.T) {
	// y0=0, y1=100, x0=0, x1=10, x=10 → 100
	got := runSingleOp(t, OpInterpolate, []int{0, 100, 0, 10, 10})
	if got != 100 {
		t.Errorf("INTERPOLATE(0,100,0,10,10): got %d, want 100", got)
	}
}

// TestInterpolateDivZeroReturnsZero pins the TS de-facto result for x1==x0
// (L24). TS has no div-by-zero guard: the slope is +Inf (y1>y0) and
// Inf*(x-x0)+y0 → ±Inf or NaN, which pushInt's toInt32 maps to 0. The prior
// Go guard returned y0 (42), which TS never produces.
func TestInterpolateDivZeroReturnsZero(t *testing.T) {
	got := runSingleOp(t, OpInterpolate, []int{42, 99, 5, 5, 5})
	if got != 0 {
		t.Errorf("INTERPOLATE div-zero: got %d, want 0 (TS Inf/NaN→toInt32)", got)
	}
}

// TestInterpolateDivZeroEqualEndpoints covers the y1==y0 sub-case where the
// slope is 0/0 = NaN (not Inf); toInt32(NaN) is also 0.
func TestInterpolateDivZeroEqualEndpoints(t *testing.T) {
	got := runSingleOp(t, OpInterpolate, []int{7, 7, 5, 5, 9})
	if got != 0 {
		t.Errorf("INTERPOLATE div-zero NaN: got %d, want 0", got)
	}
}

// TestInterpolateNonIntegerSlopeMatchesTS pins TS operator precedence
// floor((y1-y0)/(x1-x0)) * (x-x0) + y0. For y0=0, y1=10, x0=0, x1=3, x=5
// TS yields floor(10/3) * 5 + 0 = 3 * 5 = 15. A "natural" precedence
// floor((y1-y0)*(x-x0) / (x1-x0)) + y0 would yield floor(50/3) = 16.
func TestInterpolateNonIntegerSlopeMatchesTS(t *testing.T) {
	got := runSingleOp(t, OpInterpolate, []int{0, 10, 0, 3, 5})
	if got != 15 {
		t.Errorf("INTERPOLATE(0,10,0,3,5): got %d, want 15 (TS precedence)", got)
	}
}

// TestInterpolateNegativeSlope confirms floor-toward-minus-infinity
// matches TS Math.floor for negative inner quotients.
// y0=10, y1=0, x0=0, x1=3, x=5 → floor(-10/3) * 5 + 10 = -4*5 + 10 = -10.
func TestInterpolateNegativeSlope(t *testing.T) {
	got := runSingleOp(t, OpInterpolate, []int{10, 0, 0, 3, 5})
	if got != -10 {
		t.Errorf("INTERPOLATE(10,0,0,3,5): got %d, want -10", got)
	}
}

// TestInterpolateExtrapolateBelow checks x < x0 extrapolation.
// y0=0, y1=10, x0=0, x1=3, x=-2 → floor(10/3) * -2 + 0 = 3*-2 = -6.
func TestInterpolateExtrapolateBelow(t *testing.T) {
	got := runSingleOp(t, OpInterpolate, []int{0, 10, 0, 3, -2})
	if got != -6 {
		t.Errorf("INTERPOLATE(0,10,0,3,-2): got %d, want -6", got)
	}
}

// TestInterpolateExactEndpointX1 checks endpoint match where the TS
// precedence loses information (floor((y1-y0)/(x1-x0))*(x1-x0) does NOT
// generally equal y1-y0) — bug or not, parity with TS is the contract.
// y0=0, y1=10, x0=0, x1=3, x=3 → floor(10/3)*3 + 0 = 3*3 = 9 (NOT 10).
func TestInterpolateExactEndpointX1FollowsTS(t *testing.T) {
	got := runSingleOp(t, OpInterpolate, []int{0, 10, 0, 3, 3})
	if got != 9 {
		t.Errorf("INTERPOLATE(0,10,0,3,3): got %d, want 9 (TS lossy precedence)", got)
	}
}

// -- S5k: coord unpack + distance --

func TestCoordX(t *testing.T) {
	// pack (level=0, x=3222, z=999)
	c := (0 << 28) | (3222 << 14) | 999
	got := runSingleOp(t, OpCoordX, []int{c})
	if got != 3222 {
		t.Errorf("COORDX: got %d, want 3222", got)
	}
}

func TestCoordY(t *testing.T) {
	// COORDY returns the level (TS calls plane "y").
	c := (3 << 28) | (3222 << 14) | 999
	got := runSingleOp(t, OpCoordY, []int{c})
	if got != 3 {
		t.Errorf("COORDY: got %d, want 3 (level)", got)
	}
}

func TestCoordZ(t *testing.T) {
	c := (0 << 28) | (3222 << 14) | 999
	got := runSingleOp(t, OpCoordZ, []int{c})
	if got != 999 {
		t.Errorf("COORDZ: got %d, want 999", got)
	}
}

func TestDistance(t *testing.T) {
	// Two coords at same level: (3222, 999) and (3220, 1004) → max(2, 5) = 5.
	c1 := (0 << 28) | (3222 << 14) | 999
	c2 := (0 << 28) | (3220 << 14) | 1004
	got := runSingleOp(t, OpDistance, []int{c1, c2})
	if got != 5 {
		t.Errorf("DISTANCE: got %d, want 5", got)
	}
}

func TestDistanceSameCoord(t *testing.T) {
	c := (0 << 28) | (3222 << 14) | 999
	got := runSingleOp(t, OpDistance, []int{c, c})
	if got != 0 {
		t.Errorf("DISTANCE same: got %d, want 0", got)
	}
}

// TestCoordOpsAbortOnInvalidCoord pins L18: COORDX/Y/Z, DISTANCE, INZONE,
// MOVECOORD validate the packed coord with checkCoord (TS CoordValid, range
// [0, 2^31-1]) and abort on a negative/out-of-range input rather than silently
// bit-masking. Inputs are pushed bottom-to-top in TS popInts order.
func TestCoordOpsAbortOnInvalidCoord(t *testing.T) {
	cases := []struct {
		name   string
		op     Opcode
		inputs []int
	}{
		{"COORDX", OpCoordX, []int{-1}},
		{"COORDY", OpCoordY, []int{-1}},
		{"COORDZ", OpCoordZ, []int{-1}},
		{"DISTANCE/c1", OpDistance, []int{-1, 0}},
		{"DISTANCE/c2", OpDistance, []int{0, -1}},
		{"MOVECOORD", OpMoveCoord, []int{-1, 0, 0, 0}},   // [coord, x, y, z]
		{"INZONE/from", OpInZone, []int{-1, 0, 0}},       // [from, to, pos]
		{"INZONE/pos", OpInZone, []int{0, 0, -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sf := &ScriptFile{
				Name:             "bad_" + tc.op.String(),
				Opcodes:          []Opcode{tc.op, OpReturn},
				IntOperands:      []int32{0, 0},
				StringOperands:   []string{"", ""},
				InstructionCount: 2,
			}
			state := Init(sf, nil, false, nil, nil)
			for _, v := range tc.inputs {
				state.PushInt(v)
			}
			if err := Execute(state); err == nil {
				t.Fatalf("%s: Execute returned nil, want abort on invalid coord", tc.name)
			}
		})
	}
}
