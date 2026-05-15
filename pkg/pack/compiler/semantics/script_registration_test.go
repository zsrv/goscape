// pkg/pack/compiler/semantics/script_registration_test.go
package semantics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// makeProcTrigger constructs a minimal proc trigger that allows
// parameters + returns and uses ModeName subject mode.
func makeProcTrigger() *trigger.TriggerType {
	return &trigger.TriggerType{
		ID:              0,
		Identifier:      "proc",
		SubjectMode:     trigger.ModeName,
		AllowParameters: true,
		Parameters:      nil,
		AllowReturns:    true,
		Returns:         nil,
	}
}

// makeLabelTrigger constructs a minimal label trigger that allows
// parameters but NOT returns.
func makeLabelTrigger() *trigger.TriggerType {
	return &trigger.TriggerType{
		ID:              1,
		Identifier:      "label",
		SubjectMode:     trigger.ModeName,
		AllowParameters: true,
		Parameters:      nil,
		AllowReturns:    false,
		Returns:         nil,
	}
}

// scriptFor builds an *ast.Script with the named trigger/name and no params/returns.
func scriptFor(trig, name string) *ast.Script {
	loc := lexer.NodeSourceLocation{Name: "<test>", Line: 1}
	return &ast.Script{
		SrcLoc:  loc,
		Trigger: &ast.Identifier{SrcLoc: loc, Text: trig},
		Name:    &ast.Identifier{SrcLoc: loc, Text: name},
	}
}

func TestVisitScript_TriggerInvalid_ReportsError(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("nope", "foo")

	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if s.TriggerType != nil {
		t.Fatalf("TriggerType = %v, want nil (lookup failed)", s.TriggerType)
	}
	if !d.HasErrors() {
		t.Fatal("HasErrors = false, want true (SCRIPT_TRIGGER_INVALID)")
	}
	list := d.List()
	found := false
	for _, e := range list {
		if e.Message == diagnostics.MessageScriptTriggerInvalid {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_TRIGGER_INVALID diagnostic; got %+v", list)
	}
}

func TestVisitScript_HappyPath_RegistersSymbol(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	if err := trm.RegisterTrigger(proc); err != nil {
		t.Fatal(err)
	}

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("proc", "foo")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if len(d.List()) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", d.List())
	}
	gotTrig, ok := s.TriggerType.(*trigger.TriggerType)
	if !ok || gotTrig != proc {
		t.Fatalf("TriggerType = %v (concrete %T), want proc", s.TriggerType, s.TriggerType)
	}
	if s.Symbol == nil {
		t.Fatal("Symbol nil, want ServerScriptSymbol")
	}
	if s.Block == nil {
		t.Fatal("Block nil, want SymbolTable")
	}
	if s.ParameterType == nil {
		t.Fatal("ParameterType nil")
	}
	if s.ReturnType == nil {
		t.Fatal("ReturnType nil")
	}
	// Root table contains the ServerScriptSymbol under (server:proc, "foo").
	got := root.Find(symbol.SymbolTypeServerScript(proc), "foo")
	if got == nil {
		t.Fatal("root table missing ServerScriptSymbol after register")
	}
}

func TestVisitScript_StarOnNonCommand_ReportsError(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("proc", "foo")
	s.IsStar = true
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptCommandOnly {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_COMMAND_ONLY diagnostic; got %+v", d.List())
	}
}

func TestVisitScript_Redeclaration(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	a := scriptFor("proc", "foo")
	b := scriptFor("proc", "foo")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{a, b}})

	// First one set Symbol; second one didn't.
	if a.Symbol == nil {
		t.Fatal("first Script.Symbol nil")
	}
	if b.Symbol != nil {
		t.Fatal("second Script.Symbol non-nil; want nil after redeclaration")
	}

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptRedeclaration {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_REDECLARATION diagnostic; got %+v", d.List())
	}
}

func TestVisitScript_ReturnTokens_Resolved(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)
	_ = tm.RegisterByRepresentation(typ.PrimitiveString)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	loc := lexer.NodeSourceLocation{Name: "<test>", Line: 1}
	s := scriptFor("proc", "foo")
	s.ReturnTokens = []*ast.Token{
		{SrcLoc: loc, Text: "int"},
		{SrcLoc: loc, Text: "string"},
	}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if len(d.List()) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", d.List())
	}
	// Two return tokens → TupleType
	if s.ReturnType == nil {
		t.Fatal("ReturnType nil")
	}
	tup, ok := s.ReturnType.(*typ.TupleType)
	if !ok {
		t.Fatalf("ReturnType = %T, want *typ.TupleType", s.ReturnType)
	}
	if len(tup.Children) != 2 || tup.Children[0] != typ.PrimitiveInt || tup.Children[1] != typ.PrimitiveString {
		t.Fatalf("ReturnType children = %v, want [int, string]", tup.Children)
	}
}

func TestVisitScript_NoReturnTokens_AllowReturns_DefaultsUnit(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger() // AllowReturns=true
	_ = trm.RegisterTrigger(proc)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("proc", "foo")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if s.ReturnType != typ.MetaUnit {
		t.Fatalf("ReturnType = %v, want MetaUnit (proc allows returns; no tokens)", s.ReturnType)
	}
}

func TestVisitScript_NoReturnTokens_DisallowReturns_DefaultsNothing(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	label := makeLabelTrigger() // AllowReturns=false
	_ = trm.RegisterTrigger(label)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("label", "foo")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if s.ReturnType != typ.MetaNothing {
		t.Fatalf("ReturnType = %v, want MetaNothing (label disallows returns)", s.ReturnType)
	}
}

func TestVisitScript_BadReturnType_EmitsInvalidType(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)
	// TypeManager has nothing registered → return token "int" misses.

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	loc := lexer.NodeSourceLocation{Name: "<test>", Line: 1}
	s := scriptFor("proc", "foo")
	s.ReturnTokens = []*ast.Token{{SrcLoc: loc, Text: "int"}}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageGenericInvalidType {
			found = true
		}
	}
	if !found {
		t.Fatalf("no GENERIC_INVALID_TYPE diagnostic; got %+v", d.List())
	}
}
