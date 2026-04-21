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

func (n *Npc) ShowHit(amount, dmgType, cur, base int) {
	n.damageAmt = amount
	n.damageType = dmgType
	n.curHP = cur
	n.baseHP = base
	n.masks |= rsbuf.NpcMaskDamage
}

func (n *Npc) ChangeType(newType int) {
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
	prevHP := n.curHP
	if amount > prevHP {
		n.damageAmt = prevHP
	} else {
		n.damageAmt = amount
	}
	n.damageType = dmgType
	n.curHP -= amount
	if n.curHP < 0 {
		n.curHP = 0
	}
	n.masks |= rsbuf.NpcMaskDamage
}

// ResetMasks clears mask bits + ephemeral per-tick state. Persistent fields
// (animID, faceEntity, faceSquareX/Z, changeTypeID, curHP, baseHP) are
// retained across ticks — S6d promoted curHP/baseHP from ephemeral to
// persistent. damageAmt / damageType remain per-tick hitsplat payload.
func (n *Npc) ResetMasks() {
	n.masks = 0
	n.sayText = nil
	n.damageAmt = -1
	n.damageType = -1
	n.spotanimID = -1
	n.spotanimHeight = -1
	n.spotanimDelay = -1
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
