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

// 244 wire opcodes. TS ClientGameProt.ts (244 pin) — the TS ctor's first arg (NXT packet index) has zero readers at the pin and is not modeled.
const (
	OpcNoTimeout     uint8 = 107
	OpcIdleTimer     uint8 = 146
	OpcEventTracking uint8 = 217

	OpcAnticheatOplogic1 uint8 = 47
	OpcAnticheatOplogic2 uint8 = 218
	OpcAnticheatOplogic3 uint8 = 37
	OpcAnticheatOplogic4 uint8 = 34
	OpcAnticheatOplogic5 uint8 = 7
	OpcAnticheatOplogic6 uint8 = 177
	OpcAnticheatOplogic7 uint8 = 50
	OpcAnticheatOplogic8 uint8 = 100
	OpcAnticheatOplogic9 uint8 = 169

	OpcAnticheatCyclelogic1 uint8 = 46
	OpcAnticheatCyclelogic2 uint8 = 148
	OpcAnticheatCyclelogic3 uint8 = 144
	OpcAnticheatCyclelogic4 uint8 = 41
	OpcAnticheatCyclelogic5 uint8 = 232
	OpcAnticheatCyclelogic6 uint8 = 215

	OpcOpObj1 uint8 = 231
	OpcOpObj2 uint8 = 110
	OpcOpObj3 uint8 = 27
	OpcOpObj4 uint8 = 17
	OpcOpObj5 uint8 = 225
	OpcOpObjT uint8 = 25
	OpcOpObjU uint8 = 111

	OpcOpNpc1 uint8 = 222
	OpcOpNpc2 uint8 = 84
	OpcOpNpc3 uint8 = 132
	OpcOpNpc4 uint8 = 229
	OpcOpNpc5 uint8 = 102
	OpcOpNpcT uint8 = 101
	OpcOpNpcU uint8 = 52

	OpcOpLoc1 uint8 = 238
	OpcOpLoc2 uint8 = 38
	OpcOpLoc3 uint8 = 19
	OpcOpLoc4 uint8 = 55
	OpcOpLoc5 uint8 = 243
	OpcOpLocT uint8 = 182
	OpcOpLocU uint8 = 106

	OpcOpPlayer1 uint8 = 211
	OpcOpPlayer2 uint8 = 219
	OpcOpPlayer3 uint8 = 64
	OpcOpPlayer4 uint8 = 43
	OpcOpPlayerT uint8 = 73
	OpcOpPlayerU uint8 = 48

	OpcOpHeld1 uint8 = 228
	OpcOpHeld2 uint8 = 166
	OpcOpHeld3 uint8 = 221
	OpcOpHeld4 uint8 = 6
	OpcOpHeld5 uint8 = 133
	OpcOpHeldT uint8 = 143
	OpcOpHeldU uint8 = 58

	OpcInvButton1 uint8 = 153
	OpcInvButton2 uint8 = 193
	OpcInvButton3 uint8 = 158
	OpcInvButton4 uint8 = 204
	OpcInvButton5 uint8 = 212

	OpcIfButton           uint8 = 39
	OpcResumePauseButton  uint8 = 11
	OpcCloseModal         uint8 = 187
	OpcResumePCountdialog uint8 = 190
	OpcTutorialClickSide  uint8 = 233

	OpcMoveOpClick      uint8 = 167
	OpcReportAbuse      uint8 = 251
	OpcMoveMinimapClick uint8 = 56
	OpcInvButtonD       uint8 = 81
	OpcIgnorelistDel    uint8 = 207
	OpcIgnorelistAdd    uint8 = 203
	OpcIfPlayerDesign   uint8 = 8
	OpcChatSetmode      uint8 = 98
	OpcMessagePrivate   uint8 = 170
	OpcFriendlistDel    uint8 = 69
	OpcFriendlistAdd    uint8 = 9
	OpcClientCheat      uint8 = 76
	OpcMessagePublic    uint8 = 171
	OpcMoveGameClick    uint8 = 63
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
