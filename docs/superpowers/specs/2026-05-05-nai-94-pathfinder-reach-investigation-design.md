# NAI-94 — Pathfinder reach for through-door / blocked-passage routes (Stage 1 investigation)

**Status:** spec — investigation sub-spec (Stage 1; Stage 2 fix routes to NAI-95).
**Cadence:** `investigation_subspec_cadence` — Stage 1 audit + pinned reproducers (no production change) → Stage 2 (NAI-95) fix → smoke.
**Tech stack:** Go 1.26+.
**Upstream source:** `2004scape/rsmod-pathfinder` v5.0.4 — what `LostCityRS/Engine-TS` pins via npm. v5.0.4 is the **Rust + wasm-pack** rewrite (`371cc97 feat: rsmod rust`). Pre-Rust HEAD of the checked-out repo at `/home/owner/Code/github.com/2004scape/rsmod-pathfinder/` is the **AssemblyScript** source (commit `8ff4033`, between v5.0.0 release and the Rust rewrite). goscape's `pkg/pathfinder/routefinder/` was almost certainly ported from the AssemblyScript line (Go field shapes match AS). Audit primary: AssemblyScript HEAD source at `src/rsmod/`. Cross-reference primary: pinned-Rust v5.0.4 — reachable by `git checkout 8dd111e` in the rsmod-pathfinder repo if a Rust-vs-AS algo divergence is suspected. **Not** `LostCityRS/Engine-TS_274/src/engine/routefinder/` (per `ts_source_canonical_path`); that's a parallel TS port the LostCity production server doesn't ship.

---

## 1. Context & motivation

NAI-92 closed `PathingEntity.pathToTarget` dispatch: the wrapper layer is verified correct in smoke (waypoint_idx goes positive post-NAI-92 vs always -1 pre). The 2026-05-05 NAI-92 user-launched smoke surfaced that the *underlying* pathfinder (`pkg/pathfinder/routefinder/RouteFinder.FindPath`) returns short partial paths or no path for routes the player should reach:

- **Repro A — Survival Expert (NPC typeId=943):** player at world coord (3101, 3103) → NPC at (3103, 3095). Cheb=8. Player gets within ~6 tiles, no closer. Cabin wall in the path.
- **Repro B — Hans (NPC typeId=961):** player at (3219, 3224) → NPC at (3219, 3222). Cheb=2, straight-line, no obstacle. `waypoint_idx=-1` next tick — pathfinder returns no path on a trivial 2-tile move.

Repro B is the high-signal anomaly: a 2-tile straight-line path failing means the bug is not solely about door-flag handling — something in BFS termination, waypoint extraction, or pathfinder API plumbing is wrong even on minimum-distance paths.

Latent artifact spotted while scoping: `pkg/pathfinder/routefinder/routefinder.go:43` carries

```go
useRouteBlockerFlags bool // TODO: unused - funcs written as if false
```

This becomes hypothesis H2 below.

---

## 2. Scope

### In scope (Stage 1)

- Reproduce both anomalies as Go unit tests in `pkg/pathfinder/routefinder/` against goscape's actual `RouteFinder.FindPath` / `RouteFinder.FindPathDefault` API.
- Risk-weighted-short-circuit hypothesis audit (H1→H5; §3).
- Diagnosis report (`docs/superpowers/investigations/2026-05-05-nai-94-diagnosis.md`) with per-hypothesis verdict (confirmed / eliminated / partial / undetermined-with-reason) plus minimal reproducible test code locations.
- Update `nai_followups.md` with NAI-95 (Stage 2) handoff entry.

### Out of scope (Stage 2 / future)

- Production code changes to `routefinder.go`, `stepvalidator.go`, or any pathfinder consumer in `pkg/` or `modules/world/`.
- Lifting `t.Skip("NAI-94: ...")` markers — that is the NAI-95 success criterion.
- NPC-side pathing failures (Survival Expert *being unreachable* by player AI when no player is involved). NAI-91 followups list this as a separate residual.
- Reach-strategy / `ReachStrategy.ts` audits — only the pathfinder, not the reach layer NAI-91 ported.

---

## 3. Hypothesis register (short-circuit ordered)

