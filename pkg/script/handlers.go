package script

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// handlers maps each Opcode to its implementation function.
// Only the 19 S1 MVP opcodes are registered; all others abort with an
// informative error via the Execute dispatch loop.
var handlers = map[Opcode]func(*ScriptState) error{
	OpPushConstantInt:    handlePushConstantInt,
	OpPushConstantString: handlePushConstantString,
	OpReturn:             handleReturn,
	OpPushIntLocal:       handlePushIntLocal,
	OpPopIntLocal:        handlePopIntLocal,
	OpPushStringLocal:    handlePushStringLocal,
	OpPopStringLocal:     handlePopStringLocal,
	OpBranch:             handleBranch,
	OpBranchEquals:       handleBranchEquals,
	OpBranchNot:          handleBranchNot,
	OpPopIntDiscard:      handlePopIntDiscard,
	OpPopStringDiscard:   handlePopStringDiscard,
	OpJoinString:         handleJoinString,
	OpAdd:                handleAdd,
	OpSub:                handleSub,
	OpToString:           handleToString,
	OpGosubWithParams:    handleGosubWithParams,
	OpMes:                handleMes,
	OpName:               handleName,
	OpConsole:            handleConsole,
	OpPDelay:             handlePDelay,
	OpQueue:              handleQueue,

	// S5a: comparison branches.
	OpBranchLessThan:            handleBranchLessThan,
	OpBranchGreaterThan:         handleBranchGreaterThan,
	OpBranchLessThanOrEquals:    handleBranchLessThanOrEquals,
	OpBranchGreaterThanOrEquals: handleBranchGreaterThanOrEquals,

	// S5a: arithmetic.
	OpMultiply:   handleMultiply,
	OpDivide:     handleDivide,
	OpModulo:     handleModulo,
	OpAbs:        handleAbs,
	OpAddPercent: handleAddPercent,
	OpScale:      handleScale,
	OpMin:        handleMin,
	OpMax:        handleMax,
	OpPow:        handlePow,
	OpInvPow:     handleInvPow,

	// S5a: bitwise.
	OpAnd:              handleAnd,
	OpOr:               handleOr,
	OpBitCount:         handleBitCount,
	OpTestBit:          handleTestBit,
	OpSetBit:           handleSetBit,
	OpClearBit:         handleClearBit,
	OpToggleBit:        handleToggleBit,
	OpGetBitRange:      handleGetBitRange,
	OpSetBitRange:      handleSetBitRange,
	OpClearBitRange:    handleClearBitRange,
	OpSetBitRangeToInt: handleSetBitRangeToInt,

	// S5a: random.
	OpRandom:    handleRandom,
	OpRandomInc: handleRandomInc,

	// S5j: math completion (trig + interpolate).
	OpSinDeg:      handleSinDeg,
	OpCosDeg:      handleCosDeg,
	OpAtan2Deg:    handleAtan2Deg,
	OpInterpolate: handleInterpolate,

	// S5k: coord unpack + distance + GOSUB-no-params.
	OpCoordX:   handleCoordX,
	OpCoordY:   handleCoordY,
	OpCoordZ:   handleCoordZ,
	OpDistance: handleDistance,
	OpGosub:    handleGosub,

	// S5l: server/world ops.
	OpMapClock:    handleMapClock,
	OpPlayerCount: handlePlayerCount,
	OpMoveCoord:   handleMoveCoord,
	OpMapMembers:  handleMapMembers,
	OpMapLive:     handleMapLive,
	OpInZone:      handleInZone,

	// NAI-35-T2: rect-bounded player-count enumeration.
	OpMapPlayerCount: handleMapPlayerCount,

	// NAI-35-T6: free-square finder (imps + general placement).
	OpMapFindSquare: handleMapFindSquare,

	// NAI-36-T4: per-tile blocked-state query with F2P short-circuit.
	OpMapBlocked: handleMapBlocked,

	// NAI-120 Bundle 2A: multi-combat zone query.
	OpMapMultiway: handleMapMultiway,

	// NAI-36-T5: tile-anchored spotanim broadcast.
	OpSpotAnimMap: handleSpotAnimMap,

	// NAI-114 Stage 2: zone-wide active-loc occupancy probe for the
	// firemaking-chain area-allow check.
	OpMapLocAddUnsafe: handleMapLocAddUnsafe,

	// NAI-115 Bundle 1+2: firemaking-cascade Obj/Inv/Server/Player ports.
	OpObjCoord:    handleObjCoord,
	OpObjDel:      handleObjDel,
	OpObjAdd:      handleObjAdd,
	OpObjAddAll:   handleObjAddAll,
	OpLineOfWalk:  handleLineOfWalk,
	OpInvDropSlot: handleInvDropSlot,
	// NAI-115 stretch: LOWMEM surfaced by Tutorial Island smoke.
	OpLowMem: handleLowMem,
	// NAI-149 T2: PLAYERMEMBER.
	OpPlayerMember: handlePlayerMember,
	// NAI-149 T3: AFK_EVENT.
	OpAfkEvent: handleAfkEvent,
	// NAI-149 T4: WEIGHT.
	OpWeight: handleWeight,

	// S5m: last-input queries.
	OpLastInt:        handleLastInt,
	OpLastItem:       handleLastItem,
	OpLastSlot:       handleLastSlot,
	OpLastUseItem:    handleLastUseItem,
	OpLastUseSlot:    handleLastUseSlot,
	OpLastTargetSlot: handleLastTargetSlot,

	// Camera control — minimal stub set for login-script compatibility.
	OpCamReset:   handleCamReset,
	OpCamShake:   handleCamShake,
	OpCamMoveTo:  handleCamMoveTo,
	OpCamLookAt:  handleCamLookAt,

	// Staff / moderator state.
	OpStaffModLevel: handleStaffModLevel,

	// Account identity — persistent uid from login RPC.
	OpUID: handleUID,

	// LOC lookup + FIND iterator family. LOC_FIND (3007) remains a stub
	// (always "not found") pending a content consumer; LOC_FINDALLZONE
	// (3008) and LOC_FINDNEXT (3009) are wired by NAI-119.
	OpLocCoord:       handleLocCoord,
	OpLocFind:        handleLocFind,
	OpLocFindAllZone: handleLocFindAllZone,
	OpLocFindNext:    handleLocFindNext,
	// LOC active-loc reads + mutations.
	OpLocAdd:      handleLocAdd,
	OpLocAngle:    handleLocAngle,
	OpLocAnim:     handleLocAnim,
	OpLocCategory: handleLocCategory,
	OpLocChange:   handleLocChange,
	OpLocDel:      handleLocDel,
	OpLocName:     handleLocName,
	OpLocOp:       handleLocOp,
	OpLocParam:    handleLocParam,
	OpLocShape:    handleLocShape,
	OpLocType:     handleLocType,

	// DB ops (7500-7510).
	// Pointer-gate asymmetry across this family — see preamble comment on handlers_db.go.
	OpDbFindWithCount:       handleDbFindWithCount,       // 7500
	OpDbFindNext:            handleDbFindNext,            // 7501
	OpDbGetField:            handleDbGetField,            // 7502
	OpDbGetFieldCount:       handleDbGetFieldCount,       // 7503
	OpDbListAllWithCount:    handleDbListAllWithCount,    // 7504
	OpDbGetRowTable:         handleDbGetRowTable,         // 7505
	OpDbFindByIndex:         handleDbFindByIndex,         // 7506
	OpDbFindRefineWithCount: handleDbFindRefineWithCount, // 7507
	OpDbFind:                handleDbFind,                // 7508
	OpDbFindRefine:          handleDbFindRefine,          // 7509
	OpDbListAll:             handleDbListAll,             // 7510

	// S5a: string ops.
	OpAppend:              handleAppend,
	OpAppendNum:           handleAppendNum,
	OpAppendChar:          handleAppendChar,
	OpAppendSignNum:       handleAppendSignNum,
	OpLowercase:           handleLowercase,
	OpTextGender:          handleTextGender,
	OpCompare:             handleCompare,
	OpStringLength:        handleStringLength,
	OpSubstring:           handleSubstring,
	OpStringIndexOfChar:   handleStringIndexOfChar,
	OpStringIndexOfString: handleStringIndexOfString,
	OpTextSwitch:          handleTextSwitch,

	// S5a: SPLIT_* stubs (dialog pagination deferred).
	OpSplitInit:      handleSplitInit,
	OpSplitGet:       handleSplitGet,
	OpSplitGetAnim:   handleSplitGetAnim,
	OpSplitLineCount: handleSplitLineCount,
	OpSplitPageCount: handleSplitPageCount,

	// S5a: debug ops.
	OpError:        handleError,
	OpGetTimeSpent: handleGetTimeSpent,
	OpTimeSpent:    handleTimeSpent,

	// S5a: array ops + SWITCH.
	OpDefineArray:  handleDefineArray,
	OpPushArrayInt: handlePushArrayInt,
	OpPopArrayInt:  handlePopArrayInt,
	OpSwitch:       handleSwitch,

	// S5b: VAR ops.
	OpPushVarp: handlePushVarp,
	OpPopVarp:  handlePopVarp,
	OpPushVars: handlePushVars,
	OpPopVars:  handlePopVars,
	OpPushVarn: handlePushVarn, // stub until S6
	OpPopVarn:  handlePopVarn,  // stub until S6

	// S5c: player stat/coord/facing/anim.
	// Stat read + mutation ops.
	OpStat:        handleStat,
	OpStatBase:    handleStatBase,
	OpStatTotal:   handleStatTotal,
	OpStatAdd:     handleStatAdd,
	OpStatSub:     handleStatSub,
	OpStatBoost:   handleStatBoost,
	OpStatDrain:   handleStatDrain,
	OpStatHeal:    handleStatHeal,
	OpStatAdvance: handleStatAdvance,
	OpStatRandom:  handleStatRandom,
	// Coord / facing / teleport.
	OpCoord:       handleCoord,
	OpDamage:      handleDamage,
	OpDisplayName: handleDisplayName,
	OpGender:      handleGender,
	OpFaceSquare:  handleFaceSquare,
	OpPTeleport:   handlePTeleport,
	OpPTeleJump:   handlePTeleJump,
	// Animation.
	OpAnim:            handleAnim,
	OpBothHeroPoints:  handleBothHeroPoints,
	OpSpotAnimPl:      handleSpotAnimPl,
	OpReadyAnim:  handleReadyAnim,
	OpTurnAnim:   handleTurnAnim,
	OpWalkAnim:   handleWalkAnim,
	OpWalkAnimB:  handleWalkAnimB,
	OpWalkAnimL:  handleWalkAnimL,
	OpWalkAnimR:  handleWalkAnimR,
	OpRunAnim:    handleRunAnim,
	// NAI-51: walktrigger consumer ops (Player side).
	OpWalkTrigger:    handleWalkTrigger,
	OpGetWalkTrigger: handleGetWalkTrigger,
	// P_WALK stub — real impl needs pathfinder + waypoint integration.
	OpPWalk: handlePWalk,
	// NAI-117 T1: run-mode toggle (gated by ProtectedActivePlayer).
	OpPRun: handlePRun,
	// NAI-117 T2: run-energy reader (gated by ActivePlayer).
	OpRunEnergy: handleRunEnergy,

	// S5d: config-read ops (enum/struct/loc/npc/obj).
	// EnumOps (2).
	OpEnum:               handleEnum,
	OpEnumGetOutputCount: handleEnumGetOutputCount,
	// StructOps (1).
	OpStructParam: handleStructParam,
	// LocConfigOps (7).
	OpLcName:      handleLcName,
	OpLcParam:     handleLcParam,
	OpLcCategory:  handleLcCategory,
	OpLcDesc:      handleLcDesc,
	OpLcDebugName: handleLcDebugName,
	OpLcWidth:     handleLcWidth,
	OpLcLength:    handleLcLength,
	// NpcConfigOps (9).
	OpNcName:      handleNcName,
	OpNcParam:     handleNcParam,
	OpNpcParam:    handleNpcParam,
	OpNcCategory:  handleNcCategory,
	OpNcDesc:      handleNcDesc,
	OpNcDebugName: handleNcDebugName,
	OpNcOp:        handleNcOp,
	OpNcSize:      handleNcSize,
	OpNcVisLevel:  handleNcVisLevel,
	// ObjConfigOps (15).
	OpOcName:      handleOcName,
	OpOcParam:     handleOcParam,
	OpOcCategory:  handleOcCategory,
	OpOcDesc:      handleOcDesc,
	OpOcMembers:   handleOcMembers,
	OpOcWeight:    handleOcWeight,
	OpOcWearPos:   handleOcWearPos,
	OpOcWearPos2:  handleOcWearPos2,
	OpOcWearPos3:  handleOcWearPos3,
	OpOcCost:      handleOcCost,
	OpOcTradeable: handleOcTradeable,
	OpOcDebugName: handleOcDebugName,
	OpOcCert:      handleOcCert,
	OpOcUncert:    handleOcUncert,
	OpOcStackable: handleOcStackable,

	// S5e: inventory.
	// Both-player mutations (1).
	OpBothMoveInv: handleBothMoveInv,
	// Reads (9).
	OpInvTotal:      handleInvTotal,
	OpInvGetObj:     handleInvGetObj,
	OpInvGetNum:     handleInvGetNum,
	OpInvSize:       handleInvSize,
	OpInvFreeSpace:  handleInvFreeSpace,
	OpInvItemSpace:  handleInvItemSpace,
	OpInvItemSpace2: handleInvItemSpace2,
	OpInvTotalParam: handleInvTotalParam,
	OpInvTotalCat:   handleInvTotalCat,
	// Mutations (8).
	OpInvAdd:          handleInvAdd,
	OpInvDel:          handleInvDel,
	OpInvDelSlot:      handleInvDelSlot,
	OpInvSetSlot:      handleInvSetSlot,
	OpInvClear:        handleInvClear,
	OpInvMoveItem:     handleInvMoveItem,
	OpInvMoveFromSlot: handleInvMoveFromSlot,
	OpInvMoveToSlot:   handleInvMoveToSlot,
	OpInvChangeSlot:     handleInvChangeSlot,
	OpInvMoveItemCert:   handleInvMoveItemCert,
	OpInvMoveItemUncert: handleInvMoveItemUncert,
	OpInvDropItem:       handleInvDropItem,
	OpInvDropItemDelayed: handleInvDropItemDelayed,
	// S6u+S6y: listener registration (3).
	OpInvTransmit:      handleInvTransmit,
	OpInvStopTransmit:  handleInvStopTransmit,
	OpInvOtherTransmit: handleInvOtherTransmit,

	// S5f: interface / modal.
	// Modal management (8).
	OpIfClose:        handleIfClose,
	OpIfOpenMain:     handleIfOpenMain,
	OpIfOpenChat:     handleIfOpenChat,
	OpIfOpenSide:     handleIfOpenSide,
	OpIfOpenMainSide: handleIfOpenMainSide,
	OpTutOpen:        handleTutOpen,
	OpTutClose:       handleTutClose,
	OpTutFlash:       handleTutFlash,
	// Per-component setters (12).
	OpIfSetText:       handleIfSetText,
	OpIfSetModel:      handleIfSetModel,
	OpIfSetNpcHead:    handleIfSetNpcHead,
	OpIfSetPlayerHead: handleIfSetPlayerHead,
	OpIfSetAnim:       handleIfSetAnim,
	OpIfSetHide:       handleIfSetHide,
	OpIfSetTab:        handleIfSetTab,
	OpIfSetObject:     handleIfSetObject,
	OpIfSetColour:     handleIfSetColour,
	OpIfSetPosition:   handleIfSetPosition,
	OpIfSetRecol:      handleIfSetRecol,
	// Misc (2).
	OpIfSetTabActive:     handleIfSetTabActive,
	OpIfSetResumeButtons: handleIfSetResumeButtons,

	// S5g: dialog suspension.
	OpPPauseButton: handlePPauseButton,
	OpPCountDialog: handlePCountDialog,
	OpLastCom:      handleLastCom,

	// S5h: tail-call.
	OpJump:           handleJump,
	OpJumpWithParams: handleJumpWithParams,

	// S5h: queue variants. (NAI-26 added STRONG/WEAK/LONG; NAI-27 added
	// the four VARARG siblings; OpQueue itself remains in the MVP block
	// above as the original S1 opcode.)
	OpQueueVarArg:       handleQueueVarArg,
	OpWeakQueue:         handleWeakQueue,
	OpWeakQueueVarArg:   handleWeakQueueVarArg,
	OpStrongQueue:       handleStrongQueue,
	OpStrongQueueVarArg: handleStrongQueueVarArg,
	OpLongQueue:         handleLongQueue,
	OpLongQueueVarArg:   handleLongQueueVarArg,

	// S5h: action-clear.
	OpPStopAction:         handlePStopAction,
	OpPClearPendingAction: handlePClearPendingAction,
	OpPLogout:             handlePLogout,
	OpPPreventLogout:      handlePPreventLogout,

	// NAI-82: arrive-delay opcode.
	OpPArriveDelay: handlePArriveDelay,

	// S6l: APLOC approach-range opcode.
	OpPApRange: handlePApRange,

	// S6v: p_op* re-anchor ops.
	OpPOpLoc:    handleP_OpLoc,
	OpPOpNpc:    handleP_OpNpc,
	OpPOpObj:    handleP_OpObj,
	OpBusy2:     handleBusy2,
	OpPOpNpcT:   handlePOpNpcT,
	OpPOpPlayer: handlePOpPlayer,

	// S5i: timer ops.
	OpSetTimer:       handleSetTimer,
	OpSoftTimer:      handleSoftTimer,
	OpClearTimer:     handleClearTimer,
	OpClearSoftTimer: handleClearSoftTimer,
	OpGetTimer:       handleGetTimer,

	// S6a: NPC reads.
	OpNpcType:     handleNpcType,
	OpNpcCoord:    handleNpcCoord,
	OpNpcStat:     handleNpcStat,
	OpNpcBaseStat: handleNpcBaseStat,
	OpNpcName:     handleNpcName,
	OpNpcHasOp:    handleNpcHasOp,
	OpNpcUID:      handleNpcUID,
	OpNpcCategory: handleNpcCategory,

	// S6b: NPC mutating ops.
	OpNpcSay: handleNpcSay,

	// S6c: NPC mutating ops batch.
	OpNpcAnim:              handleNpcAnim,
	OpNpcArriveDelay:       handleNpcArriveDelay,
	OpNpcChangeType:        handleNpcChangeType,
	OpNpcChangeTypeKeepAll: handleNpcChangeTypeKeepAll,
	OpNpcDamage:            handleNpcDamage,
	OpNpcDel:               handleNpcDel,
	OpNpcDelay:             handleNpcDelay,
	OpNpcFaceSquare:        handleNpcFaceSquare,
	OpNpcQueue:             handleNpcQueue,
	OpNpcSetHunt:           handleNpcSetHunt,
	OpNpcSetHuntMode:       handleNpcSetHuntMode,
	OpNpcSetTimer:          handleNpcSetTimer,
	OpNpcGetMode:           handleNpcGetMode,
	OpNpcSetMode:           handleNpcSetMode,
	OpNpcTele:              handleNpcTele,
	OpNpcWalk:              handleNpcWalk,
	OpNpcWalkTrigger:       handleNpcWalkTrigger,

	// NAI-120 Bundle 2A: UID-keyed NPC lookup + range query.
	OpNpcFindUID: handleNpcFindUID,
	OpNpcRange:   handleNpcRange,

	// NAI-120 Bundle 2C: NPC stat write ops + NPC spotanim.
	OpNpcStatAdd:   handleNpcStatAdd,
	OpNpcStatSub:   handleNpcStatSub,
	OpSpotAnimNpc:  handleSpotAnimNpc,

	// NAI-120 Bundle 2D: NPC hero-point ledger write.
	OpNpcHeroPoints: handleNpcHeroPoints,

	// NPC find (S7f) — closest-single cluster.
	OpNpcFind:      handleNpcFind,
	OpNpcFindCat:   handleNpcFindCat,
	OpNpcFindExact: handleNpcFindExact,

	// NPC find (NAI-33) — iterator family (DISTANCE + ZONE).
	OpNpcFindAll:     handleNpcFindAll,
	OpNpcFindAllAny:  handleNpcFindAllAny,
	OpNpcFindAllZone: handleNpcFindAllZone,
	OpNpcFindHero:    handleNpcFindHero,
	OpNpcFindNext:    handleNpcFindNext,

	// NPC hunt (NAI-35-T3) — HuntAll iterator (distance + active huntvis).
	OpNpcHuntAll: handleNpcHuntAll,

	// Player hunt (NAI-35-T4/T5) — HuntAll iterator over players +
	// HUNTNEXT consumer.
	OpHuntAll:  handleHuntAll,
	OpHuntNext: handleHuntNext,

	// S7a: player UID lookup.
	OpFindUID:  handleFindUID,
	OpFindHero: handleFindHero,
	OpPFindUID: handlePFindUID,

	// S7b: anim-protect flag.
	OpPAnimProtect: handlePAnimProtect,

	// S7c: BUILDAPPEARANCE dispatch.
	OpBuildAppearance: handleBuildAppearance,

	// NAI-47: identity-kit body-part setter.
	OpSetIdKit: handleSetIdKit,

	// S7e: character-design flag setter.
	OpAllowDesign: handleAllowDesign,

	// S7h + NAI-87: audio — MIDI_SONG + MIDI_JINGLE + SOUND_SYNTH.
	OpMidiJingle: handleMidiJingle,
	OpMidiSong:   handleMidiSong,
	OpSoundSynth: handleSoundSynth,

	// NAI-37 T6 + NAI-39: hint-arrow — full HintArrowEncoder coverage.
	//   - HINT_NPC   (type=1)     — NAI-37
	//   - HINT_COORD (type=2..6)  — NAI-39
	//   - HINT_PL    (type=10)    — NAI-39
	//   - HINT_STOP  (type=-1)    — NAI-39
	OpHintNpc:   handleHintNpc,
	OpHintCoord: handleHintCoord,
	OpHintPl:    handleHintPl,
	OpHintStop:  handleHintStop,

	// NAI-37 T7: world-script delay — WORLD_DELAY (handler-only; consumer wiring T8-T12).
	OpWorldDelay: handleWorldDelay,

	// NAI-74: SESSION_LOG opcode → ActivePlayer.AddSessionLog dispatch.
	OpSessionLog: handleSessionLog,
}

