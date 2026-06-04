# NAI-193 — .varp packer slice

**Date:** 2026-05-13
**Status:** Spec (awaiting plan)
**Tech stack:** Go 1.26+
**TS source:** `LostCityRS/Engine-TS/tools/pack/config/VarpConfig.ts` (~95 LOC body), `tools/pack/config/PackShared.ts:261-669` (`packConfigs` orchestrator, varp branch at 615-635 + cross-domain uniqueness check at 292-310), `src/io/Jagfile.ts:80-230` (TS Jagfile.new + Save).
**Predecessors:** NAI-191 (pack-pipeline source-side foundation), NAI-192 (varn + vars packers + PackShared infrastructure).
**HEAD at spec-write:** `fb2cf43`

## §1 Goal

Port `tools/pack/config/VarpConfig.ts` onto the NAI-192 PackShared infrastructure. Adds the first dual-output packer (server `.dat`/`.idx` loose files + a client jagfile entry under `<outDir>/client/config`) — the first slice that exercises goscape's `pkg/io/jagfile` writer.

Single-config slice (unlike NAI-192's varn+vars pair). The schema is materially different from varn/vars (4 opcode types vs varn's 1), and the client-side jagfile threading is novel surface that I want isolated from sibling per-config ports.

Same slice retires `NAI-192-D-VARP-UNIQUENESS-DEFERRED` — varp is the third and final var-domain packer, so the cross-domain `{VarpPack, VarnPack, VarsPack}` name-uniqueness check lands now at the top of `PackConfigs`.

## §2 Out of scope

| Concern | TS location | Why deferred | Tag |
|---|---|---|---|
| `VarpPack` module-level singleton | `tools/pack/PackFile.ts:231-256` | Continuation of NAI-191 §2 / NAI-192 deferral of all 26 module-level pack singletons. `packVarpConfigs` takes `*PackFile` as an explicit parameter. | `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED` |
| `BUILD_VERIFY` checksum validate callback (TS magic `705633567`) | `PackShared.ts:251-253, 631-633` | Continuation of NAI-191 §2 deferral. Magic CRC value preserved in the deviation-tag body for revival when `BUILD_VERIFY` lands. | `NAI-193-D-VALIDATE-DEFERRED` |
| `validateConfigPack` (auto-generated `<srcDir>/pack/varp.pack`) | `tools/pack/PackFile.ts:8-228` | NAI-191 §2 deferred all 6 `validate*` functions. Tests hand-craft `pack/varp.pack` (matches NAI-192 pattern). | (continuation) |
| Production callsite (build CLI, `::rebuild` cheat) | `ClientCheatHandler.ts:151-153` | Closes the arc. Standalone slice. | (continuation) |
| Other client-side configs (varbit/mes/mesanim/param/loc/flo/spotanim/seq/npc/etc.) | `PackShared.ts:440-610` | Each is its own per-config slice in the NAI-194+ track. | (per-slice) |
| Preserving pre-existing entries in `<outDir>/client/config` jagfile | TS opens fresh `Jagfile.new()` — does not load existing | Mirroring TS this slice. Real production callsite will rebuild the full client jagfile per `rebuildClient`. | `NAI-193-D-FRESH-CLIENT-JAGFILE` |

**Retired this slice:** `NAI-192-D-VARP-UNIQUENESS-DEFERRED` — varp is the third and final of the var-name trio. The cross-domain uniqueness check across `{VarpPack, VarnPack, VarsPack}` lands at the top of `PackConfigs` after the three `*PackFile` instances are constructed.

## §3 Pre-flight audit

Per `controller_preflight` + `risk_register_premise_grep`, every premise below was re-verified against HEAD `fb2cf43`.

### §3.1 TS schema (`VarpConfig.ts`)

`parseVarpConfig`:
- `clientcode` — numberKeys. Decimal or `0x`-prefixed hex (regex `/^-?[0-9a-fA-F]+$/` on the slice after `0x`). `Number.isNaN` rejected. Goscape uses `strconv.ParseInt(value, 0, 64)` which natively handles `0x` and decimal with a single call.
- `protect`, `transmit` — booleanKeys. Via `isConfigBoolean` / `getConfigBoolean` (already ported in NAI-192 §4.2).
- `scope` — `"perm"` → `VarPlayerType.SCOPE_PERM` (1), `"temp"` → `VarPlayerType.SCOPE_TEMP` (0). Unknown → `null` (reject).
- `type` — `ScriptVarType.getTypeChar(value)`. Goscape `ScriptVarTypeFromName` already returns `(ScriptVarType, bool)`; unknown → reject.
- Unknown key — `undefined` (TS) → `ok=false, err=nil` (Go, per NAI-192 `ParseFn` contract).

