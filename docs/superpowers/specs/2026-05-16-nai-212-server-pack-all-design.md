# NAI-212 — Server-side `PackAll` orchestration + `runServerCompiler` glue

**Date:** 2026-05-16
**Tech stack:** Go 1.26+ per `[[go_version]]`
**Cadence:** Standard per `[[runescript_cadence]]` — not compressed (~300 LOC across 3 packages, cross-package smoke).
**Execution mode:** `subagent-driven-development` per `[[execution_mode_default]]`.
**TS canonical source:** `$HOME/Code/github.com/LostCityRS/Engine-TS` per `[[ts_source_canonical_path]]`.

---

## §1. Scope

This slice ports TS `packAll()` (`tools/pack/PackAll.ts`) restricted to its **server-applicable** steps, and the final `CompileServerScript({ symbols })` invocation of `runServerCompiler` (`tools/pack/Compiler.ts:329-365`).

Three layers, smallest-to-largest:

1. **`compiler.ToCompilerTypeInfo`** — bridge from `compiler.TypeInfo` (NAI-200 dual-map shape) → `runescript.CompilerTypeInfo` (NAI-210 single-map shape). Pure conversion.
2. **`compiler.RunServerCompiler(srcDir, outDir, dataPackDir)`** — chain `BuildSymbols` → per-entry bridge → `runescript.Compile()`. The goscape equivalent of TS `runServerCompiler()` final step.
3. **`pack.PackAll(srcDir, outDir, dataPackDir)`** — orchestrator: `ClearFsCache()` → `PackConfigs(srcDir, outDir)` → `compiler.RunServerCompiler(...)`. The goscape equivalent of TS `packAll()`, restricted to server-applicable steps.

**Out of scope:**
- Client-side packers (`packClientInterface/Title/Media/Texture/Wordenc/Sound/Graphics/Midi`) — no goscape implementation exists. One umbrella deviation tag.
- `packMaps` — likewise no goscape implementation. Same umbrella tag.
- `revalidatePack()` standalone — `PackConfigs()` already constructs+saves every `PackFile` it touches, so a standalone revalidate is a no-op in goscape. Deviation tag documents the divergence.
- CLI wiring (`cmd/goscape` integration) — leave for a follow-up slice; `PackAll` lands as a library entry point only.
- Macros (`NAI-211-D-MACRO-LOOKUP-DEFERRED` stays live).

---

## §2. TS source mapping

| TS construct | Lines | goscape destination |
|---|---|---|
| `packAll()` | `tools/pack/PackAll.ts:17-52` | `pkg/pack/pack_all.go:PackAll` |
| `clearFsCache()` | called in packAll | `pkg/pack/fscache.go:ClearFsCache` (already exists) |
| `revalidatePack()` | `tools/pack/PackFile.ts:220-247` | (no-op in goscape; deviation tag — see §8) |
| `packConfigs()` | called in packAll | `pkg/pack/pack_configs.go:PackConfigs(srcDir, outDir)` (already exists) |
| `runServerCompiler()` data-collection | `tools/pack/Compiler.ts:109-329` | `pkg/pack/compiler/symbols.go:BuildSymbols` (already exists, NAI-202) |
| `CompileServerScript({ symbols })` | `tools/pack/Compiler.ts:330-365` | `pkg/pack/compiler/run_server_compiler.go:RunServerCompiler` (NEW) |
| `runescript.Compile(cfg)` driver | TS `@lostcityrs/runescript` | `pkg/pack/compiler/runescript/compile.go:Compile` (already exists, NAI-210) |

---

## §3. What already exists

- `pkg/pack/fscache.go:ClearFsCache()` (no-arg, idempotent).
- `pkg/pack/pack_configs.go:PackConfigs(srcDir, outDir) error` — drives all 18 config packers; writes `<outDir>/server/<type>.{dat,idx}` and one `<outDir>/client/config` jagfile. Internally constructs+saves all 22 namemap `*.pack` files via `pkg/pack/PackFile.Save`.
- `pkg/pack/compiler/symbols.go:BuildSymbols(srcDir, dataPackDir) (map[string]*TypeInfo, error)` — assembles the 32-key symbol dict (commands w/ pointers, constants, 22 .pack loads + 4 synth/enrich passes for writeinv/interface/overlay/dbcolumn/varp/varn/vars/param, 3 meta-maps, 2 static arrays).
- `pkg/pack/compiler/runescript/compile.go:Compile(cfg Config) error` — full pipeline driver. Requires `cfg.Symbols["command"]` + `cfg.Symbols["runescript"]`.

Missing: type bridge, `RunServerCompiler`, `PackAll`.

