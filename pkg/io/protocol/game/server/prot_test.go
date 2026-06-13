package server

import "testing"

// TestServerProt274Table pins every server opcode/size against the 274 TS
// contract (Engine-TS@dee467c8 ServerGameProt.ts + ServerGameZoneProt.ts).
//
// The four DATA_* packets were removed at 244 (maps move to engine
// OnDemand in Bundle 3); their vars and senders are deleted.
// FRIENDLIST_LOADED and SET_PLAYER_OP are new at 254.
// MINIMAP_TOGGLE is new at 274; ENABLE_TRACKING/FINISH_TRACKING were
// deleted at 274 (rows absent from ServerGameProt.ts @dee467c8).
func TestServerProt274Table(t *testing.T) {
	cases := []struct {
		name   string
		op     Op
		opcode byte
		size   int
	}{
		// interfaces
		{"IF_OPENCHAT", OpIfOpenChat, 166, 2},
		{"IF_OPENMAIN_SIDE", OpIfOpenMainSide, 158, 4},
		{"IF_CLOSE", OpIfClose, 171, 0},
		{"IF_SETTAB", OpIfSetTab, 215, 3},
		{"IF_SETTAB_ACTIVE", OpIfSetTabActive, 241, 1},
		{"IF_OPENMAIN", OpIfOpenMain, 211, 2},
		{"IF_OPENSIDE", OpIfOpenSide, 16, 2},
		{"IF_OPENOVERLAY", OpIfOpenOverlay, 240, 2},
		// interface setters
		{"IF_SETCOLOUR", OpIfSetColour, 183, 4},
		{"IF_SETHIDE", OpIfSetHide, 10, 3},
		{"IF_SETOBJECT", OpIfSetObject, 28, 6},
		{"IF_SETMODEL", OpIfSetModel, 129, 4},
		// IF_SETRECOL absent from ServerGameProt.ts @dee467c8 — removed at 244;
		// encoder/model deleted upstream; see PORTING.md §B2/§B4.
		{"IF_SETANIM", OpIfSetAnim, 134, 4},
		{"IF_SETPLAYERHEAD", OpIfSetPlayerHead, 192, 2},
		{"IF_SETTEXT", OpIfSetText, 44, -2},
		{"IF_SETNPCHEAD", OpIfSetNpcHead, 142, 4},
		{"IF_SETPOSITION", OpIfSetPosition, 77, 6},
		{"IF_SETSCROLLPOS", OpIfSetScrollPos, 54, 4}, // new in 245.2
		// tutorial area
		{"TUT_FLASH", OpTutFlash, 90, 1},
		{"TUT_OPEN", OpTutOpen, 130, 2},
		// inventory
		{"UPDATE_INV_STOP_TRANSMIT", OpUpdateInvStopTransmit, 227, 2},
		{"UPDATE_INV_FULL", OpUpdateInvFull, 106, -2},
		{"UPDATE_INV_PARTIAL", OpUpdateInvPartial, 172, -2},
		// camera
		{"CAM_LOOKAT", OpCamLookAt, 233, 6},
		{"CAM_SHAKE", OpCamShake, 64, 4},
		{"CAM_MOVETO", OpCamMoveTo, 200, 6},
		{"CAM_RESET", OpCamReset, 101, 0},
		// entity updates
		{"NPC_INFO", OpNpcInfo, 197, -2},
		{"PLAYER_INFO", OpPlayerInfo, 167, -2},
		// ENABLE_TRACKING / FINISH_TRACKING deleted at 274 — rows absent
		// from ServerGameProt.ts @dee467c8.
		// social
		{"FRIENDLIST_LOADED", OpFriendlistLoaded, 185, 1}, // new in 254
		{"MESSAGE_GAME", OpMessageGame, 161, -1},
		{"UPDATE_IGNORELIST", OpUpdateIgnoreList, 3, -2},
		{"CHAT_FILTER_SETTINGS", OpChatFilterSettings, 114, 3},
		{"MESSAGE_PRIVATE", OpMessagePrivate, 235, -1},
		{"UPDATE_FRIENDLIST", OpUpdateFriendList, 247, 9},
		// misc
		{"UNSET_MAP_FLAG", OpUnsetMapFlag, 115, 0},
		{"UPDATE_RUNWEIGHT", OpUpdateRunWeight, 67, 2},
		{"HINT_ARROW", OpHintArrow, 156, 6},
		{"UPDATE_REBOOT_TIMER", OpUpdateRebootTimer, 89, 2},
		{"UPDATE_STAT", OpUpdateStat, 105, 6},
		{"UPDATE_RUNENERGY", OpUpdateRunEnergy, 83, 1},
		{"RESET_ANIMS", OpResetAnims, 47, 0},
		{"LOGOUT", OpLogout, 88, 0},
		{"P_COUNTDIALOG", OpPCountDialog, 210, 0},
		{"SET_MULTIWAY", OpSetMultiway, 207, 1},
		{"SET_PLAYER_OP", OpSetPlayerOp, 17, -1},    // new in 254
		{"MINIMAP_TOGGLE", OpMinimapToggle, 194, 1}, // new in 274
		// varps
		{"VARP_SMALL", OpVarpSmall, 203, 3},
		{"VARP_LARGE", OpVarpLarge, 245, 6},
		{"RESET_CLIENT_VARCACHE", OpResetClientVarCache, 190, 0},
		// audio (non-deferred)
		{"SYNTH_SOUND", OpSynthSound, 34, 5},
		// zone outer
		{"UPDATE_ZONE_PARTIAL_FOLLOWS", OpUpdateZonePartialFollows, 32, 2},
		{"UPDATE_ZONE_FULL_FOLLOWS", OpUpdateZoneFullFollows, 153, 2},
		{"UPDATE_ZONE_PARTIAL_ENCLOSED", OpUpdateZonePartialEnclosed, 195, -2},
		// zone nested (ServerGameZoneProt.ts 274)
		{"LOC_MERGE", OpLocMerge, 176, 14},
		{"LOC_ANIM", OpLocAnim, 48, 4},
		{"OBJ_DEL", OpObjDel, 52, 3},
		{"OBJ_REVEAL", OpObjReveal, 219, 7},
		{"LOC_ADD_CHANGE", OpLocAddChange, 138, 4},
		{"MAP_PROJANIM", OpMapProjAnim, 107, 15},
		{"LOC_DEL", OpLocDel, 173, 2},
		{"OBJ_COUNT", OpObjCount, 95, 7},
		{"MAP_ANIM", OpMapAnim, 85, 6},
		{"OBJ_ADD", OpObjAdd, 81, 5},

		// TS ServerGameProt.ts (274): UPDATE_PID=133/3
		{"UPDATE_PID", OpUpdatePid, 133, 3},
		// TS ServerGameProt.ts (274): LAST_LOGIN_INFO=91/10
		{"LAST_LOGIN_INFO", OpLastLoginInfo, 91, 10},
		// TS ServerGameProt.ts (274): REBUILD_NORMAL=231/4
		{"REBUILD_NORMAL", OpRebuildNormal, 231, 4},
		// TS ServerGameProt.ts (274): MIDI_SONG=23/2
		{"MIDI_SONG", OpMidiSong, 23, 2},
		// TS ServerGameProt.ts (274): MIDI_JINGLE=15/4
		{"MIDI_JINGLE", OpMidiJingle, 15, 4},
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
		{OpIfClose, 171, 0},
		{OpIfOpenMain, 211, 2},
		{OpIfOpenChat, 166, 2},
		{OpIfOpenSide, 16, 2},
		{OpIfOpenMainSide, 158, 4},
		{OpLogout, 88, 0},
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
		{OpRebuildNormal, 231, 4}, // TS ServerGameProt.ts (274): REBUILD_NORMAL=231/4
		{OpUpdateInvFull, 106, -2},
		{OpUpdateInvPartial, 172, -2},
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
	if OpPlayerInfo.Opcode != 167 {
		t.Errorf("OpPlayerInfo.Opcode = %d, want 167", OpPlayerInfo.Opcode)
	}
	if OpPlayerInfo.PayloadSize != -2 {
		t.Errorf("OpPlayerInfo.PayloadSize = %d, want -2", OpPlayerInfo.PayloadSize)
	}
}

func TestSubSpec3COpcodes(t *testing.T) {
	if OpNpcInfo.Opcode != 197 {
		t.Errorf("OpNpcInfo.Opcode = %d, want 197", OpNpcInfo.Opcode)
	}
	if OpNpcInfo.PayloadSize != -2 {
		t.Errorf("OpNpcInfo.PayloadSize = %d, want -2", OpNpcInfo.PayloadSize)
	}
}

// TestIfSetRecolRemoved244 asserts that IF_SETRECOL is absent from
// the AllOps name table. TS 244 deletes IfSetRecolEncoder.ts + its model;
// the wire row remains absent from ServerGameProt.ts @dee467c8 as well
// (verified with grep — no IF_SETRECOL line exists at the 274 pin).
// See PORTING.md §B2/§B4 for context.
func TestIfSetRecolRemoved244(t *testing.T) {
	for _, e := range AllOps() {
		if e.Name == "IF_SETRECOL" {
			t.Fatalf("IF_SETRECOL wire row still registered")
		}
	}
}

// TestTrackingOpsRemoved274 asserts that ENABLE_TRACKING and
// FINISH_TRACKING are absent from the AllOps name table. The rows are
// gone from TS ServerGameProt.ts @dee467c8 (274); the 254 table had kept
// them registered with no sender (NAI-73). Their Op vars are deleted.
func TestTrackingOpsRemoved274(t *testing.T) {
	for _, e := range AllOps() {
		if e.Name == "ENABLE_TRACKING" || e.Name == "FINISH_TRACKING" {
			t.Fatalf("%s wire row still registered — deleted at 274", e.Name)
		}
	}
}
