package world

import (
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// updatePlayers runs during processClientsOut. Calls
// s.rsbuf.PlayerInfo.Encode for the local player's PlayerInfo
// payload, writes the result as an OpPlayerInfo packet.
func (p *Player) updatePlayers() {
	s := p.client.server
	if s == nil || s.rsbuf == nil || s.renderer == nil {
		return
	}
	payload := s.rsbuf.PlayerInfo.Encode(s.rsbuf, int32(p.slot), s.renderer)
	p.writeOut(gameserver.OpPlayerInfo, payload)
}
