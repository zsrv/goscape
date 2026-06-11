// Package config — driver.go implements Unpack, the top-level config-family
// unpack entry point that mirrors TS tools/unpack/config/Unpack.ts unpackConfigs
// (lines 278-354) plus its private helpers unpackConfigNames (lines 47-82),
// reorderUnpacked (lines 84-111), unpackConfig (lines 115-201), and
// unpackModelNames (lines 205-276).
//
// TS source: tools/unpack/config/Unpack.ts (Engine-TS 2e3bcf43).
package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/unpack/internal/model"
)

// Options holds all inputs for a config-family unpack run.
type Options struct {
	// CacheDir is the directory containing main_file_cache.dat/idx0-4
	// (the "data/unpack" path in TS).
	CacheDir string

	// PackDir is the optional second cache for the merge-compare path
	// (the "data/pack" path in TS). Empty string disables the path.
	PackDir string

	// SrcDir is the content tree root (BUILD_SRC_DIR in TS). All output files
	// are written relative to this directory.
	SrcDir string

	// DisplaySrcDir is the string printed in the "Unpacking rev … into …/scripts"
	// line in place of SrcDir. This exists because the reference STDOUT-NORM was
	// captured with TS running under a relative path ("../unpack-ref/scratch") that
	// differs from any absolute temp path a Go test would use. Pass "" to fall back
	// to SrcDir (the faithful default for non-parity callers).
	//
	// Infra-adaptation: parity test passes DisplaySrcDir: "../unpack-ref/scratch"
	// while SrcDir points at the real temp scratch.
	DisplaySrcDir string

	// Revision is embedded in the output path scripts/_unpack/<Revision>/all.<type>.
	// TS hardcodes "254" at the call site (Unpack.ts:356 @2e3bcf43;
	// was "245" at Unpack.ts:368 @3c16994c).
	Revision string

	// Out receives printInfo / printWarning lines (bare message + "\n").
	// nil uses io.Discard.
	Out io.Writer

	// Errorf is the console.error sink (NOT written to Out; used for non-fatal
	// errors such as missing model files). nil = no-op.
	Errorf func(format string, args ...any)
}

// reorderUnpackedSettings mirrors the per-type settings struct in TS
// Unpack.ts:143-160.
type reorderUnpackedSettings struct {
	moveName  bool
	moveDesc  bool
	moveRecol bool
	moveModel bool
}

// reorderUnpacked mirrors TS Unpack.ts:84-111 @2e3bcf43.
// Splits the config lines into buckets and returns them in the canonical order:
// debugname (lines starting with '['), name=, desc=, model, recol/retex, others.
// Buckets are only populated when the corresponding settings flag is true.
//
// 254 deltas: the model bucket matches only the 'model' prefix ('ldmodel' is
// gone from the check), and hasalpha=/code9= lines are DROPPED entirely
// (TS line 106 excludes them from the others bucket).
//
// TS source: tools/unpack/config/Unpack.ts:84-111 @2e3bcf43.
func reorderUnpacked(config []string, settings reorderUnpackedSettings) []string {
	var debugname, name, desc, model, recol, others []string

	for _, line := range config {
		switch {
		case strings.HasPrefix(line, "["):
			debugname = append(debugname, line)
		case settings.moveName && strings.HasPrefix(line, "name="):
			name = append(name, line)
		case settings.moveDesc && strings.HasPrefix(line, "desc="):
			desc = append(desc, line)
		case settings.moveModel && strings.HasPrefix(line, "model"):
			model = append(model, line)
		case settings.moveRecol && (strings.HasPrefix(line, "recol") || strings.HasPrefix(line, "retex")):
			recol = append(recol, line)
		case !strings.HasPrefix(line, "hasalpha=") && !strings.HasPrefix(line, "code9="):
			others = append(others, line)
		}
	}

	// TS return: [...debugname, ...name, ...desc, ...model, ...recol, ...others]
	return append(append(append(append(append(append(
		make([]string, 0, len(debugname)+len(name)+len(desc)+len(model)+len(recol)+len(others)),
		debugname...), name...), desc...), model...), recol...), others...)
}

