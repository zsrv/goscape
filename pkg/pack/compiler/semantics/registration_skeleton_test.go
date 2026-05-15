// pkg/pack/compiler/semantics/registration_skeleton_test.go
package semantics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// newTestFixture returns the four-tuple every ScriptRegistration test wires
// up. Type+trigger managers are minimal (no triggers/types registered);
// callers add what their test needs.
func newTestFixture(t *testing.T) (*typ.TypeManager, *trigger.TriggerManager, *symbol.SymbolTable, *diagnostics.Diagnostics) {
	t.Helper()
	return typ.NewTypeManager(), trigger.NewTriggerManager(), symbol.NewSymbolTable(nil), &diagnostics.Diagnostics{}
}

func TestScriptRegistration_NewVisit_Empty(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	sr.Visit(&ast.ScriptFile{}) // no scripts → no panic, no diagnostics

	if got := d.List(); len(got) != 0 {
		t.Fatalf("diagnostics for empty file: %+v", got)
	}
}

func TestScriptRegistration_ScopedTable_PushPop(t *testing.T) {
	// Each visited script gets a fresh sub-table. After visit, the stack
	// returns to its original depth.
	tm, trm, root, d := newTestFixture(t)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	if got := sr.tableStackDepth(); got != 1 {
		t.Fatalf("initial table-stack depth = %d, want 1 (file-level table)", got)
	}
	// withScopedTable should push, run, and pop back to 1.
	sr.withScopedTable(func() {
		if got := sr.tableStackDepth(); got != 2 {
			t.Fatalf("inside scoped block: depth = %d, want 2", got)
		}
	})
	if got := sr.tableStackDepth(); got != 1 {
		t.Fatalf("after scoped block: depth = %d, want 1", got)
	}
}
