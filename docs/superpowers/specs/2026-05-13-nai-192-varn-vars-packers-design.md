# NAI-192 — First per-config packers (varn + vars)

**Date:** 2026-05-13
**Status:** Spec (awaiting plan)
**Tech stack:** Go 1.26+
**TS source:** `LostCityRS/Engine-TS/tools/pack/config/VarnConfig.ts`, `tools/pack/config/VarsConfig.ts`, `tools/pack/config/PackShared.ts`, `src/cache/config/ScriptVarType.ts` (`getTypeChar`).
**Predecessors:** NAI-191 (pack-pipeline source-side foundation — `pkg/pack/{fscache,parse,namemap,packfile,freshness,crawl}.go`).
**HEAD at spec-write:** `c68e07b`

## §1 Goal

Ship the first per-config packer slice on top of the NAI-191 foundation: parse and pack `.varn` (var-npc) and `.vars` (var-shared) source configs into byte-identical `data/pack/server/{varn,vars}.{dat,idx}` outputs.

Same slice introduces the once-per-arc infrastructure every subsequent per-config packer (`param`, `mesanim`, `inv`, `enum`, …) reuses: `PackedData` (paired dat+idx Packet buffers), `ConfigValue`/`ConfigLine` typed shape, `Constants` map with `^foo` substitution, a typed `ReadTypedConfigs` reader on top of the NAI-191 `LoadDirExtFull`, and a `PackConfigs(srcDir, outDir) error` orchestrator skeleton with one branch wired.

Varn and Vars are picked first because they share the smallest, simplest TS source (82 LOC each, line-identical apart from the target `PackFile`). Splitting them is artificial — vars adds <30 LOC of mechanical duplication once varn lands.

## §2 Out of scope

| Concern | TS location | Why deferred |
|---|---|---|
| Module-level `VarnPack` / `VarsPack` singletons | `tools/pack/PackFile.ts:231-256` | NAI-191 §2 deferred all 26 module-level pack singletons. NAI-192 takes `*PackFile` as an explicit parameter to `packVarnConfigs`/`packVarsConfigs`. Tag: `NAI-192-D-PACKFILE-SINGLETONS-DEFERRED`. |
| Cross-domain var-name uniqueness check across `{VarpPack, VarnPack, VarsPack}` | `PackShared.ts:292-310` | Requires `VarpPack` (which lands with the .varp packer slice). Belongs with the **last** of the three to land. Tag: `NAI-192-D-VARP-UNIQUENESS-DEFERRED`. |
| `validateConfigPack` (auto-generates `<srcDir>/pack/<type>.pack` from crawled config names + cert_ prefix logic) | `tools/pack/PackFile.ts:8-228` | NAI-191 §2 deferred all 6 `validate*` functions. Without it, production callers must hand-author `varn.pack`/`vars.pack`; NAI-192 tests supply hand-crafted fixtures. Production callsite waits. |
| `.varp` packer (`VarpConfig.ts`, 111 LOC) | `tools/pack/config/VarpConfig.ts` | Larger schema (`type`/`scope`/`transmit`/`protect`, server+client dual output, CRC validate). Lands in NAI-193 or later. |
| `BUILD_VERIFY` checksum validate callback in `readConfigs` | `PackShared.ts:251-253` | NAI-191 §2 deferred. Varn/vars don't have a TS CRC anyway (no `Packet.checkcrc` arg in PackShared `readConfigs` call sites for varn/vars — both use `noOp` saveClient and no validate). |
| Client-side jagfile write (`saveClient`) | `PackShared.ts:435-440, 647-663` | Varn and vars TS source uses `noOp` for `saveClient` — server-only. No jagfile threaded this slice. |
| Production callsite (build CLI, `::rebuild` cheat) | `ClientCheatHandler.ts:151-153` | Closes the arc. Standalone slice. |
| Sprite/graphics/interface/map/midi/sound packers | `tools/pack/{PixPack,chat,graphics,interface,map,midi,sound,sprite}/*.ts` | Each is its own sub-spec under the NAI-192+ track. |

