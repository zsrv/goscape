package ast

// ExpressionBase is the shared mixin embedded by every concrete
// Expression-implementing type. Carries Type (the resolved type, set
// during type checking) and TypeHint (the expected type, propagated
// top-down during type checking).
//
// NAI-206-D-EXPR-BASE: TS Expression is an abstract superclass holding
// these two fields. Goscape lacks a shared base struct; embedding a
// mixin gives field promotion (e.g. `node.Type`, `node.TypeHint`)
// without forcing a runtime-polymorphic getter. Field type is TypeRef
// (the cyclic-import marker from symbol_refs.go) so concrete
// pkg/pack/compiler/type values satisfy it without ast importing the
// type package.
type ExpressionBase struct {
	Type     TypeRef
	TypeHint TypeRef
}
