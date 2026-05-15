package parser

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

// parseCondition climbs the condition operator precedence ladder.
//
// Precedence (low → high), per grammar's left-recursive alt order:
//
//	OR
//	AND
//	EQ | EXCL
//	LT | GT | LTE | GTE
//	atom: LPAREN condition RPAREN | expression
//
// All operators left-associative.
func (p *Parser) parseCondition() ast.Expression {
	return p.parseConditionOr()
}

func (p *Parser) parseConditionOr() ast.Expression {
	left := p.parseConditionAnd()
	if left == nil {
		return nil
	}
	for p.ts.LA(1) == lexer.OR {
		opTok := p.consume()
		right := p.parseConditionAnd()
		if right == nil {
			return nil
		}
		left = &ast.ConditionExpression{
			SrcLoc:   spanOfNodes(left, right),
			Left:     left,
			Operator: &ast.Token{SrcLoc: opTok.Source, Text: opTok.Text},
			Right:    right,
		}
	}
	return left
}

func (p *Parser) parseConditionAnd() ast.Expression {
	left := p.parseConditionEq()
	if left == nil {
		return nil
	}
	for p.ts.LA(1) == lexer.AND {
		opTok := p.consume()
		right := p.parseConditionEq()
		if right == nil {
			return nil
		}
		left = &ast.ConditionExpression{
			SrcLoc:   spanOfNodes(left, right),
			Left:     left,
			Operator: &ast.Token{SrcLoc: opTok.Source, Text: opTok.Text},
			Right:    right,
		}
	}
	return left
}

func (p *Parser) parseConditionEq() ast.Expression {
	left := p.parseConditionRel()
	if left == nil {
		return nil
	}
	for la := p.ts.LA(1); la == lexer.EQ || la == lexer.EXCL; la = p.ts.LA(1) {
		opTok := p.consume()
		right := p.parseConditionRel()
		if right == nil {
			return nil
		}
		left = &ast.ConditionExpression{
			SrcLoc:   spanOfNodes(left, right),
			Left:     left,
			Operator: &ast.Token{SrcLoc: opTok.Source, Text: opTok.Text},
			Right:    right,
		}
	}
	return left
}

func (p *Parser) parseConditionRel() ast.Expression {
	left := p.parseConditionAtom()
	if left == nil {
		return nil
	}
	for {
		la := p.ts.LA(1)
		if la != lexer.LT && la != lexer.GT && la != lexer.LTE && la != lexer.GTE {
			break
		}
		opTok := p.consume()
		right := p.parseConditionAtom()
		if right == nil {
			return nil
		}
		left = &ast.ConditionExpression{
			SrcLoc:   spanOfNodes(left, right),
			Left:     left,
			Operator: &ast.Token{SrcLoc: opTok.Source, Text: opTok.Text},
			Right:    right,
		}
	}
	return left
}

// parseConditionAtom handles `LPAREN condition RPAREN` (wrapped in a
// ParenthesizedExpression) or falls through to a normal expression.
func (p *Parser) parseConditionAtom() ast.Expression {
	if p.ts.LA(1) == lexer.LPAREN {
		startTok := p.ts.LT(1)
		p.ts.Consume() // LPAREN
		inner := p.parseCondition()
		if inner == nil {
			return nil
		}
		rparen, ok := p.consumeIf(lexer.RPAREN)
		if !ok {
			p.reportError(p.ts.LT(1), "expected RPAREN to close parenthesized condition but found %s", p.ts.LA(1))
			return nil
		}
		return &ast.ParenthesizedExpression{
			SrcLoc:     spanOf(startTok, &rparen),
			Expression: inner,
		}
	}
	return p.parseExpression()
}

// parseArithmetic climbs the arithmetic operator precedence ladder.
//
// Precedence (low → high), per grammar:
//
//	OR
//	AND
//	PLUS | MINUS
//	MUL | DIV | MOD
//	atom: LPAREN arithmetic RPAREN | expression
//
// All operators left-associative.
func (p *Parser) parseArithmetic() ast.Expression {
	return p.parseArithmeticOr()
}

