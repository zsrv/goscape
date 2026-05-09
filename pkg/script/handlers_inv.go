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
				// NAI-122 in-scope-stretch: sign-extend through int32
				// so negative-encoded bonuses sum correctly. See
				// paramLookup in handlers_config.go for the rationale.
				total += int(int32(iv))
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

// handleInvAdd ports TS InvOps.ts:57-83 (INV_ADD, opcode 4302). Pops
// [inv, obj, count]; validates each via TS check chain (InvTypeValid,
// ObjTypeValid, ObjStackValid), enforces the protect/scope gate, and
// rejects dummy items in non-dummy invs. Adds count units of obj to
// the inv via Inventory.Add with caller-precomputed Stackable/StockObj
// flags. Per TS, any overflow drops to the world at the player's tile
// via World.AddObj — branched on (!stackable || overflow == 1) for the
// per-unit-loop case vs the single-stack-drop case (TS InvOps.ts:73-82,
// duration=200).
//
// Validator chain (NAI-131): InvTypeValid → ObjTypeValid → ObjStackValid
// → protect/scope (rejects unprotected scripts when invType.Protect &&
// scope != SHARED) → dummyitem (rejects ObjType.DummyItem != 0 when
// invType.DummyInv == false). All 5 gates throw in TS; goscape returns
// errors with TS-shaped literals.
//
// DEVIATION-NAI-130-D2: defensive nil-World guard skips the overflow
// drop when s.World is unset (goscape defensive; TS uses static World
// import which is never null). Per defensive_gate_doc_comment_label.
//
// DEVIATION-NAI-130-D3: defensive nil-Configs fallback in
// lookupStackableStockObj retained for sibling callers (handleInvMoveItem
// etc.); INV_ADD itself is now ObjType-validated before the helper
// runs, making the fallback unreachable on this path. The defensive
// fallback stays for the sibling Move handlers (NAI-131 T4).
func handleInvAdd(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_ADD"); err != nil {
		return err
	}
	count := s.PopInt()
	obj := s.PopInt()
	typeID := s.PopInt()

	// TS InvOps.ts:60-62 — InvTypeValid, ObjTypeValid, ObjStackValid.
	if err := checkInvType(s, typeID, "INV_ADD"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_ADD"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_ADD"); err != nil {
		return err
	}

	invType := s.Configs.InvType(typeID)
	objType := s.Configs.ObjType(obj)

	// TS InvOps.ts:64-66 — protect/scope gate.
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_ADD: $inv requires protected access: %s", invType.DebugName)
	}

	// TS InvOps.ts:68-70 — dummyitem-in-non-dummyinv gate.
	if !invType.DummyInv && objType.DummyItem != 0 {
		return fmt.Errorf("INV_ADD: dummyitem in non-dummyinv: %s -> %s", objType.DebugName, invType.DebugName)
	}

	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
		return fmt.Errorf("INV_ADD: no inv for type %d", typeID)
	}

	stackable, stockObj := lookupStackableStockObj(s, inv.Type, obj)

	tx := inv.Add(obj, count, inventory.AddOpts{
		BeginSlot:           -1,
		AssureFullInsertion: false,
		Stackable:           stackable,
		StockObj:            stockObj,
	})

	overflow := count - tx.Completed
	if overflow > 0 && s.World != nil {
		level := (s.Self.CoordPacked() >> 28) & 0x3
		x := s.Self.X()
		z := s.Self.Z()
		receiverID := s.Self.UID()
		if !stackable || overflow == 1 {
			for range overflow {
				s.World.AddObj(level, x, z, obj, 1, 200, receiverID)
			}
		} else {
			s.World.AddObj(level, x, z, obj, overflow, 200, receiverID)
		}
	}

	return nil
}

// lookupStackableStockObj returns the (Stackable, StockObj) pair for the
// given (invType, objId), pre-computed from s.Configs for inventory.Add
// to consume. Returns (false, false) on nil-Configs / missing types
// (goscape defensive — see DEVIATION-NAI-130-D3).
func lookupStackableStockObj(s *ScriptState, invTypeID, objID int) (stackable, stockObj bool) {
	if s.Configs == nil {
		return false, false
	}
	if ot := s.Configs.ObjType(objID); ot != nil {
		stackable = ot.Stackable
	}
	if it := s.Configs.InvType(invTypeID); it != nil {
		for _, id := range it.StockObj {
			if int(id) == objID {
				stockObj = true
				break
			}
		}
	}
	return stackable, stockObj
}

