package world

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/zone"
)

// writeFullFollows sends UpdateZoneFullFollows (client zone reset) followed
// by a PartialFollows wrapper + synthesized per-entity messages replaying
// every currently-active dynamic loc/obj in the zone. Entities transitioned
// THIS tick are skipped — the Enclosed buffer already carries their change.
//
// TODO(beyond-4b): handle Respawn-lifecycle (static) loc branches once
// static loading from cache maps is wired up.
func (p *Player) writeFullFollows(z *zone.Zone, currentTick int) {
	buf := packet.NewPacket(nil)
	rsbuf.EncodeZoneFullFollows(buf, z.X, z.Z, p.originX, p.originZ)
	p.writeOut(gameserver.OpUpdateZoneFullFollows, buf.Bytes())

	hasMessages := false
	ensureHeader := func() {
		if hasMessages {
			return
		}
		hb := packet.NewPacket(nil)
		rsbuf.EncodeZonePartialFollows(hb, z.X, z.Z, p.originX, p.originZ)
		p.writeOut(gameserver.OpUpdateZonePartialFollows, hb.Bytes())
		hasMessages = true
	}

	for _, obj := range z.Objs {
		if obj.LastLifecycleTick == currentTick {
			continue
		}
		if obj.ReceiverID != zone.PublicReceiver && obj.ReceiverID != p.uid {
			continue
		}
		if !obj.CheckLifecycle(currentTick) {
			continue
		}
		ensureHeader()
		pb := packet.NewPacket(nil)
		rsbuf.EncodeObjAdd(pb, coordgrid.PackZoneCoord(obj.X, obj.Z), obj.Type, obj.Count)
		p.writeOut(gameserver.OpObjAdd, pb.Bytes())
	}

	for _, loc := range z.Locs {
		if loc.LastLifecycleTick == currentTick {
			continue
		}
		if loc.Lifecycle == entitypkg.LifecycleDespawn && loc.CheckLifecycle(currentTick) {
			ensureHeader()
			pb := packet.NewPacket(nil)
			rsbuf.EncodeLocAddChange(pb, coordgrid.PackZoneCoord(loc.X, loc.Z),
				loc.Shape(), loc.Angle(), loc.Type())
			p.writeOut(gameserver.OpLocAddChange, pb.Bytes())
		}
	}
}

// writePartialFollows iterates the zone's per-tick Follows events, filtered
// by recipient, emitting a PartialFollows header once (if any match) then
// each event as its own top-level zone-nested packet.
func (p *Player) writePartialFollows(z *zone.Zone) {
	hasAnyForMe := false
	for _, e := range z.Events() {
		if e.Type != zone.ZoneEventFollows || e.Bytes == nil {
			continue
		}
		if e.ReceiverID != zone.PublicReceiver && e.ReceiverID != p.uid {
			continue
		}
		if !hasAnyForMe {
			hb := packet.NewPacket(nil)
			rsbuf.EncodeZonePartialFollows(hb, z.X, z.Z, p.originX, p.originZ)
			p.writeOut(gameserver.OpUpdateZonePartialFollows, hb.Bytes())
			hasAnyForMe = true
		}
		sendZoneNested(p, e.Bytes)
	}
}

// sendZoneNested dispatches a [opcode_byte, ...payload] byte slice as a
// top-level packet using the Op{} registered for that zone-nested opcode.
func sendZoneNested(p *Player, b []byte) {
	if len(b) == 0 {
		return
	}
	p.writeOut(zoneNestedOp(b[0]), b[1:])
}

// zoneNestedOp maps a zone-nested opcode byte to its top-level Op{} entry.
// Panics on unknown opcodes — an assertion that only pkg/zone-produced
// bytes reach here.
func zoneNestedOp(op byte) gameserver.Op {
	switch op {
	case rsbuf.ZoneOpLocAddChange:
		return gameserver.OpLocAddChange
	case rsbuf.ZoneOpLocAnim:
		return gameserver.OpLocAnim
	case rsbuf.ZoneOpLocDel:
		return gameserver.OpLocDel
	case rsbuf.ZoneOpLocMerge:
		return gameserver.OpLocMerge
	case rsbuf.ZoneOpMapAnim:
		return gameserver.OpMapAnim
	case rsbuf.ZoneOpMapProjAnim:
		return gameserver.OpMapProjAnim
	case rsbuf.ZoneOpObjAdd:
		return gameserver.OpObjAdd
	case rsbuf.ZoneOpObjCount:
		return gameserver.OpObjCount
	case rsbuf.ZoneOpObjDel:
		return gameserver.OpObjDel
	case rsbuf.ZoneOpObjReveal:
		return gameserver.OpObjReveal
	}
	panic(fmt.Sprintf("unknown zone-nested opcode %d", op))
}
