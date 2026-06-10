package server

import "testing"

// TestServerProt254Table pins every server opcode/size against the 254 TS
// contract (Engine-TS@43e02957 ServerGameProt.ts + ServerGameZoneProt.ts).
//
// The four DATA_* packets were removed at 244 (maps move to engine
// OnDemand in Bundle 3); their vars and senders are deleted.
// FRIENDLIST_LOADED and SET_PLAYER_OP are new at 254.
func TestServerProt254Table(t *testing.T) {
	cases := []struct {
		name   string
		op     Op
		opcode byte
		size   int
	}{
		// interfaces
		{"IF_OPENCHAT", OpIfOpenChat, 141, 2},
		{"IF_OPENMAIN_SIDE", OpIfOpenMainSide, 249, 4},
		{"IF_CLOSE", OpIfClose, 174, 0},
		{"IF_SETTAB", OpIfSetTab, 91, 3},
		{"IF_SETTAB_ACTIVE", OpIfSetTabActive, 138, 1},
		{"IF_OPENMAIN", OpIfOpenMain, 197, 2},
		{"IF_OPENSIDE", OpIfOpenSide, 187, 2},
		{"IF_OPENOVERLAY", OpIfOpenOverlay, 85, 2},
		// interface setters
		{"IF_SETCOLOUR", OpIfSetColour, 38, 4},
		{"IF_SETHIDE", OpIfSetHide, 227, 3},
		{"IF_SETOBJECT", OpIfSetObject, 222, 6},
		{"IF_SETMODEL", OpIfSetModel, 211, 4},
		// IF_SETRECOL absent from ServerGameProt.ts @43e02957 — removed at 244;
		// encoder/model deleted upstream; see PORTING.md §B2/§B4.
		{"IF_SETANIM", OpIfSetAnim, 95, 4},
		{"IF_SETPLAYERHEAD", OpIfSetPlayerHead, 161, 2},
		{"IF_SETTEXT", OpIfSetText, 41, -2},
		{"IF_SETNPCHEAD", OpIfSetNpcHead, 3, 4},
		{"IF_SETPOSITION", OpIfSetPosition, 27, 6},
		{"IF_SETSCROLLPOS", OpIfSetScrollPos, 14, 4}, // new in 245.2
		// tutorial area
		{"TUT_FLASH", OpTutFlash, 58, 1},
		{"TUT_OPEN", OpTutOpen, 239, 2},
		// inventory
		{"UPDATE_INV_STOP_TRANSMIT", OpUpdateInvStopTransmit, 168, 2},
		{"UPDATE_INV_FULL", OpUpdateInvFull, 28, -2},
		{"UPDATE_INV_PARTIAL", OpUpdateInvPartial, 170, -2},
		// camera
		{"CAM_LOOKAT", OpCamLookAt, 0, 6},
		{"CAM_SHAKE", OpCamShake, 225, 4},
		{"CAM_MOVETO", OpCamMoveTo, 55, 6},
		{"CAM_RESET", OpCamReset, 167, 0},
		// entity updates
		{"NPC_INFO", OpNpcInfo, 123, -2},
		{"PLAYER_INFO", OpPlayerInfo, 87, -2},
		// input tracking
		{"FINISH_TRACKING", OpFinishTracking, 29, 0},
		{"ENABLE_TRACKING", OpEnableTracking, 251, 0},
		// social
		{"FRIENDLIST_LOADED", OpFriendlistLoaded, 255, 1}, // new in 254
		{"MESSAGE_GAME", OpMessageGame, 73, -1},
		{"UPDATE_IGNORELIST", OpUpdateIgnoreList, 63, -2},
		{"CHAT_FILTER_SETTINGS", OpChatFilterSettings, 24, 3},
		{"MESSAGE_PRIVATE", OpMessagePrivate, 60, -1},
		{"UPDATE_FRIENDLIST", OpUpdateFriendList, 111, 9},
		// misc
		{"UNSET_MAP_FLAG", OpUnsetMapFlag, 108, 0},
		{"UPDATE_RUNWEIGHT", OpUpdateRunWeight, 164, 2},
		{"HINT_ARROW", OpHintArrow, 64, 6},
		{"UPDATE_REBOOT_TIMER", OpUpdateRebootTimer, 143, 2},
		{"UPDATE_STAT", OpUpdateStat, 136, 6},
		{"UPDATE_RUNENERGY", OpUpdateRunEnergy, 94, 1},
		{"RESET_ANIMS", OpResetAnims, 203, 0},
		{"LOGOUT", OpLogout, 21, 0},
		{"P_COUNTDIALOG", OpPCountDialog, 5, 0},
		{"SET_MULTIWAY", OpSetMultiway, 75, 1},
		{"SET_PLAYER_OP", OpSetPlayerOp, 204, -1}, // new in 254
		// varps
		{"VARP_SMALL", OpVarpSmall, 186, 3},
		{"VARP_LARGE", OpVarpLarge, 196, 6},
		{"RESET_CLIENT_VARCACHE", OpResetClientVarCache, 140, 0},
		// audio (non-deferred)
		{"SYNTH_SOUND", OpSynthSound, 25, 5},
		// zone outer
		{"UPDATE_ZONE_PARTIAL_FOLLOWS", OpUpdateZonePartialFollows, 173, 2},
		{"UPDATE_ZONE_FULL_FOLLOWS", OpUpdateZoneFullFollows, 159, 2},
		{"UPDATE_ZONE_PARTIAL_ENCLOSED", OpUpdateZonePartialEnclosed, 61, -2},
		// zone nested (ServerGameZoneProt.ts 254)
		{"LOC_MERGE", OpLocMerge, 218, 14},
		{"LOC_ANIM", OpLocAnim, 30, 4},
		{"OBJ_DEL", OpObjDel, 115, 3},
		{"OBJ_REVEAL", OpObjReveal, 8, 7},
		{"LOC_ADD_CHANGE", OpLocAddChange, 70, 4},
		{"MAP_PROJANIM", OpMapProjAnim, 37, 15},
		{"LOC_DEL", OpLocDel, 88, 2},
		{"OBJ_COUNT", OpObjCount, 98, 7},
		{"MAP_ANIM", OpMapAnim, 114, 6},
		{"OBJ_ADD", OpObjAdd, 120, 5},

		// TS ServerGameProt.ts (254): UPDATE_PID=213/3
		{"UPDATE_PID", OpUpdatePid, 213, 3},
		// TS ServerGameProt.ts (254): LAST_LOGIN_INFO=146/10
		{"LAST_LOGIN_INFO", OpLastLoginInfo, 146, 10},
		// TS ServerGameProt.ts (254): REBUILD_NORMAL=209/4
		{"REBUILD_NORMAL", OpRebuildNormal, 209, 4},
		// TS ServerGameProt.ts (254): MIDI_SONG=163/2
		{"MIDI_SONG", OpMidiSong, 163, 2},
		// TS ServerGameProt.ts (254): MIDI_JINGLE=242/4
		{"MIDI_JINGLE", OpMidiJingle, 242, 4},
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
		{OpIfOpenMain, 197, 2},
		{OpIfOpenChat, 141, 2},
		{OpIfOpenSide, 187, 2},
		{OpIfOpenMainSide, 249, 4},
		{OpLogout, 21, 0},
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
		{OpRebuildNormal, 209, 4}, // TS ServerGameProt.ts (254): REBUILD_NORMAL=209/4
		{OpUpdateInvFull, 28, -2},
		{OpUpdateInvPartial, 170, -2},
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
	if OpPlayerInfo.Opcode != 87 {
		t.Errorf("OpPlayerInfo.Opcode = %d, want 87", OpPlayerInfo.Opcode)
	}
	if OpPlayerInfo.PayloadSize != -2 {
		t.Errorf("OpPlayerInfo.PayloadSize = %d, want -2", OpPlayerInfo.PayloadSize)
	}
}

func TestSubSpec3COpcodes(t *testing.T) {
	if OpNpcInfo.Opcode != 123 {
		t.Errorf("OpNpcInfo.Opcode = %d, want 123", OpNpcInfo.Opcode)
	}
	if OpNpcInfo.PayloadSize != -2 {
		t.Errorf("OpNpcInfo.PayloadSize = %d, want -2", OpNpcInfo.PayloadSize)
	}
}

// TestIfSetRecolRemoved244 asserts that IF_SETRECOL is absent from
// the AllOps name table. TS 244 deletes IfSetRecolEncoder.ts + its model;
// the wire row remains absent from ServerGameProt.ts @43e02957 as well
// (verified with grep — no IF_SETRECOL line exists at the 254 pin).
// See PORTING.md §B2/§B4 for context.
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
		{OpEnableTracking, 251, 0},
		{OpFinishTracking, 29, 0},
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
