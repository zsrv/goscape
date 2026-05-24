package inventory

import (
	"slices"

	"github.com/zsrv/goscape/pkg/objtype"
)

// Stacking strategy constants.
const (
	StackNormal = 0 // stack based on ObjType.Stackable
	StackAlways = 1 // always stack
	StackNever  = 2 // never stack (each unit its own slot)
)

// StackLimit is the maximum count for a single stacked item.
const StackLimit = 0x7fffffff

type Item struct {
	Id    int
	Count int
}

type Inventory struct {
	Type      int // InvType.ID
	Capacity  int
	StackType int
	Items     []*Item // length == Capacity; nil = empty
	Update    bool    // consumed by world.updateInvs()

	// stockObjIDs is the InvType's stock-obj list (InvType.stockobj),
	// populated by FromType. Add/Remove consult it to decide whether a
	// slot reaching count 0 is retained as a restockable placeholder —
	// mirroring TS, which computes `InvType.get(this.type).stockobj?.includes(id)`
	// inside add()/remove() (Inventory.ts:160,245). Keeping the membership
	// on the inventory (rather than per-call opts) means every caller —
	// INV_DEL/shop-buy, INV_MOVEITEM, INV_DROPITEM, restock — gets the
	// correct behavior without re-deriving it.
	stockObjIDs []uint16
}

// New returns an empty inventory of the given capacity.
func New(typeId, capacity, stackType int) *Inventory {
	return &Inventory{
		Type:      typeId,
		Capacity:  capacity,
		StackType: stackType,
		Items:     make([]*Item, capacity),
	}
}

// FromType builds an inventory matching an InvType's size/stack semantics and
// populates its StockObj items.
func FromType(t *objtype.InvType) *Inventory {
	inv := New(t.ID, t.Size, stackTypeFrom(t))
	inv.stockObjIDs = t.StockObj
	// L11: TS Inventory.fromType (Inventory.ts:66-73) seeds every stock index
	// with the literal {stockobj[i], stockcount[i]} — no id==0 skip and no
	// count fallback. This is load-bearing now that shop restock is live
	// (tick.go processCleanup): a count-0 stock slot must seed at 0 and then
	// restock up toward stockcount rather than start at 1, and obj id 0 is a
	// valid obj. StockObj/StockCount are allocated in lockstep
	// (invtype.go:42-43), so the indices line up.
	for i := range t.StockObj {
		if i >= len(t.StockCount) {
			break
		}
		inv.Items[i] = &Item{Id: int(t.StockObj[i]), Count: int(t.StockCount[i])}
	}
	return inv
}

func stackTypeFrom(t *objtype.InvType) int {
	if t.StackAll {
		return StackAlways
	}
	return StackNormal
}

func (inv *Inventory) Get(slot int) *Item {
	if slot < 0 || slot >= inv.Capacity {
		return nil
	}
	return inv.Items[slot]
}

func (inv *Inventory) Contains(id int) bool {
	return inv.GetItemIndex(id) >= 0
}

// isStockObj reports whether id is in this inventory's InvType stock list
// (TS `InvType.get(this.type).stockobj?.includes(id)`). Populated by
// FromType; a bare New inventory has no stock list and always returns false.
func (inv *Inventory) isStockObj(id int) bool {
	return slices.Contains(inv.stockObjIDs, uint16(id))
}

func (inv *Inventory) HasAt(slot, id int) bool {
	it := inv.Get(slot)
	return it != nil && it.Id == id
}

func (inv *Inventory) GetItemCount(id int) int {
	total := 0
	for _, it := range inv.Items {
		if it != nil && it.Id == id {
			total += it.Count
		}
	}
	return total
}

func (inv *Inventory) GetItemIndex(id int) int {
	for i, it := range inv.Items {
		if it != nil && it.Id == id {
			return i
		}
	}
	return -1
}

func (inv *Inventory) NextFreeSlot() int {
	for i, it := range inv.Items {
		if it == nil {
			return i
		}
	}
	return -1
}

func (inv *Inventory) FreeSlotCount() int {
	n := 0
	for _, it := range inv.Items {
		if it == nil {
			n++
		}
	}
	return n
}

func (inv *Inventory) IsFull() bool  { return inv.FreeSlotCount() == 0 }
func (inv *Inventory) IsEmpty() bool { return inv.FreeSlotCount() == inv.Capacity }

