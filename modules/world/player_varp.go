package world

import (
	"encoding/hex"

	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	applog "github.com/zsrv/goscape/pkg/util/log"
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
		applog.Trace(p.client.server.log, "nai138.write_varp",
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

// varbitTypeConfig returns the VarBitType for id, or nil if the server
// hasn't loaded configs (245.2-era cache without varbit.dat, or test
// fixture) or the id is out of range. Symmetric with varpTypeConfig;
// the registry's Get handles the nil/OOB cases.
func (p *Player) varbitTypeConfig(id int) *objtype.VarBitType {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.varbitTypes.Get(id)
}
