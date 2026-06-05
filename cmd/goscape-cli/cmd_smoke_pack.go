package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
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
	"github.com/zsrv/goscape/pkg/pack/worldmap"
	"github.com/zsrv/goscape/pkg/packall"
	"github.com/zsrv/goscape/pkg/util/log"
)

// runSmokePack implements the `smoke-pack` verb: a best-effort driver
// over all PackAll stages (PackConfigs + downstream) plus the standalone
// Worldmap packer, with per-stage logging and an end-of-run summary table.
// See docs/superpowers/specs/2026-05-17-smoke-pack-verb-design.md for the
// design contract. The stages mirror pkg/packall/packall.go; Worldmap is
// appended last (TS parity keeps it out of PackAll itself but it produces a
// Jagfile worth byte-pinning against the TS reference).
//
// Exit codes:
//
//	0 — all stages succeeded (or `-h`/`--help`)
//	1 — at least one stage failed (best-effort) or first failing stage
//	    in --stop-on-error mode
//	2 — flag parse error
//	3 — setup error (missing/unreadable --content-dir, logger init, etc.)
func runSmokePack(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("smoke-pack", flag.ContinueOnError)
	fs.SetOutput(stderr)

	contentDir := fs.String("content-dir", "",
		"Source content directory (required).")
	outDir := fs.String("out-dir", "",
		"Output directory. Empty → auto-create temp dir (deleted on exit unless --keep).")
	dataPackDir := fs.String("datapack-dir", "",
		"Entity-type cache directory (default: effective --out-dir).")
	refDir := fs.String("reference-dir", "",
		"Reference pack output to byte-diff against (typically Engine-TS/data/pack). Empty → diff disabled. "+
			"Note: SourceName is embedded in server/script.dat as the literal path passed via --content-dir "+
			"(both goscape and TS are case-faithful to the build environment). For byte-faithful diffs against "+
			"a TS ref packed via Engine-TS' default ../content path, point --content-dir at a directory whose "+
			"absolute path matches the ref build's (e.g., the LostCityRS/content lowercase symlink).")
	keep := fs.Bool("keep", false,
		"Preserve auto-created --out-dir on exit.")
	stopOnError := fs.Bool("stop-on-error", false,
		"Exit at the first failing stage (default: log and continue).")

	var logLevel slog.Level = slog.LevelInfo
	fs.TextVar(&logLevel, "log.level", logLevel,
		"Log severity (debug|info|warn|error).")
	logFormat := fs.String("log.format", "text",
		"Log format (text|json).")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *contentDir == "" {
		fmt.Fprintln(stderr, "smoke-pack: --content-dir is required")
		return 3
	}
	if info, err := os.Stat(*contentDir); err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "smoke-pack: --content-dir %q is not a readable directory\n", *contentDir)
		return 3
	}
	if *refDir != "" {
		if info, err := os.Stat(*refDir); err != nil || !info.IsDir() {
			fmt.Fprintf(stderr, "smoke-pack: --reference-dir %q is not a readable directory\n", *refDir)
			return 3
		}
	}

	logger, err := log.NewLogger(logLevel, *logFormat, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "smoke-pack: logger init failed: %v\n", err)
		return 3
	}

	effectiveOut := *outDir
	autoCreated := false
	if effectiveOut == "" {
		tmp, mkErr := os.MkdirTemp("", "goscape-smoke-pack-*")
		if mkErr != nil {
			fmt.Fprintf(stderr, "smoke-pack: mkdtemp failed: %v\n", mkErr)
			return 3
		}
		effectiveOut = tmp
		autoCreated = true
		defer func() {
			if !*keep {
				_ = os.RemoveAll(effectiveOut)
			}
		}()
	}
	effectiveDataPack := *dataPackDir
	if effectiveDataPack == "" {
		effectiveDataPack = effectiveOut
	}
	runStart := time.Now()
	results := runStages(*contentDir, effectiveOut, effectiveDataPack, "data/raw", *refDir, *stopOnError, logger)
	totalElapsed := time.Since(runStart)
	printSummary(stdout, results, totalElapsed, *refDir != "")

	suffix := ""
	if autoCreated {
		if *keep {
			suffix = " (kept; --keep)"
		} else {
			suffix = " (auto-deleted)"
		}
	}
	fmt.Fprintf(stdout, "out-dir: %s%s\n", effectiveOut, suffix)

	anyErr := false
	for _, r := range results {
		if r.Status == stageErr {
			anyErr = true
			break
		}
	}
	if anyErr {
		return 1
	}
	return 0
}

