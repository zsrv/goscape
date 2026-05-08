package inventory

import "testing"

func TestNewHasCapacity(t *testing.T) {
	inv := New(1, 28, StackNormal)
	if inv.Capacity != 28 {
		t.Errorf("Capacity: got %d, want 28", inv.Capacity)
	}
	if len(inv.Items) != 28 {
		t.Errorf("Items len: got %d, want 28", len(inv.Items))
	}
}

func TestAddIntoEmptyInventory(t *testing.T) {
	inv := New(1, 28, StackNormal)
	tx := inv.Add(10, 1, AddOpts{})
	if tx.Completed != 1 {
		t.Errorf("Completed: got %d, want 1", tx.Completed)
	}
	if inv.Items[0] == nil || inv.Items[0].Id != 10 || inv.Items[0].Count != 1 {
		t.Errorf("slot 0 after add: %+v", inv.Items[0])
	}
	if !inv.Update {
		t.Error("Update flag should be true after add")
	}
}

func TestAddStackingBehavior(t *testing.T) {
	inv := New(1, 28, StackAlways)
	inv.Add(10, 3, AddOpts{})
	inv.Add(10, 5, AddOpts{})
	if inv.Items[0].Count != 8 {
		t.Errorf("stacked count: got %d, want 8", inv.Items[0].Count)
	}
	if inv.Items[1] != nil {
		t.Error("slot 1 should be empty")
	}
}

func TestAddNoStackFillsSlots(t *testing.T) {
	inv := New(1, 28, StackNever)
	inv.Add(10, 3, AddOpts{})
	for i := 0; i < 3; i++ {
		if inv.Items[i] == nil || inv.Items[i].Count != 1 {
			t.Errorf("slot %d: %+v", i, inv.Items[i])
		}
	}
}

func TestRemoveDecrementsCount(t *testing.T) {
	inv := New(1, 28, StackAlways)
	inv.Add(10, 5, AddOpts{})
	tx := inv.Remove(10, 2, RemoveOpts{})
	if tx.Completed != 2 {
		t.Errorf("Completed: got %d, want 2", tx.Completed)
	}
	if inv.Items[0].Count != 3 {
		t.Errorf("after remove count: got %d, want 3", inv.Items[0].Count)
	}
}

func TestRemovePartialWhenInsufficient(t *testing.T) {
	inv := New(1, 28, StackAlways)
	inv.Add(10, 3, AddOpts{})
	tx := inv.Remove(10, 5, RemoveOpts{})
	if tx.Completed != 3 {
		t.Errorf("Completed (partial): got %d, want 3", tx.Completed)
	}
	if inv.Items[0] != nil {
		t.Error("slot should be empty after full drain")
	}
}

func TestSwapExchangesSlots(t *testing.T) {
	inv := New(1, 28, StackNormal)
	inv.Items[0] = &Item{Id: 10, Count: 1}
	inv.Items[1] = &Item{Id: 20, Count: 1}
	inv.Swap(0, 1)
	if inv.Items[0].Id != 20 || inv.Items[1].Id != 10 {
		t.Errorf("after swap: %+v %+v", inv.Items[0], inv.Items[1])
	}
}

func TestIsFullAndFreeSlotCount(t *testing.T) {
	inv := New(1, 3, StackNormal)
	if inv.IsFull() {
		t.Error("new inv should not be full")
	}
	if inv.FreeSlotCount() != 3 {
		t.Errorf("free: got %d, want 3", inv.FreeSlotCount())
	}
	inv.Add(10, 3, AddOpts{})
	if !inv.IsFull() {
		t.Error("inv should be full after 3 adds")
	}
}

// -- NAI-130 TS-fidelity tests --

// (1) Bronze-arrow analogue: stackable obj into normal-stack inv stacks
// in a single slot. Pre-NAI-130 this distributed 25 slots × count=1.
func TestAdd_StackableObj_NormalStackInv_StacksInOneSlot(t *testing.T) {
	inv := New(1, 28, StackNormal)
	tx := inv.Add(10, 25, AddOpts{BeginSlot: -1, Stackable: true})
	if tx.Completed != 25 {
		t.Errorf("Completed: got %d, want 25", tx.Completed)
	}
	if inv.Items[0] == nil || inv.Items[0].Id != 10 || inv.Items[0].Count != 25 {
		t.Errorf("slot 0 after add: got %+v, want {Id:10 Count:25}", inv.Items[0])
	}
	if inv.Items[1] != nil {
		t.Errorf("slot 1 should remain empty, got %+v", inv.Items[1])
	}
}