// settingsForType returns the reorderUnpackedSettings for the given config type.
// TS Unpack.ts:143-160.
func settingsForType(typeName string) reorderUnpackedSettings {
	s := reorderUnpackedSettings{}
	switch typeName {
	case "loc", "npc", "obj":
		s.moveName = true
		s.moveRecol = true
	case "idk":
		s.moveRecol = true
	}
	if typeName == "loc" {
		s.moveDesc = true
	}
	if typeName == "loc" || typeName == "npc" {
		s.moveModel = true
	}
	return s
}

// unpackConfigNames mirrors TS Unpack.ts:47-79.
// For the given type, reads the idx to get the count, registers missing ids as
// "<type>_<id>", and saves the pack file.
//
// TS source: tools/unpack/config/Unpack.ts:47-79.
func unpackConfigNames(typeName string, jag *jagfile.Jagfile, reg *pack.Registry) error {
	var pf *pack.PackFile
	var err error
	switch typeName {
	case "loc":
		pf, err = reg.EnsureLoc()
	case "npc":
		pf, err = reg.EnsureNpc()
	case "obj":
		pf, err = reg.EnsureObj()
	case "seq":
		pf, err = reg.EnsureSeq()
	case "idk":
		pf, err = reg.EnsureIdk()
	case "flo":
		pf, err = reg.EnsureFlo()
	case "varp":
		pf, err = reg.EnsureVarp()
	case "varbit":
		// NEW at rev-254 (TS Unpack.ts:63-64 @2e3bcf43).
		pf, err = reg.EnsureVarbit()
	case "spotanim":
		pf, err = reg.EnsureSpotAnim()
	default:
		return fmt.Errorf("unrecognized config type %s", typeName)
	}
	if err != nil {
		return fmt.Errorf("ensure pack for %s: %w", typeName, err)
	}

	// TS: readConfigIdx(config.read(type + '.idx'), config.read(type + '.dat'))
	idxPkt, err := jag.Read(typeName + ".idx")
	if err != nil {
		return fmt.Errorf("read %s.idx: %w", typeName, err)
	}
	datPkt, err := jag.Read(typeName + ".dat")
	if err != nil {
		return fmt.Errorf("read %s.dat: %w", typeName, err)
	}

	sourceIdx, err := ReadConfigIdx(idxPkt, datPkt)
	if err != nil {
		return fmt.Errorf("readConfigIdx %s: %w", typeName, err)
	}

	// TS lines 73-77: for id 0..size: if !pack.getById(id) → register default name
	for id := range sourceIdx.Size {
		if pf.GetByID(id) == "" {
			pf.Register(id, fmt.Sprintf("%s_%d", typeName, id))
		}
	}

	// TS line 78: pack.save()
	if err := pf.Save(); err != nil {
		return fmt.Errorf("save %s pack: %w", typeName, err)
	}

	return nil
}

