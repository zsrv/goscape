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
