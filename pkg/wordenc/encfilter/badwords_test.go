package encfilter

import "testing"

// TestBadWords_getIndex pins the combo-key mapping (WordEncBadWords.ts:375-384).
//
//	'a'..'z' → 1..26
//	"'"      → 28
//	'0'..'9' → 29..38
//	other    → 27
func TestBadWords_getIndex(t *testing.T) {
	cases := []struct {
		in   rune
		want int
	}{
		{'a', 1}, {'z', 26},
		{'\'', 28},
		{'0', 29}, {'9', 38},
		{' ', 27}, {'.', 27}, {'A', 27},
	}
	for _, c := range cases {
		if got := badGetIndex(c.in); got != c.want {
			t.Errorf("badGetIndex(%q): got %d, want %d", c.in, got, c.want)
		}
	}
}

// TestBadWords_getEmulatedBadCharLen_DirectMatch covers the trivial case
// (badChar == currentChar → 1).
func TestBadWords_getEmulatedBadCharLen_DirectMatch(t *testing.T) {
	if got := getEmulatedBadCharLen(0, 'a', 'a'); got != 1 {
		t.Errorf("a→a: got %d, want 1", got)
	}
}

// TestBadWords_getEmulatedBadCharLen_Leetspeak covers single-char leet substitutions
// (WordEncBadWords.ts:186-355).
func TestBadWords_getEmulatedBadCharLen_Leetspeak(t *testing.T) {
	cases := []struct {
		next, bad, current rune
		want               int
	}{
		{0, 'a', '4', 0}, // want=0 sentinel → resolved to 1 below (not in wants2)
		{0, 'a', '@', 0},
		{0, 'a', '^', 0},
		{'\\', 'a', '/', 0}, // 2-char: a→/\, resolved via wants2
		{0, 'b', '6', 0},
		{0, 'b', '8', 0},
		{'3', 'b', '1', 0}, // 2-char: b→13, resolved via wants2
		{0, 'e', '3', 0},
		{0, 'e', '€', 0},
		{0, 'l', '1', 0},
		{0, 'l', '|', 0},
		{0, 'l', 'i', 0},
		{0, 's', '5', 0}, {0, 's', '$', 0}, {0, 's', '2', 0}, {0, 's', 'z', 0},
		{0, 't', '7', 0}, {0, 't', '+', 0},
		{0, 'u', 'v', 0},
	}
	// wants2 lists the cases that return 2 (two-char substitution); all others return 1.
	wants2 := map[[3]rune]int{
		{'\\', 'a', '/'}: 2,
		{'3', 'b', '1'}:  2,
	}
	for _, c := range cases {
		want := 1
		if v, ok := wants2[[3]rune{c.next, c.bad, c.current}]; ok {
			want = v
		}
		if got := getEmulatedBadCharLen(c.next, c.bad, c.current); got != want {
			t.Errorf("getEmulatedBadCharLen(next=%q bad=%q cur=%q): got %d, want %d", c.next, c.bad, c.current, got, want)
		}
	}
}

// TestBadWords_getEmulatedBadCharLen_NoMatch covers chars that do not leet-match.
func TestBadWords_getEmulatedBadCharLen_NoMatch(t *testing.T) {
	cases := [][3]rune{
		{0, 'a', 'b'},  // 'b' does not substitute for 'a'
		{0, 'j', 'x'},  // 'j' has no substitutions (any non-j char)
		{0, 'n', 'x'},  // 'n' has no substitutions
		{0, 'm', 'x'},  // 'm' has no substitutions
		{0, 'k', 'x'},  // 'k' has no substitutions
		{0, 's', 'x'},  // 'x' does not substitute for 's'
	}
	for _, c := range cases {
		if got := getEmulatedBadCharLen(c[0], c[1], c[2]); got != 0 {
			t.Errorf("getEmulatedBadCharLen(next=%q bad=%q cur=%q): got %d, want 0", c[0], c[1], c[2], got)
		}
	}
}

