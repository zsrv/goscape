# NAI-210 — Compiler driver + file-output sinks (slice 6c of 6, FINAL)

**Status:** spec
**Date:** 2026-05-16
**Predecessor:** NAI-209 (`c5beea6`) — closes the binary writer pipeline
**TS pin:** `LostCityRS/RuneScriptTS @ b8c338801fbb72d294ff9576a58925a8d3f6de47`
**Tech stack:** Go 1.26+ (`use-modern-go`)

## 1. Summary

NAI-210 is the **terminal slice** of the six-slice compiler envelope. It lands:

1. `BytePacket` (`crc32` + `ByteWriter`) — retires `NAI-209-D-BYTEPACKET-DEFER`.
2. Three concrete `BinaryOutput` sinks consuming `BinaryScriptWriter` from NAI-209:
   `BinaryFileScriptWriter`, `JagFileScriptWriter`, `Js5PackScriptWriter`.
3. `CompilerTypeInfo` data struct + three `SymbolLoader` implementations
   (`CompilerTypeInfoConstantLoader`, `CompilerTypeInfoLoader`,
   `CompilerTypeInfoProtectedLoader`) + the `SymbolLoader` abstract-base
   helpers (`AddConstant` / `AddBasic`).
4. `ServerScriptCompiler` driver — single Go struct flattening TS
   `ScriptCompiler` + `ServerScriptCompiler`. Carries `Setup()` (outer-half
   type/sym-loader/handler registrations + default type-checkers) and `Run(ext)`
   (full pipeline: load → parse → analyze → codegen → check-pointers → write).
5. `LoadSpecialSymbols` — populates `commandPointers` + seeds `SymbolMapper`
   command/script maps. Retires `NAI-208-D-COMMAND-POINTERS-DEFERRED`.
6. `PrimitiveCategory` singleton — `generateLookupKey` switches from
   `TypeMarker.Category` to per-subject `subject.Type == PrimitiveCategory`.
   Retires `NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR`.
7. Feature-gating wired through `command.RegisterAllDynCommands` (queue-typed,
   enums, structs, db-tables, procs). Retires
   `NAI-207-D-REGISTERALL-NO-FEATURES`.
8. `runescript.Compile(cfg) error` — the inner logic of TS
   `ServerScriptCompilerApplication.CompileServerScript` as a callable Go
   helper. No CLI / `process.exit` / `path.resolve` wrapper.

**Net retirement:** 4 live tags (NAI-209-D-BYTEPACKET-DEFER,
NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR, NAI-208-D-COMMAND-POINTERS-DEFERRED,
NAI-207-D-REGISTERALL-NO-FEATURES). Up to 3 new tags expected (see §11).

**Sizing:** ~785 TS LOC ported (613 explicit + 172 unported prerequisites).
Expect ~1500–1800 Go LOC including tests, comparable to NAI-209 (~993 Go).

## 2. Architecture

```
ServerScriptCompiler  (runescript/server_script_compiler.go)
  ├─ Setup()                                  // type/trigger/handler/loader regs
  │   ├─ registerScriptVarTypes()             // feature-gated
  │   ├─ setupDefaultTypeCheckers()           // 7 checkers (TS L121-184)
  │   ├─ MetaScript regs (proc/label/queue/timer/softtimer/walktrigger)
  │   ├─ addSymConstantLoaders()              // CompilerTypeInfoConstantLoader
  │   ├─ 18× addSymLoader / 3× addSymLoaderWithSupplier
  │   ├─ 2× addProtectedSymLoaderWithSupplier (varp, varbit)
  │   └─ command.RegisterAllDynCommands(c, features)   // inner half (existing)
  └─ Run("rs2")
      ├─ loadSymbols      → SymbolLoader[].Load(rootTable, c)
      ├─ parse(ext)       → parser pkg (existing)
      ├─ analyze(files)   → semantics pkg (existing) + registerSecondaryCommands
      ├─ codegen(files)   → codegen pkg (existing) → []*RuneScript
      ├─ checkPointers    → runescript.ServerPointerChecker (NAI-208)
      └─ write(scripts)   → BinaryScriptWriter.WriteScript → BinaryOutput

BinaryOutput  (existing interface from NAI-209)
  ├─ BinaryFileScriptWriter   // <output>/<id>
  ├─ JagFileScriptWriter      // script.dat + script.idx + Close()
  └─ Js5PackScriptWriter      // packed .js5 archive + Close()

Compile(cfg Config) error  (runescript/compile.go)
  ├─ validate Symbols["command"] + Symbols["runescript"]
  ├─ default CheckPointers=true; default Jag writer to ./data/pack/server
  ├─ resolve paths via filepath.Abs
  ├─ construct SymbolMapper + writer + LoadSpecialSymbols(...)
  ├─ construct ServerScriptCompiler
  └─ Setup() + Run("rs2") + writer.Close() if io.Closer
```

## 3. Package layout

All paths are under `pkg/pack/compiler/`.

