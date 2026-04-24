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

	// S5m: last-input queries.
	OpLastInt:        handleLastInt,
	OpLastItem:       handleLastItem,
	OpLastSlot:       handleLastSlot,
	OpLastUseItem:    handleLastUseItem,
	OpLastUseSlot:    handleLastUseSlot,
	OpLastTargetSlot: handleLastTargetSlot,

	// Camera control — minimal stub set for login-script compatibility.
	OpCamReset: handleCamReset,

	// Staff / moderator state.
	OpStaffModLevel: handleStaffModLevel,

	// Account identity — persistent uid from login RPC.
	OpUID: handleUID,

	// LOC lookup — stub (always "not found"). Real impl ships with S6.
	OpLocFind: handleLocFind,
	// LOC active-loc reads.
	OpLocOp: handleLocOp,

	// DB ops (7501-7510).
	OpDbFindNext:         handleDbFindNext,
	OpDbGetField:         handleDbGetField,
	OpDbGetFieldCount:    handleDbGetFieldCount,
	OpDbListAllWithCount: handleDbListAllWithCount,
	OpDbGetRowTable:      handleDbGetRowTable,
	OpDbFindByIndex:      handleDbFindByIndex,
	OpDbListAll:          handleDbListAll,

	// S5a: string ops.
	OpAppend:              handleAppend,
	OpAppendNum:           handleAppendNum,
	OpAppendChar:          handleAppendChar,
	OpAppendSignNum:       handleAppendSignNum,
	OpLowercase:           handleLowercase,
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
	OpCoord:      handleCoord,
	OpFaceSquare: handleFaceSquare,
	OpPTeleport:  handlePTeleport,
	OpPTeleJump:  handlePTeleJump,
	// Animation.
	OpAnim:       handleAnim,
	OpSpotAnimPl: handleSpotAnimPl,
	OpReadyAnim:  handleReadyAnim,
	OpTurnAnim:   handleTurnAnim,
	OpWalkAnim:   handleWalkAnim,
	OpWalkAnimB:  handleWalkAnimB,
	OpWalkAnimL:  handleWalkAnimL,
	OpWalkAnimR:  handleWalkAnimR,
	OpRunAnim:    handleRunAnim,
	// P_WALK stub — real impl needs pathfinder + waypoint integration.
	OpPWalk: handlePWalk,

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
	// NpcConfigOps (8).
	OpNcName:      handleNcName,
	OpNcParam:     handleNcParam,
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
	// S6u+S6y: listener registration (3).
	OpInvTransmit:      handleInvTransmit,
	OpInvStopTransmit:  handleInvStopTransmit,
	OpInvOtherTransmit: handleInvOtherTransmit,

	// S5f: interface / modal.
	// Modal management (5).
	OpIfClose:        handleIfClose,
	OpIfOpenMain:     handleIfOpenMain,
	OpIfOpenChat:     handleIfOpenChat,
	OpIfOpenSide:     handleIfOpenSide,
	OpIfOpenMainSide: handleIfOpenMainSide,
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

	// S5h: queue variants.
	OpWeakQueue:   handleWeakQueue,
	OpStrongQueue: handleStrongQueue,
	OpLongQueue:   handleLongQueue,

	// S5h: action-clear.
	OpPStopAction:         handlePStopAction,
	OpPClearPendingAction: handlePClearPendingAction,

	// S6l: APLOC approach-range opcode.
	OpPApRange: handlePApRange,

	// S6v: p_op* re-anchor ops.
	OpPOpLoc: handleP_OpLoc,
	OpPOpNpc: handleP_OpNpc,

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
	OpNpcChangeType:        handleNpcChangeType,
	OpNpcChangeTypeKeepAll: handleNpcChangeTypeKeepAll,
	OpNpcDamage:            handleNpcDamage,
	OpNpcDelay:             handleNpcDelay,
	OpNpcFaceSquare:        handleNpcFaceSquare,
	OpNpcQueue:             handleNpcQueue,
	OpNpcSetHunt:           handleNpcSetHunt,
	OpNpcSetHuntMode:       handleNpcSetHuntMode,
	OpNpcSetTimer:          handleNpcSetTimer,

	// S7a: player UID lookup.
	OpFindUID:  handleFindUID,
	OpPFindUID: handlePFindUID,

	// S7b: anim-protect flag.
	OpPAnimProtect: handlePAnimProtect,

	// S7c: BUILDAPPEARANCE dispatch.
	OpBuildAppearance: handleBuildAppearance,
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

// handlePDelay implements P_DELAY (opcode 2071): pop int n, delay the
// active player by n+1 ticks, and suspend execution. TS PlayerOps.ts
// sets state.delayedUntil = currentTick + 1 + n; we push the whole
// calculation into the ActivePlayer.SetDelayed implementation so
// pkg/script stays decoupled from the server's current-tick counter.
func handlePDelay(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_DELAY"); err != nil {
		return err
	}
	n := int(s.PopInt())
	s.Self.SetDelayed(n)
	s.Execution = Suspended
	return nil
}

// enqueueTyped is the shared body for QUEUE / WEAKQUEUE / STRONGQUEUE /
// LONGQUEUE. Pops (scriptID, delay, arg) and calls Self.EnqueueScriptTyped
// with the requested type.
//
// TS (engine/script/handlers/PlayerOps.ts:148):
//
//	const [scriptId, delay, arg] = state.popInts(3);
//
// popInts(n) fills ints[n-1] down to ints[0] via PopInt, so the stack
// top is `arg`, then `delay`, then `scriptId`. The VARARG variants are
// deferred.
func enqueueTyped(s *ScriptState, qtype PlayerQueueType, op string) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return fmt.Errorf("%s: no active player", op)
	}
	arg := int(s.PopInt())
	delay := int(s.PopInt())
	scriptID := uint32(s.PopInt())
	s.Self.EnqueueScriptTyped(scriptID, delay, arg, qtype)
	return nil
}

// handleQueue implements QUEUE (opcode 2092): enqueue a fresh-run
// script request on the active player with QueueNormal type.
func handleQueue(s *ScriptState) error { return enqueueTyped(s, QueueNormal, "QUEUE") }

// handleWeakQueue implements WEAKQUEUE.
func handleWeakQueue(s *ScriptState) error { return enqueueTyped(s, QueueWeak, "WEAKQUEUE") }

// handleStrongQueue implements STRONGQUEUE.
func handleStrongQueue(s *ScriptState) error {
	return enqueueTyped(s, QueueStrong, "STRONGQUEUE")
}

// handleLongQueue implements LONGQUEUE.
func handleLongQueue(s *ScriptState) error { return enqueueTyped(s, QueueLong, "LONGQUEUE") }
