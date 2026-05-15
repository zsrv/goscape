// pkg/pack/compiler/semantics/subject_test.go
package semantics

import (
	"strconv"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// triggerWithSubjectMode builds a proc-like trigger with the given subject mode.
func triggerWithSubjectMode(ident string, mode trigger.SubjectMode) *trigger.TriggerType {
	return &trigger.TriggerType{
		Identifier:      ident,
		SubjectMode:     mode,
		AllowParameters: true,
		AllowReturns:    true,
	}
}

func TestSubject_GlobalUnderscore_NoSubjectModeReturnsClean(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	t1 := triggerWithSubjectMode("foo", trigger.ModeNone)
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "_")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})
	if d.HasErrors() {
		t.Fatalf("HasErrors for ModeNone+'_': %+v", d.List())
	}
}

func TestSubject_GlobalUnderscore_TypeModeWithGlobalFalse_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)
	t1 := triggerWithSubjectMode("foo", trigger.NewModeType(typ.PrimitiveInt, true, false))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "_")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptSubjectNoGlobal {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_SUBJECT_NO_GLOBAL diagnostic: %+v", d.List())
	}
}

func TestSubject_TypeReference_ResolvesViaBasicLookup(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)
	// Pre-populate root table with an "obj_bowl" BasicSymbol.
	root.Insert(symbol.SymbolTypeBasic(typ.PrimitiveInt), &symbol.BasicSymbol{
		Name: "obj_bowl",
		Type: typ.PrimitiveInt,
	})
	t1 := triggerWithSubjectMode("foo", trigger.NewModeType(typ.PrimitiveInt, true, true))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "obj_bowl")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if d.HasErrors() {
		t.Fatalf("HasErrors: %+v", d.List())
	}
	if s.SubjectReference == nil {
		t.Fatal("SubjectReference nil")
	}
	bs, ok := s.SubjectReference.(*symbol.BasicSymbol)
	if !ok {
		t.Fatalf("SubjectReference = %T, want *symbol.BasicSymbol", s.SubjectReference)
	}
	if bs.Name != "obj_bowl" {
		t.Fatalf("SubjectReference name = %q", bs.Name)
	}
}

func TestSubject_TypeReference_Unresolved_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)
	t1 := triggerWithSubjectMode("foo", trigger.NewModeType(typ.PrimitiveInt, true, true))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "nope_does_not_exist")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageGenericUnresolvedSymbol {
			found = true
		}
	}
	if !found {
		t.Fatalf("no GENERIC_UNRESOLVED_SYMBOL diagnostic: %+v", d.List())
	}
}

func TestSubject_SpaceInSubject_NonTypeMode_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	t1 := triggerWithSubjectMode("foo", trigger.ModeNone)
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "has space")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptSubjectNoSpaces {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_SUBJECT_NO_SPACES diagnostic: %+v", d.List())
	}
}

func TestSubject_MapzoneSubject_PackedAndStoredAsBasicSymbol(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveMapzone)
	t1 := triggerWithSubjectMode("foo", trigger.NewModeType(typ.PrimitiveMapzone, true, true))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "0_50_50")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if d.HasErrors() {
		t.Fatalf("HasErrors for valid mapzone: %+v", d.List())
	}
	if s.SubjectReference == nil {
		t.Fatal("SubjectReference nil")
	}
	bs := s.SubjectReference.(*symbol.BasicSymbol)
	// Per TS L319 packed = (z & 0x3fff) | ((x & 0x3fff) << 14)
	// where x = mxInt<<6 = 50<<6 = 3200, z = mzInt<<6 = 50<<6 = 3200.
	x := int32(50 << 6)
	z := int32(50 << 6)
	want := (z & 0x3fff) | ((x & 0x3fff) << 14)
	if bs.Name != strconv.Itoa(int(want)) {
		t.Fatalf("SubjectReference name = %q, want %q (packed %d)", bs.Name, strconv.Itoa(int(want)), want)
	}
}

