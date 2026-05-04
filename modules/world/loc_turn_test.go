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
