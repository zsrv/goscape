// pkg/pack/compiler/trigger/subjectmode_test.go
package trigger

import (
	"testing"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestSubjectMode_NoneAndName_DistinctSingletons(t *testing.T) {
	if ModeNone == nil || ModeName == nil {
		t.Fatal("ModeNone or ModeName is nil")
	}
	if ModeNone == ModeName {
		t.Fatal("ModeNone == ModeName; want distinct")
	}
}

func TestSubjectMode_NewModeTypeFields(t *testing.T) {
	tm := NewModeType(typ.PrimitiveInt, true, true)
	if tm.Type != typ.PrimitiveInt {
		t.Fatalf("TypeMode.Type = %v, want PrimitiveInt", tm.Type)
	}
	if !tm.Category {
		t.Fatal("TypeMode.Category = false, want true")
	}
	if !tm.Global {
		t.Fatal("TypeMode.Global = false, want true")
	}
}

func TestIsTypeMode_PositiveAndNegative(t *testing.T) {
	tm := NewModeType(typ.PrimitiveInt, false, false)
	got, ok := IsTypeMode(tm)
	if !ok {
		t.Fatal("IsTypeMode(TypeMode) ok=false, want true")
	}
	if got.Type != typ.PrimitiveInt {
		t.Fatalf("IsTypeMode returned wrong TypeMode: %+v", got)
	}

	if _, ok := IsTypeMode(ModeNone); ok {
		t.Fatal("IsTypeMode(ModeNone) ok=true, want false")
	}
	if _, ok := IsTypeMode(ModeName); ok {
		t.Fatal("IsTypeMode(ModeName) ok=true, want false")
	}
}
