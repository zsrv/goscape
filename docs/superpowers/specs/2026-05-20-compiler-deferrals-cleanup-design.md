# Compiler deferrals cleanup — NAI-208 logProcRequirement port + NAI-212 fallback recharacterization

**Date:** 2026-05-20
**Predecessor:** drop unused log field cleanup (HEAD `8940d3af`)
**Retires:** `NAI-208-D-LOGPROCREQ-DEFERRED` (recursive HINT chain port). Removes phantom `NAI-212-D-CLIENT-PACKERS-DEFERRED` reference from `symbols.go` + `symbols_test.go`.
**Opens:** none.

## 1. Goal

Close two `*-DEFERRED` items surfaced by the post-runtime-fixups deviation audit. After this slice:

- **Item A (NAI-208):** `PointerChecker.validatePointer` emits the full recursive `POINTER_REQUIRED_LOC` HINT chain, walking down Gosub/Jump targets to mark every call boundary where the missing pointer is required. Mirrors `RuneScriptTS/src/compiler/codegen/script/config/PointerChecker.ts:243-301`.
- **Item B (NAI-212):** The `populateInterfaceOverlay` doc comment is refreshed: the phantom reference to `pack_all.go NAI-212-D-CLIENT-PACKERS-DEFERRED` is removed (no such file exists; the actual `clientinterface.Pack` is wired into `packall.PackAll` at line 51), and the fallback is recharacterized as **permanent** (defensive coverage for standalone `cmd compile` runs and stale/missing caches). The sibling tag `NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO` stays in place as the canonical permanent marker.

## 2. Scope

### In scope

- New `(*PointerChecker).logProcRequirement` method porting TS L243-301.
- Single call site at the end of `validatePointer` replacing the deferral comment.
- Doc-comment refresh in `pkg/pack/compiler/symbols.go` (the `populateInterfaceOverlay` block at L586-601).
- Doc-comment refresh in `pkg/pack/compiler/symbols_test.go:451-455` (the `TestPopulateInterfaceOverlay_NilConfig_FallsBack` comment block that repeats the stale "deferred"/"lands" framing).
- Tests as enumerated in §5.

### Out of scope

- **NAI-211-D-MACRO-LOOKUP-DEFERRED** — gated on RuneScript macros not yet being ported. Will be addressed in a future session via the macro-port slice.
- Refactoring `requiresPointerPathScript` or `staticLabelArgsByCall` — both already exist and are used elsewhere in `pointer_checker.go`. We compose them, we don't change them.
- Adding a NEW deviation tag for the standalone-compile fallback. `NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO` already covers it; we just clarify *why* it's permanent in the doc comment.

## 3. Architecture

### 3.1 Item A — port `logProcRequirement`

#### TS reference

`RuneScriptTS/src/compiler/codegen/script/config/PointerChecker.ts:243-301`:

```ts
const logProcRequirement = (node: InstructionNode): void => {
    const inst = node.instruction;
    const opcode = inst?.opcode;
    if (opcode !== Opcode.Gosub && opcode !== Opcode.Jump) {
        return;
    }

    const symbol = inst!.operand as ScriptSymbol;
    const calledScript = this.scriptsBySymbol.get(symbol) ?? this.scripts.find(s => s.symbol === symbol);
    if (!calledScript) {
        throw new Error('Unable to find script.');
    }

    const scriptPath = this.requiresPointerPathScript(calledScript, pointer);
    if (!scriptPath) {
        const staticArgs = analysis.staticLabelArgsByCall.get(inst!);
        if (!staticArgs) {
            return;
        }

        const jumpParamNodes = this.getJumpParamNodes(calledScript);
        for (const [paramIndex, labelSymbol] of staticArgs.entries()) {
            if (!this.getPointers(labelSymbol).required.has(pointer)) {
                continue;
            }

            const nodes = jumpParamNodes.get(paramIndex);
            if (!nodes || nodes.length === 0) {
                continue;
            }

            if (!this.requiresPointerAtNodes(calledScript, pointer, nodes)) {
                continue;
            }

            const requiredNode = nodes[0];
            // ... emit HINT POINTER_REQUIRED_LOC at requiredNode.instruction.source
        }

        return;
    }

    const requiredNode = scriptPath[0];
    // ... emit HINT POINTER_REQUIRED_LOC at requiredNode.instruction.source
    logProcRequirement(requiredNode);
};

logProcRequirement(errorNode);
```

