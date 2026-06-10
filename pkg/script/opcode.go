package script

import "fmt"

// Opcode is a RuneScript2 opcode. Numeric values match TS ScriptOpcode.ts exactly;
// the cache bakes them in and they are non-negotiable.
type Opcode uint16

// isLargeOperand reports whether op takes a u32 operand.
// Opcodes with opcode <= 100 that are NOT in the small-operand set are large (u32).
// The small-operand set (u8) within the <=100 range: RETURN, POP_INT_DISCARD,
// POP_STRING_DISCARD, GOSUB, JUMP.
// Opcodes > 100 always take a u8 operand (not large).
//
// This mirrors TS ScriptFile.ts isLargeOperand.
func isLargeOperand(op Opcode) bool {
	if op > 100 {
		return false
	}
	switch op {
	case OpReturn,
		OpPopIntDiscard,
		OpPopStringDiscard,
		OpGosub,
		OpJump:
		return false
	}
	return true
}

// Core language ops (0–99)
const (
	OpPushConstantInt    Opcode = 0
	OpPushVarp           Opcode = 1
	OpPopVarp            Opcode = 2
	OpPushConstantString Opcode = 3
	OpPushVarn           Opcode = 4
	OpPopVarn            Opcode = 5
	OpBranch             Opcode = 6
	OpBranchNot          Opcode = 7
	OpBranchEquals       Opcode = 8
	OpBranchLessThan     Opcode = 9
	OpBranchGreaterThan  Opcode = 10
	OpPushVars           Opcode = 11
	OpPopVars            Opcode = 12

	OpReturn Opcode = 21
	OpGosub  Opcode = 22
	OpJump   Opcode = 23
	OpSwitch Opcode = 24

	OpPushVarbit Opcode = 25 // official (cs2); restored at 254 (TS ScriptOpcode.ts:20)
	OpPopVarbit  Opcode = 27 // official (cs2); restored at 254 (TS ScriptOpcode.ts:21)

	OpBranchLessThanOrEquals    Opcode = 31
	OpBranchGreaterThanOrEquals Opcode = 32
	OpPushIntLocal              Opcode = 33
	OpPopIntLocal               Opcode = 34
	OpPushStringLocal           Opcode = 35
	OpPopStringLocal            Opcode = 36
	OpJoinString                Opcode = 37
	OpPopIntDiscard             Opcode = 38
	OpPopStringDiscard          Opcode = 39
	OpGosubWithParams           Opcode = 40
	OpJumpWithParams            Opcode = 41

	OpDefineArray  Opcode = 44
	OpPushArrayInt Opcode = 45
	OpPopArrayInt  Opcode = 46
)

// Server ops (1000–1999)
// 244 renumbered: HUNTALL+HUNTNEXT moved from 2031–2032 to 1004–1005;
// SPLIT_GET/GETANIM/INIT/LINECOUNT/PAGECOUNT moved from 4513–4517 to 1022–1026;
// STRUCT_PARAM moved from 4700 to 1028; MAP_LIVE deleted;
// NPCCOUNT/ZONECOUNT/LOCCOUNT/OBJCOUNT/MAP_MULTIWAY added.
const (
	OpCoordX          Opcode = 1000
	OpCoordY          Opcode = 1001
	OpCoordZ          Opcode = 1002
	OpDistance        Opcode = 1003
	OpHuntAll         Opcode = 1004
	OpHuntNext        Opcode = 1005
	OpInZone          Opcode = 1006
	OpLineOfSight     Opcode = 1007
	OpLineOfWalk      Opcode = 1008
	OpMapBlocked      Opcode = 1009
	OpMapIndoors      Opcode = 1010
	OpMapClock        Opcode = 1011
	OpMapLocAddUnsafe Opcode = 1012
	OpMapMembers      Opcode = 1013
	OpMapPlayerCount  Opcode = 1014
	OpMapFindSquare   Opcode = 1015
	OpMoveCoord       Opcode = 1016
	OpPlayerCount     Opcode = 1017
	OpProjAnimMap     Opcode = 1018
	OpProjAnimNpc     Opcode = 1019
	OpProjAnimPl      Opcode = 1020
	OpSeqLength       Opcode = 1021
	OpSplitGet        Opcode = 1022
	OpSplitGetAnim    Opcode = 1023
	OpSplitInit       Opcode = 1024
	OpSplitLineCount  Opcode = 1025
	OpSplitPageCount  Opcode = 1026
	OpSpotAnimMap     Opcode = 1027
	OpStructParam     Opcode = 1028
	OpWorldDelay      Opcode = 1029
	OpNpcCount        Opcode = 1030
	OpZoneCount       Opcode = 1031
	OpLocCount        Opcode = 1032
	OpZoneObjCount    Opcode = 1033 // OBJCOUNT (zone-wide); distinct from OBJ_COUNT (OpObjCount=3503)
	OpMapMultiway     Opcode = 1034
)

