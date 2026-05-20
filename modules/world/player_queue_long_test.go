package world

import (
	"slices"
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// TestProcessPlayerQueue_LongStripsArgs0 pins TS Player.ts:887-889:
// for QueueLong entries, the first int arg is the logoutAction indicator
// (prepended at handlers_player_vararg.go:80-81 for LONGQUEUEVARARG and
// at handlers.go:988 for fixed-arg LONGQUEUE) and is stripped before
// the script runs. QueueNormal entries are NOT stripped (regression pin).
func TestProcessPlayerQueue_LongStripsArgs0(t *testing.T) {
	cases := []struct {
		name       string
		queueType  script.PlayerQueueType
		intArgs    []int
		wantPassed []int
	}{
		{"QueueLong strips args[0]", script.QueueLong, []int{7, 100, 200}, []int{100, 200}},
		{"QueueLong with 1-element args strips to empty", script.QueueLong, []int{0}, []int{}},
		{"QueueNormal NOT stripped (regression pin)", script.QueueNormal, []int{7, 100, 200}, []int{7, 100, 200}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			s.scriptProvider = script.NewProvider()
			var captured []int
			sentinel := &script.ScriptFile{
				Name:      "[sentinel," + tc.name + "]",
				LookupKey: 0xC0DE,
			}
			s.scriptProvider.Register(sentinel)
			s.runScriptFn = func(_ *script.ScriptFile, _ script.ActivePlayer, _ any, _ bool, intArgs []int, _ []string) {
				captured = append([]int{}, intArgs...)
			}

			p, _ := newTestPlayer(t)
			p.client.server = s
			s.playerLoop = []*Player{p}

			p.queue = append(p.queue, playerQueueRequest{
				Script:  sentinel,
				Delay:   0,
				Type:    tc.queueType,
				IntArgs: tc.intArgs,
			})

			s.processPlayerQueue(p)

			if !slices.Equal(captured, tc.wantPassed) {
				t.Fatalf("script intArgs: got %v, want %v", captured, tc.wantPassed)
			}
		})
	}
}

// TestProcessPlayerQueue_LoggingOutAcceleratesLongZeroAction pins TS
// Player.ts:877-881: when p.loggingOut==true, a QueueLong entry whose
// IntArgs[0]==0 (the ACCELERATE indicator) is force-fired this tick
// regardless of its remaining delay. Non-ACCELERATE LONG entries and
// non-LONG entries decrement normally.
func TestProcessPlayerQueue_LoggingOutAcceleratesLongZeroAction(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[long,acc]", LookupKey: 0xAC1}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.loggingOut = true
	s.playerLoop = []*Player{p}

	// Entry 0: ACCELERATE (args[0]==0), Delay=5 — should fire this tick.
	// Entry 1: non-accelerate LONG (args[0]==1), Delay=5 — should decrement.
	p.queue = []playerQueueRequest{
		{Script: sf, Delay: 5, Type: script.QueueLong, IntArgs: []int{0, 42}},
		{Script: sf, Delay: 5, Type: script.QueueLong, IntArgs: []int{1, 99}},
	}

	s.processPlayerQueue(p)

	// After one tick:
	//   - entry 0: accel→delay=0, post-decrement→-1, fires & removes.
	//   - entry 1: no accel, post-decrement→4, stays.
	if len(p.queue) != 1 {
		t.Fatalf("p.queue len after tick: got %d, want 1", len(p.queue))
	}
	if got := p.queue[0].Delay; got != 4 {
		t.Errorf("surviving entry delay: got %d, want 4", got)
	}
	if got := p.queue[0].IntArgs[0]; got != 1 {
		t.Errorf("surviving entry IntArgs[0]: got %d, want 1 (non-accel entry should remain)", got)
	}
}
