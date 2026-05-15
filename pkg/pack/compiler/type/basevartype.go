// pkg/pack/compiler/type/basevartype.go
package typ

// BaseVarType enumerates the three low-level storage classes for any Type.
// Mirrors TS src/compiler/type/BaseVarType.ts.
type BaseVarType int

const (
	BaseVarInteger BaseVarType = 0
	BaseVarLong    BaseVarType = 1
	BaseVarString  BaseVarType = 2
)
