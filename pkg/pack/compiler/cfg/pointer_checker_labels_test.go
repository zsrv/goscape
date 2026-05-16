// pkg/pack/compiler/cfg/pointer_checker_labels_test.go
package cfg

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TestPointerChecker_LabelJump_RequirementPropagates pins that when a
// label-typed proc parameter is jumped to via `jump $param`, the label's
// required pointers propagate back to the call site.
func TestPointerChecker_LabelJump_RequirementPropagates(t *testing.T) {
	procTr := &trigger.TriggerType{ID: 0, Identifier: "proc"}
	labelTr := &trigger.TriggerType{ID: 1, Identifier: "label", Pointers: pointer.NewPointerSet(pointer.ActivePlayer)}

	// label symbol — body requires ACTIVE_PLAYER
	labelSym := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: labelTr, Name: "mylabel"}}
	labelScript := codegen.NewRuneScript("test.rs2", labelSym, labelTr, "mylabel", nil)
	lb := codegen.NewBlock(&codegen.Label{Name: "entry"})
	require := makeCommandSymbol("p_kickout")
	lb.Add(codegen.Instruction{Opcode: codegen.Command, Operand: require})
	lb.Add(codegen.Instruction{Opcode: codegen.Return})
	labelScript.Blocks = []*codegen.Block{lb}

	// caller proc — body: `gosub label_consumer(.mylabel)`
	labelMetaType := typ.NewMetaScript("label", typ.PrimitiveInt, typ.PrimitiveInt)
	consumerSym := &symbol.ServerScriptSymbol{
		ScriptSymbolFields: symbol.ScriptSymbolFields{
			Trigger:    procTr,
			Name:       "consumer",
			Parameters: labelMetaType,
		},
	}
	consumerScript := codegen.NewRuneScript("test.rs2", consumerSym, procTr, "consumer", nil)
	cb := codegen.NewBlock(&codegen.Label{Name: "entry"})
	labelParam := &symbol.LocalVariableSymbol{Name: "lbl", Type: labelMetaType}
	consumerScript.Locals = &codegen.LocalTable{
		Parameters: []*symbol.LocalVariableSymbol{labelParam},
		All:        []*symbol.LocalVariableSymbol{labelParam},
	}
	jumpCmd := makeCommandSymbol("jump")
	cb.Add(codegen.Instruction{Opcode: codegen.PushLocalVar, Operand: labelParam})
	cb.Add(codegen.Instruction{Opcode: codegen.Command, Operand: jumpCmd})
	cb.Add(codegen.Instruction{Opcode: codegen.Return})
	consumerScript.Blocks = []*codegen.Block{cb}

	// callerProc gosubs consumer with .mylabel as the static arg
	callerSym := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTr, Name: "caller"}}
	callerScript := codegen.NewRuneScript("test.rs2", callerSym, procTr, "caller", nil)
	calb := codegen.NewBlock(&codegen.Label{Name: "entry"})
	calb.Add(codegen.Instruction{Opcode: codegen.PushConstantSymbol, Operand: labelSym})
	calb.Add(codegen.Instruction{Opcode: codegen.Gosub, Operand: consumerSym})
	calb.Add(codegen.Instruction{Opcode: codegen.Return})
	callerScript.Blocks = []*codegen.Block{calb}

	cp := map[string]*pointer.PointerHolder{
		"p_kickout": {Required: pointer.NewPointerSet(pointer.ActivePlayer)},
	}
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{labelScript, consumerScript, callerScript}, cp, semantics.StrictFeatureLevel{})
	pc.Run()

	// callerScript trigger does NOT set ACTIVE_PLAYER, so propagation must
	// produce at least one uninitialized-pointer error.
	if len(errorDiagnostics(d)) == 0 {
		t.Fatalf("expected at least one error diagnostic from label propagation; got %v", d.List())
	}
}
