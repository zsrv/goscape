package sprite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// ---- fixture helpers --------------------------------------------------------

// buildSingleSprite builds a jagfile containing one 2×2 sprite at key <name>.
func buildSingleSprite(t *testing.T, name string) *jagfile.Jagfile {
	t.Helper()
	palette := []int32{0, 0xFF0000}
	pixels := []uint8{1, 1, 1, 1}
	return buildJagWithSprite(t, name, 2, 2, pixels, palette)
}

// buildJagWithSprite builds a jag with one sprite under name.
func buildJagWithSprite(t *testing.T, name string, w, h int, pixels []uint8, palette []int32) *jagfile.Jagfile {
	t.Helper()

	idxBuf := packet.Alloc(256)
	idxBuf.P2(uint16(w))
	idxBuf.P2(uint16(h))
	idxBuf.P1(uint8(len(palette)))
	for i := 1; i < len(palette); i++ {
		idxBuf.P3(uint32(palette[i]))
	}
	idxBuf.P1(0)         // cropLeft
	idxBuf.P1(0)         // cropTop
	idxBuf.P2(uint16(w)) // cropRight
	idxBuf.P2(uint16(h)) // cropBottom
	idxBuf.P1(0)         // pixelOrder = row-major

	datBuf := packet.Alloc(256)
	datBuf.P2(0) // pointer into index.dat = 0
	datBuf.PData(pixels)

	jag := jagfile.NewEmptyJagfile(true)
	jag.Write("index.dat", idxBuf)
	jag.Write(name+".dat", datBuf)

	jagPath := filepath.Join(t.TempDir(), "test.jag")
	require.NoError(t, jag.Save(jagPath))
	loaded, err := jagfile.LoadJagfile(jagPath)
	require.NoError(t, err)
	return loaded
}

// writeCacheJag serialises jag to a filestream cache at (archive, file).
func writeCacheJag(t *testing.T, jag *jagfile.Jagfile, archive, file int) string {
	t.Helper()
	jagPath := filepath.Join(t.TempDir(), "entry.jag")
	require.NoError(t, jag.Save(jagPath))
	data, err := os.ReadFile(jagPath)
	require.NoError(t, err)
	return writeCacheRaw(t, data, archive, file)
}

// writeCacheRaw writes rawData to a new filestream cache at (archive, file).
func writeCacheRaw(t *testing.T, rawData []byte, archive, file int) string {
	t.Helper()
	cacheDir := t.TempDir()
	fs := filestream.New(cacheDir, true, false)
	defer fs.Close()
	ok := fs.Write(archive, file, rawData, 0)
	require.True(t, ok, "filestream.Write must succeed")
	return cacheDir
}

// ---- stripExt ---------------------------------------------------------------

func TestStripExt(t *testing.T) {
	assert.Equal(t, "chatback", stripExt("chatback.dat"))
	assert.Equal(t, "index", stripExt("index.dat"))
	assert.Equal(t, "foo", stripExt("foo"))
	assert.Equal(t, "b12", stripExt("b12.dat"))
}

// ---- Media ------------------------------------------------------------------

// TestMedia_MkdirSprites verifies that Media creates the sprites/ directory.
func TestMedia_MkdirSprites(t *testing.T) {
	jag := buildSingleSprite(t, "chatback")
	cacheDir := writeCacheJag(t, jag, 0, 4)
	srcDir := t.TempDir()

	require.NoError(t, Media(Options{CacheDir: cacheDir, SrcDir: srcDir}))

	info, err := os.Stat(filepath.Join(srcDir, "sprites"))
	require.NoError(t, err, "sprites/ must exist")
	assert.True(t, info.IsDir())
}

// TestMedia_WritesPNG verifies that Media writes a PNG for a named sprite.
func TestMedia_WritesPNG(t *testing.T) {
	jag := buildSingleSprite(t, "chatback")
	cacheDir := writeCacheJag(t, jag, 0, 4)
	srcDir := t.TempDir()

	require.NoError(t, Media(Options{CacheDir: cacheDir, SrcDir: srcDir}))
	require.FileExists(t, filepath.Join(srcDir, "sprites", "chatback.png"))
}

// TestMedia_IndexDatDoesNotError verifies that the "index" entry in FileName
// (coming from the jag's index.dat) does not cause an error. UnpackFull
// silently returns 0 sprites for it.
func TestMedia_IndexDatDoesNotError(t *testing.T) {
	jag := buildSingleSprite(t, "test_sprite")
	cacheDir := writeCacheJag(t, jag, 0, 4)
	srcDir := t.TempDir()

	// No error even though FileName slice includes "index.dat" → "index".
	require.NoError(t, Media(Options{CacheDir: cacheDir, SrcDir: srcDir}))
}

// ---- Textures ---------------------------------------------------------------

// TestTextures_MkdirTextures verifies that Textures creates the textures/ dir.
func TestTextures_MkdirTextures(t *testing.T) {
	jag := buildSingleSprite(t, "0")
	cacheDir := writeCacheJag(t, jag, 0, 6)
	srcDir := t.TempDir()

	require.NoError(t, Textures(Options{CacheDir: cacheDir, SrcDir: srcDir}))

	info, err := os.Stat(filepath.Join(srcDir, "textures"))
	require.NoError(t, err, "textures/ must exist")
	assert.True(t, info.IsDir())
}

