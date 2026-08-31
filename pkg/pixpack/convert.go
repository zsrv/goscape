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
// <srcDir>/meta/<name>.opt for the spritesheet tiling, encodes the image into
// the RS sprite format, and appends frame headers to index. Returns the
// per-sprite payload Packet (caller must Release).
//
// Ports TS PixPack.ts:99-146 @1d25566c.
//
// Engine-TS 8139461a reduced the .opt sidecar to a single "<tileX>x<tileY>"
// line: the per-sprite crop/pixel-order rows are gone, because WriteImage now
// derives both by scanning. Tiling is detected by comparing the tile size
// against the image size rather than by counting sidecar rows, so a sidecar
// whose tiling equals the full image is equivalent to having none.
//
// The upstream signature also grew optional `source` and `palette`
// parameters. No caller passes them at this pin (verified across
// tools/pack/map/Worldmap.ts, sprite/media.ts, sprite/textures.ts and
// sprite/title.ts @1d25566c), so they are omitted here rather than carried as
// dead API.
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

	tileX, tileY, err := loadTiling(srcDir, name, img.Width, img.Height)
	if err != nil {
		data.Release()
		return nil, fmt.Errorf("ConvertImage(%q): %w", name, err)
	}

	index.P2(uint16(tileX))
	index.P2(uint16(tileY))

	// TS PixPack.ts @2e3bcf43 removed the rev-244-era meta/<name>.pal.png
	// CRC-preserving palette workaround — the palette always derives from
	// the source image now (a stray pal.png file is ignored). Caught by
	// the T30 rev-254 correspondence audit; vacuous on the pinned Content
	// caee3f2e (zero pal.png files), so the full-tree parity gate could
	// not see it.
	colors := generatePalette(img)
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

	// TS: `if (tileX !== img.bitmap.width || tileY !== img.bitmap.height)` —
	// tiling is inferred from the tile size, not from a sidecar row count.
	if tileX != img.Width || tileY != img.Height {
		for y := range img.Height / tileY {
			for x := range img.Width / tileX {
				tile := cropBitmap(img, x*tileX, y*tileY, tileX, tileY)
				WriteImage(tile, data, index, colors)
			}
		}
	} else {
		WriteImage(img, data, index, colors)
	}

	return data, nil
}

// loadTiling parses the tile dimensions from <srcDir>/meta/<name>.opt.
//
// Ports TS PixPack.ts:110-119 @1d25566c. Since 8139461a the sidecar holds
// exactly one meaningful line, "<tileX>x<tileY>"; everything after it is
// ignored (TS passes a limit of 1 to split). Absent sidecar means the whole
// image is one tile.
//
// The tiling is validated: both values must parse as integers and must divide
// the image exactly. TS raises `Invalid image metadata: <path>` for any of
// those failures — including the NaN that a malformed line produces — so a
// bad sidecar is a hard error, not a silent fallback.
func loadTiling(srcDir, name string, defaultTileX, defaultTileY int) (int, int, error) {
	tileX, tileY := defaultTileX, defaultTileY

	path := filepath.Join(srcDir, "meta", name+".opt")
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// no sidecar: the image is a single tile
	case err != nil:
		return 0, 0, err
	default:
		// TS: .split(/\r?\n/, 1)[0].trim().split('x').map(Number)
		first, _, _ := strings.Cut(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
		xs, ys, ok := strings.Cut(strings.TrimSpace(first), "x")
		if !ok {
			return 0, 0, fmt.Errorf("invalid image metadata: %s", path)
		}
		tileX, err = strconv.Atoi(xs)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid image metadata: %s", path)
		}
		tileY, err = strconv.Atoi(ys)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid image metadata: %s", path)
		}
	}

	if tileX <= 0 || tileY <= 0 || defaultTileX%tileX != 0 || defaultTileY%tileY != 0 {
		return 0, 0, fmt.Errorf("invalid image metadata: %s", path)
	}
	return tileX, tileY, nil
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
