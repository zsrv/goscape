package pack

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackConfigs_EndToEnd_VarnAndVars(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "scripts", "n.varn"),
		"[npctier]\ntype=int\n[npchealth]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "scripts", "s.vars"),
		"[shared_quest]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=npctier\n1=npchealth\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"),
		"0=shared_quest\n")
	ClearFsCache()

	outDir := filepath.Join(dir, "out")
	if err := PackConfigs(dir, outDir); err != nil {
		t.Fatal(err)
	}

	// varn outputs exist
	for _, p := range []string{"varn.dat", "varn.idx", "vars.dat", "vars.idx"} {
		full := filepath.Join(outDir, "server", p)
		if _, err := os.Stat(full); err != nil {
			t.Fatalf("missing %s: %v", full, err)
		}
	}

	// Roundtrip through loaders.
	vnc, err := objtype.LoadVarnTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(vnc.Configs) != 2 {
		t.Fatalf("varn configs len=%d", len(vnc.Configs))
	}
	if vnc.Configs[0].DebugName != "npctier" || vnc.Configs[1].DebugName != "npchealth" {
		t.Fatalf("varn names=%q,%q", vnc.Configs[0].DebugName, vnc.Configs[1].DebugName)
	}

	vsc, err := objtype.LoadVarsTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(vsc.Configs) != 1 {
		t.Fatalf("vars configs len=%d", len(vsc.Configs))
	}
	if vsc.Configs[0].DebugName != "shared_quest" {
		t.Fatalf("vars name=%q", vsc.Configs[0].DebugName)
	}
}

func TestPackConfigs_FreshnessGateSkipsSecondRun(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "n.varn"),
		"[npctier]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=npctier\n")
	ClearFsCache()

	outDir := filepath.Join(dir, "out")
	if err := PackConfigs(dir, outDir); err != nil {
		t.Fatal(err)
	}
	datPath := filepath.Join(outDir, "server", "varn.dat")
	info1, err := os.Stat(datPath)
	if err != nil {
		t.Fatal(err)
	}
	mtime1 := info1.ModTime()

	// Sleep slightly to ensure mtime resolution can distinguish writes.
	time.Sleep(20 * time.Millisecond)

	// Re-run — ShouldBuild should return false because varn.dat is
	// fresher than the source files.
	ClearFsCache()
	if err := PackConfigs(dir, outDir); err != nil {
		t.Fatal(err)
	}
	info2, err := os.Stat(datPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info2.ModTime().Equal(mtime1) {
		t.Fatalf("file rewritten unexpectedly: mtime1=%v mtime2=%v", mtime1, info2.ModTime())
	}
}

func TestPackConfigs_NoSourceFilesReturnsNoError(t *testing.T) {
	dir := t.TempDir()
	// No scripts at all.
	ClearFsCache()
	if err := PackConfigs(dir, filepath.Join(dir, "out")); err != nil {
		t.Fatal(err)
	}
	// Freshness-gated outputs (varn, vars, enum, inv, mesanim, struct) not written;
	// unconditional branches (param, loc, npc, obj, varp + client/config jagfile)
	// are written with empty bodies.
	if _, err := os.Stat(filepath.Join(dir, "out", "server", "varn.dat")); !os.IsNotExist(err) {
		t.Fatalf("varn.dat should not exist; err=%v", err)
	}
}

func TestPackConfigs_PropagatesParseError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "n.varn"),
		"[npctier]\nbad_key=anything\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=npctier\n")
	ClearFsCache()
	err := PackConfigs(dir, filepath.Join(dir, "out"))
	if err == nil {
		t.Fatal("want parse error, got nil")
	}
}

func TestPackConfigs_VarpOnly_ProducesServerAndClientJagfile(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "test.varp"),
		"[run]\nscope=perm\ntype=int\ntransmit=yes\nclientcode=7\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=run\n")
	// Empty varn/vars packs so the orchestrator's PackFile construction
	// for the uniqueness check has all three available.
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		filepath.Join(outDir, "server", "varp.dat"),
		filepath.Join(outDir, "server", "varp.idx"),
		filepath.Join(outDir, "client", "config"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
	}
}

func TestPackConfigs_MixedVarpVarnVars(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "a.varp"),
		"[health]\nscope=perm\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "b.varn"),
		"[npctier]\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "c.vars"),
		"[shared_a]\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=health\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "0=npctier\n")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "0=shared_a\n")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		filepath.Join(outDir, "server", "varp.dat"),
		filepath.Join(outDir, "server", "varp.idx"),
		filepath.Join(outDir, "server", "varn.dat"),
		filepath.Join(outDir, "server", "varn.idx"),
		filepath.Join(outDir, "server", "vars.dat"),
		filepath.Join(outDir, "server", "vars.idx"),
		filepath.Join(outDir, "client", "config"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
	}
}

