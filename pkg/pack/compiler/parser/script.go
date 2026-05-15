package parser

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

// parseScriptFileBody parses `script* EOF`. Mirrors TS
// AstBuilder.visitScriptFile + ScriptParser.invokeParser.
func (p *Parser) parseScriptFileBody() *ast.ScriptFile {
	startTok := p.ts.LT(1)
	scripts := []*ast.Script{}
	for p.ts.LA(1) != lexer.EOF {
		s := p.parseScript()
		if s == nil {
			// Hard failure — sync to next LBRACK or EOF and try again.
			for p.ts.LA(1) != lexer.LBRACK && p.ts.LA(1) != lexer.EOF {
				p.ts.Consume()
			}
			continue
		}
		scripts = append(scripts, s)
	}
	stopTok := p.ts.LT(1) // EOF token
	return &ast.ScriptFile{
		SrcLoc:  spanOf(startTok, stopTok),
		Scripts: scripts,
	}
}

// parseScript parses one script. Grammar:
//
//	script
//	    : LBRACK trigger=identifier COMMA name=scriptName MUL? RBRACK
//	      ((LPAREN parameterList? RPAREN) (LPAREN typeList? RPAREN)?)?
//	      statement*
//	    ;
//
// Mirrors TS AstBuilder.visitScript.
func (p *Parser) parseScript() *ast.Script {
	if p.ts.LA(1) != lexer.LBRACK {
		p.reportError(p.ts.LT(1), "expected LBRACK (script header) but found %s", p.ts.LA(1))
		return nil
	}
	startTok := p.ts.LT(1)
	p.ts.Consume() // LBRACK

	trigger := p.parseIdentifier()
	if trigger == nil {
		return nil
	}
	if _, ok := p.consumeIf(lexer.COMMA); !ok {
		p.reportError(p.ts.LT(1), "expected COMMA after script trigger but found %s", p.ts.LA(1))
		return nil
	}
	name := p.parseScriptName()
	if name == nil {
		return nil
	}
	_, isStar := p.consumeIf(lexer.MUL)
	if _, ok := p.consumeIf(lexer.RBRACK); !ok {
		p.reportError(p.ts.LT(1), "expected RBRACK to close script header but found %s", p.ts.LA(1))
		return nil
	}

	var parameters []*ast.Parameter
	var returnTokens []*ast.Token

	// Optional `(parameterList?) (typeList?)?` block — both lists are
	// LPAREN-delimited; second LPAREN only exists when first one was
	// present (per grammar).
	if p.ts.LA(1) == lexer.LPAREN {
		p.ts.Consume() // LPAREN
		parameters = p.parseParameterList()
		if parameters == nil {
			return nil
		}
		if _, ok := p.consumeIf(lexer.RPAREN); !ok {
			p.reportError(p.ts.LT(1), "expected RPAREN to close parameter list but found %s", p.ts.LA(1))
			return nil
		}
		if p.ts.LA(1) == lexer.LPAREN {
			p.ts.Consume() // LPAREN (typeList)
			returnTokens = p.parseTypeList()
			if returnTokens == nil {
				return nil
			}
			if _, ok := p.consumeIf(lexer.RPAREN); !ok {
				p.reportError(p.ts.LT(1), "expected RPAREN to close type list but found %s", p.ts.LA(1))
				return nil
			}
		}
	}

	statements := []ast.Statement{}
	// statement* — consume until next LBRACK (next script) or EOF.
	for p.ts.LA(1) != lexer.LBRACK && p.ts.LA(1) != lexer.EOF {
		st := p.parseStatement()
		if st == nil {
			// parseStatement either already reported and synced, or hit
			// a hard error; sync and retry.
			p.syncToStatement()
			continue
		}
		statements = append(statements, st)
	}

	stopTok := p.ts.LT(1)
	return &ast.Script{
		SrcLoc:       spanOf(startTok, stopTok),
		Trigger:      trigger,
		Name:         name,
		IsStar:       isStar,
		Parameters:   parameters,
		ReturnTokens: returnTokens,
		Statements:   statements,
	}
}

// parseScriptName parses `identifier (identifier)*`. Mirrors TS
// AstBuilder.visitScriptName: a single identifier is returned as-is;
// multiple identifiers concatenate text with single-space separator,
// span the full range.
func (p *Parser) parseScriptName() *ast.Identifier {
	first := p.parseIdentifier()
	if first == nil {
		return nil
	}
	if !isIdentifierStart(p.ts.LA(1)) {
		return first
	}
	last := first
	text := first.Text
	for isIdentifierStart(p.ts.LA(1)) {
		nxt := p.parseIdentifier()
		if nxt == nil {
			return nil
		}
		text += " " + nxt.Text
		last = nxt
	}
	return &ast.Identifier{
		SrcLoc: lexer.NodeSourceLocation{
			Name:      first.SrcLoc.Name,
			Line:      first.SrcLoc.Line,
			Column:    first.SrcLoc.Column,
			EndLine:   last.SrcLoc.EndLine,
			EndColumn: last.SrcLoc.EndColumn,
		},
		Text: text,
	}
}

// parseParameterList parses zero-or-more `parameter (COMMA parameter)*`
// inside the LPAREN/RPAREN — caller already consumed the LPAREN.
// Returns a non-nil possibly-empty slice when LPAREN-RPAREN matched.
func (p *Parser) parseParameterList() []*ast.Parameter {
	out := []*ast.Parameter{}
	if p.ts.LA(1) == lexer.RPAREN {
		return out
	}
	first := p.parseParameter()
	if first == nil {
		return nil
	}
	out = append(out, first)
	for p.ts.LA(1) == lexer.COMMA {
		p.ts.Consume() // COMMA
		nxt := p.parseParameter()
		if nxt == nil {
			return nil
		}
		out = append(out, nxt)
	}
	return out
}

