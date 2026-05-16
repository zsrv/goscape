// pkg/pack/compiler/semantics/type_checking_expr.go
//
// Expression walker arms — NAI-206 T12-T18.
//
// Each arm mirrors TS src/compiler/semantics/TypeChecking.ts at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47.
//
// Arms covered in this file (T12):
//   - visitParenthesizedExpression  (L511-520)
//   - visitConditionExpression      (L522-541)
//   - checkBinaryConditionOperation (L543-640)
//   - isConditionExpression         (L642-648)
//   - findInvalidConditionExpression (L258-275)
//   - getTypeHintRef                (internal helper)
//
// Arms covered in this file (T14):
//   - visitCommandCallExpression (L706-722)
//   - visitProcCallExpression    (L724-733)
//   - visitJumpCallExpression    (L735-754)
//   - checkCallExpression        (L797-815)
//   - typeCheckArguments         (L825-867)
//   - checkDynamicCommand        (stub — NAI-206-D-DYNCOMMAND-STUB-T15)
//   - callName / setCallSymbol / callArgs (helpers)
package semantics

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// visitParenthesizedExpression mirrors TS visitParenthesizedExpression
// (L511-520). Propagates the hint inward and lifts the inner's type back
// onto the paren node.
func (tc *TypeChecker) visitParenthesizedExpression(p *ast.ParenthesizedExpression) {
	if p.Expression == nil {
		return
	}
	setTypeHint(p.Expression, asType(p.TypeHint))
	tc.Visit(p.Expression)
	if t := getType(p.Expression); t != nil {
		p.Type = t
	}
}

// visitConditionExpression mirrors TS visitConditionExpression (L522-541).
// Always sets Type to Boolean on the AST node (TS does the same) but
// sets MetaError if checkBinaryConditionOperation fails.
func (tc *TypeChecker) visitConditionExpression(c *ast.ConditionExpression) {
	if !tc.checkBinaryConditionOperation(c.Left, c.Operator, c.Right) {
		c.Type = typ.MetaError
		return
	}
	c.Type = typ.PrimitiveBoolean
}

// allowedLogicalTypes is the allowed-type set for '&' / '|' operators.
// Mirrors TS TypeChecking.ALLOWED_LOGICAL_TYPES.
var allowedLogicalTypes = []typ.Type{typ.PrimitiveBoolean}

// allowedRelationalTypes returns the allowed-type set for '<', '>', '<=', '>='.
// Mirrors TS TypeChecking.ALLOWED_RELATIONAL_TYPES.
func allowedRelationalTypes() []typ.Type {
	return []typ.Type{typ.PrimitiveInt, typ.PrimitiveLong}
}

