package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

func TestProcessZonesComputesShared(t *testing.T) {
	s := newZoneTestServer(t)
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)

	// Before processZones: shared is nil.
	var beforeZone *zone.Zone
	for z := range s.zonesTracking {
		beforeZone = z
	}
	if beforeZone == nil {
		t.Fatal("expected a tracked zone")
	}
	if beforeZone.Shared() != nil {
		t.Error("Shared should be nil before processZones")
	}

	s.processZones()

	if beforeZone.Shared() == nil {
		t.Error("Shared should be non-nil after processZones")
	}
}

func TestProcessCleanupResetsAndClearsTracking(t *testing.T) {
	s := newZoneTestServer(t)
	// need valid playersMu and s.players for processCleanup's existing code path.
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)
	s.processZones()

	var trackedZone *zone.Zone
	for z := range s.zonesTracking {
		trackedZone = z
	}
	if trackedZone == nil {
		t.Fatal("expected a tracked zone before cleanup")
	}

	s.processCleanup()

	if trackedZone.Shared() != nil {
		t.Error("Shared should be nil after processCleanup Reset")
	}
	if len(trackedZone.Events()) != 0 {
		t.Error("events should be empty after Reset")
	}
	if len(s.zonesTracking) != 0 {
		t.Errorf("zonesTracking should be empty; got %d entries", len(s.zonesTracking))
	}
}
