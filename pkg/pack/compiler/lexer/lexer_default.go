package lexer

import "strings"

// nextDefault is the DEFAULT-mode token dispatcher. Returns the next
// token (possibly hidden-channel). EOF when pos >= len(input).
//
// Algorithm: longest-match-then-declaration-order. Routes by first-char
// equivalence class to candidate rules, then materializes longest-match
// where the class admits multiple rules. See spec §5.5 for full
// dispatch table.
//
// This file lands incrementally:
//
//	T5  — whitespace, line comment, block comment (this commit)
//	T6  — symbols (single+multi char) + CHAR_LITERAL
//	T7  — identifier + keyword + suffix-keyword (§5.5.2)
//	T8  — numeric-or-identifier dispatch (§5.5.1)
//	T10 — String-mode wiring
//	T11 — GT semantic action (depth>0 → STRING_EXPR_END + popMode)
//	T11 — error recovery (unterminated comment, etc.)
func (lx *Lexer) nextDefault() Token {
	c := lx.input[lx.pos]

	// Whitespace run (.g4:75)
	if isWhitespace(c) {
		return lx.consumeWhitespace()
	}

	// Comments (.g4:59-60)
	if c == '/' && lx.pos+1 < len(lx.input) {
		next := lx.input[lx.pos+1]
		if next == '/' {
			return lx.consumeLineComment()
		}
		if next == '*' {
			return lx.consumeBlockComment()
		}
	}

	// CHAR_LITERAL (.g4:55)
	if c == '\'' {
		return lx.consumeCharLiteral()
	}

	// QUOTE_OPEN — String mode entry (.g4:73). T9 wires the body;
	// for T6, just emit the token and push mode.
	if c == '"' {
		return lx.consumeQuoteOpen()
	}

	// Symbols with multi-char extensions (longest-match within family)
	if c == '>' {
		return lx.consumeGt()
	}
	if c == '<' {
		return lx.consumeLt()
	}
	if c == '.' {
		// .% (DOTMOD, line 21) or . (single-char IDENTIFIER — handled in T7).
		if lx.pos+1 < len(lx.input) && lx.input[lx.pos+1] == '%' {
			start := lx.pos
			startLn, startCol := lx.line, lx.col
			lx.advance(2)
			return lx.makeToken(DOTMOD, start, lx.pos-1, ".%", startLn, startCol+1, lx.line, lx.col)
		}
		// fall through to identifier path below
	}

	// COLON — special-case because it's in identifier class AND has its
	// own token. Longest-match: ':' followed by an id-class char starts
	// an IDENTIFIER run (.g4:74 beats :10 when the run is longer).
	// Bare ':' falls to singleCharSymbol and emits COLON.
	if c == ':' {
		if lx.pos+1 < len(lx.input) && isIdentChar(lx.input[lx.pos+1]) {
			// Fall through to identifier dispatch below.
		} else {
			start := lx.pos
			startLn, startCol := lx.line, lx.col
			lx.advance(1)
			return lx.makeToken(COLON, start, start, ":", startLn, startCol+1, startLn, startCol+1)
		}
	}

	// Single-char symbols (no id-class overlap)
	if tt, ok := singleCharSymbol(c); ok {
		start := lx.pos
		startLn, startCol := lx.line, lx.col
		lx.advance(1)
		return lx.makeToken(tt, start, start, string(c), startLn, startCol+1, startLn, startCol+1)
	}

	// PLUS / MINUS — special-case because they're in identifier class
	// AND have their own token. For T6: if standalone (no id-class
	// follow), emit single-char. T7+ extends with id-path routing.
	if c == '+' {
		if lx.pos+1 < len(lx.input) && isIdentChar(lx.input[lx.pos+1]) {
			// Fall through to identifier dispatch — T7 lands the code.
		} else {
			start := lx.pos
			startLn, startCol := lx.line, lx.col
			lx.advance(1)
			return lx.makeToken(PLUS, start, start, "+", startLn, startCol+1, startLn, startCol+1)
		}
	}
	if c == '-' {
		// If followed by digit → INTEGER_LITERAL with leading `-` (T8).
		// Otherwise MINUS.
		if lx.pos+1 < len(lx.input) && isDigit(lx.input[lx.pos+1]) {
			// Fall through to numeric dispatch — T8 lands the code.
		} else {
			start := lx.pos
			startLn, startCol := lx.line, lx.col
			lx.advance(1)
			return lx.makeToken(MINUS, start, start, "-", startLn, startCol+1, startLn, startCol+1)
		}
	}

	// Numeric-or-identifier dispatch (.g4 INTEGER/HEX/BIN/COORD/MAPZONE
	// + IDENTIFIER overlap on digit start). Per §5.5.1 worked-example
	// table: tentatively try each numeric candidate + identifier, pick
	// longest then declaration order.
	//
	// Also catches the `-`+digit fall-through from the MINUS branch above
	// (that branch does not return when pos+1 is a digit).
	if isDigit(c) || (c == '-' && lx.pos+1 < len(lx.input) && isDigit(lx.input[lx.pos+1])) {
		return lx.consumeNumericOrIdentifier()
	}

	// Identifier-class dispatch (.g4 IDENTIFIER charclass [a-zA-Z0-9_+.:]).
	// Per §5.5 steps 4-8: digit starts route to numeric dispatch above;
	// letter/_ starts route to identifier-or-keyword here. Symbol-class
	// chars (+ . :) are id-class too; the prior checks already routed them.
	if isLetterOrUnderscore(c) || c == '+' || c == '.' || c == ':' {
		return lx.consumeIdentifierOrKeyword()
	}

	// Unrecognized — advance past the byte so the lexer makes progress.
	lx.advance(1)
	return lx.makeToken(EOF, lx.pos, lx.pos-1, "", lx.line, lx.col+1, lx.line, lx.col+1)
}

