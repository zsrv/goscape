package world

import "github.com/zsrv/goscape/pkg/rsbuf"

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
// baseType after `duration` ticks. Mirrors TS Npc.changeType at
// Engine-TS/.../Npc.ts:427-449.
//
// Semantics:
//   - No-op when duration < 1 (TS guard; rejects 0 and negatives in
//     one check) OR when the NPC is dead (TS `!this.isActive`).
//   - On success: writes typeId, recomputes uid, writes lifecycleTick
//     (consumed by the Events block at npc_ai.go:27-43 to fire
//     revertType when it hits 0 on RESPAWN+alive), writes the mask
//     payload field changeTypeID, raises NpcMaskChangeType.
//
// DEFERRED (TS parity gaps, left for a follow-up sub-spec):
//   - Stats-reset branch (TS:436-443) — requires baseLevels/levels
//     arrays on *Npc which don't exist yet. Current engine has only
//     curHP/baseHP; a full 6-stat array port is a separate concern.
//   - The optional `reset=false` flag and its NPC_CHANGETYPE_KEEPALL
//     opcode (opcode 2506 is a reserved constant at
//     pkg/script/opcode.go:243 with no handler). Wiring KEEPALL
//     requires the stats-array infra above, so both land together.
//   - The `type === baseType && RESPAWN → setLifeCycle(-1)` fast-path
//     (TS:444-445) — minor corner case; current behavior writes
//     lifecycleTick=duration unconditionally, which fires a harmless
//     no-op revert at tick 0 (revertType is idempotent when
//     typeId == baseType).
func (n *Npc) ChangeType(newType, duration int) {
	if duration < 1 || n.dead {
		return
	}
	n.typeId = newType
	n.uid = (n.typeId << 16) | n.nid
	n.lifecycleTick = duration
	n.changeTypeID = newType
	n.masks |= rsbuf.NpcMaskChangeType
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
// NpcMaskDamage so the NPC-info encoder emits the hitsplat. curHP decrements
// by amount (clamped at 0). On overkill (amount > curHP), the emitted
// damageAmt is clamped to the pre-hit curHP so the client shows only damage
// actually dealt — matches TS Npc.applyDamage (Npc.ts:472-485). Negative
// amount is coerced to 0 defensively so a script bug cannot heal the NPC.
//
// baseHP is seeded at NPC construction (NewNpc) and refilled by ResetHP;
// Damage no longer touches it. curHP is persistent state (S6d); scripts
// calling NPC_STAT(0) on later ticks see real decremented HP.
//
// This method is a pure output op — no death / auto-retaliate / aggro logic.
// Scripts that need death handling should check NPC_STAT(0) and fire their
// own despawn flow. The AI sub-spec will later ship a real combat loop.
func (n *Npc) Damage(amount, dmgType int) {
	if amount < 0 {
		amount = 0
	}
	n.damageAmt = min(amount, n.curHP)
	n.damageType = dmgType
	n.curHP -= amount
	if n.curHP < 0 {
		n.curHP = 0
	}
	n.masks |= rsbuf.NpcMaskDamage
}

// ResetMasks clears mask bits + ephemeral per-tick state. Persistent fields
// (animID, faceSquareX/Z, changeTypeID, curHP, baseHP) are retained across
// ticks — S6d promoted curHP/baseHP from ephemeral to persistent.
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

// ResetHP re-seeds curHP + baseHP from the NPC's current NpcType.Stats
// Hitpoints slot. Called by respawn paths (on NPC death-and-respawn) and by
// AI sub-spec code that needs to restore max HP on some trigger. Safe on
// nil typ (leaves both at 0).
func (n *Npc) ResetHP() {
	hp := initialHP(n.typ)
	n.curHP = hp
	n.baseHP = hp
}
