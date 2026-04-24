package world

import (
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

// playerQueueRequest is one queued fresh-run script request with a
// single int arg. Queue entries are processed in processPlayerQueue;
// when Delay reaches zero (or below) the target script runs as a brand-
// new ScriptState. Type selects the queue variant (NORMAL/WEAK/LONG/
// STRONG); STRONG fires even when the player is delayed, the others
// wait for idle.
//
// As of S6h, Script holds the pre-resolved *ScriptFile directly. ID →
// ScriptFile resolution happens at enqueue time via Player.EnqueueScriptTyped;
// engine-dispatch paths (e.g. changeStat) use Player.EnqueueScriptFile.
type playerQueueRequest struct {
	Script *script.ScriptFile
	Delay  int
	IntArg int
	Type   script.PlayerQueueType
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
func (p *Player) EnqueueScriptFile(sf *script.ScriptFile, delay, intArg int, qtype script.PlayerQueueType) {
	if sf == nil {
		return
	}
	p.queue = append(p.queue, playerQueueRequest{
		Script: sf,
		Delay:  delay,
		IntArg: intArg,
		Type:   qtype,
	})
}

// EnqueueScriptTyped implements script.ActivePlayer.EnqueueScriptTyped by
// resolving scriptID → *ScriptFile via scriptProvider.GetByID and
// delegating to EnqueueScriptFile. Silent no-op on missing script or
// unwired server — same observable contract as the pre-S6h impl, where
// processPlayerQueue's GetByID check served the same role.
//
// Resolution shifts from fire-time (pre-S6h) to enqueue-time (S6h).
// Same tick boundary in practice; simpler codepath.
func (p *Player) EnqueueScriptTyped(scriptID uint32, delay, intArg int, qtype script.PlayerQueueType) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	p.EnqueueScriptFile(p.client.server.scriptProvider.GetByID(scriptID), delay, intArg, qtype)
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
	if p.activeScript != nil && p.activeScript.Protect {
		return false
	}
	return true
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
	p.x = x
	p.z = z
	p.level = level
	p.tele = true
	p.jump = true
}

// Teleport moves the player to (x, z, level) and flags the client for a
// smooth teleport transition (tele without jump).
func (p *Player) Teleport(x, z, level int) {
	p.x = x
	p.z = z
	p.level = level
	p.tele = true
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
	p.EnqueueScriptFile(sf, 0, 0, script.QueueNormal)
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
	p.EnqueueScriptFile(sf, 0, 0, script.QueueNormal)
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

// SetInteractionScriptNpc implements script.ActivePlayer.
func (p *Player) SetInteractionScriptNpc(npc script.ActiveNpc, op int) {
	realNpc, ok := npc.(*Npc)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realNpc, op, -1)
}
