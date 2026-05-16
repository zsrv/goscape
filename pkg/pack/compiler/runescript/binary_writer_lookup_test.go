// pkg/pack/compiler/runescript/binary_writer_lookup_test.go
package runescript_test

import (
	"encoding/binary"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// runAndExtractLookupKey runs Write() on a 1-block / 1-instruction script
// and extracts the lookupKey from the Finish() blob. `stubIdProvider` from
// binary_writer_test.go is used directly — both files are package
// runescript_test so they share test fixtures.
func runAndExtractLookupKey(t *testing.T, s *codegen.RuneScript, idp stubIdProvider) int32 {
	t.Helper()
	if len(s.Blocks) == 0 {
		s.Blocks = []*codegen.Block{codegen.NewBlock(&codegen.Label{Name: "e"})}
	}
	s.Blocks[0].Add(codegen.Instruction{Opcode: codegen.Return})
	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(idp, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1
	return int32(binary.BigEndian.Uint32(out.data[off:]))
}

// TestLookupKey_NameMode pins SubjectMode.Name → -1 (TS L65).
func TestLookupKey_NameMode(t *testing.T) {
	tr := &trigger.TriggerType{ID: 5, Identifier: "proc", SubjectMode: trigger.ModeName, AllowParameters: true, AllowReturns: true}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "x", Parameters: typ.MetaUnit, Returns: typ.MetaUnit}}
	s := codegen.NewRuneScript("smoke.rs2", ss, tr, "x", nil)
	if got := runAndExtractLookupKey(t, s, stubIdProvider{}); got != -1 {
		t.Errorf("lookupKey = %d, want -1", got)
	}
}

// TestLookupKey_TypeMode_NonCategory pins category=false subject →
// trigger.ID + (2<<8) + (subjectId<<10).
// subject = BasicSymbol "weapon1"; stub IdProvider returns 17.
// Trigger.ID = 5 → key = 5 + (2<<8) + (17<<10) = 5 + 512 + 17408 = 17925.
func TestLookupKey_TypeMode_NonCategory(t *testing.T) {
	tm := trigger.NewModeType(typ.PrimitiveInt, false, false)
	tr := &trigger.TriggerType{ID: 5, Identifier: "opheld1", SubjectMode: tm}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "x", Parameters: typ.MetaUnit, Returns: typ.MetaUnit}}
	subject := &symbol.BasicSymbol{Name: "weapon1", Type: typ.PrimitiveInt}
	s := codegen.NewRuneScript("smoke.rs2", ss, tr, "x", subject)

	if got := runAndExtractLookupKey(t, s, stubIdProvider{id: 17}); got != 17925 {
		t.Errorf("lookupKey = %d, want 17925", got)
	}
}

// TestLookupKey_TypeMode_Category pins category=true → typeMarker = 1.
// Trigger.ID = 5; subject id 17; key = 5 + (1<<8) + (17<<10) = 5 + 256 + 17408 = 17669.
//
// NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR: this asserts the goscape
// per-trigger interpretation. TS asserts per-subject. The test pins the
// current behavior; resolving the deviation will require updating this test.
func TestLookupKey_TypeMode_Category(t *testing.T) {
	tm := trigger.NewModeType(typ.PrimitiveInt, true, false)
	tr := &trigger.TriggerType{ID: 5, Identifier: "opheld1", SubjectMode: tm}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "x", Parameters: typ.MetaUnit, Returns: typ.MetaUnit}}
	subject := &symbol.BasicSymbol{Name: "weapons", Type: typ.PrimitiveInt}
	s := codegen.NewRuneScript("smoke.rs2", ss, tr, "x", subject)

	if got := runAndExtractLookupKey(t, s, stubIdProvider{id: 17}); got != 17669 {
		t.Errorf("lookupKey = %d, want 17669", got)
	}
}

// TestLookupKey_MapzonePath pins the strconv.Atoi(subject.SymbolName()) path
// for MAPZONE primitives.
func TestLookupKey_MapzonePath(t *testing.T) {
	tm := trigger.NewModeType(typ.PrimitiveMapzone, false, false)
	tr := &trigger.TriggerType{ID: 5, Identifier: "zone_enter", SubjectMode: tm}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "x", Parameters: typ.MetaUnit, Returns: typ.MetaUnit}}
	subject := &symbol.BasicSymbol{Name: "12345", Type: typ.PrimitiveMapzone}
	s := codegen.NewRuneScript("smoke.rs2", ss, tr, "x", subject)

	// Expected: 5 + (2<<8) + (12345<<10) = 5 + 512 + 12641280 = 12641797.
	if got := runAndExtractLookupKey(t, s, stubIdProvider{}); got != 12641797 {
		t.Errorf("lookupKey = %d, want 12641797", got)
	}
}

// TestLookupKey_MapzoneInvalidPanics pins NAI-209-D-MAPZONE-COORD-PARSE-PANIC.
func TestLookupKey_MapzoneInvalidPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("invalid MAPZONE name did not panic")
		}
	}()
	tm := trigger.NewModeType(typ.PrimitiveMapzone, false, false)
	tr := &trigger.TriggerType{ID: 5, Identifier: "zone_enter", SubjectMode: tm}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "x", Parameters: typ.MetaUnit, Returns: typ.MetaUnit}}
	subject := &symbol.BasicSymbol{Name: "not-a-number", Type: typ.PrimitiveMapzone}
	s := codegen.NewRuneScript("smoke.rs2", ss, tr, "x", subject)
	runAndExtractLookupKey(t, s, stubIdProvider{})
}

// TestIsNameMode pins the new trigger helper.
func TestIsNameMode(t *testing.T) {
	if !trigger.IsNameMode(trigger.ModeName) {
		t.Errorf("IsNameMode(ModeName) = false, want true")
	}
	if trigger.IsNameMode(trigger.ModeNone) {
		t.Errorf("IsNameMode(ModeNone) = true, want false")
	}
	if trigger.IsNameMode(trigger.NewModeType(typ.PrimitiveInt, false, false)) {
		t.Errorf("IsNameMode(TypeMode) = true, want false")
	}
}