// printSummary renders the per-stage report + result line to w.
// elapsed is the whole-run wall clock for the Result line. When
// withDiff is true, a DIFF column is inserted between BYTES and ERR
// and a "Diff details:" block is appended after the Result line.
func printSummary(w io.Writer, results []stageResult, elapsed time.Duration, withDiff bool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if withDiff {
		fmt.Fprintln(tw, "STAGE\tSTATUS\tELAPSED\tFILES\tBYTES\tDIFF\tERR")
	} else {
		fmt.Fprintln(tw, "STAGE\tSTATUS\tELAPSED\tFILES\tBYTES\tERR")
	}
	var ok, errCount, skip int
	for _, r := range results {
		switch r.Status {
		case stageOK:
			ok++
		case stageErr:
			errCount++
		case stageSkip:
			skip++
		}
		elapsedStr := "-"
		filesStr := "-"
		bytesStr := "-"
		diffStr := "-"
		errStr := ""
		if r.Status != stageSkip {
			elapsedStr = r.Elapsed.Round(time.Millisecond).String()
			filesStr = fmt.Sprintf("%d", r.OutputFiles)
			bytesStr = fmt.Sprintf("%d", r.OutputBytes)
		}
		if withDiff && r.Status == stageOK {
			diffStr = fmt.Sprintf("%d", len(r.Diffs))
		}
		if r.Err != nil {
			errStr = r.Err.Error()
		}
		if withDiff {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Name, r.Status, elapsedStr, filesStr, bytesStr, diffStr, errStr)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Name, r.Status, elapsedStr, filesStr, bytesStr, errStr)
		}
	}
	tw.Flush()
	fmt.Fprintf(w, "\nResult: %d OK, %d ERR, %d SKIP  total elapsed: %s\n", ok, errCount, skip, elapsed.Round(time.Millisecond))
	if withDiff {
		printDiffDetails(w, results)
	}
}

