package world

import (
	"fmt"
	"strings"

	"github.com/zsrv/goscape/pkg/cache"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

// playerQueueRequest is one queued fresh-run script request carrying its
// caller-supplied parallel arg slices (IntArgs + StringArgs). Queue
// entries are processed in processPlayerQueue; when Delay reaches zero
// (or below) the target script runs as a brand-new ScriptState. Type
// selects the queue variant (NORMAL/WEAK/LONG/STRONG); STRONG fires
// even when the player is delayed, the others wait for idle.
//
// As of S6h, Script holds the pre-resolved *ScriptFile directly. ID →
// ScriptFile resolution happens at enqueue time via Player.EnqueueScriptArgs;
// engine-dispatch paths (e.g. changeStat) use Player.EnqueueScriptFile.
//
// As of NAI-26 Bundle 1, the single IntArg int field is widened to
// parallel IntArgs []int + StringArgs []string slices to match the TS
// PlayerQueueRequest.args ScriptArgument[] shape (TS
// Engine-TS/src/engine/entity/PlayerQueueRequest.ts:15). The widening
// is required for STRONGQUEUE's variadic popScriptArgs body
// (PlayerOps.ts:98) and LONGQUEUE's 2-element [logoutAction, arg]
// args array (PlayerOps.ts:179), neither of which fit a single-int field.
type playerQueueRequest struct {
	Script     *script.ScriptFile
	Delay      int
	IntArgs    []int
	StringArgs []string
	Type       script.PlayerQueueType
}

// SetDelayed marks the player as suspended for `ticks` ticks starting
// next tick, per the P_DELAY opcode contract: the player resumes at
// currentTick + 1 + ticks.
//
// No-op if the player is not wired to a server (e.g. in fixtures that
// create a player without calling newTestServer + wiring).
func (p *Player) SetDelayed(ticks int) {
	if p.client == nil || p.client.server == nil {
		return
	}
	p.delayed = true
	p.delayedUntil = p.client.server.currentTick + 1 + ticks
}

// EnqueueScriptFile appends a queued fresh-run request for a specific
// ScriptFile. Delay=0 fires on the next processPlayerQueue pass (subject
// to the STRONG/NORMAL gate). Nil sf is a silent no-op — engine
// dispatchers (e.g. changeStat) call GetByTrigger and may legitimately
// pass nil when no cache script is registered for the event.
//
// intArgs and stringArgs are the parallel-slice args the target script
// will read from its IntArgCount / StringArgCount-sized prelude slots
// (matches TS ScriptArgument[] shape per
// Engine-TS/src/engine/entity/PlayerQueueRequest.ts:15). nil/nil
// expresses "no args" — the TS-faithful default for engine-dispatch
// paths (TS Engine-TS/src/engine/entity/Player.ts:821 args=[] default).
func (p *Player) EnqueueScriptFile(sf *script.ScriptFile, delay int, intArgs []int, stringArgs []string, qtype script.PlayerQueueType) {
	if sf == nil {
		return
	}
	p.queue = append(p.queue, playerQueueRequest{
		Script:     sf,
		Delay:      delay,
		IntArgs:    intArgs,
		StringArgs: stringArgs,
		Type:       qtype,
	})
}

// clearWeakQueue removes every QueueWeak entry from p.queue, preserving
// relative order of remaining entries. Mirrors TS
// Player.weakQueue.clear() (Player.ts:743). Goscape unifies all queue
// types into p.queue with a Type discriminator, so "clear weak queue"
// becomes a filter on the Type field.
func (p *Player) clearWeakQueue() {
	out := p.queue[:0]
	for _, req := range p.queue {
		if req.Type != script.QueueWeak {
			out = append(out, req)
		}
	}
	p.queue = out
}

// EnqueueScriptArgs implements script.ActivePlayer.EnqueueScriptArgs by
// resolving scriptID → *ScriptFile via scriptProvider.GetByID and
// delegating to EnqueueScriptFile. Returns a non-nil error when the
// scriptID does not resolve to a registered script — mirrors TS
// PlayerOps.ts:103-105 throw shape ("Unable to find queue script: ${id}").
//
// NAI-26 Bundle 2: this implementation now returns a non-nil error
// when GetByID returns nil — TS-faithful to PlayerOps.ts:103-105
// (and the parallel sites in :127-129, :152-154, :175-177). Bundle 1
// shipped a placeholder body returning nil; the rollout of the error
// activation was deferred to Bundle 2 to keep the mechanical signature
// widening separate from the behavior change for review-surface
// isolation.
//
// Silent no-op on unwired server (p.client / p.client.server /
// p.client.server.scriptProvider nil) is preserved across both bundles
// — that path corresponds to test fixtures that don't wire a Server,
// not to a script-author error worth surfacing.
//
// Resolution shifts from fire-time (pre-S6h) to enqueue-time (S6h).
// Same tick boundary in practice; simpler codepath.
func (p *Player) EnqueueScriptArgs(scriptID uint32, delay int, intArgs []int, stringArgs []string, qtype script.PlayerQueueType) error {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return nil
	}
	sf := p.client.server.scriptProvider.GetByID(scriptID)
	if sf == nil {
		// NAI-26 Bundle 2: surfaces script-author errors that pre-NAI-26
		// silent-no-op masked. Mirrors TS PlayerOps.ts:103-105
		// (STRONGQUEUE), :127-129 (WEAKQUEUE), :152-154 (QUEUE),
		// :175-177 (LONGQUEUE) — all four queue handlers throw
		// `Unable to find queue script: ${scriptId}` when the
		// scriptProvider lookup fails.
		return fmt.Errorf("unable to find queue script: %d", scriptID)
	}
	p.EnqueueScriptFile(sf, delay, intArgs, stringArgs, qtype)
	return nil
}

