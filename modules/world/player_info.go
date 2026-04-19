package world

import (
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// updatePlayers runs during processClientsOut. Snapshots all players, feeds to
// rsbuf.Encode, and writes the result as an OpPlayerInfo packet.
func (p *Player) updatePlayers() {
	s := p.client.server
	if s == nil || p.buildArea == nil || s.renderer == nil || s.grid == nil {
		return
	}

	s.playersMu.RLock()
	snapshot := make([]*Player, len(s.playerLoop))
	copy(snapshot, s.playerLoop)
	s.playersMu.RUnlock()

	sources := make([]rsbuf.PlayerSource, len(snapshot))
	for i, op := range snapshot {
		sources[i] = op
	}

	payload := rsbuf.Encode(p, sources, p.buildArea, s.grid, s.renderer)
	p.writeOut(gameserver.OpPlayerInfo, payload)
}
