package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConstants_HappyPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "a.constant"),
		"^FOO=100\nBAR=hello\n")
	writeFile(t, filepath.Join(dir, "scripts", "sub", "b.constant"),
		"BAZ=world\n")
	ClearFsCache()
	c, err := LoadConstants(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c["FOO"] != "100" {
		t.Errorf("FOO=%q, want 100", c["FOO"])
	}
	if c["BAR"] != "hello" {
		t.Errorf("BAR=%q, want hello", c["BAR"])
	}
	if c["BAZ"] != "world" {
		t.Errorf("BAZ=%q, want world", c["BAZ"])
	}
}

func TestLoadConstants_SkipsBlankAndLineComments(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "a.constant"),
		"\n// a comment\nFOO=1\n   \nBAR=2\n")
	ClearFsCache()
	c, err := LoadConstants(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 2 || c["FOO"] != "1" || c["BAR"] != "2" {
		t.Fatalf("c=%v", c)
	}
}

func TestLoadConstants_DuplicateNameErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "a.constant"),
		"FOO=1\nFOO=2\n")
	ClearFsCache()
	_, err := LoadConstants(dir)
	if err == nil {
		t.Fatal("want duplicate-constant error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "FOO") {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadConstants_MissingScriptsDirReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	ClearFsCache()
	c, err := LoadConstants(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 0 {
		t.Fatalf("want empty, got %v", c)
	}
}

func TestSubstituteConstants_Table(t *testing.T) {
	c := Constants{"FOO": "100", "BAR": "hello"}
	cases := []struct {
		in   string
		want string
	}{
		{"^FOO", "100"},
		{"^FOO\n", "100\n"},
		{"^FOO\r", "100\r"},
		{"^FOO,extra", "100,extra"},
		{"^FOO extra", "100 extra"},
		{"prefix ^FOO", "prefix 100"},
		{"^FOO,^BAR", "100,hello"},
		{"^MISSING", "^MISSING"},                 // absent — literal preserved
		{"no_sub_here", "no_sub_here"},           // no caret at all
		{"^FOOBAR_no_match", "^FOOBAR_no_match"}, // FOOBAR_no_match not in map
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := substituteConstants(tc.in, c)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
