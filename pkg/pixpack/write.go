package pixpack

import (
	"slices"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// magenta is the transparency sentinel: pixels of this colour are excluded
// from the crop bounds and occupy palette entry 0.
const magenta int32 = 0xff00ff

// PixelBounds is the tight non-transparent extent of a bitmap, derived by
// scanning rather than read from a .opt sidecar.
type PixelBounds struct {
	Left, Top, Width, Height int
}

// GetPixelBounds returns the smallest rectangle containing every
// non-magenta pixel. A fully-transparent (or empty) image yields the full
// frame, matching the TS `right === -1` fallback.
//
// Ports TS PixPack.ts:62-81 @1d25566c. Engine-TS 8139461a replaced the
// per-sprite crop rows in meta/<name>.opt with this scan, which is why the
// Content sidecars shrank to a bare "<tileX>x<tileY>" line (or vanished) in
// Content 687b6a1a1.
func GetPixelBounds(img *Bitmap) PixelBounds {
	left, top := img.Width, img.Height
	right, bottom := -1, -1

	for y := range img.Height {
		for x := range img.Width {
			pos := (x + y*img.Width) * 4
			if img.Data[pos] == 0xff && img.Data[pos+1] == 0 && img.Data[pos+2] == 0xff {
				continue
			}
			left = min(left, x)
			top = min(top, y)
			right = max(right, x)
			bottom = max(bottom, y)
		}
	}

	if right == -1 {
		return PixelBounds{Left: 0, Top: 0, Width: img.Width, Height: img.Height}
	}
	return PixelBounds{
		Left:   left,
		Top:    top,
		Width:  right - left + 1,
		Height: bottom - top + 1,
	}
}

// WriteImage emits one sprite frame to data, appending header bytes to index.
//
// Ports TS PixPack.ts:83-97 @1d25566c. Engine-TS 8139461a rewrote this: the
// crop rectangle is now scanned (GetPixelBounds) instead of read from a
// sidecar, the pixel order is chosen by GeneratePixelOrder over that
// rectangle, and the meta parameter is gone entirely.
//
// The pixel walk also lost its early exit. The old code broke out of the loop
// when a colour was absent from the palette; TS now writes
// `colors.indexOf(rgb)` unconditionally, so an absent colour emits 0xff (p1 of
// -1) and the frame keeps its full length. Mirrored verbatim — the palette is
// generated from the same bitmap, so an absent colour only arises when the
// caller injects a foreign palette.
func WriteImage(img *Bitmap, data, index *packet.Packet, colors []int32) {
	b := GetPixelBounds(img)
	pixelOrder := GeneratePixelOrder(img, b.Left, b.Top, b.Width, b.Height)

	index.P1(uint8(b.Left))     // crop x offset
	index.P1(uint8(b.Top))      // crop y offset
	index.P2(uint16(b.Width))   // actual width
	index.P2(uint16(b.Height))  // actual height
	index.P1(uint8(pixelOrder)) // 0 = row-major, 1 = column-major

	outer, inner := b.Height, b.Width
	if pixelOrder == 1 {
		outer, inner = b.Width, b.Height
	}

	for a := range outer {
		for bb := range inner {
			x, y := bb, a
			if pixelOrder == 1 {
				x, y = a, bb
			}
			pos := (x + b.Left + (y+b.Top)*img.Width) * 4
			rgb := int32(img.Data[pos])<<16 | int32(img.Data[pos+1])<<8 | int32(img.Data[pos+2])
			data.P1(uint8(slices.Index(colors, rgb)))
		}
	}
}
