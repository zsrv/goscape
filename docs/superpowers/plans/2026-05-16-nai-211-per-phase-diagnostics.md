# NAI-211 Per-Phase Diagnostics + BaseDiagnosticsHandler Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the `diagnostics.Handler` interface dispatch into `ServerScriptCompiler.Run` with per-phase fresh `*Diagnostics`, land `BaseDiagnosticsHandler` as the default user-facing printer, and add a `ParserErrorListener` adapter so parser syntax errors flow through the diagnostic stream.

**Architecture:** Rename `ServerScriptCompiler.DiagHandler *diagnostics.Diagnostics` → `Handler diagnostics.Handler`. Each phase method (`parsePhase` / `analyzePhase` / `codegenPhase` / `checkPointersPhase`) allocates a local `d := &diagnostics.Diagnostics{}`, threads it into the phase constructor (no signature change there), and dispatches `c.Handler.HandleXxx(d)` at phase end. New `BaseDiagnosticsHandler` (in `diagnostics/base_handler.go`) prints `location: TYPE: message` plus source line + caret to a configurable `io.Writer` (default stdout). New `ParserErrorListener` (in `diagnostics/parser_error_listener.go`) bridges `lexer.ErrorListener` → `*Diagnostics`.

**Tech Stack:** Go 1.26+, `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`, `git commit --no-gpg-sign`. TS pin: `LostCityRS/RuneScriptTS @ b8c338801fbb72d294ff9576a58925a8d3f6de47`.

**Authoritative task numbering:** T1, T2, T3, T4, T5, T6, T7, T8. Per `[[plan_code_block_t_number_drift]]`, all in-file doc comments and commit subjects must use this numbering.

**Spec:** `docs/superpowers/specs/2026-05-16-nai-211-per-phase-diagnostics-design.md` (commit `13ae5fb`).

**Predecessor:** NAI-210 close (`19dc451`) + overlay-interfaces follow-up (`767a99e`).

---

## File Structure

### Created

- `pkg/pack/compiler/diagnostics/base_handler.go` — `BaseDiagnosticsHandler` struct + four `HandleXxx` methods + shared `handleShared` formatter (T1, T2, T3)
- `pkg/pack/compiler/diagnostics/base_handler_test.go` — 7 tests covering format / source / caret / edge cases / all-four-delegate / no-os-exit (T1, T2, T3)
- `pkg/pack/compiler/diagnostics/parser_error_listener.go` — `ParserErrorListener` struct + `SyntaxError(...)` adapter (T4)
- `pkg/pack/compiler/diagnostics/parser_error_listener_test.go` — 2 tests (T4)
- `pkg/pack/compiler/runescript/nai211_deviation_pins_test.go` — 3 pin tests for the three NAI-211 tags (T8)

### Modified

- `pkg/pack/compiler/runescript/server_script_compiler.go` — rename `DiagHandler` → `Handler`; per-phase fresh `*Diagnostics`; handler dispatch; parser-error-listener wiring in `parsePhase` (T5, T6)
- `pkg/pack/compiler/runescript/server_script_compiler_test.go` — extend with 5 new tests (T5, T6)
- `pkg/pack/compiler/runescript/compile.go` — add `Config.Handler` field + default to `&diagnostics.BaseDiagnosticsHandler{}` (T7)
- `pkg/pack/compiler/runescript/compile_test.go` — extend with one Handler-injection test (T7)

### Deviation tags (set in this slice)

