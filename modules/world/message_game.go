package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/script"
)

// Compile-time check: *Player implements script.ActivePlayer. Breaks the
// build if either side drifts.
var _ script.ActivePlayer = (*Player)(nil)

// MessageGame writes a MESSAGE_GAME packet (opcode 4) with a NUL-terminated
// JagString payload. Used by the MES script opcode.
func (p *Player) MessageGame(msg string) {
	buf := packet.NewPacket(nil)
	buf.PJStrNUL(msg)
	p.writeOut(gameserver.OpMessageGame, buf.Bytes())
}

// Username returns the player's account name. Used by the NAME script opcode.
func (p *Player) Username() string {
	return p.username
}
