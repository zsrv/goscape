package fonttype

import (
	"os"
	"path/filepath"
	"testing"
)

// ref274PackDir resolves the Server274-ref cache-pack directory (the dir
// that contains client/title) for the rev-274 *_full 256-glyph fonts from
// GOSCAPE_REF274_DIR (pointing at .../engine, pack derived as data/pack).
// Returns "" when the env var is unset or its pack lacks client/title.
//
// The local repo data/pack is still 254-era (p11.dat etc., NO *_full)
// until plan T21 refreshes it, so the real-cache Load tests must point at
// the 274 ref cache to exercise the new contract.
func ref274PackDir() string {
	if ref := os.Getenv("GOSCAPE_REF274_DIR"); ref != "" {
		dir := filepath.Join(ref, "data", "pack")
		if _, err := os.Stat(filepath.Join(dir, "client", "title")); err == nil {
			return dir
		}
	}
	return ""
}

func load274(t *testing.T) []*FontType {
	t.Helper()
	dir := ref274PackDir()
	if dir == "" {
		t.Skip("Server274-ref cache not available (set GOSCAPE_REF274_DIR or provision the worktree)")
	}
	fonts, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return fonts
}

func TestLoad_FourFonts(t *testing.T) {
	fonts := load274(t)
	if len(fonts) != 4 {
		t.Fatalf("len(fonts): got %d, want 4 (p11_full/p12_full/b12_full/q8_full)", len(fonts))
	}
	for i, f := range fonts {
		if f == nil {
			t.Errorf("fonts[%d]: nil", i)
		}
	}
}

// TestLoad_HeightGuard pins the rev-274 height contract: height is the
// tallest glyph height among codes < 128 only (the `c < 128` guard).
// Values captured from the real Server274-ref *_full fonts.
func TestLoad_HeightGuard(t *testing.T) {
	fonts := load274(t)
	want := []int{10, 12, 12, 15} // p11_full, p12_full, b12_full, q8_full
	for i, w := range want {
		if got := fonts[i].Height(); got != w {
			t.Errorf("fonts[%d].Height(): got %d, want %d", i, got, w)
		}
	}
}

// TestDrawWidth_PerCharCode pins the 256-glyph direct-index advance fill.
// Char codes >= 128 (e.g. £ 0xA3, À 0xC0, ÿ 0xFF) now carry their own
// glyph advance instead of the old CHAR_LOOKUP=74 fallback. Values
// captured from the real Server274-ref fonts.
func TestDrawWidth_PerCharCode(t *testing.T) {
	fonts := load274(t)
	type wc struct {
		code byte
		want byte
	}
	cases := map[string][]wc{
		"p11_full": {{'A', 7}, {'a', 6}, {'i', 3}, {'I', 3}, {'0', 7}, {0xA3, 7}, {0xC0, 7}, {0xFF, 6}},
		"p12_full": {{'A', 8}, {'a', 7}, {'i', 3}, {'I', 3}, {'0', 8}, {0xA3, 10}, {0xC0, 8}, {0xFF, 6}},
		"b12_full": {{'A', 9}, {'a', 8}, {'i', 4}, {'I', 6}, {'0', 9}, {0xA3, 11}, {0xC0, 9}, {0xFF, 8}},
		"q8_full":  {{'A', 9}, {'a', 7}, {'i', 3}, {'I', 7}, {'0', 9}, {0xA3, 10}, {0xC0, 9}, {0xFF, 9}},
	}
	idx := map[int]string{0: "p11_full", 1: "p12_full", 2: "b12_full", 3: "q8_full"}
	for i, name := range idx {
		for _, c := range cases[name] {
			if got := fonts[i].DrawWidth(c.code); got != c.want {
				t.Errorf("%s.DrawWidth(%d %q): got %d, want %d", name, c.code, string(c.code), got, c.want)
			}
		}
	}
}

// TestQuillSpaceRule pins the space-advance rule: for the quill font
// (q8_full) the space (code 32) copies glyph 73 (`I`); for the non-quill
// fonts it copies glyph 105 (`i`). TS FontType.ts:101-105.
func TestQuillSpaceRule(t *testing.T) {
	fonts := load274(t)
	// Non-quill: space == advance of 'i' (105).
	for i := range 3 {
		if got, want := fonts[i].DrawWidth(' '), fonts[i].DrawWidth('i'); got != want {
			t.Errorf("non-quill font %d: space advance %d, want == 'i' advance %d", i, got, want)
		}
	}
	// Quill (q8_full, id 3): space == advance of 'I' (73).
	if got, want := fonts[3].DrawWidth(' '), fonts[3].DrawWidth('I'); got != want {
		t.Errorf("quill q8_full: space advance %d, want == 'I' advance %d", got, want)
	}
	// And concretely: q8_full space == 7 (its 'I' advance), NOT its 'i'
	// advance of 3 — proves the quill branch is taken.
	if got := fonts[3].DrawWidth(' '); got != 7 {
		t.Errorf("q8_full space advance: got %d, want 7 (== 'I')", got)
	}
	if fonts[3].DrawWidth('i') == fonts[3].DrawWidth(' ') {
		t.Errorf("q8_full space must NOT equal 'i' advance (quill copies 'I' not 'i')")
	}
}

