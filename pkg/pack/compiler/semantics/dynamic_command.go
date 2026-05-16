// pkg/pack/compiler/semantics/dynamic_command.go
package semantics

// DynamicCommandHandler and TypeCheckingContext — port of TS
// DynamicCommandHandler.ts + TypeCheckingContext.ts.
//
// NAI-206-D-DYNCOMMAND-EMPTY: no concrete handlers are registered here;
// the follow-up handler cohort (enum/struct_param/db_*) wires them in a
// separate NAI.
//
// NAI-207 extends DynamicCommandHandler with GenerateCode, retiring
// NAI-206-D-DYNCOMMAND-NO-CODEGEN. See also NAI-207-D-DYNCOMMAND-BOOLRESULT
// (GenerateCode returns bool, not void, per goscape convention) and
// NAI-207-D-CODEGENCONTEXT-MARKER (CodeGenContext is a marker interface in
// semantics to avoid a codegen→semantics import cycle).
//
// The type-switch helpers setTypeHint / getType / setType / asType cover all
// 19 concrete Expression types that embed ExpressionBase. They are the
// cross-cutting plumbing the walker arms (T8-T18) use to read/write the
// ExpressionBase mixin fields (Type and TypeHint) without per-arm boilerplate,
// since the fields are embedded in the concrete structs (not accessible via the
// Expression interface).

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// CodeGenContext is a marker interface passed to DynamicCommandHandler.GenerateCode.
// Defined here (in semantics) so that codegen can implement it without importing
// semantics, avoiding a circular dependency. Mirrors TS CodeGenContext.ts.
//
// NAI-207-D-CODEGENCONTEXT-MARKER: goscape places this marker in semantics
// rather than codegen; the concrete *codegen.CodeGeneratorContext satisfies it
// via IsCodeGenContext(). NAI-207-D-CODEGENCONTEXT-EXPORTEDMARKER: the marker
// method is exported because Go's interface rules prevent a type in package
// codegen from satisfying an unexported method declared in package semantics —
// unexported interface methods are only satisfiable within the declaring package.
// Because the method is exported, marker-protection relies on convention +
// doc-comment rather than Go enforcement. Mirrors the sibling pattern in
// pkg/pack/compiler/ast/symbol_refs.go (NAI-205-D-AST-REF-INTERFACES) where
// AsSymbolRef()/AsTypeRef() are also exported markers for the same cross-package
// reason.
type CodeGenContext interface {
	IsCodeGenContext()
}

// DynamicCommandHandler allows complex commands to do custom type checking
// and code generation. Implementations must set expression.Type in TypeCheck
// and emit instructions in GenerateCode. Mirrors TS interface
// DynamicCommandHandler (DynamicCommandHandler.ts).
//
// Retires NAI-206-D-DYNCOMMAND-NO-CODEGEN.
// See NAI-207-D-DYNCOMMAND-BOOLRESULT for the bool return convention.
type DynamicCommandHandler interface {
	// TypeCheck performs type-checking for the dynamic command call. The
	// expression will be a *CommandCallExpression or *Identifier. The
	// implementation MUST set the expression's Type field via
	// ctx.SetType/ctx.CheckArgument or equivalent.
	TypeCheck(ctx *TypeCheckingContext)

	// GenerateCode emits codegen instructions for the dynamic command call.
	// Returns true if code was emitted, false if the default Command emission
	// should proceed. NAI-207-D-DYNCOMMAND-BOOLRESULT: TS returns void;
	// goscape returns bool so callers can apply a default fallback.
	GenerateCode(ctx CodeGenContext) bool
}

// TypeCheckingContext carries the context of a TypeChecker visit for use by
// DynamicCommandHandler implementations. Mirrors TS class TypeCheckingContext
// (TypeCheckingContext.ts).
type TypeCheckingContext struct {
	// typeChecker is the owning TypeChecker; used for visitNodeOrNull, etc.
	typeChecker *TypeChecker

	// TypeManager is the global type registry. Exported for handler use.
	TypeManager *typ.TypeManager

	// expression is the AST node being checked (CommandCallExpression or Identifier).
	expression ast.Expression

	// Diagnostics is the shared diagnostic collector. Exported for handler use.
	Diagnostics *diagnostics.Diagnostics
}

