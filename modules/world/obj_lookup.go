package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

// GetObj returns the first obj at (level, x, z) whose type matches objId and
// is visible to receiverID, or nil. Mirrors TS World.getObj / Zone.getObj
// (Zone.ts:353-360): matches public objs (ReceiverID == zone.PublicReceiver)
// OR objs privately owned by this player (ReceiverID == receiverID).
// Callers pass p.slot as receiverID.
func (s *Server) GetObj(level, x, z, objId, receiverID int) *entitypkg.Obj {
	zn := s.zoneMap.Get(level, x, z)
	for _, o := range zn.Objs {
		if o.X == x && o.Z == z && o.Type == objId &&
			(o.ReceiverID == zone.PublicReceiver || o.ReceiverID == receiverID) {
			return o
		}
	}
	return nil
}
