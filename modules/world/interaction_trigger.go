package world

import (
	"github.com/zsrv/goscape/pkg/script"
)

// tryFireOpTrigger fires the [opnpc<op>,<npcType>] trigger for the player's
// anchored NPC target when the player has just reached interaction range.
// Matches TS Player.tryInteract() for the NPC branch.
//
// Preconditions (guaranteed by caller — Player.processInteraction):
//   - p.interacted == true
//   - p.interactionKind == InteractionEngine
//   - p.target != nil
//   - p.interactionFired == false
//
// Behaviour:
//   - Target not *Npc: no-op; set interactionFired so we don't retry.
//     (OPLOC/OPOBJ branches will extend this switch in a later sub-spec.)
//   - Player became delayed between reach and dispatch: defer; leave
//     interactionFired false so we retry next tick.
//   - NPC dead: clear interaction silently.
//   - targetOp out of [1,5]: clear interaction silently.
//   - No script found (type/category/global): clear interaction silently.
//   - Script suspends (P_DELAY/P_PAUSEBUTTON/P_COUNTDIALOG): keep
//     interaction anchored; resumeOrFinish already stored the state.
//   - Script finishes / aborts: clear interaction.
func tryFireOpTrigger(p *Player) {
	srv := p.client.server

	npc, ok := p.target.(*Npc)
	if !ok {
		p.interactionFired = true
		return
	}

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
