package compiler

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// TestLoadCompilerConstants_StripsLeadingCaret pins TS Compiler.ts:162-164:
// names beginning with '^' have the '^' stripped before storage.
func TestLoadCompilerConstants_StripsLeadingCaret(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "^FOO=bar\n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	if got, want := m["FOO"], "bar"; got != want {
		t.Errorf("m[\"FOO\"] = %q, want %q", got, want)
	}
	if _, present := m["^FOO"]; present {
		t.Errorf("m has both ^FOO and FOO; caret should have been stripped")
	}
}

// TestLoadCompilerConstants_StripsSurroundingQuotes pins TS Compiler.ts:166-169.
func TestLoadCompilerConstants_StripsSurroundingQuotes(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant",
		"A=\"quoted\"\nB=unquoted\nC=\"mismatch\nD=mismatch\"\nE=\"in\"middle\"\n",
	)

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	cases := map[string]string{
		"A": "quoted",      // both-sided quotes stripped
		"B": "unquoted",    // no quotes, unchanged
		"C": "\"mismatch",  // open-only, unchanged
		"D": "mismatch\"",  // close-only, unchanged
		"E": "in\"middle",  // input "in"middle" — outer pair stripped, inner quote retained
	}
	for k, want := range cases {
		if got := m[k]; got != want {
			t.Errorf("m[%q] = %q, want %q", k, got, want)
		}
	}
}

// TestLoadCompilerConstants_LastWriterWins pins NAI-202-D-CONSTANT-LOOSE-PARSER:
// duplicate names within the same file resolve last-line-wins (no error,
// unlike pkg/pack.LoadConstants which errors).
func TestLoadCompilerConstants_LastWriterWins(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "K=a\nK=b\n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v (NAI-202-D-CONSTANT-LOOSE-PARSER: dup must not error)", err)
	}
	if got, want := m["K"], "b"; got != want {
		t.Errorf("m[\"K\"] = %q, want %q (last-line-wins per loose parser)", got, want)
	}
}

// TestLoadCompilerConstants_SkipsComments pins TS Compiler.ts:155:
// lines beginning with '//' are skipped.
func TestLoadCompilerConstants_SkipsComments(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "// K=a\nK=b\n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	if got, want := m["K"], "b"; got != want {
		t.Errorf("m[\"K\"] = %q, want %q (// line skipped)", got, want)
	}
	if len(m) != 1 {
		t.Errorf("len(m) = %d, want 1; map = %v", len(m), m)
	}
}

// TestLoadCompilerConstants_DiscardsPastSecondEquals pins TS-faithful
// destructure of unbounded split: parts[0]=name, parts[1]=value, parts[2:]
// dropped. K=v=extra → m["K"] = "v" (not "v=extra").
func TestLoadCompilerConstants_DiscardsPastSecondEquals(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "K=v=extra\n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	if got, want := m["K"], "v"; got != want {
		t.Errorf("m[\"K\"] = %q, want %q (parts past second '=' discarded)", got, want)
	}
}

// TestLoadCompilerConstants_ErrorsOnMissingEquals pins TS-faithful
// behaviour: a line with no '=' triggers an undefined-index throw in TS.
// Goscape returns a wrapped error including the file path.
func TestLoadCompilerConstants_ErrorsOnMissingEquals(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "broken_line_no_equals\n")

	_, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err == nil {
		t.Fatal("expected error on line missing '=', got nil")
	}
	if !strings.Contains(err.Error(), "a.constant") {
		t.Errorf("error %q must mention the offending file path", err.Error())
	}
}

// TestLoadCompilerConstants_TrimsWhitespace pins TS Compiler.ts:161,166
// which call .trim() on both name and value.
func TestLoadCompilerConstants_TrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "  K  =  v  \n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	if got, want := m["K"], "v"; got != want {
		t.Errorf("m[\"K\"] = %q, want %q (whitespace trimmed)", got, want)
	}
}

// TestLoadCompilerConstants_EmptyScriptsDir pins: missing scripts dir
// returns an empty map with nil error.
func TestLoadCompilerConstants_EmptyScriptsDir(t *testing.T) {
	dir := t.TempDir()
	// No scripts/ subdir created.

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v (missing dir must not error)", err)
	}
	if len(m) != 0 {
		t.Errorf("len(m) = %d, want 0", len(m))
	}
}

