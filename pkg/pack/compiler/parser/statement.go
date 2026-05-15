package parser

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

// parseStatement dispatches to the per-statement-kind parsers based on
// LA(1). Dispatch order is final and non-overlapping — each TokenType
// case is unique.
//
// NAI-204-D-PARSER-PANIC-SYNC: on hard-fail returns, callers (parseScript,
// parseBlockStatement, parseSwitchCase) MUST call syncToStatement.
func (p *Parser) parseStatement() ast.Statement {
	switch p.ts.LA(1) {
	case lexer.LBRACE:
		return p.parseBlockStatement()
	case lexer.RETURN:
		return p.parseReturnStatement()
	case lexer.IF:
		return p.parseIfStatement()
	case lexer.WHILE:
		return p.parseWhileStatement()
	case lexer.SWITCH_TYPE:
		return p.parseSwitchStatement()
	case lexer.DEF_TYPE:
		return p.parseDeclOrArrayDecl()
	case lexer.SEMICOLON:
		return p.parseEmptyStatement()
	}
	return p.parseAssignOrExprStatement()
}

// parseBlockStatement parses `LBRACE statement* RBRACE`. Mirrors TS
// AstBuilder.visitBlockStatement.
func (p *Parser) parseBlockStatement() *ast.BlockStatement {
	startTok := p.ts.LT(1)
	p.ts.Consume() // LBRACE
	stmts := []ast.Statement{}
	// Exit on LBRACK too — a stray script-header inside a block is
	// malformed; without this guard, syncToStatement halts on LBRACK
	// (a sync boundary) without consuming it, and the loop would spin.
	for p.ts.LA(1) != lexer.RBRACE && p.ts.LA(1) != lexer.EOF && p.ts.LA(1) != lexer.LBRACK {
		st := p.parseStatement()
		if st == nil {
			p.syncToStatement()
			continue
		}
		stmts = append(stmts, st)
	}
	closeTok, ok := p.consumeIf(lexer.RBRACE)
	if !ok {
		p.reportError(p.ts.LT(1), "expected RBRACE to close block but found %s", p.ts.LA(1))
		return nil
	}
	return &ast.BlockStatement{
		SrcLoc:     spanOf(startTok, &closeTok),
		Statements: stmts,
	}
}

// parseEmptyStatement parses a bare `;`.
func (p *Parser) parseEmptyStatement() *ast.EmptyStatement {
	tok := p.consume() // SEMICOLON
	return &ast.EmptyStatement{SrcLoc: tok.Source}
}

// parseReturnStatement parses `RETURN (LPAREN expressionList? RPAREN)? SEMICOLON`.
// Bare `return;` and `return();` both yield zero-length Expressions.
func (p *Parser) parseReturnStatement() *ast.ReturnStatement {
	startTok := p.ts.LT(1)
	p.ts.Consume() // RETURN
	var exprs []ast.Expression
	if _, ok := p.consumeIf(lexer.LPAREN); ok {
		exprs = p.parseExpressionList()
		if exprs == nil {
			return nil
		}
		if _, ok := p.consumeIf(lexer.RPAREN); !ok {
			p.reportError(p.ts.LT(1), "expected RPAREN to close return list but found %s", p.ts.LA(1))
			return nil
		}
	} else {
		exprs = []ast.Expression{}
	}
	semiTok, ok := p.consumeIf(lexer.SEMICOLON)
	if !ok {
		p.reportError(p.ts.LT(1), "expected SEMICOLON after return but found %s", p.ts.LA(1))
		return nil
	}
	return &ast.ReturnStatement{
		SrcLoc:      spanOf(startTok, &semiTok),
		Expressions: exprs,
	}
}

