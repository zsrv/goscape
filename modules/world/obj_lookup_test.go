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
	obj.IsActive = true
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
	obj.IsActive = true
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
	obj.IsActive = true
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
	obj.IsActive = true
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
	obj.IsActive = true
	z := s.zoneMap.Get(0, 3200, 3208)
	z.Objs = append(z.Objs, obj)

	got := s.GetObj(0, 3200, 3200, 42, -1) // asking for (3200,3200)
	if got != nil {
		t.Errorf("GetObj: got %v, want nil (wrong tile)", got)
	}
}

// TestServerGetObjSkipsInvalidObjs pins L9: GetObj filters out invalid objs the
// way TS Zone.getObj does via getObjsSafe → isValid (count >= 1 && isActive).
func TestServerGetObjSkipsInvalidObjs(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	z := s.zoneMap.Get(0, 3200, 3200)

	// Depleted (count < 1) must not be returned.
	depleted := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 42, 0)
	depleted.IsActive = true
	z.Objs = append(z.Objs, depleted)
	if got := s.GetObj(0, 3200, 3200, 42, 99); got != nil {
		t.Errorf("GetObj returned a depleted (count<1) obj: %v", got)
	}

	// Inactive (e.g. a taken static obj awaiting respawn) must not be returned.
	inactive := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleRespawn, 43, 1)
	inactive.IsActive = false
	z.Objs = append(z.Objs, inactive)
	if got := s.GetObj(0, 3200, 3200, 43, 99); got != nil {
		t.Errorf("GetObj returned an inactive obj: %v", got)
	}
}

// TestGetObjOfReceiver_SkipsInvalidObjs pins the zone-sub-4 fix: TS
// Zone.getObjOfReceiver (Zone.ts:362-369) iterates getObjsSafe (Zone.ts:423-429)
// which gates each yielded obj on obj.isValid() = count >= 1 && isActive
// (Obj.ts:52-62, hash-less form). goscape's pre-fix loop matched on (x,z,type,
// receiver) alone, so a depleted (count<1) or removed (!isActive) obj lingering
// in zn.Objs with a matching ReceiverID would be returned to world.AddObj's
// merge-decision path and a fresh drop would silently merge into a stale pile.
func TestGetObjOfReceiver_SkipsInvalidObjs(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	z := s.zoneMap.Get(0, 3200, 3200)

	// Depleted (count == 0) private obj must not be returned as a merge target.
	depleted := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 42, 0)
	depleted.IsActive = true
	depleted.ReceiverID = 5
	z.Objs = append(z.Objs, depleted)
	if got := s.getObjOfReceiver(0, 3200, 3200, 42, 5); got != nil {
		t.Errorf("getObjOfReceiver returned a depleted (count<1) obj: %v (TS getObjsSafe Obj.isValid count gate must strip)", got)
	}

	// Inactive private obj at a different type must not be returned as a merge
	// target — the !isActive branch fires independently of count.
	inactive := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleRespawn, 43, 1)
	inactive.IsActive = false
	inactive.ReceiverID = 5
	z.Objs = append(z.Objs, inactive)
	if got := s.getObjOfReceiver(0, 3200, 3200, 43, 5); got != nil {
		t.Errorf("getObjOfReceiver returned an inactive obj: %v (TS getObjsSafe Obj.isValid isActive gate must strip)", got)
	}

	// Control: a healthy private obj at the same receiver remains a valid
	// merge target — the new filter is not over-broad.
	healthy := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 44, 7)
	healthy.IsActive = true
	healthy.ReceiverID = 5
	z.Objs = append(z.Objs, healthy)
	if got := s.getObjOfReceiver(0, 3200, 3200, 44, 5); got != healthy {
		t.Errorf("getObjOfReceiver dropped a healthy merge target: got %v, want %v", got, healthy)
	}
}
