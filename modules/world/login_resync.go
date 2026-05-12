package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendUpdatePid writes one UPDATE_PID packet. Mirrors TS
// UpdatePidEncoder (`buf.p2(message.uid)`); TS passes p.slot at
// Player.ts:495 via `new UpdatePid(this.slot)` — slot is the int
// field, not the composed uid. NAI-182.
func sendUpdatePid(p *Player, slot int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(slot))
	p.writeOut(gameserver.OpUpdatePid, buf.Bytes())
}

// sendResetClientVarCache writes one RESET_CLIENT_VARCACHE packet
// (0-byte payload). NAI-182.
func sendResetClientVarCache(p *Player) {
	p.writeOut(gameserver.OpResetClientVarCache, nil)
}

// sendResetAnims writes one RESET_ANIMS packet (0-byte payload). NAI-182.
func sendResetAnims(p *Player) {
	p.writeOut(gameserver.OpResetAnims, nil)
}
