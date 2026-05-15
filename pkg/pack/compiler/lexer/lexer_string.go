package lexer

// nextString is the modeString dispatch. Per spec §5.6 and
// RuneScriptLexer.g4:80-87.
//
// Algorithm:
//   - `"` → QUOTE_CLOSE (depth--, popMode)
//   - `<` → try STRING_P_TAG, STRING_CLOSE_TAG, STRING_TAG,
//           STRING_PARTIAL_TAG; fall through to STRING_EXPR_START
//           (T10 pushes modeDefault).
//   - else → STRING_TEXT run (escapes + non-special chars)
//
// T10 wires interpolation (STRING_EXPR_START pushes modeDefault) and
// the GT semantic action (depth>0 → STRING_EXPR_END + popMode).
// T11 wires error recovery for unterminated strings.
func (lx *Lexer) nextString() Token {
	c := lx.input[lx.pos]

	if c == '"' {
		return lx.consumeQuoteClose()
	}

	if c == '<' {
		if n := matchStringPTag(lx.input, lx.pos); n > 0 {
			return lx.emitStringMode(STRING_P_TAG, n)
		}
		if n := matchStringCloseTag(lx.input, lx.pos); n > 0 {
			return lx.emitStringMode(STRING_CLOSE_TAG, n)
		}
		if n := matchStringTag(lx.input, lx.pos); n > 0 {
			return lx.emitStringMode(STRING_TAG, n)
		}
		if n := matchStringPartialTag(lx.input, lx.pos); n > 0 {
			return lx.emitStringMode(STRING_PARTIAL_TAG, n)
		}
		return lx.consumeStringExprStart()
	}

	return lx.consumeStringText()
}

// consumeQuoteClose emits QUOTE_CLOSE, decrements depth, pops mode.
func (lx *Lexer) consumeQuoteClose() Token {
	start := lx.pos
	startLn, startCol := lx.line, lx.col
	lx.advance(1)
	lx.depth--
	lx.popMode()
	return lx.makeToken(QUOTE_CLOSE, start, start, `"`, startLn, startCol+1, startLn, startCol+1)
}

// emitStringMode advances n bytes and emits the given TokenType as a
// String-mode token (default channel).
func (lx *Lexer) emitStringMode(tt TokenType, n int) Token {
	start := lx.pos
	startLn, startCol := lx.line, lx.col
	for i := 0; i < n; i++ {
		lx.advance(1)
	}
	stop := lx.pos - 1
	endLn, endCol := lx.lineColAt(stop)
	return lx.makeToken(tt, start, stop, lx.input[start:lx.pos], startLn, startCol+1, endLn, endCol+1)
}

// consumeStringExprStart emits STRING_EXPR_START and pushes modeDefault
// (.g4:86). The inner expression is then lexed with DEFAULT-mode rules;
// the closing `>` retypes to STRING_EXPR_END via consumeGt's depth-check
// and pops back to modeString.
func (lx *Lexer) consumeStringExprStart() Token {
	start := lx.pos
	startLn, startCol := lx.line, lx.col
	lx.advance(1)
	lx.pushMode(modeDefault)
	return lx.makeToken(STRING_EXPR_START, start, start, "<", startLn, startCol+1, startLn, startCol+1)
}

// consumeStringText consumes one or more (escape | non-special) chars
// and emits STRING_TEXT. Escapes recognized: \\ \" \< (per .g4
// StringEscapeSequence fragment). Excluded chars: " < \r \n \\ (\\ is
// included via escape match).
func (lx *Lexer) consumeStringText() Token {
	start := lx.pos
	startLn, startCol := lx.line, lx.col
	for lx.pos < len(lx.input) {
		c := lx.input[lx.pos]
		if c == '\\' && lx.pos+1 < len(lx.input) {
			next := lx.input[lx.pos+1]
			if next == '\\' || next == '"' || next == '<' {
				lx.advance(2)
				continue
			}
			break
		}
		if c == '"' || c == '<' || c == '\r' || c == '\n' {
			break
		}
		lx.advance(1)
	}
	stop := lx.pos - 1
	endLn, endCol := lx.lineColAt(stop)
	return lx.makeToken(STRING_TEXT, start, stop, lx.input[start:lx.pos], startLn, startCol+1, endLn, endCol+1)
}

// matchStringTag — `'<' Tag ('=' ~('<' | '>')+)? '>'`.
func matchStringTag(input string, pos int) int {
	if pos >= len(input) || input[pos] != '<' {
		return 0
	}
	p := pos + 1
	tagLen := matchTag(input, p)
	if tagLen == 0 {
		return 0
	}
	p += tagLen
	if p < len(input) && input[p] == '=' {
		p++
		valStart := p
		for p < len(input) && input[p] != '<' && input[p] != '>' {
			p++
		}
		if p == valStart {
			return 0
		}
	}
	if p >= len(input) || input[p] != '>' {
		return 0
	}
	return (p + 1) - pos
}

// matchStringCloseTag — `'</' Tag '>'`.
func matchStringCloseTag(input string, pos int) int {
	if pos+1 >= len(input) || input[pos] != '<' || input[pos+1] != '/' {
		return 0
	}
	p := pos + 2
	tagLen := matchTag(input, p)
	if tagLen == 0 {
		return 0
	}
	p += tagLen
	if p >= len(input) || input[p] != '>' {
		return 0
	}
	return (p + 1) - pos
}

// matchStringPartialTag — `'<' Tag '='` (no closing >).
func matchStringPartialTag(input string, pos int) int {
	if pos >= len(input) || input[pos] != '<' {
		return 0
	}
	p := pos + 1
	tagLen := matchTag(input, p)
	if tagLen == 0 {
		return 0
	}
	p += tagLen
	if p >= len(input) || input[p] != '=' {
		return 0
	}
	return (p + 1) - pos
}

// matchStringPTag — `'<p,' ~('<' | '>')+ '>'`.
func matchStringPTag(input string, pos int) int {
	if pos+2 >= len(input) || input[pos] != '<' || input[pos+1] != 'p' || input[pos+2] != ',' {
		return 0
	}
	p := pos + 3
	valStart := p
	for p < len(input) && input[p] != '<' && input[p] != '>' {
		p++
	}
	if p == valStart {
		return 0
	}
	if p >= len(input) || input[p] != '>' {
		return 0
	}
	return (p + 1) - pos
}

// matchTag — .g4 Tag fragment: 'br' | 'col' | 'str' | 'shad' | 'u' |
// 'img' | 'gt' | 'lt'. Returns matched length or 0.
func matchTag(input string, pos int) int {
	tags := []string{"shad", "col", "str", "img", "br", "gt", "lt", "u"}
	for _, tag := range tags {
		if pos+len(tag) <= len(input) && input[pos:pos+len(tag)] == tag {
			return len(tag)
		}
	}
	return 0
}
