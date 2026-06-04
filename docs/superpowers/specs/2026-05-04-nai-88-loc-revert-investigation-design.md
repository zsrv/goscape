# NAI-88 — Loc auto-revert investigation (Stage 1: probes)

**Status:** spec
**Date:** 2026-05-04
**Predecessor:** NAI-87 (SOUND_SYNTH port; cascade silenced, lifecycle revert exposed)
**Successors:** NAI-89 (Stage 2 fix; conditional on Stage 1 result)

## Goal

Surface the root cause of the **observed non-revert** of a script-driven `loc_change(inviswall, 3)` on Tutorial Island's `newbie_door1` at HEAD `60a7055`. Stage 1 of the investigation: install **probe `slog.Debug` emissions** on the goscape lifecycle revert path so a single user-driven re-smoke discriminates the two surviving hypotheses. **No behavior changes.** Stage 2 (NAI-89, separate brainstorm) ports the fix once the probe data lands.

This is an investigation sub-spec per `investigation_subspec_cadence.md`: Stage 1 risk-weighted-short-circuit audit → Stage 2 fix → conditional Stage 3.

## Smoke evidence (HEAD `60a7055`, NAI-87 close re-smoke)

User clicked `newbie_door1` at tick 40 (player at 3095, target (3098, 3107), `cheb_dist=3`). Player walked ticks 40→43, reached (3098, 3107) at tick 43; **at tick 43 `target_type_id` flipped 3014 → 83 (inviswall)** at the same coord, with `interacted=true` and `target_still_set=false`. Player walked through the inviswall (target now walkable). User reports closed door **does NOT reappear past tick 46+**. Tick rate ≈600 ms/tick (3-tick lifecycle window = 1.8 s).

User confirmed: **no `"loc tracked but no event matched"` error log fired** during the smoke. That `s.log.Error` lives in `modules/world/loc_turn.go:29` in the `turnLoc` switch's `default` branch — the only existing slog emission on the entire revert path.

## Hypothesis state at spec write

Pre-Stage-1 hypothesis ranking (from `nai_followups.md` NAI-87 carry-forward):

