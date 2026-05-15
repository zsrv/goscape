// pkg/pack/compiler/symbol/table.go
package symbol

import "strings"

// SymbolTable is the goscape port of TS class SymbolTable.
//
// Outer map is keyed by SymbolType.Key(); inner map is keyed by
// normalise(kind, name). Normalisation lowercases + collapses whitespace
// only for Kind == Basic; all other kinds preserve the original name.
type SymbolTable struct {
	parent  *SymbolTable
	symbols map[string]map[string]Symbol
}

// NewSymbolTable returns a fresh SymbolTable optionally chained to parent.
// Mirrors TS constructor.
func NewSymbolTable(parent *SymbolTable) *SymbolTable {
	return &SymbolTable{parent: parent, symbols: map[string]map[string]Symbol{}}
}

// CreateSubTable returns a SymbolTable whose parent is this table.
// Mirrors TS createSubTable() (TS L93-95).
func (st *SymbolTable) CreateSubTable() *SymbolTable {
	return NewSymbolTable(st)
}

func (st *SymbolTable) normalize(kind SymbolKind, name string) string {
	if kind != SymbolKindBasic {
		return name
	}
	// lowercase + collapse any run of whitespace to a single underscore.
	var b strings.Builder
	b.Grow(len(name))
	prevSpace := false
	for _, r := range name {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f':
			if !prevSpace {
				b.WriteRune('_')
			}
			prevSpace = true
		default:
			prevSpace = false
			if r >= 'A' && r <= 'Z' {
				r += 'a' - 'A'
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Insert returns true iff the symbol was inserted. Returns false if the
// symbol already exists in this table OR any parent table. Mirrors TS
// SymbolTable.insert L28-46.
func (st *SymbolTable) Insert(t SymbolType, s Symbol) bool {
	key := st.normalize(t.Kind, s.SymbolName())
	outerKey := t.Key()

	// Walk the parent chain checking for collisions.
	for cur := st; cur != nil; cur = cur.parent {
		if inner, ok := cur.symbols[outerKey]; ok {
			if _, exists := inner[key]; exists {
				return false
			}
		}
	}

	inner, ok := st.symbols[outerKey]
	if !ok {
		inner = map[string]Symbol{}
		st.symbols[outerKey] = inner
	}
	inner[key] = s
	return true
}

// Find returns the symbol matching (t, name), walking the parent chain.
// Mirrors TS SymbolTable.find L51-58.
func (st *SymbolTable) Find(t SymbolType, name string) Symbol {
	outerKey := t.Key()
	key := st.normalize(t.Kind, name)
	for cur := st; cur != nil; cur = cur.parent {
		if inner, ok := cur.symbols[outerKey]; ok {
			if s, exists := inner[key]; exists {
				return s
			}
		}
	}
	return nil
}

// AsSymbolTableRef satisfies ast.SymbolTableRef.
func (*SymbolTable) AsSymbolTableRef() {}