| Path | Action |
|---|---|
| `runescript/bytepacket.go` | NEW — `Crc32` + `ByteWriter` |
| `runescript/bytepacket_test.go` | NEW |
| `runescript/binary_file_writer.go` | NEW — per-script-id file sink |
| `runescript/binary_file_writer_test.go` | NEW |
| `runescript/jag_file_writer.go` | NEW — `script.dat` + `script.idx` sink |
| `runescript/jag_file_writer_test.go` | NEW |
| `runescript/js5_pack_writer.go` | NEW — JS5 archive (gzip) sink |
| `runescript/js5_pack_writer_test.go` | NEW |
| `runescript/compiler_type_info.go` | NEW — `CompilerTypeInfo` struct |
| `runescript/type_info_loader.go` | NEW — three `SymbolLoader` impls |
| `runescript/type_info_loader_test.go` | NEW |
| `runescript/load_special_symbols.go` | NEW — `LoadSpecialSymbols` helper |
| `runescript/load_special_symbols_test.go` | NEW |
| `runescript/server_script_compiler.go` | NEW — driver struct + Setup + Run |
| `runescript/server_script_compiler_test.go` | NEW |
| `runescript/setup.go` | NEW — outer-half registrations (called by `Setup`) |
| `runescript/setup_test.go` | NEW |
| `runescript/default_type_checkers.go` | NEW — 7-checker registration helper |
| `runescript/default_type_checkers_test.go` | NEW |
| `runescript/compile.go` | NEW — `runescript.Compile(cfg)` facade |
| `runescript/compile_test.go` | NEW — driver-level smoke (Jag + Js5) |
| `runescript/binary_writer.go` | EDIT — `generateLookupKey` per-subject category check |
| `runescript/binary_writer_lookup_test.go` | EDIT — TYPEMARKER-CATEGORY case lands real |
| `runescript/nai209_deviation_pins_test.go` | EDIT — retire 2 NAI-209 tags |
| `runescript/nai210_deviation_pins_test.go` | NEW — pin NAI-210-introduced tags |
| `symbol/loader.go` | NEW — `SymbolLoader` interface + `AddConstant`/`AddBasic` |
| `symbol/loader_test.go` | NEW |
| `type/primitive.go` | EDIT — add `PrimitiveCategory` + extend iteration list |
| `type/primitive_test.go` | EDIT |
| `command/register.go` | EDIT — wire features.DisableX gates; drop `_ = features` |
| `command/cohort_a_test.go` | EDIT — add feature-gating cases |
| `command/cohort_b_test.go` | EDIT — add feature-gating cases |
| `codegen/nai207_deviation_pins_test.go` | EDIT — retire `NAI-207-D-REGISTERALL-NO-FEATURES` |
| `codegen/smoke_test.go` | KEEP — narrowest byte-pin; new driver smoke is its peer |

## 4. BytePacket — `runescript/bytepacket.go`

```go
package runescript

var crcTable = func() [256]uint32 {
    var t [256]uint32
    for b := range t {
        r := uint32(b)
        for bit := 0; bit < 8; bit++ {
            if r&1 == 1 {
                r = (r >> 1) ^ 0xedb88320
            } else {
                r >>= 1
            }
        }
        t[b] = r
    }
    return t
}()

// Crc32 mirrors TS BytePacket.crc32 — returns the signed int32 form of ~crc.
func Crc32(data []byte) int32 {
    crc := uint32(0xffffffff)
    for _, b := range data {
        crc = (crc >> 8) ^ crcTable[(crc^uint32(b))&0xff]
    }
    return int32(^crc)
}

type ByteWriter struct {
    buf    []byte
    offset int
}

func NewByteWriter(initialSize int) *ByteWriter {
    if initialSize < 64 {
        initialSize = 64
    }
    return &ByteWriter{buf: make([]byte, initialSize)}
}

func (w *ByteWriter) P1(v int)            // uint8 BE
func (w *ByteWriter) P2(v int)            // uint16 BE
func (w *ByteWriter) P4(v int32)          // int32 BE
func (w *ByteWriter) PSmart2or4(v int)    // <32768 → P2; else P4(int32(uint32(v) | 0x80000000))
func (w *ByteWriter) PData(data []byte)
func (w *ByteWriter) Bytes() []byte       // returns buf[:offset], no copy
func (w *ByteWriter) Len() int            // == offset, helper for tests
func (w *ByteWriter) ensure(extra int)    // doubles until fits
```

`PSmart2or4` matches TS `value | 0x80000000` for `value >= 32768`. Go must cast through `uint32` to avoid signed overflow when `value >= 0x80000000` — caller-side `int` values from TS are always within `[0, 2^31)` so the result fits in `int32`.

**Retires `NAI-209-D-BYTEPACKET-DEFER`.** The `BinaryScriptWriterContext.Finish()` machinery from NAI-209 keeps its hand-coded `[]byte`+offset path — `ByteWriter` is consumed by file-output sinks only.

## 5. File-output sinks

All three embed `*BinaryScriptWriter` (NAI-209) and satisfy the `BinaryOutput`
interface. Each constructor returns an `error` rather than panicking (Go-vs-TS
idiomatic; no deviation tag — error-handling convention is a global cadence
choice already established across the port).

### 5.1 `BinaryFileScriptWriter`

```go
type BinaryFileScriptWriter struct {
    *BinaryScriptWriter
    output string
}

func NewBinaryFileScriptWriter(output string, ids IdProvider, diag diagnostics.Handler) (*BinaryFileScriptWriter, error)
```

Constructor: `os.MkdirAll(output, 0755)`; `os.Stat` rejects non-directory paths
with `fmt.Errorf("%s is not a directory", absPath)`.

`OutputScript(script *codegen.RuneScript, data []byte)` writes
`filepath.Join(w.output, strconv.Itoa(id))` via `os.WriteFile(..., 0644)`.

No `Close`.

### 5.2 `JagFileScriptWriter`

```go
type JagFileScriptWriter struct {
    *BinaryScriptWriter
    output  string
    buffers map[int][]byte
}

const jagFileVersion = 27
```

`OutputScript` stores `bytes.Clone(data)` keyed by id (mimics TS
`Buffer.from(data)` retain).

`Close()`:

1. Sort keys ascending; `lastID = max(0, keys[len-1])`.
2. Open `script.dat` + `script.idx` for writing.
3. `script.dat`: `P4(lastID+1)`, `P4(jagFileVersion)`, then for `i = 0..lastID`:
   - present → `script.dat ← data`, `script.idx ← P4(len(data))`
   - absent (gap) → `script.idx ← P4(0)`
4. `script.idx`: starts with `P4(lastID+1)`, then per-id length records.

Helpers `p2(file, num)` and `p4(file, num)` write big-endian via a small
stack-buffer; mirrors TS `Buffer.allocUnsafe(N)` + `writeUInt*BE`.

### 5.3 `Js5PackScriptWriter`

```go
type Js5PackScriptWriter struct {
    *BinaryScriptWriter
    output  string
    buffers map[int][]byte
}

const (
    js5IndexFormat   = 7
    js5IndexVersion  = 1
    js5GroupVersion  = 1
)

type js5CompressionType uint8
const (
    js5CompressionNone js5CompressionType = 0
    js5CompressionBzip2 js5CompressionType = 1
    js5CompressionGzip  js5CompressionType = 2
)
```

Constructor: `os.MkdirAll(filepath.Dir(output), 0755)` + `os.Stat` directory
check on the parent.

`OutputScript` stores `bytes.Clone(data)` keyed by id.

`Close()`:

1. Build `groups` from sorted map entries; for each script:
   - `packed := packGroup(scriptData, js5CompressionNone)`
   - `checksum := Crc32(packed)`, `version := js5GroupVersion`
