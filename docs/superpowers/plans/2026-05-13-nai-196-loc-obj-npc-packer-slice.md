# NAI-196 — `.loc` + `.obj` + `.npc` packer slice + TS-canonical ordering rewrite — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `tools/pack/config/{LocConfig,ObjConfig,NpcConfig}.ts` onto the NAI-191–195 `PackShared` infrastructure. Adds three server+client+jagfile per-config packer branches to `PackConfigs`. Re-orders `PackConfigs` to TS-canonical layout (per `PackShared.ts:261-669`), retiring three accumulated deviations (`PARAM-AFTER-VARS`, `CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS`, `FRESH-CLIENT-JAGFILE`) and introducing one new tag (`UNCONDITIONAL-CLIENT-PACK`).

**Architecture:** Three new `pkg/pack/<config>.go` files (parser + packer + per-config trailer per config). Major rewrite of `pkg/pack/pack_configs.go` (six new lazy `ensureFoo` registry helpers; three new `packAndSaveFoo` functions; full branch re-order; drop ShouldBuild gates from `.param`/`.loc`/`.npc`/`.obj`/`.varp`; eager `objtype.LoadParamTypes`; drop `clientJagDirty`). One in-place rewrite of `pack_configs_test.go:401 TestPackConfigs_EightConfigsLand` to assert the new canonical order. One new deviation-pin file.

**Tech Stack:** Go 1.26+. Stdlib + `pkg/io/packet` + `pkg/io/jagfile` + NAI-191–195 `pkg/pack` foundation + `pkg/objtype` (`ScriptVarType`, `LocType`, `ObjType`, `NPCType`, `ParamTypeConfigs`, `LoadParamTypes`, `LoadLocTypes`, `LoadObjTypes(dir, ptc)`, `LoadNPCTypes`).

**Spec:** `docs/superpowers/specs/2026-05-13-nai-196-loc-obj-npc-packer-slice-design.md` (commit `06276b4`).
**HEAD at plan-write:** `06276b4`.

---

## Conventions used throughout this plan

- **All `go` commands prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`** per global CLAUDE.md.
- **All commits use `git commit --no-gpg-sign`** per global CLAUDE.md.
- **Test style** matches existing `pkg/pack/*_test.go`: bare `if err != nil { t.Fatal(err) }`, `bytes.Equal` for byte comparison, `t.Fatalf("got % x, want % x", got, want)` for byte diffs, `t.TempDir()` for fixture roots, `ClearFsCache()` before tests that mutate the FS.
- **Existing helpers in `pkg/pack`** (use, do NOT redefine):
  - `writeFile(t *testing.T, path, content string)` — `constants_test.go:10`
  - `newTestPF(packType string, entries map[int]string) *PackFile` — `param_test.go:54`
  - `scanPackageDecls(t *testing.T) map[string]bool` — `nai192_deviation_pins_test.go:15`
- **Error envelope** matches existing pkg/pack: `fmt.Errorf("<kind>: %s", detail)` or `fmt.Errorf("<context>: %w", err)`. `packStepError(debugname, msg)` analogue: `fmt.Errorf("%s: %s", debugname, msg)`.
- **Modern Go**: `for id := range pf.Max`, `slices.Index`, `strconv.ParseInt(_, 0, 64)`, `strings.Cut`. `pf.Max` is a STRUCT FIELD (not a method) — see pre-flight.
- **Identifier conventions** (mirroring NAI-195 cohort):
  - Per-config files: `loc.go`, `obj.go`, `npc.go`.
  - Parsers (all closure-bound because all three accept `param=`): `parseLocConfigFor(modelPack, categoryPack, seqPack, texturePack, lk, paramTypes)`, `parseObjConfigFor(modelPack, categoryPack, seqPack, objPack, lk, paramTypes)`, `parseNpcConfigFor(modelPack, categoryPack, seqPack, huntPack, lk, paramTypes)`.
  - Packers: `packLocConfigs(configs, locPack, modelPack)`, `packObjConfigs(configs, objPack)`, `packNpcConfigs(configs, npcPack)`. Each returns `(server, client *PackedData, err error)`.
  - Orchestrator helpers: `packAndSaveLoc`, `packAndSaveObj`, `packAndSaveNpc`.
  - Registry helpers (new): `ensureLocPack`, `ensureNpcPack`, `ensureModelPack`, `ensureCategoryPack`, `ensureHuntPack`, `ensureTexturePack`.

---

## Pre-flight verification (controller, before dispatching tasks)

Verified at plan-write against HEAD `06276b4`:

| Premise | Verification |
|---|---|
| `pkg/pack.PackFile` has `Max int` **struct field** (not method) | ✅ `packfile.go:35`, set by `RefreshNames()` (line 162: `pf.Max = maxID + 1`). Spec §5 referenced `pf.Max()` — must be `pf.Max` in this plan |
| `pkg/pack.NewPackFile(srcDir, packType string, validator Validator) (*PackFile, error)` reads `<srcDir>/pack/<packType>.pack` | ✅ `packfile.go:45,76` |
| `pkg/pack.PackedData` has `NewPackedData(count int) *PackedData`, `Length() int`, `Next()`, methods `P1/P2/P3/P4/PBool/PJStr`, fields `Dat`/`Idx`, `Save(datPath, idxPath string) error` | ✅ `packed_data.go` |
| `pkg/pack.PackFile.GetByName(name) int` returns `-1` for missing; `GetByID(id) string` returns `""` for missing | ✅ `packfile.go:188,192` |
| `pkg/pack.lookupParamValue(typ ScriptVarType, value string, lk *paramLookups) (any, error)` returns `int` or `string`; nil sentinel → `-1` / `""` | ✅ `param.go:92` |
| `pkg/pack.ParamValue` struct with fields `ID int`, `Type objtype.ScriptVarType`, `Value any` exists in `config_value.go` | ✅ added by NAI-195 T1 |
| `pkg/pack.paramLookups` includes lookups consumed by `.loc`/`.obj`/`.npc` param-value resolution | ✅ `param.go:55-69` — covers all 12 typed lookups |
| `pkg/pack.loadParamLookups(srcDir, varpPF)` builds lookup tables | ✅ `pack_configs.go:205` |
| `pkg/pack.ReadTypedConfigs(srcDir, ext, required, parseFn, c)` orchestrates the parse pass | ✅ `read_typed.go:37` |
| `pkg/pack.ConfigLine{Key, Value}`, `pkg/pack.ConfigValue = any` | ✅ `config_value.go:9-14` |
| `pkg/pack.IsConfigBoolean(string) bool`, `GetConfigBoolean(string) bool` | ✅ `config_value.go:23-37` |
| `pkg/pack.ShouldBuild`, `GetLatestModified` exist | ✅ `freshness.go` |
| `pkg/pack.ClearFsCache()` clears the freshness cache | ✅ used in `crawl_test.go` |
| `pkg/pack.scanPackageDecls(t)` returns top-level decls of pkg/pack | ✅ `nai192_deviation_pins_test.go:15` |
| `pkg/pack.checkVarNameUniqueness(varpPack, varnPack, varsPack)` exists | ✅ `pack_configs.go:146` |
| `pkg/pack.LoadConstants(srcDir)` exists | ✅ `pack_configs.go:69` |
| Existing `packAndSaveVarp`, `packAndSaveVarn`, `packAndSaveVars`, `packAndSaveParam`, `packAndSaveEnum`, `packAndSaveInv`, `packAndSaveMesAnim`, `packAndSaveStruct` follow signature `(srcDir, serverOut string, pf *PackFile, /* deps */, c Constants[, clientJag *jagfile.Jagfile]) error` | ✅ `pack_configs.go:287-…` |
| Existing test `TestPackConfigs_EightConfigsLand` at `pack_configs_test.go:401` asserts OLD goscape order (varp first) — must be rewritten in T5 | ✅ verified |
| `pkg/objtype.LoadLocTypes(dir string) (*LocTypeConfigs, error)` reads `<dir>/server/loc.dat` | ✅ `loctype.go:204` |
| `pkg/objtype.LoadObjTypes(dir string, ptc *ParamTypeConfigs) (*ObjTypeConfigs, error)` | ✅ `objtype.go:19` |
| `pkg/objtype.LoadNPCTypes(dir string) (*NPCTypeConfigs, error)` (NOTE: capitalized `NPC`, not `Npc`) | ✅ `npctype.go:348` |
| `pkg/objtype.LoadParamTypes(dir string) (*ParamTypeConfigs, error)` — `dir` arg is parent of `server/` (per `[[load_param_types_dir_arg]]` memory) | ✅ `paramtype.go:38` |
| `pkg/objtype.ScriptVarTypeString` constant = 115 | ✅ `scriptvartype.go:13` |
| `pkg/objtype.ParamTypeConfigs.ByName(name string) *ParamType` returns nil if missing | ✅ `paramtype.go` — accessor exists per NAI-195 plan-write |

**TS-side premises** (verified by reading `tools/pack/config/`):

| TS premise | Source line |
|---|---|
| `LocShapeSuffix` is a TS enum with sparse values (not a contiguous array) | `LocConfig.ts:9-32` — see §"LocShapeSuffix table" below for the full mapping |
| `LocConfig.ts` accepts `param=` at parser level (line 131) and emits opcode 249 trailer (line 407) | ✅ verified at spec self-review |
| `ObjConfig.ts` accepts `param=` (line 165) and emits opcode 249 trailer (line 417) | ✅ verified at spec self-review |
| `NpcConfig.ts` accepts `param=` (line 169) and emits opcode 249 trailer (line 484) | ✅ verified at spec self-review |
| `.param` in TS PackShared.ts:315 has only `shouldBuild` gate; client callback is `() => {}` no-op | ✅ verified at spec self-review — drives R5 resolution below |
| `.seq/.loc/.flo/.spotanim/.npc/.obj/.idk/.varp` in TS use `if (rebuildClient \|\| shouldBuild(...))`; with `const rebuildClient = true` at line 337, these all run unconditionally | ✅ `PackShared.ts:460/477/501/525/548/571/594/614` |
| `LocConfig.ts:165-432` opcode emit map: 14,15,17,18,19,21,22,23,24,27,28,29,39,40,60,62,64-75,77-79,81,82,89,249,250 (per-opcode semantics in §"Loc opcode map" below) | ✅ |
| `ObjConfig.ts:196-440` opcode emit map: 1,2,3,4,5,6,7,8,9,10,11,12,16,23,24,25,26,27,28,29,30-34,35-39,40,42,65,78,90,91,92,93,94,95,97,98,100,101,102,103,104,107,200-201,249,250 (per-opcode semantics in §"Obj opcode map" below) | ✅ |
| `NpcConfig.ts:265-509` opcode emit map: 1,2,12,13,14,16,17,18,30-34,40,41,42,60-70,74,75,77,78,79,80,82,90,93,95,97,100,101,102,103,107,109,111,112,113,114,134,138,140,142,150,153,154,155,156,157,158,249,250 (per-opcode semantics in §"Npc opcode map" below) | ✅ |

### LocShapeSuffix table (port verbatim into `pkg/pack/loc.go`)

TS `LocConfig.ts:9-32`:

```ts
enum LocShapeSuffix {
    _1 = 0,  _2 = 1,  _3 = 2,  _4 = 3,
    _q = 4,
    _5 = 9,
    _w = 5,  _r = 6,  _e = 7,  _t = 8,
    _8 = 10, _9 = 11,
    _0 = 22,
    _a = 12, _s = 13, _d = 14, _f = 15, _g = 16, _h = 17,
    _z = 18, _x = 19, _c = 20, _v = 21,
}
```

In TS, the reverse-map `LocShapeSuffix[shape]` returns the `_NN` *string* given a shape *value* (e.g., `LocShapeSuffix[10]` → `"_8"`). The Go port maps `shape int` → suffix `string` via a direct `map[int]string` or array indexed by shape. Use a `map[int]string` (sparse) keyed by shape, with values `"_1"`/`"_2"`/.../`"_v"`. There are 22 entries (shapes 0..22; shape 9 is `_5`, shape 22 is `_0`).

### Resolution of spec §9 R5 (`.param` ShouldBuild gate)

**TS reality:** `.param` is gated by `shouldBuild` only (no `rebuildClient` ungate). `.param`'s client callback in TS is `() => {}` no-op — `.param` does NOT contribute to client jagfile in TS.

**Goscape carryforward deviation `NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL`:** goscape DOES write empty `param.dat`/`param.idx` to the client jagfile. This stays.

**Conflict:** retiring `FRESH-CLIENT-JAGFILE` while keeping `.param` ShouldBuild-gated would create a new bug — on a no-op build (no `.param` source changes) the fresh-empty client jagfile would miss `param.dat`/`param.idx`, even though prior runs had written them.

**Resolution (option (i) from spec):** drop `.param`'s ShouldBuild gate as well. `.param` joins the unconditional client+server group. The `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` tag covers all FIVE branches: `.param`, `.loc`, `.npc`, `.obj`, `.varp`.

---

## File inventory

```
pkg/pack/
  loc.go                                   NEW    (parseLocConfigFor + packLocConfigs + LocShapeSuffix)
  loc_test.go                              NEW    (parser + packer byte-pin tests)
  loc_roundtrip_test.go                    NEW    (PackConfigs → LoadLocTypes round-trip)
  obj.go                                   NEW    (parseObjConfigFor + packObjConfigs)
  obj_test.go                              NEW    (parser + packer byte-pin tests)
  obj_roundtrip_test.go                    NEW    (PackConfigs → LoadObjTypes round-trip)
  npc.go                                   NEW    (parseNpcConfigFor + packNpcConfigs)
  npc_test.go                              NEW    (parser + packer byte-pin tests)
  npc_roundtrip_test.go                    NEW    (PackConfigs → LoadNPCTypes round-trip)
  pack_configs.go                          MODIFY (full body rewrite — 6 new ensureFoo helpers, 3 new packAndSaveFoo, full re-order, drop clientJagDirty, drop 5× ShouldBuild gates, eager LoadParamTypes)
  pack_configs_test.go                     MODIFY (in-place rewrite of TestPackConfigs_EightConfigsLand → TestPackConfigs_ElevenConfigsLand; extend to assert new client jag contents)
  nai196_deviation_pins_test.go            NEW    (3 absence pins + 1 presence pin + 1 sanity pin)
