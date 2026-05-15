package ast

import "github.com/zsrv/goscape/pkg/pack/compiler/lexer"

// LocalVariableExpression is `$name` or `$name(index)`. Mirrors TS
// LocalVariableExpression (which TS uses for both LocalVariable and
// LocalArrayVariable contexts — distinguished by Index nil/non-nil).
//
// NAI-204-D-AST-NO-TYPE-FIELDS: TS .reference is NAI-205-owned.
type LocalVariableExpression struct {
	SrcLoc lexer.NodeSourceLocation
	Name   *Identifier
	Index  Expression // nil if plain $name
}

func (v *LocalVariableExpression) Source() lexer.NodeSourceLocation { return v.SrcLoc }
func (v *LocalVariableExpression) Kind() NodeKind                   { return KindLocalVariableExpression }
func (v *LocalVariableExpression) Children() []Node {
	out := make([]Node, 0, 2)
	if v.Name != nil {
		out = append(out, v.Name)
	}
	if v.Index != nil {
		out = append(out, v.Index)
	}
	return out
}
func (v *LocalVariableExpression) isNode()               {}
func (v *LocalVariableExpression) isExpression()         {}
func (v *LocalVariableExpression) isVariableExpression() {}

// IsArray reports whether this is the `$name(index)` form.
func (v *LocalVariableExpression) IsArray() bool { return v.Index != nil }

// GameVariableExpression is `%name` or `.%name`. Dot==true for `.%`.
// Mirrors TS GameVariableExpression.
type GameVariableExpression struct {
	SrcLoc lexer.NodeSourceLocation
	Dot    bool
	Name   *Identifier
}

func (v *GameVariableExpression) Source() lexer.NodeSourceLocation { return v.SrcLoc }
func (v *GameVariableExpression) Kind() NodeKind                   { return KindGameVariableExpression }
func (v *GameVariableExpression) Children() []Node {
	if v.Name == nil {
		return nil
	}
	return []Node{v.Name}
}
func (v *GameVariableExpression) isNode()               {}
func (v *GameVariableExpression) isExpression()         {}
func (v *GameVariableExpression) isVariableExpression() {}

// ConstantVariableExpression is `^name`. Mirrors TS ConstantVariableExpression.
//
// NAI-204-D-AST-NO-TYPE-FIELDS: TS .subExpression is NAI-205-owned.
type ConstantVariableExpression struct {
	SrcLoc lexer.NodeSourceLocation
	Name   *Identifier
}

func (v *ConstantVariableExpression) Source() lexer.NodeSourceLocation { return v.SrcLoc }
func (v *ConstantVariableExpression) Kind() NodeKind {
	return KindConstantVariableExpression
}
func (v *ConstantVariableExpression) Children() []Node {
	if v.Name == nil {
		return nil
	}
	return []Node{v.Name}
}
func (v *ConstantVariableExpression) isNode()               {}
func (v *ConstantVariableExpression) isExpression()         {}
func (v *ConstantVariableExpression) isVariableExpression() {}
