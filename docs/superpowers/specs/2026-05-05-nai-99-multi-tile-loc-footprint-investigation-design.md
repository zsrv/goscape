# NAI-99 — Multi-tile Loc footprint coverage investigation (Stage 1)

**Status:** spec — investigation sub-spec (Stage 1; Stage 2 fix routes to NAI-100).
**Cadence:** `investigation_subspec_cadence` — Bundle 0 controller pre-flight (no commits) → Bundle 1 Stage 1 risk-weighted-short-circuit audit + reproducer pins → no Stage 2 in NAI-99 → smoke handoff to NAI-100.
**Tech stack:** Go 1.26+.
**Upstream source:** `LostCityRS/Engine-TS` (per `ts_source_canonical_path`). For pathfinder questions, also `2004scape/rsmod-pathfinder` AS HEAD (per NAI-94 §risks). For Rust references, `rust_source_canonical_path` applies (`2004scape/rsbuf` for rsbuf; analogous repo for rsmod).

---

## 1. Context & motivation

NAI-98 closed GroundDecor reach Stage 2 (sub-H8 dispatch fix) at `51b693b`. User-launched smoke (2026-05-05, post-NAI-98 close) surfaced a **new** anomaly:

> "I walk partly into the fountain before getting stuck; the fountain is multiple tiles wide but it appears it's being treated like it's only one tile wide."

The Lumbridge fountain is a multi-tile feature near (3221, 3218). Player walked NW from Lumbridge spawn into the fountain footprint and got stuck on what appears to be a single tile rather than being kept out of the full footprint.

**Why NAI-98 H8-fix did not surface this:** NAI-98 Phase 1 probe at `pkg/gamemap/nai98_realcache_probe_test.go` walked every BFS sub-step through `gm.CanTravel`. BFS-CanMove and CanTravel use the same FlagMap. If only N of M footprint tiles are flagged, BFS happily routes through the unflagged tiles AND CanTravel agrees on those tiles — the probe sees no contradiction. Smoke catches it because the player physically walks through unflagged tiles into the footprint and stops at the one correctly-flagged tile.

**Pre-flight observations (Bundle 0 controller probe):**

- `pkg/gamemap/gamemap.go:61-78` — `ChangeLocCollision` `LayerGroundDecor` branch writes single-tile via `pf.ChangeFloor(x, z, level, add)` when `active==1`. Identical shape to TS `Engine-TS/src/engine/GameMap.ts:336-340` (which calls single-tile `rsmod.changeFloor`).
- `pkg/pathfinder/routefinder/api.go:74-80` — `ChangeFloor` is single-tile; no W×L loop.
- `pkg/pathfinder/routefinder/api.go:82-99` — `ChangeLoc` (LayerGround) iterates `width*length`. Multi-tile awareness exists for LayerGround, NOT for LayerGroundDecor.
- `modules/world/server.go:315-335` `populateStaticLocsIntoZones` — calls `ChangeLocCollision` only when `lt.BlockWalk == true` (line 327). Locs with `BlockWalk=false` get no collision write.
- `pkg/objtype/loctype.go:166-176` `PostDecode` — `Active=-1` coerced to `1` when `Op != nil` or `Shapes==[10]`.
- `pkg/entity/loc.go:52` — per-instance `Shape()` is bits 14..18 of `CurrentInfo`, decoded from the `l_x_z` pack (NOT from LocType.Shapes).

---

## 2. Scope

### In scope (Stage 1)

- Reproduce the anomaly as a Go test against the actual cache + collision wiring; pin observed-coverage shape with `t.Skip("NAI-99: …")`.
- Risk-weighted-short-circuit hypothesis audit (H1→H4; §3).
- Diagnosis report at `docs/superpowers/investigations/2026-05-05-nai-99-diagnosis.md` with per-hypothesis verdict and file:line evidence.
- `nai_followups.md` update with NAI-100 (Stage 2) handoff.

### Out of scope (Stage 2 / future)

- **Production code changes** to `pkg/objtype/loctype.go`, `pkg/gamemap/gamemap.go`, `pkg/pathfinder/routefinder/`, or any consumer in `modules/world/`. Stage 2 (NAI-100) fixes.
- Lifting any `t.Skip("NAI-99: …")` markers; lifting them is the NAI-100 success criterion.
- Smoke confirmation. NAI-100 smokes Stage 2.
- Other parked smoke residuals from earlier NAI follow-up tracker items.

