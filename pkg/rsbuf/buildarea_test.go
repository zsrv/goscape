package rsbuf

import (
	"testing"
)

func TestNewBuildArea_ZeroInit(t *testing.T) {
	b := newBuildArea()
	if b == nil {
		t.Fatal("newBuildArea returned nil")
	}
	if b.Players == nil || b.Npcs == nil {
		t.Errorf("newBuildArea: Players=%v, Npcs=%v, want both non-nil", b.Players, b.Npcs)
	}
	if b.Players.Len() != 0 || b.Npcs.Len() != 0 {
		t.Errorf("newBuildArea: Players.Len=%d, Npcs.Len=%d, want both 0", b.Players.Len(), b.Npcs.Len())
	}
	if b.ViewDistance != preferredViewDistance {
		t.Errorf("newBuildArea: ViewDistance=%d, want %d", b.ViewDistance, preferredViewDistance)
	}
	for i := range b.appearances {
		if b.appearances[i] != 0 {
			t.Errorf("newBuildArea: appearances[%d]=%d, want 0", i, b.appearances[i])
			break
		}
	}
}

func TestBuildArea_HasAppearance_FreshIsFalse(t *testing.T) {
	b := newBuildArea()
	// tick=0 is NOT tested here: appearances is zero-initialized, so
	// HasAppearance(pid, 0) returns true on a fresh BuildArea (0 == 0).
	// This matches upstream Rust has_appearance at build.rs:151-153.
	// In practice, callers guard with last_appearance != -1 before calling
	// HasAppearance, so tick=0 is never passed by the engine (info.rs:305).
	for _, tick := range []uint32{1, 100} {
		if b.HasAppearance(7, tick) {
			t.Errorf("fresh BuildArea: HasAppearance(7, %d) = true, want false", tick)
		}
	}
}

func TestBuildArea_SaveAppearance_RoundTrip(t *testing.T) {
	b := newBuildArea()
	b.SaveAppearance(7, 100)
	if !b.HasAppearance(7, 100) {
		t.Error("after SaveAppearance(7, 100), HasAppearance(7, 100) is false")
	}
	if b.HasAppearance(7, 99) {
		t.Error("HasAppearance(7, 99) is true after SaveAppearance(7, 100) — should be false (tick mismatch)")
	}
	if b.HasAppearance(7, 101) {
		t.Error("HasAppearance(7, 101) is true after SaveAppearance(7, 100) — should be false")
	}
	if b.HasAppearance(8, 100) {
		t.Error("SaveAppearance(7, 100) leaked into pid=8")
	}
}

func TestBuildArea_Cleanup_ClearsAll(t *testing.T) {
	b := newBuildArea()
	b.Players.Insert(5)
	b.Players.Insert(10)
	b.Npcs.Insert(3)
	b.SaveAppearance(7, 100)
	b.SaveAppearance(8, 200)

	b.Cleanup()

	if b.Players.Len() != 0 {
		t.Errorf("Cleanup: Players.Len=%d, want 0", b.Players.Len())
	}
	if b.Npcs.Len() != 0 {
		t.Errorf("Cleanup: Npcs.Len=%d, want 0", b.Npcs.Len())
	}
	if b.HasAppearance(7, 100) || b.HasAppearance(8, 200) {
		t.Error("Cleanup: appearances not cleared")
	}
}