| Tag | Origin |
|---|---|
| `NAI-211-D-NO-PROCESS-EXIT` | T2 — `BaseDiagnosticsHandler.handleShared` does NOT call `os.Exit`; control flow stays in `Run() error` |
| `NAI-211-D-MACRO-LOOKUP-DEFERRED` | T1 — TS `BaseDiagnosticsHandler.macroLookup` + `MacroLookup*` types omitted (macros aren't ported yet) |
| `NAI-211-D-PHASE-DIAGNOSTICS-FRESH` | T5 — TS-faithful per-phase `new Diagnostics()`; pre-NAI-211 shared accumulator retired |

No tags retired in this slice. Two NAI-210 follow-up tags (richer driver smoke, per-phase Diagnostics) — this plan completes the second; the first is unblocked but lives in its own follow-up.

---

## Task T1: `BaseDiagnosticsHandler` skeleton + location/type/message format

**Files:**
- Create: `pkg/pack/compiler/diagnostics/base_handler.go`
- Create: `pkg/pack/compiler/diagnostics/base_handler_test.go`

**TS source:** `RuneScriptTS/src/compiler/diagnostics/DiagnosticsHandler.ts` L47-147.

**Scope:** Just the header line — `"<path>:<line>:<col>: <TYPE>: <formatted-message>\n"`. Source-line + caret rendering is T2; edge cases are T3.

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/diagnostics/base_handler_test.go`:

```go
// pkg/pack/compiler/diagnostics/base_handler_test.go
package diagnostics

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

// writeFile is a test helper used across base_handler_test.go to seed
// fixture files into t.TempDir() before invoking the handler.
func writeFile(t *testing.T, path, content string) error {
	t.Helper()
	return os.WriteFile(path, []byte(content), 0o644)
}

// TestBaseDiagnosticsHandler_FormatsLocationTypeMessage pins the TS L98
// "<path>:<line>:<column>: <type>: <message>" header line. Mirrors TS
// BaseDiagnosticsHandler.handleShared L100.
func TestBaseDiagnosticsHandler_FormatsLocationTypeMessage(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "f.rs2")
	// File contents don't matter for this test; we only assert the header.
	if err := writeFile(t, tmp, "irrelevant\n"); err != nil {
		t.Fatalf("write tmp: %v", err)
	}

	var buf bytes.Buffer
	h := &BaseDiagnosticsHandler{Out: &buf}
	d := &Diagnostics{}
	d.Report(Diagnostic{
		Type:           DiagnosticError,
		SourceLocation: lexer.NodeSourceLocation{Name: tmp, Line: 1, Column: 1},
		Message:        "boom",
	})

	h.HandleParse(d)

	got := buf.String()
	want := tmp + ":1:1: ERROR: boom"
	if !strings.Contains(got, want) {
		t.Errorf("output missing header %q; got:\n%s", want, got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/diagnostics/ -run TestBaseDiagnosticsHandler_FormatsLocationTypeMessage -v`
Expected: FAIL with `undefined: BaseDiagnosticsHandler`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/diagnostics/base_handler.go`:

```go
// pkg/pack/compiler/diagnostics/base_handler.go
package diagnostics

import (
	"fmt"
	"io"
	"os"
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
// h.Out is nil). Mirrors TS handleShared L74-146. Source-line + caret
// rendering land in T2; edge-case handling lands in T3.
func (h *BaseDiagnosticsHandler) handleShared(d *Diagnostics) {
	out := h.Out
	if out == nil {
		out = os.Stdout
	}
	for _, diag := range d.List() {
		loc := diag.SourceLocation
		msg := fmt.Sprintf(diag.Message, diag.MessageArgs...)
		fmt.Fprintf(out, "%s:%d:%d: %s: %s\n", loc.Name, loc.Line, loc.Column, diag.Type, msg)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/diagnostics/ -run TestBaseDiagnosticsHandler_FormatsLocationTypeMessage -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/diagnostics/base_handler.go \
        pkg/pack/compiler/diagnostics/base_handler_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/diagnostics): NAI-211 T1 — BaseDiagnosticsHandler skeleton + header line

Adds BaseDiagnosticsHandler with the four HandleXxx phase-dispatch methods
delegating to handleShared, which prints the TS-faithful
"<path>:<line>:<col>: <TYPE>: <message>" header line. Source-line + caret
rendering land in T2; edge cases in T3.

Tags NAI-211-D-NO-PROCESS-EXIT + NAI-211-D-MACRO-LOOKUP-DEFERRED placed
on the struct doc-comment for the pin test in T8.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T2: `BaseDiagnosticsHandler` — source line + caret rendering

**Files:**
- Modify: `pkg/pack/compiler/diagnostics/base_handler.go` (extend `handleShared`)
- Modify: `pkg/pack/compiler/diagnostics/base_handler_test.go` (add test)

**TS source:** `RuneScriptTS/src/compiler/diagnostics/DiagnosticsHandler.ts` L74-117.

**Scope:** Lazy-load file lines, tab→4-space expansion on the printed source line, caret offset `tabCount*3 + (column-1)`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/pack/compiler/diagnostics/base_handler_test.go`:

```go
// TestBaseDiagnosticsHandler_RendersSourceLineAndCaret pins the source-line
// readout (with tabs expanded to 4 spaces) and the caret-pointer offset.
// Mirrors TS handleShared L102-116. The test file uses a literal tab on
// line 2 col 2 (1-based: '\t' is col 1, 'h' is col 2) — so caret offset =
// tabCount*3 + (col-1) = 3 + 1 = 4 spaces.
func TestBaseDiagnosticsHandler_RendersSourceLineAndCaret(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "f.rs2")
	if err := writeFile(t, tmp, "line1\n\thello\nline3\n"); err != nil {
		t.Fatalf("write tmp: %v", err)
	}

	var buf bytes.Buffer
	h := &BaseDiagnosticsHandler{Out: &buf}
	d := &Diagnostics{}
	d.Report(Diagnostic{
		Type:           DiagnosticError,
		SourceLocation: lexer.NodeSourceLocation{Name: tmp, Line: 2, Column: 2},
		Message:        "bad",
	})

	h.HandleParse(d)

	got := buf.String()
	wantSrc := "    >     hello" // 4-space prefix + tab-expanded "    hello"
	wantCaret := "    > " + strings.Repeat(" ", 4) + "^"
	if !strings.Contains(got, wantSrc) {
		t.Errorf("output missing source line %q; got:\n%s", wantSrc, got)
	}
	if !strings.Contains(got, wantCaret) {
		t.Errorf("output missing caret line %q; got:\n%s", wantCaret, got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/diagnostics/ -run TestBaseDiagnosticsHandler_RendersSourceLineAndCaret -v`
Expected: FAIL — no source/caret line emitted yet.

- [ ] **Step 3: Extend `handleShared`**

Replace `pkg/pack/compiler/diagnostics/base_handler.go` `handleShared` with:

```go
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
		col := loc.Column
		if col < 1 {
			col = 1
		}
		caretOffset := tabCount*3 + (col - 1)
		if caretOffset < 0 {
			caretOffset = 0
		}
		fmt.Fprintf(out, "    > %s\n", lineNoTabs)
		fmt.Fprintf(out, "    > %s^\n", strings.Repeat(" ", caretOffset))
	}
}
```

Add `"path/filepath"` and `"strings"` to the imports.

- [ ] **Step 4: Run all base_handler tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/diagnostics/ -run TestBaseDiagnosticsHandler -v`
Expected: PASS for both `_FormatsLocationTypeMessage` and `_RendersSourceLineAndCaret`.

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/diagnostics/base_handler.go \
        pkg/pack/compiler/diagnostics/base_handler_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/diagnostics): NAI-211 T2 — BaseDiagnosticsHandler source line + caret

Extends handleShared to lazy-load file lines, expand tabs to 4 spaces in
the printed source readout, and emit a caret pointer at offset
tabCount*3 + (col-1). Mirrors TS handleShared L102-116.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T3: `BaseDiagnosticsHandler` — edge cases + remaining pins

**Files:**
- Modify: `pkg/pack/compiler/diagnostics/base_handler_test.go` (add 5 tests)

**Scope:** No production-code change needed (handleShared already handles edge cases via `continue` paths). Tests only.

- [ ] **Step 1: Write the failing tests**

Append five tests to `pkg/pack/compiler/diagnostics/base_handler_test.go`:

```go
// TestBaseDiagnosticsHandler_LineOutOfBoundsSkipsSource asserts no `>` line
// is emitted when the diagnostic line exceeds the file length. The header
// still prints.
func TestBaseDiagnosticsHandler_LineOutOfBoundsSkipsSource(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "f.rs2")
	if err := writeFile(t, tmp, "a\nb\nc\n"); err != nil {
		t.Fatalf("write tmp: %v", err)
	}

	var buf bytes.Buffer
	h := &BaseDiagnosticsHandler{Out: &buf}
	d := &Diagnostics{}
	d.Report(Diagnostic{
		Type:           DiagnosticError,
		SourceLocation: lexer.NodeSourceLocation{Name: tmp, Line: 99, Column: 1},
		Message:        "oob",
	})

	h.HandleParse(d)

	got := buf.String()
	if !strings.Contains(got, "ERROR: oob") {
		t.Errorf("header missing; got:\n%s", got)
	}
	if strings.Contains(got, "    >") {
		t.Errorf("source/caret line should be skipped; got:\n%s", got)
	}
}

// TestBaseDiagnosticsHandler_FileMissingSkipsSource asserts no panic + no
// `>` line when the path does not exist on disk. The header still prints.
func TestBaseDiagnosticsHandler_FileMissingSkipsSource(t *testing.T) {
	var buf bytes.Buffer
	h := &BaseDiagnosticsHandler{Out: &buf}
	d := &Diagnostics{}
	d.Report(Diagnostic{
		Type:           DiagnosticError,
		SourceLocation: lexer.NodeSourceLocation{Name: "/nonexistent/path.rs2", Line: 1, Column: 1},
		Message:        "ghost",
	})

	h.HandleParse(d) // must not panic
	got := buf.String()
	if !strings.Contains(got, "ERROR: ghost") {
		t.Errorf("header missing; got:\n%s", got)
	}
	if strings.Contains(got, "    >") {
		t.Errorf("source/caret line should be skipped; got:\n%s", got)
	}
}

// TestBaseDiagnosticsHandler_MessageArgsFormatted pins that printf-style
// %s verbs in Message are substituted from MessageArgs (mirrors TS L100
// `util.format(message, ...messageArgs)`).
func TestBaseDiagnosticsHandler_MessageArgsFormatted(t *testing.T) {
	var buf bytes.Buffer
	h := &BaseDiagnosticsHandler{Out: &buf}
	d := &Diagnostics{}
	d.Report(Diagnostic{
		Type:           DiagnosticError,
		SourceLocation: lexer.NodeSourceLocation{Name: "x", Line: 1, Column: 1},
		Message:        "bad %s",
		MessageArgs:    []any{"foo"},
	})

	h.HandleParse(d)

	if !strings.Contains(buf.String(), "ERROR: bad foo") {
		t.Errorf("MessageArgs not formatted; got:\n%s", buf.String())
	}
}

// TestBaseDiagnosticsHandler_AllFourPhaseMethodsDispatchSame pins TS
// L58-72: each of the four HandleXxx methods delegates to handleShared
// with identical output.
func TestBaseDiagnosticsHandler_AllFourPhaseMethodsDispatchSame(t *testing.T) {
	mkDiag := func() *Diagnostics {
		d := &Diagnostics{}
		d.Report(Diagnostic{
			Type:           DiagnosticError,
			SourceLocation: lexer.NodeSourceLocation{Name: "x", Line: 1, Column: 1},
			Message:        "shared",
		})
		return d
	}

	captures := []string{}
	for _, call := range []func(h *BaseDiagnosticsHandler, d *Diagnostics){
		func(h *BaseDiagnosticsHandler, d *Diagnostics) { h.HandleParse(d) },
		func(h *BaseDiagnosticsHandler, d *Diagnostics) { h.HandleTypeChecking(d) },
		func(h *BaseDiagnosticsHandler, d *Diagnostics) { h.HandleCodeGeneration(d) },
		func(h *BaseDiagnosticsHandler, d *Diagnostics) { h.HandlePointerChecking(d) },
	} {
		var buf bytes.Buffer
		call(&BaseDiagnosticsHandler{Out: &buf}, mkDiag())
		captures = append(captures, buf.String())
	}

	for i := 1; i < len(captures); i++ {
		if captures[i] != captures[0] {
			t.Errorf("phase method %d output diverges:\nfirst:\n%s\nthis:\n%s", i, captures[0], captures[i])
		}
	}
}

// TestBaseDiagnosticsHandler_NoOsExit pins NAI-211-D-NO-PROCESS-EXIT:
// even when diagnostics contain ERRORs, handleShared returns normally
// (no os.Exit). Test-process survival is the assertion.
func TestBaseDiagnosticsHandler_NoOsExit(t *testing.T) {
	var buf bytes.Buffer
	h := &BaseDiagnosticsHandler{Out: &buf}
	d := &Diagnostics{}
	d.Report(Diagnostic{
		Type:           DiagnosticError,
		SourceLocation: lexer.NodeSourceLocation{Name: "x", Line: 1, Column: 1},
		Message:        "fatal-but-not-really",
	})
	if !d.HasErrors() {
		t.Fatal("test setup: expected HasErrors()==true")
	}
	h.HandleParse(d) // would call os.Exit(1) if NAI-211-D-NO-PROCESS-EXIT regressed
	// If we got here, the deviation is honored.
}
```

- [ ] **Step 2: Run all base_handler tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/diagnostics/ -run TestBaseDiagnosticsHandler -v`
Expected: 7 tests PASS.

- [ ] **Step 3: Commit**

```bash
git add pkg/pack/compiler/diagnostics/base_handler_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(compiler/diagnostics): NAI-211 T3 — BaseDiagnosticsHandler edge cases + no-exit pin

Adds five tests: line-out-of-bounds (skip source/caret), file-missing
(skip source/caret, no panic), MessageArgs formatting, all-four-phase-
methods-dispatch-same delegation, and the NAI-211-D-NO-PROCESS-EXIT
behavioral pin (handleShared returns normally on HasErrors()==true).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T4: `ParserErrorListener` adapter

**Files:**
- Create: `pkg/pack/compiler/diagnostics/parser_error_listener.go`
- Create: `pkg/pack/compiler/diagnostics/parser_error_listener_test.go`

**TS source:** `RuneScriptTS/src/compiler/parser/ParserErrorListener.ts`.

**Scope:** Adapter that implements `lexer.ErrorListener` and pushes one `DiagnosticSyntaxError` into a `*Diagnostics` per `SyntaxError` callback. Constructor's `sourceName` arg overrides whatever path the callback passes (TS-faithful).

- [ ] **Step 1: Write the failing tests**

Create `pkg/pack/compiler/diagnostics/parser_error_listener_test.go`:

```go
// pkg/pack/compiler/diagnostics/parser_error_listener_test.go
package diagnostics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

// TestParserErrorListener_SyntaxErrorPushesDiagnostic pins that one
// SyntaxError callback produces one SyntaxError-typed Diagnostic with the
// constructor's sourceName and the callback's line/column/msg captured.
// Mirrors TS ParserErrorListener.syntaxError.
func TestParserErrorListener_SyntaxErrorPushesDiagnostic(t *testing.T) {
	d := &Diagnostics{}
	p := NewParserErrorListener("foo.rs2", d)

	p.SyntaxError("foo.rs2", 4, 7, "expected token")

	got := d.List()
	if len(got) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(got))
	}
	entry := got[0]
	if entry.Type != DiagnosticSyntaxError {
		t.Errorf("Type: got %v, want DiagnosticSyntaxError", entry.Type)
	}
	want := lexer.NodeSourceLocation{Name: "foo.rs2", Line: 4, Column: 7}
	if entry.SourceLocation != want {
		t.Errorf("SourceLocation: got %+v, want %+v", entry.SourceLocation, want)
	}
	if entry.Message != "%s" {
		t.Errorf("Message: got %q, want %q", entry.Message, "%s")
	}
	if len(entry.MessageArgs) != 1 || entry.MessageArgs[0] != "expected token" {
		t.Errorf("MessageArgs: got %v, want [\"expected token\"]", entry.MessageArgs)
	}
}