// TestScriptVarTypeName_KnownCodes pins the name returned for each
// ScriptVarType constant, mirroring TS ScriptVarType.getType
// (ScriptVarType.ts:85-170) and goscape's existing
// objtype.ParamType.GetType() (paramtype.go:105).
func TestScriptVarTypeName_KnownCodes(t *testing.T) {
	cases := []struct {
		t    objtype.ScriptVarType
		want string
	}{
		{objtype.ScriptVarTypeInt, "int"},
		{objtype.ScriptVarTypeString, "string"},
		{objtype.ScriptVarTypeEnum, "enum"},
		{objtype.ScriptVarTypeObj, "obj"},
		{objtype.ScriptVarTypeLoc, "loc"},
		{objtype.ScriptVarTypeComponent, "component"},
		{objtype.ScriptVarTypeNamedObj, "namedobj"},
		{objtype.ScriptVarTypeStruct, "struct"},
		{objtype.ScriptVarTypeBoolean, "boolean"},
		{objtype.ScriptVarTypeCoord, "coord"},
		{objtype.ScriptVarTypeCategory, "category"},
		{objtype.ScriptVarTypeSpotanim, "spotanim"},
		{objtype.ScriptVarTypeNPC, "npc"},
		{objtype.ScriptVarTypeInv, "inv"},
		{objtype.ScriptVarTypeSynth, "synth"},
		{objtype.ScriptVarTypeSeq, "seq"},
		{objtype.ScriptVarTypeStat, "stat"},
		{objtype.ScriptVarTypeInterface, "interface"},
	}
	for _, c := range cases {
		if got := scriptVarTypeName(c.t); got != c.want {
			t.Errorf("scriptVarTypeName(%d) = %q, want %q", c.t, got, c.want)
		}
	}
}

// TestScriptVarTypeName_UnknownCode pins the "unknown" return for type
// codes not in the switch (matches ParamType.GetType()).
// Note: constants use ASCII codepoints (e.g. 99='c'=coord, 105='i'=int);
// values 0 and 1 are not assigned to any ScriptVarType constant.
func TestScriptVarTypeName_UnknownCode(t *testing.T) {
	if got := scriptVarTypeName(objtype.ScriptVarType(0)); got != "unknown" {
		t.Errorf("scriptVarTypeName(0) = %q, want \"unknown\"", got)
	}
	if got := scriptVarTypeName(objtype.ScriptVarType(1)); got != "unknown" {
		t.Errorf("scriptVarTypeName(1) = %q, want \"unknown\"", got)
	}
}

// TestPopulateCommandInfoFrom_AscendingIteration pins TS Compiler.ts:111
// — iteration is sorted ascending by opcode value. Tests via the seam
// populateCommandInfoFrom which accepts the opmap/pointers as args.
func TestPopulateCommandInfoFrom_AscendingIteration(t *testing.T) {
	opmap := map[string]script.Opcode{
		"ZETA":  script.Opcode(100),
		"ALPHA": script.Opcode(1),
		"BETA":  script.Opcode(10),
	}
	pointers := map[script.Opcode]script.Pointers{}

	info := newTypeInfo()
	populateCommandInfoFrom(info, opmap, pointers)

	// Max should be 101 (highest opcode = 100, Max = id+1).
	if info.Max != 101 {
		t.Errorf("Max = %d, want 101 (highest opcode 100 → Max = 100+1)", info.Max)
	}
	// Names are lowercased.
	want := map[int]string{1: "alpha", 10: "beta", 100: "zeta"}
	for id, name := range want {
		if got := info.Map[id]; got != name {
			t.Errorf("Map[%d] = %q, want %q", id, got, name)
		}
	}
}

// TestPopulateCommandInfoFrom_RequireSetCorrupt pins TS Compiler.ts:123-149
// — for an opcode with Require/Set/Corrupt fields, the corresponding
// commandInfo maps are populated with comma-joined strings.
func TestPopulateCommandInfoFrom_RequireSetCorrupt(t *testing.T) {
	op := script.Opcode(42)
	opmap := map[string]script.Opcode{"FOO": op}
	pointers := map[script.Opcode]script.Pointers{
		op: {
			Require: []string{"active_player", "find_loc"},
			Set:     []string{"active_npc"},
			Corrupt: []string{"find_npc", "find_loc"},
		},
	}

	info := newTypeInfo()
	populateCommandInfoFrom(info, opmap, pointers)

	if got, want := info.Require[42], "active_player,find_loc"; got != want {
		t.Errorf("Require[42] = %q, want %q", got, want)
	}
	if got, want := info.Set[42], "active_npc"; got != want {
		t.Errorf("Set[42] = %q, want %q", got, want)
	}
	if got, want := info.Corrupt[42], "find_npc,find_loc"; got != want {
		t.Errorf("Corrupt[42] = %q, want %q", got, want)
	}
}