// checkBinaryConditionOperation mirrors TS checkBinaryConditionOperation
// (L543-640). Validates the operator + operand types, handling:
//   - feature gates: '&' may be disabled; '<='/'<=' may be disabled
//   - allowed-type restriction: '&'/'|' → boolean; '<','>','<=','>=' → int/long
//   - condition-inside-condition guard: only allowed for '&' / '|'
//   - tuple rejection on either side
//   - unit rejection on either side
//   - assignability between left and right (when no allowed-type set)
//   - string=string equality rejected
func (tc *TypeChecker) checkBinaryConditionOperation(left ast.Expression, op *ast.Token, right ast.Expression) bool {
	opText := op.Text

	// Feature gate: '&' (logical AND) may be disabled.
	if opText == "&" && tc.features.DisableLogicalAnd {
		diagnostics.ReportErrorAt(tc.diagnostics, op, diagnostics.MessageFeatureDisabledOperator, opText)
		return false
	}

	// Feature gate: '<=' / '>=' (relational equals) may be disabled.
	if (opText == "<=" || opText == ">=") && tc.features.DisableRelationalEquals {
		diagnostics.ReportErrorAt(tc.diagnostics, op, diagnostics.MessageFeatureDisabledOperator, opText)
		return false
	}

	// Some operators expect a specific type on both sides.
	var allowed []typ.Type
	switch opText {
	case "&", "|":
		allowed = allowedLogicalTypes
	case "<", ">", "<=", ">=":
		allowed = allowedRelationalTypes()
	}

	// Condition-inside-condition guard: only '&' and '|' may contain conditions
	// on either side. Other operators (=, !, <, >, <=, >=) cannot.
	if opText != "&" && opText != "|" && (tc.isConditionExpression(left) || tc.isConditionExpression(right)) {
		diagnostics.ReportErrorAt(tc.diagnostics, op, diagnostics.MessageConditionNotValid)
		return false
	}

	// Set type hints. If a specific allowed set exists, use the first type as
	// hint for both sides. Otherwise cross-hint from the opposite side's type
	// if the hint isn't already set (mirrors TS left.typeHint ?? right.type ?? null).
	if allowed != nil {
		setTypeHint(left, allowed[0])
		setTypeHint(right, allowed[0])
	} else {
		if asType(getTypeHintRef(left)) == nil {
			setTypeHint(left, getType(right))
		}
		if asType(getTypeHintRef(right)) == nil {
			setTypeHint(right, getType(left))
		}
	}

	// Visit left side first so its type is available to hint the right side.
	tc.visitNodeOrNull(left)

	// Type-hint right from left's evaluated type if right's hint is still nil.
	if getTypeHintRef(right) == nil {
		setTypeHint(right, getType(left))
	}
	tc.visitNodeOrNull(right)

	leftType := getType(left)
	rightType := getType(right)

	// Both types must be resolved; report BINOP_INVALID_TYPES otherwise.
	if leftType == nil || rightType == nil {
		leftRep := "<null>"
		if leftType != nil {
			leftRep = leftType.Representation()
		}
		rightRep := "<null>"
		if rightType != nil {
			rightRep = rightType.Representation()
		}
		diagnostics.ReportErrorAt(tc.diagnostics, op, diagnostics.MessageBinopInvalidTypes, opText, leftRep, rightRep)
		return false
	}

	// Tuple rejection: binary condition operands must be scalar.
	_, leftIsTuple := leftType.(*typ.TupleType)
	_, rightIsTuple := rightType.(*typ.TupleType)
	if leftIsTuple || rightIsTuple {
		if leftIsTuple {
			diagnostics.ReportErrorAt(tc.diagnostics, left, diagnostics.MessageBinopTupleType, "Left", leftType.Representation())
		}
		if rightIsTuple {
			diagnostics.ReportErrorAt(tc.diagnostics, right, diagnostics.MessageBinopTupleType, "Right", rightType.Representation())
		}
		return false
	}

	// Unit rejection: unit values may not appear in conditions.
	if leftType == typ.MetaUnit || rightType == typ.MetaUnit {
		diagnostics.ReportErrorAt(tc.diagnostics, op, diagnostics.MessageBinopInvalidTypes, opText, leftType.Representation(), rightType.Representation())
		return false
	}

	// Operator-specific allowed-type enforcement.
	if allowed != nil {
		if !tc.checkTypeMatchAny(left, allowed, leftType) || !tc.checkTypeMatchAny(right, allowed, rightType) {
			diagnostics.ReportErrorAt(tc.diagnostics, op, diagnostics.MessageBinopInvalidTypes, opText, leftType.Representation(), rightType.Representation())
			return false
		}
	}

	// Assignability check: left.type must be assignable FROM right.type.
	if !tc.checkTypeMatch(left, leftType, rightType, false) {
		diagnostics.ReportErrorAt(tc.diagnostics, op, diagnostics.MessageBinopInvalidTypes, opText, leftType.Representation(), rightType.Representation())
		return false
	}

	// String equality is not allowed (string = string).
	if leftType == typ.PrimitiveString && rightType == typ.PrimitiveString {
		diagnostics.ReportErrorAt(tc.diagnostics, op, diagnostics.MessageBinopInvalidTypes, opText, leftType.Representation(), rightType.Representation())
		return false
	}

	return true
}

// isConditionExpression mirrors TS isConditionExpression (L642-648).
// Returns true for ConditionExpression and ParenthesizedExpression wrapping one.
func (tc *TypeChecker) isConditionExpression(expr ast.Expression) bool {
	if p, ok := expr.(*ast.ParenthesizedExpression); ok {
		return tc.isConditionExpression(p.Expression)
	}
	_, ok := expr.(*ast.ConditionExpression)
	return ok
}

// findInvalidConditionExpression mirrors TS L258-275. Returns the first
// non-binary, non-parenthesized expression descendant; nil means the
// whole tree is valid conditional expressions.
func (tc *TypeChecker) findInvalidConditionExpression(expr ast.Expression) ast.Node {
	if c, ok := expr.(*ast.ConditionExpression); ok {
		op := ""
		if c.Operator != nil {
			op = c.Operator.Text
		}
		// '|' and '&' are logical operators — recurse into both sides.
		if op == "|" || op == "&" {
			if l := tc.findInvalidConditionExpression(c.Left); l != nil {
				return l
			}
			return tc.findInvalidConditionExpression(c.Right)
		}
		// All other operators (=, !, <, >, <=, >=) are valid leaf-level
		// condition expressions.
		return nil
	}
	if p, ok := expr.(*ast.ParenthesizedExpression); ok {
		return tc.findInvalidConditionExpression(p.Expression)
	}
	// Any other expression node is not a condition expression — invalid.
	return expr
}

