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
