package world

import (
	"fmt"
	"strings"

	"github.com/zsrv/goscape/pkg/cache"
	"github.com/zsrv/goscape/pkg/coordgrid"
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

// OnScriptFinishedOrAborted handles the Finished/Aborted post-Execute
// tail for a player-anchored script. If state is the player's current
// activeScript, nulls it; and if no MAIN modal is open, fires
// CloseModal(false) to auto-close any open chat / side dialogue.
//
// Mirrors TS Player.executeScript Finished/Aborted tail
// (Player.ts:2143-2148). The match-guard preserves a Suspended /
// PauseButton / CountDialog activeScript when a different fresh script
// Finishes on the same player in the same tick. The MAIN-bit gate on
// CloseModal preserves any open main modal while dropping chat /
// side dialogues — TS comment: "close chat dialogues automatically
// and leave main modals alone".
//
// NAI-54 T1.
func (p *Player) OnScriptFinishedOrAborted(state *script.ScriptState) {
	if p.activeScript != state {
		return
	}
	p.activeScript = nil
	if p.modalState&modalStateMain == modalStateNone {
		p.CloseModal(false)
	}
}

// Playtime implements script.ActivePlayer.Playtime. The playtime field
// is incremented in processIn each tick.
func (p *Player) Playtime() int { return int(p.playtime) }

// LastMovement returns the player's lastMovement field. See the
// pkg/script.ActivePlayer.LastMovement docstring for semantics.
func (p *Player) LastMovement() int { return p.lastMovement }

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
// bool from activeScript.Pointers&PtrProtectedActivePlayer. They are equivalent: TS persists the
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
// the activeScript.Pointers&PtrProtectedActivePlayer ↔ TS Player.protect equivalence rationale.
// NAI-52.
func (p *Player) protectedScriptActive() bool {
	return p.activeScript != nil && p.activeScript.Pointers&script.PtrProtectedActivePlayer != 0
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

// VarpString implements script.ActivePlayer.VarpString. Returns the
// STRING-typed per-player var at id, or "" on OOB / unsized slice.
func (p *Player) VarpString(id int) string {
	if id < 0 || id >= len(p.varpsString) {
		return ""
	}
	return p.varpsString[id]
}

// SetVarpString implements script.ActivePlayer.SetVarpString. OOB
// silently dropped (slice sized to varpTypes.Configs at login). No
// wire-send: this protocol revision has no varp_string opcode.
func (p *Player) SetVarpString(id int, val string) {
	if id < 0 || id >= len(p.varpsString) {
		return
	}
	p.varpsString[id] = val
}

// SetRun implements script.ActivePlayer.SetRun. Writes the run-mode
// toggle (0=walk, 1=run) to the player's run field. Mirrors TS field
// write at PlayerOps.ts:1205. Backs the P_RUN opcode handler. NAI-117.
func (p *Player) SetRun(v int) {
	p.run = v
}

// RunVarpID implements script.ActivePlayer.RunVarpID. Returns the varp
// id discovered at config-load time as the engine run-mode varp (the
// config with ClientCode==7). Mirrors TS VarPlayerType.RUN at
// Engine-TS/src/cache/config/VarPlayerType.ts:50-53. Returns 0 (goscape
// defensive; TS skips this check) if the server has no varpTypes loaded
// (test-fixture / pre-config-load).
func (p *Player) RunVarpID() int {
	if p.client == nil || p.client.server == nil || p.client.server.varpTypes == nil {
		return 0
	}
	return p.client.server.varpTypes.RunID
}

// RunEnergy implements script.ActivePlayer.RunEnergy. Returns the
// player's current run-energy as an int (range [0, 10000]). Backs the
// RUNENERGY opcode handler. NAI-117.
func (p *Player) RunEnergy() int {
	return p.runenergy
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
// NAI-36-T7 closed D1 (level clamp), D2 (unallocated-zone reject), order
// (refresh BEFORE tele=true), and D5 (level-change INSTANT + jump branch)
// for Player. NAI-65 closed D3-Player (focus call) and D4-Player
// (lastStepX = x-1; lastStepZ = z). See DEVIATION block at npc_script.go
// for the full tracker; D4-NPC, D5-NPC, and NAI-41 remain residual.
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

	// NAI-65 D3-Player: focus call from TS PathingEntity.ts:286-289.
	// Player width=length=1 (no struct field; PathingEntity-default).
	dir := coordgrid.Face(prevX, prevZ, x, z)
	moveX := coordgrid.MoveX(p.x, dir)
	moveZ := coordgrid.MoveZ(p.z, dir)
	p.focus(coordgrid.Fine(moveX, 1), coordgrid.Fine(moveZ, 1), false)

	// Order: refreshPlayerZone runs BEFORE p.tele = true to match TS
	// PathingEntity.ts:290-293. The two writes are functionally
	// commutative (refresh reads only previous coords + current
	// x/z/level; the tele bit is independent), but TS-faithful order is
	// the project's true-to-TS gate default.
	refreshPlayerZone(p, prevX, prevZ, prevLevel)

	// NAI-65 D4-Player: lastStep adjust from TS PathingEntity.ts:291-292.
	// Currently dead-write at HEAD (no production reader of
	// p.lastStepX/Z besides the dead-write of p.followX/Z in
	// processInteraction). Tracked.
	p.lastStepX = p.x - 1
	p.lastStepZ = p.z

	p.tele = true

	// D5: level-change → INSTANT + jump per PathingEntity.ts:295-298.
	if prevLevel != level {
		p.moveSpeed = MoveSpeedInstant
		p.jump = true
	}
}

// focus records the fine-grained face-angle coord. Mirrors TS
// PathingEntity.focus (Engine-TS/src/engine/entity/PathingEntity.ts:321-333).
// instant=true ALSO writes faceSquareX/Z to (fx, fz) and ORs
// MaskFaceCoord into masks.
//
// Coord-frame note: focus() takes RAW fine coords (already
// CoordGrid.fine'd). Distinct from (*Player).FaceSquare in
// modules/world/player_masks.go which takes absolute coords and
// applies *2+1.
//
// Drivers per TS: Teleport (PathingEntity.ts:289), takeStep
// (PathingEntity.ts:220), reorient (PathingEntity.ts:353,358),
// setInteraction (PathingEntity.ts:528). The setInteraction site is
// the only one that ever passes instant=true — gated on
// (target instanceof NonPathingEntity && interaction === Interaction.ENGINE).
func (p *Player) focus(fx, fz int, instant bool) {
	p.faceAngleX = fx
	p.faceAngleZ = fz
	if instant {
		p.faceSquareX = fx
		p.faceSquareZ = fz
		p.masks |= rsbuf.MaskFaceCoord
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
// the player's primary animation slot. seqID=-1 clears. Mirrors TS
// Player.playAnimation (Player.ts:1840-1851): bounds-reject on
// seqID >= SeqType.count, animProtect early-return, and priority-comparison
// overwrite gate. The seqID==-1 / animID==-1 short-circuits in the priority
// arm guard the slice dereferences. Closes deviation NAI-56-D1.
func (p *Player) PlayAnim(seqID, delay int) {
	if seqID >= p.seqTypes.Count() || p.animProtect != 0 {
		return // TS Player.ts:1841
	}
	if seqID == -1 || p.animID == -1 ||
		p.seqTypes.Configs[seqID].Priority > p.seqTypes.Configs[p.animID].Priority ||
		p.seqTypes.Configs[p.animID].Priority == 0 {
		p.animID = seqID
		p.animDelay = delay
		p.masks |= rsbuf.MaskAnim
	}
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
// IF_CLOSE on the next encodeOut pass. When clearWeakQueue is true
// (TS default), drops every QueueWeak entry from p.queue before
// processing. When the player is not delayed, clears any active
// script's Protect flag (NAI-52 convergence). Early-returns if no
// modal is currently open. Otherwise: nulls activeScript on
// COUNTDIALOG/PAUSEBUTTON suspends (closes NAI-52-F1) and dispatches
// a per-slot IF_CLOSE trigger script (Main → Chat → Side, TS order).
//
// Mirrors TS Player.closeModal (Player.ts:741-794). Body fully
// landed across NAI-53 T1-T5; per-slot clearComListeners wired in
// NAI-64 (TS Player.ts:728-739, 767, 778, 789).
func (p *Player) CloseModal(clearWeakQueue bool) {
	if clearWeakQueue {
		p.clearWeakQueue()
	}
	if !p.delayed && p.activeScript != nil {
		p.activeScript.Pointers &^= script.PtrProtectedActivePlayer
	}

	if p.modalState == modalStateNone {
		return
	}

	p.modalState = modalStateNone

	// Close any input-dialogue suspended scripts. NAI-52-F1.
	if p.activeScript != nil &&
		(p.activeScript.Execution == script.CountDialog ||
			p.activeScript.Execution == script.PauseButton) {
		p.activeScript = nil
	}

	// Per-slot IF_CLOSE dispatch (Main → Chat → Side, TS order).
	if p.client != nil && p.client.server != nil {
		s := p.client.server
		if p.modalMain != -1 {
			p.runIfCloseTrigger(s, p.modalMain)
			p.clearComListeners(p.modalMain)
			p.modalMain = -1
		}
		if p.modalChat != -1 {
			p.runIfCloseTrigger(s, p.modalChat)
			p.clearComListeners(p.modalChat)
			p.modalChat = -1
		}
		if p.modalSide != -1 {
			p.runIfCloseTrigger(s, p.modalSide)
			p.clearComListeners(p.modalSide)
			p.modalSide = -1
		}
	} else {
		// No server (test path with no Server bound) — still reset slots.
		p.modalMain = -1
		p.modalChat = -1
		p.modalSide = -1
	}

	p.refreshModalClose = true
}

// runIfCloseTrigger looks up TriggerIfClose for slotCom and runs it
// if found. Mirrors TS Player.closeModal per-slot
// `executeScript(ScriptRunner.init(closeTrigger, this), false)`
// (Player.ts:761-769, 772-780, 783-791).
//
// Nil-safe on s.scriptProvider; runScript is itself nil-safe on the
// returned ScriptFile.
func (p *Player) runIfCloseTrigger(s *Server, slotCom int) {
	if s.scriptProvider == nil {
		return
	}
	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerIfClose, slotCom, -1)
	s.runScript(sf, p, nil, false, nil, nil)
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

// OpenTutorial sets the player's tutorial-overlay component and writes
// the matching TUT_OPEN wire packet UNCONDITIONALLY. Mirrors TS
// Player.openTutorial at Engine-TS/src/engine/entity/Player.ts:1999-2003,
// which writes `new TutOpen(com)` on every call regardless of prior
// state. NAI-112 Stage 2.2 retired the goscape diff at
// modules/world/player.go (pre-NAI-112: emit-only-on-change introduced
// by NAI-76 T2) — that suppressed the re-open the client needs to flush
// IF_SETTEXT updates when the same tutorial component is re-opened.
// Surfaced at NAI-110 close smoke; bound NAI-112 Stage 2.1 instrumentation
// 2026-05-06.
//
// Opening the tutorial does NOT close any other modal — the TUT bit is
// OR'd into modalState and the tutorial id is stored alongside the
// wire emit.
func (p *Player) OpenTutorial(com int) {
	payload := []byte{byte(com >> 8), byte(com)}
	p.writeOut(gameserver.OpTutOpen, payload)
	p.modalState |= modalStateTut
	p.modalTutorial = com
}

// CloseTutorial closes the player's tutorial overlay. Per TS:
// no-op if no tutorial open; otherwise dispatches the matching IF_CLOSE
// trigger (if registered) for the current modalTutorial component,
// resets modalTutorial to -1, and writes TUT_OPEN(-1) directly.
//
// TS Player.closeTutorial at Engine-TS/src/engine/entity/Player.ts:716-726
// writes `new TutOpen(-1)` directly. NAI-76 routed this through
// encodeOut's diff-check (NAI-76 pin); NAI-112 Stage 2.2 inlines the
// write to match TS unconditional-emit semantics and to symmetrize with
// the OpenTutorial fix.
//
// TS does NOT call clearComListeners(modalTutorial) here (contrast with
// closeModal); we mirror that absence.
//
// Clears modalStateTut on the goscape-internal modalState bitmap
// (goscape defensive; TS skips this check). Labelled per
// defensive_gate_doc_comment_label.md.
func (p *Player) CloseTutorial() {
	if p.modalTutorial == -1 {
		return
	}
	if p.client != nil && p.client.server != nil {
		p.runIfCloseTrigger(p.client.server, p.modalTutorial)
	}
	p.modalTutorial = -1
	p.modalState &^= modalStateTut
	payload := []byte{0xff, 0xff} // -1 as int16 BE
	p.writeOut(gameserver.OpTutOpen, payload)
}

// FlashTutorial implements script.ActivePlayer.FlashTutorial. Writes
// a TUT_FLASH server packet (opcode 126, 1-byte tab payload). Direct
// write — TUT_FLASH is fire-and-forget UI hint, not a modal-state
// transition like TUT_OPEN, so no deferred-flush pathway. Mirrors
// LostCityRS/Engine-TS Player.write(new TutFlash(tab)) call from
// PlayerOps.ts:694-696 + TutFlashEncoder.ts:9-11.
//
// No client-nil guard — matches goscape's direct-writer convention
// (CamReset at line 189-191, HintNpc at line 201-209, WriteEnableTracking
// at player.go:416-418); writeOut itself does not nil-guard either.
func (p *Player) FlashTutorial(tab int) {
	p.writeOut(gameserver.OpTutFlash, []byte{byte(tab)})
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

// RequestLogout implements script.ActivePlayer.RequestLogout. Flags the
// player for tick-loop logout processing; processLogouts (tick.go) tears
// the session down at the next boundary. Mirrors TS PlayerOps.ts:622-624
// (P_LOGOUT) — `state.activePlayer.requestLogout = true`.
func (p *Player) RequestLogout() {
	p.requestLogout = true
}

// ClearPendingAction implements script.ActivePlayer.ClearPendingAction.
// Resets interaction kind/target/op to idle and closes any open modal.
// Walk queue is preserved.
func (p *Player) ClearPendingAction() {
	p.interactionKind = InteractionEngine
	p.target = nil
	p.targetOp = -1
	p.CloseModal(true)
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
// anim-protect flag; when nonzero, PlayAnim suppresses in-engine animation
// requests (NAI-56 wired the reader at PlayAnim's L1842 gate). Matches TS
// Player.ts:321 (field) + PlayerOps.ts:1171-1172.
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

// QueueWaypoint implements script.ActivePlayer.QueueWaypoint by
// delegating to the package-private queueWaypoint at movement.go.
// NAI-115 T7.
func (p *Player) QueueWaypoint(x, z int) { p.queueWaypoint(x, z) }

// SetInteractionScriptObj implements script.ActivePlayer. Type-asserts
// the script-side ActiveObj to *entity.Obj and anchors the player
// with InteractionScript + APOBJ<op>. Silently no-ops if the obj
// isn't a real *entity.Obj. NAI-115 T7.
func (p *Player) SetInteractionScriptObj(obj script.ActiveObj, op int) {
	realObj, ok := obj.(*entitypkg.Obj)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realObj, op, -1)
}

// HasInteraction reports whether the player has an interaction target,
// excluding the follow interaction (APPLAYER3 / OPPLAYER3). Mirrors TS
// Player.hasInteraction at Engine-TS Player.ts:955-964 — "the follow
// interaction doesn't do anything", so it is reported as not-busy.
// NAI-120 Bundle 2B. Implements script.ActivePlayer.
func (p *Player) HasInteraction() bool {
	if p.target == nil {
		return false
	}
	if isFollowOp(p) {
		return false
	}
	return true
}

// HasWaypoints reports whether the player has a waypoint queue active.
// Wraps the package-private hasWaypoints helper at interaction.go:297.
// NAI-120 Bundle 2B. Implements script.ActivePlayer.
func (p *Player) HasWaypoints() bool { return p.hasWaypoints() }

// SetInteractionScriptNpcT implements script.ActivePlayer.
// Routes via SetInteraction(InteractionScript, npc, targetOpNpcT, spellCom)
// — the targetOpNpcT sentinel (=8 at modules/world/interaction.go:35) selects
// the APNPCT/OPNPCT trigger family in resolveTriggerTypeId. NAI-120 Bundle 2B.
func (p *Player) SetInteractionScriptNpcT(npc script.ActiveNpc, spellCom int) {
	realNpc, ok := npc.(*Npc)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realNpc, targetOpNpcT, spellCom)
}

// SetInteractionScriptPlayer implements script.ActivePlayer. Routes via
// SetInteraction(InteractionScript, realPlayer2, op, -1). The com=-1 means
// no spellCom association — APPLAYER<N> reads no targetSubject.com. NAI-120
// Bundle 2B.
func (p *Player) SetInteractionScriptPlayer(player2 script.ActivePlayer, op int) {
	realPlayer2, ok := player2.(*Player)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realPlayer2, op, -1)
}

// LowMemory returns the player's low-memory flag as plumbed from the
// RS2 login request (req.LowMemory) through client.lowMemory and
// copied onto the Player at newPlayer().
func (p *Player) LowMemory() bool { return p.lowMemory }

// NAI-47: SETIDKIT appearance mutation.
func (p *Player) Gender() int                  { return p.gender }


// AddHeroPoints implements script.ActivePlayer. Credits amount to
// playerUID on the player's hero-point ledger. Used by BOTH_HEROPOINTS.
// Mirrors TS Player.heroPoints.addHero at PlayerOps.ts:1167.
func (p *Player) AddHeroPoints(playerUID, amount int) {
	p.heroPoints.AddHero(playerUID, amount)
}

// TopContributor implements script.ActivePlayer. Returns the playerUID
// with the largest HeroPoints credit, or 0 if the ledger is empty.
// Used by FINDHERO. Mirrors TS state.activePlayer.heroPoints.findHero()
// at PlayerOps.ts:1139.
func (p *Player) TopContributor() int {
	return p.heroPoints.TopContributor()
}
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

// PlaySynth sends a synthesized sound effect to the client. Called by
// the SOUND_SYNTH script opcode (PlayerOps.ts:466-474). Encodes
// synth/loops/delay via encodeSynthSound and writes OpSynthSound.
//
// No name normalization, no PRELOADED lookup, no validation — TS
// handler has none. Out-of-range int values truncate at the
// uint16/uint8/uint16 cast boundary (matches TS p1/p2 narrowing).
func (p *Player) PlaySynth(synth, loops, delay int) {
	buf := packet.NewPacket(make([]byte, 0, 5))
	encodeSynthSound(buf, uint16(synth), uint8(loops), uint16(delay))
	p.writeOut(gameserver.OpSynthSound, buf.Bytes())
}


// SetPreventLogout implements script.ActivePlayer. Mirrors TS
// PlayerOps.ts:628-629 (state.activePlayer.preventLogoutMessage =
// msg; state.activePlayer.preventLogoutUntil = currentTick + ticks).
// NAI-127 Bundle 2.
func (p *Player) SetPreventLogout(message string, untilTick int) {
	p.preventLogoutMessage = message
	p.preventLogoutUntil = untilTick
}

// ApplyDamage implements script.ActivePlayer. Delegates to
// Player.Damage (player_masks.go:126), which is the existing
// damage-mask producer. Mirrors TS player.applyDamage(amount, type)
// at PlayerOps.ts:778. NAI-127 Bundle 2.
func (p *Player) ApplyDamage(amount, dmgType int) {
	p.Damage(amount, dmgType)
}
