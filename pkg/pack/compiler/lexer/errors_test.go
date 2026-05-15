package lexer

import "testing"

// TestDiscardErrorListener_AcceptsErrors pins that DiscardErrorListener
// is a no-op — accepts errors silently.
func TestDiscardErrorListener_AcceptsErrors(t *testing.T) {
	var l DiscardErrorListener
	l.SyntaxError("x.rs2", 1, 1, "oops")
	// no panic, no return value — success is silence
}

// TestCollectingErrorListener_RecordsErrors pins that
// CollectingErrorListener accumulates SyntaxError records in order.
func TestCollectingErrorListener_RecordsErrors(t *testing.T) {
	l := &CollectingErrorListener{}
	l.SyntaxError("x.rs2", 1, 1, "first")
	l.SyntaxError("x.rs2", 2, 5, "second")
	if got, want := len(l.Errors), 2; got != want {
		t.Fatalf("len(Errors) = %d, want %d", got, want)
	}
	if l.Errors[0].Msg != "first" || l.Errors[1].Msg != "second" {
		t.Errorf("errors out of order: %+v", l.Errors)
	}
	if l.Errors[1].Line != 2 || l.Errors[1].Column != 5 {
		t.Errorf("second error location wrong: %+v", l.Errors[1])
	}
	if l.Errors[0].SourceName != "x.rs2" {
		t.Errorf("source name not captured: %+v", l.Errors[0])
	}
}
