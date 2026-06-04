# NAI-210-FU-RICHER-DRIVER-SMOKE — Richer driver smoke fixture

**Date:** 2026-05-16
**Status:** spec
**Scope:** test-only follow-up to NAI-210 (driver + output sinks, shipped 2026-05-16 at `4a9b212`)
**Predecessor:** NAI-211 (per-phase Diagnostics + BaseDiagnosticsHandler, shipped 2026-05-16 at `08a1e09`) unblocked this work.
**Closes:** NAI-210 follow-up #1 ("RICHER-DRIVER-SMOKE") tracked in [[nai210_close]] / [[nai_followups]].

## 1. Motivation

The driver smokes at `pkg/pack/compiler/runescript/compile_test.go` exercise `runescript.Compile()` end-to-end through `parsePhase` → `analyzePhase` → `codegenPhase` → `checkPointersPhase` → `writePhase`. Today both writer-sink smokes (`TestCompile_JagWriter_EndToEnd`, `TestCompile_Js5Writer_EndToEnd`) use a header-only fixture:

```rs2
[proc,hello]
```

That source parses cleanly, but with no body it produces zero RuneScripts in codegen, so `writePhase` reaches the writer sink with an empty `buffers` map and emits an empty/minimal archive header. The tests only assert `Compile()` returns nil. No real bytes are pinned, so the end-to-end byte-production contract of either sink is unverified at the driver level.

The codegen-package smoke at `pkg/pack/compiler/codegen/smoke_test.go::TestPipeline_FullSliceWithWriter` already pins the helper-script header bytes via a `BinaryScriptWriter` + recording adapter, but it bypasses (a) the driver's `LoadSpecialSymbols` seed, (b) on-disk source file I/O via `SourcePaths`, and (c) the writer-sink layer (Jag / Js5 file emission). The driver-level pin's job is to verify those three things glue together correctly.

## 2. Goal

Replace the two header-only smokes with end-to-end fixtures that:

1. Drive `runescript.Compile()` with a real script body that produces a non-empty codegen output.
2. Read the resulting on-disk archive file(s).
3. Pin deterministic header bytes that prove the producing script's header was written through the sink's envelope.

## 3. Non-goals

- No new deviation tags. This is test-only.
- No golden byte file. The codegen + writer unit tests already pin opcode-stream encoding at finer granularity; a driver-level golden would be high maintenance for low marginal coverage.
- No multi-script or multi-file fixture. The single-proc fixture is the minimum that produces a non-empty script through the full pipeline.
- No NAI-211-FU-CODEGEN-ERROR-DISPATCH-PIN work (separate follow-up tracked in [[nai211_close]]).

## 4. Fixture

Single proc, written to `<tmp>/scripts/helper.rs2`:

```rs2
[proc,helper](int $n)(int)
return(calc($n * 2));
```

The body produces ~4 opcodes (`PushLocalVar $n`, `PushConstantInt(2)`, `Multiply`, `Return`). Verified by inspection: identical-shape sources are exercised by `codegen.smoke_test.go::TestPipeline_FullSlice` (where `helper` has body `return(calc($n*2));` and codegens to one Block).

All required types/triggers/dyn-handlers come from `ServerScriptCompiler.Setup()`. No symbol-table seeding is needed beyond what the existing smokes already provide. In particular:

- `ServerTriggerProc.Pointers == pointer.NewPointerSet(pointer.All...)` (verified at `pkg/pack/compiler/trigger/server_trigger_type.go:23`), so the script enters the `return` call with `active_player` already set — `Required: active_player` from the seeded command map is satisfied.
- `calc(...)` and `return(...)` are parser/codegen-level language constructs, not symbol-table commands, so they need no root-table entry.

The `Config.Symbols` seed is unchanged from the existing tests:

```go
Symbols: map[string]*CompilerTypeInfo{
    "command":    {Map: map[string]string{"0": "return"}, Require: map[string]string{"0": "active_player"}},
    "runescript": {Map: map[string]string{}},
},
```

## 5. Tests

### 5.1 `TestCompile_JagWriter_PinsScriptHeader`

