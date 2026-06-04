# NAI-197: `.seq` + `.flo` + `.spotanim` + `.idk` packer slice

**Date**: 2026-05-14
**Predecessor**: NAI-196 (`.loc`/`.obj`/`.npc` packer slice + TS-canonical ordering rewrite; closed at `542950a`)
**Cohort identity**: Second client+server config family — four "client-bound" branches that TS gates on `rebuildClient = true` and that mirror NAI-196's unconditional-client-pack pattern. Uniform shape across all four: per-id walk of the type-specific `PackFile`, client opcodes per TS, server contains 250-trailer + `pjstr(debugname)` for three of four (`.flo` server is empty per-id). No new dispatch patterns.

## 1. Goal

Port the next four per-config packer branches from TS `tools/pack/config/PackShared.ts`:

- `.seq` (`PackShared.ts:454-475`)
- `.flo` (`PackShared.ts:500-521`)
- `.spotanim` (`PackShared.ts:523-544`)
- `.idk` (`PackShared.ts:592-613`)

After this slice, a goscape pack run produces four additional `<serverOut>/<type>.{dat,idx}` pairs (`seq`, `flo`, `spotanim`, `idk`) byte-identical to TS output for equivalent source inputs, and contributes their client-side counterparts to the now-14-entry client jagfile (alongside the existing 10 entries from NAI-196). All four branches run unconditionally per the `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` pattern.

## 2. Scope

**In**:

