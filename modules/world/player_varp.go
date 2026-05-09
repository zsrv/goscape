package world

import (
	"encoding/hex"

	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
)

// writeVarp queues a VARP_SMALL or VARP_LARGE packet for the given
// varp change. Gated by VarPlayerType.Transmit — non-transmit varps
// stay server-only.
func (p *Player) writeVarp(id int, value int32) {
	cfg := p.varpTypeConfig(id)
	if cfg == nil || !cfg.Transmit {
		return
	}
	buf := packet.NewPacket(nil)
	buf.P2(uint16(id))
	var op gameserver.Op
	if value >= -128 && value <= 127 {
		buf.P1(uint8(int8(value)))
		op = gameserver.OpVarpSmall
	} else {
		buf.P4(uint32(value))
		op = gameserver.OpVarpLarge
	}
	payload := buf.Bytes()
	if p.client != nil && p.client.server != nil &&
		p.client.server.cfg.NodeDebug && p.client.server.log != nil {
		p.client.server.log.Info("nai138.write_varp",
			"tick", p.client.server.currentTick,
			"player_uid", p.uid,
			"id", id,
			"value", value,
			"opcode", int(op.Opcode),
			"payload_hex", hex.EncodeToString(payload),
			"payload_len", len(payload),
		)
	}
	p.writeOut(op, payload)
}

// varpTypeConfig returns the VarPlayerType for id, or nil if the
// server hasn't loaded configs or the id is out of range.
func (p *Player) varpTypeConfig(id int) *objtype.VarPlayerType {
	if p.client == nil || p.client.server == nil || p.client.server.varpTypes == nil {
		return nil
	}
	if id < 0 || id >= len(p.client.server.varpTypes.Configs) {
		return nil
	}
	return p.client.server.varpTypes.Configs[id]
}
