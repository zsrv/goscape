package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/pack/audio"
	"github.com/zsrv/goscape/pkg/pack/clientinterface"
	"github.com/zsrv/goscape/pkg/pack/compiler"
	"github.com/zsrv/goscape/pkg/pack/graphics"
	"github.com/zsrv/goscape/pkg/pack/maps"
	"github.com/zsrv/goscape/pkg/pack/sprites"
	"github.com/zsrv/goscape/pkg/pack/wordenc"
	"github.com/zsrv/goscape/pkg/util/log"
)

// runSmokePack implements the `smoke-pack` verb: a best-effort driver
// over packall.PackAll's 10 stages with per-stage logging and an
// end-of-run summary table. See
// docs/superpowers/specs/2026-05-17-smoke-pack-verb-design.md for the
// design contract. Stage order mirrors pkg/packall/packall.go.
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

	logger, err := log.NewLogger(logLevel, *logFormat, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "smoke-pack: logger init failed: %v\n", err)
		return 3
	}

	// Out-dir lifecycle lands in Task 5; for now, --out-dir is required.
	if *outDir == "" {
		fmt.Fprintln(stderr, "smoke-pack: --out-dir is required (auto-create lands in a later task)")
		return 3
	}
	effectiveOut := *outDir
	effectiveDataPack := *dataPackDir
	if effectiveDataPack == "" {
		effectiveDataPack = effectiveOut
	}
	_ = keep
	_ = stopOnError
	_ = stdout

	results := runStages(*contentDir, effectiveOut, effectiveDataPack, logger)

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
}

// walkOutDir returns (fileCount, totalBytes) for regular files under
// dir. Missing dir → (0, 0, nil). Other errors propagate so the driver
// can log them but still record the stage.
func walkOutDir(dir string) (int, int64, error) {
	var files int
	var bytes int64
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
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
			bytes += info.Size()
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return files, bytes, err
	}
	return files, bytes, nil
}

// runStages drives the 10 PackAll stages best-effort. PackConfigs is
// special: if it fails, all downstream stages render as SKIP because
// they consume the *pack.Registry it produces.
func runStages(srcDir, outDir, dataPackDir string, logger *slog.Logger) []stageResult {
	pack.ClearFsCache()

	results := make([]stageResult, 0, 11)

	logger.Info("stage_start", "stage", "PackConfigs")
	pcStart := time.Now()
	reg, pcErr := pack.PackConfigsForRegistry(srcDir, outDir)
	pcElapsed := time.Since(pcStart)
	pcFiles, pcBytes, _ := walkOutDir(outDir)
	if pcErr != nil {
		logger.Error("stage_err", "stage", "PackConfigs", "elapsed_ms", pcElapsed.Milliseconds(), "files", pcFiles, "bytes", pcBytes, "err", pcErr)
		results = append(results, stageResult{Name: "PackConfigs", Status: stageErr, Elapsed: pcElapsed, OutputFiles: pcFiles, OutputBytes: pcBytes, Err: pcErr})
		for _, name := range []string{
			"ClientInterface", "RunServerCompiler", "Title", "Media", "Texture",
			"Wordenc", "Sound", "Graphics", "Midi", "Maps",
		} {
			results = append(results, stageResult{Name: name, Status: stageSkip})
		}
		return results
	}
	logger.Info("stage_done", "stage", "PackConfigs", "elapsed_ms", pcElapsed.Milliseconds(), "files", pcFiles, "bytes", pcBytes)
	results = append(results, stageResult{Name: "PackConfigs", Status: stageOK, Elapsed: pcElapsed, OutputFiles: pcFiles, OutputBytes: pcBytes})

	type stage struct {
		name string
		run  func() error
	}
	rest := []stage{
		{"ClientInterface", func() error { return clientinterface.Pack(reg, srcDir, outDir) }},
		{"RunServerCompiler", func() error { return compiler.RunServerCompiler(srcDir, outDir, dataPackDir) }},
		{"Title", func() error { return sprites.PackTitle(srcDir, outDir) }},
		{"Media", func() error { return sprites.PackMedia(srcDir, outDir) }},
		{"Texture", func() error { return sprites.PackTexture(reg, srcDir, outDir) }},
		{"Wordenc", func() error { return wordenc.Pack(srcDir, outDir) }},
		{"Sound", func() error { return audio.PackSound(reg, srcDir, outDir) }},
		{"Graphics", func() error { return graphics.Pack(reg, srcDir, outDir) }},
		{"Midi", func() error { return audio.PackMidi(srcDir, outDir) }},
		{"Maps", func() error { return maps.Pack(srcDir, outDir) }},
	}
	for _, st := range rest {
		logger.Info("stage_start", "stage", st.name)
		start := time.Now()
		err := st.run()
		elapsed := time.Since(start)
		files, bytesSum, _ := walkOutDir(outDir)
		if err != nil {
			logger.Error("stage_err", "stage", st.name, "elapsed_ms", elapsed.Milliseconds(), "files", files, "bytes", bytesSum, "err", err)
			results = append(results, stageResult{Name: st.name, Status: stageErr, Elapsed: elapsed, OutputFiles: files, OutputBytes: bytesSum, Err: err})
			continue
		}
		logger.Info("stage_done", "stage", st.name, "elapsed_ms", elapsed.Milliseconds(), "files", files, "bytes", bytesSum)
		results = append(results, stageResult{Name: st.name, Status: stageOK, Elapsed: elapsed, OutputFiles: files, OutputBytes: bytesSum})
	}
	return results
}
