# NAI-211 — Per-phase Diagnostics + BaseDiagnosticsHandler

**Date:** 2026-05-16
**Status:** spec
**Predecessors:** NAI-205 (`Handler` interface + `NopHandler`), NAI-210 (driver shape, deferred per-phase dispatch)
**Follow-ups it unblocks:** NAI-210-FU-RICHER-DRIVER-SMOKE

## Tech Stack

Go 1.26+. Packages touched: `pkg/pack/compiler/diagnostics`, `pkg/pack/compiler/runescript`, `pkg/pack/compiler/parser` (wiring only).

## Goal

Complete the port of TS `ScriptCompiler.compile` (`RuneScriptTS/src/compiler/ScriptCompiler.ts` L238-265) so each pipeline phase gets a fresh `Diagnostics` accumulator and dispatches it through the `Handler` interface ported in NAI-205. Land `BaseDiagnosticsHandler` as the default user-facing concrete implementation, mirroring TS `BaseDiagnosticsHandler` (`RuneScriptTS/src/compiler/diagnostics/DiagnosticsHandler.ts` L47-147).

## Motivation

NAI-205 ported the `Handler` interface + `NopHandler` but never wired the dispatch. NAI-210 then landed `ServerScriptCompiler.Run` with one shared `c.DiagHandler *diagnostics.Diagnostics` accumulator across all phases — drift from the TS shape, which uses a `DiagnosticsHandler` interface field with per-phase fresh `Diagnostics` instances. The NAI-210 spec (`2026-05-16-nai-210-driver-and-output-sinks-design.md:473`) actually prescribed `DiagHandler diagnostics.Handler` but the implementation regressed. NAI-211 reconciles the structural debt and surfaces parser errors (which currently flow through `lexer.ErrorListener` but never reach the driver) into the diagnostic stream.

## Architecture

### Field reshape on `ServerScriptCompiler`

```go
type ServerScriptCompiler struct {
    // ... unchanged ...
    Handler diagnostics.Handler   // was: DiagHandler *diagnostics.Diagnostics
    // ... unchanged ...
}
```

`Run()` opens with a nil-default:

```go
if c.Handler == nil {
    c.Handler = diagnostics.NopHandler{}
}
```

### Per-phase fresh `Diagnostics`

Each phase method allocates a local `d := &diagnostics.Diagnostics{}`, passes it to the phase constructor unchanged (`NewScriptRegistration`/`NewTypeChecker`/`NewCodeGenerator`/`NewServerPointerChecker` already take `*Diagnostics`), then at phase end dispatches `c.Handler.HandleXxx(d)` and halts on `d.HasErrors()` with `fmt.Errorf("<phase>: diagnostics reported errors")`.

```
loadSymbols                         (no diagnostics — TS L229-233)
  ↓
parsePhase                          d := &Diagnostics{}
                                    parser.AddErrorListener(diagnostics.NewParserErrorListener(path, d))
                                    c.Handler.HandleParse(d)
                                    halt on d.HasErrors()
  ↓
analyzePhase                        d := &Diagnostics{}
                                    ScriptRegistration.Visit + registerSecondaryCommands + TypeChecker.Visit
                                    c.Handler.HandleTypeChecking(d)
                                    halt on d.HasErrors()
  ↓
codegenPhase                        d := &Diagnostics{}
                                    CodeGenerator.Visit per file
                                    c.Handler.HandleCodeGeneration(d)
                                    halt on d.HasErrors()
  ↓
checkPointersPhase                  if len(CommandPointers) < 1 → return halt=true
                                    (TS-faithful: NO Handler call, NO Diagnostics created)
                                    d := &Diagnostics{}
                                    NewServerPointerChecker(d, ...).Run()
                                    c.Handler.HandlePointerChecking(d)
                                    return d.HasErrors()
  ↓
writePhase / Close
```

`Run` keeps its `error`-return signature; TS `process.exit(1)` is replaced by goscape's error-propagation. See `NAI-211-D-NO-PROCESS-EXIT`.

### `BaseDiagnosticsHandler` (new)

File: `pkg/pack/compiler/diagnostics/base_handler.go`.

