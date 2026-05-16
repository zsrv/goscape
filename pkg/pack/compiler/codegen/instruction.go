package codegen

import "github.com/zsrv/goscape/pkg/pack/compiler/lexer"

// Instruction is a single codegen output. The Operand's concrete Go type
// must match Opcode.Kind — verified at emission site, not at construction.
// Source may be zero-value when the instruction is synthesised (e.g. default
// returns). Mirrors TS Instruction<T> (Instruction.ts).
type Instruction struct {
	Opcode  Opcode
	Operand any
	Source  lexer.NodeSourceLocation
}