// Player ops (2000–2499)
// 244 renumbered substantially: many ops shifted due to BAS_* family
// insertion and ALLOWDESIGN moved to 2000 (was AFK_EVENT).
// Renames: READYANIM→BAS_READYANIM, RUNANIM→BAS_RUNNING, TURNANIM→BAS_TURNONSPOT,
// WALKANIM_B→BAS_WALK_B, WALKANIM→BAS_WALK_F, WALKANIM_L→BAS_WALK_L,
// WALKANIM_R→BAS_WALK_R, HINT_PL→HINT_PLAYER, LOWMEM→LOWMEMORY.
// Deleted: STAT_TOTAL, IF_SETRECOL.
// New: BUFFER_FULL, IF_MULTIZONE, IF_OPENOVERLAY, PLAYER_FINDALLZONE,
// PLAYER_FINDNEXT, IF_OPENMAINOVERLAY, LAST_COORD, STRONGQUEUEVARARG
// (vararg keys for queue ops also renumbered).
const (
	OpAllowDesign         Opcode = 2000
	OpAnim                Opcode = 2001
	OpBasReadyAnim        Opcode = 2002
	OpBasRunning          Opcode = 2003
	OpBasTurnOnSpot       Opcode = 2004
	OpBasWalkB            Opcode = 2005
	OpBasWalkF            Opcode = 2006
	OpBasWalkL            Opcode = 2007
	OpBasWalkR            Opcode = 2008
	OpBufferFull          Opcode = 2009
	OpBuildAppearance     Opcode = 2010
	OpBusy                Opcode = 2011
	OpCamLookAt           Opcode = 2012
	OpCamMoveTo           Opcode = 2013
	OpCamReset            Opcode = 2014
	OpCamShake            Opcode = 2015
	OpClearQueue          Opcode = 2016
	OpClearSoftTimer      Opcode = 2017
	OpClearTimer          Opcode = 2018
	OpGetTimer            Opcode = 2019
	OpCoord               Opcode = 2020
	OpDamage              Opcode = 2021
	OpDisplayName         Opcode = 2022
	OpFaceSquare          Opcode = 2023
	OpFindUID             Opcode = 2024
	OpGender              Opcode = 2025
	OpGetQueue            Opcode = 2026
	OpStatAdvance         Opcode = 2027
	OpHeadIconsGet        Opcode = 2028
	OpHeadIconsSet        Opcode = 2029
	OpHealEnergy          Opcode = 2030
	OpHintCoord           Opcode = 2031
	OpHintNpc             Opcode = 2032
	OpHintPlayer          Opcode = 2033
	OpHintStop            Opcode = 2034
	OpIfClose             Opcode = 2035
	OpTutClose            Opcode = 2036
	OpIfMultizone         Opcode = 2037
	OpIfOpenChat          Opcode = 2038
	OpTutOpen             Opcode = 2039
	OpIfOpenMain          Opcode = 2040
	OpIfOpenOverlay       Opcode = 2041
	OpIfOpenMainSide      Opcode = 2042
	OpIfOpenSide          Opcode = 2043
	OpIfSetAnim           Opcode = 2044
	OpIfSetColour         Opcode = 2045
	OpIfSetHide           Opcode = 2046
	OpIfSetModel          Opcode = 2047
	OpIfSetNpcHead        Opcode = 2048
	OpIfSetObject         Opcode = 2049
	OpIfSetPlayerHead     Opcode = 2050
	OpIfSetPosition       Opcode = 2051
	OpIfSetResumeButtons  Opcode = 2052
	OpIfSetTab            Opcode = 2053
	OpIfSetTabActive      Opcode = 2054
	OpTutFlash            Opcode = 2055
	OpIfSetText           Opcode = 2056
	OpLastLoginInfo       Opcode = 2057
	OpLastCom             Opcode = 2058
	OpLastInt             Opcode = 2059
	OpLastItem            Opcode = 2060
	OpLastSlot            Opcode = 2061
	OpLastTargetSlot      Opcode = 2062
	OpLastUseItem         Opcode = 2063
	OpLastUseSlot         Opcode = 2064
	OpLongQueue           Opcode = 2065
	OpMes                 Opcode = 2066
	OpMidiJingle          Opcode = 2067
	OpMidiSong            Opcode = 2068
	OpName                Opcode = 2069
	OpPApRange            Opcode = 2070
	OpPArriveDelay        Opcode = 2071
	OpPCountDialog        Opcode = 2072
	OpPDelay              Opcode = 2073
	OpPExactMove          Opcode = 2074
	OpPFindUID            Opcode = 2075
	OpPLocMerge           Opcode = 2076
	OpPLogout             Opcode = 2077
	OpPPreventLogout      Opcode = 2078
	OpPOpHeld             Opcode = 2079
	OpPOpLoc              Opcode = 2080
	OpPOpNpc              Opcode = 2081
	OpPOpNpcT             Opcode = 2082
	OpPOpObj              Opcode = 2083
	OpPOpPlayer           Opcode = 2084
	OpPOpPlayerT          Opcode = 2085
	OpPPauseButton        Opcode = 2086
	OpPStopAction         Opcode = 2087
	OpPTeleJump           Opcode = 2088
	OpPTeleport           Opcode = 2089
	OpPWalk               Opcode = 2090
	OpPlayerFindAllZone   Opcode = 2091
	OpPlayerFindNext      Opcode = 2092
	OpQueue               Opcode = 2093
	OpSay                 Opcode = 2094
	OpWalkTrigger         Opcode = 2095
	OpSetTimer            Opcode = 2096
	OpSoftTimer           Opcode = 2097
	OpSoundSynth          Opcode = 2098
	OpSpotAnimPl          Opcode = 2099
	OpStaffModLevel       Opcode = 2100
	OpStat                Opcode = 2101
	OpStatAdd             Opcode = 2102
	OpStatBase            Opcode = 2103
	OpStatHeal            Opcode = 2104
	OpStatSub             Opcode = 2105
	OpStatBoost           Opcode = 2106
	OpStatDrain           Opcode = 2107
	OpStatRandom          Opcode = 2108
	OpStrongQueue         Opcode = 2109
	OpUID                 Opcode = 2110
	OpWeakQueue           Opcode = 2111
	OpIfOpenMainOverlay   Opcode = 2112
	OpAfkEvent            Opcode = 2113
	OpLowMemory           Opcode = 2114
	OpSetIdKit            Opcode = 2115
	OpPClearPendingAction Opcode = 2116
	OpGetWalkTrigger      Opcode = 2117
	OpBusy2               Opcode = 2118
	OpFindHero            Opcode = 2119
	OpBothHeroPoints      Opcode = 2120
	OpSetGender           Opcode = 2121
	OpSetSkinColour       Opcode = 2122
	OpPAnimProtect        Opcode = 2123
	OpRunEnergy           Opcode = 2124
	OpWeight              Opcode = 2125
	OpLastCoord           Opcode = 2126
	OpSessionLog          Opcode = 2127
	OpWealthEvent         Opcode = 2128
	OpPRun                Opcode = 2129
	OpPlayerMember        Opcode = 2130
	OpIfSetScrollPos      Opcode = 2131 // new in 245.2 (TS ScriptOpcode.ts:208 @3c16994c)
	OpQueueVarArg         Opcode = 2132 // 2131→2132 at 245.2 (IF_SETSCROLLPOS insert)
	OpLongQueueVarArg     Opcode = 2133
	OpWeakQueueVarArg     Opcode = 2134
	OpStrongQueueVarArg   Opcode = 2135
)