---

## 3. Hypothesis register (risk-weighted-short-circuit ordered)

| # | Hypothesis | Probe cost | Probe shape | If confirmed |
|---|---|---|---|---|
| **H1** | Fountain LocType has `BlockWalk=false` (or `Active!=1`), so `populateStaticLocsIntoZones` skips the collision write entirely (`server.go:327`). The single tile the player gets stuck on is a **different adjacent loc** (basin, plinth) — not the fountain itself | Low — bbox dump + LocType lookup | **Stage 1.2** (consumes Stage 1.1 dump): for every loc in the bbox, identify which one's footprint the player got stuck inside; check `lt.BlockWalk`, `lt.Active`, and which loc instance produced the FlagBlockWalk write at the stuck tile | NAI-100 fixes either the LocType decode OR adjusts the gating at `server.go:327` per TS World boot path |
| **H2** | Fountain is a **single multi-tile GroundDecor LocType** (`Width=2,Length=2`) with `BlockWalk=true,Active=1`, but `ChangeFloor` writes only the origin tile. **Both TS and goscape behave the same way** at this layer — confirming this hypothesis would mean goscape needs to *diverge* from TS by adding a W×L loop at `LayerGroundDecor` (or fixing `ChangeFloor` to take `width,length`), since the canonical Rust rsmod-pathfinder presumably handles this internally | Low — dump + Rust rsmod cross-check | **Stage 1.3:** confirm `lt.Width>1 \|\| lt.Length>1` for the fountain LocType from the dump; then read `2004scape/rsmod-pathfinder` `change_floor` (Rust) to verify whether rsmod handles W×L internally. If rsmod single-tile, content matches TS and H2 is "matches TS, no fix" with H3 covering it; if rsmod multi-tile, **goscape's `ChangeFloor` is the divergence** | NAI-100 fixes `pkg/pathfinder/routefinder/api.go:74-80` `ChangeFloor` to iterate W×L (or fixes the call site at `gamemap.go:73-75` to loop) |
| **H3** | Fountain is **N adjacent single-tile loc placements** in the `l_x_z` pack (content-side multi-tile composition), and only some have `BlockWalk=true`. The "partway in" symptom = the BlockWalk subset is < the visible footprint subset | Low — dump | **Stage 1.1** enumerates per-tile loc instances; if multiple distinct loc IDs at fountain coords with mixed BlockWalk, this confirms | NAI-100 likely no-fix (content matches TS); diagnosis explicitly closes as "matches TS, content authoring choice" |
| **H4** | Fountain is a **`LayerGround` centrepiece** (shape 10 / 11) with `Width=2,Length=2`, but per-instance Shape decoded from the l-pack is wrong (e.g., decoded as 22 GroundDecor) → routes through `LayerGroundDecor` single-tile branch instead of `LayerGround` W×L `ChangeLoc` | Med — dump + l-pack decode read | **Stage 1.4:** from the dump, compare per-instance `Shape()` against TS expected per-LocType shape for the same fountain ID. Read `pkg/gamemap/loc_loader.go` (or wherever `loadLocs` decodes per-instance shape) against TS `LocLoader`/equivalent | NAI-100 fixes the per-instance shape decode in `loadLocs` or `entity.NewLoc` |

**Order rationale:**

- H1 first: cheapest probe and most likely explanation (single-tile-stuck = single Loc with a BlockWalk write, not the "fountain footprint" failing). Short-circuits H2/H4 if it lands.
- H2 second: requires Rust cross-reference; only fires if the fountain is provably a single multi-tile GroundDecor LocType per the dump.
- H3 closes naturally as "matches TS, no fix" if observed in the dump.
- H4 last: highest narrowing cost (l-pack per-instance decode read).

**Short-circuit policy:** each substage's verdict appended to the diagnosis doc immediately. If H1 surfaces a smoking-gun "stuck on adjacent loc, not fountain" explanation, controller decides whether to continue H2/H3/H4 enumeration (for completeness) or close at H1.

---

## 4. Probe / reproducer matrix