// allowedArithmeticTypes is the set of types allowed for arithmetic
// operands and calc(...) results. Mirrors TS TypeChecking.ALLOWED_ARITHMETIC_TYPES.
func allowedArithmeticTypes() []typ.Type {
	return []typ.Type{typ.PrimitiveInt, typ.PrimitiveLong}
}

// safeType replaces a nil type with MetaError. Mirrors the lookup-then-
// fallback pattern TS uses inline (e.g. `left.type ?? MetaType.Error`).
func safeType(t typ.Type) typ.Type {
	if t == nil {
		return typ.MetaError
	}
	return t
}

// visitArithmeticExpression mirrors TS visitArithmeticExpression (L650-682)
// at HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47. Defaults the expected
// type to PrimitiveInt when no hint is supplied; type-hints both sides to
// the expected type and walks them; reports BinopInvalidTypes if either
// side fails the allowed-type check or assignability against the expected type.
//
// NAI-206-D-ARITH-RIGHT-NODE: TS L671 passes `left` as the node for both
// the left-type and right-type allowed-type checks (copy-paste in TS). goscape
// passes the correct node (a.Right) for the right-side check to ensure
// diagnostic source locations are accurate.
func (tc *TypeChecker) visitArithmeticExpression(a *ast.ArithmeticExpression) {
	expected := asType(a.TypeHint)
	if expected == nil {
		expected = typ.PrimitiveInt
	}
	setTypeHint(a.Left, expected)
	tc.visitNodeOrNull(a.Left)
	setTypeHint(a.Right, expected)
	tc.visitNodeOrNull(a.Right)

	leftType := getType(a.Left)
	rightType := getType(a.Right)
	allowed := allowedArithmeticTypes()
	if leftType == nil || rightType == nil ||
		!tc.checkTypeMatchAny(a.Left, allowed, safeType(leftType)) ||
		!tc.checkTypeMatchAny(a.Right, allowed, safeType(rightType)) ||
		!tc.checkTypeMatch(a.Left, expected, safeType(leftType), false) ||
		!tc.checkTypeMatch(a.Right, expected, safeType(rightType), false) {
		leftRep := "<null>"
		if leftType != nil {
			leftRep = leftType.Representation()
		}
		rightRep := "<null>"
		if rightType != nil {
			rightRep = rightType.Representation()
		}
		diagnostics.ReportErrorAt(tc.diagnostics, a.Operator, diagnostics.MessageBinopInvalidTypes, a.Operator.Text, leftRep, rightRep)
		a.Type = typ.MetaError
		return
	}
	a.Type = expected
}

// visitCalcExpression mirrors TS visitCalcExpression (L683-704).
// DisableCalc feature-gate first; then defaults hint to PrimitiveInt; walks the
// inner expression and asserts its type is in allowedArithmeticTypes (int|long).
func (tc *TypeChecker) visitCalcExpression(c *ast.CalcExpression) {
	if tc.features.DisableCalc {
		diagnostics.ReportErrorAt(tc.diagnostics, c, diagnostics.MessageFeatureDisabledCalc)
		c.Type = typ.MetaError
		return
	}
	hint := asType(c.TypeHint)
	if hint == nil {
		hint = typ.PrimitiveInt
	}
	setTypeHint(c.Expression, hint)
	tc.visitNodeOrNull(c.Expression)
	inner := getType(c.Expression)
	if inner == nil || !tc.checkTypeMatchAny(c.Expression, allowedArithmeticTypes(), safeType(inner)) {
		rep := "<null>"
		if inner != nil {
			rep = inner.Representation()
		}
		diagnostics.ReportErrorAt(tc.diagnostics, c.Expression, diagnostics.MessageArithmeticInvalidType, rep)
		c.Type = typ.MetaError
		return
	}
	c.Type = inner
}

