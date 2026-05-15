// Package parser ports RuneScriptTS's parser layer
// (src/antlr/RuneScriptParser.g4 + src/parser/parser/AstBuilder.ts +
// src/parser/parser/ScriptParser.ts, HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47)
// to a hand-written Go recursive-descent parser over NAI-203's
// lexer.TokenStream.
//
// NAI-204-D-PARSER-PANIC-SYNC: error recovery uses panic-mode sync at
// statement boundaries (SEMICOLON, LBRACE, RBRACE, LBRACK, EOF), not
// ANTLR's DefaultErrorStrategy.
//
// Lazy lexer drain: AddErrorListener registers listeners that flow to
// both the lexer stage (via Lexer.AddErrorListener at drain time) and
// the parser stage. The first Parse* call triggers drain.
package parser

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

// Parser holds one source's parse state. One Parser = one goroutine.
type Parser struct {
	lx         *lexer.Lexer
	ts         *lexer.TokenStream // nil until ensureStream() called
	sourceName string
	listeners  []lexer.ErrorListener
	numErrors  int
}

// NewScriptFileParser constructs a Parser positioned at the scriptFile
// entry rule.
func NewScriptFileParser(input, sourceName string) *Parser {
	return &Parser{
		lx:         lexer.NewLexer(input, sourceName),
		sourceName: sourceName,
	}
}

// NewScriptParser constructs a Parser positioned at the single-script
// entry rule (parses one `[trigger,name]` block).
func NewScriptParser(input, sourceName string) *Parser {
	return &Parser{
		lx:         lexer.NewLexer(input, sourceName),
		sourceName: sourceName,
	}
}

// NewClientScriptParser constructs a Parser positioned at the
// clientScript entry rule (one ClientScript reference, used for
// e.g. cc_setonop).
func NewClientScriptParser(input, sourceName string) *Parser {
	return &Parser{
		lx:         lexer.NewLexer(input, sourceName),
		sourceName: sourceName,
	}
}

// AddErrorListener registers l for both lexer-stage and parser-stage
// syntax errors. Must be called before the first Parse* invocation to
// catch lexer-stage errors. Listeners added after drain still receive
// subsequent parser-stage errors via reportError, but miss any
// lexer-stage errors that already fired during ensureStream.
func (p *Parser) AddErrorListener(l lexer.ErrorListener) {
	p.listeners = append(p.listeners, l)
}

// RemoveErrorListeners drops all registered listeners.
func (p *Parser) RemoveErrorListeners() {
	p.listeners = nil
}

// ensureStream lazily drains the lexer into a TokenStream, wiring all
// registered listeners onto the lexer first so lexer-stage SyntaxError
// callbacks reach them.
func (p *Parser) ensureStream() {
	if p.ts != nil {
		return
	}
	for _, l := range p.listeners {
		p.lx.AddErrorListener(l)
	}
	p.ts = lexer.NewTokenStream(p.lx)
}

// ParseScriptFile parses the input as a full scriptFile production.
// Returns nil if at least one syntax error was reported (parity with
// TS ScriptParser.invokeParser's "numberOfSyntaxErrors > 0 ⇒ return null").
func (p *Parser) ParseScriptFile() *ast.ScriptFile {
	p.ensureStream()
	sf := p.parseScriptFileBody()
	if p.numErrors > 0 {
		return nil
	}
	return sf
}

// ParseScript parses the input as a single script production.
func (p *Parser) ParseScript() *ast.Script {
	p.ensureStream()
	s := p.parseScript()
	if p.numErrors > 0 {
		return nil
	}
	return s
}

// ParseClientScript parses the input as a single clientScript expression.
func (p *Parser) ParseClientScript() *ast.ClientScriptExpression {
	p.ensureStream()
	c := p.parseClientScriptBody()
	if p.numErrors > 0 {
		return nil
	}
	return c
}

// parseClientScriptBody parses the standalone `clientScript` grammar
// entry rule:
//
//	clientScript
//	    : identifier (LPAREN args=expressionList? RPAREN)?
//	      (LBRACE triggers=expressionList? RBRACE)? EOF
//	    ;
//
// Mirrors TS AstBuilder.visitClientScript.
func (p *Parser) parseClientScriptBody() *ast.ClientScriptExpression {
	startTok := p.ts.LT(1)
	name := p.parseIdentifier()
	if name == nil {
		return nil
	}

	var args []ast.Expression
	if _, ok := p.consumeIf(lexer.LPAREN); ok {
		args = p.parseExpressionList()
		if args == nil {
			return nil
		}
		if _, ok := p.consumeIf(lexer.RPAREN); !ok {
			p.reportError(p.ts.LT(1), "expected RPAREN to close clientscript args but found %s", p.ts.LA(1))
			return nil
		}
	} else {
		args = []ast.Expression{}
	}

	var triggers []ast.Expression
	if _, ok := p.consumeIf(lexer.LBRACE); ok {
		triggers = p.parseTransmitList()
		if triggers == nil {
			return nil
		}
		if _, ok := p.consumeIf(lexer.RBRACE); !ok {
			p.reportError(p.ts.LT(1), "expected RBRACE to close clientscript triggers but found %s", p.ts.LA(1))
			return nil
		}
	} else {
		triggers = []ast.Expression{}
	}

	if p.ts.LA(1) != lexer.EOF {
		p.reportError(p.ts.LT(1), "expected EOF after clientscript but found %s", p.ts.LA(1))
		return nil
	}
	endTok := p.ts.LT(1) // EOF
	return &ast.ClientScriptExpression{
		SrcLoc:       spanOf(startTok, endTok),
		Name:         name,
		Arguments:    args,
		TransmitList: triggers,
	}
}

// parseTransmitList parses `expression (COMMA expression)*` stopping at
// RBRACE. Returns a non-nil possibly-empty slice. Mirrors the
// expressionList grammar rule but terminates on RBRACE instead of
// RPAREN (used exclusively by parseClientScriptBody's trigger list).
func (p *Parser) parseTransmitList() []ast.Expression {
	out := []ast.Expression{}
	if p.ts.LA(1) == lexer.RBRACE {
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
