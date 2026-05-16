// pkg/pack/compiler/command/queue_vararg_handler.go
package command

// QueueVarArgCommandHandler handles dynamic type-checking and code generation
// for the variadic-arg queue command. Mirrors TS QueueVarArgCommandHandler.ts.
//
// Unlike QueueCommandHandler (cohort A), this handler:
//   - accepts a variadic secondary argument list (Arguments2) matching the
//     queued script's parameter types.
//   - emits a typecode string built from those variadic args.
//
// TypeCheck:
//   - arg 0: the queue-script type.
//   - arg 1: int (delay ticks).
//   - Arguments2: variadic args matching the script's parameter types (if any).
//   - Sets expression type to MetaUnit.
//
// GenerateCode:
//   - Emit Arguments (primary args).
//   - Emit Arguments2 (varargs).
//   - Emit PushConstantString(typecode) — codes from vararg types or "".
//   - Emit Command.

import (
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// QueueVarArgCommandHandler implements DynamicCommandHandler for the vararg
// queue command.
type QueueVarArgCommandHandler struct {
	// queueType is the expected MetaScript type for the first argument.
	// Mirrors TS QueueVarArgCommandHandler.queueType.
	queueType typ.Type
}

// NewQueueVarArgCommandHandler constructs a QueueVarArgCommandHandler.
// queueType is the expected script type for the first argument.
func NewQueueVarArgCommandHandler(queueType typ.Type) *QueueVarArgCommandHandler {
	return &QueueVarArgCommandHandler{queueType: queueType}
}

// TypeCheck ports QueueVarArgCommandHandler.ts typeCheck verbatim.
func (h *QueueVarArgCommandHandler) TypeCheck(ctx *semantics.TypeCheckingContext) {
	queue := ctx.CheckArgument(0, h.queueType, false) // Script to queue.
	ctx.CheckArgument(1, typ.PrimitiveInt, false)     // Delay before running.

	baseTypeList := []typ.Type{h.queueType, typ.PrimitiveInt}
	var varArgTypesList []typ.Type

	if queue != nil {
		queueExprType := semantics.ExprType(queue)
		if queueExprType != nil {
			// TS: queueExpressionType instanceof MetaType.Script &&
			//     queueExpressionType.trigger == this.queueType.trigger &&
			//     queueExpressionType.parameterType != MetaType.Unit
			//
			// In Go: IsMetaScript discriminates; trigger identity is checked via
			// Representation() equality (goscape stores trigger identifier as rep).
			// NAI-207-D-COHORT-B-QUEUEVARARG-TRIGGERCHECK: TS uses object identity
			// (trigger pointer); goscape approximates via Representation() equality
			// since metaScript.trigger is unexported. Faithfully equivalent for all
			// registered trigger types (each has a unique identifier string).
			if params, _, ok := typ.IsMetaScript(queueExprType); ok &&
				queueExprType.Representation() == h.queueType.Representation() &&
				params != typ.MetaUnit {
				varArgTypesList = append(varArgTypesList, typ.TupleToList(params)...)
			}
		}
	}

	ctx.CheckArgumentTypes(typ.TupleFromList(baseTypeList), true, false)
	ctx.CheckArgumentTypes(typ.TupleFromList(varArgTypesList), true, true)
	ctx.SetType(typ.MetaUnit)
}

// GenerateCode ports QueueVarArgCommandHandler.ts generateCode verbatim.
// Emits: primary args + vararg args + typecode string + Command.
// Mirrors TS L36-L53.
func (h *QueueVarArgCommandHandler) GenerateCode(ctx semantics.CodeGenContext) bool {
	cgc := ctx.(*codegen.CodeGeneratorContext)

	// Emit primary arguments.
	for _, arg := range cgc.Arguments() {
		cgc.VisitNode(arg)
	}

	// Emit secondary (vararg) arguments.
	args2 := cgc.Arguments2()
	for _, arg := range args2 {
		cgc.VisitNode(arg)
	}

	// Build and emit the typecode string. Mirrors TS L43-L51.
	var typecode string
	if len(args2) > 0 {
		var sb strings.Builder
		for _, arg := range args2 {
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
