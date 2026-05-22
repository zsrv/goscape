package world

import (
	"github.com/zsrv/goscape/pkg/script"
)

// scriptLineValidator returns the LineValidator wired into all script
// state-init sites and into serverNpcLookup's huntvisGate (Prong B of
// the NAI-33-D1 retire). Lookup order: lineValidatorOverride (test
// seam) → gamemap.Pathfinder.LineValidator → nil. Production callers
// always have gamemap set via New(); the test seam is for fixtures
// that need a stub validator without a real gamemap. Distance + HuntAll
// passesFilter and huntvisGate all pessimistically allow on a nil
// return, so the test path degrades gracefully. NAI-35-T3.
func (s *Server) scriptLineValidator() script.LineValidator {
	if s.lineValidatorOverride != nil {
		return s.lineValidatorOverride
	}
	if s.gamemap == nil {
		return nil
	}
	return s.gamemap.Pathfinder.LineValidator
}

// buildPlayerScriptState initialises a ScriptState for a player-anchored
// fresh run. Pure — no side effects on server state — so callers can
// test the target-dispatch logic in isolation.
//
// NAI-39: target may be nil (the common case — no secondary entity), or
// a concrete value satisfying one of the Active* interfaces. The
// type-switch wires the matching ScriptState field and pointer flag,
// mirroring buildNpcScriptState's NAI-11 shape (npc_script.go:225-261)
// and the TS ScriptRunner.init target-dispatch at ScriptRunner.ts:84-116.
//
// case script.ActivePlayer is the secondary-binding arm consumed by
// the OPPLAYER<N>/APPLAYER<N> player→player trigger family
// (player_interaction_trigger.go). Sets state.Self2 = target +
// PtrActivePlayer2 when the second arg is a *Player. Mirrors TS
// ScriptRunner.ts:84-87 _activePlayer2 dispatch (self=Player &&
// target=Player → _activePlayer2=target). NAI-40 closure of the
// activePlayer2 substrate; NAI-70 realigned the call sites in
// player_interaction_trigger.go to TS-true binding.
func (s *Server) buildPlayerScriptState(
	sf *script.ScriptFile,
	self script.ActivePlayer,
	target any,
	trigger script.ServerTriggerType,
	protect bool,
	intArgs []int,
	stringArgs []string,
) *script.ScriptState {
	state := script.Init(sf, self, protect, intArgs, stringArgs)
	state.Trigger = trigger
	state.NodeDebug = s.cfg.NodeDebug
	state.Log = s.log
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	state.LocOps = s.locOps
	state.Npcs = s.npcLookup
	state.PlayerLookup = s
	state.LineValidator = s.scriptLineValidator()

	switch t := target.(type) {
	case nil:
		// No secondary pointer.
	case script.ActivePlayer:
		// TS: self=Player, target=Player → _activePlayer2 = target,
		// PtrActivePlayer2 (ScriptRunner.ts:84-87).
		state.Self2 = t
		state.Pointers |= script.PtrActivePlayer2
	case script.ActiveNpc:
		state.ActiveNpc = t
		state.Pointers |= script.PtrActiveNpc
	case script.ActiveLoc:
		state.ActiveLoc = t
		state.Pointers |= script.PtrActiveLoc
	case script.ActiveObj:
		state.ActiveObj = t
		state.Pointers |= script.PtrActiveObj
	}

	return state
}

// runScript initialises a ScriptState for a fresh invocation and routes
// the result via resumeOrFinish. Safe to call with a nil scriptFile
// (no-op) so callers don't have to nil-check the trigger lookup.
//
// If the script suspends (Execution == Suspended), the state is stored
// on the active player and the tick loop will resume it when the
// player's delay expires via processActiveScripts.
//
// NAI-39: target is the secondary-entity binding for triggers that
// dispatch through an active_player2 / active_npc / active_loc /
// active_obj slot. Pass nil when there is no secondary binding (the
// common case — engine-dispatched timers, queue runs, login).
func (s *Server) runScript(
	sf *script.ScriptFile,
	self script.ActivePlayer,
	target any,
	trigger script.ServerTriggerType,
	protect bool,
	intArgs []int,
	stringArgs []string,
) {
	if sf == nil {
		return
	}
	state := s.buildPlayerScriptState(sf, self, target, trigger, protect, intArgs, stringArgs)
	s.resumeOrFinish(state, self)
}