// TestParserErrorListener_SourceNameOverridesCallback pins that the
// constructor's sourceName wins over the callback's sourceName arg.
// Mirrors TS ParserErrorListener which captures the file at construction.
func TestParserErrorListener_SourceNameOverridesCallback(t *testing.T) {
	d := &Diagnostics{}
	p := NewParserErrorListener("ctor.rs2", d)

	p.SyntaxError("cb.rs2", 1, 1, "msg")

	got := d.List()
	if len(got) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(got))
	}
	if got[0].SourceLocation.Name != "ctor.rs2" {
		t.Errorf("SourceLocation.Name: got %q, want %q", got[0].SourceLocation.Name, "ctor.rs2")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/diagnostics/ -run TestParserErrorListener -v`
Expected: FAIL with `undefined: NewParserErrorListener`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/diagnostics/parser_error_listener.go`:

```go
// pkg/pack/compiler/diagnostics/parser_error_listener.go
package diagnostics

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

// ParserErrorListener adapts lexer.ErrorListener to *Diagnostics. One
// SyntaxError callback produces one Diagnostic of type
// DiagnosticSyntaxError. The constructor's sourceName overrides whatever
// path the callback passes — mirrors TS ParserErrorListener
// (RuneScriptTS/src/compiler/parser/ParserErrorListener.ts) which
// captures the file at construction time.
//
// Implements lexer.ErrorListener structurally.
type ParserErrorListener struct {
	SourceName string
	Diag       *Diagnostics
}

// NewParserErrorListener constructs an adapter bound to sourceName + d.
func NewParserErrorListener(sourceName string, d *Diagnostics) *ParserErrorListener {
	return &ParserErrorListener{SourceName: sourceName, Diag: d}
}

// SyntaxError pushes one DiagnosticSyntaxError into Diag. The callback's
// sourceName arg is ignored — Diag uses the constructor's SourceName.
func (p *ParserErrorListener) SyntaxError(_ string, line, column int, msg string) {
	p.Diag.Report(Diagnostic{
		Type:           DiagnosticSyntaxError,
		SourceLocation: lexer.NodeSourceLocation{Name: p.SourceName, Line: line, Column: column},
		Message:        "%s",
		MessageArgs:    []any{msg},
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/diagnostics/ -run TestParserErrorListener -v`
Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/diagnostics/parser_error_listener.go \
        pkg/pack/compiler/diagnostics/parser_error_listener_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/diagnostics): NAI-211 T4 — ParserErrorListener adapter