2. `indexData := encodeIndex(groups)`; `packedIndex := packGroup(indexData, js5CompressionGzip)`.
3. Open `output`; write `packedIndex`; write each `packedGroup`; write per-group
   `P4(len(packedGroup))` trailer.

`encodeIndex(groups)` uses `ByteWriter`:
```
P1(indexFormat)
P4(indexVersion)
P1(0)                                    // flags: no names, no digests, no lengths, no uncompressed checksums
PSmart2or4(len(groups))
for each group: PSmart2or4(groupID - previousGroupID)   // delta-encoded
for each group: P4(checksum)
for each group: P4(version)
for each group: PSmart2or4(1)            // one file per group
for each group: PSmart2or4(0)            // single file id (0), delta-encoded
```

`packGroup(src, compression)`:
- `js5CompressionNone`: `P1(0) P4(len(src)) PData(src)`
- `js5CompressionGzip`:
  - Compress via `compress/gzip` writer at default level.
  - **Zero byte at offset 9** (OS byte) of the gzip output — matches TS
    `compressed[9] = 0`.
  - `P1(2) P4(len(compressed)) P4(len(src)) PData(compressed)`
- `js5CompressionBzip2`: not supported; return error or panic
  (TS throws). Production never exercises this path.

**New deviation tag:** `NAI-210-D-GZIP-OS-BYTE-ZEROED` — TS `BytePacket.ts`
context: TS unconditionally sets `compressed[9] = 0` for byte-identical
reproducibility across hosts; Go `compress/gzip` writes the actual host OS
byte (typically `0xFF unknown` but varies). Zero post-write to match TS exactly.
Pinned by `js5_pack_writer_test.go` asserting `output[9] == 0` for a known
small-payload GZIP pack.

## 6. CompilerTypeInfo + SymbolLoader prerequisites

### 6.1 `runescript/compiler_type_info.go`

```go
type CompilerTypeInfo struct {
    Max         int
    Map         map[string]string  // key=stringified-id, value=name
    Vartype     map[string]string  // some configs only
    Protect     map[string]bool    // some configs only
    Require     map[string]string  // commands only
    Require2    map[string]string
    Conditional map[string]bool
    Set         map[string]string
    Set2        map[string]string
    Corrupt     map[string]string
    Corrupt2    map[string]string
}
```

This is a pure data carrier — no methods. Mirrors TS export-type.

### 6.2 `symbol/loader.go`

```go
// SymbolLoader is the contract for pre-compilation symbol loading.
// Mirrors TS abstract class SymbolLoader at src/compiler/configuration/SymbolLoader.ts.
type SymbolLoader interface {
    Load(table *Table, compiler CompilerContext) error
}

// CompilerContext is the narrow interface SymbolLoader implementations need.
// Avoids import cycle between symbol/ and runescript/.
type CompilerContext interface {
    FindType(name string) typ.Type
}

// AddConstant inserts a ConstantSymbol; returns the inserted symbol or
// error if Insert returned false (TS throws).
func AddConstant(table *Table, name, value string) (*ConstantSymbol, error)

// AddBasic inserts a BasicSymbol. isProtected gates Type-checker writes.
func AddBasic(table *Table, t typ.Type, name string, isProtected bool) (*BasicSymbol, error)
```

`CompilerContext` lives in the `symbol` package to keep its consumers
(the three loaders) cycle-free; `ServerScriptCompiler` implements it via
its `Types.Find`-equivalent.

### 6.3 `runescript/type_info_loader.go`

Three structs, each implementing `symbol.SymbolLoader`:

```go
type CompilerTypeInfoConstantLoader struct {
    Symbols *CompilerTypeInfo
}

func (l *CompilerTypeInfoConstantLoader) Load(table *symbol.Table, c symbol.CompilerContext) error {
    for key, value := range l.Symbols.Map {
        if _, err := symbol.AddConstant(table, key, value); err != nil { return err }
    }
    return nil
}

type CompilerTypeInfoLoader struct {
    Mapper       *SymbolMapper
    Symbols      *CompilerTypeInfo
    TypeSupplier func(subTypes typ.Type) typ.Type
}

func (l *CompilerTypeInfoLoader) Load(table *symbol.Table, c symbol.CompilerContext) error {
    for key, name := range l.Symbols.Map {
        id, err := strconv.Atoi(key)
        if err != nil { return err }

        subTypes := typ.MetaUnit
        if vartype, ok := l.Symbols.Vartype[key]; ok && vartype != "" {
            parts := strings.Split(vartype, ",")
            children := make([]typ.Type, len(parts))
            for i, tn := range parts {
                t := c.FindType(tn)
                if t == nil { t = typ.MetaError }
                children[i] = t
            }
            subTypes = typ.TupleFromList(children)
        }

        t := l.TypeSupplier(subTypes)
        sym, err := symbol.AddBasic(table, t, name, false)
        if err != nil { return err }
        l.Mapper.PutSymbol(id, sym)
    }
    return nil
}

type CompilerTypeInfoProtectedLoader struct {
    Mapper       *SymbolMapper
    Symbols      *CompilerTypeInfo
    TypeSupplier func(subTypes typ.Type) typ.Type
}
// Load: same as CompilerTypeInfoLoader but consults Symbols.Protect[key] for isProtected arg.
```

**Iteration order:** TS uses `Object.entries(map)` which preserves insertion
order. Go map iteration is **non-deterministic**. To match TS deterministic
behavior (and keep `SymbolMapper` reproducible across runs), iterate over a
sorted slice of keys — sort numerically by parsed int. **New deviation tag:**
`NAI-210-D-LOADER-SORTED-ITERATION` — Go map iteration randomized;
loader iteration sorted-by-id for byte-identical SymbolMapper across runs.

## 7. `LoadSpecialSymbols` — `runescript/load_special_symbols.go`

```go
func LoadSpecialSymbols(
    commandInfo, scriptInfo *CompilerTypeInfo,
    mapper *SymbolMapper,
    commandPointers map[string]*pointer.Holder,
    checkPointers bool,
) error
```

For each `(idStr, name)` in **sorted-by-id** iteration of `commandInfo.Map`:

