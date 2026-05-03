# NAI-79: Tutorial Island door cascade-blocker investigation

**Status:** Draft (brainstorm output)
**Date:** 2026-05-03
**Cadence:** Investigation+fix sub-spec (Stage 1 instrumentation → smoke handoff → Stage 2 conditional fix)
**Predecessor:** NAI-78 (`tryInteract` 4-branch port; partial close, smoke confirmed cascade-residual)
**Tech stack:** Go 1.26+ (project) / TS reference at LostCityRS/Engine-TS

---

## 1. Goal

Pin which of three hypotheses owns the post-NAI-78 cascade-residual at the Tutorial Island RS Guide door (and the bookcase + drawer items, which share the same OPLOC1 → Loc-target dispatch path), then ship the corresponding fix in the same sub-spec.

**Symptom (re-confirmed at NAI-78 close):** Player clicks the RS Guide door (item 1), bookcase (item 2), or drawer (item 3) on Tutorial Island. OPLOC1 is received. Player does not move and no script effect is observed. Symptom shape is byte-for-byte identical to the pre-NAI-78 baseline; NAI-78's tryInteract 4-branch port closed the auto-clear divergence at the engine layer (regression test green) but is not symptom-resolving on its own.

Per `smoke_unchanged_means_multiple_blockers.md` and `cascade_theory_smoke_binding.md`: the unchanged smoke shape after a fix that tests confirm correct is a strong signal that the cascade graph has at least one more node. NAI-79 finds it.

## 2. Context: hypotheses (from `nai_followups.md` "From NAI-78" section)

**H1 (most likely): `Player.pathToTarget` is shape-blind.**
`modules/world/interaction.go:489–493`'s `pathToTarget` calls `pathToMoveClick` which calls `Pathfinder.FindPathDefault` (`pkg/pathfinder/routefinder/api.go:37`, deprecated wrapper) at `movement.go:132`. That wrapper hardcodes `destWidth=1, destLength=1, angle=0, shape=-1`. For wall-shape doors / multi-tile locs, the pathfinder doesn't know the loc's geometry, so the route either ends at a non-operable tile or produces zero waypoints. `moveNear=true` should fall back, but the TS analogue (`PathingEntity.pathToTarget` at Engine-TS Player.ts:457–508) threads `width/length/angle/shape` through to `findpath`. The Loc-aware API exists at `api.go:42` (`FindPath(... destWidth, destLength, angle, shape, ...)`) but is not used by `Player.pathToTarget`.

**H2 (less likely): cache-key resolution mismatch.**
`getOpTrigger` / `getApTrigger` (interaction_trigger.go) derive `typeId` via `tgt.Type()` and `categoryId` via `srv.locTypes.Configs[locId].Category`. Production cache loads scripts keyed at `LookupKeyForType(TriggerOpLoc1, doorTypeID)`. If the cached typeID does not match runtime `Loc.Type()`, branch 1 of tryInteract never sees a non-nil `opTrigger`.

**H3 (low-probability): unauthorized state mutation.**
Some path through `processInteraction` nulls `p.target` before the post-step `tryInteract` runs (e.g., a script side-effect, an unrelated `ClearInteraction` call, an out-of-tree write).

**H4 (open): captured frames don't fit H1/H2/H3.**
Fallback bundle dispatches an audit-subagent over the captured logs.

## 3. Cadence

Per `investigation_subspec_cadence.md`:

1. **Bundle 0 — controller pre-flight.** Read HEAD-current shape of `interaction.go:331–400` (tryInteract 4-branch), `interaction.go:489–493` (pathToTarget), `movement.go:119–143` (pathToMoveClick), `handler_oploc.go:25–96` (handleOpLoc). Already done as part of brainstorm; no findings invalidate the seed.
2. **Bundle 1 — Stage 1 instrumentation.** Land permanent `Cfg.NodeDebug`-gated structured logs at four diagnostic choke-points. One feat commit. Tests pin gate behavior + frame field population.
3. **Smoke handoff (out-of-band).** User runs server with default config (`NodeDebug=true`), drives the Java client through items 1+2+3 + a control case, attaches the captured log.
4. **Bundle 2 — Stage 2 conditional fix.** Plan-author reads §5 routing rules against the captured log → promotes ONE of bundles H1/H2/H3/H4 (§6) into concrete tasks → ships fix + tests.
5. **Smoke #2.** User confirms items 1+2+3 PASS at the Stage 2 commit.
6. **Close commit.** `chore(close): NAI-79 — door cascade-blocker [hypothesis]` with memory trailer.

