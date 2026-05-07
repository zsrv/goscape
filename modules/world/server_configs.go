package world

import "github.com/zsrv/goscape/pkg/objtype"

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
	if cfg == nil {
		return objtype.ScriptVarTypeInt, false
	}
	return cfg.Type, cfg.Protect
}

func (c serverConfigsView) VarnType(id int) objtype.ScriptVarType {
	if c.s == nil || c.s.varnTypes == nil {
		return objtype.ScriptVarTypeInt
	}
	if id < 0 || id >= len(c.s.varnTypes.Configs) {
		return objtype.ScriptVarTypeInt
	}
	cfg := c.s.varnTypes.Configs[id]
	if cfg == nil {
		return objtype.ScriptVarTypeInt
	}
	return cfg.Type
}
