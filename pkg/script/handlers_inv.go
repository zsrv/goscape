package script

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
)

// resolveInv looks up the inventory for typeID via the script's
// InvLookup. Returns nil if InvLookup is unset, the typeID is invalid,
// or the active player has no such inv. All INV_* handlers start with a
// resolveInv nil-check so they never dereference a missing container.
func resolveInv(s *ScriptState, typeID int) *inventory.Inventory {
	if s.Inv == nil {
		return nil
	}
	return s.Inv.Get(s.Self, typeID)
}

// -- Reads --

// handleInvTotal (INV_TOTAL) pops [inv, obj] and pushes the total count
// of obj across all slots of inv. Matches TS popInts(2) order — obj on
// top, inv below.
func handleInvTotal(s *ScriptState) error {
	obj := s.PopInt()
	typeID := s.PopInt()
	// TS INV_TOTAL short-circuits with obj == -1 → push 0.
	if obj == -1 {
		s.PushInt(0)
		return nil
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_TOTAL: no inv for type %d", typeID)
	}
	s.PushInt(inv.GetItemCount(obj))
	return nil
}

// handleInvGetObj (INV_GETOBJ) pops [inv, slot] and pushes the obj id
// at that slot, or -1 if the slot is empty / out of range.
func handleInvGetObj(s *ScriptState) error {
	slot := s.PopInt()
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_GETOBJ: no inv for type %d", typeID)
	}
	it := inv.Get(slot)
	if it == nil {
		s.PushInt(-1)
		return nil
	}
	s.PushInt(it.Id)
	return nil
}

// handleInvGetNum (INV_GETNUM) pops [inv, slot] and pushes the count at
// that slot, or 0 if the slot is empty / out of range.
func handleInvGetNum(s *ScriptState) error {
	slot := s.PopInt()
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_GETNUM: no inv for type %d", typeID)
	}
	it := inv.Get(slot)
	if it == nil {
		s.PushInt(0)
		return nil
	}
	s.PushInt(it.Count)
	return nil
}

// handleInvSize (INV_SIZE) pops an inv id and pushes its Capacity.
func handleInvSize(s *ScriptState) error {
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_SIZE: no inv for type %d", typeID)
	}
	s.PushInt(inv.Capacity)
	return nil
}

// handleInvFreeSpace (INV_FREESPACE) pops an inv id and pushes the
// number of empty slots.
func handleInvFreeSpace(s *ScriptState) error {
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_FREESPACE: no inv for type %d", typeID)
	}
	s.PushInt(inv.FreeSlotCount())
	return nil
}

// invItemSpaceRemaining computes the overflow count — how many of
// `count` units of `obj` would NOT fit into `inv` given stacking +
// size. Mirrors TS Player.invItemSpace.
//
//   - If obj is stackable (or an uncerted link, or the inv always
//     stacks), overflow = max(0, count - (StackLimit - currentTotal)),
//     with the special case that a zero-total + no-free-slots + no
//     stock-slot returns full count.
//   - Otherwise (non-stackable), overflow = max(0, count -
//     (freeSlots - (capacity - size))). `size` is a caller-supplied
//     max-reserved-slot count.
func invItemSpaceRemaining(s *ScriptState, inv *inventory.Inventory, obj, count, size int) int {
	var ot *objtype.ObjType
	if s.Configs != nil {
		ot = s.Configs.ObjType(obj)
	}

	// oc_uncert: a note/cert variant points back at its base. Treat
	// note as "stackable-equivalent" when link >= 0 and template >= 0.
	uncert := obj
	if ot != nil && ot.CertTemplate >= 0 && ot.CertLink >= 0 {
		uncert = ot.CertLink
	}

	stackable := ot != nil && ot.Stackable
	alwaysStack := inv.StackType == inventory.StackAlways

	if stackable || uncert != obj || alwaysStack {
		total := inv.GetItemCount(obj)
		free := inv.FreeSlotCount()
		// Check stock-obj membership via InvType configs.
		stockObj := false
		if s.Configs != nil {
			if it := s.Configs.InvType(inv.Type); it != nil {
				for _, id := range it.StockObj {
					if int(id) == obj {
						stockObj = true
						break
					}
				}
			}
		}
		if total == 0 && free == 0 && !stockObj {
			return count
		}
		room := inventory.StackLimit - total
		if room < 0 {
			room = 0
		}
		rem := count - room
		if rem < 0 {
			rem = 0
		}
		return rem
	}

	// Non-stackable: size is a reserved-slot count. If size >=
	// capacity, no reservation. If size < capacity, only
	// (free - (capacity - size)) slots are usable for this obj.
	free := inv.FreeSlotCount()
	avail := free - (inv.Capacity - size)
	if avail < 0 {
		avail = 0
	}
	rem := count - avail
	if rem < 0 {
		rem = 0
	}
	return rem
}

