package world

import "github.com/zsrv/goscape/pkg/script"

// SetTimer implements script.ActivePlayer.SetTimer.
func (p *Player) SetTimer(scriptID uint32, interval, intArg int, ttype script.PlayerTimerType) {
	if p.timers == nil {
		p.timers = make(map[uint32]*playerTimer)
	}
	now := 0
	if p.client != nil && p.client.server != nil {
		now = p.client.server.currentTick
	}
	p.timers[scriptID] = &playerTimer{
		ScriptID: scriptID,
		Type:     ttype,
		Interval: interval,
		Clock:    now,
		IntArg:   intArg,
	}
}

// ClearTimer implements script.ActivePlayer.ClearTimer.
func (p *Player) ClearTimer(scriptID uint32) {
	if p.timers == nil {
		return
	}
	delete(p.timers, scriptID)
}

// GetTimer implements script.ActivePlayer.GetTimer. Returns -1 if no
// timer is registered at scriptID.
func (p *Player) GetTimer(scriptID uint32) int {
	if p.timers == nil {
		return -1
	}
	t, ok := p.timers[scriptID]
	if !ok {
		return -1
	}
	now := 0
	if p.client != nil && p.client.server != nil {
		now = p.client.server.currentTick
	}
	return (t.Clock + t.Interval) - now
}