Bundle 1's instrumentation is **permanent** (per brainstorm decision A): it ships as production code under `Cfg.NodeDebug`, partially retiring the `NAI-78-D-DEBUG-MSG-DEFERRED` deferred-debug-channel motivation. Default config has `NodeDebug=true`; production deployments setting `NodeDebug=false` silence all of it.

## 4. Stage 1 — instrumentation surface

Two structured-log frames, both gated on `s.cfg.NodeDebug`. All field names use `snake_case` to match existing slog convention in `server.go`.

### 4.1 Frame A — handler frame at `handleOpLoc` tail

**Site:** `modules/world/handler_oploc.go`, immediately before the success `return nil` on line 95 (after `targetSubject` snapshot).

**Schema:**

```go
if s.cfg.NodeDebug {
    s.log.Debug("oploc handler",
        "tick",       s.currentTick,
        "player_uid", p.uid,
        "op",         op,                     // 1..5
        "click_x",    x,
        "click_z",    z,
        "loc_id",     locId,
        "loc_name",   locType.DebugName,      // H2 evidence; ConfigType.DebugName (config field)
        "loc_shape",  loc.Shape(),            // H1 evidence (see pkg/entity/loc.go:31)
        "loc_angle",  loc.Angle(),            // H1 (loc.go:34)
        "lt_width",   locType.Width,          // H1
        "lt_length",  locType.Length,         // H1
        "op_slot",    locType.Op[op-1],       // H2: trigger key (e.g. "newbie_door1")
    )
}
```

Drives H1 (Loc geometry inputs that `FindPathDefault` ignores) and H2 (cache-key trigger string). Note: there is no `LocType.Active` field; the per-op interactability check is `op_slot != ""` (matches the handler gate at `handler_oploc.go:83`).

### 4.2 Frame B — interaction tick frame at `processInteraction` tail

**Site:** `modules/world/interaction.go`, as the LAST statement of `processInteraction` (after the line-259 mapflag clear, line ~262), only when `p.target != nil` was true at function ENTRY.

**Schema:** All target-coord fields refer to the INITIAL target (snapshotted at function entry); `target_still_set` separately signals whether `p.target` was nulled during the tick. This keeps `cheb_dist` consistent with the displayed `target_x`/`target_z` even when an auto-clear happened.

```go
if hadTarget && s.cfg.NodeDebug {
    s.log.Debug("interaction tick",
        "tick",             s.currentTick,
        "player_uid",       p.uid,
        "target_kind",      targetKindString(initialTarget), // "Loc"|"Npc"|"Player"|"Obj"
        "target_type_id",   initialTarget.Type(),
        "target_x",         initialTargetX,                  // initial; not live
        "target_z",         initialTargetZ,                  // initial; not live
        "player_x",         p.x,
        "player_z",         p.z,
        "cheb_dist",        chebDist(p.x, p.z, initialTargetX, initialTargetZ),
        "op_trigger",       opTriggerPresent,                // captured at pre-step tryInteract entry
        "ap_trigger",       apTriggerPresent,                // captured at pre-step tryInteract entry
        "ap_range",         p.apRange,
        "waypoint_idx",     p.waypointIndex,                 // H1: -1 = no path
        "branch_pre",       p.lastInteractBranchPre,         // 0=fallthrough|1|2|3|4
        "branch_post",      p.lastInteractBranchPost,        // 0=fallthrough|1|2|3|4
        "interacted",       interactedFinal,
        "interaction_fired",p.interactionFired,
        "steps_taken",      p.stepsTaken,
        "repathed",         p.repathed,
        "target_still_set", p.target != nil,                 // H3: did auto-clear nuke?
    )
}
```

