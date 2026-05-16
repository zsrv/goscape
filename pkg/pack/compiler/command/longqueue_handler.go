// pkg/pack/compiler/command/longqueue_handler.go
package command

// LongQueueCommandHandler handles dynamic type-checking for the `longqueue`
// command. Mirrors TS LongQueueCommandHandler.ts.
//
// The `longqueue` command is like `queue` but adds an extra int argument
// specifying the action to perform if logout succeeds mid-queue:
//
//	longqueue(script_name, delay, int_arg, logout_action);
//
// The handler checks that:
//   - arg 0 is a script of the queue type
//   - arg 1 is an int (delay ticks)
//   - arg 2 is an int (argument to pass to the script)
//   - arg 3 is an int (logout action)
//
// Sets the expression type to MetaUnit (no return value).

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// LongQueueCommandHandler implements DynamicCommandHandler for `longqueue`.
type LongQueueCommandHandler struct {
	// queueType is the MetaScript type for longqueue scripts. Mirrors TS
	// LongQueueCommandHandler.queueType.
	queueType typ.Type
}

// NewLongQueueCommandHandler constructs a LongQueueCommandHandler. queueType
// is the expected script type for the first argument (the script to queue).
func NewLongQueueCommandHandler(queueType typ.Type) *LongQueueCommandHandler {
	return &LongQueueCommandHandler{queueType: queueType}
}

// TypeCheck ports LongQueueCommandHandler.ts typeCheck verbatim.
func (h *LongQueueCommandHandler) TypeCheck(ctx *semantics.TypeCheckingContext) {
	ctx.CheckArgument(0, h.queueType, false)      // Script to queue.
	ctx.CheckArgument(1, typ.PrimitiveInt, false) // Delay before running.
	ctx.CheckArgument(2, typ.PrimitiveInt, false) // Int arg to pass.
	ctx.CheckArgument(3, typ.PrimitiveInt, false) // Action if logout mid-queue.

	// TODO: (Type safety) Make sure queue script only expects up to 1 int arg.
	// Mirrors TS LongQueueCommandHandler.ts comment.
	expected := typ.TupleFromList([]typ.Type{h.queueType, typ.PrimitiveInt, typ.PrimitiveInt, typ.PrimitiveInt})
	ctx.CheckArgumentTypes(expected, true, false)
	ctx.SetType(typ.MetaUnit)
}

// GenerateCode returns false — cohort A: codegen falls back to the default
// "visit args + emit Command" path.
func (h *LongQueueCommandHandler) GenerateCode(ctx semantics.CodeGenContext) bool {
	return false
}
