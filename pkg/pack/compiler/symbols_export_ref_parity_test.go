package compiler

// RETIRED GATE (rev-254 A16): TestWriteCompilerSymbols_RefParity used to
// byte-compare WriteCompilerSymbols output against the reference checkout's
// data/symbols/*.sym, activated via GOSCAPE_REF245_DIR. At the rev-254 pin
// (Engine-TS 2e3bcf43) upstream DELETED tools/pack/CompilerSymbols.ts — the
// @lostcityrs/runescript compiler holds symbols in-memory (CompilerTypeInfo)
// and the 254 reference cache contains NO data/symbols directory, so the
// upstream-parity comparison is unsatisfiable. The .sym export is now a
// documented Go-only feature (see the symbols_export.go package header:
// the symbols-export-go-only exception in symbols_export.go).
//
// What replaces it: TestWriteCompilerSymbols_SelfConsistency below — a
// synthetic-fixture format test that pins the export surface (the full
// 32-file census) and the per-family line formats (constant/pack-driven/
// registry-driven), keeping the seam clean for future per-file unit tests
// (e.g. T18's varbit.sym writer).

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// wantSymCensus is the full set of .sym files WriteCompilerSymbols emits —
// the 32 files CompilerSymbols.ts generated at 9aadcec4. The census is the
// export surface: a missing or extra file is a format regression.
var wantSymCensus = []string{
	"category.sym", "commands.sym", "component.sym", "constant.sym",
	"dbcolumn.sym", "dbrow.sym", "dbtable.sym", "enum.sym",
	"fontmetrics.sym", "hunt.sym", "idk.sym", "interface.sym",
	"inv.sym", "loc.sym", "locshape.sym", "mesanim.sym",
	"npc.sym", "npc_mode.sym", "npc_stat.sym", "obj.sym",
	"overlayinterface.sym", "param.sym", "runescript.sym", "seq.sym",
	"spotanim.sym", "stat.sym", "struct.sym", "synth.sym",
	"varn.sym", "varp.sym", "vars.sym", "writeinv.sym",
}

// TestWriteCompilerSymbols_SelfConsistency exercises WriteCompilerSymbols
// against a minimal synthetic content tree and pins:
//
//  1. the full 32-file census (no more, no less);
//  2. constant.sym: name\tvalue lines from scripts/**/*.constant;
//  3. obj.sym (pack-driven): id\tname lines from pack/obj.pack;
//  4. stat.sym (registry-driven): deterministic value-sorted
//     value\tlowername lines from objtype.PlayerStatMap;
//  5. missing pack files produce EMPTY .sym files (TS loadPack-on-absent
//     parity), not errors.
func TestWriteCompilerSymbols_SelfConsistency(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	symbolsDir := filepath.Join(t.TempDir(), "symbols")

	// Zero-count server .dat stubs for the config loaders (2-byte big-endian
	// count header = 0), plus a minimal client/config jagfile holding a
	// zero-count varp.dat (LoadVarpTypes hard-requires it). The
	// client/interface jagfile stays absent — LoadComponentTypes tolerates
	// ErrNotExist.
	serverDir := filepath.Join(outDir, "server")
	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"inv.dat", "varp.dat", "varn.dat", "vars.dat", "param.dat", "dbtable.dat"} {
		if err := os.WriteFile(filepath.Join(serverDir, f), []byte{0, 0}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	clientDir := filepath.Join(outDir, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgJag := jagfile.NewEmptyJagfile(false)
	cfgJag.Write("varp.dat", packet.NewPacket([]byte{0, 0}))
	if err := cfgJag.Save(filepath.Join(clientDir, "config")); err != nil {
		t.Fatal(err)
	}

	// scripts/<f>.constant → constant.sym input.
	scriptsDir := filepath.Join(srcDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	constSrc := "^max_int = 2147483647\n^true = 1\n"
	if err := os.WriteFile(filepath.Join(scriptsDir, "test.constant"), []byte(constSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// pack/obj.pack → obj.sym input (sparse ids to pin the gap-fill
	// behaviour: obj is in the non-skipEmpty family, so ids 1-4 emit
	// empty-name lines — matching TS CompilerSymbols obj output).
	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	objPack := "0=coins\n5=bronze_sword\n"
	if err := os.WriteFile(filepath.Join(packDir, "obj.pack"), []byte(objPack), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteCompilerSymbols(srcDir, outDir, symbolsDir); err != nil {
		t.Fatalf("WriteCompilerSymbols: %v", err)
	}

	// 1. Census.
	entries, err := os.ReadDir(symbolsDir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sym") {
			got = append(got, e.Name())
		}
	}
	sort.Strings(got)
	want := append([]string(nil), wantSymCensus...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("census: got %d .sym files %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("census[%d]: got %q, want %q", i, got[i], want[i])
		}
	}

	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(symbolsDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(b)
	}

	// 2. constant.sym format: name\tvalue\n (caret stripped).
	if got := read("constant.sym"); got != "max_int\t2147483647\ntrue\t1\n" {
		t.Errorf("constant.sym: got %q", got)
	}

	// 3. obj.sym format: id\tname\n with empty-name gap fill (non-skipEmpty).
	if got := read("obj.sym"); got != "0\tcoins\n1\t\n2\t\n3\t\n4\t\n5\tbronze_sword\n" {
		t.Errorf("obj.sym: got %q", got)
	}

	// 4. stat.sym: registry-driven, value-sorted, lowercase names. Pin the
	// first two lines + line shape rather than the whole 21-stat table.
	statLines := strings.Split(strings.TrimSuffix(read("stat.sym"), "\n"), "\n")
	if len(statLines) < 2 || statLines[0] != "0\tattack" || statLines[1] != "1\tdefence" {
		t.Errorf("stat.sym head: got %v", statLines[:min(2, len(statLines))])
	}
	for i, l := range statLines {
		if !strings.Contains(l, "\t") {
			t.Errorf("stat.sym line %d not tab-separated: %q", i, l)
		}
	}

	// 5. Missing pack file → empty .sym, not an error.
	if got := read("npc.sym"); got != "" {
		t.Errorf("npc.sym: got %q, want empty (no npc.pack in fixture)", got)
	}
}
