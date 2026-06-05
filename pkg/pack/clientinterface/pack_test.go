package clientinterface

import (
	"os"
	"path/filepath"
	"testing"

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
	if err := Pack(reg, src, out, nil); err != nil {
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
	if err := Pack(reg, src, out, nil); err != nil {
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

func TestPack_MissingSrcReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	reg := &pack.Registry{SrcDir: filepath.Join(tmp, "src")}
	if err := Pack(reg, filepath.Join(tmp, "src"), filepath.Join(tmp, "out"), nil); err != nil {
		t.Errorf("Pack: %v, want nil", err)
	}
}
