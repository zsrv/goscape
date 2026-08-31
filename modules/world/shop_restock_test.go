package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
)

// TestProcessCleanup_ShopRestock pins H3: processCleanup restocks a shop inv
// slot below its stockcount and decays one above it, gated by stockrate.
// Mirrors TS World.ts:1159-1190.
func TestProcessCleanup_ShopRestock(t *testing.T) {
	s := newServerForScriptTest(t)
	const shopType = 5
	const objID = 100

	it := objtype.NewInvType(shopType)
	it.Size = 28
	it.StackAll = true
	it.Restock = true
	it.StockObj = []uint16{objID}
	it.StockCount = []uint16{10}
	it.StockRate = []int32{1} // every tick

	cfgs := make([]*objtype.InvType, shopType+1)
	cfgs[shopType] = it
	s.invTypes = &objtype.InvTypeConfigs{Configs: cfgs}

	// Build the shop inv via FromType (as server_invs.go does) so it carries
	// its own stock list; stock-obj retention is derived from that, not a
	// per-call opt.
	inv := inventory.FromType(it)
	inv.Items[0] = &inventory.Item{Id: objID, Count: 5} // below stockcount 10
	s.invs = map[int]*inventory.Inventory{shopType: inv}
	s.currentTick = 2 // multiple of stockrate 1

	s.processCleanup()
	if inv.Items[0] == nil || inv.Items[0].Count != 6 {
		t.Fatalf("restock below-min: got %+v, want count 6", inv.Items[0])
	}

	// Above-min decays one.
	inv.Items[0].Count = 12
	s.processCleanup()
	if inv.Items[0] == nil || inv.Items[0].Count != 11 {
		t.Errorf("decay above-min: got %+v, want count 11", inv.Items[0])
	}

	// At stockcount exactly → no change.
	inv.Items[0].Count = 10
	s.processCleanup()
	if inv.Items[0].Count != 10 {
		t.Errorf("at-min: got %d, want 10 (no change)", inv.Items[0].Count)
	}
}

// TestProcessCleanup_ShopRestock_StockObjRetainsSlot pins that decaying a
// stock-obj slot to 0 keeps the slot (count 0) so it can later restock — the
// M11 retention behaviour exercised through the restock pass.
func TestProcessCleanup_ShopRestock_StockObjRetainsSlot(t *testing.T) {
	s := newServerForScriptTest(t)
	const shopType = 5
	const objID = 100

	it := objtype.NewInvType(shopType)
	it.Size = 28
	it.StackAll = true
	it.Restock = true
	it.AllStock = true
	it.StockObj = []uint16{objID}
	it.StockCount = []uint16{0} // unlisted → general-store decay path
	it.StockRate = []int32{1}   // non-empty so the top-level guard passes

	cfgs := make([]*objtype.InvType, shopType+1)
	cfgs[shopType] = it
	s.invTypes = &objtype.InvTypeConfigs{Configs: cfgs}

	// Build the shop inv via FromType so it carries its own stock list.
	inv := inventory.FromType(it)
	inv.Items[0] = &inventory.Item{Id: objID, Count: 1}
	s.invs = map[int]*inventory.Inventory{shopType: inv}
	s.currentTick = invStockRate // hits the allstock decay

	s.processCleanup()
	if inv.Items[0] == nil {
		t.Fatal("stock-obj slot vacated after decay to 0; want retained")
	}
	if inv.Items[0].Count != 0 {
		t.Errorf("retained slot count: got %d, want 0", inv.Items[0].Count)
	}
}

// TestProcessCleanup_ShopRestock_NonStackallStackableObj pins that restocking
// a stackable obj in a NON-stackall shop increments the existing stack slot
// rather than spilling into a second slot. TS reads ObjType.stackable inside
// add() (World.ts:1173 -> Inventory.ts:159); the Go restock caller must pass
// Stackable so a non-stackall restock shop matches. (Latent for shipped
// Content — every restock shop is stackall — but a real TS divergence.)
func TestProcessCleanup_ShopRestock_NonStackallStackableObj(t *testing.T) {
	s := newServerForScriptTest(t)
	const shopType = 5
	const objID = 100

	it := objtype.NewInvType(shopType)
	it.Size = 28
	it.StackAll = false // NON-stackall: stacking must come from ObjType.stackable
	it.Restock = true
	it.StockObj = []uint16{objID}
	it.StockCount = []uint16{10}
	it.StockRate = []int32{1}
	cfgs := make([]*objtype.InvType, shopType+1)
	cfgs[shopType] = it
	s.invTypes = &objtype.InvTypeConfigs{Configs: cfgs}

	// obj 100 is a stackable obj (e.g. an arrow/rune).
	if s.objTypes == nil {
		s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, objID+1)}
	}
	s.objTypes.Configs[objID] = &objtype.ObjType{
		ID: objID, DebugName: "stackable_stock",
		Stackable: true,
	}

	inv := inventory.FromType(it)
	inv.Items[0] = &inventory.Item{Id: objID, Count: 5} // below stockcount 10
	s.invs = map[int]*inventory.Inventory{shopType: inv}
	s.currentTick = 2

	s.processCleanup()

	if inv.Items[0] == nil || inv.Items[0].Count != 6 {
		t.Fatalf("restock should increment the stack slot: got %+v, want count 6", inv.Items[0])
	}
	if inv.Items[1] != nil {
		t.Errorf("restock must not spill into a second slot; got duplicate %+v", inv.Items[1])
	}
}