// NPC ops (2500–2999)
// 244 renumbered: ordering changed significantly within 2500–2547.
const (
	OpNpcAdd               Opcode = 2500
	OpNpcAnim              Opcode = 2501
	OpNpcBaseStat          Opcode = 2502
	OpNpcCategory          Opcode = 2503
	OpNpcChangeType        Opcode = 2504
	OpNpcChangeTypeKeepAll Opcode = 2505
	OpNpcCoord             Opcode = 2506
	OpNpcDamage            Opcode = 2507
	OpNpcDel               Opcode = 2508
	OpNpcDelay             Opcode = 2509
	OpNpcFaceSquare        Opcode = 2510
	OpNpcFind              Opcode = 2511
	OpNpcFindCat           Opcode = 2512
	OpNpcFindAllAny        Opcode = 2513
	OpNpcFindAll           Opcode = 2514
	OpNpcFindExact         Opcode = 2515
	OpNpcFindHero          Opcode = 2516
	OpNpcFindAllZone       Opcode = 2517
	OpNpcFindNext          Opcode = 2518
	OpNpcFindUID           Opcode = 2519
	OpNpcGetMode           Opcode = 2520
	OpNpcHeroPoints        Opcode = 2521
	OpNpcName              Opcode = 2522
	OpNpcParam             Opcode = 2523
	OpNpcQueue             Opcode = 2524
	OpNpcRange             Opcode = 2525
	OpNpcSay               Opcode = 2526
	OpNpcHunt              Opcode = 2527
	OpNpcHuntAll           Opcode = 2528
	OpNpcHuntNext          Opcode = 2529
	OpNpcSetHunt           Opcode = 2530
	OpNpcSetHuntMode       Opcode = 2531
	OpNpcSetMode           Opcode = 2532
	OpNpcWalkTrigger       Opcode = 2533
	OpNpcSetTimer          Opcode = 2534
	OpNpcStat              Opcode = 2535
	OpNpcStatAdd           Opcode = 2536
	OpNpcStatHeal          Opcode = 2537
	OpNpcStatSub           Opcode = 2538
	OpNpcTele              Opcode = 2539
	OpNpcType              Opcode = 2540
	OpNpcUID               Opcode = 2541
	OpSpotAnimNpc          Opcode = 2542
	OpNpcWalk              Opcode = 2543
	OpNpcAttackRange       Opcode = 2544
	OpNpcHasOp             Opcode = 2545
	OpNpcArriveDelay       Opcode = 2546
	OpNpcInRange           Opcode = 2547
)

// Loc ops (3000–3499)
// 244: LOC_OP does not exist in this range. LC_OP (the loc-config query form)
// lives at 4104 (OpLcOp). No LOC_OP constant is defined here.
const (
	OpLocAdd         Opcode = 3000
	OpLocAngle       Opcode = 3001
	OpLocAnim        Opcode = 3002
	OpLocCategory    Opcode = 3003
	OpLocChange      Opcode = 3004
	OpLocCoord       Opcode = 3005
	OpLocDel         Opcode = 3006
	OpLocFind        Opcode = 3007
	OpLocFindAllZone Opcode = 3008
	OpLocFindNext    Opcode = 3009
	OpLocName        Opcode = 3010
	OpLocParam       Opcode = 3011
	OpLocShape       Opcode = 3012
	OpLocType        Opcode = 3013
)

// Obj ops (3500–4000)
// 244: OBJ_FIND/FINDALLZONE/FINDNEXT moved from 3505–3507 to 3509–3511.
// OBJ_COUNT (ground-obj stack count) stays at 3503, unchanged.
// The zone-wide OBJCOUNT (1033) is a different op — see OpZoneObjCount above.
const (
	OpObjAdd         Opcode = 3500
	OpObjAddAll      Opcode = 3501
	OpObjCoord       Opcode = 3502
	OpObjCount       Opcode = 3503
	OpObjDel         Opcode = 3504
	OpObjName        Opcode = 3505
	OpObjParam       Opcode = 3506
	OpObjTakeItem    Opcode = 3507
	OpObjType        Opcode = 3508
	OpObjFind        Opcode = 3509
	OpObjFindAllZone Opcode = 3510
	OpObjFindNext    Opcode = 3511
)

// NPC config ops (4000–4099)
const (
	OpNcCategory  Opcode = 4000
	OpNcDebugName Opcode = 4001
	OpNcDesc      Opcode = 4002
	OpNcName      Opcode = 4003
	OpNcOp        Opcode = 4004
	OpNcParam     Opcode = 4005
	OpNcSize      Opcode = 4006
	OpNcVisLevel  Opcode = 4007
)

// Loc config ops (4100–4199)
// 244: LC_LENGTH moved from 4103 to 4107; LC_WIDTH from 4107 to 4106.
// LC_OP stub at 4104 (no TS handler entry, no map entry in 244).
const (
	OpLcCategory  Opcode = 4100
	OpLcDebugName Opcode = 4101
	OpLcDesc      Opcode = 4102
	OpLcName      Opcode = 4103
	OpLcOp        Opcode = 4104
	OpLcParam     Opcode = 4105
	OpLcWidth     Opcode = 4106
	OpLcLength    Opcode = 4107
)

// Obj config ops (4200–4299)
// 244: OC_WEARPOS2/3/WEARPOS reordered (213→4213, 214→4214, 215→4215).
// Was: WEARPOS=4213, WEARPOS2=4214, WEARPOS3=4215.
// Now: WEARPOS2=4213, WEARPOS3=4214, WEARPOS=4215.
const (
	OpOcCategory  Opcode = 4200
	OpOcCert      Opcode = 4201
	OpOcCost      Opcode = 4202
	OpOcDebugName Opcode = 4203
	OpOcDesc      Opcode = 4204
	OpOcIop       Opcode = 4205
	OpOcMembers   Opcode = 4206
	OpOcName      Opcode = 4207
	OpOcOp        Opcode = 4208
	OpOcParam     Opcode = 4209
	OpOcStackable Opcode = 4210
	OpOcTradeable Opcode = 4211
	OpOcUncert    Opcode = 4212
	OpOcWearPos2  Opcode = 4213
	OpOcWearPos3  Opcode = 4214
	OpOcWearPos   Opcode = 4215
	OpOcWeight    Opcode = 4216
)

