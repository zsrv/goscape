package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendUpdateStat writes one UpdateStat packet for the given skill slot.
// Wire: p1(stat) p4(exp/10) p1(level). XP is lossy on the wire (divided by 10).
func sendUpdateStat(p *Player, stat, exp, level int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(stat))
	buf.P4(uint32(exp / 10))
	buf.P1(uint8(level))
	p.writeOut(gameserver.OpUpdateStat, buf.Bytes())
}

// sendUpdateRunEnergy writes one UpdateRunEnergy packet.
// Internal energy is 0-10000; the wire value is energy/100 (0-100 byte).
func sendUpdateRunEnergy(p *Player, energy int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(energy / 100))
	p.writeOut(gameserver.OpUpdateRunEnergy, buf.Bytes())
}

// sendUpdateRunWeight writes one UpdateRunWeight packet (kg field is
// the truncated runweight/1000). Mirrors TS UpdateRunWeightEncoder
// (`buf.p2(kg)`). Negative kg is signed-16-bit-encoded for parity with
// TS p2 (which is signed); in practice kg is always >= 0 since
// calculateRunWeight sums non-negative ObjType.Weight values.
//
// NAI-136.
func sendUpdateRunWeight(p *Player, kg int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(int16(kg)))
	p.writeOut(gameserver.OpUpdateRunWeight, buf.Bytes())
}