// handleInvItemSpace (INV_ITEMSPACE) pops [inv, obj, count, size] and
// pushes 1 if the inv can fit `count` of `obj` (overflow == 0), else 0.
// If count == 0, pushes 0 (matches TS).
func handleInvItemSpace(s *ScriptState) error {
	size := s.PopInt()
	count := s.PopInt()
	obj := s.PopInt()
	typeID := s.PopInt()
	if count == 0 {
		s.PushInt(0)
		return nil
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_ITEMSPACE: no inv for type %d", typeID)
	}
	if size < 0 || size > inv.Capacity {
		return fmt.Errorf("INV_ITEMSPACE: size %d out of range for inv %d", size, typeID)
	}
	if invItemSpaceRemaining(s, inv, obj, count, size) == 0 {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}

// handleInvItemSpace2 (INV_ITEMSPACE2) pops [inv, obj, count, size] and
// pushes the overflow count (how many of `count` units would NOT fit).
// If count == 0, pushes 0 (matches TS).
func handleInvItemSpace2(s *ScriptState) error {
	size := s.PopInt()
	count := s.PopInt()
	obj := s.PopInt()
	typeID := s.PopInt()
	if count == 0 {
		s.PushInt(0)
		return nil
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_ITEMSPACE2: no inv for type %d", typeID)
	}
	s.PushInt(invItemSpaceRemaining(s, inv, obj, count, size))
	return nil
}

// handleInvTotalParam (INV_TOTALPARAM) pops [inv, param] and sums the
// per-slot ObjType.Params[param] across every non-empty slot. Missing
// params fall back to ParamType.DefaultInt. Matches TS
// Player._invTotalParam(..., stack=false) — does NOT multiply by slot
// count (that's INV_TOTALPARAM_STACK).
func handleInvTotalParam(s *ScriptState) error {
	param := s.PopInt()
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_TOTALPARAM: no inv for type %d", typeID)
	}
	if s.Configs == nil {
		return fmt.Errorf("INV_TOTALPARAM: Configs not set on ScriptState")
	}
	pt := s.Configs.ParamType(param)
	if pt == nil {
		return fmt.Errorf("INV_TOTALPARAM: unknown param id %d", param)
	}
	total := 0
	for _, it := range inv.Items {
		if it == nil || it.Id < 0 {
			continue
		}
		ot := s.Configs.ObjType(it.Id)
		if ot == nil {
			continue
		}
		if v, ok := ot.Params[uint32(param)]; ok {
			if iv, ok := v.(uint32); ok {
				total += int(iv)
				continue
			}
		}
		total += int(pt.DefaultInt)
	}
	s.PushInt(total)
	return nil
}

// handleInvTotalCat (INV_TOTALCAT) pops [inv, category] and sums the
// counts across non-empty slots whose ObjType.Category == category.
func handleInvTotalCat(s *ScriptState) error {
	category := s.PopInt()
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_TOTALCAT: no inv for type %d", typeID)
	}
	if s.Configs == nil {
		return fmt.Errorf("INV_TOTALCAT: Configs not set on ScriptState")
	}
	total := 0
	for _, it := range inv.Items {
		if it == nil {
			continue
		}
		ot := s.Configs.ObjType(it.Id)
		if ot == nil {
			continue
		}
		if ot.Category == category {
			total += it.Count
		}
	}
	s.PushInt(total)
	return nil
}

// -- Mutations --

// handleInvAdd (INV_ADD) pops [inv, obj, count] and adds count units of
// obj to the inv via inventory.Add. Overflow-to-world drop is NOT
// implemented; the overflow is silently discarded (documented
// limitation — needs active_obj plumbing).
func handleInvAdd(s *ScriptState) error {
	count := s.PopInt()
	obj := s.PopInt()
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_ADD: no inv for type %d", typeID)
	}
	inv.Add(obj, count, inventory.AddOpts{BeginSlot: -1})
	return nil
}

