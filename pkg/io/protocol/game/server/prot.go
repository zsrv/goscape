package server

// Op describes a server→client game packet opcode.
type Op struct {
	Opcode      byte
	PayloadSize int // 0=fixed-zero, 2=fixed-2, 4=fixed-4, -1=1-byte-len, -2=2-byte-len
}

// Modal interface opcodes and logout — sub-spec 1 only.
// Remaining ~40 server opcodes added in sub-specs 2–4.
var (
	OpIfClose        = Op{Opcode: 129, PayloadSize: 0}
	OpIfOpenMain     = Op{Opcode: 168, PayloadSize: 2}
	OpIfOpenChat     = Op{Opcode: 14, PayloadSize: 2}
	OpIfOpenSide     = Op{Opcode: 195, PayloadSize: 2}
	OpIfOpenMainSide = Op{Opcode: 28, PayloadSize: 4}
	OpTutOpen        = Op{Opcode: 185, PayloadSize: 2}
	OpTutFlash       = Op{Opcode: 126, PayloadSize: 1}
	OpLogout         = Op{Opcode: 142, PayloadSize: 0}

	// S5f: per-component setters (fire-and-forget wire ops used by IF_SET* opcodes).
	OpIfSetText       = Op{Opcode: 201, PayloadSize: -2}
	OpIfSetModel      = Op{Opcode: 87, PayloadSize: 4}
	OpIfSetNpcHead    = Op{Opcode: 204, PayloadSize: 4}
	OpIfSetPlayerHead = Op{Opcode: 197, PayloadSize: 2}
	OpIfSetAnim       = Op{Opcode: 146, PayloadSize: 4}
	OpIfSetHide       = Op{Opcode: 26, PayloadSize: 3}
	OpIfSetObject     = Op{Opcode: 46, PayloadSize: 6}
	OpIfSetColour     = Op{Opcode: 2, PayloadSize: 4}
	OpIfSetPosition   = Op{Opcode: 209, PayloadSize: 6}
	OpIfSetRecol      = Op{Opcode: 103, PayloadSize: 6}
	OpIfSetTab        = Op{Opcode: 167, PayloadSize: 3}
	OpIfSetTabActive  = Op{Opcode: 84, PayloadSize: 1}

	// S5g: dialog suspension. Server sends only the opcode byte to
	// prompt the client to open an "enter a number" count dialog.
	OpPCountDialog = Op{Opcode: 243, PayloadSize: 0}

	// Camera control. TS ServerGameProt.CAM_RESET = 239, payload 0.
	// Sent by the CAM_RESET script opcode to reset the client's camera.
	OpCamReset = Op{Opcode: 239, PayloadSize: 0}
	// Camera control. TS ServerGameProt.CAM_SHAKE = (13, 4), payload p1×4.
	// Sent by the CAM_SHAKE script opcode for cutscene camera shake.
	OpCamShake = Op{Opcode: 13, PayloadSize: 4}
	// Camera control. TS ServerGameProt.CAM_MOVETO = (3, 6), payload
	// p1(localX) p1(localZ) p2(height) p1(rotationSpeed) p1(rotationMultiplier).
	// Coords are zone-relative against player.originX/originZ at drain-time
	// (TS NetworkPlayer.ts:245-246). Sent by the CAM_MOVETO script opcode.
	OpCamMoveTo = Op{Opcode: 3, PayloadSize: 6}
	// Camera control. TS ServerGameProt.CAM_LOOKAT = (74, 6); same payload
	// shape as OpCamMoveTo. Sent by the CAM_LOOKAT script opcode.
	OpCamLookAt = Op{Opcode: 74, PayloadSize: 6}

	// HINT_ARROW — directs the client to render a hint indicator pointing
	// at an NPC, player, tile, or to clear. All 5 TS HintArrowEncoder
	// type variants are wired: type=1 NPC (NAI-37), type=2..6 TILE (NAI-39),
	// type=10 PL (NAI-39), type=-1 STOP (NAI-39).
	// TS ServerGameProt.HINT_ARROW = (25, 6).
	OpHintArrow = Op{Opcode: 25, PayloadSize: 6}

	OpRebuildNormal    = Op{Opcode: 237, PayloadSize: -2}
	OpUpdateInvFull    = Op{Opcode: 98, PayloadSize: -2}
	OpUpdateInvPartial = Op{Opcode: 213, PayloadSize: -2}
	OpPlayerInfo       = Op{Opcode: 184, PayloadSize: -2}
	OpNpcInfo          = Op{Opcode: 1, PayloadSize: -2}

	OpUpdateStat      = Op{Opcode: 44, PayloadSize: 6}
	OpUpdateRunEnergy = Op{Opcode: 68, PayloadSize: 1}
	// Per-player run-weight (kg). Emitted from NetworkPlayer.updateInvs when an
	// inv with RunWeight=true is dirtied or first-seen. Mirrors TS
	// ServerGameProt.UPDATE_RUNWEIGHT (opcode 22, 2-byte payload).
	OpUpdateRunWeight = Op{Opcode: 22, PayloadSize: 2}
	// OpSetMultiway tells the client to show or hide the multi-combat
	// overlay icon (top-right of the chatbox). Sent on transitions across
	// multi-combat zone boundaries from updateBuildArea. 1-byte payload
	// (pbool): 0 to hide overlay (left a multi zone), 1 to show overlay
	// (entered a multi zone). Mirrors TS ServerGameProt.SET_MULTIWAY
	// (opcode 254, size 1) and SetMultiwayEncoder (`buf.pbool(message.hidden)`)
	// at Engine-TS/src/network/game/server/codec/SetMultiwayEncoder.ts.
	OpSetMultiway           = Op{Opcode: 254, PayloadSize: 1}
	OpUpdateInvStopTransmit = Op{Opcode: 15, PayloadSize: 2}

	// Per-player VARP sync. VARP_SMALL fits values in [-128, 127];
	// VARP_LARGE carries full int32 range.
	OpVarpSmall = Op{Opcode: 150, PayloadSize: 3}
	OpVarpLarge = Op{Opcode: 175, PayloadSize: 6}

	OpUpdateZonePartialFollows  = Op{Opcode: 7, PayloadSize: 2}
	OpUpdateZoneFullFollows     = Op{Opcode: 135, PayloadSize: 2}
	OpUpdateZonePartialEnclosed = Op{Opcode: 162, PayloadSize: -2}

	// Zone-nested opcodes, reused as top-level packets for per-player
	// UpdateZonePartialFollows delivery. Sizes match the Java client's
	// SERVERPROT_SIZES at the matching indices.
	OpLocAddChange = Op{Opcode: 59, PayloadSize: 4}
	OpLocAnim      = Op{Opcode: 42, PayloadSize: 4}
	OpLocDel       = Op{Opcode: 76, PayloadSize: 2}
	OpLocMerge     = Op{Opcode: 23, PayloadSize: 14}
	OpMapAnim      = Op{Opcode: 191, PayloadSize: 6}
	OpMapProjAnim  = Op{Opcode: 69, PayloadSize: 15}
	OpObjAdd       = Op{Opcode: 223, PayloadSize: 5}
	OpObjCount     = Op{Opcode: 151, PayloadSize: 7}
	OpObjDel       = Op{Opcode: 49, PayloadSize: 3}
	OpObjReveal    = Op{Opcode: 50, PayloadSize: 7}

	// Map-data streaming (sub-spec 5b). 991-byte chunk size per DATA_LAND/LOC.
	OpDataLand     = Op{Opcode: 132, PayloadSize: -2}
	OpDataLoc      = Op{Opcode: 220, PayloadSize: -2}
	OpDataLandDone = Op{Opcode: 80, PayloadSize: 2}
	OpDataLocDone  = Op{Opcode: 20, PayloadSize: 2}

	// Interaction (sub-spec 6a).
	OpUnsetMapFlag = Op{Opcode: 19, PayloadSize: 0}

	// RuneScript S2 — chat output emitted by the MES opcode.
	OpMessageGame = Op{Opcode: 4, PayloadSize: -1}

	// MIDI client-audio packets (verified against TS ServerGameProt.ts:81-82).
	// MIDI_SONG streams a song reference (name + crc + length so the client
	// can fetch the .mid blob from the asset server); MIDI_JINGLE streams
	// an inline jingle payload. Wired from the MIDI_SONG (2064) / MIDI_JINGLE
	// (2063) script opcodes via (*Player).PlaySong / PlayJingle.
	OpMidiSong   = Op{Opcode: 54, PayloadSize: -1}
	OpMidiJingle = Op{Opcode: 212, PayloadSize: -2}

	// Sound-effect packet (verified against TS ServerGameProt.ts:80).
	// SYNTH_SOUND plays a short synthesized sound effect; payload is
	// fixed 5 bytes: p2(synth) p1(loops) p2(delay) per
	// SynthSoundEncoder.ts:9-13. Wired from the SOUND_SYNTH (2104)
	// script opcode via (*Player).PlaySynth.
	OpSynthSound = Op{Opcode: 12, PayloadSize: 5}

	// Input-tracking signals — server tells client to start/stop sending
	// EVENT_TRACKING blobs (op 81). NAI-73; mirrors TS ServerGameProt.ts:43-44.
	OpEnableTracking = Op{Opcode: 226, PayloadSize: 0}
	OpFinishTracking = Op{Opcode: 133, PayloadSize: 0}

	// OpLastLoginInfo carries previous-login telemetry the client renders
	// on the welcome screen: last-login IP (always 127.0.0.1 / 2130706433
	// per TS Player.ts:2194), days since previous login, days since
	// recovery-questions changed (always 201, hidden), and the unread
	// message count. Fixed 9-byte payload: p4(lastIp), p2(daysSinceLogin),
	// p1(daysSinceRecoveriesChanged), p2(messageCount). Mirrors TS
	// ServerGameProt.LAST_LOGIN_INFO (140, 9) and LastLoginInfoEncoder.ts.
	OpLastLoginInfo = Op{Opcode: 140, PayloadSize: 9}

	// OpUpdatePid carries the player's server-side slot to the client
	// so the client's localPlayer reference is bound to the correct
	// PlayerInfo slot. Emitted once at onLogin. Fixed 2-byte payload:
	// p2(slot). Mirrors TS ServerGameProt.UPDATE_PID (139, 2) and
	// UpdatePidEncoder.ts (NAI-182).
	OpUpdatePid = Op{Opcode: 139, PayloadSize: 2}

	// OpResetAnims tells the client to clear all animation layers on the
	// local player. Zero-byte payload. Emitted at onLogin (after varp
	// resync) and onReconnect (after per-stat UpdateStat/UpdateRunEnergy).
	// Mirrors TS ServerGameProt.RESET_ANIMS (136, 0) and
	// ResetAnimsEncoder.ts (NAI-182).
	OpResetAnims = Op{Opcode: 136, PayloadSize: 0}

	// OpResetClientVarCache tells the client to drop its cached varp
	// values so the next varp packets become authoritative. Emitted at
	// onLogin and onReconnect immediately before the varp transmit-loop.
	// Zero-byte payload. Mirrors TS ServerGameProt.RESET_CLIENT_VARCACHE
	// (193, 0) and ResetClientVarCacheEncoder.ts (NAI-182).
	OpResetClientVarCache = Op{Opcode: 193, PayloadSize: 0}

	// OpUpdateRebootTimer carries the number of game ticks (600ms each)
	// remaining until the world reboots. Sent broadcast by
	// Server.rebootTimer and to each connecting player at processLogins
	// if a shutdown is pending. Fixed 2-byte payload: p2(ticks). Mirrors
	// TS ServerGameProt.UPDATE_REBOOT_TIMER (43, 2) and
	// UpdateRebootTimerEncoder.ts (NAI-182).
	OpUpdateRebootTimer = Op{Opcode: 43, PayloadSize: 2}

	// OpUpdateFriendList carries one friend-entry update. Fixed 9-byte
	// payload: p8(username37) + p1(worldId). worldId == 0 means the friend
	// is offline / hidden. Emitted once per entry by the friends-server
	// dispatcher (one packet per FriendEntry in the FriendlistUpdate batch).
	// Mirrors TS ServerGameProt.UPDATE_FRIENDLIST (152, 9) and
	// UpdateFriendListEncoder.ts.
	OpUpdateFriendList = Op{Opcode: 152, PayloadSize: 9}

	// OpUpdateIgnoreList carries the complete ignorelist snapshot. Variable
	// 2-byte-length-prefixed payload: p8(username37) × N. Emitted on every
	// ignorelist mutation; the entire list is re-sent rather than a delta.
	// Mirrors TS ServerGameProt.UPDATE_IGNORELIST (21, -2) and
	// UpdateIgnoreListEncoder.ts.
	OpUpdateIgnoreList = Op{Opcode: 21, PayloadSize: -2}

	// OpChatFilterSettings carries the player's chat-filter mode triple.
	// Fixed 3-byte payload: p1(publicChat) + p1(privateChat) + p1(tradeDuel).
	// Emitted once at onLogin (before UpdatePid). Mirrors TS
	// ServerGameProt.CHAT_FILTER_SETTINGS (32, 3) and
	// ChatFilterSettingsEncoder.ts.
	OpChatFilterSettings = Op{Opcode: 32, PayloadSize: 3}

	// OpMessagePrivate carries one inbound private-chat delivery to the
	// recipient. Variable 1-byte-length-prefixed payload:
	// p8(fromUsername37) + p4(pmId) + p1(staffLvlAdjusted) +
	// WordPack.pack(chat). staffLvlAdjusted = staffLvl > 0 ? staffLvl + 1 :
	// staffLvl. Emitted by the friends-server dispatcher on
	// PrivateMessageDelivery. Mirrors TS ServerGameProt.MESSAGE_PRIVATE
	// (41, -1) and MessagePrivateEncoder.ts.
	OpMessagePrivate = Op{Opcode: 41, PayloadSize: -1}
)
