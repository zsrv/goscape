package world

import (
	"time"

	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
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
		s.processLogouts()
		s.processLogins()
		s.processClientsOut()
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
