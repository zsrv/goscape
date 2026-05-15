// pkg/pack/compiler/semantics/type_checking_stmt.go
//
// Statement walker arms — NAI-206 T8, T9.
//
// Each arm mirrors TS src/compiler/semantics/TypeChecking.ts at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47.
//
// Arms covered in this file:
//   - visitScriptFile      (L174-178)
//   - visitScript          (L179-188)
//   - visitBlockStatement  (L189-195)
//   - visitReturnStatement (L196-217)
//   - visitIfStatement     (L218-223)
//   - visitWhileStatement  (L224-228)
//   - visitSwitchStatement (L278-313)
//   - visitSwitchCase      (L315-342)
//   - visitEmptyStatement  (L507-509)
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

// checkCondition propagates a Boolean type-hint into the condition expression
// and descends into it. Full condition validation (Logical AND coercion,
// boolean enforcement) lands in T12.
//
// NAI-206-D-CHECK-COND-STUB-T12: condition-arm validators land in T12.
func (tc *TypeChecker) checkCondition(expr ast.Expression) {
	if expr == nil {
		return
	}
	setTypeHint(expr, typ.PrimitiveBoolean)
	tc.Visit(expr)
}