Replaces `TestCompile_JagWriter_EndToEnd` (rename + behavior change; the old name's "end-to-end" framing is now a misnomer since the assertions are richer than nil-error).

**Setup:**

- `tmp := t.TempDir()`
- Write fixture to `<tmp>/scripts/helper.rs2`.
- `packDir := <tmp>/pack`
- Call `Compile(Config{SourcePaths: [<tmp>/scripts], Symbols: …, Features: StrictFeatureLevel{}, Writer: WriterConfig{Jag: &JagWriterConfig{Output: packDir}}})`.

**Assertions:**

1. `Compile()` returns nil.
2. `<packDir>/script.dat` exists.
3. `<packDir>/script.idx` exists.
4. `script.dat`:
   - `binary.BigEndian.Uint32(bytes[0:4]) == 1` (lastID+1 for one script with id 0)
   - `binary.BigEndian.Uint32(bytes[4:8]) == 27` (`jagFileVersion`)
   - bytes `[8:21] == "[proc,helper]"` (fullName start at offset 8, right after the 8-byte header)
   - byte `[21] == 0` (fullName NUL terminator)
   - bytes `[22:32] == "helper.rs2"` (sourceName)
   - byte `[32] == 0` (sourceName NUL terminator)
   - `int32(binary.BigEndian.Uint32(bytes[33:37])) == -1` (lookupKey for `SubjectMode.Name`)
   - byte `[37] == 0` (debugproc-zero)
   - `len(bytes) > 50` (opcode stream is non-empty)
5. `script.idx`:
   - `binary.BigEndian.Uint32(bytes[0:4]) == 1` (lastID+1)
   - `binary.BigEndian.Uint32(bytes[4:8]) > 30` (length of the single script's blob — non-empty)

### 5.2 `TestCompile_Js5Writer_PinsScriptHeader`

Replaces `TestCompile_Js5Writer_EndToEnd`.

**Setup:**

- `tmp := t.TempDir()`
- Write fixture to `<tmp>/scripts/helper.rs2`.
- `js5Out := <tmp>/pack/scripts.js5`
- Call `Compile(Config{SourcePaths: [<tmp>/scripts], Symbols: …, Features: StrictFeatureLevel{}, Writer: WriterConfig{Js5: &Js5WriterConfig{Output: js5Out}}})`.

**Assertions:**

1. `Compile()` returns nil.
2. `js5Out` exists.
3. File starts with the packedIndex envelope (gzip-compressed):
   - byte `[0] == 0x02` (compression type marker = `js5CompressionGzip`)
   - bytes `[1:5]`: `BE32 compressedLen` — assert `> 0`
   - bytes `[5:9]`: `BE32 origLen` — assert `> 0`
4. Gzip stream starts at file offset 9:
   - bytes `[9:11] == {0x1f, 0x8b}` (gzip magic)
   - byte `[18] == 0` (gzip OS byte = 0; re-pins `NAI-210-D-GZIP-OS-BYTE-ZEROED` at the driver level; the existing pin at the writer-unit level covers the local mutation, this one covers it through `Compile()`)
5. `len(bytes) > 50` (full file includes the packedIndex envelope + at least one packedGroup + lengths array).

Body bytes of the helper script (post-index, post-compression-type byte, post-len header) are NOT pinned at the driver level — the writer-unit tests at `pkg/pack/compiler/runescript/js5_pack_writer_test.go` cover the inner layout.

## 6. Tests preserved unchanged

- `TestCompile_MissingCoreSymbols_ReturnsError` — table-driven nil-symbol error path.
- `TestCompile_HandlerInjectionUsedDuringRun` — NAI-211 handler-dispatch test.
- `TestCompile_NilHandlerDefaultsToBase` — NAI-211 default-handler test.

## 7. Files touched

Modified:

- `pkg/pack/compiler/runescript/compile_test.go` — two test functions replaced in place; new helper assertions added.

No production code changes. No new deviation tag pins.

## 8. Risks

- **None identified.** The fixture has been verified by inspection against:
  - `pkg/pack/compiler/trigger/server_trigger_type.go:23` (proc pointer set = All)
  - `pkg/pack/compiler/codegen/smoke_test.go::TestPipeline_FullSlice` (identical body shape produces clean codegen + zero diagnostics)
  - `pkg/pack/compiler/runescript/jag_file_writer.go` (file layout: `script.dat` header is `BE32(lastID+1) + BE32(version=27)` at offsets 0–8, blob starts at offset 8)
  - `pkg/pack/compiler/runescript/js5_pack_writer.go` (packGroup gzip layout: `[type=2:1][compLen:4][origLen:4][gzipStream...]`)
  - `pkg/pack/compiler/runescript/binary_script_writer.go` (the byte-pin offsets `[0:14] fullName`, `[14:?] sourceName`, `[?+1:?+5] lookupKey`, `[?+5] debugproc` are pinned by `TestPipeline_FullSliceWithWriter`)

## 9. Test strategy

Single TDD slice. Red: rewrite the two test functions with the fixture + assertions; they fail (existing fixture path is `[proc,hello]\n`, so the file paths and pin offsets don't match). Green: change source-file path / fixture content / writer config to produce real output.

Run: `go test ./pkg/pack/compiler/runescript/... -run TestCompile_` (full driver test set; ~7 tests).

Race: `go test -race ./pkg/pack/compiler/runescript/...` — no concurrency added in this work; race run is for regression-net only.

## 10. Deviations from TS

None. There is no canonical TS test fixture to mirror: `RuneScriptTS/test/` does not exist, and Engine-TS's compiler has no TS-side test files. The fixture is goscape-original and matches what the goscape-side codegen smoke already validates.
