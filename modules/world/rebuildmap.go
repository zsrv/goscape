package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendRebuildNormal writes a RebuildNormal packet for the player.
// TS RebuildNormalEncoder.ts (244): p2 zoneX, p2 zoneZ.
//
// The 225 per-mapsquare CRC list is GONE at 244 — the 244 client fetches
// maps via OnDemand, which lands in Bundle 3.
func sendRebuildNormal(p *Player) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(p.x >> 3))
	buf.P2(uint16(p.z >> 3))
	p.writeOut(gameserver.OpRebuildNormal, buf.Bytes())
}
