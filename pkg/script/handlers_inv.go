package script

import (
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/telemetry"
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

// selectProtectedActivePlayerSlot returns the slot-routed protect-flag
// for the current opcode's intOperand, mirroring TS's
// `ProtectedActivePlayer[state.intOperand]` (ScriptPointer.ts:39) — a
// 2-element array indexed by operand. operand=0 selects slot-0
// (PtrProtectedActivePlayer), operand=1 selects slot-1
// (PtrProtectedActivePlayer2). Out-of-range operands error per parity
// with handleInvDropItemDelayed (TS array-index `undefined` →
// pointerGet returns falsy → gate fires; goscape surfaces this as a
// hard error since the operand is opcode-author input and out-of-range
// is unreachable from valid bytecode).
//
// NAI-133 sibling: BOTH_MOVEINV and BOTH_DROPSLOT already route via
// intOperand. This helper unifies the per-handler inline gate in the
// 13 single-player INV-write opcodes (INV_ADD, INV_CHANGESLOT,
// INV_CLEAR, INV_DEL, INV_DELSLOT, INV_DROPITEM, INV_DROPSLOT,
// INV_MOVEFROMSLOT, INV_MOVEITEM, INV_MOVEITEM_CERT, INV_MOVEITEM_UNCERT,
// INV_MOVETOSLOT, INV_SETSLOT). Pre-fix all 13 hardcoded slot-0,
// silently dropping the operand=1 routing TS specifies at InvOps.ts:64,
// 91, 119, 136, 149, 172, 220, 329, 333, 359, 363, 507, 511, 543, 547,
// 578, 582, 607.
func selectProtectedActivePlayerSlot(s *ScriptState, op string) (Pointer, error) {
	operand := s.Script.IntOperands[s.PC]
	switch operand {
	case 0:
		return PtrProtectedActivePlayer, nil
	case 1:
		return PtrProtectedActivePlayer2, nil
	default:
		return 0, fmt.Errorf("%s: invalid intOperand %d", op, operand)
	}
}

// -- Reads --

// handleInvTotal (INV_TOTAL) pops [inv, obj] and pushes the total count
// of obj across all slots of inv. Matches TS popInts(2) order — obj on
// top, inv below.
func handleInvTotal(s *ScriptState) error {
	obj := s.PopInt()
	typeID := s.PopInt()
	if err := checkInvType(s, typeID, "INV_TOTAL"); err != nil {
		return err
	}
	// TS INV_TOTAL short-circuits with obj == -1 → push 0.
	if obj == -1 {
		s.PushInt(0)
		return nil
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
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
	if err := checkInvType(s, typeID, "INV_GETOBJ"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
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
	if err := checkInvType(s, typeID, "INV_GETNUM"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
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

// handleInvSize (INV_SIZE) pops an inv id and pushes its configured size.
//
// L21: TS INV_SIZE is a pure config read — state.pushInt(invType.size)
// (InvOps.ts:27-31) — it does NOT require a resolvable player inventory.
// Read the InvType.Size directly (== Inventory.Capacity, see FromType which
// builds New(t.ID, t.Size, ...)) rather than resolving a live inv instance,
// so the op works even when no inv is bound (s.Inv == nil). checkInvType
// guarantees the InvType is non-nil here.
func handleInvSize(s *ScriptState) error {
	typeID := s.PopInt()
	if err := checkInvType(s, typeID, "INV_SIZE"); err != nil {
		return err
	}
	s.PushInt(s.Configs.InvType(typeID).Size)
	return nil
}

// handleInvFreeSpace (INV_FREESPACE) pops an inv id and pushes the
// number of empty slots.
func handleInvFreeSpace(s *ScriptState) error {
	typeID := s.PopInt()
	if err := checkInvType(s, typeID, "INV_FREESPACE"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
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
	if err := checkInvType(s, typeID, "INV_ITEMSPACE"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_ITEMSPACE"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_ITEMSPACE"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
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
	if err := checkInvType(s, typeID, "INV_ITEMSPACE2"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_ITEMSPACE2"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_ITEMSPACE2"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
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
	if err := checkInvType(s, typeID, "INV_TOTALPARAM"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
		return fmt.Errorf("INV_TOTALPARAM: no inv for type %d", typeID)
	}
	if err := checkParamType(s, param, "INV_TOTALPARAM"); err != nil {
		return err
	}
	pt := s.Configs.ParamType(param)
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
// Mirrors TS InvOps.ts:634-640 — validates inv via InvTypeValid then
// category via CategoryTypeValid (partial in goscape, see
// checkCategoryType at handlers_npc.go:159).
func handleInvTotalCat(s *ScriptState) error {
	category := s.PopInt()
	typeID := s.PopInt()
	if err := checkInvType(s, typeID, "INV_TOTALCAT"); err != nil {
		return err
	}
	if err := checkCategoryType(category, "INV_TOTALCAT"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
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
// lookupStackable retained for sibling callers (handleInvMoveItem
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
	// TS InvOps.ts:73 passes assureFullInsertion=false explicitly.
	return performInvAdd(s, typeID, obj, count, false, "INV_ADD")
}

// performInvAdd is the shared body of the INV_ADD opcode. Mirrors TS
// InvOps.ts:57-83: validates invType + objType + count, enforces
// protect/scope + dummyitem gates, resolves the inv, routes via
// Inventory.Add, and drops overflow at the player's tile.
//
// Note: TS Player.invAdd (Player.ts:1496-1504) is bare — getInventory +
// container.add only — with `assureFullInsertion` defaulting to `true`
// at the entity layer. The INV_ADD opcode gates (Protect/Scope,
// dummyitem) live in TS's InvOps.ts:57-83 and call invAdd(..., false)
// explicitly. OBJ_TAKEITEM (ObjOps.ts:147) calls invAdd without the
// 4th arg, picking up the `true` default — a tight-destination Add
// either fully inserts or rolls back to nothing. goscape lacks a
// separate bare-invAdd entity path; both opcodes route through this
// helper and the caller picks the assureFullInsertion bit via the
// `assureFull` parameter. The InvType.Protect / DummyInv gates are
// no-ops for OBJ_TAKEITEM's realistic call shape (mindrune-style:
// non-protected inv 93, non-dummyitem obj), so routing through here
// doesn't change OBJ_TAKEITEM's hot path.
//
// Pre-conditions: caller has invoked requireActivePlayer (s.Self must
// be non-nil with PtrActivePlayer set; it is dereferenced for the
// overflow drop). Inputs are raw script ints; performInvAdd does its
// own check chain so each call site stays minimal.
func performInvAdd(s *ScriptState, typeID, obj, count int, assureFull bool, op string) error {
	// TS InvOps.ts:60-62 — InvTypeValid, ObjTypeValid, ObjStackValid.
	if err := checkInvType(s, typeID, op); err != nil {
		return err
	}
	if err := checkObjType(s, obj, op); err != nil {
		return err
	}
	if err := checkObjStack(count, op); err != nil {
		return err
	}

	invType := s.Configs.InvType(typeID)
	objType := s.Configs.ObjType(obj)

	// TS InvOps.ts:64-66 — protect/scope gate, slot-routed by intOperand
	// (NAI-133 sibling). OBJ_TAKEITEM also routes here; its invType has
	// Protect=false in realistic call shapes so the gate is a no-op
	// regardless of operand.
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, op)
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("%s: $inv requires protected access: %s", op, invType.DebugName)
		}
	}

	// TS InvOps.ts:68-70 — dummyitem-in-non-dummyinv gate.
	if !invType.DummyInv && objType.DummyItem != 0 {
		return fmt.Errorf("%s: dummyitem in non-dummyinv: %s -> %s", op, objType.DebugName, invType.DebugName)
	}

	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
		return fmt.Errorf("%s: no inv for type %d", op, typeID)
	}

	stackable := lookupStackable(s, obj)

	tx := inv.Add(obj, count, inventory.AddOpts{
		BeginSlot:           -1,
		AssureFullInsertion: assureFull,
		Stackable:           stackable,
	})

	overflow := count - tx.Completed
	if overflow > 0 && s.World != nil {
		level := (s.activePlayer().CoordPacked() >> 28) & 0x3
		x := s.activePlayer().X()
		z := s.activePlayer().Z()
		receiverID := s.activePlayer().UID()
		if !stackable || overflow == 1 {
			for range overflow {
				s.World.AddObj(level, x, z, obj, 1, 200, receiverID, s.activePlayer().AccountID())
			}
		} else {
			s.World.AddObj(level, x, z, obj, overflow, 200, receiverID, s.activePlayer().AccountID())
		}
	}

	return nil
}

// lookupStackable returns whether objID is stackable (ObjType.stackable),
// pre-computed from s.Configs for inventory.Add to consume. Returns false
// on nil-Configs / missing type (goscape defensive — see DEVIATION-NAI-130-D3).
//
// Stock-obj retention (TS `InvType.stockobj.includes(id)`) is NOT computed
// here: inventory.Add/Remove derive it from the inventory's own InvType,
// matching TS which computes it inside add()/remove() (Inventory.ts:160,245).
func lookupStackable(s *ScriptState, objID int) (stackable bool) {
	if s.Configs == nil {
		return false
	}
	if ot := s.Configs.ObjType(objID); ot != nil {
		stackable = ot.Stackable
	}
	return stackable
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
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_DEL")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_DEL: $inv requires protected access: %s", invType.DebugName)
		}
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
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_DELSLOT")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_DELSLOT: $inv requires protected access: %s", invType.DebugName)
		}
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

	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_SETSLOT")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_SETSLOT: $inv requires protected access: %s", invType.DebugName)
		}
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
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_CLEAR")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_CLEAR: $inv requires protected access: %s", invType.DebugName)
		}
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

	// TS InvOps.ts:507-509 — from-protect gate uses fromInv.scope, slot
	// routed by intOperand (NAI-133 sibling).
	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_MOVEITEM")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_MOVEITEM: $inv requires protected access: %s", fromInvType.DebugName)
		}
	}
	// TS InvOps.ts:511-513 — to-protect gate ALSO uses fromInv.scope (DEVIATION-NAI-131-D1).
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_MOVEITEM")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_MOVEITEM: $inv requires protected access: %s", toInvType.DebugName)
		}
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
	stackable := lookupStackable(s, obj)
	addTx := toInv.Add(obj, tx.Completed, inventory.AddOpts{
		BeginSlot: -1,
		Stackable: stackable,
	})
	// TS InvOps.ts:521-530 — items that don't fit in the destination drop to
	// the floor (DESPAWN, owned by the active player, 200t) rather than
	// vanishing. overflow is `count - added`, mirroring TS exactly.
	if overflow := count - addTx.Completed; overflow > 0 && s.World != nil {
		dropOverflowToFloor(s, obj, overflow, stackable)
	}
	return nil
}

