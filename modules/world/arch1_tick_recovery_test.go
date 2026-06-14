package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// ARCH-1 (A): a panic during the lifecycle transition must re-arm
// lifecycleTick=1 (TS Npc.ts:144-150 setLifeCycle(1) retry), NOT evict.
// Deterministic panic: s.npcs is a fixed [16384]*Npc array, so an
// out-of-bounds nid forces an index-out-of-range panic inside removeNpc's
// despawn branch (s.npcs[n.nid] = nil) — standing in for any transition fault.
func TestFireNpcLifecycle_DespawnPanicRetries(t *testing.T) {
	s := &Server{log: discardLogger()}
	n := &Npc{nid: 1 << 20, typeId: 42, lifecycle: NpcLifecycleDespawn}

	fired := s.fireNpcLifecycle(n)

	if !fired {
		t.Error("fired: want true (transition attempted), got false")
	}
	if n.lifecycleTick != 1 {
		t.Errorf("lifecycleTick: want 1 (TS setLifeCycle(1) retry), got %d", n.lifecycleTick)
	}
	// Reaching here proves the panic did not propagate.
}

// ARCH-1 (A): a clean transition must NOT re-arm lifecycleTick (no retry).
func TestFireNpcLifecycle_DespawnCleanNoRetry(t *testing.T) {
	s := &Server{log: discardLogger()} // scriptProvider nil → no trigger
	n := &Npc{nid: 7, typeId: 42, lifecycle: NpcLifecycleDespawn, lifecycleTick: 0}

	fired := s.fireNpcLifecycle(n)

	if !fired {
		t.Error("fired: want true, got false")
	}
	if n.lifecycleTick != 0 {
		t.Errorf("lifecycleTick: want 0 (clean path does not retry), got %d", n.lifecycleTick)
	}
	if !n.dead {
		t.Error("n.dead: want true after clean despawn, got false")
	}
}

// ARCH-1 (A): the inner recover pre-empts the outer recoverNpc eviction.
// Run the npc through the same closure shape processNpcs uses; the inner
// recover handles the panic so recoverNpc never fires.
func TestNpcLifecyclePanic_InnerRecoverPreemptsEviction(t *testing.T) {
	s := &Server{log: discardLogger()}
	n := &Npc{nid: 1 << 20, typeId: 42, lifecycle: NpcLifecycleDespawn, lifecycleTick: 1}

	func(n *Npc) {
		defer recoverNpc(n, s, "processNpcTurn", s.log)
		n.turn(s)
	}(n)

	if n.lifecycleTick != 1 {
		t.Errorf("lifecycleTick: want 1 (inner recover re-armed; outer evict pre-empted), got %d", n.lifecycleTick)
	}
}

// newPanickingWorldScript builds a ScriptState whose script.Execute panics
// with zero world wiring: handlePushArrayInt (OpPushArrayInt) unconditionally
// reads s.Script.IntOperands[s.PC], so an empty IntOperands makes that index
// out-of-range → a real Go panic escaping Execute (stands in for any panic
// that escapes the script runtime).
func newPanickingWorldScript(t *testing.T) *script.ScriptState {
	t.Helper()
	sf := &script.ScriptFile{
		Name:             "[world,panic]",
		Opcodes:          []script.Opcode{script.OpPushArrayInt},
		IntOperands:      []int32{}, // empty → IntOperands[0] panics in the handler
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	return script.Init(sf, nil, false, nil, nil)
}

// ARCH-1 (B): fireWorldScript reports panicked=true when execution panics.
func TestFireWorldScript_PanicReported(t *testing.T) {
	s := newTestServer(t)
	state := newPanickingWorldScript(t)

	if !s.fireWorldScript(state) {
		t.Error("panicked: want true for a panicking world script, got false")
	}
}

// ARCH-1 (B): fireWorldScript reports panicked=false for a clean script.
func TestFireWorldScript_CleanReturnsFalse(t *testing.T) {
	s := newTestServer(t)
	state := newReturnImmediatelyScript(t)

	if s.fireWorldScript(state) {
		t.Error("panicked: want false for a clean script, got true")
	}
}

// ARCH-1 (B): a panicking world-queue entry is LEFT queued (retry next tick),
// mirroring TS World.ts:542-558 (unlink runs after execute; a throw skips it).
func TestProcessWorldQueue_PanicRetriesNextTick(t *testing.T) {
	s := newTestServer(t)
	state := newPanickingWorldScript(t)
	s.EnqueueWorldScript(state, 0) // stored=1; fires on the 2nd drain

	s.processWorldQueue() // tick 1: skip
	s.processWorldQueue() // tick 2: fire → panics → LEFT queued
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after panic-fire: queue length got %d, want 1 (entry retained for retry)", got)
	}
	s.processWorldQueue() // tick 3: fires again, still panics, still queued
	if got := len(s.worldScriptQueue); got != 1 {
		t.Errorf("after 2nd panic-fire: queue length got %d, want 1 (still retrying)", got)
	}
}

// ARCH-1 (B): a clean world-queue entry is removed after firing.
func TestProcessWorldQueue_CleanEntryRemovedAfterFire(t *testing.T) {
	s := newTestServer(t)
	state := newReturnImmediatelyScript(t)
	s.EnqueueWorldScript(state, 0)

	s.processWorldQueue() // tick 1: skip
	s.processWorldQueue() // tick 2: fire → clean → removed
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("after clean fire: queue length got %d, want 0 (removed)", got)
	}
}
