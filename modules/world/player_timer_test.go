package world

import "testing"

// TestPlayerGetTimerReturnsClock pins NAI-27 Bundle 2: GetTimer
// returns the absolute Clock tick (TS-faithful per Player.ts:910 +
// PlayerOps.ts:858), NOT the pre-NAI-27 "(Clock+Interval)-now"
// remaining-ticks computation. The negative pin: passing the test
// after a future regression to the remaining-ticks formula would
// require advance-clock < (Clock+Interval), which the assertion below
// with currentTick=25, Clock=10, Interval=10 would compute as -5
// (not 10).
func TestPlayerGetTimerReturnsClock(t *testing.T) {
	p := &Player{}
	p.timers = map[uint32]*playerTimer{
		42: {
			ScriptID: 42,
			Interval: 10,
			Clock:    10,
		},
	}
	got := p.GetTimer(42)
	if got != 10 {
		t.Errorf("GetTimer: got %d, want 10 (absolute Clock per TS Player.ts:910 / PlayerOps.ts:858; not (Clock+Interval)-now=-5 nor any other arithmetic)", got)
	}
}

// TestPlayerGetTimerNotFoundReturnsMinusOne pins the -1 sentinel for
// unset scriptIDs (TS PlayerOps.ts:863 pushInt(-1) fallthrough).
func TestPlayerGetTimerNotFoundReturnsMinusOne(t *testing.T) {
	p := &Player{}
	got := p.GetTimer(0xDEADBEEF)
	if got != -1 {
		t.Errorf("GetTimer: got %d, want -1 (no timer registered → TS PlayerOps.ts:863)", got)
	}
}

// TestPlayerGetTimerNilTimersMapReturnsMinusOne pins the nil-map fast
// path before the lookup attempt.
func TestPlayerGetTimerNilTimersMapReturnsMinusOne(t *testing.T) {
	p := &Player{}
	if p.timers != nil {
		t.Fatalf("setup: p.timers should be nil")
	}
	got := p.GetTimer(0)
	if got != -1 {
		t.Errorf("GetTimer: got %d, want -1 (nil timers map)", got)
	}
}
