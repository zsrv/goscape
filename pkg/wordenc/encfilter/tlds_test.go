package encfilter

import (
	"slices"
	"testing"
)

// TestTlds_filter_MasksBareUrl — input "go to foo.com please" should mask
// "foo.com" (or a superset of it) given a TLD list with "com" type 2.
func TestTlds_filter_MasksBareUrl(t *testing.T) {
	tl := &tlds{
		bads:    &badWords{fragments_: &fragments{}},
		domains: &domains{bads: &badWords{fragments_: &fragments{}}},
		tlds:    [][]rune{[]rune("com")},
		types:   []int{2},
	}
	chars := []rune("go to foo.com please")
	tl.filter(chars)
	// "com" at 10..13 should be masked. Plus possibly "foo." preceding given the
	// period traversal at TS lines 71-81.
	if !slices.Contains(chars, '*') {
		t.Errorf("tlds.filter: nothing masked in %q", string(chars))
	}
}
