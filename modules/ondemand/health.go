package ondemand

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

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
//
// ready is the world's readiness verdict, supplied by the app adapter in
// cmd/goscape/app/modules.go: it reports (error, hasWorld). hasWorld is false
// when no world is wired (standalone ondemand) — /healthz is then a plain
// process-up 200. A nil error is 200; any other error is 503 with the error's
// message as the body.
//
// The decision itself — the 30s boot grace and the 10s staleness bound — used
// to live here as healthzStatus. It now belongs to its owner,
// modules/world.Server.CheckReady, which the same adapter calls; the adapter
// translates world.ErrStarting (inside the boot grace) to a nil verdict, which
// is why a world that has not completed its first tick still answers 200 here.
// That backward compatibility is deliberate and local to this legacy route:
// the process-wide /readyz on the admin listener treats "starting" as not
// ready, because a pod must not receive players before its first tick.
//
// /healthz is wired only as a readiness probe (production/helm), never a
// liveness probe, so a 503 removes the pod from Service endpoints but never
// restarts it, and it self-heals the moment the first tick lands.
//
// snap feeds GET /debug/status only. debugStatus gates that route
// (SEC1 M-12 — default off, see ondemand.debug_status_enabled).
func RegisterHealthRoutes(mux *http.ServeMux, ready func(context.Context) (error, bool), snap func() (HealthSnapshot, bool), debugStatus bool) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		err, hasWorld := ready(r.Context())
		if hasWorld && err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	if debugStatus {
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
}
