# Sub-spec RuneScript S1: VM Core — Implementation Plan

> 5 tasks. Each produces a self-contained commit and leaves the tree green. Implementation order: enums → decoder → provider → VM shell → handlers.

**Goal:** `pkg/script/` package with two-stack VM, 19 opcodes, cache loader.
**Spec:** `docs/superpowers/specs/2026-04-21-runescript-s1-vm-core-design.md`.
**Build prefix:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`.
**Commit flag:** `--no-gpg-sign`.
**Canonical source of opcode numeric values and trigger IDs:** `Engine-TS/src/engine/script/ScriptOpcode.ts` and `Engine-TS/src/engine/script/ServerTriggerType.ts`. Copy verbatim.

---

## Task 1: Enums + interfaces + disassembler

Data-and-display layer. All low-risk constants and formatting; no VM logic yet.

**Files:**
- Create: `pkg/script/opcode.go`
- Create: `pkg/script/pointer.go`
- Create: `pkg/script/trigger.go`
- Create: `pkg/script/execution.go`
- Create: `pkg/script/active.go`
- Create: `pkg/script/disasm.go`
- Create: `pkg/script/disasm_test.go`

- [ ] **Step 1.1: `pkg/script/opcode.go`**

Define `type Opcode uint16` with named constants for all ~350 opcodes. Numeric values MUST match TS `ScriptOpcode.ts` exactly. Include an `isLargeOperand(Opcode) bool` helper that returns true for every opcode except the small-operand set (RETURN=21, POP_INT_DISCARD=38, POP_STRING_DISCARD=39, plus anything TS's list includes — verify against TS source before writing). Include an `Opcode.String() string` method (switch or map-based) that returns the uppercase mnemonic for every defined opcode, falling back to `fmt.Sprintf("opcode_%d", o)` for undefined values.

> **Reference doc**: spec §1 has the list of S1-critical opcodes to ensure are present: `OpPushConstantInt`, `OpPushConstantString`, `OpReturn`, `OpPushIntLocal`/`OpPopIntLocal`/`OpPushStringLocal`/`OpPopStringLocal`, `OpJoinString`, `OpPopIntDiscard`/`OpPopStringDiscard`, `OpGosubWithParams` (40), `OpBranch` (6), `OpBranchNot` (7), `OpBranchEquals` (8), `OpMes` (2416), `OpName` (2436), `OpAdd` (4600), `OpSub` (4601), `OpToString` (4505), `OpConsole` (10000). Defining ALL 350 opcodes is required so the disassembler can name unknown-to-handler opcodes. Use the TS file as authoritative.

- [ ] **Step 1.2: `pkg/script/pointer.go`**

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

- [ ] **Step 1.3: `pkg/script/trigger.go`**

```go
package script

type ServerTriggerType int

const (
    TriggerProc       ServerTriggerType = 0
    TriggerLabel      ServerTriggerType = 1
    TriggerDebugProc  ServerTriggerType = 2
    // ... full 168-value enum per TS ServerTriggerType.ts ...
    TriggerLogin      ServerTriggerType = 71
    TriggerOpNpc1     ServerTriggerType = ?
    // etc.
)
```

> **Reference doc**: copy values from TS `ServerTriggerType.ts`. S1 only exercises `TriggerProc`, `TriggerLogin`, `TriggerOpNpc1..5` — but defining all 168 prevents churn in S3/S5/S6.

- [ ] **Step 1.4: `pkg/script/execution.go`**

```go
package script

type Execution int

const (
    Running Execution = iota
    Finished
    Aborted
    Suspended
    CountDialog
    PauseButton
    NpcSuspended
    WorldSuspended
)
```

- [ ] **Step 1.5: `pkg/script/active.go`**

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

- [ ] **Step 1.6: `pkg/script/disasm.go`**

Define `Disassemble(f *ScriptFile) string` per spec §11. Since `ScriptFile` is defined in Task 2, this step declares the function signature with a stub body; Task 2 will either fill in the body OR we just defer implementation until after `file.go` lands. Simpler: have Task 1 declare a placeholder ScriptFile type in a file in this package so disasm compiles — no — cleaner: make disasm.go's `Disassemble` take `Opcodes []Opcode, IntOperands []int32, StringOperands []string, Name string, ...` raw args. Or defer disasm entirely to Task 2.

**Decision:** defer `disasm.go` to Task 2 where `ScriptFile` already exists. Task 1 only writes enums + interfaces.

Remove `disasm.go` / `disasm_test.go` from Task 1's file list. Plan is revised below in Task 2.

- [ ] **Step 1.7: Build + commit**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/
```
Expected: success. Package has no tests yet.