// dropOverflowToFloor drops `overflow` units of obj at the active player's
// tile as DESPAWN objs owned by that player (200-tick lifetime). Non-stackable
// items (or a single unit) drop as N count-1 piles; stackables drop as one
// pile of `overflow`. Mirrors the TS World.addObj overflow branch shared by
// INV_MOVEITEM / INV_MOVEFROMSLOT / INV_ADD (InvOps.ts:521-530, 339-347).
func dropOverflowToFloor(s *ScriptState, obj, overflow int, stackable bool) {
	level := (s.activePlayer().CoordPacked() >> 28) & 0x3
	x := s.activePlayer().X()
	z := s.activePlayer().Z()
	receiverID := s.activePlayer().UID()
	accountID := s.activePlayer().AccountID()
	if !stackable || overflow == 1 {
		for range overflow {
			s.World.AddObj(level, x, z, obj, 1, 200, receiverID, accountID)
		}
	} else {
		s.World.AddObj(level, x, z, obj, overflow, 200, receiverID, accountID)
	}
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

	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_MOVEFROMSLOT")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_MOVEFROMSLOT: $inv requires protected access: %s", fromInvType.DebugName)
		}
	}
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_MOVEFROMSLOT")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_MOVEFROMSLOT: $inv requires protected access: %s", toInvType.DebugName)
		}
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
	stackable := lookupStackable(s, id)
	addTx := toInv.Add(id, cnt, inventory.AddOpts{
		BeginSlot: -1,
		Stackable: stackable,
	})
	// TS Player.invMoveFromSlot (Player.ts:1651) + InvOps.ts:339-347 — the
	// whole slot is removed; whatever the destination can't hold drops to the
	// floor (owned by the active player) instead of vanishing.
	if overflow := cnt - addTx.Completed; overflow > 0 && s.World != nil {
		dropOverflowToFloor(s, id, overflow, stackable)
	}
	return nil
}