type AddOpts struct {
	// BeginSlot is the slot to start inserting at. L12: TS's `beginSlot`
	// defaults to -1, the sentinel for "append from the first free slot"
	// (NextFreeSlot on the stack path). The Go zero value is 0, which is a
	// REAL slot index, not the sentinel — every caller that wants default
	// append behavior MUST set BeginSlot:-1 explicitly. A caller that leaves
	// it zero silently scans from slot 0 instead of appending.
	BeginSlot           int
	AssureFullInsertion bool
	ForceNoStack        bool
	DryRun              bool

	// Stackable signals whether the obj being added is stackable
	// (`ObjType.stackable` in TS). Caller pre-computes from
	// objtype.Configs.ObjType(id).Stackable. Drives the new TS-fidelity
	// stack predicate per Inventory.ts:161. Default zero-value (false)
	// means non-stackable.
	//
	// (Stock-obj membership is NOT a caller opt: Add derives it from the
	// inventory's own InvType via isStockObj, matching TS Inventory.ts:160.)
	Stackable bool
}

type RemoveOpts struct {
	// BeginSlot is the slot to start removing at. L10/L12: TS defaults to -1
	// ("from slot 0, no wrap"). A BeginSlot >= 1 scans [BeginSlot, capacity)
	// then wraps to the skipped prefix [0, BeginSlot). The Go zero value (0)
	// behaves identically to -1 here (start at 0, no wrap), but callers should
	// still pass -1 for clarity and parity with AddOpts.
	BeginSlot         int
	AssureFullRemoval bool

	// (Stock-obj membership is NOT a caller opt: Remove derives it from the
	// inventory's own InvType via isStockObj, matching TS Inventory.ts:245.
	// A slot reaching count 0 is retained as a restockable placeholder when
	// the obj is in the inv's stock list — Inventory.ts:280-286.)
}

func (inv *Inventory) Set(slot int, item *Item) {
	if slot < 0 || slot >= inv.Capacity {
		return
	}
	inv.Items[slot] = item
	inv.Update = true
}

func (inv *Inventory) Delete(slot int) { inv.Set(slot, nil) }

// Clear removes every item slot and dirty-flags for wire sync.
func (inv *Inventory) Clear() {
	for j := range inv.Items {
		inv.Items[j] = nil
	}
	inv.Update = true
}

func (inv *Inventory) Swap(from, to int) {
	if from < 0 || from >= inv.Capacity || to < 0 || to >= inv.Capacity {
		return
	}
	inv.Items[from], inv.Items[to] = inv.Items[to], inv.Items[from]
	inv.Update = true
}

