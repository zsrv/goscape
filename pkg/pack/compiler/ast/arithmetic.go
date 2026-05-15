package ast

import "github.com/zsrv/goscape/pkg/pack/compiler/lexer"

// ArithmeticExpression is a binary arithmetic op inside `calc(...)`.
// Operators per grammar: MUL DIV MOD PLUS MINUS AND OR. Mirrors TS
// ArithmeticExpression (extends BinaryExpression).
type ArithmeticExpression struct {
	SrcLoc   lexer.NodeSourceLocation
	Left     Expression
	Operator *Token
	Right    Expression
	ExpressionBase
}

func (a *ArithmeticExpression) Source() lexer.NodeSourceLocation { return a.SrcLoc }
func (a *ArithmeticExpression) Kind() NodeKind                   { return KindArithmeticExpression }
func (a *ArithmeticExpression) Children() []Node {
	out := make([]Node, 0, 3)
	if a.Left != nil {
		out = append(out, a.Left)
	}
	if a.Operator != nil {
		out = append(out, a.Operator)
	}
	if a.Right != nil {
		out = append(out, a.Right)
	}
	return out
}
func (a *ArithmeticExpression) isNode()       {}
func (a *ArithmeticExpression) isExpression() {}

// CalcExpression is `calc(arithmetic)`. Mirrors TS CalcExpression.
type CalcExpression struct {
	SrcLoc     lexer.NodeSourceLocation
	Expression Expression
	ExpressionBase
}

func (c *CalcExpression) Source() lexer.NodeSourceLocation { return c.SrcLoc }
func (c *CalcExpression) Kind() NodeKind                   { return KindCalcExpression }
func (c *CalcExpression) Children() []Node {
	if c.Expression == nil {
		return nil
	}
	return []Node{c.Expression}
}
func (c *CalcExpression) isNode()       {}
func (c *CalcExpression) isExpression() {}
