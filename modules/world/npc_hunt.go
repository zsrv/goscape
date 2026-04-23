package world

import (
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
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
// CheckVis (NAI-12): LoS/LoW gate wired per TS
// ScriptIterators.ts:88-94 with the TS player-as-source /
// NPC-as-dest argument swap quirk preserved — see gate at the
// filter chain below.
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
		// CheckVis gate — TS ScriptIterators.ts:88-94.
		// FIDELITY: TS huntPlayers swaps src/dest vs other three variants —
		// player-as-source (p.x, p.z) → NPC-as-dest (n.x, n.z). Preserve
		// the asymmetry verbatim. See NAI-12 spec § Architecture.
		// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
		if hunt.CheckVis == objtype.HuntVisLineOfSight && s.gamemap != nil &&
			!s.gamemap.Pathfinder.LineValidator.HasLineOfSight(
				n.level, p.x, p.z, n.x, n.z, 1, 1, 1, 0) {
			continue
		}
		if hunt.CheckVis == objtype.HuntVisLineOfWalk && s.gamemap != nil &&
			!s.gamemap.Pathfinder.LineValidator.HasLineOfWalk(
				n.level, p.x, p.z, n.x, n.z, 1, 1, 1, 0) {
			continue
		}
		// checkVars (TS:950-957): AND-chain of varp/operator/value predicates.
		// Nil/empty CheckVars → no-op (ranging nil slice yields zero iterations,
		// matching TS empty-`every` → true semantics).
		passCheckVars := true
		for _, cv := range hunt.CheckVars {
			if cv.VarID == -1 {
				// TS:953 `checkVar.varId === -1 ||` short-circuit.
				continue
			}
			if !hunt.CheckHuntCondition(int(p.Varp(cv.VarID)), cv.Condition, cv.Val) {
				passCheckVars = false
				break
			}
		}
		if !passCheckVars {
			continue
		}
		hunted = append(hunted, p)
	}
	return hunted
}

// consumeHuntTarget converts a hunt-phase result (n.huntTarget) into
// interaction state. Matches TS Npc.consumeHuntTarget at
// Engine-TS/.../Npc.ts:887-919.
//
// Control flow:
//   - Entry guards: huntTarget non-nil, huntMode in bounds, hunt config
//     non-nil, hunt.Type != HuntModeOff. Any guard fires → no-op.
//   - Branch on hunt.FindNewMode:
//     QUEUE1..QUEUE20 → fire TriggerAiQueueN directly via runNpcScript.
//     else           → n.SetInteraction(InteractionScript, huntTarget,
//     FindNewMode, -1). Closes the full setInteraction side-effect set
//     (apRange, apRangeCalled, targetSubject, focus, faceEntity+masks,
//     targetX/Z, isValid pre-check) atomically.
//   - Common tail (both branches): n.huntTarget = nil, n.huntClock = 0.
//   - If !hunt.FindKeepHunting: n.huntMode = -1.
//
// The NAI-10 DEVIATION block (listing apRange/targetSubject/focus/etc.
// as deferred) is closed by the NAI-11 migration — see Npc.SetInteraction
// for the full side-effect contract.
func (s *Server) consumeHuntTarget(n *Npc) {
	if n.huntTarget == nil {
		return
	}
	if s.huntTypes == nil ||
		n.huntMode < 0 ||
		n.huntMode >= len(s.huntTypes.Configs) {
		return
	}
	hunt := s.huntTypes.Configs[n.huntMode]
	if hunt == nil || hunt.Type == objtype.HuntModeOff {
		return
	}
	if hunt.FindNewMode >= objtype.NPCModeQueue1 &&
		hunt.FindNewMode <= objtype.NPCModeQueue20 {
		// QUEUE1..QUEUE20 branch: fire TriggerAiQueueN directly (not
		// enqueued). target/targetOp NOT written — the script owns
		// subsequent state. Matches TS Npc.ts:896-903.
		if n.typ != nil && s.scriptProvider != nil {
			trigger := script.TriggerAiQueue1 +
				script.ServerTriggerType(hunt.FindNewMode-objtype.NPCModeQueue1)
			sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.typ.Category)
			s.runNpcScript(sf, n, nil, nil, nil)
		}
	} else {
		// Interaction branch: full SetInteraction port closes the NAI-10
		// setInteraction-deferral set (7 fields) atomically.
		n.SetInteraction(InteractionScript, n.huntTarget, hunt.FindNewMode, -1)
	}

	// Common tail: clear huntTarget and reset huntClock.
	n.huntTarget = nil
	n.huntClock = 0

	// Stop-hunting clause: once an NPC finds a huntTarget, it won't
	// hunt again until its interactions are cleared — unless the hunt
	// config explicitly opts into keep-hunting. Matches TS Npc.ts:913-918.
	if !hunt.FindKeepHunting {
		n.huntMode = -1
	}
}
