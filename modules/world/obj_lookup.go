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
		// L9: skip invalid objs. TS Zone.getObj iterates getObjsSafe, which
		// filters obj.isValid() = count >= 1 && isActive (Zone.ts:425, Obj.ts:52
		// — the reveal/hash64 branch is skipped because getObjsSafe calls
		// isValid() with no hash). Without this a depleted (count<1) or removed
		// (!isActive) obj lingering in zn.Objs could be returned.
		if o.Count < 1 || !o.IsActive {
			continue
		}
		if o.X == x && o.Z == z && o.Type == objId &&
			(o.ReceiverID == zone.PublicReceiver || o.ReceiverID == receiverID) {
			return o
		}
	}
	return nil
}

// getObjOfReceiver returns the obj at (level, x, z) of type objId whose
// ReceiverID *exactly* equals receiverID, or nil. Unlike GetObj — which also
// matches public objs visible to the receiver — this is an exact-receiver
// match: it is the variant TS World.addObj uses to decide whether a fresh
// drop should merge into an existing pile (you only merge with a pile owned
// by the same receiver, never a public pile into a private drop or vice
// versa). For public drops both receiverID and the existing ReceiverID are
// zone.PublicReceiver, so exact equality handles that case too. Mirrors TS
// Zone.getObjOfReceiver (Engine-TS/src/engine/zone/Zone.ts:362-369), which
// iterates getObjsSafe (Zone.ts:423-429) so only obj.isValid() entries are
// yielded; Obj.isValid (Obj.ts:52-62) called with no hash reduces to
// count >= 1 && super.isValid() (Entity.isActive). Without the filter a
// depleted (count<1) or removed-but-still-linked (!isActive) obj would
// merge a fresh drop into a stale pile — TS skips it and falls through to
// addObj's "no merge target" branch.
func (s *Server) getObjOfReceiver(level, x, z, objId, receiverID int) *entitypkg.Obj {
	zn := s.zoneMap.Get(level, x, z)
	for _, o := range zn.Objs {
		if o.Count < 1 || !o.IsActive {
			continue
		}
		if o.X == x && o.Z == z && o.Type == objId && o.ReceiverID == receiverID {
			return o
		}
	}
	return nil
}