// StoreActiveScript saves a Suspended ScriptState so the tick loop can
// resume it when the player's delay expires.
func (p *Player) StoreActiveScript(state *script.ScriptState) {
	p.activeScript = state
}

// ClearActiveScript discards any stored ScriptState. Called after
// Finished/Aborted runs and on logout cleanup.
func (p *Player) ClearActiveScript() {
	p.activeScript = nil
}

// Playtime implements script.ActivePlayer.Playtime. The playtime field
// is incremented in processIn each tick.
func (p *Player) Playtime() int { return int(p.playtime) }

// S5m: last-input queries. Return the matching Player field.
func (p *Player) LastItem() int       { return p.lastItem }
func (p *Player) LastSlot() int       { return p.lastSlot }
func (p *Player) LastUseItem() int    { return p.lastUseItem }
func (p *Player) LastUseSlot() int    { return p.lastUseSlot }
func (p *Player) LastTargetSlot() int { return p.lastTargetSlot }

// CamReset sends an OpCamReset wire packet to the client. Called by
// the CAM_RESET script opcode (e.g. from the LOGIN script).
func (p *Player) CamReset() {
	p.writeOut(gameserver.OpCamReset, nil)
}

// HintNpc sends a HINT_ARROW (type=1, NPC variant) wire packet to the
// client. Encodes 6 bytes matching TS HintArrowEncoder type=1 branch:
// p1(type=1), p2(nid), p2(0), p1(0). Called by the HINT_NPC (opcode
// 2028) script handler. Mirrors TS Player.hintNpc at Player.ts:2174-2176.
//
// Sibling encoder branches: (*Player).HintCoord (type=2..6, NAI-39),
// (*Player).HintPlayer (type=10, NAI-39), (*Player).HintStop
// (type=-1, NAI-39). Closes the partial-encoder follow-up from NAI-37.
func (p *Player) HintNpc(nid int) {
	payload := []byte{
		0x01,                      // p1: type = 1 (NPC hint)
		byte(nid >> 8), byte(nid), // p2: nid (big-endian)
		0x00, 0x00, // p2: 0 (unused playerSlot for type=1)
		0x00, // p1: 0 (unused y for type=1)
	}
	p.writeOut(gameserver.OpHintArrow, payload)
}

// HintCoord sends a HINT_ARROW (type=2..6, TILE variant) wire packet to
// the client. Encodes 6 bytes matching TS HintArrowEncoder type=2..6
// branch (HintArrowEncoder.ts:17-27): p1(offset), p2(x), p2(z),
// p1(height). Called by the HINT_COORD (opcode 2027) script handler.
// Mirrors TS Player.hintTile at Player.ts:2178-2180.
//
// Out-of-range offset (not in [2,6]) is TS-faithful: the wire packet
// is emitted with the offset as byte[0]. Script-authors are responsible
// for offset bounds; the entity-method does not validate.
func (p *Player) HintCoord(offset, x, z, height int) {
	payload := []byte{
		byte(offset),          // p1: type = offset (2..6)
		byte(x >> 8), byte(x), // p2: x (big-endian)
		byte(z >> 8), byte(z), // p2: z (big-endian)
		byte(height), // p1: height
	}
	p.writeOut(gameserver.OpHintArrow, payload)
}

