package world

import (
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// processNpcHunt runs the per-tick hunt pass. Matches TS
// Npc.ts:158-171.
//
// Observer gate: calls rsbuf.GetNpcObservers(n.nid) — the counter
// maintained by pkg/rsbuf's NpcInfo encoder (subscription add/remove)
// and by processLogouts's bulk-decrement. Mirrors TS rsbuf.getNpcObservers
// public API.
//
// TS NobodyNear values:
//   - HuntNobodyNearKeepHunting: gate always passes
//   - HuntNobodyNearPauseHunt: gate passes iff observers > 0 OR
//     type == HuntModePlayer
//
// Note on mode bounds: SetHuntMode accepts any int (including
// out-of-range) to match TS. processNpcHunt validates bounds
// against s.huntTypes.Configs and silently no-ops on invalid.
func (s *Server) processNpcHunt(n *Npc) {
	if n.huntMode == -1 {
		return
	}
	if s.huntTypes == nil ||
		n.huntMode < 0 ||
		n.huntMode >= len(s.huntTypes.Configs) {
		return
	}
	hunt := s.huntTypes.Configs[n.huntMode]
	if hunt == nil {
		return
	}
	observers := rsbuf.GetNpcObservers(n.nid)
	if hunt.NobodyNear == objtype.HuntNobodyNearPauseHunt &&
		observers <= 0 &&
		hunt.Type != objtype.HuntModePlayer {
		return
	}
	// Player-type hunts skip the huntAll dispatcher at the turn()
	// level — matches TS Npc.ts:164 "hunt && hunt.type !== HuntModeType.PLAYER".
	// The HuntModePlayer branch in huntAll is reachable only via
	// explicit scripted calls, not the turn() path.
	if hunt.Type != objtype.HuntModePlayer {
		n.huntAll(s, hunt)
	}
	n.huntClock++
}

// huntAll dispatches to a hunted-type variant and sets huntTarget.
// Matches TS Npc.ts:249-277. Variants are stubs at NAI-7; NAI-8
// fills huntPlayers; NAI-9 fills huntNpcs/huntObjs/huntLocs.
func (n *Npc) huntAll(s *Server, hunt *objtype.HuntType) {
	n.huntTarget = nil
	if n.huntClock < hunt.Rate-1 {
		return
	}
	if hunt.Type == objtype.HuntModeOff || n.huntRange < 1 {
		return
	}
	var hunted []entity
	switch hunt.Type {
	case objtype.HuntModePlayer:
		hunted = n.huntPlayers(s, hunt)
	case objtype.HuntModeNpc:
		hunted = n.huntNpcs(s, hunt)
	case objtype.HuntModeObj:
		hunted = n.huntObjs(s, hunt)
	case objtype.HuntModeScenery:
		hunted = n.huntLocs(s, hunt)
	}
	if len(hunted) > 0 {
		n.huntTarget = hunted[rand.IntN(len(hunted))]
	}
}

// huntPlayers iterates the player grid in huntRange and returns
// players passing the filter chain. Matches TS Npc.huntPlayers at
// Engine-TS/.../Npc.ts:921-973.
//
// Filter coverage (NAI-8):
//   - Range + level match: always
//   - checkAfk: via p.IsZonesAfk (TS:935-937)
//
// Filters DEFERRED to future audit pass (Go infrastructure
// missing; each TS line cited):
//   - checkNotBusy (TS:931-933)       — no Player.Busy()
//   - checkNotTooStrong (TS:939-941)  — wilderness + combat
//   - checkNotCombat (TS:943-945)     — varp+8-tick window
//   - checkNotCombatSelf (TS:946-948) — varp+8-tick window
//   - checkVars (TS:950-957)          — varp condition chain
//   - checkInv (TS:959-969)           — inventory queries
//
// NAI-8 dispatches NO scripts. TS huntPlayers is a config-driven
// filter pipeline, not a script runner.
func (n *Npc) huntPlayers(s *Server, hunt *objtype.HuntType) []entity {
	if s.grid == nil {
		return nil
	}
	// TS HuntIterator zone-radius formula at ScriptIterators.ts:57:
	// radius = (1 + distance/8) | 0.
	zoneRadius := 1 + n.huntRange/8
	slots := s.grid.NearbyPlayers(n.x, n.z, n.level, zoneRadius)
	var hunted []entity
	for _, slot := range slots {
		if slot < 0 || slot >= len(s.players) {
			continue
		}
		p := s.players[slot]
		if p == nil {
			continue
		}
		if p.level != n.level {
			continue
		}
		dx := p.x - n.x
		if dx < 0 {
			dx = -dx
		}
		dz := p.z - n.z
		if dz < 0 {
			dz = -dz
		}
		if dx > n.huntRange || dz > n.huntRange {
			continue
		}
		// checkAfk (TS:935-937): filter players who've gone AFK
		// (1000-tick same-zone threshold).
		if hunt.CheckAfk && p.IsZonesAfk() {
			continue
		}
		hunted = append(hunted, p)
	}
	return hunted
}
