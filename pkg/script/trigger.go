package script

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
