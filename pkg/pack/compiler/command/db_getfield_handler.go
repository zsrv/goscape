// pkg/pack/compiler/command/db_getfield_handler.go
package command

// DbGetFieldCommandHandler handles the `db_getfield` command that returns a
// dynamic type based on the column that was passed in. Mirrors TS
// DbGetFieldCommandHandler.ts.
//
// Example:
//
//	$int, $obj, $string = db_getfield(some_row, table:column, 0);
//
// The handler:
//  1. Checks arg 0 as a dbrow type.
//  2. Checks arg 1 as a dbcolumn<any> type, then extracts the column's inner type.
//  3. Checks arg 2 as an int (field index).
//  4. Validates all three argument types as (dbrow, dbcolumn<T>, int).
//  5. Sets the expression type to T (or MetaError if unknown/unresolvable).

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// DbGetFieldCommandHandler implements DynamicCommandHandler for `db_getfield`.
type DbGetFieldCommandHandler struct{}

// dbColumnAny is a dbcolumn<any> sentinel used as the hint when checking the
// column argument. Mirrors TS `new DbColumnType(MetaType.Any)`.
var dbColumnAny = typ.NewDbColumnType(typ.MetaAny)

// TypeCheck ports DbGetFieldCommandHandler.ts typeCheck verbatim.
func (h *DbGetFieldCommandHandler) TypeCheck(ctx *semantics.TypeCheckingContext) {
	// Check arg 0 as dbrow.
	dbrowType := ctx.TypeManager.FindOrNil("dbrow", false)
	ctx.CheckArgument(0, dbrowType, false)

	// Check arg 1 as dbcolumn<any>; extract the column's inner type.
	columnExpr := ctx.CheckArgument(1, dbColumnAny, false)

	// Check arg 2 as int (field index).
	ctx.CheckArgument(2, typ.PrimitiveInt, false)

	// Extract the column return type from the expression's resolved type.
	var columnReturnType typ.Type
	if columnExpr != nil {
		if t := semantics.ExprType(columnExpr); t != nil {
			if inner, ok := typ.IsDbColumnType(t); ok {
				columnReturnType = inner
			}
		}
	}

	// Build expected tuple: (dbrow, dbcolumn<T>, int).
	var dbrowHint typ.Type
	if dbrowType != nil {
		dbrowHint = dbrowType
	} else {
		dbrowHint = typ.MetaAny
	}
	colInner := typ.MetaAny
	if columnReturnType != nil {
		colInner = columnReturnType
	}
	expected := typ.TupleFromList([]typ.Type{
		dbrowHint,
		typ.NewDbColumnType(colInner),
		typ.PrimitiveInt,
	})

	if !ctx.CheckArgumentTypes(expected, true, false) {
		ctx.SetType(typ.MetaError)
		return
	}

	// If columnExpr was nil (out of bounds), report an error.
	if columnExpr == nil {
		ctx.SetType(typ.MetaError)
		return
	}

	// Set the return type to the column's inner type.
	if columnReturnType != nil {
		ctx.SetType(columnReturnType)
	} else {
		ctx.SetType(typ.MetaError)
	}
}

// GenerateCode returns false — cohort A: codegen falls back to the default
// "visit args + emit Command" path.
func (h *DbGetFieldCommandHandler) GenerateCode(ctx semantics.CodeGenContext) bool {
	return false
}
