// pkg/pack/compiler/command/queue_handler.go
package command

// QueueCommandHandler handles dynamic type-checking for the `queue` command.
// Mirrors TS QueueCommandHandler.ts.
//
// The `queue` command enqueues a script to run after a delay:
//
//	queue(script_name, delay, int_arg);
//
// The handler checks that:
//   - arg 0 is a script of the queue type
//   - arg 1 is an int (delay ticks)
//   - arg 2 is an int (argument to pass to the script)
//
// Sets the expression type to MetaUnit (no return value).

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// QueueCommandHandler implements DynamicCommandHandler for the `queue` command.
type QueueCommandHandler struct {
	// queueType is the MetaScript type for queue scripts. Mirrors TS
	// QueueCommandHandler.queueType.
	queueType typ.Type
}

// NewQueueCommandHandler constructs a QueueCommandHandler. queueType is the
// expected script type for the first argument (the script to queue).
func NewQueueCommandHandler(queueType typ.Type) *QueueCommandHandler {
	return &QueueCommandHandler{queueType: queueType}
}

// TypeCheck ports QueueCommandHandler.ts typeCheck verbatim.
func (h *QueueCommandHandler) TypeCheck(ctx *semantics.TypeCheckingContext) {
	ctx.CheckArgument(0, h.queueType, false)      // Script to queue.
	ctx.CheckArgument(1, typ.PrimitiveInt, false) // Delay before running.
	ctx.CheckArgument(2, typ.PrimitiveInt, false) // Int arg to pass.

	// TODO: (Type safety) Make sure queue script only expects up to 1 int arg.
	// Mirrors TS QueueCommandHandler.ts comment.
	expected := typ.TupleFromList([]typ.Type{h.queueType, typ.PrimitiveInt, typ.PrimitiveInt})
	ctx.CheckArgumentTypes(expected, true, false)
	ctx.SetType(typ.MetaUnit)
}

// GenerateCode returns false — cohort A: codegen falls back to the default
// "visit args + emit Command" path.
func (h *QueueCommandHandler) GenerateCode(ctx semantics.CodeGenContext) bool {
	return false
}
