package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// TestEnqueueQueueEngineRoutesToEngineQueue pins NAI-144 routing: a
// QueueEngine enqueue must land in p.engineQueue, never in p.queue.
func TestEnqueueQueueEngineRoutesToEngineQueue(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[engine,test]", LookupKey: 0xdeadbeef}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	// EnqueueScriptArgs resolves via GetByID (index slot, not LookupKey);
	// Register appended the script at index 0.
	if err := p.EnqueueScriptArgs(0, 0, nil, nil, script.QueueEngine); err != nil {
		t.Fatalf("EnqueueScriptArgs: unexpected error: %v", err)
	}

	if len(p.queue) != 0 {
		t.Errorf("p.queue len: got %d, want 0 (QueueEngine must NOT route to primary queue)", len(p.queue))
	}
	if len(p.engineQueue) != 1 {
		t.Fatalf("p.engineQueue len: got %d, want 1", len(p.engineQueue))
	}
	if got := p.engineQueue[0].Script; got != sf {
		t.Errorf("p.engineQueue[0].Script: got %v, want %v", got, sf)
	}
	if got := p.engineQueue[0].Type; got != script.QueueEngine {
		t.Errorf("p.engineQueue[0].Type: got %v, want QueueEngine", got)
	}
}

// TestEnqueueQueueNormalDoesNotRouteToEngineQueue is a regression fence:
// QueueNormal must continue to land in p.queue (not p.engineQueue) after
// the NAI-144 switch is added.
func TestEnqueueQueueNormalDoesNotRouteToEngineQueue(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[normal,test]", LookupKey: 0xc0ffee}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	// EnqueueScriptArgs resolves via GetByID (index slot, not LookupKey);
	// Register appended the script at index 0.
	if err := p.EnqueueScriptArgs(0, 0, nil, nil, script.QueueNormal); err != nil {
		t.Fatalf("EnqueueScriptArgs: unexpected error: %v", err)
	}

	if len(p.queue) != 1 {
		t.Errorf("p.queue len: got %d, want 1 (QueueNormal must route to primary queue)", len(p.queue))
	}
	if len(p.engineQueue) != 0 {
		t.Errorf("p.engineQueue len: got %d, want 0", len(p.engineQueue))
	}
}

// TestProcessPlayerEngineQueuesFiresWhenDelayReachesZero pins TS
// Player.ts:641-651 drain semantics: `const delay = request.delay--;` reads
// the PRE-decrement value and fires when `canAccess() && delay <= 0`. So an
// entry enqueued with Delay=N fires on the (N+1)th drain — Delay=2 fires on
// the 3rd, after the stored delay has reached 0 and is read once more.
func TestProcessPlayerEngineQueuesFiresWhenDelayReachesZero(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{
		Name:      "[engine,delay_test]",
		LookupKey: 0xde1a000,
	}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playerLoop = []*Player{p}

	// Manual append — bypass EnqueueScriptFile to set Delay=2 explicitly.
	p.engineQueue = append(p.engineQueue, playerQueueRequest{
		Script: sf,
		Delay:  2,
		Type:   script.QueueEngine,
	})

	// Tick 1: read 2 (>0, no fire), Delay → 1.
	s.processPlayerEngineQueues()
	if len(p.engineQueue) != 1 {
		t.Fatalf("after tick 1: p.engineQueue len: got %d, want 1 (read 2, no fire)", len(p.engineQueue))
	}
	if got := p.engineQueue[0].Delay; got != 1 {
		t.Errorf("after tick 1: Delay: got %d, want 1", got)
	}

	// Tick 2: read 1 (>0, no fire), Delay → 0.
	s.processPlayerEngineQueues()
	if len(p.engineQueue) != 1 {
		t.Fatalf("after tick 2: p.engineQueue len: got %d, want 1 (read 1, no fire)", len(p.engineQueue))
	}
	if got := p.engineQueue[0].Delay; got != 0 {
		t.Errorf("after tick 2: Delay: got %d, want 0", got)
	}

	// Tick 3: read 0 (<=0, fires + removes).
	s.processPlayerEngineQueues()
	if len(p.engineQueue) != 0 {
		t.Errorf("after tick 3: p.engineQueue len: got %d, want 0 (read 0, fired + removed)", len(p.engineQueue))
	}
}

