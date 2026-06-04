# NAI-211-FU — Codegen-Error Dispatch Pin

## Status

Combined spec + plan. Compressed cadence per [[compressed_cadence]] memory:
single defensive test addition, ~60-80 LOC, no production change, single
subagent execution with self-review.

## Tech Stack

Go 1.26+. Test-only. No dependencies added.

## Motivation

NAI-211 close (commit `159e7f1`) identified this follow-up:

> **NAI-211-FU-CODEGEN-ERROR-DISPATCH-PIN**: spec §Testing Strategy item 5
> (`TestRun_HandlerInvokedEvenWhenPhaseErrors`) was not implemented as
> written. Plan's self-review misclassified T6's parser-syntax-error test
> as covering this; they pin different invariants. The codegen-phase
> "HandleCodeGeneration called BEFORE Run halts on error" ordering is
> currently only structurally enforced by source-line order in
> `codegenPhase` (server_script_compiler.go:233-235). Defensive pin against
> future reorder would need a fixture that survives parse+analyze but
> trips codegen `d.HasErrors()`. Low priority — structural correctness
> verified by final review.

The parse phase already has this functional pin
(`TestRun_ParserSyntaxErrorReachesParseDiagnostics` in
`server_script_compiler_test.go`). This follow-up adds the symmetric pin
for the codegen phase.

## Invariant Being Pinned

`codegenPhase` (`server_script_compiler.go:225-238`) dispatches to
`c.Handler.HandleCodeGeneration(d)` BEFORE checking `d.HasErrors()`. If
someone refactors to dispatch-after-halt:

```go
// REGRESSION SHAPE:
if d.HasErrors() {
    return nil, fmt.Errorf("codegen: diagnostics reported errors")
}
c.Handler.HandleCodeGeneration(d)
```

…the handler is never invoked on error paths and downstream tooling
(IDE plugins, build daemons, error-reporting wrappers) loses visibility
into codegen-phase diagnostics. This is the parallel of the parse-phase
ordering that NAI-211 T6 already pins.

## Approach

Inject a codegen-phase error through a test-only `DynamicCommandHandler`
registered in `c.DynHandlers`. The dyn-handler dispatch path in
`codegen/codegen_call.go:21` is unconditional and name-keyed —
`emitDynamicCommand(cc.NameString(), cc)` is invoked for every
`CommandCallExpression` with a non-nil `Symbol`. By installing a handler
under a sentinel name and writing a source fixture that invokes that
command, we trigger codegen-phase diagnostic accumulation without
requiring any production-code injection point.

### Why not a source-shape pin via `go/parser`?

Considered. Rejected because:

- Doesn't pin runtime semantic — only source-line order in a specific
  function. An equivalent refactor that extracts a helper (e.g.,
  `c.dispatchCodegen(d)` containing the call) would silently neuter the
  pin.
- The functional pin is roughly the same LOC cost.

### Why not declared WONTFIX?

Considered. The parse-phase analog DOES have a functional pin; leaving
codegen asymmetric long-term invites the same drift NAI-211's
post-mortem identified. The cost is bounded and the dyn-handler hook is
a legitimate codegen-time injection point that already exists in
production.

## Design

### New test file

`pkg/pack/compiler/runescript/server_script_compiler_codegen_dispatch_test.go`

Lives sibling to `server_script_compiler_test.go`. Separate file
(rather than appending to the existing test) so the follow-up tag is
discoverable by `rg NAI-211-FU-CODEGEN-ERROR-DISPATCH-PIN`.

### Test-only handler

```go
// codegenErrorInjector is a DynamicCommandHandler used only by
// TestRun_HandleCodeGenerationDispatchedBeforeHalt to trigger a
// codegen-phase diagnostic error. TypeCheck is a no-op (the symbol's
// static Parameters=MetaUnit / Returns=MetaUnit suffice). GenerateCode
// reports an error against the passed Diagnostics, simulating the
// "codegen produced an error" path.
type codegenErrorInjector struct{}

func (codegenErrorInjector) TypeCheck(_ *semantics.TypeCheckingContext) {}

func (codegenErrorInjector) GenerateCode(ctx semantics.CodeGenContext) bool {
    cgc := ctx.(*codegen.CodeGeneratorContext)
    diagnostics.ReportErrorAt(cgc.Diagnostics, cgc.Expression,
        "NAI-211-FU test-injected codegen error")
    return true
}
```

### Test body

