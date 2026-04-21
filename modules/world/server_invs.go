package world

import (
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// invLookupView adapts *Server to script.InvLookup. The single
// *Player downcast is contained here.
type invLookupView struct {
	s *Server
}

func (v invLookupView) Get(self script.ActivePlayer, typeID int) *inventory.Inventory {
	if v.s == nil || v.s.invTypes == nil {
		return nil
	}
	if typeID < 0 || typeID >= len(v.s.invTypes.Configs) {
		return nil
	}
	cfg := v.s.invTypes.Configs[typeID]
	if cfg == nil {
		return nil
	}
	if cfg.Scope == objtype.InvTypeScopeShared {
		return v.s.invs[typeID]
	}
	p, ok := self.(*Player)
	if !ok {
		return nil
	}
	return p.invs[typeID]
}
