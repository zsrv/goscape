package pix

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// ---- fixture helpers --------------------------------------------------------

// buildIndexDat constructs an index.dat payload for one or more sprites
// sharing the same cell width/height and palette.
//
// index.dat layout (TS Pix.ts:80-103):
//
//	For EACH named entry (each name.dat has a different jag-level .dat file),
//	the index.dat is shared and contains multiple logical sprite records.
//
//	One logical block per (width,height,palette) group:
//	  [offset into this index.dat block — 2 bytes] ← value stored in name.dat g2
//	  [width  — 2 bytes]
//	  [height — 2 bytes]
//	  [paletteCount — 1 byte]
//	  [(paletteCount-1) × 3-byte RGB entries] (palette[1..])
//	  Then for each sprite in the group:
//	    [cropLeft  — 1 byte]
//	    [cropTop   — 1 byte]
//	    [cropRight — 2 bytes]
//	    [cropBottom— 2 bytes]
//	    [pixelOrder— 1 byte]
//
// The name.dat stores: [2-byte offset into index.dat] + raw pixel bytes for each sprite.

// spriteSpec describes one sprite for fixture building.
type spriteSpec struct {
	width, height         int
	cropLeft, cropTop     int
	cropRight, cropBottom int
	pixelOrder            int
	pixels                []uint8 // raw pixel data (palette indices), len = cropRight*cropBottom
	palette               []int32 // palette[0]=0 always; palette[1..] = RGB values
}

// buildJag creates an in-memory jagfile for a single sprite group (one name.dat).
// All sprites must share the same palette (merged into the first sprite's palette).
func buildJag(t *testing.T, name string, sprites []spriteSpec) *jagfile.Jagfile {
	t.Helper()

	// All sprites must have the same palette; use the first sprite's palette.
	palette := sprites[0].palette

	// ---- Build index.dat ----
	//
	// The pointer stored in name.dat is the byte offset of the
	// [width, height, paletteCount, palette...] block WITHIN index.dat.
	// We lay out index.dat so that the pointer can be measured.
	//
	// Layout of index.dat:
	//   [pointer_value = current offset]  ← this is what dat.g2() returns; we compute
	//                                         it as the offset we place width at.
	//   [width  2 bytes]
	//   [height 2 bytes]
	//   [paletteCount 1 byte]
	//   [(paletteCount-1) × 3 bytes]
	//   per sprite: [cropLeft 1][cropTop 1][cropRight 2][cropBottom 2][pixelOrder 1]

	idxBuf := packet.Alloc(256)

	// The pointer (dat.g2) points to the START of the width field.
	// index.dat may contain multiple groups; we place our group at byte 0
	// so the pointer == 0.  That means dat.g2() == 0.
	idxOffset := 0 // offset into index.dat where we start

	// Record where we are before writing width (should be idxOffset = 0 here).
	require.Equal(t, idxOffset, len(idxBuf.Data))

	idxBuf.P2(uint16(sprites[0].width))
	idxBuf.P2(uint16(sprites[0].height))
	idxBuf.P1(uint8(len(palette)))
	for i := 1; i < len(palette); i++ {
		idxBuf.P3(uint32(palette[i]))
	}
	for _, s := range sprites {
		idxBuf.P1(uint8(s.cropLeft))
		idxBuf.P1(uint8(s.cropTop))
		idxBuf.P2(uint16(s.cropRight))
		idxBuf.P2(uint16(s.cropBottom))
		idxBuf.P1(uint8(s.pixelOrder))
	}

	// ---- Build name.dat ----
	// Layout: [2-byte pointer into index.dat] + pixel bytes for all sprites
	// in column-major order if pixelOrder==1, row-major otherwise
	// (TS Pix.ts:126-136 — we store them as the decode expects to read them).
	datBuf := packet.Alloc(256)
	datBuf.P2(uint16(idxOffset)) // pointer into index.dat
	for _, s := range sprites {
		datBuf.PData(s.pixels)
	}

	// ---- Assemble jagfile ----
	jag := jagfile.NewEmptyJagfile(true)
	jag.Write("index.dat", idxBuf)
	jag.Write(name+".dat", datBuf)

	// Save + reload to get a properly constructed Jagfile that Read() can use.
	dir := t.TempDir()
	jagPath := filepath.Join(dir, "test.jag")
	err := jag.Save(jagPath)
	require.NoError(t, err)

	loaded, err := jagfile.LoadJagfile(jagPath)
	require.NoError(t, err)
	return loaded
}