// unpackConfig mirrors TS Unpack.ts:115-201 @2e3bcf43.
// Reads all entries from jag (and optionally jag2 for merge-compare),
// reorders each unpacked entry, and appends to scripts/_unpack/<revision>/all.<type>.
// When jag2 is non-nil, differing entries are appended to all.<type>.merge instead.
//
// modelRenameOffset is threaded into every unpackFn call; the main loop also
// passes compareIdx, the compare-side call passes nil (TS lines 166+171).
//
// TS source: tools/unpack/config/Unpack.ts:115-201 @2e3bcf43.
func unpackConfig(
	revision, typeName string,
	unpackFn func(*ConfigIdx, int, *ConfigIdx, int) ([]string, error),
	jag *jagfile.Jagfile,
	jag2 *jagfile.Jagfile,
	modelRenameOffset int,
	srcDir string,
	printInfoFn func(string),
) error {
	// TS line 116: readConfigIdx
	idxPkt, err := jag.Read(typeName + ".idx")
	if err != nil {
		return fmt.Errorf("read %s.idx: %w", typeName, err)
	}
	datPkt, err := jag.Read(typeName + ".dat")
	if err != nil {
		return fmt.Errorf("read %s.dat: %w", typeName, err)
	}
	sourceIdx, err := ReadConfigIdx(idxPkt, datPkt)
	if err != nil {
		return fmt.Errorf("readConfigIdx %s: %w", typeName, err)
	}

	// TS line 117: printInfo(`Unpacking ${sourceIdx.size} ${type} configs`)
	printInfoFn(fmt.Sprintf("Unpacking %d %s configs", sourceIdx.Size, typeName))

	// TS lines 119-122 @2e3bcf43: optional compareIdx, guarded by
	// `config2 && config2.has(type + '.idx')`. Go probes via Read error:
	// a missing .idx means no compare; a present .idx with a missing .dat is
	// the TS readConfigIdx printFatalError path → error.
	var compareIdx *ConfigIdx
	if jag2 != nil {
		if idxPkt2, err2 := jag2.Read(typeName + ".idx"); err2 == nil {
			datPkt2, err3 := jag2.Read(typeName + ".dat")
			if err3 != nil {
				return fmt.Errorf("compare cache has %s.idx but no %s.dat: %w", typeName, typeName, err3)
			}
			compareIdx, err = ReadConfigIdx(idxPkt2, datPkt2)
			if err != nil {
				return fmt.Errorf("readConfigIdx compare %s: %w", typeName, err)
			}
		}
	}

	// TS lines 136-138: mkdir scripts/_unpack/<revision>
	unpackDir := filepath.Join(srcDir, "scripts", "_unpack", revision)
	if err := os.MkdirAll(unpackDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", unpackDir, err)
	}

	// TS lines 140-141: out path; writeFileSync(out, '') → truncate/create
	outPath := filepath.Join(unpackDir, "all."+typeName)
	if err := os.WriteFile(outPath, []byte{}, 0o644); err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}

	settings := settingsForType(typeName)

	// TS lines 165-200: main loop
	for id := range sourceIdx.Size {
		// TS line 166: unpack(sourceIdx, id, compareIdx, modelRenameOffset)
		unpacked, err := unpackFn(sourceIdx, id, compareIdx, modelRenameOffset)
		if err != nil {
			return fmt.Errorf("unpack %s id %d: %w", typeName, id, err)
		}
		unpacked = reorderUnpacked(unpacked, settings)
		unpacked = append(unpacked, "") // TS: unpacked.push('')

		if compareIdx != nil {
			// TS lines 169-196: merge-compare path
			if id < compareIdx.Size {
				// TS line 171: unpack(compareIdx, id, undefined, modelRenameOffset)
				unpacked2, err := unpackFn(compareIdx, id, nil, modelRenameOffset)
				if err != nil {
					return fmt.Errorf("unpack %s compare id %d: %w", typeName, id, err)
				}
				unpacked2 = reorderUnpacked(unpacked2, settings)
				unpacked2 = append(unpacked2, "") // TS: unpacked2.push('')

				// TS lines 171-176: if any line differs → append both to .merge
				differs := false
				for i := range min(len(unpacked), len(unpacked2)) {
					if unpacked[i] != unpacked2[i] {
						differs = true
						break
					}
				}
				// Go-side guard TS lacks (TS iterates only unpacked2's
				// length, so a longer prefix-matching unpacked would read
				// equal). Unreachable: the "" sentinel pushed above pins
				// position len2-1, forcing equal lengths on a prefix match.
				if !differs && len(unpacked) != len(unpacked2) {
					differs = true
				}

				if differs {
					mergePath := outPath + ".merge"
					// TS line 173: appendFileSync(`${out}.merge`, '// --------\n' + unpacked.join('\n') + '\n')
					block1 := "// --------\n" + strings.Join(unpacked, "\n") + "\n"
					// TS line 174: appendFileSync(`${out}.merge`, unpacked2.join('\n') + '\n')
					block2 := strings.Join(unpacked2, "\n") + "\n"
					if err := appendString(mergePath, block1+block2); err != nil {
						return err
					}
				}
				// When compareIdx exists AND id < compareIdx.Size AND entries are the same,
				// TS does NOT write to the main file (the else branch at TS line 192 only fires
				// when id >= compareIdx.Size).
			} else {
				// TS lines 192-193: id >= compareIdx.Size → append to out
				if err := appendString(outPath, strings.Join(unpacked, "\n")+"\n"); err != nil {
					return err
				}
			}
		} else {
			// TS lines 194-196: no compareIdx → always append to out
			if err := appendString(outPath, strings.Join(unpacked, "\n")+"\n"); err != nil {
				return err
			}
		}
	}

	return nil
}

