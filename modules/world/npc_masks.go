package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

func (n *Npc) Animate(id, delay int) {
	n.animID = id
	n.animDelay = delay
	n.masks |= rsbuf.NpcMaskAnim
}

func (n *Npc) Say(msg []byte) {
	n.sayText = msg
	n.masks |= rsbuf.NpcMaskSay
}

// ChangeType morphs the NPC to newType and schedules a revert to
// baseType after `duration` ticks. Resets all 6 stats onto the new
// type's base values using a boost/drain-preserving formula. Mirrors
// TS Npc.changeType at Engine-TS/.../Npc.ts:427-449 with reset=true.
// No-op when duration < 1 OR when the NPC is dead.
func (n *Npc) ChangeType(newType, duration int) {
	n.changeTypeImpl(newType, duration, true)
}

// changeTypeImpl is the shared body behind ChangeType and the
// Task 3 ChangeTypeKeepAll. Mirrors TS Npc.changeType at
// Engine-TS/.../Npc.ts:427-449.
//
// Semantics:
//   - No-op when duration < 1 (TS guard; rejects 0 and negatives) OR
//     when the NPC is dead (TS `!this.isActive`).
//   - On success: writes typeId, changeTypeID, CHANGE_TYPE mask,
//     recomputes uid, writes resetOnRevert=reset.
//   - If reset: runs the TS:436-443 stats-reset loop against the new
//     type's stats (lookupType returns nil when the server/registry
//     is unavailable, in which case the reset silently skips — same
//     tolerance revertType already exhibits).
//   - Fast-path TS:444-445: if newType==baseType && lifecycle==RESPAWN,
//     lifecycleTick=-1 (suppresses Events-block revert). Otherwise
//     lifecycleTick=duration.
func (n *Npc) changeTypeImpl(newType, duration int, reset bool) {
	if duration < 1 || n.dead {
		return
	}
	n.typeId = newType
	n.changeTypeID = newType
	n.masks |= rsbuf.NpcMaskChangeType
	n.uid = (newType << 16) | n.nid
	n.resetOnRevert = reset

	if reset {
		if newTyp := n.lookupType(newType); newTyp != nil {
			n.resetStatsForType(newTyp)
		}
	}

	if newType == n.baseType && n.lifecycle == NpcLifecycleRespawn {
		n.lifecycleTick = -1
	} else {
		n.lifecycleTick = duration
	}
}

// lookupType returns the NpcType config for typeId, or nil if server
// or registry is unavailable or typeId is out of bounds. Mirrors the
// guard shape revertType already uses (npc.go pre-NAI-17 lines 265-268).
func (n *Npc) lookupType(typeId int) *objtype.NpcType {
	if n.server == nil || n.server.npcTypes == nil {
		return nil
	}
	if typeId < 0 || typeId >= len(n.server.npcTypes.Configs) {
		return nil
	}
	return n.server.npcTypes.Configs[typeId]
}

// resetStatsForType applies the TS Npc.ts:436-443 boost/drain-preserving
// stats reset against newTyp's Stats. For each slot i:
//
//	drain := baseLevels[i] - levels[i]     // positive: drained; negative: boosted
//	levels[i] = max(newBase - drain, 0)
//	baseLevels[i] = newBase
//
// Iterates over min(NpcStatCount, len(newTyp.Stats)) slots.
func (n *Npc) resetStatsForType(newTyp *objtype.NpcType) {
	for i := 0; i < objtype.NpcStatCount && i < len(newTyp.Stats); i++ {
		newBase := int(newTyp.Stats[i])
		drain := n.baseLevels[i] - n.levels[i]
		v := newBase - drain
		if v < 0 {
			v = 0
		}
		n.levels[i] = v
		n.baseLevels[i] = newBase
	}
}

func (n *Npc) SpotAnim(id, height, delay int) {
	n.spotanimID = id
	n.spotanimHeight = height
	n.spotanimDelay = delay
	n.masks |= rsbuf.NpcMaskSpotAnim
}

