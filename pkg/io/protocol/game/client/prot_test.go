package client

import "testing"

// TestClientProt254 pins the full 254 client opcode table against the TS
// ClientGameProt.ts contract at the 254 pin (43e02957). All 83 entries must
// be present with correct opcode, name, size, and category. New rows at 254:
// EVENT_MOUSE_CLICK, EVENT_MOUSE_MOVE, EVENT_APPLET_FOCUS,
// EVENT_CAMERA_POSITION, ANTICHEAT_CYCLELOGIC7, OPPLAYER5,
// MAP_BUILD_COMPLETE. EVENT_TRACKING survives (renumbered 19 -> 142).
// Categories: the 254 EVENT_* split packets are CLIENT_EVENT per the TS
// model files; EVENT_TRACKING keeps RESTRICTED_EVENT from its last TS model
// (245.2); MAP_BUILD_COMPLETE has no TS model (ClientEvent by precedent).
func TestClientProt254(t *testing.T) {
	type row struct {
		opcode   uint8
		name     string
		size     int
		category int
	}

	c := CategoryClientEvent
	r := CategoryRestrictedEvent
	u := CategoryUserEvent

	want := []row{
		{OpcNoTimeout, "NO_TIMEOUT", 0, c},

		{OpcIdleTimer, "IDLE_TIMER", 0, c},
		{OpcEventMouseClick, "EVENT_MOUSE_CLICK", 4, c},
		{OpcEventMouseMove, "EVENT_MOUSE_MOVE", -1, c},
		{OpcEventAppletFocus, "EVENT_APPLET_FOCUS", 1, c},
		{OpcEventTracking, "EVENT_TRACKING", -2, r},
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

	// Pin the exact 254 wire opcode for every row (constants alone would let
	// a const+table double-swap pass).
	wantOpcode := map[string]uint8{
		"NO_TIMEOUT": 239,

		"IDLE_TIMER":            144,
		"EVENT_MOUSE_CLICK":     234,
		"EVENT_MOUSE_MOVE":      232,
		"EVENT_APPLET_FOCUS":    8,
		"EVENT_TRACKING":        142,
		"EVENT_CAMERA_POSITION": 91,

		"ANTICHEAT_OPLOGIC1": 28,
		"ANTICHEAT_OPLOGIC2": 77,
		"ANTICHEAT_OPLOGIC3": 56,
		"ANTICHEAT_OPLOGIC4": 121,
		"ANTICHEAT_OPLOGIC5": 233,
		"ANTICHEAT_OPLOGIC6": 131,
		"ANTICHEAT_OPLOGIC7": 187,
		"ANTICHEAT_OPLOGIC8": 206,
		"ANTICHEAT_OPLOGIC9": 162,

		"ANTICHEAT_CYCLELOGIC1": 51,
		"ANTICHEAT_CYCLELOGIC2": 225,
		"ANTICHEAT_CYCLELOGIC3": 4,
		"ANTICHEAT_CYCLELOGIC4": 226,
		"ANTICHEAT_CYCLELOGIC5": 100,
		"ANTICHEAT_CYCLELOGIC6": 36,
		"ANTICHEAT_CYCLELOGIC7": 182,

		"OPOBJ1": 141,
		"OPOBJ2": 67,
		"OPOBJ3": 178,
		"OPOBJ4": 47,
		"OPOBJ5": 97,
		"OPOBJT": 202,
		"OPOBJU": 245,

		"OPNPC1": 143,
		"OPNPC2": 195,
		"OPNPC3": 69,
		"OPNPC4": 122,
		"OPNPC5": 118,
		"OPNPCT": 231,
		"OPNPCU": 119,

		"OPLOC1": 33,
		"OPLOC2": 213,
		"OPLOC3": 98,
		"OPLOC4": 87,
		"OPLOC5": 147,
		"OPLOCT": 26,
		"OPLOCU": 240,

		"OPPLAYER1": 192,
		"OPPLAYER2": 17,
		"OPPLAYER3": 18,
		"OPPLAYER4": 72,
		"OPPLAYER5": 230,
		"OPPLAYERT": 68,
		"OPPLAYERU": 113,

		"OPHELD1": 243,
		"OPHELD2": 228,
		"OPHELD3": 80,
		"OPHELD4": 163,
		"OPHELD5": 74,
		"OPHELDT": 102,
		"OPHELDU": 200,

		"INV_BUTTON1": 181,
		"INV_BUTTON2": 70,
		"INV_BUTTON3": 59,
		"INV_BUTTON4": 160,
		"INV_BUTTON5": 62,

		"IF_BUTTON":            244,
		"RESUME_PAUSEBUTTON":   146,
		"CLOSE_MODAL":          58,
		"RESUME_P_COUNTDIALOG": 161,
		"TUTORIAL_CLICKSIDE":   201,

		"MAP_BUILD_COMPLETE": 134,
		"MOVE_OPCLICK":       127,
		"REPORT_ABUSE":       203,
		"MOVE_MINIMAPCLICK":  220,
		"INV_BUTTOND":        176,
		"IGNORELIST_DEL":     193,
		"IGNORELIST_ADD":     189,
		"IF_PLAYERDESIGN":    13,
		"CHAT_SETMODE":       129,
		"MESSAGE_PRIVATE":    214,
		"FRIENDLIST_DEL":     84,
		"FRIENDLIST_ADD":     9,
		"CLIENT_CHEAT":       86,
		"MESSAGE_PUBLIC":     83,
		"MOVE_GAMECLICK":     6,
	}
	if len(wantOpcode) != len(want) {
		t.Fatalf("wantOpcode has %d entries, want rows = %d", len(wantOpcode), len(want))
	}

	// Verify each expected row.
	for _, w := range want {
		if wire, ok := wantOpcode[w.name]; !ok {
			t.Errorf("%s: missing wire-opcode pin", w.name)
		} else if wire != w.opcode {
			t.Errorf("%s: opcode constant = %d, want wire value %d (TS 254 pin)", w.name, w.opcode, wire)
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

	// Count non-empty entries; must equal exactly 83.
	count := 0
	for i := range Ops {
		if Ops[i].Name != "" {
			count++
		}
	}
	if count != 83 {
		t.Errorf("Ops non-empty count = %d, want 83 (stale entries or missing 254 entries)", count)
	}

	// Names removed/renamed before 254 must not appear anywhere in the table.
	oldNames := map[string]bool{
		"REBUILD_GETMAPS": true, // removed at 244
		"IDK_SAVEDESIGN":  true, // renamed at 244 (IF_PLAYERDESIGN)
		"TUT_CLICKSIDE":   true, // renamed at 244 (TUTORIAL_CLICKSIDE)
	}
	for i := range Ops {
		if oldNames[Ops[i].Name] {
			t.Errorf("Ops[%d].Name = %q: pre-254 name must not appear in 254 table", i, Ops[i].Name)
		}
	}
}