func TestPackConfigs_NoVarpSource_ClientJagfileAlwaysWritten(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	// .varn only. No .varp source. Per NAI-196-D-UNCONDITIONAL-CLIENT-PACK
	// the client jagfile is ALWAYS saved (with empty entries for the five
	// unconditional client-side branches) regardless of source presence.
	writeFile(t, filepath.Join(srcDir, "scripts", "b.varn"),
		"[npctier]\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "0=npctier\n")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "server", "varn.dat")); err != nil {
		t.Fatalf("expected varn.dat to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "client", "config")); err != nil {
		t.Fatalf("expected client/config to exist: %v", err)
	}
}

func TestPackConfigs_CrossDomainUniquenessRejection(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "a.varp"),
		"[dup_name]\nscope=perm\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "b.varn"),
		"[dup_name]\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=dup_name\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "0=dup_name\n")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	err := PackConfigs(srcDir, outDir)
	if err == nil {
		t.Fatal("want error for cross-domain duplicate name")
	}
	if !strings.Contains(err.Error(), "dup_name") {
		t.Fatalf("err=%q, want it to mention dup_name", err)
	}

	// And no server-side outputs should exist (uniqueness check runs
	// before any branch fires).
	if _, err := os.Stat(filepath.Join(outDir, "server", "varp.dat")); !os.IsNotExist(err) {
		t.Fatalf("expected no varp.dat after early reject; got err=%v", err)
	}
}

// helper: write a fixture .param + supporting .pack files into srcDir.
// Returns the srcDir for convenience. Caller must call ClearFsCache().
func setupParamFixture(t *testing.T, srcDir string, slotName, typeName, defaultVal string, extraPacks map[string]map[int]string) {
	t.Helper()
	scriptsDir := filepath.Join(srcDir, "scripts")
	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// .param source
	src := fmt.Sprintf("[%s]\ntype=%s\ndefault=%s\n", slotName, typeName, defaultVal)
	writeFile(t, filepath.Join(scriptsDir, "test.param"), src)

	// param.pack (slot 0 → slotName)
	writeFile(t, filepath.Join(packDir, "param.pack"), fmt.Sprintf("0=%s\n", slotName))

	// Default-required typed-id packs (caller supplies). varp/varn/vars
	// are always written so the up-front PackConfigs constructions don't fail.
	writeFile(t, filepath.Join(packDir, "varp.pack"), "")
	writeFile(t, filepath.Join(packDir, "varn.pack"), "")
	writeFile(t, filepath.Join(packDir, "vars.pack"), "")

	for kind, entries := range extraPacks {
		var body strings.Builder
		for id, name := range entries {
			body.WriteString(fmt.Sprintf("%d=%s\n", id, name))
		}
		writeFile(t, filepath.Join(packDir, kind+".pack"), body.String())
	}
}

// emptyTypedPacks creates the 12 non-varp typed-id .pack files as
// empty stubs, so loadParamLookups doesn't fail when the .param branch
// fires. Used when the test's param uses a primitive default.
func writeEmptyTypedPacks(t *testing.T, srcDir string) {
	t.Helper()
	packDir := filepath.Join(srcDir, "pack")
	for _, kind := range []string{"enum", "obj", "loc", "interface", "struct", "category", "spotanim", "npc", "inv", "synth", "seq", "dbrow"} {
		writeFile(t, filepath.Join(packDir, kind+".pack"), "")
	}
}

