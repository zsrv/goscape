package world

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/zsrv/goscape/pkg/friendspb"
	"github.com/zsrv/goscape/pkg/loginpb"
)

func (s *Server) addPlayer(p *Player) error {
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	// Lowest-free slot assignment. TS World.ts:875-883 @2e3bcf43:
	// getNextPlayerSlot() (linear 1..2046); -1 → world full → reject.
	slot := s.players.nextSlot()
	if slot == -1 {
		return errWorldFull
	}
	// playerLoop bucketing by client IP (TS World.ts:902-917): connected
	// clients derive the key from their remote address; headless logins
	// (no client socket) land in the 127.0.0.1 bucket. The bucket fixes
	// this player's per-tick processing position — independent of slot.
	var remoteAddr string
	if p.client != nil && p.client.conn != nil {
		remoteAddr = p.client.conn.RemoteAddr().String()
	}
	s.players.add(slot, playerLoopKey(remoteAddr), p) // sets p.slot (TS World.ts:919-921)
	p.uid = composeUID(p.username37, p.slot)          // NAI-113: TS World.ts:922
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
	return nil
}

// getTotalPlayers returns the count of live players.
// TS World.ts:1691-1702 @2e3bcf43 recounts occupied slots 1..2046 on
// every call (with a "todo: could cache this" note); playerList.count
// caches the identical value.
// Takes the read lock: called from connection goroutines (handleLogin's
// NodeMaxConnected gate) concurrently with tick-goroutine mutations.
func (s *Server) getTotalPlayers() int {
	s.playersMu.RLock()
	defer s.playersMu.RUnlock()
	return int(s.players.count.Load())
}

// isUsernameLoggingOut reports whether a player slot is occupied by an
// entry with this username (already safe-name normalized) whose
// loggingOut flag is set. Mirrors the lookup TS World.logoutRequests.has
// (World.ts:2194) performs against its in-flight-logout set; goscape
// stores the equivalent signal on Player.loggingOut (player.go:310,
// flipped in world_state_ops.go:101 / tick.go:342,350 / reboot.go:56).
// Takes the read lock: called from connection goroutines while the tick
// goroutine's loopUnlink rewrites bucket-slice headers via slices.Delete
// (the pre-A2 fixed array tolerated lock-free pointer reads; the bucket
// slices do NOT).
func (s *Server) isUsernameLoggingOut(safeName string) bool {
	s.playersMu.RLock()
	defer s.playersMu.RUnlock()
	for p := range s.players.all() {
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

// removePlayerInternal performs the slot/zone/players cleanup for p.
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

	if p.slot < 1 || p.slot >= len(s.players.entities) || s.players.get(p.slot) != p {
		return
	}
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
	// TS World.ts:1588-1589 @2e3bcf43: `delete this.players[player.slot]`
	// + `player.unlink()` (drop out of the playerLoop bucket).
	s.players.remove(p)

	// world-ops-2: TS World.removePlayer (World.ts:1642) calls
	// changeNpcCollision(player.width, player.x, player.z, player.level,
	// false) unconditionally after deleting the slot, clearing the
	// FlagBlockNPCs at the player's current tile. The flag is planted by
	// SetVisibility(Default) (player.go:674) and moved on every step and
	// teleport by refreshPlayerZonePresence (zone_refresh.go), so the
	// current tile is where it lives. Width is always 1 per TS
	// PathingEntity init (matching the goscape hardcode in SetVisibility).
	if s.gamemap != nil {
		s.gamemap.ChangeNPCCollision(1, p.x, p.z, p.level, false)
	}

	// 244 delta: TS Player.cleanup (Player.ts:452-454) clears buildArea then
	// resets appearanceInv to -1. heroPoints.clear() (Player.ts:452) is
	// omitted — newPlayer allocates a fresh ledger per login (NAI-120 B2D).
	// buildArea.clear(false) wired here per TS field order; the onReconnect
	// path calls clear(true), which is a TS no-op (BuildArea.ts:24-28).
	p.buildArea.clear(false)
	p.appearanceInv = -1
	// A9 @2e3bcf43: TS Player.cleanup clears resumeButtons — twice, a
	// 2dc4a811 sync quirk (Player.ts:454 `this.resumeButtons = []` AND
	// :456 `this.resumeButtons.length = 0`). One nil-out suffices; mostly
	// hygiene since goscape allocates a fresh *Player per login, but it
	// also guards a late RESUME/IF_BUTTON packet racing the teardown.
	p.resumeButtons = nil
	// 254 delta: TS Player.cleanup (Player.ts:458 @43e02957) ends with
	// this.input.flush() so a tracked player logging out mid-buffer
	// still submits the partial accumulation blob. Nil-guard is
	// goscape-defensive for direct struct-literal Players in tests.
	if p.input != nil {
		p.input.Flush()
	}
}

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
		profile := s.cfg.NodeProfile
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
				Profile:    profile,
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

// saveAllOnShutdown saves and removes every online player. Called from the
// tick goroutine when the tick loop is exiting (Shutdown closed s.quit), so
// p.Save() inside removePlayerOnTick is race-free. Each removePlayerOnTick
// fires a saveWg-tracked PlayerLogout RPC; Shutdown then waits on saveWg
// (bounded by playerSaveFlushTimeout) before cancelling bridgesCtx so these
// saves reach the login server. Without this, players still online when the
// operator stops the server lost all progress since the last autosave.
func (s *Server) saveAllOnShutdown() {
	s.playersMu.RLock()
	var players []*Player
	for p := range s.players.all() {
		players = append(players, p)
	}
	s.playersMu.RUnlock()
	for _, p := range players {
		if p.username != "" {
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
		s.log.Warn("timed out waiting for player saves to flush on shutdown")
	}
}

// autosavePlayers fires a best-effort PlayerAutosave RPC for each
// active player. Must only be called from the tick goroutine
// (reads s.players and captures per-player Save() bytes on-tick
// for goroutine-safety).
//
// Deviation NAI-PLAYERLOADING-D-AUTOSAVE-FIRE-AND-FORGET: per-call
// failures log only (PlayerAutosave is best-effort by design); no
// automatic remediation.
func (s *Server) autosavePlayers() {
	if s.loginClient == nil {
		return
	}
	for p := range s.players.all() {
		if p.username == "" {
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
	for p := range s.players.all() {
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
