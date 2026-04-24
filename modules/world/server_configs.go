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