func TestPackConfigs_ParamOnly_PrimitiveDefault(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupParamFixture(t, srcDir, "health_param", "int", "100", nil)
	writeEmptyTypedPacks(t, srcDir)

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "server", "param.dat")); err != nil {
		t.Errorf("server/param.dat missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "server", "param.idx")); err != nil {
		t.Errorf("server/param.idx missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "client", "config")); err != nil {
		t.Errorf("client/config jagfile missing: %v", err)
	}
}

func TestPackConfigs_ParamWithTypedDefault(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupParamFixture(t, srcDir, "boss_param", "npc", "kalphite_queen", map[string]map[int]string{
		"npc": {42: "kalphite_queen"},
	})
	// Stub the remaining 11 typed packs so loadParamLookups doesn't fail.
	for _, kind := range []string{"enum", "obj", "loc", "interface", "struct", "category", "spotanim", "inv", "synth", "seq", "dbrow"} {
		writeFile(t, filepath.Join(srcDir, "pack", kind+".pack"), "")
	}

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}
	// Round-trip via LoadParamTypes confirms DefaultInt=42.
	ptc, err := objtype.LoadParamTypes(outDir)
	if err != nil {
		t.Fatalf("LoadParamTypes: %v", err)
	}
	if len(ptc.Configs) != 1 {
		t.Fatalf("got %d configs, want 1", len(ptc.Configs))
	}
	if got, want := ptc.Configs[0].DefaultInt, int32(42); got != want {
		t.Errorf("DefaultInt: got %d, want %d", got, want)
	}
	if got, want := ptc.Configs[0].DebugName, "boss_param"; got != want {
		t.Errorf("DebugName: got %q, want %q", got, want)
	}
}

func TestPackConfigs_ParamMissingTypedPackFile(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupParamFixture(t, srcDir, "x", "npc", "kalphite_queen", nil)
	// Do NOT write npc.pack — NewPackFile succeeds with an empty registry
	// (missing files are not errors), but packParamConfigs will fail when
	// lookupParamValue cannot resolve "kalphite_queen" against the empty npcPF.

	err := PackConfigs(srcDir, outDir)
	if err == nil {
		t.Fatalf("missing npc.pack: want error, got nil")
	}
	if !strings.Contains(err.Error(), "npc") {
		t.Errorf("error should mention npc: got %v", err)
	}
}

func TestPackConfigs_ParamNoSrc_WritesEmptyOutputs(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()
	// Only var-domain .pack files; no .param source.
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	// Intentionally omit param.pack and all 12 typed-id .pack files.
	// loadParamLookups still runs (NAI-196-D-UNCONDITIONAL-CLIENT-PACK)
	// but with empty registries — pack succeeds and writes empty
	// .dat/.idx pairs.

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}
	// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: .param always runs, so
	// server/param.dat exists (empty when no source).
	if _, err := os.Stat(filepath.Join(outDir, "server", "param.dat")); err != nil {
		t.Errorf("server/param.dat should exist (empty): %v", err)
	}
}

func TestPackConfigs_ParamUnknownTypedDefault(t *testing.T) {
	ClearFsCache()
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupParamFixture(t, srcDir, "boss", "npc", "nonexistent_npc", map[string]map[int]string{
		"npc": {0: "kalphite_queen"}, // doesn't include nonexistent_npc
	})
	for _, kind := range []string{"enum", "obj", "loc", "interface", "struct", "category", "spotanim", "inv", "synth", "seq", "dbrow"} {
		writeFile(t, filepath.Join(srcDir, "pack", kind+".pack"), "")
	}

	err := PackConfigs(srcDir, outDir)
	if err == nil {
		t.Fatalf("unknown npc default: want error, got nil")
	}
	if !strings.Contains(err.Error(), "boss") {
		t.Errorf("error should name the param: got %v", err)
	}
}

