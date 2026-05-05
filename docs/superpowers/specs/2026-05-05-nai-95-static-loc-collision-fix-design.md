# NAI-95 — Static-loc collision write at world init (Stage 2 fix to NAI-94)

**Status:** spec — investigation sub-spec (Stage 1 probe + Stage 2 fix + smoke; one sub-spec).
**Cadence:** `investigation_subspec_cadence` — Stage 1 binary-pass/fail probe via real-cache test → Stage 2 engine fix → user-driven smoke.
**Tech stack:** Go 1.26+.
**Parent:** NAI-94 — `docs/superpowers/specs/2026-05-05-nai-94-pathfinder-reach-investigation-design.md` and the diagnosis ceiling at `docs/superpowers/investigations/2026-05-05-nai-94-diagnosis.md`.
**TS reference:** `LostCityRS/Engine-TS` (per `ts_source_canonical_path`). Plan-author re-reads the TS world-init / mapsquare-load path to confirm TS does write static-loc collision at boot, naming the exact file:line for the deviation register.

---

## 1. Context & motivation

NAI-94 ELIMINATED the routefinder algorithm itself: `TestNAI94_AllocatedZones_PathfinderWorks/HansCheb2` and `/SurvivalExpertCheb8` PASS when supplied a FlagMap with the relevant zones allocated. The empty-FlagMap probes fail for a DEGENERATE reason (`FlagMap.Get()` returns `FlagNull=-1` for unallocated zones; `CanMove(-1, mask, TypeNormal)=false` for any non-zero mask, so BFS expansion is impossible from the start). The diagnosis ceiling was: NAI-95 must determine why production FlagMap allocation differs from the test fixture for the failing tiles.

Stage 0 static code-trace (this controller, pre-spec):

| File:line | Behavior | Allocates a zone? |
|---|---|---|
| `pkg/gamemap/load.go:55-57` | `loadGround` calls `gm.Pathfinder.ChangeFloor` only when `land & BLOCK_MAP_SQUARE != 0 && level == 0`. | Yes — but only zones whose ground includes at least one BLOCK_MAP_SQUARE tile at level 0. |
| `pkg/gamemap/load.go:92-134` | `loadLocs` parses `l{X}_{Z}` into `gm.staticLocs`. | **No.** Pure data parse; no FlagMap write. |
| `modules/world/server.go:318-323` | `populateStaticLocsIntoZones` calls `z.AddStaticLoc(loc)` for each parsed static loc. | **No.** Zone-side bookkeeping only. |
| `modules/world/world_zone.go:17-22` | Runtime `AddLoc` calls `s.gamemap.ChangeLocCollision(...)` when `LocType.BlockWalk`. | Yes. |

The runtime path (`AddLoc`) writes loc collision; the boot path (`populateStaticLocsIntoZones`) does not. Asymmetry → static castle walls around Hans never write `FlagBlockWalk` into the FlagMap → the zones whose only blockers are static locs (most of Lumbridge castle interior and surroundings) remain unallocated → BFS expansion from Hans's neighborhood hits `FlagNull=-1` adjacent tiles → moveNear closest-approach can't find any finite-distance candidate → returns `Success=true Alternative=true Waypoints=[]`.

Consumer-side amplification: `modules/world/movement.go:181-189` `routeToPacked` returns `nil` whenever `len(Waypoints)==0`, even with `Success=true`. The if-guard at `movement.go:143` then skips `queueWaypoints` entirely — the player stands still, `waypoint_idx=-1`. **Consumer behavior is correct given the input shape**; the bug is that the input shape should not occur for a 2-tile straight-line path. Fix is upstream (FlagMap allocation), not consumer.

---

## 2. Scope

### In scope (this sub-spec)

