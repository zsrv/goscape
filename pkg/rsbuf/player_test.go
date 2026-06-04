package rsbuf

import (
	"testing"
)

func TestNewPlayer_SentinelDefaults(t *testing.T) {
	p := newPlayer(42)
	if p == nil {
		t.Fatal("newPlayer returned nil")
	}
	if p.PID != 42 {
		t.Errorf("PID: got %d, want 42", p.PID)
	}
	if p.Coord != 0 {
		t.Errorf("Coord: got %d, want 0", p.Coord)
	}
	if p.Origin != 0 {
		t.Errorf("Origin: got %d, want 0", p.Origin)
	}
	if p.Tele || p.Jump {
		t.Errorf("Tele/Jump should default false")
	}
	if p.RunDir != -1 || p.WalkDir != -1 {
		t.Errorf("RunDir/WalkDir: got (%d, %d), want (-1, -1)", p.RunDir, p.WalkDir)
	}
	if p.Visibility != VisibilityDefault {
		t.Errorf("Visibility: got %d, want VisibilityDefault", p.Visibility)
	}
	if p.Active {
		t.Error("Active should default false")
	}
	if p.Masks != 0 {
		t.Errorf("Masks: got %d, want 0", p.Masks)
	}
	if p.Appearance != nil {
		t.Errorf("Appearance: got %v, want nil", p.Appearance)
	}
	if p.LastAppearance != -1 {
		t.Errorf("LastAppearance: got %d, want -1", p.LastAppearance)
	}
	for _, tt := range []struct {
		name string
		got  int32
	}{
		{"FaceEntity", p.FaceEntity},
		{"FaceX", p.FaceX}, {"FaceZ", p.FaceZ},
		{"OrientationX", p.OrientationX}, {"OrientationZ", p.OrientationZ},
		{"DamageTaken", p.DamageTaken}, {"DamageType", p.DamageType},
		{"CurrentHitpoints", p.CurrentHitpoints}, {"BaseHitpoints", p.BaseHitpoints},
		{"AnimID", p.AnimID}, {"AnimDelay", p.AnimDelay},
		{"GraphicID", p.GraphicID}, {"GraphicHeight", p.GraphicHeight}, {"GraphicDelay", p.GraphicDelay},
	} {
		if tt.got != -1 {
			t.Errorf("%s: got %d, want -1", tt.name, tt.got)
		}
	}
	if p.Say != nil {
		t.Error("Say should default nil")
	}
	if p.Chat != nil {
		t.Error("Chat should default nil")
	}
	if p.ExactMove != nil {
		t.Error("ExactMove should default nil")
	}
}

func TestPlayerCleanup_ZeroesTransient(t *testing.T) {
	p := newPlayer(5)
	// Populate transient fields.
	p.WalkDir = 3
	p.RunDir = 4
	p.Jump = true
	p.Tele = true
	p.Masks = 0xff
	p.FaceX = 100
	p.FaceZ = 200
	p.DamageTaken = 5
	p.DamageType = 1
	p.CurrentHitpoints = 90
	p.BaseHitpoints = 99
	p.AnimID = 808
	p.AnimDelay = 0
	s := "hi"
	p.Say = &s
	p.Chat = &Chat{Bytes: []byte{1, 2}, Color: 9}
	p.GraphicID = 100
	p.GraphicHeight = 92
	p.GraphicDelay = 0
	p.ExactMove = &ExactMove{StartX: 1}

	p.cleanup()

	if p.WalkDir != -1 || p.RunDir != -1 {
		t.Errorf("cleanup: WalkDir/RunDir not reset to -1 (got %d, %d)", p.WalkDir, p.RunDir)
	}
	if p.Jump || p.Tele {
		t.Error("cleanup: Jump/Tele not zeroed")
	}
	if p.Masks != 0 {
		t.Errorf("cleanup: Masks not zeroed (got %d)", p.Masks)
	}
	if p.FaceX != -1 || p.FaceZ != -1 {
		t.Errorf("cleanup: FaceX/FaceZ not reset to -1 (got %d, %d)", p.FaceX, p.FaceZ)
	}
	for _, tt := range []struct {
		name string
		got  int32
	}{
		{"DamageTaken", p.DamageTaken}, {"DamageType", p.DamageType},
		{"CurrentHitpoints", p.CurrentHitpoints}, {"BaseHitpoints", p.BaseHitpoints},
		{"AnimID", p.AnimID}, {"AnimDelay", p.AnimDelay},
		{"GraphicID", p.GraphicID}, {"GraphicHeight", p.GraphicHeight}, {"GraphicDelay", p.GraphicDelay},
	} {
		if tt.got != -1 {
			t.Errorf("cleanup: %s not reset to -1 (got %d)", tt.name, tt.got)
		}
	}
	if p.Say != nil {
		t.Error("cleanup: Say not nilled")
	}
	if p.Chat != nil {
		t.Error("cleanup: Chat not nilled")
	}
	if p.ExactMove != nil {
		t.Error("cleanup: ExactMove not nilled")
	}
}

