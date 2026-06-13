package inventory

import (
	"slices"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestNewHasCapacity(t *testing.T) {
	inv := New(1, 28, StackNormal)
	if inv.Capacity != 28 {
		t.Errorf("Capacity: got %d, want 28", inv.Capacity)
	}
	if len(inv.Items) != 28 {
		t.Errorf("Items len: got %d, want 28", len(inv.Items))
	}
}

// --- Add: 274 contract — bare count return, partial fill, dirty tracking ---
//
// Add's signature is Add(id, count, beginSlot, stackable). The stackable bool
// is the pre-resolved ObjType.stackable predicate (TS reads it inside add() via
// ObjType.get; goscape pre-resolves it at the call site to keep the container
// decoupled from the config registry).

func TestAddIntoEmptyInventory(t *testing.T) {
	inv := New(1, 28, StackNormal)
	completed := inv.Add(10, 1, -1, false)
	if completed != 1 {
		t.Errorf("completed: got %d, want 1", completed)
	}
	if inv.Items[0] == nil || inv.Items[0].Id != 10 || inv.Items[0].Count != 1 {
		t.Errorf("slot 0 after add: %+v", inv.Items[0])
	}
	if !inv.Update {
		t.Error("Update flag should be true after add")
	}
	if got := inv.GetDirtySlots(); !slices.Equal(got, []int{0}) {
		t.Errorf("dirty slots: got %v, want [0]", got)
	}
}

func TestAddStackingBehavior(t *testing.T) {
	inv := New(1, 28, StackAlways)
	inv.Add(10, 3, -1, false)
	inv.Add(10, 5, -1, false)
	if inv.Items[0].Count != 8 {
		t.Errorf("stacked count: got %d, want 8", inv.Items[0].Count)
	}
	if inv.Items[1] != nil {
		t.Error("slot 1 should be empty")
	}
}

// Non-stackable obj in StackNever inv distributes one-per-slot.
func TestAddNoStackFillsSlots(t *testing.T) {
	inv := New(1, 28, StackNever)
	inv.Add(10, 3, -1, false)
	for i := range 3 {
		if inv.Items[i] == nil || inv.Items[i].Count != 1 {
			t.Errorf("slot %d: %+v", i, inv.Items[i])
		}
	}
	if got := inv.GetDirtySlots(); !slices.Equal(got, []int{0, 1, 2}) {
		t.Errorf("dirty slots: got %v, want [0 1 2]", got)
	}
}

// 274: a non-stack add that runs out of room partial-fills and returns the
// completed count (no all-or-nothing rollback — assureFullInsertion is gone).
func TestAddPartialNonStackReturnsCompleted(t *testing.T) {
	inv := New(1, 3, StackNever)
	completed := inv.Add(10, 5, -1, false)
	if completed != 3 {
		t.Errorf("completed (partial): got %d, want 3", completed)
	}
	for i := range 3 {
		if inv.Items[i] == nil || inv.Items[i].Count != 1 {
			t.Errorf("slot %d: got %+v, want {Id:10 Count:1}", i, inv.Items[i])
		}
	}
}

// 274: stackable obj into a normal-stack inv stacks in one slot.
func TestAddStackableObjNormalStackInvStacksOneSlot(t *testing.T) {
	inv := New(1, 28, StackNormal)
	completed := inv.Add(10, 25, -1, true) // stackable=true
	if completed != 25 {
		t.Errorf("completed: got %d, want 25", completed)
	}
	if inv.Items[0] == nil || inv.Items[0].Count != 25 {
		t.Errorf("slot 0: got %+v, want {Id:10 Count:25}", inv.Items[0])
	}
	if inv.Items[1] != nil {
		t.Errorf("slot 1 should remain empty, got %+v", inv.Items[1])
	}
}

// 274: a non-stackable obj into a normal-stack inv distributes one-per-slot.
func TestAddNonStackableObjNormalStackInvFillsSlots(t *testing.T) {
	inv := New(1, 28, StackNormal)
	completed := inv.Add(10, 3, -1, false)
	if completed != 3 {
		t.Errorf("completed: got %d, want 3", completed)
	}
	for i := range 3 {
		if inv.Items[i] == nil || inv.Items[i].Count != 1 {
			t.Errorf("slot %d: got %+v, want {Id:10 Count:1}", i, inv.Items[i])
		}
	}
}

// 274: even a non-stackable obj stacks when the inv is StackAlways (bank).
func TestAddAlwaysStackInvIgnoresStackable(t *testing.T) {
	inv := New(1, 28, StackAlways)
	completed := inv.Add(10, 10, -1, false) // stackable=false but StackAlways
	if completed != 10 {
		t.Errorf("completed: got %d, want 10", completed)
	}
	if inv.Items[0] == nil || inv.Items[0].Count != 10 {
		t.Errorf("slot 0: got %+v, want {Id:10 Count:10}", inv.Items[0])
	}
}

// 274: StackLimit clamp on stack add; completed reports the clamped amount.
func TestAddStackLimitClamp(t *testing.T) {
	inv := New(1, 28, StackAlways)
	inv.Items[0] = &Item{Id: 10, Count: StackLimit - 3}
	completed := inv.Add(10, 10, -1, true)
	if completed != 3 {
		t.Errorf("completed (clamped): got %d, want 3", completed)
	}
	if inv.Items[0].Count != StackLimit {
		t.Errorf("clamped slot: got %d, want %d", inv.Items[0].Count, StackLimit)
	}
}

// 274: stack add with no existing stack and no free slot returns 0.
func TestAddStackNoFreeSlotReturnsZero(t *testing.T) {
	inv := New(1, 2, StackAlways)
	inv.Items[0] = &Item{Id: 99, Count: 1}
	inv.Items[1] = &Item{Id: 88, Count: 1}
	completed := inv.Add(10, 5, -1, true)
	if completed != 0 {
		t.Errorf("completed (no slot): got %d, want 0", completed)
	}
	if inv.Items[0].Id != 99 || inv.Items[1].Id != 88 {
		t.Errorf("slots should be unchanged; got %+v %+v", inv.Items[0], inv.Items[1])
	}
}

// 274: stack add finds an existing depleted (count-0) stack slot even when the
// inv is otherwise full (stock-obj depleted placeholder). getItemIndex finds
// the count-0 slot regardless of free space.
func TestAddStackFindsDepletedExistingStack(t *testing.T) {
	inv := New(1, 2, StackAlways)
	inv.Items[0] = &Item{Id: 10, Count: 0}
	inv.Items[1] = &Item{Id: 99, Count: 1}
	completed := inv.Add(10, 5, -1, true)
	if completed != 5 {
		t.Errorf("completed: got %d, want 5", completed)
	}
	if inv.Items[0].Count != 5 {
		t.Errorf("depleted stack slot: got %d, want 5", inv.Items[0].Count)
	}
}

// 274: stack add with beginSlot != -1 scans for the first nil slot at-or-after
// beginSlot when no existing stack of the id is present.
func TestAddStackBeginSlot(t *testing.T) {
	inv := New(1, 5, StackAlways)
	inv.Items[0] = &Item{Id: 99, Count: 1}
	completed := inv.Add(10, 4, 1, true)
	if completed != 4 {
		t.Errorf("completed: got %d, want 4", completed)
	}
	if inv.Items[1] == nil || inv.Items[1].Id != 10 || inv.Items[1].Count != 4 {
		t.Errorf("slot 1: got %+v, want {Id:10 Count:4}", inv.Items[1])
	}
}

// 274: non-stack add with beginSlot starts at max(0, beginSlot).
func TestAddNonStackBeginSlot(t *testing.T) {
	inv := New(1, 5, StackNever)
	completed := inv.Add(10, 2, 2, false)
	if completed != 2 {
		t.Errorf("completed: got %d, want 2", completed)
	}
	if inv.Items[0] != nil || inv.Items[1] != nil {
		t.Error("slots before beginSlot must stay empty")
	}
	if inv.Items[2] == nil || inv.Items[3] == nil {
		t.Error("slots 2,3 should be filled")
	}
}

// --- Remove: 274 contract — bare count return, partial removal, dirty ---

func TestRemoveDecrementsCount(t *testing.T) {
	inv := New(1, 28, StackAlways)
	inv.Add(10, 5, -1, false)
	inv.ResetTracking()
	removed := inv.Remove(10, 2, -1)
	if removed != 2 {
		t.Errorf("removed: got %d, want 2", removed)
	}
	if inv.Items[0].Count != 3 {
		t.Errorf("after remove count: got %d, want 3", inv.Items[0].Count)
	}
	if got := inv.GetDirtySlots(); !slices.Equal(got, []int{0}) {
		t.Errorf("dirty slots: got %v, want [0]", got)
	}
}

// 274: partial removal when fewer present; returns the removed count.
func TestRemovePartialWhenInsufficient(t *testing.T) {
	inv := New(1, 28, StackAlways)
	inv.Add(10, 3, -1, false)
	removed := inv.Remove(10, 5, -1)
	if removed != 3 {
		t.Errorf("removed (partial): got %d, want 3", removed)
	}
	if inv.Items[0] != nil {
		t.Error("slot should be empty after full drain")
	}
}

// 274: a stock-obj slot reaching count 0 is retained (count 0), not vacated.
// Mirrors TS Inventory.remove (Inventory.ts:181-183 — `!stockObj`).
func TestRemoveStockObjRetainsSlot(t *testing.T) {
	inv := New(1, 28, StackAlways)
	inv.stockObjIDs = []uint16{10}
	inv.Add(10, 2, -1, false)
	removed := inv.Remove(10, 2, -1)
	if removed != 2 {
		t.Errorf("removed: got %d, want 2", removed)
	}
	if inv.Items[0] == nil {
		t.Fatal("stock-obj slot must be retained at count 0, got nil")
	}
	if inv.Items[0].Count != 0 {
		t.Errorf("retained slot count: got %d, want 0", inv.Items[0].Count)
	}

	inv2 := New(1, 28, StackAlways)
	inv2.Add(10, 2, -1, false)
	inv2.Remove(10, 2, -1)
	if inv2.Items[0] != nil {
		t.Error("non-stock slot must vacate at count 0")
	}
}

// 274: Remove with beginSlot >= 1 scans [beginSlot, capacity) first, then wraps
// to the skipped prefix [0, beginSlot). Mirrors TS Inventory.remove
// (Inventory.ts:157-211).
func TestRemoveBeginSlotWrapsPrefix(t *testing.T) {
	inv := New(1, 5, StackNever)
	inv.Items[0] = &Item{Id: 10, Count: 1}
	inv.Items[1] = &Item{Id: 10, Count: 1}
	removed := inv.Remove(10, 2, 1)
	if removed != 2 {
		t.Errorf("removed: got %d, want 2 (first pass + prefix wrap)", removed)
	}
	if inv.Items[0] != nil || inv.Items[1] != nil {
		t.Errorf("both slots should clear: [0]=%+v [1]=%+v", inv.Items[0], inv.Items[1])
	}
}

// FromType stock-buy bug: buying out a permanently-stocked item leaves a count-0
// placeholder, not an empty slot. The retention comes from the inventory's own
// InvType (TS computes it inside remove()).
func TestFromTypeStockObjRemoveRetainsPlaceholder(t *testing.T) {
	it := &objtype.InvType{Size: 40, StockObj: []uint16{100}, StockCount: []uint16{10}}
	inv := FromType(it)
	removed := inv.Remove(100, 10, -1)
	if removed != 10 {
		t.Fatalf("removed: got %d, want 10", removed)
	}
	if inv.Items[0] == nil {
		t.Fatal("permanently-stocked slot must remain as a count-0 placeholder, got nil")
	}
	if inv.Items[0].Id != 100 || inv.Items[0].Count != 0 {
		t.Errorf("placeholder: got %+v, want {Id:100 Count:0}", inv.Items[0])
	}
	inv.Items[1] = &Item{Id: 200, Count: 3}
	inv.Remove(200, 3, -1)
	if inv.Items[1] != nil {
		t.Error("non-stock obj must vacate at count 0 even in a stock inventory")
	}
}

// --- Set / Delete / RemoveAll dirty tracking ---

func TestSetMarksDirty(t *testing.T) {
	inv := New(1, 5, StackNormal)
	inv.Set(3, &Item{Id: 7, Count: 2})
	if inv.Items[3] == nil || inv.Items[3].Id != 7 {
		t.Errorf("slot 3: got %+v", inv.Items[3])
	}
	if !inv.Update {
		t.Error("Set must mark Update")
	}
	if got := inv.GetDirtySlots(); !slices.Equal(got, []int{3}) {
		t.Errorf("dirty slots: got %v, want [3]", got)
	}
}

func TestDeleteMarksDirty(t *testing.T) {
	inv := New(1, 5, StackNormal)
	inv.Items[2] = &Item{Id: 7, Count: 2}
	inv.Delete(2)
	if inv.Items[2] != nil {
		t.Error("Delete must clear the slot")
	}
	if got := inv.GetDirtySlots(); !slices.Equal(got, []int{2}) {
		t.Errorf("dirty slots: got %v, want [2]", got)
	}
}

// 274: RemoveAll marks each PREVIOUSLY-OCCUPIED slot dirty (loop, not fill).
// Mirrors TS Inventory.removeAll (Inventory.ts:98-105).
func TestRemoveAllMarksOccupiedSlotsDirty(t *testing.T) {
	inv := New(1, 5, StackNormal)
	inv.Items[1] = &Item{Id: 7, Count: 1}
	inv.Items[3] = &Item{Id: 8, Count: 1}
	inv.ResetTracking()

	inv.RemoveAll()
	for i := range inv.Items {
		if inv.Items[i] != nil {
			t.Errorf("slot %d not cleared: %+v", i, inv.Items[i])
		}
	}
	if got := inv.GetDirtySlots(); !slices.Equal(got, []int{1, 3}) {
		t.Errorf("dirty slots: got %v, want [1 3]", got)
	}
}

// --- Dirty-slot tracking + ResetTracking ---

func TestGetDirtySlotsSorted(t *testing.T) {
	inv := New(1, 10, StackNormal)
	inv.Set(7, &Item{Id: 1, Count: 1})
	inv.Set(2, &Item{Id: 1, Count: 1})
	inv.Set(5, &Item{Id: 1, Count: 1})
	if got := inv.GetDirtySlots(); !slices.Equal(got, []int{2, 5, 7}) {
		t.Errorf("dirty slots: got %v, want [2 5 7] (sorted)", got)
	}
}

func TestResetTrackingClearsFlagAndSet(t *testing.T) {
	inv := New(1, 10, StackNormal)
	inv.Set(4, &Item{Id: 1, Count: 1})
	if !inv.Update || len(inv.GetDirtySlots()) == 0 {
		t.Fatal("precondition: Set should dirty the inv")
	}
	inv.ResetTracking()
	if inv.Update {
		t.Error("ResetTracking must clear Update")
	}
	if got := inv.GetDirtySlots(); len(got) != 0 {
		t.Errorf("ResetTracking must clear dirty slots; got %v", got)
	}
}

// --- FromType seeding (L11) ---

func TestFromTypeSeedsLiteralStockObjAndCount(t *testing.T) {
	tp := &objtype.InvType{
		ConfigType: objtype.ConfigType{ID: 1},
		Size:       3,
		StockObj:   []uint16{0, 7, 0},
		StockCount: []uint16{5, 0, 0},
	}
	inv := FromType(tp)
	if inv.Items[0] == nil || inv.Items[0].Id != 0 || inv.Items[0].Count != 5 {
		t.Errorf("slot 0: got %+v, want {Id:0 Count:5}", inv.Items[0])
	}
	if inv.Items[1] == nil || inv.Items[1].Id != 7 || inv.Items[1].Count != 0 {
		t.Errorf("slot 1: got %+v, want {Id:7 Count:0}", inv.Items[1])
	}
	if inv.Items[2] == nil || inv.Items[2].Id != 0 || inv.Items[2].Count != 0 {
		t.Errorf("slot 2: got %+v, want {Id:0 Count:0}", inv.Items[2])
	}
}

// ValidSlot bounds.
func TestValidSlot(t *testing.T) {
	inv := New(1, 3, StackNormal)
	for _, tc := range []struct {
		slot int
		want bool
	}{{-1, false}, {0, true}, {2, true}, {3, false}} {
		if got := inv.ValidSlot(tc.slot); got != tc.want {
			t.Errorf("ValidSlot(%d): got %v, want %v", tc.slot, got, tc.want)
		}
	}
}
