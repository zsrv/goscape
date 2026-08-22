package world

import (
	"bytes"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
	"github.com/zsrv/goscape/pkg/telemetry"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
)

// gameHandlers is indexed by decrypted game opcode. Nil means no handler
// registered for that opcode; readPacket() silently discards such packets
// (they still must be in Ops[] to be accepted at all).
var gameHandlers [256]func(*Player, []byte) error

func init() {
	gameHandlers[gameclient.OpcNoTimeout] = handleNoTimeout // NO_TIMEOUT
	gameHandlers[gameclient.OpcIdleTimer] = handleIdleTimer // IDLE_TIMER

	gameHandlers[gameclient.OpcMoveGameClick] = handleMoveGameClick       // MOVE_GAMECLICK (opClick=false)
	gameHandlers[gameclient.OpcMoveOpClick] = handleMoveOpClick           // MOVE_OPCLICK (opClick=true)
	gameHandlers[gameclient.OpcMoveMinimapClick] = handleMoveMinimapClick // MOVE_MINIMAPCLICK

	gameHandlers[gameclient.OpcClientCheat] = handleClientCheat // CLIENT_CHEAT

	gameHandlers[gameclient.OpcOpNpc1] = handleOpNpc1 // OPNPC1
	gameHandlers[gameclient.OpcOpNpc2] = handleOpNpc2 // OPNPC2
	gameHandlers[gameclient.OpcOpNpc3] = handleOpNpc3 // OPNPC3
	gameHandlers[gameclient.OpcOpNpc4] = handleOpNpc4 // OPNPC4
	gameHandlers[gameclient.OpcOpNpc5] = handleOpNpc5 // OPNPC5

	gameHandlers[gameclient.OpcOpLoc1] = handleOpLoc1 // OPLOC1
	gameHandlers[gameclient.OpcOpLoc2] = handleOpLoc2 // OPLOC2
	gameHandlers[gameclient.OpcOpLoc3] = handleOpLoc3 // OPLOC3
	gameHandlers[gameclient.OpcOpLoc4] = handleOpLoc4 // OPLOC4
	gameHandlers[gameclient.OpcOpLoc5] = handleOpLoc5 // OPLOC5
	gameHandlers[gameclient.OpcOpLocT] = handleOpLocT // OPLOCT
	gameHandlers[gameclient.OpcOpLocU] = handleOpLocU // OPLOCU

	gameHandlers[gameclient.OpcOpNpcT] = handleOpNpcT // OPNPCT
	gameHandlers[gameclient.OpcOpNpcU] = handleOpNpcU // OPNPCU

	gameHandlers[gameclient.OpcOpPlayer1] = handleOpPlayer1 // OPPLAYER1
	gameHandlers[gameclient.OpcOpPlayer2] = handleOpPlayer2 // OPPLAYER2
	gameHandlers[gameclient.OpcOpPlayer3] = handleOpPlayer3 // OPPLAYER3
	gameHandlers[gameclient.OpcOpPlayer4] = handleOpPlayer4 // OPPLAYER4
	gameHandlers[gameclient.OpcOpPlayer5] = handleOpPlayer5 // OPPLAYER5 (254)
	gameHandlers[gameclient.OpcOpPlayerT] = handleOpPlayerT // OPPLAYERT
	gameHandlers[gameclient.OpcOpPlayerU] = handleOpPlayerU // OPPLAYERU

	gameHandlers[gameclient.OpcOpObj1] = handleOpObj1 // OPOBJ1
	gameHandlers[gameclient.OpcOpObj2] = handleOpObj2 // OPOBJ2
	gameHandlers[gameclient.OpcOpObj3] = handleOpObj3 // OPOBJ3
	gameHandlers[gameclient.OpcOpObj4] = handleOpObj4 // OPOBJ4
	gameHandlers[gameclient.OpcOpObj5] = handleOpObj5 // OPOBJ5
	gameHandlers[gameclient.OpcOpObjT] = handleOpObjT // OPOBJT
	gameHandlers[gameclient.OpcOpObjU] = handleOpObjU // OPOBJU

	gameHandlers[gameclient.OpcOpHeld1] = handleOpHeld1 // OPHELD1
	gameHandlers[gameclient.OpcOpHeld2] = handleOpHeld2 // OPHELD2
	gameHandlers[gameclient.OpcOpHeld3] = handleOpHeld3 // OPHELD3
	gameHandlers[gameclient.OpcOpHeld4] = handleOpHeld4 // OPHELD4
	gameHandlers[gameclient.OpcOpHeld5] = handleOpHeld5 // OPHELD5
	gameHandlers[gameclient.OpcOpHeldT] = handleOpHeldT // OPHELDT
	gameHandlers[gameclient.OpcOpHeldU] = handleOpHeldU // OPHELDU

	gameHandlers[gameclient.OpcChatSetmode] = handleChatSetMode     // CHAT_SETMODE
	gameHandlers[gameclient.OpcFriendlistAdd] = handleFriendListAdd // FRIENDLIST_ADD
	gameHandlers[gameclient.OpcFriendlistDel] = handleFriendListDel // FRIENDLIST_DEL
	gameHandlers[gameclient.OpcIgnorelistAdd] = handleIgnoreListAdd // IGNORELIST_ADD
	gameHandlers[gameclient.OpcIgnorelistDel] = handleIgnoreListDel // IGNORELIST_DEL
	gameHandlers[gameclient.OpcReportAbuse] = handleReportAbuse     // REPORT_ABUSE

	// Client event packets (introduced at 254 to replace the EVENT_TRACKING
	// upload; at 274 the EVENT_TRACKING row itself was DELETED from the prot
	// table — TS ClientGameProt.ts @dee467c8 — so its old opcode is now a
	// protocol error like any other unknown opcode).
	gameHandlers[gameclient.OpcEventMouseClick] = handleEventMouseClick         // EVENT_MOUSE_CLICK
	gameHandlers[gameclient.OpcEventMouseMove] = handleEventMouseMove           // EVENT_MOUSE_MOVE
	gameHandlers[gameclient.OpcEventAppletFocus] = handleEventAppletFocus       // EVENT_APPLET_FOCUS
	gameHandlers[gameclient.OpcEventCameraPosition] = handleEventCameraPosition // EVENT_CAMERA_POSITION

	gameHandlers[gameclient.OpcMessagePrivate] = handleMessagePrivate // MESSAGE_PRIVATE
	gameHandlers[gameclient.OpcMessagePublic] = handleMessagePublic   // MESSAGE_PUBLIC

	gameHandlers[gameclient.OpcResumePauseButton] = handleResumePauseButton  // RESUME_PAUSEBUTTON
	gameHandlers[gameclient.OpcResumePCountdialog] = handleResumeCountDialog // RESUME_P_COUNTDIALOG

	gameHandlers[gameclient.OpcCloseModal] = handleCloseModal               // CLOSE_MODAL
	gameHandlers[gameclient.OpcTutorialClickSide] = handleTutorialClickSide // TUTORIAL_CLICKSIDE

	gameHandlers[gameclient.OpcIfButton] = handleIfButton                 // IF_BUTTON
	gameHandlers[gameclient.OpcInvButton1] = invButtonHandler(1)          // INV_BUTTON1
	gameHandlers[gameclient.OpcInvButton2] = invButtonHandler(2)          // INV_BUTTON2
	gameHandlers[gameclient.OpcInvButton3] = invButtonHandler(3)          // INV_BUTTON3
	gameHandlers[gameclient.OpcInvButton4] = invButtonHandler(4)          // INV_BUTTON4
	gameHandlers[gameclient.OpcInvButton5] = invButtonHandler(5)          // INV_BUTTON5
	gameHandlers[gameclient.OpcInvButtonD] = handleInvButtonD             // INV_BUTTOND
	gameHandlers[gameclient.OpcIfPlayerDesign] = handleIfPlayerDesignGame // IF_PLAYERDESIGN
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

// handleCloseModal handles client opcode 187 (CLOSE_MODAL). Zero-byte
// payload. Sets requestModalClose so processPlayerQueue closes the modal
// before queue scripts run this tick.
// Mirrors TS CloseModalHandler.ts — the modal is NOT closed directly here.
func handleCloseModal(p *Player, _ []byte) error {
	p.requestModalClose = true
	return nil
}

func handleTutorialClickSide(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleTutorialClickSide(p, payload)
}

// handleIfButton is the package-level adapter that wires the []byte-payload
// gameHandlers dispatch into the Server method of the same name
// (see handler_interface.go). Looks up the Server via p.client.server.
func handleIfButton(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleIfButton(p, payload)
}

// invButtonHandler returns the gameHandlers adapter for INV_BUTTON<n>
// (n=1..5). Looks up the Server via p.client.server, matching handleIfButton
// and the other Server-method adapters in this file. Replaces five
// byte-identical handleInvButton1..5 adapters that differed only by the
// trailing int literal passed to (*Server).handleInvButton.
func invButtonHandler(n int) func(*Player, []byte) error {
	return func(p *Player, payload []byte) error {
		if p.client == nil || p.client.server == nil {
			return nil
		}
		return p.client.server.handleInvButton(p, payload, n)
	}
}

func handleInvButtonD(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleInvButtonD(p, payload)
}

func handleIfPlayerDesignGame(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleIfPlayerDesign(p, payload)
}

func handleNoTimeout(_ *Player, _ []byte) error {
	return nil
}

// handleIdleTimer is IDLE_TIMER (opcode 146). The Java client sends it after
// 4500 idle input-cycles; TS IdleTimerHandler sets requestIdleLogout=true so
// the next processLogouts pass tears the player down (unless NODE_DEBUG, which
// keeps developers logged in). Mirrors Engine-TS IdleTimerHandler.ts:8-12.
//
// M23: previously routed to handleNoTimeout, so idle clients were never logged
// out via their explicit idle signal. requestIdleLogout is consumed by
// processLogouts (tick.go) exactly as the InputTracking kick branch sets it.
func handleIdleTimer(p *Player, _ []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	if !p.client.server.cfg.NodeDebug {
		p.requestIdleLogout = true
	}
	return nil
}

// handleMoveGameClick is the dispatch entry for MOVE_GAMECLICK (opcode 63).
func handleMoveGameClick(p *Player, payload []byte) error {
	return moveClickInner(p, payload, false, 0)
}

// handleMoveOpClick is the dispatch entry for MOVE_OPCLICK (opcode 167).
func handleMoveOpClick(p *Player, payload []byte) error {
	return moveClickInner(p, payload, true, 0)
}

// moveClickInner is the shared move-click implementation for all three
// move opcodes — MOVE_GAMECLICK (63), MOVE_OPCLICK (167), and
// MOVE_MINIMAPCLICK (56) — mirroring TS, which binds all three to a
// single MoveClickHandler (ClientGameProtRepository.ts:125-127).
// Mirrors TS MoveClickHandler.ts:11-57 at the rev-254 pin (rewritten by
// f0ccbe8a).
//
// Wire payload (per TS MoveClickDecoder.ts):
//
//	byte 0:    ctrlHeld (G1, expected 0 or 1)
//	bytes 1-2: startX (G2)
//	bytes 3-4: startZ (G2)
//	bytes 5+:  up to 24 waypoints, each 2 bytes (dx:G1B, dz:G1B)
//	[trailer]: trailingBytes content-irrelevant bytes (14 for minimap
//	           click only — the camera/compass trailer; 0 otherwise).
//	           Mirrors TS MoveClickDecoder.ts:16 offset.
//
// Flow per the pinned handler:
//  1. p.delayed → write UnsetMapFlag (no waypoint clear), no-op (TS L13-16).
//  2. ctrlHeld out of [0,1] OR DistanceToSW(player, start) > 104 →
//     unsetMapFlag (clearWaypoints + write) + clear userPath, no-op
//     (TS L20-25).
//  3. ClearPendingAction — gated on !opClick (TS L26-32 @4c95f87e): a
//     MOVE_OPCLICK is always paired with a following op packet that
//     clears+sets the interaction itself, so clearing here would drop the
//     target in the gap when the per-tick user packet limit splits the pair
//     across ticks.
//  4. tempRun = ctrlHeld; override to 0 if runenergy<100 && ctrlHeld==1
//     (TS L30-35).
//  5. Routefinder on (TS L36-50 @dee467c8): rebuild userPath from the client
//     path and queueWaypoints directly — including a single-tile own-tile
//     click (upstream #100 deleted the click-own-tile AllowRepath.NONE arm,
//     the "I can't reach that!" fix). processWalktrigger fires
//     unconditionally after.
//     Routefinder off (TS L51-54): server-side findPath to the last coord.
//     userPath is NOT touched on this branch (TS-faithful).
func moveClickInner(p *Player, payload []byte, opClick bool, trailingBytes int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 5+trailingBytes {
		return nil
	}

	r := packet.NewPacket(payload)
	ctrlHeld := int(r.G1())
	startX := int(r.G2())
	startZ := int(r.G2())

	// Validate input (TS L20-25).
	if ctrlHeld < 0 || ctrlHeld > 1 || coordgrid.DistanceToSW(p.x, p.z, startX, startZ) > 104 {
		sendUnsetMapFlag(p)
		p.waypointIndex = -1 // TS Player.unsetMapFlag → clearWaypoints
		p.userPath = nil
		return nil
	}

	waypointCount := min((len(payload)-5-trailingBytes)/2, 24)
	packed := make([]int, 0, waypointCount+1)
	packed = append(packed, coordgrid.PackCoord(p.level, startX, startZ))
	for range waypointCount {
		ddx := int(r.G1B())
		ddz := int(r.G1B())
		packed = append(packed, coordgrid.PackCoord(p.level, startX+ddx, startZ+ddz))
	}

	p.client.log.Debug("move click", "ctrl_held", ctrlHeld, "dest_packed", packed[len(packed)-1], "op_click", opClick)

	// Clear previous interaction — but not for op-click moves (TS
	// MoveClickHandler.ts:26-32 @4c95f87e, upstream #103): a MOVE_OPCLICK is
	// always paired with a following op packet that clears+sets the
	// interaction itself; clearing here would drop the target in the gap when
	// the per-tick user packet limit splits the pair across ticks.
	if !opClick {
		p.ClearPendingAction()
	}

	// Handle ctrl run (TS L30-35).
	if p.runenergy < 100 && ctrlHeld == 1 {
		p.tempRun = 0
	} else {
		p.tempRun = ctrlHeld
	}

	// Set new path (TS MoveClickHandler.ts:36-54 @dee467c8).
	if s.cfg.NodeClientRoutefinder {
		// Upstream #100 deleted the click-own-tile special case (it set
		// AllowRepath.NONE and froze the player → "I can't reach that!").
		// The client path is now rebuilt into userPath + queued
		// unconditionally, including a single-tile own-tile click.
		p.userPath = append(p.userPath[:0], packed...)
		p.queueWaypoints(p.userPath)
		// TS L50 — fires regardless of opClick or hasWaypoints.
		p.processWalktrigger()
	} else {
		// TS L51-54: server-side pathfind to the clicked destination.
		// userPath deliberately untouched (TS-faithful).
		dest := coordgrid.UnpackCoord(packed[len(packed)-1])
		pf := s.pathfinder()
		if pf == nil {
			// (goscape defensive; TS skips this check)
			p.queueWaypoint(dest.X, dest.Z)
			return nil
		}
		route := pf.FindPathPlain(p.level, p.x, p.z, dest.X, dest.Z)
		p.queueWaypoints(routeToPacked(route))
	}

	return nil
}

// handleMessagePublic decodes a MESSAGE_PUBLIC packet (opcode 171) and sets
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
	rawPacked := payload[2:]

	// TS MessagePublicHandler.ts:14 — gate on socialProtect, color (0..11),
	// effect (0..2), and packed-input length (≤100). Reject silently on any.
	if p.socialProtect || color < 0 || color > 11 || effect < 0 || effect > 2 || len(rawPacked) > 100 {
		return nil
	}

	// TS MessagePublicHandler.ts:18-21 — drop chat while muted.
	if !p.mutedUntil.IsZero() && p.mutedUntil.After(time.Now()) {
		return nil
	}

	// Unpack raw word-packed text first — needed for both wordenc filtering
	// (TS MessagePublicHandler.ts:26) and audit-log (TS line 32).
	pk := packet.NewPacket(bytes.Clone(rawPacked))
	decoded := wordpack.Unpack(pk, len(rawPacked))

	// Apply WordEnc.filter and repack for the wire — mirrors TS lines 34-39.
	// NewServer always populates s.wordenc; newTestServer injects encfilter.Empty().
	// Tests that construct a Player directly must wire p.client.server = newTestServer(t).
	filtered := p.client.server.wordenc.Filter(decoded)
	out := packet.NewPacket(nil)
	wordpack.Pack(out, filtered)

	// TS-DRIFT-1: clamp rights to min(staffModLevel, 2) per
	// MessagePublicHandler.ts:31 — Rev 244 mod-crown visibility caps the
	// rendered crown level at 2 (any staff level >2 still displays as 2).
	rights := int(p.staffModLevel)
	if rights > 2 {
		rights = 2
	}
	p.Chat(color, effect, rights, out.Bytes())

	// Chat is Kafka-only (documented TS divergence, spec
	// docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md). TS
	// stores player.logMessage (UNFILTERED, MessagePublicHandler.ts:32,
	// BEFORE filter), drains it once per tick (World.ts:630 @2e3bcf43)
	// and persists {session_uuid, coord, message} to public_chat via the
	// friend server (World.ts:1567-1574). goscape emits one
	// PublicChatEvent inline instead — same tuple, same
	// no-session-gate posture ('headless' sessions emit too,
	// World.ts:629-631); the public_chat table and the friends-server
	// audit-log RPC are retired.
	{
		s := p.client.server
		coord := coordgrid.PackCoord(p.level, p.x, p.z)
		telemetry.Get().EmitWorld(&eventspb.WorldEnvelope{
			SchemaVersion: 1,
			EventId:       uuid.NewString(),
			Ts:            timestamppb.Now(),
			WorldId:       int32(s.cfg.NodeID),
			AccountId:     p.accountID,
			Payload: &eventspb.WorldEnvelope_PublicChat{
				PublicChat: &eventspb.PublicChatEvent{
					Text:        decoded,
					SessionUuid: p.sessionOrHeadless(),
					Coord:       int32(coord),
				},
			},
		})
	}

	// TS MessagePublicHandler.ts:42 — set socialProtect after a successful
	// emit so further chat in the same tick is gated. Reset in processCleanup.
	p.socialProtect = true
	return nil
}

