// Package graphics — models.go implements the model-family unpack entry point
// that mirrors TS tools/unpack/graphics/UnpackModels.ts (57 lines,
// Engine-TS 9aadcec4).
//
// # Gunzip decision
//
// TS UnpackModels.ts:47-49 reads the raw model bytes with
// cache.read(1, id) (no decompress argument → decompress=false on the TS
// FileStream overload) and then gunzips them explicitly:
//
//	const model = cache.read(1, id);
//	if (model) { fs.writeFileSync(destFile, zlib.gunzipSync(model)); }
//
// The filestream.Read(archive, file, decompress=true) path in Go performs an
// identical gunzip (Multistream(false), CRC-strict) — it is the canonical
// equivalent of the inline gunzipSync call.  Using decompress=true therefore
// produces bit-exact output and avoids a second decompressor layer.
//
// # Loop bound
//
// TS iterates id from 0 while id < modelCount (cache.count(1)) AND
// id < models.length (the versionlist flag slice).  Both bounds must hold.
//
// # Existing-file lookup
//
// TS listFilesExt finds all .ob2 files under BUILD_SRC_DIR/models/ and
// existingFiles.find(x => x.endsWith('/<name>.ob2')) returns the FIRST match.
// Go mirrors with filepath.WalkDir + first-wins semantics (identical to the
// sound package).
//
// # Missing-data routing
//
// models[id] (the versionlist flag byte) non-zero → printWarning; zero →
// printDebug.  Both are emitted to Out.  The manifest stdout includes the
// printDebug lines normalised.
//
// TS source: tools/unpack/graphics/UnpackModels.ts.
package graphics

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// Options holds all inputs for a graphics-family unpack run.
type Options struct {
	// CacheDir is the directory containing main_file_cache.dat/idx0-4.
	CacheDir string

	// SrcDir is the content tree root (BUILD_SRC_DIR in TS).  Output files
	// are written to <SrcDir>/models/ and <SrcDir>/models/_unpack/.
	SrcDir string

	// Out is the console.log / printWarning / printDebug channel.  nil = discard.
	Out io.Writer
}

// Models is the top-level entry point for the model-family unpack.
// It mirrors TS tools/unpack/graphics/UnpackModels.ts.
//
// The function:
//  1. Reads versionlist (archive 0 / file 5) and parses model_index.
//  2. Reads cache.count(1) as modelCount; logs "Extracting N models".
//  3. Iterates id 0..min(modelCount,len(models))-1; resolves/registers name;
//     finds existing .ob2 or falls back to models/_unpack/<name>.ob2;
//     reads+gunzips model data; writes file; warns/debugs on missing.
//  4. Calls ModelPack.save().
//
// TS source: tools/unpack/graphics/UnpackModels.ts.
func Models(opts Options) error {
	printWarning := func(msg string) {
		if opts.Out != nil {
			fmt.Fprintf(opts.Out, "%s\n", msg)
		}
	}
	printDebug := func(msg string) {
		if opts.Out != nil {
			fmt.Fprintf(opts.Out, "%s\n", msg)
		}
	}
	consolelog := func(msg string) {
		if opts.Out != nil {
			fmt.Fprintf(opts.Out, "%s\n", msg)
		}
	}

	// TS lines 18-22: listFilesExt + mkdir _unpack.
	modelsDir := filepath.Join(opts.SrcDir, "models")
	unpackDir := filepath.Join(modelsDir, "_unpack")
	if err := os.MkdirAll(unpackDir, 0o755); err != nil {
		return fmt.Errorf("graphics/models: mkdir _unpack: %w", err)
	}

	// Collect existing .ob2 files; first-wins on duplicate basenames.
	existingOb2, err := listFilesExtFirstWins(modelsDir, ".ob2")
	if err != nil {
		return fmt.Errorf("graphics/models: list existing .ob2: %w", err)
	}

	// TS line 16: new FileStream('data/unpack').
	cache := filestream.New(opts.CacheDir, false, true)
	defer cache.Close()

	// TS line 24: versionlist = cache.read(0, 5) parsed as Jagfile.
	vlData := cache.Read(0, 5, false)
	if vlData == nil {
		return fmt.Errorf("graphics/models: no versionlist in cache")
	}
	versionlist, err := jagfile.NewJagfile(packet.NewPacket(vlData))
	if err != nil {
		return fmt.Errorf("graphics/models: parse versionlist: %w", err)
	}

	// TS lines 28-33: read model_index → flag slice.
	var models []uint8
	modelIndex, err := versionlist.Read("model_index")
	if err == nil && modelIndex != nil {
		// Each byte is a flag for the corresponding model id.
		for modelIndex.Len() > 0 {
			models = append(models, modelIndex.G1())
		}
	}

	// TS line 35: modelCount = cache.count(1).
	modelCount := cache.Count(1)

	// TS line 36: console.log(`Extracting ${modelCount} models`).
	consolelog(fmt.Sprintf("Extracting %d models", modelCount))

	// Load ModelPack.
	reg := &pack.Registry{SrcDir: opts.SrcDir}
	modelPack, err := reg.EnsureModel()
	if err != nil {
		return fmt.Errorf("graphics/models: ensure model pack: %w", err)
	}

	// TS lines 38-55: main loop.
	for id := range modelCount {
		if id >= len(models) {
			break
		}

		// TS lines 39-41: register if not already present.
		if modelPack.GetByID(id) == "" {
			modelPack.Register(id, fmt.Sprintf("model_%d", id))
		}
		name := modelPack.GetByID(id)

		// TS lines 44-45: existing path or _unpack fallback.
		var destFile string
		if existing, ok := existingOb2[name]; ok {
			destFile = existing
		} else {
			destFile = filepath.Join(unpackDir, name+".ob2")
		}

		// TS line 47: cache.read(1, id) — raw read; then gunzipSync in TS.
		// Go: Read(1, id, true) is the canonical equivalent (Multistream(false),
		// CRC-strict) — produces bit-exact output for the same gzip stream.
		modelData := cache.Read(1, id, true)
		if modelData != nil {
			// TS line 49: writeFileSync(destFile, gunzipSync(model)).
			if err := os.WriteFile(destFile, modelData, 0o644); err != nil {
				return fmt.Errorf("graphics/models: write %s: %w", destFile, err)
			}
		} else if models[id] != 0 {
			// TS line 51: printWarning(`Missing model ${name}`).
			printWarning(fmt.Sprintf("Missing model %s", name))
		} else {
			// TS line 53: printDebug(`Missing unreferenced model ${name}`).
			printDebug(fmt.Sprintf("Missing unreferenced model %s", name))
		}
	}

	// TS line 57: ModelPack.save().
	if err := modelPack.Save(); err != nil {
		return fmt.Errorf("graphics/models: save model pack: %w", err)
	}

	return nil
}

// listFilesExtFirstWins returns a map of basename-without-ext → full path for
// all regular files under root with the given extension.  The first path seen
// for a given basename wins, mirroring TS existingFiles.find(endsWith).
func listFilesExtFirstWins(root, ext string) (map[string]string, error) {
	result := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ext) {
			return nil
		}
		base := strings.TrimSuffix(d.Name(), ext)
		if _, ok := result[base]; !ok {
			result[base] = path
		}
		return nil
	})
	return result, err
}
