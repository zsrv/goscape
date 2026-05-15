package lexer

import (
	"strings"
	"testing"
)

// TestLex_ErrorListener_FiresOnce_UnrecognizedChar pins that an
// unrecognized character (like `?`) fires SyntaxError exactly once
// with correct 1-based line/column.
func TestLex_ErrorListener_FiresOnce_UnrecognizedChar(t *testing.T) {
	l := NewLexer("?", "x.rs2")
	c := &CollectingErrorListener{}
	l.AddErrorListener(c)
	_ = l.NextToken() // recovery emits some token (or EOF)
	if len(c.Errors) != 1 {
		t.Fatalf("got %d errors, want 1", len(c.Errors))
	}
	e := c.Errors[0]
	if e.Line != 1 || e.Column != 1 {
		t.Errorf("error location = (%d,%d), want (1,1)", e.Line, e.Column)
	}
	if !strings.Contains(e.Msg, "?") {
		t.Errorf("error msg = %q, expected to mention '?'", e.Msg)
	}
}

func TestLex_ErrorListener_RemoveListeners(t *testing.T) {
	l := NewLexer("?", "x.rs2")
	c := &CollectingErrorListener{}
	l.AddErrorListener(c)
	l.RemoveErrorListeners()
	_ = l.NextToken()
	if len(c.Errors) != 0 {
		t.Errorf("got %d errors after Remove, want 0", len(c.Errors))
	}
}

func TestLex_ErrorListener_MultipleListeners(t *testing.T) {
	l := NewLexer("?", "x.rs2")
	c1 := &CollectingErrorListener{}
	c2 := &CollectingErrorListener{}
	l.AddErrorListener(c1)
	l.AddErrorListener(c2)
	_ = l.NextToken()
	if len(c1.Errors) != 1 || len(c2.Errors) != 1 {
		t.Errorf("c1=%d, c2=%d, want 1 each", len(c1.Errors), len(c2.Errors))
	}
}

func TestLex_BlockComment_Unterminated(t *testing.T) {
	l := NewLexer("/* foo", "bcu.rs2")
	c := &CollectingErrorListener{}
	l.AddErrorListener(c)
	tok := l.NextToken()
	if tok.Type != BLOCK_COMMENT {
		t.Errorf("type = %s, want BLOCK_COMMENT (partial)", tok.Type)
	}
	if tok.Text != "/* foo" {
		t.Errorf("text = %q, want %q", tok.Text, "/* foo")
	}
	if len(c.Errors) != 1 {
		t.Errorf("got %d errors, want 1", len(c.Errors))
	}
}

func TestLex_String_Unterminated_Newline(t *testing.T) {
	l := NewLexer("\"foo\nbar", "su.rs2")
	c := &CollectingErrorListener{}
	l.AddErrorListener(c)
	tokens := drainTokens(t, l)
	if len(c.Errors) == 0 {
		t.Errorf("no errors fired, want ≥1")
	}
	if l.currentMode() != modeDefault {
		t.Errorf("mode after recovery != modeDefault")
	}
	if tokens[0].Type != QUOTE_OPEN {
		t.Errorf("first token = %s, want QUOTE_OPEN", tokens[0].Type)
	}
	for _, tok := range tokens {
		if tok.Type == QUOTE_CLOSE {
			t.Errorf("unexpected QUOTE_CLOSE in unterminated string")
		}
	}
}

func TestLex_String_Unterminated_EOF(t *testing.T) {
	l := NewLexer("\"foo", "se.rs2")
	c := &CollectingErrorListener{}
	l.AddErrorListener(c)
	_ = drainTokens(t, l)
	if len(c.Errors) == 0 {
		t.Errorf("no errors fired")
	}
}

func TestLex_CharLiteral_Unterminated(t *testing.T) {
	l := NewLexer("'a", "cu.rs2")
	c := &CollectingErrorListener{}
	l.AddErrorListener(c)
	_ = drainTokens(t, l)
	if len(c.Errors) == 0 {
		t.Errorf("no errors fired")
	}
}