// (2) Regression: non-stackable obj into normal-stack inv distributes
// one-per-slot. Same shape as TestAddNoStackFillsSlots (which uses
// StackNever) but exercises the stackable=false path through StackNormal.
func TestAdd_NonStackableObj_NormalStackInv_FillsSlots(t *testing.T) {
	inv := New(1, 28, StackNormal)
	tx := inv.Add(10, 3, AddOpts{BeginSlot: -1, Stackable: false})
	if tx.Completed != 3 {
		t.Errorf("Completed: got %d, want 3", tx.Completed)
	}
	for i := 0; i < 3; i++ {
		if inv.Items[i] == nil || inv.Items[i].Count != 1 {
			t.Errorf("slot %d: got %+v, want {Id:10 Count:1}", i, inv.Items[i])
		}
	}
	if inv.Items[3] != nil {
		t.Errorf("slot 3 should remain empty, got %+v", inv.Items[3])
	}
}

// (3) ALWAYS_STACK predicate's right-hand disjunct: even a non-stackable
// obj stacks when the inv is StackAlways (e.g., bank).
func TestAdd_AlwaysStackInv_IgnoresStackableFlag(t *testing.T) {
	inv := New(1, 28, StackAlways)
	tx := inv.Add(10, 10, AddOpts{BeginSlot: -1, Stackable: false})
	if tx.Completed != 10 {
		t.Errorf("Completed: got %d, want 10", tx.Completed)
	}
	if inv.Items[0] == nil || inv.Items[0].Count != 10 {
		t.Errorf("slot 0: got %+v, want {Id:10 Count:10}", inv.Items[0])
	}
	if inv.Items[1] != nil {
		t.Errorf("slot 1 should remain empty, got %+v", inv.Items[1])
	}
}

// (4) AssureFullInsertion + stack-overflow rolls back: previousCount=10,
// StackLimit-count would overflow → tx.Completed=0, slot unchanged.
func TestAdd_AssureFullInsertion_StackOverflow_RollsBack(t *testing.T) {
	inv := New(1, 28, StackAlways)
	inv.Items[0] = &Item{Id: 10, Count: StackLimit - 5}
	tx := inv.Add(10, 10, AddOpts{
		BeginSlot:           -1,
		AssureFullInsertion: true,
		Stackable:           true,
	})
	if tx.Completed != 0 {
		t.Errorf("Completed (should roll back): got %d, want 0", tx.Completed)
	}
	if inv.Items[0].Count != StackLimit-5 {
		t.Errorf("slot 0 should be unchanged: got Count=%d, want %d", inv.Items[0].Count, StackLimit-5)
	}
}

// (5) AssureFullInsertion + non-stack overflow rolls back: free=2,
// count=3 → tx.Completed=0, no slot mutation.
func TestAdd_AssureFullInsertion_NonStackOverflow_RollsBack(t *testing.T) {
	inv := New(1, 3, StackNormal)
	// Pre-fill 1 slot so free=2.
	inv.Items[0] = &Item{Id: 99, Count: 1}
	tx := inv.Add(10, 3, AddOpts{
		BeginSlot:           -1,
		AssureFullInsertion: true,
		Stackable:           false,
	})
	if tx.Completed != 0 {
		t.Errorf("Completed (should roll back): got %d, want 0", tx.Completed)
	}
	if inv.Items[1] != nil || inv.Items[2] != nil {
		t.Errorf("non-stack rollback should leave slots 1/2 empty; got %+v %+v", inv.Items[1], inv.Items[2])
	}
}

