package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"

	"github.com/zsrv/goscape/pkg/unpack/checksum"
	"github.com/zsrv/goscape/pkg/unpack/clientinterface"
	"github.com/zsrv/goscape/pkg/unpack/config"
	"github.com/zsrv/goscape/pkg/unpack/graphics"
	"github.com/zsrv/goscape/pkg/unpack/maps"
	"github.com/zsrv/goscape/pkg/unpack/midi"
	"github.com/zsrv/goscape/pkg/unpack/sound"
	"github.com/zsrv/goscape/pkg/unpack/sprite"
	"github.com/zsrv/goscape/pkg/unpack/versionlist"
	"github.com/zsrv/goscape/pkg/unpack/worldmap"
	"github.com/zsrv/goscape/pkg/util/log"
)

// runUnpack implements the `unpack` verb: dispatches to one of 16 family
// functions that extract a client cache into Content-tree sources.
//
// args[0] is the family name (consumed before flag.Parse, mirroring runJag's
// sub-verb pattern). Remaining args are parsed as flags.
//
// stdout receives print-tool output (Out / console.log channels).
// stderr receives slog logger output (Errorf sinks, error diagnostics).
//
// Exit codes:
//
//	0 — success (or `-h`/`--help`)
//	1 — runtime error returned by the family library
//	2 — missing/unknown family, or flag-parse error
func runUnpack(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "unpack: missing family")
		fmt.Fprintln(stderr)
		unpackUsage(stderr)
		return 2
	}

	family, rest := args[0], args[1:]
	switch family {
	case "-h", "--help", "help":
		unpackUsage(stdout)
		return 0
	}

	switch family {
	case "config":
		return runUnpackConfig(rest, stdout, stderr)
	case "interface":
		return runUnpackInterface(rest, stdout, stderr)
	case "map":
		return runUnpackMap(rest, stdout, stderr)
	case "midi":
		return runUnpackMidi(rest, stdout, stderr)
	case "sound":
		return runUnpackSound(rest, stdout, stderr)
	case "models":
		return runUnpackModels(rest, stdout, stderr)
	case "anims":
		return runUnpackAnims(rest, stdout, stderr)
	case "sprite-media":
		return runUnpackSpriteMedia(rest, stdout, stderr)
	case "sprite-textures":
		return runUnpackSpriteTextures(rest, stdout, stderr)
	case "sprite-title":
		return runUnpackSpriteTitle(rest, stdout, stderr)
	case "versionlist-anim":
		return runUnpackVersionlistAnim(rest, stdout, stderr)
	case "versionlist-midi":
		return runUnpackVersionlistMidi(rest, stdout, stderr)
	case "versionlist-model":
		return runUnpackVersionlistModel(rest, stdout, stderr)
	case "worldmap":
		return runUnpackWorldmap(rest, stdout, stderr)
	case "checksum":
		return runUnpackChecksum(rest, stdout, stderr)
	case "compare":
		return runUnpackCompare(rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unpack: unknown family %q\n\n", family)
		unpackUsage(stderr)
		return 2
	}
}

func unpackUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: goscape-cli unpack <family> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Families:")
	fmt.Fprintln(w, "  config           Unpack entity config files (npc/obj/loc/seq/…).")
	fmt.Fprintln(w, "  interface        Unpack client interface tree.")
	fmt.Fprintln(w, "  map              Unpack map tiles and location definitions.")
	fmt.Fprintln(w, "  midi             Unpack MIDI songs and jingles.")
	fmt.Fprintln(w, "  sound            Unpack synthesised sound instruments.")
	fmt.Fprintln(w, "  models           Unpack 3-D model files.")
	fmt.Fprintln(w, "  anims            Unpack animation-set files.")
	fmt.Fprintln(w, "  sprite-media     Unpack media sprites.")
	fmt.Fprintln(w, "  sprite-textures  Unpack texture sprites.")
	fmt.Fprintln(w, "  sprite-title     Unpack title-screen sprites.")
	fmt.Fprintln(w, "  versionlist-anim Print anim_index entries from the versionlist.")
	fmt.Fprintln(w, "  versionlist-midi Print midi_index entries from the versionlist.")
	fmt.Fprintln(w, "  versionlist-model Print model_index entries from the versionlist.")
	fmt.Fprintln(w, "  worldmap         Unpack worldmap data.")
	fmt.Fprintln(w, "  checksum         Print per-member CRC32s and extract cache jags.")
	fmt.Fprintln(w, "  compare          Compare packed output against unpacked cache.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Common flags (most families):")
	fmt.Fprintln(w, "  -cache-dir  string   Client cache directory (default: data/unpack).")
	fmt.Fprintln(w, "  -src-dir    string   Content source tree root (default: data/src).")
	fmt.Fprintln(w, "  -pack-dir   string   Pack output directory for compare/worldmap/config (default: data/pack).")
	fmt.Fprintln(w, "                       For config: merge-compare path used only when <pack-dir>/main_file_cache.dat exists.")
	fmt.Fprintln(w, "  -revision   string   Revision tag embedded in config output path (default: 245; config only).")
	fmt.Fprintln(w, "  -type       string   Config type for compare (default: npc; compare only).")
	fmt.Fprintln(w, "  -log.level  string   Log severity (debug|info|warn|error; default: info).")
	fmt.Fprintln(w, "  -log.format string   Log format (text|json; default: text).")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run `goscape-cli unpack <family> -h` for family-specific flags.")
}

