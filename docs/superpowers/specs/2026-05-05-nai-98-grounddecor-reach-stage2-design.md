# NAI-98 — GroundDecor reach abandonment Stage 2 fix (narrow-then-fix)

**Status:** spec — Stage 2 fix sub-spec. Single doc, two-phase, with explicit mid-spec plan-amendment checkpoint.
**Cadence:** `investigation_subspec_cadence` extended — Phase 1 narrowing probe (real-cache integration test) → controller plan-amendment checkpoint → Phase 2 sub-H-conditional surgical fix → user-launched smoke binds the close.
**Tech stack:** Go 1.26+.
**Upstream source:** `LostCityRS/Engine-TS` (per `ts_source_canonical_path`). For pathfinder predicate questions, `2004scape/rsmod-pathfinder` branch 225 (per `rust_source_canonical_path`).

---

## 1. Context & motivation

NAI-97 closed Stage 1 at diagnosis ceiling (`docs/superpowers/investigations/2026-05-05-nai-97-diagnosis.md`). Verdicts:

- **H1–H4 ELIMINATED.** Decoder, PostDecode, ChangeLocCollision dispatch + GroundDecor write side, and `interaction.go` SMART pathToTarget arms all match TS Engine-TS byte-for-byte.
- **H5 PARTIAL — DIAGNOSIS-CEILING.** Tickloop call-graph mapped (`movement.go:96` CanTravel-fail; `interaction.go:255-258` "I can't reach that"; `interaction.go:236-239` repathed-once gate); no static-readable mutator surfaced as a smoking gun.
- **Empty-grid reproducers** (`pkg/pathfinder/routefinder/nai97_repro_test.go`) skip-pinned with documented degenerate symptom (`{Waypoints:[] Alternative:true Success:true}` from `findClosestApproachPoint` source-tile fallback under `moveNear=true` per `empty_flagmap_degenerate_routefinder`).

NAI-97 surfaced three sub-hypotheses that static audit could not disambiguate:

- **Sub-H6:** `pkg/pathfinder/routefinder/routefinder.go` BFS expansion or `pkg/pathfinder/reach.Reached` predicate divergent from the Rust `2004scape/rsmod-pathfinder` source.
- **Sub-H7:** StepValidator (`pkg/gamemap/gamemap.go:97-101` `CanTravel`) per-step predicate disagrees with the BFS's CanMove predicate (per-direction `clipFlag` selection at `pkg/pathfinder/routefinder/routefinder.go:187, 197`); BFS produces a tile that StepValidator rejects.
- **Sub-H8:** `interaction.go:236-239` repathed-once-per-interaction gate is correct vs TS but combined with a short-or-empty initial path produces "walked partial path then abandons" smoke shape.

Smoke residuals that NAI-98 must close:

- **Repro A** — NPC type 943 at (3218, 3216), player at (3221, 3218), cheb=3. Smoke trace: tick N `waypoint_idx=1, repathed=true, steps_taken=0` → tick N+1 `waypoint_idx=-1, target_still_set=false`.
- **Repro B** — NPC type 3 at (3223, 3216), player at (3218, 3213), cheb=5. Smoke trace: player closes 7→5 then `waypoint_idx=-1` abandons mid-route at (3218, 3213).

---

## 2. Scope

### In scope

- **Phase 1 (pre-authored, codified in this spec + plan):** real-cache integration test in `pkg/gamemap` that runs a three-signal H6/H7/H8 probe over the smoke geometry. Test pins which sub-hypothesis fires.
- **Plan-amendment checkpoint (controller, not subagent):** read Phase 1 output, re-grep premises against HEAD, diff against the relevant upstream source (TS for H8; Rust `rsmod-pathfinder` branch 225 for H6/H7), draft Phase 2 task block.
- **Phase 2 (drafted post-Phase-1, codified in plan-amendment):** surgical fix per surfaced sub-H + sub-H-specific regression test. Production code changes constrained to the file set listed in §5 per surfaced sub-H.
- **Repro test bookkeeping:** `pkg/pathfinder/routefinder/nai97_repro_test.go` deleted at Phase 2 close. Real-cache probe in `pkg/gamemap` is the durable regression test.
- **User-launched smoke binding:** Repro A + Repro B over real client; binds the close per `cascade_theory_smoke_binding`.
- **Memory hygiene:** rename `FindPathDefault` → `FindPathPlain` in `pathfinder_api_loc_aware.md` (carry-forward from NAI-97 §1 pre-flight).

