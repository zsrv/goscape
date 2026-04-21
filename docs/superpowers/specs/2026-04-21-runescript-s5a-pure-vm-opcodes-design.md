# Sub-spec RuneScript S5a: Pure VM Opcode Expansion — Design

**Status:** Draft → ready for plan
**Scope:** 49 pure-VM opcode handlers (28 NumberOps including 4 comparison branches, 11 active + 5 stub StringOps, 3 DebugOps, 3 array ops + SWITCH). Zero new server state. One new `ActivePlayer.Playtime()` method for `TIMESPENT` / `GETTIMESPENT`. All testable as pkg/script unit tests.
**Out of scope:** VARP/VARN/VARBIT/VARS (S5b), `JUMP` / `JUMP_WITH_PARAMS` (own sub-spec), trig ops (`SIN_DEG` / `COS_DEG` / `ATAN` / `INTERPOLATE`), active entity ops (S6), dialog pagination (`SPLIT_*` are stubbed to return 0/"").

---

## Goal

After S5a:

- Cache scripts that do integer arithmetic, bit manipulation, randomization, comparison branching, string building/probing, or local-array manipulation run to completion without the `unknown opcode` error.
- `SWITCH` works end-to-end (S1 already decoded the tables; S5a adds the handler).
- `ERROR` aborts a script cleanly with its scripted message logged.
- `TIMESPENT` and `GETTIMESPENT` return the active player's playtime.
- `SPLIT_*` stubs silently log at Debug so we can see which scripts would otherwise have hit them once we build real dialog pagination.

Concrete demo: a hand-built script that exercises every handler passes its 40 assertion-driven unit tests, and running a real cache script that chains compare/branch/math (e.g. any stat-gated OPHELD check) no longer aborts in the VM.

## Architecture

The existing `pkg/script/handlers.go` is 279 LOC with 23 handlers. Adding 40 more would balloon it past 800 LOC and make review painful, so we split by TS handler-file category:

```
pkg/script/
├── handlers.go              unchanged (existing 23 handlers; handlers map lives here)
├── handlers_number.go (new) 24 arithmetic/bitwise/random + 4 comparison branches (28 total)
├── handlers_string.go (new) 11 string handlers + 5 SPLIT_* stubs (16 total)
├── handlers_debug.go (new)  3 debug handlers
├── handlers_array.go (new)  3 array handlers + SWITCH (4 total)
├── state.go                 + Arrays [][]int32 field
└── active.go                + ActivePlayer.Playtime() int

modules/world/
└── player_script.go         + (p *Player) Playtime() int
```

All handlers register into the existing `handlers` map in `handlers.go`. The split files just contribute definitions; they do NOT define parallel maps.

Tests mirror the split:

```
handlers_number_test.go, handlers_string_test.go,
handlers_debug_test.go, handlers_array_test.go
```

## Components

### 1. `handlers_number.go` — arithmetic + bitwise + random + comparison (28 handlers)

Pop order for all binary ops is `rhs = pop, lhs = pop, push(lhs op rhs)` — matches TS where the right operand is on top.

