package world

import (
	"uuid"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/eventspb"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/telemetry"
)

// updatePlayers runs during processClientsOut. Calls
// s.rsbuf.PlayerInfo.Encode for the local player's PlayerInfo
// payload, writes the result as an OpPlayerInfo packet.
func (p *Player) updatePlayers() {
	s := p.client.server
	if s == nil || s.rsbuf == nil || s.renderer == nil {
		return
	}
	payload := s.rsbuf.PlayerInfo.Encode(s.rsbuf, int32(p.pid), s.renderer)
	p.writeOut(gameserver.OpPlayerInfo, payload)

	telemetry.Get().EmitWorld(&eventspb.WorldEnvelope{
		SchemaVersion: 1,
		EventId:       uuid.New().String(),
		Ts:            timestamppb.Now(),
		WorldId:       int32(s.cfg.NodeID),
		AccountId:     p.accountID,
		Payload: &eventspb.WorldEnvelope_TilePosition{
			TilePosition: &eventspb.TilePositionEvent{
				X:     int32(p.x),
				Y:     int32(p.z),
				Plane: int32(p.level),
			},
		},
	})
}