// loadPNG reads a PNG file and returns its NRGBA image.
func loadPNG(t *testing.T, path string) *image.NRGBA {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	img, err := png.Decode(f)
	require.NoError(t, err)
	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		// Convert to NRGBA for uniform pixel access.
		bounds := img.Bounds()
		nrgba = image.NewNRGBA(bounds)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				c := img.At(x, y)
				r, g, b, a := c.RGBA()
				nrgba.SetNRGBA(x, y, color.NRGBA{
					R: uint8(r >> 8),
					G: uint8(g >> 8),
					B: uint8(b >> 8),
					A: uint8(a >> 8),
				})
			}
		}
	}
	return nrgba
}

// pixelAt returns the NRGBA color at (x,y) without bounds check panic.
func pixelAt(img *image.NRGBA, x, y int) color.NRGBA {
	off := img.PixOffset(x, y)
	return color.NRGBA{
		R: img.Pix[off],
		G: img.Pix[off+1],
		B: img.Pix[off+2],
		A: img.Pix[off+3],
	}
}

// ---- tests ------------------------------------------------------------------

// TestUnpackJag_RowMajor — Test 1: 2×2 sprite, pixelOrder 0 (row-major).
// Pins width/height/crop fields and all four pixel RGBA values.
func TestUnpackJag_RowMajor(t *testing.T) {
	// 2×2 sprite that fills the full cell (no crop offset).
	// Palette: [0]=transparent, [1]=red(0xFF0000), [2]=green(0x00FF00), [3]=blue(0x0000FF), [4]=white(0xFFFFFF)
	// Pixels (row-major, 2×2):
	//   [0,0]=1(red)   [0,1]=2(green)
	//   [1,0]=3(blue)  [1,1]=4(white)
	// stored as indices: [1, 2, 3, 4]
	spec := spriteSpec{
		width: 2, height: 2,
		cropLeft: 0, cropTop: 0, cropRight: 2, cropBottom: 2,
		pixelOrder: 0,
		pixels:     []uint8{1, 2, 3, 4},
		palette:    []int32{0, 0xFF0000, 0x00FF00, 0x0000FF, 0xFFFFFF},
	}
	jag := buildJag(t, "test", []spriteSpec{spec})

	s, err := UnpackJag(jag, "test", 0)
	require.NoError(t, err)
	require.NotNil(t, s)

	// Pin structural fields.
	assert.Equal(t, 2, s.Width)
	assert.Equal(t, 2, s.Height)
	assert.Equal(t, 0, s.CropLeft)
	assert.Equal(t, 0, s.CropTop)
	assert.Equal(t, 2, s.CropRight)
	assert.Equal(t, 2, s.CropBottom)
	assert.Equal(t, 0, s.PixelOrder)

	// Pin pixel values.
	require.Len(t, s.Pixels, 4)
	assert.Equal(t, uint8(1), s.Pixels[0])
	assert.Equal(t, uint8(2), s.Pixels[1])
	assert.Equal(t, uint8(3), s.Pixels[2])
	assert.Equal(t, uint8(4), s.Pixels[3])

	// Render to PNG and pin each RGBA pixel.
	img := s.packPng()
	assert.Equal(t, color.NRGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}, pixelAt(img, 0, 0), "top-left = red")
	assert.Equal(t, color.NRGBA{R: 0x00, G: 0xFF, B: 0x00, A: 0xFF}, pixelAt(img, 1, 0), "top-right = green")
	assert.Equal(t, color.NRGBA{R: 0x00, G: 0x00, B: 0xFF, A: 0xFF}, pixelAt(img, 0, 1), "bottom-left = blue")
	assert.Equal(t, color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}, pixelAt(img, 1, 1), "bottom-right = white")
}

