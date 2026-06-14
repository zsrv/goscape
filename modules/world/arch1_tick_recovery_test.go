package world

import "testing"

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