```bash
git add pkg/script/opcode.go pkg/script/pointer.go pkg/script/trigger.go pkg/script/execution.go pkg/script/active.go
git commit --no-gpg-sign -m "feat(script): opcode / trigger / pointer / execution / active

Foundational enums and interfaces for pkg/script:
- Opcode: ~350-value enum, numeric values from TS ScriptOpcode.ts
- ServerTriggerType: 168-value enum from TS ServerTriggerType.ts
- Pointer: 8-bit flag set for active entity slots
- Execution: 8-value runner state enum
- ActivePlayer: minimal interface Player implements in sub-spec S2

No handlers wired yet - Task 2 adds the script file decoder, and the
disassembler follows in Task 2 now that ScriptFile exists."
```

---

## Task 2: File decoder + disassembler + tests

**Files:**
- Create: `pkg/script/file.go`
- Create: `pkg/script/disasm.go`
- Create: `pkg/script/file_test.go`
- Create: `pkg/script/disasm_test.go`

- [ ] **Step 2.1: `pkg/script/file.go`**

Implement `ScriptFile` struct and `Decode(data []byte) (*ScriptFile, error)` per spec §5. Critical details:
- `u32 lookupKey` (NOT u16 — rs-server-225 had this bug).
- Per-instruction loop reads `u16 opcode` then:
  - if `opcode == OpPushConstantString`: null-terminated string (`GJStrNUL` in `pkg/io/packet`)
  - else if `isLargeOperand(opcode)`: `u8` into int operand
  - else: `u32` into int operand
- Trailer located at `fileLen - trailerLen - 12 - 2`. Read trailer: `u32 instructionCount`, 4 × `u16` (intLocalCount, stringLocalCount, intArgCount, stringArgCount), `u8 switchTableCount`, then per table: `u16 caseCount` followed by `(u32 key, u32 jumpOffset)` entries.
- Last 2 bytes of file = `u16 trailerByteLength`.

Use `pkg/io/packet.NewPacket(data)` to read values.

- [ ] **Step 2.2: `pkg/script/disasm.go`**

Implement `Disassemble(f *ScriptFile) string` per spec §11:

```go
func Disassemble(f *ScriptFile) string {
    var b strings.Builder
    fmt.Fprintf(&b, "name: %s\n", f.Name)
    fmt.Fprintf(&b, "source: %s\n", f.SourceFile)
    fmt.Fprintf(&b, "lookup_key: %#x\n", f.LookupKey)
    fmt.Fprintf(&b, "int_locals: %d  string_locals: %d  int_args: %d  string_args: %d\n\n",
        f.IntLocalCount, f.StringLocalCount, f.IntArgCount, f.StringArgCount)
    for i, op := range f.Opcodes {
        switch {
        case op == OpPushConstantString:
            fmt.Fprintf(&b, "%3d:  %-25s %q\n", i, op.String(), f.StringOperands[i])
        default:
            fmt.Fprintf(&b, "%3d:  %-25s %d\n", i, op.String(), f.IntOperands[i])
        }
    }
    return b.String()
}
```

- [ ] **Step 2.3: `pkg/script/file_test.go`**

Tests:
- `TestDecodeMinimalScript` — hand-build bytes for `[PUSH_CONSTANT_STRING "hi", MES, RETURN]` + trailer. Decode. Assert all fields including `len(Opcodes)==3`, `StringOperands[0]=="hi"`, `IntLocalCount==0`, etc.
- `TestDecodeLargeOperandOpcode` — opcode with `isLargeOperand=true` gets 4-byte operand.
- `TestDecodeSmallOperandOpcode` — RETURN gets 1-byte operand.
- `TestDecodeOneSwitchTable` — trailer with 1 table, 2 cases; verify `SwitchTables[0]` map content.
- `TestDecodeRealCacheBlob` — skip if `data/pack/server/script.dat` absent. Slice out one entry using `script.idx`; Decode; assert no error. Use the smallest entry (first `idx[0]` bytes).

- [ ] **Step 2.4: `pkg/script/disasm_test.go`**

Tests:
- `TestDisassembleHandRolledScript` — build a 3-instruction `ScriptFile` in memory; call Disassemble; assert output contains `PUSH_CONSTANT_STRING "hi"`, `MES`, `RETURN`, and the header block.
- `TestDisassembleUnknownOpcode` — script with `Opcodes = []Opcode{9999}`; assert output contains `opcode_9999`.

- [ ] **Step 2.5: Build + test + commit**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestDecode -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestDisassemble -v
```

Commit:
```bash
git add pkg/script/file.go pkg/script/disasm.go pkg/script/file_test.go pkg/script/disasm_test.go
git commit --no-gpg-sign -m "feat(script): ScriptFile decoder + disassembler

