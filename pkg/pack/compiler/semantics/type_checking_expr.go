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
//
// Arms covered in this file (T16):
//   - visitLocalVariableExpression   (L909-949)
//   - visitGameVariableExpression    (L950-985)
//   - visitConstantVariableExpression (L987-1082)
//   - parseConstantExpression        (L1053-1081)
//   - isGameVarType / gameVarInner   (helpers)
//
// Arms covered in this file (T18):
//   - visitIdentifier        (L1214-1238)
//   - resolveSymbol          (L1240-1287) — full impl replaces T17 stub
//   - symbolToType           (L1357-1391)
//   - allowStringConversion  (L1353-1356)
//   - symbolKindName         (internal helper)
package semantics

import (
	"fmt"
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/parser"
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
//
// TS builds expectedTypes as `parameters instanceof TupleType ? children : [parameters]`
// — a non-tuple `parameters` (including MetaUnit) becomes a SINGLE-ELEMENT list,
// not empty. This shape matters for `~no_arg_proc(null)`: the null arg gets
// typeHint=MetaUnit, visitNullLiteral sets its type to MetaUnit, the actual
// tuple collapses to MetaUnit, and the check passes. Mirror that exactly —
// TupleToList(MetaUnit) returns nil, which would mis-hint the arg as int.
// Real content (gnomeball_pass.rs2) relies on this TS-quirk.
func (tc *TypeChecker) typeCheckArguments(script *symbol.ServerScriptSymbol, call ast.CallExpressionNode, name string) {
	var parameterTypes typ.Type
	if script == nil {
		parameterTypes = typ.MetaError
	} else {
		parameterTypes = script.Parameters
	}
	var expectedTypes []typ.Type
	if tup, ok := parameterTypes.(*typ.TupleType); ok {
		expectedTypes = tup.Children
	} else {
		expectedTypes = []typ.Type{parameterTypes}
	}
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

// checkDynamicCommand mirrors TS checkDynamicCommand (L756-797) at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47. Looks up the handler in the
// dynamic-command registry, invokes its TypeCheck, then validates the
// handler populated Type + Symbol (or Reference for Identifier) on the
// expression. Returns true if either: the command name is feature-
// disabled OR a registered handler ran.
//
// Retires NAI-206-D-DYNCOMMAND-STUB-T15 — full impl is here.
func (tc *TypeChecker) checkDynamicCommand(name string, expr ast.Expression) bool {
	if tc.isDisabledCommandName(name) {
		diagnostics.ReportErrorAt(tc.diagnostics, expr, diagnostics.MessageFeatureDisabledCommand, name)
		setType(expr, typ.MetaError)
		return true
	}
	h, ok := tc.dynamicCommands[name]
	if !ok {
		return false
	}
	ctx := newTypeCheckingContext(tc, tc.typeManager, expr, tc.diagnostics)
	h.TypeCheck(ctx)
	if getType(expr) == nil {
		diagnostics.ReportErrorAt(tc.diagnostics, expr, diagnostics.MessageCustomHandlerNoType)
	}
	// Symbol/Reference fixup. TS checks: Identifier.reference || CallExpression.symbol.
	// Goscape covers all concrete call shapes + Identifier.
	needsSymbol := false
	switch e := expr.(type) {
	case *ast.Identifier:
		if e.Reference == nil {
			needsSymbol = true
		}
	case *ast.CommandCallExpression:
		if e.Symbol == nil {
			needsSymbol = true
		}
	case *ast.ProcCallExpression:
		if e.Symbol == nil {
			needsSymbol = true
		}
	case *ast.JumpCallExpression:
		if e.Symbol == nil {
			needsSymbol = true
		}
	case *ast.ClientScriptExpression:
		if e.Symbol == nil {
			needsSymbol = true
		}
	}
	if needsSymbol {
		var s symbol.Symbol
		if tc.commandTrigger != nil {
			s = tc.rootTable.Find(symbol.SymbolTypeServerScript(tc.commandTrigger), name)
		}
		if s == nil {
			diagnostics.ReportErrorAt(tc.diagnostics, expr, diagnostics.MessageCustomHandlerNoSymbol)
		}
		switch e := expr.(type) {
		case *ast.Identifier:
			e.Reference = s
		case *ast.CommandCallExpression:
			e.Symbol = s
		case *ast.ProcCallExpression:
			e.Symbol = s
		case *ast.JumpCallExpression:
			e.Symbol = s
		case *ast.ClientScriptExpression:
			e.Symbol = s
		}
	}
	return true
}

// visitClientScriptExpression mirrors TS visitClientScriptExpression
// (L817-869) at HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47.
//
// Requires the clientscript trigger to be registered. Requires the
// TypeHint to be a MetaType.Hook (a clientscript reference is always
// inside a hook hint, set by the dynamic-command surface).
//
// NAI-206-D-CLIENTSCRIPT-NO-PANIC: TS throws "Expected MetaType Hook"
// when the hint is wrong; goscape emits an internal-compiler diagnostic
// and bails. End state for downstream consumers is identical.
func (tc *TypeChecker) visitClientScriptExpression(cse *ast.ClientScriptExpression) {
	if tc.clientscriptTrigger == nil {
		diagnostics.ReportErrorAt(tc.diagnostics, cse, diagnostics.MessageTriggerTypeNotFound, "clientscript")
		setType(cse, typ.MetaError)
		return
	}
	hint := asType(cse.TypeHint)
	transmitListType, isHook := typ.IsMetaHook(hint)
	if !isHook {
		diagnostics.ReportErrorAt(tc.diagnostics, cse, "Internal compiler error: Expected MetaType.Hook hint on ClientScriptExpression.")
		setType(cse, typ.MetaError)
		return
	}
	name := cse.Name.Text
	sym := tc.rootTable.Find(symbol.SymbolTypeClientScript(tc.clientscriptTrigger), name)
	clientSym, _ := sym.(*symbol.ClientScriptSymbol)
	if clientSym == nil {
		diagnostics.ReportErrorAt(tc.diagnostics, cse, diagnostics.MessageClientScriptReferenceUnresolved, name)
		cse.Type = typ.MetaError
	} else {
		cse.Symbol = clientSym
		cse.Type = hint
	}
	// Arg type-check via the shared helper. typeCheckArguments takes a
	// *ServerScriptSymbol; ClientScriptSymbol shares ScriptSymbolFields,
	// so we adapt by constructing a temporary ServerScriptSymbol view.
	var asServer *symbol.ServerScriptSymbol
	if clientSym != nil {
		asServer = &symbol.ServerScriptSymbol{ScriptSymbolFields: clientSym.ScriptSymbolFields}
	}
	tc.typeCheckArguments(asServer, cse, name)

	if transmitListType == typ.MetaUnit && len(cse.TransmitList) > 0 {
		diagnostics.ReportErrorAt(tc.diagnostics, cse.TransmitList[0], diagnostics.MessageHookTransmitListUnexpected)
		cse.Type = typ.MetaError
		return
	}
	for _, expr := range cse.TransmitList {
		setTypeHint(expr, transmitListType)
		tc.visitNodeOrNull(expr)
		if transmitListType != nil {
			tc.checkTypeMatch(expr, transmitListType, tc.getSafeType(expr), true)
		}
	}
}

// handleClientScriptExpression mirrors TS L1156-1184. Called from T17's
// visitStringLiteral when the hint is a Hook — re-parses the literal as
// a clientscript expression, hints + visits it, and copies the type back
// to the host StringLiteral.
func (tc *TypeChecker) handleClientScriptExpression(sl *ast.StringLiteral, hint typ.Type) {
	src := sl.Source()
	p := parser.NewClientScriptParser(sl.Value, src.Name)
	// NAI-206-D-CONST-PARSE: re-parse silences default listeners; the
	// adapter below routes syntax errors back to the diagnostics sink
	// using the HOST literal's source location (cheaper than re-mapping
	// inner re-parse line/col onto host).
	p.RemoveErrorListeners()
	p.AddErrorListener(&clientScriptReparseListener{d: tc.diagnostics, hostLoc: src})
	cse := p.ParseClientScript()
	if cse == nil {
		sl.Type = typ.MetaError
		return
	}
	cse.TypeHint = hint
	tc.Visit(cse)
	sl.SubExpression = cse
	sl.Type = getType(cse)
}

// clientScriptReparseListener adapts parser syntax errors back to
// diagnostics at the host literal's source location. NAI-206-D-CONST-PARSE-LOC:
// TS re-maps inner re-parse line/col onto host literal via offset arithmetic;
// goscape attaches at the host literal location (simpler, less precise).
type clientScriptReparseListener struct {
	d       *diagnostics.Diagnostics
	hostLoc lexer.NodeSourceLocation
}

func (c *clientScriptReparseListener) SyntaxError(sourceName string, line, column int, msg string) {
	c.d.Report(diagnostics.NewDiagnostic(c.hostLoc, diagnostics.DiagnosticError, "%s", msg))
}

// ---------------------------------------------------------------------------
// T16 — Variable expressions: Local, Game, Constant.
//
// TS reference: TypeChecking.ts HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47
//   visitLocalVariableExpression   L909-949
//   visitGameVariableExpression    L950-985
//   visitConstantVariableExpression L987-1082
//   parseConstantExpression        L1053-1063 (goscape caches AST not parse tree)
// ---------------------------------------------------------------------------

// visitLocalVariableExpression mirrors TS visitLocalVariableExpression
// (L909-949) at HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47.
// Resolves $name in the active local symbol table; validates array vs
// scalar usage; visits and type-checks the index when present.
func (tc *TypeChecker) visitLocalVariableExpression(lv *ast.LocalVariableExpression) {
	if tc.features.DisableProcs {
		diagnostics.ReportErrorAt(tc.diagnostics, lv, diagnostics.MessageFeatureDisabledLocal)
		lv.Type = typ.MetaError
		return
	}
	name := lv.Name.Text
	sym := tc.table.Find(symbol.SymbolTypeLocalVariable(), name)
	var local *symbol.LocalVariableSymbol
	if sym != nil {
		local, _ = sym.(*symbol.LocalVariableSymbol)
	}
	if local == nil {
		diagnostics.ReportErrorAt(tc.diagnostics, lv, diagnostics.MessageLocalReferenceUnresolved, name)
		lv.Type = typ.MetaError
		return
	}
	_, isArr := local.Type.(*typ.ArrayType)
	if !isArr && lv.IsArray() {
		diagnostics.ReportErrorAt(tc.diagnostics, lv, diagnostics.MessageLocalReferenceNotArray, name)
		lv.Type = typ.MetaError
		return
	}
	if isArr && !lv.IsArray() {
		diagnostics.ReportErrorAt(tc.diagnostics, lv, diagnostics.MessageLocalArrayReferenceNoIndex, name)
		lv.Type = typ.MetaError
		return
	}
	if isArr && lv.Index != nil {
		setTypeHint(lv.Index, typ.PrimitiveInt)
		tc.visitNodeOrNull(lv.Index)
		tc.checkTypeMatch(lv.Index, typ.PrimitiveInt, tc.getSafeType(lv.Index), true)
	}
	lv.Reference = local
	if arr, ok := local.Type.(*typ.ArrayType); ok {
		lv.Type = arr.Inner()
	} else {
		lv.Type = local.Type
	}
}

// visitGameVariableExpression mirrors TS visitGameVariableExpression
// (L950-985). Looks up `name` across all SymbolTypes in the root table,
// finds the first BasicSymbol whose Type is one of the game-var shapes
// (VarPlayer/VarBit/VarNpc/VarShared), and unwraps its Inner() type.
func (tc *TypeChecker) visitGameVariableExpression(gv *ast.GameVariableExpression) {
	name := gv.Name.Text
	all := tc.rootTable.FindAll(name)
	var found *symbol.BasicSymbol
	for _, s := range all {
		bs, ok := s.(*symbol.BasicSymbol)
		if !ok {
			continue
		}
		if isGameVarType(bs.Type) {
			found = bs
			break
		}
	}
	if found == nil {
		gv.Type = typ.MetaError
		diagnostics.ReportErrorAt(tc.diagnostics, gv, diagnostics.MessageGameReferenceUnresolved, name)
		return
	}
	gv.Reference = found
	gv.Type = gameVarInner(found.Type)
}

// isGameVarType returns true when t is one of the four game-var shapes.
func isGameVarType(t typ.Type) bool {
	switch t.(type) {
	case *typ.VarPlayerType, *typ.VarBitType, *typ.VarNpcType, *typ.VarSharedType:
		return true
	}
	return false
}

// gameVarInner returns the inner Type of a game-var, or t itself when t
// is not a game-var.
func gameVarInner(t typ.Type) typ.Type {
	switch v := t.(type) {
	case *typ.VarPlayerType:
		return v.Inner()
	case *typ.VarBitType:
		return v.Inner()
	case *typ.VarNpcType:
		return v.Inner()
	case *typ.VarSharedType:
		return v.Inner()
	}
	return t
}

// visitConstantVariableExpression mirrors TS visitConstantVariableExpression
// (L987-1082). Requires a type hint, looks up the constant symbol, parses
// the constant value as an expression (with cycle detection), visits the
// parsed AST, and asserts the result is itself a constant expression.
func (tc *TypeChecker) visitConstantVariableExpression(cv *ast.ConstantVariableExpression) {
	name := cv.Name.Text
	hint := asType(cv.TypeHint)
	if hint == nil {
		diagnostics.ReportErrorAt(tc.diagnostics, cv, diagnostics.MessageConstantUnknownType, name)
		cv.Type = typ.MetaError
		return
	}
	if hint == typ.MetaError {
		// Avoid attempting to parse the constant if it was type hinted to
		// error. An error will have been reported elsewhere.
		cv.Type = typ.MetaError
		return
	}
	sym := tc.rootTable.Find(symbol.SymbolTypeConstant(), name)
	constant, _ := sym.(*symbol.ConstantSymbol)
	if constant == nil {
		diagnostics.ReportErrorAt(tc.diagnostics, cv, diagnostics.MessageConstantReferenceUnresolved, name)
		cv.Type = typ.MetaError
		return
	}
	if tc.constantsBeingEvaluated[constant] {
		// Build a cycle-stack string (mirrors TS Set iteration + append).
		parts := make([]string, 0, len(tc.constantsBeingEvaluated)+1)
		for k := range tc.constantsBeingEvaluated {
			parts = append(parts, "^"+k.SymbolName())
		}
		parts = append(parts, "^"+constant.SymbolName())
		stack := strings.Join(parts, " -> ")
		diagnostics.ReportErrorAt(tc.diagnostics, cv, diagnostics.MessageConstantCyclicRef, stack)
		cv.Type = typ.MetaError
		return
	}
	tc.constantsBeingEvaluated[constant] = true
	defer delete(tc.constantsBeingEvaluated, constant)

	src := cv.Source()
	graphicType := tc.typeManager.FindOrNil("graphic", false)
	stringExpected := hint == typ.PrimitiveString || (graphicType != nil && hint == graphicType)

	var parsed ast.Expression
	if stringExpected {
		// String constants: wrap raw value in a StringLiteral at the host
		// location (TS does the same — no re-parse needed for strings).
		parsed = &ast.StringLiteral{
			SrcLoc: src,
			Value:  constant.Value,
		}
	} else {
		parsed = tc.parseConstantExpression(constant.Value, src)
	}
	if parsed == nil {
		diagnostics.ReportErrorAt(tc.diagnostics, cv, diagnostics.MessageConstantParseError, constant.Value, hint.Representation())
		cv.Type = typ.MetaError
		return
	}
	setTypeHint(parsed, hint)
	tc.visitNodeOrNull(parsed)
	if !tc.isConstantExpression(parsed) {
		diagnostics.ReportErrorAt(tc.diagnostics, cv, diagnostics.MessageConstantNonConstant, constant.Value)
		cv.Type = typ.MetaError
		return
	}
	cv.SubExpression = parsed
	cv.Type = getType(parsed)
}

// parseConstantExpression mirrors TS parseConstantExpression /
// parseConstantExpressionTree (L1053-1081 combined). Caches parsed AST
// nodes per value-string (NAI-206-D-CONST-CACHE-AST: goscape parses
// straight to AST so we cache the AST expression, unlike TS which caches
// the ParserRuleContext and runs AstBuilder on each read).
func (tc *TypeChecker) parseConstantExpression(value string, source lexer.NodeSourceLocation) ast.Expression {
	if cached, ok := tc.constantExpressionCache[value]; ok {
		return cached
	}
	p := parser.NewSingleExpressionParser(value, source.Name)
	p.RemoveErrorListeners()
	expr := p.ParseSingleExpression()
	tc.constantExpressionCache[value] = expr
	return expr
}

// ---------------------------------------------------------------------------
// T17 — Literals: Integer, Coord, Boolean, Character, Null, StringLiteral,
//        JoinedStringExpression, JoinedStringPart.
//
// TS reference: TypeChecking.ts HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47
//   visitIntegerLiteral        L1083-1098
//   visitCoordLiteral          L1100-1102
//   visitBooleanLiteral        L1104-1118
//   visitCharacterLiteral      L1120-1122
//   visitNullLiteral           L1124-1132
//   visitStringLiteral         L1134-1154
//   visitJoinedStringExpression L1192-1198
//   visitJoinedStringPart      L1200-1212
// ---------------------------------------------------------------------------

// literalTypes is the set of primitive types that a literal can naturally
// have without invoking the symbol-resolution fallback. Mirrors TS
// TypeChecking.LITERAL_TYPES static field.
var literalTypes = map[typ.Type]bool{
	typ.PrimitiveInt:     true,
	typ.PrimitiveBoolean: true,
	typ.PrimitiveCoord:   true,
	typ.PrimitiveString:  true,
	typ.PrimitiveChar:    true,
	typ.PrimitiveLong:    true,
}

// isHookType returns true when t is a MetaType.Hook(transmitListType).
func isHookType(t typ.Type) bool {
	_, ok := typ.IsMetaHook(t)
	return ok
}

// resolveSymbol mirrors TS resolveSymbol (L1240-1287) at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47. Walks every symbol named
// `name` in the active table (using FindAll), preferring the first one
// whose type is assignable from the hint. Falls back to the first
// resolved-with-a-type symbol when no exact match. Special-case for
// string-conversion: a string-hint and a string-convertible symbol
// resolves to "node.Type = string, return nil" (no symbol reference).
//
// Retires NAI-206-D-RESOLVE-SYMBOL-STUB-T18 — real impl is here.
func (tc *TypeChecker) resolveSymbol(node ast.Expression, name string, hint typ.Type, allowToString bool) symbol.Symbol {
	var sym symbol.Symbol
	var symbolType typ.Type
	for _, tmp := range tc.table.FindAll(name) {
		tt := tc.symbolToType(tmp)
		if tt == nil {
			continue
		}
		if hint == nil {
			if _, _, ok := typ.IsMetaScript(tt); ok {
				continue
			}
		}
		if hint == nil || tc.typeManager.Check(hint, tt) {
			sym = tmp
			symbolType = tt
			break
		}
		if sym == nil {
			sym = tmp
			symbolType = tt
		}
	}
	if allowToString && hint == typ.PrimitiveString && tc.allowStringConversion(sym) {
		setType(node, typ.PrimitiveString)
		return nil
	}
	if sym == nil {
		setType(node, typ.MetaError)
		diagnostics.ReportErrorAt(tc.diagnostics, node, diagnostics.MessageGenericUnresolvedSymbol, name)
		return nil
	}
	if symbolType == nil {
		setType(node, typ.MetaError)
		diagnostics.ReportErrorAt(tc.diagnostics, node, diagnostics.MessageUnsupportedSymbolTypeToType, symbolKindName(sym))
		return nil
	}
	setType(node, symbolType)
	return sym
}

// symbolKindName returns a human-readable kind label for diagnostic
// messages (mirrors TS symbol-class names).
func symbolKindName(s symbol.Symbol) string {
	switch s.(type) {
	case *symbol.ServerScriptSymbol:
		return "ServerScriptSymbol"
	case *symbol.ClientScriptSymbol:
		return "ClientScriptSymbol"
	case *symbol.LocalVariableSymbol:
		return "LocalVariableSymbol"
	case *symbol.BasicSymbol:
		return "BasicSymbol"
	case *symbol.ConstantSymbol:
		return "ConstantSymbol"
	}
	return "Unknown"
}

// symbolToType mirrors TS symbolToType (L1357-1391). Maps a symbol to
// the type used during identifier resolution:
//   - ServerScriptSymbol with the command trigger ⇒ Returns
//   - ServerScriptSymbol with any other trigger    ⇒ MetaType.Script
//   - LocalVariableSymbol (array) ⇒ ArrayType
//   - LocalVariableSymbol (scalar) ⇒ nil  (only arrays are identifier-resolvable)
//   - BasicSymbol ⇒ Type
//   - ConstantSymbol ⇒ nil (consumed via the ^FOO syntax, not by identifier)
func (tc *TypeChecker) symbolToType(s symbol.Symbol) typ.Type {
	switch v := s.(type) {
	case *symbol.ServerScriptSymbol:
		if tc.commandTrigger != nil && v.Trigger == tc.commandTrigger {
			return v.Returns
		}
		if v.Trigger == nil {
			return nil
		}
		return typ.NewMetaScript(v.Trigger.Identifier, v.Parameters, v.Returns)
	case *symbol.LocalVariableSymbol:
		if _, isArr := v.Type.(*typ.ArrayType); isArr {
			return v.Type
		}
		return nil
	case *symbol.BasicSymbol:
		return v.Type
	case *symbol.ConstantSymbol:
		return nil
	}
	return nil
}

// allowStringConversion mirrors TS allowStringConversion (L1353-1356).
// Commands cannot be string-converted (their names are not values);
// every other symbol kind can. Nil symbol ⇒ true (no constraint).
func (tc *TypeChecker) allowStringConversion(s symbol.Symbol) bool {
	if s == nil {
		return true
	}
	if script, ok := s.(*symbol.ServerScriptSymbol); ok {
		if tc.commandTrigger != nil && script.Trigger == tc.commandTrigger {
			return false
		}
	}
	return true
}

// visitIdentifier mirrors TS visitIdentifier (L1214-1238). Routes
// through checkDynamicCommand first (dynamic-cmd registry path); then
// resolves the symbol via resolveSymbol; emits diagnostics if the
// resolved command symbol has parameters but is referenced as a bare
// identifier (TS line 1226-1230) OR is feature-disabled.
func (tc *TypeChecker) visitIdentifier(id *ast.Identifier) {
	name := id.Text
	if tc.checkDynamicCommand(name, id) {
		return
	}
	hint := asType(id.TypeHint)
	sym := tc.resolveSymbol(id, name, hint, true)
	if sym == nil {
		return
	}
	if script, ok := sym.(*symbol.ServerScriptSymbol); ok && tc.commandTrigger != nil && script.Trigger == tc.commandTrigger {
		if script.Parameters != typ.MetaUnit {
			diagnostics.ReportErrorAt(tc.diagnostics, id, diagnostics.MessageGenericTypeMismatch, "<unit>", script.Parameters.Representation())
		}
		if tc.isDisabledCommandName(script.Name) {
			diagnostics.ReportErrorAt(tc.diagnostics, id, diagnostics.MessageFeatureDisabledCommand, script.Name)
			id.Type = typ.MetaError
			return
		}
	}
	id.Reference = sym
}

// intToString formats an int32 as a decimal string, used by
// visitIntegerLiteral to pass the integer value as a name to resolveSymbol.
func intToString(n int32) string {
	return fmt.Sprintf("%d", n)
}

// visitIntegerLiteral mirrors TS visitIntegerLiteral (L1083-1098).
// Branch order is preserved exactly from TS:
//  1. No hint / Unit hint / int-assignable hint → int.
//  2. Non-literal hint → resolveSymbol (sets .Type and .Reference).
//  3. Boolean hint with value 0 or 1 → boolean.
//  4. String hint → string.
//  5. Any other literal hint → int.
func (tc *TypeChecker) visitIntegerLiteral(il *ast.IntegerLiteral) {
	hint := asType(il.TypeHint)
	if hint == nil || hint == typ.MetaUnit || tc.typeManager.Check(hint, typ.PrimitiveInt) {
		il.Type = typ.PrimitiveInt
	} else if !literalTypes[hint] {
		il.Reference = tc.resolveSymbol(il, intToString(il.Value), hint, false)
		// resolveSymbol (T18) always sets il.Type; the stub (T17) returns nil
		// without setting it, so guard with a fallback.
		if il.Type == nil {
			il.Type = typ.PrimitiveInt
		}
	} else if hint == typ.PrimitiveBoolean && (il.Value == 0 || il.Value == 1) {
		il.Type = typ.PrimitiveBoolean
	} else if hint == typ.PrimitiveString {
		il.Type = typ.PrimitiveString
	} else {
		il.Type = typ.PrimitiveInt
	}
}

// visitCoordLiteral mirrors TS visitCoordLiteral (L1100-1102).
func (tc *TypeChecker) visitCoordLiteral(cl *ast.CoordLiteral) {
	cl.Type = typ.PrimitiveCoord
}

// visitBooleanLiteral mirrors TS visitBooleanLiteral (L1104-1118).
// TS reads hint from booleanLiteral.type (not .typeHint) — this is a TS
// quirk where the type field is populated before the visitor runs. In
// goscape the hint is always in .TypeHint; .Type is only set by visitors.
//
// NAI-206-D-BOOL-HINT-FIELD: TS L1111 reads `booleanLiteral.type` (the
// type field) as the hint; goscape reads TypeHint (the intended hint
// field). Semantics are identical: the AstBuilder sets neither field, so
// both are nil in normal use; the only difference surfaces in tests that
// manually set one field or the other.
func (tc *TypeChecker) visitBooleanLiteral(bl *ast.BooleanLiteral) {
	if tc.features.DisableBooleans {
		diagnostics.ReportErrorAt(tc.diagnostics, bl, diagnostics.MessageFeatureDisabledBoolean)
		bl.Type = typ.MetaError
		return
	}
	if asType(bl.TypeHint) == typ.PrimitiveString {
		bl.Type = typ.PrimitiveString
		return
	}
	bl.Type = typ.PrimitiveBoolean
}

// visitCharacterLiteral mirrors TS visitCharacterLiteral (L1120-1122).
func (tc *TypeChecker) visitCharacterLiteral(cl *ast.CharacterLiteral) {
	cl.Type = typ.PrimitiveChar
}

// visitNullLiteral mirrors TS visitNullLiteral (L1124-1132). NullLiteral
// is a generic -1; defaults to int when no hint, else inherits hint.
func (tc *TypeChecker) visitNullLiteral(nl *ast.NullLiteral) {
	if hint := asType(nl.TypeHint); hint != nil {
		nl.Type = hint
		return
	}
	nl.Type = typ.PrimitiveInt
}

// visitStringLiteral mirrors TS visitStringLiteral (L1134-1154).
// No-hint or string-assignable hint ⇒ string.
// Hook hint ⇒ re-parse the value as a clientscript reference via
// handleClientScriptExpression (T15).
// Non-literal hint ⇒ route to resolveSymbol (T18).
// Any other literal hint ⇒ string.
func (tc *TypeChecker) visitStringLiteral(sl *ast.StringLiteral) {
	hint := asType(sl.TypeHint)
	if hint == nil || tc.typeManager.Check(hint, typ.PrimitiveString) {
		sl.Type = typ.PrimitiveString
	} else if isHookType(hint) {
		tc.handleClientScriptExpression(sl, hint)
	} else if !literalTypes[hint] {
		sl.Reference = tc.resolveSymbol(sl, sl.Value, hint, false)
		// resolveSymbol (T18) always sets sl.Type; the stub returns nil
		// without setting it, so guard with a fallback.
		if sl.Type == nil {
			sl.Type = typ.PrimitiveString
		}
	} else {
		sl.Type = typ.PrimitiveString
	}
}

// visitJoinedStringExpression mirrors TS visitJoinedStringExpression
// (L1192-1198). Walks each part via visitJoinedStringPart; result type is
// always string.
func (tc *TypeChecker) visitJoinedStringExpression(jse *ast.JoinedStringExpression) {
	for _, part := range jse.Parts {
		tc.visitJoinedStringPart(part)
	}
	jse.Type = typ.PrimitiveString
}

// visitJoinedStringPart mirrors TS visitJoinedStringPart (L1200-1212).
// For ExpressionStringPart, type-hints the inner expression to string,
// walks it, then asserts the result is assignable to string.
// BasicStringPart and PTagStringPart carry no expression — no action taken.
func (tc *TypeChecker) visitJoinedStringPart(part ast.StringPart) {
	esp, ok := part.(*ast.ExpressionStringPart)
	if !ok {
		return
	}
	if esp.Expression == nil {
		return
	}
	setTypeHint(esp.Expression, typ.PrimitiveString)
	tc.Visit(esp.Expression)
	tc.checkTypeMatch(esp.Expression, typ.PrimitiveString, tc.getSafeType(esp.Expression), true)
}
