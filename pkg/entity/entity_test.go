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

// TestSetLifecycleClampsToOne pins the clamp Engine-TS 8139461a added to
// Entity.setLifeCycle (Entity.ts:36-42 @1d25566c):
//
//	if (tick === -1) { this.lifecycleTick = tick; }
//	else { this.lifecycleTick = Math.max(1, tick); }
//
// -1 is the "no transition scheduled" sentinel and must survive unchanged;
// every real tick floors at 1, because World.currentTick starts at 1 and a
// stored 0 would read as already-due on the very first cycle.
func TestSetLifecycleClampsToOne(t *testing.T) {
	tests := []struct {
		name           string
		transitionTick int
		want           int
	}{
		{"sentinel survives", -1, -1},
		{"zero clamps up", 0, 1},
		{"negative below sentinel clamps up", -5, 1},
		{"one is unchanged", 1, 1},
		{"positive is unchanged", 42, 42},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var e Entity
			e.SetLifecycle(tc.transitionTick, 7)
			if e.LifecycleTick != tc.want {
				t.Errorf("LifecycleTick: got %d, want %d", e.LifecycleTick, tc.want)
			}
			if e.LastLifecycleTick != 7 {
				t.Errorf("LastLifecycleTick: got %d, want 7", e.LastLifecycleTick)
			}
		})
	}
}
