package server

// Op describes a server→client game packet opcode.
type Op struct {
	Opcode      byte
	PayloadSize int // 0=fixed-zero, 2=fixed-2, 4=fixed-4, -1=1-byte-len, -2=2-byte-len
}

// Modal interface opcodes and logout.
// TS ServerGameProt.ts (254): IF_CLOSE=174/0, IF_OPENMAIN=197/2, IF_OPENCHAT=141/2,
// IF_OPENSIDE=187/2, IF_OPENMAIN_SIDE=249/4, IF_OPENOVERLAY=85/2, LOGOUT=21/0.
var (
	OpIfClose        = Op{Opcode: 174, PayloadSize: 0}
	OpIfOpenMain     = Op{Opcode: 197, PayloadSize: 2}
	OpIfOpenChat     = Op{Opcode: 141, PayloadSize: 2}
	OpIfOpenSide     = Op{Opcode: 187, PayloadSize: 2}
	OpIfOpenMainSide = Op{Opcode: 249, PayloadSize: 4}
	// OpIfOpenOverlay opens a full-screen overlay interface. TS ServerGameProt.ts (254): IF_OPENOVERLAY=85/2.
	// Call site lands with B4's IF_OPENOVERLAY script op.
	OpIfOpenOverlay = Op{Opcode: 85, PayloadSize: 2}
	OpTutOpen       = Op{Opcode: 239, PayloadSize: 2}
	OpTutFlash      = Op{Opcode: 58, PayloadSize: 1}
	OpLogout        = Op{Opcode: 21, PayloadSize: 0}

	// S5f: per-component setters (fire-and-forget wire ops used by IF_SET* opcodes).
	// TS ServerGameProt.ts (254): IF_SETTEXT=41/-2, IF_SETMODEL=211/4,
	// IF_SETNPCHEAD=3/4, IF_SETPLAYERHEAD=161/2, IF_SETANIM=95/4,
	// IF_SETHIDE=227/3, IF_SETOBJECT=222/6, IF_SETCOLOUR=38/4,
	// IF_SETPOSITION=27/6, IF_SETSCROLLPOS=14/4,
	// IF_SETTAB=91/3, IF_SETTAB_ACTIVE=138/1.
	// IF_SETRECOL removed at 244 — encoder/model deleted upstream; see PORTING.md §B2/§B4.
	OpIfSetText       = Op{Opcode: 41, PayloadSize: -2}
	OpIfSetModel      = Op{Opcode: 211, PayloadSize: 4}
	OpIfSetNpcHead    = Op{Opcode: 3, PayloadSize: 4}
	OpIfSetPlayerHead = Op{Opcode: 161, PayloadSize: 2}
	OpIfSetAnim       = Op{Opcode: 95, PayloadSize: 4}
	OpIfSetHide       = Op{Opcode: 227, PayloadSize: 3}
	OpIfSetObject     = Op{Opcode: 222, PayloadSize: 6}
	OpIfSetColour     = Op{Opcode: 38, PayloadSize: 4}
	OpIfSetPosition   = Op{Opcode: 27, PayloadSize: 6}
	// IF_SETSCROLLPOS sets a layer component's vertical scrollbar position.
	// 4-byte payload: p2(component) p2(y). New in 245.2 —
	// TS IfSetScrollPosEncoder.ts @43e02957, ServerGameProt.ts (254): IF_SETSCROLLPOS=14/4.
	OpIfSetScrollPos = Op{Opcode: 14, PayloadSize: 4}
	OpIfSetTab       = Op{Opcode: 91, PayloadSize: 3}
	OpIfSetTabActive = Op{Opcode: 138, PayloadSize: 1}

	// S5g: dialog suspension. Server sends only the opcode byte to
	// prompt the client to open an "enter a number" count dialog.
	// TS ServerGameProt.ts (254): P_COUNTDIALOG=5/0.
	OpPCountDialog = Op{Opcode: 5, PayloadSize: 0}

	// Camera control. TS ServerGameProt.ts (254): CAM_RESET=167/0.
	// Sent by the CAM_RESET script opcode to reset the client's camera.
	OpCamReset = Op{Opcode: 167, PayloadSize: 0}
	// Camera control. TS ServerGameProt.ts (254): CAM_SHAKE=225/4, payload p1×4.
	// Sent by the CAM_SHAKE script opcode for cutscene camera shake.
	OpCamShake = Op{Opcode: 225, PayloadSize: 4}
	// Camera control. TS ServerGameProt.ts (254): CAM_MOVETO=55/6, payload
	// p1(localX) p1(localZ) p2(height) p1(rotationSpeed) p1(rotationMultiplier).
	// Coords are zone-relative against player.originX/originZ at drain-time
	// (TS NetworkPlayer.ts:245-246). Sent by the CAM_MOVETO script opcode.
	OpCamMoveTo = Op{Opcode: 55, PayloadSize: 6}
	// Camera control. TS ServerGameProt.ts (254): CAM_LOOKAT=0/6; same payload
	// shape as OpCamMoveTo. Sent by the CAM_LOOKAT script opcode.
	OpCamLookAt = Op{Opcode: 0, PayloadSize: 6}

	// HINT_ARROW — directs the client to render a hint indicator pointing
	// at an NPC, player, tile, or to clear. All 5 TS HintArrowEncoder
	// type variants are wired: type=1 NPC (NAI-37), type=2..6 TILE (NAI-39),
	// type=10 PL (NAI-39), type=-1 STOP (NAI-39).
	// TS ServerGameProt.ts (254): HINT_ARROW=64/6.
	OpHintArrow = Op{Opcode: 64, PayloadSize: 6}

	// REBUILD_NORMAL: wire = fixed 4 bytes: p2(zoneX) p2(zoneZ).
	// The 225 per-mapsquare CRC loop is gone; the client fetches maps via
	// OnDemand (Bundle 3). TS RebuildNormalEncoder.ts: test() returns 4.
	// TS ServerGameProt.ts (254): REBUILD_NORMAL=209/4.
	OpRebuildNormal    = Op{Opcode: 209, PayloadSize: 4}
	OpUpdateInvFull    = Op{Opcode: 28, PayloadSize: -2}
	OpUpdateInvPartial = Op{Opcode: 170, PayloadSize: -2}
	OpPlayerInfo       = Op{Opcode: 87, PayloadSize: -2}
	OpNpcInfo          = Op{Opcode: 123, PayloadSize: -2}

	OpUpdateStat      = Op{Opcode: 136, PayloadSize: 6}
	OpUpdateRunEnergy = Op{Opcode: 94, PayloadSize: 1}
	// Per-player run-weight (kg). Emitted from NetworkPlayer.updateInvs when an
	// inv with RunWeight=true is dirtied or first-seen. Mirrors TS
	// ServerGameProt.ts (254): UPDATE_RUNWEIGHT=164/2.
	OpUpdateRunWeight = Op{Opcode: 164, PayloadSize: 2}
	// OpSetMultiway tells the client to show or hide the multi-combat
	// overlay icon (top-right of the chatbox). Sent on transitions across
	// multi-combat zone boundaries from updateBuildArea. 1-byte payload
	// (pbool): 0 to hide overlay (left a multi zone), 1 to show overlay
	// (entered a multi zone). Mirrors TS ServerGameProt.ts (254): SET_MULTIWAY=75/1.
	OpSetMultiway           = Op{Opcode: 75, PayloadSize: 1}
	OpUpdateInvStopTransmit = Op{Opcode: 168, PayloadSize: 2}

	// Per-player VARP sync. VARP_SMALL fits values in [-128, 127];
	// VARP_LARGE carries full int32 range.
	// TS ServerGameProt.ts (254): VARP_SMALL=186/3, VARP_LARGE=196/6.
	OpVarpSmall = Op{Opcode: 186, PayloadSize: 3}
	OpVarpLarge = Op{Opcode: 196, PayloadSize: 6}

	// TS ServerGameProt.ts (254): UPDATE_ZONE_PARTIAL_FOLLOWS=173/2,
	// UPDATE_ZONE_FULL_FOLLOWS=159/2, UPDATE_ZONE_PARTIAL_ENCLOSED=61/-2.
	OpUpdateZonePartialFollows  = Op{Opcode: 173, PayloadSize: 2}
	OpUpdateZoneFullFollows     = Op{Opcode: 159, PayloadSize: 2}
	OpUpdateZonePartialEnclosed = Op{Opcode: 61, PayloadSize: -2}

	// Zone-nested opcodes, reused as top-level packets for per-player
	// UpdateZonePartialFollows delivery. Sizes match the Java client's
	// SERVERPROT_SIZES at the matching indices.
	// TS ServerGameZoneProt.ts (254): LOC_ADD_CHANGE=70/4, LOC_ANIM=30/4,
	// LOC_DEL=88/2, LOC_MERGE=218/14, MAP_ANIM=114/6, MAP_PROJANIM=37/15,
	// OBJ_ADD=120/5, OBJ_COUNT=98/7, OBJ_DEL=115/3, OBJ_REVEAL=8/7.
	OpLocAddChange = Op{Opcode: 70, PayloadSize: 4}
	OpLocAnim      = Op{Opcode: 30, PayloadSize: 4}
	OpLocDel       = Op{Opcode: 88, PayloadSize: 2}
	OpLocMerge     = Op{Opcode: 218, PayloadSize: 14}
	OpMapAnim      = Op{Opcode: 114, PayloadSize: 6}
	OpMapProjAnim  = Op{Opcode: 37, PayloadSize: 15}
	OpObjAdd       = Op{Opcode: 120, PayloadSize: 5}
	OpObjCount     = Op{Opcode: 98, PayloadSize: 7}
	OpObjDel       = Op{Opcode: 115, PayloadSize: 3}
	OpObjReveal    = Op{Opcode: 8, PayloadSize: 7}

	// DATA_LAND/DATA_LOC/DATA_LAND_DONE/DATA_LOC_DONE were removed at 244:
	// the client fetches maps via OnDemand (engine OnDemand.ts, Bundle 3)
	// instead of REBUILD_GETMAPS-driven streaming.

	// Interaction (sub-spec 6a). TS ServerGameProt.ts (254): UNSET_MAP_FLAG=108/0.
	OpUnsetMapFlag = Op{Opcode: 108, PayloadSize: 0}

	// RuneScript S2 — chat output emitted by the MES opcode.
	// TS ServerGameProt.ts (254): MESSAGE_GAME=73/-1.
	OpMessageGame = Op{Opcode: 73, PayloadSize: -1}

	// MIDI client-audio packets. Wire: MIDI_SONG = p2(id); MIDI_JINGLE = p2(id) p2(delay).
	// The name+crc+length blob is gone; client now fetches by pack id via OnDemand.
	// Name→id lookup via midiIDByName() returns -1 until B3 MidiPack lands.
	// TS ServerGameProt.ts (254): MIDI_SONG=163/2, MIDI_JINGLE=242/4.
	OpMidiSong   = Op{Opcode: 163, PayloadSize: 2}
	OpMidiJingle = Op{Opcode: 242, PayloadSize: 4}

	// Sound-effect packet. TS ServerGameProt.ts (254): SYNTH_SOUND=25/5.
	// SYNTH_SOUND plays a short synthesized sound effect; payload is
	// fixed 5 bytes: p2(synth) p1(loops) p2(delay) per
	// SynthSoundEncoder.ts:9-13. Wired from the SOUND_SYNTH (2104)
	// script opcode via (*Player).PlaySynth.
	OpSynthSound = Op{Opcode: 25, PayloadSize: 5}

	// Input-tracking signals — server tells client to start/stop sending
	// EVENT_TRACKING blobs (op 81). NAI-73; mirrors TS ServerGameProt.ts (254):
	// ENABLE_TRACKING=251/0, FINISH_TRACKING=29/0.
	OpEnableTracking = Op{Opcode: 251, PayloadSize: 0}
	OpFinishTracking = Op{Opcode: 29, PayloadSize: 0}

	// OpLastLoginInfo: wire = p4+p2+p1+p2+pbool = 10 bytes.
	// Carries previous-login telemetry the client renders on the welcome screen.
	// TS ServerGameProt.ts (254): LAST_LOGIN_INFO=146/10.
	OpLastLoginInfo = Op{Opcode: 146, PayloadSize: 10}

	// OpUpdatePid: wire = p2(uid) + pbool(members) = 3 bytes.
	// Carries the player's server-side slot and world-members flag.
	// TS UpdatePidEncoder.ts: p2(uid) pbool(members).
	// TS ServerGameProt.ts (254): UPDATE_PID=213/3.
	OpUpdatePid = Op{Opcode: 213, PayloadSize: 3}

	// OpResetAnims tells the client to clear all animation layers on the
	// local player. Zero-byte payload. Emitted at onLogin (after varp
	// resync) and onReconnect (after per-stat UpdateStat/UpdateRunEnergy).
	// TS ServerGameProt.ts (254): RESET_ANIMS=203/0.
	OpResetAnims = Op{Opcode: 203, PayloadSize: 0}

	// OpResetClientVarCache tells the client to drop its cached varp
	// values so the next varp packets become authoritative. Emitted at
	// onLogin and onReconnect immediately before the varp transmit-loop.
	// Zero-byte payload. TS ServerGameProt.ts (254): RESET_CLIENT_VARCACHE=140/0.
	OpResetClientVarCache = Op{Opcode: 140, PayloadSize: 0}

	// OpUpdateRebootTimer carries the number of game ticks (600ms each)
	// remaining until the world reboots. Sent broadcast by
	// Server.rebootTimer and to each connecting player at processLogins
	// if a shutdown is pending. Fixed 2-byte payload: p2(ticks).
	// TS ServerGameProt.ts (254): UPDATE_REBOOT_TIMER=143/2.
	OpUpdateRebootTimer = Op{Opcode: 143, PayloadSize: 2}

	// OpUpdateFriendList carries one friend-entry update. Fixed 9-byte
	// payload: p8(username37) + p1(worldId). worldId == 0 means the friend
	// is offline / hidden. Emitted once per entry by the friends-server
	// dispatcher (one packet per FriendEntry in the FriendlistUpdate batch).
	// TS ServerGameProt.ts (254): UPDATE_FRIENDLIST=111/9.
	OpUpdateFriendList = Op{Opcode: 111, PayloadSize: 9}

	// OpFriendlistLoaded reports friends-list bootstrap state to the client.
	// 1-byte payload: p1(status) — 0 loading, 1 connecting to friendserver,
	// 2 online (anything else renders "Please wait..."). New in 254 —
	// TS FriendlistLoadedEncoder.ts @43e02957, ServerGameProt.ts: FRIENDLIST_LOADED=255/1.
	OpFriendlistLoaded = Op{Opcode: 255, PayloadSize: 1}

	// OpUpdateIgnoreList carries the complete ignorelist snapshot. Variable
	// 2-byte-length-prefixed payload: p8(username37) × N. Emitted on every
	// ignorelist mutation; the entire list is re-sent rather than a delta.
	// TS ServerGameProt.ts (254): UPDATE_IGNORELIST=63/-2.
	OpUpdateIgnoreList = Op{Opcode: 63, PayloadSize: -2}

	// OpChatFilterSettings carries the player's chat-filter mode triple.
	// Fixed 3-byte payload: p1(publicChat) + p1(privateChat) + p1(tradeDuel).
	// Emitted once at onLogin (before UpdatePid). TS ServerGameProt.ts (254):
	// CHAT_FILTER_SETTINGS=24/3.
	OpChatFilterSettings = Op{Opcode: 24, PayloadSize: 3}

	// OpMessagePrivate carries one inbound private-chat delivery to the
	// recipient. Variable 1-byte-length-prefixed payload:
	// p8(fromUsername37) + p4(pmId) + p1(staffLvlAdjusted) +
	// WordPack.pack(chat). staffLvlAdjusted = staffLvl > 0 ? staffLvl + 1 :
	// staffLvl. Emitted by the friends-server dispatcher on
	// PrivateMessageDelivery. TS ServerGameProt.ts (254): MESSAGE_PRIVATE=60/-1.
	OpMessagePrivate = Op{Opcode: 60, PayloadSize: -1}

	// OpSetPlayerOp sets a right-click player-menu entry. Variable
	// 1-byte-length-prefixed payload: p1(op index) p1(primary) pjstr(text)
	// per SetPlayerOpEncoder.ts (note: the TS model ctor order is
	// (op, text, primary); the wire order is op, primary, text). New in 254 —
	// TS SetPlayerOpEncoder.ts @43e02957, ServerGameProt.ts: SET_PLAYER_OP=204/-1.
	OpSetPlayerOp = Op{Opcode: 204, PayloadSize: -1}
)

// OpEntry pairs a server opcode with the symbolic name used by the
// external decoder. Names match the TS ServerProt enum.
type OpEntry struct {
	Name string
	Op   Op
}

// AllOps returns every declared server-side packet operation. The order
// is not stable; callers must not rely on it. Used by external decoders
// to build the rev254 outbound table without each consumer manually
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
		{"IF_SETSCROLLPOS", OpIfSetScrollPos},
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
		{"SET_PLAYER_OP", OpSetPlayerOp},
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
		{"FRIENDLIST_LOADED", OpFriendlistLoaded},
		{"UPDATE_IGNORELIST", OpUpdateIgnoreList},
		{"CHAT_FILTER_SETTINGS", OpChatFilterSettings},
		{"MESSAGE_PRIVATE", OpMessagePrivate},
	}
}
