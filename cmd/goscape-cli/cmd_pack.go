package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"

	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/util/log"
)

// runPack implements the `pack` verb: parses flags, calls
// pack.PackAll, returns an exit code.
//
// stderr receives both flag-parse error output and slog logger
// output. stdout is unused — pack has no human-readable success
// output beyond the logger.
//
// Exit codes:
//
//	0 — success (or `-h`/`--help` print)
//	1 — logger init failed or pack.PackAll returned an error
//	2 — flag parse error
func runPack(args []string, stdout, stderr io.Writer) int {
	_ = stdout // unused
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	fs.SetOutput(stderr)

	srcDir := fs.String("src-dir", "data/src",
		"Source content directory.")
	outDir := fs.String("out-dir", "data/pack",
		"Output directory.")
	dataPackDir := fs.String("datapack-dir", "",
		"Entity-type cache directory (default: --out-dir).")

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

	logger, err := log.NewLogger(logLevel, *logFormat, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "failed to create logger: %v\n", err)
		return 1
	}

	dpd := *dataPackDir
	if dpd == "" {
		dpd = *outDir
	}

	logger.Info("packing",
		"src_dir", *srcDir,
		"out_dir", *outDir,
		"datapack_dir", dpd,
	)
	if err := pack.PackAll(*srcDir, *outDir, dpd); err != nil {
		logger.Error("pack failed", "err", err)
		return 1
	}
	logger.Info("pack succeeded")
	return 0
}
