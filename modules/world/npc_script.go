package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// Compile-time check: *Npc satisfies script.ActiveNpc.
var _ script.ActiveNpc = (*Npc)(nil)

// npcVarnCap caps the per-NPC var slice so a rogue script cannot grow
// it unboundedly. Matches the engine-wide soft cap used in S6a.
const npcVarnCap = 1024

// NpcType returns the NPC's type id.
func (n *Npc) NpcType() int { return n.typeId }

// NpcX returns the current world x coord.
func (n *Npc) NpcX() int { return n.x }

// NpcZ returns the current world z coord.
func (n *Npc) NpcZ() int { return n.z }

// NpcLevel returns the current plane/level.
func (n *Npc) NpcLevel() int { return n.level }

// NpcUID returns the packed (typeId<<16)|nid identifier.
func (n *Npc) NpcUID() int { return n.uid }

// NpcCategory returns the NPC's category, or -1 if its NpcType is nil.
func (n *Npc) NpcCategory() int {
	if n.typ == nil {
		return -1
	}
	return n.typ.Category
}

// NpcStat returns the current (boosted) stat level for the given stat id.
// Reads n.levels[stat] — seeded from typ.Stats at NewNpc time and maintained
// by ChangeType / Damage / processNpcRegen.
func (n *Npc) NpcStat(stat int) int {
	// DEVIATION NAI-17-D2: defensive bounds check — TS returns undefined
	// on OOB which coerces to NaN downstream; Go would panic, so we
	// clamp to 0.
	if stat < 0 || stat >= objtype.NpcStatCount {
		return 0
	}
	return n.levels[stat]
}

// NpcBaseStat returns the base stat level for the given stat id.
func (n *Npc) NpcBaseStat(stat int) int {
	// DEVIATION NAI-17-D2: see NpcStat above.
	if stat < 0 || stat >= objtype.NpcStatCount {
		return 0
	}
	return n.baseLevels[stat]
}

// NpcVarN reads the per-NPC var at id. Returns 0 for out-of-range ids
// (including any id never written to).
func (n *Npc) NpcVarN(id int) int32 {
	if id < 0 || id >= len(n.varns) {
		return 0
	}
	return n.varns[id]
}

// SetNpcVarN writes val to the per-NPC var at id, lazily growing the
// backing slice. Writes beyond npcVarnCap are silently dropped.
func (n *Npc) SetNpcVarN(id int, val int32) {
	if id < 0 {
		return
	}
	if id >= npcVarnCap {
		return
	}
	if id >= len(n.varns) {
		next := make([]int32, id+1)
		copy(next, n.varns)
		n.varns = next
	}
	n.varns[id] = val
}

// Teleport moves the NPC to (x, z, level), refreshes its zone
// subscription if the zone changed, and flags the client for a tele
// transition (no walk-anim interpolation). Mirrors TS
// PathingEntity.teleport at PathingEntity.ts:267-298.
//
// Used by NPC_TELE script handler (pkg/script/handlers_npc.go) and by
// AI teleport sites — wanderMode home-tele (npc_interaction.go ~:95)
// and patrolMode waypoint-tele (~:121).
//
// DEVIATION NAI-34-D3, D4 (both entities) and NAI-34-D5-NPC vs TS
// PathingEntity.teleport (PathingEntity.ts:267) — partial closure as of
// NAI-36-T7:
//
// CLOSED in NAI-36-T7:
//   - D1 (level clamp to [0, 3]) — closed for both Npc.Teleport and
//     Player.Teleport.
//   - D2 (unallocated-zone reject via IsZoneAllocated) — closed for both
//     entities.
//   - D5-Player (level-change → moveSpeed=INSTANT + jump=true) — closed
//     for Player only; Npc lacks jump field.
//
// RESIDUAL (active deviations, both entities):
//   - D3-Player + D3-NPC: no focus() call (PathingEntity.ts:286).
//     Player has FaceCoord at player_masks.go:45; Npc has focus() at
//     npc_interaction.go:686 AND FaceCoord at npc_masks.go:120.
//     Neither side is dead-API gated, but closure was deferred because
//     fine-coord conversion + instant-flag semantics need cross-entity
//     design. Tracked for future "pathing-entity-focus-and-step-tracking"
//     sub-spec.
//   - D4-Player + D4-NPC: no lastStepX/Z adjust (PathingEntity.ts:289-290).
//     Player has lastStepX/Z fields (player.go:79); Npc does NOT. Adding
//     to Npc is dead-API per dead_api_polish.md until a consumer
//     materializes; Player-side closure deferred for symmetry. Tracked
//     for the same future sub-spec.
//   - D5-NPC: no `previousLevel != level → INSTANT + jump=true` branch
//     on Npc. Npc has no jump field; dead-API foot-gun. (D5-Player closed
//     in NAI-36-T7.) Tracked for the same future sub-spec.
//
// Body order (refresh, then tele = true) matches TS
// PathingEntity.ts:290-293; Player.Teleport's order was aligned to
// match in NAI-36-T7.
func (n *Npc) Teleport(x, z, level int) {
	// D1: clamp level to [0, 3] per PathingEntity.ts:268-271.
	if level < 0 {
		level = 0
	} else if level > 3 {
		level = 3
	}
	// D2: reject teleports to unallocated zones per PathingEntity.ts:273-278.
	if n.server != nil && !n.server.IsZoneAllocated(level, x, z) {
		return
	}

	prevX, prevZ, prevLevel := n.x, n.z, n.level
	n.x, n.z, n.level = x, z, level
	refreshNpcZone(n.server, n, prevX, prevZ, prevLevel)
	n.tele = true
}