// parseIfStatement parses `IF LPAREN condition RPAREN statement (ELSE statement)?`.
func (p *Parser) parseIfStatement() *ast.IfStatement {
	startTok := p.ts.LT(1)
	p.ts.Consume() // IF
	if _, ok := p.consumeIf(lexer.LPAREN); !ok {
		p.reportError(p.ts.LT(1), "expected LPAREN after if but found %s", p.ts.LA(1))
		return nil
	}
	cond := p.parseCondition()
	if cond == nil {
		return nil
	}
	if _, ok := p.consumeIf(lexer.RPAREN); !ok {
		p.reportError(p.ts.LT(1), "expected RPAREN after if-condition but found %s", p.ts.LA(1))
		return nil
	}
	thenStmt := p.parseStatement()
	if thenStmt == nil {
		return nil
	}
	var elseStmt ast.Statement
	if _, ok := p.consumeIf(lexer.ELSE); ok {
		elseStmt = p.parseStatement()
		if elseStmt == nil {
			return nil
		}
	}
	endLoc := thenStmt.Source()
	if elseStmt != nil {
		endLoc = elseStmt.Source()
	}
	return &ast.IfStatement{
		SrcLoc: lexer.NodeSourceLocation{
			Name:      startTok.Source.Name,
			Line:      startTok.Source.Line,
			Column:    startTok.Source.Column,
			EndLine:   endLoc.EndLine,
			EndColumn: endLoc.EndColumn,
		},
		Condition:     cond,
		ThenStatement: thenStmt,
		ElseStatement: elseStmt,
	}
}

// parseWhileStatement parses `WHILE LPAREN condition RPAREN statement`.
func (p *Parser) parseWhileStatement() *ast.WhileStatement {
	startTok := p.ts.LT(1)
	p.ts.Consume() // WHILE
	if _, ok := p.consumeIf(lexer.LPAREN); !ok {
		p.reportError(p.ts.LT(1), "expected LPAREN after while but found %s", p.ts.LA(1))
		return nil
	}
	cond := p.parseCondition()
	if cond == nil {
		return nil
	}
	if _, ok := p.consumeIf(lexer.RPAREN); !ok {
		p.reportError(p.ts.LT(1), "expected RPAREN after while-condition but found %s", p.ts.LA(1))
		return nil
	}
	body := p.parseStatement()
	if body == nil {
		return nil
	}
	return &ast.WhileStatement{
		SrcLoc: lexer.NodeSourceLocation{
			Name:      startTok.Source.Name,
			Line:      startTok.Source.Line,
			Column:    startTok.Source.Column,
			EndLine:   body.Source().EndLine,
			EndColumn: body.Source().EndColumn,
		},
		Condition:     cond,
		ThenStatement: body,
	}
}

// parseExpressionList parses `expression (COMMA expression)*`. Returns
// non-nil possibly-empty slice. Returns empty slice if LA(1) is RPAREN.
func (p *Parser) parseExpressionList() []ast.Expression {
	out := []ast.Expression{}
	if p.ts.LA(1) == lexer.RPAREN {
		return out
	}
	first := p.parseExpression()
	if first == nil {
		return nil
	}
	out = append(out, first)
	for p.ts.LA(1) == lexer.COMMA {
		p.ts.Consume()
		nxt := p.parseExpression()
		if nxt == nil {
			return nil
		}
		out = append(out, nxt)
	}
	return out
}

// parseSwitchStatement parses `SWITCH_TYPE parenthesis LBRACE switchCase* RBRACE`.
// Mirrors TS AstBuilder.visitSwitchStatement.
func (p *Parser) parseSwitchStatement() *ast.SwitchStatement {
	startTok := p.ts.LT(1)
	typeToken := p.consume() // SWITCH_TYPE
	if _, ok := p.consumeIf(lexer.LPAREN); !ok {
		p.reportError(p.ts.LT(1), "expected LPAREN after switch type but found %s", p.ts.LA(1))
		return nil
	}
	cond := p.parseExpression()
	if cond == nil {
		return nil
	}
	if _, ok := p.consumeIf(lexer.RPAREN); !ok {
		p.reportError(p.ts.LT(1), "expected RPAREN after switch condition but found %s", p.ts.LA(1))
		return nil
	}
	if _, ok := p.consumeIf(lexer.LBRACE); !ok {
		p.reportError(p.ts.LT(1), "expected LBRACE to open switch body but found %s", p.ts.LA(1))
		return nil
	}
	cases := []*ast.SwitchCase{}
	for p.ts.LA(1) == lexer.CASE {
		c := p.parseSwitchCase()
		if c == nil {
			return nil
		}
		cases = append(cases, c)
	}
	closeTok, ok := p.consumeIf(lexer.RBRACE)
	if !ok {
		p.reportError(p.ts.LT(1), "expected RBRACE to close switch body but found %s", p.ts.LA(1))
		return nil
	}
	return &ast.SwitchStatement{
		SrcLoc:    spanOf(startTok, &closeTok),
		TypeToken: &ast.Token{SrcLoc: typeToken.Source, Text: typeToken.Text},
		Condition: cond,
		Cases:     cases,
	}
}

