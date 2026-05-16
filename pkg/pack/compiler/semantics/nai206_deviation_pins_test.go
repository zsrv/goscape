package semantics

import (
	"os"
	"strings"
	"testing"
)

// readSrcRelative loads a project file using a path relative to the test
// binary's working directory (which is the semantics package dir).
func readSrcRelative(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(rel)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// TestNAI206_DeviationPins pins each NAI-206 deviation tag to the file
// where it must live. Renaming or relocating a tag should fail this
// test, prompting a deliberate audit instead of silent loss of
// design-rationale documentation.
//
// See plan docs/superpowers/plans/2026-05-15-nai-206-typechecking.md T19.
func TestNAI206_DeviationPins(t *testing.T) {
	tests := []struct {
		tag  string
		path string
	}{
		{"NAI-206-D-EXPR-BASE", "../ast/expression_base.go"},
		{"NAI-206-D-WALKER-OWNS-CONTEXT", "type_checking.go"},
		{"NAI-206-D-CONST-CACHE-AST", "type_checking.go"},
		{"NAI-206-D-TRIGGER-LOOKUPS-NILABLE", "type_checking.go"},
		{"NAI-206-D-CONST-PARSE", "../parser/parser.go"},
		{"NAI-206-D-DYNCOMMAND-EMPTY", "dynamic_command.go"},
		{"NAI-206-D-DYNCOMMAND-NO-CODEGEN", "dynamic_command.go"},
		{"NAI-206-D-CLIENTSCRIPT-NO-PANIC", "type_checking_expr.go"},
		{"NAI-206-D-CONST-PARSE-LOC", "type_checking_expr.go"},
		{"NAI-206-D-ARITH-RIGHT-NODE", "type_checking_expr.go"},
		{"NAI-206-D-BOOL-HINT-FIELD", "type_checking_expr.go"},
		{"NAI-206-D-TUPLE-SINGLE-SCALAR", "type_checking_test.go"},
	}
	for _, c := range tests {
		t.Run(c.tag, func(t *testing.T) {
			src := readSrcRelative(t, c.path)
			if !strings.Contains(src, c.tag) {
				t.Errorf("%s tag not found in %s", c.tag, c.path)
			}
		})
	}
}