// handlePushConstantInt pushes the instruction's int operand onto the int stack.
func handlePushConstantInt(s *ScriptState) error {
	s.PushInt(int(s.Script.IntOperands[s.PC]))
	return nil
}

// handlePushConstantString pushes the instruction's string operand onto the string stack.
func handlePushConstantString(s *ScriptState) error {
	s.PushString(s.Script.StringOperands[s.PC])
	return nil
}

// handleReturn pops a call frame. When the frame stack is empty,
// sets Execution = Finished (clean script exit).
func handleReturn(s *ScriptState) error {
	return s.Return()
}

// handlePushIntLocal reads local variable slot [operand] and pushes it.
func handlePushIntLocal(s *ScriptState) error {
	idx := int(s.Script.IntOperands[s.PC])
	if idx < len(s.IntLocals) {
		s.PushInt(s.IntLocals[idx])
	} else {
		s.PushInt(0)
	}
	return nil
}

// handlePopIntLocal pops the top of the int stack into local variable slot [operand].
func handlePopIntLocal(s *ScriptState) error {
	idx := int(s.Script.IntOperands[s.PC])
	v := s.PopInt()
	if idx >= len(s.IntLocals) {
		// Grow locals if needed.
		grown := make([]int, idx+1)
		copy(grown, s.IntLocals)
		s.IntLocals = grown
	}
	s.IntLocals[idx] = v
	return nil
}