// TestPopulateCommandInfoFrom_Conditional pins TS Compiler.ts:132-134
// — Conditional is set in commandInfo.Conditional[op] only when both
// Set is non-empty AND ptrs.Conditional is true.
func TestPopulateCommandInfoFrom_Conditional(t *testing.T) {
	cases := []struct {
		name        string
		set         []string
		conditional bool
		wantPresent bool
		wantValue   bool
	}{
		{"set + conditional", []string{"x"}, true, true, true},
		{"set, not conditional", []string{"x"}, false, false, false},
		{"conditional, no set", nil, true, false, false}, // TS only writes inside `if pointers.set`
		{"neither", nil, false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			op := script.Opcode(1)
			opmap := map[string]script.Opcode{"OP": op}
			pointers := map[script.Opcode]script.Pointers{
				op: {Set: c.set, Conditional: c.conditional},
			}
			info := newTypeInfo()
			populateCommandInfoFrom(info, opmap, pointers)

			got, present := info.Conditional[1]
			if present != c.wantPresent {
				t.Errorf("Conditional[1] present=%v, want %v", present, c.wantPresent)
			}
			if got != c.wantValue {
				t.Errorf("Conditional[1] = %v, want %v", got, c.wantValue)
			}
		})
	}
}

// TestPopulateCommandInfoFrom_Corrupt2FieldFix pins NAI-202-D-CORRUPT2-FIELD:
// TS Compiler.ts:146-147 had a typo (corrupt2 arm assigned to .corrupt,
// overwriting). Goscape fixes by assigning corrupt2 to .Corrupt2.
func TestPopulateCommandInfoFrom_Corrupt2FieldFix(t *testing.T) {
	op := script.Opcode(7)
	opmap := map[string]script.Opcode{"OP": op}
	pointers := map[script.Opcode]script.Pointers{
		op: {
			Corrupt:  []string{"corrupt_a", "corrupt_b"},
			Corrupt2: []string{"corrupt2_x", "corrupt2_y"},
		},
	}

	info := newTypeInfo()
	populateCommandInfoFrom(info, opmap, pointers)

	if got, want := info.Corrupt[7], "corrupt_a,corrupt_b"; got != want {
		t.Errorf("Corrupt[7] = %q, want %q (must NOT be overwritten by corrupt2)", got, want)
	}
	if got, want := info.Corrupt2[7], "corrupt2_x,corrupt2_y"; got != want {
		t.Errorf("Corrupt2[7] = %q, want %q (NAI-202-D-CORRUPT2-FIELD: corrected destination)", got, want)
	}
}

// TestPopulateCommandInfoFrom_Require2Set2 covers the require2 + set2
// arms which are not bugs in TS but are still part of the enrichment.
func TestPopulateCommandInfoFrom_Require2Set2(t *testing.T) {
	op := script.Opcode(3)
	opmap := map[string]script.Opcode{"OP": op}
	pointers := map[script.Opcode]script.Pointers{
		op: {
			Require:  []string{"active_player"},
			Require2: []string{"active_player2"},
			Set:      []string{"active_npc"},
			Set2:     []string{"active_npc2"},
		},
	}

	info := newTypeInfo()
	populateCommandInfoFrom(info, opmap, pointers)

	if got, want := info.Require2[3], "active_player2"; got != want {
		t.Errorf("Require2[3] = %q, want %q", got, want)
	}
	if got, want := info.Set2[3], "active_npc2"; got != want {
		t.Errorf("Set2[3] = %q, want %q", got, want)
	}
}

// TestPopulateCommandInfo_RealData smoke-tests populateCommandInfo
// against the real ScriptOpcodeMap/ScriptOpcodePointers — the same
// data BuildSymbols feeds. Asserts the 393-entry parity and one or two
// spot-checks against NAI-201's known opcodes.
func TestPopulateCommandInfo_RealData(t *testing.T) {
	info := newTypeInfo()
	populateCommandInfo(info)

	if got, want := len(info.Map), len(script.ScriptOpcodeMap); got != want {
		t.Errorf("len(Map) = %d, want %d (one entry per ScriptOpcodeMap)", got, want)
	}
	// Spot-check: opcode 0 is PUSH_CONSTANT_INT.
	if got, want := info.Map[0], "push_constant_int"; got != want {
		t.Errorf("Map[0] = %q, want %q (lowercased)", got, want)
	}
	// Ordering sanity: every populated Map key must be reachable by a TS-
	// style inclusive iteration (`for id := 0; id <= max; id++`). Add
	// sets Max = id+1 only when Max < id (strict); consecutive opcodes
	// (e.g. 10002 then 10003 ascending) leave Max == maxOp because the
	// second call sees Max == id, not Max < id. The invariant is:
	// info.Max >= maxOp (all keys reachable by the inclusive loop).
	maxOp := -1
	for op := range info.Map {
		if op > maxOp {
			maxOp = op
		}
	}
	if info.Max < maxOp {
		t.Errorf("Max = %d < max(Map keys) = %d; ascending-iteration invariant broken", info.Max, maxOp)
	}
	// Confirm sort.Slice is referenced (compile-only — prevent stale import).
	_ = sort.Slice
}

// writeConstantFile writes content to <dir>/scripts/<rel>, creating the
// scripts subdir + any intermediate dirs of rel.
func writeConstantFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, "scripts", rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
