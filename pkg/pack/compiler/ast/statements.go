package ast

import "github.com/zsrv/goscape/pkg/pack/compiler/lexer"

// BlockStatement is `{ statement* }`. Mirrors TS BlockStatement.
type BlockStatement struct {
	SrcLoc     lexer.NodeSourceLocation
	Statements []Statement
}

func (b *BlockStatement) Source() lexer.NodeSourceLocation { return b.SrcLoc }
func (b *BlockStatement) Kind() NodeKind                   { return KindBlockStatement }
func (b *BlockStatement) Children() []Node {
	out := make([]Node, 0, len(b.Statements))
	for _, s := range b.Statements {
		out = append(out, s)
	}
	return out
}
func (b *BlockStatement) isNode()      {}
func (b *BlockStatement) isStatement() {}

// EmptyStatement is a bare `;`. Mirrors TS EmptyStatement.
type EmptyStatement struct {
	SrcLoc lexer.NodeSourceLocation
}

func (e *EmptyStatement) Source() lexer.NodeSourceLocation { return e.SrcLoc }
func (e *EmptyStatement) Kind() NodeKind                   { return KindEmptyStatement }
func (e *EmptyStatement) Children() []Node                 { return nil }
func (e *EmptyStatement) isNode()                          {}
func (e *EmptyStatement) isStatement()                     {}

// ReturnStatement is `return(expr*);`. Mirrors TS ReturnStatement.
type ReturnStatement struct {
	SrcLoc      lexer.NodeSourceLocation
	Expressions []Expression
}

func (r *ReturnStatement) Source() lexer.NodeSourceLocation { return r.SrcLoc }
func (r *ReturnStatement) Kind() NodeKind                   { return KindReturnStatement }
func (r *ReturnStatement) Children() []Node {
	out := make([]Node, 0, len(r.Expressions))
	for _, e := range r.Expressions {
		out = append(out, e)
	}
	return out
}
func (r *ReturnStatement) isNode()      {}
func (r *ReturnStatement) isStatement() {}

// IfStatement is `if (cond) then else?`. Mirrors TS IfStatement.
type IfStatement struct {
	SrcLoc        lexer.NodeSourceLocation
	Condition     Expression
	ThenStatement Statement
	ElseStatement Statement // nil if absent
}

func (s *IfStatement) Source() lexer.NodeSourceLocation { return s.SrcLoc }
func (s *IfStatement) Kind() NodeKind                   { return KindIfStatement }
func (s *IfStatement) Children() []Node {
	out := make([]Node, 0, 3)
	if s.Condition != nil {
		out = append(out, s.Condition)
	}
	if s.ThenStatement != nil {
		out = append(out, s.ThenStatement)
	}
	if s.ElseStatement != nil {
		out = append(out, s.ElseStatement)
	}
	return out
}
func (s *IfStatement) isNode()      {}
func (s *IfStatement) isStatement() {}

// WhileStatement is `while (cond) body`. Mirrors TS WhileStatement.
type WhileStatement struct {
	SrcLoc        lexer.NodeSourceLocation
	Condition     Expression
	ThenStatement Statement
}

func (w *WhileStatement) Source() lexer.NodeSourceLocation { return w.SrcLoc }
func (w *WhileStatement) Kind() NodeKind                   { return KindWhileStatement }
func (w *WhileStatement) Children() []Node {
	out := make([]Node, 0, 2)
	if w.Condition != nil {
		out = append(out, w.Condition)
	}
	if w.ThenStatement != nil {
		out = append(out, w.ThenStatement)
	}
	return out
}
func (w *WhileStatement) isNode()      {}
func (w *WhileStatement) isStatement() {}

// SwitchStatement is `switch_T (cond) { case ... }`. Mirrors TS SwitchStatement.
type SwitchStatement struct {
	SrcLoc      lexer.NodeSourceLocation
	TypeToken   *Token
	Condition   Expression
	Cases       []*SwitchCase
	DefaultCase *SwitchCase // NAI-206-owned (cached pointer to default case if any)
	Type        TypeRef     // NAI-206-owned (resolved switch type)
}

func (s *SwitchStatement) Source() lexer.NodeSourceLocation { return s.SrcLoc }
func (s *SwitchStatement) Kind() NodeKind                   { return KindSwitchStatement }
func (s *SwitchStatement) Children() []Node {
	out := make([]Node, 0, 2+len(s.Cases))
	if s.TypeToken != nil {
		out = append(out, s.TypeToken)
	}
	if s.Condition != nil {
		out = append(out, s.Condition)
	}
	for _, c := range s.Cases {
		out = append(out, c)
	}
	return out
}
func (s *SwitchStatement) isNode()      {}
func (s *SwitchStatement) isStatement() {}