// isWhitespace mirrors .g4 WHITESPACE charclass: [ \t\n\r].
func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// consumeQuoteOpen emits QUOTE_OPEN, increments depth, pushes
// modeString. The String-mode body is wired in T9.
func (lx *Lexer) consumeQuoteOpen() Token {
	start := lx.pos
	startLn, startCol := lx.line, lx.col
	lx.advance(1)
	lx.depth++
	lx.pushMode(modeString)
	return lx.makeToken(QUOTE_OPEN, start, start, `"`, startLn, startCol+1, startLn, startCol+1)
}

// consumeGt handles `>` — GTE if followed by `=`, otherwise GT. The
// semantic action (.g4:31 — if depth>0, retype to STRING_EXPR_END and
// popMode) is added in T10.
func (lx *Lexer) consumeGt() Token {
	start := lx.pos
	startLn, startCol := lx.line, lx.col
	if lx.pos+1 < len(lx.input) && lx.input[lx.pos+1] == '=' {
		lx.advance(2)
		return lx.makeToken(GTE, start, start+1, ">=", startLn, startCol+1, startLn, startCol+2)
	}
	lx.advance(1)
	// T10 inserts the depth>0 retype here.
	return lx.makeToken(GT, start, start, ">", startLn, startCol+1, startLn, startCol+1)
}

// consumeLt handles `<` — LTE if followed by `=`, otherwise LT. In
// DEFAULT mode the bare `<` is NOT STRING_EXPR_START (that lives in
// String mode only).
func (lx *Lexer) consumeLt() Token {
	start := lx.pos
	startLn, startCol := lx.line, lx.col
	if lx.pos+1 < len(lx.input) && lx.input[lx.pos+1] == '=' {
		lx.advance(2)
		return lx.makeToken(LTE, start, start+1, "<=", startLn, startCol+1, startLn, startCol+2)
	}
	lx.advance(1)
	return lx.makeToken(LT, start, start, "<", startLn, startCol+1, startLn, startCol+1)
}

// singleCharSymbol maps an unambiguous single-byte symbol char to its
// TokenType. Returns ok=false for chars that belong to multi-char
// families (>, <, ., +, -) or to other dispatch paths.
func singleCharSymbol(c byte) (TokenType, bool) {
	switch c {
	case '(':
		return LPAREN, true
	case ')':
		return RPAREN, true
	case ';':
		return SEMICOLON, true
	case ',':
		return COMMA, true
	case '[':
		return LBRACK, true
	case ']':
		return RBRACK, true
	case '{':
		return LBRACE, true
	case '}':
		return RBRACE, true
	case '*':
		return MUL, true
	case '/':
		return DIV, true
	case '%':
		return MOD, true
	case '&':
		return AND, true
	case '|':
		return OR, true
	case '=':
		return EQ, true
	case '!':
		return EXCL, true
	case '$':
		return DOLLAR, true
	case '^':
		return CARET, true
	case '~':
		return TILDE, true
	case '@':
		return AT, true
	}
	return 0, false
}

