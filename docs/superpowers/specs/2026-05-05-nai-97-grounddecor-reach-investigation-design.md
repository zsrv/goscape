# NAI-97 — GroundDecor over-blocking + reach abandonment investigation (Stage 1)

**Status:** spec — investigation sub-spec (Stage 1; Stage 2 fix routes to NAI-98).
**Cadence:** `investigation_subspec_cadence` — Bundle 0 controller pre-flight (no commits) → Bundle 1 Stage 1 risk-weighted-short-circuit audit + reproducer pins → no Stage 2 in NAI-97 → smoke handoff to NAI-98.
**Tech stack:** Go 1.26+.
**Upstream source:** `LostCityRS/Engine-TS` (per `ts_source_canonical_path`). For pathfinder questions, also `2004scape/rsmod-pathfinder` AS HEAD (per NAI-94 §risks).

---

## 1. Context & motivation

NAI-96 closed pkg/gamemap/ collision-write divergences at `79c8ec4`. User-launched smoke (2026-05-05 ~12:18 EDT) confirmed:

- ✅ NPC straight-line paths work (BLOCK_MAP_SQUARE flip + LINK_BELOW level adjustment)
- ✅ Walking INTO Lumbridge fountain blocked (GroundDecor active=1 → ChangeFloor writes `FlagBlockWalk`)
- ❌ Pathing AROUND GroundDecor fails: clicking on NPC/door with GroundDecor between source and destination produces "I can't reach that" or mid-route abandonment.

**Anomaly set (after dropping `openbankdoor_l` per Q3.5; that symptom traces to the parked DISPLAYNAME opcode 2016 residual, not collision/reach):**

- **Repro A — NPC type 943 at (3218, 3216), player at (3221, 3218), cheb=3.** Smoke trace: tick N `waypoint_idx=1, repathed=true, steps_taken=0` → tick N+1 `waypoint_idx=-1, target_still_set=false`. Pathfinder *did* return a waypoint; tickloop dropped it without consuming a step.
- **Repro B — NPC type 3 at (3223, 3216), player at (3218, 3213), cheb=5.** Smoke trace: player closes 7→5 then `waypoint_idx=-1` abandons mid-route at (3218, 3213). Same abandonment-without-completion shape; not "no path returned."

**Why the cause space is layered:** Goscape pre-NAI-96 wrote NO GroundDecor collision; pathfinder didn't need to route around them. Post-NAI-96, GroundDecor active=1 correctly writes `FlagBlockWalk` via `ChangeFloor`; pathfinder must route around. The path-around-ness is new; failure modes could live in the collision-write side (over-blocking), the pathfinder API call-site routing, or the post-FindPath tickloop consumption of the produced waypoint.

**Pre-flight observations (Bundle 0 controller probe):**

- `pkg/pathfinder/routefinder/api.go:40` — `FindPathPlain` is the renamed `FindPathDefault`; shape-blind 1×1 wrapper. Memory `pathfinder_api_loc_aware` is stale on the name.
- `pkg/pathfinder/routefinder/api.go:54` — `FindPathToLoc` threads `destWidth/destLength/angle/shape/blockAccessFlags` (shape-aware).
- `pkg/pathfinder/routefinder/api.go:47` — `FindPathToEntity` threads `srcSize/destWidth/destLength` (shape-aware for entities, opaque destination shape).
- `modules/world/interaction.go:623/642/659/670` — interaction dispatch already routes **shape-aware** by target class: Loc → `FindPathToLoc`; PathingEntity → `FindPathToEntity` (or `FindNaivePath` shortcut on bbox-intersect); Obj → `FindPathPlain` (TS plain `findPath`). H4 must therefore be reframed: not "API is shape-blind," but "are the existing dispatch arms correct vs TS?"
- `pkg/objtype/loctype.go:166-176` — `PostDecode` coerces `Active=-1` to 0/1 based on Shapes/Op presence; line 183 default `BlockWalk: true`.
- `pkg/gamemap/gamemap.go:61` — `ChangeLocCollision(shape, angle, blocksRange, length, width, active, x, z, level, add)` dispatches per-layer; `LayerGroundDecor` branch writes `FlagBlockWalk` via `ChangeFloor` when active==1.

---

## 2. Scope

### In scope (Stage 1)

- Reproduce both anomalies (Repro A, Repro B) as Go tests against the actual pathfinder + collision wiring, with `t.Skip("NAI-97: …")` pins where reproduction succeeds.
- Risk-weighted-short-circuit hypothesis audit (H1→H5; §3).
- Diagnosis report at `docs/superpowers/investigations/2026-05-05-nai-97-diagnosis.md` with per-hypothesis verdict and file:line evidence.
- `nai_followups.md` update with NAI-98 (Stage 2) handoff.

