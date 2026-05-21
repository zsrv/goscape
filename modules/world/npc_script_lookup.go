package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// serverNpcLookup implements script.NpcLookup by linearly iterating
// s.npcs (S7f-D2 — direct-scan vs TS's NpcIterator(DISTANCE)-then-closest
// shape at NpcOps.ts:347-380). huntvis filtering on FindClosest* is
// active via huntvisGate (closes NAI-33-D1 / S7f-D1).
type serverNpcLookup struct{ s *Server }

// Compile-time assertion: serverNpcLookup must satisfy script.NpcLookup.
var _ script.NpcLookup = serverNpcLookup{}

// FindClosestNpcByType returns the NPC of typeID closest (euclidean-squared)
// to (level, x, z) within a square-bounded dist. Later-iterated NPCs win
// ties (TS NpcOps.ts:353 uses `<=`). Mirrors TS NpcOps.ts:336-367.
//
// huntvis is consumed via huntvisGate (LoS/LoW filter applied after the
// square-bounds check, before euclidean-squared distance compute);
// nil-LineValidator pessimistically allows. Semantic-equivalent to TS
// NPC_FIND's NpcIterator(DISTANCE)-then-closest shape at NpcOps.ts:347-365.
func (l serverNpcLookup) FindClosestNpcByType(level, x, z, dist, typeID, huntvis int) script.ActiveNpc {
	var closest *Npc
	bestDist := 1<<31 - 1 // max int32-safe sentinel
	for _, n := range l.s.npcs {
		if n == nil || n.level != level || n.typeId != typeID {
			continue
		}
		dx := n.x - x
		dz := n.z - z
		if dx < 0 {
			dx = -dx
		}
		if dz < 0 {
			dz = -dz
		}
		if dx > dist || dz > dist {
			continue
		}
		if !l.huntvisGate(level, x, z, n.x, n.z, huntvis) {
			continue
		}
		d := (n.x-x)*(n.x-x) + (n.z-z)*(n.z-z)
		if d <= bestDist {
			closest = n
			bestDist = d
		}
	}
	if closest == nil {
		return nil
	}
	return closest
}

// FindClosestNpcByCategory is the NPC_FINDCAT analogue of FindClosestNpcByType.
// Filters on NpcType.Category == cat rather than typeID. Looks up Category
// via l.s.npcTypes.Configs (with nil guards). Mirrors TS NpcOps.ts:369-400.
//
// huntvis is consumed via huntvisGate (same shape as FindClosestNpcByType);
// semantic-equivalent to TS NPC_FINDCAT's NpcIterator(DISTANCE)-then-closest
// shape at NpcOps.ts:380-396.
func (l serverNpcLookup) FindClosestNpcByCategory(level, x, z, dist, cat, huntvis int) script.ActiveNpc {
	if l.s.npcTypes == nil {
		return nil
	}
	var closest *Npc
	bestDist := 1<<31 - 1
	for _, n := range l.s.npcs {
		if n == nil || n.level != level {
			continue
		}
		if n.typeId < 0 || n.typeId >= len(l.s.npcTypes.Configs) {
			continue
		}
		nt := l.s.npcTypes.Configs[n.typeId]
		if nt == nil || nt.Category != cat {
			continue
		}
		dx := n.x - x
		dz := n.z - z
		if dx < 0 {
			dx = -dx
		}
		if dz < 0 {
			dz = -dz
		}
		if dx > dist || dz > dist {
			continue
		}
		if !l.huntvisGate(level, x, z, n.x, n.z, huntvis) {
			continue
		}
		d := (n.x-x)*(n.x-x) + (n.z-z)*(n.z-z)
		if d <= bestDist {
			closest = n
			bestDist = d
		}
	}
	if closest == nil {
		return nil
	}
	return closest
}

