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
