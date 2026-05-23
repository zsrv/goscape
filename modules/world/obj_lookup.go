package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

// GetObj returns the first obj at (level, x, z) whose type matches objId and
// is visible to receiverID, or nil. Mirrors TS World.getObj / Zone.getObj
// (Zone.ts:353-360): matches public objs (ReceiverID == zone.PublicReceiver)
// OR objs privately owned by this player (ReceiverID == receiverID).
// Callers pass the player's UID (p.uid / Self.UID()) as receiverID — the
// same identity the drop path stores (handlers_inv.go / handlers_obj.go use
// s.Self.UID()) and the zone-visibility filter compares (player_zone.go
// p.uid). NOT p.slot: a private drop's ReceiverID is a composeUID-shaped
// hash, so querying with the small slot index would never match the
// dropper's own obj until reveal flips it to PublicReceiver.
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
