package client

import "testing"

// TestClientProt244 pins the full 244 client opcode table against the TS
// ClientGameProt.ts contract at the 244 pin (9aadcec4). All 76 entries must
// be present with correct opcode, name, size, and category. Removed entries
// (REBUILD_GETMAPS, EVENT_CAMERA_POSITION) and renamed entries
// (IDK_SAVEDESIGN, TUT_CLICKSIDE) must not appear.
func TestClientProt244(t *testing.T) {
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
		{OpcEventTracking, "EVENT_TRACKING", -2, r},

		{OpcAnticheatOplogic1, "ANTICHEAT_OPLOGIC1", 4, c},
		{OpcAnticheatOplogic2, "ANTICHEAT_OPLOGIC2", 4, c},
		{OpcAnticheatOplogic3, "ANTICHEAT_OPLOGIC3", 3, c},
		{OpcAnticheatOplogic4, "ANTICHEAT_OPLOGIC4", 2, c},
		{OpcAnticheatOplogic5, "ANTICHEAT_OPLOGIC5", 0, c},
		{OpcAnticheatOplogic6, "ANTICHEAT_OPLOGIC6", 4, c},
		{OpcAnticheatOplogic7, "ANTICHEAT_OPLOGIC7", 4, c},
		{OpcAnticheatOplogic8, "ANTICHEAT_OPLOGIC8", 2, c},
		{OpcAnticheatOplogic9, "ANTICHEAT_OPLOGIC9", 1, c},

		{OpcAnticheatCyclelogic1, "ANTICHEAT_CYCLELOGIC1", 1, c},
		{OpcAnticheatCyclelogic2, "ANTICHEAT_CYCLELOGIC2", -1, c},
		{OpcAnticheatCyclelogic3, "ANTICHEAT_CYCLELOGIC3", 3, c},
		{OpcAnticheatCyclelogic4, "ANTICHEAT_CYCLELOGIC4", 4, c},
		{OpcAnticheatCyclelogic5, "ANTICHEAT_CYCLELOGIC5", 0, c},
		{OpcAnticheatCyclelogic6, "ANTICHEAT_CYCLELOGIC6", -1, c},

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

	// Verify each expected row.
	for _, w := range want {
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

	// Count non-empty entries; must equal exactly 78.
	count := 0
	for i := range Ops {
		if Ops[i].Name != "" {
			count++
		}
	}
	if count != 76 {
		t.Errorf("Ops non-empty count = %d, want 76 (stale 225 entries or missing 244 entries)", count)
	}

	// Removed entries must not appear.
	absent := []struct {
		opcode uint8
		name   string
	}{
		{150, "REBUILD_GETMAPS"},
		{189, "EVENT_CAMERA_POSITION"},
	}
	for _, a := range absent {
		if Ops[a.opcode].Name == a.name {
			t.Errorf("Ops[%d]: %q must not be present (removed at 244)", a.opcode, a.name)
		}
	}

	// Renamed entries must not appear under their old names anywhere in the table.
	oldNames := map[string]bool{
		"IDK_SAVEDESIGN": true,
		"TUT_CLICKSIDE":  true,
	}
	for i := range Ops {
		if oldNames[Ops[i].Name] {
			t.Errorf("Ops[%d].Name = %q: old 225 name must not appear in 244 table", i, Ops[i].Name)
		}
	}
}
