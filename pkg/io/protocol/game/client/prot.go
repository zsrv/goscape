package client

// Op describes a client game packet opcode.
type Op struct {
	Name        string
	PayloadSize int // 0=fixed-zero, N=fixed-N, -1=1-byte-len, -2=2-byte-len
	Category    int // CategoryClientEvent | CategoryUserEvent | CategoryRestrictedEvent
}

const (
	CategoryClientEvent     = 0 // limit 20/tick
	CategoryUserEvent       = 1 // limit 5/tick
	CategoryRestrictedEvent = 2 // limit 2/tick
)

// 274 wire opcodes. TS ClientGameProt.ts (274 pin dee467c8) — the TS ctor is
// (id, length) only; categories come from the model files (ClientGameMessage
// subclasses under network/game/client/model/). EVENT_TRACKING was DELETED at
// 274 (no replacement; the four discrete event packets survive, renumbered) —
// an unknown opcode from a 274 client is a protocol error, matching TS where
// ClientGameProt.byId has no row.
const (
	OpcNoTimeout uint8 = 120

	OpcIdleTimer           uint8 = 209
	OpcEventMouseClick     uint8 = 20
	OpcEventMouseMove      uint8 = 222
	OpcEventAppletFocus    uint8 = 73
	OpcEventCameraPosition uint8 = 53

	OpcAnticheatOplogic1 uint8 = 219
	OpcAnticheatOplogic2 uint8 = 201
	OpcAnticheatOplogic3 uint8 = 41
	OpcAnticheatOplogic4 uint8 = 80
	OpcAnticheatOplogic5 uint8 = 235
	OpcAnticheatOplogic6 uint8 = 250
	OpcAnticheatOplogic7 uint8 = 25
	OpcAnticheatOplogic8 uint8 = 0
	OpcAnticheatOplogic9 uint8 = 24

	OpcAnticheatCyclelogic1 uint8 = 12
	OpcAnticheatCyclelogic2 uint8 = 149
	OpcAnticheatCyclelogic3 uint8 = 52
	OpcAnticheatCyclelogic4 uint8 = 230
	OpcAnticheatCyclelogic5 uint8 = 100
	OpcAnticheatCyclelogic6 uint8 = 188
	OpcAnticheatCyclelogic7 uint8 = 89

	OpcOpObj1 uint8 = 247
	OpcOpObj2 uint8 = 169
	OpcOpObj3 uint8 = 108
	OpcOpObj4 uint8 = 62
	OpcOpObj5 uint8 = 117
	OpcOpObjT uint8 = 91
	OpcOpObjU uint8 = 39

	OpcOpNpc1 uint8 = 236
	OpcOpNpc2 uint8 = 233
	OpcOpNpc3 uint8 = 223
	OpcOpNpc4 uint8 = 147
	OpcOpNpc5 uint8 = 189
	OpcOpNpcT uint8 = 181
	OpcOpNpcU uint8 = 150

	OpcOpLoc1 uint8 = 215
	OpcOpLoc2 uint8 = 103
	OpcOpLoc3 uint8 = 187
	OpcOpLoc4 uint8 = 157
	OpcOpLoc5 uint8 = 127
	OpcOpLocT uint8 = 213
	OpcOpLocU uint8 = 60

	OpcOpPlayer1 uint8 = 109
	OpcOpPlayer2 uint8 = 166
	OpcOpPlayer3 uint8 = 196
	OpcOpPlayer4 uint8 = 98
	OpcOpPlayer5 uint8 = 174
	OpcOpPlayerT uint8 = 240
	OpcOpPlayerU uint8 = 36

	OpcOpHeld1 uint8 = 185
	OpcOpHeld2 uint8 = 2
	OpcOpHeld3 uint8 = 123
	OpcOpHeld4 uint8 = 216
	OpcOpHeld5 uint8 = 42
	OpcOpHeldT uint8 = 135
	OpcOpHeldU uint8 = 136

	OpcInvButton1 uint8 = 74
	OpcInvButton2 uint8 = 82
	OpcInvButton3 uint8 = 239
	OpcInvButton4 uint8 = 179
	OpcInvButton5 uint8 = 46

	OpcIfButton           uint8 = 9
	OpcResumePauseButton  uint8 = 72
	OpcCloseModal         uint8 = 51
	OpcResumePCountdialog uint8 = 102
	OpcTutorialClickSide  uint8 = 94

	OpcMapBuildComplete uint8 = 214
	OpcMoveOpClick      uint8 = 138
	OpcReportAbuse      uint8 = 137
	OpcMoveMinimapClick uint8 = 86
	OpcInvButtonD       uint8 = 93
	OpcIgnorelistDel    uint8 = 101
	OpcIgnorelistAdd    uint8 = 255
	OpcIfPlayerDesign   uint8 = 125
	OpcChatSetmode      uint8 = 154
	OpcMessagePrivate   uint8 = 139
	OpcFriendlistDel    uint8 = 106
	OpcFriendlistAdd    uint8 = 13
	OpcClientCheat      uint8 = 224
	OpcMessagePublic    uint8 = 253
	OpcMoveGameClick    uint8 = 207
)

