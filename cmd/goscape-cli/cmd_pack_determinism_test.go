package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunPack_DeterministicOutput packs the same source twice into
// separate temp output dirs and asserts byte-identical server/script.dat
// and server/script.idx.
//
// The bug under test: pkg/pack/compiler/symbol/table.go findAllInto
// iterates a Go map without sorting, and two callers in
// pkg/pack/compiler/semantics/type_checking_expr.go
// (visitGameVariableExpression, resolveSymbol) break on the first match
// of FindAll(name). When symbols of DIFFERENT KINDS share a name (e.g.
// an obj and an inv both named "apple", or a varp and a varn both named
// "gold"), the caller picks a different symbol between runs, and that
// shuffle shows up as different operand bytes in compiled scripts.
//
// The fixture below deliberately seeds many cross-kind name collisions
// (obj↔inv, obj↔varp, varp↔dbtable, etc.) and references them from
// procs to maximize the chance of the shuffle reaching script bytecode.
//
// The deterministic regression test is
// TestSymbolTable_FindAll_DeterministicOrder in
// pkg/pack/compiler/symbol/table_test.go — that is the hard gate. This
// higher-level test catches the property end-to-end through the packer
// pipeline.
func TestRunPack_DeterministicOutput(t *testing.T) {
	src := t.TempDir()
	seedDeterminismFixture(t, src)
	rawDir := makeRawDir(t, src)

	out1 := t.TempDir()
	seedWorldmapJagStub(t, out1) // 254: keep the packMaps worldmap gate closed
	var stderr1 bytes.Buffer
	if code := runPack([]string{"--src-dir", src, "--out-dir", out1, "--raw-dir", rawDir}, io.Discard, &stderr1); code != 0 {
		t.Fatalf("run 1: runPack returned %d; stderr=%s", code, stderr1.String())
	}
	out2 := t.TempDir()
	seedWorldmapJagStub(t, out2) // 254: keep the packMaps worldmap gate closed
	var stderr2 bytes.Buffer
	if code := runPack([]string{"--src-dir", src, "--out-dir", out2, "--raw-dir", rawDir}, io.Discard, &stderr2); code != 0 {
		t.Fatalf("run 2: runPack returned %d; stderr=%s", code, stderr2.String())
	}

	for _, name := range []string{"server/script.dat", "server/script.idx"} {
		a, err := os.ReadFile(filepath.Join(out1, name))
		if err != nil {
			t.Fatalf("read out1 %s: %v", name, err)
		}
		b, err := os.ReadFile(filepath.Join(out2, name))
		if err != nil {
			t.Fatalf("read out2 %s: %v", name, err)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s differs between runs at byte offset %d (len out1=%d, out2=%d)",
				name, firstDiffOffsetDeterminism(a, b), len(a), len(b))
		}
	}
}

func firstDiffOffsetDeterminism(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}

