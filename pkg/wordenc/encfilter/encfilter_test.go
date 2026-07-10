package encfilter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// makeSyntheticJag builds a minimal wordenc jagfile containing one entry in
// each of the 4 sections, mirroring the TS pack format (Engine-TS/src/cache/
// wordenc/WordEnc.ts:190-221 decoders + pkg/pack/wordenc/pack.go encoders).
func makeSyntheticJag(t *testing.T) *jagfile.Jagfile {
	t.Helper()
	jf := jagfile.NewEmptyJagfile(false)

	// badenc.txt: 1 entry "anal" with one combo 3:19.
	bad := packet.Alloc(2)
	bad.P4(1)
	bad.P1(4) // word length
	for _, c := range []byte("anal") {
		bad.P1(c)
	}
	bad.P1(1) // combo count
	bad.P1(3)
	bad.P1(19)
	jf.Write("badenc.txt", bad)

	// fragmentsenc.txt: 1 entry value 42.
	frag := packet.Alloc(2)
	frag.P4(1)
	frag.P2(42)
	jf.Write("fragmentsenc.txt", frag)

	// domainenc.txt: 1 entry "test".
	dom := packet.Alloc(2)
	dom.P4(1)
	dom.P1(4)
	for _, c := range []byte("test") {
		dom.P1(c)
	}
	jf.Write("domainenc.txt", dom)

	// tldlist.txt: 1 entry type=2 tld="com".
	tld := packet.Alloc(2)
	tld.P4(1)
	tld.P1(2) // tld type
	tld.P1(3)
	for _, c := range []byte("com") {
		tld.P1(c)
	}
	jf.Write("tldlist.txt", tld)

	// Round-trip through Save+NewJagfile so .FileQueue lands in .FileHash + .FileSize.
	tmpPath := t.TempDir() + "/wordenc.jag"
	if err := jf.Save(tmpPath); err != nil {
		t.Fatalf("Save synthetic jag: %v", err)
	}
	raw, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("read synthetic jag: %v", err)
	}
	out, err := jagfile.NewJagfile(packet.NewPacket(raw))
	if err != nil {
		t.Fatalf("parse synthetic jag: %v", err)
	}
	return out
}

func TestLoadFromJag_DecodesAllFourSections(t *testing.T) {
	jf := makeSyntheticJag(t)
	f, err := LoadFromJag(jf)
	if err != nil {
		t.Fatalf("LoadFromJag: %v", err)
	}
	if got := len(f.bads); got != 1 {
		t.Errorf("bads: got %d, want 1", got)
	}
	if got := string(f.bads[0]); got != "anal" {
		t.Errorf("bads[0]: got %q, want %q", got, "anal")
	}
	if got := len(f.badCombos[0]); got != 1 {
		t.Errorf("badCombos[0]: got %d, want 1", got)
	}
	if f.badCombos[0][0] != [2]int{3, 19} {
		t.Errorf("badCombos[0][0]: got %v, want [3 19]", f.badCombos[0][0])
	}
	if got := f.fragments; len(got) != 1 || got[0] != 42 {
		t.Errorf("fragments: got %v, want [42]", got)
	}
	if got := len(f.domains); got != 1 || string(f.domains[0]) != "test" {
		t.Errorf("domains: got %v, want [test]", f.domains)
	}
	if got := len(f.tlds); got != 1 || string(f.tlds[0]) != "com" || f.tldTypes[0] != 2 {
		t.Errorf("tlds: got %v / types=%v, want [com] / [2]", f.tlds, f.tldTypes)
	}
}

func TestEmpty_FilterIsIdentity(t *testing.T) {
	f := Empty()
	got := f.Filter("hello world")
	if got != "hello world" {
		t.Errorf("Empty().Filter: got %q, want %q", got, "hello world")
	}
}