`packVarpConfigs(configs)` returns `{client, server}` PackedData pair. Server opcodes:
- `scope` → `p1(1) p1(value)`
- `type` → `p1(2) p1(value)`
- `protect=false` → `p1(4)` (no payload; opcode emitted *only* when value is false)
- `transmit=true` → `p1(6)` (no payload; opcode emitted *only* when value is true)
- debugname trailer (when slot has a name) → `p1(250) pjstr(debugname)`

Client opcode:
- `clientcode` → `p1(5) p2(value)` — client side does NOT emit a debugname trailer.

Both buffers call `Next()` after each slot, including empty slots.

### §3.2 Loader parity target (`pkg/objtype/varptype.go`)

`LoadVarpTypes(dir)` (lines 60-86):
- Reads server side: `packet.Load(<dir>/server/varp.dat)`.
- Reads client side: `jagfile.LoadJagfile(<dir>/client/config)`, then `clientJag.Read("varp.dat")`.
- `parseVarpTypes` reads `count := server.G2()` and `client.Pos = 2` (skip the 2-byte client count header). Both server and client `.dat` payloads start with a `p2(count)` header — which is exactly what `NewPackedData(size)` already emits.
- Per-slot: `DecodeType(server, config)` consumes server opcodes (1, 2, 4, 6, 250) until the 0x00 terminator; `DecodeType(client, config)` consumes client opcodes (5) until terminator. The `*PackedData.Next()` 0x00-terminator from NAI-192 maps to this read-until-zero loop.

Binding: NAI-193 test `TestVarpPacker_LoaderRoundTrip` runs `PackConfigs(srcDir, outDir)` against a hand-crafted fixture, then calls `objtype.LoadVarpTypes(outDir)` and asserts each varp's `Scope`/`Type`/`Protect`/`ClientCode`/`Transmit`/`DebugName` round-trips correctly.

### §3.3 `pkg/io/jagfile` writer gap (infrastructure dependency)

`Jagfile.Save` (`pkg/io/jagfile/jagfile.go:122-167`) indexes into `jf.FileHash[index] = queued.Hash` and `jf.FileWrite[index] = queued.Data` without first growing the slices. Constructing `&Jagfile{}` empty (or `NewJagfile(nil)` per line 268 → returns `&Jagfile{}`) then calling `Write` → `Save` panics with index-out-of-range because `jf.FileHash` is nil/zero-length when the first write tries to assign at `index = 0`.

TS works because JS arrays auto-grow on indexed assignment. Goscape slices don't.

**Fix lands in this slice as the first task** (see §4.1). Mechanical: when `index == jf.FileCount` and the per-field slices have length `< jf.FileCount+1`, append a zero element to each (`FileHash`, `FileName`, `FileUnpackedSize`, `FilePackedSize`, `FilePos`, `FileWrite`). TS-faithful — fixes a goscape-only panic.

### §3.4 Fresh-jagfile semantics

TS `packConfigs` (`PackShared.ts:336`) opens with `const jag = Jagfile.new()` — fresh, not loaded from disk. When `packConfigs` runs partially (e.g. only `.varp` source changed; varbit/mes/mesanim unchanged), TS writes a jagfile containing *only* `varp.dat`/`varp.idx` — losing every other entry that was in the previous client jagfile.

This is TS behavior. Goscape mirrors it via `NAI-193-D-FRESH-CLIENT-JAGFILE`. The risk is academic this slice (no production caller); when `rebuildClient` and the full client-config arc lands, it'll force all branches to fire together so the truncation doesn't matter in practice.

A future "preserve-existing-entries" deviation (load existing jagfile, merge writes, save back) has a clear anchor in this tag.

### §3.5 Cross-package byte-parity (server side)

`data/pack/server/varp.{dat,idx}` exists in repo (consumed by `LoadVarpTypes`). NAI-192 used hand-crafted fixtures matching cache bytes for varn/vars parity. NAI-193 does the same for the *server* side via the cross-package consumer test in §7.4.

**Client side parity is bound via an intra-package round-trip** (build jagfile via `packVarpConfigs` + `jagfile.Jagfile.Save` → re-load via `LoadJagfile` → `jag.Read("varp.dat")` → byte-equal a hand-built reference), not against `data/pack/client/config`'s embedded `varp.dat`. Reason: the cache jagfile's outer container is BZip2-compressed; byte-stability of the compressed container across compressor implementations is not under test in this slice and not a goscape correctness target. Per-entry byte parity (`varp.dat` payload inside the jagfile) IS bound, just not at the outer container level.