// TestProcessPlayerEngineQueuesGatedByCanAccess pins TS Player.ts:644
// gating: when CanAccess() is false, the entry stays in the queue (no
// fire, no removal); when CanAccess() is true on a later drain, the
// entry fires and is removed.
//
// goscape's CanAccess() returns false when delayed=true (player_script.go:324),
// so that's how this test forces the gate closed.
func TestProcessPlayerEngineQueuesGatedByCanAccess(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[engine,gate_test]", LookupKey: 0x9a7e000}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playerLoop = []*Player{p}

	// Force CanAccess()=false via delayed.
	p.delayed = true
	p.delayedUntil = 999_999

	p.engineQueue = append(p.engineQueue, playerQueueRequest{
		Script: sf,
		Delay:  0,
		Type:   script.QueueEngine,
	})

	s.processPlayerEngineQueues()

	if len(p.engineQueue) != 1 {
		t.Errorf("after tick (gated): p.engineQueue len: got %d, want 1 (CanAccess=false → no fire, no removal)", len(p.engineQueue))
	}

	// Release the gate.
	p.delayed = false

	s.processPlayerEngineQueues()

	if len(p.engineQueue) != 0 {
		t.Errorf("after tick (released): p.engineQueue len: got %d, want 0 (CanAccess=true → fired + removed)", len(p.engineQueue))
	}
}

// TestProcessPlayerEngineQueuesSanityFire pins the basic fire path:
// delay=0 + CanAccess()=true → entry fires and is removed in one drain.
//
// NB: the plan's original T5 ("no-delayed-gate distinct from primary
// queue") is not writable as specified — goscape's CanAccess() includes
// `delayed`, so delayed=true gates engineQueue same as primary queue.
// DEVIATION-NAI-144-D4 in processPlayerEngineQueues' doc comment notes
// the divergence. T5 is reframed as a sanity fire path until a TS-faithful
// canAccess port lands (out-of-scope follow-up).
func TestProcessPlayerEngineQueuesSanityFire(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[engine,sanity_fire]", LookupKey: 0xde1a7ed0}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playerLoop = []*Player{p}

	// p.delayed defaults to false → CanAccess()=true (assuming no modals).
	p.engineQueue = append(p.engineQueue, playerQueueRequest{
		Script: sf,
		Delay:  0,
		Type:   script.QueueEngine,
	})

	s.processPlayerEngineQueues()

	if len(p.engineQueue) != 0 {
		t.Errorf("p.engineQueue len: got %d, want 0 (delay=0 + CanAccess=true → fires)", len(p.engineQueue))
	}
}

// TestProcessPlayerEngineQueuesSameTickReentrant pins that the
// index-based drain loop fires multiple ready entries in one tick.
// Precondition for TS LinkList chain semantics where script A enqueues
// script B mid-fire and B is visible same-tick (T6 simulates the
// equivalent shape with two pre-seeded entries — both fire in one pass).
func TestProcessPlayerEngineQueuesSameTickReentrant(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sfA := &script.ScriptFile{Name: "[engine,reentry_a]", LookupKey: 0xa1}
	sfB := &script.ScriptFile{Name: "[engine,reentry_b]", LookupKey: 0xa2}
	s.scriptProvider.Register(sfA)
	s.scriptProvider.Register(sfB)

	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playerLoop = []*Player{p}

	p.engineQueue = append(p.engineQueue,
		playerQueueRequest{Script: sfA, Delay: 0, Type: script.QueueEngine},
		playerQueueRequest{Script: sfB, Delay: 0, Type: script.QueueEngine},
	)

	s.processPlayerEngineQueues()

	if len(p.engineQueue) != 0 {
		t.Errorf("p.engineQueue len: got %d, want 0 (both delay=0 entries fire in one drain)", len(p.engineQueue))
	}
}

// TestProcessPlayerEngineQueuesEmptyIsNoop pins defensive sanity: drain
// on an empty engineQueue must not panic and must not mutate state.
func TestProcessPlayerEngineQueuesEmptyIsNoop(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playerLoop = []*Player{p}

	if len(p.engineQueue) != 0 {
		t.Fatalf("p.engineQueue len: precondition got %d, want 0", len(p.engineQueue))
	}

	s.processPlayerEngineQueues() // should not panic

	if len(p.engineQueue) != 0 {
		t.Errorf("p.engineQueue len after drain: got %d, want 0", len(p.engineQueue))
	}
}
