package world

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// TestNAI101_FountainPathAround_RealCache pins the full-stack
// pathfinder → queueWaypoints → stepOnce path-around behavior against the
// real Lumbridge cache with NAI-100's fountain footprint coverage.
//
// Scenario: player at (3222, 3225) requests a route to NPC tile (3219, 3230)
// past the 4-tile fountain footprint (3221..3222, 3226..3227). Pre-NAI-101,
// queueWaypoints stored route in natural src→dst order; stepOnce read
// waypoints[n-1]=dest and Face headed straight NW into the FlagLoc-blocked
// (3221, 3226). Post-NAI-101, reversed storage means stepOnce reads first
// direction-change point (3220, 3225), walks W around the fountain, then N,
// then NW to reach (3219, 3229) (entity-reach adjacent S of NPC).
//
// Skip-if-absent guard keeps the test CI-portable; pattern mirrors
// TestNAI95_StaticLocCollision_HansArea.
func TestNAI101_FountainPathAround_RealCache(t *testing.T) {
	cacheDir := ref274CacheDir(t)
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "maps", "m48_50")); err != nil {
		t.Skipf("data/pack/server/maps/m48_50 unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}

	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.gamemap.SetMembers(true) // members world: real-cache pathfinding test, not F2P gating (test cache has no F2P CSV)

	locTypes, err := objtype.LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}
	s.locTypes = locTypes
	s.gamemap.SetLocTypes(locTypes)

	if err := s.gamemap.Init(cacheDir); err != nil {
		t.Fatalf("gamemap.Init: %v", err)
	}

	s.populateStaticLocsIntoZones()

	// Sanity-check NAI-100 footprint coverage still holds at HEAD.
	t.Run("FountainFootprintFlagged", func(t *testing.T) {
		want := [][2]int{{3221, 3226}, {3221, 3227}, {3222, 3226}, {3222, 3227}}
		for _, c := range want {
			flag := s.gamemap.Pathfinder.Flags.Get(c[0], c[1], 0)
			if flag&collision.FlagLoc == 0 {
				t.Errorf("(%d, %d, 0): flag=0x%x missing FlagLoc bit (NAI-100 regression?)", c[0], c[1], flag)
			}
		}
	})

	// Pin the routefinder output shape (3 direction-change points) the
	// stepOnce iteration must traverse. Bundle 0 probe captured this exact
	// shape at HEAD `d58a60f` (post-NAI-100):
	//   [0] (3220, 3225, 0)
	//   [1] (3220, 3229, 0)
	//   [2] (3219, 3230, 0)
	t.Run("FindPathPlain_ProducesDetour", func(t *testing.T) {
		route := s.gamemap.Pathfinder.FindPathPlain(0, 3222, 3225, 3219, 3230)
		if !route.Success {
			t.Fatalf("Success=false; route=%+v", route)
		}
		if route.Alternative {
			t.Fatalf("Alternative=true; want false (full reach, not closest-approach); route=%+v", route)
		}
		if len(route.Waypoints) < 2 {
			t.Fatalf("len(Waypoints)=%d; want ≥2 (detour around fountain); route=%+v", len(route.Waypoints), route)
		}
		// Smoke-evidence pin: last waypoint must be the destination tile.
		last := route.Waypoints[len(route.Waypoints)-1]
		if last.X() != 3219 || last.Z() != 3230 {
			t.Errorf("last waypoint: got (%d, %d), want (3219, 3230)", last.X(), last.Z())
		}
	})

	// Full-stack regression: queue the routefinder's output, tick movement,
	// observe player ends adjacent to dest and stepsTaken > 0.
	t.Run("StepThroughDetour", func(t *testing.T) {
		p, _ := newTestPlayer(t)
		p.client.server = s
		p.x, p.z, p.level = 3222, 3225, 0
		p.run = 1
		p.runanim = 0
		p.moveSpeed = MoveSpeedWalk // bridge input; bridge will elevate to Run
		p.runenergy = 10000

		route := s.gamemap.Pathfinder.FindPathPlain(0, 3222, 3225, 3219, 3230)
		if !route.Success {
			t.Fatalf("FindPathPlain failed: %+v", route)
		}

		packed := make([]int, 0, len(route.Waypoints))
		for _, wp := range route.Waypoints {
			packed = append(packed, coordgrid.PackCoord(wp.Level(), wp.X(), wp.Z()))
		}
		p.queueWaypoints(packed)

		// Tick up to 12 times. Run-step covers ≤2 tiles per tick; the path
		// is ~5-7 tiles around the fountain, so 12 is generous.
		const maxTicks = 12
		stepsTotal := 0
		for tick := 0; tick < maxTicks; tick++ {
			p.resolveMovement()
			stepsTotal += p.stepsTaken
			if p.waypointIndex < 0 {
				break
			}
		}

		if stepsTotal == 0 {
			t.Fatalf("stepsTotal=0; player never moved (path lost on tick 1 — NAI-101 bug not fixed)")
		}
		// Final position: dest tile (3219, 3230). FindPathPlain
		// (vs. FindPathToEntity) reaches dest exactly.
		if p.x != 3219 || p.z != 3230 {
			t.Errorf("final position: got (%d, %d), want (3219, 3230); stepsTotal=%d, waypointIndex=%d",
				p.x, p.z, stepsTotal, p.waypointIndex)
		}
		// Player must NOT have stepped onto a fountain tile.
		// (Stronger: per-step audit. Lighter: just verify final and stepsTotal.)
		if p.x == 3221 || p.x == 3222 {
			if p.z == 3226 || p.z == 3227 {
				t.Errorf("player ended on fountain tile (%d, %d) — collision check bypassed", p.x, p.z)
			}
		}
	})
}