If the cross-package outer-container byte parity is wanted, it can be added later as a stretch test once `BUILD_VERIFY` lands — the CRC validate callback at `PackShared.ts:631-633` already encodes the "what counts as parity?" answer for the canonical build.

### §3.6 No existing `varp.pack` source file

`find` returns zero `varp.pack` files in either goscape or `LostCityRS/Engine-TS` (matches NAI-192 §3.6 finding for varn/vars). Tests construct hand-crafted `pack/varp.pack` in `t.TempDir()`.

### §3.7 ScriptVarType char codes

All 25 hard-coded `ScriptVarType` char codes are already exported from `pkg/objtype/scriptvartype.go` (NAI-192 T1). `ScriptVarTypeFromName` already accepts every TS-known type-name. No `pkg/objtype` changes this slice.

## §4 Components

### §4.1 Jagfile writer auto-grow (`pkg/io/jagfile/jagfile.go`)

Inside `Save`'s write branch (current lines 126-143), when `index == jf.FileCount` and a per-field slice length is below `jf.FileCount+1`, append a zero element:

```go
if queued.Write {
    if index == -1 {
        index = jf.FileCount
        jf.FileCount++
        // Grow per-field slices on demand (TS arrays auto-grow on indexed
        // assignment; goscape slices need explicit append).
        if len(jf.FileHash) < jf.FileCount {
            jf.FileHash = append(jf.FileHash, 0)
            jf.FileName = append(jf.FileName, "")
            jf.FileUnpackedSize = append(jf.FileUnpackedSize, 0)
            jf.FilePackedSize = append(jf.FilePackedSize, 0)
            jf.FilePos = append(jf.FilePos, 0)
            jf.FileWrite = append(jf.FileWrite, nil)
        }
        jf.FileHash[index] = queued.Hash
        jf.FileName[index] = queued.Name
    }
    // ...rest unchanged
}
```

No deviation tag — TS-faithful fix to a goscape-only panic.

Existing `pkg/io/jagfile` tests must stay green. One new test in the same package covers "fresh-empty Jagfile via `NewJagfile(nil)` + `Write(\"a.dat\", pA)` + `Write(\"b.dat\", pB)` + `Save(path, false)` + `LoadJagfile(path)` + `Read(\"a.dat\")`/`Read(\"b.dat\")` round-trip → byte-equal".

### §4.2 `parseVarpConfig` (`pkg/pack/varp.go`)

```go
func parseVarpConfig(key, value string) (ConfigValue, bool, error) {
    switch key {
    case "clientcode":
        n, err := strconv.ParseInt(value, 0, 64)
        if err != nil {
            return nil, true, fmt.Errorf("invalid clientcode: %s", value)
        }
        return int(n), true, nil
    case "protect", "transmit":
        if !IsConfigBoolean(value) {
            return nil, true, fmt.Errorf("invalid boolean: %s", value)
        }
        return GetConfigBoolean(value), true, nil
    case "scope":
        switch value {
        case "perm":
            return objtype.VarpScopePerm, true, nil
        case "temp":
            return objtype.VarpScopeTemp, true, nil
        default:
            return nil, true, fmt.Errorf("invalid scope: %s", value)
        }
    case "type":
        t, ok := objtype.ScriptVarTypeFromName(value)
        if !ok {
            return nil, true, fmt.Errorf("unknown script var type: %s", value)
        }
        return t, true, nil
    }
    return nil, false, nil // unknown key
}
```

Notes:
- `strconv.ParseInt(value, 0, 64)` accepts decimal *and* `0x`-prefixed hex; rejects non-numeric. Equivalent to the TS `value.startsWith('0x') ? parseInt(value, 16) : parseInt(value)` plus regex validation plus `Number.isNaN` check.
- `objtype.VarpScopePerm` / `VarpScopeTemp` are already exported from `pkg/objtype/varptype.go:11-13`.

### §4.3 `packVarpConfigs` (`pkg/pack/varp.go`)

```go
func packVarpConfigs(configs map[string][]ConfigLine, pf *PackFile) (server, client *PackedData) {
    server = NewPackedData(pf.Max)
    client = NewPackedData(pf.Max)

    for id := range pf.Max {
        name := pf.GetByID(id)
        if cfg, ok := configs[name]; ok {
            for _, line := range cfg {
                switch line.Key {
                case "scope":
                    server.P1(1)
                    server.P1(uint8(line.Value.(int)))
                case "type":
                    server.P1(2)
                    server.P1(uint8(line.Value.(objtype.ScriptVarType)))
                case "protect":
                    if !line.Value.(bool) {
                        server.P1(4)
                    }
                case "clientcode":
                    client.P1(5)
                    client.P2(uint16(line.Value.(int)))
                case "transmit":
                    if line.Value.(bool) {
                        server.P1(6)
                    }
                }
            }
        }
        if len(name) > 0 {
            server.P1(250)
            server.PJStr(name)
        }
        server.Next()
        client.Next()
    }
    return server, client
}
```