1. `id, err := strconv.Atoi(idStr)` → return error if parse fails.
2. If `checkPointers && (commandInfo.Require[idStr] != "" || commandInfo.Set[idStr] != "" || commandInfo.Corrupt[idStr] != "")`:
   - `required := parsePointerList(commandInfo.Require[idStr])`
   - `required2 := parsePointerList(commandInfo.Require2[idStr])`
   - `setter := parsePointerList(commandInfo.Set[idStr])`
   - `setter2 := parsePointerList(commandInfo.Set2[idStr])`
   - `conditionalSet := commandInfo.Conditional[idStr]` (zero value `false`)
   - `corrupted := parsePointerList(commandInfo.Corrupt[idStr])`
   - `corrupted2 := parsePointerList(commandInfo.Corrupt2[idStr])`
   - `commandPointers[name] = &pointer.Holder{Required: required, Set: setter, ConditionalSet: conditionalSet, Corrupted: corrupted}`
   - If `len(required2) > 0 || len(setter2) > 0 || len(corrupted2) > 0`:
     `commandPointers["."+name] = &pointer.Holder{Required: required2, Set: setter2, ConditionalSet: conditionalSet, Corrupted: corrupted2}`
3. `mapper.PutCommand(id, name)`.

For each `(idStr, name)` in sorted-by-id iteration of `scriptInfo.Map`:
`mapper.PutScript(id, name)`.

```go
func parsePointerList(s string) (map[pointer.Type]struct{}, error) {
    if s == "" || s == "none" { return map[pointer.Type]struct{}{}, nil }
    out := map[pointer.Type]struct{}{}
    for _, name := range strings.Split(s, ",") {
        p, ok := pointer.ForName(name)
        if !ok { return nil, fmt.Errorf("invalid pointer name: %s", name) }
        out[p] = struct{}{}
    }
    return out, nil
}
```

(Exact `pointer.Holder` field names/types from NAI-208 — plan writer to verify at plan-write per [[controller_preflight]].)

**Retires `NAI-208-D-COMMAND-POINTERS-DEFERRED`.**

## 8. `ServerScriptCompiler` driver

### 8.1 Struct

```go
package runescript

type ServerScriptCompiler struct {
    SourcePaths     []string
    ExcludePaths    []string

    Types           *typ.Manager
    Triggers        *trigger.Manager
    RootTable       *symbol.Table
    DynHandlers     map[string]command.DynamicHandler
    SymbolLoaders   []symbol.SymbolLoader
    CompilerSymbols map[string]*CompilerTypeInfo
    Mapper          *SymbolMapper
    CommandPointers map[string]*pointer.Holder
    Features        semantics.StrictFeatureLevel

    DiagHandler     diagnostics.Handler

    // Assembled in Setup():
    BinaryWriter *BinaryScriptWriter   // wraps user-supplied BinaryOutput
    Writer       BinaryOutput          // user-supplied sink (BinaryFile/Jag/Js5)
}

// FindType implements symbol.CompilerContext.
func (c *ServerScriptCompiler) FindType(name string) typ.Type {
    return c.Types.Find(name)
}
```

Path normalization: sourcePaths/excludePaths normalized via `filepath.Abs` +
`filepath.Clean` in the constructor (`NewServerScriptCompiler(...)`) **or** at
the top of `Run`. Plan picks one; prefer constructor for fail-fast.

### 8.2 `Setup()`

Defined in `runescript/setup.go`. Invoked once before `Run`. Mirrors TS
`ServerScriptCompiler.setup()` lines 64–212 + parent `ScriptCompiler`
constructor lines 96–115. Order matters — TS file order is canonical.

**Outline** (see TS for exact registrations):

```go
func (c *ServerScriptCompiler) Setup() {
    // From TS ScriptCompiler constructor:
    c.Types.RegisterAll(typ.PrimitiveAll)
    c.Types.Register("any", typ.MetaAny)
    c.Types.Register("type", typ.MetaAny)
    c.setupDefaultTypeCheckers()
    c.Triggers.RegisterTrigger(trigger.CommandTrigger)

    // From TS ServerScriptCompiler.setup():
    c.Triggers.RegisterAll(trigger.ServerTriggerTypeAll)
    c.registerScriptVarTypes()

    c.Types.ChangeOptions("long", func(o *typ.Options) {
        o.AllowDeclaration = false
        o.AllowParameter = true
    })

    if !c.Features.DisableProcs {
        c.Types.Register("proc", typ.NewMetaScript(trigger.ServerTriggerPROC, typ.MetaUnit, typ.MetaUnit))
    }
    c.Types.Register("label", typ.NewMetaScript(trigger.ServerTriggerLABEL, typ.MetaUnit, typ.MetaNothing))

    // Allow assignment of namedobj → obj
    c.Types.AddTypeChecker(func(l, r typ.Type) bool {
        return l == typ.ScriptVarObj && r == typ.ScriptVarNamedObj
    })

    c.addSymConstantLoaders()

    c.Types.Register("walktrigger", typ.NewMetaScript(trigger.ServerTriggerWALKTRIGGER, typ.MetaAny, typ.MetaNothing))
    c.Types.Register("queue", typ.NewMetaScript(trigger.ServerTriggerQUEUE, typ.MetaAny, typ.MetaNothing))
    c.Types.Register("timer", typ.NewMetaScript(trigger.ServerTriggerTIMER, typ.MetaAny, typ.MetaNothing))

    // Sym-loaders for: loc / npc / obj / component / interface / overlayinterface /
    //                  fontmetrics / category / hunt / inv / idk / mesanim /
    //                  param + intparam / seq / spotanim / varp / varn / vars /
    //                  stat / locshape / movespeed / npc_mode / npc_stat /
    //                  model / synth / midi / jingle.
    c.addSymLoader("loc", typ.ScriptVarLoc)
    // ... etc (see TS lines 110-176)

    // varbit (September 2004 in TS)
    c.Types.Register("varbit", typ.NewVarBitType(typ.MetaAny))
    c.addProtectedSymLoaderWithSupplier("varbit", func(sub typ.Type) typ.Type { return typ.NewVarBitType(sub) })

    if !c.Features.DisableEnums {
        c.addSymLoader("enum", typ.ScriptVarEnum)
    }
    if !c.Features.DisableStructs {
        c.addSymLoader("struct", typ.ScriptVarStruct)
    }
    c.Types.Register("softtimer", typ.NewMetaScript(trigger.ServerTriggerSOFTTIMER, typ.MetaAny, typ.MetaNothing))

    if !c.Features.DisableDBTables {
        c.Types.Register("dbcolumn", typ.NewDbColumnType(typ.MetaAny))
        c.addSymLoaderWithSupplier("dbcolumn", func(sub typ.Type) typ.Type { return typ.NewDbColumnType(sub) })
        c.addSymLoader("dbrow", typ.ScriptVarDbRow)
        c.addSymLoader("dbtable", typ.ScriptVarDbTable)
    }

    // Inner-half: queues, timers, params, dump/script, db_*, enum command handler.
    // All gated on features inside RegisterAllDynCommands.
    command.RegisterAllDynCommands(c, c.Features)
}

// helpers:
func (c *ServerScriptCompiler) addSymLoader(name string, t typ.Type) {
    c.addSymLoaderWithSupplier(name, func(_ typ.Type) typ.Type { return t })
}

func (c *ServerScriptCompiler) addSymLoaderWithSupplier(name string, ts func(typ.Type) typ.Type) {
    if info, ok := c.CompilerSymbols[name]; ok {
        c.SymbolLoaders = append(c.SymbolLoaders, &CompilerTypeInfoLoader{
            Mapper:       c.Mapper,
            Symbols:      info,
            TypeSupplier: ts,
        })
    }
}

func (c *ServerScriptCompiler) addProtectedSymLoaderWithSupplier(name string, ts func(typ.Type) typ.Type) {
    if info, ok := c.CompilerSymbols[name]; ok {
        c.SymbolLoaders = append(c.SymbolLoaders, &CompilerTypeInfoProtectedLoader{
            Mapper:       c.Mapper,
            Symbols:      info,
            TypeSupplier: ts,
        })
    }
}

func (c *ServerScriptCompiler) addSymConstantLoaders() {
    if info, ok := c.CompilerSymbols["constant"]; ok {
        c.SymbolLoaders = append(c.SymbolLoaders, &CompilerTypeInfoConstantLoader{Symbols: info})
    }
}

func (c *ServerScriptCompiler) registerScriptVarTypes() {
    for _, t := range typ.ScriptVarTypeAll {
        if c.Features.DisableEnums && t == typ.ScriptVarEnum { continue }
        if c.Features.DisableStructs && t == typ.ScriptVarStruct { continue }
        if c.Features.DisableDBTables && (t == typ.ScriptVarDbRow || t == typ.ScriptVarDbTable) { continue }
        c.Types.Register(t)
    }
}
```