// consumeCharLiteral handles `'X'` where X is either a single non-
// special char or one of the CharEscapeSequence escapes (\\ or \').
// On unterminated, returns CHAR_LITERAL with partial text — error
// recovery + listener fire are wired in T11 (NAI-203-D-LEXER-ERROR-
// RECOVERY).
func (lx *Lexer) consumeCharLiteral() Token {
	start := lx.pos
	startLn, startCol := lx.line, lx.col
	// Consume opening `'`
	lx.advance(1)
	if lx.pos >= len(lx.input) {
		// Unterminated — T11 wires reportError.
		return lx.makeToken(CHAR_LITERAL, start, lx.pos-1, lx.input[start:lx.pos], startLn, startCol+1, lx.line, lx.col)
	}
	// One inner char: either `\\` `\'` (escape) or any byte that isn't
	// `'` `\\` `\r` `\n` (.g4 CharEscapeSequence + ~['\\\r\n]).
	c := lx.input[lx.pos]
	if c == '\\' && lx.pos+1 < len(lx.input) {
		next := lx.input[lx.pos+1]
		if next == '\\' || next == '\'' {
			lx.advance(2)
		} else {
			// Invalid escape — consume what we can; T11 wires error.
			lx.advance(1)
		}
	} else if c != '\'' && c != '\r' && c != '\n' {
		lx.advance(1)
	}
	// Expect closing `'`
	if lx.pos < len(lx.input) && lx.input[lx.pos] == '\'' {
		lx.advance(1)
	}
	stop := lx.pos - 1
	endLn, endCol := lx.lineColAt(stop)
	return lx.makeToken(CHAR_LITERAL, start, stop, lx.input[start:lx.pos], startLn, startCol+1, endLn, endCol+1)
}

// isDigit mirrors .g4 fragment Digit: [0-9].
func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// isLetterOrUnderscore returns true for the id-class start chars
// [a-zA-Z_].
func isLetterOrUnderscore(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

// consumeIdentifierOrKeyword implements §5.5.2 of the spec. Consumes
// the maximal IDENTIFIER run, then classifies:
//  1. Exact-match keyword/literal (declaration order line 37-56 in .g4
//     wins ties over IDENTIFIER line 74).
//  2. Suffix-pattern keyword (TYPE_ARRAY / DEF_TYPE / SWITCH_TYPE,
//     lines 44-46).
//  3. Else IDENTIFIER.
func (lx *Lexer) consumeIdentifierOrKeyword() Token {
	start := lx.pos
	startLn, startCol := lx.line, lx.col
	for lx.pos < len(lx.input) && isIdentChar(lx.input[lx.pos]) {
		lx.advance(1)
	}
	stop := lx.pos - 1
	endLn, endCol := lx.lineColAt(stop)
	text := lx.input[start:lx.pos]

	tt := classifyIdentifierRun(text)
	return lx.makeToken(tt, start, stop, text, startLn, startCol+1, endLn, endCol+1)
}

// classifyIdentifierRun applies §5.5.2 disambiguation. Pure function on
// the matched text — no Lexer state.
func classifyIdentifierRun(text string) TokenType {
	switch text {
	case "if":
		return IF
	case "else":
		return ELSE
	case "while":
		return WHILE
	case "case":
		return CASE
	case "default":
		return DEFAULT
	case "return":
		return RETURN
	case "calc":
		return CALC
	case "true", "false":
		return BOOLEAN_LITERAL
	case "null":
		return NULL_LITERAL
	}

	if strings.HasPrefix(text, "def_") && len(text) > 4 {
		return DEF_TYPE
	}
	if strings.HasPrefix(text, "switch_") && len(text) > 7 {
		return SWITCH_TYPE
	}
	if strings.HasSuffix(text, "array") && len(text) > 5 {
		return TYPE_ARRAY
	}

	return IDENTIFIER
}

// isIdentChar mirrors .g4 IDENTIFIER charclass: [a-zA-Z0-9_+.:].
func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_' || c == '+' || c == '.' || c == ':'
}

