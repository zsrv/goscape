// pkg/pack/compiler/semantics/trigger_check_test.go
package semantics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestCheckParameters_TriggerDisallowsParameters_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	// Build a trigger that disallows parameters.
	t1 := makeProcTrigger()
	t1.AllowParameters = false
	t1.Identifier = "nopars"
	_ = trm.RegisterTrigger(t1)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("nopars", "foo")
	s.Parameters = []*ast.Parameter{paramFor("int", "x")}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptTriggerNoParameters {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_TRIGGER_NO_PARAMETERS: %+v", d.List())
	}
}

func TestCheckParameters_TriggerExpectsParameters_TypeMismatch_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)

	// Trigger expects (string) params; script provides (int).
	t1 := makeProcTrigger()
	t1.Identifier = "needsstr"
	t1.Parameters = typ.PrimitiveString // single-type tuple
	_ = trm.RegisterTrigger(t1)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("needsstr", "foo")
	s.Parameters = []*ast.Parameter{paramFor("int", "x")}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptTriggerExpectedParameters {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_TRIGGER_EXPECTED_PARAMETERS: %+v", d.List())
	}
}

func TestCheckReturns_TriggerDisallowsReturns_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)

	// label trigger disallows returns.
	t1 := makeLabelTrigger()
	_ = trm.RegisterTrigger(t1)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("label", "foo")
	loc := s.SrcLoc
	s.ReturnTokens = []*ast.Token{{SrcLoc: loc, Text: "int"}}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptTriggerNoReturns {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_TRIGGER_NO_RETURNS: %+v", d.List())
	}
}

func TestCheckReturns_TriggerExpectsSpecificReturns_Mismatch_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)
	_ = tm.RegisterByRepresentation(typ.PrimitiveString)

	t1 := makeProcTrigger()
	t1.Identifier = "needsint"
	t1.Returns = typ.PrimitiveInt
	_ = trm.RegisterTrigger(t1)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("needsint", "foo")
	loc := s.SrcLoc
	s.ReturnTokens = []*ast.Token{{SrcLoc: loc, Text: "string"}}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptTriggerExpectedReturns {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_TRIGGER_EXPECTED_RETURNS: %+v", d.List())
	}
}
