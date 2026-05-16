// pkg/pack/compiler/runescript/default_type_checkers_test.go
package runescript

import (
	"testing"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestDefaultTypeCheckers_AnyAcceptsAnything(t *testing.T) {
	tm := typ.NewTypeManager()
	registerDefaultTypeCheckers(tm)
	if !tm.Check(typ.MetaAny, typ.PrimitiveInt) {
		t.Error("MetaAny <- PrimitiveInt: want accept")
	}
	if !tm.Check(typ.MetaAny, typ.PrimitiveString) {
		t.Error("MetaAny <- PrimitiveString: want accept")
	}
}

func TestDefaultTypeCheckers_ErrorPropagation(t *testing.T) {
	tm := typ.NewTypeManager()
	registerDefaultTypeCheckers(tm)
	if !tm.Check(typ.MetaError, typ.PrimitiveInt) {
		t.Error("MetaError <- PrimitiveInt: want accept (error propagation)")
	}
	if !tm.Check(typ.PrimitiveInt, typ.MetaError) {
		t.Error("PrimitiveInt <- MetaError: want accept (error propagation)")
	}
}

func TestDefaultTypeCheckers_ReflexiveEquality(t *testing.T) {
	tm := typ.NewTypeManager()
	registerDefaultTypeCheckers(tm)
	if !tm.Check(typ.PrimitiveInt, typ.PrimitiveInt) {
		t.Error("Int <- Int: want accept")
	}
	if tm.Check(typ.PrimitiveInt, typ.PrimitiveString) {
		t.Error("Int <- String: want reject")
	}
}

func TestDefaultTypeCheckers_MetaScriptMatchesParamsAndReturns(t *testing.T) {
	tm := typ.NewTypeManager()
	registerDefaultTypeCheckers(tm)
	a := typ.NewMetaScript("proc", typ.PrimitiveInt, typ.PrimitiveString)
	b := typ.NewMetaScript("proc", typ.PrimitiveInt, typ.PrimitiveString)
	if !tm.Check(a, b) {
		t.Error("matching MetaScript pair: want accept")
	}
}
