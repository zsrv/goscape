package ondemand

import (
	"encoding/json"
	"net/http"
	"time"
)

// healthzStaleAfter: a tick loop silent for this long is wedged — the
// world accepts TCP but strands players, which is exactly what the old
// tcpSocket readiness probe could not see (arch-29.6).
const healthzStaleAfter = 10 * time.Second

// healthzBootGrace: how long after the health routes are registered a world
// that has not yet completed its first tick is still treated as "starting"
// rather than "wedged". A world normally ticks within one ~600ms cycle of
// starting, so this is generous (cache load, an async WorldStartup retry).
// Past it, a world that never produced a first tick is wedged during startup —
// which the pre-first-tick grace alone would otherwise hide forever, since
// LastTick stays at its zero value and time.Since is never consulted. Closing
// that blind spot is the arch-29.6 follow-up this bound exists for.
const healthzBootGrace = 30 * time.Second

// HealthSnapshot is the subset of world state the health endpoints need.
// modules/world's Server provides a compatible method
// (world.Server.HealthSnapshot), and the app wires it through an adapter
// func (cmd/goscape/app/modules.go initOnDemand) that converts field-by-
// field, so modules/ondemand never imports modules/world.
type HealthSnapshot struct {
	LastTick        time.Time
	CurrentTick     int64
	PlayersOnline   int
	LastCycleMillis int
}

// healthzStatus decides the GET /healthz response. It is pure so both the
// boot-deadline and staleness branches are testable without a real clock:
// sinceBoot is how long the health routes have been registered.
//
// Boot-grace decision (arch-29.6): before the world's tick loop has completed
// a single tick, HealthSnapshot.LastTick is time.Unix(0, 0) (the atomic
// backing it defaults to zero) rather than a real timestamp, and Go's zero
// time.Time from callers that never stamped anything reads the same way. Both
// satisfy LastTick.Unix() <= 0.
//
//   - no world wired (standalone ondemand): always 200 (process-up).
//   - world wired, before the first tick: 200 while sinceBoot is within
//     healthzBootGrace ("starting"), then 503 ("no first tick") — a tick loop
//     that never produced a first tick within the grace window is wedged
//     during startup, not merely booting.
//   - world wired, after the first tick: 503 ("tick stale") if the last tick
//     is older than healthzStaleAfter (a stalled tick loop), else 200.
//
// Pinned by TestHealthzStatus_* and the HTTP-level TestHealthz* tests.
func healthzStatus(s HealthSnapshot, hasWorld bool, sinceBoot time.Duration) (int, string) {
	if !hasWorld {
		return http.StatusOK, "ok"
	}
	if s.LastTick.Unix() <= 0 {
		if sinceBoot > healthzBootGrace {
			return http.StatusServiceUnavailable, "no first tick"
		}
		return http.StatusOK, "starting"
	}
	if time.Since(s.LastTick) > healthzStaleAfter {
		return http.StatusServiceUnavailable, "tick stale"
	}
	return http.StatusOK, "ok"
}

// RegisterHealthRoutes wires GET /healthz and GET /debug/status onto mux.
// snap returns (HealthSnapshot, false) when no world is wired (standalone
// ondemand) — in that case /healthz is a plain process-up 200. bootTime is
// captured here (health routes are registered once, during ondemand init,
// at or after the world starts) and fed to healthzStatus as the boot-deadline
// reference; capturing at-or-after world start only ever makes the deadline
// more lenient, never a false positive.
func RegisterHealthRoutes(mux *http.ServeMux, snap func() (HealthSnapshot, bool)) {
	bootTime := time.Now()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		s, hasWorld := snap()
		if code, reason := healthzStatus(s, hasWorld, time.Since(bootTime)); code != http.StatusOK {
			http.Error(w, reason, code)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /debug/status", func(w http.ResponseWriter, r *http.Request) {
		s, hasWorld := snap()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"world_wired":      hasWorld,
			"ticking":          s.LastTick.Unix() > 0,
			"last_tick_age_ms": time.Since(s.LastTick).Milliseconds(),
			"current_tick":     s.CurrentTick,
			"players_online":   s.PlayersOnline,
			"tick_ms":          s.LastCycleMillis,
		})
	})
}
