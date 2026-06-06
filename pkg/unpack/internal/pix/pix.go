package pix

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/jagfile"
)

// background is the sheet/sprite fill color mirroring Jimp's `color: 0xff00ffff`
// (RGBA bytes: R=0xFF G=0x00 B=0xFF A=0xFF — opaque magenta).
// TS Pix.ts:198-203 (sheet) and TS Pix.ts:260-264 (packPng single sprite).
var background = color.NRGBA{R: 0xFF, G: 0x00, B: 0xFF, A: 0xFF}

// Sprite holds the decoded fields of a single RS2 sprite.
// Mirrors the TS Pix constructor parameters (TS Pix.ts:20-31).
type Sprite struct {
	// Pixels holds palette indices in row-major order (cropRight × cropBottom).
	// TS Pix.ts:22 — `pixels: Uint8Array`
	Pixels []uint8
	// Palette holds RGB colors; index 0 is always transparent (value 0).
	// TS Pix.ts:23 — `palette: Int32Array`
	Palette []int32
	// Width / Height are the logical cell dimensions of the full spritesheet cell.
	// TS Pix.ts:24-25
	Width  int
	Height int
	// Crop fields describe the sub-region occupied by actual pixel data.
	// CropRight and CropBottom are the pixel-data width and height.
	// TS Pix.ts:26-29
	CropLeft   int
	CropTop    int
	CropRight  int
	CropBottom int
	// PixelOrder: 0 = row-major, 1 = column-major in the stored pixel stream.
	// TS Pix.ts:30
	PixelOrder int
}

// isPrime is O(sqrt(n)). TS Pix.ts:10-18.
func isPrime(n int) bool {
	if n <= 1 {
		return false
	}
	for i, s := 2, int(math.Sqrt(float64(n))); i <= s; i++ {
		if n%i == 0 {
			return false
		}
	}
	return true
}

// UnpackJag decodes sprite index from the named entry in jag.
// Returns (nil, nil) when there is no sprite at that index (mirrors TS null return).
// TS Pix.ts:72-139.
func UnpackJag(jag *jagfile.Jagfile, name string, index int) (*Sprite, error) {
	// TS Pix.ts:73-78 — read dat + idx, return null if missing.
	dat, err := jag.Read(name + ".dat")
	if err != nil {
		return nil, nil //nolint:nilerr // mirror TS: missing file → null
	}
	idx, err := jag.Read("index.dat")
	if err != nil {
		return nil, nil //nolint:nilerr // mirror TS: missing file → null
	}

	// TS Pix.ts:80 — idx.pos = dat.g2()
	// dat.g2() reads a uint16 from dat telling us where in index.dat to start.
	datG2 := int(dat.G2())
	idx.Pos = datG2

	// TS Pix.ts:82-84 — if (idx.pos >= idx.length) return null
	if idx.Pos >= len(idx.Data) {
		return nil, nil
	}

	// TS Pix.ts:86-93 — read width, height, palette.
	width := int(idx.G2())
	height := int(idx.G2())

	paletteCount := int(idx.G1())
	palette := make([]int32, paletteCount)
	// palette[0] is always 0 (transparent key), indices 1..paletteCount-1 are read.
	for i := range paletteCount - 1 {
		palette[i+1] = int32(idx.G3())
	}

	// TS Pix.ts:95-97 — second bounds check after palette read.
	if idx.Pos >= len(idx.Data) {
		return nil, nil
	}

	// TS Pix.ts:99-103 — skip past earlier sprites to reach the requested index.
	// Each sprite entry in index.dat costs: 1+1+2+2+1 = 7 bytes (cropX,cropY,w,h,pixelOrder).
	// The pixel data size is idx.g2()*idx.g2() bytes in dat.
	for range index {
		idx.Pos += 2 // cropLeft, cropTop (1 byte each)
		w := int(idx.G2())
		h := int(idx.G2())
		dat.Pos += w * h
		idx.Pos++ // pixelOrder
	}

	// TS Pix.ts:105-107 — bounds check after skip loop.
	if idx.Pos >= len(idx.Data) || dat.Pos >= len(dat.Data) {
		return nil, nil
	}

	// TS Pix.ts:109-113 — read crop fields + pixelOrder for this sprite.
	cropLeft := int(idx.G1())
	cropTop := int(idx.G1())
	cropRight := int(idx.G2())
	cropBottom := int(idx.G2())
	pixelOrder := int(idx.G1())

	// TS Pix.ts:115-117 — strict post-read bounds check.
	if idx.Pos > len(idx.Data) {
		return nil, nil
	}

	// TS Pix.ts:119-124 — allocate and bounds-check pixel buffer.
	pixLen := cropRight * cropBottom
	pixels := make([]uint8, pixLen)

	if dat.Pos+pixLen > len(dat.Data) {
		return nil, nil
	}

	// TS Pix.ts:126-136 — decode pixel stream by pixelOrder.
	if pixelOrder == 0 {
		// Row-major: pixels stored left→right, top→bottom.
		for i := range pixLen {
			pixels[i] = dat.G1()
		}
	} else if pixelOrder == 1 {
		// Column-major: pixels stored top→bottom within each column.
		for x := range cropRight {
			for y := range cropBottom {
				pixels[y*cropRight+x] = dat.G1()
			}
		}
	} else {
		return nil, fmt.Errorf("pix: unknown pixelOrder %d", pixelOrder)
	}

	// TS Pix.ts:138
	return &Sprite{
		Pixels:     pixels,
		Palette:    palette,
		Width:      width,
		Height:     height,
		CropLeft:   cropLeft,
		CropTop:    cropTop,
		CropRight:  cropRight,
		CropBottom: cropBottom,
		PixelOrder: pixelOrder,
	}, nil
}

