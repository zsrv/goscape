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