// handleInvDel (INV_DEL) ports TS InvOps.ts:129-141. Pops [inv, obj,
// count] and removes count units of obj from the inv. Validates via
// TS check chain (no dummyitem gate — TS doesn't apply it on DEL).
//
// Validator chain (NAI-131): InvTypeValid → ObjTypeValid → ObjStackValid
// → protect/scope.
func handleInvDel(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_DEL"); err != nil {
		return err
	}
	count := s.PopInt()
	obj := s.PopInt()
	typeID := s.PopInt()

	if err := checkInvType(s, typeID, "INV_DEL"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_DEL"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_DEL"); err != nil {
		return err
	}

	invType := s.Configs.InvType(typeID)
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_DEL: $inv requires protected access: %s", invType.DebugName)
	}

	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_DEL: no inv for type %d", typeID)
	}
	inv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
	return nil
}

// handleInvDelSlot (INV_DELSLOT) ports TS InvOps.ts:144-159. Pops
// [inv, slot] and clears that slot. Out-of-range slots are silently
// ignored by inventory.Delete (matches TS).
//
// Validator chain (NAI-131): InvTypeValid → protect/scope.
func handleInvDelSlot(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_DELSLOT"); err != nil {
		return err
	}
	slot := s.PopInt()
	typeID := s.PopInt()

	if err := checkInvType(s, typeID, "INV_DELSLOT"); err != nil {
		return err
	}

	invType := s.Configs.InvType(typeID)
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_DELSLOT: $inv requires protected access: %s", invType.DebugName)
	}

	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_DELSLOT: no inv for type %d", typeID)
	}
	inv.Delete(slot)
	return nil
}

// handleInvSetSlot (INV_SETSLOT) ports TS InvOps.ts:600-616. Pops
// [inv, slot, obj, count] (popInts(4) order — count on top). Validates
// via TS check chain, enforces protect/scope and dummyitem gates, then
// replaces the slot with {obj, count}. Out-of-range slot is silently
// ignored by inv.Set (matches TS Inventory.set behavior).
//
// Validator chain (NAI-131): InvTypeValid → ObjTypeValid → ObjStackValid
// → protect/scope → dummyitem.
func handleInvSetSlot(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_SETSLOT"); err != nil {
		return err
	}
	count := s.PopInt()
	obj := s.PopInt()
	slot := s.PopInt()
	typeID := s.PopInt()

	if err := checkInvType(s, typeID, "INV_SETSLOT"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_SETSLOT"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_SETSLOT"); err != nil {
		return err
	}

	invType := s.Configs.InvType(typeID)
	objType := s.Configs.ObjType(obj)

	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_SETSLOT: $inv requires protected access: %s", invType.DebugName)
	}

	if !invType.DummyInv && objType.DummyItem != 0 {
		return fmt.Errorf("INV_SETSLOT: dummyitem in non-dummyinv: %s -> %s", objType.DebugName, invType.DebugName)
	}

	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_SETSLOT: no inv for type %d", typeID)
	}
	inv.Set(slot, &inventory.Item{Id: obj, Count: count})
	return nil
}

// handleInvClear (INV_CLEAR) ports TS InvOps.ts:116-124. Pops an inv
// id and empties every slot.
//
// Validator chain (NAI-131): InvTypeValid → protect/scope.
func handleInvClear(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_CLEAR"); err != nil {
		return err
	}
	typeID := s.PopInt()

	if err := checkInvType(s, typeID, "INV_CLEAR"); err != nil {
		return err
	}

	invType := s.Configs.InvType(typeID)
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_CLEAR: $inv requires protected access: %s", invType.DebugName)
	}

	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_CLEAR: no inv for type %d", typeID)
	}
	inv.Clear()
	return nil
}

