package pixpack

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// solid builds a w x h bitmap where every pixel is the same colour.
func solid(w, h int, r, g, b uint8) *Bitmap {
	bm := &Bitmap{Width: w, Height: h, Data: make([]uint8, w*h*4)}
	for i := range w * h {
		bm.Data[i*4+0] = r
		bm.Data[i*4+1] = g
		bm.Data[i*4+2] = b
		bm.Data[i*4+3] = 0xff
	}
	return bm
}

// setPx writes one pixel of bm.
func setPx(bm *Bitmap, x, y int, r, g, b uint8) {
	pos := (x + y*bm.Width) * 4
	bm.Data[pos+0] = r
	bm.Data[pos+1] = g
	bm.Data[pos+2] = b
	bm.Data[pos+3] = 0xff
}

// TestGetPixelBounds_AllMagentaIsFullFrame pins the TS `right === -1`
// fallback: an image with no opaque pixel keeps the full frame rather than
// collapsing to an inverted rectangle (PixPack.ts:78 @1d25566c).
func TestGetPixelBounds_AllMagentaIsFullFrame(t *testing.T) {
	bm := solid(4, 3, 0xff, 0x00, 0xff)
	got := GetPixelBounds(bm)
	want := PixelBounds{Left: 0, Top: 0, Width: 4, Height: 3}
	if got != want {
		t.Errorf("GetPixelBounds: got %+v, want %+v", got, want)
	}
}

// TestGetPixelBounds_TrimsMagentaBorder pins the scan: bounds tighten to the
// non-magenta extent, inclusive on both ends (PixPack.ts:62-81 @1d25566c).
// This is what replaced the per-sprite crop rows in the .opt sidecar.
func TestGetPixelBounds_TrimsMagentaBorder(t *testing.T) {
	bm := solid(5, 4, 0xff, 0x00, 0xff)
	setPx(bm, 1, 1, 10, 20, 30)
	setPx(bm, 3, 2, 40, 50, 60)

	got := GetPixelBounds(bm)
	want := PixelBounds{Left: 1, Top: 1, Width: 3, Height: 2}
	if got != want {
		t.Errorf("GetPixelBounds: got %+v, want %+v", got, want)
	}
}

// TestGetPixelBounds_SinglePixel pins the inclusive arithmetic: one opaque
// pixel yields a 1x1 rectangle, not 0x0.
func TestGetPixelBounds_SinglePixel(t *testing.T) {
	bm := solid(4, 4, 0xff, 0x00, 0xff)
	setPx(bm, 2, 3, 1, 1, 1)

	got := GetPixelBounds(bm)
	want := PixelBounds{Left: 2, Top: 3, Width: 1, Height: 1}
	if got != want {
		t.Errorf("GetPixelBounds: got %+v, want %+v", got, want)
	}
}

// TestGeneratePixelOrder_PrefersFewerTransitions pins both halves of the
// 8139461a rewrite: the metric is a colour-transition count (not a signed
// delta sum) and the polarity is `columnTransitions < rowTransitions ? 1 : 0`.
//
// The bitmap has uniform columns and striped rows, so scanning column-major
// sees no transitions while row-major sees one per step — column-major (1)
// must win. A build that flipped only the metric, or only the polarity, would
// return 0 here.
func TestGeneratePixelOrder_PrefersFewerTransitions(t *testing.T) {
	bm := solid(4, 4, 0, 0, 0)
	for y := range 4 {
		for x := range 4 {
			setPx(bm, x, y, uint8(x*10), 0, 0) // colour depends on x only
		}
	}
	if got := GeneratePixelOrder(bm, 0, 0, 4, 4); got != 1 {
		t.Errorf("column-uniform bitmap: got pixel order %d, want 1 (column-major)", got)
	}
}

// TestGeneratePixelOrder_RowUniformPicksRowMajor is the mirror case: colour
// depends on y only, so row-major traversal is the quiet one and 0 must win.
func TestGeneratePixelOrder_RowUniformPicksRowMajor(t *testing.T) {
	bm := solid(4, 4, 0, 0, 0)
	for y := range 4 {
		for x := range 4 {
			setPx(bm, x, y, uint8(y*10), 0, 0) // colour depends on y only
		}
	}
	if got := GeneratePixelOrder(bm, 0, 0, 4, 4); got != 0 {
		t.Errorf("row-uniform bitmap: got pixel order %d, want 0 (row-major)", got)
	}
}

// TestGeneratePixelOrder_TieGoesToRowMajor pins the non-strict comparison: a
// constant-colour bitmap scores 0 transitions both ways, and
// `columnTransitions < rowTransitions` is false, so row-major (0) wins.
func TestGeneratePixelOrder_TieGoesToRowMajor(t *testing.T) {
	bm := solid(4, 4, 100, 100, 100)
	if got := GeneratePixelOrder(bm, 0, 0, 4, 4); got != 0 {
		t.Errorf("constant-colour bitmap: got pixel order %d, want 0 (tie -> row-major)", got)
	}
}

