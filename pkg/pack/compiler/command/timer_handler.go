// pkg/pack/compiler/command/timer_handler.go
package command

// TimerCommandHandler handles dynamic type-checking and code generation for
// timer commands. Mirrors TS TimerCommandHandler.ts.
//
// The ctor receives the timer's script type (the trigger marker).
//
// TypeCheck:
//   - arg 0: the timer-script type.
//   - arg 1: int (interval ticks).
//   - Additional args: variadic args matching the script's parameter types.
//   - Sets expression type to MetaUnit.
//
// GenerateCode:
//   - Emit all arguments via VisitNode.
//   - If len(args) > 2: emit PushConstantString with typecodes of args[2:].
//   - Else: emit PushConstantString("").
//   - Emit Command.

import (
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TimerCommandHandler implements DynamicCommandHandler for timer commands.
type TimerCommandHandler struct {
	// timerType is the MetaScript type for timer scripts. Mirrors TS
	// TimerCommandHandler.timerType.
	timerType typ.Type
}

// NewTimerCommandHandler constructs a TimerCommandHandler. queueType is the
// expected script type for the first argument (the timer marker). The
// parameter name "queueType" mirrors TS TimerCommandHandler.ts constructor.
func NewTimerCommandHandler(queueType typ.Type) *TimerCommandHandler {
	return &TimerCommandHandler{timerType: queueType}
}

// TypeCheck ports TimerCommandHandler.ts typeCheck verbatim.
func (h *TimerCommandHandler) TypeCheck(ctx *semantics.TypeCheckingContext) {
	timer := ctx.CheckArgument(0, h.timerType, false) // Timer script.
	ctx.CheckArgument(1, typ.PrimitiveInt, false)     // Interval.

	expectedTypesList := []typ.Type{h.timerType, typ.PrimitiveInt}

	if timer != nil {
		timerExprType := semantics.ExprType(timer)
		if timerExprType != nil {
			// NAI-207-D-COHORT-B-QUEUEVARARG-TRIGGERCHECK: same Representation()
			// trigger-identity approximation used by Queue/LongQueue vararg handlers.
			if params, _, ok := typ.IsMetaScript(timerExprType); ok &&
				timerExprType.Representation() == h.timerType.Representation() &&
				params != typ.MetaUnit {
				expectedTypesList = append(expectedTypesList, typ.TupleToList(params)...)
			}
		}
	}

	ctx.CheckArgumentTypes(typ.TupleFromList(expectedTypesList), true, false)
	ctx.SetType(typ.MetaUnit)
}

// GenerateCode ports TimerCommandHandler.ts generateCode verbatim.
// Emits: all args + typecode string (tail args > 2) or "" + Command.
// Mirrors TS L34-L55.
func (h *TimerCommandHandler) GenerateCode(ctx semantics.CodeGenContext) bool {
	cgc := ctx.(*codegen.CodeGeneratorContext)

	args := cgc.Arguments()

	// Visit all arguments. Mirrors TS L37-38.
	for _, arg := range args {
		cgc.VisitNode(arg)
	}

	// Emit typecode string for tail args (args[2:]) when len > 2.
	// Mirrors TS L44-L53: if (args.length > 2) { ... } else { "" }
	var typecode string
	if len(args) > 2 {
		var sb strings.Builder
		for _, arg := range args[2:] {
			if t := semantics.ExprType(arg); t != nil {
				if code, ok := t.Code(); ok {
					sb.WriteString(code)
				}
			}
		}
		typecode = sb.String()
	}
	cgc.Instruction(codegen.PushConstantString, typecode, cgc.Expression.Source())

	// Emit the Command.
	cgc.Command()

	return true
}
