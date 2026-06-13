package worldmap

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/unpack/unpacktest"
)

// buildFloPackets constructs the server and client flo packets for a list of
// flo configs. Each config is (rgb int, texture int, overlay bool, occlude bool, debugname string).
type floConfig struct {
	rgb       int
	texture   int  // -1 = none
	overlay   bool // opcode 3 present
	occlude   bool // TS default = true; false when opcode 5 present
	debugname string
}

func buildFloPackets(t *testing.T, cfgs []floConfig) (*packet.Packet, *jagfile.Jagfile) {
	t.Helper()

	server := packet.Alloc(1)
	server.P2(uint16(len(cfgs)))
	for range cfgs {
		server.P1(0) // zero terminator per id (TS: PackedData.next())
	}

	clientFlo := packet.Alloc(1)
	defer clientFlo.Release()
	clientFlo.P2(uint16(len(cfgs)))
	for _, c := range cfgs {
		if c.rgb != 0 {
			clientFlo.P1(1)
			clientFlo.P3(uint32(c.rgb))
		}
		if c.texture != -1 {
			clientFlo.P1(2)
			clientFlo.P1(uint8(c.texture))
		}
		if c.overlay {
			clientFlo.P1(3)
		}
		if !c.occlude {
			clientFlo.P1(5)
		}
		if c.debugname != "" {
			clientFlo.P1(6)
			clientFlo.PJStrLF(c.debugname)
		}
		clientFlo.P1(0)
	}

	jf := jagfile.NewEmptyJagfile(false)
	jf.Write("flo.dat", clientFlo)
	cfgPath := filepath.Join(t.TempDir(), "config")
	if err := jf.Save(cfgPath); err != nil {
		t.Fatalf("Save client/config jagfile: %v", err)
	}
	loaded, err := jagfile.LoadJagfile(cfgPath)
	if err != nil {
		t.Fatalf("LoadJagfile client/config: %v", err)
	}
	return server, loaded
}

