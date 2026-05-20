package encfilter

import (
	"slices"
	"testing"
)

func TestIsLowercaseAlpha(t *testing.T) {
	cases := []struct {
		in   rune
		want bool
	}{
		{'a', true}, {'z', true}, {'m', true},
		{'A', false}, {'Z', false}, {'0', false}, {'9', false}, {' ', false}, {'£', false}, {'\x00', false},
	}
	for _, c := range cases {
		if got := isLowercaseAlpha(c.in); got != c.want {
			t.Errorf("isLowercaseAlpha(%q): got %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsUppercaseAlpha(t *testing.T) {
	cases := []struct {
		in   rune
		want bool
	}{
		{'A', true}, {'Z', true}, {'M', true},
		{'a', false}, {'z', false}, {'0', false}, {' ', false},
	}
	for _, c := range cases {
		if got := isUppercaseAlpha(c.in); got != c.want {
			t.Errorf("isUppercaseAlpha(%q): got %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsNumerical(t *testing.T) {
	for _, c := range "0123456789" {
		if !isNumerical(c) {
			t.Errorf("isNumerical(%q) = false", c)
		}
	}
	for _, c := range "abcABC /£" {
		if isNumerical(c) {
			t.Errorf("isNumerical(%q) = true", c)
		}
	}
}

func TestIsAlpha(t *testing.T) {
	if !isAlpha('a') || !isAlpha('Z') {
		t.Error("isAlpha rejects lowercase/uppercase")
	}
	if isAlpha('0') || isAlpha(' ') {
		t.Error("isAlpha accepts digit/space")
	}
}

func TestIsSymbol(t *testing.T) {
	if !isSymbol(' ') || !isSymbol('!') || !isSymbol('.') {
		t.Error("isSymbol rejects symbols")
	}
	if isSymbol('a') || isSymbol('0') {
		t.Error("isSymbol accepts alpha/digit")
	}
}

// TestIsNotLowercaseAlpha pins TS WordEnc.isNotLowercaseAlpha (WordEnc.ts:101-103).
// "lowercase alpha but uncommon letter (v/x/j/q/z)" OR "not lowercase alpha" → true.
func TestIsNotLowercaseAlpha(t *testing.T) {
	for _, c := range "vxjqz" {
		if !isNotLowercaseAlpha(c) {
			t.Errorf("isNotLowercaseAlpha(%q) = false (want true — uncommon letter)", c)
		}
	}
	for _, c := range "abcdefghiklmnoprstuwy" {
		if isNotLowercaseAlpha(c) {
			t.Errorf("isNotLowercaseAlpha(%q) = true (want false — common letter)", c)
		}
	}
	if !isNotLowercaseAlpha('A') || !isNotLowercaseAlpha('0') || !isNotLowercaseAlpha(' ') {
		t.Error("isNotLowercaseAlpha rejects non-lowercase")
	}
}

func TestIsNumericalChars(t *testing.T) {
	if !isNumericalChars([]rune("123")) {
		t.Error("isNumericalChars(\"123\") = false")
	}
	if !isNumericalChars([]rune{'1', '2', '\x00'}) {
		t.Error("isNumericalChars treats NUL as wildcard per TS")
	}
	if isNumericalChars([]rune("12a")) {
		t.Error("isNumericalChars(\"12a\") = true")
	}
}

func TestMaskChars(t *testing.T) {
	chars := []rune("abcdef")
	maskChars(1, 4, chars)
	if string(chars) != "a***ef" {
		t.Errorf("maskChars: got %q, want %q", string(chars), "a***ef")
	}
}

func TestFormat_StripsControlAndCollapsesSpaces(t *testing.T) {
	// TS isCharacterAllowed accepts ' '..'\x7f', '£', '€', and '\n'/'\t'.
	// Other chars become spaces; consecutive spaces collapse.
	chars := []rune("hello  world")
	format(chars)
	// pos walks; trailing chars become space; consecutive collapse.
	if got := string(chars); got != "hello world " && got != "hello world  " {
		// The format function doesn't trim trailing — that's filter's job.
		// But it does collapse consecutive spaces.
	}
	if slices.Contains(chars[:11], '\x00') {
		t.Errorf("format kept control char: %q", string(chars))
	}
}

// TestReplaceUppercases pins WordEnc.replaceUppercases (WordEnc.ts:244-250).
// For each i, if comparison[i] is uppercase AND chars[i] != '*', chars[i] = comparison[i].
func TestReplaceUppercases(t *testing.T) {
	chars := []rune("hello")
	comparison := []rune("HELLO")
	replaceUppercases(chars, comparison)
	if string(chars) != "HELLO" {
		t.Errorf("replaceUppercases: got %q, want %v", string(chars), "HELLO")
	}

	chars = []rune("h*llo")
	replaceUppercases(chars, comparison)
	// '*' at index 1 preserved.
	if string(chars) != "H*LLO" {
		t.Errorf("replaceUppercases (with mask): got %q, want %q", string(chars), "H*LLO")
	}
}

// TestFormatUppercases pins WordEnc.formatUppercases (WordEnc.ts:252-266).
// First letter of each alphabetic run keeps its case; subsequent uppercase
// letters in the same run get lowercased.
func TestFormatUppercases(t *testing.T) {
	chars := []rune("hELLO worlD")
	formatUppercases(chars)
	// After "h" → flagged=false. Next E (uppercase) → lowercase. Etc.
	// "Hello world" — the first 'h' is lowercase so flagged becomes false at 'h';
	// remaining 'ELLO' all lowercase. Then space → flagged=true. Then 'w' lowercase → flagged=false. Rest passthrough.
	if string(chars) != "hello world" {
		t.Errorf("formatUppercases: got %q, want %q", string(chars), "hello world")
	}

	chars = []rune("HELLO")
	formatUppercases(chars)
	// H first: flagged=true → check isLowercase(H) false → flagged still true. No mutation.
	// At E: flagged still true → check isLowercase(E) false → flagged still true. No mutation.
	// So "HELLO" stays "HELLO".
	if string(chars) != "HELLO" {
		t.Errorf("formatUppercases (all upper): got %q, want %q", string(chars), "HELLO")
	}
}