func (n *Npc) FaceCoord(x, z int) {
	n.faceSquareX = x*2 + 1
	n.faceSquareZ = z*2 + 1
	n.masks |= rsbuf.NpcMaskFaceCoord
}

func (n *Npc) SetFaceEntity(entityIndex int) {
	n.faceEntity = entityIndex
	n.masks |= rsbuf.NpcMaskFaceEntity
}

// Damage applies `amount` damage of `dmgType` to the NPC this tick, flagging
// NpcMaskDamage so the NPC-info encoder emits the hitsplat. levels[HP]
// decrements by amount (clamped at 0). On overkill (amount > cur HP), the
// emitted damageAmt is clamped to the pre-hit HP so the client shows only
// damage actually dealt — matches TS Npc.applyDamage (Npc.ts:472-485).
// Negative amount is coerced to 0 defensively so a script bug cannot heal
// the NPC.
//
// baseLevels[HP] is seeded at NPC construction (NewNpc) and refilled by
// ResetHP; Damage no longer touches it. levels[HP] is persistent state
// (S6d; extended to the full array in NAI-17); scripts calling NPC_STAT(0)
// on later ticks see real decremented HP.
//
// This method is a pure output op — no death / auto-retaliate / aggro logic.
// Scripts that need death handling should check NPC_STAT(0) and fire their
// own despawn flow. The AI sub-spec will later ship a real combat loop.
func (n *Npc) Damage(amount, dmgType int) {
	if amount < 0 {
		amount = 0
	}
	cur := n.levels[objtype.NpcStatHitpoints]
	n.damageAmt = min(amount, cur)
	n.damageType = dmgType
	cur -= amount
	if cur < 0 {
		cur = 0
	}
	n.levels[objtype.NpcStatHitpoints] = cur
	n.masks |= rsbuf.NpcMaskDamage
}

// ResetMasks clears mask bits + ephemeral per-tick state. Persistent fields
// (animID, faceSquareX/Z, changeTypeID, and the levels[]/baseLevels[]
// arrays) are retained across ticks — S6d promoted HP to persistent, NAI-17
// extended that to all 6 stats via the array migration.
// damageAmt / damageType remain per-tick hitsplat payload. faceEntity is
// retained unless the trailing-clear condition below fires.
//
// The trailing clear mirrors TS PathingEntity.ts:611-614: when the NPC
// has no target but still has a lingering faceEntity, the entitymask
// bit is re-emitted and faceEntity is snapped to -1 so the client
// receives the "stopped facing" update. Go's ResetMasks runs at tick
// end (tick.go processCleanup), so the mask bit survives into the next
// tick's info-pass — a one-tick lag vs TS which fires at tick start.
// Accepted deviation; all "official" target-clear paths
// (resetDefaults, clearInteraction) emit the mask same-tick, so this
// conditional is a defensive net for stray n.target = nil assignments.
func (n *Npc) ResetMasks() {
	n.masks = 0
	n.sayText = nil
	n.damageAmt = -1
	n.damageType = -1
	n.spotanimID = -1
	n.spotanimHeight = -1
	n.spotanimDelay = -1
	if n.target == nil && n.faceEntity != -1 {
		n.masks |= n.entitymask
		n.faceEntity = -1
	}
}

// ResetHP re-seeds the levels/baseLevels Hitpoints slot from the NPC's
// current NpcType.Stats. Called by respawn paths (on NPC death-and-respawn)
// and by AI sub-spec code that needs to restore max HP on some trigger.
// Safe on nil typ or a Stats slice that doesn't cover the Hitpoints slot
// (leaves both at 0).
func (n *Npc) ResetHP() {
	if n.typ == nil || len(n.typ.Stats) <= objtype.NpcStatHitpoints {
		n.levels[objtype.NpcStatHitpoints] = 0
		n.baseLevels[objtype.NpcStatHitpoints] = 0
		return
	}
	hp := int(n.typ.Stats[objtype.NpcStatHitpoints])
	n.levels[objtype.NpcStatHitpoints] = hp
	n.baseLevels[objtype.NpcStatHitpoints] = hp
}