// ---- shared flag-set builder ----

// unpackFlags is the common set of flags shared across most families.
type unpackFlags struct {
	cacheDir  string
	srcDir    string
	packDir   string
	revision  string
	typ       string
	logLevel  slog.Level
	logFormat string
}

// parseUnpackFlags builds and parses a FlagSet for the unpack sub-commands.
// Returns nil when the caller should return the given exit code immediately
// (-h → 0, parse error → 2).
func parseUnpackFlags(name string, args []string, stderr io.Writer, showPackDir, showRevision, showType bool) (*unpackFlags, int) {
	fs := flag.NewFlagSet("unpack "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)

	f := &unpackFlags{}
	fs.StringVar(&f.cacheDir, "cache-dir", "data/unpack",
		"Client cache directory (main_file_cache.dat/idx0-4).")
	fs.StringVar(&f.srcDir, "src-dir", "data/src",
		"Content source tree root (output files written here).")
	if showPackDir {
		fs.StringVar(&f.packDir, "pack-dir", "data/pack",
			"Pack output directory (compare/worldmap/config merge path).\n\t\tFor config: the merge-compare path is used only when <pack-dir>/main_file_cache.dat exists.")
	}
	if showRevision {
		fs.StringVar(&f.revision, "revision", "245",
			"Revision tag embedded in config output path (scripts/_unpack/<revision>/…).")
	}
	if showType {
		fs.StringVar(&f.typ, "type", "npc",
			"Config type to compare (e.g. npc, obj, loc); default: npc.")
	}

	f.logLevel = slog.LevelInfo
	fs.TextVar(&f.logLevel, "log.level", f.logLevel,
		"Log severity (debug|info|warn|error).")
	fs.StringVar(&f.logFormat, "log.format", "text",
		"Log format (text|json).")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, 0
		}
		return nil, 2
	}
	return f, -1
}

// newLogger creates a slog logger writing to stderr, returning exit 1 on failure.
func newUnpackLogger(f *unpackFlags, stderr io.Writer) (*slog.Logger, int) {
	logger, err := log.NewLogger(f.logLevel, f.logFormat, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "failed to create logger: %v\n", err)
		return nil, 1
	}
	return logger, -1
}

// errorfFromLogger returns an Errorf func that logs at Error level.
func errorfFromLogger(logger *slog.Logger) func(string, ...any) {
	return func(format string, args ...any) {
		logger.Error(fmt.Sprintf(format, args...))
	}
}

// ---- family handlers ----