// -- Listener registration (S6u) -----------------------------------------

// handleInvTransmit implements INV_TRANSMIT. Registers a listener on
// the active player for UI component `com` tracking the active
// player's own inventory of type `invType` (source = activePlayer.uid).
//
// TS: InvOps.ts INV_TRANSMIT — `const [inv, com] = state.popInts(2)`
// (popInts returns [bottom, top] — i.e. inv pushed FIRST, com pushed
// SECOND). Then check(inv, InvTypeValid); check(com, NumberNotNull);
// activePlayer.invListenOnCom(inv, com, activePlayer.uid). TS validates
// inv BEFORE com (InvOps.ts:647-648); mirror that order.
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
	if err := checkInvType(s, invType, "INV_TRANSMIT"); err != nil {
		return err
	}
	if err := checkNotNull(com, "INV_TRANSMIT"); err != nil {
		return err
	}
	s.activePlayer().InvListenOnCom(invType, com, s.activePlayer().UID())
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
	s.activePlayer().InvStopListenOnCom(com)
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
// Validation order mirrors TS (InvOps.ts:657-659): uid → inv → com.
// Closes S6u-SB1.
func handleInvOtherTransmit(s *ScriptState) error {
	if err := requireActivePlayer(s, "INVOTHER_TRANSMIT"); err != nil {
		return err
	}
	com := s.PopInt()
	invType := s.PopInt()
	uid := s.PopInt()
	if err := checkNotNull(uid, "INVOTHER_TRANSMIT"); err != nil {
		return err
	}
	if err := checkInvType(s, invType, "INVOTHER_TRANSMIT"); err != nil {
		return err
	}
	if err := checkNotNull(com, "INVOTHER_TRANSMIT"); err != nil {
		return err
	}
	s.activePlayer().InvListenOnCom(invType, com, uid)
	return nil
}