func (p *Parser) parseArithmeticOr() ast.Expression {
	left := p.parseArithmeticAnd()
	if left == nil {
		return nil
	}
	for p.ts.LA(1) == lexer.OR {
		opTok := p.consume()
		right := p.parseArithmeticAnd()
		if right == nil {
			return nil
		}
		left = &ast.ArithmeticExpression{
			SrcLoc:   spanOfNodes(left, right),
			Left:     left,
			Operator: &ast.Token{SrcLoc: opTok.Source, Text: opTok.Text},
			Right:    right,
		}
	}
	return left
}

func (p *Parser) parseArithmeticAnd() ast.Expression {
	left := p.parseArithmeticAdd()
	if left == nil {
		return nil
	}
	for p.ts.LA(1) == lexer.AND {
		opTok := p.consume()
		right := p.parseArithmeticAdd()
		if right == nil {
			return nil
		}
		left = &ast.ArithmeticExpression{
			SrcLoc:   spanOfNodes(left, right),
			Left:     left,
			Operator: &ast.Token{SrcLoc: opTok.Source, Text: opTok.Text},
			Right:    right,
		}
	}
	return left
}

func (p *Parser) parseArithmeticAdd() ast.Expression {
	left := p.parseArithmeticMul()
	if left == nil {
		return nil
	}
	for la := p.ts.LA(1); la == lexer.PLUS || la == lexer.MINUS; la = p.ts.LA(1) {
		opTok := p.consume()
		right := p.parseArithmeticMul()
		if right == nil {
			return nil
		}
		left = &ast.ArithmeticExpression{
			SrcLoc:   spanOfNodes(left, right),
			Left:     left,
			Operator: &ast.Token{SrcLoc: opTok.Source, Text: opTok.Text},
			Right:    right,
		}
	}
	return left
}

func (p *Parser) parseArithmeticMul() ast.Expression {
	left := p.parseArithmeticAtom()
	if left == nil {
		return nil
	}
	for la := p.ts.LA(1); la == lexer.MUL || la == lexer.DIV || la == lexer.MOD; la = p.ts.LA(1) {
		opTok := p.consume()
		right := p.parseArithmeticAtom()
		if right == nil {
			return nil
		}
		left = &ast.ArithmeticExpression{
			SrcLoc:   spanOfNodes(left, right),
			Left:     left,
			Operator: &ast.Token{SrcLoc: opTok.Source, Text: opTok.Text},
			Right:    right,
		}
	}
	return left
}

// parseArithmeticAtom handles `LPAREN arithmetic RPAREN` (wrapped in a
// ParenthesizedExpression) or falls through to a normal expression.
func (p *Parser) parseArithmeticAtom() ast.Expression {
	if p.ts.LA(1) == lexer.LPAREN {
		startTok := p.ts.LT(1)
		p.ts.Consume() // LPAREN
		inner := p.parseArithmetic()
		if inner == nil {
			return nil
		}
		rparen, ok := p.consumeIf(lexer.RPAREN)
		if !ok {
			p.reportError(p.ts.LT(1), "expected RPAREN to close parenthesized arithmetic but found %s", p.ts.LA(1))
			return nil
		}
		return &ast.ParenthesizedExpression{
			SrcLoc:     spanOf(startTok, &rparen),
			Expression: inner,
		}
	}
	return p.parseExpression()
}

// spanOfNodes builds a NodeSourceLocation covering both endpoint nodes.
func spanOfNodes(start, stop ast.Node) lexer.NodeSourceLocation {
	a, b := start.Source(), stop.Source()
	return lexer.NodeSourceLocation{
		Name:      a.Name,
		Line:      a.Line,
		Column:    a.Column,
		EndLine:   b.EndLine,
		EndColumn: b.EndColumn,
	}
}