// TestUnpack_NoTexture verifies the no-texture output line format.
// TS line 19: `[0x${underlay}, 0x${overlay}], // debugname=X overlay=Y occlude=Z rgb=0xABCDEF`
func TestUnpack_NoTexture(t *testing.T) {
	t.Parallel()

	packDir := t.TempDir()
	srcDir := t.TempDir()

	// One flo entry: rgb=0xaabbcc, no texture, overlay=true, occlude=true, debugname=water.
	cfgs := []floConfig{
		{rgb: 0xaabbcc, texture: -1, overlay: true, occlude: true, debugname: "water"},
	}
	server, clientJag := buildFloPackets(t, cfgs)
	defer server.Release()

	// Save server and client into packDir.
	if err := os.MkdirAll(filepath.Join(packDir, "server"), 0o755); err != nil {
		t.Fatalf("mkdir server: %v", err)
	}
	if err := server.Save(filepath.Join(packDir, "server", "flo.dat"), server.Length(), 0); err != nil {
		t.Fatalf("Save server/flo.dat: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(packDir, "client"), 0o755); err != nil {
		t.Fatalf("mkdir client: %v", err)
	}
	if err := clientJag.Save(filepath.Join(packDir, "client", "config")); err != nil {
		t.Fatalf("Save client/config: %v", err)
	}

	// Build worldmap.jag: one floorcol entry (underlay=0x00000001, overlay=0x00000002), one label.
	floorcol := packet.Alloc(1)
	defer floorcol.Release()
	floorcol.P2(1)          // count = 1
	floorcol.P4(0x00000001) // underlay
	floorcol.P4(0x00000002) // overlay

	labelsP := packet.Alloc(1)
	defer labelsP.Release()
	labelsP.P2(1)              // labelCount = 1
	labelsP.PJStrLF("Varrock") // text
	labelsP.P2(3200)           // x
	labelsP.P2(3200)           // y
	labelsP.P1(0)              // font

	wm := jagfile.NewEmptyJagfile(false)
	wm.Write("floorcol.dat", floorcol)
	wm.Write("labels.dat", labelsP)
	cacheDir := t.TempDir()
	if err := wm.Save(filepath.Join(cacheDir, "worldmap.jag")); err != nil {
		t.Fatalf("Save worldmap.jag: %v", err)
	}

	// Empty texture.pack.
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "pack", "texture.pack"), []byte(""), 0o644); err != nil {
		t.Fatalf("Write texture.pack: %v", err)
	}

	var out bytes.Buffer
	if err := Unpack(cacheDir, packDir, srcDir, &out); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	got := out.String()
	// Expected line: [0x00000001, 0x00000002], // debugname=water overlay=true occlude=true rgb=0xaabbcc
	wantFloLine := "[0x00000001, 0x00000002], // debugname=water overlay=true occlude=true rgb=0xaabbcc\n"
	wantSep := "----\n"
	wantLabel := "=Varrock,3200,3200,0\n"
	want := wantFloLine + wantSep + wantLabel
	if got != want {
		t.Errorf("Unpack output mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

// TestUnpack_WithTexture verifies the with-texture output line format.
// TS line 17: `[0x...], // debugname=X overlay=Y occlude=Z rgb=0xABCDEF texture=NAME`
func TestUnpack_WithTexture(t *testing.T) {
	t.Parallel()

	packDir := t.TempDir()
	srcDir := t.TempDir()

	// One flo entry: rgb=0x112233, texture=0, overlay=false, occlude=false, debugname=stone.
	cfgs := []floConfig{
		{rgb: 0x112233, texture: 0, overlay: false, occlude: false, debugname: "stone"},
	}
	server, clientJag := buildFloPackets(t, cfgs)
	defer server.Release()

	if err := os.MkdirAll(filepath.Join(packDir, "server"), 0o755); err != nil {
		t.Fatalf("mkdir server: %v", err)
	}
	if err := server.Save(filepath.Join(packDir, "server", "flo.dat"), server.Length(), 0); err != nil {
		t.Fatalf("Save server/flo.dat: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(packDir, "client"), 0o755); err != nil {
		t.Fatalf("mkdir client: %v", err)
	}
	if err := clientJag.Save(filepath.Join(packDir, "client", "config")); err != nil {
		t.Fatalf("Save client/config: %v", err)
	}

	// Build worldmap.jag with one floorcol entry.
	floorcol := packet.Alloc(1)
	defer floorcol.Release()
	floorcol.P2(1)
	floorcol.P4(0xdeadbeef) // underlay — large value, tests 8-char hex
	floorcol.P4(0x00ff0000) // overlay

	labelsP := packet.Alloc(1)
	defer labelsP.Release()
	labelsP.P2(0) // no labels

	wm := jagfile.NewEmptyJagfile(false)
	wm.Write("floorcol.dat", floorcol)
	wm.Write("labels.dat", labelsP)
	cacheDir := t.TempDir()
	if err := wm.Save(filepath.Join(cacheDir, "worldmap.jag")); err != nil {
		t.Fatalf("Save worldmap.jag: %v", err)
	}

	// Seed texture.pack naming id=0 "granite" — under 274 suspendAutoReload the
	// TexturePack singleton is constructed empty (TS PackFile.ts:276 @dee467c8),
	// so this is ignored and the texture= field is emitted empty.
	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "pack", "texture.pack"), []byte("0=granite\n"), 0o644); err != nil {
		t.Fatalf("Write texture.pack: %v", err)
	}

	var out bytes.Buffer
	if err := Unpack(cacheDir, packDir, srcDir, &out); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	got := out.String()
	// texture=0 → empty TexturePack → GetByID(0) = "" → "texture="
	// occlude=false → "false"; overlay=false → "false"
	wantFloLine := "[0xdeadbeef, 0x00ff0000], // debugname=stone overlay=false occlude=false rgb=0x112233 texture=\n"
	wantSep := "----\n"
	want := wantFloLine + wantSep
	if got != want {
		t.Errorf("Unpack output mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

// TestUnpack_HexPadding verifies that small values are zero-padded to 8 hex digits
// and that rgb is always 6 digits.
func TestUnpack_HexPadding(t *testing.T) {
	t.Parallel()

	packDir := t.TempDir()
	srcDir := t.TempDir()

	// Flo with rgb=0 (smallest) and no texture.
	cfgs := []floConfig{
		{rgb: 0, texture: -1, overlay: false, occlude: true, debugname: "void"},
	}
	server, clientJag := buildFloPackets(t, cfgs)
	defer server.Release()

	if err := os.MkdirAll(filepath.Join(packDir, "server"), 0o755); err != nil {
		t.Fatalf("mkdir server: %v", err)
	}
	if err := server.Save(filepath.Join(packDir, "server", "flo.dat"), server.Length(), 0); err != nil {
		t.Fatalf("Save server/flo.dat: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(packDir, "client"), 0o755); err != nil {
		t.Fatalf("mkdir client: %v", err)
	}
	if err := clientJag.Save(filepath.Join(packDir, "client", "config")); err != nil {
		t.Fatalf("Save client/config: %v", err)
	}

	// worldmap.jag: underlay=0, overlay=0 → padded to 00000000.
	floorcol := packet.Alloc(1)
	defer floorcol.Release()
	floorcol.P2(1)
	floorcol.P4(0x00000000)
	floorcol.P4(0x00000000)

	labelsP := packet.Alloc(1)
	defer labelsP.Release()
	labelsP.P2(0)

	wm := jagfile.NewEmptyJagfile(false)
	wm.Write("floorcol.dat", floorcol)
	wm.Write("labels.dat", labelsP)
	cacheDir := t.TempDir()
	if err := wm.Save(filepath.Join(cacheDir, "worldmap.jag")); err != nil {
		t.Fatalf("Save worldmap.jag: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "pack", "texture.pack"), []byte(""), 0o644); err != nil {
		t.Fatalf("Write texture.pack: %v", err)
	}

	var out bytes.Buffer
	if err := Unpack(cacheDir, packDir, srcDir, &out); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	got := out.String()
	// underlay/overlay = 0 → "00000000" (8 hex digits); rgb = 0 → "000000" (6 hex digits).
	wantLine := "[0x00000000, 0x00000000], // debugname=void overlay=false occlude=true rgb=0x000000\n"
	if !strings.Contains(got, wantLine) {
		t.Errorf("hex padding: want line %q in output %q", wantLine, got)
	}
}

// TestUnpack_FloorcolCountClamped verifies that when floorcolCount > flo.Configs.length,
// only flo.Configs.length entries are emitted (TS line 11: i < FloType.configs.length).
func TestUnpack_FloorcolCountClamped(t *testing.T) {
	t.Parallel()

	packDir := t.TempDir()
	srcDir := t.TempDir()

	// 1 flo config, but floorcol has 3 entries — only 1 should be printed.
	cfgs := []floConfig{
		{rgb: 0x010101, texture: -1, overlay: false, occlude: true, debugname: "grass"},
	}
	server, clientJag := buildFloPackets(t, cfgs)
	defer server.Release()

	if err := os.MkdirAll(filepath.Join(packDir, "server"), 0o755); err != nil {
		t.Fatalf("mkdir server: %v", err)
	}
	if err := server.Save(filepath.Join(packDir, "server", "flo.dat"), server.Length(), 0); err != nil {
		t.Fatalf("Save server/flo.dat: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(packDir, "client"), 0o755); err != nil {
		t.Fatalf("mkdir client: %v", err)
	}
	if err := clientJag.Save(filepath.Join(packDir, "client", "config")); err != nil {
		t.Fatalf("Save client/config: %v", err)
	}

	floorcol := packet.Alloc(1)
	defer floorcol.Release()
	floorcol.P2(3) // count=3, but only 1 flo config
	floorcol.P4(0xaaaaaaaa)
	floorcol.P4(0xbbbbbbbb)
	floorcol.P4(0xcccccccc)
	floorcol.P4(0xdddddddd)
	floorcol.P4(0xeeeeeeee)
	floorcol.P4(0xffffffff)

	labelsP := packet.Alloc(1)
	defer labelsP.Release()
	labelsP.P2(0)

	wm := jagfile.NewEmptyJagfile(false)
	wm.Write("floorcol.dat", floorcol)
	wm.Write("labels.dat", labelsP)
	cacheDir := t.TempDir()
	if err := wm.Save(filepath.Join(cacheDir, "worldmap.jag")); err != nil {
		t.Fatalf("Save worldmap.jag: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "pack", "texture.pack"), []byte(""), 0o644); err != nil {
		t.Fatalf("Write texture.pack: %v", err)
	}

	var out bytes.Buffer
	if err := Unpack(cacheDir, packDir, srcDir, &out); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	// Lines: 1 floorcol entry + "----" = 2 lines.
	if len(lines) != 2 {
		t.Errorf("want 2 lines (1 flo + separator), got %d: %q", len(lines), out.String())
	}
}

// TestUnpack_MultipleLabels verifies that multiple label lines are emitted correctly.
func TestUnpack_MultipleLabels(t *testing.T) {
	t.Parallel()

	packDir := t.TempDir()
	srcDir := t.TempDir()

	// Zero flo configs.
	cfgs := []floConfig{}
	server, clientJag := buildFloPackets(t, cfgs)
	defer server.Release()

	if err := os.MkdirAll(filepath.Join(packDir, "server"), 0o755); err != nil {
		t.Fatalf("mkdir server: %v", err)
	}
	if err := server.Save(filepath.Join(packDir, "server", "flo.dat"), server.Length(), 0); err != nil {
		t.Fatalf("Save server/flo.dat: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(packDir, "client"), 0o755); err != nil {
		t.Fatalf("mkdir client: %v", err)
	}
	if err := clientJag.Save(filepath.Join(packDir, "client", "config")); err != nil {
		t.Fatalf("Save client/config: %v", err)
	}

	floorcol := packet.Alloc(1)
	defer floorcol.Release()
	floorcol.P2(0)

	labelsP := packet.Alloc(1)
	defer labelsP.Release()
	labelsP.P2(3)
	labelsP.PJStrLF("Lumbridge")
	labelsP.P2(3222)
	labelsP.P2(3218)
	labelsP.P1(1)
	labelsP.PJStrLF("Draynor Village")
	labelsP.P2(3093)
	labelsP.P2(3244)
	labelsP.P1(2)
	labelsP.PJStrLF("Al Kharid")
	labelsP.P2(3293)
	labelsP.P2(3166)
	labelsP.P1(0)

	wm := jagfile.NewEmptyJagfile(false)
	wm.Write("floorcol.dat", floorcol)
	wm.Write("labels.dat", labelsP)
	cacheDir := t.TempDir()
	if err := wm.Save(filepath.Join(cacheDir, "worldmap.jag")); err != nil {
		t.Fatalf("Save worldmap.jag: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "pack", "texture.pack"), []byte(""), 0o644); err != nil {
		t.Fatalf("Write texture.pack: %v", err)
	}

	var out bytes.Buffer
	if err := Unpack(cacheDir, packDir, srcDir, &out); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	want := "----\n" +
		"=Lumbridge,3222,3218,1\n" +
		"=Draynor Village,3093,3244,2\n" +
		"=Al Kharid,3293,3166,0\n"
	if got := out.String(); got != want {
		t.Errorf("labels output:\ngot:  %q\nwant: %q", got, want)
	}
}

// TestWorldmapParity is the env-gated parity test against the TS reference run.
// It requires GOSCAPE_REF274_DIR. Run with:
//
//	GOSCAPE_REF274_DIR=/path/to/Server274-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/worldmap/ -run TestWorldmapParity -v -count=1 -timeout 900s
//
// This is a stdout-only tool (no content or cache writes). cacheDir contains
// worldmap.jag; packDir = GOSCAPE_REF274_DIR/data/pack (read-only);
// srcDir = content scratch for TexturePack reads (pack/texture.pack).
func TestWorldmapParity(t *testing.T) {
	refRoot := unpacktest.RefDir(t)
	contentDir := unpacktest.ContentDir(t)
	scratch := unpacktest.Scratch(t, contentDir)
	cacheDir := unpacktest.CacheDir(t)
	marker := unpacktest.Marker(t)

	// packDir: Server274-ref engine data/pack (read-only).
	packDir := filepath.Join(refRoot, "engine", "data", "pack")

	var out bytes.Buffer
	if err := Unpack(cacheDir, packDir, scratch, &out); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	// worldmap is stdout-only: no content or cache writes.
	content := unpacktest.ChangedSet(t, contentDir, scratch)
	cachePristine := refRoot + "/unpack-ref/cache"
	cache := unpacktest.ChangedSet(t, cachePristine, cacheDir)
	wrote := unpacktest.WroteSince(t, scratch, marker)

	r := unpacktest.Result{
		Content:      content,
		Cache:        cache,
		Wrote:        wrote,
		Stdout:       out.Bytes(),
		PostDir:      scratch,
		CachePostDir: cacheDir,
	}

	unpacktest.AssertManifest(t, refRoot, "worldmap", r)
}
