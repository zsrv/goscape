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

// RegisterHealthRoutes wires GET /healthz and GET /debug/status onto mux.
// snap returns (HealthSnapshot, false) when no world is wired (standalone
// ondemand) — in that case /healthz is a plain process-up 200.
//
// Boot-grace decision (arch-29.6): before the world's tick loop has
// completed a single tick, HealthSnapshot.LastTick is time.Unix(0, 0) (the
// atomic backing it defaults to zero) rather than a real timestamp. A
// naive staleness check would see that as ~56 years old and permanently
// 503 a freshly-started world. LastTick.Unix() <= 0 catches both that
// pre-first-tick epoch value and Go's zero time.Time (used by callers
// that haven't stamped anything at all), and is treated as "still
// starting up" — 200, not stale — since a wedged tick loop is what this
// check exists to catch, not a normal boot sequence. Pinned by
// TestHealthzBootGraceBeforeFirstTick.
func RegisterHealthRoutes(mux *http.ServeMux, snap func() (HealthSnapshot, bool)) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		s, hasWorld := snap()
		if hasWorld && s.LastTick.Unix() > 0 && time.Since(s.LastTick) > healthzStaleAfter {
			http.Error(w, "tick stale", http.StatusServiceUnavailable)
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
