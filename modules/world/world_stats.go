package world

import "time"

// WorldStat indexes the per-cycle stats arrays. Order mirrors TS
// WorldStat.ts:1-14 exactly (identical at both pins — 225 and 244).
const (
	statCycle        = iota // 0
	statWorld               // 1
	statClientIn            // 2
	statNpc                 // 3
	statPlayer              // 4
	statLogout              // 5
	statLogin               // 6
	statZone                // 7
	statClientOut           // 8
	statCleanup             // 9
	statBandwidthIn         // 10
	statBandwidthOut        // 11
	numWorldStats           // 12 — iota continuation; auto-tracks the list above
)

// addCycleTime accumulates elapsed wall-clock ms into cycleStats[stat].
// TS assigns once per section (cycleStats[X] = Date.now() - start);
// goscape's tick pipeline splits several TS sections into multiple passes
// (documented deviations NAI-93/NAI-122/NAI-217 et al.), so the Go shape
// zeroes the timing stats at tick start (resetCycleTimes) and ACCUMULATES
// per pass — the per-section total is the same sum TS measures. uint16
// arithmetic wraps mod 65536, matching TS Uint16Array truncation.
func (s *Server) addCycleTime(stat int, start time.Time) {
	s.cycleStats[stat] += uint16(time.Since(start).Milliseconds())
}

// resetCycleTimes zeroes the ten timing entries at tick start. The two
// bandwidth counters are NOT touched here: BANDWIDTH_IN resets at its
// TS-cited point (World.ts:629, head of client-in); BANDWIDTH_OUT resets
// at tick start — see PORTING-EXCEPTION (rev244-b4-bwout-reset) in
// tick.go (TS resets at World.ts:1111, but goscape writes throughout
// the tick).
func (s *Server) resetCycleTimes() {
	for i := statCycle; i <= statCleanup; i++ {
		s.cycleStats[i] = 0
	}
}

// snapshotCycleStats copies cycleStats into lastCycleStats at cycle end.
// Mirrors TS World.ts:489-500. Tick-goroutine-only.
func (s *Server) snapshotCycleStats() {
	s.lastCycleStats = s.cycleStats
}

// LastCycleStat returns lastCycleStats[stat], the surface the MAP_LAST*
// debug script ops read (DebugOps.ts:20-68). Tick-goroutine-only (script
// execution runs on-tick).
func (s *Server) LastCycleStat(stat int) int {
	if stat < 0 || stat >= numWorldStats {
		return 0
	}
	return int(s.lastCycleStats[stat])
}

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
// from the tick goroutine, right after s.currentTick++ (tick.go) so
// lastCycleStats already reflects the tick that just completed
// (snapshotCycleStats runs earlier in the same tick body). This is the
// ONLY stamp site — no locks are added to the tick path.
func (s *Server) stampTick() {
	s.lastTickNano.Store(time.Now().UnixNano())
	s.currentTickAtomic.Store(int64(s.currentTick))
	s.lastCycleMillis.Store(int64(s.lastCycleStats[statCycle]))
}

// HealthSnapshot returns a HealthSnapshot built entirely from atomics —
// LastTick/CurrentTick/LastCycleMillis from stampTick, and PlayersOnline
// from playerList.count, which is already atomic and only mutated at the
// two guarded sites (playerList.add/remove; the latter's slot-identity
// check makes removePlayerInternal's decrement idempotent under
// double-removal). Safe to call from any goroutine.
func (s *Server) HealthSnapshot() HealthSnapshot {
	return HealthSnapshot{
		LastTick:        time.Unix(0, s.lastTickNano.Load()),
		CurrentTick:     s.currentTickAtomic.Load(),
		PlayersOnline:   int(s.players.count.Load()),
		LastCycleMillis: int(s.lastCycleMillis.Load()),
	}
}
