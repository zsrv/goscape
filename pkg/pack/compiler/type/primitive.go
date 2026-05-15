// pkg/pack/compiler/type/primitive.go
package typ

// PrimitiveType represents one of the seven main RuneScript primitive types.
// Mirrors TS src/compiler/type/PrimitiveType.ts.
//
// All seven singletons are package-level vars; the constructor is unexported.
type PrimitiveType struct {
	rep      string
	code     string // "" means "no code" — see Code().
	codeOK   bool
	baseType BaseVarType
	dv       any
	options  TypeOptions
}

func newPrimitiveType(name, code string, base BaseVarType, dv any, builders ...func(*TypeOptions)) *PrimitiveType {
	return &PrimitiveType{
		rep:      lowerASCII(name),
		code:     code,
		codeOK:   code != "",
		baseType: base,
		dv:       dv,
		options:  NewTypeOptions(builders...),
	}
}

func (p *PrimitiveType) Representation() string        { return p.rep }
func (p *PrimitiveType) Code() (string, bool)          { return p.code, p.codeOK }
func (p *PrimitiveType) BaseType() (BaseVarType, bool) { return p.baseType, true }
func (p *PrimitiveType) DefaultValue() any             { return p.dv }
func (p *PrimitiveType) Options() TypeOptions          { return p.options }
func (p *PrimitiveType) AsTypeRef()                    {}

// Singletons. Names + codes + baseType + defaultValue match TS L40-46.
var (
	PrimitiveInt     = newPrimitiveType("INT", "i", BaseVarInteger, 0)
	PrimitiveBoolean = newPrimitiveType("BOOLEAN", "1", BaseVarInteger, 0)
	PrimitiveCoord   = newPrimitiveType("COORD", "c", BaseVarInteger, -1)
	PrimitiveString  = newPrimitiveType("STRING", "s", BaseVarString, "", func(o *TypeOptions) {
		o.AllowArray = false
		o.AllowSwitch = false
	})
	PrimitiveChar    = newPrimitiveType("CHAR", "z", BaseVarInteger, -1)
	PrimitiveLong    = newPrimitiveType("LONG", "Ï", BaseVarLong, -1, func(o *TypeOptions) {
		o.AllowArray = false
		o.AllowSwitch = false
	})
	PrimitiveMapzone = newPrimitiveType("MAPZONE", "0", BaseVarInteger, -1)
)

// PrimitiveAll preserves TS L57 ordering. Used for round-trip / table-driven tests.
var PrimitiveAll = []*PrimitiveType{
	PrimitiveInt, PrimitiveBoolean, PrimitiveCoord, PrimitiveString,
	PrimitiveChar, PrimitiveLong, PrimitiveMapzone,
}

// PrimitiveByRepresentation returns the matching singleton or nil. Mirrors TS L59-61.
func PrimitiveByRepresentation(rep string) *PrimitiveType {
	for _, p := range PrimitiveAll {
		if p.rep == rep {
			return p
		}
	}
	return nil
}

// lowerASCII is the package-local lowercaser for type names. TS uses
// `.toLowerCase()` on type names — all primitive names are ASCII so a
// simple byte-loop suffices and avoids a Unicode dep.
func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
