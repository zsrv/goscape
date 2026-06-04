# NAI-144 — QueueEngine wiring (predecessor to NAI-145 D2+D3)

**Status:** SPEC
**Date:** 2026-05-09
**Predecessor relation:** NAI-144 is a foundational predecessor sub-spec to NAI-145 (NAI-142-D-R-D2 lastMapZone+triggerMapzone + NAI-142-D-R-D3 triggerZone/triggerZoneExit + SetMultiway). NAI-145 brainstorm/spec begins fresh-session AFTER NAI-144 closes (per memory `superpowers_clear_between_spec_and_impl`, `session_context_management`).

**Tech stack:** Go 1.26+.

---

## §1 Scope

### In scope

1. **`Player.engineQueue []playerQueueRequest`** — new slice field, separate from `p.queue`. TS keeps `engineQueue` and `queue` as distinct `LinkList`s and the movement gate references `engineQueue.head()` independently of `queue.head()`.

2. **Enqueue routing** — `EnqueueScriptFile` (`modules/world/player_script.go:69`) switches on `qtype`: `case script.QueueEngine` appends to `p.engineQueue`; default keeps the existing `p.queue` append path.

3. **`s.processPlayerEngineQueues()`** — new server-level loop in `modules/world/tick.go`, mirroring `processActiveScripts` shape. Per-player drain semantics match TS `Player.processEngineQueue` (`Engine-TS/src/engine/entity/Player.ts:641-651`):
   - Per entry, decrement `Delay`.
   - Fire (and remove) when `p.canAccess() && Delay <= 0`.
   - Otherwise leave in place; advance index.
   - Distinct from `processPlayerQueue` (`tick.go:298`): no `QueueStrong` modal-close pre-pass; gated by `canAccess()` not `!p.delayed`; no STRONG bypass (TS engineQueue has no STRONG concept).

4. **Tick-slot insertion** — call `s.processPlayerEngineQueues()` between `s.processPlayerTimers()` and `s.processPathing()` in `runTickLoopWithRate` (`tick.go:53-54`). Matches TS World.ts:715-728 ordering: `processQueues → processTimers → processEngineQueue → processInteraction`.

5. **Movement gate (TS Player.ts:653-660 `updateMovement` parity)** — `moveClickRequest` is presently set at handler_opheld sites but never read at HEAD. Port the gate at the top of `resolveMovement` (`modules/world/movement.go:46`), AFTER the existing `p.stepsTaken = 0` reset (so the per-tick stepsTaken contract is preserved when gated):

   ```go
   if p.moveClickRequest && p.busy() && (len(p.queue) > 0 || len(p.engineQueue) > 0) {
       return
   }
   ```

   Doc-comment cites TS `Player.ts:657` directly.

6. **`changeStat` migration** — `modules/world/player_script.go:583-589` flips `script.QueueNormal` → `script.QueueEngine`. Closes the S6h tracked deviation (`docs/superpowers/specs/2026-04-21-runescript-s6h-changestat-trigger-design.md:241-242`: "until a consumer behavioral delta appears"). NAI-144 is that trigger.

7. **`pkg/script/queue.go`** — remove `// reserved` comment from `QueueEngine` enum value.

### Out of scope (deferred to NAI-145)

- D2 `lastMapZone` field + transition block + `triggerMapzone(x,z)` + `triggerMapzoneExit(x,z)` dispatch.
- D3 zone-transition block (currently only fires `rebuildZones`; NAI-145 wires the rest), `triggerZone(level,x,z)` + `triggerZoneExit(level,x,z)` dispatch, `SetMultiway` packet (opcode 254, 1-byte `pbool(hidden)`) encoder + emit on multi-flag transition.
- Any content-side trigger fixture/integration testing.

### Out of scope (other deferred items)

