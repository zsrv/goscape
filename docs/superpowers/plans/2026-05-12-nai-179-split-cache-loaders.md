# NAI-179 — SPLIT_* cache-loader port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `MesanimType` (server `.dat`) and new `pkg/fonttype` (client `title` Jagfile) cache loaders, surface them through `pkg/script.Configs`, and rewrite `handleSplitInit`/`handleSplitGetAnim` to call them — retiring `NAI-75-D-MESANIM-NOT-PORTED` and `NAI-75-D-FONT-WRAP-NAIVE`.

**Architecture:** MesanimType mirrors `IdkType`'s server-only loader pattern (one new file in `pkg/objtype`). FontType is a new package (`pkg/fonttype`) decoding 4 bitmap fonts from `data/pack/client/title`, retaining only per-char `drawWidth` and the TS-faithful `Split(str, maxWidth) []string` word-wrap algorithm. Both surface via three new methods on `pkg/script.Configs`. `handleSplitInit` resolves `<p,name>` to a real MesanimType id and calls `font.Split` instead of `strings.Split(text, "|")`; `handleSplitGetAnim` reads `MesanimType.Len[lineCount-1]`.

**Tech Stack:** Go 1.26+, `pkg/io/jagfile`, `pkg/io/packet`, existing `pkg/objtype.ConfigType` base shape.

**Spec:** `docs/superpowers/specs/2026-05-12-nai-179-split-cache-loaders-design.md` (commit `869e258`).

**TS source anchors:**
- `Engine-TS/src/cache/config/MesanimType.ts:1-71`
- `Engine-TS/src/cache/config/FontType.ts:1-177`
- `Engine-TS/src/engine/script/handlers/StringOps.ts:76-122`

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `pkg/objtype/mesanimtype.go` | Create | `MesanimType` config struct + `LoadMesanimTypes` |
| `pkg/objtype/mesanimtype_test.go` | Create | Decoder + load tests |
| `pkg/fonttype/fonttype.go` | Create | `FontType` struct, `Load`, `StringWidth`, `Split`, `CharLookup` |
| `pkg/fonttype/fonttype_test.go` | Create | CharLookup, Load, StringWidth, Split tests |
| `pkg/script/configs.go` | Modify | Add 3 interface methods |
| `pkg/script/handlers_config_test.go` | Modify | Add mockConfigs fields + 3 methods |
| `pkg/script/handlers_string.go` | Modify | Rewrite `handleSplitInit`, `handleSplitGetAnim`; retire NAI-75-D doc-comments |
| `pkg/script/handlers_string_test.go` | Modify | Update existing pins, add 6 new tests, retire NAI-75-D test pins |
| `pkg/script/state.go` | Modify | Retire NAI-75-D-MESANIM-NOT-PORTED doc-comment |
| `modules/world/server.go` | Modify | Load `mesanimTypes` + `fontTypes`; store on `Server` |
| `modules/world/server_configs.go` | Modify | Add `MesanimType`/`MesanimByName`/`FontType` accessors |

---

## Task 1: MesanimType decoder + tests

**Files:**
- Create: `pkg/objtype/mesanimtype.go`
- Create: `pkg/objtype/mesanimtype_test.go`

- [ ] **Step 1.1: Write the failing decoder test**

Create `pkg/objtype/mesanimtype_test.go`:

```go
package objtype

import (
	"os"
	"path/filepath"
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

func TestNewMesanimType_LenInitMinusOne(t *testing.T) {
	m := NewMesanimType(3)
	for i, v := range m.Len {
		if v != -1 {
			t.Errorf("Len[%d]: got %d, want -1", i, v)
		}
	}
	if m.ID != 3 {
		t.Errorf("ID: got %d, want 3", m.ID)
	}
}

// flipReader builds a writer Packet via build, then returns a reader
// Packet over the bytes written. Mirrors decodeIdk's pattern in
// idktype_test.go but exposes the raw reader for direct Decode calls.
func flipReader(build func(*packet2.Packet)) *packet2.Packet {
	w := packet2.NewPacket(nil)
	build(w)
	return packet2.NewPacket(w.Bytes())
}

func TestMesanimDecode_Code1WritesLen0(t *testing.T) {
	m := NewMesanimType(0)
	r := flipReader(func(w *packet2.Packet) { w.P2(42) })
	if err := m.Decode(1, r); err != nil {
		t.Fatalf("Decode(1): %v", err)
	}
	if m.Len[0] != 42 {
		t.Errorf("Len[0]: got %d, want 42", m.Len[0])
	}
}

func TestMesanimDecode_Code4WritesLen3(t *testing.T) {
	m := NewMesanimType(0)
	r := flipReader(func(w *packet2.Packet) { w.P2(7) })
	if err := m.Decode(4, r); err != nil {
		t.Fatalf("Decode(4): %v", err)
	}
	if m.Len[3] != 7 {
		t.Errorf("Len[3]: got %d, want 7", m.Len[3])
	}
}

func TestMesanimDecode_Code250WritesDebugName(t *testing.T) {
	m := NewMesanimType(0)
	r := flipReader(func(w *packet2.Packet) { w.PJStrLF("neutral") })
	if err := m.Decode(250, r); err != nil {
		t.Fatalf("Decode(250): %v", err)
	}
	if m.DebugName != "neutral" {
		t.Errorf("DebugName: got %q, want %q", m.DebugName, "neutral")
	}
}

func TestMesanimDecode_UnknownCodeErrors(t *testing.T) {
	m := NewMesanimType(0)
	r := packet2.NewPacket(nil)
	err := m.Decode(5, r)
	if err == nil {
		t.Fatalf("Decode(5): expected error, got nil")
	}
}

func TestLoadMesanimTypes_MissingFileEmptyRegistry(t *testing.T) {
	tmp := t.TempDir()
	cfgs, err := LoadMesanimTypes(tmp)
	if err != nil {
		t.Fatalf("LoadMesanimTypes: %v", err)
	}
	if len(cfgs.Configs) != 0 {
		t.Errorf("Configs len: got %d, want 0", len(cfgs.Configs))
	}
	if len(cfgs.ConfigNames) != 0 {
		t.Errorf("ConfigNames len: got %d, want 0", len(cfgs.ConfigNames))
	}
}

func TestLoadMesanimTypes_RealCache(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "mesanim.dat")); err != nil {
		t.Skipf("data/pack/server/mesanim.dat unavailable: %v", err)
	}
	cfgs, err := LoadMesanimTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadMesanimTypes: %v", err)
	}
	if len(cfgs.Configs) == 0 {
		t.Fatalf("real cache produced zero configs")
	}
	// At least one config should have a non-empty DebugName.
	gotName := false
	for _, c := range cfgs.Configs {
		if c != nil && c.DebugName != "" {
			gotName = true
			break
		}
	}
	if !gotName {
		t.Errorf("no config has a non-empty DebugName")
	}
	// ConfigNames map should mirror the named configs.
	for name, id := range cfgs.ConfigNames {
		if id < 0 || id >= len(cfgs.Configs) {
			t.Errorf("ConfigNames[%q] = %d: out of range", name, id)
			continue
		}
		if cfgs.Configs[id] == nil || cfgs.Configs[id].DebugName != name {
			t.Errorf("ConfigNames[%q] = %d: roundtrip mismatch", name, id)
		}
	}
}
```

