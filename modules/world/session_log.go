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

// SessionLog mirrors TS SessionLog (SessionLog.ts:1-7). One entry per
// addSessionLog call; flushed batched per tick by Server.processSessionLogs.
type SessionLog struct {
	SessionUUID string          // TS session_uuid
	Timestamp   int64           // TS timestamp (ms since epoch via time.Now().UnixMilli())
	Coord       int             // TS coord (CoordGrid.packCoord(level,x,z))
	Event       string          // TS event (message + ' ' + args.join(' ') if args, else message)
	EventType   LoggerEventType // TS event_type
}
