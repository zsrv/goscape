package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
)

// huntNpcs iterates NPCs in the grid within huntRange and returns
// those passing type + category + distance filters. Matches TS
// Npc.huntNpcs at Engine-TS/.../Npc.ts:975-977, which delegates to
// HuntIterator's NPC branch at ScriptIterators.ts:98-120.
//
// Filter coverage (NAI-9):
//   - Range: Chebyshev distance <= n.huntRange
//   - Level match: NearbyNpcs is level-filtered internally
//   - CheckNpc: type-ID filter (-1 == allow all)
//   - CheckCategory: NpcType.Category filter (-1 == allow all)
//
// DEFERRED to audit pass:
//   - CheckVis (TS ScriptIterators.ts:113-118) — LoS/LoW gate.
//
// Does NOT exclude self (TS doesn't either — NPC can appear in its
// own zone's NPC list and pass all filters). Preserves TS quirk.
func (n *Npc) huntNpcs(s *Server, hunt *objtype.HuntType) []entity {
	if s.grid == nil || s.npcTypes == nil {
		return nil
	}
	zoneRadius := 1 + n.huntRange/8
	nids := s.grid.NearbyNpcs(n.x, n.z, n.level, zoneRadius)
	var hunted []entity
	for _, nid := range nids {
		if nid < 0 || nid >= len(s.npcs) {
			continue
		}
		other := s.npcs[nid]
		if other == nil {
			continue
		}
		if hunt.CheckNpc != -1 && other.typeId != hunt.CheckNpc {
			continue
		}
		if hunt.CheckCategory != -1 {
			if other.typeId < 0 || other.typeId >= len(s.npcTypes.Configs) {
				continue
			}
			ot := s.npcTypes.Configs[other.typeId]
			if ot == nil || ot.Category != hunt.CheckCategory {
				continue
			}
		}
		dx := other.x - n.x
		if dx < 0 {
			dx = -dx
		}
		dz := other.z - n.z
		if dz < 0 {
			dz = -dz
		}
		if dx > n.huntRange || dz > n.huntRange {
			continue
		}
		// TODO: CheckVis gate — TS ScriptIterators.ts:113-118.
		// Deferred; see nai_followups.md.
		hunted = append(hunted, other)
	}
	return hunted
}
