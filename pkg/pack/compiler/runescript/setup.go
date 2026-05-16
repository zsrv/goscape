// pkg/pack/compiler/runescript/setup.go
package runescript

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/command"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// Setup installs the compiler's types, triggers, sym-loaders, and dynamic
// command handlers. Mirrors TS ScriptCompiler constructor body +
// ServerScriptCompiler.setup() L60-212.
func (c *ServerScriptCompiler) Setup() {
	if c.Triggers == nil {
		c.Triggers = trigger.NewTriggerManager()
	}
	if c.RootTable == nil {
		c.RootTable = symbol.NewSymbolTable(nil)
	}
	if c.DynHandlers == nil {
		c.DynHandlers = map[string]semantics.DynamicCommandHandler{}
	}
	if c.CommandPointers == nil {
		c.CommandPointers = map[string]*pointer.PointerHolder{}
	}

	// TS ScriptCompiler constructor L96-115: primitives + any/type aliases.
	for _, p := range typ.PrimitiveAll {
		_ = c.TypeManager.RegisterByRepresentation(p)
	}
	_ = c.TypeManager.Register("any", typ.MetaAny)
	_ = c.TypeManager.Register("type", typ.MetaAny)
	registerDefaultTypeCheckers(c.TypeManager)
	_ = c.Triggers.RegisterTrigger(trigger.CommandTrigger)

	// TS ServerScriptCompiler.setup L60-212.
	_ = c.Triggers.RegisterAll(trigger.ServerTriggerTypeAll)

	c.registerScriptVarTypes()

	// TS L70: long.AllowDeclaration=false, AllowParameter=true.
	_ = c.TypeManager.ChangeOptions("long", func(o *typ.TypeOptions) {
		o.AllowDeclaration = false
		o.AllowParameter = true
	})

	// TS L78-80: proc gated on DisableProcs.
	if !c.Features.DisableProcs {
		_ = c.TypeManager.Register("proc", typ.NewMetaScript("proc", typ.MetaUnit, typ.MetaUnit))
	}
	_ = c.TypeManager.Register("label", typ.NewMetaScript("label", typ.MetaUnit, typ.MetaNothing))

	// TS L83-84: namedobj → obj.
	c.TypeManager.AddTypeChecker(func(left, right typ.Type) bool {
		return left == typ.ScriptVarObj && right == typ.ScriptVarNamedObj
	})

	c.addSymConstantLoaders()

	// TS L88-93: walktrigger, queue, timer MetaScript.
	_ = c.TypeManager.Register("walktrigger", typ.NewMetaScript("walktrigger", typ.MetaAny, typ.MetaNothing))
	_ = c.TypeManager.Register("queue", typ.NewMetaScript("queue", typ.MetaAny, typ.MetaNothing))
	_ = c.TypeManager.Register("timer", typ.NewMetaScript("timer", typ.MetaAny, typ.MetaNothing))

	// TS L110-176 — sym-loaders gated on CompilerSymbols[name] presence.
	c.addSymLoader("loc", typ.ScriptVarLoc)
	c.addSymLoader("npc", typ.ScriptVarNpc)
	c.addSymLoader("obj", typ.ScriptVarNamedObj)
	c.addSymLoader("component", typ.ScriptVarComponent)
	c.addSymLoader("interface", typ.ScriptVarInterface)
	c.addSymLoader("overlayinterface", typ.ScriptVarOverlayInterface)
	c.addSymLoader("fontmetrics", typ.ScriptVarFontMetrics)
	c.addSymLoader("category", typ.PrimitiveCategory)
	c.addSymLoader("hunt", typ.ScriptVarHunt)
	c.addSymLoader("inv", typ.ScriptVarInv)
	c.addSymLoader("idk", typ.ScriptVarIdKit)
	c.addSymLoader("mesanim", typ.ScriptVarMesAnim)

	// param + intparam — TS L138-140 ParamType wrappers.
	_ = c.TypeManager.Register("param", typ.NewParamType(typ.MetaAny))
	_ = c.TypeManager.Register("intparam", typ.NewParamType(typ.PrimitiveInt))
	c.addSymLoaderWithSupplier("param", func(sub typ.Type) typ.Type {
		return typ.NewParamType(sub)
	})
	c.addSymLoader("seq", typ.ScriptVarSeq)
	c.addSymLoader("spotanim", typ.ScriptVarSpotAnim)

	// TS L147-149: varp register + protected loader.
	_ = c.TypeManager.Register("varp", typ.NewVarPlayerType(typ.MetaAny))
	c.addProtectedSymLoaderWithSupplier("varp", func(sub typ.Type) typ.Type {
		return typ.NewVarPlayerType(sub)
	})
	c.addSymLoaderWithSupplier("varn", func(sub typ.Type) typ.Type {
		return typ.NewVarNpcType(sub)
	})
	c.addSymLoaderWithSupplier("vars", func(sub typ.Type) typ.Type {
		return typ.NewVarSharedType(sub)
	})

	c.addSymLoader("stat", typ.ScriptVarStat)
	c.addSymLoader("locshape", typ.ScriptVarLocShape)
	c.addSymLoader("movespeed", typ.ScriptVarMoveSpeed)
	c.addSymLoader("npc_mode", typ.ScriptVarNpcMode)
	c.addSymLoader("npc_stat", typ.ScriptVarNpcStat)
	c.addSymLoader("model", typ.ScriptVarModel)
	c.addSymLoader("synth", typ.ScriptVarSynth)
	c.addSymLoader("midi", typ.ScriptVarMidi)
	c.addSymLoader("jingle", typ.ScriptVarJingle)

	// TS L168-170: varbit register + protected loader.
	_ = c.TypeManager.Register("varbit", typ.NewVarBitType(typ.MetaAny))
	c.addProtectedSymLoaderWithSupplier("varbit", func(sub typ.Type) typ.Type {
		return typ.NewVarBitType(sub)
	})

	// TS L182-189: enum gated on DisableEnums.
	if !c.Features.DisableEnums {
		c.addSymLoader("enum", typ.ScriptVarEnum)
	}

	// TS L190-194: structs gated on DisableStructs.
	if !c.Features.DisableStructs {
		c.addSymLoader("struct", typ.ScriptVarStruct)
	}

	_ = c.TypeManager.Register("softtimer", typ.NewMetaScript("softtimer", typ.MetaAny, typ.MetaNothing))

	// TS L199-212: dbtables gated on DisableDBTables.
	if !c.Features.DisableDBTables {
		_ = c.TypeManager.Register("dbcolumn", typ.NewDbColumnType(typ.MetaAny))
		c.addSymLoaderWithSupplier("dbcolumn", func(sub typ.Type) typ.Type {
			return typ.NewDbColumnType(sub)
		})
		c.addSymLoader("dbrow", typ.ScriptVarDbRow)
		c.addSymLoader("dbtable", typ.ScriptVarDbTable)
	}

	// Dynamic command handlers — uses T6 feature-gated register.
	command.RegisterAllDynCommands(c.TypeManager, c.Features, func(name string, h semantics.DynamicCommandHandler) {
		c.DynHandlers[name] = h
	})
}

