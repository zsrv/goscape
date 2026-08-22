package world

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/zsrv/goscape/pkg/friendspb"
	"github.com/zsrv/goscape/pkg/loginpb"
)

var errWorldFull = errors.New("world full")

func (s *Server) addPlayer(p *Player) error {
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	for i := 1; i < len(s.players); i++ {
		if s.players[i] == nil {
			p.slot = i
			p.uid = composeUID(p.username37, p.slot) // NAI-113: TS World.ts:937
			s.players[i] = p
			s.playerLoop = append(s.playerLoop, p)
			p.active = true
			// Seed the default-south orientation now that p.x/p.z are set, so
			// the always-forced FACE_COORD low-def orients a freshly-logged-in
			// player south rather than the client's north-east default.
			p.unfocus()
			if s.zoneMap != nil {
				z := s.zoneMap.Get(p.level, p.x, p.z)
				p.zoneListElement = z.EnterPlayer(p, s.zoneMap.Grid(p.level))
			}
			if s.rsbuf != nil {
				s.rsbuf.AddPlayer(int32(p.slot))
			}
			// arch-29.6: track the live count for HealthSnapshot.PlayersOnline.
			s.playerCount.Add(1)
			return nil
		}
	}
	return errWorldFull
}

// getTotalPlayers returns the count of live players. It reads the
// playerCount atomic (maintained at the two guarded add/remove sites, so it
// equals the live slot occupancy) instead of iterating the slot table
// lock-free: the old iteration raced the tick goroutine's slot writes
// whenever called off-tick — e.g. the connection-admission check at ~L1286
// (getTotalPlayers() > NodeMaxConnected) runs on a connection goroutine.
// Also O(1) and a closer match to TS World.getTotalPlayers
// (World.ts:1730-1732: return this.players.count).
func (s *Server) getTotalPlayers() int {
	return int(s.playerCount.Load())
}

// isUsernameLoggingOut reports whether a player slot is occupied by an
// entry with this username (already safe-name normalized) whose
// loggingOut flag is set. Mirrors the lookup TS World.logoutRequests.has
// (World.ts:2194) performs against its in-flight-logout set; goscape
// stores the equivalent signal on Player.loggingOut (player.go:310,
// flipped in world_state_ops.go:101 / tick.go:342,350 / reboot.go:56).
// Lock-free read — same convention as getTotalPlayers above.
func (s *Server) isUsernameLoggingOut(safeName string) bool {
	for _, p := range s.players {
		if p == nil {
			continue
		}
		if p.username == safeName && p.loggingOut {
			return true
		}
	}
	return false
}

// scaleByPlayerCount scales a tick rate (typically a respawn duration)
// by the current live-player count. Mirrors TS
// World.scaleByPlayerCount at World.ts:1715-1719.
//
// Formula: playerCount = min(getTotalPlayers(), 2000)
//
//	return ((4000 - playerCount) * rate) / 4000  // int truncation
//
// Empty world returns rate unchanged; 2000+ players halves it.
func (s *Server) scaleByPlayerCount(rate int) int {
	playerCount := min(s.getTotalPlayers(), 2000)
	return ((4000 - playerCount) * rate) / 4000
}