// handleInvDel (INV_DEL) pops [inv, obj, count] and removes count units
// of obj from the inv.
func handleInvDel(s *ScriptState) error {
	count := s.PopInt()
	obj := s.PopInt()
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_DEL: no inv for type %d", typeID)
	}
	inv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
	return nil
}

// handleInvDelSlot (INV_DELSLOT) pops [inv, slot] and clears that slot.
// Out-of-range slots are silently ignored by inventory.Delete.
func handleInvDelSlot(s *ScriptState) error {
	slot := s.PopInt()
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_DELSLOT: no inv for type %d", typeID)
	}
	inv.Delete(slot)
	return nil
}

// handleInvSetSlot (INV_SETSLOT) pops [inv, slot, obj, count] and
// replaces the slot with {obj, count}. Matches TS popInts(4) order —
// count on top. Out-of-range slot is silently ignored.
func handleInvSetSlot(s *ScriptState) error {
	count := s.PopInt()
	obj := s.PopInt()
	slot := s.PopInt()
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_SETSLOT: no inv for type %d", typeID)
	}
	inv.Set(slot, &inventory.Item{Id: obj, Count: count})
	return nil
}

// handleInvClear (INV_CLEAR) pops an inv id and empties every slot.
func handleInvClear(s *ScriptState) error {
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_CLEAR: no inv for type %d", typeID)
	}
	inv.Clear()
	return nil
}

// handleInvMoveItem (INV_MOVEITEM) pops [fromInv, toInv, obj, count]
// and moves up to `count` of `obj` from fromInv to toInv. Remove is
// performed first, then Add with the removed count (matches TS). Both
// invs must resolve before any mutation.
func handleInvMoveItem(s *ScriptState) error {
	count := s.PopInt()
	obj := s.PopInt()
	toTypeID := s.PopInt()
	fromTypeID := s.PopInt()
	fromInv := resolveInv(s, fromTypeID)
	if fromInv == nil {
		return fmt.Errorf("INV_MOVEITEM: no inv for from-type %d", fromTypeID)
	}
	toInv := resolveInv(s, toTypeID)
	if toInv == nil {
		return fmt.Errorf("INV_MOVEITEM: no inv for to-type %d", toTypeID)
	}
	tx := fromInv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
	if tx.Completed == 0 {
		return nil
	}
	toInv.Add(obj, tx.Completed, inventory.AddOpts{BeginSlot: -1})
	return nil
}

// handleInvMoveFromSlot (INV_MOVEFROMSLOT) pops [fromInv, toInv,
// fromSlot] and moves the entire slot contents from fromInv to toInv.
func handleInvMoveFromSlot(s *ScriptState) error {
	fromSlot := s.PopInt()
	toTypeID := s.PopInt()
	fromTypeID := s.PopInt()
	fromInv := resolveInv(s, fromTypeID)
	if fromInv == nil {
		return fmt.Errorf("INV_MOVEFROMSLOT: no inv for from-type %d", fromTypeID)
	}
	toInv := resolveInv(s, toTypeID)
	if toInv == nil {
		return fmt.Errorf("INV_MOVEFROMSLOT: no inv for to-type %d", toTypeID)
	}
	it := fromInv.Get(fromSlot)
	if it == nil {
		return fmt.Errorf("INV_MOVEFROMSLOT: from slot %d empty", fromSlot)
	}
	// Capture before mutating — Delete nulls the slot pointer.
	id, cnt := it.Id, it.Count
	fromInv.Delete(fromSlot)
	toInv.Add(id, cnt, inventory.AddOpts{BeginSlot: -1})
	return nil
}

// -- Listener registration (S6u) -----------------------------------------

