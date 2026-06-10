package world

import (
	"os"
	"path/filepath"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
)

// TestNAI95_StaticLocCollision_HansArea pins NAI-95: populateStaticLocsIntoZones
// must write FlagLoc into FlagMap for each static loc whose LocType has
// BlockWalk=true (routed via gamemap.ChangeLocCollision → Pathfinder.ChangeLoc).
// Pre-NAI-95, only the runtime AddLoc path wrote collision;
// boot-time static locs (e.g., Lumbridge castle walls around Hans) were skipped.
//
// Smoke symptom (NAI-92 surfaced, NAI-94 diagnosed): player click on Hans
// produced waypoint_idx=-1 because BFS read FlagNull for unallocated zones
// in the castle interior.
//
// Test exercises the real m48_50 / l48_50 cache. Skip-if-absent keeps the
// test CI-portable; pattern mirrors pkg/objtype/loctype_realcache_test.go.
func TestNAI95_StaticLocCollision_HansArea(t *testing.T) {
	cacheDir := ref244CacheDir(t)
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "maps", "m48_50")); err != nil {
		t.Skipf("data/pack/server/maps/m48_50 unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}

	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.gamemap.SetMembers(true) // members world: real-cache collision test, not F2P gating (test cache has no F2P CSV)
	if err := s.gamemap.Init(cacheDir); err != nil {
		t.Fatalf("gamemap.Init: %v", err)
	}

	locTypes, err := objtype.LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}
	s.locTypes = locTypes

	s.populateStaticLocsIntoZones()

	t.Run("ZoneAllocation_HansArea", func(t *testing.T) {
		// Hans NPC zone (covers (3216-3223, 3216-3223) at level 0).
		// Pre-fix: false (no entity collision writes; static-loc walls don't write).
		// Post-fix: true (castle walls in zone write FlagLoc via ChangeLoc).
		if !s.gamemap.Pathfinder.Flags.IsZoneAllocated(3216, 3216, 0) {
			t.Errorf("zone (3216, 3216, 0) [Hans area]: expected allocated post-NAI-95; got unallocated")
		}
		// Player-spawn zone (covers (3216-3223, 3224-3231) at level 0).
		if !s.gamemap.Pathfinder.Flags.IsZoneAllocated(3216, 3224, 0) {
			t.Errorf("zone (3216, 3224, 0) [player walk-in zone]: expected allocated post-NAI-95; got unallocated")
		}
	})

	t.Run("FindPathPlain_HansCheb2", func(t *testing.T) {
		// Post-NAI-95 the production cache path-shape may include detour
		// waypoints owing to a separate divergence at pkg/gamemap/load.go:9
		// (gameMapBlockMapSquare = 0x2 vs TS BLOCK_MAP_SQUARE = 0x1) that
		// marks (3219, 3223) as floor-blocked. NAI-95 scope is zone
		// allocation; the route reaching the destination tile in any number
		// of waypoints is the NAI-95 success signal. Path-shape optimality
		// is tracked as a NAI-96+ followup.
		route := s.gamemap.Pathfinder.FindPathPlain(0, 3219, 3224, 3219, 3222)
		if !route.Success {
			t.Fatalf("Success: got false, want true; route=%+v", route)
		}
		if len(route.Waypoints) == 0 {
			t.Fatalf("Waypoints empty; expected at least one waypoint reaching (3219, 3222); route=%+v", route)
		}
		last := route.Waypoints[len(route.Waypoints)-1]
		if last.X() != 3219 || last.Z() != 3222 || last.Level() != 0 {
			t.Errorf("last waypoint: got (%d, %d, %d), want (3219, 3222, 0); route=%+v",
				last.X(), last.Z(), last.Level(), route)
		}
	})

	t.Run("WallTileBlocked", func(t *testing.T) {
		// Dynamic positive pin: find the first static loc whose LocType has
		// BlockWalk=true, then assert FlagMap.Get for its tile has FlagLoc set.
		// LocType.BlockWalk routes through gamemap.ChangeLocCollision →
		// Pathfinder.ChangeLoc, which writes FlagLoc (iota 8 = 0x100).
		// FlagBlockWalk (iota 21 = 0x200000) is a separate floor flag written
		// by ChangeFloor / loadGround — unrelated despite the matching identifier
		// name. Don't hardcode a specific castle-wall coord — the cache may
		// shift across builds.
		var found bool
		for _, loc := range s.gamemap.StaticLocs() {
			if loc.Type() < 0 || loc.Type() >= len(s.locTypes.Configs) {
				continue
			}
			lt := s.locTypes.Configs[loc.Type()]
			if lt == nil || !lt.BlockWalk {
				continue
			}
			flag := s.gamemap.Pathfinder.Flags.Get(loc.X, loc.Z, loc.Level)
			if flag == collision.FlagNull {
				t.Errorf("static loc %d at (%d, %d, %d) BlockWalk=true: FlagMap returned FlagNull (zone unallocated post-NAI-95)",
					loc.Type(), loc.X, loc.Z, loc.Level)
				return
			}
			if flag&collision.FlagLoc == 0 {
				t.Errorf("static loc %d at (%d, %d, %d) BlockWalk=true: flag=0x%x missing FlagLoc bit (0x%x)",
					loc.Type(), loc.X, loc.Z, loc.Level, flag, collision.FlagLoc)
				return
			}
			found = true
			break
		}
		if !found {
			t.Skip("no BlockWalk static loc found in cache; cannot pin positive wall-tile collision")
		}
	})
}

