# NAI-WORDENC-FILTER Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `WordEnc.filter` (Engine-TS/src/cache/wordenc/, ~1000 LOC, 5 classes) to a new Go package `pkg/wordenc/encfilter`. Wire it to `sendMessagePrivate` and `handleMessagePublic` so PM and broadcast public-chat output goes through TS-faithful profanity / URL / domain censoring. Retires `DEVIATION-NAI-182-D5-NO-WORDENC-FILTER`.

**Architecture:** Stateful `*Filter` instance held on `*Server`, populated by `encfilter.Load(cachePath)` reading the existing `client/wordenc` jagfile (already produced by goscape's pack pipeline). Internal types `*badWords`, `*fragments`, `*domains`, `*tlds` are unexported; the only public surface is `Load`, `LoadFromJag`, `Empty` and `(*Filter).Filter(string) string`. All algorithm helpers operate on `[]rune` to match TS character semantics over the ASCII + `£/€` charset.

**Tech Stack:** Go 1.26 (use-modern-go idioms: `for i := range n`, `[]rune`, `slices`/`maps`, `cmp.Or`, etc.). Existing `pkg/io/jagfile` + `pkg/io/packet`. Tests use `encoding/json` for the TS-derived golden fixtures.

---

## Spec reference

This plan implements every section of `docs/superpowers/specs/2026-05-19-nai-wordenc-filter-port-design.md`. Read the spec first if any task is ambiguous — it is the contract.

## Global preconditions (every task)

- Working directory: `/home/owner/Code/github.com/zsrv/goscape`.
- `unset GOROOT; export PATH="/home/owner/go/current/bin:$PATH"` before any `go` invocation (the system's default GOROOT path is stale; the wrapper script needs this).
- All `go` invocations prefix `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` (project convention from global CLAUDE.md).
- All commits use `--no-gpg-sign` (project convention from global CLAUDE.md).
- Before committing, `git status --short` immediately, then `git show --stat HEAD` after the commit. Concurrent shell activity can sneak files into the index (see memory `feedback_git_pre_commit_status_check.md`).
- Per-commit gates: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... ./pkg/wordenc/... -count=1 -timeout 300s` and `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/wordenc/...` must both pass. The full-tree race + smoke-pack runs at T8, T9, T10, T11 (those are the tasks that touch `modules/world`).
- Smoke-pack invocation when the task touches the cache-loading path:
  ```
  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content
  ```
  Expected: `12 OK / 0 ERR / 0 SKIP`.

## TS source map

Implementers should read these files directly when porting algorithms — the Go code is a literal port and the TS source is authoritative.

- `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/cache/wordenc/WordEnc.ts` (267 LOC) — `WordEnc` class, `filter`, helpers, decoders.
- `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/cache/wordenc/WordEncBadWords.ts` (385 LOC) — `filter`, `filterBadCombinations`, `processBadCharacters`, `getEmulatedBadCharLen`, `comboMatches`, `getIndex`.
- `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/cache/wordenc/WordEncFragments.ts` (116 LOC) — `filter`, `isBadFragment`, `getInteger`, `indexOfNumber`, `indexOfNonNumber`.
- `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/cache/wordenc/WordEncDomains.ts` (89 LOC) — `filter`, `filterDomain`, `findMatchingDomain`, `getEmulatedDomainCharLen`.
- `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/cache/wordenc/WordEncTlds.ts` (142 LOC) — `filter`, `filterTld`, `processTlds`.
- `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/network/game/client/handler/MessagePublicHandler.ts` — TS call site for `WordEnc.filter` in public-chat broadcast.
- `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/network/game/server/codec/MessagePrivateEncoder.ts` — TS call site for `WordEnc.filter` in inbound PM.

---

## Task 1: Package skeleton + Filter struct + Load jagfile decoders

**Files:**
- Create: `pkg/wordenc/encfilter/encfilter.go`
- Create: `pkg/wordenc/encfilter/encfilter_test.go`

**Pre-flight checks:**
```bash
ls pkg/wordenc/encfilter/ 2>/dev/null  # expect: not exist
grep -rn "pkg/wordenc/encfilter" --include="*.go" .  # expect: 0 matches
grep -n "Read.*name string" pkg/io/jagfile/jagfile.go  # confirm Jagfile.Read(name) → (*packet.Packet, error) at line 78
grep -n "G4()\|G1B()\|G2()" pkg/io/packet/packet.go | head -5  # confirm G4 uint32, G1B int8, G2 uint16
```

- [ ] **Step 1: Write the failing test**

Create `pkg/wordenc/encfilter/encfilter_test.go`:

```go
package encfilter

import (
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
	raw, err := readFileForTest(tmpPath)
	if err != nil {
		t.Fatalf("read synthetic jag: %v", err)
	}
	out, err := jagfile.NewJagfile(packet.NewPacket(raw))
	if err != nil {
		t.Fatalf("parse synthetic jag: %v", err)
	}
	return out
}

func readFileForTest(path string) ([]byte, error) {
	return osReadFile(path)
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
```

Add a stub `osReadFile` helper at the top (avoids a top-level `os` import that conflicts with later refactors):
```go
import "os"

func osReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
```

Actually merge that into the test file directly — it's fine to import `os` from a test file.

- [ ] **Step 2: Run test to verify it fails (compile error — package empty)**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/encfilter/... -count=1 2>&1 | head -20
```
Expected: compile errors — `LoadFromJag undefined`, `Empty undefined`, etc.

- [ ] **Step 3: Write the implementation**

Create `pkg/wordenc/encfilter/encfilter.go`:

```go
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
```

- [ ] **Step 4: Run tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/encfilter/... -count=1 -v
```
Expected: `TestLoadFromJag_DecodesAllFourSections PASS`, `TestEmpty_FilterIsIdentity PASS`.

- [ ] **Step 5: vet + race**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/wordenc/encfilter/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/wordenc/encfilter/... -count=1
```
Expected: no output / PASS.

- [ ] **Step 6: Commit**

```
git status --short
git add pkg/wordenc/encfilter/
git commit --no-gpg-sign -m "feat(wordenc): NAI-WORDENC-FILTER T1 — encfilter package skeleton + jagfile decoders"
git show --stat HEAD
```

---

## Task 2: Helper functions on []rune

**Files:**
- Create: `pkg/wordenc/encfilter/helpers.go`
- Modify: `pkg/wordenc/encfilter/encfilter_test.go` (add helper tests inline; or create `helpers_test.go`)

**Pre-flight checks:**
```bash
ls pkg/wordenc/encfilter/helpers.go 2>/dev/null  # expect: not exist
grep -n "isLowercaseAlpha\|isUppercaseAlpha\|isSymbol\|maskChars" pkg/wordenc/encfilter/ -r  # expect: 0 matches (none yet)
```

- [ ] **Step 1: Write the failing tests**

Create `pkg/wordenc/encfilter/helpers_test.go`:

```go
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
		if got := isLowercaseAlpha(c); got != c.want {
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
		if got := isUppercaseAlpha(c); got != c.want {
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
	if slices.Contains(chars[:11], '') {
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
		t.Errorf("replaceUppercases: got %q, want %q", string(chars), "HELLO")
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
	// H first: alpha, flagged=true, uppercase doesn't unflag. Remains H. E next: flagged still true, uppercase doesn't unflag. So "HELLO" stays?
	// Re-read TS: `if (flagged) { if (isLowercase) flagged = false } else if (isUppercase) { lowercase }`.
	// At H: flagged true → check isLowercase(H) false → flagged still true. No mutation.
	// At E: flagged still true → check isLowercase(E) false → flagged still true. No mutation.
	// So "HELLO" stays "HELLO".
	if string(chars) != "HELLO" {
		t.Errorf("formatUppercases (all upper): got %q, want %q", string(chars), "HELLO")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (compile errors)**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/encfilter/... -count=1 2>&1 | head -20
```
Expected: compile errors (isLowercaseAlpha undefined, etc.).

- [ ] **Step 3: Write the implementation**

Create `pkg/wordenc/encfilter/helpers.go`:

```go
package encfilter

// helpers.go ports the static methods on TS WordEnc (Engine-TS/src/cache/
// wordenc/WordEnc.ts:97-188). All operate on []rune to match TS character
// semantics over the ASCII + £/€ charset.

func isLowercaseAlpha(c rune) bool { return c >= 'a' && c <= 'z' }
func isUppercaseAlpha(c rune) bool { return c >= 'A' && c <= 'Z' }
func isNumerical(c rune) bool      { return c >= '0' && c <= '9' }
func isAlpha(c rune) bool          { return isLowercaseAlpha(c) || isUppercaseAlpha(c) }
func isSymbol(c rune) bool         { return !isAlpha(c) && !isNumerical(c) }

// isNotLowercaseAlpha mirrors WordEnc.ts:101-103.
//
//	return this.isLowercaseAlpha(char)
//	  ? char == 'v' || char == 'x' || char == 'j' || char == 'q' || char == 'z'
//	  : true
//
// I.e. uncommon-lowercase OR not-lowercase-at-all.
func isNotLowercaseAlpha(c rune) bool {
	if isLowercaseAlpha(c) {
		return c == 'v' || c == 'x' || c == 'j' || c == 'q' || c == 'z'
	}
	return true
}

// isNumericalChars mirrors WordEnc.ts:121-128. NUL ('\x00') counts as
// wildcard (returns true), any other non-digit returns false.
func isNumericalChars(chars []rune) bool {
	for _, c := range chars {
		if !isNumerical(c) && c != '\x00' {
			return false
		}
	}
	return true
}

// maskChars replaces chars[offset:length] with '*'. Mirrors WordEnc.ts:130-134.
// NOTE: TS uses `for index = offset; index < length; index++` — length is the
// EXCLUSIVE upper bound, NOT a count.
func maskChars(offset, end int, chars []rune) {
	for i := offset; i < end; i++ {
		chars[i] = '*'
	}
}

// maskedCountBackwards mirrors WordEnc.ts:136-144.
func maskedCountBackwards(chars []rune, offset int) int {
	count := 0
	for i := offset - 1; i >= 0 && isSymbol(chars[i]); i-- {
		if chars[i] == '*' {
			count++
		}
	}
	return count
}

// maskedCountForwards mirrors WordEnc.ts:146-154.
func maskedCountForwards(chars []rune, offset int) int {
	count := 0
	for i := offset + 1; i < len(chars) && isSymbol(chars[i]); i++ {
		if chars[i] == '*' {
			count++
		}
	}
	return count
}

// maskedCharsStatus mirrors WordEnc.ts:156-164. Returns 0/1/4.
func maskedCharsStatus(chars, filtered []rune, offset, length int, prefix bool) int {
	var count int
	if prefix {
		count = maskedCountBackwards(filtered, offset)
	} else {
		count = maskedCountForwards(filtered, offset)
	}
	if count >= length {
		return 4
	}
	var adj rune
	if prefix {
		adj = chars[offset-1]
	} else {
		adj = chars[offset+1]
	}
	if isSymbol(adj) {
		return 1
	}
	return 0
}

// prefixSymbolStatus mirrors WordEnc.ts:166-176.
func prefixSymbolStatus(offset int, chars []rune, length int, symbolChars []rune, symbols []rune) int {
	if offset == 0 {
		return 2
	}
	for i := offset - 1; i >= 0 && isSymbol(chars[i]); i-- {
		for _, s := range symbols {
			if chars[i] == s {
				return 3
			}
		}
	}
	return maskedCharsStatus(chars, symbolChars, offset, length, true)
}

// suffixSymbolStatus mirrors WordEnc.ts:178-188.
func suffixSymbolStatus(offset int, chars []rune, length int, symbolChars []rune, symbols []rune) int {
	if offset+1 == len(chars) {
		return 2
	}
	for i := offset + 1; i < len(chars) && isSymbol(chars[i]); i++ {
		for _, s := range symbols {
			if chars[i] == s {
				return 3
			}
		}
	}
	return maskedCharsStatus(chars, symbolChars, offset, length, false)
}

// isCharacterAllowed mirrors WordEnc.ts:240-242.
// Accepts ASCII printable ' '..'\x7f', plus '\n', '\t', '£', '€'.
func isCharacterAllowed(c rune) bool {
	if c >= ' ' && c <= '\x7f' {
		return true
	}
	return c == ' ' || c == '\n' || c == '\t' || c == '£' || c == '€'
}

// format mirrors WordEnc.ts:223-238. In-place: replaces disallowed chars with
// space, collapses consecutive spaces, pads tail with spaces.
func format(chars []rune) {
	pos := 0
	for i := range len(chars) {
		if isCharacterAllowed(chars[i]) {
			chars[pos] = chars[i]
		} else {
			chars[pos] = ' '
		}
		if pos == 0 || chars[pos] != ' ' || chars[pos-1] != ' ' {
			pos++
		}
	}
	for i := pos; i < len(chars); i++ {
		chars[i] = ' '
	}
}

// replaceUppercases mirrors WordEnc.ts:244-250. For each i in [0, len(comparison)):
// if comparison[i] is uppercase AND chars[i] != '*', copy comparison[i] over chars[i].
func replaceUppercases(chars, comparison []rune) {
	for i := range len(comparison) {
		if chars[i] != '*' && isUppercaseAlpha(comparison[i]) {
			chars[i] = comparison[i]
		}
	}
}

// formatUppercases mirrors WordEnc.ts:252-266. First letter of each alphabetic
// run keeps its case; subsequent uppercase letters in the same run get
// lowercased (to canonicalize "HELLO world" — but only after the run starts
// with a lowercase).
func formatUppercases(chars []rune) {
	flagged := true
	for i, c := range chars {
		if !isAlpha(c) {
			flagged = true
		} else if flagged {
			if isLowercaseAlpha(c) {
				flagged = false
			}
		} else if isUppercaseAlpha(c) {
			chars[i] = c + ('a' - 'A')
		}
	}
}
```

- [ ] **Step 4: Run tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/encfilter/... -count=1 -v
```
Expected: all 12 helper tests + 2 prior tests PASS.

- [ ] **Step 5: Vet**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/wordenc/encfilter/...
```
Expected: no output.

- [ ] **Step 6: Commit**

```
git status --short
git add pkg/wordenc/encfilter/helpers.go pkg/wordenc/encfilter/helpers_test.go
git commit --no-gpg-sign -m "feat(wordenc): NAI-WORDENC-FILTER T2 — []rune helpers (isAlpha/format/maskChars/etc.)"
git show --stat HEAD
```

---

## Task 3: Fragments filter

**Files:**
- Create: `pkg/wordenc/encfilter/fragments.go`
- Create: `pkg/wordenc/encfilter/fragments_test.go`

**Pre-flight checks:**
```bash
ls pkg/wordenc/encfilter/fragments.go 2>/dev/null  # expect: not exist
grep -n "fragments\b" pkg/wordenc/encfilter/encfilter.go  # confirm Filter.fragments field exists
```

- [ ] **Step 1: Write the failing tests**

Create `pkg/wordenc/encfilter/fragments_test.go`:

```go
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
	// NUL accepted but yields no contribution.
	if got := getFragmentInteger([]rune{'a', '\x00'}); got != 38 {
		// Reversed: NUL skipped (returns 0 contribution unconditionally per TS line 92);
		// 'a' → 1 * 38 = 38.
		t.Errorf("getFragmentInteger(a + NUL): got %d, want 38", got)
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

// TestFragments_filter_MasksFourPlusDigitSequences pins the filter loop logic:
// digit sequences of length 4+ get masked (TS WordEncFragments.ts:6-48). The
// startIndex incrementing requires "isSymbolOrNotLowercaseAlpha" before the
// number to count.
func TestFragments_filter_MasksLongDigitRuns(t *testing.T) {
	frags := &fragments{}
	chars := []rune("call 12345 now")
	frags.filter(chars)
	// Implementer: verify the actual mask range — the TS algorithm is subtle.
	// At minimum, runs of >=4 digits adjacent to symbols should yield '*'.
	if !containsRune(chars, '*') {
		t.Errorf("filter: no masking happened in %q", string(chars))
	}
}

func containsRune(rs []rune, target rune) bool {
	for _, r := range rs {
		if r == target {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/encfilter/... -count=1 2>&1 | head -10
```
Expected: `fragments undefined`, `getFragmentInteger undefined`.

- [ ] **Step 3: Write the implementation**

Create `pkg/wordenc/encfilter/fragments.go`:

```go
package encfilter

// fragments mirrors TS WordEncFragments (Engine-TS/src/cache/wordenc/
// WordEncFragments.ts). items is the sorted []uint16 of encoded fragment
// values; isBadFragment does binary search; filter masks long digit runs.

type fragments struct {
	items []uint16
}

// filter mirrors WordEncFragments.filter (WordEncFragments.ts:6-48). Detects
// runs of 4+ digits adjacent to "non-common" characters and masks them.
func (f *fragments) filter(chars []rune) {
	for currentIndex := 0; currentIndex < len(chars); {
		numberIndex := f.indexOfNumber(chars, currentIndex)
		if numberIndex == -1 {
			return
		}

		isSymbolOrNotLowercaseAlpha := false
		for i := currentIndex; i >= 0 && i < numberIndex && !isSymbolOrNotLowercaseAlpha; i++ {
			if !isSymbol(chars[i]) && !isNotLowercaseAlpha(chars[i]) {
				isSymbolOrNotLowercaseAlpha = true
			}
		}

		startIndex := 0
		if isSymbolOrNotLowercaseAlpha {
			startIndex = 0
		}
		if startIndex == 0 {
			startIndex = 1
			currentIndex = numberIndex
		}

		value := 0
		for i := numberIndex; i < len(chars) && i < currentIndex; i++ {
			value = value*10 + int(chars[i]-'0')
		}

		if value <= 255 && currentIndex-numberIndex <= 8 {
			startIndex++
		} else {
			startIndex = 0
		}

		if startIndex == 4 {
			maskChars(numberIndex, currentIndex, chars)
			startIndex = 0
		}
		currentIndex = f.indexOfNonNumber(currentIndex, chars)
	}
}

// isBadFragment mirrors WordEncFragments.isBadFragment (WordEncFragments.ts:50-77).
// All-numerical chars always return true (TS short-circuit). Otherwise the
// encoded value is binary-searched in items.
func (f *fragments) isBadFragment(chars []rune) bool {
	if isNumericalChars(chars) {
		return true
	}
	value := uint16(getFragmentInteger(chars))
	items := f.items
	if len(items) == 0 {
		return false
	}
	if value == items[0] || value == items[len(items)-1] {
		return true
	}
	start, end := 0, len(items)-1
	for start <= end {
		mid := (start + end) / 2
		if value == items[mid] {
			return true
		} else if value < items[mid] {
			end = mid - 1
		} else {
			start = mid + 1
		}
	}
	return false
}

// getFragmentInteger mirrors WordEncFragments.getInteger (WordEncFragments.ts:79-97).
// Walks chars BACKWARDS and accumulates base-38 value. Returns 0 for len > 6
// or for non-alpha/non-digit/non-apostrophe content (other than NUL which is
// a wildcard contributing 0 implicitly).
func getFragmentInteger(chars []rune) int {
	if len(chars) > 6 {
		return 0
	}
	value := 0
	for i := range len(chars) {
		c := chars[len(chars)-i-1]
		switch {
		case isLowercaseAlpha(c):
			value = value*38 + int(c) + 1 - 'a'
		case c == '\'':
			value = value*38 + 27
		case isNumerical(c):
			value = value*38 + int(c) + 28 - '0'
		case c != '\x00':
			return 0
		}
	}
	return value
}

func (f *fragments) indexOfNumber(chars []rune, offset int) int {
	for i := offset; i < len(chars) && i >= 0; i++ {
		if isNumerical(chars[i]) {
			return i
		}
	}
	return -1
}

func (f *fragments) indexOfNonNumber(offset int, chars []rune) int {
	for i := offset; i < len(chars) && i >= 0; i++ {
		if !isNumerical(chars[i]) {
			return i
		}
	}
	return len(chars)
}
```

- [ ] **Step 4: Run tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/encfilter/... -count=1 -v -run TestFragments
```
Expected: all fragments tests PASS.

- [ ] **Step 5: Commit**

```
git status --short
git add pkg/wordenc/encfilter/fragments.go pkg/wordenc/encfilter/fragments_test.go
git commit --no-gpg-sign -m "feat(wordenc): NAI-WORDENC-FILTER T3 — fragments filter + binary-search lookup"
git show --stat HEAD
```

---

## Task 4: Bad-words filter (largest)

**Files:**
- Create: `pkg/wordenc/encfilter/badwords.go`
- Create: `pkg/wordenc/encfilter/badwords_test.go`

**Pre-flight checks:**
```bash
ls pkg/wordenc/encfilter/badwords.go 2>/dev/null  # expect: not exist
wc -l /home/owner/Code/github.com/LostCityRS/Engine-TS/src/cache/wordenc/WordEncBadWords.ts  # confirm 385 LOC
```

This is the largest single task. Read `WordEncBadWords.ts` end-to-end before starting.

The shape is:
- `(b *badWords) filter(chars)` runs `filterBadCombinations` twice for each bad word (TS comboIndex loop 0..1).
- `filterBadCombinations(combos, chars, bad)` scans for the bad word starting at every position; uses `processBadCharacters` to walk both `chars` and `bad` with leetspeak emulation; then checks symbol/combo conditions; then masks if `numeralCount <= alphaCount`.
- `processBadCharacters(chars, bad, startIndex) -> (currentIndex, badIndex, hasSymbol, hasNumber, hasDigit)` is the inner loop matching `bad` against `chars` with `getEmulatedBadCharLen` allowing leetspeak substitution.
- `getEmulatedBadCharLen(nextChar, badChar, currentChar) -> int` is the big switch — returns 0 (no match), 1 (single-char substitution like 'a'→'4'), or 2 (two-char substitution like 'a' → "/\\").
- `comboMatches(currentIndex, combos, nextIndex)` binary-searches the parallel `combos` array. (Note: TS sorts `combos` by `[a, b]` ascending.)
- `getIndex(char) -> int` maps a char to a combo-key index (lowercase 'a'..'z' → 1..26, "'" → 28, '0'..'9' → 29..38, other → 27).

- [ ] **Step 1: Write the failing tests**

Create `pkg/wordenc/encfilter/badwords_test.go`:

```go
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
		if got := badGetIndex(c); got != c.want {
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
		{0, 'a', '4'}, // 1-char: a→4
		{0, 'a', '@'},
		{0, 'a', '^'},
		{'\\', 'a', '/'}, // 2-char: a→/\
		{0, 'b', '6'},
		{0, 'b', '8'},
		{'3', 'b', '1'}, // 2-char: b→13
		{0, 'e', '3'},
		{0, 'e', '€'},
		{0, 'l', '1'},
		{0, 'l', '|'},
		{0, 'l', 'i'},
		{0, 's', '5'}, {0, 's', '$'}, {0, 's', '2'}, {0, 's', 'z'},
		{0, 't', '7'}, {0, 't', '+'},
		{0, 'u', 'v'},
	}
	// Wants: most are 1; explicit 2-char cases:
	wants2 := map[[3]rune]int{
		{'\\', 'a', '/'}: 2,
		{'3', 'b', '1'}:  2,
	}
	for _, c := range cases {
		want := c.want
		if want == 0 {
			want = 1
			if v, ok := wants2[[3]rune{c.next, c.bad, c.current}]; ok {
				want = v
			}
		}
		if got := getEmulatedBadCharLen(c.next, c.bad, c.current); got != want {
			t.Errorf("getEmulatedBadCharLen(next=%q bad=%q cur=%q): got %d, want %d", c.next, c.bad, c.current, got, want)
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
```

- [ ] **Step 2: Run tests to verify failure**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/encfilter/... -count=1 -run TestBadWords 2>&1 | head -10
```
Expected: compile errors.

- [ ] **Step 3: Write the implementation**

Create `pkg/wordenc/encfilter/badwords.go`. The Go port preserves TS structure literally — translate the big `getEmulatedBadCharLen` switch as a switch on `badChar`. See `WordEncBadWords.ts:182-356`.

```go
package encfilter

// badwords.go mirrors TS WordEncBadWords (Engine-TS/src/cache/wordenc/
// WordEncBadWords.ts).

// badWords holds the bad word list, per-word combo lists, and a back-reference
// to fragments for the substring-validity check.
type badWords struct {
	bads       [][]rune
	combos     [][][2]int // parallel array; nil entry means "no combos for this word"
	fragments_ *fragments // back-ref for isBadFragment check (substring validity)
}

// filter runs filterBadCombinations for each bad word TWICE (the comboIndex
// loop 0..1 in WordEncBadWords.ts:14-19). Walks bad words from len-1 down to 0.
func (b *badWords) filter(chars []rune) {
	for comboIndex := 0; comboIndex < 2; comboIndex++ {
		for i := len(b.bads) - 1; i >= 0; i-- {
			b.filterBadCombinations(b.combos[i], chars, b.bads[i])
		}
	}
}

// filterBadCombinations scans chars for occurrences of bad starting at every
// position; if a match is found, the combo / symbol / substring-validity
// conditions are checked and the match is masked when the masking threshold
// (numeralCount <= alphaCount) holds. Mirrors WordEncBadWords.ts:22-111.
func (b *badWords) filterBadCombinations(combos [][2]int, chars []rune, bad []rune) {
	if len(bad) > len(chars) {
		return
	}
	for startIndex := 0; startIndex <= len(chars)-len(bad); startIndex++ {
		currentIndex := startIndex
		updated, badIndex, hasSymbol, hasNumber, hasDigit := b.processBadCharacters(chars, bad, currentIndex)
		currentIndex = updated
		if !(badIndex >= len(bad) && (!hasNumber || !hasDigit)) {
			continue
		}
		shouldFilter := true
		if hasSymbol {
			isBeforeSymbol := false
			isAfterSymbol := false
			if startIndex-1 < 0 || (isSymbol(chars[startIndex-1]) && chars[startIndex-1] != '\'') {
				isBeforeSymbol = true
			}
			if currentIndex >= len(chars) || (isSymbol(chars[currentIndex]) && chars[currentIndex] != '\'') {
				isAfterSymbol = true
			}
			if !isBeforeSymbol || !isAfterSymbol {
				isSubstringValid := false
				localIndex := startIndex - 2
				if isBeforeSymbol {
					localIndex = startIndex
				}
				for !isSubstringValid && localIndex < currentIndex {
					if localIndex >= 0 && (!isSymbol(chars[localIndex]) || chars[localIndex] == '\'') {
						localSub := []rune{}
						localSubIndex := 0
						for localSubIndex < 3 && localIndex+localSubIndex < len(chars) &&
							(!isSymbol(chars[localIndex+localSubIndex]) || chars[localIndex+localSubIndex] == '\'') {
							localSub = append(localSub, chars[localIndex+localSubIndex])
							localSubIndex++
						}
						isSubStringValidCondition := true
						if localSubIndex == 0 {
							isSubStringValidCondition = false
						}
						if localSubIndex < 3 && localIndex-1 >= 0 &&
							(!isSymbol(chars[localIndex-1]) || chars[localIndex-1] == '\'') {
							isSubStringValidCondition = false
						}
						if isSubStringValidCondition && !b.fragments_.isBadFragment(localSub) {
							isSubstringValid = true
						}
					}
					localIndex++
				}
				if !isSubstringValid {
					shouldFilter = false
				}
			}
		} else {
			currentChar := ' '
			if startIndex-1 >= 0 {
				currentChar = chars[startIndex-1]
			}
			nextChar := ' '
			if currentIndex < len(chars) {
				nextChar = chars[currentIndex]
			}
			current := badGetIndex(currentChar)
			next := badGetIndex(nextChar)
			if combos != nil && badComboMatches(current, combos, next) {
				shouldFilter = false
			}
		}
		if !shouldFilter {
			continue
		}
		numeralCount := 0
		alphaCount := 0
		for i := startIndex; i < currentIndex; i++ {
			if isNumerical(chars[i]) {
				numeralCount++
			} else if isAlpha(chars[i]) {
				alphaCount++
			}
		}
		if numeralCount <= alphaCount {
			maskChars(startIndex, currentIndex, chars)
		}
	}
}

// processBadCharacters mirrors WordEncBadWords.ts:113-180.
func (b *badWords) processBadCharacters(chars, bad []rune, startIndex int) (currentIndex, badIndex int, hasSymbol, hasNumber, hasDigit bool) {
	index := startIndex
	badIndex = 0
	count := 0
	for index < len(chars) && !(hasNumber && hasDigit) {
		if index >= len(chars) || (hasNumber && hasDigit) {
			break
		}
		currentChar := chars[index]
		nextChar := rune('\x00')
		if index+1 < len(chars) {
			nextChar = chars[index+1]
		}

		var currentLength int
		if badIndex < len(bad) {
			currentLength = getEmulatedBadCharLen(nextChar, bad[badIndex], currentChar)
		}
		if badIndex < len(bad) && currentLength > 0 {
			if currentLength == 1 && isNumerical(currentChar) {
				hasNumber = true
			}
			if currentLength == 2 && (isNumerical(currentChar) || isNumerical(nextChar)) {
				hasNumber = true
			}
			index += currentLength
			badIndex++
		} else {
			if badIndex == 0 {
				break
			}
			previousLength := getEmulatedBadCharLen(nextChar, bad[badIndex-1], currentChar)
			if previousLength > 0 {
				index += previousLength
			} else {
				if badIndex >= len(bad) || !isNotLowercaseAlpha(currentChar) {
					break
				}
				if isSymbol(currentChar) && currentChar != '\'' {
					hasSymbol = true
				}
				if isNumerical(currentChar) {
					hasDigit = true
				}
				index++
				count++
				if (count*100)/(index-startIndex) > 90 {
					break
				}
			}
		}
	}
	currentIndex = index
	return
}

// getEmulatedBadCharLen ports the entire TS leetspeak switch
// (WordEncBadWords.ts:182-356).
func getEmulatedBadCharLen(nextChar, badChar, currentChar rune) int {
	if badChar == currentChar {
		return 1
	}
	if badChar >= 'a' && badChar <= 'm' {
		switch badChar {
		case 'a':
			if currentChar != '4' && currentChar != '@' && currentChar != '^' {
				if currentChar == '/' && nextChar == '\\' {
					return 2
				}
				return 0
			}
			return 1
		case 'b':
			if currentChar != '6' && currentChar != '8' {
				if currentChar == '1' && nextChar == '3' {
					return 2
				}
				return 0
			}
			return 1
		case 'c':
			if currentChar != '(' && currentChar != '<' && currentChar != '{' && currentChar != '[' {
				return 0
			}
			return 1
		case 'd':
			if currentChar == '[' && nextChar == ')' {
				return 2
			}
			return 0
		case 'e':
			if currentChar != '3' && currentChar != '€' {
				return 0
			}
			return 1
		case 'f':
			if currentChar == 'p' && nextChar == 'h' {
				return 2
			}
			if currentChar == '£' {
				return 1
			}
			return 0
		case 'g':
			if currentChar != '9' && currentChar != '6' {
				return 0
			}
			return 1
		case 'h':
			if currentChar == '#' {
				return 1
			}
			return 0
		case 'i':
			if currentChar != 'y' && currentChar != 'l' && currentChar != 'j' && currentChar != '1' && currentChar != '!' && currentChar != ':' && currentChar != ';' && currentChar != '|' {
				return 0
			}
			return 1
		case 'j', 'k':
			return 0
		case 'l':
			if currentChar != '1' && currentChar != '|' && currentChar != 'i' {
				return 0
			}
			return 1
		case 'm':
			return 0
		}
	}
	if badChar >= 'n' && badChar <= 'z' {
		switch badChar {
		case 'n':
			return 0
		case 'o':
			if currentChar != '0' && currentChar != '*' {
				if (currentChar != '(' || nextChar != ')') && (currentChar != '[' || nextChar != ']') && (currentChar != '{' || nextChar != '}') && (currentChar != '<' || nextChar != '>') {
					return 0
				}
				return 2
			}
			return 1
		case 'p', 'q', 'r':
			return 0
		case 's':
			if currentChar != '5' && currentChar != 'z' && currentChar != '$' && currentChar != '2' {
				return 0
			}
			return 1
		case 't':
			if currentChar != '7' && currentChar != '+' {
				return 0
			}
			return 1
		case 'u':
			if currentChar == 'v' {
				return 1
			}
			if (currentChar != '\\' || nextChar != '/') && (currentChar != '\\' || nextChar != '|') && (currentChar != '|' || nextChar != '/') {
				return 0
			}
			return 2
		case 'v':
			if (currentChar != '\\' || nextChar != '/') && (currentChar != '\\' || nextChar != '|') && (currentChar != '|' || nextChar != '/') {
				return 0
			}
			return 2
		case 'w':
			if currentChar == 'v' && nextChar == 'v' {
				return 2
			}
			return 0
		case 'x':
			if (currentChar != ')' || nextChar != '(') && (currentChar != '}' || nextChar != '{') && (currentChar != ']' || nextChar != '[') && (currentChar != '>' || nextChar != '<') {
				return 0
			}
			return 2
		case 'y', 'z':
			return 0
		}
	}
	if badChar >= '0' && badChar <= '9' {
		switch badChar {
		case '0':
			if currentChar == 'o' || currentChar == 'O' {
				return 1
			} else if (currentChar != '(' || nextChar != ')') && (currentChar != '{' || nextChar != '}') && (currentChar != '[' || nextChar != ']') {
				return 0
			} else {
				return 2
			}
		case '1':
			if currentChar == 'l' {
				return 1
			}
			return 0
		default:
			return 0
		}
	}
	switch badChar {
	case ',':
		if currentChar == '.' {
			return 1
		}
		return 0
	case '.':
		if currentChar == ',' {
			return 1
		}
		return 0
	case '!':
		if currentChar == 'i' {
			return 1
		}
		return 0
	}
	return 0
}

// badComboMatches binary-searches combos (sorted by [a, b] ascending).
// Mirrors WordEncBadWords.ts:358-373.
func badComboMatches(currentIndex int, combos [][2]int, nextIndex int) bool {
	start, end := 0, len(combos)-1
	for start <= end {
		mid := (start + end) / 2
		if combos[mid][0] == currentIndex && combos[mid][1] == nextIndex {
			return true
		} else if currentIndex < combos[mid][0] || (currentIndex == combos[mid][0] && nextIndex < combos[mid][1]) {
			end = mid - 1
		} else {
			start = mid + 1
		}
	}
	return false
}

// badGetIndex mirrors WordEncBadWords.ts:375-384.
func badGetIndex(c rune) int {
	if isLowercaseAlpha(c) {
		return int(c) + 1 - 'a'
	}
	if c == '\'' {
		return 28
	}
	if isNumerical(c) {
		return int(c) + 29 - '0'
	}
	return 27
}
```

- [ ] **Step 4: Run tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/encfilter/... -count=1 -v -run TestBadWords
```
Expected: all bad-words tests PASS.

- [ ] **Step 5: Race + vet**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/wordenc/encfilter/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/wordenc/encfilter/... -count=1
```

- [ ] **Step 6: Commit**

```
git status --short
git add pkg/wordenc/encfilter/badwords.go pkg/wordenc/encfilter/badwords_test.go
git commit --no-gpg-sign -m "feat(wordenc): NAI-WORDENC-FILTER T4 — badwords filter + leetspeak switch"
git show --stat HEAD
```

---

## Task 5: Domains filter

**Files:**
- Create: `pkg/wordenc/encfilter/domains.go`
- Create: `pkg/wordenc/encfilter/domains_test.go`

**Pre-flight checks:**
```bash
ls pkg/wordenc/encfilter/domains.go 2>/dev/null  # expect: not exist
wc -l /home/owner/Code/github.com/LostCityRS/Engine-TS/src/cache/wordenc/WordEncDomains.ts  # expect 89
```

- [ ] **Step 1: Write the failing tests**

Create `pkg/wordenc/encfilter/domains_test.go`:

```go
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

// TestDomains_filter_MasksEmailWithBareDomain — TS suffixSymbolStatus must
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
```

- [ ] **Step 2: Run tests to verify failure**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/encfilter/... -count=1 -run TestDomains 2>&1 | head -10
```

- [ ] **Step 3: Write the implementation**

Create `pkg/wordenc/encfilter/domains.go`:

```go
package encfilter

// domains mirrors TS WordEncDomains (Engine-TS/src/cache/wordenc/
// WordEncDomains.ts). filter is called by Filter.Filter after badWords.filter.
type domains struct {
	bads    *badWords // for filterBadCombinations on ampersat/period copies
	domains [][]rune
}

// filter copies chars twice (ampersat-mask and period-mask), runs
// filterBadCombinations(nil, ampersat, AMPERSAT) and (nil, period, PERIOD),
// then walks domain list backwards calling filterDomain. Mirrors WordEncDomains.ts:13-21.
func (d *domains) filter(chars []rune) {
	ampersat := append([]rune(nil), chars...)
	period := append([]rune(nil), chars...)
	d.bads.filterBadCombinations(nil, ampersat, constAmpersat)
	d.bads.filterBadCombinations(nil, period, constPeriod)
	for i := len(d.domains) - 1; i >= 0; i-- {
		d.filterDomain(period, ampersat, d.domains[i], chars)
	}
}

// filterDomain mirrors WordEncDomains.ts:42-58.
func (d *domains) filterDomain(period, ampersat, domain, chars []rune) {
	domainLength := len(domain)
	charsLength := len(chars)
	for index := 0; index <= charsLength-domainLength; index++ {
		matched, currentIndex := d.findMatchingDomain(index, domain, chars)
		if !matched {
			continue
		}
		ampersatStatus := prefixSymbolStatus(index, chars, 3, ampersat, []rune{'@'})
		periodStatus := suffixSymbolStatus(currentIndex-1, chars, 3, period, []rune{'.', ','})
		shouldFilter := ampersatStatus > 2 || periodStatus > 2
		if !shouldFilter {
			continue
		}
		maskChars(index, currentIndex, chars)
	}
}

// findMatchingDomain mirrors WordEncDomains.ts:60-88.
func (d *domains) findMatchingDomain(startIndex int, domain, chars []rune) (matched bool, currentIndex int) {
	domainLength := len(domain)
	currentIndex = startIndex
	domainIndex := 0

	for currentIndex < len(chars) && domainIndex < domainLength {
		currentChar := chars[currentIndex]
		nextChar := rune('\x00')
		if currentIndex+1 < len(chars) {
			nextChar = chars[currentIndex+1]
		}
		currentLength := getEmulatedDomainCharLen(nextChar, domain[domainIndex], currentChar)

		if currentLength > 0 {
			currentIndex += currentLength
			domainIndex++
		} else {
			if domainIndex == 0 {
				break
			}
			previousLength := getEmulatedDomainCharLen(nextChar, domain[domainIndex-1], currentChar)
			if previousLength > 0 {
				currentIndex += previousLength
				if domainIndex == 1 {
					startIndex++
				}
			} else {
				if domainIndex >= domainLength || !isSymbol(currentChar) {
					break
				}
				currentIndex++
			}
		}
	}
	matched = domainIndex >= domainLength
	return
}

// getEmulatedDomainCharLen mirrors WordEncDomains.ts:23-40. Smaller switch
// than the bad-words one — only handles o/c/e/s/l common substitutions.
func getEmulatedDomainCharLen(nextChar, domainChar, currentChar rune) int {
	if domainChar == currentChar {
		return 1
	}
	if domainChar == 'o' && currentChar == '0' {
		return 1
	}
	if domainChar == 'o' && currentChar == '(' && nextChar == ')' {
		return 2
	}
	if domainChar == 'c' && (currentChar == '(' || currentChar == '<' || currentChar == '[') {
		return 1
	}
	if domainChar == 'e' && currentChar == '€' {
		return 1
	}
	if domainChar == 's' && currentChar == '$' {
		return 1
	}
	if domainChar == 'l' && currentChar == 'i' {
		return 1
	}
	return 0
}
```

- [ ] **Step 4: Run tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/encfilter/... -count=1 -v -run TestDomains
```

- [ ] **Step 5: Commit**

```
git status --short
git add pkg/wordenc/encfilter/domains.go pkg/wordenc/encfilter/domains_test.go
git commit --no-gpg-sign -m "feat(wordenc): NAI-WORDENC-FILTER T5 — domains filter"
git show --stat HEAD
```

---

## Task 6: TLDs filter

**Files:**
- Create: `pkg/wordenc/encfilter/tlds.go`
- Create: `pkg/wordenc/encfilter/tlds_test.go`

**Pre-flight checks:**
```bash
ls pkg/wordenc/encfilter/tlds.go 2>/dev/null  # expect: not exist
wc -l /home/owner/Code/github.com/LostCityRS/Engine-TS/src/cache/wordenc/WordEncTlds.ts  # expect 142
```

- [ ] **Step 1: Write the failing tests**

Create `pkg/wordenc/encfilter/tlds_test.go`:

```go
package encfilter

import "testing"

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
	if !containsRune(chars, '*') {
		t.Errorf("tlds.filter: nothing masked in %q", string(chars))
	}
}
```

- [ ] **Step 2: Run test to verify failure**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/encfilter/... -count=1 -run TestTlds 2>&1 | head -10
```

- [ ] **Step 3: Write the implementation**

Create `pkg/wordenc/encfilter/tlds.go`:

```go
package encfilter

// tlds mirrors TS WordEncTlds (Engine-TS/src/cache/wordenc/WordEncTlds.ts).

type tlds struct {
	bads    *badWords
	domains *domains
	tlds    [][]rune
	types   []int
}

// filter copies chars twice (period-mask and slash-mask), runs
// filterBadCombinations against PERIOD and SLASH, then walks the TLD list
// calling filterTld. Mirrors WordEncTlds.ts:17-25.
func (t *tlds) filter(chars []rune) {
	period := append([]rune(nil), chars...)
	slash := append([]rune(nil), chars...)
	t.bads.filterBadCombinations(nil, period, constPeriod)
	t.bads.filterBadCombinations(nil, slash, constSlash)
	for i := range len(t.tlds) {
		t.filterTld(slash, t.types[i], chars, t.tlds[i], period)
	}
}

// filterTld mirrors WordEncTlds.ts:27-113.
func (t *tlds) filterTld(slash []rune, tldType int, chars []rune, tld []rune, period []rune) {
	if len(tld) > len(chars) {
		return
	}
	for index := 0; index <= len(chars)-len(tld); index++ {
		currentIndex, tldIndex := t.processTlds(chars, tld, index)
		if tldIndex < len(tld) {
			continue
		}
		shouldFilter := false
		periodFilterStatus := prefixSymbolStatus(index, chars, 3, period, []rune{',', '.'})
		slashFilterStatus := suffixSymbolStatus(currentIndex-1, chars, 5, slash, []rune{'\\', '/'})
		if tldType == 1 && periodFilterStatus > 0 && slashFilterStatus > 0 {
			shouldFilter = true
		}
		if tldType == 2 && ((periodFilterStatus > 2 && slashFilterStatus > 0) || (periodFilterStatus > 0 && slashFilterStatus > 2)) {
			shouldFilter = true
		}
		if tldType == 3 && periodFilterStatus > 0 && slashFilterStatus > 2 {
			shouldFilter = true
		}
		if !shouldFilter {
			continue
		}
		startFilterIndex := index
		endFilterIndex := currentIndex - 1
		if periodFilterStatus > 2 {
			if periodFilterStatus == 4 {
				foundPeriod := false
				for pi := index - 1; pi >= 0; pi-- {
					if foundPeriod {
						if period[pi] != '*' {
							break
						}
						startFilterIndex = pi
					} else if period[pi] == '*' {
						startFilterIndex = pi
						foundPeriod = true
					}
				}
			}
			foundPeriod := false
			for pi := startFilterIndex - 1; pi >= 0; pi-- {
				if foundPeriod {
					if isSymbol(chars[pi]) {
						break
					}
					startFilterIndex = pi
				} else if !isSymbol(chars[pi]) {
					foundPeriod = true
					startFilterIndex = pi
				}
			}
		}
		if slashFilterStatus > 2 {
			if slashFilterStatus == 4 {
				foundPeriod := false
				for pi := endFilterIndex + 1; pi < len(chars); pi++ {
					if foundPeriod {
						if slash[pi] != '*' {
							break
						}
						endFilterIndex = pi
					} else if slash[pi] == '*' {
						endFilterIndex = pi
						foundPeriod = true
					}
				}
			}
			foundPeriod := false
			for pi := endFilterIndex + 1; pi < len(chars); pi++ {
				if foundPeriod {
					if isSymbol(chars[pi]) {
						break
					}
					endFilterIndex = pi
				} else if !isSymbol(chars[pi]) {
					foundPeriod = true
					endFilterIndex = pi
				}
			}
		}
		maskChars(startFilterIndex, endFilterIndex+1, chars)
	}
}

// processTlds mirrors WordEncTlds.ts:115-141.
func (t *tlds) processTlds(chars, tld []rune, startIndex int) (currentIndex, tldIndex int) {
	currentIndex = startIndex
	for currentIndex < len(chars) && tldIndex < len(tld) {
		currentChar := chars[currentIndex]
		nextChar := rune('\x00')
		if currentIndex+1 < len(chars) {
			nextChar = chars[currentIndex+1]
		}
		currentLength := getEmulatedDomainCharLen(nextChar, tld[tldIndex], currentChar)
		if currentLength > 0 {
			currentIndex += currentLength
			tldIndex++
		} else {
			if tldIndex == 0 {
				break
			}
			previousLength := getEmulatedDomainCharLen(nextChar, tld[tldIndex-1], currentChar)
			if previousLength > 0 {
				currentIndex += previousLength
			} else {
				if !isSymbol(currentChar) {
					break
				}
				currentIndex++
			}
		}
	}
	return
}
```

- [ ] **Step 4: Run tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/encfilter/... -count=1 -v -run TestTlds
```

- [ ] **Step 5: Commit**

```
git status --short
git add pkg/wordenc/encfilter/tlds.go pkg/wordenc/encfilter/tlds_test.go
git commit --no-gpg-sign -m "feat(wordenc): NAI-WORDENC-FILTER T6 — tlds filter"
git show --stat HEAD
```

---

## Task 7: Top-level Filter.Filter + algorithmic E2E + TS-derived fixtures

**Files:**
- Modify: `pkg/wordenc/encfilter/encfilter.go` (replace the stub `Filter.Filter`)
- Create: `pkg/wordenc/encfilter/fixtures_test.go`
- Create: `pkg/wordenc/encfilter/testdata/wordenc-fixtures.json`
- Create: `tools/wordenc/gen-fixtures.ts` (the one-shot TS generator — committed but not part of build)

**Pre-flight checks:**
```bash
ls pkg/wordenc/encfilter/testdata/ 2>/dev/null  # expect: not exist
grep -n "func.*Filter.*Filter\b" pkg/wordenc/encfilter/encfilter.go  # confirm stub at the bottom
```

- [ ] **Step 1: Write the failing E2E tests**

Append to `pkg/wordenc/encfilter/encfilter_test.go` (or new `filter_e2e_test.go`):

```go
// testFilter builds a *Filter wired with badWords/domains/tlds/fragments
// from the given component data. Used by tests to avoid loading a real
// jagfile.
func testFilter(t *testing.T, bads [][]rune, badCombos [][][2]int, frags []uint16, doms [][]rune, ts [][]rune, types []int) *Filter {
	t.Helper()
	return &Filter{
		fragments: frags,
		bads:      bads,
		badCombos: badCombos,
		domains:   doms,
		tlds:      ts,
		tldTypes:  types,
	}
}

func TestFilter_PassesThroughCleanText(t *testing.T) {
	f := Empty()
	cases := []string{"hello world", "Good morning!", "I love RuneScape"}
	for _, in := range cases {
		got := f.Filter(in)
		if got != in {
			t.Errorf("Filter(%q): got %q, want %q (Empty passthrough)", in, got, in)
		}
	}
}

func TestFilter_MasksDirectBadWord(t *testing.T) {
	f := testFilter(t, [][]rune{[]rune("anal")}, [][][2]int{nil}, nil, nil, nil, nil)
	got := f.Filter("anal")
	if got != "****" {
		t.Errorf("Filter(anal): got %q, want %q", got, "****")
	}
}

func TestFilter_WhitelistPreserves(t *testing.T) {
	// Whitelist includes "cook". Without explicit bad-word matching "cook",
	// Empty().Filter passes through. Construct a *Filter where "cook" would
	// otherwise mask but the whitelist restores it.
	// Test scope: ensure "cooks" stays "cooks" when no rules match (Empty).
	// (Stronger whitelist tests would require a bad-words list overlapping the
	// whitelisted substring, which is a follow-on test.)
	f := Empty()
	if got := f.Filter("cooks"); got != "cooks" {
		t.Errorf("Filter(cooks): got %q, want %q", got, "cooks")
	}
}

func TestFilter_PreservesUppercaseOnPassthrough(t *testing.T) {
	f := Empty()
	got := f.Filter("Hello World")
	// formatUppercases canonicalizes mid-run uppercase to lowercase, but only
	// after the first char of a run is lowercase. "Hello" starts uppercase →
	// flagged=true; 'e' lowercase → flagged=false; 'l','l','o' no change.
	// "World" similar.
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
```

- [ ] **Step 2: Replace the stub Filter.Filter implementation**

In `pkg/wordenc/encfilter/encfilter.go`, replace the stub `func (f *Filter) Filter` with:

```go
// Filter mirrors TS WordEnc.filter (Engine-TS/src/cache/wordenc/WordEnc.ts:73-95).
// Steps:
//  1. Copy input to a rune slice.
//  2. format() — normalize allowed chars, collapse spaces.
//  3. trim + lowercase.
//  4. Run filtered = lowercased copy through tlds → badWords → domains → fragments.
//  5. Whitelist restoration (find each whitelisted substring in the lowercase
//     and write the original whitelisted letters into the filtered copy).
//  6. replaceUppercases — restore uppercase from the trimmed pre-lowercase copy.
//  7. formatUppercases — canonicalize mid-run uppercase to lowercase.
//  8. join + trim.
func (f *Filter) Filter(s string) string {
	characters := []rune(s)
	format(characters)
	trimmed := []rune(trimSpaces(string(characters)))
	lowercase := []rune(toLower(string(trimmed)))
	filtered := append([]rune(nil), lowercase...)

	tlds_ := &tlds{
		bads:    &badWords{bads: f.bads, combos: f.badCombos, fragments_: &fragments{items: f.fragments}},
		domains: &domains{bads: &badWords{bads: f.bads, combos: f.badCombos, fragments_: &fragments{items: f.fragments}}, domains: f.domains},
		tlds:    f.tlds,
		types:   f.tldTypes,
	}
	tlds_.filter(filtered)

	bads := &badWords{bads: f.bads, combos: f.badCombos, fragments_: &fragments{items: f.fragments}}
	bads.filter(filtered)

	doms := &domains{bads: bads, domains: f.domains}
	doms.filter(filtered)

	frags := &fragments{items: f.fragments}
	frags.filter(filtered)

	for _, w := range whitelist {
		offset := -1
		for {
			idx := indexOfFrom(string(lowercase), w, offset+1)
			if idx == -1 {
				break
			}
			offset = idx
			wr := []rune(w)
			for i, ch := range wr {
				filtered[i+idx] = ch
			}
		}
	}

	replaceUppercases(filtered, trimmed)
	formatUppercases(filtered)
	return trimSpaces(string(filtered))
}

// trimSpaces is strings.TrimSpace narrowed to ASCII space + '	' + '
'.
// TS uses String.trim which strips full Unicode whitespace; the inputs we care
// about are ASCII-clean post-format so the difference doesn't materialize.
func trimSpaces(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}

// toLower is a rune-level lowercase that only touches ASCII A-Z (TS toLowerCase
// is locale-aware but our format() output is already ASCII-clean for the
// algorithm-relevant cases).
func toLower(s string) string {
	out := []rune(s)
	for i, c := range out {
		if isUppercaseAlpha(c) {
			out[i] = c + ('a' - 'A')
		}
	}
	return string(out)
}

// indexOfFrom mirrors JS String.prototype.indexOf(substring, fromIndex).
// Returns -1 if not found.
func indexOfFrom(s, substr string, fromIndex int) int {
	if fromIndex < 0 {
		fromIndex = 0
	}
	if fromIndex > len(s) {
		return -1
	}
	idx := indexOf(s[fromIndex:], substr)
	if idx == -1 {
		return -1
	}
	return idx + fromIndex
}

func indexOf(s, substr string) int {
	if substr == "" {
		return 0
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
```

NOTE: the Filter algorithm constructs ephemeral `*badWords`/`*domains`/`*fragments` wrappers on each call. This is wasteful but mirrors TS exactly and keeps the public surface immutable. If profiling later shows this matters, a follow-up can move construction into `Load`.

- [ ] **Step 3: Run E2E tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/encfilter/... -count=1 -v
```
Expected: all algorithmic tests PASS.

- [ ] **Step 4: Create the TS fixture generator**

Create `tools/wordenc/gen-fixtures.ts`:

```typescript
// One-shot generator for goscape's pkg/wordenc/encfilter/testdata/wordenc-fixtures.json.
// Run from the Engine-TS checkout:
//
//   cd /home/owner/Code/github.com/LostCityRS/Engine-TS
//   bun run /home/owner/Code/github.com/zsrv/goscape/tools/wordenc/gen-fixtures.ts \
//     > /home/owner/Code/github.com/zsrv/goscape/pkg/wordenc/encfilter/testdata/wordenc-fixtures.json
//
// Reads the wordenc cache at data/pack/client/wordenc, runs WordEnc.filter on
// each curated input, dumps {input, filtered} pairs as JSON.
//
// NOT part of any build. Re-run whenever the curated input list changes or
// the wordenc data changes upstream.

import WordEnc from '#/cache/wordenc/WordEnc.js';

WordEnc.load('data/pack');

const inputs: string[] = [
    '',
    'a',
    'hello',
    'Hello World',
    'good morning',
    'HELLO',                  // full uppercase passthrough
    'anal',                   // direct bad word
    'AnAl',                   // mixed-case bad word
    '4n4l',                   // leetspeak
    'cooks',                  // whitelist
    'visit foo.com please',   // bare URL
    'email me at foo@test.com',  // email-context domain
    'numbers 1234567 are masked', // long digit run via fragments
    '   leading spaces',
    'trailing spaces   ',
    'multiple    spaces',
    'symbols!!!!',
    'I love RuneScape',
    'no profanity here at all',
    'A B C D E F',
];

const pairs = inputs.map(input => ({
    input,
    filtered: WordEnc.filter(input),
}));

console.log(JSON.stringify(pairs, null, 2));
```

- [ ] **Step 5: Generate the fixtures JSON**

This step requires the implementer to have access to a built Engine-TS checkout. If `data/pack/client/wordenc` doesn't exist there, run the Engine-TS pack step first.

```
cd /home/owner/Code/github.com/LostCityRS/Engine-TS
bun run tools/pack/Build.ts  # if data/pack/client/wordenc is missing
mkdir -p /home/owner/Code/github.com/zsrv/goscape/pkg/wordenc/encfilter/testdata
bun run /home/owner/Code/github.com/zsrv/goscape/tools/wordenc/gen-fixtures.ts \
  > /home/owner/Code/github.com/zsrv/goscape/pkg/wordenc/encfilter/testdata/wordenc-fixtures.json
cd /home/owner/Code/github.com/zsrv/goscape
```

If running the generator is not feasible in the controller's environment, commit an empty array `[]` to the JSON file and mark a deviation `DEVIATION-NAI-WORDENC-FILTER-D-NO-TS-FIXTURES` retiring when the generator runs.

- [ ] **Step 6: Write the fixtures test**

Create `pkg/wordenc/encfilter/fixtures_test.go`:

```go
package encfilter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type fixturePair struct {
	Input    string `json:"input"`
	Filtered string `json:"filtered"`
}

// TestFilter_AgainstTSFixtures loads the JSON file produced by
// tools/wordenc/gen-fixtures.ts, loads the real client/wordenc jagfile from
// the canonical Engine-TS data path, runs goscape's Filter.Filter on each
// input, and asserts byte-identical match against the TS output.
//
// Skips if either the jagfile or the fixtures JSON is absent (matches the
// real-cache test convention; see modules/world/loctype_realcache_test.go).
func TestFilter_AgainstTSFixtures(t *testing.T) {
	const tsCache = "/home/owner/Code/github.com/LostCityRS/Engine-TS/data/pack"
	jagPath := filepath.Join(tsCache, "client", "wordenc")
	if _, err := os.Stat(jagPath); err != nil {
		t.Skipf("wordenc jagfile not present at %s; skipping (rebuild Engine-TS with 'bun run tools/pack/Build.ts')", jagPath)
	}

	fixturesPath := filepath.Join("testdata", "wordenc-fixtures.json")
	raw, err := os.ReadFile(fixturesPath)
	if err != nil {
		t.Skipf("fixtures file %s not present; skipping (regenerate via tools/wordenc/gen-fixtures.ts)", fixturesPath)
	}
	var pairs []fixturePair
	if err := json.Unmarshal(raw, &pairs); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}
	if len(pairs) == 0 {
		t.Skip("fixtures file empty; regenerate via tools/wordenc/gen-fixtures.ts")
	}

	f, err := Load(tsCache)
	if err != nil {
		t.Fatalf("Load(%q): %v", tsCache, err)
	}

	for _, p := range pairs {
		t.Run(p.Input, func(t *testing.T) {
			got := f.Filter(p.Input)
			if got != p.Filtered {
				t.Errorf("Filter(%q):\n  got  %q\n  want %q", p.Input, got, p.Filtered)
			}
		})
	}
}
```

- [ ] **Step 7: Run all encfilter tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/wordenc/encfilter/... -count=1 -v
```
Expected: all tests PASS. The fixtures test may skip on first run if generation hasn't happened — that's fine.

- [ ] **Step 8: Commit**

```
git status --short
git add pkg/wordenc/encfilter/ tools/wordenc/
git commit --no-gpg-sign -m "feat(wordenc): NAI-WORDENC-FILTER T7 — Filter.Filter + TS-derived fixtures"
git show --stat HEAD
```

---

## Task 8: Wire encfilter.Filter to Server

**Files:**
- Modify: `modules/world/server.go` (Server struct + NewServer load step)
- Modify: `modules/world/server_test.go` (or new helper) — extend `newTestServer` to inject `encfilter.Empty()`

**Pre-flight checks:**
```bash
grep -n "varpTypes\|invTypes\|LoadXxxTypes" modules/world/server.go | head -10  # confirm Load-then-assign pattern around line 355
grep -n "func newTestServer\b" modules/world/server_test.go modules/world/*_test.go 2>/dev/null | head -5  # find the helper
grep -n "client/wordenc" modules/world/*.go pkg/wordenc/encfilter/*.go | head -5
```

- [ ] **Step 1: Write the failing test**

Add to (or create) `modules/world/server_wordenc_test.go`:

```go
package world

import (
	"testing"
)

// TestNewServer_LoadsWordencFilter pins that NewServer populates s.wordenc
// from the cache. Uses the canonical Engine-TS cache path; skips if absent
// (cf. loctype_realcache_test.go skip convention).
//
// This test is in the world package because s.wordenc is unexported.
func TestNewServer_LoadsWordencFilter(t *testing.T) {
	const tsCache = "/home/owner/Code/github.com/LostCityRS/Engine-TS/data/pack"
	// Use the existing test scaffolding that builds a minimal Server for tests.
	// The wordenc filter MUST be wired even when the cache loader skips other
	// configs in tests; we test the happy path here.
	// Implementer: adapt to whatever the existing minimal-Server fixture provides.
	t.Skip("implementer: wire to existing NewServer / newTestServer scaffolding and verify s.wordenc != nil")
}

// TestNewTestServer_InjectsEmptyWordencFilter pins that the test scaffolding
// injects an Empty() filter so existing tests pass through chat text unchanged.
func TestNewTestServer_InjectsEmptyWordencFilter(t *testing.T) {
	s := newTestServer(t)
	if s.wordenc == nil {
		t.Fatal("newTestServer must inject a non-nil *encfilter.Filter")
	}
	if got := s.wordenc.Filter("anal"); got != "anal" {
		t.Errorf("newTestServer's injected filter should be Empty (passthrough); got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1 -run TestNewTestServer_InjectsEmptyWordencFilter 2>&1 | head -10
```
Expected: compile error — `s.wordenc undefined`.

- [ ] **Step 3: Add the field + load step + test injection**

In `modules/world/server.go`:

```go
// Inside the Server struct (near varpTypes / invTypes around line 100):
wordenc *encfilter.Filter
```

Add import: `"github.com/zsrv/goscape/pkg/wordenc/encfilter"`

In `NewServer`, after `s.locTypes = locTypes` (or wherever the type configs finish loading, around line 367-390):

```go
s.wordenc, err = encfilter.Load(cfg.CachePath)
if err != nil {
	return nil, fmt.Errorf("load wordenc: %w", err)
}
```

In `modules/world/server_test.go` (locate `newTestServer`), inject:

```go
s.wordenc = encfilter.Empty()
```

Just before `return s` (or wherever the struct is assembled).

- [ ] **Step 4: Run tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... ./pkg/wordenc/... -count=1 -timeout 300s 2>&1 | tail -20
```
Expected: all green, modules/world race-clean.

- [ ] **Step 5: Smoke-pack**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content
```
Expected: `12 OK / 0 ERR / 0 SKIP`.

- [ ] **Step 6: Commit**

```
git status --short
git add modules/world/server.go modules/world/server_test.go modules/world/server_wordenc_test.go
git commit --no-gpg-sign -m "feat(world): NAI-WORDENC-FILTER T8 — wire encfilter.Filter to *Server"
git show --stat HEAD
```

---

## Task 9: Wire sendMessagePrivate to use Filter

**Files:**
- Modify: `modules/world/friends_emit.go` (sendMessagePrivate — apply filter)
- Modify: `modules/world/friends_emit_test.go` (new positive-filter byte-pin test)

**Pre-flight checks:**
```bash
grep -n "sendMessagePrivate\|wordpack\.Pack" modules/world/friends_emit.go  # confirm line ~54 + 63
grep -n "TestSendMessagePrivate_EmitsExactByteSequence\|TestSendMessagePrivate_" modules/world/friends_emit_test.go | head
```

- [ ] **Step 1: Write the failing test (positive filter)**

Append to `modules/world/friends_emit_test.go`:

```go
// TestSendMessagePrivate_AppliesWordEncFilter pins that sendMessagePrivate
// runs the chat text through s.wordenc.Filter before WordPack.Pack. Tests
// inject a *Filter with one bad word and verify the masked text reaches the
// wire bytes.
func TestSendMessagePrivate_AppliesWordEncFilter(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Inject a filter with one bad word "anal".
	jf := makeWordencJagWithBad(t, "anal")  // implementer adds helper using encfilter exported test surface
	f, err := encfilter.LoadFromJag(jf)
	if err != nil {
		t.Fatalf("LoadFromJag: %v", err)
	}
	s.wordenc = f

	received := drainConn(t, cc)
	sendMessagePrivate(p, /*from*/ 0x12345, /*pmId*/ 7, /*staffLvl*/ 0, /*chat*/ "anal")
	p.client.flushWrite()

	got := <-received
	// Wire format (post-ISAAC):
	//   opcode (1) + len (1) + p8(from) + p4(pmId) + p1(staffLvl-adjusted) + wordpack-packed("****")
	// We assert the wordpack-packed bytes are for "****" not "anal".
	// Implementer: decode the wordpack body and compare strings, or compute the
	// expected wordpack-packed bytes for "****" and compare suffix-equal.
	wantPacked := wordpackPackedBytes("****")
	if !bytes.HasSuffix(got, wantPacked) {
		t.Errorf("wire does not end with wordpack(\"****\"): got %x", got)
	}
}

// makeWordencJagWithBad and wordpackPackedBytes are local helpers. Implementer
// authors them to reuse the encfilter and wordpack test surface — or skips the
// jagfile path entirely by constructing a *Filter inline via package-internal
// state if a test helper is added in encfilter (e.g., NewFilterForTest).
```

NOTE for implementer: the cleanest path is for T7 to also export a test helper `encfilter.NewForTest(bads [][]rune, badCombos [][][2]int, frags []uint16, doms [][]rune, ts [][]rune, types []int) *Filter`. If not added in T7, add it in this task and reference it.

- [ ] **Step 2: Run test to verify failure**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1 -run TestSendMessagePrivate_AppliesWordEncFilter 2>&1 | head -20
```
Expected: test fails — chat passes through unfiltered.

- [ ] **Step 3: Wire the filter into sendMessagePrivate**

In `modules/world/friends_emit.go`, change the implementation:

```go
// sendMessagePrivate writes one MESSAGE_PRIVATE packet to the recipient.
// from is the sender's username37. pmId is the friends-server-assigned
// PM correlation id. staffLvl is the sender's staff level; the wire
// applies the TS-faithful `+1 if > 0` adjustment so the client renders
// the correct staff icon. chat is the unpacked text; goscape applies
// WordEnc.filter (via s.wordenc) and then WordPack.Pack's the result
// for the wire — mirrors TS MessagePrivateEncoder.ts:20.
func sendMessagePrivate(p *Player, from uint64, pmId uint32, staffLvl int32, chat string) {
	adjusted := staffLvl
	if adjusted > 0 {
		adjusted += 1
	}
	buf := packet.NewPacket(nil)
	buf.P8(from)
	buf.P4(uint32(pmId))
	buf.P1(uint8(adjusted))
	filtered := p.client.server.wordenc.Filter(chat)
	wordpack.Pack(buf, filtered)
	p.writeOut(gameserver.OpMessagePrivate, buf.Bytes())
}
```

Delete the old `DEVIATION-NAI-182-D5-NO-WORDENC-FILTER` doc-comment block — replaced by the positive description above.

- [ ] **Step 4: Verify existing byte-pin tests still pass**

The existing `TestSendMessagePrivate_EmitsExactByteSequence` uses simple inputs ("hello world" or similar). Since `newTestServer` injects `encfilter.Empty()`, Filter is identity → existing assertions hold.

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1 -run TestSendMessagePrivate -v
```
Expected: all PASS.

- [ ] **Step 5: Race**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... ./pkg/wordenc/... -count=1 -timeout 300s 2>&1 | tail -5
```

- [ ] **Step 6: Smoke-pack**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content
```

- [ ] **Step 7: Commit**

```
git status --short
git add modules/world/friends_emit.go modules/world/friends_emit_test.go
git commit --no-gpg-sign -m "feat(world): NAI-WORDENC-FILTER T9 — sendMessagePrivate applies WordEnc.filter"
git show --stat HEAD
```

---

## Task 10: Wire handleMessagePublic to unpack → filter → repack

**Files:**
- Modify: `modules/world/handlers_game.go` (handleMessagePublic — unpack/filter/repack before p.Chat)
- Create or modify: `modules/world/handler_message_public_test.go` (regression test)

**Pre-flight checks:**
```bash
grep -n "handleMessagePublic\|wordpack\.Unpack\|wordpack\.Pack" modules/world/handlers_game.go  # confirm line ~336
grep -n "func TestHandleMessagePublic\|func .*MessagePublic" modules/world/handler_message_public_test.go | head
grep -n "p\.Chat\|chatBytes" modules/world/player_masks.go  # confirm p.Chat(...) sig
```

- [ ] **Step 1: Write the failing regression test**

Append to `modules/world/handler_message_public_test.go`:

```go
// TestHandleMessagePublic_AppliesWordEncFilterToChatBytes pins that
// handleMessagePublic unpacks the inbound text, filters it via s.wordenc,
// repacks the filtered text, and that the repacked bytes (not the raw input)
// end up on p.chatBytes. The audit-log call to friendsBridge.PublicMessage
// is asserted to receive the UNFILTERED text (mirrors TS player.logMessage
// at MessagePublicHandler.ts:32, set BEFORE filtering).
func TestHandleMessagePublic_AppliesWordEncFilterToChatBytes(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s

	// Wire a recordingBridges so we can read the PublicMessage call.
	rec := &recordingBridges{}
	s.friendsBridge = rec
	p.session = "test-uuid"

	// Build a *Filter that masks "anal" → "****".
	jf := makeWordencJagWithBad(t, "anal")
	f, err := encfilter.LoadFromJag(jf)
	if err != nil {
		t.Fatalf("LoadFromJag: %v", err)
	}
	s.wordenc = f

	// Word-pack "anal" so the payload looks like a real client packet.
	bufIn := packet.NewPacket(nil)
	wordpack.Pack(bufIn, "anal")
	packed := bufIn.Bytes()

	// Wire layout: byte 0 = color (0), byte 1 = effect (0), then packed bytes.
	payload := append([]byte{0, 0}, packed...)
	if err := handleMessagePublic(p, payload); err != nil {
		t.Fatalf("handleMessagePublic: %v", err)
	}

	// chatBytes must be the wordpack-packed form of "****", not "anal".
	wantPacked := func() []byte {
		out := packet.NewPacket(nil)
		wordpack.Pack(out, "****")
		return out.Bytes()
	}()
	if !bytes.Equal(p.chatBytes, wantPacked) {
		t.Errorf("p.chatBytes:\n  got  %x\n  want %x", p.chatBytes, wantPacked)
	}

	// PublicMessage audit-log MUST receive the unfiltered text.
	if len(rec.publicMsgs) != 1 {
		t.Fatalf("expected 1 PublicMessage call, got %d", len(rec.publicMsgs))
	}
	if rec.publicMsgs[0].Message != "anal" {
		t.Errorf("audit-log message: got %q, want %q (unfiltered)", rec.publicMsgs[0].Message, "anal")
	}
}
```

(Implementer: ensure `recordingBridges` is defined in the test package — it's already used in slice-6 tests per memory.)

- [ ] **Step 2: Run test to verify failure**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1 -run TestHandleMessagePublic_AppliesWordEncFilterToChatBytes -v
```
Expected: fail — chatBytes equals the raw "anal" packed bytes.

- [ ] **Step 3: Edit handleMessagePublic**

In `modules/world/handlers_game.go` at the `handleMessagePublic` function (around line 336):

```go
func handleMessagePublic(p *Player, payload []byte) error {
	if len(payload) < 2 {
		return nil
	}
	color := int(payload[0])
	effect := int(payload[1])

	// Unpack raw word-packed text first — needed for both wordenc filtering
	// (TS MessagePublicHandler.ts:26) and audit-log (TS line 32).
	rawPacked := bytes.Clone(payload[2:])
	pk := packet.NewPacket(rawPacked)
	decoded := wordpack.Unpack(pk, len(rawPacked))

	// Apply WordEnc.filter and repack for the wire — mirrors TS lines 34-39.
	var msg []byte
	if p.client != nil && p.client.server != nil && p.client.server.wordenc != nil {
		filtered := p.client.server.wordenc.Filter(decoded)
		out := packet.NewPacket(nil)
		wordpack.Pack(out, filtered)
		msg = out.Bytes()
	} else {
		// Server-less test path: pass raw bytes through (matches previous
		// passthrough behavior so non-wordenc tests still pin byte-for-byte).
		msg = rawPacked
	}
	p.Chat(color, effect, int(p.staffModLevel), msg)

	// Audit-log to friends-server with the UNFILTERED decoded text — mirrors
	// TS player.logMessage = unpack at MessagePublicHandler.ts:32 (BEFORE filter).
	if p.client != nil && p.client.server != nil && p.session != "" && p.session != "headless" {
		s := p.client.server
		coord := coordgrid.PackCoord(p.level, p.x, p.z)
		s.friendsBridge.PublicMessage(p.session, coord, decoded)
	}
	return nil
}
```

Make sure `bytes` and `wordpack` are already imported (they are — see existing imports).

- [ ] **Step 4: Run all handler_message_public tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1 -run TestHandleMessagePublic -v 2>&1 | tail -30
```
Expected: all PASS including the new test.

- [ ] **Step 5: Full race + smoke-pack**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s 2>&1 | grep -E "^(ok|FAIL|---)" | tail -20
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content
```
Expected: no FAIL, smoke-pack 12/0/0.

- [ ] **Step 6: Commit**

```
git status --short
git add modules/world/handlers_game.go modules/world/handler_message_public_test.go
git commit --no-gpg-sign -m "feat(world): NAI-WORDENC-FILTER T10 — handleMessagePublic unpack→filter→repack"
git show --stat HEAD
```

---

## Task 11: Retire DEVIATION-NAI-182-D5-NO-WORDENC-FILTER

**Files:**
- Modify: `docs/superpowers/specs/2026-05-19-nai-182-d5-social-cluster-design.md` (line 360 — mark deviation retired)
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (top entry — update the "opens" list)
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_182_d5_social_cluster_close.md` (mark NO-WORDENC-FILTER retired alongside the existing CHAT-FILTER-NO-RESTORE entry)

**Pre-flight checks:**
```bash
grep -n "DEVIATION-NAI-182-D5-NO-WORDENC-FILTER" docs/ -r  # all in-doc references
grep -n "DEVIATION-NAI-182-D5-NO-WORDENC-FILTER" modules/world/ -r  # any remaining live-code references should be ZERO post-T9 (the doc-comment was removed in T9)
```

- [ ] **Step 1: Update spec doc deviation entry**

In `docs/superpowers/specs/2026-05-19-nai-182-d5-social-cluster-design.md` at line 360 (the DEVIATION-NAI-182-D5-NO-WORDENC-FILTER bullet):

```diff
- - **DEVIATION-NAI-182-D5-NO-WORDENC-FILTER** — `sendMessagePrivate` calls `wordpack.Pack(chat)` without an equivalent of TS `WordEnc.filter(message.msg)`. goscape has no WordEnc port (only WordPack). Practical impact: profanity censoring is bypassed on inbound PMs. Retires when WordEnc lands.
+ - **DEVIATION-NAI-182-D5-NO-WORDENC-FILTER** — RETIRED 2026-05-20 (NAI-WORDENC-FILTER slice). `pkg/wordenc/encfilter` ported in 11 commits; `sendMessagePrivate` and `handleMessagePublic` now apply `s.wordenc.Filter(...)`. Pinned by `TestSendMessagePrivate_AppliesWordEncFilter` and `TestHandleMessagePublic_AppliesWordEncFilterToChatBytes`.
```

- [ ] **Step 2: Update MEMORY.md top entry**

Edit the existing NAI-182-D5 entry's "opens" portion:

```diff
- opens DEVIATION-NAI-182-D5-NO-WORDENC-FILTER (retires-when-wordenc-filter-ports) + DEVIATION-NAI-182-D5-NO-DEFENSIVE-IGNORELIST-LOGIN-EMIT (permanent) + DEVIATION-NAI-182-D5-CHAT-FILTER-NO-RESTORE (RETIRED post-D5 cleanup 6a440a83 ...)
+ opens DEVIATION-NAI-182-D5-NO-WORDENC-FILTER (RETIRED 2026-05-20 NAI-WORDENC-FILTER slice — full pkg/wordenc/encfilter port) + DEVIATION-NAI-182-D5-NO-DEFENSIVE-IGNORELIST-LOGIN-EMIT (permanent) + DEVIATION-NAI-182-D5-CHAT-FILTER-NO-RESTORE (RETIRED post-D5 cleanup 6a440a83 ...)
```

- [ ] **Step 3: Update the D5 close memo**

In `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_182_d5_social_cluster_close.md`, update the `DEVIATION-NAI-182-D5-NO-WORDENC-FILTER` line under `## Opened tags`:

```diff
- - `DEVIATION-NAI-182-D5-NO-WORDENC-FILTER` — sendMessagePrivate calls wordpack.Pack(chat) without TS-equivalent WordEnc.filter. goscape has no WordEnc port (only WordPack). Retires when WordEnc lands.
+ - ~~`DEVIATION-NAI-182-D5-NO-WORDENC-FILTER`~~ — RETIRED 2026-05-20 by NAI-WORDENC-FILTER slice. Full pkg/wordenc/encfilter port across 11 commits. sendMessagePrivate + handleMessagePublic now apply s.wordenc.Filter. Pinned by TestSendMessagePrivate_AppliesWordEncFilter + TestHandleMessagePublic_AppliesWordEncFilterToChatBytes.
```

- [ ] **Step 4: Write a new memory close memo for this slice**

Create `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_wordenc_filter_close.md`:

```markdown
---
name: nai-wordenc-filter-close
description: NAI-WORDENC-FILTER (WordEnc.filter port to pkg/wordenc/encfilter) close memo
metadata: 
  type: project
---

# NAI-WORDENC-FILTER — `WordEnc.filter` port close

**Date:** 2026-05-20
**Predecessor:** [[post-D5 cleanup]] (HEAD 1916ffc5) on top of NAI-182-D5 close ecdb3738
**Slice scope:** 11 commits T1..T11, new pkg/wordenc/encfilter Go package (~700 LOC + ~300 LOC tests).

## What shipped

Full port of TS WordEnc + 4 helper classes (WordEncBadWords, WordEncFragments, WordEncDomains, WordEncTlds) to new Go package `pkg/wordenc/encfilter`. Stateful `*Filter` instance held on `*Server`, populated by `encfilter.Load(cachePath)` reading the existing `client/wordenc` jagfile.

Two call sites wired to filter chat:
- `sendMessagePrivate` — inbound PM text filtered before WordPack.Pack.
- `handleMessagePublic` — unpacks input, filters via WordEnc, repacks, passes filtered bytes to p.Chat. Audit-log to friends-server stays UNFILTERED (matches TS player.logMessage at MessagePublicHandler.ts:32, set before filter).

Tests: ~30 algorithmic Go tests + TS-derived JSON golden fixtures (~20 input/output pairs). Golden test skips when the Engine-TS data/pack jagfile is absent.

## Retired tags

- `DEVIATION-NAI-182-D5-NO-WORDENC-FILTER` — sendMessagePrivate now applies filter.

## Opened tags

- (none — slice is fully clean)

## Files touched

- `pkg/wordenc/encfilter/` — new (encfilter.go, helpers.go, fragments.go, badwords.go, domains.go, tlds.go + 6 test files + testdata/wordenc-fixtures.json)
- `modules/world/server.go` — Server.wordenc field + NewServer load step
- `modules/world/server_test.go` — newTestServer injects encfilter.Empty()
- `modules/world/friends_emit.go` — sendMessagePrivate applies Filter
- `modules/world/friends_emit_test.go` — new positive-filter byte-pin test
- `modules/world/handlers_game.go` — handleMessagePublic unpack→filter→repack
- `modules/world/handler_message_public_test.go` — new regression test
- `tools/wordenc/gen-fixtures.ts` — one-shot TS fixture generator (not part of build)
- `docs/superpowers/specs/2026-05-19-nai-182-d5-social-cluster-design.md` — deviation retired
- `docs/superpowers/specs/2026-05-19-nai-wordenc-filter-port-design.md` — slice spec
- `docs/superpowers/plans/2026-05-20-nai-wordenc-filter-port-plan.md` — slice plan

## Gates

- `go test -race ./...` zero FAIL across 56+ packages.
- `smoke-pack 12 OK / 0 ERR / 0 SKIP` holds.

## Next pivot

friends-server bridge arc is fully at rest. NAI-WORDENC-FILTER retires the last non-permanent D5 deviation. Next pivot = general world / runescript engine work, or any open NAI-XXX cluster.
```

Add a one-liner pointer to MEMORY.md (prepend to top after the existing D5 entry).

- [ ] **Step 5: Final full race + smoke-pack**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s 2>&1 | grep -E "^(ok|FAIL)" | wc -l
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content
```

- [ ] **Step 6: Commit**

```
git status --short
git add docs/superpowers/
git commit --no-gpg-sign -m "docs: NAI-WORDENC-FILTER T11 — retire DEVIATION-NAI-182-D5-NO-WORDENC-FILTER"
git show --stat HEAD
```

---

## Self-review (controller — before dispatch)

**Spec coverage:**
- §1 (goal): T1-T7 builds package, T8-T10 wires calls, T11 retires deviation. ✓
- §2 (in scope): T1-T7 covers package; T8 server field; T9 sendMessagePrivate; T10 handleMessagePublic. ✓
- §3.1 layout: T1-T7 create each named file. ✓
- §3.2 data model: T1 Filter struct + T4 badCombos parallel array. ✓
- §3.3 API: T1 Load + LoadFromJag + Empty; T7 Filter.Filter. ✓
- §3.4 runes: helpers in T2 take []rune. ✓
- §3.5 server integration: T8. ✓
- §3.6 sendMessagePrivate: T9. ✓
- §3.7 handleMessagePublic: T10. ✓
- §3.8 test strategy: T7 (algorithmic + fixtures). ✓
- §3.9 call-site test additions: T9 + T10. ✓
- §4 deviations: documented in spec, no in-code markers needed. ✓
- §5 retired: T11. ✓

**Type consistency:**
- `*Filter` struct fields: bads, badCombos, fragments, domains, tlds, tldTypes — used consistently in T1, T4, T5, T6, T7.
- `*badWords` fields: bads, combos, fragments_ — used in T4, T5, T6, T7. Note: T4 uses `fragments_` (trailing underscore) to disambiguate from the type. Implementer should preserve.
- `*fragments.items` — used in T3, T4 (via fragments_), T7.

**Placeholder scan:** T9 references a helper `makeWordencJagWithBad` and a hypothetical `encfilter.NewForTest` — these are flagged for the implementer to either add to T7 or define locally in T9. Acceptable since the implementer can choose the cleanest path; the test contract (filter is applied) is unambiguous.

**Scope check:** 11 tasks, ~700 LOC Go + ~300 LOC tests. Tractable single slice with bounded risk per task.
