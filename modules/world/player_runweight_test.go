package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
)

// buildRunWeightServer builds a minimal Server with both invTypes and objTypes
// populated, suitable for calculateRunWeight tests. The caller provides the
// invType configs slice (length must be > invTypeID) and objType configs slice
// (length must be > objTypeID). Returns the server only; the caller must assign
// p.client.server = s.
func buildRunWeightServer(
	invConfigs []*objtype.InvType,
	objConfigs []*objtype.ObjType,
) *Server {
	return &Server{
		log:      discardLogger(),
		invTypes: &objtype.InvTypeConfigs{Configs: invConfigs},
		objTypes: &objtype.ObjTypeConfigs{Configs: objConfigs},
	}
}

// TestCalculateRunWeight_EmptyInvs — player with no invs → runweight stays 0.
func TestCalculateRunWeight_EmptyInvs(t *testing.T) {
	p, _ := newTestPlayer(t)
	invConfigs := make([]*objtype.InvType, 10)
	objConfigs := make([]*objtype.ObjType, 10)
	p.client.server = buildRunWeightServer(invConfigs, objConfigs)
	// p.invs is nil by default (no entries).

	p.calculateRunWeight()

	if p.runweight != 0 {
		t.Errorf("runweight: got %d, want 0", p.runweight)
	}
}

// TestCalculateRunWeight_SkipsNonRunWeightInv — inv with RunWeight=false; item
// has Weight=1000; runweight must remain 0.
func TestCalculateRunWeight_SkipsNonRunWeightInv(t *testing.T) {
	const invTypeID = 1
	const objTypeID = 5
	invConfigs := make([]*objtype.InvType, 10)
	invConfigs[invTypeID] = &objtype.InvType{
		ID:        invTypeID,
		Scope:     objtype.InvTypeScopeTemp,
		Size:      5,
		RunWeight: false,
	}
	objConfigs := make([]*objtype.ObjType, 10)
	objConfigs[objTypeID] = &objtype.ObjType{
		Stackable: false,
		Weight:    1000,
	}

	p, _ := newTestPlayer(t)
	p.client.server = buildRunWeightServer(invConfigs, objConfigs)
	inv := inventory.New(invTypeID, 5, inventory.StackNormal)
	inv.Items[0] = &inventory.Item{Id: objTypeID, Count: 1}
	if p.invs == nil {
		p.invs = make(map[int]*inventory.Inventory)
	}
	p.invs[invTypeID] = inv

	p.calculateRunWeight()

	if p.runweight != 0 {
		t.Errorf("runweight: got %d, want 0 (non-RunWeight inv must be skipped)", p.runweight)
	}
}

// TestCalculateRunWeight_SkipsStackableObj — RunWeight inv, stackable obj × 100;
// stackable items contribute 0 to runweight (TS Player.ts:620-622).
func TestCalculateRunWeight_SkipsStackableObj(t *testing.T) {
	const invTypeID = 1
	const objTypeID = 5
	invConfigs := make([]*objtype.InvType, 10)
	invConfigs[invTypeID] = &objtype.InvType{
		ID:        invTypeID,
		Scope:     objtype.InvTypeScopeTemp,
		Size:      5,
		RunWeight: true,
	}
	objConfigs := make([]*objtype.ObjType, 10)
	objConfigs[objTypeID] = &objtype.ObjType{
		Stackable: true,
		Weight:    500,
	}

	p, _ := newTestPlayer(t)
	p.client.server = buildRunWeightServer(invConfigs, objConfigs)
	inv := inventory.New(invTypeID, 5, inventory.StackAlways)
	inv.Items[0] = &inventory.Item{Id: objTypeID, Count: 100}
	if p.invs == nil {
		p.invs = make(map[int]*inventory.Inventory)
	}
	p.invs[invTypeID] = inv

	p.calculateRunWeight()

	if p.runweight != 0 {
		t.Errorf("runweight: got %d, want 0 (stackable obj must be skipped)", p.runweight)
	}
}

// TestCalculateRunWeight_SkipsNilInvOrItem — RunWeight inv with a nil slot and
// a nil entry in p.invs; no panic, runweight stays 0.
func TestCalculateRunWeight_SkipsNilInvOrItem(t *testing.T) {
	const invTypeID = 1
	invConfigs := make([]*objtype.InvType, 10)
	invConfigs[invTypeID] = &objtype.InvType{
		ID:        invTypeID,
		Scope:     objtype.InvTypeScopeTemp,
		Size:      5,
		RunWeight: true,
	}
	objConfigs := make([]*objtype.ObjType, 10)

	p, _ := newTestPlayer(t)
	p.client.server = buildRunWeightServer(invConfigs, objConfigs)
	// Set up p.invs with a nil entry AND a real inv that has nil items.
	if p.invs == nil {
		p.invs = make(map[int]*inventory.Inventory)
	}
	p.invs[99] = nil // nil value in map
	inv := inventory.New(invTypeID, 5, inventory.StackNormal)
	// All slots remain nil.
	p.invs[invTypeID] = inv

	p.calculateRunWeight()

	if p.runweight != 0 {
		t.Errorf("runweight: got %d, want 0", p.runweight)
	}
}