// seedDeterminismFixture writes a srcDir layout with deliberate cross-kind
// name collisions to exercise FindAll iteration order.
//
// Var-domain triplet (varp / varn / vars) keeps unique names within itself
// because pkg/pack/pack_configs.go:checkVarNameUniqueness rejects collisions
// across that trio. Collisions are placed between var-domain ↔ obj/inv/dbtable
// (which are separate SymbolKindBasic entries keyed by type representation
// in the symbol table — same name, different outer key, different
// FindAll-iteration slot).
func seedDeterminismFixture(t *testing.T, dir string) {
	t.Helper()

	// Names that will appear in MULTIPLE symbol kinds. Each of these will
	// produce >=2 BasicSymbols in the table all under the same FindAll key.
	collidingNames := []string{
		"gold", "silver", "apple", "stone", "bone", "fish",
		"bow", "ring", "rune", "ore",
	}

	// --- objs: include every colliding name + 6 unique fillers ---
	objNames := append([]string{}, collidingNames...)
	objNames = append(objNames,
		"bronze_sword", "iron_sword", "steel_sword",
		"mithril_sword", "adamant_sword", "dragon_sword")
	var objSrc, objPack strings.Builder
	for i, n := range objNames {
		fmt.Fprintf(&objSrc, "[%s]\nname=%s\n", n, strings.ReplaceAll(n, "_", " "))
		fmt.Fprintf(&objPack, "%d=%s\n", i, n)
	}
	writeFile(t, filepath.Join(dir, "scripts", "o.obj"), objSrc.String())
	writeFile(t, filepath.Join(dir, "pack", "obj.pack"), objPack.String())

	// --- varps (game variables) reusing first 6 colliding names ---
	// varp + obj share names → BasicSymbol cross-kind collision.
	varpNames := collidingNames[:6]
	var varpSrc, varpPack strings.Builder
	for i, n := range varpNames {
		fmt.Fprintf(&varpSrc, "[%s]\ntype=int\n", n)
		fmt.Fprintf(&varpPack, "%d=%s\n", i, n)
	}
	writeFile(t, filepath.Join(dir, "scripts", "p.varp"), varpSrc.String())
	writeFile(t, filepath.Join(dir, "pack", "varp.pack"), varpPack.String())

	// --- invs reusing colliding names 6..9 + fillers (collide with obj only) ---
	invNames := []string{
		collidingNames[6], collidingNames[7], collidingNames[8], collidingNames[9],
		"backpack", "worn", "bank", "trade",
	}
	var invSrc, invPack strings.Builder
	for i, n := range invNames {
		fmt.Fprintf(&invSrc, "[%s]\n", n)
		fmt.Fprintf(&invPack, "%d=%s\n", i, n)
	}
	writeFile(t, filepath.Join(dir, "scripts", "i.inv"), invSrc.String())
	writeFile(t, filepath.Join(dir, "pack", "inv.pack"), invPack.String())

	// --- varns: disjoint from varp+vars per var-trio uniqueness check ---
	varnNames := []string{
		"npc_hp", "npc_combat_level", "npc_aggro", "npc_speed",
		"npc_state", "npc_mood",
	}
	var varnSrc, varnPack strings.Builder
	for i, n := range varnNames {
		fmt.Fprintf(&varnSrc, "[%s]\ntype=int\n", n)
		fmt.Fprintf(&varnPack, "%d=%s\n", i, n)
	}
	writeFile(t, filepath.Join(dir, "scripts", "n.varn"), varnSrc.String())
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"), varnPack.String())

	// --- vars: disjoint from varp+varn ---
	varsNames := []string{
		"shared_xp", "shared_kills", "shared_quest",
		"shared_event", "shared_progress",
	}
	var varsSrc, varsPack strings.Builder
	for i, n := range varsNames {
		fmt.Fprintf(&varsSrc, "[%s]\ntype=int\n", n)
		fmt.Fprintf(&varsPack, "%d=%s\n", i, n)
	}
	writeFile(t, filepath.Join(dir, "scripts", "s.vars"), varsSrc.String())
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"), varsPack.String())

	// --- dbtables ---
	dbNames := []string{"records", "quests", "achievements", "stats"}
	var dbSrc, dbPack strings.Builder
	for i, n := range dbNames {
		fmt.Fprintf(&dbSrc, "[%s]\n", n)
		fmt.Fprintf(&dbPack, "%d=%s\n", i, n)
	}
	writeFile(t, filepath.Join(dir, "scripts", "d.dbtable"), dbSrc.String())
	writeFile(t, filepath.Join(dir, "pack", "dbtable.pack"), dbPack.String())

	// --- procs that reference colliding names ---
	// These procs cross-call (~name) to broaden the GOSUB surface so any
	// symbol-id shuffle ripples into operand bytes.
	procs := []struct {
		name string
		body string
	}{
		{"alpha", "~bravo;\n~charlie;\n~delta;\n~echo;\nreturn;"},
		{"bravo", "~charlie;\n~delta;\n~echo;\n~foxtrot;\nreturn;"},
		{"charlie", "~delta;\n~echo;\n~foxtrot;\n~golf;\nreturn;"},
		{"delta", "~echo;\n~foxtrot;\n~golf;\n~hotel;\nreturn;"},
		{"echo", "~foxtrot;\n~golf;\n~hotel;\n~india;\nreturn;"},
		{"foxtrot", "~golf;\n~hotel;\n~india;\n~juliet;\nreturn;"},
		{"golf", "~hotel;\n~india;\n~juliet;\n~alpha;\nreturn;"},
		{"hotel", "~india;\n~juliet;\n~alpha;\n~bravo;\nreturn;"},
		{"india", "~juliet;\n~alpha;\n~bravo;\n~charlie;\nreturn;"},
		{"juliet", "~alpha;\n~bravo;\n~charlie;\n~delta;\nreturn;"},
	}
	var scriptPack strings.Builder
	for i, p := range procs {
		writeFile(t, filepath.Join(dir, "scripts", p.name+".rs2"),
			fmt.Sprintf("[proc,%s]\n%s\n", p.name, p.body))
		fmt.Fprintf(&scriptPack, "%d=[proc,%s]\n", i, p.name)
	}

	// --- aux procs that read/write game-vars whose names collide with objs ---
	// %gold etc. flow through visitGameVariableExpression → FindAll(name) →
	// first-match-wins. With varp gold + obj gold both present, the first
	// BasicSymbol returned by FindAll is non-deterministic.
	for i, gv := range varpNames {
		name := fmt.Sprintf("read_%s", gv)
		body := fmt.Sprintf("[proc,%s]()(int)\nreturn(%%%s);\n", name, gv)
		writeFile(t, filepath.Join(dir, "scripts", name+".rs2"), body)
		fmt.Fprintf(&scriptPack, "%d=[proc,%s]\n", len(procs)+i, name)
	}
	for i, gv := range varnNames {
		name := fmt.Sprintf("read_%s", gv)
		body := fmt.Sprintf("[proc,%s](npc $n)(int)\nreturn(%%%s);\n", name, gv)
		writeFile(t, filepath.Join(dir, "scripts", name+".rs2"), body)
		fmt.Fprintf(&scriptPack, "%d=[proc,%s]\n", len(procs)+len(varpNames)+i, name)
	}

	writeFile(t, filepath.Join(dir, "pack", "script.pack"), scriptPack.String())

	// VersionList: versionlist.Pack reads <srcDir>/maps/free2play.csv.
	// Rev-244 B6: required by PackAll pipeline.
	if err := os.MkdirAll(filepath.Join(dir, "maps"), 0o755); err != nil {
		t.Fatalf("MkdirAll maps: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "maps", "free2play.csv"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile free2play.csv: %v", err)
	}
}
