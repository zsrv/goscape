package world

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// TestNAI95_StaticLocCollision_HansArea pins NAI-95: populateStaticLocsIntoZones
// must write FlagBlockWalk into FlagMap for each static loc whose LocType has
// BlockWalk=true. Pre-NAI-95, only the runtime AddLoc path wrote collision;
// boot-time static locs (e.g., Lumbridge castle walls around Hans) were skipped.
//
// Smoke symptom (NAI-92 surfaced, NAI-94 diagnosed): player click on Hans
// produced waypoint_idx=-1 because BFS read FlagNull for unallocated zones
// in the castle interior.
//
// Test exercises the real m48_50 / l48_50 cache. Skip-if-absent keeps the
// test CI-portable; pattern mirrors pkg/objtype/loctype_realcache_test.go.
func TestNAI95_StaticLocCollision_HansArea(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "maps", "m48_50")); err != nil {
		t.Skipf("data/pack/server/maps/m48_50 unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}

	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
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
		// Post-fix: true (castle walls in zone write FlagBlockWalk via ChangeLoc).
		if !s.gamemap.Pathfinder.Flags.IsZoneAllocated(3216, 3216, 0) {
			t.Errorf("zone (3216, 3216, 0) [Hans area]: expected allocated post-NAI-95; got unallocated")
		}
		// Player-spawn zone (covers (3216-3223, 3224-3231) at level 0).
		if !s.gamemap.Pathfinder.Flags.IsZoneAllocated(3216, 3224, 0) {
			t.Errorf("zone (3216, 3224, 0) [player walk-in zone]: expected allocated post-NAI-95; got unallocated")
		}
	})

	t.Run("FindPathPlain_HansCheb2", func(t *testing.T) {
		// Mirrors NAI-94's TestNAI94_AllocatedZones_PathfinderWorks/HansCheb2
		// but against the real cache's static-loc collision instead of synthetic
		// internal.BuildCollisionMap. Post-NAI-95 the production cache must
		// produce the same shape: Success=true Alternative=false single waypoint
		// at the dest tile.
		route := s.gamemap.Pathfinder.FindPathPlain(0, 3219, 3224, 3219, 3222)
		if !route.Success {
			t.Errorf("Success: got false, want true; route=%+v", route)
		}
		if route.Alternative {
			t.Errorf("Alternative: got true (moveNear fell back), want false; route=%+v", route)
		}
		if len(route.Waypoints) != 1 {
			t.Fatalf("Waypoints len: got %d, want 1; route=%+v", len(route.Waypoints), route)
		}
		w := route.Waypoints[0]
		if w.X() != 3219 || w.Z() != 3222 || w.Level() != 0 {
			t.Errorf("Waypoints[0]: got (%d, %d, %d), want (3219, 3222, 0)",
				w.X(), w.Z(), w.Level())
		}
	})

	t.Run("WallTileBlocked", func(t *testing.T) {
		// Dynamic positive pin: find the first static loc whose LocType has
		// BlockWalk=true, then assert FlagMap.Get for its tile has FlagBlockWalk
		// set. Don't hardcode a specific castle-wall coord — the cache may
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
			if flag&collision.FlagBlockWalk == 0 {
				t.Errorf("static loc %d at (%d, %d, %d) BlockWalk=true: flag=0x%x missing FlagBlockWalk bit (0x%x)",
					loc.Type(), loc.X, loc.Z, loc.Level, flag, collision.FlagBlockWalk)
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
