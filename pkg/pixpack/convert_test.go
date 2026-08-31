package pixpack

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func TestConvertImage_NoMeta_2x2(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.png")
	writeTestPNG(t, path, 2, 2, []color.RGBA{
		{1, 2, 3, 255}, {4, 5, 6, 255},
		{7, 8, 9, 255}, {10, 11, 12, 255},
	})

	index := packet.Alloc(64)
	defer index.Release()

	data, err := ConvertImage(index, tmp, "test")
	if err != nil {
		t.Fatalf("ConvertImage: %v", err)
	}
	defer data.Release()

	if data.Length() < 2 {
		t.Fatalf("data len=%d, want >=2", data.Length())
	}
	idxBytes := index.Data
	if idxBytes[0] != 0 || idxBytes[1] != 2 || idxBytes[2] != 0 || idxBytes[3] != 2 {
		t.Errorf("index[0..4] = %v, want [0 2 0 2]", idxBytes[:4])
	}
	if idxBytes[4] != 5 {
		t.Errorf("index[4]=%d, want 5 (sentinel + 4 colors)", idxBytes[4])
	}
}

// TestConvertImage_PalPngIgnored pins the rev-254 contract: TS PixPack.ts
// @2e3bcf43 removed the meta/<name>.pal.png palette workaround (rev-244
// PixPack.ts:185-192 @9aadcec4), so a stray pal.png must be IGNORED and the
// palette derived from the source image. The test verifies that the index
// palette count (byte 4 of the index packet) reflects the source image
// colors even when a pal.png with a different palette is present.
func TestConvertImage_PalPngIgnored(t *testing.T) {
	tmp := t.TempDir()
	metaDir := filepath.Join(tmp, "meta")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Source image: 2x2 with 4 distinct colors.
	writeTestPNG(t, filepath.Join(tmp, "src.png"), 2, 2, []color.RGBA{
		{1, 2, 3, 255}, {4, 5, 6, 255},
		{7, 8, 9, 255}, {10, 11, 12, 255},
	})
	// pal.png with a 1-color palette — pre-254 this would shrink the
	// palette to [sentinel, color] = 2 entries; at 254 it must be ignored.
	writeTestPNG(t, filepath.Join(metaDir, "src.pal.png"), 1, 1, []color.RGBA{
		{0x11, 0x22, 0x33, 255},
	})

	index := packet.Alloc(64)
	defer index.Release()

	data, err := ConvertImage(index, tmp, "src")
	if err != nil {
		t.Fatalf("ConvertImage: %v", err)
	}
	defer data.Release()

	// index[4] = palette length = 5 (sentinel + the 4 source colors).
	// The pre-254 pal.png workaround would have yielded 2 here.
	idxBytes := index.Data
	if idxBytes[4] != 5 {
		t.Errorf("index[4]=%d, want 5 (palette from source image; pal.png ignored at 254)", idxBytes[4])
	}
}

