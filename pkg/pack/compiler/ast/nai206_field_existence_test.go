package ast

import (
	"reflect"
	"testing"
)

// TestNAI206_DeferredFieldsExist pins the 13 NAI-206-owned fields onto
// their concrete AST types. If any field is dropped during refactor
// the test fails before walker code can silently regress.
//
// See spec §5 — NAI-206 design doc.
func TestNAI206_DeferredFieldsExist(t *testing.T) {
	cases := []struct {
		name      string
		instance  interface{}
		fieldName string
		wantKind  reflect.Kind
	}{
		// Expression.Type / TypeHint via ExpressionBase mixin
		{"ParenthesizedExpression.Type", &ParenthesizedExpression{}, "Type", reflect.Interface},
		{"ParenthesizedExpression.TypeHint", &ParenthesizedExpression{}, "TypeHint", reflect.Interface},
		{"ArithmeticExpression.Type", &ArithmeticExpression{}, "Type", reflect.Interface},
		{"CalcExpression.Type", &CalcExpression{}, "Type", reflect.Interface},
		{"ConditionExpression.Type", &ConditionExpression{}, "Type", reflect.Interface},
		{"IntegerLiteral.Type", &IntegerLiteral{}, "Type", reflect.Interface},
		{"StringLiteral.Type", &StringLiteral{}, "Type", reflect.Interface},
		{"Identifier.Type", &Identifier{}, "Type", reflect.Interface},
		{"LocalVariableExpression.Type", &LocalVariableExpression{}, "Type", reflect.Interface},
		{"GameVariableExpression.Type", &GameVariableExpression{}, "Type", reflect.Interface},
		{"ConstantVariableExpression.Type", &ConstantVariableExpression{}, "Type", reflect.Interface},
		{"CommandCallExpression.Type", &CommandCallExpression{}, "Type", reflect.Interface},
		{"ProcCallExpression.Type", &ProcCallExpression{}, "Type", reflect.Interface},
		{"JumpCallExpression.Type", &JumpCallExpression{}, "Type", reflect.Interface},
		{"ClientScriptExpression.Type", &ClientScriptExpression{}, "Type", reflect.Interface},
		{"JoinedStringExpression.Type", &JoinedStringExpression{}, "Type", reflect.Interface},

		// Reference fields
		{"Identifier.Reference", &Identifier{}, "Reference", reflect.Interface},
		{"IntegerLiteral.Reference", &IntegerLiteral{}, "Reference", reflect.Interface},
		{"StringLiteral.Reference", &StringLiteral{}, "Reference", reflect.Interface},
		{"LocalVariableExpression.Reference", &LocalVariableExpression{}, "Reference", reflect.Interface},
		{"GameVariableExpression.Reference", &GameVariableExpression{}, "Reference", reflect.Interface},

		// SubExpression
		{"ConstantVariableExpression.SubExpression", &ConstantVariableExpression{}, "SubExpression", reflect.Interface},
		{"StringLiteral.SubExpression", &StringLiteral{}, "SubExpression", reflect.Interface},

		// Symbol on call + decl
		{"CommandCallExpression.Symbol", &CommandCallExpression{}, "Symbol", reflect.Interface},
		{"ProcCallExpression.Symbol", &ProcCallExpression{}, "Symbol", reflect.Interface},
		{"JumpCallExpression.Symbol", &JumpCallExpression{}, "Symbol", reflect.Interface},
		{"ClientScriptExpression.Symbol", &ClientScriptExpression{}, "Symbol", reflect.Interface},
		{"DeclarationStatement.Symbol", &DeclarationStatement{}, "Symbol", reflect.Interface},
		{"ArrayDeclarationStatement.Symbol", &ArrayDeclarationStatement{}, "Symbol", reflect.Interface},

		// SwitchStatement
		{"SwitchStatement.DefaultCase", &SwitchStatement{}, "DefaultCase", reflect.Ptr},
		{"SwitchStatement.Type", &SwitchStatement{}, "Type", reflect.Interface},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := reflect.ValueOf(c.instance).Elem()
			f := v.FieldByName(c.fieldName)
			if !f.IsValid() {
				t.Fatalf("field %s not found on %T", c.fieldName, c.instance)
			}
			if f.Kind() != c.wantKind {
				t.Fatalf("field %s on %T: kind=%v, want %v", c.fieldName, c.instance, f.Kind(), c.wantKind)
			}
		})
	}
}
