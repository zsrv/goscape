package world

import (
	"log/slog"
	"time"

	"github.com/zsrv/goscape/pkg/script"
)

// enqueueRelayAction posts a closure onto the relay action queue.
// Non-blocking: drops the action if the queue is full (matches
// NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL server-side posture). Called by
// WorldStateOps methods on the per-world subscriber goroutine.
//
// A dropped action represents one lost RELAY_* event. Logged at Warn
// so operators see queue pressure. In practice the queue is sized
// generously (64) and tick cadence is sub-second, so drops should be
// rare.
func (s *Server) enqueueRelayAction(action func()) {
	select {
	case s.relayActionQueue <- action:
	default:
		s.log.Warn("relay action queue full; dropping action")
	}
}

// drainRelayActions runs every pending action on the queue. Must be
// invoked from the tick goroutine. Non-blocking — exits as soon as the
// queue is empty. Actions are executed in FIFO order in the same
// iteration; they observe and mutate tick-owned state directly.
//
// Placement: top of tick loop body, between the rebuildResult drain and
// processShutdown so that a RELAY_SHUTDOWN that arrived this iteration
// can take effect on this same tick.
func (s *Server) drainRelayActions() {
	for {
		select {
		case action := <-s.relayActionQueue:
			action()
		default:
			return
		}
	}
}

// lookupPlayerByUsername37 returns the active player whose username37
// matches u37, or nil if none. Compares the pre-computed Player.username37
// field directly (set at login) rather than recomputing the base37 hash
// per-iteration — matches the precedent at LookupPlayerByUsername
// (server.go:1116). Tick-only: iterates s.players without acquiring
// playersMu, mirroring the existing LookupPlayerByUsername(string)
// helper. WorldStateOps closures call this on the tick goroutine where
// players is unguarded.
//
// Lookup-miss is a normal occurrence (the friends-server fans a relay
// to every world; the target may live on a different one). Callers log
// a miss at Debug — not Warn — to avoid log spam.
func (s *Server) lookupPlayerByUsername37(u37 uint64) *Player {
	for p := range s.players.all() {
		if !p.active {
			continue
		}
		if p.username37 == u37 {
			return p
		}
	}
	return nil
}

// SetPlayerMute persists a mute deadline on the looked-up player.
// Mirrors TS Player.muted_until = new Date(muted_until) at
// World.ts:2006. Lookup-miss is silently dropped at Debug.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION (MUTE bullet) — retired here.
func (s *Server) SetPlayerMute(username37 uint64, mutedUntilMs int64) {
	s.enqueueRelayAction(func() {
		p := s.lookupPlayerByUsername37(username37)
		if p == nil {
			s.log.Debug("RELAY_MUTE: player not online; skipping",
				slog.Uint64("username37", username37))
			return
		}
		p.mutedUntil = time.UnixMilli(mutedUntilMs)
	})
}

// KickPlayer flags the looked-up player for logout. Mirrors TS
// Player.loggingOut = true at World.ts:2013-2018 (goscape defers the
// teardown to processLogouts per NAI-186-D1, identical to the ::kick
// cheat at handlers_game.go:1231). Lookup-miss is silently dropped.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION (KICK bullet) — retired here.
func (s *Server) KickPlayer(username37 uint64) {
	s.enqueueRelayAction(func() {
		p := s.lookupPlayerByUsername37(username37)
		if p == nil {
			s.log.Debug("RELAY_KICK: player not online; skipping",
				slog.Uint64("username37", username37))
			return
		}
		p.loggingOut = true
	})
}

// SetPlayerInputTracking flips the per-player input-tracking gate.
// Mirrors TS `player.input.active = state` at World.ts:2075 @43e02957
// (RELAY_TRACK). Goscape stores Active as bool; convert via
// state != 0. Lookup-miss is silently dropped.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION (TRACK bullet) — retired here.
func (s *Server) SetPlayerInputTracking(username37 uint64, state int32) {
	s.enqueueRelayAction(func() {
		p := s.lookupPlayerByUsername37(username37)
		if p == nil {
			s.log.Debug("RELAY_TRACK: player not online; skipping",
				slog.Uint64("username37", username37))
			return
		}
		p.input.Active = state != 0
	})
}

// RelayShutdown schedules a world reboot in `durationTicks` ticks.
// Mirrors TS World.rebootTimer (World.ts:1787-1793) via the existing
// rebootTimer helper, which writes shutdownTick + broadcasts
// UPDATE_REBOOT_TIMER packets to all online players.
//
// Named `RelayShutdown` rather than the plan's `Shutdown` because
// *Server already has a `Shutdown()` method (full TCP teardown) — the
// `Relay` prefix matches the originating RPC opcode name.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION (SHUTDOWN bullet) — retired here.
func (s *Server) RelayShutdown(durationTicks int32) {
	s.enqueueRelayAction(func() {
		s.rebootTimer(int(durationTicks))
	})
}

