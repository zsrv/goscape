package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/gamemap"
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
