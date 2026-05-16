# NAI-209 — Binary script writer (compiler slice 6b of 6)

**Date:** 2026-05-16
**Series:** Go rewrite of LostCityRS Engine-TS, compiler port (NAI-188 → NAI-210)
**TS pin:** LostCityRS/RuneScriptTS @ `b8c338801fbb72d294ff9576a58925a8d3f6de47`
**Tech stack:** Go 1.26+, `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`
**Commits:** `git commit --no-gpg-sign`

## 1. Context

NAI-208 closed compiler slice 6a on 2026-05-16: pointer-flow validator
(`pkg/pack/compiler/pointer/`, `cfg/`), seeded `pkg/pack/compiler/runescript/`
with `ServerPointerChecker`, and extended `TestPipeline_FullSlice` to run
the pointer checker after codegen.

NAI-209 ports the next ~993 LOC of TS (originally framed as 1167 with
`BytePacket`; see §3): the writer pipeline that converts each
`*codegen.RuneScript` into a binary script blob. Disk output and the
end-to-end `ServerScriptCompiler` driver are deferred to NAI-210.

| Slice                       | Scope                                                                                                          | TS LOC |
| --------------------------- | -------------------------------------------------------------------------------------------------------------- | ------ |
| NAI-208 (6a, done)          | pointer/cfg/runescript pkgs + PointerChecker + ServerPointerChecker + pipeline smoke                           | ~1126  |
| **NAI-209 (6b, this spec)** | `ServerScriptOpcode` + `SymbolMapper` + `BaseScriptWriter` + `BinaryScriptWriter` + `BinaryScriptWriterContext` | ~993   |
| NAI-210 (6c)                | `BytePacket` + `BinaryFileScriptWriter` + `JagFileScriptWriter` + `Js5PackScriptWriter` + `ServerScriptCompiler` driver + retire `NAI-207-D-REGISTERALL-NO-FEATURES` | ~613   |

## 2. Goals (NAI-209 only)

1. Port TS `compiler/writer/BaseScriptWriter.ts` to a new `pkg/pack/compiler/writer/`
   package (`OpcodeWriter` dispatch interface + `BaseContext` + helper functions
   `GenerateLineNumberTable`, `GenerateJumpTable`, `GetVariableId`,
   `GetLocalCount`, `GetParameterCount`, `IdProvider` interface).
2. Port TS `runescript/ServerScriptOpcode.ts` to `pkg/pack/compiler/writer/opcode.go`
   (40 opcode singletons with numeric ID + `LargeOperand` bool + `All` slice).
3. Port TS `runescript/SymbolMapper.ts` to `pkg/pack/compiler/runescript/symbol_mapper.go`
   (implements `writer.IdProvider`).
4. Port TS `runescript/writer/BinaryScriptWriterContext.ts` to
   `pkg/pack/compiler/runescript/binary_context.go` (raw `[]byte`+offset
   instruction/switch buffers + `Finish()` header layout with placeholder
   backpatch).
5. Port TS `runescript/writer/BinaryScriptWriter.ts` to
   `pkg/pack/compiler/runescript/binary_writer.go` (per-opcode `Write*`
   methods + lookup-key generation + `BinaryOutput` interface for the
   file-output hook).
6. Extend `TestPipeline_FullSlice` (NAI-208 T8) so it runs
   `BinaryScriptWriter` after `ServerPointerChecker` and byte-pins the
   output blob for the existing 2-script source.

## 3. Out of scope (deferred to NAI-210)

- `BytePacket` (`crc32` + `ByteWriter`). **Rationale:** the only NAI-209 file
  that needs raw byte manipulation is `BinaryScriptWriterContext`, which
  uses random-access placeholder backpatching (`switchBuffer.writeUInt16BE(totalKeyCount,
  sizePos)`) — not the append-only shape that `ByteWriter` exposes. Adding
  it here would land an API with zero consumers in this slice.
  Tagged `NAI-209-D-BYTEPACKET-DEFER`.
- File-output sinks (`BinaryFileScriptWriter`, `JagFileScriptWriter`,
  `Js5PackScriptWriter`).
- `ServerScriptCompiler` end-to-end driver.
- Feature-gating in `RegisterAllDynCommands` (retires
  `NAI-207-D-REGISTERALL-NO-FEATURES` in NAI-210).

## 4. Architecture

