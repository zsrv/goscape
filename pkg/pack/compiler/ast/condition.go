package ast

import "github.com/zsrv/goscape/pkg/pack/compiler/lexer"

// ConditionExpression is a binary condition op inside `if`/`while`.
// Operators per grammar: LT GT LTE GTE EQ EXCL AND OR. Mirrors TS
// ConditionExpression (extends BinaryExpression).
type ConditionExpression struct {
	SrcLoc   lexer.NodeSourceLocation
	Left     Expression
	Operator *Token
	Right    Expression
}

func (c *ConditionExpression) Source() lexer.NodeSourceLocation { return c.SrcLoc }
func (c *ConditionExpression) Kind() NodeKind                   { return KindConditionExpression }
func (c *ConditionExpression) Children() []Node {
	out := make([]Node, 0, 3)
	if c.Left != nil {
		out = append(out, c.Left)
	}
	if c.Operator != nil {
		out = append(out, c.Operator)
	}
	if c.Right != nil {
		out = append(out, c.Right)
	}
	return out
}
func (c *ConditionExpression) isNode()       {}
func (c *ConditionExpression) isExpression() {}
