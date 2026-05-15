// pkg/pack/compiler/semantics/type_checking.go
package semantics

// TypeChecking walker — port of TS TypeChecking.ts.
//
// This file contains:
//   - TypeChecker struct + NewTypeChecker constructor.
//   - scoped() table swapping.
//   - isDisabledTypeName / isDisabledCommandName feature gates.
//   - Visit() top-level dispatch (empty type-switch fallback; T8-T18 fill arms).
//   - visitNodeFallback / visitNodeOrNull / visitNodes.
//   - getSafeType / checkTypeMatch / checkTypeMatchAny.
//   - flattenTuple (thin wrapper over typ.TupleToList).
//   - typeHintExpressionList.
//   - isConstantExpression stub (T9 wires the real impl).
//   - lowerASCII helper.
//
// NAI-206-D-WALKER-OWNS-CONTEXT: walker carries currentScript /
// currentSwitch / atScriptTopLevel context fields, replacing TS
// findParentByType. Arms in T8-T18 set/read these fields.
//
// NAI-206-D-CONST-CACHE-AST: constantExpressionCache maps string keys to
// ast.Expression nodes (TS caches ParserRuleContext because AstBuilder runs
// per-read; goscape parses straight to AST so we cache AST nodes).
//
// NAI-206-D-TRIGGER-LOOKUPS-NILABLE: plan codified Find() panic-on-miss for
// command/proc triggers; goscape uses FindOrNil throughout so test fixtures
// that don't register these triggers don't panic. T14 (call infra) must guard
// against nil before consulting commandTrigger/procTrigger.
//
// NAI-206-D-CONSTEXPR-STUB-T9: isConstantExpression returns false here
// (conservative — treats every expression as non-constant). T9 wires the full
// constant-detection logic.

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// Package-level disabled command sets. Mirrors TS static class fields.
//
// NAI-206-D-DYNCOMMAND-EMPTY: no concrete dynamic handlers are registered
// here — the cohort wires specific handlers in a follow-up NAI.
var (
	// disabledEnumCommands is the set of command names blocked when
	// StrictFeatureLevel.DisableEnums is true. Mirrors TS DISABLED_ENUM_COMMANDS.
	disabledEnumCommands = map[string]bool{
		"enum": true,
	}

	// disabledStructCommands is the set of command names blocked when
	// StrictFeatureLevel.DisableStructs is true. Mirrors TS DISABLED_STRUCT_COMMANDS.
	disabledStructCommands = map[string]bool{
		"struct_param": true,
	}

	// disabledDBCommands is the set of command names blocked when
	// StrictFeatureLevel.DisableDBTables is true. Mirrors TS DISABLED_DB_COMMANDS.
	disabledDBCommands = map[string]bool{
		"db_find":                   true,
		"db_find_refine":            true,
		"db_find_with_count":        true,
		"db_find_refine_with_count": true,
		"db_getfield":               true,
	}
)

// TypeChecker performs type-checking over a parsed AST. It is the Go port of
// TS TypeChecking extends AstVisitor<void>. Consumers instantiate via
// NewTypeChecker and call Visit(node) for each top-level ScriptFile.
//
// Field visibility: all fields are unexported — arms in T8-T18 live in the
// same package and access them directly (same-package field access, matching
// TS private access within the class).
type TypeChecker struct {
	// Core infrastructure (set at construction, never mutated).
	typeManager     *typ.TypeManager
	triggerManager  *trigger.TriggerManager
	rootTable       *symbol.SymbolTable
	dynamicCommands map[string]DynamicCommandHandler
	diagnostics     *diagnostics.Diagnostics
	features        StrictFeatureLevel

	// Cached trigger lookups (FindOrNil — nil when trigger not registered).
	commandTrigger      *trigger.TriggerType // TS commandTrigger (required in production, optional in tests)
	procTrigger         *trigger.TriggerType // TS procTrigger (required in production, optional in tests)
	clientscriptTrigger *trigger.TriggerType // TS clientscriptTrigger (optional)
	labelTrigger        *trigger.TriggerType // TS labelTrigger (optional)

	// Active symbol table — swapped by scoped() as the walker descends.
	table *symbol.SymbolTable

	// Per-script / per-switch context set by T8-T18 arms (NAI-206-D-WALKER-OWNS-CONTEXT).
	currentScript    *ast.Script
	currentSwitch    *ast.SwitchStatement
	atScriptTopLevel bool

	// Constant-expression evaluation guards (NAI-206-D-CONST-CACHE-AST).
	constantsBeingEvaluated map[ast.Expression]bool
	constantExpressionCache map[string]ast.Expression
}

