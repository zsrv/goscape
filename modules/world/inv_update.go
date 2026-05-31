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
	// Clamp to the target component's slot grid. TS UpdateInvFullEncoder
	// (UpdateInvFullEncoder.ts:14) sends `Math.min(inv.capacity,
	// comType.width * comType.height)` UNCONDITIONALLY — a comType whose
	// grid is 0 yields size=0 (empty send), not the full inv capacity.
	// Sending more slots than the component can hold overruns the client's
	// invSlotObjId[] array and crashes it. trademain:inv is a 28-slot
	// grid, but tradeconfirm:inv1/inv2 are smaller grids — and a routing
	// to a zero-grid component must produce size=0 to match TS, NOT the
	// inv capacity (pre-fix the `grid > 0` guard let a zero-grid comType
	// fall through to a full-capacity send). Closes inventory-2 /
	// gap-server-codec-models-1 (2026-05-28 fresh-audit MED). The
	// nil-component fallback (Go-side robustness for the bare-fixture
	// test path) keeps size = inv.Capacity; TS would throw on Component.get
	// returning undefined, which production callers never trigger because
	// inventory routes are component-validated upstream.
	if p.client != nil && p.client.server != nil {
		if ct := p.client.server.lookupComponent(com); ct != nil {
			if grid := ct.Width * ct.Height; grid < size {
				size = grid
			}
		}
	}
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

// sendUpdateInvPartial writes an UpdateInvPartial packet for the listed slots.
// Used by handleInvButtonD to revert the client drag visual when the player
// is delayed. Mirrors TS UpdateInvPartialEncoder.ts:9-32.
func sendUpdateInvPartial(p *Player, com int, inv *inventory.Inventory, slots ...int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	for _, slot := range slots {
		item := inv.Get(slot)
		buf.P1(uint8(slot))
		if item != nil {
			buf.P2(uint16(item.Id + 1))
			if item.Count >= 255 {
				buf.P1(255)
				buf.P4(uint32(item.Count))
			} else {
				buf.P1(uint8(item.Count))
			}
		} else {
			buf.P2(0)
			buf.P1(0)
		}
	}
	p.writeOut(gameserver.OpUpdateInvPartial, buf.Bytes())
}
