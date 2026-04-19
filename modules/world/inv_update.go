package world

import (
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendUpdateInvFull writes an UpdateInvFull packet for a single inventory.
// Mirrors TS UpdateInvFullEncoder: p2(component), p1(size), per slot either
// p2(id+1)+p1(count) (small counts fit in 1 byte; larger use p1(255)+p4(count))
// or p2(0)+p1(0) for empty slots.
//
// Sub-spec 3a uses invId as the component placeholder; sub-spec 3b+ consults
// p.invListeners to route updates to each subscriber's component id.
func sendUpdateInvFull(p *Player, invId int, inv *inventory.Inventory) {
	buf := packet.NewPacket(nil)

	com := invId

	buf.P2(uint16(com))
	size := inv.Capacity
	if size > 0xff {
		size = 0xff
	}
	buf.P1(uint8(size))
	for slot := 0; slot < size; slot++ {
		item := inv.Get(slot)
		if item == nil {
			buf.P2(0)
			buf.P1(0)
			continue
		}
		buf.P2(uint16(item.Id + 1))
		if item.Count >= 255 {
			buf.P1(255)
			buf.P4(uint32(item.Count))
		} else {
			buf.P1(uint8(item.Count))
		}
	}

	p.writeOut(gameserver.OpUpdateInvFull, buf.Bytes())
}
