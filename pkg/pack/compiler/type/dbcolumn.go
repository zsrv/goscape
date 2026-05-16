// pkg/pack/compiler/type/dbcolumn.go
package typ

// dbColumnType wraps an inner Type to represent a database column type.
// Mirrors TS DbColumnType.ts — used by db_getfield / db_find dynamic command
// handlers to carry per-column type information through the type-checker.
//
// The outer type's representation is "dbcolumn<inner.representation>".
// All TypeOptions flags remain false (same as metaHook/metaWrapping).
// BaseType() and DefaultValue() delegate to the inner type so that column
// types participate in base-type-routing (NAI-207 codegen).
type dbColumnType struct {
	inner   Type
	rep     string
	options TypeOptions
}

// NewDbColumnType constructs a dbColumnType wrapping inner. The representation
// is "dbcolumn<inner.Representation()>". Mirrors NAI-205's metaHook/metaScript
// constructor pattern.
func NewDbColumnType(inner Type) Type {
	return &dbColumnType{
		inner: inner,
		rep:   "dbcolumn<" + inner.Representation() + ">",
		options: NewTypeOptions(func(o *TypeOptions) {
			o.AllowSwitch = false
			o.AllowArray = false
			o.AllowDeclaration = false
			o.AllowParameter = false
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

// Code delegates to the inner type. dbcolumn columns carry the same code
// character as their underlying primitive.
func (d *dbColumnType) Code() (string, bool) { return d.inner.Code() }

// BaseType delegates to the inner type so codegen can route to int/long/string
// slots based on the underlying storage class.
func (d *dbColumnType) BaseType() (BaseVarType, bool) { return d.inner.BaseType() }

// DefaultValue delegates to the inner type.
func (d *dbColumnType) DefaultValue() any { return d.inner.DefaultValue() }

func (d *dbColumnType) Options() TypeOptions { return d.options }
func (d *dbColumnType) AsTypeRef()           {}