## §3 Pre-flight audit

Per `controller_preflight` + `risk_register_premise_grep`, every premise below was re-verified against HEAD `c68e07b`.

### §3.1 TS file scope

| TS file | LOC | Port scope in NAI-192 |
|---|---:|---|
| `tools/pack/config/VarnConfig.ts` | 82 | **Yes** — wholesale port. |
| `tools/pack/config/VarsConfig.ts` | 82 | **Yes** — wholesale port (line-identical to VarnConfig modulo `VarsPack` target). |
| `tools/pack/config/PackShared.ts` | 670 | **Partial.** Port: `PackedData` class, `ConfigValue`/`ConfigLine`/`ConfigDatIdx`/`ParseFn`/`PackFn` types, `isConfigBoolean`/`getConfigBoolean`, `CONSTANTS` map + load, `readConfigs` typed-reader. **Defer**: full `packConfigs()` orchestrator body (port only the varn+vars branches), the var-name uniqueness check, `frame_del` / `category.dat` / `dbtable.dat` / `dbrow.dat` / `enum.dat` / `inv.dat` / `mesanim.dat` / `struct.dat` / `seq.dat` / `loc.dat` / `flo.dat` / `spotanim.dat` / `npc.dat` / `obj.dat` / `idk.dat` / `varp.dat` / `hunt.dat` branches. |
| `src/cache/config/ScriptVarType.ts` | 181 | **Partial.** `ScriptVarType` constants already in `pkg/objtype/paramtype.go`; add `ScriptVarTypeFromName(name string) (ScriptVarType, bool)` — TS `getTypeChar`. **Defer**: `getType` and `getDefault` until a consumer needs them. |

**Total in scope:** ~290 LOC TS → projected ~350-400 LOC Go + tests.

### §3.2 Foundation status

NAI-191 left in `pkg/pack/`: `FsCache`, `Parse` (`LoadFile`, `LoadFileFull`, `LoadDirExt`, `LoadDirExtFull`, flat `ReadConfigs`), `NameMap` (`LoadOrder`, `LoadPack`, `LoadDirExact`), `PackFile` struct (`NewPackFile`, `Reload`, `Load`, `Save`, `Register`, `Delete`, `GetByID`, `GetByName`, `RefreshNames`, `Max`), `Freshness` (`GetModified`, `GetLatestModified`, `ShouldBuild`, `ShouldBuildFile`, `ShouldBuildFileAny`), `Crawl` (`CrawlConfigNames`).

The existing flat `ReadConfigs(srcDir, ext) (map[string][]string, error)` is **not modified** by NAI-192. It returns line-arrays-per-header and serves use cases where typed parsing is unnecessary. NAI-192 adds `ReadTypedConfigs(...)` alongside it — they coexist.

### §3.3 `Packet` write-pointer audit (`packet_rw_pointer_gotcha`)

Memory `packet_rw_pointer_gotcha`: `Packet.Pos` is the **read** pointer. Writes (`P1`/`P2`/…) advance `len(Data)` via `tryGrowByReslice`/`grow`, not `Pos`. TS `PackedData.next()` uses `this.dat.pos` because TS Packet bumps `pos` on writes; goscape's port must use `p.Length()` (i.e., `len(p.Data)`) for the write-cursor.

This is the single most likely correctness trap in NAI-192. Plan §4 codifies it.

### §3.4 `Packet.Alloc(size)` / `Packet.Save(path, length, start)`

Both verified at HEAD `c68e07b`:
- `Alloc(size int) *Packet` (line 73) — pool-backed.
- `(*Packet).Save(filePath string, length int, start int) error` (line 108) — writes `Data[start:start+length]` to `filePath`, `os.MkdirAll` on the parent dir. Matches TS `Packet.save`.