```go
type BaseDiagnosticsHandler struct {
    Out io.Writer // defaults to os.Stdout when nil at call time
}

func (h *BaseDiagnosticsHandler) HandleParse(d *Diagnostics)           { h.handleShared(d) }
func (h *BaseDiagnosticsHandler) HandleTypeChecking(d *Diagnostics)    { h.handleShared(d) }
func (h *BaseDiagnosticsHandler) HandleCodeGeneration(d *Diagnostics)  { h.handleShared(d) }
func (h *BaseDiagnosticsHandler) HandlePointerChecking(d *Diagnostics) { h.handleShared(d) }

func (h *BaseDiagnosticsHandler) handleShared(d *Diagnostics) {
    out := h.Out
    if out == nil {
        out = os.Stdout
    }
    fileLines := map[string][]string{} // per-call cache — mirrors TS L75
    for _, diag := range d.List() {
        // Resolve absolute path; lazy-read + cache lines on first hit.
        // Print "<path>:<line>:<col>: <TYPE>: <formatted-message>"
        // Print source line with tabs → 4 spaces, prefixed "    > "
        // Print caret with " " * (tabCount*3 + (col-1)), prefixed "    > "
        // If file unreadable or line out of bounds: skip source/caret lines.
    }
    // NO process.exit — see NAI-211-D-NO-PROCESS-EXIT.
}
```

Message formatting uses `fmt.Sprintf(diag.Message, diag.MessageArgs...)`. Verified safe: `messages.go` uses only `%s` verbs.

Caret offset formula `tabCount*3 + (column-1)` mirrors TS L112 exactly — accounts for tab→4-space expansion.

`MacroLookup` / `MacroOrigin` / `MacroLookupResult` types and the `macroLookup` field are omitted — macros aren't ported yet (see `parsePhase` deferral comment at `server_script_compiler.go:120`). Tagged `NAI-211-D-MACRO-LOOKUP-DEFERRED`; will re-introduce in the macro slice.

### `ParserErrorListener` adapter (new)

File: `pkg/pack/compiler/diagnostics/parser_error_listener.go`. Lives in `diagnostics` (not `parser`) because `diagnostics` already imports `lexer` (for `NodeSourceLocation`); keeps `parser` package dependency-light.

```go
type ParserErrorListener struct {
    SourceName string
    Diag       *Diagnostics
}

func NewParserErrorListener(sourceName string, d *Diagnostics) *ParserErrorListener {
    return &ParserErrorListener{SourceName: sourceName, Diag: d}
}

func (p *ParserErrorListener) SyntaxError(_ string, line, column int, msg string) {
    p.Diag.Report(Diagnostic{
        Type:           DiagnosticSyntaxError,
        SourceLocation: lexer.NodeSourceLocation{Name: p.SourceName, Line: line, Column: column},
        Message:        "%s",
        MessageArgs:    []any{msg},
    })
}
```

Implements `lexer.ErrorListener` structurally. Constructor `sourceName` overrides the callback's `sourceName` arg — matches TS `ParserErrorListener` constructor-file-path behavior.

### `Compile(cfg)` facade — handler injection

```go
type Config struct {
    // ... unchanged ...
    Handler diagnostics.Handler  // nil → &diagnostics.BaseDiagnosticsHandler{}
}
```

In `Compile`:

```go
c.Handler = cfg.Handler
if c.Handler == nil {
    c.Handler = &diagnostics.BaseDiagnosticsHandler{}
}
```

Public-entry default is `BaseDiagnosticsHandler` (user-facing tool prints diagnostics). Direct-struct usage in tests defaults to `NopHandler` (silent). Tests that want to assert on output construct a `BaseDiagnosticsHandler{Out: &bytes.Buffer{}}`.

## Components Summary

| Component | File | New / Modified |
|---|---|---|
| `Handler` interface | `diagnostics/handler.go` | unchanged (from NAI-205) |
| `NopHandler` | `diagnostics/handler.go` | unchanged (from NAI-205) |
| `BaseDiagnosticsHandler` | `diagnostics/base_handler.go` | **new** |
| `ParserErrorListener` | `diagnostics/parser_error_listener.go` | **new** |
| `ServerScriptCompiler.Handler` field | `runescript/server_script_compiler.go` | renamed from `DiagHandler` |
| `parsePhase` wires parser-error listener | `runescript/server_script_compiler.go` | modified |
| Per-phase fresh `Diagnostics` | `runescript/server_script_compiler.go` | modified (each phase method) |
| `Config.Handler` injection | `runescript/compile.go` | **new field** |

## Data Flow

See ASCII diagram in **Architecture → Per-phase fresh `Diagnostics`** above.

Key TS-faithfulness pins:

1. `checkPointers` empty-CommandPointers early-return happens **before** `new Diagnostics()` is created — so `HandlePointerChecking` is NOT called in that path. Existing `NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE` covers the halt; NAI-211 honors the no-dispatch contract.
2. `loadSymbols` and `registerSecondaryCommands` do not produce diagnostics — outside per-phase scoping.
3. `BaseDiagnosticsHandler` prints **before** the phase function checks `d.HasErrors()`. On failure, the user sees diagnostics before `Run` returns `error`.