// newTypeCheckingContext constructs a TypeCheckingContext. Used by the walker
// arms (T14-T18) when dispatching to a DynamicCommandHandler.
func newTypeCheckingContext(
	tc *TypeChecker,
	tm *typ.TypeManager,
	expr ast.Expression,
	d *diagnostics.Diagnostics,
) *TypeCheckingContext {
	return &TypeCheckingContext{
		typeChecker: tc,
		TypeManager: tm,
		expression:  expr,
		Diagnostics: d,
	}
}

// Arguments returns the argument list if expression is a CallExpressionNode,
// otherwise returns nil. Mirrors TS get arguments() in TypeCheckingContext.
func (ctx *TypeCheckingContext) Arguments() []ast.Expression {
	if call, ok := ctx.expression.(ast.CallExpressionNode); ok {
		return argumentsList(call, false)
	}
	return nil
}

// argumentsList returns the argument list for a CallExpressionNode. When
// args2 is true and the call is a *CommandCallExpression with a secondary
// argument list, returns Arguments2 instead. Mirrors TS
// TypeCheckingContext.getArgumentsList().
func argumentsList(call ast.CallExpressionNode, args2 bool) []ast.Expression {
	if args2 {
		if cmd, ok := call.(*ast.CommandCallExpression); ok && cmd.Arguments2 != nil {
			return cmd.Arguments2
		}
	}
	switch c := call.(type) {
	case *ast.CommandCallExpression:
		return c.Arguments
	case *ast.ProcCallExpression:
		return c.Arguments
	case *ast.JumpCallExpression:
		return c.Arguments
	case *ast.ClientScriptExpression:
		return c.Arguments
	}
	return nil
}

// Arguments2 returns the secondary argument list if expression is a
// *ast.CommandCallExpression with a non-nil Arguments2, otherwise returns nil.
// Mirrors TS get arguments2() in TypeCheckingContext.
func (ctx *TypeCheckingContext) Arguments2() []ast.Expression {
	if call, ok := ctx.expression.(ast.CallExpressionNode); ok {
		if cmd, ok := call.(*ast.CommandCallExpression); ok && cmd.Arguments2 != nil {
			return argumentsList(cmd, true)
		}
	}
	return nil
}

// CheckArgumentTypes compares the wrapped call's actual argument types against
// expected (a tuple). Returns true iff they match. When reportError is true and
// there is a mismatch, emits a diagnostic via checkTypeMatch. args2 controls
// whether to use the secondary argument list (Arguments2). Mirrors TS
// TypeCheckingContext.checkArgumentTypes() (TypeCheckingContext.ts L156).
func (ctx *TypeCheckingContext) CheckArgumentTypes(expected typ.Type, reportError bool, args2 bool) bool {
	var args []ast.Expression
	if call, ok := ctx.expression.(ast.CallExpressionNode); ok {
		args = argumentsList(call, args2)
	}
	actualList := make([]typ.Type, 0, len(args))
	for _, a := range args {
		actualList = append(actualList, getType(a))
	}
	actual := typ.TupleFromList(actualList)
	return ctx.typeChecker.checkTypeMatch(ctx.expression, expected, actual, reportError)
}

// CheckArgument visits the argument at index with an optional typeHint. Returns
// the argument expression or nil if out of bounds. Mirrors TS
// TypeCheckingContext.checkArgument().
func (ctx *TypeCheckingContext) CheckArgument(index int, typeHint typ.Type, args2 bool) ast.Expression {
	var args []ast.Expression
	if call, ok := ctx.expression.(ast.CallExpressionNode); ok {
		args = argumentsList(call, args2)
	}
	if index < 0 || index >= len(args) {
		return nil
	}
	arg := args[index]
	if typeHint != nil {
		setTypeHint(arg, typeHint)
	}
	if ctx.typeChecker != nil {
		ctx.typeChecker.visitNodeOrNull(arg)
	}
	return arg
}

// IsConstant reports whether the expression is a constant expression. Delegates
// to TypeChecker.isConstantExpression (which is a stub returning false until T9).
// Mirrors TS TypeCheckingContext.isConstant getter.
func (ctx *TypeCheckingContext) IsConstant() bool {
	if ctx.expression == nil {
		return false
	}
	if ctx.typeChecker == nil {
		return false
	}
	return ctx.typeChecker.isConstantExpression(ctx.expression)
}

