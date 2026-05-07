package world

import (
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// processNpcHunt runs the per-tick hunt pass. Matches TS
// Npc.ts:158-171.
//
// Observer gate: calls s.rsbuf.GetNpcObservers(int32(n.nid)) — the counter
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
	var observers int32
	if s.rsbuf != nil {
		observers = s.rsbuf.GetNpcObservers(int32(n.nid))
	}
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

// huntPlayers iterates players in zone-subscription within huntRange and returns
// players passing the filter chain. Matches TS Npc.huntPlayers at
// Engine-TS/.../Npc.ts:921-973.
//
// Spatial backend: pkg/zone.Zone.PlayersSafe (NAI-28). Iterates zones in
// zoneRadius via s.zoneMap.NearbyZones, type-asserts each PlayerLike to *Player.
//
// Filter coverage:
//   - Range + level match:     always
//   - checkNotBusy             (NAI-23, TS:931-933)
//   - checkAfk                 (NAI-8,  TS:935-937)
//   - CheckVis LoS/LoW         (NAI-12, TS per ScriptIterators.ts:88-94)
//   - checkNotTooStrong        (NAI-23, TS:939-941)
//   - Outer combat guard       (NAI-15, TS:942)
//   - checkNotCombat           (NAI-15, TS:943-945)
//   - checkNotCombatSelf       (NAI-16, TS:946-948)
//   - checkVars                (NAI-15, TS:950-957)
//   - checkInv                 (NAI-22, TS:959-969)
//
// All NAI-8 deferred filters now ported (NAI-23 closes checkNotBusy +
// checkNotTooStrong).
//
// CheckVis (NAI-12) preserves the TS player-as-source / NPC-as-dest
// argument swap quirk — see FIDELITY note at the gate below.
//
// NAI-8 dispatches NO scripts. TS huntPlayers is a config-driven
// filter pipeline, not a script runner.
func (n *Npc) huntPlayers(s *Server, hunt *objtype.HuntType) []entity {
	if s.zoneMap == nil {
		return nil
	}
	// TS HuntIterator zone-radius formula at ScriptIterators.ts:57:
	// radius = (1 + distance/8) | 0.
	zoneRadius := 1 + n.huntRange/8
	var hunted []entity
	for _, zn := range s.zoneMap.NearbyZones(n.level, n.x, n.z, zoneRadius) {
		for pl := range zn.PlayersSafe(false) {
			// Type-assertion guard for the PlayerLike cyclic-import boundary
			// (pkg/zone defines PlayerLike; modules/world/*Player satisfies it).
			// Production EnterPlayer only ever receives *Player, so ok=false is
			// currently unreachable — kept as forward-compatible safety.
			p, ok := pl.(*Player)
			if !ok {
				continue
			}
			// Level filter is redundant — NearbyZones is already level-filtered —
			// but kept for defensive symmetry with TS huntPlayers; harmless.
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
			// checkNotBusy (TS:931-933): skip players whose state cannot
			// accept a hunt interaction (delayed or main/chat modal open).
			if hunt.CheckNotBusy && p.Busy() {
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
			// checkNotTooStrong (TS:939-941): skip players whose combatLevel
			// is more than 2x the NPC's vislevel when they are OUTSIDE the
			// wilderness (the wilderness disables this protection). Filter
			// only applies when CheckNotTooStrong is OutsideWilderness;
			// Off → filter skipped.
			if hunt.CheckNotTooStrong == objtype.HuntCheckNotTooStrongOutsideWilderness &&
				!p.IsInWilderness() &&
				p.combatLevel > n.typ.VisLevel*2 {
				continue
			}

			// Outer combat guard — TS:942. Only when the candidate is not the
			// NPC's current target AND not in a multi-combat zone.
			// FIDELITY: when s.gamemap is nil, IsMulti can't be called — treat
			// as not-multi, so the guard APPLIES and the combat filter fires.
			// This nil-short-circuit direction is the OPPOSITE of CheckVis's
			// in the same file (CheckVis's nil → filter skipped; here nil →
			// filter runs). Do NOT "simplify" to
			// `s.gamemap != nil && !s.gamemap.IsMulti(...)` — that inverts the
			// guard's nil behavior and flips TestHuntPlayersCombatGuard's
			// `gamemap-nil-applies-guard` sub-case red.
			applyCombatGuard := entity(p) != n.target &&
				(s.gamemap == nil || !s.gamemap.IsMulti(p.x, p.z, p.level))
			if applyCombatGuard {
				// checkNotCombat (TS:943-945): skip players whose last-combat
				// varp was written within the past 8 ticks.
				if hunt.CheckNotCombat != -1 &&
					int(p.Varp(hunt.CheckNotCombat))+8 > s.currentTick {
					continue
				}
				// checkNotCombatSelf (TS:946-948): skip candidate if this NPC's
				// own combat-tracker varn was written within the past 8 ticks.
				// Symmetric to checkNotCombat above, but reads the NPC side
				// (n.NpcVarN) instead of the player side (p.Varp).
				if hunt.CheckNotCombatSelf != -1 &&
					int(n.NpcVarN(hunt.CheckNotCombatSelf))+8 > s.currentTick {
					continue
				}
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

			// checkInv (TS Npc.ts:959-969): if CheckInv is set, compute quantity
			// per CheckObj or CheckObjParam branch, then evaluate CheckHuntCondition.
			// Defensive: missing inv → quantity=0 (TS Player._invTotalParam throws
			// 'Invalid inventory type' here, but goscape huntPlayers must continue
			// iteration on one bad player; live players have all standard invs in
			// practice, so this divergence is dead-path. No deviation tag tracked.
			if hunt.CheckInv != -1 {
				quantity := 0
				if pInv := p.invs[hunt.CheckInv]; pInv != nil {
					if hunt.CheckObj != -1 {
						quantity = pInv.GetItemCount(hunt.CheckObj)
					} else if hunt.CheckObjParam != -1 {
						quantity = invTotalParam(pInv, hunt.CheckObjParam,
							s.objTypes, s.paramTypes)
					}
				}
				if !hunt.CheckHuntCondition(quantity,
					hunt.CheckInvCondition, hunt.CheckInvVal) {
					continue
				}
			}

			hunted = append(hunted, p)
		}
	}
	return hunted
}

// invTotalParam mirrors handleInvTotalParam (pkg/script/handlers_inv.go:224)
// for non-ScriptState callers. Sums per-slot ObjType.Params[param] across
// every non-empty slot of inv, falling back to ParamType.DefaultInt for
// missing params. Returns 0 if any required config is nil — defensive,
// huntPlayers cannot abort iteration on a single param-resolution failure.
//
// TS source: Player._invTotalParam at Player.ts:1668-1697 (stack=false branch).
func invTotalParam(inv *inventory.Inventory, param int,
	objs *objtype.ObjTypeConfigs, params *objtype.ParamTypeConfigs) int {
	if inv == nil || objs == nil || params == nil {
		return 0
	}
	if param < 0 || param >= len(params.Configs) {
		return 0
	}
	pt := params.Configs[param]
	if pt == nil {
		return 0
	}
	total := 0
	for _, it := range inv.Items {
		if it == nil || it.Id < 0 {
			continue
		}
		if it.Id >= len(objs.Configs) {
			continue
		}
		ot := objs.Configs[it.Id]
		if ot == nil {
			continue
		}
		if v, ok := ot.Params[uint32(param)]; ok {
			if iv, ok := v.(uint32); ok {
				// NAI-122 in-scope-stretch: sign-extend through int32.
				// See paramLookup in pkg/script/handlers_config.go.
				total += int(int32(iv))
				continue
			}
		}
		total += int(pt.DefaultInt)
	}
	return total
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
