package world

import (
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// updateNpcs runs during processClientsOut. Calls
// s.rsbuf.NpcInfo.Encode for the local player's NpcInfo
// payload, writes the result as an OpNpcInfo packet.
func (p *Player) updateNpcs() {
	s := p.client.server
	if s == nil || s.rsbuf == nil || s.renderer == nil {
		return
	}
	payload := s.rsbuf.NpcInfo.Encode(s.rsbuf, int32(p.slot), s.renderer)
	p.writeOut(gameserver.OpNpcInfo, payload)
}