`PackedData.Save(dataPath, idxPath string) error` will call `pd.Dat.Save(dataPath, pd.Dat.Length(), 0)` then `pd.Idx.Save(idxPath, pd.Idx.Length(), 0)`.

### §3.5 `ScriptVarType` location

`pkg/objtype/paramtype.go:27` declares `type ScriptVarType int` + 25 const codes (verified). NAI-192 extracts these into a new `pkg/objtype/scriptvartype.go` file (no behavior change for paramtype.go consumers — same package). `ScriptVarTypeFromName` is appended.

### §3.6 No existing `.pack` source files

`find` returns zero `varn.pack`/`vars.pack`/`*.pack` files in either goscape or `LostCityRS/Engine-TS`. They are generated at runtime by `validateConfigPack` (deferred). NAI-192 tests construct hand-crafted `pack/varn.pack` + `pack/vars.pack` files in `t.TempDir()`.

### §3.7 Output destination

`data/pack/server/varn.dat` + `varn.idx` + `vars.dat` + `vars.idx` already exist as cache fixtures (loaded by `pkg/objtype.LoadVarnTypes` / `LoadVarsTypes`). NAI-192 produces byte-compatible output — cross-package consumer test in §7.4 binds parity.

## §4 Components

### §4.1 `PackedData` (`pkg/pack/packed_data.go`)

```go
type PackedData struct {
    Dat    *packet.Packet
    Idx    *packet.Packet
    Size   int
    marker int
}

func NewPackedData(size int) *PackedData {
    pd := &PackedData{
        Dat:  packet.Alloc(5),
        Idx:  packet.Alloc(3),
        Size: size,
    }
    pd.Dat.P2(uint16(size))
    pd.Idx.P2(uint16(size))
    pd.marker = 2
    return pd
}

// Next writes one terminator (0x00) to dat, records the bytes-since-marker
// to idx as a p2, and advances marker to the new dat write cursor.
//
// NAI-192-D-PACKET-WRITE-CURSOR: TS uses dat.pos; goscape's Packet.Pos is
// the read pointer (memory packet_rw_pointer_gotcha). Use Dat.Length().
func (pd *PackedData) Next() {
    pd.Dat.P1(0)
    pd.Idx.P2(uint16(pd.Dat.Length() - pd.marker))
    pd.marker = pd.Dat.Length()
}

func (pd *PackedData) P1(v uint8)       { pd.Dat.P1(v) }
func (pd *PackedData) P2(v uint16)      { pd.Dat.P2(v) }
func (pd *PackedData) P3(v uint32)      { pd.Dat.P3(v) }
func (pd *PackedData) P4(v uint32)      { pd.Dat.P4(v) }
func (pd *PackedData) PBool(v bool)     { pd.Dat.PBool(v) }
func (pd *PackedData) PJStr(s string)   { pd.Dat.PJStrLF(s) }  // TS pjstr = LF-terminated (0x0a)

// Save writes Dat and Idx to disk. Caller picks paths.
func (pd *PackedData) Save(dataPath, idxPath string) error { ... }
```

**TS source:** `tools/pack/config/PackShared.ts:39-84`. `pjstr` in TS Packet (`io/Packet.ts:330-337`) writes string then `setUint8(pos++, 10)` — LF (0x0a) terminator. Maps to goscape `PJStrLF`. The existing decoder at `pkg/objtype/varntype.go:21` reads `GJStrLF()`, confirming wire-format parity.

### §4.2 `ConfigValue` / `ConfigLine` (`pkg/pack/config_value.go`)

