# Sub-spec RuneScript S5j: Math Completion — Design

**Status:** Draft → ready for plan (combined here, since scope is 4 handlers)
**Scope:** 4 deferred-from-S5a math handlers: SIN_DEG, COS_DEG, ATAN2_DEG, INTERPOLATE. Pure VM, no entity state. ~150 LOC.
**Out of scope:** Anything that needs entity state, wire ops, or new VM machinery.

## Rationale

S5a deferred trig + interpolate as "uncommon in cache". Closing the gap now is a tiny, safe, no-risk addition that completes the math handler surface.

## Components

### Handlers — extend `pkg/script/handlers_number.go`

```go
import "math"

const trigScale = 16384.0
// 16384 / (2*pi) for ATAN2_DEG result encoding.
const atan2Scale = trigScale / (2 * math.Pi)

func handleSinDeg(s *ScriptState) error {
    angle := s.PopInt() & 0x3fff
    rad := float64(angle) / trigScale * 2 * math.Pi
    s.PushInt(int(math.Round(math.Sin(rad) * trigScale)))
    return nil
}

func handleCosDeg(s *ScriptState) error {
    angle := s.PopInt() & 0x3fff
    rad := float64(angle) / trigScale * 2 * math.Pi
    s.PushInt(int(math.Round(math.Cos(rad) * trigScale)))
    return nil
}

func handleAtan2Deg(s *ScriptState) error {
    // TS Trig.atan2(y, x) — popInts(2) = [y, x] with x on top.
    x := s.PopInt()
    y := s.PopInt()
    res := int(math.Round(math.Atan2(float64(y), float64(x))*atan2Scale)) & 0x3fff
    s.PushInt(res)
    return nil
}

func handleInterpolate(s *ScriptState) error {
    // TS popInts(5) = [y0, y1, x0, x1, x] with x on top.
    x := s.PopInt()
    x1 := s.PopInt()
    x0 := s.PopInt()
    y1 := s.PopInt()
    y0 := s.PopInt()
    if x1 == x0 {
        // Avoid div-by-zero; TS doesn't guard but cache scripts don't hit this.
        s.PushInt(y0)
        return nil
    }
    s.PushInt(floorDiv((y1-y0)*(x-x0), x1-x0) + y0)
    return nil
}
```

`floorDiv` already exists from S5a.

### Registration — `pkg/script/handlers.go`

Add at end of map:
```go
// S5j: math completion (trig + interpolate).
OpSinDeg:      handleSinDeg,
OpCosDeg:      handleCosDeg,
OpAtan2Deg:    handleAtan2Deg,
OpInterpolate: handleInterpolate,
```

### Tests — extend `pkg/script/handlers_number_test.go`

Append:
```go
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
    // atan2(0, 1) = 0 → 0
    got := runSingleOp(t, OpAtan2Deg, []int{0, 1}) // y=0, x=1
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
```

## LOC

| File | LOC |
|---|---|
| `pkg/script/handlers_number.go` (diff) | +60 |
| `pkg/script/handlers.go` (diff) | +6 |
| `pkg/script/handlers_number_test.go` (diff) | +80 |
| **Total** | **~146** |

## Plan

Single commit:

```bash
git add pkg/script/handlers_number.go pkg/script/handlers_number_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "S5j math completion: SIN_DEG + COS_DEG + ATAN2_DEG + INTERPOLATE"
```

Handler count after: 197 (193 + 4).
