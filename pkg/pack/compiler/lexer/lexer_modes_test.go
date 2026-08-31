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

// TestLex_StringEscapes pins the 0.9.6 maximal-munch STRING_TEXT
// semantics: escapes FUSE with adjacent plain runs into a single token
// (RuneScriptLexer.g4:81 @ RuneScriptTS b8c3388, repetition over the
// union — upstream commit ef6636e "fix: update string escape sequence
// parsing rules"). Source `"a\\b\"c\<d"` lexes as ONE STRING_TEXT token
// with the raw text `a\\b\"c\<d`.
//
// History (NAI-221, retired at rev-254): 0.9.4's alternation grammar
// made each escape its own token (7 STRING_TEXT tokens for this input);
// the rev-225/244/245.2 ports pinned that. The 254 pin
// (@lostcityrs/runescript@0.9.6) fuses — the T23 full-tree gate caught
// the split form fragmenting joined-string parts.
func TestLex_StringEscapes(t *testing.T) {
	src := `"a\\b\"c\<d"`
	l := NewLexer(src, "se.rs2")
	tokens := drainTokens(t, l)
	want := []TokenType{
		QUOTE_OPEN,
		STRING_TEXT, // `a\\b\"c\<d` — single fused token
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
	if wantText := `a\\b\"c\<d`; tokens[1].Text != wantText {
		t.Errorf("token 1 text = %q, want %q", tokens[1].Text, wantText)
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
	for range 1000 {
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
