package ast

import "github.com/zsrv/goscape/pkg/pack/compiler/lexer"

// CommandCallExpression is `name(args)` or `name*(args)(args2)`. Mirrors
// TS CommandCallExpression. IsStar() reports the latter (parity with
// TS .isStar getter: Arguments2 != null).
//
// NAI-204-D-AST-NO-TYPE-FIELDS: TS CallExpression.symbol is NAI-205-owned.
type CommandCallExpression struct {
	SrcLoc     lexer.NodeSourceLocation
	Name       *Identifier
	Arguments  []Expression
	Arguments2 []Expression // nil if not `name*(...)(...)` form
}

func (c *CommandCallExpression) Source() lexer.NodeSourceLocation { return c.SrcLoc }
func (c *CommandCallExpression) Kind() NodeKind                   { return KindCommandCallExpression }
func (c *CommandCallExpression) Children() []Node {
	out := make([]Node, 0, 1+len(c.Arguments)+len(c.Arguments2))
	if c.Name != nil {
		out = append(out, c.Name)
	}
	for _, a := range c.Arguments {
		out = append(out, a)
	}
	for _, a := range c.Arguments2 {
		out = append(out, a)
	}
	return out
}
func (c *CommandCallExpression) isNode()           {}
func (c *CommandCallExpression) isExpression()     {}
func (c *CommandCallExpression) isCallExpression() {}

// IsStar reports whether the call had `*` after the name (i.e.
// Arguments2 != nil). Parity with TS CommandCallExpression.isStar.
func (c *CommandCallExpression) IsStar() bool { return c.Arguments2 != nil }

// NameString returns Name.Text with optional `*` suffix.
func (c *CommandCallExpression) NameString() string {
	if c.IsStar() {
		return c.Name.Text + "*"
	}
	return c.Name.Text
}

// ProcCallExpression is `~name(args)`. Mirrors TS ProcCallExpression.
type ProcCallExpression struct {
	SrcLoc    lexer.NodeSourceLocation
	Name      *Identifier
	Arguments []Expression
}

func (c *ProcCallExpression) Source() lexer.NodeSourceLocation { return c.SrcLoc }
func (c *ProcCallExpression) Kind() NodeKind                   { return KindProcCallExpression }
func (c *ProcCallExpression) Children() []Node {
	out := make([]Node, 0, 1+len(c.Arguments))
	if c.Name != nil {
		out = append(out, c.Name)
	}
	for _, a := range c.Arguments {
		out = append(out, a)
	}
	return out
}
func (c *ProcCallExpression) isNode()           {}
func (c *ProcCallExpression) isExpression()     {}
func (c *ProcCallExpression) isCallExpression() {}

// JumpCallExpression is `@name(args)`. Mirrors TS JumpCallExpression.
type JumpCallExpression struct {
	SrcLoc    lexer.NodeSourceLocation
	Name      *Identifier
	Arguments []Expression
}

func (c *JumpCallExpression) Source() lexer.NodeSourceLocation { return c.SrcLoc }
func (c *JumpCallExpression) Kind() NodeKind                   { return KindJumpCallExpression }
func (c *JumpCallExpression) Children() []Node {
	out := make([]Node, 0, 1+len(c.Arguments))
	if c.Name != nil {
		out = append(out, c.Name)
	}
	for _, a := range c.Arguments {
		out = append(out, a)
	}
	return out
}
func (c *JumpCallExpression) isNode()           {}
func (c *JumpCallExpression) isExpression()     {}
func (c *JumpCallExpression) isCallExpression() {}

// ClientScriptExpression is `name(args){triggers}` — the standalone
// parse entry rule for clientscript references. Mirrors TS ClientScriptExpression.
type ClientScriptExpression struct {
	SrcLoc       lexer.NodeSourceLocation
	Name         *Identifier
	Arguments    []Expression
	TransmitList []Expression
}

func (c *ClientScriptExpression) Source() lexer.NodeSourceLocation { return c.SrcLoc }
func (c *ClientScriptExpression) Kind() NodeKind                   { return KindClientScriptExpression }
func (c *ClientScriptExpression) Children() []Node {
	out := make([]Node, 0, 1+len(c.Arguments)+len(c.TransmitList))
	if c.Name != nil {
		out = append(out, c.Name)
	}
	for _, a := range c.Arguments {
		out = append(out, a)
	}
	for _, t := range c.TransmitList {
		out = append(out, t)
	}
	return out
}
func (c *ClientScriptExpression) isNode()           {}
func (c *ClientScriptExpression) isExpression()     {}
func (c *ClientScriptExpression) isCallExpression() {}
