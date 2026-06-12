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

func TestLoadSpriteMeta_MissingFileReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	sprites, tx, ty, err := loadSpriteMeta(tmp, "nonexistent", 32, 64)
	if err != nil {
		t.Fatalf("err=%v, want nil", err)
	}
	if sprites != nil {
		t.Errorf("sprites=%v, want nil", sprites)
	}
	if tx != 32 || ty != 64 {
		t.Errorf("tileX,Y = %d,%d, want 32,64", tx, ty)
	}
}

func TestLoadSpriteMeta_SingleSprite(t *testing.T) {
	tmp := t.TempDir()
	metaDir := filepath.Join(tmp, "meta")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "x.opt"), []byte("1,2,3,4,row\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	sprites, _, _, err := loadSpriteMeta(tmp, "x", 0, 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(sprites) != 1 {
		t.Fatalf("len(sprites)=%d, want 1", len(sprites))
	}
	got := sprites[0]
	want := SpriteMeta{X: 1, Y: 2, W: 3, H: 4, PixelOrder: 1}
	if got != want {
		t.Errorf("sprite = %+v, want %+v", got, want)
	}
}

func TestLoadSpriteMeta_TiledSheet(t *testing.T) {
	tmp := t.TempDir()
	metaDir := filepath.Join(tmp, "meta")
	_ = os.MkdirAll(metaDir, 0o755)
	body := "16x16\n0,0,16,16,col\n0,0,16,16,row\n"
	_ = os.WriteFile(filepath.Join(metaDir, "sheet.opt"), []byte(body), 0o644)

	sprites, tx, ty, err := loadSpriteMeta(tmp, "sheet", 0, 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if tx != 16 || ty != 16 {
		t.Errorf("tileX,Y = %d,%d, want 16,16", tx, ty)
	}
	if len(sprites) != 2 {
		t.Fatalf("len(sprites)=%d, want 2", len(sprites))
	}
	if sprites[0].PixelOrder != 0 || sprites[1].PixelOrder != 1 {
		t.Errorf("pixelOrders = %d,%d, want 0,1", sprites[0].PixelOrder, sprites[1].PixelOrder)
	}
}

// TestParseSpriteLine_FourFieldsAccepted pins TS PixPack.ts:154-162 (9aadcec4):
// sprite[4] is undefined for 4-field lines → undefined !== 'row' → pixelOrder=0,
// no error. Real 244 content (title/meta/runes.opt) has lines like "10,11,74,74".
func TestParseSpriteLine_FourFieldsAccepted(t *testing.T) {
	s, err := parseSpriteLine("10,11,74,74")
	if err != nil {
		t.Fatalf("parseSpriteLine 4-field: unexpected error: %v", err)
	}
	want := SpriteMeta{X: 10, Y: 11, W: 74, H: 74, PixelOrder: 0}
	if s != want {
		t.Errorf("got %+v, want %+v", s, want)
	}
}

// TestParseSpriteLine_FiveFieldRow pins that "row" in field 5 → pixelOrder=1.
func TestParseSpriteLine_FiveFieldRow(t *testing.T) {
	s, err := parseSpriteLine("1,2,3,4,row")
	if err != nil {
		t.Fatalf("parseSpriteLine 5-field row: unexpected error: %v", err)
	}
	if s.PixelOrder != 1 {
		t.Errorf("pixelOrder = %d, want 1", s.PixelOrder)
	}
}

// TestParseSpriteLine_FiveFieldOther pins that non-"row" field 5 → pixelOrder=0.
func TestParseSpriteLine_FiveFieldOther(t *testing.T) {
	s, err := parseSpriteLine("1,2,3,4,col")
	if err != nil {
		t.Fatalf("parseSpriteLine 5-field col: unexpected error: %v", err)
	}
	if s.PixelOrder != 0 {
		t.Errorf("pixelOrder = %d, want 0", s.PixelOrder)
	}
}

// TestParseSpriteLine_TooFewFieldsErrors pins the defensive Go deviation:
// <4 fields return an error (TS silently uses NaN, Go guards instead).
func TestParseSpriteLine_TooFewFieldsErrors(t *testing.T) {
	_, err := parseSpriteLine("1,2,3")
	if err == nil {
		t.Fatal("parseSpriteLine 3-field: want error, got nil")
	}
}