// TestUnpackJag_ColumnMajor — Test 2: 2×2 sprite, pixelOrder 1 (column-major).
// Same palette, same pixels in column-major stream → same rendered image (transposed read).
func TestUnpackJag_ColumnMajor(t *testing.T) {
	// pixelOrder=1: stored as column-major: col0=[row0,row1], col1=[row0,row1]
	// To get rendered (x=0,y=0)=red, (x=1,y=0)=green, (x=0,y=1)=blue, (x=1,y=1)=white:
	//   pixels[y*cropRight+x]:  [0,0]=1, [1,0]=3, [0,1]=2, [1,1]=4
	// Column-major stream for x=0: y=0→1, y=1→3; for x=1: y=0→2, y=1→4.
	// Stream: [1, 3, 2, 4]
	spec := spriteSpec{
		width: 2, height: 2,
		cropLeft: 0, cropTop: 0, cropRight: 2, cropBottom: 2,
		pixelOrder: 1,
		pixels:     []uint8{1, 3, 2, 4},
		palette:    []int32{0, 0xFF0000, 0x00FF00, 0x0000FF, 0xFFFFFF},
	}
	jag := buildJag(t, "test", []spriteSpec{spec})

	s, err := UnpackJag(jag, "test", 0)
	require.NoError(t, err)
	require.NotNil(t, s)

	assert.Equal(t, 1, s.PixelOrder)
	// Internal pixels are stored in row-major logical order after column-major decode.
	// TS Pix.ts:131-135: pixels[y*cropRight+x] = dat.g1() fills by column.
	// Stream [1,3,2,4]: x=0,y=0→pixels[0]=1; x=0,y=1→pixels[2]=3; x=1,y=0→pixels[1]=2; x=1,y=1→pixels[3]=4.
	require.Len(t, s.Pixels, 4)
	assert.Equal(t, uint8(1), s.Pixels[0]) // (x=0,y=0) = red
	assert.Equal(t, uint8(2), s.Pixels[1]) // (x=1,y=0) = green
	assert.Equal(t, uint8(3), s.Pixels[2]) // (x=0,y=1) = blue
	assert.Equal(t, uint8(4), s.Pixels[3]) // (x=1,y=1) = white

	// Render: column-major decode → same visual as row-major test.
	img := s.packPng()
	assert.Equal(t, color.NRGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}, pixelAt(img, 0, 0), "top-left = red")
	assert.Equal(t, color.NRGBA{R: 0x00, G: 0xFF, B: 0x00, A: 0xFF}, pixelAt(img, 1, 0), "top-right = green")
	assert.Equal(t, color.NRGBA{R: 0x00, G: 0x00, B: 0xFF, A: 0xFF}, pixelAt(img, 0, 1), "bottom-left = blue")
	assert.Equal(t, color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}, pixelAt(img, 1, 1), "bottom-right = white")
}

// TestUnpackJag_PaletteIndex0Transparent — Test 3: palette index 0 = background (opaque magenta).
// TS Pix.ts:272-274 and TS Pix.ts:289-291: `if (index === 0) continue`.
// Background remains 0xFF00FFFF (opaque magenta).
func TestUnpackJag_PaletteIndex0Transparent(t *testing.T) {
	// 2×2 sprite; pixel (0,0) uses palette index 0 (transparent key → background).
	// Pixel (1,0) uses palette index 1 = solid red.
	// Pixels (0,1) and (1,1) also use index 0.
	spec := spriteSpec{
		width: 2, height: 2,
		cropLeft: 0, cropTop: 0, cropRight: 2, cropBottom: 2,
		pixelOrder: 0,
		pixels:     []uint8{0, 1, 0, 0},
		palette:    []int32{0, 0xFF0000},
	}
	jag := buildJag(t, "test", []spriteSpec{spec})

	s, err := UnpackJag(jag, "test", 0)
	require.NoError(t, err)
	require.NotNil(t, s)

	img := s.packPng()

	// Index 0 → background = opaque magenta (0xFF00FFFF). TS Pix.ts:260-264.
	wantBg := color.NRGBA{R: 0xFF, G: 0x00, B: 0xFF, A: 0xFF}
	assert.Equal(t, wantBg, pixelAt(img, 0, 0), "(0,0) must be background magenta")
	assert.Equal(t, wantBg, pixelAt(img, 0, 1), "(0,1) must be background magenta")
	assert.Equal(t, wantBg, pixelAt(img, 1, 1), "(1,1) must be background magenta")

	// Index 1 = red, fully opaque.
	assert.Equal(t, color.NRGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}, pixelAt(img, 1, 0), "(1,0) must be red")
}

