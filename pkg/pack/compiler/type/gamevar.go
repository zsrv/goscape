// pkg/pack/compiler/type/gamevar.go
package typ

// GameVarType-family: four shapes that wrap an inner Type. Mirrors
// TS wrapped/GameVarType.ts.
//
// All four share field shape (inner, rep, options) — defined once via gameVarBase.
type gameVarBase struct {
	inner   Type
	rep     string
	options TypeOptions
}

func newGameVarOptions() TypeOptions {
	return NewTypeOptions(func(o *TypeOptions) {
		o.AllowSwitch = false
		o.AllowArray = false
		o.AllowDeclaration = false
		o.AllowParameter = false
	})
}

type VarPlayerType struct{ gameVarBase }
type VarBitType struct{ gameVarBase }
type VarNpcType struct{ gameVarBase }
type VarSharedType struct{ gameVarBase }

func NewVarPlayerType(inner Type) *VarPlayerType {
	return &VarPlayerType{gameVarBase{inner, "varp<" + inner.Representation() + ">", newGameVarOptions()}}
}

func NewVarBitType(inner Type) *VarBitType {
	return &VarBitType{gameVarBase{inner, "varbit<" + inner.Representation() + ">", newGameVarOptions()}}
}

func NewVarNpcType(inner Type) *VarNpcType {
	return &VarNpcType{gameVarBase{inner, "varn<" + inner.Representation() + ">", newGameVarOptions()}}
}

func NewVarSharedType(inner Type) *VarSharedType {
	return &VarSharedType{gameVarBase{inner, "vars<" + inner.Representation() + ">", newGameVarOptions()}}
}

// Type-interface methods on gameVarBase. Concrete-type method-sets pick these up
// via Go struct embedding. All four sub-types share these implementations.
func (g gameVarBase) Representation() string        { return g.rep }
func (g gameVarBase) Code() (string, bool)          { return "", false }
func (g gameVarBase) BaseType() (BaseVarType, bool) { return BaseVarInteger, true }
func (g gameVarBase) DefaultValue() any             { return -1 }
func (g gameVarBase) Options() TypeOptions          { return g.options }
func (g gameVarBase) Inner() Type                   { return g.inner }
func (g gameVarBase) AsTypeRef()                    {}
