// Package worldmap implements the worldmap floorcol/labels dump tool.
//
// Mirrors TS tools/unpack/worldmap/Unpack.ts (Engine-TS 2e3bcf43, rev-254;
// identical to the 244 pin 9aadcec4 - the cross-pin diff does not touch it):
//
//	FloType.load('data/pack')
//	const worldmap = Jagfile.load('data/unpack/worldmap.jag')
//	// floorcol.dat: per-entry underlay/overlay hex + flo metadata
//	// labels.dat:   per-label =text,x,y,font
//
// TS source: tools/unpack/worldmap/Unpack.ts (33 lines).
package worldmap

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pack"
)

// Unpack mirrors tools/unpack/worldmap/Unpack.ts: prints the floorcol table
// and map labels from <cacheDir>/worldmap.jag, with flo metadata from packDir.
//
// Parameters:
//   - cacheDir: directory containing worldmap.jag
//     (TS: 'data/unpack' — Jagfile.load('data/unpack/worldmap.jag'))
//   - packDir: content pack directory passed to FloType.load
//     (TS: 'data/pack' — reads packDir/server/flo.dat + packDir/client/config)
//   - srcDir: content source directory for TexturePack
//     (TS: BUILD_SRC_DIR — loads <srcDir>/pack/texture.pack)
//   - out: stdout equivalent; nil = discard
func Unpack(cacheDir, packDir, srcDir string, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}

	// TS line 5: FloType.load('data/pack')
	// LoadFloTypes reads packDir/server/flo.dat + packDir/client/config jagfile.
	flo, err := objtype.LoadFloTypes(packDir)
	if err != nil {
		return fmt.Errorf("Unpack: LoadFloTypes(%q): %w", packDir, err)
	}

	// TS line 3: import { TexturePack } from '#tools/pack/PackFile.js'.
	// That shared singleton is constructed empty under 274 suspendAutoReload
	// (TS PackFile.ts:276 @dee467c8), so getById returns "" for every id and
	// the floorcol comment prints an empty `texture=` field.
	reg := &pack.Registry{SrcDir: srcDir, SuspendAutoReload: true}
	texturePack, err := reg.EnsureTexture()
	if err != nil {
		return fmt.Errorf("Unpack: EnsureTexture(%q): %w", srcDir, err)
	}

	// TS line 7: const worldmap = Jagfile.load('data/unpack/worldmap.jag')
	wm, err := jagfile.LoadJagfile(filepath.Join(cacheDir, "worldmap.jag"))
	if err != nil {
		return fmt.Errorf("Unpack: LoadJagfile worldmap.jag: %w", err)
	}

	// TS lines 9-21: floorcol.dat loop
	floorcol, err := wm.Read("floorcol.dat")
	if err != nil {
		return fmt.Errorf("Unpack: read floorcol.dat: %w", err)
	}

	// TS line 10: const floorcolCount = floorcol.g2()
	floorcolCount := int(floorcol.G2())

	// TS line 11: for (let i = 0; i < floorcolCount && i < FloType.configs.length; i++)
	limit := min(floorcolCount, len(flo.Configs))
	for i := range limit {
		// TS line 12: underlay = floorcol.g4().toString(16).padStart(8, '0')
		// g4() → uint32; toString(16) → lowercase hex; padStart(8,'0') → 8 digits.
		// Go: %08x on uint32 matches exactly.
		underlay := floorcol.G4()
		// TS line 13: overlay = floorcol.g4().toString(16).padStart(8, '0')
		overlay := floorcol.G4()

		// TS line 15: const flo = FloType.get(i)
		ft := flo.Configs[i]

		// TS lines 16-20: if texture != -1 → with texture= suffix; else without.
		if ft.Texture != -1 {
			// TS line 17:
			// `[0x${underlay}, 0x${overlay}], // debugname=${flo.debugname} overlay=${flo.overlay} occlude=${flo.occlude} rgb=0x${flo.rgb.toString(16).padStart(6,'0')} texture=${TexturePack.getById(flo.texture)}`
			// flo.rgb is a 24-bit int; toString(16).padStart(6,'0') → 6 lowercase hex digits.
			// flo.overlay / flo.occlude are booleans → TS prints "true" / "false" (lowercase).
			fmt.Fprintf(out, "[0x%08x, 0x%08x], // debugname=%s overlay=%t occlude=%t rgb=0x%06x texture=%s\n",
				underlay, overlay,
				ft.DebugName, ft.Overlay, ft.Occlude,
				ft.RGB,
				texturePack.GetByID(ft.Texture),
			)
		} else {
			// TS line 19:
			// `[0x${underlay}, 0x${overlay}], // debugname=${flo.debugname} overlay=${flo.overlay} occlude=${flo.occlude} rgb=0x${flo.rgb.toString(16).padStart(6,'0')}`
			fmt.Fprintf(out, "[0x%08x, 0x%08x], // debugname=%s overlay=%t occlude=%t rgb=0x%06x\n",
				underlay, overlay,
				ft.DebugName, ft.Overlay, ft.Occlude,
				ft.RGB,
			)
		}
	}

	// TS line 23: console.log('----')
	fmt.Fprintln(out, "----")

	// TS lines 25-33: labels.dat loop
	labels, err := wm.Read("labels.dat")
	if err != nil {
		return fmt.Errorf("Unpack: read labels.dat: %w", err)
	}

	// TS line 26: const labelCount = labels.g2()
	labelCount := int(labels.G2())

	// TS lines 27-33: for (let i = 0; i < labelCount; i++)
	for range labelCount {
		// TS line 28: const text = labels.gjstr()
		text := labels.GJStrLF()
		// TS line 29: const x = labels.g2()
		x := labels.G2()
		// TS line 30: const y = labels.g2()
		y := labels.G2()
		// TS line 31: const font = labels.g1()
		font := labels.G1()
		// TS line 32: console.log(`=${text},${x},${y},${font}`)
		fmt.Fprintf(out, "=%s,%d,%d,%d\n", text, x, y, font)
	}

	return nil
}