// TestCalculateRunWeight_SumsWeightTimesCount — RunWeight inv, 4 non-stack
// items with Weight=32g + 1 non-stack item with Weight=100g → 4*32 + 100 = 228.
func TestCalculateRunWeight_SumsWeightTimesCount(t *testing.T) {
	const invTypeID = 1
	const objA = 5 // weight 32
	const objB = 6 // weight 100
	invConfigs := make([]*objtype.InvType, 10)
	invConfigs[invTypeID] = &objtype.InvType{
		ID:        invTypeID,
		Scope:     objtype.InvTypeScopeTemp,
		Size:      5,
		RunWeight: true,
	}
	objConfigs := make([]*objtype.ObjType, 10)
	objConfigs[objA] = &objtype.ObjType{Stackable: false, Weight: 32}
	objConfigs[objB] = &objtype.ObjType{Stackable: false, Weight: 100}

	p, _ := newTestPlayer(t)
	p.client.server = buildRunWeightServer(invConfigs, objConfigs)
	inv := inventory.New(invTypeID, 5, inventory.StackNormal)
	// 4 items of objA (each Count=1) + 1 item of objB (Count=1).
	inv.Items[0] = &inventory.Item{Id: objA, Count: 1}
	inv.Items[1] = &inventory.Item{Id: objA, Count: 1}
	inv.Items[2] = &inventory.Item{Id: objA, Count: 1}
	inv.Items[3] = &inventory.Item{Id: objA, Count: 1}
	inv.Items[4] = &inventory.Item{Id: objB, Count: 1}
	if p.invs == nil {
		p.invs = make(map[int]*inventory.Inventory)
	}
	p.invs[invTypeID] = inv

	p.calculateRunWeight()

	const want = 4*32 + 100
	if p.runweight != want {
		t.Errorf("runweight: got %d, want %d", p.runweight, want)
	}
}

// TestCalculateRunWeight_MultipleInvs — two RunWeight invs with different
// weighted contents; runweight must sum across both.
func TestCalculateRunWeight_MultipleInvs(t *testing.T) {
	const invTypeA = 1 // RunWeight
	const invTypeB = 2 // RunWeight
	const objTypeID = 5
	invConfigs := make([]*objtype.InvType, 10)
	invConfigs[invTypeA] = &objtype.InvType{
		ID:        invTypeA,
		Scope:     objtype.InvTypeScopeTemp,
		Size:      3,
		RunWeight: true,
	}
	invConfigs[invTypeB] = &objtype.InvType{
		ID:        invTypeB,
		Scope:     objtype.InvTypeScopePerm,
		Size:      3,
		RunWeight: true,
	}
	objConfigs := make([]*objtype.ObjType, 10)
	objConfigs[objTypeID] = &objtype.ObjType{Stackable: false, Weight: 2275}

	p, _ := newTestPlayer(t)
	p.client.server = buildRunWeightServer(invConfigs, objConfigs)
	if p.invs == nil {
		p.invs = make(map[int]*inventory.Inventory)
	}
	invA := inventory.New(invTypeA, 3, inventory.StackNormal)
	invA.Items[0] = &inventory.Item{Id: objTypeID, Count: 1} // 2275g
	p.invs[invTypeA] = invA

	invB := inventory.New(invTypeB, 3, inventory.StackNormal)
	invB.Items[0] = &inventory.Item{Id: objTypeID, Count: 1} // 2275g
	invB.Items[1] = &inventory.Item{Id: objTypeID, Count: 1} // 2275g
	p.invs[invTypeB] = invB

	p.calculateRunWeight()

	const want = 3 * 2275
	if p.runweight != want {
		t.Errorf("runweight: got %d, want %d", p.runweight, want)
	}
}

// TestCalculateRunWeight_OutOfBoundsTypeIDsSkipped — inv.Type outside
// len(invConfigs) and item.Id outside len(objConfigs); no panic, runweight=0.
func TestCalculateRunWeight_OutOfBoundsTypeIDsSkipped(t *testing.T) {
	// invConfigs of length 3; inv.Type=10 is out of bounds.
	invConfigs := make([]*objtype.InvType, 3)
	// objConfigs of length 3; item.Id=10 is out of bounds.
	objConfigs := make([]*objtype.ObjType, 3)

	p, _ := newTestPlayer(t)
	p.client.server = buildRunWeightServer(invConfigs, objConfigs)
	if p.invs == nil {
		p.invs = make(map[int]*inventory.Inventory)
	}
	// Inv with out-of-bounds Type.
	outOfBoundsInv := inventory.New(10, 2, inventory.StackNormal)
	outOfBoundsInv.Items[0] = &inventory.Item{Id: 1, Count: 1}
	p.invs[10] = outOfBoundsInv

	// Inv with valid Type but item with out-of-bounds Id.
	// We need invConfigs[1] to be a RunWeight inv.
	invConfigs[1] = &objtype.InvType{
		ID:        1,
		Size:      2,
		RunWeight: true,
	}
	validInv := inventory.New(1, 2, inventory.StackNormal)
	validInv.Items[0] = &inventory.Item{Id: 10, Count: 1} // objId 10 out of bounds
	p.invs[1] = validInv

	p.calculateRunWeight()

	if p.runweight != 0 {
		t.Errorf("runweight: got %d, want 0 (out-of-bounds IDs must be skipped)", p.runweight)
	}
}

// TestCalculateRunWeight_NilServerNoOp — p.client == nil; no panic, runweight=0.
func TestCalculateRunWeight_NilServerNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.client = nil // detach client entirely

	// Must not panic.
	p.calculateRunWeight()

	if p.runweight != 0 {
		t.Errorf("runweight: got %d, want 0", p.runweight)
	}
}