### Out of scope (Stage 2 / future)

- **Production code changes** to `pkg/objtype/loctype.go`, `pkg/gamemap/gamemap.go`, `pkg/pathfinder/routefinder/`, or any consumer in `modules/world/`. Stage 2 (NAI-98) fixes.
- Lifting any `t.Skip("NAI-97: …")` markers; lifting them is the NAI-98 success criterion.
- The `openbankdoor_l` symptom (parked DISPLAYNAME opcode 2016 residual; out-of-band per `nai_followups`).
- Other parked smoke residuals from NAI-92/NAI-94 follow-up tracker (LOC_FINDALLZONE 3008, TUT_FLASH 2121, Survival Expert reach at 3214,3205, chatplayer_page DISPLAYNAME 2016).

---

## 3. Hypothesis register (risk-weighted-short-circuit ordered)

| # | Hypothesis | Probe cost | Probe shape | If confirmed |
|---|---|---|---|---|
| H1 | TS-divergent over-blocking — `LocType.BlockWalk` default (`loctype.go:183`) or per-LocType decode mismatches TS for some IDs that write `FlagBlockWalk` near smoke coords | Med — fixture load + enumeration | **Stage 1.1**: load real m48_50 collision via the production loader; for tiles in a small bbox around (3218,3216) and (3223,3216), enumerate `(layer, locID, locTypeID, locTypeName, BlockWalk, Active)` per loc + read `FlagMap` flags. **Stage 1.2**: cross-reference each enumerated LocType against TS `LocType.ts` `blockwalk` field. | Per-LocType `BlockWalk` divergence in goscape decoder; NAI-98 fixes in `loctype.go`. |
| H2 | `LocType.Active` PostDecode coercion catches decorative locs (`loctype.go:166-176`: `Active=-1` → `1` when `Op != nil` or shape==10-only). Many decorative GroundDecor have right-click `Op` → `Active=1` → `ChangeLocCollision` writes block | Low — Read + per-ID compare | **Stage 1.3**: for each LocType enumerated in H1 with `Active=1` post-decode, run the same raw cache bytes through TS `LocType.postDecode` (LocType.ts:202-214 per goscape comment). Pin Active divergences. | Tighten Active=1 conditions in goscape `PostDecode` to match TS. |
| H3 | `LayerGroundDecor` write-side divergence vs TS `GameMap.ts` — NAI-96 pinned `LayerGround` width/length angle-swap (ff799be/49e70b7); `LayerGroundDecor` branch + `ChangeFloor` are single-tile, untouched. TS may swap, may write blockrange, may use a different angle offset. | Low — code diff | **Stage 1.4**: read `pkg/gamemap/gamemap.go` `LayerGroundDecor` branch + `ChangeFloor` callees vs TS `GameMap.ts changeLocCollision` GroundDecor branch; enumerate every divergence. | Per-divergence fix in `pkg/gamemap/gamemap.go`. |
| H4 | Pathfinder API call-site routing mismatch — `modules/world/interaction.go` dispatch arms (Loc → `FindPathToLoc`, Entity → `FindPathToEntity`, Obj → `FindPathPlain`) might have wrong arm or wrong args for the smoke target classes. Reframed from `pathfinder_api_loc_aware` (stale name) | Low-med — Read + grep | **Stage 1.5**: read `interaction.go:571-680` dispatch + `pathing.go` interface; trace what Repro A / Repro B target classes (NPC = PathingEntity) actually invoke. Cross-reference against TS `Interaction.ts` `getInteractionPath` equivalent. Verify `FindPathToEntity` `destWidth/destLength` reflect actual NPC dimensions. | Per-arm fix at the dispatch site in `interaction.go`. |
| H5 | Post-FindPath waypoint discard / interaction-state reset — pathfinder returns waypoint, tickloop drops it before stepping. Smoke trace evidence: NPC 943 tick N `waypoint_idx=1, steps_taken=0` → tick N+1 `waypoint_idx=-1, target_still_set=false` | Med — grep + read tickloop | **Stage 1.6**: grep `modules/world/` for `waypoint_idx` writers + `repathed` flag handling + `target` clearers; trace tickloop ordering between pathfinder-return and step-consume; identify mutator that resets state mid-tick or on next-tick re-entry without honoring the produced path. | Fix at the interaction/tickloop site in `modules/world/`. |

**Order rationale:**

- Stage 1.1 (H1 enumeration) produces input that Stage 1.3 (H2) consumes. Run first.
- Stage 1.4 (H3 code-diff) is independent of the enumeration and can run in parallel; placed after 1.3 in the linear order because its yield is conditional on H1/H2 not already explaining the smoke.
- H1 / H2 / H3 are **collision-write-side** hypotheses: if any confirms an over-block at a tile that should be open, the pathfinder behavior is downstream-correct and H4 / H5 close as eliminated.
- H4 / H5 are **post-write-side** hypotheses; cheaper to defer until write-side verdict is in.
- H5 last because abandonment-without-stepping has the highest narrowing cost and is most likely to require modules/world tickloop reading rather than test reproduction.