// handlePushStringLocal reads string local variable slot [operand] and pushes it.
func handlePushStringLocal(s *ScriptState) error {
	idx := int(s.Script.IntOperands[s.PC])
	if idx < len(s.StringLocals) {
		s.PushString(s.StringLocals[idx])
	} else {
		s.PushString("")
	}
	return nil
}

// handlePopStringLocal pops the top of the string stack into string local slot [operand].
func handlePopStringLocal(s *ScriptState) error {
	idx := int(s.Script.IntOperands[s.PC])
	v := s.PopString()
	if idx >= len(s.StringLocals) {
		grown := make([]string, idx+1)
		copy(grown, s.StringLocals)
		s.StringLocals = grown
	}
	s.StringLocals[idx] = v
	return nil
}

// handleBranch performs an unconditional relative branch.
//
// Branch convention: the operand is added directly to s.PC. The runner's
// post-handler s.PC++ means the effective target is (currentPC + operand + 1).
// This matches the TS pattern where the branch handler adds to pc and the loop's
// ++pc advances one further.
func handleBranch(s *ScriptState) error {
	s.PC += int(s.Script.IntOperands[s.PC])
	return nil
}

// handleBranchEquals pops b then a; if a == b, applies the branch offset.
func handleBranchEquals(s *ScriptState) error {
	b := s.PopInt()
	a := s.PopInt()
	if a == b {
		s.PC += int(s.Script.IntOperands[s.PC])
	}
	return nil
}

