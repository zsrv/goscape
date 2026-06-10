package script

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"math/rand/v2"
)

// bitMask returns a mask covering bits [start..end] inclusive.
func bitMask(start, end int) int {
	width := end - start + 1
	if width <= 0 {
		return 0
	}
	return ((1 << width) - 1) << start
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
	// L25: TS MULTIPLY (NumberOps.ts:20-24) is `pushInt(a * b)` — the product is
	// formed in float64 and only then narrowed by toInt32 (`| 0`). Computing
	// int32(lhs)*int32(rhs) wraps mod 2^32 exactly, which diverges from TS once
	// the true product exceeds 2^53 (float64 loses precision before the mask).
	// Mirror TS: float64 product → floatToInt32 (JS ToInt32).
	s.PushInt(floatToInt32(float64(lhs) * float64(rhs)))
	return nil
}

func handleDivide(s *ScriptState) error {
	rhs := s.PopInt()
	lhs := s.PopInt()
	if rhs == 0 {
		return errors.New("DIVIDE: division by zero")
	}
	// M15: TS DIVIDE (NumberOps.ts:26) is `pushInt(a / b)` → toInt32 truncates
	// toward zero. Go's integer `/` truncates toward zero too (e.g. -7/2 = -3),
	// so use it directly — the old floorDiv rounded toward -inf (-7/2 = -4).
	s.PushInt(lhs / rhs)
	return nil
}

func handleModulo(s *ScriptState) error {
	rhs := s.PopInt()
	lhs := s.PopInt()
	if rhs == 0 {
		return errors.New("MODULO: division by zero")
	}
	// M16: TS MODULO (NumberOps.ts:69) is `pushInt(n1 % n2)` — JS `%` is the
	// truncated remainder (sign follows the dividend, e.g. -7 % 3 = -1). Go's
	// `%` is identical; the old posMod returned a Euclidean-positive result
	// (-7 mod 3 = 2).
	s.PushInt(lhs % rhs)
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
	// SCALE: TS NumberOps.ts:124-127 — pushInt((a*c)/b).
	// Runescript scale(value, max, newMax) → value*newMax/max.
	c := s.PopInt()
	b := s.PopInt()
	a := s.PopInt()
	if b == 0 {
		return errors.New("SCALE: division by zero")
	}
	// M17: TS SCALE (NumberOps.ts:124) is `pushInt((a * c) / b)` → toInt32
	// truncates toward zero. Use Go integer division (truncating) rather than
	// the old floorDiv (toward -inf).
	s.PushInt((a * c) / b)
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
	// L25: TS POW (NumberOps.ts:74-77) is `pushInt(Math.pow(base, exponent))` —
	// computed in float64 then narrowed by toInt32. The old int32 multiply loop
	// diverged two ways: (1) results above 2^53 wrap mod 2^32 exactly instead of
	// tracking float64's rounded value, and (2) the `exp < 0 → 0` shortcut is
	// wrong for base ±1 (Math.pow(1,-5)=1, Math.pow(-1,-2)=1 → toInt32 keeps 1).
	// Mirror TS directly: math.Pow → floatToInt32 (JS ToInt32).
	s.PushInt(floatToInt32(math.Pow(float64(base), float64(exp))))
	return nil
}

// handleInvPow computes invpow(n1, n2) = the n2-th root of n1, truncated
// toward zero. Mirrors TS NumberOps.ts:79-100 exactly: sqrt for n2==2,
// cbrt for n2==3, sqrt(sqrt) for n2==4, and pow(n1, 1/n2) otherwise.
// invpow is the inverse of pow (base^exp), NOT a logarithm.
func handleInvPow(s *ScriptState) error {
	n2 := s.PopInt()
	n1 := s.PopInt()
	if n1 == 0 || n2 == 0 {
		s.PushInt(0)
		return nil
	}
	switch n2 {
	case 1:
		s.PushInt(n1)
	case 2:
		s.PushInt(floatToInt32(math.Sqrt(float64(n1))))
	case 3:
		s.PushInt(floatToInt32(math.Cbrt(float64(n1))))
	case 4:
		s.PushInt(floatToInt32(math.Sqrt(math.Sqrt(float64(n1)))))
	default:
		s.PushInt(floatToInt32(math.Pow(float64(n1), 1.0/float64(n2))))
	}
	return nil
}

