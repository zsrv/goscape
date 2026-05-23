package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/zone"
)

// objMergeTestServer is newZoneTestServer plus an ObjType table where id
// 884 is stackable and id 1205 is not, so the merge tests can exercise both
// branches of Server.AddObj's stack-merge.
func objMergeTestServer(t *testing.T) *Server {
	t.Helper()
	s := newZoneTestServer(t)
	cfgs := make([]*objtype.ObjType, 2000)
	cfgs[884] = &objtype.ObjType{Stackable: true}   // arrow-like
	cfgs[1205] = &objtype.ObjType{Stackable: false} // bronze dagger-like
	s.objTypes = &objtype.ObjTypeConfigs{Configs: cfgs}
	return s
}

func objsAt(s *Server, level, x, z int) []*entitypkg.Obj {
	return s.zoneMap.Get(level, x, z).Objs
}

// TestServerAddObj_MergesStackablePublicDrops pins the core user-reported
// behavior: dropping a stackable obj onto a tile that already holds a
// stackable obj of the same type+receiver merges the counts into one pile
// instead of leaving two separate entries. Mirrors TS World.addObj
// (Engine-TS/src/engine/World.ts:1453-1465).
func TestServerAddObj_MergesStackablePublicDrops(t *testing.T) {
	s := objMergeTestServer(t)
	s.currentTick = 10

	first := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 884, 5)
	s.AddObj(first, zone.PublicReceiver, 200, 0)
	second := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 884, 3)
	s.AddObj(second, zone.PublicReceiver, 200, 0)

	objs := objsAt(s, 0, 3200, 3200)
	if len(objs) != 1 {
		t.Fatalf("zone obj count: got %d, want 1 (merged)", len(objs))
	}
	if objs[0] != first {
		t.Error("merge must keep the existing pile, not the new obj")
	}
	if got := objs[0].Count; got != 8 {
		t.Errorf("merged count: got %d, want 8", got)
	}
}

// TestServerAddObj_MergeRefreshesDespawnTimer pins that a merge resets the
// surviving pile's despawn countdown to the new drop's duration, mirroring
// TS `existing.lifecycleTick = duration` (World.ts:1461) expressed in
// goscape's absolute-tick model.
func TestServerAddObj_MergeRefreshesDespawnTimer(t *testing.T) {
	s := objMergeTestServer(t)
	s.currentTick = 10

	first := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 884, 5)
	s.AddObj(first, zone.PublicReceiver, 200, 0)

	s.currentTick = 50
	second := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 884, 3)
	s.AddObj(second, zone.PublicReceiver, 200, 0)

	if got, want := first.LifecycleTick, 50+200; got != want {
		t.Errorf("merged pile LifecycleTick: got %d, want %d (refreshed)", got, want)
	}
}

// TestServerAddObj_DoesNotMergeDifferentReceivers pins that a private drop
// only merges with an existing pile of the *same* receiver — TS uses
// getObjOfReceiver (exact match), not getObj (visibility match).
func TestServerAddObj_DoesNotMergeDifferentReceivers(t *testing.T) {
	s := objMergeTestServer(t)
	s.currentTick = 10

	a := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 884, 5)
	s.AddObj(a, 111, 200, 0)
	b := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 884, 3)
	s.AddObj(b, 222, 200, 0)

	if got := len(objsAt(s, 0, 3200, 3200)); got != 2 {
		t.Errorf("zone obj count: got %d, want 2 (different receivers don't merge)", got)
	}
}

// TestServerAddObj_DoesNotMergeNonStackable pins that non-stackable objs
// never merge (count===1 drops stay separate), matching the stackable guard
// in TS World.addObj.
func TestServerAddObj_DoesNotMergeNonStackable(t *testing.T) {
	s := objMergeTestServer(t)
	s.currentTick = 10

	a := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 1205, 1)
	s.AddObj(a, zone.PublicReceiver, 200, 0)
	b := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 1205, 1)
	s.AddObj(b, zone.PublicReceiver, 200, 0)

	if got := len(objsAt(s, 0, 3200, 3200)); got != 2 {
		t.Errorf("zone obj count: got %d, want 2 (non-stackable doesn't merge)", got)
	}
}

// TestServerAddObj_DoesNotMergeWhenOverStackLimit pins that a merge that
// would exceed StackLimit is rejected, leaving two separate piles — mirrors
// TS `if (nextCount <= Inventory.STACK_LIMIT)` (World.ts:1457).
func TestServerAddObj_DoesNotMergeWhenOverStackLimit(t *testing.T) {
	s := objMergeTestServer(t)
	s.currentTick = 10

	a := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 884, inventory.StackLimit-1)
	s.AddObj(a, zone.PublicReceiver, 200, 0)
	b := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 884, 5)
	s.AddObj(b, zone.PublicReceiver, 200, 0)

	if got := len(objsAt(s, 0, 3200, 3200)); got != 2 {
		t.Errorf("zone obj count: got %d, want 2 (over StackLimit doesn't merge)", got)
	}
}
