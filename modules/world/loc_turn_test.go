package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// newLocTurnTestServer is a fixture for tick-driven Loc.Turn tests.
// Wires gamemap + locTypes + zoneMap + tracker so AddLoc/ChangeLoc work end-to-end.
func newLocTurnTestServer(t *testing.T) *Server {
	s := newZoneTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	s.locTypes.Configs[100] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 100}, BlockWalk: true, BlockRange: true,
	}
	s.locTypes.Configs[101] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 101}, BlockWalk: false,
	}
	return s
}

func TestRevertLocSnapsToBaseInfoAndCollision(t *testing.T) {
	s := newLocTurnTestServer(t)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleRespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	s.ChangeLoc(loc, 101, loc.Shape(), loc.Angle(), 1)
	if s.gamemap.Pathfinder.Flags.IsFlagged(3094, 3106, 0, collision.FlagWallWest) {
		t.Fatal("setup: changed-to-101 should clear FlagWallWest")
	}
	if !loc.IsChanged() {
		t.Fatal("setup: loc should be IsChanged after Change")
	}
	s.RevertLoc(loc)
	if loc.IsChanged() {
		t.Error("after Revert, IsChanged must be false")
	}
	if loc.Type() != 100 {
		t.Errorf("after Revert, Type: got %d, want 100", loc.Type())
	}
	if !s.gamemap.Pathfinder.Flags.IsFlagged(3094, 3106, 0, collision.FlagWallWest) {
		t.Error("after Revert, FlagWallWest should be restored")
	}
}

func TestTurnLocDespawnFiresRemove(t *testing.T) {
	s := newLocTurnTestServer(t)
	s.currentTick = 100
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 5) // schedule despawn at tick 105

	s.currentTick = 105
	s.turnLoc(loc, 105)

	if loc.IsActive {
		t.Error("after turnLoc DESPAWN at scheduled tick, IsActive must be false")
	}
}

func TestTurnLocRespawnChangedFiresRevert(t *testing.T) {
	s := newLocTurnTestServer(t)
	s.currentTick = 100
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleRespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	s.ChangeLoc(loc, 101, loc.Shape(), loc.Angle(), 5) // schedule revert at tick 105

	s.currentTick = 105
	s.turnLoc(loc, 105)

	if loc.IsChanged() {
		t.Error("after turnLoc RESPAWN+changed at scheduled tick, IsChanged must be false")
	}
	if loc.Type() != 100 {
		t.Errorf("after turnLoc Revert, Type: got %d, want 100", loc.Type())
	}
}

func TestTurnLocRespawnInactiveFiresAdd(t *testing.T) {
	s := newLocTurnTestServer(t)
	s.currentTick = 100
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleRespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	s.RemoveLoc(loc, 5) // schedule re-add at tick 105

	s.currentTick = 105
	s.turnLoc(loc, 105)

	if !loc.IsActive {
		t.Error("after turnLoc RESPAWN+!active at scheduled tick, IsActive must be true (re-added)")
	}
}

func TestTurnLocBeforeScheduledTickIsNoOp(t *testing.T) {
	s := newLocTurnTestServer(t)
	s.currentTick = 100
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 5)

	s.currentTick = 103
	s.turnLoc(loc, 103) // not the scheduled tick (105)

	if !loc.IsActive {
		t.Error("turnLoc before scheduled tick must be no-op")
	}
}

func TestProcessZonesFiresTurnLocAtScheduledTick(t *testing.T) {
	s := newLocTurnTestServer(t)
	s.currentTick = 100
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 3) // schedule despawn at tick 103

	// tick 101, 102: not scheduled
	s.currentTick = 101
	s.processZones()
	if !loc.IsActive {
		t.Errorf("loc must stay active at tick 101 (scheduled 103)")
	}
	s.currentTick = 102
	s.processZones()
	if !loc.IsActive {
		t.Errorf("loc must stay active at tick 102")
	}

	// tick 103: scheduled
	s.currentTick = 103
	s.processZones()
	if loc.IsActive {
		t.Errorf("loc must be deactivated at scheduled tick 103")
	}
}

func TestProcessZonesSnapshotsBeforeIterating(t *testing.T) {
	// turnLoc → RemoveLoc → SetLifeCycle(-1, ..., nil) calls
	// tracker.Unregister mid-iteration. processZones must snapshot
	// to avoid undefined iteration over the modified list.
	s := newLocTurnTestServer(t)
	s.currentTick = 100
	for i := 0; i < 5; i++ {
		loc := entitypkg.NewLoc(0, 3094+i, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
		s.AddLoc(loc, 1) // all schedule despawn at tick 101
	}

	s.currentTick = 101
	// Must not panic and must process all 5
	s.processZones()
}

// TestTurnLocRevertChangedStaticMapLoc is the smoke-equivalent unit
// test for the NAI-88 door-revert bug. Setup mirrors the production
// path: a static map loc (loaded via Zone.AddStaticLoc, never via
// Server.AddLoc) is changed by a script call, then the revert tick
// fires. Pre-NAI-89, this would mis-dispatch to AddLoc because the
// static-loc init path never set IsActive=true and Server.ChangeLoc
// didn't either.
func TestTurnLocRevertChangedStaticMapLoc(t *testing.T) {
	s := newLocTurnTestServer(t)
	s.currentTick = 100

	// Build a static map loc and inject it via Zone.AddStaticLoc to
	// mirror Server.populateStaticLocsIntoZones.
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleRespawn, 100, 0, 0)
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.AddStaticLoc(loc)
	if !loc.IsActive {
		t.Fatal("setup: AddStaticLoc must set IsActive=true (NAI-89 T1)")
	}

	// Script-driven change_loc with duration=5 schedules revert at tick 105.
	s.ChangeLoc(loc, 101, loc.Shape(), loc.Angle(), 5)
	if !loc.IsChanged() {
		t.Fatal("setup: ChangeLoc should have flipped IsChanged=true")
	}
	if !loc.IsActive {
		t.Fatal("setup: ChangeLoc must keep IsActive=true (NAI-89 T3)")
	}

	// Advance to the scheduled tick and dispatch.
	s.currentTick = 105
	s.turnLoc(loc, 105)

	// Post-revert assertions.
	if loc.IsChanged() {
		t.Error("after turnLoc revert at scheduled tick, IsChanged must be false")
	}
	if loc.Type() != 100 {
		t.Errorf("after turnLoc revert, Type: got %d, want 100 (BaseInfo type)", loc.Type())
	}
	if !loc.IsActive {
		t.Error("after turnLoc revert, IsActive must remain true (TS Zone.changeLoc force-flip held through revert)")
	}
	if loc.LifecycleTick != -1 {
		t.Errorf("after turnLoc revert, LifecycleTick: got %d, want -1 (untracked)", loc.LifecycleTick)
	}
}
