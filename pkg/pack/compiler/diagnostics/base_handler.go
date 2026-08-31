// pkg/pack/compiler/diagnostics/base_handler.go
package diagnostics

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// BaseDiagnosticsHandler is the default user-facing Handler implementation.
// Mirrors TS BaseDiagnosticsHandler (RuneScriptTS
// src/compiler/diagnostics/DiagnosticsHandler.ts L47-147): prints each
// diagnostic as "<path>:<line>:<col>: <TYPE>: <message>", followed (in T2)
// by the source line and a caret pointer.
//
// NAI-211-D-NO-PROCESS-EXIT: TS L143-145 calls process.exit(1) when there
// are errors; goscape returns errors up through ServerScriptCompiler.Run
// instead, so this handler is print-only. See spec §"Error Handling".
//
// NAI-211-D-MACRO-LOOKUP-DEFERRED: TS L52-56 BaseDiagnosticsHandler holds
// an optional macroLookup field for resolving macro origins in
// diagnostics output. Macros aren't ported yet (see parsePhase deferral
// in server_script_compiler.go); this handler does not yet honor macro
// origins. Re-introduce when macros land.
type BaseDiagnosticsHandler struct {
	// Out is the destination writer. When nil, defaults to os.Stdout
	// at handler-call time (mirrors TS console.log which goes to stdout).
	Out io.Writer
}

// HandleParse dispatches a parse-phase Diagnostics through handleShared.
func (h *BaseDiagnosticsHandler) HandleParse(d *Diagnostics) { h.handleShared(d) }

// HandleTypeChecking dispatches an analyze-phase Diagnostics.
func (h *BaseDiagnosticsHandler) HandleTypeChecking(d *Diagnostics) { h.handleShared(d) }

// HandleCodeGeneration dispatches a codegen-phase Diagnostics.
func (h *BaseDiagnosticsHandler) HandleCodeGeneration(d *Diagnostics) { h.handleShared(d) }

// HandlePointerChecking dispatches a pointer-checking-phase Diagnostics.
func (h *BaseDiagnosticsHandler) HandlePointerChecking(d *Diagnostics) { h.handleShared(d) }

// handleShared prints every diagnostic in d to h.Out (or os.Stdout when
// h.Out is nil). Mirrors TS handleShared L74-146. Edge-case handling
// (file-missing, line out-of-bounds) lands in T3.
func (h *BaseDiagnosticsHandler) handleShared(d *Diagnostics) {
	out := h.Out
	if out == nil {
		out = os.Stdout
	}
	// Per-call file-line cache (mirrors TS L75 `const fileLines = new Map()`).
	fileLines := map[string][]string{}

	for _, diag := range d.List() {
		loc := diag.SourceLocation
		msg := fmt.Sprintf(diag.Message, diag.MessageArgs...)
		fmt.Fprintf(out, "%s:%d:%d: %s: %s\n", loc.Name, loc.Line, loc.Column, diag.Type, msg)

		// Lazy-load source lines.
		absPath, err := filepath.Abs(loc.Name)
		if err != nil {
			continue
		}
		lines, ok := fileLines[absPath]
		if !ok {
			b, rerr := os.ReadFile(absPath)
			if rerr != nil {
				fileLines[absPath] = nil // negative-cache; T3 covers display
				continue
			}
			lines = strings.Split(string(b), "\n")
			// Mirrors TS split(/\r?\n/) — also drop trailing \r from each line.
			for i, ln := range lines {
				lines[i] = strings.TrimSuffix(ln, "\r")
			}
			fileLines[absPath] = lines
		}
		if lines == nil {
			continue
		}

		lineIdx := loc.Line - 1
		if lineIdx < 0 || lineIdx >= len(lines) {
			continue
		}
		line := lines[lineIdx]
		lineNoTabs := strings.ReplaceAll(line, "\t", "    ")
		tabCount := strings.Count(line, "\t")
		col := max(loc.Column, 1)
		caretOffset := max(tabCount*3+(col-1), 0)
		fmt.Fprintf(out, "    > %s\n", lineNoTabs)
		fmt.Fprintf(out, "    > %s^\n", strings.Repeat(" ", caretOffset))
	}
}