// testFilter builds a *Filter wired with the given component data. Used by
// algorithmic E2E tests to avoid loading a real jagfile. Mirrors the internal
// Filter struct directly (T7 algorithmic tests; encfilter.go).
func testFilter(t *testing.T, bads [][]rune, badCombos [][][2]int, frags []uint16, doms [][]rune, tl [][]rune, tlTypes []int) *Filter {
	t.Helper()
	return &Filter{
		bads:      bads,
		badCombos: badCombos,
		fragments: frags,
		domains:   doms,
		tlds:      tl,
		tldTypes:  tlTypes,
	}
}

func TestFilter_PassesThroughCleanText(t *testing.T) {
	f := Empty()
	// These inputs are already fully lowercase so formatUppercases is a no-op.
	cases := []string{"hello world", "good morning!"}
	for _, in := range cases {
		got := f.Filter(in)
		if got != in {
			t.Errorf("Filter(%q): got %q, want %q (Empty passthrough)", in, got, in)
		}
	}
	// "I love RuneScape" → formatUppercases lowercases the 'S' mid-alpha-run:
	// 'R' starts a new run (uppercase, flagged stays true), 'u' makes flagged=false,
	// 'S' is then mid-run uppercase → lowercased to 's'.
	got := f.Filter("I love RuneScape")
	want := "I love Runescape"
	if got != want {
		t.Errorf("Filter(\"I love RuneScape\"): got %q, want %q", got, want)
	}
}

func TestFilter_MasksDirectBadWord(t *testing.T) {
	// "anal" with one combo [3,19] — matches the synthetic jag fixture above.
	f := testFilter(t,
		[][]rune{[]rune("anal")},
		[][][2]int{{{3, 19}}},
		nil, nil, nil, nil,
	)
	got := f.Filter("anal")
	if got != "****" {
		t.Errorf("Filter(anal): got %q, want ****", got)
	}
}

func TestFilter_WhitelistPreserves(t *testing.T) {
	// Empty Filter has no bad-word rules, so whitelist words pass through as-is.
	f := Empty()
	for _, w := range []string{"cook", "cooks", "cook's", "seeks", "sheet"} {
		got := f.Filter(w)
		if got != w {
			t.Errorf("Filter(%q): got %q, want %q", w, got, w)
		}
	}
}

func TestFilter_PreservesUppercaseOnPassthrough(t *testing.T) {
	f := Empty()
	got := f.Filter("Hello World")
	// "Hello World" → format is identity → trim identity → toLower "hello world"
	// → filters no-op → replaceUppercases restores 'H' and 'W' → formatUppercases
	// sees 'H' uppercase first in run (flagged=true, no lowercase before) → leaves
	// 'H'; 'e' lowercase → flagged=false; rest unchanged. Same for 'W'.
	if got != "Hello World" {
		t.Errorf("Filter(Hello World): got %q, want %q", got, "Hello World")
	}
}

func TestFilter_EmptyInput(t *testing.T) {
	f := Empty()
	if got := f.Filter(""); got != "" {
		t.Errorf("Filter(\"\"): got %q, want \"\"", got)
	}
}

// TestLoadFromFile_SyntheticRawDir pins the rev-244 path behavior: loadFromFile
// reads the jagfile at the supplied path (no subdir added) and returns a
// populated *Filter. Mirrors the TS WordEnc.ts:35-37 contract where
// WordEnc.load hardcodes "data/raw/wordenc" and passes it as a full path to
// Jagfile.load; goscape's Load(path) now takes that path as a parameter
// (world.Config.WordEncPath defaults to "data/raw/wordenc") instead of
// hardcoding it internally.
func TestLoadFromFile_SyntheticRawDir(t *testing.T) {
	// Build a synthetic jagfile and save it to a temp location named "wordenc"
	// to mirror the TS "data/raw/wordenc" layout.
	jf := makeSyntheticJag(t)
	rawDir := t.TempDir()
	jagPath := filepath.Join(rawDir, "wordenc")

	// Round-trip: save via jagfile.Save, then load via loadFromFile.
	if err := jf.Save(jagPath); err != nil {
		t.Fatalf("Save synthetic jag to %q: %v", jagPath, err)
	}

	f, err := loadFromFile(jagPath)
	if err != nil {
		t.Fatalf("loadFromFile(%q): %v", jagPath, err)
	}
	// Sanity: synthetic jag has one bad-word entry "anal" (from makeSyntheticJag).
	if got := len(f.bads); got != 1 {
		t.Errorf("bads count: got %d, want 1", got)
	}
	if got := string(f.bads[0]); got != "anal" {
		t.Errorf("bads[0]: got %q, want %q", got, "anal")
	}
}