func runUnpackConfig(args []string, stdout, stderr io.Writer) int {
	f, code := parseUnpackFlags("config", args, stderr, true, true, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	opts := config.Options{
		CacheDir: f.cacheDir,
		PackDir:  f.packDir,
		SrcDir:   f.srcDir,
		Revision: f.revision,
		Out:      stdout,
		Errorf:   errorfFromLogger(logger),
	}
	if err := config.Unpack(opts); err != nil {
		logger.Error("unpack config failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackInterface(args []string, stdout, stderr io.Writer) int {
	f, code := parseUnpackFlags("interface", args, stderr, false, false, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	opts := clientinterface.Options{
		CacheDir: f.cacheDir,
		SrcDir:   f.srcDir,
		Out:      stdout,
		Errorf:   errorfFromLogger(logger),
	}
	if err := clientinterface.Unpack(opts); err != nil {
		logger.Error("unpack interface failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackMap(args []string, stdout, stderr io.Writer) int {
	f, code := parseUnpackFlags("map", args, stderr, false, false, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	opts := maps.Options{
		CacheDir: f.cacheDir,
		SrcDir:   f.srcDir,
		Out:      stdout,
		Errorf:   errorfFromLogger(logger),
	}
	if err := maps.Unpack(opts); err != nil {
		logger.Error("unpack map failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackMidi(args []string, stdout, stderr io.Writer) int {
	f, code := parseUnpackFlags("midi", args, stderr, false, false, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	opts := midi.Options{
		CacheDir: f.cacheDir,
		SrcDir:   f.srcDir,
		Out:      stdout,
	}
	if err := midi.Unpack(opts); err != nil {
		logger.Error("unpack midi failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackSound(args []string, stdout, stderr io.Writer) int {
	f, code := parseUnpackFlags("sound", args, stderr, false, false, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	opts := sound.Options{
		CacheDir: f.cacheDir,
		SrcDir:   f.srcDir,
		Out:      stdout,
	}
	if err := sound.Unpack(opts); err != nil {
		logger.Error("unpack sound failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackModels(args []string, stdout, stderr io.Writer) int {
	f, code := parseUnpackFlags("models", args, stderr, false, false, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	opts := graphics.Options{
		CacheDir: f.cacheDir,
		SrcDir:   f.srcDir,
		Out:      stdout,
	}
	if err := graphics.Models(opts); err != nil {
		logger.Error("unpack models failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackAnims(args []string, stdout, stderr io.Writer) int {
	f, code := parseUnpackFlags("anims", args, stderr, false, false, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	opts := graphics.Options{
		CacheDir: f.cacheDir,
		SrcDir:   f.srcDir,
		Out:      stdout,
	}
	if err := graphics.Anims(opts); err != nil {
		logger.Error("unpack anims failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackSpriteMedia(args []string, stdout, stderr io.Writer) int {
	_ = stdout // sprite families have no Out channel
	f, code := parseUnpackFlags("sprite-media", args, stderr, false, false, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	opts := sprite.Options{
		CacheDir: f.cacheDir,
		SrcDir:   f.srcDir,
		Errorf:   errorfFromLogger(logger),
	}
	if err := sprite.Media(opts); err != nil {
		logger.Error("unpack sprite-media failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackSpriteTextures(args []string, stdout, stderr io.Writer) int {
	_ = stdout
	f, code := parseUnpackFlags("sprite-textures", args, stderr, false, false, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	opts := sprite.Options{
		CacheDir: f.cacheDir,
		SrcDir:   f.srcDir,
		Errorf:   errorfFromLogger(logger),
	}
	if err := sprite.Textures(opts); err != nil {
		logger.Error("unpack sprite-textures failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackSpriteTitle(args []string, stdout, stderr io.Writer) int {
	_ = stdout
	f, code := parseUnpackFlags("sprite-title", args, stderr, false, false, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	opts := sprite.Options{
		CacheDir: f.cacheDir,
		SrcDir:   f.srcDir,
		Errorf:   errorfFromLogger(logger),
	}
	if err := sprite.Title(opts); err != nil {
		logger.Error("unpack sprite-title failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackVersionlistAnim(args []string, stdout, stderr io.Writer) int {
	f, code := parseUnpackFlags("versionlist-anim", args, stderr, false, false, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	if err := versionlist.AnimIndex(f.cacheDir, stdout); err != nil {
		logger.Error("unpack versionlist-anim failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackVersionlistMidi(args []string, stdout, stderr io.Writer) int {
	f, code := parseUnpackFlags("versionlist-midi", args, stderr, false, false, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	if err := versionlist.MidiIndex(f.cacheDir, f.srcDir, stdout); err != nil {
		logger.Error("unpack versionlist-midi failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackVersionlistModel(args []string, stdout, stderr io.Writer) int {
	f, code := parseUnpackFlags("versionlist-model", args, stderr, false, false, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	if err := versionlist.ModelIndex(f.cacheDir, f.srcDir, stdout); err != nil {
		logger.Error("unpack versionlist-model failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackWorldmap(args []string, stdout, stderr io.Writer) int {
	f, code := parseUnpackFlags("worldmap", args, stderr, true, false, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	if err := worldmap.Unpack(f.cacheDir, f.packDir, f.srcDir, stdout); err != nil {
		logger.Error("unpack worldmap failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackChecksum(args []string, stdout, stderr io.Writer) int {
	f, code := parseUnpackFlags("checksum", args, stderr, false, false, false)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	if err := checksum.Run(f.cacheDir, stdout); err != nil {
		logger.Error("unpack checksum failed", "err", err)
		return 1
	}
	return 0
}

func runUnpackCompare(args []string, stdout, stderr io.Writer) int {
	f, code := parseUnpackFlags("compare", args, stderr, true, false, true)
	if code != -1 {
		return code
	}
	logger, code := newUnpackLogger(f, stderr)
	if code != -1 {
		return code
	}

	if err := config.Compare(f.cacheDir, f.packDir, f.typ, stdout); err != nil {
		logger.Error("unpack compare failed", "err", err)
		return 1
	}
	return 0
}