// HintPlayer sends a HINT_ARROW (type=10, PL variant) wire packet to
// the client. Encodes 6 bytes matching TS HintArrowEncoder type=10
// branch (HintArrowEncoder.ts:28-32): p1(10), p2(slot), p2(0), p1(0).
// Called by the HINT_PL (opcode 2029) script handler. Mirrors TS
// Player.hintPlayer at Player.ts:2182-2184.
func (p *Player) HintPlayer(slot int) {
	payload := []byte{
		0x0A,                        // p1: type = 10 (player hint)
		byte(slot >> 8), byte(slot), // p2: slot (big-endian)
		0x00, 0x00, // p2: 0
		0x00, // p1: 0
	}
	p.writeOut(gameserver.OpHintArrow, payload)
}

// HintStop sends a HINT_ARROW (type=-1, STOP variant) wire packet to
// the client, clearing any active hint arrow. Encodes 6 bytes matching
// TS HintArrowEncoder type=-1 branch (HintArrowEncoder.ts:33-38):
// p1(-1), p2(0), p2(0), p1(0). p1(-1) on the wire is 0xFF (low byte of
// two's-complement). Called by the HINT_STOP (opcode 2030) script
// handler. Mirrors TS Player.stopHint at Player.ts:2186-2188.
func (p *Player) HintStop() {
	payload := []byte{
		0xFF,       // p1: type = -1 (stop sentinel; two's-complement low byte)
		0x00, 0x00, // p2: 0
		0x00, 0x00, // p2: 0
		0x00, // p1: 0
	}
	p.writeOut(gameserver.OpHintArrow, payload)
}

// StaffModLevel is provided by player_source.go (returns int32 per
// rsbuf.PlayerSource). Re-used here to satisfy script.ActivePlayer.

// UID implements script.ActivePlayer.UID. Returns the persistent
// account uid captured during login.
func (p *Player) UID() int { return p.uid }

// CanAccess implements script.ActivePlayer.CanAccess — the P_FINDUID
// protected-binding gate. False when delayed, when a modal main/chat
// is open, or when a suspended protected script is stored. Mirrors TS
// Player.canAccess at Engine-TS/src/engine/entity/Player.ts:805-812.
//
// The World-shutdown early-return from TS is omitted — goscape has
// no global shutdown flag to consult and rejects lookups uniformly.
//
// The third branch derives what TS expresses as a single Player.protect
// bool from activeScript.Protect. They are equivalent: TS persists the
// flag onto the player at script suspension (Player.ts:2141) and clears
// it at script completion (:2103-2114), so "is the player in a stored
// protected script?" and "is the player-level protect flag set?" are
// the same condition — goscape just reads it from the stored state
// instead of a redundant bool field.
func (p *Player) CanAccess() bool {
	if p.delayed {
		return false
	}
	if p.modalState&(modalStateMain|modalStateChat) != 0 {
		return false
	}
	if p.protectedScriptActive() {
		return false
	}
	return true
}

// protectedScriptActive reports whether the player currently owns a
// suspended protected script — goscape's mapping of TS Player.protect.
// Used by CanAccess (above) and processWalktrigger to gate operations
// that TS guards with !this.protect. See the CanAccess doc-comment for
// the activeScript.Protect ↔ TS Player.protect equivalence rationale.
// NAI-52.
func (p *Player) protectedScriptActive() bool {
	return p.activeScript != nil && p.activeScript.Protect
}

// Varp implements script.ActivePlayer.Varp.
func (p *Player) Varp(id int) int32 {
	if id < 0 || id >= len(p.varps) {
		return 0
	}
	return p.varps[id]
}

// SetVarp implements script.ActivePlayer.SetVarp. Writes the server-
// side value then wire-sends via VARP_SMALL / VARP_LARGE if the varp
// type is transmit=true.
func (p *Player) SetVarp(id int, val int32) {
	if id < 0 || id >= len(p.varps) {
		return
	}
	p.varps[id] = val
	p.writeVarp(id, val)
}