Adds ParserErrorListener bridging lexer.ErrorListener → *Diagnostics. Each
SyntaxError callback pushes one DiagnosticSyntaxError; constructor's
sourceName overrides the callback's path arg (TS-faithful). Implements
lexer.ErrorListener structurally; wired into parsePhase in T6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T5: `ServerScriptCompiler` field reshape + per-phase fresh `Diagnostics` + handler dispatch

**Files:**
- Modify: `pkg/pack/compiler/runescript/server_script_compiler.go`
- Modify: `pkg/pack/compiler/runescript/server_script_compiler_test.go`

**Scope:** Rename `DiagHandler *diagnostics.Diagnostics` → `Handler diagnostics.Handler` (interface). Each phase method allocates `d := &diagnostics.Diagnostics{}` and dispatches `c.Handler.HandleXxx(d)` at phase end. `Run()` nil-defaults `c.Handler` to `diagnostics.NopHandler{}`. NO parser-error-listener wiring in this task (T6).

**Risk:** rename touches several call sites inside the file. Per `[[plan_doc_replaceall_timeline]]`, edit per-occurrence — do NOT `replace_all`.

- [ ] **Step 1: Write the failing tests**

Append to `pkg/pack/compiler/runescript/server_script_compiler_test.go` (before the closing imports, add new imports as needed):

