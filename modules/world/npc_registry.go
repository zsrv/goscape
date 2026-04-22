package world

import "errors"

var errNpcsFull = errors.New("npc registry full")

// allocNpcSlot returns a free nid (1..8191). Returns -1 if full.
func (s *Server) allocNpcSlot() int {
	for offset := 0; offset < len(s.npcs)-1; offset++ {
		i := s.nextNpcSlot + offset
		if i < 1 {
			i = 1
		}
		if i >= len(s.npcs) {
			i = (i % (len(s.npcs) - 1)) + 1
		}
		if s.npcs[i] == nil {
			s.nextNpcSlot = (i + 1) % len(s.npcs)
			if s.nextNpcSlot < 1 {
				s.nextNpcSlot = 1
			}
			return i
		}
	}
	return -1
}

// addNpc places n into a free slot, sets n.nid, appends to npcLoop.
// Caller responsible for synchronisation (called during NewServer or under playersMu).
func (s *Server) addNpc(n *Npc) error {
	nid := s.allocNpcSlot()
	if nid < 0 {
		return errNpcsFull
	}
	n.nid = nid
	n.server = s
	s.npcs[nid] = n
	s.npcLoop = append(s.npcLoop, n)
	return nil
}

// removeNpc clears the npc's slot and removes from npcLoop.
func (s *Server) removeNpc(n *Npc) {
	if n.nid < 1 || n.nid >= len(s.npcs) || s.npcs[n.nid] != n {
		return
	}
	s.npcs[n.nid] = nil
	for i, ln := range s.npcLoop {
		if ln == n {
			s.npcLoop = append(s.npcLoop[:i], s.npcLoop[i+1:]...)
			return
		}
	}
}
