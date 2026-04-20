package entity

import "testing"

func TestCheckLifecycleForever(t *testing.T) {
	e := Entity{Lifecycle: LifecycleForever, LifecycleTick: 999}
	for _, tick := range []int{0, 100, 1000, 1_000_000} {
		if !e.CheckLifecycle(tick) {
			t.Errorf("Forever should be alive at tick %d; got false", tick)
		}
	}
}

func TestCheckLifecycleRespawn(t *testing.T) {
	e := Entity{Lifecycle: LifecycleRespawn, LifecycleTick: 100}
	if e.CheckLifecycle(100) {
		t.Error("Respawn should be dead at the exact respawn tick (boundary)")
	}
	if e.CheckLifecycle(99) {
		t.Error("Respawn should be dead before the respawn tick")
	}
	if !e.CheckLifecycle(101) {
		t.Error("Respawn should be alive after the respawn tick")
	}
}

func TestCheckLifecycleDespawn(t *testing.T) {
	e := Entity{Lifecycle: LifecycleDespawn, LifecycleTick: 100}
	if !e.CheckLifecycle(99) {
		t.Error("Despawn should be alive before the despawn tick")
	}
	if e.CheckLifecycle(100) {
		t.Error("Despawn should be dead at the exact despawn tick (boundary)")
	}
	if e.CheckLifecycle(101) {
		t.Error("Despawn should be dead after the despawn tick")
	}
}

func TestUpdateLifecycleMatchesOnlyOnTickAndNotForever(t *testing.T) {
	for _, lc := range []Lifecycle{LifecycleRespawn, LifecycleDespawn} {
		e := Entity{Lifecycle: lc, LifecycleTick: 42}
		if !e.UpdateLifecycle(42) {
			t.Errorf("lifecycle %v: expected fire at exact tick match", lc)
		}
		if e.UpdateLifecycle(41) || e.UpdateLifecycle(43) {
			t.Errorf("lifecycle %v: expected silence off the exact tick", lc)
		}
	}
	e := Entity{Lifecycle: LifecycleForever, LifecycleTick: 42}
	if e.UpdateLifecycle(42) {
		t.Error("Forever should never fire UpdateLifecycle")
	}
}

func TestSetLifecycleRecordsBothTicks(t *testing.T) {
	var e Entity
	e.SetLifecycle(100, 50)
	if e.LifecycleTick != 100 {
		t.Errorf("LifecycleTick: got %d, want 100", e.LifecycleTick)
	}
	if e.LastLifecycleTick != 50 {
		t.Errorf("LastLifecycleTick: got %d, want 50", e.LastLifecycleTick)
	}
}

func TestNewEntitySetsSpawnFields(t *testing.T) {
	e := NewEntity(2, 3094, 3106, 1, 1, LifecycleRespawn)
	if e.Level != 2 || e.X != 3094 || e.Z != 3106 {
		t.Errorf("position: got (%d,%d,%d), want (2,3094,3106)", e.Level, e.X, e.Z)
	}
	if e.Width != 1 || e.Length != 1 {
		t.Errorf("size: got (%d,%d), want (1,1)", e.Width, e.Length)
	}
	if e.Lifecycle != LifecycleRespawn {
		t.Errorf("lifecycle: got %v, want Respawn", e.Lifecycle)
	}
	if e.LifecycleTick != 0 || e.LastLifecycleTick != 0 {
		t.Error("runtime tick fields should be zero after NewEntity")
	}
}
