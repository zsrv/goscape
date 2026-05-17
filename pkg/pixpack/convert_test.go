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
