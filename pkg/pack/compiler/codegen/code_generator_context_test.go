package codegen

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
)

func TestCodeGeneratorContext_SatisfiesMarker(t *testing.T) {
	cg := &CodeGenerator{}
	cgc := NewCodeGeneratorContext(cg, symbol.NewSymbolTable(nil), &ast.IntegerLiteral{}, &diagnostics.Diagnostics{})
	var _ semantics.CodeGenContext = cgc
}

func TestCodeGeneratorContext_ArgumentsExtraction(t *testing.T) {
	cmd := &ast.CommandCallExpression{
		Arguments:  []ast.Expression{&ast.IntegerLiteral{}, &ast.StringLiteral{}},
		Arguments2: []ast.Expression{&ast.IntegerLiteral{}}, // IsStar() == true
	}
	cgc := NewCodeGeneratorContext(&CodeGenerator{}, symbol.NewSymbolTable(nil), cmd, &diagnostics.Diagnostics{})

	if got := cgc.Arguments(); len(got) != 2 {
		t.Errorf("Arguments len: got %d, want 2", len(got))
	}
	if got := cgc.Arguments2(); len(got) != 1 {
		t.Errorf("Arguments2 len (CommandCall Arguments2 set): got %d, want 1", len(got))
	}

	// CommandCall without Arguments2 → Arguments2() returns nil.
	cmdPlain := &ast.CommandCallExpression{Arguments: []ast.Expression{&ast.IntegerLiteral{}}}
	cgc = NewCodeGeneratorContext(&CodeGenerator{}, symbol.NewSymbolTable(nil), cmdPlain, &diagnostics.Diagnostics{})
	if got := cgc.Arguments2(); got != nil {
		t.Errorf("Arguments2(no Arguments2 set): want nil, got %v", got)
	}

	// Non-CommandCall (ProcCall): Arguments2 returns nil.
	proc := &ast.ProcCallExpression{Arguments: []ast.Expression{&ast.IntegerLiteral{}}}
	cgc = NewCodeGeneratorContext(&CodeGenerator{}, symbol.NewSymbolTable(nil), proc, &diagnostics.Diagnostics{})
	if got := cgc.Arguments(); len(got) != 1 {
		t.Errorf("Arguments(ProcCall) len: got %d, want 1", len(got))
	}
	if got := cgc.Arguments2(); got != nil {
		t.Errorf("Arguments2(ProcCall): want nil, got %v", got)
	}
}
