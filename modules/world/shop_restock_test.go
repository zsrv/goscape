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
	it.Restock = true
	it.StockObj = []uint16{objID}
	it.StockCount = []uint16{10}
	it.StockRate = []int32{1} // every tick

	cfgs := make([]*objtype.InvType, shopType+1)
	cfgs[shopType] = it
	s.invTypes = &objtype.InvTypeConfigs{Configs: cfgs}

	inv := inventory.New(shopType, 28, inventory.StackAlways)
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
	it.Restock = true
	it.AllStock = true
	it.StockObj = []uint16{objID}
	it.StockCount = []uint16{0}    // unlisted → general-store decay path
	it.StockRate = []int32{1}      // non-empty so the top-level guard passes

	cfgs := make([]*objtype.InvType, shopType+1)
	cfgs[shopType] = it
	s.invTypes = &objtype.InvTypeConfigs{Configs: cfgs}

	inv := inventory.New(shopType, 28, inventory.StackAlways)
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
