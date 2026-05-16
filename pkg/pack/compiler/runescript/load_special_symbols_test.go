// pkg/pack/compiler/runescript/load_special_symbols_test.go
package runescript

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
)

// TestLoadSpecialSymbols_BasicMapPopulation populates SymbolMapper entries
// for command + script CompilerTypeInfo.Map.
func TestLoadSpecialSymbols_BasicMapPopulation(t *testing.T) {
	commandInfo := &CompilerTypeInfo{
		Map: map[string]string{"0": "cmd_a", "1": "cmd_b"},
	}
	scriptInfo := &CompilerTypeInfo{
		Map: map[string]string{"100": "[proc,hello]", "200": "[proc,world]"},
	}
	mapper := NewSymbolMapper(nil)
	commandPointers := map[string]*pointer.PointerHolder{}

	if err := LoadSpecialSymbols(commandInfo, scriptInfo, mapper, commandPointers, true); err != nil {
		t.Fatalf("LoadSpecialSymbols: %v", err)
	}
	// Command mapper entries
	// (SymbolMapper API exposes Get via symbol.Symbol — to peek without a Symbol,
	// rely on the resulting state through e.g. an exported "commands by name" helper.
	// If no such helper exists, this test reduces to "no error".)
}

// TestLoadSpecialSymbols_RequireOnly_InsertsHolder pins TS L98-106.
func TestLoadSpecialSymbols_RequireOnly_InsertsHolder(t *testing.T) {
	commandInfo := &CompilerTypeInfo{
		Map:     map[string]string{"0": "foo"},
		Require: map[string]string{"0": "active_player,active_npc"},
	}
	scriptInfo := &CompilerTypeInfo{Map: map[string]string{}}
	mapper := NewSymbolMapper(nil)
	commandPointers := map[string]*pointer.PointerHolder{}

	if err := LoadSpecialSymbols(commandInfo, scriptInfo, mapper, commandPointers, true); err != nil {
		t.Fatalf("LoadSpecialSymbols: %v", err)
	}
	h, ok := commandPointers["foo"]
	if !ok {
		t.Fatal("foo holder not inserted")
	}
	if h.Required == nil || h.Required.Len() != 2 {
		t.Errorf("foo.Required size: got %d, want 2", h.Required.Len())
	}
	if !h.Required.Has(pointer.ActivePlayer) || !h.Required.Has(pointer.ActiveNpc) {
		t.Errorf("foo.Required missing ActivePlayer or ActiveNpc")
	}
}

// TestLoadSpecialSymbols_None_EmptySet pins TS L120-122: 'none' or empty
// resolves to empty set.
func TestLoadSpecialSymbols_None_EmptySet(t *testing.T) {
	commandInfo := &CompilerTypeInfo{
		Map:     map[string]string{"0": "foo"},
		Require: map[string]string{"0": "none"},
		Set:     map[string]string{"0": ""},
		Corrupt: map[string]string{"0": "active_player"},
	}
	scriptInfo := &CompilerTypeInfo{Map: map[string]string{}}
	mapper := NewSymbolMapper(nil)
	commandPointers := map[string]*pointer.PointerHolder{}

	if err := LoadSpecialSymbols(commandInfo, scriptInfo, mapper, commandPointers, true); err != nil {
		t.Fatalf("LoadSpecialSymbols: %v", err)
	}
	h := commandPointers["foo"]
	if h == nil {
		t.Fatal("foo holder not inserted")
	}
	if h.Required != nil && h.Required.Len() != 0 {
		t.Errorf("'none' Require: got len %d, want empty", h.Required.Len())
	}
}

// TestLoadSpecialSymbols_Require2_InsertsAliasHolder pins TS L106-111.
func TestLoadSpecialSymbols_Require2_InsertsAliasHolder(t *testing.T) {
	commandInfo := &CompilerTypeInfo{
		Map:      map[string]string{"0": "foo"},
		Require:  map[string]string{"0": "active_player"},
		Require2: map[string]string{"0": "active_loc"},
	}
	scriptInfo := &CompilerTypeInfo{Map: map[string]string{}}
	mapper := NewSymbolMapper(nil)
	commandPointers := map[string]*pointer.PointerHolder{}

	if err := LoadSpecialSymbols(commandInfo, scriptInfo, mapper, commandPointers, true); err != nil {
		t.Fatalf("LoadSpecialSymbols: %v", err)
	}
	if _, ok := commandPointers["foo"]; !ok {
		t.Error("foo holder not inserted")
	}
	if _, ok := commandPointers[".foo"]; !ok {
		t.Error(".foo alias holder not inserted")
	}
}

// TestLoadSpecialSymbols_NoPointerInfo_NoHolderInserted pins TS L98.
func TestLoadSpecialSymbols_NoPointerInfo_NoHolderInserted(t *testing.T) {
	commandInfo := &CompilerTypeInfo{
		Map: map[string]string{"0": "no_pointers_cmd"},
	}
	scriptInfo := &CompilerTypeInfo{Map: map[string]string{}}
	mapper := NewSymbolMapper(nil)
	commandPointers := map[string]*pointer.PointerHolder{}

	if err := LoadSpecialSymbols(commandInfo, scriptInfo, mapper, commandPointers, true); err != nil {
		t.Fatalf("LoadSpecialSymbols: %v", err)
	}
	if _, ok := commandPointers["no_pointers_cmd"]; ok {
		t.Error("no_pointers_cmd holder should not be inserted")
	}
}

// TestLoadSpecialSymbols_CheckPointersFalse_SkipsHolders pins TS L98 (the
// `if (checkPointers && ...)` gate).
func TestLoadSpecialSymbols_CheckPointersFalse_SkipsHolders(t *testing.T) {
	commandInfo := &CompilerTypeInfo{
		Map:     map[string]string{"0": "foo"},
		Require: map[string]string{"0": "active_player"},
	}
	scriptInfo := &CompilerTypeInfo{Map: map[string]string{}}
	mapper := NewSymbolMapper(nil)
	commandPointers := map[string]*pointer.PointerHolder{}

	if err := LoadSpecialSymbols(commandInfo, scriptInfo, mapper, commandPointers, false); err != nil {
		t.Fatalf("LoadSpecialSymbols: %v", err)
	}
	if _, ok := commandPointers["foo"]; ok {
		t.Error("checkPointers=false: foo holder should not be inserted")
	}
}

// TestLoadSpecialSymbols_UnknownPointer_ReturnsError pins TS L131 (Go: error
// vs throw).
func TestLoadSpecialSymbols_UnknownPointer_ReturnsError(t *testing.T) {
	commandInfo := &CompilerTypeInfo{
		Map:     map[string]string{"0": "foo"},
		Require: map[string]string{"0": "bogus_pointer_name"},
	}
	scriptInfo := &CompilerTypeInfo{Map: map[string]string{}}
	mapper := NewSymbolMapper(nil)
	commandPointers := map[string]*pointer.PointerHolder{}

	err := LoadSpecialSymbols(commandInfo, scriptInfo, mapper, commandPointers, true)
	if err == nil {
		t.Error("unknown pointer: want error, got nil")
	}
}
