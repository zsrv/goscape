package parser

import (
	"strconv"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

// parseExpression dispatches on LA(1) to one of the expression-shape
// productions per grammar rule `expression`. Calls / paren / calc /
// precedence-climber land in T9; T8 covers literals, variables, and
// bare-identifier subset plus joined-string-collapse.
func (p *Parser) parseExpression() ast.Expression {
	switch p.ts.LA(1) {
	// Literals
	case lexer.INTEGER_LITERAL, lexer.HEX_LITERAL, lexer.BIN_LITERAL:
		return p.parseIntegerLiteral()
	case lexer.COORD_LITERAL:
		return p.parseCoordLiteral()
	case lexer.BOOLEAN_LITERAL:
		return p.parseBooleanLiteral()
	case lexer.CHAR_LITERAL:
		return p.parseCharacterLiteral()
	case lexer.NULL_LITERAL:
		return p.parseNullLiteral()
	case lexer.QUOTE_OPEN:
		return p.parseStringLiteralOrJoined()

	// Variables
	case lexer.DOLLAR:
		return p.parseLocalVariableOrArray()
	case lexer.MOD, lexer.DOTMOD:
		return p.parseGameVariable()
	case lexer.CARET:
		return p.parseConstantVariable()

	// Call shapes
	case lexer.TILDE:
		return p.parseProcCall()
	case lexer.AT:
		return p.parseJumpCall()

	// Parenthesized expression or calc
	case lexer.LPAREN:
		return p.parseParenthesizedExpression()
	case lexer.CALC:
		return p.parseCalcExpression()
	}

	// Bare identifier — covers all identifier-start tokens per grammar
	// `identifier` rule. May be followed by LPAREN for a command call.
	if isIdentifierStart(p.ts.LA(1)) {
		name := p.parseIdentifier()
		if name == nil {
			return nil
		}
		if p.ts.LA(1) == lexer.LPAREN {
			return p.parseCommandCallTail(name /*star=*/, false)
		}
		if p.ts.LA(1) == lexer.MUL && p.ts.LA(2) == lexer.LPAREN {
			p.ts.Consume() // MUL
			return p.parseCommandCallTail(name /*star=*/, true)
		}
		return name
	}

	p.reportError(p.ts.LT(1), "expected expression but found %s", p.ts.LA(1))
	return nil
}

// parseCommandCallTail parses the LPAREN expressionList? RPAREN (one or
// two times for `name*(args)(args2)`). `star` reflects whether the
// caller already consumed the MUL.
func (p *Parser) parseCommandCallTail(name *ast.Identifier, star bool) *ast.CommandCallExpression {
	if _, ok := p.consumeIf(lexer.LPAREN); !ok {
		p.reportError(p.ts.LT(1), "expected LPAREN after command name but found %s", p.ts.LA(1))
		return nil
	}
	args := p.parseExpressionList()
	if args == nil {
		return nil
	}
	rparen1, ok := p.consumeIf(lexer.RPAREN)
	if !ok {
		p.reportError(p.ts.LT(1), "expected RPAREN to close command args but found %s", p.ts.LA(1))
		return nil
	}
	var args2 []ast.Expression
	endTok := rparen1
	if star {
		if _, ok := p.consumeIf(lexer.LPAREN); !ok {
			p.reportError(p.ts.LT(1), "expected LPAREN after `*` for star command but found %s", p.ts.LA(1))
			return nil
		}
		args2 = p.parseExpressionList()
		if args2 == nil {
			return nil
		}
		rparen2, ok := p.consumeIf(lexer.RPAREN)
		if !ok {
			p.reportError(p.ts.LT(1), "expected RPAREN to close star command args2 but found %s", p.ts.LA(1))
			return nil
		}
		endTok = rparen2
	}
	startLoc := name.SrcLoc
	return &ast.CommandCallExpression{
		SrcLoc: lexer.NodeSourceLocation{
			Name: startLoc.Name, Line: startLoc.Line, Column: startLoc.Column,
			EndLine: endTok.Source.EndLine, EndColumn: endTok.Source.EndColumn,
		},
		Name:       name,
		Arguments:  args,
		Arguments2: args2,
	}
}

// parseProcCall parses `TILDE identifier (LPAREN expressionList? RPAREN)?`.
func (p *Parser) parseProcCall() *ast.ProcCallExpression {
	startTok := p.ts.LT(1)
	p.ts.Consume() // TILDE
	name := p.parseIdentifier()
	if name == nil {
		return nil
	}
	var args []ast.Expression
	endLoc := name.SrcLoc
	if _, ok := p.consumeIf(lexer.LPAREN); ok {
		args = p.parseExpressionList()
		if args == nil {
			return nil
		}
		rparen, ok := p.consumeIf(lexer.RPAREN)
		if !ok {
			p.reportError(p.ts.LT(1), "expected RPAREN to close proc args but found %s", p.ts.LA(1))
			return nil
		}
		endLoc = rparen.Source
	} else {
		args = []ast.Expression{}
	}
	return &ast.ProcCallExpression{
		SrcLoc: lexer.NodeSourceLocation{
			Name: startTok.Source.Name, Line: startTok.Source.Line, Column: startTok.Source.Column,
			EndLine: endLoc.EndLine, EndColumn: endLoc.EndColumn,
		},
		Name:      name,
		Arguments: args,
	}
}

// parseJumpCall parses `AT identifier (LPAREN expressionList? RPAREN)?`.
func (p *Parser) parseJumpCall() *ast.JumpCallExpression {
	startTok := p.ts.LT(1)
	p.ts.Consume() // AT
	name := p.parseIdentifier()
	if name == nil {
		return nil
	}
	var args []ast.Expression
	endLoc := name.SrcLoc
	if _, ok := p.consumeIf(lexer.LPAREN); ok {
		args = p.parseExpressionList()
		if args == nil {
			return nil
		}
		rparen, ok := p.consumeIf(lexer.RPAREN)
		if !ok {
			p.reportError(p.ts.LT(1), "expected RPAREN to close jump args but found %s", p.ts.LA(1))
			return nil
		}
		endLoc = rparen.Source
	} else {
		args = []ast.Expression{}
	}
	return &ast.JumpCallExpression{
		SrcLoc: lexer.NodeSourceLocation{
			Name: startTok.Source.Name, Line: startTok.Source.Line, Column: startTok.Source.Column,
			EndLine: endLoc.EndLine, EndColumn: endLoc.EndColumn,
		},
		Name:      name,
		Arguments: args,
	}
}

// parseParenthesizedExpression parses `LPAREN expression RPAREN`.
func (p *Parser) parseParenthesizedExpression() *ast.ParenthesizedExpression {
	startTok := p.ts.LT(1)
	p.ts.Consume() // LPAREN
	inner := p.parseExpression()
	if inner == nil {
		return nil
	}
	rparen, ok := p.consumeIf(lexer.RPAREN)
	if !ok {
		p.reportError(p.ts.LT(1), "expected RPAREN to close parenthesized expression but found %s", p.ts.LA(1))
		return nil
	}
	return &ast.ParenthesizedExpression{
		SrcLoc:     spanOf(startTok, &rparen),
		Expression: inner,
	}
}

// parseCalcExpression parses `CALC LPAREN arithmetic RPAREN`. Inner
// expression is parsed via the arithmetic precedence climber.
func (p *Parser) parseCalcExpression() *ast.CalcExpression {
	startTok := p.ts.LT(1)
	p.ts.Consume() // CALC
	if _, ok := p.consumeIf(lexer.LPAREN); !ok {
		p.reportError(p.ts.LT(1), "expected LPAREN after calc but found %s", p.ts.LA(1))
		return nil
	}
	arith := p.parseArithmetic()
	if arith == nil {
		return nil
	}
	rparen, ok := p.consumeIf(lexer.RPAREN)
	if !ok {
		p.reportError(p.ts.LT(1), "expected RPAREN to close calc but found %s", p.ts.LA(1))
		return nil
	}
	return &ast.CalcExpression{
		SrcLoc:     spanOf(startTok, &rparen),
		Expression: arith,
	}
}

// parseIntegerLiteral handles INTEGER_LITERAL / HEX_LITERAL / BIN_LITERAL.
// Mirrors TS AstBuilder.visitIntegerLiteral lines 296-303.
func (p *Parser) parseIntegerLiteral() *ast.IntegerLiteral {
	tok := p.consume()
	v, err := decodeIntegerLiteralText(tok.Type, tok.Text)
	if err != nil {
		p.reportError(&tok, "invalid integer literal %q: %v", tok.Text, err)
		return nil
	}
	return &ast.IntegerLiteral{SrcLoc: tok.Source, Value: v}
}

// decodeIntegerLiteralText converts a lexer integer-literal token's
// text into an int32. INTEGER_LITERAL is decimal (with optional leading
// '-'). HEX_LITERAL is 0x/0X-prefixed hex. BIN_LITERAL is 0b/0B-prefixed
// binary.
func decodeIntegerLiteralText(tt lexer.TokenType, text string) (int32, error) {
	switch tt {
	case lexer.HEX_LITERAL:
		n, err := strconv.ParseUint(text[2:], 16, 32)
		if err != nil {
			return 0, err
		}
		return int32(uint32(n)), nil
	case lexer.BIN_LITERAL:
		n, err := strconv.ParseUint(text[2:], 2, 32)
		if err != nil {
			return 0, err
		}
		return int32(uint32(n)), nil
	default: // INTEGER_LITERAL
		n, err := strconv.ParseInt(text, 10, 32)
		if err != nil {
			return 0, err
		}
		return int32(n), nil
	}
}

// parseCoordLiteral handles COORD_LITERAL. Bit-packs per TS
// AstBuilder.visitCoordLiteral lines 306-316.
func (p *Parser) parseCoordLiteral() *ast.CoordLiteral {
	tok := p.consume()
	parts, err := splitCoordParts(tok.Text)
	if err != nil {
		p.reportError(&tok, "invalid coord literal %q: %v", tok.Text, err)
		return nil
	}
	if len(parts) != 5 {
		p.reportError(&tok, "coord literal %q must have 5 underscore-separated parts", tok.Text)
		return nil
	}
	x := (parts[1] << 6) | (parts[3] & 0x3fff)
	z := (parts[2] << 6) | (parts[4] & 0x3fff)
	y := parts[0] & 0x3
	packed := int32(z | (x << 14) | (y << 28))
	return &ast.CoordLiteral{SrcLoc: tok.Source, Value: packed}
}

// splitCoordParts splits "A_B_C_D_E" into [A, B, C, D, E] as ints.
func splitCoordParts(text string) ([]int32, error) {
	out := []int32{}
	start := 0
	for i := 0; i <= len(text); i++ {
		if i == len(text) || text[i] == '_' {
			seg := text[start:i]
			n, err := strconv.ParseInt(seg, 10, 32)
			if err != nil {
				return nil, err
			}
			out = append(out, int32(n))
			start = i + 1
		}
	}
	return out, nil
}

// parseBooleanLiteral handles BOOLEAN_LITERAL.
func (p *Parser) parseBooleanLiteral() *ast.BooleanLiteral {
	tok := p.consume()
	return &ast.BooleanLiteral{SrcLoc: tok.Source, Value: tok.Text == "true"}
}

// parseCharacterLiteral handles CHAR_LITERAL: `'x'` after stripping
// quotes + unescaping must have length 1.
func (p *Parser) parseCharacterLiteral() *ast.CharacterLiteral {
	tok := p.consume()
	if len(tok.Text) < 2 {
		p.reportError(&tok, "invalid character literal %q: too short", tok.Text)
		return nil
	}
	inner := tok.Text[1 : len(tok.Text)-1]
	cleaned, err := unescapeStringPart(inner)
	if err != nil {
		p.reportError(&tok, "invalid character literal %q: %v", tok.Text, err)
		return nil
	}
	if len([]rune(cleaned)) != 1 {
		p.reportError(&tok, "invalid character literal %q: cleaned text %q is not exactly 1 char", tok.Text, cleaned)
		return nil
	}
	return &ast.CharacterLiteral{SrcLoc: tok.Source, Value: cleaned}
}

// parseNullLiteral handles NULL_LITERAL.
func (p *Parser) parseNullLiteral() *ast.NullLiteral {
	tok := p.consume()
	return &ast.NullLiteral{SrcLoc: tok.Source}
}

// parseStringLiteralOrJoined handles QUOTE_OPEN through QUOTE_CLOSE.
//
// Decision: if the body contains only STRING_TEXT parts, return
// *StringLiteral; if any STRING_TAG / STRING_CLOSE_TAG /
// STRING_PARTIAL_TAG / STRING_P_TAG / STRING_EXPR_START appears,
// return *JoinedStringExpression.
func (p *Parser) parseStringLiteralOrJoined() ast.Expression {
	startTok := p.consume() // QUOTE_OPEN

	type rawPart struct {
		kind lexer.TokenType
		tok  lexer.Token
		expr ast.Expression
		src  lexer.NodeSourceLocation
	}
	var raw []rawPart
	hasNonText := false

	for {
		la := p.ts.LA(1)
		switch la {
		case lexer.STRING_TEXT:
			tok := p.consume()
			raw = append(raw, rawPart{kind: la, tok: tok, src: tok.Source})
		case lexer.STRING_TAG, lexer.STRING_CLOSE_TAG, lexer.STRING_PARTIAL_TAG:
			tok := p.consume()
			raw = append(raw, rawPart{kind: la, tok: tok, src: tok.Source})
			hasNonText = true
		case lexer.STRING_P_TAG:
			tok := p.consume()
			raw = append(raw, rawPart{kind: la, tok: tok, src: tok.Source})
			hasNonText = true
		case lexer.STRING_EXPR_START:
			startExpr := p.consume()
			inner := p.parseExpression()
			if inner == nil {
				return nil
			}
			endExpr, ok := p.consumeIf(lexer.STRING_EXPR_END)
			if !ok {
				p.reportError(p.ts.LT(1), "expected > to close string interpolation but found %s", p.ts.LA(1))
				return nil
			}
			raw = append(raw, rawPart{
				kind: la,
				tok:  startExpr,
				expr: inner,
				src:  spanOf(&startExpr, &endExpr),
			})
			hasNonText = true
		case lexer.QUOTE_CLOSE:
			closeTok := p.consume()
			if !hasNonText {
				var sb []byte
				for _, r := range raw {
					unesc, err := unescapeStringPart(r.tok.Text)
					if err != nil {
						p.reportError(&r.tok, "invalid escape in string: %v", err)
						return nil
					}
					sb = append(sb, unesc...)
				}
				return &ast.StringLiteral{
					SrcLoc: spanOf(&startTok, &closeTok),
					Value:  string(sb),
				}
			}
			parts := []ast.StringPart{}
			for _, r := range raw {
				switch r.kind {
				case lexer.STRING_TEXT:
					unesc, err := unescapeStringPart(r.tok.Text)
					if err != nil {
						p.reportError(&r.tok, "invalid escape in string: %v", err)
						return nil
					}
					parts = append(parts, &ast.BasicStringPart{SrcLoc: r.src, Value: unesc})
				case lexer.STRING_TAG, lexer.STRING_CLOSE_TAG, lexer.STRING_PARTIAL_TAG:
					parts = append(parts, &ast.BasicStringPart{SrcLoc: r.src, Value: r.tok.Text})
				case lexer.STRING_P_TAG:
					parts = append(parts, &ast.PTagStringPart{SrcLoc: r.src, Value: r.tok.Text})
				case lexer.STRING_EXPR_START:
					parts = append(parts, &ast.ExpressionStringPart{SrcLoc: r.src, Expression: r.expr})
				}
			}
			return &ast.JoinedStringExpression{
				SrcLoc: spanOf(&startTok, &closeTok),
				Parts:  parts,
			}
		case lexer.EOF:
			p.reportError(p.ts.LT(1), "unexpected EOF inside string literal")
			return nil
		default:
			p.reportError(p.ts.LT(1), "unexpected token %s inside string literal", la)
			return nil
		}
	}
}

// unescapeStringPart replaces `\\ \' \" \<` with the unescaped chars.
// Mirrors TS AstBuilder.unescape lines 463-480.
func unescapeStringPart(value string) (string, error) {
	var out []byte
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == '\\' {
			if i == len(value)-1 {
				return "", errUnsupportedEscape{next: '\\'}
			}
			n := value[i+1]
			switch n {
			case '\\', '\'', '"', '<':
				out = append(out, n)
				i++
				continue
			}
			return "", errUnsupportedEscape{next: n}
		}
		out = append(out, c)
	}
	return string(out), nil
}

