# NPC AI Tick Loop Decomposition — Roadmap

Sub-spec series `NAI-1` through `NAI-12` porting full `Npc.turn()` parity from
the TypeScript reference server (`Engine-TS/src/engine/entity/Npc.ts:110-185`)
into the Go implementation.

This is a **roadmap / decomposition** document. It identifies the sub-specs,
their ordering, and their boundaries. It is **not** itself an implementation
spec. Each listed sub-spec below gets its own full brainstorm → spec → plan
→ subagent-driven TDD in a separate session, following the established
RuneScript-s cadence (see memory: *Runescript-s cadence*).

## Goal

Achieve full behavioural parity between Go `Npc.turn()`
(`modules/world/npc_ai.go`) and TypeScript `Npc.turn()` (`Engine-TS/.../Npc.ts`).

Current Go implementation covers ~4 of the 10 TS `turn()` phases:

- ✅ basic lifecycle (dead-bool + respawn)
- ✅ `moveRestrict` gate
- ✅ waypoint advance (movement)
- ✅ wander/patrol mode generation
- ❌ delayed + suspended-script resume
- ❌ full `EntityLifeCycle` state machine (despawn, revert, AI_DESPAWN trigger)
- ❌ `huntAll` (player/npc/obj/loc)
- ❌ `consumeHuntTarget` → target assignment
- ❌ `processRegen`
- ❌ `processTimers` (NPC-side)
- ❌ `processQueue` (NPC-side)
- ❌ `processMovementInteraction` (NPC-initiated AP/OP triggers)
- ❌ `validateDistanceWalked`

## Non-goals

1. **Player-side `processMovementInteraction` parity gaps** — this series
   covers NPC-side only.
2. **Combat formulas / damage calculations** — NAI-11 fires OP triggers;
   combat is downstream, separate brainstorm.
3. **Hero points tracking** — deferred.
4. **Multi-tile NPCs (size > 1)** — separate brainstorm.
5. **NPC-to-player observer optimisation** — `rsbuf.GetNpcObservers` is
   already in place; NAI-7 consumes as-is.

## Series shape

```
Data layer           NAI-1  HuntType loader
Script integration   NAI-2  activeScript + resume
                     NAI-3  queue
                     NAI-4  timer
Lifecycle            NAI-5  state machine + NpcEventQueue + ai_despawn
                     NAI-6  regen
Hunt                 NAI-7  core dispatcher
                     NAI-8  huntPlayers (split out — thorniest fidelity)
                     NAI-9  huntNpcs / huntObjs / huntLocs (bundle)
                     NAI-10 consumeHuntTarget glue
Interaction          NAI-11 movement-interaction
Validation           NAI-12 validateDistanceWalked
```

## Dependency graph

```
NAI-1 (HuntType loader) ──────────────┐
                                      ▼
NAI-2 (activeScript infra) ───┬──► NAI-7 (hunt core) ──► NAI-8 (huntPlayers) ──┐
                              │          │                                    ├──► NAI-10 ──► NAI-11 ──► NAI-12
                              │          └─► NAI-9 (huntNpcs/Objs/Locs) ──────┘
                              ├──► NAI-3 (queue)
                              ├──► NAI-4 (timer)
                              ├──► NAI-5 (lifecycle + NpcEventQueue)
                              └──► NAI-6 (regen)
```

Critical path: **NAI-1 → NAI-2 → NAI-7 → NAI-8/9 → NAI-10 → NAI-11 → NAI-12**.

NAI-3, NAI-4, NAI-5, NAI-6 are independent of each other once NAI-2 lands,
and can be taken in any order between NAI-2 and NAI-7.

## Per-sub-spec boundaries

### NAI-1 — HuntType cache loader

**Scope:** Parse `hunt.dat` / `hunt.idx` from the cache, build a `hunt.Types`
config registry, expose via the world type registry alongside `NpcType`,
`LocType`, etc. Mirrors existing `NpcType` loader shape
(`pkg/objtype/npctype.go`).

**Blockers:** none.

**Rough LOC:** ~150 prod+test.

**New files:** `pkg/objtype/hunttype.go` (+ test).

**Fidelity risk:** Low — data-format-only port. Golden-byte test against
TS-produced loader output.

**TS reference:** `Engine-TS/src/engine/entity/hunt/HuntType.ts`.

### NAI-2 — NPC script infrastructure

**Scope:** Extend `Npc` with `activeScript *script.State`, `delayed bool`,
`delayedUntil int`. Add `runNpcScript` helper in a new `pkg/script/npc_script.go`
mirroring `runScript` but anchoring `activeNpc` instead of `activePlayer`.
Add suspended-script resume block at top of `Npc.turn()` matching the player
pattern at `tick.go:199-205`.

**Blockers:** none — note this is *independent of* NAI-1. Either can ship first.

**Rough LOC:** ~120 prod+test.

**New files:** `pkg/script/npc_script.go`.

