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
// by amount (clamped at 0); baseHP is set from NpcType.Stats[3] (the
// unexported npcStatHitpoints index in pkg/objtype/npctype.go) if available,
// otherwise left at its current value. Negative amount is coerced to 0
// defensively so a script bug cannot heal the NPC.
//
// This method is a pure output op — no death / auto-retaliate / aggro logic.
// Scripts that need death handling should check NPC_STAT(0) and fire their
// own despawn flow. The AI sub-spec will later ship a real combat loop.
func (n *Npc) Damage(amount, dmgType int) {
	if amount < 0 {
		amount = 0
	}
	n.damageAmt = amount
	n.damageType = dmgType
	n.curHP -= amount
	if n.curHP < 0 {
		n.curHP = 0
	}
	if n.typ != nil {
		// Stats[3] is the Hitpoints slot (npcStatHitpoints=3 in
		// pkg/objtype/npctype.go; unexported, so we use the literal index).
		if len(n.typ.Stats) > 3 {
			if hp := int(n.typ.Stats[3]); hp > 0 {
				n.baseHP = hp
			}
		}
	}
	n.masks |= rsbuf.NpcMaskDamage
}

// ResetMasks clears mask bits + ephemeral state. Persistent fields (animID,
// faceEntity, faceSquareX/Z, changeTypeID) retained.
func (n *Npc) ResetMasks() {
	n.masks = 0
	n.sayText = nil
	n.damageAmt = -1
	n.damageType = -1
	n.curHP = -1
	n.baseHP = -1
	n.spotanimID = -1
	n.spotanimHeight = -1
	n.spotanimDelay = -1
}
