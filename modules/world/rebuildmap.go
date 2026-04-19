package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendRebuildNormal writes a RebuildNormal packet for the player.
// Mirrors TS RebuildNormalEncoder: p2(zoneX), p2(zoneZ), per mapsquare:
// p1(mapX), p1(mapZ), p4(mCRC), p4(lCRC).
func sendRebuildNormal(p *Player, mapsquares []uint16) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(p.x >> 3))
	buf.P2(uint16(p.z >> 3))

	for _, msq := range mapsquares {
		mx := int(msq >> 8)
		mz := int(msq & 0xff)
		var mCRC, lCRC uint32
		if p.client != nil && p.client.server != nil && p.client.server.gamemap != nil {
			mCRC, lCRC = p.client.server.gamemap.MapsquareCRC(mx, mz)
		}
		buf.P1(uint8(mx))
		buf.P1(uint8(mz))
		buf.P4(mCRC)
		buf.P4(lCRC)
	}
	p.writeOut(gameserver.OpRebuildNormal, buf.Bytes())
}