Drives all three hypotheses.

### 4.3 Branch tracking inside `tryInteract`

Two new `Player` fields:
- `lastInteractBranchPre  int  // 0=fallthrough|1|2|3|4`
- `lastInteractBranchPost int  // same encoding`

`processInteraction` resets both to 0 at function entry, then inside `tryInteract` each of the four `return` statements writes the branch number via a small helper. The pre/post distinction is signalled by a transient `interactCallSlot int` field on `Player` (`0`=pre, `1`=post) that `processInteraction` sets immediately before each `tryInteract` call. The helper inside tryInteract reads the slot and writes to `lastInteractBranchPre` or `lastInteractBranchPost` accordingly.

Field-on-Player chosen over a `(bool, int)` return tuple to keep `tryInteract`'s public signature intact (every existing test of tryInteract continues to work; only the bookkeeping is added).

### 4.4 Helpers (file-local, unexported)

- `targetKindString(t entity) string` — switch on type, returns `"Loc"|"Npc"|"Player"|"Obj"|"unknown"`.
- `chebDist(ax, az, bx, bz int) int` — Chebyshev distance.

### 4.5 Pre-step state capture

`processInteraction` captures pre-step values into local variables before any mutation:
- `initialTarget := p.target`
- `initialTargetX, initialTargetZ, _ := p.target.Coords()`
- `hadTarget := p.target != nil`
- `opTriggerPresent := getOpTrigger(p, s) != nil`
- `apTriggerPresent := getApTrigger(p, s) != nil`

These survive the function so Frame B can emit even if `p.target` is nulled by an auto-clear.

## 5. Hypothesis-to-log-signature decision tree

Applied in order; first match wins.

### H1 — Loc-aware pathToTarget needed

**Match condition:** Frame A shows `op_slot != ""` (cache has a registered op script for this slot, so the click was actionable). AND for the SAME `player_uid`, Frame B records over the next 2–5 ticks show EITHER:

- **H1a (no path):** every tick shows `waypoint_idx=-1` AND `steps_taken=0` AND `cheb_dist > 1`. Pathfinder produces zero waypoints. Most likely against wall-shape doors.
- **H1b (wrong-tile path):** at least one tick shows `waypoint_idx >= 0` and `steps_taken > 0`, but `cheb_dist > 1` persists after movement consumed across multiple ticks (path ends at a non-operable tile).