| Test | Location | Shape | Skip disposition |
|---|---|---|---|
| `TestNAI99_FountainFootprintDump_Lumbridge` | new `pkg/gamemap/nai99_fountain_dump_test.go` | Loads real cache via `gm.Init`, replays `populateStaticLocsIntoZones` (mirrors `modules/world/server.go:315-330` and `pkg/gamemap/nai98_realcache_probe_test.go:48-59`). For tiles in bbox `(3217..3225, 3214..3220, level=0)`, dumps `(x, z, perInstance.Shape, perInstance.Angle, locID, locTypeID, locTypeName, lt.Width, lt.Length, lt.BlockWalk, lt.BlockRange, lt.Active, FlagMap[x,z,level])` per loc instance to `t.Log`. **No assertions** — output captured into the diagnosis doc as Stage 1.1 input. | Always passes; dump-only. |
| `TestNAI99_FountainCoverage_Lumbridge` | same file | After Stage 1.1 identifies the fountain LocType (by name match `*fountain*` or hand-supplied ID), for its instance(s) in the bbox, asserts every tile in `[X .. X+W-1] × [Z .. Z+L-1]` (rotated by Angle per the existing `LayerGround` swap convention at `gamemap.go:67-71`) carries the expected flag — `FlagBlockWalk` for GroundDecor active=1, `FlagLoc` for LayerGround. | If reproduces (some footprint tiles unflagged), `t.Skip("NAI-99: Lumbridge fountain footprint coverage divergence — observed flagged tiles X of Y, expected Y. Stage 2 lifts in NAI-100.")` pinning observed shape. |

**Fixture-load constraint:** real cache must allocate via `gm.Init` (per `empty_flagmap_degenerate_routefinder`). If `data/pack/server/maps/m48_50` or `data/pack/server/loc.dat` is unavailable, `t.Skipf` (per NAI-98 precedent at `nai98_realcache_probe_test.go:31-36`). No synthetic fallback acceptable — the anomaly is fountain-specific content, not algorithmic.

---

## 5. Methodology — hybrid probe-then-diff

1. **Bundle 0 — controller pre-flight (no commits).** Already done in §1; remaining items:
   - Verify `data/pack/server/maps/m48_50` and `data/pack/server/loc.dat` exist locally.
   - Verify the chosen bbox `(3217..3225, 3214..3220)` actually contains fountain tiles via a quick dump-test pass (subagent can confirm in Stage 1.1 if uncertain).
   - Re-read `nai_98_fountain_footprint_residual.md` against current code state to ensure all hypotheses in §3 cover the suggested probe shape.
2. **Bundle 1 — Stage 1 audit, dispatched as ONE Explore subagent** (per `investigation_subspec_cadence` Bundle 1 shape). Sub-stages 1.1 → 1.4, each producing a verdict appended to the diagnosis doc immediately.
3. **Per-hypothesis gating:** if Stage 1.1 dump surfaces an "adjacent loc, not fountain" explanation (H1), Stage 1.3 (H2 Rust cross-check) and Stage 1.4 (H4) close as eliminated *only* if the H1 fix would also explain the "partway in" shape. H3 verdict ("matches TS, content authoring") is reported regardless.
4. **Subagent verification gate (per `audit_subagent_fabrication`, `verify_implementer_claims`):** controller verifies every claimed file:line citation with `git show` / `rg` / `Read` before writing into the diagnosis report. **High fabrication surface:** the Rust rsmod-pathfinder read in Stage 1.3 — controller-side verification mandatory; cite the exact Rust file path and function signature.

---

## 6. Deliverables

1. **This spec** at `docs/superpowers/specs/2026-05-05-nai-99-multi-tile-loc-footprint-investigation-design.md`.
2. **Diagnosis report** at `docs/superpowers/investigations/2026-05-05-nai-99-diagnosis.md` — per-hypothesis verdict (confirmed / eliminated / partial / undetermined-with-reason), file:line evidence, and either root-cause finding or explicit "diagnosis ceiling: NAI-100 needs X to break through."
3. **Reproducer tests** at the locations in §4 — dump always passes; coverage assertion `t.Skip`-pins observed shape if it diverges. Marked `// NAI-99:` for grep.
4. **Memory followup entry** in `nai_followups.md` under "From NAI-99" — Stage 2 (NAI-100) handoff per the §10 template (root cause, files, repro to lift, smoke spec).