type errUnsupportedEscape struct{ next byte }

func (e errUnsupportedEscape) Error() string {
	return "unsupported escape sequence: \\" + string([]byte{e.next})
}

// parseLocalVariableOrArray handles `$name` and `$name(index)`.
func (p *Parser) parseLocalVariableOrArray() *ast.LocalVariableExpression {
	startTok := p.ts.LT(1)
	p.ts.Consume() // DOLLAR
	name := p.parseAdvancedIdentifier()
	if name == nil {
		return nil
	}
	if p.ts.LA(1) != lexer.LPAREN {
		return &ast.LocalVariableExpression{
			SrcLoc: lexer.NodeSourceLocation{
				Name: startTok.Source.Name, Line: startTok.Source.Line, Column: startTok.Source.Column,
				EndLine: name.SrcLoc.EndLine, EndColumn: name.SrcLoc.EndColumn,
			},
			Name: name,
		}
	}
	p.ts.Consume() // LPAREN
	idx := p.parseExpression()
	if idx == nil {
		return nil
	}
	rparen, ok := p.consumeIf(lexer.RPAREN)
	if !ok {
		p.reportError(p.ts.LT(1), "expected RPAREN after array index but found %s", p.ts.LA(1))
		return nil
	}
	return &ast.LocalVariableExpression{
		SrcLoc: spanOf(startTok, &rparen),
		Name:   name,
		Index:  idx,
	}
}

