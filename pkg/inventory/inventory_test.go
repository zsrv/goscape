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
