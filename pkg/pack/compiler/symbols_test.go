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
		"A": "quoted",     // both-sided quotes stripped
		"B": "unquoted",   // no quotes, unchanged
		"C": "\"mismatch", // open-only, unchanged
		"D": "mismatch\"", // close-only, unchanged
		"E": "in\"middle", // input "in"middle" — outer pair stripped, inner quote retained
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
// (ScriptVarType.ts:28-83) and goscape's existing
// objtype.ParamType.GetType() (paramtype.go:105).
//
// pack-media-compiler-1 (2026-05-28 audit): the last 7 entries
// (autoint/varp/player_uid/npc_uid/npc_stat/idkit/dbrow) pin cases
// goscape's switch was missing pre-fix, where every miss returned
// "unknown" → emitted "unknown" in compiler symbol output.
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
		// pack-media-compiler-1: TS ScriptVarType.getType (ScriptVarType.ts:64-79)
		// covers these 7 cases; goscape's switch omitted them so the compiler
		// emitted the literal "unknown" for varp/dbrow/uid/etc. symbols.
		{objtype.ScriptVarTypeAutoInt, "autoint"},
		{objtype.ScriptVarTypeVarp, "varp"},
		{objtype.ScriptVarTypePlayerUid, "player_uid"},
		{objtype.ScriptVarTypeNpcUid, "npc_uid"},
		{objtype.ScriptVarTypeNpcStat, "npc_stat"},
		{objtype.ScriptVarTypeIdkit, "idkit"},
		{objtype.ScriptVarTypeDbrow, "dbrow"},
		{objtype.ScriptVarTypeMidi, "midi"}, // 254 (2dc4a811)
	}
	for _, c := range cases {
		if got := scriptVarTypeName(c.t); got != c.want {
			t.Errorf("scriptVarTypeName(%d) = %q, want %q (TS ScriptVarType.getType)", c.t, got, c.want)
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

// TestPopulateCommandInfoFrom_Corrupt2OverwritesCorrupt pins TS-faithful
// behavior at TS Compiler.ts:147: the corrupt2 arm assigns to
// commandInfo.corrupt[opcode], overwriting the value written by the
// corrupt arm one line above. commandInfo.corrupt2 is never populated.
func TestPopulateCommandInfoFrom_Corrupt2OverwritesCorrupt(t *testing.T) {
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

	if got, want := info.Corrupt[7], "corrupt2_x,corrupt2_y"; got != want {
		t.Errorf("Corrupt[7] = %q, want %q (TS-faithful: corrupt2 arm overwrites Corrupt)", got, want)
	}
	if _, present := info.Corrupt2[7]; present {
		t.Errorf("Corrupt2[7] present; want absent (TS-faithful: Corrupt2 is never written)")
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

// TestPopulateInterfaceOverlay_PrefersComName pins CompilerSymbols.ts:135
// (Engine-TS@9aadcec4) — when com.comName is non-empty, that name takes
// precedence over the componentInfo.Map[id] fallback.
func TestPopulateInterfaceOverlay_PrefersComName(t *testing.T) {
	componentInfo := newTypeInfo()
	componentInfo.Add(5, "fallback_name", true)

	components := &objtype.ComponentTypeConfigs{
		Configs: make([]*objtype.ComponentType, 10),
	}
	components.Configs[5] = &objtype.ComponentType{
		ComName: "preferred_name",
		Overlay: false,
	}

	interfaceInfo := newTypeInfo()
	overlayInfo := newTypeInfo()
	populateInterfaceOverlay(componentInfo, interfaceInfo, overlayInfo, components)

	if got, want := interfaceInfo.Map[5], "preferred_name"; got != want {
		t.Errorf("interface.Map[5] = %q, want %q (com.comName must override fallback)", got, want)
	}
}

// TestPopulateInterfaceOverlay_FallsBackToComponentInfoMap pins the
// `com.comName || componentInfo.map[id]` fallback at TS Compiler.ts:225.
func TestPopulateInterfaceOverlay_FallsBackToComponentInfoMap(t *testing.T) {
	componentInfo := newTypeInfo()
	componentInfo.Add(3, "from_pack_file", true)

	components := &objtype.ComponentTypeConfigs{
		Configs: make([]*objtype.ComponentType, 10),
	}
	components.Configs[3] = &objtype.ComponentType{
		ComName: "",
		Overlay: false,
	}

	interfaceInfo := newTypeInfo()
	overlayInfo := newTypeInfo()
	populateInterfaceOverlay(componentInfo, interfaceInfo, overlayInfo, components)

	if got, want := interfaceInfo.Map[3], "from_pack_file"; got != want {
		t.Errorf("interface.Map[3] = %q, want %q (fallback to componentInfo)", got, want)
	}
}

// TestPopulateInterfaceOverlay_OverlayOnlyOnTrue pins CompilerSymbols.ts:139-148
// (Engine-TS@9aadcec4, rev244-b6-cs-delta1):
//   - overlayInfo gets the entry only when com.Overlay == true
//   - overlay entries are ALSO added to interfaceInfo (temporary duplication)
func TestPopulateInterfaceOverlay_OverlayOnlyOnTrue(t *testing.T) {
	componentInfo := newTypeInfo()
	componentInfo.Add(1, "a", true)
	componentInfo.Add(2, "b", true)

	components := &objtype.ComponentTypeConfigs{
		Configs: make([]*objtype.ComponentType, 10),
	}
	components.Configs[1] = &objtype.ComponentType{ComName: "a", Overlay: true}
	components.Configs[2] = &objtype.ComponentType{ComName: "b", Overlay: false}

	interfaceInfo := newTypeInfo()
	overlayInfo := newTypeInfo()
	populateInterfaceOverlay(componentInfo, interfaceInfo, overlayInfo, components)

	if _, ok := overlayInfo.Map[1]; !ok {
		t.Errorf("overlay.Map[1]: missing; want present (overlay=true)")
	}
	if _, ok := overlayInfo.Map[2]; ok {
		t.Errorf("overlay.Map[2]: present; want absent (overlay=false)")
	}
	// Overlay entries are temporarily duplicated into interfaceInfo (CompilerSymbols.ts:145-148).
	if _, ok := interfaceInfo.Map[1]; !ok {
		t.Errorf("interface.Map[1]: missing; want present (overlay dup into interface)")
	}
}

// TestPopulateInterfaceOverlay_ColonNameGoesToComponentOnly pins the NEW
// three-way split (rev244-b6-cs-delta1, CompilerSymbols.ts:137-138):
// entries whose resolved name contains ':' are NOT added to interfaceInfo
// or overlayInfo; they remain in componentInfo (already loaded from
// interface.pack).
func TestPopulateInterfaceOverlay_ColonNameGoesToComponentOnly(t *testing.T) {
	componentInfo := newTypeInfo()
	componentInfo.Add(1, "inv:com_0", true) // colon name — component only

	components := &objtype.ComponentTypeConfigs{
		Configs: make([]*objtype.ComponentType, 10),
	}
	components.Configs[1] = &objtype.ComponentType{ComName: "inv:com_0", Overlay: false}

	interfaceInfo := newTypeInfo()
	overlayInfo := newTypeInfo()
	populateInterfaceOverlay(componentInfo, interfaceInfo, overlayInfo, components)

	if _, ok := interfaceInfo.Map[1]; ok {
		t.Errorf("interface.Map[1]: present; want absent (colon-name → component only, rev244-b6-cs-delta1)")
	}
	if _, ok := overlayInfo.Map[1]; ok {
		t.Errorf("overlay.Map[1]: present; want absent (colon-name → component only)")
	}
}

// TestPopulateInterfaceOverlay_NilConfig_FallsBack pins the defensive
// fallback path: when loaders.comp is empty (standalone `goscape-cli
// compile` against a fresh dataPackDir), populateInterfaceOverlay
// populates interfaceInfo from componentInfo.Map alone using the base
// name. Permanent pin for NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO.
func TestPopulateInterfaceOverlay_NilConfig_FallsBack(t *testing.T) {
	componentInfo := newTypeInfo()
	componentInfo.Add(4, "x", true)

	components := &objtype.ComponentTypeConfigs{
		Configs: make([]*objtype.ComponentType, 10),
	}
	// Configs[4] left nil.

	interfaceInfo := newTypeInfo()
	overlayInfo := newTypeInfo()
	populateInterfaceOverlay(componentInfo, interfaceInfo, overlayInfo, components)

	if got, want := interfaceInfo.Map[4], "x"; got != want {
		t.Errorf("interface.Map[4] = %q, want %q (fallback to componentInfo.Map base name)", got, want)
	}
	if _, ok := overlayInfo.Map[4]; ok {
		t.Errorf("overlay.Map[4]: present; want absent (no Overlay flag without cache)")
	}
}

// TestPopulateInterfaceOverlay_SkipsIdsAbsentFromComponentInfo pins TS
// Compiler.ts:216-218 — ids without a componentInfo.Map[id] entry get
// skipped (inclusive <= Max loop with map-presence guard).
func TestPopulateInterfaceOverlay_SkipsIdsAbsentFromComponentInfo(t *testing.T) {
	componentInfo := newTypeInfo()
	componentInfo.Add(0, "zero", true)
	componentInfo.Add(5, "five", true) // Max becomes 6; ids 1-4 absent

	components := &objtype.ComponentTypeConfigs{
		Configs: make([]*objtype.ComponentType, 10),
	}
	// Populate Configs[2] even though componentInfo skips id 2.
	components.Configs[2] = &objtype.ComponentType{ComName: "two", Overlay: true}

	interfaceInfo := newTypeInfo()
	overlayInfo := newTypeInfo()
	populateInterfaceOverlay(componentInfo, interfaceInfo, overlayInfo, components)

	if _, ok := interfaceInfo.Map[2]; ok {
		t.Errorf("interface.Map[2]: present; want absent (componentInfo.Map[2] missing → skip)")
	}
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

// TestBuildSymbolsCore_AllCategoriesPresent pins §5.8: the returned map
// has exactly 34 keys covering the TS symbols dict at Compiler.ts:337-376
// @ 2e3bcf43 ("varbit" joined at rev-254; "midi" at Compiler.ts:199+368,
// bridged at T23 — plain pack/midi.pack load, no enrichment).
func TestBuildSymbolsCore_AllCategoriesPresent(t *testing.T) {
	dir := t.TempDir()
	// Seed a few .pack files so Load returns non-nil TypeInfo; absent ones
	// also return non-nil (Load is silent-on-missing).
	writePackFile(t, dir, "npc.pack", "0=goblin\n")
	writePackFile(t, dir, "obj.pack", "0=bronze_sword\n")

	loaders := emptyConfigLoaders()

	symbols, err := buildSymbolsCore(dir, loaders)
	if err != nil {
		t.Fatalf("buildSymbolsCore: %v", err)
	}

	wantKeys := []string{
		"command", "constant", "npc", "obj", "inv", "writeinv", "seq",
		"idk", "spotanim", "loc", "component", "interface",
		"overlayinterface", "varp", "varbit", "varn", "vars", "param", "struct",
		"enum", "hunt", "mesanim", "synth", "category", "runescript",
		"dbtable", "dbcolumn", "dbrow", "midi", "stat", "npc_stat",
		"npc_mode", "fontmetrics", "locshape",
	}
	if got, want := len(symbols), len(wantKeys); got != want {
		t.Errorf("len(symbols) = %d, want %d", got, want)
	}
	for _, k := range wantKeys {
		if _, ok := symbols[k]; !ok {
			t.Errorf("symbols[%q]: missing", k)
		}
		if symbols[k] == nil {
			t.Errorf("symbols[%q]: nil *TypeInfo (every category must be non-nil)", k)
		}
	}
}

// TestBuildSymbolsCore_StaticArraysPopulated pins the LoadArray inputs
// (fontmetrics, locshape) against TS Compiler.ts:303-328.
func TestBuildSymbolsCore_StaticArraysPopulated(t *testing.T) {
	dir := t.TempDir()
	loaders := emptyConfigLoaders()
	symbols, err := buildSymbolsCore(dir, loaders)
	if err != nil {
		t.Fatalf("buildSymbolsCore: %v", err)
	}

	// fontmetrics — 4 entries.
	fm := symbols["fontmetrics"]
	wantFm := []string{"p11_full", "p12_full", "b12_full", "q8_full"}
	if got, want := len(fm.Map), len(wantFm); got != want {
		t.Errorf("len(fontmetrics.Map) = %d, want %d", got, want)
	}
	for i, name := range wantFm {
		if got := fm.Map[i]; got != name {
			t.Errorf("fontmetrics.Map[%d] = %q, want %q", i, got, name)
		}
	}

	// locshape — 23 entries.
	ls := symbols["locshape"]
	if got, want := len(ls.Map), 23; got != want {
		t.Errorf("len(locshape.Map) = %d, want %d", got, want)
	}
	if got, want := ls.Map[0], "wall_straight"; got != want {
		t.Errorf("locshape.Map[0] = %q, want %q", got, want)
	}
	if got, want := ls.Map[22], "grounddecor"; got != want {
		t.Errorf("locshape.Map[22] = %q, want %q (final entry)", got, want)
	}
}

// TestBuildSymbolsCore_MetaMaps pins TS Compiler.ts:300-302 — the three
// LoadMap(valueAsKey=true) outputs (stat, npc_stat, npc_mode).
func TestBuildSymbolsCore_MetaMaps(t *testing.T) {
	dir := t.TempDir()
	loaders := emptyConfigLoaders()
	symbols, err := buildSymbolsCore(dir, loaders)
	if err != nil {
		t.Fatalf("buildSymbolsCore: %v", err)
	}

	// stat: PlayerStatMap with valueAsKey=true → NameMap["0"] = "attack" etc.
	stat := symbols["stat"]
	if got, want := stat.NameMap["0"], "attack"; got != want {
		t.Errorf("stat.NameMap[\"0\"] = %q, want %q", got, want)
	}

	// npc_stat: NpcStatMap with valueAsKey=true → NameMap["0"] = "attack" etc.
	npcStat := symbols["npc_stat"]
	if got, want := npcStat.NameMap["0"], "attack"; got != want {
		t.Errorf("npc_stat.NameMap[\"0\"] = %q, want %q", got, want)
	}

	// npc_mode: NpcModeMap with valueAsKey=true → NameMap["-1"] = "null" etc.
	npcMode := symbols["npc_mode"]
	if got, want := npcMode.NameMap["-1"], "null"; got != want {
		t.Errorf("npc_mode.NameMap[\"-1\"] = %q, want %q", got, want)
	}
}

// TestBuildSymbolsCore_ConstantsLoaded pins TS Compiler.ts:152-174 ->
// LoadRecords flow.
func TestBuildSymbolsCore_ConstantsLoaded(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "^FOO=bar\nBAZ=\"quoted\"\n")

	loaders := emptyConfigLoaders()
	symbols, err := buildSymbolsCore(dir, loaders)
	if err != nil {
		t.Fatalf("buildSymbolsCore: %v", err)
	}

	c := symbols["constant"]
	if got, want := c.NameMap["FOO"], "bar"; got != want {
		t.Errorf("constant.NameMap[FOO] = %q, want %q", got, want)
	}
	if got, want := c.NameMap["BAZ"], "quoted"; got != want {
		t.Errorf("constant.NameMap[BAZ] = %q, want %q (quote-stripped)", got, want)
	}
}

// TestBuildSymbolsCore_PackFilesConsumed pins TS Compiler.ts:177-200 — each
// of the 22 .pack files lands as the named TypeInfo.
func TestBuildSymbolsCore_PackFilesConsumed(t *testing.T) {
	dir := t.TempDir()
	writePackFile(t, dir, "inv.pack", "5=secret_stash\n")
	writePackFile(t, dir, "script.pack", "99=some_script\n")

	loaders := emptyConfigLoaders()
	symbols, err := buildSymbolsCore(dir, loaders)
	if err != nil {
		t.Fatalf("buildSymbolsCore: %v", err)
	}

	if got, want := symbols["inv"].Map[5], "secret_stash"; got != want {
		t.Errorf("inv.Map[5] = %q, want %q", got, want)
	}
	// script.pack → "runescript" symbol key.
	if got, want := symbols["runescript"].Map[99], "some_script"; got != want {
		t.Errorf("runescript.Map[99] = %q, want %q (script.pack → \"runescript\" key)", got, want)
	}
}

// TestBuildSymbolsCore_CommandPopulated pins the commandInfo step at
// integration level (T5 covers the unit-level behaviour).
func TestBuildSymbolsCore_CommandPopulated(t *testing.T) {
	dir := t.TempDir()
	loaders := emptyConfigLoaders()
	symbols, err := buildSymbolsCore(dir, loaders)
	if err != nil {
		t.Fatalf("buildSymbolsCore: %v", err)
	}

	cmd := symbols["command"]
	if got, want := len(cmd.Map), len(script.ScriptOpcodeMap); got != want {
		t.Errorf("len(command.Map) = %d, want %d", got, want)
	}
}

// emptyConfigLoaders returns a *configLoaders with every field set to an
// empty (but non-nil) Configs slice — exercises the
// inclusive-<=Max-with-presence-guard pattern for the case where no
// entity-type data is available.
func emptyConfigLoaders() *configLoaders {
	return &configLoaders{
		inv:     &objtype.InvTypeConfigs{Configs: []*objtype.InvType{}},
		comp:    &objtype.ComponentTypeConfigs{Configs: []*objtype.ComponentType{}},
		varp:    &objtype.VarpTypeConfigs{Configs: []*objtype.VarPlayerType{}},
		varbit:  &objtype.VarBitTypeConfigs{Configs: []*objtype.VarBitType{}},
		varn:    &objtype.VarnTypeConfigs{Configs: []*objtype.VarNpcType{}},
		varsCfg: &objtype.VarsTypeConfigs{Configs: []*objtype.VarSharedType{}},
		param:   &objtype.ParamTypeConfigs{Configs: []*objtype.ParamType{}},
		dbtable: &objtype.DbTableTypeConfigs{Configs: []*objtype.DbTableType{}},
	}
}

// writePackFile writes content to <dir>/pack/<name> creating the pack/
// subdir.
func writePackFile(t *testing.T, dir, name, content string) {
	t.Helper()
	packDir := filepath.Join(dir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPopulateDbColumns_SingleTypeColumn pins TS Compiler.ts:285-287 for
// a 1-type column: one primary entry, no tuple entries, Max unchanged.
func TestPopulateDbColumns_SingleTypeColumn(t *testing.T) {
	tables := &objtype.DbTableTypeConfigs{
		Configs: []*objtype.DbTableType{
			{
				ConfigType:  objtype.ConfigType{ID: 1, DebugName: "tbl1"},
				ColumnNames: []string{"col0"},
				Types:       [][]objtype.ScriptVarType{{objtype.ScriptVarTypeInt}},
			},
		},
	}

	info := newTypeInfo()
	populateDbColumns(info, tables)

	primaryID := (1 << 12) | (0 << 4) // = 4096
	if got, want := info.Map[primaryID], "tbl1:col0"; got != want {
		t.Errorf("Map[%d] = %q, want %q", primaryID, got, want)
	}
	if got, want := info.VarType[primaryID], "int"; got != want {
		t.Errorf("VarType[%d] = %q, want %q", primaryID, got, want)
	}
	if info.Max != -1 {
		t.Errorf("Max = %d, want -1 (updateMax=false)", info.Max)
	}
	// No tuple entries.
	for id := range info.Map {
		if id != primaryID {
			t.Errorf("unexpected Map entry at %d: %q (single-type column should produce no tuples)", id, info.Map[id])
		}
	}
}

// TestPopulateDbColumns_MultiTypeColumn pins TS Compiler.ts:289-294 for a
// 2-type column: primary entry with comma-joined vartypes + 2 tuple
// entries with single vartype each.
func TestPopulateDbColumns_MultiTypeColumn(t *testing.T) {
	tables := &objtype.DbTableTypeConfigs{
		Configs: []*objtype.DbTableType{
			{
				ConfigType:  objtype.ConfigType{ID: 1, DebugName: "tbl1"},
				ColumnNames: []string{"col0"},
				Types: [][]objtype.ScriptVarType{
					{objtype.ScriptVarTypeInt, objtype.ScriptVarTypeObj},
				},
			},
		},
	}

	info := newTypeInfo()
	populateDbColumns(info, tables)

	primary := (1 << 12) | (0 << 4) // 4096
	tup1 := primary | 1             // 4097
	tup2 := primary | 2             // 4098

	if got, want := info.Map[primary], "tbl1:col0"; got != want {
		t.Errorf("Map[primary=%d] = %q, want %q", primary, got, want)
	}
	if got, want := info.VarType[primary], "int,obj"; got != want {
		t.Errorf("VarType[primary] = %q, want %q", got, want)
	}
	if got, want := info.Map[tup1], "tbl1:col0:0"; got != want {
		t.Errorf("Map[tup1=%d] = %q, want %q", tup1, got, want)
	}
	if got, want := info.VarType[tup1], "int"; got != want {
		t.Errorf("VarType[tup1] = %q, want %q", got, want)
	}
	if got, want := info.Map[tup2], "tbl1:col0:1"; got != want {
		t.Errorf("Map[tup2=%d] = %q, want %q", tup2, got, want)
	}
	if got, want := info.VarType[tup2], "obj"; got != want {
		t.Errorf("VarType[tup2] = %q, want %q", got, want)
	}
}

// TestPopulateDbColumns_BitfieldEncoding pins the exact ID arithmetic for
// non-trivial (table, column) values. table=2, column=5, types=[STRING]
// → primary id = (2<<12) | (5<<4) = 8192 | 80 = 8272.
func TestPopulateDbColumns_BitfieldEncoding(t *testing.T) {
	tables := &objtype.DbTableTypeConfigs{
		Configs: []*objtype.DbTableType{
			nil, nil,
			{
				ConfigType:  objtype.ConfigType{ID: 2, DebugName: "tbl2"},
				ColumnNames: []string{"a", "b", "c", "d", "e", "col5"},
				Types: [][]objtype.ScriptVarType{
					nil, nil, nil, nil, nil,
					{objtype.ScriptVarTypeString},
				},
			},
		},
	}

	info := newTypeInfo()
	populateDbColumns(info, tables)

	want := (2 << 12) | (5 << 4)
	if want != 8272 {
		t.Fatalf("want arithmetic wrong: (2<<12)|(5<<4) = %d, expected 8272", want)
	}
	if got, ok := info.Map[want]; !ok || got != "tbl2:col5" {
		t.Errorf("Map[%d] = %q (present=%v), want \"tbl2:col5\"", want, got, ok)
	}
}

// TestPopulateDbColumns_NilColumnTypes pins: a column whose Types[col]
// is nil (no `code 1` block written) is skipped.
func TestPopulateDbColumns_NilColumnTypes(t *testing.T) {
	tables := &objtype.DbTableTypeConfigs{
		Configs: []*objtype.DbTableType{
			{
				ConfigType:  objtype.ConfigType{ID: 1, DebugName: "tbl1"},
				ColumnNames: []string{"present", "absent"},
				Types: [][]objtype.ScriptVarType{
					{objtype.ScriptVarTypeInt},
					nil,
				},
			},
		},
	}

	info := newTypeInfo()
	populateDbColumns(info, tables)

	presentID := (1 << 12) | (0 << 4)
	if _, ok := info.Map[presentID]; !ok {
		t.Errorf("Map[%d]: missing; want present (column 0 has types)", presentID)
	}
	absentID := (1 << 12) | (1 << 4)
	if _, ok := info.Map[absentID]; ok {
		t.Errorf("Map[%d]: present; want absent (column 1 has nil types)", absentID)
	}
}

// TestPopulateDbColumns_SkipsNilTable pins: a nil entry in tables.Configs
// is skipped (TS Compiler.ts:277 inclusive-loop with `Map[id]` guard;
// goscape mirrors by guarding on Configs[id] != nil).
func TestPopulateDbColumns_SkipsNilTable(t *testing.T) {
	tables := &objtype.DbTableTypeConfigs{
		Configs: []*objtype.DbTableType{
			nil,
			{
				ConfigType:  objtype.ConfigType{ID: 1, DebugName: "tbl1"},
				ColumnNames: []string{"col0"},
				Types:       [][]objtype.ScriptVarType{{objtype.ScriptVarTypeInt}},
			},
		},
	}

	info := newTypeInfo()
	populateDbColumns(info, tables)

	// Table 0 is nil → no entries.
	id0 := (0 << 12) | (0 << 4) // = 0
	if _, ok := info.Map[id0]; ok {
		t.Errorf("Map[0]: present; want absent (Configs[0] nil)")
	}
	// Table 1 → present.
	id1 := (1 << 12) | (0 << 4)
	if _, ok := info.Map[id1]; !ok {
		t.Errorf("Map[%d]: missing; want present", id1)
	}
}

// TestEnrichWriteinvInfo pins TS Compiler.ts:203-212 — writeinv.Protect[id]
// reflects the loaded InvType.Protect (which defaults to true, falsified
// by op-code-7 in the .dat).
func TestEnrichWriteinvInfo(t *testing.T) {
	writeinv := newTypeInfo()
	writeinv.Add(0, "main_inv", true)
	writeinv.Add(1, "worn_inv", true)

	invs := &objtype.InvTypeConfigs{
		Configs: []*objtype.InvType{
			{ConfigType: objtype.ConfigType{ID: 0}, Protect: true},
			{ConfigType: objtype.ConfigType{ID: 1}, Protect: false},
		},
	}

	enrichWriteinvInfo(writeinv, invs)

	if got, want := writeinv.Protect[0], true; got != want {
		t.Errorf("Protect[0] = %v, want %v", got, want)
	}
	if got, want := writeinv.Protect[1], false; got != want {
		t.Errorf("Protect[1] = %v, want %v", got, want)
	}
}

// TestEnrichVarpInfo pins TS Compiler.ts:234-243 — varp.VarType[id] and
// varp.Protect[id] reflect VarPlayerType.Type / VarPlayerType.Protect.
func TestEnrichVarpInfo(t *testing.T) {
	varp := newTypeInfo()
	varp.Add(0, "v0", true)

	configs := &objtype.VarpTypeConfigs{
		Configs: []*objtype.VarPlayerType{
			{
				ConfigType: objtype.ConfigType{ID: 0},
				Type:       objtype.ScriptVarTypeInt,
				Protect:    true,
			},
		},
	}

	enrichVarpInfo(varp, configs)

	if got, want := varp.VarType[0], "int"; got != want {
		t.Errorf("VarType[0] = %q, want %q", got, want)
	}
	if got, want := varp.Protect[0], true; got != want {
		t.Errorf("Protect[0] = %v, want %v", got, want)
	}
}

// TestEnrichVarbitInfo pins TS Compiler.ts:240-250 @ 2e3bcf43 (NEW at
// rev-254) — varbit.VarType[id] and varbit.Protect[id] come from the
// BASE VAR's VarPlayerType (type + protect inheritance through
// varbit.basevar), NOT from the varbit itself.
func TestEnrichVarbitInfo(t *testing.T) {
	varbit := newTypeInfo()
	varbit.Add(0, "vb0", true)

	varbits := &objtype.VarBitTypeConfigs{
		Configs: []*objtype.VarBitType{
			{
				ConfigType: objtype.ConfigType{ID: 0},
				Basevar:    2,
				Startbit:   0,
				Endbit:     3,
			},
		},
	}
	varps := &objtype.VarpTypeConfigs{
		Configs: make([]*objtype.VarPlayerType, 3),
	}
	varps.Configs[2] = &objtype.VarPlayerType{
		ConfigType: objtype.ConfigType{ID: 2},
		Type:       objtype.ScriptVarTypeBoolean,
		Protect:    true,
	}

	enrichVarbitInfo(varbit, varbits, varps)

	if got, want := varbit.VarType[0], "boolean"; got != want {
		t.Errorf("VarType[0] = %q, want %q (must inherit from basevar)", got, want)
	}
	if got, want := varbit.Protect[0], true; got != want {
		t.Errorf("Protect[0] = %v, want %v (must inherit from basevar)", got, want)
	}
}

// TestEnrichVarbitInfo_OOBBasevarSkipped pins the Go-side defensive
// guard: a varbit whose Basevar is out of range (e.g. the -1 default
// from NewVarBitType) is skipped instead of panicking — TS would throw
// on the undefined VarPlayerType.get access; this state is unreachable
// through the packer (requiredProperties enforce basevar).
func TestEnrichVarbitInfo_OOBBasevarSkipped(t *testing.T) {
	varbit := newTypeInfo()
	varbit.Add(0, "vb0", true)

	varbits := &objtype.VarBitTypeConfigs{
		Configs: []*objtype.VarBitType{
			{ConfigType: objtype.ConfigType{ID: 0}, Basevar: -1},
		},
	}
	varps := &objtype.VarpTypeConfigs{Configs: []*objtype.VarPlayerType{}}

	enrichVarbitInfo(varbit, varbits, varps)

	if _, present := varbit.VarType[0]; present {
		t.Errorf("VarType[0]: present; want absent (OOB basevar → skip)")
	}
}

// TestEnrichVarnInfo_HappyPath pins NEW CompilerSymbols.ts:167-178 semantics
// (rev244-b6-cs-delta3): a varn id present in varn.pack is enriched
// regardless of whether a varp exists at the same id slot.
func TestEnrichVarnInfo_HappyPath(t *testing.T) {
	varn := newTypeInfo()
	varn.Add(0, "n0", true)

	configs := &objtype.VarnTypeConfigs{
		Configs: []*objtype.VarNpcType{
			{ConfigType: objtype.ConfigType{ID: 0}, Type: objtype.ScriptVarTypeString},
		},
	}

	enrichVarnInfo(varn, configs)

	if got, want := varn.VarType[0], "string"; got != want {
		t.Errorf("VarType[0] = %q, want %q", got, want)
	}
}

// TestEnrichVarnInfo_VarnOnlyIDEnriched pins NEW semantics (rev244-b6-cs-delta3):
// a varn id with no varp at the same id IS enriched (unlike OLD Compiler.ts:247
// which checked varpInfo.map[id] and would have skipped it).
func TestEnrichVarnInfo_VarnOnlyIDEnriched(t *testing.T) {
	varn := newTypeInfo()
	varn.Add(7, "lonely_varn", true) // id=7 — no varp at this id

	configs := &objtype.VarnTypeConfigs{
		Configs: make([]*objtype.VarNpcType, 10),
	}
	configs.Configs[7] = &objtype.VarNpcType{
		ConfigType: objtype.ConfigType{ID: 7},
		Type:       objtype.ScriptVarTypeBoolean,
	}

	enrichVarnInfo(varn, configs)

	if got, present := varn.VarType[7]; !present {
		t.Errorf("VarType[7]: absent; want present (NEW: varn-only ids ARE enriched, rev244-b6-cs-delta3)")
	} else if got != "boolean" {
		t.Errorf("VarType[7] = %q, want %q", got, "boolean")
	}
}

// TestEnrichVarnInfo_AbsentFromPackSkipped pins that ids absent from
// varn.pack (no entry in varnInfo.Map) are NOT enriched even if a
// VarNpcType config exists for that id.
func TestEnrichVarnInfo_AbsentFromPackSkipped(t *testing.T) {
	varn := newTypeInfo()
	// id=0 is present in varn.pack; id=3 is absent.
	varn.Add(0, "n0", true)

	configs := &objtype.VarnTypeConfigs{
		Configs: make([]*objtype.VarNpcType, 10),
	}
	configs.Configs[0] = &objtype.VarNpcType{ConfigType: objtype.ConfigType{ID: 0}, Type: objtype.ScriptVarTypeInt}
	configs.Configs[3] = &objtype.VarNpcType{ConfigType: objtype.ConfigType{ID: 3}, Type: objtype.ScriptVarTypeBoolean}

	enrichVarnInfo(varn, configs)

	if _, present := varn.VarType[3]; present {
		t.Errorf("VarType[3]: present; want absent (id=3 not in varn.pack → skip)")
	}
}

// TestEnrichVarsInfo pins TS Compiler.ts:255-263.
func TestEnrichVarsInfo(t *testing.T) {
	vars := newTypeInfo()
	vars.Add(0, "s0", true)

	configs := &objtype.VarsTypeConfigs{
		Configs: []*objtype.VarSharedType{
			{ConfigType: objtype.ConfigType{ID: 0}, Type: objtype.ScriptVarTypeCoord},
		},
	}

	enrichVarsInfo(vars, configs)

	if got, want := vars.VarType[0], "coord"; got != want {
		t.Errorf("VarType[0] = %q, want %q", got, want)
	}
}

// TestEnrichParamInfo pins TS Compiler.ts:265-273 — uses ParamType.GetType()
// directly (rather than scriptVarTypeName) to honour the existing
// instance method.
func TestEnrichParamInfo(t *testing.T) {
	param := newTypeInfo()
	param.Add(0, "p0", true)

	configs := &objtype.ParamTypeConfigs{
		Configs: []*objtype.ParamType{
			{
				ConfigType: objtype.ConfigType{ID: 0},
				Type:       objtype.ScriptVarTypeNamedObj,
			},
		},
	}

	enrichParamInfo(param, configs)

	if got, want := param.VarType[0], "namedobj"; got != want {
		t.Errorf("VarType[0] = %q, want %q", got, want)
	}
}

// TestEnrich_SkipsIdsAbsentFromMap pins the inclusive-<=Max + Map-presence
// guard on every enricher (TS Compiler.ts:206-208 etc.).
func TestEnrich_SkipsIdsAbsentFromMap(t *testing.T) {
	// One enricher exercises the pattern; the others share the same
	// loop shape.
	writeinv := newTypeInfo()
	writeinv.Add(0, "present", true)
	writeinv.Add(5, "present", true) // Max=6; ids 1..4 absent

	invs := &objtype.InvTypeConfigs{
		Configs: make([]*objtype.InvType, 10),
	}
	for i := range invs.Configs {
		invs.Configs[i] = &objtype.InvType{
			ConfigType: objtype.ConfigType{ID: i},
			Protect:    true,
		}
	}

	enrichWriteinvInfo(writeinv, invs)

	for i := 1; i <= 4; i++ {
		if _, ok := writeinv.Protect[i]; ok {
			t.Errorf("Protect[%d]: present; want absent (id missing from Map)", i)
		}
	}
}
