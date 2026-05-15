package ast

import "testing"

func TestBlockStatement_KindAndChildren(t *testing.T) {
	b := &BlockStatement{
		SrcLoc: loc("<test>", 1, 1, 3, 1),
		Statements: []Statement{
			&EmptyStatement{SrcLoc: loc("<test>", 2, 1, 2, 1)},
			&EmptyStatement{SrcLoc: loc("<test>", 2, 3, 2, 3)},
		},
	}
	if b.Kind() != KindBlockStatement {
		t.Fatalf("Kind = %v, want KindBlockStatement", b.Kind())
	}
	if got, want := len(b.Children()), 2; got != want {
		t.Fatalf("len(Children) = %d, want %d", got, want)
	}
}

func TestEmptyStatement_Kind(t *testing.T) {
	e := &EmptyStatement{SrcLoc: loc("<test>", 1, 1, 1, 1)}
	if e.Kind() != KindEmptyStatement {
		t.Fatalf("Kind = %v, want KindEmptyStatement", e.Kind())
	}
	if got := e.Children(); got != nil {
		t.Fatalf("Children = %v, want nil", got)
	}
}

func TestReturnStatement_KindAndChildren(t *testing.T) {
	r := &ReturnStatement{
		SrcLoc:      loc("<test>", 1, 1, 1, 10),
		Expressions: []Expression{&Identifier{SrcLoc: loc("<test>", 1, 8, 1, 8), Text: "x"}},
	}
	if r.Kind() != KindReturnStatement {
		t.Fatalf("Kind = %v", r.Kind())
	}
	if got, want := len(r.Children()), 1; got != want {
		t.Fatalf("len(Children) = %d, want %d", got, want)
	}
}

func TestIfStatement_ChildrenIncludesElseWhenSet(t *testing.T) {
	cond := &Identifier{SrcLoc: loc("<test>", 1, 4, 1, 8), Text: "true"}
	then := &EmptyStatement{SrcLoc: loc("<test>", 1, 11, 1, 11)}
	els := &EmptyStatement{SrcLoc: loc("<test>", 1, 18, 1, 18)}

	withElse := &IfStatement{SrcLoc: loc("<test>", 1, 1, 1, 18), Condition: cond, ThenStatement: then, ElseStatement: els}
	if got, want := len(withElse.Children()), 3; got != want {
		t.Fatalf("with else: len = %d, want %d", got, want)
	}

	withoutElse := &IfStatement{SrcLoc: loc("<test>", 1, 1, 1, 11), Condition: cond, ThenStatement: then}
	if got, want := len(withoutElse.Children()), 2; got != want {
		t.Fatalf("without else: len = %d, want %d", got, want)
	}
	if withoutElse.Kind() != KindIfStatement {
		t.Fatalf("Kind = %v", withoutElse.Kind())
	}
}

func TestWhileStatement_KindAndChildren(t *testing.T) {
	w := &WhileStatement{
		SrcLoc:        loc("<test>", 1, 1, 1, 20),
		Condition:     &Identifier{SrcLoc: loc("<test>", 1, 7, 1, 11), Text: "true"},
		ThenStatement: &EmptyStatement{SrcLoc: loc("<test>", 1, 14, 1, 14)},
	}
	if w.Kind() != KindWhileStatement {
		t.Fatalf("Kind = %v", w.Kind())
	}
	if got, want := len(w.Children()), 2; got != want {
		t.Fatalf("len(Children) = %d, want %d", got, want)
	}
}