// NewTypeChecker constructs a TypeChecker. All four trigger names are looked
// up via FindOrNil so fixtures that don't register "command"/"proc" do not
// panic (NAI-206-D-TRIGGER-LOOKUPS-NILABLE).
func NewTypeChecker(
	tm *typ.TypeManager,
	trm *trigger.TriggerManager,
	root *symbol.SymbolTable,
	dynamicCommands map[string]DynamicCommandHandler,
	d *diagnostics.Diagnostics,
	features StrictFeatureLevel,
) *TypeChecker {
	if dynamicCommands == nil {
		dynamicCommands = map[string]DynamicCommandHandler{}
	}
	tc := &TypeChecker{
		typeManager:             tm,
		triggerManager:          trm,
		rootTable:               root,
		dynamicCommands:         dynamicCommands,
		diagnostics:             d,
		features:                features,
		commandTrigger:          trm.FindOrNil("command"),
		procTrigger:             trm.FindOrNil("proc"),
		clientscriptTrigger:     trm.FindOrNil("clientscript"),
		labelTrigger:            trm.FindOrNil("label"),
		table:                   root,
		constantsBeingEvaluated: map[ast.Expression]bool{},
		constantExpressionCache: map[string]ast.Expression{},
	}
	return tc
}

// scoped temporarily sets tc.table to newTable, runs block, then restores the
// previous table. Mirrors TS TypeChecking.scoped(). Concurrent callers are not
// supported — TypeChecker is single-threaded (one goroutine per compilation).
func (tc *TypeChecker) scoped(newTable *symbol.SymbolTable, block func()) {
	old := tc.table
	tc.table = newTable
	block()
	tc.table = old
}

// isDisabledTypeName returns true when typeText names a type that is disabled
// by the current StrictFeatureLevel. Array suffixes are stripped before the
// check. Mirrors TS TypeChecking.isDisabledTypeName().
func (tc *TypeChecker) isDisabledTypeName(typeText string) bool {
	text := lowerASCII(typeText)
	// Strip "array" suffix to get the base type name.
	base := text
	if len(base) > 5 && base[len(base)-5:] == "array" {
		base = base[:len(base)-5]
	}
	if tc.features.DisableBooleans && base == typ.PrimitiveBoolean.Representation() {
		return true
	}
	if tc.features.DisableEnums && base == "enum" {
		return true
	}
	if tc.features.DisableStructs && base == "struct" {
		return true
	}
	if tc.features.DisableDBTables && (base == "dbtable" || base == "dbrow" || base == "dbcolumn") {
		return true
	}
	return false
}

// isDisabledCommandName returns true when name is a command blocked by the
// current StrictFeatureLevel. Mirrors TS TypeChecking.isDisabledCommandName().
func (tc *TypeChecker) isDisabledCommandName(name string) bool {
	if tc.features.DisableEnums && disabledEnumCommands[name] {
		return true
	}
	if tc.features.DisableStructs && disabledStructCommands[name] {
		return true
	}
	if tc.features.DisableDBTables && disabledDBCommands[name] {
		return true
	}
	return false
}

// Visit is the top-level dispatch entry point. Arms for each concrete node type
// are added in T8-T18. For now only the default fallback is present. Mirrors
// TS AstVisitor dispatch (visitXxx methods), here unified into a single
// type-switch per NAI-204-D-AST-NO-VISITOR.
func (tc *TypeChecker) Visit(n ast.Node) {
	if n == nil {
		return
	}
	switch n.(type) {
	// T8-T18 will insert cases here.
	default:
		tc.visitNodeFallback(n)
	}
}

// visitNodeFallback is called for node types not yet handled. Emits an INFO
// diagnostic so unhandled nodes are visible in the diagnostic stream during
// development. Mirrors TS AstVisitor.visitDefault() behaviour for undefined
// visit methods (which would be no-ops in TS but we surface them explicitly).
func (tc *TypeChecker) visitNodeFallback(n ast.Node) {
	diagnostics.ReportAt(tc.diagnostics, n, diagnostics.DiagnosticInfo, "Unhandled node: %s.", n.Kind().String())
}

// visitNodeOrNull calls Visit(n) if n is non-nil. Mirrors TS visitNodeOrNull.
func (tc *TypeChecker) visitNodeOrNull(n ast.Node) {
	if n == nil {
		return
	}
	tc.Visit(n)
}

