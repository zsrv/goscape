package lexer

import "testing"

// TestLex_PlainString covers `"hello"` end-to-end through DEFAULT
// (QUOTE_OPEN) → String (STRING_TEXT) → DEFAULT (QUOTE_CLOSE).
func TestLex_PlainString(t *testing.T) {
	l := NewLexer(`"hello"`, "ps.rs2")
	tokens := drainTokens(t, l)
	want := []TokenType{QUOTE_OPEN, STRING_TEXT, QUOTE_CLOSE, EOF}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(tokens), len(want), tokens)
	}
	for i, w := range want {
		if tokens[i].Type != w {
			t.Errorf("token %d = %s, want %s", i, tokens[i].Type, w)
		}
	}
	if tokens[1].Text != "hello" {
		t.Errorf("STRING_TEXT.Text = %q, want %q", tokens[1].Text, "hello")
	}
	if l.depth != 0 {
		t.Errorf("depth after balanced string = %d, want 0", l.depth)
	}
	if l.currentMode() != modeDefault {
		t.Errorf("mode after balanced string != modeDefault")
	}
}

// TestLex_StringEscapes pins the NAI-221 token-split semantics: each
// StringEscapeSequence is its own STRING_TEXT token, matching TS
// RuneScript 0.9.4 (the version pinned by Server225_2 at
// node_modules/@lostcityrs/runescript). Source `"a\\b\"c\<d"` lexes as
// 7 STRING_TEXT tokens: ["a", "\\", "b", "\"", "c", "\<", "d"].
//
// Cross-reference: upstream commit ef6636e ("fix: update string escape
// sequence parsing rules") relaxed this in TS 0.9.6 by changing the
// STRING_TEXT grammar from alternation to repetition. Goscape stays on
// the pre-fix shape so that Go-packed and TS-packed script.dat are
// byte-identical for the currently pinned TS version. If Server225_2
// upgrades to RuneScript >= 0.9.6, this test (and the lexer split)
// can be retired.
func TestLex_StringEscapes(t *testing.T) {
	src := `"a\\b\"c\<d"`
	l := NewLexer(src, "se.rs2")
	tokens := drainTokens(t, l)
	want := []TokenType{
		QUOTE_OPEN,
		STRING_TEXT, // "a"
		STRING_TEXT, // "\\"
		STRING_TEXT, // "b"
		STRING_TEXT, // "\""
		STRING_TEXT, // "c"
		STRING_TEXT, // "\<"
		STRING_TEXT, // "d"
		QUOTE_CLOSE,
		EOF,
	}
	if len(tokens) != len(want) {
		t.Fatalf("token count: got %d, want %d; all=%v", len(tokens), len(want), tokens)
	}
	for i, w := range want {
		if tokens[i].Type != w {
			t.Fatalf("token %d = %s, want %s; all=%v", i, tokens[i].Type, w, tokens)
		}
	}
	wantText := []string{"", "a", `\\`, "b", `\"`, "c", `\<`, "d", "", ""}
	for i, wt := range wantText {
		if wt == "" {
			continue
		}
		if tokens[i].Text != wt {
			t.Errorf("token %d text = %q, want %q", i, tokens[i].Text, wt)
		}
	}
}

func TestLex_StringTags(t *testing.T) {
	cases := []struct {
		src  string
		want []TokenType
	}{
		{`"<br>"`, []TokenType{QUOTE_OPEN, STRING_TAG, QUOTE_CLOSE, EOF}},
		{`"<col=ff0000>"`, []TokenType{QUOTE_OPEN, STRING_TAG, QUOTE_CLOSE, EOF}},
		{`"</col>"`, []TokenType{QUOTE_OPEN, STRING_CLOSE_TAG, QUOTE_CLOSE, EOF}},
		{`"<p,head>"`, []TokenType{QUOTE_OPEN, STRING_P_TAG, QUOTE_CLOSE, EOF}},
	}
	for _, c := range cases {
		l := NewLexer(c.src, "tags.rs2")
		tokens := drainTokens(t, l)
		if len(tokens) != len(c.want) {
			t.Errorf("input %q: got %d tokens, want %d: %v", c.src, len(tokens), len(c.want), tokens)
			continue
		}
		for i, w := range c.want {
			if tokens[i].Type != w {
				t.Errorf("input %q token %d = %s, want %s", c.src, i, tokens[i].Type, w)
			}
		}
	}
}