func TestSwitchStatement_AndCase(t *testing.T) {
	c := &SwitchCase{
		SrcLoc: loc("<test>", 2, 1, 2, 10),
		Keys: []Expression{
			&Identifier{SrcLoc: loc("<test>", 2, 6, 2, 6), Text: "x"},
		},
		Statements: []Statement{&EmptyStatement{SrcLoc: loc("<test>", 2, 10, 2, 10)}},
	}
	if got, want := c.IsDefault(), false; got != want {
		t.Fatalf("IsDefault = %v, want %v", got, want)
	}
	d := &SwitchCase{SrcLoc: loc("<test>", 3, 1, 3, 10), Keys: nil}
	if !d.IsDefault() {
		t.Fatal("IsDefault: empty Keys should be default")
	}

	s := &SwitchStatement{
		SrcLoc:    loc("<test>", 1, 1, 4, 1),
		TypeToken: &Token{SrcLoc: loc("<test>", 1, 1, 1, 10), Text: "switch_int"},
		Condition: &Identifier{SrcLoc: loc("<test>", 1, 12, 1, 12), Text: "x"},
		Cases:     []*SwitchCase{c, d},
	}
	if s.Kind() != KindSwitchStatement {
		t.Fatalf("Kind = %v", s.Kind())
	}
	if got, want := len(s.Children()), 4; got != want { // TypeToken + Condition + 2 cases
		t.Fatalf("len(Children) = %d, want %d", got, want)
	}
}

func TestDeclarationStatement_ChildrenIncludesInitWhenSet(t *testing.T) {
	d := &DeclarationStatement{
		SrcLoc:      loc("<test>", 1, 1, 1, 18),
		TypeToken:   &Token{SrcLoc: loc("<test>", 1, 1, 1, 7), Text: "def_int"},
		Name:        &Identifier{SrcLoc: loc("<test>", 1, 10, 1, 12), Text: "var"},
		Initializer: &Identifier{SrcLoc: loc("<test>", 1, 16, 1, 16), Text: "0"},
	}
	if d.Kind() != KindDeclarationStatement {
		t.Fatalf("Kind = %v", d.Kind())
	}
	if got, want := len(d.Children()), 3; got != want {
		t.Fatalf("with init: len = %d, want %d", got, want)
	}
	d2 := &DeclarationStatement{
		SrcLoc:    loc("<test>", 1, 1, 1, 14),
		TypeToken: &Token{SrcLoc: loc("<test>", 1, 1, 1, 7), Text: "def_int"},
		Name:      &Identifier{SrcLoc: loc("<test>", 1, 10, 1, 12), Text: "var"},
	}
	if got, want := len(d2.Children()), 2; got != want {
		t.Fatalf("without init: len = %d, want %d", got, want)
	}
}

func TestArrayDeclarationStatement_Kind(t *testing.T) {
	a := &ArrayDeclarationStatement{
		SrcLoc:      loc("<test>", 1, 1, 1, 20),
		TypeToken:   &Token{SrcLoc: loc("<test>", 1, 1, 1, 7), Text: "def_int"},
		Name:        &Identifier{SrcLoc: loc("<test>", 1, 10, 1, 13), Text: "ints"},
		Initializer: &Identifier{SrcLoc: loc("<test>", 1, 16, 1, 17), Text: "50"},
	}
	if a.Kind() != KindArrayDeclarationStatement {
		t.Fatalf("Kind = %v", a.Kind())
	}
	if got, want := len(a.Children()), 3; got != want {
		t.Fatalf("len(Children) = %d, want %d", got, want)
	}
}

func TestAssignmentStatement_Kind(t *testing.T) {
	a := &AssignmentStatement{
		SrcLoc:      loc("<test>", 1, 1, 1, 10),
		Vars:        nil, // any VariableExpression slice; nil tolerated for shape test
		Expressions: []Expression{&Identifier{SrcLoc: loc("<test>", 1, 8, 1, 8), Text: "0"}},
	}
	if a.Kind() != KindAssignmentStatement {
		t.Fatalf("Kind = %v", a.Kind())
	}
	// Children: 0 vars + 1 expr
	if got, want := len(a.Children()), 1; got != want {
		t.Fatalf("len(Children) = %d, want %d", got, want)
	}
}

func TestExpressionStatement_Kind(t *testing.T) {
	es := &ExpressionStatement{
		SrcLoc:     loc("<test>", 1, 1, 1, 10),
		Expression: &Identifier{SrcLoc: loc("<test>", 1, 1, 1, 8), Text: "noop"},
	}
	if es.Kind() != KindExpressionStatement {
		t.Fatalf("Kind = %v", es.Kind())
	}
	if got, want := len(es.Children()), 1; got != want {
		t.Fatalf("len(Children) = %d, want %d", got, want)
	}
}
