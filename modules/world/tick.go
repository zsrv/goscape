package world

import (
	"time"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/inventory"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

const tickRate = 600 * time.Millisecond

const (
	timeoutNoResponse   = 100 // ticks = 60s at 600ms
	timeoutNoConnection = 50  // ticks = 30s at 600ms
)

func (s *Server) runTickLoop() {
	s.runTickLoopWithRate(tickRate)
}

func (s *Server) runTickLoopWithRate(rate time.Duration) {
	nextTick := time.Now()
	for {
		start := time.Now()
		drift := start.Sub(nextTick)
		if drift < 0 {
			drift = 0
		}

		s.processClientsIn()
		s.processPathing()
		s.processNpcs()
		s.processLogouts()
		s.processLogins()
		s.processInfo()
		s.processClientsOut()
		s.processCleanup()
		s.currentTick++

		nextTick = nextTick.Add(rate)
		delay := rate - time.Since(start) - drift
		if delay < 0 {
			delay = 0
		}

		select {
		case <-s.quit:
			return
		case <-time.After(delay):
		}
	}
}

func (s *Server) processClientsIn() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		p.processIn(s.currentTick)
	}
}

func (s *Server) processLogins() {
	s.playersMu.Lock()
	batch := s.newPlayers
	s.newPlayers = nil
	s.playersMu.Unlock()

	for _, p := range batch {
		if err := s.addPlayer(p); err != nil {
			// world full — reject cleanly
			p.writeOut(gameserver.OpLogout, nil)
			_ = p.client.flushWrite()
			_ = p.client.conn.Close()
			continue
		}
		p.lastConnected = s.currentTick
		p.lastResponse = s.currentTick
		p.originX = p.x
		p.originZ = p.z

		// sub-spec 3a: initialise buildarea, worn inventory, and appearance dirty flag
		p.buildArea = buildarea.New()
		p.invs = map[int]*inventory.Inventory{}
		if s.invTypes != nil && s.invTypes.Worn >= 0 && s.invTypes.Worn < len(s.invTypes.Configs) {
			wornType := s.invTypes.Configs[s.invTypes.Worn]
			if wornType != nil {
				worn := inventory.FromType(wornType)
				worn.Update = true
				p.invs[s.invTypes.Worn] = worn
			}
		}
		p.masks |= MaskAppearance
	}
}

func (s *Server) processLogouts() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		force := false
		if s.currentTick-p.lastResponse >= timeoutNoResponse {
			p.loggingOut = true
			force = true
		} else if s.currentTick-p.lastConnected >= timeoutNoConnection {
			p.requestIdleLogout = true
		}

		if p.requestLogout || p.requestIdleLogout {
			if s.currentTick >= p.preventLogoutUntil {
				p.loggingOut = true
			}
			p.requestLogout = false
			p.requestIdleLogout = false
		}

		if p.loggingOut && (force || s.currentTick >= p.preventLogoutUntil) {
			p.writeOut(gameserver.OpLogout, nil)
			_ = p.client.flushWrite()
			_ = p.client.conn.Close()
			s.removePlayer(p)
		}
	}
}

func (s *Server) processPathing() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		p.resolveMovement()
	}
}

func (s *Server) processClientsOut() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		p.processOut()
	}
}

func (s *Server) processInfo() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	// Update grid when players cross zone boundaries.
	for _, p := range players {
		curZX, curZZ := p.x>>3, p.z>>3
		prevZX, prevZZ := p.lastTickX>>3, p.lastTickZ>>3
		if prevZX != curZX || prevZZ != curZZ || p.lastLevel != p.level {
			if p.lastTickX >= 0 {
				s.grid.Remove(p.slot, p.lastTickX, p.lastTickZ, p.lastLevel)
			}
			s.grid.Add(p.slot, p.x, p.z, p.level)
		}
	}

	sources := make([]rsbuf.PlayerSource, len(players))
	for i, p := range players {
		sources[i] = p
	}
	s.renderer.ComputePlayers(sources)

	npcSources := make([]rsbuf.NpcSource, len(s.npcLoop))
	for i, n := range s.npcLoop {
		npcSources[i] = n
	}
	s.renderer.ComputeNpcs(npcSources)
}

func (s *Server) processNpcs() {
	for _, n := range s.npcLoop {
		prevX, prevZ, prevLevel := n.x, n.z, n.level
		n.turn(s)
		if n.x != prevX || n.z != prevZ || n.level != prevLevel {
			s.grid.RemoveNpc(n.nid, prevX, prevZ, prevLevel)
			s.grid.AddNpc(n.nid, n.x, n.z, n.level)
		}
	}
}

func (s *Server) processCleanup() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()
	for _, p := range players {
		p.ResetMasks()
	}
	for _, n := range s.npcLoop {
		n.ResetMasks()
	}
}
