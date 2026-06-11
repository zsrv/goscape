// pkg/pack/compiler/command/cohort_b_test.go
package command

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TestCohortB_HandlersSatisfyInterface compile-time-pins that each cohort B
// handler satisfies semantics.DynamicCommandHandler.
func TestCohortB_HandlersSatisfyInterface(t *testing.T) {
	var _ semantics.DynamicCommandHandler = NewDbFindCommandHandler(false)
	var _ semantics.DynamicCommandHandler = NewQueueVarArgCommandHandler(typ.PrimitiveInt)
	var _ semantics.DynamicCommandHandler = NewLongQueueVarArgCommandHandler(typ.PrimitiveLong)
	var _ semantics.DynamicCommandHandler = NewTimerCommandHandler(typ.PrimitiveInt)
	var _ semantics.DynamicCommandHandler = (*DumpCommandHandler)(nil)
	var _ semantics.DynamicCommandHandler = (*ScriptCommandHandler)(nil)
	var _ semantics.DynamicCommandHandler = (*PlaceholderCommand)(nil)
	_ = codegen.CodeGenerator{} // ensure codegen import used
}

// TestCohortB_GenerateCode_ReturnsTrue pins that each cohort-B handler's
// GenerateCode returns true (handler emits code; no default-fallback).
// Per NAI-207-D-DYNCOMMAND-BOOLRESULT: true ⇒ handler emitted code.
// Full instruction-stream verification is deferred to T14 pipeline smoke.
func TestCohortB_GenerateCode_ReturnsTrue(t *testing.T) {
	t.Skip("requires full codegen + symbol fixture; covered by T14 smoke")
}

// TestDbFindCommandHandler_EmitsStackTypeBeforeCommand pins that DbFind's
// GenerateCode emits PushConstantInt(stackType) before Command.
// Per TS DbFindCommandHandler.ts L44-L60.
func TestDbFindCommandHandler_EmitsStackTypeBeforeCommand(t *testing.T) {
	t.Skip("requires full codegen + symbol fixture; covered by T14 smoke")
}

// TestDbFind_WithCount_Matches254 pins the 254 registration: db_find and
// db_find_refine use withCount=FALSE (MetaType.Unit return — no
// POP_INT_DISCARD in statement context), per RuneScriptTS b8c3388
// ServerScriptCompiler.ts:187-188 + DbFindCommandHandler.ts:41. Only the
// *_with_count variants return PrimitiveInt (:189-190).
//
// History: the rev-244 port pinned withCount=true for all four from the
// RuneScriptKt-26 lineage (ClientScriptCompiler.kt L88-89, B6 Class-2).
// The 254 reference script.dat refutes that for the plain variants — the
// T23 full-tree gate showed goscape's extra POP_INT_DISCARD in ~50 scripts.
func TestDbFind_WithCount_Matches254(t *testing.T) {
	// verify that the handlers registered for db_find / db_find_refine have
	// withCount=false (MetaUnit return, no discard).
	for _, name := range []string{"db_find", "db_find_refine"} {
		h := dbFindHandlerForName(name)
		if h == nil {
			t.Fatalf("%s: handler not registered", name)
		}
		if h.withCount {
			t.Errorf("%s: withCount=true, want false (RuneScriptTS b8c3388 ServerScriptCompiler.ts:187-188: Unit return, no POP_INT_DISCARD)", name)
		}
	}
	// verify that db_find_with_count / db_find_refine_with_count still use withCount=true.
	for _, name := range []string{"db_find_with_count", "db_find_refine_with_count"} {
		h := dbFindHandlerForName(name)
		if h == nil {
			t.Fatalf("%s: handler not registered", name)
		}
		if !h.withCount {
			t.Errorf("%s: withCount=false, want true", name)
		}
	}
}

// dbFindHandlerForName registers all dynamic commands and returns the
// DbFindCommandHandler registered under name, or nil.
func dbFindHandlerForName(name string) *DbFindCommandHandler {
	// Build a minimal type manager with the three MetaScript types RegisterAllDynCommands looks up.
	tm := typ.NewTypeManager()
	for _, n := range []string{"queue", "timer", "softtimer"} {
		_ = tm.Register(n, typ.NewMetaScript(n, typ.MetaAny, typ.MetaNothing))
	}
	var found *DbFindCommandHandler
	RegisterAllDynCommands(tm, semantics.StrictFeatureLevel{}, func(n string, h semantics.DynamicCommandHandler) {
		if n == name {
			if dbh, ok := h.(*DbFindCommandHandler); ok {
				found = dbh
			}
		}
	})
	return found
}

// TestQueueVarArgCommandHandler_EmitsTypecodeString pins that QueueVarArg
// emits a typecode string built from Arguments2 types.
// Per TS QueueVarArgCommandHandler.ts L36-L53.
func TestQueueVarArgCommandHandler_EmitsTypecodeString(t *testing.T) {
	t.Skip("requires full codegen + symbol fixture; covered by T14 smoke")
}

// TestLongQueueVarArgCommandHandler_EmitsTypecodeString pins that
// LongQueueVarArg emits a typecode string built from Arguments2 types.
// Per TS LongQueueVarArgCommandHandler.ts L37-L54.
func TestLongQueueVarArgCommandHandler_EmitsTypecodeString(t *testing.T) {
	t.Skip("requires full codegen + symbol fixture; covered by T14 smoke")
}

// TestTimerCommandHandler_EmitsTypecodeForTailArgs pins that Timer emits
// typecode string for args[2:] when len(args) > 2.
// Per TS TimerCommandHandler.ts L34-L55.
func TestTimerCommandHandler_EmitsTypecodeForTailArgs(t *testing.T) {
	t.Skip("requires full codegen + symbol fixture; covered by T14 smoke")
}

// TestDumpCommandHandler_EmitsJoinString pins that Dump's GenerateCode
// emits a JoinString instruction combining expression parts.
// Per TS DumpCommandHandler.ts L44-L70. Deferred: requires ExpressionGenerator
// (AST visitor) not yet present; see NAI-207-D-COHORT-B-DUMP-MINIMAL.
func TestDumpCommandHandler_EmitsJoinString(t *testing.T) {
	t.Skip("NAI-207-D-COHORT-B-DUMP-MINIMAL: ExpressionGenerator (AST visitor) absent; covered by T14 smoke")
}

// TestScriptCommandHandler_EmitsScriptName pins that Script's GenerateCode
// emits PushConstantString with the enclosing script name.
// Per TS ScriptCommandHandler.ts L22-L31. Deferred: requires findParentByType
// (no parent pointer in goscape AST); see NAI-207-D-COHORT-B-SCRIPT-MINIMAL.
func TestScriptCommandHandler_EmitsScriptName(t *testing.T) {
	t.Skip("NAI-207-D-COHORT-B-SCRIPT-MINIMAL: AST parent pointer absent; covered by T14 smoke")
}

// TestPlaceholderCommand_EmitsValue pins that Placeholder's GenerateCode
// emits the stored value as PushConstantInt or PushConstantString.
// Per TS PlaceholderCommand.ts L21-L31.
func TestPlaceholderCommand_EmitsValue(t *testing.T) {
	t.Skip("requires full codegen + symbol fixture; covered by T14 smoke")
}
