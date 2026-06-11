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
	OpPushConstantInt           Opcode = 0
	OpPushVarp                  Opcode = 1
	OpPopVarp                   Opcode = 2
	OpPushConstantString        Opcode = 3
	OpPushVarn                  Opcode = 4
	OpPopVarn                   Opcode = 5
	OpBranch                    Opcode = 6
	OpBranchNot                 Opcode = 7
	OpBranchEquals              Opcode = 8
	OpBranchLessThan            Opcode = 9
	OpBranchGreaterThan         Opcode = 10
	OpPushVars                  Opcode = 11
	OpPopVars                   Opcode = 12
	OpReturn                    Opcode = 21
	OpGosub                     Opcode = 22
	OpJump                      Opcode = 23
	OpSwitch                    Opcode = 24
	OpPushVarbit                Opcode = 25
	OpPopVarbit                 Opcode = 27
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
	OpDefineArray               Opcode = 44
	OpPushArrayInt              Opcode = 45
	OpPopArrayInt               Opcode = 46
)

// Server ops (1000–1999)
// 2e3bcf43 (254 pin-advance): HUNTALL/HUNTNEXT moved back to the player
// block (2031–2032); SPLIT_* back to 4513–4517; STRUCT_PARAM back to 4700;
// NPCCOUNT/ZONECOUNT/LOCCOUNT/OBJCOUNT (1030–1033) deleted; MAP_LIVE
// restored at 1011; MIDI_LENGTH added at 1022.
const (
	OpCoordX          Opcode = 1000
	OpCoordY          Opcode = 1001
	OpCoordZ          Opcode = 1002
	OpDistance        Opcode = 1003
	OpInZone          Opcode = 1004
	OpLineOfSight     Opcode = 1005
	OpLineOfWalk      Opcode = 1006
	OpMapBlocked      Opcode = 1007
	OpMapClock        Opcode = 1008
	OpMapFindSquare   Opcode = 1009
	OpMapIndoors      Opcode = 1010
	OpMapLive         Opcode = 1011
	OpMapLocAddUnsafe Opcode = 1012
	OpMapMembers      Opcode = 1013
	OpMapMultiway     Opcode = 1014
	OpMapPlayerCount  Opcode = 1015
	OpMoveCoord       Opcode = 1016
	OpPlayerCount     Opcode = 1017
	OpProjAnimMap     Opcode = 1018
	OpSeqLength       Opcode = 1019
	OpSpotAnimMap     Opcode = 1020
	OpWorldDelay      Opcode = 1021
	OpMidiLength      Opcode = 1022
)

