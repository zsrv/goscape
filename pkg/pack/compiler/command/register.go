// pkg/pack/compiler/command/register.go — canonical dynamic-command registry.
// Ports TS ServerScriptCompiler.setupDefaultTypeCheckers
// (ServerScriptCompiler.ts L84-L201).
package command

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// RegisterAllDynCommands invokes register(name, handler) once per known
// dynamic command. The register callback adopts each handler — typically by
// inserting into a map[string]semantics.DynamicCommandHandler that is
// passed to both TypeChecker and CodeGenerator constructors. Mirrors TS
// addDynamicCommandHandler-call sites in setupDefaultTypeCheckers.
//
// features is accepted for forward-compatibility; no StrictFeatureLevel
// field currently gates any handler registration in goscape's HEAD
// (NAI-207-D-REGISTERALL-NO-FEATURES: TS feature-flag equivalents
// DisableQueueVarArg, DisableLongQueueVarArg, DisableEnumStructParam,
// DisableDbCommands are absent from goscape's StrictFeatureLevel; all
// handlers are registered unconditionally until those flags are added).
func RegisterAllDynCommands(
	tm *typ.TypeManager,
	features semantics.StrictFeatureLevel,
	register func(name string, h semantics.DynamicCommandHandler),
) {
	queueType := tm.FindOrNil("queue", false)
	timerType := tm.FindOrNil("timer", false)
	softTimerType := tm.FindOrNil("softtimer", false)

	// queue / .queue / weakqueue / .weakqueue / strongqueue / .strongqueue.
	// Mirrors TS L87-L94.
	for _, name := range []string{"queue", ".queue", "weakqueue", ".weakqueue", "strongqueue", ".strongqueue"} {
		register(name, NewQueueCommandHandler(queueType))
	}

	// queue* vararg variants. Mirrors TS L95-L102.
	// NAI-207-D-REGISTERALL-NO-FEATURES: TS gates these on features.queueTyped;
	// goscape registers unconditionally (no DisableQueueVarArg flag at HEAD).
	for _, name := range []string{"queue*", ".queue*", "weakqueue*", ".weakqueue*", "strongqueue*", ".strongqueue*"} {
		register(name, NewQueueVarArgCommandHandler(queueType))
	}

	// longqueue / .longqueue + vararg variants. Mirrors TS L103-L110.
	for _, name := range []string{"longqueue", ".longqueue"} {
		register(name, NewLongQueueCommandHandler(queueType))
	}
	// NAI-207-D-REGISTERALL-NO-FEATURES: TS gates longqueue* on features.queueTyped.
	for _, name := range []string{"longqueue*", ".longqueue*"} {
		register(name, NewLongQueueVarArgCommandHandler(queueType))
	}

	// settimer / .settimer. Mirrors TS L111-L116.
	for _, name := range []string{"settimer", ".settimer"} {
		register(name, NewTimerCommandHandler(timerType))
	}
	// softtimer / .softtimer. Mirrors TS L117-L122.
	for _, name := range []string{"softtimer", ".softtimer"} {
		register(name, NewTimerCommandHandler(softTimerType))
	}

	// loc/npc/obj _param variants. Mirrors TS L104-L113.
	// NAI-207-D-PARAM-NO-CONSTRAINT: TS passes ScriptVarType.loc/npc/obj/struct
	// as the paramReturnType constraint; goscape has no equivalent ScriptVarType
	// constants at HEAD. nil is passed instead (unconstrained), deferring type
	// narrowing to a later NAI.
	register("lc_param", NewParamCommandHandler(nil))
	register("loc_param", NewParamCommandHandler(nil))
	register("nc_param", NewParamCommandHandler(nil))
	register("npc_param", NewParamCommandHandler(nil))
	register("oc_param", NewParamCommandHandler(nil))
	register("obj_param", NewParamCommandHandler(nil))

	// enum. Mirrors TS L123-L125.
	// NAI-207-D-REGISTERALL-NO-FEATURES: TS gates on features.enums; registered
	// unconditionally (no DisableEnumStructParam flag at HEAD).
	register("enum", &EnumCommandHandler{})

	// struct_param. Mirrors TS L126-L128.
	// NAI-207-D-PARAM-NO-CONSTRAINT + NAI-207-D-REGISTERALL-NO-FEATURES: nil
	// constraint; registered unconditionally.
	register("struct_param", NewParamCommandHandler(nil))

	// db_find / db_find_refine / db_find_with_count / db_find_refine_with_count /
	// db_getfield. Mirrors TS L129-L155.
	// NAI-207-D-REGISTERALL-NO-FEATURES: TS gates on features.dbTables;
	// registered unconditionally (no DisableDbCommands flag at HEAD).
	register("db_find", NewDbFindCommandHandler(false))
	register("db_find_refine", NewDbFindCommandHandler(false))
	register("db_find_with_count", NewDbFindCommandHandler(true))
	register("db_find_refine_with_count", NewDbFindCommandHandler(true))
	register("db_getfield", &DbGetFieldCommandHandler{})

	// debug commands. Mirrors TS L156-L160.
	register("dump", &DumpCommandHandler{})
	register("script", &ScriptCommandHandler{})

	_ = features // consumed above: see NAI-207-D-REGISTERALL-NO-FEATURES
}
