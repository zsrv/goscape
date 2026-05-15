package ast

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

func loc(name string, line, col, endLine, endCol int) lexer.NodeSourceLocation {
	return lexer.NodeSourceLocation{
		Name: name, Line: line, Column: col, EndLine: endLine, EndColumn: endCol,
	}
}

func TestScriptFile_Kind(t *testing.T) {
	sf := &ScriptFile{SrcLoc: loc("<test>", 1, 1, 1, 1)}
	if sf.Kind() != KindScriptFile {
		t.Fatalf("ScriptFile.Kind() = %v, want KindScriptFile", sf.Kind())
	}
}

func TestScriptFile_ChildrenWalksScripts(t *testing.T) {
	sf := &ScriptFile{
		SrcLoc: loc("<test>", 1, 1, 1, 1),
		Scripts: []*Script{
			{SrcLoc: loc("<test>", 1, 1, 1, 1)},
			{SrcLoc: loc("<test>", 2, 1, 2, 1)},
		},
	}
	children := sf.Children()
	if got, want := len(children), 2; got != want {
		t.Fatalf("len(Children()) = %d, want %d", got, want)
	}
	if _, ok := children[0].(*Script); !ok {
		t.Fatalf("Children()[0] type = %T, want *Script", children[0])
	}
}

func TestScript_Kind(t *testing.T) {
	s := &Script{SrcLoc: loc("<test>", 1, 1, 1, 1)}
	if s.Kind() != KindScript {
		t.Fatalf("Script.Kind() = %v, want KindScript", s.Kind())
	}
}

func TestScript_ChildrenIncludesTriggerNameParamsReturnsStatements(t *testing.T) {
	s := &Script{
		SrcLoc:       loc("<test>", 1, 1, 1, 1),
		Trigger:      &Identifier{SrcLoc: loc("<test>", 1, 2, 1, 4), Text: "proc"},
		Name:         &Identifier{SrcLoc: loc("<test>", 1, 7, 1, 11), Text: "test"},
		Parameters:   []*Parameter{{SrcLoc: loc("<test>", 1, 13, 1, 18)}},
		ReturnTokens: []*Token{{SrcLoc: loc("<test>", 1, 20, 1, 22), Text: "int"}},
	}
	children := s.Children()
	if got, want := len(children), 4; got != want {
		t.Fatalf("len(Children()) = %d, want %d", got, want)
	}
}

func TestParameter_Kind(t *testing.T) {
	p := &Parameter{SrcLoc: loc("<test>", 1, 1, 1, 1)}
	if p.Kind() != KindParameter {
		t.Fatalf("Parameter.Kind() = %v, want KindParameter", p.Kind())
	}
}

func TestToken_Kind(t *testing.T) {
	tok := &Token{SrcLoc: loc("<test>", 1, 1, 1, 1), Text: "int"}
	if tok.Kind() != KindToken {
		t.Fatalf("Token.Kind() = %v, want KindToken", tok.Kind())
	}
	if tok.Text != "int" {
		t.Fatalf("Token.Text = %q, want %q", tok.Text, "int")
	}
}