// TestNAI96_GroundDecor_Active1_WritesFloor pins TS GameMap.ts:336-340 —
// LocLayer.GROUND_DECOR + active==1 writes ChangeFloor (FlagBlockWalk).
func TestNAI96_GroundDecor_Active1_WritesFloor(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())

	// Build a synthetic LocType: BlockWalk=true, Active=1, BlockRange=false.
	// LocTypeConfigs.Configs is indexed by typeId; index 0 reserved by convention.
	lt := &objtype.LocType{BlockWalk: true, Active: 1}
	s.locTypes = &objtype.LocTypeConfigs{Configs: []*objtype.LocType{nil, lt}}

	// Static loc with GroundDecor shape (ShapeGroundDecor=22) at (3220, 3220, 0).
	// Width/Length 1x1 (matches load.go convention).
	const absX, absZ, level = 3220, 3220, 0
	staticLoc := entitypkg.NewLoc(level, absX, absZ, 1, 1,
		entitypkg.LifecycleRespawn,
		1, /*locId*/
		int(loc.ShapeGroundDecor),
		int(loc.AngleWest))
	s.gamemap.AddStaticLoc(staticLoc)

	// Pre-allocate the touched zone so flag reads return real values.
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, level)

	s.populateStaticLocsIntoZones()

	flag := s.gamemap.Pathfinder.Flags.Get(absX, absZ, level)
	if flag&collision.FlagBlockWalk == 0 {
		t.Errorf("GroundDecor active=1 at (%d, %d, %d): flag=0x%x missing FlagBlockWalk (0x%x)",
			absX, absZ, level, flag, collision.FlagBlockWalk)
	}
}

// TestNAI96_GroundDecor_Active0_NoWrite pins that GroundDecor with active=0
// does not write collision (TS GameMap.ts:337 — only active===1 calls changeFloor).
func TestNAI96_GroundDecor_Active0_NoWrite(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())

	lt := &objtype.LocType{BlockWalk: true, Active: 0}
	s.locTypes = &objtype.LocTypeConfigs{Configs: []*objtype.LocType{nil, lt}}

	const absX, absZ, level = 3220, 3220, 0
	staticLoc := entitypkg.NewLoc(level, absX, absZ, 1, 1,
		entitypkg.LifecycleRespawn,
		1,
		int(loc.ShapeGroundDecor),
		int(loc.AngleWest))
	s.gamemap.AddStaticLoc(staticLoc)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, level)

	s.populateStaticLocsIntoZones()

	flag := s.gamemap.Pathfinder.Flags.Get(absX, absZ, level)
	if flag&collision.FlagBlockWalk != 0 {
		t.Errorf("GroundDecor active=0 at (%d, %d, %d): flag=0x%x unexpectedly has FlagBlockWalk",
			absX, absZ, level, flag)
	}
}

// TestNAI96_WallDecor_NoWrite pins that WallDecor never writes collision
// regardless of active (TS GameMap.ts:326-340 has no WALL_DECOR branch).
func TestNAI96_WallDecor_NoWrite(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())

	lt := &objtype.LocType{BlockWalk: true, Active: 1}
	s.locTypes = &objtype.LocTypeConfigs{Configs: []*objtype.LocType{nil, lt}}

	const absX, absZ, level = 3220, 3220, 0
	// ShapeWallDecorStraightNoOffset = 4 (LayerWallDecor per LayerOf).
	staticLoc := entitypkg.NewLoc(level, absX, absZ, 1, 1,
		entitypkg.LifecycleRespawn,
		1,
		int(loc.ShapeWallDecorStraightNoOffset),
		int(loc.AngleWest))
	s.gamemap.AddStaticLoc(staticLoc)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, level)

	s.populateStaticLocsIntoZones()

	flag := s.gamemap.Pathfinder.Flags.Get(absX, absZ, level)
	if flag != collision.FlagOpen {
		t.Errorf("WallDecor active=1 at (%d, %d, %d): flag=0x%x, want FlagOpen (0x0)",
			absX, absZ, level, flag)
	}
}