| #  | Hypothesis | Probe cost | Probe shape | If confirmed |
|----|---|---|---|---|
| H1 | BFS / waypoint return is broken at minimum-distance paths | Lowest — 1 unit test | Hans-shape reproducer with empty collision flags. If `FindPath` returns empty waypoints or destination not reached, H1 fires. | De-prioritize H3/H4; focus diff on `findPath1` BFS termination + waypoint extraction. |
| H2 | `useRouteBlockerFlags` field declared but never consulted | Low — `grep` + Read | Field at `routefinder.go:43` carries `// TODO: unused`. Audit goscape BFS expansion sites against rsmod's `findPath1/2/N` for the route-blocker branches. | Likely root cause for door-blocked routes; produce minimal synthetic reproducer with `BlockMaskRouteBlocker`. |
| H3 | Ring buffer wrap (`ringBufferSize=4096`) or waypoint cap truncates valid paths before destination reached | Low-med — instrumentation in test | Survival-Expert reproducer with logging on `bufWriterIndex` wrap, distance at pop, BFS exit reason. | Fix path is bound increase or termination cond change; document at H3 verdict. |
| H4 | rsmod-pathfinder (AS) vs goscape behavioral divergence in `findPath1` 8-direction step expansion (collision-flag mask logic) | Med — line-by-line read | Diff `pkg/pathfinder/routefinder/routefinder.go` against AS `src/rsmod/PathFinder.ts:135-266` (findPath1) — register every divergent branch. If H4 surfaces nothing, spot-check Rust v5.0.4 (`git checkout 8dd111e` in rsmod-pathfinder repo, then read the Rust impl) to confirm AS↔Rust↔goscape three-way agreement on the suspect path. | Enumerate all divergences in diagnosis; NAI-95 prioritizes by smoke-impact. |
| H5 | Reach / waypoint-compaction / closest-approach returns early on blocked-near-destination paths | Low-med — read closest-approach analog | Audit goscape's "moveNear" / closest-approach helpers against rsmod's `findClosestApproachPoint` (PathFinder.ts:535-695). | Likely explains "gets within 6 tiles, no closer" Survival Expert pattern. |

**Order rationale:** H1 first because it's the cheapest probe AND closes the largest hypothesis space — if Hans cheb=2 fails, all door-flag hypotheses become subsidiary. H2 second because the artifact (`// TODO: unused`) is already evidence; verification is grep-and-read. H3 third because it's instrumentation-only on the existing reproducer. H4 is the systematic backstop. H5 covers the partial-path "gets within 6 tiles" symptom that's distinct from "no path returned."

---

## 4. Probe / reproducer matrix

| Test | Coords / fixture | Expected behavior | Disposition if reproduces |
|---|---|---|---|
| `TestNAI94_HansCheb2_StraightLineMustReach` | src=(3219,3224) → dest=(3219,3222); empty collision flags (no walls) | Last waypoint == dest; waypoints non-empty; reaches destination tile. | `t.Skip("NAI-94: documented bug, fix in NAI-95")`; pin observed behavior in skip body for diff against NAI-95 fix. |
| `TestNAI94_SurvivalExpert_BlockedPassage` | src=(3101,3103) → dest=(3103,3095); collision flags loaded from real m48_50 mapsquare (preferred) OR synthetic minimal-cabin-wall (fallback) | Path reaches dest, OR documented closest-approach within 1 tile of dest. | `t.Skip("NAI-94: …")` with current shortest-distance reached pinned in skip body. |
| `TestNAI94_RouteBlockerFlag_Consulted` | synthetic 5×5 grid: closed-door tile (`BlockMaskRouteBlocker`) between src and dest | With `useRouteBlockerFlags=false`: path reaches dest. With `=true`: blocked or routes around. Two subtests differentiate. | If both behave identical (route always passes through), H2 confirmed. |

**Test placement:** new file `pkg/pathfinder/routefinder/nai94_repro_test.go`. All tests in this file gated behind `t.Skip` if they reproduce the anomaly; passing tests stay unskipped.

**Fixture-load fallback:** if loading real m48_50 collision flags into a unit test is heavyweight (requires bringing up large parts of asset/world wiring), fall back to a synthetic minimal-cabin-wall fixture and document the simplification at the test site so NAI-95 knows it's not the real production layout.

---

## 5. Methodology — hybrid probe-then-diff

1. Write H1 reproducer (Hans cheb=2). Run. Observe.
2. **Branch on H1 result:**
   - H1 fires (path not returned) → short-circuit: focus diff on `findPath1` BFS termination + waypoint extraction in goscape vs rsmod. Skip the systematic H4 diff initially; come back to it only if H1 root cause leaves residuals.
   - H1 passes (path returned correctly) → continue to H2.
3. H2 grep audit: `rg "useRouteBlockerFlags|RouteBlocker|BlockMaskRouteBlocker" pkg/pathfinder/` vs rsmod's route-blocker handling sites.
4. H3 instrumentation on Survival Expert reproducer (test-only logging or expose internal counters via test-only helper).
5. H4 systematic line-by-line diff: register every divergent branch between `routefinder.go` and `rsmod-pathfinder/src/rsmod/PathFinder.ts`.
6. H5 closest-approach audit only if H1–H4 don't surface a smoking gun for the partial-path Survival Expert pattern.

After each hypothesis: write its verdict to the diagnosis report immediately. Don't batch.

---

## 6. Deliverables

