// Package sprite — sprite.go implements the sprite-family unpack entry points
// that mirror the TS tools at:
//
//   - tools/unpack/sprite/media.ts    (17 lines, Engine-TS 9aadcec4)
//   - tools/unpack/sprite/textures.ts (19 lines, Engine-TS 9aadcec4)
//   - tools/unpack/sprite/title.ts    (39 lines, Engine-TS 9aadcec4)
//
// Each function reads a Jagfile from the FileStream cache and decodes its
// sprite entries via [pix.UnpackFull] into the appropriate sub-directory of
// SrcDir.
package sprite

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/unpack/internal/pix"
)

// Options holds all inputs for a sprite-family unpack run.
type Options struct {
	// CacheDir is the directory containing main_file_cache.dat/idx0-4.
	CacheDir string

	// SrcDir is the content tree root (BUILD_SRC_DIR in TS). Output files
	// are written to sub-directories of SrcDir.
	SrcDir string

	// Errorf is the console.error / Pix printError sink; nil = no-op.
	Errorf func(format string, args ...any)
}

// Media decodes all sprites from the media Jagfile (cache archive 0 / file 4)
// into <SrcDir>/sprites/.
//
// Mirrors TS tools/unpack/sprite/media.ts:
//
//	const media = new Jagfile(new Packet(cache.read(0, 4)!));
//	fs.mkdirSync(`${BUILD_SRC_DIR}/sprites`, { recursive: true });
//	for (const name of media.fileName) {
//	    Pix.unpackFull(media, path.basename(name, path.extname(name)), `${BUILD_SRC_DIR}/sprites`);
//	}
func Media(opts Options) error {
	cache := filestream.New(opts.CacheDir, false, true)
	defer cache.Close()

	// TS media.ts:11 — cache.read(0, 4)
	data := cache.Read(0, 4, false)
	if data == nil {
		return fmt.Errorf("sprite: Media: no media archive in cache")
	}

	media, err := jagfile.NewJagfile(packet.NewPacket(data))
	if err != nil {
		return fmt.Errorf("sprite: Media: parse jagfile: %w", err)
	}

	// TS media.ts:13 — mkdir sprites/
	spritesDir := filepath.Join(opts.SrcDir, "sprites")
	if err := os.MkdirAll(spritesDir, 0o755); err != nil {
		return fmt.Errorf("sprite: Media: mkdir sprites: %w", err)
	}

	// TS media.ts:15-17 — iterate all jag members; strip extension for name.
	// media.fileName contains "index.dat" too — passing "index" to UnpackFull
	// produces 0 sprites (null return in TS), so it contributes nothing.
	// TS path.basename(name, path.extname(name)) strips the file extension.
	// The name == "" guard below differs from TS only for a hypothetical
	// bare-dot member (".dat"): Go's path.Ext(".dat")==".dat" strips to "",
	// where Node's extname(".dat")=="" keeps it. No real member is bare-dot.
	for _, fullName := range media.FileName {
		name := stripExt(fullName)
		if name == "" {
			continue
		}
		if err := pix.UnpackFull(media, spritesDir, name, "", opts.Errorf); err != nil {
			return fmt.Errorf("sprite: Media: unpack %q: %w", name, err)
		}
	}

	return nil
}

// Textures decodes all texture sprites from the textures Jagfile (cache
// archive 0 / file 6) into <SrcDir>/textures/.
//
// Mirrors TS tools/unpack/sprite/textures.ts:
//
//	const textures = new Jagfile(new Packet(cache.read(0, 6)!));
//	for (let id = 0; id < 50; id++) {
//	    Pix.unpackFull(textures, id.toString(), `${BUILD_SRC_DIR}/textures`, TexturePack.getById(id) || id.toString());
//	}
//
// TexturePack.getById reads pack/texture.pack to map id → name (e.g. 0 → "door").
// The overrideName controls the output file stem; the jag lookup key is id.toString().
func Textures(opts Options) error {
	cache := filestream.New(opts.CacheDir, false, true)
	defer cache.Close()

	// TS textures.ts:11 — cache.read(0, 6)
	data := cache.Read(0, 6, false)
	if data == nil {
		return fmt.Errorf("sprite: Textures: no textures archive in cache")
	}

	textures, err := jagfile.NewJagfile(packet.NewPacket(data))
	if err != nil {
		return fmt.Errorf("sprite: Textures: parse jagfile: %w", err)
	}

	// TS textures.ts:13-15 — mkdir textures/
	texturesDir := filepath.Join(opts.SrcDir, "textures")
	if err := os.MkdirAll(texturesDir, 0o755); err != nil {
		return fmt.Errorf("sprite: Textures: mkdir textures: %w", err)
	}

	// TexturePack is the shared #tools/pack/PackFile.js singleton, constructed
	// empty under 274 suspendAutoReload (TS PackFile.ts:276 @dee467c8), so
	// getById misses for every id and each texture falls back to the numeric
	// id (TS `TexturePack.getById(id) || id.toString()`).
	reg := &pack.Registry{SrcDir: opts.SrcDir, SuspendAutoReload: true}
	texturePack, err := reg.EnsureTexture()
	if err != nil {
		return fmt.Errorf("sprite: Textures: ensure texture pack: %w", err)
	}

	// TS textures.ts:17-19 — loop id 0..49.
	for id := range 50 {
		jagKey := fmt.Sprintf("%d", id)
		outputName := texturePack.GetByID(id)
		if outputName == "" {
			outputName = jagKey
		}
		if err := pix.UnpackFull(textures, texturesDir, jagKey, outputName, opts.Errorf); err != nil {
			return fmt.Errorf("sprite: Textures: unpack id=%d: %w", id, err)
		}
	}

	return nil
}

