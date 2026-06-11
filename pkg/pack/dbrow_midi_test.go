package pack

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// --- A14: dbrow midi values + validation errors (rev-254 pin advance;
// upstream 2dc4a811) ---

// TestPackDbRowConfigs_MidiValueResolvesViaMidiPack pins that a data
// value in a midi-typed column resolves the NAME through the midi pack
// registry to its id (TS ParamConfig.ts:158-160 @2e3bcf43:
// `case ScriptVarType.MIDI: index = MidiPack.getByName(value)`).
func TestPackDbRowConfigs_MidiValueResolvesViaMidiPack(t *testing.T) {
	dbtableTypes := buildDbTableTypes(t, 0,
		[][]objtype.ScriptVarType{{objtype.ScriptVarTypeMidi}},
		[]string{"track"},
		[]uint8{0},
	)
	pf := newTestPF("dbrow", map[int]string{0: "r_midi"})
	lk := buildParamLookupsForDbRowTest(t)
	lk.midiPF = newTestPF("midi", map[int]string{219: "advance attack"})
	configs := map[string][]ConfigLine{
		"r_midi": {
			{Key: "table", Value: 0},
			{Key: "data", Value: "track,advance attack"},
		},
	}
	pd, err := packDbRowConfigs(configs, pf, dbtableTypes, lk, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x00, 0x01, // count header
		3, 1, 0, 1, 77, 1, 0, 0, 0, 219, 255, // opcode 3: type 77, P4(219)
		4, 0, 0, // opcode 4: table id 0
		250, 'r', '_', 'm', 'i', 'd', 'i', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

// TestPackDbRowConfigs_MidiUnknownNameErrors pins the TS reference-error
// shape (TS DbRowConfig.ts:160-162 @2e3bcf43).
func TestPackDbRowConfigs_MidiUnknownNameErrors(t *testing.T) {
	dbtableTypes := buildDbTableTypes(t, 0,
		[][]objtype.ScriptVarType{{objtype.ScriptVarTypeMidi}},
		[]string{"track"},
		[]uint8{0},
	)
	pf := newTestPF("dbrow", map[int]string{0: "r_mbad"})
	configs := map[string][]ConfigLine{
		"r_mbad": {
			{Key: "table", Value: 0},
			{Key: "data", Value: "track,no_such_song"},
		},
	}
	_, err := packDbRowConfigs(configs, pf, dbtableTypes, buildParamLookupsForDbRowTest(t), nil)
	if err == nil {
		t.Fatal("want error for unknown midi name, got nil")
	}
	if !strings.Contains(err.Error(), "Data invalid in row, double-check the reference exists: data=track,no_such_song") {
		t.Fatalf("err=%q, want TS `Data invalid in row...reference exists` shape", err)
	}
}

// TestPackDbRowConfigs_UnknownColumnErrors pins the unknown-column error
// added at 2e3bcf43 (TS DbRowConfig.ts:120-122: data lines naming a
// nonexistent column throw instead of being silently dropped).
func TestPackDbRowConfigs_UnknownColumnErrors(t *testing.T) {
	dbtableTypes := buildDbTableTypes(t, 0,
		[][]objtype.ScriptVarType{{objtype.ScriptVarTypeInt}},
		[]string{"col_name"},
		[]uint8{0},
	)
	pf := newTestPF("dbrow", map[int]string{0: "r_badcol"})
	configs := map[string][]ConfigLine{
		"r_badcol": {
			{Key: "table", Value: 0},
			{Key: "data", Value: "nosuchcol,1"},
		},
	}
	_, err := packDbRowConfigs(configs, pf, dbtableTypes, buildParamLookupsForDbRowTest(t), nil)
	if err == nil {
		t.Fatal("want error for unknown data column, got nil")
	}
	if !strings.Contains(err.Error(), "Data invalid in row, double-check the column exists: data=nosuchcol,1") {
		t.Fatalf("err=%q, want TS `Data invalid in row...column exists` shape", err)
	}
}

// ref254ContentDir resolves the Server254-ref content worktree:
// GOSCAPE_REF254_DIR (pointing at .../engine, content derived as a
// sibling) first, then the known local worktree path. Returns "" when
// neither exists.
func ref254ContentDir() string {
	if ref := os.Getenv("GOSCAPE_REF254_DIR"); ref != "" {
		dir := filepath.Join(ref, "..", "content")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	const local = "/home/owner/Code/github.com/LostCityRS/Server254-ref/content"
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return ""
}

// TestPackDbRow_Real254LevelupContent is the A14 empirical proof: the
// REAL Server254-ref levelup.dbtable + levelup.dbrow (the original
// rev-254 pack blocker — `column=levelup_jingle,midi`) pack end-to-end
// through goscape's dbtable → LoadDbTableTypes → dbrow pipeline with
// every [levelup_*] row resolving its midi names against the real
// content midi.pack. Before A14 this errored at the [levelup] column
// parse (unknown type "midi") and would have errored again at
// [levelup_attack]'s `data=levelup_jingle,advance attack` value lookup.
func TestPackDbRow_Real254LevelupContent(t *testing.T) {
	contentDir := ref254ContentDir()
	if contentDir == "" {
		t.Skip("Server254-ref content not available (set GOSCAPE_REF254_DIR or provision the worktree)")
	}
	levelupDir := filepath.Join(contentDir, "scripts", "levelup", "configs")
	if _, err := os.Stat(filepath.Join(levelupDir, "levelup.dbtable")); err != nil {
		t.Skipf("levelup.dbtable not found: %v", err)
	}

	// Stage just the levelup pair into a temp src tree so ReadTypedConfigs
	// sees exactly one dbtable and one dbrow file.
	srcDir := t.TempDir()
	stage := filepath.Join(srcDir, "scripts", "levelup")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"levelup.dbtable", "levelup.dbrow"} {
		b, err := os.ReadFile(filepath.Join(levelupDir, f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stage, f), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Real content pack registries for the lookups the levelup rows use:
	// interface/component (levelup_attack, levelup_attack:line1), enum
	// (levelup_unlocks_attack), and midi (advance attack) — stat resolves
	// via the hardcoded list.
	mustPF := func(name string) *PackFile {
		t.Helper()
		pf, err := NewPackFile(contentDir, name, nil)
		if err != nil {
			t.Fatalf("load %s pack: %v", name, err)
		}
		if pf.Size() == 0 {
			t.Fatalf("%s pack empty — content worktree incomplete?", name)
		}
		return pf
	}
	lk := buildParamLookupsForDbRowTest(t)
	lk.interfacePF = mustPF("interface")
	lk.enumPF = mustPF("enum")
	lk.midiPF = mustPF("midi")

	dbtablePF := mustPF("dbtable")
	dbrowPF := mustPF("dbrow")

	c := Constants{}

	// Stage 1: pack the dbtable.
	tableCfgs, err := ReadTypedConfigs(srcDir, ".dbtable", nil, parseDbTableConfig, c)
	if err != nil {
		t.Fatalf("ReadTypedConfigs(.dbtable): %v", err)
	}
	if _, ok := tableCfgs["levelup"]; !ok {
		t.Fatal("levelup.dbtable did not yield a [levelup] config block")
	}
	tablePD, err := packDbTableConfigs(tableCfgs, dbtablePF, lk, nil)
	if err != nil {
		t.Fatalf("packDbTableConfigs(levelup): %v — the A14 midi column type must pack", err)
	}

	// Stage 2: mid-pipeline DbTableType load (mirrors pack_configs.go).
	outDir := t.TempDir()
	serverOut := filepath.Join(outDir, "server")
	if err := os.MkdirAll(serverOut, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := tablePD.Save(
		filepath.Join(serverOut, "dbtable.dat"),
		filepath.Join(serverOut, "dbtable.idx"),
	); err != nil {
		t.Fatal(err)
	}
	dbtableTypes, err := objtype.LoadDbTableTypes(outDir)
	if err != nil {
		t.Fatalf("LoadDbTableTypes: %v", err)
	}

	// Sanity: the levelup table's midi columns decoded as type 77.
	levelupID := dbtablePF.GetByName("levelup")
	if levelupID < 0 {
		t.Fatal("levelup missing from dbtable.pack")
	}
	tbl := dbtableTypes.Configs[levelupID]
	if tbl == nil {
		t.Fatalf("levelup table id %d not decoded", levelupID)
	}
	jingleCol := -1
	for i, n := range tbl.ColumnNames {
		if n == "levelup_jingle" {
			jingleCol = i
		}
	}
	if jingleCol == -1 {
		t.Fatal("levelup_jingle column missing from decoded table")
	}
	if got := tbl.Types[jingleCol][0]; got != objtype.ScriptVarTypeMidi {
		t.Fatalf("levelup_jingle column type: got %d, want 77 (MIDI)", got)
	}

	// Stage 3: pack the dbrow file — the [levelup_attack] proof.
	rowCfgs, err := ReadTypedConfigs(srcDir, ".dbrow", nil, parseDbRowConfigFor(dbtablePF), c)
	if err != nil {
		t.Fatalf("ReadTypedConfigs(.dbrow): %v", err)
	}
	if _, ok := rowCfgs["levelup_attack"]; !ok {
		t.Fatal("levelup.dbrow did not yield a [levelup_attack] config block")
	}
	rowPD, err := packDbRowConfigs(rowCfgs, dbrowPF, dbtableTypes, lk, nil)
	if err != nil {
		t.Fatalf("packDbRowConfigs: %v — [levelup_attack] must no longer error (A14 proof)", err)
	}

	// The packed [levelup_attack] block must contain the resolved midi id
	// for "advance attack" (P4 big-endian) in its data.
	attackID := dbrowPF.GetByName("levelup_attack")
	if attackID < 0 {
		t.Fatal("levelup_attack missing from dbrow.pack")
	}
	midiID := lk.midiPF.GetByName("advance attack")
	if midiID < 0 {
		t.Fatal("'advance attack' missing from midi.pack")
	}
	block := packedBlockFor(t, rowPD, attackID)
	wantP4 := []byte{byte(midiID >> 24), byte(midiID >> 16), byte(midiID >> 8), byte(midiID)}
	if !bytes.Contains(block, wantP4) {
		t.Fatalf("[levelup_attack] packed block does not contain P4(%d) for 'advance attack'", midiID)
	}
}

// packedBlockFor slices the per-id body (terminator included) out of a
// PackedData by walking the idx u16 lengths past the 2-byte count
// headers in both buffers.
func packedBlockFor(t *testing.T, pd *PackedData, id int) []byte {
	t.Helper()
	idx := pd.Idx.Data
	dat := pd.Dat.Data
	off := 2 // past the dat count header
	pos := 2 // past the idx count header
	for i := 0; ; i++ {
		if pos+2 > len(idx) {
			t.Fatalf("packedBlockFor: id %d beyond idx entries", id)
		}
		n := int(idx[pos])<<8 | int(idx[pos+1])
		pos += 2
		if i == id {
			return dat[off : off+n]
		}
		off += n
	}
}