// parseDebugprocCoord mirrors TS ClientCheatHandler.ts:113-124. Returns
// the packed coord; level coerces to -1 if the slice(6) substring fails
// to parse (the common case — see DEVIATION-NAI-189-D1).
func parseDebugprocCoord(rawCheat string) int {
	parts := strings.Split(rawCheat, "_")
	if len(parts) < 5 {
		return -1
	}

	atoiOr := func(s string, def int) int {
		v, err := strconv.Atoi(s)
		if err != nil {
			return def
		}
		return v
	}

	levelStr := ""
	if len(parts[0]) >= 6 {
		levelStr = parts[0][6:]
	}
	level := atoiOr(levelStr, -1)
	mx := atoiOr(parts[1], 0)
	mz := atoiOr(parts[2], 0)
	lx := atoiOr(parts[3], 0)
	lz := atoiOr(parts[4], 0)
	return coordgrid.PackCoord(level, (mx<<6)+lx, (mz<<6)+lz)
}

// marshalDebugprocArgs walks sf.ParamTypes byte-by-byte, casts each to
// objtype.ScriptVarType, and appends to intArgs or stringArgs per the
// 12 TS arms in ClientCheatHandler.ts:69-140. Missing tokens degrade
// per-TS-arm:
//   - STRING → "" (TS `?? ”`)
//   - INT → 0 (TS `parseInt(v ?? '0', 10) | 0`)
//   - ByName-lookup arms → -1 (TS `getId(”)` returns -1)
//   - STAT → -1 (TS `PlayerStatMap.get(undefined)` returns undefined)
//
// The COORD arm re-parses rawCheat (TS L113-124); mirrored verbatim
// in parseDebugprocCoord (see DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE,
// landing in T7).
//
// NAI-189.
func (s *Server) marshalDebugprocArgs(sf *script.ScriptFile, args string, rawCheat string) ([]int, []string) {
	tokens := strings.Fields(args)
	take := func() string {
		if len(tokens) == 0 {
			return ""
		}
		t := tokens[0]
		tokens = tokens[1:]
		return t
	}

	intArgs := make([]int, 0, len(sf.ParamTypes))
	stringArgs := make([]string, 0, len(sf.ParamTypes))

	for _, pt := range sf.ParamTypes {
		switch objtype.ScriptVarType(pt) {
		case objtype.ScriptVarTypeString:
			stringArgs = append(stringArgs, take())
		case objtype.ScriptVarTypeInt:
			intArgs = append(intArgs, parseIntOr(take(), 0))
		case objtype.ScriptVarTypeObj, objtype.ScriptVarTypeNamedObj:
			if t := s.objTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, t.ID)
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeNPC:
			if t := s.npcTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, t.ID)
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeLoc:
			if t := s.locTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, t.ID)
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeSeq:
			if t := s.seqTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, t.ID)
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeStat:
			tok := strings.ToUpper(take())
			if stat, ok := objtype.PlayerStatMap[tok]; ok {
				intArgs = append(intArgs, stat)
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeInv:
			if t := s.invTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, t.ID)
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeCoord:
			// DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE
			// TS L113-124 re-parses the full lowered cheat string by
			// underscore and computes level via args2[0].slice(6). For all
			// reasonable debugproc names this produces a non-digit string
			// → TS NaN / goscape -1 sentinel. Mirrored verbatim per the
			// true-to-TS gate; the level component is effectively always -1
			// while x/z parse correctly from (mx<<6)+lx and (mz<<6)+lz.
			// A future upstream fix should derive the offset from cmd
			// length; until then this matches TS observable behavior.
			intArgs = append(intArgs, parseDebugprocCoord(rawCheat))
		case objtype.ScriptVarTypeInterface:
			if t := s.componentTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, t.ID)
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeSpotanim:
			if t := s.spotanimTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, t.ID)
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeIdkit:
			if t := s.idkTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, t.ID)
			} else {
				intArgs = append(intArgs, -1)
			}
		default:
			// TS has no default; any unrecognised type leaves the slot at -1.
			intArgs = append(intArgs, -1)
		}
	}

	return intArgs, stringArgs
}

