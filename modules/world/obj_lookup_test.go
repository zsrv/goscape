package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

func TestServerGetObjReturnsPublicObjWhenPresent(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 42, 1)
	// ReceiverID -1 == public
	z := s.zoneMap.Get(0, 3200, 3200)
	z.Objs = append(z.Objs, obj)

	got := s.GetObj(0, 3200, 3200, 42, 99)
	if got != obj {
		t.Errorf("GetObj: got %v, want obj", got)
	}
}

func TestServerGetObjReturnsPrivateObjForMatchingReceiver(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 42, 1)
	obj.ReceiverID = 5 // privately owned by player slot 5
	z := s.zoneMap.Get(0, 3200, 3200)
	z.Objs = append(z.Objs, obj)

	got := s.GetObj(0, 3200, 3200, 42, 5)
	if got != obj {
		t.Errorf("GetObj: got %v, want obj (matching receiver)", got)
	}
}

func TestServerGetObjRejectsPrivateObjForNonMatchingReceiver(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 42, 1)
	obj.ReceiverID = 5 // owned by slot 5
	z := s.zoneMap.Get(0, 3200, 3200)
	z.Objs = append(z.Objs, obj)

	got := s.GetObj(0, 3200, 3200, 42, 9) // different receiver
	if got != nil {
		t.Errorf("GetObj: got %v, want nil (receiver mismatch)", got)
	}
}

func TestServerGetObjReturnsNilWhenAbsent(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	if got := s.GetObj(0, 3200, 3200, 42, -1); got != nil {
		t.Errorf("GetObj: got %v, want nil (empty zone)", got)
	}
}

func TestServerGetObjFiltersByTypeID(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 99, 1)
	z := s.zoneMap.Get(0, 3200, 3200)
	z.Objs = append(z.Objs, obj)

	got := s.GetObj(0, 3200, 3200, 42, -1) // looking for type 42, only 99 present
	if got != nil {
		t.Errorf("GetObj: got %v, want nil (wrong typeID)", got)
	}
}

func TestServerGetObjFiltersByCoords(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	// obj at (3200, 3208) — same zone, different tile
	obj := entitypkg.NewObj(0, 3200, 3208, entitypkg.LifecycleDespawn, 42, 1)
	z := s.zoneMap.Get(0, 3200, 3208)
	z.Objs = append(z.Objs, obj)

	got := s.GetObj(0, 3200, 3200, 42, -1) // asking for (3200,3200)
	if got != nil {
		t.Errorf("GetObj: got %v, want nil (wrong tile)", got)
	}
}
