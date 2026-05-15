package parser

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

// newTestParserCollecting constructs a ScriptFile parser wired to a
// CollectingErrorListener for both lexer-stage and parser-stage errors.
func newTestParserCollecting(t *testing.T, src string) (*Parser, *lexer.CollectingErrorListener) {
	t.Helper()
	p := NewScriptFileParser(src, "<test>")
	cl := &lexer.CollectingErrorListener{}
	p.AddErrorListener(cl)
	return p, cl
}

func TestNewScriptFileParser_EmptyInputReturnsEmptyScriptFile(t *testing.T) {
	p, cl := newTestParserCollecting(t, "")
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("ParseScriptFile() = nil; errors: %+v", cl.Errors)
	}
	if got, want := len(sf.Scripts), 0; got != want {
		t.Fatalf("len(Scripts) = %d, want %d", got, want)
	}
	if len(cl.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", cl.Errors)
	}
}

func TestAddErrorListener_CapturesLexerStageError(t *testing.T) {
	// Source has an unterminated string — lexer fires
	// "unterminated string literal at EOF" (NAI-203-D-LEXER-ERROR-RECOVERY).
	p, cl := newTestParserCollecting(t, `[proc,x] "no close`)
	sf := p.ParseScriptFile()
	if sf != nil {
		t.Fatalf("ParseScriptFile() = %+v; want nil because lexer errors > 0", sf)
	}
	if len(cl.Errors) == 0 {
		t.Fatal("expected at least one collected error from lexer stage")
	}
}

func TestRemoveErrorListeners(t *testing.T) {
	// Bad input would normally fire a lexer error; after
	// RemoveErrorListeners the wired listener should hear nothing.
	p := NewScriptFileParser(`"no close`, "<test>")
	cl := &lexer.CollectingErrorListener{}
	p.AddErrorListener(cl)
	p.RemoveErrorListeners()
	_ = p.ParseScriptFile()
	if len(cl.Errors) != 0 {
		t.Fatalf("unexpected errors after RemoveErrorListeners: %+v", cl.Errors)
	}
}
