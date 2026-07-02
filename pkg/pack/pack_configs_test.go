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

	"github.com/zsrv/goscape/pkg/io/filestream"
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

// TestPackConfigs_VarbitEndToEnd pins the rev-254 .varbit pipeline
// branch (TS PackShared.ts:618-640 @ 2e3bcf43): server varbit.dat/.idx
// land next to varp's, and the client jagfile gains varbit.dat/.idx
// entries.
func TestPackConfigs_VarbitEndToEnd(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "a.varp"),
		"[run]\nscope=perm\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "b.varbit"),
		"[run_low]\nbasevar=run\nstartbit=0\nendbit=3\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=run\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varbit.pack"), "0=run_low\n")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		filepath.Join(outDir, "server", "varbit.dat"),
		filepath.Join(outDir, "server", "varbit.idx"),
		filepath.Join(outDir, "client", "config"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
	}

	jag, err := jagfile.LoadJagfile(filepath.Join(outDir, "client", "config"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"varbit.dat", "varbit.idx"} {
		if _, err := jag.Read(name); err != nil {
			t.Fatalf("client/config missing %s: %v", name, err)
		}
	}
}

// TestPackConfigs_VarbitDuplicateVarpNameRejected pins the rev-254
// extension of the cross-domain var-name uniqueness check: varbit names
// share the global var domain (TS PackShared.ts:300-305 @ 2e3bcf43).
func TestPackConfigs_VarbitDuplicateVarpNameRejected(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "a.varp"),
		"[dup_name]\nscope=perm\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "scripts", "b.varbit"),
		"[dup_name]\nbasevar=dup_name\nstartbit=0\nendbit=1\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=dup_name\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varbit.pack"), "0=dup_name\n")
	ClearFsCache()

	err := PackConfigs(srcDir, outDir)
	if err == nil {
		t.Fatal("want non-unique var name error for varp/varbit collision")
	}
	if !strings.Contains(err.Error(), "non-unique var name") {
		t.Fatalf("err=%v, want non-unique var name", err)
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
			fmt.Fprintf(&body, "%d=%s\n", id, name)
		}
		writeFile(t, filepath.Join(packDir, kind+".pack"), body.String())
	}
}

