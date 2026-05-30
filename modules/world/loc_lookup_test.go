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
	// Mirror pkg/zone.Zone.AddStaticLoc — raw-append leaves IsActive=false,
	// which the new GetLoc isActive filter (TS Zone.ts:471-477) would
	// strip. Set true here to preserve this test's named intent
	// (GetLoc returns a present, active loc). See
	// TestServerGetLoc_FiltersInactiveLoc for the negative side.
	loc.IsActive = true

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
	otherLoc.IsActive = true // see TestServerGetLocReturnsLocWhenPresent for rationale.

	if got := s.GetLoc(0, 3200, 3200, 42); got != nil {
		t.Errorf("GetLoc: got %v, want nil (typeID 42 absent, only 99 present)", got)
	}
}

// TestServerGetLoc_FiltersInactiveLoc pins TS Zone.getLoc → getLocsSafe
// → isValid (==isActive) filter (Zone.ts:259-266, 471-477; Entity.ts:
// 32-34). Closes h-loc-1 / h-loc-2 / zone-sub-5 / entity-base-3.
//
// Before the fix, Server.GetLoc skipped the IsActive check and returned
// any matching loc — so a Loc that had been removed (or never activated)
// but was still linked into zn.Locs would resurface, letting OpLoc /
// OpLocT / OpLocU clicks and the LOC_FIND script op succeed against a
// stale target.
//
// Toggle-off proof: revert the `&& l.IsActive` clause in
// loc_lookup.go's Server.GetLoc inner loop → this test fails with
// "GetLoc(inactive): got <loc>, want nil".
func TestServerGetLoc_FiltersInactiveLoc(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	loc := entitypkg.NewLoc(0, 3200, 3200, 1, 1, entitypkg.LifecycleForever, 42, 10, 0)
	z := s.zoneMap.Get(0, 3200, 3200)
	z.Locs = append(z.Locs, loc)
	// IsActive intentionally NOT set — models a loc that was removed
	// (Zone.removeLoc flips isActive=false but, for RESPAWN locs, keeps
	// it linked in the zone list) or one that never finished AddLoc.

	if got := s.GetLoc(0, 3200, 3200, 42); got != nil {
		t.Errorf("GetLoc(inactive): got %v, want nil (TS Zone.ts:471-477 isValid filter must strip inactive locs)", got)
	}
}
