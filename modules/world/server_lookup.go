package world

import (
	"context"
	"log/slog"
	"time"

	"github.com/zsrv/goscape/pkg/script"
	"github.com/zsrv/goscape/pkg/zone"
)

// retryBridgeRegistration runs an idempotent bridge registration call
// (WorldStartup / WorldConnect) until it succeeds once, in a background
// goroutine parented to bridgesCtx. Each attempt gets bridgeCallTimeout.
// arch-29.3: one failed WorldStartup at boot used to strand every
// crashed-out player at ALREADY_LOGGED_IN with no self-healing — the
// gRPC UPDATE clearing account_login.logged_in is idempotent, so retrying
// forever in the background (rather than blocking boot or giving up after
// one Warn-and-swallow attempt) is safe and closes that gap.
func (s *Server) retryBridgeRegistration(name string, call func(context.Context) error) {
	s.bridgeWg.Go(func() {
		for {
			ctx, cancel := context.WithTimeout(s.bridgesCtx, bridgeCallTimeout)
			err := call(ctx)
			cancel()
			if err == nil {
				return
			}
			if s.bridgesCtx.Err() != nil {
				return
			}
			s.log.Warn("bridge registration failed; retrying",
				slog.String("call", name), slog.Any("err", err))
			select {
			case <-time.After(s.bridgeRetryDelay):
			case <-s.bridgesCtx.Done():
				return
			}
		}
	})
}

// initLoginGate seeds the worldStartupDone login gate (arch-29.3 fix
// wave). A standalone world (nil login client) has no WorldStartup
// registration to wait for — there is no login server whose stale
// logged_in rows could be wiped — so its gate starts open. With a login
// client configured, the gate stays closed until worldStartupCall's first
// success. Split out of NewServer so the nil/non-nil branch is unit
// testable without NewServer's listener + cache dependencies.
func (s *Server) initLoginGate(loginClient LoginClient) {
	if loginClient == nil {
		s.worldStartupDone.Store(true)
	}
}

// worldStartupCall returns the retryBridgeRegistration call for the login
// WorldStartup registration. On the first success it opens the login gate
// (worldStartupDone) — arch-29.3 fix wave: the gate must open strictly
// AFTER the blanket logged_in wipe inside the WorldStartup RPC, so every
// admitted login postdates the wipe and the retry can never erase a live
// session's flag. The retry loop exits on that same success, which also
// makes the steady-state variant of the race unreachable.
func (s *Server) worldStartupCall(lc LoginClient) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := lc.WorldStartup(ctx, int32(s.cfg.NodeID), s.cfg.NodeProfile); err != nil {
			return err
		}
		s.worldStartupDone.Store(true)
		return nil
	}
}

// TrackZone marks a zone as modified this tick. Idempotent (map semantics).
// processZones will call ComputeShared on each tracked zone; processCleanup
// will Reset them and clear the set.
//
// Must only be called from the tick goroutine — the zonesTracking map is
// unguarded.
func (s *Server) TrackZone(z *zone.Zone) { s.zonesTracking[z] = struct{}{} }

// LookupPlayerByUID returns the logged-in player whose uid field matches
// the argument, or nil if no such player is active. Intended to be
// called from the tick goroutine (playerLoop is unguarded there).
// Implements the script.PlayerLookup interface consumed by
// FINDUID / P_FINDUID (S7a).
//
// Does NOT filter on CanAccess — callers that need the protected
// variant consult the returned player's CanAccess() separately. Mirrors
// TS World.getPlayerByUid which is a pure lookup.
func (s *Server) LookupPlayerByUID(uid int) script.ActivePlayer {
	// Compare via int32-cast: scripts push uids through ScriptState.PushInt
	// which int32-normalises per TS toInt32 parity (Numbers.ts:7), while
	// composeUID stores the uint32 bit-pattern as a positive Go int. Both
	// representations share the bottom 32 bits; sign-extension reconciles
	// them. Without this, ~50% of usernames (those with bit 31 set in
	// composeUID output) failed every p_finduid call after the int32-cast
	// PushInt fix landed.
	target := int32(uid)
	for _, p := range s.playerLoop {
		if p == nil || !p.active {
			continue
		}
		if int32(p.uid) == target {
			return p
		}
	}
	return nil
}

// LookupPlayerByUsername returns the logged-in player whose username
// field matches the argument exactly, or nil if none is active.
// Mirrors TS World.getPlayerByUsername (World.ts:1675-1689). Intended
// to be called from the tick goroutine (playerLoop is unguarded there).
//
// Match is case-sensitive on the goscape username field (which is set
// at login from the client-supplied display name). TS keys on
// username37 (base37-encoded) but the inputs to this lookup are
// already strings in goscape's call sites.
func (s *Server) LookupPlayerByUsername(name string) *Player {
	for _, p := range s.playerLoop {
		if p == nil || !p.active {
			continue
		}
		if p.username == name {
			return p
		}
	}
	return nil
}

// LookupPlayerBySlot returns the logged-in player at the given slot
// index, or nil if slot is out of range or unoccupied. Mirrors TS
// World.getPlayer(slot). Used by OpPlayer handlers to resolve a
// message's PlayerSlot to a target Player.
func (s *Server) LookupPlayerBySlot(slot int) *Player {
	if slot < 0 || slot >= len(s.players) {
		return nil
	}
	return s.players[slot]
}

// ZonePlayers returns all valid players in the zone at (level, zoneX, zoneZ).
// Mirrors the NpcLookup.ZoneNpcs shape and serverNpcLookup.ZoneNpcs impl
// at modules/world/npc_script_lookup.go:115. Zone resolution via
// pkg/zone.ZoneMap.Get which masks coords to zone bounds internally.
// nil zoneMap (defense) and nil zone (off-grid) both return nil.
// PlayersSafe filters non-IsValid entries (zone.go:424). NAI-35.
func (s *Server) ZonePlayers(level, zoneX, zoneZ int) []script.ActivePlayer {
	if s.zoneMap == nil {
		return nil
	}
	z := s.zoneMap.Get(level, zoneX, zoneZ)
	if z == nil {
		return nil
	}
	out := make([]script.ActivePlayer, 0, z.PlayersCount())
	for p := range z.PlayersSafe(true) {
		// Production EnterPlayer only ever receives *Player, which compile-time
		// satisfies script.ActivePlayer (assertion at message_game.go:11). The
		// ok-form is forward-compatible safety: if a future PlayerLike impl
		// doesn't satisfy ActivePlayer, this skips it instead of panicking.
		pp, ok := p.(script.ActivePlayer)
		if !ok {
			continue
		}
		out = append(out, pp)
	}
	return out
}
