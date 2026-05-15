package lexer

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

	// Subsequent tasks (T6+) replace this fallthrough with full
	// DEFAULT-mode dispatch. Until then, advance one byte and return
	// EOF so partial integration tests progress.
	lx.advance(1)
	return lx.makeToken(EOF, lx.pos, lx.pos-1, "", lx.line, lx.col+1, lx.line, lx.col+1)
}

// isWhitespace mirrors .g4 WHITESPACE charclass: [ \t\n\r].
func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
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
