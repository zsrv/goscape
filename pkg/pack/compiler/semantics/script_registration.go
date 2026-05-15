// pkg/pack/compiler/semantics/script_registration.go
package semantics

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// ScriptRegistration is the first-pass semantic walker. It registers each
// script's ServerScriptSymbol into the passed-in root SymbolTable and
// writes seven fields onto each Script + one field onto each Parameter.
// Mirrors TS src/compiler/semantics/ScriptRegistration.ts.
//
// Two-stage compiler pipeline:
//  1. ScriptRegistration (THIS) — register script symbols + parameter symbols.
//  2. TypeChecking (NAI-206) — walk expressions/statements, resolve
//     references, type-check operands, populate the remaining AST fields.
//
// NAI-205-D-NO-VISIT-BLOCK: TS ScriptRegistration calls accept(this) on
// Script.Statements (block walk; no-ops via AstVisitor base class).
// Goscape skips the walk entirely since the no-op visit has no observable
// effect on the seven AST fields ScriptRegistration writes.
type ScriptRegistration struct {
	typeManager    *typ.TypeManager
	triggerManager *trigger.TriggerManager
	rootTable      *symbol.SymbolTable
	diagnostics    *diagnostics.Diagnostics
	features       StrictFeatureLevel

	// Stack of nested SymbolTables; tables[0] is the active table.
	// Mirrors TS `private readonly tables: SymbolTable[]`.
	tables []*symbol.SymbolTable

	// Cached lookup for the `category` type, used for category-subject
	// resolution. nil if the TypeManager has no 'category' type registered.
	categoryType typ.Type
}

// NewScriptRegistration constructs a ScriptRegistration walker.
// Mirrors TS ScriptRegistration constructor L52-62.
func NewScriptRegistration(
	tm *typ.TypeManager,
	trm *trigger.TriggerManager,
	rootTable *symbol.SymbolTable,
	d *diagnostics.Diagnostics,
	features StrictFeatureLevel,
) *ScriptRegistration {
	sr := &ScriptRegistration{
		typeManager:    tm,
		triggerManager: trm,
		rootTable:      rootTable,
		diagnostics:    d,
		features:       features,
		categoryType:   tm.FindOrNil("category", false),
	}
	// Push the file-level table (the TS constructor's `tables.unshift(...)`).
	sr.tables = []*symbol.SymbolTable{rootTable.CreateSubTable()}
	return sr
}

// activeTable returns the SymbolTable at the top of the stack.
// Mirrors TS `private get table()`.
func (sr *ScriptRegistration) activeTable() *symbol.SymbolTable {
	return sr.tables[0]
}

// tableStackDepth is a test-only helper.
func (sr *ScriptRegistration) tableStackDepth() int {
	return len(sr.tables)
}

// withScopedTable runs block with a fresh sub-table at the top of the stack.
// Mirrors TS `createScopedTable(block)` L78-86.
func (sr *ScriptRegistration) withScopedTable(block func()) {
	sub := sr.activeTable().CreateSubTable()
	sr.tables = append([]*symbol.SymbolTable{sub}, sr.tables...)
	defer func() {
		sr.tables = sr.tables[1:]
	}()
	block()
}

// Visit is the public entry. Mirrors TS visitScriptFile L88-94.
func (sr *ScriptRegistration) Visit(file *ast.ScriptFile) {
	for _, script := range file.Scripts {
		sr.withScopedTable(func() {
			sr.visitScript(script)
		})
	}
}

// visitScript is the per-script walker. Skeleton — implementation lands
// in T10/T11/T12.
func (sr *ScriptRegistration) visitScript(script *ast.Script) {
	// T10 fills this in.
	_ = script
}
