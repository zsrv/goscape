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
// returns the path. Uses CompressWhole=true so the per-entry payload
// stays raw on disk (after whole-blob bzip2), giving per-entry
// FileUnpackedSize == FilePackedSize == 1 for the `list` byte-pin.
func makeTestJagPath(t *testing.T) string {
	t.Helper()
	jf := jagfile.NewEmptyJagfile(true)
	jf.Write("hitmarks.dat", packet.NewPacket([]byte{0xFF}))
	path := filepath.Join(t.TempDir(), "test.jag")
	if err := jf.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return path
}

// makeTestJagPathWithUnknownEntry writes a single-entry Jagfile whose
// entry name (and therefore hash) is NOT present in jagfile.knownNames.
// NewJagfile's reverse-resolution leaves FileName[0] as "" on reload,
// letting us exercise the runJagDump unknown-hash skip path.
func makeTestJagPathWithUnknownEntry(t *testing.T) string {
	t.Helper()
	jf := jagfile.NewEmptyJagfile(true)
	jf.Write("unknownentry.dat", packet.NewPacket([]byte{0xFF}))
	path := filepath.Join(t.TempDir(), "test.jag")
	if err := jf.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return path
}

// TestRunJag_DumpUnknownHash verifies that entries whose hash isn't
// in jagfile.knownNames are skipped with a stderr warning, instead of
// silently writing to the outDir path itself via filepath.Join("dump", "").
func TestRunJag_DumpUnknownHash(t *testing.T) {
	path := makeTestJagPathWithUnknownEntry(t)
	outDir := filepath.Join(t.TempDir(), "dump")

	var stdout, stderr bytes.Buffer
	code := runJag([]string{"dump", path, "--out", outDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runJag returned %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "not in known-names table") {
		t.Errorf("stderr %q missing 'not in known-names table'", stderr.String())
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("outDir has %d entries, want 0 (unknown-hash entry should be skipped)", len(entries))
	}
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

// TestSafeBasename verifies the path-traversal guard rejects entries
// that would escape the dump directory when joined as a leaf.
func TestSafeBasename(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"hitmarks.dat", true},
		{"a", true},
		{"foo.bar.baz", true},
		{"", false},
		{".", false},
		{"..", false},
		{"../escape.txt", false},
		{"../../etc/passwd", false},
		{"a/b", false},
		{`a\b`, false},
	} {
		if got := safeBasename(tc.in); got != tc.want {
			t.Errorf("safeBasename(%q) = %v, want %v", tc.in, got, tc.want)
		}
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
