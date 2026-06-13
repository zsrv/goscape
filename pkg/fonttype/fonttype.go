// Package fonttype ports Engine-TS's client-side FontType
// (Engine-TS/src/cache/config/FontType.ts @dee467c8) — width-only, no
// rendering.
//
// Loaded from the client/title Jagfile as 4 fixed instances by their
// rev-274 *_full names:
//
//	id 0 = p11_full, 1 = p12_full, 2 = b12_full, 3 = q8_full
//
// q8_full is the only "quill" font (its space-advance copies the 'I'
// glyph; the others copy the 'i' glyph).
//
// goscape retains only per-character drawWidth (the metric needed by the
// SPLIT_INIT word-wrap algorithm). Character bitmap data is read through
// to advance the file cursor and to compute drawWidth, then discarded —
// we do not render text. This is a deliberately SIMPLIFIED port: the TS
// charMask/charMaskWidth/charMaskHeight/charOffsetX/charOffsetY rendering
// arrays are not retained, only the observable charAdvance (drawWidth)
// per char code.
//
// rev-274 reworked FontType to 256-glyph fonts indexed DIRECTLY by char
// code (the old CHAR_LOOKUP 94-glyph indirection is gone), so drawWidth
// now carries a real advance for every code 0..255.
package fonttype

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/jagfile"
)

// FontType is a parsed title-Jagfile font: only the width metrics are
// retained. height is the tallest glyph height among codes < 128 (the
// rev-274 `c < 128` guard) — kept exported via Height() for future
// text-rendering uses.
type FontType struct {
	drawWidth [256]byte // per-char-code advance width (charAdvance)
	height    int
}

// fontSpec pairs a *_full font name with its quill flag. Mirrors TS
// FontType.load (FontType.ts:6-13): only q8_full is a quill font.
type fontSpec struct {
	name  string
	quill bool
}

var fontSpecs = []fontSpec{
	{"p11_full", false},
	{"p12_full", false},
	{"b12_full", false},
	{"q8_full", true},
}

// Load parses dir/client/title and returns 4 FontType instances in id
// order (p11_full, p12_full, b12_full, q8_full). Mirrors TS
// FontType.load. Returns nil + err if the title file or any font entry
// is missing.
func Load(dir string) ([]*FontType, error) {
	title, err := jagfile.LoadJagfile(filepath.Join(dir, "client", "title"))
	if err != nil {
		return nil, fmt.Errorf("load title jagfile: %w", err)
	}
	fonts := make([]*FontType, len(fontSpecs))
	for i, spec := range fontSpecs {
		f, err := decodeFont(title, spec.name, spec.quill)
		if err != nil {
			return nil, fmt.Errorf("decode font %s: %w", spec.name, err)
		}
		fonts[i] = f
	}
	return fonts, nil
}

// decodeFont ports the TS FontType constructor (FontType.ts:33-106) in
// simplified form: it walks all 256 glyphs to compute charAdvance
// (kept as drawWidth) and the height, discarding the pixel masks after
// the advance computation reads them.
func decodeFont(title *jagfile.Jagfile, name string, quill bool) (*FontType, error) {
	// FontType.ts:34-38 — a missing data or index entry is a SILENT no-op:
	// TS returns from the constructor leaving an all-zero FontType. We
	// mirror that (empty FontType, no error) so a cache that lacks a given
	// font — e.g. a pre-274 title jag without the *_full entries — degrades
	// to zero-width text rather than failing the whole load.
	data, err := title.Read(name + ".dat")
	if err != nil {
		if errors.Is(err, jagfile.ErrFileNotFound) {
			return &FontType{}, nil
		}
		return nil, err
	}
	index, err := title.Read("index.dat")
	if err != nil {
		if errors.Is(err, jagfile.ErrFileNotFound) {
			return &FontType{}, nil
		}
		return nil, err
	}

	// FontType.ts:40-44
	index.Pos = int(data.G2()) + 4
	palCount := int(index.G1())
	if palCount > 0 {
		index.Pos += (palCount - 1) * 3
	}

	f := &FontType{}

	// FontType.ts:46-99 — all 256 glyphs, direct char-code index.
	for c := 0; c < 256; c++ {
		_ = index.G1() // charOffsetX — read but unused in this width-only port
		_ = index.G1() // charOffsetY — likewise
		wi := int(index.G2())
		hi := int(index.G2())

		pixelOrder := index.G1()
		mask := make([]byte, wi*hi)
		switch pixelOrder {
		case 0:
			for j := 0; j < wi*hi; j++ {
				mask[j] = data.G1()
			}
		case 1:
			for x := 0; x < wi; x++ {
				for y := 0; y < hi; y++ {
					mask[x+y*wi] = data.G1()
				}
			}
		}

		// FontType.ts:70-72 — height trusted only from glyphs < 128.
		if hi > f.height && c < 128 {
			f.height = hi
		}

		// FontType.ts:75 — base advance is glyph width + 2.
		adv := wi + 2

		// FontType.ts:79-87 — trim left empty column.
		// NOTE: the len(mask) bounds guards below intentionally diverge from
		// TS for a degenerate zero-area glyph (wi*hi==0): TS reads undefined →
		// NaN → no decrement, whereas the guarded Go path leaves space==0 →
		// decrement. Moot for the real 274 fonts (no empty glyphs; drawWidth
		// verified against the live cache) — the guards are panic-safety, not
		// a faithful transcription of the empty-glyph corner.
		space := 0
		for y := hi / 7; y < hi; y++ {
			if y*wi < len(mask) {
				space += int(mask[y*wi])
			}
		}
		if space <= hi/7 {
			adv--
		}

		// FontType.ts:91-98 — trim right empty column.
		space = 0
		for y := hi / 7; y < hi; y++ {
			if idx := wi + y*wi - 1; idx >= 0 && idx < len(mask) {
				space += int(mask[idx])
			}
		}
		if space <= hi/7 {
			adv--
		}

		f.drawWidth[c] = byte(adv)
	}

	// FontType.ts:101-105 — space (code 32) advance: quill copies glyph 73
	// (the 'I' glyph), else glyph 105 (the 'i' glyph).
	if quill {
		f.drawWidth[32] = f.drawWidth[73]
	} else {
		f.drawWidth[32] = f.drawWidth[105]
	}

	return f, nil
}