// handleBranchNot pops b then a; if a != b, applies the branch offset.
func handleBranchNot(s *ScriptState) error {
	b := s.PopInt()
	a := s.PopInt()
	if a != b {
		s.PC += int(s.Script.IntOperands[s.PC])
	}
	return nil
}

// handlePopIntDiscard pops and discards the top int.
func handlePopIntDiscard(s *ScriptState) error {
	s.PopInt()
	return nil
}

// handlePopStringDiscard pops and discards the top string.
func handlePopStringDiscard(s *ScriptState) error {
	s.PopString()
	return nil
}

// handleJoinString pops N strings and concatenates them in push order.
// The operand is the count N. The top of stack is the last pushed string.
func handleJoinString(s *ScriptState) error {
	n := int(s.Script.IntOperands[s.PC])
	if n <= 0 {
		s.PushString("")
		return nil
	}
	parts := make([]string, n)
	for i := n - 1; i >= 0; i-- {
		parts[i] = s.PopString()
	}
	s.PushString(strings.Join(parts, ""))
	return nil
}

// handleAdd pops b then a, pushes a + b.
func handleAdd(s *ScriptState) error {
	b := s.PopInt()
	a := s.PopInt()
	s.PushInt(a + b)
	return nil
}

// handleSub pops b then a, pushes a - b.
func handleSub(s *ScriptState) error {
	b := s.PopInt()
	a := s.PopInt()
	s.PushInt(a - b)
	return nil
}