```go
// ConfigValue is the typed value of a parsed `key=value` line.
// TS is a discriminated union (`string | number | boolean | number[] | LocModelShape[] | ParamValue | ...`);
// Go uses `any` and per-packer type assertion. The list grows as more
// packers land (NAI-193+ adds LocModelShape, ParamValue, HuntCheckVar, etc.).
type ConfigValue = any

type ConfigLine struct {
    Key   string
    Value ConfigValue
}

func IsConfigBoolean(v string) bool { return v == "yes" || v == "no" || v == "true" || v == "false" || v == "1" || v == "0" }
func GetConfigBoolean(v string) bool { return v == "yes" || v == "true" || v == "1" }
```

### §4.3 `Constants` (`pkg/pack/constants.go`)

```go
type Constants map[string]string

// LoadConstants walks <srcDir>/scripts/**/*.constant via LoadDirExt and
// returns a name→value map. Lines: skip blank / "//"; expect "name=value";
// strip leading "^" from name; error on duplicate name.
//
// TS source: PackShared.ts:262-289 (loadDir callback body).
func LoadConstants(srcDir string) (Constants, error) { ... }

// substituteConstants walks `value` for ^foo runs (terminators: '\r' '\n'
// ',' ' ') and replaces with c[foo] when present. Absent constants leave
// the literal "^foo" in place (TS parity — no error).
//
// TS source: PackShared.ts:200-223.
func substituteConstants(value string, c Constants) string { ... }
```

### §4.4 `ReadTypedConfigs` (`pkg/pack/read_typed.go`)

```go
// ParseFn returns (value, ok, err):
//   - ok=true, err=nil  → accepted
//   - ok=false, err=nil → invalid key  (TS undefined → "Invalid property key")
//   - err != nil        → invalid value (TS null      → "Invalid property value")
type ParseFn func(key, value string) (ConfigValue, bool, error)

// ReadTypedConfigs walks <srcDir>/scripts/*.<ext> via LoadDirExtFull,
// splits each file into [name]-delimited blocks, applies constants
// substitution to every value, calls parseFn per key=value line, enforces
// required-properties at block close, and returns map[debugname][]ConfigLine.
//
// TS source: PackShared.ts:141-247.
func ReadTypedConfigs(srcDir, ext string, required []string, parseFn ParseFn, c Constants) (map[string][]ConfigLine, error)
```

Error envelope mirrors TS `parseStepError(file, line, msg)`: returned errors are of shape `Error during parsing - see <file>:<line+1>\n<msg>` for re-grep parity with TS.

### §4.5 `ScriptVarTypeFromName` (`pkg/objtype/scriptvartype.go`)

```go
// ScriptVarTypeFromName returns the ScriptVarType code for a type name,
// or (0, false) for unknown names. Matches TS ScriptVarType.getTypeChar.
//
// TS source: src/cache/config/ScriptVarType.ts:85-170.
func ScriptVarTypeFromName(name string) (ScriptVarType, bool)
```

Table of 25 (name → code): `int → 105`, `autoint → 255`, `string → 115`, `enum → 103`, `obj → 111`, `loc → 108`, `component → 73`, `namedobj → 79`, `struct → 74`, `boolean → 49`, `coord → 99`, `category → 121`, `spotanim → 116`, `npc → 110`, `inv → 118`, `synth → 80`, `seq → 65`, `stat → 83`, `varp → 86`, `player_uid → 112`, `npc_uid → 78`, `interface → 97`, `npc_stat → 254`, `idkit → 75`, `dbrow → 208`. All hard-coded constants verified in TS source.

### §4.6 `parseVarnConfig` / `packVarnConfigs` (`pkg/pack/varn.go`)

```go
func parseVarnConfig(key, value string) (ConfigValue, bool, error) {
    if key == "type" {
        t, ok := objtype.ScriptVarTypeFromName(value)
        if !ok { return nil, true, fmt.Errorf("unknown type: %s", value) }
        return t, true, nil
    }
    return nil, false, nil
}

func packVarnConfigs(configs map[string][]ConfigLine, pf *PackFile) *PackedData {
    pd := NewPackedData(pf.Max)
    for id := 0; id < pf.Max; id++ {
        name := pf.GetByID(id)
        if cfg, ok := configs[name]; ok {
            for _, line := range cfg {
                if line.Key == "type" {
                    pd.P1(1)
                    pd.P1(uint8(line.Value.(objtype.ScriptVarType)))
                }
            }
        }
        if len(name) > 0 {
            pd.P1(250)
            pd.PJStr(name)
        }
        pd.Next()
    }
    return pd
}
```

