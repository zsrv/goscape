// pkg/pack/compiler/semantics/script_registration.go
package semantics

import (
	"strconv"
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

// checkScriptSubject validates that the script's subject (the name field
// after the trigger) is allowed by the trigger's SubjectMode.
// Mirrors TS L184-217.
func (sr *ScriptRegistration) checkScriptSubject(t *trigger.TriggerType, script *ast.Script) {
	if t == nil {
		return
	}
	mode := t.SubjectMode
	if mode == nil {
		return
	}

	subject := script.Name.Text
	if strings.Contains(subject, " ") {
		_, isType := trigger.IsTypeMode(mode)
		if !isType {
			diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
				diagnostics.MessageScriptSubjectNoSpaces, t.Identifier)
			return
		}
	}

	// Name mode allows anything as the subject.
	if mode == trigger.ModeName {
		return
	}

	// Check for global subject.
	if subject == "_" {
		sr.checkGlobalScriptSubject(t, script)
		return
	}

	// Check for category reference subject.
	if strings.HasPrefix(subject, "_") {
		sr.checkCategoryScriptSubject(t, script, subject[1:])
		return
	}

	// Check for reference subject.
	sr.checkTypeScriptSubject(t, script, subject)
}

// checkGlobalScriptSubject validates that `_` subjects are allowed for this
// trigger's SubjectMode. Mirrors TS L222-239.
func (sr *ScriptRegistration) checkGlobalScriptSubject(t *trigger.TriggerType, script *ast.Script) {
	mode := t.SubjectMode
	if mode == trigger.ModeNone {
		return
	}
	if tm, ok := trigger.IsTypeMode(mode); ok {
		if !tm.Global {
			diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
				diagnostics.MessageScriptSubjectNoGlobal, t.Identifier)
		}
		return
	}
	// Unexpected mode: TS throws. Goscape silently no-ops since this state is
	// unreachable given the sealed SubjectMode interface.
}

// checkCategoryScriptSubject validates `_FOO`-shaped subjects. Mirrors TS L244-268.
func (sr *ScriptRegistration) checkCategoryScriptSubject(t *trigger.TriggerType, script *ast.Script, subject string) {
	mode := t.SubjectMode
	cat := sr.categoryType
	if cat == nil {
		// TS throws "'category' type not defined." Goscape mirrors as a panic
		// since this is an impossible state when the type registry is correct.
		panic("'category' type not defined.")
	}
	if mode == trigger.ModeNone {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageScriptSubjectOnlyGlobal, t.Identifier)
		return
	}
	if tm, ok := trigger.IsTypeMode(mode); ok {
		if !tm.Category {
			diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
				diagnostics.MessageScriptSubjectNoCategory, t.Identifier)
			return
		}
		sr.resolveSubjectSymbol(script, subject, cat)
		return
	}
	// Unexpected mode: unreachable given sealed SubjectMode interface.
}

// checkTypeScriptSubject validates type-reference subjects (e.g. "obj_bowl").
// Mirrors TS L273-290.
func (sr *ScriptRegistration) checkTypeScriptSubject(t *trigger.TriggerType, script *ast.Script, subject string) {
	mode := t.SubjectMode
	if mode == trigger.ModeNone {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageScriptSubjectOnlyGlobal, t.Identifier)
		return
	}
	if tm, ok := trigger.IsTypeMode(mode); ok {
		sr.resolveSubjectSymbol(script, subject, tm.Type)
		return
	}
	// Unexpected mode: unreachable given sealed SubjectMode interface.
}

// NAI-205-D-MAPZONE-ATOI-STRICT: TS uses parseInt which returns NaN on
// non-numeric input; NaN passes all numeric comparisons (NaN < 0 → false,
// NaN > 255 → false, NaN !== 0 → true), so an input like "abc_x_y" would
// reach the level check and emit MessageMapzoneOnlyLevelZero. Goscape uses
// strconv.Atoi which returns an error on non-numeric input, so we emit
// MessageMapzoneSubjectForm + return -1 at that point instead. Same
// behaviour applies to tryParseZone. The affected input (non-numeric coord
// component) is invalid RuneScript regardless of which diagnostic emits;
// the stricter Go behaviour is functionally equivalent for valid programs.