// handleInvMoveItem (INV_MOVEITEM) ports TS InvOps.ts:499-531. Pops
// [fromInv, toInv, obj, count] and moves up to count of obj from
// fromInv to toInv. Remove first, then Add with the removed count
// (matches TS).
//
// Validator chain (NAI-131): from-InvTypeValid → to-InvTypeValid →
// ObjTypeValid → ObjStackValid → from-protect/scope → to-protect/scope.
//
// DEVIATION-NAI-131-D1: TS asymmetry — both protect/scope gates check
// fromInvType.scope !== SCOPE_SHARED (toInv's own scope is never
// consulted). Pinned per ts_asymmetry_dual_pin.md (positive presence
// + absence-pin in tests). Escalates if upstream TS fixes.
func handleInvMoveItem(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_MOVEITEM"); err != nil {
		return err
	}
	count := s.PopInt()
	obj := s.PopInt()
	toTypeID := s.PopInt()
	fromTypeID := s.PopInt()

	if err := checkInvType(s, fromTypeID, "INV_MOVEITEM"); err != nil {
		return err
	}
	if err := checkInvType(s, toTypeID, "INV_MOVEITEM"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_MOVEITEM"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_MOVEITEM"); err != nil {
		return err
	}

	fromInvType := s.Configs.InvType(fromTypeID)
	toInvType := s.Configs.InvType(toTypeID)

	// TS InvOps.ts:507-509 — from-protect gate uses fromInv.scope.
	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_MOVEITEM: $inv requires protected access: %s", fromInvType.DebugName)
	}
	// TS InvOps.ts:511-513 — to-protect gate ALSO uses fromInv.scope (DEVIATION-NAI-131-D1).
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_MOVEITEM: $inv requires protected access: %s", toInvType.DebugName)
	}

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
	stackable, stockObj := lookupStackableStockObj(s, toInv.Type, obj)
	toInv.Add(obj, tx.Completed, inventory.AddOpts{
		BeginSlot: -1,
		Stackable: stackable,
		StockObj:  stockObj,
	})
	return nil
}

// handleInvMoveFromSlot (INV_MOVEFROMSLOT) ports TS InvOps.ts:323-349.
// Pops [fromInv, toInv, fromSlot] and moves the entire slot contents
// from fromInv to toInv.
//
// Validator chain (NAI-131): from-InvTypeValid → to-InvTypeValid →
// from-protect/scope → to-protect/scope. No Obj-gates (the obj id
// comes from the source slot, not a stack-pushed input).
//
// DEVIATION-NAI-131-D1 applies (see handleInvMoveItem above).
func handleInvMoveFromSlot(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_MOVEFROMSLOT"); err != nil {
		return err
	}
	fromSlot := s.PopInt()
	toTypeID := s.PopInt()
	fromTypeID := s.PopInt()

	if err := checkInvType(s, fromTypeID, "INV_MOVEFROMSLOT"); err != nil {
		return err
	}
	if err := checkInvType(s, toTypeID, "INV_MOVEFROMSLOT"); err != nil {
		return err
	}

	fromInvType := s.Configs.InvType(fromTypeID)
	toInvType := s.Configs.InvType(toTypeID)

	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_MOVEFROMSLOT: $inv requires protected access: %s", fromInvType.DebugName)
	}
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_MOVEFROMSLOT: $inv requires protected access: %s", toInvType.DebugName)
	}

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
	id, cnt := it.Id, it.Count
	fromInv.Delete(fromSlot)
	stackable, stockObj := lookupStackableStockObj(s, toInv.Type, id)
	toInv.Add(id, cnt, inventory.AddOpts{
		BeginSlot: -1,
		Stackable: stackable,
		StockObj:  stockObj,
	})
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
// invType.Scope is not InvTypeScopeShared, require PtrProtectedActivePlayer via
// requireProtectedActivePlayer. Otherwise require only ActivePlayer.
// Gate is checked AFTER validators, matching TS opcode dispatch order.
//
// NAI-115-D1 deviation: TS inlines addWealthEvent for SCOPE_PERM drops.
// (goscape: skipped; content can emit via OpWealthEvent 2131 — TS calls
// addWealthEvent here.)
//
// NAI-115-D3 retired in-bundle stretch: AddObj now returns ActiveObj;
// state.activeObj writeback + pointerAdd(ActiveObj) wired below, matching
// TS InvOps.ts:248-258.
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

	// Stackable branch: mirrors TS InvOps.ts:248-258.
	// state.activeObj set after each spawn; last wins for non-stackable count=N.
	if !objType.Stackable || completed == 1 {
		for range completed {
			obj := s.World.AddObj(level, x, z, objID, 1, duration, receiverID)
			if obj != nil {
				s.ActiveObj = obj
				s.Pointers |= PtrActiveObj
			}
		}
	} else {
		obj := s.World.AddObj(level, x, z, objID, completed, duration, receiverID)
		if obj != nil {
			s.ActiveObj = obj
			s.Pointers |= PtrActiveObj
		}
	}
	return nil
}

