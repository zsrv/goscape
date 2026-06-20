// Command goscape-cli is the operational-tooling sibling of cmd/goscape.
// The daemon binary runs long-lived services; this binary runs one-shot
// utilities like `pack`, `compile`, and `jag`.
//
// Layout mirrors grafana/tempo's cmd/tempo-cli: subcommand-dispatched,
// one verb per file. Verbs register themselves in the `verbs` slice
// below; dispatch and usage both consume that slice.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

// verbHandler is the uniform signature every top-level verb implements.
// Verbs that don't need stdout simply ignore it.
type verbHandler func(args []string, stdout, stderr io.Writer) int

type verb struct {
	name    string
	handler verbHandler
	summary string
}

// verbs is the source of truth for top-level commands. To add a verb,
// implement a runXxx in a sibling file and append one entry here.
var verbs = []verb{
	{"pack", runPack, "Build server-side packs (configs + compiled scripts)."},
	{"compile", runCompile, "Run the runescript compiler on a single .rs2 source file."},
	{"jag", runJag, "Inspect a .jag archive (list | extract | dump)."},
	{"smoke-pack", runSmokePack, "Run all PackAll stages best-effort against a content dir and report per-stage outcomes."},
	{"worldmap", runWorldmap, "Build mapview/worldmap.jag from packed map output and Content assets."},
	{"rsa", runRSA, "Generate or inspect RSA login keys (gen | info)."},
}

// dispatch routes args[0] to a verb handler. stdout receives help
// output; stderr receives errors and usage-on-failure.
//
// Exit codes:
//
//	0 — success (or help-flag print)
//	1 — verb returned a runtime error
//	2 — no verb, unknown verb, or verb flag-parse error
func dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}

	name, rest := args[0], args[1:]
	switch name {
	case "-h", "--help", "help":
		usage(stdout)
		return 0
	}
	for _, v := range verbs {
		if v.name == name {
			return v.handler(rest, stdout, stderr)
		}
	}
	fmt.Fprintf(stderr, "unknown verb: %q\n\n", name)
	usage(stderr)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: goscape-cli <verb> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Verbs:")
	for _, v := range verbs {
		fmt.Fprintf(w, "  %-10s %s\n", v.name, v.summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run `goscape-cli <verb> -h` for verb-specific flags.")
}