// packFileMax returns the equivalent of TS PackFileBase.max: the highest registered
// id + 1, or 0 if the registry is empty. This mirrors TS tools/pack/PackFileBase.ts
// `get max()` which returns `Math.max(...this.pack.keys()) + 1` (or 0 when empty).
// Rather than modifying pkg/pack, the computation is done locally over pf.Pack.
//
// TS source: tools/pack/PackFileBase.ts (get max getter).
func packFileMax(pf *pack.PackFile) int {
	if len(pf.Pack) == 0 {
		return 0
	}
	maxID := 0
	for id := range pf.Pack {
		if id > maxID {
			maxID = id
		}
	}
	return maxID + 1
}

// unpackModelNames mirrors TS Unpack.ts:205-276 @2e3bcf43.
// Performs the loc-models pass: collects LocModels for all ids, builds the
// seenAsNonCentrepiece table, then renames model files.
//
// 254 deltas: the optional compare cache (jag2) sets the loop start to its
// loc count (entries the old cache already named are skipped wholesale);
// models below modelRenameOffset are skipped; the ldModels pass is GONE.
//
// TS source: tools/unpack/config/Unpack.ts:205-276 @2e3bcf43.
func unpackModelNames(jag, jag2 *jagfile.Jagfile, modelRenameOffset int, env *Env, srcDir string) error {
	// TS line 206: readConfigIdx(config.read('loc.idx'), config.read('loc.dat'))
	idxPkt, err := jag.Read("loc.idx")
	if err != nil {
		return fmt.Errorf("read loc.idx: %w", err)
	}
	datPkt, err := jag.Read("loc.dat")
	if err != nil {
		return fmt.Errorf("read loc.dat: %w", err)
	}
	sourceIdx, err := ReadConfigIdx(idxPkt, datPkt)
	if err != nil {
		return fmt.Errorf("readConfigIdx loc: %w", err)
	}

	// TS lines 208-211: optional compareIdx, guarded by
	// `config2 && config2.has(type + '.dat')` (NOTE: .dat here, not .idx).
	var compareIdx *ConfigIdx
	if jag2 != nil {
		if datPkt2, err2 := jag2.Read("loc.dat"); err2 == nil {
			idxPkt2, err3 := jag2.Read("loc.idx")
			if err3 != nil {
				return fmt.Errorf("compare cache has loc.dat but no loc.idx: %w", err3)
			}
			compareIdx, err = ReadConfigIdx(idxPkt2, datPkt2)
			if err != nil {
				return fmt.Errorf("readConfigIdx compare loc: %w", err)
			}
		}
	}

	// TS lines 213-216: collect LocModels for all ids
	locs := make([]LocModels, sourceIdx.Size)
	for id := range sourceIdx.Size {
		locs[id] = env.UnpackLocModels(sourceIdx, id)
	}

	// TS lines 218-225: build seenAsNonCentrepiece table
	// shape 10 = centrepiece_straight (_8 suffix); everything else = non-centrepiece.
	seenAsNonCentrepiece := make(map[int]bool)
	for _, cfg := range locs {
		for _, info := range cfg.Models {
			if info.Shape != 10 {
				seenAsNonCentrepiece[info.Model] = true
			}
		}
	}

	// TS line 227: existingFiles = listFilesExt(`${BUILD_SRC_DIR}/models`, '.ob2')
	existingFiles := pack.ListFilesExt(filepath.Join(srcDir, "models"), ".ob2")

	locPack := env.Loc
	modelPack := env.Model

	// TS lines 229-232: start at the compare cache's size when present.
	start := 0
	if compareIdx != nil {
		start = compareIdx.Size
	}
	for id := start; id < len(locs); id++ {
		cfg := locs[id]

		// TS line 235: debugname = LocPack.getById(id)
		debugname := ""
		if locPack != nil {
			debugname = locPack.GetByID(id)
		}

		// TS lines 237-242: strip the shape suffix to get the base loc name.
		// "debugname.endsWith(LocShapeSuffix[shape])" → remove the last "_X" part.
		for shape := range 23 {
			if strings.HasSuffix(debugname, LocShapeSuffix[shape]) {
				// TS line 239: debugname.substring(0, debugname.lastIndexOf('_'))
				//               + debugname.substring(debugname.length - 1)
				lastUnderscore := strings.LastIndex(debugname, "_")
				if lastUnderscore >= 0 {
					debugname = debugname[:lastUnderscore] + debugname[len(debugname)-1:]
				}
				break
			}
		}

		// TS lines 244-272: models pass
		for _, info := range cfg.Models {
			modelID := info.Model
			shape := info.Shape

			// TS line 246: skip if shape==_8 && seenAsNonCentrepiece[model]
			// LocShapeSuffix[10] == "_8" is the centrepiece_straight suffix.
			if LocShapeSuffix[shape] == "_8" && seenAsNonCentrepiece[modelID] {
				continue
			}

			// TS lines 250-252 @2e3bcf43: skip models below the rename offset.
			if modelID < modelRenameOffset {
				continue
			}

			if modelPack == nil {
				continue
			}
			modelName := modelPack.GetByID(modelID)
			// TS line 255: skip if !modelName.startsWith('model_')
			if !strings.HasPrefix(modelName, "model_") {
				continue
			}

			// TS lines 259-264: collision loop
			// initial name = `${debugname}${LocShapeSuffix[shape]}`
			// collision: `${debugname}i${i}${LocShapeSuffix[shape]}`
			suffix := LocShapeSuffix[shape]
			name := debugname + suffix
			i := 2
			for modelPack.GetByName(name) != -1 {
				name = fmt.Sprintf("%si%d%s", debugname, i, suffix)
				i++
			}

			// TS lines 266-269: renameSync if file found
			// filePath = existingFiles.find(x => x.endsWith(`/${modelName}.ob2`))
			filePath := findFileInList(existingFiles, modelName+".ob2")
			if filePath != "" {
				dest := filepath.Join(srcDir, "models", "loc", name+".ob2")
				if err := os.Rename(filePath, dest); err != nil && env.Errorf != nil {
					env.errorf("rename loc model %s -> %s: %v", modelName, name, err)
				}
			}
			// TS line 271: ModelPack.register(model, name)
			modelPack.Register(modelID, name)
		}
	}

	// TS line 275: ModelPack.save()
	if modelPack != nil {
		if err := modelPack.Save(); err != nil {
			return fmt.Errorf("save model pack (unpackModelNames): %w", err)
		}
	}

	return nil
}