// handleInvMoveToSlot (INV_MOVETOSLOT) ports TS InvOps.ts:353-368. Pops
// [fromInv, toInv, fromSlot, toSlot] (popInts(4) — toSlot on top) and
// swaps the two slot contents (nil-safe both directions). Matches TS
// Player.invMoveToSlot.
//
// Validator chain (NAI-131): from-InvTypeValid → to-InvTypeValid →
// from-protect/scope → to-protect/scope.
//
// DEVIATION-NAI-131-D1: TS asymmetry — both protect/scope gates check
// fromInvType.Scope, never toInvType.Scope. Pinned per ts_asymmetry_dual_pin.md.
func handleInvMoveToSlot(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_MOVETOSLOT"); err != nil {
		return err
	}
	toSlot := s.PopInt()
	fromSlot := s.PopInt()
	toTypeID := s.PopInt()
	fromTypeID := s.PopInt()

	if err := checkInvType(s, fromTypeID, "INV_MOVETOSLOT"); err != nil {
		return err
	}
	if err := checkInvType(s, toTypeID, "INV_MOVETOSLOT"); err != nil {
		return err
	}

	fromInvType := s.Configs.InvType(fromTypeID)
	toInvType := s.Configs.InvType(toTypeID)

	// TS InvOps.ts:359-361 — from-protect gate uses fromInvType.Scope.
	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_MOVETOSLOT: $inv requires protected access: %s", fromInvType.DebugName)
	}
	// TS InvOps.ts:363-365 — to-protect gate ALSO uses fromInvType.Scope (DEVIATION-NAI-131-D1).
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_MOVETOSLOT: $inv requires protected access: %s", toInvType.DebugName)
	}

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

// handleInvChangeSlot (INV_CHANGESLOT) ports TS InvOps.ts:86-113. Pops
// [inv, find, replace, replaceCount]. Loops the inventory for the first
// slot whose item.Id == findObj.Id; on hit, replaces with replaceObj.Id
// at replaceCount. No-match is a silent no-op.
//
// Validator chain (NAI-131 shape, partial): InvTypeValid → protect/scope
// → ObjTypeValid(find) → ObjTypeValid(replace). NOTE: TS does NOT
// validate replaceCount (no `check(count, ObjStackValid)` at InvOps.ts:86-113).
// Goscape preserves this — pop-without-validate is intentional;
// absence-pinned via TestInvChangeSlot_ReplaceCountZeroAbsencePin.
func handleInvChangeSlot(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_CHANGESLOT"); err != nil {
		return err
	}
	replaceCount := s.PopInt()
	replace := s.PopInt()
	find := s.PopInt()
	typeID := s.PopInt()

	if err := checkInvType(s, typeID, "INV_CHANGESLOT"); err != nil {
		return err
	}

	invType := s.Configs.InvType(typeID)
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_CHANGESLOT: $inv requires protected access: %s", invType.DebugName)
	}

	if err := checkObjType(s, find, "INV_CHANGESLOT"); err != nil {
		return err
	}
	if err := checkObjType(s, replace, "INV_CHANGESLOT"); err != nil {
		return err
	}

	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_CHANGESLOT: no inv for type %d", typeID)
	}

	findObj := s.Configs.ObjType(find)
	replaceObj := s.Configs.ObjType(replace)
	for slot := 0; slot < inv.Capacity; slot++ {
		it := inv.Get(slot)
		if it == nil {
			continue
		}
		if it.Id == findObj.ID {
			inv.Set(slot, &inventory.Item{Id: replaceObj.ID, Count: replaceCount})
			return nil
		}
	}
	return nil
}

