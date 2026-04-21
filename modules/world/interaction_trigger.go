package world

import (
	"slices"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/script"
)

// tryFireOpTrigger fires the [op<entity><op>,<typeID>] trigger for the
// player's anchored target when the player has just reached interaction
// range. Matches TS Player.tryInteract() for the Engine-kind interaction.
//
// Preconditions (guaranteed by caller — Player.processInteraction):
//   - p.interacted == true
//   - p.interactionKind == InteractionEngine
//   - p.target != nil
//   - p.interactionFired == false
//
// Branches by target concrete type. Common behaviour across branches:
//   - Player became delayed between reach and dispatch: defer; leave
//     interactionFired false so we retry next tick.
//   - Lifecycle gate fail (NPC dead / Loc despawn-or-mutated): silent
//     clear interaction.
//   - targetOp out of [1,5]: silent clear.
//   - No script found (type/category/global): silent clear.
//   - Script suspends (P_DELAY/P_PAUSEBUTTON/P_COUNTDIALOG): keep
//     interaction anchored; resumeOrFinish already stored the state.
//   - Script finishes / aborts: clear interaction.
//
// (OPOBJ branch will extend this switch in a later sub-spec.)
func tryFireOpTrigger(p *Player) {
	srv := p.client.server

	switch tgt := p.target.(type) {
	case *Npc:
		fireOpTriggerNpc(p, srv, tgt)
	case *entitypkg.Loc:
		fireOpTriggerLoc(p, srv, tgt)
	default:
		// Target type not handled by any branch: skip; mark fired so we
		// don't retry every tick.
		p.interactionFired = true
	}
}

// fireOpTriggerNpc fires the [opnpc<op>,<npcType>] trigger. Extracted
// from the original tryFireOpTrigger body verbatim.
func fireOpTriggerNpc(p *Player, srv *Server, npc *Npc) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	if npc.dead {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	op := p.targetOp
	if op < 1 || op > 5 {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	trigger := script.TriggerOpNpc1 + script.ServerTriggerType(op-1)
	category := 0
	if npc.typ != nil {
		category = npc.typ.Category
	}

	sf := srv.scriptProvider.GetByTrigger(trigger, npc.typeId, category)
	if sf == nil {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	state := script.Init(sf, p, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= script.PtrActiveNpc
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup

	srv.resumeOrFinish(state, p)

	if state.Execution == script.Finished || state.Execution == script.Aborted {
		p.ClearInteraction()
	}
	p.interactionFired = true
}

// fireOpTriggerLoc fires the [oploc<op>,<locType>] trigger.
//
// Lifecycle gate: locStillValid checks BOTH zone membership (loc still
// in zoneMap.Get(level,x,z).Locs by pointer) AND type match (loc.Type()
// == targetSubject.typ). The combined check defends against:
//   - In-place Info mutation (tree → stump via Loc.Info bitfield change)
//   - Removal from zone (loc despawned, axed, etc.)
//
// DEVIATION S6j-D2: TS handler sets targetOp=APLOC1+(op-1) and engine
// fires APLOC at approach range, OPLOC at contact. We fire OPLOC
// directly (no APLOC fallback) — inherits S6b OPNPC convention.
// Follow-up: "approach-vs-operate range gating" sub-spec.
//
// DEVIATION S6j-D4: Loc has no cached typ pointer (only the packed
// Info bitfield); category lookup goes through srv.locTypes.Configs[typeID],
// unlike *Npc which reads npc.typ.Category from a cached pointer.
func fireOpTriggerLoc(p *Player, srv *Server, loc *entitypkg.Loc) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	if !locStillValid(srv, loc, p.targetSubject.typ, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level) {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	op := p.targetOp
	if op < 1 || op > 5 {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	trigger := script.TriggerOpLoc1 + script.ServerTriggerType(op-1)
	// Loc has no cached LocType pointer (only packed Info bitfield);
	// resolve category through the LocType registry. Use direct slice
	// access matching the server_configs.go pattern (LocTypeConfigs has
	// no Get method).
	category := 0
	if locId := loc.Type(); locId >= 0 && locId < len(srv.locTypes.Configs) {
		if lt := srv.locTypes.Configs[locId]; lt != nil {
			category = lt.Category
		}
	}

	sf := srv.scriptProvider.GetByTrigger(trigger, loc.Type(), category)
	if sf == nil {
		// S6j-D7 closed in S6k: defaultOp fallback. TS Player.ts:~1095
		// fires "Nothing interesting happens." when the player reaches
		// contact range and no op-trigger is registered for this loc.
		// Message infra was already in place (Player.MessageGame at
		// modules/world/message_game.go); S6j's "needs message infra"
		// concern was spurious.
		p.MessageGame("Nothing interesting happens.")
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	state := script.Init(sf, p, false, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= script.PtrActiveLoc
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup

	srv.resumeOrFinish(state, p)

	if state.Execution == script.Finished || state.Execution == script.Aborted {
		p.ClearInteraction()
	}
	p.interactionFired = true
}

// locStillValid checks whether the held *Loc pointer still represents
// the same loc the player clicked. Two checks combined — both required
// because each defends against a different mutation:
//   - Zone membership: catches loc removal (e.g., axed tree).
//   - Type match: catches in-place Info mutation (e.g., tree → stump
//     via the same *Loc pointer; Loc's docstring explicitly notes
//     "callers can mutate Info in place ... without re-allocating").
func locStillValid(srv *Server, loc *entitypkg.Loc, wantType, wantX, wantZ, wantLevel int) bool {
	if loc.Type() != wantType {
		return false
	}
	zn := srv.zoneMap.Get(wantLevel, wantX, wantZ)
	return slices.Contains(zn.Locs, loc)
}