// dispatchDebugproc resolves a [debugproc,X] script by name and dispatches
// it via s.runScript with arguments marshaled per the script's ParamTypes.
// Mirrors TS ClientCheatHandler.ts:59-148.
//
// cmd is the lowered first token of the cheat (already verified to start
// with s.cfg.NodeDebugprocChar). args is the post-first-space tail.
// rawCheat is the full lowered cheat string (needed by the COORD arm).
//
// TS-fidelity:
//   - Unknown script name → silent return (TS L62-64 `return false`).
//   - ByName misses → -1 in slot; dispatch continues (TS L74-139 swallow misses).
//
// NAI-189.
func (s *Server) dispatchDebugproc(p *Player, cmd string, args string, rawCheat string) {
	prefix := s.cfg.NodeDebugprocChar
	if prefix == "" || len(cmd) <= len(prefix) || !strings.HasPrefix(cmd, prefix) {
		return
	}
	if s.scriptProvider == nil {
		return
	}
	name := cmd[len(prefix):]
	sf := s.scriptProvider.GetByName("[debugproc," + name + "]")
	if sf == nil {
		return
	}
	intArgs, stringArgs := s.marshalDebugprocArgs(sf, args, rawCheat)
	s.runScript(sf, p, nil, script.TriggerDebugProc, false, intArgs, stringArgs)
}

