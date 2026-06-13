package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	pfloc "github.com/zsrv/goscape/pkg/pathfinder/loc"
	"github.com/zsrv/goscape/pkg/zone"
)

func TestServerStaticLocsPopulateZones(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	s.zonesTracking = map[*zone.Zone]struct{}{}
	s.gamemap = gamemap.New(discardLogger())

	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleRespawn, 100, 0, 0)
	s.gamemap.AddStaticLoc(loc)

	s.populateStaticLocsIntoZones()

	z := s.zoneMap.Get(0, 3094, 3106)
	if len(z.Locs) != 1 || z.Locs[0] != loc {
		t.Errorf("zone should contain the seeded static loc; Locs=%v", z.Locs)
	}
}

func TestPopulateStaticLocsEmptyIsNoop(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	s.zonesTracking = map[*zone.Zone]struct{}{}
	s.gamemap = gamemap.New(discardLogger())

	s.populateStaticLocsIntoZones()

	if s.zoneMap.ZoneCount() != 0 {
		t.Errorf("no statics should create no zones; ZoneCount=%d", s.zoneMap.ZoneCount())
	}
}

// TestStaticLocActiveGate_274 pins the rev-274 GameMap.ts zone-registration
// gate (TS GameMap.ts:281-286 @dee467c8): a static loc is added to its zone
// ONLY when its LocType.active is set, but its collision flags are written
// REGARDLESS of active (the changeLocCollision call is gated only on
// type.blockwalk). TS hunk:
//
//	if (type.blockwalk) { changeLocCollision(...) }   // regardless of active
//	if (type.active)    { ...addStaticLoc(...) }       // gated on active
//
// goscape's populateStaticLocsIntoZones is the analog of TS loadLocations.
func TestStaticLocActiveGate_274(t *testing.T) {
	// Use a wall shape (LayerWall) so collision is written purely on the
	// blockwalk gate, independent of the GroundDecor active==1 collision
	// branch — this isolates the active gate to the zone-add decision.
	const absX, absZ, level = 3094, 3106, 0
	mkServer := func(active int) *Server {
		s := newTestServer(t)
		s.zoneMap = zone.NewZoneMap()
		s.zonesTracking = map[*zone.Zone]struct{}{}
		s.gamemap = gamemap.New(discardLogger())

		lt := &objtype.LocType{BlockWalk: true, BlockRange: false, Active: active, Width: 1, Length: 1}
		s.locTypes = &objtype.LocTypeConfigs{Configs: []*objtype.LocType{nil, lt}}

		l := entitypkg.NewLoc(level, absX, absZ, 1, 1, entitypkg.LifecycleRespawn,
			1 /*locId*/, int(pfloc.ShapeWallStraight), int(pfloc.AngleWest))
		s.gamemap.AddStaticLoc(l)
		return s
	}

	t.Run("active=1: added to zone AND collision written", func(t *testing.T) {
		s := mkServer(1)
		s.populateStaticLocsIntoZones()

		z := s.zoneMap.Get(level, absX, absZ)
		if len(z.Locs) != 1 {
			t.Fatalf("active=1: zone should contain 1 static loc; Locs=%v", z.Locs)
		}
		// A wall shape writes a wall-direction flag (FlagWallWest for AngleWest),
		// not FlagLoc — just assert collision is non-open.
		flag := s.gamemap.Pathfinder.Flags.Get(absX, absZ, level)
		if flag == collision.FlagOpen {
			t.Errorf("active=1: collision must be written (flag != FlagOpen); flag=0x%x", flag)
		}
	})

	t.Run("active=0: ABSENT from zone but collision STILL written", func(t *testing.T) {
		s := mkServer(0)
		s.populateStaticLocsIntoZones()

		z := s.zoneMap.Get(level, absX, absZ)
		if len(z.Locs) != 0 {
			t.Errorf("active=0: loc must NOT be added to zone (TS GameMap.ts:285); Locs=%v", z.Locs)
		}
		flag := s.gamemap.Pathfinder.Flags.Get(absX, absZ, level)
		if flag == collision.FlagOpen {
			t.Errorf("active=0: collision must STILL be written (changeLocCollision is gated only on blockwalk); flag=0x%x", flag)
		}
	})
}