```go
// recordingHandler is a test-only diagnostics.Handler that records the
// sequence of HandleXxx method names called and the per-call diagnostic
// count.
type recordingHandler struct {
	calls    []string
	counts   []int
	capDiags map[string]*diagnostics.Diagnostics
}

func newRecordingHandler() *recordingHandler {
	return &recordingHandler{capDiags: map[string]*diagnostics.Diagnostics{}}
}

func (r *recordingHandler) record(name string, d *diagnostics.Diagnostics) {
	r.calls = append(r.calls, name)
	r.counts = append(r.counts, len(d.List()))
	r.capDiags[name] = d
}

func (r *recordingHandler) HandleParse(d *diagnostics.Diagnostics) {
	r.record("HandleParse", d)
}
func (r *recordingHandler) HandleTypeChecking(d *diagnostics.Diagnostics) {
	r.record("HandleTypeChecking", d)
}
func (r *recordingHandler) HandleCodeGeneration(d *diagnostics.Diagnostics) {
	r.record("HandleCodeGeneration", d)
}
func (r *recordingHandler) HandlePointerChecking(d *diagnostics.Diagnostics) {
	r.record("HandlePointerChecking", d)
}

// TestRun_HandlerDispatchedInOrder pins the per-phase dispatch sequence
// when CommandPointers is empty (TS-faithful early-return per
// NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE means HandlePointerChecking is
// NOT called in that path).
func TestRun_HandlerDispatchedInOrder(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	rh := newRecordingHandler()
	c := &ServerScriptCompiler{
		SourcePaths:     []string{},
		TypeManager:     typ.NewTypeManager(),
		Mapper:          mapper,
		CommandPointers: map[string]*pointer.PointerHolder{}, // EMPTY → early halt
		Writer:          &noopBinaryOutput{},
		Handler:         rh,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	_ = c.Run("rs2")

	want := []string{"HandleParse", "HandleTypeChecking", "HandleCodeGeneration"}
	if !equalStrings(rh.calls, want) {
		t.Errorf("dispatch order: got %v, want %v", rh.calls, want)
	}
}

// TestRun_HandlerDispatchedForPointerChecking pins that
// HandlePointerChecking IS called when CommandPointers is non-empty.
func TestRun_HandlerDispatchedForPointerChecking(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	rh := newRecordingHandler()
	c := &ServerScriptCompiler{
		SourcePaths: []string{},
		TypeManager: typ.NewTypeManager(),
		Mapper:      mapper,
		// Non-empty CommandPointers → HandlePointerChecking called.
		CommandPointers: map[string]*pointer.PointerHolder{"foo": {Required: pointer.NewPointerSet()}},
		Writer:          &noopBinaryOutput{},
		Handler:         rh,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	_ = c.Run("rs2")

	want := []string{"HandleParse", "HandleTypeChecking", "HandleCodeGeneration", "HandlePointerChecking"}
	if !equalStrings(rh.calls, want) {
		t.Errorf("dispatch order: got %v, want %v", rh.calls, want)
	}
}

// TestRun_PerPhaseDiagnosticsAreFresh pins NAI-211-D-PHASE-DIAGNOSTICS-FRESH:
// each phase gets its OWN *Diagnostics; a diagnostic reported in one
// phase does not leak into the next phase's accumulator.
func TestRun_PerPhaseDiagnosticsAreFresh(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	rh := newRecordingHandler()
	c := &ServerScriptCompiler{
		SourcePaths:     []string{},
		TypeManager:     typ.NewTypeManager(),
		Mapper:          mapper,
		CommandPointers: map[string]*pointer.PointerHolder{"foo": {Required: pointer.NewPointerSet()}},
		Writer:          &noopBinaryOutput{},
		Handler:         rh,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	_ = c.Run("rs2")

	// With empty SourcePaths there are no diagnostics anywhere; the
	// structural pin is that each call captured a DISTINCT *Diagnostics
	// pointer.
	seen := map[*diagnostics.Diagnostics]string{}
	for name, d := range rh.capDiags {
		if prev, dup := seen[d]; dup {
			t.Errorf("per-phase isolation: %s and %s share the same *Diagnostics pointer", prev, name)
		}
		seen[d] = name
	}
}

// TestRun_NilHandlerDefaultsToNop pins that Run() initializes a nil Handler
// to NopHandler{} and runs to completion without panic.
func TestRun_NilHandlerDefaultsToNop(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	c := &ServerScriptCompiler{
		SourcePaths:     []string{},
		TypeManager:     typ.NewTypeManager(),
		Mapper:          mapper,
		CommandPointers: map[string]*pointer.PointerHolder{"foo": {Required: pointer.NewPointerSet()}},
		Writer:          &noopBinaryOutput{},
		Handler:         nil,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	if err := c.Run("rs2"); err != nil {
		t.Errorf("Run with nil Handler: got %v, want nil", err)
	}
	if _, ok := c.Handler.(diagnostics.NopHandler); !ok {
		t.Errorf("nil Handler should default to NopHandler{}, got %T", c.Handler)
	}
}

// equalStrings is a test helper for slice equality.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

Add `"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"` to the imports if not present.

- [ ] **Step 2: Run new tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestRun_HandlerDispatchedInOrder -v`
Expected: FAIL — `unknown field Handler in struct literal of type ServerScriptCompiler` (field rename hasn't landed).

- [ ] **Step 3: Edit `server_script_compiler.go` — rename field**

Edit `pkg/pack/compiler/runescript/server_script_compiler.go`. Replace the struct field block:

Old:
```go
	CompilerSymbols map[string]*CompilerTypeInfo
	Mapper          *SymbolMapper
	CommandPointers map[string]*pointer.PointerHolder
	Features        semantics.StrictFeatureLevel

	DiagHandler *diagnostics.Diagnostics

	BinaryWriter *BinaryScriptWriter
	Writer       BinaryOutput
```

New:
```go
	CompilerSymbols map[string]*CompilerTypeInfo
	Mapper          *SymbolMapper
	CommandPointers map[string]*pointer.PointerHolder
	Features        semantics.StrictFeatureLevel

	// NAI-211-D-PHASE-DIAGNOSTICS-FRESH: each phase allocates its own
	// *diagnostics.Diagnostics; the pre-NAI-211 shared accumulator
	// (`DiagHandler *diagnostics.Diagnostics`) is retired. This field
	// holds the user-pluggable Handler that receives the per-phase
	// Diagnostics via HandleParse / HandleTypeChecking /
	// HandleCodeGeneration / HandlePointerChecking. Nil defaults to
	// NopHandler{} in Run().
	Handler diagnostics.Handler

	BinaryWriter *BinaryScriptWriter
	Writer       BinaryOutput
```

- [ ] **Step 4: Edit `Run()` — nil-default + per-phase fresh Diagnostics**

Replace the body of `Run` and each phase method. Replace lines around the existing `Run`:

Old `Run`:
```go
func (c *ServerScriptCompiler) Run(ext string) error {
	if c.DiagHandler == nil {
		c.DiagHandler = &diagnostics.Diagnostics{}
	}

	if err := c.loadSymbols(); err != nil {
		return err
	}

	files, err := c.parsePhase(ext)
	if err != nil {
		return err
	}
	if c.DiagHandler.HasErrors() {
		return fmt.Errorf("parse: diagnostics reported errors")
	}

	if err := c.analyzePhase(files); err != nil {
		return err
	}

	scripts, err := c.codegenPhase(files)
	if err != nil {
		return err
	}

	if c.checkPointersPhase(scripts) {
		return nil // NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE early-return
	}

	c.writePhase(scripts)

	if closer, ok := c.Writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
```

New `Run`:
```go
func (c *ServerScriptCompiler) Run(ext string) error {
	if c.Handler == nil {
		c.Handler = diagnostics.NopHandler{}
	}

	if err := c.loadSymbols(); err != nil {
		return err
	}

	files, parseDiag, err := c.parsePhase(ext)
	if err != nil {
		return err
	}
	c.Handler.HandleParse(parseDiag)
	if parseDiag.HasErrors() {
		return fmt.Errorf("parse: diagnostics reported errors")
	}

	if err := c.analyzePhase(files); err != nil {
		return err
	}

	scripts, err := c.codegenPhase(files)
	if err != nil {
		return err
	}

	if c.checkPointersPhase(scripts) {
		return nil // NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE early-return
	}

	c.writePhase(scripts)

	if closer, ok := c.Writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
```

- [ ] **Step 5: Edit `parsePhase` — return fresh Diagnostics**

Old:
```go
func (c *ServerScriptCompiler) parsePhase(ext string) ([]*ast.ScriptFile, error) {
	var files []*ast.ScriptFile
	for _, sourcePath := range c.SourcePaths {
		err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, "."+ext) {
				return nil
			}
			content, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			p := parser.NewScriptFileParser(string(content), path)
			node := p.ParseScriptFile()
			if node != nil {
				files = append(files, node)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	return files, nil
}
```

New (parser-error-listener wiring happens in T6; this task just returns the fresh `*Diagnostics`):
```go
func (c *ServerScriptCompiler) parsePhase(ext string) ([]*ast.ScriptFile, *diagnostics.Diagnostics, error) {
	d := &diagnostics.Diagnostics{}
	var files []*ast.ScriptFile
	for _, sourcePath := range c.SourcePaths {
		err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, "."+ext) {
				return nil
			}
			content, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			p := parser.NewScriptFileParser(string(content), path)
			node := p.ParseScriptFile()
			if node != nil {
				files = append(files, node)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, d, err
		}
	}
	return files, d, nil
}
```

- [ ] **Step 6: Edit `analyzePhase` — fresh Diagnostics + dispatch**

Old:
```go
func (c *ServerScriptCompiler) analyzePhase(files []*ast.ScriptFile) error {
	reg := semantics.NewScriptRegistration(c.TypeManager, c.Triggers, c.RootTable, c.DiagHandler, c.Features)
	for _, f := range files {
		reg.Visit(f)
	}

	c.registerSecondaryCommands()

	tc := semantics.NewTypeChecker(c.TypeManager, c.Triggers, c.RootTable, c.DynHandlers, c.DiagHandler, c.Features)
	for _, f := range files {
		tc.Visit(f)
	}

	if c.DiagHandler.HasErrors() {
		return fmt.Errorf("analyze: diagnostics reported errors")
	}
	return nil
}
```

New:
```go
func (c *ServerScriptCompiler) analyzePhase(files []*ast.ScriptFile) error {
	d := &diagnostics.Diagnostics{}
	reg := semantics.NewScriptRegistration(c.TypeManager, c.Triggers, c.RootTable, d, c.Features)
	for _, f := range files {
		reg.Visit(f)
	}

	c.registerSecondaryCommands()

	tc := semantics.NewTypeChecker(c.TypeManager, c.Triggers, c.RootTable, c.DynHandlers, d, c.Features)
	for _, f := range files {
		tc.Visit(f)
	}

	c.Handler.HandleTypeChecking(d)
	if d.HasErrors() {
		return fmt.Errorf("analyze: diagnostics reported errors")
	}
	return nil
}
```

- [ ] **Step 7: Edit `codegenPhase` — fresh Diagnostics + dispatch**

Old:
```go
func (c *ServerScriptCompiler) codegenPhase(files []*ast.ScriptFile) ([]*codegen.RuneScript, error) {
	var scripts []*codegen.RuneScript
	for _, f := range files {
		gen := codegen.NewCodeGenerator(c.RootTable, c.DynHandlers, c.DiagHandler)
		gen.Visit(f)
		scripts = append(scripts, gen.Scripts()...)
	}
	if c.DiagHandler.HasErrors() {
		return nil, fmt.Errorf("codegen: diagnostics reported errors")
	}
	return scripts, nil
}
```

New:
```go
func (c *ServerScriptCompiler) codegenPhase(files []*ast.ScriptFile) ([]*codegen.RuneScript, error) {
	d := &diagnostics.Diagnostics{}
	var scripts []*codegen.RuneScript
	for _, f := range files {
		gen := codegen.NewCodeGenerator(c.RootTable, c.DynHandlers, d)
		gen.Visit(f)
		scripts = append(scripts, gen.Scripts()...)
	}
	c.Handler.HandleCodeGeneration(d)
	if d.HasErrors() {
		return nil, fmt.Errorf("codegen: diagnostics reported errors")
	}
	return scripts, nil
}
```

- [ ] **Step 8: Edit `checkPointersPhase` — fresh Diagnostics + dispatch (only when non-empty)**

Old:
```go
func (c *ServerScriptCompiler) checkPointersPhase(scripts []*codegen.RuneScript) (halt bool) {
	if len(c.CommandPointers) < 1 {
		return true
	}
	checker := NewServerPointerChecker(c.DiagHandler, scripts, c.CommandPointers, c.Features, c.collectOverlayInterfaces())
	checker.Run()
	return c.DiagHandler.HasErrors()
}
```

New:
```go
func (c *ServerScriptCompiler) checkPointersPhase(scripts []*codegen.RuneScript) (halt bool) {
	// TS-faithful: NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE early-returns
	// BEFORE allocating Diagnostics or dispatching the handler. The
	// pointer-checking Handler is NOT called on this path.
	if len(c.CommandPointers) < 1 {
		return true
	}
	d := &diagnostics.Diagnostics{}
	checker := NewServerPointerChecker(d, scripts, c.CommandPointers, c.Features, c.collectOverlayInterfaces())
	checker.Run()
	c.Handler.HandlePointerChecking(d)
	return d.HasErrors()
}
```

- [ ] **Step 9: Run all runescript tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -v`
Expected: All existing tests + the four new ones (`TestRun_HandlerDispatchedInOrder`, `TestRun_HandlerDispatchedForPointerChecking`, `TestRun_PerPhaseDiagnosticsAreFresh`, `TestRun_NilHandlerDefaultsToNop`) PASS.

- [ ] **Step 10: Run full suite (regression check)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS. (No external consumers of `DiagHandler` per pre-flight grep; rename is safe.)

- [ ] **Step 11: Commit**

```bash
git add pkg/pack/compiler/runescript/server_script_compiler.go \
        pkg/pack/compiler/runescript/server_script_compiler_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-211 T5 — per-phase Diagnostics + Handler dispatch

Renames ServerScriptCompiler.DiagHandler (*diagnostics.Diagnostics) to
Handler (diagnostics.Handler). Each phase method allocates its own
*Diagnostics and dispatches via c.Handler.HandleXxx at phase end. Run()
nil-defaults to NopHandler{}. Pointer-check phase remains TS-faithful: no
Handler call when CommandPointers is empty (NAI-210-D-EMPTYPOINTERS-
RETURNS-FALSE).

Tag NAI-211-D-PHASE-DIAGNOSTICS-FRESH placed on the Handler field
doc-comment.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T6: Wire `ParserErrorListener` in `parsePhase`

**Files:**
- Modify: `pkg/pack/compiler/runescript/server_script_compiler.go` (parsePhase only)
- Modify: `pkg/pack/compiler/runescript/server_script_compiler_test.go` (add 1 test)

**Scope:** Attach a `diagnostics.NewParserErrorListener(path, d)` to each parser before invoking `ParseScriptFile()` so lexer/parser syntax errors flow into the parse-phase `*Diagnostics`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/pack/compiler/runescript/server_script_compiler_test.go`:

```go
// TestRun_ParserSyntaxErrorReachesParseDiagnostics pins that a syntactically
// invalid source file produces at least one diagnostic in the parse-phase
// *Diagnostics handed to HandleParse. The fixture uses an obviously bad
// `[` start with no closing bracket; the lexer/parser will report a
// SyntaxError which the new ParserErrorListener forwards into d.
func TestRun_ParserSyntaxErrorReachesParseDiagnostics(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "bad.rs2"), []byte("[proc,\n"), 0o644); err != nil {
		t.Fatalf("write bad.rs2: %v", err)
	}

	mapper := NewSymbolMapper(nil)
	rh := newRecordingHandler()
	c := &ServerScriptCompiler{
		SourcePaths:     []string{tmpDir},
		TypeManager:     typ.NewTypeManager(),
		Mapper:          mapper,
		CommandPointers: map[string]*pointer.PointerHolder{"foo": {Required: pointer.NewPointerSet()}},
		Writer:          &noopBinaryOutput{},
		Handler:         rh,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	err := c.Run("rs2")
	// Run returns an error from parsePhase ("parse: diagnostics reported errors").
	if err == nil {
		t.Fatal("Run on syntactically invalid source: got nil error, want non-nil")
	}
	parseDiag, ok := rh.capDiags["HandleParse"]
	if !ok || parseDiag == nil {
		t.Fatal("HandleParse was not called")
	}
	if !parseDiag.HasErrors() {
		t.Errorf("parse-phase Diagnostics has no errors after invalid source; got %d entries", len(parseDiag.List()))
	}
}
```

Add `"os"` and `"path/filepath"` to test imports if not present.

- [ ] **Step 2: Run the new test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestRun_ParserSyntaxErrorReachesParseDiagnostics -v`
Expected: FAIL — `parseDiag.HasErrors() == false` because the parser-error-listener isn't wired yet (parser silently swallows the error today).