func handleClientCheat(p *Player, payload []byte) error {
	r := packet.NewPacket(payload)
	// TS ClientCheatDecoder.decode reads ONLY gjstr() — there is no leading
	// control byte. The Java client sends p1isaac(4), p1(len) [the var-size
	// length prefix consumed by the framing layer], then pjstr(substring(2)).
	// So the payload here is purely the cheat string; reading a G1() first
	// would eat the command's first character.
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
	// NAI-189-D1 / NAI-REBUILD-ASYNC: the ::rebuild cheat dispatches
	// asynchronously via rebuildWorker; see rebuild_worker.go and the
	// case "rebuild": arm below. Spec
	// docs/superpowers/specs/2026-05-18-rebuild-async-fsnotify-design.md.
	// NAI-190 retired ::reload (TS L149-150). World.reload() is
	// ported as (*Server).Reload(clearInvs bool) error in
	// modules/world/reload.go. Three DEVIATION tags live in the
	// method doc-comment:
	//   D1-GAMEMAP-RE-INJECT — glue-only: TS reads package
	//     singletons, goscape re-injects loc/obj types into the
	//     GameMap struct via SetLocTypes/SetObjTypes.
	//   D2-HALF-SWAP — TS-parity: TS does not roll back on
	//     mid-pipeline errors. Post-step-3 errors leave s.*
	//     partially mutated. Pinned via skip-with-pin in
	//     reload_test.go.
	//   D3-CANDIDATE-VARSHARED-CLOBBER — TS L259-267 clobbers
	//     copied values; mirrored verbatim per true-to-TS gate.
	//   D4-NO-CATEGORYTYPES — goscape has no CategoryType loader;
	//     TS L216 has no analog.
	// NAI-189 retired DEBUGPROC dispatch (TS L59-148). See
	// dispatchDebugproc/marshalDebugprocArgs/parseDebugprocCoord
	// (DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE).
	// NAI-188 retired ::speed (TS L154-167). The tickRate
	// package-level const at modules/world/tick.go:15 was promoted
	// to Server.tickRate.
	// NAI-187 retired the admin spawn/interface cluster (locadd /
	// npcadd / openmain).
	// TS ClientCheatHandler.ts:52-54 — addSessionLog tier. Logs every
	// cheat invocation from staffModLevel >= 2 to the MODERATOR session
	// log channel. Ported via Player.AddSessionLog (modules/world/player.go).
	// Join semantics: "Ran cheat" + " " + cheat. NAI-183.
	if p.staffModLevel >= 2 {
		p.AddSessionLog(LoggerEventTypeModerator, "Ran cheat", cheat)
	}

	// TS ClientCheatHandler.ts:56 — developer block. Gated on
	// `!Environment.NODE_PRODUCTION && staffModLevel >= 4`. Goscape
	// reads s.cfg.NodeProduction (modules/world/config.go:43, default
	// false). NAI-183.
	if !p.client.server.cfg.NodeProduction && p.staffModLevel >= 4 {
		// TS ClientCheatHandler.ts:59 — debugproc prefix dispatch BEFORE
		// the fixed-cmd ladder. Cmd-form is `<NodeDebugprocChar><scriptname>`
		// (default "~scriptname"). NAI-189.
		if prefix := p.client.server.cfg.NodeDebugprocChar; prefix != "" && strings.HasPrefix(parts[0], prefix) {
			p.client.server.dispatchDebugproc(p, parts[0], args, cheat)
			return nil
		}
		switch parts[0] {
		case "fly":
			// TS L168-175 — toggles between Fly and Smart strategies and
			// emits a MessageGame describing the current state.
			if p.moveStrategy == MoveStrategyFly {
				p.moveStrategy = MoveStrategySmart
			} else {
				p.moveStrategy = MoveStrategyFly
			}
			if p.moveStrategy == MoveStrategyFly {
				p.MessageGame("Changed move strategy: fly")
			} else {
				p.MessageGame("Changed move strategy: smart")
			}
		case "naive":
			// TS L176-183 — toggles between Naive and Smart.
			if p.moveStrategy == MoveStrategyNaive {
				p.moveStrategy = MoveStrategySmart
			} else {
				p.moveStrategy = MoveStrategyNaive
			}
			if p.moveStrategy == MoveStrategyNaive {
				p.MessageGame("Naive move strategy: naive")
			} else {
				p.MessageGame("Naive move strategy: smart")
			}
		case "random":
			// TS L184-186 — primes the AFK event for the next tick.
			p.afkEventReady = true
		case "speed":
			// TS ClientCheatHandler.ts:154-167. NAI-188.
			// Args layout: single positional integer (ms). Branches:
			//   empty args  → "Usage: ::speed <ms>"; no state change.
			//   parsed < 20 → "::speed input was too low."; no state change.
			//   else        → "World speed was changed to {ms}ms"; mutate s.tickRate.
			// Non-numeric arg: parseIntOr defaults to 20 → success at 20ms
			// (mirrors TS tryParseInt fallback). Per spec §6, no lock — this
			// runs on the tick goroutine, same as the loop that reads s.tickRate.
			if args == "" {
				p.MessageGame("Usage: ::speed <ms>")
				return nil
			}
			// args.shift() in TS takes the first whitespace-delimited token;
			// goscape's `args` is the post-first-space tail. Slice the first
			// whitespace token to match.
			first := args
			if i := strings.IndexAny(args, " \t"); i >= 0 {
				first = args[:i]
			}
			speed := parseIntOr(first, 20)
			if speed < 20 {
				p.MessageGame("::speed input was too low.")
				return nil
			}
			p.MessageGame(fmt.Sprintf("World speed was changed to %dms", speed))
			p.client.server.tickRate = time.Duration(speed) * time.Millisecond
		case "reload":
			// TS ClientCheatHandler.ts:149-150 — World.reload() default
			// clearInvs=true. NAI-190.
			if err := p.client.server.Reload(true); err != nil {
				// TS dispatches via try/catch on uncaught throws; goscape
				// surfaces explicitly. DEVIATION-NAI-190-D2-HALF-SWAP
				// documents the half-swap risk on post-step-3 errors.
				p.client.server.log.Error("reload cheat failed", "err", err)
				p.MessageGame("Reload failed: see server log.")
			}
			return nil
		case "rebuild":
			// NAI-REBUILD-ASYNC (spec 2026-05-18-rebuild-async-fsnotify).
			// Async dispatch via rebuildWorker. Mirrors TS
			// ClientCheatHandler.ts:151-153 → World.rebuild() →
			// DevThread.processChangedFiles. Handler returns
			// immediately; result is drained at top-of-tick.
			if p.client.server.cfg.ContentPath == "" {
				p.MessageGame("Rebuild failed: --world.content-path is not configured.")
				return nil
			}
			p.client.server.rebuildMu.Lock()
			if p.client.server.rebuildManualInvoker == nil {
				p.client.server.rebuildManualInvoker = p
			}
			p.client.server.rebuildMu.Unlock()
			// Staff-only broadcast is the deliberate Spec §4.5 scope
			// (gap-world-reload-events-2 AND gap-world-reload-events-3
			// both EXCEPTION-DOCUMENTED — see the CONFIRMED EXCEPTION
			// block on broadcastRebuildStaff in rebuild_worker.go for
			// rationale). TS's "Rebuilding scripts..." (and the per-tick
			// "Packing…" / "Reloading…" updates that goscape's
			// synchronous packFn cannot reproduce) would reach every
			// connected player via broadcastMes (World.ts:1758).
			p.client.server.broadcastRebuildStaff(p, "Rebuilding scripts...")
			p.client.server.dispatchRebuildRequest()
			return nil
		}
	}

	// TS ClientCheatHandler.ts:189 — admin block. Gated on
	// staffModLevel >= 3. NAI-184 T2 added this outer guard and
	// relocated reboot/slowreboot/serverdrop here from the dev block
	// (NAI-183 misclassified them — see spec §2.1).
	if p.staffModLevel >= 3 {
		switch parts[0] {
		case "reboot":
			// TS L360-364. Production-only via inner && NodeProduction;
			// under default NodeProduction=false, this arm is dead.
			if p.client.server.cfg.NodeProduction {
				p.client.server.rebootTimer(0)
			}
		case "slowreboot":
			// TS L365-373. Production-only via the outer `&& NodeProduction`
			// arm selector; goscape mirrors with an inner `if NodeProduction`.
			// TS L367-371 rejects with `return false` when args is empty —
			// mirrored here as `args == "" → return nil` (no default 30s).
			// Formula: ticks = ceil(seconds * 1000/600).
			if p.client.server.cfg.NodeProduction {
				if args == "" {
					return nil
				}
				seconds := parseIntOr(args, 30)
				ticks := int(math.Ceil(float64(seconds) * 1000.0 / 600.0))
				p.client.server.rebootTimer(ticks)
			}
		case "serverdrop":
			// TS L374-376 player.terminate(). No NP gate — fires at >=3
			// regardless of NodeProduction. Closes the TCP conn without
			// removing the player from s.players; the next reconnect hits
			// this player's slot and runs the onReconnect path.
			if p.client != nil && p.client.conn != nil {
				p.client.closeConn()
			}
		case "setstat":
			// TS L401-414 — setstat <skill> <level> via PlayerStatMap.
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			if len(sub) < 2 {
				return nil
			}
			stat, ok := objtype.PlayerStatMap[strings.ToUpper(sub[0])]
			if !ok {
				return nil
			}
			level := parseIntOr(sub[1], 0)
			p.SetStat(stat, level)
		case "advancestat":
			// TS L415-431 — zero stats/baseLevels/levels then AddXP to
			// reach `level`. AddXP fires [changestat,X] and [advancestat,X]
			// triggers on level-up.
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			stat, ok := objtype.PlayerStatMap[strings.ToUpper(sub[0])]
			if !ok {
				return nil
			}
			levelStr := ""
			if len(sub) > 1 {
				levelStr = sub[1]
			}
			level := parseIntOr(levelStr, 0)
			p.stats[stat] = 0
			p.baseLevels[stat] = 1
			p.levels[stat] = 1
			// allowMulti=false — TS ClientCheatHandler.ts:431 grants the exact
			// XP for the target level (getExpByLevel), so node_xp_rate must NOT
			// scale it (M7).
			p.AddXP(stat, objtype.GetExpByLevel(level), false)
		case "give":
			// TS L288-302 — give <obj> [count]. Count clamps to
			// [1, 0x7fffffff] (default 1). Routes through Player.InvAdd
			// (the bare entity helper) with assureFullInsertion=false.
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			objType := p.client.server.objTypes.ByName(sub[0])
			if objType == nil {
				return nil
			}
			count := clampCount(sub, 1)
			p.InvAdd(p.client.server.invTypes.Inv, objType.ID, count)
		case "givemany":
			// TS L339-352 — givemany <obj>. Fixed count = 1000.
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			objType := p.client.server.objTypes.ByName(sub[0])
			if objType == nil {
				return nil
			}
			p.InvAdd(p.client.server.invTypes.Inv, objType.ID, 1000)
		case "minme":
			// TS L432-440 — set every stat to 1 except HITPOINTS=10.
			// TS iterates indices [0, PlayerStatEnabled.length); it does
			// NOT filter on PlayerStatEnabled value. STAT18/STAT19 are
			// reserved/unused so the call has no in-game effect, but
			// TS-fidelity requires the SetStat invocation.
			for i := range objtype.PlayerStatCount {
				if i == objtype.PlayerStatHitpoints {
					p.SetStat(i, 10)
				} else {
					p.SetStat(i, 1)
				}
			}
		case "locadd":
			// TS L441-452 — admin spawn. Resolves LocType by debugname,
			// spawns a CENTREPIECE_STRAIGHT loc with angle=WEST=0,
			// duration=500 ticks. Mirrors TS:
			//   World.addLoc(new Loc(player.level, player.x, player.z,
			//                        type.width, type.length,
			//                        EntityLifeCycle.DESPAWN, type.id,
			//                        LocShape.CENTREPIECE_STRAIGHT,
			//                        LocAngle.WEST), 500);
			if args == "" {
				return nil
			}
			name := strings.Fields(args)[0]
			lt := p.client.server.locTypes.ByName(name)
			if lt == nil {
				return nil
			}
			l := entitypkg.NewLoc(
				p.level, p.x, p.z,
				lt.Width, lt.Length,
				entitypkg.LifecycleDespawn,
				lt.ID,
				int(loc.ShapeCentrepieceStraight),
				0, // LocAngle.WEST
			)
			p.client.server.AddLoc(l, 500)
			p.MessageGame(fmt.Sprintf("Loc Added: %s (ID: %d)", name, lt.ID))
		case "npcadd":
			// TS L453-463 — admin spawn. Resolves NpcType by debugname,
			// constructs a DESPAWN npc at (p.x, p.z, p.level) with
			// duration=500 ticks. nid is allocated inside s.addNpc
			// (firstSpawn=true). TS has no MessageGame on success.
			// Mirrors TS:
			//   World.addNpc(new Npc(player.level, player.x, player.z,
			//                        type.size, type.size,
			//                        EntityLifeCycle.DESPAWN,
			//                        World.getNextNid(), type.id,
			//                        type.moverestrict, type.blockwalk), 500);
			if args == "" {
				return nil
			}
			name := strings.Fields(args)[0]
			nt := p.client.server.npcTypes.ByName(name)
			if nt == nil {
				return nil
			}
			n := NewNpc(0 /* placeholder; allocated inside addNpc */, nt.ID, p.x, p.z, p.level, nt)
			n.lifecycle = NpcLifecycleDespawn
			_ = p.client.server.addNpc(n, 500, true)
		case "openmain":
			// TS L464-476 — admin interface routing. Resolves
			// ComponentType by debugname, gates on rootLayer == id
			// (only root layers can be main modals), routes through
			// p.OpenMain (which closes chat + side modals and sets
			// refreshModal per TS Player.openMainModal modal-mutex).
			// TS L476: player.openMainModal(type.id).
			if args == "" {
				return nil
			}
			name := strings.Fields(args)[0]
			ct := p.client.server.componentTypes.ByName(name)
			if ct == nil || ct.RootLayer != ct.ID {
				return nil
			}
			p.OpenMain(ct.ID)
		case "openoverlay":
			// TS L530-542 @43e02957 — resolves ComponentType by
			// debugname, gates on rootLayer == id (root layers only,
			// same gate as openmain), routes through p.OpenOverlay
			// (sets p.overlay; IF_OPENOVERLAY flush deferred to
			// encodeOut on overlay != lastOverlay).
			if args == "" {
				return nil
			}
			name := strings.Fields(args)[0]
			ct := p.client.server.componentTypes.ByName(name)
			if ct == nil || ct.RootLayer != ct.ID {
				return nil
			}
			p.OpenOverlay(ct.ID)
		case "closeoverlay":
			// TS L543-544 @43e02957 — player.openOverlay(-1) (clears
			// the old overlay's com listeners, then resets the field).
			p.OpenOverlay(-1)
		case "teleother":
			// TS L377-400 — teleother <username> (production-only via
			// outer arm selector; goscape mirrors with inner NP gate).
			if !p.client.server.cfg.NodeProduction {
				break
			}
			if args == "" {
				return nil
			}
			other := lookupOrReport(p, args)
			if other == nil {
				return nil
			}
			other.CloseModal(true)
			if !other.CanAccess() {
				p.MessageGame(fmt.Sprintf("%s is busy right now.", args))
				return nil
			}
			other.ClearInteraction()
			sendUnsetMapFlag(other)
			other.waypointIndex = -1
			other.TeleJump(p.x, p.z, p.level)
		case "snapshot":
			// TS L477-480 — writes a heap snapshot. TS uses v8's JSON
			// format; goscape uses runtime/pprof.WriteHeapProfile (Go's
			// native heap-profile format). Functional analog —
			// TS-fidelity here is dispatch behavior, not output bytes.
			path := filepath.Join(os.TempDir(), fmt.Sprintf("heap-%d.pprof", time.Now().UnixNano()))
			if f, err := os.Create(path); err == nil {
				if err := pprof.WriteHeapProfile(f); err == nil && p.client.server.log != nil {
					p.client.server.log.Info("heap snapshot written", "path", path)
				}
				f.Close()
			}
		case "setvar":
			// TS L193-246 @43e02957. Not NP-gated. setvar <name> <value>:
			// varbit-FIRST lookup (VarBitType.getByName, L205) — a hit
			// resolves the base varp via VarPlayerType.get(varbit.basevar)
			// and runs the protect gate against it (L209-219); a miss falls
			// through to VarPlayerType.getByName (L221). The original
			// protect block (L228-238) still runs after the branch — on the
			// varbit path with a protected basevar TS literally runs the
			// gate TWICE (idempotent: the 1st pass closes the modal /
			// clears the interaction the 2nd pass re-checks). Mirrored
			// as-is. Write goes through SetVarBit on the varbit path with
			// the VARBIT's debugname in the reply (L240-242), SetVarp +
			// varp debugname otherwise (L244-245).
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			if len(sub) < 2 {
				return nil
			}
			// TS L202: rev-254 moved the int32 clamp ABOVE the lookup.
			value := clampInt32(sub, 1)
			var cfg *objtype.VarPlayerType
			varbit := p.client.server.varbitTypes.ByName(sub[0])
			if varbit != nil {
				// TS L207 indexes configs[basevar] unguarded and would
				// throw on undefined.protect; Go's nil tolerance folds
				// that into the `if (!varp) return false` reject below.
				cfg = p.varpTypeConfig(varbit.Basevar)
				if cfg != nil && cfg.Protect {
					p.CloseModal(true)
					if !p.CanAccess() {
						p.MessageGame("Please finish what you are doing first.")
						return nil
					}
					p.ClearInteraction()
					p.unsetMapFlag()
				}
			} else {
				cfg = p.client.server.varpTypes.ByName(sub[0])
			}
			if cfg == nil {
				return nil
			}
			if cfg.Protect {
				p.CloseModal(true)
				if !p.CanAccess() {
					p.MessageGame("Please finish what you are doing first.")
					return nil
				}
				p.ClearInteraction()
				p.unsetMapFlag()
			}
			if varbit != nil {
				p.SetVarBit(varbit.ID, int32(value))
				p.MessageGame(fmt.Sprintf("set %s: to %d", varbit.DebugName, value))
			} else {
				p.SetVarp(cfg.ID, int32(value))
				p.MessageGame(fmt.Sprintf("set %s: to %d", cfg.DebugName, value))
			}
		case "setvarother":
			// TS L247-279 @43e02957 (shifted by the 254 varbit-aware setvar
			// block above). NP-gated via inner break (DEVIATION-NAI-185-D2).
			// setvarother <username> <name> <value>. Missing-user message
			// goes to caller; busy-target message ALSO goes to caller
			// (DEVIATION-NAI-185-D3 — TS L269 asymmetry).
			if !p.client.server.cfg.NodeProduction {
				break
			}
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 3)
			if len(sub) < 3 {
				return nil
			}
			other := lookupOrReport(p, sub[0])
			if other == nil {
				return nil
			}
			cfg := p.client.server.varpTypes.ByName(sub[1])
			if cfg == nil {
				return nil
			}
			if cfg.Protect {
				other.CloseModal(true)
				if !other.CanAccess() {
					p.MessageGame(fmt.Sprintf("%s is busy right now.", sub[0]))
					return nil
				}
				other.ClearInteraction()
				other.unsetMapFlag()
			}
			value := clampInt32(sub, 2)
			other.SetVarp(cfg.ID, int32(value))
			p.MessageGame(fmt.Sprintf("set %s: to %d on %s", cfg.DebugName, value, other.username))
		case "getvar":
			// TS L280-320 @43e02957. Not NP-gated. getvar <name>:
			// varbit-FIRST lookup (L291) — a hit resolves the base varp
			// and runs the protect gate against it (L295-305, same shape
			// as setvar's varbit branch; the plain-varp path keeps its
			// historical no-gate behavior), then replies
			// `get <varbit.debugname>: <getVarBit(id)>` (L314-316).
			// A miss falls through to the varp path (L307) → caller gets
			// `get <debugname>: <value>` where value is p.Varp(id) (0
			// for unset).
			if args == "" {
				return nil
			}
			var cfg *objtype.VarPlayerType
			varbit := p.client.server.varbitTypes.ByName(args)
			if varbit != nil {
				// TS L293 indexes configs[basevar] unguarded (see setvar).
				cfg = p.varpTypeConfig(varbit.Basevar)
				if cfg != nil && cfg.Protect {
					p.CloseModal(true)
					if !p.CanAccess() {
						p.MessageGame("Please finish what you are doing first.")
						return nil
					}
					p.ClearInteraction()
					p.unsetMapFlag()
				}
			} else {
				cfg = p.client.server.varpTypes.ByName(args)
			}
			if cfg == nil {
				return nil
			}
			if varbit != nil {
				p.MessageGame(fmt.Sprintf("get %s: %d", varbit.DebugName, p.GetVarBit(varbit.ID)))
			} else {
				p.MessageGame(fmt.Sprintf("get %s: %d", cfg.DebugName, p.Varp(cfg.ID)))
			}
		case "getvarother":
			// TS L321-340 @43e02957. NP-gated via inner break. getvarother
			// <username> <name>. Caller gets the target's varp value
			// formatted as `get <debugname>: <value> on <other.username>`.
			if !p.client.server.cfg.NodeProduction {
				break
			}
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			if len(sub) < 2 {
				return nil
			}
			other := lookupOrReport(p, sub[0])
			if other == nil {
				return nil
			}
			cfg := p.client.server.varpTypes.ByName(sub[1])
			if cfg == nil {
				return nil
			}
			p.MessageGame(fmt.Sprintf("get %s: %d on %s", cfg.DebugName, other.Varp(cfg.ID), other.username))
		case "giveother":
			// TS L303-322. NP-gated via inner break. giveother
			// <username> <obj> [count]. Count defaults to 1, clamps
			// to [1, 0x7fffffff].
			if !p.client.server.cfg.NodeProduction {
				break
			}
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 3)
			if len(sub) < 2 {
				return nil
			}
			other := lookupOrReport(p, sub[0])
			if other == nil {
				return nil
			}
			objType := p.client.server.objTypes.ByName(sub[1])
			if objType == nil {
				return nil
			}
			count := clampCount(sub, 2)
			other.InvAdd(p.client.server.invTypes.Inv, objType.ID, count)
		case "givecrap":
			// TS L323-338. Not NP-gated. Fills inventory with 28
			// random items filtered by NodeMembers + DummyItem + CertTemplate.
			// Retry-loop matches TS `while (random === -1)`.
			for range 28 {
				for {
					id := rand.IntN(len(p.client.server.objTypes.Configs))
					obj := p.client.server.objTypes.Configs[id]
					if obj == nil {
						continue
					}
					if !p.client.server.cfg.NodeMembers && obj.Members {
						continue
					}
					if obj.DummyItem != 0 {
						continue
					}
					if obj.CertTemplate != -1 {
						continue
					}
					p.InvAdd(p.client.server.invTypes.Inv, id, 1)
					break
				}
			}
		case "broadcast":
			// TS L353-359. NP-gated via inner break. broadcast <message>.
			// DEVIATION-NAI-185-D1-DEAD-GUARD: TS L355 `args.length < 0`
			// is unreachable (array length is non-negative); not ported.
			// TS uses cheat.substring(cmd.length+1); goscape uses `args`
			// (already the post-first-space remainder of the lowercased
			// input) — semantically identical for any single-token cmd.
			if !p.client.server.cfg.NodeProduction {
				break
			}
			p.client.server.BroadcastMes(args)
		}
	}

	// TS ClientCheatHandler.ts:483 — super-mod block. Gated on
	// staffModLevel >= 2. NAI-183.
	if p.staffModLevel >= 2 {
		switch parts[0] {
		case "getcoord":
			// Mirrors TS L489 — `::getcoord` displays the player's
			// current coord as level,mapX,mapZ,localX,localZ.
			p.MessageGame(coordgrid.FormatString(p.level, p.x, p.z, ","))
		case "tele":
			// Mirrors TS `::tele level,mapX,mapZ[,localX,localZ]` at
			// ClientCheatHandler.ts:491-524. Single-arg form:
			// "::tele 0,50,50,32,32".
			//
			// NAI-93 closed the prior DEVIATION block here: closeModal,
			// canAccess gate, and the unsetMapFlag bundle (sendUnsetMapFlag
			// + waypointIndex reset, per TS Player.unsetMapFlag at
			// Player.ts:2169-2172) are now wired. ClearInteraction
			// preserved.
			if args == "" {
				return nil
			}
			coord := strings.Split(args, ",")
			if len(coord) < 3 {
				return nil
			}

			// Pre-tele cleanup chain — order per TS lines 504-512.
			p.CloseModal(true) // TS closeModal() default-arg.
			if !p.CanAccess() {
				p.MessageGame("Please finish what you are doing first.")
				return nil
			}
			p.ClearInteraction()
			sendUnsetMapFlag(p)
			p.waypointIndex = -1 // TS Player.unsetMapFlag → clearWaypoints.

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
			p.TeleJump((mx<<6)+lx, (mz<<6)+lz, level)
		case "teleto":
			// TS L525-548 — teleto <username> (production-only).
			if !p.client.server.cfg.NodeProduction {
				break
			}
			if args == "" {
				return nil
			}
			other := lookupOrReport(p, args)
			if other == nil {
				return nil
			}
			p.CloseModal(true)
			if !p.CanAccess() {
				p.MessageGame("Please finish what you are doing first.")
				return nil
			}
			p.ClearInteraction()
			sendUnsetMapFlag(p)
			p.waypointIndex = -1
			p.TeleJump(other.x, other.z, other.level)
		case "setvis":
			// TS ClientCheatHandler.ts:549-568 — ::setvis <level>.
			// NodeProduction-gated. NAI-186.
			if !p.client.server.cfg.NodeProduction {
				return nil
			}
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			switch sub[0] {
			case "0":
				p.SetVisibility(rsbuf.VisibilityDefault)
			case "1":
				p.SetVisibility(rsbuf.VisibilitySoft)
			case "2":
				p.SetVisibility(rsbuf.VisibilityHard)
			default:
				return nil
			}
		case "ban":
			// TS ClientCheatHandler.ts:569-581 — ::ban <username> <minutes>.
			// NodeProduction-gated. Calls World.notifyPlayerBan with
			// staff=p.username (manual-staff invocation; distinct from the
			// "automated" callers at handler_reportabuse.go:50 and
			// handler_message_private.go:42). NAI-186.
			if !p.client.server.cfg.NodeProduction {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			if len(sub) < 2 || sub[0] == "" {
				p.MessageGame("Usage: ::ban <username> <minutes>")
				return nil
			}
			username := sub[0]
			minutes := parseIntOr(sub[1], 60)
			if minutes < 0 {
				minutes = 0
			}
			p.client.server.loginBridgeMod.NotifyPlayerBan(p.username, username, time.Now().Add(time.Duration(minutes)*time.Minute))
			p.MessageGame(fmt.Sprintf("Player '%s' has been banned for %d minutes.", username, minutes))
		case "mute":
			// TS ClientCheatHandler.ts:582-594 — ::mute <username> <minutes>.
			// NodeProduction-gated. Calls World.notifyPlayerMute with
			// staff=p.username. NAI-186.
			if !p.client.server.cfg.NodeProduction {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			if len(sub) < 2 || sub[0] == "" {
				p.MessageGame("Usage: ::mute <username> <minutes>")
				return nil
			}
			username := sub[0]
			minutes := parseIntOr(sub[1], 60)
			if minutes < 0 {
				minutes = 0
			}
			p.client.server.loginBridgeMod.NotifyPlayerMute(p.username, username, time.Now().Add(time.Duration(minutes)*time.Minute))
			p.MessageGame(fmt.Sprintf("Player '%s' has been muted for %d minutes.", username, minutes))

		case "kick":
			// TS ClientCheatHandler.ts:595-616 — ::kick <username>.
			// NodeProduction-gated. Lookup via LookupPlayerByUsername; on
			// hit, set loggingOut=true and ack.
			//
			// DEVIATION-NAI-186-D1 — TS does inline `other.logout(); other.client.close()`
			// at L608-611. Goscape sets loggingOut=true and lets processLogouts
			// (tick.go:277) handle teardown (writeOut OpLogout + flushWrite +
			// conn.Close + removePlayerOnTick). Same end-state, ≤1 tick defer.
			// Retire if/when goscape grows a synchronous force-logout helper.
			//
			// NAI-186.
			if !p.client.server.cfg.NodeProduction {
				return nil
			}
			if args == "" {
				p.MessageGame("Usage: ::kick <username>")
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			username := sub[0]
			if other := p.client.server.LookupPlayerByUsername(username); other != nil {
				other.loggingOut = true
				p.MessageGame(fmt.Sprintf("Player '%s' has been kicked from the game.", username))
			} else {
				p.MessageGame(fmt.Sprintf("Player '%s' does not exist or is not logged in.", username))
			}
		}
	}

	// Ungated arms. ::say has no TS counterpart in ClientCheatHandler
	// (TS routes it through ChatHandler instead); kept ungated. NAI-183.
	switch parts[0] {
	case "say":
		if args != "" {
			p.Say([]byte(args))
		}
	}

	return nil
}

// clampCount parses sub[idx] as an item count for the give/giveother cheat
// handlers, clamping to [1, 0x7fffffff]. Missing arg (len(sub) <= idx)
// defaults to 1, matching TS L288-302 (give) / L303-322 (giveother). The
// arg index differs between the two callers (give reads sub[1], giveother
// reads sub[2]), hence idx is a parameter rather than hardcoded.
func clampCount(sub []string, idx int) int {
	count := 1
	if len(sub) > idx {
		count = parseIntOr(sub[idx], 1)
		if count < 1 {
			count = 1
		}
		if count > 0x7fffffff {
			count = 0x7fffffff
		}
	}
	return count
}

// clampInt32 parses sub[idx] as an integer for the setvar/setvarother cheat
// handlers, clamping to the int32 range [-0x80000000, 0x7fffffff].
// Non-numeric input defaults to 0 via parseIntOr. Callers must guarantee
// len(sub) > idx: unlike clampCount, this helper does not length-guard and
// panics on a missing arg. Both callers satisfy this (setvar rejects
// len(sub) < 2, setvarother rejects len(sub) < 3 before calling). The arg
// index differs between the two callers (setvar reads sub[1], setvarother
// reads sub[2]), hence idx is a parameter rather than hardcoded.
func clampInt32(sub []string, idx int) int {
	value := parseIntOr(sub[idx], 0)
	if value > 0x7fffffff {
		value = 0x7fffffff
	}
	if value < -0x80000000 {
		value = -0x80000000
	}
	return value
}

// lookupOrReport looks up other by username for the teleother/setvarother/
// getvarother/giveother/teleto cheat handlers. On a miss it sends the
// caller the "%s is not logged in." MessageGame and returns nil; callers
// must treat a nil return as "handler done, return nil". Does NOT cover
// ::kick (handlers_game.go case "kick"), which uses a different message
// ("does not exist or is not logged in.") and if/else control flow rather
// than an early return — that site is intentionally left inline.
func lookupOrReport(p *Player, name string) *Player {
	other := p.client.server.LookupPlayerByUsername(name)
	if other == nil {
		p.MessageGame(fmt.Sprintf("%s is not logged in.", name))
		return nil
	}
	return other
}

// parseIntOr ports TS tryParseInt (src/util/TryParse.ts:11-26) used by the
// cheat handler at ClientCheatHandler.ts. tryParseInt delegates to JS
// parseInt(value) (no radix). Per the ECMAScript parseInt grammar:
// leading whitespace is skipped, optional +/- sign accepted, a leading
// "0x"/"0X" switches to base 16, otherwise the longest leading decimal-digit
// prefix is consumed and the parse stops at the first non-digit. Any input
// without parseable leading digits yields NaN, which tryParseInt maps to def.
//
// Diverges from Go's strconv.Atoi (strict — rejects trailing garbage, hex
// prefix, decimal points, even leading whitespace). Pre-fix concrete
// divergences on cheat-command args: "100ms" → TS 100 / Go def; "30s" →
// TS 30 / Go def; "0x10" → TS 16 / Go def; "  42" → TS 42 / Go def.
func parseIntOr(s string, def int) int {
	s = strings.TrimLeft(s, " \t\n\v\f\r")
	if s == "" {
		return def
	}
	sign := 1
	i := 0
	if s[0] == '+' {
		i = 1
	} else if s[0] == '-' {
		sign = -1
		i = 1
	}
	base := 10
	if len(s)-i >= 2 && s[i] == '0' && (s[i+1] == 'x' || s[i+1] == 'X') {
		base = 16
		i += 2
	}
	start := i
	if base == 10 {
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	} else {
		for i < len(s) && isHexDigit(s[i]) {
			i++
		}
	}
	if i == start {
		return def
	}
	n, err := strconv.ParseInt(s[start:i], base, 64)
	if err != nil {
		return def
	}
	return sign * int(n)
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// handleMoveMinimapClick is the dispatch entry for MOVE_MINIMAPCLICK
// (opcode 56). It routes to the shared inner handler with opClick=false
// — so the !opClick body fires (ClearPendingAction closes any open modal,
// e.g. the bank, when the player walks away via the minimap) — and a
// 14-byte trailer offset for the camera/compass bytes the client appends
// to this opcode only (TS MoveClickDecoder.ts:16). TS binds all three
// move opcodes to one MoveClickHandler; this mirrors that.
func handleMoveMinimapClick(p *Player, payload []byte) error {
	return moveClickInner(p, payload, false, 14)
}
