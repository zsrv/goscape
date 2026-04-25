package world

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/script"
)

// SetTimer implements script.ActivePlayer.SetTimer.
//
// NAI-27 Bundle 2: adds error return for the script-missing propagation
// pattern mirroring (*Player).EnqueueScriptArgs at player_script.go:102-118.
// When the scriptProvider chain is nil (engine-dispatch path with no
// provider configured), returns nil — preserves the no-op tolerance.
// When the provider returns nil for the scriptID, returns an error
// matching TS PlayerOps.ts:838-840 / :822-824 throw shape.
func (p *Player) SetTimer(scriptID uint32, interval int, intArgs []int, stringArgs []string, ttype script.PlayerTimerType) error {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		// Engine-dispatch tolerance — no provider configured, no script to validate.
		// Mirrors EnqueueScriptArgs guard at player_script.go:103-105.
	} else if p.client.server.scriptProvider.GetByID(scriptID) == nil {
		return fmt.Errorf("unable to find timer script: %d", scriptID)
	}
	if p.timers == nil {
		p.timers = make(map[uint32]*playerTimer)
	}
	now := 0
	if p.client != nil && p.client.server != nil {
		now = p.client.server.currentTick
	}
	p.timers[scriptID] = &playerTimer{
		ScriptID:   scriptID,
		Type:       ttype,
		Interval:   interval,
		Clock:      now,
		IntArgs:    intArgs,
		StringArgs: stringArgs,
	}
	return nil
}

// ClearTimer implements script.ActivePlayer.ClearTimer.
func (p *Player) ClearTimer(scriptID uint32) {
	if p.timers == nil {
		return
	}
	delete(p.timers, scriptID)
}

// GetTimer implements script.ActivePlayer.GetTimer. Returns the
// absolute tick when the timer was last set or fired (TS-faithful per
// PlayerOps.ts:858 → Player.ts:910 timer.clock semantics). Returns -1
// if no timer is registered at scriptID.
//
// NAI-27 Bundle 2: flipped from the prior "(Clock+Interval)-now"
// remaining-ticks computation, which was an untracked semantic
// divergence from TS. The new return matches what TS GETTIMER pushes.
func (p *Player) GetTimer(scriptID uint32) int {
	if p.timers == nil {
		return -1
	}
	t, ok := p.timers[scriptID]
	if !ok {
		return -1
	}
	return t.Clock
}
