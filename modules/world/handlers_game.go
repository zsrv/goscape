package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// gameHandlers is indexed by decrypted game opcode. Nil means no handler
// registered for that opcode; handleGame() silently discards such packets
// (they still must be in Ops[] to be accepted at all).
var gameHandlers [256]func(*client, []byte) error

func init() {
	// Keepalive — discard silently
	gameHandlers[108] = handleNoTimeout  // NO_TIMEOUT
	gameHandlers[70] = handleNoTimeout   // IDLE_TIMER

	// Movement
	gameHandlers[181] = handleMoveClick        // MOVE_GAMECLICK
	gameHandlers[93] = handleMoveClick         // MOVE_OPCLICK
	gameHandlers[165] = handleMoveMinimapClick // MOVE_MINIMAPCLICK
}

func handleNoTimeout(_ *client, _ []byte) error {
	return nil
}

// handleMoveClick decodes MOVE_GAMECLICK and MOVE_OPCLICK.
//
// Payload layout (from MoveClickDecoder.ts):
//   - 1 byte:  ctrlHeld
//   - 2 bytes: startX (G2, unsigned)
//   - 2 bytes: startZ (G2, unsigned)
//   - N pairs: signed-byte deltaX + signed-byte deltaZ (up to 24 waypoints)
func handleMoveClick(c *client, payload []byte) error {
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

	c.log.Info("move click", "ctrl_held", ctrlHeld, "path", path)
	return nil
}

// handleMoveMinimapClick decodes MOVE_MINIMAPCLICK.
//
// Same layout as MOVE_GAMECLICK but with 14 trailing bytes (camera/anticheat
// data) that must be excluded from the waypoint count.
func handleMoveMinimapClick(c *client, payload []byte) error {
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

	c.log.Info("minimap click", "ctrl_held", ctrlHeld, "path", path)
	return nil
}