### Out of scope

- The openbankdoor_l symptom (parked DISPLAYNAME opcode 2016 residual; `nai_followups`).
- Other NAI-92/NAI-94 follow-up tracker residuals (LOC_FINDALLZONE 3008, TUT_FLASH 2121, Survival Expert reach at 3214,3205, chatplayer_page DISPLAYNAME 2016).
- `routefinder.go:110` QF1003 staticcheck (informational; only addressed if Phase 2 touches the file).
- Hypotheses outside {H6, H7, H8}: if Phase 1 surfaces something else, escalate to user before Phase 2 (§3 step 2).

---

## 3. Phase 1 — real-cache three-signal probe

### 3.1 Test layout

**File:** `pkg/gamemap/nai98_realcache_probe_test.go` (new). Sibling to `pkg/gamemap/nai97_loc_walk_test.go` (Stage 1.1 dump).

**Why `pkg/gamemap` and not `pkg/pathfinder/routefinder`:** the test calls `gm.Init(cacheDir)` + `objtype.LoadLocTypes` + replays the production `populateStaticLocsIntoZones` collision-write loop + then exercises `gm.Pathfinder.FindPathToEntity` and walks the produced waypoints through `gm.CanTravel`. `pkg/pathfinder/routefinder` cannot import `pkg/gamemap` (cycle: `pkg/gamemap` → `pkg/pathfinder/routefinder`).

**Test functions:**

- `TestNAI98_RealCacheReachProbe_NPC943` — Repro A, src=(3221,3218) → dst=(3218,3216).
- `TestNAI98_RealCacheReachProbe_NPC3` — Repro B, src=(3218,3213) → dst=(3223,3216).

Both call a shared helper `runRealCacheReachProbe(t *testing.T, srcX, srcZ, dstX, dstZ int)`.

### 3.2 Per-probe flow

1. **Setup.** Skip if `data/pack/server/maps/m48_50` or `data/pack/server/loc.dat` unavailable (mirror `nai97_loc_walk_test.go`). Construct `gm := New(slog.New(slog.NewTextHandler(io.Discard, nil)))`. Call `gm.Init(cacheDir)`. Load `cfgs := objtype.LoadLocTypes(cacheDir)`.
2. **Replay `populateStaticLocsIntoZones` GLOBALLY.** Iterate `gm.StaticLocs()`; for each `lt := cfgs.Configs[l.Type()]`, if `lt != nil && lt.BlockWalk`, call `gm.ChangeLocCollision(l.Shape(), l.Angle(), lt.BlockRange, l.Length, l.Width, lt.Active, l.X, l.Z, l.Level, true)`. **Do not bbox-restrict** — the FlagMap must match production exactly so that BFS expansion sees real flags at every reachable tile.
3. **Signal H6 — BFS / reach predicate.** Call `route := gm.Pathfinder.FindPathToEntity(0, srcX, srcZ, dstX, dstZ, 1, 1, 1)`. If `!route.Success || len(route.Waypoints) == 0`, fail with `"H6 FIRES: FindPathToEntity returned no path on real-cache geometry. Route=%+v"`. If last waypoint cheb-distance > 1 from `(dstX, dstZ)`, fail with `"H6 FIRES: last waypoint=(%d,%d) cheb=%d > 1 from dst=(%d,%d). Route=%+v"`.
4. **Signal H7 — StepValidator divergence.** With H6 passed, walk `route.Waypoints` from source. **Critical:** BFS waypoints are direction-change points (per `pkg/pathfinder/routefinder/routefinder.go:130-153`: appends only when `currDir != nextDir`), NOT one tile apart — a straight stretch of N tiles produces 1 waypoint at its end. The H7 probe must therefore step single tiles along each (prev, curr) straight segment.

   Algorithm: for each `(prev, curr)` waypoint pair (prev = `(srcX, srcZ)` for the first segment; prev = previous waypoint thereafter), compute `dx := curr.X() - prev.X(); dz := curr.Z() - prev.Z()`. Compute step deltas `sx := sgn(dx); sz := sgn(dz)` (each in {-1, 0, 1}). Walk from prev toward curr in single-tile steps: at each step, call `gm.CanTravel(0, x, z, sx, sz)` (actual signature per `pkg/gamemap/gamemap.go:97-100`). Any false → fail with `"H7 FIRES at sub-step (%d,%d)→(%d,%d) inside segment %d/%d (waypoint (%d,%d)→(%d,%d)) step=(%d,%d) but CanTravel=false. Route=%+v"`. The full waypoint stretch must walk to (curr.X(), curr.Z()) inclusive before advancing to the next segment.

   **Plan-author note:** loop guard must use `for x != curr.X() || z != curr.Z()` (NOT a fixed step count). If sx and sz are BOTH zero (degenerate same-tile waypoint), that's an unexpected route shape — skip-pin and escalate per §3.3 special case.