// Title decodes title-screen assets from the title Jagfile (cache archive 0 /
// file 1) into <SrcDir>/binary/, <SrcDir>/fonts/, and <SrcDir>/title/.
//
// Mirrors TS tools/unpack/sprite/title.ts:
//
//	const title = new Jagfile(new Packet(cache.read(0, 1)!));
//	// title.dat → binary/title.jpg (raw bytes)
//	const fonts = ['b12_full', 'p11_full', 'p12_full', 'q8_full'];
//	for (const name of fonts) { Pix.unpackFull(title, name, `${BUILD_SRC_DIR}/fonts`); }
//	const titleImages = ['logo', 'runes', 'titlebox', 'titlebutton'];
//	for (const name of titleImages) { Pix.unpackFull(title, name, `${BUILD_SRC_DIR}/title`); }
func Title(opts Options) error {
	cache := filestream.New(opts.CacheDir, false, true)
	defer cache.Close()

	// TS title.ts:10 — cache.read(0, 1)
	data := cache.Read(0, 1, false)
	if data == nil {
		return fmt.Errorf("sprite: Title: no title archive in cache")
	}

	title, err := jagfile.NewJagfile(packet.NewPacket(data))
	if err != nil {
		return fmt.Errorf("sprite: Title: parse jagfile: %w", err)
	}

	// TS title.ts:12-22 — mkdir binary/, title/, fonts/
	binaryDir := filepath.Join(opts.SrcDir, "binary")
	if err := os.MkdirAll(binaryDir, 0o755); err != nil {
		return fmt.Errorf("sprite: Title: mkdir binary: %w", err)
	}
	titleDir := filepath.Join(opts.SrcDir, "title")
	if err := os.MkdirAll(titleDir, 0o755); err != nil {
		return fmt.Errorf("sprite: Title: mkdir title: %w", err)
	}
	fontsDir := filepath.Join(opts.SrcDir, "fonts")
	if err := os.MkdirAll(fontsDir, 0o755); err != nil {
		return fmt.Errorf("sprite: Title: mkdir fonts: %w", err)
	}

	// TS title.ts:24-27 — read title.dat and write to binary/title.jpg.
	bgPkt, err := title.Read("title.dat")
	if err == nil && bgPkt != nil {
		jpgPath := filepath.Join(binaryDir, "title.jpg")
		if err := os.WriteFile(jpgPath, bgPkt.Data, 0o644); err != nil {
			return fmt.Errorf("sprite: Title: write title.jpg: %w", err)
		}
	}

	// TS title.ts:29-33 @dee467c8 — unpack fonts b12_full/p11_full/p12_full/q8_full
	// into fonts/.  The rev-274 cache stores the title fonts under the "_full"
	// member names (the pack-side rename); the member key doubles as the output
	// filename, so the .png stems are also "_full".
	fonts := []string{"b12_full", "p11_full", "p12_full", "q8_full"}
	for _, name := range fonts {
		if err := pix.UnpackFull(title, fontsDir, name, "", opts.Errorf); err != nil {
			return fmt.Errorf("sprite: Title: unpack font %q: %w", name, err)
		}
	}

	// TS title.ts:35-39 — unpack title images logo/runes/titlebox/titlebutton into title/
	titleImages := []string{"logo", "runes", "titlebox", "titlebutton"}
	for _, name := range titleImages {
		if err := pix.UnpackFull(title, titleDir, name, "", opts.Errorf); err != nil {
			return fmt.Errorf("sprite: Title: unpack %q: %w", name, err)
		}
	}

	return nil
}

// stripExt strips the file extension from name (e.g. "chatback.dat" → "chatback").
// Mirrors TS path.basename(name, path.extname(name)).
func stripExt(name string) string {
	base := path.Base(name)
	ext := path.Ext(base)
	if ext == "" {
		return base
	}
	return base[:len(base)-len(ext)]
}
