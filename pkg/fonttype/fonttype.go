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
