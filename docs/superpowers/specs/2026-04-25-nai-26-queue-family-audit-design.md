# NAI-26 — queue family TS-faithfulness audit (STRONGQUEUE / WEAKQUEUE / QUEUE / LONGQUEUE / P_DELAY)

- **Sub-spec**: NAI-26
- **Date**: 2026-04-25
- **Scope label**: A (TS-faithfulness audit, NAI-25 Bundle 1-style cadence applied to a 5-opcode family — `pkg/script/handlers.go` queue-family + `pkg/script/active.go` interface + `modules/world/player_script.go` + `modules/world/player.go` + `modules/world/tick.go` + 4 test files; ~250-300 LOC production + ~150-200 LOC tests across 2 bundles; resolves From-NAI-25 STRONGQUEUE/P_DELAY tracker entry plus 8 audit-discovered structural divergences in the same family; introduces 0 new deviation tags by default; net deviation count 14 → 14)
- **Predecessors**: NAI-25 (follow-up bundle) — last on `main` as `f0d1ed9`
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

The From-NAI-25 tracker entry at `nai_followups.md:1574-1617` enumerates two PlayerOps.ts NumberNotNull sites that NAI-25 Bundle 2's audit charter intentionally excluded (handlers.go was outside the spec's 13-file in-scope list): `STRONGQUEUE:99` and `P_DELAY:377`. The tracker frames the work as "~10 LOC, 2 `checkNotNull` wraps, eligible for compressed cadence."

Brainstorm-time line-by-line audit against TS `Engine-TS/src/engine/script/handlers/PlayerOps.ts:97-180,375-379` reframed the scope. Per `audit_full_method_against_ts` memory (the lesson freshly recorded at NAI-25 Bundle 1 close): a tracker entry pointing at one wrap can sit atop a larger structural divergence. Auditing the entire queue family revealed **8 additional divergences** hiding under goscape's shared `enqueueTyped` helper (which currently fronts QUEUE / WEAKQUEUE / STRONGQUEUE / LONGQUEUE with a uniform 3-popInt body modeled on `QUEUE`).

The 8 discovered divergences:

1. (α) **STRONGQUEUE NumberNotNull on `delay` missing.** TS PlayerOps.ts:99 wraps with `check(state.popInt(), NumberNotNull)`. (Already known from tracker.)
2. (β) **STRONGQUEUE `popScriptArgs(state)` missing.** TS PlayerOps.ts:98 calls `popScriptArgs(state)` *first* (popping a type-tags string then N typed args). Goscape's `enqueueTyped` instead pops a single int `arg` for STRONGQUEUE — silently using the QUEUE shape for a variadic opcode. Goscape's helper docstring acknowledges "VARARG variants are deferred"; STRONGQUEUE itself is the variadic shape per TS, not a separate `*VARARG` opcode.
3. (γ) **STRONGQUEUE script-missing error missing.** TS PlayerOps.ts:103-105 throws `"Unable to find queue script: ${scriptId}"` if `ScriptProvider.get` returns null. Goscape's `(*Player).EnqueueScriptTyped` silently no-ops on missing script (intentional pre-existing engine-dispatch tolerance — see `EnqueueScriptFile` nil-check rationale at `player_script.go:51-61`). For opcode-driven dispatch, the silent no-op masks script-author errors that TS surfaces.
4. (δ) **WEAKQUEUE script-missing error missing.** Same as (γ); TS PlayerOps.ts:127-129.
5. (ε) **QUEUE script-missing error missing.** Same as (γ); TS PlayerOps.ts:152-154.
6. (ζ) **LONGQUEUE `popInts(4)` missing.** TS PlayerOps.ts:172 pops `[scriptId, delay, arg, logoutAction]` (4 ints). Goscape's `enqueueTyped` pops only 3 (sharing the QUEUE shape). The `logoutAction` arg is silently dropped from the script's args.
7. (η) **LONGQUEUE `[logoutAction, arg]` 2-arg array missing.** TS PlayerOps.ts:179 passes `[logoutAction, arg]` to `enqueueScript` as a 2-element args array. Goscape passes `[arg]` (single int).
8. (θ) **LONGQUEUE script-missing error missing.** Same as (γ); TS PlayerOps.ts:175-177.
9. (κ) **P_DELAY NumberNotNull on `n` missing.** TS PlayerOps.ts:377 wraps with `check(state.popInt(), NumberNotNull)`. (Already known from tracker.)

Letters left intentionally non-contiguous (skip ι) to mirror the existing convention of one-letter-per-divergence in the audit narrative.

The structural divergences (β, ζ, η) require a type-system widening: `playerQueueRequest` currently stores `IntArg int` (single int arg); TS `PlayerQueueRequest.args` is `ScriptArgument[]` (mixed-type variadic). Goscape's existing parallel-slice convention (`runScript(sf, self, target, intArgs []int, stringArgs []string)` per `tick.go:243-246` and `handlers.go:516-523`) is the natural extension: widen `playerQueueRequest` to `IntArgs []int` + `StringArgs []string`, and provide a `popScriptArgs` helper that pops the type-tags string and fills the two parallel slices.

The script-missing-error divergence (γ/δ/ε/θ) requires changing `script.ActivePlayer.EnqueueScriptTyped` to return an error.

The two new TS-faithfulness disciplines from NAI-25 close (`audit_full_method_against_ts`, `tracker_entry_framing_can_be_incomplete`) drove this brainstorm — the re-derivation from primary sources surfaced the 8 additional divergences that the tracker's narrow framing did not name.

## Tech stack