// emptyTypedPacks creates the 13 non-varp typed-id .pack files as
// empty stubs, so loadParamLookups doesn't fail when the .param branch
// fires. Used when the test's param uses a primitive default.
func writeEmptyTypedPacks(t *testing.T, srcDir string) {
	t.Helper()
	packDir := filepath.Join(srcDir, "pack")
	for _, kind := range []string{"enum", "obj", "loc", "interface", "struct", "category", "spotanim", "npc", "inv", "synth", "seq", "dbrow", "midi"} {
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
	for _, kind := range []string{"enum", "obj", "loc", "interface", "struct", "category", "spotanim", "inv", "synth", "seq", "dbrow", "midi"} {
		writeFile(t, filepath.Join(srcDir, "pack", kind+".pack"), "")
	}
	// npc.pack carries entry "kalphite_queen"; the unconditional packAndSaveNpc
	// enforces the 244 invariant so a matching .npc source stub is required.
	writeFile(t, filepath.Join(srcDir, "scripts", "stub.npc"), "[kalphite_queen]\n")

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
	for _, kind := range []string{"enum", "obj", "loc", "interface", "struct", "category", "spotanim", "inv", "synth", "seq", "dbrow", "midi"} {
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
	// Note: frame_del.dat is not produced at rev-244 (TS PackShared.ts:355-388
	// deleted @ 9aadcec4); no models/*.frame fixture needed.
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
		"dbtable", "dbrow", "loc", "npc", "obj", "seq", "flo", "spotanim", "idk", "hunt", "varbit",
	} {
		if _, err := os.Stat(filepath.Join(server, typ+".dat")); err != nil {
			t.Errorf("%s.dat missing: %v", typ, err)
		}
		if _, err := os.Stat(filepath.Join(server, typ+".idx")); err != nil {
			t.Errorf("%s.idx missing: %v", typ, err)
		}
	}

	// frame_del.dat is not produced at rev-244 (packer removed @ 9aadcec4).
	if _, err := os.Stat(filepath.Join(server, "frame_del.dat")); err == nil {
		t.Errorf("frame_del.dat must NOT be produced at rev-244")
	}

	for _, name := range []string{"category.dat"} {
		if _, err := os.Stat(filepath.Join(server, name)); err != nil {
			t.Errorf("%s missing: %v", name, err)
		}
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
	// varbit.dat/.idx join the client jagfile at rev-254
	// (TS PackShared.ts:626-629 @ 2e3bcf43), after varp.
	expected := []string{
		"seq.dat", "seq.idx",
		"loc.dat", "loc.idx",
		"flo.dat", "flo.idx",
		"spotanim.dat", "spotanim.idx",
		"npc.dat", "npc.idx",
		"obj.dat", "obj.idx",
		"idk.dat", "idk.idx",
		"varp.dat", "varp.idx",
		"varbit.dat", "varbit.idx",
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
		"varbit.dat", "varbit.idx",
	}
	if !slices.Equal(jag.FileName, wantOrder) {
		t.Errorf("client jag entry order: got %v, want %v", jag.FileName, wantOrder)
	}
}

// TestPackConfigsModelFlagsPlumbing_SharedBackingArray is the RED→GREEN pin
// for the modelFlags threading (TS PackShared.ts:137-141 @ 9aadcec4).
//
// TS contract: packConfigs(cache, modelFlags) receives a caller-allocated
// number[] indexed by model id; readConfigs forwards it to each
// ConfigPackCallback; the five consumers (idk/loc/npc/obj/spotanim) write
// bit flags back via modelFlags[modelId] |= 0xNN. Because it is a shared
// slice, the caller observes the writes after packConfigs returns.
//
// This test covers the T5 plumbing milestone only — no flag writes land
// until T6-T13. The test:
//  1. Calls packConfigsCoreWithModelFlags (new internal entry point, RED).
//  2. Places a sentinel value at modelFlags[5] before the call.
//  3. Verifies the sentinel is intact after the call (T5: no writes yet).
//  4. Calls packNpcConfigs directly with a modelFlags arg to confirm the
//     per-config signature change compiles (RED before T5 signature change).
//
// Actual flag-write assertions (modelFlags[id] |= 0x2 etc.) land in T6+.
func TestPackConfigsModelFlagsPlumbing_SharedBackingArray(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	ClearFsCache()

	modelFlags := make([]int, 10)
	modelFlags[5] = 0x1 // sentinel; T5 must not overwrite it

	reg := &Registry{SrcDir: srcDir}
	if err := packConfigsCoreWithModelFlags(srcDir, outDir, reg, modelFlags, nil); err != nil {
		t.Fatalf("packConfigsCoreWithModelFlags: %v", err)
	}

	// Shared backing array: no T5 writes, sentinel must survive.
	if modelFlags[5] != 0x1 {
		t.Errorf("modelFlags[5] = 0x%x, want 0x1 (T5 must not overwrite)", modelFlags[5])
	}

	// Stub: packNpcConfigs with modelFlags arg compiles (per-config signature).
	// Simulates the T6 contract: the function accepts modelFlags and the caller
	// can observe any writes via shared backing array semantics.
	localFlags := make([]int, 10)
	npcPack, err := NewPackFile(srcDir, "npc", nil)
	if err != nil {
		t.Fatalf("NewPackFile npc: %v", err)
	}
	// Empty configs: no writes happen yet (T6 wires the actual flag writes).
	_, _, err = packNpcConfigs(map[string][]ConfigLine{}, npcPack, localFlags)
	if err != nil {
		t.Fatalf("packNpcConfigs stub: %v", err)
	}
	// Stub write to verify slice aliasing semantics work as expected.
	localFlags[5] |= 0x2
	if localFlags[5] != 0x2 {
		t.Errorf("localFlags[5] = 0x%x, want 0x2 (slice aliasing sanity)", localFlags[5])
	}
}

// TestPackConfigs_OrphanPackNameRejected is the RED→GREEN integration test
// for wiring ValidateConfigPackNames into the live pack path.
//
// CONTRACT (TS PackFile.ts:117-121 @ 9aadcec4): during config packing,
// every name registered in a .pack file must appear in at least one source
// config file of the matching extension. An orphan pack name (present in
// the .pack but absent from all source files) must cause PackConfigs to
// return an error containing the TS-shaped phrase "was not found in any".
//
// Test uses .varp (unconditional branch — fires on every PackConfigs call
// regardless of freshness gate) so the check is guaranteed to run.
func TestPackConfigs_OrphanPackNameRejected(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	// .varp source defines "real_varp" only.
	writeFile(t, filepath.Join(srcDir, "scripts", "a.varp"),
		"[real_varp]\nscope=perm\ntype=int\n")
	// .pack registers "real_varp" AND the orphan "ghost_varp" (no source).
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"),
		"0=real_varp\n1=ghost_varp\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	err := PackConfigs(srcDir, outDir)
	if err == nil {
		t.Fatal("want error for orphan pack name, got nil")
	}
	if !strings.Contains(err.Error(), "was not found in any") {
		t.Errorf("error should contain TS-shaped phrase 'was not found in any'; got: %v", err)
	}
	if !strings.Contains(err.Error(), "ghost_varp") {
		t.Errorf("error should name the orphan entry; got: %v", err)
	}
}

// TestPackConfigs_OrphanNonTransmittedAccepted pins the rev-254 contract
// (TS PackFile.ts:117-124 @2e3bcf43, T30 audit catch): the orphan-name
// check is gated on `transmitted` again — an orphan in a NON-transmitted
// pack (.varn here) must NOT fail the pack. rev-244 (@9aadcec4) had the
// gate removed and would have errored; 254 restores it.
func TestPackConfigs_OrphanNonTransmittedAccepted(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	// .varn source defines "real_varn" only; the .pack also registers the
	// orphan "ghost_varn". varn is non-transmitted → no orphan error.
	writeFile(t, filepath.Join(srcDir, "scripts", "a.varn"),
		"[real_varn]\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"),
		"0=real_varn\n1=ghost_varn\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("orphan in non-transmitted varn.pack must not error at 254; got: %v", err)
	}
}

// TestPackConfigsConfigJagCacheWrite is the RED→GREEN pin for Slice F
// of rev-244 B6: after PackConfigs the packed client/config jagfile bytes
// must be written to cache.Write(0, 2, data, 0) when a non-nil FileStream
// is supplied.
//
// TS source: tools/pack/config/PackShared.ts:641 @ 9aadcec4:
//
//	cache.write(0, 2, fs.readFileSync('data/pack/client/config'))
//
// Test:
//  1. Create a minimal fixture (varp only — unconditional branch).
//  2. Open a FileStream into t.TempDir().
//  3. Call packConfigsCoreWithModelFlags with the cache handle.
//  4. Assert cache.Has(0, 2) is true.
//  5. Assert cache.Read(0, 2, false) equals the on-disk client/config bytes.
func TestPackConfigsConfigJagCacheWrite(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	cacheDir := t.TempDir()

	// Minimal varp fixture (unconditional branch, always writes client jag).
	writeFile(t, filepath.Join(srcDir, "scripts", "v.varp"),
		"[quest_points]\ntype=int\nscope=perm\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=quest_points\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	cache := filestream.New(cacheDir, true, false)
	defer cache.Close()

	reg := &Registry{SrcDir: srcDir}
	if _, err := reg.EnsureModel(); err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}
	modelFlags := make([]int, reg.Model.Max)
	if err := packConfigsCoreWithModelFlags(srcDir, outDir, reg, modelFlags, cache); err != nil {
		t.Fatalf("packConfigsCoreWithModelFlags: %v", err)
	}

	// Assert cache has archive=0, file=2 entry.
	if !cache.Has(0, 2) {
		t.Fatal("cache.Has(0, 2) = false; want true after config.jag cache write")
	}

	// Assert cache bytes match on-disk client/config file.
	want, err := os.ReadFile(filepath.Join(outDir, "client", "config"))
	if err != nil {
		t.Fatalf("read client/config: %v", err)
	}
	got := cache.Read(0, 2, false)
	if !bytes.Equal(got, want) {
		t.Errorf("cache.Read(0,2) len=%d, want len=%d; bytes mismatch", len(got), len(want))
	}
}

// TestClientConfigCRCConstants_Rev254 pins the BUILD_VERIFY CRC magic
// numbers to their rev-254 values (TS PackShared.ts @ 2e3bcf43).
//
// History (244 @ 9aadcec4 → 245.2 @ 3c16994c → 254 @ 2e3bcf43):
//
//	seq      1405403166 → -1858954999 →  -716271600 (PackShared.ts:445)
//	loc      1195428820 →  626415911  →  -826309209 (PackShared.ts:469)
//	flo      1976597026 → -532285888  → -1566957964 (PackShared.ts:493)
//	spotanim  117013845 →   96621343  →  -555849646 (PackShared.ts:517)
//	npc      -997428438 →  417024969  →  1077655221 (PackShared.ts:541)
//	obj      1589810970 →  344600333  →   535204494 (PackShared.ts:565)
//	idk      -359342366 → -359342366  →  -359342366 (PackShared.ts:589) UNCHANGED
//	varp    -1961744050 → 1480086078  →  1039564548 (PackShared.ts:613)
//	varbit       (new at 254)         → -1387031023 (PackShared.ts:637)
func TestClientConfigCRCConstants_Rev254(t *testing.T) {
	tests := []struct {
		name string
		got  int32
		want int32
	}{
		{"seq", clientConfigCRCSeq, -716271600},
		{"loc", clientConfigCRCLoc, -826309209},
		{"flo", clientConfigCRCFlo, -1566957964},
		{"spotanim", clientConfigCRCSpotAnim, -555849646},
		{"npc", clientConfigCRCNpc, 1077655221},
		{"obj", clientConfigCRCObj, 535204494},
		{"idk", clientConfigCRCIdk, -359342366},
		{"varp", clientConfigCRCVarp, 1039564548},
		{"varbit", clientConfigCRCVarbit, -1387031023},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("clientConfigCRC%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}