Notes:
- Returns server-first to match `parseVarpTypes` read order in `pkg/objtype/varptype.go:84-104` (server.G2 count then per-slot server-decode-then-client-decode).
- TS comment at `VarpConfig.ts:97` (`// todo: maybe this was opcode 10?`) preserved as a Go comment alongside the `250` literal — flagged as a known TS-author uncertainty, not a goscape deviation.

### §4.4 `PackConfigs` orchestrator extension (`pkg/pack/pack_configs.go`)

Signature unchanged: `PackConfigs(srcDir, outDir string) error`.

Internal structural changes:

```go
func PackConfigs(srcDir, outDir string) error {
    constants, err := LoadConstants(srcDir)
    if err != nil {
        return err
    }

    scriptsDir := filepath.Join(srcDir, "scripts")
    serverOut := filepath.Join(outDir, "server")
    clientOut := filepath.Join(outDir, "client")

    // Construct all three var-domain PackFiles up-front so the cross-domain
    // uniqueness check has all three name maps available. Each *.pack file
    // is small (<1 KB); cost is fixed regardless of which branches fire.
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

    // Cross-domain var-name uniqueness check — retires
    // NAI-192-D-VARP-UNIQUENESS-DEFERRED.
    //
    // TS source: PackShared.ts:292-310.
    if err := checkVarNameUniqueness(varpPack, varnPack, varsPack); err != nil {
        return err
    }

    // Fresh client jagfile — TS-faithful per NAI-193-D-FRESH-CLIENT-JAGFILE.
    // Saved only if at least one client-side branch fires this invocation.
    clientJag, err := jagfile.NewJagfile(nil)
    if err != nil {
        return err
    }
    clientJagDirty := false

    // .varp — server + client outputs.
    if GetLatestModified(scriptsDir, ".varp") > 0 &&
        ShouldBuild(scriptsDir, ".varp", filepath.Join(serverOut, "varp.dat")) {
        if err := packAndSaveVarp(srcDir, serverOut, varpPack, constants, clientJag); err != nil {
            return err
        }
        clientJagDirty = true
    }

    // .varn / .vars — unchanged from NAI-192, but now reuse the up-front
    // PackFiles instead of constructing per-branch.
    if GetLatestModified(scriptsDir, ".varn") > 0 &&
        ShouldBuild(scriptsDir, ".varn", filepath.Join(serverOut, "varn.dat")) {
        if err := packAndSaveVarn(serverOut, varnPack, srcDir, constants); err != nil {
            return err
        }
    }
    if GetLatestModified(scriptsDir, ".vars") > 0 &&
        ShouldBuild(scriptsDir, ".vars", filepath.Join(serverOut, "vars.dat")) {
        if err := packAndSaveVars(serverOut, varsPack, srcDir, constants); err != nil {
            return err
        }
    }

    if clientJagDirty {
        if err := clientJag.Save(filepath.Join(clientOut, "config"), false); err != nil {
            return err
        }
    }
    return nil
}

func checkVarNameUniqueness(pfs ...*PackFile) error {
    seen := map[string]string{} // name → which pack first declared it
    for _, pf := range pfs {
        for id := range pf.Max {
            name := pf.GetByID(id)
            if name == "" {
                continue
            }
            if prior, dup := seen[name]; dup {
                return fmt.Errorf("non-unique var name %q (declared in %s and again)", name, prior)
            }
            seen[name] = filepath.Base(pf.SourcePath())
        }
    }
    return nil
}

func packAndSaveVarp(srcDir, serverOut string, pf *PackFile, c Constants, clientJag *jagfile.Jagfile) error {
    cfgs, err := ReadTypedConfigs(srcDir, ".varp", nil, parseVarpConfig, c)
    if err != nil {
        return err
    }
    server, client := packVarpConfigs(cfgs, pf)
    if err := server.Save(
        filepath.Join(serverOut, "varp.dat"),
        filepath.Join(serverOut, "varp.idx"),
    ); err != nil {
        return err
    }
    clientJag.Write("varp.dat", client.Dat)
    clientJag.Write("varp.idx", client.Idx)
    return nil
}
```