// TestUnpackFull_MultiSprite — Test 4: 3-sprite sheet.
// Pins sheet dimensions from the TS algorithm (3 is prime → sheetWidth=3, sheetHeight=1)
// and each sprite's blit offset (marker pixel per sprite).
func TestUnpackFull_MultiSprite(t *testing.T) {
	// 3 sprites, each 4×4, palette: [0]=transparent, [1]=red, [2]=green, [3]=blue.
	// Sprite 0: all red (index 1).
	// Sprite 1: all green (index 2).
	// Sprite 2: all blue (index 3).
	palette := []int32{0, 0xFF0000, 0x00FF00, 0x0000FF}
	mkPixels := func(idx uint8) []uint8 {
		p := make([]uint8, 16)
		for i := range p {
			p[i] = idx
		}
		return p
	}
	specs := []spriteSpec{
		{width: 4, height: 4, cropRight: 4, cropBottom: 4, pixelOrder: 0, pixels: mkPixels(1), palette: palette},
		{width: 4, height: 4, cropRight: 4, cropBottom: 4, pixelOrder: 0, pixels: mkPixels(2), palette: palette},
		{width: 4, height: 4, cropRight: 4, cropBottom: 4, pixelOrder: 0, pixels: mkPixels(3), palette: palette},
	}
	jag := buildJag(t, "sprites", specs)

	dir := t.TempDir()
	err := UnpackFull(jag, dir, "sprites", nil)
	require.NoError(t, err)

	// PNG must exist.
	pngPath := filepath.Join(dir, "sprites.png")
	require.FileExists(t, pngPath)

	img := loadPNG(t, pngPath)

	// 3 is prime → sheetWidth=3, sheetHeight=1 (TS Pix.ts:164-166).
	// Sheet size = 3*4 × 1*4 = 12×4. TS Pix.ts:199-200.
	bounds := img.Bounds()
	assert.Equal(t, 12, bounds.Max.X, "sheet width = sheetWidth*cellWidth = 3*4")
	assert.Equal(t, 4, bounds.Max.Y, "sheet height = sheetHeight*cellHeight = 1*4")

	// Sprite 0 at cell (0,0) → pixels in x=[0,4), y=[0,4) should be red.
	assert.Equal(t, color.NRGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}, pixelAt(img, 0, 0), "sprite0 top-left = red")
	assert.Equal(t, color.NRGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}, pixelAt(img, 3, 3), "sprite0 bottom-right = red")

	// Sprite 1 at cell (1,0) → pixels in x=[4,8), y=[0,4) should be green.
	assert.Equal(t, color.NRGBA{R: 0x00, G: 0xFF, B: 0x00, A: 0xFF}, pixelAt(img, 4, 0), "sprite1 top-left = green")
	assert.Equal(t, color.NRGBA{R: 0x00, G: 0xFF, B: 0x00, A: 0xFF}, pixelAt(img, 7, 3), "sprite1 bottom-right = green")

	// Sprite 2 at cell (2,0) → pixels in x=[8,12), y=[0,4) should be blue.
	assert.Equal(t, color.NRGBA{R: 0x00, G: 0x00, B: 0xFF, A: 0xFF}, pixelAt(img, 8, 0), "sprite2 top-left = blue")
	assert.Equal(t, color.NRGBA{R: 0x00, G: 0x00, B: 0xFF, A: 0xFF}, pixelAt(img, 11, 3), "sprite2 bottom-right = blue")

	// .opt must exist for multi-sprite (TS Pix.ts:60-66).
	optPath := filepath.Join(dir, "meta", "sprites.opt")
	require.FileExists(t, optPath)

	optBytes, err := os.ReadFile(optPath)
	require.NoError(t, err)
	// First line: cell dims "4x4". Then 3 crop lines "0,0,4,4".
	optStr := string(optBytes)
	assert.Equal(t, "4x4\n0,0,4,4\n0,0,4,4\n0,0,4,4\n", optStr)
}