// removePlayerInternal performs the slot/zone/playerLoop cleanup for p.
// Must only be called from the tick goroutine.
//
// Callers should use removePlayerOnTick or removePlayerOnDisconnect,
// which add the appropriate gRPC-side cleanup before invoking this.
//
// TS Player.cleanup at Engine-TS/src/engine/entity/Player.ts:446 calls
// player.heroPoints.clear() as part of cleanup. goscape omits the
// call: newPlayer (player.go:506) allocates a fresh *Player per login
// with a fresh NewHeroPoints(16) (player.go:544), so clearing the
// about-to-be-GC'd ledger has no observable effect. Informal English
// deferral (no NAI-XXX-D pin); precedent set by combat sub-spec
// framing cleanup (2026-05-20). NAI-120 Bundle 2D follow-up.
func (s *Server) removePlayerInternal(p *Player) {
	p.active = false
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	if p.slot < 1 || p.slot >= len(s.players) || s.players[p.slot] != p {
		return
	}
	// arch-29.6: past the slot-identity guard this is a genuine removal, so
	// the decrement is idempotent under double-removal (a repeat call returns
	// at the guard above). Keeps HealthSnapshot.PlayersOnline accurate.
	s.playerCount.Add(-1)
	if s.zoneMap != nil && p.zoneListElement != nil {
		z := s.zoneMap.Get(p.level, p.x, p.z)
		z.LeavePlayer(p, p.zoneListElement, s.zoneMap.Grid(p.level))
		p.zoneListElement = nil
	}
	if s.rsbuf != nil {
		// RemovePlayer follows upstream lib.rs:186-203 ordering: iterate
		// player.Build.Npcs to decrement observer counts, then run
		// Build.Cleanup. Order matters; running cleanup first would
		// clear Npcs before the iteration and silently skip the
		// observer decrement.
		s.rsbuf.RemovePlayer(int32(p.slot))
	}
	s.players[p.slot] = nil

	// world-ops-2: TS World.removePlayer (World.ts:1601) calls
	// changeNpcCollision(player.width, player.x, player.z, player.level,
	// false) unconditionally after deleting the slot, clearing the
	// FlagBlockNPCs at the player's current tile. The flag is planted by
	// SetVisibility(Default) (player.go SetVisibility) and moved on every step and
	// teleport by refreshPlayerZonePresence (zone_refresh.go), so the
	// current tile is where it lives. Width is always 1 per TS
	// PathingEntity init (matching the goscape hardcode in SetVisibility).
	if s.gamemap != nil {
		s.gamemap.ChangeNPCCollision(1, p.x, p.z, p.level, false)
	}

	for i, lp := range s.playerLoop {
		if lp == p {
			s.playerLoop = append(s.playerLoop[:i], s.playerLoop[i+1:]...)
			break
		}
	}
}

// logoutSaveAttempts bounds sendPlayerLogoutWithRetry's retry loop
// (arch-28.5): TS's login "server" is an in-process worker whose message
// queue survives with the process, so a momentary outage never lost a
// save; the gRPC split introduced a loss window (last-autosave rollback,
// up to ~15 min) that a couple of retries close for restart-blip outages.
// Retries abort early once bridgesCtx is cancelled (shutdown's
// waitForSaveFlush stays bounded).
const logoutSaveAttempts = 3

// sendPlayerLogoutWithRetry fires the PlayerLogout RPC, retrying up to
// logoutSaveAttempts total attempts on failure (arch-28.5), with
// s.logoutSaveRetryDelay between attempts. Blocking — callers run it on
// their own goroutine (removePlayerOnTick tracks it via saveWg). Aborts
// early if s.bridgesCtx is cancelled, so shutdown's bounded
// waitForSaveFlush isn't held hostage by a dead login service.
func (s *Server) sendPlayerLogoutWithRetry(username string, save []byte) {
	req := &loginpb.PlayerLogoutRequest{
		NodeId:   int32(s.cfg.NodeID),
		Profile:  s.cfg.NodeProfile,
		Username: username,
		Save:     save,
	}
	for attempt := 1; ; attempt++ {
		ctx, cancel := context.WithTimeout(s.bridgesCtx, bridgeCallTimeout)
		_, err := s.loginClient.PlayerLogout(ctx, req)
		cancel()
		if err == nil {
			return
		}
		if attempt >= logoutSaveAttempts || s.bridgesCtx.Err() != nil {
			s.log.Error("PlayerLogout RPC failed; save lost until next login",
				slog.String("username", username), slog.Int("attempt", attempt), slog.Any("err", err))
			return
		}
		s.log.Warn("PlayerLogout RPC failed; retrying",
			slog.String("username", username), slog.Int("attempt", attempt), slog.Any("err", err))
		select {
		case <-time.After(s.logoutSaveRetryDelay):
		case <-s.bridgesCtx.Done():
		}
	}
}