// handleToString pops an int and pushes its decimal string representation.
func handleToString(s *ScriptState) error {
	v := s.PopInt()
	s.PushString(strconv.Itoa(v))
	return nil
}

// popArgsForTarget pops int + string args in reverse order based on
// target.IntArgCount / StringArgCount. Returns (intArgs, stringArgs)
// ordered so index 0 is the first declared arg, ready to pass to
// GosubCall / JumpCall. The last-pushed argument is popped first;
// args are placed back-to-front so their declaration order is
// preserved.
func popArgsForTarget(s *ScriptState, target *ScriptFile) (intArgs []int, stringArgs []string) {
	intArgs = make([]int, target.IntArgCount)
	for i := int(target.IntArgCount) - 1; i >= 0; i-- {
		intArgs[i] = s.PopInt()
	}
	stringArgs = make([]string, target.StringArgCount)
	for i := int(target.StringArgCount) - 1; i >= 0; i-- {
		stringArgs[i] = s.PopString()
	}
	return intArgs, stringArgs
}

// handleGosubWithParams calls a sub-script identified by its LookupKey (the operand).
//
// The int and string arguments are popped from the stacks in reverse order before
// the call (last arg pushed = first popped), then re-ordered so that args[0] is
// the first argument — matching TS setupNewScript's reversed-pop pattern.
//
// GosubCall sets PC = -1 so the runner's post-handler PC++ lands at 0, executing
// the first instruction of the callee on the next loop iteration.
func handleGosubWithParams(s *ScriptState) error {
	if s.Provider == nil {
		return errors.New("GOSUB_WITH_PARAMS: Provider not set on ScriptState")
	}
	targetID := uint32(s.Script.IntOperands[s.PC])
	target := s.Provider.GetByID(targetID)
	if target == nil {
		return fmt.Errorf("GOSUB_WITH_PARAMS: no script with id %d", targetID)
	}

	intArgs, stringArgs := popArgsForTarget(s, target)

	s.GosubCall(target, intArgs, stringArgs)
	return nil
}

