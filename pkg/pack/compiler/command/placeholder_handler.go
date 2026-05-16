// pkg/pack/compiler/command/placeholder_handler.go
package command

// PlaceholderCommand is a command-handler placeholder that emits a constant
// value. Mirrors TS PlaceholderCommand.ts.
//
// The TS PlaceholderCommand.generateCode? is optional (marked with `?`). Per
// NAI-207-D-DYNCOMMAND-BOOLRESULT: since TS has a body, the goscape port
// declares GenerateCode and returns true.
//
// TypeCheck:
//   - Accepts no arguments (MetaUnit).
//   - Sets expression type to the constructor-supplied type.
//
// GenerateCode:
//   - Emits LineInstruction.
//   - Emits PushConstantInt(value) if value is int, or PushConstantString(value) if string.
//   - Panics for unsupported value types (mirrors TS `throw new Error(...)`).

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// PlaceholderCommand implements DynamicCommandHandler for placeholder commands.
type PlaceholderCommand struct {
	// typ is the return type of the placeholder expression.
	// Mirrors TS PlaceholderCommand.type.
	typ typ.Type
	// value is the constant to emit. Must be int or string.
	// Mirrors TS PlaceholderCommand.value (typed as unknown).
	value any
}

// NewPlaceholderCommand constructs a PlaceholderCommand. t is the expression
// return type; value must be an int or string constant to emit.
func NewPlaceholderCommand(t typ.Type, value any) *PlaceholderCommand {
	return &PlaceholderCommand{typ: t, value: value}
}

// TypeCheck ports PlaceholderCommand.ts typeCheck verbatim.
func (h *PlaceholderCommand) TypeCheck(ctx *semantics.TypeCheckingContext) {
	ctx.CheckArgumentTypes(typ.MetaUnit, true, false)
	ctx.SetType(h.typ)
}

// GenerateCode ports PlaceholderCommand.ts generateCode verbatim.
// Emits: LineInstruction + PushConstantInt or PushConstantString.
// Mirrors TS L21-L31.
func (h *PlaceholderCommand) GenerateCode(ctx semantics.CodeGenContext) bool {
	cgc := ctx.(*codegen.CodeGeneratorContext)

	cgc.LineInstruction(cgc.Expression)

	switch v := h.value.(type) {
	case int:
		cgc.Instruction(codegen.PushConstantInt, v, cgc.Expression.Source())
	case string:
		cgc.Instruction(codegen.PushConstantString, v, cgc.Expression.Source())
	default:
		// Mirrors TS `throw new Error(`Unsupported value: ${this.value}`)`.
		panic(fmt.Sprintf("PlaceholderCommand.GenerateCode: unsupported value type %T", h.value))
	}

	return true
}