The closure captures `pointer`, `analysis`, and `this.diagnostics`. It recurses **across script boundaries**: after emitting a HINT for the script-path case, it descends into the called script's CFG.

#### Goscape shape

Add a private method on `*PointerChecker`:

```go
// logProcRequirement walks the call graph from node downward, emitting
// MessagePointerRequiredLoc HINT diagnostics at every Gosub/Jump boundary
// where pt is required. Recurses into called scripts when the call site
// directly requires pt; falls back to static-label-arg inspection when
// it doesn't. Mirrors TS PointerChecker.logProcRequirement
// (RuneScriptTS PointerChecker.ts:243-301).
func (p *PointerChecker) logProcRequirement(
    node *InstructionNode,
    pt *pointer.PointerType,
    analysis *scriptPointerAnalysis,
) {
    inst := node.Instruction
    if inst == nil {
        return
    }
    if inst.Opcode != codegen.Gosub && inst.Opcode != codegen.Jump {
        return
    }

    sym, ok := inst.Operand.(symbol.Symbol)
    if !ok {
        return
    }
    calledScript, ok := p.scriptsBySymbol[sym]
    if !ok {
        // TS throws here; goscape prefers a silent return because
        // synthetic test fixtures and partially-loaded compile contexts
        // can plausibly hit it. The earlier error diagnostic already
        // surfaces the user-visible failure.
        return
    }

    scriptPath := p.requiresPointerPathScript(calledScript, pt)
    if scriptPath == nil {
        // Fallback: static-label-arg path
        staticArgs, ok := analysis.staticLabelArgsByCall[inst]
        if !ok {
            return
        }
        jumpParamNodes := p.getJumpParamNodes(calledScript)
        for paramIndex, labelSym := range staticArgs {
            if !p.GetPointers(labelSym).Required.Has(pt) {
                continue
            }
            nodes := jumpParamNodes[paramIndex]
            if len(nodes) == 0 {
                continue
            }
            if !p.requiresPointerAtNodes(calledScript, pt, nodes) {
                continue
            }
            required := nodes[0]
            if required.Instruction == nil {
                continue
            }
            p.diagnostics.Report(diagnostics.NewDiagnostic(
                required.Instruction.Source,
                diagnostics.DiagnosticHint,
                diagnostics.MessagePointerRequiredLoc,
                pt.Representation,
            ))
        }
        return
    }

    required := scriptPath[0]
    if required.Instruction == nil {
        return
    }
    p.diagnostics.Report(diagnostics.NewDiagnostic(
        required.Instruction.Source,
        diagnostics.DiagnosticHint,
        diagnostics.MessagePointerRequiredLoc,
        pt.Representation,
    ))
    p.logProcRequirement(required, pt, analysis)
}
```

Call site at end of `validatePointer` (replaces L180-185 deferral block):

```go
p.logProcRequirement(errorNode, pt, analysis)
```

#### Deviations from TS

| TS behavior | Goscape behavior | Rationale |
|---|---|---|
| Throws `Error('Unable to find script.')` on lookup miss | Silent return | Synthetic test contexts may hit it; earlier error diagnostic already surfaces failure. No new deviation tag — consistent with goscape's existing "no-panic in compiler path" posture (see `NAI-211-D-NO-PROCESS-EXIT`). |
| Casts `inst!.operand as ScriptSymbol` (TS structural typing) | Type-asserts `inst.Operand.(symbol.Symbol)` and silently returns on failure | Go's type system requires explicit assertion; defensive fallthrough matches the lookup-miss posture. |
| Throws `Error('Unknown instruction source.')` / `Error('Invalid instruction/source.')` on `inst.source == null` | Silent skip via `if required.Instruction == nil { continue }` and outer `if inst == nil` | Same no-panic rationale. |

These three are paraphrased adjustments, not deviations worth tagging — they all reduce to the same posture documented at `NAI-209-D-PUSHLONG-PANIC` (panic→error), `NAI-209-D-LONGMATH-PANIC` etc. for runtime safety.

### 3.2 Item B — recharacterize NAI-212 fallback

#### Current comment (`symbols.go:594-601`)

