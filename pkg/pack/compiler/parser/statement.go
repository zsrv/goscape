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

// --- T6 placeholders that T7/T8/T9 will replace ---

// parseSwitchStatement is implemented by T7.
func (p *Parser) parseSwitchStatement() *ast.SwitchStatement {
	p.reportError(p.ts.LT(1), "switch parsing unimplemented in T6 (token: %s)", p.ts.LA(1))
	return nil
}

// parseDeclOrArrayDecl is implemented by T7.
func (p *Parser) parseDeclOrArrayDecl() ast.Statement {
	p.reportError(p.ts.LT(1), "def_T parsing unimplemented in T6 (token: %s)", p.ts.LA(1))
	return nil
}

// parseAssignOrExprStatement is implemented by T8.
func (p *Parser) parseAssignOrExprStatement() ast.Statement {
	p.reportError(p.ts.LT(1), "assignment/expression-statement parsing unimplemented in T6 (token: %s)", p.ts.LA(1))
	return nil
}

// parseCondition is implemented by T9 — for T6 it stubs to
// parseExpression (sufficient for the `if (true) ;` tests which use
// BOOLEAN_LITERAL as the entire condition).
func (p *Parser) parseCondition() ast.Expression {
	return p.parseExpression()
}

// parseExpression is implemented by T8 — for T6 we provide a minimal
// version that handles only INTEGER_LITERAL and BOOLEAN_LITERAL so the
// statement tests can construct return/if/while bodies.
func (p *Parser) parseExpression() ast.Expression {
	switch p.ts.LA(1) {
	case lexer.INTEGER_LITERAL:
		tok := p.consume()
		v, _ := parseIntegerLiteralValue(tok.Text)
		return &ast.IntegerLiteral{SrcLoc: tok.Source, Value: v}
	case lexer.BOOLEAN_LITERAL:
		tok := p.consume()
		return &ast.BooleanLiteral{SrcLoc: tok.Source, Value: tok.Text == "true"}
	}
	p.reportError(p.ts.LT(1), "expected expression but found %s", p.ts.LA(1))
	return nil
}

// parseIntegerLiteralValue parses decimal integer text into an int32.
// T6-provisional implementation — T8 expands to hex/bin.
func parseIntegerLiteralValue(text string) (int32, error) {
	var v int64
	negative := false
	i := 0
	if len(text) > 0 && text[0] == '-' {
		negative = true
		i = 1
	}
	for ; i < len(text); i++ {
		c := text[i]
		if c < '0' || c > '9' {
			return 0, nil
		}
		v = v*10 + int64(c-'0')
	}
	if negative {
		v = -v
	}
	return int32(v), nil
}