// getTypeHintRef returns the raw ast.TypeRef from the ExpressionBase.TypeHint
// embedded in each concrete Expression type. Unlike asType(e.TypeHint), this
// returns the interface value itself so callers can distinguish "nil interface"
// from "interface holding a nil type". Used by checkBinaryConditionOperation
// when nil-checking hints.
func getTypeHintRef(e ast.Expression) ast.TypeRef {
	switch v := e.(type) {
	case *ast.ParenthesizedExpression:
		return v.TypeHint
	case *ast.JoinedStringExpression:
		return v.TypeHint
	case *ast.ArithmeticExpression:
		return v.TypeHint
	case *ast.CalcExpression:
		return v.TypeHint
	case *ast.ConditionExpression:
		return v.TypeHint
	case *ast.IntegerLiteral:
		return v.TypeHint
	case *ast.CoordLiteral:
		return v.TypeHint
	case *ast.BooleanLiteral:
		return v.TypeHint
	case *ast.CharacterLiteral:
		return v.TypeHint
	case *ast.StringLiteral:
		return v.TypeHint
	case *ast.NullLiteral:
		return v.TypeHint
	case *ast.LocalVariableExpression:
		return v.TypeHint
	case *ast.GameVariableExpression:
		return v.TypeHint
	case *ast.ConstantVariableExpression:
		return v.TypeHint
	case *ast.CommandCallExpression:
		return v.TypeHint
	case *ast.ProcCallExpression:
		return v.TypeHint
	case *ast.JumpCallExpression:
		return v.TypeHint
	case *ast.ClientScriptExpression:
		return v.TypeHint
	case *ast.Identifier:
		return v.TypeHint
	}
	return nil
}

// ---------------------------------------------------------------------------
// T14 — Call dispatch: visitCommandCallExpression, visitProcCallExpression,
//        visitJumpCallExpression, checkCallExpression, typeCheckArguments,
//        checkDynamicCommand (stub), callName, setCallSymbol, callArgs.
//
// TS reference: TypeChecking.ts HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47
//   visitCommandCallExpression L706-722
//   visitProcCallExpression    L724-733
//   visitJumpCallExpression    L735-754
//   checkCallExpression        L797-815
//   typeCheckArguments         L825-867
// ---------------------------------------------------------------------------

// visitCommandCallExpression mirrors TS visitCommandCallExpression (L706-722).
// Routes through checkDynamicCommand first (dynamic-cmd registry), then falls
// back to the standard server-script lookup.
func (tc *TypeChecker) visitCommandCallExpression(cc *ast.CommandCallExpression) {
	name := cc.NameString()
	if tc.isDisabledCommandName(name) {
		diagnostics.ReportErrorAt(tc.diagnostics, cc, diagnostics.MessageFeatureDisabledCommand, name)
		cc.Type = typ.MetaError
		return
	}
	if tc.checkDynamicCommand(name, cc) {
		return
	}
	tc.checkCallExpression(cc, tc.commandTrigger, diagnostics.MessageCommandReferenceUnresolved)
}

// visitProcCallExpression mirrors TS visitProcCallExpression (L724-733).
func (tc *TypeChecker) visitProcCallExpression(pc *ast.ProcCallExpression) {
	if tc.features.DisableProcs {
		diagnostics.ReportErrorAt(tc.diagnostics, pc, diagnostics.MessageFeatureDisabledTrigger, "proc")
		pc.Type = typ.MetaError
		return
	}
	tc.checkCallExpression(pc, tc.procTrigger, diagnostics.MessageProcReferenceUnresolved)
}

// visitJumpCallExpression mirrors TS visitJumpCallExpression (L735-754).
// Rejects jumps when label trigger is unregistered or when called from
// inside a proc (procs return values; jumps don't).
//
// NAI-206-D-WALKER-OWNS-CONTEXT: TS findParentByType(Script) is replaced by
// tc.currentScript set by visitScript. When currentScript is nil (e.g. bare
// expression tests) the parent-not-found branch emits an internal-error.
func (tc *TypeChecker) visitJumpCallExpression(jc *ast.JumpCallExpression) {
	if tc.labelTrigger == nil {
		diagnostics.ReportErrorAt(tc.diagnostics, jc, "Jump expression not allowed.")
		return
	}
	if tc.currentScript == nil {
		diagnostics.ReportErrorAt(tc.diagnostics, jc, "Internal compiler error: Parent script not found.")
		return
	}
	if scriptTrigger, ok := tc.currentScript.TriggerType.(*trigger.TriggerType); ok && scriptTrigger == tc.procTrigger {
		diagnostics.ReportErrorAt(tc.diagnostics, jc, "Unable to jump to labels from within a proc.")
		return
	}
	tc.checkCallExpression(jc, tc.labelTrigger, diagnostics.MessageJumpReferenceUnresolved)
}

