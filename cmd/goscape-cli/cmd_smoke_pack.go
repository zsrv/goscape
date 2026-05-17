package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

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

	// Out-dir lifecycle, stage driver, summary printer land in later tasks.
	_ = outDir
	_ = dataPackDir
	_ = keep
	_ = stopOnError
	_ = stdout
	logger.Info("smoke-pack skeleton — stage driver lands in subsequent tasks")
	return 0
}
