package world

// BroadcastMes sends a MESSAGE_GAME packet to every logged-in player.
// Mirrors TS World.broadcastMes (single-line forEach over players).
// Holds Server.playersMu.RLock for the duration of the fan-out —
// callers must NOT hold playersMu. Used by ::broadcast cheat arm
// (NAI-185 T8) and any future server-wide announcement path.
func (s *Server) BroadcastMes(msg string) {
	s.playersMu.RLock()
	defer s.playersMu.RUnlock()
	for _, p := range s.players {
		if p == nil {
			continue
		}
		p.MessageGame(msg)
	}
}