func TestPackConfigs_TwentyConfigsLand(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	scripts := filepath.Join(srcDir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=quest_points\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "0=npc_state\n")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "0=server_clock\n")
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "0=damage\n")
	writeFile(t, filepath.Join(srcDir, "pack", "enum.pack"), "0=days\n")
	writeFile(t, filepath.Join(srcDir, "pack", "inv.pack"), "0=bank\n")
	writeFile(t, filepath.Join(srcDir, "pack", "mesanim.pack"), "0=hero_chat\n")
	writeFile(t, filepath.Join(srcDir, "pack", "struct.pack"), "0=goblin_loot\n")
	writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "0=table\n")
	writeFile(t, filepath.Join(srcDir, "pack", "npc.pack"), "0=rat\n")
	writeFile(t, filepath.Join(srcDir, "pack", "obj.pack"), "0=egg\n")
	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=walk\n")
	writeFile(t, filepath.Join(srcDir, "pack", "flo.pack"), "0=red\n")
	writeFile(t, filepath.Join(srcDir, "pack", "spotanim.pack"), "0=flame\n")
	writeFile(t, filepath.Join(srcDir, "pack", "idk.pack"), "0=man_hair_default\n")
	writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "0=cat_one\n")
	writeFile(t, filepath.Join(srcDir, "pack", "hunt.pack"), "0=h_off\n")
	writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "anim.pack"), "0=walk\n")
	walkFrame := []byte{
		0xab,                                           // del[0] sentinel
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // trailer: head=0,tran1=0,tran2=0,delLen=1
	}
	modelsDir := filepath.Join(srcDir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "walk.frame"), walkFrame, 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(srcDir, "pack", "dbtable.pack"), "0=t_simple\n")
	writeFile(t, filepath.Join(srcDir, "pack", "dbrow.pack"), "0=r_one\n")
	for _, p := range []string{"interface", "synth"} {
		writeFile(t, filepath.Join(srcDir, "pack", p+".pack"), "")
	}

	writeFile(t, filepath.Join(scripts, "v.varp"),
		"[quest_points]\ntype=int\nscope=perm\n")
	writeFile(t, filepath.Join(scripts, "n.varn"),
		"[npc_state]\ntype=int\n")
	writeFile(t, filepath.Join(scripts, "s.vars"),
		"[server_clock]\ntype=int\n")
	writeFile(t, filepath.Join(scripts, "p.param"),
		"[damage]\ntype=int\ndefault=10\n")
	writeFile(t, filepath.Join(scripts, "e.enum"),
		"[days]\ninputtype=int\noutputtype=int\ndefault=0\nval=1,1\n")
	writeFile(t, filepath.Join(scripts, "i.inv"),
		"[bank]\nscope=shared\nsize=1\n")
	writeFile(t, filepath.Join(scripts, "m.mesanim"),
		"[hero_chat]\nlen1=walk\n")
	writeFile(t, filepath.Join(scripts, "x.struct"),
		"[goblin_loot]\nparam=damage,7\n")
	writeFile(t, filepath.Join(scripts, "l.loc"),
		"[table]\nname=Table\n")
	writeFile(t, filepath.Join(scripts, "k.npc"),
		"[rat]\nname=Rat\n")
	writeFile(t, filepath.Join(scripts, "o.obj"),
		"[egg]\nname=Egg\n")
	writeFile(t, filepath.Join(scripts, "q.seq"),
		"[walk]\nloops=1\n")
	writeFile(t, filepath.Join(scripts, "f.flo"),
		"[red]\ncolour=0xFF0000\n")
	writeFile(t, filepath.Join(scripts, "a.spotanim"),
		"[flame]\nangle=180\n")
	writeFile(t, filepath.Join(scripts, "d.idk"),
		"[man_hair_default]\ntype=man_hair\n")
	writeFile(t, filepath.Join(scripts, "t.dbtable"),
		"[t_simple]\ncolumn=score,int\n")
	writeFile(t, filepath.Join(scripts, "r.dbrow"),
		"[r_one]\ntable=t_simple\ndata=score,7\n")
	writeFile(t, filepath.Join(scripts, "h.hunt"),
		"[h_off]\ntype=off\n")

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}

	server := filepath.Join(outDir, "server")
	for _, typ := range []string{
		"varp", "varn", "vars", "param", "enum", "inv", "mesanim", "struct",
		"dbtable", "dbrow", "loc", "npc", "obj", "seq", "flo", "spotanim", "idk", "hunt",
	} {
		if _, err := os.Stat(filepath.Join(server, typ+".dat")); err != nil {
			t.Errorf("%s.dat missing: %v", typ, err)
		}
		if _, err := os.Stat(filepath.Join(server, typ+".idx")); err != nil {
			t.Errorf("%s.idx missing: %v", typ, err)
		}
	}

	for _, name := range []string{"category.dat", "frame_del.dat"} {
		if _, err := os.Stat(filepath.Join(server, name)); err != nil {
			t.Errorf("%s missing: %v", name, err)
		}
	}

	frameDel, err := os.ReadFile(filepath.Join(server, "frame_del.dat"))
	if err != nil {
		t.Fatalf("read frame_del.dat: %v", err)
	}
	if len(frameDel) != 1 || frameDel[0] != 0xab {
		t.Fatalf("frame_del.dat: got % x, want ab", frameDel)
	}

	cat, err := os.ReadFile(filepath.Join(server, "category.dat"))
	if err != nil {
		t.Fatalf("read category.dat: %v", err)
	}
	wantCat := []byte{0x00, 0x01, 0x01, 'c', 'a', 't', '_', 'o', 'n', 'e', 0x0a, 0x00}
	if !bytes.Equal(cat, wantCat) {
		t.Fatalf("category.dat mismatch:\ngot  % x\nwant % x", cat, wantCat)
	}

	jagPath := filepath.Join(outDir, "client", "config")
	if _, err := os.Stat(jagPath); err != nil {
		t.Fatalf("client/config jagfile missing: %v", err)
	}
	jag, err := jagfile.LoadJagfile(jagPath)
	if err != nil {
		t.Fatalf("LoadJagfile: %v", err)
	}
	expected := []string{
		"seq.dat", "seq.idx",
		"loc.dat", "loc.idx",
		"flo.dat", "flo.idx",
		"spotanim.dat", "spotanim.idx",
		"npc.dat", "npc.idx",
		"obj.dat", "obj.idx",
		"idk.dat", "idk.idx",
		"varp.dat", "varp.idx",
	}
	for _, name := range expected {
		if _, err := jag.Read(name); err != nil {
			t.Errorf("client jagfile missing entry %q: %v", name, err)
		}
	}
	if jag.FileCount != len(expected) {
		t.Errorf("client jagfile has %d entries, want %d (names=%v)", jag.FileCount, len(expected), jag.FileName)
	}
	wantOrder := []string{
		"seq.dat", "seq.idx",
		"loc.dat", "loc.idx",
		"flo.dat", "flo.idx",
		"spotanim.dat", "spotanim.idx",
		"npc.dat", "npc.idx",
		"obj.dat", "obj.idx",
		"idk.dat", "idk.idx",
		"varp.dat", "varp.idx",
	}
	if !slices.Equal(jag.FileName, wantOrder) {
		t.Errorf("client jag entry order: got %v, want %v", jag.FileName, wantOrder)
	}
}

