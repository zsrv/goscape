package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// gameHandlers is indexed by decrypted game opcode. Nil means no handler
// registered for that opcode; readPacket() silently discards such packets
// (they still must be in Ops[] to be accepted at all).
var gameHandlers [256]func(*Player, []byte) error

func init() {
	gameHandlers[108] = handleNoTimeout  // NO_TIMEOUT
	gameHandlers[70] = handleNoTimeout   // IDLE_TIMER

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
	startX := r.G2()
	startZ := r.G2()

	type point struct{ x, z int }
	path := make([]point, 0, min((len(payload)-5)/2, 24)+1)
	path = append(path, point{int(startX), int(startZ)})
	for range min((len(payload)-5)/2, 24) {
		dx := r.G1B()
		dz := r.G1B()
		path = append(path, point{int(startX) + int(dx), int(startZ) + int(dz)})
	}

	p.client.log.Info("move click", "ctrl_held", ctrlHeld, "path", path)
	return nil
}

func handleMoveMinimapClick(p *Player, payload []byte) error {
	const trailingBytes = 14
	if len(payload) < 5+trailingBytes {
		return nil
	}
	r := packet.NewPacket(payload)
	ctrlHeld := r.G1()
	startX := r.G2()
	startZ := r.G2()

	type point struct{ x, z int }
	path := make([]point, 0, min((len(payload)-5-trailingBytes)/2, 24)+1)
	path = append(path, point{int(startX), int(startZ)})
	for range min((len(payload)-5-trailingBytes)/2, 24) {
		dx := r.G1B()
		dz := r.G1B()
		path = append(path, point{int(startX) + int(dx), int(startZ) + int(dz)})
	}

	p.client.log.Info("minimap click", "ctrl_held", ctrlHeld, "path", path)
	return nil
}