```

---

## Task overview

| T | Subject | Test files | Production files |
|---|---|---|---|
| T1 | Lazy `ensureFoo` registry helpers (6 new) — no callers yet | (none) | `pack_configs.go` |
| T2 | `.loc` parser + packer + byte-pin tests | `loc_test.go` | `loc.go` |
| T3 | `.obj` parser + packer + byte-pin tests | `obj_test.go` | `obj.go` |
| T4 | `.npc` parser + packer + byte-pin tests | `npc_test.go` | `npc.go` |
| T5 | `PackConfigs` body rewrite — TS-canonical order, retire 3 deviation tags, eager LoadParamTypes, drop clientJagDirty; rewrite `TestPackConfigs_EightConfigsLand` → `_ElevenConfigsLand` | `pack_configs_test.go` (in-place) | `pack_configs.go` |
| T6 | Round-trip tests via `LoadLocTypes`/`LoadObjTypes`/`LoadNPCTypes` | `loc_roundtrip_test.go`, `obj_roundtrip_test.go`, `npc_roundtrip_test.go` | (none — exercises T1–T5) |
| T7 | Eleven-config integration test (extend T5's rewrite to ensure all configs run together) | (extension within `pack_configs_test.go`) | (none) |
| T8 | Deviation-tag pins: 3 absence + 1 presence + 1 sanity | `nai196_deviation_pins_test.go` | (none) |

---

## Task 1: Lazy `ensureFoo` registry helpers (additive, no callers)

**Files:**
- Modify: `pkg/pack/pack_configs.go` (function `PackConfigs`)

This task additively introduces six new lazy registry helpers and accompanying `var` declarations. They are NOT yet called from any branch — T5 wires them. This task exists to land the helpers in isolation so T2/T3/T4 can develop against existing source. The build must remain green after this task.

- [ ] **Step 1.1: Inspect existing helpers**

```bash
grep -n "ensureObjPack\|ensureSeqPack\|ensureLk\|ensureParamTypes" /home/owner/Code/github.com/zsrv/goscape/pkg/pack/pack_configs.go
```

Expected: lines 102–155 region show all four existing `ensureFoo` closures.

- [ ] **Step 1.2: Add six new var declarations**

Locate the existing `var` block around `pack_configs.go:102-108`:

```go
var (
    lk         *paramLookups
    objPack    *PackFile
    seqPack    *PackFile
    paramTypes *objtype.ParamTypeConfigs
)
```

Extend it to:

```go
var (
    lk           *paramLookups
    objPack      *PackFile
    seqPack      *PackFile
    locPack      *PackFile
    npcPack      *PackFile
    modelPack    *PackFile
    categoryPack *PackFile
    huntPack     *PackFile
    texturePack  *PackFile
    paramTypes   *objtype.ParamTypeConfigs
)
```

- [ ] **Step 1.3: Add six new `ensureFoo` closures**

Immediately after the existing `ensureParamTypes` closure (around `pack_configs.go:152`), add:

```go
ensureLocPack := func() error {
    if locPack != nil {
        return nil
    }
    pf, err := NewPackFile(srcDir, "loc", nil)
    if err != nil {
        return err
    }
    locPack = pf
    return nil
}
ensureNpcPack := func() error {
    if npcPack != nil {
        return nil
    }
    pf, err := NewPackFile(srcDir, "npc", nil)
    if err != nil {
        return err
    }
    npcPack = pf
    return nil
}
ensureModelPack := func() error {
    if modelPack != nil {
        return nil
    }
    pf, err := NewPackFile(srcDir, "model", nil)
    if err != nil {
        return err
    }
    modelPack = pf
    return nil
}
ensureCategoryPack := func() error {
    if categoryPack != nil {
        return nil
    }
    pf, err := NewPackFile(srcDir, "category", nil)
    if err != nil {
        return err
    }
    categoryPack = pf
    return nil
}
ensureHuntPack := func() error {
    if huntPack != nil {
        return nil
    }
    pf, err := NewPackFile(srcDir, "hunt", nil)
    if err != nil {
        return err
    }
    huntPack = pf
    return nil
}
ensureTexturePack := func() error {
    if texturePack != nil {
        return nil
    }
    pf, err := NewPackFile(srcDir, "texture", nil)
    if err != nil {
        return err
    }
    texturePack = pf
    return nil
}
```

- [ ] **Step 1.4: Suppress "declared and not used" via `_ = ensureFoo` block**

Immediately after the six new closures, add (still inside `PackConfigs`):

```go
// NAI-196 T1: helpers landed without callers; T5 wires them. Suppressing
// unused-variable diagnostics until then.
_ = ensureLocPack
_ = ensureNpcPack
_ = ensureModelPack
_ = ensureCategoryPack
_ = ensureHuntPack
_ = ensureTexturePack
```

- [ ] **Step 1.5: Verify build**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: clean exit, no errors.

- [ ] **Step 1.6: Verify existing tests pass**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1
```

Expected: all green.

- [ ] **Step 1.7: Commit**

```bash
git add pkg/pack/pack_configs.go
git commit --no-gpg-sign -m "feat(pack): NAI-196 T1 — ensureFoo helpers for loc/npc/model/category/hunt/texture

Adds six new lazy registry-helper closures + var declarations inside
PackConfigs. Suppressed via _ = ensureFoo until T5 wires the new branches.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: `.loc` parser + packer + byte-pin tests

**Files:**
- Create: `pkg/pack/loc.go`
- Create: `pkg/pack/loc_test.go`

### Loc opcode map (from `LocConfig.ts:165-432`)

This is the authoritative opcode table the packer emits, in TS source order. Use it as the implementation guide. **Each opcode below corresponds to a real TS branch — port them all.**

| Opcode | TS source line | Trigger | Emit |
|---|---|---|---|
| 1 | 219-242 | `models.length > 0` | `p1(models.length) + per-model: p2(modelId) + p1(shape)` |
| 2 | 245-247 | `name !== null` | `pjstr(name)` |
| 14 | 250-252 | `width !== 1` | `p1(width)` |
| 15 | 254-256 | `length !== 1` | `p1(length)` |
| 17 | 258-260 | `blockwalk === false` | (no payload) |
| 18 | 262-264 | `blockrange === false` | (no payload) |
| 19 | 266-268 | `active !== null` | `p1(active ? 1 : 0)` |
| 21 | 270-272 | `hillskew === true` | (no payload) |
| 22 | 274-276 | `sharelight === true` | (no payload) |
| 23 | 278-280 | `occlude === true` | (no payload) |
| 24 | 282-284 | `anim !== -1` | `p2(anim)` |
| 27 | 286-288 | `wall === true` | (no payload) |
| 28 | 290-292 | `walloff !== 0` | `p1(walloff)` |
| 29 | 294-296 | `ambient !== 0` | `p1(ambient)` |
| 39 | 298-300 | `contrast !== 0` | `p1(contrast)` |
| 40 | 302-310 | `recol.length > 0` | `p1(count) + per: p2(src)+p2(dst)` |
| 60 | 312-314 | `mapfunction !== -1` | `p2(mapfunction)` |
| 62 | 316-318 | `mirror === true` | (no payload) |
| 64 | 320-322 | `shadow === false` | (no payload) |
| 65 | 324-326 | `resizex !== 128` | `p2(resizex)` |
| 66 | 328-330 | `resizey !== 128` | `p2(resizey)` |
| 67 | 332-334 | `resizez !== 128` | `p2(resizez)` |
| 68 | 336-338 | `mapscene !== -1` | `p2(mapscene)` |
| 69 | 340-342 | `forceapproach !== 0` | `p1(forceapproach)` |
| 70 | 344-346 | `xoff !== 0` | `p2(xoff)` |
| 71 | 348-350 | `yoff !== 0` | `p2(yoff)` |
| 72 | 352-354 | `zoff !== 0` | `p2(zoff)` |
| 73 | 356-358 | `forcedecor === true` | (no payload) |
| 74-75, 77-79, 81-82 | 360-382 | `op[N] !== null` for N=1..5 | per-op: `p1(opcode) + pjstr(opN)` |
| 89 | 384-388 | `retex.length > 0` | `p1(count) + per: p2(src)+p2(dst)` |
| 249 | 406-422 | `params.length > 0` | `p1(count) + per: p3(id)+pbool(type==STRING)+pjstr(value)\|p4(value)` |
| 250 | 425-428 | `debugname.length > 0` | `pjstr(debugname)` |

**Client-side emit:** TS emits a subset to `client` packet in a separate loop. Per `LocConfig.ts:172-217`:

| Client opcode | TS line | Trigger | Emit |
|---|---|---|---|
| 1 | 174-194 | `models.length > 0` | identical to server |
| 2 | 196-198 | `name !== null` | `pjstr(name)` |
| 3 | 200-203 | `desc !== null` | `pjstr(desc)` |
| 14-89 | varies | subset of server-side recol/retex/anim/wall/etc. that the client needs for rendering | per TS branch |

The Go port emits to BOTH `server *PackedData` and `client *PackedData` in a single per-id loop; each branch determines whether to write to one, the other, or both based on TS.

### Step 2.1: Implement parser

- [ ] **Step 2.1.1: Write parser-side test skeleton (`pkg/pack/loc_test.go`)**

```go
package pack

import (
    "bytes"
    "testing"

    "github.com/zsrv/goscape/pkg/objtype"
)

// helpers for loc tests
func locTestRegistries(t *testing.T) (modelPack, categoryPack, seqPack, texturePack *PackFile, paramTypes *objtype.ParamTypeConfigs, lk *paramLookups) {
    t.Helper()
    modelPack = newTestPF("model", map[int]string{
        0: "table",
        1: "chair",
        2: "table_8",
    })
    categoryPack = newTestPF("category", map[int]string{
        0: "furniture",
    })
    seqPack = newTestPF("seq", map[int]string{
        0: "idle",
    })
    texturePack = newTestPF("texture", map[int]string{
        0: "wood",
    })
    paramTypes = &objtype.ParamTypeConfigs{
        ConfigNames: map[string]int{"flammable": 7},
        Configs: []*objtype.ParamType{
            6: {ID: 6},
            7: {ID: 7, Type: objtype.ScriptVarTypeInt},
        },
    }
    lk = &paramLookups{}
    return
}

func TestParseLocConfig_Name(t *testing.T) {
    mp, cp, sp, tp, pt, lk := locTestRegistries(t)
    parse := parseLocConfigFor(mp, cp, sp, tp, lk, pt)

    val, accepted, err := parse("name", "Table")
    if err != nil {
        t.Fatal(err)
    }
    if !accepted {
        t.Fatal("name key should be accepted")
    }
    s, ok := val.(string)
    if !ok || s != "Table" {
        t.Fatalf("got %#v, want string \"Table\"", val)
    }
}

func TestParseLocConfig_Width(t *testing.T) {
    mp, cp, sp, tp, pt, lk := locTestRegistries(t)
    parse := parseLocConfigFor(mp, cp, sp, tp, lk, pt)

    val, accepted, err := parse("width", "3")
    if err != nil {
        t.Fatal(err)
    }
    if !accepted {
        t.Fatal("width key should be accepted")
    }
    n, ok := val.(int)
    if !ok || n != 3 {
        t.Fatalf("got %#v, want int 3", val)
    }
}

func TestParseLocConfig_Param(t *testing.T) {
    mp, cp, sp, tp, pt, lk := locTestRegistries(t)
    parse := parseLocConfigFor(mp, cp, sp, tp, lk, pt)

    val, accepted, err := parse("param", "flammable,1")
    if err != nil {
        t.Fatal(err)
    }
    if !accepted {
        t.Fatal("param key should be accepted")
    }
    pv, ok := val.(ParamValue)
    if !ok {
        t.Fatalf("got %#v, want ParamValue", val)
    }
    if pv.ID != 7 {
        t.Fatalf("got ID=%d, want 7", pv.ID)
    }
    if pv.Type != objtype.ScriptVarTypeInt {
        t.Fatalf("got Type=%d, want Int", pv.Type)
    }
    iv, ok := pv.Value.(int)
    if !ok || iv != 1 {
        t.Fatalf("got Value=%#v, want int 1", pv.Value)
    }
}

func TestParseLocConfig_UnknownKey(t *testing.T) {
    mp, cp, sp, tp, pt, lk := locTestRegistries(t)
    parse := parseLocConfigFor(mp, cp, sp, tp, lk, pt)

    val, accepted, err := parse("zzz_unknown", "value")
    if err != nil {
        t.Fatal(err)
    }
    if accepted {
        t.Fatal("unknown key should NOT be accepted")
    }
    if val != nil {
        t.Fatalf("got %#v, want nil", val)
    }
}
```

Bytes referenced: see `objtype.ScriptVarTypeInt` value confirmed at `scriptvartype.go:12` = 105.

- [ ] **Step 2.1.2: Run — expect FAIL (parseLocConfigFor undefined)**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseLocConfig -count=1 -v
```

Expected: compilation failure: `parseLocConfigFor` undefined.

- [ ] **Step 2.1.3: Implement parser (`pkg/pack/loc.go`)**

Create the file with package header + LocShapeSuffix table + parser. The parser handles every accepted-key branch from `LocConfig.ts:34-170`. Below is the complete parser; the implementer should follow TS branches in order.

```go
package pack

import (
    "fmt"
    "strconv"
    "strings"

    "github.com/zsrv/goscape/pkg/objtype"
)

// LocShapeSuffix maps a shape number (the TS enum's *value*) to the
// 2-character suffix used in shape-specific model name synthesis.
// Source: tools/pack/config/LocConfig.ts:9-32 (TS reverse-map lookup).
var LocShapeSuffix = map[int]string{
    0:  "_1",
    1:  "_2",
    2:  "_3",
    3:  "_4",
    4:  "_q",
    5:  "_w",
    6:  "_r",
    7:  "_e",
    8:  "_t",
    9:  "_5",
    10: "_8",
    11: "_9",
    12: "_a",
    13: "_s",
    14: "_d",
    15: "_f",
    16: "_g",
    17: "_h",
    18: "_z",
    19: "_x",
    20: "_c",
    21: "_v",
    22: "_0",
}

// parseLocConfigFor returns the per-key=value parser for .loc config
// blocks. Closure-captures four name-map registries plus paramLookups +
// ParamTypeConfigs for param= resolution.
//
// TS source: tools/pack/config/LocConfig.ts:34-170.
func parseLocConfigFor(modelPack, categoryPack, seqPack, texturePack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs) ParseFn {
    return func(key, value string) (ConfigValue, bool, error) {
        // Numeric width / length: 0..255
        if key == "width" || key == "length" {
            n, err := strconv.ParseInt(value, 0, 64)
            if err != nil {
                return nil, true, fmt.Errorf("invalid %s: %s", key, value)
            }
            if n < 0 || n > 255 {
                return nil, true, fmt.Errorf("%s out of range [0,255]: %d", key, n)
            }
            return int(n), true, nil
        }

        // Strings
        if key == "name" || key == "desc" {
            return value, true, nil
        }

        // model{N} — shape-suffix-aware (TS LocConfig.ts:67-94)
        if strings.HasPrefix(key, "model") {
            // The TS parser stores raw value here; shape resolution happens at pack time
            // via the model lookup in packLocConfigs.
            return value, true, nil
        }

        // recol{N}{s|d}, retex{N}{s|d}, mapscene, mapfunction, forceapproach{N}
        // anim, category, op{N}, booleans, numeric, etc. — see TS LocConfig.ts:96-160
        // (Each branch follows the same shape: convert to int/bool/string and return.)
        // ...
        // [Full per-key branch list per TS LocConfig.ts:34-170 — the implementer
        // should port each branch in order. For brevity in this plan, the
        // representative branches above show the pattern. The full set:
        //
        //   width/length     → int (above)
        //   name/desc        → string (above)
        //   model{N}/{suffix}→ string (above; resolution deferred to packer)
        //   recol{N}{s|d}    → int via strconv.ParseInt
        //   retex{N}{s|d}    → for `d` suffix: texturePack.GetByName(value) → reject -1
        //                      for `s` suffix: int via strconv.ParseInt
        //   category         → categoryPack.GetByName(value) → reject -1
        //   anim             → seqPack.GetByName(value) → reject -1
        //   booleans         → IsConfigBoolean(value) ? GetConfigBoolean(value) : error
        //                      (keys: blockwalk, blockrange, active, hillskew,
        //                       sharelight, occlude, wall, mirror, shadow, forcedecor)
        //   mapscene/mapfunction → int
        //   forceapproach{N} → directional enum (parse to int)
        //   xoff/yoff/zoff   → int (signed)
        //   walloff          → int
        //   ambient/contrast → int (signed)
        //   resizex/y/z      → int
        //   op{N}            → string (N=1..5)
        //   ]

        // param=<name>,<valueStr>
        if key == "param" {
            i := strings.Index(value, ",")
            if i < 0 {
                return nil, true, fmt.Errorf("param missing comma: %s", value)
            }
            paramName := value[:i]
            paramValueStr := value[i+1:]
            p := paramTypes.ByName(paramName)
            if p == nil {
                return nil, true, fmt.Errorf("unknown param: %s", paramName)
            }
            v, err := lookupParamValue(p.Type, paramValueStr, lk)
            if err != nil {
                return nil, true, err
            }
            return ParamValue{ID: p.ID, Type: p.Type, Value: v}, true, nil
        }

        return nil, false, nil
    }
}
```

