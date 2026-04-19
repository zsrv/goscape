package client

// Op describes a client game packet opcode.
type Op struct {
	Name        string
	PayloadSize int // 0=fixed-zero, N=fixed-N, -1=1-byte-len, -2=2-byte-len
}

// Ops is a 256-entry lookup table indexed by decrypted game opcode.
// A zero-value Op (empty Name) means the opcode is unknown.
var Ops [256]Op

func init() {
	set := func(opcode uint8, name string, payloadSize int) {
		Ops[opcode] = Op{Name: name, PayloadSize: payloadSize}
	}

	set(150, "REBUILD_GETMAPS", -1)
	set(108, "NO_TIMEOUT", 0)
	set(70, "IDLE_TIMER", 0)
	set(81, "EVENT_TRACKING", -2)
	set(189, "EVENT_CAMERA_POSITION", 6)

	set(7, "ANTICHEAT_OPLOGIC1", 4)
	set(88, "ANTICHEAT_OPLOGIC2", 4)
	set(30, "ANTICHEAT_OPLOGIC3", 3)
	set(176, "ANTICHEAT_OPLOGIC4", 2)
	set(220, "ANTICHEAT_OPLOGIC5", 0)
	set(66, "ANTICHEAT_OPLOGIC6", 4)
	set(17, "ANTICHEAT_OPLOGIC7", 4)
	set(2, "ANTICHEAT_OPLOGIC8", 2)
	set(238, "ANTICHEAT_OPLOGIC9", 1)

	set(233, "ANTICHEAT_CYCLELOGIC1", 1)
	set(146, "ANTICHEAT_CYCLELOGIC2", -1)
	set(215, "ANTICHEAT_CYCLELOGIC3", 3)
	set(236, "ANTICHEAT_CYCLELOGIC4", 4)
	set(85, "ANTICHEAT_CYCLELOGIC5", 0)
	set(219, "ANTICHEAT_CYCLELOGIC6", -1)

	set(140, "OPOBJ1", 6)
	set(40, "OPOBJ2", 6)
	set(200, "OPOBJ3", 6)
	set(178, "OPOBJ4", 6)
	set(247, "OPOBJ5", 6)
	set(138, "OPOBJT", 8)
	set(239, "OPOBJU", 12)

	set(194, "OPNPC1", 2)
	set(8, "OPNPC2", 2)
	set(27, "OPNPC3", 2)
	set(113, "OPNPC4", 2)
	set(100, "OPNPC5", 2)
	set(134, "OPNPCT", 4)
	set(202, "OPNPCU", 8)

	set(245, "OPLOC1", 6)
	set(172, "OPLOC2", 6)
	set(96, "OPLOC3", 6)
	set(97, "OPLOC4", 6)
	set(116, "OPLOC5", 6)
	set(9, "OPLOCT", 8)
	set(75, "OPLOCU", 12)

	set(164, "OPPLAYER1", 2)
	set(53, "OPPLAYER2", 2)
	set(185, "OPPLAYER3", 2)
	set(206, "OPPLAYER4", 2)
	set(177, "OPPLAYERT", 4)
	set(248, "OPPLAYERU", 8)

	set(195, "OPHELD1", 6)
	set(71, "OPHELD2", 6)
	set(133, "OPHELD3", 6)
	set(157, "OPHELD4", 6)
	set(211, "OPHELD5", 6)
	set(48, "OPHELDT", 8)
	set(130, "OPHELDU", 12)

	set(31, "INV_BUTTON1", 6)
	set(59, "INV_BUTTON2", 6)
	set(212, "INV_BUTTON3", 6)
	set(38, "INV_BUTTON4", 6)
	set(6, "INV_BUTTON5", 6)

	set(155, "IF_BUTTON", 2)
	set(235, "RESUME_PAUSEBUTTON", 2)
	set(231, "CLOSE_MODAL", 0)
	set(237, "RESUME_P_COUNTDIALOG", 4)
	set(175, "TUT_CLICKSIDE", 1)

	set(93, "MOVE_OPCLICK", -1)
	set(190, "REPORT_ABUSE", 10)
	set(165, "MOVE_MINIMAPCLICK", -1)
	set(159, "INV_BUTTOND", 6)
	set(171, "IGNORELIST_DEL", 8)
	set(79, "IGNORELIST_ADD", 8)
	set(52, "IDK_SAVEDESIGN", 13)
	set(244, "CHAT_SETMODE", 3)
	set(148, "MESSAGE_PRIVATE", -1)
	set(11, "FRIENDLIST_DEL", 8)
	set(118, "FRIENDLIST_ADD", 8)
	set(4, "CLIENT_CHEAT", -1)
	set(158, "MESSAGE_PUBLIC", -1)
	set(181, "MOVE_GAMECLICK", -1)
}