// packPng converts a single Sprite into an NRGBA image.
// Background pixels (palette index 0) are left as opaque magenta (0xFF00FFFF).
// TS Pix.ts:259-308 (packPng method).
func (s *Sprite) packPng() *image.NRGBA {
	// TS Pix.ts:260-264 — create image filled with opaque magenta background.
	img := image.NewNRGBA(image.Rect(0, 0, s.Width, s.Height))
	// Fill with background color (magenta 0xFF00FFFF).
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = background.R
		img.Pix[i+1] = background.G
		img.Pix[i+2] = background.B
		img.Pix[i+3] = background.A
	}

	// TS Pix.ts:267-285 (pixelOrder==0) / TS Pix.ts:286-304 (pixelOrder==1).
	if s.PixelOrder == 0 {
		pixLen := s.CropRight * s.CropBottom
		for i := range pixLen {
			paletteIdx := s.Pixels[i]
			if paletteIdx == 0 {
				continue // TS: index===0 → skip (leave magenta background)
			}
			startX := s.CropLeft + (i % s.CropRight)
			startY := s.CropTop + (i / s.CropRight)
			rgb := s.Palette[paletteIdx]
			off := img.PixOffset(startX, startY)
			img.Pix[off] = uint8((rgb >> 16) & 0xFF)
			img.Pix[off+1] = uint8((rgb >> 8) & 0xFF)
			img.Pix[off+2] = uint8(rgb & 0xFF)
			img.Pix[off+3] = 0xFF
		}
	} else {
		for x := range s.CropRight {
			for y := range s.CropBottom {
				paletteIdx := s.Pixels[y*s.CropRight+x]
				if paletteIdx == 0 {
					continue // TS: index===0 → skip
				}
				startX := s.CropLeft + x
				startY := s.CropTop + y
				rgb := s.Palette[paletteIdx]
				off := img.PixOffset(startX, startY)
				img.Pix[off] = uint8((rgb >> 16) & 0xFF)
				img.Pix[off+1] = uint8((rgb >> 8) & 0xFF)
				img.Pix[off+2] = uint8(rgb & 0xFF)
				img.Pix[off+3] = 0xFF
			}
		}
	}

	return img
}

