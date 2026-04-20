package world

import (
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendUpdateInvFullCom writes an UpdateInvFull packet for a single inventory
// routed to the given UI component. Matches TS UpdateInvFullEncoder:
//
//	p2(com) p1(size)
//	per slot: p2(id+1) p1(count) OR p1(255)+p4(count) for count >= 255,
//	          or p2(0)+p1(0) for empty slots.
func sendUpdateInvFullCom(p *Player, com int, inv *inventory.Inventory) {
	buf := packet.NewPacket(nil)

	buf.P2(uint16(com))
	size := inv.Capacity
	if size > 0xff {
		size = 0xff
	}
	buf.P1(uint8(size))
	for slot := range size {
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