// S5c: position / facing / teleport, stats, and animation.

// CoordPacked returns the player's current position as a single RS2 coord
// int: (level<<28) | (x<<14) | z. Used by the COORD opcode.
func (p *Player) CoordPacked() int {
	return (p.level << 28) | (p.x << 14) | p.z
}

// TeleJump instantly teleports the player to (x, z, level) with no
// interpolation, clearing any pending walk. ResetMasks clears the one-
// shot tele/jump flags after emission.
func (p *Player) TeleJump(x, z, level int) {
	prevX, prevZ, prevLevel := p.x, p.z, p.level
	p.x = x
	p.z = z
	p.level = level
	p.tele = true
	p.jump = true
	refreshPlayerZone(p, prevX, prevZ, prevLevel)
}

// Teleport moves the player to (x, z, level) and flags the client for a
// smooth teleport transition (tele without jump in the same-level case;
// tele+jump+INSTANT speed when crossing levels). Mirrors TS
// PathingEntity.teleport at PathingEntity.ts:267-298.
//
// NAI-36-T7 closes deviations D1 (level clamp), D2 (unallocated-zone
// reject), order (refresh BEFORE tele=true), and D5 (level-change INSTANT
// + jump branch) for Player. Residual: D3 (focus orientation), D4
// (lastStepX/Z adjust). See DEVIATION block at npc_script.go for the full
// tracker.
func (p *Player) Teleport(x, z, level int) {
	// D1: clamp level to [0, 3] per PathingEntity.ts:268-271.
	if level < 0 {
		level = 0
	} else if level > 3 {
		level = 3
	}
	// D2: reject teleports to unallocated zones per PathingEntity.ts:273-278.
	// (TS additionally exempts staffModLevel >= 3; goscape has no staff-mod
	// flag yet, so the gate is unconditional. messageGame on rejection is
	// a future polish item.)
	if p.client != nil && p.client.server != nil &&
		!p.client.server.IsZoneAllocated(level, x, z) {
		return
	}

	prevX, prevZ, prevLevel := p.x, p.z, p.level
	p.x = x
	p.z = z
	p.level = level

	// Order: refreshPlayerZone runs BEFORE p.tele = true to match TS
	// PathingEntity.ts:290-293. The two writes are functionally
	// commutative (refresh reads only previous coords + current
	// x/z/level; the tele bit is independent), but TS-faithful order is
	// the project's true-to-TS gate default.
	refreshPlayerZone(p, prevX, prevZ, prevLevel)
	p.tele = true

	// D5: level-change → INSTANT + jump per PathingEntity.ts:295-298.
	if prevLevel != level {
		p.moveSpeed = MoveSpeedInstant
		p.jump = true
	}
}

// FaceSquare rotates the player to face the square at absolute (x, z)
// on the current level. Wire coords are doubled+1 (face-center).
func (p *Player) FaceSquare(x, z int) {
	p.faceSquareX = x*2 + 1
	p.faceSquareZ = z*2 + 1
	p.masks |= rsbuf.MaskFaceCoord
}

// statBounds bounds-checks a skill id against the 21-skill array range.
func statBounds(id int) bool { return id >= 0 && id < 21 }

// Stat returns the player's current (possibly boosted/drained) level for
// skill id. Returns 0 on OOB.
func (p *Player) Stat(id int) int {
	if !statBounds(id) {
		return 0
	}
	return int(p.levels[id])
}

// StatBase returns the player's base (unboosted) level for skill id.
// Returns 0 on OOB.
func (p *Player) StatBase(id int) int {
	if !statBounds(id) {
		return 0
	}
	return int(p.baseLevels[id])
}

// StatXP returns the player's accumulated XP for skill id as a scaled
// integer (authentic: XP * 10). Returns 0 on OOB.
func (p *Player) StatXP(id int) int {
	if !statBounds(id) {
		return 0
	}
	return int(p.stats[id])
}

// SetCurLevel overrides the player's current level for skill id, clamped
// to [0, 255]. OOB ids are dropped silently. The existing updateStats()
// diff against lastLevels picks up the change and emits UpdateStat.
func (p *Player) SetCurLevel(id int, level int) {
	if !statBounds(id) {
		return
	}
	if level < 0 {
		level = 0
	} else if level > 255 {
		level = 255
	}
	p.levels[id] = uint8(level)
}

