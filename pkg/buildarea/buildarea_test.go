package buildarea

import "testing"

func TestNewNeedsFirstRebuild(t *testing.T) {
	ba := New()
	if !ba.ShouldRebuild(3094, 3106, false) {
		t.Error("first ShouldRebuild (OriginX=-1) should be true")
	}
}

func TestRebuildCommitsOrigin(t *testing.T) {
	ba := New()
	ba.Rebuild(3094, 3106, 5)
	if ba.OriginX != 3094 || ba.OriginZ != 3106 {
		t.Errorf("origin: got (%d,%d), want (3094,3106)", ba.OriginX, ba.OriginZ)
	}
	if ba.LastBuild != 5 {
		t.Errorf("LastBuild: got %d, want 5", ba.LastBuild)
	}
}

func TestShouldNotRebuildWithinWindow(t *testing.T) {
	ba := New()
	ba.Rebuild(3094, 3106, 1)
	if ba.ShouldRebuild(3094, 3107, false) {
		t.Error("single-step movement should not trigger rebuild")
	}
}

func TestShouldRebuildAtWindowEdge(t *testing.T) {
	ba := New()
	ba.Rebuild(3094, 3106, 1)
	// Origin zone x=386. reloadLeftX=(386-4)<<3=3056. x=3055 triggers rebuild.
	if !ba.ShouldRebuild(3055, 3106, false) {
		t.Error("crossing west window edge should trigger rebuild")
	}
}

func TestReconnectAlwaysTriggers(t *testing.T) {
	ba := New()
	ba.Rebuild(3094, 3106, 1)
	if !ba.ShouldRebuild(3094, 3106, true) {
		t.Error("reconnect should always trigger rebuild")
	}
}

func TestRebuildPopulatesMapsquares(t *testing.T) {
	ba := New()
	ms := ba.Rebuild(3094, 3106, 1)
	if len(ms) == 0 {
		t.Error("rebuild should return mapsquares")
	}
	// Origin mapsquare = (3094>>6, 3106>>6) = (48, 48). Must be in set.
	want := uint16((48 << 8) | 48)
	found := false
	for _, m := range ms {
		if m == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected (48,48) in mapsquare list; got %v", ms)
	}
}

func TestPlayersSetAddRemove(t *testing.T) {
	ba := New()
	if _, ok := ba.Players[5]; ok {
		t.Error("new BuildArea should have empty Players")
	}
	ba.Players[5] = struct{}{}
	if _, ok := ba.Players[5]; !ok {
		t.Error("add should succeed")
	}
	delete(ba.Players, 5)
	if _, ok := ba.Players[5]; ok {
		t.Error("remove should succeed")
	}
}

func TestAppearanceHasRecord(t *testing.T) {
	ba := New()
	if ba.HasAppearance(5, 0x12345) {
		t.Error("fresh BuildArea should not have appearance cached")
	}
	ba.RecordAppearance(5, 0x12345)
	if !ba.HasAppearance(5, 0x12345) {
		t.Error("RecordAppearance did not stick")
	}
	if ba.HasAppearance(5, 0xdeadbeef) {
		t.Error("different hash should miss")
	}
}
