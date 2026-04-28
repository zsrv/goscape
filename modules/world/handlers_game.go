package world

import (
	"bytes"
	"strconv"
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

	gameHandlers[164] = handleOpPlayer1 // OPPLAYER1
	gameHandlers[53] = handleOpPlayer2  // OPPLAYER2
	gameHandlers[185] = handleOpPlayer3 // OPPLAYER3
	gameHandlers[206] = handleOpPlayer4 // OPPLAYER4
	gameHandlers[177] = handleOpPlayerT // OPPLAYERT
	gameHandlers[248] = handleOpPlayerU // OPPLAYERU

	gameHandlers[158] = handleMessagePublic // MESSAGE_PUBLIC

	gameHandlers[235] = handleResumePauseButton // RESUME_PAUSEBUTTON
	gameHandlers[237] = handleResumeCountDialog // RESUME_P_COUNTDIALOG

	gameHandlers[231] = handleCloseModal   // CLOSE_MODAL
	gameHandlers[175] = handleTutClickSide // TUT_CLICKSIDE
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

// handleCloseModal handles client opcode 231 (CLOSE_MODAL). Zero-byte
// payload. Sets requestModalClose so processPlayerQueue closes the modal
// before queue scripts run this tick.
// Mirrors TS CloseModalHandler.ts — the modal is NOT closed directly here.
func handleCloseModal(p *Player, _ []byte) error {
	p.requestModalClose = true
	return nil
}

func handleTutClickSide(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleTutClickSide(p, payload)
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
	// Mirrors TS ClientCheatHandler.ts:40-50:
	//   - Reject if input.length > 80
	//   - Lowercase the entire string
	//   - Split on space; first token is cmd, rest is args
	//   - Reject if cmd is empty
	// The Java client strips the `::` prefix before sending the CLIENT_CHEAT
	// packet, so `raw` never includes it. Routing to opcode 4 IS the prefix
	// check.
	if len(raw) > 80 {
		return nil
	}
	cheat := strings.ToLower(raw)
	parts := strings.SplitN(cheat, " ", 2)
	if parts[0] == "" {
		return nil
	}
	args := ""
	if len(parts) == 2 {
		args = parts[1]
	}
	switch parts[0] {
	case "say":
		if args != "" {
			p.Say([]byte(args))
		}
	case "getcoord":
		// staffModLevel >= 2 gate mirrors TS ClientCheatHandler.ts:483.
		if p.staffModLevel < 2 {
			return nil
		}
		// Mirrors TS ClientCheatHandler.ts:489 — `::getcoord` displays the
		// player's current coord as level,mapX,mapZ,localX,localZ.
		p.MessageGame(coordgrid.FormatString(p.level, p.x, p.z, ","))
	case "tele":
		// staffModLevel >= 2 gate mirrors TS ClientCheatHandler.ts:483.
		if p.staffModLevel < 2 {
			return nil
		}
		// Mirrors TS `::tele level,mapX,mapZ[,localX,localZ]` at
		// ClientCheatHandler.ts:491-523. Single-arg form:
		// "::tele 0,50,50,32,32".
		//
		// DEVIATION: TS pre-tele cleanup calls player.closeModal(),
		// player.canAccess() (with "Please finish what you are doing
		// first." gate), and player.unsetMapFlag() — none of which exist
		// on goscape Player yet. We call ClearInteraction (the one that
		// does exist) and skip the others; tele is a staff-only debug
		// op so the cleanup gap is acceptable for the smoke-test
		// enabler scope. Track in nai_followups.md with the
		// pathing-entity-teleport-parity sub-spec.
		if args == "" {
			return nil
		}
		coord := strings.Split(args, ",")
		if len(coord) < 3 {
			return nil
		}
		level := parseIntOr(coord[0], 0)
		mx := parseIntOr(coord[1], 50)
		mz := parseIntOr(coord[2], 50)
		lx := 32
		if len(coord) > 3 {
			lx = parseIntOr(coord[3], 32)
		}
		lz := 32
		if len(coord) > 4 {
			lz = parseIntOr(coord[4], 32)
		}
		if level < 0 || level > 3 || mx < 0 || mx > 255 || mz < 0 || mz > 255 || lx < 0 || lx > 63 || lz < 0 || lz > 63 {
			return nil
		}
		p.ClearInteraction()
		p.TeleJump((mx<<6)+lx, (mz<<6)+lz, level)
	}
	return nil
}

// parseIntOr parses s as a base-10 int, returning def on any error.
// Mirrors TS tryParseInt used by the cheat handler at ClientCheatHandler.ts.
func parseIntOr(s string, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return v
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
