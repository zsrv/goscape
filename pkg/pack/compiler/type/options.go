// pkg/pack/compiler/type/options.go
package typ

// TypeOptions controls which uses of a Type the compiler accepts.
// Mirrors TS src/compiler/type/TypeOptions.ts L4-37.
//
// NAI-205-D-TYPEOPTIONS-FLAT: TS exports both a readonly interface
// (TypeOptions) and a mutable subclass (MutableOptionsType). Goscape
// collapses to one mutable struct + a builder-fn convention since the
// readonly-vs-mutable distinction has no Go-idiomatic counterpart.
type TypeOptions struct {
	AllowSwitch      bool
	AllowArray       bool
	AllowDeclaration bool
	AllowParameter   bool
}

// NewTypeOptions returns a TypeOptions with all four flags true,
// optionally adjusted by builders called in order.
// Mirrors TS MutableOptionsType ctor + Object.assign(init).
func NewTypeOptions(builders ...func(*TypeOptions)) TypeOptions {
	o := TypeOptions{
		AllowSwitch:      true,
		AllowArray:       true,
		AllowDeclaration: true,
		AllowParameter:   true,
	}
	for _, b := range builders {
		b(&o)
	}
	return o
}