// handleInvTransmit implements INV_TRANSMIT. Registers a listener on
// the active player for UI component `com` tracking the active
// player's own inventory of type `invType` (source = activePlayer.uid).
//
// TS: InvOps.ts INV_TRANSMIT — `const [inv, com] = state.popInts(2)`
// (popInts returns [bottom, top] — i.e. inv pushed FIRST, com pushed
// SECOND). Then activePlayer.invListenOnCom(inv, com, activePlayer.uid).
// com is wrapped with check(com, NumberNotNull) in TS; invType uses
// InvTypeValid (not NumberNotNull) — stays raw (NAI-23 Bundle 4b).
// Source porting fix landed in NAI-24 Bundle 2 — origin commit fa57ee4
// (S6u) erroneously hard-coded -1.
//
// NAI-113 T9: pop order corrected to match TS (com from top, invType
// from below). Pre-fix the assignments were swapped — INV_TRANSMIT
// stored Type=<com>, Com=<inv>, masking inventory side-panel emission
// in production (the listener Type never matched any allocated inv).
// Sibling INVOTHER_TRANSMIT was already correct; only this opcode was
// inverted. Existing TestInvTransmitRegistersListener was hand-tuned
// to the buggy order and is migrated here.
func handleInvTransmit(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_TRANSMIT"); err != nil {
		return err
	}
	com := s.PopInt()
	invType := s.PopInt()
	if err := checkNotNull(com, "INV_TRANSMIT"); err != nil {
		return err
	}
	s.Self.InvListenOnCom(invType, com, s.Self.UID())
	return nil
}

// handleInvStopTransmit implements INV_STOPTRANSMIT. Unregisters the
// listener at UI component `com`. Safe when no listener exists there.
//
// TS: InvOps.ts INV_STOPTRANSMIT — check(state.popInt(), NumberNotNull),
// activePlayer.invStopListenOnCom(com). com is wrapped with NumberNotNull
// (NAI-23 Bundle 4b).
func handleInvStopTransmit(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_STOPTRANSMIT"); err != nil {
		return err
	}
	com := s.PopInt()
	if err := checkNotNull(com, "INV_STOPTRANSMIT"); err != nil {
		return err
	}
	s.Self.InvStopListenOnCom(com)
	return nil
}

// handleInvOtherTransmit implements INVOTHER_TRANSMIT (opcode 4332).
// 3-arg variant of INV_TRANSMIT: registers a listener on the active
// player at UI component `com` tracking inv type `invType` with source
// = `uid` (another player's server slot). Used by trade/shop/bank-view
// flows where the viewer watches another player's inventory.
//
// TS: InvOps.ts INVOTHER_TRANSMIT — popInts(3) → [uid, inv, com];
// check(uid, NumberNotNull), check(inv, InvTypeValid),
// check(com, NumberNotNull); activePlayer.invListenOnCom(invType.id, com, uid).
// uid and com are wrapped with NumberNotNull; invType uses InvTypeValid
// (not NumberNotNull) — stays raw (NAI-23 Bundle 4b). Closes S6u-SB1.
func handleInvOtherTransmit(s *ScriptState) error {
	if err := requireActivePlayer(s, "INVOTHER_TRANSMIT"); err != nil {
		return err
	}
	com := s.PopInt()
	if err := checkNotNull(com, "INVOTHER_TRANSMIT"); err != nil {
		return err
	}
	invType := s.PopInt()
	uid := s.PopInt()
	if err := checkNotNull(uid, "INVOTHER_TRANSMIT"); err != nil {
		return err
	}
	s.Self.InvListenOnCom(invType, com, uid)
	return nil
}

