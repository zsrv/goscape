package parser

import (
	"testing"
)

func TestParseScriptFile_EmptyHeaderOnly(t *testing.T) {
	p, cl := newTestParserCollecting(t, `[proc,foo]`)
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("ParseScriptFile() = nil; errors: %+v", cl.Errors)
	}
	if got, want := len(sf.Scripts), 1; got != want {
		t.Fatalf("len(Scripts) = %d, want %d", got, want)
	}
	s := sf.Scripts[0]
	if s.Trigger.Text != "proc" {
		t.Errorf("Trigger.Text = %q, want %q", s.Trigger.Text, "proc")
	}
	if s.Name.Text != "foo" {
		t.Errorf("Name.Text = %q, want %q", s.Name.Text, "foo")
	}
	if s.IsStar {
		t.Errorf("IsStar = true, want false")
	}
	if s.Parameters != nil {
		t.Errorf("Parameters = %+v, want nil", s.Parameters)
	}
	if s.ReturnTokens != nil {
		t.Errorf("ReturnTokens = %+v, want nil", s.ReturnTokens)
	}
}

func TestParseScriptFile_StarHeader(t *testing.T) {
	p, cl := newTestParserCollecting(t, `[proc,foo*]`)
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("ParseScriptFile() = nil; errors: %+v", cl.Errors)
	}
	s := sf.Scripts[0]
	if !s.IsStar {
		t.Fatal("IsStar should be true")
	}
	if got, want := s.NameString(), "foo*"; got != want {
		t.Errorf("NameString = %q, want %q", got, want)
	}
}

func TestParseScriptFile_HeaderWithParameters(t *testing.T) {
	p, cl := newTestParserCollecting(t, `[proc,foo](int $a, string $b)`)
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("ParseScriptFile() = nil; errors: %+v", cl.Errors)
	}
	s := sf.Scripts[0]
	if got, want := len(s.Parameters), 2; got != want {
		t.Fatalf("len(Parameters) = %d, want %d", got, want)
	}
	if s.Parameters[0].TypeToken.Text != "int" {
		t.Errorf("Parameters[0].TypeToken.Text = %q, want %q", s.Parameters[0].TypeToken.Text, "int")
	}
	if s.Parameters[0].Name.Text != "a" {
		t.Errorf("Parameters[0].Name.Text = %q, want %q", s.Parameters[0].Name.Text, "a")
	}
	if s.Parameters[1].TypeToken.Text != "string" {
		t.Errorf("Parameters[1].TypeToken.Text = %q, want %q", s.Parameters[1].TypeToken.Text, "string")
	}
}

func TestParseScriptFile_HeaderWithParametersAndReturns(t *testing.T) {
	p, cl := newTestParserCollecting(t, `[proc,foo](int $a)(int, string)`)
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("ParseScriptFile() = nil; errors: %+v", cl.Errors)
	}
	s := sf.Scripts[0]
	if got, want := len(s.Parameters), 1; got != want {
		t.Fatalf("len(Parameters) = %d, want %d", got, want)
	}
	if got, want := len(s.ReturnTokens), 2; got != want {
		t.Fatalf("len(ReturnTokens) = %d, want %d", got, want)
	}
	if s.ReturnTokens[0].Text != "int" {
		t.Errorf("ReturnTokens[0].Text = %q, want %q", s.ReturnTokens[0].Text, "int")
	}
	if s.ReturnTokens[1].Text != "string" {
		t.Errorf("ReturnTokens[1].Text = %q, want %q", s.ReturnTokens[1].Text, "string")
	}
}

func TestParseScriptFile_HeaderWithEmptyParameterList(t *testing.T) {
	p, cl := newTestParserCollecting(t, `[proc,foo]()`)
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("ParseScriptFile() = nil; errors: %+v", cl.Errors)
	}
	s := sf.Scripts[0]
	if s.Parameters == nil {
		t.Fatalf("Parameters = nil, want non-nil empty slice")
	}
	if got, want := len(s.Parameters), 0; got != want {
		t.Fatalf("len(Parameters) = %d, want %d", got, want)
	}
}

func TestParseScriptFile_TwoScripts(t *testing.T) {
	src := "[proc,a]\n[proc,b]\n"
	p, cl := newTestParserCollecting(t, src)
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("ParseScriptFile() = nil; errors: %+v", cl.Errors)
	}
	if got, want := len(sf.Scripts), 2; got != want {
		t.Fatalf("len(Scripts) = %d, want %d", got, want)
	}
	if sf.Scripts[0].Name.Text != "a" || sf.Scripts[1].Name.Text != "b" {
		t.Errorf("names = %q,%q; want a,b", sf.Scripts[0].Name.Text, sf.Scripts[1].Name.Text)
	}
}

func TestParseScriptFile_ScriptNameMultiIdentifier(t *testing.T) {
	p, cl := newTestParserCollecting(t, `[label,my hud]`)
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("ParseScriptFile() = nil; errors: %+v", cl.Errors)
	}
	s := sf.Scripts[0]
	if s.Name.Text != "my hud" {
		t.Errorf("Name.Text = %q, want %q", s.Name.Text, "my hud")
	}
}

func TestParseScriptFile_HeaderMissingRBrackReportsError(t *testing.T) {
	p, cl := newTestParserCollecting(t, `[proc,foo`)
	sf := p.ParseScriptFile()
	if sf != nil {
		t.Fatalf("ParseScriptFile() = %+v, want nil", sf)
	}
	if len(cl.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
}