## Error Handling

- `Run()` keeps `error` return. Each phase returns `fmt.Errorf("<phase>: diagnostics reported errors")` when `d.HasErrors()` after dispatch.
- `BaseDiagnosticsHandler` is print-only; no `os.Exit` (`NAI-211-D-NO-PROCESS-EXIT`).
- `NopHandler` is the default for direct `ServerScriptCompiler` use (silent). `BaseDiagnosticsHandler{}` is the default through `Compile(cfg)` (user-facing).
- File-read errors during pretty-printing are swallowed; the diagnostic header still prints. Line-out-of-bounds is swallowed; same.

## Deviations

| Tag | Where | Reason |
|---|---|---|
| `NAI-211-D-NO-PROCESS-EXIT` | `diagnostics/base_handler.go` | TS `process.exit(1)` after error printing replaced by goscape's `Run() → error` return. `BaseDiagnosticsHandler` is print-only; control flow stays in the pipeline. |
| `NAI-211-D-MACRO-LOOKUP-DEFERRED` | `diagnostics/base_handler.go` | TS `BaseDiagnosticsHandler.macroLookup` + `MacroLookup` / `MacroLookupResult` / `MacroOrigin` types omitted — macros aren't ported yet. Re-introduce in the macro slice. |
| `NAI-211-D-PHASE-DIAGNOSTICS-FRESH` | `runescript/server_script_compiler.go` | TS-faithful: each phase creates its own `&Diagnostics{}`. The pre-NAI-211 shared accumulator (`c.DiagHandler *Diagnostics`) is retired. Structural pin. |

All three get pin tests in `nai211_deviation_pins_test.go`. Each tag must appear as a comment in production code (not just `_test.go`) — placement: `NO-PROCESS-EXIT` in `base_handler.go` (above `handleShared`), `MACRO-LOOKUP-DEFERRED` in `base_handler.go` (above the struct or in package doc), `PHASE-DIAGNOSTICS-FRESH` in `server_script_compiler.go` (above `Run` or above the first phase method). The `pin(t, dir, tag)` helper (`semantics/nai205_deviation_pins_test.go:36`) is reusable; negative-grep assertions reuse `readPackageFiles` + `!strings.Contains`.

## Testing Strategy

### `diagnostics/base_handler_test.go` (new)

1. `TestBaseDiagnosticsHandler_FormatsLocationTypeMessage` — one ERROR diag at line 2 col 5 of a temp file; assert output starts `<path>:2:5: ERROR: <msg>`.
2. `TestBaseDiagnosticsHandler_RendersSourceLineAndCaret` — temp file `"line1\n\thello\n"`, diag at line 2 col 2; assert output contains `    >     hello` (tab→4 spaces) and caret offset of 4 spaces (`tabCount*3 + (col-1) = 3 + 1`).
3. `TestBaseDiagnosticsHandler_LineOutOfBoundsSkipsSource` — diag at line 99 of a 3-line file; assert no `>` line printed; location/message still printed.
4. `TestBaseDiagnosticsHandler_FileMissingSkipsSource` — diag pointing at nonexistent path; assert location/message still printed; no panic.
5. `TestBaseDiagnosticsHandler_MessageArgsFormatted` — diag with `Message="bad %s"`, `MessageArgs=["foo"]`; assert `bad foo` in output.
6. `TestBaseDiagnosticsHandler_NoOsExit` — pin for `NAI-211-D-NO-PROCESS-EXIT`: handler with `hasErrors()=true` returns normally; test-process survival is the assertion.
7. `TestBaseDiagnosticsHandler_AllFourPhaseMethodsDispatchSame` — call each `HandleXxx(d)`, capture each output, assert all four equal (TS L58-72 all four delegate to `handleShared`).

### `diagnostics/parser_error_listener_test.go` (new)

1. `TestParserErrorListener_SyntaxErrorPushesDiagnostic` — call `.SyntaxError("foo.rs2", 4, 7, "expected token")`; assert `Diag.List()` contains one entry with type `DiagnosticSyntaxError`, location `{Name:"foo.rs2", Line:4, Column:7}`, message `"%s"`, args `["expected token"]`.
2. `TestParserErrorListener_SourceNameOverridesCallback` — pass `sourceName="ctor.rs2"` in ctor, invoke `.SyntaxError("cb.rs2", 1, 1, "msg")`; assert `Diag.List()[0].SourceLocation.Name == "ctor.rs2"`. Mirrors TS ParserErrorListener constructor-path behavior.

