package world

import (
	"github.com/zsrv/goscape/pkg/script"
)

// serverNpcLookup implements script.NpcLookup by linearly iterating
// s.npcs. See S7f spec §3.3 and the residual deviation NAI-33-D1 /
// S7f-D1 (huntvis validated-only on FindClosest* — partially closed by
// NAI-35 for HuntAll-mode iterators) and S7f-D2 (linear iteration).
type serverNpcLookup struct{ s *Server }

// Compile-time assertion: serverNpcLookup must satisfy script.NpcLookup.
var _ script.NpcLookup = serverNpcLookup{}

// FindClosestNpcByType returns the NPC of typeID closest (euclidean-squared)
// to (level, x, z) within a square-bounded dist. Later-iterated NPCs win
// ties (TS NpcOps.ts:353 uses `<=`). Mirrors TS NpcOps.ts:336-367.
//
// huntvis is validated upstream but NOT filtered on here — preserves the
// NAI-33-D1 / S7f-D1 deferred posture (audit if NPC_FIND consumers gain
// LoS/LoW gating). HuntAll-mode iterators (NewHuntAllNpcIterator) DO
// filter; this method does not.
func (l serverNpcLookup) FindClosestNpcByType(level, x, z, dist, typeID, _ int) script.ActiveNpc {
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
// huntvis is validated upstream but NOT filtered on here — preserves the
// NAI-33-D1 / S7f-D1 deferred posture (audit if NPC_FINDCAT consumers
// gain LoS/LoW gating). HuntAll-mode iterators (NewHuntAllNpcIterator)
// DO filter; this method does not.
func (l serverNpcLookup) FindClosestNpcByCategory(level, x, z, dist, cat, _ int) script.ActiveNpc {
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
