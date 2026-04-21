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
