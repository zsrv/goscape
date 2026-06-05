// Package graphics ports tools/pack/graphics/pack.ts at Engine-TS 9aadcec4
// (rev-244): per-file gzip cache archives for models and animation sets.
//
// Rev-244 replaces the 225 Jagfile aggregation (21 named streams written to
// client/models) with direct per-file gzip writes into a FileStream cache.
// The 225 packBaseStreams / packAnimStreams / packModelStreams helpers and the
// shouldBuildFileAny gate are all deleted upstream; this port follows suit.
package graphics

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/gziputil"
	"github.com/zsrv/goscape/pkg/pack"
)

// Pack ports TS tools/pack/graphics/pack.ts:packClientGraphics at rev-244.
//
// For each <srcDir>/models/*.ob2 (lexical order via pack.ListFilesExt):
//   - id = ModelPack.getByName(basename sans ".ob2")
//   - data = file bytes; if len > 0: cache.Write(1, id, CompressGz(data), 1)
//
// Missing-model warning: for every id in [0, ModelPack.max) not present in
// cache, log.Warn if modelFlags[id] > 0, else silent (TS commented printDebug).
//
// For each <srcDir>/models/*.anim (lexical order):
//   - id = AnimSetPack.getByName(basename sans ".anim")
//   - data = file bytes; if len > 0: cache.Write(2, id, CompressGz(data), 1)
//
// cache is required for the write path. When nil the stage is a no-op (the
// cache is the only output at 244; there is no file fallback). Callers in
// pkg/packall pass nil until T15 wires the real FileStream handle.
//
// modelFlags may be nil (treated as all-zero, no missing-model warnings fired).
//
// TS source: tools/pack/graphics/pack.ts:11-41 @ 9aadcec4 (rev-244).
func Pack(reg *pack.Registry, srcDir string, modelFlags []int, cache *filestream.FileStream, lg *slog.Logger) error {
	if cache == nil {
		// T15 comment: nil cache means real FileStream not yet wired; no-op.
		return nil
	}
	if lg == nil {
		lg = slog.Default()
	}

	modelPack, err := reg.EnsureModel()
	if err != nil {
		return err
	}
	animSetPack, err := reg.EnsureAnimSet()
	if err != nil {
		return err
	}

	modelsSrc := filepath.Join(srcDir, "models")

	// TS line 12-19: pack .ob2 files → archive 1.
	ob2Files := pack.ListFilesExt(modelsSrc, ".ob2")
	for _, file := range ob2Files {
		base := filepath.Base(file)
		name := strings.TrimSuffix(base, ".ob2")
		id := modelPack.GetByName(name)
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("graphics.Pack: read %q: %w", file, err)
		}
		if len(data) > 0 {
			compressed := gziputil.CompressGz(data, 0, len(data))
			if compressed != nil {
				cache.Write(1, id, compressed, 1)
			}
		}
	}

	// TS line 22-29: warn for any model id in [0, max) not in cache.
	for id := 0; id < modelPack.Max; id++ {
		if !cache.Has(1, id) {
			if modelFlags != nil && id < len(modelFlags) && modelFlags[id] > 0 {
				name := modelPack.GetByID(id)
				lg.Warn(fmt.Sprintf("missing model %s (%d)", name, id))
			}
			// else: TS has commented printDebug — silent in Go too.
		}
	}

	// TS line 32-40: pack .anim files → archive 2.
	animFiles := pack.ListFilesExt(modelsSrc, ".anim")
	for _, file := range animFiles {
		base := filepath.Base(file)
		name := strings.TrimSuffix(base, ".anim")
		id := animSetPack.GetByName(name)
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("graphics.Pack: read %q: %w", file, err)
		}
		if len(data) > 0 {
			compressed := gziputil.CompressGz(data, 0, len(data))
			if compressed != nil {
				cache.Write(2, id, compressed, 1)
			}
		}
	}

	return nil
}
