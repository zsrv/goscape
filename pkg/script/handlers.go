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
	targetKey := uint32(s.Script.IntOperands[s.PC])
	target, ok := s.Provider.byKey[targetKey]
	if !ok {
		return fmt.Errorf("GOSUB_WITH_PARAMS: no script with lookup key %#x", targetKey)
	}

	intArgs := make([]int, target.IntArgCount)
	for i := int(target.IntArgCount) - 1; i >= 0; i-- {
		intArgs[i] = s.PopInt()
	}
	stringArgs := make([]string, target.StringArgCount)
	for i := int(target.StringArgCount) - 1; i >= 0; i-- {
		stringArgs[i] = s.PopString()
	}

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
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("P_DELAY: no active player")
	}
	n := int(s.PopInt())
	s.Self.SetDelayed(n)
	s.Execution = Suspended
	return nil
}

// handleQueue implements QUEUE (opcode 2092): enqueue a fresh-run
// script request on the active player.
//
// TS (engine/script/handlers/PlayerOps.ts:148):
//
//	const [scriptId, delay, arg] = state.popInts(3);
//
// popInts(n) fills ints[n-1] down to ints[0] via PopInt, so the stack
// top is `arg`, then `delay`, then `scriptId`. For S4 we support only
// the single-int-arg variant (QUEUEVARARG is deferred).
func handleQueue(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("QUEUE: no active player")
	}
	arg := int(s.PopInt())
	delay := int(s.PopInt())
	scriptID := uint32(s.PopInt())
	s.Self.EnqueueScript(scriptID, delay, arg)
	return nil
}