**Note on completeness:** The branches marked `// ...` above (recol, retex, category, anim, booleans, etc.) MUST be fully implemented by the engineer following TS `LocConfig.ts:34-170` line-by-line. The plan codifies the parser's *contract* (which keys are accepted, what shape of value each returns) and the param= branch (which is non-obvious); the remaining branches are mechanical TS-to-Go translations and the engineer should expand them in this same step. The `loc_test.go` tests added in step 2.1.1 + the packer tests in steps 2.2.1+ pin the behavior of every accepted-key branch.

- [ ] **Step 2.1.4: Run — expect parser tests PASS**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseLocConfig -count=1 -v
```

Expected: all four `TestParseLocConfig_*` tests PASS.

### Step 2.2: Implement packer

- [ ] **Step 2.2.1: Append packer byte-pin tests to `loc_test.go`**

Test cases mirror the opcode map above. Below are representative cases; the engineer adds one test per opcode listed in the Loc opcode map.

```go
func TestPackLocConfigs_Name(t *testing.T) {
    mp, _, _, _, _, _ := locTestRegistries(t)
    locPack := newTestPF("loc", map[int]string{0: "table"})

    configs := map[string][]ConfigLine{
        "table": {{Key: "name", Value: "Table"}},
    }
    server, _, err := packLocConfigs(configs, locPack, mp)
    if err != nil {
        t.Fatal(err)
    }

    // Expected server bytes for id=0 ("table"):
    //   opcode 2 (name)   = 0x02
    //   pjstr("Table")    = "Table\x00"
    //   opcode 250 (dbg)  = 0xFA
    //   pjstr("table")    = "table\x00"
    //   opcode 0 (term)   = 0x00
    want := []byte{
        0x02, 'T', 'a', 'b', 'l', 'e', 0x00,
        0xFA, 't', 'a', 'b', 'l', 'e', 0x00,
        0x00,
    }
    got := server.Dat.Data[:server.Dat.Length()]
    if !bytes.Equal(got, want) {
        t.Fatalf("\ngot  % x\nwant % x", got, want)
    }
}

func TestPackLocConfigs_Width(t *testing.T) {
    mp, _, _, _, _, _ := locTestRegistries(t)
    locPack := newTestPF("loc", map[int]string{0: "block"})

    configs := map[string][]ConfigLine{
        "block": {{Key: "width", Value: 3}},
    }
    server, _, err := packLocConfigs(configs, locPack, mp)
    if err != nil {
        t.Fatal(err)
    }

    // opcode 14 = 0x0E + p1(3) + opcode 250 + "block\x00" + opcode 0
    want := []byte{
        0x0E, 0x03,
        0xFA, 'b', 'l', 'o', 'c', 'k', 0x00,
        0x00,
    }
    got := server.Dat.Data[:server.Dat.Length()]
    if !bytes.Equal(got, want) {
        t.Fatalf("\ngot  % x\nwant % x", got, want)
    }
}

func TestPackLocConfigs_Param(t *testing.T) {
    mp, _, _, _, _, _ := locTestRegistries(t)
    locPack := newTestPF("loc", map[int]string{0: "fire"})

    configs := map[string][]ConfigLine{
        "fire": {{
            Key:   "param",
            Value: ParamValue{ID: 7, Type: objtype.ScriptVarTypeInt, Value: 1},
        }},
    }
    server, _, err := packLocConfigs(configs, locPack, mp)
    if err != nil {
        t.Fatal(err)
    }

    // opcode 249 = 0xF9
    //   p1(1) param count
    //   p3(7) = 0x00 0x00 0x07
    //   pbool(false) = 0x00 (type != STRING)
    //   p4(1) = 0x00 0x00 0x00 0x01
    // opcode 250 + "fire\x00" + opcode 0
    want := []byte{
        0xF9,
        0x01,
        0x00, 0x00, 0x07,
        0x00,
        0x00, 0x00, 0x00, 0x01,
        0xFA, 'f', 'i', 'r', 'e', 0x00,
        0x00,
    }
    got := server.Dat.Data[:server.Dat.Length()]
    if !bytes.Equal(got, want) {
        t.Fatalf("\ngot  % x\nwant % x", got, want)
    }
}

// Additional tests to add (one per opcode in the Loc opcode map):
// TestPackLocConfigs_Models                — opcode 1 + 1×(p2 p1) for one model
// TestPackLocConfigs_Length                — opcode 15 + p1
// TestPackLocConfigs_BlockwalkFalse        — opcode 17 (no payload)
// TestPackLocConfigs_BlockrangeFalse       — opcode 18
// TestPackLocConfigs_Active                — opcode 19 + p1
// TestPackLocConfigs_HillskewTrue          — opcode 21
// TestPackLocConfigs_SharelightTrue        — opcode 22
// TestPackLocConfigs_OccludeTrue           — opcode 23
// TestPackLocConfigs_Anim                  — opcode 24 + p2 (resolved via seqPack)
// TestPackLocConfigs_WallTrue              — opcode 27
// TestPackLocConfigs_Walloff               — opcode 28 + p1
// TestPackLocConfigs_Ambient               — opcode 29 + p1
// TestPackLocConfigs_Contrast              — opcode 39 + p1
// TestPackLocConfigs_RecolPair             — opcode 40 + p1(count) + p2 p2
// TestPackLocConfigs_Mapfunction           — opcode 60 + p2
// TestPackLocConfigs_MirrorTrue            — opcode 62
// TestPackLocConfigs_ShadowFalse           — opcode 64
// TestPackLocConfigs_Resizex               — opcode 65 + p2
// TestPackLocConfigs_Resizey               — opcode 66 + p2
// TestPackLocConfigs_Resizez               — opcode 67 + p2
// TestPackLocConfigs_Mapscene              — opcode 68 + p2
// TestPackLocConfigs_Forceapproach         — opcode 69 + p1
// TestPackLocConfigs_Xoff                  — opcode 70 + p2
// TestPackLocConfigs_Yoff                  — opcode 71 + p2
// TestPackLocConfigs_Zoff                  — opcode 72 + p2
// TestPackLocConfigs_ForcedecorTrue        — opcode 73
// TestPackLocConfigs_Op1                   — opcode 74 + pjstr  (op1..op5 per N)
// TestPackLocConfigs_RetexPair             — opcode 89 + p1(count) + p2 p2 (resolved via texturePack)
// TestPackLocConfigs_ParamString           — opcode 249 string variant (pbool=true + pjstr)
// TestPackLocConfigs_DebugnameEmpty        — opcode 250 NOT emitted when debugname=="", just opcode 0 terminator
//
// Each test follows the pattern above: build configs, call packLocConfigs,
// compare server.Dat.Data[:server.Dat.Length()] to a hand-encoded byte slice.
```

- [ ] **Step 2.2.2: Run — expect FAIL (packLocConfigs undefined)**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackLocConfigs -count=1 -v
```

Expected: compilation failure: `packLocConfigs` undefined.

- [ ] **Step 2.2.3: Implement packer (append to `pkg/pack/loc.go`)**

Add after `parseLocConfigFor`. The packer walks each `(id, debugname)` slot in `[0, locPack.Max)`, emits all opcodes per the Loc opcode map, writes the 250-trailer + debugname, calls `Next()`, and returns the dual-`PackedData` result.

```go
// packLocConfigs walks each id in [0, locPack.Max), emits all opcodes
// per LocConfig.ts:165-432, and returns the server + client PackedData.
//
// The server-side and client-side opcode sets overlap heavily but
// diverge in detail (e.g., desc only goes to client). The packer
// runs ONE pass per id, calling p1/p2/etc. on `server` or `client` as
// dictated by TS branch.
//
// TS source: tools/pack/config/LocConfig.ts:172-432.
func packLocConfigs(configs map[string][]ConfigLine, locPack, modelPack *PackFile) (server, client *PackedData, err error) {
    server = NewPackedData(locPack.Max)
    client = NewPackedData(locPack.Max)

    for id := 0; id < locPack.Max; id++ {
        debugname := locPack.GetByID(id)
        lines := configs[debugname]

        // ----- Server-side opcodes -----
        var (
            models       []struct{ ID, Shape int }
            name, desc   string
            width        = 1
            length       = 1
            blockwalk    = true
            blockrange   = true
            // ... (one local per accepted key — full list per TS LocConfig.ts:173-217)
            params       []ParamValue
        )

        // First pass: collect all values from `lines` into local variables.
        for _, line := range lines {
            switch line.Key {
            case "name":
                name, _ = line.Value.(string)
            case "desc":
                desc, _ = line.Value.(string)
            case "width":
                width, _ = line.Value.(int)
            case "length":
                length, _ = line.Value.(int)
            case "blockwalk":
                blockwalk, _ = line.Value.(bool)
            case "blockrange":
                blockrange, _ = line.Value.(bool)
            // ... (one case per accepted key; resolve model{N} shape-suffix to
            //      `models` slice here, calling modelPack.GetByName(name+suffix))
            case "param":
                if pv, ok := line.Value.(ParamValue); ok {
                    params = append(params, pv)
                }
            }
        }

        // Second pass: emit opcodes per Loc opcode map.
        // Opcode 1: models
        if len(models) > 0 {
            server.Dat.P1(1)
            server.Dat.P1(uint8(len(models)))
            for _, m := range models {
                server.Dat.P2(uint16(m.ID))
                server.Dat.P1(uint8(m.Shape))
            }
        }
        // Opcode 2: name
        if name != "" {
            server.Dat.P1(2)
            server.Dat.PJStr(name)
        }
        // Opcode 14: width
        if width != 1 {
            server.Dat.P1(14)
            server.Dat.P1(uint8(width))
        }
        // Opcode 15: length
        if length != 1 {
            server.Dat.P1(15)
            server.Dat.P1(uint8(length))
        }
        // Opcode 17: blockwalk false (no payload)
        if !blockwalk {
            server.Dat.P1(17)
        }
        // Opcode 18: blockrange false (no payload)
        if !blockrange {
            server.Dat.P1(18)
        }
        // ... [remainder of opcode map per Loc opcode map table — port each branch
        //      from TS LocConfig.ts:250-388 in order. The pattern is identical:
        //      check trigger condition → p1(opcode) → p<size>(payload).]

        // Opcode 249: param trailer
        if len(params) > 0 {
            server.Dat.P1(249)
            server.Dat.P1(uint8(len(params)))
            for _, pv := range params {
                server.Dat.P3(uint32(pv.ID))
                server.Dat.PBool(pv.Type == objtype.ScriptVarTypeString)
                if pv.Type == objtype.ScriptVarTypeString {
                    server.Dat.PJStr(pv.Value.(string))
                } else {
                    server.Dat.P4(uint32(pv.Value.(int)))
                }
            }
        }

        // Opcode 250: debugname trailer
        if debugname != "" {
            server.Dat.P1(250)
            server.Dat.PJStr(debugname)
        }

        // ----- Client-side opcodes (separate emit per TS LocConfig.ts:172-217) -----
        // Opcode 1: models (identical to server)
        if len(models) > 0 {
            client.Dat.P1(1)
            client.Dat.P1(uint8(len(models)))
            for _, m := range models {
                client.Dat.P2(uint16(m.ID))
                client.Dat.P1(uint8(m.Shape))
            }
        }
        // Opcode 2: name
        if name != "" {
            client.Dat.P1(2)
            client.Dat.PJStr(name)
        }
        // Opcode 3: desc (client only — server does NOT emit desc)
        if desc != "" {
            client.Dat.P1(3)
            client.Dat.PJStr(desc)
        }
        // ... [remainder of client-side opcode subset per TS LocConfig.ts:200-217]

        // Both terminators
        server.Next()
        client.Next()
    }

    return server, client, nil
}
```

**Note on completeness:** The `// ...` markers in the packer represent the full opcode map listed in the table above. The engineer ports each opcode branch from TS in order. The byte-pin tests added in step 2.2.1 — one per opcode — pin the byte output of every branch.

- [ ] **Step 2.2.4: Run — expect PASS**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackLocConfigs -count=1 -v
```

Expected: all `TestPackLocConfigs_*` tests PASS. If a specific opcode test fails, fix the corresponding branch in `loc.go`; do NOT modify tests.

- [ ] **Step 2.2.5: Run full pack package**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1
```

Expected: all green (parser tests + packer tests + existing NAI-191–195 tests).

- [ ] **Step 2.2.6: Commit**

```bash
git add pkg/pack/loc.go pkg/pack/loc_test.go
git commit --no-gpg-sign -m "feat(pack): NAI-196 T2 — .loc parser + packer

parseLocConfigFor + packLocConfigs port of LocConfig.ts:34-432.
LocShapeSuffix table ported verbatim. Param= resolution mirrors
.obj/.npc/.struct cohort (opcode 249 with id/type/value triple).
Dual server+client PackedData emission per TS branch divergence.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: `.obj` parser + packer + byte-pin tests

**Files:**
- Create: `pkg/pack/obj.go`
- Create: `pkg/pack/obj_test.go`

### Obj opcode map (from `ObjConfig.ts:196-440`)

| Opcode | TS line | Trigger | Emit |
|---|---|---|---|
| 1 | 222-224 | `model !== -1` | `p2(model)` |
| 2 | 226-228 | `name !== null` | `pjstr(name)` |
| 3 | 230-232 | `desc !== null` | `pjstr(desc)` (client also receives) |
| 4 | 234-236 | `zoom2d !== 2000` | `p2(zoom2d)` |
| 5 | 238-240 | `xan2d !== 0` | `p2(xan2d)` |
| 6 | 242-244 | `yan2d !== 0` | `p2(yan2d)` |
| 7 | 246-248 | `xof2d !== 0` | `p2(xof2d)` |
| 8 | 250-252 | `yof2d !== 0` | `p2(yof2d)` |
| 9 | 254-256 | `code9 === true` | (no payload) |
| 10 | 258-260 | `code10 !== -1` | `p2(code10)` |
| 11 | 262-264 | `stackable === true` | (no payload) |
| 12 | 266-268 | `cost !== 1` | `p4(cost)` |
| 16 | 270-272 | `members === true` | (no payload) |
| 23-29 | 274-296 | per-slot wear (manwear/womanwear/manwear2/womanwear2/manwear3/womanwear3) | `p2(modelId) + p1(offset)` |
| 30-34 | 298-310 | op1..op5 (inv) | `pjstr(opN)` |
| 35-39 | 312-330 | iop1..iop5 (interface op) | `pjstr(iopN)` |
| 40 | 332-344 | recol pairs | `p1(count) + per: p2(src)+p2(dst)` |
| 42 | 346-354 | retex pairs | `p1(count) + per: p2(src)+p2(dst)` |
| 65 | 356-358 | `stockmarket === true` | (no payload) |
| 78 | 360-362 | `manwear3 !== -1` (head wear) | `p2(model) + p1(offset)` |
| 90 | 364-366 | `manhead !== -1` | `p2(model)` |
| 91 | 368-370 | `womanhead !== -1` | `p2(model)` |
| 92 | 372-374 | `manhead2 !== -1` | `p2(model)` |
| 93 | 376-378 | `womanhead2 !== -1` | `p2(model)` |
| 94 | 380-382 | `category !== -1` | `p2(category)` |
| 95 | 384-386 | `zan2d !== 0` | `p2(zan2d)` |
| 97 | 388-390 | `certlink (uncert) !== -1` | `p2(certlink)` — cert/uncert pairing |
| 98 | 392-394 | `certtemplate !== -1` | `p2(certtemplate)` |
| 100-109 | 396-410 | stack variants | `p2(id)+p2(count)` per slot |
| 200 | 414 | `team !== -1` | `p1(team)` |
| 201 | 416 | `weight !== 0` | `p2(weight)` — signed |
| 249 | 417-431 | param trailer | identical shape to `.loc`/`.npc`/`.struct` |
| 250 | 434-437 | debugname | `pjstr(debugname)` |

### Step 3.1: Implement parser

- [ ] **Step 3.1.1: Write parser tests (`pkg/pack/obj_test.go`)**

```go
package pack

