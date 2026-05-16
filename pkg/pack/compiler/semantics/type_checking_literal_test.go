package semantics

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestVisitIntegerLiteral_NoHintDefaultsToInt(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	il := &ast.IntegerLiteral{Value: 5}
	tc.Visit(il)
	gotType, _ := il.Type.(typ.Type)
	if gotType != typ.PrimitiveInt {
		t.Errorf("il.Type = %v, want PrimitiveInt", il.Type)
	}
}

func TestVisitIntegerLiteral_BooleanHint01(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	il := &ast.IntegerLiteral{Value: 1}
	il.TypeHint = typ.PrimitiveBoolean
	tc.Visit(il)
	gotType, _ := il.Type.(typ.Type)
	if gotType != typ.PrimitiveBoolean {
		t.Errorf("il.Type = %v, want PrimitiveBoolean", il.Type)
	}
}

func TestVisitCoordLiteral(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	cl := &ast.CoordLiteral{Value: 12345}
	tc.Visit(cl)
	gotType, _ := cl.Type.(typ.Type)
	if gotType != typ.PrimitiveCoord {
		t.Errorf("cl.Type = %v, want PrimitiveCoord", cl.Type)
	}
}

func TestVisitBooleanLiteral_FeatureDisabled(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	tc.features.DisableBooleans = true
	bl := &ast.BooleanLiteral{Value: true}
	tc.Visit(bl)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "Boolean") || strings.Contains(d.Message, "disabled") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageFeatureDisabledBoolean; got %v", tc.diagnostics.List())
	}
	gotType, _ := bl.Type.(typ.Type)
	if gotType != typ.MetaError {
		t.Errorf("bl.Type = %v, want MetaError", bl.Type)
	}
}

func TestVisitBooleanLiteral_Enabled(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	bl := &ast.BooleanLiteral{Value: true}
	tc.Visit(bl)
	gotType, _ := bl.Type.(typ.Type)
	if gotType != typ.PrimitiveBoolean {
		t.Errorf("bl.Type = %v, want PrimitiveBoolean", bl.Type)
	}
}

func TestVisitCharacterLiteral(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	cl := &ast.CharacterLiteral{Value: "a"}
	tc.Visit(cl)
	gotType, _ := cl.Type.(typ.Type)
	if gotType != typ.PrimitiveChar {
		t.Errorf("cl.Type = %v, want PrimitiveChar", cl.Type)
	}
}

func TestVisitNullLiteral_DefaultsToInt(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	nl := &ast.NullLiteral{}
	tc.Visit(nl)
	gotType, _ := nl.Type.(typ.Type)
	if gotType != typ.PrimitiveInt {
		t.Errorf("nl.Type = %v, want PrimitiveInt", nl.Type)
	}
}

func TestVisitNullLiteral_RespectsHint(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	nl := &ast.NullLiteral{}
	nl.TypeHint = typ.PrimitiveString
	tc.Visit(nl)
	gotType, _ := nl.Type.(typ.Type)
	if gotType != typ.PrimitiveString {
		t.Errorf("nl.Type = %v, want PrimitiveString", nl.Type)
	}
}

func TestVisitStringLiteral_NoHintIsString(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	sl := &ast.StringLiteral{Value: "hello"}
	tc.Visit(sl)
	gotType, _ := sl.Type.(typ.Type)
	if gotType != typ.PrimitiveString {
		t.Errorf("sl.Type = %v, want PrimitiveString", sl.Type)
	}
}

func TestVisitJoinedStringExpression_PropagatesToString(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	jse := &ast.JoinedStringExpression{Parts: []ast.StringPart{
		&ast.BasicStringPart{Value: "hi"},
	}}
	tc.Visit(jse)
	gotType, _ := jse.Type.(typ.Type)
	if gotType != typ.PrimitiveString {
		t.Errorf("jse.Type = %v, want PrimitiveString", jse.Type)
	}
}
