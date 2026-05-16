// pkg/pack/compiler/command/enum_handler.go
package command

// EnumCommandHandler handles the `enum` command's dynamic type-checking.
// Mirrors TS EnumCommandHandler.ts.
//
// Example:
//
//	def_obj $item = enum(int, obj, item_list, $index);
//
// The handler:
//  1. Resolves arg 0 as a type reference (the input type).
//  2. Resolves arg 1 as a type reference (the output type).
//  3. Checks arg 2 as an enum symbol.
//  4. Type-hints arg 3 with the resolved input type.
//  5. Validates all four argument types as (type<in>, type<out>, enum, in).
//  6. Sets the expression's type to the resolved output type (or MetaError).

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// EnumCommandHandler implements DynamicCommandHandler for the `enum` command.
type EnumCommandHandler struct{}

// TypeCheck ports EnumCommandHandler.ts typeCheck verbatim.
func (h *EnumCommandHandler) TypeCheck(ctx *semantics.TypeCheckingContext) {
	// Resolve arg 0 as a type reference (input type).
	inputTypeExpr := ctx.CheckTypeArgument(0)

	// Resolve arg 1 as a type reference (output type).
	outputTypeExpr := ctx.CheckTypeArgument(1)

	// Look up the registered enum type and check arg 2 with that hint.
	enumType := ctx.TypeManager.FindOrNil("enum", false)
	ctx.CheckArgument(2, enumType, false)

	// Extract the inner types from the MetaWrapping results.
	var inputType, outputType typ.Type
	if inputTypeExpr != nil {
		if inner, ok := typ.IsMetaWrapping(semantics.ExprType(inputTypeExpr)); ok {
			inputType = inner
		}
	}
	if outputTypeExpr != nil {
		if inner, ok := typ.IsMetaWrapping(semantics.ExprType(outputTypeExpr)); ok {
			outputType = inner
		}
	}

	// Type-hint arg 3 with the resolved input type (or MetaAny if unresolved).
	inputHint := inputType
	if inputHint == nil {
		inputHint = typ.MetaAny
	}
	ctx.CheckArgument(3, inputHint, false)

	// Build expected tuple: (type<in>, type<out>, enum, in).
	inWrapped := typ.NewMetaWrapping(inputHint)
	outWrapped := typ.NewMetaWrapping(func() typ.Type {
		if outputType != nil {
			return outputType
		}
		return typ.MetaAny
	}())
	var enumHint typ.Type
	if enumType != nil {
		enumHint = enumType
	} else {
		enumHint = typ.MetaAny
	}
	expected := typ.TupleFromList([]typ.Type{inWrapped, outWrapped, enumHint, inputHint})
	ctx.CheckArgumentTypes(expected, true, false)

	// Set the expression type to the output type (or MetaError if unknown).
	if outputType != nil {
		ctx.SetType(outputType)
	} else {
		ctx.SetType(typ.MetaError)
	}
}

// GenerateCode returns false — cohort A: codegen falls back to the default
// "visit args + emit Command" path.
func (h *EnumCommandHandler) GenerateCode(ctx semantics.CodeGenContext) bool {
	return false
}