// Player ops (2000–2499)
// 2e3bcf43 (254 pin-advance): 418→396 enum regen. BAS_* family renamed
// back (READYANIM/RUNANIM/TURNANIM/WALKANIM{,_B,_L,_R}) and relocated;
// HINT_PLAYER→HINT_PL (now reads activePlayer2, no uid pop);
// IF_SETRESUMEBUTTONS→IF_ADDRESUMEBUTTON (appends one com id);
// LOWMEMORY→LOWMEM; AFK_EVENT back at 2000; HUNTALL/HUNTNEXT rejoin at
// 2031–2032; BUFFER_FULL/IF_MULTIZONE/PLAYER_FINDALLZONE/PLAYER_FINDNEXT/
// IF_OPENMAINOVERLAY/LAST_COORD deleted.
const (
	OpAfkEvent            Opcode = 2000
	OpAllowDesign         Opcode = 2001
	OpAnim                Opcode = 2002
	OpBothHeroPoints      Opcode = 2003
	OpBuildAppearance     Opcode = 2004
	OpBusy                Opcode = 2005
	OpBusy2               Opcode = 2006
	OpCamLookAt           Opcode = 2007
	OpCamMoveTo           Opcode = 2008
	OpCamReset            Opcode = 2009
	OpCamShake            Opcode = 2010
	OpClearQueue          Opcode = 2011
	OpClearSoftTimer      Opcode = 2012
	OpClearTimer          Opcode = 2013
	OpCoord               Opcode = 2014
	OpDamage              Opcode = 2015
	OpDisplayName         Opcode = 2016
	OpFaceSquare          Opcode = 2017
	OpFindHero            Opcode = 2018
	OpFindUID             Opcode = 2019
	OpGender              Opcode = 2020
	OpGetQueue            Opcode = 2021
	OpGetTimer            Opcode = 2022
	OpGetWalkTrigger      Opcode = 2023
	OpHeadIconsGet        Opcode = 2024
	OpHeadIconsSet        Opcode = 2025
	OpHealEnergy          Opcode = 2026
	OpHintCoord           Opcode = 2027
	OpHintNpc             Opcode = 2028
	OpHintPl              Opcode = 2029
	OpHintStop            Opcode = 2030
	OpHuntAll             Opcode = 2031
	OpHuntNext            Opcode = 2032
	OpIfClose             Opcode = 2033
	OpIfOpenChat          Opcode = 2034
	OpIfOpenMainSide      Opcode = 2035
	OpIfOpenMain          Opcode = 2036
	OpIfOpenOverlay       Opcode = 2037
	OpIfOpenSide          Opcode = 2038
	OpIfSetAnim           Opcode = 2039
	OpIfSetColour         Opcode = 2040
	OpIfSetHide           Opcode = 2041
	OpIfSetModel          Opcode = 2042
	OpIfSetNpcHead        Opcode = 2043
	OpIfSetObject         Opcode = 2044
	OpIfSetPlayerHead     Opcode = 2045
	OpIfSetPosition       Opcode = 2046
	OpIfAddResumeButton   Opcode = 2047
	OpIfSetScrollPos      Opcode = 2048
	OpIfSetTab            Opcode = 2049
	OpIfSetTabActive      Opcode = 2050
	OpIfSetText           Opcode = 2051
	OpLastCom             Opcode = 2052
	OpLastInt             Opcode = 2053
	OpLastItem            Opcode = 2054
	OpLastLoginInfo       Opcode = 2055
	OpLastSlot            Opcode = 2056
	OpLastTargetSlot      Opcode = 2057
	OpLastUseItem         Opcode = 2058
	OpLastUseSlot         Opcode = 2059
	OpLongQueue           Opcode = 2060
	OpLongQueueVarArg     Opcode = 2061
	OpLowMem              Opcode = 2062
	OpMes                 Opcode = 2063
	OpMidiJingle          Opcode = 2064
	OpMidiSong            Opcode = 2065
	OpName                Opcode = 2066
	OpPAnimProtect        Opcode = 2067
	OpPApRange            Opcode = 2068
	OpPArriveDelay        Opcode = 2069
	OpPClearPendingAction Opcode = 2070
	OpPCountDialog        Opcode = 2071
	OpPDelay              Opcode = 2072
	OpPExactMove          Opcode = 2073
	OpPFindUID            Opcode = 2074
	OpPLocMerge           Opcode = 2075
	OpPLogout             Opcode = 2076
	OpPOpHeld             Opcode = 2077
	OpPOpLoc              Opcode = 2078
	OpPOpNpc              Opcode = 2079
	OpPOpNpcT             Opcode = 2080
	OpPOpObj              Opcode = 2081
	OpPOpPlayer           Opcode = 2082
	OpPOpPlayerT          Opcode = 2083
	OpPPauseButton        Opcode = 2084
	OpPPreventLogout      Opcode = 2085
	OpPRun                Opcode = 2086
	OpPStopAction         Opcode = 2087
	OpPTeleJump           Opcode = 2088
	OpPTeleport           Opcode = 2089
	OpPWalk               Opcode = 2090
	OpPlayerMember        Opcode = 2091
	OpProjAnimPl          Opcode = 2092
	OpQueue               Opcode = 2093
	OpQueueVarArg         Opcode = 2094
	OpReadyAnim           Opcode = 2095
	OpRunAnim             Opcode = 2096
	OpRunEnergy           Opcode = 2097
	OpSay                 Opcode = 2098
	OpSessionLog          Opcode = 2099
	OpSetPlayerOp         Opcode = 2100
	OpSetGender           Opcode = 2101
	OpSetIdKit            Opcode = 2102
	OpSetSkinColour       Opcode = 2103
	OpSetTimer            Opcode = 2104
	OpSoftTimer           Opcode = 2105
	OpSoundSynth          Opcode = 2106
	OpSpotAnimPl          Opcode = 2107
	OpStaffModLevel       Opcode = 2108
	OpStatAdd             Opcode = 2109
	OpStatAdvance         Opcode = 2110
	OpStatBase            Opcode = 2111
	OpStatBoost           Opcode = 2112
	OpStatDrain           Opcode = 2113
	OpStatHeal            Opcode = 2114
	OpStatRandom          Opcode = 2115
	OpStatSub             Opcode = 2116
	OpStatTotal           Opcode = 2117
	OpStat                Opcode = 2118
	OpStrongQueue         Opcode = 2119
	OpStrongQueueVarArg   Opcode = 2120
	OpTurnAnim            Opcode = 2121
	OpTutClose            Opcode = 2122
	OpTutFlash            Opcode = 2123
	OpTutOpen             Opcode = 2124
	OpUID                 Opcode = 2125
	OpWalkAnimB           Opcode = 2126
	OpWalkAnimL           Opcode = 2127
	OpWalkAnimR           Opcode = 2128
	OpWalkAnim            Opcode = 2129
	OpWalkTrigger         Opcode = 2130
	OpWeakQueue           Opcode = 2131
	OpWeakQueueVarArg     Opcode = 2132
	OpWealthEvent         Opcode = 2133
	OpWeight              Opcode = 2134
)