// Add inserts up to count units of obj id into the inv. Mirrors TS
// Inventory.add (Engine-TS/src/engine/Inventory.ts:158-225) 1:1.
//
// Stack predicate (TS line 161):
//
//	stack = !ForceNoStack && stackType != StackNever
//	     && (Stackable || stackType == StackAlways)
//
// Returns Transaction with Completed (units written) and Added (per-slot
// SlotEntries actually written; empty for no-op or DryRun).
func (inv *Inventory) Add(id, count int, opts AddOpts) Transaction {
	tx := Transaction{Requested: count}
	if count <= 0 {
		return tx
	}

	// TS line 160: stockObj is computed from the inventory's own InvType,
	// not a caller-supplied flag.
	stockObj := inv.isStockObj(id)

	// TS line 161: stack predicate.
	stack := !opts.ForceNoStack &&
		inv.StackType != StackNever &&
		(opts.Stackable || inv.StackType == StackAlways)

	// TS lines 163-166: previousCount is non-zero only on the stack path.
	var previousCount int
	if stack {
		previousCount = inv.GetItemCount(id)
	}

	// TS lines 168-170: stack already at limit — short-circuit.
	if previousCount == StackLimit {
		return tx
	}

	free := inv.FreeSlotCount()
	// TS lines 172-175: free=0 guard with stockObj exception.
	if free == 0 && (!stack || (stack && previousCount == 0 && !stockObj)) {
		return tx
	}

	// TS lines 177-191: AssureFullInsertion gate.
	if opts.AssureFullInsertion {
		if stack && previousCount > StackLimit-count {
			return tx
		}
		if !stack && count > free {
			return tx
		}
	} else {
		if stack && previousCount == StackLimit {
			return tx
		}
		if !stack && free == 0 {
			return tx
		}
	}

	// TS lines 196-213: non-stack branch.
	if !stack {
		startSlot := max(opts.BeginSlot, 0)
		completed := 0
		for i := startSlot; i < inv.Capacity && completed < count; i++ {
			if inv.Items[i] != nil {
				continue
			}
			it := Item{Id: id, Count: 1}
			if !opts.DryRun {
				inv.Items[i] = &Item{Id: id, Count: 1}
			}
			tx.Added = append(tx.Added, SlotEntry{Slot: i, Item: it})
			completed++
		}
		if !opts.DryRun && completed > 0 {
			inv.Update = true
		}
		tx.Completed = completed
		return tx
	}

	// TS lines 214-225: stack branch — find or allocate the stack slot.
	// When no existing stack and BeginSlot != -1, scan for the first nil
	// slot at-or-after BeginSlot (TS `items.indexOf(null, beginSlot)`).
	stackIndex := inv.GetItemIndex(id)
	if stackIndex == -1 {
		if opts.BeginSlot == -1 {
			stackIndex = inv.NextFreeSlot()
		} else {
			stackIndex = -1
			for i := opts.BeginSlot; i < inv.Capacity; i++ {
				if inv.Items[i] == nil {
					stackIndex = i
					break
				}
			}
		}
		if stackIndex < 0 || stackIndex >= inv.Capacity {
			return tx
		}
	}

	// L13: TS lines 229-237 clamp using the PER-SLOT stack count at stackIndex
	// (`this.get(stackIndex)?.count`), not GetItemCount which sums across all
	// slots, and SET the slot to the new total rather than incrementing. These
	// differ only when a stack-typed inv holds duplicate stacks of one id (an
	// invariant violation); TS clamps per-slot, so we match it. previousCount
	// (the sum) remains the basis for the earlier entry gates, matching TS.
	stackCount := 0
	if inv.Items[stackIndex] != nil {
		stackCount = inv.Items[stackIndex].Count
	}
	total := min(StackLimit, stackCount+count)
	written := Item{Id: id, Count: total}
	if !opts.DryRun {
		inv.Items[stackIndex] = &Item{Id: id, Count: total}
		inv.Update = true
	}
	tx.Completed = total - stackCount
	tx.Added = []SlotEntry{{Slot: stackIndex, Item: written}}
	return tx
}

func (inv *Inventory) Remove(id, count int, opts RemoveOpts) Transaction {
	tx := Transaction{Requested: count}
	if count <= 0 {
		return tx
	}
	// M10: assureFullRemoval is all-or-nothing — if the inv doesn't hold the
	// full requested count, remove nothing. Mirrors TS Inventory.ts:247-248.
	if opts.AssureFullRemoval && inv.GetItemCount(id) < count {
		return tx
	}
	// TS line 245: stockObj is computed from the inventory's own InvType.
	stockObj := inv.isStockObj(id)
	removed := 0
	begin := max(opts.BeginSlot, 0)
	removeFrom := func(lo, hi int) {
		for i := lo; i < hi && removed < count; i++ {
			it := inv.Items[i]
			if it == nil || it.Id != id {
				continue
			}
			take := min(count-removed, it.Count)
			it.Count -= take
			removed += take
			// M11: a stock-obj slot is retained at count 0 so a shop can restock
			// it; everything else vacates the slot. Mirrors TS Inventory.ts:280-286.
			if it.Count == 0 && !stockObj {
				inv.Items[i] = nil
			}
		}
	}
	removeFrom(begin, inv.Capacity)
	// L10: with a beginSlot (>= 1), TS scans [beginSlot, capacity) first, then
	// wraps to the skipped prefix [0, beginSlot) if not yet satisfied
	// (Inventory.ts:256-316). BeginSlot == -1 (or 0) starts at slot 0 with no
	// wrap. Live restock callers pass BeginSlot=index where the id is at that
	// slot, so the wrap isn't exercised today, but a future caller could pass a
	// beginSlot past the id — match TS so it doesn't silently under-remove.
	if opts.BeginSlot > 0 && removed < count {
		removeFrom(0, begin)
	}
	if removed > 0 {
		inv.Update = true
	}
	tx.Completed = removed
	return tx
}
