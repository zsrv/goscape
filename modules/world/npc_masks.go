package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	applog "github.com/zsrv/goscape/pkg/util/log"
)

// Animate schedules sequence id with the given client-side delay on the
// NPC's primary animation slot. id=-1 clears. Mirrors TS Npc.playAnimation
// (Npc.ts:456-466) at Engine-TS pin 9aadcec4: bounds-reject on
// id >= SeqType.count and priority >= overwrite gate. NPCs have no
// animProtect equivalent (TS-faithful — Player-only field). The
// n.server == nil guard is a goscape-only nil-safe concession for test
// fixtures that construct a bare *Npc without registering through addNpc;
// TS has no analogue (its registry is a static class). 244 changed strict
// `>` to `>=`, meaning equal nonzero priority now overwrites.
// Closes deviation NAI-56-D1.
func (n *Npc) Animate(id, delay int) {
	if n.server == nil {
		return // goscape-only nil-guard for test fixtures
	}
	if id >= n.server.seqTypes.Count() {
		return // TS Npc.ts:457
	}
	if id == -1 || n.animID == -1 ||
		n.server.seqTypes.Configs[id].Priority >= n.server.seqTypes.Configs[n.animID].Priority {
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

// Damage applies `amount` damage of `dmgType` to the NPC this tick.
// HP clamp is unchanged: levels[HP] decrements by amount (clamped at 0);
// on overkill the emitted amount clamps to the pre-hit HP. Negative amount
// coerces to 0 defensively.
//
// rev-244: implements the hitmarkSlot alternation from TS Npc.ts:484-494
// (Npc.ts:475-494, 244). hitmarkSlot%2==0 → first slot (damageAmt/damageType
// + NpcMaskDamage); hitmarkSlot%2==1 → second slot (damage2Amt/damage2Type +
// NpcMaskDamage2). hitmarkSlot increments always. Slot resets to 0 each tick
// in ResetMasks (PathingEntity.ts:610). This is a pure output op.
func (n *Npc) Damage(amount, dmgType int) {
	if amount < 0 {
		amount = 0
	}
	cur := n.levels[objtype.NpcStatHitpoints]
	preHit := cur
	clamped := min(amount, cur)
	cur -= amount
	if cur < 0 {
		cur = 0
	}
	n.levels[objtype.NpcStatHitpoints] = cur
	if n.server != nil && n.server.cfg.NodeDebug && n.server.log != nil {
		applog.Trace(n.server.log, "nai128.npc.damage",
			"npc", n.uid,
			"typeId", n.typeId,
			"amount", amount,
			"dmgType", dmgType,
			"cur", preHit,
			"new", cur,
		)
	}
	// rev-244: TS Npc.ts:484-493 hitmarkSlot alternation.
	if n.hitmarkSlot%2 == 1 {
		// second slot → DAMAGE2
		n.damage2Amt = clamped
		n.damage2Type = dmgType
		n.masks |= rsbuf.NpcMaskDamage2
	} else {
		// first slot → DAMAGE
		n.damageAmt = clamped
		n.damageType = dmgType
		n.masks |= rsbuf.NpcMaskDamage
	}
	n.hitmarkSlot++
}

// ResetMasks clears mask bits + ephemeral per-tick state. Persistent fields
// (changeTypeID, and the levels[]/baseLevels[] arrays) are retained across
// ticks — S6d promoted HP to persistent, NAI-17 extended that to all 6 stats
// via the array migration. animID/animDelay are NOT persistent: TS resets them
// every tick (PathingEntity.ts:598-601) so a repeated NPC animation (combat
// attack/defend, scripted emote) can replay; see the reset below.
//
// faceSquareX/Z are reset each tick (D1, TS PathingEntity.ts:608-609): the
// rsbuf low-def force-emits FACE_COORD reading effectiveFaceCoord(), so a
// square the NPC walked away from would leak to newly-visible observers.
// effectiveFaceCoord() falls back to faceAngle (unfocus-on-spawn south + M2's
// per-step focus).
// damageAmt / damageType remain per-tick hitsplat payload. faceEntity is
// re-derived from the current target by the trailing setFaceEntity().
//
// The trailing setFaceEntity() call mirrors TS PathingEntity.ts:626
// @2e3bcf43 (ee28c1aa replaced the old `if (!this.target && faceEntity
// !== -1)` clear at L611-614 — and removed the same-tick mask emission
// from resetDefaults/clearInteraction entirely; see npc_interaction.go).
// Both engines run the reset at TICK END (Go's ResetMasks via tick.go
// processCleanup; TS's resetPathingEntity via World.processCleanup
// which runs after processClientsOut) — the armed mask is consumed by
// the next tick's info-pass in both engines, identical timing.
//
// (Investigated 2026-06-01: the prior comment claimed a "1-tick lag vs
// TS which fires at tick start" but TS resetPathingEntity is called
// from processCleanup at tick end — not tick start — so no lag exists.
// See docs/PORTING-CLOSED.md NAI-91 closure row.)
func (n *Npc) ResetMasks() {
	n.masks = 0
	n.sayText = nil
	n.damageAmt = -1
	n.damageType = -1
	// rev-244: TS PathingEntity.ts:608-610 resets hitmark2Damage/Type → -1
	// and hitmarkSlot → 0 each tick.
	n.damage2Amt = -1
	n.damage2Type = -1
	n.hitmarkSlot = 0
	// Reset the primary animation slot at tick end (TS PathingEntity.ts:598-601);
	// NpcInfo already emitted any NpcMaskAnim this tick. Without this, Animate's
	// priority guard rejects an equal-priority repeat and the NPC anim plays once.
	n.animID = -1
	n.animDelay = -1
	n.spotanimID = -1
	n.spotanimHeight = -1
	n.spotanimDelay = -1
	// D1: reset faceSquare each tick (TS PathingEntity.ts:608-609). NpcInfo
	// already read FaceSquareX()/Z() for this tick; clearing makes
	// effectiveFaceCoord fall back to faceAngle next tick so a walked-away-from
	// square doesn't render stale to newly-visible observers.
	n.faceSquareX = -1
	n.faceSquareZ = -1
	// NAI-157/NAI-167: walkDir/runDir are now reset in resetPathingEntity
	// (called from processCleanup before ResetMasks). See resetPathingEntity
	// doc-comment for the full NAI-157 rationale and TS source reference.
	// ee28c1aa @2e3bcf43: the old `if (!this.target && this.faceEntity !==
	// -1)` block at the resetPathingEntity tail was replaced by
	// `this.setFaceEntity();` (TS PathingEntity.ts:626) — facing is now
	// re-derived from the CURRENT target every tick end: live Player/Npc
	// target refreshes, nil/Loc/Obj target clears. See face_entity.go.
	n.setFaceEntity()
}

// resetPathingEntity resets the per-tick PathingEntity fields at end-of-tick,
// mirroring TS PathingEntity.resetPathingEntity (PathingEntity.ts:577-587).
// Called from processCleanup (tick.go) before ResetMasks.
//
// Fields reset here (matching TS L579-586):
//   - walkDir = -1, runDir = -1 (TS L579-580; migrated from ResetMasks, NAI-157)
//   - jump = false (TS L581; rev-274 — now a real wire field, see below)
//   - tele = false (TS L582)
//   - lastTickX = n.x, lastTickZ = n.z, lastLevel = n.level (TS L583-585)
//   - stepsTaken = 0 (TS L586)
//   - apRangeCalled = false (TS L588, L8) — see note below.
//
// rev-274: n.jump is reset here per TS PathingEntity.ts:581. It was formerly a
// player-only field (the NpcInfo protocol had no jump bit); the rev-274 add-leaf
// jump bit (crate 66911610) made it a live wire field, so the per-tick reset is
// now load-bearing (set by Teleport D5 / validateDistanceWalked, consumed by
// rsbuf.ComputeNpc, cleared here at end-of-tick).
//
// TS fields with no matching reset here (all CONFIRMED-EXCEPTION net-equivalent):
//   - moveSpeed = defaultMoveSpeed() (TS L578) — moveSpeed IS plumbed on *Npc
//     (set by ai/script paths, consumed at npc_interaction.go:347) but the per-
//     tick reset is omitted. Verdict NONE (pathing-10): Go's updateMovement
//     doesn't gate the WALK-reset path on moveSpeed, so no observable delta.
//   - interacted (TS L587) — no equivalent field on *Npc (goscape's NPC
//     interaction model has no `interacted` flag; never read on NPC).
//
// L8: apRangeCalled is now reset per-tick here, matching TS PathingEntity.ts:588,
// instead of only event-driven at SetInteraction/clearInteraction. The NPC-side
// field currently has no reader (only p.apRangeCalled is consumed, in the player
// interaction path), so the per-tick-vs-event distinction is unobservable today —
// but this makes the field TS-faithful for when an NPC AP-range read-gate is ported.
func (n *Npc) resetPathingEntity() {
	n.walkDir = -1
	n.runDir = -1
	n.jump = false // rev-274: PathingEntity.ts:581 — see doc-block above
	n.tele = false
	n.lastTickX = n.x
	n.lastTickZ = n.z
	n.lastLevel = n.level
	n.stepsTaken = 0
	n.apRangeCalled = false
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