`RegisterAllDynCommands` (existing) is edited to honor `features.DisableX`
gates per TS lines 80–212 — see §10.

`addProtectedSymLoader` (non-supplier variant) is **NOT** used by TS
`ServerScriptCompiler` per current TS source. Port only the supplier variant.

**Plan-task gate:** the call list in `Setup` is long (~50 calls). Plan splits
this into a single task with explicit per-line verification against TS line
numbers; reviewer enforces line-by-line parity.

### 8.3 `setupDefaultTypeCheckers`

Defined in `runescript/default_type_checkers.go`. Registers seven checkers
mirroring TS `ScriptCompiler.setupDefaultTypeCheckers` (parent class lines
123–204):

1. `left == typ.MetaAny` → always true
2. `left == typ.MetaError || right == typ.MetaError` → true
3. `left == right` → reflexive equality
4. `MetaScript` checker — both `MetaScript`, same trigger, parametric type
   recursion via `c.Types.Check(...)`
5. `MetaHook` checker — both `MetaHook`, recursive on `TransmitListType`
6. `WrappedType` checker — both have non-nil `Inner` field; same Go type
   (use `reflect.TypeOf` equality); recursive on Inner
7. `TupleType` checker — both `TupleType`, equal child counts, all recurse
8. Representation-string fallback — `left.Representation() == right.Representation()`

The TS "bottom type" checker (allow Nothing on right) is **commented out** in
TS — port the commented-out form as a Go comment, do not register.

The WrappedType + representation-string checkers are existing in goscape (NAI-205-D-WRAPPED-TYPE-INNER-PROBE-VIA-CONSTRUCTOR-NAME or similar — verify at plan-write); reuse if already registered elsewhere, otherwise add fresh here. If reuse conflicts (double-registration), gate behind a "default checkers already registered" flag.

### 8.4 `Run(ext string) error`

```go
func (c *ServerScriptCompiler) Run(ext string) error {
    if err := c.loadSymbols(); err != nil { return err }
    files, err := c.parse(ext)
    if err != nil { return err }                 // halt-on-errors per TS

    if err := c.analyze(files); err != nil { return err }
    scripts, err := c.codegen(files)
    if err != nil { return err }

    if err := c.checkPointers(scripts); err != nil { return err }

    if err := c.write(scripts); err != nil { return err }

    if closer, ok := c.Writer.(io.Closer); ok {
        return closer.Close()
    }
    return nil
}
```

Each phase calls into existing packages (parser, semantics, codegen,
runescript.ServerPointerChecker, BinaryScriptWriter). The driver wires
diagnostics through `c.DiagHandler` matching TS `handleParse` / `handleTypeChecking`
/ `handleCodeGeneration` / `handlePointerChecking` hooks. If `c.DiagHandler == nil`,
use a no-op default.

**`registerSecondaryCommands`** runs inside `analyze` between script-registration
and type-checking, matching TS line 296. For each name in `c.CommandPointers`
starting with `.`, look up the base symbol via `c.RootTable.Find(SymbolTypeServerScript(CommandTrigger), baseName)` and insert a `ServerScriptSymbol` alias if not already present.

**Pointer-check early-return parity:** TS returns `false` when
`commandPointers.size < 1` (treats empty pointers as compile failure, which
seems wrong but is TS behavior). **New deviation tag:**
`NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE` — only if we port TS behavior verbatim.
**Alternative:** treat empty pointers as a benign skip (success). Plan picks
one; the cadence pattern is TS-faithful, so port verbatim with deviation tag.

**Path-normalization at construction** vs **at Run start:** decided
plan-time. Test fixtures pass already-absolute paths via `t.TempDir()` so
either works.

## 9. `runescript.Compile(cfg) error` — `runescript/compile.go`