// changeStat fires the [changestat,<skill>] trigger for the given stat
// slot when a cache script is registered. Enqueued as QueueNormal so it
// runs asynchronously through processPlayerQueue, not inline with the
// triggering action. Matches TS Player.changeStat (Player.ts:1816-1821)
// which uses PlayerQueueType.ENGINE.
//
// QueueNormal is goscape's closest available approximation: it runs on
// the next tick (matching ENGINE's async semantic) but is gated by the
// shared queue's STRONG-override delayed-player check, whereas TS's
// engineQueue uses canAccess() at fire time. A dedicated QueueEngine
// variant is a deferred follow-up — fine until a consumer needs the
// distinction.
//
// Silent no-op if no script is registered (GetByTrigger returns nil →
// EnqueueScriptFile's nil-check short-circuits). Called from AddXP's
// level-up branch.
func (p *Player) changeStat(stat int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	sf := p.client.server.scriptProvider.GetByTrigger(script.TriggerChangeStat, stat, -1)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueNormal)
}

// advanceStat fires the [advancestat,<skill>] trigger for the given stat
// slot when a cache script is registered for that exact stat. Unlike
// changeStat (which uses the 3-level fallback via GetByTrigger), this
// uses GetByTriggerSpecific — type-specific only, no category or global
// fallback. A global [advancestat,_] script would be wrong here: cache
// scripts that say "Congratulations, you just advanced an Attack level!"
// must be skill-keyed.
//
// Enqueued as QueueNormal so it runs asynchronously through
// processPlayerQueue. Matches TS Player.ts:1804-1807 exactly.
//
// Silent no-op if no specific script is registered (GetByTriggerSpecific
// returns nil → EnqueueScriptFile's nil-check short-circuits). Called
// from AddXP's level-up branch after changeStat.
func (p *Player) advanceStat(stat int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	sf := p.client.server.scriptProvider.GetByTriggerSpecific(script.TriggerAdvanceStat, stat, -1)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueNormal)
}

// AddXP adds xp (scaled ×10) to the player's stored XP for skill id and
// recomputes baseLevels from the XP curve. Matches TS Player.advanceStat
// (Player.ts:1752-1772) in three branches:
//
//   - Un-buffed (levels[id] == baseLevels[id]): advance BOTH levels and
//     baseLevels together. This is the common case — every fresh-player
//     training session. TS line 1760-1763.
//   - Buffed (levels[id] > baseLevels[id]): update baseLevels only;
//     preserve the buff on levels. Level-ups don't strip active potions.
//   - Drained (levels[id] < baseLevels[id]): update baseLevels; on
//     level-up replenish levels by the level delta. TS line 1767-1770.
//
// XP is clamped at objtype.MaxXP (200m real, stored as 2B ×10). Negative
// xp is clamped to keep stats[id] >= 0 defensively — deviation from TS
// where a bug could reduce stored XP. Matches the convention from
// Player.Damage / *Npc.Damage negative-amount clamps.
//
// On level-up (baseLevels increases), fires the [changestat,<skill>] trigger
// via changeStat (TS Player.ts:1772) then the [advancestat,<skill>] trigger
// via advanceStat (TS Player.ts:1804-1807). Does NOT recompute combat
// level (future combat sub-spec) or emit session-log / milestone events
// (TS Player.ts:1773-1803; session-log infrastructure not yet ported).
func (p *Player) AddXP(id int, xp int) {
	if !statBounds(id) {
		return
	}
	next := min(int64(p.stats[id])+int64(xp), int64(objtype.MaxXP))
	if next < 0 {
		next = 0
	}
	beforeBase := int(p.baseLevels[id])
	p.stats[id] = int32(next)
	newBase := objtype.GetLevelByExp(int(p.stats[id]))

	// Un-buffed branch: advance levels in lockstep with baseLevels so a
	// fresh-player level-up is visible on the stat display. TS Player.ts:1760-1763.
	if int(p.levels[id]) == beforeBase {
		p.levels[id] = uint8(newBase)
	}
	p.baseLevels[id] = uint8(newBase)
	afterBase := newBase

	if afterBase > beforeBase && int(p.levels[id]) < beforeBase {
		// Drained + level-up: replenish levels by the level delta.
		// Matches TS Player.ts:1767-1770.
		p.levels[id] = uint8(min(int(p.levels[id])+(afterBase-beforeBase), 255))
	}
	if afterBase > beforeBase {
		// Level-up: fire [changestat,<skill>] then [advancestat,<skill>]
		// triggers if registered. Matches TS Player.ts:1772, 1804-1807.
		p.changeStat(id)
		p.advanceStat(id)
	}
}

