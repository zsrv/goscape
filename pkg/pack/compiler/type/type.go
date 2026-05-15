// pkg/pack/compiler/type/type.go
package typ

// Type is the goscape port of TS abstract class Type.
//
// All five fields TS declares as `readonly` are exposed as zero-arg accessors:
//   - Representation() string  — always present
//   - Code() (string, bool)    — TS optional; second return is the presence bit
//   - BaseType() (BaseVarType, bool) — TS optional
//   - DefaultValue() any       — TS optional; returns nil when absent
//   - Options() TypeOptions    — always present
//
// Concrete implementations: PrimitiveType, TupleType, MetaType (and the
// MetaType-Wrapped/Script variants), ArrayType, VarPlayerType, VarBitType,
// VarNpcType, VarSharedType.
//
// Every concrete Type must also satisfy ast.TypeRef via an AsTypeRef() method
// (see NAI-205-D-AST-REF-INTERFACES in pkg/pack/compiler/ast/symbol_refs.go).
type Type interface {
	Representation() string
	Code() (string, bool)
	BaseType() (BaseVarType, bool)
	DefaultValue() any
	Options() TypeOptions

	// AsTypeRef satisfies ast.TypeRef. Embedding this method in the Type
	// interface ensures every concrete Type can be assigned to ast.TypeRef
	// without consumers needing to re-assert.
	AsTypeRef()
}