**TS parity note:** The `stringKeys` / `numberKeys` / `booleanKeys` branches in TS `parseVarnConfig` are empty arrays — dead code preserved by the TS author. Goscape omits the dead branches per project YAGNI; if a future schema addition needs them, that addition revives them with the relevant keys populated. Tag: `NAI-192-D-DEADBRANCH-OMITTED`.

### §4.7 `parseVarsConfig` / `packVarsConfigs` (`pkg/pack/vars.go`)

Byte-identical structure to §4.6 — same parse logic, same pack loop, same opcodes. The only difference is the caller passes the `VarsPack` (still an explicit `*PackFile` parameter per `NAI-192-D-PACKFILE-SINGLETONS-DEFERRED`).

### §4.8 `PackConfigs` orchestrator (`pkg/pack/pack_configs.go`)

```go
// PackConfigs runs the per-config packing pipeline. NAI-192 wires only
// .varn and .vars; subsequent NAI-193+ sub-specs add branches.
//
// TS source: PackShared.ts:261-669 (packConfigs function body).
func PackConfigs(srcDir, outDir string) error {
    constants, err := LoadConstants(srcDir)
    if err != nil { return err }

    // TODO(NAI-VARP+): var-name uniqueness across {VarpPack, VarnPack, VarsPack}.
    // NAI-192-D-VARP-UNIQUENESS-DEFERRED.

    serverOut := filepath.Join(outDir, "server")

    if ShouldBuild(filepath.Join(srcDir, "scripts"), ".varn", filepath.Join(serverOut, "varn.dat")) {
        varnPack, err := NewPackFile(srcDir, "varn", nil)
        if err != nil { return err }
        cfgs, err := ReadTypedConfigs(srcDir, ".varn", nil, parseVarnConfig, constants)
        if err != nil { return err }
        pd := packVarnConfigs(cfgs, varnPack)
        if err := pd.Save(filepath.Join(serverOut, "varn.dat"), filepath.Join(serverOut, "varn.idx")); err != nil {
            return err
        }
    }

    if ShouldBuild(filepath.Join(srcDir, "scripts"), ".vars", filepath.Join(serverOut, "vars.dat")) {
        // ...same shape, vars.
    }

    return nil
}
```

## §5 Data flow

```
<srcDir>/scripts/**/*.constant  ──►  LoadConstants  ──►  Constants
                                                              │
                                                              ▼
<srcDir>/scripts/**/*.varn  ──►  ReadTypedConfigs(.varn, parseVarnConfig, constants)
                                                              │  map[string][]ConfigLine
                                                              ▼
              <srcDir>/pack/varn.pack  ──►  NewPackFile  ──►  packVarnConfigs(cfgs, pf)
                                                              │  *PackedData
                                                              ▼
                                                      pd.Save(varn.dat, varn.idx)
                                                              │
                                                              ▼
                                          <outDir>/server/varn.{dat,idx}
```

Same path for `.vars`. Freshness gate via NAI-191 `ShouldBuild` at each branch top.

## §6 Error handling

