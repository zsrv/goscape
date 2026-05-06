// Package script — handlers for the Obj family of script opcodes.
package script

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/coordgrid"
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

// checkObjStack validates a stack count is positive. Mirrors TS
// NumberPositive (ScriptValidators.ts).
func checkObjStack(c int, op string) error {
	if c < 1 {
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
// NAI-115-D3 deviation: TS sets state.activeObj after each AddObj for
// pointerAdd(ActiveObj). goscape's WorldVars.AddObj has no world→state
// writeback path; smoke binds because the firemaking script does not
// consume the obj-pointer in the same handler invocation.
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

	if !objType.Stackable || count == 1 {
		for range count {
			s.World.AddObj(level, x, z, objId, 1, duration, receiverID)
		}
	} else {
		s.World.AddObj(level, x, z, objId, count, duration, receiverID)
	}
	return nil
}

// handleObjAdd (OBJ_ADD, opcode 3500) drops a private (caller-only) obj
// at the unpacked coord. Mirrors TS ObjOps.ts:20-55.
func handleObjAdd(s *ScriptState) error {
	if s.Self == nil {
		return fmt.Errorf("OBJ_ADD: no active player")
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
