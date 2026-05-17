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

// TestLex_LineComment_CrLf pins that `// foo\r\n` ends the comment at the
// CRLF, not later. Regression for Content/scripts/drop tables/scripts/man.rs2
// where the file's first line ended with \r\n and the comment loop swallowed
// the 5 following script declarations because it only broke on bare \n.
func TestLex_LineComment_CrLf(t *testing.T) {
	l := NewLexer("// hi\r\n[", "lc-crlf.rs2")
	tok := l.NextToken()
	if tok.Type != LINE_COMMENT {
		t.Fatalf("type = %s, want LINE_COMMENT", tok.Type)
	}
	if tok.Text != "// hi\r\n" {
		t.Errorf("text = %q, want %q", tok.Text, "// hi\r\n")
	}
	// Hidden WHITESPACE may or may not exist; what matters is the next
	// default-channel token is LBRACK, not nothing.
	for {
		next := l.NextToken()
		if next.Channel == ChannelHidden {
			continue
		}
		if next.Type != LBRACK {
			t.Fatalf("token after LINE_COMMENT type = %s, want LBRACK", next.Type)
		}
		break
	}
}

// TestLex_LineComment_BareCr pins that `// foo\r` (classic-Mac line ending)
// ends the comment at the \r. Lexer.advance treats bare \r as a line break,
// so consumeLineComment should too.
func TestLex_LineComment_BareCr(t *testing.T) {
	l := NewLexer("// hi\r[", "lc-cr.rs2")
	tok := l.NextToken()
	if tok.Type != LINE_COMMENT {
		t.Fatalf("type = %s, want LINE_COMMENT", tok.Type)
	}
	if tok.Text != "// hi\r" {
		t.Errorf("text = %q, want %q", tok.Text, "// hi\r")
	}
	for {
		next := l.NextToken()
		if next.Channel == ChannelHidden {
			continue
		}
		if next.Type != LBRACK {
			t.Fatalf("token after LINE_COMMENT type = %s, want LBRACK", next.Type)
		}
		break
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

// TestLex_IntegerLiteral pins INTEGER variants.
func TestLex_IntegerLiteral(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"0", "0"},
		{"12345", "12345"},
		{"-5", "-5"},
		{"9999999999", "9999999999"},
	}
	for _, c := range cases {
		l := NewLexer(c.src, "int.rs2")
		tok := l.NextToken()
		if tok.Type != INTEGER_LITERAL {
			t.Errorf("input %q: type = %s, want INTEGER_LITERAL", c.src, tok.Type)
		}
		if tok.Text != c.want {
			t.Errorf("input %q: text = %q, want %q", c.src, tok.Text, c.want)
		}
	}
}

func TestLex_HexLiteral(t *testing.T) {
	cases := []string{"0x1F", "0X1f", "0xDEADBEEF", "0x0"}
	for _, src := range cases {
		l := NewLexer(src, "hex.rs2")
		tok := l.NextToken()
		if tok.Type != HEX_LITERAL {
			t.Errorf("input %q: type = %s, want HEX_LITERAL", src, tok.Type)
		}
		if tok.Text != src {
			t.Errorf("input %q: text = %q", src, tok.Text)
		}
	}
}

func TestLex_HexLiteral_NoHexDigit(t *testing.T) {
	l := NewLexer("0x", "h0.rs2")
	tok := l.NextToken()
	if tok.Type != IDENTIFIER {
		t.Errorf("'0x' alone: type = %s, want IDENTIFIER", tok.Type)
	}
	if tok.Text != "0x" {
		t.Errorf("text = %q", tok.Text)
	}
}

func TestLex_BinLiteral(t *testing.T) {
	cases := []string{"0b101", "0B0", "0b11111111"}
	for _, src := range cases {
		l := NewLexer(src, "bin.rs2")
		tok := l.NextToken()
		if tok.Type != BIN_LITERAL {
			t.Errorf("input %q: type = %s, want BIN_LITERAL", src, tok.Type)
		}
		if tok.Text != src {
			t.Errorf("input %q: text = %q", src, tok.Text)
		}
	}
}

func TestLex_CoordLiteral(t *testing.T) {
	l := NewLexer("0_50_50_50_50", "c.rs2")
	tok := l.NextToken()
	if tok.Type != COORD_LITERAL {
		t.Errorf("type = %s, want COORD_LITERAL", tok.Type)
	}
	if tok.Text != "0_50_50_50_50" {
		t.Errorf("text = %q", tok.Text)
	}
}

func TestLex_MapzoneLiteral(t *testing.T) {
	l := NewLexer("1_50_50", "m.rs2")
	tok := l.NextToken()
	if tok.Type != MAPZONE_LITERAL {
		t.Errorf("type = %s, want MAPZONE_LITERAL", tok.Type)
	}
	if tok.Text != "1_50_50" {
		t.Errorf("text = %q", tok.Text)
	}
}

func TestLex_FourGroupUnderscores_IDENTIFIER(t *testing.T) {
	l := NewLexer("1_2_3_4", "fg.rs2")
	tok := l.NextToken()
	if tok.Type != IDENTIFIER {
		t.Errorf("type = %s, want IDENTIFIER (longest-match)", tok.Type)
	}
	if tok.Text != "1_2_3_4" {
		t.Errorf("text = %q", tok.Text)
	}
}

func TestLex_SixGroupUnderscores_IDENTIFIER(t *testing.T) {
	l := NewLexer("1_2_3_4_5_6", "sg.rs2")
	tok := l.NextToken()
	if tok.Type != IDENTIFIER {
		t.Errorf("type = %s, want IDENTIFIER", tok.Type)
	}
	if tok.Text != "1_2_3_4_5_6" {
		t.Errorf("text = %q", tok.Text)
	}
}

