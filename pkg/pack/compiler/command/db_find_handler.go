// pkg/pack/compiler/command/db_find_handler.go
package command

// DbFindCommandHandler handles dynamic type-checking and code generation for
// the `db_find` and `db_find_with_count` commands. Mirrors TS
// DbFindCommandHandler.ts.
//
// The handler:
//  1. Checks arg 0 as a dbcolumn<any> type (the column to search in).
//  2. Uses the column's inner type to type-hint arg 1 (the key value).
//  3. Rejects Tuple inner types with a diagnostic.
//  4. Sets the expression type to PrimitiveInt (withCount) or MetaUnit.
//
// GenerateCode:
//  1. Emits all arguments via VisitNode.
//  2. Emits PushConstantInt(stackType) — the BaseVarType of the column's inner type.
//  3. Emits Command.

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// DbFindCommandHandler implements DynamicCommandHandler for `db_find` and
// `db_find_with_count`.
type DbFindCommandHandler struct {
	// withCount determines whether the expression returns an int (true) or unit.
	// Mirrors TS DbFindCommandHandler.withCount.
	withCount bool
}

// NewDbFindCommandHandler constructs a DbFindCommandHandler.
// withCount=true for db_find_with_count, false for db_find.
func NewDbFindCommandHandler(withCount bool) *DbFindCommandHandler {
	return &DbFindCommandHandler{withCount: withCount}
}

// dbFindColumnAny is a dbcolumn<any> sentinel for the first argument hint.
// Mirrors TS `new DbColumnType(MetaType.Any)`.
var dbFindColumnAny = typ.NewDbColumnType(typ.MetaAny)

// TypeCheck ports DbFindCommandHandler.ts typeCheck verbatim.
func (h *DbFindCommandHandler) TypeCheck(ctx *semantics.TypeCheckingContext) {
	// Check arg 0 as a dbcolumn<any>; extract the column's inner type.
	columnExpr := ctx.CheckArgument(0, dbFindColumnAny, false)

	// Type-hint the key argument using the dbcolumn's inner type, if resolved.
	var keyType typ.Type
	if columnExpr != nil {
		if t := semantics.ExprType(columnExpr); t != nil {
			if inner, ok := typ.IsDbColumnType(t); ok {
				keyType = inner
			}
		}
	}
	ctx.CheckArgument(1, keyType, false)

	// Build the expected tuple: (dbcolumn<keyType>, keyType).
	colHint := keyType
	if colHint == nil {
		colHint = typ.MetaAny
	}
	expected := typ.TupleFromList([]typ.Type{
		typ.NewDbColumnType(colHint),
		colHint,
	})

	// Check that the key type is not a Tuple type. Mirrors TS L31-38:
	// if (keyType instanceof TupleType) { columnExpr.reportError(...) }
	// else { context.checkArgumentTypes(expectedTypes); }
	if _, ok := keyType.(*typ.TupleType); ok {
		// TS reports the error on columnExpr and skips checkArgumentTypes.
		// Mirrors NAI-205-D-NO-NODE-REPORT-ERROR.
		if columnExpr != nil {
			diagnostics.ReportErrorAt(ctx.Diagnostics, columnExpr, "Tuple columns are not supported.")
		}
	} else {
		ctx.CheckArgumentTypes(expected, true, false)
	}

	// Set the return type.
	if h.withCount {
		ctx.SetType(typ.PrimitiveInt)
	} else {
		ctx.SetType(typ.MetaUnit)
	}
}

// GenerateCode ports DbFindCommandHandler.ts generateCode verbatim.
// Emits: args + PushConstantInt(stackType) + Command.
// stackType is the int value of the column inner's BaseVarType.
// Mirrors TS L44-L60.
func (h *DbFindCommandHandler) GenerateCode(ctx semantics.CodeGenContext) bool {
	cgc := ctx.(*codegen.CodeGeneratorContext)

	args := cgc.Arguments()

	// Extract the column inner's BaseVarType from arg 0's resolved type.
	// Should not get to this point unless arg 0 is a dbcolumn.
	var stackType typ.BaseVarType
	if len(args) > 0 {
		if t := semantics.ExprType(args[0]); t != nil {
			if inner, ok := typ.IsDbColumnType(t); ok {
				if base, ok2 := inner.BaseType(); ok2 {
					stackType = base
				}
			}
		}
	}

	// Emit all arguments.
	for _, arg := range args {
		cgc.VisitNode(arg)
	}

	// Emit the stack type as an integer constant. Mirrors TS L57:
	// context.instruction(Opcode.PushConstantInt, stackType).
	cgc.Instruction(codegen.PushConstantInt, int(stackType), cgc.Expression.Source())

	// Emit the Command.
	cgc.Command()

	return true
}