**Short-circuit policy:** each substage's verdict appended to the diagnosis doc immediately. If H1 surfaces a smoking-gun BlockWalk diff for a LocType blocking the smoke path, controller decides whether to continue H2/H3 enumeration (for completeness) or close at H1.

---

## 4. Probe / reproducer matrix

| Test | Location | Shape | Skip disposition |
|---|---|---|---|
| `TestNAI97_LocWalkDump_Lumbridge` | new `pkg/gamemap/nai97_loc_walk_test.go` | Loads real m48_50 collision via the production loader; for tiles in bbox `(3215..3225, 3214..3220, level=0)`, dumps `(x, z, layer, locID, locTypeID, locTypeName, BlockWalk, Active, FlagMap-flag)` per loc to `t.Log`. **No assertions** — output captured into the diagnosis doc as Stage 1.1 input. | Always passes; dump-only. |
| `TestNAI97_NPC943_PathAround` | new `pkg/pathfinder/routefinder/nai97_repro_test.go` | Path from src=(3221,3218) → dest=(3218,3216), level=0, srcSize=1, destWidth=1, destLength=1 (NPC dims), against the same fixture as the dump test. Asserts last waypoint reaches dest (cheb<=1) and waypoint count > 0. | If reproduces, `t.Skip("NAI-97: NPC reach abandonment around GroundDecor — observed waypoints=…")` pinning observed shape. |
| `TestNAI97_NPC3_PathAround` | same file | Path from src=(3218,3213) → dest=(3223,3216). Same assertion shape. | Same skip disposition. |
| `TestNAI97_TickloopAbandon_NPC943` | new `modules/world/nai97_abandon_test.go` *if* a reachable test seam exists between `pf.FindPathToEntity` return and step-consume; **otherwise H5 is grep+read only**, with diagnosis text capturing tickloop call-graph in lieu of a runtime test. | Tick N: produce a waypoint via the pathfinder. Tick N+1 with no environmental change: assert `waypoint_idx` does not regress to -1 unless destination reached or `target` cleared by a state mutator we can name. | If reproduces, `t.Skip("NAI-97: …")`. If no test seam, omit. |

**Fixture-load fallback:** real m48_50 may pull in heavy wiring. Acceptable fallback: synthetic 16×16 grid with hand-placed `ChangeFloor`-style writes that match the dumped tile shape. Document the simplification at the test site so NAI-98 doesn't optimize toward synthetic-only repro.

---

## 5. Methodology — hybrid probe-then-diff

1. **Bundle 0 — controller pre-flight (no commits).** Already partially done in §1; remaining items:
   - grep + Read confirmation of `interaction.go` dispatch arms current at HEAD (verify §1 lines against `git show HEAD`).
   - Verify NAI-94/NAI-95/NAI-96 t.Skip lifts haven't already addressed any of H1–H5.
   - Re-read `nai_96_grounddecor_path_around_residual.md` against current code state to ensure all 4 original hypotheses have a probe in §3.
2. **Bundle 1 — Stage 1 audit, dispatched as ONE Explore subagent** (per `investigation_subspec_cadence` Bundle 1 shape). Sub-stages 1.1 → 1.6, each producing a verdict appended to the diagnosis doc immediately.
3. **Per-hypothesis gating:** if Stage 1.1 surfaces an over-block on a tile that TS would not block (H1 confirmed for a specific LocType), Stage 1.3 (H2) still runs to enumerate any *additional* coercion-driven over-blocks; Stage 1.4 (H3) closes only if the smoking gun is provably elsewhere; Stage 1.5 (H4) and Stage 1.6 (H5) close as eliminated *only* if the H1 fix would also explain the abandonment shape.
4. **Subagent verification gate (per `audit_subagent_fabrication`, `verify_implementer_claims`):** controller verifies every claimed file:line citation with `git show` / `rg` / `Read` before writing into the diagnosis report. Pathfinder/algorithm audits and TS-PostDecode comparisons have known fabrication surface.

---

## 6. Deliverables

1. **This spec** at `docs/superpowers/specs/2026-05-05-nai-97-grounddecor-reach-investigation-design.md`.
2. **Diagnosis report** at `docs/superpowers/investigations/2026-05-05-nai-97-diagnosis.md` — per-hypothesis verdict (confirmed / eliminated / partial / undetermined-with-reason), file:line evidence, and either root-cause finding or explicit "diagnosis ceiling: NAI-98 needs X to break through."
3. **Reproducer tests** at the locations in §4 — skipped tests pinning observed-anomaly behavior, marked `// NAI-97:` for grep.
4. **Memory followup entry** in `nai_followups.md` under "From NAI-97" — Stage 2 (NAI-98) handoff with confirmed root cause, exact files/lines for the fix, plus any residual hypotheses NAI-98 needs to chase.

