package world

import (
	"github.com/zsrv/goscape/pkg/fonttype"
	"github.com/zsrv/goscape/pkg/objtype"
)

// serverConfigsView adapts *Server to script.Configs. Kept value-typed
// so tests can construct it without a running server.
type serverConfigsView struct {
	s *Server
}

func (c serverConfigsView) ObjType(id int) *objtype.ObjType {
	if c.s == nil || c.s.objTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.objTypes.Configs) {
		return nil
	}
	return c.s.objTypes.Configs[id]
}

func (c serverConfigsView) NpcType(id int) *objtype.NpcType {
	if c.s == nil || c.s.npcTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.npcTypes.Configs) {
		return nil
	}
	return c.s.npcTypes.Configs[id]
}

func (c serverConfigsView) LocType(id int) *objtype.LocType {
	if c.s == nil || c.s.locTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.locTypes.Configs) {
		return nil
	}
	return c.s.locTypes.Configs[id]
}

func (c serverConfigsView) EnumType(id int) *objtype.EnumType {
	if c.s == nil || c.s.enumTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.enumTypes.Configs) {
		return nil
	}
	return c.s.enumTypes.Configs[id]
}

func (c serverConfigsView) StructType(id int) *objtype.StructType {
	if c.s == nil || c.s.structTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.structTypes.Configs) {
		return nil
	}
	return c.s.structTypes.Configs[id]
}

func (c serverConfigsView) ParamType(id int) *objtype.ParamType {
	if c.s == nil || c.s.paramTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.paramTypes.Configs) {
		return nil
	}
	return c.s.paramTypes.Configs[id]
}

func (c serverConfigsView) InvType(id int) *objtype.InvType {
	if c.s == nil || c.s.invTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.invTypes.Configs) {
		return nil
	}
	return c.s.invTypes.Configs[id]
}

func (c serverConfigsView) IdkType(id int) *objtype.IdkType {
	if c.s == nil || c.s.idkTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.idkTypes.Configs) {
		return nil
	}
	return c.s.idkTypes.Configs[id]
}

func (c serverConfigsView) SpotAnimType(id int) *objtype.SpotanimType {
	if c.s == nil || c.s.spotanimTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.spotanimTypes.Configs) {
		return nil
	}
	return c.s.spotanimTypes.Configs[id]
}

func (c serverConfigsView) SeqType(id int) *objtype.SeqType {
	if c.s == nil || c.s.seqTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.seqTypes.Configs) {
		return nil
	}
	return c.s.seqTypes.Configs[id]
}

// HuntType returns the hunt-config entry for id, or nil when not loaded
// or out of range. Used by NPC_SETHUNTMODE validation. Mirrors TS
// HuntType.get at Engine-TS/src/cache/config/HuntType.ts.
func (c serverConfigsView) HuntType(id int) *objtype.HuntType {
	if c.s == nil || c.s.huntTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.huntTypes.Configs) {
		return nil
	}
	return c.s.huntTypes.Configs[id]
}

func (c serverConfigsView) DbTableType(id int) *objtype.DbTableType {
	if c.s == nil || c.s.dbTableTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.dbTableTypes.Configs) {
		return nil
	}
	return c.s.dbTableTypes.Configs[id]
}

func (c serverConfigsView) DbRowType(id int) *objtype.DbRowType {
	if c.s == nil || c.s.dbRowTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.dbRowTypes.Configs) {
		return nil
	}
	return c.s.dbRowTypes.Configs[id]
}

// DbRowsInTable returns the pre-computed row IDs for the given table
// (S7d-D4). Returns nil when the catalogue is absent or no rows match.
func (c serverConfigsView) DbRowsInTable(tableID int) []int {
	if c.s == nil || c.s.dbRowTypes == nil {
		return nil
	}
	return c.s.dbRowTypes.RowsByTable[tableID]
}

// FindDbRowsInt delegates to the DbTableIndex built at world bootstrap.
// Returns nil if the server or index is uninitialized.
func (c serverConfigsView) FindDbRowsInt(query int32, packed int) []int {
	if c.s == nil || c.s.dbTableIndex == nil {
		return nil
	}
	return c.s.dbTableIndex.FindInt(query, packed)
}

// FindDbRowsStr — string-valued variant of FindDbRowsInt.
func (c serverConfigsView) FindDbRowsStr(query string, packed int) []int {
	if c.s == nil || c.s.dbTableIndex == nil {
		return nil
	}
	return c.s.dbTableIndex.FindStr(query, packed)
}

func (c serverConfigsView) VarpType(id int) (objtype.ScriptVarType, bool) {
	if c.s == nil || c.s.varpTypes == nil {
		return objtype.ScriptVarTypeInt, false
	}
	if id < 0 || id >= len(c.s.varpTypes.Configs) {
		return objtype.ScriptVarTypeInt, false
	}
	cfg := c.s.varpTypes.Configs[id]
	return cfg.Type, cfg.Protect
}

func (c serverConfigsView) VarnType(id int) objtype.ScriptVarType {
	if c.s == nil || c.s.varnTypes == nil {
		return objtype.ScriptVarTypeInt
	}
	if id < 0 || id >= len(c.s.varnTypes.Configs) {
		return objtype.ScriptVarTypeInt
	}
	return c.s.varnTypes.Configs[id].Type
}

// VarsType implements script.Configs.VarsType. Out-of-range / unloaded
// id returns ScriptVarTypeInt (DEVIATION-NAI-121-D3 — silent default
// matching VarpType/VarnType). Read by handlePushVars / handlePopVars
// to route between WorldVars.VarsInt and WorldVars.VarsString.
func (c serverConfigsView) VarsType(id int) objtype.ScriptVarType {
	if c.s == nil || c.s.varsTypes == nil {
		return objtype.ScriptVarTypeInt
	}
	if id < 0 || id >= len(c.s.varsTypes.Configs) {
		return objtype.ScriptVarTypeInt
	}
	return c.s.varsTypes.Configs[id].Type
}

// ObjByName implements script.Configs.ObjByName. Delegates to
// ObjTypeConfigs.ByName. Returns nil when the server or configs are
// uninitialized or the name has no match. NAI-162 B2.
func (c serverConfigsView) ObjByName(name string) *objtype.ObjType {
	if c.s == nil || c.s.objTypes == nil {
		return nil
	}
	return c.s.objTypes.ByName(name)
}

// MesanimType returns the message-animation config for id or nil when
// out of range. NAI-179.
func (c serverConfigsView) MesanimType(id int) *objtype.MesanimType {
	if c.s.mesanimTypes == nil || id < 0 || id >= len(c.s.mesanimTypes.Configs) {
		return nil
	}
	return c.s.mesanimTypes.Configs[id]
}

// MesanimByName resolves a mesanim debugname to its config id, or -1.
// Mirrors TS MesanimType.getId. NAI-179.
func (c serverConfigsView) MesanimByName(name string) int {
	if c.s.mesanimTypes == nil {
		return -1
	}
	if id, ok := c.s.mesanimTypes.ConfigNames[name]; ok {
		return id
	}
	return -1
}

// FontType returns the per-byte width table for font id 0..3, or nil
// when the title cache wasn't loaded / id is out of range. NAI-179.
func (c serverConfigsView) FontType(id int) *fonttype.FontType {
	if id < 0 || id >= len(c.s.fontTypes) {
		return nil
	}
	return c.s.fontTypes[id]
}