- **Stage 1 probe test** at `modules/world/static_loc_collision_test.go` (new) that boots a `Server` against the project's real cache directory, asserts the broken state at HEAD (zones around Hans's path unallocated, `FindPathPlain(0, 3219, 3224, 3219, 3222)` returns `{Success: true, Alternative: true, Waypoints: []}`), and PASSES against HEAD. The pass-on-broken assertion is the binary signal; Stage 2 inverts it.
- **Stage 2 fix** at `modules/world/server.go::populateStaticLocsIntoZones` (~8 LOC) — gate on `s.locTypes != nil` and `LocType.BlockWalk`, then call `s.gamemap.ChangeLocCollision(...)` mirroring `world_zone.go:17-22` `AddLoc`.
- **Cascade audit** of existing `modules/world/*_test.go` tests that boot a real `Server` with cache loading. Plan-author enumerates fallout, triages each failure as: (a) genuine pre-existing test bug now surfaced (fix), (b) fixture relying on absent collision (update), (c) escalate to NAI-96+.
- **Smoke handoff** to user: launch server, click Hans (Lumbridge spawn), confirm interaction completes.
- **NAI-94 reproducer cleanup:** Stage 2 may unskip the `TestNAI94_AllocatedZones_PathfinderWorks/HansCheb2` regression-guard tests if their behavior is now reachable without synthetic `internal.BuildCollisionMap` (only if natural — don't force).

### Out of scope (parked for NAI-96+ per NAI-94 diagnosis tail)

- `pkg/pathfinder/routefinder/routefinder.go:315` `routeFindSize2` `baseZ, baseZ` typo (size-2 source pathing).
- `pkg/pathfinder/routefinder/routefinder.go:26` `useRouteBlockerFlags` dead-stub field (remove or wire to a CollisionStrategy polymorphism port).
- `RouteFinder.FindRoute` defensive short-circuit when source-tile read returns `FlagNull` (would surface degenerate cases earlier).
- Bridge-tile / LINK_BELOW handling for multi-level mapsquares (already TODO at `gamemap/load.go:91`).

---

## 3. Stage 1 probe — `modules/world/static_loc_collision_test.go` (new)

### Why modules/world (not pkg/gamemap)

The missing call site is in `modules/world/server.go::populateStaticLocsIntoZones`, which depends on `s.locTypes` for `BlockWalk` lookup. Reproducing the gap requires the full `Server` boot path (cache load + locTypes load + populateStaticLocsIntoZones), not just the gamemap-only path.

### Test fixture strategy

- Skip-if-absent guard: at the top of each test, check whether the project's cache directory exists at the expected path (plan-author confirms exact path against existing `login_map_test.go` / `npc_test.go` fixtures). If missing, `t.Skip("real cache absent; CI-portable")`.
- Boot a `Server` via the closest existing test scaffolding to a real-init path. Plan-author identifies the canonical test-init helper during cascade audit.
- Three concrete asserts on Hans's path:

| Coord | Why | Pre-fix expected | Post-fix expected |
|---|---|---|---|
| `IsZoneAllocated(3216, 3216, 0)` (zone covering 3219, 3219) | Hans spawn zone | `false` | `true` |
| `IsZoneAllocated(3216, 3224, 0)` | Adjacent zone (player walks north into) | `false` | `true` |
| `IsZoneAllocated(3224, 3216, 0)` | Adjacent zone (path BFS expansion edge) | `false` | `true` |
| `gamemap.Pathfinder.FindPathPlain(0, 3219, 3224, 3219, 3222)` | The bug-shape repro | `Route{Success: true, Alternative: true, Waypoints: []}` | `Route{Success: true, Alternative: false, Waypoints: [(3219, 3222, 0)]}` (matches NAI-94's `TestNAI94_AllocatedZones_PathfinderWorks/HansCheb2`) |
| `Flags.Get(<castle wall coord>, 0) & FlagBlockWalk` | Positive wall pin | `0` (never set) | `!= 0` (FlagBlockWalk on) |

The castle wall coord must be a real wall tile from `l48_50.dat`. Plan-author resolves this at plan time by reading the actual static-loc list from the cache (or by reading TS spec for a known castle wall coord).

### Test naming

Single test function `TestNAI95_StaticLocCollision_HansArea` with subtests `ZoneAllocation`, `FindPathPlain`, `WallTileBlocked`. Rename to `TestStaticLocCollision_HansArea` post-close (drop the NAI-95 prefix once memorialized in `nai_followups.md`).

### No temp logging in production code

Probe data lives in test asserts only. `pkg/pathfinder/routefinder/api.go` is not touched.

---

## 4. Stage 2 fix — `modules/world/server.go::populateStaticLocsIntoZones`

### Change

For-loop body extension. Mirror `modules/world/world_zone.go:17-22`:

```go
func (s *Server) populateStaticLocsIntoZones() {
    for _, loc := range s.gamemap.StaticLocs() {
        if s.locTypes != nil {
            if lt := s.locTypeOrNil(loc.Type()); lt != nil && lt.BlockWalk {
                s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
                    loc.Length, loc.Width, loc.X, loc.Z, loc.Level, true)
            }
        }
        z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
        z.AddStaticLoc(loc)
    }
}
```

### Ordering invariant

`s.locTypes` is loaded earlier in `NewServer` (server.go ~line 250-280, before line 310 which calls `populateStaticLocsIntoZones`). The `s.locTypes != nil` gate is defensive (matches the `AddLoc` runtime gate); plan-author confirms `s.locTypes` is non-nil at the call point — if so, the gate is per-pattern, not load-bearing.

### Why precede `z.AddStaticLoc`

`AddLoc` at world_zone.go writes collision **before** the zone-side `z.AddLoc(loc)` call. Mirror that ordering for consistency. No semantic dependency between the two calls in either direction (collision write touches FlagMap; zone-side AddStaticLoc touches Zone.Locs); ordering is for code-shape parity.

### Spec test inversion

Same test file, same asserts, flipped expected values per the §3 table. The flip from "expected unallocated → expected allocated" is the visible regression-guard signal. Future audits that touch `populateStaticLocsIntoZones` see the test fail if the fix is regressed.

### No call-site changes elsewhere

The fix is isolated to `populateStaticLocsIntoZones`. Runtime `AddLoc` / `ChangeLoc` / `RemoveLoc` paths already write collision and don't need touching. NPCs / players / floor-blockers continue to write via their existing call sites.

---

## 5. Cascade audit — required pre-Stage-2

Per `latent_bug_at_migration_boundary` and `cascade_theory_smoke_binding` memories.

### Plan-author task

Grep `modules/world/*_test.go` for tests that:
- Boot a real `Server` via cache loading (not synthetic `BuildCollisionMap`-only fixtures).
- Call any pathfinder API (`FindPath*`, `LineValidator.*`, `StepValidator.*`, `AddNpc`, `AddPlayer`).
- Assert NPCs / players moving through tiles that are now genuinely wall-blocked at boot.

For each such test, run with the Stage 2 fix applied and triage failures:

| Failure mode | Triage |
|---|---|
| Test asserted NPC walks through a castle wall coord that is now FlagBlockWalk-set. | (b) update fixture — the test was relying on absent collision; either move the NPC's path off the wall, or make the test explicitly opt-out of static-loc collision (justify why, document on the test). |
| Test was a pre-existing bug (asserted incorrect post-fix behavior, e.g., player path that was always wrong but happened to "succeed" via degenerate-empty-FlagMap fall-through). | (a) fix the test to assert correct post-fix behavior. |
| Test surfaces a real production bug not previously caught (e.g., NPC wander tile is inside a now-allocated wall). | (c) escalate to NAI-96 — content data fix, not engine. Track in `nai_followups.md`. |

### Pre-known fallout candidates

`grep -l "BuildCollisionMap\|gamemap\.Init\|cachePath\|cache_path" modules/world/*_test.go` — plan-author starts here.

### Risk register entry

`R1: Cascade audit may surface 0-N tests requiring fixture updates. Estimated LOC of fixture changes: 0-50.` Plan-author re-estimates after grepping.

---

## 6. Smoke (Stage 3 — user-driven)

Per `smoke_test_server_handoff`.

### Primary smoke

1. Controller commits Stage 2 fix on a working branch.
2. Hand server config + cache path to user.
3. User launches server (`CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml`).
4. Java client logs into Lumbridge spawn.
5. User clicks Hans (NPC ID 961, near 3219, 3222).
6. **Pass:** player paths to Hans, interaction completes (chat dialogue or talk-to action triggers). **Fail:** player stays put OR symptom shape changes (paste new symptom for routing).

### Secondary smokes

- Click around Lumbridge castle interior (multi-room nav).
- Walk through a castle doorway (NAI-90/91 territory; if doors regress, escalate per `door_throughwalk_gap` memory).
- Click Survival Expert (3103, 3095) from outside — NAI-94 cheb=8 case.

### Routing rule for residuals

Per `smoke_surfaces_adjacent_divergences`:
- ≤30 LOC + same root cause → in-scope stretch (this sub-spec).
- Else → NAI-96+ (new sub-spec; track in `nai_followups.md`).

Doors / RESUME_PAUSEBUTTON / TUT_OPEN cascade items remain on existing tracker entries unless smoke directly implicates them.

---

## 7. Deviations from TS reference

Plan-author re-reads `LostCityRS/Engine-TS` world-init / mapsquare-load path for the static-loc collision write site. Expected finding: TS DOES write static-loc collision at boot (otherwise TS would have the same Hans bug and it doesn't). If TS DOES write — this is a parity fix, no tracking deviation.

Provisional entry, plan-author confirms or removes:

> `D-NAI-95-1` (provisional): TS writes static-loc collision at world init (citation TBD by plan-author). goscape's `populateStaticLocsIntoZones` previously omitted this; NAI-95 closes the gap. **No tracking deviation** — fix matches TS.

If plan-author finds TS does NOT write static-loc collision at boot but the symptom doesn't reproduce in TS, that's a real puzzle requiring controller routing — likely a different upstream wiring difference. Plan-author surfaces this immediately.

---

## 8. Deliverables

1. **This spec** at `docs/superpowers/specs/2026-05-05-nai-95-static-loc-collision-fix-design.md`.
2. **Plan** at `docs/superpowers/plans/2026-05-05-nai-95-static-loc-collision-fix.md` (writing-plans skill).
3. **Stage 1 probe test** at `modules/world/static_loc_collision_test.go` (new file, ~50 LOC).
4. **Stage 2 fix** at `modules/world/server.go::populateStaticLocsIntoZones` (~8 LOC).
5. **Cascade audit** results — fixture updates as needed (0-50 LOC, plan-author re-estimates).
6. **Memory followups** if smoke surfaces residuals — append to `nai_followups.md` under "From NAI-95."
7. **Close commit** with `Closes memory:` trailer for any new memory entries.

---

## 9. Exit criteria

- Stage 1 probe test passes against HEAD (pre-fix), pinning broken state.
- Stage 2 fix lands; Stage 1 test inverted asserts pass.
- All `modules/world/*_test.go` tests pass post-fix (cascade audit complete).
- All `pkg/...` tests pass post-fix (no upstream breakage).
- `go test -race ./...` passes.
- User-driven smoke confirms Hans interaction completes; no door/Survival-Expert regressions surface (or surfaced residuals routed per §6).
- Close commit on `main` with `Closes memory:` trailer.

---

## 10. Risks / known unknowns

- **R1 — Cascade scope:** Tests that boot a real cache may have N fixture updates required. Bounded at the cascade-audit step; plan-author re-estimates LOC.
- **R2 — Cache directory portability:** Probe test requires a real `m48_50` / `l48_50` in the project's cache. Skip-if-absent gate keeps CI portable; controller ensures local dev + smoke environments have the cache.
- **R3 — TS reference confirmation:** Plan-author must locate the TS site that writes static-loc collision at boot. If TS does NOT write but symptom doesn't reproduce in TS — escalate (different upstream wiring; this sub-spec re-scopes).
- **R4 — Latent bugs from cascade:** Per `latent_bug_at_migration_boundary`, fix may surface bugs in test fixtures or in NPC wander positions on now-walled tiles. Cascade audit triages; NAI-96+ catches genuine content / engine residuals.
- **R5 — populateStaticLocsIntoZones loc filtering:** Unknown whether some static locs at boot are NPCs / Objs improperly classified as locs. Plan-author confirms `gm.staticLocs` slice contains only real `*entity.Loc` instances. Existing helper `locTypeOrNil` handles missing types gracefully.
- **R6 — Subagent fabrication:** Per `audit_subagent_fabrication`, if any audit sub-task is delegated, controller verifies every claimed file:line citation with `git show` / `rg` / `Read` before merging.

---

## 11. Cadence & memory references

- `investigation_subspec_cadence` — Stage 1 binary probe, Stage 2 fix, Stage 3 smoke; one sub-spec.
- `dispatch_correct_reach_blocked` — NAI-91→NAI-92 precedent: dispatch correct, downstream engine layer blocks. NAI-94 confirmed routefinder algo correct on allocated zones; NAI-95 fixes the production allocation gap.
- `empty_flagmap_degenerate_routefinder` — explains the BFS-failure mechanics when a zone returns `FlagNull=-1`. Same mechanics here, just with production data instead of empty test FlagMap.
- `latent_bug_at_migration_boundary` — cascade audit must triage test fallout as new-bug vs. fixture-fix vs. content-issue.
- `smoke_surfaces_adjacent_divergences` — routing rule for smoke residuals (≤30 LOC in-scope; else NAI-96+).
- `smoke_test_server_handoff` — user launches server; controller cannot smoke directly.
- `cascade_theory_smoke_binding` — smoke binds attribution; if Stage 2 fix lands and smoke still fails for Hans, the cascade theory was incomplete (controller re-brainstorms).
- `verify_implementer_claims` — controller pre-flight + post-task verification; 30-second protocol.
- `audit_subagent_fabrication` — verify every audit citation with independent grep/Read before code change.
- `close_commit_memory_trailer` — close commit carries `Closes memory: ...` trailer.
- `controller_preflight` — controller does 30-sec grep+Read verification of every plan-author premise before each implementer dispatch.
- `ts_source_canonical_path` — only `LostCityRS/Engine-TS` is the porting reference for the deviation register entry.
