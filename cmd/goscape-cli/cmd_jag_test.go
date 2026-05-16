package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// makeTestJagPath writes a small single-entry Jagfile (one file
// "hitmarks.dat" with payload []byte{0xFF}) to t.TempDir() and
// returns the path. Byte layout mirrors
// pkg/io/jagfile/jagfile_test.go:MakeTestJagfile, which lives in
// _test.go and is invisible cross-package.
func makeTestJagPath(t *testing.T) string {
	t.Helper()
	p := packet.NewPacket(make([]byte, 0, 19))
	p.P3(1)                        // UnpackedSize
	p.P3(1)                        // PackedSize
	p.P2(1)                        // FileCount
	p.P4(-1502153170 & 0xFFFFFFFF) // hash("hitmarks.dat")
	p.P3(1)                        // FileUnpackedSize[0]
	p.P3(1)                        // FilePackedSize[0]
	p.P1(255)                      // payload byte
	p.Pos = 0

	jf, err := jagfile.NewJagfile(p)
	if err != nil {
		t.Fatalf("NewJagfile: %v", err)
	}
	// NewJagfile reverse-resolves FileName[i] from FileHash[i] via
	// the package's knownNames table (which includes "hitmarks.dat"
	// at jagfile.go:432), so FileName is already populated here.
	//
	// However, NewJagfile parses bytes into FileHash/FileName/Size
	// arrays only — it does NOT populate FileWrite (the per-entry
	// blob slice consumed by Save). Save iterates FileWrite[i] for
	// i in [0, FileCount), so without manual seeding it panics with
	// index-out-of-range. Inject a one-byte payload to mirror the
	// fixture's intent. (Plan-codified fixture did not anticipate
	// this; deviation logged in commit body.)
	jf.FileWrite = [][]uint8{{0xFF}}

	path := filepath.Join(t.TempDir(), "test.jag")
	if err := jf.Save(path, false); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return path
}

// TestRunJag_List verifies `jag list` writes one TAB-separated line
// per entry to stdout.
func TestRunJag_List(t *testing.T) {
	path := makeTestJagPath(t)

	var stdout, stderr bytes.Buffer
	code := runJag([]string{"list", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runJag returned %d, want 0; stderr=%q", code, stderr.String())
	}
	want := "hitmarks.dat\t1\t1\n"
	if stdout.String() != want {
		t.Errorf("stdout %q, want %q", stdout.String(), want)
	}
}

// TestRunJag_ExtractToStdout verifies extract writes raw entry bytes
// to stdout when --out is unset (or "-").
func TestRunJag_ExtractToStdout(t *testing.T) {
	path := makeTestJagPath(t)

	var stdout, stderr bytes.Buffer
	code := runJag([]string{"extract", path, "hitmarks.dat"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runJag returned %d, want 0; stderr=%q", code, stderr.String())
	}
	if !bytes.Equal(stdout.Bytes(), []byte{0xFF}) {
		t.Errorf("stdout bytes %v, want [255]", stdout.Bytes())
	}
}

// TestRunJag_ExtractToFile verifies --out <path> writes raw bytes
// to the file.
func TestRunJag_ExtractToFile(t *testing.T) {
	path := makeTestJagPath(t)
	out := filepath.Join(t.TempDir(), "out.bin")

	var stdout, stderr bytes.Buffer
	code := runJag([]string{"extract", path, "hitmarks.dat", "--out", out}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runJag returned %d, want 0; stderr=%q", code, stderr.String())
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, []byte{0xFF}) {
		t.Errorf("file bytes %v, want [255]", got)
	}
}

// TestRunJag_ExtractMissingEntry expects exit 1 with a "no such
// entry" diagnostic in stderr.
func TestRunJag_ExtractMissingEntry(t *testing.T) {
	path := makeTestJagPath(t)

	var stdout, stderr bytes.Buffer
	code := runJag([]string{"extract", path, "nope.dat"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runJag returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no such entry") {
		t.Errorf("stderr %q missing 'no such entry'", stderr.String())
	}
}

// TestRunJag_Dump writes every entry into --out <dir>. With a one-
// entry fixture, asserts <dir>/hitmarks.dat exists with the right
// bytes.
func TestRunJag_Dump(t *testing.T) {
	path := makeTestJagPath(t)
	outDir := filepath.Join(t.TempDir(), "dump")

	var stdout, stderr bytes.Buffer
	code := runJag([]string{"dump", path, "--out", outDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runJag returned %d, want 0; stderr=%q", code, stderr.String())
	}
	got, err := os.ReadFile(filepath.Join(outDir, "hitmarks.dat"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, []byte{0xFF}) {
		t.Errorf("dumped bytes %v, want [255]", got)
	}
}