### `runescript/server_script_compiler_test.go` (extend)

1. `TestRun_HandlerDispatchedInOrder` — fake `Handler` records `[]string` of method names called; run with valid empty fixture; assert sequence is `["HandleParse","HandleTypeChecking","HandleCodeGeneration"]` (no `HandlePointerChecking` — empty CommandPointers per `NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE`).
2. `TestRun_HandlerDispatchedForPointerChecking` — fixture with non-empty CommandPointers; assert sequence ends with `HandlePointerChecking`.
3. `TestRun_PerPhaseDiagnosticsAreFresh` — fake Handler captures `len(d.List())` per phase; force a codegen-only error via injected fixture; assert HandleParse + HandleTypeChecking each got `d` with `len(List())==0`, HandleCodeGeneration got >0. Pins per-phase isolation.
4. `TestRun_NilHandlerDefaultsToNop` — `c.Handler == nil` at entry; `Run` defaults, does not panic, does not print to stdout. Captured via `os.Stdout` swap or via inspection of `c.Handler` after first phase.
5. `TestRun_HandlerInvokedEvenWhenPhaseErrors` — fake Handler records calls; force codegen error; assert `HandleCodeGeneration` was called BEFORE `Run` returns error (printing happens, then halt).

### `runescript/nai211_deviation_pins_test.go` (new)

Three subtests using the existing `pin(t, dir, tag)` helper from `nai205_deviation_pins_test.go`:

- `NAI-211-D-NO-PROCESS-EXIT` — `pin(t, "../diagnostics", "NAI-211-D-NO-PROCESS-EXIT")` + grep `base_handler.go` for `os.Exit`; assert zero matches.
- `NAI-211-D-MACRO-LOOKUP-DEFERRED` — `pin(t, "../diagnostics", "NAI-211-D-MACRO-LOOKUP-DEFERRED")` + grep `base_handler.go` for `MacroLookup\|macroLookup`; assert zero matches.
- `NAI-211-D-PHASE-DIAGNOSTICS-FRESH` — `pin(t, ".", "NAI-211-D-PHASE-DIAGNOSTICS-FRESH")` + grep `server_script_compiler.go` for `c\.DiagHandler`; assert zero matches (field is gone) + grep `&diagnostics\.Diagnostics{}` occurs ≥3 times (parse / analyze / codegen).

### Regression check

Full `go test ./...` after the field rename + parser-error-listener wiring. Risk: existing parser-driven tests that previously silently succeeded may now fail because parser errors flow into `d` and trip the `parse: diagnostics reported errors` early-return. The smoke driver fixture `[proc,hello]\n` (header-only) parses cleanly today and is expected to continue passing.

## Open Decisions Resolved

1. **Approach A vs B (dual-field) vs C (BaseHandler in/out of scope)** — A + BaseDiagnosticsHandler in scope (user-confirmed).
2. **Per-call vs per-handler-lifetime file-line cache** — per-call (mirrors TS L75).
3. **`Out` defaults to `os.Stdout`** — mirrors TS `console.log`.
4. **`Compile(cfg)` default Handler** — `BaseDiagnosticsHandler{}`. Direct struct use defaults to `NopHandler{}`.
5. **`ParserErrorListener` location** — `diagnostics` package (parser stays minimal).

## Out of Scope

- Macro support and `MacroLookup` plumbing (deferred to macro slice).
- `BaseDiagnosticsHandler` color output / terminal detection.
- Configurable error/warning gating (TS just dumps everything; we match).
- Richer driver-smoke fixture (NAI-210-FU-RICHER-DRIVER-SMOKE; this spec unblocks it).
- Replacing the existing `Diagnostics` accumulator implementation.

## References

- TS `ScriptCompiler.compile`: `RuneScriptTS/src/compiler/ScriptCompiler.ts` L218-265 (and per-phase methods L270-465).
- TS `BaseDiagnosticsHandler`: `RuneScriptTS/src/compiler/diagnostics/DiagnosticsHandler.ts` L47-147.
- TS `ParserErrorListener`: `RuneScriptTS/src/compiler/parser/ParserErrorListener.ts`.
- Predecessor specs: `2026-05-15-nai-205-typesys-symbol-script-registration-design.md`, `2026-05-16-nai-210-driver-and-output-sinks-design.md`.
- Existing `Handler` interface: `pkg/pack/compiler/diagnostics/handler.go` (NAI-205-D-HANDLER-REQUIRED-METHODS).
- Pin-test helper precedent: `pkg/pack/compiler/semantics/nai205_deviation_pins_test.go`.