// floatToInt32 truncates f toward zero and wraps to int32, mirroring JS's
// ToInt32 (`x | 0`). NaN and ±Inf map to 0, matching `NaN | 0 === 0` —
// e.g. Math.sqrt of a negative in TS yields NaN, then 0.
func floatToInt32(f float64) int {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return int(int32(int64(math.Trunc(f))))
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
//
// TS uses the shared JavaRandom PRNG (java.util.Random port); goscape uses
// math/rand/v2 — sequences differ but the distribution contract is the same
// (uniform over [0, bound)). 254 (TS NumberOps.ts:32-40 @43e02957) drops the
// 245.2 Math.max(0,·) clamp, exposing JavaRandom.nextInt's bound guard:
// checkIsPositiveInt throws RangeError on bound < 0 (JavaRandom.ts:58-62),
// while bound == 0 does NOT throw — it satisfies the power-of-two branch
// ((0 & -0) === 0, JavaRandom.ts:93-96) and returns 0.

func handleRandom(s *ScriptState) error {
	n := s.PopInt()
	if n < 0 {
		// TS: JavaRandom.nextInt(n) throws RangeError (JavaRandom.ts:58-62).
		return fmt.Errorf("RANDOM: bound must be non-negative: %d", n)
	}
	if n == 0 {
		// TS: nextInt(0) hits the power-of-two branch and returns 0;
		// Go rand.IntN(0) would panic, so special-case it.
		s.PushInt(0)
		return nil
	}
	s.PushInt(rand.IntN(n))
	return nil
}

func handleRandomInc(s *ScriptState) error {
	n := s.PopInt()
	if n+1 < 0 {
		// TS: JavaRandom.nextInt(n+1) throws RangeError (JavaRandom.ts:58-62).
		return fmt.Errorf("RANDOMINC: bound must be non-negative: n=%d, bound=n+1=%d", n, n+1)
	}
	if n+1 == 0 {
		// TS: nextInt(0) returns 0 (power-of-two branch); avoid IntN(0) panic.
		s.PushInt(0)
		return nil
	}
	s.PushInt(rand.IntN(n + 1))
	return nil
}

// -- Trig + INTERPOLATE (S5j) --
//
// Trig uses 16384-step "RuneScript degrees" (a full circle = 16384).
// Sin/cos return values are scaled by 16384 to give integer fixed-point
// output. Atan2 returns the same 16384-step degree representation.

const trigScale = 16384.0

// trigStep is the per-step angle in radians, copied verbatim from TS Trig
// (src/util/Trig.ts:6 — `const size = 3.834951969714103e-4`). TS uses this
// exact float literal (NOT a recomputed 2π/16384) to build its sin/cos table,
// so reproducing the literal keeps the argument bit-identical.
const trigStep = 3.834951969714103e-4

// atan2Scale = 16384 / (2π) — converts radians to RuneScript degrees.
var atan2Scale = trigScale / (2 * math.Pi)

// handleSinDeg pops a 16384-step angle and pushes trunc(sin(angle) * 16384).
// M18: TS Trig._sin (Trig.ts:8) is `(Math.sin(index * size) * 16384.0) | 0` —
// truncation toward zero, not rounding. goscape's table is computed on the fly
// with the same per-index formula, so int() (truncating) replaces math.Round.
func handleSinDeg(s *ScriptState) error {
	angle := s.PopInt() & 0x3fff
	s.PushInt(int(math.Sin(float64(angle)*trigStep) * trigScale))
	return nil
}

// handleCosDeg pops a 16384-step angle and pushes trunc(cos(angle) * 16384).
// M18: mirrors TS Trig._cos (Trig.ts:9) `(Math.cos(index * size) * 16384.0) | 0`.
func handleCosDeg(s *ScriptState) error {
	angle := s.PopInt() & 0x3fff
	s.PushInt(int(math.Cos(float64(angle)*trigStep) * trigScale))
	return nil
}

// handleAtan2Deg pops (y, x) (x on top), pushes the 16384-step
// representation of atan2(y, x), masked to the [0, 16383] range.
func handleAtan2Deg(s *ScriptState) error {
	x := s.PopInt()
	y := s.PopInt()
	res := int(math.Round(math.Atan2(float64(y), float64(x))*atan2Scale)) & 0x3fff
	s.PushInt(res)
	return nil
}

// handleInterpolate pops [y0, y1, x0, x1, x] (x on top) and pushes
// floor((y1-y0)/(x1-x0)) * (x-x0) + y0, matching TS NumberOps.ts:42-47
// INTERPOLATE (floor applies to the slope BEFORE multiplying by the run).
//
// L24: computed in float64 to mirror TS exactly. TS has no div-by-zero guard:
// when x1==x0 the slope is ±Infinity (or NaN when y1==y0), and the final
// pushInt's toInt32 maps Inf/NaN to 0 — so the de-facto TS result is 0, not the
// y0 the prior Go guard returned. The float64 path reproduces that naturally
// (floatToInt32 of ±Inf/NaN == 0) and also tracks TS's float rounding for the
// >2^53 product range, where an integer floor-div*run would wrap differently.
func handleInterpolate(s *ScriptState) error {
	x := s.PopInt()
	x1 := s.PopInt()
	x0 := s.PopInt()
	y1 := s.PopInt()
	y0 := s.PopInt()
	lerp := math.Floor(float64(y1-y0)/float64(x1-x0))*float64(x-x0) + float64(y0)
	s.PushInt(floatToInt32(lerp))
	return nil
}

// -- S5k: coord unpack + distance --
//
// Coords pack as (level << 28) | (x << 14) | z with 14-bit x/z and 4-bit
// level. TS calls the level "y" (so COORDY returns the level). All three
// COORD* handlers pop a packed coord and push one component.

// L18: COORD*/DISTANCE validate the packed coord with checkCoord (TS
// CoordValid) before unpacking — a negative (out-of-range, incl. the -1 null
// sentinel) aborts the script, where the prior raw bit-unpack silently masked.
func handleCoordX(s *ScriptState) error {
	_, x, _, err := checkCoord(s.PopInt(), "COORDX")
	if err != nil {
		return err
	}
	s.PushInt(x)
	return nil
}

// handleCoordY pushes the level. TS naming convention: "y" = vertical
// plane (level 0..3), not the world-space Y axis.
func handleCoordY(s *ScriptState) error {
	level, _, _, err := checkCoord(s.PopInt(), "COORDY")
	if err != nil {
		return err
	}
	s.PushInt(level)
	return nil
}

func handleCoordZ(s *ScriptState) error {
	_, _, z, err := checkCoord(s.PopInt(), "COORDZ")
	if err != nil {
		return err
	}
	s.PushInt(z)
	return nil
}

// handleDistance pops two packed coords and pushes the king-move
// (Chebyshev) distance — max(|dx|, |dz|). Matches TS CoordGrid.distanceToSW.
// Pop order: popInts(2) = [c1, c2] with c2 on top; TS validates c1 then c2.
func handleDistance(s *ScriptState) error {
	c2 := s.PopInt()
	c1 := s.PopInt()
	_, x1, z1, err := checkCoord(c1, "DISTANCE")
	if err != nil {
		return err
	}
	_, x2, z2, err := checkCoord(c2, "DISTANCE")
	if err != nil {
		return err
	}
	dx := x1 - x2
	if dx < 0 {
		dx = -dx
	}
	dz := z1 - z2
	if dz < 0 {
		dz = -dz
	}
	if dx > dz {
		s.PushInt(dx)
	} else {
		s.PushInt(dz)
	}
	return nil
}
