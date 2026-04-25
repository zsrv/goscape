package world

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/cache"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendRebuildNormal writes a RebuildNormal packet for the player.
// Mirrors TS RebuildNormalEncoder.ts:10-21: p2(zoneX), p2(zoneZ),
// per mapsquare: p1(mapX), p1(mapZ), p4(mCRC), p4(lCRC).
//
// CRCs are read from cache.PreloadedCRC keyed by `m{x}_{z}` / `l{x}_{z}`
// per TS RebuildNormalEncoder.ts:18-19. Missing keys default to 0
// (TS `?? 0`).
func sendRebuildNormal(p *Player, mapsquares []uint16) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(p.x >> 3))
	buf.P2(uint16(p.z >> 3))

	for _, msq := range mapsquares {
		mx := int(msq >> 8)
		mz := int(msq & 0xff)
		mCRC := cache.PreloadedCRC[fmt.Sprintf("m%d_%d", mx, mz)]
		lCRC := cache.PreloadedCRC[fmt.Sprintf("l%d_%d", mx, mz)]
		buf.P1(uint8(mx))
		buf.P1(uint8(mz))
		buf.P4(mCRC)
		buf.P4(lCRC)
	}
	p.writeOut(gameserver.OpRebuildNormal, buf.Bytes())
}