// TestRegisterMissingNames_RegistersMissingConfigNames pins the half of
// TS validateConfigPack that goscape had not ported: names found in the
// config sources but absent from the .pack index are registered with fresh
// ids and the index is written back to disk.
//
// Content gitignores these ten indexes ("generated for the server only"),
// so a fresh clone has none of them and every struct `param=` lookup fails
// until they are generated.
//
// TS source: tools/pack/PackFile.ts:validateConfigPack (register loop at
// :144-148, pack.save() at :183).
func TestRegisterMissingNames_RegistersMissingConfigNames(t *testing.T) {
	srcDir := t.TempDir()
	writeScript(t, filepath.Join(srcDir, "scripts"), "a.param",
		"[alpha]\ntype=int\n\n[beta]\ntype=int\n")
	ClearFsCache()

	pf := &PackFile{
		Type:     "param",
		SrcDir:   srcDir,
		Pack:     map[int]string{},
		Names:    map[string]struct{}{},
		NameToID: map[string]int{},
	}
	if err := registerMissingNames(srcDir, pf, ".param", false); err != nil {
		t.Fatalf("registerMissingNames: %v", err)
	}

	// Ids follow crawl order, not map order — see the determinism test.
	if got := pf.NameToID["alpha"]; got != 0 {
		t.Errorf("alpha id = %d, want 0", got)
	}
	if got := pf.NameToID["beta"]; got != 1 {
		t.Errorf("beta id = %d, want 1", got)
	}

	data, err := os.ReadFile(filepath.Join(srcDir, "pack", "param.pack"))
	if err != nil {
		t.Fatalf("index was not written: %v", err)
	}
	if want := "0=alpha\n1=beta\n"; string(data) != want {
		t.Errorf("param.pack = %q, want %q", string(data), want)
	}
}

// TestRegisterMissingNames_DeterministicAcrossRuns pins that ids come from
// the ORDERED crawl, not from ranging the cfgs map. Go randomises map
// iteration, so registering from cfgs keys would assign different ids on
// different runs — and since these ids are written into param.dat, two
// packs of identical content would produce different bytes, breaking both
// the release digest and the byte-parity gates.
func TestRegisterMissingNames_DeterministicAcrossRuns(t *testing.T) {
	// Enough names that a map-order bug is overwhelmingly likely to show.
	body := ""
	for _, n := range []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8"} {
		body += "[" + n + "]\ntype=int\n\n"
	}
	var first string
	for run := range 5 {
		srcDir := t.TempDir()
		writeScript(t, filepath.Join(srcDir, "scripts"), "a.param", body)
		ClearFsCache()
		pf := &PackFile{
			Type: "param", SrcDir: srcDir,
			Pack: map[int]string{}, Names: map[string]struct{}{}, NameToID: map[string]int{},
		}
		if err := registerMissingNames(srcDir, pf, ".param", false); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		data, err := os.ReadFile(filepath.Join(srcDir, "pack", "param.pack"))
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if run == 0 {
			first = string(data)
			if want := "0=p1\n1=p2\n2=p3\n3=p4\n4=p5\n5=p6\n6=p7\n7=p8\n"; first != want {
				t.Fatalf("param.pack = %q, want crawl order %q", first, want)
			}
			continue
		}
		if string(data) != first {
			t.Fatalf("run %d differs from run 0:\n got %q\nwant %q", run, string(data), first)
		}
	}
}

