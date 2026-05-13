package world

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// gameHandlers is indexed by decrypted game opcode. Nil means no handler
// registered for that opcode; readPacket() silently discards such packets
// (they still must be in Ops[] to be accepted at all).
var gameHandlers [256]func(*Player, []byte) error

func init() {
	gameHandlers[108] = handleNoTimeout // NO_TIMEOUT
	gameHandlers[70] = handleNoTimeout  // IDLE_TIMER

	gameHandlers[181] = handleMoveGameClick    // MOVE_GAMECLICK (opClick=false)
	gameHandlers[93] = handleMoveOpClick       // MOVE_OPCLICK (opClick=true)
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

	gameHandlers[140] = handleOpObj1 // OPOBJ1
	gameHandlers[40] = handleOpObj2  // OPOBJ2
	gameHandlers[200] = handleOpObj3 // OPOBJ3
	gameHandlers[178] = handleOpObj4 // OPOBJ4
	gameHandlers[247] = handleOpObj5 // OPOBJ5
	gameHandlers[138] = handleOpObjT // OPOBJT
	gameHandlers[239] = handleOpObjU // OPOBJU

	gameHandlers[195] = handleOpHeld1 // OPHELD1
	gameHandlers[71] = handleOpHeld2  // OPHELD2
	gameHandlers[133] = handleOpHeld3 // OPHELD3
	gameHandlers[157] = handleOpHeld4 // OPHELD4
	gameHandlers[211] = handleOpHeld5 // OPHELD5
	gameHandlers[48] = handleOpHeldT   // OPHELDT
	gameHandlers[130] = handleOpHeldU // OPHELDU

	gameHandlers[244] = handleChatSetMode // CHAT_SETMODE
	gameHandlers[118] = handleFriendListAdd // FRIENDLIST_ADD
	gameHandlers[11] = handleFriendListDel  // FRIENDLIST_DEL
	gameHandlers[79] = handleIgnoreListAdd  // IGNORELIST_ADD
	gameHandlers[171] = handleIgnoreListDel // IGNORELIST_DEL
	gameHandlers[190] = handleReportAbuse   // REPORT_ABUSE

	gameHandlers[81] = handleEventTracking // EVENT_TRACKING

	gameHandlers[148] = handleMessagePrivate // MESSAGE_PRIVATE
	gameHandlers[158] = handleMessagePublic  // MESSAGE_PUBLIC

	gameHandlers[235] = handleResumePauseButton // RESUME_PAUSEBUTTON
	gameHandlers[237] = handleResumeCountDialog // RESUME_P_COUNTDIALOG

	gameHandlers[231] = handleCloseModal   // CLOSE_MODAL
	gameHandlers[175] = handleTutClickSide // TUT_CLICKSIDE

	gameHandlers[155] = handleIfButton         // IF_BUTTON
	gameHandlers[31] = handleInvButton1        // INV_BUTTON1
	gameHandlers[59] = handleInvButton2        // INV_BUTTON2
	gameHandlers[212] = handleInvButton3       // INV_BUTTON3
	gameHandlers[38] = handleInvButton4        // INV_BUTTON4
	gameHandlers[6] = handleInvButton5         // INV_BUTTON5
	gameHandlers[159] = handleInvButtonD       // INV_BUTTOND
	gameHandlers[52] = handleIdkSaveDesignGame // IDK_SAVEDESIGN
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

// handleIfButton is the package-level adapter that wires the []byte-payload
// gameHandlers dispatch into the Server method of the same name
// (see handler_interface.go). Looks up the Server via p.client.server.
func handleIfButton(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleIfButton(p, payload)
}

func handleInvButton1(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleInvButton(p, payload, 1)
}

func handleInvButton2(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleInvButton(p, payload, 2)
}

func handleInvButton3(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleInvButton(p, payload, 3)
}

func handleInvButton4(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleInvButton(p, payload, 4)
}

func handleInvButton5(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleInvButton(p, payload, 5)
}

func handleInvButtonD(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleInvButtonD(p, payload)
}

func handleIdkSaveDesignGame(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleIdkSaveDesign(p, payload)
}

func handleNoTimeout(_ *Player, _ []byte) error {
	return nil
}

// handleMoveGameClick is the dispatch entry for MOVE_GAMECLICK (opcode 181).
// Routes to the shared inner handler with opClick=false, which causes the
// !opClick body to fire (clearPendingAction + tempRun + walktrigger).
func handleMoveGameClick(p *Player, payload []byte) error {
	return moveClickInner(p, payload, false)
}

// handleMoveOpClick is the dispatch entry for MOVE_OPCLICK (opcode 93).
// Routes to the shared inner handler with opClick=true, which skips the
// !opClick body (the move was triggered by an op click, not a plain
// ground click — the op click already handled modal/interaction state).
func handleMoveOpClick(p *Player, payload []byte) error {
	return moveClickInner(p, payload, true)
}

// moveClickInner is the shared move-click implementation.
// Mirrors TS MoveClickHandler.ts:10-58.
//
// Wire payload (per TS MoveClickDecoder.ts; identical between opcodes
// 181 and 93):
//
//	byte 0:    ctrlHeld (G1, expected 0 or 1)
//	bytes 1-2: startX (G2)
//	bytes 3-4: startZ (G2)
//	bytes 5+:  up to 24 waypoints, each 2 bytes (dx:G1B, dz:G1B)
//
// Gates per TS MoveClickHandler.ts:11-22:
//  1. p.delayed → write UnsetMapFlag (no waypoint clear), no-op
//  2. ctrlHeld out of [0,1] OR DistanceToSW(player, start) > 104 →
//     player.unsetMapFlag() (clearWaypoints + write) + clear userPath,
//     no-op. TS Player.ts:2169 unsetMapFlag = clearWaypoints + write;
//     goscape's sendUnsetMapFlag is the write only, so gate-2 also
//     resets waypointIndex inline to halt any in-flight movement from
//     a prior click.
//
// On success:
//  3. Build packed waypoint slice
//  4. cfg.WalkTriggerSetting==PLAYERPACKET → pathToMoveClick
//  5. !opClick:
//     a. ClearPendingAction (fires CloseModal(true) — symptom-2 fix)
//     b. tempRun = ctrlHeld; override to 0 if runenergy<100 && ctrlHeld==1
//     c. cfg.WalkTriggerSetting==PLAYERPACKET && hasWaypoints → processWalktrigger
func moveClickInner(p *Player, payload []byte, opClick bool) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 5 {
		return nil
	}

	r := packet.NewPacket(payload)
	ctrlHeld := int(r.G1())
	startX := int(r.G2())
	startZ := int(r.G2())

	if ctrlHeld < 0 || ctrlHeld > 1 || coordgrid.DistanceToSW(p.x, p.z, startX, startZ) > 104 {
		sendUnsetMapFlag(p)
		p.waypointIndex = -1 // TS Player.unsetMapFlag → clearWaypoints (Player.ts:2169)
		p.userPath = nil
		return nil
	}

	pathLen := min((len(payload)-5)/2, 24) + 1
	packed := make([]int, 0, pathLen)
	packed = append(packed, coordgrid.PackCoord(p.level, startX, startZ))
	for range min((len(payload)-5)/2, 24) {
		ddx := int(r.G1B())
		ddz := int(r.G1B())
		packed = append(packed, coordgrid.PackCoord(p.level, startX+ddx, startZ+ddz))
	}

	// Persist for per-tick WalkTriggerSetting fallback (T3).
	// Under client-routefinder, store the full path; otherwise store
	// only the dest. Mirrors TS MoveClickHandler.ts:23-37.
	if s.cfg.NodeClientRoutefinder {
		p.userPath = append(p.userPath[:0], packed...)
	} else {
		dest := packed[len(packed)-1]
		if cap(p.userPath) > 0 {
			p.userPath = p.userPath[:0]
		}
		p.userPath = append(p.userPath, dest)
	}

	p.client.log.Debug("move click", "ctrl_held", ctrlHeld, "dest_packed", packed[0], "op_click", opClick)

	if s.cfg.NodeWalktriggerSetting == WalkTriggerSettingPlayerpacket {
		needsFinding := !s.cfg.NodeClientRoutefinder
		p.pathToMoveClick(packed, needsFinding)
	}

	if !opClick {
		p.ClearPendingAction()

		if p.runenergy < 100 && ctrlHeld == 1 {
			p.tempRun = 0
		} else {
			p.tempRun = ctrlHeld
		}

		if s.cfg.NodeWalktriggerSetting == WalkTriggerSettingPlayerpacket && p.hasWaypoints() {
			p.processWalktrigger()
		}
	}

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
	// DEVIATION-NAI-184-D2-D3-CARRYFORWARD — supersedes
	// DEVIATION-NAI-182-D3-OTHER-CHEATS. 17 TS ClientCheatHandler cheats
	// remain unported:
	//   Dev block (!NP && >=4): reload, rebuild, speed.
	//   Admin block (>=3):      setvar, setvarother, getvar, getvarother,
	//                           giveother, givecrap, broadcast, locadd,
	//                           npcadd, openmain.
	//   Super-mod (>=2):        setvis, ban, mute, kick.
	// Each is blocked on a missing subsystem (VarPlayerType.GetByName,
	// World.broadcastMes, runtime tick-rate mutation, login moderation
	// callbacks, dynamic Loc/NPC spawn, Visibility plumbing). Deferred
	// to follow-up sub-specs.
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
				_ = p.client.conn.Close()
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
			p.AddXP(stat, objtype.GetExpByLevel(level))
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
			count := 1
			if len(sub) > 1 {
				count = parseIntOr(sub[1], 1)
				if count < 1 {
					count = 1
				}
				if count > 0x7fffffff {
					count = 0x7fffffff
				}
			}
			p.InvAdd(p.client.server.invTypes.Inv, objType.ID, count, false)
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
			p.InvAdd(p.client.server.invTypes.Inv, objType.ID, 1000, false)
		case "minme":
			// TS L432-440 — set every stat to 1 except HITPOINTS=10.
			// TS iterates indices [0, PlayerStatEnabled.length); it does
			// NOT filter on PlayerStatEnabled value. STAT18/STAT19 are
			// reserved/unused so the call has no in-game effect, but
			// TS-fidelity requires the SetStat invocation.
			for i := 0; i < objtype.PlayerStatCount; i++ {
				if i == objtype.PlayerStatHitpoints {
					p.SetStat(i, 10)
				} else {
					p.SetStat(i, 1)
				}
			}
		case "teleother":
			// TS L377-400 — teleother <username> (production-only via
			// outer arm selector; goscape mirrors with inner NP gate).
			if !p.client.server.cfg.NodeProduction {
				break
			}
			if args == "" {
				return nil
			}
			other := p.client.server.LookupPlayerByUsername(args)
			if other == nil {
				p.MessageGame(fmt.Sprintf("%s is not logged in.", args))
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
			// TS L192-219. Not NP-gated. setvar <name> <value>: ByName
			// lookup, optional protect-path modal close + canAccess gate
			// + clearInteraction + unsetMapFlag, then SetVarp with
			// int32-clamped value. Caller gets `set <debugname>: to <value>`.
			if args == "" {
				return nil
			}
			sub := strings.SplitN(args, " ", 2)
			if len(sub) < 2 {
				return nil
			}
			cfg := p.client.server.varpTypes.ByName(sub[0])
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
			value := parseIntOr(sub[1], 0)
			if value > 0x7fffffff {
				value = 0x7fffffff
			}
			if value < -0x80000000 {
				value = -0x80000000
			}
			p.SetVarp(cfg.ID, int32(value))
			p.MessageGame(fmt.Sprintf("set %s: to %d", cfg.DebugName, value))
		case "setvarother":
			// TS L220-252. NP-gated via inner break (DEVIATION-NAI-185-D2).
			// setvarother <username> <name> <value>. Missing-user message
			// goes to caller; busy-target message ALSO goes to caller
			// (DEVIATION-NAI-185-D3 — TS L242 asymmetry).
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
			other := p.client.server.LookupPlayerByUsername(sub[0])
			if other == nil {
				p.MessageGame(fmt.Sprintf("%s is not logged in.", sub[0]))
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
			value := parseIntOr(sub[2], 0)
			if value > 0x7fffffff {
				value = 0x7fffffff
			}
			if value < -0x80000000 {
				value = -0x80000000
			}
			other.SetVarp(cfg.ID, int32(value))
			p.MessageGame(fmt.Sprintf("set %s: to %d on %s", cfg.DebugName, value, other.username))
		case "getvar":
			// TS L253-267. Not NP-gated. getvar <name> → caller gets
			// `get <debugname>: <value>` where value is p.Varp(id) (0
			// for unset).
			if args == "" {
				return nil
			}
			cfg := p.client.server.varpTypes.ByName(args)
			if cfg == nil {
				return nil
			}
			p.MessageGame(fmt.Sprintf("get %s: %d", cfg.DebugName, p.Varp(cfg.ID)))
		case "getvarother":
			// TS L268-287. NP-gated via inner break. getvarother
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
			other := p.client.server.LookupPlayerByUsername(sub[0])
			if other == nil {
				p.MessageGame(fmt.Sprintf("%s is not logged in.", sub[0]))
				return nil
			}
			cfg := p.client.server.varpTypes.ByName(sub[1])
			if cfg == nil {
				return nil
			}
			p.MessageGame(fmt.Sprintf("get %s: %d on %s", cfg.DebugName, other.Varp(cfg.ID), other.username))
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
			other := p.client.server.LookupPlayerByUsername(args)
			if other == nil {
				p.MessageGame(fmt.Sprintf("%s is not logged in.", args))
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
