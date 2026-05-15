// pkg/pack/compiler/semantics/parameter_test.go
package semantics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func paramFor(typeText, name string) *ast.Parameter {
	loc := lexer.NodeSourceLocation{Name: "<test>", Line: 1}
	return &ast.Parameter{
		SrcLoc:    loc,
		TypeToken: &ast.Token{SrcLoc: loc, Text: typeText},
		Name:      &ast.Identifier{SrcLoc: loc, Text: name},
	}
}

func TestVisitParameter_RegistersLocal(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("proc", "foo")
	s.Parameters = []*ast.Parameter{paramFor("int", "x")}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if d.HasErrors() {
		t.Fatalf("unexpected errors: %+v", d.List())
	}
	if s.Parameters[0].Symbol == nil {
		t.Fatal("Parameter.Symbol nil")
	}
	loc, ok := s.Parameters[0].Symbol.(*symbol.LocalVariableSymbol)
	if !ok {
		t.Fatalf("Symbol = %T, want *LocalVariableSymbol", s.Parameters[0].Symbol)
	}
	if loc.Name != "x" || loc.Type != typ.PrimitiveInt {
		t.Fatalf("LocalVariableSymbol = %+v", loc)
	}
}

func TestVisitParameter_DuplicateName_EmitsLocalRedeclaration(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("proc", "foo")
	s.Parameters = []*ast.Parameter{paramFor("int", "x"), paramFor("int", "x")}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptLocalRedeclaration {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_LOCAL_REDECLARATION: %+v", d.List())
	}
}

func TestVisitParameter_InvalidType_EmitsGenericInvalidType(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)
	// TypeManager has nothing registered.

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("proc", "foo")
	s.Parameters = []*ast.Parameter{paramFor("nope", "x")}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageGenericInvalidType {
			found = true
		}
	}
	if !found {
		t.Fatalf("no GENERIC_INVALID_TYPE: %+v", d.List())
	}
}

func TestVisitParameter_FeatureDisabledType_EmitsFeatureDisabledType(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)
	_ = tm.RegisterByRepresentation(typ.PrimitiveBoolean)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{DisableBooleans: true})
	s := scriptFor("proc", "foo")
	s.Parameters = []*ast.Parameter{paramFor("boolean", "x")}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageFeatureDisabledType {
			found = true
		}
	}
	if !found {
		t.Fatalf("no FEATURE_DISABLED_TYPE: %+v", d.List())
	}
}

func TestVisitParameter_ProcsDisabled_NonCommand_EmitsFeatureDisabledLocal(t *testing.T) {
	// Per TS L420-422: when features.procs===false AND triggerType !== CommandTrigger,
	// any parameter on the script emits FEATURE_DISABLED_LOCAL.
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{DisableProcs: true})
	s := scriptFor("proc", "foo")
	s.Parameters = []*ast.Parameter{paramFor("int", "x")}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageFeatureDisabledLocal {
			found = true
		}
	}
	if !found {
		t.Fatalf("no FEATURE_DISABLED_LOCAL: %+v", d.List())
	}
}
