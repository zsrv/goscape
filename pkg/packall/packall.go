// Package packall is the top-level pack orchestrator. It lives in its
// own package to avoid an import cycle between pkg/pack (shared
// Registry / freshness / packfile types) and the per-stage subpackages
// under pkg/pack/<stage> that depend on it.
package packall

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/pack/audio"
	"github.com/zsrv/goscape/pkg/pack/clientinterface"
	"github.com/zsrv/goscape/pkg/pack/compiler"
	"github.com/zsrv/goscape/pkg/pack/graphics"
	"github.com/zsrv/goscape/pkg/pack/maps"
	"github.com/zsrv/goscape/pkg/pack/sprites"
	"github.com/zsrv/goscape/pkg/pack/versionlist"
	"github.com/zsrv/goscape/pkg/pack/wordenc"
)

// PackAll is the goscape equivalent of TS packAll (PackAll.ts @ 9aadcec4).
//
// Pipeline (TS-faithful order):
//  1. ClearFsCache
//  2. Alloc reg + EnsureModel → alloc modelFlags (TS PackAll.ts:38-40)
//  3. FileStream created with createNew=true (truncates cache on every pack)
//  4. PackConfigsForPackAll (server-side configs + registry + modelFlags + cache.Write(0,2))
//  5. clientinterface.Pack (BUILD_VERIFY-gated, informational only; cache.Write(0,3))
//  6. compiler.RunServerCompiler (native compiler; no jar download)
//  7. sprites.PackTitle    → cache.Write(0,1)
//  8. sprites.PackMedia    → cache.Write(0,4)
//  9. sprites.PackTexture  → cache.Write(0,6)
// 10. wordenc.Pack         → cache.Write(0,7)
// 11. audio.PackSound      → cache.Write(0,8)
// 12. graphics.Pack        → cache.Write(1,id) + cache.Write(2,id)
// 13. audio.PackMidi       → cache.Write(3,id)
// 14. maps.Pack            → cache.Write(4,id)
// 15. versionlist.Pack     → cache.Write(0,5)
// 16. server/build stamp   (4-byte uint32 unix seconds)
// 17. ondemand.zip         (archives 1-4, stored, deterministic ModTime)
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
//
// rev244-b6-packall-modelflags: TS packAll(modelFlags) out-param is read by
// NO caller at the pin (app.ts:28-29, DevThread.ts:24-25, Build.ts:163-166).
// Go keeps PackAll(srcDir,outDir,dataPackDir,rawDir) and owns the slice
// internally. NO-OP at the boundary; CLOSES the B1-deferred DevThread row +
// B3-deferred app.ts packAll row.
func PackAll(srcDir, outDir, dataPackDir, rawDir string) error {
	pack.ClearFsCache()

	// TS PackAll.ts:38-40: modelFlags zeroed to ModelPack.max before cache open.
	reg := &pack.Registry{SrcDir: srcDir}
	if _, err := reg.EnsureModel(); err != nil {
		return fmt.Errorf("PackAll: EnsureModel: %w", err)
	}
	modelFlags := make([]int, reg.Model.Max)

	// TS PackAll.ts:43: const cache = new FileStream('data/pack', true)
	// createNew=true truncates the cache on every pack (TS parity).
	cache := filestream.New(outDir, true, false)
	defer cache.Close()

	if err := pack.PackConfigsForPackAll(srcDir, outDir, reg, modelFlags, cache); err != nil {
		return fmt.Errorf("PackAll: PackConfigs: %w", err)
	}
	if err := clientinterface.Pack(reg, srcDir, outDir, modelFlags, cache); err != nil {
		return fmt.Errorf("PackAll: ClientInterface: %w", err)
	}
	// TS PackAll.ts:49 @ 9aadcec4: generateCompilerSymbols() ran BEFORE
	// the RuneScriptCompiler jar invocation. At the rev-254 pin (2e3bcf43)
	// upstream DELETED CompilerSymbols.ts (symbols live in-memory in the
	// @lostcityrs/runescript compiler); goscape KEEPS the export as a
	// documented Go-only feature — see the symbols-export-go-only
	// exception in pkg/pack/compiler/symbols_export.go / docs/PORTING.md.
	// The full-tree parity manifest (testdata/ref254_manifest.txt) EXCLUDES
	// data/symbols/ (no upstream baseline exists; the retired pre-254
	// manifest included it). symbolsDir is a sibling of outDir named "symbols"
	// (e.g. data/pack → data/symbols), matching TS's historical hardcoded
	// 'data/symbols' output path.
	symbolsDir := filepath.Join(filepath.Dir(outDir), "symbols")
	if err := compiler.WriteCompilerSymbols(srcDir, outDir, symbolsDir); err != nil {
		return fmt.Errorf("PackAll: WriteCompilerSymbols: %w", err)
	}
	if err := compiler.RunServerCompiler(srcDir, outDir, dataPackDir); err != nil {
		return fmt.Errorf("PackAll: RunServerCompiler: %w", err)
	}
	if err := sprites.PackTitle(srcDir, outDir, cache); err != nil {
		return fmt.Errorf("PackAll: Title: %w", err)
	}
	if err := sprites.PackMedia(srcDir, outDir, cache); err != nil {
		return fmt.Errorf("PackAll: Media: %w", err)
	}
	if err := sprites.PackTexture(reg, srcDir, outDir, cache); err != nil {
		return fmt.Errorf("PackAll: Texture: %w", err)
	}
	// wordenc.Pack reads from rawDir/wordenc (TS-faithful convention).
	if err := wordenc.Pack(rawDir, cache); err != nil {
		return fmt.Errorf("PackAll: Wordenc: %w", err)
	}
	if err := audio.PackSound(reg, srcDir, outDir, cache); err != nil {
		return fmt.Errorf("PackAll: Sound: %w", err)
	}
	// modelFlags threaded from PackConfigs (TS PackAll.ts:65 @ 9aadcec4).
	if err := graphics.Pack(reg, srcDir, modelFlags, cache, nil); err != nil {
		return fmt.Errorf("PackAll: Graphics: %w", err)
	}
	if err := audio.PackMidi(reg, srcDir, cache); err != nil {
		return fmt.Errorf("PackAll: Midi: %w", err)
	}
	// EnsureMap so the mapPack is available for cache write-back keying.
	// maps.Pack skips cache writes when mapPack is nil; with a live cache
	// we must supply the real PackFile. TS PackAll.ts:69 @ 9aadcec4.
	if _, err := reg.EnsureMap(); err != nil {
		return fmt.Errorf("PackAll: EnsureMap: %w", err)
	}
	// modelFlags threaded from PackConfigs (TS PackAll.ts:69 @ 9aadcec4).
	if err := maps.Pack(srcDir, outDir, reg.Map, cache, modelFlags); err != nil {
		return fmt.Errorf("PackAll: Maps: %w", err)
	}
	// modelFlags threaded from PackConfigs + maps.Pack (TS PackAll.ts:71 @ 9aadcec4).
	if err := versionlist.Pack(reg, srcDir, outDir, modelFlags, cache); err != nil {
		return fmt.Errorf("PackAll: VersionList: %w", err)
	}

	// TS PackAll.ts:73-75 @ 9aadcec4:
	//   const build = Packet.alloc(0); build.p4(Date.now()/1000); build.save('data/pack/server/build')
	// Go: uint32 big-endian unix seconds, written to <outDir>/server/build.
	// PORTING-EXCEPTION (rev244-b6-build-stamp, parity-exempt artifact):
	// TS truncates to signed 32-bit via Packet.p4 (int); goscape writes the
	// same 4 bytes but treats the value as uint32. Observable wire difference
	// only when unix seconds overflow int32 (~2038). Comment retained.
	if err := WriteServerBuild(outDir); err != nil {
		return fmt.Errorf("PackAll: build stamp: %w", err)
	}

	// TS PackAll.ts:77-90 @ 9aadcec4:
	//   for archive 1..4, file 0..count-1: if data != null → zipPack[`${archive}.${file}`] = data
	//   fflate.zipSync(zipPack, { level: 0 }) → fs.writeFileSync('data/pack/ondemand.zip', zip)
	// PORTING-EXCEPTION (rev244-b6-ondemand-zip, content-level parity):
	// TS uses fflate zipSync with level=0 (STORE). Go uses archive/zip with
	// zip.Store method and FIXED ModTime (time.Unix(0,0).UTC()) so goscape's
	// zip is deterministic. The zip container bytes are NOT byte-identical to
	// TS output (zip header timestamps, tool identifiers differ); entry
	// content is identical. See docs/PORTING.md §B6 decision rows.
	if err := WriteOndemandZip(outDir, cache); err != nil {
		return fmt.Errorf("PackAll: ondemand.zip: %w", err)
	}

	return nil
}

