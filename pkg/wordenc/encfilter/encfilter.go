// Package encfilter ports the TS WordEnc + WordEncBadWords + WordEncFragments
// + WordEncDomains + WordEncTlds classes from Engine-TS/src/cache/wordenc/ to Go.
//
// One *Filter per Server; constructed via Load (reads data/raw/wordenc jagfile,
// per TS WordEnc.ts rev-244 hardcode) or LoadFromJag (reads an already-parsed
// Jagfile). After construction, *Filter is read-only and Filter.Filter is safe
// for concurrent calls.
package encfilter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// Package-level constants mirroring TS WordEnc.PERIOD / AMPERSAT / SLASH
// (Engine-TS/src/cache/wordenc/WordEnc.ts:11-28). Used by domain/tld filters
// to mask the candidate "dot" / "(a)" / "slash" representations of '.' '@' '/'.
var (
	constPeriod   = []rune("dot")
	constAmpersat = []rune("(a)")
	constSlash    = []rune("slash")
)

// whitelist mirrors TS WordEnc.whitelist (WordEnc.ts:35). Bad-words matching
// these substrings is reverted by replacing the masked chars with the
// whitelisted letters during Filter.Filter.
var whitelist = []string{"cook", "cook's", "cooks", "seeks", "sheet"}

// Filter holds the decoded wordenc sections. Read-only after construction.
type Filter struct {
	// fragments: sorted list of fragment values (TS WordEncFragments.fragments;
	// each element packs a short lowercase substring into uint16 via getInteger).
	fragments []uint16

	// bads[i] is the i-th bad word; badCombos[i] is its parallel combo list.
	// badCombos[i][k] == [a, b] where a,b are signed bytes (TS g1b).
	bads      [][]rune
	badCombos [][][2]int

	// domains[i] is the i-th domain. No parallel array.
	domains [][]rune

	// tlds[i] is the i-th TLD, tldTypes[i] is its category (1/2/3).
	tlds     [][]rune
	tldTypes []int
}

// Load reads the word-encoding Jagfile and returns a populated *Filter.
//
// Rev-244 port: TS WordEnc.load ignores its dir argument and loads from the
// hardcoded relative path "data/raw/wordenc" (TS WordEnc.ts:35-37,
// Engine-TS@9aadcec4). Missing file → error (TS Jagfile.load throws; no
// silent-return). Path is relative to the process working directory, matching
// the TS hardcode convention.
func Load() (*Filter, error) {
	// TS WordEnc.ts:35-37: load(_dir) { const wordenc = Jagfile.load('data/raw/wordenc'); … }
	return loadFromFile(filepath.Join("data", "raw", "wordenc"))
}

// loadFromFile reads jagPath and returns a populated *Filter. Used by Load and
// by tests that supply a synthetic fixture directory.
func loadFromFile(jagPath string) (*Filter, error) {
	raw, err := os.ReadFile(jagPath)
	if err != nil {
		return nil, fmt.Errorf("encfilter: read %q: %w", jagPath, err)
	}
	jf, err := jagfile.NewJagfile(packet.NewPacket(raw))
	if err != nil {
		return nil, fmt.Errorf("encfilter: parse jagfile %q: %w", jagPath, err)
	}
	return LoadFromJag(jf)
}

// LoadFromJag populates a *Filter from an already-parsed Jagfile. Mirrors TS
// WordEnc.readAll (WordEnc.ts:46-71).
func LoadFromJag(jf *jagfile.Jagfile) (*Filter, error) {
	f := &Filter{}
	if err := f.decodeBadEnc(jf); err != nil {
		return nil, fmt.Errorf("encfilter: decode badenc: %w", err)
	}
	if err := f.decodeDomainEnc(jf); err != nil {
		return nil, fmt.Errorf("encfilter: decode domainenc: %w", err)
	}
	if err := f.decodeFragmentsEnc(jf); err != nil {
		return nil, fmt.Errorf("encfilter: decode fragmentsenc: %w", err)
	}
	if err := f.decodeTldList(jf); err != nil {
		return nil, fmt.Errorf("encfilter: decode tldlist: %w", err)
	}
	return f, nil
}

// Empty returns a Filter with no rules — Filter.Filter is identity. For
// tests that don't care about censoring.
func Empty() *Filter { return &Filter{} }