// TestNAI96_AngleSwap_North_2x3 pins TS GameMap.ts:331-332 — N/S angles call
// ChangeLoc with (length, width) order, producing a length-along-X,
// width-along-Z footprint.
//
// Goscape Pathfinder.ChangeLoc(x, z, level, w, l, ...) iterates w*l tiles at
// offsets (index%w, index/w). With N/S swap, ChangeLoc receives the loc's
// (length=3, width=2) as its (w, l) args, so X-extent=3 and Z-extent=2 — the
// footprint covers X∈[x..x+2], Z∈[z..z+1] (3 tiles wide, 2 tiles deep).
func TestNAI96_AngleSwap_North_2x3(t *testing.T) {
	s := newZoneTestServer(t)
	s.gamemap = gamemap.New(discardLogger())

	// LocType: BlockWalk=true, Active=0 (LayerGround doesn't gate on active).
	lt := &objtype.LocType{BlockWalk: true, Active: 0}
	s.locTypes = &objtype.LocTypeConfigs{Configs: []*objtype.LocType{nil, lt}}

	// Loc: width=2, length=3, angle=North, LayerGround shape (Centrepiece).
	const absX, absZ, level = 3220, 3220, 0
	dynamicLoc := entitypkg.NewLoc(level, absX, absZ, 2 /*width*/, 3, /*length*/
		entitypkg.LifecycleRespawn,
		1,
		int(loc.ShapeCentrepieceStraight),
		int(loc.AngleNorth))

	// Pre-allocate all tiles in the maximum possible footprint.
	for dx := range 3 {
		for dz := range 3 {
			s.gamemap.Pathfinder.Flags.AllocateIfAbsent(absX+dx, absZ+dz, level)
		}
	}

	// Use AddLoc to exercise the runtime path through ChangeLocCollision.
	s.AddLoc(dynamicLoc, -1)

	// Expected N/S footprint: 3 wide along X, 2 along Z.
	expected := map[[2]int]bool{
		{0, 0}: true, {1, 0}: true, {2, 0}: true,
		{0, 1}: true, {1, 1}: true, {2, 1}: true,
	}
	for dx := range 3 {
		for dz := range 3 {
			flag := s.gamemap.Pathfinder.Flags.Get(absX+dx, absZ+dz, level)
			has := flag&collision.FlagLoc != 0
			want := expected[[2]int{dx, dz}]
			if has != want {
				t.Errorf("N-angled 2x3 loc at (%d, %d, %d) offset (%d, %d): FlagLoc=%v, want %v (flag=0x%x)",
					absX, absZ, level, dx, dz, has, want, flag)
			}
		}
	}
}

// TestNAI96_AngleSwap_East_2x3 pins TS GameMap.ts:333-334 — non-N/S angles
// call ChangeLoc with (width, length) order, producing a width-along-X,
// length-along-Z footprint (2 wide along X, 3 along Z).
func TestNAI96_AngleSwap_East_2x3(t *testing.T) {
	s := newZoneTestServer(t)
	s.gamemap = gamemap.New(discardLogger())

	lt := &objtype.LocType{BlockWalk: true, Active: 0}
	s.locTypes = &objtype.LocTypeConfigs{Configs: []*objtype.LocType{nil, lt}}

	const absX, absZ, level = 3220, 3220, 0
	dynamicLoc := entitypkg.NewLoc(level, absX, absZ, 2 /*width*/, 3, /*length*/
		entitypkg.LifecycleRespawn,
		1,
		int(loc.ShapeCentrepieceStraight),
		int(loc.AngleEast))

	for dx := range 3 {
		for dz := range 3 {
			s.gamemap.Pathfinder.Flags.AllocateIfAbsent(absX+dx, absZ+dz, level)
		}
	}

	s.AddLoc(dynamicLoc, -1)

	// Expected E/W footprint: 2 wide along X, 3 along Z.
	expected := map[[2]int]bool{
		{0, 0}: true, {1, 0}: true,
		{0, 1}: true, {1, 1}: true,
		{0, 2}: true, {1, 2}: true,
	}
	for dx := range 3 {
		for dz := range 3 {
			flag := s.gamemap.Pathfinder.Flags.Get(absX+dx, absZ+dz, level)
			has := flag&collision.FlagLoc != 0
			want := expected[[2]int{dx, dz}]
			if has != want {
				t.Errorf("E-angled 2x3 loc at (%d, %d, %d) offset (%d, %d): FlagLoc=%v, want %v (flag=0x%x)",
					absX, absZ, level, dx, dz, has, want, flag)
			}
		}
	}
}
