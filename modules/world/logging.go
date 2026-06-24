package world

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

// initChildLoggers derives the component child loggers from s.log. Called by
// NewServer during initialization and by test helpers that construct a Server
// directly (without going through NewServer) when they exercise code paths
// that use the child loggers.
func (s *Server) initChildLoggers() {
	s.logNet = s.log.With("component", compNet)
	s.logTick = s.log.With("component", compTick)
	s.logScript = s.log.With("component", compScript)
	s.logContent = s.log.With("component", compContent)
}
