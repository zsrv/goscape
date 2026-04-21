package world

// worldVarsView adapts *Server to script.WorldVars. Kept value-typed so
// tests can construct it without a running server.
type worldVarsView struct {
	s *Server
}

func (w worldVarsView) VarsInt(id int) int32 {
	if w.s == nil || id < 0 || id >= len(w.s.vars) {
		return 0
	}
	return w.s.vars[id]
}

func (w worldVarsView) SetVarsInt(id int, val int32) {
	if w.s == nil || id < 0 || id >= len(w.s.vars) {
		return
	}
	w.s.vars[id] = val
}

func (w worldVarsView) VarsString(id int) string {
	if w.s == nil || id < 0 || id >= len(w.s.varsStrings) {
		return ""
	}
	return w.s.varsStrings[id]
}

func (w worldVarsView) SetVarsString(id int, val string) {
	if w.s == nil || id < 0 || id >= len(w.s.varsStrings) {
		return
	}
	w.s.varsStrings[id] = val
}

// CurrentTick returns the server's current tick counter. Used by
// MAP_CLOCK opcode.
func (w worldVarsView) CurrentTick() int {
	if w.s == nil {
		return 0
	}
	return w.s.currentTick
}

// PlayerCount returns the number of players currently in the world.
// Used by PLAYERCOUNT opcode.
func (w worldVarsView) PlayerCount() int {
	if w.s == nil {
		return 0
	}
	w.s.playersMu.RLock()
	n := len(w.s.playerLoop)
	w.s.playersMu.RUnlock()
	return n
}

// MapMembers returns 1 if the server is a members world, else 0.
// Matches TS Environment.NODE_MEMBERS. Used by MAP_MEMBERS opcode.
func (w worldVarsView) MapMembers() int {
	if w.s == nil || !w.s.cfg.NodeMembers {
		return 0
	}
	return 1
}

// MapLive returns 1 if the server is in production mode, else 0.
// Matches TS Environment.NODE_PRODUCTION. Used by MAP_LIVE opcode.
func (w worldVarsView) MapLive() int {
	if w.s == nil || !w.s.cfg.NodeProduction {
		return 0
	}
	return 1
}