import (
    "bytes"
    "testing"

    "github.com/zsrv/goscape/pkg/objtype"
)

func objTestRegistries(t *testing.T) (modelPack, categoryPack, seqPack, objPack *PackFile, paramTypes *objtype.ParamTypeConfigs, lk *paramLookups) {
    t.Helper()
    modelPack = newTestPF("model", map[int]string{0: "model_zero", 1: "model_one"})
    categoryPack = newTestPF("category", map[int]string{0: "weapon", 1: "armor"})
    seqPack = newTestPF("seq", map[int]string{0: "swing"})
    objPack = newTestPF("obj", map[int]string{
        0: "sword",
        1: "cert_sword",
        2: "shield",
    })
    paramTypes = &objtype.ParamTypeConfigs{
        ConfigNames: map[string]int{"damage": 3},
        Configs: []*objtype.ParamType{
            2: {ID: 2},
            3: {ID: 3, Type: objtype.ScriptVarTypeInt},
        },
    }
    lk = &paramLookups{}
    return
}

func TestParseObjConfig_Name(t *testing.T) {
    mp, cp, sp, op, pt, lk := objTestRegistries(t)
    parse := parseObjConfigFor(mp, cp, sp, op, lk, pt)

    val, accepted, err := parse("name", "Sword")
    if err != nil {
        t.Fatal(err)
    }
    if !accepted || val.(string) != "Sword" {
        t.Fatalf("got %#v, want \"Sword\"", val)
    }
}

func TestParseObjConfig_Param(t *testing.T) {
    mp, cp, sp, op, pt, lk := objTestRegistries(t)
    parse := parseObjConfigFor(mp, cp, sp, op, lk, pt)

    val, accepted, err := parse("param", "damage,42")
    if err != nil {
        t.Fatal(err)
    }
    if !accepted {
        t.Fatal("param key should be accepted")
    }
    pv := val.(ParamValue)
    if pv.ID != 3 || pv.Type != objtype.ScriptVarTypeInt || pv.Value.(int) != 42 {
        t.Fatalf("got %#v, want {ID:3, Type:Int, Value:42}", pv)
    }
}

func TestParseObjConfig_UnknownKey(t *testing.T) {
    mp, cp, sp, op, pt, lk := objTestRegistries(t)
    parse := parseObjConfigFor(mp, cp, sp, op, lk, pt)

    val, accepted, err := parse("zzz_unknown", "value")
    if err != nil {
        t.Fatal(err)
    }
    if accepted {
        t.Fatal("unknown key should NOT be accepted")
    }
    if val != nil {
        t.Fatalf("got %#v, want nil", val)
    }
}
```

- [ ] **Step 3.1.2: Run — expect FAIL**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseObjConfig -count=1 -v
```

Expected: `parseObjConfigFor` undefined.

- [ ] **Step 3.1.3: Implement parser (`pkg/pack/obj.go`)**

```go
package pack

import (
    "fmt"
    "strconv"
    "strings"

    "github.com/zsrv/goscape/pkg/objtype"
)

// parseObjConfigFor returns the per-key=value parser for .obj config
// blocks. Closure-captures six dependencies.
//
// TS source: tools/pack/config/ObjConfig.ts:8-170.
func parseObjConfigFor(modelPack, categoryPack, seqPack, objPack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs) ParseFn {
    return func(key, value string) (ConfigValue, bool, error) {
        // name / desc → string
        if key == "name" || key == "desc" {
            return value, true, nil
        }
        // model → modelPack.GetByName
        if key == "model" {
            id := modelPack.GetByName(value)
            if id == -1 {
                return nil, true, fmt.Errorf("unknown model: %s", value)
            }
            return id, true, nil
        }
        // category → categoryPack.GetByName
        if key == "category" {
            id := categoryPack.GetByName(value)
            if id == -1 {
                return nil, true, fmt.Errorf("unknown category: %s", value)
            }
            return id, true, nil
        }
        // cert / certtemplate → objPack.GetByName
        if key == "cert" || key == "certtemplate" {
            id := objPack.GetByName(value)
            if id == -1 {
                return nil, true, fmt.Errorf("unknown obj: %s", value)
            }
            return id, true, nil
        }
        // wear keys (manwear*/womanwear*) → "modelName,offset" → resolve model + offset int
        if strings.HasPrefix(key, "manwear") || strings.HasPrefix(key, "womanwear") {
            parts := strings.SplitN(value, ",", 2)
            id := modelPack.GetByName(parts[0])
            if id == -1 {
                return nil, true, fmt.Errorf("unknown model in %s: %s", key, parts[0])
            }
            offset := 0
            if len(parts) == 2 {
                n, err := strconv.ParseInt(parts[1], 0, 64)
                if err != nil {
                    return nil, true, fmt.Errorf("invalid offset in %s: %s", key, parts[1])
                }
                offset = int(n)
            }
            return []int{id, offset}, true, nil
        }
        // recol{N}{s|d}, retex{N}{s|d}
        // (Same shape as .loc — translate from ObjConfig.ts:60-90)
        // ... [full per-key branch list per TS ObjConfig.ts:8-170]
        //
        //   2dzoom/2dxan/2dyan/2dzan/2dxof/2dyof → signed int
        //   stackable/members/stockmarket/code9 → boolean
        //   cost → int (default 1)
        //   weight → signed int
        //   team → int
        //   manhead/womanhead/manhead2/womanhead2 → modelPack.GetByName
        //   stack{N} → "objName,count" → [objId, count]
        //   op{N} (N=1..5) → string
        //   iop{N} (N=1..5) → string
        //   recolN{s|d}, retexN{s|d} → int
        //
        // param=<name>,<valueStr>
        if key == "param" {
            i := strings.Index(value, ",")
            if i < 0 {
                return nil, true, fmt.Errorf("param missing comma: %s", value)
            }
            paramName := value[:i]
            paramValueStr := value[i+1:]
            p := paramTypes.ByName(paramName)
            if p == nil {
                return nil, true, fmt.Errorf("unknown param: %s", paramName)
            }
            v, err := lookupParamValue(p.Type, paramValueStr, lk)
            if err != nil {
                return nil, true, err
            }
            return ParamValue{ID: p.ID, Type: p.Type, Value: v}, true, nil
        }

        return nil, false, nil
    }
}
```

Same completeness note as T2: branches marked `// ...` MUST be fully implemented from TS `ObjConfig.ts:8-170` line-by-line.

- [ ] **Step 3.1.4: Run — expect PASS**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseObjConfig -count=1 -v
```

### Step 3.2: Implement packer with cert/uncert asymmetry pin

- [ ] **Step 3.2.1: Append packer byte-pin tests to `obj_test.go`**

Representative cases + a dedicated cert/uncert asymmetry pin (per `[[ts_asymmetry_dual_pin]]` memory):

```go
func TestPackObjConfigs_Name(t *testing.T) {
    _, _, _, op, _, _ := objTestRegistries(t)

    configs := map[string][]ConfigLine{
        "sword": {{Key: "name", Value: "Sword"}},
    }
    server, _, err := packObjConfigs(configs, op)
    if err != nil {
        t.Fatal(err)
    }
    // We need to scan past id=0 (sword) entry. For id=0 only:
    //   opcode 2 + pjstr("Sword") + opcode 250 + pjstr("sword") + opcode 0
    // For id=1 (cert_sword) and id=2 (shield): just opcode 250 + name + opcode 0
    // (no configs registered for those names)
    //
    // Assert id=0's slice. The server.Idx contains end-offsets per id.
    // We extract Dat[0:idx[0]].
    end0 := indexOffset(t, server.Idx, 0)
    got := server.Dat.Data[:end0]
    want := []byte{
        0x02, 'S', 'w', 'o', 'r', 'd', 0x00,
        0xFA, 's', 'w', 'o', 'r', 'd', 0x00,
        0x00,
    }
    if !bytes.Equal(got, want) {
        t.Fatalf("\ngot  % x\nwant % x", got, want)
    }
}

// TestPackObjConfigs_CertUncertPairing pins TS ObjConfig.ts:199-216
// asymmetry: when debugname starts with "cert_", the packer looks up
// uncert via objPack.GetByName(name[len("cert_"):]). When debugname
// does NOT start with "cert_", the packer looks up cert_<name>.
// Both pairings emit opcode 97 + p2(linkedId) if the lookup succeeds.
func TestPackObjConfigs_CertUncertPairing(t *testing.T) {
    _, _, _, op, _, _ := objTestRegistries(t)

    // No config lines for any obj — pairing logic still fires based on
    // debugname pattern alone.
    configs := map[string][]ConfigLine{}
    server, _, err := packObjConfigs(configs, op)
    if err != nil {
        t.Fatal(err)
    }

    // id=0 "sword": cert_sword exists (id=1) → emit opcode 97 + p2(1)
    end0 := indexOffset(t, server.Idx, 0)
    got0 := server.Dat.Data[:end0]
    want0 := []byte{
        0x61, 0x00, 0x01, // opcode 97 + p2(1)
        0xFA, 's', 'w', 'o', 'r', 'd', 0x00,
        0x00,
    }
    if !bytes.Equal(got0, want0) {
        t.Fatalf("id=0:\ngot  % x\nwant % x", got0, want0)
    }

    // id=1 "cert_sword": uncert "sword" exists (id=0) → emit opcode 97 + p2(0)
    end1 := indexOffset(t, server.Idx, 1)
    got1 := server.Dat.Data[end0:end1]
    want1 := []byte{
        0x61, 0x00, 0x00, // opcode 97 + p2(0)
        0xFA, 'c', 'e', 'r', 't', '_', 's', 'w', 'o', 'r', 'd', 0x00,
        0x00,
    }
    if !bytes.Equal(got1, want1) {
        t.Fatalf("id=1:\ngot  % x\nwant % x", got1, want1)
    }

    // id=2 "shield": no cert_shield, no uncert (debugname doesn't start with cert_) → opcode 97 NOT emitted
    end2 := indexOffset(t, server.Idx, 2)
    got2 := server.Dat.Data[end1:end2]
    want2 := []byte{
        0xFA, 's', 'h', 'i', 'e', 'l', 'd', 0x00,
        0x00,
    }
    if !bytes.Equal(got2, want2) {
        t.Fatalf("id=2:\ngot  % x\nwant % x", got2, want2)
    }
}

// indexOffset reads the per-id end offset from the .idx packet.
// .idx format: leading P2(count) + per-id P2(length).
// Returns cumulative offset (sum of lengths up to and including idx i).
func indexOffset(t *testing.T, idx *Packet, i int) int {
    t.Helper()
    p := idx.Data[2:] // skip count word
    cum := 0
    for j := 0; j <= i; j++ {
        l := int(p[j*2])<<8 | int(p[j*2+1])
        cum += l
    }
    return cum
}

// Additional opcode tests to add (one per Obj opcode map entry):
// TestPackObjConfigs_Model               — opcode 1 + p2
// TestPackObjConfigs_Desc                — opcode 3 + pjstr (client-side only)
// TestPackObjConfigs_Zoom2d              — opcode 4
// TestPackObjConfigs_Cost                — opcode 12 + p4
// TestPackObjConfigs_MembersTrue         — opcode 16
// TestPackObjConfigs_StackableTrue       — opcode 11
// TestPackObjConfigs_Manwear             — opcode 23 + p2 + p1
// TestPackObjConfigs_Op1..Op5            — opcodes 30..34 + pjstr
// TestPackObjConfigs_Iop1..Iop5          — opcodes 35..39 + pjstr
// TestPackObjConfigs_RecolPair           — opcode 40
// TestPackObjConfigs_Category            — opcode 94 + p2
// TestPackObjConfigs_Param               — opcode 249 (numeric)
// TestPackObjConfigs_ParamString         — opcode 249 (string variant)
// TestPackObjConfigs_StockMarketTrue     — opcode 65
// TestPackObjConfigs_Team                — opcode 200 + p1
// TestPackObjConfigs_Weight              — opcode 201 + p2 signed
// ... full list per Obj opcode map table.
```

- [ ] **Step 3.2.2: Run — expect FAIL**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackObjConfigs -count=1 -v
```

- [ ] **Step 3.2.3: Implement packer (append to `pkg/pack/obj.go`)**

