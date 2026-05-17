package wordenc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/jagfile"
)

func TestPack_BytePinned(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	wordencDir := filepath.Join(src, "wordenc")
	if err := os.MkdirAll(wordencDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for name, body := range map[string]string{
		"badenc.txt":       "hi 1:2\n",
		"fragmentsenc.txt": "42\n",
		"tldlist.txt":      "com 1\n",
		"domainenc.txt":    "x.com\n",
	} {
		if err := os.WriteFile(filepath.Join(wordencDir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	outDir := filepath.Join(tmp, "out")
	if err := Pack(src, outDir); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	jagPath := filepath.Join(outDir, "client", "wordenc")
	jag, err := jagfile.LoadJagfile(jagPath)
	if err != nil {
		t.Fatalf("LoadJagfile: %v", err)
	}

	for _, name := range []string{"badenc.txt", "fragmentsenc.txt", "tldlist.txt", "domainenc.txt"} {
		pkt, err := jag.Read(name)
		if err != nil {
			t.Errorf("Read %q: %v", name, err)
			continue
		}
		if pkt.Length() == 0 {
			t.Errorf("entry %q is empty", name)
		}
	}
}

func TestPack_MissingSrcReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	outDir := filepath.Join(tmp, "out")

	// No src/wordenc dir exists. Should no-op cleanly.
	if err := Pack(src, outDir); err != nil {
		t.Errorf("Pack(missing src): %v, want nil", err)
	}
}