// resumeOrFinish is the shared post-Execute handler for both fresh runs
// (from runScript) and resumed runs (from processActiveScripts). It
// drives the state-store / state-clear decision in one place so the
// tick loop doesn't need to type-assert back to *Player.
func (s *Server) resumeOrFinish(state *script.ScriptState, self script.ActivePlayer) {
	if err := script.Execute(state); err != nil {
		s.log.Warn("script execute error",
			"script", state.Script.Name, "err", err)
		// NAI-55: fall through. script.Execute sets state.Execution =
		// Aborted on every error path (pkg/script/runner.go:54-83),
		// so the switch routes via OnScriptFinishedOrAborted —
		// match-guarded, identical to a clean Aborted. Mirrors TS
		// ScriptRunner.execute setting state.execution = ABORTED on
		// throw (ScriptRunner.ts:228), then Player.executeScript
		// re-entering the (script === this.activeScript) guard
		// (Player.ts:2143-2148). Closes NAI-54-F1.
	}
	switch state.Execution {
	case script.Finished, script.Aborted:
		// NAI-54: TS Player.ts:2143-2148 — only nulls activeScript when
		// state matches, and additionally fires CloseModal(false) on
		// no-MAIN-modal. Both behaviors live in OnScriptFinishedOrAborted.
		self.OnScriptFinishedOrAborted(state)
	case script.Suspended, script.PauseButton, script.CountDialog:
		self.StoreActiveScript(state)
	case script.WorldSuspended:
		// NAI-37 T10 / NAI-155: player-bound script suspended to world
		// queue. Pop the wakeup-tick (pushed before WORLD_DELAY — see
		// handlers_server.go:87-108) and enqueue.
		//
		// Clear p.activeScript (via self.ClearActiveScript) BEFORE
		// enqueue. TS Player.executeScript:2135-2136 does NOT assign
		// script.activePlayer.activeScript in the WORLD_SUSPENDED arm —
		// neither does it set this.protect — so the player's protect
		// boolean remains false during the world-queue wait window.
		// Goscape's NAI-44 divergence (hold the pointer for "safe
		// resume") was incorrect: it made protectedScriptActive() return
		// true for the entire wait window, blocking CanAccess and all
		// interactions. The resume gate (tick.go:281) is doubly guarded
		// (non-nil AND Execution==Suspended), so a nil activeScript
		// produces no false-resume. Retiring NAI-44-D-WORLDSUSPENDED-HOLD.
		self.ClearActiveScript()
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
	default:
		// Defensive: player-side scripts cannot reach NpcSuspended
		// (NPC_DELAY / NPC_ARRIVEDELAY require ActiveNpc, set at
		// handlers_npc.go:446/:492) and there are no other unhandled
		// Execution states. NpcSuspended is dispatched for world-queue
		// scripts at resumeOrFinishWorld (script.go:202).
		s.log.Warn("script in unsupported execution state",
			"script", state.Script.Name, "execution", state.Execution)
		self.ClearActiveScript()
	}
}

// resumeOrFinishWorld dispatches the post-Execute state for a script
// run from the world-script queue (called by processWorldQueue after
// removing the entry).
//
// Dispatch table mirrors TS World.processWorld (World.ts:530-560):
//   - Finished, Aborted: clean exit (entry already unlink()'d).
//   - WorldSuspended: re-enqueue (self-loop). Pops the wakeup-tick from
//     the script's int stack and re-appends to worldScriptQueue.
//   - Suspended: rebind to state.Self.activeScript (TS L548-549).
//   - NpcSuspended: rebind to state.ActiveNpc.activeScript (TS L550-552).
//   - PauseButton, CountDialog: silent fall-through (TS World.processWorld
//     has no branch for these; matches TS behavior).
//   - default (Running, future-added): warn+drop.
//
// NAI-44 closure of NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP.
func (s *Server) resumeOrFinishWorld(state *script.ScriptState) {
	if err := script.Execute(state); err != nil {
		s.log.Warn("world script execute error",
			"script", state.Script.Name, "err", err)
		return
	}
	switch state.Execution {
	case script.Finished, script.Aborted:
		// Clean exit; nothing to do (entry already removed by caller).
	case script.WorldSuspended:
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
	case script.Suspended:
		// TS World.ts:548-549 — bind to script.activePlayer.activeScript.
		// The "(probably not needed)" TS comment notes this case isn't
		// expected from world-queued scripts in practice, but the binding
		// exists for completeness.
		if state.Self != nil {
			state.Self.StoreActiveScript(state)
		} else {
			s.log.Warn("world-queue script Suspended with nil Self; dropping",
				"script", state.Script.Name)
		}
	case script.NpcSuspended:
		// TS World.ts:550-552 — bind to script.activeNpc.activeScript.
		if state.ActiveNpc != nil {
			state.ActiveNpc.StoreActiveScript(state)
		} else {
			s.log.Warn("world-queue script NpcSuspended with nil ActiveNpc; dropping",
				"script", state.Script.Name)
		}
	case script.PauseButton, script.CountDialog:
		// TS World.processWorld (World.ts:530-560) has no branch for these
		// states. request.unlink() at L545 already removed the entry, so
		// they are silently dropped. Match TS by intentionally falling
		// through with no rebind and no warn.
	default:
		// Running, or any future-added Execution value.
		s.log.Warn("world-queue script in unexpected execution state",
			"script", state.Script.Name, "execution", state.Execution)
	}
}
