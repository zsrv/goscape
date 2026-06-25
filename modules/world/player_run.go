package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
	applog "github.com/zsrv/goscape/pkg/util/log"
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
		// player-core-2: TS Player.ts:690-693 keeps weightKg as float
		// (`this.runweight / 1000`) and only truncates the final loss
		// via `| 0`, so a partial-kg encumbrance contributes a
		// fractional drain that rounds away properly at the end. Int
		// division on runweight here would drop the fraction BEFORE
		// the 67*weightKg/64 math and systematically under-drain.
		weightKg := float64(p.runweight) / 1000
		clampWeight := max(min(weightKg, 64), 0)
		loss := int(67 + 67*clampWeight/64)
		p.runenergy = max(p.runenergy-loss, 0)
	}
	if p.runenergy == 0 {
		varpID := p.RunVarpID()
		if p.client != nil && p.client.server != nil &&
			p.client.server.cfg.NodeDebug && p.client.server.log != nil {
			var varpPre int32
			if varpID >= 0 && varpID < len(p.varps) {
				varpPre = p.varps[varpID]
			}
			applog.Trace(p.client.server.log, "nai138.update_energy.zero",
				"tick", p.client.server.currentTick,
				"player_uid", p.uid,
				"varp_id", varpID,
				"varp_pre", varpPre,
				"run_pre", p.run,
			)
		}
		p.run = 0
		p.SetVarp(varpID, 0)
	}
	if p.runenergy < 100 {
		p.tempRun = 0
	}
}
