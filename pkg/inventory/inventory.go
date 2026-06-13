package inventory

import (
	"slices"

	"github.com/zsrv/goscape/pkg/objtype"
)

// Stacking strategy constants. Mirror TS Inventory.NORMAL_STACK / ALWAYS_STACK
// / NEVER_STACK (Inventory.ts:16-18).
const (
	StackNormal = 0 // stack based on ObjType.stackable
	StackAlways = 1 // always stack
	StackNever  = 2 // never stack (each unit its own slot)
)

// StackLimit is the maximum count for a single stacked item. Mirrors TS
// Inventory.STACK_LIMIT = 0x7fffffff (Inventory.ts:14).
const StackLimit = 0x7fffffff

type Item struct {
	Id    int
	Count int
}

// Inventory is the RS2 item container. Ported whole from TS Inventory.ts
// @dee467c8 (rev-274): the all-or-nothing transaction model is gone; Add/Remove
// return a bare completed/removed count (partial fills succeed), and every
// touched slot is recorded in dirtySlots so the per-tick sync can emit
// UpdateInvPartial instead of a full resend.
type Inventory struct {
	Type      int // InvType.ID
	Capacity  int
	StackType int
	Items     []*Item // length == Capacity; nil = empty

	// Update is set whenever the inventory changes; consumed by the per-tick
	// inv-sync (world.updateInvs). Cleared, together with dirtySlots, by
	// ResetTracking in the world cleanup pass. Mirrors TS `inv.update`.
	Update bool

	// dirtySlots records every slot index mutated since the last
	// ResetTracking. GetDirtySlots returns them sorted; the partial encoder
	// walks exactly these. Mirrors TS `dirtySlots: Set<number>`.
	dirtySlots map[int]struct{}

	// stockObjIDs is the InvType's stock-obj list (InvType.stockobj),
	// populated by FromType. Remove consults it to decide whether a slot
	// reaching count 0 is retained as a restockable placeholder — mirroring
	// TS, which computes `InvType.get(this.type).stockobj?.includes(id)`
	// inside remove() (Inventory.ts:153). Keeping the membership on the
	// inventory (rather than per-call) means every caller — INV_DEL/shop-buy,
	// restock — gets the correct behavior without re-deriving it.
	stockObjIDs []uint16
}

// New returns an empty inventory of the given capacity. Mirrors the TS
// constructor (Inventory.ts:57-62).
func New(typeId, capacity, stackType int) *Inventory {
	return &Inventory{
		Type:       typeId,
		Capacity:   capacity,
		StackType:  stackType,
		Items:      make([]*Item, capacity),
		dirtySlots: make(map[int]struct{}),
	}
}

