// Package encfilter ports the TS WordEnc + WordEncBadWords + WordEncFragments
// + WordEncDomains + WordEncTlds classes from Engine-TS/src/cache/wordenc/ to Go.
//
// One *Filter per Server; constructed via Load (reads cachePath/client/wordenc
// jagfile) or LoadFromJag (reads an already-parsed Jagfile). After
// construction, *Filter is read-only and Filter.Filter is safe for concurrent
// calls.
package encfilter

import (
	"fmt"
	"os"
	"path/filepath"

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

// Load reads <cachePath>/client/wordenc and returns a populated *Filter.
// Mirrors TS WordEnc.load (Engine-TS/src/cache/wordenc/WordEnc.ts:37-44).
func Load(cachePath string) (*Filter, error) {
	jagPath := filepath.Join(cachePath, "client", "wordenc")
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

// Filter is the T7 entry point. STUB — implemented in T7.
func (f *Filter) Filter(s string) string {
	return s
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