```
pkg/pack/compiler/
  writer/                            (NEW — TS: src/compiler/writer/ + src/runescript/ServerScriptOpcode.ts)
    opcode.go                        — ServerScriptOpcode struct + 40 singletons + All
    id_provider.go                   — IdProvider interface
    base_context.go                  — BaseContext{CurIndex, LineNumberTable, JumpTable}
    base_writer.go                   — OpcodeWriter interface
                                       + WriteScript(w, script) free function (dispatch)
                                       + GenerateLineNumberTable / GenerateJumpTable
                                       + GetVariableId / GetLocalCount / GetParameterCount
  runescript/                        (EXTEND from NAI-208)
    symbol_mapper.go                 — SymbolMapper{commands, scripts, symbols} → IdProvider
    binary_context.go                — BinaryScriptWriterContext (embeds BaseContext)
                                       + Instruction / InstructionRaw / InstructionString
                                       + Switch / SwitchCase / Finish + LookupKey
    binary_writer.go                 — BinaryScriptWriter (implements OpcodeWriter)
                                       + GenerateLookupKey + BinaryOutput interface
  trigger/                           (MODIFY)
    subjectmode.go                   — add IsNameMode(SubjectMode) bool helper
```

Imports:

- `writer/` imports `codegen/`, `symbol/`, `type/`.
- `runescript/` (this slice) imports `writer/`, `codegen/`, `symbol/`,
  `trigger/`, `type/`, `diagnostics/`.

## 5. Component-by-component design

### 5.1 `writer.ServerScriptOpcode`

TS exports a class with `private constructor(id, largeOperand)` and 40 static
singletons. Go shape:

```go
type ServerScriptOpcode struct {
    ID           uint16
    LargeOperand bool
}

var (
    OpPushConstantInt    = &ServerScriptOpcode{ID: 0, LargeOperand: true}
    OpPushVarp           = &ServerScriptOpcode{ID: 1, LargeOperand: true}
    // ... 40 total
    OpAnd                = &ServerScriptOpcode{ID: 4614}
    OpOr                 = &ServerScriptOpcode{ID: 4615}
)

var All = []*ServerScriptOpcode{ /* same insertion order as TS */ }
```

Pointer-typed values mirror TS's reference-equality semantics for the dispatch
checks in `BinaryScriptWriter.writeBranch`. Numeric IDs are listed verbatim from
TS (`ServerScriptOpcode.ts:13-54`).

### 5.2 `writer.IdProvider`

```go
type IdProvider interface {
    Get(symbol symbol.Symbol) int
}
```

`Get` returns `-1` for missing script/command symbols (TS does not throw for
those — it reports via diagnostics and returns `-1`). For missing basic symbols
TS throws; Go panics (parity).

### 5.3 `writer.BaseContext`

```go
type BaseContext struct {
    Script           *codegen.RuneScript
    CurIndex         int
    LineNumberTable  map[int]int           // pc → source line
    JumpTable        map[*codegen.Label]int // label → pc
}

func NewBaseContext(script *codegen.RuneScript) *BaseContext { ... }
```

`LineNumberTable` and `JumpTable` are populated in the constructor by the
static helpers below. Map ordering is non-deterministic in Go but the consumers
either iterate in insertion order (`Finish()` line-number write) or do
direct lookups — see §5.6 for the determinism handling.

### 5.4 `writer.BaseScriptWriter` helpers + dispatch