- D-R-D4 (rename `updateMap` → `rebuildNormal`) — cosmetic, piggy-back on later cam-touching scope.
- NAI-143 deferred smoke — orthogonal.
- Any sibling-consumer migration beyond `changeStat` (e.g., `advanceStat` at `player_script.go:599+` is presently `QueueNormal` — leave as-is unless audit reveals it's also a TS-ENGINE site; check during plan-write).

---

## §2 TS source references (canonical: `LostCityRS/Engine-TS`)

- **`Engine-TS/src/engine/entity/Player.ts:325`** — `moveClickRequest: boolean = false` field declaration.
- **`Engine-TS/src/engine/entity/Player.ts:343`** — `engineQueue: LinkList<PlayerQueueRequest> = new LinkList()`.
- **`Engine-TS/src/engine/entity/Player.ts:443`** — `this.engineQueue.clear()` on logout.
- **`Engine-TS/src/engine/entity/Player.ts:641-651`** — `processEngineQueue()` body (drain semantics).
- **`Engine-TS/src/engine/entity/Player.ts:655-680`** — `updateMovement()` body, with line 657 movement gate.
- **`Engine-TS/src/engine/entity/Player.ts:821-826`** — `enqueueScript(script, type, delay, args)`: branches to `engineQueue.addTail(request)` when `type === ENGINE`.
- **`Engine-TS/src/engine/entity/Player.ts:1816-1821`** — `changeStat(stat)`: `enqueueScript(trigger, ENGINE)` — the migration target for goscape's `changeStat`.
- **`Engine-TS/src/engine/World.ts:725`** — `player.processEngineQueue()` per-player tick-loop call site (between `processTimers` and `processInteraction`).
- **`Engine-TS/src/engine/World.ts:788`** — `player.canAccess() && player.engineQueue.head() === null && queueDiscardable` — engineQueue head is part of "queue is discardable" composite check (out of scope unless an existing goscape consumer needs it; flag during plan-write if discovered).

Per memory `spec_followup_tracker_freshness`: every "TS does X" assertion above re-grep+Read at HEAD `cba768c` 2026-05-09. All confirmed at the cited line numbers.

---

## §3 Architecture

### 3.1 Data model

```go
// modules/world/player.go (next to existing `queue []playerQueueRequest` at :162)
engineQueue []playerQueueRequest
```

Re-uses `playerQueueRequest` struct (Script, Delay, IntArgs, StringArgs, Type) — entries always carry `Type == script.QueueEngine`, but the discriminator field is kept for struct-shape parity with `p.queue`. Slice over LinkList: matches goscape idiom + supports the index-based reentrancy pattern documented in `processPlayerQueue` (`tick.go:312-336`).

### 3.2 Enqueue routing

```go
// modules/world/player_script.go EnqueueScriptFile
func (p *Player) EnqueueScriptFile(sf *script.ScriptFile, delay int, intArgs []int, stringArgs []string, qtype script.PlayerQueueType) {
    if sf == nil {
        return
    }
    req := playerQueueRequest{Script: sf, Delay: delay, IntArgs: intArgs, StringArgs: stringArgs, Type: qtype}
    if qtype == script.QueueEngine {
        p.engineQueue = append(p.engineQueue, req)
        return
    }
    p.queue = append(p.queue, req)
}
```

### 3.3 Drain mechanics

```go
// modules/world/tick.go (new function; insert call between processPlayerTimers and processPathing)
func (s *Server) processPlayerEngineQueues() {
    s.playersMu.RLock()
    players := make([]*Player, len(s.playerLoop))
    copy(players, s.playerLoop)
    s.playersMu.RUnlock()

    for _, p := range players {
        func(p *Player) {
            defer recoverPlayer(p, "processPlayerEngineQueues", s.log)
            i := 0
            for i < len(p.engineQueue) {
                req := &p.engineQueue[i]
                req.Delay--
                if req.Delay > 0 || !p.canAccess() {
                    i++
                    continue
                }
                sf, intArgs, stringArgs := req.Script, req.IntArgs, req.StringArgs
                p.engineQueue = append(p.engineQueue[:i], p.engineQueue[i+1:]...)
                if sf != nil {
                    s.runScript(sf, p, nil, true, intArgs, stringArgs)
                }
                // Don't advance i — index now points to next entry.
            }
        }(p)
    }
}
```

Distinctions from `processPlayerQueue`:
- No `QueueStrong` modal-close pre-pass.
- Gating is `!p.canAccess()` (not `p.delayed && req.Type != QueueStrong`).
- No STRONG bypass: TS engineQueue is uniform.
- Reentrancy: same index-based loop pattern; same-tick chain-enqueue is supported (T6).

### 3.4 Movement gate (TS `Player.ts:657`)

Inserted at `modules/world/movement.go:47` (after `p.stepsTaken = 0` reset, before `p.moveSpeed` setup):

```go
// NAI-144: TS Player.ts:657 — modal-busy gate suppresses movement when
// the player has a pending move click and unfinished queue work.
if p.moveClickRequest && p.busy() && (len(p.queue) > 0 || len(p.engineQueue) > 0) {
    return
}
```

When the gate fires: `walkDir`/`runDir` retain prior-tick values (since they are set later in `resolveMovement`). To avoid stale outbound movement deltas, the gate must also clear them. Plan-author task: verify whether `walkDir`/`runDir` are reset elsewhere per-tick before the outgoing info block reads them; if not, the gate body needs explicit `p.walkDir = -1; p.runDir = -1` before `return`.

### 3.5 changeStat migration

```go
// modules/world/player_script.go (was line 588: script.QueueNormal)
func (p *Player) changeStat(stat int) {
    if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
        return
    }
    sf := p.client.server.scriptProvider.GetByTrigger(script.TriggerChangeStat, stat, -1)
    p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}
```

Doc-comment update: replace S6h "QueueNormal is goscape's closest available approximation..." paragraph with NAI-144 reference noting QueueEngine parity with TS `Player.ts:1816-1821`.

---

## §4 Test plan

All new tests in `modules/world/player_engine_queue_test.go` unless noted.

| ID | Name | Pins |
|----|------|------|
| T1 | `TestEnqueueQueueEngineRoutesToEngineQueue` | EnqueueScriptArgs(QueueEngine) → `len(p.engineQueue)==1 && len(p.queue)==0` |
| T2 | `TestEnqueueQueueNormalDoesNotRouteToEngineQueue` | Regression fence: QueueNormal stays in `p.queue`; `p.engineQueue` untouched |
| T3 | `TestProcessPlayerEngineQueuesFiresWhenDelayReachesZero` | delay=2 → drain twice → script fires on second drain (TS decrement-then-test pattern); entry removed |
| T4 | `TestProcessPlayerEngineQueuesGatedByCanAccess` | canAccess()=false → entry stays, no fire; canAccess()=true → fires next drain |
| T5 | `TestProcessPlayerEngineQueuesNoStrongBypassNoDelayedGate` | p.delayed=true + canAccess()=true + delay=0 → engineQueue entry FIRES (TS engineQueue ignores `p.delayed`, distinct from primary queue) |
| T6 | `TestProcessPlayerEngineQueuesSameTickReentrant` | Enqueue A; drain; A fires and (via `runScript`) enqueues B with delay=0 to engineQueue; B is visible mid-loop and fires same tick (LinkList iteration via `next` pointer in TS; slice index-based loop in goscape — equivalent observable effect) |
| T7 | `TestResolveMovementGatedByModalBusyAndQueueWork` (in `movement_test.go` or new `player_movement_gate_test.go`) | Three sub-cases: (a) gate fires when `moveClickRequest && busy() && len(queue)>0`; (b) gate fires when `moveClickRequest && busy() && len(engineQueue)>0`; (c) gate releases when both queues empty — movement resumes |
| T8 | `TestChangeStatUsesQueueEngine` (in `player_script_test.go`) | `p.changeStat(skill)` lands in `p.engineQueue`, not `p.queue` |
| T9 | Regression-fence: existing `TestAddXPFiresChangeStatOnLevelUp` (`player_script_test.go:204`) | Continues passing post-migration; if assertion pins `p.queue` directly, switch to `p.engineQueue` |
| T10 | `TestProcessPlayerEngineQueuesEmptyIsNoop` | Drain on empty engineQueue — no panic, no state change |

Plan-author crosscheck (per `plan_test_coverage_crosscheck`): each task's code block in §5 sequencing must include the test ID(s) it adds.

---

## §5 Sequencing (subagent-driven TDD)

Per memory `runescript_cadence` + `execution_mode_default`: dispatch via subagent-driven-development. Each task = implementer dispatch → reviewer dispatch (Sonnet only — `superpowers_code_reviewer_model`) → fixup → green tests → commit.

### Task 1 — Foundation

**Files:** `pkg/script/queue.go`, `modules/world/player.go`, `modules/world/player_script.go`, `modules/world/player_engine_queue_test.go` (new).

**Production:**
- Remove `// reserved` from `QueueEngine` in `pkg/script/queue.go`.
- Add `engineQueue []playerQueueRequest` field to Player struct (`player.go:162`-area).
- Refactor `EnqueueScriptFile` (`player_script.go:69`) to switch on `qtype` and route QueueEngine to `p.engineQueue`.

**Tests:** T1, T2.

### Task 2 — Drain + tick wiring

**Files:** `modules/world/tick.go`, `modules/world/player_engine_queue_test.go`.

**Production:**
- New `processPlayerEngineQueues` function in `tick.go` (mirror of `processActiveScripts` shape).
- Call site in `runTickLoopWithRate` between `processPlayerTimers()` (line 53) and `processPathing()` (line 54).

**Tests:** T3, T4, T5, T6, T10.

### Task 3 — Movement gate

**Files:** `modules/world/movement.go`, `modules/world/movement_test.go` (or new `player_movement_gate_test.go`).

**Production:**
- Insert gate at `movement.go:47` per §3.4. Plan-author verifies `walkDir`/`runDir` reset semantics before deciding whether the gate body needs explicit clears.

**Tests:** T7 (three sub-cases).

### Task 4 — changeStat migration

**Files:** `modules/world/player_script.go`, `modules/world/player_script_test.go`.

**Production:**
- `changeStat` (`player_script.go:583-589`) flips `QueueNormal` → `QueueEngine`. Update doc-comment.
- Audit: grep all `EnqueueScriptFile(...QueueNormal)` and `EnqueueScriptArgs(...QueueNormal)` for any other TS-ENGINE-aliased site beyond `changeStat`. If found, defer to follow-up unless trivial; document in spec §1 Out of scope.

**Tests:** T8 + T9 regression-fence verification.

---

## §6 Tracked deviations

- **D1: slice instead of LinkList.** `engineQueue []playerQueueRequest` instead of TS `LinkList<PlayerQueueRequest>`. Behaviorally equivalent for the operations used (append-tail, iterate-while-mutating, remove-by-index). Same idiom as the existing `p.queue`.
- **D2: discriminator field always `QueueEngine`.** `playerQueueRequest.Type` is redundant for engineQueue entries but kept for struct-shape parity with `p.queue`. Cost is one byte per entry; benefit is no parallel struct definition. YAGNI on a dedicated `engineQueueRequest` type.
- **D3: World.ts:788 `engineQueue.head() === null` discardability check.** Out of scope for NAI-144 unless plan-author audit finds an existing goscape consumer site that's broken without it. Tracker entry to add if discovered: NAI-144-followup-discardable.

---

## §7 Risk register

- **R1 (low): runScript reentrancy from engineQueue drain.** When a fired engine script enqueues another engineQueue entry mid-drain, the index-based loop must re-evaluate `len(p.engineQueue)` each iteration. Slice mutability supports this. T6 pins.
- **R2 (medium): `TestAddXPFiresChangeStatOnLevelUp` may pin `p.queue`.** Plan-author MUST grep for `TestAddXP*` and any other test asserting changeStat queue state, update to `p.engineQueue`. Per memory `verify_implementer_claims` — implementer post-dispatch, controller verifies via fresh test run.
- **R3 (medium): movement gate over-blocks.** If `moveClickRequest` is set in handler_opheld then never cleared by code that doesn't run when modal-open, gate could permanently freeze a player. Plan-author audit (per `risk_register_premise_grep`): re-grep all `moveClickRequest = ` assignments at HEAD; verify the `=false` branches are reachable from a busy/modal-open state OR document an explicit clear path. If no clear path exists at HEAD, NAI-144 must add one — likely `moveClickRequest = false` after gate fires and queues drain.
- **R4 (low): tick-slot insertion ordering breaks existing tests.** New `processPlayerEngineQueues` phase shouldn't disturb existing tests (it operates on a brand-new field). Verification: full `go test ./modules/world/...` post-Task 2.
- **R5 (low): `walkDir`/`runDir` staleness when gate fires.** Per §3.4, plan-author verifies whether `walkDir`/`runDir` reset elsewhere or whether the gate body needs explicit `-1` assignment before return.

---

## §8 Acceptance

**SECONDARY pins (test-only):** T1–T10 green at HEAD post-Task 4.

**PRIMARY pin:** `SMOKE DEFERRED` per user choice + memory `cascade_theory_smoke_binding` (foundational infra acceptable to ship test-only). Carry-forward: at NAI-145 close OR any future scope touching `[changestat,*]` content / introducing a second QueueEngine consumer, bind a smoke target (level-up modal via combat-XP train OR zone-walk SetMultiway transition).

**Bundle close commit:** `Closes memory:` trailer per memory `close_commit_memory_trailer`.

---

## §9 Carry-forward to NAI-145

NAI-145 (NAI-142-D-R-D2 + NAI-142-D-R-D3) brainstorm/spec begins fresh-session AFTER NAI-144 close (per `superpowers_clear_between_spec_and_impl`). NAI-145 will:

- Add `Player.lastMapZone int = -1` field.
- Port `triggerMapzone(x,z)`, `triggerMapzoneExit(x,z)`, `triggerZone(level,x,z)`, `triggerZoneExit(level,x,z)` per TS `Player.ts:561-596`. Each calls `scriptProvider.GetByName(<key>)` then `EnqueueScriptArgs(scriptID, 0, nil, nil, script.QueueEngine)`.
- Trigger-key strings (TS `Player.ts:561-595`):
  - `[mapzone,0_{x>>6}_{z>>6}]`
  - `[mapzoneexit,0_{x>>6}_{z>>6}]`
  - `[zone,{level}_{x>>6}_{z>>6}_{(x&0x3f)>>3<<3}_{(z&0x3f)>>3<<3}]`
  - `[zoneexit,{level}_{x>>6}_{z>>6}_{(x&0x3f)>>3<<3}_{(z&0x3f)>>3<<3}]`

  Note: `mapzoneexit` / `zoneexit` have no underscore separator — verified at HEAD against `LostCityRS/Content` 2026-05-09 (284 `[mapzone,...]` + 17 `[mapzoneexit,...]` + 100 `[zone,...]` + 5 `[zoneexit,...]` real declarations).
- Insert mapzone-transition block between step 1 (cam drain) and step 2 (lastZone) in `(*Player).updateBuildArea` (`modules/world/player.go:917-941`); enrich step 2 with `triggerZone`/`triggerZoneExit` dispatch + `SetMultiway` emission per TS `NetworkPlayer.ts:255-287` ordering. Existing slot comment at `player.go:911-912` is the authoritative landing reference.
- Add `OpSetMultiway = 254` (1-byte payload, single `pbool(hidden)`) outbound packet encoder.
- Multi-flag check via existing `pkg/gamemap.GameMap.IsMulti(x,z,level)` (NAI-120 Bundle 2A — already complete; tracker NAI-142-D-R-D3 entry's "needs porting from TS" claim is STALE).
- PRIMARY smoke target: wilderness multi-zone entry/exit (SetMultiway transition + `[zone,...]` content fires e.g. `wilderness_warning`).

---

## §10 Memories applied

- `spec_followup_tracker_freshness` — every cross-reference re-grep+Read at HEAD `cba768c` 2026-05-09; flagged stale claims (D3 `isMulti API needs porting`, D2 `[mapzone_exit,...]` underscore).
- `scope_gate_prerequisite_chain` — QueueEngine carved out as predecessor sub-spec instead of folded into D2/D3 mega-bundle.
- `consume_reserved_constant` — `QueueEngine // reserved` is exactly the placeholder this sub-spec activates.
- `runescript_cadence`, `compressed_cadence` — full cadence (not compressed) given moderate surface (~150 production LOC + ~200 test LOC + 4 task split).
- `execution_mode_default` — subagent-driven-development, no execution-mode menu.
- `superpowers_code_reviewer_model` — reviewer dispatches Sonnet-only.
- `close_commit_memory_trailer` — bundle close commit will carry `Closes memory:` trailer.
- `superpowers_clear_between_spec_and_impl`, `session_context_management` — NAI-144 close = fresh session boundary before NAI-145 brainstorm.
- `verify_implementer_claims` — controller verifies TestAddXP* assertions post-Task 4 dispatch.
- `risk_register_premise_grep` — R3 (gate over-block) requires plan-author grep audit of `moveClickRequest = ` clear paths.
- `ts_source_canonical_path` — only `LostCityRS/Engine-TS` cited; no sibling repos.
- `cascade_theory_smoke_binding` — smoke deferral acceptable for foundational infra; carry-forward note in §8.
