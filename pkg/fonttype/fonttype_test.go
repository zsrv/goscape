package fonttype

import (
	"os"
	"path/filepath"
	"testing"
)

// ref244PackDir resolves the rev-244 reference cache pack dir from
// GOSCAPE_REF244_DIR, or "" when unset/unavailable. The repo's own data/pack
// is revision-specific generated output shared across worktrees; pinning to the
// per-branch reference cache avoids the cross-revision-cache hazard.
func ref244PackDir() string {
	if ref := os.Getenv("GOSCAPE_REF244_DIR"); ref != "" {
		dir := filepath.Join(ref, "data", "pack")
		if _, err := os.Stat(filepath.Join(dir, "client", "title")); err == nil {
			return dir
		}
	}
	return ""
}

func TestCharLookup_AsciiA(t *testing.T) {
	if CharLookup['A'] != 0 {
		t.Errorf("CharLookup['A']: got %d, want 0", CharLookup['A'])
	}
	if CharLookup['a'] != 26 {
		t.Errorf("CharLookup['a']: got %d, want 26", CharLookup['a'])
	}
	if CharLookup['0'] != 52 {
		t.Errorf("CharLookup['0']: got %d, want 52", CharLookup['0'])
	}
	// Charset layout: 26 upper + 26 lower + 10 digits + 33 specials =
	// 95 char positions; space is the last char at rune index 94.
	// TS charAdvance[94] = charAdvance[8] handles the missing-from-loop
	// space glyph; CharLookup[' '] points to slot 94.
	if CharLookup[' '] != 94 {
		t.Errorf("CharLookup[' ']: got %d, want 94", CharLookup[' '])
	}
}

func TestCharLookup_Unknown(t *testing.T) {
	if CharLookup[0x01] != 74 {
		t.Errorf("CharLookup[0x01]: got %d, want 74 (unmapped fallback)", CharLookup[0x01])
	}
}

func TestLoad_FourFonts(t *testing.T) {
	cacheDir := ref244PackDir()
	if cacheDir == "" {
		t.Skip("Server244-ref cache not available; set GOSCAPE_REF244_DIR")
	}
	fonts, err := Load(cacheDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(fonts) != 4 {
		t.Fatalf("len(fonts): got %d, want 4 (p11/p12/b12/q8)", len(fonts))
	}
	for i, f := range fonts {
		if f == nil {
			t.Errorf("fonts[%d]: nil", i)
		}
	}
}

func TestFontType_StringWidth_Empty(t *testing.T) {
	cacheDir := ref244PackDir()
	if cacheDir == "" {
		t.Skip("Server244-ref cache not available; set GOSCAPE_REF244_DIR")
	}
	fonts, err := Load(cacheDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if w := fonts[0].StringWidth(""); w != 0 {
		t.Errorf("StringWidth(\"\"): got %d, want 0", w)
	}
}

func TestFontType_StringWidth_AtColorEscape(t *testing.T) {
	cacheDir := ref244PackDir()
	if cacheDir == "" {
		t.Skip("Server244-ref cache not available; set GOSCAPE_REF244_DIR")
	}
	fonts, err := Load(cacheDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// TS FontType.stringWidth treats "@xxx@" as a 4-char skip (the second
	// '@' lands at c+4; the for-loop's c++ advances past it). Net effect:
	// "@cya@hi" has the same width as "hi".
	plain := fonts[0].StringWidth("hi")
	withColor := fonts[0].StringWidth("@cya@hi")
	if plain != withColor {
		t.Errorf("StringWidth with color escape: got %d, want %d", withColor, plain)
	}
}

func TestFontType_Split_EmptyString(t *testing.T) {
	cacheDir := ref244PackDir()
	if cacheDir == "" {
		t.Skip("Server244-ref cache not available; set GOSCAPE_REF244_DIR")
	}
	fonts, err := Load(cacheDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := fonts[0].Split("", 100)
	if len(got) != 1 || got[0] != "" {
		t.Errorf("Split(\"\", 100): got %v, want [\"\"]", got)
	}
}

func TestFontType_Split_NoBreakNeeded(t *testing.T) {
	cacheDir := ref244PackDir()
	if cacheDir == "" {
		t.Skip("Server244-ref cache not available; set GOSCAPE_REF244_DIR")
	}
	fonts, err := Load(cacheDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// "hi" is short; with maxWidth large enough it fits on one line.
	got := fonts[0].Split("hi", 1000)
	if len(got) != 1 || got[0] != "hi" {
		t.Errorf("Split: got %v, want [\"hi\"]", got)
	}
}

func TestFontType_Split_OnPipe(t *testing.T) {
	cacheDir := ref244PackDir()
	if cacheDir == "" {
		t.Skip("Server244-ref cache not available; set GOSCAPE_REF244_DIR")
	}
	fonts, err := Load(cacheDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := fonts[0].Split("alpha|beta|gamma", 10000)
	want := []string{"alpha", "beta", "gamma"}
	if !equalStrings(got, want) {
		t.Errorf("Split on '|': got %v, want %v", got, want)
	}
}

func TestFontType_Split_OnSpace_ExceedsMaxWidth(t *testing.T) {
	cacheDir := ref244PackDir()
	if cacheDir == "" {
		t.Skip("Server244-ref cache not available; set GOSCAPE_REF244_DIR")
	}
	fonts, err := Load(cacheDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Build a long string of single-word groups separated by spaces and
	// derive a maxWidth that forces at least one break.
	src := "alpha bravo charlie delta echo foxtrot golf hotel india"
	full := fonts[0].StringWidth(src)
	// Halve the width so a break is mandatory.
	maxWidth := full / 2
	got := fonts[0].Split(src, maxWidth)
	if len(got) < 2 {
		t.Fatalf("Split: got %d lines, want >= 2 (maxWidth=%d, full=%d, src=%q)", len(got), maxWidth, full, src)
	}
	for i, line := range got {
		if w := fonts[0].StringWidth(line); w > maxWidth {
			// Exception: a single overflowing word with no space boundary
			// inside maxWidth must still be emitted (TS:159-170 default).
			// All test words here are short enough to fit individually.
			t.Errorf("Split line %d %q has width %d > maxWidth %d", i, line, w, maxWidth)
		}
	}
}

func TestFontType_Split_NoSpaceForcesFullLine(t *testing.T) {
	cacheDir := ref244PackDir()
	if cacheDir == "" {
		t.Skip("Server244-ref cache not available; set GOSCAPE_REF244_DIR")
	}
	fonts, err := Load(cacheDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// One long word, no space — TS default splitIndex == str.length means
	// the whole word is emitted as a single line (overflowing maxWidth).
	got := fonts[0].Split("AAAAAAAAAAAAAAAAAAAAA", 5)
	if len(got) != 1 || got[0] != "AAAAAAAAAAAAAAAAAAAAA" {
		t.Errorf("Split (no space, overflow): got %v, want [%q]", got, "AAAAAAAAAAAAAAAAAAAAA")
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