// registerScriptVarTypes iterates typ.ScriptVarTypeAll and registers each,
// skipping enum/struct/dbrow/dbtable per features. Mirrors TS
// ServerScriptCompiler.registerScriptVarTypes L218-225.
func (c *ServerScriptCompiler) registerScriptVarTypes() {
	for _, t := range typ.ScriptVarTypeAll {
		if c.Features.DisableEnums && t == typ.ScriptVarEnum {
			continue
		}
		if c.Features.DisableStructs && t == typ.ScriptVarStruct {
			continue
		}
		if c.Features.DisableDBTables && (t == typ.ScriptVarDbRow || t == typ.ScriptVarDbTable) {
			continue
		}
		_ = c.TypeManager.RegisterByRepresentation(t)
	}
}

func (c *ServerScriptCompiler) addSymLoader(name string, t typ.Type) {
	c.addSymLoaderWithSupplier(name, func(_ typ.Type) typ.Type { return t })
}

func (c *ServerScriptCompiler) addSymLoaderWithSupplier(name string, ts func(typ.Type) typ.Type) {
	info, ok := c.CompilerSymbols[name]
	if !ok {
		return
	}
	c.SymbolLoaders = append(c.SymbolLoaders, &CompilerTypeInfoLoader{
		Mapper:       c.Mapper,
		Symbols:      info,
		TypeSupplier: ts,
	})
}

func (c *ServerScriptCompiler) addProtectedSymLoaderWithSupplier(name string, ts func(typ.Type) typ.Type) {
	info, ok := c.CompilerSymbols[name]
	if !ok {
		return
	}
	c.SymbolLoaders = append(c.SymbolLoaders, &CompilerTypeInfoProtectedLoader{
		Mapper:       c.Mapper,
		Symbols:      info,
		TypeSupplier: ts,
	})
}

func (c *ServerScriptCompiler) addSymConstantLoaders() {
	info, ok := c.CompilerSymbols["constant"]
	if !ok {
		return
	}
	c.SymbolLoaders = append(c.SymbolLoaders, &CompilerTypeInfoConstantLoader{Symbols: info})
}
