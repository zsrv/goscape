package world

import (
	"bytes"
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

	gameHandlers[150] = handleRebuildGetMaps // REBUILD_GETMAPS

	gameHandlers[194] = handleOpNpc1 // OPNPC1
	gameHandlers[8] = handleOpNpc2   // OPNPC2
	gameHandlers[27] = handleOpNpc3  // OPNPC3
	gameHandlers[113] = handleOpNpc4 // OPNPC4
	gameHandlers[100] = handleOpNpc5 // OPNPC5

	gameHandlers[245] = handleOpLoc1 // OPLOC1
	gameHandlers[172] = handleOpLoc2 // OPLOC2
	gameHandlers[96] = handleOpLoc3  // OPLOC3
	gameHandlers[97] = handleOpLoc4  // OPLOC4
	gameHandlers[116] = handleOpLoc5 // OPLOC5
	gameHandlers[9] = handleOpLocT   // OPLOCT
	gameHandlers[75] = handleOpLocU  // OPLOCU

	gameHandlers[134] = handleOpNpcT // OPNPCT
	gameHandlers[202] = handleOpNpcU // OPNPCU

	gameHandlers[158] = handleMessagePublic // MESSAGE_PUBLIC

	gameHandlers[235] = handleResumePauseButton // RESUME_PAUSEBUTTON
	gameHandlers[237] = handleResumeCountDialog // RESUME_P_COUNTDIALOG
}

// handleResumePauseButton is the package-level adapter that wires the
// []byte-payload gameHandlers dispatch into the Server method of the
// same name (see resume_dialog.go). Looks up the Server via
// p.client.server, matching existing handlers in this file.
func handleResumePauseButton(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleResumePauseButton(p, packet.NewPacket(payload))
}

func handleResumeCountDialog(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleResumeCountDialog(p, packet.NewPacket(payload))
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

// handleMessagePublic decodes a MESSAGE_PUBLIC packet (opcode 158) and sets
// MaskChat on the sender so the per-tick encoder propagates chat to tracked
// observers via HighDefWithChatOf (NAI-32 Task 3 swap surface).
//
// Wire format (per Client-Java client.java:2903-2909):
//
//	byte 0: color (0..11)
//	byte 1: effect (0=none, 1=wave, 2=scroll)
//	bytes 2+: word-packed text (raw bytes; server transports unchanged,
//	          receiving Java client decodes via WordPack.unpack).
//
// Rights: passed through from p.staffModLevel so staff/admin chat gets
// the priority bit on the receiving client (var15 > 1 in client.java:10520
// routes to addMessage type 1 vs type 2).
func handleMessagePublic(p *Player, payload []byte) error {
	if len(payload) < 2 {
		return nil
	}
	color := int(payload[0])
	effect := int(payload[1])
	// Copy the message bytes — the underlying packet buffer may be reused.
	msg := bytes.Clone(payload[2:])
	p.Chat(color, effect, int(p.staffModLevel), msg)
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