// (6) Free=0 + stack + previousCount=0 + !stockObj → fail. This is the
// TS line 173 early-return for invs with no slots and no existing stack
// for a non-stock obj.
func TestAdd_FreeZero_NoExistingStack_NoStockObj_Fails(t *testing.T) {
	inv := New(1, 2, StackAlways)
	// Fill both slots with OTHER objs so free=0 and obj 10 has no stack.
	inv.Items[0] = &Item{Id: 99, Count: 1}
	inv.Items[1] = &Item{Id: 88, Count: 1}
	tx := inv.Add(10, 5, AddOpts{
		BeginSlot: -1,
		Stackable: true,
		StockObj:  false,
	})
	if tx.Completed != 0 {
		t.Errorf("Completed (no slot, no stock): got %d, want 0", tx.Completed)
	}
	if inv.Items[0].Id != 99 || inv.Items[1].Id != 88 {
		t.Errorf("slots should be unchanged; got %+v %+v", inv.Items[0], inv.Items[1])
	}
}

// (7) Free=0 + stack + StockObj=true + existing depleted stock slot →
// the TS line 173 stockObj guard skips the early-return; getItemIndex
// finds the depleted slot; stack branch increments it.
func TestAdd_FreeZero_StockObj_ExistingDepletedStock_Succeeds(t *testing.T) {
	inv := New(1, 2, StackAlways)
	// Slot 0 holds the depleted stock slot for obj 10 (Count=0 but
	// non-nil so freeSlotCount() == 0). Slot 1 holds another obj.
	inv.Items[0] = &Item{Id: 10, Count: 0}
	inv.Items[1] = &Item{Id: 99, Count: 1}
	tx := inv.Add(10, 5, AddOpts{
		BeginSlot: -1,
		Stackable: true,
		StockObj:  true,
	})
	if tx.Completed != 5 {
		t.Errorf("Completed (stockObj depleted): got %d, want 5", tx.Completed)
	}
	if inv.Items[0].Count != 5 {
		t.Errorf("depleted stock slot: got Count=%d, want 5", inv.Items[0].Count)
	}
}

// (8) StackLimit clamp on stack add without AssureFullInsertion: adds
// up to the limit and reports clamped tx.Completed.
func TestAdd_StackLimitClamp_NonAssure(t *testing.T) {
	inv := New(1, 28, StackAlways)
	inv.Items[0] = &Item{Id: 10, Count: StackLimit - 3}
	tx := inv.Add(10, 10, AddOpts{
		BeginSlot:           -1,
		AssureFullInsertion: false,
		Stackable:           true,
	})
	if tx.Completed != 3 {
		t.Errorf("Completed (clamped): got %d, want 3", tx.Completed)
	}
	if inv.Items[0].Count != StackLimit {
		t.Errorf("clamped slot: got Count=%d, want %d", inv.Items[0].Count, StackLimit)
	}
}

// (9) Partial non-stack add without AssureFullInsertion: fills as many
// slots as available and reports partial tx.Completed.
func TestAdd_PartialNonStack_ReturnsCompletedCount(t *testing.T) {
	inv := New(1, 3, StackNormal)
	tx := inv.Add(10, 5, AddOpts{
		BeginSlot:           -1,
		AssureFullInsertion: false,
		Stackable:           false,
	})
	if tx.Completed != 3 {
		t.Errorf("Completed (partial): got %d, want 3", tx.Completed)
	}
	for i := 0; i < 3; i++ {
		if inv.Items[i] == nil || inv.Items[i].Count != 1 {
			t.Errorf("slot %d: got %+v, want {Id:10 Count:1}", i, inv.Items[i])
		}
	}
}

// (10) Transaction.Added populated for non-stack add: lists each (slot,
// item) actually written. Mirrors TS Inventory.add `added` array.
func TestAdd_TransactionAddedPopulated(t *testing.T) {
	inv := New(1, 28, StackNormal)
	tx := inv.Add(10, 2, AddOpts{BeginSlot: -1, Stackable: false})
	if len(tx.Added) != 2 {
		t.Fatalf("Added len: got %d, want 2", len(tx.Added))
	}
	if tx.Added[0] != (SlotEntry{Slot: 0, Item: Item{Id: 10, Count: 1}}) {
		t.Errorf("Added[0]: got %+v, want {Slot:0 Item:{Id:10 Count:1}}", tx.Added[0])
	}
	if tx.Added[1] != (SlotEntry{Slot: 1, Item: Item{Id: 10, Count: 1}}) {
		t.Errorf("Added[1]: got %+v, want {Slot:1 Item:{Id:10 Count:1}}", tx.Added[1])
	}
}
