package world

import "time"

// HealthSnapshot is a point-in-time liveness/observability snapshot
// (arch-29.6), read by the ondemand /healthz and /debug/status handlers
// via an app-level adapter (cmd/goscape/app/modules.go). LastTick is
// time.Unix(0, 0) (the Unix epoch — NOT Go's zero time.Time, which is
// year 1) until the first tick completes, because it is built from
// lastTickNano's atomic zero value via time.Unix(0, n). Callers must
// treat a non-positive LastTick.Unix() as "not yet ticking" rather than
// "stale" — see modules/ondemand/health.go's boot-grace handling.
type HealthSnapshot struct {
	LastTick        time.Time
	CurrentTick     int64
	PlayersOnline   int
	LastCycleMillis int
}

// stampTick copies tick-goroutine-owned state into the cross-goroutine
// atomics HealthSnapshot reads (arch-29.6). Call exactly once per tick,
// from the tick goroutine, right after s.currentTick++ (tick.go).
// cycleMillis is the just-completed tick's wall-clock work duration
// (time.Since(start) in the tick loop) — rev-225 has no lastCycleStats
// array to read it from, so the loop passes it in directly. This is the
// ONLY stamp site — no locks are added to the tick path.
func (s *Server) stampTick(cycleMillis int64) {
	s.lastTickNano.Store(time.Now().UnixNano())
	s.currentTickAtomic.Store(int64(s.currentTick))
	s.lastCycleMillis.Store(cycleMillis)
}

// HealthSnapshot returns a HealthSnapshot built entirely from atomics —
// LastTick/CurrentTick/LastCycleMillis from stampTick, and PlayersOnline
// from playerCount, which is maintained at the two guarded write sites
// (addPlayer/removePlayerInternal; the latter's slot-identity check makes
// its decrement idempotent under double-removal). Safe to call from any
// goroutine.
func (s *Server) HealthSnapshot() HealthSnapshot {
	return HealthSnapshot{
		LastTick:        time.Unix(0, s.lastTickNano.Load()),
		CurrentTick:     s.currentTickAtomic.Load(),
		PlayersOnline:   int(s.playerCount.Load()),
		LastCycleMillis: int(s.lastCycleMillis.Load()),
	}
}