// visitNodes calls visitNodeOrNull for every element of nodes. Mirrors TS
// private visitNodes(nodes: readonly Node[] | null | undefined).
func (tc *TypeChecker) visitNodes(nodes []ast.Node) {
	for _, n := range nodes {
		tc.visitNodeOrNull(n)
	}
}

// getSafeType returns e.Type if e is non-nil and has a resolved type, or
// MetaError otherwise. Mirrors TS getSafeType(expr).
// Uses getType() from dynamic_command.go to read ExpressionBase.Type through
// the type-switch over all 19 concrete Expression types.
func (tc *TypeChecker) getSafeType(e ast.Expression) typ.Type {
	if e == nil {
		return typ.MetaError
	}
	if t := getType(e); t != nil {
		return t
	}
	return typ.MetaError
}

// checkTypeMatch verifies that actual matches expected. If the types are
// TupleTypes, they are flattened and compared element-by-element. Returns true
// on match. If reportErrors is true and there is a mismatch, emits an error
// diagnostic. Mirrors TS TypeChecking.checkTypeMatch().
func (tc *TypeChecker) checkTypeMatch(n ast.Node, expected, actual typ.Type, reportErrors bool) bool {
	expectedFlat := flattenTuple(expected)
	actualFlat := flattenTuple(actual)

	var match bool
	switch {
	case expected == typ.MetaError:
		// If expected is error, allow anything to prevent error propagation.
		match = true
	case len(expectedFlat) != len(actualFlat):
		match = false
	default:
		match = true
		for i := range expectedFlat {
			match = match && tc.typeManager.Check(expectedFlat[i], actualFlat[i])
		}
	}

	if !match && reportErrors {
		actualRep := actual.Representation()
		if actual == typ.MetaUnit {
			actualRep = "<unit>"
		}
		diagnostics.ReportErrorAt(tc.diagnostics, n, diagnostics.MessageGenericTypeMismatch, actualRep, expected.Representation())
	}
	return match
}

// checkTypeMatchAny returns true if actual matches any of the expected types.
// Never emits diagnostics (mismatches are silent — caller is responsible).
// Mirrors TS TypeChecking.checkTypeMatchAny().
func (tc *TypeChecker) checkTypeMatchAny(n ast.Node, expected []typ.Type, actual typ.Type) bool {
	for _, e := range expected {
		if tc.checkTypeMatch(n, e, actual, false) {
			return true
		}
	}
	return false
}

// flattenTuple returns the children of a TupleType, or a single-element slice
// for any other type. Delegates to typ.TupleToList which handles MetaUnit and
// MetaNothing as empty lists. Mirrors TS TupleType-flattenning inside
// checkTypeMatch.
func flattenTuple(t typ.Type) []typ.Type {
	return typ.TupleToList(t)
}

// typeHintExpressionList visits each expression with a type hint drawn from
// expectedTypes (indexed by position, accounting for tuple widths) and returns
// the evaluated actual types. Mirrors TS TypeChecking.typeHintExpressionList().
func (tc *TypeChecker) typeHintExpressionList(expectedTypes []typ.Type, expressions []ast.Expression) []typ.Type {
	actualTypes := make([]typ.Type, 0, len(expressions))
	typeCounter := 0

	for _, expr := range expressions {
		// Set the type hint if we haven't exhausted the expected types.
		if typeCounter < len(expectedTypes) {
			setTypeHint(expr, expectedTypes[typeCounter])
		} else {
			setTypeHint(expr, nil)
		}

		// Visit the expression (evaluates its type).
		tc.Visit(expr)

		// Add the evaluated type.
		actualTypes = append(actualTypes, tc.getSafeType(expr))

		// Increment counter: tuples advance by their child count, scalars by 1.
		if t := getType(expr); t != nil {
			if tup, ok := t.(*typ.TupleType); ok {
				typeCounter += len(tup.Children)
				continue
			}
		}
		typeCounter++
	}
	return actualTypes
}

// isConstantExpression returns whether e is a constant expression.
// Stub for NAI-206 T7 — T9 wires the full impl. Default conservative
// answer is false (treats every expression as non-constant).
//
// NAI-206-D-CONSTEXPR-STUB-T9: Constant-expression analysis lives in T9.
// Returning false here is conservative (treats every expression as
// non-constant).
func (tc *TypeChecker) isConstantExpression(e ast.Expression) bool {
	return false
}

// lowerASCII lowercases ASCII letters in s. Mirrors TS String.toLowerCase()
// for type-name comparisons (all RuneScript type names are ASCII).
// This is a package-local copy — the same function exists in
// pkg/pack/compiler/type/primitive.go but lives in a different package.
func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