// Inventory ops (4300–4399)
// 244: entire block renumbered — INV_ALLSTOCK starts at 4300 (not BOTH_DROPSLOT).
const (
	OpInvAllStock        Opcode = 4300
	OpInvSize            Opcode = 4301
	OpInvStockBase       Opcode = 4302
	OpInvAdd             Opcode = 4303
	OpInvChangeSlot      Opcode = 4304
	OpInvClear           Opcode = 4305
	OpInvDel             Opcode = 4306
	OpInvDelSlot         Opcode = 4307
	OpInvDropItem        Opcode = 4308
	OpInvDropItemDelayed Opcode = 4309
	OpInvDropSlot        Opcode = 4310
	OpInvFreeSpace       Opcode = 4311
	OpInvGetNum          Opcode = 4312
	OpInvGetObj          Opcode = 4313
	OpInvItemSpace       Opcode = 4314
	OpInvItemSpace2      Opcode = 4315
	OpInvMoveFromSlot    Opcode = 4316
	OpInvMoveToSlot      Opcode = 4317
	OpBothMoveInv        Opcode = 4318
	OpInvMoveItem        Opcode = 4319
	OpInvMoveItemCert    Opcode = 4320
	OpInvMoveItemUncert  Opcode = 4321
	OpInvSetSlot         Opcode = 4322
	OpInvTotal           Opcode = 4323
	OpInvTotalCat        Opcode = 4324
	OpInvTransmit        Opcode = 4325
	OpInvOtherTransmit   Opcode = 4326
	OpInvStopTransmit    Opcode = 4327
	OpBothDropSlot       Opcode = 4328
	OpInvDropAll         Opcode = 4329
	OpInvTotalParam      Opcode = 4330
	OpInvTotalParamStack Opcode = 4331
	OpInvDebugName       Opcode = 4332
)

// Enum ops (4400–4499)
const (
	OpEnum               Opcode = 4400
	OpEnumGetOutputCount Opcode = 4401
)

// String ops (4500–4599)
// 244: SPLIT_* ops moved to 1022–1026 (server block).
const (
	OpAppendNum           Opcode = 4500
	OpAppend              Opcode = 4501
	OpAppendSignNum       Opcode = 4502
	OpLowercase           Opcode = 4503
	OpTextGender          Opcode = 4504
	OpToString            Opcode = 4505
	OpCompare             Opcode = 4506
	OpTextSwitch          Opcode = 4507
	OpAppendChar          Opcode = 4508
	OpStringLength        Opcode = 4509
	OpSubstring           Opcode = 4510
	OpStringIndexOfChar   Opcode = 4511
	OpStringIndexOfString Opcode = 4512
)

// Number ops (4600–4699)
const (
	OpAdd              Opcode = 4600
	OpSub              Opcode = 4601
	OpMultiply         Opcode = 4602
	OpDivide           Opcode = 4603
	OpRandom           Opcode = 4604
	OpRandomInc        Opcode = 4605
	OpInterpolate      Opcode = 4606
	OpAddPercent       Opcode = 4607
	OpSetBit           Opcode = 4608
	OpClearBit         Opcode = 4609
	OpTestBit          Opcode = 4610
	OpModulo           Opcode = 4611
	OpPow              Opcode = 4612
	OpInvPow           Opcode = 4613
	OpAnd              Opcode = 4614
	OpOr               Opcode = 4615
	OpMin              Opcode = 4616
	OpMax              Opcode = 4617
	OpScale            Opcode = 4618
	OpBitCount         Opcode = 4619
	OpToggleBit        Opcode = 4620
	OpSetBitRange      Opcode = 4621
	OpClearBitRange    Opcode = 4622
	OpGetBitRange      Opcode = 4623
	OpSetBitRangeToInt Opcode = 4624
	OpSinDeg           Opcode = 4625
	OpCosDeg           Opcode = 4626
	OpAtan2Deg         Opcode = 4627
	OpAbs              Opcode = 4628
)

// DB ops (7500–7599)
const (
	OpDbFindWithCount       Opcode = 7500
	OpDbFindNext            Opcode = 7501
	OpDbGetField            Opcode = 7502
	OpDbGetFieldCount       Opcode = 7503
	OpDbListAllWithCount    Opcode = 7504
	OpDbGetRowTable         Opcode = 7505
	OpDbFindByIndex         Opcode = 7506
	OpDbFindRefineWithCount Opcode = 7507
	OpDbFind                Opcode = 7508
	OpDbFindRefine          Opcode = 7509
	OpDbListAll             Opcode = 7510
)

// Debug ops (10000–11000)
// 244: ERROR=10000, MAP_PRODUCTION=10001, MAP_LAST*=10002–10013,
// TIMESPENT=10014, GETTIMESPENT=10015, CONSOLE=10016.
const (
	OpError               Opcode = 10000
	OpMapProduction       Opcode = 10001
	OpMapLastClock        Opcode = 10002
	OpMapLastWorld        Opcode = 10003
	OpMapLastClientIn     Opcode = 10004
	OpMapLastNpc          Opcode = 10005
	OpMapLastPlayer       Opcode = 10006
	OpMapLastLogout       Opcode = 10007
	OpMapLastLogin        Opcode = 10008
	OpMapLastZone         Opcode = 10009
	OpMapLastClientOut    Opcode = 10010
	OpMapLastCleanup      Opcode = 10011
	OpMapLastBandwidthIn  Opcode = 10012
	OpMapLastBandwidthOut Opcode = 10013
	OpTimeSpent           Opcode = 10014
	OpGetTimeSpent        Opcode = 10015
	OpConsole             Opcode = 10016
)

