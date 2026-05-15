// pkg/pack/compiler/semantics/script_registration.go
package semantics

import (
	"strings"

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

// isDisabledTypeName mirrors TS L66-74 — feature-flag-based disable of
// boolean / enum / struct / dbtable / dbrow / dbcolumn (+ their array forms).
func (sr *ScriptRegistration) isDisabledTypeName(typeText string) bool {
	text := strings.ToLower(typeText)
	base := text
	if strings.HasSuffix(text, "array") {
		base = text[:len(text)-5]
	}
	if sr.features.DisableBooleans && base == typ.PrimitiveBoolean.Representation() {
		return true
	}
	if sr.features.DisableEnums && base == "enum" {
		return true
	}
	if sr.features.DisableStructs && base == "struct" {
		return true
	}
	if sr.features.DisableDBTables && (base == "dbtable" || base == "dbrow" || base == "dbcolumn") {
		return true
	}
	return false
}

// isDisabledTrigger mirrors TS L76-80.
func (sr *ScriptRegistration) isDisabledTrigger(t *trigger.TriggerType) bool {
	if t == nil {
		return false
	}
	if sr.features.DisableProcs && t.Identifier == "proc" {
		return true
	}
	return false
}

// visitScript is the per-script walker. Mirrors TS ScriptRegistration.ts L107-182.
func (sr *ScriptRegistration) visitScript(script *ast.Script) {
	// L98-105: trigger lookup.
	trig := sr.triggerManager.FindOrNil(script.Trigger.Text)
	if trig == nil {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Trigger,
			diagnostics.MessageScriptTriggerInvalid, script.Trigger.Text)
	} else {
		script.TriggerType = trig
		if sr.isDisabledTrigger(trig) {
			diagnostics.ReportErrorAt(sr.diagnostics, script.Trigger,
				diagnostics.MessageFeatureDisabledTrigger, trig.Identifier)
		}
	}

	// L107-117: '*' suffix only valid on command trigger.
	if script.IsStar && trig != trigger.CommandTrigger {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageScriptCommandOnly)
	}

	// L119-120: subject validation. Implementation lives in T11.
	sr.checkScriptSubject(trig, script)

	// L122-125: visit parameters (T12 fills in visitParameter).
	for _, p := range script.Parameters {
		sr.visitParameter(p)
	}

	// L127-128: ParameterType = TupleFromList(params' symbol types).
	paramTypes := make([]typ.Type, 0, len(script.Parameters))
	for _, p := range script.Parameters {
		var pt typ.Type = typ.MetaError
		if p.Symbol != nil {
			if local, ok := p.Symbol.(*symbol.LocalVariableSymbol); ok {
				pt = local.Type
			}
		}
		paramTypes = append(paramTypes, pt)
	}
	script.ParameterType = typ.TupleFromList(paramTypes)

	// L130-131: parameter-vs-trigger compat check (T12).
	sr.checkScriptParameters(trig, script, script.Parameters)

	// L133-153: return type construction.
	if len(script.ReturnTokens) > 0 {
		returns := make([]typ.Type, 0, len(script.ReturnTokens))
		for _, tok := range script.ReturnTokens {
			var ty typ.Type
			if sr.isDisabledTypeName(tok.Text) {
				diagnostics.ReportErrorAt(sr.diagnostics, tok,
					diagnostics.MessageFeatureDisabledType, tok.Text)
				ty = typ.MetaError
			} else {
				ty = sr.typeManager.FindOrNil(tok.Text, false)
				if ty == nil {
					diagnostics.ReportErrorAt(sr.diagnostics, tok,
						diagnostics.MessageGenericInvalidType, tok.Text)
					ty = typ.MetaError
				}
			}
			returns = append(returns, ty)
		}
		script.ReturnType = typ.TupleFromList(returns)
	} else {
		// L154-155: default based on trigger.
		switch {
		case trig == nil:
			script.ReturnType = typ.MetaError
		case trig.AllowReturns:
			script.ReturnType = typ.MetaUnit
		default:
			script.ReturnType = typ.MetaNothing
		}
	}

	// L157: return-vs-trigger compat check (T12).
	sr.checkScriptReturns(trig, script)

	// L159-169: insert ServerScriptSymbol into root table (gated on trigger
	// being present + not disabled).
	if trig != nil && !sr.isDisabledTrigger(trig) {
		ssym := &symbol.ServerScriptSymbol{
			ScriptSymbolFields: symbol.ScriptSymbolFields{
				Trigger:    trig,
				Name:       script.NameString(),
				Parameters: typeRefAsType(script.ParameterType),
				Returns:    typeRefAsType(script.ReturnType),
			},
		}
		inserted := sr.rootTable.Insert(symbol.SymbolTypeServerScript(trig), ssym)
		if !inserted {
			diagnostics.ReportErrorAt(sr.diagnostics, script,
				diagnostics.MessageScriptRedeclaration, trig.Identifier, script.NameString())
		} else {
			script.Symbol = ssym
		}
	}

	// L172: file-level block table assignment.
	script.Block = sr.activeTable()
}

// typeRefAsType unwraps an ast.TypeRef that this package wrote (always a
// typ.Type) back into typ.Type. The field is interface-typed only for the
// cyclic-import bridge; the concrete value is always typ.Type.
func typeRefAsType(t ast.TypeRef) typ.Type {
	if t == nil {
		return typ.MetaError
	}
	return t.(typ.Type)
}

// checkScriptSubject is the T11 stub.
func (sr *ScriptRegistration) checkScriptSubject(t *trigger.TriggerType, script *ast.Script) {
	_ = t
	_ = script
}

// visitParameter is the T12 stub.
func (sr *ScriptRegistration) visitParameter(p *ast.Parameter) {
	_ = p
}

// checkScriptParameters is the T12 stub.
func (sr *ScriptRegistration) checkScriptParameters(t *trigger.TriggerType, script *ast.Script, params []*ast.Parameter) {
	_ = t
	_ = script
	_ = params
}

// checkScriptReturns is the T12 stub.
func (sr *ScriptRegistration) checkScriptReturns(t *trigger.TriggerType, script *ast.Script) {
	_ = t
	_ = script
}