// removePlayerOnTick handles graceful logout from the tick goroutine.
// Captures p.Save() while still on-tick (thread-safe) and fires a
// best-effort PlayerLogout RPC (bounded retry, arch-28.5) in a goroutine,
// then performs internal cleanup.
//
// Deviation NAI-PLAYERLOADING-D-LOGOUT-NO-FORCE-FALLBACK: on RPC
// failure, log only — no PlayerForceLogout belt-and-braces (TS parity).
//
// PlayerLogout RPC contents pinned by TestRemovePlayerOnTick_*
// (server_logout_test.go).
func (s *Server) removePlayerOnTick(p *Player) {
	if s.loginClient != nil && p.username != "" {
		save := p.Save(s.invTypes, s.varpTypes)
		username := p.username
		// saveWg: Shutdown waits for this to flush before bridgesCancel so a
		// logout-then-stop doesn't lose the save (the RPC is parented to
		// bridgesCtx, which Shutdown otherwise cancels immediately).
		s.saveWg.Add(1)
		go func() {
			defer s.saveWg.Done()
			s.sendPlayerLogoutWithRetry(username, save)
		}()
	}
	if s.friendsClient != nil && p.username != "" {
		username37 := p.username37
		worldID := int32(s.cfg.NodeID)
		// arch-29.13: enqueue on the single global friends dispatcher
		// instead of firing an ad hoc goroutine, so this PlayerLogout
		// executes strictly after any PlayerLogin/other friends mutation
		// for this player enqueued earlier this tick or a prior tick —
		// restoring the TS friendThread.postMessage FIFO guarantee. The
		// per-call timeout now lives on the dispatcher's worker
		// (context.WithTimeout(bridgesCtx, callTimeout) per dequeue).
		s.friendsMutationDispatcher.enqueue(func(ctx context.Context) {
			s.friendsClient.PlayerLogout(ctx, &friendspb.PlayerLogoutRequest{
				WorldId:    worldID,
				Username37: username37,
			})
		})
	}
	if p.friendsSubCancel != nil {
		p.friendsSubCancel()
		p.friendsSubCancel = nil
	}
	// logger-transport-4: TS World.removePlayer (World.ts:1606) emits a
	// MODERATOR session log immediately before flushPlayer/cleanup. Mirror
	// the order here — fire BEFORE removePlayerInternal so p.session, p.x,
	// p.z are still set when AddSessionLog snapshots them. The graceful and
	// disconnect paths both funnel through this function (the disconnect
	// path enqueues this on the removal queue), so the log emits once per
	// logout regardless of how the player left.
	p.AddSessionLog(LoggerEventTypeModerator, "Logged out")
	s.removePlayerInternal(p)
	// Last tick-side touch of this connection's buffers: drop the tick's
	// ref (idempotent — the idle-logout and disconnect paths can both
	// land here for the same player).
	if p.client != nil {
		p.client.dropTickRef()
	}
}

// removePlayerOnDisconnect handles an ungraceful socket close from the
// per-conn goroutine. It cannot call p.Save() here (that reads player state
// the tick goroutine concurrently mutates — a data race), so it defers the
// whole removal to the tick by enqueuing removePlayerOnTick on the removal
// queue; guaranteed, non-lossy (drained at the top of the tick loop, before
// the lossy relay queue — arch-28.4a: a dropped removal here ghosts the
// player in-world for the 100-tick no-response timeout while the tick keeps
// writing into a dead connection's buffers). removePlayerOnTick runs
// on-tick, so p.Save() is safe and the player IS saved — matching TS,
// which keeps a dropped player in-world and saves them via the idle-logout
// (the earlier "PlayerForceLogout, no save" path lost all progress since the
// last 15-minute autosave, and its "TS has the same window" note was wrong).
//
// removePlayerInternal is idempotent (slot-identity guard), so this is safe
// even if the tick's own no-connection idle-logout fires for the same player.
func (s *Server) removePlayerOnDisconnect(p *Player) {
	s.enqueueRemoval(func() {
		s.removePlayerOnTick(p)
	})
}

// playerSaveFlushTimeout bounds how long Shutdown waits for in-flight save
// RPCs to flush before cancelling bridgesCtx — long enough for one bridge
// call (bridgeCallTimeout) plus margin, but bounded so a hung login server
// cannot wedge shutdown indefinitely.
const playerSaveFlushTimeout = bridgeCallTimeout + 2*time.Second