// handleInvDropSlot (INV_DROPSLOT, opcode 4312) drops the entire stack at
// slot from inv onto the ground at coord with private (caller-only)
// visibility for duration ticks. Mirrors TS InvOps.ts:213-260.
//
// Pop order: [inv, coord, slot, duration] — duration on top-of-stack.
//
// Validation order (mirrors TS): InvTypeValid → DurationValid → CoordValid
// → protect gate → slot lookup → empty check → addWealthEvent (D1) →
// invDel → completed check → AddObj per-item or stacked.
//
// Protect gate (conditional per InvType): when invType.Protect is true AND
// invType.Scope is not InvTypeScopeShared, require s.Protect via
// requireProtectedActivePlayer. Otherwise require only ActivePlayer.
// Gate is checked AFTER validators, matching TS opcode dispatch order.
//
// NAI-115-D1 deviation: TS inlines addWealthEvent for SCOPE_PERM drops.
// (goscape: skipped; content can emit via OpWealthEvent 2131 — TS calls
// addWealthEvent here.)
//
// NAI-115-D3 deviation: TS sets state.activeObj = floorObj and calls
// pointerAdd(ActiveObj) after each AddObj. goscape's WorldVars.AddObj has
// no world→state writeback path; omitted here (smoke-binding).
func handleInvDropSlot(s *ScriptState) error {
	duration := s.PopInt()
	slot := s.PopInt()
	coord := s.PopInt()
	invID := s.PopInt()

	if s.Configs == nil {
		return fmt.Errorf("INV_DROPSLOT: no configs")
	}

	// InvTypeValid: resolve InvType config.
	invType := s.Configs.InvType(invID)
	if invType == nil {
		return fmt.Errorf("INV_DROPSLOT: invalid inv id (%d)", invID)
	}

	// DurationValid.
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("INV_DROPSLOT: %w", err)
	}

	// CoordValid.
	level, x, z, err := checkCoord(coord, "INV_DROPSLOT")
	if err != nil {
		return err
	}

	// Protect gate: conditional on InvType protect + scope.
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
		if err := requireProtectedActivePlayer(s, "INV_DROPSLOT"); err != nil {
			return err
		}
	} else {
		if err := requireActivePlayer(s, "INV_DROPSLOT"); err != nil {
			return err
		}
	}

	if s.World == nil {
		return fmt.Errorf("INV_DROPSLOT: no world surface")
	}

	// Resolve inventory and get slot contents.
	inv := resolveInv(s, invID)
	if inv == nil {
		return fmt.Errorf("INV_DROPSLOT: inv unresolved (id=%d)", invID)
	}
	it := inv.Get(slot)
	if it == nil {
		return fmt.Errorf("INV_DROPSLOT: $slot is empty (slot=%d)", slot)
	}

	objID := it.Id
	count := it.Count

	// Validate obj config (mirrors TS ObjType.get after slot lookup).
	if s.Configs.ObjType(objID) == nil {
		return fmt.Errorf("INV_DROPSLOT: invalid obj id at slot (id=%d)", objID)
	}
	objType := s.Configs.ObjType(objID)

	// NAI-115-D1: TS calls addWealthEvent here for SCOPE_PERM. Skipped.
	// (goscape: content can emit via OpWealthEvent 2131.)

	// Slot-scoped removal: mirrors TS player.invDel(invType.id, obj.id,
	// obj.count, slot). completed = count removed (constrained to the slot).
	completed := count
	inv.Delete(slot)
	if completed == 0 {
		return nil
	}

	// Caller-only (private) drop: receiverID = active player's UID.
	receiverID := s.Self.UID()

	// Stackable branch: mirrors TS. NAI-115-D3: no activeObj writeback.
	if !objType.Stackable || completed == 1 {
		for range completed {
			s.World.AddObj(level, x, z, objID, 1, duration, receiverID)
		}
	} else {
		s.World.AddObj(level, x, z, objID, completed, duration, receiverID)
	}
	return nil
}

// handleInvMoveToSlot (INV_MOVETOSLOT) pops [fromInv, toInv, fromSlot,
// toSlot] and swaps the two slot contents (nil-safe both directions).
// Matches TS Player.invMoveToSlot.
func handleInvMoveToSlot(s *ScriptState) error {
	toSlot := s.PopInt()
	fromSlot := s.PopInt()
	toTypeID := s.PopInt()
	fromTypeID := s.PopInt()
	fromInv := resolveInv(s, fromTypeID)
	if fromInv == nil {
		return fmt.Errorf("INV_MOVETOSLOT: no inv for from-type %d", fromTypeID)
	}
	toInv := resolveInv(s, toTypeID)
	if toInv == nil {
		return fmt.Errorf("INV_MOVETOSLOT: no inv for to-type %d", toTypeID)
	}
	// Snapshot both ends; Set/Delete may rewrite the original slot when
	// from == to, so copy the item fields out first.
	var fromCopy, toCopy *inventory.Item
	if src := fromInv.Get(fromSlot); src != nil {
		fromCopy = &inventory.Item{Id: src.Id, Count: src.Count}
	}
	if dst := toInv.Get(toSlot); dst != nil {
		toCopy = &inventory.Item{Id: dst.Id, Count: dst.Count}
	}
	if fromCopy != nil {
		toInv.Set(toSlot, fromCopy)
	} else {
		toInv.Delete(toSlot)
	}
	if toCopy != nil {
		fromInv.Set(fromSlot, toCopy)
	} else {
		fromInv.Delete(fromSlot)
	}
	return nil
}
