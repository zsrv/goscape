// pkg/pack/compiler/diagnostics/nai207_codegen_pin_test.go
package diagnostics

import "testing"

// TestNAI207_CodegenDiagnosticTemplatesShipped pins the 4 code-gen
// internal-compiler-error templates that NAI-207 T2 originally planned to add.
// Pre-flight confirmed all 4 were already shipped in messages.go with
// TS-verbatim format strings at HEAD (NAI-205 landed them early).
// T2 is therefore a no-op task; this test serves as a regression guard so
// any accidental drift in the template text surfaces immediately during the
// NAI-207 codegen implementation.
func TestNAI207_CodegenDiagnosticTemplatesShipped(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"MessageTypeHasNoBaseType", MessageTypeHasNoBaseType, "Internal compiler error: Type has no defined base type: %s."},
		{"MessageInvalidCondition", MessageInvalidCondition, "Internal compiler error: %s is not a supported expression type for conditions."},
		{"MessageNullConstant", MessageNullConstant, "Internal compiler error: %s evaluated to 'null' constant value."},
		{"MessageExpressionNoSubExpr", MessageExpressionNoSubExpr, "Internal compiler error: No sub expression node."},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, c.got, c.want)
		}
	}
}