func TestFontType_StringWidth_Empty(t *testing.T) {
	fonts := load274(t)
	if w := fonts[0].StringWidth(""); w != 0 {
		t.Errorf("StringWidth(\"\"): got %d, want 0", w)
	}
}

func TestFontType_StringWidth_AtColorEscape(t *testing.T) {
	fonts := load274(t)
	// "@xxx@" is a 4-char skip; "@cya@hi" has the same width as "hi".
	plain := fonts[0].StringWidth("hi")
	withColor := fonts[0].StringWidth("@cya@hi")
	if plain != withColor {
		t.Errorf("StringWidth with color escape: got %d, want %d", withColor, plain)
	}
}

func TestFontType_Split_EmptyString(t *testing.T) {
	fonts := load274(t)
	got := fonts[0].Split("", 100)
	if len(got) != 1 || got[0] != "" {
		t.Errorf("Split(\"\", 100): got %v, want [\"\"]", got)
	}
}

func TestFontType_Split_NoBreakNeeded(t *testing.T) {
	fonts := load274(t)
	got := fonts[0].Split("hi", 1000)
	if len(got) != 1 || got[0] != "hi" {
		t.Errorf("Split: got %v, want [\"hi\"]", got)
	}
}

func TestFontType_Split_OnPipe(t *testing.T) {
	fonts := load274(t)
	got := fonts[0].Split("alpha|beta|gamma", 10000)
	want := []string{"alpha", "beta", "gamma"}
	if !equalStrings(got, want) {
		t.Errorf("Split on '|': got %v, want %v", got, want)
	}
}

func TestFontType_Split_OnSpace_ExceedsMaxWidth(t *testing.T) {
	fonts := load274(t)
	src := "alpha bravo charlie delta echo foxtrot golf hotel india"
	full := fonts[0].StringWidth(src)
	maxWidth := full / 2
	got := fonts[0].Split(src, maxWidth)
	if len(got) < 2 {
		t.Fatalf("Split: got %d lines, want >= 2 (maxWidth=%d, full=%d, src=%q)", len(got), maxWidth, full, src)
	}
	for i, line := range got {
		if w := fonts[0].StringWidth(line); w > maxWidth {
			t.Errorf("Split line %d %q has width %d > maxWidth %d", i, line, w, maxWidth)
		}
	}
}

func TestFontType_Split_NoSpaceForcesFullLine(t *testing.T) {
	fonts := load274(t)
	got := fonts[0].Split("AAAAAAAAAAAAAAAAAAAAA", 5)
	if len(got) != 1 || got[0] != "AAAAAAAAAAAAAAAAAAAAA" {
		t.Errorf("Split (no space, overflow): got %v, want [%q]", got, "AAAAAAAAAAAAAAAAAAAAA")
	}
}

// --- Split colour-persistence (rev-274 FontType.ts:160-193) ---
//
// These exercise the savedCol carry-across-breaks logic added in 274.
// They use a synthetic font so the wrap points are deterministic and do
// not depend on real glyph metrics (the colour transform is independent
// of glyph widths once a break occurs).

// fixedFont returns a FontType where every char code advances by 1, so
// StringWidth(s) == len(s) (ignoring @xxx@ escapes). Lets tests pick exact
// break columns via maxWidth.
func fixedFont() *FontType {
	f := &FontType{height: 8}
	for i := range f.drawWidth {
		f.drawWidth[i] = 1
	}
	return f
}

// TestSplit_ColourCarry: a colour opened on an earlier line is re-applied
// to the start of the next line (TS: `str = savedCol + str`).
func TestSplit_ColourCarry(t *testing.T) {
	f := fixedFont()
	// "@red@aaaa bbbb" — width(@red@aaaa)=4 fits in 4; next break carries
	// @red@ onto the "bbbb" continuation. (Verified vs TS split @dee467c8.)
	got := f.Split("@red@aaaa bbbb", 4)
	want := []string{"@red@aaaa", "@red@bbbb"}
	if !equalStrings(got, want) {
		t.Errorf("colour carry: got %v, want %v", got, want)
	}
}

// TestSplit_ColourReset: an @str@ on the line clears the saved colour so
// nothing is carried forward.
func TestSplit_ColourReset(t *testing.T) {
	f := fixedFont()
	// width(@red@aaaa@str@)=4 fits in 4; @str@ clears savedCol so nothing
	// carries onto "bbbb". (Verified vs TS split @dee467c8.)
	got := f.Split("@red@aaaa@str@ bbbb", 4)
	want := []string{"@red@aaaa@str@", "bbbb"}
	if !equalStrings(got, want) {
		t.Errorf("colour reset: got %v, want %v", got, want)
	}
}

// TestSplit_ColourCarry_InsertBla: when the saved colour is carried and
// the continuation already contains an @str@, TS inserts @bla@ after it
// (rather than prefixing) so the reset point keeps a sane default.
func TestSplit_ColourCarry_InsertBla(t *testing.T) {
	f := fixedFont()
	// First line "@red@aa" (width 2) fits in 6; continuation
	// "bb@str@cc" gets @bla@ inserted after its @str@.
	got := f.Split("@red@aa bb@str@cc", 6)
	want := []string{"@red@aa", "bb@str@@bla@cc"}
	if !equalStrings(got, want) {
		t.Errorf("colour carry + insert @bla@: got %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