- Go 1.26+
- Existing packages touched:
  - `pkg/script/handlers.go` (un-share `enqueueTyped` → 4 own handlers; new `popScriptArgs` helper; NumberNotNull wraps on STRONGQUEUE delay + P_DELAY n)
  - `pkg/script/active.go` (`EnqueueScriptTyped` → `EnqueueScriptArgs(scriptID, delay, intArgs, stringArgs, qtype) error` — signature change + error return)
  - `modules/world/player_script.go` (`playerQueueRequest` struct field rename at `:25-30` (`IntArg int` → `IntArgs []int` + `StringArgs []string`); `EnqueueScriptFile` signature widening; `EnqueueScriptTyped` → `EnqueueScriptArgs` rename + error-returning body; engine-dispatch sites at `:263, :285` migrate from `intArg=0` to `nil, nil`)
  - `modules/world/player.go` is **not** touched. The `IntArg int` at `player.go:45` belongs to `playerTimer` (a sibling struct for `SetTimer` requests, out of scope per the timer-family deferral in §Out-of-scope)
  - `modules/world/tick.go` (`processPlayerQueue`: pass `req.IntArgs, req.StringArgs` to `s.runScript`)
- Test files touched:
  - `pkg/script/handlers_test.go` (`TestQueueOpcode`, `TestQueueVariants` updated; new tests for STRONGQUEUE popScriptArgs / LONGQUEUE 4-popInt / null-pin / script-missing)
  - `pkg/script/runner_test.go` (`mockPlayer.EnqueueScriptTyped` → `EnqueueScriptArgs`)
  - `modules/world/player_script_test.go` (`TestEnqueueScriptFileDirectPath` / `TestEnqueueScriptFileNilIsNoop` widened)
  - `modules/world/script_test.go` (6 `EnqueueScriptTyped` call sites at `:239, :269, :336, :337, :825, :1020` migrated to `EnqueueScriptArgs(id, delay, nil, nil, qtype)`)