// handleInvMoveItemCert (INV_MOVEITEM_CERT) ports TS InvOps.ts:535-566.
// Pops [fromInv, toInv, obj, count]. invDel → if obj is certifiable
// (CertTemplate == -1 && CertLink >= 0) finalObj=CertLink; invAdd
// finalObj. Overflow drops to world as a single stacked Obj — TS
// comment "should be a stackable cert already" → no per-item branch.
//
// Validator chain (NAI-131): from-InvTypeValid → to-InvTypeValid →
// ObjTypeValid → ObjStackValid → from-protect/scope → to-protect/scope
// (DEVIATION-NAI-131-D1: both gates evaluate fromInvType.Scope).
//
// DEVIATION-NAI-130-D2: defensive nil-World guard skips overflow drop
// when s.World is unset (goscape defensive; TS uses static World import).
func handleInvMoveItemCert(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_MOVEITEM_CERT"); err != nil {
		return err
	}
	count := s.PopInt()
	obj := s.PopInt()
	toTypeID := s.PopInt()
	fromTypeID := s.PopInt()

	if err := checkInvType(s, fromTypeID, "INV_MOVEITEM_CERT"); err != nil {
		return err
	}
	if err := checkInvType(s, toTypeID, "INV_MOVEITEM_CERT"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_MOVEITEM_CERT"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_MOVEITEM_CERT"); err != nil {
		return err
	}

	fromInvType := s.Configs.InvType(fromTypeID)
	toInvType := s.Configs.InvType(toTypeID)

	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_MOVEITEM_CERT: $inv requires protected access: %s", fromInvType.DebugName)
	}
	// DEVIATION-NAI-131-D1: to-gate uses fromInvType.Scope (mirrors TS; goscape defensive label).
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_MOVEITEM_CERT: $inv requires protected access: %s", toInvType.DebugName)
	}

	fromInv := resolveInv(s, fromTypeID)
	if fromInv == nil {
		return fmt.Errorf("INV_MOVEITEM_CERT: no inv for from-type %d", fromTypeID)
	}
	toInv := resolveInv(s, toTypeID)
	if toInv == nil {
		return fmt.Errorf("INV_MOVEITEM_CERT: no inv for to-type %d", toTypeID)
	}

	tx := fromInv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
	if tx.Completed == 0 {
		return nil
	}

	objType := s.Configs.ObjType(obj)
	finalObj := obj
	// CERT gate: INVERTED vs UNCERT — certifiable item has CertTemplate==-1 && CertLink>=0.
	if objType.CertTemplate == -1 && objType.CertLink >= 0 {
		finalObj = objType.CertLink
	}
	stackable, stockObj := lookupStackableStockObj(s, toInv.Type, finalObj)
	tx2 := toInv.Add(finalObj, tx.Completed, inventory.AddOpts{
		BeginSlot: -1,
		Stackable: stackable,
		StockObj:  stockObj,
	})

	overflow := count - tx2.Completed
	// DEVIATION-NAI-130-D2: defensive nil-World guard (goscape defensive; TS skips this check).
	if overflow > 0 && s.World != nil {
		level := (s.Self.CoordPacked() >> 28) & 0x3
		receiverID := s.Self.UID()
		// TS comment: "should be a stackable cert already" → single stacked drop.
		s.World.AddObj(level, s.Self.X(), s.Self.Z(), finalObj, overflow, 200, receiverID)
	}
	return nil
}

// handleInvMoveItemUncert (INV_MOVEITEM_UNCERT) ports TS InvOps.ts:570-597.
// Pops [fromInv, toInv, obj, count]. invDel → if obj is a certificate
// (CertTemplate >= 0 && CertLink >= 0) add CertLink to toInv else add
// obj.Id. No overflow-to-world drop (TS InvOps.ts:593-595 just calls
// player.invAdd without overflow-handling).
//
// Validator chain (NAI-131): from-InvTypeValid → to-InvTypeValid →
// ObjTypeValid → ObjStackValid → from-protect/scope → to-protect/scope
// (DEVIATION-NAI-131-D1: both gates evaluate fromInvType.Scope).
func handleInvMoveItemUncert(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_MOVEITEM_UNCERT"); err != nil {
		return err
	}
	count := s.PopInt()
	obj := s.PopInt()
	toTypeID := s.PopInt()
	fromTypeID := s.PopInt()

	if err := checkInvType(s, fromTypeID, "INV_MOVEITEM_UNCERT"); err != nil {
		return err
	}
	if err := checkInvType(s, toTypeID, "INV_MOVEITEM_UNCERT"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_MOVEITEM_UNCERT"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_MOVEITEM_UNCERT"); err != nil {
		return err
	}

	fromInvType := s.Configs.InvType(fromTypeID)
	toInvType := s.Configs.InvType(toTypeID)

	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_MOVEITEM_UNCERT: $inv requires protected access: %s", fromInvType.DebugName)
	}
	// DEVIATION-NAI-131-D1: to-gate uses fromInvType.Scope.
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_MOVEITEM_UNCERT: $inv requires protected access: %s", toInvType.DebugName)
	}

	fromInv := resolveInv(s, fromTypeID)
	if fromInv == nil {
		return fmt.Errorf("INV_MOVEITEM_UNCERT: no inv for from-type %d", fromTypeID)
	}
	toInv := resolveInv(s, toTypeID)
	if toInv == nil {
		return fmt.Errorf("INV_MOVEITEM_UNCERT: no inv for to-type %d", toTypeID)
	}

	tx := fromInv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
	if tx.Completed == 0 {
		return nil
	}

	objType := s.Configs.ObjType(obj)
	finalObj := obj
	if objType.CertTemplate >= 0 && objType.CertLink >= 0 {
		finalObj = objType.CertLink
	}
	stackable, stockObj := lookupStackableStockObj(s, toInv.Type, finalObj)
	toInv.Add(finalObj, tx.Completed, inventory.AddOpts{
		BeginSlot: -1,
		Stackable: stackable,
		StockObj:  stockObj,
	})
	return nil
}

