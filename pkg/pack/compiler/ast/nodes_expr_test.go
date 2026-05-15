package ast

import "testing"

func TestParenthesizedExpression_Kind(t *testing.T) {
	p := &ParenthesizedExpression{
		SrcLoc:     loc("<test>", 1, 1, 1, 5),
		Expression: &Identifier{SrcLoc: loc("<test>", 1, 2, 1, 4), Text: "foo"},
	}
	if p.Kind() != KindParenthesizedExpression {
		t.Fatalf("Kind = %v", p.Kind())
	}
	if got, want := len(p.Children()), 1; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
}

func TestIntegerLiteral_KindAndValue(t *testing.T) {
	il := &IntegerLiteral{SrcLoc: loc("<test>", 1, 1, 1, 3), Value: 42}
	if il.Kind() != KindIntegerLiteral {
		t.Fatalf("Kind = %v", il.Kind())
	}
	if il.Value != 42 {
		t.Fatalf("Value = %d, want 42", il.Value)
	}
}

func TestCoordLiteral_Kind(t *testing.T) {
	cl := &CoordLiteral{SrcLoc: loc("<test>", 1, 1, 1, 9), Value: 0}
	if cl.Kind() != KindCoordLiteral {
		t.Fatalf("Kind = %v", cl.Kind())
	}
}

func TestBooleanLiteral_Value(t *testing.T) {
	bl := &BooleanLiteral{SrcLoc: loc("<test>", 1, 1, 1, 4), Value: true}
	if bl.Kind() != KindBooleanLiteral {
		t.Fatalf("Kind = %v", bl.Kind())
	}
	if !bl.Value {
		t.Fatal("Value should be true")
	}
}

func TestCharacterLiteral_Value(t *testing.T) {
	cl := &CharacterLiteral{SrcLoc: loc("<test>", 1, 1, 1, 3), Value: "x"}
	if cl.Kind() != KindCharacterLiteral {
		t.Fatalf("Kind = %v", cl.Kind())
	}
	if cl.Value != "x" {
		t.Fatalf("Value = %q, want %q", cl.Value, "x")
	}
}

func TestStringLiteral_Value(t *testing.T) {
	sl := &StringLiteral{SrcLoc: loc("<test>", 1, 1, 1, 7), Value: "hello"}
	if sl.Kind() != KindStringLiteral {
		t.Fatalf("Kind = %v", sl.Kind())
	}
}

func TestNullLiteral_ValueIsNegativeOne(t *testing.T) {
	nl := &NullLiteral{SrcLoc: loc("<test>", 1, 1, 1, 4)}
	if nl.Kind() != KindNullLiteral {
		t.Fatalf("Kind = %v", nl.Kind())
	}
	if nl.Value() != -1 {
		t.Fatalf("Value() = %d, want -1", nl.Value())
	}
}

func TestLocalVariableExpression_IsArrayWhenIndexSet(t *testing.T) {
	plain := &LocalVariableExpression{
		SrcLoc: loc("<test>", 1, 1, 1, 5),
		Name:   &Identifier{SrcLoc: loc("<test>", 1, 2, 1, 5), Text: "var"},
	}
	if plain.IsArray() {
		t.Fatal("plain $var should not be array")
	}
	if got, want := len(plain.Children()), 1; got != want {
		t.Fatalf("plain Children = %d, want %d", got, want)
	}

	arr := &LocalVariableExpression{
		SrcLoc: loc("<test>", 1, 1, 1, 10),
		Name:   &Identifier{SrcLoc: loc("<test>", 1, 2, 1, 5), Text: "arr"},
		Index:  &Identifier{SrcLoc: loc("<test>", 1, 7, 1, 7), Text: "0"},
	}
	if !arr.IsArray() {
		t.Fatal("$arr(0) should be array")
	}
	if got, want := len(arr.Children()), 2; got != want {
		t.Fatalf("array Children = %d, want %d", got, want)
	}
	if arr.Kind() != KindLocalVariableExpression {
		t.Fatalf("Kind = %v", arr.Kind())
	}
}

func TestGameVariableExpression_DotMod(t *testing.T) {
	g := &GameVariableExpression{
		SrcLoc: loc("<test>", 1, 1, 1, 5),
		Dot:    true,
		Name:   &Identifier{SrcLoc: loc("<test>", 1, 3, 1, 5), Text: "var"},
	}
	if g.Kind() != KindGameVariableExpression {
		t.Fatalf("Kind = %v", g.Kind())
	}
	if !g.Dot {
		t.Fatal("Dot should be true for dot-percent-var")
	}
}

func TestConstantVariableExpression_Kind(t *testing.T) {
	c := &ConstantVariableExpression{
		SrcLoc: loc("<test>", 1, 1, 1, 4),
		Name:   &Identifier{SrcLoc: loc("<test>", 1, 2, 1, 4), Text: "max"},
	}
	if c.Kind() != KindConstantVariableExpression {
		t.Fatalf("Kind = %v", c.Kind())
	}
}