func TestSubject_MapzoneBadLevel_EmitsErrorAndStillSetsSubjectReference(t *testing.T) {
	// Per TS L302-304 + caller pattern at L357-380: when level != 0,
	// reportError is emitted AND the BasicSymbol is constructed with the
	// sentinel value (-1). SubjectReference is set regardless of error.
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveMapzone)
	t1 := triggerWithSubjectMode("foo", trigger.NewModeType(typ.PrimitiveMapzone, true, true))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "1_50_50") // level 1 → error
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if !d.HasErrors() {
		t.Fatal("HasErrors = false; want true (level 1 invalid)")
	}
	if s.SubjectReference == nil {
		t.Fatal("SubjectReference nil; TS sets it even on error")
	}
}

func TestSubject_CategorySubject_ResolvedViaCategoryType(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_, err := tm.RegisterNew("category", "", typ.BaseVarInteger, -1)
	if err != nil {
		t.Fatal(err)
	}
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)
	// Pre-populate the category table with "foo".
	cat, _ := tm.Find("category", false)
	root.Insert(symbol.SymbolTypeBasic(cat), &symbol.BasicSymbol{
		Name: "foo",
		Type: cat,
	})
	t1 := triggerWithSubjectMode("trig", trigger.NewModeType(typ.PrimitiveInt, true, true))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("trig", "_foo")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if d.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", d.List())
	}
	if s.SubjectReference == nil {
		t.Fatal("SubjectReference nil for category subject _foo")
	}
	bs := s.SubjectReference.(*symbol.BasicSymbol)
	if bs.Name != "foo" {
		t.Fatalf("SubjectReference name = %q, want \"foo\"", bs.Name)
	}
}

func TestSubject_CategorySubject_TypeMode_CategoryFalse_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_, _ = tm.RegisterNew("category", "", typ.BaseVarInteger, -1)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)
	t1 := triggerWithSubjectMode("trig", trigger.NewModeType(typ.PrimitiveInt, false, true))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("trig", "_foo")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptSubjectNoCategory {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_SUBJECT_NO_CATEGORY diagnostic: %+v", d.List())
	}
}

func TestSubject_CoordSubject_PackedAndStoredAsBasicSymbol(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveCoord)
	t1 := triggerWithSubjectMode("foo", trigger.NewModeType(typ.PrimitiveCoord, true, true))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "0_50_50_0_0")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if d.HasErrors() {
		t.Fatalf("HasErrors for valid coord: %+v", d.List())
	}
	if s.SubjectReference == nil {
		t.Fatal("SubjectReference nil")
	}
	bs := s.SubjectReference.(*symbol.BasicSymbol)
	// Per tryParseZone: x = (((mx<<6)|lx)>>3)<<3, z = (((mz<<6)|lz)>>3)<<3
	// "0_50_50_0_0": mx=mz=50, lx=lz=0, level=0
	// x = (((50<<6)|0)>>3)<<3 = (3200>>3)<<3 = 400<<3 = 3200
	// z = 3200
	// packed = (z & 0x3fff) | ((x & 0x3fff) << 14) | ((level & 0x3) << 28)
	x := int32((((50 << 6) | 0) >> 3) << 3)
	z := int32((((50 << 6) | 0) >> 3) << 3)
	level := int32(0)
	want := (z & 0x3fff) | ((x & 0x3fff) << 14) | ((level & 0x3) << 28)
	if bs.Name != strconv.Itoa(int(want)) {
		t.Fatalf("SubjectReference name = %q, want %q (packed %d)", bs.Name, strconv.Itoa(int(want)), want)
	}
}

func TestSubject_CoordSubject_BadAlignment_EmitsErrorAndStillSetsSubjectReference(t *testing.T) {
	// lx=4 is not a multiple of 8; tryParseZone emits
	// MessageZoneLocalCoordMultipleOf8 and returns -1. resolveSubjectSymbol
	// still constructs the BasicSymbol (with "-1") and assigns SubjectReference.
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveCoord)
	t1 := triggerWithSubjectMode("foo", trigger.NewModeType(typ.PrimitiveCoord, true, true))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "0_50_50_4_0") // lx=4, not a multiple of 8
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageZoneLocalCoordMultipleOf8 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no MessageZoneLocalCoordMultipleOf8 diagnostic: %+v", d.List())
	}
	if s.SubjectReference == nil {
		t.Fatal("SubjectReference nil; resolveSubjectSymbol sets it even on error")
	}
	bs := s.SubjectReference.(*symbol.BasicSymbol)
	if bs.Name != "-1" {
		t.Fatalf("SubjectReference name = %q, want \"-1\" (sentinel)", bs.Name)
	}
}