```go
type Config struct {
    SourcePaths   []string
    ExcludePaths  []string
    Symbols       map[string]*CompilerTypeInfo
    CheckPointers *bool   // nil → default true
    Features      semantics.StrictFeatureLevel
    Writer        WriterConfig
}

type WriterConfig struct {
    Jag *JagWriterConfig
    Js5 *Js5WriterConfig
}

type JagWriterConfig struct{ Output string }
type Js5WriterConfig struct{ Output string }

func Compile(cfg Config) error {
    if cfg.Symbols == nil || cfg.Symbols["command"] == nil || cfg.Symbols["runescript"] == nil {
        return errors.New("core symbols missing from compiler: provide command and runescript symbols")
    }

    sourcePaths := cfg.SourcePaths
    if len(sourcePaths) == 0 {
        sourcePaths = []string{"../content/scripts"}
    }
    excludePaths := cfg.ExcludePaths
    checkPointers := true
    if cfg.CheckPointers != nil { checkPointers = *cfg.CheckPointers }

    jag := cfg.Writer.Jag
    js5 := cfg.Writer.Js5
    if jag == nil && js5 == nil {
        jag = &JagWriterConfig{Output: "./data/pack/server"}
    } else if jag != nil && js5 != nil {
        return errors.New("only one of writer.jag / writer.js5 may be set")
    }

    absSources, err := absAll(sourcePaths)
    if err != nil { return err }
    absExcludes, err := absAll(excludePaths)
    if err != nil { return err }

    mapper := NewSymbolMapper(/* diag */)
    var writer BinaryOutput
    if jag != nil {
        absOut, err := filepath.Abs(jag.Output)
        if err != nil { return err }
        writer, err = NewJagFileScriptWriter(absOut, mapper, /* diag */)
        if err != nil { return err }
    } else {
        absOut, err := filepath.Abs(js5.Output)
        if err != nil { return err }
        writer, err = NewJs5PackScriptWriter(absOut, mapper, /* diag */)
        if err != nil { return err }
    }

    commandPointers := map[string]*pointer.Holder{}
    if err := LoadSpecialSymbols(cfg.Symbols["command"], cfg.Symbols["runescript"], mapper, commandPointers, checkPointers); err != nil {
        return err
    }

    c := &ServerScriptCompiler{
        SourcePaths:     absSources,
        ExcludePaths:    absExcludes,
        Types:           typ.NewManager(),
        Triggers:        trigger.NewManager(),
        RootTable:       symbol.NewTable(),
        DynHandlers:     map[string]command.DynamicHandler{},
        CompilerSymbols: cfg.Symbols,
        Mapper:          mapper,
        CommandPointers: commandPointers,
        Features:        cfg.Features,
        Writer:          writer,
    }
    c.Setup()
    return c.Run("rs2")
}
```

`NewJagFileScriptWriter` / `NewJs5PackScriptWriter` take a `*SymbolMapper`
as `IdProvider` (matches TS `idProvider` constructor param — SymbolMapper
implements the `IdProvider` interface from NAI-209). Plan-task verifies the
interface match at plan-write.

`CheckPointers` is a `*bool` because we need to distinguish "unset" (default
true) from "explicitly false". The TS interpretation key is the use of
`typeof ... !== 'undefined'`.

## 10. Feature-gating in `command/register.go`

Current state: `func RegisterAllDynCommands(c CompilerContext, features semantics.StrictFeatureLevel)` with body marking `_ = features`. NAI-207-D-REGISTERALL-NO-FEATURES doc-comments on five sections.

Changes:
- Remove `_ = features`.
- Wrap `queue*` / `weakqueue*` / `strongqueue*` / `longqueue*` registrations in `if !features.DisableQueueTyped { ... }`.
- Wrap `enum` command handler in `if !features.DisableEnums { ... }`.
- Wrap `struct_param` in `if !features.DisableStructs { ... }`.
- Wrap `db_find` / `db_find_refine` / `db_find_with_count` / `db_find_refine_with_count` / `db_getfield` in `if !features.DisableDBTables { ... }`.
- Procs gate (`DisableProcs`) does NOT touch `RegisterAllDynCommands` — TS
  `procs` gates the `proc` type registration in `Setup`, not any command
  handler. Verify at plan-write.
- Delete the five NAI-207-D-REGISTERALL-NO-FEATURES doc-comments + the trailing `_ = features`.

Cohort tests (`cohort_a_test.go` and/or `cohort_b_test.go`) add:
- `TestRegisterAll_QueueTypedDisabled` — `features.DisableQueueTyped = true` → `queue*` etc. not in handler map
- `TestRegisterAll_EnumsDisabled` — `features.DisableEnums = true` → `enum` not in handler map
- `TestRegisterAll_StructsDisabled` — `features.DisableStructs = true` → `struct_param` not in handler map
- `TestRegisterAll_DBTablesDisabled` — `features.DisableDBTables = true` → `db_find` etc. not in handler map

Each test pairs with a positive case (gate enabled → handler present).

**Retires `NAI-207-D-REGISTERALL-NO-FEATURES`** by deleting the doc-comments and the cohort-test pin in `codegen/nai207_deviation_pins_test.go`.

## 11. PrimitiveCategory + lookup-key fix

### 11.1 `type/primitive.go` edits

Add a new singleton:
```go
// PrimitiveCategory mirrors TS ScriptVarType.CATEGORY (code 'y').
// Lands as part of NAI-210 to discriminate per-subject category lookups
// in the binary writer.
var PrimitiveCategory = newPrimitive('y', "category")
```
Extend the existing `PrimitiveAll` (or equivalent) iteration slice to include `PrimitiveCategory`. **Plan-task verifies the slice name + ordering**; new entry goes at the end to avoid disturbing existing positional assumptions.

### 11.2 `runescript/binary_writer.go` edit

```go
// OLD (NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR):
//   if tm.Category { ... }
// NEW:
//   if subject.Type == typ.PrimitiveCategory { ... }
```

(Exact field name `tm.Category` / `subject.Type` per current code — plan verifies at plan-write.)

### 11.3 `binary_writer_lookup_test.go::TestLookupKey_TypeMode_Category`

Currently asserts goscape's per-trigger category behavior (NAI-209-D-…). Switch to:
- Build a fixture with `subject.Type = typ.PrimitiveCategory`.
- Assert the resulting lookup key string matches the TS-equivalent format.

