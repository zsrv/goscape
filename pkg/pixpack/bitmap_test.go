package pixpack

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodePNG_2x2(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.png")
	writeTestPNG(t, path, 2, 2, []color.RGBA{
		{255, 0, 0, 255}, {0, 255, 0, 255},
		{0, 0, 255, 255}, {255, 255, 0, 255},
	})

	bm, err := decodePNG(path)
	if err != nil {
		t.Fatalf("decodePNG: %v", err)
	}
	if bm.Width != 2 || bm.Height != 2 {
		t.Errorf("Width=%d Height=%d, want 2,2", bm.Width, bm.Height)
	}
	if bm.Data[0] != 255 || bm.Data[1] != 0 || bm.Data[2] != 0 {
		t.Errorf("(0,0) RGB = %d,%d,%d, want 255,0,0", bm.Data[0], bm.Data[1], bm.Data[2])
	}
	pos11 := (1 + 1*2) * 4
	if bm.Data[pos11+0] != 255 || bm.Data[pos11+1] != 255 || bm.Data[pos11+2] != 0 {
		t.Errorf("(1,1) RGB = %d,%d,%d, want 255,255,0",
			bm.Data[pos11+0], bm.Data[pos11+1], bm.Data[pos11+2])
	}
}

// writeTestPNG writes an RGBA PNG to path with the given pixels (row-major).
func writeTestPNG(t *testing.T, path string, w, h int, pixels []color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i, p := range pixels {
		x := i % w
		y := i / w
		img.Set(x, y, p)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