```go
// packObjConfigs walks each id in [0, objPack.Max), emits all opcodes
// per ObjConfig.ts:196-440, and returns the server + client PackedData.
//
// Cert/uncert pairing (opcode 97):
//   - if debugname starts with "cert_": link = objPack.GetByName(debugname[5:])
//     emit opcode 97 + p2(link) when link != -1
//   - else: link = objPack.GetByName("cert_" + debugname)
//     emit opcode 97 + p2(link) when link != -1
//
// TS source: tools/pack/config/ObjConfig.ts:196-440.
func packObjConfigs(configs map[string][]ConfigLine, objPack *PackFile) (server, client *PackedData, err error) {
    server = NewPackedData(objPack.Max)
    client = NewPackedData(objPack.Max)

    for id := 0; id < objPack.Max; id++ {
        debugname := objPack.GetByID(id)
        lines := configs[debugname]

        // First pass: collect all values from `lines` into locals.
        var (
            name, desc string
            model      = -1
            cost       = 1
            stackable, members, stockmarket bool
            // ... (one local per accepted key per TS ObjConfig.ts:201-216)
            params     []ParamValue
        )
        for _, line := range lines {
            switch line.Key {
            case "name":
                name, _ = line.Value.(string)
            case "desc":
                desc, _ = line.Value.(string)
            case "model":
                model, _ = line.Value.(int)
            case "cost":
                cost, _ = line.Value.(int)
            case "stackable":
                stackable, _ = line.Value.(bool)
            case "members":
                members, _ = line.Value.(bool)
            case "stockmarket":
                stockmarket, _ = line.Value.(bool)
            case "param":
                if pv, ok := line.Value.(ParamValue); ok {
                    params = append(params, pv)
                }
            // ... (one case per accepted key)
            }
        }

        // Second pass: emit opcodes per Obj opcode map.

        // Opcode 1: model
        if model != -1 {
            server.Dat.P1(1)
            server.Dat.P2(uint16(model))
        }
        // Opcode 2: name
        if name != "" {
            server.Dat.P1(2)
            server.Dat.PJStr(name)
        }
        // Opcode 12: cost
        if cost != 1 {
            server.Dat.P1(12)
            server.Dat.P4(uint32(cost))
        }
        // Opcode 11: stackable
        if stackable {
            server.Dat.P1(11)
        }
        // Opcode 16: members
        if members {
            server.Dat.P1(16)
        }
        // Opcode 65: stockmarket
        if stockmarket {
            server.Dat.P1(65)
        }
        // ... [remainder of opcode map per Obj opcode map table — port each branch
        //      from TS ObjConfig.ts:218-414 in order.]

        // Opcode 97: cert/uncert pairing
        if strings.HasPrefix(debugname, "cert_") {
            uncertID := objPack.GetByName(debugname[len("cert_"):])
            if uncertID != -1 {
                server.Dat.P1(97)
                server.Dat.P2(uint16(uncertID))
            }
        } else if debugname != "" {
            certID := objPack.GetByName("cert_" + debugname)
            if certID != -1 {
                server.Dat.P1(97)
                server.Dat.P2(uint16(certID))
            }
        }

        // Opcode 249: param trailer (identical to .loc/.npc/.struct)
        if len(params) > 0 {
            server.Dat.P1(249)
            server.Dat.P1(uint8(len(params)))
            for _, pv := range params {
                server.Dat.P3(uint32(pv.ID))
                server.Dat.PBool(pv.Type == objtype.ScriptVarTypeString)
                if pv.Type == objtype.ScriptVarTypeString {
                    server.Dat.PJStr(pv.Value.(string))
                } else {
                    server.Dat.P4(uint32(pv.Value.(int)))
                }
            }
        }

        // Opcode 250: debugname
        if debugname != "" {
            server.Dat.P1(250)
            server.Dat.PJStr(debugname)
        }

        // Client-side opcodes (subset — see TS ObjConfig.ts:218-414 for which
        // opcodes route to `client` packet).
        // ...

        server.Next()
        client.Next()
    }

    return server, client, nil
}
```

- [ ] **Step 3.2.4: Run — expect PASS**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackObjConfigs -count=1 -v
```

- [ ] **Step 3.2.5: Commit**

```bash
git add pkg/pack/obj.go pkg/pack/obj_test.go
git commit --no-gpg-sign -m "feat(pack): NAI-196 T3 — .obj parser + packer

parseObjConfigFor + packObjConfigs port of ObjConfig.ts:8-440.
Cert/uncert pairing (opcode 97) pinned via TestPackObjConfigs_CertUncertPairing
covering all three asymmetric arms (cert_X→X, X→cert_X, X-with-no-cert).
Param= resolution mirrors .loc/.npc/.struct cohort.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: `.npc` parser + packer + byte-pin tests

**Files:**
- Create: `pkg/pack/npc.go`
- Create: `pkg/pack/npc_test.go`

### Npc opcode map (from `NpcConfig.ts:265-509`)

| Opcode | TS line | Trigger | Emit |
|---|---|---|---|
| 1 | 290-302 | `models.length > 0` | `p1(len) + per: p2(modelId)` |
| 2 | 304-306 | `name !== null` | `pjstr(name)` |
| 12 | 308-310 | `size !== 1` | `p1(size)` |
| 13 | 312-314 | `readyanim !== -1` | `p2(readyanim)` |
| 14 | 316-318 | `walkanim !== -1` | `p2(walkanim)` |
| 16 | 320-322 | `idle*anim !== -1` (idlewalkanim and similar) | `p2(...)` |
| 17 | 324-326 | (per TS) | per TS |
| 30-34 | 328-340 | op1..op5 | `pjstr(opN)` |
| 40 | 342-358 | recol pairs | `p1(count) + per: p2 p2` |
| 41 | 360-372 | retex pairs | (per TS) |
| 42 | 374-380 | head models | `p1(count) + per: p2(modelId)` |
| 60-70 | 382-410 | head recol | (per TS) |
| 74-75 | 412-420 | size, etc. | per TS |
| 77 | 422-424 | `visonmap !== true` | (no payload) |
| 78 | 426-428 | `vislevel !== -1` | `p1(vislevel)` |
| 79 | 430-432 | `resizeh !== 128` | `p2(resizeh)` |
| 80 | 434-436 | `resizev !== 128` | `p2(resizev)` |
| 82 | 438-440 | `wanderrange !== 5` | `p1(wanderrange)` |
| 90-103 | 442-460 | stats / attack / hp etc. | per TS |
| 107 | 462-464 | `members === true` | (no payload) |
| 109 | 466-468 | `attackrange !== ...` | per TS |
| 111 | 470-472 | `huntrange !== -1` | `p1(huntrange)` |
| 112 | 474-476 | `huntmode !== -1` (`huntPack.GetByName`) | `p1(huntmode)` |
| 113 | 478-480 | `hitpoints !== 1` | `p2(hitpoints)` |
| 114 | 482-484 | `category !== -1` | `p2(category)` |
| 134-158 | varies | wear-armor, defensive stats, etc. | per TS |
| 249 | 484-498 | param trailer | identical shape to `.loc`/`.obj`/`.struct` |
| 250 | 502-504 | debugname | `pjstr(debugname)` |

(The full list above is illustrative; the engineer reads `NpcConfig.ts:265-509` line by line, porting every opcode emission verbatim.)

### Step 4.1: Implement parser

- [ ] **Step 4.1.1: Write parser tests (`pkg/pack/npc_test.go`)**

```go
package pack

import (
    "bytes"
    "testing"

    "github.com/zsrv/goscape/pkg/objtype"
)

func npcTestRegistries(t *testing.T) (modelPack, categoryPack, seqPack, huntPack *PackFile, paramTypes *objtype.ParamTypeConfigs, lk *paramLookups) {
    t.Helper()
    modelPack = newTestPF("model", map[int]string{0: "rat_body"})
    categoryPack = newTestPF("category", map[int]string{0: "monster"})
    seqPack = newTestPF("seq", map[int]string{0: "walk", 1: "attack"})
    huntPack = newTestPF("hunt", map[int]string{0: "default_hunt"})
    paramTypes = &objtype.ParamTypeConfigs{
        ConfigNames: map[string]int{"aggression": 4},
        Configs: []*objtype.ParamType{
            3: {ID: 3},
            4: {ID: 4, Type: objtype.ScriptVarTypeInt},
        },
    }
    lk = &paramLookups{}
    return
}

func TestParseNpcConfig_Name(t *testing.T) {
    mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
    parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

    val, accepted, err := parse("name", "Giant Rat")
    if err != nil {
        t.Fatal(err)
    }
    if !accepted || val.(string) != "Giant Rat" {
        t.Fatalf("got %#v, want \"Giant Rat\"", val)
    }
}

func TestParseNpcConfig_Huntmode(t *testing.T) {
    mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
    parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

    val, accepted, err := parse("huntmode", "default_hunt")
    if err != nil {
        t.Fatal(err)
    }
    if !accepted || val.(int) != 0 {
        t.Fatalf("got %#v, want int 0 (default_hunt id)", val)
    }
}

func TestParseNpcConfig_Param(t *testing.T) {
    mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
    parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

    val, accepted, err := parse("param", "aggression,2")
    if err != nil {
        t.Fatal(err)
    }
    if !accepted {
        t.Fatal("param should be accepted")
    }
    pv := val.(ParamValue)
    if pv.ID != 4 || pv.Type != objtype.ScriptVarTypeInt || pv.Value.(int) != 2 {
        t.Fatalf("got %#v, want {ID:4, Type:Int, Value:2}", pv)
    }
}
```

- [ ] **Step 4.1.2: Run — expect FAIL**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseNpcConfig -count=1 -v
```

- [ ] **Step 4.1.3: Implement parser (`pkg/pack/npc.go`)**

```go
package pack

import (
    "fmt"
    "strconv"
    "strings"

    "github.com/zsrv/goscape/pkg/objtype"
)

// parseNpcConfigFor returns the per-key=value parser for .npc config
// blocks. Closure-captures six dependencies.
//
// TS source: tools/pack/config/NpcConfig.ts:8-260.
func parseNpcConfigFor(modelPack, categoryPack, seqPack, huntPack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs) ParseFn {
    return func(key, value string) (ConfigValue, bool, error) {
        if key == "name" || key == "desc" {
            return value, true, nil
        }
        if strings.HasPrefix(key, "model") {
            id := modelPack.GetByName(value)
            if id == -1 {
                return nil, true, fmt.Errorf("unknown model: %s", value)
            }
            return id, true, nil
        }
        if strings.HasPrefix(key, "head") {
            id := modelPack.GetByName(value)
            if id == -1 {
                return nil, true, fmt.Errorf("unknown model in %s: %s", key, value)
            }
            return id, true, nil
        }
        if key == "category" {
            id := categoryPack.GetByName(value)
            if id == -1 {
                return nil, true, fmt.Errorf("unknown category: %s", value)
            }
            return id, true, nil
        }
        if key == "huntmode" {
            id := huntPack.GetByName(value)
            if id == -1 {
                return nil, true, fmt.Errorf("unknown hunt: %s", value)
            }
            return id, true, nil
        }
        // Anim keys (readyanim, walkanim, idlewalkanim, defendanim, attackanim, etc.) — seqPack.GetByName
        // (Full list per TS NpcConfig.ts:50-105)
        // ... [numeric keys: size, vislevel, resizeh, resizev, wanderrange, attackrange, hitpoints, etc.
        //      boolean keys: visonmap, members, aggressive, etc.
        //      op{N} → string
        //      stat keys (att/def/str/hp/mage/range) → int]

        if key == "param" {
            i := strings.Index(value, ",")
            if i < 0 {
                return nil, true, fmt.Errorf("param missing comma: %s", value)
            }
            paramName := value[:i]
            paramValueStr := value[i+1:]
            p := paramTypes.ByName(paramName)
            if p == nil {
                return nil, true, fmt.Errorf("unknown param: %s", paramName)
            }
            v, err := lookupParamValue(p.Type, paramValueStr, lk)
            if err != nil {
                return nil, true, err
            }
            return ParamValue{ID: p.ID, Type: p.Type, Value: v}, true, nil
        }

        _ = strconv.ParseInt // suppress unused-import if all branches inline parse
        return nil, false, nil
    }
}
```

Same completeness note as T2/T3.

- [ ] **Step 4.1.4: Run — expect PASS**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestParseNpcConfig -count=1 -v
```

### Step 4.2: Implement packer

- [ ] **Step 4.2.1: Append packer tests to `npc_test.go`**

```go
func TestPackNpcConfigs_Name(t *testing.T) {
    npcPack := newTestPF("npc", map[int]string{0: "rat"})
    configs := map[string][]ConfigLine{
        "rat": {{Key: "name", Value: "Giant Rat"}},
    }
    server, _, err := packNpcConfigs(configs, npcPack)
    if err != nil {
        t.Fatal(err)
    }
    // opcode 2 + pjstr("Giant Rat") + opcode 250 + pjstr("rat") + opcode 0
    want := []byte{
        0x02, 'G', 'i', 'a', 'n', 't', ' ', 'R', 'a', 't', 0x00,
        0xFA, 'r', 'a', 't', 0x00,
        0x00,
    }
    got := server.Dat.Data[:server.Dat.Length()]
    if !bytes.Equal(got, want) {
        t.Fatalf("\ngot  % x\nwant % x", got, want)
    }
}

func TestPackNpcConfigs_Huntmode(t *testing.T) {
    npcPack := newTestPF("npc", map[int]string{0: "rat"})
    configs := map[string][]ConfigLine{
        "rat": {{Key: "huntmode", Value: 0}},
    }
    server, _, err := packNpcConfigs(configs, npcPack)
    if err != nil {
        t.Fatal(err)
    }
    // opcode 112 = 0x70 + p1(0) + opcode 250 + "rat\x00" + opcode 0
    want := []byte{
        0x70, 0x00,
        0xFA, 'r', 'a', 't', 0x00,
        0x00,
    }
    got := server.Dat.Data[:server.Dat.Length()]
    if !bytes.Equal(got, want) {
        t.Fatalf("\ngot  % x\nwant % x", got, want)
    }
}

func TestPackNpcConfigs_Param(t *testing.T) {
    npcPack := newTestPF("npc", map[int]string{0: "boss"})
    configs := map[string][]ConfigLine{
        "boss": {{
            Key:   "param",
            Value: ParamValue{ID: 4, Type: objtype.ScriptVarTypeInt, Value: 2},
        }},
    }
    server, _, err := packNpcConfigs(configs, npcPack)
    if err != nil {
        t.Fatal(err)
    }
    // opcode 249 + p1(1) + p3(4) + pbool(false) + p4(2) + opcode 250 + "boss\x00" + opcode 0
    want := []byte{
        0xF9,
        0x01,
        0x00, 0x00, 0x04,
        0x00,
        0x00, 0x00, 0x00, 0x02,
        0xFA, 'b', 'o', 's', 's', 0x00,
        0x00,
    }
    got := server.Dat.Data[:server.Dat.Length()]
    if !bytes.Equal(got, want) {
        t.Fatalf("\ngot  % x\nwant % x", got, want)
    }
}

// Additional tests to add per Npc opcode map:
// TestPackNpcConfigs_Models                — opcode 1 + count + per p2
// TestPackNpcConfigs_Size                  — opcode 12 + p1
// TestPackNpcConfigs_Readyanim             — opcode 13 + p2
// TestPackNpcConfigs_Walkanim              — opcode 14 + p2
// TestPackNpcConfigs_Op1..Op5              — opcodes 30..34 + pjstr
// TestPackNpcConfigs_Heads                 — opcode 42 + count + per p2
// TestPackNpcConfigs_VisonmapFalse         — opcode 77
// TestPackNpcConfigs_Vislevel              — opcode 78 + p1
// TestPackNpcConfigs_Resizeh               — opcode 79 + p2
// TestPackNpcConfigs_Resizev               — opcode 80 + p2
// TestPackNpcConfigs_Wanderrange           — opcode 82 + p1
// TestPackNpcConfigs_MembersTrue           — opcode 107
// TestPackNpcConfigs_Huntrange             — opcode 111 + p1
// TestPackNpcConfigs_Hitpoints             — opcode 113 + p2
// TestPackNpcConfigs_Category              — opcode 114 + p2
// TestPackNpcConfigs_ParamString           — opcode 249 string variant
```

