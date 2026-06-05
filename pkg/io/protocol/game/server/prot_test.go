package server

import "testing"

// TestServerProt244Table pins every server opcode/size against the 244 TS
// contract (Engine-TS@9aadcec4 ServerGameProt.ts + ServerGameZoneProt.ts).
//
// The four DATA_* packets were removed at 244 (maps move to engine
// OnDemand in Bundle 3); their vars and senders are deleted.
func TestServerProt244Table(t *testing.T) {
	cases := []struct {
		name   string
		op     Op
		opcode byte
		size   int
	}{
		// interfaces
		{"IF_OPENCHAT", OpIfOpenChat, 189, 2},
		{"IF_OPENMAIN_SIDE", OpIfOpenMainSide, 207, 4},
		{"IF_CLOSE", OpIfClose, 214, 0},
		{"IF_SETTAB", OpIfSetTab, 200, 3},
		{"IF_SETTAB_ACTIVE", OpIfSetTabActive, 56, 1},
		{"IF_OPENMAIN", OpIfOpenMain, 10, 2},
		{"IF_OPENSIDE", OpIfOpenSide, 176, 2},
		{"IF_OPENOVERLAY", OpIfOpenOverlay, 158, 2}, // new at 244
		// interface setters
		{"IF_SETCOLOUR", OpIfSetColour, 78, 4},
		{"IF_SETHIDE", OpIfSetHide, 123, 3},
		{"IF_SETOBJECT", OpIfSetObject, 164, 6},
		{"IF_SETMODEL", OpIfSetModel, 245, 4},
		// IF_SETRECOL (103/6) removed at 244 — encoder/model deleted upstream; see PORTING.md §B2/§B4.
		{"IF_SETANIM", OpIfSetAnim, 219, 4},
		{"IF_SETPLAYERHEAD", OpIfSetPlayerHead, 108, 2},
		{"IF_SETTEXT", OpIfSetText, 154, -2},
		{"IF_SETNPCHEAD", OpIfSetNpcHead, 129, 4},
		{"IF_SETPOSITION", OpIfSetPosition, 241, 6},
		// tutorial area
		{"TUT_FLASH", OpTutFlash, 168, 1},
		{"TUT_OPEN", OpTutOpen, 174, 2},
		// inventory
		{"UPDATE_INV_STOP_TRANSMIT", OpUpdateInvStopTransmit, 162, 2},
		{"UPDATE_INV_FULL", OpUpdateInvFull, 72, -2},
		{"UPDATE_INV_PARTIAL", OpUpdateInvPartial, 132, -2},
		// camera
		{"CAM_LOOKAT", OpCamLookAt, 222, 6},
		{"CAM_SHAKE", OpCamShake, 50, 4},
		{"CAM_MOVETO", OpCamMoveTo, 12, 6},
		{"CAM_RESET", OpCamReset, 53, 0},
		// entity updates
		{"NPC_INFO", OpNpcInfo, 244, -2},
		{"PLAYER_INFO", OpPlayerInfo, 86, -2},
		// input tracking
		{"FINISH_TRACKING", OpFinishTracking, 60, 0},
		{"ENABLE_TRACKING", OpEnableTracking, 22, 0},
		// social
		{"MESSAGE_GAME", OpMessageGame, 95, -1},
		{"UPDATE_IGNORELIST", OpUpdateIgnoreList, 7, -2},
		{"CHAT_FILTER_SETTINGS", OpChatFilterSettings, 9, 3},
		{"MESSAGE_PRIVATE", OpMessagePrivate, 30, -1},
		{"UPDATE_FRIENDLIST", OpUpdateFriendList, 70, 9},
		// misc
		{"UNSET_MAP_FLAG", OpUnsetMapFlag, 62, 0},
		{"UPDATE_RUNWEIGHT", OpUpdateRunWeight, 160, 2},
		{"HINT_ARROW", OpHintArrow, 49, 6},
		{"UPDATE_REBOOT_TIMER", OpUpdateRebootTimer, 85, 2},
		{"UPDATE_STAT", OpUpdateStat, 24, 6},
		{"UPDATE_RUNENERGY", OpUpdateRunEnergy, 177, 1},
		{"RESET_ANIMS", OpResetAnims, 242, 0},
		{"LOGOUT", OpLogout, 17, 0},
		{"P_COUNTDIALOG", OpPCountDialog, 152, 0},
		{"SET_MULTIWAY", OpSetMultiway, 97, 1},
		// varps
		{"VARP_SMALL", OpVarpSmall, 236, 3},
		{"VARP_LARGE", OpVarpLarge, 226, 6},
		{"RESET_CLIENT_VARCACHE", OpResetClientVarCache, 87, 0},
		// audio (non-deferred)
		{"SYNTH_SOUND", OpSynthSound, 151, 5},
		// zone outer
		{"UPDATE_ZONE_PARTIAL_FOLLOWS", OpUpdateZonePartialFollows, 94, 2},
		{"UPDATE_ZONE_FULL_FOLLOWS", OpUpdateZoneFullFollows, 131, 2},
		{"UPDATE_ZONE_PARTIAL_ENCLOSED", OpUpdateZonePartialEnclosed, 233, -2},
		// zone nested (ServerGameZoneProt.ts 244)
		{"LOC_MERGE", OpLocMerge, 29, 14},
		{"LOC_ANIM", OpLocAnim, 155, 4},
		{"OBJ_DEL", OpObjDel, 39, 3},
		{"OBJ_REVEAL", OpObjReveal, 69, 7},
		{"LOC_ADD_CHANGE", OpLocAddChange, 232, 4},
		{"MAP_PROJANIM", OpMapProjAnim, 137, 15},
		{"LOC_DEL", OpLocDel, 125, 2},
		{"OBJ_COUNT", OpObjCount, 209, 7},
		{"MAP_ANIM", OpMapAnim, 198, 6},
		{"OBJ_ADD", OpObjAdd, 234, 5},

		// --- five rows updated in Task 3 (emitters co-updated) ---
		// TS ServerGameProt.ts (244): UPDATE_PID=210/3
		{"UPDATE_PID", OpUpdatePid, 210, 3},
		// TS ServerGameProt.ts (244): LAST_LOGIN_INFO=44/10
		{"LAST_LOGIN_INFO", OpLastLoginInfo, 44, 10},
		// TS ServerGameProt.ts (244): REBUILD_NORMAL=165/4
		{"REBUILD_NORMAL", OpRebuildNormal, 165, 4},
		// TS ServerGameProt.ts (244): MIDI_SONG=240/2
		{"MIDI_SONG", OpMidiSong, 240, 2},
		// TS ServerGameProt.ts (244): MIDI_JINGLE=173/4
		{"MIDI_JINGLE", OpMidiJingle, 173, 4},
	}
	for _, tc := range cases {
		if tc.op.Opcode != tc.opcode {
			t.Errorf("%s: Opcode = %d, want %d", tc.name, tc.op.Opcode, tc.opcode)
		}
		if tc.op.PayloadSize != tc.size {
			t.Errorf("%s: PayloadSize = %d, want %d", tc.name, tc.op.PayloadSize, tc.size)
		}
	}
}

