// pkg/pack/compiler/type/tuple.go
package typ

import (
	"errors"
	"strings"
)

// TupleType combines multiple Types into one. Mirrors TS TupleType.ts.
// Children is a flattened list (never contains a nested TupleType).
type TupleType struct {
	Children []Type
	rep      string
	options  TypeOptions
}

// NewTupleType returns a *TupleType wrapping the given child Types.
// Nested TupleType arguments are flattened. Errors when fewer than 2
// children remain after flattening (TS throws — TupleType.ts L23).
func NewTupleType(children ...Type) (*TupleType, error) {
	flat := flattenTuples(children)
	if len(flat) < 2 {
		return nil, errors.New("TupleType requires at least 2 children")
	}
	reps := make([]string, len(flat))
	for i, c := range flat {
		reps[i] = c.Representation()
	}
	return &TupleType{
		Children: flat,
		rep:      strings.Join(reps, ","),
		options: NewTypeOptions(func(o *TypeOptions) {
			o.AllowSwitch = false
			o.AllowArray = false
			o.AllowDeclaration = false
			o.AllowParameter = false
		}),
	}, nil
}

func flattenTuples(children []Type) []Type {
	var out []Type
	for _, c := range children {
		if t, ok := c.(*TupleType); ok {
			out = append(out, t.Children...)
		} else {
			out = append(out, c)
		}
	}
	return out
}

func (t *TupleType) Representation() string        { return t.rep }
func (t *TupleType) Code() (string, bool)          { return "", false }
func (t *TupleType) BaseType() (BaseVarType, bool) { return 0, false }
func (t *TupleType) DefaultValue() any             { return nil }
func (t *TupleType) Options() TypeOptions          { return t.options }
func (t *TupleType) AsTypeRef()                    {}

// TupleFromList collapses a []Type into either MetaUnit, the single
// element, or a TupleType. Mirrors TS TupleType.fromList (L42-49).
func TupleFromList(types []Type) Type {
	if len(types) == 0 {
		return MetaUnit
	}
	if len(types) == 1 {
		return types[0]
	}
	t, err := NewTupleType(types...)
	if err != nil {
		// TS treats this as unreachable since len >= 2; goscape returns
		// MetaError as a safety net so callers don't have to error-check.
		return MetaError
	}
	return t
}

// TupleToList inverts TupleFromList. Mirrors TS TupleType.toList (L57-66).
func TupleToList(t Type) []Type {
	if t == nil || t == MetaUnit || t == MetaNothing {
		return nil
	}
	if tup, ok := t.(*TupleType); ok {
		return tup.Children
	}
	return []Type{t}
}