**Retires `NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR`** by deleting the pin entry in `nai209_deviation_pins_test.go`.

## 12. Testing strategy

### 12.1 Unit — sinks

- **BytePacket:** round-trip each method (P1, P2, P4, PSmart2or4 boundary at 32767/32768, PData); Crc32 golden vectors against known small payloads (empty, single byte, ASCII "abc", binary blob).
- **BinaryFileScriptWriter:** write 3 scripts → 3 files under tempdir; rejects non-directory output path with error.
- **JagFileScriptWriter:** write 3 scripts including a gap at id=1 → `script.dat` + `script.idx` byte-pinned:
  - `.idx` first 4 bytes = `P4(lastID+1)`; per-id length records
  - `.dat` first 4 bytes = `P4(lastID+1)`; next 4 bytes = `P4(27)`; then concatenated script data
- **Js5PackScriptWriter:**
  - byte-pin first 32 bytes of the `.js5` for a 2-script fixture
  - **assert `output[9] == 0`** (the OS-byte zeroing pin) on the packed index group
  - assert trailer = N × `P4(packedGroupLen)`

### 12.2 Unit — driver pieces

- **`setupDefaultTypeCheckers`:** register on a fresh `typ.Manager`; assert each of the 7 checkers fires on the expected pair, doesn't fire on the wrong pair.
- **`Setup`:** invoke on a fresh `ServerScriptCompiler` with synthetic `CompilerSymbols`; assert (a) handler map size matches expected for `Features{}`, (b) `Types.Find("queue")` returns a `MetaScript`, (c) `SymbolLoaders` slice contains the right loader subtypes.
- **`Setup` feature gates:**
  - `Features{DisableProcs:true}` → `Types.Find("proc") == nil`
  - `Features{DisableEnums:true}` → no `enum` loader added; `enum` not in `ScriptVarType` registrations
  - `Features{DisableStructs:true}` → no `struct` loader added
  - `Features{DisableDBTables:true}` → no `dbcolumn` / `dbrow` / `dbtable` loaders
- **`registerSecondaryCommands`:** fixture with `commandPointers["foo"]` + `commandPointers[".foo"]` + base symbol `"foo"` in rootTable; assert `.foo` alias inserted.
- **`LoadSpecialSymbols`:**
  - `Require="active_player,active_npc"` → 2-element pointer set in holder
  - `Require="none"` → empty set
  - `Require=""` AND `Set=""` AND `Corrupt=""` → no holder inserted
  - `Require2="active_loc"` non-empty → `.alias` holder inserted
  - unknown pointer name → error
  - SymbolMapper has command + script entries populated in sorted-by-id order
- **`CompilerTypeInfoLoader`:**
  - `vartype="int,string"` → `TupleFromList([Int, String])`
  - missing vartype → `MetaUnit`
  - unknown type name → `MetaError`
- **`CompilerTypeInfoProtectedLoader`:**
  - `protect[key] = true` → `BasicSymbol.IsProtected = true`
  - missing protect entry → `false`
- **`CompilerTypeInfoConstantLoader`:** inserts `ConstantSymbol` per map entry.

### 12.3 Smoke — driver end-to-end

`compile_test.go::TestCompile_JagWriter_EndToEnd`:
- Tiny fresh fixture (single proc, single command): `[proc,hello] return;` written to `t.TempDir()/scripts/hello.rs2`.
- `cfg.Symbols` seeded inline with `command`, `runescript`, and any minimal sym-info maps.
- **Critical:** at least one entry in `cfg.Symbols["command"]` must have a non-empty `Require`/`Set`/`Corrupt` field so `LoadSpecialSymbols` populates `commandPointers`; otherwise `ServerPointerChecker` early-returns false (see `NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE`) and `write` is skipped. Use e.g. `Require: "active_player"` on a single seeded command id.
- Invoke `Compile(cfg)` with `Jag` writer pointed at `t.TempDir()/pack`.
- Assert `script.dat` + `script.idx` exist; byte-pin first 16 bytes of each.

`compile_test.go::TestCompile_Js5Writer_EndToEnd`:
- Same fixture; `Js5` writer.
- Assert `.js5` file exists; byte-pin first 32 bytes; assert OS-byte at offset 9 is zero.

`compile_test.go::TestCompile_MissingCoreSymbols_ReturnsError`:
- Empty `Symbols` map → error containing "core symbols missing".
- `Symbols` with only "command" → error.

### 12.4 Deviation pins — NAI-210

`nai210_deviation_pins_test.go` enumerates the new tags via grep-and-assert
(same pattern as `nai209_deviation_pins_test.go`):

```go
func TestDeviationPinsLive_NAI210(t *testing.T) {
    tags := []struct{ Tag, Why string }{
        {"NAI-210-D-GZIP-OS-BYTE-ZEROED", "compress/gzip writes OS byte; zero to match TS reproducibility"},
        {"NAI-210-D-LOADER-SORTED-ITERATION", "Go map iteration randomized; sort by id for byte-identical SymbolMapper"},
        {"NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE", "TS-faithful early-return false on empty commandPointers"},
    }
    // grep each tag in pkg/ + modules/ + cmd/; require ≥1 production touch point
}
```

`nai209_deviation_pins_test.go` table edited to remove
`NAI-209-D-BYTEPACKET-DEFER` + `NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR`.

`codegen/nai207_deviation_pins_test.go` table edited to remove
`NAI-207-D-REGISTERALL-NO-FEATURES`.

(NAI-208 had a single deviation tag for command-pointers. If a
`nai208_deviation_pins_test.go` table includes `NAI-208-D-COMMAND-POINTERS-DEFERRED`,
remove that entry too. Plan-task verifies.)

## 13. Deviation tag accounting

| Tag | Disposition | Reason |
|---|---|---|
| `NAI-207-D-REGISTERALL-NO-FEATURES` | **RETIRE** | Feature gates now wired |
| `NAI-208-D-COMMAND-POINTERS-DEFERRED` | **RETIRE** | `LoadSpecialSymbols` lands |
| `NAI-209-D-BYTEPACKET-DEFER` | **RETIRE** | `ByteWriter` lands |
| `NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR` | **RETIRE** | `PrimitiveCategory` lands |
| `NAI-210-D-GZIP-OS-BYTE-ZEROED` | NEW | Go `compress/gzip` writes host OS byte; zero to match TS |
| `NAI-210-D-LOADER-SORTED-ITERATION` | NEW | Go map iteration randomized; sort by id |
| `NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE` | NEW | TS-faithful early-return on empty `commandPointers` |

