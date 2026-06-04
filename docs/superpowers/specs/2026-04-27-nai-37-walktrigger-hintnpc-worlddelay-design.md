# NAI-37 — NPC_WALKTRIGGER + HINT_NPC + WORLD_DELAY (with full world-script-queue infrastructure)

## Motivation

Three script-VM opcodes are declared in `pkg/script/opcode.go` but have no handler registered in `pkg/script/handlers.go` at HEAD `22064f9` (NAI-36 close). All three surface as `runner.go:71` runtime errors during the post-NAI-36 smoke run (`no handler for X (opcode N)`). User goal at NAI-37: silence the smoke log noise.

Smoke-surfaced stubs:

- **NPC_WALKTRIGGER** (opcode 2545, declared at `pkg/script/opcode.go:282`) — pops 2 ints `(queueID, arg)`, validates `queueID ∈ [1, 20]`, writes `walktrigger = queueID-1` and `walktriggerArg = arg` on the active NPC. TS `NpcOps.ts:483-490`.
- **HINT_NPC** (opcode 2028, declared at `pkg/script/opcode.go:128`) — sends a HintArrow server packet (type=1, the NPC-hint variant) to the active player's connection, encoding the active NPC's nid. TS `PlayerOps.ts:972-974` + `HintArrow.ts` + `HintArrowEncoder.ts`.
- **WORLD_DELAY** (opcode 1021, declared at `pkg/script/opcode.go:95`) — suspends the active script to the world-script queue with a popped wakeup-tick count. TS `ServerOps.ts:166-169`.

The first two are mechanical handler ports; **WORLD_DELAY is foundational infrastructure**. The `WorldSuspended` execution state has been declared in `pkg/script/execution.go:16` since NAI-S1 (~30 sub-specs ago) with a "future sub-specs" marker at `modules/world/script.go:59` and `modules/world/npc_script.go:309`. No producer or consumer exists at HEAD: a script-side handler that sets `Execution = WorldSuspended` would land in the default branch of `resumeOrFinish` and be silently discarded with a warn.

A faithful WORLD_DELAY port requires three interlocking pieces (TS `World.ts:530-559` + `Player.ts:2135-2136` + `Npc.ts:219-220`):

1. A `worldScriptQueue` on `*Server` (mirror of TS `World.queue`) holding script states awaiting their wakeup tick.
2. A tick-step `processWorldQueue()` (mirror of the world-queue loop in TS `World.processWorld`) that decrements per-entry delays, fires ready entries, and routes post-execute states.
3. Producer wiring at three call sites: player-bound `resumeOrFinish` (script.go), npc-bound resume function (npc_script.go), and `processWorldQueue` itself (the self-loop case from a script that re-suspends).

Pre-NAI-37 behavior: 3 opcodes abort with `Aborted` execution state; logs spam `runner.go:71`. `WorldSuspended` is declared but inert — neither produced nor consumed.

Post-NAI-37 behavior: all 3 opcodes execute TS-faithfully; `WorldSuspended` is fully wired (3 producer sites, 1 consumer); the world-script-queue infrastructure is reusable by any future world-suspending opcode (none currently planned, but the runescript opcode space contains adjacent shapes).

## Tech stack

- **Go 1.26+** (per `go_version.md`; use modern Go syntax via the `use-modern-go` skill).
- TS source: `Engine-TS` only per `ts_source_canonical_path.md`.
  - `src/engine/script/handlers/NpcOps.ts:483-490` (NPC_WALKTRIGGER)
  - `src/engine/script/handlers/PlayerOps.ts:972-974` (HINT_NPC)
  - `src/engine/script/handlers/ServerOps.ts:166-169` (WORLD_DELAY handler)
  - `src/engine/script/ScriptValidators.ts:114` (`QueueValid` range [0, 19], inline-validated as [1, 20] before `queueID-1` transform)
  - `src/engine/entity/Player.ts:2174-2176` (`hintNpc` method; writes `HintArrow(1, nid, 0, 0, 0, 0)`)
  - `src/network/game/server/model/HintArrow.ts` + `src/network/game/server/codec/HintArrowEncoder.ts` (packet shape: type=1 branch is `p1(type), p2(nid), p2(0), p1(0)` = 6 bytes)
  - `src/network/game/server/ServerGameProt.ts:56` (`HINT_ARROW = (25, 6)`)
  - `src/engine/World.ts:530-559` (`processWorld` world-queue iteration)
  - `src/engine/World.ts:1238` (`enqueueScript` API)
  - `src/engine/entity/Player.ts:2125-2151` (`executeScript` — player-path WorldSuspended producer)
  - `src/engine/entity/Npc.ts:216-238` (`executeScript` — npc-path WorldSuspended producer)