// handleMes sends a pop'd string to the active player via MessageGame.
// Requires PtrActivePlayer to be set and Self to be non-nil.
func handleMes(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("MES: no active player")
	}
	s.Self.MessageGame(s.PopString())
	return nil
}

// handleName pushes the active player's username onto the string stack.
// Requires PtrActivePlayer to be set and Self to be non-nil.
func handleName(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("NAME: no active player")
	}
	s.PushString(s.Self.Username())
	return nil
}

// handleConsole pops and discards a debug string. In S1, console output is
// silently dropped; a logger will be wired in S2/S3.
func handleConsole(s *ScriptState) error {
	_ = s.PopString()
	return nil
}

// handlePDelay implements P_DELAY (opcode 2071): pop int n
// (NumberNotNull-checked), delay the active player by n+1 ticks, and
// suspend execution. TS PlayerOps.ts:375-379 sets
// state.delayedUntil = currentTick + 1 + check(state.popInt(),
// NumberNotNull); we push the +1 calculation into the
// ActivePlayer.SetDelayed implementation so pkg/script stays decoupled
// from the server's current-tick counter.
//
// NAI-26 Bundle 2: NumberNotNull wrap added to fix divergence κ — TS
// PlayerOps.ts:377 wraps the popped n with check(..., NumberNotNull).
func handlePDelay(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_DELAY"); err != nil {
		return err
	}
	n := s.PopInt()
	if err := checkNotNull(n, "P_DELAY"); err != nil {
		return err
	}
	s.Self.SetDelayed(n)
	s.Execution = Suspended
	return nil
}