// PlayAnim schedules sequence seqID with the given client-side delay on
// the player's primary animation slot. seqID=-1 clears.
func (p *Player) PlayAnim(seqID, delay int) {
	p.animID = seqID
	p.animDelay = delay
	p.masks |= rsbuf.MaskAnim
}

// PlaySpotAnim schedules a graphic (spotanim) on the player at the given
// height with the given client-side delay. id=-1 clears.
func (p *Player) PlaySpotAnim(id, height, delay int) {
	p.spotanimID = id
	p.spotanimHeight = height
	p.spotanimDelay = delay
	p.masks |= rsbuf.MaskSpotAnim
}

// SetAppearanceInv stores id on Player.appearanceInv and flags
// MaskAppearance. Mirrors TS Player.buildAppearance (the literal
// two-liner at Engine-TS/src/engine/entity/Player.ts:1836-1839). The
// mask triggers generateAppearance regeneration on the next tick in
// tick.go:325-335.
func (p *Player) SetAppearanceInv(id int) {
	p.appearanceInv = id
	p.masks |= rsbuf.MaskAppearance
}

// SetReadyAnim sets the player's idle/stand animation. BAS anims are
// persistent and flow through the appearance buffer, which regenerates
// on MaskAppearance — no per-call mask flip needed.
func (p *Player) SetReadyAnim(seqID int) { p.readyanim = seqID }

// SetTurnAnim sets the player's turn-in-place animation.
func (p *Player) SetTurnAnim(seqID int) { p.turnanim = seqID }

// SetWalkAnim sets the player's forward-walk animation.
func (p *Player) SetWalkAnim(seqID int) { p.walkanim = seqID }

// SetWalkAnimB sets the player's backward-walk animation.
func (p *Player) SetWalkAnimB(seqID int) { p.walkanim_b = seqID }

// SetWalkAnimL sets the player's strafe-left walk animation.
func (p *Player) SetWalkAnimL(seqID int) { p.walkanim_l = seqID }

// SetWalkAnimR sets the player's strafe-right walk animation.
func (p *Player) SetWalkAnimR(seqID int) { p.walkanim_r = seqID }

// SetRunAnim sets the player's run animation.
func (p *Player) SetRunAnim(seqID int) { p.runanim = seqID }

// WalkTrigger implements script.ActivePlayer.WalkTrigger. Returns the
// queued walktrigger script id, or -1 if none. NAI-51.
func (p *Player) WalkTrigger() int { return p.walktrigger }

// SetWalkTrigger implements script.ActivePlayer.SetWalkTrigger. Stores
// scriptID in p.walktrigger. -1 clears. NAI-51.
func (p *Player) SetWalkTrigger(scriptID int) { p.walktrigger = scriptID }

// S5f: interface / modal control.
//
// Modal mutex rules mirror LostCityRS/Engine-TS Player.ts:1928-2022:
//   - openMainModal  closes CHAT + SIDE.
//   - openChatModal  closes MAIN + SIDE.
//   - openSideModal  closes MAIN + CHAT.
//   - openMainSideModal closes CHAT (keeps main/side by definition).
//
// All methods set refreshModal so the next encodeOut() emits the matching
// IF_OPEN* (and any IF_CLOSE) packets.

// CloseModal clears every modal slot and flags the client to emit
// IF_CLOSE on the next encodeOut pass.
func (p *Player) CloseModal() {
	p.modalMain = -1
	p.modalChat = -1
	p.modalSide = -1
	p.modalState = modalStateNone
	p.refreshModalClose = true
}

// OpenMain opens com as the main modal. Per TS, opening main closes any
// currently-open chat and side modals.
func (p *Player) OpenMain(com int) {
	p.modalMain = com
	p.modalChat = -1
	p.modalSide = -1
	p.modalState = modalStateMain
	p.refreshModal = true
}