**No production code changes.** Test files + docs only.

---

## 7. Exit criteria

- All 5 hypotheses (H1–H5) have a verdict in the diagnosis report: confirmed / eliminated / partial / undetermined-with-reason.
- ≥1 reproducer per anomaly (Repro A, Repro B) committed as `t.Skip` in `nai97_repro_test.go` (unless every hypothesis closes as "false anomaly" — unlikely given smoke evidence).
- Diagnosis report identifies root cause with file:line evidence OR explicitly documents diagnosis ceiling and what NAI-98 would need to break through it.
- `nai_followups.md` updated with NAI-98 handoff.
- **No smoke gate** in Stage 1 — no production change to smoke. Stage 2 (NAI-98) will smoke.

---

## 8. Risks / known unknowns

- **Fixture load weight (per `empty_flagmap_degenerate_routefinder`):** bare `collision.NewFlagMap()` returns `FlagNull=-1` for unallocated zones; H1 probe must allocate via the production loader (real cache load). Synthetic fallback is acceptable but documented at the test site. NAI-94 had the same risk and resolved with synthetic + documentation.
- **H1+H2+H3 multi-divergence blowup:** if all three surface real divergences, NAI-98 prioritizes by smoke impact; NAI-97 enumerates without sequencing the fix.
- **H4 may close as eliminated trivially.** Pre-flight (§1) already shows interaction.go dispatches shape-aware by target class. The remaining H4 risk is NPC entity-dimension passing — verify `n.Width()`/`n.Length()` are non-trivially correct for the smoke NPCs (943, 3) at the dispatch sites.
- **H5 unit-test boundary:** `modules/world/` tickloop may not have a clean test seam between pathfinder-return and step-consume. If so, H5 is grep+read only; diagnosis ceiling explicitly notes "no runtime repro produced; H5 verdict is from static reading."
- **Subagent fabrication risk** (per `audit_subagent_fabrication`, `verify_implementer_claims`): controller-side independent verification of every citation before it lands in the diagnosis report. Specifically high-risk: TS PostDecode comparisons (Stage 1.3) and pathfinder dispatch reads (Stage 1.5).
- **Stale memory: `pathfinder_api_loc_aware`** references `FindPathDefault` (renamed to `FindPathPlain`). Update as part of NAI-97 close commit if not already done by NAI-94/95/96.

---

## 9. Cadence & memory references

- `investigation_subspec_cadence` — Stage 1 audit + reproducers; Stage 2 (NAI-98) fix; Stage 3 conditional on smoke. NAI-31 first instance, NAI-90/NAI-94 precedents.
- `ts_source_canonical_path` — only `LostCityRS/Engine-TS` is the porting reference.
- `empty_flagmap_degenerate_routefinder` — fixture must allocate via real cache.
- `pathfinder_api_loc_aware` — **stale**: refers to `FindPathDefault` which is now `FindPathPlain` at `api.go:40`. Update during NAI-97 close.
- `cascade_theory_smoke_binding` — NAI-96 smoke surfaced this residual; binding attribution.
- `smoke_surfaces_adjacent_divergences` — explicit precedent for "post-fix smoke surfaces adjacent divergence."
- `dispatch_correct_reach_blocked` — NAI-91→NAI-92 precedent for dispatch-correct vs downstream-blocked split.
- `audit_subagent_fabrication`, `verify_implementer_claims` — controller pre-flight + verification gate.
- `controller_preflight` — Bundle 0 structural answer.
- `close_commit_memory_trailer` — NAI-97 close commit carries `Closes memory: nai_96_grounddecor_path_around_residual.md` trailer.
- `superpowers_clear_between_spec_and_impl` — after spec lands, emit resume prompt and stop; user `/clear`s before plan writing.

---

## 10. Stage 2 handoff template (for NAI-98)

When Stage 1 closes, populate this in the NAI-97 close commit / followup memory entry:

- **Root cause:** _[file:line + 1-2 sentence summary]_
- **Repro tests to lift skip on:** _[list of tests + expected post-fix behavior]_
- **Files NAI-98 will touch:** _[exact list]_
- **Estimated LOC for fix:** _[ballpark]_
- **Residual hypotheses for NAI-99+:** _[any divergences not in NAI-98 scope]_
- **Smoke spec:** _[which client interactions confirm fix; specifically NPC 943 path-around (3221,3218)→(3218,3216) and NPC 3 path-around (3218,3213)→(3223,3216)]_