Decode parses the per-script blob format (u32 lookupKey, parameter types,
line table, instruction stream with u8/u32 operand variants, trailer with
counts + switch tables). Fixes the two bugs in rs-server-225's prior port
(u16 lookupKey, hardcoded version 19).

Disassemble formats a ScriptFile with header + numbered instructions using
the Opcode.String() mnemonic, falling back to opcode_N for unknown values."
```

---

## Task 3: Provider + cache loader + tests

**Files:**
- Create: `pkg/script/provider.go`
- Create: `pkg/script/provider_test.go`

- [ ] **Step 3.1: `pkg/script/provider.go`**

Per spec §6. Key points:
- `CompilerVersion = 26`, fail-fast mismatch.
- `Load(cacheDir)` reads `<cacheDir>/server/script.{dat,idx}` (confirm path by checking where the existing cache lives — spec says `data/pack/server`, probably `cacheDir` should be `data/pack/server`). Actually the spec says `Load(cacheDir)` where cacheDir = `data/pack/server`. So file paths are `cacheDir + "/script.dat"` etc.
- For each entry: slice the dat at `cumulativeOffset + 8 /* header */`, then `idx[i]` bytes; call `Decode`; push into slice + maps.
- `GetByTrigger` tries three keys in fallback order (specific → category → global).
- `GetByName` is a plain map lookup.

- [ ] **Step 3.2: `pkg/script/provider_test.go`**

Tests:
- `TestProviderRejectsVersionMismatch` — write a fake `.dat` with version 19 + zero entries; Load returns error.
- `TestProviderLoadRealCache` — skip if cache absent; Load real cache; `Count() > 0`; `GetByName` returns non-nil for at least one probe. Helper: iterate byName keys, pick the first, re-look-up it.
- `TestProviderGetByTriggerFallback` — manually populate `byKey` with three scripts: specific (trigger=5, typeID=10, cat=3), category (trigger=5, cat=3), global (trigger=5). Call `GetByTrigger(5, 10, 3)` → specific. Remove specific, re-call → category. Remove category, re-call → global. Remove global, re-call → nil.
- `TestProviderByNameUnique` — if two loaded scripts share a name (probably never happens with valid cache), document which one wins. Tiebreak: last-decoded wins. Test asserts this.

- [ ] **Step 3.3: Build + test + commit**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestProvider -v
```

```bash
git add pkg/script/provider.go pkg/script/provider_test.go
git commit --no-gpg-sign -m "feat(script): Provider cache loader + trigger/name lookup

Provider.Load reads script.dat + script.idx, validates compiler version 26,
slices each entry and delegates to Decode. Populates byKey (lookupKey ->
script) and byName (identifier -> script) maps.

GetByTrigger performs the TS-standard three-step fallback: specific
(typeID-keyed) -> category (categoryID-keyed) -> global. GetByName is a
direct lookup."
```

---

## Task 4: ScriptState + Runner + tests

**Files:**
- Create: `pkg/script/state.go`
- Create: `pkg/script/runner.go`
- Create: `pkg/script/state_test.go`
- Create: `pkg/script/runner_test.go`

- [ ] **Step 4.1: `pkg/script/state.go`**

Per spec §8. Key surfaces:

```go
const (
    StackCapacity = 1024
    OpCountLimit  = 500_000
    FrameCapacity = 50
)

type Frame struct {
    Script       *ScriptFile
    PC           int
    IntLocals    []int
    StringLocals []string
}

type ScriptState struct {
    Script   *ScriptFile
    Provider *Provider

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

func (s *ScriptState) PushInt(v int)
func (s *ScriptState) PopInt() int      // underflow returns 0
func (s *ScriptState) PushString(v string)
func (s *ScriptState) PopString() string // underflow returns ""

func (s *ScriptState) GosubCall(target *ScriptFile, intArgs []int, stringArgs []string)
func (s *ScriptState) Return() error    // pops frame; empty → Execution = Finished
```

- [ ] **Step 4.2: `pkg/script/runner.go`**

Per spec §9:

```go
func Init(script *ScriptFile, self ActivePlayer, protect bool, intArgs []int, stringArgs []string) *ScriptState

func Execute(s *ScriptState) error
```

`Init` allocates stacks at `StackCapacity`, copies args into locals, sets `Self` + `PtrActivePlayer` if `self != nil`, sets `Protect`.

`Execute` dispatch loop per spec §9.

- [ ] **Step 4.3: `pkg/script/state_test.go`**

