package server

import "testing"

// TestServerProt245_2Table pins every server opcode/size against the 245.2 TS
// contract (Engine-TS@3c16994c ServerGameProt.ts + ServerGameZoneProt.ts).
//
// The four DATA_* packets were removed at 244 (maps move to engine
// OnDemand in Bundle 3); their vars and senders are deleted.
func TestServerProt245_2Table(t *testing.T) {
	cases := []struct {
		name   string
		op     Op
		opcode byte
		size   int
	}{
		// interfaces
		{"IF_OPENCHAT", OpIfOpenChat, 7, 2},
		{"IF_OPENMAIN_SIDE", OpIfOpenMainSide, 229, 4},
		{"IF_CLOSE", OpIfClose, 174, 0},
		{"IF_SETTAB", OpIfSetTab, 29, 3},
		{"IF_SETTAB_ACTIVE", OpIfSetTabActive, 8, 1},
		{"IF_OPENMAIN", OpIfOpenMain, 177, 2},
		{"IF_OPENSIDE", OpIfOpenSide, 236, 2},
		{"IF_OPENOVERLAY", OpIfOpenOverlay, 115, 2},
		// interface setters
		{"IF_SETCOLOUR", OpIfSetColour, 135, 4},
		{"IF_SETHIDE", OpIfSetHide, 225, 3},
		{"IF_SETOBJECT", OpIfSetObject, 153, 6},
		{"IF_SETMODEL", OpIfSetModel, 60, 4},
		// IF_SETRECOL absent from ServerGameProt.ts @3c16994c — removed at 244;
		// encoder/model deleted upstream; see docs/PORTING.md §B2/§B4.
		{"IF_SETANIM", OpIfSetAnim, 69, 4},
		{"IF_SETPLAYERHEAD", OpIfSetPlayerHead, 83, 2},
		{"IF_SETTEXT", OpIfSetText, 32, -2},
		{"IF_SETNPCHEAD", OpIfSetNpcHead, 76, 4},
		{"IF_SETPOSITION", OpIfSetPosition, 230, 6},
		{"IF_SETSCROLLPOS", OpIfSetScrollPos, 226, 4}, // new in 245.2
		// tutorial area
		{"TUT_FLASH", OpTutFlash, 132, 1},
		{"TUT_OPEN", OpTutOpen, 152, 2},
		// inventory
		{"UPDATE_INV_STOP_TRANSMIT", OpUpdateInvStopTransmit, 143, 2},
		{"UPDATE_INV_FULL", OpUpdateInvFull, 156, -2},
		{"UPDATE_INV_PARTIAL", OpUpdateInvPartial, 95, -2},
		// camera
		{"CAM_LOOKAT", OpCamLookAt, 123, 6},
		{"CAM_SHAKE", OpCamShake, 103, 4},
		{"CAM_MOVETO", OpCamMoveTo, 86, 6},
		{"CAM_RESET", OpCamReset, 134, 0},
		// entity updates
		{"NPC_INFO", OpNpcInfo, 105, -2},
		{"PLAYER_INFO", OpPlayerInfo, 161, -2},
		// input tracking
		{"FINISH_TRACKING", OpFinishTracking, 165, 0},
		{"ENABLE_TRACKING", OpEnableTracking, 28, 0},
		// social
		{"MESSAGE_GAME", OpMessageGame, 175, -1},
		{"UPDATE_IGNORELIST", OpUpdateIgnoreList, 181, -2},
		{"CHAT_FILTER_SETTINGS", OpChatFilterSettings, 2, 3},
		{"MESSAGE_PRIVATE", OpMessagePrivate, 207, -1},
		{"UPDATE_FRIENDLIST", OpUpdateFriendList, 109, 9},
		// misc
		{"UNSET_MAP_FLAG", OpUnsetMapFlag, 233, 0},
		{"UPDATE_RUNWEIGHT", OpUpdateRunWeight, 70, 2},
		{"HINT_ARROW", OpHintArrow, 243, 6},
		{"UPDATE_REBOOT_TIMER", OpUpdateRebootTimer, 26, 2},
		{"UPDATE_STAT", OpUpdateStat, 110, 6},
		{"UPDATE_RUNENERGY", OpUpdateRunEnergy, 208, 1},
		{"RESET_ANIMS", OpResetAnims, 144, 0},
		{"LOGOUT", OpLogout, 36, 0},
		{"P_COUNTDIALOG", OpPCountDialog, 56, 0},
		{"SET_MULTIWAY", OpSetMultiway, 35, 1},
		// varps
		{"VARP_SMALL", OpVarpSmall, 192, 3},
		{"VARP_LARGE", OpVarpLarge, 75, 6},
		{"RESET_CLIENT_VARCACHE", OpResetClientVarCache, 25, 0},
		// audio (non-deferred)
		{"SYNTH_SOUND", OpSynthSound, 209, 5},
		// zone outer
		{"UPDATE_ZONE_PARTIAL_FOLLOWS", OpUpdateZonePartialFollows, 203, 2},
		{"UPDATE_ZONE_FULL_FOLLOWS", OpUpdateZoneFullFollows, 140, 2},
		{"UPDATE_ZONE_PARTIAL_ENCLOSED", OpUpdateZonePartialEnclosed, 15, -2},
		// zone nested (ServerGameZoneProt.ts 245.2)
		{"LOC_MERGE", OpLocMerge, 188, 14},
		{"LOC_ANIM", OpLocAnim, 71, 4},
		{"OBJ_DEL", OpObjDel, 13, 3},
		{"OBJ_REVEAL", OpObjReveal, 190, 7},
		{"LOC_ADD_CHANGE", OpLocAddChange, 119, 4},
		{"MAP_PROJANIM", OpMapProjAnim, 187, 15},
		{"LOC_DEL", OpLocDel, 198, 2},
		{"OBJ_COUNT", OpObjCount, 151, 7},
		{"MAP_ANIM", OpMapAnim, 141, 6},
		{"OBJ_ADD", OpObjAdd, 94, 5},

		// TS ServerGameProt.ts (245.2): UPDATE_PID=49/3
		{"UPDATE_PID", OpUpdatePid, 49, 3},
		// TS ServerGameProt.ts (245.2): LAST_LOGIN_INFO=238/10
		{"LAST_LOGIN_INFO", OpLastLoginInfo, 238, 10},
		// TS ServerGameProt.ts (245.2): REBUILD_NORMAL=66/4
		{"REBUILD_NORMAL", OpRebuildNormal, 66, 4},
		// TS ServerGameProt.ts (245.2): MIDI_SONG=96/2
		{"MIDI_SONG", OpMidiSong, 96, 2},
		// TS ServerGameProt.ts (245.2): MIDI_JINGLE=39/4
		{"MIDI_JINGLE", OpMidiJingle, 39, 4},
	}
	for _, tc := range cases {
		if tc.op.Opcode != tc.opcode {
			t.Errorf("%s: Opcode = %d, want %d", tc.name, tc.op.Opcode, tc.opcode)
		}
		if tc.op.PayloadSize != tc.size {
			t.Errorf("%s: PayloadSize = %d, want %d", tc.name, tc.op.PayloadSize, tc.size)
		}
	}
	// An op declared in the var block but omitted from AllOps() would be
	// invisible to external decoders; anchor the registry size to the table.
	if got := len(AllOps()); got != len(cases) {
		t.Errorf("AllOps len = %d, want %d (missing or duplicate entry)", got, len(cases))
	}
}

