// pkg/pack/compiler/writer/base_context_test.go
package writer_test

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// TestNewBaseContext_PopulatesJumpAndLineTables pins that the ctor invokes
// the static helpers — both tables non-nil and CurIndex zero.
func TestNewBaseContext_PopulatesJumpAndLineTables(t *testing.T) {
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "foo"}}
	script := codegen.NewRuneScript("smoke.rs2", ss, procTrig, "foo", nil)
	b := codegen.NewBlock(&codegen.Label{Name: "entry"})
	b.Add(codegen.Instruction{
		Opcode: codegen.Return,
		Source: lexer.NodeSourceLocation{Line: 1},
	})
	script.Blocks = []*codegen.Block{b}

	ctx := writer.NewBaseContext(script)
	if ctx.Script != script {
		t.Errorf("ctx.Script: pointer mismatch")
	}
	if ctx.CurIndex != 0 {
		t.Errorf("CurIndex = %d, want 0", ctx.CurIndex)
	}
	if ctx.JumpTable == nil {
		t.Errorf("JumpTable nil")
	}
	if ctx.LineNumberTable == nil {
		t.Errorf("LineNumberTable nil")
	}
	if len(ctx.LineNumberPCs) != 1 {
		t.Errorf("LineNumberPCs length = %d, want 1", len(ctx.LineNumberPCs))
	}
}
