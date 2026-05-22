package world

import "github.com/zsrv/goscape/pkg/script"

// apPlayerTriggerForOp returns the APPLAYER trigger for the player's
// targetOp. Returns ok=false if op is neither 1..4 nor a T/U sentinel.
// fireOpTriggerPlayer derives the OPPLAYER trigger by adding 7 to the
// returned APPLAYER (TS Player.ts:~997 offset convention):
//
//	APPLAYER1..4 (87..90) + 7 → OPPLAYER1..4 (94..97)
//	APPLAYERT    (93)     + 7 → OPPLAYERT    (100)
//	APPLAYERU    (92)     + 7 → OPPLAYERU    (99)
//
// Note: ops are 1..4, NOT 1..5 — the real client only sends OPPLAYER1..4
// (handler_op_player.go ports OPPLAYER1..4, OPPLAYERT, OPPLAYERU only).
// The 5-slot TriggerOpPlayer<5> is reserved for the AI-side family
// (TriggerAiOpPlayer1..5) and is not produced by player-actor dispatch.
func apPlayerTriggerForOp(op int) (script.ServerTriggerType, bool) {
	switch {
	case op >= 1 && op <= 4:
		return script.TriggerApPlayer1 + script.ServerTriggerType(op-1), true
	case op == targetOpPlayerT:
		return script.TriggerApPlayerT, true
	case op == targetOpPlayerU:
		return script.TriggerApPlayerU, true
	default:
		return 0, false
	}
}

// fireOpTriggerPlayer fires the [opplayer<op>,_] trigger for a Player
// target. Self = `p` (clicker), Self2 = target. Mirrors TS Player.ts:1129
// + ScriptRunner.ts:84-87: ScriptRunner.init(opTrigger, this=clicker,
// target=target_player) yields _activePlayer=clicker, _activePlayer2=target.
//
// Self2 binding flows through srv.runScript → buildPlayerScriptState's
// `case script.ActivePlayer:` arm (script.go:55-59), which sets
// state.Self2 = target and OR-s in script.PtrActivePlayer2.
//
// Closes NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER: this is the
// production producer for state.Self2. Players have no type, so the
// trigger lookup is global-only — pass (-1, -1) to fall through
// LookupKeyForType / LookupKeyForCategory to LookupKeyForGlobal in
// script.Provider.GetByTrigger (provider.go:114-127).
func fireOpTriggerPlayer(p *Player, srv *Server, target *Player) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	apTrigger, ok := apPlayerTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}
	trigger := apTrigger + 7 // APPLAYER → OPPLAYER offset

	// Reads p.targetSubject.com per TS Player.getOpTrigger:993-995 via
	// resolveTriggerTypeId — useObj override default (-1) when set.
	// Player has no NpcType/LocType/ObjType counterpart in TS so the
	// default typeId is -1 (matches TS's getOpTrigger early skip of the
	// type-fetching if-block when target is a Player).
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, -1), -1)
	// Defensive-only post-NAI-78 (goscape defensive; TS skips this
	// re-check). tryInteract pre-gates on resolved-trigger-non-nil so
	// this branch is unreachable from the hot path. Preserved for
	// non-tryInteract callers and as a goscape belt-and-braces.
	if sf == nil {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	// TS Player.ts:1129-1134 OP save/clear/exec/capture/restore. NAI-68.
	savedTarget := p.target
	p.target = nil
	p.waypointIndex = -1 // TS L1131

	// Run with `p` (clicker) as Self and `target` threaded as the
	// ActivePlayer-typed second arg → buildPlayerScriptState's
	// case-ActivePlayer arm sets state.Self2 = target, Pointers |=
	// PtrActivePlayer2 (TS-true binding per NAI-70).
	srv.runScript(sf, p, target, trigger, true, nil, nil)

	p.nextTarget = p.target
	p.target = savedTarget

	p.interactionFired = true
}

// fireApTriggerPlayer fires the [applayer<op>,_] trigger at approach
// distance. On no-script-found: sets p.apRange = -1 to skip re-lookup
// next tick (matches fireApTriggerLoc behaviour at S6r).
//
// Self/Self2 binding mirrors TS Player.ts:1151 + ScriptRunner.ts:84-87:
// ScriptRunner.init(apTrigger, this=clicker, target=target_player) →
// state.Self=clicker (`p`), state.Self2=target. Same as
// fireOpTriggerPlayer.
//
// Same-tick AP retry path active per NAI-69 T1 guard at
// interaction.go:336: handlePApRange's s.Self.SetApRange mutates
// clicker.apRangeCalled, the guard fires, and tryInteract returns
// false to allow processInteraction's walk-arm a same-tick retry.
func fireApTriggerPlayer(p *Player, srv *Server, target *Player) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	trigger, ok := apPlayerTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	// Reads p.targetSubject.com per TS Player.getApTrigger:1027-1029 via
	// resolveTriggerTypeId — useObj override default (-1) when set.
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, -1), -1)
	// Defensive-only post-NAI-78 (goscape defensive; TS skips this
	// re-check). tryInteract pre-gates on resolved-trigger-non-nil so
	// this branch is unreachable from the hot path. Preserved for
	// non-tryInteract callers and as a goscape belt-and-braces.
	if sf == nil {
		p.apRange = -1
		return
	}

	// TS L1141 — apRangeCalled pre-reset.
	p.apRangeCalled = false

	// TS Player.ts:1145-1162 AP save/clear/exec/capture/restore. NAI-68
	// framework. AP-Player same-tick retry active under the realigned
	// Self=clicker binding (NAI-70).
	savedTarget := p.target
	savedWP := p.waypoints
	savedIdx := p.waypointIndex
	p.target = nil
	p.waypointIndex = -1

	srv.runScript(sf, p, target, trigger, true, nil, nil)

	p.nextTarget = p.target
	p.target = savedTarget
	if p.nextTarget != nil {
		p.waypointIndex = -1
	} else {
		p.waypoints = savedWP
		p.waypointIndex = savedIdx
	}

	p.interactionFired = true
}