func TestLex_DigitsThenLetters_IDENTIFIER(t *testing.T) {
	l := NewLexer("5abc", "dtl.rs2")
	tok := l.NextToken()
	if tok.Type != IDENTIFIER {
		t.Errorf("type = %s, want IDENTIFIER", tok.Type)
	}
	if tok.Text != "5abc" {
		t.Errorf("text = %q", tok.Text)
	}
}

// TestLex_MinusInteger_NoSpaces pins `5-3` → INTEGER `5` + INTEGER `-3`.
// ANTLR's lexer is context-free: leading `-` bonds with the next digit
// run. Per spec §5.5.1 closing paragraph + open question §8.
func TestLex_MinusInteger_NoSpaces(t *testing.T) {
	l := NewLexer("5-3", "mi.rs2")
	t1 := l.NextToken()
	if t1.Type != INTEGER_LITERAL || t1.Text != "5" {
		t.Errorf("first token = %s %q, want INTEGER_LITERAL '5'", t1.Type, t1.Text)
	}
	t2 := l.NextToken()
	if t2.Type != INTEGER_LITERAL || t2.Text != "-3" {
		t.Errorf("second token = %s %q, want INTEGER_LITERAL '-3'", t2.Type, t2.Text)
	}
	if l.NextToken().Type != EOF {
		t.Errorf("third token not EOF")
	}
}

func TestLex_MinusInteger_WithSpaces(t *testing.T) {
	l := NewLexer("5 - 3", "mis.rs2")
	want := []TokenType{INTEGER_LITERAL, WHITESPACE, MINUS, WHITESPACE, INTEGER_LITERAL, EOF}
	for i, w := range want {
		got := l.NextToken().Type
		if got != w {
			t.Errorf("token %d = %s, want %s", i, got, w)
		}
	}
}

// TestLex_SourceLocation_OneLine pins basic same-line column tracking.
func TestLex_SourceLocation_OneLine(t *testing.T) {
	l := NewLexer("abc def", "sl.rs2")
	t1 := l.NextToken() // IDENTIFIER "abc"
	if t1.Type != IDENTIFIER {
		t.Fatalf("t1.Type = %s, want IDENTIFIER", t1.Type)
	}
	if t1.Source.Line != 1 || t1.Source.Column != 1 || t1.Source.EndLine != 1 || t1.Source.EndColumn != 3 {
		t.Errorf("t1 src = (%d:%d-%d:%d), want (1:1-1:3)", t1.Source.Line, t1.Source.Column, t1.Source.EndLine, t1.Source.EndColumn)
	}
	_ = l.NextToken() // WHITESPACE
	t3 := l.NextToken() // IDENTIFIER "def"
	if t3.Source.Line != 1 || t3.Source.Column != 5 || t3.Source.EndColumn != 7 {
		t.Errorf("t3 src = (%d:%d-?:%d), want (1:5-?:7)", t3.Source.Line, t3.Source.Column, t3.Source.EndColumn)
	}
}

// TestLex_SourceLocation_NewlineCrLf pins \r\n line-counting.
func TestLex_SourceLocation_NewlineCrLf(t *testing.T) {
	l := NewLexer("a\r\nb", "crlf.rs2")
	t1 := l.NextToken() // IDENT a
	if t1.Source.Line != 1 || t1.Source.Column != 1 {
		t.Errorf("t1 = (%d:%d), want (1:1)", t1.Source.Line, t1.Source.Column)
	}
	_ = l.NextToken() // WS \r\n
	t3 := l.NextToken() // IDENT b
	if t3.Source.Line != 2 || t3.Source.Column != 1 {
		t.Errorf("t3 = (%d:%d), want (2:1)", t3.Source.Line, t3.Source.Column)
	}
}

// TestLex_SourceLocation_NewlineCr pins bare \r line-counting.
func TestLex_SourceLocation_NewlineCr(t *testing.T) {
	l := NewLexer("a\rb", "cr.rs2")
	_ = l.NextToken() // IDENT a
	_ = l.NextToken() // WS \r
	t3 := l.NextToken() // IDENT b
	if t3.Source.Line != 2 || t3.Source.Column != 1 {
		t.Errorf("t3 = (%d:%d), want (2:1)", t3.Source.Line, t3.Source.Column)
	}
}

// TestLex_SourceLocation_BlockComment_EndPosition pins that a
// multi-line BLOCK_COMMENT reports endLine/endColumn correctly.
// NOTE: plan-author traced advance/lineColAt arithmetic to pre-correct
// the expected values: BC end=(2:4), IDENT 'c' col=5. The original plan
// pre-correction was (2:5) and col=6; corrected by the plan-author note.
func TestLex_SourceLocation_BlockComment_EndPosition(t *testing.T) {
	l := NewLexer("/* a\nb */c", "bce.rs2")
	t1 := l.NextToken() // BLOCK_COMMENT
	if t1.Type != BLOCK_COMMENT {
		t.Fatalf("t1.Type = %s, want BLOCK_COMMENT", t1.Type)
	}
	if t1.Source.Line != 1 || t1.Source.Column != 1 {
		t.Errorf("BC start = (%d:%d), want (1:1)", t1.Source.Line, t1.Source.Column)
	}
	if t1.Source.EndLine != 2 || t1.Source.EndColumn != 4 {
		t.Errorf("BC end = (%d:%d), want (2:4)", t1.Source.EndLine, t1.Source.EndColumn)
	}
	t2 := l.NextToken() // IDENT c
	if t2.Source.Line != 2 || t2.Source.Column != 5 {
		t.Errorf("IDENT 'c' = (%d:%d), want (2:5)", t2.Source.Line, t2.Source.Column)
	}
}