// handleInvDropSlot (INV_DROPSLOT, opcode 4312) drops the entire stack at
// slot from inv onto the ground at coord with private (caller-only)
// visibility for duration ticks. Mirrors TS InvOps.ts:213-260.
//
// Pop order: [inv, coord, slot, duration] — duration on top-of-stack.
//
// Validation order (mirrors TS): InvTypeValid → DurationValid → CoordValid
// → protect gate → slot lookup → empty check → addWealthEvent →
// invDel → completed check → AddObj per-item or stacked.
//
// Protect gate (conditional per InvType): when invType.Protect is true AND
// invType.Scope is not InvTypeScopeShared, require PtrProtectedActivePlayer via
// requireProtectedActivePlayer. Otherwise require only ActivePlayer.
// Gate is checked AFTER validators, matching TS opcode dispatch order.
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

	// InvTypeValid: registry-presence check via canonical validator.
	if err := checkInvType(s, invID, "INV_DROPSLOT"); err != nil {
		return err
	}
	invType := s.Configs.InvType(invID)

	// DurationValid.
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("INV_DROPSLOT: %w", err)
	}

	// CoordValid.
	level, x, z, err := checkCoord(coord, "INV_DROPSLOT")
	if err != nil {
		return err
	}

	// Protect gate: conditional on InvType protect + scope, slot-routed by
	// intOperand (NAI-133 sibling). The active-player gate always uses
	// slot-0 (TS InvOps.ts:213-260 references `state.activePlayer`
	// throughout, never `_activePlayer2`); only the protect-flag check
	// swaps between PtrProtectedActivePlayer (operand=0) and
	// PtrProtectedActivePlayer2 (operand=1). Inline shape matches the
	// other 12 INV-write opcodes (NAI-133 unification).
	if err := requireActivePlayer(s, "INV_DROPSLOT"); err != nil {
		return err
	}
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_DROPSLOT")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_DROPSLOT: $inv requires protected access: %s", invType.DebugName)
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

	// TS-faithful per InvOps.ts:231-238: emit addWealthEvent for SCOPE_PERM
	// drop (ammo drops are temp — avoid spamming ranged combat).
	// NAI-115-D1 retired at NAI-162 B2.
	if invType.Scope == objtype.InvTypeScopePerm {
		s.activePlayer().AddWealthEvent(WealthEvent{
			EventType:    WealthEventTypeDrop,
			AccountItems: []WealthItem{{ID: objID, Name: objType.DebugName, Count: count}},
			AccountValue: count * objType.Cost,
		})
		if s.NodeDebug && s.Log != nil {
			s.Log.Info("nai162.wealth.invdropslot",
				"event_type", WealthEventTypeDrop,
				"value", count*objType.Cost,
				"inv", invID,
				"count", count,
			)
		}
	}

	// Slot-scoped removal mirroring TS player.invDel(invType.id, obj.id,
	// obj.count, slot) → Inventory.remove(obj, count, beginSlot). Routing
	// through Remove (not a direct Delete) preserves the stock-obj placeholder
	// retention TS's remove() applies (Inventory.ts:280) — TS-true for stock
	// invs; for the common non-stock inv it vacates the slot identically to a
	// direct delete. completed = units actually removed.
	completed := inv.Remove(objID, count, inventory.RemoveOpts{BeginSlot: slot}).Completed
	if completed == 0 {
		return nil
	}

	// Caller-only (private) drop: receiverID = active player's UID.
	receiverID := s.activePlayer().UID()

	// Stackable branch: mirrors TS InvOps.ts:248-258.
	// state.activeObj set after each spawn; last wins for non-stackable count=N.
	// L20: operand-aware activeObj writeback via setActiveObjSlot, mirroring
	// TS `state.activeObj = floorObj; state.pointerAdd(ActiveObj[state.intOperand])`
	// (InvOps.ts:250-251/257-258). IntOperand 1 (.obj2) routes to OtherActiveObj.
	if !objType.Stackable || completed == 1 {
		for range completed {
			obj := s.World.AddObj(level, x, z, objID, 1, duration, receiverID, s.activePlayer().AccountID())
			if obj != nil {
				setActiveObjSlot(s, obj)
			}
		}
	} else {
		obj := s.World.AddObj(level, x, z, objID, completed, duration, receiverID, s.activePlayer().AccountID())
		if obj != nil {
			setActiveObjSlot(s, obj)
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

	// TS InvOps.ts:359-361 — from-protect gate uses fromInvType.Scope,
	// slot-routed by intOperand (NAI-133 sibling).
	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_MOVETOSLOT")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_MOVETOSLOT: $inv requires protected access: %s", fromInvType.DebugName)
		}
	}
	// TS InvOps.ts:363-365 — to-protect gate ALSO uses fromInvType.Scope (DEVIATION-NAI-131-D1).
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_MOVETOSLOT")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_MOVETOSLOT: $inv requires protected access: %s", toInvType.DebugName)
		}
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
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_CHANGESLOT")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_CHANGESLOT: $inv requires protected access: %s", invType.DebugName)
		}
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

	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_MOVEITEM_CERT")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_MOVEITEM_CERT: $inv requires protected access: %s", fromInvType.DebugName)
		}
	}
	// DEVIATION-NAI-131-D1: to-gate uses fromInvType.Scope (mirrors TS; goscape defensive label).
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_MOVEITEM_CERT")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_MOVEITEM_CERT: $inv requires protected access: %s", toInvType.DebugName)
		}
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
	stackable := lookupStackable(s, finalObj)
	tx2 := toInv.Add(finalObj, tx.Completed, inventory.AddOpts{
		BeginSlot: -1,
		Stackable: stackable,
	})

	overflow := count - tx2.Completed
	// DEVIATION-NAI-130-D2: defensive nil-World guard (goscape defensive; TS skips this check).
	if overflow > 0 && s.World != nil {
		level := (s.activePlayer().CoordPacked() >> 28) & 0x3
		receiverID := s.activePlayer().UID()
		// TS comment: "should be a stackable cert already" → single stacked drop.
		s.World.AddObj(level, s.activePlayer().X(), s.activePlayer().Z(), finalObj, overflow, 200, receiverID, s.activePlayer().AccountID())
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

	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_MOVEITEM_UNCERT")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_MOVEITEM_UNCERT: $inv requires protected access: %s", fromInvType.DebugName)
		}
	}
	// DEVIATION-NAI-131-D1: to-gate uses fromInvType.Scope.
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_MOVEITEM_UNCERT")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_MOVEITEM_UNCERT: $inv requires protected access: %s", toInvType.DebugName)
		}
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
	stackable := lookupStackable(s, finalObj)
	// TS InvOps.ts:592-596 calls player.invAdd(...) with no 4th arg, so
	// assureFullInsertion defaults to true (Player.ts:1496) — all-or-nothing
	// on the destination. Without this, a near-full toInv silently partial-
	// fills and the overflow is lost.
	toInv.Add(finalObj, tx.Completed, inventory.AddOpts{
		BeginSlot:           -1,
		Stackable:           stackable,
		AssureFullInsertion: true,
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
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_DROPITEM")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_DROPITEM: $inv requires protected access: %s", invType.DebugName)
		}
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
	receiverID := s.activePlayer().UID()
	o := s.World.AddObj(level, x, z, obj, completed, duration, receiverID, s.activePlayer().AccountID())
	if o != nil {
		// L20: operand-aware writeback (TS InvOps.ts:184-185
		// state.activeObj = floorObj; pointerAdd(ActiveObj[intOperand])).
		setActiveObjSlot(s, o)
	}

	// NAI-Phase2: emit ItemDroppedEvent — only fires when both inventory
	// removal completed (tx.Completed > 0 above) AND the world AddObj
	// returned a non-nil obj.
	//
	// world_id comes from WorldVars.NodeID() (mirrors emission sites in
	// modules/world/ that read cfg.NodeID directly).
	//
	// alch_value / market_value are goscape-extension fields (no TS
	// upstream — see proto/events/v1/wealth.proto and the alerting
	// semantics in the wealth-event schema notes
	// "Excessive Wealth Transfers" / "High Value Item Drops"):
	//   - alch_value  = high-alch = cost * 6 / 10 (standard RS formula).
	//   - market_value = cost when the item is tradeable, else 0 ("if it
	//     exists for the item", per the design doc — this engine has no
	//     live market, so we use the configured shop price as the
	//     tradeable-item proxy).
	// ObjType is guaranteed non-nil here by the earlier checkObjType
	// validator.
	var worldID int32
	if s.World != nil {
		worldID = int32(s.World.NodeID())
	}
	objType := s.Configs.ObjType(obj)
	alchValue := int64(objType.Cost) * 6 / 10
	var marketValue int64
	if objType.Tradeable {
		marketValue = int64(objType.Cost)
	}
	if o != nil {
		telemetry.Get().EmitWealth(&eventspb.WealthEnvelope{
			SchemaVersion: 1,
			EventId:       uuid.NewString(),
			Ts:            timestamppb.Now(),
			AccountId:     s.activePlayer().AccountID(),
			WorldId:       worldID,
			Payload: &eventspb.WealthEnvelope_ItemDropped{
				ItemDropped: &eventspb.ItemDroppedEvent{
					ItemId:      int32(obj),
					Qty:         int32(completed),
					X:           int32(x),
					Y:           int32(z),
					Plane:       int32(level),
					AlchValue:   alchValue,
					MarketValue: marketValue,
				},
			},
		})
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
// stackable many-overflow case). Wealth events are emitted per TS
// InvOps.ts:445-494: STAKE for 'dueloffer', TRADE for non-secondary trade.
// NAI-115-D1 retired at NAI-162 B2.
//
// Deviation: RecipientSession is empty (goscape lacks a Session() method
// on ActivePlayer; analytics RPC deferred per NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY).
func handleBothMoveInv(s *ScriptState) error {
	operand := s.Script.IntOperands[s.PC]
	if operand != 0 && operand != 1 {
		return fmt.Errorf("BOTH_MOVEINV: invalid intOperand %d", operand)
	}
	secondary := operand == 1

	// TS InvOps.ts:373 wraps in checkedHandler(ActivePlayer) unconditionally
	// regardless of operand; the secondary player is validated by a runtime
	// null-check (line 389) after slot selection, not by a wrapper that
	// asserts the secondary binding.
	if err := requireActivePlayer(s, "BOTH_MOVEINV"); err != nil {
		return err
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
	} else {
		fromPlayer = s.Self
		toPlayer = s.Self2
		fromProtectedFlag = PtrProtectedActivePlayer
		toProtectedFlag = PtrProtectedActivePlayer2
	}
	// TS InvOps.ts:389 — null-check after slot selection. Catches the case
	// where secondary mode references Self2 but PtrActivePlayer2 is unset.
	if fromPlayer == nil || toPlayer == nil {
		return fmt.Errorf("BOTH_MOVEINV: player is null")
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

	// fromLogs accumulates per-obj item counts for wealth event emission.
	// Mirrors TS InvOps.ts:410-413 fromLogs Map<id, WealthEventItem&{cost}>.
	type logEntry struct {
		id    int
		name  string
		count int
		cost  int
	}
	var fromLogs []logEntry         // ordered; we merge by objID below
	fromLogIdx := make(map[int]int) // objID → index in fromLogs
	fromTotal := 0

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

		stackable := lookupStackable(s, objID)
		tx := toInv.Add(objID, count, inventory.AddOpts{
			BeginSlot:           -1,
			AssureFullInsertion: false,
			Stackable:           stackable,
		})
		overflow := count - tx.Completed
		if overflow > 0 && s.World != nil {
			level := (toPlayer.CoordPacked() >> 28) & 0x3
			x := toPlayer.X()
			z := toPlayer.Z()
			receiverID := toPlayer.UID()
			if !objType.Stackable || overflow == 1 {
				for range overflow {
					s.World.AddObj(level, x, z, objID, 1, 200, receiverID, s.activePlayer().AccountID())
				}
			} else {
				s.World.AddObj(level, x, z, objID, overflow, 200, receiverID, s.activePlayer().AccountID())
			}
		}

		// Accumulate log entry (mirrors TS InvOps.ts:435-443).
		if idx, ok := fromLogIdx[objID]; ok {
			fromLogs[idx].count += count
		} else {
			fromLogIdx[objID] = len(fromLogs)
			fromLogs = append(fromLogs, logEntry{id: objID, name: objType.DebugName, count: count, cost: objType.Cost})
		}
		fromTotal += objType.Cost * count
	}

	// Emit wealth events per TS InvOps.ts:445-494.
	// TS-faithful per InvOps.ts:445-494: STAKE for 'dueloffer', TRADE for
	// non-secondary trade. NAI-115-D1 retired at NAI-162 B2.
	//
	// STAKE gate: TS InvOps.ts:449 `if (fromItems.length > 0)` — only emit when
	// the staker actually moved items.
	//
	// TRADE gate: TS InvOps.ts:483 `if (fromItems.length > 0 || toLogs.size > 0)`
	// — emit when EITHER side has items. The toItems scan must happen unconditionally
	// (outside the fromLogs>0 guard) so the case "fromInv empty, toInv has items"
	// still fires the event. See NAI-162 B2.6.fixup.
	fromItems := make([]WealthItem, len(fromLogs))
	for i, e := range fromLogs {
		fromItems[i] = WealthItem{ID: e.id, Name: e.name, Count: e.count}
	}

	if fromInvType.DebugName == "dueloffer" {
		// STAKE event (mirrors TS InvOps.ts:447-453).
		if len(fromLogs) > 0 {
			fromPlayer.AddWealthEvent(WealthEvent{
				EventType:    WealthEventTypeStake,
				AccountItems: fromItems,
				AccountValue: fromTotal,
				// RecipientSession: toPlayer.Session() — deferred (NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY).
			})
			if s.NodeDebug && s.Log != nil {
				s.Log.Info("nai162.wealth.bothmoveinv_stake",
					"event_type", WealthEventTypeStake,
					"value", fromTotal,
					"inv", from,
					"count", len(fromItems),
				)
			}
		}
	} else if !secondary {
		// TRADE event (mirrors TS InvOps.ts:455-492 non-secondary branch).
		// Read the to-player's matching inventory to build recipient side.
		// This scan is unconditional so toLogs.size>0 can trigger emission.
		toTotal := 0
		var toItems []WealthItem
		if tradeInv := s.Inv.Get(toPlayer, from); tradeInv != nil {
			toLogIdx := make(map[int]int)
			for slot := 0; slot < tradeInv.Capacity; slot++ {
				it := tradeInv.Get(slot)
				if it == nil {
					continue
				}
				toObjType := s.Configs.ObjType(it.Id)
				if toObjType == nil {
					continue
				}
				if idx, ok := toLogIdx[it.Id]; ok {
					toItems[idx].Count += it.Count
				} else {
					toLogIdx[it.Id] = len(toItems)
					toItems = append(toItems, WealthItem{ID: it.Id, Name: toObjType.DebugName, Count: it.Count})
				}
				toTotal += toObjType.Cost * it.Count
			}
		}
		// TS InvOps.ts:483: emit when fromItems OR toItems non-empty.
		if len(fromLogs) > 0 || len(toItems) > 0 {
			fromPlayer.AddWealthEvent(WealthEvent{
				EventType:    WealthEventTypeTrade,
				AccountItems: fromItems,
				AccountValue: fromTotal,
				// RecipientSession: toPlayer.Session() — deferred (NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY).
			})
			var tradeWorldID int32
			if s.World != nil {
				tradeWorldID = int32(s.World.NodeID())
			}
			telemetry.Get().EmitWealth(&eventspb.WealthEnvelope{
				SchemaVersion: 1,
				EventId:       uuid.NewString(),
				Ts:            timestamppb.Now(),
				AccountId:     s.activePlayer().AccountID(),
				WorldId:       tradeWorldID,
				Payload: &eventspb.WealthEnvelope_TradeCompleted{
					TradeCompleted: &eventspb.TradeCompletedEvent{
						PartnerAccountId: toPlayer.AccountID(),
						ItemsGiven:       wealthItemsToStacks(fromItems),
						ItemsReceived:    wealthItemsToStacks(toItems),
						ValueGiven:       int64(fromTotal),
						ValueReceived:    int64(toTotal),
					},
				},
			})
			if s.NodeDebug && s.Log != nil {
				s.Log.Info("nai162.wealth.bothmoveinv_trade",
					"event_type", WealthEventTypeTrade,
					"value", fromTotal,
					"inv", from,
					"count", len(fromItems),
				)
			}
		}
	}

	return nil
}

// wealthItemsToStacks converts a []WealthItem slice (script-internal
// representation) to the []*eventspb.ItemStack wire type used in
// TradeCompletedEvent fields. Used by handleBothMoveInv.
func wealthItemsToStacks(items []WealthItem) []*eventspb.ItemStack {
	out := make([]*eventspb.ItemStack, 0, len(items))
	for _, it := range items {
		out = append(out, &eventspb.ItemStack{ItemId: int32(it.ID), Qty: int32(it.Count)})
	}
	return out
}

// handleInvDropItemDelayed (INV_DROPITEM_DELAYED, opcode 4310) ports
// TS InvOps.ts:188-209. Pops [inv, coord, obj, count, duration, delay].
// Removes count of obj from inv; if completed > 0, enqueues an
// ObjDelayedRequest onto World.objDelayedQueue (drained per-tick by
// Server.processObjDelayedQueue at modules/world/obj_delayed_queue.go).
//
// Validator chain (mirrors handleInvDropItem at handlers_inv.go:1142):
// InvTypeValid → CoordValid → ObjTypeValid → ObjStackValid → DurationValid
// → operand-aware protect-gate.
//
// `delay` is unvalidated — TS InvOps.ts:188-195 lacks DelayValid.
//
// Operand-aware protect gate (NAI-133 slot routing): operand=0 selects
// PtrProtectedActivePlayer; operand=1 selects PtrProtectedActivePlayer2.
// Out-of-range operand returns an error.
//
// TS-asymmetry vs INV_DROPITEM (handlers_inv.go:1142-1203): does NOT set
// state.ActiveObj or PtrActiveObj — the obj does not yet exist as a
// tracked world entity at enqueue time. TS verbatim at InvOps.ts:206-208.
//
// DEVIATION-NAI-130-D2 sibling: defensive nil-World guard returns clean
// error rather than nil-deref, matching handleInvDropItem.
func handleInvDropItemDelayed(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_DROPITEM_DELAYED"); err != nil {
		return err
	}
	delay := s.PopInt()
	duration := s.PopInt()
	count := s.PopInt()
	obj := s.PopInt()
	coord := s.PopInt()
	invID := s.PopInt()

	if err := checkInvType(s, invID, "INV_DROPITEM_DELAYED"); err != nil {
		return err
	}
	level, x, z, err := checkCoord(coord, "INV_DROPITEM_DELAYED")
	if err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_DROPITEM_DELAYED"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_DROPITEM_DELAYED"); err != nil {
		return err
	}
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("INV_DROPITEM_DELAYED: %w", err)
	}

	// Operand-aware protect gate (NAI-133 slot routing).
	operand := s.Script.IntOperands[s.PC]
	if operand != 0 && operand != 1 {
		return fmt.Errorf("INV_DROPITEM_DELAYED: invalid intOperand %d", operand)
	}
	protectFlag := PtrProtectedActivePlayer
	if operand == 1 {
		protectFlag = PtrProtectedActivePlayer2
	}
	invType := s.Configs.InvType(invID)
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && s.Pointers&protectFlag == 0 {
		return fmt.Errorf("INV_DROPITEM_DELAYED: $inv requires protected access: %s", invType.DebugName)
	}

	inv := resolveInv(s, invID)
	if inv == nil {
		return fmt.Errorf("INV_DROPITEM_DELAYED: inv unresolved (id=%d)", invID)
	}
	tx := inv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
	completed := tx.Completed
	if completed == 0 {
		return nil
	}
	if s.World == nil {
		return fmt.Errorf("INV_DROPITEM_DELAYED: no world surface")
	}
	s.World.EnqueueObjDelayed(level, x, z, obj, completed, duration, delay, s.activePlayer().UID(), s.activePlayer().AccountID())
	return nil
}