**Fidelity risk:** **High.** `activeNpc` pointer anchoring semantics,
`NpcSuspended` vs `Suspended` execution-state switching, protected-pointer
cleanup on script finish. Script infra is where past S6 work has had its
subtlest bugs.

**TS reference:** `Engine-TS/.../Npc.ts:110-119`, `:216-239`.

### NAI-3 — NPC queue + `ai_queue{1..20}` dispatch

**Scope:** Define `NpcQueueRequest` in `pkg/script` (matches where
`PlayerQueueRequest` / `QueueStrong` live today — see `tick.go:212-216`).
Add `queue []NpcQueueRequest` field on `Npc`. Queue walker inside
`Npc.turn()` matching TS `processQueue`. Wire `OpNpcQueue` handler to
enqueue. Dispatch picks `TriggerAiQueue{1..20}` by request type.

**Blockers:** NAI-2.

**Rough LOC:** ~100 prod+test.

**Fidelity risk:** Medium. Re-entrant enqueue during iteration — player-side
queue at `tick.go:212-216` documents the same TS "speedup quirk"; NPC side
must preserve it identically.

**TS reference:** `Engine-TS/.../Npc.ts:180`, `NpcQueueRequest.ts`.

### NAI-4 — NPC timer + `ai_timer` trigger

**Scope:** Add `timerInterval int`, `timerClock int` on `Npc`. Single-slot
timer (TS has one per NPC, unlike player-side map). Fire inside `Npc.turn()`.
Wire `OpNpcSetTimer` handler.

**Blockers:** NAI-2.

**Rough LOC:** ~70 prod+test.

**Fidelity risk:** Low — single-slot simplifies ordering vs player-side
timer map.

**TS reference:** `Engine-TS/.../Npc.ts:178`, `:210-214`.

### NAI-5 — Full lifecycle state machine + `NpcEventQueue` + `ai_despawn`

**Scope:** Replace current `dead` bool + simplified respawn with full TS
`EntityLifeCycle` semantics (`RESPAWN` / `DESPAWN` / `FOREVER`). `lifecycleTick`
decrement gated on `!delayed`. Handle respawn, despawn, and revertType paths.
Introduce `NpcEventQueue` on `Server` plus a `processNpcEventQueue` tick phase
run *before* `processNpcs`. `ai_despawn` trigger queued via `NpcEventQueue`
on despawn.

**Blockers:** NAI-2.

**Rough LOC:** ~180 prod+test.

**New files:** `modules/world/npc_event_queue.go` (dedicated file — the queue
type, enqueue helper, and tick-phase function belong together, and keeping
them out of `tick.go` keeps that file the pure tick-order manifest it
currently is).

**Modifies:** `modules/world/tick.go` (new phase call added before
`processNpcs`).

**Fidelity risk:** **High.** Current simplified lifecycle is a known divergence.
`changeType` / `revertType` semantics, per-stat reset on revert, and lifecycle
error-retry path all have TS-specific quirks.

**TS reference:** `Engine-TS/.../Npc.ts:121-151`, `:280-317`. World-side
processing at `World.ts:356, :681-692`.

### NAI-6 — Stat regen (`processRegen`)

**Scope:** Regen clock (separate from NAI-4 timer per TS). HP + prayer + stat
regen. Call inside `Npc.turn()` after lifecycle, before timer dispatch.

**Blockers:** NAI-2.

**Rough LOC:** ~60 prod+test.

**Fidelity risk:** Medium. Per-stat regen rates; `NpcStat.*` constants
already present.

**TS reference:** `Engine-TS/.../Npc.ts:176`, `PathingEntity.processRegen`.

### NAI-7 — Hunt core dispatcher

**Scope:** Add `huntMode`, `huntRange`, `huntClock`, `huntTarget` fields on
`Npc`. `huntAll` dispatcher skeleton. Observer gating via
`rsbuf.GetNpcObservers` and `HuntNobodyNear.PAUSEHUNT` semantics. Wire
`OpNpcSetHunt`, `OpNpcSetHuntMode` handlers. The variant functions
(`huntPlayers` etc.) are stubs at this stage — filled in by NAI-8 / NAI-9.

**Blockers:** NAI-1, NAI-2.

**Rough LOC:** ~100 prod+test.

**New files:** `modules/world/npc_hunt.go`.

**Fidelity risk:** Medium. Observer gating and `huntrate` throttling via
`huntClock`.

**TS reference:** `Engine-TS/.../Npc.ts:158-171`, `:249-277`.

### NAI-8 — `huntPlayers` variant

**Scope:** Iterate player grid in `huntRange`, filter by `HuntCheck` script
fire, build `hunted[]`. Note: PAUSEHUNT semantics don't apply to the
player-type hunt path in TS (explicit exception at `Npc.ts:162`).

**Blockers:** NAI-7.

**Rough LOC:** ~80 prod+test.

**New files:** `modules/world/npc_hunt_players.go`.

**Fidelity risk:** **High.** Player-hunt has its own JagexAsh-documented
quirks (`Npc.ts:247-248` links Twitter threads). PAUSEHUNT exception and
`HuntCheck` script dispatch are where bugs hide.

