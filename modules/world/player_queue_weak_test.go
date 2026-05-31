package world

import (
	"slices"
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// TestProcessPlayerQueue_NonWeakFiresBeforeWeakSameTick pins TS
// Player.processQueues (Engine-TS/src/engine/entity/Player.ts:854-869),
// which delegates to processQueue() (walks `this.queue`) and then
// processWeakQueue() (walks the separate `this.weakQueue` LinkList).
// Non-weak entries (NORMAL/STRONG/LONG) fire BEFORE weak entries on the
// same tick, regardless of insertion order.
//
// Pre-fix (player-script-2): goscape stored both kinds in a single
// p.queue slice and walked it in insertion order, so a WEAK entry
// inserted between two NORMAL entries fired BETWEEN them.
// Post-fix: a two-pass filter — non-weak first, then weak — restores
// the TS-faithful ordering.
func TestProcessPlayerQueue_NonWeakFiresBeforeWeakSameTick(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sfA := &script.ScriptFile{Name: "[A]", LookupKey: 0xA}
	sfB := &script.ScriptFile{Name: "[B]", LookupKey: 0xB}
	sfC := &script.ScriptFile{Name: "[C]", LookupKey: 0xC}
	s.scriptProvider.Register(sfA)
	s.scriptProvider.Register(sfB)
	s.scriptProvider.Register(sfC)

	var fired []uint32
	s.runScriptFn = func(f *script.ScriptFile, _ script.ActivePlayer, _ any, _ script.ServerTriggerType, _ bool, _ []int, _ []string) {
		fired = append(fired, f.LookupKey)
	}

	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playerLoop = []*Player{p}

	// Interleave NORMAL-WEAK-NORMAL at insertion order, all Delay=0.
	p.queue = []playerQueueRequest{
		{Script: sfA, Delay: 0, Type: script.QueueNormal},
		{Script: sfB, Delay: 0, Type: script.QueueWeak},
		{Script: sfC, Delay: 0, Type: script.QueueNormal},
	}

	s.processPlayerQueue(p)

	want := []uint32{0xA, 0xC, 0xB}
	if !slices.Equal(fired, want) {
		t.Fatalf("fire order: got %v, want %v (TS processQueues fires non-weak queue first, then weak queue — Player.ts:867-868 / player-script-2)", fired, want)
	}
}

// TestProcessPlayerQueue_WeakOnlyDelayDecrementsInWeakPass pins that
// a tick with ONLY a WEAK entry still decrements its Delay (matches
// TS processWeakQueue at Player.ts:896-905 which post-decrements
// `request.delay` regardless of whether processQueue had any entries).
// Regression pin for the 2-pass refactor: each pass must decrement
// every matching entry exactly once per tick.
func TestProcessPlayerQueue_WeakOnlyDelayDecrementsInWeakPass(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[weak-delay]", LookupKey: 0xDEAD}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playerLoop = []*Player{p}

	p.queue = []playerQueueRequest{
		{Script: sf, Delay: 3, Type: script.QueueWeak},
	}

	s.processPlayerQueue(p)

	if got := len(p.queue); got != 1 {
		t.Fatalf("p.queue len: got %d, want 1 (entry not yet ready)", got)
	}
	if got := p.queue[0].Delay; got != 2 {
		t.Errorf("weak entry Delay: got %d, want 2 (one tick post-decrement from 3)", got)
	}
}

// TestProcessPlayerQueue_WeakEnqueuedByNonWeakFiresSameTick pins TS-
// faithful re-entrancy across the pass boundary: a NORMAL script that
// runs in the non-weak pass and enqueues a WEAK entry mid-pass should
// see that WEAK entry fire in the SAME tick (during the weak pass).
// Matches TS processQueue → processWeakQueue (Player.ts:867-868):
// processWeakQueue walks weakQueue AFTER processQueue returns, so a
// weak entry appended during processQueue is visible. Regression pin.
func TestProcessPlayerQueue_WeakEnqueuedByNonWeakFiresSameTick(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sfNormal := &script.ScriptFile{Name: "[normal-enqueues-weak]", LookupKey: 0xCAFE}
	sfWeak := &script.ScriptFile{Name: "[weak-late]", LookupKey: 0xBEEF}
	s.scriptProvider.Register(sfNormal)
	s.scriptProvider.Register(sfWeak)

	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playerLoop = []*Player{p}

	var fired []uint32
	s.runScriptFn = func(f *script.ScriptFile, _ script.ActivePlayer, _ any, _ script.ServerTriggerType, _ bool, _ []int, _ []string) {
		fired = append(fired, f.LookupKey)
		// When the NORMAL fires, enqueue a WEAK entry at Delay=0 — TS-
		// faithfully it should fire in the same tick during the weak pass.
		if f.LookupKey == 0xCAFE {
			p.queue = append(p.queue, playerQueueRequest{Script: sfWeak, Delay: 0, Type: script.QueueWeak})
		}
	}

	p.queue = []playerQueueRequest{
		{Script: sfNormal, Delay: 0, Type: script.QueueNormal},
	}

	s.processPlayerQueue(p)

	want := []uint32{0xCAFE, 0xBEEF}
	if !slices.Equal(fired, want) {
		t.Fatalf("fire order: got %v, want %v (weak entry enqueued by NORMAL should fire in same-tick weak pass — TS Player.ts:867-868 / player-script-2)", fired, want)
	}
}