// handleInvStockBase (INV_STOCKBASE, opcode 4325) returns the configured
// stock count for an object in an inventory's stock list, or -1 if the
// inventory has no stock or the object is not in the stock list.
// Pop order: obj on top (popped first), inv below (popped second) —
// matches the goscape INV_* convention (e.g. handleInvTotal).
// Mirrors TS LostCityRS/Engine-TS/.../InvOps.ts:41-54.
func handleInvStockBase(s *ScriptState) error {
	obj := s.PopInt()
	inv := s.PopInt()
	if err := checkInvType(s, inv, "INV_STOCKBASE"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_STOCKBASE"); err != nil {
		return err
	}
	invType := s.Configs.InvType(inv)
	objType := s.Configs.ObjType(obj)
	if len(invType.StockObj) == 0 || len(invType.StockCount) == 0 {
		s.PushInt(-1)
		return nil
	}
	idx := -1
	for i, id := range invType.StockObj {
		if int(id) == objType.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.PushInt(-1)
		return nil
	}
	s.PushInt(int(invType.StockCount[idx]))
	return nil
}

// handleInvAllStock implements OpInvAllStock (TS INV_ALLSTOCK at
// InvOps.ts:20-24). Pops a typeID, validates via checkInvType, pushes 1
// if InvType.AllStock else 0. NAI-160 T5.
func handleInvAllStock(s *ScriptState) error {
	typeID := s.PopInt()
	if err := checkInvType(s, typeID, "INV_ALLSTOCK"); err != nil {
		return err
	}
	if s.Configs.InvType(typeID).AllStock {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}

// handleInvDebugName (INV_DEBUG_NAME) pushes the debug name of an
// InvType, or "null" if the field is empty. Mirrors TS
// LostCityRS/Engine-TS/.../InvOps.ts:34-38:
//
//	const invType = check(state.popInt(), InvTypeValid)
//	state.pushString(invType.debugname ?? 'null')
func handleInvDebugName(s *ScriptState) error {
	inv := s.PopInt()
	if err := checkInvType(s, inv, "INV_DEBUG_NAME"); err != nil {
		return err
	}
	invType := s.Configs.InvType(inv)
	if invType.DebugName == "" {
		s.PushString("null")
	} else {
		s.PushString(invType.DebugName)
	}
	return nil
}

// handleBothDropSlot (BOTH_DROPSLOT, opcode 4300). Drops the entire stack
// at slot from fromPlayer's inv at coord with private visibility for
// duration ticks. fromPlayer/toPlayer swap is driven by the intOperand:
// 0 → fromPlayer=Self, toPlayer=Self2 (primary / "both_dropslot");
// 1 → fromPlayer=Self2, toPlayer=Self (secondary / ".both_dropslot").
//
// Pop order (LIFO): duration, slot, coord, invID
// (TS popInts(4) → [inv, coord, slot, duration]; duration on top).
//
// Protect gate: when invType.Protect && scope != SCOPE_SHARED, requires
// PtrProtectedActivePlayer (primary) or PtrProtectedActivePlayer2
// (secondary). Gate checked after validators, before slot lookup.
//
// SCOPE_PERM drops emit a PVP WealthEvent on state.activePlayer (Self)
// with RecipientSession="" per NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY
// (Session() not exposed through the ActivePlayer interface; deferred).
//
// Untradeable objs stay with fromPlayer (receiverID = fromPlayer.UID());
// tradeable objs go to toPlayer (receiverID = toPlayer.UID()).
// Mirrors TS InvOps.ts:672-723.
func handleBothDropSlot(s *ScriptState) error {
	if err := requireActivePlayer(s, "BOTH_DROPSLOT"); err != nil {
		return err
	}
	if s.Configs == nil {
		return fmt.Errorf("BOTH_DROPSLOT: no configs")
	}
	if s.World == nil {
		return fmt.Errorf("BOTH_DROPSLOT: no world surface")
	}

	duration := s.PopInt()
	slot := s.PopInt()
	coord := s.PopInt()
	invID := s.PopInt()

	// InvTypeValid: registry-presence check via canonical validator.
	if err := checkInvType(s, invID, "BOTH_DROPSLOT"); err != nil {
		return err
	}
	invType := s.Configs.InvType(invID)

	// DurationValid.
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("BOTH_DROPSLOT: %w", err)
	}

	// CoordValid.
	level, x, z, err := checkCoord(coord, "BOTH_DROPSLOT")
	if err != nil {
		return err
	}

	// secondary == 1 → fromPlayer = Self2, toPlayer = Self.
	secondary := s.Script.IntOperands[s.PC] == 1

	var fromPlayer, toPlayer ActivePlayer
	if secondary {
		fromPlayer = s.Self2
		toPlayer = s.Self
	} else {
		fromPlayer = s.Self
		toPlayer = s.Self2
	}
	if fromPlayer == nil || toPlayer == nil {
		return fmt.Errorf("BOTH_DROPSLOT: player is null")
	}

	// Protect gate: conditional on invType.Protect + scope, slot-routed by
	// intOperand (TS InvOps.ts:692 — `ProtectedActivePlayer[secondary ? 1 : 0]`).
	// fromPlayer/toPlayer non-nil already checked above; only the protect-flag
	// pointer varies between slot-0 and slot-1 (active-player binding is not
	// asserted by this gate).
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "BOTH_DROPSLOT")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("BOTH_DROPSLOT: inv requires protected access: %s", invType.DebugName)
		}
	}

	if s.Inv == nil {
		return fmt.Errorf("BOTH_DROPSLOT: no inv lookup")
	}

	// Resolve fromPlayer's inventory.
	inv := s.Inv.Get(fromPlayer, invID)
	if inv == nil {
		return fmt.Errorf("BOTH_DROPSLOT: fromPlayer inv missing")
	}

	it := inv.Get(slot)
	if it == nil {
		return fmt.Errorf("BOTH_DROPSLOT: $slot is empty (slot=%d)", slot)
	}

	objID := it.Id
	count := it.Count

	// Validate obj config.
	objType := s.Configs.ObjType(objID)
	if objType == nil {
		return fmt.Errorf("BOTH_DROPSLOT: invalid obj id at slot (id=%d)", objID)
	}

	// SCOPE_PERM → PVP WealthEvent on state.activePlayer (Self).
	// RecipientSession="" per NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY:
	// toPlayer.Session() is not exposed through the ActivePlayer interface.
	// (goscape adaptation; TS has toPlayer.session field access here.)
	if invType.Scope == objtype.InvTypeScopePerm {
		s.activePlayer().AddWealthEvent(WealthEvent{
			EventType:        WealthEventTypePVP,
			AccountItems:     []WealthItem{{ID: objID, Name: objType.DebugName, Count: count}},
			AccountValue:     count * objType.Cost,
			RecipientSession: "", // deferred per NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY
		})
		if s.NodeDebug && s.Log != nil {
			s.Log.Info("nai162.wealth.bothdropslot",
				"event_type", WealthEventTypePVP,
				"value", count*objType.Cost,
				"inv", invID,
				"count", count,
				"secondary", secondary,
				"obj", objID,
			)
		}
	}

	// Slot-scoped removal mirroring TS fromPlayer.invDel(invType.id, obj.id,
	// obj.count, slot) → Inventory.remove(obj, count, beginSlot). Routing
	// through Remove (not a direct Delete) keeps stock-obj retention TS-true;
	// inert for the PvP/trade player invs this opcode runs on (they hold no
	// stock objs) but faithful to TS regardless. completed = units removed.
	completed := inv.Remove(objID, count, inventory.RemoveOpts{BeginSlot: slot}).Completed
	if completed == 0 {
		return nil
	}

	// Untradeable → fromPlayer; tradeable → toPlayer. Mirrors TS
	// InvOps.ts:717-721: `!objType.tradeable ? fromPlayer.hash64 : toPlayer.hash64`.
	var receiverID int
	if !objType.Tradeable {
		receiverID = fromPlayer.UID()
	} else {
		receiverID = toPlayer.UID()
	}

	s.World.AddObj(level, x, z, objID, completed, duration, receiverID, s.activePlayer().AccountID())
	return nil
}

