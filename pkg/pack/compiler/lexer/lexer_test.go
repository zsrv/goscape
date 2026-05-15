package lexer

import "testing"

// TestLex_EmptyInput_ReturnsEOF pins the lexer skeleton's degenerate
// case: empty source → immediate EOF.
func TestLex_EmptyInput_ReturnsEOF(t *testing.T) {
	l := NewLexer("", "empty.rs2")
	tok := l.NextToken()
	if tok.Type != EOF {
		t.Errorf("NextToken on empty input = %s, want EOF", tok.Type)
	}
	if tok.Source.Line != 1 || tok.Source.Column != 1 {
		t.Errorf("EOF source = (line=%d, col=%d), want (1,1)", tok.Source.Line, tok.Source.Column)
	}
	// Repeated calls must keep returning EOF (caller-friendly).
	for i := 0; i < 3; i++ {
		if l.NextToken().Type != EOF {
			t.Errorf("repeated NextToken at EOF: iter %d not EOF", i)
		}
	}
}

// TestLex_NewLexer_InitialState pins lexer construction invariants:
// 1-based line, 0-based col, mode stack starts at modeDefault, depth 0.
func TestLex_NewLexer_InitialState(t *testing.T) {
	l := NewLexer("abc", "x.rs2")
	if l.line != 1 {
		t.Errorf("initial line = %d, want 1", l.line)
	}
	if l.col != 0 {
		t.Errorf("initial col = %d, want 0", l.col)
	}
	if l.depth != 0 {
		t.Errorf("initial depth = %d, want 0", l.depth)
	}
	if l.currentMode() != modeDefault {
		t.Errorf("initial mode != modeDefault")
	}
	if l.sourceName != "x.rs2" {
		t.Errorf("sourceName = %q, want %q", l.sourceName, "x.rs2")
	}
	if l.tokenIndex != 0 {
		t.Errorf("initial tokenIndex = %d, want 0", l.tokenIndex)
	}
}

// TestLex_ModeStack_PushPop pins push/pop/currentMode semantics.
func TestLex_ModeStack_PushPop(t *testing.T) {
	l := NewLexer("", "x.rs2")
	if l.currentMode() != modeDefault {
		t.Fatalf("initial != modeDefault")
	}
	l.pushMode(modeString)
	if l.currentMode() != modeString {
		t.Errorf("after pushMode(modeString) != modeString")
	}
	l.pushMode(modeDefault)
	if l.currentMode() != modeDefault {
		t.Errorf("after pushMode(modeDefault) on top of String != modeDefault")
	}
	l.popMode()
	if l.currentMode() != modeString {
		t.Errorf("after popMode != modeString")
	}
	l.popMode()
	if l.currentMode() != modeDefault {
		t.Errorf("after second popMode != modeDefault")
	}
}

// TestLex_AddRemoveErrorListeners pins the listener-management API.
func TestLex_AddRemoveErrorListeners(t *testing.T) {
	l := NewLexer("", "x.rs2")
	c1 := &CollectingErrorListener{}
	c2 := &CollectingErrorListener{}
	l.AddErrorListener(c1)
	l.AddErrorListener(c2)
	l.reportError(1, 1, "msg") // package-internal helper exercised here
	if len(c1.Errors) != 1 || len(c2.Errors) != 1 {
		t.Errorf("both listeners should receive: c1=%d c2=%d", len(c1.Errors), len(c2.Errors))
	}
	l.RemoveErrorListeners()
	l.reportError(2, 1, "msg2")
	if len(c1.Errors) != 1 || len(c2.Errors) != 1 {
		t.Errorf("after Remove, no new errors should arrive: c1=%d c2=%d", len(c1.Errors), len(c2.Errors))
	}
}

// TestLex_Whitespace pins single WHITESPACE emission on hidden channel
// for [ \t\n\r]+ runs (RuneScriptLexer.g4:75).
func TestLex_Whitespace(t *testing.T) {
	cases := []string{" ", "\t", "\n", "\r", " \t\n\r ", "   \t  "}
	for _, src := range cases {
		l := NewLexer(src, "ws.rs2")
		tok := l.NextToken()
		if tok.Type != WHITESPACE {
			t.Errorf("input %q: type = %s, want WHITESPACE", src, tok.Type)
		}
		if tok.Channel != ChannelHidden {
			t.Errorf("input %q: channel = %d, want Hidden(1)", src, tok.Channel)
		}
		if tok.Text != src {
			t.Errorf("input %q: text = %q, want %q", src, tok.Text, src)
		}
		if l.NextToken().Type != EOF {
			t.Errorf("input %q: token after WHITESPACE not EOF", src)
		}
	}
}

