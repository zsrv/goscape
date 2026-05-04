package entity

import "testing"

// fakeTracker records Register/Unregister calls.
type fakeTracker struct {
	registered   []*NonPathing
	unregistered []*NonPathing
}

func (ft *fakeTracker) Register(np *NonPathing)   { ft.registered = append(ft.registered, np) }
func (ft *fakeTracker) Unregister(np *NonPathing) { ft.unregistered = append(ft.unregistered, np) }

func TestSetLifeCyclePositiveDurationRegisters(t *testing.T) {
	tr := &fakeTracker{}
	np := &NonPathing{Entity: NewEntity(0, 100, 200, 1, 1, LifecycleDespawn)}
	np.parent = np

	np.SetLifeCycle(5, 100, tr)

	if len(tr.registered) != 1 || tr.registered[0] != np {
		t.Errorf("Register: got %v, want [np]", tr.registered)
	}
	if np.LifecycleTick != 105 {
		t.Errorf("LifecycleTick: got %d, want 105 (currentTick=100 + duration=5)", np.LifecycleTick)
	}
	if np.LastLifecycleTick != 100 {
		t.Errorf("LastLifecycleTick: got %d, want 100", np.LastLifecycleTick)
	}
}

func TestSetLifeCycleSecondCallUnregistersFirst(t *testing.T) {
	tr := &fakeTracker{}
	np := &NonPathing{Entity: NewEntity(0, 100, 200, 1, 1, LifecycleDespawn)}
	np.parent = np

	np.SetLifeCycle(5, 100, tr)
	np.SetLifeCycle(3, 110, tr)

	if len(tr.unregistered) != 1 || tr.unregistered[0] != np {
		t.Errorf("Unregister: got %v, want [np]", tr.unregistered)
	}
	if len(tr.registered) != 2 {
		t.Errorf("Register count: got %d, want 2", len(tr.registered))
	}
	if np.LifecycleTick != 113 {
		t.Errorf("LifecycleTick: got %d, want 113", np.LifecycleTick)
	}
	if np.tracker != tr {
		t.Errorf("np.tracker after second call: got %v, want %v", np.tracker, tr)
	}
}

func TestSetLifeCycleNonPositiveDurationUntracks(t *testing.T) {
	tr := &fakeTracker{}
	np := &NonPathing{Entity: NewEntity(0, 100, 200, 1, 1, LifecycleDespawn)}
	np.parent = np

	np.SetLifeCycle(5, 100, tr)
	np.SetLifeCycle(-1, 110, nil)

	if len(tr.unregistered) != 1 {
		t.Errorf("Unregister: got %d, want 1", len(tr.unregistered))
	}
	if np.LifecycleTick != -1 {
		t.Errorf("LifecycleTick: got %d, want -1", np.LifecycleTick)
	}
}

func TestSetLifeCycleNoTrackerNoRegister(t *testing.T) {
	// Initial call with duration<=0 and tracker=nil must not panic and
	// must not register anything.
	np := &NonPathing{Entity: NewEntity(0, 100, 200, 1, 1, LifecycleDespawn)}
	np.SetLifeCycle(0, 50, nil)
	if np.LifecycleTick != -1 {
		t.Errorf("LifecycleTick: got %d, want -1", np.LifecycleTick)
	}
}

func TestSetLifeCyclePositiveDurationNilTrackerSchedulesButSkipsRegister(t *testing.T) {
	// Transition-window invariant pin (NAI-86 B2.1 → B2.2): when
	// duration>0 and tracker is nil, SetLifeCycle still schedules the
	// transition tick (Entity.SetLifecycle) but does NOT panic and does
	// NOT update np.tracker. Bundle 2.2 wires the tracker so this path
	// becomes unreachable in production, but the contract is pinned now
	// so a future regression doesn't silently re-introduce the panic.
	np := &NonPathing{Entity: NewEntity(0, 100, 200, 1, 1, LifecycleDespawn)}
	np.parent = np
	np.SetLifeCycle(5, 100, nil)
	if np.LifecycleTick != 105 {
		t.Errorf("LifecycleTick: got %d, want 105", np.LifecycleTick)
	}
	if np.tracker != nil {
		t.Error("np.tracker must remain nil when SetLifeCycle was called with tracker=nil")
	}
}
