package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
)

// updateEnergy drains or recovers run energy for one tick, and
// disables run-mode at low-energy thresholds.
//
// Mirrors TS Engine-TS/src/engine/entity/Player.ts:682-704
// line-for-line. Called once per player per tick from
// (*Server).processEnergy after movement + interactions resolve.
//
// stepsTaken < 2 is the TS "idle / single-step" recovery branch;
// stepsTaken >= 2 is the running-this-tick drain branch (a clean
// run-step emits walk + run for stepsTaken==2).
//
// runweight is in grams; TS divides by 1000 to convert to kg, then
// clamps to [0, 64]. Loss formula = floor(67 + 67*kg/64).
//
// At runenergy==0: clear p.run AND propagate via
// SetVarp(p.RunVarpID(), 0) — the cache-resolved engine run-mode varp
// id (clientcode==7, typically 173 for option_run). Mirrors TS
// Player.ts:697-699.
//
// At runenergy<100: clear p.tempRun (TS Player.ts:701-703).
//
// NAI-135.
func (p *Player) updateEnergy() {
	if p.delayed {
		return
	}
	if p.stepsTaken < 2 {
		agility := int(p.baseLevels[objtype.PlayerStatAgility])
		recovered := agility/9 + 8
		p.runenergy = min(p.runenergy+recovered, 10000)
	} else {
		weightKg := p.runweight / 1000
		clampWeight := max(min(weightKg, 64), 0)
		loss := 67 + 67*clampWeight/64
		p.runenergy = max(p.runenergy-loss, 0)
	}
	if p.runenergy == 0 {
		p.run = 0
		p.SetVarp(p.RunVarpID(), 0)
	}
	if p.runenergy < 100 {
		p.tempRun = 0
	}
}
