package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
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
	s.AddLoc(loc)
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
	s.AddLoc(loc)
	s.ChangeLoc(loc)
	s.AnimLoc(loc, 42)
	if len(s.zonesTracking) != 1 {
		t.Errorf("zonesTracking: got %d, want 1 (same zone, 3 mutations)", len(s.zonesTracking))
	}
}