// handleInvDropItem (INV_DROPITEM) ports TS InvOps.ts:163-186. Pops
// [inv, coord, obj, count, duration]. Removes count of obj from inv,
// then drops the removed count to the world at coord as a single stacked
// Obj (TS InvOps.ts:181-184 — one Obj-construct + one World.addObj call;
// stackability is irrelevant). Sets ActiveObj + PtrActiveObj.
//
// Validator chain (NAI-131): InvTypeValid → CoordValid → ObjTypeValid
// → ObjStackValid → DurationValid → protect/scope.
//
// DEVIATION-NAI-130-D2: defensive nil-World guard returns clean error.
func handleInvDropItem(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_DROPITEM"); err != nil {
		return err
	}
	duration := s.PopInt()
	count := s.PopInt()
	obj := s.PopInt()
	coord := s.PopInt()
	invID := s.PopInt()

	if err := checkInvType(s, invID, "INV_DROPITEM"); err != nil {
		return err
	}
	level, x, z, err := checkCoord(coord, "INV_DROPITEM")
	if err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_DROPITEM"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_DROPITEM"); err != nil {
		return err
	}
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("INV_DROPITEM: %w", err)
	}

	invType := s.Configs.InvType(invID)
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("INV_DROPITEM: $inv requires protected access: %s", invType.DebugName)
	}

	inv := resolveInv(s, invID)
	if inv == nil {
		return fmt.Errorf("INV_DROPITEM: inv unresolved (id=%d)", invID)
	}
	tx := inv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
	completed := tx.Completed
	if completed == 0 {
		return nil
	}
	if s.World == nil {
		return fmt.Errorf("INV_DROPITEM: no world surface")
	}
	receiverID := s.Self.UID()
	o := s.World.AddObj(level, x, z, obj, completed, duration, receiverID)
	if o != nil {
		s.ActiveObj = o
		s.Pointers |= PtrActiveObj
	}
	return nil
}