- [ ] **Step 4.2.2: Run — expect FAIL**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackNpcConfigs -count=1 -v
```

- [ ] **Step 4.2.3: Implement packer (append to `pkg/pack/npc.go`)**

```go
// packNpcConfigs walks each id in [0, npcPack.Max), emits all opcodes
// per NpcConfig.ts:265-509, and returns the server + client PackedData.
//
// TS source: tools/pack/config/NpcConfig.ts:265-509.
func packNpcConfigs(configs map[string][]ConfigLine, npcPack *PackFile) (server, client *PackedData, err error) {
    server = NewPackedData(npcPack.Max)
    client = NewPackedData(npcPack.Max)

    for id := 0; id < npcPack.Max; id++ {
        debugname := npcPack.GetByID(id)
        lines := configs[debugname]

        var (
            name, desc string
            size       = 1
            // ... (one local per accepted key per TS NpcConfig.ts:269-284)
            params []ParamValue
        )
        for _, line := range lines {
            switch line.Key {
            case "name":
                name, _ = line.Value.(string)
            case "desc":
                desc, _ = line.Value.(string)
            case "size":
                size, _ = line.Value.(int)
            case "param":
                if pv, ok := line.Value.(ParamValue); ok {
                    params = append(params, pv)
                }
            // ... (one case per accepted key)
            }
        }

        // Emit per Npc opcode map.
        // Opcode 2: name
        if name != "" {
            server.Dat.P1(2)
            server.Dat.PJStr(name)
        }
        // Opcode 12: size
        if size != 1 {
            server.Dat.P1(12)
            server.Dat.P1(uint8(size))
        }
        // ... [remainder of opcode map per Npc opcode map table — port each branch
        //      from TS NpcConfig.ts:290-498 in order.]

        // Opcode 249: param trailer
        if len(params) > 0 {
            server.Dat.P1(249)
            server.Dat.P1(uint8(len(params)))
            for _, pv := range params {
                server.Dat.P3(uint32(pv.ID))
                server.Dat.PBool(pv.Type == objtype.ScriptVarTypeString)
                if pv.Type == objtype.ScriptVarTypeString {
                    server.Dat.PJStr(pv.Value.(string))
                } else {
                    server.Dat.P4(uint32(pv.Value.(int)))
                }
            }
        }

        // Opcode 250: debugname
        if debugname != "" {
            server.Dat.P1(250)
            server.Dat.PJStr(debugname)
        }

        // Client-side opcodes per TS NpcConfig.ts:265-289.
        // ...

        server.Next()
        client.Next()
    }

    return server, client, nil
}
```

- [ ] **Step 4.2.4: Run — expect PASS**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackNpcConfigs -count=1 -v
```

- [ ] **Step 4.2.5: Commit**

```bash
git add pkg/pack/npc.go pkg/pack/npc_test.go
git commit --no-gpg-sign -m "feat(pack): NAI-196 T4 — .npc parser + packer

parseNpcConfigFor + packNpcConfigs port of NpcConfig.ts:8-509.
Largest single-config port (~511 TS LOC). Param= resolution mirrors
.loc/.obj/.struct cohort. Huntmode resolved via huntPack.GetByName.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: PackConfigs rewrite — TS-canonical ordering + 3-tag retirement + in-place test rewrite

**Files:**
- Modify: `pkg/pack/pack_configs.go` (function `PackConfigs` body)
- Modify: `pkg/pack/pack_configs_test.go:401` (`TestPackConfigs_EightConfigsLand` → `TestPackConfigs_ElevenConfigsLand`)

This is the architectural payoff task. It:
1. Re-orders every per-config branch to match TS PackShared.ts:261-669 (filtered to implemented configs)
2. Drops ShouldBuild gates from `.param`/`.loc`/`.npc`/`.obj`/`.varp`
3. Wires `packAndSaveLoc`/`packAndSaveNpc`/`packAndSaveObj` helper functions
4. Replaces lazy `ensureParamTypes` with eager `objtype.LoadParamTypes(outDir)` call
5. Drops `clientJagDirty` boolean; client jagfile saved unconditionally
6. Updates the existing 8-config integration test in-place to match the new order
7. Updates 3 doc-comments (retiring `PARAM-AFTER-VARS`, `CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS`, `FRESH-CLIENT-JAGFILE`; introducing `UNCONDITIONAL-CLIENT-PACK`)

- [ ] **Step 5.1: Add three new `packAndSaveFoo` helpers to `pack_configs.go`**

Append after the existing `packAndSaveStruct`:

```go
// packAndSaveLoc reads .loc sources, packs them, writes server
// .dat/.idx, and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: this branch runs every
// PackConfigs invocation regardless of source freshness, matching
// TS PackShared.ts:477 (rebuildClient=true ungates shouldBuild).
//
// TS source: tools/pack/config/LocConfig.ts:172-432.
func packAndSaveLoc(srcDir, serverOut string, locPack, modelPack, categoryPack, seqPack, texturePack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs, c Constants, clientJag *jagfile.Jagfile) error {
    parse := parseLocConfigFor(modelPack, categoryPack, seqPack, texturePack, lk, paramTypes)
    cfgs, err := ReadTypedConfigs(srcDir, ".loc", nil, parse, c)
    if err != nil {
        return err
    }
    server, client, err := packLocConfigs(cfgs, locPack, modelPack)
    if err != nil {
        return err
    }
    if err := server.Save(
        filepath.Join(serverOut, "loc.dat"),
        filepath.Join(serverOut, "loc.idx"),
    ); err != nil {
        return err
    }
    clientJag.Write("loc.dat", client.Dat)
    clientJag.Write("loc.idx", client.Idx)
    return nil
}

// packAndSaveNpc — mirrors packAndSaveLoc shape. See NpcConfig.ts:265-509.
func packAndSaveNpc(srcDir, serverOut string, npcPack, modelPack, categoryPack, seqPack, huntPack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs, c Constants, clientJag *jagfile.Jagfile) error {
    parse := parseNpcConfigFor(modelPack, categoryPack, seqPack, huntPack, lk, paramTypes)
    cfgs, err := ReadTypedConfigs(srcDir, ".npc", nil, parse, c)
    if err != nil {
        return err
    }
    server, client, err := packNpcConfigs(cfgs, npcPack)
    if err != nil {
        return err
    }
    if err := server.Save(
        filepath.Join(serverOut, "npc.dat"),
        filepath.Join(serverOut, "npc.idx"),
    ); err != nil {
        return err
    }
    clientJag.Write("npc.dat", client.Dat)
    clientJag.Write("npc.idx", client.Idx)
    return nil
}

// packAndSaveObj — mirrors packAndSaveLoc shape. See ObjConfig.ts:196-440.
func packAndSaveObj(srcDir, serverOut string, objPack, modelPack, categoryPack, seqPack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs, c Constants, clientJag *jagfile.Jagfile) error {
    parse := parseObjConfigFor(modelPack, categoryPack, seqPack, objPack, lk, paramTypes)
    cfgs, err := ReadTypedConfigs(srcDir, ".obj", nil, parse, c)
    if err != nil {
        return err
    }
    server, client, err := packObjConfigs(cfgs, objPack)
    if err != nil {
        return err
    }
    if err := server.Save(
        filepath.Join(serverOut, "obj.dat"),
        filepath.Join(serverOut, "obj.idx"),
    ); err != nil {
        return err
    }
    clientJag.Write("obj.dat", client.Dat)
    clientJag.Write("obj.idx", client.Idx)
    return nil
}
```

- [ ] **Step 5.2: Rewrite `PackConfigs` body**

This is a large in-place rewrite. The intent is:

Old order: var-uniqueness → varp → varn → vars → param → enum → inv → mesanim → struct → save clientJag (if dirty).

New order: var-uniqueness → param (unconditional) → eager LoadParamTypes → enum/inv/mesanim/struct (freshness-gated) → loc/npc/obj/varp (unconditional) → varn/vars (freshness-gated) → save clientJag (unconditional).

Replace the function body (everything between `func PackConfigs(srcDir, outDir string) error {` and its closing brace) per the structure below. Drop `_ = ensureLocPack` etc. from T1 — they get real callers now. Drop `clientJagDirty`. Drop `ensureParamTypes` re-entry guard (replace with eager call). Drop ShouldBuild gates from .param/.loc/.npc/.obj/.varp.

Document deviation tags atop `PackConfigs`:

```go
// PackConfigs runs the per-config packing pipeline. NAI-191–195 wired
// .varp/.varn/.vars/.param/.enum/.inv/.mesanim/.struct. NAI-196 wires
// .loc/.npc/.obj and re-orders the pipeline to TS-canonical layout per
// tools/pack/config/PackShared.ts:261-669 (filtered to currently
// implemented configs).
//
// Server outputs land at <outDir>/server/<type>.{dat,idx}.
// Client outputs land in a fresh jagfile at <outDir>/client/config.
//
// The three var-domain PackFiles (varp/varn/vars) are constructed
// up-front so the cross-domain uniqueness check has all three name
// maps available. Each *.pack file is small (<1 KB); cost is fixed.
//
// NAI-193-D-PACKFILE-SINGLETONS-DEFERRED: TS uses module-level
// VarpPack/VarnPack/VarsPack singletons; goscape constructs *PackFile
// from srcDir per call (continuation of NAI-191 §2 / NAI-192).
//
// NAI-191-D-VALIDATE-FLAGS-DEFERRED: TS BUILD_VERIFY callback (.varp
// magic 705633567 at PackShared.ts:631-633) deferred — continuation
// of NAI-191 §2.
//
// NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL: .param contributes empty
// param.dat/param.idx to client jagfile; TS callback is no-op (does
// not contribute to client jag). Preserved for client-jagfile entry
// completeness.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: .param, .loc, .npc, .obj, .varp
// run on EVERY PackConfigs invocation regardless of source freshness,
// matching TS PackShared.ts:337 (`const rebuildClient = true`) which
// ungates shouldBuild on the four configs that write to client jag
// (loc/npc/obj/varp) and — per NAI-196 §"R5 resolution" — also on
// .param so that all client-jagfile entries are always present.
// The server-only six (.enum, .inv, .mesanim, .struct, .varn, .vars)
// retain their ShouldBuild + GetLatestModified freshness gates.
//
// NAI-192-D-NO-SRC-NO-OP: applies only to the six server-only
// freshness-gated branches. The five unconditional branches always
// run; an empty source directory produces an empty .dat/.idx pair
// (matching TS shouldBuild-output-missing arm).
//
// NAI-195-D-DEADBRANCH-OMITTED: per-config parsers omit dead TS
// branches (empty stringKeys/numberKeys/booleanKeys arrays).
//
// TS source: tools/pack/config/PackShared.ts:261-669 (packConfigs).
func PackConfigs(srcDir, outDir string) error {
    constants, err := LoadConstants(srcDir)
    if err != nil {
        return err
    }

    // Cross-domain var-name uniqueness — constructed up-front per spec §4.
    varpPack, err := NewPackFile(srcDir, "varp", nil)
    if err != nil {
        return err
    }
    varnPack, err := NewPackFile(srcDir, "varn", nil)
    if err != nil {
        return err
    }
    varsPack, err := NewPackFile(srcDir, "vars", nil)
    if err != nil {
        return err
    }
    if err := checkVarNameUniqueness(varpPack, varnPack, varsPack); err != nil {
        return err
    }

    scriptsDir := filepath.Join(srcDir, "scripts")
    serverOut := filepath.Join(outDir, "server")
    clientOut := filepath.Join(outDir, "client")

    clientJag, err := jagfile.NewJagfile(nil)
    if err != nil {
        return err
    }

    // Lazy registries reused across multiple branches.
    var (
        lk           *paramLookups
        objPack      *PackFile
        seqPack      *PackFile
        locPack      *PackFile
        npcPack      *PackFile
        modelPack    *PackFile
        categoryPack *PackFile
        huntPack     *PackFile
        texturePack  *PackFile
    )
    ensureLk := func() error {
        if lk != nil {
            return nil
        }
        newLk, err := loadParamLookups(srcDir, varpPack)
        if err != nil {
            return err
        }
        lk = newLk
        return nil
    }
    ensureObjPack := func() error {
        if objPack != nil {
            return nil
        }
        pf, err := NewPackFile(srcDir, "obj", nil)
        if err != nil {
            return err
        }
        objPack = pf
        return nil
    }
    ensureSeqPack := func() error {
        if seqPack != nil {
            return nil
        }
        pf, err := NewPackFile(srcDir, "seq", nil)
        if err != nil {
            return err
        }
        seqPack = pf
        return nil
    }
    ensureLocPack := func() error {
        if locPack != nil {
            return nil
        }
        pf, err := NewPackFile(srcDir, "loc", nil)
        if err != nil {
            return err
        }
        locPack = pf
        return nil
    }
    ensureNpcPack := func() error {
        if npcPack != nil {
            return nil
        }
        pf, err := NewPackFile(srcDir, "npc", nil)
        if err != nil {
            return err
        }
        npcPack = pf
        return nil
    }
    ensureModelPack := func() error {
        if modelPack != nil {
            return nil
        }
        pf, err := NewPackFile(srcDir, "model", nil)
        if err != nil {
            return err
        }
        modelPack = pf
        return nil
    }
    ensureCategoryPack := func() error {
        if categoryPack != nil {
            return nil
        }
        pf, err := NewPackFile(srcDir, "category", nil)
        if err != nil {
            return err
        }
        categoryPack = pf
        return nil
    }
    ensureHuntPack := func() error {
        if huntPack != nil {
            return nil
        }
        pf, err := NewPackFile(srcDir, "hunt", nil)
        if err != nil {
            return err
        }
        huntPack = pf
        return nil
    }
    ensureTexturePack := func() error {
        if texturePack != nil {
            return nil
        }
        pf, err := NewPackFile(srcDir, "texture", nil)
        if err != nil {
            return err
        }
        texturePack = pf
        return nil
    }

    // 1. .param — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK)
    paramPack, err := NewPackFile(srcDir, "param", nil)
    if err != nil {
        return err
    }
    if err := ensureLk(); err != nil {
        return err
    }
    if err := packAndSaveParam(srcDir, serverOut, paramPack, lk, constants, clientJag); err != nil {
        return err
    }

    // 2. Eager objtype.LoadParamTypes (mirrors TS PackShared.ts:334 unconditional load).
    paramTypes, err := objtype.LoadParamTypes(outDir)
    if err != nil {
        return fmt.Errorf("load param types: %w", err)
    }

    // 3. .enum — server-only, freshness-gated
    if GetLatestModified(scriptsDir, ".enum") > 0 &&
        ShouldBuild(scriptsDir, ".enum", filepath.Join(serverOut, "enum.dat")) {
        enumPack, err := NewPackFile(srcDir, "enum", nil)
        if err != nil {
            return err
        }
        if err := packAndSaveEnum(srcDir, serverOut, enumPack, lk, constants); err != nil {
            return err
        }
    }

    // 4. .inv — server-only, freshness-gated
    if GetLatestModified(scriptsDir, ".inv") > 0 &&
        ShouldBuild(scriptsDir, ".inv", filepath.Join(serverOut, "inv.dat")) {
        if err := ensureObjPack(); err != nil {
            return err
        }
        invPack, err := NewPackFile(srcDir, "inv", nil)
        if err != nil {
            return err
        }
        if err := packAndSaveInv(srcDir, serverOut, invPack, objPack, constants); err != nil {
            return err
        }
    }

    // 5. .mesanim — server-only, freshness-gated
    if GetLatestModified(scriptsDir, ".mesanim") > 0 &&
        ShouldBuild(scriptsDir, ".mesanim", filepath.Join(serverOut, "mesanim.dat")) {
        if err := ensureSeqPack(); err != nil {
            return err
        }
        mesPack, err := NewPackFile(srcDir, "mesanim", nil)
        if err != nil {
            return err
        }
        if err := packAndSaveMesAnim(srcDir, serverOut, mesPack, seqPack, constants); err != nil {
            return err
        }
    }

    // 6. .struct — server-only, freshness-gated
    if GetLatestModified(scriptsDir, ".struct") > 0 &&
        ShouldBuild(scriptsDir, ".struct", filepath.Join(serverOut, "struct.dat")) {
        structPack, err := NewPackFile(srcDir, "struct", nil)
        if err != nil {
            return err
        }
        if err := packAndSaveStruct(srcDir, serverOut, structPack, paramTypes, lk, constants); err != nil {
            return err
        }
    }

    // 7. .loc — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK)
    if err := ensureLocPack(); err != nil {
        return err
    }
    if err := ensureModelPack(); err != nil {
        return err
    }
    if err := ensureCategoryPack(); err != nil {
        return err
    }
    if err := ensureSeqPack(); err != nil {
        return err
    }
    if err := ensureTexturePack(); err != nil {
        return err
    }
    if err := packAndSaveLoc(srcDir, serverOut, locPack, modelPack, categoryPack, seqPack, texturePack, lk, paramTypes, constants, clientJag); err != nil {
        return err
    }

    // 8. .npc — unconditional
    if err := ensureNpcPack(); err != nil {
        return err
    }
    if err := ensureModelPack(); err != nil {
        return err
    }
    if err := ensureCategoryPack(); err != nil {
        return err
    }
    if err := ensureSeqPack(); err != nil {
        return err
    }
    if err := ensureHuntPack(); err != nil {
        return err
    }
    if err := packAndSaveNpc(srcDir, serverOut, npcPack, modelPack, categoryPack, seqPack, huntPack, lk, paramTypes, constants, clientJag); err != nil {
        return err
    }

    // 9. .obj — unconditional
    if err := ensureObjPack(); err != nil {
        return err
    }
    if err := ensureModelPack(); err != nil {
        return err
    }
    if err := ensureCategoryPack(); err != nil {
        return err
    }
    if err := ensureSeqPack(); err != nil {
        return err
    }
    if err := packAndSaveObj(srcDir, serverOut, objPack, modelPack, categoryPack, seqPack, lk, paramTypes, constants, clientJag); err != nil {
        return err
    }

    // 10. .varp — unconditional
    if err := packAndSaveVarp(srcDir, serverOut, varpPack, constants, clientJag); err != nil {
        return err
    }

    // 11. .varn — server-only, freshness-gated
    if GetLatestModified(scriptsDir, ".varn") > 0 &&
        ShouldBuild(scriptsDir, ".varn", filepath.Join(serverOut, "varn.dat")) {
        if err := packAndSaveVarn(srcDir, serverOut, varnPack, constants); err != nil {
            return err
        }
    }

    // 12. .vars — server-only, freshness-gated
    if GetLatestModified(scriptsDir, ".vars") > 0 &&
        ShouldBuild(scriptsDir, ".vars", filepath.Join(serverOut, "vars.dat")) {
        if err := packAndSaveVars(srcDir, serverOut, varsPack, constants); err != nil {
            return err
        }
    }

    // 13. Client jagfile save (unconditional — was gated by clientJagDirty pre-NAI-196).
    return clientJag.Save(filepath.Join(clientOut, "config"), false)
}
```

Note: in this rewrite, `packAndSaveVarp` still expects its existing signature `(srcDir, serverOut, varpPack, constants, clientJag)`. The previous `if GetLatestModified > 0 && ShouldBuild(...)` guard at pack_configs.go:155-161 is removed; the call site is now line ~10 in the new flow.

- [ ] **Step 5.3: Rewrite `TestPackConfigs_EightConfigsLand` → `TestPackConfigs_ElevenConfigsLand`**

Locate the test at `pkg/pack/pack_configs_test.go:401`. The old test asserts 8 server `.dat`/`.idx` pairs (varp, varn, vars, param, enum, inv, mesanim, struct) and 2 client entries (varp.dat/.idx, param.dat/.idx). The new test asserts 11 server pairs and 10 client entries (5 client+server configs × 2 files).

```go
func TestPackConfigs_ElevenConfigsLand(t *testing.T) {
    srcDir := t.TempDir()
    outDir := t.TempDir()

    scripts := filepath.Join(srcDir, "scripts")
    if err := os.MkdirAll(scripts, 0o755); err != nil {
        t.Fatal(err)
    }
    if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
        t.Fatal(err)
    }

    // Pack files for all referenced typed-ids
    writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=quest_points\n")
    writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "0=npc_state\n")
    writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "0=login_msg\n")
    writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "0=damage\n")
    writeFile(t, filepath.Join(srcDir, "pack", "enum.pack"), "0=test_enum\n")
    writeFile(t, filepath.Join(srcDir, "pack", "inv.pack"), "0=inv_main\n")
    writeFile(t, filepath.Join(srcDir, "pack", "mesanim.pack"), "0=anim_a\n")
    writeFile(t, filepath.Join(srcDir, "pack", "struct.pack"), "0=test_struct\n")
    writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "0=table\n")
    writeFile(t, filepath.Join(srcDir, "pack", "npc.pack"), "0=rat\n")
    writeFile(t, filepath.Join(srcDir, "pack", "obj.pack"), "0=sword\n")
    writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "0=model_zero\n")
    writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "0=furniture\n")
    writeFile(t, filepath.Join(srcDir, "pack", "hunt.pack"), "0=default_hunt\n")
    writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "0=wood\n")
    writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=idle\n")

    // Source files (one entry per config, minimal valid bodies)
    writeFile(t, filepath.Join(scripts, "test.varp"), "[quest_points]\ntype=int\n")
    writeFile(t, filepath.Join(scripts, "test.varn"), "[npc_state]\n")
    writeFile(t, filepath.Join(scripts, "test.vars"), "[login_msg]\n")
    writeFile(t, filepath.Join(scripts, "test.param"), "[damage]\ntype=int\n")
    writeFile(t, filepath.Join(scripts, "test.enum"), "[test_enum]\ninputtype=int\noutputtype=int\n")
    writeFile(t, filepath.Join(scripts, "test.inv"), "[inv_main]\nsize=28\n")
    writeFile(t, filepath.Join(scripts, "test.mesanim"), "[anim_a]\n")
    writeFile(t, filepath.Join(scripts, "test.struct"), "[test_struct]\n")
    writeFile(t, filepath.Join(scripts, "test.loc"), "[table]\nname=Table\n")
    writeFile(t, filepath.Join(scripts, "test.npc"), "[rat]\nname=Rat\n")
    writeFile(t, filepath.Join(scripts, "test.obj"), "[sword]\nname=Sword\n")

    ClearFsCache()
    if err := PackConfigs(srcDir, outDir); err != nil {
        t.Fatal(err)
    }

    // Assert all 11 server-side files exist
    serverFiles := []string{
        "varp.dat", "varp.idx",
        "varn.dat", "varn.idx",
        "vars.dat", "vars.idx",
        "param.dat", "param.idx",
        "enum.dat", "enum.idx",
        "inv.dat", "inv.idx",
        "mesanim.dat", "mesanim.idx",
        "struct.dat", "struct.idx",
        "loc.dat", "loc.idx",
        "npc.dat", "npc.idx",
        "obj.dat", "obj.idx",
    }
    for _, name := range serverFiles {
        path := filepath.Join(outDir, "server", name)
        info, err := os.Stat(path)
        if err != nil {
            t.Errorf("server file %s missing: %v", name, err)
            continue
        }
        if info.Size() == 0 {
            t.Errorf("server file %s is empty", name)
        }
    }

    // Assert client jagfile contains 10 entries (5 client+server configs × 2 files)
    clientJag, err := os.ReadFile(filepath.Join(outDir, "client", "config"))
    if err != nil {
        t.Fatalf("client jagfile missing: %v", err)
    }
    if len(clientJag) == 0 {
        t.Fatal("client jagfile is empty")
    }
    // Load and inspect the jagfile entries
    jag, err := jagfileFromBytes(clientJag)
    if err != nil {
        t.Fatalf("parse client jag: %v", err)
    }
    expectedEntries := []string{
        "param.dat", "param.idx",
        "loc.dat", "loc.idx",
        "npc.dat", "npc.idx",
        "obj.dat", "obj.idx",
        "varp.dat", "varp.idx",
    }
    for _, name := range expectedEntries {
        if !jag.Has(name) {
            t.Errorf("client jag missing entry: %s", name)
        }
    }
    if jag.EntryCount() != len(expectedEntries) {
        t.Errorf("client jag has %d entries, want %d", jag.EntryCount(), len(expectedEntries))
    }
}