```go
type OpcodeWriter interface {
    EnterBlock(block *codegen.Block)
    WritePushConstantInt(value int32)
    WritePushConstantString(value string)
    WritePushConstantLong(value int64)
    WritePushConstantSymbol(sym symbol.Symbol)
    WritePushLocalVar(sym *symbol.LocalVariableSymbol)
    WritePopLocalVar(sym *symbol.LocalVariableSymbol)
    WritePushVar(sym *symbol.BasicSymbol, dot bool)
    WritePopVar(sym *symbol.BasicSymbol, dot bool)
    WriteDefineArray(sym *symbol.LocalVariableSymbol)
    WriteSwitch(table *codegen.SwitchTable)
    WriteBranch(opcode *codegen.Opcode, label *codegen.Label)
    WriteJoinString(count int)
    WriteDiscard(baseType typ.BaseVarType)
    WriteJump(sym *symbol.ServerScriptSymbol)
    WriteGosub(sym *symbol.ServerScriptSymbol)
    WriteCommand(sym *symbol.ServerScriptSymbol)
    WriteReturn()
    WriteMath(opcode *codegen.Opcode)
}

// Dispatch — mirrors TS BaseScriptWriter.writeInstruction.
// Caller supplies the BaseContext so CurIndex is incremented on the same
// context the OpcodeWriter impl reads from (BinaryScriptWriterContext embeds
// *BaseContext). Blocks and Instructions are exported FIELDS on goscape's
// codegen types, not methods — match the existing struct shape verbatim.
func WriteScript(w OpcodeWriter, ctx *BaseContext, script *codegen.RuneScript) {
    for _, block := range script.Blocks {
        w.EnterBlock(block)
        for _, ins := range block.Instructions {
            dispatch(w, ins)
            ctx.CurIndex++   // increment AFTER per-opcode method returns (parity)
        }
    }
}

// Static helpers — public package functions, not methods.
func GenerateLineNumberTable(script *codegen.RuneScript) map[int]int
func GenerateJumpTable(script *codegen.RuneScript) map[*codegen.Label]int
func GetParameterCount(locals *codegen.LocalTable, baseType typ.BaseVarType) int
func GetLocalCount(locals *codegen.LocalTable, baseType typ.BaseVarType) int
func GetVariableId(locals *codegen.LocalTable, local *symbol.LocalVariableSymbol) int
```

`dispatch` is an unexported function with the opcode `switch` ported verbatim
from TS `BaseScriptWriter.writeInstruction:55-148`. `advance` is exposed on
the interface (not a side effect inside the dispatch) so `BinaryScriptWriter`
sees the same `CurIndex` semantics as TS (incremented *after* the
`writeInstruction` call returns).

### 5.5 `runescript.SymbolMapper`

```go
type SymbolMapper struct {
    diags    diagnostics.Reporter // ctor-injected; see NAI-209-D-SYMMAPPER-DIAG-CTOR
    commands map[string]int
    scripts  map[string]int
    symbols  map[symbol.Symbol]int
}

func NewSymbolMapper(diags diagnostics.Reporter) *SymbolMapper { ... }

func (m *SymbolMapper) PutSymbol(id int, s symbol.Symbol)
func (m *SymbolMapper) PutCommand(id int, name string)
func (m *SymbolMapper) PutScript(id int, name string)
func (m *SymbolMapper) Get(s symbol.Symbol) int
```

`Get` branches on `*symbol.ServerScriptSymbol` via type-switch:

- If `Trigger == trigger.CommandTrigger`: strip leading `.` prefix from name
  (TS `substring(indexOf('.') + 1)`); look up in `commands`; report-and-return
  -1 on miss.
- Else: build key `[<trigger.Identifier>,<name>]`; look up in `scripts`;
  report-and-return -1 on miss.
- For any other symbol type: direct lookup in `symbols`; **panic** on miss
  (parity with TS `throw new Error`).

Duplicate `PutSymbol` reports through diagnostics and returns. Duplicate
`PutCommand`/`PutScript` silently return (TS pattern).

### 5.6 `runescript.BinaryScriptWriterContext`

Embeds `*writer.BaseContext`. Holds two grown-on-demand `[]byte` slices
(`instructionBuffer`, `switchBuffer`) with explicit `instructionOffset` /
`switchOffset` ints. `Buffer.alloc(N)` in TS → `make([]byte, N)` in Go.
`Buffer.writeUInt16BE(v, off)` → `binary.BigEndian.PutUint16(b[off:], v)`.

Initial capacity `512` matches TS.

```go
type BinaryScriptWriterContext struct {
    *writer.BaseContext
    lookupKey         int32
    instructionBuffer []byte
    switchBuffer      []byte
    instructionCount  int
    instructionOffset int
    switchOffset      int
}

func (c *BinaryScriptWriterContext) Instruction(op *writer.ServerScriptOpcode, operand int32)
func (c *BinaryScriptWriterContext) InstructionRaw(opcode, operand int)
func (c *BinaryScriptWriterContext) InstructionString(op *writer.ServerScriptOpcode, operand string)
func (c *BinaryScriptWriterContext) Switch(id int, block func() int)
func (c *BinaryScriptWriterContext) SwitchCase(key, jump int32)
func (c *BinaryScriptWriterContext) Finish() []byte
```

**Determinism note.** TS `Finish()` iterates `this.lineNumberTable` with a
`for...of` over a `Map`, which preserves insertion order. Go `map` iteration
is randomized — we keep a parallel `[]int` of pcs in insertion order
inside `BaseContext` (or rebuild from the original ascending scan in
`GenerateLineNumberTable`), and `Finish()` walks that slice.