Tests:
- `TestPushPopIntRoundTrip` / `TestPushPopStringRoundTrip`.
- `TestPopEmptyIntStackReturnsZero` / `TestPopEmptyStringStackReturnsEmpty`.
- `TestIntStackOverflowPanics` — push `StackCapacity+1` values, expect panic.
- `TestGosubCallRestoresFrame` — set up locals [1,2,3]; GosubCall; in new frame mutate locals; Return; assert original locals restored.
- `TestReturnEmptyFramesFinishes` — single-frame state, call Return, assert `Execution == Finished`.

- [ ] **Step 4.4: `pkg/script/runner_test.go`**

Tests:
- `TestExecuteEmptyScriptFinishesOnReturn` — single RETURN instruction, Execute returns nil, Execution = Finished. (Depends on Task 5's `handleReturn`. Tentatively use a pre-declared dummy handler inline in the test, OR accept that this test only becomes runnable after Task 5. **Recommend**: tag these with `t.Skip("needs handlers")` if Task 5 not done; or move them to Task 5.)
- `TestExecuteUnknownOpcodeAborts` — opcode not in handlers map → Execute returns error, Execution = Aborted.
- `TestExecutePcOutOfRangeAborts` — PC >= len(Opcodes) → error.
- `TestExecuteOpcountLimitHit` — requires BRANCH handler; move to Task 5.

**Decision:** move runner-behaviour tests that depend on specific handlers to Task 5. Task 4's `runner_test.go` covers ONLY: unknown-opcode abort, pc-out-of-range abort, init correctness.

- [ ] **Step 4.5: Build + test + commit**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestPush|TestPop|TestGosub|TestReturn|TestExecute' -v
```

```bash
git add pkg/script/state.go pkg/script/runner.go pkg/script/state_test.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "feat(script): ScriptState + Runner dispatch loop

Two-stack state (int + string, cap 1024 each) with independent SPs plus
int/string local arrays, frame stack for GOSUB/RETURN, active-entity
pointer bitmask and self/target pointers, protect flag, opcount counter.

Runner.Init allocates stacks, copies int/string args into locals, wires
Self + PtrActivePlayer, respects Protect.

Runner.Execute is a hot dispatch loop: fetch opcode, call handlers[op],
bump PC. Aborts on unknown opcode, PC out of range, or 500k opcount cap.
Branches set PC directly; loop's PC++ lands correctly because handlers
pre-subtract."
```

---

## Task 5: Handlers + tests + final verification

**Files:**
- Create: `pkg/script/handlers.go`
- Create: `pkg/script/handlers_test.go`

- [ ] **Step 5.1: `pkg/script/handlers.go`**

Per spec §10. Implement all 19 MVP opcode handlers plus the `handlers` map. Handler-specific details:

- `OpBranch`: `s.PC += int(s.Script.IntOperands[s.PC])`. The runner's `PC++` still fires, so the effective jump target is `PC + operand + 1` — but that's the TS convention and matches the compiled cache.

- `OpBranchEquals`: pops two ints (b, a in that order), if `a == b` apply branch offset.

- `OpJoinString`: operand is the count N; pops N strings, concatenates in original push order (i.e., pop into `parts[N-1..0]`), pushes result.

- `OpGosubWithParams`: operand is the target LookupKey (a `uint32`). Requires `s.Provider` to be non-nil. Look up `s.Provider.byKey[uint32(operand)]`; pop arg counts per target's `IntArgCount` + `StringArgCount`; call `s.GosubCall(target, intArgs, stringArgs)`. The loop's `PC++` at the top of the outer loop is disabled for GOSUB — actually NO, GosubCall stashes the PC+1 as the return address, so the loop's PC++ applies AFTER the handler but we've already left this script's PC context. Subtle. **Detail to resolve in implementation**: the GOSUB handler should set `s.PC` to `-1` in the new frame before the loop's `PC++` fires, or the `GosubCall` should set `s.PC = -1` so the first instruction of the callee is at index 0 after the bump. Verify against TS `handleGosubWithParams` behavior and add a comment.

- `OpConsole`: debug opcode. Ignore args (just pop a string). When a logger exists on ScriptState in S3, log the message.

Handler map declaration + 19 entries.

- [ ] **Step 5.2: `pkg/script/handlers_test.go`**

One test per opcode (19 total), plus a few integration tests that Task 4 deferred:
- `TestHandlePushConstantInt` — operand=42, push int, top-of-stack is 42.
- `TestHandlePushConstantString` — string operand "hi" pushed.
- `TestHandleReturn` — single RETURN, Execution = Finished.
- `TestHandlePushPopIntLocal` — Push constant 99, POP_INT_LOCAL 0, PUSH_INT_LOCAL 0, verify top-of-stack is 99.
- `TestHandlePushPopStringLocal` — symmetric.
- `TestHandleBranchUnconditional` — BRANCH +2 skips over a no-op-ish sequence.
- `TestHandleBranchEqualsTaken` / `TestHandleBranchEqualsNotTaken`.
- `TestHandleBranchNotTaken` / `TestHandleBranchNotNotTaken`.
- `TestHandleAdd` — `2+3 = 5`.
- `TestHandleSub` — `5-3 = 2`.
- `TestHandleJoinString` — `["a","b","c"]` → `"abc"`.
- `TestHandleToString` — `42` → `"42"`.
- `TestHandlePopIntDiscard` / `TestHandlePopStringDiscard`.
- `TestHandleGosubWithParams` — main script pushes 5, GOSUBs to sub-script `[PUSH_INT_LOCAL 0; RETURN]` with intArgCount=1; sub pushes its local 5, returns; main sees 5 on top of stack.
- `TestHandleMes` — mock player captures MessageGame; Execute a 3-op script; assert capture.
- `TestHandleMesWithoutPlayerErrors` — no active player → Execute returns error.
- `TestHandleName` — mock player returns "Alice"; Execute pushes "Alice".
- `TestHandleConsole` — Push string, call CONSOLE, verify string is popped.
- Plus: `TestExecuteOpcountLimitHit` — script with infinite BRANCH loop, after opcount limit returns error.

Use a small test helper:
```go
func runScript(t *testing.T, ops []Opcode, intOps []int32, strOps []string, self ActivePlayer) *ScriptState {
    t.Helper()
    f := &ScriptFile{Opcodes: ops, IntOperands: intOps, StringOperands: strOps, InstructionCount: uint32(len(ops))}
    s := Init(f, self, true, nil, nil)
    if err := Execute(s); err != nil {
        t.Fatal(err)
    }
    return s
}
```

And a mock:
```go
type mockPlayer struct {
    messages []string
    username string
}
func (m *mockPlayer) MessageGame(msg string) { m.messages = append(m.messages, msg) }
func (m *mockPlayer) Username() string       { return m.username }
```

- [ ] **Step 5.3: Build + test + vet + race**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```
Expected: all green.

- [ ] **Step 5.4: Commit**

```bash
git add pkg/script/handlers.go pkg/script/handlers_test.go
git commit --no-gpg-sign -m "feat(script): 19 MVP opcode handlers + Execute integration tests

Handlers for the S1 opcode set: stack push/pop, locals, control flow
(BRANCH / BRANCH_EQUALS / BRANCH_NOT), arithmetic (ADD/SUB), string ops
(JOIN_STRING / TOSTRING), proc call (GOSUB_WITH_PARAMS / RETURN), player
ops (MES / NAME), CONSOLE, discards.

GOSUB looks up the target by LookupKey via ScriptState.Provider. MES and
NAME require PtrActivePlayer + non-nil Self. Branch handlers pre-subtract
so the loop's PC++ lands on the target.

Closes sub-spec RuneScript S1 - the VM core is self-demoable: a hand-built
or cache-loaded script with these 19 opcodes runs to completion and emits
observable output via the ActivePlayer interface."
```

---

## Final Verification

- [ ] `go test -race ./...` — PASS
- [ ] `go vet ./...` — clean
- [ ] `ls pkg/script/ | wc -l` — 18 (11 production + 6 test + 1 for disasm which merges with file layer = let me count: opcode, pointer, trigger, execution, active, file, disasm, provider, state, runner, handlers = 11 production; file_test, disasm_test, provider_test, state_test, runner_test, handlers_test = 6 test. Total 17.)
- [ ] If the real cache is present (`data/pack/server/script.dat`), `TestProviderLoadRealCache` passes with a non-zero count.

## Spec Coverage

| Spec item | Task |
|---|---|
| `opcode.go` + all ~350 opcode constants | 1 |
| `pointer.go` | 1 |
| `trigger.go` + all 168 trigger constants | 1 |
| `execution.go` | 1 |
| `active.go` (ActivePlayer interface + stubs) | 1 |
| `file.go` + Decode + 4 unit tests | 2 |
| `disasm.go` + 2 tests | 2 |
| `provider.go` + Load + GetByTrigger + GetByName + 4 tests | 3 |
| `state.go` + 5 tests | 4 |
| `runner.go` + 3 tests (unknown-opcode, pc-range, Init) | 4 |
| `handlers.go` + 19 opcodes + ~19 tests | 5 |
| Acceptance: test + vet + race | Final |