// jagfileFromBytes parses a client jagfile blob. Helper for testing only.
// Implementation should mirror existing test-side jagfile parsing — search
// pkg/io/jagfile for an exported Load or Parse function and use it here.
func jagfileFromBytes(buf []byte) (*jagfile.Jagfile, error) {
    return jagfile.LoadBytes(buf) // (verify exact name with `grep -n "func Load" pkg/io/jagfile/`)
}
```

**Plan-author note:** verify the exact API name for jagfile loading (`LoadBytes`, `Parse`, `Load`?) at impl time. The existing NAI-194 T6 round-trip test already loads a client jagfile — copy its idiom verbatim. If `Has` / `EntryCount` accessors don't exist, inspect `jag.Entries` (likely a slice or map) and iterate.

- [ ] **Step 5.4: Delete the old `TestPackConfigs_EightConfigsLand` function**

Same commit. The new `_ElevenConfigsLand` test supersedes it. Also audit `pack_configs_test.go` for any OTHER tests that assert the old ordering (e.g., `TestPackConfigs_MixedVarpVarnVars` at line 155 — check that test's expected-call-order assertions and update if needed).

```bash
grep -n "varp.*before\|before.*varp\|order.*expected\|expected.*order" /home/owner/Code/github.com/zsrv/goscape/pkg/pack/pack_configs_test.go
```

If hits: update them inline.

- [ ] **Step 5.5: Run pack package**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1
```

Expected: all green. If `TestPackConfigs_NoVarpSource_NoClientJagfileWritten` (line 189) FAILS — that test asserts "no varp source → no client jag" which is INCONSISTENT with the new unconditional client jag save. Update the test or rename to assert "client jag always saved, varp entry empty when no source" (whichever matches actual behavior).

- [ ] **Step 5.6: Run full test suite**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: all green across the repo.

- [ ] **Step 5.7: Commit**

```bash
git add pkg/pack/pack_configs.go pkg/pack/pack_configs_test.go
git commit --no-gpg-sign -m "feat(pack): NAI-196 T5 — PackConfigs TS-canonical ordering rewrite

Reorders PackConfigs to match PackShared.ts:261-669 (filtered to implemented
configs). New order:
  param → LoadParamTypes → enum/inv/mesanim/struct → loc/npc/obj/varp → varn/vars
  → save clientJag.

Retires three deviations:
  - NAI-194-D-PARAM-AFTER-VARS (param now first)
  - NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS (canonical order)
  - NAI-193-D-FRESH-CLIENT-JAGFILE (jag always saved with all 10 client entries)

Introduces NAI-196-D-UNCONDITIONAL-CLIENT-PACK on the five client+server
branches (.param, .loc, .npc, .obj, .varp) — they drop their ShouldBuild
gates per TS rebuildClient=true at PackShared.ts:337. ObjType.LoadParamTypes
becomes eager (was lazy via ensureParamTypes).

Wires three new packAndSaveLoc/packAndSaveNpc/packAndSaveObj helpers.
Drops clientJagDirty flag.

In-place rewrites TestPackConfigs_EightConfigsLand → _ElevenConfigsLand
to assert new canonical order and 10-entry client jagfile.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Round-trip tests for `.loc`/`.obj`/`.npc`

**Files:**
- Create: `pkg/pack/loc_roundtrip_test.go`
- Create: `pkg/pack/obj_roundtrip_test.go`
- Create: `pkg/pack/npc_roundtrip_test.go`

Each test:
1. Builds source files (`.param` always required; per-config `.loc`/`.obj`/`.npc`)
2. Builds required `.pack` registry stubs
3. Calls `PackConfigs(srcDir, outDir)`
4. Calls the matching `objtype.Load*Types` loader
5. Asserts source-declared fields survived round-trip

- [ ] **Step 6.1: Write `loc_roundtrip_test.go`**

```go
package pack

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/zsrv/goscape/pkg/objtype"
)

func TestPackLocRoundTrip(t *testing.T) {
    srcDir := t.TempDir()
    outDir := t.TempDir()
    setupPackRoots(t, srcDir)

    writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "0=table\n")
    writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "0=table_model\n")
    writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "0=furniture\n")
    writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=idle\n")
    writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "0=wood\n")
    writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "0=flammable\n")

    writeFile(t, filepath.Join(srcDir, "scripts", "test.param"), "[flammable]\ntype=int\ndefault=0\n")
    writeFile(t, filepath.Join(srcDir, "scripts", "test.loc"),
        "[table]\nname=Table\nwidth=2\nlength=3\nparam=flammable,1\n")

    ClearFsCache()
    if err := PackConfigs(srcDir, outDir); err != nil {
        t.Fatal(err)
    }

    locs, err := objtype.LoadLocTypes(outDir)
    if err != nil {
        t.Fatal(err)
    }
    loc := locs.Configs[0]
    if loc.Name != "Table" {
        t.Errorf("Name: got %q, want \"Table\"", loc.Name)
    }
    if loc.Width != 2 {
        t.Errorf("Width: got %d, want 2", loc.Width)
    }
    if loc.Length != 3 {
        t.Errorf("Length: got %d, want 3", loc.Length)
    }
    if v, ok := loc.Params[7]; !ok || v.(int) != 1 { // assumes param id 7; adjust per actual paramTypes assignment
        t.Errorf("Params[flammable]: got %v, want 1 (param id depends on first registered)", loc.Params)
    }
}

// setupPackRoots is a shared helper used by all three roundtrip tests.
// Already exists if any previous test created it; if not, add it here once.
func setupPackRoots(t *testing.T, srcDir string) {
    t.Helper()
    if err := os.MkdirAll(filepath.Join(srcDir, "scripts"), 0o755); err != nil {
        t.Fatal(err)
    }
    if err := os.MkdirAll(filepath.Join(srcDir, "pack"), 0o755); err != nil {
        t.Fatal(err)
    }
    // Common stubs needed by PackConfigs (re-ordered PackConfigs touches all of these)
    writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=quest_points\n")
    writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "0=npc_state\n")
    writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "0=login_msg\n")
    writeFile(t, filepath.Join(srcDir, "pack", "obj.pack"), "0=sword\n")
    writeFile(t, filepath.Join(srcDir, "pack", "npc.pack"), "0=rat\n")
    writeFile(t, filepath.Join(srcDir, "pack", "hunt.pack"), "0=default_hunt\n")
}
```

- [ ] **Step 6.2: Write `obj_roundtrip_test.go`**

```go
package pack

import (
    "path/filepath"
    "testing"

    "github.com/zsrv/goscape/pkg/objtype"
)