// printDiffDetails emits a "Diff details:" block when any stage carried
// non-empty Diffs. Each stage's diffs are capped at diffDetailsPerStage
// lines; truncated entries are summarised in a trailing "... +K more"
// line aggregated across stages.
func printDiffDetails(w io.Writer, results []stageResult) {
	anyDiffs := false
	for _, r := range results {
		if len(r.Diffs) > 0 {
			anyDiffs = true
			break
		}
	}
	if !anyDiffs {
		return
	}
	fmt.Fprintln(w, "\nDiff details:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	var truncated int
	var truncatedStages int
	for _, r := range results {
		if len(r.Diffs) == 0 {
			continue
		}
		shown := 0
		for _, d := range r.Diffs {
			if shown >= diffDetailsPerStage {
				break
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", r.Name, d.Kind, d.Path, formatDiffSuffix(d))
			shown++
		}
		if len(r.Diffs) > shown {
			truncated += len(r.Diffs) - shown
			truncatedStages++
		}
	}
	tw.Flush()
	if truncated > 0 {
		fmt.Fprintf(w, "  ... +%d more across %d stage(s)\n", truncated, truncatedStages)
	}
}

const diffDetailsPerStage = 10

// formatDiffSuffix renders the kind-specific tail of a Diff details line
// (offset/got/want, out/ref sizes, MISS marker, or ERR note).
func formatDiffSuffix(d fileDiff) string {
	switch d.Kind {
	case "DIFF":
		return fmt.Sprintf("offset=%d got=%#x want=%#x", d.Offset, d.Got, d.Want)
	case "SIZE":
		return fmt.Sprintf("out=%d ref=%d", d.OutSize, d.RefSize)
	case "MISS":
		return "(absent from reference)"
	case "ERR":
		return d.Note
	}
	return ""
}

type stageStatus int

const (
	stageOK stageStatus = iota
	stageErr
	stageSkip
)

func (s stageStatus) String() string {
	switch s {
	case stageOK:
		return "OK"
	case stageErr:
		return "ERR"
	case stageSkip:
		return "SKIP"
	}
	return "?"
}

type stageResult struct {
	Name        string
	Status      stageStatus
	Elapsed     time.Duration
	OutputFiles int
	OutputBytes int64
	Err         error
	// Diffs is the per-stage list of byte-divergences from --reference-dir.
	// Populated only when --reference-dir is set and the stage succeeded;
	// nil otherwise. Files are attributed to the stage that most recently
	// modified them via the snapshot-delta machinery in runStages.
	Diffs []fileDiff
}

// walkOutDir returns (fileCount, totalBytes) for regular files under
// dir. Missing dir → (0, 0, nil). Other errors propagate so the driver
// can log them but still record the stage.
func walkOutDir(dir string) (int, int64, error) {
	var files int
	var totalBytes int64
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.Type().IsRegular() {
			info, statErr := d.Info()
			if statErr != nil {
				return statErr
			}
			files++
			totalBytes += info.Size()
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return files, totalBytes, err
	}
	return files, totalBytes, nil
}

// safeRun invokes fn and converts any panic into an error so a single
// crashing stage cannot halt the rest of the best-effort smoke run.
// Without this, a panic inside a stage (e.g. an EOF panic in a packet
// parser) would unwind past runStages and abort the whole binary,
// hiding downstream divergences from the operator.
func safeRun(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return fn()
}

// runStages drives all PackAll stages plus the standalone Worldmap packer
// best-effort. PackConfigs is special: if it fails, all downstream stages
// render as SKIP because they consume the *pack.Registry it produces. When
// refDir != "", every successful stage is also byte-diffed against the same
// relpath under refDir; results are attached to stageResult.Diffs.
func runStages(srcDir, outDir, dataPackDir, rawDir, refDir string, stopOnError bool, logger *slog.Logger) []stageResult {
	pack.ClearFsCache()

	// Create the FileStream cache (createNew=true, matching TS PackAll.ts:43).
	// The cache is shared across all stages that write to it; closed after all
	// stages complete (including Worldmap which does not use it).
	cache := filestream.New(outDir, true, false)
	defer cache.Close()

	results := make([]stageResult, 0, 16)

	// prevSnapshot is the outDir state after the most recently
	// successful stage. Empty when refDir is unset (snapshots are
	// only useful for diffing).
	var prevSnapshot stageSnapshot
	if refDir != "" {
		prevSnapshot, _ = snapshotOutDir(outDir)
	}

	// PackConfigs: special — owns reg + modelFlags allocation.
	logger.Info("stage_start", "stage", "PackConfigs")
	pcStart := time.Now()
	var reg *pack.Registry
	var modelFlags []int
	pcErr := safeRun(func() error {
		reg = &pack.Registry{SrcDir: srcDir}
		if _, err := reg.EnsureModel(); err != nil {
			return err
		}
		modelFlags = make([]int, reg.Model.Max)
		return pack.PackConfigsForPackAll(srcDir, outDir, reg, modelFlags, cache)
	})
	pcElapsed := time.Since(pcStart)
	pcFiles, pcBytes, _ := walkOutDir(outDir)
	if pcErr != nil || reg == nil {
		if pcErr == nil {
			pcErr = fmt.Errorf("PackConfigsForPackAll returned nil registry")
		}
		logger.Error("stage_err", "stage", "PackConfigs", "elapsed_ms", pcElapsed.Milliseconds(), "files", pcFiles, "bytes", pcBytes, "err", pcErr)
		results = append(results, stageResult{Name: "PackConfigs", Status: stageErr, Elapsed: pcElapsed, OutputFiles: pcFiles, OutputBytes: pcBytes, Err: pcErr})
		for _, name := range []string{
			"ClientInterface", "RunServerCompiler", "Title", "Media", "Texture",
			"Wordenc", "Sound", "Graphics", "Midi", "Maps",
			"VersionList", "BuildStamp", "OndemandZip", "Worldmap",
		} {
			results = append(results, stageResult{Name: name, Status: stageSkip})
		}
		return results
	}
	pcDiffs, pcPrev := computeStageDiffs(refDir, outDir, prevSnapshot)
	prevSnapshot = pcPrev
	logger.Info("stage_done", "stage", "PackConfigs", "elapsed_ms", pcElapsed.Milliseconds(), "files", pcFiles, "bytes", pcBytes)
	results = append(results, stageResult{Name: "PackConfigs", Status: stageOK, Elapsed: pcElapsed, OutputFiles: pcFiles, OutputBytes: pcBytes, Diffs: pcDiffs})

	type stage struct {
		name string
		run  func() error
	}
	rest := []stage{
		{"ClientInterface", func() error { return clientinterface.Pack(reg, srcDir, outDir, cache) }},
		{"RunServerCompiler", func() error { return compiler.RunServerCompiler(srcDir, outDir, dataPackDir) }},
		{"Title", func() error { return sprites.PackTitle(srcDir, outDir, cache) }},
		{"Media", func() error { return sprites.PackMedia(srcDir, outDir, cache) }},
		{"Texture", func() error { return sprites.PackTexture(reg, srcDir, outDir, cache) }},
		{"Wordenc", func() error { return wordenc.Pack(rawDir, cache) }},
		{"Sound", func() error { return audio.PackSound(reg, srcDir, outDir, cache) }},
		{"Graphics", func() error { return graphics.Pack(reg, srcDir, modelFlags, cache, nil) }},
		{"Midi", func() error { return audio.PackMidi(reg, srcDir, cache) }},
		{"Maps", func() error {
			if _, err := reg.EnsureMap(); err != nil {
				return err
			}
			return maps.Pack(srcDir, outDir, reg.Map, cache, modelFlags)
		}},
		{"VersionList", func() error { return versionlist.Pack(reg, srcDir, outDir, modelFlags, cache) }},
		{"BuildStamp", func() error { return packall.WriteServerBuild(outDir) }},
		{"OndemandZip", func() error { return packall.WriteOndemandZip(outDir, cache) }},
		{"Worldmap", func() error { return worldmap.Pack(srcDir, outDir) }},
	}
	for i, st := range rest {
		logger.Info("stage_start", "stage", st.name)
		start := time.Now()
		err := safeRun(st.run)
		elapsed := time.Since(start)
		files, bytesSum, _ := walkOutDir(outDir)
		if err != nil {
			logger.Error("stage_err", "stage", st.name, "elapsed_ms", elapsed.Milliseconds(), "files", files, "bytes", bytesSum, "err", err)
			results = append(results, stageResult{Name: st.name, Status: stageErr, Elapsed: elapsed, OutputFiles: files, OutputBytes: bytesSum, Err: err})
			if stopOnError {
				for _, remaining := range rest[i+1:] {
					results = append(results, stageResult{Name: remaining.name, Status: stageSkip})
				}
				return results
			}
			continue
		}
		diffs, nextSnap := computeStageDiffs(refDir, outDir, prevSnapshot)
		prevSnapshot = nextSnap
		logger.Info("stage_done", "stage", st.name, "elapsed_ms", elapsed.Milliseconds(), "files", files, "bytes", bytesSum)
		results = append(results, stageResult{Name: st.name, Status: stageOK, Elapsed: elapsed, OutputFiles: files, OutputBytes: bytesSum, Diffs: diffs})
	}
	return results
}

// computeStageDiffs is a no-op when refDir is empty. Otherwise it
// snapshots outDir, computes the delta against prev, and byte-diffs
// each added/modified file against the same relpath under refDir.
// Returns the diff list and the new snapshot (for use as `prev` in the
// next stage).
//
// main_file_cache.* files are excluded from the per-stage diff: the RS2
// client cache grows incrementally across all stages, so comparing an
// intermediate stage snapshot against a completed refDir always produces
// spurious SIZE diffs. The cache files are effectively the "sum" of all
// stage writes; final parity is already implied by the ondemand.zip and
// the per-archive content visible after all stages complete.
func computeStageDiffs(refDir, outDir string, prev stageSnapshot) ([]fileDiff, stageSnapshot) {
	if refDir == "" {
		return nil, nil
	}
	next, _ := snapshotOutDir(outDir)
	delta := deltaFiles(prev, next)
	var diffs []fileDiff
	for _, rel := range delta {
		// Skip RS2 client cache files: they accumulate writes across every stage,
		// so per-stage size comparison against a completed refDir is meaningless.
		base := filepath.Base(rel)
		if strings.HasPrefix(base, "main_file_cache.") {
			continue
		}
		d, err := diffOneFile(filepath.Join(outDir, rel), filepath.Join(refDir, rel))
		if err != nil {
			diffs = append(diffs, fileDiff{Path: rel, Kind: "ERR", Note: err.Error()})
			continue
		}
		if d != nil {
			d.Path = rel
			diffs = append(diffs, *d)
		}
	}
	return diffs, next
}
