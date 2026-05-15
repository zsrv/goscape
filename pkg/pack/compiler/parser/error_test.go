package parser

import (
	"strings"
	"testing"
)

func TestError_MissingSemicolon(t *testing.T) {
	p, cl := newTestParserCollecting(t, "[proc,t] return")
	sf := p.ParseScriptFile()
	if sf != nil {
		t.Fatalf("ParseScriptFile() = %+v, want nil", sf)
	}
	if len(cl.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
	found := false
	for _, e := range cl.Errors {
		if strings.Contains(e.Msg, "SEMICOLON") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected SEMICOLON error; got: %+v", cl.Errors)
	}
}

func TestError_UnexpectedTokenAtStatementStart(t *testing.T) {
	// '*' alone is not a valid statement-start. Parser reports and syncs.
	p, cl := newTestParserCollecting(t, "[proc,t] * ;")
	sf := p.ParseScriptFile()
	if sf != nil {
		t.Fatalf("ParseScriptFile() = %+v, want nil", sf)
	}
	if len(cl.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
}

func TestError_UnterminatedSwitch(t *testing.T) {
	p, cl := newTestParserCollecting(t, "[proc,t] switch_int (1) { case 1 : ;")
	sf := p.ParseScriptFile()
	if sf != nil {
		t.Fatalf("ParseScriptFile() = %+v, want nil", sf)
	}
	if len(cl.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
}

func TestError_InvalidEscapeInString(t *testing.T) {
	// `\n` is not in the allowed escape set (\\ \' \" \<).
	p, cl := newTestParserCollecting(t, `[proc,t] "bad\n";`)
	sf := p.ParseScriptFile()
	if sf != nil {
		t.Fatalf("ParseScriptFile() = %+v, want nil", sf)
	}
	if len(cl.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
	found := false
	for _, e := range cl.Errors {
		if strings.Contains(e.Msg, "escape") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected escape error; got: %+v", cl.Errors)
	}
}

func TestClientScript_BareName(t *testing.T) {
	p, cl := newTestParserCollecting(t, "some_handler")
	c := p.ParseClientScript()
	if c == nil {
		t.Fatalf("ParseClientScript() = nil; errors: %+v", cl.Errors)
	}
	if c.Name.Text != "some_handler" {
		t.Errorf("Name = %q, want %q", c.Name.Text, "some_handler")
	}
	if got, want := len(c.Arguments), 0; got != want {
		t.Errorf("len(Arguments) = %d, want %d", got, want)
	}
	if got, want := len(c.TransmitList), 0; got != want {
		t.Errorf("len(TransmitList) = %d, want %d", got, want)
	}
}

func TestClientScript_WithArgsAndTriggers(t *testing.T) {
	p, cl := newTestParserCollecting(t, "some_handler(1, 2){var1}")
	c := p.ParseClientScript()
	if c == nil {
		t.Fatalf("ParseClientScript() = nil; errors: %+v", cl.Errors)
	}
	if got, want := len(c.Arguments), 2; got != want {
		t.Errorf("len(Arguments) = %d, want %d", got, want)
	}
	if got, want := len(c.TransmitList), 1; got != want {
		t.Errorf("len(TransmitList) = %d, want %d", got, want)
	}
}

func TestClientScript_WithOnlyTriggers(t *testing.T) {
	p, cl := newTestParserCollecting(t, "some_handler{var1, var2}")
	c := p.ParseClientScript()
	if c == nil {
		t.Fatalf("ParseClientScript() = nil; errors: %+v", cl.Errors)
	}
	if got, want := len(c.Arguments), 0; got != want {
		t.Errorf("len(Arguments) = %d, want %d", got, want)
	}
	if got, want := len(c.TransmitList), 2; got != want {
		t.Errorf("len(TransmitList) = %d, want %d", got, want)
	}
}
