package pack

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPackSeqRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	setupPackRoots(t, srcDir)

	writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=walk\n")
	writeFile(t, filepath.Join(srcDir, "pack", "anim.pack"), "0=walk_frame_1\n")
	writeFile(t, filepath.Join(srcDir, "pack", "flo.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "idk.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "")

	writeFile(t, filepath.Join(srcDir, "scripts", "test.seq"),
		"[walk]\nloops=2\npriority=5\nframe1=walk_frame_1\n")

	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	frames := &objtype.SeqFrameConfigs{}
	seqs, err := objtype.LoadSeqTypes(outDir, frames)
	if err != nil {
		t.Fatal(err)
	}
	seq := seqs.Configs[0]

	if seq.Loops != 2 {
		t.Errorf("Loops: got %d, want 2", seq.Loops)
	}
	if seq.Priority != 5 {
		t.Errorf("Priority: got %d, want 5", seq.Priority)
	}
	// frame1=walk_frame_1 maps to anim id 0 (the only entry in anim.pack)
	if len(seq.Frames) == 0 || seq.Frames[0] != 0 {
		t.Errorf("Frames[0]: got %v, want 0", seq.Frames)
	}
}