Notes:
- `packAndSaveVarn` / `packAndSaveVars` signatures change to accept the pre-constructed `*PackFile` (refactor: shift `NewPackFile` calls out of the per-branch helpers). The behavior is unchanged for `.varn` / `.vars`.
- `checkVarNameUniqueness` uses `PackFile.SourcePath()` for the error message. If that accessor doesn't exist on `*PackFile`, we add a trivial getter; alternative is to pass a name string per `*PackFile` arg.
- The orchestrator's existing `NAI-192-D-VARP-UNIQUENESS-DEFERRED` comment block is replaced by a comment pointing at the new `checkVarNameUniqueness` call and noting the tag is retired.

### §4.5 Deviation-tag pin tests (`pkg/pack/nai193_deviation_pins_test.go` — new file alongside NAI-192's)

New file (separate from `nai192_deviation_pins_test.go` to keep each slice's pins discoverable by file name).

Pinned tags this slice:
- `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED` — assert no `VarpPack` top-level identifier in `pkg/pack` (reuses `scanPackageDecls` helper from NAI-192's pin file).
- `NAI-193-D-VALIDATE-DEFERRED` — assert no `validateVarp` / `BuildVerify` / `checkCRC` identifier in `pkg/pack/varp.go` body.
- `NAI-193-D-FRESH-CLIENT-JAGFILE` — assert `PackConfigs` source contains `NewJagfile(nil)` and does NOT contain `LoadJagfile` (the latter would indicate the deviation has flipped to "preserve existing entries").

Also retired: `NAI-192-D-VARP-UNIQUENESS-DEFERRED`. The existing `TestNAI192_VarpUniquenessDeferred_NoCheckInOrchestrator` in `pkg/pack/nai192_deviation_pins_test.go` is **deleted** as part of this slice (the deviation is retired; the pin would now fail). Its retirement is a deliberate plan-task with a one-line commit citing the tag.

## §5 Data flow

```
<srcDir>/scripts/**/*.constant     ──►  LoadConstants  ──►  Constants
<srcDir>/pack/{varp,varn,vars}.pack ──►  NewPackFile × 3 ──►  PackFile × 3
                                                              │
                                                              ▼  uniqueness check
                                                              │  (retires NAI-192-D-VARP-UNIQUENESS-DEFERRED)
                                                              │
                                fresh jagfile.Jagfile (NewJagfile(nil))
                                                              │
                                ┌─────────────────────────────┼─────────────────────────────┐
                                ▼                             ▼                             ▼
<srcDir>/scripts/**/*.varp     ReadTypedConfigs               ReadTypedConfigs (.varn)      ReadTypedConfigs (.vars)
        │                            │                              │                              │
        ▼                            ▼                              ▼                              ▼
        └─────► packVarpConfigs ──► (server, client)        packVarnConfigs ─► server          packVarsConfigs ─► server
                                       │      │                       │                              │
                                       ▼      ▼                       ▼                              ▼
                          server.Save     clientJag.Write   server.Save (varn.{dat,idx})  server.Save (vars.{dat,idx})
                          (varp.{dat,idx})  varp.{dat,idx}
                                              │
                                              ▼
                                       clientJag.Save(<outDir>/client/config)
                                       (skipped if no client-side branch fired)
```

## §6 Error handling

Inherits NAI-192 error envelope (parse-stage errors via `parseStepError` shape).

New error sites this slice:

| Site | Behavior | TS parity |
|---|---|---|
| `parseVarpConfig` clientcode non-numeric / non-hex | `"Error during parsing - see %s:%d\nInvalid property value: ..."` (envelope by `ReadTypedConfigs`) | ✅ |
| `parseVarpConfig` scope unknown | `"...Invalid property value: ..."` | ✅ |
| `parseVarpConfig` protect/transmit non-boolean | `"...Invalid property value: ..."` | ✅ |
| `parseVarpConfig` type unknown | `"...Invalid property value: ..."` | ✅ |
| `checkVarNameUniqueness` duplicate across packs | `fmt.Errorf("non-unique var name %q (declared in %s and again)", name, prior)` | ✅ (TS throws `"Non-unique var name found: <name>"` — equivalent rejection, slightly richer goscape message) |
| `jagfile.Jagfile.Save` write error | Propagate | ✅ |
| `clientJag.Save` when no client-side branch fired | Skipped — no save call. No empty-jagfile-write artifact. | n/a (TS unconditionally saves at end of `packConfigs`; goscape is stricter — minor optimization, no behavioral risk for production callers because `rebuildClient` would force all branches) |

## §7 Testing strategy

### §7.1 `pkg/io/jagfile` writer fix (T1)

`pkg/io/jagfile/jagfile_test.go` gains:

```go
func TestJagfile_FreshEmptyWriteSaveRoundTrip(t *testing.T) {
    jf, err := NewJagfile(nil)
    require.NoError(t, err)

    a := packet.NewPacket([]byte{0xAA, 0xBB})
    b := packet.NewPacket([]byte{0xCC, 0xDD, 0xEE})
    jf.Write("a.dat", a)
    jf.Write("b.dat", b)

    path := filepath.Join(t.TempDir(), "config")
    require.NoError(t, jf.Save(path, false))

    reloaded, err := LoadJagfile(path)
    require.NoError(t, err)
    gotA, err := reloaded.Read("a.dat")
    require.NoError(t, err)
    require.Equal(t, []byte{0xAA, 0xBB}, gotA.Data)
    gotB, err := reloaded.Read("b.dat")
    require.NoError(t, err)
    require.Equal(t, []byte{0xCC, 0xDD, 0xEE}, gotB.Data)
}
```

Red-green: pre-fix → panic. Post-fix → green.

### §7.2 `parseVarpConfig` per-key coverage (`pkg/pack/varp_test.go`)

Table-driven:

| Input | Expected `(value, ok, err)` |
|---|---|
| `("clientcode", "7")` | `(int(7), true, nil)` |
| `("clientcode", "0x42")` | `(int(66), true, nil)` |
| `("clientcode", "-5")` | `(int(-5), true, nil)` |
| `("clientcode", "abc")` | `(nil, true, non-nil)` |
| `("protect", "yes")` | `(true, true, nil)` |
| `("protect", "false")` | `(false, true, nil)` |
| `("protect", "maybe")` | `(nil, true, non-nil)` |
| `("transmit", "1")` | `(true, true, nil)` |
| `("scope", "perm")` | `(objtype.VarpScopePerm, true, nil)` |
| `("scope", "temp")` | `(objtype.VarpScopeTemp, true, nil)` |
| `("scope", "global")` | `(nil, true, non-nil)` |
| `("type", "int")` | `(objtype.ScriptVarTypeInt, true, nil)` |
| `("type", "bogus")` | `(nil, true, non-nil)` |
| `("unknownkey", "x")` | `(nil, false, nil)` |

### §7.3 `packVarpConfigs` byte-pin (`pkg/pack/varp_test.go`)

Fixture: 2 varp slots. Slot 0 = `run` (scope=temp, type=int, transmit=yes, clientcode=7). Slot 1 = empty.

Expected server dat (1 slot populated + 1 empty slot, both terminated):
- `00 02` — count header (size=2)
- Slot 0 body: `01 00` (scope=temp), `02 69` (type=int=105), `06` (transmit=true), `fa 72 75 6e 0a` (debugname "run" + LF)
- Slot 0 terminator: `00`
- Slot 1 (empty, no name): `00`

Expected server idx:
- `00 02` count header
- Slot 0 size: `p2(0x0a)` — 10 bytes payload (`01 00 02 69 06 fa 72 75 6e 0a`) + terminator counts toward `Next()`'s `Dat.Length() - marker` — count includes the terminator. So `p2(0x0b)` = 11.
- Slot 1 size: `p2(1)` (just the terminator)

Expected client dat:
- `00 02` count header
- Slot 0 body: `05 00 07` (clientcode=5, then p2(7) = `00 07`)
- Slot 0 terminator: `00`
- Slot 1: `00`

Expected client idx:
- `00 02` count header
- Slot 0 size: `p2(4)` (3-byte payload + 1-byte terminator)
- Slot 1 size: `p2(1)`

Pin both `server.Dat.Data` / `server.Idx.Data` / `client.Dat.Data` / `client.Idx.Data` byte-for-byte.

(Note for plan-author: the exact byte counts above depend on whether `PackedData.Next()`'s `Dat.Length() - marker` includes or excludes the terminator. Plan must verify against the NAI-192 implementation at `pkg/pack/packed_data.go` — the existing varn byte-pin test already established the answer, and this slice's pins must agree.)

### §7.4 Cross-package loader round-trip (`pkg/pack/varp_test.go`)

```go
func TestVarpPacker_LoaderRoundTrip(t *testing.T) {
    srcDir := t.TempDir()
    // hand-craft scripts/test.varp + pack/varp.pack
    // ... (id 0 = "run" with scope=perm, type=int, transmit=yes, clientcode=7)
    outDir := t.TempDir()
    require.NoError(t, pack.PackConfigs(srcDir, outDir))

    cfgs, err := objtype.LoadVarpTypes(outDir)
    require.NoError(t, err)
    require.Len(t, cfgs.Configs, 1)
    require.Equal(t, "run", cfgs.Configs[0].DebugName)
    require.Equal(t, objtype.VarpScopePerm, cfgs.Configs[0].Scope)
    require.Equal(t, objtype.ScriptVarTypeInt, cfgs.Configs[0].Type)
    require.True(t, cfgs.Configs[0].Transmit)
    require.Equal(t, uint16(7), cfgs.Configs[0].ClientCode)
    require.Equal(t, 0, cfgs.RunID) // clientcode==7 ⇒ engine run-mode varp
}
```

### §7.5 `PackConfigs` integration (`pkg/pack/pack_configs_test.go` — extend)

- `TestPackConfigs_VarpOnly`: hand-crafted `.varp` source only. Asserts `<outDir>/server/varp.{dat,idx}` and `<outDir>/client/config` jagfile both exist; jagfile contains `varp.dat` + `varp.idx`. Re-run idempotent (mtimes stable).
- `TestPackConfigs_MixedVarpVarnVars`: all three source types present + matching `pack/*.pack` files. Asserts all six server `.{dat,idx}` files + client jagfile (containing `varp.dat`+`varp.idx`).
- `TestPackConfigs_NoClientBranchSkipsJagfileSave`: only `.varn` source present. Asserts `<outDir>/server/varn.{dat,idx}` exists; `<outDir>/client/config` does NOT exist.
- `TestPackConfigs_CrossDomainUniquenessRejection`: hand-crafted fixture where `pack/varp.pack` and `pack/varn.pack` both declare the same debugname `dup_name`. Asserts `PackConfigs` returns an error mentioning `dup_name`.

### §7.6 Deviation-tag pin tests

New file `pkg/pack/nai193_deviation_pins_test.go` per §4.5. Existing `TestNAI192_VarpUniquenessDeferred_NoCheckInOrchestrator` is deleted (deviation retired).

## §8 File inventory

```
pkg/io/jagfile/
  jagfile.go                              MODIFY (auto-grow in Save's write branch)
  jagfile_test.go                         MODIFY (add fresh-empty round-trip test)

pkg/pack/
  varp.go                                 NEW (parseVarpConfig + packVarpConfigs)
  varp_test.go                            NEW
  pack_configs.go                         MODIFY (cross-domain uniqueness, fresh jagfile,
                                                  varp branch, refactor varn/vars helpers
                                                  to accept pre-constructed *PackFile)
  pack_configs_test.go                    MODIFY (add varp/mixed/no-client-branch/uniqueness tests)
  nai192_deviation_pins_test.go           MODIFY (delete TestNAI192_VarpUniquenessDeferred_...)
  nai193_deviation_pins_test.go           NEW

docs/superpowers/specs/
  2026-05-13-nai-193-varp-packer-design.md  NEW (this file)
```

Outside `pkg/io/jagfile/` (one file) and `pkg/pack/` (the per-slice surface), no production code changes. No new exported API on `pkg/objtype`.

## §9 Risk register

| # | Risk | Mitigation | Verified |
|---|---|---|---|
| R1 | `jagfile.Jagfile.Save` panic on fresh-empty jagfile (§3.3) | §4.1 fix lands as T1 with a red-green test that panics pre-fix. | §3.3 read of `pkg/io/jagfile/jagfile.go:122-167`. |
| R2 | Client-side payload byte parity not verified against the cache fixture (§3.5) | Intra-package round-trip pins payload bytes; cache-fixture container parity deferred to `BUILD_VERIFY` slice. Server-side parity retained via §7.4. | §3.5 reasoning. |
| R3 | TS `parseInt(value, 16)` allows uppercase hex; `strconv.ParseInt(value, 0, 64)` also accepts uppercase. ✅ | None — semantics match. | Go stdlib docs. |
| R4 | `protect`/`transmit` asymmetric emission (`protect=false` → emit 4; `transmit=true` → emit 6) is easy to invert. | Byte-pin test §7.3 covers both arms; round-trip test §7.4 binds the default-construction semantics (`NewVarPlayerType` defaults `Protect=true, Transmit=false`, so absent opcode = default, present opcode = inverted-from-default). | TS `VarpConfig.ts:88-100` + `pkg/objtype/varptype.go:47-53`. |
| R5 | Cross-domain uniqueness check error message format diverges from TS | Tag `NAI-193-D-UNIQUENESS-MSG-FORMAT` if reviewer flags. Current Go message is semantically equivalent (rejection on duplicate); TS message is `"Non-unique var name found: <name>"`. Goscape's `"non-unique var name %q (declared in %s and again)"` is richer but the rejection itself is parity. | §6 row. |
| R6 | `packAndSaveVarn` / `packAndSaveVars` signature change is a refactor — easy to miss a caller. | Both helpers are private to `pkg/pack`. Grep confirms only `PackConfigs` calls them. Test stays green if the refactor is mechanical. | grep `packAndSaveV` in `pkg/`. |
| R7 | `PackFile.SourcePath()` accessor may not exist; uniqueness-check error message references it. | Plan T-step adds `SourcePath()` getter (trivial 3-line addition) if grep confirms absence. Alternative: pass a name string per `*PackFile` arg. | §4.4 note; grep `SourcePath` in `pkg/pack/packfile.go`. |
| R8 | Fresh-jagfile semantics (§3.4) truncate any pre-existing entries when only `.varp` is rebuilt. | TS-faithful per `NAI-193-D-FRESH-CLIENT-JAGFILE`. Real production callsite (when wired) forces full rebuild via `rebuildClient`. | §3.4 reasoning. |
| R9 | `clientJagDirty` gate's "no client save when no client branch fired" diverges from TS (TS always saves at end of `packConfigs`). | Behavioral optimization, not a semantic divergence — an empty saved jagfile would be a 7-byte (header) file with `FileCount=0`. Real production always has at least varp+varbit+mes+param firing together. Tag if reviewer flags: `NAI-193-D-SKIP-EMPTY-CLIENTJAG-SAVE`. | §6 last row. |

## §10 Deviations from TS source

| Tag | What | Why |
|---|---|---|
| `NAI-193-D-PACKFILE-SINGLETONS-DEFERRED` | No module-level `VarpPack`. `packVarpConfigs` takes `*PackFile` parameter. | Continuation of NAI-191 §2 deferral. |
| `NAI-193-D-VALIDATE-DEFERRED` | No `BUILD_VERIFY` CRC validate callback (TS magic `705633567` preserved in tag body for revival). | Continuation of NAI-191 §2 deferral. |
| `NAI-193-D-FRESH-CLIENT-JAGFILE` | `PackConfigs` opens a fresh `jagfile.Jagfile` via `NewJagfile(nil)` instead of loading the existing `<outDir>/client/config`. | TS-faithful (`Jagfile.new()`). Pre-existing entries are truncated if only `.varp` rebuilds. Real production runs all client-side branches together. |

**Retired this slice:**
- `NAI-192-D-VARP-UNIQUENESS-DEFERRED` — cross-domain uniqueness check now implemented in `checkVarNameUniqueness`.

## §11 References

- TS source: `LostCityRS/Engine-TS/tools/pack/config/VarpConfig.ts` (111 LOC), `PackShared.ts:261-669` (orchestrator), `PackShared.ts:292-310` (cross-domain uniqueness), `src/io/Jagfile.ts:80-230` (TS Jagfile constructor + Save).
- NAI-191 spec: `docs/superpowers/specs/2026-05-13-nai-191-pack-pipeline-foundation-design.md`.
- NAI-192 spec: `docs/superpowers/specs/2026-05-13-nai-192-varn-vars-packers-design.md`.
- Existing loader (round-trip target): `pkg/objtype/varptype.go`.
- Jagfile writer: `pkg/io/jagfile/jagfile.go`.
- Memories: `packet_rw_pointer_gotcha`, `rsbuf_roundtrip_tests`, `ts_asymmetry_dual_pin`, `controller_preflight`, `risk_register_premise_grep`, `emergent_deviation_mid_impl`, `true_to_ts_gate`, `retire_deviation_grep_all_comments`.

## §12 Acceptance criteria

- `go test ./pkg/io/jagfile/... ./pkg/pack/... ./pkg/objtype/... -count=1 -race` — PASS.
- `go vet ./...` — clean.
- `gofmt -l pkg/io/jagfile pkg/pack pkg/objtype` — empty.
- `rg "NAI-192-D-VARP-UNIQUENESS-DEFERRED" pkg/ modules/ cmd/` — zero matches (deviation retired; per `retire_deviation_grep_all_comments`, grep both production code AND doc comments).
- `rg "NAI-193-D-" pkg/` — three matches (one per new deviation tag).
- All pin tests in `nai193_deviation_pins_test.go` green; the deleted `TestNAI192_VarpUniquenessDeferred_NoCheckInOrchestrator` is removed (commit body cites the retirement).
- `LoadVarpTypes(<outDir>)` round-trips correctly after `PackConfigs` runs against the hand-crafted fixture (binds end-to-end byte-format parity with the production loader).
- No production callsite added; `PackConfigs` remains test-only wired (NAI-194+ track will either thread test-only wiring or stand up the production `cmd/` entry point when the per-config arc closes).
