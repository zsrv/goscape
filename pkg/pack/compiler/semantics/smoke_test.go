// pkg/pack/compiler/semantics/smoke_test.go
package semantics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/parser"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TestSmoke_Parser_ScriptRegistration_E2E parses a 2-script source file
// and runs ScriptRegistration. Asserts no diagnostics + root-table state.
func TestSmoke_Parser_ScriptRegistration_E2E(t *testing.T) {
	src := "[proc,foo]\nreturn;\n[label,bar]\nreturn;\n"
	p := parser.NewScriptFileParser(src, "smoke.rs2")
	file := p.ParseScriptFile()
	if file == nil {
		t.Fatal("parser returned nil ScriptFile")
	}
	if got, want := len(file.Scripts), 2; got != want {
		t.Fatalf("parsed Scripts len = %d, want %d", got, want)
	}

	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	label := makeLabelTrigger()
	_ = trm.RegisterTrigger(proc)
	_ = trm.RegisterTrigger(label)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	sr.Visit(file)

	if d.HasErrors() {
		t.Fatalf("smoke had errors: %+v", d.List())
	}

	for _, s := range file.Scripts {
		if s.Symbol == nil {
			t.Fatalf("script %q Symbol nil", s.NameString())
		}
		if s.Block == nil {
			t.Fatalf("script %q Block nil", s.NameString())
		}
		if s.TriggerType == nil {
			t.Fatalf("script %q TriggerType nil", s.NameString())
		}
	}

	if got := root.Find(symbol.SymbolTypeServerScript(proc), "foo"); got == nil {
		t.Fatal("root table missing proc/foo")
	}
	if got := root.Find(symbol.SymbolTypeServerScript(label), "bar"); got == nil {
		t.Fatal("root table missing label/bar")
	}

	_ = typ.MetaUnit // keep import even if not asserted
}