// VisitNode passes node through the type checker. Mirrors TS
// TypeCheckingContext.visitNode().
func (ctx *TypeCheckingContext) VisitNode(n ast.Node) {
	if n == nil || ctx.typeChecker == nil {
		return
	}
	ctx.typeChecker.visitNodeOrNull(n)
}

// VisitExpression visits expr with an optional type hint. Mirrors TS
// TypeCheckingContext.visitExpression().
func (ctx *TypeCheckingContext) VisitExpression(expr ast.Expression, typeHint typ.Type) {
	if expr == nil {
		return
	}
	if typeHint != nil {
		setTypeHint(expr, typeHint)
	}
	if ctx.typeChecker != nil {
		ctx.typeChecker.visitNodeOrNull(expr)
	}
}

// VisitNodeList passes all nodes through the type checker. Mirrors TS
// TypeCheckingContext.visitNodeList().
func (ctx *TypeCheckingContext) VisitNodeList(nodes []ast.Node) {
	if ctx.typeChecker == nil {
		return
	}
	for _, n := range nodes {
		ctx.typeChecker.visitNodeOrNull(n)
	}
}

// ExprType reads the Type field from any ast.Expression. This is the
// public sibling of the unexported getType helper, exposed so that
// DynamicCommandHandler implementations in external packages (e.g.
// pkg/pack/compiler/command) can inspect the type of argument expressions
// returned by CheckArgument / CheckTypeArgument. Mirrors TS
// `expression.getNullableType()` / `expression.type`.
func ExprType(expr ast.Expression) typ.Type {
	return getType(expr)
}

// SetType sets the Type field on the wrapped expression. This is the
// primary mechanism for DynamicCommandHandler implementations to report
// their computed return type. Mirrors TS `context.expression.type = t`.
func (ctx *TypeCheckingContext) SetType(t typ.Type) {
	setType(ctx.expression, t)
}

// CheckTypeArgument validates the argument at index as a type reference.
// If the argument is an *ast.Identifier, it looks up the identifier text
// in TypeManager; on success, sets the argument's Type to
// NewMetaWrapping(found) and its Reference to a BasicSymbol, then returns
// the argument expression. On failure (out-of-bounds, not an Identifier,
// or unknown type), emits a diagnostic and sets the argument's Type to
// MetaError. Mirrors TS TypeCheckingContext.checkTypeArgument().
//
// Used by EnumCommandHandler to resolve the input/output type arguments
// of the `enum` command.
func (ctx *TypeCheckingContext) CheckTypeArgument(index int) ast.Expression {
	var args []ast.Expression
	if call, ok := ctx.expression.(ast.CallExpressionNode); ok {
		args = argumentsList(call, false)
	}
	if index < 0 || index >= len(args) {
		return nil
	}
	arg := args[index]
	id, ok := arg.(*ast.Identifier)
	if !ok {
		diagnostics.ReportErrorAt(ctx.Diagnostics, arg, diagnostics.MessageTypeRefExpected)
		setType(arg, typ.MetaError)
		return arg
	}
	found := ctx.TypeManager.FindOrNil(id.Text, false)
	if found == nil {
		diagnostics.ReportErrorAt(ctx.Diagnostics, id, diagnostics.MessageGenericInvalidType, id.Text)
		id.Type = typ.MetaError
		return id
	}
	wrapped := typ.NewMetaWrapping(found)
	id.Type = wrapped
	id.Reference = &symbol.BasicSymbol{Name: id.Text, Type: wrapped}
	return id
}

// ---------------------------------------------------------------------------
// Type-switch helpers — setTypeHint / getType / setType / asType
//
// These provide read/write access to ExpressionBase.Type and
// ExpressionBase.TypeHint on any concrete Expression without requiring a
// cast at each call site. The type-switch exhaustively covers all 19 concrete
// Expression types that embed ExpressionBase (verified against T1 audit).
// ---------------------------------------------------------------------------