// sheetDimensions computes (sheetWidth, sheetHeight, ok) using the same
// algorithm as TS Pix.ts:161-194 (unpackJagToPng, defaulted preferHorizontal=true).
// Returns ok=false when the product does not equal count (sheet mismatch).
func sheetDimensions(count int) (sheetWidth, sheetHeight int, ok bool) {
	if isPrime(count) {
		// TS Pix.ts:164-166
		sheetWidth = count
		sheetHeight = 1
	} else {
		// TS Pix.ts:167-169
		sheetWidth = int(math.Ceil(math.Sqrt(float64(count))))
		sheetHeight = int(math.Ceil(float64(count) / float64(sheetWidth)))
	}

	// TS Pix.ts:172-188 — prefer horizontal: increment width, decrement height.
	if sheetWidth*sheetHeight > count {
		widthTries := 0
		for sheetWidth*sheetHeight > count && widthTries < 10 {
			sheetWidth++
			sheetHeight--
			widthTries++
		}
	}

	// TS Pix.ts:191-194 — validate exact fit.
	if sheetWidth*sheetHeight != count {
		return 0, 0, false
	}
	return sheetWidth, sheetHeight, true
}

// UnpackFull decodes all sprites from jag[name], writes <dir>/<name>.png,
// and writes <dir>/meta/<name>.opt when the .opt conditions are met.
// No-op when zero sprites found. TS Pix.ts:33-70.
//
// errorf is called when a spritesheet dimension mismatch is detected
// (TS Pix.ts:192 — printError("wrong spritesheet size! ...")).
// A nil errorf is treated as a no-op; the CLI passes its logger.
// Signature matches fmt.Printf to allow direct forwarding.
func UnpackFull(jag *jagfile.Jagfile, dir, name string, errorf func(format string, args ...any)) error {
	if errorf == nil {
		errorf = func(format string, args ...any) {}
	}
	// TS Pix.ts:34-41 — collect all sprites up to 1000.
	var all []*Sprite
	for i := range 1000 {
		s, err := UnpackJag(jag, name, i)
		if err != nil {
			return fmt.Errorf("pix: UnpackJag index %d: %w", i, err)
		}
		if s == nil {
			break
		}
		all = append(all, s)
	}

	// TS Pix.ts:44-46 — no sprites → return.
	if len(all) == 0 {
		return nil
	}

	// Build the PNG image. TS Pix.ts:48 calls unpackJagToPng which handles
	// both single and multi-sprite cases.
	//
	// TS Pix.ts:191-194: when sheetWidth*sheetHeight != count, unpackJagToPng
	// calls printError and returns null.  TS Pix.ts:52 then skips the PNG write
	// because `if (png)` is false, but the .opt write at Pix.ts:56-69 still runs.
	// We mirror that: on dimension failure we call Errorf (wired by the CLI),
	// set img=nil (skip PNG), and fall through to the .opt section.
	var img *image.NRGBA
	if len(all) == 1 {
		// TS Pix.ts:157-159 — single sprite: return all[0].packPng() directly.
		img = all[0].packPng()
	} else {
		// Multi-sprite sheet. TS Pix.ts:161-222.
		count := len(all)
		sheetWidth, sheetHeight, ok := sheetDimensions(count)
		if !ok {
			// TS Pix.ts:191-194 — dimension mismatch: log and return null (skip PNG).
			errorf("wrong spritesheet size! you may have to manually define its dimensions: %dx%d != %d", sheetWidth, sheetHeight, count)
			// img stays nil — PNG write is skipped below (TS Pix.ts:52 `if (png)`).
		} else {
			cellWidth := all[0].Width
			cellHeight := all[0].Height

			// TS Pix.ts:198-202 — create sheet filled with opaque magenta.
			sheet := image.NewNRGBA(image.Rect(0, 0, sheetWidth*cellWidth, sheetHeight*cellHeight))
			for i := 0; i < len(sheet.Pix); i += 4 {
				sheet.Pix[i] = background.R
				sheet.Pix[i+1] = background.G
				sheet.Pix[i+2] = background.B
				sheet.Pix[i+3] = background.A
			}

			// TS Pix.ts:204-221 — blit each sprite onto the sheet.
			for idx, s := range all {
				// TS Pix.ts:208-209 — sheet cell coordinates.
				cellX := idx % sheetWidth
				cellY := idx / sheetWidth
				destX := cellX * cellWidth
				destY := cellY * cellHeight

				// Render each sprite into its own image then blit it.
				// TS Pix.ts:206 — pix.packPng()
				// TS Pix.ts:211-219 — sheet.blit({src: img, x, y, srcX:0, srcY:0, srcW:cellWidth, srcH:cellHeight})
				// Jimp blit copies srcW×srcH pixels from (srcX,srcY) of src to (x,y) of dst.
				sprite := s.packPng()
				for py := range cellHeight {
					for px := range cellWidth {
						srcOff := sprite.PixOffset(px, py)
						dstOff := sheet.PixOffset(destX+px, destY+py)
						sheet.Pix[dstOff] = sprite.Pix[srcOff]
						sheet.Pix[dstOff+1] = sprite.Pix[srcOff+1]
						sheet.Pix[dstOff+2] = sprite.Pix[srcOff+2]
						sheet.Pix[dstOff+3] = sprite.Pix[srcOff+3]
					}
				}
			}
			img = sheet
		}
	}

	// TS Pix.ts:52-54 — write PNG file only when unpackJagToPng returned non-null.
	// On dimension failure img is nil and we skip the write (mirrors `if (png)` guard).
	if img != nil {
		pngPath := filepath.Join(dir, name+".png")
		if err := writePNG(pngPath, img); err != nil {
			return fmt.Errorf("pix: write PNG: %w", err)
		}
	}

	// TS Pix.ts:56-58 — ensure meta/ directory exists.
	metaDir := filepath.Join(dir, "meta")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		return fmt.Errorf("pix: mkdir meta: %w", err)
	}

	// TS Pix.ts:60-69 — write .opt file under the appropriate conditions.
	optPath := filepath.Join(metaDir, name+".opt")
	if len(all) > 1 {
		// TS Pix.ts:61-64 — multi-sprite: sheet cell dims + per-sprite crop lines.
		var opt []byte
		opt = fmt.Appendf(opt, "%dx%d\n", all[0].Width, all[0].Height)
		for _, s := range all {
			opt = fmt.Appendf(opt, "%d,%d,%d,%d\n", s.CropLeft, s.CropTop, s.CropRight, s.CropBottom)
		}
		if err := os.WriteFile(optPath, opt, 0644); err != nil {
			return fmt.Errorf("pix: write .opt: %w", err)
		}
	} else {
		// TS Pix.ts:67-69 — single sprite: only emit .opt when crop != full cell.
		s := all[0]
		if s.CropLeft != 0 || s.CropTop != 0 || s.CropRight != s.Width || s.CropBottom != s.Height {
			opt := fmt.Sprintf("%d,%d,%d,%d\n", s.CropLeft, s.CropTop, s.CropRight, s.CropBottom)
			if err := os.WriteFile(optPath, []byte(opt), 0644); err != nil {
				return fmt.Errorf("pix: write .opt: %w", err)
			}
		}
	}

	return nil
}

// writePNG encodes img as a PNG file at path, creating parent dirs as needed.
func writePNG(path string, img image.Image) (retErr error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && retErr == nil {
			retErr = cerr
		}
	}()
	return png.Encode(f, img)
}