// parseGameVariable handles `%name` and `.%name`.
func (p *Parser) parseGameVariable() *ast.GameVariableExpression {
	startTok := p.ts.LT(1)
	dot := startTok.Type == lexer.DOTMOD
	p.ts.Consume() // MOD or DOTMOD
	name := p.parseAdvancedIdentifier()
	if name == nil {
		return nil
	}
	return &ast.GameVariableExpression{
		SrcLoc: lexer.NodeSourceLocation{
			Name: startTok.Source.Name, Line: startTok.Source.Line, Column: startTok.Source.Column,
			EndLine: name.SrcLoc.EndLine, EndColumn: name.SrcLoc.EndColumn,
		},
		Dot:  dot,
		Name: name,
	}
}

// parseConstantVariable handles `^name`.
func (p *Parser) parseConstantVariable() *ast.ConstantVariableExpression {
	startTok := p.ts.LT(1)
	p.ts.Consume() // CARET
	name := p.parseAdvancedIdentifier()
	if name == nil {
		return nil
	}
	return &ast.ConstantVariableExpression{
		SrcLoc: lexer.NodeSourceLocation{
			Name: startTok.Source.Name, Line: startTok.Source.Line, Column: startTok.Source.Column,
			EndLine: name.SrcLoc.EndLine, EndColumn: name.SrcLoc.EndColumn,
		},
		Name: name,
	}
}
