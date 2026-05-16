// pkg/pack/compiler/command/longqueue_vararg_handler.go
package command

// LongQueueVarArgCommandHandler handles dynamic type-checking and code generation
// for the variadic-arg longqueue command. Mirrors TS
// LongQueueVarArgCommandHandler.ts.
//
// Symmetric to QueueVarArgCommandHandler but with an extra int argument
// (action to perform if logout succeeds mid-queue).
//
// TypeCheck:
//   - arg 0: the queue-script type.
//   - arg 1: int (delay ticks).
//   - arg 2: int (logout action).
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

// LongQueueVarArgCommandHandler implements DynamicCommandHandler for the vararg
// longqueue command.
type LongQueueVarArgCommandHandler struct {
	// queueType is the expected MetaScript type for the first argument.
	// Mirrors TS LongQueueVarArgCommandHandler.queueType.
	queueType typ.Type
}

// NewLongQueueVarArgCommandHandler constructs a LongQueueVarArgCommandHandler.
// queueType is the expected script type for the first argument.
func NewLongQueueVarArgCommandHandler(queueType typ.Type) *LongQueueVarArgCommandHandler {
	return &LongQueueVarArgCommandHandler{queueType: queueType}
}

// TypeCheck ports LongQueueVarArgCommandHandler.ts typeCheck verbatim.
func (h *LongQueueVarArgCommandHandler) TypeCheck(ctx *semantics.TypeCheckingContext) {
	queue := ctx.CheckArgument(0, h.queueType, false) // Script to queue.
	ctx.CheckArgument(1, typ.PrimitiveInt, false)     // Delay before running.
	ctx.CheckArgument(2, typ.PrimitiveInt, false)     // Action if logout mid-queue.

	baseTypeList := []typ.Type{h.queueType, typ.PrimitiveInt, typ.PrimitiveInt}
	var varArgTypesList []typ.Type

	if queue != nil {
		queueExprType := semantics.ExprType(queue)
		if queueExprType != nil {
			// NAI-207-D-COHORT-B-QUEUEVARARG-TRIGGERCHECK: same approximation as
			// QueueVarArgCommandHandler — Representation() equality for trigger identity.
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

// GenerateCode ports LongQueueVarArgCommandHandler.ts generateCode verbatim.
// Emits: primary args + vararg args + typecode string + Command.
// Mirrors TS L37-L54.
func (h *LongQueueVarArgCommandHandler) GenerateCode(ctx semantics.CodeGenContext) bool {
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

	// Build and emit the typecode string. Mirrors TS L44-L51.
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
