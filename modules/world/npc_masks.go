package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// Animate schedules sequence id with the given client-side delay on the
// NPC's primary animation slot. id=-1 clears. Mirrors TS Npc.playAnimation
// (Npc.ts:451-462): bounds-reject on id >= SeqType.count and
// priority-comparison overwrite gate. NPCs have no animProtect equivalent
// (TS-faithful — Player-only field). The n.server == nil guard is a
// goscape-only nil-safe concession for test fixtures that construct a
// bare *Npc without registering through addNpc; TS has no analogue
// (its registry is a static class). Closes deviation NAI-56-D1.
func (n *Npc) Animate(id, delay int) {
	if n.server == nil {
		return // goscape-only nil-guard for test fixtures
	}
	if id >= n.server.seqTypes.Count() {
		return // TS Npc.ts:452
	}
	if id == -1 || n.animID == -1 ||
		n.server.seqTypes.Configs[id].Priority > n.server.seqTypes.Configs[n.animID].Priority ||
		n.server.seqTypes.Configs[n.animID].Priority == 0 {
		n.animID = id
		n.animDelay = delay
		n.masks |= rsbuf.NpcMaskAnim
	}
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

// ChangeTypeKeepAll morphs the NPC to newType and schedules a revert
// after `duration` ticks without resetting stats. Dispatched from
// NPC_CHANGETYPE_KEEPALL (opcode 2506). Mirrors TS Npc.changeType at
// Engine-TS/.../Npc.ts:427-449 with reset=false. The revert, when it
// fires, takes the light path (resetOnRevert=false → Task 4 branch).
func (n *Npc) ChangeTypeKeepAll(newType, duration int) {
	n.changeTypeImpl(newType, duration, false)
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

	// NAI-19 (B2): refresh n.typ snapshot on BOTH paths so post-changetype
	// geometry reads (NAI-18 inApproachDistance LoS via n.typ.Size, future
	// combat / wander reads) see the new type. TS fetches type fresh on
	// every access via NpcType.get(this.type); Go's snapshot model needs
	// explicit refresh.
	if newTyp := n.lookupType(newType); newTyp != nil {
		n.typ = newTyp
		if reset {
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
	for i := range min(objtype.NpcStatCount, len(newTyp.Stats)) {
		newBase := int(newTyp.Stats[i])
		drain := n.baseLevels[i] - n.levels[i]
		v := max(newBase-drain, 0)
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
// This method is a pure output op — no death / auto-retaliate / aggro logic,
// matching TS Npc.applyDamage (which is itself called only from the
// NPC_DAMAGE script handler at NpcOps.ts:267). Death is content-script
// driven in TS too — there is no engine-side death trigger. Content
// scripts check NPC_STAT(0)<=0 and call npc_del; the engine path from
// there (handleNpcDel → Server.removeNpc → NpcLifecycleRespawn at
// npc_ai.go:26-65 → revertType + AI_SPAWN) is already wired.
func (n *Npc) Damage(amount, dmgType int) {
	if amount < 0 {
		amount = 0
	}
	cur := n.levels[objtype.NpcStatHitpoints]
	preHit := cur
	n.damageAmt = min(amount, cur)
	n.damageType = dmgType
	cur -= amount
	if cur < 0 {
		cur = 0
	}
	n.levels[objtype.NpcStatHitpoints] = cur
	if n.server != nil && n.server.cfg.NodeDebug && n.server.log != nil {
		n.server.log.Info("nai128.npc.damage",
			"npc", n.uid,
			"typeId", n.typeId,
			"amount", amount,
			"dmgType", dmgType,
			"cur", preHit,
			"new", cur,
		)
	}
	n.masks |= rsbuf.NpcMaskDamage
}

// ResetMasks clears mask bits + ephemeral per-tick state. Persistent fields
// (faceSquareX/Z, changeTypeID, and the levels[]/baseLevels[] arrays) are
// retained across ticks — S6d promoted HP to persistent, NAI-17 extended
// that to all 6 stats via the array migration. animID/animDelay are NOT
// persistent: TS resets them every tick (PathingEntity.ts:598-601) so a
// repeated NPC animation (combat attack/defend, scripted emote) can replay;
// see the reset below.
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
	// Reset the primary animation slot at tick end (TS PathingEntity.ts:598-601);
	// NpcInfo already emitted any NpcMaskAnim this tick. Without this, Animate's
	// priority guard rejects an equal-priority repeat and the NPC anim plays once.
	n.animID = -1
	n.animDelay = -1
	n.spotanimID = -1
	n.spotanimHeight = -1
	n.spotanimDelay = -1
	// NAI-157/NAI-167: walkDir/runDir are now reset in resetPathingEntity
	// (called from processCleanup before ResetMasks). See resetPathingEntity
	// doc-comment for the full NAI-157 rationale and TS source reference.
	if n.target == nil && n.faceEntity != -1 {
		n.masks |= n.entitymask
		n.faceEntity = -1
	}
}

// resetPathingEntity resets the per-tick PathingEntity fields at end-of-tick,
// mirroring TS PathingEntity.resetPathingEntity (PathingEntity.ts:577-587).
// Called from processCleanup (tick.go) before ResetMasks.
//
// Fields reset here (matching TS L579-586):
//   - walkDir = -1, runDir = -1 (TS L579-580; migrated from ResetMasks, NAI-157)
//   - tele = false (TS L582)
//   - lastTickX = n.x, lastTickZ = n.z, lastLevel = n.level (TS L583-585)
//   - stepsTaken = 0 (TS L586)
//
// Out-of-scope per spec §6 (fields not yet on *Npc):
//   - moveSpeed = defaultMoveSpeed() (TS L578) — NPC moveSpeed plumbing deferred
//   - jump = false (TS L581) — player-only field
//   - interacted = false (TS L587), apRangeCalled = false (TS L588) — AP-range deferred
func (n *Npc) resetPathingEntity() {
	n.walkDir = -1
	n.runDir = -1
	n.tele = false
	n.lastTickX = n.x
	n.lastTickZ = n.z
	n.lastLevel = n.level
	n.stepsTaken = 0
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
