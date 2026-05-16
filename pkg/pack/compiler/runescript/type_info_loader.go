// pkg/pack/compiler/runescript/type_info_loader.go
package runescript

import (
	"slices"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// CompilerTypeInfoConstantLoader inserts every Map entry as a ConstantSymbol.
// Mirrors TS src/runescript/CompilerTypeInfoConstantLoader.ts.
type CompilerTypeInfoConstantLoader struct {
	Symbols *CompilerTypeInfo
}

// Load iterates Symbols.Map (sorted by key for byte-identical reproducibility;
// see NAI-210-D-LOADER-SORTED-ITERATION) and calls symbol.AddConstant per entry.
func (l *CompilerTypeInfoConstantLoader) Load(table *symbol.SymbolTable, c symbol.CompilerContext) error {
	keys := sortedStringKeys(l.Symbols.Map)
	for _, key := range keys {
		value := l.Symbols.Map[key]
		if _, err := symbol.AddConstant(table, key, value); err != nil {
			return err
		}
	}
	return nil
}

// CompilerTypeInfoLoader registers BasicSymbols with the supplied type and
// updates SymbolMapper. Mirrors TS src/runescript/CompilerTypeInfoLoader.ts.
type CompilerTypeInfoLoader struct {
	Mapper       *SymbolMapper
	Symbols      *CompilerTypeInfo
	TypeSupplier func(subType typ.Type) typ.Type
}

// Load iterates Symbols.Map sorted by numeric id
// (NAI-210-D-LOADER-SORTED-ITERATION) for byte-identical SymbolMapper output.
func (l *CompilerTypeInfoLoader) Load(table *symbol.SymbolTable, c symbol.CompilerContext) error {
	keys := sortedNumericKeys(l.Symbols.Map)
	for _, key := range keys {
		name := l.Symbols.Map[key]
		id, err := strconv.Atoi(key)
		if err != nil {
			return err
		}
		subTypes := resolveVartype(l.Symbols.Vartype[key], c)
		t := l.TypeSupplier(subTypes)
		sym, err := symbol.AddBasic(table, t, name, false)
		if err != nil {
			return err
		}
		l.Mapper.PutSymbol(id, sym)
	}
	return nil
}

// CompilerTypeInfoProtectedLoader is identical to CompilerTypeInfoLoader except
// it propagates the Protect[key] flag to the inserted BasicSymbol.
// Mirrors TS src/runescript/CompilerTypeInfoProtectedLoader.ts.
type CompilerTypeInfoProtectedLoader struct {
	Mapper       *SymbolMapper
	Symbols      *CompilerTypeInfo
	TypeSupplier func(subType typ.Type) typ.Type
}

// Load iterates Symbols.Map sorted by numeric id
// (NAI-210-D-LOADER-SORTED-ITERATION) for byte-identical SymbolMapper output.
func (l *CompilerTypeInfoProtectedLoader) Load(table *symbol.SymbolTable, c symbol.CompilerContext) error {
	keys := sortedNumericKeys(l.Symbols.Map)
	for _, key := range keys {
		name := l.Symbols.Map[key]
		id, err := strconv.Atoi(key)
		if err != nil {
			return err
		}
		subTypes := resolveVartype(l.Symbols.Vartype[key], c)
		isProtected := l.Symbols.Protect[key]
		t := l.TypeSupplier(subTypes)
		sym, err := symbol.AddBasic(table, t, name, isProtected)
		if err != nil {
			return err
		}
		l.Mapper.PutSymbol(id, sym)
	}
	return nil
}

// resolveVartype handles TS L19-24: comma-separated type names become a
// TupleType (via typ.TupleFromList which collapses 0→MetaUnit, 1→element,
// 2+→TupleType). Missing/empty vartype → MetaUnit. Unknown name → MetaError.
//
// allowArray=false on FindOrNil: the TS vartype tokens are bare type names,
// never with an "array" suffix.
func resolveVartype(vartype string, c symbol.CompilerContext) typ.Type {
	if vartype == "" {
		return typ.MetaUnit
	}
	parts := strings.Split(vartype, ",")
	children := make([]typ.Type, len(parts))
	for i, tn := range parts {
		t := c.Types().FindOrNil(tn, false)
		if t == nil {
			t = typ.MetaError
		}
		children[i] = t
	}
	return typ.TupleFromList(children)
}

// sortedStringKeys returns keys in lex order. Used for the constant loader
// where keys are symbol names rather than numeric ids.
func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// sortedNumericKeys parses each key as int and sorts ascending. Non-numeric
// keys sort after all numeric keys in lex order; this matches TS V8's
// Object.entries deterministic ordering for integer-shaped string keys.
// NAI-210-D-LOADER-SORTED-ITERATION.
func sortedNumericKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int {
		ai, errA := strconv.Atoi(a)
		bi, errB := strconv.Atoi(b)
		switch {
		case errA == nil && errB == nil:
			return ai - bi
		case errA == nil:
			return -1
		case errB == nil:
			return 1
		default:
			return strings.Compare(a, b)
		}
	})
	return keys
}