// TestLex_LineComment pins LINE_COMMENT semantics: // ... \n consumed
// as a single hidden token (text includes the trailing \n).
func TestLex_LineComment(t *testing.T) {
	l := NewLexer("// hi\n", "lc.rs2")
	tok := l.NextToken()
	if tok.Type != LINE_COMMENT {
		t.Fatalf("type = %s, want LINE_COMMENT", tok.Type)
	}
	if tok.Channel != ChannelHidden {
		t.Errorf("channel = %d, want Hidden", tok.Channel)
	}
	if tok.Text != "// hi\n" {
		t.Errorf("text = %q, want %q", tok.Text, "// hi\n")
	}
	if l.NextToken().Type != EOF {
		t.Errorf("token after LINE_COMMENT not EOF")
	}
}

// TestLex_LineComment_EOF pins the trailing-no-newline case: // ... EOF
// is also one LINE_COMMENT spanning to end-of-input.
func TestLex_LineComment_EOF(t *testing.T) {
	l := NewLexer("// no newline", "lce.rs2")
	tok := l.NextToken()
	if tok.Type != LINE_COMMENT {
		t.Fatalf("type = %s, want LINE_COMMENT", tok.Type)
	}
	if tok.Text != "// no newline" {
		t.Errorf("text = %q", tok.Text)
	}
}

// TestLex_BlockComment pins single-line /* */ → one hidden token.
func TestLex_BlockComment(t *testing.T) {
	l := NewLexer("/* foo */", "bc.rs2")
	tok := l.NextToken()
	if tok.Type != BLOCK_COMMENT {
		t.Fatalf("type = %s, want BLOCK_COMMENT", tok.Type)
	}
	if tok.Channel != ChannelHidden {
		t.Errorf("channel = %d, want Hidden", tok.Channel)
	}
	if tok.Text != "/* foo */" {
		t.Errorf("text = %q", tok.Text)
	}
}

// TestLex_BlockComment_MultiLine pins multi-line BLOCK_COMMENT with
// endLine > line.
func TestLex_BlockComment_MultiLine(t *testing.T) {
	l := NewLexer("/* a\nb */", "bcm.rs2")
	tok := l.NextToken()
	if tok.Type != BLOCK_COMMENT {
		t.Fatalf("type = %s, want BLOCK_COMMENT", tok.Type)
	}
	if tok.Source.Line != 1 {
		t.Errorf("start line = %d, want 1", tok.Source.Line)
	}
	if tok.Source.EndLine != 2 {
		t.Errorf("end line = %d, want 2", tok.Source.EndLine)
	}
}

// TestLex_SourceLocation_NewlineLf pins line-counter behavior on \n.
func TestLex_SourceLocation_NewlineLf(t *testing.T) {
	l := NewLexer("\n\n", "nl.rs2") // two WS tokens? no — one run
	tok := l.NextToken()
	if tok.Type != WHITESPACE {
		t.Fatalf("type = %s, want WHITESPACE", tok.Type)
	}
	if tok.Source.Line != 1 || tok.Source.Column != 1 {
		t.Errorf("start = (%d,%d), want (1,1)", tok.Source.Line, tok.Source.Column)
	}
	if tok.Source.EndLine != 3 || tok.Source.EndColumn != 1 {
		t.Errorf("end = (%d,%d), want (3,1) — two \\n advance line to 3, col resets to 0=>1-based 1",
			tok.Source.EndLine, tok.Source.EndColumn)
	}
}