- [ ] **Step 1.2: Run tests to verify they fail (build error: undefined symbol)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run Mesanim -count=1`
Expected: build failure with `undefined: NewMesanimType` / `undefined: LoadMesanimTypes`.

- [ ] **Step 1.3: Write the MesanimType implementation**

Create `pkg/objtype/mesanimtype.go`:

```go
package objtype

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// MesanimType is a single mesanim.dat config record (message-animation
// frame-length table for chathead animations). Mirrors
// Engine-TS/src/cache/config/MesanimType.ts.
//
// TS-faithful: server-side .dat only (no client-jag side).
type MesanimType struct {
	ConfigType
	Len [4]int // init -1; code N (1..4) writes Len[N-1] = G2()
}

// NewMesanimType returns a MesanimType with TS-faithful defaults
// (Len init -1, matching TS Array(4).fill(-1)).
func NewMesanimType(id int) *MesanimType {
	return &MesanimType{
		ConfigType: ConfigType{ID: id},
		Len:        [4]int{-1, -1, -1, -1},
	}
}

// Decode dispatches on the mesanim config opcode, matching TS
// MesanimType.decode at Engine-TS/src/cache/config/MesanimType.ts:62-70.
func (t *MesanimType) Decode(code uint8, dat *packet2.Packet) error {
	switch {
	case code >= 1 && code <= 4:
		t.Len[code-1] = int(dat.G2())
	case code == 250:
		t.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized mesanim config code %d", code)
	}
	return nil
}

// MesanimTypeConfigs is the parsed registry of all mesanim.dat records.
type MesanimTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*MesanimType
}

// LoadMesanimTypes parses server/mesanim.dat into a MesanimTypeConfigs
// registry. Returns an empty registry with nil error when
// server/mesanim.dat is absent (silent-on-missing, matching TS
// MesanimType.load at Engine-TS/src/cache/config/MesanimType.ts:11-17).
func LoadMesanimTypes(dir string) (*MesanimTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "mesanim.dat"), false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &MesanimTypeConfigs{ConfigNames: map[string]int{}}, nil
		}
		return nil, err
	}

	count := int(server.G2())
	configs := make([]*MesanimType, count)
	configNames := make(map[string]int, count)
	for id := range count {
		c := NewMesanimType(id)
		if err := DecodeType(server, c); err != nil {
			return nil, err
		}
		configs[id] = c
		if c.DebugName != "" {
			configNames[c.DebugName] = id
		}
	}
	return &MesanimTypeConfigs{ConfigNames: configNames, Configs: configs}, nil
}
```

- [ ] **Step 1.4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run Mesanim -count=1 -v`
Expected: 7 tests PASS (`TestNewMesanimType_LenInitMinusOne`, `TestMesanimDecode_Code1WritesLen0`, `TestMesanimDecode_Code4WritesLen3`, `TestMesanimDecode_Code250WritesDebugName`, `TestMesanimDecode_UnknownCodeErrors`, `TestLoadMesanimTypes_MissingFileEmptyRegistry`, `TestLoadMesanimTypes_RealCache`).

- [ ] **Step 1.5: Run full package suite to ensure no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -count=1`
Expected: ok across the package (pre-existing sibling tests still pass).

- [ ] **Step 1.6: Commit**

```bash
git add pkg/objtype/mesanimtype.go pkg/objtype/mesanimtype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-179 T1 — MesanimType cache loader

Server-only .dat config port mirroring Engine-TS MesanimType.ts:1-71.
Codes 1-4 write Len[code-1] as G2; code 250 writes DebugName via
GJStrLF. LoadMesanimTypes is silent-on-missing per TS load(). 7 unit
+ real-cache tests.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: FontType package + tests

**Files:**
- Create: `pkg/fonttype/fonttype.go`
- Create: `pkg/fonttype/fonttype_test.go`

- [ ] **Step 2.1: Write the failing CharLookup + Load test scaffolding**

Create `pkg/fonttype/fonttype_test.go`:

