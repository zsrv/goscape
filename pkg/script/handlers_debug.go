package script

import (
	"fmt"
	"time"
)

// WorldStat indexes (TS WorldStat.ts:1-14; mirrored in
// modules/world/world_stats.go — keep in sync).
const (
	worldStatCycle        = iota // 0
	worldStatWorld               // 1
	worldStatClientIn            // 2
	worldStatNpc                 // 3
	worldStatPlayer              // 4
	worldStatLogout              // 5
	worldStatLogin               // 6
	worldStatZone                // 7
	worldStatClientOut           // 8
	worldStatCleanup             // 9
	worldStatBandwidthIn         // 10
	worldStatBandwidthOut        // 11
)

// handleError aborts the script with a scripted error message. The
// error propagates up to runScript which logs it; Execution is set to
// Aborted by the dispatch loop.
func handleError(s *ScriptState) error {
	msg := s.PopString()
	return fmt.Errorf("ERROR: %s", msg)
}

// handleTimeSpent starts the script-side stopwatch by recording the
// current monotonic time in state.Timespent. Mirrors TS DebugOps.ts:13
// `state.timespent = performance.now()`. No active-player gate — TS
// doesn't require one, and the stopwatch is per-ScriptState not per-player.
func handleTimeSpent(s *ScriptState) error {
	s.Timespent = time.Now()
	return nil
}

// handleGetTimeSpent pops a unit flag and pushes elapsed time since the
// last TIMESPENT call: milliseconds when flag != 1, microseconds when
// flag == 1. Mirrors TS DebugOps.ts:16-26.
func handleGetTimeSpent(s *ScriptState) error {
	unit := s.PopInt()
	elapsed := time.Since(s.Timespent)
	if unit == 1 {
		s.PushInt(int(elapsed.Microseconds()))
	} else {
		s.PushInt(int(elapsed.Milliseconds()))
	}
	return nil
}

// handleMapProduction (MAP_PRODUCTION, opcode 10001) pushes the NODE_PRODUCTION
// flag. TS DebugOps.ts:16-18 — the 225 MAP_LIVE body, relocated to DebugOps
// and renamed at 244.
func handleMapProduction(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_PRODUCTION: %w", ErrNoWorld)
	}
	s.PushInt(s.World.MapProduction())
	return nil
}

// handleMapLastClock (MAP_LASTCLOCK, opcode 10002) pushes
// lastCycleStats[WorldStat.CYCLE]. TS DebugOps.ts:20-22.
func handleMapLastClock(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LASTCLOCK: %w", ErrNoWorld)
	}
	s.PushInt(s.World.LastCycleStat(worldStatCycle))
	return nil
}

// handleMapLastWorld (MAP_LASTWORLD, opcode 10003) pushes
// lastCycleStats[WorldStat.WORLD]. TS DebugOps.ts:24-26.
func handleMapLastWorld(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LASTWORLD: %w", ErrNoWorld)
	}
	s.PushInt(s.World.LastCycleStat(worldStatWorld))
	return nil
}

// handleMapLastClientIn (MAP_LASTCLIENTIN, opcode 10004) pushes
// lastCycleStats[WorldStat.CLIENT_IN]. TS DebugOps.ts:28-30.
func handleMapLastClientIn(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LASTCLIENTIN: %w", ErrNoWorld)
	}
	s.PushInt(s.World.LastCycleStat(worldStatClientIn))
	return nil
}

// handleMapLastNpc (MAP_LASTNPC, opcode 10005) pushes
// lastCycleStats[WorldStat.NPC]. TS DebugOps.ts:32-34.
func handleMapLastNpc(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LASTNPC: %w", ErrNoWorld)
	}
	s.PushInt(s.World.LastCycleStat(worldStatNpc))
	return nil
}

// handleMapLastPlayer (MAP_LASTPLAYER, opcode 10006) pushes
// lastCycleStats[WorldStat.PLAYER]. TS DebugOps.ts:36-38.
func handleMapLastPlayer(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LASTPLAYER: %w", ErrNoWorld)
	}
	s.PushInt(s.World.LastCycleStat(worldStatPlayer))
	return nil
}

// handleMapLastLogout (MAP_LASTLOGOUT, opcode 10007) pushes
// lastCycleStats[WorldStat.LOGOUT]. TS DebugOps.ts:40-42.
func handleMapLastLogout(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LASTLOGOUT: %w", ErrNoWorld)
	}
	s.PushInt(s.World.LastCycleStat(worldStatLogout))
	return nil
}

// handleMapLastLogin (MAP_LASTLOGIN, opcode 10008) pushes
// lastCycleStats[WorldStat.LOGIN]. TS DebugOps.ts:44-46.
func handleMapLastLogin(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LASTLOGIN: %w", ErrNoWorld)
	}
	s.PushInt(s.World.LastCycleStat(worldStatLogin))
	return nil
}

// handleMapLastZone (MAP_LASTZONE, opcode 10009) pushes
// lastCycleStats[WorldStat.ZONE]. TS DebugOps.ts:48-50.
func handleMapLastZone(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LASTZONE: %w", ErrNoWorld)
	}
	s.PushInt(s.World.LastCycleStat(worldStatZone))
	return nil
}

// handleMapLastClientOut (MAP_LASTCLIENTOUT, opcode 10010) pushes
// lastCycleStats[WorldStat.CLIENT_OUT]. TS DebugOps.ts:52-54.
func handleMapLastClientOut(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LASTCLIENTOUT: %w", ErrNoWorld)
	}
	s.PushInt(s.World.LastCycleStat(worldStatClientOut))
	return nil
}

// handleMapLastCleanup (MAP_LASTCLEANUP, opcode 10011) pushes
// lastCycleStats[WorldStat.CLEANUP]. TS DebugOps.ts:56-58.
func handleMapLastCleanup(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LASTCLEANUP: %w", ErrNoWorld)
	}
	s.PushInt(s.World.LastCycleStat(worldStatCleanup))
	return nil
}

// handleMapLastBandwidthIn (MAP_LASTBANDWIDTHIN, opcode 10012) pushes
// lastCycleStats[WorldStat.BANDWIDTH_IN]. TS DebugOps.ts:60-62.
func handleMapLastBandwidthIn(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LASTBANDWIDTHIN: %w", ErrNoWorld)
	}
	s.PushInt(s.World.LastCycleStat(worldStatBandwidthIn))
	return nil
}

// handleMapLastBandwidthOut (MAP_LASTBANDWIDTHOUT, opcode 10013) pushes
// lastCycleStats[WorldStat.BANDWIDTH_OUT]. TS DebugOps.ts:64-66.
func handleMapLastBandwidthOut(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LASTBANDWIDTHOUT: %w", ErrNoWorld)
	}
	s.PushInt(s.World.LastCycleStat(worldStatBandwidthOut))
	return nil
}