// handleBothMoveInv ports TS InvOps.ts:373-495 (BOTH_MOVEINV, opcode 4301).
//
// Dispatch shape: state.intOperand selects primary (0) vs secondary (1).
// Primary:    from = active_player (Self), to = .active_player (Self2).
// Secondary:  pointers swap — from = Self2, to = Self.
//
// Pop order (TS popInts(2)): from on bottom, to on top → PopInt() returns
// to first.
//
// Protect gates per TS (slot-flipped on secondary):
//   - fromPlayer's slot must be Protected if fromInv.Protect && fromInv.Scope != Shared
//   - toPlayer's slot must be Protected if toInv.Protect && fromInv.Scope != Shared
//     (TS quirk preserved: to-gate gates on FROM scope, InvOps.ts:397)
//
// Drain loop: for each non-empty slot in fromInv, delete the slot, attempt
// to add the count to toInv at toPlayer; spill any overflow to toPlayer's
// tile via World.AddObj using TS InvOps.ts:423-432 stackable branching
// (per-unit loop for non-stackable / overflow==1, single stack for the
// stackable many-overflow case).
//
// DEVIATION-NAI-115-D1 (reuse): TS InvOps.ts:445-494 emits addWealthEvent
// for dueloffer/STAKE and trade/TRADE. Goscape skips inline emission;
// content can emit via OpWealthEvent (2131). Single-point retire when
// WealthEvent subsystem lands. NAI-115-D1.
func handleBothMoveInv(s *ScriptState) error {
	operand := s.Script.IntOperands[s.PC]
	if operand != 0 && operand != 1 {
		return fmt.Errorf("BOTH_MOVEINV: invalid intOperand %d", operand)
	}
	secondary := operand == 1

	if secondary {
		if err := requireActivePlayer2(s, "BOTH_MOVEINV"); err != nil {
			return err
		}
	} else {
		if err := requireActivePlayer(s, "BOTH_MOVEINV"); err != nil {
			return err
		}
	}

	to := s.PopInt()
	from := s.PopInt()

	if err := checkInvType(s, from, "BOTH_MOVEINV"); err != nil {
		return err
	}
	if err := checkInvType(s, to, "BOTH_MOVEINV"); err != nil {
		return err
	}

	fromInvType := s.Configs.InvType(from)
	toInvType := s.Configs.InvType(to)

	var fromPlayer, toPlayer ActivePlayer
	var fromProtectedFlag, toProtectedFlag Pointer
	if secondary {
		fromPlayer = s.Self2
		toPlayer = s.Self
		fromProtectedFlag = PtrProtectedActivePlayer2
		toProtectedFlag = PtrProtectedActivePlayer
		if toPlayer == nil || s.Pointers&PtrActivePlayer == 0 {
			return fmt.Errorf("BOTH_MOVEINV: no active player")
		}
	} else {
		fromPlayer = s.Self
		toPlayer = s.Self2
		fromProtectedFlag = PtrProtectedActivePlayer
		toProtectedFlag = PtrProtectedActivePlayer2
		if toPlayer == nil || s.Pointers&PtrActivePlayer2 == 0 {
			return fmt.Errorf("BOTH_MOVEINV: no active player2")
		}
	}

	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared &&
		s.Pointers&fromProtectedFlag == 0 {
		return fmt.Errorf("BOTH_MOVEINV: $from_inv requires protected access: %s", fromInvType.DebugName)
	}
	// TS quirk preserved (InvOps.ts:397): to-gate gates on FROM scope.
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared &&
		s.Pointers&toProtectedFlag == 0 {
		return fmt.Errorf("BOTH_MOVEINV: $to_inv requires protected access: %s", toInvType.DebugName)
	}

	if s.Inv == nil {
		return fmt.Errorf("BOTH_MOVEINV: no inv lookup")
	}
	fromInv := s.Inv.Get(fromPlayer, from)
	toInv := s.Inv.Get(toPlayer, to)
	if fromInv == nil || toInv == nil {
		return fmt.Errorf("BOTH_MOVEINV: inv is null")
	}

	for slot := 0; slot < fromInv.Capacity; slot++ {
		it := fromInv.Get(slot)
		if it == nil {
			continue
		}
		objID := it.Id
		count := it.Count

		objType := s.Configs.ObjType(objID)
		if objType == nil {
			return fmt.Errorf("BOTH_MOVEINV: invalid obj id at slot (id=%d)", objID)
		}

		fromInv.Delete(slot)

		stackable, stockObj := lookupStackableStockObj(s, toInv.Type, objID)
		tx := toInv.Add(objID, count, inventory.AddOpts{
			BeginSlot:           -1,
			AssureFullInsertion: false,
			Stackable:           stackable,
			StockObj:            stockObj,
		})
		overflow := count - tx.Completed
		if overflow > 0 && s.World != nil {
			level := (toPlayer.CoordPacked() >> 28) & 0x3
			x := toPlayer.X()
			z := toPlayer.Z()
			receiverID := toPlayer.UID()
			if !objType.Stackable || overflow == 1 {
				for range overflow {
					s.World.AddObj(level, x, z, objID, 1, 200, receiverID)
				}
			} else {
				s.World.AddObj(level, x, z, objID, overflow, 200, receiverID)
			}
		}
	}

	// NAI-115-D1 (reuse): TS InvOps.ts:445-494 emits addWealthEvent for
	// dueloffer/STAKE and trade/TRADE. Skipped — content emits via
	// OpWealthEvent (2131).

	return nil
}