// parseSwitchCase parses `CASE (DEFAULT | expressionList) COLON statement*`.
// Mirrors TS AstBuilder.visitSwitchCase. DEFAULT keyword → empty Keys
// slice (parity with TS SwitchCase.isDefault: keys.length === 0).
func (p *Parser) parseSwitchCase() *ast.SwitchCase {
	startTok := p.ts.LT(1)
	p.ts.Consume() // CASE
	var keys []ast.Expression
	if _, ok := p.consumeIf(lexer.DEFAULT); ok {
		keys = []ast.Expression{}
	} else {
		keys = p.parseExpressionList()
		if keys == nil {
			return nil
		}
	}
	if _, ok := p.consumeIf(lexer.COLON); !ok {
		p.reportError(p.ts.LT(1), "expected COLON after case keys but found %s", p.ts.LA(1))
		return nil
	}
	stmts := []ast.Statement{}
	for p.ts.LA(1) != lexer.CASE && p.ts.LA(1) != lexer.RBRACE && p.ts.LA(1) != lexer.EOF {
		st := p.parseStatement()
		if st == nil {
			p.syncToStatement()
			continue
		}
		stmts = append(stmts, st)
	}
	endTok := p.ts.LT(1)
	return &ast.SwitchCase{
		SrcLoc: lexer.NodeSourceLocation{
			Name:      startTok.Source.Name,
			Line:      startTok.Source.Line,
			Column:    startTok.Source.Column,
			EndLine:   endTok.Source.EndLine,
			EndColumn: endTok.Source.EndColumn,
		},
		Keys:       keys,
		Statements: stmts,
	}
}

// parseDeclOrArrayDecl handles both:
//
//	declarationStatement      : DEF_TYPE DOLLAR advancedIdentifier (EQ expression)? SEMICOLON
//	arrayDeclarationStatement : DEF_TYPE DOLLAR advancedIdentifier parenthesis SEMICOLON
//
// Dispatch happens after consuming `DEF_TYPE DOLLAR advancedIdentifier`:
// LA(1) == LPAREN → array; LA(1) == EQ → decl-with-init; LA(1) == SEMICOLON → decl-no-init.
func (p *Parser) parseDeclOrArrayDecl() ast.Statement {
	startTok := p.ts.LT(1)
	typeToken := p.consume() // DEF_TYPE
	if _, ok := p.consumeIf(lexer.DOLLAR); !ok {
		p.reportError(p.ts.LT(1), "expected DOLLAR after type %q but found %s", typeToken.Text, p.ts.LA(1))
		return nil
	}
	name := p.parseAdvancedIdentifier()
	if name == nil {
		return nil
	}
	switch p.ts.LA(1) {
	case lexer.LPAREN:
		p.ts.Consume() // LPAREN
		size := p.parseExpression()
		if size == nil {
			return nil
		}
		if _, ok := p.consumeIf(lexer.RPAREN); !ok {
			p.reportError(p.ts.LT(1), "expected RPAREN after array size but found %s", p.ts.LA(1))
			return nil
		}
		semiTok, ok := p.consumeIf(lexer.SEMICOLON)
		if !ok {
			p.reportError(p.ts.LT(1), "expected SEMICOLON after array declaration but found %s", p.ts.LA(1))
			return nil
		}
		return &ast.ArrayDeclarationStatement{
			SrcLoc:      spanOf(startTok, &semiTok),
			TypeToken:   &ast.Token{SrcLoc: typeToken.Source, Text: typeToken.Text},
			Name:        name,
			Initializer: size,
		}
	case lexer.EQ:
		p.ts.Consume() // EQ
		init := p.parseExpression()
		if init == nil {
			return nil
		}
		semiTok, ok := p.consumeIf(lexer.SEMICOLON)
		if !ok {
			p.reportError(p.ts.LT(1), "expected SEMICOLON after declaration initializer but found %s", p.ts.LA(1))
			return nil
		}
		return &ast.DeclarationStatement{
			SrcLoc:      spanOf(startTok, &semiTok),
			TypeToken:   &ast.Token{SrcLoc: typeToken.Source, Text: typeToken.Text},
			Name:        name,
			Initializer: init,
		}
	case lexer.SEMICOLON:
		semiTok := p.consume()
		return &ast.DeclarationStatement{
			SrcLoc:      spanOf(startTok, &semiTok),
			TypeToken:   &ast.Token{SrcLoc: typeToken.Source, Text: typeToken.Text},
			Name:        name,
			Initializer: nil,
		}
	}
	p.reportError(p.ts.LT(1), "expected LPAREN/EQ/SEMICOLON after declaration name but found %s", p.ts.LA(1))
	return nil
}

