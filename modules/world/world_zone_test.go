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
	s.AddObj(objA, zone.PublicReceiver, 0, 0)
	s.AddObj(objB, zone.PublicReceiver, 0, 0)
	if len(s.zonesTracking) != 2 {
		t.Errorf("zonesTracking: got %d, want 2 (distinct zones)", len(s.zonesTracking))
	}
}

// TestServerAddObjEvictsOldestWhenZoneFull pins the per-zone obj cap (H2):
// adding a despawn obj to a zone already at MaxObjs (129) evicts the oldest
// despawn obj first, so the zone never exceeds the cap. Mirrors TS
// Zone.addObj eviction (Zone.ts:281-289). Distinct obj types avoid the
// stack-merge path (unconfigured ObjType → not stackable).
func TestServerAddObjEvictsOldestWhenZoneFull(t *testing.T) {
	s := newZoneTestServer(t)
	const tileX, tileZ = 3094, 3106
	n := zone.MaxObjs + 1
	objs := make([]*entitypkg.Obj, 0, n)
	for i := range n {
		o := entitypkg.NewObj(0, tileX, tileZ, entitypkg.LifecycleDespawn, 1000+i, 1)
		s.AddObj(o, zone.PublicReceiver, 0, 0)
		objs = append(objs, o)
	}
	z := s.zoneMap.Get(0, tileX, tileZ)
	if z.TotalObjs() != zone.MaxObjs {
		t.Errorf("TotalObjs after %d adds: got %d, want %d (cap)", n, z.TotalObjs(), zone.MaxObjs)
	}
	if objs[0].IsActive {
		t.Error("oldest obj should have been evicted (IsActive=false)")
	}
	if !objs[n-1].IsActive {
		t.Error("newest obj should be active")
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
	// Pre-allocate the flagmap zone so IsFlagged returns false (not FlagNull = 0x7FFFFFFF)
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

// --- NAI-151: populateStaticObjsIntoZones wiring ---

func TestPopulateStaticObjsIntoZones_RoutesByCoord(t *testing.T) {
	s := newZoneTestServer(t)
	gm := gamemap.New(discardLogger())
	gm.SetMembers(false)
	gm.SetObjTypes(&objtype.ObjTypeConfigs{Configs: []*objtype.ObjType{
		nil,
		{Members: false},
	}})
	const mapX, mapZ = 50, 50
	const packed1 = 0<<12 | 5<<6 | 5
	header1 := []byte{byte(packed1 >> 8), byte(packed1 & 0xFF), 0x01}
	entry1 := []byte{0x00, 0x01, 0x07} // typeID=1, count=7
	const packed2 = 0<<12 | 50<<6 | 50
	header2 := []byte{byte(packed2 >> 8), byte(packed2 & 0xFF), 0x01}
	entry2 := []byte{0x00, 0x01, 0x08} // typeID=1, count=8
	data := append(header1, entry1...)
	data = append(data, header2...)
	data = append(data, entry2...)
	gm.SetFreeMapForTest(mapX*64+5, mapZ*64+5)
	gm.SetFreeMapForTest(mapX*64+50, mapZ*64+50)
	gm.LoadObjsForTest(data, mapX, mapZ)

	s.gamemap = gm
	s.populateStaticObjsIntoZones()

	zA := s.zoneMap.Get(0, mapX*64+5, mapZ*64+5)
	zB := s.zoneMap.Get(0, mapX*64+50, mapZ*64+50)
	if len(zA.Objs) != 1 || zA.Objs[0].Count != 7 {
		t.Errorf("zone A: got Objs=%v, want one obj count=7", zA.Objs)
	}
	if len(zB.Objs) != 1 || zB.Objs[0].Count != 8 {
		t.Errorf("zone B: got Objs=%v, want one obj count=8", zB.Objs)
	}
	if zA == zB {
		t.Error("zone A and B should be distinct (tiles 8 zones apart)")
	}
}

func TestPopulateStaticObjsIntoZones_LifecycleRespawnAndActive(t *testing.T) {
	s := newZoneTestServer(t)
	gm := gamemap.New(discardLogger())
	gm.SetMembers(false)
	gm.SetObjTypes(&objtype.ObjTypeConfigs{Configs: []*objtype.ObjType{
		nil,
		{Members: false},
	}})
	const mapX, mapZ = 50, 50
	const packed = 0<<12 | 5<<6 | 5
	header := []byte{byte(packed >> 8), byte(packed & 0xFF), 0x01}
	entry := []byte{0x00, 0x01, 0x07}
	gm.SetFreeMapForTest(mapX*64+5, mapZ*64+5)
	gm.LoadObjsForTest(append(header, entry...), mapX, mapZ)
	s.gamemap = gm

	s.populateStaticObjsIntoZones()

	z := s.zoneMap.Get(0, mapX*64+5, mapZ*64+5)
	if len(z.Objs) != 1 {
		t.Fatalf("Objs: got %d, want 1", len(z.Objs))
	}
	o := z.Objs[0]
	if o.Lifecycle != entitypkg.LifecycleRespawn {
		t.Errorf("Lifecycle: got %v, want LifecycleRespawn", o.Lifecycle)
	}
	if !o.IsActive {
		t.Error("IsActive: got false, want true")
	}
	if o.X != mapX*64+5 || o.Z != mapZ*64+5 {
		t.Errorf("Coords: got (%d,%d), want (%d,%d)", o.X, o.Z, mapX*64+5, mapZ*64+5)
	}
}

func TestPopulateStaticObjsIntoZones_EmptySpawnsNoOp(t *testing.T) {
	s := newZoneTestServer(t)
	gm := gamemap.New(discardLogger())
	s.gamemap = gm
	s.populateStaticObjsIntoZones()
}