// WriteServerBuild writes a 4-byte big-endian uint32 unix timestamp to
// <outDir>/server/build. Exported so pkg cmd/goscape-cli smoke-pack can
// run this as a named stage. Mirrors TS PackAll.ts:73-75 @ 9aadcec4.
func WriteServerBuild(outDir string) error {
	serverDir := filepath.Join(outDir, "server")
	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		return err
	}
	ts := uint32(time.Now().Unix())
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, ts)
	return os.WriteFile(filepath.Join(serverDir, "build"), buf, 0o644)
}

// WriteOndemandZip writes <outDir>/ondemand.zip containing stored (level=0)
// entries for each non-nil file in archives 1-4. Entry names follow the TS
// convention `${archive}.${file}`. ModTime is fixed at Unix epoch for
// determinism (TS fflate does not embed timestamps). Exported so
// cmd/goscape-cli smoke-pack can run this as a named stage.
// Mirrors TS PackAll.ts:77-90.
//
// PORTING-EXCEPTION (rev244-b6-ondemand-zip, content-level parity): zip
// container bytes differ from TS output; entry content is identical.
func WriteOndemandZip(outDir string, cache *filestream.FileStream) error {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	fixedTime := time.Unix(0, 0).UTC()
	for archive := 1; archive <= 4; archive++ {
		count := cache.Count(archive)
		for file := range count {
			data := cache.Read(archive, file, false)
			if data == nil {
				continue
			}
			fh := &zip.FileHeader{
				Name:     fmt.Sprintf("%d.%d", archive, file),
				Method:   zip.Store,
				Modified: fixedTime,
			}
			ew, err := w.CreateHeader(fh)
			if err != nil {
				return err
			}
			if _, err := ew.Write(data); err != nil {
				return err
			}
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "ondemand.zip"), buf.Bytes(), 0o644)
}
