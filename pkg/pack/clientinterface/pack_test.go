package clientinterface

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pack"
)

func TestPack_BytePinned(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	scriptsDir := filepath.Join(src, "scripts")
	packDir := filepath.Join(src, "pack")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// interface.pack registers two ids:
	//   0 = mychat (the root interface)
	//   1 = mychat:layer1 (a child layer)
	if err := os.WriteFile(filepath.Join(packDir, "interface.pack"), []byte("0=mychat\n1=mychat:layer1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "interface.order"), []byte("0\n1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Empty .pack files for cross-domain registries.
	for _, name := range []string{"obj", "model", "seq", "varp"} {
		if err := os.WriteFile(filepath.Join(packDir, name+".pack"), []byte(""), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	body := "[layer1]\ntype=layer\nwidth=100\nheight=100\n"
	if err := os.WriteFile(filepath.Join(scriptsDir, "mychat.if"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile mychat.if: %v", err)
	}

	reg := &pack.Registry{SrcDir: src}
	out := filepath.Join(tmp, "out")
	if err := Pack(reg, src, out, nil, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	jag, err := jagfile.LoadJagfile(filepath.Join(out, "client", "interface"))
	if err != nil {
		t.Fatalf("LoadJagfile: %v", err)
	}
	if _, err := jag.Read("data"); err != nil {
		t.Errorf("Read \"data\": %v", err)
	}

	serverPath := filepath.Join(out, "server", "interface.dat")
	info, err := os.Stat(serverPath)
	if err != nil {
		t.Errorf("Stat %q: %v", serverPath, err)
	} else if info.Size() == 0 {
		t.Errorf("%q is empty", serverPath)
	}
}

// TestPack_StringFieldsRoundTrip pins that string fields written by
// clientinterface.Pack are readable by objtype.LoadComponentTypes —
// i.e. writer-pjstr matches reader-gjstr (both LF=10 per TS Packet.ts).
//
// Surfaced during smoke-pack T7 real-Content shakedown 2026-05-17:
// writer used PJStrNUL (NUL terminator) which made gjstr (LF) read past
// the intended string boundary, misaligning all subsequent fields and
// eventually causing G2 to panic on EOF in parseComponentTypes' script
// loop. Synthetic fixtures with no string-bearing components had hidden
// the bug.
func TestPack_StringFieldsRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	scriptsDir := filepath.Join(src, "scripts")
	packDir := filepath.Join(src, "pack")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := os.WriteFile(filepath.Join(packDir, "interface.pack"),
		[]byte("0=mychat\n1=mychat:label1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "interface.order"),
		[]byte("0\n1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.order: %v", err)
	}
	for _, name := range []string{"obj", "model", "seq", "varp"} {
		if err := os.WriteFile(filepath.Join(packDir, name+".pack"), []byte(""), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	// comType=text uses pjstr for text+activetext; pins both terminators.
	body := "[label1]\ntype=text\nwidth=100\nheight=20\ntext=hello world\nactivetext=goodbye\n"
	if err := os.WriteFile(filepath.Join(scriptsDir, "mychat.if"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile mychat.if: %v", err)
	}

	reg := &pack.Registry{SrcDir: src}
	out := filepath.Join(tmp, "out")
	if err := Pack(reg, src, out, nil, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	configs, err := objtype.LoadComponentTypes(out)
	if err != nil {
		t.Fatalf("LoadComponentTypes: %v (panic-EOF if terminator mismatch)", err)
	}
	if configs == nil || len(configs.Configs) < 2 {
		t.Fatalf("LoadComponentTypes returned %d configs, want >=2", len(configs.Configs))
	}
	label := configs.Configs[1]
	if label == nil {
		t.Fatalf("configs[1] is nil")
	}
	if label.Text != "hello world" {
		t.Errorf("label.Text = %q, want %q", label.Text, "hello world")
	}
	if label.ActiveText != "goodbye" {
		t.Errorf("label.ActiveText = %q, want %q", label.ActiveText, "goodbye")
	}
}

// TestAtoiOr0 pins parseInt-equivalent semantics for 0x/0X-prefixed hex,
// the form Content uses for colour/activecolour/overcolour. Without the
// hex branch, strconv.Atoi returns 0 for every "0xRRGGBB", zeroing all
// label/rect/paragraph colour P4s and diverging from TS at offset ~3430
// of the client/interface payload on real Content.
func TestAtoiOr0(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"0", 0},
		{"123", 123},
		{"-7", -7},
		{"0xFF0000", 0xFF0000},
		{"0xFFFFFF", 0xFFFFFF},
		{"0X00FF00", 0x00FF00},
		{"0xff0000", 0xff0000},
		{"0xFF000000", -16777216}, // 0xFF000000 wraps to int32 -16777216
		{"abc", 0},
		{"0xZZ", 0},
	}
	for _, tt := range tests {
		if got := atoiOr0(tt.in); got != tt.want {
			t.Errorf("atoiOr0(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// TestPack_245_2_InvSwappable pins rev-245.2: swappable bool byte is emitted
// directly after usable in TYPE_INVENTORY (TS PackShared.ts:444, @3c16994c).
// Pack → LoadComponentTypes round-trip: swappable=yes → Swappable=true.
func TestPack_245_2_InvSwappable(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	scriptsDir := filepath.Join(src, "scripts")
	packDir := filepath.Join(src, "pack")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "interface.pack"),
		[]byte("0=myinv\n1=myinv:bag\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "interface.order"),
		[]byte("0\n1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.order: %v", err)
	}
	for _, name := range []string{"obj", "model", "seq", "varp"} {
		if err := os.WriteFile(filepath.Join(packDir, name+".pack"), []byte(""), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	// comType=inv with swappable=yes — pins 245.2 swappable byte.
	body := "[bag]\ntype=inv\nwidth=100\nheight=100\nswappable=yes\n"
	if err := os.WriteFile(filepath.Join(scriptsDir, "myinv.if"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile myinv.if: %v", err)
	}

	reg := &pack.Registry{SrcDir: src}
	out := filepath.Join(tmp, "out")
	if err := Pack(reg, src, out, nil, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	configs, err := objtype.LoadComponentTypes(out)
	if err != nil {
		t.Fatalf("LoadComponentTypes: %v", err)
	}
	if configs == nil || len(configs.Configs) < 2 {
		t.Fatalf("LoadComponentTypes returned %d configs, want >=2", len(configs.Configs))
	}
	bag := configs.Configs[1]
	if bag == nil {
		t.Fatalf("configs[1] is nil")
	}
	if !bag.Swappable {
		t.Errorf("Swappable: want true (TS PackShared.ts:444 @3c16994c, new in 245.2)")
	}
}

// TestPack_245_2_ActiveOverColour pins rev-245.2: activeovercolour p4 is
// emitted directly after overcolour for TYPE_RECT and TYPE_TEXT
// (TS PackShared.ts:498, @3c16994c). Tests both component types via
// Pack → LoadComponentTypes round-trip.
func TestPack_245_2_ActiveOverColour(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	scriptsDir := filepath.Join(src, "scripts")
	packDir := filepath.Join(src, "pack")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "interface.pack"),
		[]byte("0=myif\n1=myif:box\n2=myif:lbl\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "interface.order"),
		[]byte("0\n1\n2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.order: %v", err)
	}
	for _, name := range []string{"obj", "model", "seq", "varp"} {
		if err := os.WriteFile(filepath.Join(packDir, name+".pack"), []byte(""), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	// box = rect (comType=3), lbl = text (comType=4); both carry activeovercolour.
	// Colours are decimal, mirroring how atoiOr0 also accepts plain ints.
	body := "[box]\ntype=rect\nwidth=50\nheight=50\ncolour=1\nactivecolour=2\novercolour=3\nactiveovercolour=11259375\n" +
		"[lbl]\ntype=text\nwidth=50\nheight=10\ncolour=1\nactivecolour=2\novercolour=3\nactiveovercolour=11259375\ntext=hi\nactivetext=bye\n"
	if err := os.WriteFile(filepath.Join(scriptsDir, "myif.if"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile myif.if: %v", err)
	}

	reg := &pack.Registry{SrcDir: src}
	out := filepath.Join(tmp, "out")
	if err := Pack(reg, src, out, nil, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	configs, err := objtype.LoadComponentTypes(out)
	if err != nil {
		t.Fatalf("LoadComponentTypes: %v (likely EOF — packer missing activeovercolour P4)", err)
	}
	if configs == nil || len(configs.Configs) < 3 {
		t.Fatalf("LoadComponentTypes returned %d configs, want >=3", len(configs.Configs))
	}

	box := configs.Configs[1]
	if box == nil {
		t.Fatalf("configs[1] (rect) is nil")
	}
	// 11259375 decimal = 0x00ABCDEF
	if box.ActiveOverColour != int32(0x00ABCDEF) {
		t.Errorf("rect ActiveOverColour: got %08x, want 00abcdef (TS PackShared.ts:498 @3c16994c, new in 245.2)", box.ActiveOverColour)
	}

	lbl := configs.Configs[2]
	if lbl == nil {
		t.Fatalf("configs[2] (text) is nil")
	}
	if lbl.ActiveOverColour != int32(0x00ABCDEF) {
		t.Errorf("text ActiveOverColour: got %08x, want 00abcdef (TS PackShared.ts:498 @3c16994c, new in 245.2)", lbl.ActiveOverColour)
	}
}

func TestPack_MissingSrcReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	reg := &pack.Registry{SrcDir: filepath.Join(tmp, "src")}
	if err := Pack(reg, filepath.Join(tmp, "src"), filepath.Join(tmp, "out"), nil, nil); err != nil {
		t.Errorf("Pack: %v, want nil", err)
	}
}

// TestPack_RepackWritesArchiveIntoTruncatedCache reproduces the rev-274
// stale-cache 404 bug. PackAll opens the FileStream with createNew=true, which
// TRUNCATES dat+idx on every run, then calls clientinterface.Pack. On a second
// pack the intermediate data/pack/client/interface already exists and is fresh,
// so the freshness gate (ShouldBuild) says "no rebuild". TS PackClient.ts:48
// still writes the archive into the freshly-truncated cache unconditionally and
// only early-returns when `!rebuild && cache.has(0, 3)`. goscape dropped the
// `cache.has(0, 3)` guard, so it returned early and left interface (idx0 file 3)
// empty — the OnDemand /interface route then 404s.
//
// The test mirrors two consecutive PackAll runs: createNew=true truncate +
// ClearFsCache before each Pack. After the second pack the interface archive
// must still be present.
func TestPack_RepackWritesArchiveIntoTruncatedCache(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	scriptsDir := filepath.Join(src, "scripts")
	packDir := filepath.Join(src, "pack")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "interface.pack"), []byte("0=mychat\n1=mychat:layer1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "interface.order"), []byte("0\n1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.order: %v", err)
	}
	for _, name := range []string{"obj", "model", "seq", "varp"} {
		if err := os.WriteFile(filepath.Join(packDir, name+".pack"), []byte(""), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	ifPath := filepath.Join(scriptsDir, "mychat.if")
	if err := os.WriteFile(ifPath, []byte("[layer1]\ntype=layer\nwidth=100\nheight=100\n"), 0o644); err != nil {
		t.Fatalf("WriteFile mychat.if: %v", err)
	}
	// Age the source so the freshly-written client/interface is unambiguously
	// newer ⇒ ShouldBuild==false (no rebuild) on the second pack.
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(ifPath, past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	reg := &pack.Registry{SrcDir: src}
	out := filepath.Join(tmp, "out")

	// First PackAll run: fresh (empty) cache, then pack.
	pack.ClearFsCache()
	cache1, err := filestream.New(out, true, false)
	if err != nil {
		t.Fatalf("filestream.New cache1: %v", err)
	}
	if err := Pack(reg, src, out, nil, cache1); err != nil {
		t.Fatalf("first Pack: %v", err)
	}
	if !cache1.Has(0, 3) {
		t.Fatalf("first pack did not write interface archive (0,3)")
	}
	if err := cache1.Close(); err != nil {
		t.Fatalf("cache1 Close: %v", err)
	}

	// Second PackAll run: createNew=true truncates the cache (so (0,3) is gone),
	// but out/client/interface already exists and is fresh.
	pack.ClearFsCache()
	cache2, err := filestream.New(out, true, false)
	if err != nil {
		t.Fatalf("filestream.New cache2: %v", err)
	}
	defer cache2.Close()
	if cache2.Has(0, 3) {
		t.Fatalf("precondition failed: createNew=true should have truncated (0,3)")
	}
	if err := Pack(reg, src, out, nil, cache2); err != nil {
		t.Fatalf("second Pack: %v", err)
	}
	if !cache2.Has(0, 3) {
		t.Fatalf("repack lost interface archive (0,3): cache empty after fresh-cache repack")
	}
	if data := cache2.Read(0, 3, false); len(data) == 0 {
		t.Fatalf("repack wrote empty interface archive (0,3)")
	}
}

// TestPack_LayerIdZeroErrors pins TS PackShared.ts:209 (9aadcec4) `!layerId`
// semantics: a layer name that resolves to id 0 must now error (new in rev-244;
// rev-225 only errored on -1 / not-found). The fixture gives the root interface
// id=0 and a child layer id=1; the layer= directive points at the ROOT (id=0),
// which resolves to 0 and must be rejected.
func TestPack_LayerIdZeroErrors(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	scriptsDir := filepath.Join(src, "scripts")
	packDir := filepath.Join(src, "pack")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// id=0 → "chat" (root); id=1 → "chat:label1" (child).
	// layer=chat means the child tries to re-parent under the ROOT,
	// which resolves to id 0 — must error at rev-244.
	if err := os.WriteFile(filepath.Join(packDir, "interface.pack"), []byte("0=chat\n1=chat:label1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "interface.order"), []byte("0\n1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.order: %v", err)
	}
	for _, name := range []string{"obj", "model", "seq", "varp"} {
		if err := os.WriteFile(filepath.Join(packDir, name+".pack"), []byte(""), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	// label1 with layer=chat — layerId for "chat:chat" won't exist, but
	// here we want to test id=0 path: reference the root name directly.
	// "chat:chat" is not in the pack, so GetByName returns -1 (unchanged).
	// To hit id=0, the layer must resolve to the root interface name.
	// InterfacePack.GetByName("chat:chat") = -1 (not registered); for id=0
	// we need layer=<name_registered_as_id0>. But id=0 is "chat" (the root
	// interface itself — no colon). So layer=<something> where
	// ifName+":"+value maps to id=0 i.e. GetByName("chat:chat")=0 is not
	// in pack. The canonical fixture: register "chat:root" at id=0 and use
	// that as the layer target.
	//
	// Revised fixture: id=0 → "chat:root", id=1 → "chat:label1",
	// root interface is "chat" (not in pack). We use only the scripts
	// directory name "chat.if" to set ifName.
	if err := os.WriteFile(filepath.Join(packDir, "interface.pack"), []byte("0=chat:root\n1=chat:label1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "interface.order"), []byte("0\n1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.order: %v", err)
	}
	// "chat.if" body: the root section (no [header]) plus label1 with
	// layer=root — GetByName("chat:root") = 0, which must error.
	body := "[root]\ntype=layer\nwidth=100\nheight=100\n[label1]\ntype=text\nwidth=10\nheight=10\nlayer=root\n"
	if err := os.WriteFile(filepath.Join(scriptsDir, "chat.if"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile chat.if: %v", err)
	}

	reg := &pack.Registry{SrcDir: src}
	out := filepath.Join(tmp, "out")
	err := Pack(reg, src, out, nil, nil)
	if err == nil {
		t.Fatal("Pack: want error for layer resolving to id=0, got nil")
	}
}

// TestPack_ModelFlagsSet pins TS PackShared.ts:511,522 (9aadcec4):
// when a comType=6 interface component references a model or activemodel,
// modelFlags[modelId] must have 0x2 OR-ed in. The fixture creates a
// model+interface that uses model id=0; after Pack, flags[0] must be 0x2.
func TestPack_ModelFlagsSet(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	scriptsDir := filepath.Join(src, "scripts")
	packDir := filepath.Join(src, "pack")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// interface.pack: id=0 → myif (root), id=1 → myif:widget.
	if err := os.WriteFile(filepath.Join(packDir, "interface.pack"), []byte("0=myif\n1=myif:widget\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "interface.order"), []byte("0\n1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.order: %v", err)
	}
	// model.pack: id=0 → mymodel
	if err := os.WriteFile(filepath.Join(packDir, "model.pack"), []byte("0=mymodel\n"), 0o644); err != nil {
		t.Fatalf("WriteFile model.pack: %v", err)
	}
	for _, name := range []string{"obj", "seq", "varp"} {
		if err := os.WriteFile(filepath.Join(packDir, name+".pack"), []byte(""), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	// comType=6 component referencing model=mymodel (id=0).
	body := "[widget]\ntype=model\nwidth=100\nheight=100\nmodel=mymodel\n"
	if err := os.WriteFile(filepath.Join(scriptsDir, "myif.if"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile myif.if: %v", err)
	}

	reg := &pack.Registry{SrcDir: src}
	out := filepath.Join(tmp, "out")
	flags := make([]int, 1) // model max = 1 (id=0)
	if err := Pack(reg, src, out, flags, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if flags[0]&0x2 == 0 {
		t.Errorf("flags[0] = %d, want 0x2 bit set (TS PackShared.ts:511 modelFlags[modelId] |= 0x2)", flags[0])
	}
}

// TestPack_254_ScriptOps14to20 pins the rev-254 interface script ops
// (TS PackShared.ts:91-104 name map, :353-358 opcount, :438-450 emission
// @ 2e3bcf43): push_varbit(14, operand = VarbitPack id) and
// push_constant(20, operand = parseInt) carry one operand word each;
// subtract(15)/divide(16)/multiply(17)/coordx(18)/coordz(19) carry none.
// Verified via Pack -> LoadComponentTypes round-trip on the raw script
// word list (the runtime decoder reads opcodeCount p2 words verbatim).
func TestPack_254_ScriptOps14to20(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	scriptsDir := filepath.Join(src, "scripts")
	packDir := filepath.Join(src, "pack")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "interface.pack"),
		[]byte("0=myif\n1=myif:lay\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "interface.order"),
		[]byte("0\n1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile interface.order: %v", err)
	}
	for _, name := range []string{"obj", "model", "seq", "varp"} {
		if err := os.WriteFile(filepath.Join(packDir, name+".pack"), []byte(""), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	// varbit registry for the push_varbit operand lookup.
	if err := os.WriteFile(filepath.Join(packDir, "varbit.pack"), []byte("0=vb_unused\n1=vb_target\n"), 0o644); err != nil {
		t.Fatalf("WriteFile varbit.pack: %v", err)
	}

	body := "[lay]\ntype=layer\nwidth=10\nheight=10\n" +
		"script1=eq,1\n" +
		"script1op1=push_varbit,vb_target\n" +
		"script1op2=subtract\n" +
		"script1op3=divide\n" +
		"script1op4=multiply\n" +
		"script1op5=coordx\n" +
		"script1op6=coordz\n" +
		"script1op7=push_constant,42\n"
	if err := os.WriteFile(filepath.Join(scriptsDir, "myif.if"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile myif.if: %v", err)
	}

	reg := &pack.Registry{SrcDir: src}
	out := filepath.Join(tmp, "out")
	if err := Pack(reg, src, out, nil, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	configs, err := objtype.LoadComponentTypes(out)
	if err != nil {
		t.Fatalf("LoadComponentTypes: %v (likely word-count drift in ops 14-20)", err)
	}
	if configs == nil || len(configs.Configs) < 2 {
		t.Fatalf("LoadComponentTypes returned %d configs, want >=2", len(configs.Configs))
	}
	lay := configs.Configs[1]
	if lay == nil {
		t.Fatalf("configs[1] is nil")
	}
	if len(lay.Scripts) != 1 {
		t.Fatalf("Scripts count = %d, want 1", len(lay.Scripts))
	}
	// opCount = 7 ops + 1 (push_varbit operand) + 1 (push_constant operand)
	// = 9; script1op1 non-empty so the stored word count is opCount+1 = 10,
	// which includes the trailing 0 terminator word.
	want := []uint16{14, 1, 15, 16, 17, 18, 19, 20, 42, 0}
	got := lay.Scripts[0]
	if len(got) != len(want) {
		t.Fatalf("Scripts[0] = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Scripts[0][%d] = %d, want %d (full: %v)", i, got[i], want[i], got)
		}
	}
}