// parseParameter parses `(IDENTIFIER | TYPE_ARRAY) DOLLAR advancedIdentifier`.
// Mirrors TS AstBuilder.visitParameter.
func (p *Parser) parseParameter() *ast.Parameter {
	if la := p.ts.LA(1); la != lexer.IDENTIFIER && la != lexer.TYPE_ARRAY {
		p.reportError(p.ts.LT(1), "expected type (IDENTIFIER or TYPE_ARRAY) in parameter but found %s", la)
		return nil
	}
	typeTok := p.consume()
	if _, ok := p.consumeIf(lexer.DOLLAR); !ok {
		p.reportError(p.ts.LT(1), "expected DOLLAR after parameter type but found %s", p.ts.LA(1))
		return nil
	}
	name := p.parseAdvancedIdentifier()
	if name == nil {
		return nil
	}
	return &ast.Parameter{
		SrcLoc: lexer.NodeSourceLocation{
			Name:      typeTok.Source.Name,
			Line:      typeTok.Source.Line,
			Column:    typeTok.Source.Column,
			EndLine:   name.SrcLoc.EndLine,
			EndColumn: name.SrcLoc.EndColumn,
		},
		TypeToken: &ast.Token{SrcLoc: typeTok.Source, Text: typeTok.Text},
		Name:      name,
	}
}

// parseTypeList parses `IDENTIFIER (COMMA IDENTIFIER)*` inside
// LPAREN/RPAREN — caller already consumed the LPAREN. Returns a
// non-nil possibly-empty slice.
func (p *Parser) parseTypeList() []*ast.Token {
	out := []*ast.Token{}
	if p.ts.LA(1) == lexer.RPAREN {
		return out
	}
	if p.ts.LA(1) != lexer.IDENTIFIER {
		p.reportError(p.ts.LT(1), "expected IDENTIFIER in type list but found %s", p.ts.LA(1))
		return nil
	}
	first := p.consume()
	out = append(out, &ast.Token{SrcLoc: first.Source, Text: first.Text})
	for p.ts.LA(1) == lexer.COMMA {
		p.ts.Consume() // COMMA
		if p.ts.LA(1) != lexer.IDENTIFIER {
			p.reportError(p.ts.LT(1), "expected IDENTIFIER after COMMA in type list but found %s", p.ts.LA(1))
			return nil
		}
		nxt := p.consume()
		out = append(out, &ast.Token{SrcLoc: nxt.Source, Text: nxt.Text})
	}
	return out
}

// parseIdentifier parses the `identifier` rule from the grammar:
//
//	identifier
//	    : IDENTIFIER | HEX_LITERAL | BOOLEAN_LITERAL | NULL_LITERAL
//	    | COORD_LITERAL | MAPZONE_LITERAL | TYPE_ARRAY | SWITCH_TYPE
//	    | DEF_TYPE | DEFAULT
//	    ;
//
// All ten token types contribute their .Text to the resulting Identifier.
func (p *Parser) parseIdentifier() *ast.Identifier {
	if !isIdentifierStart(p.ts.LA(1)) {
		p.reportError(p.ts.LT(1), "expected identifier but found %s", p.ts.LA(1))
		return nil
	}
	tok := p.consume()
	return &ast.Identifier{SrcLoc: tok.Source, Text: tok.Text}
}

// parseAdvancedIdentifier parses the `advancedIdentifier` rule:
//
//	advancedIdentifier
//	    : identifier
//	    | INTEGER_LITERAL | IF | ELSE | WHILE | RETURN | CALC
//	    ;
//
// Same as parseIdentifier plus the extra keyword tokens.
func (p *Parser) parseAdvancedIdentifier() *ast.Identifier {
	if !isAdvancedIdentifierStart(p.ts.LA(1)) {
		p.reportError(p.ts.LT(1), "expected (advanced) identifier but found %s", p.ts.LA(1))
		return nil
	}
	tok := p.consume()
	return &ast.Identifier{SrcLoc: tok.Source, Text: tok.Text}
}

// isIdentifierStart reports whether the given token type is a valid
// first token for the `identifier` production.
func isIdentifierStart(tt lexer.TokenType) bool {
	switch tt {
	case lexer.IDENTIFIER,
		lexer.HEX_LITERAL,
		lexer.BOOLEAN_LITERAL,
		lexer.NULL_LITERAL,
		lexer.COORD_LITERAL,
		lexer.MAPZONE_LITERAL,
		lexer.TYPE_ARRAY,
		lexer.SWITCH_TYPE,
		lexer.DEF_TYPE,
		lexer.DEFAULT:
		return true
	}
	return false
}

// isAdvancedIdentifierStart reports whether the given token type is a
// valid first token for `advancedIdentifier`.
func isAdvancedIdentifierStart(tt lexer.TokenType) bool {
	if isIdentifierStart(tt) {
		return true
	}
	switch tt {
	case lexer.INTEGER_LITERAL, lexer.IF, lexer.ELSE, lexer.WHILE, lexer.RETURN, lexer.CALC:
		return true
	}
	return false
}

// parseStatement is implemented by T6 — for T5, it's a placeholder
// that just reports "statement parsing not yet supported".
//
// (T6 deletes this stub and provides the real dispatch.)
func (p *Parser) parseStatement() ast.Statement {
	p.reportError(p.ts.LT(1), "statement parsing unimplemented in T5 (token: %s)", p.ts.LA(1))
	return nil
}
