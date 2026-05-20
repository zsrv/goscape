package encfilter

import "testing"

// TestDomains_getEmulatedDomainCharLen pins the (smaller) domain-leet switch
// (WordEncDomains.ts:23-40).
func TestDomains_getEmulatedDomainCharLen(t *testing.T) {
	if got := getEmulatedDomainCharLen(0, 'a', 'a'); got != 1 {
		t.Errorf("a→a: got %d", got)
	}
	if got := getEmulatedDomainCharLen(0, 'o', '0'); got != 1 {
		t.Errorf("o→0: got %d", got)
	}
	if got := getEmulatedDomainCharLen(')', 'o', '('); got != 2 {
		t.Errorf("o→(): got %d", got)
	}
	if got := getEmulatedDomainCharLen(0, 'c', '('); got != 1 {
		t.Errorf("c→(: got %d", got)
	}
	if got := getEmulatedDomainCharLen(0, 'e', '€'); got != 1 {
		t.Errorf("e→€: got %d", got)
	}
	if got := getEmulatedDomainCharLen(0, 's', '$'); got != 1 {
		t.Errorf("s→$: got %d", got)
	}
	if got := getEmulatedDomainCharLen(0, 'l', 'i'); got != 1 {
		t.Errorf("l→i: got %d", got)
	}
	if got := getEmulatedDomainCharLen(0, 'z', 'a'); got != 0 {
		t.Errorf("z→a: got %d (want 0)", got)
	}
}

// TestDomains_filter_MasksEmailContext — TS suffixSymbolStatus must
// detect period/comma → status 3 → mask. Constructed: "foo@test.com" should
// have "test" masked.
func TestDomains_filter_MasksEmailContext(t *testing.T) {
	d := &domains{
		bads:    &badWords{fragments_: &fragments{}},
		domains: [][]rune{[]rune("test")},
	}
	chars := []rune("foo@test.com")
	d.filter(chars)
	// "test" at indices 4..7 should be masked given @ before and . after.
	if string(chars[4:8]) != "****" {
		t.Errorf("domains.filter: got %q, want %q at offset 4..8 (full=%q)", string(chars[4:8]), "****", string(chars))
	}
}
