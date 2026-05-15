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

// parseScriptFileBody is a placeholder until T5 lands; returns an empty
// ScriptFile when the token stream starts at EOF, otherwise reports
// "expected EOF" once and returns nil.
//
// (T5 replaces this with the real implementation.)
func (p *Parser) parseScriptFileBody() *ast.ScriptFile {
	if p.ts.LA(1) == lexer.EOF {
		return &ast.ScriptFile{
			SrcLoc: lexer.NodeSourceLocation{Name: p.sourceName, Line: 1, Column: 1, EndLine: 1, EndColumn: 1},
		}
	}
	p.reportError(p.ts.LT(1), "expected EOF (parser entry rules unimplemented in T4 skeleton)")
	return nil
}

// parseScript is a placeholder until T5 lands.
func (p *Parser) parseScript() *ast.Script {
	p.reportError(p.ts.LT(1), "parseScript unimplemented in T4 skeleton")
	return nil
}

// parseClientScriptBody is a placeholder until T10 lands.
func (p *Parser) parseClientScriptBody() *ast.ClientScriptExpression {
	p.reportError(p.ts.LT(1), "parseClientScriptBody unimplemented in T4 skeleton")
	return nil
}
