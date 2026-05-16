// pkg/pack/compiler/symbol/loader.go
package symbol

import (
	"fmt"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// CompilerContext is the narrow interface SymbolLoader implementations need.
// Defined here (not in runescript/) to break the import cycle that storing
// a *runescript.ServerScriptCompiler in SymbolLoader.Load would create.
//
// TS `SymbolLoader.load(table, compiler: ScriptCompiler)` references
// `compiler.types`; goscape exposes the TypeManager via this getter so
// loaders can call FindOrNil / Find etc. directly.
type CompilerContext interface {
	Types() *typ.TypeManager
}

// SymbolLoader is the contract for pre-compilation external-symbol loading.
// Mirrors TS abstract class SymbolLoader at
// src/compiler/configuration/SymbolLoader.ts.
type SymbolLoader interface {
	Load(table *SymbolTable, compiler CompilerContext) error
}

// AddConstant inserts a ConstantSymbol with the given name + value. Returns
// the inserted symbol or an error if Insert returned false (TS throws).
// Mirrors TS SymbolLoader.addConstant L26-32.
func AddConstant(table *SymbolTable, name, value string) (*ConstantSymbol, error) {
	s := &ConstantSymbol{Name: name, Value: value}
	if !table.Insert(SymbolTypeConstant(), s) {
		return nil, fmt.Errorf("unable to add constant: name=%s, value=%s", name, value)
	}
	return s, nil
}

// AddBasic inserts a BasicSymbol with the given type, name, and protected
// flag. Returns the inserted symbol or an error if Insert returned false.
// Mirrors TS SymbolLoader.addBasic L38-47.
func AddBasic(table *SymbolTable, t typ.Type, name string, isProtected bool) (*BasicSymbol, error) {
	s := &BasicSymbol{Name: name, Type: t, IsProtected: isProtected}
	if !table.Insert(SymbolTypeBasic(t), s) {
		return nil, fmt.Errorf("unable to add basic: type=%v, name=%s", t, name)
	}
	return s, nil
}