func TestCommandCallExpression_IsStarAndNameString(t *testing.T) {
	plain := &CommandCallExpression{
		SrcLoc:    loc("<test>", 1, 1, 1, 10),
		Name:      &Identifier{SrcLoc: loc("<test>", 1, 1, 1, 3), Text: "mes"},
		Arguments: []Expression{&Identifier{SrcLoc: loc("<test>", 1, 5, 1, 8), Text: "x"}},
	}
	if plain.IsStar() {
		t.Fatal("plain command should not be star")
	}
	if got, want := plain.NameString(), "mes"; got != want {
		t.Fatalf("NameString = %q, want %q", got, want)
	}

	star := &CommandCallExpression{
		SrcLoc:     loc("<test>", 1, 1, 1, 15),
		Name:       &Identifier{SrcLoc: loc("<test>", 1, 1, 1, 5), Text: "abc"},
		Arguments:  []Expression{},
		Arguments2: []Expression{},
	}
	if !star.IsStar() {
		t.Fatal("Arguments2 non-nil should be star")
	}
	if got, want := star.NameString(), "abc*"; got != want {
		t.Fatalf("NameString = %q, want %q", got, want)
	}
}

func TestProcCallExpression_Kind(t *testing.T) {
	p := &ProcCallExpression{
		SrcLoc: loc("<test>", 1, 1, 1, 10),
		Name:   &Identifier{SrcLoc: loc("<test>", 1, 2, 1, 5), Text: "foo"},
	}
	if p.Kind() != KindProcCallExpression {
		t.Fatalf("Kind = %v", p.Kind())
	}
}

func TestJumpCallExpression_Kind(t *testing.T) {
	j := &JumpCallExpression{
		SrcLoc: loc("<test>", 1, 1, 1, 10),
		Name:   &Identifier{SrcLoc: loc("<test>", 1, 2, 1, 5), Text: "lbl"},
	}
	if j.Kind() != KindJumpCallExpression {
		t.Fatalf("Kind = %v", j.Kind())
	}
}

func TestClientScriptExpression_Kind(t *testing.T) {
	c := &ClientScriptExpression{
		SrcLoc:       loc("<test>", 1, 1, 1, 20),
		Name:         &Identifier{SrcLoc: loc("<test>", 1, 1, 1, 5), Text: "hdlr"},
		Arguments:    []Expression{},
		TransmitList: []Expression{},
	}
	if c.Kind() != KindClientScriptExpression {
		t.Fatalf("Kind = %v", c.Kind())
	}
}

func TestArithmeticExpression_Kind(t *testing.T) {
	a := &ArithmeticExpression{
		SrcLoc:   loc("<test>", 1, 1, 1, 7),
		Left:     &Identifier{SrcLoc: loc("<test>", 1, 1, 1, 1), Text: "1"},
		Operator: &Token{SrcLoc: loc("<test>", 1, 3, 1, 3), Text: "+"},
		Right:    &Identifier{SrcLoc: loc("<test>", 1, 5, 1, 5), Text: "2"},
	}
	if a.Kind() != KindArithmeticExpression {
		t.Fatalf("Kind = %v", a.Kind())
	}
	if got, want := len(a.Children()), 3; got != want {
		t.Fatalf("len(Children) = %d, want %d", got, want)
	}
}

func TestCalcExpression_Kind(t *testing.T) {
	c := &CalcExpression{
		SrcLoc:     loc("<test>", 1, 1, 1, 8),
		Expression: &Identifier{SrcLoc: loc("<test>", 1, 6, 1, 6), Text: "x"},
	}
	if c.Kind() != KindCalcExpression {
		t.Fatalf("Kind = %v", c.Kind())
	}
}

func TestConditionExpression_Kind(t *testing.T) {
	c := &ConditionExpression{
		SrcLoc:   loc("<test>", 1, 1, 1, 7),
		Left:     &Identifier{SrcLoc: loc("<test>", 1, 1, 1, 1), Text: "a"},
		Operator: &Token{SrcLoc: loc("<test>", 1, 3, 1, 3), Text: "="},
		Right:    &Identifier{SrcLoc: loc("<test>", 1, 5, 1, 5), Text: "b"},
	}
	if c.Kind() != KindConditionExpression {
		t.Fatalf("Kind = %v", c.Kind())
	}
}

func TestJoinedStringExpression_PartKinds(t *testing.T) {
	js := &JoinedStringExpression{
		SrcLoc: loc("<test>", 1, 1, 1, 20),
		Parts: []StringPart{
			&BasicStringPart{SrcLoc: loc("<test>", 1, 2, 1, 5), Value: "hi"},
			&PTagStringPart{SrcLoc: loc("<test>", 1, 6, 1, 12), Value: "<p,head>"},
			&ExpressionStringPart{SrcLoc: loc("<test>", 1, 13, 1, 18),
				Expression: &Identifier{SrcLoc: loc("<test>", 1, 14, 1, 17), Text: "x"}},
		},
	}
	if js.Kind() != KindJoinedStringExpression {
		t.Fatalf("Kind = %v", js.Kind())
	}
	if got, want := js.Parts[0].Kind(), KindBasicStringPart; got != want {
		t.Fatalf("Parts[0].Kind = %v, want %v", got, want)
	}
	if got, want := js.Parts[1].Kind(), KindPTagStringPart; got != want {
		t.Fatalf("Parts[1].Kind = %v, want %v", got, want)
	}
	if got, want := js.Parts[2].Kind(), KindExpressionStringPart; got != want {
		t.Fatalf("Parts[2].Kind = %v, want %v", got, want)
	}
	if got, want := len(js.Children()), 3; got != want {
		t.Fatalf("len(Children) = %d, want %d", got, want)
	}
}
