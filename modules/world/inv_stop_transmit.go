package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendUpdateInvStopTransmit tells the client to stop receiving updates for
// the given UI component. Fired when a modal containing the component closes.
// Wire: p2(component).
func sendUpdateInvStopTransmit(p *Player, com int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	p.writeOut(gameserver.OpUpdateInvStopTransmit, buf.Bytes())
}