```go
package fonttype

import (
	"os"
	"path/filepath"
	"testing"
)

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
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "client", "title")); err != nil {
		t.Skipf("data/pack/client/title unavailable: %v", err)
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
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "client", "title")); err != nil {
		t.Skipf("data/pack/client/title unavailable: %v", err)
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
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "client", "title")); err != nil {
		t.Skipf("data/pack/client/title unavailable: %v", err)
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
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "client", "title")); err != nil {
		t.Skipf("data/pack/client/title unavailable: %v", err)
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
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "client", "title")); err != nil {
		t.Skipf("data/pack/client/title unavailable: %v", err)
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
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "client", "title")); err != nil {
		t.Skipf("data/pack/client/title unavailable: %v", err)
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
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "client", "title")); err != nil {
		t.Skipf("data/pack/client/title unavailable: %v", err)
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
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "client", "title")); err != nil {
		t.Skipf("data/pack/client/title unavailable: %v", err)
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
```

- [ ] **Step 2.2: Run tests to verify they fail (undefined symbols)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/fonttype/ -count=1`
Expected: build failure with `undefined: CharLookup`, `undefined: Load`.

- [ ] **Step 2.3: Write the FontType implementation**

Create `pkg/fonttype/fonttype.go`:

```go
// Package fonttype ports Engine-TS's client-side FontType
// (Engine-TS/src/cache/config/FontType.ts) — width-only, no rendering.
//
// Loaded from the client/title Jagfile as 4 fixed instances:
// id 0 = p11, 1 = p12, 2 = b12, 3 = q8.
//
// goscape retains only per-character drawWidth (the metric needed by
// the SPLIT_INIT word-wrap algorithm). Character bitmap data is read
// through to advance the file cursor and to compute drawWidth, then
// discarded — we do not render text.
package fonttype

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/jagfile"
)

// CharLookup maps an 8-bit character to its slot in the 94-glyph
// per-font drawWidth table. Mirrors FontType.ts:7-18.
var CharLookup [256]byte

// init populates CharLookup matching TS FontType static initializer
// (FontType.ts:7-18). The charset includes '£' (a multi-byte UTF-8
// rune in Go source); we iterate by RUNE (not byte) so that ASCII
// chars positioned AFTER '£' in the charset get the correct char-index
// slot — matching JS String.indexOf semantics. Byte 0xA3 alone falls
// through to the 74 fallback (matches TS for any code point not in
// the charset).
func init() {
	charset := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789!\"£$%^&*()-_=+[{]};:'@#~,<.>/?\\| ")
	for i := 0; i < 256; i++ {
		slot := byte(74)
		for j, r := range charset {
			if int(r) == i {
				slot = byte(j)
				break
			}
		}
		CharLookup[i] = slot
	}
}

// FontType is a parsed title-Jagfile font: only the width metrics are
// retained. height is the tallest glyph height (used by no goscape
// caller today but kept exported for future text-rendering uses).
type FontType struct {
	drawWidth [256]byte // per-byte advance width
	height    int
}

// Load parses dir/client/title and returns 4 FontType instances in
// id order (p11, p12, b12, q8). Mirrors FontType.ts:20-27. Returns
// nil + err if the title file or any font entry is missing.
func Load(dir string) ([]*FontType, error) {
	title, err := jagfile.LoadJagfile(filepath.Join(dir, "client", "title"))
	if err != nil {
		return nil, fmt.Errorf("load title jagfile: %w", err)
	}
	names := []string{"p11", "p12", "b12", "q8"}
	fonts := make([]*FontType, len(names))
	for i, name := range names {
		f, err := decodeFont(title, name)
		if err != nil {
			return nil, fmt.Errorf("decode font %s: %w", name, err)
		}
		fonts[i] = f
	}
	return fonts, nil
}

func decodeFont(title *jagfile.Jagfile, name string) (*FontType, error) {
	data, err := title.Read(name + ".dat")
	if err != nil {
		return nil, err
	}
	index, err := title.Read("index.dat")
	if err != nil {
		return nil, err
	}

	// FontType.ts:55-59
	index.Pos = int(data.G2()) + 4
	palCount := int(index.G1())
	if palCount > 0 {
		index.Pos += (palCount - 1) * 3
	}

	f := &FontType{}
	var charMaskWidth [94]int
	var charMaskHeight [94]int
	var charOffsetX [94]int
	var charAdvance [95]byte

	for c := 0; c < 94; c++ {
		charOffsetX[c] = int(index.G1())
		_ = index.G1() // charOffsetY — read but unused outside decode
		wi := int(index.G2())
		hi := int(index.G2())
		charMaskWidth[c] = wi
		charMaskHeight[c] = hi

		pixelOrder := index.G1()
		charMask := make([]byte, wi*hi)
		switch pixelOrder {
		case 0:
			for j := 0; j < wi*hi; j++ {
				charMask[j] = data.G1()
			}
		case 1:
			for x := 0; x < wi; x++ {
				for y := 0; y < hi; y++ {
					charMask[x+y*wi] = data.G1()
				}
			}
		}

		if hi > f.height {
			f.height = hi
		}

		charOffsetX[c] = 1
		charAdvance[c] = byte(wi + 2)

		// FontType.ts:94-102 — trim left empty column.
		space := 0
		for y := hi / 7; y < hi; y++ {
			if y*wi < len(charMask) {
				space += int(charMask[y*wi])
			}
		}
		if space <= hi/7 {
			charAdvance[c]--
			charOffsetX[c] = 0
		}

		// FontType.ts:106-113 — trim right empty column.
		space = 0
		for y := hi / 7; y < hi; y++ {
			if idx := wi + y*wi - 1; idx >= 0 && idx < len(charMask) {
				space += int(charMask[idx])
			}
		}
		if space <= hi/7 {
			charAdvance[c]--
		}
	}

	// FontType.ts:116 — space (index 94) inherits advance from charAdvance[8].
	charAdvance[94] = charAdvance[8]

	for c := 0; c < 256; c++ {
		slot := CharLookup[c]
		if int(slot) < len(charAdvance) {
			f.drawWidth[c] = charAdvance[slot]
		}
	}
	return f, nil
}

