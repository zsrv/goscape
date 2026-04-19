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
