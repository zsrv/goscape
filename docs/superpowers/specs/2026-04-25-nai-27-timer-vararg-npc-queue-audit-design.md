# NAI-27 — player timer family TS-faithfulness audit + player VARARG opcode family port + NPC queue audit memo

- **Sub-spec**: NAI-27
- **Date**: 2026-04-25
- **Scope label**: A (TS-faithfulness audit, NAI-26-style cadence applied to a sibling family — `pkg/script/handlers_timer.go` (5 timer handlers) + `pkg/script/active.go` interface + `modules/world/player_timer.go` + `modules/world/player.go` `playerTimer` struct + `modules/world/tick.go` consumer + 4 new VARARG opcode handlers in a new `pkg/script/handlers_player_vararg.go` file + 0-LOC NPC queue audit memo; ~360-520 LOC production + tests across 3 bundles; resolves one untracked GETTIMER semantic divergence + introduces 4 new opcodes consuming the NAI-26 `popScriptArgs` infrastructure; introduces 0 new deviation tags by default; net deviation count 14 → 14)
- **Predecessors**: NAI-26 (queue family audit) — last on `main` as `ea725e1`
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

NAI-26 closed the player **queue** family TS-faithfulness audit by widening `playerQueueRequest` to parallel `IntArgs []int` + `StringArgs []string` slices, introducing `popScriptArgs`, activating script-missing error returns, and porting 9 line-by-line divergences across STRONGQUEUE / WEAKQUEUE / QUEUE / LONGQUEUE / P_DELAY. Two sibling families share the same infrastructure but were explicitly out-of-scope at NAI-26 close: the player **timer** family (Out-of-scope #4) and the NPC queue family (Out-of-scope #3).

Brainstorm-time line-by-line audit against TS `Engine-TS/src/engine/script/handlers/PlayerOps.ts:820-864` and `Engine-TS/src/engine/entity/Player.ts:907-941` confirmed two structural divergences in the timer family that mirror the NAI-26 queue audit pattern:

1. **`playerTimer.IntArg int` is single-int.** TS `Player.setTimer` accepts `args: ScriptArgument[]` (variadic mixed-type) per `Player.ts:908`. TS SETTIMER and SOFTTIMER handlers pop a `popScriptArgs(state)` array as the first stack pop per `PlayerOps.ts:826,834`. The NAI-26 widening pattern (parallel `[]int` + `[]string` slices) applies verbatim.
2. **GETTIMER returns "remaining ticks until next fire" instead of TS's "absolute clock".** Goscape's `(*Player).GetTimer` at `modules/world/player_timer.go:45` computes `(t.Clock + t.Interval) - now`. TS `PlayerOps.ts:858` pushes `timer.clock` directly — the absolute tick when the timer was last set/fired. This is an untracked semantic divergence with no existing deviation tag.

Two further per-handler divergences match the NAI-26 script-missing-error pattern:

3. **SETTIMER, SOFTTIMER, GETTIMER do not validate the timer scriptID.** TS `PlayerOps.ts:822-824, 838-840, 852-854` all call `ScriptProvider.get(timerId)` and throw `Unable to find timer script: ${timerId}` if nil. Goscape's handlers pass the scriptID through to the entity layer without validation — silent no-op on missing scripts (the same engine-dispatch tolerance that NAI-26 caught and remediated for the queue family).

A third family — the player **VARARG** opcode variants (`STRONGQUEUEVARARG`, `WEAKQUEUEVARARG`, `QUEUEVARARG`, `LONGQUEUEVARARG` per `PlayerOps.ts:110-192`) — is unreachable in goscape because the four opcodes are not yet wired. With NAI-26's `popScriptArgs` and `EnqueueScriptArgs` in place, the four handlers become straightforward additions that reuse the queue family's plumbing without further infrastructure changes.

A fourth family — the NPC queue (`handleNpcQueue` at `pkg/script/handlers_npc.go:316-332` vs TS `NpcOps.ts:144-150`) — was deferred at NAI-26 close. NPC queues have no TS variadic counterpart (`NpcQueueRequest` carries a single `arg: number`), so no widening is required. The audit reduces to a line-by-line check; the NAI-21 Bundle 3 strong-form test (`TestNpcTurnReentryQueueAppendDuringIteration` at `npc_script_test.go:300+`) already pins the speedup-quirk semantics, so no test-strengthening work remains.

The disciplines `audit_full_method_against_ts`, `tracker_entry_framing_can_be_incomplete`, `controller_preflight`, `enumerate_all_sites`, `parallel_slice_convention_for_mixed_type_args`, and `vararg_opcode_shapes_dont_share_with_fixed_arg_siblings` (all freshly added during NAI-25/NAI-26) drove this brainstorm — re-derivation from primary TS sources surfaced the GETTIMER semantic divergence and the script-missing-error gaps that an entry-by-entry tracker review would not have named.

## Tech stack

- Go 1.26+
- Existing packages touched:
  - `pkg/script/opcode.go` (4 new `Op*` constants for the VARARG variants; 4 new `String()` arms)
  - `pkg/script/handlers.go` (4 new dispatch-table entries for the VARARG handlers)
  - `pkg/script/handlers_timer.go` (5 handler bodies migrate to `requireActivePlayer`; SETTIMER/SOFTTIMER add `popScriptArgs` + script-missing check; GETTIMER adds script-missing check; dead `int(s.PopInt())` casts removed)
  - `pkg/script/active.go:234` (`ActivePlayer.SetTimer` signature widening: `intArg int` → `intArgs []int, stringArgs []string` AND adds `error` return for the script-missing propagation pattern; mirrors `ActivePlayer.EnqueueScriptArgs` per `active.go` queue interface added in NAI-26)
  - `modules/world/player.go` (`playerTimer` struct field replacement at `:40-46`: `IntArg int` → `IntArgs []int` + `StringArgs []string`)
  - `modules/world/player_timer.go` (`(*Player).SetTimer` signature widening at `:6` AND adds `error` return + internal `Provider.GetByID` script-missing check mirroring `(*Player).EnqueueScriptArgs` from NAI-26; `(*Player).GetTimer` semantic flip at `:45`: return `t.Clock` directly, drop the `now` lookup)
  - `modules/world/tick.go` (consumer at `:292`: `[]int{t.IntArg}` → `t.IntArgs, t.StringArgs`)
- New files in production packages:
  - `pkg/script/handlers_player_vararg.go` (4 new VARARG handler bodies — separated from `handlers_player.go` which is already 770 LOC per `wc -l`; design-for-isolation principle)
- Test files touched:
  - `pkg/script/handlers_timer_test.go` (existing 4 tests updated for the widened signature; ~6 new tests added: SETTIMER/SOFTTIMER args-capture, SETTIMER/SOFTTIMER/GETTIMER script-missing)
  - `pkg/script/runner_test.go:176-177` (`mockPlayer.SetTimer` signature widening; `lastSetTimer` recorder struct widened: `intArg int` → `intArgs []int, stringArgs []string`; Bundle 2 also widens to record the `error` return; existing assertions in `handlers_timer_test.go` updated)
  - `modules/world/player_timer_test.go` (new file — `TestPlayerGetTimerReturnsClock` + not-found pin)
  - `pkg/script/handlers_player_vararg_test.go` (new file — ~17 tests across the 4 new opcodes: 4 round-trip + 4 script-missing + 4 active-gate (or one combined parameterized) + 4 negative-pin "accepts null delay" + 1 LONGQUEUEVARARG logout-action prepend)
- NPC queue audit produces no production diff under the expected outcome (no divergence found); the audit memo lives in the close commit body.
- Memory files:
  - `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (NAI-27 close: new entry recording the timer family + VARARG family + NPC queue audit resolutions; NAI-26 Out-of-scope #3 and #4 marked Resolved)
  - Possible new memory entry on the GETTIMER pattern (untracked semantic divergences in passthrough opcodes — audit lesson)

## Scope

### Bundle 1 — Plumbing: timer struct + signature widening (mechanical, ~80-100 LOC)

**Goal**: Widen `playerTimer`, `script.ActivePlayer.SetTimer`, `(*Player).SetTimer`, and the `tick.go` consumer to carry parallel `IntArgs []int` + `StringArgs []string` slices instead of a single `IntArg int`. Existing handler bodies in `handlers_timer.go` migrate to call the widened signature with `nil, nil` placeholder slices (Bundle 2 swaps in `popScriptArgs`). No semantic changes. No new tests.

**Source mappings**:

- `playerTimer` struct (`modules/world/player.go:40-46`): replace `IntArg int` with `IntArgs []int` and `StringArgs []string`.
- `(*Player).SetTimer` (`modules/world/player_timer.go:6-21`):
  - Old: `(scriptID uint32, interval, intArg int, ttype script.PlayerTimerType)`
  - New: `(scriptID uint32, interval int, intArgs []int, stringArgs []string, ttype script.PlayerTimerType)` — **no error return in Bundle 1**; the `error` return is added in Bundle 2 alongside the script-missing check, to keep Bundle 1's diff purely mechanical.
  - Body: assign `IntArgs: intArgs, StringArgs: stringArgs` into the struct literal.
- `script.ActivePlayer.SetTimer` interface (`pkg/script/active.go:234`): same signature widening on the interface contract (`error` return deferred to Bundle 2).
- `enqueueTimer` (`pkg/script/handlers_timer.go:9-21`): drop `arg := int(s.PopInt())`, change call to `s.Self.SetTimer(scriptID, interval, nil, nil, ttype)`. Remains a `nil`-returning shape; Bundle 2 adds error propagation.
- Consumer (`modules/world/tick.go:292`): `s.runScript(sf, p, false, []int{t.IntArg}, nil)` → `s.runScript(sf, p, false, t.IntArgs, t.StringArgs)`.
- `mockPlayer` (`pkg/script/handlers_test.go`): widen recorded fields parallel to the struct change; update existing assertions in `TestSetTimerCapturesArgs` and `TestSoftTimerSetsSoftType` to compare against the new shape.

**Plan-author premise verification (per `controller_preflight`)**: re-grep all `playerTimer.IntArg`, `(*Player).SetTimer(`, and `ActivePlayer.SetTimer(` call sites at HEAD before plan freeze. Implementer re-greps post-commit per `enumerate_all_sites`. Expected full inventory (subject to plan-author re-verification with line numbers):

- `modules/world/player.go:40` (struct field)
- `modules/world/player_timer.go:6, 14, 19` (method signature + struct literal)
- `modules/world/tick.go:292` (consumer)
- `pkg/script/handlers_timer.go:16` (handler caller)
- `pkg/script/active.go:234` (interface decl — confirmed at brainstorm)
- `pkg/script/handlers_timer_test.go:28` (test pin)
- `pkg/script/runner_test.go:176-177` mockPlayer SetTimer recorder (`lastSetTimer struct{ scriptID uint32; interval, intArg int; ttype PlayerTimerType }` + `setTimerCalls int`) — confirmed at brainstorm

**Acceptance criteria**:

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` passes.
2. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` passes.
3. `git grep -E "IntArg\b" modules/world/player.go modules/world/player_timer.go pkg/script/handlers_timer.go` returns no results from the `playerTimer` field (NPC queue's `IntArg` at `pkg/script/queue.go:45` is unrelated and out-of-scope).
4. `popScriptArgs` is **not** introduced anywhere new in Bundle 1 — that is Bundle 2's work.

### Bundle 2 — Timer family TS-faithfulness audit (~140-200 LOC)

**Goal**: Per-handler line-by-line audit of all 5 timer handlers vs `PlayerOps.ts:820-864`. Activate `popScriptArgs` on SETTIMER/SOFTTIMER. Activate script-missing error on SETTIMER/SOFTTIMER/GETTIMER. Flip `(*Player).GetTimer` semantic. Migrate inline player-active gates to `requireActivePlayer`. Remove dead `int(s.PopInt())` casts.

**TS reference matrix** (read at brainstorm; plan author re-verifies):

| TS opcode | TS lines | Pop order (top→bottom) | Script lookup? | Calls |
|---|---|---|---|---|
| SOFTTIMER | 815-827 | `popScriptArgs`, `interval = popInt`, `timerId = popInt` | yes (throws if nil) | `setTimer(SOFT, script, args, interval)` |
| CLEARSOFTTIMER | 829-831 | `popInt` | no | `clearTimer(timerId)` |
| SETTIMER | 833-843 | `popScriptArgs`, `interval = popInt`, `timerId = popInt` | yes (throws if nil) | `setTimer(NORMAL, script, args, interval)` |
| CLEARTIMER | 845-847 | `popInt` | no | `clearTimer(timerId)` |
| GETTIMER | 849-864 | `timerId = popInt` | yes (throws if nil) | iterate `timers.values()`; `pushInt(timer.clock)` if found else `pushInt(-1)` |

**Per-handler diff plan**:

1. **`handleSetTimer` / `handleSoftTimer` (`enqueueTimer` shared body)**:
   - Replace inline gate with `if err := requireActivePlayer(s, op); err != nil { return err }`.
   - Pop order: `intArgs, stringArgs := popScriptArgs(s)` FIRST (top of stack per TS — `popScriptArgs` defined at `pkg/script/handlers.go:630`), then `interval := s.PopInt()`, then `scriptID := uint32(s.PopInt())`.
   - Script-missing check: **propagated via the entity-layer return**, matching the NAI-26 queue family pattern at `pkg/script/handlers.go:670-746`. The handler simply returns `s.Self.SetTimer(scriptID, interval, intArgs, stringArgs, ttype)`. The check itself lives inside `(*Player).SetTimer`, which now does `Provider.GetByID(scriptID)` and returns `fmt.Errorf("SETTIMER: unable to find timer script: %d", scriptID)` (or the SOFTTIMER variant — pass the op name through, mirroring how `EnqueueScriptArgs` does it). Plan author confirms `*Player` reaches the provider via `p.client.server.scriptProvider` (the same chain `EnqueueScriptArgs` uses).

2. **`handleClearTimer` / `handleClearSoftTimer`**:
   - Migrate inline gate to `requireActivePlayer`.
   - Remove dead `int()` cast.
   - Otherwise unchanged (TS does no script lookup).

3. **`handleGetTimer`**:
   - Migrate inline gate to `requireActivePlayer`.
   - **Script-missing check is handler-side here**, NOT entity-side, because `(*Player).GetTimer` returns a plain `int` (-1 sentinel) and the queue-family entity-layer pattern doesn't fit a value-returning method. Use the explicit handler-side check pattern visible at `pkg/script/handlers.go:546-549` (GOSUB_WITH_PARAMS): `if s.Provider.GetByID(scriptID) == nil { return fmt.Errorf("GETTIMER: unable to find timer script: %d", scriptID) }`.
   - Body unchanged: `s.PushInt(s.Self.GetTimer(scriptID))`.

4. **`(*Player).GetTimer` (entity layer, `modules/world/player_timer.go:33-46`) — semantic flip**:
   - Old body: computes `(t.Clock + t.Interval) - now`.
   - New body: `return t.Clock`.
   - The `now` lookup (`p.client.server.currentTick`) becomes dead code — remove the surrounding `if p.client != nil && p.client.server != nil` block.
   - The `-1`-on-not-found path is preserved (matches TS `pushInt(-1)` fallthrough).

**Test surface (Bundle 2)**:

- **`TestSetTimerCapturesArgs` (existing, `handlers_timer_test.go:5-31`)**: restore real assertion. Build bytecode that pushes typed args (e.g. intArgs=[42, 7], stringArgs=["foo"], type-tag="iis"), assert mockPlayer captures the slices via the widened recorder.
- **New: `TestSoftTimerCapturesArgs`**: parallel for SOFTTIMER.
- **New: `TestSetTimerScriptMissing`**: push a scriptID the test provider returns nil for; assert handler returns the exact error string.
- **New: `TestSoftTimerScriptMissing`**: same for SOFTTIMER.
- **New: `TestGetTimerScriptMissing`**: same for GETTIMER.
- **New file: `modules/world/player_timer_test.go`**:
  - `TestPlayerGetTimerReturnsClock`: set a timer with Interval=10 at currentTick=10, advance currentTick to 25, call `p.GetTimer(scriptID)`, assert returned value is `10` (the absolute clock — not `-15`, not `-5+30`).
  - `TestPlayerGetTimerNotFoundReturnsMinusOne`: query an unset scriptID, assert `-1`.
- **`TestGetTimer` (handlers_timer_test.go:74-92)**: needs a one-line **setup change** — the test must register a script with the test provider so the new handler-side script-missing check passes through to the existing `s.Self.GetTimer` call. The passthrough mechanic (mockPlayer.getTimerValue → state.PopInt) is preserved; mockPlayer.getTimerValue is now interpreted as "the absolute clock the entity returned". Update the docstring to clarify the new semantic.
- **`TestTimerOpsRequireActivePlayer` (handlers_timer_test.go:94-113)**: stays valid; helper migration preserves the error path.

**Acceptance criteria**:

1. All Bundle 1 + Bundle 2 tests pass.
2. `git grep "Pointers&PtrActivePlayer" pkg/script/handlers_timer.go` returns nothing.
3. `git grep "int(s.PopInt())" pkg/script/handlers_timer.go` returns nothing.
4. `git grep -E "Clock \+ .*Interval" modules/world/player_timer.go` returns nothing.
5. New file `modules/world/player_timer_test.go` exists with the two new tests.
6. `(*Player).SetTimer` returns `error` and `script.ActivePlayer.SetTimer` interface declares `error` return.
7. `git grep -E "popScriptArgs.*SETTIMER|SETTIMER.*popScriptArgs|popScriptArgs\(s\)" pkg/script/handlers_timer.go` shows `popScriptArgs` is wired into both SETTIMER and SOFTTIMER paths.

### Bundle 3 — Player VARARG opcodes + NPC queue audit memo (~140-200 LOC + 0-LOC audit)

**Goal**: Add 4 new player VARARG opcodes (STRONGQUEUEVARARG, WEAKQUEUEVARARG, QUEUEVARARG, LONGQUEUEVARARG) with handlers mirroring `PlayerOps.ts:110-192` line-by-line. Wire opcode constants, dispatch entries, and `String()` arms. Bundle commit body documents the NPC queue audit (no production diff expected).

**Opcode allocation**: Plan author selects 4 unused IDs by reading `pkg/script/opcode.go` for free slots in the 21XX–22XX range adjacent to the existing fixed-arg siblings (`OpLongQueue=2059`, `OpQueue=2092`, `OpStrongQueue=2117`, `OpWeakQueue=2129`). Required: each new constant in the same numeric block as its fixed-arg sibling; pre-flight grep at plan-freeze to confirm the chosen IDs collide with nothing.

**Per-handler shape (mirror PlayerOps.ts:110-192 line-by-line; brainstorm-time read)**:

| Handler | TS lines | Pop order (top→bottom) | NumberNotNull on delay? | Calls |
|---|---|---|---|---|
| `handleStrongQueueVararg` | 110-120 | `popScriptArgs`, `delay = popInt`, `scriptId = popInt` | **No** | `EnqueueScriptArgs(scriptID, delay, intArgs, stringArgs, QueueStrong)` |
| `handleWeakQueueVararg` | 134-144 | same as STRONG | **No** | same with `QueueWeak` |
| `handleQueueVararg` | 159-169 | same as STRONG | **No** | same with `QueueNormal` |
| `handleLongQueueVararg` | 182-192 | `popScriptArgs`, `logoutAction = popInt`, `delay = popInt`, `scriptId = popInt` (popInts(3) destructured as `[scriptId, delay, logoutAction]` in TS — top of stack is `logoutAction`) | **No** | `EnqueueScriptArgs(scriptID, delay, append([]int{logoutAction}, intArgs...), stringArgs, QueueLong)` (TS line 191 passes `[logoutAction, ...args]` — logoutAction prepended to popScriptArgs ints; stringArgs from popScriptArgs pass through unchanged) |

**Discipline reminder per memory `vararg_opcode_shapes_dont_share_with_fixed_arg_siblings`**: LONGQUEUEVARARG diverges from its three siblings (extra `logoutAction` popInt + prepended args). Do NOT factor a shared helper across the four handlers; each gets its own body. Plan author re-reads the 4 TS handler bodies independently before plan freeze and confirms the table above.

**Plan-author premise verification (per `controller_preflight` + `vararg_opcode_shapes_dont_share_with_fixed_arg_siblings`)**: For each VARARG handler, plan author MUST read the exact TS lines and re-derive the pop order, the script-missing check pattern, the NumberNotNull placement, and the call signature **independently for each handler**. Do NOT shotgun a shared helper across the four. NAI-26 Bundle 2 caught LONGQUEUE diverging from its siblings exactly here.

**Wiring**:

- 4 new `Op*` constants in `pkg/script/opcode.go` (same numeric block as fixed-arg siblings).
- 4 new `String()` arms in the same file.
- 4 new dispatch-table entries in `pkg/script/handlers.go`.
- Handler bodies in **new file** `pkg/script/handlers_player_vararg.go` (separation from existing 770-LOC `handlers_player.go` per design-for-isolation).

**Test surface (Bundle 3)**:

For each of the 4 handlers (in new file `pkg/script/handlers_player_vararg_test.go`):

- **`TestX_RoundTrip`**: push args + delay + scriptId (+ logoutAction for LONGQUEUEVARARG), assert mockPlayer.EnqueueScriptArgs captured the correct queue type, delay, intArgs (with logoutAction prepended for LONGQUEUEVARARG), stringArgs.
- **`TestX_ScriptMissing`**: push unknown scriptId, assert script-missing error (propagated from mockPlayer.EnqueueScriptArgs return).
- **`TestX_RequireActivePlayer`**: assert error path with no active player; OR add the 4 opcodes to a parameterized table-test parallel to `TestTimerOpsRequireActivePlayer`.
- **`TestX_AcceptsNullDelay` (all 4 variants)**: per memory `ts_asymmetry_dual_pin` — none of the VARARG variants check NumberNotNull on delay (only the fixed-arg STRONGQUEUE does). Push delay = null sentinel for each of the 4, assert it does NOT error and does enqueue with `delay=<sentinel>`. These tests act as escalation triggers if upstream TS adds NumberNotNull to a VARARG variant later.
- **Specifically for `handleLongQueueVararg`**: an extra `TestLongQueueVararg_LogoutActionPrepended` test asserts that with `popScriptArgs` returning `intArgs=[1,2,3]` and `logoutAction=99`, the captured args are `[99,1,2,3]` not `[1,2,3,99]` and not `[99]`.

**NPC queue audit (Family 3, close-commit memo)**:

Re-read `handleNpcQueue` (`pkg/script/handlers_npc.go:316-332`) line-by-line vs TS `NpcOps.ts:144-150`. Expected outcome: **no behavioral changes** (NAI-3 + NAI-20 already attended to this handler; the brainstorm-time read found nothing new). The audit is performed at plan-author and implementer phases; if no divergences are found, the NAI-27 close commit body documents the audit (file path, TS line range, "no divergence found, audit memo only") with no production diff. If divergences are found, they are folded into Bundle 3 (if small) or extracted as a new audit-followups entry.

**Acceptance criteria**:

1. All Bundle 1 + 2 + 3 tests pass.
2. `git grep -E "OpStrongQueueVararg|OpWeakQueueVararg|OpQueueVararg|OpLongQueueVararg" pkg/script/` returns the 4 new constants in `opcode.go`, the 4 dispatch entries in `handlers.go`, the 4 `String()` arms, and the 4 handlers in `handlers_player_vararg.go`.
3. The NPC queue audit narrative appears in the close commit body with explicit file/TS-line reference.

### Polish + close

**Polish commit absorbs** (per NAI-26 mirror cadence — see `351cc2f` for shape):

- Any review minors flagged during the per-bundle code-review step.
- Any drive-by stale-narrative comment fixes the audits surface (e.g. handler docstrings that referenced "single intArg" need to reflect the parallel-slice shape).

**Close commit** (per memory `close_commit_memory_trailer`):

- Update `nai_followups.md`:
  - Mark NAI-26 Out-of-scope #3 (NPC queue audit) Resolved with audit-memo provenance.
  - Mark NAI-26 Out-of-scope #4 (player timer family) Resolved with NAI-27 commit shas.
  - New NAI-27 entry summarizing all four families' resolutions and the GETTIMER divergence retirement.
- `Closes memory:` git trailer pointing at the new entry.
- Re-derive NAI-26's deviation count (14) and confirm NAI-27 closes 0 net deviations (the GETTIMER divergence was untracked, not a numbered deviation).

## Out-of-scope (explicitly deferred)

1. **NAI-19-D1 zone state during NPC respawn** — biggest open structural item. Requires Zone abstraction infrastructure design first; standalone sub-spec.
2. **NAI-11 deferred items** — SMART pathfinding, reach helpers, focus() instant flag. Each warrants its own brainstorm and sub-spec.
3. **NPC speedup-quirk test strengthening** — already resolved in NAI-21 Bundle 3 per `nai_followups.md:92` (`TestNpcTurnReentryQueueAppendDuringIteration`). No further work.
4. **NPC timer family widening** — NPC timer in TS has no variadic args (NPC timer is a single per-NPC interval); no widening required.
5. **NPC vararg opcodes** — TS has none; nothing to port.
6. **Any opcode outside the timer / player-queue / NPC-queue families.**
7. **`processTimers` execution-order audit** — TS `Player.ts:925-941` semantics (`World.currentTick >= timer.clock + timer.interval` and post-fire `clock = World.currentTick` reset). Goscape's `processTimers`-equivalent in `tick.go` is presumed faithful per pre-existing tests; auditing it is out-of-scope unless Bundle 2's GETTIMER fix surfaces a related divergence. If surfaced, extract as a new audit-followups entry.

## Test strategy

| Bundle | Focus | Test files touched | New test funcs (rough) |
|---|---|---|---|
| 1 | Plumbing | `handlers_timer_test.go`, `runner_test.go` (mockPlayer recorder widening) | 0 new |
| 2 | Timer family audit | `handlers_timer_test.go`, `modules/world/player_timer_test.go` (new file) | ~7 new |
| 3 | VARARG opcodes + NPC queue audit | `pkg/script/handlers_player_vararg_test.go` (new file) | ~17 new |

Per memory `plan_test_coverage_crosscheck`: plan author cross-checks each task's prescribed test list against this table at plan-freeze. Per memory `plan_runnable_test_fixtures`: every plan-codified bytecode fixture is mentally executed (operand-count matches push count, instruction count matches `Opcodes` len, `LookupKey` is correct). Per memory `match_spec_tests_to_library_capability`: Bundle 2's script-missing tests must use whatever `ScriptState.Provider` actually exposes — plan author confirms by reading `pkg/script/state.go` at plan-freeze.

## Risks

1. **GETTIMER consumer scripts may rely on the divergent "remaining ticks" semantic.** Per `true_to_ts_gate`, TS is canonical; if user-facing scripts break, they are subject to follow-up. Verify by greping for any test or production code that depends on a specific GETTIMER return-value range — there should be none, since GETTIMER is a pure-passthrough opcode at the engine layer with mockPlayer-driven tests only.
2. **`popScriptArgs` infrastructure assumes a fully-populated string stack.** Bundle 2 must ensure timer-set test fixtures push the type-tag string + typed args correctly. Per memory `plan_runnable_test_fixtures`.
3. **VARARG opcode ID collisions.** Plan author owns ID assignment; pre-flight grep for the chosen IDs to ensure they are free.
4. **`handlers_player.go` file size.** Adding 4 handlers in `handlers_player_vararg.go` instead of growing the existing 770-LOC file. Per design-for-isolation.
5. **Bundle ordering risk for `nil, nil` placeholder in Bundle 1.** Bundle 2's failure mode "we forgot to swap nil for popScriptArgs" is caught by the TestX_CapturesArgs assertions added in Bundle 2 — they will fail until popScriptArgs is wired. Acceptable given the NAI-26 mirror cadence has shipped this pattern cleanly.

## Deviations

NAI-27 introduces **0 new deviation tags**. It closes one untracked divergence (GETTIMER semantic) by porting TS behavior verbatim. NAI-26's deviation count of 14 stays at 14, unless plan-author or implementer audit surfaces an existing deviation that is now obsolete (decrement and document in close commit).

## Memory entries authored at close

1. **New entry in `nai_followups.md`** for NAI-27 close (resolution memo for all four families).
2. **Possible new memory entry** on the GETTIMER pattern (untracked semantic divergences in passthrough opcodes — audit lesson, parallel to existing audit memories).
3. **Conditional update** to `vararg_opcode_shapes_dont_share_with_fixed_arg_siblings` if Bundle 3 surfaces additional asymmetries beyond what NAI-26 caught.