// consumeNumericOrIdentifier implements §5.5.1 of the spec. Tentatively
// matches every candidate (HEX, BIN, COORD, MAPZONE, INTEGER, IDENTIFIER)
// at lx.pos, picks the longest, breaks ties by .g4 declaration order
// (TokenType comparison, lower value = earlier declaration = wins).
//
// Precondition: lx.input[lx.pos] is [0-9] OR `-` followed by [0-9].
func (lx *Lexer) consumeNumericOrIdentifier() Token {
	start := lx.pos
	startLn, startCol := lx.line, lx.col

	// IDENTIFIER candidate — maximal id-class run starting at pos.
	// `-` is NOT in id class, so if pos starts with `-`, IDENTIFIER
	// candidate length is zero.
	identLen := 0
	if lx.input[lx.pos] != '-' {
		idEnd := lx.pos
		for idEnd < len(lx.input) && isIdentChar(lx.input[idEnd]) {
			idEnd++
		}
		identLen = idEnd - lx.pos
	}

	hexLen := matchHex(lx.input, lx.pos)
	binLen := matchBin(lx.input, lx.pos)
	coordLen := matchCoord(lx.input, lx.pos)
	mapzoneLen := matchMapzone(lx.input, lx.pos)
	integerLen := matchInteger(lx.input, lx.pos)

	type cand struct {
		tt     TokenType
		length int
	}
	// Listed in .g4 declaration order; tie-breaking prefers the first
	// entry with the winning length (lower TokenType value = earlier decl).
	candidates := []cand{
		{INTEGER_LITERAL, integerLen},
		{HEX_LITERAL, hexLen},
		{BIN_LITERAL, binLen},
		{COORD_LITERAL, coordLen},
		{MAPZONE_LITERAL, mapzoneLen},
		{IDENTIFIER, identLen},
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.length > best.length || (c.length == best.length && c.tt < best.tt) {
			best = c
		}
	}

	for i := 0; i < best.length; i++ {
		lx.advance(1)
	}
	stop := lx.pos - 1
	endLn, endCol := lx.lineColAt(stop)
	return lx.makeToken(best.tt, start, stop, lx.input[start:lx.pos], startLn, startCol+1, endLn, endCol+1)
}

// matchHex returns the byte-length of a HEX_LITERAL starting at pos, or
// 0 if none. Pattern: 0[xX][0-9a-fA-F]+.
func matchHex(input string, pos int) int {
	if pos+1 >= len(input) || input[pos] != '0' {
		return 0
	}
	if input[pos+1] != 'x' && input[pos+1] != 'X' {
		return 0
	}
	end := pos + 2
	for end < len(input) && isHexDigit(input[end]) {
		end++
	}
	if end == pos+2 {
		// No hex digits after the prefix — not a valid HEX_LITERAL.
		return 0
	}
	return end - pos
}

// matchBin returns the byte-length of a BIN_LITERAL starting at pos, or
// 0 if none. Pattern: 0[bB][01]+.
func matchBin(input string, pos int) int {
	if pos+1 >= len(input) || input[pos] != '0' {
		return 0
	}
	if input[pos+1] != 'b' && input[pos+1] != 'B' {
		return 0
	}
	end := pos + 2
	for end < len(input) && (input[end] == '0' || input[end] == '1') {
		end++
	}
	if end == pos+2 {
		// No binary digits after the prefix — not a valid BIN_LITERAL.
		return 0
	}
	return end - pos
}

// matchInteger returns the byte-length of an INTEGER_LITERAL starting at
// pos, or 0 if none. Pattern: -? [0-9]+.
func matchInteger(input string, pos int) int {
	p := pos
	if p < len(input) && input[p] == '-' {
		p++
	}
	digitStart := p
	for p < len(input) && isDigit(input[p]) {
		p++
	}
	if p == digitStart {
		// No digits — not a valid INTEGER_LITERAL.
		return 0
	}
	return p - pos
}

// matchCoord returns the byte-length of a COORD_LITERAL (5-group
// digit run separated by underscores) starting at pos, or 0 if none.
func matchCoord(input string, pos int) int {
	return matchUnderscoreGroups(input, pos, 5)
}

// matchMapzone returns the byte-length of a MAPZONE_LITERAL (3-group
// digit run) starting at pos, or 0 if none.
func matchMapzone(input string, pos int) int {
	return matchUnderscoreGroups(input, pos, 3)
}

// matchUnderscoreGroups returns the byte-length of exactly n digit-groups
// separated by underscores, or 0 if the input at pos doesn't match.
// Each group must contain at least one digit.
func matchUnderscoreGroups(input string, pos int, n int) int {
	p := pos
	for i := 0; i < n; i++ {
		groupStart := p
		for p < len(input) && isDigit(input[p]) {
			p++
		}
		if p == groupStart {
			// Empty digit group — no match.
			return 0
		}
		if i < n-1 {
			// Expect underscore separator between groups.
			if p >= len(input) || input[p] != '_' {
				return 0
			}
			p++
		}
	}
	return p - pos
}

