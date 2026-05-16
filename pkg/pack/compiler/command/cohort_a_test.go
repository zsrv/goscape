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

// newTestTypeManager builds a TypeManager pre-registered with the three
// MetaScript types (queue/timer/softtimer) that RegisterAllDynCommands looks
// up via FindOrNil.
func newTestTypeManager(t *testing.T) *typ.TypeManager {
	t.Helper()
	tm := typ.NewTypeManager()
	for _, name := range []string{"queue", "timer", "softtimer"} {
		if err := tm.Register(name, typ.NewMetaScript(name, typ.MetaAny, typ.MetaNothing)); err != nil {
			t.Fatal(err)
		}
	}
	return tm
}

// TestRegisterAll_QueueTypedDisabled pins TS L95-102 (queue*-family) +
// L108-110 (longqueue*-family) — gated on features.queueTyped.
func TestRegisterAll_QueueTypedDisabled(t *testing.T) {
	tm := newTestTypeManager(t)
	handlers := map[string]semantics.DynamicCommandHandler{}
	register := func(name string, h semantics.DynamicCommandHandler) { handlers[name] = h }
	RegisterAllDynCommands(tm, semantics.StrictFeatureLevel{DisableQueueTyped: true}, register)

	disallowed := []string{
		"queue*", ".queue*", "weakqueue*", ".weakqueue*",
		"strongqueue*", ".strongqueue*", "longqueue*", ".longqueue*",
	}
	for _, name := range disallowed {
		if _, present := handlers[name]; present {
			t.Errorf("DisableQueueTyped: handler %q should NOT be registered", name)
		}
	}
	for _, name := range []string{"queue", ".queue", "longqueue", "settimer"} {
		if _, present := handlers[name]; !present {
			t.Errorf("DisableQueueTyped: handler %q should still be registered", name)
		}
	}
}

func TestRegisterAll_EnumsDisabled(t *testing.T) {
	tm := newTestTypeManager(t)
	handlers := map[string]semantics.DynamicCommandHandler{}
	register := func(name string, h semantics.DynamicCommandHandler) { handlers[name] = h }
	RegisterAllDynCommands(tm, semantics.StrictFeatureLevel{DisableEnums: true}, register)
	if _, present := handlers["enum"]; present {
		t.Error("DisableEnums: 'enum' handler should NOT be registered")
	}
	for _, name := range []string{"queue", "settimer", "struct_param", "db_find"} {
		if _, present := handlers[name]; !present {
			t.Errorf("DisableEnums: handler %q should still be registered", name)
		}
	}
}

func TestRegisterAll_StructsDisabled(t *testing.T) {
	tm := newTestTypeManager(t)
	handlers := map[string]semantics.DynamicCommandHandler{}
	register := func(name string, h semantics.DynamicCommandHandler) { handlers[name] = h }
	RegisterAllDynCommands(tm, semantics.StrictFeatureLevel{DisableStructs: true}, register)
	if _, present := handlers["struct_param"]; present {
		t.Error("DisableStructs: 'struct_param' handler should NOT be registered")
	}
	for _, name := range []string{"queue", "settimer", "enum", "db_find"} {
		if _, present := handlers[name]; !present {
			t.Errorf("DisableStructs: handler %q should still be registered", name)
		}
	}
}

func TestRegisterAll_DBTablesDisabled(t *testing.T) {
	tm := newTestTypeManager(t)
	handlers := map[string]semantics.DynamicCommandHandler{}
	register := func(name string, h semantics.DynamicCommandHandler) { handlers[name] = h }
	RegisterAllDynCommands(tm, semantics.StrictFeatureLevel{DisableDBTables: true}, register)
	for _, name := range []string{"db_find", "db_find_refine", "db_find_with_count", "db_find_refine_with_count", "db_getfield"} {
		if _, present := handlers[name]; present {
			t.Errorf("DisableDBTables: %q handler should NOT be registered", name)
		}
	}
	for _, name := range []string{"queue", "settimer", "enum", "struct_param"} {
		if _, present := handlers[name]; !present {
			t.Errorf("DisableDBTables: handler %q should still be registered", name)
		}
	}
}

func TestRegisterAll_AllEnabled_GatesAllRegister(t *testing.T) {
	tm := newTestTypeManager(t)
	handlers := map[string]semantics.DynamicCommandHandler{}
	register := func(name string, h semantics.DynamicCommandHandler) { handlers[name] = h }
	RegisterAllDynCommands(tm, semantics.StrictFeatureLevel{}, register)
	for _, name := range []string{
		"queue*", ".queue*", "longqueue*", "enum", "struct_param", "db_find", "db_getfield",
	} {
		if _, present := handlers[name]; !present {
			t.Errorf("default features: %q handler should be registered", name)
		}
	}
}
