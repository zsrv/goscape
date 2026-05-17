// Package sprites ports tools/pack/sprite/{title,media,textures}.ts.
//
// Each Pack* function is a thin wrapper over pkg/pixpack.ConvertImage
// that bundles per-image results into a Jagfile under
// <outDir>/client/<name>.
package sprites

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/pixpack"
)

// PackTitle ports tools/pack/sprite/title.ts (30 LOC).
//
// Reads 4 PNGs from <srcDir>/title, 4 fonts from <srcDir>/fonts, and
// raw title.jpg from <srcDir>/binary. Bundles into Jagfile at
// <outDir>/client/title.
//
// NAI-192-D-NO-SRC-NO-OP mirror: missing <srcDir>/title → no-op cleanly.
func PackTitle(srcDir, outDir string) error {
	if _, err := os.Stat(filepath.Join(srcDir, "title")); os.IsNotExist(err) {
		return nil
	}
	index := packet.Alloc(3)

	type entry struct{ name, subdir string }
	all := []entry{
		{"logo", "title"}, {"runes", "title"}, {"titlebox", "title"}, {"titlebutton", "title"},
		{"b12", "fonts"}, {"p11", "fonts"}, {"p12", "fonts"}, {"q8", "fonts"},
	}
	results := make([]*packet.Packet, len(all))
	for i, e := range all {
		p, err := pixpack.ConvertImage(index, filepath.Join(srcDir, e.subdir), e.name)
		if err != nil {
			return err
		}
		results[i] = p
	}

	jag := jagfile.NewEmptyJagfile(false)
	jpg, err := os.ReadFile(filepath.Join(srcDir, "binary", "title.jpg"))
	if err != nil {
		return err
	}
	titleDat := packet.Alloc(len(jpg) + 8)
	titleDat.PData(jpg)
	jag.Write("title.dat", titleDat)
	jag.Write("index.dat", index)
	for i, e := range all {
		jag.Write(e.name+".dat", results[i])
	}

	dest := filepath.Join(outDir, "client", "title")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return jag.Save(dest)
}

// PackMedia ports tools/pack/sprite/media.ts (34 LOC).
//
// Walks <srcDir>/sprites/*.png, sorts spritesheets (those with a
// <srcDir>/sprites/meta/<name>.opt sidecar) last per TS line 16-20,
// converts each, bundles into Jagfile at <outDir>/client/media.
func PackMedia(srcDir, outDir string) error {
	index := packet.Alloc(3)

	spritesDir := filepath.Join(srcDir, "sprites")
	files := pack.ListFilesExt(spritesDir, ".png")
	slices.SortStableFunc(files, func(a, b string) int {
		aName := strings.TrimSuffix(filepath.Base(a), filepath.Ext(a))
		bName := strings.TrimSuffix(filepath.Base(b), filepath.Ext(b))
		aHas := pack.FileExists(filepath.Join(spritesDir, "meta", aName+".opt"))
		bHas := pack.FileExists(filepath.Join(spritesDir, "meta", bName+".opt"))
		if aHas == bHas {
			return 0
		}
		if aHas {
			return 1
		}
		return -1
	})

	results := map[string]*packet.Packet{}
	names := make([]string, 0, len(files))
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		p, err := pixpack.ConvertImage(index, spritesDir, name)
		if err != nil {
			return err
		}
		results[name] = p
		names = append(names, name)
	}

	jag := jagfile.NewEmptyJagfile(false)
	jag.Write("index.dat", index)
	for _, name := range names {
		jag.Write(name+".dat", results[name])
	}

	dest := filepath.Join(outDir, "client", "media")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return jag.Save(dest)
}

// PackTexture ports tools/pack/sprite/textures.ts (21 LOC).
//
// Iterates id ∈ [0, 50), converts <srcDir>/textures/<reg.Texture.GetByID(id)>.png,
// bundles into Jagfile at <outDir>/client/textures.
//
// NAI-213-D-PACKTEXTURE-MISSING-ID-SKIP: TS crashes on missing IDs
// (passes undefined to convertImage → reads "/.png"); goscape skips
// them gracefully. Tolerance-divergence; production datapack has all
// 50 IDs populated so the path is unreachable in practice.
func PackTexture(reg *pack.Registry, srcDir, outDir string) error {
	texturePack, err := reg.EnsureTexture()
	if err != nil {
		return err
	}
	index := packet.Alloc(3)

	texturesDir := filepath.Join(srcDir, "textures")
	results := make([]*packet.Packet, 50)
	for id := range 50 {
		name := texturePack.GetByID(id)
		if name == "" {
			continue
		}
		p, err := pixpack.ConvertImage(index, texturesDir, name)
		if err != nil {
			return err
		}
		results[id] = p
	}

	jag := jagfile.NewEmptyJagfile(false)
	jag.Write("index.dat", index)
	for id, p := range results {
		if p == nil {
			continue
		}
		jag.Write(strconv.Itoa(id)+".dat", p)
	}

	dest := filepath.Join(outDir, "client", "textures")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return jag.Save(dest)
}
