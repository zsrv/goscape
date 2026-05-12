package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendUpdateRebootTimer writes one UPDATE_REBOOT_TIMER packet carrying
// the remaining tick count (NOT seconds). Mirrors TS
// UpdateRebootTimerEncoder (`buf.p2(message.ticks)`). NAI-182.
func sendUpdateRebootTimer(p *Player, ticks int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(int16(ticks)))
	p.writeOut(gameserver.OpUpdateRebootTimer, buf.Bytes())
}

// rebootTimer schedules a world reboot in `duration` ticks and
// broadcasts the new countdown to every connected player in
// s.playerLoop. Mirrors TS World.rebootTimer (World.ts:1787-1793).
// NAI-182.
func (s *Server) rebootTimer(duration int) {
	s.shutdownTick = s.currentTick + duration
	for _, p := range s.playerLoop {
		if p == nil {
			continue
		}
		sendUpdateRebootTimer(p, s.shutdownTick-s.currentTick)
	}
}

// isPendingShutdown reports whether a shutdown is currently scheduled.
// Mirrors TS World.isPendingShutdown (World.ts:1795-1797). Equivalent
// to s.shutdownTicksRemaining() > -1. NAI-182.
func (s *Server) isPendingShutdown() bool {
	return s.shutdownTicksRemaining() > -1
}

// shutdownTicksRemaining returns shutdownTick - currentTick. Returns a
// negative number when no shutdown is scheduled (shutdownTick == -1).
// Mirrors TS World.shutdownTicksRemaining (World.ts:1799-1801). NAI-182.
func (s *Server) shutdownTicksRemaining() int {
	return s.shutdownTick - s.currentTick
}

// processShutdown runs at the top of s.tick() when s.shutdownTick != -1
// && s.currentTick >= s.shutdownTick. Mirrors TS World.processShutdown
// (World.ts:1198-1226). NAI-182.
func (s *Server) processShutdown() {
	// (a) For every connected player, request logout. TS calls
	// player.logout() + player.client.close() inline; goscape reuses
	// the existing logout machinery (processLogouts drain path) by
	// flagging p.loggingOut. The current tick's processLogouts will
	// then run the standard logout sequence.
	for _, p := range s.playerLoop {
		if p != nil && p.client != nil {
			p.loggingOut = true
		}
	}

	duration := s.currentTick - s.shutdownTick

	// (b) After 1024 ticks (~10 minutes at 600ms/tick), force-remove any
	// player that hasn't completed logout. Mirrors TS World.processShutdown
	// (W.ts:1207-1213). The p.forceRemove flag drives processLogouts'
	// force-branch.
	if duration >= 1024 {
		for _, p := range s.playerLoop {
			if p != nil {
				p.forceRemove = true
			}
		}
	}

	// (c) Graceful exit when zero players remain. TS calls
	// process.exit(0); goscape signals via shutdownGraceful + closes
	// gracefulExit. The tick loop returns; Server.Run() selects on
	// gracefulExit and returns nil; world.go runFn checks
	// shutdownGraceful to distinguish from "unexpected" stop.
	//
	// We deliberately do NOT close(s.quit) — the dskit stoppingFn later
	// calls Server.Shutdown() which closes s.quit; double-close would panic.
	if s.getTotalPlayers() == 0 {
		s.shutdownGraceful = true
		close(s.gracefulExit)
	}
}