// TestLoadFromFile_MissingFile pins that loadFromFile returns a non-nil error
// when the jagfile is absent. Rev-244: TS Jagfile.load throws on missing file;
// the silent-return-on-missing from rev-225 is gone.
func TestLoadFromFile_MissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "wordenc")
	_, err := loadFromFile(missing)
	if err == nil {
		t.Fatalf("loadFromFile on missing path: want error, got nil")
	}
}

// TestLoad_CustomAbsolutePath pins the embedding fix: Load(path) reads the
// jagfile at an arbitrary absolute path with no dependency on the process
// working directory — unlike the old no-arg Load(), which hardcoded the
// cwd-relative "data/raw/wordenc" internally. This is what lets a
// singleplayer binary embedding this server (running from any cwd) point
// world.Config.WordEncPath at an absolute path instead. Deliberately does
// NOT t.Chdir anywhere, to prove cwd independence.
func TestLoad_CustomAbsolutePath(t *testing.T) {
	jf := makeSyntheticJag(t)
	jagPath := filepath.Join(t.TempDir(), "wordenc")
	if err := jf.Save(jagPath); err != nil {
		t.Fatalf("Save synthetic jag to %q: %v", jagPath, err)
	}
	if !filepath.IsAbs(jagPath) {
		t.Fatalf("test setup: jagPath must be absolute, got %q", jagPath)
	}

	f, err := Load(jagPath)
	if err != nil {
		t.Fatalf("Load(%q): %v", jagPath, err)
	}
	if got := len(f.bads); got != 1 || string(f.bads[0]) != "anal" {
		t.Errorf("bads: got %v, want [anal]", f.bads)
	}
}

// TestLoad_DefaultRelativePath pins that Load, called with the literal
// world.Config.WordEncPath default ("data/raw/wordenc"), preserves the exact
// rev-244 TS-hardcode behavior: a relative path resolved against the process
// working directory. Complements TestLoad_CustomAbsolutePath (override case)
// and TestNewServer_LoadsWordencFilter in modules/world (real jagfile,
// exercised through world.Config).
func TestLoad_DefaultRelativePath(t *testing.T) {
	jf := makeSyntheticJag(t)
	dir := t.TempDir()
	// Lay the jag out at <dir>/data/raw/wordenc and chdir into <dir>, mirroring
	// how a server run from its repo root resolves the default.
	rawDir := filepath.Join(dir, "data", "raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatalf("MkdirAll %q: %v", rawDir, err)
	}
	if err := jf.Save(filepath.Join(rawDir, "wordenc")); err != nil {
		t.Fatalf("Save synthetic jag to %q: %v", rawDir, err)
	}
	t.Chdir(dir)

	const defaultWordEncPath = "data/raw/wordenc" // world.Config.WordEncPath default
	f, err := Load(defaultWordEncPath)
	if err != nil {
		t.Fatalf("Load(%q): %v", defaultWordEncPath, err)
	}
	if got := len(f.bads); got != 1 || string(f.bads[0]) != "anal" {
		t.Errorf("bads: got %v, want [anal]", f.bads)
	}
}

// TestLoad_MissingFile pins that Load(path) surfaces a non-nil error when the
// jagfile at path is absent — same contract as loadFromFile
// (TestLoadFromFile_MissingFile), exercised through the public Load entry
// point that world.NewServer actually calls.
func TestLoad_MissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "wordenc")
	if _, err := Load(missing); err == nil {
		t.Fatalf("Load(%q): want error, got nil", missing)
	}
}