// FromType builds an inventory matching an InvType's size/stack semantics and
// populates its stock items. Mirrors TS Inventory.fromType (Inventory.ts:20-44).
func FromType(t *objtype.InvType) *Inventory {
	inv := New(t.ID, t.Size, stackTypeFrom(t))
	inv.stockObjIDs = t.StockObj
	// TS Inventory.fromType (Inventory.ts:34-41) seeds every stock index with
	// the literal {stockobj[i], stockcount[i]} — no id==0 skip and no count
	// fallback. A count-0 stock slot seeds at 0 and then restocks up toward
	// stockcount; obj id 0 is a valid obj. StockObj/StockCount are allocated in
	// lockstep (invtype.go), so the indices line up. Seeding goes through the
	// raw Items slice (not Set) because fromType pre-dates any listener and
	// must not pre-dirty the brand-new inventory.
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

// Get returns the item at slot, or nil if the slot is empty / out of range.
// Mirrors TS get() (Inventory.ts:220-222) with an added bounds guard.
func (inv *Inventory) Get(slot int) *Item {
	if slot < 0 || slot >= inv.Capacity {
		return nil
	}
	return inv.Items[slot]
}

// HasAt reports whether the item at slot has the given id. Mirrors TS hasAt
// (Inventory.ts:64-67).
func (inv *Inventory) HasAt(slot, id int) bool {
	it := inv.Get(slot)
	return it != nil && it.Id == id
}

// GetItemCount sums every stack of id across the inventory, clamped to
// StackLimit. Mirrors TS getItemCount (Inventory.ts:81-92).
func (inv *Inventory) GetItemCount(id int) int {
	total := 0
	for _, it := range inv.Items {
		if it != nil && it.Id == id {
			total += it.Count
		}
	}
	return min(StackLimit, total)
}

// GetItemIndex returns the first slot holding id, or -1. Mirrors TS
// getItemIndex (Inventory.ts:94-96).
func (inv *Inventory) GetItemIndex(id int) int {
	for i, it := range inv.Items {
		if it != nil && it.Id == id {
			return i
		}
	}
	return -1
}

// NextFreeSlot returns the first empty slot, or -1. Mirrors TS nextFreeSlot
// getter (Inventory.ts:69-71).
func (inv *Inventory) NextFreeSlot() int {
	for i, it := range inv.Items {
		if it == nil {
			return i
		}
	}
	return -1
}

// FreeSlotCount returns how many slots are empty. Mirrors TS freeSlotCount
// getter (Inventory.ts:73-75).
func (inv *Inventory) FreeSlotCount() int {
	n := 0
	for _, it := range inv.Items {
		if it == nil {
			n++
		}
	}
	return n
}

// ValidSlot reports whether slot is in range. Mirrors TS validSlot
// (Inventory.ts:229-231).
func (inv *Inventory) ValidSlot(slot int) bool {
	return slot >= 0 && slot < inv.Capacity
}

// isStockObj reports whether id is in this inventory's InvType stock list
// (TS `InvType.get(this.type).stockobj?.includes(id)`). Populated by FromType;
// a bare New inventory has no stock list and always returns false.
func (inv *Inventory) isStockObj(id int) bool {
	return slices.Contains(inv.stockObjIDs, uint16(id))
}

// Set writes item (or nil) at slot and marks the slot dirty. Mirrors TS set()
// (Inventory.ts:224-227) with an added bounds guard.
func (inv *Inventory) Set(slot int, item *Item) {
	if slot < 0 || slot >= inv.Capacity {
		return
	}
	inv.Items[slot] = item
	inv.markDirty(slot)
}

// Delete empties slot. Mirrors TS delete() (Inventory.ts:216-218).
func (inv *Inventory) Delete(slot int) { inv.Set(slot, nil) }

// RemoveAll empties every occupied slot, marking each one dirty. Empty slots
// are left untouched (not dirtied). Mirrors TS removeAll (Inventory.ts:98-105).
func (inv *Inventory) RemoveAll() {
	for slot := range inv.Items {
		if inv.Items[slot] != nil {
			inv.Items[slot] = nil
			inv.markDirty(slot)
		}
	}
}

// Add inserts up to count units of obj id and returns the number actually
// inserted (the completed count). Ports TS Inventory.add (Inventory.ts:107-150)
// whole. The 274 model has no all-or-nothing validation: a partial fill
// succeeds and the caller computes overflow = count - completed.
//
// stackable is the pre-resolved ObjType.stackable predicate (TS reads it inside
// add() via ObjType.get; goscape resolves it at the call site to keep the
// container decoupled from the config registry).
//
// Stack predicate (TS line 109):
//
//	stack = stackType != StackNever && (stackable || stackType == StackAlways)
func (inv *Inventory) Add(id, count, beginSlot int, stackable bool) int {
	stack := inv.StackType != StackNever && (stackable || inv.StackType == StackAlways)

	completed := 0

	if !stack {
		// TS lines 113-126: non-stack — fill empty slots from max(0, beginSlot).
		startSlot := max(0, beginSlot)
		for i := startSlot; i < inv.Capacity; i++ {
			if inv.Items[i] != nil {
				continue
			}
			inv.Set(i, &Item{Id: id, Count: 1})
			completed++
			if completed >= count {
				break
			}
		}
		return completed
	}

	// TS lines 127-147: stack — find or allocate the stack slot, then clamp.
	stackIndex := inv.GetItemIndex(id)
	if stackIndex == -1 {
		if beginSlot == -1 {
			stackIndex = inv.NextFreeSlot()
		} else {
			// TS items.indexOf(null, beginSlot): first nil slot at-or-after
			// beginSlot.
			stackIndex = -1
			for i := beginSlot; i < inv.Capacity; i++ {
				if inv.Items[i] == nil {
					stackIndex = i
					break
				}
			}
		}
		if stackIndex == -1 {
			return completed
		}
	}

	stackCount := 0
	if s := inv.Get(stackIndex); s != nil {
		stackCount = s.Count
	}
	total := min(StackLimit, stackCount+count)
	inv.Set(stackIndex, &Item{Id: id, Count: total})
	completed = total - stackCount

	return completed
}

// Remove takes up to count units of obj id and returns the number actually
// removed (totalRemoved). Ports TS Inventory.remove (Inventory.ts:152-214)
// whole. A stock-obj slot reaching count 0 is retained as a placeholder; every
// other slot vacates. A beginSlot >= 0 scans [beginSlot, capacity) then wraps
// to the skipped prefix [0, beginSlot).
func (inv *Inventory) Remove(id, count, beginSlot int) int {
	stockObj := inv.isStockObj(id)

	totalRemoved := 0

	// TS lines 166-169: the primary scan starts at beginSlot (or 0).
	index := 0
	if beginSlot != -1 {
		index = beginSlot
	}

	removeFrom := func(lo, hi int) {
		for i := lo; i < hi; i++ {
			cur := inv.Items[i]
			if cur == nil || cur.Id != id {
				continue
			}
			removeCount := min(cur.Count, count-totalRemoved)
			totalRemoved += removeCount
			cur.Count -= removeCount
			if cur.Count == 0 && !stockObj {
				inv.Items[i] = nil
			}
			inv.markDirty(i)
			if totalRemoved >= count {
				return
			}
		}
	}

	removeFrom(index, inv.Capacity)

	// TS lines 191-211: with a beginSlot, wrap to the skipped prefix
	// [0, beginSlot) if the request is not yet satisfied.
	if beginSlot != -1 && totalRemoved < count {
		removeFrom(0, beginSlot)
	}

	return totalRemoved
}

// GetDirtySlots returns the slots touched since the last ResetTracking, sorted
// ascending. Mirrors TS getDirtySlots (Inventory.ts:233-235).
func (inv *Inventory) GetDirtySlots() []int {
	slots := make([]int, 0, len(inv.dirtySlots))
	for s := range inv.dirtySlots {
		slots = append(slots, s)
	}
	slices.Sort(slots)
	return slots
}

// ResetTracking clears the update flag and the dirty-slot set. Called in the
// world cleanup pass after every player's updateInvs has read them. Mirrors TS
// resetTracking (Inventory.ts:237-240).
func (inv *Inventory) ResetTracking() {
	inv.Update = false
	clear(inv.dirtySlots)
}

// markDirty records a slot mutation and sets the update flag. Mirrors TS
// markDirty (Inventory.ts:242-245).
func (inv *Inventory) markDirty(slot int) {
	if inv.dirtySlots == nil {
		inv.dirtySlots = make(map[int]struct{})
	}
	inv.dirtySlots[slot] = struct{}{}
	inv.Update = true
}
