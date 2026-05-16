// Package writer ports TS src/compiler/writer/ + src/runescript/ServerScriptOpcode.ts.
// It exposes the numeric ServerScriptOpcode IDs, the IdProvider interface
// (numeric mapping for symbols), and the OpcodeWriter dispatch interface that
// concrete writers (binary/text/etc.) implement.
//
// Mirrors TS BaseScriptWriter abstract class via a Go interface + free function
// (NAI-209-D-OPCODE-WRITER-INTERFACE).
package writer

// ServerScriptOpcode is one entry in the binary opcode table. ID is the numeric
// opcode written to the binary stream; LargeOperand selects between a 1-byte
// operand and a 4-byte operand encoding. Mirrors TS ServerScriptOpcode.ts.
type ServerScriptOpcode struct {
	ID           uint16
	LargeOperand bool
}

// Core language ops (0-99). IDs are verbatim from TS ServerScriptOpcode.ts L13-46.
var (
	OpPushConstantInt           = &ServerScriptOpcode{ID: 0, LargeOperand: true}
	OpPushVarp                  = &ServerScriptOpcode{ID: 1, LargeOperand: true}
	OpPopVarp                   = &ServerScriptOpcode{ID: 2, LargeOperand: true}
	OpPushConstantString        = &ServerScriptOpcode{ID: 3, LargeOperand: true}
	OpPushVarn                  = &ServerScriptOpcode{ID: 4, LargeOperand: true}
	OpPopVarn                   = &ServerScriptOpcode{ID: 5, LargeOperand: true}
	OpBranch                    = &ServerScriptOpcode{ID: 6, LargeOperand: true}
	OpBranchNot                 = &ServerScriptOpcode{ID: 7, LargeOperand: true}
	OpBranchEquals              = &ServerScriptOpcode{ID: 8, LargeOperand: true}
	OpBranchLessThan            = &ServerScriptOpcode{ID: 9, LargeOperand: true}
	OpBranchGreaterThan         = &ServerScriptOpcode{ID: 10, LargeOperand: true}
	OpPushVars                  = &ServerScriptOpcode{ID: 11, LargeOperand: true}
	OpPopVars                   = &ServerScriptOpcode{ID: 12, LargeOperand: true}
	OpReturn                    = &ServerScriptOpcode{ID: 21}
	OpGosub                     = &ServerScriptOpcode{ID: 22}
	OpJump                      = &ServerScriptOpcode{ID: 23}
	OpSwitch                    = &ServerScriptOpcode{ID: 24, LargeOperand: true}
	OpPushVarbit                = &ServerScriptOpcode{ID: 25, LargeOperand: true}
	OpPopVarbit                 = &ServerScriptOpcode{ID: 27, LargeOperand: true}
	OpBranchLessThanOrEquals    = &ServerScriptOpcode{ID: 31, LargeOperand: true}
	OpBranchGreaterThanOrEquals = &ServerScriptOpcode{ID: 32, LargeOperand: true}
	OpPushIntLocal              = &ServerScriptOpcode{ID: 33, LargeOperand: true}
	OpPopIntLocal               = &ServerScriptOpcode{ID: 34, LargeOperand: true}
	OpPushStringLocal           = &ServerScriptOpcode{ID: 35, LargeOperand: true}
	OpPopStringLocal            = &ServerScriptOpcode{ID: 36, LargeOperand: true}
	OpJoinString                = &ServerScriptOpcode{ID: 37, LargeOperand: true}
	OpPopIntDiscard             = &ServerScriptOpcode{ID: 38}
	OpPopStringDiscard          = &ServerScriptOpcode{ID: 39}
	OpGosubWithParams           = &ServerScriptOpcode{ID: 40, LargeOperand: true}
	OpJumpWithParams            = &ServerScriptOpcode{ID: 41, LargeOperand: true}
	OpDefineArray               = &ServerScriptOpcode{ID: 44, LargeOperand: true}
	OpPushArrayInt              = &ServerScriptOpcode{ID: 45, LargeOperand: true}
	OpPopArrayInt               = &ServerScriptOpcode{ID: 46, LargeOperand: true}
)

// Number ops (4600-4699). Verbatim from TS L48-54.
var (
	OpAdd      = &ServerScriptOpcode{ID: 4600}
	OpSub      = &ServerScriptOpcode{ID: 4601}
	OpMultiply = &ServerScriptOpcode{ID: 4602}
	OpDivide   = &ServerScriptOpcode{ID: 4603}
	OpModulo   = &ServerScriptOpcode{ID: 4611}
	OpAnd      = &ServerScriptOpcode{ID: 4614}
	OpOr       = &ServerScriptOpcode{ID: 4615}
)

// All enumerates every defined ServerScriptOpcode singleton in the same order
// as TS ServerScriptOpcode.ALL (L56-97). Stable iteration order; safe to
// range over.
var All = []*ServerScriptOpcode{
	OpPushConstantInt, OpPushVarp, OpPopVarp, OpPushConstantString,
	OpPushVarn, OpPopVarn, OpBranch, OpBranchNot, OpBranchEquals,
	OpBranchLessThan, OpBranchGreaterThan, OpPushVars, OpPopVars,
	OpReturn, OpGosub, OpJump, OpSwitch, OpPushVarbit, OpPopVarbit,
	OpBranchLessThanOrEquals, OpBranchGreaterThanOrEquals,
	OpPushIntLocal, OpPopIntLocal, OpPushStringLocal, OpPopStringLocal,
	OpJoinString, OpPopIntDiscard, OpPopStringDiscard,
	OpGosubWithParams, OpJumpWithParams, OpDefineArray,
	OpPushArrayInt, OpPopArrayInt,
	OpAdd, OpSub, OpMultiply, OpDivide, OpModulo, OpAnd, OpOr,
}
