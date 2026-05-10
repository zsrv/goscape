package script

import "fmt"

// ServerTriggerType identifies which event type a script is bound to.
// Numeric values match TS ServerTriggerType.ts exactly.
type ServerTriggerType int

const (
	TriggerProc      ServerTriggerType = 0
	TriggerLabel     ServerTriggerType = 1
	TriggerDebugProc ServerTriggerType = 2

	TriggerApNpc1 ServerTriggerType = 3
	TriggerApNpc2 ServerTriggerType = 4
	TriggerApNpc3 ServerTriggerType = 5
	TriggerApNpc4 ServerTriggerType = 6
	TriggerApNpc5 ServerTriggerType = 7
	TriggerApNpcU ServerTriggerType = 8
	TriggerApNpcT ServerTriggerType = 9
	TriggerOpNpc1 ServerTriggerType = 10
	TriggerOpNpc2 ServerTriggerType = 11
	TriggerOpNpc3 ServerTriggerType = 12
	TriggerOpNpc4 ServerTriggerType = 13
	TriggerOpNpc5 ServerTriggerType = 14
	TriggerOpNpcU ServerTriggerType = 15
	TriggerOpNpcT ServerTriggerType = 16

	TriggerAiApNpc1 ServerTriggerType = 17
	TriggerAiApNpc2 ServerTriggerType = 18
	TriggerAiApNpc3 ServerTriggerType = 19
	TriggerAiApNpc4 ServerTriggerType = 20
	TriggerAiApNpc5 ServerTriggerType = 21
	TriggerAiOpNpc1 ServerTriggerType = 24
	TriggerAiOpNpc2 ServerTriggerType = 25
	TriggerAiOpNpc3 ServerTriggerType = 26
	TriggerAiOpNpc4 ServerTriggerType = 27
	TriggerAiOpNpc5 ServerTriggerType = 28

	TriggerApObj1 ServerTriggerType = 31
	TriggerApObj2 ServerTriggerType = 32
	TriggerApObj3 ServerTriggerType = 33
	TriggerApObj4 ServerTriggerType = 34
	TriggerApObj5 ServerTriggerType = 35
	TriggerApObjU ServerTriggerType = 36
	TriggerApObjT ServerTriggerType = 37
	TriggerOpObj1 ServerTriggerType = 38
	TriggerOpObj2 ServerTriggerType = 39
	TriggerOpObj3 ServerTriggerType = 40
	TriggerOpObj4 ServerTriggerType = 41
	TriggerOpObj5 ServerTriggerType = 42
	TriggerOpObjU ServerTriggerType = 43
	TriggerOpObjT ServerTriggerType = 44

	TriggerAiApObj1 ServerTriggerType = 45
	TriggerAiApObj2 ServerTriggerType = 46
	TriggerAiApObj3 ServerTriggerType = 47
	TriggerAiApObj4 ServerTriggerType = 48
	TriggerAiApObj5 ServerTriggerType = 49
	TriggerAiOpObj1 ServerTriggerType = 52
	TriggerAiOpObj2 ServerTriggerType = 53
	TriggerAiOpObj3 ServerTriggerType = 54
	TriggerAiOpObj4 ServerTriggerType = 55
	TriggerAiOpObj5 ServerTriggerType = 56

	TriggerApLoc1 ServerTriggerType = 59
	TriggerApLoc2 ServerTriggerType = 60
	TriggerApLoc3 ServerTriggerType = 61
	TriggerApLoc4 ServerTriggerType = 62
	TriggerApLoc5 ServerTriggerType = 63
	TriggerApLocU ServerTriggerType = 64
	TriggerApLocT ServerTriggerType = 65
	TriggerOpLoc1 ServerTriggerType = 66
	TriggerOpLoc2 ServerTriggerType = 67
	TriggerOpLoc3 ServerTriggerType = 68
	TriggerOpLoc4 ServerTriggerType = 69
	TriggerOpLoc5 ServerTriggerType = 70
	TriggerOpLocU ServerTriggerType = 71
	TriggerOpLocT ServerTriggerType = 72

	TriggerAiApLoc1 ServerTriggerType = 73
	TriggerAiApLoc2 ServerTriggerType = 74
	TriggerAiApLoc3 ServerTriggerType = 75
	TriggerAiApLoc4 ServerTriggerType = 76
	TriggerAiApLoc5 ServerTriggerType = 77
	TriggerAiOpLoc1 ServerTriggerType = 80
	TriggerAiOpLoc2 ServerTriggerType = 81
	TriggerAiOpLoc3 ServerTriggerType = 82
	TriggerAiOpLoc4 ServerTriggerType = 83
	TriggerAiOpLoc5 ServerTriggerType = 84

	TriggerApPlayer1 ServerTriggerType = 87
	TriggerApPlayer2 ServerTriggerType = 88
	TriggerApPlayer3 ServerTriggerType = 89
	TriggerApPlayer4 ServerTriggerType = 90
	TriggerApPlayer5 ServerTriggerType = 91
	TriggerApPlayerU ServerTriggerType = 92
	TriggerApPlayerT ServerTriggerType = 93
	TriggerOpPlayer1 ServerTriggerType = 94
	TriggerOpPlayer2 ServerTriggerType = 95
	TriggerOpPlayer3 ServerTriggerType = 96
	TriggerOpPlayer4 ServerTriggerType = 97
	TriggerOpPlayer5 ServerTriggerType = 98
	TriggerOpPlayerU ServerTriggerType = 99
	TriggerOpPlayerT ServerTriggerType = 100

	TriggerAiApPlayer1 ServerTriggerType = 101
	TriggerAiApPlayer2 ServerTriggerType = 102
	TriggerAiApPlayer3 ServerTriggerType = 103
	TriggerAiApPlayer4 ServerTriggerType = 104
	TriggerAiApPlayer5 ServerTriggerType = 105
	TriggerAiOpPlayer1 ServerTriggerType = 108
	TriggerAiOpPlayer2 ServerTriggerType = 109
	TriggerAiOpPlayer3 ServerTriggerType = 110
	TriggerAiOpPlayer4 ServerTriggerType = 111
	TriggerAiOpPlayer5 ServerTriggerType = 112

	TriggerQueue     ServerTriggerType = 116
	TriggerAiQueue1  ServerTriggerType = 117
	TriggerAiQueue2  ServerTriggerType = 118
	TriggerAiQueue3  ServerTriggerType = 119
	TriggerAiQueue4  ServerTriggerType = 120
	TriggerAiQueue5  ServerTriggerType = 121
	TriggerAiQueue6  ServerTriggerType = 122
	TriggerAiQueue7  ServerTriggerType = 123
	TriggerAiQueue8  ServerTriggerType = 124
	TriggerAiQueue9  ServerTriggerType = 125
	TriggerAiQueue10 ServerTriggerType = 126
	TriggerAiQueue11 ServerTriggerType = 127
	TriggerAiQueue12 ServerTriggerType = 128
	TriggerAiQueue13 ServerTriggerType = 129
	TriggerAiQueue14 ServerTriggerType = 130
	TriggerAiQueue15 ServerTriggerType = 131
	TriggerAiQueue16 ServerTriggerType = 132
	TriggerAiQueue17 ServerTriggerType = 133
	TriggerAiQueue18 ServerTriggerType = 134
	TriggerAiQueue19 ServerTriggerType = 135
	TriggerAiQueue20 ServerTriggerType = 136

	TriggerSoftTimer ServerTriggerType = 137
	TriggerTimer     ServerTriggerType = 138
	TriggerAiTimer   ServerTriggerType = 139

	TriggerOpHeld1 ServerTriggerType = 140
	TriggerOpHeld2 ServerTriggerType = 141
	TriggerOpHeld3 ServerTriggerType = 142
	TriggerOpHeld4 ServerTriggerType = 143
	TriggerOpHeld5 ServerTriggerType = 144
	TriggerOpHeldU ServerTriggerType = 145
	TriggerOpHeldT ServerTriggerType = 146

	TriggerIfButton   ServerTriggerType = 147
	TriggerIfClose    ServerTriggerType = 148
	TriggerInvButton1 ServerTriggerType = 149
	TriggerInvButton2 ServerTriggerType = 150
	TriggerInvButton3 ServerTriggerType = 151
	TriggerInvButton4 ServerTriggerType = 152
	TriggerInvButton5 ServerTriggerType = 153
	TriggerInvButtonD ServerTriggerType = 154

	TriggerWalkTrigger   ServerTriggerType = 155
	TriggerAiWalkTrigger ServerTriggerType = 156

	TriggerLogin       ServerTriggerType = 157
	TriggerLogout      ServerTriggerType = 158
	TriggerTutorial    ServerTriggerType = 159
	TriggerAdvanceStat ServerTriggerType = 160
	TriggerMapZone     ServerTriggerType = 161
	TriggerMapZoneExit ServerTriggerType = 162
	TriggerZone        ServerTriggerType = 163
	TriggerZoneExit    ServerTriggerType = 164
	TriggerChangeStat  ServerTriggerType = 165
	TriggerAiSpawn     ServerTriggerType = 166
	TriggerAiDespawn   ServerTriggerType = 167
)

