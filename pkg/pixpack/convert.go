package pixpack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// ConvertImage reads <srcDir>/<name>.png, optionally reads
// <srcDir>/meta/<name>.opt for spritesheet metadata, encodes the
// image into the RS sprite format, and appends frame headers to index.
// Returns the per-sprite payload Packet (caller must Release).
//
// Ports TS PixPack.ts:136-214.
func ConvertImage(index *packet.Packet, srcDir, name string) (*packet.Packet, error) {
	data := packet.Alloc(4)
	// TS: data.p2(index.pos) — record the current write-position of
	// the shared index buffer (where this image's headers will begin)
	// at the head of this image's payload.
	data.P2(uint16(index.Length()))

	img, err := decodePNG(filepath.Join(srcDir, name+".png"))
	if err != nil {
		data.Release()
		return nil, err
	}

	tileX := img.Width
	tileY := img.Height

	sprites, tileX2, tileY2, err := loadSpriteMeta(srcDir, name, tileX, tileY)
	if err != nil {
		data.Release()
		return nil, fmt.Errorf("ConvertImage(%q): %w", name, err)
	}
	tileX, tileY = tileX2, tileY2

	index.P2(uint16(tileX))
	index.P2(uint16(tileY))

	// TS PixPack.ts:185-192 (9aadcec4): if <srcDir>/meta/<name>.pal.png exists,
	// read its palette as a CRC-preserving workaround; otherwise use the source image.
	var colors []int32
	palPath := filepath.Join(srcDir, "meta", name+".pal.png")
	if _, err := os.Stat(palPath); err == nil {
		palImg, err := decodePNG(palPath)
		if err != nil {
			data.Release()
			return nil, fmt.Errorf("ConvertImage(%q): read pal.png: %w", name, err)
		}
		colors = generatePalette(palImg)
	} else {
		colors = generatePalette(img)
	}
	if len(colors) > 255 {
		// NAI-213-D-PIXPACK-QUANTIZE-MISSING: TS calls img.quantize({ colors: 255 });
		// goscape stdlib has no equivalent — surface error instead of silent truncate.
		data.Release()
		return nil, fmt.Errorf("ConvertImage(%q): palette size %d > 255 and stdlib quantize not implemented", name, len(colors))
	}

	index.P1(uint8(len(colors)))
	for j := 1; j < len(colors); j++ {
		index.P3(uint32(colors[j]))
	}

	switch {
	case len(sprites) > 1:
		for y := 0; y < img.Height/tileY; y++ {
			for x := 0; x < img.Width/tileX; x++ {
				tile := cropBitmap(img, x*tileX, y*tileY, tileX, tileY)
				WriteImage(tile, data, index, colors, &sprites[x+y*(img.Width/tileX)])
			}
		}
	case len(sprites) == 1:
		WriteImage(img, data, index, colors, &sprites[0])
	default:
		// TS passes sprites[0] which is undefined; writeImage's meta
		// param defaults to null. Go: pass nil.
		WriteImage(img, data, index, colors, nil)
	}

	return data, nil
}

// loadSpriteMeta parses <srcDir>/meta/<name>.opt if present.
//
// Two formats:
//
//	single sprite:  "x,y,w,h,row|col"
//	tiled sheet:    "<tileX>x<tileY>\n<sprite>\n..."
//
// If the file is absent, returns (nil, defaultTileX, defaultTileY, nil).
func loadSpriteMeta(srcDir, name string, defaultTileX, defaultTileY int) ([]SpriteMeta, int, int, error) {
	path := filepath.Join(srcDir, "meta", name+".opt")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, defaultTileX, defaultTileY, nil
	}
	if err != nil {
		return nil, 0, 0, err
	}
	// TS 244 PixPack.ts:147-151 (9aadcec4): .replace(/\r/g,'').split('\n')
	// — strip ALL \r before splitting so mid-line \r bytes are removed.
	lines := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r", ""), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return nil, defaultTileX, defaultTileY, nil
	}

	if !strings.Contains(lines[0], "x") {
		s, err := parseSpriteLine(lines[0])
		if err != nil {
			return nil, 0, 0, err
		}
		return []SpriteMeta{s}, defaultTileX, defaultTileY, nil
	}

	parts := strings.SplitN(lines[0], "x", 2)
	tileX, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, 0, 0, fmt.Errorf("tileX: %w", err)
	}
	tileY, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, 0, 0, fmt.Errorf("tileY: %w", err)
	}

	sprites := make([]SpriteMeta, 0, len(lines)-1)
	for _, line := range lines[1:] {
		s, err := parseSpriteLine(line)
		if err != nil {
			return nil, 0, 0, err
		}
		sprites = append(sprites, s)
	}
	return sprites, tileX, tileY, nil
}

// parseSpriteLine parses a single "x,y,w,h[,row|col]" line into a SpriteMeta.
//
// TS PixPack.ts:154-162 (9aadcec4): sprite = line.split(','); pixelOrder is
// sprite[4] === 'row' ? 1 : 0 — sprite[4] is undefined for 4-field lines
// (JS: undefined !== 'row'), so pixelOrder is 0 with no error. Real 244
// content (e.g. title/meta/runes.opt) has 4-field lines.
//
// Go mirrors: >=4 fields accepted; field 5 absent or != "row" → pixelOrder 0;
// == "row" → pixelOrder 1. <4 fields remain an error — TS does not guard
// (parseInt(undefined)=NaN), so the defensive error is a Go deviation
// (documented below). This applies to BOTH the single-sprite and tiled-sheet
// parse paths in loadSpriteMeta (both call parseSpriteLine).
//
// PORTING-EXCEPTION: <4-field lines return an error in Go where TS would
// silently use NaN x/y/w/h. Go keeps the guard as a defensive deviation.
func parseSpriteLine(line string) (SpriteMeta, error) {
	parts := strings.Split(line, ",")
	if len(parts) < 4 {
		return SpriteMeta{}, fmt.Errorf("sprite line %q: want at least 4 fields, got %d", line, len(parts))
	}
	x, err := strconv.Atoi(parts[0])
	if err != nil {
		return SpriteMeta{}, err
	}
	y, err := strconv.Atoi(parts[1])
	if err != nil {
		return SpriteMeta{}, err
	}
	w, err := strconv.Atoi(parts[2])
	if err != nil {
		return SpriteMeta{}, err
	}
	h, err := strconv.Atoi(parts[3])
	if err != nil {
		return SpriteMeta{}, err
	}
	order := 0
	if len(parts) >= 5 && parts[4] == "row" {
		order = 1
	}
	return SpriteMeta{X: x, Y: y, W: w, H: h, PixelOrder: order}, nil
}

// cropBitmap returns a new Bitmap copying the [x, x+w) by [y, y+h)
// region of img.
func cropBitmap(img *Bitmap, x, y, w, h int) *Bitmap {
	dst := &Bitmap{Width: w, Height: h, Data: make([]uint8, w*h*4)}
	for j := range h {
		srcOff := ((y+j)*img.Width + x) * 4
		dstOff := j * w * 4
		copy(dst.Data[dstOff:dstOff+w*4], img.Data[srcOff:srcOff+w*4])
	}
	return dst
}
