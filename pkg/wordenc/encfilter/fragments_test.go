package encfilter

import "testing"

// TestFragments_getInteger pins the base-38 encoding for fragment lookup:
// 'a'..'z' → 1..26, "'" → 27, '0'..'9' → 28..37. (Engine-TS/src/cache/
// wordenc/WordEncFragments.ts:79-97.)
func TestFragments_getInteger_KnownEncoding(t *testing.T) {
	// "a" → 1 (1 char, base 38)
	if got := getFragmentInteger([]rune("a")); got != 1 {
		t.Errorf("getFragmentInteger(a): got %d, want 1", got)
	}
	// "ab" → reversed traversal: from chars[len-1]='b' (value 2 * 38^0 = 2)
	// then chars[0]='a' (value 2 * 38 + 1 = 77).
	if got := getFragmentInteger([]rune("ab")); got != 77 {
		t.Errorf("getFragmentInteger(ab): got %d, want 77", got)
	}
	// More than 6 chars → 0.
	if got := getFragmentInteger([]rune("abcdefg")); got != 0 {
		t.Errorf("getFragmentInteger(7-char): got %d, want 0", got)
	}
	// NUL accepted but yields no contribution: reversed iteration processes
	// chars[1]=NUL first (no contribution), then chars[0]='a' → value = 0*38+1 = 1.
	if got := getFragmentInteger([]rune{'a', '\x00'}); got != 1 {
		t.Errorf("getFragmentInteger(a + NUL): got %d, want 1", got)
	}
}

// TestFragments_isBadFragment_BinarySearch builds a synthetic fragments list
// and checks lookup.
func TestFragments_isBadFragment(t *testing.T) {
	// Construct sorted fragments containing the encoded value for "a" (=1)
	// and "ab" (=77).
	frags := &fragments{items: []uint16{1, 77, 200}}
	if !frags.isBadFragment([]rune("a")) {
		t.Error("isBadFragment(a) = false")
	}
	if !frags.isBadFragment([]rune("ab")) {
		t.Error("isBadFragment(ab) = false")
	}
	if frags.isBadFragment([]rune("z")) {
		t.Error("isBadFragment(z) = true (value 26 not in list)")
	}
	// All-digit chars → always bad per isNumericalChars short-circuit.
	if !frags.isBadFragment([]rune("123")) {
		t.Error("isBadFragment(123) = false (numerical chars)")
	}
}

// TestFragments_filter_NoMaskWhenCommonAlphaPrecedes checks the filter loop
// logic for the common case (TS WordEncFragments.ts:6-48). When common
// lowercase letters (not v/x/j/q/z) precede the digit run, startIndex stays
// low and no masking occurs in isolation. The TS mask path (startIndex==4)
// only fires after the broader filter pipeline has pre-masked surrounding chars.
func TestFragments_filter_NoMaskWhenCommonAlphaPrecedes(t *testing.T) {
	frags := &fragments{}
	chars := []rune("call 12345 now")
	frags.filter(chars)
	// The TS algorithm does not mask in this case (startIndex never reaches 4
	// in a single outer-loop iteration given local startIndex resets each pass).
	if containsRune(chars, '*') {
		t.Errorf("filter: unexpected masking in %q", string(chars))
	}
}

// TestFragments_filter_AdvancesPastDigitRuns checks that filter does not hang
// (infinite loop guard) when processing digit sequences.
func TestFragments_filter_AdvancesPastDigitRuns(t *testing.T) {
	frags := &fragments{}
	// Input with multiple digit groups — just verify it terminates.
	chars := []rune("abc 123 xyz 456 end")
	frags.filter(chars) // must return (not loop forever)
}

func containsRune(rs []rune, target rune) bool {
	for _, r := range rs {
		if r == target {
			return true
		}
	}
	return false
}
