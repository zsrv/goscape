package clientinterface

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/jagfile"
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
	if err := Pack(reg, src, out); err != nil {
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

func TestPack_MissingSrcReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	reg := &pack.Registry{SrcDir: filepath.Join(tmp, "src")}
	if err := Pack(reg, filepath.Join(tmp, "src"), filepath.Join(tmp, "out")); err != nil {
		t.Errorf("Pack: %v, want nil", err)
	}
}