// TestLex_Symbols_SingleChar covers the 24 single-char symbol tokens.
func TestLex_Symbols_SingleChar(t *testing.T) {
	cases := []struct {
		src  string
		want TokenType
	}{
		{"(", LPAREN}, {")", RPAREN}, {":", COLON}, {";", SEMICOLON},
		{",", COMMA}, {"[", LBRACK}, {"]", RBRACK}, {"{", LBRACE},
		{"}", RBRACE}, {"*", MUL}, {"/", DIV}, {"%", MOD},
		{"&", AND}, {"|", OR}, {"=", EQ}, {"!", EXCL},
		{"$", DOLLAR}, {"^", CARET}, {"~", TILDE}, {"@", AT},
		{">", GT}, {"<", LT},
	}
	// Plus the two that need leading-char-only context: + and -
	// (handled separately when followed by id-class / digit).
	cases = append(cases, struct {
		src  string
		want TokenType
	}{"+", PLUS}, struct {
		src  string
		want TokenType
	}{"-", MINUS})
	for _, c := range cases {
		l := NewLexer(c.src, "sym.rs2")
		tok := l.NextToken()
		if tok.Type != c.want {
			t.Errorf("input %q: type = %s, want %s", c.src, tok.Type, c.want)
		}
		if tok.Text != c.src {
			t.Errorf("input %q: text = %q", c.src, tok.Text)
		}
		if next := l.NextToken(); next.Type != EOF {
			t.Errorf("input %q: token after symbol = %s, want EOF", c.src, next.Type)
		}
	}
}

// TestLex_Symbols_MultiChar pins GTE/LTE/DOTMOD longest-match.
func TestLex_Symbols_MultiChar(t *testing.T) {
	cases := []struct {
		src  string
		want TokenType
	}{
		{">=", GTE}, {"<=", LTE}, {".%", DOTMOD},
	}
	for _, c := range cases {
		l := NewLexer(c.src, "mc.rs2")
		tok := l.NextToken()
		if tok.Type != c.want {
			t.Errorf("input %q: type = %s, want %s", c.src, tok.Type, c.want)
		}
		if tok.Text != c.src {
			t.Errorf("input %q: text = %q", c.src, tok.Text)
		}
	}
}

// TestLex_Symbols_GteVsGt pins that `>= ` is GTE, `> =` is GT + WS + EQ.
func TestLex_Symbols_GteVsGt(t *testing.T) {
	l1 := NewLexer(">=", "x.rs2")
	if got := l1.NextToken().Type; got != GTE {
		t.Errorf("'>=' = %s, want GTE", got)
	}

	l2 := NewLexer("> =", "x.rs2")
	if got := l2.NextToken().Type; got != GT {
		t.Errorf("'> =' first token = %s, want GT", got)
	}
	if got := l2.NextToken().Type; got != WHITESPACE {
		t.Errorf("'> =' second token = %s, want WHITESPACE", got)
	}
	if got := l2.NextToken().Type; got != EQ {
		t.Errorf("'> =' third token = %s, want EQ", got)
	}
}

// TestLex_Symbols_GtSemanticAction_OutsideString pins that `>` outside
// any string emits plain GT (not STRING_EXPR_END) and depth stays 0.
func TestLex_Symbols_GtSemanticAction_OutsideString(t *testing.T) {
	l := NewLexer(">", "x.rs2")
	tok := l.NextToken()
	if tok.Type != GT {
		t.Errorf("type = %s, want GT", tok.Type)
	}
	if l.depth != 0 {
		t.Errorf("depth = %d, want 0", l.depth)
	}
}

// TestLex_CharLiteral pins CHAR_LITERAL: 'X' for one inner char,
// with escape support for \\ and \'.
func TestLex_CharLiteral(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"'a'", "'a'"},
		{`'\\'`, `'\\'`}, // \\ escape — backslash
		{`'\''`, `'\''`}, // \' escape — single quote
		{"'Z'", "'Z'"},
		{"'5'", "'5'"},
	}
	for _, c := range cases {
		l := NewLexer(c.src, "ch.rs2")
		tok := l.NextToken()
		if tok.Type != CHAR_LITERAL {
			t.Errorf("input %q: type = %s, want CHAR_LITERAL", c.src, tok.Type)
		}
		if tok.Text != c.want {
			t.Errorf("input %q: text = %q, want %q", c.src, tok.Text, c.want)
		}
	}
}

// TestLex_Keywords_Exact pins each exact-match keyword/literal.
func TestLex_Keywords_Exact(t *testing.T) {
	cases := []struct {
		src  string
		want TokenType
	}{
		{"if", IF}, {"else", ELSE}, {"while", WHILE}, {"case", CASE},
		{"default", DEFAULT}, {"return", RETURN}, {"calc", CALC},
		{"true", BOOLEAN_LITERAL}, {"false", BOOLEAN_LITERAL},
		{"null", NULL_LITERAL},
	}
	for _, c := range cases {
		l := NewLexer(c.src, "kw.rs2")
		tok := l.NextToken()
		if tok.Type != c.want {
			t.Errorf("input %q: type = %s, want %s", c.src, tok.Type, c.want)
		}
		if tok.Text != c.src {
			t.Errorf("input %q: text = %q", c.src, tok.Text)
		}
	}
}

