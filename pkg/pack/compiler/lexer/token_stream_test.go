package lexer

import "testing"

// TestTokenStream_LT1_AfterInit pins that LT(1) returns the first
// default-channel token (skips leading whitespace + comments).
func TestTokenStream_LT1_AfterInit(t *testing.T) {
	ts := NewTokenStream(NewLexer("// hi\n  foo", "ts.rs2"))
	tok := ts.LT(1)
	if tok.Type != IDENTIFIER {
		t.Errorf("LT(1).Type = %s, want IDENTIFIER", tok.Type)
	}
	if tok.Text != "foo" {
		t.Errorf("LT(1).Text = %q", tok.Text)
	}
}

// TestTokenStream_LT2_Lookahead pins LT(2) peeks the second default-
// channel token.
func TestTokenStream_LT2_Lookahead(t *testing.T) {
	ts := NewTokenStream(NewLexer("foo bar", "ts.rs2"))
	t1 := ts.LT(1)
	t2 := ts.LT(2)
	if t1.Text != "foo" {
		t.Errorf("LT(1) = %q, want foo", t1.Text)
	}
	if t2.Text != "bar" {
		t.Errorf("LT(2) = %q, want bar", t2.Text)
	}
}

// TestTokenStream_LT_BeyondEnd pins LT past EOF returns EOF (not nil).
func TestTokenStream_LT_BeyondEnd(t *testing.T) {
	ts := NewTokenStream(NewLexer("foo", "ts.rs2"))
	tok := ts.LT(99)
	if tok == nil {
		t.Fatal("LT(99) returned nil")
	}
	if tok.Type != EOF {
		t.Errorf("LT(99).Type = %s, want EOF", tok.Type)
	}
}

// TestTokenStream_LTNegative pins LT(-1) returns the just-consumed
// default-channel token.
func TestTokenStream_LTNegative(t *testing.T) {
	ts := NewTokenStream(NewLexer("foo bar baz", "ts.rs2"))
	_ = ts.LT(1) // foo
	ts.Consume()
	prev := ts.LT(-1)
	if prev == nil || prev.Text != "foo" {
		t.Errorf("LT(-1) = %v, want token with text 'foo'", prev)
	}
}

// TestTokenStream_Consume_SkipsHidden pins that Consume skips past
// hidden-channel tokens after the consumed default-channel one.
func TestTokenStream_Consume_SkipsHidden(t *testing.T) {
	ts := NewTokenStream(NewLexer("a /* c */ b", "ts.rs2"))
	if ts.LT(1).Text != "a" {
		t.Fatalf("LT(1) = %q, want a", ts.LT(1).Text)
	}
	ts.Consume()
	if ts.LT(1).Text != "b" {
		t.Errorf("after Consume, LT(1) = %q, want b", ts.LT(1).Text)
	}
}

// TestTokenStream_MarkRewind pins round-trip Mark/Consume×3/Rewind.
func TestTokenStream_MarkRewind(t *testing.T) {
	ts := NewTokenStream(NewLexer("a b c d", "ts.rs2"))
	mark := ts.Mark()
	ts.Consume() // past a
	ts.Consume() // past b
	ts.Consume() // past c
	if ts.LT(1).Text != "d" {
		t.Fatalf("after 3 Consume, LT(1) = %q, want d", ts.LT(1).Text)
	}
	ts.Rewind(mark)
	if ts.LT(1).Text != "a" {
		t.Errorf("after Rewind, LT(1) = %q, want a", ts.LT(1).Text)
	}
}

// TestTokenStream_Index pins raw-index semantics: every token (including
// hidden) advances the raw index by 1.
func TestTokenStream_Index(t *testing.T) {
	ts := NewTokenStream(NewLexer("a b", "ts.rs2"))
	// Tokens: IDENT(a) WS IDENT(b) EOF — 4 tokens total.
	if got := len(ts.tokens); got != 4 {
		t.Fatalf("len(tokens) = %d, want 4", got)
	}
	if ts.Index() != 0 {
		t.Errorf("initial Index = %d, want 0", ts.Index())
	}
	ts.Consume() // skip past IDENT(a)
	if ts.Index() != 2 {
		t.Errorf("after Consume, Index = %d, want 2", ts.Index())
	}
}

// TestTokenStream_Release pins Release as a no-op.
func TestTokenStream_Release(t *testing.T) {
	ts := NewTokenStream(NewLexer("a b", "ts.rs2"))
	m := ts.Mark()
	ts.Consume()
	ts.Release(m) // no-op
	if ts.LT(1).Text != "b" {
		t.Errorf("after Release, LT(1) = %q, want b", ts.LT(1).Text)
	}
}
