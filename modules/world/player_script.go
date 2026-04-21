package world

import "github.com/zsrv/goscape/pkg/script"

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