// isHexDigit returns true for [0-9a-fA-F].
func isHexDigit(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// consumeWhitespace consumes [ \t\n\r]+ from pos and emits one
// WHITESPACE token on the hidden channel.
func (lx *Lexer) consumeWhitespace() Token {
	start := lx.pos
	startLn, startCol := lx.line, lx.col
	for lx.pos < len(lx.input) && isWhitespace(lx.input[lx.pos]) {
		lx.advance(1)
	}
	stop := lx.pos - 1
	endLn, endCol := lx.lineColAt(stop)
	t := lx.makeToken(WHITESPACE, start, stop, lx.input[start:lx.pos], startLn, startCol+1, endLn, endCol+1)
	t.Channel = ChannelHidden
	return t
}

// consumeLineComment consumes // ... (\n | EOF) and emits one
// LINE_COMMENT token on the hidden channel. The trailing \n is INCLUDED
// in Text per .g4 rule `'//' .*? ('\n' | EOF)`.
func (lx *Lexer) consumeLineComment() Token {
	start := lx.pos
	startLn, startCol := lx.line, lx.col
	// Consume `//`
	lx.advance(2)
	// Consume to first \n inclusive or to EOF
	for lx.pos < len(lx.input) {
		c := lx.input[lx.pos]
		lx.advance(1)
		if c == '\n' {
			break
		}
	}
	stop := lx.pos - 1
	endLn, endCol := lx.lineColAt(stop)
	t := lx.makeToken(LINE_COMMENT, start, stop, lx.input[start:lx.pos], startLn, startCol+1, endLn, endCol+1)
	t.Channel = ChannelHidden
	return t
}

// consumeBlockComment consumes /* ... */ (non-greedy) and emits one
// BLOCK_COMMENT token on the hidden channel. Unterminated comment
// (EOF before */) emits SyntaxError and emits the partial range
// (NAI-203-D-LEXER-ERROR-RECOVERY — formalized in T11).
func (lx *Lexer) consumeBlockComment() Token {
	start := lx.pos
	startLn, startCol := lx.line, lx.col
	// Consume `/*`
	lx.advance(2)
	// Scan for `*/`
	closed := false
	for lx.pos+1 < len(lx.input) {
		if lx.input[lx.pos] == '*' && lx.input[lx.pos+1] == '/' {
			lx.advance(2)
			closed = true
			break
		}
		lx.advance(1)
	}
	if !closed {
		// Consume any tail (1 byte left) so the BLOCK_COMMENT spans
		// to EOF.
		for lx.pos < len(lx.input) {
			lx.advance(1)
		}
		// Error recovery firing is deferred to T11 — left silent here
		// to keep T5 happy-path scoped. T11 wires reportError + a
		// NAI-203-D-LEXER-ERROR-RECOVERY tag.
	}
	stop := lx.pos - 1
	endLn, endCol := lx.lineColAt(stop)
	t := lx.makeToken(BLOCK_COMMENT, start, stop, lx.input[start:lx.pos], startLn, startCol+1, endLn, endCol+1)
	t.Channel = ChannelHidden
	return t
}

// advance consumes n bytes from input, updating pos/line/col. Handles
// \n (line++ / col=0), \r\n (treated as one line ending), bare \r
// (line++ / col=0). All other bytes advance col by 1 regardless of
// width (antlr's consume() does the same — tabs and multi-byte UTF8
// count as 1).
func (lx *Lexer) advance(n int) {
	for i := 0; i < n && lx.pos < len(lx.input); i++ {
		c := lx.input[lx.pos]
		switch {
		case c == '\r':
			lx.line++
			lx.col = 0
			lx.pos++
			// Skip \n in \r\n pairs WITHOUT a second line increment.
			if lx.pos < len(lx.input) && lx.input[lx.pos] == '\n' {
				lx.pos++
			}
		case c == '\n':
			lx.line++
			lx.col = 0
			lx.pos++
		default:
			lx.col++
			lx.pos++
		}
	}
}

// lineColAt returns the 1-based line and (effectively 0-based-then-
// adjusted) column of byte offset `at`. Used to compute end-line/end-
// column for multi-line tokens. Re-scans from input start — O(n) but
// called once per multi-line token, fine for compile workload.
func (lx *Lexer) lineColAt(at int) (line, col int) {
	if at < 0 {
		return 1, 0
	}
	line = 1
	col = 0
	for i := 0; i <= at && i < len(lx.input); i++ {
		c := lx.input[i]
		switch c {
		case '\r':
			line++
			col = 0
			// Skip \n in \r\n
			if i+1 < len(lx.input) && lx.input[i+1] == '\n' {
				i++
			}
		case '\n':
			line++
			col = 0
		default:
			col++
		}
	}
	if col > 0 {
		col--
	}
	return line, col
}