- Parsers + packers for `.seq`, `.flo`, `.spotanim`, `.idk`
- Four new lazy `*PackFile` registry helpers (`ensureAnimPack`, `ensureFloPack`, `ensureSpotAnimPack`, `ensureIdkPack`); existing `ensureSeqPack`, `ensureModelPack`, `ensureObjPack`, `ensureTexturePack` reused
- `PackConfigs` re-ordering: insert four new unconditional branches in TS-canonical positions (`.seq` between `.struct` and `.loc`; `.flo` between `.loc` and `.spotanim`; `.spotanim` between `.flo` and `.npc`; `.idk` between `.obj` and `.varp`)
- Round-trip tests for three of four configs (via existing `pkg/objtype.LoadSeqTypes`, `LoadSpotanimTypes`, `LoadIdkTypes`); `.flo` byte-pin only (no runtime `objtype.LoadFloTypes` exists — confirmed at spec-write, see §9 R3)
- 15-config integration test (extends NAI-196's 11-config `TestPackConfigs_ElevenConfigsLand`)
- Deviation-tag absence pins are not expected (no NAI-196 tag retires); presence pin re-asserts that `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` now references all 9 client+server branches (5 from NAI-196 + 4 new)

**Out**:

- `.dbtable`/`.dbrow` (server-only paired group with joint `shouldBuild` + `DbTableType.load` cache-hop; sub-spec deferred — distinct dispatch shape from this slice)
- `.hunt` (server-only tail; sub-spec deferred — 545 TS LOC isolated outlier)
- `category.pack → category.dat` writer and `frame_del.dat` writer (TS interleaves these between `.param` and `.enum`; sub-spec deferred — non-`.<ext>`-source pipelines)
- BUILD_VERIFY/CRC validator callbacks (continues `NAI-191-D-VALIDATE-FLAGS-DEFERRED`)
- Module-level pack singletons (continues `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED`)
- NAI-191 follow-ups #1 (`TrimLeft` Unicode narrowing) and #3 (`ShouldBuildFileAny` `ReadDir` failure) — not on this slice's hot paths
- Reconsideration of `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` (the four new branches inherit the same unconditional pattern without revisiting)

## 3. Tech stack

- Go 1.26+ (per `[[go_version]]` memory)
- TS source: `LostCityRS/Engine-TS` (per `[[ts_source_canonical_path]]` memory). Specifically:
  - `tools/pack/config/PackShared.ts:261-669` (full `packConfigs` body — canonical order reference)
  - `tools/pack/config/SeqConfig.ts:1-208`
  - `tools/pack/config/FloConfig.ts:1-104`
  - `tools/pack/config/SpotAnimConfig.ts:1-152`
  - `tools/pack/config/IdkConfig.ts:1-206`
  - `tools/pack/PackFile.ts` (singleton declarations — confirms each config's registry consumers)

Per `[[true_to_ts_gate]]` and the NAI-196 retrospective: this spec does NOT codify opcode tables. Each per-config task block references the TS file + line range and instructs the implementer to read TS directly. Plan-author follows the same discipline.

## 4. Architecture

### 4.1 New files (in `pkg/pack/`)

| File | Contents |
|---|---|
| `seq.go` | `parseSeqConfigFor(animPack, objPack)`, `packSeqConfigs(configs, seqPack)` (returns `server, client *PackedData, err error`) |
| `seq_test.go` | byte-pin tests for `packSeqConfigs` (per opcode branch) |
| `seq_roundtrip_test.go` | source → `PackConfigs` → `objtype.LoadSeqTypes(serverOut, &SeqFrameConfigs{})` round-trip |
| `flo.go` | `parseFloConfigFor(texturePack)`, `packFloConfigs(configs, floPack)` |
| `flo_test.go` | byte-pin tests; includes the unique `!startsWith("flo_")` opcode-6 emission gate |
| (no `flo_roundtrip_test.go`) | per §9 R3 — `pkg/objtype` exposes no `LoadFloTypes`; byte-pin coverage is the contract |
| `spotanim.go` | `parseSpotAnimConfigFor(modelPack, seqPack)`, `packSpotAnimConfigs(configs, spotanimPack)` |
| `spotanim_test.go` | byte-pin tests |
| `spotanim_roundtrip_test.go` | round-trip via `objtype.LoadSpotanimTypes(serverOut)` |
| `idk.go` | `parseIdkConfigFor(modelPack)`, `packIdkConfigs(configs, idkPack)` |
| `idk_test.go` | byte-pin tests |
| `idk_roundtrip_test.go` | round-trip via `objtype.LoadIdkTypes(serverOut)` |
| `nai197_deviation_pins_test.go` | presence pins (no retirements; one extended-scope reaffirmation) |

### 4.2 Modified file

`pkg/pack/pack_configs.go` — body extension:

- Four new lazy `ensureFooPack` helpers (`ensureAnimPack`, `ensureFloPack`, `ensureSpotAnimPack`, `ensureIdkPack`) added alongside the eight existing ones from NAI-196
- Four new `packAndSaveSeq` / `packAndSaveFlo` / `packAndSaveSpotAnim` / `packAndSaveIdk` functions following the NAI-196 `packAndSaveLoc`/`Npc`/`Obj` shape
- Four new unconditional branches inserted in TS-canonical positions (see §6)
- The `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` doc-comment on `PackConfigs` is extended in-place to enumerate all 9 client+server branches (the original 5 + the 4 new)
- The `NAI-192-D-NO-SRC-NO-OP` doc-comment scope shrinks: the six server-only freshness-gated branches remain (`.enum`, `.inv`, `.mesanim`, `.struct`, `.varn`, `.vars`). NO change to the existing scope statement other than count phrasing if needed.

### 4.3 Per-config registry consumer table

| Config | Iterated by | Resolves refs in | New helpers needed |
|---|---|---|---|
| `.seq` | `SeqPack` | `AnimPack`, `ObjPack` | `ensureAnimPack` (new); reuses `ensureSeqPack`, `ensureObjPack` |
| `.flo` | `FloPack` | `TexturePack` | `ensureFloPack` (new); reuses `ensureTexturePack` |
| `.spotanim` | `SpotAnimPack` | `ModelPack`, `SeqPack` | `ensureSpotAnimPack` (new); reuses `ensureModelPack`, `ensureSeqPack` |
| `.idk` | `IdkPack` | `ModelPack` | `ensureIdkPack` (new); reuses `ensureModelPack` |

(Verified at spec-write — see `grep TexturePack|AnimPack|ObjPack|ModelPack|SeqPack|...` against the four TS configs.)

## 5. Per-config design

For each per-config task, plan-author writes a code-block instructing the implementer to:

1. Read the cited TS file end-to-end.
2. Port the parser as `parseXxxConfigFor(<deps>)`: closure-captures the listed `*PackFile` registries; returns the per-key parser shape matching `parseLocConfigFor`/`parseObjConfigFor`/`parseNpcConfigFor` (NAI-196).
3. Port the packer as `packXxxConfigs(configs, xxxPack)` returning `(server, client *PackedData, err error)`. Where TS returns `{ server, client }`, goscape mirrors. Where TS server side is empty (`.flo`), goscape returns an empty-but-valid server `PackedData` (one `Next()` per id, zero opcode bytes — matches TS line 100 `server.next()` with no preceding `server.p<N>` calls).
4. Omit dead TS branches per `NAI-195-D-DEADBRANCH-OMITTED` (`.idk` has empty `numberKeys: []` — see TS `IdkConfig.ts:8`; verify per config).

### 5.1 `.seq`

**TS source**: `SeqConfig.ts:1-208` — parser at `:4-119`, packer at `:121-207`.

- **Iterated by**: `SeqPack` (per id in `[0, seqPack.Max())`).
- **Reference registries**: `AnimPack` (frame/iframe keys; values are model-frame indices), `ObjPack` (`replaceheldleft`/`replaceheldright` keys; emitted with `+ 512` offset per `SeqConfig.ts:96-118`).
- **Notable keys**: `loops`, `priority`, `maxloops` (numeric); `stretches` (boolean); `frame{N}`, `iframe{N}`, `delay{N}` (frame-index arrays); `replaceheldleft`/`replaceheldright` (special `'hide'` literal → 0; otherwise `ObjPack.getByName + 512`).
- **Server side**: 250-trailer + `pjstr(debugname)` only when `debugname.length` (`SeqConfig.ts:197-201`); empty otherwise.
- **Client side**: opcodes per `SeqConfig.ts:139-194` — includes frame-block emission (opcode 1 with frames/iframes/delays per-element).
- **Per-id finalize**: `client.next()` + `server.next()`.

### 5.2 `.flo`

**TS source**: `FloConfig.ts:1-104` — parser at `:4-61`, packer at `:63-104`.

- **Iterated by**: `FloPack`.
- **Reference registries**: `TexturePack` (`texture` key).
- **Notable keys**: `colour` (numeric); `overlay`, `occlude` (booleans, emitted asymmetrically: `overlay=true` emits opcode 3; `occlude=false` emits opcode 5 — see TS `:80-89`); `texture` (numeric, via `TexturePack.getByName`).
- **Server side**: EMPTY per id (`FloConfig.ts:64-65` constructs `server: PackedData` but the body has no `server.p<N>` calls; only `server.next()` at line 100). Plan-author pin: assert that a flo source with N defined ids produces a `flo.dat` of length equal to N (each id contributes exactly the boundary marker, no payload).
- **Client side**: opcodes 1, 2, 3, 5 + the debugname trailer (opcode 6) which is gated `!debugname.startsWith('flo_')` per `FloConfig.ts:91-97`. This is unique to `.flo` and warrants a dedicated byte-pin test case (per `[[ts_asymmetry_dual_pin]]` — pin both the "with prefix → no emission" and "without prefix → emission" branches).
- **Per-id finalize**: `client.next()` + `server.next()`.

### 5.3 `.spotanim`

**TS source**: `SpotAnimConfig.ts:1-152` — parser at `:4-90`, packer at `:92-151`.

- **Iterated by**: `SpotAnimPack`.
- **Reference registries**: `ModelPack` (`model` key); `SeqPack` (`anim` key).
- **Notable keys**: `model` (`ModelPack.getByName`); `anim` (`SeqPack.getByName`); `recol{N}{s,d}` (`p2`); `resizeh`, `resizev`, `angle`, `ambient`, `contrast` (numeric); `hasalpha` (boolean). Imports `ColorConversion` — recol values may flow through 24→15 bit conversion (verify against TS line-by-line; per `[[colorconv_rgb24to15_in_writer]]` memory, conversion typically lives at writer site not parser).
- **Server side**: 250-trailer + `pjstr(debugname)` only when `debugname.length`.
- **Client side**: opcodes per `SpotAnimConfig.ts:101-140`.

### 5.4 `.idk`

**TS source**: `IdkConfig.ts:1-206` — parser at `:4-119`, packer at `:121-205`.

- **Iterated by**: `IdkPack`.
- **Reference registries**: `ModelPack` (`model{N}` and `head{N}` keys).
- **Notable keys**: `model{N}` / `head{N}` (`ModelPack.getByName`); `recol{N}{s,d}` (`p2`); `disable` (boolean — only entry in `booleanKeys`); imports `ColorConversion` (same caveat as §5.3).
- **Server side**: 250-trailer + `pjstr(debugname)` only when `debugname.length`.
- **Client side**: opcodes per `IdkConfig.ts:130-194`.
- **Dead branches**: TS `numberKeys: []` (empty) — `NAI-195-D-DEADBRANCH-OMITTED` applies. `IdkConfig.ts:8` — verify before omitting in Go port.

## 6. Pipeline integration

Full new branch insertions in `PackConfigs`. Other branches unchanged from NAI-196.

```go
// (existing NAI-196 branches: .param → ParamType load → .enum → .inv → .mesanim → .struct)

// NEW: .seq — server+client jag; unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK)
// TS PackShared.ts:454-475
if err := ensureSeqPack(); err != nil { return err }
if err := ensureAnimPack(); err != nil { return err }
if err := ensureObjPack(); err != nil { return err }
if err := packAndSaveSeq(srcDir, serverOut, seqPack, animPack, objPack, constants, clientJag); err != nil {
    return err
}

// (existing NAI-196 .loc branch)

// NEW: .flo — server+client jag; unconditional
// TS PackShared.ts:500-521
if err := ensureFloPack(); err != nil { return err }
if err := ensureTexturePack(); err != nil { return err }
if err := packAndSaveFlo(srcDir, serverOut, floPack, texturePack, constants, clientJag); err != nil {
    return err
}

// NEW: .spotanim — server+client jag; unconditional
// TS PackShared.ts:523-544
if err := ensureSpotAnimPack(); err != nil { return err }
if err := ensureModelPack(); err != nil { return err }
if err := ensureSeqPack(); err != nil { return err }
if err := packAndSaveSpotAnim(srcDir, serverOut, spotanimPack, modelPack, seqPack, constants, clientJag); err != nil {
    return err
}

// (existing NAI-196 .npc branch)
// (existing NAI-196 .obj branch)

// NEW: .idk — server+client jag; unconditional
// TS PackShared.ts:592-613
if err := ensureIdkPack(); err != nil { return err }
if err := ensureModelPack(); err != nil { return err }
if err := packAndSaveIdk(srcDir, serverOut, idkPack, modelPack, constants, clientJag); err != nil {
    return err
}

// (existing NAI-196 .varp branch, then .varn / .vars freshness-gated, then clientJag.Save)
```

Resulting TS-canonical order across 15 implemented configs:
`.param` → `.enum` → `.inv` → `.mesanim` → `.struct` → **`.seq`** → `.loc` → **`.flo`** → **`.spotanim`** → `.npc` → `.obj` → **`.idk`** → `.varp` → `.varn` → `.vars`.

Plan-author note (per `[[plan_sibling_site_guard_audit]]`): each new `ensureFooPack` site reuses the lazy-init pattern from NAI-196. The four new helpers go alphabetically under the existing block in `pack_configs.go:91-201`. No call-site needs a `nil`-check guard (matches existing convention).

Plan-author note (per `[[plan_var_name_collision]]`): mentally compile each new `packAndSave*` function body. Parameter names (`seqPack`, `animPack`, etc.) shadow the outer `PackConfigs`-scoped lazy vars; this is intentional (parameter is the resolved non-nil `*PackFile`, outer var is the lazy pointer). Do NOT use `:=` for pack-file vars inside the inner function.

## 7. Deviations

### 7.1 Retired (0)

None this slice. NAI-196 retired three accumulated ordering-related tags; this slice extends the established pattern without introducing or resolving new ones.

### 7.2 Carryforward (6 — all unchanged in scope, one extended in citation count)

- `NAI-191-D-VALIDATE-FLAGS-DEFERRED` — unchanged.
- `NAI-192-D-NO-SRC-NO-OP` — unchanged. Continues to apply to the six server-only freshness-gated branches (`.enum`, `.inv`, `.mesanim`, `.struct`, `.varn`, `.vars`). Does NOT apply to the four new branches (unconditional).
- `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED` — unchanged. The four new `*PackFile` constructions follow the existing per-call pattern.
- `NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL` — unchanged. Tag is `.param`-specific; this slice does not touch `.param`.
- `NAI-195-D-DEADBRANCH-OMITTED` — applies to `.idk` parser (`numberKeys: []` per `IdkConfig.ts:8`). Verify per config in T2/T3/T4/T5 before omitting.
- `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` — **scope extended in-place**. The existing doc-comment on `PackConfigs` lists the 5 NAI-196-era branches (`.param`, `.loc`, `.npc`, `.obj`, `.varp`); this slice updates the same comment to list 9 branches (those 5 plus `.seq`, `.flo`, `.spotanim`, `.idk`). No new tag identifier introduced.

### 7.3 New (0)

No new deviation tags. All four new branches follow established patterns.

## 8. Tests

### 8.1 Byte-pin tests (one file per config — 4 files)

`seq_test.go`, `flo_test.go`, `spotanim_test.go`, `idk_test.go` — table-driven cases against `packXxxConfigs` asserting byte-exact output. Each opcode branch listed in §5 gets at least one positive case. Notable test cases:

- `seq_test.go`: frame/iframe/delay array emission; `replaceheldleft`/`replaceheldright` with `'hide'` literal vs `ObjPack.getByName + 512` (per `[[ts_asymmetry_dual_pin]]`); 250-trailer present-vs-absent pin (debugname empty vs non-empty).
- `flo_test.go`: opcode-6 emission gate (`!startsWith("flo_")` per `FloConfig.ts:92`) — both branches pinned per `[[ts_asymmetry_dual_pin]]`; empty-server-bytes invariant pin (assert `server.Dat` for an N-id flo source has length matching the `Next()`-boundary-only output).
- `spotanim_test.go`: `recol{N}{s,d}` round-trip; `hasalpha=true` boolean opcode emission vs absence; `ColorConversion` invocation site if any (verify against TS line-by-line at implementation time).
- `idk_test.go`: `model{N}` slot population; `head{N}` slot independence; `disable=true` boolean emission; verify `numberKeys: []` dead branch is omitted (or, equivalently, that no number-key code path exists in the Go parser).

Per `[[plan_runnable_test_fixtures]]`: every fixture must be mentally executable as written. Per `[[mock_recorder_field_naming_check]]`: if plan-author introduces test recorders, grep the actual mock struct names before referencing.

### 8.2 Round-trip tests (3 of 4 configs)

- `seq_roundtrip_test.go` — sources for `.seq` (no `.param` needed; `.seq` does not consume `paramTypes`); run `PackConfigs(srcDir, outDir)`; load via `objtype.LoadSeqTypes(serverOut, &SeqFrameConfigs{})`; assert 2–3 seq-type fields per config including a frame-list round-trip. **Pre-flight**: plan-author re-greps `LoadSeqTypes` signature at HEAD per `[[load_param_types_dir_arg]]` — `dir` arg is parent-of-`server/`, NOT `serverOut` itself. (Confirmed at spec-write: `pkg/objtype/seqtype.go:126`.)
- `spotanim_roundtrip_test.go` — sources for `.spotanim` + minimal `.pack` registries (model.pack, seq.pack, spotanim.pack); run `PackConfigs`; load via `objtype.LoadSpotanimTypes(serverOut)`; assert per-spotanim fields including `model` and `anim` references.
- `idk_roundtrip_test.go` — sources for `.idk` + minimal `.pack` registries (model.pack, idk.pack); run `PackConfigs`; load via `objtype.LoadIdkTypes(serverOut)`; assert per-idk fields including `model{N}` slot population.
- `.flo`: **no round-trip test.** `pkg/objtype` exposes no `LoadFloTypes` (verified at spec-write: `ls pkg/objtype/ | grep -i flo` returns nothing). Per `[[match_spec_tests_to_library_capabilities]]` (informal — see `[[spec_library_capability_match]]`), substitute byte-pin coverage of all four opcodes + the debugname gate. If a `LoadFloTypes` lands in a future sub-spec, follow-up: add `flo_roundtrip_test.go` symmetrically.

### 8.3 Integration test

Extend `pack_configs_test.go` to add `TestPackConfigs_FifteenConfigsLand`:

- Sources for all 15 implemented configs: `.varp`, `.varn`, `.vars`, `.param`, `.enum`, `.inv`, `.mesanim`, `.struct`, `.loc`, `.npc`, `.obj`, **`.seq`, `.flo`, `.spotanim`, `.idk`**
- Required `.pack` registry files for: varp, varn, vars, param, enum, inv, mesanim, struct, loc, npc, obj, model, category, hunt, texture, seq, **anim, flo, spotanim, idk**
- Assert all 15 `<serverOut>/<type>.{dat,idx}` pairs exist
- Assert client jagfile at `<clientOut>/config` contains exactly 18 entries: `param.dat/idx`, `loc.dat/idx`, `npc.dat/idx`, `obj.dat/idx`, `varp.dat/idx`, **`seq.dat/idx`, `flo.dat/idx`, `spotanim.dat/idx`, `idk.dat/idx`** (9 configs × 2 files)
- Assert config emission order via `clientJag.Write` call order (or equivalent inspection if the format preserves it). The new TS-canonical order: `param → seq → loc → flo → spotanim → npc → obj → idk → varp`.

**NAI-196 T8 carry-forward**: the existing `TestPackConfigs_ElevenConfigsLand` asserts 11 configs + 10 client jag entries with the post-NAI-196 ordering. This slice's T6 (PackConfigs wiring) MUST atomically update that test to the 15-config / 18-entry / new-ordering assertions, OR add a sibling `TestPackConfigs_FifteenConfigsLand` and delete the older. Plan-author chooses; recommendation is rename-and-extend the existing test (single source of truth, simpler regression surface — matches NAI-196 T5's atomic rewrite of NAI-195 T8's 8-config test).

### 8.4 Deviation-tag pins

`nai197_deviation_pins_test.go`:

- **Presence pin (1, extended scope)**: `rg "NAI-196-D-UNCONDITIONAL-CLIENT-PACK" pkg/` returns ≥1 hit AND the matched doc-comment lists all 9 client+server branches (regex assertion or substring check for each of the 9 type names within the doc-block).
- **Absence pins (0)**: no NAI-196-era tag retires in this slice.
- **No new tag pin**: this slice introduces no new tag identifier.

Per `[[pin_test_self_trigger_production_doc]]`: if the extended doc-comment incorporates new TS-side phrasing, rephrase using goscape's own concept names (the existing comment already says "unconditional client pack"; extending the branch list with `.seq`/`.flo`/`.spotanim`/`.idk` does not introduce new TS-identifiers — no self-trigger risk).

## 9. Risk register

| Risk | Likelihood | Mitigation | Verified at spec-write? |
|---|---|---|---|
| R1: `AnimPack` is a distinct registry from `SeqPack` (used by `.seq` for frame indices) | Low | TS `SeqConfig.ts:1` imports `AnimPack` separately; goscape needs new `ensureAnimPack` helper backing `<srcDir>/pack/anim.pack`. Verify `pack/anim.pack` convention at plan-write. | ⚠️ plan-author verifies `<srcDir>/pack/anim.pack` exists in test fixture pattern; `NewPackFile(srcDir, "anim", nil)` parallels other typed packs |
| R2: `.flo` empty server emission | Med | TS `FloConfig.ts` server side is empty per id (only `next()`). Byte-pin test must assert this; round-trip not available (no `LoadFloTypes`). Risk is implementer adding a 250-trailer "by analogy" with the other three — explicitly forbidden in plan code-block. | ⚠️ plan-author tags `.flo` task as "NO 250-TRAILER — verify against TS line 91-99" |
| R3: No `objtype.LoadFloTypes` exists | High | Confirmed at spec-write: `ls pkg/objtype/ \| grep -i flo` returns no files. Round-trip test omitted; byte-pin coverage is the contract. Plan-author flags this in §8.2 task block. | ✅ verified |
| R4: `LoadSeqTypes(dir, frames *SeqFrameConfigs)` signature takes a second arg | Med | `pkg/objtype/seqtype.go:126` confirmed. Round-trip test passes `&SeqFrameConfigs{}` (empty stub). Plan-author re-verifies at HEAD per `[[load_param_types_dir_arg]]`. | ✅ verified at spec-write; plan-author re-verifies |
| R5: `LoadSpotanimTypes` is capitalised `Spotanim` not `SpotAnim` | Low | `pkg/objtype/spotanimtype.go:93` confirmed. Round-trip test uses `LoadSpotanimTypes` not `LoadSpotAnimTypes`. | ✅ verified |
| R6: `ColorConversion` invocation site for `.spotanim` and `.idk` recol values | Med | TS `SpotAnimConfig.ts:1` and `IdkConfig.ts:1` import `ColorConversion`. Per `[[colorconv_rgb24to15_in_writer]]`, conversion typically lives at writer site. Plan-author reads TS line-by-line; if writer-side, port via `pkg/colorconv` (already exists for NAI-140 work); if parser-side, port in `parseFooConfigFor`. | ⚠️ plan-author traces each file's `ColorConversion.` call sites + ports through `pkg/colorconv` |
| R7: Empty `numberKeys` / `stringKeys` dead branches in `.idk` (and possibly others) | Med | Per `[[dead_param_from_literal_ts_port]]` + `NAI-195-D-DEADBRANCH-OMITTED`, omit empty branches in Go port. `.idk` has `numberKeys: []` confirmed; the other three need per-config verification. | ⚠️ plan-author verifies per config |
| R8: NAI-196 T6 round-trip test setup pattern reuse | Low | Existing `loc_roundtrip_test.go`, `obj_roundtrip_test.go`, `npc_roundtrip_test.go` (landed at NAI-196 T6, commit `dcd57e0`) are the templates. Plan-author cross-references at plan-write for shape (`.pack` files, `PackConfigs` invocation, loader call). | ✅ verified |
| R9: Required `.pack` registry source files for 15-config integration test | Med | Add `anim.pack`, `flo.pack`, `spotanim.pack`, `idk.pack` to the NAI-196 fixture builder's pack-registry list (the existing list already covers model/category/hunt/texture/seq/loc/npc/obj/varp/varn/vars/param/enum/inv/mesanim/struct = 16 entries; this slice adds 4 → 20 total). | ⚠️ plan-author audits NAI-196 fixture builder + adds 4 new `.pack` stubs |
| R10: `PackedData(FooPack.Max)` boundary semantics | Low | NAI-196 verified `PackFile.Max()` semantics match TS `PackFile.max` (size+1). Pattern reused unchanged. | ✅ verified |
| R11: `.spotanim` and `.flo` debugname-trailer condition asymmetry | Med | `.spotanim` and `.idk` emit 250-trailer when `debugname.length > 0` (TS uniform `if (debugname.length)`). `.flo` does NOT emit 250-trailer — its debugname is on the CLIENT side as opcode 6 (gated `!startsWith("flo_")`). Plan-author distinguishes per-config. | ⚠️ plan-author writes per-task code-block reflecting the four distinct shapes |

Per `[[risk_register_premise_grep]]`: every ⚠️ row MUST be re-verified by the plan author against HEAD before codifying affected task code blocks. Per `[[plan_geometry_premise_pretrace]]`: if any row flags math/geometry (none in this slice), pre-trace.

## 10. Out-of-scope follow-ups

Tracked for subsequent NAI sub-specs:

- **NAI-198+ "specials slice"**: `category.pack → category.dat` writer + `frame_del.dat` writer (TS PackShared.ts:341-389; non-`.<ext>`-source pipelines). Currently goscape's `PackConfigs` skips both.
- **NAI-199+ "dbtable/dbrow slice"**: paired server-only configs with joint `shouldBuild` and `DbTableType.load` cache-hop between the two packers; novel dispatch shape requiring its own design.
- **NAI-200+ "hunt slice"**: `.hunt` server-only packer (545 TS LOC isolated outlier).
- **NAI-201+ "flo runtime loader"**: add `pkg/objtype.LoadFloTypes` if/when a runtime consumer requires it, then extend the round-trip suite with `flo_roundtrip_test.go` (per §8.2 deferral).
- **Long-tail**: retire `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED` once all per-config packers exist (the per-call `NewPackFile` cost will be amortised across 15+ registries); retire `NAI-191-D-VALIDATE-FLAGS-DEFERRED` once a BUILD_VERIFY-equivalent surface lands.

## 11. References

- `[[runescript_cadence]]` — applied: this spec is brainstorm → spec; plan + subagent-driven TDD follow.
- `[[true_to_ts_gate]]` — applied: this spec does NOT codify opcode tables; per-config task blocks instruct implementers to read TS directly.
- `[[ts_source_canonical_path]]` — applied: TS references all point at `LostCityRS/Engine-TS`.
- `[[pin_test_self_trigger_production_doc]]` — applies to §8.4: extended-scope doc-comment uses existing concept name "unconditional client pack" (no TS-identifier self-trigger).
- `[[plan_runnable_test_fixtures]]` — applies to §8.1/§8.2/§8.3 fixtures.
- `[[risk_register_premise_grep]]` — applies to §9 ⚠️ rows.
- `[[dead_param_from_literal_ts_port]]` — applies to §5 dead-branch omissions per-config.
- `[[ts_asymmetry_dual_pin]]` — applies to §8.1 `.flo` opcode-6 gate and `.seq` `replaceheldright='hide'` literal pins.
- `[[load_param_types_dir_arg]]` — applies to §8.2: loader `dir` arg is parent-of-`server/`.
- `[[colorconv_rgb24to15_in_writer]]` — applies to §9 R6: trace `ColorConversion` call sites for `.spotanim`/`.idk`; route through `pkg/colorconv`.
- `[[plan_sibling_site_guard_audit]]` — applies to §6: re-grep sibling `ensureFooPack` sites for shared guards before adding 4 new helpers.
- `[[plan_var_name_collision]]` — applies to §6: mentally compile each new `packAndSave*` body to avoid `:=` parameter-shadow bugs.
- `[[plan_doc_replaceall_timeline]]` — applies to §7.2: doc-comment scope-extension uses per-instance Edits, not `replace_all` (the comment evolves across NAI-196 → 197 → future slices).
- `[[mock_recorder_field_naming_check]]` — applies if plan-author introduces test recorders for §8.1.
- `[[spec_library_capability_match]]` — applies to §8.2: no `.flo` round-trip since no `LoadFloTypes` exists.
- `[[file_scoped_audits_miss_cross_file_ts]]` — applies to NAI-196 T8 integration-test carryforward in §8.3.
- `[[close_commit_memory_trailer]]` — applies at NAI-197 close commit.

## 12. Task-count estimate

Mirroring NAI-196's 8-task / ~13-commit shape:

| Task | Scope | Est. commits |
|---|---|---|
| T1 | Add 4 `ensureFooPack` helpers (`ensureAnimPack`, `ensureFloPack`, `ensureSpotAnimPack`, `ensureIdkPack`) in `pack_configs.go`; no per-config branch wiring yet | 1 |
| T2 | `.seq` parser + packer + byte-pin tests | 1–2 |
| T3 | `.flo` parser + packer + byte-pin tests (incl. empty-server-bytes pin + opcode-6 dual-asymmetry pin) | 1–2 |
| T4 | `.spotanim` parser + packer + byte-pin tests | 1–2 |
| T5 | `.idk` parser + packer + byte-pin tests | 1–2 |
| T6 | `PackConfigs` wiring: insert 4 new branches in TS-canonical order; extend `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` doc-comment scope; atomically update `TestPackConfigs_ElevenConfigsLand` → `TestPackConfigs_FifteenConfigsLand` | 1 |
| T7 | Round-trip tests for `.seq`, `.spotanim`, `.idk` (3 files; `.flo` excluded per §8.2) | 1–2 |
| T8 | Deviation-tag pins (`nai197_deviation_pins_test.go`); cleanup if any audit finds drift | 1 |

**Total**: 8 tasks, ~9–13 commits, ~670 LOC TS ported (vs NAI-196's 1389 LOC / 13 commits — comparable density at lower absolute size, reflecting that the four client-bound configs are simpler than `.loc`/`.npc`/`.obj`).