5. **Signal H8 — by elimination.** If H6 + H7 pass, log `t.Logf("H8 FIRES by elimination on (%d,%d)→(%d,%d): BFS path internally consistent (%d waypoints) and StepValidator-walkable. Phase 2 must investigate tickloop-level state mutation in modules/world/.", srcX, srcZ, dstX, dstZ, len(route.Waypoints))`. Test passes (no failure marker; `t.Logf` for plan-amendment input).

### 3.3 Phase 1 commit deliverable

Single Phase 1 task: write `pkg/gamemap/nai98_realcache_probe_test.go`, run with `-v`, capture output. Commit lands the test; **the commit is expected to FAIL CI** if H6 or H7 fires (these are `t.Fatalf` paths). If H8 fires, commit passes CI. Either way, the controller reads the test output verbatim from the commit's run as input to the plan-amendment.

**Special case — Phase 1 surfaces a hypothesis NOT in {H6, H7, H8}.** If, against expectation, the FlagMap is empty post-replay (production loader not exercising as expected), or the test panics, or the route shape is `{Waypoints:[…non-empty…] Alternative:false Success:true}` with last waypoint INSIDE cheb=1 of dst (i.e. matches expected behavior — no bug reproduces in-test), wrap the test in `t.Skip(...)` with verbatim `%+v` Route + flag dump pinned, halt Phase 2 plan-amendment, escalate to user. (Per `audit_subagent_fabrication`: don't infer Phase 2 scope from undocumented findings.)

---

## 4. Plan-amendment checkpoint (controller-driven)

After Phase 1 commit lands, **controller** (main session, NOT a subagent) executes the following before dispatching Phase 2:

1. **Read Phase 1 test output verbatim** from a fresh `go test -run TestNAI98_RealCacheReachProbe -v ./pkg/gamemap/...` run at the Phase 1 commit (per `verify_implementer_claims`: do not trust the implementer's commit message; rerun).
2. **Confirm sub-H surfaced is in {H6, H7, H8}.** If not, halt and escalate to user.
3. **Re-grep premises against HEAD** per `controller_preflight`:
   - Sub-H6: `pkg/pathfinder/routefinder/routefinder.go:107, 117-153, 165-180`.
   - Sub-H7: `pkg/gamemap/gamemap.go:97-101` `CanTravel`; BFS per-direction `clipFlag` at `pkg/pathfinder/routefinder/routefinder.go:187, 197`.
   - Sub-H8: `modules/world/interaction.go:87, 135, 171-294, 236-239, 255-258, 454`; `modules/world/movement.go:34-115`.
4. **Cross-reference upstream source for the surfaced sub-H:**
   - Sub-H6: diff goscape predicate against `2004scape/rsmod-pathfinder` branch 225 at the surfaced site. Capture divergence shape verbatim.
   - Sub-H7: diff goscape `CanTravel` and BFS clip-flag selection against the Rust source. Identify which side disagrees with Rust and align goscape to it.
   - Sub-H8: diff goscape `interaction.go` repathed-gate against TS `LostCityRS/Engine-TS/src/engine/entity/PathingEntity.ts` and any tickloop counterpart in TS `Player.ts` / `Npc.ts`.
5. **Draft Phase 2 task block.** Append directly to `docs/superpowers/plans/2026-05-05-nai-98-...md` (Edit only, NOT replace_all per `plan_doc_replaceall_timeline`). One task per identified divergence; per-task code blocks codified per the plan's existing style; each task includes verbatim before/after snippets sourced from `Read` at HEAD (not inferred).
6. **Pre-flight Phase 2 task premises** per `controller_preflight` (file paths, line numbers, signatures, helper init state).
7. **Dispatch Phase 2 implementer(s)** subagent-driven-development per `execution_mode_default`. Use `superpowers:code-reviewer` on Sonnet (not Opus per `superpowers_code_reviewer_model`) for the post-implementer review.

**Failure mode the spec must guard against:** controller skips re-grep / upstream-source diff and dispatches Phase 2 on stale premises. The above checklist is HARD prerequisite — no Phase 2 dispatch without it.

---

## 5. Phase 2 fix scope (sub-H-conditional)

Spec enumerates fix shape per surfaced sub-H but does **not** codify per-task plan content (that's the plan-amendment's job).

### 5.1 If H6 fires (BFS / reach predicate divergent)

- **Likely sites:** `pkg/pathfinder/routefinder/routefinder.go` (BFS `routeFindSize1` at `:165-180`, `findClosestApproachPoint` at `:117-124`); possibly `pkg/pathfinder/reach/`.
- **Diff target:** `2004scape/rsmod-pathfinder` branch 225.
- **Estimated LOC:** 5–50 (predicate bit-flag mismatch on the small end; BFS expansion semantics rework on the large end).
- **Phase 2 regression test:** per-predicate unit test in `pkg/pathfinder/routefinder/`. Fixture allocated via `internal.BuildCollisionMap` per `empty_flagmap_degenerate_routefinder` to avoid empty-FlagMap degenerate symptom.

### 5.2 If H7 fires (StepValidator vs BFS CanMove divergence)

- **Likely sites:** `pkg/gamemap/gamemap.go:97-101` (`CanTravel`); BFS per-direction `clipFlag` selection at `pkg/pathfinder/routefinder/routefinder.go:187, 197`.
- **Diff target:** Rust `rsmod-pathfinder` branch 225; align goscape such that BFS-CanMove and StepValidator-CanTravel agree on every (src, dst, dir, flagPattern) tuple.
- **Estimated LOC:** 5–15.
- **Phase 2 regression test:** unit test that constructs a `FlagMap` with a known flag pattern (via `internal.BuildCollisionMap`), runs BFS over it, walks the result through `CanTravel`, asserts agreement on every step.

### 5.3 If H8 fires (repathed-once + tickloop ordering)

- **Likely sites:** `modules/world/interaction.go:236-239` repathed gate; `modules/world/movement.go:34-115` resolveMovement → stepOnce; `modules/world/interaction.go:255-258` "I can't reach that" mutator; `modules/world/interaction.go:454` defaultOp NIH fallback.
- **Diff target:** TS `LostCityRS/Engine-TS/src/engine/entity/PathingEntity.ts` repathed-equivalent + tickloop ordering in TS `Player.ts` / `Npc.ts` (per `ts_base_class_read_for_inherited_behavior` — read both leaf and base).
- **Estimated LOC:** 5–30.
- **Phase 2 regression test:** `modules/world/`-level test that mocks Player + Interaction + drives ticks; asserts `waypointIndex` and `target` lifecycle across the smoke trace.

### 5.4 Common to all three sub-Hs

- `pkg/pathfinder/routefinder/nai97_repro_test.go` — **deleted** at Phase 2 close. Empty-grid degenerate per `empty_flagmap_degenerate_routefinder`; never going to pass; superseded by `pkg/gamemap/nai98_realcache_probe_test.go`. Phase 2 close commit message documents the deletion + reasoning.
- The Phase 1 real-cache probe (`nai98_realcache_probe_test.go`) remains as the durable regression test. Post-fix, the H6/H7 `t.Fatalf` paths must not fire and the H8 `t.Logf` must not fire (or be re-purposed if H8 was the surfaced sub-H — plan-amendment specifies exact post-fix expected output).

---

## 6. Smoke gate (binds the close)

Per `cascade_theory_smoke_binding`, smoke binds the close.

**Smoke is user-launched** per `smoke_test_server_handoff` — Claude's sandboxed server is unreachable from the Java client.

**Smoke acceptance, Repro A:** player at (3221, 3218), level 0, clicks NPC type 943 at (3218, 3216). Player walks the path AROUND the Lumbridge fountain GroundDecor and contacts the NPC. No "I can't reach that". No mid-route abandonment.

**Smoke acceptance, Repro B:** player at (3218, 3213), level 0, clicks NPC type 3 at (3223, 3216). Player walks the cheb=5 path and contacts the NPC. No mid-route abandonment.

**Smoke routing rules:**

- Both repros pass → close NAI-98.
- One passes, one fails (same shape as pre-fix on the failure) → analyse: same-shape failure on a different geometry implies the surfaced sub-H wasn't the only blocker; reopen as NAI-99 with refined sub-H taxonomy (per `smoke_unchanged_means_multiple_blockers`).
- One passes, one fails (different shape) → cascade-adjacent failure; route per `smoke_surfaces_adjacent_divergences` (in-scope-stretch if ≤30 LOC, else NAI-99).
- Both fail with same shape as pre-fix → brainstorm under-diagnosed; either spec amendment within NAI-98 (if a fourth sub-H surfaces from re-running Phase 1 + new evidence) or NAI-99 reopen.

---

## 7. Deliverables

1. **This spec** at `docs/superpowers/specs/2026-05-05-nai-98-grounddecor-reach-stage2-design.md`.
2. **Plan** at `docs/superpowers/plans/2026-05-05-nai-98-grounddecor-reach-stage2.md` — Phase 1 task pre-authored; Phase 2 task block stub-out with explicit "controller plan-amendment fills this in after Phase 1 commit" placeholder.
3. **Phase 1 test** at `pkg/gamemap/nai98_realcache_probe_test.go` — three-signal probe per §3.
4. **Phase 2 fix** — sub-H-conditional production code change; codified at plan-amendment time.
5. **Phase 2 regression test** — sub-H-conditional; codified at plan-amendment time.
6. **Repro test cleanup** — `pkg/pathfinder/routefinder/nai97_repro_test.go` deleted at Phase 2 close.
7. **Memory updates:**
   - Rename `FindPathDefault` → `FindPathPlain` in `pathfinder_api_loc_aware.md`.
   - New `nai_followups.md` § "From NAI-98" entry with cascade-attribution + any deferred-to-NAI-99 items.
   - New entry per surfaced sub-H if non-obvious lesson surfaces (e.g. predicate bit-flag mismatch class, tickloop-ordering invariant).
   - Update `nai_96_grounddecor_path_around_residual.md` to closed status, OR rename / supersede via new entry.
8. **Close commit** carries `Closes memory:` trailer per `close_commit_memory_trailer`.

---

## 8. Exit criteria

- Phase 1 test commits and pins which sub-H fires (H6/H7/H8), OR escalates to user via skip-pin if hypothesis falls outside {H6,H7,H8}.
- Plan-amendment checkpoint completes: re-grep + upstream-source diff + Phase 2 task draft committed to plan doc.
- Phase 2 fix commits with sub-H-specific regression test passing.
- `pkg/gamemap/nai98_realcache_probe_test.go` post-fix: no `H6 FIRES` / `H7 FIRES` / `H8 FIRES` (if H8 was surfaced and fixed, the `t.Logf` line documents the fix shape; plan-amendment specifies exact post-fix output).
- `pkg/pathfinder/routefinder/nai97_repro_test.go` deleted.
- User-launched smoke confirms Repro A + Repro B both pass per §6 acceptance shapes.
- `nai_followups.md` updated with NAI-98 close + any NAI-99 residuals.
- All goscape tests pass: `go test ./...`.

---

## 9. Risks / known unknowns

- **Phase 1 surfaces a hypothesis outside {H6,H7,H8}.** Mitigation: §3.3 skip-pin discipline; user escalation halts Phase 2.
- **Mid-spec plan-amendment introduces stale premises** (per `plan_doc_replaceall_timeline`). Mitigation: controller uses Edit not replace_all; per-task pre-flight per `controller_preflight` before Phase 2 dispatch.
- **Real-cache fixture flakiness.** Mitigation: mirrored against `nai97_loc_walk_test.go`'s known-passing pattern; if drift surfaces, hold Phase 1 and reconcile against the existing test.
- **Phase 2 fix lands but smoke unchanged** (per `smoke_unchanged_means_multiple_blockers`). Mitigation: §6 routing rules; NAI-99 reopen if both repros fail post-fix with same shape.
- **`route.Waypoints` semantics confirmed at spec-write time** (per `pkg/pathfinder/routefinder/routefinder.go:130-153` Read): waypoints are direction-change points; closest-to-source first, dest-side last. Source itself is NOT in `route.Waypoints`. §3.2 Step 4 H7 algorithm stitches single-tile sub-steps within each segment, not per waypoint.
- **Audit subagent fabrication risk** (per `audit_subagent_fabrication`, `verify_implementer_claims`). Mitigation: controller-side independent verification of Phase 1 test output + Phase 2 fix premises. Pathfinder/algorithm audits and TS/Rust source comparisons have known fabrication surface.
- **CI failure on Phase 1 commit.** Phase 1 is *expected* to fail CI when H6/H7 fires. Mitigation: commit message explicitly notes "Phase 1 narrowing test — failure is the diagnostic signal for plan-amendment; do not revert"; controller does not gate on green CI for the Phase 1 commit.

---

## 10. Cadence & memory references

- `investigation_subspec_cadence` — Stage 1 audit + reproducers (NAI-97); Stage 2 fix (NAI-98). NAI-98 extends with two-phase narrow-then-fix structure.
- `ts_source_canonical_path` — only `LostCityRS/Engine-TS` is the porting reference for goscape.
- `rust_source_canonical_path` — `2004scape/rsmod-pathfinder` branch 225 is the Rust reference for `pkg/pathfinder/routefinder` and `pkg/pathfinder/reach`.
- `empty_flagmap_degenerate_routefinder` — Phase 2 regression tests for H6/H7 must allocate via `internal.BuildCollisionMap`; Phase 1 uses production `gm.Init` + replay.
- `cascade_theory_smoke_binding` — smoke binds the close.
- `smoke_unchanged_means_multiple_blockers`, `smoke_surfaces_adjacent_divergences` — smoke routing rules at §6.
- `dispatch_correct_reach_blocked` — NAI-91→NAI-92 precedent for dispatch-correct-vs-downstream-blocked split; H8 is the analogous case for NAI-98.
- `audit_subagent_fabrication`, `verify_implementer_claims`, `controller_preflight` — controller verification gates at plan-amendment.
- `plan_doc_replaceall_timeline` — Edit-not-replace_all when amending plan doc.
- `execution_mode_default` — subagent-driven-development for Phase 2 dispatch.
- `superpowers_code_reviewer_model` — Sonnet for code-reviewer agent.
- `close_commit_memory_trailer` — close commit carries `Closes memory:` trailer.
- `superpowers_clear_between_spec_and_impl` — after plan lands, emit resume prompt and stop; user `/clear`s before Phase 1 implementer dispatch.
- `pathfinder_api_loc_aware` — **stale**: refers to `FindPathDefault` (renamed to `FindPathPlain`). Rename during NAI-98 close.
- `nai_96_grounddecor_path_around_residual` — NAI-98 closes this followup.

---

## 11. Phase-amendment checkpoint contract (for the plan doc)

When the plan doc is written, the Phase 2 task block stub-out must read:

```
## Phase 2 — sub-H-conditional surgical fix

**STATUS: PLACEHOLDER — controller fills in after Phase 1 commit per
spec §4 plan-amendment checkpoint.**

Tasks below are DRAFTED post-Phase-1, not at plan-author time. Controller
verifies sub-H surfaced ∈ {H6, H7, H8}, re-greps premises, diffs against
upstream source per spec §4 step 4, then appends per-task code blocks
following the plan's existing style.

Before any Phase 2 task dispatches, controller must complete spec §4
steps 1-7. Phase 2 dispatch on stale premises is the explicit failure
mode this checkpoint exists to prevent.
```

This stub-out IS the deliverable for that section at plan-write time. Phase 2 tasks land via plan amendment after Phase 1 commit.
