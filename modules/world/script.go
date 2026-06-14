package world

import (
	"fmt"

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
	// Protected-access guard — TS Player.runScript (Player.ts:2094):
	//   if (!force && protect && (this.protect || this.delayed)) return -1;
	// A fresh protected script cannot acquire protected access while the
	// player already holds it (a protected script suspended on a chat/
	// choice dialogue keeps protectedScriptActive() true) or is delayed.
	// Resumes bypass runScript — they call resumeOrFinish directly, the
	// force=true path — so no `force` parameter is needed here. Without
	// this guard an opheld/opheldu/opheldt/if_button/inv_button fired
	// mid-dialogue would execute, enabling item dupes against the
	// consume-after-yield content pattern (drop the input during the
	// dialogue; the post-resume inv_del removes nothing but the reward
	// is still granted). The immediate-run handlers reach this method
	// without an upstream CanAccess gate, so the guard lives here.
	if protect {
		if p, ok := self.(*Player); ok && (p.protectedScriptActive() || p.delayed) {
			return
		}
	}
	state := s.buildPlayerScriptState(sf, self, target, trigger, protect, intArgs, stringArgs)
	// TS Player.ts:2103 — set this.protect = true at the start of a
	// protected script execution. The post-Execute clear lives in
	// resumeOrFinish so that the re-set/preserve logic for
	// Suspended/PauseButton/CountDialog can read state.Pointers&PAP at
	// dispatch. NAI-111-D1.
	if protect {
		if p, ok := self.(*Player); ok {
			p.protect = true
		}
	}
	s.resumeOrFinish(state, self)
}

