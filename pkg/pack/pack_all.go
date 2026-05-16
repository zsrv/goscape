// pkg/pack/pack_all.go
package pack

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack/compiler"
)

// PackAll is the goscape equivalent of TS packAll (PackAll.ts:17-52),
// restricted to its server-applicable steps.
//
// Pipeline:
//  1. ClearFsCache — drop cached file stats so the per-file
//     freshness gates re-stat from disk.
//  2. PackConfigs(srcDir, outDir) — pack all 18 server-side config
//     types into <outDir>/server/<type>.{dat,idx} + the
//     <outDir>/client/config jagfile.
//  3. compiler.RunServerCompiler(srcDir, outDir, dataPackDir) —
//     compile all .rs2 sources into <outDir>/server/script.{dat,idx}
//     using the symbol tables freshly written by stage 2.
//
// dataPackDir is the cache directory RunServerCompiler reads (the 7
// entity-type loaders: InvType, Component, VarP, VarN, VarS, Param,
// DbTableType). Most callers pass outDir (read back what PackConfigs
// just wrote); the spec leaves it explicit so callers can point at a
// pre-built cache without re-packing.
//
// Errors from any stage are wrapped with the stage name and returned
// immediately. Subsequent stages do not execute.
//
// NAI-212-D-CLIENT-PACKERS-DEFERRED: TS packAll calls 9 additional
// stages with no goscape implementation: packClientInterface,
// packClientTitle, packClientMedia, packClientTexture,
// packClientWordenc, packClientSound, packClientGraphics,
// packClientMidi, packMaps. Retires when the client-pack arc lands.
//
// NAI-212-D-REVALIDATEPACK-INSIDE-PACKCONFIGS: TS packAll calls
// revalidatePack() before packConfigs(). PackConfigs constructs+saves
// every PackFile it touches internally, making a standalone revalidate
// a no-op in goscape. Permanent.
func PackAll(srcDir, outDir, dataPackDir string) error {
	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		return fmt.Errorf("PackAll: PackConfigs: %w", err)
	}
	if err := compiler.RunServerCompiler(srcDir, outDir, dataPackDir); err != nil {
		return fmt.Errorf("PackAll: RunServerCompiler: %w", err)
	}
	return nil
}
