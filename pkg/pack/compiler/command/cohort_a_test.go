// pkg/pack/compiler/command/cohort_a_test.go
package command

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TestCohortA_HandlersSatisfyInterface compile-time-pins that each cohort A
// handler satisfies semantics.DynamicCommandHandler.
func TestCohortA_HandlersSatisfyInterface(t *testing.T) {
	var _ semantics.DynamicCommandHandler = (*EnumCommandHandler)(nil)
	var _ semantics.DynamicCommandHandler = NewParamCommandHandler(nil)
	var _ semantics.DynamicCommandHandler = NewQueueCommandHandler(typ.PrimitiveInt)
	var _ semantics.DynamicCommandHandler = NewLongQueueCommandHandler(typ.PrimitiveInt)
	var _ semantics.DynamicCommandHandler = (*DbGetFieldCommandHandler)(nil)
}

// TestCohortA_GenerateCode_FallsBack pins that each cohort-A handler's
// GenerateCode returns false ⇒ default-fallback in emitDynamicCommand.
func TestCohortA_GenerateCode_FallsBack(t *testing.T) {
	for _, h := range []semantics.DynamicCommandHandler{
		&EnumCommandHandler{},
		NewParamCommandHandler(nil),
		NewQueueCommandHandler(typ.PrimitiveInt),
		NewLongQueueCommandHandler(typ.PrimitiveInt),
		&DbGetFieldCommandHandler{},
	} {
		// nil context is acceptable for the no-op codegen path.
		if got := h.GenerateCode(nil); got != false {
			t.Errorf("%T.GenerateCode: got true, want false (cohort A: default-fallback)", h)
		}
	}
}