// saveAllOnShutdown saves and removes every online player. Called from the
// tick goroutine when the tick loop is exiting (Shutdown closed s.quit), so
// p.Save() inside removePlayerOnTick is race-free. Each removePlayerOnTick
// fires a saveWg-tracked PlayerLogout RPC; Shutdown then waits on saveWg
// (bounded by playerSaveFlushTimeout) before cancelling bridgesCtx so these
// saves reach the login server. Without this, players still online when the
// operator stops the server lost all progress since the last autosave.
func (s *Server) saveAllOnShutdown() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()
	for _, p := range players {
		if p != nil && p.username != "" {
			s.removePlayerOnTick(p)
		}
	}
}

// waitForSaveFlush blocks until all in-flight save RPCs complete or
// playerSaveFlushTimeout elapses, whichever comes first.
func (s *Server) waitForSaveFlush() {
	done := make(chan struct{})
	go func() {
		s.saveWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(playerSaveFlushTimeout):
		s.log.Warn("timed out waiting for player saves to flush")
	}
}

// PlayerSaveRate is the autosave cadence in ticks. 1500 ticks at ~600ms
// ≈ 15 minutes. Mirrors TS World.PLAYER_SAVERATE.
const PlayerSaveRate = 1500

// autosavePlayers fires a best-effort PlayerAutosave RPC for each
// active player. Must only be called from the tick goroutine
// (reads s.playerLoop and captures per-player Save() bytes on-tick
// for goroutine-safety).
//
// Deviation NAI-PLAYERLOADING-D-AUTOSAVE-FIRE-AND-FORGET: per-call
// failures log only (PlayerAutosave is best-effort by design); no
// automatic remediation.
func (s *Server) autosavePlayers() {
	if s.loginClient == nil {
		return
	}
	for _, p := range s.playerLoop {
		if p == nil || p.username == "" {
			continue
		}
		save := p.Save(s.invTypes, s.varpTypes)
		req := &loginpb.PlayerAutosaveRequest{
			Profile:  s.cfg.NodeProfile,
			Username: p.username,
			Save:     save,
		}
		// Arc 18 R3 — per-call timeout + shutdown-derived parent.
		s.saveWg.Add(1)
		go func() {
			defer s.saveWg.Done()
			ctx, cancel := context.WithTimeout(s.bridgesCtx, bridgeCallTimeout)
			defer cancel()
			s.loginClient.PlayerAutosave(ctx, req)
		}()
	}
}

// crashSavePlayers is crashSaveAll's per-player pass: fires a best-effort
// PlayerAutosave for every online player, but — unlike autosavePlayers —
// isolates each player behind its own recover. The tick loop is already
// panicking when this runs, so a SECOND panic (e.g. corrupt in-memory
// state making p.Save itself panic) must not abort the pass and strand
// every player behind the offender unsaved; that player is logged and
// skipped, and the rest still get saved.
//
// saveFn defaults to (*Player).Save (bound to s.invTypes/s.varpTypes);
// tests override it to inject a panicking save for one player while
// proving the others still land on PlayerAutosave.
func (s *Server) crashSavePlayers(saveFn func(*Player) []byte) {
	if s.loginClient == nil {
		return
	}
	// rev-225 iterates s.playerLoop (the tick-goroutine-owned live slice)
	// exactly as autosavePlayers above does; later revisions range a
	// PlayerList iterator instead.
	for _, p := range s.playerLoop {
		if p == nil {
			continue
		}
		func(p *Player) {
			defer func() {
				if r := recover(); r != nil {
					s.log.Error("crash-save: skipped player after panic in save path",
						"player", p.username,
						"err", r,
						"stack", string(debug.Stack()))
				}
			}()
			if p.username == "" {
				return
			}
			save := saveFn(p)
			req := &loginpb.PlayerAutosaveRequest{
				Profile:  s.cfg.NodeProfile,
				Username: p.username,
				Save:     save,
			}
			s.saveWg.Add(1)
			go func() {
				defer s.saveWg.Done()
				ctx, cancel := context.WithTimeout(s.bridgesCtx, bridgeCallTimeout)
				defer cancel()
				s.loginClient.PlayerAutosave(ctx, req)
			}()
		}(p)
	}
}
