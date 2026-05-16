// pkg/pack/compiler/runescript/server_script_compiler.go
package runescript

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// ServerScriptCompiler is the goscape port of TS ScriptCompiler +
// ServerScriptCompiler (single struct, no inheritance). Mirrors TS
// src/runescript/ServerScriptCompiler.ts.
//
// Setup() installs default type checkers + server triggers + script var
// types + dynamic command handlers + sym loaders; Run(ext) orchestrates
// parse → analyze → codegen → check-pointers → write. Both land in T13/T14.
type ServerScriptCompiler struct {
	SourcePaths  []string
	ExcludePaths []string

	TypeManager     *typ.TypeManager
	Triggers        *trigger.TriggerManager
	RootTable       *symbol.SymbolTable
	DynHandlers     map[string]semantics.DynamicCommandHandler
	SymbolLoaders   []symbol.SymbolLoader
	CompilerSymbols map[string]*CompilerTypeInfo
	Mapper          *SymbolMapper
	CommandPointers map[string]*pointer.PointerHolder
	Features        semantics.StrictFeatureLevel

	DiagHandler *diagnostics.Diagnostics

	BinaryWriter *BinaryScriptWriter
	Writer       BinaryOutput
}

// Types satisfies symbol.CompilerContext: returns the underlying TypeManager
// so symbol loaders (T10/T11) can perform type lookups during Load().
func (c *ServerScriptCompiler) Types() *typ.TypeManager {
	return c.TypeManager
}

// Compile-time assertion that *ServerScriptCompiler satisfies the loader
// callback contract.
var _ symbol.CompilerContext = (*ServerScriptCompiler)(nil)
