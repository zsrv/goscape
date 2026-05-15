// pkg/pack/compiler/symbol/symbol.go
package symbol

import (
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// Symbol is the marker interface for every kind of symbol the compiler
// tracks. Mirrors TS RuneScriptSymbol interface.
type Symbol interface {
	SymbolName() string
	AsSymbolRef() // ast.SymbolRef satisfaction
}

// LocalVariableSymbol represents a script-local variable / parameter.
// Mirrors TS LocalVariableSymbol.
type LocalVariableSymbol struct {
	Name string
	Type typ.Type
}

func (s *LocalVariableSymbol) SymbolName() string { return s.Name }
func (*LocalVariableSymbol) AsSymbolRef()         {}

// BasicSymbol represents a top-level named object (npc / loc / obj / etc.).
// IsProtected gates write access in TypeChecking. Mirrors TS BasicSymbol.
type BasicSymbol struct {
	Name        string
	Type        typ.Type
	IsProtected bool
}

func (s *BasicSymbol) SymbolName() string { return s.Name }
func (*BasicSymbol) AsSymbolRef()         {}

// ConstantSymbol represents a `^FOO = value` constant. Mirrors TS ConstantSymbol.
type ConstantSymbol struct {
	Name  string
	Value string
}

func (s *ConstantSymbol) SymbolName() string { return s.Name }
func (*ConstantSymbol) AsSymbolRef()         {}
