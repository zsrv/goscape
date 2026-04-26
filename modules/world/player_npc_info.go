package world

import (
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// updateNpcs runs during processClientsOut. Snapshots all NPCs, feeds to
// rsbuf.EncodeNpcLegacy, writes the result as an OpNpcInfo packet.
func (p *Player) updateNpcs() {
	s := p.client.server
	if s == nil || p.buildArea == nil || s.renderer == nil || s.grid == nil {
		return
	}

	s.playersMu.RLock()
	snapshot := make([]*Npc, len(s.npcLoop))
	copy(snapshot, s.npcLoop)
	s.playersMu.RUnlock()

	sources := make([]rsbuf.NpcSource, len(snapshot))
	for i, n := range snapshot {
		sources[i] = n
	}

	payload := rsbuf.EncodeNpcLegacy(p, sources, p.buildArea, s.grid, s.renderer)
	p.writeOut(gameserver.OpNpcInfo, payload)
}