- (a) Tracker enrolment key mismatch — leading
- (b) Tick off-by-one — DEMOTED (pure ±1 still reverts)
- (c) Zone routing (LOC_CHANGE'd zone not in processZones iteration set) — leading
- (d) `RevertLoc` / `turnLoc` early-return guard — leading
- (e) `processZones` only enrols zones touched by current-tick script-driven LOC ops — leading

**Code-reading at spec write falsifies (c) and (e):** `Server.processZones` (modules/world/tick.go:470-495) iterates `s.locObjTracker` (a flat doubly-linked list of `*NonPathing`) **independently of `s.zonesTracking`**. The `zonesTracking` map governs `ComputeShared` + `Reset` only. Per-tick `clear(s.zonesTracking)` at tick.go:532 cannot strand a tracked NonPathing across ticks, because the tracker's iteration source is a different data structure that never gets cleared except by per-entry `Unregister`.

**User confirmation falsifies a third candidate** — call it (f) "switch-case fall-through": if `turnLoc` ran at tick 46+ with mismatched `Lifecycle`/`IsActive`/`IsChanged()` (i.e. `np` reached tracker iter and `LifecycleTick == now` matched, but none of the three explicit `case` arms matched) it would have logged `"loc tracked but no event matched"` from the `default` arm at line 29 and untracked. No such log = (f) ruled out. Remaining live hypotheses: **(a)** np not in tracker / spurious Unregister, OR **(d)** `LifecycleTick != now` silent early-return at line 16.

**Surviving hypotheses (Stage 1 must discriminate):**

- **(a) Tracker enrolment key mismatch** — `np` was never Registered at tick-43 ChangeLoc (e.g. `s.locObjTracker == nil` check at world_zone.go:60-64 fell into the `else` arm via wrong duration), OR `np` was Registered then spuriously Unregistered between tick 43 and tick 46 (some other call site mutating the tracker), OR the registered `np` and the iterated `np` are distinct pointers (key mismatch).
- **(d) `turnLoc` silent early-return** — `np` IS in tracker but `LifecycleTick` is stuck at a value that never equals `s.currentTick` on tick 46+ iterations. Possible mechanisms: `LifecycleTick` overwrite by an unrelated call (e.g. `Entity.SetLifecycle(-1, ...)` from a different code path), or the tick-43 `SetLifeCycle(3, currentTick=43, ...)` path running with a stale `s.currentTick` reading.

## Reference

- TS counterparts (no behavior change at Stage 1; for context only):
  - `Engine-TS/src/engine/World.ts:961-986` — `processZones`
  - `Engine-TS/src/engine/World.ts:1350-1386` — `changeLoc`
  - `Engine-TS/src/engine/World.ts:1427-1448` — `revertLoc`
  - `Engine-TS/src/engine/entity/NonPathingEntity.ts:11-25` — `setLifeCycle`
- Existing in-repo probe convention (gating, schema, NodeDebug-gated, nil-log safe):
  - `modules/world/handler_oploc.go:104-118` — single-call-site frame
  - `modules/world/interaction_debug.go:48-69` — multi-field interaction frame
- HEAD pin: `60a7055` (NAI-87 close).

## Stage 1 design — probe set

All probes are **`slog.Debug`-level**, **`s.cfg.NodeDebug`-gated**, **nil-log safe** (`s.log != nil` check), and tagged with the literal comment `// NAI-88 probe; remove at Stage 2 close` so retire is one grep + Edit per site.

### Probe sites and field schemas

| # | Site | File:line at HEAD `60a7055` | Event name | Purpose |
|---|---|---|---|---|
| P1 | `Server.processZones` lifecycle iter (top of `for _, np := range snap`) | `modules/world/tick.go:482` | `nai88 process_zones iter` | Confirms `processZones` runs and reports tracker size + iteration cursor |
| P2 | `Server.turnLoc` entry (before `LifecycleTick != now` early-return) | `modules/world/loc_turn.go:15` | `nai88 turn_loc entry` | Discriminates (d): if fires every tick 44→46+ with the right LifecycleTick we expect a match at tick 46 |
| P3 | `Server.RevertLoc` entry | `modules/world/loc_turn.go:39` | `nai88 revert_loc entry` | Confirms case-2 dispatch fired (== "RESPAWN+IsChanged+IsActive matched") |
| P4 | `Server.ChangeLoc` post-decision (both arms) | `modules/world/world_zone.go:60-64` | `nai88 change_loc setlifecycle` | Discriminates (a): records which `SetLifeCycle` arm fired (duration vs. -1) and the tracker pointer passed |
| P5 | `locObjTracker.Register` | `modules/world/loc_tracker.go:34` | `nai88 tracker register` | Discriminates (a): confirms np entered the list with the expected key + tracker_size delta |
| P6 | `locObjTracker.Unregister` | `modules/world/loc_tracker.go:43` | `nai88 tracker unregister` | Discriminates (a): catches spurious mid-window Unregister between tick 43 and tick 46 |

P5 and P6 require a logger reference inside `locObjTracker`. Plumb via constructor signature change:

- `newLocObjTracker(log *slog.Logger, nodeDebug bool) *locObjTracker` — store both as unexported fields; method bodies do `if t.nodeDebug && t.log != nil { t.log.Debug(...) }`.
- Update production call site `Server.New` (modules/world/server.go:167): pass `s.log` and `s.cfg.NodeDebug`.
- Update test-fixture call sites (4 in `modules/world/loc_tracker_test.go` lines 10/23/37/51, 1 in `modules/world/server_test.go` line 318) to pass `(nil, false)`. The nil-log + false-debug guard makes these no-ops; existing tracker unit tests stay green unchanged.

Alternative considered: probe at all 5 `SetLifeCycle` call sites in `world_zone.go` (lines 27, 61, 63, 85, 87) + the `RevertLoc` tail at `loc_turn.go:60`. Rejected: 6 call sites × 3-line probe vs. 2 method-body probes; tracker-internal probes also catch any future SetLifeCycle caller without re-instrumenting.

### Common field schema (all 6 probes)

Every probe emits at minimum: `tick` (= `s.currentTick`), `event_id` (literal `"P1"`..`"P6"` for cross-ref). Site-specific fields:

- **P1** `tracker_size`, `cursor` (iteration index)
- **P2** `loc_x`, `loc_z`, `loc_level`, `loc_type`, `lifecycle` (int), `is_active` (bool), `is_changed` (bool), `lifecycle_tick`, `now`
- **P3** `loc_x`, `loc_z`, `loc_level`, `loc_type` (still inviswall pre-Revert)
- **P4** `loc_x`, `loc_z`, `loc_type` (post-Change), `is_changed`, `lifecycle` (int), `duration`, `arm` (`"register"` / `"untrack"`)
- **P5** `np_addr` (formatted `%p`), `tracker_size_after`
- **P6** `np_addr` (formatted `%p`), `tracker_size_after`, `caller` (one-frame `runtime.Caller(1)` "file:line" — P6 is the highest-suspicion site for hypothesis (a))

`np_addr` (pointer formatting) is the cross-probe correlation key — Stage 1 analysis will grep all 6 probes for a single `np_addr` and reconstruct the np's lifetime across ticks 43→46+.

### Files touched (Stage 1)

| File | Change | Approx LOC |
|---|---|---|
| `modules/world/tick.go` | P1 probe at `processZones` lifecycle iter; pre-loop `tracker_size` capture | 8 |
| `modules/world/loc_turn.go` | P2 probe at `turnLoc` entry; P3 probe at `RevertLoc` entry | 18 |
| `modules/world/world_zone.go` | P4 probe at `ChangeLoc` post-decision (both arms) | 10 |
| `modules/world/loc_tracker.go` | Add `log *slog.Logger` + `nodeDebug bool` fields; constructor plumbing; P5 + P6 probes | 20 |
| `modules/world/server.go` | Update `newLocObjTracker(...)` call site at line 167 to pass `s.log, s.cfg.NodeDebug` | 1 |
| `modules/world/loc_tracker_test.go` | Update 4 fixture call sites (lines 10/23/37/51) to pass `(nil, false)` | 4 |
| `modules/world/server_test.go` | Update 1 fixture call site (line 318) to pass `(nil, false)` | 1 |

**Total Stage 1 production LOC: ~57.** Test surface: 5 mechanical fixture-arg updates (no new tests). Probes are runtime-observable, not compile-time-verifiable; the existing tracker unit tests already pin Register/Unregister behavior, and the `(nil, false)` guard preserves those tests unchanged.

## Discrimination table

After re-smoke, grep for `nai88` on the world log file. Expected outcomes by hypothesis:

| Observation | Implies | Stage 2 root cause |
|---|---|---|
| P1 fires every tick with `tracker_size=0` from tick 43 onward; no P5 ever fired | (a) np never registered at tick-43 ChangeLoc | World.ChangeLoc setLifeCycle decision logic / tracker init |
| P5 fired at tick 43 with `tracker_size_after=1`; P6 fired between tick 43 and tick 46 with `caller` outside `RevertLoc`/`ChangeLoc`/`RemoveLoc` | (a) spurious Unregister | The unexpected caller revealed by P6's `caller` field |
| P5 fired at tick 43; no P6 fired before tick 46; P1 reports `tracker_size=1` at tick 46; P2 fires with `lifecycle_tick != now` and `now != 46` | (d) currentTick desync — `now` arg threading bug or `s.currentTick` racing | Tick clock plumbing into processZones / turnLoc |
| P5 fired at tick 43 with `duration=...` ≠ 3; P2 fires at wrong tick with right LifecycleTick | (a) wrong duration into ChangeLoc | LOC_CHANGE handler or LocOps.ChangeLoc plumbing |
| P5 fired at tick 43; P2 fires at tick 46 with match; P3 fires; **revert still not visually observed** | Wiring through to revert is correct; bug is downstream (zone Shared, client decode, collision) | Open Stage 1.5 to probe zone delivery / client-side; out-of-scope for current spec |
| P4 at tick 43 reports `arm="untrack"` (i.e. `IsChanged()`-and-DESPAWN both false at decision point) | (a) — the Change at world_zone.go:50 didn't dirty CurrentInfo, OR Lifecycle is unexpectedly DESPAWN | `loc.Change` semantics or static-loc Lifecycle init |

## Out-of-scope (Stage 1)

- **Behavior changes** of any kind. Stage 1 is observation only.
- **Probes outside the lifecycle revert path.** Player movement / pathfinding / interaction probes already exist (NAI-79, handler_oploc, interaction_debug); Stage 1 only adds the missing lifecycle channel.
- **Zone broadcast / client decode probes.** If discrimination row 5 lands (revert fires but visual missing), Stage 1.5 is brainstormed separately.
- **Stage 2 fix.** Routed to NAI-89 once the probe data is in hand.

## Cadence and review

Investigation sub-spec, single bundle. Per `compressed_cadence.md` this could qualify as compressed (~57 LOC, no tests) but I keep full cadence (separate plan doc, two-stage Sonnet review per task) because:

1. Multi-file constructor signature ripple (`loc_tracker.go` → `server.go` + 5 test fixtures); review catches missed call sites.
2. Multiple probe sites must share a coherent field schema for Stage 2 grep to work — review catches schema drift.
3. Investigation sub-specs benefit from explicit Stage 1/2 boundary discipline; compressed cadence muddies the close-then-reopen handoff.

Per `execution_mode_default.md`: subagent-driven-development. Per `controller_preflight.md`: re-grep all named line numbers (tick.go:482, loc_turn.go:15/29/39, world_zone.go:60-64, loc_tracker.go:34/43, server.go:167) at HEAD before each implementer dispatch.

## Tech Stack

- Go 1.26+
- `log/slog` (existing project logger; `*slog.Logger` plumbing)
- No new dependencies

## Stage 1 close criteria

- Build green: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
- Tests green at unchanged shape: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` (probe sites add no test surface; existing `loc_turn_test.go` + tracker tests must still pass; nil-log safety asserts no test breaks from missing logger in tracker fixtures)
- User-driven re-smoke captured: world log file with `nai88` lines for ticks 40→50 around a `newbie_door1` click on Tutorial Island
- Probe data analyzed against discrimination table; root cause identified or escalated to Stage 1.5

## Followups deferred to Stage 2 / NAI-89

- Actual fix implementation per the discriminated hypothesis.
- Probe retire (grep `// NAI-88 probe` and revert each site; ditto the `locObjTracker` logger plumbing if not load-bearing for the fix).

## Memory ties

- `investigation_subspec_cadence.md` — Stage 1/2/3 pattern.
- `protocol_stub_not_completed.md` — adjacent class (declared-but-unwired); lifecycle wiring IS in place per NAI-86 B2.3/B2.4 unit tests, so this is a different failure mode (wired-but-silent), informing why probes are required.
- `verify_implementer_claims.md` — Stage 1 itself is an instance: NAI-86 unit tests pass but production behavior diverges.
- `controller_preflight.md` — re-grep all 9 named line numbers at HEAD pre-dispatch.
- `cascade_theory_smoke_binding.md` — Stage 1 close attribution waits on user re-smoke; do not pre-attribute.
- `smoke_test_server_handoff.md` — server is user-launched.
- `close_commit_memory_trailer.md` — Stage 1 close commit carries `Closes memory: nai_followups.md NAI-87 carry-forward (NAI-88 candidate lifecycle revert).`
