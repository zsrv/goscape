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

// 245.2 wire opcodes. TS ClientGameProt.ts (245.2 pin 3c16994c) — the TS ctor's first arg (NXT packet index) has zero readers at the pin and is not modeled.
const (
	OpcNoTimeout     uint8 = 206
	OpcIdleTimer     uint8 = 102
	OpcEventTracking uint8 = 19

	OpcAnticheatOplogic1 uint8 = 87
	OpcAnticheatOplogic2 uint8 = 95
	OpcAnticheatOplogic3 uint8 = 146
	OpcAnticheatOplogic4 uint8 = 186
	OpcAnticheatOplogic5 uint8 = 74
	OpcAnticheatOplogic6 uint8 = 250
	OpcAnticheatOplogic7 uint8 = 119
	OpcAnticheatOplogic8 uint8 = 171
	OpcAnticheatOplogic9 uint8 = 233

	OpcAnticheatCyclelogic1 uint8 = 136
	OpcAnticheatCyclelogic2 uint8 = 223
	OpcAnticheatCyclelogic3 uint8 = 181
	OpcAnticheatCyclelogic4 uint8 = 94
	OpcAnticheatCyclelogic5 uint8 = 63
	OpcAnticheatCyclelogic6 uint8 = 112

	OpcOpObj1 uint8 = 113
	OpcOpObj2 uint8 = 238
	OpcOpObj3 uint8 = 55
	OpcOpObj4 uint8 = 17
	OpcOpObj5 uint8 = 247
	OpcOpObjT uint8 = 122
	OpcOpObjU uint8 = 143

	OpcOpNpc1 uint8 = 180
	OpcOpNpc2 uint8 = 252
	OpcOpNpc3 uint8 = 196
	OpcOpNpc4 uint8 = 107
	OpcOpNpc5 uint8 = 43
	OpcOpNpcT uint8 = 141
	OpcOpNpcU uint8 = 14

	OpcOpLoc1 uint8 = 1
	OpcOpLoc2 uint8 = 219
	OpcOpLoc3 uint8 = 226
	OpcOpLoc4 uint8 = 204
	OpcOpLoc5 uint8 = 86
	OpcOpLocT uint8 = 208
	OpcOpLocU uint8 = 147

	OpcOpPlayer1 uint8 = 135
	OpcOpPlayer2 uint8 = 165
	OpcOpPlayer3 uint8 = 172
	OpcOpPlayer4 uint8 = 54
	OpcOpPlayerT uint8 = 52
	OpcOpPlayerU uint8 = 210

	OpcOpHeld1 uint8 = 104
	OpcOpHeld2 uint8 = 193
	OpcOpHeld3 uint8 = 115
	OpcOpHeld4 uint8 = 194
	OpcOpHeld5 uint8 = 9
	OpcOpHeldT uint8 = 188
	OpcOpHeldU uint8 = 126

	OpcInvButton1 uint8 = 13
	OpcInvButton2 uint8 = 58
	OpcInvButton3 uint8 = 48
	OpcInvButton4 uint8 = 183
	OpcInvButton5 uint8 = 242

	OpcIfButton           uint8 = 177
	OpcResumePauseButton  uint8 = 239
	OpcCloseModal         uint8 = 245
	OpcResumePCountdialog uint8 = 241
	OpcTutorialClickSide  uint8 = 243

	OpcMoveOpClick      uint8 = 216
	OpcReportAbuse      uint8 = 205
	OpcMoveMinimapClick uint8 = 198
	OpcInvButtonD       uint8 = 7
	OpcIgnorelistDel    uint8 = 4
	OpcIgnorelistAdd    uint8 = 20
	OpcIfPlayerDesign   uint8 = 150
	OpcChatSetmode      uint8 = 8
	OpcMessagePrivate   uint8 = 99
	OpcFriendlistDel    uint8 = 61
	OpcFriendlistAdd    uint8 = 116
	OpcClientCheat      uint8 = 11
	OpcMessagePublic    uint8 = 78
	OpcMoveGameClick    uint8 = 182
)

// Ops is a 256-entry lookup table indexed by decrypted game opcode.
// A zero-value Op (empty Name) means the opcode is unknown.
var Ops [256]Op

func init() {
	u := CategoryUserEvent
	c := CategoryClientEvent
	r := CategoryRestrictedEvent

	set := func(opcode uint8, name string, payloadSize int, category int) {
		Ops[opcode] = Op{Name: name, PayloadSize: payloadSize, Category: category}
	}

	set(OpcNoTimeout, "NO_TIMEOUT", 0, c)
	set(OpcIdleTimer, "IDLE_TIMER", 0, c)
	set(OpcEventTracking, "EVENT_TRACKING", -2, r)

	set(OpcAnticheatOplogic1, "ANTICHEAT_OPLOGIC1", 4, c)
	set(OpcAnticheatOplogic2, "ANTICHEAT_OPLOGIC2", 4, c)
	set(OpcAnticheatOplogic3, "ANTICHEAT_OPLOGIC3", 3, c)
	set(OpcAnticheatOplogic4, "ANTICHEAT_OPLOGIC4", 2, c)
	set(OpcAnticheatOplogic5, "ANTICHEAT_OPLOGIC5", 0, c)
	set(OpcAnticheatOplogic6, "ANTICHEAT_OPLOGIC6", 4, c)
	set(OpcAnticheatOplogic7, "ANTICHEAT_OPLOGIC7", 4, c)
	set(OpcAnticheatOplogic8, "ANTICHEAT_OPLOGIC8", 2, c)
	set(OpcAnticheatOplogic9, "ANTICHEAT_OPLOGIC9", 1, c)

	set(OpcAnticheatCyclelogic1, "ANTICHEAT_CYCLELOGIC1", 1, c)
	set(OpcAnticheatCyclelogic2, "ANTICHEAT_CYCLELOGIC2", -1, c)
	set(OpcAnticheatCyclelogic3, "ANTICHEAT_CYCLELOGIC3", 3, c)
	set(OpcAnticheatCyclelogic4, "ANTICHEAT_CYCLELOGIC4", 4, c)
	set(OpcAnticheatCyclelogic5, "ANTICHEAT_CYCLELOGIC5", 0, c)
	set(OpcAnticheatCyclelogic6, "ANTICHEAT_CYCLELOGIC6", -1, c)

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
