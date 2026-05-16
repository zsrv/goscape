// pkg/pack/compiler/command/param_handler.go
package command

// ParamCommandHandler handles dynamic type-checking for param-lookup commands
// (e.g. loc_param, npc_param) that look up a typed value from a param config.
// Mirrors TS ParamCommandHandler.ts.
//
// The ctor receives the entity type (e.g. loc / npc) or nil for the general
// case.  The param argument must resolve to a param<T> type; the handler
// propagates T as the expression's return type.

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// paramAny is a param<any> sentinel used as the type hint when checking the
// param-reference argument. Mirrors TS ParamCommandHandler.PARAM_ANY.
var paramAny = typ.NewParamType(typ.MetaAny)

// ParamCommandHandler implements DynamicCommandHandler for param-lookup commands.
type ParamCommandHandler struct {
	// paramReturnType is the required entity type (e.g. loc / npc), or nil
	// for the general case. Mirrors TS ParamCommandHandler.type.
	paramReturnType typ.Type
}

// NewParamCommandHandler constructs a ParamCommandHandler. paramReturnType
// may be nil for general param lookups (no entity-type constraint).
func NewParamCommandHandler(paramReturnType typ.Type) *ParamCommandHandler {
	return &ParamCommandHandler{paramReturnType: paramReturnType}
}

// TypeCheck ports ParamCommandHandler.ts typeCheck verbatim.
func (h *ParamCommandHandler) TypeCheck(ctx *semantics.TypeCheckingContext) {
	expectedTypes := make([]typ.Type, 0, 2)

	// If an entity type was provided, check arg 0 with that hint.
	paramArgIndex := 0
	if h.paramReturnType != nil {
		expectedTypes = append(expectedTypes, h.paramReturnType)
		ctx.CheckArgument(0, h.paramReturnType, false)
		paramArgIndex = 1
	}

	// Check the param-reference argument with a param<any> hint.
	paramExpr := ctx.CheckArgument(paramArgIndex, paramAny, false)

	// Extract the param's inner return type.
	var paramReturnType typ.Type
	if paramExpr != nil {
		if t := semantics.ExprType(paramExpr); t != nil {
			if inner, ok := typ.IsParamType(t); ok {
				paramReturnType = inner
			}
		}
	}

	// Build the expected type list: [entityType?, param<T>].
	innerForExpected := typ.MetaAny
	if paramReturnType != nil {
		innerForExpected = paramReturnType
	}
	expectedTypes = append(expectedTypes, typ.NewParamType(innerForExpected))

	if !ctx.CheckArgumentTypes(typ.TupleFromList(expectedTypes), true, false) {
		ctx.SetType(typ.MetaError)
		return
	}

	if paramReturnType == nil {
		ctx.SetType(typ.MetaError)
		return
	}

	ctx.SetType(paramReturnType)
}

// GenerateCode returns false — cohort A: codegen falls back to the default
// "visit args + emit Command" path.
func (h *ParamCommandHandler) GenerateCode(ctx semantics.CodeGenContext) bool {
	return false
}