**TS reference:** `Engine-TS/.../Npc.ts:263`, `huntPlayers` method.

### NAI-9 — `huntNpcs` + `huntObjs` + `huntLocs` bundle

**Scope:** Three variants sharing zone-tracked iteration. Type / category
filter via `HuntType.checkType`.

**Blockers:** NAI-7.

**Rough LOC:** ~180 prod+test (bundled).

**New files:** `modules/world/npc_hunt_entities.go`.

**Fidelity risk:** Medium. Zone-tracking iteration is an established pattern;
three variants are structurally similar.

**TS reference:** `Engine-TS/.../Npc.ts:265-271`.

### NAI-10 — `consumeHuntTarget` + `target` glue

**Scope:** Copy `huntTarget` → `target` after hunt phase, nil `huntTarget`.
Single call in `Npc.turn()` between hunt and interaction. `target` already
exists on `Npc` struct — this sub-spec wires its population.

**Blockers:** NAI-7 (any variant).

**Rough LOC:** ~40 prod+test.

**Fidelity risk:** Low — one-line glue. Easy to forget the nil-out step;
tests must guard it explicitly.

**TS reference:** `Engine-TS/.../Npc.ts:174`, `consumeHuntTarget` method.

### NAI-11 — NPC movement-interaction

**Scope:** NPC paths toward `target`, AP range check, fires
`TriggerAiApNpc{1..5}` / `TriggerAiOpNpc{1..5}` / `TriggerAiApPlayer/OpPlayer`
/ etc. Mirrors player-side `processInteraction` at
`modules/world/interaction.go`.

**Blockers:** NAI-2, NAI-10.

**Rough LOC:** ~200 prod+test.

**New files:** `modules/world/npc_interaction.go`.

**Fidelity risk:** **High.** AP vs OP range-gating matrix, trigger-type
selection by target category (player/npc/obj/loc × 1..5 + U + T). This
is the same shape as S6l → S6x work for player-side; reuse those learnings.

**TS reference:** `Engine-TS/.../Npc.ts:182`, `processMovementInteraction`
in `PathingEntity`.

### NAI-12 — `validateDistanceWalked` (NPC)

**Scope:** Per-tick sanity check: NPC walked at most 1 tile normally, 2 if
running. Mirrors player-side `validateDistanceWalked`. Final call in `turn()`.

**Blockers:** NAI-11.

**Rough LOC:** ~40 prod+test.

**Fidelity risk:** Low — direct port.

**TS reference:** `Engine-TS/.../Npc.ts:184`.

## Cross-cutting concerns

### Fields added to `Npc` struct

Tracked per sub-spec that introduces each field:

```
// NAI-2
activeScript   *script.State
delayed        bool
delayedUntil   int

// NAI-3
queue          []NpcQueueRequest

// NAI-4
timerInterval  int
timerClock     int

// NAI-5
baseType       int                 (for revertType; lifecycleTick already exists)

// NAI-6
regenClock     int

// NAI-7
huntMode       int
huntRange      int
huntClock      int
huntTarget     entity

// NAI-10 — target field already exists; this sub-spec only populates it
```

### Test strategy

Per-sub-spec, matching the established S6 pattern:

- Unit tests alongside each file (`npc_hunt_test.go`, etc.)
- Fidelity tests with golden-byte comparisons against TS-produced output
  where applicable (NAI-1 especially — follows `NpcType` loader pattern)
- Tick-integration tests for any sub-spec modifying `Npc.turn()` phase
  ordering — NAI-5 is the biggest offender
- Re-grep post-commit per memory *"Enumerate ALL call sites when propagating
  through a shared file"* — nearly every sub-spec in this series touches
  `npc_ai.go`, so this applies throughout

### Deviation tracking

Per memory *"True-to-TS fidelity gate"*: each sub-spec's spec doc carries an
explicit "TS reference" block citing exact TS `file:line` for the behaviour
it ports. Any Go-idiom divergence (e.g., slice-based queue vs TS linked-list)
gets a tracked deviation with rationale + follow-up.

### External prerequisites already satisfied

- `pkg/script/execution.go:15` — `NpcSuspended` execution state already
  defined. NAI-2 consumes it.
- `pkg/script/trigger.go` — all `TriggerAi*` types already enumerated
  (`TriggerAiApNpc{1..5}`, `TriggerAiOpNpc{1..5}`, `TriggerAiQueue{1..20}`,
  `TriggerAiTimer`, `TriggerAiSpawn`, `TriggerAiDespawn`, etc.). No sub-spec
  adds trigger constants.
- `pkg/objtype/npctype.go:80-194` — `NpcType.HuntRange` and `NpcType.HuntMode`
  fields are already loaded. NAI-7 consumes directly.

### Totals

- 12 sub-specs
- ~1,320 LOC prod+test combined
- Comparable overall scope to the S6 series, but with a single tightly-scoped
  subsystem focus