| Site | Behavior | TS parity |
|---|---|---|
| Unclosed multi-line comment in source | Propagate from `LoadDirExtFull` (NAI-191) | ✅ |
| Missing `=` separator | `"Error during parsing - see %s:%d\nMissing property separator: %s"` | ✅ |
| Unclosed `[name` bracket | `"...Missing closing bracket: %s"` | ✅ |
| Empty `[]` name | `"...No config name"` | ✅ |
| Duplicate `[name]` | `"...Duplicate config found: %s"` | ✅ |
| `parseFn → ok=false, err=nil` | `"...Invalid property key: %s"` | ✅ |
| `parseFn → err != nil` | `"...Invalid property value: %s"` | ✅ |
| Missing required property at block close | `"...Missing required property: %s"` (line `-1` per TS) | ✅ |
| `ScriptVarTypeFromName(unknown)` in `parseVarnConfig` | returns `err != nil` → caller maps to `"Invalid property value"` | ✅ |
| Duplicate constant | `"duplicate constant: %s"` | ✅ |
| `ShouldBuild` returns false | Skip branch, no error | ✅ |
| `os.WriteFile` failure | Propagate | ✅ |
| TS `BUILD_VERIFY` checksum validate callback | **Deferred** — varn/vars TS passes no validate arg | n/a |

## §7 Testing strategy

### §7.1 Unit tests

- **`packed_data_test.go`** — `NewPackedData(N)` writes `p2(N)` to both `Dat` and `Idx`; `marker == 2`. `Next()` after `P1(1); P1(105)` writes `0x00` to Dat at offset 4 (3-byte entry + terminator) and `p2(3)` to Idx; `marker` advances to 5. Byte-pin both buffers.
- **`constants_test.go`** — multi-file load + dedup-error + leading-`^` strip; substitution table-driven: `^foo` terminated by `\r` / `\n` / `,` / ` ` / end-of-string; constant absent → literal unchanged.
- **`read_typed_test.go`** — happy path with 2 blocks × 2 lines; required-property miss errors; duplicate name errors; unclosed bracket errors; constants substitute end-to-end; `parseFn` ok-false / err-nonnil / ok-true paths.
- **`scriptvartype_test.go`** — table-driven for all 25 (name, code) pairs; unknown→`(0, false)`.

### §7.2 Byte-pin tests (varn + vars)

`varn_test.go` / `vars_test.go`: `t.TempDir()` fixture with one `scripts/test.varn` file declaring `[npctier]\ntype=int\n` and `[npchealth]\ntype=int\n`, plus a hand-crafted `pack/varn.pack` declaring `0=npctier\n1=npchealth\n`.

```go
configs, err := ReadTypedConfigs(srcDir, ".varn", nil, parseVarnConfig, Constants{})
require.NoError(t, err)
pf, err := NewPackFile(srcDir, "varn", nil)
require.NoError(t, err)
pd := packVarnConfigs(configs, pf)

// Expected dat:
//   p2(size=2)             — 00 02
//   id=0 body:
//     p1(1), p1(105)       — 01 69
//     p1(250), pjstrlf("npctier") — fa 6e 70 63 74 69 65 72 0a
//   next() terminator:     — 00
//   id=1 body:
//     p1(1), p1(105)       — 01 69
//     p1(250), pjstrlf("npchealth") — fa 6e 70 63 68 65 61 6c 74 68 0a
//   next() terminator:     — 00
require.Equal(t, []byte{0x00, 0x02, 0x01, 0x69, 0xfa, 0x6e,0x70,0x63,0x74,0x69,0x65,0x72, 0x0a, 0x00,
                        0x01, 0x69, 0xfa, 0x6e,0x70,0x63,0x68,0x65,0x61,0x6c,0x74,0x68, 0x0a, 0x00},
                pd.Dat.Data)

// Expected idx:
//   p2(size=2)             — 00 02
//   id=0 entry length:     — p2(12) = 00 0c
//   id=1 entry length:     — p2(14) = 00 0e
require.Equal(t, []byte{0x00, 0x02, 0x00, 0x0c, 0x00, 0x0e}, pd.Idx.Data)
```

Mirror for `.vars`.

### §7.3 Integration test (`PackConfigs` end-to-end)

