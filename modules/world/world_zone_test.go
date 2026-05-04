package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/zone"
)

func newZoneTestServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	s.zonesTracking = map[*zone.Zone]struct{}{}
	return s
}

func TestServerAddLocTracksZone(t *testing.T) {
	s := newZoneTestServer(t)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	if len(s.zonesTracking) != 1 {
		t.Errorf("zonesTracking: got %d, want 1", len(s.zonesTracking))
	}
}

func TestServerAddObjRoutesByCoord(t *testing.T) {
	s := newZoneTestServer(t)
	objA := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 10)
	objB := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 995, 10)
	s.AddObj(objA, zone.PublicReceiver)
	s.AddObj(objB, zone.PublicReceiver)
	if len(s.zonesTracking) != 2 {
		t.Errorf("zonesTracking: got %d, want 2 (distinct zones)", len(s.zonesTracking))
	}
}

func TestServerChangeObjPassesCurrentTick(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 42
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 10)
	obj.ReceiverID = 7
	s.ChangeObj(obj, 10, 25)
	if obj.Count != 25 {
		t.Errorf("Count: got %d, want 25", obj.Count)
	}
	if obj.LastChange != 42 {
		t.Errorf("LastChange: got %d, want 42", obj.LastChange)
	}
}

func TestServerDispatchersTrackOncePerZone(t *testing.T) {
	s := newZoneTestServer(t)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	s.ChangeLoc(loc, loc.Type(), loc.Shape(), loc.Angle(), 0)
	s.AnimLoc(loc, 42)
	if len(s.zonesTracking) != 1 {
		t.Errorf("zonesTracking: got %d, want 1 (same zone, 3 mutations)", len(s.zonesTracking))
	}
}

func TestServerAddLocAddsCollisionWhenBlockwalk(t *testing.T) {
	s := newZoneTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	s.locTypes.Configs[100] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 100},
		BlockWalk:  true,
		BlockRange: true,
	}
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	if !loc.IsActive {
		t.Error("AddLoc must set IsActive=true")
	}
	if !s.gamemap.Pathfinder.Flags.IsFlagged(3094, 3106, 0, collision.FlagWallWest) {
		t.Error("AddLoc with BlockWalk=true should set FlagWallWest at (3094,3106,0)")
	}
}

func TestServerAddLocSkipsCollisionWhenNotBlockwalk(t *testing.T) {
	s := newZoneTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	s.locTypes.Configs[100] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 100},
		BlockWalk:  false,
	}
	// Pre-allocate the flagmap zone so IsFlagged returns false (not FlagNull = -1)
	// for an unset flag rather than mis-reporting all flags set.
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3094, 3106, 0)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	if s.gamemap.Pathfinder.Flags.IsFlagged(3094, 3106, 0, collision.FlagWallWest) {
		t.Error("AddLoc with BlockWalk=false should not set wall collision")
	}
}

func TestServerChangeLocSwapsCollision(t *testing.T) {
	s := newZoneTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	s.locTypes.Configs[100] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 100},
		BlockWalk:  true,
		BlockRange: true,
	}
	s.locTypes.Configs[101] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 101},
		BlockWalk:  false,
	}
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	if !s.gamemap.Pathfinder.Flags.IsFlagged(3094, 3106, 0, collision.FlagWallWest) {
		t.Fatal("setup: expected FlagWallWest after AddLoc")
	}
	s.ChangeLoc(loc, 101, loc.Shape(), loc.Angle(), 1)
	if s.gamemap.Pathfinder.Flags.IsFlagged(3094, 3106, 0, collision.FlagWallWest) {
		t.Error("ChangeLoc to non-blockwalk type should clear FlagWallWest")
	}
	if loc.Type() != 101 {
		t.Errorf("loc.Type after Change: got %d, want 101", loc.Type())
	}
}

func TestServerRemoveLocClearsCollision(t *testing.T) {
	s := newZoneTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	s.locTypes.Configs[100] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 100},
		BlockWalk:  true,
		BlockRange: true,
	}
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	s.RemoveLoc(loc, 0)
	if loc.IsActive {
		t.Error("RemoveLoc must set IsActive=false")
	}
	if s.gamemap.Pathfinder.Flags.IsFlagged(3094, 3106, 0, collision.FlagWallWest) {
		t.Error("RemoveLoc should clear FlagWallWest")
	}
}

func TestServerChangeLocOnInactiveDespawnIsNoOp(t *testing.T) {
	s := newZoneTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	s.locTypes.Configs[100] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 100},
		BlockWalk:  true,
		BlockRange: true,
	}
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	// Never AddLoc → IsActive stays false
	s.ChangeLoc(loc, 100, 0, 1, 1)
	if loc.Angle() == 1 {
		t.Error("ChangeLoc on inactive DESPAWN must early-return; angle not mutated")
	}
}
