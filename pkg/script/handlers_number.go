package script

import (
	"errors"
	"math"
	"math/bits"
	"math/rand/v2"
)

// floorDiv returns floor(a / b), matching TS's Math.floor(a/b). Panics
// on zero divisor; callers must pre-check and return an error.
func floorDiv(a, b int) int {
	q := a / b
	// Go truncates toward zero; floor division rounds toward -inf.
	// Adjust when the quotient is negative and there is a non-zero
	// remainder.
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// posMod returns a mathematical modulus that is always non-negative
// when b > 0, matching TS's ((a%b)+b)%b idiom.
func posMod(a, b int) int {
	r := a % b
	if r < 0 && b > 0 {
		r += b
	} else if r > 0 && b < 0 {
		r += b
	}
	return r
}

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
	// invpow(value, base) = floor(log_base(value)).
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

// -- Trig + INTERPOLATE (S5j) --
//
// Trig uses 16384-step "RuneScript degrees" (a full circle = 16384).
// Sin/cos return values are scaled by 16384 to give integer fixed-point
// output. Atan2 returns the same 16384-step degree representation.

const trigScale = 16384.0

// atan2Scale = 16384 / (2π) — converts radians to RuneScript degrees.
var atan2Scale = trigScale / (2 * math.Pi)

// handleSinDeg pops a 16384-step angle and pushes int(round(sin(angle) * 16384)).
func handleSinDeg(s *ScriptState) error {
	angle := s.PopInt() & 0x3fff
	rad := float64(angle) / trigScale * 2 * math.Pi
	s.PushInt(int(math.Round(math.Sin(rad) * trigScale)))
	return nil
}

// handleCosDeg pops a 16384-step angle and pushes int(round(cos(angle) * 16384)).
func handleCosDeg(s *ScriptState) error {
	angle := s.PopInt() & 0x3fff
	rad := float64(angle) / trigScale * 2 * math.Pi
	s.PushInt(int(math.Round(math.Cos(rad) * trigScale)))
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

// handleInterpolate pops [y0, y1, x0, x1, x] (x on top) and pushes the
// linear interpolation y = y0 + (y1-y0) * (x-x0) / (x1-x0). Floor division
// is used to match TS Math.floor semantics. Returns y0 if x1==x0 to avoid
// div-by-zero (TS doesn't guard but cache scripts shouldn't hit this).
func handleInterpolate(s *ScriptState) error {
	x := s.PopInt()
	x1 := s.PopInt()
	x0 := s.PopInt()
	y1 := s.PopInt()
	y0 := s.PopInt()
	if x1 == x0 {
		s.PushInt(y0)
		return nil
	}
	s.PushInt(floorDiv((y1-y0)*(x-x0), x1-x0) + y0)
	return nil
}

// -- S5k: coord unpack + distance --
//
// Coords pack as (level << 28) | (x << 14) | z with 14-bit x/z and 4-bit
// level. TS calls the level "y" (so COORDY returns the level). All three
// COORD* handlers pop a packed coord and push one component.

func handleCoordX(s *ScriptState) error {
	c := s.PopInt()
	s.PushInt((c >> 14) & 0x3fff)
	return nil
}

// handleCoordY pushes the level. TS naming convention: "y" = vertical
// plane (level 0..3), not the world-space Y axis.
func handleCoordY(s *ScriptState) error {
	c := s.PopInt()
	s.PushInt((c >> 28) & 0x3)
	return nil
}

func handleCoordZ(s *ScriptState) error {
	c := s.PopInt()
	s.PushInt(c & 0x3fff)
	return nil
}

// handleDistance pops two packed coords and pushes the king-move
// (Chebyshev) distance — max(|dx|, |dz|). Matches TS CoordGrid.distanceToSW.
// Pop order: popInts(2) = [c1, c2] with c2 on top.
func handleDistance(s *ScriptState) error {
	c2 := s.PopInt()
	c1 := s.PopInt()
	x1 := (c1 >> 14) & 0x3fff
	z1 := c1 & 0x3fff
	x2 := (c2 >> 14) & 0x3fff
	z2 := c2 & 0x3fff
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
