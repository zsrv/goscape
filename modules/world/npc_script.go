package world

import "github.com/zsrv/goscape/pkg/script"

// Compile-time check: *Npc satisfies script.ActiveNpc.
var _ script.ActiveNpc = (*Npc)(nil)

// npcVarnCap caps the per-NPC var slice so a rogue script cannot grow
// it unboundedly. Matches the engine-wide soft cap used in S6a.
const npcVarnCap = 1024

// NpcType returns the NPC's type id.
func (n *Npc) NpcType() int { return n.typeId }

// NpcX returns the current world x coord.
func (n *Npc) NpcX() int { return n.x }

// NpcZ returns the current world z coord.
func (n *Npc) NpcZ() int { return n.z }

// NpcLevel returns the current plane/level.
func (n *Npc) NpcLevel() int { return n.level }

// NpcUID returns the packed (typeId<<16)|nid identifier.
func (n *Npc) NpcUID() int { return n.uid }

// NpcCategory returns the NPC's category, or -1 if its NpcType is nil.
func (n *Npc) NpcCategory() int {
	if n.typ == nil {
		return -1
	}
	return n.typ.Category
}

// NpcStat returns the current (boosted) stat level for the given stat id.
// S6a: only HP (id 0) is real; other stat ids return 0 (TODO: full NPC stats).
func (n *Npc) NpcStat(stat int) int {
	if stat == 0 {
		return n.curHP
	}
	return 0
}

// NpcBaseStat returns the base stat level for the given stat id.
// S6a: only HP (id 0) is real; other stat ids return 0 (TODO: full NPC stats).
func (n *Npc) NpcBaseStat(stat int) int {
	if stat == 0 {
		return n.baseHP
	}
	return 0
}

// NpcVarN reads the per-NPC var at id. Returns 0 for out-of-range ids
// (including any id never written to).
func (n *Npc) NpcVarN(id int) int32 {
	if id < 0 || id >= len(n.varns) {
		return 0
	}
	return n.varns[id]
}

// SetNpcVarN writes val to the per-NPC var at id, lazily growing the
// backing slice. Writes beyond npcVarnCap are silently dropped.
func (n *Npc) SetNpcVarN(id int, val int32) {
	if id < 0 {
		return
	}
	if id >= npcVarnCap {
		return
	}
	if id >= len(n.varns) {
		next := make([]int32, id+1)
		copy(next, n.varns)
		n.varns = next
	}
	n.varns[id] = val
}
