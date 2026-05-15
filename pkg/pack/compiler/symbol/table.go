// pkg/pack/compiler/symbol/table.go
package symbol

import (
	"strings"
	"unicode"
)

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

// normalize applies the Basic-kind name normalisation rule used by Insert/Find.
// Lowercases ASCII letters; collapses runs of Unicode-whitespace to a single
// '_'. Non-Basic kinds pass through unchanged.
//
// NAI-205-D-NORMALIZE-UNICODE-SUBSET: TS uses `name.toLowerCase().replace(/\s+/g, '_')`.
// The JS `\s` class covers 21 code points including U+FEFF (BOM). Goscape uses
// unicode.IsSpace which covers 20 — same 6 ASCII whitespace chars TS handles plus
// NEL (U+0085), NBSP (U+00A0), and all category Z separators (U+1680, U+2000-200A,
// U+2028, U+2029, U+202F, U+205F, U+3000). The only TS-side whitespace goscape
// doesn't cover is U+FEFF (zero-width no-break space / BOM), which is never present
// in RuneScript symbol names in practice. Documented as a deviation rather than fixed
// because adding a single-char carve-out for U+FEFF is more code than it's worth.
//
// `toLowerCase` is ASCII-only here (TS would also lowercase non-ASCII letters per
// Unicode rules); RuneScript symbol names are ASCII-only in practice. The lowerASCII
// helper in primitive.go documents the same simplification.
func (st *SymbolTable) normalize(kind SymbolKind, name string) string {
	if kind != SymbolKindBasic {
		return name
	}
	var b strings.Builder
	b.Grow(len(name))
	prevSpace := false
	for _, r := range name {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteRune('_')
			}
			prevSpace = true
			continue
		}
		prevSpace = false
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		b.WriteRune(r)
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

// FindAll returns every Symbol matching name across all symbol-type kinds,
// walking the parent chain. Mirrors TS SymbolTable.findAll (L65-71) which
// collects from findAllIter without a kind filter.
//
// Name normalisation is applied per-kind via the same rules as Find — a Basic
// "Wooden Bowl" symbol matches a "wooden_bowl" query; ServerScript names are
// case-sensitive.
//
// The result slice is fresh; mutating it does not affect the table.
func (st *SymbolTable) FindAll(name string) []Symbol {
	var out []Symbol
	st.findAllInto(name, &out)
	return out
}

// findAllInto walks the parent chain accumulating into out.
func (st *SymbolTable) findAllInto(name string, out *[]Symbol) {
	for outerKey, inner := range st.symbols {
		// Reverse-derive the kind from the outer key prefix to pick the right
		// normalisation rule. Avoids constructing a SymbolType per-pair.
		kind := keyToKind(outerKey)
		nk := st.normalize(kind, name)
		if s, ok := inner[nk]; ok {
			*out = append(*out, s)
		}
	}
	if st.parent != nil {
		st.parent.findAllInto(name, out)
	}
}

// keyToKind maps the Key() prefix back to a SymbolKind for the purpose of
// driving normalize(). Only Basic vs non-Basic actually matters; the function
// returns SymbolKindBasic for "basic:*" keys and any other kind otherwise.
func keyToKind(outerKey string) SymbolKind {
	if len(outerKey) >= 6 && outerKey[:6] == "basic:" {
		return SymbolKindBasic
	}
	// Any non-Basic kind suffices; pick LocalVariable.
	return SymbolKindLocalVariable
}

// AsSymbolTableRef satisfies ast.SymbolTableRef.
func (*SymbolTable) AsSymbolTableRef() {}
