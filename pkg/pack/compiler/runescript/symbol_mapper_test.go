// pkg/pack/compiler/runescript/symbol_mapper_test.go
package runescript_test

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TestSymbolMapper_PutGetCommand pins the command-symbol path: script
// symbol whose Trigger == CommandTrigger looks up via the commands map,
// dot-prefix stripped per TS SymbolMapper.get L60-68.
func TestSymbolMapper_PutGetCommand(t *testing.T) {
	d := &diagnostics.Diagnostics{}
	m := runescript.NewSymbolMapper(d)
	m.PutCommand(42, "mes")
	cmd := &symbol.ServerScriptSymbol{
		Trigger: trigger.CommandTrigger,
		Name:    "mes",
	}
	if got := m.Get(cmd); got != 42 {
		t.Errorf("Get(mes) = %d, want 42", got)
	}
	// Dot-prefixed name: TS strips everything up to and including the first dot.
	dot := &symbol.ServerScriptSymbol{
		Trigger: trigger.CommandTrigger,
		Name:    ".mes",
	}
	if got := m.Get(dot); got != 42 {
		t.Errorf("Get(.mes) = %d, want 42 (dot stripped)", got)
	}
}

// TestSymbolMapper_PutGetScript pins the script-symbol path: non-command
// trigger looks up via the scripts map keyed by "[ident,name]".
func TestSymbolMapper_PutGetScript(t *testing.T) {
	d := &diagnostics.Diagnostics{}
	m := runescript.NewSymbolMapper(d)
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	m.PutScript(7, "[proc,foo]")
	sym := &symbol.ServerScriptSymbol{
		Trigger: procTrig, Name: "foo",
	}
	if got := m.Get(sym); got != 7 {
		t.Errorf("Get([proc,foo]) = %d, want 7", got)
	}
}

// TestSymbolMapper_PutGetSymbol pins the BasicSymbol path: direct map lookup.
func TestSymbolMapper_PutGetSymbol(t *testing.T) {
	d := &diagnostics.Diagnostics{}
	m := runescript.NewSymbolMapper(d)
	b := &symbol.BasicSymbol{Name: "weapon1", Type: typ.PrimitiveInt}
	m.PutSymbol(99, b)
	if got := m.Get(b); got != 99 {
		t.Errorf("Get(weapon1) = %d, want 99", got)
	}
}

// TestSymbolMapper_MissingCommand pins the report-and-return-(-1) semantics
// for an unknown command symbol. Verifies a diagnostic was reported.
func TestSymbolMapper_MissingCommand(t *testing.T) {
	d := &diagnostics.Diagnostics{}
	m := runescript.NewSymbolMapper(d)
	cmd := &symbol.ServerScriptSymbol{
		Trigger: trigger.CommandTrigger,
		Name:    "ghost",
	}
	if got := m.Get(cmd); got != -1 {
		t.Errorf("Get(ghost) = %d, want -1", got)
	}
	if len(d.List()) != 1 {
		t.Errorf("diagnostics: got %d, want 1", len(d.List()))
	}
}

// TestSymbolMapper_MissingScript pins -1 + diagnostic for unknown script.
func TestSymbolMapper_MissingScript(t *testing.T) {
	d := &diagnostics.Diagnostics{}
	m := runescript.NewSymbolMapper(d)
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	sym := &symbol.ServerScriptSymbol{
		Trigger: procTrig, Name: "ghost",
	}
	if got := m.Get(sym); got != -1 {
		t.Errorf("Get([proc,ghost]) = %d, want -1", got)
	}
	if len(d.List()) != 1 {
		t.Errorf("diagnostics: got %d, want 1", len(d.List()))
	}
}

// TestSymbolMapper_MissingBasicPanics pins the TS `throw new Error` parity
// for an unmapped non-script symbol.
func TestSymbolMapper_MissingBasicPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Get on unmapped BasicSymbol did not panic")
		}
	}()
	d := &diagnostics.Diagnostics{}
	m := runescript.NewSymbolMapper(d)
	b := &symbol.BasicSymbol{Name: "x", Type: typ.PrimitiveInt}
	m.Get(b)
}

// TestSymbolMapper_DuplicateSymbolDispatches pins duplicate-PutSymbol behavior:
// reports diagnostic, leaves first mapping intact.
func TestSymbolMapper_DuplicateSymbolDispatches(t *testing.T) {
	d := &diagnostics.Diagnostics{}
	m := runescript.NewSymbolMapper(d)
	b := &symbol.BasicSymbol{Name: "x", Type: typ.PrimitiveInt}
	m.PutSymbol(1, b)
	m.PutSymbol(2, b)
	if got := m.Get(b); got != 1 {
		t.Errorf("Get after duplicate Put = %d, want 1 (first wins)", got)
	}
	if len(d.List()) != 1 {
		t.Errorf("diagnostics on duplicate: got %d, want 1", len(d.List()))
	}
}