- [ ] **Step 3: Wire the listener in `parsePhase`**

Edit `pkg/pack/compiler/runescript/server_script_compiler.go` `parsePhase`. Replace the parser-creation block inside the walk callback:

Old:
```go
			p := parser.NewScriptFileParser(string(content), path)
			node := p.ParseScriptFile()
```

New:
```go
			p := parser.NewScriptFileParser(string(content), path)
			p.AddErrorListener(diagnostics.NewParserErrorListener(path, d))
			node := p.ParseScriptFile()
```

- [ ] **Step 4: Run the new test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestRun_ParserSyntaxErrorReachesParseDiagnostics -v`
Expected: PASS.

- [ ] **Step 5: Run full suite (regression check)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS. (Risk: previously-silent parser errors in fixtures may now surface. If a known-good fixture fails, the parser is genuinely reporting an error that was silently swallowed before; investigate and fix the fixture rather than reverting this change. Reference [[verify_implementer_claims]] — re-grep before attributing to a pre-existing issue.)

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/runescript/server_script_compiler.go \
        pkg/pack/compiler/runescript/server_script_compiler_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-211 T6 — wire ParserErrorListener in parsePhase

Attaches diagnostics.NewParserErrorListener(path, d) to each parser before
ParseScriptFile() so lexer/parser SyntaxError callbacks flow into the
parse-phase *Diagnostics. Surfaces previously-silent parser errors in
the diagnostic stream.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T7: `Compile(cfg)` facade — `Config.Handler` injection + `BaseDiagnosticsHandler` default

**Files:**
- Modify: `pkg/pack/compiler/runescript/compile.go`
- Modify: `pkg/pack/compiler/runescript/compile_test.go` (add 1 test)

**Scope:** Add `Config.Handler diagnostics.Handler` field; `Compile` defaults nil to `&diagnostics.BaseDiagnosticsHandler{}` and assigns into the constructed `ServerScriptCompiler.Handler` before calling `c.Run`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/pack/compiler/runescript/compile_test.go`:

