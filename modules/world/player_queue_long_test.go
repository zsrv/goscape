package world

import (
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

			if !intSliceEqual(captured, tc.wantPassed) {
				t.Fatalf("script intArgs: got %v, want %v", captured, tc.wantPassed)
			}
		})
	}
}

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
