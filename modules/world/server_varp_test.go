package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/zone"
)

// TestWorldVarsView_TotalNpcs pins that TotalNpcs counts non-nil slots in
// Server.npcs. Mirrors TS World.getTotalNpcs (npcs.count, World.ts:1734-1736).
func TestWorldVarsView_TotalNpcs(t *testing.T) {
	s := newZoneTestServer(t)
	w := worldVarsView{s: s}

	// Empty: 0.
	if got := w.TotalNpcs(); got != 0 {
		t.Fatalf("TotalNpcs empty: got %d, want 0", got)
	}

	// Seed 3 NPCs at arbitrary slots.
	s.npcs[0] = newTestNpc(0)
	s.npcs[7] = newTestNpc(7)
	s.npcs[42] = newTestNpc(42)
	if got := w.TotalNpcs(); got != 3 {
		t.Errorf("TotalNpcs after 3 seeded: got %d, want 3", got)
	}

	// Remove one: count drops.
	s.npcs[7] = nil
	if got := w.TotalNpcs(); got != 2 {
		t.Errorf("TotalNpcs after nil slot 7: got %d, want 2", got)
	}

	// Nil-server defensive.
	wn := worldVarsView{}
	if got := wn.TotalNpcs(); got != 0 {
		t.Errorf("TotalNpcs nil-server: got %d, want 0", got)
	}
}

// TestWorldVarsView_TotalZonesLocsObjs pins that the zone-count accessors
// delegate to Server.zoneMap. Rather than hard-coding brittle literals, we
// assert the returned values equal the zoneMap's own counts — confirming the
// delegation plumbing is correct regardless of how many zones the fixture
// materialises. Mirrors TS GameMap.getTotalZones/Locs/Objs (GameMap.ts:102-112).
func TestWorldVarsView_TotalZonesLocsObjs(t *testing.T) {
	s := newZoneTestServer(t)
	w := worldVarsView{s: s}

	// Materialise zones by adding a loc and an obj (side-effect: zoneMap.Get
	// creates the zone entry on first access). Add to two different tiles so
	// we cover distinct zones for loc and obj.
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc, 0)

	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 995, 1)
	s.AddObj(obj, zone.PublicReceiver, 0, 0)

	// Zones: assert delegation matches zoneMap directly.
	if got, want := w.TotalZones(), s.zoneMap.ZoneCount(); got != want {
		t.Errorf("TotalZones: got %d, want %d (zoneMap.ZoneCount)", got, want)
	}
	// Locs: at least 1 from the loc we added.
	if got, want := w.TotalLocs(), s.zoneMap.LocCount(); got != want {
		t.Errorf("TotalLocs: got %d, want %d (zoneMap.LocCount)", got, want)
	}
	if got := w.TotalLocs(); got < 1 {
		t.Errorf("TotalLocs: got %d, want >=1 (at least the seeded loc)", got)
	}
	// Objs: at least 1 from the obj we added.
	if got, want := w.TotalObjs(), s.zoneMap.ObjCount(); got != want {
		t.Errorf("TotalObjs: got %d, want %d (zoneMap.ObjCount)", got, want)
	}
	if got := w.TotalObjs(); got < 1 {
		t.Errorf("TotalObjs: got %d, want >=1 (at least the seeded obj)", got)
	}

	// Nil-server defensives.
	wn := worldVarsView{}
	if got := wn.TotalZones(); got != 0 {
		t.Errorf("TotalZones nil-server: got %d, want 0", got)
	}
	if got := wn.TotalLocs(); got != 0 {
		t.Errorf("TotalLocs nil-server: got %d, want 0", got)
	}
	if got := wn.TotalObjs(); got != 0 {
		t.Errorf("TotalObjs nil-server: got %d, want 0", got)
	}
}