// TargetOp returns n.targetOp. ActiveNpc interface adapter for NPC_GETMODE
// (NAI-36).
func (n *Npc) TargetOp() int {
	return n.targetOp
}

// ClearInteraction is the ActiveNpc-interface adapter for n.clearInteraction
// (NAI-36). Production caller is the NPC_SETMODE script handler.
func (n *Npc) ClearInteraction() {
	n.clearInteraction()
}

// ResetDefaults is the ActiveNpc-interface adapter for n.resetDefaults
// (NAI-36). Production caller is the NPC_SETMODE script handler (NULL
// mode + no-target fallthrough).
func (n *Npc) ResetDefaults() {
	n.resetDefaults()
}

// ClearPatrol is the ActiveNpc-interface adapter for n.clearPatrol
// (NAI-36). Production caller is the NPC_SETMODE script handler when
// the new mode is PATROL.
func (n *Npc) ClearPatrol() {
	n.clearPatrol()
}

// SetTargetOp sets n.targetOp directly (no interaction binding).
// ActiveNpc-interface adapter used by NPC_SETMODE for both clear-target
// and target-binding branches (NAI-36).
func (n *Npc) SetTargetOp(mode int) {
	n.targetOp = mode
}

// SetInteractionScript binds the NPC's interaction to target via
// InteractionScript with mode as the targetOp/op argument. Type-switches
// the script-side script.Active* interface value to the underlying
// world-side concrete entity, then delegates to n.SetInteraction. Mirrors
// TS Npc.setInteraction(Interaction.SCRIPT, target, mode) at
// NpcOps.ts:225-228.
//
// The 4-arg signature passes com=-1 as the TS-faithful "no com" sentinel
// (TS calls setInteraction with only 3 args; goscape carries the
// targetSubject.com snapshot as a fourth arg with -1 sentinel matching
// the TS `com ? com : -1` coercion at SetInteraction body).
//
// Passing nil is a defensive no-op: the caller (NPC_SETMODE handler)
// already routes null targets through resetDefaults instead.
func (n *Npc) SetInteractionScript(target any, mode int) {
	var ent entity
	switch t := target.(type) {
	case *Player:
		ent = t
	case *Npc:
		ent = t
	case *entitypkg.Loc:
		ent = t
	case *entitypkg.Obj:
		ent = t
	default:
		// Should not happen — script-side resolution narrows to one of
		// the four concrete world-side types via interface dispatch.
		// If the type-switch misses, no-op rather than panic so
		// production stays alive.
		return
	}
	n.SetInteraction(InteractionScript, ent, mode, -1)
}

