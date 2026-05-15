package ast

import "github.com/zsrv/goscape/pkg/pack/compiler/lexer"

// ScriptFile is the top-level node for a parsed `.rs2` file. Mirrors TS
// src/parser/ast/ScriptFile.ts.
type ScriptFile struct {
	SrcLoc  lexer.NodeSourceLocation
	Scripts []*Script
}

func (s *ScriptFile) Source() lexer.NodeSourceLocation { return s.SrcLoc }
func (s *ScriptFile) Kind() NodeKind                   { return KindScriptFile }
func (s *ScriptFile) Children() []Node {
	out := make([]Node, 0, len(s.Scripts))
	for _, sc := range s.Scripts {
		out = append(out, sc)
	}
	return out
}
func (s *ScriptFile) isNode() {}

// Script is a single `[trigger,name] params returns statements*` block.
// Mirrors TS src/parser/ast/Scripts.ts.
//
// NAI-204-D-AST-NO-TYPE-FIELDS: TS Script.symbol, .block, .returnType,
// .triggerType, .subjectReference, .parameterType fields are NAI-205-owned
// and not present here.
type Script struct {
	SrcLoc       lexer.NodeSourceLocation
	Trigger      *Identifier
	Name         *Identifier
	IsStar       bool
	Parameters   []*Parameter // nil if header had no parameter list
	ReturnTokens []*Token     // nil if header had no return-type list
	Statements   []Statement
}

func (s *Script) Source() lexer.NodeSourceLocation { return s.SrcLoc }
func (s *Script) Kind() NodeKind                   { return KindScript }
func (s *Script) Children() []Node {
	out := make([]Node, 0, 2+len(s.Parameters)+len(s.ReturnTokens)+len(s.Statements))
	if s.Trigger != nil {
		out = append(out, s.Trigger)
	}
	if s.Name != nil {
		out = append(out, s.Name)
	}
	for _, p := range s.Parameters {
		out = append(out, p)
	}
	for _, rt := range s.ReturnTokens {
		out = append(out, rt)
	}
	for _, st := range s.Statements {
		out = append(out, st)
	}
	return out
}
func (s *Script) isNode() {}

// NameString returns the script name with optional `*` suffix. Mirrors
// TS Script.nameString getter. Precondition: Name must be non-nil
// (parser-constructed Scripts always satisfy this; TS makes Name a
// required ctor arg).
func (s *Script) NameString() string {
	if s.IsStar {
		return s.Name.Text + "*"
	}
	return s.Name.Text
}

// Parameter is one `type DOLLAR advancedIdentifier` in a script header.
// Mirrors TS src/parser/ast/Parameter.ts.
//
// NAI-204-D-AST-NO-TYPE-FIELDS: TS Parameter.symbol is NAI-205-owned.
type Parameter struct {
	SrcLoc    lexer.NodeSourceLocation
	TypeToken *Token
	Name      *Identifier
}

func (p *Parameter) Source() lexer.NodeSourceLocation { return p.SrcLoc }
func (p *Parameter) Kind() NodeKind                   { return KindParameter }
func (p *Parameter) Children() []Node {
	out := make([]Node, 0, 2)
	if p.TypeToken != nil {
		out = append(out, p.TypeToken)
	}
	if p.Name != nil {
		out = append(out, p.Name)
	}
	return out
}
func (p *Parameter) isNode() {}

// Token is a leaf node carrying a single antlr-token's text + location.
// Mirrors TS src/parser/ast/Token.ts. Used by SwitchStatement.TypeToken,
// DeclarationStatement.TypeToken, Script.ReturnTokens, BinaryExpression.Operator.
type Token struct {
	SrcLoc lexer.NodeSourceLocation
	Text   string
}

func (t *Token) Source() lexer.NodeSourceLocation { return t.SrcLoc }
func (t *Token) Kind() NodeKind                   { return KindToken }
func (t *Token) Children() []Node                 { return nil }
func (t *Token) isNode()                          {}

// Identifier is an identifier expression. Mirrors TS
// src/parser/ast/expr/Identifier.ts. Implements Expression — used both
// for bare identifiers and as a sub-node for variable/call names.
//
// NAI-204-D-AST-NO-TYPE-FIELDS: TS Identifier.reference is NAI-205-owned.
type Identifier struct {
	SrcLoc lexer.NodeSourceLocation
	Text   string
}

func (i *Identifier) Source() lexer.NodeSourceLocation { return i.SrcLoc }
func (i *Identifier) Kind() NodeKind                   { return KindIdentifier }
func (i *Identifier) Children() []Node                 { return nil }
func (i *Identifier) isNode()                          {}
func (i *Identifier) isExpression()                    {}
