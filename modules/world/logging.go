package world

import "log/slog"

// Component attribute values for this module's loggers. Every world log
// line carries exactly one component= attr; the dotted prefix encodes the
// module. Children are derived from the RAW (un-stamped) module logger so
// a line never carries two component= attrs.
const (
	compWorld   = "world"         // module-level / fallback
	compServer  = "world.server"  // server lifecycle (listen/load/start/stop)
	compNet     = "world.net"     // per-connection packet I/O
	compTick    = "world.tick"    // per-tick processing
	compScript  = "world.script"  // RuneScript engine
	compFriends = "world.friends" // friends-server RPC
	compLogin   = "world.login"   // login-server RPC
	compContent = "world.content" // content watcher / hot reload
	compReport  = "world.report"  // player report / session log / input track
)

// initChildLoggers derives the per-subsystem component child loggers from the
// provided RAW (un-stamped) base logger. Passing an already-stamped logger
// (e.g. s.log which carries component=world.server) would cause every child
// line to emit two component= attrs — callers must pass the un-stamped base.
// Called by NewServer (passing the raw logger parameter) and by test helpers
// that construct a Server directly (passing their plain discard/buffer logger).
// Derives logNet, logTick, logScript, logContent, and logFriends children.
func (s *Server) initChildLoggers(base *slog.Logger) {
	s.logNet = base.With("component", compNet)
	s.logTick = base.With("component", compTick)
	s.logScript = base.With("component", compScript)
	s.logContent = base.With("component", compContent)
	s.logFriends = base.With("component", compFriends)
}