// DrawWidth returns the advance width for a single char code.
func (f *FontType) DrawWidth(c byte) byte { return f.drawWidth[c] }

// Height returns the font height (tallest glyph among codes < 128).
func (f *FontType) Height() int { return f.height }

// StringWidth ports FontType.ts:108-123. Treats "@xxx@" 5-character run
// as a 4-byte forward skip (the trailing '@' is then consumed by the
// for-loop's c++).
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

// Split ports FontType.ts:125-196. Returns a slice of lines whose
// StringWidth is ≤ maxWidth, breaking on '|' (forced) or at the last
// space boundary that fits. An empty input string returns [""] (TS
// special case at :126-129). A single word wider than maxWidth with no
// space inside it is emitted on its own line (default splitIndex =
// len(str) per TS:142-156).
//
// rev-274 added colour persistence: an "@xxx@" colour code opened on one
// line is re-applied to the start of the next line; an "@str@" reset
// clears the carry. This is FontType's OWN split loop (TS does not share
// it with the IF_SETTEXT handler), ported verbatim.
func (f *FontType) Split(s string, maxWidth int) []string {
	if len(s) == 0 {
		return []string{s}
	}
	var lines []string
	var savedCol string // "" == null (no carried colour)
	for len(s) > 0 {
		// FontType.ts:135-139 — does the line even need breaking?
		w := f.StringWidth(s)
		if w <= maxWidth && !strings.ContainsRune(s, '|') {
			lines = append(lines, s)
			break
		}

		// FontType.ts:142-156 — find the next word boundary.
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

		line := s[:splitIndex]
		lines = append(lines, line)

		// FontType.ts:162-178 — scan the emitted line for the last colour
		// code, updating savedCol. "@str@" resets; "@str@bla@" stays reset.
		if strings.IndexByte(line, '@') != -1 {
			for i := 0; i+4 < len(line); i++ {
				if line[i] == '@' && i+4 < len(line) && line[i+4] == '@' {
					col := line[i+1 : i+4]
					if col == "str" {
						savedCol = ""
						if i+10 <= len(line) && line[i+5:i+10] == "@bla@" {
							i += 9
							continue
						}
					} else {
						savedCol = line[i : i+5]
					}
					i += 4
				}
			}
		}

		// FontType.ts:180 — advance past the break char.
		if splitIndex+1 <= len(s) {
			s = s[splitIndex+1:]
		} else {
			s = ""
		}

		// FontType.ts:182-193 — re-apply the carried colour to the
		// continuation (unless it starts with '|' or is empty). If the
		// continuation already contains an "@str@", insert "@bla@" after
		// it instead of prefixing, then clear the carry.
		if savedCol != "" && len(s) > 0 && s[0] != '|' {
			if strIndex := strings.Index(s, "@str@"); strIndex != -1 {
				if !(strIndex+10 <= len(s) && s[strIndex+5:strIndex+10] == "@bla@") {
					s = s[:strIndex+5] + "@bla@" + s[strIndex+5:]
				}
				savedCol = ""
			} else {
				s = savedCol + s
			}
		}
	}
	return lines
}
