// Package codegen lowers a type-checked ast.ScriptFile to RuneScript records
// containing abstract-Opcode instruction streams organised into labelled
// Blocks. Ports src/compiler/codegen/ (TS) — see NAI-207 spec.
//
// The abstract Opcode is distinct from the runtime numeric opcodes in
// pkg/script/opcode.go. NAI-208's writer pass maps abstract → numeric.
package codegen

// OperandKind classifies the dynamic type of Instruction.Operand. Consumers
// (writer pass, tests) switch on OperandKind. Per NAI-207-D-OPCODE-UNTYPED:
// goscape uses an untyped operand because Go generics don't compose with a
// []Instruction[any]-style heterogeneous slice.
type OperandKind int

const (
	OperandNone          OperandKind = iota
	OperandInt                       // int
	OperandString                    // string
	OperandLong                      // int64
	OperandLabel                     // *Label
	OperandLocalVar                  // *symbol.LocalVariableSymbol
	OperandBasicVar                  // *symbol.BasicSymbol
	OperandScriptSym                 // symbol.Symbol (concrete: *ServerScriptSymbol/*ClientScriptSymbol) — Gosub/Jump/Command
	OperandRuneScriptSym             // symbol.Symbol (concrete: any script symbol or BasicSymbol) — PushConstantSymbol
	OperandSwitchTable               // *SwitchTable
	OperandBaseVarType               // typ.BaseVarType (Discard operand)
)

// Opcode is the abstract typed-by-convention codegen opcode. Singletons
// below mirror TS Opcode.ts statics one-for-one. The Kind field is
// bookkeeping for writers/tests; the codegen author and reviewers verify
// the right concrete operand type at each emission site.
type Opcode struct {
	Name string
	Kind OperandKind
}

// Push constants
var (
	PushConstantInt    = Opcode{"PushConstantInt", OperandInt}
	PushConstantString = Opcode{"PushConstantString", OperandString}
	PushConstantLong   = Opcode{"PushConstantLong", OperandLong}
	PushConstantSymbol = Opcode{"PushConstantSymbol", OperandRuneScriptSym}
)

// Variables
var (
	PushLocalVar = Opcode{"PushLocalVar", OperandLocalVar}
	PopLocalVar  = Opcode{"PopLocalVar", OperandLocalVar}
	PushVar      = Opcode{"PushVar", OperandBasicVar}
	PushVar2     = Opcode{"PushVar2", OperandBasicVar}
	PopVar       = Opcode{"PopVar", OperandBasicVar}
	PopVar2      = Opcode{"PopVar2", OperandBasicVar}
	DefineArray  = Opcode{"DefineArray", OperandLocalVar}
)

// Control flow
var (
	Switch = Opcode{"Switch", OperandSwitchTable}
	Branch = Opcode{"Branch", OperandLabel}

	BranchNot                 = Opcode{"BranchNot", OperandLabel}
	BranchEquals              = Opcode{"BranchEquals", OperandLabel}
	BranchLessThan            = Opcode{"BranchLessThan", OperandLabel}
	BranchGreaterThan         = Opcode{"BranchGreaterThan", OperandLabel}
	BranchLessThanOrEquals    = Opcode{"BranchLessThanOrEquals", OperandLabel}
	BranchGreaterThanOrEquals = Opcode{"BranchGreaterThanOrEquals", OperandLabel}

	LongBranchNot                 = Opcode{"LongBranchNot", OperandLabel}
	LongBranchEquals              = Opcode{"LongBranchEquals", OperandLabel}
	LongBranchLessThan            = Opcode{"LongBranchLessThan", OperandLabel}
	LongBranchGreaterThan         = Opcode{"LongBranchGreaterThan", OperandLabel}
	LongBranchLessThanOrEquals    = Opcode{"LongBranchLessThanOrEquals", OperandLabel}
	LongBranchGreaterThanOrEquals = Opcode{"LongBranchGreaterThanOrEquals", OperandLabel}

	ObjBranchNot    = Opcode{"ObjBranchNot", OperandLabel}
	ObjBranchEquals = Opcode{"ObjBranchEquals", OperandLabel}
)

// String + discard
var (
	JoinString = Opcode{"JoinString", OperandInt}
	Discard    = Opcode{"Discard", OperandBaseVarType}
)

// Calls
var (
	Gosub   = Opcode{"Gosub", OperandScriptSym}
	Jump    = Opcode{"Jump", OperandScriptSym}
	Command = Opcode{"Command", OperandScriptSym}
	Return  = Opcode{"Return", OperandNone}
)

// Integer math
var (
	Add      = Opcode{"Add", OperandNone}
	Sub      = Opcode{"Sub", OperandNone}
	Multiply = Opcode{"Multiply", OperandNone}
	Divide   = Opcode{"Divide", OperandNone}
	Modulo   = Opcode{"Modulo", OperandNone}
	Or       = Opcode{"Or", OperandNone}
	And      = Opcode{"And", OperandNone}
)

// Long math
var (
	LongAdd      = Opcode{"LongAdd", OperandNone}
	LongSub      = Opcode{"LongSub", OperandNone}
	LongMultiply = Opcode{"LongMultiply", OperandNone}
	LongDivide   = Opcode{"LongDivide", OperandNone}
	LongModulo   = Opcode{"LongModulo", OperandNone}
	LongOr       = Opcode{"LongOr", OperandNone}
	LongAnd      = Opcode{"LongAnd", OperandNone}
)

// Meta
var (
	LineNumber = Opcode{"LineNumber", OperandInt}
)
