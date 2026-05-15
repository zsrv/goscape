// pkg/pack/compiler/type/wrapped.go
package typ

// WrappedType is implemented by every Type that wraps an inner Type.
// Mirrors TS src/compiler/type/wrapped/WrappedType.ts.
// Both ArrayType and the four GameVarType variants implement WrappedType.
type WrappedType interface {
	Type
	Inner() Type
}