- Memory file:
  - `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (NAI-26 close: marks From-NAI-25 entry at `:1574-1617` Resolved with the audit reframing — "tracker scope was 2 wraps; full-method audit found 8 additional divergences; all 9+ remediated TS-faithfully")
- No new files in production packages.

## Scope

### Bundle 1 — Storage + signature widening (mechanical, ~80-100 LOC)

**Goal**: Widen `playerQueueRequest`, `script.ActivePlayer.EnqueueScriptArgs`, `(*Player).EnqueueScriptFile`, `(*Player).EnqueueScriptArgs`, and `processPlayerQueue` to carry parallel `[]int` + `[]string` slices instead of a single `intArg int`. Add the script-missing error return on `EnqueueScriptArgs`. Existing 4 queue handlers remain fronted by a temporary `enqueueTyped` adapter that wraps `[]int{arg}` into the new shape — no behavioral change, no new tests in Bundle 1. Establishes the foundation for Bundle 2's per-opcode TS-faithful bodies.

**Source**: NAI-26 Bundle 1 (storage layer).

#### Touch points

1. **`modules/world/player_script.go`**:
   - `playerQueueRequest` struct (`:25-30` per HEAD `f0d1ed9`): `IntArg int` (line `:28`) → `IntArgs []int` + `StringArgs []string`. Verify struct location at controller pre-flight via `grep -n "type playerQueueRequest" modules/world/`.
   - `EnqueueScriptFile` signature (`:51`): `(sf *script.ScriptFile, delay, intArg int, qtype script.PlayerQueueType)` → `(sf *script.ScriptFile, delay int, intArgs []int, stringArgs []string, qtype script.PlayerQueueType)`. Body assigns `IntArgs: intArgs, StringArgs: stringArgs` to the queue request.
   - `EnqueueScriptTyped` rename → `EnqueueScriptArgs`, signature `(scriptID uint32, delay int, intArgs []int, stringArgs []string, qtype script.PlayerQueueType) error`. **Bundle 1 body** (silent-no-op preserved): resolves script via `scriptProvider.GetByID`; if nil, returns `nil` (Bundle 1 placeholder; Bundle 2 activates the `fmt.Errorf("unable to find queue script: %d", scriptID)` return per the sequencing decision in touch point #4 below). Otherwise delegates to `EnqueueScriptFile` with the parallel slices.
   - Engine-dispatch sites at `:263` and `:285`: migrate from `p.EnqueueScriptFile(sf, 0, 0, script.QueueNormal)` → `p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueNormal)` (TS-faithful: engine dispatch passes `args=[]` per Player.ts:821 default).

2. **`pkg/script/active.go`**:
   - Line `:19`: `EnqueueScriptTyped(scriptID uint32, delay int, intArg int, qtype PlayerQueueType)` → `EnqueueScriptArgs(scriptID uint32, delay int, intArgs []int, stringArgs []string, qtype PlayerQueueType) error`.
   - Update interface contract docstring at `:15-19` to narrate the parallel-slice arg passing and the error-return contract (`error` is non-nil when `scriptID` does not resolve to a registered script — mirrors TS PlayerOps.ts:103-105).

3. **`modules/world/tick.go`**:
   - Line `:243-246`: `intArg := req.IntArg; ... s.runScript(sf, p, false, []int{intArg}, nil)` → `s.runScript(sf, p, false, req.IntArgs, req.StringArgs)`.
   - Verify the surrounding processPlayerQueue body's other field reads (delay, type, script lookup) remain unchanged.

4. **`pkg/script/handlers.go`**:
   - `enqueueTyped` (line `:599-619`): retained as a temporary adapter; body changes from `s.Self.EnqueueScriptTyped(scriptID, delay, arg, qtype)` to `return s.Self.EnqueueScriptArgs(scriptID, delay, []int{arg}, nil, qtype)`. Returns the error from `EnqueueScriptArgs` directly. Behavior preserved (silent no-op on missing script preserved if EnqueueScriptArgs returns nil for that case in Bundle 1; **see Bundle 2 plan for the script-missing error rollout**).
   - **Critical sequencing decision**: in Bundle 1, `EnqueueScriptArgs` returns nil instead of the script-missing error so the behavior change (silent-no-op → error-return on missing script) lands in its own bundle separate from the mechanical signature widening. **Verified at plan-write (commit `93dfd2a`)**: all 6 `script_test.go` call sites at lines 239/269/336/337/825/1020 enqueue scripts that are **registered** in the test fixture just before the call (`s.scriptProvider.RegisterAt(0xAAAA, ...)` at `:231`, `0xBBBB` at `:261`, `0xCCC1/0xCCC2` at `:327-328`, `0xBEEF` at `:808`, `0xBEE2` at `:1006`). No test exercises the missing-script path. The Bundle 1 nil-return is therefore a no-op for observable behavior — its purpose is review-surface isolation (mechanical widening vs. behavior change in separate bundles), not test-behavior preservation. Alternative considered (return error in Bundle 1): rejected — couples mechanical signature widening to behavior changes; increases Bundle 1 review surface.

5. **`pkg/script/runner_test.go`**:
   - Line `:259`: `mockPlayer.EnqueueScriptTyped(id, delay, arg, qtype)` → `mockPlayer.EnqueueScriptArgs(id, delay, intArgs, stringArgs, qtype) error`.
   - Mock body records `IntArgs []int`, `StringArgs []string`, returns nil error.
   - Update `mockEnqueue` struct (`:249`): `IntArg int` → `IntArgs []int` + `StringArgs []string`.

6. **`modules/world/script_test.go`** (6 call sites):
   - Lines `:239, :269, :336, :337, :825, :1020`: `p.EnqueueScriptTyped(id, delay, 0, script.QueueX)` → `p.EnqueueScriptArgs(id, delay, nil, nil, script.QueueX)`. The integration tests don't exercise args; nil/nil expresses "no args" and matches the TS-faithful engine-dispatch convention.
   - **Plan-time verification correction**: each call site enqueues a script that is **registered** in the surrounding test fixture (verified at plan-write commit `93dfd2a`). The migration is purely a signature update — the test scripts hit the happy path, not the missing-script path. Bundle 2's per-site evaluation is a no-op confirmation pass; no functional migration is needed.

7. **`modules/world/player_script_test.go`**:
   - Line `:167`: `p.EnqueueScriptFile(sf, 3, 42, script.QueueNormal)` → `p.EnqueueScriptFile(sf, 3, []int{42}, nil, script.QueueNormal)`. Assertion at `:178` (`req.IntArg != 42`) → `len(req.IntArgs) != 1 || req.IntArgs[0] != 42`.
   - Line `:188`: `p.EnqueueScriptFile(nil, 0, 0, script.QueueNormal)` → `p.EnqueueScriptFile(nil, 0, nil, nil, script.QueueNormal)`. Nil-short-circuit semantics preserved.

8. **`pkg/script/handlers_test.go`** (existing tests):
   - `TestQueueOpcode` (`:407`): assertion at `:432-435` (`mockEnqueue{ScriptID: 77, Delay: 3, IntArg: 42}`) → `mockEnqueue{ScriptID: 77, Delay: 3, IntArgs: []int{42}, StringArgs: nil}`. Use `slices.Equal` or per-field assertions for slice comparison (per `use-modern-go` Go 1.21+ `slices.Equal`).
   - `TestQueueVariants` (`:439`): same per-variant assertion update.

9. **`modules/world/npc_event_queue_test.go`** + `modules/world/npc_test.go`:
    - **Verify out-of-scope at controller pre-flight**: NPC queue path is `NpcQueueRequest` (separate struct), not `playerQueueRequest`. The `IntArg int` references at `npc_event_queue_test.go:61`, `npc_test.go:465, :530` are NPC queue, not player queue. NPC queue family is **not** in NAI-26 scope. (TS NPC-queue ops live in NpcOps.ts; their TS-faithfulness audit is a separate sub-spec if/when needed.)

#### Bundle 1 deviation impact

0 — mechanical signature widening + error-return-disabled adapter. No behavioral change; no deviation tags introduced or retired.

#### Bundle 1 commit shape

Single feat commit: `feat(world,script): NAI-26 Bundle 1 — playerQueueRequest parallel-slice plumbing`. Body explains the type-system widening, the temporary enqueueTyped adapter, the engine-dispatch nil-migration (player_script.go:263, :285), and the deferred error-return rollout (Bundle 2). Standard `Co-Authored-By: Claude Opus 4.7 (1M context)` trailer.

### Bundle 2 — Per-opcode TS-faithfulness audit + popScriptArgs + NumberNotNull (~150-200 LOC + ~100-150 LOC tests)

**Goal**: Un-share STRONGQUEUE / WEAKQUEUE / QUEUE / LONGQUEUE handlers from `enqueueTyped`; give each its TS-faithful body. Add `popScriptArgs` helper. Add NumberNotNull wraps on STRONGQUEUE delay + P_DELAY n. Mirror script-missing error for all 4 queue handlers. Migrate the 6 `script_test.go` integration tests that currently rely on silent no-op to either (a) register the test scripts in the test fixture, or (b) explicitly assert the error. Resolve the From-NAI-25 tracker entry as "Tracker scope was 2 wraps; full-method audit found 8 additional divergences; all 9+ remediated TS-faithfully."

**Source**: NAI-26 Bundle 2 (the actual TS-faithfulness work).

#### TS source canonical paths

Per `ts_source_canonical_path` memory:
- `Engine-TS/src/engine/script/handlers/PlayerOps.ts:97-180` — STRONGQUEUE / WEAKQUEUE / QUEUE / LONGQUEUE bodies.
- `Engine-TS/src/engine/script/handlers/PlayerOps.ts:375-379` — P_DELAY body.
- `Engine-TS/src/engine/script/handlers/PlayerOps.ts:1248-1263` — `popScriptArgs` helper.
- `Engine-TS/src/engine/entity/PlayerQueueRequest.ts:15` — `ScriptArgument = number | string`.
- `Engine-TS/src/engine/entity/Player.ts:821` — `enqueueScript` default `args=[]`.

#### popScriptArgs helper (new)

**TS source** (`PlayerOps.ts:1248-1263`):
```ts
function popScriptArgs(state: ScriptState): ScriptArgument[] {
    const types = state.popString();
    const count = types.length;

    const args: ScriptArgument[] = [];
    for (let i = count - 1; i >= 0; i--) {
        const type = types.charAt(i);

        if (type === 's') {
            args[i] = state.popString();
        } else {
            args[i] = state.popInt();
        }
    }
    return args;
}
```

**Goscape port** (parallel-slice adaptation):
```go
// popScriptArgs pops a type-tags string from the stack, then pops typed
// args in reverse tag order (i = count-1 down to 0): tag char 's' pops
// a string into stringArgs; any other tag pops an int into intArgs.
// Mirrors TS PlayerOps.ts:1248-1263.
//
// TS returns []ScriptArgument (a single ordered slice with mixed types
// indexed by tag position). Goscape's parallel-slice convention encodes
// the same data with two slices: each tag's value lands in the slice
// for its type, in tag-position order. The caller does not need to
// reconstruct positional access — runScript consumes intArgs and
// stringArgs separately, and ScriptState.Init unpacks each into its
// own typed local-variable slot per IntArgCount / StringArgCount.
func popScriptArgs(s *ScriptState) (intArgs []int, stringArgs []string) {
    types := s.PopString()
    count := len(types)
    if count == 0 {
        return nil, nil
    }
    // Pre-pass: count int and string tags to size the slices.
    var intCount, stringCount int
    for _, t := range types {
        if t == 's' {
            stringCount++
        } else {
            intCount++
        }
    }
    intArgs = make([]int, intCount)
    stringArgs = make([]string, stringCount)
    // Reverse-pop pass: TS iterates i = count-1 down to 0.
    intIdx := intCount - 1
    stringIdx := stringCount - 1
    for i := count - 1; i >= 0; i-- {
        if types[i] == 's' {
            stringArgs[stringIdx] = s.PopString()
            stringIdx--
        } else {
            intArgs[intIdx] = int(s.PopInt())
            intIdx--
        }
    }
    return intArgs, stringArgs
}
```

**Co-located in `pkg/script/handlers.go`** near the queue handlers. Per `plan_grep_helper_patterns` memory: the file already has helpers (e.g., `enqueueTyped` itself), so co-location is the established pattern.

**Order semantics**: TS's `args[i]` indexes the unified slice by tag position. Goscape's parallel slices preserve same order: an `"isi"` tag string with values `popInt → "hello" → popInt` yields `intArgs=[i0, i2]`, `stringArgs=["hello"]` — int args land in tag-position-relative-to-int-count order; string args land in tag-position-relative-to-string-count order. Tests pin this.

#### handleStrongQueue (un-shared body)

**TS source** (`PlayerOps.ts:97-108`):
```ts
[ScriptOpcode.STRONGQUEUE]: checkedHandler(ActivePlayer, state => {
    const args = popScriptArgs(state);
    const delay = check(state.popInt(), NumberNotNull);
    const scriptId = state.popInt();

    const script = ScriptProvider.get(scriptId);
    if (!script) {
        throw new Error(`Unable to find queue script: ${scriptId}`);
    }

    state.activePlayer.enqueueScript(script, PlayerQueueType.STRONG, delay, args);
}),
```

**Goscape port**:
```go
// handleStrongQueue implements STRONGQUEUE (opcode 86): pop variadic
// typed args via popScriptArgs, then pop delay (NumberNotNull-checked),
// then pop scriptID, and enqueue a STRONG-typed queue request. Mirrors
// TS PlayerOps.ts:97-108 line-by-line.
func handleStrongQueue(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return fmt.Errorf("STRONGQUEUE: no active player")
    }
    intArgs, stringArgs := popScriptArgs(s)
    delay := int(s.PopInt())
    if err := checkNotNull(delay, "STRONGQUEUE"); err != nil {
        return err
    }
    scriptID := uint32(s.PopInt())
    return s.Self.EnqueueScriptArgs(scriptID, delay, intArgs, stringArgs, QueueStrong)
}
```

#### handleWeakQueue (un-shared body)

**TS source** (`PlayerOps.ts:123-132`):
```ts
[ScriptOpcode.WEAKQUEUE]: checkedHandler(ActivePlayer, state => {
    const [scriptId, delay, arg] = state.popInts(3);
    // ...script lookup + error...
    state.activePlayer.enqueueScript(script, PlayerQueueType.WEAK, delay, [arg]);
}),
```

**Goscape port**:
```go
// handleWeakQueue implements WEAKQUEUE: pop scriptID/delay/arg (3 ints)
// and enqueue a WEAK-typed queue request with [arg] as the args.
// Mirrors TS PlayerOps.ts:123-132.
func handleWeakQueue(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return fmt.Errorf("WEAKQUEUE: no active player")
    }
    arg := int(s.PopInt())
    delay := int(s.PopInt())
    scriptID := uint32(s.PopInt())
    return s.Self.EnqueueScriptArgs(scriptID, delay, []int{arg}, nil, QueueWeak)
}
```

#### handleQueue (un-shared body)

**TS source** (`PlayerOps.ts:148-157`): structurally identical to WEAKQUEUE except `PlayerQueueType.NORMAL`.

**Goscape port**:
```go
// handleQueue implements QUEUE: pop scriptID/delay/arg (3 ints) and
// enqueue a NORMAL-typed queue request with [arg] as the args. Mirrors
// TS PlayerOps.ts:148-157.
func handleQueue(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return fmt.Errorf("QUEUE: no active player")
    }
    arg := int(s.PopInt())
    delay := int(s.PopInt())
    scriptID := uint32(s.PopInt())
    return s.Self.EnqueueScriptArgs(scriptID, delay, []int{arg}, nil, QueueNormal)
}
```

#### handleLongQueue (un-shared body)

**TS source** (`PlayerOps.ts:171-180`):
```ts
[ScriptOpcode.LONGQUEUE]: checkedHandler(ActivePlayer, state => {
    const [scriptId, delay, arg, logoutAction] = state.popInts(4);
    // ...script lookup + error...
    state.activePlayer.enqueueScript(script, PlayerQueueType.LONG, delay, [logoutAction, arg]);
}),
```

**Goscape port**:
```go
// handleLongQueue implements LONGQUEUE: pop scriptID/delay/arg/logoutAction
// (4 ints) and enqueue a LONG-typed queue request with [logoutAction, arg]
// as the args. Mirrors TS PlayerOps.ts:171-180. The 4th popInt
// (logoutAction) is what distinguishes LONGQUEUE from QUEUE/WEAKQUEUE/
// STRONGQUEUE; the args array is 2 elements ordered logoutAction-first.
func handleLongQueue(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return fmt.Errorf("LONGQUEUE: no active player")
    }
    logoutAction := int(s.PopInt())
    arg := int(s.PopInt())
    delay := int(s.PopInt())
    scriptID := uint32(s.PopInt())
    return s.Self.EnqueueScriptArgs(scriptID, delay, []int{logoutAction, arg}, nil, QueueLong)
}
```

#### handlePDelay (NumberNotNull wrap added)

**TS source** (`PlayerOps.ts:375-379`):
```ts
[ScriptOpcode.P_DELAY]: checkedHandler(ProtectedActivePlayer, state => {
    state.activePlayer.delayed = true;
    state.activePlayer.delayedUntil = World.currentTick + 1 + check(state.popInt(), NumberNotNull);
    state.execution = ScriptState.SUSPENDED;
}),
```

**Goscape post-Bundle-2 body**:
```go
func handlePDelay(s *ScriptState) error {
    if err := requireProtectedActivePlayer(s, "P_DELAY"); err != nil {
        return err
    }
    n := int(s.PopInt())
    if err := checkNotNull(n, "P_DELAY"); err != nil {
        return err
    }
    s.Self.SetDelayed(n)
    s.Execution = Suspended
    return nil
}
```

The `delayed = true` flag and `delayedUntil = currentTick + 1 + n` calculation remain encapsulated in `(*Player).SetDelayed` (per `player_script.go:38-44`); already TS-faithful at the method-level.

#### enqueueTyped removal

Bundle 1 retained `enqueueTyped` as a temporary adapter. Bundle 2 removes it once all 4 queue handlers have own bodies. Removal commit-internal; no separate touch point.

#### `EnqueueScriptArgs` script-missing error rollout (deferred from Bundle 1)

In Bundle 2, `(*Player).EnqueueScriptArgs` returns `fmt.Errorf("unable to find queue script: %d", scriptID)` when `scriptProvider.GetByID` returns nil. The 4 un-shared queue handlers propagate this error up via their `return` path (signature `func(*ScriptState) error` is already error-returning). The dispatch loop at `pkg/script/runner.go` already handles handler errors.

The 6 `script_test.go` call sites get a no-op confirmation pass:
- **Plan-time verification (commit `93dfd2a`)**: all 6 sites enqueue scripts that are registered in the same test (`0xAAAA` at `:231` registered before `:239` enqueue; `0xBBBB` at `:261` before `:269`; `0xCCC1/0xCCC2` at `:327-328` before `:336/:337`; `0xBEEF` at `:808` before `:825`; `0xBEE2` at `:1006` before `:1020`). No test exercises the missing-script path. Activating the script-missing error in Bundle 2 has no observable effect on these tests beyond the signature update already made in Bundle 1.
- **Spec note**: the original spec (pre-correction) framed these as "tests that intentionally enqueue non-existent scripts" — that framing was incorrect. The Bundle 1/Bundle 2 split rationale (review-surface isolation) remains valid; the rationale is no longer about preserving observable test behavior because no test ever depended on the silent-no-op.

#### Touch points

1. **`pkg/script/handlers.go`**:
   - Add `popScriptArgs` helper (new function, ~25 LOC).
   - Replace `enqueueTyped` body (used through Bundle 1) with removal; un-share STRONGQUEUE / WEAKQUEUE / QUEUE / LONGQUEUE handlers per the TS-faithful bodies above.
   - Add NumberNotNull wrap to `handlePDelay`.
   - **Verify**: re-grep `pkg/script/handlers.go` at controller pre-flight to confirm `enqueueTyped` has no consumers outside the 4 queue handlers — per `enumerate_all_sites` memory.

2. **`modules/world/player_script.go`**:
   - Activate the script-missing error in `EnqueueScriptArgs` (Bundle 1 placeholder returned nil; Bundle 2 returns the formatted error).

3. **`modules/world/script_test.go`** (6 call sites):
   - Per-site re-evaluation per the plan-write enumeration. Each site either:
     - Migrates to a real script ID registered in the test fixture (preserves silent-no-op intent without using a sentinel), OR
     - Asserts the error is returned (for tests that semantically pin "missing script is an error").

4. **`pkg/script/handlers_test.go`** (new + updated tests per the testing strategy below).

5. **`pkg/script/runner_test.go`**: `mockPlayer.EnqueueScriptArgs` body updated to support both nil-error (Bundle 1) and configurable error-return (Bundle 2 — for script-missing tests). Pattern: a `wantScriptMissing func(uint32) bool` field on `mockPlayer` configurable per-test.

#### Tests

Per `plan_test_coverage_crosscheck` memory: every divergence in the audit table maps to at least one pinning test.

**Per-divergence test mapping**:

| Divergence | New / updated test | Pin shape |
|---|---|---|
| α STRONGQUEUE delay NumberNotNull | `TestStrongQueueDelayNullRejected` (NEW) | push tags="" string, NULL delay (-1), scriptId → expect `"STRONGQUEUE: NumberNotNull"` error |
| β STRONGQUEUE popScriptArgs (mixed) | `TestStrongQueuePopsMixedScriptArgs` (NEW) | tags="is", int arg, string arg, delay, scriptId → IntArgs=[..], StringArgs=[..] |
| β STRONGQUEUE popScriptArgs (empty) | `TestStrongQueueEmptyScriptArgs` (NEW) | tags="", delay, scriptId → IntArgs=nil, StringArgs=nil |
| β STRONGQUEUE popScriptArgs (all-int) | `TestStrongQueueAllIntScriptArgs` (NEW) | tags="iii", 3 ints, delay, scriptId → IntArgs=[..3..], StringArgs=nil |
| γ STRONGQUEUE script-missing | `TestStrongQueueScriptNotFound` (NEW) | mockPlayer.EnqueueScriptArgs returns `"unable to find queue script: <id>"` → handler propagates |
| δ WEAKQUEUE script-missing | (covered by table-driven `TestQueueScriptNotFound`) | |
| ε QUEUE script-missing | (covered by table-driven `TestQueueScriptNotFound`) | |
| ζ LONGQUEUE popInts(4) | `TestLongQueuePopsFourInts` (NEW) | scriptId, delay, arg, logoutAction → IntArgs=[logoutAction, arg] |
| η LONGQUEUE 2-arg array | (covered by ζ via IntArgs ordering assertion: position 0 = logoutAction, position 1 = arg) | |
| θ LONGQUEUE script-missing | (covered by table-driven `TestQueueScriptNotFound`) | |
| κ P_DELAY n NumberNotNull | `TestPDelayNullRejected` (NEW) | push NULL (-1) → expect `"P_DELAY: NumberNotNull"` error |

**popScriptArgs unit tests** (separate from opcode tests, exercise the helper directly via a synthetic `*ScriptState`):
- `TestPopScriptArgs_Empty` — type-tags="" → nil/nil
- `TestPopScriptArgs_AllInt` — tags="iii" → intArgs filled in tag-position order
- `TestPopScriptArgs_AllString` — tags="sss" → stringArgs filled in tag-position order
- `TestPopScriptArgs_Mixed` — tags="isi" → intArgs[0]=first-int, intArgs[1]=third-int, stringArgs[0]=second-string
- `TestPopScriptArgs_ReverseOrder` — pin reverse-pop semantics (push args in reverse, assert popped-back in forward order)

**Updates to existing tests**:
- `TestQueueOpcode` (handlers_test.go:407): existing 3-popInt fixture works for QUEUE; assertion updates to `IntArgs=[42]` not `IntArg=42`. Already updated in Bundle 1 — Bundle 2 confirms still passing.
- `TestQueueVariants` (handlers_test.go:439): in Bundle 1 the table-driven test still works with 3-popInt fixture for all 3 variants. In Bundle 2:
  - **STRONGQUEUE case removed from table** — STRONGQUEUE has different stack shape (popScriptArgs first); covered by new `TestStrongQueue*` tests.
  - **LONGQUEUE case removed from table** — LONGQUEUE pops 4 ints; covered by new `TestLongQueuePopsFourInts`.
  - WEAKQUEUE case remains in the table (3-popInt shape unchanged from QUEUE).

**Integration test** (`modules/world`):
- `TestProcessPlayerQueueDeliversAllArgs` (NEW or extension of existing) — fire a 2-int-arg queue request through `processPlayerQueue`, assert `runScript` receives both ints in `intArgs []int` parameter. Pin shape: `IntArgs=[100, 200]` → captured at `runScript` mock. Validates the Bundle 1 plumbing under realistic queue-fire conditions.

Per `plan_runnable_test_fixtures` memory: the plan author mentally executes (or `go test -run <test-name>` dry-runs) each new test before dispatch.

Per `plan_helper_coverage` memory: no new test helpers introduced. The `mockPlayer.EnqueueScriptArgs` configurable error-return (Bundle 2) is a mock-side convention, not a shared helper.

Per `enumerate_all_sites` memory: at controller pre-flight for Bundle 2, re-grep `EnqueueScriptArgs|EnqueueScriptTyped|enqueueTyped|popScriptArgs` across the repo to confirm Bundle 1 fully landed and no consumers reference the old names.

#### Bundle 2 deviation impact

0 — every divergence is remediated TS-faithfully. No deviation tags retired or introduced. The From-NAI-25 tracker entry's resolution + commit hash serves as the archaeological record.

#### Bundle 2 commit shape

Single feat commit: `feat(world,script): NAI-26 Bundle 2 — queue family TS audit (NumberNotNull + popScriptArgs + script-missing error)`. Body explains the 9 divergences (α/β/γ/δ/ε/ζ/η/θ/κ) with TS:line citations from PlayerOps.ts:97-180, 375-379, 1248-1263, the un-sharing of `enqueueTyped`, the `popScriptArgs` helper addition, the script-missing error rollout (with per-call-site migration log for the 6 `script_test.go` sites), the new tests, and the tracker resolution direction (close at NAI-26 close commit). Standard `Co-Authored-By: Claude Opus 4.7 (1M context)` trailer.

### Polish commit (between Bundle 2 close and NAI-26 close)

Standard cadence: one polish commit absorbs minor review feedback from both bundles. Per `dead_api_polish` memory: polish commit also catches any helpers shipped with zero consumers (none expected — `popScriptArgs` is consumed by `handleStrongQueue`).

## Out-of-scope (explicitly deferred)

1. **Zone state during respawn (NAI-19-D1 closure track).** Inherited deferral; needs Zone abstraction infrastructure design first.

2. **NAI-11 deferrals**: SMART pathfinding, reach helpers, focus() instant flag — each its own substantial sub-spec. Inherited deferral.

3. **NPC queue family TS-faithfulness audit** (`NpcQueueRequest` in `modules/world/npc.go`, `EnqueueScriptForTrigger` at `:213`, NpcOps.ts dispatchers). Out of scope: NAI-26 charter is `pkg/script/handlers.go` queue-family + `playerQueueRequest` only. NPC-side queue refactor is its own sub-spec if the parallel-slice convention turns out to apply there too.

4. **Player timer family** (`SetTimer` at `pkg/script/active.go:221`, `(*Player).SetTimer` at `modules/world/player_timer.go:6`, `playerTimerRequest` struct). Out of scope: timers are a sibling family with the same `intArg int` field, but their TS audit would be a separate sub-spec. Bundle 1's plumbing widening does **not** touch timer plumbing — those calls stay on `intArg int` until a future NAI-N timer audit.

5. **`*VARARG` opcode family** (`STRONGQUEUEVARARG` at PlayerOps.ts:110-120, `WEAKQUEUEVARARG` at :134-144, `QUEUEVARARG` at :159-169, `LONGQUEUEVARARG` at :182-192). Out of scope: distinct opcodes in TS; goscape has not yet ported them. The current goscape comment at `enqueueTyped` saying "VARARG variants are deferred" remains accurate. Future sub-spec.

6. **`Active*Player` interface naming churn** if the interface rename (`EnqueueScriptTyped` → `EnqueueScriptArgs`) suggests broader simplification. Out of scope: rename scope is bounded to the queue-family method. Other interface methods (`SetTimer`, `EnqueueScriptForTrigger`) keep their current names.

## Risks & mitigations

- **Bundle 1 `playerQueueRequest` struct location.** Risk: spec assumes the struct lives in `modules/world/player_script.go:25-30`, but the field-rename target referenced from grep was at `modules/world/player.go:45`. Mitigation: controller pre-flight re-greps `type.*Request struct` across `modules/world/`; the implementer's first action is to confirm the struct location before editing.

- **Bundle 1 `(*Server).runScript` signature.** Risk: `runScript` may not accept `[]string` as a 5th arg if the existing convention pre-dates string-arg support. Mitigation: pre-flight verifies via `grep -n "func.*runScript\b" modules/world/script.go` (or equivalent). If `runScript` takes only `[]int` today, Bundle 1 widens the signature too — adds to the bundle's mechanical scope. Pre-flight catches this.

- **Bundle 1 `processPlayerQueue` field reads.** Risk: `req.IntArg` is read at multiple points in `processPlayerQueue` (not just the one at `:243-246`). Mitigation: pre-flight re-greps `req\.IntArg` across `modules/world/tick.go`.

- **Bundle 1 6-call-site `script_test.go` migration.** Risk: a site's surrounding test logic depends on a specific `IntArg` field name in mock assertions. Mitigation: each migration is per-site evaluated; the plan author reads each test body before prescribing the migration.

- **Bundle 2 `enqueueTyped` removal cascade.** Risk: the controller pre-flight at Bundle 2 dispatch finds an unexpected consumer of `enqueueTyped` (e.g., a test added between brainstorm and dispatch). Mitigation: `enumerate_all_sites` re-grep at Bundle 2 controller pre-flight.

- **Bundle 2 `popScriptArgs` order semantics differ from TS.** Risk: TS's parallel-position semantics (`args[i]` indexes into a unified slice) differ from goscape's parallel-slice semantics in non-obvious ways for mixed type-tags. Mitigation: explicit unit tests pin both orderings — `TestPopScriptArgs_Mixed` asserts that for `tags="isi"` and stack-pushed `[1, "two", 3]`, the result is `intArgs=[1,3]` and `stringArgs=["two"]` (int args in tag-relative-int-position, string args in tag-relative-string-position). Plan author mentally executes this before dispatch.

- **Bundle 2 script-missing error message format.** Risk: the TS error message format may differ in subtle ways from goscape's `fmt.Errorf` rendering (e.g., scriptID hex vs decimal). Mitigation: pin the exact format in `TestQueueScriptNotFound` via substring match (`"unable to find queue script:"`) so the test doesn't break on hex-vs-decimal cosmetic preferences. TS uses template-literal interpolation which renders decimal — goscape's `%d` matches.

- **Bundle 2 6-site `script_test.go` migration semantic mismatch.** **Resolved at plan-write (commit `93dfd2a`)**: all 6 sites enqueue registered scripts; no test pins the silent-no-op behavior. The Bundle 2 error-return activation has no observable effect on these tests. No deviation needed.

- **`controller_preflight` discipline at task dispatch.** Per memory: 30-second grep+Read pass against HEAD before each implementer dispatch to verify file paths, line numbers, signatures, helper init state. Applied per-bundle.

- **`spec_followup_tracker_freshness` discipline.** Per memory: tracker entries silently rot. Spec-write-time re-greps verified the From-NAI-25 entry assertions: `STRONGQUEUE` site at `handlers.go:629` (verified at HEAD `f0d1ed9`); `P_DELAY` site at `handlers.go:589` (verified); `enqueueTyped` shared helper at `handlers.go:599-619` (verified). All assertions held at HEAD.

- **`audit_full_method_against_ts` discipline.** Per memory (provenance NAI-25 Bundle 1): when picking up a tracker entry that points at one line of a method, audit the entire method against TS line-by-line. Applied at this brainstorm — surfaced 8 additional divergences beyond the tracker's named 2.

- **`tracker_entry_framing_can_be_incomplete` discipline.** Per memory (provenance NAI-25 Bundle 1): tracker assertions can be fact-correct but framing-wrong. Applied at this brainstorm — the From-NAI-25 entry's "~10 LOC, 2 wraps, compressed cadence" framing was reframed to "9+ divergences, structural type-system widening, standard cadence ~250 LOC."

## Review structure

Per `runescript_cadence` memory: two-stage review per bundle (spec compliance → code quality, both via opus). Final whole-impl review after all bundles.

- **Bundle 1**: Stage 1 spec compliance (signature widening landed at every cited site; `enqueueTyped` adapter preserves silent-no-op behavior; engine-dispatch sites migrated to `nil, nil`; existing tests pass after slice-comparison updates) + Stage 2 code-quality review (slice-comparison patterns use `slices.Equal`, doc-comment narration on `EnqueueScriptArgs` interface contract, error-return-disabled rationale documented).
- **Bundle 2**: Stage 1 audit-table review (each of α/β/γ/δ/ε/ζ/η/θ/κ remediated at the cited TS:line; popScriptArgs order semantics match TS; script-missing error rollout cleanly handles all 6 `script_test.go` migrations) + Stage 2 code-quality review (test naming, popScriptArgs helper docstring, `enqueueTyped` fully removed, no shipped-with-zero-consumers helpers).
- **Whole-impl review**: validates that NAI-26 closes the From-NAI-25 tracker entry at `nai_followups.md:1574-1617` and that the per-bundle commit shapes match the spec. Verifies the 8 audit-discovered divergences (β/γ/δ/ε/ζ/η/θ + the popScriptArgs-related-to-β) all have pinning tests.

Polish commits land if final whole-impl review surfaces remediable findings, per NAI-23 / NAI-24 / NAI-25 precedent.

## NAI-26 close

The close commit:
- Updates `nai_followups.md`: marks the From-NAI-25 entry at `:1574-1617` Resolved with the audit-reframing context — "Tracker scope was 2 NumberNotNull wraps; full-method audit found 8 additional structural divergences (popScriptArgs missing, LONGQUEUE 4-popInt missing, script-missing error missing across 4 opcodes); all 9 remediated TS-faithfully."
- Per `close_commit_memory_trailer` memory: includes the standard `Co-Authored-By` trailer and `Closes memory: nai_followups.md` for the memory edits.
- Per `post_task_handoff` memory: at NAI-26 close, save non-derivable info to memory AND give the user a paste-ready resume prompt for NAI-27 (with HEAD hash, deviation count, and the most actionable next-NAI candidates).
- Memory entry candidates pre-flagged at brainstorm time (re-evaluate at close):
  - `parallel_slice_convention_for_mixed_type_args` — when porting a TS `ScriptArgument[]` (sum-type) interface to goscape, extend the existing parallel-slice convention rather than introducing a new sum type. Provenance: NAI-26 Bundle 1's `playerQueueRequest` widening. Likely save (new convention pattern).
  - `defer_behavior_change_within_mechanical_widening` — when a refactor mixes mechanical signature widening with a behavior change (here: error-return on script-missing), bundle the mechanical change first with a behavior-disabled adapter, then activate the behavior in a follow-up bundle. Provenance: NAI-26 Bundle 1 → Bundle 2 sequencing. Maybe save (overlap with general bundle-decomposition heuristics).
  - `vararg_opcode_shapes_dont_share_with_fixed-arg_siblings` — STRONGQUEUE looks like a sibling of QUEUE/WEAKQUEUE, but its TS shape is variadic via popScriptArgs. Sharing a fixed-arg helper (`enqueueTyped`) silently corrupts the variadic semantics. Provenance: NAI-26 brainstorm. Likely save.
  - `engine_dispatch_args_default_is_nil_not_zero` — engine-dispatch (changeStat, AddXP-equivalent) call sites in TS pass `args=[]` (default); goscape's `intArg=0` is a pre-existing minor divergence. Migrating to `nil, nil` aligns with TS. Provenance: NAI-26 Bundle 1 engine-dispatch migration. Maybe save (small enough to bundle into the parallel-slice memory).