// buildNpcScriptState initialises a ScriptState for an NPC-anchored
// script run. Pure — no side effects on server state — so callers can
// test the target-dispatch logic in isolation.
//
// NAI-11: target may be nil (AI_TIMER/DESPAWN/QUEUE* paths), or a
// concrete value satisfying one of the Active* interfaces. The
// type-switch wires the matching ScriptState field and pointer flag.
// Check order matters: ActivePlayer first (most specific), ActiveNpc
// last (so a Player target doesn't accidentally fall into the
// OtherActiveNpc branch via interface promotion). This mirrors the TS
// ScriptRunner.init target-dispatch at Engine-TS/.../ScriptRunner.ts:84-116.
func (s *Server) buildNpcScriptState(
	sf *script.ScriptFile,
	npc script.ActiveNpc,
	target any,
	intArgs []int,
	stringArgs []string,
) *script.ScriptState {
	state := script.Init(sf, nil, false, intArgs, stringArgs)
	state.ActiveNpc = npc
	state.Pointers |= script.PtrActiveNpc
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	state.Npcs = s.npcLookup
	state.LineValidator = s.scriptLineValidator()

	switch t := target.(type) {
	case nil:
		// No secondary pointer.
	case script.ActivePlayer:
		// TS: self=Npc, target=Player → _activePlayer = target, PtrActivePlayer.
		state.Self = t
		state.Pointers |= script.PtrActivePlayer
	case script.ActiveLoc:
		state.ActiveLoc = t
		state.Pointers |= script.PtrActiveLoc
	case script.ActiveObj:
		state.ActiveObj = t
		state.Pointers |= script.PtrActiveObj
	case script.ActiveNpc:
		state.OtherActiveNpc = t
		state.Pointers |= script.PtrOtherActiveNpc
	}

	return state
}

// runNpcScript initialises a ScriptState anchored on npc (not a
// player) and routes the result via resumeOrFinishNpc. Safe to call
// with a nil scriptFile (no-op) so callers don't have to nil-check
// the trigger lookup. Mirrors runScript at modules/world/script.go:14.
//
// NAI-11: target is the interaction target for AI_*PLAYER / AI_*NPC /
// AI_*LOC / AI_*OBJ triggers. Pass nil for AI_TIMER, DESPAWN, and
// QUEUE* paths (no secondary entity). The type-switch in
// buildNpcScriptState wires the matching ScriptState field and pointer
// flag; ActivePlayer is checked first so a *Player target never
// accidentally falls into the OtherActiveNpc branch.
//
// If the script suspends (Execution == NpcSuspended), the state is
// stored on the NPC and Npc.turn() resumes it when the NPC's delay
// expires via the prefix block added in NAI-2.
func (s *Server) runNpcScript(
	sf *script.ScriptFile,
	npc script.ActiveNpc,
	target any,
	intArgs []int,
	stringArgs []string,
) {
	if sf == nil {
		return
	}
	state := s.buildNpcScriptState(sf, npc, target, intArgs, stringArgs)
	s.resumeOrFinishNpc(state, npc)
}

// resumeOrFinishNpc is the shared post-Execute handler for both fresh
// NPC-anchored runs (from runNpcScript) and resumed runs (from
// Npc.turn()). Mirrors resumeOrFinish at modules/world/script.go:30
// but routes via the ActiveNpc interface instead of ActivePlayer.
func (s *Server) resumeOrFinishNpc(state *script.ScriptState, npc script.ActiveNpc) {
	if err := script.Execute(state); err != nil {
		s.log.Warn("npc script execute error",
			"script", state.Script.Name, "err", err)
		npc.ClearActiveScript()
		return
	}
	switch state.Execution {
	case script.Finished, script.Aborted:
		npc.ClearActiveScript()
	case script.NpcSuspended:
		npc.StoreActiveScript(state)
	case script.WorldSuspended:
		// NAI-37: npc-bound script suspended to world queue. Symmetric
		// to resumeOrFinish (player path). Mirrors TS Npc.ts:219-220.
		//
		// NAI-44: TS Npc.executeScript (L226-228) only nulls activeScript
		// on FINISHED/ABORTED. Same logic as the player-path: holding
		// the pointer is safe because Npc.turn() does not re-fire
		// WorldSuspended states.
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
	default:
		// Suspended / PauseButton / CountDialog —
		// not reachable via npc_delay alone, but defensively clear.
		s.log.Warn("npc script in unexpected execution state",
			"script", state.Script.Name, "execution", state.Execution)
		npc.ClearActiveScript()
	}
}

