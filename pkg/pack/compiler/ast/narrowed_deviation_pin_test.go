package ast

import (
	"os"
	"strings"
	"testing"
)

// TestPin_NarrowedNAI204DAstNoTypeFields pins that the
// NAI-204-D-AST-NO-TYPE-FIELDS doc-comment in scriptfile.go has been narrowed
// to mention NAI-206 explicitly. Removing the NAI-206 mention regresses the
// "scope of remaining deferral" contract that NAI-205 establishes.
func TestPin_NarrowedNAI204DAstNoTypeFields(t *testing.T) {
	b, err := os.ReadFile("scriptfile.go")
	if err != nil {
		t.Fatalf("read scriptfile.go: %v", err)
	}
	src := string(b)
	if !strings.Contains(src, "NAI-204-D-AST-NO-TYPE-FIELDS") {
		t.Fatal("scriptfile.go missing NAI-204-D-AST-NO-TYPE-FIELDS tag")
	}
	if !strings.Contains(src, "NAI-206") {
		t.Fatal("NAI-204-D-AST-NO-TYPE-FIELDS tag does not mention NAI-206 — the deviation should now reference its retirement slice")
	}
}

func TestScript_NewFieldsExist(t *testing.T) {
	// Compile-only structural check: the seven new fields must be addressable
	// at the Script and Parameter types. Tests will set them via test setters
	// in T9-T13.
	s := &Script{}
	_ = s.TriggerType
	_ = s.Symbol
	_ = s.Block
	_ = s.ParameterType
	_ = s.ReturnType
	_ = s.SubjectReference

	p := &Parameter{}
	_ = p.Symbol
}