// setTypeHint sets the TypeHint field on the ExpressionBase embedded in expr.
// Mirrors TS `expr.typeHint = t` in TypeChecking / TypeCheckingContext.
func setTypeHint(expr ast.Expression, t typ.Type) {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		e.TypeHint = t
	case *ast.CoordLiteral:
		e.TypeHint = t
	case *ast.BooleanLiteral:
		e.TypeHint = t
	case *ast.CharacterLiteral:
		e.TypeHint = t
	case *ast.StringLiteral:
		e.TypeHint = t
	case *ast.NullLiteral:
		e.TypeHint = t
	case *ast.Identifier:
		e.TypeHint = t
	case *ast.LocalVariableExpression:
		e.TypeHint = t
	case *ast.GameVariableExpression:
		e.TypeHint = t
	case *ast.ConstantVariableExpression:
		e.TypeHint = t
	case *ast.ConditionExpression:
		e.TypeHint = t
	case *ast.ArithmeticExpression:
		e.TypeHint = t
	case *ast.CalcExpression:
		e.TypeHint = t
	case *ast.ParenthesizedExpression:
		e.TypeHint = t
	case *ast.JoinedStringExpression:
		e.TypeHint = t
	case *ast.CommandCallExpression:
		e.TypeHint = t
	case *ast.ProcCallExpression:
		e.TypeHint = t
	case *ast.JumpCallExpression:
		e.TypeHint = t
	case *ast.ClientScriptExpression:
		e.TypeHint = t
	}
}

// getType reads the Type field from the ExpressionBase embedded in expr.
// Returns nil if expr is nil or if the type has not been set yet.
// Mirrors TS `expr.type` / `expr.getNullableType()`.
func getType(expr ast.Expression) typ.Type {
	if expr == nil {
		return nil
	}
	return asType(typeRefOf(expr))
}

// setType sets the Type field on the ExpressionBase embedded in expr.
// Mirrors TS `expr.type = t`.
func setType(expr ast.Expression, t typ.Type) {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		e.Type = t
	case *ast.CoordLiteral:
		e.Type = t
	case *ast.BooleanLiteral:
		e.Type = t
	case *ast.CharacterLiteral:
		e.Type = t
	case *ast.StringLiteral:
		e.Type = t
	case *ast.NullLiteral:
		e.Type = t
	case *ast.Identifier:
		e.Type = t
	case *ast.LocalVariableExpression:
		e.Type = t
	case *ast.GameVariableExpression:
		e.Type = t
	case *ast.ConstantVariableExpression:
		e.Type = t
	case *ast.ConditionExpression:
		e.Type = t
	case *ast.ArithmeticExpression:
		e.Type = t
	case *ast.CalcExpression:
		e.Type = t
	case *ast.ParenthesizedExpression:
		e.Type = t
	case *ast.JoinedStringExpression:
		e.Type = t
	case *ast.CommandCallExpression:
		e.Type = t
	case *ast.ProcCallExpression:
		e.Type = t
	case *ast.JumpCallExpression:
		e.Type = t
	case *ast.ClientScriptExpression:
		e.Type = t
	}
}

// typeRefOf reads the raw ast.TypeRef (which carries a typ.Type under the
// AsTypeRef() contract) from the concrete ExpressionBase. Used internally
// by getType.
func typeRefOf(expr ast.Expression) ast.TypeRef {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return e.Type
	case *ast.CoordLiteral:
		return e.Type
	case *ast.BooleanLiteral:
		return e.Type
	case *ast.CharacterLiteral:
		return e.Type
	case *ast.StringLiteral:
		return e.Type
	case *ast.NullLiteral:
		return e.Type
	case *ast.Identifier:
		return e.Type
	case *ast.LocalVariableExpression:
		return e.Type
	case *ast.GameVariableExpression:
		return e.Type
	case *ast.ConstantVariableExpression:
		return e.Type
	case *ast.ConditionExpression:
		return e.Type
	case *ast.ArithmeticExpression:
		return e.Type
	case *ast.CalcExpression:
		return e.Type
	case *ast.ParenthesizedExpression:
		return e.Type
	case *ast.JoinedStringExpression:
		return e.Type
	case *ast.CommandCallExpression:
		return e.Type
	case *ast.ProcCallExpression:
		return e.Type
	case *ast.JumpCallExpression:
		return e.Type
	case *ast.ClientScriptExpression:
		return e.Type
	}
	return nil
}

// asType converts an ast.TypeRef to typ.Type. Both interfaces satisfy the
// AsTypeRef() method (it is the marker). Returns nil if ref is nil.
func asType(ref ast.TypeRef) typ.Type {
	if ref == nil {
		return nil
	}
	t, ok := ref.(typ.Type)
	if !ok {
		return nil
	}
	return t
}