func TestPlayerCleanup_PreservesPersistent(t *testing.T) {
	// Per upstream player.rs:96-121 commented-out cleanup lines:
	// appearance, lastAppearance, faceEntity, orientationX, orientationZ
	// MUST NOT be cleared by cleanup — they persist across ticks.
	p := newPlayer(5)
	p.Appearance = []byte{1, 2, 3}
	p.LastAppearance = 100
	p.FaceEntity = 42
	p.OrientationX = 50
	p.OrientationZ = 60

	p.cleanup()

	if got := p.Appearance; len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("cleanup CLEARED Appearance: got %v, want [1 2 3]", got)
	}
	if p.LastAppearance != 100 {
		t.Errorf("cleanup CLEARED LastAppearance: got %d, want 100", p.LastAppearance)
	}
	if p.FaceEntity != 42 {
		t.Errorf("cleanup CLEARED FaceEntity: got %d, want 42", p.FaceEntity)
	}
	if p.OrientationX != 50 || p.OrientationZ != 60 {
		t.Errorf("cleanup CLEARED Orientation*: got (%d, %d), want (50, 60)", p.OrientationX, p.OrientationZ)
	}
}

// TestPlayerDamage2SentinelDefaults pins that newPlayer initialises
// DamageTaken2 and DamageType2 to -1 (rsbuf 244 player.rs:82-83).
func TestPlayerDamage2SentinelDefaults(t *testing.T) {
	p := newPlayer(42)
	if p.DamageTaken2 != -1 {
		t.Errorf("DamageTaken2: got %d, want -1", p.DamageTaken2)
	}
	if p.DamageType2 != -1 {
		t.Errorf("DamageType2: got %d, want -1", p.DamageType2)
	}
}

// TestPlayerCleanup_ResetsDamage2 pins that cleanup resets DamageTaken2/DamageType2
// to -1 (rsbuf 244 player.rs:113-114).
func TestPlayerCleanup_ResetsDamage2(t *testing.T) {
	p := newPlayer(5)
	p.DamageTaken2 = 3
	p.DamageType2 = 1
	p.cleanup()
	if p.DamageTaken2 != -1 {
		t.Errorf("cleanup: DamageTaken2 not reset to -1 (got %d)", p.DamageTaken2)
	}
	if p.DamageType2 != -1 {
		t.Errorf("cleanup: DamageType2 not reset to -1 (got %d)", p.DamageType2)
	}
}

func TestChat_Construction(t *testing.T) {
	c := &Chat{Bytes: []byte{0x10, 0x20}, Color: 1, Effect: 2, Ignored: 3}
	if c.Color != 1 || c.Effect != 2 || c.Ignored != 3 {
		t.Errorf("Chat fields: got (%d,%d,%d), want (1,2,3)", c.Color, c.Effect, c.Ignored)
	}
	if len(c.Bytes) != 2 || c.Bytes[0] != 0x10 || c.Bytes[1] != 0x20 {
		t.Errorf("Chat.Bytes: got %v", c.Bytes)
	}
}

func TestExactMove_Construction(t *testing.T) {
	e := &ExactMove{StartX: 1, StartZ: 2, EndX: 3, EndZ: 4, Begin: 5, Finish: 6, Dir: 7}
	if e.StartX != 1 || e.StartZ != 2 || e.EndX != 3 || e.EndZ != 4 ||
		e.Begin != 5 || e.Finish != 6 || e.Dir != 7 {
		t.Errorf("ExactMove fields not set correctly: %+v", e)
	}
}