**Comparison branches** (need `ScriptState.Script.IntOperands[PC]` as the branch offset, same as S1's `BRANCH_EQUALS`):

| Opcode | Constant | Action |
|---|---|---|
| `OpBranchLessThan` (9) | `handleBranchLessThan` | pop rhs, lhs; if lhs < rhs, PC += operand |
| `OpBranchGreaterThan` (10) | `handleBranchGreaterThan` | pop rhs, lhs; if lhs > rhs, PC += operand |
| `OpBranchLessThanOrEquals` (31) | `handleBranchLessThanOrEquals` | pop rhs, lhs; if lhs <= rhs, PC += operand |
| `OpBranchGreaterThanOrEquals` (32) | `handleBranchGreaterThanOrEquals` | pop rhs, lhs; if lhs >= rhs, PC += operand |

**Arithmetic** (10):

- `OpMultiply` (4602): `push(lhs * rhs)` with int32 wrap semantics matching TS's `Math.imul`. Use Go `int32` math.
- `OpDivide` (4603): floor division; abort on div by zero. `floor(a/b)` when signs differ; Go `/` truncates — compute manually or use `math.Floor`.
- `OpModulo` (4611): match TS `(a % b + b) % b` to produce positive remainders.
- `OpAbs` (4628): `push(max(-x, x))` — handles `INT_MIN` as int overflow (match TS).
- `OpAddPercent` (4607): `push(lhs + (lhs * rhs) / 100)` with TS rounding.
- `OpScale` (4618): `push((a * b) / c)` with div-by-zero abort.
- `OpMin` (4616), `OpMax` (4617): trivial.
- `OpPow` (4612): `push(lhs ^ rhs)` integer power; TS clamps to 0 for negative exponents.
- `OpInvPow` (4613): `push(floor(log_base_rhs(lhs)))` — sparse usage; mirror TS algorithm exactly.

**Bitwise** (11):

- `OpAnd` (4614), `OpOr` (4615): trivial.
- `OpBitCount` (4619): `push(bits.OnesCount32(uint32(x)))`.
- `OpTestBit` (4610): `push((value >> bit) & 1)`.
- `OpSetBit` (4608), `OpClearBit` (4609), `OpToggleBit` (4620): single-bit mutations.
- `OpGetBitRange` (4623): `push((value >> start) & ((1 << (end-start+1)) - 1))`.
- `OpSetBitRange` (4621): set all bits `[start..end]` to 1.
- `OpClearBitRange` (4622): clear all bits `[start..end]` to 0.
- `OpSetBitRangeToInt` (4624): replace bits `[start..end]` with the low bits of a value.

**Random** (2):

- `OpRandom` (4604): `push(rand.IntN(n))` — pops n, returns `[0, n-1]`. TS has `Math.random() * n | 0`.
- `OpRandomInc` (4605): `push(rand.IntN(n+1))` — `[0, n]` inclusive.

### 2. `handlers_string.go` — string ops + SPLIT stubs (16 handlers)

String push/pop uses the string stack (existing). All "append" ops pop a suffix + a base string, push `base + suffix`.

- `OpAppend` (4501): `push(popS() + popS())` (note pop order — second pop is the prefix).
- `OpAppendNum` (4500): `push(popS() + strconv.Itoa(popI()))`.
- `OpAppendChar` (4508): `push(popS() + string(rune(popI())))`.
- `OpAppendSignNum` (4502): `push(popS() + signedNumStr(popI()))` where negatives keep `-` and positives get no sign (match TS).
- `OpLowercase` (4503): `push(strings.ToLower(popS()))`.
- `OpCompare` (4506): `push(int(strings.Compare(popS(), popS())))` — be careful about pop order (rhs on top, lhs below).
- `OpStringLength` (4509): `push(len(popS()))` — byte length; ASCII-safe for rev 225 cache.
- `OpSubstring` (4510): `push(popS()[start:end])` with bounds clamped; TS errors on OOB but cache scripts are well-formed.
- `OpStringIndexOfChar` (4511): `push(strings.IndexRune(popS(), rune(popI())))`.
- `OpStringIndexOfString` (4512): `push(strings.Index(popS(), popS()))` — again watch pop order.
- `OpTextSwitch` (4507): like `SWITCH` but keyed by string. We decode string-switch tables separately — but S1 only set up int `SwitchTables`. *Design call: check whether string switches have their own table type in file.go. If not, stub with a warning for S5a and defer to S5b/S6.*

**SPLIT stubs** (5): log `slog.Debug("SPLIT_* called; not implemented", ...)` and push a sentinel:

- `OpSplitInit` (4515): pop all args, no push.
- `OpSplitGet` (4513), `OpSplitGetAnim` (4514): pop args, push 0.
- `OpSplitLineCount` (4516), `OpSplitPageCount` (4517): pop args, push 0.

### 3. `handlers_debug.go` — debug ops (3 handlers)

- `OpError` (10001): pop message, return `errors.New("ERROR: " + msg)` so the Execute dispatch sets `Execution = Aborted` and surfaces the message.
- `OpGetTimeSpent` (10002): `push(Self.Playtime())`. Requires active-player check.
- `OpTimeSpent` (10003): same — TS has both names for historical reasons. Point the handler at the same impl.

### 4. `handlers_array.go` — array + switch (4 handlers)

`ScriptState` grows:

```go
// Arrays holds script-local int[] arrays defined via DEFINE_ARRAY.
// Index = array slot (0..4 per TS; unbounded here — first-access-wins).
Arrays [][]int32
```

- `OpDefineArray` (44): pops length, reads `IntOperands[PC]` for slot number. Grows `Arrays` if needed, allocates `[]int32` of requested length (zeroed). TS caps at 5; we'll match (5 slot limit → return error if idx >= 5).
- `OpPushArrayInt` (45): pops index, reads slot from operand, pushes `Arrays[slot][idx]` or 0 if OOB.
- `OpPopArrayInt` (46): pops index and value, reads slot from operand, writes `Arrays[slot][idx] = val` (no-op if OOB).
- `OpSwitch` (24): pops key; `table := Script.SwitchTables[IntOperands[PC]]`; if `offset, ok := table[key]`, set `PC += int(offset)`; otherwise fall through (no branch).

### 5. `ActivePlayer.Playtime() int`

Add to `pkg/script/active.go`:

```go
// Playtime returns the number of ticks the player has been online this
// session, used by the TIMESPENT / GETTIMESPENT opcodes.
Playtime() int
```

And in `modules/world/player_script.go`:

```go
// Playtime implements script.ActivePlayer.
func (p *Player) Playtime() int { return int(p.playtime) }
```

(`playtime` is already incremented in `processIn`; see `TestProcessInIncrementsPlaytime`.)

## Data flow

One new slice on `ScriptState` (`Arrays`), one new field on nobody else. All other state changes are local pushes/pops. No server-tick integration. No ActivePlayer wiring beyond `Playtime()`.

## Error handling

- **Divide by zero / Scale by zero / Modulo by zero**: return error from handler. Execute dispatch already routes handler errors to `Execution = Aborted`.
- **Array slot >= 5**: return error. (TS throws at parse time via compiler limit, but we enforce at runtime for safety.)
- **`OpError`**: is an error by design; returns a scripted error. Dispatch code handles it identically to any other handler error.
- **Missing ActivePlayer for `Playtime`**: return error (matches existing `MES`/`NAME` pattern).
- **`SWITCH` operand out of range**: guard `IntOperands[PC] < len(SwitchTables)`; fall through otherwise.
- **`TEXT_SWITCH`**: no string-switch table infrastructure yet; stub with warn log for S5a.

## Testing

Table-driven tests wherever the arity is uniform. Example for arithmetic:

```go
func TestNumberHandlers(t *testing.T) {
    cases := []struct {
        name   string
        op     Opcode
        inputs []int32 // pushed bottom-to-top
        expect int32
    }{
        {"multiply pos", OpMultiply, []int32{6, 7}, 42},
        {"multiply neg", OpMultiply, []int32{-3, 5}, -15},
        {"divide floor neg", OpDivide, []int32{-7, 2}, -4},
        {"modulo pos", OpModulo, []int32{10, 3}, 1},
        {"modulo neg", OpModulo, []int32{-7, 3}, 2},   // TS positive remainder
        {"abs neg", OpAbs, []int32{-9}, 9},
        {"min", OpMin, []int32{5, 3}, 3},
        {"bitcount", OpBitCount, []int32{0xFF}, 8},
        {"testbit set", OpTestBit, []int32{0b1010, 1}, 1},
        {"testbit clear", OpTestBit, []int32{0b1010, 0}, 0},
        // ... ~60 rows covering all arithmetic + bitwise handlers
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) { ... })
    }
}
```

String tests use similar table-driven style with string stacks.

Switch tests check three cases: hit, miss (fall-through), out-of-range operand.

Random tests assert range (`got >= 0 && got < n`) over 100 iterations rather than exact values, to avoid flake.

Error tests assert `Execute(state)` returns non-nil error and `state.Execution == Aborted` afterwards.

Playtime test: one in `modules/world/script_test.go` that sets `p.playtime = 42`, runs a `getTimeSpent` script that emits via `mes`, asserts the wire shows "42".

## LOC estimate

| File | LOC |
|---|---|
| `pkg/script/handlers_number.go` | ~280 |
| `pkg/script/handlers_string.go` | ~200 |
| `pkg/script/handlers_debug.go` | ~40 |
| `pkg/script/handlers_array.go` | ~90 |
| `pkg/script/state.go` (diff) | +3 |
| `pkg/script/active.go` (diff) | +3 |
| `pkg/script/handlers.go` (diff) | +40 (register all 40) |
| `modules/world/player_script.go` (diff) | +3 |
| `pkg/script/handlers_number_test.go` | ~200 |
| `pkg/script/handlers_string_test.go` | ~150 |
| `pkg/script/handlers_debug_test.go` | ~50 |
| `pkg/script/handlers_array_test.go` | ~80 |
| `modules/world/script_test.go` (diff) | +30 (playtime test) |
| **Total** | **~1170** |

One sub-spec. The opcodes are mechanical and repetitive — table-driven tests keep test code compact.

## Key design calls

- **Handler file split by TS category**, not alphabetical. Mirrors the upstream layout so diffing against TS changes stays cheap.
- **`handlers` map stays in `handlers.go`.** Split files just define free functions and register via the map literal extension. One source of truth for "which opcodes does this VM handle".
- **Floor division** matches TS `Math.floor(a/b)`, not Go's truncate-toward-zero. Worth a dedicated helper `floorDiv(a, b int32) int32` in `handlers_number.go`.
- **Positive-modulo convention**: TS `((a % b) + b) % b`. Needed because Go `%` can return negative.
- **`TEXT_SWITCH` stubbed** rather than half-implemented. The string-switch table format isn't part of our current file decoder; adding it is ~30 LOC but not load-bearing for any script we care about today.
- **`SPLIT_*` stubbed at `slog.Debug`** so production logs stay quiet but we can grep for what cache scripts want once dialog work starts.
- **No new world-server state.** The only cross-package addition is `Playtime()` on `ActivePlayer`, which pipes to an already-maintained `Player.playtime` field.
- **Random is non-deterministic** via `math/rand/v2.IntN`. If tests flake on bitmap coverage we can thread a `Rand *rand.Rand` onto ScriptState later; not needed for MVP.

## Gotchas

- **Pop order for binary ops**: `rhs = PopInt(); lhs = PopInt(); push(lhs op rhs)`. Easy to get backwards on non-commutative ops (SUB, DIVIDE, MODULO).
- **Go `%` vs TS `%`**: `-7 % 3` is `-1` in Go, `2` in TS for the ES5 semantics cache uses. Helper required.
- **`POW` overflow**: TS casts to 32-bit via `Math.imul` equivalent — we need int32 wraparound. `int32(lhs) * int32(rhs)` suffices for multiply; for `POW` iterate with int32 math and let overflow wrap.
- **`APPEND` pop order**: `suffix = PopString(); base = PopString()` — the suffix is on top. `COMPARE` and `STRING_INDEXOF_STRING` have the same trap.
- **`OpError` test**: ensure `Execute` returns the error, AND `state.Execution == Aborted`. Our existing dispatch loop already handles this — the test just verifies both post-conditions.
- **Array slot operand**: `IntOperands[PC]` for `DEFINE_ARRAY`/`PUSH_ARRAY_INT`/`POP_ARRAY_INT` is the slot number (0..4), not the length. Length is popped from the int stack for `DEFINE_ARRAY`.
- **`SWITCH` fall-through**: on lookup miss, `PC` is unchanged — the dispatch loop's `PC++` at the end of the iteration advances to the next instruction. This is the TS behavior.
- **`handlers` map literal is single-key — double-registration will silent-overwrite.** Code review should compare the map vs. the combined opcode list to catch typos.
