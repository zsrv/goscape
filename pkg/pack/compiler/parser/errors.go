package parser

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

// reportError fires SyntaxError on every registered listener with the
// supplied token's source location and a formatted message, then bumps
// numErrors.
func (p *Parser) reportError(tok *lexer.Token, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	for _, l := range p.listeners {
		l.SyntaxError(p.sourceName, tok.Source.Line, tok.Source.Column, msg)
	}
	p.numErrors++
}

// expect consumes the current default-channel token if its type matches
// tt and returns it. Otherwise reports an "expected X but found Y"
// error using the current token's source location, returns a zero
// Token, and DOES NOT advance (caller decides whether to sync).
func (p *Parser) expect(tt lexer.TokenType) lexer.Token {
	if p.ts.LA(1) != tt {
		cur := p.ts.LT(1)
		p.reportError(cur, "expected %s but found %s", tt, cur.Type)
		return lexer.Token{}
	}
	cur := *p.ts.LT(1)
	p.ts.Consume()
	return cur
}

// consumeIf consumes and returns the current token (with ok=true) if
// its type matches tt; otherwise leaves the stream untouched and
// returns ok=false. Use when a production allows an optional token.
func (p *Parser) consumeIf(tt lexer.TokenType) (lexer.Token, bool) {
	if p.ts.LA(1) != tt {
		return lexer.Token{}, false
	}
	cur := *p.ts.LT(1)
	p.ts.Consume()
	return cur, true
}

// consume unconditionally returns the current token and advances. Used
// where dispatch already verified LA(1) — typical at the top of a
// production after a switch.
func (p *Parser) consume() lexer.Token {
	cur := *p.ts.LT(1)
	p.ts.Consume()
	return cur
}

// syncToStatement consumes tokens until LA(1) is a statement-boundary
// token (SEMICOLON, LBRACE, RBRACE, LBRACK, or EOF). If sync landed on
// SEMICOLON, consume it (assume the statement is done) and return.
//
// NAI-204-D-PARSER-PANIC-SYNC: panic-mode sync — see package doc.
func (p *Parser) syncToStatement() {
	for {
		switch p.ts.LA(1) {
		case lexer.SEMICOLON:
			p.ts.Consume()
			return
		case lexer.LBRACE, lexer.RBRACE, lexer.LBRACK, lexer.EOF:
			return
		}
		p.ts.Consume()
	}
}

// spanOf builds a NodeSourceLocation covering from start's beginning to
// stop's end. Both inputs are 1-based; the lexer hands tokens already
// in 1-based, so no offset arithmetic is applied here.
func spanOf(start, stop *lexer.Token) lexer.NodeSourceLocation {
	if start == nil && stop == nil {
		return lexer.NodeSourceLocation{}
	}
	if start == nil {
		start = stop
	}
	if stop == nil {
		stop = start
	}
	return lexer.NodeSourceLocation{
		Name:      start.Source.Name,
		Line:      start.Source.Line,
		Column:    start.Source.Column,
		EndLine:   stop.Source.EndLine,
		EndColumn: stop.Source.EndColumn,
	}
}
