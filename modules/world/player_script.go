package world

import (
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

// playerQueueRequest is one queued fresh-run script request with a
// single int arg. Queue entries are processed in processActiveScripts;
// when Delay reaches zero (or below) the target script runs as a brand-
// new ScriptState.
type playerQueueRequest struct {
	ScriptID uint32
	Delay    int
	IntArg   int
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

// EnqueueScript appends a queued fresh-run request to the player's
// normal queue. Delay=0 fires on the next processActiveScripts pass.
func (p *Player) EnqueueScript(scriptID uint32, delay int, intArg int) {
	p.queue = append(p.queue, playerQueueRequest{
		ScriptID: scriptID,
		Delay:    delay,
		IntArg:   intArg,
	})
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

// AddXP adds xp (scaled * 10) to the player's stored XP for skill id.
// OOB ids are dropped silently.
// TODO: recompute baseLevels from getLevelByExp table and clamp at XP cap.
func (p *Player) AddXP(id int, xp int) {
	if !statBounds(id) {
		return
	}
	p.stats[id] += int32(xp)
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
