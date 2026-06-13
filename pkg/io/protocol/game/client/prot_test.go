package client

import "testing"

// TestClientProt274 pins the full 274 client opcode table against the TS
// ClientGameProt.ts contract at the 274 pin (dee467c8). All 82 entries must
// be present with correct opcode, name, size, and category. 274 renumbers
// every client->server opcode and DELETES EVENT_TRACKING (no replacement;
// the four discrete event packets survive, renumbered). No new packets.
// Categories: the TS ctor is (id, length) only — categories come from the
// model files (CLIENT_EVENT for the EVENT_* split packets per
// model/EventMouseClick.ts etc.); MAP_BUILD_COMPLETE has no TS model
// (ClientEvent by precedent).
func TestClientProt274(t *testing.T) {
	type row struct {
		opcode   uint8
		name     string
		size     int
		category int
	}

	c := CategoryClientEvent
	u := CategoryUserEvent

	want := []row{
		{OpcNoTimeout, "NO_TIMEOUT", 0, c},

		{OpcIdleTimer, "IDLE_TIMER", 0, c},
		{OpcEventMouseClick, "EVENT_MOUSE_CLICK", 4, c},
		{OpcEventMouseMove, "EVENT_MOUSE_MOVE", -1, c},
		{OpcEventAppletFocus, "EVENT_APPLET_FOCUS", 1, c},
		{OpcEventCameraPosition, "EVENT_CAMERA_POSITION", 4, c},

		{OpcAnticheatOplogic1, "ANTICHEAT_OPLOGIC1", 4, c},
		{OpcAnticheatOplogic2, "ANTICHEAT_OPLOGIC2", 2, c},
		{OpcAnticheatOplogic3, "ANTICHEAT_OPLOGIC3", 4, c},
		{OpcAnticheatOplogic4, "ANTICHEAT_OPLOGIC4", 1, c},
		{OpcAnticheatOplogic5, "ANTICHEAT_OPLOGIC5", 1, c},
		{OpcAnticheatOplogic6, "ANTICHEAT_OPLOGIC6", 2, c},
		{OpcAnticheatOplogic7, "ANTICHEAT_OPLOGIC7", 4, c},
		{OpcAnticheatOplogic8, "ANTICHEAT_OPLOGIC8", 1, c},
		{OpcAnticheatOplogic9, "ANTICHEAT_OPLOGIC9", 3, c},

		{OpcAnticheatCyclelogic1, "ANTICHEAT_CYCLELOGIC1", -1, c},
		{OpcAnticheatCyclelogic2, "ANTICHEAT_CYCLELOGIC2", -1, c},
		{OpcAnticheatCyclelogic3, "ANTICHEAT_CYCLELOGIC3", 1, c},
		{OpcAnticheatCyclelogic4, "ANTICHEAT_CYCLELOGIC4", 1, c},
		{OpcAnticheatCyclelogic5, "ANTICHEAT_CYCLELOGIC5", 0, c},
		{OpcAnticheatCyclelogic6, "ANTICHEAT_CYCLELOGIC6", 1, c},
		{OpcAnticheatCyclelogic7, "ANTICHEAT_CYCLELOGIC7", 0, c},

		{OpcOpObj1, "OPOBJ1", 6, u},
		{OpcOpObj2, "OPOBJ2", 6, u},
		{OpcOpObj3, "OPOBJ3", 6, u},
		{OpcOpObj4, "OPOBJ4", 6, u},
		{OpcOpObj5, "OPOBJ5", 6, u},
		{OpcOpObjT, "OPOBJT", 8, u},
		{OpcOpObjU, "OPOBJU", 12, u},

		{OpcOpNpc1, "OPNPC1", 2, u},
		{OpcOpNpc2, "OPNPC2", 2, u},
		{OpcOpNpc3, "OPNPC3", 2, u},
		{OpcOpNpc4, "OPNPC4", 2, u},
		{OpcOpNpc5, "OPNPC5", 2, u},
		{OpcOpNpcT, "OPNPCT", 4, u},
		{OpcOpNpcU, "OPNPCU", 8, u},

		{OpcOpLoc1, "OPLOC1", 6, u},
		{OpcOpLoc2, "OPLOC2", 6, u},
		{OpcOpLoc3, "OPLOC3", 6, u},
		{OpcOpLoc4, "OPLOC4", 6, u},
		{OpcOpLoc5, "OPLOC5", 6, u},
		{OpcOpLocT, "OPLOCT", 8, u},
		{OpcOpLocU, "OPLOCU", 12, u},

		{OpcOpPlayer1, "OPPLAYER1", 2, u},
		{OpcOpPlayer2, "OPPLAYER2", 2, u},
		{OpcOpPlayer3, "OPPLAYER3", 2, u},
		{OpcOpPlayer4, "OPPLAYER4", 2, u},
		{OpcOpPlayer5, "OPPLAYER5", 2, u},
		{OpcOpPlayerT, "OPPLAYERT", 4, u},
		{OpcOpPlayerU, "OPPLAYERU", 8, u},

		{OpcOpHeld1, "OPHELD1", 6, u},
		{OpcOpHeld2, "OPHELD2", 6, u},
		{OpcOpHeld3, "OPHELD3", 6, u},
		{OpcOpHeld4, "OPHELD4", 6, u},
		{OpcOpHeld5, "OPHELD5", 6, u},
		{OpcOpHeldT, "OPHELDT", 8, u},
		{OpcOpHeldU, "OPHELDU", 12, u},

		{OpcInvButton1, "INV_BUTTON1", 6, u},
		{OpcInvButton2, "INV_BUTTON2", 6, u},
		{OpcInvButton3, "INV_BUTTON3", 6, u},
		{OpcInvButton4, "INV_BUTTON4", 6, u},
		{OpcInvButton5, "INV_BUTTON5", 6, u},

		{OpcIfButton, "IF_BUTTON", 2, u},
		{OpcResumePauseButton, "RESUME_PAUSEBUTTON", 2, u},
		{OpcCloseModal, "CLOSE_MODAL", 0, u},
		{OpcResumePCountdialog, "RESUME_P_COUNTDIALOG", 4, u},
		{OpcTutorialClickSide, "TUTORIAL_CLICKSIDE", 1, u},

		{OpcMapBuildComplete, "MAP_BUILD_COMPLETE", 0, c},
		{OpcMoveOpClick, "MOVE_OPCLICK", -1, u},
		{OpcReportAbuse, "REPORT_ABUSE", 10, u},
		{OpcMoveMinimapClick, "MOVE_MINIMAPCLICK", -1, u},
		{OpcInvButtonD, "INV_BUTTOND", 7, u},
		{OpcIgnorelistDel, "IGNORELIST_DEL", 8, u},
		{OpcIgnorelistAdd, "IGNORELIST_ADD", 8, u},
		{OpcIfPlayerDesign, "IF_PLAYERDESIGN", 13, u},
		{OpcChatSetmode, "CHAT_SETMODE", 3, u},
		{OpcMessagePrivate, "MESSAGE_PRIVATE", -1, u},
		{OpcFriendlistDel, "FRIENDLIST_DEL", 8, u},
		{OpcFriendlistAdd, "FRIENDLIST_ADD", 8, u},
		{OpcClientCheat, "CLIENT_CHEAT", -1, u},
		{OpcMessagePublic, "MESSAGE_PUBLIC", -1, u},
		{OpcMoveGameClick, "MOVE_GAMECLICK", -1, u},
	}

	// Pin the exact 274 wire opcode for every row (constants alone would let
	// a const+table double-swap pass).
	wantOpcode := map[string]uint8{
		"NO_TIMEOUT": 120,

		"IDLE_TIMER":            209,
		"EVENT_MOUSE_CLICK":     20,
		"EVENT_MOUSE_MOVE":      222,
		"EVENT_APPLET_FOCUS":    73,
		"EVENT_CAMERA_POSITION": 53,

		"ANTICHEAT_OPLOGIC1": 219,
		"ANTICHEAT_OPLOGIC2": 201,
		"ANTICHEAT_OPLOGIC3": 41,
		"ANTICHEAT_OPLOGIC4": 80,
		"ANTICHEAT_OPLOGIC5": 235,
		"ANTICHEAT_OPLOGIC6": 250,
		"ANTICHEAT_OPLOGIC7": 25,
		"ANTICHEAT_OPLOGIC8": 0,
		"ANTICHEAT_OPLOGIC9": 24,

		"ANTICHEAT_CYCLELOGIC1": 12,
		"ANTICHEAT_CYCLELOGIC2": 149,
		"ANTICHEAT_CYCLELOGIC3": 52,
		"ANTICHEAT_CYCLELOGIC4": 230,
		"ANTICHEAT_CYCLELOGIC5": 100,
		"ANTICHEAT_CYCLELOGIC6": 188,
		"ANTICHEAT_CYCLELOGIC7": 89,

		"OPOBJ1": 247,
		"OPOBJ2": 169,
		"OPOBJ3": 108,
		"OPOBJ4": 62,
		"OPOBJ5": 117,
		"OPOBJT": 91,
		"OPOBJU": 39,

		"OPNPC1": 236,
		"OPNPC2": 233,
		"OPNPC3": 223,
		"OPNPC4": 147,
		"OPNPC5": 189,
		"OPNPCT": 181,
		"OPNPCU": 150,

		"OPLOC1": 215,
		"OPLOC2": 103,
		"OPLOC3": 187,
		"OPLOC4": 157,
		"OPLOC5": 127,
		"OPLOCT": 213,
		"OPLOCU": 60,

		"OPPLAYER1": 109,
		"OPPLAYER2": 166,
		"OPPLAYER3": 196,
		"OPPLAYER4": 98,
		"OPPLAYER5": 174,
		"OPPLAYERT": 240,
		"OPPLAYERU": 36,

		"OPHELD1": 185,
		"OPHELD2": 2,
		"OPHELD3": 123,
		"OPHELD4": 216,
		"OPHELD5": 42,
		"OPHELDT": 135,
		"OPHELDU": 136,

		"INV_BUTTON1": 74,
		"INV_BUTTON2": 82,
		"INV_BUTTON3": 239,
		"INV_BUTTON4": 179,
		"INV_BUTTON5": 46,

		"IF_BUTTON":            9,
		"RESUME_PAUSEBUTTON":   72,
		"CLOSE_MODAL":          51,
		"RESUME_P_COUNTDIALOG": 102,
		"TUTORIAL_CLICKSIDE":   94,

		"MAP_BUILD_COMPLETE": 214,
		"MOVE_OPCLICK":       138,
		"REPORT_ABUSE":       137,
		"MOVE_MINIMAPCLICK":  86,
		"INV_BUTTOND":        93,
		"IGNORELIST_DEL":     101,
		"IGNORELIST_ADD":     255,
		"IF_PLAYERDESIGN":    125,
		"CHAT_SETMODE":       154,
		"MESSAGE_PRIVATE":    139,
		"FRIENDLIST_DEL":     106,
		"FRIENDLIST_ADD":     13,
		"CLIENT_CHEAT":       224,
		"MESSAGE_PUBLIC":     253,
		"MOVE_GAMECLICK":     207,
	}
	if len(wantOpcode) != len(want) {
		t.Fatalf("wantOpcode has %d entries, want rows = %d", len(wantOpcode), len(want))
	}

	// Verify each expected row.
	for _, w := range want {
		if wire, ok := wantOpcode[w.name]; !ok {
			t.Errorf("%s: missing wire-opcode pin", w.name)
		} else if wire != w.opcode {
			t.Errorf("%s: opcode constant = %d, want wire value %d (TS 274 pin)", w.name, w.opcode, wire)
		}
		op := Ops[w.opcode]
		if op.Name != w.name {
			t.Errorf("Ops[%d].Name = %q, want %q", w.opcode, op.Name, w.name)
		}
		if op.PayloadSize != w.size {
			t.Errorf("Ops[%d] (%s): PayloadSize = %d, want %d", w.opcode, w.name, op.PayloadSize, w.size)
		}
		if op.Category != w.category {
			t.Errorf("Ops[%d] (%s): Category = %d, want %d", w.opcode, w.name, op.Category, w.category)
		}
	}

	// Count non-empty entries; must equal exactly 82.
	count := 0
	for i := range Ops {
		if Ops[i].Name != "" {
			count++
		}
	}
	if count != 82 {
		t.Errorf("Ops non-empty count = %d, want 82 (stale entries or missing 274 entries)", count)
	}

	// Names removed/renamed before 274 must not appear anywhere in the table.
	oldNames := map[string]bool{
		"REBUILD_GETMAPS": true, // removed at 244
		"IDK_SAVEDESIGN":  true, // renamed at 244 (IF_PLAYERDESIGN)
		"TUT_CLICKSIDE":   true, // renamed at 244 (TUTORIAL_CLICKSIDE)
		"EVENT_TRACKING":  true, // deleted at 274 (no replacement)
	}
	for i := range Ops {
		if oldNames[Ops[i].Name] {
			t.Errorf("Ops[%d].Name = %q: pre-274 name must not appear in 274 table", i, Ops[i].Name)
		}
	}
}