// StringWidth ports FontType.ts:123-138. Treats "@xxx@" 5-character
// run as a 4-byte forward skip (the trailing '@' is then consumed by
// the for-loop's c++).
func (f *FontType) StringWidth(s string) int {
	size := 0
	for c := 0; c < len(s); c++ {
		if s[c] == '@' && c+4 < len(s) && s[c+4] == '@' {
			c += 4
		} else {
			size += int(f.drawWidth[s[c]])
		}
	}
	return size
}

// Split ports FontType.ts:140-176. Returns a slice of lines whose
// StringWidth is ≤ maxWidth, breaking on '|' (forced) or at the
// last space boundary that fits. An empty input string returns
// [""] (TS special case at :141-144). A single word wider than
// maxWidth with no space inside it is emitted on its own line
// (default splitIndex = len(str) per TS:156-170).
func (f *FontType) Split(s string, maxWidth int) []string {
	if len(s) == 0 {
		return []string{s}
	}
	var lines []string
	for len(s) > 0 {
		w := f.StringWidth(s)
		if w <= maxWidth && !strings.ContainsRune(s, '|') {
			lines = append(lines, s)
			break
		}

		splitIndex := len(s)
		for i := 0; i < len(s); i++ {
			if s[i] == ' ' {
				if f.StringWidth(s[:i]) > maxWidth {
					break
				}
				splitIndex = i
			} else if s[i] == '|' {
				splitIndex = i
				break
			}
		}

		lines = append(lines, s[:splitIndex])
		if splitIndex+1 <= len(s) {
			s = s[splitIndex+1:]
		} else {
			s = ""
		}
	}
	return lines
}
```

- [ ] **Step 2.4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/fonttype/ -count=1 -v`
Expected: 9 tests PASS (2 CharLookup tests run always; 7 title-Jagfile tests pass when `data/pack/client/title` exists, else SKIP).

- [ ] **Step 2.5: Run full repo build to confirm no compile regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean exit.

- [ ] **Step 2.6: Commit**

```bash
git add pkg/fonttype/fonttype.go pkg/fonttype/fonttype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(fonttype): NAI-179 T2 — FontType cache loader + Split

New pkg/fonttype porting Engine-TS FontType.ts. Loads 4 instances
(p11/p12/b12/q8) from client/title Jagfile; retains per-byte drawWidth
metric only (no glyph mask retained). StringWidth treats @xxx@ inline
color escapes as 4-byte skips; Split implements the TS word-wrap
algorithm (break on '|' or at last fitting space; single-word
overflow emitted as one line). 9 unit + real-cache tests.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Configs interface extensions + production wiring

**Files:**
- Modify: `pkg/script/configs.go` (add 3 methods)
- Modify: `pkg/script/handlers_config_test.go` (mock 3 methods + fields)
- Modify: `modules/world/server.go` (load mesanim/fonts)
- Modify: `modules/world/server_configs.go` (3 accessor methods)

- [ ] **Step 3.1: Extend the `Configs` interface**

Edit `pkg/script/configs.go`: add three method signatures inside the `Configs` interface block (after the existing `ObjByName` line, before the closing `}`):

```go
	// MesanimType returns the message-animation frame table for id, or
	// nil when out of range / unloaded. NAI-179 (retires
	// NAI-75-D-MESANIM-NOT-PORTED).
	MesanimType(id int) *objtype.MesanimType

	// MesanimByName resolves a debugname to a MesanimType id (TS
	// MesanimType.getId). Returns -1 on miss. NAI-179.
	MesanimByName(name string) int

	// FontType returns the per-byte width table for font id 0..3
	// (p11/p12/b12/q8 per TS FontType.load). Returns nil on out-of-range.
	// NAI-179 (retires NAI-75-D-FONT-WRAP-NAIVE).
	FontType(id int) *fonttype.FontType
```

And add the import line at the top of `configs.go`:

```go
import (
	"github.com/zsrv/goscape/pkg/fonttype"
	"github.com/zsrv/goscape/pkg/objtype"
)
```

- [ ] **Step 3.2: Add mockConfigs fields and methods (red-phase build break of consumers)**

Edit `pkg/script/handlers_config_test.go`:

Add fields to the struct (between `seqs` and the closing `}` at line ~25):

```go
	mesanims       map[int]*objtype.MesanimType
	mesanimsByName map[string]int
	fonts          map[int]*fonttype.FontType
```

Add methods after `SeqType` (around line ~36):

```go
func (m *mockConfigs) MesanimType(id int) *objtype.MesanimType {
	return m.mesanims[id]
}
func (m *mockConfigs) MesanimByName(name string) int {
	if m.mesanimsByName == nil {
		return -1
	}
	if id, ok := m.mesanimsByName[name]; ok {
		return id
	}
	return -1
}
func (m *mockConfigs) FontType(id int) *fonttype.FontType {
	return m.fonts[id]
}
```

Add the import:

```go
"github.com/zsrv/goscape/pkg/fonttype"
```

- [ ] **Step 3.3: Verify the mock + interface compile (no production wiring yet)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...`
Expected: clean build (mockConfigs now satisfies the extended interface).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: build failure in `modules/world` because `serverConfigsView` doesn't yet implement the three new methods.

- [ ] **Step 3.4: Add field + loader calls in `modules/world/server.go`**

