// pkg/pack/compiler/semantics/type_checking_stmt.go
//
// Statement walker arms — NAI-206 T8, T9, T10, T11.
//
// Each arm mirrors TS src/compiler/semantics/TypeChecking.ts at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47.
//
// Arms covered in this file:
//   - visitScriptFile              (L174-178)
//   - visitScript                  (L179-188)
//   - visitBlockStatement          (L189-195)
//   - visitReturnStatement         (L196-217)
//   - visitIfStatement             (L218-223)
//   - visitWhileStatement          (L224-228)
//   - visitSwitchStatement         (L278-313)
//   - visitSwitchCase              (L315-342)
//   - visitEmptyStatement          (L507-509)
//   - visitDeclarationStatement    (L380-422)
//   - visitArrayDeclarationStatement (L423-472)
//   - visitAssignmentStatement     (L474-495)
//   - visitExpressionStatement     (L497-505)
//   - expressionHasSideEffects     (L1290-1345)
//   - commandHasSideEffects        (L1347-1361)
package semantics

import (
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// visitScriptFile mirrors TS visitScriptFile (L174-178). Walks each
// top-level Script.
func (tc *TypeChecker) visitScriptFile(sf *ast.ScriptFile) {
	for _, s := range sf.Scripts {
		tc.visitNodeOrNull(s)
	}
}

// visitScript mirrors TS visitScript (L179-188). Sets currentScript and
// atScriptTopLevel for the duration of the body walk; opens the per-script
// local SymbolTable stored on s.Block (NAI-205-D-AST-REF-INTERFACES).
//
// NAI-206-D-WALKER-OWNS-CONTEXT: walker carries currentScript /
// atScriptTopLevel context fields, replacing TS findParentByType.
func (tc *TypeChecker) visitScript(s *ast.Script) {
	oldScript := tc.currentScript
	oldTopLevel := tc.atScriptTopLevel
	tc.currentScript = s
	tc.atScriptTopLevel = true
	defer func() {
		tc.currentScript = oldScript
		tc.atScriptTopLevel = oldTopLevel
	}()

	visit := func() {
		for _, st := range s.Statements {
			tc.visitNodeOrNull(st)
		}
	}

	if s.Block == nil {
		// ScriptRegistration didn't set Block (registration failed). Still
		// walk statements — inner diagnostics are still useful.
		visit()
		return
	}

	// NAI-205-D-AST-REF-INTERFACES: SymbolTableRef's concrete type is
	// *symbol.SymbolTable — assert at consumer site.
	scriptTable, ok := s.Block.(*symbol.SymbolTable)
	if !ok {
		// Defensive — should never happen given NAI-205's contract.
		visit()
		return
	}
	tc.scoped(scriptTable, visit)
}

// visitBlockStatement mirrors TS visitBlockStatement (L189-195). Opens a
// fresh sub-table via CreateSubTable() and toggles atScriptTopLevel false
// within the block, restoring both on exit.
func (tc *TypeChecker) visitBlockStatement(bs *ast.BlockStatement) {
	sub := tc.table.CreateSubTable()
	oldTopLevel := tc.atScriptTopLevel
	tc.atScriptTopLevel = false
	tc.scoped(sub, func() {
		for _, st := range bs.Statements {
			tc.visitNodeOrNull(st)
		}
	})
	tc.atScriptTopLevel = oldTopLevel
}

// visitReturnStatement mirrors TS visitReturnStatement (L196-217). Reports
// MessageReturnOrphan if there is no enclosing script; otherwise verifies
// that the returned expressions match the script's declared return type.
func (tc *TypeChecker) visitReturnStatement(rs *ast.ReturnStatement) {
	if tc.currentScript == nil {
		diagnostics.ReportErrorAt(tc.diagnostics, rs, diagnostics.MessageReturnOrphan)
		return
	}

	var scriptReturnType typ.Type
	if rt, ok := tc.currentScript.ReturnType.(typ.Type); ok {
		scriptReturnType = rt
	} else {
		scriptReturnType = typ.MetaError
	}

	expectedTypes := typ.TupleToList(scriptReturnType)
	actualTypes := tc.typeHintExpressionList(expectedTypes, rs.Expressions)

	expected := typ.TupleFromList(expectedTypes)
	actual := typ.TupleFromList(actualTypes)
	tc.checkTypeMatch(rs, expected, actual, true)
}

// visitIfStatement mirrors TS visitIfStatement (L218-223). Delegates
// condition validation to checkCondition (stub in T8 — full impl in T12).
func (tc *TypeChecker) visitIfStatement(is *ast.IfStatement) {
	tc.checkCondition(is.Condition)
	tc.visitNodeOrNull(is.ThenStatement)
	tc.visitNodeOrNull(is.ElseStatement)
}

// visitWhileStatement mirrors TS visitWhileStatement (L224-228).
func (tc *TypeChecker) visitWhileStatement(ws *ast.WhileStatement) {
	tc.checkCondition(ws.Condition)
	tc.visitNodeOrNull(ws.ThenStatement)
}

// visitEmptyStatement mirrors TS visitEmptyStatement (L507-509). No-op.
func (tc *TypeChecker) visitEmptyStatement(_ *ast.EmptyStatement) {
}

// visitSwitchStatement mirrors TS visitSwitchStatement (L278-313).
// Resolves the switch_T type name (stripping "switch_" prefix), checks
// it's allowed for switching, hints the condition, walks each case
// (recording the first default; reporting Duplicate Default for any
// subsequent default-case).
func (tc *TypeChecker) visitSwitchStatement(sw *ast.SwitchStatement) {
	typeName := strings.TrimPrefix(sw.TypeToken.Text, "switch_")
	t := tc.typeManager.FindOrNil(typeName, false)
	if t == nil {
		diagnostics.ReportErrorAt(tc.diagnostics, sw.TypeToken, diagnostics.MessageGenericInvalidType, typeName)
	} else if !t.Options().AllowSwitch {
		diagnostics.ReportErrorAt(tc.diagnostics, sw.TypeToken, diagnostics.MessageSwitchInvalidType, t.Representation())
	}
	sw.Type = t

	if sw.Condition != nil {
		setTypeHint(sw.Condition, t)
		tc.Visit(sw.Condition)
		if t != nil {
			tc.checkTypeMatch(sw.Condition, t, tc.getSafeType(sw.Condition), true)
		}
	}

	var defaultCase *ast.SwitchCase
	for _, c := range sw.Cases {
		if c.IsDefault() {
			if defaultCase == nil {
				defaultCase = c
			} else {
				diagnostics.ReportErrorAt(tc.diagnostics, c, diagnostics.MessageSwitchDuplicateDefault)
			}
		}
		old := tc.currentSwitch
		tc.currentSwitch = sw
		tc.visitNodeOrNull(c)
		tc.currentSwitch = old
	}
	sw.DefaultCase = defaultCase
}

// visitSwitchCase mirrors TS visitSwitchCase (L315-342). Validates each
// case-key is a constant expression matching the switch type; walks the
// case body inside a fresh sub-table.
func (tc *TypeChecker) visitSwitchCase(sc *ast.SwitchCase) {
	if tc.currentSwitch == nil {
		diagnostics.ReportErrorAt(tc.diagnostics, sc, diagnostics.MessageCaseWithoutSwitch)
		return
	}
	var switchType typ.Type
	if t, ok := tc.currentSwitch.Type.(typ.Type); ok {
		switchType = t
	}
	for _, key := range sc.Keys {
		setTypeHint(key, switchType)
		tc.visitNodeOrNull(key)
		if !tc.isConstantExpression(key) {
			diagnostics.ReportErrorAt(tc.diagnostics, key, diagnostics.MessageSwitchCaseNotConstant)
			continue
		}
		if switchType != nil {
			tc.checkTypeMatch(key, switchType, tc.getSafeType(key), true)
		}
	}
	sub := tc.table.CreateSubTable()
	tc.scoped(sub, func() {
		for _, st := range sc.Statements {
			tc.visitNodeOrNull(st)
		}
	})
}

// checkCondition mirrors TS code path called by visitIf/visitWhile when
// validating a condition. Type-hints to Boolean, runs the validator,
// then either emits InvalidNodeType OR walks + asserts Boolean match.
//
// Retires NAI-206-D-CHECK-COND-STUB-T12 — the T12 implementation is here.
func (tc *TypeChecker) checkCondition(expr ast.Expression) {
	if expr == nil {
		return
	}
	setTypeHint(expr, typ.PrimitiveBoolean)
	invalid := tc.findInvalidConditionExpression(expr)
	if invalid != nil {
		diagnostics.ReportErrorAt(tc.diagnostics, invalid, diagnostics.MessageConditionInvalidNodeType)
		return
	}
	tc.visitNodeOrNull(expr)
	t := getType(expr)
	if t == nil {
		t = typ.MetaError
	}
	tc.checkTypeMatch(expr, typ.PrimitiveBoolean, t, true)
}

// visitDeclarationStatement mirrors TS visitDeclarationStatement (L380-422)
// at HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47. Reports
// FeatureDisabledLocal when locals/procs are disabled (TS ties these
// together since proc declarations are the only context for locals).
// Reports NotTopLevel when TopLevelDefOnly is set and we're not at script
// top level (tc.atScriptTopLevel — NAI-206-D-WALKER-OWNS-CONTEXT).
func (tc *TypeChecker) visitDeclarationStatement(d *ast.DeclarationStatement) {
	if tc.features.DisableProcs {
		diagnostics.ReportErrorAt(tc.diagnostics, d, diagnostics.MessageFeatureDisabledLocal)
		return
	}
	if tc.features.TopLevelDefOnly && !tc.atScriptTopLevel {
		diagnostics.ReportErrorAt(tc.diagnostics, d, diagnostics.MessageLocalDeclarationNotTopLevel)
		return
	}
	typeName := strings.TrimPrefix(d.TypeToken.Text, "def_")
	name := d.Name.Text
	t := tc.typeManager.FindOrNil(typeName, false)
	switch {
	case tc.isDisabledTypeName(typeName):
		diagnostics.ReportErrorAt(tc.diagnostics, d.TypeToken, diagnostics.MessageFeatureDisabledType, typeName)
		t = typ.MetaError
	case t == nil:
		diagnostics.ReportErrorAt(tc.diagnostics, d.TypeToken, diagnostics.MessageGenericInvalidType, typeName)
		t = typ.MetaError
	case t != typ.MetaError && !t.Options().AllowDeclaration:
		diagnostics.ReportErrorAt(tc.diagnostics, d.TypeToken, diagnostics.MessageLocalDeclarationInvalidType, t.Representation())
	}
	sym := &symbol.LocalVariableSymbol{Name: name, Type: t}
	if !tc.table.Insert(symbol.SymbolTypeLocalVariable(), sym) {
		diagnostics.ReportErrorAt(tc.diagnostics, d.Name, diagnostics.MessageScriptLocalRedeclaration, name)
	}
	if d.Initializer != nil {
		setTypeHint(d.Initializer, sym.Type)
		tc.visitNodeOrNull(d.Initializer)
		tc.checkTypeMatch(d.Initializer, sym.Type, tc.getSafeType(d.Initializer), true)
	}
	d.Symbol = sym
}

// visitArrayDeclarationStatement mirrors TS visitArrayDeclarationStatement
// (L423-472). Wraps the base type in ArrayType, hints Initializer (size)
// to PrimitiveInt, inserts the local symbol.
func (tc *TypeChecker) visitArrayDeclarationStatement(d *ast.ArrayDeclarationStatement) {
	if tc.features.DisableProcs {
		diagnostics.ReportErrorAt(tc.diagnostics, d, diagnostics.MessageFeatureDisabledLocal)
		return
	}
	if tc.features.TopLevelDefOnly && !tc.atScriptTopLevel {
		diagnostics.ReportErrorAt(tc.diagnostics, d, diagnostics.MessageLocalDeclarationNotTopLevel)
		return
	}
	typeName := strings.TrimPrefix(d.TypeToken.Text, "def_")
	name := d.Name.Text
	t := tc.typeManager.FindOrNil(typeName, false)
	switch {
	case tc.isDisabledTypeName(typeName):
		diagnostics.ReportErrorAt(tc.diagnostics, d.TypeToken, diagnostics.MessageFeatureDisabledType, typeName)
		t = typ.MetaError
	case t == nil:
		diagnostics.ReportErrorAt(tc.diagnostics, d.TypeToken, diagnostics.MessageGenericInvalidType, typeName)
		t = typ.MetaError
	case t != typ.MetaError && !t.Options().AllowDeclaration:
		diagnostics.ReportErrorAt(tc.diagnostics, d.TypeToken, diagnostics.MessageLocalDeclarationInvalidType, t.Representation())
	case t != typ.MetaError && !t.Options().AllowArray:
		diagnostics.ReportErrorAt(tc.diagnostics, d.TypeToken, diagnostics.MessageLocalArrayInvalidType, t.Representation())
	}
	var wrapped typ.Type
	if t == typ.MetaError {
		wrapped = typ.MetaError
	} else {
		arr, err := typ.NewArrayType(t)
		if err != nil {
			diagnostics.ReportErrorAt(tc.diagnostics, d.TypeToken, diagnostics.MessageLocalArrayInvalidType, t.Representation())
			wrapped = typ.MetaError
		} else {
			wrapped = arr
		}
	}
	if d.Initializer != nil {
		setTypeHint(d.Initializer, typ.PrimitiveInt)
		tc.visitNodeOrNull(d.Initializer)
		tc.checkTypeMatch(d.Initializer, typ.PrimitiveInt, tc.getSafeType(d.Initializer), true)
	}
	sym := &symbol.LocalVariableSymbol{Name: name, Type: wrapped}
	if !tc.table.Insert(symbol.SymbolTypeLocalVariable(), sym) {
		diagnostics.ReportErrorAt(tc.diagnostics, d.Name, diagnostics.MessageScriptLocalRedeclaration, name)
	}
	d.Symbol = sym
}

// visitAssignmentStatement mirrors TS visitAssignmentStatement (L474-495)
// at HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47. Walks each LHS var,
// collects their types as the type-hint vector for the RHS expression
// list, type-checks the tuple-match, and reports MessageAssignMultiArray
// when an array element appears among multiple LHS vars.
func (tc *TypeChecker) visitAssignmentStatement(a *ast.AssignmentStatement) {
	leftTypes := make([]typ.Type, 0, len(a.Vars))
	for _, v := range a.Vars {
		tc.visitNodeOrNull(v)
		leftTypes = append(leftTypes, tc.getSafeType(v))
	}
	rightTypes := tc.typeHintExpressionList(leftTypes, a.Expressions)
	leftType := typ.TupleFromList(leftTypes)
	rightType := typ.TupleFromList(rightTypes)
	tc.checkTypeMatch(a, leftType, rightType, true)

	if len(a.Vars) > 1 {
		for _, v := range a.Vars {
			if lv, ok := v.(*ast.LocalVariableExpression); ok && lv.IsArray() {
				diagnostics.ReportErrorAt(tc.diagnostics, lv, diagnostics.MessageAssignMultiArray)
				break
			}
		}
	}
}

// visitExpressionStatement mirrors TS visitExpressionStatement (L497-505).
// Visits the inner expression; warns when the expression has a non-error
// type but no side effects.
func (tc *TypeChecker) visitExpressionStatement(es *ast.ExpressionStatement) {
	tc.visitNodeOrNull(es.Expression)
	t := getType(es.Expression)
	if t == nil || t == typ.MetaError {
		return
	}
	if !tc.expressionHasSideEffects(es.Expression) {
		diagnostics.ReportAt(tc.diagnostics, es, diagnostics.DiagnosticWarning, diagnostics.MessageExpressionStatementNoSideEffect)
	}
}

// expressionHasSideEffects mirrors TS expressionHasSideEffects (L1290-1345)
// at HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47. Command and
// identifier-resolving-to-command have side effects iff their return type
// is unit-like (commandHasSideEffects). Proc/jump/clientscript always have
// side effects. Compound expressions recurse.
func (tc *TypeChecker) expressionHasSideEffects(expr ast.Expression) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case *ast.CommandCallExpression:
		return tc.commandHasSideEffects(getType(e))
	case *ast.ProcCallExpression:
		return true
	case *ast.JumpCallExpression:
		return true
	case *ast.ClientScriptExpression:
		return true
	case *ast.Identifier:
		ref, ok := e.Reference.(*symbol.ServerScriptSymbol)
		if !ok || ref == nil {
			return false
		}
		if tc.commandTrigger == nil || ref.Trigger != tc.commandTrigger {
			return false
		}
		t := getType(e)
		if t == nil {
			t = ref.Returns
		}
		return tc.commandHasSideEffects(t)
	case *ast.LocalVariableExpression:
		return tc.expressionHasSideEffects(e.Index)
	case *ast.ConstantVariableExpression:
		return tc.expressionHasSideEffects(e.SubExpression)
	case *ast.ParenthesizedExpression:
		return tc.expressionHasSideEffects(e.Expression)
	case *ast.CalcExpression:
		return tc.expressionHasSideEffects(e.Expression)
	case *ast.ArithmeticExpression:
		return tc.expressionHasSideEffects(e.Left) || tc.expressionHasSideEffects(e.Right)
	case *ast.ConditionExpression:
		return tc.expressionHasSideEffects(e.Left) || tc.expressionHasSideEffects(e.Right)
	case *ast.JoinedStringExpression:
		for _, part := range e.Parts {
			if esp, ok := part.(*ast.ExpressionStringPart); ok {
				if tc.expressionHasSideEffects(esp.Expression) {
					return true
				}
			}
		}
		return false
	}
	return false
}

// commandHasSideEffects mirrors TS commandHasSideEffects (L1347-1361)
// at HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47. A command "has side
// effects" when its return type is unit-like (MetaUnit, MetaError, nil,
// or a tuple whose every child is MetaUnit). Non-unit returns mean the
// value is meaningful and absence of a consumer (statement-position) is
// suspect.
func (tc *TypeChecker) commandHasSideEffects(t typ.Type) bool {
	if t == nil || t == typ.MetaError {
		return true
	}
	if t == typ.MetaUnit {
		return true
	}
	if tup, ok := t.(*typ.TupleType); ok {
		for _, ch := range tup.Children {
			if ch != typ.MetaUnit {
				return false
			}
		}
		return true
	}
	return false
}
