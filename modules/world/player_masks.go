package world

import "github.com/zsrv/goscape/pkg/rsbuf"

func (p *Player) Animate(id, delay int) {
	p.animID = id
	p.animDelay = delay
	p.masks |= rsbuf.MaskAnim
}

func (p *Player) Say(msg []byte) {
	p.sayText = msg
	p.masks |= rsbuf.MaskSay
}

func (p *Player) Chat(colour, effect, rights int, msg []byte) {
	p.chatColour = colour
	p.chatEffect = effect
	p.chatRights = rights
	p.chatBytes = msg
	p.masks |= rsbuf.MaskChat
}

func (p *Player) ShowHit(amount, dmgType, cur, base int) {
	p.damageAmt = amount
	p.damageType = dmgType
	p.curHP = cur
	p.baseHP = base
	p.masks |= rsbuf.MaskDamage
}

func (p *Player) SpotAnim(id, height, delay int) {
	p.spotanimID = id
	p.spotanimHeight = height
	p.spotanimDelay = delay
	p.masks |= rsbuf.MaskSpotAnim
}

func (p *Player) ExactMove(sX, sZ, eX, eZ, begin, finish, dir int) {
	p.exactStartX = sX
	p.exactStartZ = sZ
	p.exactEndX = eX
	p.exactEndZ = eZ
	p.exactBegin = begin
	p.exactFinish = finish
	p.exactDir = dir
	p.masks |= rsbuf.MaskExactMove
}

func (p *Player) FaceCoord(x, z int) {
	p.faceSquareX = x*2 + 1
	p.faceSquareZ = z*2 + 1
	p.masks |= rsbuf.MaskFaceCoord
}

func (p *Player) SetFaceEntity(entityIndex int) {
	p.faceEntity = entityIndex
	p.masks |= rsbuf.MaskFaceEntity
}

// ResetMasks clears mask bits and ephemeral mask state for the next tick.
// Persistent fields (animID, faceEntity, faceSquareX/Z) retained so new
// observers still see them.
func (p *Player) ResetMasks() {
	p.masks = 0
	p.sayText = nil
	p.chatBytes = nil
	p.damageAmt = -1
	p.damageType = -1
	p.curHP = -1
	p.baseHP = -1
	p.spotanimID = -1
	p.spotanimHeight = -1
	p.spotanimDelay = -1
	p.exactStartX = -1
	p.exactStartZ = -1
	p.exactEndX = -1
	p.exactEndZ = -1
	p.exactBegin = -1
	p.exactFinish = -1
	p.exactDir = -1
}