// processNpcTimer fires the ai_timer trigger script when timerClock
// reaches timerInterval. Matches TS Npc.processTimers at
// Engine-TS/.../Npc.ts:527-536.
//
// Behaviour:
//   - No-op while delayed (TS gates via the isValid return in
//     turn(); Go gates internally).
//   - No-op when timerInterval <= 0 (unset or explicitly disabled
//     via SetTimer with a non-positive value).
//   - timerClock increments once per call when conditions pass.
//   - timerClock resets to 0 ONLY after a successful script fire.
//     If no ai_timer trigger script is registered for the NPC's
//     type, timerClock stays at threshold and retries every tick —
//     matches TS's "script may be registered later" semantics.
func (s *Server) processNpcTimer(n *Npc) {
	if n.delayed || n.timerInterval <= 0 {
		return
	}
	n.timerClock++
	if n.timerClock < n.timerInterval {
		return
	}
	if n.typ == nil || s.scriptProvider == nil {
		return
	}
	sf := s.scriptProvider.GetByTrigger(script.TriggerAiTimer, n.typeId, n.typ.Category)
	if sf == nil {
		return
	}
	s.runNpcScript(sf, n, nil, nil, nil)
	n.timerClock = 0
}

// processNpcRegen ticks the regen clock and, on interval elapse,
// reloads the interval from NpcType.RegenRate and converges every
// levels[i] one step toward baseLevels[i]. Matches TS Npc.processRegen
// at Engine-TS/.../Npc.ts:505-525.
//
// Behaviour:
//   - regenClock increments unconditionally when called (TS has no
//     internal delayed gate; the caller's isValid check handles it).
//   - When regenClock hits regenInterval: reload regenInterval from
//     n.typ.RegenRate (Vorkath-changetype quirk: new rate takes
//     effect on fire, not on changetype), reset clock to 0, and
//     iterate all 6 stat slots, moving each one step toward its base
//     (TS Npc.ts:515-523).
func (s *Server) processNpcRegen(n *Npc) {
	n.regenClock++
	if n.regenClock < n.regenInterval {
		return
	}
	if n.typ != nil {
		n.regenInterval = int(n.typ.RegenRate)
	}
	n.regenClock = 0
	// NAI-17: iterate all 6 stats, converging levels[i] toward
	// baseLevels[i]. Mirrors TS Npc.ts:515-523.
	for i := range objtype.NpcStatCount {
		switch {
		case n.levels[i] < n.baseLevels[i]:
			n.levels[i]++
		case n.levels[i] > n.baseLevels[i]:
			n.levels[i]--
		}
	}
}

// processNpcQueue walks the NPC's queue, decrementing delays and
// firing ready entries as fresh NPC-anchored script runs. Iterates
// by index so a request appended mid-pass (via a fired script calling
// EnqueueScriptForTrigger again) is visible in the same iteration —
// preserves TS's "speedup quirk" at Npc.ts:538-560.
//
// Delay only decrements when the NPC is not delayed (TS Npc.ts:544-547
// "purposely only decrements the delay when the npc is not delayed").
// Removal happens BEFORE firing so a re-entrant enqueue doesn't
// collide with the index pointer. Matches the player-side pattern at
// modules/world/tick.go:219-242.
func (s *Server) processNpcQueue(n *Npc) {
	if n.typ == nil {
		return
	}
	i := 0
	for i < len(n.queue) {
		req := &n.queue[i]
		if !n.delayed {
			req.Delay--
		}
		if n.delayed || req.Delay > 0 {
			i++
			continue
		}
		trigger := req.Trigger
		intArg := req.IntArg
		n.queue = append(n.queue[:i], n.queue[i+1:]...)
		if s.scriptProvider == nil {
			continue
		}
		sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.typ.Category)
		s.runNpcScript(sf, n, nil, []int{intArg}, nil)
		// Don't advance i — removed current element.
	}
}
