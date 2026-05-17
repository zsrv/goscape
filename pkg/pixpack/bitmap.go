// Package pixpack ports tools/pack/PixPack.ts from the TS Engine to
// Go, using stdlib image/png. It provides the sprite-codec primitives
// (palette generation, pixel-order scoring, frame writing, and the
// top-level ConvertImage entry point) used by later pack stages.
package pixpack

import (
	"fmt"
	"image"
	_ "image/png" // register PNG decoder
	"os"
)

// Bitmap is the RGBA buffer shim mirroring Jimp's bitmap.data layout
// for byte-faithful palette/RLE logic ports from TS PixPack.
//
// Layout: len(Data) == Width*Height*4; byte order per pixel is R, G,
// B, A; pixels are row-major (pixel (x,y) starts at byte
// (x + y*Width) * 4).
//
// NAI-213-D-PIXPACK-RGBA-LAYOUT: custom RGBA buffer instead of
// third-party Jimp dep. Permanent.
type Bitmap struct {
	Width, Height int
	Data          []uint8
}

// decodePNG reads <path>, decodes it as PNG, and returns a Bitmap
// with the pixels laid out as R, G, B, A row-major.
func decodePNG(path string) (*Bitmap, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("decodePNG: open %q: %w", path, err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decodePNG: decode %q: %w", path, err)
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	data := make([]uint8, w*h*4)
	for y := range h {
		for x := range w {
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			pos := (x + y*w) * 4
			data[pos+0] = uint8(r >> 8)
			data[pos+1] = uint8(g >> 8)
			data[pos+2] = uint8(b >> 8)
			data[pos+3] = uint8(a >> 8)
		}
	}
	return &Bitmap{Width: w, Height: h, Data: data}, nil
}