// parseAssignOrExprStatement disambiguates between:
//
//	assignmentStatement : assignableVariableList EQ expressionList SEMICOLON
//	expressionStatement : expression SEMICOLON
//
// Both can start with a $local, %game, or .%game variable. The parser
// marks the current position, attempts to parse an assignable-variable
// list with errors suppressed, and if LA(1) lands on EQ it's an
// assignment — otherwise it rewinds and parses as an expression statement.
func (p *Parser) parseAssignOrExprStatement() ast.Statement {
	startTok := p.ts.LT(1)

	la := p.ts.LA(1)
	if la == lexer.DOLLAR || la == lexer.MOD || la == lexer.DOTMOD {
		mark := p.ts.Mark()
		vars := p.tryParseAssignableVariableList()
		if vars != nil && p.ts.LA(1) == lexer.EQ {
			p.ts.Consume() // EQ
			exprs := p.parseExpressionList()
			if exprs == nil {
				return nil
			}
			semiTok, ok := p.consumeIf(lexer.SEMICOLON)
			if !ok {
				p.reportError(p.ts.LT(1), "expected SEMICOLON after assignment but found %s", p.ts.LA(1))
				return nil
			}
			return &ast.AssignmentStatement{
				SrcLoc:      spanOf(startTok, &semiTok),
				Vars:        vars,
				Expressions: exprs,
			}
		}
		// Not an assignment — rewind and fall through.
		p.ts.Rewind(mark)
	}

	expr := p.parseExpression()
	if expr == nil {
		return nil
	}
	semiTok, ok := p.consumeIf(lexer.SEMICOLON)
	if !ok {
		p.reportError(p.ts.LT(1), "expected SEMICOLON after expression but found %s", p.ts.LA(1))
		return nil
	}
	return &ast.ExpressionStatement{
		SrcLoc:     spanOf(startTok, &semiTok),
		Expression: expr,
	}
}

// tryParseAssignableVariableList attempts to parse
// `assignableVariable (COMMA assignableVariable)*` without reporting
// any errors. Returns nil if the prefix doesn't match an assignable
// variable shape.
func (p *Parser) tryParseAssignableVariableList() []ast.VariableExpressionNode {
	out := []ast.VariableExpressionNode{}
	suppressed := p.suppressErrorsBegin()
	defer p.suppressErrorsEnd(suppressed)

	first := p.tryParseAssignableVariable()
	if first == nil {
		return nil
	}
	out = append(out, first)
	for p.ts.LA(1) == lexer.COMMA {
		p.ts.Consume()
		nxt := p.tryParseAssignableVariable()
		if nxt == nil {
			return nil
		}
		out = append(out, nxt)
	}
	return out
}

// tryParseAssignableVariable parses one of:
//
//	localVariable      : DOLLAR advancedIdentifier
//	localArrayVariable : DOLLAR advancedIdentifier parenthesis
//	gameVariable       : (MOD | DOTMOD) advancedIdentifier
//
// Returns nil if LA(1) doesn't match.
func (p *Parser) tryParseAssignableVariable() ast.VariableExpressionNode {
	switch p.ts.LA(1) {
	case lexer.DOLLAR:
		return p.parseLocalVariableOrArray()
	case lexer.MOD, lexer.DOTMOD:
		return p.parseGameVariable()
	}
	return nil
}

// suppressedState saves listener+numErrors for backtracking.
type suppressedState struct {
	listeners []lexer.ErrorListener
	numErrors int
}

// suppressErrorsBegin replaces listeners with nil and snapshots
// numErrors so backtracking attempts can be silently undone.
func (p *Parser) suppressErrorsBegin() suppressedState {
	prev := suppressedState{listeners: p.listeners, numErrors: p.numErrors}
	p.listeners = nil
	return prev
}

// suppressErrorsEnd restores the previous listener + numErrors state.
func (p *Parser) suppressErrorsEnd(prev suppressedState) {
	p.listeners = prev.listeners
	p.numErrors = prev.numErrors
}