Locate the `Server` struct definition (`grep -n "type Server struct" modules/world/server.go`). Add two fields alongside the existing `*types` fields (look for `idkTypes *objtype.IdkTypeConfigs` and add immediately after):

```go
	mesanimTypes *objtype.MesanimTypeConfigs
	fontTypes    []*fonttype.FontType
```

Add the import at the top of `server.go`:

```go
"github.com/zsrv/goscape/pkg/fonttype"
```

In `NewServer`, add two loader calls. Locate the existing `idkTypes, err := objtype.LoadIdkTypes(cfg.CachePath)` block at line ~278 and append two new blocks immediately after `s.idkTypes = idkTypes`:

```go
	mesanimTypes, err := objtype.LoadMesanimTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load mesanim types: %w", err)
	}
	s.mesanimTypes = mesanimTypes

	fontTypes, err := fonttype.Load(cfg.CachePath)
	if err != nil {
		// Title file is optional in test fixtures; treat NotFound as
		// empty registry but propagate any other error.
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load font types: %w", err)
		}
		s.log.Warn("client/title font cache unavailable; font split disabled", "err", err)
		fontTypes = nil
	}
	s.fontTypes = fontTypes
```

Verify the `errors` and `os` imports are present in `server.go` (`grep '"errors"\|"os"' modules/world/server.go`); add if missing.

- [ ] **Step 3.5: Add the three accessor methods to `serverConfigsView`**

Edit `modules/world/server_configs.go`. After the existing `ObjByName` method at line ~181, append:

```go
// MesanimType returns the message-animation config for id or nil when
// out of range. NAI-179.
func (c serverConfigsView) MesanimType(id int) *objtype.MesanimType {
	if c.s.mesanimTypes == nil || id < 0 || id >= len(c.s.mesanimTypes.Configs) {
		return nil
	}
	return c.s.mesanimTypes.Configs[id]
}

// MesanimByName resolves a mesanim debugname to its config id, or -1.
// Mirrors TS MesanimType.getId. NAI-179.
func (c serverConfigsView) MesanimByName(name string) int {
	if c.s.mesanimTypes == nil {
		return -1
	}
	if id, ok := c.s.mesanimTypes.ConfigNames[name]; ok {
		return id
	}
	return -1
}

// FontType returns the per-byte width table for font id 0..3, or nil
// when the title cache wasn't loaded / id is out of range. NAI-179.
func (c serverConfigsView) FontType(id int) *fonttype.FontType {
	if id < 0 || id >= len(c.s.fontTypes) {
		return nil
	}
	return c.s.fontTypes[id]
}
```

Add the import at the top of `server_configs.go`:

```go
"github.com/zsrv/goscape/pkg/fonttype"
```

- [ ] **Step 3.6: Verify full repo compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean build.

