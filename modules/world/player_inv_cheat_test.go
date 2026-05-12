package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
)

// TestPlayerInvAdd_NonStackable_FillsSlots pins TS Player.invAdd
// (Player.ts:1496-1504) on a non-stackable item: each unit takes a new
// slot, transaction.completed equals the units actually inserted.
func TestPlayerInvAdd_NonStackable_FillsSlots(t *testing.T) {
	p, _, s := teleTestPlayer(t)
	invID := mustSetupTestInv(t, s, /*invTypeID=*/ 0, /*capacity=*/ 28)
	objID := mustSetupTestObj(t, s, /*objTypeID=*/ 1277, /*stackable=*/ false)

	completed := p.InvAdd(invID, objID, 5, false)

	if completed != 5 {
		t.Errorf("InvAdd returned %d, want 5", completed)
	}
	inv := s.invLookup.Get(p, invID)
	if inv == nil {
		t.Fatalf("inv lookup returned nil")
	}
	count := countSlots(inv, objID)
	if count != 5 {
		t.Errorf("filled slots = %d, want 5", count)
	}
}

// TestPlayerInvAdd_Stackable_OneSlot pins that a stackable obj
// accumulates into one slot.
func TestPlayerInvAdd_Stackable_OneSlot(t *testing.T) {
	p, _, s := teleTestPlayer(t)
	invID := mustSetupTestInv(t, s, 0, 28)
	objID := mustSetupTestObj(t, s, 995 /*coins*/, true /*stackable*/)

	completed := p.InvAdd(invID, objID, 100, false)

	if completed != 100 {
		t.Errorf("InvAdd returned %d, want 100", completed)
	}
	inv := s.invLookup.Get(p, invID)
	total := totalUnits(inv, objID)
	if total != 100 {
		t.Errorf("total units = %d, want 100", total)
	}
}

// TestPlayerInvAdd_OverflowNotDropped pins TS L1496-1504 bare behavior:
// overflow is NOT dropped to the floor (unlike performInvAdd which does
// the INV_ADD opcode body's overflow drop). transaction.completed
// reports the insertion count; the rest is silently lost.
func TestPlayerInvAdd_OverflowNotDropped(t *testing.T) {
	p, _, s := teleTestPlayer(t)
	invID := mustSetupTestInv(t, s, 0, 28)
	objID := mustSetupTestObj(t, s, 1277, false) // non-stackable

	completed := p.InvAdd(invID, objID, 30, false) // 28-capacity, 2 overflow

	if completed != 28 {
		t.Errorf("InvAdd returned %d, want 28 (overflow not dropped, just discarded)", completed)
	}
	inv := s.invLookup.Get(p, invID)
	if c := countSlots(inv, objID); c != 28 {
		t.Errorf("filled slots = %d, want 28 (full)", c)
	}
}

// TestPlayerInvAdd_NilInv_ReturnsZero pins defensive behavior when inv
// lookup fails (TS throws — goscape returns 0 for safer test ergonomics;
// documented as DEVIATION-NAI-184-D2-INVADD-NIL-RETURN).
func TestPlayerInvAdd_NilInv_ReturnsZero(t *testing.T) {
	p, _, _ := teleTestPlayer(t)
	completed := p.InvAdd(9999, 1277, 1, false) // invalid invTypeID

	if completed != 0 {
		t.Errorf("InvAdd with invalid invTypeID = %d, want 0", completed)
	}
}

// --- helpers ---

func mustSetupTestInv(t *testing.T, s *Server, invTypeID, capacity int) int {
	t.Helper()
	if s.invTypes == nil {
		s.invTypes = &objtype.InvTypeConfigs{
			Configs: make([]*objtype.InvType, invTypeID+1),
		}
	}
	for len(s.invTypes.Configs) <= invTypeID {
		s.invTypes.Configs = append(s.invTypes.Configs, nil)
	}
	cfg := objtype.NewInvType(invTypeID)
	cfg.Size = capacity
	cfg.Scope = objtype.InvTypeScopeTemp
	s.invTypes.Configs[invTypeID] = cfg
	// newTestServer/teleTestPlayer doesn't wire invLookup; do it here so
	// callers' s.invLookup.Get(p, id) resolves the per-player inv.
	s.invLookup = invLookupView{s: s}
	return invTypeID
}

func mustSetupTestObj(t *testing.T, s *Server, objTypeID int, stackable bool) int {
	t.Helper()
	if s.objTypes == nil {
		s.objTypes = &objtype.ObjTypeConfigs{
			Configs: make([]*objtype.ObjType, 2000),
		}
	}
	for len(s.objTypes.Configs) <= objTypeID {
		s.objTypes.Configs = append(s.objTypes.Configs, nil)
	}
	s.objTypes.Configs[objTypeID] = &objtype.ObjType{
		Stackable: stackable,
	}
	return objTypeID
}

// mustSetupNamedObj is like mustSetupTestObj but also wires the
// ObjTypeConfigs.ConfigNames index so that ByName(name) resolves to the
// object. Required for ::give / ::givemany tests which look up by name.
func mustSetupNamedObj(t *testing.T, s *Server, objTypeID int, name string, stackable bool) int {
	t.Helper()
	mustSetupTestObj(t, s, objTypeID, stackable)
	// mustSetupTestObj leaves the embedded ConfigType.ID at zero; ::give
	// callers read objType.ID so we must set it to the true obj id here.
	s.objTypes.Configs[objTypeID].ID = objTypeID
	s.objTypes.Configs[objTypeID].DebugName = name
	if s.objTypes.ConfigNames == nil {
		s.objTypes.ConfigNames = map[string]int{}
	}
	s.objTypes.ConfigNames[name] = objTypeID
	return objTypeID
}

func countSlots(inv *inventory.Inventory, objID int) int {
	n := 0
	for _, item := range inv.Items {
		if item != nil && item.Id == objID {
			n++
		}
	}
	return n
}

func totalUnits(inv *inventory.Inventory, objID int) int {
	total := 0
	for _, item := range inv.Items {
		if item != nil && item.Id == objID {
			total += item.Count
		}
	}
	return total
}
