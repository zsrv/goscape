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

func (inv *Inventory) Swap(from, to int) {
	if from < 0 || from >= inv.Capacity || to < 0 || to >= inv.Capacity {
		return
	}
	inv.Items[from], inv.Items[to] = inv.Items[to], inv.Items[from]
	inv.Update = true
}

func (inv *Inventory) Add(id, count int, opts AddOpts) Transaction {
	tx := Transaction{Requested: count}
	if count <= 0 {
		return tx
	}

	shouldStack := inv.StackType == StackAlways && !opts.ForceNoStack

	if shouldStack {
		idx := inv.GetItemIndex(id)
		if idx < 0 {
			idx = inv.findFreeSlotFrom(opts.BeginSlot)
		}
		if idx < 0 {
			return tx
		}
		if !opts.DryRun {
			if inv.Items[idx] == nil {
				inv.Items[idx] = &Item{Id: id, Count: count}
			} else {
				newCount := inv.Items[idx].Count + count
				if newCount > StackLimit {
					newCount = StackLimit
				}
				inv.Items[idx].Count = newCount
			}
			inv.Update = true
		}
		tx.Completed = count
		return tx
	}

	added := 0
	for added < count {
		idx := inv.findFreeSlotFrom(opts.BeginSlot)
		if idx < 0 {
			break
		}
		if !opts.DryRun {
			inv.Items[idx] = &Item{Id: id, Count: 1}
		}
		added++
	}
	if !opts.DryRun && added > 0 {
		inv.Update = true
	}
	if opts.AssureFullInsertion && added < count {
		if !opts.DryRun {
			for i, it := range inv.Items {
				if it != nil && it.Id == id {
					inv.Items[i] = nil
				}
			}
		}
		return tx
	}
	tx.Completed = added
	return tx
}

func (inv *Inventory) findFreeSlotFrom(begin int) int {
	if begin < 0 {
		begin = 0
	}
	for i := begin; i < inv.Capacity; i++ {
		if inv.Items[i] == nil {
			return i
		}
	}
	return -1
}

func (inv *Inventory) Remove(id, count int, opts RemoveOpts) Transaction {
	tx := Transaction{Requested: count}
	if count <= 0 {
		return tx
	}
	removed := 0
	begin := opts.BeginSlot
	if begin < 0 {
		begin = 0
	}
	for i := begin; i < inv.Capacity && removed < count; i++ {
		it := inv.Items[i]
		if it == nil || it.Id != id {
			continue
		}
		take := count - removed
		if take > it.Count {
			take = it.Count
		}
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