// RelayReload performs a config-only reload, mirroring TS
// World.ts:2036 (`this.reload(false)`). It re-loads the already-packed
// config types from disk WITHOUT repacking content and WITHOUT clearing
// inventories (clearInvs=false). The friends server fans RELAY_RELOAD to
// every world after content is repacked elsewhere; each world must NOT
// repack on its own (the ::rebuild / fsnotify pipeline owns packing) and
// must preserve live inventories.
//
// Runs on the tick goroutine via drainRelayActions, where reloadFn
// (default *Server.Reload) executes synchronously against tick-owned
// state. A reload error is logged at Error and otherwise swallowed —
// there is no invoker to notify (unlike the ::rebuild path) and the old
// config remains partially/fully in place (reload.go D2-HALF-SWAP).
//
// Named `RelayReload` rather than the plan's `Reload` because *Server
// already has a `Reload(clearInvs bool) error` method — the `Relay`
// prefix matches the originating RPC opcode name.
//
// gap-world-reload-events-1: the prior wiring called
// dispatchRebuildRequest(), which both repacked content (wrong: TS
// reload does no packing) and reloaded with clearInvs=true (wrong:
// clobbered inventories).
//
// NAI-S5A-D-DISPATCHER-NO-ACTION (RELOAD bullet) — retired here.
func (s *Server) RelayReload() {
	s.enqueueRelayAction(func() {
		if err := s.reloadFn(false); err != nil {
			s.log.Error("RELAY_RELOAD: config reload failed", "err", err)
		}
	})
}

// ClearLogins drains the pending-logins queue (s.newPlayers). Mirrors
// TS World.loginRequests.clear() at World.ts:2038.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION (CLEARLOGINS bullet) — retired here.
func (s *Server) ClearLogins() {
	s.enqueueRelayAction(func() {
		s.playersMu.Lock()
		s.newPlayers = nil
		s.playersMu.Unlock()
	})
}

// ClearLogouts is a tagged no-op. Goscape has no logout-request queue
// analogous to TS's World.logoutRequests.clear() at World.ts:2040 —
// logouts are signaled via the loggingOut flag and drained by
// processLogouts. Clearing the flag from a non-tick goroutine would
// be unsafe.
//
// NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE — permanent (architectural
// divergence from TS).
func (s *Server) ClearLogouts() {
	s.enqueueRelayAction(func() {
		s.log.Info("RELAY_CLEARLOGOUTS received (no-op: goscape has no logout-request queue)")
	})
}

// BroadcastMessage fans a chat message out to every connected player.
// Delegates to the existing BroadcastMes helper.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION (BROADCAST bullet) — retired here.
func (s *Server) BroadcastMessage(message string) {
	s.enqueueRelayAction(func() {
		s.BroadcastMes(message)
	})
}

// QueueScript dispatches a [queue,<name>] script to the looked-up
// player's primary queue. Mirrors TS World.ts:2041-2051:
//
//	} else if (opcode === FriendsServerOpcodes.RELAY_QUEUESCRIPT) {
//	    const { scriptName, username } = data;
//	    const player = this.getPlayerByUsername(username);
//	    if (player) {
//	        const script = ScriptProvider.getByName(`[queue,${scriptName}]`);
//	        if (script) { player.enqueueScript(script); }
//	    }
//	}
//
// Lookup-miss (player offline OR script-name not registered) is
// silently dropped at Debug. EnqueueScriptFile is void — no error
// recovery needed (TS enqueueScript is also void).
func (s *Server) QueueScript(scriptName string, username37 uint64) {
	s.enqueueRelayAction(func() {
		p := s.lookupPlayerByUsername37(username37)
		if p == nil {
			s.log.Debug("RELAY_QUEUESCRIPT: player not online; skipping",
				slog.String("script_name", scriptName),
				slog.Uint64("username37", username37))
			return
		}
		if s.scriptProvider == nil {
			// Reached when production script directory load failed
			// (server.go warn-logs and proceeds with nil provider) or
			// when a test fixture omits the provider. Do not remove.
			return
		}
		sf := s.scriptProvider.GetByName("[queue," + scriptName + "]")
		if sf == nil {
			s.log.Debug("RELAY_QUEUESCRIPT: script not found; skipping",
				slog.String("script_name", scriptName))
			return
		}
		// TS Player.enqueueScript defaults: type=NORMAL, delay=0, args=[].
		// EnqueueScriptFile takes *ScriptFile directly (player_script.go:71)
		// — no ID lookup needed. It nil-guards and silently no-ops on
		// nil sf, but we already checked above. Returns nothing.
		p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueNormal)
	})
}