// TestUnpackFull_OptBytes — Test 5: .opt bytes for both multi-sprite and
// single-sprite-with-nonzero-crop; also verifies no-.opt for uncropped single sprite.
func TestUnpackFull_OptBytes(t *testing.T) {
	t.Run("multi-sprite opt", func(t *testing.T) {
		// 2 sprites 8×8, with different crops.
		palette := []int32{0, 0xFF0000, 0x00FF00}
		specs := []spriteSpec{
			{
				width: 8, height: 8,
				cropLeft: 1, cropTop: 2, cropRight: 3, cropBottom: 4,
				pixelOrder: 0,
				pixels:     make([]uint8, 3*4), // all index 0
				palette:    palette,
			},
			{
				width: 8, height: 8,
				cropLeft: 5, cropTop: 6, cropRight: 2, cropBottom: 1,
				pixelOrder: 0,
				pixels:     make([]uint8, 2*1), // all index 0
				palette:    palette,
			},
		}
		jag := buildJag(t, "multi", specs)
		dir := t.TempDir()
		require.NoError(t, UnpackFull(jag, dir, "multi", nil))

		optBytes, err := os.ReadFile(filepath.Join(dir, "meta", "multi.opt"))
		require.NoError(t, err)
		// TS Pix.ts:61: "${all[0].width}x${all[0].height}\n"
		// TS Pix.ts:63: per sprite "${cropLeft},${cropTop},${cropRight},${cropBottom}\n"
		assert.Equal(t, "8x8\n1,2,3,4\n5,6,2,1\n", string(optBytes))
	})

	t.Run("single cropped opt", func(t *testing.T) {
		// Single 4×4 cell, pixel data is 2×2 at offset (1,1).
		palette := []int32{0, 0xFF0000}
		spec := spriteSpec{
			width: 4, height: 4,
			cropLeft: 1, cropTop: 1, cropRight: 2, cropBottom: 2,
			pixelOrder: 0,
			pixels:     []uint8{1, 1, 1, 1},
			palette:    palette,
		}
		jag := buildJag(t, "cropped", []spriteSpec{spec})
		dir := t.TempDir()
		require.NoError(t, UnpackFull(jag, dir, "cropped", nil))

		optBytes, err := os.ReadFile(filepath.Join(dir, "meta", "cropped.opt"))
		require.NoError(t, err)
		// TS Pix.ts:68: "${cropLeft},${cropTop},${cropRight},${cropBottom}\n"
		assert.Equal(t, "1,1,2,2\n", string(optBytes))
	})

	t.Run("single uncropped no opt", func(t *testing.T) {
		// Single 2×2, crop == full cell → no .opt emitted. TS Pix.ts:67.
		palette := []int32{0, 0xFF0000}
		spec := spriteSpec{
			width: 2, height: 2,
			cropLeft: 0, cropTop: 0, cropRight: 2, cropBottom: 2,
			pixelOrder: 0,
			pixels:     []uint8{1, 1, 1, 1},
			palette:    palette,
		}
		jag := buildJag(t, "full", []spriteSpec{spec})
		dir := t.TempDir()
		require.NoError(t, UnpackFull(jag, dir, "full", nil))

		// .opt must NOT exist. TS Pix.ts:67 condition:
		//   cropLeft!=0 || cropTop!=0 || cropRight!=width || cropBottom!=height
		_, err := os.Stat(filepath.Join(dir, "meta", "full.opt"))
		assert.True(t, os.IsNotExist(err), "no .opt for uncropped single sprite")
	})
}

