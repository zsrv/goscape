package entity

import "testing"

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