func TestServerOpValues(t *testing.T) {
	cases := []struct {
		op     Op
		opcode byte
		size   int
	}{
		{OpIfClose, 174, 0},
		{OpIfOpenMain, 177, 2},
		{OpIfOpenChat, 7, 2},
		{OpIfOpenSide, 236, 2},
		{OpIfOpenMainSide, 229, 4},
		{OpLogout, 36, 0},
	}
	for _, tc := range cases {
		if tc.op.Opcode != tc.opcode {
			t.Errorf("%v: Opcode = %d, want %d", tc.op, tc.op.Opcode, tc.opcode)
		}
		if tc.op.PayloadSize != tc.size {
			t.Errorf("%v: PayloadSize = %d, want %d", tc.op, tc.op.PayloadSize, tc.size)
		}
	}
}

func TestSubSpec3AOpcodes(t *testing.T) {
	cases := []struct {
		op     Op
		opcode byte
		size   int
	}{
		{OpRebuildNormal, 66, 4}, // TS ServerGameProt.ts (245.2): REBUILD_NORMAL=66/4
		{OpUpdateInvFull, 156, -2},
		{OpUpdateInvPartial, 95, -2},
	}
	for _, tc := range cases {
		if tc.op.Opcode != tc.opcode {
			t.Errorf("%+v: Opcode = %d, want %d", tc.op, tc.op.Opcode, tc.opcode)
		}
		if tc.op.PayloadSize != tc.size {
			t.Errorf("%+v: PayloadSize = %d, want %d", tc.op, tc.op.PayloadSize, tc.size)
		}
	}
}

func TestSubSpec3BOpcodes(t *testing.T) {
	if OpPlayerInfo.Opcode != 161 {
		t.Errorf("OpPlayerInfo.Opcode = %d, want 161", OpPlayerInfo.Opcode)
	}
	if OpPlayerInfo.PayloadSize != -2 {
		t.Errorf("OpPlayerInfo.PayloadSize = %d, want -2", OpPlayerInfo.PayloadSize)
	}
}

func TestSubSpec3COpcodes(t *testing.T) {
	if OpNpcInfo.Opcode != 105 {
		t.Errorf("OpNpcInfo.Opcode = %d, want 105", OpNpcInfo.Opcode)
	}
	if OpNpcInfo.PayloadSize != -2 {
		t.Errorf("OpNpcInfo.PayloadSize = %d, want -2", OpNpcInfo.PayloadSize)
	}
}

// TestIfSetRecolRemoved244 asserts that IF_SETRECOL (103/6) is absent from
// the AllOps name table. TS 244 deletes IfSetRecolEncoder.ts + its model;
// the wire row is absent from ServerGameProt.ts @3c16994c as well (verified
// with grep — no IF_SETRECOL line exists at the 245.2 pin).
// See docs/PORTING.md §B2/§B4 for context.
func TestIfSetRecolRemoved244(t *testing.T) {
	for _, e := range AllOps() {
		if e.Name == "IF_SETRECOL" {
			t.Fatalf("IF_SETRECOL wire row still registered")
		}
	}
}

func TestNAI73TrackingOpcodes(t *testing.T) {
	cases := []struct {
		op   Op
		code byte
		size int
	}{
		{OpEnableTracking, 28, 0},
		{OpFinishTracking, 165, 0},
	}
	for _, tc := range cases {
		if tc.op.Opcode != tc.code {
			t.Errorf("opcode: got %d, want %d", tc.op.Opcode, tc.code)
		}
		if tc.op.PayloadSize != tc.size {
			t.Errorf("payload size: got %d, want %d", tc.op.PayloadSize, tc.size)
		}
	}
}