// handleInvDropAll (INV_DROPALL, opcode 4309). Walks every slot of the
// named inv, dropping each obj to the world. SCOPE_PERM accumulates a
// per-objID wealth log keyed by objID with running count; after the
// loop, if any items were seen, emits a single Death-type WealthEvent
// with aggregated items and total value. Mirrors TS InvOps.ts:726-790.
//
// Pop order (LIFO): duration, coord, invID
// (TS popInts(3) → [inv, coord, duration]; duration on top).
//
// Protect gate: when invType.Protect && scope != SCOPE_SHARED, requires
// PtrProtectedActivePlayer (intOperand=0) or PtrProtectedActivePlayer2
// (intOperand=1). Gate checked after validators, before slot walk.
//
// Per-slot receiver: untradeable → self.UID() (private); tradeable →
// -1 (PublicReceiver / Obj.NO_RECEIVER). Mirrors TS InvOps.ts:773-778:
// `!objType.tradeable ? state.activePlayer.hash64 : Obj.NO_RECEIVER`.
func handleInvDropAll(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_DROPALL"); err != nil {
		return err
	}
	if s.Configs == nil {
		return fmt.Errorf("INV_DROPALL: no configs")
	}
	if s.World == nil {
		return fmt.Errorf("INV_DROPALL: no world surface")
	}

	duration := s.PopInt()
	coord := s.PopInt()
	invID := s.PopInt()

	// InvTypeValid: registry-presence check via canonical validator.
	if err := checkInvType(s, invID, "INV_DROPALL"); err != nil {
		return err
	}
	invType := s.Configs.InvType(invID)

	// DurationValid.
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("INV_DROPALL: %w", err)
	}

	// CoordValid.
	level, x, z, err := checkCoord(coord, "INV_DROPALL")
	if err != nil {
		return err
	}

	// Protect gate: ProtectedActivePlayer[intOperand] — only the protect-flag
	// pointer is operand-routed. The handler operates entirely on s.Self
	// (slot-0) regardless of operand; TS does not assert _activePlayer2.
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
		protectFlag, err := selectProtectedActivePlayerSlot(s, "INV_DROPALL")
		if err != nil {
			return err
		}
		if s.Pointers&protectFlag == 0 {
			return fmt.Errorf("INV_DROPALL: $inv requires protected access: %s", invType.DebugName)
		}
	}

	if s.Inv == nil {
		return nil
	}
	inv := s.Inv.Get(s.Self, invID)
	if inv == nil {
		return nil
	}

	// wealthLog accumulates per-objID counts for SCOPE_PERM death event.
	// Using a pointer-to-struct map entry to avoid the value-copy gotcha
	// (R8 per vararg_opcode_shapes_dont_share_with_fixed_arg_siblings.md).
	type wealthEntry struct {
		id    int
		name  string
		count int
		cost  int
	}
	var wealthLog map[int]*wealthEntry
	totalValue := 0

	for slot := 0; slot < inv.Capacity; slot++ {
		it := inv.Get(slot)
		if it == nil {
			continue
		}

		objID := it.Id
		count := it.Count

		objType := s.Configs.ObjType(objID)
		cost := 0
		debugName := ""
		tradeable := false
		if objType != nil {
			cost = objType.Cost
			debugName = objType.DebugName
			tradeable = objType.Tradeable
		}

		// Accumulate wealth log for SCOPE_PERM (death-drop scenario).
		if invType.Scope == objtype.InvTypeScopePerm {
			if wealthLog == nil {
				wealthLog = make(map[int]*wealthEntry)
			}
			if e := wealthLog[objID]; e != nil {
				e.count += count
			} else {
				wealthLog[objID] = &wealthEntry{id: objID, name: debugName, count: count, cost: cost}
			}
			totalValue += count * cost
		}

		inv.Delete(slot)

		// Untradeable stays with the player (private); tradeable goes to
		// PublicReceiver (-1). Mirrors TS InvOps.ts:773-778.
		var receiverID int
		if !tradeable {
			receiverID = s.activePlayer().UID()
		} else {
			receiverID = -1 // Obj.NO_RECEIVER / PublicReceiver
		}

		s.World.AddObj(level, x, z, objID, count, duration, receiverID, s.activePlayer().AccountID())
	}

	// Post-loop: emit single Death event if anything was accumulated.
	if len(wealthLog) > 0 {
		items := make([]WealthItem, 0, len(wealthLog))
		for _, e := range wealthLog {
			items = append(items, WealthItem{ID: e.id, Name: e.name, Count: e.count})
		}
		s.activePlayer().AddWealthEvent(WealthEvent{
			EventType:    WealthEventTypeDeath,
			AccountItems: items,
			AccountValue: totalValue,
		})
		if s.NodeDebug && s.Log != nil {
			s.Log.Info("nai162.wealth.invdropall",
				"event_type", WealthEventTypeDeath,
				"value", totalValue,
				"items", len(items),
			)
		}
	}
	return nil
}

// handleInvTotalParamStack (INV_TOTALPARAM_STACK, opcode 4329). Pops
// param then inv (LIFO; TS popInts(2) → [inv, param] means param is on
// top). Delegates to Self.InvTotalParamStack and pushes the result.
// Mirrors TS InvOps.ts:792-796.
func handleInvTotalParamStack(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_TOTALPARAM_STACK"); err != nil {
		return err
	}
	param := s.PopInt()
	inv := s.PopInt()
	s.PushInt(s.activePlayer().InvTotalParamStack(inv, param))
	return nil
}