// appendString opens path for append (creating if absent), writes s, and
// closes the file. The close error is merged with the write error so that
// a buffered-write failure (e.g. full disk) is never silently dropped.
//
// Used by unpackConfig for the three O_APPEND write sites.
func appendString(path, s string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	_, werr := f.WriteString(s)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return fmt.Errorf("write %s: %w", path, werr)
	}
	return nil
}

// printInfoLine writes a bare message line + "\n" to w. Mirrors TS printInfo
// which emits to stdout without any additional formatting in the STDOUT-NORM sense.
func printInfoLine(w io.Writer, msg string) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "%s\n", msg)
}

// Unpack is the top-level entry point for the config-family unpack.
// It mirrors TS tools/unpack/config/Unpack.ts:294-368 (unpackConfigs).
//
// The function:
//  1. Opens the FileStream cache at opts.CacheDir and reads archive 0 / file 2
//     (the config jagfile).
//  2. Optionally reads a second cache at opts.PackDir (merge-compare path).
//     modelRenameOffset = the model-archive count (cache.count(1)), or the
//     compare cache's count when one is present (TS lines 292-299).
//  3. Preloads all model metadata from archive 1 (Model.unpack pass).
//  4. Runs unpackConfigNames for all nine config types (varbit NEW at 254).
//  5. Creates models/obj, models/spot, models/idk, models/loc, models/npc.
//  6. Runs unpackModelNames (loc-model renaming, compare/offset-gated).
//  7. Runs unpackConfig for all nine types in TS order.
//  8. Saves ModelPack; prints "Done!".
//
// TS source: tools/unpack/config/Unpack.ts:278-354 @2e3bcf43.
func Unpack(opts Options) error {
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	errorf := opts.Errorf
	if errorf == nil {
		errorf = func(string, ...any) {}
	}

	printInfo := func(msg string) { printInfoLine(out, msg) }

	// TS line 295-297: check for main_file_cache.dat
	datPath := filepath.Join(opts.CacheDir, "main_file_cache.dat")
	if _, err := os.Stat(datPath); err != nil {
		return fmt.Errorf("Place a functional cache inside data/unpack to continue.")
	}

	// TS line 299: const cache = new FileStream('data/unpack')
	// TS defaults: createNew=false, readOnly=false.
	cache := filestream.New(opts.CacheDir, false, false)
	defer cache.Close()

	// TS line 300-303: const temp = cache.read(0, 2); if (!temp) return
	temp := cache.Read(0, 2, false)
	if temp == nil {
		return fmt.Errorf("cache.Read(0, 2) returned nil")
	}

	// TS line 305: const config = new Jagfile(new Packet(temp))
	jag, err := jagfile.NewJagfile(packet.NewPacket(temp))
	if err != nil {
		return fmt.Errorf("parse config jagfile: %w", err)
	}

	// TS lines 291-300 @2e3bcf43: optional config2 from data/pack, plus the
	// modelRenameOffset — the model-archive count of the authoritative cache
	// (the compare cache when present, else the unpack cache).
	modelRenameOffset := cache.Count(1)
	var jag2 *jagfile.Jagfile
	if opts.PackDir != "" {
		packDatPath := filepath.Join(opts.PackDir, "main_file_cache.dat")
		if _, err2 := os.Stat(packDatPath); err2 == nil {
			cache2 := filestream.New(opts.PackDir, false, false)
			temp2 := cache2.Read(0, 2, false)
			if temp2 != nil {
				jag2, _ = jagfile.NewJagfile(packet.NewPacket(temp2))
			}
			// TS line 299: set even when the config jag read fails.
			modelRenameOffset = cache2.Count(1)
			cache2.Close()
		}
	}

	// TS line 316: printInfo(`Unpacking rev ${revision} into ${BUILD_SRC_DIR}/scripts`)
	displaySrcDir := opts.DisplaySrcDir
	if displaySrcDir == "" {
		displaySrcDir = opts.SrcDir
	}
	printInfo(fmt.Sprintf("Unpacking rev %s into %s/scripts", opts.Revision, displaySrcDir))

	// Build pack.Registry with opts.SrcDir so all PackFiles resolve relative to it.
	reg := &pack.Registry{SrcDir: opts.SrcDir}

	// TS lines 318-321: for id 0..ModelPack.max: Model.unpack(id, cache.read(1, id, true))
	modelPack, err := reg.EnsureModel()
	if err != nil {
		return fmt.Errorf("ensure model pack: %w", err)
	}
	modelMax := packFileMax(modelPack)
	modelStore := newModelStoreFromCache(cache, modelMax)

	// Build Env wiring all pack files and model store.
	env, err := buildEnv(reg, opts.SrcDir, modelStore, out, errorf)
	if err != nil {
		return fmt.Errorf("build env: %w", err)
	}

	// TS lines 309-317 @2e3bcf43: unpackConfigNames for all 9 types
	// (loc, npc, obj, seq, idk, flo, spotanim, varp, varbit — varbit NEW at 254)
	for _, typeName := range []string{"loc", "npc", "obj", "seq", "idk", "flo", "spotanim", "varp", "varbit"} {
		if err := unpackConfigNames(typeName, jag, reg); err != nil {
			return fmt.Errorf("unpackConfigNames %s: %w", typeName, err)
		}
	}

	// TS lines 332-349: mkdir models/{obj,spot,idk,loc,npc}
	for _, subdir := range []string{"obj", "spot", "idk", "loc", "npc"} {
		dir := filepath.Join(opts.SrcDir, "models", subdir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir models/%s: %w", subdir, err)
		}
	}

	// TS line 339: unpackModelNames('loc', unpackLocModels, config, config2, modelRenameOffset)
	if err := unpackModelNames(jag, jag2, modelRenameOffset, env, opts.SrcDir); err != nil {
		return fmt.Errorf("unpackModelNames: %w", err)
	}

	// Build the unpack functions for each type.
	// TS lines 341-349 @2e3bcf43: unpackConfig calls in order
	// (loc, obj, spotanim, idk, npc, seq, flo, varp, varbit). The first five
	// receive modelRenameOffset; the last four do not (TS passes undefined →
	// Go threads 0, and their impls ignore the extra args anyway).
	type typeEntry struct {
		name   string
		offset bool // thread the real modelRenameOffset (TS lines 341-345)
		fn     func(*ConfigIdx, int, *ConfigIdx, int) ([]string, error)
	}

	typeList := []typeEntry{
		{"loc", true, func(idx *ConfigIdx, id int, _ *ConfigIdx, _ int) ([]string, error) {
			// TS unpackLocConfig keeps the (config, id) signature — the extra args are unused.
			return env.UnpackLoc(idx, id)
		}},
		{"obj", true, func(idx *ConfigIdx, id int, cmp *ConfigIdx, off int) ([]string, error) {
			return env.UnpackObj(idx, id, cmp, off), nil
		}},
		{"spotanim", true, func(idx *ConfigIdx, id int, cmp *ConfigIdx, off int) ([]string, error) {
			return env.UnpackSpotAnim(idx, id, cmp, off), nil
		}},
		{"idk", true, func(idx *ConfigIdx, id int, cmp *ConfigIdx, off int) ([]string, error) {
			return env.UnpackIdk(idx, id, cmp, off), nil
		}},
		{"npc", true, func(idx *ConfigIdx, id int, cmp *ConfigIdx, off int) ([]string, error) {
			return env.UnpackNpc(idx, id, cmp, off), nil
		}},
		{"seq", false, func(idx *ConfigIdx, id int, _ *ConfigIdx, _ int) ([]string, error) {
			return env.UnpackSeq(idx, id), nil
		}},
		{"flo", false, func(idx *ConfigIdx, id int, _ *ConfigIdx, _ int) ([]string, error) {
			return env.UnpackFlo(idx, id), nil
		}},
		{"varp", false, func(idx *ConfigIdx, id int, _ *ConfigIdx, _ int) ([]string, error) {
			return env.UnpackVarp(idx, id), nil
		}},
		{"varbit", false, func(idx *ConfigIdx, id int, _ *ConfigIdx, _ int) ([]string, error) {
			return env.UnpackVarbit(idx, id), nil
		}},
	}

	for _, te := range typeList {
		off := 0
		if te.offset {
			off = modelRenameOffset
		}
		if err := unpackConfig(opts.Revision, te.name, te.fn, jag, jag2, off, opts.SrcDir, printInfo); err != nil {
			return fmt.Errorf("unpackConfig %s: %w", te.name, err)
		}
	}

	// TS line 363: ModelPack.save()
	if err := env.Model.Save(); err != nil {
		return fmt.Errorf("save model pack: %w", err)
	}

	// TS line 365: printInfo('Done! Manual post processing may be required.')
	printInfo("Done! Manual post processing may be required.")

	return nil
}

