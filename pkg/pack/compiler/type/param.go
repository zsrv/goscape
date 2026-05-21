// pkg/pack/compiler/type/param.go
package typ

// paramType wraps an inner Type to represent the RuneScript param type.
// Mirrors TS ParamType.ts (runescript/type/ParamType.ts).
//
// The representation is "param" when inner == MetaAny, otherwise
// "param<inner.Representation()>". BaseType() is always BaseVarInteger,
// DefaultValue() is always -1, and AllowParameter is true.
//
// Distinct from objtype.ParamType (the config-loader struct). This type
// lives in the compiler type system and is used by ParamCommandHandler
// to carry per-param return-type information through type-checking.
type paramType struct {
	inner   Type
	rep     string
	options TypeOptions
}

// NewParamType constructs a paramType wrapping inner. When inner ==
// MetaAny the representation is "param"; otherwise "param<inner>".
//
// inner must not be nil.
func NewParamType(inner Type) Type {
	if inner == nil {
		panic("NewParamType: inner must not be nil")
	}
	rep := "param"
	if inner != MetaAny {
		rep = "param<" + inner.Representation() + ">"
	}
	return &paramType{
		inner: inner,
		rep:   rep,
		options: NewTypeOptions(func(o *TypeOptions) {
			o.AllowSwitch = false
			o.AllowArray = false
			o.AllowDeclaration = false
			o.AllowParameter = true
		}),
	}
}

// IsParamType returns (inner, true) if t is a paramType produced by
// NewParamType; otherwise (nil, false). Mirrors the IsDbColumnType /
// IsMetaHook discriminator pattern used by dynamic command handlers.
func IsParamType(t Type) (inner Type, ok bool) {
	pt, ok := t.(*paramType)
	if !ok {
		return nil, false
	}
	return pt.inner, true
}

func (p *paramType) Representation() string        { return p.rep }
func (p *paramType) Code() (string, bool)          { return "", false }
func (p *paramType) BaseType() (BaseVarType, bool) { return BaseVarInteger, true }
func (p *paramType) DefaultValue() any             { return -1 }
func (p *paramType) Options() TypeOptions          { return p.options }
func (p *paramType) Inner() Type                   { return p.inner }
func (p *paramType) AsTypeRef()                    {}
