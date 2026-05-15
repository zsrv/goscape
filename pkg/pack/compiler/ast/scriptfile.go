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
// .triggerType, .subjectReference, .parameterType landed in NAI-205 (this
// file). The remaining TypeChecking-owned fields (.defaultCase/.type on
// SwitchStatement, .symbol on Declaration*/CallExpression, .reference on
// Identifier/Literal/VariableExpression, .subExpression on
// ConstantVariableExpression/StringLiteral) are NAI-206-owned.
type Script struct {
	SrcLoc       lexer.NodeSourceLocation
	Trigger      *Identifier
	Name         *Identifier
	IsStar       bool
	Parameters   []*Parameter // nil if header had no parameter list
	ReturnTokens []*Token     // nil if header had no return-type list
	Statements   []Statement

	// NAI-205-populated fields (lifted from NAI-204-D-AST-NO-TYPE-FIELDS).
	// Set by pkg/pack/compiler/semantics.ScriptRegistration.

	// TriggerType is the resolved trigger; nil if trigger lookup failed
	// during ScriptRegistration. Concrete type: *trigger.TriggerType.
	TriggerType TriggerRef

	// Symbol is the ServerScriptSymbol inserted into the root SymbolTable
	// for this script; nil if the insert failed (redeclaration).
	// Concrete type: *symbol.ServerScriptSymbol.
	Symbol SymbolRef

	// Block is the per-script local SymbolTable holding parameter symbols.
	// nil before ScriptRegistration runs. Concrete type: *symbol.SymbolTable.
	Block SymbolTableRef

	// ParameterType is the TupleType (or MetaUnit for no params, or single
	// param's type) summarising the parameter list. Concrete type: type.Type.
	ParameterType TypeRef

	// ReturnType mirrors ParameterType for the returns list. Concrete type:
	// type.Type.
	ReturnType TypeRef

	// SubjectReference is the BasicSymbol resolved for type/category subjects;
	// nil for global (`_`) subjects or unresolved references.
	// Concrete type: *symbol.BasicSymbol.
	SubjectReference SymbolRef
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
// NAI-204-D-AST-NO-TYPE-FIELDS: TS Parameter.symbol landed in NAI-205
// (Symbol field below). No further Parameter fields are deferred to NAI-206.
type Parameter struct {
	SrcLoc    lexer.NodeSourceLocation
	TypeToken *Token
	Name      *Identifier

	// Symbol is the LocalVariableSymbol inserted into the script's Block
	// table for this parameter. nil before ScriptRegistration runs.
	// Concrete type: *symbol.LocalVariableSymbol.
	Symbol SymbolRef
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
// NAI-204-D-AST-NO-TYPE-FIELDS: TS Identifier.reference is NAI-206-owned
// (lifted by TypeChecking; NAI-205 doesn't write to Identifier).
type Identifier struct {
	SrcLoc lexer.NodeSourceLocation
	Text   string
}

func (i *Identifier) Source() lexer.NodeSourceLocation { return i.SrcLoc }
func (i *Identifier) Kind() NodeKind                   { return KindIdentifier }
func (i *Identifier) Children() []Node                 { return nil }
func (i *Identifier) isNode()                          {}
func (i *Identifier) isExpression()                    {}