**No production code changes.** Test files + docs only.

---

## 7. Exit criteria

- All 4 hypotheses (H1–H4) have a verdict in the diagnosis report: confirmed / eliminated / partial / undetermined-with-reason.
- Both reproducer tests committed in `pkg/gamemap/nai99_fountain_dump_test.go`. Dump always passes; coverage may `t.Skip` per observed divergence.
- Diagnosis report identifies root cause with file:line evidence OR explicitly documents diagnosis ceiling and what NAI-100 would need to break through it.
- `nai_followups.md` updated with NAI-100 handoff.
- **No smoke gate** in Stage 1 — no production change to smoke. Stage 2 (NAI-100) will smoke.

---

## 8. Risks / known unknowns

- **Fixture load** (per `empty_flagmap_degenerate_routefinder`): real `data/pack/server/maps/m48_50` + `loc.dat` required. If unavailable, `t.Skipf` per NAI-98 precedent.
- **Multi-divergence blowup:** if H1, H3, and H4 all surface real divergences (e.g., adjacent-loc + l-pack-decode bug + GroundDecor footprint miss), NAI-100 prioritizes by smoke impact; NAI-99 enumerates without sequencing the fix.
- **Subagent fabrication risk** (per `audit_subagent_fabrication`, `verify_implementer_claims`): high in Stage 1.3 (Rust rsmod-pathfinder read). Controller-side independent verification of every citation before it lands in the diagnosis report.
- **Smoke evidence ambiguity:** user said "fountain treated like 1 tile wide", but the actual blocker may be an adjacent loc (H1). The dump enumerates regardless, so this isn't a probe-design issue, but the diagnosis report should explicitly distinguish "fountain footprint coverage" from "stuck-on-adjacent-loc" as the symptom-source.
- **BBox coverage** — if the chosen bbox `(3217..3225, 3214..3220)` doesn't contain the fountain (off-by-one or wrong square), Stage 1.1 produces a diagnostic dead end. Bundle 0 spot-check or Stage 1.1 expansion to a wider bbox is the mitigation.

---

## 9. Cadence & memory references

- `investigation_subspec_cadence` — Stage 1 audit + reproducers; Stage 2 (NAI-100) fix; Stage 3 conditional on smoke.
- `ts_source_canonical_path` — only `LostCityRS/Engine-TS` is the porting reference.
- `rust_source_canonical_path` — `2004scape/rsbuf` for rsbuf; analogous `2004scape/rsmod-pathfinder` for rsmod (per NAI-94 §risks precedent).
- `empty_flagmap_degenerate_routefinder` — fixture must allocate via real cache.
- `cascade_theory_smoke_binding` — NAI-98 smoke surfaced this residual; binding attribution.
- `smoke_surfaces_adjacent_divergences` — explicit precedent for "post-fix smoke surfaces adjacent divergence."
- `dispatch_correct_reach_blocked` — NAI-91 → NAI-92 precedent for dispatch-correct vs downstream-blocked split.
- `audit_subagent_fabrication`, `verify_implementer_claims` — controller pre-flight + verification gate.
- `controller_preflight` — Bundle 0 structural answer.
- `close_commit_memory_trailer` — NAI-99 close commit carries `Closes memory: nai_98_fountain_footprint_residual.md` trailer.
- `superpowers_clear_between_spec_and_impl` — after spec lands, produce plan via writing-plans, THEN emit resume prompt and stop; user `/clear`s before implementing.

---

## 10. Stage 2 handoff template (for NAI-100)

When Stage 1 closes, populate this in the NAI-99 close commit / followup memory entry:

- **Root cause:** _[file:line + 1-2 sentence summary]_
- **Repro tests to lift skip on:** _[list of tests + expected post-fix behavior]_
- **Files NAI-100 will touch:** _[exact list]_
- **Estimated LOC for fix:** _[ballpark]_
- **Residual hypotheses for NAI-101+:** _[any divergences not in NAI-100 scope]_
- **Smoke spec:** _[walk NW from Lumbridge spawn (3221, 3218) into the fountain footprint; verify all expected footprint tiles block; verify path-around routes correctly to NPCs on the far side]_