**Placeholder backpatch.** `Switch` writes a 2-byte placeholder at
`switchOffset`, advances `switchOffset += 2`, calls the user callback (which
records key/jump pairs), then patches the placeholder with
`binary.BigEndian.PutUint16(b.switchBuffer[sizePos:], uint16(totalKeyCount))`.
Byte-pin test required at T5.

### 5.7 `runescript.BinaryScriptWriter`

```go
type BinaryOutput interface {
    OutputScript(script *codegen.RuneScript, data []byte)
}

type BinaryScriptWriter struct {
    IdProvider writer.IdProvider
    Output     BinaryOutput
    ctx        *BinaryScriptWriterContext // set per Write() call; read by per-opcode methods
}

func NewBinaryScriptWriter(idp writer.IdProvider, out BinaryOutput) *BinaryScriptWriter

// Implements writer.OpcodeWriter — one method per dispatch arm.
```

The "abstract `outputScript` hook" in TS becomes a `BinaryOutput` interface
field. NAI-210 wires concrete file sinks; tests inject a recorder.

`Write(script)` is the public entry — internally:

1. Builds `BinaryScriptWriterContext` with `lookupKey = generateLookupKey(script)`,
   stashes it on the writer struct (`b.ctx = context`) so per-opcode methods
   can reach it without receiver gymnastics.
2. Calls `writer.WriteScript(b, b.ctx.BaseContext, script)` — the writer
   implements `OpcodeWriter` directly; per-opcode methods read `b.ctx`.
3. Calls `b.ctx.Finish()`.
4. Calls `b.Output.OutputScript(script, data)`.

#### 5.7.1 `generateLookupKey`

Ports TS `BinaryScriptWriter.generateLookupKey:58-85`:

```go
func (b *BinaryScriptWriter) generateLookupKey(script *codegen.RuneScript) int32 {
    if trigger.IsNameMode(script.Trigger.SubjectMode) {
        return -1
    }
    key := int32(script.Trigger.ID)
    if tm, ok := trigger.IsTypeMode(script.Trigger.SubjectMode); ok && script.SubjectReference != nil {
        subject := script.SubjectReference
        var subjectId int32
        switch subjectInnerType(subject) {            // exact shape verified at T7 pre-flight
        case typ.PrimitiveMapzone, typ.PrimitiveCoord:
            n, err := strconv.Atoi(subject.SymbolName())
            if err != nil {
                panic(fmt.Sprintf("BinaryScriptWriter: invalid MAPZONE/COORD subject %q: %v",
                    subject.SymbolName(), err))
            }
            subjectId = int32(n)
        default:
            subjectId = int32(b.IdProvider.Get(subject))
        }
        // typeMarker = 1 when subject is `category`; 2 otherwise.
        // The exact "category" sentinel in goscape (a primitive? a flag on
        // TypeMode?) is unresolved — T7 pre-flight must grep
        // `pkg/pack/compiler/type/*.go` and `pkg/pack/compiler/trigger/subjectmode.go`
        // and codify the actual predicate. Risk #2 in §10.
        var typeMarker int32 = 2
        if isCategorySubject(tm) {
            typeMarker = 1
        }
        key += (typeMarker << 8) | (subjectId << 10)
    }
    return key
}
```

(`trigger.IsNameMode` is the new one-line helper this slice adds to
`pkg/pack/compiler/trigger/subjectmode.go`, mirroring the existing
`IsTypeMode`.)

#### 5.7.2 Per-opcode methods

Direct ports of TS `BinaryScriptWriter:91-362`. Highlights:

- `WritePushConstantSymbol` — checks `LocalVariableSymbol` → `GetVariableId`;
  else `MetaType.Type` instance → take char code of `Inner.Code()[0]`;
  else `IdProvider.Get`. The "MetaType.Type instance" check requires picking
  the right Go type at T6 pre-flight (likely a `typ.MetaTypeOfTypes` or
  similar — verify before plan-codifying).
- `WritePushVar` / `WritePopVar` — type-switch on `VarPlayerType`, `VarBitType`,
  `VarNpcType`, `VarSharedType` (already exist in `pkg/pack/compiler/type/gamevar.go`).
  Dot-bit operand encoding: `operand += 1 << 16`.
- `WriteDefineArray` — `(id << 16) | code`.
- `WriteSwitch` — calls `context.Switch(table.ID, func() int {...})`; reads
  `JumpTable` per case; emits `SwitchCase(key, jump - curIndex - 1)`.
- `WriteBranch` — same `jumpLocation - context.CurIndex - 1` arithmetic.
- `WriteCommand` — `context.InstructionRaw(op, secondary ? 1 : 0)` where
  `secondary = strings.HasPrefix(sym.Name, ".")`. Panics on `op == -1`.
- `WritePushConstantLong` — `panic("BinaryScriptWriter: PushConstantLong not supported")`.

## 6. Test strategy

### 6.1 Unit tests (per task)

| Task | Test file                                          | Pins                                                                |
| ---- | -------------------------------------------------- | ------------------------------------------------------------------- |
| T1   | `writer/opcode_test.go`                            | All 40 opcodes have unique IDs; `LargeOperand` flags match TS verbatim. |
| T2   | `runescript/symbol_mapper_test.go`                 | put/get for script + command + basic; duplicate dispatch; missing →  -1 vs panic. |
| T3   | `writer/base_writer_test.go`                       | Hand-built 2-block script: `GenerateJumpTable` returns expected pcs; `GenerateLineNumberTable` preserves insertion order; `GetVariableId` handles array vs scalar vs param. |
| T4   | `writer/base_writer_dispatch_test.go`              | Recorder mock implements `OpcodeWriter`; assert per-opcode dispatch + `advance()` ordering + `EnterBlock` per block. |
| T5   | `runescript/binary_context_test.go`                | Byte-pin: `Instruction(OpPushConstantInt, 42)` → exact 6 bytes; `Switch` placeholder backpatch; `Finish()` header (fullName, sourceName, lookupKey, debugproc-zero, line-number-table-count, instruction-bytes, instructionCount, local-count, switch-count, switch-bytes, switch-end-offset). |
| T6   | `runescript/binary_writer_test.go`                 | One test per `Write*` method via the writer-direct API (`SymbolMapper` fake). Verifies operand encoding, opcode mapping, dot-bit, MetaType-Type char-code path. |
| T7   | `runescript/binary_writer_lookup_test.go`          | Three arms: name-mode → -1; category-mode → `id + (1<<8) + (subjectId<<10)`; other type-mode → `id + (2<<8) + (subjectId<<10)`; MAPZONE/COORD parseInt path; invalid parse panics. |
| T8   | `pack/compiler/codegen/smoke_test.go` (extend)     | Pipeline runs through writer; byte-pin first 32 bytes of output blob for one script; full-length check for both. |

### 6.2 Existing-test impact

`TestPipeline_FullSlice` extended (not replaced). NAI-208 `nai208_deviation_pins_test.go` untouched. NAI-207 deviation pins untouched.

## 7. Deviation tag inventory (NAI-209)

| Tag                                       | Reason                                                                                                       |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `NAI-209-D-BYTEPACKET-DEFER`              | `BytePacket` (`crc32` + `ByteWriter`) deferred to NAI-210 — no consumer in this slice.                       |
| `NAI-209-D-SYMMAPPER-DIAG-CTOR`           | Diagnostics injected via constructor (TS reads `(symbol as any).context?.diagnostics`); goscape symbols carry no context field. |
| `NAI-209-D-PUSHLONG-PANIC`                | `WritePushConstantLong` panics (TS `throw new Error('Not supported.')`).                                     |
| `NAI-209-D-MAPZONE-COORD-PARSE-PANIC`     | `strconv.Atoi` failure panics (TS `parseInt` would NaN-corrupt the key silently).                            |
| `NAI-209-D-OPCODE-WRITER-INTERFACE`       | TS abstract-class virtual dispatch → Go `OpcodeWriter` interface + free-function `WriteScript`.              |
| `NAI-209-D-BINARYOUTPUT-INTERFACE`        | TS abstract `outputScript` → Go `BinaryOutput` interface field on `BinaryScriptWriter`.                      |
| `NAI-209-D-LINENUMBER-ORDER-SLICE`        | Insertion-order line-number iteration preserved via parallel `[]int` of pcs (Go map iteration is randomized).|

Each tag is pinned by a no-op test in `runescript/nai209_deviation_pins_test.go`
(string-match production-source per `[[pin_test_self_trigger_production_doc]]`
— use concept names not TS identifiers in pin-test docstrings).

## 8. Retired tags (other-NAI deferrals NAI-209 closes)

None — NAI-209 does not close prior deferrals. NAI-208's open
`NAI-208-D-COMMAND-POINTERS-DEFERRED` is unrelated (closed in NAI-210
when `commandPointers` registry populates).

## 9. Task cohort (preview — plan doc is canonical)

| Task | Subject                                                                                | Est. LOC |
| ---- | -------------------------------------------------------------------------------------- | -------- |
| T1   | `writer/` package skeleton — `ServerScriptOpcode` + `IdProvider` + `BaseContext`       | ~120     |
| T2   | `runescript.SymbolMapper`                                                              | ~110     |
| T3   | `writer` static helpers + `BaseContext` ctor                                           | ~80      |
| T4   | `writer.WriteScript` dispatch + `OpcodeWriter` interface                               | ~180     |
| T5   | `runescript.BinaryScriptWriterContext`                                                 | ~220     |
| T6   | `runescript.BinaryScriptWriter` per-opcode methods                                     | ~280     |
| T7   | `generateLookupKey` + `trigger.IsNameMode` helper                                      | ~80      |
| T8   | Pipeline smoke extension — byte-pin writer output                                      | ~150     |
| T9   | Doc + retire `[[adjacent_doc_paragraph_count_drift]]` + close commit                   | ~30      |

## 10. Open risks

1. **`MetaType.Type` instance check** at `WritePushConstantSymbol`. goscape's
   `pkg/pack/compiler/type/meta.go` has `metaWrapping` with `Inner() Type` —
   need to find the concrete "type-of-types" variant (likely a `MetaTypeOfTypes`
   singleton). Verify exact shape at T6 pre-flight per
   `[[plan_constants_under_different_naming]]`.
2. **`isCategorySubject(TypeMode)` predicate.** goscape has no `PrimitiveCategory`
   primitive and no `Category` flag on `trigger.TypeMode` (verified at spec-write).
   T7 pre-flight must either (a) locate the existing recognizer, (b) port a
   primitive `CATEGORY` from TS `ScriptVarType.ts`, or (c) add a `Category bool`
   flag to `trigger.TypeMode`. Plan-time decision; tag whichever choice is taken.
3. **Switch-case key parallel-slice**. goscape's `codegen.SwitchCase.Keys`
   may be `[]int + []symbol.Symbol` (parallel-slice convention per
   `[[parallel_slice_convention_for_mixed_type_args]]`) or a single
   union-like type. Verify at T6 pre-flight in `pkg/pack/compiler/codegen/switch_table.go`.
4. **`generateLookupKey` arithmetic overflow**. `(typeMarker << 8) | (subjectId << 10)`
   needs 32-bit headroom; very large subject IDs (> 22 bits) overflow.
   T7 byte-pin includes an arithmetic-overflow check.
5. **`CurIndex` increment ordering**. TS increments *after* `writeInstruction`
   returns; `WriteBranch` reads `jumpLocation - curIndex - 1`. The Go
   `WriteScript` dispatch must increment `CurIndex` only after the
   per-opcode method returns. Pinned in T4 recorder-mock test.
6. **`Finish()` debugproc parameter codes**. TS reads `script.symbol.parameters`
   via `TupleType.toList`. goscape's `*ServerScriptSymbol.Parameters` shape
   needs T5 verification (per `[[typechecker_parameters_nil_deref_panic]]`,
   a nil-Parameters fixture must be handled — likely a synthetic
   `MetaUnit` tuple).
7. **Map-iteration determinism**. `lineNumberTable` and `jumpTable` Go maps
   are iterated randomly; only `Finish()` iterates `lineNumberTable`. We
   handle this with a parallel `[]int` of pcs in insertion order — verify
   no other consumer iterates the maps.
8. **String null terminator**. TS `writeString` writes raw bytes then a
   trailing `0x00`. Go equivalent uses `append(buf, name...)` + `append(buf, 0)`
   or `Packet.PJStrNUL` — `BinaryScriptWriterContext` is `[]byte`-based so
   the inline form is used. Byte-pin asserts the terminator at T5.

## 11. Cadence

- Subagent-driven TDD per `[[runescript_cadence]]` and `[[execution_mode_default]]`.
- Each task: spec → red test → green impl → spec-compliance review → code-quality review.
- Controller pre-flight per `[[controller_preflight]]` before every implementer dispatch.
- Post-dispatch verification per `[[verify_implementer_claims]]`.
- Close-commit memory trailer per `[[close_commit_memory_trailer]]`.
