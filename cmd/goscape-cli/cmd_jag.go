package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/jagfile"
)

// reorderFlagsFirst splits args into (flagsAndValues, positionals)
// then concatenates flags before positionals. The stdlib `flag`
// package stops parsing at the first non-flag token, so callers can
// freely interleave `--out X` after positional args. This helper
// lets us accept e.g. `[path entry --out value]` from the CLI.
//
// A "flag" token is one beginning with "-" (and not exactly "-",
// which is a stdin sentinel). For boolean flags `--foo`, the next
// token is treated as positional; for value flags `--foo bar` or
// `--foo=bar`, the next token (in the space-separated form) needs
// to follow the flag. Since all jag flags are string-valued and we
// only see `--out <value>` or `--out=<value>` shapes, we treat the
// token after a non-"=" flag as the flag value.
func reorderFlagsFirst(args []string, valueFlags map[string]bool) []string {
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && a != "-" && a != "--" {
			flags = append(flags, a)
			// If form is `--name value` (no `=`) and the flag name
			// expects a value, consume next arg too.
			if !strings.Contains(a, "=") {
				name := strings.TrimLeft(a, "-")
				if valueFlags[name] && i+1 < len(args) {
					flags = append(flags, args[i+1])
					i++
				}
			}
		} else {
			positionals = append(positionals, a)
		}
	}
	return append(flags, positionals...)
}

// runJag implements the `jag` verb: mini-dispatcher over the
// `list`, `extract`, and `dump` sub-verbs.
//
// stdout receives entry-listing or raw-byte content; stderr
// receives diagnostics and flag-parse errors.
//
// Exit codes:
//
//	0 — success (or `-h`/`--help` on the sub-verb)
//	1 — file-not-found, parse failure, missing entry, dir-not-empty,
//	    or write error
//	2 — flag parse error, missing/extra positional argument, or
//	    missing/unknown sub-verb
func runJag(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "jag: missing sub-verb (expected: list | extract | dump)")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return runJagList(rest, stdout, stderr)
	case "extract":
		return runJagExtract(rest, stdout, stderr)
	case "dump":
		return runJagDump(rest, stdout, stderr)
	case "-h", "--help", "help":
		jagUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "jag: unknown sub-verb %q\n\n", sub)
		jagUsage(stderr)
		return 2
	}
}

func jagUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: goscape-cli jag <sub-verb> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Sub-verbs:")
	fmt.Fprintln(w, "  list    <path.jag>                            List entries (name<TAB>unpacked<TAB>packed).")
	fmt.Fprintln(w, "  extract <path.jag> <entry> [--out <path>]     Extract one entry (default: stdout).")
	fmt.Fprintln(w, "  dump    <path.jag> --out <dir>                Extract every entry into <dir>.")
}

func runJagList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("jag list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(stderr, "jag list: expected exactly one path")
		return 2
	}
	jf, err := jagfile.LoadJagfile(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "jag list: %v\n", err)
		return 1
	}
	for i := 0; i < jf.FileCount; i++ {
		name := jf.FileName[i]
		if name == "" {
			name = fmt.Sprintf("0x%08x", jf.FileHash[i])
		}
		fmt.Fprintf(stdout, "%s\t%d\t%d\n",
			name, jf.FileUnpackedSize[i], jf.FilePackedSize[i])
	}
	return 0
}

func runJagExtract(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("jag extract", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "-", "Output path (\"-\" for stdout).")
	args = reorderFlagsFirst(args, map[string]bool{"out": true})
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	rest := fs.Args()
	if len(rest) != 2 {
		fmt.Fprintln(stderr, "jag extract: expected <path.jag> <entry>")
		return 2
	}
	path, entry := rest[0], rest[1]

	jf, err := jagfile.LoadJagfile(path)
	if err != nil {
		fmt.Fprintf(stderr, "jag extract: %v\n", err)
		return 1
	}
	pkt, err := jf.Read(entry)
	if err != nil {
		fmt.Fprintf(stderr, "jag extract: no such entry: %s\n", entry)
		return 1
	}

	var sink io.Writer = stdout
	if *out != "-" {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintf(stderr, "jag extract: %v\n", err)
			return 1
		}
		defer f.Close()
		sink = f
	}
	if _, err := sink.Write(pkt.Data); err != nil {
		fmt.Fprintf(stderr, "jag extract: write: %v\n", err)
		return 1
	}
	return 0
}

func runJagDump(args []string, stdout, stderr io.Writer) int {
	_ = stdout // unused; dump writes only files
	fs := flag.NewFlagSet("jag dump", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outDir := fs.String("out", "", "Output directory (required).")
	args = reorderFlagsFirst(args, map[string]bool{"out": true})
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(stderr, "jag dump: expected exactly one path")
		return 2
	}
	if *outDir == "" {
		fmt.Fprintln(stderr, "jag dump: --out is required")
		return 2
	}

	// Empty-dir safety check: refuse non-empty existing dir. Create
	// the dir if missing.
	if entries, err := os.ReadDir(*outDir); err == nil {
		if len(entries) > 0 {
			fmt.Fprintf(stderr, "jag dump: --out dir %q is not empty\n", *outDir)
			return 1
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			fmt.Fprintf(stderr, "jag dump: mkdir: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprintf(stderr, "jag dump: stat --out: %v\n", err)
		return 1
	}

	jf, err := jagfile.LoadJagfile(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "jag dump: %v\n", err)
		return 1
	}
	for i := 0; i < jf.FileCount; i++ {
		name := jf.FileName[i]
		if name == "" {
			fmt.Fprintf(stderr, "jag dump: entry %d: hash 0x%08x not in known-names table, skipping\n", i, jf.FileHash[i])
			continue
		}
		if !safeBasename(name) {
			fmt.Fprintf(stderr, "jag dump: entry %d: name %q rejected (path traversal)\n", i, name)
			continue
		}
		pkt, err := jf.Get(i)
		if err != nil {
			fmt.Fprintf(stderr, "jag dump: %s: %v\n", name, err)
			return 1
		}
		if err := os.WriteFile(filepath.Join(*outDir, name), pkt.Data, 0o644); err != nil {
			fmt.Fprintf(stderr, "jag dump: write %s: %v\n", name, err)
			return 1
		}
	}
	return 0
}

// safeBasename reports whether name is safe to use as the leaf of
// filepath.Join(outDir, name) — i.e. it can't escape outDir via "..",
// path separators, or the special "." entry. Defends against untrusted
// jagfile sources; today's knownNames table contains only safe names,
// but a future loader pulling names from arbitrary archives needs this.
func safeBasename(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}
