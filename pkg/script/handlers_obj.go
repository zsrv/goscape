// Package script — handlers for the Obj family of script opcodes.
package script

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/inventory"
)

// requireActiveObj returns an error if s.ActiveObj is nil.
func requireActiveObj(s *ScriptState, op string) error {
	if s.ActiveObj == nil {
		return fmt.Errorf("%s: no active obj", op)
	}
	return nil
}

// checkObjType validates an ObjType id is registered in s.Configs.
// Mirrors TS check(id, ObjTypeValid) (ScriptValidators.ts).
func checkObjType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.ObjType(id) == nil {
		return fmt.Errorf("%s: no ObjType with value (%d) found", op, id)
	}
	return nil
}

// checkObjStack mirrors TS ObjStackValid (ScriptValidators.ts:121) — a
// ScriptInputRangeValidator over [1, Inventory.STACK_LIMIT=0x7fffffff].
// Rejects 0, negatives, and counts above StackLimit.
func checkObjStack(c int, op string) error {
	if c < 1 || c > inventory.StackLimit {
		return fmt.Errorf("%s: invalid count (%d)", op, c)
	}
	return nil
}

// objAddCommon is the shared body of OBJ_ADD and OBJ_ADDALL. Differs
// only in receiverID: OBJ_ADD passes the active player's UID for a
// caller-only private drop; OBJ_ADDALL passes zone.PublicReceiver (-1)
// for broadcast.
//
// Mirrors TS ObjOps.ts:20-92 (both opcodes share the validation chain
// + stackable branch). Pop order matches popInts(4): top-of-stack is
// duration, then count, then objId, then coord at the bottom.
//
// Mirrors TS ObjOps.ts:20-92 (both opcodes share the validation chain
// + stackable branch + state.activeObj writeback + pointerAdd(ActiveObj)).
// (NAI-115-D3 retired in-bundle stretch: AddObj now returns ActiveObj.)
func objAddCommon(s *ScriptState, op string, receiverID int) error {
	duration := s.PopInt()
	count := s.PopInt()
	objId := s.PopInt()
	coord := s.PopInt()

	if objId == -1 || count == -1 {
		return nil
	}
	if err := checkObjType(s, objId, op); err != nil {
		return err
	}
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	level, x, z, err := checkCoord(coord, op)
	if err != nil {
		return err
	}
	if err := checkObjStack(count, op); err != nil {
		return err
	}

	objType := s.Configs.ObjType(objId)
	if objType.DummyItem != 0 {
		return fmt.Errorf("%s: attempted to add dummy item: id=%d", op, objId)
	}
	if objType.Members && s.World.MapMembers() == 0 {
		return nil
	}

	// Mirror TS: set state.activeObj on each spawn; pointerAdd(ActiveObj).
	// For non-stackable count=N, this overwrites N times — last wins,
	// matching TS Engine-TS/.../ObjOps.ts:50-54 loop.
	if !objType.Stackable || count == 1 {
		for range count {
			obj := s.World.AddObj(level, x, z, objId, 1, duration, receiverID)
			if obj != nil {
				s.ActiveObj = obj
				s.Pointers |= PtrActiveObj
			}
		}
	} else {
		obj := s.World.AddObj(level, x, z, objId, count, duration, receiverID)
		if obj != nil {
			s.ActiveObj = obj
			s.Pointers |= PtrActiveObj
		}
	}
	return nil
}

// handleObjAdd (OBJ_ADD, opcode 3500) drops a private (caller-only) obj
// at the unpacked coord. Mirrors TS ObjOps.ts:20-55.
func handleObjAdd(s *ScriptState) error {
	if s.Self == nil {
		return fmt.Errorf("OBJ_ADD: no active player")
	}
	if s.World == nil {
		return fmt.Errorf("OBJ_ADD: no world surface")
	}
	return objAddCommon(s, "OBJ_ADD", s.Self.UID())
}

// handleObjDel (OBJ_DEL, opcode 3504) removes the active obj. Mirrors
// TS ObjOps.ts:112-119.
//
// NAI-115-D2 deviation: TS reads ObjType.respawnrate and passes it to
// World.removeObj as duration. goscape's Server.RemoveObj has no
// duration arg; RESPAWN-lifecycle respawn-after-delay is a foundation
// gap. DESPAWN-lifecycle objs (the firemaking smoke target) are
// unaffected.
func handleObjDel(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_DEL"); err != nil {
		return err
	}
	if s.World == nil {
		return fmt.Errorf("OBJ_DEL: no world surface")
	}
	s.World.RemoveObj(s.ActiveObj)
	return nil
}

// objAddAllReceiverID is the receiverID sentinel passed to
// WorldVars.AddObj for broadcast (visible-to-all) drops. The world
// adapter resolves this to zone.PublicReceiver. Kept package-local so
// pkg/script does not depend on pkg/zone directly.
const objAddAllReceiverID = -1

// handleObjAddAll (OBJ_ADDALL, opcode 3501) drops a broadcast
// (visible-to-all) obj at the unpacked coord. Twin of handleObjAdd —
// identical validation chain via objAddCommon; only the receiverID
// differs. Mirrors TS ObjOps.ts:58-93.
func handleObjAddAll(s *ScriptState) error {
	return objAddCommon(s, "OBJ_ADDALL", objAddAllReceiverID)
}

// handleObjCoord (OBJ_COORD, opcode 3502) packs the active obj's tile
// position into a single RS2 coord int and pushes it. Mirrors TS
// ObjOps.ts:163-166.
func handleObjCoord(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_COORD"); err != nil {
		return err
	}
	x, z, level := s.ActiveObj.Coords()
	s.PushInt(coordgrid.PackCoord(level, x, z))
	return nil
}

// handleObjCount (OBJ_COUNT, opcode 3503) pushes the active obj's
// count if it's valid for the active player; else pushes 0. Mirrors
// TS ObjOps.ts:121-130:
//
//	const obj: Obj = state.activeObj;
//	if (obj.isValid(state.activePlayer.hash64)) {
//	    state.pushInt(state.activeObj.count);
//	    return;
//	}
//	state.pushInt(0);
//
// goscape uses Self.UID() (composeUID-shaped int) instead of TS bigint
// hash64. See NAI-153-D2 in the spec.
func handleObjCount(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_COUNT"); err != nil {
		return err
	}
	if err := requireActivePlayer(s, "OBJ_COUNT"); err != nil {
		return err
	}
	if s.ActiveObj.IsValidFor(s.Self.UID()) {
		s.PushInt(s.ActiveObj.ObjCount())
		return nil
	}
	s.PushInt(0)
	return nil
}

// handleObjType (OBJ_TYPE, opcode 3511) pushes the active obj's type id.
// Mirrors TS ObjOps.ts:132-134:
//
//	[ScriptOpcode.OBJ_TYPE]: state => {
//	    state.pushInt(check(state.activeObj.type, ObjTypeValid).id);
//	},
//
// TS validates the type id via ObjTypeValid. In goscape the active obj is
// pre-validated at the wire handler (handler_opobj.go:62-70 looks up
// ObjType.Configs[objId] before constructing the obj), so the id is
// round-trip-clean. (goscape defensive guard upstream; TS re-validates here.)
func handleObjType(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_TYPE"); err != nil {
		return err
	}
	s.PushInt(s.ActiveObj.ObjType())
	return nil
}