In both H1a and H1b, `op_trigger=false` AND `ap_trigger=false` per cache lookup against runtime `target_type_id` (these would also hold under H2; the disambiguator is whether Frame A's `op_slot` is non-empty — H1 says yes, H2 says yes; the further H1-vs-H2 split is `waypoint_idx` behavior. Both H1 sub-cases imply the routing pipeline reached its endpoint, ruling H2 out for H1a/H1b).

Final clear under H1: `target_still_set=false` typically arrives on the tick `processInteraction` emits "I can't reach that!" (interaction.go:230) — `steps_taken=0 && !hasWaypoints && !interacted`.

**Confidence-elevators:**
- `lt_width > 1` OR `lt_length > 1` (multi-tile loc).
- `loc_shape ∈ {0, 1, 2, 3, 9}` (wall-shape codes per `pkg/entity/loc.go:31` + RuneScape shape table).

**Bundle:** §6.1 (H1).

### H2 — cache-key resolution mismatch

**Match condition:** Frame A shows `op_slot != ""` (cache HAS a script registered). AND Frame B shows:
- `op_trigger=false` AND `ap_trigger=false`, AND
- `target_type_id` equals Frame A's `loc_id`.

This means: cache exposes a script key but `getOpTrigger`/`getApTrigger` cannot find it. The mismatch lives in `LookupKeyForType` semantics, scriptProvider load order, or `Loc.Type()` returning a different typeID at runtime than at load time.

**Bundle:** §6.2 (H2).

### H3 — unauthorized target mutation

**Match condition:** Frame B shows on at least one tick:
- `op_trigger=true` OR `ap_trigger=true` (resolution works), AND
- `target_still_set=false` AND `interaction_fired=false` AND `branch_pre ∈ {0, 3}` AND `branch_post ∈ {0, 3, 4}`.

This means cache works, scripts resolve, but `target` is being nulled by a code path other than the documented auto-clear (line 248) and waypoint-exhaustion clear (line 222).

**Bundle:** §6.3 (H3).

### H4 — unknown signature

**Match condition:** none of H1/H2/H3 matches.

**Bundle:** §6.4 (H4) — audit-subagent dispatch over the captured frames.

## 6. Stage 2 — conditional fix bundles

Plan-author promotes EXACTLY ONE bundle into concrete tasks based on §5 routing.

### 6.1 Bundle H1 — Loc-aware `pathToTarget` port

**Goal:** thread Loc geometry (`width`, `length`, `angle`, `shape`) from the player's target into the pathfinder so wall-shape and multi-tile loc destinations get correct approach-tile routing.

**Reference:** TS `PathingEntity.pathToTarget` at Engine-TS Player.ts:457–508.

**Tasks (template):**

1. **Extend `pathToMoveClick` signature.** Add `destWidth, destLength, destAngle, destShape int` (default `1, 1, 0, -1` reproduces current behavior). Update the existing call site in `handleMoveClick`'s `moveClickInner` to pass `(1, 1, 0, -1)`.
2. **`pathToTarget` reads geometry from `p.target`.** Type-switch on `*entitypkg.Loc` → read `LocType.Width`, `LocType.Length`, `loc.Angle()`, `loc.Shape()` from `srv.locTypes.Configs[loc.Type()]` + `loc.go:31,34`. Type-switch on `*entitypkg.Obj` → `(1, 1, 0, -1)` for now (objs are 1×1 ground items; no shape rotation). Default (`*Npc`, `*Player`) → `(1, 1, 0, -1)` to preserve NAI-11 NPC-naive deferral.
3. **Wire `FindPath` (Loc-aware) into `pathToMoveClick`.** Replace `gamemap.Pathfinder.FindPathDefault(p.level, p.x, p.z, dest.X, dest.Z)` call at `movement.go:132` with `gamemap.Pathfinder.FindPath(...)` per `api.go:42` signature. Plan-author reads `api.go:37` (the `FindPathDefault` body) to get the verbatim trailing arg-list — `FindPathDefault` itself just calls `FindPath(... 1, 1, 1, 0, -1, true, 0, 25, ...)`, so the new call is the same shape but with the four geometry args wired through from the caller.
4. **Regression test.** New file `modules/world/pathing_loc_aware_test.go`. Fixture: spawn a `LocType{Width:1, Length:1, Active:true}` with `Shape=0` (wall) at level 0 (10, 10), player at (10, 12). Call `pathToTarget(10, 10)`. Assert `len(p.waypoints) > 0` AND last waypoint is at `(11, 10)` or `(9, 10)` (operably-adjacent on the non-wall side).
5. **Bug-baseline pin.** Same fixture on the pre-NAI-79 `FindPathDefault` path: assert `waypoints` is empty OR last waypoint is non-operably-adjacent. Pinning the bug shape lets the cleanup commit verify the fix.
6. **Smoke #2.** Door + bookcase + drawer all PASS.

**Memory closures:** `pathfinder_api_loc_aware.md`'s player-side recommendation is retired in scope.

**Deviation tally:** 15 → 15 (port matches TS; no new deviation).

### 6.2 Bundle H2 — cache-key resolution audit

**Goal:** find why a registered script key doesn't resolve for the runtime Loc.

**Tasks (template):**

1. Audit-subagent dispatch (Explore). Seed: captured Frame A `loc_id`, `loc_type`, `op_slot` + Frame B `target_type_id`, `op_trigger=false`. Inputs: `interaction_trigger.go` (getOpTrigger/getApTrigger), `pkg/script/lookup.go` (LookupKeyForType), `pkg/entity/loc.go` (Loc.Type), scriptProvider load path. Question: at what layer does the trigger key not resolve?
2. Audit verdict appended to plan doc.
3. Concrete fix landed per audit findings (TBD shape).
4. Regression test pinning the resolution.
5. Smoke #2.

### 6.3 Bundle H3 — unauthorized target mutation

**Goal:** find the un-anchored `p.target = nil` write.

**Tasks (template):**

1. Add Tier-2 instrumentation: `s.log.Debug("target cleared", "site", "<callerName>", ...)` at every `p.target = nil` and `p.ClearInteraction()` write site (search via `grep -rn "p.target = nil\|ClearInteraction()" modules/world/`). Permanent, NodeDebug-gated.
2. Re-smoke. Captured Tier-2 logs identify the unauthorized site.
3. Site-specific fix.
4. Regression test pinning the new clear-site contract.
5. Smoke #2.

### 6.4 Bundle H4 — unknown

**Goal:** find a fourth root cause not covered by H1/H2/H3.

**Tasks (template):**

1. Audit-subagent dispatch (Explore) over captured frames. Question: what's the FIRST visible state mutation that doesn't happen post-click? Trace forward from there.
2. Audit verdict in plan doc.
3. Brainstorm Bundle H4-fix as a Stage 2.5 cycle.

## 7. Smoke handoff

Per `smoke_test_server_handoff.md`. Stage-1 close commit triggers a paste-ready resume prompt:

```
Build at HEAD <sha>:
  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 \
    go build -trimpath -o /tmp/goscape-nai79 ./cmd/goscape

Run on your host (not Claude's sandbox):
  /tmp/goscape-nai79 --config.file config.yaml 2>&1 | tee /tmp/nai79-smoke.log

Default config has NodeDebug=true; logs land automatically.

Launch the Java client, log in to Tutorial Island as a fresh tutorial-stage
character, and perform:

  (a) Click the RS Guide door (item 1). Wait 5 ticks. Note any visible
      symptom (player moves? message? interaction?).
  (b) Click the bookcase (item 2). Wait 5 ticks.
  (c) Click the drawer (item 3). Wait 5 ticks.
  (d) Control: click an empty floor tile to walk. Confirm normal movement.

Then attach /tmp/nai79-smoke.log (or paste the "oploc handler" + "interaction
tick" records from the run). I'll route the captured frames per spec §5
routing rules and dispatch the matching Stage 2 bundle.
```

Stage 2 close commit emits a second resume prompt for smoke #2 (re-run items 1+2+3, expect PASS).

## 8. Tests

### Stage 1 unit tests (Bundle 1)

`modules/world/interaction_instrumentation_test.go`:

1. **TestInteractionFrameA_EmittedWhenNodeDebugTrue** — handler success path with `NodeDebug=true`; assert one `oploc handler` record with all 13 fields.
2. **TestInteractionFrameA_SuppressedWhenNodeDebugFalse** — same fixture, `NodeDebug=false`; assert no `oploc handler` record.
3. **TestInteractionFrameB_EmittedWhenTargetSetAndNodeDebugTrue** — fixture: player with `target` set, run `processInteraction`; assert one `interaction tick` record with all fields.
4. **TestInteractionFrameB_SuppressedWhenNoTargetAtEntry** — fixture: player with `target=nil`; assert no `interaction tick` record.
5. **TestBranchTracking_Branch1Through4_PerCallsite** — five fixtures, one per branch outcome (1, 2, 3, 4, fallthrough); assert `lastInteractBranchPre`/`lastInteractBranchPost` populate correctly through pre-step + post-step calls.
6. **TestChebDistAndTargetKindString** — table-driven helper coverage.

Implementation strategy for asserting log records: capture via a `bytes.Buffer`-backed `*slog.Logger` constructed in test setup (existing pattern; see `server_test.go:322` `noopBridges{}` analogue).

### Stage 2 tests

Bundle-specific. H1's regression + bug-baseline tests are described in §6.1. H2/H3/H4 tests are template and finalize at plan-author dispatch.

## 9. Tracking & deviations

**Stage 1 (Bundle 1):** no new deviations. Instrumentation is additive, gated, matches existing slog conventions. Partial closure of `NAI-78-D-DEBUG-MSG-DEFERRED` motivation (the `Cfg.NodeDebug`-as-debug-channel wiring lands here, even though the specific TS `[debug] No trigger for ...` chat-side message remains deferred).

**Stage 2:** bundle-dependent. H1 closes one open follow-up (`pathfinder_api_loc_aware.md`'s player-side wiring). H2/H3/H4 may surface new deviations.

**Net deviation tally projection:**
- After Bundle 1: 15 → 15.
- After Bundle H1: 15 → 15.
- After Bundle H2/H3/H4: TBD per audit verdict.

**Memory entries to save at close:**
- One entry capturing the captured-log signature → routing decision (so future investigation sub-specs see how Stage 1 evidence pins hypothesis).
- If Bundle H1 ships: one entry on `pathToMoveClick` signature evolution (geometry threading) for downstream NPC-side port (NAI-11 deferred).

**`Closes memory:` trailer** on the close commit per `close_commit_memory_trailer.md`.

## 10. Out of scope

- NPC-side AP-pathing port (`npc_interaction.go:367`'s naive `FindPathDefault`) — explicitly NAI-11 territory. Bundle H1 leaves the npc path untouched.
- The three NAI-78 deferred deviations: `NAI-78-D-NULL-TYPE-GUARD-OMITTED`, `NAI-78-D-DEBUG-MSG-DEFERRED` (chat-side message), `NAI-78-D-HASINTERACTION-GUARD`.
- NAI-77 same-tile `NodeClientRoutefinder=true` edge case.
- TUT_CLOSE handler.
- `World.ts:624–628 moveClickRequest` per-tick assignment.

## 11. References

**TS source (canonical):**
- `LostCityRS/Engine-TS/src/engine/entity/Player.ts:457-508` — `PathingEntity.pathToTarget` (Loc-aware; threads width/length/angle/shape).
- `LostCityRS/Engine-TS/src/engine/entity/Player.ts:1113-1184` — `Player.tryInteract` (4-branch).
- `LostCityRS/Engine-TS/src/engine/entity/Player.ts:1200-1264` — `Player.processInteraction`.
- `LostCityRS/Engine-TS/src/engine/handlers/OpLocHandler.ts:14-42` — handler validation gates.

**Goscape current shape:**
- `modules/world/interaction.go:151-262` — `processInteraction`.
- `modules/world/interaction.go:331-400` — `tryInteract` 4-branch (post-NAI-78).
- `modules/world/interaction.go:489-493` — `pathToTarget`.
- `modules/world/movement.go:119-143` — `pathToMoveClick`.
- `modules/world/handler_oploc.go:25-96` — `handleOpLoc`.
- `pkg/pathfinder/routefinder/api.go:37` — `FindPathDefault` (deprecated, shape-blind).
- `pkg/pathfinder/routefinder/api.go:42` — `FindPath` (Loc-aware, target).
- `pkg/entity/loc.go:31,34` — `Loc.Shape()` / `Loc.Angle()`.

**Memory entries consulted:**
- `investigation_subspec_cadence.md` (cadence template).
- `cascade_theory_smoke_binding.md` (smoke binds cascade attribution).
- `smoke_unchanged_means_multiple_blockers.md` (unchanged smoke = under-diagnosed).
- `pathfinder_api_loc_aware.md` (FindPathDefault vs FindPath shape).
- `smoke_test_server_handoff.md` (smoke handoff procedure).
- `close_commit_memory_trailer.md` (close-commit trailer convention).
