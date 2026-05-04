package entity

import "testing"

// fakeTracker records Register/Unregister calls.
type fakeTracker struct {
	registered   []*NonPathing
	unregistered []*NonPathing
}

func (t *fakeTracker) Register(np *NonPathing)   { t.registered = append(t.registered, np) }
func (t *fakeTracker) Unregister(np *NonPathing) { t.unregistered = append(t.unregistered, np) }

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