// handlePArriveDelay implements P_ARRIVEDELAY (opcode 2068): if the
// active player has moved within the past 2 ticks, mark them delayed for
// 1 tick and suspend the script; otherwise no-op. TS PlayerOps.ts:357-366.
//
// The 2-tick window arises from the TS lastMovement contract (written to
// currentTick + 1 after a moving tick): the gate accepts moves from this
// tick (lastMovement = T+1) and last tick (lastMovement = T) but rejects
// moves from 2+ ticks ago (lastMovement = T-1; T-1 < T ⇒ return).
//
// Requires ProtectedActivePlayer pointer.
func handlePArriveDelay(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_ARRIVEDELAY"); err != nil {
		return err
	}
	if s.World == nil {
		return errors.New("P_ARRIVEDELAY: no world")
	}
	if s.Self.LastMovement() < s.World.CurrentTick() {
		return nil
	}
	s.Self.SetDelayed(0)
	s.Execution = Suspended
	return nil
}

// popScriptArgs pops a type-tags string from the stack, then pops typed
// args in reverse tag order (i = count-1 down to 0): tag char 's' pops
// a string into stringArgs; any other tag pops an int into intArgs.
// Mirrors TS PlayerOps.ts:1248-1263.
//
// TS returns []ScriptArgument (a single ordered slice with mixed types
// indexed by tag position). Goscape's parallel-slice convention encodes
// the same data with two slices: each tag's value lands in the slice
// for its type, in tag-position order. The caller does not need to
// reconstruct positional access — runScript consumes intArgs and
// stringArgs separately, and ScriptState.Init unpacks each into its
// own typed local-variable slot per IntArgCount / StringArgCount.
//
// Returns nil/nil for an empty type-tags string. The caller is
// responsible for ensuring the stack has the popped values in TS-faithful
// order (last tag's value on top of the typed block; tags string on the
// very top — popped first).
//
// Example: type-tags "isi" with stack values pushed in tag order
// [1, "two", 3] (rightmost = top of stack at popScriptArgs entry, just
// below the tags string) yields intArgs=[1, 3] and stringArgs=["two"].
// Each typed value lands at its tag-relative position in the type-specific
// output slice.
func popScriptArgs(s *ScriptState) (intArgs []int, stringArgs []string) {
	types := s.PopString()
	count := len(types)
	if count == 0 {
		return nil, nil
	}
	// Pre-pass: count int and string tags to size the slices.
	var intCount, stringCount int
	for _, t := range types {
		if t == 's' {
			stringCount++
		} else {
			intCount++
		}
	}
	if intCount > 0 {
		intArgs = make([]int, intCount)
	}
	if stringCount > 0 {
		stringArgs = make([]string, stringCount)
	}
	// Reverse-pop pass: TS iterates i = count-1 down to 0.
	intIdx := intCount - 1
	stringIdx := stringCount - 1
	for i := count - 1; i >= 0; i-- {
		if types[i] == 's' {
			stringArgs[stringIdx] = s.PopString()
			stringIdx--
		} else {
			intArgs[intIdx] = s.PopInt()
			intIdx--
		}
	}
	return intArgs, stringArgs
}

