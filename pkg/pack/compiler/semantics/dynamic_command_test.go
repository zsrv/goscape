package semantics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestTypeCheckingContext_ArgumentsCallExpression(t *testing.T) {
	call := &ast.CommandCallExpression{
		Name:      &ast.Identifier{Text: "foo"},
		Arguments: []ast.Expression{&ast.IntegerLiteral{Value: 1}, &ast.IntegerLiteral{Value: 2}},
	}
	ctx := newTypeCheckingContext(nil, nil, call, &diagnostics.Diagnostics{})
	if got := len(ctx.Arguments()); got != 2 {
		t.Errorf("Arguments() length = %d, want 2", got)
	}
}

func TestTypeCheckingContext_ArgumentsNonCallExpression(t *testing.T) {
	ident := &ast.Identifier{Text: "foo"}
	ctx := newTypeCheckingContext(nil, nil, ident, &diagnostics.Diagnostics{})
	if got := len(ctx.Arguments()); got != 0 {
		t.Errorf("Arguments() length = %d, want 0 for non-call expression", got)
	}
}

// Compile-time guard: an empty struct satisfies DynamicCommandHandler.
type _stubHandler struct{}

func (_stubHandler) TypeCheck(ctx *TypeCheckingContext) {}

func TestDynamicCommandHandler_Interface(t *testing.T) {
	var _ DynamicCommandHandler = _stubHandler{}
	_ = typ.MetaUnit // import-keepalive
}