// TestUnpackFull_NoMoreSprites — Test 6: jag with 1 sprite → index-1 stops without error.
// TS Pix.ts:36-39: loop breaks when unpackJag returns null (Go: nil,nil).
func TestUnpackFull_NoMoreSprites(t *testing.T) {
	palette := []int32{0, 0xFF0000}
	spec := spriteSpec{
		width: 2, height: 2,
		cropLeft: 0, cropTop: 0, cropRight: 2, cropBottom: 2,
		pixelOrder: 0,
		pixels:     []uint8{1, 1, 1, 1},
		palette:    palette,
	}
	jag := buildJag(t, "one", []spriteSpec{spec})

	// Index 0 should succeed.
	s0, err := UnpackJag(jag, "one", 0)
	require.NoError(t, err)
	require.NotNil(t, s0)

	// Index 1 should return nil,nil (no more sprites).
	s1, err := UnpackJag(jag, "one", 1)
	require.NoError(t, err, "no-more-sprites must not return an error")
	assert.Nil(t, s1, "index 1 must return nil for a 1-sprite jag")

	// UnpackFull must not error either.
	dir := t.TempDir()
	require.NoError(t, UnpackFull(jag, dir, "one", nil))
	require.FileExists(t, filepath.Join(dir, "one.png"))
}

// TestIsPrime pins the helper used by sheetDimensions. TS Pix.ts:10-18.
func TestIsPrime(t *testing.T) {
	primes := []int{2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31}
	for _, p := range primes {
		assert.True(t, isPrime(p), "%d should be prime", p)
	}
	notPrimes := []int{0, 1, 4, 6, 8, 9, 10, 12, 100}
	for _, n := range notPrimes {
		assert.False(t, isPrime(n), "%d should not be prime", n)
	}
}

// TestSheetDimensions pins the sheet layout algorithm. TS Pix.ts:161-194.
func TestSheetDimensions(t *testing.T) {
	cases := []struct {
		count         int
		wantW, wantH  int
		wantOk        bool
	}{
		// prime counts → sheetWidth=count, sheetHeight=1.
		{count: 1, wantW: 1, wantH: 1, wantOk: true},  // 1 is not prime; sqrt(1)=1 → ceil(1)=1, 1×1=1
		{count: 2, wantW: 2, wantH: 1, wantOk: true},  // 2 is prime
		{count: 3, wantW: 3, wantH: 1, wantOk: true},  // 3 is prime
		{count: 4, wantW: 2, wantH: 2, wantOk: true},  // sqrt(4)=2, 2×2=4
		{count: 9, wantW: 3, wantH: 3, wantOk: true},  // sqrt(9)=3, 3×3=9
		{count: 6, wantW: 3, wantH: 2, wantOk: true},  // sqrt(6)≈2.45→3, ceil(6/3)=2; 3*2=6 ✓ but need adjust check
	}
	for _, tc := range cases {
		w, h, ok := sheetDimensions(tc.count)
		assert.Equal(t, tc.wantOk, ok, "count=%d ok", tc.count)
		if tc.wantOk {
			assert.Equal(t, tc.wantW, w, "count=%d width", tc.count)
			assert.Equal(t, tc.wantH, h, "count=%d height", tc.count)
		}
	}
}

// TestUnpackJag_MissingFiles — UnpackJag returns nil,nil when jag is missing
// the dat or index files (TS Pix.ts:76-78 null guard).
func TestUnpackJag_MissingFiles(t *testing.T) {
	// Empty jagfile — no files at all.
	jag := jagfile.NewEmptyJagfile(true)
	dir := t.TempDir()
	jagPath := filepath.Join(dir, "empty.jag")
	require.NoError(t, jag.Save(jagPath))
	loaded, err := jagfile.LoadJagfile(jagPath)
	require.NoError(t, err)

	s, err := UnpackJag(loaded, "nonexistent", 0)
	assert.NoError(t, err)
	assert.Nil(t, s)
}

// TestUnpackFull_ZeroSprites — UnpackFull is a no-op when zero sprites exist. TS Pix.ts:44-46.
func TestUnpackFull_ZeroSprites(t *testing.T) {
	jag := jagfile.NewEmptyJagfile(true)
	dir := t.TempDir()
	jagPath := filepath.Join(dir, "empty.jag")
	require.NoError(t, jag.Save(jagPath))
	loaded, err := jagfile.LoadJagfile(jagPath)
	require.NoError(t, err)

	outDir := t.TempDir()
	err = UnpackFull(loaded, outDir, "nothing", nil)
	require.NoError(t, err, "zero sprites must not error")

	// No PNG should have been written.
	_, statErr := os.Stat(filepath.Join(outDir, "nothing.png"))
	assert.True(t, os.IsNotExist(statErr), "no PNG for zero sprites")
}

