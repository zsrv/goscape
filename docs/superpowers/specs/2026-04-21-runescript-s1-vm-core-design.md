# Sub-spec RuneScript S1: VM Core — Design

**Status:** Draft → ready for plan
**Scope:** New `pkg/script/` package. Loads `data/pack/server/script.{dat,idx}` (8032 compiled scripts, compiler version 26). Two-stack bytecode VM with 19 working opcodes. Lookup by name or trigger. Zero integration with `modules/world/` — pure library; all demonstrable via unit tests.
**Out of scope:** Suspension / queues (S4), tick-loop integration (S3), real Player wiring (S2), VARP/VARN/VARBIT (S5+), NPC/Loc/Obj active entities (S6), the other ~330 opcodes.

---

## Goal

Ship a standalone, testable RuneScript virtual machine. After S1:
- A caller can instantiate a `Provider`, call `Load(cacheDir)`, and find scripts by name or trigger.
- A caller can pass a `ScriptFile` to `Runner.Init(...)` and `Runner.Execute(state)` and have it run to completion.
- 19 opcodes (covering stack / control flow / locals / math / strings / MES / NAME / CONSOLE / GOSUB / RETURN) let a hand-built or cache-loaded script produce observable output via an `ActivePlayer` interface.
- A `Disassemble` utility prints any `ScriptFile` as a human-readable listing, using mnemonic names for all 350 opcodes (even the ones we don't implement — so unknown-opcode errors produce useful diagnostics).

## Architecture

Single new package `pkg/script/`. Two stacks (int + string) with independent SPs, local arrays, frame stack for GOSUB, active-entity pointers. Cooperative single-threaded — dispatch loop runs until `state.Execution != Running`.

```
pkg/script/
├── opcode.go       Opcode type + constants for all 350 opcodes (sparse enum; only 19 handled)
├── pointer.go      Pointer flag constants
├── trigger.go      ServerTriggerType enum (168 values)
├── execution.go    Execution enum (Running, Suspended, CountDialog, PauseButton, NpcSuspended, WorldSuspended, Finished)
├── file.go         ScriptFile + Decode
├── provider.go     Provider + Load + GetByTrigger + GetByName
├── state.go        ScriptState + stack ops + frames
├── runner.go       Init + Execute dispatch loop
├── handlers.go     19 MVP opcode implementations + handlers map
├── active.go       ActivePlayer interface (+ ActiveNpc/Loc/Obj stubs for later)
└── disasm.go       Disassemble(*ScriptFile) → string
```

Tests:
```
pkg/script/
├── file_test.go
├── provider_test.go
├── state_test.go
├── runner_test.go
├── handlers_test.go
└── disasm_test.go
```

## Components

### 1. `opcode.go`

```go
package script

type Opcode uint16

const (
    OpPushConstantInt    Opcode = 0
    OpPushVarp           Opcode = 1
    OpPopVarp            Opcode = 2
    OpPushConstantString Opcode = 3
    OpPushVarn           Opcode = 4
    OpPopVarn            Opcode = 5
    OpBranch             Opcode = 6
    OpBranchNot          Opcode = 7
    OpBranchEquals       Opcode = 8
    OpBranchLess         Opcode = 9
    OpBranchGreater      Opcode = 10
    OpReturn             Opcode = 21
    OpPushVarbit         Opcode = 25
    OpPopVarbit          Opcode = 27
    OpBranchLessEquals   Opcode = 31
    OpBranchGreaterEquals Opcode = 32
    OpPushIntLocal       Opcode = 33
    OpPopIntLocal        Opcode = 34
    OpPushStringLocal    Opcode = 35
    OpPopStringLocal     Opcode = 36
    OpJoinString         Opcode = 37
    OpPopIntDiscard      Opcode = 38
    OpPopStringDiscard   Opcode = 39
    OpGosubWithParams    Opcode = 40
    OpJumpWithParams     Opcode = 41
    OpSwitch             Opcode = 60

    // PlayerOps (2000 range)
    OpMes  Opcode = 2416
    OpName Opcode = 2436

    // MathOps (4600 range)
    OpAdd Opcode = 4600
    OpSub Opcode = 4601

    // StringOps (4500 range)
    OpToString Opcode = 4505

    // DebugOps (10000 range)
    OpConsole Opcode = 10000

    // ... full table of ~350 opcodes, canonical source is TS ScriptOpcode.ts
)

// String returns the uppercase mnemonic (PUSH_CONSTANT_INT, MES, …). Falls
// back to "opcode_<n>" for unknown values so disassembly of a cache script
// with new opcodes still produces readable output.
func (o Opcode) String() string
```

**Note:** exact numeric values must match the TS `ScriptOpcode.ts` table. The plan's implementation task copies the values directly — non-negotiable since the cache file bakes them in.

**Large-operand list:** opcodes that use u8 operands (rest are u32). Per TS:
```
RETURN (21), POP_INT_DISCARD (38), POP_STRING_DISCARD (39), GOSUB (unused), JUMP (unused)
```
Implementation hint: an `isLargeOperand(op Opcode) bool` helper — same condition the decoder uses.

### 2. `execution.go`

```go
package script

type Execution int

const (
    Running        Execution = iota // hot loop continues
    Finished                        // OpReturn with empty frame stack
    Aborted                         // Runtime error (e.g., unknown opcode)
    Suspended                       // Player-level pause; resumed by tick loop (S4)
    CountDialog                     // Waiting for client input (S5)
    PauseButton                     // Waiting for button click (S5)
    NpcSuspended                    // NPC variant of Suspended (S6)
    WorldSuspended                  // World-scheduled wakeup (S4)
)
```

Only `Running`, `Finished`, and `Aborted` are exercised in S1; the rest are defined so later sub-specs don't churn the enum.

### 3. `trigger.go`

168-value enum for `ServerTriggerType`. Canonical source: TS `ServerTriggerType.ts`. Starts with `Proc = 0`, `Label = 1`, goes through to `ZoneExit = 167`. Only `Proc`, `Label`, `Login`, `OpNpc1..5` are referenced by S1; the rest exist so `Provider.GetByTrigger` and future sub-specs don't need to touch this file.

### 4. `pointer.go`

```go
package script

type Pointer uint32

const (
    PtrActivePlayer  Pointer = 1 << 0
    PtrActivePlayer2 Pointer = 1 << 1
    PtrActiveNpc     Pointer = 1 << 2
    PtrActiveNpc2    Pointer = 1 << 3
    PtrActiveLoc     Pointer = 1 << 4
    PtrActiveLoc2    Pointer = 1 << 5
    PtrActiveObj     Pointer = 1 << 6
    PtrActiveObj2    Pointer = 1 << 7
)
```

Matches TS `ScriptPointer`. Used by handlers to assert that the active-entity pointer they need is set (e.g., MES requires `PtrActivePlayer`). S1 wires the Player bit only.

### 5. `file.go` — per-script decoder

```go
package script

type ScriptFile struct {
    Name           string
    SourceFile     string
    LookupKey      uint32 // 0xFFFFFFFF if no trigger hook
    ParamTypes     []byte

    Opcodes        []Opcode
    IntOperands    []int32
    StringOperands []string
    PCs            []uint32 // line-table PC (instruction index)
    Lines          []uint32 // source line at that PC

    InstructionCount uint32
    IntLocalCount    uint16
    StringLocalCount uint16
    IntArgCount      uint16
    StringArgCount   uint16

    SwitchTables []map[int32]int32
}

// Decode parses the raw bytes of one script blob.
func Decode(data []byte) (*ScriptFile, error)
```

**Critical decoder details** (from the TS reference — rs-server-225 has two bugs fixed here):
- `u32 lookupKey` (rs-server-225: `u16`).
- `COMPILER_VERSION = 26` (rs-server-225: hardcoded 19).
- Trailer position: `fileLen - trailerLen - 12 - 2`.
- Trailer starts with `u32 instructionCount`, then 4 × `u16` for local/arg counts, then `u8 switchTableCount`, then per-table `u16 caseCount` followed by `(u32 key, u32 jumpOffset)` entries.
- Opcode stream parsing: for each instruction, read `u16 opcode`, then if `opcode == OpPushConstantString` read a null-terminated Jagex string for the string operand, else if `isLargeOperand(opcode)` read `u8` into int operand, else read `u32`.
- The trailer position and total filesize together gate the opcode-stream loop.
- Last 2 bytes of the file are `u16 trailerByteLength`.

### 6. `provider.go` — cache loader + lookup

```go
package script

const CompilerVersion = 26

type Provider struct {
    scripts []*ScriptFile
    byKey   map[uint32]*ScriptFile
    byName  map[string]*ScriptFile
}

func NewProvider() *Provider

// Load reads script.dat and script.idx from cacheDir/server/ (or the path
// passed directly). Fails fast if the .dat header version != CompilerVersion.
func (p *Provider) Load(cacheDir string) error

// GetByTrigger tries three lookup keys in order: specific (typeId),
// category (categoryId), global. Returns nil if none match.
func (p *Provider) GetByTrigger(trigger ServerTriggerType, typeID, categoryID int) *ScriptFile

// GetByName returns the script with the given identifier, or nil.
func (p *Provider) GetByName(name string) *ScriptFile

// Count returns the number of loaded scripts.
func (p *Provider) Count() int
```

**Cache file format:**
- `script.dat`: header `u32 entryCount`, `u32 version`. Version must equal 26 or Load fails. Body: concatenated script blobs.
- `script.idx`: array of `u32 size` per entry — tells loader where to slice `.dat`.

**Trigger-key encoding** (same as TS):
```
specific = uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)
category = uint32(trigger) | (0x1 << 8) | (uint32(categoryID) << 10)
global   = uint32(trigger)
```

### 7. `active.go` — entity interface

```go
package script

// ActivePlayer is the minimal surface RuneScript needs from a Player.
// Sub-spec S2 wires modules/world.Player to this interface.
type ActivePlayer interface {
    MessageGame(msg string)
    Username() string
}

// Stubs for later sub-specs; defined now to avoid interface churn.
type ActiveNpc interface{}
type ActiveLoc interface{}
type ActiveObj interface{}
```

### 8. `state.go` — ScriptState

```go
package script

type Frame struct {
    Script       *ScriptFile
    PC           int
    IntLocals    []int
    StringLocals []string
}

type ScriptState struct {
    Script  *ScriptFile
    PC      int
    OpCount int

    Execution Execution

    IntStack    []int
    StringStack []string
    ISP         int
    SSP         int

    IntLocals    []int
    StringLocals []string

    Frames  []Frame
    FrameSP int

    Pointers Pointer
    Self     ActivePlayer
    Target   ActivePlayer

    Protect bool
}

const (
    StackCapacity   = 1024
    OpCountLimit    = 500_000
    FrameCapacity   = 50
)

func (s *ScriptState) PushInt(v int)
func (s *ScriptState) PopInt() int
func (s *ScriptState) PushString(v string)
func (s *ScriptState) PopString() string

func (s *ScriptState) EnsureIntLocals(n int)  // grow if needed
func (s *ScriptState) EnsureStringLocals(n int)

// GosubCall saves the current frame and jumps into a new script. Called
// by handleGosubWithParams / handleJumpWithParams.
func (s *ScriptState) GosubCall(target *ScriptFile, intArgs []int, stringArgs []string)

// Return pops one frame; if the frame stack is empty, sets Execution = Finished.
func (s *ScriptState) Return() error
```

**Stack semantics:**
- Underflow on PopInt/PopString returns 0 / "" respectively (matches TS's `toInt32(null) === 0` and `''` defaults). This is a deliberate no-error choice — scripts that underflow are programming errors but should not crash the VM.
- Overflow panics: `StackCapacity = 1024` matches TS; exceeding is a compiler bug.

### 9. `runner.go` — dispatch loop

```go
package script

import (
    "errors"
    "fmt"
)

// Init creates a fresh ScriptState for the given script, with int/string
// arguments copied into locals in declaration order, self wired for active-
// player handlers, and Protect respected.
func Init(script *ScriptFile, self ActivePlayer, protect bool, intArgs []int, stringArgs []string) *ScriptState

// Execute runs the state until Execution != Running. Returns any runtime
// error (unknown opcode, opcount cap, handler error). On a clean OpReturn
// with an empty frame stack, Execution == Finished and err == nil.
func Execute(s *ScriptState) error
```

Dispatch loop:
```go
func Execute(s *ScriptState) error {
    for s.Execution == Running {
        if s.OpCount >= OpCountLimit {
            s.Execution = Aborted
            return fmt.Errorf("script %q: opcount limit exceeded at pc=%d", s.Script.Name, s.PC)
        }
        s.OpCount++
        if s.PC < 0 || s.PC >= len(s.Script.Opcodes) {
            s.Execution = Aborted
            return fmt.Errorf("script %q: pc out of range (%d)", s.Script.Name, s.PC)
        }
        op := s.Script.Opcodes[s.PC]
        h, ok := handlers[op]
        if !ok {
            s.Execution = Aborted
            return fmt.Errorf("script %q: no handler for opcode %d (%s) at pc=%d",
                s.Script.Name, op, op.String(), s.PC)
        }
        if err := h(s); err != nil {
            s.Execution = Aborted
            return err
        }
        s.PC++
    }
    return nil
}
```

Branch handlers set `s.PC = target - 1` so the post-handler `++` lands on the correct instruction — matches the TS `pc--` trick in handler bodies.

### 10. `handlers.go` — 19 MVP opcodes

```go
package script

var handlers = map[Opcode]func(*ScriptState) error{
    OpPushConstantInt:    handlePushConstantInt,
    OpPushConstantString: handlePushConstantString,
    OpReturn:             handleReturn,
    OpPushIntLocal:       handlePushIntLocal,
    OpPopIntLocal:        handlePopIntLocal,
    OpPushStringLocal:    handlePushStringLocal,
    OpPopStringLocal:     handlePopStringLocal,
    OpBranch:             handleBranch,
    OpBranchEquals:       handleBranchEquals,
    OpBranchNot:          handleBranchNot,
    OpPopIntDiscard:      handlePopIntDiscard,
    OpPopStringDiscard:   handlePopStringDiscard,
    OpJoinString:         handleJoinString,
    OpAdd:                handleAdd,
    OpSub:                handleSub,
    OpToString:           handleToString,
    OpGosubWithParams:    handleGosubWithParams,
    OpMes:                handleMes,
    OpName:               handleName,
    OpConsole:            handleConsole,
}
```

Handler sketches:

```go
func handlePushConstantInt(s *ScriptState) error {
    s.PushInt(int(s.Script.IntOperands[s.PC]))
    return nil
}

func handlePushConstantString(s *ScriptState) error {
    s.PushString(s.Script.StringOperands[s.PC])
    return nil
}

func handleReturn(s *ScriptState) error { return s.Return() }

func handleBranch(s *ScriptState) error {
    off := int(s.Script.IntOperands[s.PC])
    s.PC += off // +1 happens in the main loop, so this is the pre-jumped position
    return nil
}

func handleBranchEquals(s *ScriptState) error {
    b := s.PopInt()
    a := s.PopInt()
    if a == b {
        off := int(s.Script.IntOperands[s.PC])
        s.PC += off
    }
    return nil
}

func handleJoinString(s *ScriptState) error {
    n := int(s.Script.IntOperands[s.PC])
    if n <= 0 {
        return nil
    }
    parts := make([]string, n)
    for i := n - 1; i >= 0; i-- {
        parts[i] = s.PopString()
    }
    s.PushString(strings.Join(parts, ""))
    return nil
}

func handleMes(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("MES: no active player")
    }
    s.Self.MessageGame(s.PopString())
    return nil
}

func handleName(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("NAME: no active player")
    }
    s.PushString(s.Self.Username())
    return nil
}

func handleConsole(s *ScriptState) error {
    // Pops and discards N string args + N int args. TS uses it for
    // server-log debugging from scripts; we just drop the args in S1 and
    // optionally log them when a logger is wired up in S2/S3.
    _ = s.PopString()
    return nil
}

func handleGosubWithParams(s *ScriptState) error {
    targetID := uint32(s.Script.IntOperands[s.PC])
    target := lookupByID(s, targetID)
    if target == nil {
        return fmt.Errorf("GOSUB target id %d not found", targetID)
    }
    intArgs := make([]int, target.IntArgCount)
    for i := int(target.IntArgCount) - 1; i >= 0; i-- {
        intArgs[i] = s.PopInt()
    }
    stringArgs := make([]string, target.StringArgCount)
    for i := int(target.StringArgCount) - 1; i >= 0; i-- {
        stringArgs[i] = s.PopString()
    }
    s.GosubCall(target, intArgs, stringArgs)
    return nil
}
```

**GOSUB target lookup:** the `IntOperand` for `OpGosubWithParams` is the `LookupKey` of the target script. `lookupByID(s, id)` walks `s.Provider` if threaded through — but Provider isn't directly on ScriptState. Two options:
1. Add `Provider *Provider` to ScriptState (small coupling, clean).
2. Pass Provider to `Runner.Init` and thread it through.

**Recommendation:** option 1 — `ScriptState.Provider` is populated by `Init`. Handlers that need cross-script lookup (GOSUB + future INVOKE variants) read from it. S1 locks this in; S5+ reuses.

Revised `state.go`:
```go
type ScriptState struct {
    Script   *ScriptFile
    Provider *Provider // for GOSUB target lookup
    ...
}
```

### 11. `disasm.go`

```go
package script

// Disassemble formats a script as a human-readable listing:
//
//     name: [proc,test]
//     source: test.rs2
//     lookup_key: 0xABCD1234
//     int_locals: 2  string_locals: 0  int_args: 1  string_args: 0
//
//     00:  PUSH_CONSTANT_INT    42
//     01:  PUSH_CONSTANT_STRING "hello"
//     02:  MES
//     03:  RETURN
func Disassemble(f *ScriptFile) string
```

## Data Flow

```
caller
  │
  │ provider := script.NewProvider()
  │ provider.Load("data/pack/server")
  │    │ open script.idx → read entry sizes
  │    │ open script.dat → header checked, then each entry sliced + Decode
  │    │ byKey[f.LookupKey]=f; byName[f.Name]=f
  │
  │ f := provider.GetByName("sample_proc")
  │ state := script.Init(f, mockPlayer, protect=true, intArgs, stringArgs)
  │    │ copies args into locals, sets Self+Pointers, allocates stacks
  │
  │ err := script.Execute(state)
  │    │ for Running: OpCount++, fetch op + operand, call handlers[op]
  │    │ branches set PC directly; loop's PC++ lands correctly
  │    │ Return pops frame; empty frames → Execution=Finished
  │
  │ state.Execution is Finished | Aborted; err is nil on Finished.
```

## Error Handling

- **Load errors**: version mismatch, short read, missing file → `Load` returns error; Provider empty.
- **Decode errors**: trailer out of range, string operand missing null terminator, switch-table count mismatch → `Decode` returns error; Provider skips that script and logs a warning.
- **Runtime errors**: unknown opcode, pc out of range, opcount limit, handler-returned error → `Execute` sets `Execution = Aborted` and returns the error.
- **Stack underflow**: returns zero-value (matches TS); no error.
- **Stack overflow**: panic (programming error).
- **MES/NAME without ActivePlayer**: handler returns error.
- **GOSUB to unknown id**: handler returns error.

## Testing

### `file_test.go`
- `TestDecodeSyntheticScript` — build a byte slice by hand for a minimal script ([`PUSH_CONSTANT_STRING "hi"`, `MES`, `RETURN`] + trailer). Call Decode. Assert all fields.
- `TestDecodeLargeOperandOpcode` — verify that non-small opcodes get u32 operand.
- `TestDecodeSmallOperandOpcode` — verify RETURN etc. get u8 operand.
- `TestDecodeSwitchTable` — one table with two cases.
- `TestDecodeRealCacheScript` — load one script blob from the real `data/pack/server/script.dat`, assert it decodes without error (skip if cache file missing; `t.Skip(...)`).

### `provider_test.go`
- `TestProviderLoadRealCache` — load real cache, assert `Count() > 0`, assert at least one `GetByName` hit for a known script name (probe `byName` for any entry that starts with `[login`). Skip if cache files absent.
- `TestProviderRejectsVersionMismatch` — write a fake `.dat` with version 19, assert Load returns error.
- `TestProviderGetByTriggerFallback` — seed 3 synthetic scripts with specific/category/global lookup keys, verify correct fallback order.

### `state_test.go`
- `TestPushPopIntRoundTrip`, `TestPushPopStringRoundTrip`.
- `TestPopEmptyIntStackReturnsZero`, `TestPopEmptyStringStackReturnsEmpty`.
- `TestGosubCallRestoresLocals` — Gosub then Return, verify locals identical.

### `runner_test.go`
- `TestExecuteMesReturn` — hand-built script, mock player captures MessageGame, assert captured == "hi".
- `TestExecuteUnknownOpcodeAborts` — one instruction with a non-handled opcode, Execute returns error, Execution = Aborted.
- `TestExecuteOpcountLimit` — infinite loop via unconditional BRANCH, Execute hits cap and returns error after 500k opcount.
- `TestExecuteGosubWithParams` — main script GOSUBs a sub-script that does `push 42; return`. After return, main pops 42.

### `handlers_test.go`
One test per opcode: ADD (2+3=5), SUB (5-3=2), BRANCH_EQUALS taken/untaken, BRANCH_NOT taken/untaken, JOIN_STRING ["a","b","c" → "abc"], TOSTRING (42 → "42"), locals push/pop round-trip, MES emits to player mock, NAME pushes username.

### `disasm_test.go`
- `TestDisassembleHandRolledScript` — 3-instruction script, assert exact formatted output including header.
- `TestDisassembleUnknownOpcode` — script with opcode 1234, assert output contains `opcode_1234`.

## Acceptance Criteria

1. `go test ./...` passes (tests don't require the real cache; cache-dependent tests `t.Skip` if files missing).
2. `go vet ./...` clean.
3. `go test -race ./...` passes.
4. All 19 MVP opcodes have a handler and a unit test.
5. Disassembly output is readable for any opcode (named mnemonic or `opcode_<n>` fallback).
6. `pkg/script/` has exactly the 11 production files + 6 test files listed above.
7. Zero changes outside `pkg/script/`.

## LOC Estimate

| File | LOC |
|---|---|
| `opcode.go` | ~200 |
| `pointer.go` | ~30 |
| `trigger.go` | ~180 |
| `execution.go` | ~20 |
| `file.go` | ~150 |
| `provider.go` | ~90 |
| `state.go` | ~140 |
| `runner.go` | ~70 |
| `handlers.go` | ~200 |
| `active.go` | ~30 |
| `disasm.go` | ~70 |
| **production** | **~1180** |
| `file_test.go` | ~130 |
| `provider_test.go` | ~80 |
| `state_test.go` | ~60 |
| `runner_test.go` | ~100 |
| `handlers_test.go` | ~150 |
| `disasm_test.go` | ~50 |
| **tests** | **~570** |
| **Total** | **~1750** |

Up from my earlier 1485 estimate as I flesh out the enum tables and tests more carefully. Still a reasonable scope for one sub-spec — the enums are boring but large, and the runner / handlers are well-bounded.

## Dependencies & Risks

- **Cache version pinned at 26.** Breaks if upstream compiler re-versions. Documented; fail-fast on mismatch.
- **TS opcode numeric values are the ground truth.** Plan task that writes `opcode.go` copies from `ScriptOpcode.ts` verbatim — no interpretation. Review-critical.
- **Branch-offset semantics** (`PC = target - 1` before loop's `++`). Easy to off-by-one.
- **Small-operand opcode list** must match TS's `isLargeOperand`. Decoder bug would scramble the instruction stream for any script using those opcodes.
- **Null-int sentinel = 0** (deviates from rs-server-225's `-1`; matches TS). Documented.
- **Provider-on-ScriptState** for GOSUB lookup. Creates a provider ↔ state ↔ handlers triangle. Keeps dispatch fast.
- **Unknown opcode in cache**: some of the 8032 scripts may use opcodes not in S1's handler set. Execute returns an informative error. Until S5 expands the handler set, only scripts that stay within the 19 MVP opcodes run to completion. That's fine — we only need one hello-world to demo S1.

## Deferred

- **S2**: wire `modules/world.Player` to `script.ActivePlayer`; add `OpMessageGame` wire opcode.
- **S3**: tick-loop script dispatch + LOGIN trigger; in-game visible scripts.
- **S4**: Suspension (P_DELAY), queues, world delay.
- **S5**: broad opcode coverage (varp/varn/varbit, inventory, entity config, math, strings).
- **S6**: NPC active entities + AI triggers + hunt/iterators + combat.
