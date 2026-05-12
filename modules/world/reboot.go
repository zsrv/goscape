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