// TestGeneratePixelOrder_RespectsCropRectangle pins that the scan is confined
// to the (left, top, width, height) rectangle, not the whole bitmap: the noisy
// border outside the rectangle must not influence the decision.
func TestGeneratePixelOrder_RespectsCropRectangle(t *testing.T) {
	bm := solid(6, 6, 0, 0, 0)
	// Noisy border.
	for i := range 6 {
		setPx(bm, i, 0, uint8(i*40), 0, 0)
		setPx(bm, 0, i, 0, uint8(i*40), 0)
		setPx(bm, i, 5, uint8(i*7), 9, 9)
		setPx(bm, 5, i, 9, uint8(i*7), 9)
	}
	// Interior 4x4 at (1,1) is column-uniform.
	for y := 1; y < 5; y++ {
		for x := 1; x < 5; x++ {
			setPx(bm, x, y, uint8(x*10), 0, 0)
		}
	}
	if got := GeneratePixelOrder(bm, 1, 1, 4, 4); got != 1 {
		t.Errorf("cropped column-uniform interior: got pixel order %d, want 1", got)
	}
}

// TestWriteImage_RowMajorHeaderAndPayload pins the emitted header
// (left, top, width, height, pixelOrder) and the row-major payload walk
// (PixPack.ts:83-97 @1d25566c).
func TestWriteImage_RowMajorHeaderAndPayload(t *testing.T) {
	// 2x1, both pixels opaque and distinct -> bounds are the full frame.
	// Row traversal sees 1 transition, column traversal also 1 (width*height
	// is 2 either way), so the tie goes to row-major.
	bm := &Bitmap{Width: 2, Height: 1, Data: []uint8{1, 2, 3, 0xff, 4, 5, 6, 0xff}}
	colors := []int32{0xff00ff, 0x010203, 0x040506}

	data := packet.Alloc(16)
	index := packet.Alloc(16)
	defer data.Release()
	defer index.Release()

	WriteImage(bm, data, index, colors)

	// left=0, top=0, width=2 (p2), height=1 (p2), pixelOrder=0
	wantIdx := []byte{0, 0, 0, 2, 0, 1, 0}
	if string(index.Data) != string(wantIdx) {
		t.Errorf("index = %v, want %v", index.Data, wantIdx)
	}
	// Palette indices of the two pixels, in row order.
	wantData := []byte{1, 2}
	if string(data.Data) != string(wantData) {
		t.Errorf("data = %v, want %v", data.Data, wantData)
	}
}

// TestWriteImage_CropsToBounds pins that the payload covers only the scanned
// rectangle: the magenta border is excluded from both the header and the
// emitted pixels.
func TestWriteImage_CropsToBounds(t *testing.T) {
	bm := solid(4, 3, 0xff, 0x00, 0xff)
	setPx(bm, 1, 1, 10, 20, 30)
	setPx(bm, 2, 1, 40, 50, 60)
	colors := []int32{0xff00ff, 0x0a141e, 0x28323c}

	data := packet.Alloc(32)
	index := packet.Alloc(32)
	defer data.Release()
	defer index.Release()

	WriteImage(bm, data, index, colors)

	// Bounds are (1,1) 2x1.
	wantIdx := []byte{1, 1, 0, 2, 0, 1, 0}
	if string(index.Data) != string(wantIdx) {
		t.Errorf("index = %v, want %v", index.Data, wantIdx)
	}
	if len(data.Data) != 2 {
		t.Fatalf("data length = %d, want 2 (cropped region only)", len(data.Data))
	}
	if data.Data[0] != 1 || data.Data[1] != 2 {
		t.Errorf("data = %v, want [1 2]", data.Data)
	}
}

// TestWriteImage_UnknownColorWritesFF pins the loss of the early exit.
// Pre-8139461a the walk broke out when a colour was missing from the palette;
// TS now writes colors.indexOf(rgb) unconditionally, so -1 lands as 0xff and
// the frame keeps its full length (PixPack.ts:96 @1d25566c).
func TestWriteImage_UnknownColorWritesFF(t *testing.T) {
	bm := &Bitmap{Width: 2, Height: 1, Data: []uint8{1, 2, 3, 0xff, 9, 9, 9, 0xff}}
	colors := []int32{0xff00ff, 0x010203} // 0x090909 deliberately absent

	data := packet.Alloc(16)
	index := packet.Alloc(16)
	defer data.Release()
	defer index.Release()

	WriteImage(bm, data, index, colors)

	if len(data.Data) != 2 {
		t.Fatalf("data length = %d, want 2 (no early exit)", len(data.Data))
	}
	if data.Data[0] != 1 {
		t.Errorf("data[0] = %d, want 1", data.Data[0])
	}
	if data.Data[1] != 0xff {
		t.Errorf("data[1] = %d, want 255 (indexOf -1 written as a byte)", data.Data[1])
	}
}
