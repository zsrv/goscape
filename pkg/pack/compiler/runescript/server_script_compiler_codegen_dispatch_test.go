// pkg/pack/compiler/runescript/server_script_compiler_codegen_dispatch_test.go
package runescript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// codegenErrorInjector is a test-only semantics.DynamicCommandHandler used
// solely by TestRun_HandleCodeGenerationDispatchedBeforeHalt to trigger a
// codegen-phase diagnostic error. TypeCheck merely assigns MetaUnit as the
// expression's return type so the analyze phase passes (the registered
// dyn-handler contract is enforced via checkDynamicCommand, which errors
// out with MessageCustomHandlerNoType unless ctx.SetType is called; the
// sentinel symbol's Parameters/Returns alone aren't consulted on this
// path). The Symbol fixup is handled by the TypeChecker's needsSymbol
// fallback (server_script_compiler.go siblings type_checking_expr.go:599).
// GenerateCode reports an error against the passed Diagnostics, simulating
// the "codegen produced an error" path needed by the
// NAI-211-FU-CODEGEN-ERROR-DISPATCH-PIN invariant.
type codegenErrorInjector struct{}

func (codegenErrorInjector) TypeCheck(ctx *semantics.TypeCheckingContext) {
	ctx.SetType(typ.MetaUnit)
}

func (codegenErrorInjector) GenerateCode(ctx semantics.CodeGenContext) bool {
	cgc := ctx.(*codegen.CodeGeneratorContext)
	diagnostics.ReportErrorAt(cgc.Diagnostics, cgc.Expression,
		"NAI-211-FU test-injected codegen error")
	return true
}

// TestRun_HandleCodeGenerationDispatchedBeforeHalt pins
// NAI-211-FU-CODEGEN-ERROR-DISPATCH-PIN: when the codegen phase reports an
// error, c.Handler.HandleCodeGeneration MUST be invoked BEFORE Run() halts.
// The regression shape this defends against is a reorder of codegenPhase
// (server_script_compiler.go:233-235) to dispatch the handler AFTER the
// HasErrors halt check, i.e.:
//
//	if d.HasErrors() {
//	    return nil, fmt.Errorf("codegen: diagnostics reported errors")
//	}
//	c.Handler.HandleCodeGeneration(d)
//
// Such a reorder would silently break downstream tooling (IDE plugins,
// build daemons, error-reporting wrappers) that rely on the handler being
// called on every codegen invocation. This is the symmetric pin for the
// parse-phase analog already covered by
// TestRun_ParserSyntaxErrorReachesParseDiagnostics.
func TestRun_HandleCodeGenerationDispatchedBeforeHalt(t *testing.T) {
	tmpDir := t.TempDir()
	src := "[proc,test]()\n_codegen_err_inject();\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "test.rs2"), []byte(src), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	mapper := NewSymbolMapper(nil)
	rh := newRecordingHandler()
	c := &ServerScriptCompiler{
		SourcePaths:     []string{tmpDir},
		TypeManager:     typ.NewTypeManager(),
		Mapper:          mapper,
		CommandPointers: map[string]*pointer.PointerHolder{"foo": {Required: pointer.NewPointerSet()}},
		Writer:          &noopBinaryOutput{},
		Handler:         rh,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	// Insert the sentinel command symbol so the source parses + typechecks.
	// Parameters=MetaUnit / Returns=MetaUnit matches the require_player
	// precedent in pkg/pack/compiler/codegen/smoke_test.go:160-175 and
	// satisfies the TypeChecker's nil-Parameters guard.
	c.RootTable.Insert(
		symbol.SymbolTypeServerScript(trigger.CommandTrigger),
		&symbol.ServerScriptSymbol{
			Trigger:    trigger.CommandTrigger,
			Name:       "_codegen_err_inject",
			Parameters: typ.MetaUnit,
			Returns:    typ.MetaUnit,
		},
	)
	// Install the codegen-phase error injector AFTER Setup. Setup populates
	// the DynHandlers map via RegisterAllDynCommands with built-in dyn
	// commands but does not touch this sentinel name.
	c.DynHandlers["_codegen_err_inject"] = codegenErrorInjector{}

	err := c.Run("rs2")
	if err == nil {
		t.Fatal("Run with codegen-error injector: got nil error, want non-nil")
	}
	if !strings.Contains(err.Error(), "codegen") {
		t.Errorf("Run error message: got %q, want substring %q", err.Error(), "codegen")
	}

	// Primary pin: HandleCodeGeneration was dispatched.
	if _, ok := rh.capDiags["HandleCodeGeneration"]; !ok {
		t.Fatal("HandleCodeGeneration was NOT called on codegen-error path " +
			"(NAI-211-FU regression: dispatch reordered after halt?)")
	}

	// Secondary pin: the diag handed to the handler contains the codegen
	// error. Guards against an accumulator regression where the handler is
	// invoked but receives a stale/empty Diagnostics pointer.
	cgDiag := rh.capDiags["HandleCodeGeneration"]
	if !cgDiag.HasErrors() {
		t.Errorf("HandleCodeGeneration diag has no errors; want at least one " +
			"(handler invoked but with stale/empty diag — possible accumulator regression)")
	}
}