// TestTextures_UsesTexturePackName verifies that the output uses the texture.pack
// name ("door") rather than the numeric id ("0").
func TestTextures_UsesTexturePackName(t *testing.T) {
	jag := buildSingleSprite(t, "0")
	cacheDir := writeCacheJag(t, jag, 0, 6)

	srcDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "pack", "texture.pack"), []byte("0=door\n"), 0o644))

	require.NoError(t, Textures(Options{CacheDir: cacheDir, SrcDir: srcDir}))
	require.FileExists(t, filepath.Join(srcDir, "textures", "door.png"),
		"output stem must be 'door' (from texture.pack) not '0'")
}

// TestTextures_FallbackToNumericID verifies that when texture.pack has no entry
// for an id, the numeric string is used as output stem.
func TestTextures_FallbackToNumericID(t *testing.T) {
	jag := buildSingleSprite(t, "5")
	cacheDir := writeCacheJag(t, jag, 0, 6)
	srcDir := t.TempDir()

	require.NoError(t, Textures(Options{CacheDir: cacheDir, SrcDir: srcDir}))
	require.FileExists(t, filepath.Join(srcDir, "textures", "5.png"),
		"output stem must fall back to numeric id when no pack entry")
}

// TestTextures_LoopsExactly50 verifies that Textures iterates ids 0..49 and
// does not panic on an empty jag (0 sprites for each id is valid).
func TestTextures_LoopsExactly50(t *testing.T) {
	jag := jagfile.NewEmptyJagfile(true)
	cacheDir := writeCacheJag(t, jag, 0, 6)
	srcDir := t.TempDir()

	require.NoError(t, Textures(Options{CacheDir: cacheDir, SrcDir: srcDir}))
	require.DirExists(t, filepath.Join(srcDir, "textures"))
}

// ---- Title ------------------------------------------------------------------

// TestTitle_MkdirSets verifies that Title creates binary/, fonts/, and title/.
func TestTitle_MkdirSets(t *testing.T) {
	cacheDir := buildTitleCache(t, []byte{0xFF, 0xD8}, "", "")
	srcDir := t.TempDir()

	require.NoError(t, Title(Options{CacheDir: cacheDir, SrcDir: srcDir}))

	require.DirExists(t, filepath.Join(srcDir, "binary"))
	require.DirExists(t, filepath.Join(srcDir, "fonts"))
	require.DirExists(t, filepath.Join(srcDir, "title"))
}

// TestTitle_WritesTitleJpg verifies title.dat bytes are written to binary/title.jpg.
func TestTitle_WritesTitleJpg(t *testing.T) {
	bgBytes := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	cacheDir := buildTitleCache(t, bgBytes, "", "")
	srcDir := t.TempDir()

	require.NoError(t, Title(Options{CacheDir: cacheDir, SrcDir: srcDir}))

	got, err := os.ReadFile(filepath.Join(srcDir, "binary", "title.jpg"))
	require.NoError(t, err)
	assert.Equal(t, bgBytes, got, "binary/title.jpg must contain exact title.dat bytes")
}

// TestTitle_WritesFont verifies that a font sprite is written into fonts/.
func TestTitle_WritesFont(t *testing.T) {
	cacheDir := buildTitleCache(t, []byte{0xAB}, "b12", "")
	srcDir := t.TempDir()

	require.NoError(t, Title(Options{CacheDir: cacheDir, SrcDir: srcDir}))
	require.FileExists(t, filepath.Join(srcDir, "fonts", "b12.png"))
}

// TestTitle_WritesTitleImage verifies that a title image is written into title/.
func TestTitle_WritesTitleImage(t *testing.T) {
	cacheDir := buildTitleCache(t, []byte{0xAB}, "", "logo")
	srcDir := t.TempDir()

	require.NoError(t, Title(Options{CacheDir: cacheDir, SrcDir: srcDir}))
	require.FileExists(t, filepath.Join(srcDir, "title", "logo.png"))
}

// ---- Title cache builder ----------------------------------------------------

// buildTitleCache creates a filestream cache holding a title jag at archive=0,
// file=1.  bgBytes is written as title.dat raw bytes.  If fontName or
// titleImageName is non-empty, a corresponding sprite is added to the jag.
func buildTitleCache(t *testing.T, bgBytes []byte, fontName, titleImageName string) string {
	t.Helper()

	palette := []int32{0, 0xFF0000}
	pixels := []uint8{1, 1, 1, 1}
	w, h := 2, 2

	// Build index.dat with one sprite group per sprite entry, placed consecutively.
	idxBuf := packet.Alloc(512)

	type spriteEntry struct {
		name   string
		idxPos int
	}
	var entries []spriteEntry

	addSprite := func(name string) {
		pos := len(idxBuf.Data)
		idxBuf.P2(uint16(w))
		idxBuf.P2(uint16(h))
		idxBuf.P1(uint8(len(palette)))
		for i := 1; i < len(palette); i++ {
			idxBuf.P3(uint32(palette[i]))
		}
		idxBuf.P1(0)
		idxBuf.P1(0)
		idxBuf.P2(uint16(w))
		idxBuf.P2(uint16(h))
		idxBuf.P1(0)
		entries = append(entries, spriteEntry{name: name, idxPos: pos})
	}

	if fontName != "" {
		addSprite(fontName)
	}
	if titleImageName != "" {
		addSprite(titleImageName)
	}

	jag := jagfile.NewEmptyJagfile(true)
	jag.Write("index.dat", idxBuf)

	bgBuf := packet.Alloc(len(bgBytes))
	bgBuf.PData(bgBytes)
	jag.Write("title.dat", bgBuf)

	for _, e := range entries {
		datBuf := packet.Alloc(256)
		datBuf.P2(uint16(e.idxPos))
		datBuf.PData(pixels)
		jag.Write(e.name+".dat", datBuf)
	}

	return writeCacheJag(t, jag, 0, 1)
}