// tryParseMapZone parses `level_mx_mz`. Returns the packed int32 (which may
// be -1 on parse failure). Reports diagnostics via script.Name. Mirrors TS
// L292-318.
func (sr *ScriptRegistration) tryParseMapZone(script *ast.Script, coord string) int32 {
	parts := strings.Split(coord, "_")
	if len(parts) != 3 {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageMapzoneSubjectForm)
		return -1
	}
	level, errA := strconv.Atoi(parts[0])
	mx, errB := strconv.Atoi(parts[1])
	mz, errC := strconv.Atoi(parts[2])
	if errA != nil || errB != nil || errC != nil {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageMapzoneSubjectForm)
		return -1
	}
	if mx < 0 || mx > 255 || mz < 0 || mz > 255 {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageMapzoneInvalidCoord)
	}
	if level != 0 {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageMapzoneOnlyLevelZero)
		return -1
	}
	x := int32(mx) << 6
	z := int32(mz) << 6
	return (z & 0x3fff) | ((x & 0x3fff) << 14)
}

// tryParseZone parses `level_mx_mz_lx_lz`. Returns packed int32 (may be -1
// on parse failure). Mirrors TS L320-348.
func (sr *ScriptRegistration) tryParseZone(script *ast.Script, coord string) int32 {
	parts := strings.Split(coord, "_")
	if len(parts) != 5 {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageZoneSubjectForm)
		return -1
	}
	level, errA := strconv.Atoi(parts[0])
	mx, errB := strconv.Atoi(parts[1])
	mz, errC := strconv.Atoi(parts[2])
	lx, errD := strconv.Atoi(parts[3])
	lz, errE := strconv.Atoi(parts[4])
	if errA != nil || errB != nil || errC != nil || errD != nil || errE != nil {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageZoneSubjectForm)
		return -1
	}
	if level < 0 || level > 3 || mx < 0 || mx > 255 || mz < 0 || mz > 255 ||
		lx < 0 || lx > 63 || lz < 0 || lz > 63 {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageZoneInvalidCoord)
	}
	if lx%8 != 0 || lz%8 != 0 {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageZoneLocalCoordMultipleOf8)
		return -1
	}
	x := (((int32(mx) << 6) | int32(lx)) >> 3) << 3
	z := (((int32(mz) << 6) | int32(lz)) >> 3) << 3
	return (z & 0x3fff) | ((x & 0x3fff) << 14) | ((int32(level) & 0x3) << 28)
}

// resolveSubjectSymbol finds the symbol-table entry for the subject + type.
// Mirrors TS L353-378.
func (sr *ScriptRegistration) resolveSubjectSymbol(script *ast.Script, subject string, t typ.Type) {
	if t == typ.PrimitiveMapzone {
		packed := sr.tryParseMapZone(script, subject)
		script.SubjectReference = &symbol.BasicSymbol{
			Name: strconv.Itoa(int(packed)),
			Type: t,
		}
		return
	}
	if t == typ.PrimitiveCoord {
		packed := sr.tryParseZone(script, subject)
		script.SubjectReference = &symbol.BasicSymbol{
			Name: strconv.Itoa(int(packed)),
			Type: t,
		}
		return
	}
	found := sr.rootTable.Find(symbol.SymbolTypeBasic(t), subject)
	if found == nil {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageGenericUnresolvedSymbol, subject)
		return
	}
	bs, ok := found.(*symbol.BasicSymbol)
	if !ok {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageGenericUnresolvedSymbol, subject)
		return
	}
	script.SubjectReference = bs
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