- [ ] **Step 3.7: Run all tests to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: all existing tests pass; no new failures (handler tests still pass because mockConfigs returns nil FontType → invalid-fontId branch in `handleSplitInit` doesn't exist yet, so current behaviour is unaffected).

- [ ] **Step 3.8: Commit**

```bash
git add pkg/script/configs.go pkg/script/handlers_config_test.go modules/world/server.go modules/world/server_configs.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script,world): NAI-179 T3 — Configs interface + production wiring

Adds MesanimType/MesanimByName/FontType to pkg/script.Configs; wires
both loaders in modules/world.NewServer; serverConfigsView returns
nil/-1 on miss matching TS get-or-default semantics. FontType load is
NotFound-tolerant (test fixtures may omit client/title).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: handleSplitInit / handleSplitGetAnim rewrites + test updates

**Files:**
- Modify: `pkg/script/handlers_string.go` (rewrite both handlers + doc-comments)
- Modify: `pkg/script/handlers_string_test.go` (update existing pins, add 6 new tests)
- Modify: `pkg/script/state.go` (retire NAI-75-D-MESANIM-NOT-PORTED doc-comment at line 405-410)

- [ ] **Step 4.1: Update existing handler tests to inject Configs**

Edit `pkg/script/handlers_string_test.go`. Modify the `runSplitInit` helper at line ~109 to attach Configs:

```go
func runSplitInit(t *testing.T, text string, maxWidth, linesPerPage, fontId int) *ScriptState {
	t.Helper()
	return runSplitInitWithConfigs(t, newTestConfigs(), text, maxWidth, linesPerPage, fontId)
}

// runSplitInitWithConfigs is the same as runSplitInit but lets callers
// supply a pre-seeded mockConfigs (e.g. with a fake FontType or a
// mesanim debugname → id mapping).
func runSplitInitWithConfigs(t *testing.T, cfg *mockConfigs, text string, maxWidth, linesPerPage, fontId int) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_split_init",
		Opcodes:          []Opcode{OpSplitInit, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.Configs = cfg
	state.PushString(text)
	state.PushInt(maxWidth)
	state.PushInt(linesPerPage)
	state.PushInt(fontId)
	if err := Execute(state); err != nil {
		t.Fatalf("SPLIT_INIT: unexpected error: %v", err)
	}
	return state
}
```

Apply the same change to `runSplitInitThen` at line ~237 — attach `state.Configs = newTestConfigs()` after `state := Init(...)`.

Also update `TestSplitInitReplacesNotAppends` at line ~172: after `state := Init(...)`, insert `state.Configs = newTestConfigs()`.

- [ ] **Step 4.2: Update the existing mesanim-pin test**

Replace `TestSplitInitMesanimPrefixStripped` at line ~142:

```go
func TestSplitInitMesanimPrefixStripped_UnknownNameStaysNegOne(t *testing.T) {
	// mockConfigs has no entry for "neutral" → MesanimByName returns -1.
	// (The previous NAI-75-D-MESANIM-NOT-PORTED pin held the constant
	// -1 unconditionally; retired in NAI-179.)
	s := runSplitInit(t, "<p,neutral>Greetings|stranger", 380, 4, 8)
	if s.SplitMesanim != -1 {
		t.Errorf("SplitMesanim: got %d, want -1 (name not in registry)", s.SplitMesanim)
	}
	if len(s.SplitPages) != 1 {
		t.Fatalf("len(SplitPages): got %d, want 1", len(s.SplitPages))
	}
	// Prefix stripped: text is "Greetings|stranger" → mockConfigs.FontType
	// returns nil → defensive fallback uses strings.Split(text, "|") → 2 lines.
	if got, want := s.SplitPages[0], []string{"Greetings", "stranger"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v (prefix should be stripped)", got, want)
	}
}
```

- [ ] **Step 4.3: Add the new mesanim-resolution test**

Append to `handlers_string_test.go`:

```go
func TestSplitInitMesanimPrefixResolves(t *testing.T) {
	// Seed mockConfigs with a fake MesanimType named "neutral" at id 7.
	cfg := newTestConfigs()
	cfg.mesanimsByName = map[string]int{"neutral": 7}
	cfg.mesanims = map[int]*objtype.MesanimType{
		7: {Len: [4]int{10, 20, 30, 40}},
	}
	s := runSplitInitWithConfigs(t, cfg, "<p,neutral>hi|there", 380, 4, 8)
	if s.SplitMesanim != 7 {
		t.Errorf("SplitMesanim: got %d, want 7 (resolved id)", s.SplitMesanim)
	}
	// Text stripped: "hi|there"; fontType nil → '|' fallback path.
	if len(s.SplitPages) != 1 {
		t.Fatalf("len(SplitPages): got %d, want 1", len(s.SplitPages))
	}
	if got, want := s.SplitPages[0], []string{"hi", "there"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v", got, want)
	}
}
```

Note: the inline `MesanimType{Len:[...]}` literal works because `MesanimType.ConfigType` is zero-valued. Confirm field reachability via `grep -n "type MesanimType" pkg/objtype/mesanimtype.go`.

- [ ] **Step 4.4: Add the font-wrap test (uses real cache; skips on missing)**

Append:

```go
func TestSplitInitFontWrap_BreaksOnMaxWidth(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "..", "data", "pack", "client", "title")); err != nil {
		t.Skipf("data/pack/client/title unavailable: %v", err)
	}
	fonts, err := fonttype.Load(filepath.Join("..", "..", "data", "pack"))
	if err != nil {
		t.Fatalf("fonttype.Load: %v", err)
	}
	cfg := newTestConfigs()
	cfg.fonts = map[int]*fonttype.FontType{0: fonts[0]}
	// Long ASCII sentence, no '|', tight maxWidth → forces ≥ 1 wrap.
	text := "alpha bravo charlie delta echo foxtrot golf hotel india juliet"
	maxWidth := fonts[0].StringWidth(text) / 3
	s := runSplitInitWithConfigs(t, cfg, text, maxWidth, 100, 0)
	if len(s.SplitPages) != 1 {
		t.Fatalf("len(SplitPages): got %d, want 1", len(s.SplitPages))
	}
	if len(s.SplitPages[0]) < 2 {
		t.Errorf("font.Split should have produced ≥2 lines with maxWidth=%d; got %v",
			maxWidth, s.SplitPages[0])
	}
}
```

Add imports at top of `handlers_string_test.go` if missing:

```go
"os"
"path/filepath"
"github.com/zsrv/goscape/pkg/fonttype"
```

- [ ] **Step 4.5: Add the invalid-fontId defensive-fallback test**

Append:

```go
func TestSplitInitInvalidFontFallsBackToPipeSplit(t *testing.T) {
	cfg := newTestConfigs()
	// cfg.fonts is nil → mockConfigs.FontType returns nil for any id.
	s := runSplitInitWithConfigs(t, cfg, "a|b", 380, 1, 999)
	// Defensive fallback splits on '|'; linesPerPage=1 → 2 pages of 1 line.
	if len(s.SplitPages) != 2 {
		t.Fatalf("len(SplitPages): got %d, want 2", len(s.SplitPages))
	}
	if got, want := s.SplitPages[0], []string{"a"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v", got, want)
	}
	if got, want := s.SplitPages[1], []string{"b"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[1]: got %v, want %v", got, want)
	}
}
```

- [ ] **Step 4.6: Replace the existing SPLIT_GETANIM pin tests**

Find and replace the existing SPLIT_GETANIM test at line ~325-336. Replace the whole block with:

```go
func TestSplitGetAnim_ResolvesLen(t *testing.T) {
	// Seed: MesanimType id 3 with Len[0]=10, Len[1]=20.
	cfg := newTestConfigs()
	cfg.mesanims = map[int]*objtype.MesanimType{
		3: {Len: [4]int{10, 20, 30, 40}},
	}
	cfg.mesanimsByName = map[string]int{"shopkeeper": 3}
	// SPLIT_INIT writes SplitMesanim=3 and SplitPages with 2 lines on page 0.
	s := runSplitInitThenWithConfigs(t, cfg, "<p,shopkeeper>hello|world", 4, OpSplitGetAnim, []int{0})
	got := s.PopInt()
	// Page 0 has 2 lines → Len[lineCount-1] = Len[1] = 20.
	if got != 20 {
		t.Errorf("SPLIT_GETANIM(0): got %d, want 20 (Len[1])", got)
	}
}

func TestSplitGetAnim_NoMesanimReturnsNegOne(t *testing.T) {
	s := runSplitInitThen(t, "no prefix here", 4, OpSplitGetAnim, []int{0})
	got := s.PopInt()
	if got != -1 {
		t.Errorf("SPLIT_GETANIM with SplitMesanim=-1: got %d, want -1", got)
	}
}