// TestLex_Keywords_PrefixIsIdentifier pins longest-match: `elsex` is
// IDENTIFIER (length 5), not ELSE + IDENT.
func TestLex_Keywords_PrefixIsIdentifier(t *testing.T) {
	cases := []struct {
		src  string
		want TokenType
	}{
		{"elsex", IDENTIFIER},
		{"ifthen", IDENTIFIER},
		{"true_x", IDENTIFIER},
		{"nullx", IDENTIFIER},
	}
	for _, c := range cases {
		l := NewLexer(c.src, "kp.rs2")
		tok := l.NextToken()
		if tok.Type != c.want {
			t.Errorf("input %q: type = %s, want %s", c.src, tok.Type, c.want)
		}
		if tok.Text != c.src {
			t.Errorf("input %q: text = %q", c.src, tok.Text)
		}
	}
}

// TestLex_TypeArray pins suffix-pattern TYPE_ARRAY.
func TestLex_TypeArray(t *testing.T) {
	cases := []struct {
		src  string
		want TokenType
	}{
		{"int_array", TYPE_ARRAY},
		{"_array", TYPE_ARRAY},
		{"array", IDENTIFIER},
		{"arrayfoo", IDENTIFIER},
	}
	for _, c := range cases {
		l := NewLexer(c.src, "ta.rs2")
		tok := l.NextToken()
		if tok.Type != c.want {
			t.Errorf("input %q: type = %s, want %s", c.src, tok.Type, c.want)
		}
		if tok.Text != c.src {
			t.Errorf("input %q: text = %q", c.src, tok.Text)
		}
	}
}

// TestLex_DefType pins DEF_TYPE (def_X) including the bare-prefix case.
func TestLex_DefType(t *testing.T) {
	cases := []struct {
		src  string
		want TokenType
	}{
		{"def_int", DEF_TYPE},
		{"def_x", DEF_TYPE},
		{"def_", IDENTIFIER},
		{"default", DEFAULT},
	}
	for _, c := range cases {
		l := NewLexer(c.src, "dt.rs2")
		tok := l.NextToken()
		if tok.Type != c.want {
			t.Errorf("input %q: type = %s, want %s", c.src, tok.Type, c.want)
		}
	}
}

// TestLex_SwitchType pins SWITCH_TYPE (switch_X).
func TestLex_SwitchType(t *testing.T) {
	cases := []struct {
		src  string
		want TokenType
	}{
		{"switch_int", SWITCH_TYPE},
		{"switch_x", SWITCH_TYPE},
		{"switch_", IDENTIFIER},
	}
	for _, c := range cases {
		l := NewLexer(c.src, "st.rs2")
		tok := l.NextToken()
		if tok.Type != c.want {
			t.Errorf("input %q: type = %s, want %s", c.src, tok.Type, c.want)
		}
	}
}

// TestLex_Identifier_PlainStart pins IDENTIFIER on plain
// letter-starting input.
func TestLex_Identifier_PlainStart(t *testing.T) {
	cases := []string{"foo", "foo_bar", "abc123", "_x", "X", "snake_case"}
	for _, src := range cases {
		l := NewLexer(src, "id.rs2")
		tok := l.NextToken()
		if tok.Type != IDENTIFIER {
			t.Errorf("input %q: type = %s, want IDENTIFIER", src, tok.Type)
		}
		if tok.Text != src {
			t.Errorf("input %q: text = %q", src, tok.Text)
		}
	}
}

// TestLex_Identifier_SymbolPrefix pins identifiers starting with id-
// class symbols (+, ., :). Per spec §5.5 step 6/7/8.
func TestLex_Identifier_SymbolPrefix(t *testing.T) {
	cases := []struct {
		src  string
		want TokenType
	}{
		{"+abc", IDENTIFIER},
		{".abc", IDENTIFIER},
		{":abc", IDENTIFIER},
		{".", IDENTIFIER},
	}
	for _, c := range cases {
		l := NewLexer(c.src, "sp.rs2")
		tok := l.NextToken()
		if tok.Type != c.want {
			t.Errorf("input %q: type = %s, want %s", c.src, tok.Type, c.want)
		}
		if tok.Text != c.src {
			t.Errorf("input %q: text = %q", c.src, tok.Text)
		}
	}
}