// Filter censors s according to the wordenc rules. It mirrors TS
// WordEnc.filter (Engine-TS/src/cache/wordenc/WordEnc.ts:73-95).
//
// Steps:
//  1. format() — normalize allowed chars, collapse spaces.
//  2. trim + lowercase.
//  3. Run filtered (lowercased copy) through tlds → badWords → domains → fragments.
//  4. Whitelist restoration: for each whitelisted word, find its occurrences in
//     the lowercase string and write the whitelisted letters back into filtered.
//  5. replaceUppercases — restore uppercase letters from the pre-lowercase copy.
//  6. formatUppercases — canonicalize mid-run uppercase to lowercase.
//  7. trim the result.
//
// Filter is safe for concurrent calls; all state on *Filter is read-only after
// construction.
func (f *Filter) Filter(s string) string {
	// Step 1: normalize in-place.
	characters := []rune(s)
	format(characters)

	// Step 2: trim + lowercase.
	trimmed := []rune(strings.TrimSpace(string(characters)))
	lowercaseRunes := []rune(toLower(string(trimmed)))
	filtered := append([]rune(nil), lowercaseRunes...)

	// Step 3: tlds → badWords → domains → fragments (TS ordering: WordEnc.ts:78-84).
	frags := &fragments{items: f.fragments}
	bw := &badWords{bads: f.bads, combos: f.badCombos, fragments_: frags}
	dom := &domains{bads: bw, domains: f.domains}
	tl := &tlds{bads: bw, domains: dom, tlds: f.tlds, types: f.tldTypes}

	tl.filter(filtered)
	bw.filter(filtered)
	dom.filter(filtered)
	frags.filter(filtered)

	// Step 4: whitelist restoration (WordEnc.ts:85-93).
	// TS searches in `lowercase` (pre-filter copy) so masked positions don't
	// prevent restoration; then writes whitelisted chars into `filtered`.
	for _, w := range whitelist {
		wr := []rune(w)
		for off := 0; ; {
			idx := indexOfRuneSlice(lowercaseRunes, wr, off)
			if idx == -1 {
				break
			}
			copy(filtered[idx:], wr)
			off = idx + 1
		}
	}

	// Steps 5-7: restore uppercase, canonicalize, trim (WordEnc.ts:94-95).
	replaceUppercases(filtered, trimmed)
	formatUppercases(filtered)
	return strings.TrimSpace(string(filtered))
}

// toLower lowercases only ASCII A-Z characters (TS toLowerCase over format()
// output is ASCII-clean for algorithm-relevant characters).
func toLower(s string) string {
	out := []rune(s)
	for i, c := range out {
		if isUppercaseAlpha(c) {
			out[i] = c + ('a' - 'A')
		}
	}
	return string(out)
}

// indexOfRuneSlice mirrors JS String.prototype.indexOf(substr, fromIndex) over
// a []rune haystack. Returns the rune index of the first occurrence of needle
// in haystack at or after fromIndex, or -1 if not found.
func indexOfRuneSlice(haystack, needle []rune, fromIndex int) int {
	if fromIndex < 0 {
		fromIndex = 0
	}
	if len(needle) == 0 {
		return fromIndex
	}
	for i := fromIndex; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// decodeBadEnc reads badenc.txt entries. TS WordEnc.ts:198-207.
//
//	g4s count
//	per entry:
//	  g1 wordLen; g1 × wordLen → uint16 word chars
//	  g1 comboCount; (g1b, g1b) × comboCount → [a,b] signed pairs
func (f *Filter) decodeBadEnc(jf *jagfile.Jagfile) error {
	pk, err := jf.Read("badenc.txt")
	if err != nil {
		return err
	}
	count := int(pk.G4())
	f.bads = make([][]rune, count)
	f.badCombos = make([][][2]int, count)
	for i := range count {
		wordLen := int(pk.G1())
		word := make([]rune, wordLen)
		for j := range wordLen {
			word[j] = rune(pk.G1())
		}
		f.bads[i] = word

		comboCount := int(pk.G1())
		combos := make([][2]int, comboCount)
		for j := range comboCount {
			combos[j] = [2]int{int(pk.G1B()), int(pk.G1B())}
		}
		f.badCombos[i] = combos
	}
	return nil
}

// decodeDomainEnc reads domainenc.txt entries. TS WordEnc.ts:209-214.
//
//	g4s count
//	per entry: g1 len; g1 × len chars
func (f *Filter) decodeDomainEnc(jf *jagfile.Jagfile) error {
	pk, err := jf.Read("domainenc.txt")
	if err != nil {
		return err
	}
	count := int(pk.G4())
	f.domains = make([][]rune, count)
	for i := range count {
		n := int(pk.G1())
		dom := make([]rune, n)
		for j := range n {
			dom[j] = rune(pk.G1())
		}
		f.domains[i] = dom
	}
	return nil
}

// decodeFragmentsEnc reads fragmentsenc.txt entries. TS WordEnc.ts:216-221.
//
//	g4s count
//	per entry: g2 value
func (f *Filter) decodeFragmentsEnc(jf *jagfile.Jagfile) error {
	pk, err := jf.Read("fragmentsenc.txt")
	if err != nil {
		return err
	}
	count := int(pk.G4())
	f.fragments = make([]uint16, count)
	for i := range count {
		f.fragments[i] = pk.G2()
	}
	return nil
}

// decodeTldList reads tldlist.txt entries. TS WordEnc.ts:190-196.
//
//	g4s count
//	per entry: g1 type; g1 len; g1 × len chars
func (f *Filter) decodeTldList(jf *jagfile.Jagfile) error {
	pk, err := jf.Read("tldlist.txt")
	if err != nil {
		return err
	}
	count := int(pk.G4())
	f.tlds = make([][]rune, count)
	f.tldTypes = make([]int, count)
	for i := range count {
		f.tldTypes[i] = int(pk.G1())
		n := int(pk.G1())
		tld := make([]rune, n)
		for j := range n {
			tld[j] = rune(pk.G1())
		}
		f.tlds[i] = tld
	}
	return nil
}