// handleQueue implements QUEUE (opcode 2092): pop scriptID, delay, arg
// (3 ints) and enqueue a NORMAL-typed queue request with [arg] as the
// args array. Mirrors TS PlayerOps.ts:148-157 line-by-line.
//
// NAI-26 Bundle 2: un-shared from the pre-NAI-26 enqueueTyped helper.
// The body here is mechanically equivalent to the old shared helper for
// QUEUE; un-sharing exists to enable per-handler script-missing error
// propagation (divergence ε — TS PlayerOps.ts:152-154) via the
// EnqueueScriptArgs return (Task 6 Step 1 activates the error).
func handleQueue(s *ScriptState) error {
	if err := requireActivePlayer(s, "QUEUE"); err != nil {
		return err
	}
	arg := s.PopInt()
	delay := s.PopInt()
	scriptID := uint32(s.PopInt())
	return s.Self.EnqueueScriptArgs(scriptID, delay, []int{arg}, nil, QueueNormal)
}

// handleWeakQueue implements WEAKQUEUE (opcode 2129): pop scriptID,
// delay, arg (3 ints) and enqueue a WEAK-typed queue request with [arg]
// as the args array. Mirrors TS PlayerOps.ts:123-132 line-by-line.
//
// NAI-26 Bundle 2: un-shared from the pre-NAI-26 enqueueTyped helper
// to enable per-handler script-missing error propagation
// (divergence δ — TS PlayerOps.ts:127-129) via the EnqueueScriptArgs
// return (Task 6 Step 1 activates the error).
func handleWeakQueue(s *ScriptState) error {
	if err := requireActivePlayer(s, "WEAKQUEUE"); err != nil {
		return err
	}
	arg := s.PopInt()
	delay := s.PopInt()
	scriptID := uint32(s.PopInt())
	return s.Self.EnqueueScriptArgs(scriptID, delay, []int{arg}, nil, QueueWeak)
}

// handleStrongQueue implements STRONGQUEUE (opcode 2117): pop variadic
// typed args via popScriptArgs (which itself first pops the type-tags
// string and then pops each typed value in tag-reverse order), then
// pop delay (NumberNotNull-checked), then pop scriptID, and enqueue a
// STRONG-typed queue request. Mirrors TS PlayerOps.ts:97-108
// line-by-line.
//
// NAI-26 Bundle 2: un-shared from the pre-NAI-26 enqueueTyped helper
// to fix divergences α (NumberNotNull on delay, missing) + β
// (popScriptArgs, missing — the helper popped only a single arg int,
// silently using the QUEUE shape for a variadic opcode).
func handleStrongQueue(s *ScriptState) error {
	if err := requireActivePlayer(s, "STRONGQUEUE"); err != nil {
		return err
	}
	intArgs, stringArgs := popScriptArgs(s)
	delay := s.PopInt()
	if err := checkNotNull(delay, "STRONGQUEUE"); err != nil {
		return err
	}
	scriptID := uint32(s.PopInt())
	return s.Self.EnqueueScriptArgs(scriptID, delay, intArgs, stringArgs, QueueStrong)
}

// handleLongQueue implements LONGQUEUE (opcode 2059): pop scriptID,
// delay, arg, logoutAction (4 ints) and enqueue a LONG-typed queue
// request with [logoutAction, arg] as the args array (logoutAction-
// first per TS PlayerOps.ts:179). Mirrors TS PlayerOps.ts:171-180
// line-by-line.
//
// NAI-26 Bundle 2: un-shared from the pre-NAI-26 enqueueTyped helper
// to fix divergences ζ (4-popInt missing — helper popped only 3) and
// η (2-element args array missing — helper passed [arg] not
// [logoutAction, arg]).
func handleLongQueue(s *ScriptState) error {
	if err := requireActivePlayer(s, "LONGQUEUE"); err != nil {
		return err
	}
	logoutAction := s.PopInt()
	arg := s.PopInt()
	delay := s.PopInt()
	scriptID := uint32(s.PopInt())
	return s.Self.EnqueueScriptArgs(scriptID, delay, []int{logoutAction, arg}, nil, QueueLong)
}