```go
// TestCompile_HandlerInjectionUsedDuringRun pins that Config.Handler is
// threaded into the constructed ServerScriptCompiler and receives the
// phase callbacks. Uses an empty SourcePaths + non-empty Symbols to
// reach Run without doing any real work.
func TestCompile_HandlerInjectionUsedDuringRun(t *testing.T) {
	tmpDir := t.TempDir()
	rh := newRecordingHandler()

	// Minimal Symbols sufficient to pass Compile's required-symbols check.
	syms := map[string]*CompilerTypeInfo{
		"command":    {Map: map[string]string{}},
		"runescript": {Map: map[string]string{}},
	}

	err := Compile(Config{
		SourcePaths: []string{tmpDir}, // empty dir → parsePhase walks zero files
		Symbols:     syms,
		Writer:      WriterConfig{Jag: &JagWriterConfig{Output: filepath.Join(tmpDir, "out")}},
		Handler:     rh,
	})
	// CommandPointers stays empty after LoadSpecialSymbols on these empty
	// CompilerTypeInfo maps; HandlePointerChecking is NOT called per
	// NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE.
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	want := []string{"HandleParse", "HandleTypeChecking", "HandleCodeGeneration"}
	if !equalStrings(rh.calls, want) {
		t.Errorf("Compile dispatch order: got %v, want %v", rh.calls, want)
	}
}

// TestCompile_NilHandlerDefaultsToBase pins that a nil Config.Handler is
// replaced with *BaseDiagnosticsHandler before Run() is invoked. Asserts
// no panic + completion; the printed output goes to BaseDiagnosticsHandler's
// default os.Stdout (acceptable for this test — there will be zero
// diagnostics from an empty SourcePaths).
func TestCompile_NilHandlerDefaultsToBase(t *testing.T) {
	tmpDir := t.TempDir()
	syms := map[string]*CompilerTypeInfo{
		"command":    {Map: map[string]string{}},
		"runescript": {Map: map[string]string{}},
	}
	err := Compile(Config{
		SourcePaths: []string{tmpDir},
		Symbols:     syms,
		Writer:      WriterConfig{Jag: &JagWriterConfig{Output: filepath.Join(tmpDir, "out")}},
		Handler:     nil,
	})
	if err != nil {
		t.Fatalf("Compile with nil Handler: %v", err)
	}
}
```

Add `"path/filepath"` to test imports if not present. `newRecordingHandler` / `equalStrings` live in `server_script_compiler_test.go` (same package).

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestCompile_HandlerInjectionUsedDuringRun -v`
Expected: FAIL — `unknown field Handler in struct literal of type Config`.

- [ ] **Step 3: Add `Handler` to `Config` and wire it through `Compile`**

Edit `pkg/pack/compiler/runescript/compile.go`. Replace the `Config` struct:

Old:
```go
type Config struct {
	SourcePaths   []string
	ExcludePaths  []string
	Symbols       map[string]*CompilerTypeInfo
	CheckPointers *bool // nil → default true
	Features      semantics.StrictFeatureLevel
	Writer        WriterConfig
}
```

New:
```go
type Config struct {
	SourcePaths   []string
	ExcludePaths  []string
	Symbols       map[string]*CompilerTypeInfo
	CheckPointers *bool // nil → default true
	Features      semantics.StrictFeatureLevel
	Writer        WriterConfig
	// Handler receives the per-phase Diagnostics from each phase of
	// ServerScriptCompiler.Run. Nil defaults to
	// &diagnostics.BaseDiagnosticsHandler{} (TS-faithful: TS uses
	// BaseDiagnosticsHandler when CompileServerScript is invoked without
	// an override).
	Handler diagnostics.Handler
}
```

Then, in the `Compile` function body, replace the `ServerScriptCompiler` literal construction. Locate:

Old:
```go
	c := &ServerScriptCompiler{
		SourcePaths:     absSources,
		ExcludePaths:    absExcludes,
		TypeManager:     typ.NewTypeManager(),
		Triggers:        trigger.NewTriggerManager(),
		RootTable:       symbol.NewSymbolTable(nil),
		DynHandlers:     map[string]semantics.DynamicCommandHandler{},
		CompilerSymbols: cfg.Symbols,
		Mapper:          mapper,
		CommandPointers: commandPointers,
		Features:        cfg.Features,
		Writer:          writer,
	}
```

New:
```go
	handler := cfg.Handler
	if handler == nil {
		handler = &diagnostics.BaseDiagnosticsHandler{}
	}
	c := &ServerScriptCompiler{
		SourcePaths:     absSources,
		ExcludePaths:    absExcludes,
		TypeManager:     typ.NewTypeManager(),
		Triggers:        trigger.NewTriggerManager(),
		RootTable:       symbol.NewSymbolTable(nil),
		DynHandlers:     map[string]semantics.DynamicCommandHandler{},
		CompilerSymbols: cfg.Symbols,
		Mapper:          mapper,
		CommandPointers: commandPointers,
		Features:        cfg.Features,
		Writer:          writer,
		Handler:         handler,
	}
```

Add `"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"` to the imports.

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestCompile -v`
Expected: PASS for both new tests + existing compile tests still PASS.

- [ ] **Step 5: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/runescript/compile.go \
        pkg/pack/compiler/runescript/compile_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-211 T7 — Compile facade Handler injection