```go
// NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO: with packClientInterface
// deferred (see pack_all.go NAI-212-D-CLIENT-PACKERS-DEFERRED), the cache
// is empty when compile runs and Component.get-equivalent has no data.
// TS would also skip in that state, but TS never reaches it because its
// packAll runs packClientInterface first. Goscape falls back to populating
// interfaceInfo from componentInfo.Map alone (base names, no ComName
// override, no overlay flag) so `interface`-typed identifier lookups
// resolve to the right BasicSymbol. Retires when packClientInterface lands.
```

**Problems:**
1. References nonexistent file `pack_all.go` (actual file is `pkg/packall/packall.go`).
2. References phantom tag `NAI-212-D-CLIENT-PACKERS-DEFERRED` (no such tag exists anywhere else in source).
3. Claims "Retires when packClientInterface lands" — but `packClientInterface` HAS landed at `pkg/pack/clientinterface/pack.go:31` and is wired into `pkg/packall/packall.go:51` *before* `compiler.RunServerCompiler` at L54.

#### Replacement comment

```go
// NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO: defensive fallback
// when loaders.comp is empty or missing entries. When compile runs
// inside packall.PackAll, packall calls clientinterface.Pack first
// (packall/packall.go:51) so loaders.comp is populated and the
// fallback is dormant. Standalone callers — primarily
// `goscape-cli compile` via compiler.LoadCompilerSymbols
// (cmd_compile.go:91) — do NOT run clientinterface.Pack first, and
// LoadComponentTypes returns empty configs when the client/interface
// jagfile is missing (componenttype.go:133-134). In that state the
// fallback populates interfaceInfo from componentInfo.Map alone (base
// names, no ComName override, no overlay flag) so `interface`-typed
// identifier lookups still resolve to a BasicSymbol. Permanent —
// removing the fallback would break `goscape-cli compile` on a
// fresh dataPackDir.
```

#### Test-file comment (`symbols_test.go:451-455`)

Current:
```go
// TestPopulateInterfaceOverlay_NilConfig_FallsBack pins that when the
// cache is empty (packClientInterface deferred), populateInterfaceOverlay
// still populates interfaceInfo from componentInfo.Map alone using the
// base name. See NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO in
// symbols.go. Retires alongside the deferral once packAll runs
// packClientInterface first.
```

Replacement:
```go
// TestPopulateInterfaceOverlay_NilConfig_FallsBack pins the defensive
// fallback path: when loaders.comp is empty (standalone `goscape-cli
// compile` against a fresh dataPackDir), populateInterfaceOverlay
// populates interfaceInfo from componentInfo.Map alone using the base
// name. Permanent pin for NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO.
```

## 4. Files touched

| File | Change |
|---|---|
| `pkg/pack/compiler/cfg/pointer_checker.go` | Add `(*PointerChecker).logProcRequirement` method (~55 LOC). Replace the L180-185 deferral comment with a single `p.logProcRequirement(errorNode, pt, analysis)` call site. Retire `NAI-208-D-LOGPROCREQ-DEFERRED` from the deferral comment. |
| `pkg/pack/compiler/cfg/pointer_checker_validation_test.go` | 3 new tests for the recursive HINT chain (see §5.1). |
| `pkg/pack/compiler/cfg/nai208_deviation_pins_test.go` | Remove `NAI-208-D-LOGPROCREQ-DEFERRED` from the pins-tag list at L134. |
| `pkg/pack/compiler/symbols.go` | Replace L594-601 doc comment per §3.2. |
| `pkg/pack/compiler/symbols_test.go` | Replace L451-455 doc comment per §3.2. |

Net: 2 production files modified, 3 test files modified. No new files. ~70 LOC added / ~15 LOC removed.

## 5. Testing

### 5.1 Item A tests (3 new in `pointer_checker_validation_test.go`)

Follow the existing `TestPointerChecker_Run_*` pattern (single error + optional single hint). Extend the test fixtures with caller→callee scripts.

1. **`TestPointerChecker_Run_LogProcRequirement_DirectProcChain`** — script `caller` does `~callee()` (Gosub) where `callee` requires pointer P that `caller` doesn't set. Assert: 1 ERROR at caller's gosub site (existing behavior) + 1 NEW HINT at `callee`'s P-requiring instruction with `MessagePointerRequiredLoc`.
2. **`TestPointerChecker_Run_LogProcRequirement_RecursesAcrossTwoHops`** — chain `caller` → `mid` → `leaf` where only `leaf` requires P. Assert: 1 ERROR + 2 HINTs (one at `mid`'s gosub-to-leaf, one at `leaf`'s P-requiring instruction).
3. **`TestPointerChecker_Run_LogProcRequirement_StaticLabelArgFallback`** — `caller` passes a label as static arg to `callee`; the label requires P. `callee` itself does not require P directly. Assert: 1 ERROR + 1 HINT at the jump-param node inside `callee`.

Negative coverage (no new test — covered by existing failing-path tests being green):
- Gosub to script that doesn't require P at all → no HINT emitted (validates the `scriptPath == nil` && no static-label fallback path).
- `validatePointer` cases that don't reach the error-emission block → `logProcRequirement` never invoked (path returns at L146 `if path == nil`).

### 5.2 Item B — no new tests

The existing `TestPopulateInterfaceOverlay_NilConfig_FallsBack` already pins the fallback; we only update its doc comment. The buildverify pin tests (`nai_213_buildverify_pins_test.go`) are unaffected.

### 5.3 Gate checklist (close-time)

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` — 56+ pkgs / 0 FAIL.
- `smoke-pack` 12 OK / 0 ERR / 0 SKIP.
- `grep -rn "NAI-208-D-LOGPROCREQ-DEFERRED" --include="*.go" .` — empty.
- `grep -rn "NAI-212-D-CLIENT-PACKERS-DEFERRED" --include="*.go" .` — empty.
- `grep -rn "pack_all.go" --include="*.go" .` — empty (or only legitimate non-deviation references).
- All 3 new tests pass.