// checkCallExpression mirrors TS checkCallExpression (L797-815). Resolves
// the (trigger, name) lookup in rootTable, populates Call.Symbol + Call.Type,
// then delegates argument type-checking.
//
// NAI-206-D-TRIGGER-LOOKUPS-NILABLE: trigger may be nil when the trigger was
// never registered (test fixtures). Guard before calling SymbolTypeServerScript.
func (tc *TypeChecker) checkCallExpression(call ast.CallExpressionNode, tr *trigger.TriggerType, unresolvedMsg string) {
	name := callName(call)
	var script *symbol.ServerScriptSymbol
	if tr != nil {
		sym := tc.rootTable.Find(symbol.SymbolTypeServerScript(tr), name)
		if sym != nil {
			script, _ = sym.(*symbol.ServerScriptSymbol)
		}
	}
	if script == nil {
		setType(call, typ.MetaError)
		diagnostics.ReportErrorAt(tc.diagnostics, call, unresolvedMsg, name)
	} else {
		setCallSymbol(call, script)
		setType(call, script.Returns)
	}
	tc.typeCheckArguments(script, call, name)
}

// callName returns the identifier text for any call-expression shape.
func callName(c ast.CallExpressionNode) string {
	switch v := c.(type) {
	case *ast.CommandCallExpression:
		return v.Name.Text
	case *ast.ProcCallExpression:
		return v.Name.Text
	case *ast.JumpCallExpression:
		return v.Name.Text
	case *ast.ClientScriptExpression:
		return v.Name.Text
	}
	return ""
}

// setCallSymbol writes s into the Symbol field of the concrete call node.
func setCallSymbol(c ast.CallExpressionNode, s symbol.Symbol) {
	switch v := c.(type) {
	case *ast.CommandCallExpression:
		v.Symbol = s
	case *ast.ProcCallExpression:
		v.Symbol = s
	case *ast.JumpCallExpression:
		v.Symbol = s
	case *ast.ClientScriptExpression:
		v.Symbol = s
	}
}

// callArgs returns the arguments slice for any call-expression shape.
func callArgs(c ast.CallExpressionNode) []ast.Expression {
	switch v := c.(type) {
	case *ast.CommandCallExpression:
		return v.Arguments
	case *ast.ProcCallExpression:
		return v.Arguments
	case *ast.JumpCallExpression:
		return v.Arguments
	case *ast.ClientScriptExpression:
		return v.Arguments
	}
	return nil
}

// typeCheckArguments mirrors TS typeCheckArguments (L825-867). Type-hints then
// walks each argument expression against the script's parameter types; reports
// call-shape-specific NoArgsExpected diagnostics when expected==Unit but actual
// arguments were supplied; otherwise asserts type-match.
func (tc *TypeChecker) typeCheckArguments(script *symbol.ServerScriptSymbol, call ast.CallExpressionNode, name string) {
	var parameterTypes typ.Type
	if script == nil {
		parameterTypes = typ.MetaError
	} else {
		parameterTypes = script.Parameters
	}
	expectedTypes := typ.TupleToList(parameterTypes)
	args := callArgs(call)
	actualTypes := tc.typeHintExpressionList(expectedTypes, args)
	expectedType := typ.TupleFromList(expectedTypes)
	actualType := typ.TupleFromList(actualTypes)

	if expectedType == typ.MetaUnit && actualType != typ.MetaUnit {
		var msg string
		switch call.(type) {
		case *ast.CommandCallExpression:
			msg = diagnostics.MessageCommandNoArgsExpected
		case *ast.ProcCallExpression:
			msg = diagnostics.MessageProcNoArgsExpected
		case *ast.JumpCallExpression:
			msg = diagnostics.MessageJumpNoArgsExpected
		case *ast.ClientScriptExpression:
			msg = diagnostics.MessageClientScriptNoArgsExpected
		default:
			return
		}
		diagnostics.ReportErrorAt(tc.diagnostics, call, msg, name, actualType.Representation())
		return
	}
	tc.checkTypeMatch(call, expectedType, actualType, true)
}

// checkDynamicCommand is a STUB for T14 — T15 lands the full invocation
// path. Returns true if dynamic-disabled blocking fired; otherwise false
// (so the caller falls through to standard command lookup).
//
// NAI-206-D-DYNCOMMAND-STUB-T15: T15 wires the full handler-invocation
// path including Reference fixup + Custom Handler diagnostics.
func (tc *TypeChecker) checkDynamicCommand(name string, expr ast.Expression) bool {
	if tc.isDisabledCommandName(name) {
		diagnostics.ReportErrorAt(tc.diagnostics, expr, diagnostics.MessageFeatureDisabledCommand, name)
		setType(expr, typ.MetaError)
		return true
	}
	// dynamicCommands registry is empty by NAI-206-D-DYNCOMMAND-EMPTY;
	// fall through to standard server-script lookup.
	_, ok := tc.dynamicCommands[name]
	if !ok {
		return false
	}
	// Handler registered but T14 doesn't invoke — return false so caller
	// falls back to standard lookup. T15 replaces this branch.
	return false
}
