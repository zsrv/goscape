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

	set(150, "REBUILD_GETMAPS", -1, r)
	set(108, "NO_TIMEOUT", 0, c)
	set(70, "IDLE_TIMER", 0, c)
	set(81, "EVENT_TRACKING", -2, r)
	set(189, "EVENT_CAMERA_POSITION", 6, c)

	set(7, "ANTICHEAT_OPLOGIC1", 4, c)
	set(88, "ANTICHEAT_OPLOGIC2", 4, c)
	set(30, "ANTICHEAT_OPLOGIC3", 3, c)
	set(176, "ANTICHEAT_OPLOGIC4", 2, c)
	set(220, "ANTICHEAT_OPLOGIC5", 0, c)
	set(66, "ANTICHEAT_OPLOGIC6", 4, c)
	set(17, "ANTICHEAT_OPLOGIC7", 4, c)
	set(2, "ANTICHEAT_OPLOGIC8", 2, c)
	set(238, "ANTICHEAT_OPLOGIC9", 1, c)

	set(233, "ANTICHEAT_CYCLELOGIC1", 1, c)
	set(146, "ANTICHEAT_CYCLELOGIC2", -1, c)
	set(215, "ANTICHEAT_CYCLELOGIC3", 3, c)
	set(236, "ANTICHEAT_CYCLELOGIC4", 4, c)
	set(85, "ANTICHEAT_CYCLELOGIC5", 0, c)
	set(219, "ANTICHEAT_CYCLELOGIC6", -1, c)

	set(140, "OPOBJ1", 6, u)
	set(40, "OPOBJ2", 6, u)
	set(200, "OPOBJ3", 6, u)
	set(178, "OPOBJ4", 6, u)
	set(247, "OPOBJ5", 6, u)
	set(138, "OPOBJT", 8, u)
	set(239, "OPOBJU", 12, u)

	set(194, "OPNPC1", 2, u)
	set(8, "OPNPC2", 2, u)
	set(27, "OPNPC3", 2, u)
	set(113, "OPNPC4", 2, u)
	set(100, "OPNPC5", 2, u)
	set(134, "OPNPCT", 4, u)
	set(202, "OPNPCU", 8, u)

	set(245, "OPLOC1", 6, u)
	set(172, "OPLOC2", 6, u)
	set(96, "OPLOC3", 6, u)
	set(97, "OPLOC4", 6, u)
	set(116, "OPLOC5", 6, u)
	set(9, "OPLOCT", 8, u)
	set(75, "OPLOCU", 12, u)

	set(164, "OPPLAYER1", 2, u)
	set(53, "OPPLAYER2", 2, u)
	set(185, "OPPLAYER3", 2, u)
	set(206, "OPPLAYER4", 2, u)
	set(177, "OPPLAYERT", 4, u)
	set(248, "OPPLAYERU", 8, u)

	set(195, "OPHELD1", 6, u)
	set(71, "OPHELD2", 6, u)
	set(133, "OPHELD3", 6, u)
	set(157, "OPHELD4", 6, u)
	set(211, "OPHELD5", 6, u)
	set(48, "OPHELDT", 8, u)
	set(130, "OPHELDU", 12, u)

	set(31, "INV_BUTTON1", 6, u)
	set(59, "INV_BUTTON2", 6, u)
	set(212, "INV_BUTTON3", 6, u)
	set(38, "INV_BUTTON4", 6, u)
	set(6, "INV_BUTTON5", 6, u)

	set(155, "IF_BUTTON", 2, u)
	set(235, "RESUME_PAUSEBUTTON", 2, u)
	set(231, "CLOSE_MODAL", 0, u)
	set(237, "RESUME_P_COUNTDIALOG", 4, u)
	set(175, "TUT_CLICKSIDE", 1, u)

	set(93, "MOVE_OPCLICK", -1, u)
	set(190, "REPORT_ABUSE", 10, u)
	set(165, "MOVE_MINIMAPCLICK", -1, u)
	set(159, "INV_BUTTOND", 6, u)
	set(171, "IGNORELIST_DEL", 8, u)
	set(79, "IGNORELIST_ADD", 8, u)
	set(52, "IDK_SAVEDESIGN", 13, u)
	set(244, "CHAT_SETMODE", 3, u)
	set(148, "MESSAGE_PRIVATE", -1, u)
	set(11, "FRIENDLIST_DEL", 8, u)
	set(118, "FRIENDLIST_ADD", 8, u)
	set(4, "CLIENT_CHEAT", -1, u)
	set(158, "MESSAGE_PUBLIC", -1, u)
	set(181, "MOVE_GAMECLICK", -1, u)
}
