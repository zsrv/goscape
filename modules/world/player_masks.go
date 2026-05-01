package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

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
// Persistent fields (animID, faceEntity, faceSquareX/Z, levels[Hitpoints],
// baseLevels[Hitpoints]) retained — S6e promoted Player HP from per-tick
// ephemeral to persistent, routed through the skill arrays. Also clears
// one-shot movement intents (tele, jump) so a single-tick teleport
// emission doesn't repeat next tick.
func (p *Player) ResetMasks() {
	p.masks = 0
	p.tele = false
	p.jump = false
	p.sayText = nil
	p.chatBytes = nil
	p.damageAmt = -1
	p.damageType = -1
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

// Damage applies `amount` damage of `dmgType` to the player this tick,
// flagging MaskDamage so the player-info encoder emits the hitsplat. HP
// decrements via levels[Hitpoints] (the single source of truth — no
// separate curHP field as of S6e). On overkill (amount > current HP),
// damageAmt clamps to the pre-hit HP so the wire shows only damage
// actually dealt — matches TS Player.applyDamage (Player.ts:1860-1873).
//
// Negative amount coerces to 0 defensively. This deviates from TS where
// negative amount would heal the player (current - (-3) = current + 3
// passes the overkill check and writes back). The TS path is almost
// certainly an unintended consequence of unsigned-input assumptions; we
// match the *Npc.Damage convention from S6c instead.
//
// This is a pure output op — no death / auto-retaliate / aggro logic.
// Death/respawn/regen belong in a future combat sub-spec.
func (p *Player) Damage(amount, dmgType int) {
	if amount < 0 {
		amount = 0
	}
	current := int(p.levels[objtype.PlayerStatHitpoints])
	p.damageAmt = min(amount, current)
	p.damageType = dmgType
	next := current - amount
	if next < 0 {
		next = 0
	}
	p.levels[objtype.PlayerStatHitpoints] = uint8(next)
	p.masks |= rsbuf.MaskDamage
}

// ResetHP restores levels[Hitpoints] to baseLevels[Hitpoints] — the
// player's "full HP" state. Called by respawn paths and certain script
// triggers in future sub-specs. Boost/drain effects on Hitpoints are
// wiped (RS2 convention: respawn fills to base, not boosted-max).
//
// No direct TS counterpart — TS performs HP refill inline within death
// handling. This Go-side helper makes the intent reusable.
func (p *Player) ResetHP() {
	p.levels[objtype.PlayerStatHitpoints] = p.baseLevels[objtype.PlayerStatHitpoints]
}