// SwitchCase is one `case (default | exprList) : statement*`. Mirrors TS SwitchCase.
type SwitchCase struct {
	SrcLoc     lexer.NodeSourceLocation
	Keys       []Expression // empty == default case
	Statements []Statement
}

func (c *SwitchCase) Source() lexer.NodeSourceLocation { return c.SrcLoc }
func (c *SwitchCase) Kind() NodeKind                   { return KindSwitchCase }
func (c *SwitchCase) Children() []Node {
	out := make([]Node, 0, len(c.Keys)+len(c.Statements))
	for _, k := range c.Keys {
		out = append(out, k)
	}
	for _, s := range c.Statements {
		out = append(out, s)
	}
	return out
}
func (c *SwitchCase) isNode() {}

// IsDefault reports whether this case has zero keys (parity with TS
// SwitchCase.isDefault getter: keys.length === 0).
func (c *SwitchCase) IsDefault() bool { return len(c.Keys) == 0 }

// DeclarationStatement is `def_T $name (= expr)? ;`. Mirrors TS DeclarationStatement.
type DeclarationStatement struct {
	SrcLoc      lexer.NodeSourceLocation
	TypeToken   *Token
	Name        *Identifier
	Initializer Expression // nil if no `= expr`
	Symbol      SymbolRef  // NAI-206-owned (TS .symbol)
}

func (d *DeclarationStatement) Source() lexer.NodeSourceLocation { return d.SrcLoc }
func (d *DeclarationStatement) Kind() NodeKind                   { return KindDeclarationStatement }
func (d *DeclarationStatement) Children() []Node {
	out := make([]Node, 0, 3)
	if d.TypeToken != nil {
		out = append(out, d.TypeToken)
	}
	if d.Name != nil {
		out = append(out, d.Name)
	}
	if d.Initializer != nil {
		out = append(out, d.Initializer)
	}
	return out
}
func (d *DeclarationStatement) isNode()      {}
func (d *DeclarationStatement) isStatement() {}

// ArrayDeclarationStatement is `def_T $name(size);`. Mirrors TS ArrayDeclarationStatement.
type ArrayDeclarationStatement struct {
	SrcLoc      lexer.NodeSourceLocation
	TypeToken   *Token
	Name        *Identifier
	Initializer Expression // size expression — non-nil per grammar
	Symbol      SymbolRef  // NAI-206-owned (TS .symbol)
}

func (a *ArrayDeclarationStatement) Source() lexer.NodeSourceLocation { return a.SrcLoc }
func (a *ArrayDeclarationStatement) Kind() NodeKind                   { return KindArrayDeclarationStatement }
func (a *ArrayDeclarationStatement) Children() []Node {
	out := make([]Node, 0, 3)
	if a.TypeToken != nil {
		out = append(out, a.TypeToken)
	}
	if a.Name != nil {
		out = append(out, a.Name)
	}
	if a.Initializer != nil {
		out = append(out, a.Initializer)
	}
	return out
}
func (a *ArrayDeclarationStatement) isNode()      {}
func (a *ArrayDeclarationStatement) isStatement() {}

// AssignmentStatement is `lhs (, lhs)* = expr (, expr)* ;`. Mirrors TS AssignmentStatement.
type AssignmentStatement struct {
	SrcLoc      lexer.NodeSourceLocation
	Vars        []VariableExpressionNode
	Expressions []Expression
}

func (a *AssignmentStatement) Source() lexer.NodeSourceLocation { return a.SrcLoc }
func (a *AssignmentStatement) Kind() NodeKind                   { return KindAssignmentStatement }
func (a *AssignmentStatement) Children() []Node {
	out := make([]Node, 0, len(a.Vars)+len(a.Expressions))
	for _, v := range a.Vars {
		out = append(out, v)
	}
	for _, e := range a.Expressions {
		out = append(out, e)
	}
	return out
}
func (a *AssignmentStatement) isNode()      {}
func (a *AssignmentStatement) isStatement() {}

// ExpressionStatement is `expression;`. Mirrors TS ExpressionStatement.
type ExpressionStatement struct {
	SrcLoc     lexer.NodeSourceLocation
	Expression Expression
}

func (e *ExpressionStatement) Source() lexer.NodeSourceLocation { return e.SrcLoc }
func (e *ExpressionStatement) Kind() NodeKind                   { return KindExpressionStatement }
func (e *ExpressionStatement) Children() []Node {
	if e.Expression == nil {
		return nil
	}
	return []Node{e.Expression}
}
func (e *ExpressionStatement) isNode()      {}
func (e *ExpressionStatement) isStatement() {}
