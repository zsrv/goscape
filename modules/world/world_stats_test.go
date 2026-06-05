package world

import (
	"testing"
	"time"
)

// TestAddCycleTime_Accumulates verifies that addCycleTime adds to the
// existing value rather than replacing it (accumulate semantics are needed
// because goscape splits several TS sections across multiple passes).
func TestAddCycleTime_Accumulates(t *testing.T) {
	s := newTestServer(t)
	s.cycleStats[statWorld] = 0

	// Two passes, each ~5 ms in the past, should sum to at least 8 ms total.
	s.addCycleTime(statWorld, time.Now().Add(-5*time.Millisecond))
	s.addCycleTime(statWorld, time.Now().Add(-5*time.Millisecond))

	if got := s.cycleStats[statWorld]; got < 8 {
		t.Errorf("cycleStats[WORLD] = %d after two 5ms adds, want >= 8", got)
	}
}

// TestAddCycleTime_Uint16Wrap verifies that the counter wraps mod 65536,
// matching TS Uint16Array truncation.
func TestAddCycleTime_Uint16Wrap(t *testing.T) {
	s := newTestServer(t)
	// Seed near overflow — 10 ms past should push it over 65535.
	s.cycleStats[statWorld] = 65530
	s.addCycleTime(statWorld, time.Now().Add(-10*time.Millisecond))

	// The result must be the wrapped value (< 65530 means it wrapped).
	if got := s.cycleStats[statWorld]; got >= 65530 {
		t.Errorf("cycleStats[WORLD] = %d, expected uint16 wrap (< 65530)", got)
	}
}

// TestResetCycleTimes_ZerosTimingButNotBandwidth verifies that resetCycleTimes
// zeroes timing entries (statCycle..statCleanup) but leaves the two bandwidth
// counters (statBandwidthIn, statBandwidthOut) untouched.
func TestResetCycleTimes_ZerosTimingButNotBandwidth(t *testing.T) {
	s := newTestServer(t)

	// Seed all 12 entries.
	for i := 0; i < numWorldStats; i++ {
		s.cycleStats[i] = uint16(100 + i)
	}

	s.resetCycleTimes()

	// Timing entries must be zero.
	for i := statCycle; i <= statCleanup; i++ {
		if got := s.cycleStats[i]; got != 0 {
			t.Errorf("cycleStats[%d] = %d after resetCycleTimes, want 0", i, got)
		}
	}

	// Bandwidth counters must be untouched.
	if got := s.cycleStats[statBandwidthIn]; got != uint16(100+statBandwidthIn) {
		t.Errorf("cycleStats[BANDWIDTH_IN] = %d, want %d (not reset)", got, 100+statBandwidthIn)
	}
	if got := s.cycleStats[statBandwidthOut]; got != uint16(100+statBandwidthOut) {
		t.Errorf("cycleStats[BANDWIDTH_OUT] = %d, want %d (not reset)", got, 100+statBandwidthOut)
	}
}

// TestSnapshotCycleStats_Copies verifies that snapshotCycleStats copies
// all 12 entries from cycleStats into lastCycleStats.
func TestSnapshotCycleStats_Copies(t *testing.T) {
	s := newTestServer(t)

	for i := 0; i < numWorldStats; i++ {
		s.cycleStats[i] = uint16(200 + i)
		s.lastCycleStats[i] = 0
	}

	s.snapshotCycleStats()

	for i := 0; i < numWorldStats; i++ {
		want := uint16(200 + i)
		if got := s.lastCycleStats[i]; got != want {
			t.Errorf("lastCycleStats[%d] = %d after snapshot, want %d", i, got, want)
		}
	}
}

// TestLastCycleStat_BoundsCheck verifies that LastCycleStat returns 0 for
// out-of-range indices (-1 and numWorldStats).
func TestLastCycleStat_BoundsCheck(t *testing.T) {
	s := newTestServer(t)

	// Seed a non-zero value at a valid index to confirm the accessor works normally.
	s.lastCycleStats[statCycle] = 42
	if got := s.LastCycleStat(statCycle); got != 42 {
		t.Errorf("LastCycleStat(statCycle) = %d, want 42", got)
	}

	// Below-range.
	if got := s.LastCycleStat(-1); got != 0 {
		t.Errorf("LastCycleStat(-1) = %d, want 0", got)
	}

	// At-range (== numWorldStats, which is 12).
	if got := s.LastCycleStat(numWorldStats); got != 0 {
		t.Errorf("LastCycleStat(%d) = %d, want 0", numWorldStats, got)
	}
}

// TestCycleStats_ResetAndSnapshotPipeline is a deterministic integration
// pin: seed stale sentinels in cycleStats and lastCycleStats, run the
// full per-tick pipeline by calling the three stat-management functions in
// their tick-loop order, and assert that the snapshot reflects the reset.
//
// This does not drive the tick loop goroutine — it calls the same
// functions the tick loop calls, on the test goroutine, which is legal
// for tick-goroutine-owned fields under single-threaded test execution.
func TestCycleStats_ResetAndSnapshotPipeline(t *testing.T) {
	s := newTestServer(t)

	// Stale values from a "previous" tick.
	s.cycleStats[statWorld] = 9999
	s.lastCycleStats[statWorld] = 7777

	// Simulate the tick-loop sequence:
	//   1. resetCycleTimes() at tick start
	//   2. addCycleTime(statWorld, ...) during the world phase
	//   3. snapshotCycleStats() at cycle end
	s.resetCycleTimes()
	// Timing measurement: 1 ms in the past ensures at least 1 ms recorded
	// even on fast hardware.
	s.addCycleTime(statWorld, time.Now().Add(-1*time.Millisecond))
	s.snapshotCycleStats()

	got := s.LastCycleStat(statWorld)
	// Must not be the stale sentinel from the previous tick's snapshot.
	if got == 7777 {
		t.Fatal("lastCycleStats[WORLD] = 7777 — snapshotCycleStats did not run")
	}
	// Must not be the raw stale cycleStats value that was present before reset.
	if got == 9999 {
		t.Fatal("lastCycleStats[WORLD] = 9999 — resetCycleTimes did not run")
	}
	// Must be at least 1 (some time was recorded after reset).
	if got < 1 {
		t.Fatalf("lastCycleStats[WORLD] = %d, want >= 1 (addCycleTime did not accumulate)", got)
	}
}
