// pkg/pack/compiler/command/debug_script_handler.go
package command

// ScriptCommandHandler is a developer debug command that replaces the call
// with a string constant containing the name of the enclosing script.
// Mirrors TS debug/ScriptCommandHandler.ts.
//
// TypeCheck:
//   - Accepts no arguments (MetaUnit).
//   - Sets expression type to PrimitiveString.
//
// GenerateCode:
//   - Emits LineInstruction.
//   - Emits PushConstantString("[trigger,name]") — the enclosing script name.
//
// NAI-207-D-COHORT-B-SCRIPT-MINIMAL: TS uses expression.findParentByType(Script)
// (NAI-204-D-AST-NO-PARENT). Goscape obtains the enclosing script name via
// CodeGeneratorContext.ActiveScript() which exposes the generator's active
// RuneScript. RuneScript.FullName carries "[triggerIdent,name]" exactly as
// TS constructs it from script.trigger.text + script.name.text.

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// ScriptCommandHandler implements DynamicCommandHandler for the `script` debug
// command.
type ScriptCommandHandler struct{}

// TypeCheck ports ScriptCommandHandler.ts typeCheck verbatim.
func (h *ScriptCommandHandler) TypeCheck(ctx *semantics.TypeCheckingContext) {
	ctx.CheckArgumentTypes(typ.MetaUnit, true, false)
	ctx.SetType(typ.PrimitiveString)
}

// GenerateCode ports ScriptCommandHandler.ts generateCode verbatim.
// Emits: LineInstruction + PushConstantString("[trigger,name]").
// Mirrors TS L22-L31.
//
// NAI-207-D-COHORT-B-SCRIPT-MINIMAL: instead of expression.findParentByType(Script),
// uses CodeGeneratorContext.ActiveScript() to get the enclosing RuneScript.
// Panics if called outside a script context (mirrors TS `throw new Error("Script not found.")`).
func (h *ScriptCommandHandler) GenerateCode(ctx semantics.CodeGenContext) bool {
	cgc := ctx.(*codegen.CodeGeneratorContext)

	script := cgc.ActiveScript()
	if script == nil {
		panic("ScriptCommandHandler.GenerateCode: no active script (NAI-207-D-COHORT-B-SCRIPT-MINIMAL)")
	}

	// TS: const name = `[${script.trigger.text}, ${script.name.text}]`
	// RuneScript.FullName carries "[triggerIdent,name]" (no space — see
	// runescript.go NewRuneScript). Match TS exactly: trigger + ", " + name.
	name := "[" + script.Trigger.Identifier + ", " + script.Name + "]"

	cgc.LineInstruction(cgc.Expression)
	cgc.Instruction(codegen.PushConstantString, name, cgc.Expression.Source())

	return true
}
