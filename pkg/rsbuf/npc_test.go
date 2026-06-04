package rsbuf

import (
	"testing"
)

func TestNewNpc_SentinelDefaults(t *testing.T) {
	n := newNpc(7, 100)
	if n.NID != 7 {
		t.Errorf("NID: got %d, want 7", n.NID)
	}
	if n.NType != 100 {
		t.Errorf("NType: got %d, want 100", n.NType)
	}
	if n.Coord != 0 {
		t.Errorf("Coord: got %d, want 0", n.Coord)
	}
	if n.Tele {
		t.Error("Tele should default false")
	}
	if n.RunDir != -1 || n.WalkDir != -1 {
		t.Errorf("RunDir/WalkDir: got (%d, %d), want (-1, -1)", n.RunDir, n.WalkDir)
	}
	if n.Active {
		t.Error("Active should default false")
	}
	if n.Masks != 0 {
		t.Errorf("Masks: got %d, want 0", n.Masks)
	}
	for _, tt := range []struct {
		name string
		got  int32
	}{
		{"FaceEntity", n.FaceEntity},
		{"FaceX", n.FaceX}, {"FaceZ", n.FaceZ},
		{"OrientationX", n.OrientationX}, {"OrientationZ", n.OrientationZ},
		{"DamageTaken", n.DamageTaken}, {"DamageType", n.DamageType},
		{"CurrentHitpoints", n.CurrentHitpoints}, {"BaseHitpoints", n.BaseHitpoints},
		{"AnimID", n.AnimID}, {"AnimDelay", n.AnimDelay},
		{"GraphicID", n.GraphicID}, {"GraphicHeight", n.GraphicHeight}, {"GraphicDelay", n.GraphicDelay},
	} {
		if tt.got != -1 {
			t.Errorf("%s: got %d, want -1", tt.name, tt.got)
		}
	}
	if n.Say != nil {
		t.Error("Say should default nil")
	}
	if n.Observers != 0 {
		t.Errorf("Observers: got %d, want 0", n.Observers)
	}
}

func TestNpcCleanup_ZeroesTransient(t *testing.T) {
	n := newNpc(7, 100)
	n.WalkDir = 3
	n.RunDir = 4
	n.Tele = true
	n.Masks = 0xff
	n.FaceX = 50
	n.FaceZ = 60
	n.DamageTaken = 9
	n.DamageType = 0
	n.CurrentHitpoints = 50
	n.BaseHitpoints = 100
	n.AnimID = 808
	n.AnimDelay = 0
	s := "rwar"
	n.Say = &s
	n.GraphicID = 100
	n.GraphicHeight = 92
	n.GraphicDelay = 0

	n.cleanup()

	if n.WalkDir != -1 || n.RunDir != -1 {
		t.Errorf("cleanup: WalkDir/RunDir not reset to -1 (got %d, %d)", n.WalkDir, n.RunDir)
	}
	if n.Tele {
		t.Error("cleanup: Tele not zeroed")
	}
	if n.Masks != 0 {
		t.Errorf("cleanup: Masks not zeroed (got %d)", n.Masks)
	}
	if n.FaceX != -1 || n.FaceZ != -1 {
		t.Errorf("cleanup: FaceX/FaceZ not reset to -1 (got %d, %d)", n.FaceX, n.FaceZ)
	}
	for _, tt := range []struct {
		name string
		got  int32
	}{
		{"DamageTaken", n.DamageTaken}, {"DamageType", n.DamageType},
		{"CurrentHitpoints", n.CurrentHitpoints}, {"BaseHitpoints", n.BaseHitpoints},
		{"AnimID", n.AnimID}, {"AnimDelay", n.AnimDelay},
		{"GraphicID", n.GraphicID}, {"GraphicHeight", n.GraphicHeight}, {"GraphicDelay", n.GraphicDelay},
	} {
		if tt.got != -1 {
			t.Errorf("cleanup: %s not reset to -1 (got %d)", tt.name, tt.got)
		}
	}
	if n.Say != nil {
		t.Error("cleanup: Say not nilled")
	}
}

// TestNpcDamage2SentinelDefaults pins that newNpc initialises
// DamageTaken2 and DamageType2 to -1 (rsbuf 244 npc.rs:50-51).
func TestNpcDamage2SentinelDefaults(t *testing.T) {
	n := newNpc(7, 100)
	if n.DamageTaken2 != -1 {
		t.Errorf("DamageTaken2: got %d, want -1", n.DamageTaken2)
	}
	if n.DamageType2 != -1 {
		t.Errorf("DamageType2: got %d, want -1", n.DamageType2)
	}
}

// TestNpcCleanup_ResetsDamage2 pins that cleanup resets DamageTaken2/DamageType2
// to -1 (rsbuf 244 npc.rs:77-78).
func TestNpcCleanup_ResetsDamage2(t *testing.T) {
	n := newNpc(7, 100)
	n.DamageTaken2 = 3
	n.DamageType2 = 1
	n.cleanup()
	if n.DamageTaken2 != -1 {
		t.Errorf("cleanup: DamageTaken2 not reset to -1 (got %d)", n.DamageTaken2)
	}
	if n.DamageType2 != -1 {
		t.Errorf("cleanup: DamageType2 not reset to -1 (got %d)", n.DamageType2)
	}
}

func TestNpcCleanup_PreservesPersistent(t *testing.T) {
	// Per upstream npc.rs:62-83 commented-out lines:
	// faceEntity, orientationX, orientationZ persist across ticks.
	// Observers also persists (it's a counter, not transient state).
	n := newNpc(7, 100)
	n.FaceEntity = 42
	n.OrientationX = 50
	n.OrientationZ = 60
	n.Observers = 3

	n.cleanup()

	if n.FaceEntity != 42 {
		t.Errorf("cleanup CLEARED FaceEntity: got %d, want 42", n.FaceEntity)
	}
	if n.OrientationX != 50 || n.OrientationZ != 60 {
		t.Errorf("cleanup CLEARED Orientation*: got (%d, %d)", n.OrientationX, n.OrientationZ)
	}
	if n.Observers != 3 {
		t.Errorf("cleanup CLEARED Observers: got %d, want 3", n.Observers)
	}
}
