package sprites

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/pack"
)

// writePNG creates a w*h PNG full of color c at path.
func writePNG(t *testing.T, path string, w, h int, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestPackTitle_AllArtifactsPresent(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	for _, p := range []string{"title", "fonts", "binary"} {
		if err := os.MkdirAll(filepath.Join(src, p), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	for _, name := range []string{"logo", "runes", "titlebox", "titlebutton"} {
		writePNG(t, filepath.Join(src, "title", name+".png"), 2, 2, color.RGBA{1, 2, 3, 255})
	}
	for _, name := range []string{"b12", "p11", "p12", "q8"} {
		writePNG(t, filepath.Join(src, "fonts", name+".png"), 2, 2, color.RGBA{4, 5, 6, 255})
	}
	if err := os.WriteFile(filepath.Join(src, "binary", "title.jpg"), []byte("jpg-bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out := filepath.Join(tmp, "out")
	if err := PackTitle(src, out, nil); err != nil {
		t.Fatalf("PackTitle: %v", err)
	}
	jag, err := jagfile.LoadJagfile(filepath.Join(out, "client", "title"))
	if err != nil {
		t.Fatalf("LoadJagfile: %v", err)
	}
	for _, name := range []string{
		"title.dat", "index.dat",
		"logo.dat", "runes.dat", "titlebox.dat", "titlebutton.dat",
		"b12.dat", "p11.dat", "p12.dat", "q8.dat",
	} {
		if _, err := jag.Read(name); err != nil {
			t.Errorf("Read %q: %v", name, err)
		}
	}
}

func TestPackTitle_WritesToCache(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	for _, p := range []string{"title", "fonts", "binary"} {
		_ = os.MkdirAll(filepath.Join(src, p), 0o755)
	}
	for _, name := range []string{"logo", "runes", "titlebox", "titlebutton"} {
		writePNG(t, filepath.Join(src, "title", name+".png"), 2, 2, color.RGBA{1, 2, 3, 255})
	}
	for _, name := range []string{"b12", "p11", "p12", "q8"} {
		writePNG(t, filepath.Join(src, "fonts", name+".png"), 2, 2, color.RGBA{4, 5, 6, 255})
	}
	_ = os.WriteFile(filepath.Join(src, "binary", "title.jpg"), []byte("jpg-bytes"), 0o644)

	out := filepath.Join(tmp, "out")
	cacheDir := t.TempDir()
	fs := filestream.New(cacheDir, true, false)
	defer fs.Close()

	if err := PackTitle(src, out, fs); err != nil {
		t.Fatalf("PackTitle: %v", err)
	}
	if !fs.Has(0, 1) {
		t.Fatal("cache(0,1) has no entry after PackTitle")
	}
}

func TestPackMedia_SortsSpritesheetsLast(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	spritesDir := filepath.Join(src, "sprites")
	metaDir := filepath.Join(spritesDir, "meta")
	_ = os.MkdirAll(metaDir, 0o755)

	writePNG(t, filepath.Join(spritesDir, "plain.png"), 2, 2, color.RGBA{1, 2, 3, 255})
	writePNG(t, filepath.Join(spritesDir, "sheet.png"), 2, 2, color.RGBA{4, 5, 6, 255})
	// Single-sprite .opt — triggers len(sprites)==1 branch and marks
	// "sheet" as a spritesheet for the sort-last comparator.
	_ = os.WriteFile(filepath.Join(metaDir, "sheet.opt"), []byte("0,0,2,2,row\n"), 0o644)

	out := filepath.Join(tmp, "out")
	if err := PackMedia(src, out, nil); err != nil {
		t.Fatalf("PackMedia: %v", err)
	}
	jag, err := jagfile.LoadJagfile(filepath.Join(out, "client", "media"))
	if err != nil {
		t.Fatalf("LoadJagfile: %v", err)
	}
	if _, err := jag.Read("plain.dat"); err != nil {
		t.Errorf("Read plain.dat: %v", err)
	}
	if _, err := jag.Read("sheet.dat"); err != nil {
		t.Errorf("Read sheet.dat: %v", err)
	}
}

func TestPackMedia_WritesToCache(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	spritesDir := filepath.Join(src, "sprites")
	_ = os.MkdirAll(spritesDir, 0o755)
	writePNG(t, filepath.Join(spritesDir, "icon.png"), 2, 2, color.RGBA{1, 2, 3, 255})

	out := filepath.Join(tmp, "out")
	cacheDir := t.TempDir()
	fs := filestream.New(cacheDir, true, false)
	defer fs.Close()

	if err := PackMedia(src, out, fs); err != nil {
		t.Fatalf("PackMedia: %v", err)
	}
	if !fs.Has(0, 4) {
		t.Fatal("cache(0,4) has no entry after PackMedia")
	}
}

func TestPackTexture_FixedLoop(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	texDir := filepath.Join(src, "textures")
	packDir := filepath.Join(src, "pack")
	_ = os.MkdirAll(texDir, 0o755)
	_ = os.MkdirAll(packDir, 0o755)
	_ = os.WriteFile(filepath.Join(packDir, "texture.pack"), []byte("0=t0\n"), 0o644)
	writePNG(t, filepath.Join(texDir, "t0.png"), 2, 2, color.RGBA{1, 2, 3, 255})

	reg := &pack.Registry{SrcDir: src}
	if _, err := reg.EnsureTexture(); err != nil {
		t.Fatalf("EnsureTexture: %v", err)
	}

	out := filepath.Join(tmp, "out")
	if err := PackTexture(reg, src, out, nil); err != nil {
		t.Fatalf("PackTexture: %v", err)
	}
	jag, err := jagfile.LoadJagfile(filepath.Join(out, "client", "textures"))
	if err != nil {
		t.Fatalf("LoadJagfile: %v", err)
	}
	if _, err := jag.Read("0.dat"); err != nil {
		t.Errorf("Read 0.dat: %v", err)
	}
	if _, err := jag.Read("index.dat"); err != nil {
		t.Errorf("Read index.dat: %v", err)
	}
}

func TestPackTexture_WritesToCache(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	texDir := filepath.Join(src, "textures")
	packDir := filepath.Join(src, "pack")
	_ = os.MkdirAll(texDir, 0o755)
	_ = os.MkdirAll(packDir, 0o755)
	_ = os.WriteFile(filepath.Join(packDir, "texture.pack"), []byte("0=t0\n"), 0o644)
	writePNG(t, filepath.Join(texDir, "t0.png"), 2, 2, color.RGBA{1, 2, 3, 255})

	reg := &pack.Registry{SrcDir: src}
	if _, err := reg.EnsureTexture(); err != nil {
		t.Fatalf("EnsureTexture: %v", err)
	}

	out := filepath.Join(tmp, "out")
	cacheDir := t.TempDir()
	fs := filestream.New(cacheDir, true, false)
	defer fs.Close()

	if err := PackTexture(reg, src, out, fs); err != nil {
		t.Fatalf("PackTexture: %v", err)
	}
	if !fs.Has(0, 6) {
		t.Fatal("cache(0,6) has no entry after PackTexture")
	}
}
