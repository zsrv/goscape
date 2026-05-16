// pkg/pack/compiler/type/scriptvar_test.go
package typ

import "testing"

// TestScriptVarTypeAll_Order pins TS L25-83 declaration order.
// Each entry: (representation, code).
func TestScriptVarTypeAll_Order(t *testing.T) {
	want := []struct {
		rep, code string
	}{
		{"seq", "A"},
		{"locshape", "H"},
		{"component", "I"},
		{"idkit", "K"},
		{"midi", "M"},
		{"npc_mode", "N"},
		{"namedobj", "O"},
		{"synth", "P"},
		{"area", "R"},
		{"stat", "S"},
		{"npc_stat", "T"},
		{"writeinv", "V"},
		{"wma", "`"},
		{"graphic", "d"},
		{"fontmetrics", "f"},
		{"enum", "g"},
		{"hunt", "h"},
		{"jingle", "j"},
		{"loc", "l"},
		{"model", "m"},
		{"npc", "n"},
		{"obj", "o"},
		{"player_uid", "p"},
		{"spotanim", "t"},
		{"npc_uid", "u"},
		{"inv", "v"},
		{"texture", "x"},
		// CATEGORY ('y') lives in PrimitiveCategory (T5); not in ScriptVarTypeAll.
		{"mapelement", "µ"},
		{"hitmark", "×"},
		{"struct", "J"},
		{"dbrow", "Ð"},
		{"interface", "a"},
		{"toplevelinterface", "F"},
		{"overlayinterface", "L"},
		{"movespeed", "Ý"},
		{"entityoverlay", "-"},
		{"dbtable", "Ø"},
		{"stringvector", "¸"},
		{"mesanim", "Á"},
		{"verifyobj", "®"},
	}
	if got := len(ScriptVarTypeAll); got != len(want) {
		t.Fatalf("ScriptVarTypeAll length = %d, want %d", got, len(want))
	}
	for i, w := range want {
		entry := ScriptVarTypeAll[i]
		if got := entry.Representation(); got != w.rep {
			t.Errorf("ScriptVarTypeAll[%d].Representation = %q, want %q", i, got, w.rep)
		}
		code, ok := entry.Code()
		if !ok || code != w.code {
			t.Errorf("ScriptVarTypeAll[%d].Code = (%q, %v), want (%q, true)", i, code, ok, w.code)
		}
	}
}

// TestScriptVarType_BaseType pins TS L21: all entries are INTEGER.
func TestScriptVarType_BaseType(t *testing.T) {
	for i, sv := range ScriptVarTypeAll {
		bt, ok := sv.BaseType()
		if !ok || bt != BaseVarInteger {
			t.Errorf("ScriptVarTypeAll[%d].BaseType = (%v, %v), want (Integer, true)", i, bt, ok)
		}
	}
}

// TestScriptVarType_AsType verifies *ScriptVarType implements Type.
func TestScriptVarType_AsType(t *testing.T) {
	var _ Type = ScriptVarLoc
}

// TestScriptVarLoc_Singleton spot-checks a named singleton.
func TestScriptVarLoc_Singleton(t *testing.T) {
	if got := ScriptVarLoc.Representation(); got != "loc" {
		t.Errorf("ScriptVarLoc.Representation = %q, want \"loc\"", got)
	}
	code, _ := ScriptVarLoc.Code()
	if code != "l" {
		t.Errorf("ScriptVarLoc.Code = %q, want \"l\"", code)
	}
}
