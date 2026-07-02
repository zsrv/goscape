package server

// Op describes a server→client game packet opcode.
type Op struct {
	Opcode      byte
	PayloadSize int // 0=fixed-zero, 2=fixed-2, 4=fixed-4, -1=1-byte-len, -2=2-byte-len
}

// Modal interface opcodes and logout.
// TS ServerGameProt.ts (245.2): IF_CLOSE=174/0, IF_OPENMAIN=177/2, IF_OPENCHAT=7/2,
// IF_OPENSIDE=236/2, IF_OPENMAIN_SIDE=229/4, IF_OPENOVERLAY=115/2, LOGOUT=36/0.
var (
	OpIfClose        = Op{Opcode: 174, PayloadSize: 0}
	OpIfOpenMain     = Op{Opcode: 177, PayloadSize: 2}
	OpIfOpenChat     = Op{Opcode: 7, PayloadSize: 2}
	OpIfOpenSide     = Op{Opcode: 236, PayloadSize: 2}
	OpIfOpenMainSide = Op{Opcode: 229, PayloadSize: 4}
	// OpIfOpenOverlay opens a full-screen overlay interface. TS ServerGameProt.ts (245.2): IF_OPENOVERLAY=115/2.
	// Call site lands with B4's IF_OPENOVERLAY script op.
	OpIfOpenOverlay = Op{Opcode: 115, PayloadSize: 2}
	OpTutOpen       = Op{Opcode: 152, PayloadSize: 2}
	OpTutFlash      = Op{Opcode: 132, PayloadSize: 1}
	OpLogout        = Op{Opcode: 36, PayloadSize: 0}

	// S5f: per-component setters (fire-and-forget wire ops used by IF_SET* opcodes).
	// TS ServerGameProt.ts (245.2): IF_SETTEXT=32/-2, IF_SETMODEL=60/4,
	// IF_SETNPCHEAD=76/4, IF_SETPLAYERHEAD=83/2, IF_SETANIM=69/4,
	// IF_SETHIDE=225/3, IF_SETOBJECT=153/6, IF_SETCOLOUR=135/4,
	// IF_SETPOSITION=230/6, IF_SETSCROLLPOS=226/4,
	// IF_SETTAB=29/3, IF_SETTAB_ACTIVE=8/1.
	// IF_SETRECOL removed at 244 — encoder/model deleted upstream; see docs/PORTING.md §B2/§B4.
	OpIfSetText       = Op{Opcode: 32, PayloadSize: -2}
	OpIfSetModel      = Op{Opcode: 60, PayloadSize: 4}
	OpIfSetNpcHead    = Op{Opcode: 76, PayloadSize: 4}
	OpIfSetPlayerHead = Op{Opcode: 83, PayloadSize: 2}
	OpIfSetAnim       = Op{Opcode: 69, PayloadSize: 4}
	OpIfSetHide       = Op{Opcode: 225, PayloadSize: 3}
	OpIfSetObject     = Op{Opcode: 153, PayloadSize: 6}
	OpIfSetColour     = Op{Opcode: 135, PayloadSize: 4}
	OpIfSetPosition   = Op{Opcode: 230, PayloadSize: 6}
	// IF_SETSCROLLPOS sets a layer component's vertical scrollbar position.
	// 4-byte payload: p2(component) p2(y). New in 245.2 —
	// TS IfSetScrollPosEncoder.ts @3c16994c, ServerGameProt.ts: IF_SETSCROLLPOS=226/4.
	OpIfSetScrollPos = Op{Opcode: 226, PayloadSize: 4}
	OpIfSetTab       = Op{Opcode: 29, PayloadSize: 3}
	OpIfSetTabActive = Op{Opcode: 8, PayloadSize: 1}

	// S5g: dialog suspension. Server sends only the opcode byte to
	// prompt the client to open an "enter a number" count dialog.
	// TS ServerGameProt.ts (245.2): P_COUNTDIALOG=56/0.
	OpPCountDialog = Op{Opcode: 56, PayloadSize: 0}

	// Camera control. TS ServerGameProt.ts (245.2): CAM_RESET=134/0.
	// Sent by the CAM_RESET script opcode to reset the client's camera.
	OpCamReset = Op{Opcode: 134, PayloadSize: 0}
	// Camera control. TS ServerGameProt.ts (245.2): CAM_SHAKE=103/4, payload p1×4.
	// Sent by the CAM_SHAKE script opcode for cutscene camera shake.
	OpCamShake = Op{Opcode: 103, PayloadSize: 4}
	// Camera control. TS ServerGameProt.ts (245.2): CAM_MOVETO=86/6, payload
	// p1(localX) p1(localZ) p2(height) p1(rotationSpeed) p1(rotationMultiplier).
	// Coords are zone-relative against player.originX/originZ at drain-time
	// (TS NetworkPlayer.ts:245-246). Sent by the CAM_MOVETO script opcode.
	OpCamMoveTo = Op{Opcode: 86, PayloadSize: 6}
	// Camera control. TS ServerGameProt.ts (245.2): CAM_LOOKAT=123/6; same payload
	// shape as OpCamMoveTo. Sent by the CAM_LOOKAT script opcode.
	OpCamLookAt = Op{Opcode: 123, PayloadSize: 6}

	// HINT_ARROW — directs the client to render a hint indicator pointing
	// at an NPC, player, tile, or to clear. All 5 TS HintArrowEncoder
	// type variants are wired: type=1 NPC (NAI-37), type=2..6 TILE (NAI-39),
	// type=10 PL (NAI-39), type=-1 STOP (NAI-39).
	// TS ServerGameProt.ts (245.2): HINT_ARROW=243/6.
	OpHintArrow = Op{Opcode: 243, PayloadSize: 6}

	// REBUILD_NORMAL: wire = fixed 4 bytes: p2(zoneX) p2(zoneZ).
	// The 225 per-mapsquare CRC loop is gone; the client fetches maps via
	// OnDemand (Bundle 3). TS RebuildNormalEncoder.ts: test() returns 4.
	// TS ServerGameProt.ts (245.2): REBUILD_NORMAL=66/4.
	OpRebuildNormal    = Op{Opcode: 66, PayloadSize: 4}
	OpUpdateInvFull    = Op{Opcode: 156, PayloadSize: -2}
	OpUpdateInvPartial = Op{Opcode: 95, PayloadSize: -2}
	OpPlayerInfo       = Op{Opcode: 161, PayloadSize: -2}
	OpNpcInfo          = Op{Opcode: 105, PayloadSize: -2}

	OpUpdateStat      = Op{Opcode: 110, PayloadSize: 6}
	OpUpdateRunEnergy = Op{Opcode: 208, PayloadSize: 1}
	// Per-player run-weight (kg). Emitted from NetworkPlayer.updateInvs when an
	// inv with RunWeight=true is dirtied or first-seen. Mirrors TS
	// ServerGameProt.ts (245.2): UPDATE_RUNWEIGHT=70/2.
	OpUpdateRunWeight = Op{Opcode: 70, PayloadSize: 2}
	// OpSetMultiway tells the client to show or hide the multi-combat
	// overlay icon (top-right of the chatbox). Sent on transitions across
	// multi-combat zone boundaries from updateBuildArea. 1-byte payload
	// (pbool): 0 to hide overlay (left a multi zone), 1 to show overlay
	// (entered a multi zone). Mirrors TS ServerGameProt.ts (245.2): SET_MULTIWAY=35/1.
	OpSetMultiway           = Op{Opcode: 35, PayloadSize: 1}
	OpUpdateInvStopTransmit = Op{Opcode: 143, PayloadSize: 2}

	// Per-player VARP sync. VARP_SMALL fits values in [-128, 127];
	// VARP_LARGE carries full int32 range.
	// TS ServerGameProt.ts (245.2): VARP_SMALL=192/3, VARP_LARGE=75/6.
	OpVarpSmall = Op{Opcode: 192, PayloadSize: 3}
	OpVarpLarge = Op{Opcode: 75, PayloadSize: 6}

	// TS ServerGameProt.ts (245.2): UPDATE_ZONE_PARTIAL_FOLLOWS=203/2,
	// UPDATE_ZONE_FULL_FOLLOWS=140/2, UPDATE_ZONE_PARTIAL_ENCLOSED=15/-2.
	OpUpdateZonePartialFollows  = Op{Opcode: 203, PayloadSize: 2}
	OpUpdateZoneFullFollows     = Op{Opcode: 140, PayloadSize: 2}
	OpUpdateZonePartialEnclosed = Op{Opcode: 15, PayloadSize: -2}

	// Zone-nested opcodes, reused as top-level packets for per-player
	// UpdateZonePartialFollows delivery. Sizes match the Java client's
	// SERVERPROT_SIZES at the matching indices.
	// TS ServerGameZoneProt.ts (245.2): LOC_ADD_CHANGE=119/4, LOC_ANIM=71/4,
	// LOC_DEL=198/2, LOC_MERGE=188/14, MAP_ANIM=141/6, MAP_PROJANIM=187/15,
	// OBJ_ADD=94/5, OBJ_COUNT=151/7, OBJ_DEL=13/3, OBJ_REVEAL=190/7.
	OpLocAddChange = Op{Opcode: 119, PayloadSize: 4}
	OpLocAnim      = Op{Opcode: 71, PayloadSize: 4}
	OpLocDel       = Op{Opcode: 198, PayloadSize: 2}
	OpLocMerge     = Op{Opcode: 188, PayloadSize: 14}
	OpMapAnim      = Op{Opcode: 141, PayloadSize: 6}
	OpMapProjAnim  = Op{Opcode: 187, PayloadSize: 15}
	OpObjAdd       = Op{Opcode: 94, PayloadSize: 5}
	OpObjCount     = Op{Opcode: 151, PayloadSize: 7}
	OpObjDel       = Op{Opcode: 13, PayloadSize: 3}
	OpObjReveal    = Op{Opcode: 190, PayloadSize: 7}

	// DATA_LAND/DATA_LOC/DATA_LAND_DONE/DATA_LOC_DONE were removed at 244:
	// the client fetches maps via OnDemand (engine OnDemand.ts, Bundle 3)
	// instead of REBUILD_GETMAPS-driven streaming.

	// Interaction (sub-spec 6a). TS ServerGameProt.ts (245.2): UNSET_MAP_FLAG=233/0.
	OpUnsetMapFlag = Op{Opcode: 233, PayloadSize: 0}

	// RuneScript S2 — chat output emitted by the MES opcode.
	// TS ServerGameProt.ts (245.2): MESSAGE_GAME=175/-1.
	OpMessageGame = Op{Opcode: 175, PayloadSize: -1}

	// MIDI client-audio packets. Wire: MIDI_SONG = p2(id); MIDI_JINGLE = p2(id) p2(delay).
	// The name+crc+length blob is gone; client now fetches by pack id via OnDemand.
	// Name→id lookup via midiIDByName() returns -1 until B3 MidiPack lands.
	// TS ServerGameProt.ts (245.2): MIDI_SONG=96/2, MIDI_JINGLE=39/4.
	OpMidiSong   = Op{Opcode: 96, PayloadSize: 2}
	OpMidiJingle = Op{Opcode: 39, PayloadSize: 4}

	// Sound-effect packet. TS ServerGameProt.ts (245.2): SYNTH_SOUND=209/5.
	// SYNTH_SOUND plays a short synthesized sound effect; payload is
	// fixed 5 bytes: p2(synth) p1(loops) p2(delay) per
	// SynthSoundEncoder.ts:9-13. Wired from the SOUND_SYNTH (2104)
	// script opcode via (*Player).PlaySynth.
	OpSynthSound = Op{Opcode: 209, PayloadSize: 5}

	// Input-tracking signals — server tells client to start/stop sending
	// EVENT_TRACKING blobs (op 81). NAI-73; mirrors TS ServerGameProt.ts (245.2):
	// ENABLE_TRACKING=28/0, FINISH_TRACKING=165/0.
	OpEnableTracking = Op{Opcode: 28, PayloadSize: 0}
	OpFinishTracking = Op{Opcode: 165, PayloadSize: 0}

	// OpLastLoginInfo: wire = p4+p2+p1+p2+pbool = 10 bytes.
	// Carries previous-login telemetry the client renders on the welcome screen.
	// TS ServerGameProt.ts (245.2): LAST_LOGIN_INFO=238/10.
	OpLastLoginInfo = Op{Opcode: 238, PayloadSize: 10}

	// OpUpdatePid: wire = p2(uid) + pbool(members) = 3 bytes.
	// Carries the player's server-side slot and world-members flag.
	// TS UpdatePidEncoder.ts: p2(uid) pbool(members).
	// TS ServerGameProt.ts (245.2): UPDATE_PID=49/3.
	OpUpdatePid = Op{Opcode: 49, PayloadSize: 3}

	// OpResetAnims tells the client to clear all animation layers on the
	// local player. Zero-byte payload. Emitted at onLogin (after varp
	// resync) and onReconnect (after per-stat UpdateStat/UpdateRunEnergy).
	// TS ServerGameProt.ts (245.2): RESET_ANIMS=144/0.
	OpResetAnims = Op{Opcode: 144, PayloadSize: 0}

	// OpResetClientVarCache tells the client to drop its cached varp
	// values so the next varp packets become authoritative. Emitted at
	// onLogin and onReconnect immediately before the varp transmit-loop.
	// Zero-byte payload. TS ServerGameProt.ts (245.2): RESET_CLIENT_VARCACHE=25/0.
	OpResetClientVarCache = Op{Opcode: 25, PayloadSize: 0}

	// OpUpdateRebootTimer carries the number of game ticks (600ms each)
	// remaining until the world reboots. Sent broadcast by
	// Server.rebootTimer and to each connecting player at processLogins
	// if a shutdown is pending. Fixed 2-byte payload: p2(ticks).
	// TS ServerGameProt.ts (245.2): UPDATE_REBOOT_TIMER=26/2.
	OpUpdateRebootTimer = Op{Opcode: 26, PayloadSize: 2}

	// OpUpdateFriendList carries one friend-entry update. Fixed 9-byte
	// payload: p8(username37) + p1(worldId). worldId == 0 means the friend
	// is offline / hidden. Emitted once per entry by the friends-server
	// dispatcher (one packet per FriendEntry in the FriendlistUpdate batch).
	// TS ServerGameProt.ts (245.2): UPDATE_FRIENDLIST=109/9.
	OpUpdateFriendList = Op{Opcode: 109, PayloadSize: 9}

	// OpUpdateIgnoreList carries the complete ignorelist snapshot. Variable
	// 2-byte-length-prefixed payload: p8(username37) × N. Emitted on every
	// ignorelist mutation; the entire list is re-sent rather than a delta.
	// TS ServerGameProt.ts (245.2): UPDATE_IGNORELIST=181/-2.
	OpUpdateIgnoreList = Op{Opcode: 181, PayloadSize: -2}

	// OpChatFilterSettings carries the player's chat-filter mode triple.
	// Fixed 3-byte payload: p1(publicChat) + p1(privateChat) + p1(tradeDuel).
	// Emitted once at onLogin (before UpdatePid). TS ServerGameProt.ts (245.2):
	// CHAT_FILTER_SETTINGS=2/3.
	OpChatFilterSettings = Op{Opcode: 2, PayloadSize: 3}

	// OpMessagePrivate carries one inbound private-chat delivery to the
	// recipient. Variable 1-byte-length-prefixed payload:
	// p8(fromUsername37) + p4(pmId) + p1(staffLvlAdjusted) +
	// WordPack.pack(chat). staffLvlAdjusted = staffLvl > 0 ? staffLvl + 1 :
	// staffLvl. Emitted by the friends-server dispatcher on
	// PrivateMessageDelivery. TS ServerGameProt.ts (245.2): MESSAGE_PRIVATE=207/-1.
	OpMessagePrivate = Op{Opcode: 207, PayloadSize: -1}
)

// OpEntry pairs a server opcode with the symbolic name used by the
// external decoder. Names match the TS ServerProt enum.
type OpEntry struct {
	Name string
	Op   Op
}

// AllOps returns every declared server-side packet operation. The order
// is not stable; callers must not rely on it. Used by external decoders
// to build the rev245.2 outbound table without each consumer manually
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