`pack_configs_test.go`: temp `srcDir` with `scripts/test.varn` + `scripts/test.vars` + matching `pack/varn.pack` + `pack/vars.pack`. Run `PackConfigs(srcDir, outDir)`. Assert `outDir/server/varn.{dat,idx}` + `outDir/server/vars.{dat,idx}` exist with expected byte content. Re-run `PackConfigs` immediately and assert mtimes are unchanged (`ShouldBuild` returns false because output is fresher than source).

### §7.4 Cross-package consumer test

`pack_configs_loader_roundtrip_test.go`: after running `PackConfigs(srcDir, outDir)`, call `objtype.LoadVarnTypes(outDir)` / `LoadVarsTypes(outDir)` and assert (a) no error, (b) `len(.Configs) == 2`, (c) `.Configs[0].DebugName == "npctier"`, (d) `.Configs[0].Type == ScriptVarTypeInt`. This binds packer-output ↔ existing-loader parity per `rsbuf_roundtrip_tests` (encode-side parity tests must round-trip via the in-tree decoder).

### §7.5 Deviation-tag pin tests

Per `ts_asymmetry_dual_pin`, every NAI-192-D tag also gets an absence-pin test (e.g. `TestNAI192_PackFileSingletonsDeferred_NoModuleLevelVarnPack` asserts no `pkg/pack.VarnPack` package-level identifier exists via `reflect.ValueOf(pkg/pack/...).FieldByName(...)` style probe, or — simpler — a grep-based test reading the package's source via `go/parser` and asserting no `var VarnPack` / `var VarsPack` top-level decl). Pins escalate to failures if a future drive-by adds them.

Pinned tags:
- `NAI-192-D-PACKFILE-SINGLETONS-DEFERRED`
- `NAI-192-D-VARP-UNIQUENESS-DEFERRED`
- `NAI-192-D-DEADBRANCH-OMITTED`
- `NAI-192-D-PACKET-WRITE-CURSOR`

## §8 File inventory

```
pkg/pack/
  packed_data.go                       NEW
  packed_data_test.go                  NEW
  config_value.go                      NEW
  constants.go                         NEW
  constants_test.go                    NEW
  read_typed.go                        NEW
  read_typed_test.go                   NEW
  varn.go                              NEW
  varn_test.go                         NEW
  vars.go                              NEW
  vars_test.go                         NEW
  pack_configs.go                      NEW
  pack_configs_test.go                 NEW
  pack_configs_loader_roundtrip_test.go NEW

pkg/objtype/
  scriptvartype.go                     NEW (move ScriptVarType + 25 consts here)
  scriptvartype_test.go                NEW
  paramtype.go                         MODIFY (remove the moved decls)
```

No changes to NAI-191 foundation files. No changes outside `pkg/pack/` and `pkg/objtype/`. No production callsite added.

## §9 Risk register

| # | Risk | Mitigation | Verified |
|---|---|---|---|
| R1 | Packet write-cursor: TS `dat.pos` vs goscape `Dat.Length()` (memory `packet_rw_pointer_gotcha`). | `NAI-192-D-PACKET-WRITE-CURSOR` tag + comment + byte-pin tests. | §3.3 read of `pkg/io/packet/packet.go:288-340` confirms write methods advance `len(Data)` via `grow`/`tryGrowByReslice`. |
| R2 | `pjstr` terminator: TS `pjstr` writes LF (0x0a); goscape has `PJStr(s, term)`, `PJStrLF`, `PJStrNUL`. Wrong terminator → loader corruption. (Original spec draft had this wrong — corrected after re-reading `io/Packet.ts:330-337`.) | `PackedData.PJStr` calls `PJStrLF` explicitly. Byte-pin test asserts `0x0a` after string. Existing decoder `pkg/objtype/varntype.go:21` already reads `GJStrLF()` — wire-format round-trip is the binding evidence. | TS `io/Packet.ts:336` writes `setUint8(pos++, 10)`. Goscape `PJStrLF` at packet.go:395 writes `PJStr(str, 10)`. |
| R3 | Empty-array TS branches (`stringKeys`/`numberKeys`/`booleanKeys`) — should they be ported as dead code? | Omitted per `NAI-192-D-DEADBRANCH-OMITTED`. Future schema additions revive them with populated arrays. | §4.6 footnote. |
| R4 | Cross-domain var-name uniqueness — landing one packer-of-three without the check leaks duplicate names. | `NAI-192-D-VARP-UNIQUENESS-DEFERRED` tag + TODO comment at orchestrator. Real production has no `.varn` / `.vars` callsite this slice (no `validateConfigPack`); test fixtures don't trigger the dup case. | `PackShared.ts:292-310` reading. Check lands with whichever of `{varp, varn, vars}` is **last** to ship. |
| R5 | Stale `varn.dat`/`vars.dat` fixtures in `data/pack/server/` ≠ what the new packer would produce for the same source. No source exists (no `pack/varn.pack` in tree). | Tests use `t.TempDir()` srcDir + outDir exclusively. The repo fixtures stay frozen until a real production callsite ships. | `find` confirms zero `*.pack` source files in tree (§3.6). |
| R6 | TS `ConfigValue` discriminated union vs Go `any` — type assertions in pack callbacks can panic on schema drift. | `packVarnConfigs` asserts `line.Value.(objtype.ScriptVarType)`. The only call site is `parseVarnConfig`, which only ever returns a `ScriptVarType` for `key == "type"`. Loop guards on `line.Key == "type"`, so an asserted non-`type` value can't appear. Future NAI-193+ packers that branch on multiple keys must guard each branch. | §4.6 code block read. |
| R7 | `validateConfigPack` deferred — no production path generates the `<srcDir>/pack/<type>.pack` registry file. `PackConfigs` can't run against a real source tree yet. | Documented as **test-only this slice**. Real wiring lands with `validateConfigPack` port (deferred from NAI-191 §2). | NAI-191 spec §2 row 1. |

## §10 Deviations from TS source

| Tag | What | Why |
|---|---|---|
| `NAI-192-D-PACKFILE-SINGLETONS-DEFERRED` | No package-level `VarnPack`/`VarsPack` `*PackFile`. Packer functions take `*PackFile` as a parameter. | NAI-191 §2 deferred all 26 module-level pack singletons. Will land alongside `revalidatePack` and cross-domain uniqueness check. |
| `NAI-192-D-VARP-UNIQUENESS-DEFERRED` | Cross-domain var-name uniqueness check across `{VarpPack, VarnPack, VarsPack}` not implemented. | Requires `VarpPack`. Land with last of three. |
| `NAI-192-D-DEADBRANCH-OMITTED` | Empty `stringKeys`/`numberKeys`/`booleanKeys` branches in TS `parseVarnConfig`/`parseVarsConfig` omitted. | Dead code (empty arrays). Revive with the first schema addition that needs them. |
| `NAI-192-D-PACKET-WRITE-CURSOR` | `PackedData.Next()` uses `Dat.Length()` (Go) where TS uses `dat.pos`. | Memory `packet_rw_pointer_gotcha`. `Pos` is the read cursor in goscape; writes append to `len(Data)`. |

## §11 References

- TS source: `LostCityRS/Engine-TS/tools/pack/config/VarnConfig.ts`, `VarsConfig.ts`, `PackShared.ts`, `src/cache/config/ScriptVarType.ts`.
- NAI-191 spec: `docs/superpowers/specs/2026-05-13-nai-191-pack-pipeline-foundation-design.md`.
- Existing loaders (round-trip target): `pkg/objtype/varntype.go`, `pkg/objtype/varstype.go`.
- Memories: `packet_rw_pointer_gotcha`, `rsbuf_roundtrip_tests`, `ts_asymmetry_dual_pin`, `controller_preflight`, `risk_register_premise_grep`, `compressed_cadence`.
