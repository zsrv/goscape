package server

// Op describes a server→client game packet opcode.
type Op struct {
	Opcode      byte
	PayloadSize int // 0=fixed-zero, 2=fixed-2, 4=fixed-4, -1=1-byte-len, -2=2-byte-len
}

// Modal interface opcodes and logout.
// TS ServerGameProt.ts (274): IF_CLOSE=171/0, IF_OPENMAIN=211/2, IF_OPENCHAT=166/2,
// IF_OPENSIDE=16/2, IF_OPENMAIN_SIDE=158/4, IF_OPENOVERLAY=240/2, LOGOUT=88/0.
var (
	OpIfClose        = Op{Opcode: 171, PayloadSize: 0}
	OpIfOpenMain     = Op{Opcode: 211, PayloadSize: 2}
	OpIfOpenChat     = Op{Opcode: 166, PayloadSize: 2}
	OpIfOpenSide     = Op{Opcode: 16, PayloadSize: 2}
	OpIfOpenMainSide = Op{Opcode: 158, PayloadSize: 4}
	// OpIfOpenOverlay opens a full-screen overlay interface. TS ServerGameProt.ts (274): IF_OPENOVERLAY=240/2.
	// Call site lands with B4's IF_OPENOVERLAY script op.
	OpIfOpenOverlay = Op{Opcode: 240, PayloadSize: 2}
	OpTutOpen       = Op{Opcode: 130, PayloadSize: 2}
	OpTutFlash      = Op{Opcode: 90, PayloadSize: 1}
	OpLogout        = Op{Opcode: 88, PayloadSize: 0}

	// S5f: per-component setters (fire-and-forget wire ops used by IF_SET* opcodes).
	// TS ServerGameProt.ts (274): IF_SETTEXT=44/-2, IF_SETMODEL=129/4,
	// IF_SETNPCHEAD=142/4, IF_SETPLAYERHEAD=192/2, IF_SETANIM=134/4,
	// IF_SETHIDE=10/3, IF_SETOBJECT=28/6, IF_SETCOLOUR=183/4,
	// IF_SETPOSITION=77/6, IF_SETSCROLLPOS=54/4,
	// IF_SETTAB=215/3, IF_SETTAB_ACTIVE=241/1.
	// IF_SETRECOL removed at 244 — encoder/model deleted upstream; see PORTING.md §B2/§B4.
	OpIfSetText       = Op{Opcode: 44, PayloadSize: -2}
	OpIfSetModel      = Op{Opcode: 129, PayloadSize: 4}
	OpIfSetNpcHead    = Op{Opcode: 142, PayloadSize: 4}
	OpIfSetPlayerHead = Op{Opcode: 192, PayloadSize: 2}
	OpIfSetAnim       = Op{Opcode: 134, PayloadSize: 4}
	OpIfSetHide       = Op{Opcode: 10, PayloadSize: 3}
	OpIfSetObject     = Op{Opcode: 28, PayloadSize: 6}
	OpIfSetColour     = Op{Opcode: 183, PayloadSize: 4}
	OpIfSetPosition   = Op{Opcode: 77, PayloadSize: 6}
	// IF_SETSCROLLPOS sets a layer component's vertical scrollbar position.
	// 4-byte payload: p2(component) p2(y). New in 245.2 —
	// TS IfSetScrollPosEncoder.ts @dee467c8, ServerGameProt.ts (274): IF_SETSCROLLPOS=54/4.
	OpIfSetScrollPos = Op{Opcode: 54, PayloadSize: 4}
	OpIfSetTab       = Op{Opcode: 215, PayloadSize: 3}
	OpIfSetTabActive = Op{Opcode: 241, PayloadSize: 1}

	// S5g: dialog suspension. Server sends only the opcode byte to
	// prompt the client to open an "enter a number" count dialog.
	// TS ServerGameProt.ts (274): P_COUNTDIALOG=210/0.
	OpPCountDialog = Op{Opcode: 210, PayloadSize: 0}

	// Camera control. TS ServerGameProt.ts (274): CAM_RESET=101/0.
	// Sent by the CAM_RESET script opcode to reset the client's camera.
	OpCamReset = Op{Opcode: 101, PayloadSize: 0}
	// Camera control. TS ServerGameProt.ts (274): CAM_SHAKE=64/4, payload p1×4.
	// Sent by the CAM_SHAKE script opcode for cutscene camera shake.
	OpCamShake = Op{Opcode: 64, PayloadSize: 4}
	// Camera control. TS ServerGameProt.ts (274): CAM_MOVETO=200/6, payload
	// p1(localX) p1(localZ) p2(height) p1(rotationSpeed) p1(rotationMultiplier).
	// Coords are zone-relative against player.originX/originZ at drain-time
	// (TS NetworkPlayer.ts:245-246). Sent by the CAM_MOVETO script opcode.
	OpCamMoveTo = Op{Opcode: 200, PayloadSize: 6}
	// Camera control. TS ServerGameProt.ts (274): CAM_LOOKAT=233/6; same payload
	// shape as OpCamMoveTo. Sent by the CAM_LOOKAT script opcode.
	OpCamLookAt = Op{Opcode: 233, PayloadSize: 6}

	// HINT_ARROW — directs the client to render a hint indicator pointing
	// at an NPC, player, tile, or to clear. All 5 TS HintArrowEncoder
	// type variants are wired: type=1 NPC (NAI-37), type=2..6 TILE (NAI-39),
	// type=10 PL (NAI-39), type=-1 STOP (NAI-39).
	// TS ServerGameProt.ts (274): HINT_ARROW=156/6.
	OpHintArrow = Op{Opcode: 156, PayloadSize: 6}

	// REBUILD_NORMAL: wire = fixed 4 bytes: p2(zoneX) p2(zoneZ).
	// The 225 per-mapsquare CRC loop is gone; the client fetches maps via
	// OnDemand (Bundle 3). TS RebuildNormalEncoder.ts: test() returns 4.
	// TS ServerGameProt.ts (274): REBUILD_NORMAL=231/4.
	OpRebuildNormal    = Op{Opcode: 231, PayloadSize: 4}
	OpUpdateInvFull    = Op{Opcode: 106, PayloadSize: -2}
	OpUpdateInvPartial = Op{Opcode: 172, PayloadSize: -2}
	OpPlayerInfo       = Op{Opcode: 167, PayloadSize: -2}
	OpNpcInfo          = Op{Opcode: 197, PayloadSize: -2}

	OpUpdateStat      = Op{Opcode: 105, PayloadSize: 6}
	OpUpdateRunEnergy = Op{Opcode: 83, PayloadSize: 1}
	// Per-player run-weight (kg). Emitted from NetworkPlayer.updateInvs when an
	// inv with RunWeight=true is dirtied or first-seen. Mirrors TS
	// ServerGameProt.ts (274): UPDATE_RUNWEIGHT=67/2.
	OpUpdateRunWeight = Op{Opcode: 67, PayloadSize: 2}
	// OpSetMultiway tells the client to show or hide the multi-combat
	// overlay icon (top-right of the chatbox). Sent on transitions across
	// multi-combat zone boundaries from updateBuildArea. 1-byte payload
	// (pbool): 0 to hide overlay (left a multi zone), 1 to show overlay
	// (entered a multi zone). Mirrors TS ServerGameProt.ts (274): SET_MULTIWAY=207/1.
	OpSetMultiway           = Op{Opcode: 207, PayloadSize: 1}
	OpUpdateInvStopTransmit = Op{Opcode: 227, PayloadSize: 2}

	// Per-player VARP sync. VARP_SMALL fits values in [-128, 127];
	// VARP_LARGE carries full int32 range.
	// TS ServerGameProt.ts (274): VARP_SMALL=203/3, VARP_LARGE=245/6.
	OpVarpSmall = Op{Opcode: 203, PayloadSize: 3}
	OpVarpLarge = Op{Opcode: 245, PayloadSize: 6}

	// TS ServerGameProt.ts (274): UPDATE_ZONE_PARTIAL_FOLLOWS=32/2,
	// UPDATE_ZONE_FULL_FOLLOWS=153/2, UPDATE_ZONE_PARTIAL_ENCLOSED=195/-2.
	OpUpdateZonePartialFollows  = Op{Opcode: 32, PayloadSize: 2}
	OpUpdateZoneFullFollows     = Op{Opcode: 153, PayloadSize: 2}
	OpUpdateZonePartialEnclosed = Op{Opcode: 195, PayloadSize: -2}

	// Zone-nested opcodes, reused as top-level packets for per-player
	// UpdateZonePartialFollows delivery. Sizes match the Java client's
	// SERVERPROT_SIZES at the matching indices.
	// TS ServerGameZoneProt.ts (274): LOC_ADD_CHANGE=138/4, LOC_ANIM=48/4,
	// LOC_DEL=173/2, LOC_MERGE=176/14, MAP_ANIM=85/6, MAP_PROJANIM=107/15,
	// OBJ_ADD=81/5, OBJ_COUNT=95/7, OBJ_DEL=52/3, OBJ_REVEAL=219/7.
	OpLocAddChange = Op{Opcode: 138, PayloadSize: 4}
	OpLocAnim      = Op{Opcode: 48, PayloadSize: 4}
	OpLocDel       = Op{Opcode: 173, PayloadSize: 2}
	OpLocMerge     = Op{Opcode: 176, PayloadSize: 14}
	OpMapAnim      = Op{Opcode: 85, PayloadSize: 6}
	OpMapProjAnim  = Op{Opcode: 107, PayloadSize: 15}
	OpObjAdd       = Op{Opcode: 81, PayloadSize: 5}
	OpObjCount     = Op{Opcode: 95, PayloadSize: 7}
	OpObjDel       = Op{Opcode: 52, PayloadSize: 3}
	OpObjReveal    = Op{Opcode: 219, PayloadSize: 7}

	// DATA_LAND/DATA_LOC/DATA_LAND_DONE/DATA_LOC_DONE were removed at 244:
	// the client fetches maps via OnDemand (engine OnDemand.ts, Bundle 3)
	// instead of REBUILD_GETMAPS-driven streaming.

	// Interaction (sub-spec 6a). TS ServerGameProt.ts (274): UNSET_MAP_FLAG=115/0.
	OpUnsetMapFlag = Op{Opcode: 115, PayloadSize: 0}

	// RuneScript S2 — chat output emitted by the MES opcode.
	// TS ServerGameProt.ts (274): MESSAGE_GAME=161/-1.
	OpMessageGame = Op{Opcode: 161, PayloadSize: -1}

	// MIDI client-audio packets. Wire: MIDI_SONG = p2(id); MIDI_JINGLE = p2(id) p2(delay).
	// The name+crc+length blob is gone; client now fetches by pack id via OnDemand.
	// Name→id resolution happens at compile time since 254 A10 (midi symbol table); the runtime plays by id.
	// TS ServerGameProt.ts (274): MIDI_SONG=23/2, MIDI_JINGLE=15/4.
	OpMidiSong   = Op{Opcode: 23, PayloadSize: 2}
	OpMidiJingle = Op{Opcode: 15, PayloadSize: 4}

	// Sound-effect packet. TS ServerGameProt.ts (274): SYNTH_SOUND=34/5.
	// SYNTH_SOUND plays a short synthesized sound effect; payload is
	// fixed 5 bytes: p2(synth) p1(loops) p2(delay) per
	// SynthSoundEncoder.ts:9-13. Wired from the SOUND_SYNTH (2104)
	// script opcode via (*Player).PlaySynth.
	OpSynthSound = Op{Opcode: 34, PayloadSize: 5}

	// ENABLE_TRACKING / FINISH_TRACKING were deleted at 274: the rows are
	// absent from TS ServerGameProt.ts @dee467c8 (the 254 table had kept
	// them registered with no sender after the InputTracking state machine
	// was replaced by event-based accumulation; 274 drops the wire rows
	// and their encoders entirely).

	// OpLastLoginInfo: wire = p4+p2+p1+p2+pbool = 10 bytes.
	// Carries previous-login telemetry the client renders on the welcome screen.
	// TS ServerGameProt.ts (274): LAST_LOGIN_INFO=91/10.
	OpLastLoginInfo = Op{Opcode: 91, PayloadSize: 10}

	// OpUpdatePid: wire = p2(uid) + pbool(members) = 3 bytes.
	// Carries the player's server-side slot and world-members flag.
	// TS UpdatePidEncoder.ts: p2(uid) pbool(members).
	// TS ServerGameProt.ts (274): UPDATE_PID=133/3.
	OpUpdatePid = Op{Opcode: 133, PayloadSize: 3}

	// OpResetAnims tells the client to clear all animation layers on the
	// local player. Zero-byte payload. Emitted at onLogin (after varp
	// resync) and onReconnect (after per-stat UpdateStat/UpdateRunEnergy).
	// TS ServerGameProt.ts (274): RESET_ANIMS=47/0.
	OpResetAnims = Op{Opcode: 47, PayloadSize: 0}

	// OpResetClientVarCache tells the client to drop its cached varp
	// values so the next varp packets become authoritative. Emitted at
	// onLogin and onReconnect immediately before the varp transmit-loop.
	// Zero-byte payload. TS ServerGameProt.ts (274): RESET_CLIENT_VARCACHE=190/0.
	OpResetClientVarCache = Op{Opcode: 190, PayloadSize: 0}

	// OpUpdateRebootTimer carries the number of game ticks (600ms each)
	// remaining until the world reboots. Sent broadcast by
	// Server.rebootTimer and to each connecting player at processLogins
	// if a shutdown is pending. Fixed 2-byte payload: p2(ticks).
	// TS ServerGameProt.ts (274): UPDATE_REBOOT_TIMER=89/2.
	OpUpdateRebootTimer = Op{Opcode: 89, PayloadSize: 2}

	// OpUpdateFriendList carries one friend-entry update. Fixed 9-byte
	// payload: p8(username37) + p1(worldId). worldId == 0 means the friend
	// is offline / hidden. Emitted once per entry by the friends-server
	// dispatcher (one packet per FriendEntry in the FriendlistUpdate batch).
	// TS ServerGameProt.ts (274): UPDATE_FRIENDLIST=247/9.
	OpUpdateFriendList = Op{Opcode: 247, PayloadSize: 9}

	// OpFriendlistLoaded reports friends-list bootstrap state to the client.
	// 1-byte payload: p1(status) — 0 loading, 1 connecting to friendserver,
	// 2 online (anything else renders "Please wait..."). New in 254 —
	// TS FriendlistLoadedEncoder.ts @dee467c8, ServerGameProt.ts (274): FRIENDLIST_LOADED=185/1.
	OpFriendlistLoaded = Op{Opcode: 185, PayloadSize: 1}

	// OpUpdateIgnoreList carries the complete ignorelist snapshot. Variable
	// 2-byte-length-prefixed payload: p8(username37) × N. Emitted on every
	// ignorelist mutation; the entire list is re-sent rather than a delta.
	// TS ServerGameProt.ts (274): UPDATE_IGNORELIST=3/-2.
	OpUpdateIgnoreList = Op{Opcode: 3, PayloadSize: -2}

	// OpChatFilterSettings carries the player's chat-filter mode triple.
	// Fixed 3-byte payload: p1(publicChat) + p1(privateChat) + p1(tradeDuel).
	// Emitted once at onLogin (before UpdatePid). TS ServerGameProt.ts (274):
	// CHAT_FILTER_SETTINGS=114/3.
	OpChatFilterSettings = Op{Opcode: 114, PayloadSize: 3}

	// OpMessagePrivate carries one inbound private-chat delivery to the
	// recipient. Variable 1-byte-length-prefixed payload:
	// p8(fromUsername37) + p4(pmId) + p1(staffLvlAdjusted) +
	// WordPack.pack(chat). staffLvlAdjusted = staffLvl > 0 ? staffLvl + 1 :
	// staffLvl. Emitted by the friends-server dispatcher on
	// PrivateMessageDelivery. TS ServerGameProt.ts (274): MESSAGE_PRIVATE=235/-1.
	OpMessagePrivate = Op{Opcode: 235, PayloadSize: -1}

	// OpSetPlayerOp sets a right-click player-menu entry. Variable
	// 1-byte-length-prefixed payload: p1(op index) p1(primary) pjstr(text)
	// per SetPlayerOpEncoder.ts (note: the TS model ctor order is
	// (op, text, primary); the wire order is op, primary, text). New in 254 —
	// TS SetPlayerOpEncoder.ts @dee467c8, ServerGameProt.ts (274): SET_PLAYER_OP=17/-1.
	OpSetPlayerOp = Op{Opcode: 17, PayloadSize: -1}

	// MINIMAP_TOGGLE sets the client minimap state. 1-byte payload:
	// p1(type) — 0 normal, 1 click-disabled, 2 blacked out. New in 274 —
	// TS MinimapToggleEncoder.ts @dee467c8, ServerGameProt.ts (274): MINIMAP_TOGGLE=194/1.
	OpMinimapToggle = Op{Opcode: 194, PayloadSize: 1}
)

// OpEntry pairs a server opcode with the symbolic name used by the
// external decoder. Names match the TS ServerProt enum.
type OpEntry struct {
	Name string
	Op   Op
}

// AllOps returns every declared server-side packet operation. The order
// is not stable; callers must not rely on it. Used by external decoders
// to build the rev274 outbound table without each consumer manually
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
		{"MINIMAP_TOGGLE", OpMinimapToggle},
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
		// ENABLE_TRACKING / FINISH_TRACKING rows deleted at 274 (absent
		// from TS ServerGameProt.ts @dee467c8).
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