// TestBadWords_filter_MasksDirectMatch is the smallest end-to-end test:
// a bad word "anal" with no combos should mask exactly those 4 chars on direct
// input "anal" (no surrounding context to trigger combo/symbol gating).
func TestBadWords_filter_MasksDirectMatch(t *testing.T) {
	b := &badWords{
		bads:       [][]rune{[]rune("anal")},
		combos:     [][][2]int{nil}, // no combos
		fragments_: &fragments{},
	}
	chars := []rune("anal")
	b.filter(chars)
	if string(chars) != "****" {
		t.Errorf("filter direct match: got %q, want %q", string(chars), "****")
	}
}

// TestBadWords_filter_PreservesNonMatch confirms unrelated text is untouched.
func TestBadWords_filter_PreservesNonMatch(t *testing.T) {
	b := &badWords{
		bads:       [][]rune{[]rune("anal")},
		combos:     [][][2]int{nil},
		fragments_: &fragments{},
	}
	chars := []rune("hello")
	b.filter(chars)
	if string(chars) != "hello" {
		t.Errorf("filter non-match: got %q, want %q", string(chars), "hello")
	}
}

// TestBadWords_filter_Leetspeak — "4n4l" should mask (since 'a'→'4' is allowed
// per getEmulatedBadCharLen). The chars have no surrounding symbols, so the
// !hasSymbol branch with combos==nil gates: in that branch shouldFilter only
// flips false via combo match, and combos is nil → shouldFilter stays true.
func TestBadWords_filter_Leetspeak(t *testing.T) {
	b := &badWords{
		bads:       [][]rune{[]rune("anal")},
		combos:     [][][2]int{nil},
		fragments_: &fragments{},
	}
	chars := []rune("4n4l")
	b.filter(chars)
	if string(chars) != "****" {
		t.Errorf("filter leetspeak: got %q, want %q", string(chars), "****")
	}
}

// TestBadWords_comboMatches binary search.
func TestBadWords_comboMatches(t *testing.T) {
	combos := [][2]int{{3, 19}, {15, 25}, {27, 14}}
	if !badComboMatches(3, combos, 19) {
		t.Error("comboMatches(3, 19) = false")
	}
	if !badComboMatches(15, combos, 25) {
		t.Error("comboMatches(15, 25) = false")
	}
	if badComboMatches(3, combos, 20) {
		t.Error("comboMatches(3, 20) = true")
	}
}

// TestBadWords_filter_EmbeddedInLargerString checks that "analsex" masks
// the "anal" portion when "anal" is in the bad list with no combos.
// (No suffix combo to gate it; embedded match should still fire.)
func TestBadWords_filter_EmbeddedInLargerString(t *testing.T) {
	b := &badWords{
		bads:       [][]rune{[]rune("anal")},
		combos:     [][][2]int{nil},
		fragments_: &fragments{},
	}
	chars := []rune("analsex")
	b.filter(chars)
	got := string(chars)
	// The first 4 chars should be masked; "sex" unchanged.
	if got[:4] != "****" {
		t.Errorf("filter embedded: first 4 chars got %q, want ****", got[:4])
	}
	if got[4:] != "sex" {
		t.Errorf("filter embedded: suffix got %q, want sex", got[4:])
	}
}

// TestBadWords_filter_NumeralCountGate verifies that a match where numeral
// count > alpha count is NOT masked (numeralCount > alphaCount → skip).
// bad="ab", input="12" — match via numeral substitution of a→1, b→?
// Actually 'b' has no numeral sub... let's use bad="as" with input="45":
// a→4 (numeral), s→5 (numeral). numeralCount=2, alphaCount=0 → 2>0 → skip.
func TestBadWords_filter_NumeralCountGate(t *testing.T) {
	b := &badWords{
		bads:       [][]rune{[]rune("as")},
		combos:     [][][2]int{nil},
		fragments_: &fragments{},
	}
	chars := []rune("45")
	b.filter(chars)
	// numeralCount=2, alphaCount=0 → NOT masked
	if string(chars) != "45" {
		t.Errorf("numeral gate: got %q, want 45 (not masked)", string(chars))
	}
}
