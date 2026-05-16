package semantics

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestVisitCommandCallExpression_Unresolved(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	cc := &ast.CommandCallExpression{Name: &ast.Identifier{Text: "nope"}}
	tc.Visit(cc)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "cannot be resolved") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected COMMAND_REFERENCE_UNRESOLVED; got %v", tc.diagnostics.List())
	}
	gotType, _ := cc.Type.(typ.Type)
	if gotType != typ.MetaError {
		t.Errorf("cc.Type = %v, want MetaError", cc.Type)
	}
}

func TestVisitProcCallExpression_ProcsDisabled(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	tc.features.DisableProcs = true
	pc := &ast.ProcCallExpression{Name: &ast.Identifier{Text: "x"}}
	tc.Visit(pc)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "is disabled") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageFeatureDisabledTrigger; got %v", tc.diagnostics.List())
	}
}

func TestVisitJumpCallExpression_LabelTriggerMissing(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	// labelTrigger is nil unless TriggerManager registers it; basic fixture
	// doesn't register it.
	jc := &ast.JumpCallExpression{Name: &ast.Identifier{Text: "x"}}
	tc.currentScript = &ast.Script{}
	tc.Visit(jc)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "not allowed") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected jump-not-allowed when labelTrigger is nil; got %v", tc.diagnostics.List())
	}
}
