// pkg/pack/compiler/type/array.go
package typ

import "errors"

// ArrayType wraps another Type. Mirrors TS wrapped/ArrayType.ts.
type ArrayType struct {
	inner   Type
	options TypeOptions
}

// NewArrayType wraps inner. Errors if inner is itself an ArrayType.
// Mirrors TS L20 throw.
func NewArrayType(inner Type) (*ArrayType, error) {
	if _, nested := inner.(*ArrayType); nested {
		return nil, errors.New("ArrayType cannot wrap another ArrayType")
	}
	return &ArrayType{
		inner: inner,
		options: NewTypeOptions(func(o *TypeOptions) {
			o.AllowArray = false
			o.AllowDeclaration = true
			o.AllowSwitch = true
			o.AllowParameter = true
		}),
	}, nil
}

func (a *ArrayType) Inner() Type                   { return a.inner }
func (a *ArrayType) Representation() string        { return a.inner.Representation() + "array" }
func (a *ArrayType) Code() (string, bool)          { return "", false }
func (a *ArrayType) BaseType() (BaseVarType, bool) { return BaseVarInteger, true }
func (a *ArrayType) DefaultValue() any             { return nil }
func (a *ArrayType) Options() TypeOptions          { return a.options }
func (a *ArrayType) AsTypeRef()                    {}
