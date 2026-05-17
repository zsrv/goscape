package pixpack

import "github.com/zsrv/goscape/pkg/io/packet"

// SpriteMeta is the parsed <srcDir>/meta/<name>.opt sidecar.
//
// Fields mirror the TS Sprite type (PixPack.ts:106-112). PixelOrder
// is 0 for column-major and 1 for row-major.
type SpriteMeta struct {
	X, Y, W, H int
	PixelOrder int
}

// WriteImage emits one sprite frame to data, appending header bytes
// to index. Ports TS PixPack.ts:35-104.
//
// When meta is nil (or has zero W/H) the full bitmap is written with
// the auto-computed pixel order. When meta is provided with non-zero
// W/H the crop/pixel-order from meta is used instead.
//
// Indexing terminates early (TS uses `break`) when an unknown RGB
// value is encountered. In row-major (pixelOrder==0) this drops the
// rest of the frame; in column-major (pixelOrder==1) it drops only
// the rest of the current column — the outer x-loop continues.
func WriteImage(img *Bitmap, data, index *packet.Packet, colors []int32, meta *SpriteMeta) {
	left := 0
	top := 0
	right := img.Width
	bottom := img.Height

	if meta != nil && meta.W != 0 && meta.H != 0 {
		left = meta.X
		top = meta.Y
		right = meta.W
		bottom = meta.H
	}

	index.P1(uint8(left))    // crop x offset
	index.P1(uint8(top))     // crop y offset
	index.P2(uint16(right))  // actual width
	index.P2(uint16(bottom)) // actual height

	pixelOrder := GeneratePixelOrder(img)
	if meta != nil {
		pixelOrder = meta.PixelOrder
	}
	index.P1(uint8(pixelOrder))

	switch pixelOrder {
	case 0:
		for j := range img.Width * img.Height {
			x := j % img.Width
			y := j / img.Width
			if x >= right || y >= bottom {
				continue
			}

			pos := j*4 + left*4 + top*img.Width*4

			red := int32(img.Data[pos+0])
			green := int32(img.Data[pos+1])
			blue := int32(img.Data[pos+2])
			rgb := (red << 16) | (green << 8) | blue

			idx := indexOf(colors, rgb)
			if idx == -1 {
				break
			}
			data.P1(uint8(idx))
		}
	case 1:
		for x := range img.Width {
			for y := range img.Height {
				if x >= right || y >= bottom {
					continue
				}

				pos := (x+y*img.Width)*4 + left*4 + top*img.Width*4

				red := int32(img.Data[pos+0])
				green := int32(img.Data[pos+1])
				blue := int32(img.Data[pos+2])
				rgb := (red << 16) | (green << 8) | blue

				idx := indexOf(colors, rgb)
				if idx == -1 {
					break
				}
				data.P1(uint8(idx))
			}
		}
	}
}

// indexOf returns the index of target in colors, or -1 if absent.
// Mirrors TS Array.prototype.indexOf semantics.
func indexOf(colors []int32, target int32) int {
	for i, c := range colors {
		if c == target {
			return i
		}
	}
	return -1
}