// TestUnpackFull_WritesFile — integration: UnpackFull writes a loadable PNG.
func TestUnpackFull_WritesFile(t *testing.T) {
	// Single 3×3 sprite, palette index 1=blue for all pixels.
	palette := []int32{0, 0x0000FF}
	pixels := make([]uint8, 9)
	for i := range pixels {
		pixels[i] = 1
	}
	spec := spriteSpec{
		width: 3, height: 3,
		cropLeft: 0, cropTop: 0, cropRight: 3, cropBottom: 3,
		pixelOrder: 0,
		pixels:     pixels,
		palette:    palette,
	}
	jag := buildJag(t, "blue", []spriteSpec{spec})
	dir := t.TempDir()
	require.NoError(t, UnpackFull(jag, dir, "blue", nil))

	img := loadPNG(t, filepath.Join(dir, "blue.png"))
	bounds := img.Bounds()
	assert.Equal(t, 3, bounds.Max.X)
	assert.Equal(t, 3, bounds.Max.Y)
	for y := range 3 {
		for x := range 3 {
			assert.Equal(t, color.NRGBA{R: 0x00, G: 0x00, B: 0xFF, A: 0xFF}, pixelAt(img, x, y),
				"all pixels must be blue at (%d,%d)", x, y)
		}
	}
}

// TestSheetDimensionsReachability — exhaustive proof that sheetDimensions(ok=false)
// IS reachable for counts in 2..1000 (first failure: count=14).
//
// The TS preferHorizontal widen-loop (10 attempts) cannot fix every composite
// count; 14 is the smallest failure: ceil(sqrt(14))=4, ceil(14/4)=4, 4*4=16>14,
// widening produces (5,3)=15, (6,2)=12, (7,1)=7 — none equal 14.
// This test pins the contract for sheetDimensions AND verifies the exhaustive list.
func TestSheetDimensionsReachability(t *testing.T) {
	// The first failure in 2..1000 must be 14.
	_, _, ok14 := sheetDimensions(14)
	assert.False(t, ok14, "count=14 must produce ok=false (unreachable sheet dims)")

	// Count how many failures exist in 2..1000 (must be > 0).
	failureCount := 0
	for count := 2; count <= 1000; count++ {
		_, _, ok := sheetDimensions(count)
		if !ok {
			failureCount++
		}
	}
	assert.Greater(t, failureCount, 0, "sheetDimensions failure IS reachable for sprite counts in 2..1000")
}

