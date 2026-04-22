package world

import "github.com/zsrv/goscape/pkg/script"

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
// S6a: only HP (id 0) is real; other stat ids return 0 (TODO: full NPC stats).
func (n *Npc) NpcStat(stat int) int {
	if stat == 0 {
		return n.curHP
	}
	return 0
}

// NpcBaseStat returns the base stat level for the given stat id.
// S6a: only HP (id 0) is real; other stat ids return 0 (TODO: full NPC stats).
func (n *Npc) NpcBaseStat(stat int) int {
	if stat == 0 {
		return n.baseHP
	}
	return 0
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

// runNpcScript initialises a ScriptState anchored on npc (not a
// player) and routes the result via resumeOrFinishNpc. Safe to call
// with a nil scriptFile (no-op) so callers don't have to nil-check
// the trigger lookup. Mirrors runScript at modules/world/script.go:14.
//
// If the script suspends (Execution == NpcSuspended), the state is
// stored on the NPC and Npc.turn() resumes it when the NPC's delay
// expires via the prefix block added in NAI-2.
func (s *Server) runNpcScript(sf *script.ScriptFile, npc script.ActiveNpc, intArgs []int, stringArgs []string) {
	if sf == nil {
		return
	}
	state := script.Init(sf, nil, false, intArgs, stringArgs)
	state.ActiveNpc = npc
	state.Pointers |= script.PtrActiveNpc
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
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
	default:
		// Suspended / PauseButton / CountDialog / WorldSuspended —
		// not reachable via npc_delay alone, but defensively clear.
		s.log.Warn("npc script in unexpected execution state",
			"script", state.Script.Name, "execution", state.Execution)
		npc.ClearActiveScript()
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
		s.runNpcScript(sf, n, []int{intArg}, nil)
		// Don't advance i — removed current element.
	}
}