// TestParamLookupsSeeRegistryRegistrations closes NAI-194-D-PACKFILE-SINGLETONS.
//
// TS keeps EnumPack/ObjPack/etc. as module-level singletons, so a name
// registered anywhere is instantly visible to every consumer. goscape used to
// build paramLookups from its own NewPackFile instances, which snapshot the
// .pack files from disk at construction time. That was invisible while the
// indexes always existed before packing began — but once the packer generates
// them mid-run, a lookup built earlier stays empty and cross-family references
// fail with "double-check the reference exists".
//
// The property that matters is not identity but visibility: a registration
// made after the lookups are built must be observable through them.
func TestParamLookupsSeeRegistryRegistrations(t *testing.T) {
	srcDir := t.TempDir()
	reg := &Registry{SrcDir: srcDir}

	lk, err := loadParamLookups(reg)
	if err != nil {
		t.Fatalf("loadParamLookups: %v", err)
	}

	enumPF, err := reg.EnsureEnum()
	if err != nil {
		t.Fatalf("EnsureEnum: %v", err)
	}
	enumPF.Register(7, "levelup_unlocks_attack")
	enumPF.RefreshNames()

	if got := lk.enumPF.GetByName("levelup_unlocks_attack"); got != 7 {
		t.Errorf("lookup sees id %d, want 7 — paramLookups is not sharing the registry's enum pack", got)
	}
}

// TestGenerateConfigPackIndexes_RunsAheadOfPacking pins the pre-pass.
//
// goscape registers a family's names while packing that family, but families
// reference each other: npc resolves `huntmode=` through the hunt index, and
// npc packs first. TS gets away with registering lazily because each pack is
// validated on first access; goscape's order is fixed, so the indexes must all
// exist before any family packs.
func TestGenerateConfigPackIndexes_RunsAheadOfPacking(t *testing.T) {
	srcDir := t.TempDir()
	scripts := filepath.Join(srcDir, "scripts")
	writeScript(t, scripts, "a.hunt", "[duck_hunt]\ntype=player\n")
	writeScript(t, scripts, "a.param", "[pyre_level]\ntype=int\n")
	ClearFsCache()

	reg := &Registry{SrcDir: srcDir}
	if err := generateConfigPackIndexes(srcDir, reg); err != nil {
		t.Fatalf("generateConfigPackIndexes: %v", err)
	}

	for _, tc := range []struct{ file, name string }{
		{"hunt.pack", "duck_hunt"},
		{"param.pack", "pyre_level"},
	} {
		data, err := os.ReadFile(filepath.Join(srcDir, "pack", tc.file))
		if err != nil {
			t.Errorf("%s not written: %v", tc.file, err)
			continue
		}
		if !strings.Contains(string(data), "="+tc.name) {
			t.Errorf("%s = %q, want it to contain %q", tc.file, string(data), tc.name)
		}
	}
}

// TestGenerateConfigPackIndexes_ScriptPack pins the script index, which the
// RuneScript compiler loads as its "runescript" symbol table
// (compiler/symbols.go: loadOrFail("script")). Without it every script symbol
// is unknown and the compiler emits an empty script.dat — a silent 6 MB hole
// in the pack rather than an error.
//
// Script names keep their brackets, unlike every other config index.
//
// TS source: tools/pack/PackFile.ts:regenScriptPack (crawlConfigNames('.rs2', true)).
func TestGenerateConfigPackIndexes_ScriptPack(t *testing.T) {
	srcDir := t.TempDir()
	writeScript(t, filepath.Join(srcDir, "scripts"), "a.rs2",
		"[proc,alpha]\nreturn;\n\n[label,beta]\nreturn;\n")
	ClearFsCache()

	reg := &Registry{SrcDir: srcDir}
	if err := generateConfigPackIndexes(srcDir, reg); err != nil {
		t.Fatalf("generateConfigPackIndexes: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(srcDir, "pack", "script.pack"))
	if err != nil {
		t.Fatalf("script.pack not written: %v", err)
	}
	if want := "0=[proc,alpha]\n1=[label,beta]\n"; string(data) != want {
		t.Errorf("script.pack = %q, want %q", string(data), want)
	}
}
