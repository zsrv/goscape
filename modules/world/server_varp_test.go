package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/zone"
)

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