// resumeOrFinish is the shared post-Execute handler for both fresh runs
// (from runScript) and resumed runs (from processActiveScripts). It
// drives the state-store / state-clear decision in one place so the
// tick loop doesn't need to type-assert back to *Player.
func (s *Server) resumeOrFinish(state *script.ScriptState, self script.ActivePlayer) {
	if err := script.Execute(state); err != nil {
		// 2e3bcf43 (254 pin-advance): TS ScriptRunner.ts logs the player
		// error console line by username only —
		// `printError("Player script error - username:${username}")` —
		// the 244-era pid attr is gone. Goscape folds this into the
		// structured warn log attrs via the variadic extra path. Nil
		// guard mirrors handlePlayerScriptError's (production callers
		// always pass a live player; defensive only).
		var extra []any
		if self != nil {
			extra = []any{"username", self.Username()}
		}
		s.logScriptExecuteError("script execute error", state, err, extra...)
		s.handlePlayerScriptError(state, self, err)
		// NAI-55: fall through. script.Execute sets state.Execution =
		// Aborted on every error path (pkg/script/runner.go:54-83),
		// so the switch routes via OnScriptFinishedOrAborted —
		// match-guarded, identical to a clean Aborted. Mirrors TS
		// ScriptRunner.execute setting state.execution = ABORTED on
		// throw (ScriptRunner.ts:228), then Player.executeScript
		// re-entering the (script === this.activeScript) guard
		// (Player.ts:2143-2148). Closes NAI-54-F1.
	}
	// TS Player.ts:2110 — unconditional post-Execute clear of this.protect.
	// The Suspended/PauseButton/CountDialog dispatch arm re-sets below
	// when the script remains player-bound with PAP still set
	// (TS Player.ts:2141). NAI-111-D1.
	if p, ok := self.(*Player); ok {
		p.protect = false
	}
	switch state.Execution {
	case script.Finished, script.Aborted:
		// NAI-54: TS Player.ts:2143-2148 — only nulls activeScript when
		// state matches, and additionally fires CloseModal(false) on
		// no-MAIN-modal. Both behaviors live in OnScriptFinishedOrAborted.
		self.OnScriptFinishedOrAborted(state)
	case script.Suspended, script.PauseButton, script.CountDialog:
		self.StoreActiveScript(state)
		// TS Player.ts:2141 — `script.activePlayer.protect = protect` to
		// preserve protected access across the suspend. Goscape derives
		// from state.Pointers&PAP (which Init at runner.go:38 sets iff
		// protect=true at script start, and StoreActiveScript preserves
		// across suspends), exactly matching the TS preserve-when-delayed
		// semantics. CloseModal still cleanly clears p.protect=false
		// without re-deriving here (its clear persists until the next
		// Execute cycle re-runs and re-evaluates this branch).
		// NAI-111-D1.
		if p, ok := self.(*Player); ok && state.Pointers&script.PtrProtectedActivePlayer != 0 {
			p.protect = true
		}
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
	case script.NpcSuspended:
		// TS Player.executeScript (Player.ts:2220-2221):
		//   } else if (state === ScriptState.NPC_SUSPENDED) {
		//       script.activeNpc.activeScript = script;
		// A player-anchored script can carry an ActiveNpc (every opnpc /
		// apnpc trigger does — buildPlayerScriptState binds target → state
		// .ActiveNpc), so NPC_DELAY / NPC_ARRIVEDELAY transition it to
		// NpcSuspended. The continuation is stored ON THE ACTIVE NPC, not
		// the player: Npc.turn() (npc_ai.go:20-25, mirroring Npc.ts:116-118)
		// resumes it when the NPC's delay expires. The player's own
		// activeScript and protect are intentionally left untouched — TS
		// only rebinds activePlayer in the player-suspend else-arm.
		//
		// This was previously dropped by the default arm (whose comment
		// wrongly assumed "player-side scripts cannot reach NpcSuspended").
		// The drop broke every opnpc/apnpc that delays-then-acts on its NPC.
		// Visible symptom: the Strange Plant (triffid) random event —
		// macro_event_triffid.rs2's [opnpc1] pick handler ends with
		// `npc_delay(22); npc_del;`; dropping the continuation left the plant
		// delayed-but-never-deleted, so its hostile ai_timer resumed and
		// attacked the player after they had picked the fruit.
		if state.ActiveNpc != nil {
			state.ActiveNpc.StoreActiveScript(state)
		} else {
			s.log.Warn("player script reached NpcSuspended with nil ActiveNpc; dropping",
				"script", state.Script.Name)
		}
	default:
		// Unhandled non-terminal Execution value (Running, or any
		// future-added state). WorldSuspended/NpcSuspended/Suspended/
		// PauseButton/CountDialog all have explicit arms above.
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
		// 2e3bcf43 (254 pin-advance): TS's username-only printError
		// (`Player script error - username:${username}`) sits in the
		// SHARED catch and fires for any player-anchored script —
		// including world-queue resumes routed through here. The 244-era
		// pid attr is gone.
		var extra []any
		if state.Self != nil {
			extra = []any{"username", state.Self.Username()}
		}
		s.logScriptExecuteError("world script execute error", state, err, extra...)
		// World-queue scripts can be either player- or npc-anchored
		// (resumeOrFinishWorld is the shared post-Execute path for both
		// suspension classes). Route the side-effects on the anchor
		// type. Mirrors TS ScriptRunner.ts:188-213 — the catch block
		// branches on `state.self instanceof Player` /
		// `state.self instanceof Npc` independently of where the script
		// resumed from.
		switch {
		case state.Self != nil:
			s.handlePlayerScriptError(state, state.Self, err)
		case state.ActiveNpc != nil:
			if realNpc, ok := state.ActiveNpc.(*Npc); ok {
				s.handleNpcScriptError(state, realNpc, err)
			}
		}
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

// logScriptExecuteError emits a structured warn log for a script.Execute
// fault. The Message string is the legacy "[…] script execute error" key
// that existing tests pin (e.g. modules/world/nai128_rat_loot_test.go);
// extending with the file / pc / backtrace fields is purely additive.
// Mirrors TS ScriptRunner.ts:215-226 console.error block (file + backtrace).
//
// extra is a variadic slog attr list (key, value, key, value …). The
// player call sites (resumeOrFinish / resumeOrFinishWorld) pass "username"
// to mirror TS ScriptRunner.ts @2e3bcf43:
// `printError("Player script error - username:${username}")` (the 244-era
// pid attr was dropped upstream). NPC and world call sites pass nothing
// extra (TS does not add a username for those paths).
func (s *Server) logScriptExecuteError(msg string, state *script.ScriptState, err error, extra ...any) {
	attrs := []any{
		"script", state.Script.Name,
		"file", state.Script.FileName,
		"pc", state.PC,
		"err", err,
		"backtrace", script.Backtrace(state),
	}
	attrs = append(attrs, extra...)
	s.log.Warn(msg, attrs...)
}

// handlePlayerScriptError applies the TS-faithful script-error reaction for
// a player-anchored script: MessageGames the formatted error + file + stack
// backtrace to the player, then in NodeProduction triggers a graceful
// logout and immediately flags loggingOut. Mirrors TS ScriptRunner.ts:
// 188-206 (the `state.self instanceof Player` branch of the catch block).
//
// Goscape deviations from TS:
//   - Goscape uses MessageGame instead of TS's wrappedMessageGame because
//     goscape has no font-driven word-wrap surface (world-ops-1 deviation,
//     already documented). The user-visible difference is that long lines
//     are not split.
//   - Goscape pairs RequestLogout() (the graceful-logout flag consumed by
//     processLogouts) with the direct loggingOut field write (so visibility
//     and iteration filters drop the player on the same tick). TS achieves
//     the same with logout()+loggingOut=true on the JS Player class.
//
// script-core-1 (with script-core-5 LineNumber accessor) closure.
func (s *Server) handlePlayerScriptError(state *script.ScriptState, self script.ActivePlayer, err error) {
	if self == nil {
		return
	}
	self.MessageGame(fmt.Sprintf("script error: %s", err))
	self.MessageGame(fmt.Sprintf("file: %s", state.Script.FileName))
	for _, line := range script.Backtrace(state) {
		self.MessageGame(line)
	}
	if s.cfg.NodeProduction {
		self.RequestLogout()
		if p, ok := self.(*Player); ok {
			p.loggingOut = true
		}
	}
}

// handleNpcScriptError applies the TS-faithful script-error reaction for an
// npc-anchored script: in NodeProduction the npc is despawned immediately
// (duration=0). Mirrors TS ScriptRunner.ts:207-213 (the
// `state.self instanceof Npc` branch). Non-production runs are a structured
// log-only no-op — the logging happens at the caller via
// logScriptExecuteError so the despawn site stays focused on the
// production-only side-effect.
//
// script-core-1 closure (NPC arm).
func (s *Server) handleNpcScriptError(_ *script.ScriptState, n *Npc, _ error) {
	if n == nil {
		return
	}
	if s.cfg.NodeProduction {
		s.removeNpc(n, 0)
	}
}