// Ops is a 256-entry lookup table indexed by decrypted game opcode.
// A zero-value Op (empty Name) means the opcode is unknown.
var Ops [256]Op

func init() {
	u := CategoryUserEvent
	c := CategoryClientEvent

	set := func(opcode uint8, name string, payloadSize int, category int) {
		Ops[opcode] = Op{Name: name, PayloadSize: payloadSize, Category: category}
	}

	set(OpcNoTimeout, "NO_TIMEOUT", 0, c)

	// The EVENT_* split packets are CLIENT_EVENT per the TS model files
	// (model/EventMouseClick.ts etc. at the 274 pin dee467c8). The 254-era
	// EVENT_TRACKING row (RESTRICTED_EVENT, -2) was deleted at 274.
	set(OpcIdleTimer, "IDLE_TIMER", 0, c)
	set(OpcEventMouseClick, "EVENT_MOUSE_CLICK", 4, c)
	set(OpcEventMouseMove, "EVENT_MOUSE_MOVE", -1, c)
	set(OpcEventAppletFocus, "EVENT_APPLET_FOCUS", 1, c)
	set(OpcEventCameraPosition, "EVENT_CAMERA_POSITION", 4, c)

	set(OpcAnticheatOplogic1, "ANTICHEAT_OPLOGIC1", 4, c)
	set(OpcAnticheatOplogic2, "ANTICHEAT_OPLOGIC2", 2, c)
	set(OpcAnticheatOplogic3, "ANTICHEAT_OPLOGIC3", 4, c)
	set(OpcAnticheatOplogic4, "ANTICHEAT_OPLOGIC4", 1, c)
	set(OpcAnticheatOplogic5, "ANTICHEAT_OPLOGIC5", 1, c)
	set(OpcAnticheatOplogic6, "ANTICHEAT_OPLOGIC6", 2, c)
	set(OpcAnticheatOplogic7, "ANTICHEAT_OPLOGIC7", 4, c)
	set(OpcAnticheatOplogic8, "ANTICHEAT_OPLOGIC8", 1, c)
	set(OpcAnticheatOplogic9, "ANTICHEAT_OPLOGIC9", 3, c)

	set(OpcAnticheatCyclelogic1, "ANTICHEAT_CYCLELOGIC1", -1, c)
	set(OpcAnticheatCyclelogic2, "ANTICHEAT_CYCLELOGIC2", -1, c)
	set(OpcAnticheatCyclelogic3, "ANTICHEAT_CYCLELOGIC3", 1, c)
	set(OpcAnticheatCyclelogic4, "ANTICHEAT_CYCLELOGIC4", 1, c)
	set(OpcAnticheatCyclelogic5, "ANTICHEAT_CYCLELOGIC5", 0, c)
	set(OpcAnticheatCyclelogic6, "ANTICHEAT_CYCLELOGIC6", 1, c)
	set(OpcAnticheatCyclelogic7, "ANTICHEAT_CYCLELOGIC7", 0, c)

	set(OpcOpObj1, "OPOBJ1", 6, u)
	set(OpcOpObj2, "OPOBJ2", 6, u)
	set(OpcOpObj3, "OPOBJ3", 6, u)
	set(OpcOpObj4, "OPOBJ4", 6, u)
	set(OpcOpObj5, "OPOBJ5", 6, u)
	set(OpcOpObjT, "OPOBJT", 8, u)
	set(OpcOpObjU, "OPOBJU", 12, u)

	set(OpcOpNpc1, "OPNPC1", 2, u)
	set(OpcOpNpc2, "OPNPC2", 2, u)
	set(OpcOpNpc3, "OPNPC3", 2, u)
	set(OpcOpNpc4, "OPNPC4", 2, u)
	set(OpcOpNpc5, "OPNPC5", 2, u)
	set(OpcOpNpcT, "OPNPCT", 4, u)
	set(OpcOpNpcU, "OPNPCU", 8, u)

	set(OpcOpLoc1, "OPLOC1", 6, u)
	set(OpcOpLoc2, "OPLOC2", 6, u)
	set(OpcOpLoc3, "OPLOC3", 6, u)
	set(OpcOpLoc4, "OPLOC4", 6, u)
	set(OpcOpLoc5, "OPLOC5", 6, u)
	set(OpcOpLocT, "OPLOCT", 8, u)
	set(OpcOpLocU, "OPLOCU", 12, u)

	set(OpcOpPlayer1, "OPPLAYER1", 2, u)
	set(OpcOpPlayer2, "OPPLAYER2", 2, u)
	set(OpcOpPlayer3, "OPPLAYER3", 2, u)
	set(OpcOpPlayer4, "OPPLAYER4", 2, u)
	set(OpcOpPlayer5, "OPPLAYER5", 2, u)
	set(OpcOpPlayerT, "OPPLAYERT", 4, u)
	set(OpcOpPlayerU, "OPPLAYERU", 8, u)

	set(OpcOpHeld1, "OPHELD1", 6, u)
	set(OpcOpHeld2, "OPHELD2", 6, u)
	set(OpcOpHeld3, "OPHELD3", 6, u)
	set(OpcOpHeld4, "OPHELD4", 6, u)
	set(OpcOpHeld5, "OPHELD5", 6, u)
	set(OpcOpHeldT, "OPHELDT", 8, u)
	set(OpcOpHeldU, "OPHELDU", 12, u)

	set(OpcInvButton1, "INV_BUTTON1", 6, u)
	set(OpcInvButton2, "INV_BUTTON2", 6, u)
	set(OpcInvButton3, "INV_BUTTON3", 6, u)
	set(OpcInvButton4, "INV_BUTTON4", 6, u)
	set(OpcInvButton5, "INV_BUTTON5", 6, u)

	set(OpcIfButton, "IF_BUTTON", 2, u)
	set(OpcResumePauseButton, "RESUME_PAUSEBUTTON", 2, u)
	set(OpcCloseModal, "CLOSE_MODAL", 0, u)
	set(OpcResumePCountdialog, "RESUME_P_COUNTDIALOG", 4, u)
	set(OpcTutorialClickSide, "TUTORIAL_CLICKSIDE", 1, u)

	// MAP_BUILD_COMPLETE has no model/decoder at the 274 pin (read-and-discard);
	// ClientEvent matches the NO_TIMEOUT/anticheat precedent.
	set(OpcMapBuildComplete, "MAP_BUILD_COMPLETE", 0, c)
	set(OpcMoveOpClick, "MOVE_OPCLICK", -1, u)
	set(OpcReportAbuse, "REPORT_ABUSE", 10, u)
	set(OpcMoveMinimapClick, "MOVE_MINIMAPCLICK", -1, u)
	set(OpcInvButtonD, "INV_BUTTOND", 7, u)
	set(OpcIgnorelistDel, "IGNORELIST_DEL", 8, u)
	set(OpcIgnorelistAdd, "IGNORELIST_ADD", 8, u)
	set(OpcIfPlayerDesign, "IF_PLAYERDESIGN", 13, u)
	set(OpcChatSetmode, "CHAT_SETMODE", 3, u)
	set(OpcMessagePrivate, "MESSAGE_PRIVATE", -1, u)
	set(OpcFriendlistDel, "FRIENDLIST_DEL", 8, u)
	set(OpcFriendlistAdd, "FRIENDLIST_ADD", 8, u)
	set(OpcClientCheat, "CLIENT_CHEAT", -1, u)
	set(OpcMessagePublic, "MESSAGE_PUBLIC", -1, u)
	set(OpcMoveGameClick, "MOVE_GAMECLICK", -1, u)
}
