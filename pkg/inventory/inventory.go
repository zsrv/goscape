package inventory

import "github.com/zsrv/goscape/pkg/objtype"

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
	for i, id := range t.StockObj {
		if id == 0 {
			continue
		}
		count := 1
		if i < len(t.StockCount) && t.StockCount[i] > 0 {
			count = int(t.StockCount[i])
		}
		inv.Items[i] = &Item{Id: int(id), Count: count}
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
	BeginSlot           int
	AssureFullInsertion bool
	ForceNoStack        bool
	DryRun              bool

	// Stackable signals whether the obj being added is stackable
	// (`ObjType.stackable` in TS). Caller pre-computes from
	// objtype.Configs.ObjType(id).Stackable. Drives the new TS-fidelity
	// stack predicate per Inventory.ts:161. Default zero-value (false)
	// means non-stackable.
	Stackable bool

	// StockObj signals whether the obj is in the inv's stock list
	// (`InvType.stockobj.includes(id)` in TS). Caller pre-computes from
	// InvType.StockObj. Drives the TS line 173 stockObj-aware free-slot
	// guard. Default zero-value (false) means not a stock obj.
	StockObj bool
}

type RemoveOpts struct {
	BeginSlot         int
	AssureFullRemoval bool
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
	if free == 0 && (!stack || (stack && previousCount == 0 && !opts.StockObj)) {
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

	// Clamp at StackLimit.
	addCount := min(count, StackLimit-previousCount)
	if addCount <= 0 {
		return tx
	}

	var written Item
	if !opts.DryRun {
		if inv.Items[stackIndex] == nil {
			inv.Items[stackIndex] = &Item{Id: id, Count: addCount}
		} else {
			inv.Items[stackIndex].Count += addCount
		}
		inv.Update = true
		written = *inv.Items[stackIndex]
	} else {
		written = Item{Id: id, Count: previousCount + addCount}
	}
	tx.Completed = addCount
	tx.Added = []SlotEntry{{Slot: stackIndex, Item: written}}
	return tx
}

func (inv *Inventory) Remove(id, count int, opts RemoveOpts) Transaction {
	tx := Transaction{Requested: count}
	if count <= 0 {
		return tx
	}
	removed := 0
	begin := max(opts.BeginSlot, 0)
	for i := begin; i < inv.Capacity && removed < count; i++ {
		it := inv.Items[i]
		if it == nil || it.Id != id {
			continue
		}
		take := min(count-removed, it.Count)
		it.Count -= take
		removed += take
		if it.Count == 0 {
			inv.Items[i] = nil
		}
	}
	if removed > 0 {
		inv.Update = true
	}
	tx.Completed = removed
	return tx
}
