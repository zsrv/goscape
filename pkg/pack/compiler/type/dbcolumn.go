// pkg/pack/compiler/type/dbcolumn.go
package typ

// dbColumnType wraps an inner Type to represent a database column type.
// Mirrors TS DbColumnType.ts — used by db_getfield / db_find dynamic command
// handlers to carry per-column type information through the type-checker.
//
// The outer type's representation is "dbcolumn<inner.representation>".
// BaseType() always returns BaseVarInteger and DefaultValue() always returns -1
// (both hardcoded in TS L14-15). Code() always returns ("", false) (TS code
// field is never assigned). AllowParameter is true (TS L24).
//
// inner must not be nil; NewDbColumnType panics if it is.
type dbColumnType struct {
	inner   Type
	rep     string
	options TypeOptions
}

// NewDbColumnType constructs a dbColumnType wrapping inner. The representation
// is "dbcolumn<inner.Representation()>". Mirrors NAI-205's metaHook/metaScript
// constructor pattern.
//
// inner must not be nil.
func NewDbColumnType(inner Type) Type {
	if inner == nil {
		panic("NewDbColumnType: inner must not be nil")
	}
	return &dbColumnType{
		inner: inner,
		rep:   "dbcolumn<" + inner.Representation() + ">",
		options: NewTypeOptions(func(o *TypeOptions) {
			o.AllowSwitch = false
			o.AllowArray = false
			o.AllowDeclaration = false
			o.AllowParameter = true
		}),
	}
}

// IsDbColumnType returns (inner, true) if t is a dbColumnType produced by
// NewDbColumnType; otherwise (nil, false). Mirrors IsMetaHook / IsMetaScript
// discriminator pattern.
func IsDbColumnType(t Type) (inner Type, ok bool) {
	dc, ok := t.(*dbColumnType)
	if !ok {
		return nil, false
	}
	return dc.inner, true
}

func (d *dbColumnType) Representation() string { return d.rep }

// Code returns ("", false). TS DbColumnType.code is declared but never assigned,
// so it is always undefined.
func (d *dbColumnType) Code() (string, bool) { return "", false }

// BaseType returns (BaseVarInteger, true) always. TS DbColumnType.baseType is
// hardcoded to BaseVarType.INTEGER (L14) — it does NOT delegate to inner.
func (d *dbColumnType) BaseType() (BaseVarType, bool) { return BaseVarInteger, true }

// DefaultValue returns -1 always. TS DbColumnType.defaultValue is hardcoded to
// -1 (L15) — it does NOT delegate to inner.
func (d *dbColumnType) DefaultValue() any { return -1 }

func (d *dbColumnType) Options() TypeOptions { return d.options }
func (d *dbColumnType) Inner() Type           { return d.inner }
func (d *dbColumnType) AsTypeRef()            {}
