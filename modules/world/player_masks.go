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

// UnsetMapFlag clears the player's map-click destination by sending the
// matching client packet. Mirrors TS Player.unsetMapFlag (called by
// P_EXACTMOVE at PlayerOps.ts:888 and by adjacent server-script paths).
// Thin wrapper over the package-local helper. NAI-160 T4.
func (p *Player) UnsetMapFlag() {
	sendUnsetMapFlag(p)
}

// ResetMasks clears mask bits + ephemeral per-tick state. Mirrors TS
// PathingEntity.resetPathingEntity (PathingEntity.ts:577-615) plus the
// Player-only fields TS resets in Player.resetEntity (Player.ts:454-467,
// non-respawn branch).
//
// Persistent-by-design (TS resets, goscape preserves):
//   - levels[Hitpoints] / baseLevels[Hitpoints] (S6e promotion to
//     persistent via the skill arrays).
//
// faceSquareX/Z ARE reset each tick below (D1), matching TS
// PathingEntity.ts:608-609. The earlier "non-symptomatic persistence"
// claim was wrong: the rsbuf low-def force-emits FACE_COORD reading
// effectiveFaceCoord(), so a persisted square leaked to newly-visible
// observers after the entity walked away. effectiveFaceCoord() falls back
// to faceAngle (kept current by unfocus-on-spawn + M2's per-step focus).
//
// Handled-elsewhere (NOT in ResetMasks; equivalent goscape paths):
//   - walkDir/runDir — reset/set in movement.go:53-65 per movement step.
//   - stepsTaken — reset in movement.go:46 (pinned by
//     TestResolveMovementResetsStepsTaken at movement_test.go:214).
//   - lastTickX/Z + lastLevel — set in movement.go:48-50 per movement step.
//   - socialProtect/reportAbuseProtect — reset in tick.go:538-539 (NAI-72).
//
// interacted/apRangeCalled ARE reset each tick below (interaction-6),
// mirroring TS PathingEntity.ts:587-588 and the NPC sibling at
// npc_masks.go:277. The apRangeCalled half is load-bearing: it has
// production readers at interaction.go:431 (`else if interacted &&
// !p.apRangeCalled` → auto-clear) and interaction.go:572 (`if
// p.nextTarget == nil && p.apRangeCalled` → NAI-69 same-tick retry).
// Without this reset, a stale-true apRangeCalled carried from a prior
// tick's p_aprange call suppressed the auto-clear on the next tick that
// fired via the OP path (which, unlike the AP path, never resets
// apRangeCalled), leaving the player stuck on a target an extra tick.
// The interacted reset is TS-parity cosmetic — the field has no
// goscape control-flow reader (processInteraction uses a fresh local
// `interacted` at interaction.go:344); but resetting matches TS
// structure and the NPC sibling pattern.
//
// Also clears one-shot movement intents (tele, jump) so a single-tick
// teleport emission doesn't repeat next tick.
//
// The trailing-clear mirrors TS PathingEntity.ts:611-614 byte-for-byte:
// both engines arm the entitymask bit at TICK END (Go's ResetMasks via
// tick.go processCleanup; TS's resetPathingEntity via World.ts:1138
// World.processCleanup which itself runs after processClientsOut at
// World.ts:1122). The armed mask is consumed by the NEXT tick's info-
// pass in both engines — identical timing, identical observable
// behavior. Closes NAI-91's "player keeps facing NPC after walking
// away" smoke residual.
//
// (Investigated 2026-06-01: the prior comment claimed a "1-tick lag
// deviation vs TS which fires at tick start" but TS resetPathingEntity
// is called from processCleanup at tick end — not tick start — so no
// lag exists. See docs/PORTING-CLOSED.md NAI-91 closure row.)
func (p *Player) ResetMasks() {
	p.masks = 0
	p.tele = false
	p.jump = false
	// interaction-6: mirror TS PathingEntity.ts:587-588 + NPC sibling
	// at npc_masks.go:277. See doc-block above for rationale.
	p.interacted = false
	p.apRangeCalled = false
	// Mirrors TS PathingEntity.resetPathingEntity at PathingEntity.ts:578.
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
	// Reset the primary animation slot at tick end (TS PathingEntity.ts:598-601).
	// processInfo already emitted any MaskAnim earlier this tick, so clearing
	// here lets the next tick's PlayAnim set a fresh animation; without it the
	// priority guard rejects an equal-priority repeat and the anim (emote, bow
	// draw, bury-bones) plays only once per session.
	p.animID = -1
	p.animDelay = -1
	p.spotanimID = -1
	p.spotanimHeight = -1
	p.spotanimDelay = -1
	// D1: reset faceSquare each tick (TS PathingEntity.ts:608-609). FACE_COORD is
	// a one-tick event: processInfo already read FaceSquareX()/Z() (→
	// effectiveFaceCoord) for this tick's payload, so clearing here makes
	// effectiveFaceCoord fall back to faceAngle next tick. Without this a square
	// set by FaceSquare/setInteraction(engine) the player then walks away from
	// would render stale to every newly-visible observer (the rsbuf low-def
	// force-emits FACE_COORD). faceAngle stays valid: unfocus() on spawn sets it
	// south, and M2's per-step focus keeps it pointing where the player walks.
	p.faceSquareX = -1
	p.faceSquareZ = -1
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
	// TS Player.ts:460 — this.protect = false in resetEntity. Defensive
	// tick-end clear of the Player-level protect gate. A suspended
	// protected script's resume cycle next tick re-establishes the gate
	// via runScript entry; during the wait window the player is ungated,
	// matching TS observable behavior. NAI-111-D1.
	p.protect = false
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
// This is a pure output op — no death / auto-retaliate / aggro logic,
// matching TS Player.applyDamage (which is itself called only from the
// DAMAGE script handler at PlayerOps.ts:778). Player death is fully
// content-script driven in TS — there is no engine-side death trigger
// (ServerTriggerType.ts enumerates 167 triggers, zero death-specific).
// Content scripts detect HP=0 via STAT(0) and drive death via AI_QUEUE
// triggers; goscape mirrors this faithfully.
func (p *Player) Damage(amount, dmgType int) {
	if amount < 0 {
		amount = 0
	}
	current := int(p.levels[objtype.PlayerStatHitpoints])
	p.damageAmt = min(amount, current)
	p.damageType = dmgType
	next := max(current-amount, 0)
	p.levels[objtype.PlayerStatHitpoints] = uint8(next)
	p.masks |= rsbuf.MaskDamage
}

// ResetHP restores levels[Hitpoints] to baseLevels[Hitpoints] — the
// player's "full HP" state. Boost/drain effects on Hitpoints are wiped
// (RS2 convention: respawn fills to base, not boosted-max).
//
// No direct TS counterpart — TS performs HP refill inline within
// content-driven death/respawn scripts. This Go-side helper exists so
// the eventual content-driven respawn opcode wiring has a single reusable
// seam; currently has no production callers (test-only invocation pins
// the body shape).
func (p *Player) ResetHP() {
	p.levels[objtype.PlayerStatHitpoints] = p.baseLevels[objtype.PlayerStatHitpoints]
}

// HeadIcons / SetHeadIcons expose the headicons field for the
// HEADICONS_GET / HEADICONS_SET RuneScript handlers. Mirrors TS direct
// read/write at PlayerOps.ts:980-986. The encoder at
// modules/world/appearance.go:65 (`buf.P1(uint8(p.headicons))`) does
// byte-truncation downstream, matching TS Player.ts:1314
// `stream.p1(this.headicons)`. NAI-160 T2/T3.
func (p *Player) HeadIcons() int { return p.headicons }

// SetHeadIcons writes the validated head-icon bitmask. NumberNotNull
// gating is the handler's responsibility (handleHeadIconsSet at
// pkg/script/handlers_player.go).
func (p *Player) SetHeadIcons(v int) { p.headicons = v }
