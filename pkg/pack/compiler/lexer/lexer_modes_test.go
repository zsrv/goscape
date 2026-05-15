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

func TestLex_StringEscapes(t *testing.T) {
	src := `"a\\b\"c\<d"`
	l := NewLexer(src, "se.rs2")
	tokens := drainTokens(t, l)
	want := []TokenType{QUOTE_OPEN, STRING_TEXT, QUOTE_CLOSE, EOF}
	for i, w := range want {
		if tokens[i].Type != w {
			t.Fatalf("token %d = %s, want %s; all=%v", i, tokens[i].Type, w, tokens)
		}
	}
	if tokens[1].Text != `a\\b\"c\<d` {
		t.Errorf("STRING_TEXT.Text = %q", tokens[1].Text)
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
