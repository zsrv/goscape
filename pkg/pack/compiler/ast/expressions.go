package ast

import "github.com/zsrv/goscape/pkg/pack/compiler/lexer"

// ParenthesizedExpression is `(expr)`. Mirrors TS ParenthesizedExpression.
type ParenthesizedExpression struct {
	SrcLoc     lexer.NodeSourceLocation
	Expression Expression
	ExpressionBase
}

func (p *ParenthesizedExpression) Source() lexer.NodeSourceLocation { return p.SrcLoc }
func (p *ParenthesizedExpression) Kind() NodeKind                   { return KindParenthesizedExpression }
func (p *ParenthesizedExpression) Children() []Node {
	if p.Expression == nil {
		return nil
	}
	return []Node{p.Expression}
}
func (p *ParenthesizedExpression) isNode()       {}
func (p *ParenthesizedExpression) isExpression() {}

// JoinedStringExpression is an interpolated string: `"text <tag> <$expr>"`.
// Mirrors TS JoinedStringExpression.
type JoinedStringExpression struct {
	SrcLoc lexer.NodeSourceLocation
	Parts  []StringPart
	ExpressionBase
}

func (j *JoinedStringExpression) Source() lexer.NodeSourceLocation { return j.SrcLoc }
func (j *JoinedStringExpression) Kind() NodeKind                   { return KindJoinedStringExpression }
func (j *JoinedStringExpression) Children() []Node {
	out := make([]Node, 0, len(j.Parts))
	for _, p := range j.Parts {
		out = append(out, p)
	}
	return out
}
func (j *JoinedStringExpression) isNode()       {}
func (j *JoinedStringExpression) isExpression() {}

// StringPart is the sealed marker for parts of a JoinedStringExpression.
// Mirrors TS abstract class StringPart.
type StringPart interface {
	Node
	isStringPart()
}

// BasicStringPart is a literal-text or simple-tag part. Mirrors TS BasicStringPart.
type BasicStringPart struct {
	SrcLoc lexer.NodeSourceLocation
	Value  string
}

func (b *BasicStringPart) Source() lexer.NodeSourceLocation { return b.SrcLoc }
func (b *BasicStringPart) Kind() NodeKind                   { return KindBasicStringPart }
func (b *BasicStringPart) Children() []Node                 { return nil }
func (b *BasicStringPart) isNode()                          {}
func (b *BasicStringPart) isStringPart()                    {}

// PTagStringPart is the `<p,name>` part. Mirrors TS PTagStringPart.
type PTagStringPart struct {
	SrcLoc lexer.NodeSourceLocation
	Value  string
}

func (p *PTagStringPart) Source() lexer.NodeSourceLocation { return p.SrcLoc }
func (p *PTagStringPart) Kind() NodeKind                   { return KindPTagStringPart }
func (p *PTagStringPart) Children() []Node                 { return nil }
func (p *PTagStringPart) isNode()                          {}
func (p *PTagStringPart) isStringPart()                    {}

// ExpressionStringPart is an interpolated `<expr>` inside a joined string.
// Mirrors TS ExpressionStringPart.
type ExpressionStringPart struct {
	SrcLoc     lexer.NodeSourceLocation
	Expression Expression
}

func (e *ExpressionStringPart) Source() lexer.NodeSourceLocation { return e.SrcLoc }
func (e *ExpressionStringPart) Kind() NodeKind                   { return KindExpressionStringPart }
func (e *ExpressionStringPart) Children() []Node {
	if e.Expression == nil {
		return nil
	}
	return []Node{e.Expression}
}
func (e *ExpressionStringPart) isNode()       {}
func (e *ExpressionStringPart) isStringPart() {}