// FindNpcAtExactCoord returns the first NPC at exactly (level, x, z) whose
// type matches typeID, or nil. Mirrors TS NpcOps.ts:94-112 (ZONE-iterator
// scope; here we approximate with a linear scan — S7f-D2).
func (l serverNpcLookup) FindNpcAtExactCoord(level, x, z, typeID int) script.ActiveNpc {
	for _, n := range l.s.npcs {
		if n == nil {
			continue
		}
		if n.level == level && n.x == x && n.z == z && n.typeId == typeID {
			return n
		}
	}
	return nil
}

// FindNpcByUID resolves a packed NPC UID to the live NPC at that slot
// only when the NPC's typeId matches the high-16-bit `expectedType`
// embedded in the UID. Mirrors TS NpcOps.ts:26-40:
//
//	const slot = npcUid & 0xffff;
//	const expectedType = (npcUid >> 16) & 0xffff;
//	const npc = World.getNpc(slot);
//	if (!npc || npc.type !== expectedType) { ... return null }
//
// NAI-120 Bundle 2A.
func (l serverNpcLookup) FindNpcByUID(uid int) script.ActiveNpc {
	slot := uid & 0xffff
	expectedType := (uid >> 16) & 0xffff
	if slot < 0 || slot >= len(l.s.npcs) {
		return nil
	}
	n := l.s.npcs[slot]
	if n == nil || n.typeId != expectedType {
		return nil
	}
	return n
}

// huntvisGate applies the HuntVisOff/LineOfSight/LineOfWalk filter
// using the server's scriptLineValidator. Nil-validator → pessimistic
// allow, matching the pkg/script iterator convention at
// npc_iterator.go:138-141 (npcVisibleViaLineOfSight).
//
// Arg tuple (1, 1, 1, 0) and iterator-as-src ordering mirror TS
// NpcIterator DISTANCE-mode at ScriptIterators.ts:348/351 — NOT the
// player-iterator-reversed shape at line 216 (see PlayerIterator
// passesFilter for that variant). Closes NAI-33-D1 / S7f-D1 for the
// FindClosestNpc* family.
func (l serverNpcLookup) huntvisGate(level, srcX, srcZ, dstX, dstZ, huntvis int) bool {
	switch huntvis {
	case objtype.HuntVisOff:
		return true
	case objtype.HuntVisLineOfSight:
		lv := l.s.scriptLineValidator()
		if lv == nil {
			return true
		}
		return lv.HasLineOfSight(level, srcX, srcZ, dstX, dstZ, 1, 1, 1, 0)
	case objtype.HuntVisLineOfWalk:
		lv := l.s.scriptLineValidator()
		if lv == nil {
			return true
		}
		return lv.HasLineOfWalk(level, srcX, srcZ, dstX, dstZ, 1, 1, 1, 0)
	}
	return true
}

// ZoneNpcs returns all valid NPCs in the zone at (level, zoneX, zoneZ).
// Mirrors TS Zone.getAllNpcsSafe(true) consumed by NpcIterator
// (ScriptIterators.ts:330,341). Zone resolution via pkg/zone.ZoneMap.Get
// which masks the world coords to zone bounds internally. nil zoneMap
// (defense) and nil zone (off-grid) both return nil. NpcsSafe filters
// non-IsValid entries (zone.go:439). reverse=true mirrors TS
// getAllNpcsSafe(true) traversal order.
func (l serverNpcLookup) ZoneNpcs(level, zoneX, zoneZ int) []script.ActiveNpc {
	if l.s.zoneMap == nil {
		return nil
	}
	z := l.s.zoneMap.Get(level, zoneX, zoneZ)
	if z == nil {
		return nil
	}
	out := make([]script.ActiveNpc, 0, z.NpcsCount())
	for n := range z.NpcsSafe(true) {
		out = append(out, n.(script.ActiveNpc)) // *Npc satisfies both pkg/zone.NpcLike and pkg/script.ActiveNpc (assertion at npc_script.go)
	}
	return out
}
