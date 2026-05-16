// pkg/pack/compiler/cfg/pointer_checker_validation_test.go
package cfg

import (
	"fmt"
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

// TestPointerChecker_Run_UninitializedReported pins that a command requiring
// ACTIVE_PLAYER in a proc whose trigger does NOT set ACTIVE_PLAYER reports
// exactly one MessagePointerUninitialized.
func TestPointerChecker_Run_UninitializedReported(t *testing.T) {
	tr := &trigger.TriggerType{ID: 0, Identifier: "proc"}
	sym := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "p1"}}
	rs := codegen.NewRuneScript("test.rs2", sym, tr, "p1", nil)
	b := codegen.NewBlock(&codegen.Label{Name: "entry"})
	cmd := makeCommandSymbol("p_kickout")
	b.Add(codegen.Instruction{Opcode: codegen.Command, Operand: cmd})
	b.Add(codegen.Instruction{Opcode: codegen.Return})
	rs.Blocks = []*codegen.Block{b}

	cp := map[string]*pointer.PointerHolder{
		"p_kickout": {Required: pointer.NewPointerSet(pointer.ActivePlayer)},
	}
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{rs}, cp, semantics.StrictFeatureLevel{})
	pc.Run()

	errs := errorDiagnostics(d)
	if len(errs) != 1 {
		t.Fatalf("got %d error diagnostics, want 1: %v", len(errs), d.List())
	}
	msg := fmt.Sprintf(errs[0].Message, errs[0].MessageArgs...)
	if !strings.Contains(msg, "uninitialized pointer active_player") {
		t.Errorf("diagnostic message = %q, want substring \"uninitialized pointer active_player\"", msg)
	}
}

// TestPointerChecker_Run_TriggerSetsPointerNoDiagnostic pins that when the
// trigger implicitly sets the required pointer, no diagnostic is reported.
func TestPointerChecker_Run_TriggerSetsPointerNoDiagnostic(t *testing.T) {
	tr := &trigger.TriggerType{
		ID:         0,
		Identifier: "opheld",
		Pointers:   pointer.NewPointerSet(pointer.ActivePlayer),
	}
	sym := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "h"}}
	rs := codegen.NewRuneScript("test.rs2", sym, tr, "h", nil)
	b := codegen.NewBlock(&codegen.Label{Name: "entry"})
	cmd := makeCommandSymbol("p_kickout")
	b.Add(codegen.Instruction{Opcode: codegen.Command, Operand: cmd})
	b.Add(codegen.Instruction{Opcode: codegen.Return})
	rs.Blocks = []*codegen.Block{b}

	cp := map[string]*pointer.PointerHolder{
		"p_kickout": {Required: pointer.NewPointerSet(pointer.ActivePlayer)},
	}
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{rs}, cp, semantics.StrictFeatureLevel{})
	pc.Run()

	if len(errorDiagnostics(d)) != 0 {
		t.Errorf("got %d error diagnostics, want 0: %v", len(errorDiagnostics(d)), d.List())
	}
}

// TestPointerChecker_Run_CorruptedReported pins the corrupted-pointer arm:
// a command that corrupts ACTIVE_PLAYER followed by a command that
// requires it.
func TestPointerChecker_Run_CorruptedReported(t *testing.T) {
	tr := &trigger.TriggerType{
		ID:         0,
		Identifier: "opheld",
		Pointers:   pointer.NewPointerSet(pointer.ActivePlayer),
	}
	sym := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "h"}}
	rs := codegen.NewRuneScript("test.rs2", sym, tr, "h", nil)
	b := codegen.NewBlock(&codegen.Label{Name: "entry"})
	corrupter := makeCommandSymbol("p_finduid")
	require := makeCommandSymbol("p_kickout")
	b.Add(codegen.Instruction{Opcode: codegen.Command, Operand: corrupter})
	b.Add(codegen.Instruction{Opcode: codegen.Command, Operand: require})
	b.Add(codegen.Instruction{Opcode: codegen.Return})
	rs.Blocks = []*codegen.Block{b}

	cp := map[string]*pointer.PointerHolder{
		"p_finduid": {Corrupted: pointer.NewPointerSet(pointer.ActivePlayer)},
		"p_kickout": {Required: pointer.NewPointerSet(pointer.ActivePlayer)},
	}
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{rs}, cp, semantics.StrictFeatureLevel{})
	pc.Run()

	errs := errorDiagnostics(d)
	if len(errs) != 1 {
		t.Fatalf("got %d error diagnostics, want 1: %v", len(errs), d.List())
	}
	msg := fmt.Sprintf(errs[0].Message, errs[0].MessageArgs...)
	if !strings.Contains(msg, "corrupted pointer active_player") {
		t.Errorf("diagnostic = %q, want substring \"corrupted pointer active_player\"", msg)
	}
}

// TestPointerChecker_Run_ProtectedPopRequiresP pins the protected-write
// arm: PopVar on a protected VarPlayer requires P_ACTIVE_PLAYER.
func TestPointerChecker_Run_ProtectedPopRequiresP(t *testing.T) {
	tr := &trigger.TriggerType{
		ID:         0,
		Identifier: "opheld",
		Pointers:   pointer.NewPointerSet(pointer.ActivePlayer), // sets ACTIVE_PLAYER only
	}
	sym := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "h"}}
	rs := codegen.NewRuneScript("test.rs2", sym, tr, "h", nil)
	b := codegen.NewBlock(&codegen.Label{Name: "entry"})
	vp := &symbol.BasicSymbol{Name: "score", Type: makeVarPlayerType(), IsProtected: true}
	b.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 0})
	b.Add(codegen.Instruction{Opcode: codegen.PopVar, Operand: vp})
	b.Add(codegen.Instruction{Opcode: codegen.Return})
	rs.Blocks = []*codegen.Block{b}

	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{rs}, map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})
	pc.Run()

	errs := errorDiagnostics(d)
	if len(errs) != 1 {
		t.Fatalf("got %d error diagnostics, want 1 (p_active_player uninitialized): %v", len(errs), d.List())
	}
	msg := fmt.Sprintf(errs[0].Message, errs[0].MessageArgs...)
	if !strings.Contains(msg, "uninitialized pointer p_active_player") {
		t.Errorf("diagnostic = %q, want substring \"uninitialized pointer p_active_player\"", msg)
	}
}

// errorDiagnostics filters d to only error-severity entries.
func errorDiagnostics(d *diagnostics.Diagnostics) []diagnostics.Diagnostic {
	var out []diagnostics.Diagnostic
	for _, e := range d.List() {
		if e.IsError() {
			out = append(out, e)
		}
	}
	return out
}

// makeVarPlayerType returns a *VarPlayerType for tests. Since VarPlayerType
// has an unexported embed, we use the typ.NewVarPlayerType constructor if
// available, else type-switch fallback.
func makeVarPlayerType() *typ.VarPlayerType {
	// Constructor signature pinned by pkg/pack/compiler/type/gamevar.go:
	//   func NewVarPlayerType(inner typ.Type) *VarPlayerType
	return typ.NewVarPlayerType(typ.PrimitiveInt)
}