---

## §4. Type bridge — `compiler.ToCompilerTypeInfo`

**Signature:**

```go
// In package compiler (NEW file pkg/pack/compiler/bridge.go):
func ToCompilerTypeInfo(src *TypeInfo) *runescript.CompilerTypeInfo
```

**Shape divergence:**

| Field | `compiler.TypeInfo` (src) | `runescript.CompilerTypeInfo` (dst) |
|---|---|---|
| `Max` | `int`, `-1` = empty | `int` |
| `Map` | `map[int]string` | `map[string]string` (numeric ids stringified) |
| `NameMap` | `map[string]string` (string-keyed entries from `LoadRecords`/`LoadMap`) | merged into `Map` |
| `VarType` | `map[int]string` | `Vartype map[string]string` (note destination field is `Vartype`, not `VarType` — confirmed at `compiler_type_info.go:19`) |
| `Protect` | `map[int]bool` | `Protect map[string]bool` |
| `Require`/`Require2`/`Set`/`Set2`/`Corrupt`/`Corrupt2` | `map[int]string` | `map[string]string` |
| `Conditional` | `map[int]bool` | `map[string]bool` |

**Conversion rules:**

1. Allocate destination with the same map-set; size hints from source map lengths.
2. For each int-keyed map: copy entry `int(k) → strconv.Itoa(k)` value-preserving.
3. **NameMap merge into Map**: `compiler.TypeInfo.NameMap` exists because Go can't mix int + string keys in a single map; in TS, both writers (`add(id, name)` and `loadRecords({key: value})`) write to the same `map: Record<string, string>` (TS string-keyed). For the bridge target, `runescript.CompilerTypeInfo.Map` is `map[string]string` — so the int-keyed entries (stringified) and the string-keyed `NameMap` entries both go into `Map`. **Collision rule**: numeric-id entries take precedence (TS Compiler.ts's `loadRecords` is only used for `constantInfo`, which has zero numeric-id entries — collision is empirically impossible, but the rule is defensive).
4. `Max` copies as-is.

**Edge cases the unit tests must pin:**

- Nil src → nil dst (or empty dst — pick one and document).
- Empty src (Max=-1, all maps empty) → dst with all initialized empty maps, Max=-1.
- Source with only NameMap entries (`constantInfo` shape) → dst Map carries the string keys.
- Source with both Map and NameMap entries (synthetic) → no collision when keys differ; precedence rule fires when they collide (pin numeric wins).

---

## §5. `compiler.RunServerCompiler`

**Signature** (NEW file `pkg/pack/compiler/run_server_compiler.go`):

```go
func RunServerCompiler(srcDir, outDir, dataPackDir string) error
```

**Body:**

```go
symbols, err := BuildSymbols(srcDir, dataPackDir)
if err != nil {
    return fmt.Errorf("RunServerCompiler: BuildSymbols: %w", err)
}
bridged := make(map[string]*runescript.CompilerTypeInfo, len(symbols))
for k, v := range symbols {
    bridged[k] = ToCompilerTypeInfo(v)
}
serverOut := filepath.Join(outDir, "server")
return runescript.Compile(runescript.Config{
    SourcePaths: []string{filepath.Join(srcDir, "scripts")},
    Symbols:     bridged,
    Writer: runescript.WriterConfig{
        Jag: &runescript.JagWriterConfig{Output: serverOut},
    },
})
```

**Notes:**

