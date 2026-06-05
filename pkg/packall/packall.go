// Package packall is the top-level pack orchestrator. It lives in its
// own package to avoid an import cycle between pkg/pack (shared
// Registry / freshness / packfile types) and the per-stage subpackages
// under pkg/pack/<stage> that depend on it.
package packall

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/pack/audio"
	"github.com/zsrv/goscape/pkg/pack/clientinterface"
	"github.com/zsrv/goscape/pkg/pack/compiler"
	"github.com/zsrv/goscape/pkg/pack/graphics"
	"github.com/zsrv/goscape/pkg/pack/maps"
	"github.com/zsrv/goscape/pkg/pack/sprites"
	"github.com/zsrv/goscape/pkg/pack/wordenc"
)

// PackAll is the goscape equivalent of TS packAll (PackAll.ts:17-52).
//
// Pipeline (TS-faithful order):
//  1. ClearFsCache
//  2. PackConfigsForRegistry (server-side configs + registry of *PackFile singletons)
//  3. clientinterface.Pack (build-verify-gated, informational only)
//  4. compiler.RunServerCompiler
//  5. sprites.PackTitle / PackMedia / PackTexture (PixPack-backed)
//  6. wordenc.Pack
//  7. audio.PackSound
//  8. graphics.Pack
//  9. audio.PackMidi
//
// 10. maps.Pack
//
// dataPackDir is the cache directory RunServerCompiler reads (the 7
// entity-type loaders: InvType, Component, VarP, VarN, VarS, Param,
// DbTableType).
//
// rawDir is the directory containing engine-owned raw blobs (wordenc Jagfile).
// Callers should pass "data/raw" (the TS-faithful project-root-relative path).
// Tests may supply a temp-dir fixture. Mirrors TS's hardcoded relative path.
//
// Errors from any stage are wrapped with the stage name and returned
// immediately. Subsequent stages do not execute.
//
// NAI-212-D-REVALIDATEPACK-INSIDE-PACKCONFIGS: TS packAll calls
// revalidatePack() before packConfigs(). PackConfigs constructs+saves
// every PackFile it touches internally, making a standalone revalidate
// a no-op in goscape. Permanent.
func PackAll(srcDir, outDir, dataPackDir, rawDir string) error {
	pack.ClearFsCache()
	reg, err := pack.PackConfigsForRegistry(srcDir, outDir)
	if err != nil {
		return fmt.Errorf("PackAll: PackConfigs: %w", err)
	}
	if err := clientinterface.Pack(reg, srcDir, outDir); err != nil {
		return fmt.Errorf("PackAll: ClientInterface: %w", err)
	}
	if err := compiler.RunServerCompiler(srcDir, outDir, dataPackDir); err != nil {
		return fmt.Errorf("PackAll: RunServerCompiler: %w", err)
	}
	// nil cache: real FileStream handle wired in T15 (PackAll.ts:42).
	if err := sprites.PackTitle(srcDir, outDir, nil); err != nil {
		return fmt.Errorf("PackAll: Title: %w", err)
	}
	if err := sprites.PackMedia(srcDir, outDir, nil); err != nil {
		return fmt.Errorf("PackAll: Media: %w", err)
	}
	if err := sprites.PackTexture(reg, srcDir, outDir, nil); err != nil {
		return fmt.Errorf("PackAll: Texture: %w", err)
	}
	// wordenc.Pack reads from rawDir/wordenc (TS-faithful convention).
	// nil cache: real FileStream handle wired in T15.
	if err := wordenc.Pack(rawDir, nil); err != nil {
		return fmt.Errorf("PackAll: Wordenc: %w", err)
	}
	// nil cache: real FileStream handle wired in T15.
	if err := audio.PackSound(reg, srcDir, outDir, nil); err != nil {
		return fmt.Errorf("PackAll: Sound: %w", err)
	}
	if err := graphics.Pack(reg, srcDir, outDir); err != nil {
		return fmt.Errorf("PackAll: Graphics: %w", err)
	}
	if err := audio.PackMidi(srcDir, outDir); err != nil {
		return fmt.Errorf("PackAll: Midi: %w", err)
	}
	if err := maps.Pack(srcDir, outDir); err != nil {
		return fmt.Errorf("PackAll: Maps: %w", err)
	}
	return nil
}