// NPC ops (2500–2999)
// 2e3bcf43 (254 pin-advance): NPC_HUNTNEXT deleted (NPC_HUNTALL now feeds
// the shared npcIterator, consumed by NPC_FINDNEXT); PROJANIM_NPC/
// SPOTANIM_NPC live at the block tail (2546–2547).
const (
	OpNpcAdd               Opcode = 2500
	OpNpcAnim              Opcode = 2501
	OpNpcArriveDelay       Opcode = 2502
	OpNpcAttackRange       Opcode = 2503
	OpNpcBaseStat          Opcode = 2504
	OpNpcCategory          Opcode = 2505
	OpNpcChangeTypeKeepAll Opcode = 2506
	OpNpcChangeType        Opcode = 2507
	OpNpcCoord             Opcode = 2508
	OpNpcDamage            Opcode = 2509
	OpNpcDel               Opcode = 2510
	OpNpcDelay             Opcode = 2511
	OpNpcFaceSquare        Opcode = 2512
	OpNpcFind              Opcode = 2513
	OpNpcFindAll           Opcode = 2514
	OpNpcFindAllAny        Opcode = 2515
	OpNpcFindAllZone       Opcode = 2516
	OpNpcFindCat           Opcode = 2517
	OpNpcFindExact         Opcode = 2518
	OpNpcFindHero          Opcode = 2519
	OpNpcFindNext          Opcode = 2520
	OpNpcFindUID           Opcode = 2521
	OpNpcGetMode           Opcode = 2522
	OpNpcHasOp             Opcode = 2523
	OpNpcHeroPoints        Opcode = 2524
	OpNpcHunt              Opcode = 2525
	OpNpcHuntAll           Opcode = 2526
	OpNpcInRange           Opcode = 2527
	OpNpcName              Opcode = 2528
	OpNpcParam             Opcode = 2529
	OpNpcQueue             Opcode = 2530
	OpNpcRange             Opcode = 2531
	OpNpcSay               Opcode = 2532
	OpNpcSetHunt           Opcode = 2533
	OpNpcSetHuntMode       Opcode = 2534
	OpNpcSetMode           Opcode = 2535
	OpNpcSetTimer          Opcode = 2536
	OpNpcStat              Opcode = 2537
	OpNpcStatAdd           Opcode = 2538
	OpNpcStatHeal          Opcode = 2539
	OpNpcStatSub           Opcode = 2540
	OpNpcTele              Opcode = 2541
	OpNpcType              Opcode = 2542
	OpNpcUID               Opcode = 2543
	OpNpcWalk              Opcode = 2544
	OpNpcWalkTrigger       Opcode = 2545
	OpProjAnimNpc          Opcode = 2546
	OpSpotAnimNpc          Opcode = 2547
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
// OBJ_COUNT (ground-obj stack count) at 3503. The zone-wide OBJCOUNT was
// deleted upstream at 2e3bcf43 (was OpZoneObjCount=1033 at 43e02957).
const (
	OpObjAdd         Opcode = 3500
	OpObjAddAll      Opcode = 3501
	OpObjCoord       Opcode = 3502
	OpObjCount       Opcode = 3503
	OpObjDel         Opcode = 3504
	OpObjFind        Opcode = 3505
	OpObjFindAllZone Opcode = 3506
	OpObjFindNext    Opcode = 3507
	OpObjName        Opcode = 3508
	OpObjParam       Opcode = 3509
	OpObjTakeItem    Opcode = 3510
	OpObjType        Opcode = 3511
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
// LC_OP stub at 4104 (no TS handler entry).
const (
	OpLcCategory  Opcode = 4100
	OpLcDebugName Opcode = 4101
	OpLcDesc      Opcode = 4102
	OpLcLength    Opcode = 4103
	OpLcName      Opcode = 4104
	OpLcOp        Opcode = 4105
	OpLcParam     Opcode = 4106
	OpLcWidth     Opcode = 4107
)

// Obj config ops (4200–4299)
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
	OpOcWearPos   Opcode = 4213
	OpOcWearPos2  Opcode = 4214
	OpOcWearPos3  Opcode = 4215
	OpOcWeight    Opcode = 4216
)

// Inventory ops (4300–4399)
const (
	OpBothDropSlot       Opcode = 4300
	OpBothMoveInv        Opcode = 4301
	OpInvAdd             Opcode = 4302
	OpInvAllStock        Opcode = 4303
	OpInvChangeSlot      Opcode = 4304
	OpInvClear           Opcode = 4305
	OpInvDebugName       Opcode = 4306
	OpInvDel             Opcode = 4307
	OpInvDelSlot         Opcode = 4308
	OpInvDropAll         Opcode = 4309
	OpInvDropItemDelayed Opcode = 4310
	OpInvDropItem        Opcode = 4311
	OpInvDropSlot        Opcode = 4312
	OpInvFreeSpace       Opcode = 4313
	OpInvGetNum          Opcode = 4314
	OpInvGetObj          Opcode = 4315
	OpInvItemSpace       Opcode = 4316
	OpInvItemSpace2      Opcode = 4317
	OpInvMoveFromSlot    Opcode = 4318
	OpInvMoveItemCert    Opcode = 4319
	OpInvMoveItemUncert  Opcode = 4320
	OpInvMoveItem        Opcode = 4321
	OpInvMoveToSlot      Opcode = 4322
	OpInvSetSlot         Opcode = 4323
	OpInvSize            Opcode = 4324
	OpInvStockBase       Opcode = 4325
	OpInvStopTransmit    Opcode = 4326
	OpInvTotal           Opcode = 4327
	OpInvTotalCat        Opcode = 4328
	OpInvTotalParamStack Opcode = 4329
	OpInvTotalParam      Opcode = 4330
	OpInvTransmit        Opcode = 4331
	OpInvOtherTransmit   Opcode = 4332
)

// Enum ops (4400–4499)
const (
	OpEnum               Opcode = 4400
	OpEnumGetOutputCount Opcode = 4401
)

// String ops (4500–4599)
// 2e3bcf43 (254 pin-advance): SPLIT_* ops return from the server block
// (1022–1026 at 43e02957) to 4513–4517.
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
	OpSplitGet            Opcode = 4513
	OpSplitGetAnim        Opcode = 4514
	OpSplitInit           Opcode = 4515
	OpSplitLineCount      Opcode = 4516
	OpSplitPageCount      Opcode = 4517
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

// Struct ops (4700)
// 2e3bcf43 (254 pin-advance): STRUCT_PARAM returns from the server block
// (1028 at 43e02957) to 4700 (TS handlers/StructOps.ts).
const (
	OpStructParam Opcode = 4700
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

// Debug ops (10000–10003 at the 254 pin 2e3bcf43)
// 2e3bcf43 (254 pin-advance): MAP_PRODUCTION (10001) and the 12 MAP_LAST*
// stat probes (10002–10013) deleted upstream; ERROR/TIMESPENT/GETTIMESPENT/
// CONSOLE renumber to 10000–10003.
const (
	OpConsole      Opcode = 10000
	OpError        Opcode = 10001
	OpGetTimeSpent Opcode = 10002
	OpTimeSpent    Opcode = 10003
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
	case OpMapLive:
		return "MAP_LIVE"
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
	case OpMidiLength:
		return "MIDI_LENGTH"
	case OpMapMultiway:
		return "MAP_MULTIWAY"
	case OpAllowDesign:
		return "ALLOWDESIGN"
	case OpAnim:
		return "ANIM"
	case OpReadyAnim:
		return "READYANIM"
	case OpRunAnim:
		return "RUNANIM"
	case OpTurnAnim:
		return "TURNANIM"
	case OpWalkAnimB:
		return "WALKANIM_B"
	case OpWalkAnim:
		return "WALKANIM"
	case OpWalkAnimL:
		return "WALKANIM_L"
	case OpWalkAnimR:
		return "WALKANIM_R"
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
	case OpHintPl:
		return "HINT_PL"
	case OpHintStop:
		return "HINT_STOP"
	case OpIfClose:
		return "IF_CLOSE"
	case OpTutClose:
		return "TUT_CLOSE"
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
	case OpIfAddResumeButton:
		return "IF_ADDRESUMEBUTTON"
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
	case OpStatTotal:
		return "STAT_TOTAL"
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
	case OpAfkEvent:
		return "AFK_EVENT"
	case OpLowMem:
		return "LOWMEM"
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
	case OpSetPlayerOp:
		return "SET_PLAYER_OP"
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