Net change: −4 retirements + 3 new = **−1 live tag**. NAI-209 had 11 live (+ NAI-207-D-REGISTERALL-NO-FEATURES + NAI-208-D-COMMAND-POINTERS-DEFERRED living outside the NAI-209 pin file = 13 total port-wide live tags spanning the compiler). NAI-210 retires 4 of those, adds up to 3, leaving roughly **10–12 live compiler tags** depending on which adjacent slices have unrelated open tags at close time. Exact number determined by `nai209_deviation_pins_test.go` table + `nai210_deviation_pins_test.go` table + a grep audit on close.

## 14. Task decomposition

Target ~10 tasks. Suggested ordering (plan writer finalizes):

| # | Task | Files / scope |
|---|---|---|
| T1 | `BytePacket` | `runescript/bytepacket.go` + test |
| T2 | `BinaryFileScriptWriter` | `runescript/binary_file_writer.go` + test |
| T3 | `JagFileScriptWriter` | `runescript/jag_file_writer.go` + test |
| T4 | `Js5PackScriptWriter` | `runescript/js5_pack_writer.go` + test (includes byte-9 zero pin) |
| T5 | `PrimitiveCategory` + lookup-key fix | `type/primitive.go` + `binary_writer.go` + lookup test edit |
| T6 | Feature-gating in `command/register.go` | `register.go` + cohort tests |
| T7 | `CompilerTypeInfo` + `SymbolLoader` + 3 loaders | `compiler_type_info.go` + `symbol/loader.go` + `type_info_loader.go` + tests |
| T8 | `LoadSpecialSymbols` | `load_special_symbols.go` + test |
| T9 | `ServerScriptCompiler` struct + `Setup()` + `setupDefaultTypeCheckers` | `server_script_compiler.go` + `setup.go` + `default_type_checkers.go` + tests |
| T10 | `Run(ext)` pipeline | `server_script_compiler.go` (Run method) + test (parse→write end-to-end on inline fixture, no file IO) |
| T11 | `Compile(cfg)` facade + driver smoke + deviation-pin updates + CLOSE | `compile.go` + `compile_test.go` + pin-test edits + `nai210_deviation_pins_test.go` |

T1–T4 are independent. T5/T6 are independent of T1–T4 but require existing
NAI-205/NAI-207 code. T7 depends on T6 (uses Features). T8 depends on T7. T9
depends on T7. T10 depends on T9. T11 depends on T2–T4, T8, T10.

If LOC envelope is too large, T9 can be split:
- T9a struct + Setup outer-half + default type-checkers
- T9b registerScriptVarTypes + sym-loader plumbing + protected sym-loaders

## 15. Risks / open items

- **`SymbolType` factory names** (`SymbolTypeBasic`, `SymbolTypeConstant`) verified at HEAD; if `SymbolTypeBasic` requires a non-nil `typ.Type` and `SymbolLoader.AddBasic` passes one, no shim needed.
- **`typ.PrimitiveAll`** iteration slice: not located by initial grep — plan writer confirms exact symbol name (`PrimitiveAll` / `AllPrimitives` / inline literal in `Manager.RegisterAll`).
- **`trigger.ServerTriggerTypeAll`** registration slice: not located by initial grep — confirm at plan-write. NAI-208 introduced `MetaScriptTriggerIdent`, so `ServerTriggerType` enum exists; need its `All` slice.
- **`typ.Manager.ChangeOptions`** API for the `long` allowDeclaration/allowParameter flip: confirm exists. If not, scope into T9.
- **`typ.NewMetaScript`** ctor: confirm exact name. NAI-205/NAI-207 introduced `typ.NewMetaScript`-equivalent or the literal `&MetaScript{}` form.
- **`typ.NewVarBitType` / `typ.NewVarPlayerType` / `typ.NewVarNpcType` / `typ.NewVarSharedType` / `typ.NewParamType` / `typ.NewDbColumnType`**: confirm all exist (NAI-205 D-TRIGGER-POINTERS-DEFERRED was about trigger pointers, not var types — these likely exist). If any missing, plan scopes them into the appropriate task.
- **`pointer.Holder` field names**: NAI-208 spec says `Required` / `Set` / `ConditionalSet` / `Corrupted` — plan writer reads the struct definition before codifying `LoadSpecialSymbols`.
- **`SymbolMapper.PutSymbol`** existence: NAI-209 introduced `PutScript` + `PutCommand`; check `PutSymbol`.
- **`semantics.StrictFeatureLevel` is in `semantics/` package** — driver imports `semantics`. Confirm no cycle: `runescript` already imports `semantics` (used by `ServerPointerChecker`).

These are plan-write verifications, not blockers — the cadence ([[plan_grep_helper_patterns]], [[controller_preflight]]) handles each one.

## 16. References

- NAI-209 close memory: `[[nai209_close]]`
- NAI-208 three-slice decomposition: `[[nai208_three_slice_decomposition]]`
- NAI-209 spec: `docs/superpowers/specs/2026-05-16-nai-209-binary-script-writer-design.md`
- NAI-209 plan: `docs/superpowers/plans/2026-05-16-nai-209-binary-script-writer.md`
- TS source files at `RuneScriptTS @ b8c3388`:
  - `src/runescript/writer/BytePacket.ts`
  - `src/runescript/writer/BinaryFileScriptWriter.ts`
  - `src/runescript/writer/JagFileScriptWriter.ts`
  - `src/runescript/writer/Js5PackScriptWriter.ts`
  - `src/runescript/ServerScriptCompiler.ts`
  - `src/runescript/ServerScriptCompilerApplication.ts`
  - `src/runescript/CompilerTypeInfo.ts`
  - `src/runescript/CompilerTypeInfoLoader.ts`
  - `src/runescript/CompilerTypeInfoConstantLoader.ts`
  - `src/runescript/CompilerTypeInfoProtectedLoader.ts`
  - `src/compiler/configuration/SymbolLoader.ts`
  - `src/compiler/ScriptCompiler.ts` (parent class, lines 60–115 / 219–260 / 285–305 / 360–410 inlined into `Setup`/`Run`)
