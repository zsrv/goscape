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

// removeNpc marks n as logically absent from the world by setting
// n.dead = true. Does NOT remove n from s.npcs[] or s.npcLoop —
// that registry manipulation is deferred to a future sub-spec
// when script-driven NPC creation/deletion lands. The old
// registry-manipulation body was unused pre-NAI-5 and was
// mid-tick-iteration-unsafe (spliced npcLoop during processNpcs
// iteration), so replacing it with the dead-bool model is also a
// correctness improvement.
func (s *Server) removeNpc(n *Npc) {
	n.dead = true
}