// OpenChat opens com as the chat modal. Per TS, opening chat closes any
// currently-open main and side modals.
func (p *Player) OpenChat(com int) {
	p.modalMain = -1
	p.modalChat = com
	p.modalSide = -1
	p.modalState = modalStateChat
	p.refreshModal = true
}

// OpenSide opens com as the side modal. Per TS, opening side closes any
// currently-open main and chat modals.
func (p *Player) OpenSide(com int) {
	p.modalMain = -1
	p.modalChat = -1
	p.modalSide = com
	p.modalState = modalStateSide
	p.refreshModal = true
}

// OpenMainSide opens mainCom as the main modal and sideCom as the side
// modal simultaneously. Per TS, this closes any currently-open chat modal.
func (p *Player) OpenMainSide(mainCom, sideCom int) {
	p.modalMain = mainCom
	p.modalChat = -1
	p.modalSide = sideCom
	p.modalState = modalStateMain | modalStateSide
	p.refreshModal = true
}

// SetResumeButtons stores the 5 resume-button interface ids for later
// consumption by P_PAUSEBUTTON. No wire op is emitted.
func (p *Player) SetResumeButtons(b1, b2, b3, b4, b5 int) {
	p.resumeButtons = [5]int{b1, b2, b3, b4, b5}
}

// S5g: dialog suspension.

func (p *Player) LastCom() int { return p.lastCom }

func (p *Player) SendCountDialog() {
	p.writeOut(gameserver.OpPCountDialog, nil)
}

// S5h: action-clear.

// StopAction implements script.ActivePlayer.StopAction. Clears any
// anchored interaction target plus any pending-action state (modals,
// interaction kind). Walk queue is preserved.
func (p *Player) StopAction() {
	p.ClearInteraction()
	p.ClearPendingAction()
}

// ClearPendingAction implements script.ActivePlayer.ClearPendingAction.
// Resets interaction kind/target/op to idle and closes any open modal.
// Walk queue is preserved.
func (p *Player) ClearPendingAction() {
	p.interactionKind = InteractionEngine
	p.target = nil
	p.targetOp = -1
	p.CloseModal()
}

// SetApRange implements script.ActivePlayer.SetApRange. Sets apRange
// and marks apRangeCalled=true in a single call (tick-serialized by
// the engine; no lock needed) to persist the interaction past the
// current tick. Matches TS PlayerOps.ts:P_APRANGE.
func (p *Player) SetApRange(n int) {
	p.apRange = n
	p.apRangeCalled = true
}

// TargetSubjectCom implements script.ActivePlayer.TargetSubjectCom.
// Returns p.targetSubject.com which was set by OpLocT's SetInteraction
// call (spellCom) or -1 for non-com callers.
func (p *Player) TargetSubjectCom() int { return p.targetSubject.com }

// InvListenOnCom implements script.ActivePlayer. Thin wrapper
// delegating to the internal unexported method landed in S6p-2.
func (p *Player) InvListenOnCom(invType, com, source int) {
	p.invListenOnCom(invType, com, source)
}

// InvStopListenOnCom implements script.ActivePlayer. Thin wrapper
// delegating to the internal unexported method landed in S6p-2.
func (p *Player) InvStopListenOnCom(com int) {
	p.invStopListenOnCom(com)
}

// SetInteractionScriptLoc implements script.ActivePlayer. Type-asserts
// the narrow script.ActiveLoc back to *entity.Loc and anchors the
// player with trigger ApLoc<op> + InteractionScript. Matches TS
// PlayerOps.ts P_OPLOC terminal setInteraction. Silently no-ops if the
// loc isn't a real *entity.Loc (defensive — only goscape's OPLOC
// routing sets ScriptState.ActiveLoc with this concrete type).
func (p *Player) SetInteractionScriptLoc(loc script.ActiveLoc, op int) {
	realLoc, ok := loc.(*entitypkg.Loc)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realLoc, op, -1)
}

// SetAnimProtect implements script.ActivePlayer.SetAnimProtect. Stores the
// anim-protect flag; when nonzero, in-engine animation requests should be
// suppressed (reader path unported — S7b-D1; paid down when anim playback
// is ported). Matches TS Player.ts:321 (field) + PlayerOps.ts:1171-1172.
func (p *Player) SetAnimProtect(v int) { p.animProtect = v }

