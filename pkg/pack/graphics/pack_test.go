package graphics

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
	modelsDir := filepath.Join(src, "models")
	packDir := filepath.Join(src, "pack")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatalf("mkdir models: %v", err)
	}
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}

	if err := os.WriteFile(filepath.Join(packDir, "model.pack"), []byte("0=m0\n"), 0o644); err != nil {
		t.Fatalf("write model.pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "anim.pack"), []byte("0=a0\n"), 0o644); err != nil {
		t.Fatalf("write anim.pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "base.pack"), []byte("0=b0\n"), 0o644); err != nil {
		t.Fatalf("write base.pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "model.order"), []byte("0\n"), 0o644); err != nil {
		t.Fatalf("write model.order: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "anim.order"), []byte("0\n"), 0o644); err != nil {
		t.Fatalf("write anim.order: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "base.order"), []byte("0\n"), 0o644); err != nil {
		t.Fatalf("write base.order: %v", err)
	}

	// Minimal valid trailer-encoded fixtures:
	//   .base = 4 trailing length bytes (typeLength=0, labelLength=0)
	//   .frame = 8 trailing length bytes (all 4 lengths = 0)
	//   .ob2 = 18 trailing bytes (all counts/flags/lengths = 0)
	if err := os.WriteFile(filepath.Join(modelsDir, "b0.base"), make([]byte, 4), 0o644); err != nil {
		t.Fatalf("write b0.base: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "a0.frame"), make([]byte, 8), 0o644); err != nil {
		t.Fatalf("write a0.frame: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "m0.ob2"), make([]byte, 18), 0o644); err != nil {
		t.Fatalf("write m0.ob2: %v", err)
	}

	reg := &pack.Registry{SrcDir: src}
	if _, err := reg.EnsureModel(); err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}
	if _, err := reg.EnsureAnim(); err != nil {
		t.Fatalf("EnsureAnim: %v", err)
	}
	if _, err := reg.EnsureBase(); err != nil {
		t.Fatalf("EnsureBase: %v", err)
	}

	out := filepath.Join(tmp, "out")
	if err := Pack(reg, src, out); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	jag, err := jagfile.LoadJagfile(filepath.Join(out, "client", "models"))
	if err != nil {
		t.Fatalf("LoadJagfile: %v", err)
	}
	want := []string{
		"base_label.dat",
		"ob_point1.dat", "ob_point2.dat", "ob_point3.dat", "ob_point4.dat", "ob_point5.dat",
		"ob_head.dat", "base_head.dat",
		"frame_head.dat", "frame_tran1.dat", "frame_tran2.dat",
		"ob_vertex1.dat", "ob_vertex2.dat", "frame_del.dat",
		"base_type.dat",
		"ob_face1.dat", "ob_face2.dat", "ob_face3.dat", "ob_face4.dat", "ob_face5.dat",
		"ob_axis.dat",
	}
	for _, name := range want {
		if _, err := jag.Read(name); err != nil {
			t.Errorf("Read %q: %v", name, err)
		}
	}
}

func TestPack_MissingSrcReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	reg := &pack.Registry{SrcDir: filepath.Join(tmp, "src")}
	if err := Pack(reg, filepath.Join(tmp, "src"), filepath.Join(tmp, "out")); err != nil {
		t.Errorf("Pack: %v, want nil", err)
	}
}