func TestLex_StringPartialTag(t *testing.T) {
	src := `"<col=red"`
	l := NewLexer(src, "pt.rs2")
	tokens := drainTokens(t, l)
	want := []TokenType{QUOTE_OPEN, STRING_PARTIAL_TAG, STRING_TEXT, QUOTE_CLOSE, EOF}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(tokens), len(want), tokens)
	}
	for i, w := range want {
		if tokens[i].Type != w {
			t.Errorf("token %d = %s, want %s", i, tokens[i].Type, w)
		}
	}
	if tokens[1].Text != "<col=" {
		t.Errorf("STRING_PARTIAL_TAG.Text = %q, want %q", tokens[1].Text, "<col=")
	}
}

// drainTokens runs NextToken until EOF is returned, returning all
// emitted tokens including EOF.
func drainTokens(t *testing.T, l *Lexer) []Token {
	t.Helper()
	var out []Token
	for i := 0; i < 1000; i++ {
		tok := l.NextToken()
		out = append(out, tok)
		if tok.Type == EOF {
			return out
		}
	}
	t.Fatalf("drainTokens exceeded 1000 tokens — likely infinite loop")
	return nil
}

// TestLex_StringInterpolation_Simple pins `"a<$x>b"` end-to-end:
// QO TEXT(a) EXPR_START DOLLAR IDENT(x) EXPR_END TEXT(b) QC.
func TestLex_StringInterpolation_Simple(t *testing.T) {
	l := NewLexer(`"a<$x>b"`, "si.rs2")
	tokens := drainTokens(t, l)
	want := []struct {
		tt   TokenType
		text string
	}{
		{QUOTE_OPEN, `"`},
		{STRING_TEXT, "a"},
		{STRING_EXPR_START, "<"},
		{DOLLAR, "$"},
		{IDENTIFIER, "x"},
		{STRING_EXPR_END, ">"},
		{STRING_TEXT, "b"},
		{QUOTE_CLOSE, `"`},
		{EOF, ""},
	}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(tokens), len(want), tokens)
	}
	for i, w := range want {
		if tokens[i].Type != w.tt {
			t.Errorf("token %d = %s, want %s (text=%q)", i, tokens[i].Type, w.tt, tokens[i].Text)
		}
		if w.text != "" && tokens[i].Text != w.text {
			t.Errorf("token %d text = %q, want %q", i, tokens[i].Text, w.text)
		}
	}
	if l.depth != 0 {
		t.Errorf("final depth = %d, want 0", l.depth)
	}
}

// TestLex_StringInterpolation_NestedString pins the depth-counter:
// `"a<"b">c"` should drive depth to 2 inside, then back to 0.
func TestLex_StringInterpolation_NestedString(t *testing.T) {
	l := NewLexer(`"a<"b">c"`, "sn.rs2")
	tokens := drainTokens(t, l)
	want := []TokenType{
		QUOTE_OPEN,        // depth 0→1, push modeString
		STRING_TEXT,       // "a"
		STRING_EXPR_START, // "<", push modeDefault
		QUOTE_OPEN,        // depth 1→2, push modeString
		STRING_TEXT,       // "b"
		QUOTE_CLOSE,       // depth 2→1, pop modeString
		STRING_EXPR_END,   // ">", retyped from GT because depth>0, pop modeDefault
		STRING_TEXT,       // "c"
		QUOTE_CLOSE,       // depth 1→0, pop modeString
		EOF,
	}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(tokens), len(want), tokens)
	}
	for i, w := range want {
		if tokens[i].Type != w {
			t.Errorf("token %d = %s, want %s", i, tokens[i].Type, w)
		}
	}
}

// TestLex_StringInterpolation_GtInsideExpr pins that `>` inside an
// interpolation expression retypes to STRING_EXPR_END regardless of
// what's "actually" parseable. .g4 semantic action: depth>0 → retype.
func TestLex_StringInterpolation_GtInsideExpr(t *testing.T) {
	l := NewLexer(`"<x>"`, "gi.rs2")
	tokens := drainTokens(t, l)
	want := []TokenType{
		QUOTE_OPEN,
		STRING_EXPR_START,
		IDENTIFIER,
		STRING_EXPR_END,
		QUOTE_CLOSE,
		EOF,
	}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(tokens), len(want), tokens)
	}
	for i, w := range want {
		if tokens[i].Type != w {
			t.Errorf("token %d = %s, want %s", i, tokens[i].Type, w)
		}
	}
}
