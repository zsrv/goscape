package semantics

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestVisitClientScriptExpression_TriggerNotRegistered(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	// clientscriptTrigger is nil since fixture doesn't register it.
	cse := &ast.ClientScriptExpression{Name: &ast.Identifier{Text: "x"}}
	tc.Visit(cse)
	// MessageTriggerTypeNotFound: "Internal compiler error: The trigger '%s' has no declaration."
	// The "clientscript" arg ends up in MessageArgs, not Message itself.
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "trigger") || strings.Contains(d.Message, "declaration") {
			found = true
			break
		}
		for _, arg := range d.MessageArgs {
			if s, ok := arg.(string); ok && strings.Contains(s, "clientscript") {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Errorf("expected MessageTriggerTypeNotFound clientscript; got %v", tc.diagnostics.List())
	}
}

func TestVisitClientScriptExpression_HookHintRequired(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	// Inject a fake clientscript trigger pointer so the no-trigger branch
	// doesn't fire. The trigger pointer itself isn't dereferenced when
	// the hook-hint check fails — only after the lookup happens.
	tc.clientscriptTrigger = &trigger.TriggerType{Identifier: "clientscript"}
	cse := &ast.ClientScriptExpression{Name: &ast.Identifier{Text: "x"}}
	cse.TypeHint = typ.PrimitiveInt // wrong kind — not a Hook
	tc.Visit(cse)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "Hook") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected hook-hint-required diag; got %v", tc.diagnostics.List())
	}
}

func TestVisitClientScriptExpression_HookHint_HappyPath(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	tc.clientscriptTrigger = &trigger.TriggerType{Identifier: "clientscript"}
	cse := &ast.ClientScriptExpression{Name: &ast.Identifier{Text: "nope"}}
	cse.TypeHint = typ.NewMetaHook(typ.MetaUnit) // valid Hook hint with no transmit
	tc.Visit(cse)
	// Name "nope" is unresolved ⇒ expect MessageClientScriptReferenceUnresolved.
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "clientscript") || strings.Contains(d.Message, "cannot be resolved") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ClientScriptReferenceUnresolved; got %v", tc.diagnostics.List())
	}
}

func TestVisitClientScriptExpression_TransmitListWhenUnitHook(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	tc.clientscriptTrigger = &trigger.TriggerType{Identifier: "clientscript"}
	// Hook with Unit transmit list type but supply a transmit-list arg —
	// should emit MessageHookTransmitListUnexpected.
	cse := &ast.ClientScriptExpression{
		Name:         &ast.Identifier{Text: "x"},
		TransmitList: []ast.Expression{&ast.IntegerLiteral{Value: 0}},
	}
	cse.TypeHint = typ.NewMetaHook(typ.MetaUnit)
	tc.Visit(cse)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "Unexpected hook transmit list") || strings.Contains(d.Message, "transmit") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageHookTransmitListUnexpected; got %v", tc.diagnostics.List())
	}
}
