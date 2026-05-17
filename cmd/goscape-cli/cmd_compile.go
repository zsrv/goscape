package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/pack/compiler"
	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
	"github.com/zsrv/goscape/pkg/util/log"
)

// runCompile implements the `compile` verb: parses flags, loads the
// compiler symbol set via compiler.LoadCompilerSymbols, and invokes
// runescript.Compile on a single .rs2 source file.
//
// stderr receives both flag-parse error output and slog logger
// output. stdout is unused — compile has no human-readable success
// output beyond the logger.
//
// Exit codes:
//
//	0 — success (or `-h`/`--help` print)
//	1 — symbol load failed, compile failed, logger init failed, or
//	    temp-dir creation failed
//	2 — flag parse error or missing/extra positional argument
func runCompile(args []string, stdout, stderr io.Writer) int {
	_ = stdout // unused
	fs := flag.NewFlagSet("compile", flag.ContinueOnError)
	fs.SetOutput(stderr)

	srcDir := fs.String("src-dir", "data/src",
		"Source content directory.")
	dataPackDir := fs.String("datapack-dir", "",
		"Entity-type cache directory (default: data/pack).")
	check := fs.Bool("check", false,
		"Diagnostics-only mode; discard compiler output.")
	outDir := fs.String("out-dir", "data/pack",
		"Output directory (ignored when --check is set).")

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

	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(stderr, "compile: expected exactly one source path")
		return 2
	}
	path := rest[0]

	logger, err := log.NewLogger(logLevel, *logFormat, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "failed to create logger: %v\n", err)
		return 1
	}

	dpd := *dataPackDir
	if dpd == "" {
		dpd = "data/pack"
	}

	writerOut := filepath.Join(*outDir, "server")
	if *check {
		tmp, err := os.MkdirTemp("", "goscape-cli-compile-*")
		if err != nil {
			logger.Error("failed to create temp dir", "err", err)
			return 1
		}
		defer os.RemoveAll(tmp)
		writerOut = tmp
	}

	logger.Info("loading compiler symbols",
		"src_dir", *srcDir,
		"datapack_dir", dpd,
	)
	symbols, err := compiler.LoadCompilerSymbols(*srcDir, dpd)
	if err != nil {
		logger.Error("load symbols failed", "err", err)
		return 1
	}

	logger.Info("compiling",
		"path", path,
		"check", *check,
		"writer_out", writerOut,
	)
	cfg := runescript.Config{
		SourcePaths: []string{path},
		Symbols:     symbols,
		Writer: runescript.WriterConfig{
			Jag: &runescript.JagWriterConfig{Output: writerOut},
		},
	}
	if err := runescript.Compile(cfg); err != nil {
		logger.Error("compile failed", "err", err)
		return 1
	}
	logger.Info("compile succeeded")
	return 0
}