func TestServerOpValues(t *testing.T) {
	cases := []struct {
		op     Op
		opcode byte
		size   int
	}{
		{OpIfClose, 214, 0},
		{OpIfOpenMain, 10, 2},
		{OpIfOpenChat, 189, 2},
		{OpIfOpenSide, 176, 2},
		{OpIfOpenMainSide, 207, 4},
		{OpLogout, 17, 0},
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
		{OpRebuildNormal, 165, 4}, // TS ServerGameProt.ts (244): REBUILD_NORMAL=165/4 — updated in Task 3
		{OpUpdateInvFull, 72, -2},
		{OpUpdateInvPartial, 132, -2},
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
	if OpPlayerInfo.Opcode != 86 {
		t.Errorf("OpPlayerInfo.Opcode = %d, want 86", OpPlayerInfo.Opcode)
	}
	if OpPlayerInfo.PayloadSize != -2 {
		t.Errorf("OpPlayerInfo.PayloadSize = %d, want -2", OpPlayerInfo.PayloadSize)
	}
}

func TestSubSpec3COpcodes(t *testing.T) {
	if OpNpcInfo.Opcode != 244 {
		t.Errorf("OpNpcInfo.Opcode = %d, want 244", OpNpcInfo.Opcode)
	}
	if OpNpcInfo.PayloadSize != -2 {
		t.Errorf("OpNpcInfo.PayloadSize = %d, want -2", OpNpcInfo.PayloadSize)
	}
}

// TestIfSetRecolRemoved244 asserts that IF_SETRECOL (103/6) is absent from
// the AllOps name table. TS 244 deletes IfSetRecolEncoder.ts + its model;
// the wire row goes with it (B2 deferral, closed in B4 Task 2).
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
		{OpEnableTracking, 22, 0},
		{OpFinishTracking, 60, 0},
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
