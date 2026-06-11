package world

// LoggerEventType is the TS LoggerEventType numeric enum domain
// (LoggerEventType.ts:1-9). Untyped int alias keeps the script-side
// ActivePlayer interface signature simple; production callers use the
// named constants below.
type LoggerEventType = int

const (
	LoggerEventTypeEngine    LoggerEventType = 0 // server engine only
	LoggerEventTypeWealth    LoggerEventType = 1 // wealth_log (separate buffer; not in NAI-74)
	LoggerEventTypeModerator LoggerEventType = 2 // session_log moderator channel
	LoggerEventTypeAdventure LoggerEventType = 3 // visible to players
)

// PlayerCoordLogRate mirrors TS World.PLAYER_COORDLOGRATE = 50
// (World.ts:125). Every PlayerCoordLogRate ticks (with tick > 0), each
// active player emits a MODERATOR "Server check in" record.
const PlayerCoordLogRate = 50

// SessionLog mirrors the TS session-log row shape pushed by
// World.addSessionLog (World.ts:2234-2243 @2e3bcf43). One entry per
// addSessionLog call; flushed batched per tick by Server.processSessionLogs.
//
// rev-254 A3: account_id dropped — the row is keyed by session_uuid
// only (World.addSessionLog signature lost its account_id parameter in
// TS 43e02957..2e3bcf43).
type SessionLog struct {
	SessionUUID string          // TS session_uuid
	Timestamp   int64           // TS timestamp (ms since epoch via time.Now().UnixMilli())
	Coord       int             // TS coord (CoordGrid.packCoord(level,x,z))
	Event       string          // TS event (message + ' ' + args.join(' ') if args, else message)
	EventType   LoggerEventType // TS event_type
}

// processSessionLogs runs as the last tick phase (after processCleanup,
// before currentTick++). Mirrors TS World.cycle() session-log block at
// World.ts:428-442:
//  1. If currentTick > 0 && currentTick % PlayerCoordLogRate == 0,
//     push MODERATOR "Server check in" for every player in the players list.
//  2. If sessionLogs is non-empty, dispatch via loggerBridge then clear.
//
// Empty-buffer skip matches TS (World.ts:435 `if (sessionLogs.length > 0)`).
// Coord-log push runs BEFORE flush so server-check-in entries land in
// the SAME tick's batch (matches TS source ordering at World.ts:428-442).
func (s *Server) processSessionLogs() {
	if s.currentTick > 0 && s.currentTick%PlayerCoordLogRate == 0 {
		for p := range s.players.all() {
			// AddSessionLog reacquires sessionLogsMu; do NOT hold it across
			// this loop. Arc 18 R1.
			p.AddSessionLog(LoggerEventTypeModerator, "Server check in")
		}
	}
	// Snapshot+clear under the mutex so concurrent appends from packet/
	// script goroutines do not race with the per-tick dispatch. Arc 18 R1.
	s.sessionLogsMu.Lock()
	logs := s.sessionLogs
	s.sessionLogs = nil
	s.sessionLogsMu.Unlock()
	if len(logs) > 0 {
		s.loggerBridge.SubmitSessionLogs(logs)
	}
}