1. **This spec** committed at `docs/superpowers/specs/2026-05-05-nai-94-pathfinder-reach-investigation-design.md`.
2. **Diagnosis report** at `docs/superpowers/investigations/2026-05-05-nai-94-diagnosis.md` — per-hypothesis verdict, file:line evidence, and either root-cause finding or explicit "diagnosis ceiling: NAI-95 needs X to break through."
3. **Reproducer tests** at `pkg/pathfinder/routefinder/nai94_repro_test.go` — skipped tests pinning observed-anomaly behavior, marked `// NAI-94:` for grep.
4. **Memory followup entry** in `nai_followups.md` under "From NAI-94" — Stage 2 (NAI-95) handoff with confirmed root cause, exact files/lines for the fix, plus any residual hypotheses NAI-95 needs to chase.

**No production code changes.** Test files + docs only.

---

## 7. Exit criteria

- All 5 hypotheses (H1–H5) have a verdict in the diagnosis report: confirmed / eliminated / partial / undetermined-with-reason.
- At least one reproducer per anomaly (Hans, Survival Expert) committed as `t.Skip` in `nai94_repro_test.go`.
- Diagnosis report identifies root cause with file:line evidence OR explicitly documents diagnosis ceiling and what NAI-95 would need to break through it.
- `nai_followups.md` updated with NAI-95 handoff.
- **No smoke gate** in Stage 1 — no production change to smoke. Stage 2 (NAI-95) will smoke.

---

## 8. Risks / known unknowns

- **rsmod-pathfinder source-flavor drift:** Engine-TS pins v5.0.4 (Rust + wasm-pack). Checked-out repo HEAD is pre-Rust AssemblyScript. goscape's port shape strongly matches AS. Risk: a Rust-rewrite-only algo fix (post-`371cc97`) is missing from the AS reference *and* from goscape. Mitigation: spot-check Rust source at `8dd111e` for any pathfinder algo paths that look unfamiliar against the AS reference, and surface as a residual if found. Type-surface sanity check available at `node_modules/@2004scape/rsmod-pathfinder/dist/rsmod-pathfinder.d.ts`.
- **Mapsquare fixture loading weight:** Survival Expert reproducer's ideal fixture is real m48_50 flags. If fixture loading drags in too much wiring for a unit test, synthetic fallback is acceptable but must be documented at the test site so NAI-95 doesn't optimize to the wrong bug.
- **H4 scope blowup:** 656-line `routefinder.go` vs 695-line `PathFinder.ts` may have multiple divergences. If so, diagnosis enumerates all; NAI-95 prioritizes by smoke-impact. Stage 1 does not commit to fixing all divergences in NAI-95.
- **Subagent fabrication risk** (per `audit_subagent_fabrication`, `verify_implementer_claims`): if any audit task is delegated, controller independently verifies every claimed file:line citation with `git show` / `rg` / `Read` before writing it into the diagnosis report. Wire-format-style fabrications are explicitly cited in NAI-31; pathfinder algo audits have similar fabrication surface.

---

## 9. Cadence & memory references

- `investigation_subspec_cadence` — Stage 1 audit + reproducers; Stage 2 (NAI-95) fix; Stage 3 conditional on smoke. NAI-31 first instance, NAI-90 second precedent.
- `ts_source_canonical_path` — only `LostCityRS/Engine-TS` is the porting reference; `Engine-TS` pins `@2004scape/rsmod-pathfinder` v5.0.4. NOT `Engine-TS_274/src/engine/routefinder/`.
- `rust_source_canonical_path` — analogue: rsmod-pathfinder v5.0.4 is Rust. For NAI-94, AS HEAD is the primary reference (matches goscape's port shape); Rust is a secondary cross-check.
- `disasm_reframes_inferred_binding` — predecessor-binding precedent; for pathfinder, the analogous static-confirm step is reading rsmod's algo before writing the spec.
- `dispatch_correct_reach_blocked` — explicit precedent (NAI-91→NAI-92): dispatch-layer correct does not imply downstream engine correct. NAI-92 verified dispatch correct; NAI-94 audits the downstream pathfinder.
- `verify_implementer_claims` — controller pre-flight + post-each-task verification; 30-second protocol.
- `audit_subagent_fabrication` — wire-format / algorithm audits can return fabricated diagnoses; verify before code change.
- `smoke_surfaces_adjacent_divergences` — NAI-92 smoke surfaced this residual; NAI-94 closes the audit branch.
- `close_commit_memory_trailer` — NAI-94 close commit carries `Closes memory: ...` trailer.

---

## 10. Stage 2 handoff template (for NAI-95)

When Stage 1 closes, populate this in the NAI-94 close commit / followup memory entry:

- **Root cause:** _[file:line + 1-2 sentence summary]_
- **Repro tests to lift skip on:** _[list of tests + expected post-fix behavior]_
- **Files NAI-95 will touch:** _[exact list]_
- **Estimated LOC for fix:** _[ballpark]_
- **Residual hypotheses for NAI-96+:** _[any H4 divergences not in NAI-95 scope]_
