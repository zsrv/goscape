package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"

	"github.com/zsrv/goscape/pkg/pack/worldmap"
	"github.com/zsrv/goscape/pkg/util/log"
)

// runWorldmap implements the `worldmap` verb: parses flags, calls
// worldmap.Pack, returns an exit code.
//
// stderr receives both flag-parse error output and slog logger
// output. stdout is unused — worldmap has no human-readable
// success output beyond the logger.
//
// Exit codes:
//
//	0 — success (or `-h`/`--help` print)
//	1 — logger init failed or worldmap.Pack returned an error
//	2 — flag parse error
func runWorldmap(args []string, stdout, stderr io.Writer) int {
	_ = stdout
	fs := flag.NewFlagSet("worldmap", flag.ContinueOnError)
	fs.SetOutput(stderr)

	srcDir := fs.String("src-dir", "data/src",
		"Source content directory (CSVs, fonts, sprites).")
	outDir := fs.String("out-dir", "data/pack",
		"Output directory (reads server/maps, writes mapview/worldmap.jag).")

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

	logger.Info("packing worldmap",
		"src_dir", *srcDir,
		"out_dir", *outDir,
	)
	if err := worldmap.Pack(*srcDir, *outDir); err != nil {
		logger.Error("worldmap pack failed", "err", err)
		return 1
	}
	logger.Info("worldmap pack succeeded")
	return 0
}
