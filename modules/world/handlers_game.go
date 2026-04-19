package world

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// gameHandlers is indexed by decrypted game opcode. Nil means no handler
// registered for that opcode; readPacket() silently discards such packets
// (they still must be in Ops[] to be accepted at all).
var gameHandlers [256]func(*Player, []byte) error

func init() {
	gameHandlers[108] = handleNoTimeout // NO_TIMEOUT
	gameHandlers[70] = handleNoTimeout  // IDLE_TIMER

	gameHandlers[181] = handleMoveClick        // MOVE_GAMECLICK
	gameHandlers[93] = handleMoveClick         // MOVE_OPCLICK
	gameHandlers[165] = handleMoveMinimapClick // MOVE_MINIMAPCLICK
}

func handleNoTimeout(_ *Player, _ []byte) error {
	return nil
}

func handleMoveClick(p *Player, payload []byte) error {
	if len(payload) < 5 {
		return nil
	}
	r := packet.NewPacket(payload)
	ctrlHeld := r.G1()
	startX := int(r.G2())
	startZ := int(r.G2())

	pathLen := min((len(payload)-5)/2, 24) + 1
	packed := make([]int, 0, pathLen)
	packed = append(packed, coordgrid.PackCoord(p.level, startX, startZ))
	for range min((len(payload)-5)/2, 24) {
		dx := int(r.G1B())
		dz := int(r.G1B())
		packed = append(packed, coordgrid.PackCoord(p.level, startX+dx, startZ+dz))
	}

	p.client.log.Debug("move click", "ctrl_held", ctrlHeld, "dest_packed", packed[0])
	needsFinding := false
	if p.client.server != nil {
		needsFinding = !p.client.server.cfg.NodeClientRoutefinder
	}
	p.pathToMoveClick(packed, needsFinding)
	return nil
}

func handleMoveMinimapClick(p *Player, payload []byte) error {
	const trailingBytes = 14
	if len(payload) < 5+trailingBytes {
		return nil
	}
	r := packet.NewPacket(payload)
	ctrlHeld := r.G1()
	startX := int(r.G2())
	startZ := int(r.G2())

	pathLen := min((len(payload)-5-trailingBytes)/2, 24) + 1
	packed := make([]int, 0, pathLen)
	packed = append(packed, coordgrid.PackCoord(p.level, startX, startZ))
	for range min((len(payload)-5-trailingBytes)/2, 24) {
		dx := int(r.G1B())
		dz := int(r.G1B())
		packed = append(packed, coordgrid.PackCoord(p.level, startX+dx, startZ+dz))
	}

	p.client.log.Debug("minimap click", "ctrl_held", ctrlHeld, "dest_packed", packed[0])
	needsFinding := false
	if p.client.server != nil {
		needsFinding = !p.client.server.cfg.NodeClientRoutefinder
	}
	p.pathToMoveClick(packed, needsFinding)
	return nil
}