```go
// TestRun_HandleCodeGenerationDispatchedBeforeHalt pins
// NAI-211-FU-CODEGEN-ERROR-DISPATCH-PIN: when codegen reports an error,
// HandleCodeGeneration MUST be invoked BEFORE Run() halts. Regression
// shape: reordering codegenPhase to `if HasErrors return` BEFORE the
// handler dispatch would cause this assertion to fail.
func TestRun_HandleCodeGenerationDispatchedBeforeHalt(t *testing.T) {
    tmpDir := t.TempDir()
    src := "[proc,test]()\n_codegen_err_inject();\n"
    if err := os.WriteFile(filepath.Join(tmpDir, "test.rs2"), []byte(src), 0o644); err != nil {
        t.Fatalf("write fixture: %v", err)
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

    // Insert the sentinel command symbol so the source parses + typechecks.
    c.RootTable.Insert(
        symbol.SymbolTypeServerScript(trigger.CommandTrigger),
        &symbol.ServerScriptSymbol{
            ScriptSymbolFields: symbol.ScriptSymbolFields{
                Trigger:    trigger.CommandTrigger,
                Name:       "_codegen_err_inject",
                Parameters: typ.MetaUnit,
                Returns:    typ.MetaUnit,
            },
        },
    )
    // Install the codegen-phase error injector AFTER Setup (Setup overwrites
    // the map via RegisterAllDynCommands but does not touch this sentinel name).
    c.DynHandlers["_codegen_err_inject"] = codegenErrorInjector{}

    err := c.Run("rs2")
    if err == nil {
        t.Fatal("Run with codegen-error injector: got nil error, want non-nil")
    }
    if !strings.Contains(err.Error(), "codegen") {
        t.Errorf("Run error message: got %q, want substring %q", err.Error(), "codegen")
    }

    // Primary pin: HandleCodeGeneration was dispatched.
    if _, ok := rh.capDiags["HandleCodeGeneration"]; !ok {
        t.Fatal("HandleCodeGeneration was NOT called on codegen-error path " +
            "(NAI-211-FU regression: dispatch reordered after halt?)")
    }

    // Secondary pin: the diag handed to the handler contains the codegen error.
    cgDiag := rh.capDiags["HandleCodeGeneration"]
    if !cgDiag.HasErrors() {
        t.Errorf("HandleCodeGeneration diag has no errors; want at least one " +
            "(handler invoked but with stale/empty diag — possible accumulator regression)")
    }
}
```

### Verification of drift detection

- **Baseline (current source order at server_script_compiler.go:233-235):**
  HandleCodeGeneration called → rh.calls includes it → primary pin
  passes. Diag contains the injected error → secondary pin passes. Run
  returns codegen error → error pin passes.
- **Reordered (dispatch-after-halt):** d.HasErrors() returns early → Run
  returns codegen error → HandleCodeGeneration NEVER called → primary
  pin FAILS with the regression message.
- **Per-phase accumulator regression (codegen shares parse's diag):**
  Possible NAI-211-D-PHASE-DIAGNOSTICS-FRESH regression. Detected
  separately by `nai211_deviation_pins_test.go`; not a concern here.

## Testing Strategy

1. **Compile check.** `go build ./pkg/pack/compiler/runescript/...` passes.
2. **Single-test verification.**
   `go test -run TestRun_HandleCodeGenerationDispatchedBeforeHalt ./pkg/pack/compiler/runescript/...`
   passes.
3. **Full suite parity.** `go test ./...` PASS — no new failures.
4. **Race-detector parity.** `go test -race ./...` PASS — handler+diag
   stays single-goroutine (no concurrency added).
5. **Drift sanity check (one-shot, NOT committed).** Temporarily swap
   the codegenPhase source order (move `HandleCodeGeneration(d)` AFTER
   the `if d.HasErrors()` block), confirm the new test FAILS with the
   expected message, then revert the production change.

## Deviation Tags

None expected. This is a pure-test addition with no production-side
divergence from the TS port.

## Out of Scope

- **Symmetric analyze-phase pin.** `analyzePhase` (line 176-178) has the
  same dispatch-before-halt pattern but isn't called out in the
  NAI-211 follow-up. If it surfaces as a separate regression target,
  open a new follow-up.
- **Refactoring `codegenPhase` to extract a dispatch helper.** Not
  required — current shape is fine.
- **Per-phase Diagnostics regression coverage.** Already pinned by
  `nai211_deviation_pins_test.go` per the NAI-211 close.

## Execution

Single task — no separate plan doc:

**T1.** Add `server_script_compiler_codegen_dispatch_test.go` as
specified above. Run `go test ./...` + `go test -race ./...`. Commit as
`test(compiler/runescript): NAI-211-FU pin HandleCodeGeneration dispatch before codegen halt`.

After T1: brief self-review (verify Testing Strategy items 1-4 pass; do
NOT commit the §Testing Strategy item 5 drift sanity check). No formal
review cycle (compressed cadence).

**T2 (close).** Empty commit with body summarizing the closure,
following the NAI-N close commit convention (Closes memory trailer per
[[close_commit_memory_trailer]]).

## References

- Parent: [[nai211_close]] — original follow-up call-out.
- Sibling: [[nai210_fu1_close]] — reaffirms this is the only remaining
  NAI-210/211 follow-up.
- Pattern: [[compressed_cadence]] — compressed-cadence threshold.
- Related: `TestRun_ParserSyntaxErrorReachesParseDiagnostics` in
  `server_script_compiler_test.go:247` — parse-phase analog.