// newModelStoreFromCache creates a *model.Store populated from cache archive 1.
// Mirrors TS Unpack.ts:318-321.
func newModelStoreFromCache(fs *filestream.FileStream, modelMax int) *model.Store {
	store := model.New()
	for id := range modelMax {
		data := fs.Read(1, id, true)
		store.Unpack(id, data)
	}
	return store
}

// buildEnv constructs an Env from the given Registry, wiring all pack files
// and the model store. Warnf is wired to write bare message+"\n" to out;
// Errorf is wired to the caller-supplied errorf.
func buildEnv(reg *pack.Registry, srcDir string, modelStore *model.Store, out io.Writer, errorf func(string, ...any)) (*Env, error) {
	loc, err := reg.EnsureLoc()
	if err != nil {
		return nil, fmt.Errorf("ensure loc: %w", err)
	}
	npc, err := reg.EnsureNpc()
	if err != nil {
		return nil, fmt.Errorf("ensure npc: %w", err)
	}
	obj, err := reg.EnsureObj()
	if err != nil {
		return nil, fmt.Errorf("ensure obj: %w", err)
	}
	seq, err := reg.EnsureSeq()
	if err != nil {
		return nil, fmt.Errorf("ensure seq: %w", err)
	}
	idk, err := reg.EnsureIdk()
	if err != nil {
		return nil, fmt.Errorf("ensure idk: %w", err)
	}
	flo, err := reg.EnsureFlo()
	if err != nil {
		return nil, fmt.Errorf("ensure flo: %w", err)
	}
	varp, err := reg.EnsureVarp()
	if err != nil {
		return nil, fmt.Errorf("ensure varp: %w", err)
	}
	varbit, err := reg.EnsureVarbit()
	if err != nil {
		return nil, fmt.Errorf("ensure varbit: %w", err)
	}
	spotanim, err := reg.EnsureSpotAnim()
	if err != nil {
		return nil, fmt.Errorf("ensure spotanim: %w", err)
	}
	texture, err := reg.EnsureTexture()
	if err != nil {
		return nil, fmt.Errorf("ensure texture: %w", err)
	}
	anim, err := reg.EnsureAnim()
	if err != nil {
		return nil, fmt.Errorf("ensure anim: %w", err)
	}
	model, err := reg.EnsureModel()
	if err != nil {
		return nil, fmt.Errorf("ensure model: %w", err)
	}

	env := &Env{
		Flo:      flo,
		Texture:  texture,
		Varp:     varp,
		Varbit:   varbit,
		Seq:      seq,
		Anim:     anim,
		Obj:      obj,
		Model:    model,
		Npc:      npc,
		Loc:      loc,
		SpotAnim: spotanim,
		Idk:      idk,
		Models:   modelStore,
		SrcDir:   srcDir,
		Warnf: func(format string, args ...any) {
			if out != nil {
				fmt.Fprintf(out, format+"\n", args...)
			}
		},
		Errorf: errorf,
	}

	return env, nil
}