- TS `runServerCompiler` does NOT pass `sourcePaths` — `CompileServerScript` defaults to `../content/scripts` (which is the TS engine layout; see `runescript.Compile`'s default at `compile.go:57-59`). goscape **must** pass an explicit path (`<srcDir>/scripts`) because `srcDir` is parameterized, not relative-CWD. Deviation tag: see §8.
- `Features` left at zero-value (default `StrictFeatureLevel`).
- `CheckPointers` left at nil (defaults to true — TS-parity).
- `Handler` left at nil (defaults to `&diagnostics.BaseDiagnosticsHandler{}` — TS-parity).
- `ExcludePaths` left nil.
- Single Jag sink (TS default; matches existing `pkg/pack/PackConfigs` server-output convention).

---

## §6. `pack.PackAll`

**Signature** (NEW file `pkg/pack/pack_all.go`):

```go
func PackAll(srcDir, outDir, dataPackDir string) error
```

**Body:**

```go
ClearFsCache()
if err := PackConfigs(srcDir, outDir); err != nil {
    return fmt.Errorf("PackAll: PackConfigs: %w", err)
}
if err := compiler.RunServerCompiler(srcDir, outDir, dataPackDir); err != nil {
    return fmt.Errorf("PackAll: RunServerCompiler: %w", err)
}
return nil
```

**`dataPackDir` argument**: TS `runServerCompiler` hardcodes `'data/pack'`; goscape parameterizes. Most callers will pass `outDir` (i.e. the same directory `PackConfigs` just wrote into — `RunServerCompiler` reads back the .dat/.idx that PackConfigs produced). Spec leaves it explicit so callers can point at a pre-built cache without re-packing.

**Other TS-packAll steps not wired in this slice:**

- `parentPort.postMessage(...)` progress beacons — DevThread-specific; goscape has no worker-thread message channel. Silently dropped (not a deviation; no behavioral surface).
- `revalidatePack()` standalone call — see `NAI-212-D-REVALIDATEPACK-INSIDE-PACKCONFIGS` at §8.
- Nine client-side steps (`packClientInterface/Title/Media/Texture/Wordenc/Sound/Graphics/Midi`, `packMaps`) — see `NAI-212-D-CLIENT-PACKERS-DEFERRED` at §8.

---

## §7. Testing strategy

### Type bridge (`compiler/bridge_test.go`, ~5 tests)

1. **Empty source** — `Max=-1`, all maps empty → dst Max=-1, all maps non-nil but empty.
2. **Numeric-id source** — synthetic `TypeInfo` with `Map[0]="alpha"`, `Map[42]="beta"`, `Max=42` → dst `Map["0"]="alpha"`, `Map["42"]="beta"`, `Max=42`.
3. **NameMap source** — `TypeInfo` with only `NameMap["foo"]="bar"` (mirrors `constantInfo` shape) → dst `Map["foo"]="bar"`.
4. **Auxiliary maps** — populate `VarType[5]="int"`, `Protect[5]=true`, `Require[7]="active_player"`, `Conditional[9]=true` → dst stringified keys preserved with correct values.
5. **Collision precedence** — synthetic with `Map[3]="from-int"` AND `NameMap["3"]="from-str"` → dst `Map["3"]="from-int"` (numeric wins per §4.3 rule).

### `RunServerCompiler` (`compiler/run_server_compiler_test.go`, ~3 tests)

1. **Happy path with minimal fixture** — set up a `t.TempDir()` with:
   - `<tmp>/scripts/helper.rs2` containing `[proc,helper]\nreturn;\n`
   - `<tmp>/pack/script.pack` containing `0=[proc,helper]\n`
   - `<tmp>/cache/server/` with empty .dat/.idx for each of the 7 configs `loadConfigs` reads (or use the `buildSymbolsCore` testable seam by exposing an alternate entrypoint).
   - Invoke `RunServerCompiler(tmp, tmp, cacheDir)`.
   - Assert: `<tmp>/server/script.dat` + `<tmp>/server/script.idx` exist and have non-zero size.

   **Risk**: `BuildSymbols` requires loading 7 cache types (`InvType`, `Component`, `VarP`, `VarN`, `VarS`, `Param`, `DbTableType`). Writing in-memory fixtures may be more work than the test is worth. Two mitigations:
   - **Mitigation A**: expose `RunServerCompilerCore(srcDir, outDir, loaders *configLoaders)` test seam (mirrors `buildSymbolsCore` precedent). Production callers use `RunServerCompiler` → `BuildSymbols` → wraps via the existing seam.
   - **Mitigation B**: skip the test if the cache fixtures are too heavy; rely on `PackAll` end-to-end smoke as the only integration pin.

   **Plan-author decision** (resolved at plan time): pick A. Mirrors existing precedent + keeps the integration smoke focused.

2. **Symbol-validation error propagates** — pass `srcDir` with missing `scripts/` → `BuildSymbols` constants step returns error → `RunServerCompiler` wraps and propagates.

3. **Compile error propagates** — pass a fixture whose `script.pack` is empty → `Compile` returns the "core symbols missing" guard rejection (or an analogous failure). Pin the wrapper message contains "RunServerCompiler".

### `PackAll` (`pack/pack_all_test.go`, ~2 tests)

1. **Three-stage smoke** — fixture with one .obj source + one .rs2 script. After `PackAll`, assert:
   - `<outDir>/server/obj.dat` exists (PackConfigs ran).
   - `<outDir>/server/script.dat` exists (RunServerCompiler ran).
   - `<outDir>/client/config` jagfile exists (PackConfigs ran).
2. **Error propagation** — pass invalid `srcDir`; assert error wraps the stage name.

### Deviation pins (`pack/nai212_deviation_pins_test.go`, ~3 tests)

One test per live tag.

---

## §8. Live deviation tags (3 expected at close)

1. **`NAI-212-D-CLIENT-PACKERS-DEFERRED`** — `PackAll` omits 9 TS packAll steps (`packClientInterface/Title/Media/Texture/Wordenc/Sound/Graphics/Midi`, `packMaps`). Goscape has no implementations. Retires when the client-pack arc lands.
2. **`NAI-212-D-REVALIDATEPACK-INSIDE-PACKCONFIGS`** — TS `packAll` calls `revalidatePack()` standalone before `packConfigs()`. goscape's `PackConfigs` constructs+saves every `PackFile` it touches internally, making a standalone revalidate a no-op. Tag documents the structural divergence. Permanent (no retirement plan unless PackConfigs is refactored to decouple namemap refresh from packing).
3. **`NAI-212-D-EXPLICIT-SOURCEPATHS`** — `RunServerCompiler` passes explicit `cfg.SourcePaths = [<srcDir>/scripts]`; TS `CompileServerScript` accepts the default `../content/scripts`. Required because `srcDir` is a parameter, not a fixed CWD-relative path. Permanent.

---

## §9. File inventory

**Created (5):**

- `pkg/pack/compiler/bridge.go` + `bridge_test.go`
- `pkg/pack/compiler/run_server_compiler.go` + `run_server_compiler_test.go`
- `pkg/pack/pack_all.go` + `pack_all_test.go`
- `pkg/pack/nai212_deviation_pins_test.go`

**Modified:** None expected. If the bridge needs a `runescript` package import that creates a cycle, fall back to placing `ToCompilerTypeInfo` in a new `pkg/pack/compiler/bridge/` subpackage (verify at plan-author time via `go list -deps`).

---

## §10. Sequencing (task outline — formal in plan doc)

1. **T1** — `ToCompilerTypeInfo` bridge + 5 unit tests. **Red → green → close.**
2. **T2** — `RunServerCompiler` skeleton + `RunServerCompilerCore` test seam + 3 tests. **Red → green → close.**
3. **T3** — `PackAll` orchestrator + 2 integration tests. **Red → green → close.**
4. **T4** — 3 deviation pins.
5. **T5** — Combined-review (Sonnet reviewer, full diff `T0..T4`).
6. **Close commit** with `Closes memory:` trailer per `[[close_commit_memory_trailer]]`.

Combined LOC budget: ~300 production + ~250 test.

---

## §11. Risks + premises (pre-flight verified)

- ✅ `compiler.TypeInfo` ≠ `runescript.CompilerTypeInfo` (shape divergence is real — verified at `pkg/pack/compiler/typeinfo.go:40` and `pkg/pack/compiler/runescript/compiler_type_info.go:12`).
- ✅ `BuildSymbols` returns `map[string]*compiler.TypeInfo` (verified at `pkg/pack/compiler/symbols.go:444`).
- ✅ `runescript.Compile(cfg Config)` requires `cfg.Symbols["command"] + cfg.Symbols["runescript"]` (verified at `pkg/pack/compiler/runescript/compile.go:52`).
- ✅ `PackConfigs(srcDir, outDir)` exists and writes both server and client outputs (verified at `pkg/pack/pack_configs.go:64`).
- ✅ `ClearFsCache()` exists, no-arg (verified at `pkg/pack/fscache.go:27`).
- ⚠️ Cycle risk: `pkg/pack/compiler/` already imports `pkg/objtype`. Adding a `pkg/pack/compiler/runescript` import to a new `compiler/bridge.go` is one-directional (the bridge is upstream of `runescript`; nothing in `runescript/` imports `compiler/`). Confirm at plan-author time with `go list -deps ./pkg/pack/compiler/runescript`. If a cycle does surface, fallback is `pkg/pack/compiler/bridge/bridge.go` subpackage.
- ⚠️ Field-name confirmation (`Vartype` vs `VarType` on `runescript.CompilerTypeInfo`): spec §4 table reads `Vartype` from the existing file at `compiler_type_info.go:19`. Verified.

---

## §12. Follow-ups already scoped out (not addressed in this slice)

- **CLI wiring**: a `cmd/goscape --target pack` mode (or separate `cmd/gscpack`) that invokes `pack.PackAll`. Saves to NAI-213+.
- **Client-pack arc**: porting `packClient*` (8 packers + `packMaps`). Each is a multi-NAI arc on its own; out of scope.
- **`::rebuild` cheat wiring**: hooking the `modules/world/handlers_game.go:handleClientCheat` "rebuild" path to invoke `pack.PackAll` in-process. Saves to NAI-214+.
- **Macros**: `NAI-211-D-MACRO-LOOKUP-DEFERRED` stays live.
