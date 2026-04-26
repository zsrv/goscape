package world

import (
	"github.com/zsrv/goscape/pkg/script"
)

// serverNpcLookup implements script.NpcLookup by linearly iterating
// s.npcs. See S7f spec §3.3 and deviations S7f-D1 (huntvis validated-only
// at the handler, not filtered here) and S7f-D2 (linear iteration).
type serverNpcLookup struct{ s *Server }

// Compile-time assertion: serverNpcLookup must satisfy script.NpcLookup.
var _ script.NpcLookup = serverNpcLookup{}

// FindClosestNpcByType returns the NPC of typeID closest (euclidean-squared)
// to (level, x, z) within a square-bounded dist. Later-iterated NPCs win
// ties (TS NpcOps.ts:353 uses `<=`). huntvis is accepted but not filtered
// on (S7f-D1). Mirrors TS NpcOps.ts:336-367.
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
// via l.s.npcTypes.Configs (with nil guards). huntvis accepted but not
// filtered (S7f-D1). Mirrors TS NpcOps.ts:369-400.
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