Adds Config.Handler (diagnostics.Handler); Compile threads it into the
constructed ServerScriptCompiler.Handler. Nil defaults to
&diagnostics.BaseDiagnosticsHandler{} so the public entry prints
diagnostics by default (TS-faithful).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T8: Deviation pins + close

**Files:**
- Create: `pkg/pack/compiler/runescript/nai211_deviation_pins_test.go`

**Scope:** Pin the three new NAI-211 deviation tags to at least one production touch point each. Run full suite + `-race`. Commit close.

- [ ] **Step 1: Write the pin file**

Create `pkg/pack/compiler/runescript/nai211_deviation_pins_test.go`:

```go
// pkg/pack/compiler/runescript/nai211_deviation_pins_test.go
package runescript

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeviationPinsLive_NAI211 grep-pins every NAI-211-introduced
// deviation tag to at least one production .go source. Mirrors the
// NAI-210 pin pattern.
func TestDeviationPinsLive_NAI211(t *testing.T) {
	tags := []struct{ Tag, Why string }{
		{"NAI-211-D-NO-PROCESS-EXIT", "BaseDiagnosticsHandler is print-only; Run() returns error instead of process.exit(1)"},
		{"NAI-211-D-MACRO-LOOKUP-DEFERRED", "TS BaseDiagnosticsHandler.macroLookup omitted; macros not yet ported"},
		{"NAI-211-D-PHASE-DIAGNOSTICS-FRESH", "Each phase allocates its own *Diagnostics; pre-NAI-211 shared accumulator retired"},
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := cwd
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatalf("go.mod not found walking up from %s", cwd)
		}
		root = parent
	}

	scanDirs := []string{"pkg", "modules", "cmd"}
	hits := map[string][]string{}
	for _, dir := range scanDirs {
		base := filepath.Join(root, dir)
		_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			if strings.HasSuffix(path, "nai211_deviation_pins_test.go") {
				return nil // skip self
			}
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return nil
			}
			content := string(b)
			for _, tag := range tags {
				if strings.Contains(content, tag.Tag) {
					hits[tag.Tag] = append(hits[tag.Tag], path)
				}
			}
			return nil
		})
	}

	for _, tag := range tags {
		t.Run(tag.Tag, func(t *testing.T) {
			files := hits[tag.Tag]
			productionHit := false
			for _, f := range files {
				if !strings.HasSuffix(f, "_test.go") {
					productionHit = true
					break
				}
			}
			if !productionHit {
				t.Errorf("tag %s has no production touch point (%s); hits=%v", tag.Tag, tag.Why, files)
			}
		})
	}
}
```

- [ ] **Step 2: Run the pin test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestDeviationPinsLive_NAI211 -v`
Expected: All three subtests PASS. If one fails, find the matching tag in the spec deviation table and add the comment to the production file noted in **File Structure → "set in this slice"** above.

Specifically:
- `NAI-211-D-NO-PROCESS-EXIT` should appear in the `BaseDiagnosticsHandler` doc-comment in `pkg/pack/compiler/diagnostics/base_handler.go` (placed in T1).
- `NAI-211-D-MACRO-LOOKUP-DEFERRED` should appear in the same doc-comment (placed in T1).
- `NAI-211-D-PHASE-DIAGNOSTICS-FRESH` should appear in the `Handler` field doc-comment in `pkg/pack/compiler/runescript/server_script_compiler.go` (placed in T5).

- [ ] **Step 3: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 4: Run race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: PASS (or only pre-existing race failures unchanged).

- [ ] **Step 5: Commit close**

```bash
git add pkg/pack/compiler/runescript/nai211_deviation_pins_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
close(compiler/runescript): NAI-211 T8 — deviation pins + close

Closes NAI-211 (NAI-210 follow-up #2: per-phase Diagnostics +
BaseDiagnosticsHandler). Three new deviation tags pinned by
nai211_deviation_pins_test.go:

  - NAI-211-D-NO-PROCESS-EXIT — BaseDiagnosticsHandler is print-only;
    Run() returns error instead of process.exit(1)
  - NAI-211-D-MACRO-LOOKUP-DEFERRED — TS macroLookup field omitted;
    macros not yet ported
  - NAI-211-D-PHASE-DIAGNOSTICS-FRESH — each phase allocates its own
    *Diagnostics; pre-NAI-211 shared accumulator retired

Unblocks NAI-210 follow-up #1 (richer driver smoke fixture).

Closes memory: nai211_close.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Post-Close

After T8 lands, save a memory entry `nai211_close.md` with:
- close commit SHA
- live deviation tags (3)
- any emergent follow-ups discovered during implementation (e.g., parser fixtures that started failing after T6 and needed adjustment)

Per [[post_task_handoff]], produce a paste-ready resume prompt for the next session pointing at:
- The remaining NAI-210 follow-up #1 (richer driver smoke fixture), now unblocked by NAI-211.
- The pre-existing working-tree mods (Dockerfile, Makefile, build.sh, config.yaml, go.mod) carried since before the compiler series.

---

## Spec Coverage Self-Review

Mapping spec sections → tasks:

| Spec section | Task(s) |
|---|---|
| Architecture → Field reshape | T5 |
| Architecture → Per-phase fresh `Diagnostics` | T5 |
| Architecture → `BaseDiagnosticsHandler` | T1, T2, T3 |
| Architecture → `ParserErrorListener` adapter | T4, T6 |
| Architecture → `Compile(cfg)` handler injection | T7 |
| Components Summary table | T1-T7 (all rows) |
| Data Flow → phase boundary diagram | T5 (Run + each phase method) |
| Data Flow → checkPointers no-dispatch when empty | T5 (`checkPointersPhase` body) |
| Error Handling → error-return preserved | T5 |
| Error Handling → NopHandler vs BaseDiagnosticsHandler defaults | T5 (Run), T7 (Compile) |
| Deviations table | T1 (NO-PROCESS-EXIT + MACRO-LOOKUP-DEFERRED comment placement), T5 (PHASE-DIAGNOSTICS-FRESH comment placement), T8 (pin tests) |
| Testing Strategy → base_handler_test.go (7 tests) | T1 (1) + T2 (1) + T3 (5) |
| Testing Strategy → parser_error_listener_test.go (2 tests) | T4 |
| Testing Strategy → server_script_compiler_test.go (5 new tests) | T5 (4 tests) + T6 (1 test) |
| Testing Strategy → nai211_deviation_pins_test.go | T8 |
| Testing Strategy → Regression check | T5 step 10, T6 step 5, T7 step 5, T8 step 3 |
| Open Decisions Resolved | T1-T7 (folded into impl) |

All spec sections covered. No placeholders or TODOs in steps. Type/method names consistent across tasks: `Handler` field, `c.Handler.HandleXxx`, `*diagnostics.Diagnostics`, `*diagnostics.BaseDiagnosticsHandler`, `*diagnostics.ParserErrorListener`, `diagnostics.NewParserErrorListener`, `diagnostics.NopHandler{}`.
