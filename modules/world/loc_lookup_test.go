package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

func TestServerGetLocReturnsLocWhenPresent(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	loc := entitypkg.NewLoc(0, 3200, 3200, 1, 1, entitypkg.LifecycleForever, 42, 10, 0)
	z := s.zoneMap.Get(0, 3200, 3200)
	z.Locs = append(z.Locs, loc)

	got := s.GetLoc(0, 3200, 3200, 42)
	if got != loc {
		t.Errorf("GetLoc: got %v, want %v", got, loc)
	}
}

func TestServerGetLocReturnsNilWhenAbsent(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	if got := s.GetLoc(0, 3200, 3200, 42); got != nil {
		t.Errorf("GetLoc: got %v, want nil", got)
	}
}

func TestServerGetLocFiltersByTypeID(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	otherLoc := entitypkg.NewLoc(0, 3200, 3200, 1, 1, entitypkg.LifecycleForever, 99, 10, 0)
	z := s.zoneMap.Get(0, 3200, 3200)
	z.Locs = append(z.Locs, otherLoc)

	if got := s.GetLoc(0, 3200, 3200, 42); got != nil {
		t.Errorf("GetLoc: got %v, want nil (typeID 42 absent, only 99 present)", got)
	}
}