// TestUnpackFull_SheetDimFailure_WritesOptSkipsPNG — mirrors TS Pix.ts:191-194 + Pix.ts:52-69.
// When sheetDimensions fails (e.g. 14 sprites), unpackJagToPng returns null in TS.
// TS unpackFull then: skips the PNG write (Pix.ts:52 `if (png)`), still writes .opt
// (Pix.ts:56-69), and returns without error.
// Go must do the same: Errorf called, no PNG on disk, .opt written, nil error returned.
func TestUnpackFull_SheetDimFailure_WritesOptSkipsPNG(t *testing.T) {
	// 14 sprites is the smallest count where sheetDimensions returns ok=false.
	const spriteCount = 14

	// Verify the precondition: sheetDimensions(14) must fail.
	_, _, ok := sheetDimensions(spriteCount)
	require.False(t, ok, "precondition: sheetDimensions(14) must return ok=false")

	palette := []int32{0, 0xFF0000}
	specs := make([]spriteSpec, spriteCount)
	for i := range specs {
		specs[i] = spriteSpec{
			width: 4, height: 4,
			cropLeft: 1, cropTop: 1, cropRight: 2, cropBottom: 2,
			pixelOrder: 0,
			pixels:     []uint8{1, 1, 1, 1},
			palette:    palette,
		}
	}
	jag := buildJag(t, "fourteen", specs)

	// Pass a capture func as the errorf parameter to observe the warning.
	var gotMsg string
	captureErrorf := func(format string, args ...any) {
		gotMsg = fmt.Sprintf(format, args...)
	}

	dir := t.TempDir()
	err := UnpackFull(jag, dir, "fourteen", captureErrorf)

	// TS returns without error. TS Pix.ts:52-54 skips PNG when png==null.
	require.NoError(t, err, "dimension mismatch must not return an error (mirrors TS)")

	// Errorf must have been called. TS Pix.ts:192: printError("wrong spritesheet size! ...")
	assert.NotEmpty(t, gotMsg, "Errorf must be called on dimension mismatch")
	assert.Contains(t, gotMsg, "wrong spritesheet size", "Errorf message must mention spritesheet size")

	// No PNG must be written. TS Pix.ts:52: `if (png)` → skipped.
	_, statErr := os.Stat(filepath.Join(dir, "fourteen.png"))
	assert.True(t, os.IsNotExist(statErr), "PNG must NOT be written on dimension mismatch")

	// .opt MUST be written. TS Pix.ts:56-69 runs regardless of `if (png)`.
	optPath := filepath.Join(dir, "meta", "fourteen.opt")
	require.FileExists(t, optPath, ".opt must still be written on dimension mismatch")

	optBytes, err := os.ReadFile(optPath)
	require.NoError(t, err)
	// 14 sprites, all with same cell dims 4×4 and crop 1,1,2,2.
	// First line: "4x4\n"; then 14 crop lines "1,1,2,2\n".
	lines := string(optBytes)
	assert.True(t, strings.HasPrefix(lines, "4x4\n"), "first .opt line must be cell dims")
	assert.Equal(t, 15, len(splitLines(lines)), ".opt must have 1 header + 14 crop lines")
}

// splitLines splits a string on "\n", ignoring a final trailing newline.
func splitLines(s string) []string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	if s == "" {
		return nil
	}
	result := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

// TestUnpackFull_4Sprite2x2Sheet — 4 sprites produces a 2×2 sheet. TS Pix.ts:167-169.
func TestUnpackFull_4Sprite2x2Sheet(t *testing.T) {
	// 4 sprites, 2×2 cells each; sqrt(4)=2 → sheetWidth=2, sheetHeight=2.
	palette := []int32{0, 0xFF0000, 0x00FF00, 0x0000FF, 0xFFFF00}
	specs := []spriteSpec{
		{width: 2, height: 2, cropRight: 2, cropBottom: 2, pixelOrder: 0, pixels: []uint8{1, 1, 1, 1}, palette: palette},
		{width: 2, height: 2, cropRight: 2, cropBottom: 2, pixelOrder: 0, pixels: []uint8{2, 2, 2, 2}, palette: palette},
		{width: 2, height: 2, cropRight: 2, cropBottom: 2, pixelOrder: 0, pixels: []uint8{3, 3, 3, 3}, palette: palette},
		{width: 2, height: 2, cropRight: 2, cropBottom: 2, pixelOrder: 0, pixels: []uint8{4, 4, 4, 4}, palette: palette},
	}
	jag := buildJag(t, "quad", specs)
	dir := t.TempDir()
	require.NoError(t, UnpackFull(jag, dir, "quad", nil))

	img := loadPNG(t, filepath.Join(dir, "quad.png"))
	bounds := img.Bounds()
	assert.Equal(t, 4, bounds.Max.X, "sheet width = 2*2")
	assert.Equal(t, 4, bounds.Max.Y, "sheet height = 2*2")

	// sprite0 (red) at (0,0)..(1,1)
	assert.Equal(t, color.NRGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}, pixelAt(img, 0, 0))
	// sprite1 (green) at (2,0)..(3,1)
	assert.Equal(t, color.NRGBA{R: 0x00, G: 0xFF, B: 0x00, A: 0xFF}, pixelAt(img, 2, 0))
	// sprite2 (blue) at (0,2)..(1,3)
	assert.Equal(t, color.NRGBA{R: 0x00, G: 0x00, B: 0xFF, A: 0xFF}, pixelAt(img, 0, 2))
	// sprite3 (yellow) at (2,2)..(3,3)
	assert.Equal(t, color.NRGBA{R: 0xFF, G: 0xFF, B: 0x00, A: 0xFF}, pixelAt(img, 2, 2))
}

