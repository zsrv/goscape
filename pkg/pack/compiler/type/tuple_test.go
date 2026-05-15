// pkg/pack/compiler/type/tuple_test.go
package typ

import (
	"errors"
	"testing"
)

func TestNewTupleType_RejectsLessThanTwo(t *testing.T) {
	if _, err := NewTupleType(); err == nil {
		t.Fatal("NewTupleType() = nil, want error")
	}
	if _, err := NewTupleType(PrimitiveInt); err == nil {
		t.Fatal("NewTupleType(INT) = nil, want error")
	}
}

func TestNewTupleType_FlattensNested(t *testing.T) {
	inner, err := NewTupleType(PrimitiveInt, PrimitiveString)
	if err != nil {
		t.Fatalf("NewTupleType inner: %v", err)
	}
	tup, err := NewTupleType(inner, PrimitiveBoolean)
	if err != nil {
		t.Fatalf("NewTupleType outer: %v", err)
	}
	// Flattened children: int, string, boolean (no nested tuple)
	got := tup.Children
	want := []Type{PrimitiveInt, PrimitiveString, PrimitiveBoolean}
	if len(got) != len(want) {
		t.Fatalf("children len=%d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("children[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestTupleType_Representation(t *testing.T) {
	tup, _ := NewTupleType(PrimitiveInt, PrimitiveString, PrimitiveBoolean)
	if got, want := tup.Representation(), "int,string,boolean"; got != want {
		t.Fatalf("representation = %q, want %q", got, want)
	}
}

func TestTupleType_FromList(t *testing.T) {
	if got := TupleFromList(nil); got != MetaUnit {
		t.Fatalf("fromList(nil) = %v, want MetaUnit", got)
	}
	if got := TupleFromList([]Type{}); got != MetaUnit {
		t.Fatalf("fromList([]) = %v, want MetaUnit", got)
	}
	if got := TupleFromList([]Type{PrimitiveInt}); got != PrimitiveInt {
		t.Fatalf("fromList([int]) = %v, want PrimitiveInt", got)
	}
	got := TupleFromList([]Type{PrimitiveInt, PrimitiveString})
	tup, ok := got.(*TupleType)
	if !ok {
		t.Fatalf("fromList([int, string]) = %T, want *TupleType", got)
	}
	if len(tup.Children) != 2 {
		t.Fatalf("tuple children len = %d, want 2", len(tup.Children))
	}
}

func TestTupleType_ToList(t *testing.T) {
	if got := TupleToList(nil); len(got) != 0 {
		t.Fatalf("toList(nil) len = %d, want 0", len(got))
	}
	if got := TupleToList(MetaUnit); len(got) != 0 {
		t.Fatalf("toList(MetaUnit) = %v, want []", got)
	}
	if got := TupleToList(MetaNothing); len(got) != 0 {
		t.Fatalf("toList(MetaNothing) = %v, want []", got)
	}
	got := TupleToList(PrimitiveInt)
	if len(got) != 1 || got[0] != PrimitiveInt {
		t.Fatalf("toList(PrimitiveInt) = %v, want [PrimitiveInt]", got)
	}
	tup, _ := NewTupleType(PrimitiveInt, PrimitiveString)
	got = TupleToList(tup)
	if len(got) != 2 || got[0] != PrimitiveInt || got[1] != PrimitiveString {
		t.Fatalf("toList(tuple) = %v, want [int, string]", got)
	}
}

func TestTupleType_SatisfiesAstTypeRef(t *testing.T) {
	tup, _ := NewTupleType(PrimitiveInt, PrimitiveString)
	var _ astTypeRef = tup
}

// Carry-forward: TupleType has slice field Children — verify the
// TupleType-mistakenly-keyed-into-SymbolType invariant by NOT making
// TupleType a comparable struct. (No assertion possible at the type-system
// level; covered by SymbolType tests in T6.)
var _ = errors.New