// String returns the uppercase mnemonic for the opcode, e.g. "PUSH_CONSTANT_INT".
// Falls back to "opcode_<n>" for values not in the defined set.
func (o Opcode) String() string {
	switch o {
	case OpPushConstantInt:
		return "PUSH_CONSTANT_INT"
	case OpPushVarp:
		return "PUSH_VARP"
	case OpPopVarp:
		return "POP_VARP"
	case OpPushConstantString:
		return "PUSH_CONSTANT_STRING"
	case OpPushVarn:
		return "PUSH_VARN"
	case OpPopVarn:
		return "POP_VARN"
	case OpBranch:
		return "BRANCH"
	case OpBranchNot:
		return "BRANCH_NOT"
	case OpBranchEquals:
		return "BRANCH_EQUALS"
	case OpBranchLessThan:
		return "BRANCH_LESS_THAN"
	case OpBranchGreaterThan:
		return "BRANCH_GREATER_THAN"
	case OpPushVars:
		return "PUSH_VARS"
	case OpPopVars:
		return "POP_VARS"
	case OpReturn:
		return "RETURN"
	case OpGosub:
		return "GOSUB"
	case OpJump:
		return "JUMP"
	case OpSwitch:
		return "SWITCH"
	case OpPushVarbit:
		return "PUSH_VARBIT"
	case OpPopVarbit:
		return "POP_VARBIT"
	case OpBranchLessThanOrEquals:
		return "BRANCH_LESS_THAN_OR_EQUALS"
	case OpBranchGreaterThanOrEquals:
		return "BRANCH_GREATER_THAN_OR_EQUALS"
	case OpPushIntLocal:
		return "PUSH_INT_LOCAL"
	case OpPopIntLocal:
		return "POP_INT_LOCAL"
	case OpPushStringLocal:
		return "PUSH_STRING_LOCAL"
	case OpPopStringLocal:
		return "POP_STRING_LOCAL"
	case OpJoinString:
		return "JOIN_STRING"
	case OpPopIntDiscard:
		return "POP_INT_DISCARD"
	case OpPopStringDiscard:
		return "POP_STRING_DISCARD"
	case OpGosubWithParams:
		return "GOSUB_WITH_PARAMS"
	case OpJumpWithParams:
		return "JUMP_WITH_PARAMS"
	case OpDefineArray:
		return "DEFINE_ARRAY"
	case OpPushArrayInt:
		return "PUSH_ARRAY_INT"
	case OpPopArrayInt:
		return "POP_ARRAY_INT"
	case OpCoordX:
		return "COORDX"
	case OpCoordY:
		return "COORDY"
	case OpCoordZ:
		return "COORDZ"
	case OpDistance:
		return "DISTANCE"
	case OpHuntAll:
		return "HUNTALL"
	case OpHuntNext:
		return "HUNTNEXT"
	case OpInZone:
		return "INZONE"
	case OpLineOfSight:
		return "LINEOFSIGHT"
	case OpLineOfWalk:
		return "LINEOFWALK"
	case OpMapBlocked:
		return "MAP_BLOCKED"
	case OpMapIndoors:
		return "MAP_INDOORS"
	case OpMapClock:
		return "MAP_CLOCK"
	case OpMapLocAddUnsafe:
		return "MAP_LOCADDUNSAFE"
	case OpMapMembers:
		return "MAP_MEMBERS"
	case OpMapPlayerCount:
		return "MAP_PLAYERCOUNT"
	case OpMapFindSquare:
		return "MAP_FINDSQUARE"
	case OpMoveCoord:
		return "MOVECOORD"
	case OpPlayerCount:
		return "PLAYERCOUNT"
	case OpProjAnimMap:
		return "PROJANIM_MAP"
	case OpProjAnimNpc:
		return "PROJANIM_NPC"
	case OpProjAnimPl:
		return "PROJANIM_PL"
	case OpSeqLength:
		return "SEQLENGTH"
	case OpSplitGet:
		return "SPLIT_GET"
	case OpSplitGetAnim:
		return "SPLIT_GETANIM"
	case OpSplitInit:
		return "SPLIT_INIT"
	case OpSplitLineCount:
		return "SPLIT_LINECOUNT"
	case OpSplitPageCount:
		return "SPLIT_PAGECOUNT"
	case OpSpotAnimMap:
		return "SPOTANIM_MAP"
	case OpStructParam:
		return "STRUCT_PARAM"
	case OpWorldDelay:
		return "WORLD_DELAY"
	case OpNpcCount:
		return "NPCCOUNT"
	case OpZoneCount:
		return "ZONECOUNT"
	case OpLocCount:
		return "LOCCOUNT"
	case OpZoneObjCount:
		return "OBJCOUNT"
	case OpMapMultiway:
		return "MAP_MULTIWAY"
	case OpAllowDesign:
		return "ALLOWDESIGN"
	case OpAnim:
		return "ANIM"
	case OpBasReadyAnim:
		return "BAS_READYANIM"
	case OpBasRunning:
		return "BAS_RUNNING"
	case OpBasTurnOnSpot:
		return "BAS_TURNONSPOT"
	case OpBasWalkB:
		return "BAS_WALK_B"
	case OpBasWalkF:
		return "BAS_WALK_F"
	case OpBasWalkL:
		return "BAS_WALK_L"
	case OpBasWalkR:
		return "BAS_WALK_R"
	case OpBufferFull:
		return "BUFFER_FULL"
	case OpBuildAppearance:
		return "BUILDAPPEARANCE"
	case OpBusy:
		return "BUSY"
	case OpCamLookAt:
		return "CAM_LOOKAT"
	case OpCamMoveTo:
		return "CAM_MOVETO"
	case OpCamReset:
		return "CAM_RESET"
	case OpCamShake:
		return "CAM_SHAKE"
	case OpClearQueue:
		return "CLEARQUEUE"
	case OpClearSoftTimer:
		return "CLEARSOFTTIMER"
	case OpClearTimer:
		return "CLEARTIMER"
	case OpGetTimer:
		return "GETTIMER"
	case OpCoord:
		return "COORD"
	case OpDamage:
		return "DAMAGE"
	case OpDisplayName:
		return "DISPLAYNAME"
	case OpFaceSquare:
		return "FACESQUARE"
	case OpFindUID:
		return "FINDUID"
	case OpGender:
		return "GENDER"
	case OpGetQueue:
		return "GETQUEUE"
	case OpStatAdvance:
		return "STAT_ADVANCE"
	case OpHeadIconsGet:
		return "HEADICONS_GET"
	case OpHeadIconsSet:
		return "HEADICONS_SET"
	case OpHealEnergy:
		return "HEALENERGY"
	case OpHintCoord:
		return "HINT_COORD"
	case OpHintNpc:
		return "HINT_NPC"
	case OpHintPlayer:
		return "HINT_PLAYER"
	case OpHintStop:
		return "HINT_STOP"
	case OpIfClose:
		return "IF_CLOSE"
	case OpTutClose:
		return "TUT_CLOSE"
	case OpIfMultizone:
		return "IF_MULTIZONE"
	case OpIfOpenChat:
		return "IF_OPENCHAT"
	case OpTutOpen:
		return "TUT_OPEN"
	case OpIfOpenMain:
		return "IF_OPENMAIN"
	case OpIfOpenOverlay:
		return "IF_OPENOVERLAY"
	case OpIfOpenMainSide:
		return "IF_OPENMAIN_SIDE"
	case OpIfOpenSide:
		return "IF_OPENSIDE"
	case OpIfSetAnim:
		return "IF_SETANIM"
	case OpIfSetColour:
		return "IF_SETCOLOUR"
	case OpIfSetHide:
		return "IF_SETHIDE"
	case OpIfSetModel:
		return "IF_SETMODEL"
	case OpIfSetNpcHead:
		return "IF_SETNPCHEAD"
	case OpIfSetObject:
		return "IF_SETOBJECT"
	case OpIfSetPlayerHead:
		return "IF_SETPLAYERHEAD"
	case OpIfSetPosition:
		return "IF_SETPOSITION"
	case OpIfSetResumeButtons:
		return "IF_SETRESUMEBUTTONS"
	case OpIfSetTab:
		return "IF_SETTAB"
	case OpIfSetTabActive:
		return "IF_SETTABACTIVE"
	case OpTutFlash:
		return "TUT_FLASH"
	case OpIfSetText:
		return "IF_SETTEXT"
	case OpLastLoginInfo:
		return "LAST_LOGIN_INFO"
	case OpLastCom:
		return "LAST_COM"
	case OpLastInt:
		return "LAST_INT"
	case OpLastItem:
		return "LAST_ITEM"
	case OpLastSlot:
		return "LAST_SLOT"
	case OpLastTargetSlot:
		return "LAST_TARGETSLOT"
	case OpLastUseItem:
		return "LAST_USEITEM"
	case OpLastUseSlot:
		return "LAST_USESLOT"
	case OpLongQueue:
		return "LONGQUEUE"
	case OpMes:
		return "MES"
	case OpMidiJingle:
		return "MIDI_JINGLE"
	case OpMidiSong:
		return "MIDI_SONG"
	case OpName:
		return "NAME"
	case OpPApRange:
		return "P_APRANGE"
	case OpPArriveDelay:
		return "P_ARRIVEDELAY"
	case OpPCountDialog:
		return "P_COUNTDIALOG"
	case OpPDelay:
		return "P_DELAY"
	case OpPExactMove:
		return "P_EXACTMOVE"
	case OpPFindUID:
		return "P_FINDUID"
	case OpPLocMerge:
		return "P_LOCMERGE"
	case OpPLogout:
		return "P_LOGOUT"
	case OpPPreventLogout:
		return "P_PREVENTLOGOUT"
	case OpPOpHeld:
		return "P_OPHELD"
	case OpPOpLoc:
		return "P_OPLOC"
	case OpPOpNpc:
		return "P_OPNPC"
	case OpPOpNpcT:
		return "P_OPNPCT"
	case OpPOpObj:
		return "P_OPOBJ"
	case OpPOpPlayer:
		return "P_OPPLAYER"
	case OpPOpPlayerT:
		return "P_OPPLAYERT"
	case OpPPauseButton:
		return "P_PAUSEBUTTON"
	case OpPStopAction:
		return "P_STOPACTION"
	case OpPTeleJump:
		return "P_TELEJUMP"
	case OpPTeleport:
		return "P_TELEPORT"
	case OpPWalk:
		return "P_WALK"
	case OpPlayerFindAllZone:
		return "PLAYER_FINDALLZONE"
	case OpPlayerFindNext:
		return "PLAYER_FINDNEXT"
	case OpQueue:
		return "QUEUE"
	case OpSay:
		return "SAY"
	case OpWalkTrigger:
		return "WALKTRIGGER"
	case OpSetTimer:
		return "SETTIMER"
	case OpSoftTimer:
		return "SOFTTIMER"
	case OpSoundSynth:
		return "SOUND_SYNTH"
	case OpSpotAnimPl:
		return "SPOTANIM_PL"
	case OpStaffModLevel:
		return "STAFFMODLEVEL"
	case OpStat:
		return "STAT"
	case OpStatAdd:
		return "STAT_ADD"
	case OpStatBase:
		return "STAT_BASE"
	case OpStatHeal:
		return "STAT_HEAL"
	case OpStatSub:
		return "STAT_SUB"
	case OpStatBoost:
		return "STAT_BOOST"
	case OpStatDrain:
		return "STAT_DRAIN"
	case OpStatRandom:
		return "STAT_RANDOM"
	case OpStrongQueue:
		return "STRONGQUEUE"
	case OpUID:
		return "UID"
	case OpWeakQueue:
		return "WEAKQUEUE"
	case OpIfOpenMainOverlay:
		return "IF_OPENMAINOVERLAY"
	case OpAfkEvent:
		return "AFK_EVENT"
	case OpLowMemory:
		return "LOWMEMORY"
	case OpSetIdKit:
		return "SETIDKIT"
	case OpPClearPendingAction:
		return "P_CLEARPENDINGACTION"
	case OpGetWalkTrigger:
		return "GETWALKTRIGGER"
	case OpBusy2:
		return "BUSY2"
	case OpFindHero:
		return "FINDHERO"
	case OpBothHeroPoints:
		return "BOTH_HEROPOINTS"
	case OpSetGender:
		return "SETGENDER"
	case OpSetSkinColour:
		return "SETSKINCOLOUR"
	case OpPAnimProtect:
		return "P_ANIMPROTECT"
	case OpRunEnergy:
		return "RUNENERGY"
	case OpWeight:
		return "WEIGHT"
	case OpLastCoord:
		return "LAST_COORD"
	case OpSessionLog:
		return "SESSION_LOG"
	case OpWealthEvent:
		return "WEALTH_EVENT"
	case OpPRun:
		return "P_RUN"
	case OpPlayerMember:
		return "PLAYERMEMBER"
	case OpIfSetScrollPos:
		return "IF_SETSCROLLPOS"
	case OpQueueVarArg:
		return "QUEUE*"
	case OpLongQueueVarArg:
		return "LONGQUEUE*"
	case OpWeakQueueVarArg:
		return "WEAKQUEUE*"
	case OpStrongQueueVarArg:
		return "STRONGQUEUE*"
	case OpNpcAdd:
		return "NPC_ADD"
	case OpNpcAnim:
		return "NPC_ANIM"
	case OpNpcBaseStat:
		return "NPC_BASESTAT"
	case OpNpcCategory:
		return "NPC_CATEGORY"
	case OpNpcChangeType:
		return "NPC_CHANGETYPE"
	case OpNpcChangeTypeKeepAll:
		return "NPC_CHANGETYPE_KEEPALL"
	case OpNpcCoord:
		return "NPC_COORD"
	case OpNpcDamage:
		return "NPC_DAMAGE"
	case OpNpcDel:
		return "NPC_DEL"
	case OpNpcDelay:
		return "NPC_DELAY"
	case OpNpcFaceSquare:
		return "NPC_FACESQUARE"
	case OpNpcFind:
		return "NPC_FIND"
	case OpNpcFindCat:
		return "NPC_FINDCAT"
	case OpNpcFindAllAny:
		return "NPC_FINDALLANY"
	case OpNpcFindAll:
		return "NPC_FINDALL"
	case OpNpcFindExact:
		return "NPC_FINDEXACT"
	case OpNpcFindHero:
		return "NPC_FINDHERO"
	case OpNpcFindAllZone:
		return "NPC_FINDALLZONE"
	case OpNpcFindNext:
		return "NPC_FINDNEXT"
	case OpNpcFindUID:
		return "NPC_FINDUID"
	case OpNpcGetMode:
		return "NPC_GETMODE"
	case OpNpcHeroPoints:
		return "NPC_HEROPOINTS"
	case OpNpcName:
		return "NPC_NAME"
	case OpNpcParam:
		return "NPC_PARAM"
	case OpNpcQueue:
		return "NPC_QUEUE"
	case OpNpcRange:
		return "NPC_RANGE"
	case OpNpcSay:
		return "NPC_SAY"
	case OpNpcHunt:
		return "NPC_HUNT"
	case OpNpcHuntAll:
		return "NPC_HUNTALL"
	case OpNpcHuntNext:
		return "NPC_HUNTNEXT"
	case OpNpcSetHunt:
		return "NPC_SETHUNT"
	case OpNpcSetHuntMode:
		return "NPC_SETHUNTMODE"
	case OpNpcSetMode:
		return "NPC_SETMODE"
	case OpNpcWalkTrigger:
		return "NPC_WALKTRIGGER"
	case OpNpcSetTimer:
		return "NPC_SETTIMER"
	case OpNpcStat:
		return "NPC_STAT"
	case OpNpcStatAdd:
		return "NPC_STATADD"
	case OpNpcStatHeal:
		return "NPC_STATHEAL"
	case OpNpcStatSub:
		return "NPC_STATSUB"
	case OpNpcTele:
		return "NPC_TELE"
	case OpNpcType:
		return "NPC_TYPE"
	case OpNpcUID:
		return "NPC_UID"
	case OpSpotAnimNpc:
		return "SPOTANIM_NPC"
	case OpNpcWalk:
		return "NPC_WALK"
	case OpNpcAttackRange:
		return "NPC_ATTACKRANGE"
	case OpNpcHasOp:
		return "NPC_HASOP"
	case OpNpcArriveDelay:
		return "NPC_ARRIVEDELAY"
	case OpNpcInRange:
		return "NPC_INRANGE"
	case OpLocAdd:
		return "LOC_ADD"
	case OpLocAngle:
		return "LOC_ANGLE"
	case OpLocAnim:
		return "LOC_ANIM"
	case OpLocCategory:
		return "LOC_CATEGORY"
	case OpLocChange:
		return "LOC_CHANGE"
	case OpLocCoord:
		return "LOC_COORD"
	case OpLocDel:
		return "LOC_DEL"
	case OpLocFind:
		return "LOC_FIND"
	case OpLocFindAllZone:
		return "LOC_FINDALLZONE"
	case OpLocFindNext:
		return "LOC_FINDNEXT"
	case OpLocName:
		return "LOC_NAME"
	case OpLocParam:
		return "LOC_PARAM"
	case OpLocShape:
		return "LOC_SHAPE"
	case OpLocType:
		return "LOC_TYPE"
	case OpObjAdd:
		return "OBJ_ADD"
	case OpObjAddAll:
		return "OBJ_ADDALL"
	case OpObjCount:
		return "OBJ_COUNT"
	case OpObjFind:
		return "OBJ_FIND"
	case OpObjFindAllZone:
		return "OBJ_FINDALLZONE"
	case OpObjFindNext:
		return "OBJ_FINDNEXT"
	case OpObjCoord:
		return "OBJ_COORD"
	case OpObjDel:
		return "OBJ_DEL"
	case OpObjName:
		return "OBJ_NAME"
	case OpObjParam:
		return "OBJ_PARAM"
	case OpObjTakeItem:
		return "OBJ_TAKEITEM"
	case OpObjType:
		return "OBJ_TYPE"
	case OpNcCategory:
		return "NC_CATEGORY"
	case OpNcDebugName:
		return "NC_DEBUGNAME"
	case OpNcDesc:
		return "NC_DESC"
	case OpNcName:
		return "NC_NAME"
	case OpNcOp:
		return "NC_OP"
	case OpNcParam:
		return "NC_PARAM"
	case OpNcSize:
		return "NC_SIZE"
	case OpNcVisLevel:
		return "NC_VISLEVEL"
	case OpLcCategory:
		return "LC_CATEGORY"
	case OpLcDebugName:
		return "LC_DEBUGNAME"
	case OpLcDesc:
		return "LC_DESC"
	case OpLcLength:
		return "LC_LENGTH"
	case OpLcName:
		return "LC_NAME"
	case OpLcOp:
		return "LC_OP"
	case OpLcParam:
		return "LC_PARAM"
	case OpLcWidth:
		return "LC_WIDTH"
	case OpOcCategory:
		return "OC_CATEGORY"
	case OpOcCert:
		return "OC_CERT"
	case OpOcCost:
		return "OC_COST"
	case OpOcDebugName:
		return "OC_DEBUGNAME"
	case OpOcDesc:
		return "OC_DESC"
	case OpOcIop:
		return "OC_IOP"
	case OpOcMembers:
		return "OC_MEMBERS"
	case OpOcName:
		return "OC_NAME"
	case OpOcOp:
		return "OC_OP"
	case OpOcParam:
		return "OC_PARAM"
	case OpOcStackable:
		return "OC_STACKABLE"
	case OpOcTradeable:
		return "OC_TRADEABLE"
	case OpOcUncert:
		return "OC_UNCERT"
	case OpOcWearPos2:
		return "OC_WEARPOS2"
	case OpOcWearPos3:
		return "OC_WEARPOS3"
	case OpOcWearPos:
		return "OC_WEARPOS"
	case OpOcWeight:
		return "OC_WEIGHT"
	case OpInvAllStock:
		return "INV_ALLSTOCK"
	case OpInvSize:
		return "INV_SIZE"
	case OpInvStockBase:
		return "INV_STOCKBASE"
	case OpInvAdd:
		return "INV_ADD"
	case OpInvChangeSlot:
		return "INV_CHANGESLOT"
	case OpInvClear:
		return "INV_CLEAR"
	case OpInvDel:
		return "INV_DEL"
	case OpInvDelSlot:
		return "INV_DELSLOT"
	case OpInvDropItem:
		return "INV_DROPITEM"
	case OpInvDropItemDelayed:
		return "INV_DROPITEM_DELAYED"
	case OpInvDropSlot:
		return "INV_DROPSLOT"
	case OpInvFreeSpace:
		return "INV_FREESPACE"
	case OpInvGetNum:
		return "INV_GETNUM"
	case OpInvGetObj:
		return "INV_GETOBJ"
	case OpInvItemSpace:
		return "INV_ITEMSPACE"
	case OpInvItemSpace2:
		return "INV_ITEMSPACE2"
	case OpInvMoveFromSlot:
		return "INV_MOVEFROMSLOT"
	case OpInvMoveToSlot:
		return "INV_MOVETOSLOT"
	case OpBothMoveInv:
		return "BOTH_MOVEINV"
	case OpInvMoveItem:
		return "INV_MOVEITEM"
	case OpInvMoveItemCert:
		return "INV_MOVEITEM_CERT"
	case OpInvMoveItemUncert:
		return "INV_MOVEITEM_UNCERT"
	case OpInvSetSlot:
		return "INV_SETSLOT"
	case OpInvTotal:
		return "INV_TOTAL"
	case OpInvTotalCat:
		return "INV_TOTALCAT"
	case OpInvTransmit:
		return "INV_TRANSMIT"
	case OpInvOtherTransmit:
		return "INVOTHER_TRANSMIT"
	case OpInvStopTransmit:
		return "INV_STOPTRANSMIT"
	case OpBothDropSlot:
		return "BOTH_DROPSLOT"
	case OpInvDropAll:
		return "INV_DROPALL"
	case OpInvTotalParam:
		return "INV_TOTALPARAM"
	case OpInvTotalParamStack:
		return "INV_TOTALPARAM_STACK"
	case OpInvDebugName:
		return "INV_DEBUGNAME"
	case OpEnum:
		return "ENUM"
	case OpEnumGetOutputCount:
		return "ENUM_GETOUTPUTCOUNT"
	case OpAppendNum:
		return "APPEND_NUM"
	case OpAppend:
		return "APPEND"
	case OpAppendSignNum:
		return "APPEND_SIGNNUM"
	case OpLowercase:
		return "LOWERCASE"
	case OpTextGender:
		return "TEXT_GENDER"
	case OpToString:
		return "TOSTRING"
	case OpCompare:
		return "COMPARE"
	case OpTextSwitch:
		return "TEXT_SWITCH"
	case OpAppendChar:
		return "APPEND_CHAR"
	case OpStringLength:
		return "STRING_LENGTH"
	case OpSubstring:
		return "SUBSTRING"
	case OpStringIndexOfChar:
		return "STRING_INDEXOF_CHAR"
	case OpStringIndexOfString:
		return "STRING_INDEXOF_STRING"
	case OpAdd:
		return "ADD"
	case OpSub:
		return "SUB"
	case OpMultiply:
		return "MULTIPLY"
	case OpDivide:
		return "DIVIDE"
	case OpRandom:
		return "RANDOM"
	case OpRandomInc:
		return "RANDOMINC"
	case OpInterpolate:
		return "INTERPOLATE"
	case OpAddPercent:
		return "ADDPERCENT"
	case OpSetBit:
		return "SETBIT"
	case OpClearBit:
		return "CLEARBIT"
	case OpTestBit:
		return "TESTBIT"
	case OpModulo:
		return "MODULO"
	case OpPow:
		return "POW"
	case OpInvPow:
		return "INVPOW"
	case OpAnd:
		return "AND"
	case OpOr:
		return "OR"
	case OpMin:
		return "MIN"
	case OpMax:
		return "MAX"
	case OpScale:
		return "SCALE"
	case OpBitCount:
		return "BITCOUNT"
	case OpToggleBit:
		return "TOGGLEBIT"
	case OpSetBitRange:
		return "SETBIT_RANGE"
	case OpClearBitRange:
		return "CLEARBIT_RANGE"
	case OpGetBitRange:
		return "GETBIT_RANGE"
	case OpSetBitRangeToInt:
		return "SETBIT_RANGE_TOINT"
	case OpSinDeg:
		return "SIN_DEG"
	case OpCosDeg:
		return "COS_DEG"
	case OpAtan2Deg:
		return "ATAN2_DEG"
	case OpAbs:
		return "ABS"
	case OpDbFindWithCount:
		return "DB_FIND_WITH_COUNT"
	case OpDbFindNext:
		return "DB_FINDNEXT"
	case OpDbGetField:
		return "DB_GETFIELD"
	case OpDbGetFieldCount:
		return "DB_GETFIELDCOUNT"
	case OpDbListAllWithCount:
		return "DB_LISTALL_WITH_COUNT"
	case OpDbGetRowTable:
		return "DB_GETROWTABLE"
	case OpDbFindByIndex:
		return "DB_FINDBYINDEX"
	case OpDbFindRefineWithCount:
		return "DB_FIND_REFINE_WITH_COUNT"
	case OpDbFind:
		return "DB_FIND"
	case OpDbFindRefine:
		return "DB_FIND_REFINE"
	case OpDbListAll:
		return "DB_LISTALL"
	case OpError:
		return "ERROR"
	case OpMapProduction:
		return "MAP_PRODUCTION"
	case OpMapLastClock:
		return "MAP_LASTCLOCK"
	case OpMapLastWorld:
		return "MAP_LASTWORLD"
	case OpMapLastClientIn:
		return "MAP_LASTCLIENTIN"
	case OpMapLastNpc:
		return "MAP_LASTNPC"
	case OpMapLastPlayer:
		return "MAP_LASTPLAYER"
	case OpMapLastLogout:
		return "MAP_LASTLOGOUT"
	case OpMapLastLogin:
		return "MAP_LASTLOGIN"
	case OpMapLastZone:
		return "MAP_LASTZONE"
	case OpMapLastClientOut:
		return "MAP_LASTCLIENTOUT"
	case OpMapLastCleanup:
		return "MAP_LASTCLEANUP"
	case OpMapLastBandwidthIn:
		return "MAP_LASTBANDWIDTHIN"
	case OpMapLastBandwidthOut:
		return "MAP_LASTBANDWIDTHOUT"
	case OpTimeSpent:
		return "TIMESPENT"
	case OpGetTimeSpent:
		return "GETTIMESPENT"
	case OpConsole:
		return "CONSOLE"
	default:
		return fmt.Sprintf("opcode_%d", uint16(o))
	}
}
