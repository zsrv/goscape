package server

// Op describes a server→client game packet opcode.
type Op struct {
	Opcode      byte
	PayloadSize int // 0=fixed-zero, 2=fixed-2, 4=fixed-4, -1=1-byte-len, -2=2-byte-len
}

// Modal interface opcodes and logout.
// TS ServerGameProt.ts (244): IF_CLOSE=214/0, IF_OPENMAIN=10/2, IF_OPENCHAT=189/2,
// IF_OPENSIDE=176/2, IF_OPENMAIN_SIDE=207/4, IF_OPENOVERLAY=158/2, LOGOUT=17/0.
var (
	OpIfClose        = Op{Opcode: 214, PayloadSize: 0}
	OpIfOpenMain     = Op{Opcode: 10, PayloadSize: 2}
	OpIfOpenChat     = Op{Opcode: 189, PayloadSize: 2}
	OpIfOpenSide     = Op{Opcode: 176, PayloadSize: 2}
	OpIfOpenMainSide = Op{Opcode: 207, PayloadSize: 4}
	// OpIfOpenOverlay opens a full-screen overlay interface. TS ServerGameProt.ts (244): IF_OPENOVERLAY.
	// Call site lands with B4's IF_OPENOVERLAY script op.
	OpIfOpenOverlay = Op{Opcode: 158, PayloadSize: 2}
	OpTutOpen       = Op{Opcode: 174, PayloadSize: 2}
	OpTutFlash      = Op{Opcode: 168, PayloadSize: 1}
	OpLogout        = Op{Opcode: 17, PayloadSize: 0}

	// S5f: per-component setters (fire-and-forget wire ops used by IF_SET* opcodes).
	// TS ServerGameProt.ts (244): IF_SETTEXT=154/-2, IF_SETMODEL=245/4,
	// IF_SETNPCHEAD=129/4, IF_SETPLAYERHEAD=108/2, IF_SETANIM=219/4,
	// IF_SETHIDE=123/3, IF_SETOBJECT=164/6, IF_SETCOLOUR=78/4,
	// IF_SETPOSITION=241/6, IF_SETTAB=200/3, IF_SETTAB_ACTIVE=56/1.
	// IF_SETRECOL (103/6) removed at 244 — encoder/model deleted upstream; see PORTING.md §B2/§B4.
	OpIfSetText       = Op{Opcode: 154, PayloadSize: -2}
	OpIfSetModel      = Op{Opcode: 245, PayloadSize: 4}
	OpIfSetNpcHead    = Op{Opcode: 129, PayloadSize: 4}
	OpIfSetPlayerHead = Op{Opcode: 108, PayloadSize: 2}
	OpIfSetAnim       = Op{Opcode: 219, PayloadSize: 4}
	OpIfSetHide       = Op{Opcode: 123, PayloadSize: 3}
	OpIfSetObject     = Op{Opcode: 164, PayloadSize: 6}
	OpIfSetColour     = Op{Opcode: 78, PayloadSize: 4}
	OpIfSetPosition   = Op{Opcode: 241, PayloadSize: 6}
	OpIfSetTab        = Op{Opcode: 200, PayloadSize: 3}
	OpIfSetTabActive  = Op{Opcode: 56, PayloadSize: 1}

	// S5g: dialog suspension. Server sends only the opcode byte to
	// prompt the client to open an "enter a number" count dialog.
	// TS ServerGameProt.ts (244): P_COUNTDIALOG=152/0.
	OpPCountDialog = Op{Opcode: 152, PayloadSize: 0}

	// Camera control. TS ServerGameProt.ts (244): CAM_RESET=53/0.
	// Sent by the CAM_RESET script opcode to reset the client's camera.
	OpCamReset = Op{Opcode: 53, PayloadSize: 0}
	// Camera control. TS ServerGameProt.ts (244): CAM_SHAKE=50/4, payload p1×4.
	// Sent by the CAM_SHAKE script opcode for cutscene camera shake.
	OpCamShake = Op{Opcode: 50, PayloadSize: 4}
	// Camera control. TS ServerGameProt.ts (244): CAM_MOVETO=12/6, payload
	// p1(localX) p1(localZ) p2(height) p1(rotationSpeed) p1(rotationMultiplier).
	// Coords are zone-relative against player.originX/originZ at drain-time
	// (TS NetworkPlayer.ts:245-246). Sent by the CAM_MOVETO script opcode.
	OpCamMoveTo = Op{Opcode: 12, PayloadSize: 6}
	// Camera control. TS ServerGameProt.ts (244): CAM_LOOKAT=222/6; same payload
	// shape as OpCamMoveTo. Sent by the CAM_LOOKAT script opcode.
	OpCamLookAt = Op{Opcode: 222, PayloadSize: 6}

	// HINT_ARROW — directs the client to render a hint indicator pointing
	// at an NPC, player, tile, or to clear. All 5 TS HintArrowEncoder
	// type variants are wired: type=1 NPC (NAI-37), type=2..6 TILE (NAI-39),
	// type=10 PL (NAI-39), type=-1 STOP (NAI-39).
	// TS ServerGameProt.ts (244): HINT_ARROW=49/6.
	OpHintArrow = Op{Opcode: 49, PayloadSize: 6}

	// REBUILD_NORMAL: 244 wire = fixed 4 bytes: p2(zoneX) p2(zoneZ).
	// The 225 per-mapsquare CRC loop is gone; 244 client fetches maps via
	// OnDemand (Bundle 3). TS RebuildNormalEncoder.ts (244): test() returns 4.
	// TS ServerGameProt.ts (244): REBUILD_NORMAL=165/4.
	OpRebuildNormal    = Op{Opcode: 165, PayloadSize: 4}
	OpUpdateInvFull    = Op{Opcode: 72, PayloadSize: -2}
	OpUpdateInvPartial = Op{Opcode: 132, PayloadSize: -2}
	OpPlayerInfo       = Op{Opcode: 86, PayloadSize: -2}
	OpNpcInfo          = Op{Opcode: 244, PayloadSize: -2}

	OpUpdateStat      = Op{Opcode: 24, PayloadSize: 6}
	OpUpdateRunEnergy = Op{Opcode: 177, PayloadSize: 1}
	// Per-player run-weight (kg). Emitted from NetworkPlayer.updateInvs when an
	// inv with RunWeight=true is dirtied or first-seen. Mirrors TS
	// ServerGameProt.ts (244): UPDATE_RUNWEIGHT=160/2.
	OpUpdateRunWeight = Op{Opcode: 160, PayloadSize: 2}
	// OpSetMultiway tells the client to show or hide the multi-combat
	// overlay icon (top-right of the chatbox). Sent on transitions across
	// multi-combat zone boundaries from updateBuildArea. 1-byte payload
	// (pbool): 0 to hide overlay (left a multi zone), 1 to show overlay
	// (entered a multi zone). Mirrors TS ServerGameProt.ts (244): SET_MULTIWAY=97/1.
	OpSetMultiway           = Op{Opcode: 97, PayloadSize: 1}
	OpUpdateInvStopTransmit = Op{Opcode: 162, PayloadSize: 2}

	// Per-player VARP sync. VARP_SMALL fits values in [-128, 127];
	// VARP_LARGE carries full int32 range.
	// TS ServerGameProt.ts (244): VARP_SMALL=236/3, VARP_LARGE=226/6.
	OpVarpSmall = Op{Opcode: 236, PayloadSize: 3}
	OpVarpLarge = Op{Opcode: 226, PayloadSize: 6}

	// TS ServerGameProt.ts (244): UPDATE_ZONE_PARTIAL_FOLLOWS=94/2,
	// UPDATE_ZONE_FULL_FOLLOWS=131/2, UPDATE_ZONE_PARTIAL_ENCLOSED=233/-2.
	OpUpdateZonePartialFollows  = Op{Opcode: 94, PayloadSize: 2}
	OpUpdateZoneFullFollows     = Op{Opcode: 131, PayloadSize: 2}
	OpUpdateZonePartialEnclosed = Op{Opcode: 233, PayloadSize: -2}

	// Zone-nested opcodes, reused as top-level packets for per-player
	// UpdateZonePartialFollows delivery. Sizes match the Java client's
	// SERVERPROT_SIZES at the matching indices.
	// TS ServerGameZoneProt.ts (244): LOC_ADD_CHANGE=232/4, LOC_ANIM=155/4,
	// LOC_DEL=125/2, LOC_MERGE=29/14, MAP_ANIM=198/6, MAP_PROJANIM=137/15,
	// OBJ_ADD=234/5, OBJ_COUNT=209/7, OBJ_DEL=39/3, OBJ_REVEAL=69/7.
	OpLocAddChange = Op{Opcode: 232, PayloadSize: 4}
	OpLocAnim      = Op{Opcode: 155, PayloadSize: 4}
	OpLocDel       = Op{Opcode: 125, PayloadSize: 2}
	OpLocMerge     = Op{Opcode: 29, PayloadSize: 14}
	OpMapAnim      = Op{Opcode: 198, PayloadSize: 6}
	OpMapProjAnim  = Op{Opcode: 137, PayloadSize: 15}
	OpObjAdd       = Op{Opcode: 234, PayloadSize: 5}
	OpObjCount     = Op{Opcode: 209, PayloadSize: 7}
	OpObjDel       = Op{Opcode: 39, PayloadSize: 3}
	OpObjReveal    = Op{Opcode: 69, PayloadSize: 7}

	// DATA_LAND/DATA_LOC/DATA_LAND_DONE/DATA_LOC_DONE were removed at 244:
	// the client fetches maps via OnDemand (engine OnDemand.ts, Bundle 3)
	// instead of REBUILD_GETMAPS-driven streaming.

	// Interaction (sub-spec 6a). TS ServerGameProt.ts (244): UNSET_MAP_FLAG=62/0.
	OpUnsetMapFlag = Op{Opcode: 62, PayloadSize: 0}

	// RuneScript S2 — chat output emitted by the MES opcode.
	// TS ServerGameProt.ts (244): MESSAGE_GAME=95/-1.
	OpMessageGame = Op{Opcode: 95, PayloadSize: -1}

	// MIDI client-audio packets. 244 wire: MIDI_SONG = p2(id); MIDI_JINGLE = p2(id) p2(delay).
	// The name+crc+length blob is gone; client now fetches by pack id via OnDemand.
	// Name→id lookup via midiIDByName() returns -1 until B3 MidiPack lands.
	// TS ServerGameProt.ts (244): MIDI_SONG=240/2, MIDI_JINGLE=173/4.
	OpMidiSong   = Op{Opcode: 240, PayloadSize: 2}
	OpMidiJingle = Op{Opcode: 173, PayloadSize: 4}

	// Sound-effect packet. TS ServerGameProt.ts (244): SYNTH_SOUND=151/5.
	// SYNTH_SOUND plays a short synthesized sound effect; payload is
	// fixed 5 bytes: p2(synth) p1(loops) p2(delay) per
	// SynthSoundEncoder.ts:9-13. Wired from the SOUND_SYNTH (2104)
	// script opcode via (*Player).PlaySynth.
	OpSynthSound = Op{Opcode: 151, PayloadSize: 5}

	// Input-tracking signals — server tells client to start/stop sending
	// EVENT_TRACKING blobs (op 81). NAI-73; mirrors TS ServerGameProt.ts (244):
	// ENABLE_TRACKING=22/0, FINISH_TRACKING=60/0.
	OpEnableTracking = Op{Opcode: 22, PayloadSize: 0}
	OpFinishTracking = Op{Opcode: 60, PayloadSize: 0}

	// OpLastLoginInfo: 244 wire = p4+p2+p1+p2+pbool = 10 bytes.
	// Carries previous-login telemetry the client renders on the welcome screen.
	// TS ServerGameProt.ts (244): LAST_LOGIN_INFO=44/10.
	OpLastLoginInfo = Op{Opcode: 44, PayloadSize: 10}

	// OpUpdatePid: 244 wire = p2(uid) + pbool(members) = 3 bytes.
	// Carries the player's server-side slot and world-members flag.
	// TS UpdatePidEncoder.ts (244): p2(uid) pbool(members).
	// TS ServerGameProt.ts (244): UPDATE_PID=210/3.
	OpUpdatePid = Op{Opcode: 210, PayloadSize: 3}

	// OpResetAnims tells the client to clear all animation layers on the
	// local player. Zero-byte payload. Emitted at onLogin (after varp
	// resync) and onReconnect (after per-stat UpdateStat/UpdateRunEnergy).
	// TS ServerGameProt.ts (244): RESET_ANIMS=242/0.
	OpResetAnims = Op{Opcode: 242, PayloadSize: 0}

	// OpResetClientVarCache tells the client to drop its cached varp
	// values so the next varp packets become authoritative. Emitted at
	// onLogin and onReconnect immediately before the varp transmit-loop.
	// Zero-byte payload. TS ServerGameProt.ts (244): RESET_CLIENT_VARCACHE=87/0.
	OpResetClientVarCache = Op{Opcode: 87, PayloadSize: 0}

	// OpUpdateRebootTimer carries the number of game ticks (600ms each)
	// remaining until the world reboots. Sent broadcast by
	// Server.rebootTimer and to each connecting player at processLogins
	// if a shutdown is pending. Fixed 2-byte payload: p2(ticks).
	// TS ServerGameProt.ts (244): UPDATE_REBOOT_TIMER=85/2.
	OpUpdateRebootTimer = Op{Opcode: 85, PayloadSize: 2}

	// OpUpdateFriendList carries one friend-entry update. Fixed 9-byte
	// payload: p8(username37) + p1(worldId). worldId == 0 means the friend
	// is offline / hidden. Emitted once per entry by the friends-server
	// dispatcher (one packet per FriendEntry in the FriendlistUpdate batch).
	// TS ServerGameProt.ts (244): UPDATE_FRIENDLIST=70/9.
	OpUpdateFriendList = Op{Opcode: 70, PayloadSize: 9}

	// OpUpdateIgnoreList carries the complete ignorelist snapshot. Variable
	// 2-byte-length-prefixed payload: p8(username37) × N. Emitted on every
	// ignorelist mutation; the entire list is re-sent rather than a delta.
	// TS ServerGameProt.ts (244): UPDATE_IGNORELIST=7/-2.
	OpUpdateIgnoreList = Op{Opcode: 7, PayloadSize: -2}

	// OpChatFilterSettings carries the player's chat-filter mode triple.
	// Fixed 3-byte payload: p1(publicChat) + p1(privateChat) + p1(tradeDuel).
	// Emitted once at onLogin (before UpdatePid). TS ServerGameProt.ts (244):
	// CHAT_FILTER_SETTINGS=9/3.
	OpChatFilterSettings = Op{Opcode: 9, PayloadSize: 3}

	// OpMessagePrivate carries one inbound private-chat delivery to the
	// recipient. Variable 1-byte-length-prefixed payload:
	// p8(fromUsername37) + p4(pmId) + p1(staffLvlAdjusted) +
	// WordPack.pack(chat). staffLvlAdjusted = staffLvl > 0 ? staffLvl + 1 :
	// staffLvl. Emitted by the friends-server dispatcher on
	// PrivateMessageDelivery. TS ServerGameProt.ts (244): MESSAGE_PRIVATE=30/-1.
	OpMessagePrivate = Op{Opcode: 30, PayloadSize: -1}
)

