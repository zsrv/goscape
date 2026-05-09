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

// ResetMasks clears mask bits + ephemeral per-tick state. Mirrors TS
// PathingEntity.resetPathingEntity (PathingEntity.ts:577-615) plus the
// Player-only fields TS resets in Player.resetEntity (Player.ts:454-467,
// non-respawn branch).
//
// Persistent-by-design (TS resets, goscape preserves):
//   - animID/animDelay (TS PathingEntity.ts:598-600) — animations carry
//     across ticks until a new PlayAnim/script-driven change.
//   - faceSquareX/Z (TS PathingEntity.ts:608-609) — non-symptomatic
//     persistence; the encoder gates on MaskFaceCoord which IS cleared
//     via `p.masks = 0` below.
//   - levels[Hitpoints] / baseLevels[Hitpoints] (S6e promotion to
//     persistent via the skill arrays).
//
// Handled-elsewhere (NOT in ResetMasks; equivalent goscape paths):
//   - walkDir/runDir — reset/set in movement.go:53-65 per movement step.
//   - stepsTaken — reset in movement.go:46 (pinned by
//     TestResolveMovementResetsStepsTaken at movement_test.go:214).
//   - lastTickX/Z + lastLevel — set in movement.go:48-50 per movement step.
//   - interacted/apRangeCalled — reset on SetInteraction (interaction.go:85-86),
//     ClearInteraction (interaction.go:133-134), and post-fire
//     (player_interaction_trigger.go:121).
//   - socialProtect/reportAbuseProtect — reset in tick.go:538-539 (NAI-72).
//
// Also clears one-shot movement intents (tele, jump) so a single-tick
// teleport emission doesn't repeat next tick.
//
// The trailing-clear mirrors TS PathingEntity.ts:611-614 with a
// one-tick lag deviation (Go's ResetMasks runs at tick end via
// tick.go processCleanup, so the mask bit is consumed by the NEXT
// tick's info-pass — same convention as Npc.ResetMasks at
// npc_masks.go:184-207). Closes NAI-91's "player keeps facing NPC
// after walking away" smoke residual.
func (p *Player) ResetMasks() {
	p.masks = 0
	p.tele = false
	p.jump = false
	// NAI-135 (retires NAI-108-D-MOVESPEED-NOT-RESET): mirrors TS
	// PathingEntity.resetPathingEntity at PathingEntity.ts:578.
	// Cleared at tick-end so the next tick's bridge sees a non-Instant
	// moveSpeed (rsbuf already consumed any same-tick teleport block
	// earlier in the cycle via Tele()/Jump() readers).
	p.moveSpeed = p.defaultMoveSpeed()
	p.sayText = nil
	p.chatBytes = nil
	p.chatColour = -1
	p.chatEffect = -1
	p.chatRights = -1
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
	if p.target == nil && p.faceEntity != -1 {
		p.masks |= p.entitymask
		p.faceEntity = -1
	}
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