// serverTriggerNames mirrors TS ServerTriggerType reverse-mapping
// (Engine-TS/.../ServerTriggerType.ts:1-162 enum keys, lowercased per
// `ServerTriggerType[trigger].toLowerCase()` at L168). Numeric gaps in
// the TS enum {22,23,29,30,50,51,57,58,78,79,85,86,106,107,113,114,115}
// are intentionally absent — TS reverse-mapping returns undefined for
// those, mapping cleanly onto String()'s "trigger_<N>" fallback.
var serverTriggerNames = map[ServerTriggerType]string{
	TriggerProc:      "proc",
	TriggerLabel:     "label",
	TriggerDebugProc: "debugproc",

	TriggerApNpc1: "apnpc1",
	TriggerApNpc2: "apnpc2",
	TriggerApNpc3: "apnpc3",
	TriggerApNpc4: "apnpc4",
	TriggerApNpc5: "apnpc5",
	TriggerApNpcU: "apnpcu",
	TriggerApNpcT: "apnpct",
	TriggerOpNpc1: "opnpc1",
	TriggerOpNpc2: "opnpc2",
	TriggerOpNpc3: "opnpc3",
	TriggerOpNpc4: "opnpc4",
	TriggerOpNpc5: "opnpc5",
	TriggerOpNpcU: "opnpcu",
	TriggerOpNpcT: "opnpct",

	TriggerAiApNpc1: "ai_apnpc1",
	TriggerAiApNpc2: "ai_apnpc2",
	TriggerAiApNpc3: "ai_apnpc3",
	TriggerAiApNpc4: "ai_apnpc4",
	TriggerAiApNpc5: "ai_apnpc5",
	TriggerAiOpNpc1: "ai_opnpc1",
	TriggerAiOpNpc2: "ai_opnpc2",
	TriggerAiOpNpc3: "ai_opnpc3",
	TriggerAiOpNpc4: "ai_opnpc4",
	TriggerAiOpNpc5: "ai_opnpc5",

	TriggerApObj1: "apobj1",
	TriggerApObj2: "apobj2",
	TriggerApObj3: "apobj3",
	TriggerApObj4: "apobj4",
	TriggerApObj5: "apobj5",
	TriggerApObjU: "apobju",
	TriggerApObjT: "apobjt",
	TriggerOpObj1: "opobj1",
	TriggerOpObj2: "opobj2",
	TriggerOpObj3: "opobj3",
	TriggerOpObj4: "opobj4",
	TriggerOpObj5: "opobj5",
	TriggerOpObjU: "opobju",
	TriggerOpObjT: "opobjt",

	TriggerAiApObj1: "ai_apobj1",
	TriggerAiApObj2: "ai_apobj2",
	TriggerAiApObj3: "ai_apobj3",
	TriggerAiApObj4: "ai_apobj4",
	TriggerAiApObj5: "ai_apobj5",
	TriggerAiOpObj1: "ai_opobj1",
	TriggerAiOpObj2: "ai_opobj2",
	TriggerAiOpObj3: "ai_opobj3",
	TriggerAiOpObj4: "ai_opobj4",
	TriggerAiOpObj5: "ai_opobj5",

	TriggerApLoc1: "aploc1",
	TriggerApLoc2: "aploc2",
	TriggerApLoc3: "aploc3",
	TriggerApLoc4: "aploc4",
	TriggerApLoc5: "aploc5",
	TriggerApLocU: "aplocu",
	TriggerApLocT: "aploct",
	TriggerOpLoc1: "oploc1",
	TriggerOpLoc2: "oploc2",
	TriggerOpLoc3: "oploc3",
	TriggerOpLoc4: "oploc4",
	TriggerOpLoc5: "oploc5",
	TriggerOpLocU: "oplocu",
	TriggerOpLocT: "oploct",

	TriggerAiApLoc1: "ai_aploc1",
	TriggerAiApLoc2: "ai_aploc2",
	TriggerAiApLoc3: "ai_aploc3",
	TriggerAiApLoc4: "ai_aploc4",
	TriggerAiApLoc5: "ai_aploc5",
	TriggerAiOpLoc1: "ai_oploc1",
	TriggerAiOpLoc2: "ai_oploc2",
	TriggerAiOpLoc3: "ai_oploc3",
	TriggerAiOpLoc4: "ai_oploc4",
	TriggerAiOpLoc5: "ai_oploc5",

	TriggerApPlayer1: "applayer1",
	TriggerApPlayer2: "applayer2",
	TriggerApPlayer3: "applayer3",
	TriggerApPlayer4: "applayer4",
	TriggerApPlayer5: "applayer5",
	TriggerApPlayerU: "applayeru",
	TriggerApPlayerT: "applayert",
	TriggerOpPlayer1: "opplayer1",
	TriggerOpPlayer2: "opplayer2",
	TriggerOpPlayer3: "opplayer3",
	TriggerOpPlayer4: "opplayer4",
	TriggerOpPlayer5: "opplayer5",
	TriggerOpPlayerU: "opplayeru",
	TriggerOpPlayerT: "opplayert",

	TriggerAiApPlayer1: "ai_applayer1",
	TriggerAiApPlayer2: "ai_applayer2",
	TriggerAiApPlayer3: "ai_applayer3",
	TriggerAiApPlayer4: "ai_applayer4",
	TriggerAiApPlayer5: "ai_applayer5",
	TriggerAiOpPlayer1: "ai_opplayer1",
	TriggerAiOpPlayer2: "ai_opplayer2",
	TriggerAiOpPlayer3: "ai_opplayer3",
	TriggerAiOpPlayer4: "ai_opplayer4",
	TriggerAiOpPlayer5: "ai_opplayer5",

	TriggerQueue:     "queue",
	TriggerAiQueue1:  "ai_queue1",
	TriggerAiQueue2:  "ai_queue2",
	TriggerAiQueue3:  "ai_queue3",
	TriggerAiQueue4:  "ai_queue4",
	TriggerAiQueue5:  "ai_queue5",
	TriggerAiQueue6:  "ai_queue6",
	TriggerAiQueue7:  "ai_queue7",
	TriggerAiQueue8:  "ai_queue8",
	TriggerAiQueue9:  "ai_queue9",
	TriggerAiQueue10: "ai_queue10",
	TriggerAiQueue11: "ai_queue11",
	TriggerAiQueue12: "ai_queue12",
	TriggerAiQueue13: "ai_queue13",
	TriggerAiQueue14: "ai_queue14",
	TriggerAiQueue15: "ai_queue15",
	TriggerAiQueue16: "ai_queue16",
	TriggerAiQueue17: "ai_queue17",
	TriggerAiQueue18: "ai_queue18",
	TriggerAiQueue19: "ai_queue19",
	TriggerAiQueue20: "ai_queue20",

	TriggerSoftTimer: "softtimer",
	TriggerTimer:     "timer",
	TriggerAiTimer:   "ai_timer",

	TriggerOpHeld1: "opheld1",
	TriggerOpHeld2: "opheld2",
	TriggerOpHeld3: "opheld3",
	TriggerOpHeld4: "opheld4",
	TriggerOpHeld5: "opheld5",
	TriggerOpHeldU: "opheldu",
	TriggerOpHeldT: "opheldt",

	TriggerIfButton:   "if_button",
	TriggerIfClose:    "if_close",
	TriggerInvButton1: "inv_button1",
	TriggerInvButton2: "inv_button2",
	TriggerInvButton3: "inv_button3",
	TriggerInvButton4: "inv_button4",
	TriggerInvButton5: "inv_button5",
	TriggerInvButtonD: "inv_buttond",

	TriggerWalkTrigger:   "walktrigger",
	TriggerAiWalkTrigger: "ai_walktrigger",

	TriggerLogin:       "login",
	TriggerLogout:      "logout",
	TriggerTutorial:    "tutorial",
	TriggerAdvanceStat: "advancestat",
	TriggerMapZone:     "mapzone",
	TriggerMapZoneExit: "mapzoneexit",
	TriggerZone:        "zone",
	TriggerZoneExit:    "zoneexit",
	TriggerChangeStat:  "changestat",
	TriggerAiSpawn:     "ai_spawn",
	TriggerAiDespawn:   "ai_despawn",
}

// String returns the TS-faithful lowered enum name (e.g. TriggerOpNpc1
// returns "opnpc1", TriggerAiQueue4 returns "ai_queue4"). Mirrors TS
// ServerTriggerType.toString at Engine-TS/.../ServerTriggerType.ts:166-170:
//
//	ServerTriggerType[trigger].toLowerCase()
//
// Unknown values return "trigger_<N>" rather than panicking. TS would
// throw on `undefined.toLowerCase()`; Go's nil-handling and the
// debug-only call site (defaultOp under cfg.NodeDebug) make a sentinel
// safer (DEVIATION-NAI-148-D-STRING-FALLBACK).
func (t ServerTriggerType) String() string {
	if name, ok := serverTriggerNames[t]; ok {
		return name
	}
	return fmt.Sprintf("trigger_%d", int(t))
}
