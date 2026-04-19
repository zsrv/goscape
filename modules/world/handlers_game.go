package world

import (
	"strings"

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

	gameHandlers[4] = handleClientCheat // CLIENT_CHEAT
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

func handleClientCheat(p *Player, payload []byte) error {
	r := packet.NewPacket(payload)
	_ = r.G1() // unused ctrlHeld-style byte per TS ClientCheat handler
	raw := r.GJStrLF()
	if !strings.HasPrefix(raw, "::") {
		return nil
	}
	cmd := strings.TrimPrefix(raw, "::")
	parts := strings.SplitN(cmd, " ", 2)
	switch parts[0] {
	case "say":
		if len(parts) == 2 {
			p.Say([]byte(parts[1]))
		}
	}
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