## 6. Deviation tag accounting

### Retired

| Tag | Sites |
|---|---|
| `NAI-208-D-LOGPROCREQ-DEFERRED` | `pointer_checker.go` (1 comment block), `nai208_deviation_pins_test.go` (1 pin entry). |

### Phantom-references removed (not retirements — these never existed as canonical tags)

| String | Sites |
|---|---|
| `NAI-212-D-CLIENT-PACKERS-DEFERRED` | `symbols.go` (1 reference), `symbols_test.go` (1 reference). |
| `pack_all.go` | `symbols.go` (1 reference — wrong filename). |

### Opened

None.

### Unchanged but noted

- `NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO` — stays as the canonical permanent marker. Comment refreshed; tag identity preserved.
- `NAI-211-D-MACRO-LOOKUP-DEFERRED` — explicitly out of scope; deferred to a future macro-port slice.

## 7. Risks

| Risk | Mitigation |
|---|---|
| Recursive `logProcRequirement` infinite-loops on a cyclic call graph (script A gosubs B, B gosubs A) | Each recursion strictly descends to a NEW script via `scriptsBySymbol` lookup. Cycles in the call graph would cause repeated HINTs on a finite cycle, eventually overflowing the stack. **Mitigation:** add a visited-script set to the helper, scoped to one `validatePointer` invocation. Defensive; TS doesn't have this but TS scripts in practice don't recurse this way. |
| `staticLabelArgsByCall` map iteration order non-determinism causes test flakiness | The TS code iterates `staticArgs.entries()` (insertion-ordered in TS). Go maps are unordered. Sort the keys (paramIndex int) before iterating to keep HINT-emission order deterministic. Already a known goscape pattern (see `NAI-210-D-LOADER-SORTED-ITERATION`). |
| Test fixtures for caller/callee scripts need new helper boilerplate | Reuse `compileForTest` / `runescript.RuneScript` builders from `codegen_test.go` and existing `pointer_checker_validation_test.go` setup. |
| New HINTs leak into smoke-pack output and cause noise | HINTs are emitted only when there is already an ERROR — so any pipeline that's clean stays clean. Smoke-pack content (LostCityRS) is known-good per `[[friends-server-slice4a-close]]`. |
| Item B's comment refresh accidentally retires a tag that's still load-bearing | `NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO` is kept verbatim as the deviation tag; only the surrounding prose changes. Pin test `TestPopulateInterfaceOverlay_NilConfig_FallsBack` keeps its tag reference. |

## 8. Close-time deliverables

1. All §5.3 gates green.
2. New memory file `compiler_deferrals_cleanup_close.md` summarizing: items shipped, LOC, tags retired, commit range, gate evidence.
3. New entry near MEMORY.md top: `- [Compiler deferrals cleanup close](compiler_deferrals_cleanup_close.md) — NAI-208 logProcRequirement port + NAI-212 fallback recharacterization`.
4. Final commit `chore(close): compiler-deferrals-cleanup — NAI-208 logProcRequirement + NAI-212 doc refresh`.