- Existing infrastructure (verified at HEAD `22064f9`):
  - `Execution` enum at `pkg/script/execution.go:8-17` already declares `WorldSuspended`. No enum churn.
  - `(s *Server).processPlayerQueue` at `modules/world/tick.go:226-249` — the slice-with-mid-pass-visibility queue pattern this sub-spec mirrors.
  - `(s *Server).resumeOrFinish` at `modules/world/script.go:46-64` — extension target for the player-path producer.
  - Symmetric npc-path resume function at `modules/world/npc_script.go:295-320` (approximate; plan-author confirms exact shape).
  - `requireActiveNpc(s, opName)` at `pkg/script/handlers_npc.go:72`, `requireActivePlayer(s, opName)` at `pkg/script/handlers_player.go` (per `plan_grep_helper_patterns.md` — reuse, don't reinvent).
  - Existing inline-range-check pattern at `handleNpcQueue` (`handlers_npc.go:339-341`) for `queueID ∈ [1, 20]`. Mirror for NPC_WALKTRIGGER, do NOT extract a `checkQueueID` helper (would be a single-call-site helper).
  - `(p *Player).writeOut(op gameserver.Op, payload []byte)` pattern as used by `(p *Player).CamReset` at `modules/world/player_script.go:143-147`. Mirror for `HintNpc`.
  - `mockNpc` and `mockPlayer` recorders in `pkg/script/runner_test.go` — extend with new fields per `mock_recorder_field_naming_check.md`.
- Validators are inline range checks (NOT exported `XxxValid` predicates per goscape convention).
- TS source canonical path: per `ts_source_canonical_path.md`.

## Scope

**In scope:**

1. **B1 — NPC_WALKTRIGGER (opcode 2545).**
   - Add `walktrigger int` and `walktriggerArg int` fields to `*Npc` at `modules/world/npc.go`. Default `walktrigger = -1` (unset sentinel; queue-index 0 is a valid post-transform value, so 0 cannot be the unset marker).
   - Audit ALL `Npc{` struct literals and constructor sites in modules/world/ and tests; add explicit `walktrigger: -1` initialization at every literal site OR ensure all paths flow through a constructor that sets the default. Per `plan_enumerate_struct_literals.md`.
   - Add `SetWalkTrigger(queueID int)` and `SetWalkTriggerArg(arg int)` methods to the `ActiveNpc` interface in `pkg/script/active.go`.
   - Implement the two methods on `*Npc` at `modules/world/npc_script.go` as direct field writes (no validation; handler validates).
   - Extend `mockNpc` in `pkg/script/runner_test.go` with `walkTriggerCalls []int` and `walkTriggerArgCalls []int` recorder fields.
   - `handleNpcWalkTrigger` in `pkg/script/handlers_npc.go`: `requireActiveNpc("NPC_WALKTRIGGER")` + `arg := s.PopInt()` + `queueID := s.PopInt()` + inline `if queueID < 1 || queueID > 20 { return error }` + `s.ActiveNpc.SetWalkTrigger(queueID-1)` + `s.ActiveNpc.SetWalkTriggerArg(arg)`.
   - Register `OpNpcWalkTrigger: handleNpcWalkTrigger` in `pkg/script/handlers.go`.

2. **B2 — HINT_NPC (opcode 2028).**
   - Add `OpHintArrow = Op{Opcode: 25, PayloadSize: 6}` to `pkg/io/protocol/game/server/prot.go`.
   - Add `HintNpc(nid int)` to the `ActivePlayer` interface in `pkg/script/active.go`.
   - Implement `(p *Player).HintNpc(nid int)` at `modules/world/player_script.go`. Encode 6 bytes `[0x01, p2(nid), p2(0), p1(0)]` into a `[]byte`, call `p.writeOut(gameserver.OpHintArrow, payload)`. Mirror only the type=1 branch of TS HintArrowEncoder; do NOT add type-branching (other branches deferred).
   - Extend `mockPlayer` in `pkg/script/runner_test.go` with `hintNpcCalls []int` recorder field.
   - `handleHintNpc` in `pkg/script/handlers_player.go`: `requireActivePlayer("HINT_NPC")` + `requireActiveNpc("HINT_NPC")` + `s.Self.HintNpc(s.ActiveNpc.NID())`.
   - Register `OpHintNpc: handleHintNpc` in `pkg/script/handlers.go`.

3. **B3 — WORLD_DELAY (opcode 1021) full infrastructure.**

   3a. **Queue + scheduler infrastructure** (`modules/world/world_script_queue.go`, new file):
   - `type worldScriptQueueEntry struct { script *script.ScriptState; delay int }`.
   - `s.worldScriptQueue []worldScriptQueueEntry` field on `*Server` (declared in server.go or wherever existing per-server queue fields live; plan-author co-locates).
   - `(s *Server) EnqueueWorldScript(state *script.ScriptState, delay int)` — appends an entry. No mutex (tick is single-threaded; verify against existing per-server queue access patterns at plan-author time).

   3b. **Tick-loop integration** (`modules/world/tick.go`):
   - `(s *Server) processWorldQueue()` — slice-with-mid-pass-visibility iteration mirroring `processPlayerQueue` lines 226-249. Decrement delay first, skip if `delay > 0`, remove entry from slice BEFORE firing (prevents re-entrant index collision), call `script.Execute`, dispatch via `resumeOrFinishWorld(state)`. Don't advance `i` after removal — the loop re-reads `len(s.worldScriptQueue)`.
   - Wire `s.processWorldQueue()` into the tick loop. Placement: at tick start, **before** `processNpcEventQueue` (currently at tick.go:39). Matches TS `World.processWorld` ordering (called before per-player and per-npc loops).

   3c. **Player-path producer** (`modules/world/script.go`):
   - Extend `resumeOrFinish` at lines 53-63 with an explicit `case script.WorldSuspended:` arm: `delay := state.PopInt(); s.EnqueueWorldScript(state, delay); self.ClearActiveScript()`. Place BEFORE the default branch.
   - Update the default-branch comment to remove the WorldSuspended half. NpcSuspended remains in the comment (still future-sub-spec).

   3d. **Npc-path producer** (`modules/world/npc_script.go`):
   - Locate the symmetric resume function (around line 295-320). Add a parallel `case script.WorldSuspended:` arm that pops the delay, enqueues to the world queue, and clears the npc's active script.

   3e. **Consumer-side dispatch** (`modules/world/script.go`):
   - New helper `(s *Server) resumeOrFinishWorld(state *script.ScriptState)` — called from `processWorldQueue` after `script.Execute`. Switch on `state.Execution`:
     - `Finished`, `Aborted`: drop entry, no log (Aborted may already be logged at script.go:48-49 level).
     - `WorldSuspended`: pop the delay, re-enqueue. Self-loop case (path P3).
     - `Suspended`, `NpcSuspended`, `PauseButton`, `CountDialog`: warn `"world-queue script transitioned to <state>; cross-context resume unsupported"` and drop. Tracked deviation `NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP`.
     - `Running`: warn loudly (shouldn't happen) and drop.

   3f. **Handler** (`pkg/script/handlers_server.go` — confirm file existence at plan-write; if absent, place in the most appropriate sibling):
   - `handleWorldDelay(s *ScriptState) error` — TS-faithful one-liner: `s.Execution = WorldSuspended; return nil`. Does NOT pop the int (suspending side pops at suspend time, not handler time).
   - Register `OpWorldDelay: handleWorldDelay` in `pkg/script/handlers.go`.

**Out of scope:**

- AI-tick walktrigger consumption (the npc-movement code that reads `walktrigger`/`walktriggerArg` and fires the queued trigger script when the NPC completes a walk step) — tracked deviation `NAI-37-D-WALKTRIGGER-NOREADER`.
- HINT_COORD (opcode 2027, goscape's name for TS HINT_TILE), HINT_PL (opcode 2029), HINT_STOP (opcode 2030, goscape's name for TS STOPHINT) handlers and their type=2-6/10/-1 HintArrow encoder branches — tracked deviation `NAI-37-D-HINTARROW-PARTIAL-ENCODER`.
- Cross-context (player-bound or npc-bound) resume semantics for scripts that, while running from the world queue, transition to `Suspended`/`NpcSuspended`/`PauseButton`/`CountDialog` — tracked deviation `NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP`.
- Tick-wide panic recovery for `script.Execute` panics from world-queue iteration (TS uses try/catch at World.ts:557-559) — tracked deviation `NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY`. Closure when goscape adopts a project-wide tick panic-recovery convention.
- WORLD_DELAY use from contexts other than the three TS producer sites (e.g., timer-fired scripts) — current scope covers the three TS producer paths verbatim. If goscape has additional active-script entry points not present in TS, plan-author flags them at preflight.

**Closes:**

- Smoke log noise for the three opcodes.
- Implicit "WorldSuspended declared but no producer" stub (long-staged since NAI-S1).
- Implicit "WorldSuspended declared but no consumer" stub (same provenance).
- Future world-suspending opcodes are unblocked: the queue + scheduler infrastructure is reusable.

## Architecture

### Three-bundle structure

NAI-37 decomposes into three bundles with NO inter-bundle dependencies. Plan-author may execute in any order or in parallel:

- **B1: NPC_WALKTRIGGER** — state-only port; `*Npc` field writes via ActiveNpc setters.
- **B2: HINT_NPC** — fire-and-forget I/O port; new server packet + Player method.
- **B3: WORLD_DELAY** — state-machine transition + multi-tick coordination + new world-script-queue infrastructure.

The interface boundary keeps the three slices independent: B1 and B2 share neither state nor wire format with B3, and B1 and B2 don't touch each other.

### File layout

```
pkg/io/protocol/game/server/prot.go          (B2: +OpHintArrow declaration)
pkg/script/active.go                         (B1, B2: +interface methods)
pkg/script/handlers.go                       (B1, B2, B3: +3 registrations)
pkg/script/handlers_npc.go                   (B1: +handleNpcWalkTrigger)
pkg/script/handlers_player.go                (B2: +handleHintNpc)
pkg/script/handlers_server.go                (B3: +handleWorldDelay; new file if needed)
pkg/script/handlers_npc_test.go              (B1: +5 handler tests)
pkg/script/handlers_player_test.go           (B2: +3 handler tests)
pkg/script/handlers_server_test.go           (B3: +1 handler test)
pkg/script/runner_test.go                    (B1, B2: +mock recorder fields)

modules/world/npc.go                         (B1: +walktrigger/-Arg fields, +sentinel default)
modules/world/npc_script.go                  (B1: +setter methods)
                                             (B3: +WorldSuspended producer arm)
modules/world/npc_script_test.go             (B1: +1 entity test)
modules/world/player_script.go               (B2: +HintNpc method)
modules/world/player_script_test.go          (B2: +1 byte-pin test)
modules/world/script.go                      (B3: +WorldSuspended producer arm
                                                  +resumeOrFinishWorld helper)
modules/world/tick.go                        (B3: +processWorldQueue + tick-step wiring)
modules/world/world_script_queue.go          (B3: NEW FILE — queue type +
                                                  EnqueueWorldScript + queue field on Server)
modules/world/world_script_queue_test.go     (B3: NEW FILE — 6 scheduler tests
                                                  + 3 producer-path tests
                                                  + 1 integration test)
```

### Test layout (~21 new tests)

- **B1 — 6 tests**: 5 handler-side (no-active-npc, queueID below/above range, pop-order + queueID-1 transform, boundary IDs 1+20) + 1 entity-side (field-write round-trip).
- **B2 — 4 tests**: 3 handler-side (no-active-player, no-active-npc, success records nid) + 1 entity-side byte-pin (`HintNpc(0x1234)` produces `[0x01, 0x12, 0x34, 0x00, 0x00, 0x00]`).
- **B3 — 11 tests**: 1 handler-side (sets WorldSuspended, doesn't pop) + 6 scheduler (delay=0, delay=N, remove-before-fire, FIFO order, re-entrant enqueue, self-loop) + 3 producer-path (player WorldSuspended → enqueue+clear, npc WorldSuspended → enqueue+clear, resumeOrFinishWorld dispatch table) + 1 full round-trip integration (player script with delay=2 fires at tick 4).

## Test strategy

Per `plan_test_coverage_crosscheck.md`, every test case enumerated in this section MUST appear in the corresponding plan task's code block. Plan-author cross-checks before dispatch.

Per `gettimer_passthrough_opcode_semantic_audit.md`, handler tests against mocks pass values through unchanged. The B3 integration test pins the actual cross-tick coordination — handler tests alone are not sufficient for WORLD_DELAY.

Per `rsbuf_roundtrip_tests.md`, byte-pin tests pin **each field at its byte position**. Pick a HINT_NPC test nid value (`0x1234`) where every byte is distinguishable from the zero-fill, distinguishable from the type byte, and distinguishable byte-order-wise (BE vs LE).

Per `plan_runnable_test_fixtures.md`, the B3 integration-test bytecode MUST be mentally compilable against `runner_test.go`'s ScriptFile-construction patterns. Plan-author confirms the opcodes used (`pushInt`, `WORLD_DELAY`, arithmetic, `RETURN`) are all supported by the test infrastructure; if any aren't, the integration test is reframed to use only supported opcodes.

Per `test_passes_for_wrong_reason.md`, the B3 scheduler tests must verify the **specific branch** they claim to exercise. The "remove-before-fire" test uses an Execute stub that inspects `len(s.worldScriptQueue) == 0` from inside the call; the "re-entrant enqueue" test verifies both A and B fired on the same tick (not just that B ran eventually).

## Expected deviations

NAI-37 introduces 4 new tracked deviations and retires 2 implicit-untracked stubs.

| Tag | Site | Closure |
|---|---|---|
| **NAI-37-D-WALKTRIGGER-NOREADER** | `modules/world/npc.go` walktrigger fields; `pkg/script/handlers_npc.go` handleNpcWalkTrigger | Port AI-tick walktrigger consumption from TS Npc.ts (future sub-spec). |
| **NAI-37-D-HINTARROW-PARTIAL-ENCODER** | `modules/world/player_script.go` HintNpc method | Port HINT_PL / HINT_COORD / HINT_STOP handlers and their encoder branches. |
| **NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP** | `modules/world/script.go` resumeOrFinishWorld | Cross-context resume semantics — likely tied to broader player-script-lifecycle alignment. |
| **NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY** | `modules/world/tick.go` processWorldQueue | Project-wide tick panic-recovery convention (separate sub-spec). |

**Implicit stubs retired** (no prior tracker entries; called out for provenance per `audit_arithmetic_correction_in_rollup.md`):

- `WorldSuspended` execution constant declared at `pkg/script/execution.go:16` since NAI-S1 with no producer. Activated by `handleWorldDelay`.
- `WorldSuspended` consumer absent at `modules/world/script.go:58-62` (default-branch warn). Activated by `resumeOrFinishWorld` and `processWorldQueue`.

**Net deviation tally:** 15 (post-NAI-36) + 4 (NAI-37 explicit) - 0 (no prior-tracked closures) = **19**. The 2 implicit retirements are NOT in the tally because they were never tracker-counted — but the close-out commit body must enumerate them explicitly so the provenance is grep-discoverable. Per `audit_arithmetic_correction_in_rollup.md`.

**Non-divergences flagged for the doc-comment** (TS-faithful behavior that future reviewers might mistake as a bug to "fix"):

- Stale `activePlayer` / `activeNpc` references on resume after the entity is removed/despawned. TS World.ts:557-559 try/catch handles the resulting throw; goscape's equivalent path (`resumeOrFinishWorld` warn+drop on `script.Execute` error) is the matching behavior.
- Negative or zero `delay` parameters to `EnqueueWorldScript` produce next-tick fire. TS-faithful (no validation).
- The `popInt`-at-suspend pattern (resumer pops, not handler) is TS-faithful and load-bearing — the script bytecode contract pushes the wakeup-tick before WORLD_DELAY and expects it to be off the stack on resume.

## Cadence

Standard cadence per `compressed_cadence.md` — estimated ~280-420 LOC total, well above the 100-LOC threshold.

- Spec doc: this file (one `docs(spec): ...` commit).
- Plan doc: separate `docs(plan): ...` commit, written by `superpowers:writing-plans`.
- Execution: `superpowers:subagent-driven-development` per `execution_mode_default.md`. Each bundle ships as its own commit (or sequence of task commits within a bundle if the bundle subdivides further during plan-authoring). Two-stage review per task: implementer self-review on commit + Stage 2 reviewer subagent.
- Final whole-impl review covers cross-bundle integration (does the world-script-queue interact correctly with existing per-player/per-npc queues? does the new tick-step ordering introduce subtle ordering bugs in mixed-context scripts?).
- Close-out commit per `close_commit_memory_trailer.md` — `Closes memory:` trailer enumerating any new memory entries seeded by NAI-37 sub-spec experience.

## Sequencing rationale

Three independent bundles with no inter-bundle dependencies. Recommended order:

1. **B1 first** — smallest, lowest-risk; touches only NPC handler and entity files. Establishes confidence in the test-cross-check workflow before B3's heavier infrastructure.
2. **B2 second** — medium-sized; introduces a new server packet but no new tick-loop touch. Independent of B1.
3. **B3 last** — largest and most invasive; touches tick.go, both resume paths, introduces new file. Goes last so any unexpected churn doesn't block the smaller bundles.

If parallelization is preferred: B1 and B2 can run concurrently (no shared touch points), with B3 sequenced after both to keep merge surface clean.

## Risk + mitigations

| Risk | Mitigation |
|---|---|
| **`Npc{}` struct-literal sites silently default `walktrigger=0` (a valid queue index post-transform)**, accidentally activating a walktrigger that never fires | Sentinel `-1` default; plan-author enumerates ALL `Npc{` literals per `plan_enumerate_struct_literals.md`; B1 task includes the literal-site audit explicitly. |
| **HintArrow byte-pin test passes for the wrong reason** (e.g., zero-valued nid that matches zero-fill bytes) | Test nid is `0x1234` — every byte distinguishable. |
| **Mock recorder field names diverge from actual mockNpc/mockPlayer convention** | Plan-author greps `runner_test.go` for existing field naming convention before referencing in code blocks. Per `mock_recorder_field_naming_check.md`. |
| **Integration-test bytecode uses opcodes not supported by `runner_test.go` ScriptFile-construction infrastructure** | Plan-author audits supported opcodes at plan-write time and reframes the integration test to use only supported ones. Per `plan_runnable_test_fixtures.md`. |
| **`processWorldQueue` placement in tick loop creates ordering race with `processNpcEventQueue`** | TS ordering puts world-queue first (World.processWorld is called before per-player/npc loops). Mirror exactly. Plan-author re-reads tick.go entry function and confirms the new step inserts before line 39. |
| **`resumeOrFinish` extension introduces a subtle bug for non-WorldSuspended states** | New `case script.WorldSuspended:` arm placed BEFORE the default branch; existing cases (`Finished`/`Aborted`/`Suspended`/`PauseButton`/`CountDialog`) untouched and re-tested via existing test surface (no test churn for those). |
| **Implementer drift on TS-faithful semantics** (e.g., adds resume-time entity-validity check thinking it's "obvious") | Doc-comments at each TS-faithful non-divergence site explicitly note the divergence-temptation and the TS reasoning. Per `true_to_ts_gate.md`. |
| **Spec test-coverage assertions decay between spec-write and dispatch** | Plan-author cross-checks the 21-case matrix at plan-write time; controller pre-flights at dispatch. Per `plan_test_coverage_crosscheck.md` + `controller_preflight.md`. |

## Smoke handoff

Per `smoke_test_server_handoff.md`, smoke-test server invocation is user-driven. NAI-37 close should:

1. Confirm all three opcode "no handler" warns disappear from a fresh smoke run touching the relevant scripts (player-bound script that hits WORLD_DELAY; npc that calls NPC_WALKTRIGGER; player who triggers a HINT_NPC content path).
2. Confirm no new noise (e.g., the new `world-queue script transitioned to <state>` warn from `resumeOrFinishWorld` doesn't fire under happy-path content — if it does, the cross-context-drop deviation surfaced a real script case that needs scoping).
3. Confirm WORLD_DELAY-using scripts complete their full lifecycle (script Aborts pre-NAI-37; post-NAI-37, scripts that call WORLD_DELAY should run to completion if their logic supports it).

User-driven smoke launch; Claude does not start the smoke server (sandboxed process unreachable from the Java client host).