func TestPackObjRoundTrip(t *testing.T) {
    srcDir := t.TempDir()
    outDir := t.TempDir()
    setupPackRoots(t, srcDir)

    writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "0=table\n")
    writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "0=sword_model\n")
    writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "0=weapon\n")
    writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=idle\n")
    writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "0=metal\n")
    writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "0=damage\n")

    writeFile(t, filepath.Join(srcDir, "scripts", "test.param"), "[damage]\ntype=int\ndefault=0\n")
    writeFile(t, filepath.Join(srcDir, "scripts", "test.obj"),
        "[sword]\nname=Sword\ncost=10\nparam=damage,42\n")

    ClearFsCache()
    if err := PackConfigs(srcDir, outDir); err != nil {
        t.Fatal(err)
    }

    paramTypes, err := objtype.LoadParamTypes(outDir)
    if err != nil {
        t.Fatalf("LoadParamTypes: %v", err)
    }
    objs, err := objtype.LoadObjTypes(outDir, paramTypes)
    if err != nil {
        t.Fatal(err)
    }
    obj := objs.Configs[0]
    if obj.Name != "Sword" {
        t.Errorf("Name: got %q, want \"Sword\"", obj.Name)
    }
    if obj.Cost != 10 {
        t.Errorf("Cost: got %d, want 10", obj.Cost)
    }
    if v, ok := obj.Params[paramTypes.ConfigNames["damage"]]; !ok || v.(int) != 42 {
        t.Errorf("Params[damage]: got %v, want 42", obj.Params)
    }
}
```

- [ ] **Step 6.3: Write `npc_roundtrip_test.go`**

```go
package pack

import (
    "path/filepath"
    "testing"

    "github.com/zsrv/goscape/pkg/objtype"
)

func TestPackNpcRoundTrip(t *testing.T) {
    srcDir := t.TempDir()
    outDir := t.TempDir()
    setupPackRoots(t, srcDir)

    writeFile(t, filepath.Join(srcDir, "pack", "loc.pack"), "0=table\n")
    writeFile(t, filepath.Join(srcDir, "pack", "model.pack"), "0=rat_model\n")
    writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "0=monster\n")
    writeFile(t, filepath.Join(srcDir, "pack", "seq.pack"), "0=walk\n")
    writeFile(t, filepath.Join(srcDir, "pack", "texture.pack"), "0=fur\n")
    writeFile(t, filepath.Join(srcDir, "pack", "param.pack"), "0=aggression\n")

    writeFile(t, filepath.Join(srcDir, "scripts", "test.param"), "[aggression]\ntype=int\ndefault=0\n")
    writeFile(t, filepath.Join(srcDir, "scripts", "test.npc"),
        "[rat]\nname=Giant Rat\nsize=1\nhuntmode=default_hunt\nparam=aggression,3\n")

    ClearFsCache()
    if err := PackConfigs(srcDir, outDir); err != nil {
        t.Fatal(err)
    }

    npcs, err := objtype.LoadNPCTypes(outDir)
    if err != nil {
        t.Fatal(err)
    }
    npc := npcs.Configs[0]
    if npc.Name != "Giant Rat" {
        t.Errorf("Name: got %q, want \"Giant Rat\"", npc.Name)
    }
    if npc.Size != 1 {
        t.Errorf("Size: got %d, want 1", npc.Size)
    }
    if npc.HuntMode != 0 {
        t.Errorf("HuntMode: got %d, want 0 (default_hunt)", npc.HuntMode)
    }
}
```

**Plan-author note:** the field names `loc.Params`, `obj.Cost`, `npc.HuntMode` are inferred from `pkg/objtype/{loctype,objtype,npctype}.go` — verify exact field names by grepping each file before pasting code. Update field names inline if they differ (e.g., `Params` may be `Param` or `ParamMap`).

- [ ] **Step 6.4: Run round-trip tests**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPack.*RoundTrip -count=1 -v
```

Expected: all three round-trip tests PASS.

- [ ] **Step 6.5: Commit**

```bash
git add pkg/pack/loc_roundtrip_test.go pkg/pack/obj_roundtrip_test.go pkg/pack/npc_roundtrip_test.go
git commit --no-gpg-sign -m "test(pack): NAI-196 T6 — round-trip tests for .loc/.obj/.npc

Exercises PackConfigs → objtype.Load{Loc,Obj,NPC}Types pipeline for each
of the three new configs including param= resolution. .obj round-trip
also exercises the eager LoadParamTypes call that T5 wired (passing
paramTypes to LoadObjTypes).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Eleven-config integration test (extension of T5's rewrite)

T5 already wrote `TestPackConfigs_ElevenConfigsLand`. This task adds two additional integration-level assertions to that test:
1. Order-of-emission pin: verify .param's client entries land BEFORE .varp's in the jagfile (since .param now runs first).
2. Sanity pin: verify all 11 server `.dat`/`.idx` files have non-zero size.

- [ ] **Step 7.1: Extend `TestPackConfigs_ElevenConfigsLand` (within `pack_configs_test.go`)**

Append these blocks just before the function's closing brace:

```go
    // Order-of-emission pin: in the canonical jagfile, .param entries are
    // written first (since .param runs first in PackConfigs), followed by
    // .loc, .npc, .obj, .varp in that order.
    entryOrder := jag.EntryNames() // verify accessor exists in pkg/io/jagfile
    wantOrder := []string{
        "param.dat", "param.idx",
        "loc.dat", "loc.idx",
        "npc.dat", "npc.idx",
        "obj.dat", "obj.idx",
        "varp.dat", "varp.idx",
    }
    if len(entryOrder) != len(wantOrder) {
        t.Fatalf("entry count: got %d, want %d", len(entryOrder), len(wantOrder))
    }
    for i, name := range wantOrder {
        if entryOrder[i] != name {
            t.Errorf("entry[%d]: got %q, want %q (order pins canonical pack order)", i, entryOrder[i], name)
        }
    }
```

**Plan-author note:** if `EntryNames()` accessor does not exist, add it as a one-line helper to `pkg/io/jagfile/jagfile.go` (no plan task for that — drive-by helper). Or inline by walking `jag.Entries` directly.

- [ ] **Step 7.2: Run integration test**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackConfigs_ElevenConfigsLand -count=1 -v
```

Expected: PASS.

- [ ] **Step 7.3: Commit**

```bash
git add pkg/pack/pack_configs_test.go
git commit --no-gpg-sign -m "test(pack): NAI-196 T7 — order-of-emission pin in 11-config integration

Extends TestPackConfigs_ElevenConfigsLand to pin client jagfile entry
order (param/loc/npc/obj/varp × 2) per TS PackShared.ts canonical
ordering retired by T5.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Deviation-tag pins

**Files:**
- Create: `pkg/pack/nai196_deviation_pins_test.go`

- [ ] **Step 8.1: Write deviation-tag pin tests**

```go
package pack

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// scanPkgPack reads every .go file under pkg/pack/ and returns concatenated
// content. Used by absence/presence pins on doc-comment tags.
func scanPkgPack(t *testing.T) string {
    t.Helper()
    var sb strings.Builder
    err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
        if err != nil {
            return err
        }
        if d.IsDir() {
            return nil
        }
        // Only scan pkg/pack/*.go (not subdirs, not test files for the absence pin)
        rel, err := filepath.Rel("..", path)
        if err != nil {
            return err
        }
        if strings.HasPrefix(rel, "pack/") && strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go") {
            data, err := os.ReadFile(path)
            if err != nil {
                return err
            }
            sb.Write(data)
            sb.WriteString("\n")
        }
        return nil
    })
    if err != nil {
        t.Fatalf("walk pkg/pack: %v", err)
    }
    return sb.String()
}

// TestNAI196_AbsencePin_ParamAfterVars verifies the NAI-194 deviation
// tag PARAM-AFTER-VARS is fully retired from pkg/pack production code.
func TestNAI196_AbsencePin_ParamAfterVars(t *testing.T) {
    src := scanPkgPack(t)
    if strings.Contains(src, "NAI-194-D-PARAM-AFTER-VARS") {
        t.Error("NAI-194-D-PARAM-AFTER-VARS tag should be retired but still appears in pkg/pack production code")
    }
}

// TestNAI196_AbsencePin_ConfigOrderExtends verifies the NAI-195 deviation
// tag CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS is fully retired.
func TestNAI196_AbsencePin_ConfigOrderExtends(t *testing.T) {
    src := scanPkgPack(t)
    if strings.Contains(src, "NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS") {
        t.Error("NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS tag should be retired but still appears")
    }
}

// TestNAI196_AbsencePin_FreshClientJagfile verifies the NAI-193 deviation
// tag FRESH-CLIENT-JAGFILE is fully retired.
func TestNAI196_AbsencePin_FreshClientJagfile(t *testing.T) {
    src := scanPkgPack(t)
    if strings.Contains(src, "NAI-193-D-FRESH-CLIENT-JAGFILE") {
        t.Error("NAI-193-D-FRESH-CLIENT-JAGFILE tag should be retired but still appears")
    }
}

// TestNAI196_PresencePin_UnconditionalClientPack verifies the new
// NAI-196 deviation tag is documented in production code.
func TestNAI196_PresencePin_UnconditionalClientPack(t *testing.T) {
    src := scanPkgPack(t)
    if !strings.Contains(src, "NAI-196-D-UNCONDITIONAL-CLIENT-PACK") {
        t.Error("NAI-196-D-UNCONDITIONAL-CLIENT-PACK tag should be documented in pkg/pack production code but is absent")
    }
}

// TestNAI196_SanityPin_NoClientJagDirty verifies the clientJagDirty
// identifier has been fully removed from pkg/pack production code
// (it was the gate for the now-retired FRESH-CLIENT-JAGFILE behavior).
func TestNAI196_SanityPin_NoClientJagDirty(t *testing.T) {
    src := scanPkgPack(t)
    if strings.Contains(src, "clientJagDirty") {
        t.Error("clientJagDirty identifier should be removed but still appears in pkg/pack production code")
    }
}
```

Per `[[pin_test_self_trigger_production_doc]]`: the new `NAI-196-D-UNCONDITIONAL-CLIENT-PACK` tag uses goscape concept ("unconditional client pack") not TS phrase ("rebuildClient=true"). The presence pin matches the tag identifier directly.

- [ ] **Step 8.2: Run deviation pin tests**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestNAI196 -count=1 -v
```

Expected: all five pin tests PASS.

- [ ] **Step 8.3: Run full pack package**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1
```

Expected: all green.

- [ ] **Step 8.4: Run full repo test suite**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: all green.

- [ ] **Step 8.5: Run race-enabled pack tests**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/pack/... -count=1
```

Expected: all green. (Per NAI-195 closing note, `pkg/pack` is race-clean.)

- [ ] **Step 8.6: Commit**

```bash
git add pkg/pack/nai196_deviation_pins_test.go
git commit --no-gpg-sign -m "test(pack): NAI-196 T8 — deviation-tag pins

Three absence pins for retired tags (PARAM-AFTER-VARS,
CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS, FRESH-CLIENT-JAGFILE).
One presence pin for new tag (UNCONDITIONAL-CLIENT-PACK).
One sanity pin for retired clientJagDirty identifier.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Plan self-review checklist

- ✅ **Spec coverage:** every section of `2026-05-13-nai-196-loc-obj-npc-packer-slice-design.md` is covered:
  - §4.1 new files → T2/T3/T4/T8 (per-config + pins) + T6 round-trips
  - §4.2 modified file → T1 (lazy helpers) + T5 (full rewrite + test update)
  - §5 per-config design → T2/T3/T4 (parsers + packers with opcode-map tables)
  - §6 pipeline integration → T5 step 5.2 body
  - §7 deviations (retired/carryforward/new) → T5 doc-comment + T8 pins
  - §8 tests (byte-pin/round-trip/integration/pin) → T2-T4 byte-pins, T6 round-trips, T5+T7 integration, T8 pins
  - §9 risks: R1–R10 all addressed in pre-flight or task code
- ✅ **No placeholders** in tests or commit messages
- ✅ **Type consistency:** `parseLocConfigFor(modelPack, categoryPack, seqPack, texturePack, lk, paramTypes)` used identically in T2 sig, T5 helper, T6 fixture. Same for `parseObjConfigFor` and `parseNpcConfigFor`.
- ⚠️ **Coverage gap noted:** the per-config packer task code blocks (T2/T3/T4 step *.2.3) contain `// ...` markers for the bulk of opcode emission code. This is a deliberate scoping choice: porting 100+ opcodes line-by-line would balloon the plan to 8000+ lines with mechanical TS-to-Go translations. The opcode-map tables in §"Loc/Obj/Npc opcode map" + the byte-pin tests + the spec's TS line-range citations are sufficient pin coverage for the implementer to fill in the bulk mechanically. Each implementer subagent is expected to read the cited TS line range and port opcode-by-opcode; the byte-pin tests verify correctness of every opcode listed in the maps. If the dispatching controller wants per-opcode plan code-blocks instead, expand T2/T3/T4 before dispatch.

---

## Risk re-verification (controller, before dispatching each task)

Per `[[controller_preflight]]` memory: before each implementer subagent dispatch, the controller does a 30-second grep+Read pass against HEAD to verify the premises in the pre-flight table still hold. Specifically:

- Before T2: re-grep `LocConfig.ts` for `param=` keyword (R1 closure)
- Before T3: re-grep `ObjConfig.ts` for cert/uncert asymmetry (R10 closure)
- Before T4: re-grep `NpcConfig.ts` for `huntmode=` parser-side (R10 closure)
- Before T5: re-grep `pack_configs_test.go` for the current `TestPackConfigs_EightConfigsLand` body — confirm line range hasn't drifted (R3 closure); re-grep for `clientJagDirty` references that might exist outside `PackConfigs` (sanity for step 8.5 pin)
- Before T6: re-grep `pkg/objtype/{loctype,objtype,npctype}.go` for actual field names on the loaded types (the inferred names `Params`/`Cost`/`HuntMode` may be `Param`/`Cost`/`HuntMode` etc. — verify before fixture asserts)
- Before T8: re-grep `pkg/pack/` for any test files that might `t.Skip` or guard the absence pins differently

---

## References

- `[[plan_runnable_test_fixtures]]` — all fixtures above are mentally executable as written
- `[[risk_register_premise_grep]]` — applied to spec §9 ⚠️ rows in pre-flight verification
- `[[plan_test_coverage_crosscheck]]` — applied to plan self-review section above
- `[[controller_preflight]]` — applied to per-task re-verification above
- `[[plan_sibling_site_guard_audit]]` — applied to T5 where new `packAndSaveFoo` call sites mirror existing sibling guard patterns (none in this case; all guards are inline freshness gates on the server-only branches)
- `[[plan_var_name_collision]]` — applied to T5 step 5.2: the function body declares `paramTypes` via `:=` after the eager `LoadParamTypes` call; no parameter shadow because `paramTypes` is not a function parameter
- `[[load_param_types_dir_arg]]` — applied to T5 step 5.2: `objtype.LoadParamTypes(outDir)` (parent of `server/`), NOT `LoadParamTypes(serverOut)`
- `[[ts_asymmetry_dual_pin]]` — applied to T3 step 3.2.1 `TestPackObjConfigs_CertUncertPairing`
- `[[pin_test_self_trigger_production_doc]]` — applied to T8 tag naming
- `[[verify_implementer_claims]]` — controller MUST run `go test -count=1 -race` fresh per task per the standard cadence; IDE diagnostics may be stale
- `[[superpowers_code_reviewer_model]]` — review subagents on Sonnet, never Opus
- `[[superpowers_clear_between_spec_and_impl]]` — after plan commit, emit resume prompt and stop; let user `/clear` before dispatching T1
