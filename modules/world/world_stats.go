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
	numWorldStats    = 12
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
// bandwidth counters have their own TS-cited reset points (World.ts:629,
// :1111) and are NOT touched here.
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