// SetAllowDesign implements script.ActivePlayer.SetAllowDesign. Stores the
// flag; ALLOWDESIGN (opcode 2001) is the sole writer. Reader path unported
// per S7e-D1.
func (p *Player) SetAllowDesign(v bool) { p.allowDesign = v }

// SetInteractionScriptNpc implements script.ActivePlayer.
func (p *Player) SetInteractionScriptNpc(npc script.ActiveNpc, op int) {
	realNpc, ok := npc.(*Npc)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realNpc, op, -1)
}

// LowMemory returns the player's low-memory flag as plumbed from the
// RS2 login request (req.LowMemory) through client.lowMemory and
// copied onto the Player at newPlayer().
func (p *Player) LowMemory() bool { return p.lowMemory }

// NAI-47: SETIDKIT appearance mutation.
func (p *Player) Gender() int                  { return p.gender }
func (p *Player) SetBodyPart(slot, idkit int)  { p.body[slot] = idkit }
func (p *Player) SetColorPart(slot, color int) { p.colors[slot] = color }

// normalizeSongName mirrors TS Player.playSong's normalization step
// (Engine-TS/src/engine/entity/Player.ts:1903) — lowercase + spaces
// replaced by underscores. Extracted for direct testability given
// PlaySong's current no-op write body (S7h-D1). Asymmetric with
// normalizeJingleName (spaces→underscores vs. underscores→spaces);
// the asymmetry is TS-intentional — songs key into disk with
// underscore filenames; jingles key into a space-separated title map.
func normalizeSongName(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), " ", "_")
}

// PlaySong normalizes the song name per TS Player.playSong
// (Engine-TS/src/engine/entity/Player.ts:1902-1914), looks up the
// preloaded blob + CRC, and writes MidiSong to the client. Silent
// no-op on empty name or missing PRELOADED entry (mirrors TS's
// `if (song && crc)` guard at Player.ts:1910).
//
// NAI-16 retires S7h-D1: the PRELOADED lookup and MidiSong write are
// now wired. TestPlaySongWritesOut is the positive-pin; the miss-path
// pins (TestPlaySong*ReturnsSilently) verify the silent-no-op guards.
func (p *Player) PlaySong(name string) {
	name = normalizeSongName(name)
	if name == "" {
		return
	}
	key := name + ".mid"
	song, okSong := cache.Preloaded[key]
	crc, okCRC := cache.PreloadedCRC[key]
	if !okSong || !okCRC {
		return
	}
	buf := packet.NewPacket(make([]byte, 0, len(name)+10))
	encodeMidiSong(buf, name, crc, uint32(len(song)))
	p.writeOut(gameserver.OpMidiSong, buf.Bytes())
}

// normalizeJingleName mirrors TS Player.playJingle's normalization step
// (Engine-TS/src/engine/entity/Player.ts:1917) — lowercase + underscores
// replaced by spaces. Extracted for direct testability given
// PlayJingle's current no-op write body (S7h-D1). Asymmetric with
// normalizeSongName (underscores→spaces vs. spaces→underscores);
// the asymmetry is TS-intentional — jingles key into a space-separated
// title map; songs key into underscore-filename disk paths.
func normalizeJingleName(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), "_", " ")
}

// PlayJingle normalizes the jingle name per TS Player.playJingle
// (Engine-TS/src/engine/entity/Player.ts:1916-1926), looks up the
// preloaded blob, and writes MidiJingle to the client. Silent no-op
// on empty name or missing PRELOADED entry (mirrors TS's `if (jingle)`
// guard at Player.ts:1923).
//
// NAI-16 retires S7h-D1 (jingle side). TestPlayJingleWritesOut pins
// the positive path; TestPlayJingleMissingFromPreloadedReturnsSilently
// pins the silent-no-op guard.
func (p *Player) PlayJingle(delay int, name string) {
	name = normalizeJingleName(name)
	if name == "" {
		return
	}
	jingle, ok := cache.Preloaded[name+".mid"]
	if !ok {
		return
	}
	buf := packet.NewPacket(make([]byte, 0, 2+len(jingle)))
	encodeMidiJingle(buf, uint16(delay), jingle)
	p.writeOut(gameserver.OpMidiJingle, buf.Bytes())
}