// OpEntry pairs a server opcode with the symbolic name used by the
// external decoder. Names match the TS ServerProt enum.
type OpEntry struct {
	Name string
	Op   Op
}

// AllOps returns every declared server-side packet operation. The order
// is not stable; callers must not rely on it. Used by external decoders
// to build the rev244 outbound table without each consumer manually
// enumerating the constants.
func AllOps() []OpEntry {
	return []OpEntry{
		// modal interface
		{"IF_CLOSE", OpIfClose},
		{"IF_OPENMAIN", OpIfOpenMain},
		{"IF_OPENCHAT", OpIfOpenChat},
		{"IF_OPENSIDE", OpIfOpenSide},
		{"IF_OPENMAIN_SIDE", OpIfOpenMainSide},
		{"IF_OPENOVERLAY", OpIfOpenOverlay},
		{"TUT_OPEN", OpTutOpen},
		{"TUT_FLASH", OpTutFlash},
		{"LOGOUT", OpLogout},
		// interface setters
		{"IF_SETTEXT", OpIfSetText},
		{"IF_SETMODEL", OpIfSetModel},
		{"IF_SETNPCHEAD", OpIfSetNpcHead},
		{"IF_SETPLAYERHEAD", OpIfSetPlayerHead},
		{"IF_SETANIM", OpIfSetAnim},
		{"IF_SETHIDE", OpIfSetHide},
		{"IF_SETOBJECT", OpIfSetObject},
		{"IF_SETCOLOUR", OpIfSetColour},
		{"IF_SETPOSITION", OpIfSetPosition},
		{"IF_SETTAB", OpIfSetTab},
		{"IF_SETTAB_ACTIVE", OpIfSetTabActive},
		// dialog
		{"P_COUNTDIALOG", OpPCountDialog},
		// camera
		{"CAM_RESET", OpCamReset},
		{"CAM_SHAKE", OpCamShake},
		{"CAM_MOVETO", OpCamMoveTo},
		{"CAM_LOOKAT", OpCamLookAt},
		// hint
		{"HINT_ARROW", OpHintArrow},
		// world rebuild + entity info
		{"REBUILD_NORMAL", OpRebuildNormal},
		{"UPDATE_INV_FULL", OpUpdateInvFull},
		{"UPDATE_INV_PARTIAL", OpUpdateInvPartial},
		{"PLAYER_INFO", OpPlayerInfo},
		{"NPC_INFO", OpNpcInfo},
		// stats + energy
		{"UPDATE_STAT", OpUpdateStat},
		{"UPDATE_RUNENERGY", OpUpdateRunEnergy},
		{"UPDATE_RUNWEIGHT", OpUpdateRunWeight},
		// misc
		{"SET_MULTIWAY", OpSetMultiway},
		{"UPDATE_INV_STOP_TRANSMIT", OpUpdateInvStopTransmit},
		// varps
		{"VARP_SMALL", OpVarpSmall},
		{"VARP_LARGE", OpVarpLarge},
		// zone updates
		{"UPDATE_ZONE_PARTIAL_FOLLOWS", OpUpdateZonePartialFollows},
		{"UPDATE_ZONE_FULL_FOLLOWS", OpUpdateZoneFullFollows},
		{"UPDATE_ZONE_PARTIAL_ENCLOSED", OpUpdateZonePartialEnclosed},
		// zone-nested opcodes
		{"LOC_ADD_CHANGE", OpLocAddChange},
		{"LOC_ANIM", OpLocAnim},
		{"LOC_DEL", OpLocDel},
		{"LOC_MERGE", OpLocMerge},
		{"MAP_ANIM", OpMapAnim},
		{"MAP_PROJANIM", OpMapProjAnim},
		{"OBJ_ADD", OpObjAdd},
		{"OBJ_COUNT", OpObjCount},
		{"OBJ_DEL", OpObjDel},
		{"OBJ_REVEAL", OpObjReveal},
		// interaction
		{"UNSET_MAP_FLAG", OpUnsetMapFlag},
		// chat
		{"MESSAGE_GAME", OpMessageGame},
		// audio
		{"MIDI_SONG", OpMidiSong},
		{"MIDI_JINGLE", OpMidiJingle},
		{"SYNTH_SOUND", OpSynthSound},
		// input tracking
		{"ENABLE_TRACKING", OpEnableTracking},
		{"FINISH_TRACKING", OpFinishTracking},
		// login / session
		{"LAST_LOGIN_INFO", OpLastLoginInfo},
		{"UPDATE_PID", OpUpdatePid},
		{"RESET_ANIMS", OpResetAnims},
		{"RESET_CLIENT_VARCACHE", OpResetClientVarCache},
		{"UPDATE_REBOOT_TIMER", OpUpdateRebootTimer},
		// social
		{"UPDATE_FRIENDLIST", OpUpdateFriendList},
		{"UPDATE_IGNORELIST", OpUpdateIgnoreList},
		{"CHAT_FILTER_SETTINGS", OpChatFilterSettings},
		{"MESSAGE_PRIVATE", OpMessagePrivate},
	}
}