func TestSplitGetAnim_NilConfigTypeReturnsNegOne(t *testing.T) {
	// SplitMesanim is set to a non-negative id, but mockConfigs.MesanimType
	// returns nil for it → defensive -1 (TS MesanimValid would throw).
	cfg := newTestConfigs()
	cfg.mesanimsByName = map[string]int{"ghost": 42}
	// mesanims map is empty → MesanimType(42) returns nil.
	s := runSplitInitThenWithConfigs(t, cfg, "<p,ghost>hello", 4, OpSplitGetAnim, []int{0})
	got := s.PopInt()
	if got != -1 {
		t.Errorf("SPLIT_GETANIM with nil MesanimType: got %d, want -1", got)
	}
}
```

Add the `runSplitInitThenWithConfigs` helper alongside `runSplitInitThen`:

```go
func runSplitInitThenWithConfigs(t *testing.T, cfg *mockConfigs, initText string, linesPerPage int, follow Opcode, followInts []int) *ScriptState {
	t.Helper()
	ops := []Opcode{OpSplitInit, follow, OpReturn}
	sf := &ScriptFile{
		Name:             "test_split_init_then_" + follow.String(),
		Opcodes:          ops,
		IntOperands:      make([]int32, len(ops)),
		StringOperands:   make([]string, len(ops)),
		InstructionCount: uint32(len(ops)),
	}
	state := Init(sf, nil, false, nil, nil)
	state.Configs = cfg
	for _, v := range followInts {
		state.PushInt(v)
	}
	state.PushString(initText)
	state.PushInt(380)
	state.PushInt(linesPerPage)
	state.PushInt(8)
	if err := Execute(state); err != nil {
		t.Fatalf("SPLIT_INIT+%s: unexpected error: %v", follow.String(), err)
	}
	return state
}
```

- [ ] **Step 4.7: Run the failing tests to confirm red phase**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "SplitInit|SplitGetAnim" -count=1 -v`
Expected: build OK; the new mesanim-resolves and font-wrap tests FAIL (because `handleSplitInit` still doesn't call `MesanimByName` or `font.Split`); the existing renamed tests pass.

- [ ] **Step 4.8: Rewrite `handleSplitInit`**

Replace the entire `handleSplitInit` function and its preceding comment block in `pkg/script/handlers_string.go` (lines 97-150):

```go
// -- SPLIT_* dialog pagination handlers (NAI-75 light-fidelity port;
// NAI-179 wired font.Split + MesanimType resolution).

// handleSplitInit ports TS SPLIT_INIT (StringOps.ts:76-96). Pops
// (text, maxWidth, linesPerPage, fontId), parses any leading <p,name>
// mesanim prefix (resolving NAME to a MesanimType id via
// Configs.MesanimByName), splits the prefix-stripped text into lines
// via FontType.Split (width-aware word-wrap), and chunks the lines
// into pages of linesPerPage each.
//
// On invalid fontId (Configs.FontType returns nil — TS FontTypeValid
// would throw) the handler logs slog.Warn and falls back to the
// NAI-75 light-fidelity '|'-only split. Goscape defensive per
// defensive_gate_doc_comment_label.md.
func handleSplitInit(s *ScriptState) error {
	fontId := s.PopInt()
	linesPerPage := s.PopInt()
	maxWidth := s.PopInt()
	text := s.PopString()

	s.SplitMesanim = -1
	if strings.HasPrefix(text, "<p,") {
		if end := strings.IndexByte(text, '>'); end != -1 {
			name := text[3:end]
			s.SplitMesanim = s.Configs.MesanimByName(name)
			text = text[end+1:]
		}
	}

	var lines []string
	if font := s.Configs.FontType(fontId); font != nil {
		lines = font.Split(text, maxWidth)
	} else {
		slog.Warn("SPLIT_INIT: invalid fontId; falling back to '|' split",
			"script", s.Script.Name, "fontId", fontId)
		lines = strings.Split(text, "|")
	}

	if linesPerPage < 1 {
		// Defensive: TS would divide-by-zero on splice(0, 0). Goscape
		// defensive (TS throws).
		s.SplitPages = [][]string{lines}
		return nil
	}
	pages := make([][]string, 0, (len(lines)+linesPerPage-1)/linesPerPage)
	for i := 0; i < len(lines); i += linesPerPage {
		end := i + linesPerPage
		if end > len(lines) {
			end = len(lines)
		}
		pages = append(pages, lines[i:end])
	}
	s.SplitPages = pages
	slog.Debug("SPLIT_INIT processed",
		"script", s.Script.Name, "pages", len(pages), "mesanim", s.SplitMesanim)
	return nil
}
```

- [ ] **Step 4.9: Rewrite `handleSplitGetAnim`**

Replace the existing `handleSplitGetAnim` block in `pkg/script/handlers_string.go` (lines ~176-185):

```go
// handleSplitGetAnim ports TS SPLIT_GETANIM (StringOps.ts:114-122).
// Pops page; pushes MesanimType.Len[lineCount-1] where lineCount =
// len(SplitPages[page]). When SplitMesanim is negative (no prefix),
// MesanimType lookup is nil, or any index is out-of-range, pushes -1
// (TS MesanimValid would throw; goscape defensive per
// defensive_gate_doc_comment_label.md).
func handleSplitGetAnim(s *ScriptState) error {
	page := s.PopInt()
	if s.SplitMesanim < 0 {
		s.PushInt(-1)
		return nil
	}
	typ := s.Configs.MesanimType(s.SplitMesanim)
	if typ == nil {
		s.PushInt(-1)
		return nil
	}
	if page < 0 || page >= len(s.SplitPages) {
		s.PushInt(-1)
		return nil
	}
	idx := len(s.SplitPages[page]) - 1
	if idx < 0 || idx >= len(typ.Len) {
		s.PushInt(-1)
		return nil
	}
	s.PushInt(typ.Len[idx])
	return nil
}
```

- [ ] **Step 4.10: Retire the NAI-75-D-MESANIM-NOT-PORTED doc-comment in `state.go`**

Read `pkg/script/state.go` around line 405-410 first to get the exact text. Replace the block that mentions `NAI-75-D-MESANIM-NOT-PORTED` and "unconditionally" with a NAI-179-aware doc-comment, e.g.:

```go
	// SplitMesanim is the MesanimType id parsed from a leading <p,name>
	// prefix on a SPLIT_INIT text argument. -1 when no prefix or when
	// the name does not resolve to a known MesanimType (NAI-179 retired
	// the unconditional -1 of NAI-75-D-MESANIM-NOT-PORTED).
	SplitMesanim int32
```

If the exact field declaration / type differs (`int32` vs `int`), preserve the original type and only rewrite the comment.

- [ ] **Step 4.11: Run all SPLIT_* tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "SplitInit|SplitGet|SplitPageCount|SplitLineCount|SplitGetAnim" -count=1 -v`
Expected: all SPLIT-prefixed tests PASS.

- [ ] **Step 4.12: Run full repo test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: clean (zero new failures across pkg/ and modules/).

- [ ] **Step 4.13: Confirm no NAI-75-D tag references remain**

Run: `rg "NAI-75-D-(MESANIM|FONT-WRAP)" pkg/ modules/`
Expected: zero hits.

- [ ] **Step 4.14: Commit**

```bash
git add pkg/script/handlers_string.go pkg/script/handlers_string_test.go pkg/script/state.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-179 T4 — SPLIT_INIT/GETANIM wired to MesanimType+FontType

handleSplitInit now resolves <p,name> via Configs.MesanimByName and
splits via FontType.Split when available; falls back to '|'-only
split with slog.Warn on invalid fontId (TS FontTypeValid would
throw — goscape defensive). handleSplitGetAnim reads
MesanimType.Len[lineCount-1] with full defensive ladder for missing
prefix / nil type / out-of-range page.

Retires NAI-75-D-MESANIM-NOT-PORTED and NAI-75-D-FONT-WRAP-NAIVE
(zero remaining tag references across pkg/ and modules/).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Smoke handoff + close

**Files:** none (run-only / commit-only).

- [ ] **Step 5.1: Final build + race-detector test pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1`
Expected: clean.

- [ ] **Step 5.2: Final NAI-75-D tag absence grep**

Run: `rg "NAI-75-D" pkg/ modules/ cmd/`
Expected: zero hits.

- [ ] **Step 5.3: Hand off to user for smoke**

Per `smoke_test_server_handoff.md`: prompt the user to launch the server (sandbox cannot run it from the harness) and re-run the Tutorial Island chatnpc flow.

Pass criteria:
- Long chatnpc lines without explicit `|` no longer overflow horizontally on the dialog component.
- Chathead anim plays during `~chatnpc` / `~chatnpcrange` / `~chatplayer` dialogs (head talks instead of staying static).

Failure routing:
- If overflow persists: inspect `slog.Warn` output for "invalid fontId" — indicates the title cache didn't load. Investigate.
- If chathead is still static: confirm `~chatnpc` script passes a `<p,…>` prefix; if so, check `mesanim.dat` actually has the named entry (run `go test ./pkg/objtype/ -run RealCache -v`).

- [ ] **Step 5.4: Close commit + memory update**

After user confirms smoke pass, write a close commit. Update `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` with a new top-level section for NAI-179 documenting:
- Scope retired (both NAI-75-D-* deviations).
- Production behaviour delta (font-aware word-wrap + chathead anim).
- Commits chronological (T1, T2, T3, T4, close).
- Net deviation tally delta (N → N-2).
- Lessons confirmed / surfaced.
- `Closes memory:` trailer in the commit body.

Sample close commit:

```bash
git add ~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-179 — SPLIT_* cache-loader port (MesanimType + FontType)

Retires NAI-75-D-MESANIM-NOT-PORTED and NAI-75-D-FONT-WRAP-NAIVE
via MesanimType + new pkg/fonttype cache loaders. handleSplitInit
gains <p,name> resolution + font.Split word-wrap; handleSplitGetAnim
reads MesanimType.Len[lineCount-1]. User-confirmed smoke: chatnpc
overflow gone; chathead anims play.

Closes memory: NAI-75-D-MESANIM-NOT-PORTED, NAI-75-D-FONT-WRAP-NAIVE.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Notes

- **Spec coverage:** §5.1 → T1; §5.2 → T2; §5.3 → T3.1+T3.2+T3.5; §5.4 → T3.4; §5.5 → T4.8; §5.6 → T4.9; §7 → T1.1, T2.1, T4.2-4.6; §10 retirement → T4.13.
- **Placeholder scan:** none. All "..." patterns inside spec code blocks are expanded as full bodies in the corresponding task.
- **Type consistency:** `MesanimType.Len` is `[4]int` (declared T1.3, consumed T4.9, mocked T4.3); `FontType` is `*fonttype.FontType` everywhere; `Configs.MesanimByName` returns `int` (mocked T3.2, called T4.8); `SplitMesanim` retains its existing `int32` field type in `state.go` (preserved at T4.10).
- **Imports:** new `pkg/fonttype` imported in `pkg/script/configs.go`, `pkg/script/handlers_config_test.go`, `pkg/script/handlers_string_test.go`, `modules/world/server.go`, `modules/world/server_configs.go` — all spelled out in their respective steps.