// TestWorldVarsView_MapProjAnim_Delegates pins that the WorldVars
// MapProjAnim method routes through Server.MapProjAnim →
// Zone.MapProjAnim, producing an enclosed ZoneOpMapProjAnim event in
// the source-coord zone. NAI-150 T4.
func TestWorldVarsView_MapProjAnim_Delegates(t *testing.T) {
	s := newZoneTestServer(t)
	w := worldVarsView{s: s}

	// Args mirror pkg/zone TestMapProjAnimEnclosed (zone_test.go:307)
	// 13-arg signature on top of (level=0).
	w.MapProjAnim(0, 3, 4, 5, 7, 0, 100, 10, 0, 0, 50, 40, 30)

	if len(s.zonesTracking) != 1 {
		t.Fatalf("zonesTracking: got %d, want 1", len(s.zonesTracking))
	}
	var z *zone.Zone
	for k := range s.zonesTracking {
		z = k
	}
	events := z.Events()
	if len(events) == 0 {
		t.Fatalf("zone events: got 0, want 1 (MapProjAnim)")
	}
	e := events[0]
	if e.Type != zone.ZoneEventEnclosed {
		t.Errorf("event type: got %v, want Enclosed", e.Type)
	}
	if e.Bytes[0] != rsbuf.ZoneOpMapProjAnim {
		t.Errorf("opcode: got %d, want ZoneOpMapProjAnim=%d", e.Bytes[0], rsbuf.ZoneOpMapProjAnim)
	}
}

// TestWorldVarsView_LookupNpcBySlot table-pins slot resolution against
// Server.npcs. NAI-150 T4.
func TestWorldVarsView_LookupNpcBySlot(t *testing.T) {
	s := newZoneTestServer(t)
	w := worldVarsView{s: s}

	// Empty server: any slot returns nil.
	if got := w.LookupNpcBySlot(0); got != nil {
		t.Errorf("empty server slot 0: got %v, want nil", got)
	}
	if got := w.LookupNpcBySlot(100); got != nil {
		t.Errorf("empty server slot 100: got %v, want nil", got)
	}

	// OOB negative.
	if got := w.LookupNpcBySlot(-1); got != nil {
		t.Errorf("OOB slot -1: got %v, want nil", got)
	}

	// OOB positive (Server.npcs is fixed-size [8192]*Npc — server.go:93).
	if got := w.LookupNpcBySlot(8192); got != nil {
		t.Errorf("OOB slot 8192: got %v, want nil", got)
	}
	if got := w.LookupNpcBySlot(99999); got != nil {
		t.Errorf("OOB slot 99999: got %v, want nil", got)
	}

	// Populated slot returns the registered NPC.
	n := newTestNpc(7)
	s.npcs[7] = n
	got := w.LookupNpcBySlot(7)
	if got == nil {
		t.Fatalf("populated slot 7: got nil, want non-nil")
	}
	if got.Nid() != 7 {
		t.Errorf("populated slot 7: got Nid=%d, want 7", got.Nid())
	}

	// Adjacent unpopulated slot still returns nil.
	if got := w.LookupNpcBySlot(8); got != nil {
		t.Errorf("unpopulated slot 8 adjacent to populated 7: got %v, want nil", got)
	}

	// Nil-server defensive (sanity).
	wn := worldVarsView{}
	if got := wn.LookupNpcBySlot(7); got != nil {
		t.Errorf("nil-server: got %v, want nil", got)
	}
}

// TestWorldVarsViewRemoveObj_PlumbsDuration pins that the adapter
// forwards the caller's duration arg through to Server.RemoveObj.
// Single-player world (empty s.players) gives identity scaling, so an
// active RESPAWN obj ends up with LifecycleTick == currentTick + 42.
// NAI-178 B2.
func TestWorldVarsViewRemoveObj_PlumbsDuration(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 10
	w := worldVarsView{s: s}

	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleRespawn, 995, 1)
	obj.IsActive = true

	w.RemoveObj(obj, 42)

	if got, want := obj.LifecycleTick, s.currentTick+42; got != want {
		t.Errorf("obj.LifecycleTick: got %d, want %d (duration plumbed through, identity scale)", got, want)
	}
}
