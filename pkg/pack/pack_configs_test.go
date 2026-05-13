package pack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	// No outputs expected.
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

func TestPackConfigs_NoVarpSource_NoClientJagfileWritten(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	// .varn only. No .varp source => no client-side branch fires =>
	// client/config jagfile must NOT be written.
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
	if _, err := os.Stat(filepath.Join(outDir, "client", "config")); !os.IsNotExist(err) {
		t.Fatalf("expected client/config to NOT exist; got err=%v", err)
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

func TestPackConfigs_ParamNoSrcNoOp(t *testing.T) {
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
	// loadParamLookups must not run.

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("no .param source: PackConfigs should be no-op for param branch, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "server", "param.dat")); !os.IsNotExist(err) {
		t.Errorf("server/param.dat should NOT exist when no .param source")
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
