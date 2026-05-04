# NAI-91: Shape-aware inOperableDistance for Loc targets

**Status:** Draft (brainstorm output)
**Date:** 2026-05-04
**Cadence:** Single-bundle fix sub-spec. Diagnosis is static-confirmed (no Stage 1 instrumentation needed).
**Predecessor:** NAI-90 (door throughwalk investigation; bound H2 = proc/branch divergence in `[proc,open_and_close_door]`). NAI-91 reframes the binding: the proc executes correctly per its content; the actual root cause is engine-side `inOperableDistance` ignoring loc shape/angle.
**Tech stack:** Go 1.26+ / TS reference at LostCityRS/Engine-TS / pkg/pathfinder/reach already ports the canonical reach predicates (Rust source path: 2004scape/rsbuf branch 225 analogue per `rust_source_canonical_path` memory).

---

## 1. Goal

Wire goscape's `inOperableDistance` (player-side `interaction.go:460-473` and NPC-side `npc_interaction.go:521-546`) to dispatch through `pkg/pathfinder/reach.Reached` for Loc targets. This closes the user-reported Tutorial Island RS Guide door re-click failure ("I can't reach that!" from the door tile, blocking exit) and fixes the same gap for any other wall / wall-deco / rectangular Loc where the on-tile or shape-specific approach is the legitimate interaction tile.

The current Chebyshev≤1-and-not-same-tile predicate is an untagged shape deviation grouped informally under S6l-D4 by the NPC-side comment at `npc_interaction.go:525-528` ("inherits player-side's S6l-D4 posture"). The official S6l-D4 tag (per `interaction.go:477`) is the *LOS-in-approach* deviation, not the shape-in-operable gap; the comment-side aggregation is loose. NAI-91 closes the Loc-side shape gap and registers a new tag (`NAI-91-D-OPERABLE-CHEB-FALLBACK`) for the residual entity-shape (`reachedEntity`) and Obj-shape (`reachedObj`) work. S6l-D4 itself is untouched.

## 2. Context: NAI-90 H2 binding reframed

NAI-90's close commit framed the bug as proc/branch divergence in `[proc,open_and_close_door]`. NAI-91's brainstorm static-confirmed that framing was a misread:

**Diagnosis chain (verified at brainstorm-time):**

1. **Disasm of `[proc,open_and_close_door]`** (via one-off `pkg/script.Disassemble`): PC=47 is `P_TELEPORT 0` preceded by `PUSH_INT_LOCAL 8` at PC=46. Local 8 is `$dest`. The proc's `BRANCH 25` at PC=20 jumps over the entire `if ($entering = true) { ... }` body to PC=45 when `$entering=false`. The trailing `p_teleport($dest)` at PC=47 fires with `$dest = $loc_coord` (the default initialised at PC=15-16).
2. **`~check_axis` returning false was content-correct** for the smoke geometry: player at `(3097,3107)` clicked door at `(3098,3107,0)` with `loc_shape=0` (wall_straight) `loc_angle=0` (loc_west). check_axis routes angle=0 to the `^loc_west, ^loc_east` arm, checks `coordx(player)==coordx(loc)` → `3097 ≠ 3098` → returns false → `$entering=false` → trailing teleport to `$loc_coord`. **The door tile is east of the wall = building interior; the single-step-in is the designed first-click behavior.**
3. **The user-reported breakage is the re-click**, not the first click. From the door tile, `check_axis(coord==$loc_coord)` returns true → `$entering=true` → inner-first teleport gated off (`if (coord != $loc_coord)` is false) → `$dest = movecoord(loc, $x=-1, 0, $z=0) = (3097,3107)` → `p_teleport((3097,3107))` exits west. **The proc would do the right thing if it ran.**
4. **It doesn't run.** Smoke tick 44 shows `cheb_dist=0`, `op_trigger=true`, `interaction_fired=false`, `repathed=true`, `steps_taken=0`. `tryInteract`'s branch 1 (OP fire) gates on `inOperableDistance`; that returns false because `dx==0 && dz==0` is excluded by the same-tile rule at `interaction.go:472`. Branch 4 (default-OP) gates on the same predicate. Both skip; the proc never gets dispatched.
5. **TS Player.inOperableDistance** (Player.ts:1099-1111) dispatches to `reachedLoc(level, x, z, tx, tz, w, l, srcW, angle, shape, forceapproach)` for Loc targets — shape-aware. For wall_straight at the loc tile, it returns true.
6. **goscape already has the impl** at `pkg/pathfinder/reach.Reached(...)` (`strategy.go:35`). At `ReachWall1` line 80, the `srcSize == 1 && srcX == destX && srcZ == destZ → return true` short-circuit covers the on-tile case. A brainstorm-time probe (`reach.Reached` called with the door's exact inputs) confirmed `true` for both "player on door tile" and "player west of door"; existing first-click behavior is preserved. The piece is unit-tested at `pkg/pathfinder/reach/strategy_test.go`.

**Conclusion:** The H2 binding ("proc/branch divergence") was based on inferred semantics. The actual bug is at the engine-side reach gate. NAI-91 ships a one-line dispatch swap on each side (Player and NPC) and a doc-comment rewrite of the NPC-side `inOperableDistance` block (which was the only site informally bundling the shape gap under S6l-D4).

## 3. Cadence

Single bundle, three tasks, end-of-bundle user-launched smoke. Per `compressed_cadence` memory: production code surface is small (~55 LOC of new/replaced production code across two files); tests are mechanical matrix coverage; no Stage 1 instrumentation needed because the diagnosis is static-confirmed.

**Task list (derived from §4):**

1. **T1 — Player-side `inOperableDistance` Loc dispatch.** TDD. Failing-tests-first matrix in `interaction_test.go` (§5.1) plus the on-tile pin (§5.1 named test). Implement §4.1's body, retain `inOperableDistanceCheb` for non-Loc targets, swap caller at `interaction.go:381`. One commit.
2. **T2 — NPC-side `(*Npc).inOperableDistance` Loc dispatch.** TDD. Failing-tests-first mirror matrix in `npc_interaction_test.go` (§5.2) parameterized over `srcSize ∈ {1,2}`. Implement §4.2's body. One commit.
3. **T3 — Doc-comment rewrite at the NPC-side block** (`npc_interaction.go:525-528`): drop the "inherits S6l-D4 posture" framing for the now-fixed Loc path; scope the residual under `NAI-91-D-OPERABLE-CHEB-FALLBACK`. Cross-grep `S6l-D4` per §6's expected-set. One commit.

End-of-bundle user-launched smoke (§7) follows T3. Close commit follows smoke confirmation per `cascade_theory_smoke_binding`.

## 4. Architecture

### 4.1 Player-side dispatch

**Site:** `modules/world/interaction.go:460-473` (`inOperableDistance` free function) and its single caller at `interaction.go:381` (inside `Player.tryInteract`). The free function takes `(px, pz, tx, tz int)` today — that signature loses access to the target's shape/angle/width/length and to the server. Replace with a method-style or target-aware signature.

**New signature** (free function, takes the player and target):

```go
// inOperableDistance reports whether p is in contact range of target.
// Mirrors TS Player.inOperableDistance (Player.ts:1099-1111):
//   - Loc targets dispatch to pkg/pathfinder/reach.Reached for shape/
//     angle/forceapproach-aware reach.
//   - PathingEntity targets (Player, Npc) use Chebyshev≤1, exclude same
//     tile (entity-shape reachedEntity port deferred under NAI-91-D-OPERABLE-CHEB-FALLBACK).
//   - Obj targets use Chebyshev≤1 (reachedObj port deferred under NAI-91-D-OPERABLE-CHEB-FALLBACK).
//
// Diff from TS: target.level mismatch returns false (TS guard preserved).
func inOperableDistance(p *Player, target entity) bool {
    tx, tz, tlevel := target.Coords()
    if tlevel != p.level { return false }
    switch t := target.(type) {
    case *entitypkg.Loc:
        srv := p.client.server
        flags := srv.gamemap.Pathfinder.Flags
        var fap int
        if cfg := srv.locTypeOrNil(t.Type()); cfg != nil {
            fap = cfg.ForceApproach
        }
        return reach.Reached(flags, p.level, p.x, p.z, tx, tz,
            t.Width, t.Length, 1, t.Angle(), t.Shape(), fap)
    default:
        return inOperableDistanceCheb(p.x, p.z, tx, tz)
    }
}

// inOperableDistanceCheb is the legacy Chebyshev≤1 predicate retained
// for PathingEntity / Obj targets pending the entity-shape /
// reachedObj port (DEVIATION NAI-91-D-OPERABLE-CHEB-FALLBACK).
// Excludes same-tile.
func inOperableDistanceCheb(px, pz, tx, tz int) bool {
    dx := px - tx
    if dx < 0 { dx = -dx }
    dz := pz - tz
    if dz < 0 { dz = -dz }
    if dx > 1 || dz > 1 { return false }
    return !(dx == 0 && dz == 0)
}
```

**Caller update** at `interaction.go:381`:

```go
operable := inOperableDistance(p, p.target)
```

(Replaces `inOperableDistance(p.x, p.z, tx, tz)`.)

### 4.2 NPC-side dispatch

**Site:** `modules/world/npc_interaction.go:521-546` (`(*Npc).inOperableDistance` method).

**New body** (preserves the method receiver):

```go
// inOperableDistance reports whether n is in contact range of target.
// Mirrors TS Npc.inOperableDistance via PathingEntity.inOperableDistance
// (PathingEntity.ts:378-389) for Loc targets; Chebyshev≤1 fallback for
// PathingEntity / Obj retained under NAI-91-D-OPERABLE-CHEB-FALLBACK.
func (n *Npc) inOperableDistance(target entity) bool {
    tx, tz, tlevel := target.Coords()
    if tlevel != n.level { return false }
    switch t := target.(type) {
    case *entitypkg.Loc:
        srv := n.server // resolve actual NPC→server access path at plan-write
        flags := srv.gamemap.Pathfinder.Flags
        var fap int
        if cfg := srv.locTypeOrNil(t.Type()); cfg != nil {
            fap = cfg.ForceApproach
        }
        srcSize := int(n.size)
        if srcSize <= 0 { srcSize = 1 }
        return reach.Reached(flags, n.level, n.x, n.z, tx, tz,
            t.Width, t.Length, srcSize, t.Angle(), t.Shape(), fap)
    default:
        // Chebyshev fallback (existing behavior).
        dx := n.x - tx
        if dx < 0 { dx = -dx }
        dz := n.z - tz
        if dz < 0 { dz = -dz }
        if dx > 1 || dz > 1 { return false }
        return !(dx == 0 && dz == 0)
    }
}
```

The NPC `srcSize=int(n.size)` passes through `reachWallN` for multi-tile NPCs. The existing `reachWallN` impl is unit-tested at `pkg/pathfinder/reach/strategy_test.go`; we rely on that coverage.

### 4.3 Width/Length invariant

`pkg/entity/Loc.Width` / `Loc.Length` store **absolute** (un-rotated) dimensions. Verified at `modules/world/script_loc_ops.go:35-43` (LocOps.AddLoc passes `lt.Width` / `lt.Length` directly from the LocType) and `pkg/gamemap/load.go:128` (static load passes 1,1). `reach.Reached` rotates internally via `rotation.Rotate(locAngle, destWidth, destLength)`. **No double-rotation; no shim needed.** Documented as an invariant note in the dispatch comment.

### 4.4 LocType lookup

The Server already exposes `s.locTypeOrNil(typeId)` (used at `script_loc_ops.go:31` and elsewhere). NAI-91 reuses it. If the lookup returns nil (e.g., a loc whose type id is out of range or the LocTypeConfigs slice has a hole), `forceapproach=0` is the safe default — `reach.Reached` treats it as "no extra access restriction", matching TS behavior when LocType.forceapproach is 0/unset. Plan-author MUST grep for the actual server-receiver naming on the NPC side (`s` vs `srv` per `controller_preflight` memory) and confirm `n.server` access path (likely `n.server` based on existing usage at `n.server.gamemap` per the FlagMap grep, but plan-author re-greps).

## 5. Tests (TDD)

Per `superpowers:test-driven-development`. Failing tests written first, run RED, then implement.

### 5.1 Player-side tests in `modules/world/interaction_test.go`

Matrix per loc shape+angle:

| Shape | Angle | Cases |
|---|---|---|
| wall_straight (0) | west (0) | on-tile, west-adjacent, north-adjacent (gated by FlagBlockNorth on src tile), south-adjacent (gated by FlagBlockSouth), east-adjacent (false), diagonal (false) |
| wall_straight (0) | north (1) | on-tile, north-adjacent, west-adjacent (gated), east-adjacent (gated), south-adjacent (false) |
| wall_straight (0) | east (2) | on-tile, east-adjacent, north-adjacent (gated), south-adjacent (gated), west-adjacent (false) |
| wall_straight (0) | south (3) | on-tile, south-adjacent, west-adjacent (gated), east-adjacent (gated), north-adjacent (false) |
| wall_l (2) | west (0) | on-tile, west-adjacent, north-adjacent, east-adjacent (gated), south-adjacent (gated) |
| wall_diagonal (9) | any | on-tile, four orthogonal-with-flag cases |

Cross-cutting assertions per case:
- The test fixture wires a real `pkg/pathfinder/collision.FlagMap` with the relevant `FlagBlock*` flags set/unset.
- Asserts `inOperableDistance(p, loc)` returns expected bool.
- Builds the Loc via `entitypkg.NewLoc(level, x, z, w=1, l=1, lifecycle, typ, shape, angle)` (same import alias as production code).

The matrix is mechanically derived from `pkg/pathfinder/reach/strategy.go:ReachWall1` semantics. Plan-author copies the existing `strategy_test.go` table-driven idiom rather than inventing a new shape.

**Pin the user's actual symptom.** Add one named test:

```go
func TestPlayer_InOperableDistance_DoorTile_AllowsReClick(t *testing.T) {
    // Tutorial Island RS Guide door re-click case (NAI-91 root symptom).
    // Player on the door tile clicking the door. wall_straight angle=west,
    // 1×1 footprint. Pre-NAI-91 returned false; post-NAI-91 returns true.
    ...
}
```

### 5.2 NPC-side tests in `modules/world/npc_interaction_test.go`

Mirror the player-side matrix. Plan-author parameterizes by `srcSize` ∈ {1, 2} to exercise both `ReachWall1` and `reachWallN`. Cross-foot against `pkg/pathfinder/reach/strategy_test.go` fixtures.

### 5.3 Test count budget

~16 player-side cases (4 shapes × ~4 angles), ~12 NPC-side mirror cases. Total ≤ ~30 new test functions; matrix-driven via `[]struct` fixtures so the LOC stays bounded.

## 6. File map

**Modified:**
- `modules/world/interaction.go` — replace `inOperableDistance` body + caller at line 381.
- `modules/world/npc_interaction.go` — replace `(*Npc).inOperableDistance` body; rewrite the doc-comment block at lines 525-528 to drop the (now-fixed) Loc-side claim and to scope the residual under the new `NAI-91-D-OPERABLE-CHEB-FALLBACK` tag (entity-shape + Obj). The "S6l-D4 posture" reference is removed from this site since S6l-D4's actual scope (LOS-in-approach) is unrelated.
- `modules/world/interaction_test.go` — append player-side matrix tests + on-tile pin.
- `modules/world/npc_interaction_test.go` — append NPC-side matrix tests.

**Touched comments only (deviation tracker):**
- Plan-author runs `rg "S6l-D4" pkg/ modules/ cmd/` per `retire_deviation_grep_all_comments` memory. **Expected:** S6l-D4 references at `interaction.go:477` (LOS-in-approach — *unrelated; do not touch*) and `npc_interaction.go:528` (the only site we rewrite, see above). If grep surfaces additional sites, plan-author audits them: any site claiming S6l-D4 covers a *shape* gap is mistagged and should be re-scoped to `NAI-91-D-OPERABLE-CHEB-FALLBACK`; any site claiming S6l-D4 for *LOS* is correct and stays.

**Created:** None.

## 7. Smoke protocol (out-of-band)

User-launched per `smoke_test_server_handoff` memory. Default config (`NodeDebug=true`).

1. Java client login at default Tutorial Island spawn.
2. Walk to RS Guide door (`loc_3014`, `(3098,3107,0)`); click once. **Expected:** player ends on door tile (3098,3107). *(Same as pre-NAI-91; this is the designed first-click.)*
3. From door tile (3098,3107), click door again. **Expected (the binding test):** `~check_axis=true` → `$entering=true` → throughwalk fires → player ends on (3097,3107) west of wall.
4. Repeat once more from (3097,3107) — confirm enter still works (regression check).
5. Walk to a Tutorial Island ladder if accessible (cross-shape smoke; ladder is `wall_l` or rectangular). Click; confirm interaction fires.
6. Re-attempt Survival Expert NPC (typeId=943) interaction. **Expected:** likely still blocked (NPC-side pathing across cabin wall is a *path-finding* gap, not an *operable-distance* gap). If unexpectedly resolved, fold note into close commit; otherwise NAI-92 unchanged.
7. Capture `goscape.log` for the click ticks; user pastes evidence.

If smoke (3) fails (door re-click still gives "I can't reach that!"), the diagnosis is wrong — return to brainstorm with new evidence, do not patch around. Per `cascade_theory_smoke_binding` memory: smoke is binding.

## 8. Tracked deviations

**Untouched:** `S6l-D4` (LOS-in-approach at `interaction.go:477`) — scope is unrelated to the shape gap NAI-91 fixes; the "shape" framing in `npc_interaction.go:528`'s comment was a loose aggregation, not a formal extension of S6l-D4.

**New:** `NAI-91-D-OPERABLE-CHEB-FALLBACK` — the residual `inOperableDistance` Chebyshev≤1 fallback retained for PathingEntity (`*Player`, `*Npc`) and Obj targets, pending TS `reachedEntity` / `reachedObj` ports. Lives in:
- `modules/world/interaction.go` (`inOperableDistanceCheb` body comment).
- `modules/world/npc_interaction.go` (`(*Npc).inOperableDistance` default-arm comment).

Routes to a future sub-spec when smoke surfaces a need (e.g., multi-tile NPCs whose adjacency-via-corner currently false-rejects, or stackable Obj reach edge cases). Per `nai_followups.md` carry-forward convention.

## 9. Risk register

- **R1 — Loc Width/Length pre-rotation.** Resolved at brainstorm: callers pass absolute (un-rotated) dimensions; `reach.Reached` rotates internally. No double-rotation. Documented as invariant comment in §4.1's dispatch.
- **R2 — `forceapproach` plumbing.** New per-call lookup of `LocType.ForceApproach`. Same access pattern as existing `script_loc_ops.go:31` `s.locTypeOrNil`; nil-safe (`fap=0` fallback). No new hot-path cost.
- **R3 — NPC `srcSize > 1` paths into `reachWallN`.** Not new code; already unit-tested at `pkg/pathfinder/reach/strategy_test.go`. Trust existing coverage.
- **R4 — Smoke regression.** Door re-click might cascade-surface a downstream bug (e.g., loc revert timing now matters because the player is no longer wedged). Mitigation: end-of-bundle smoke + `pkg/pathfinder/reach` tests. If a downstream bug surfaces, route per `smoke_surfaces_adjacent_divergences`.
- **R5 — NPC server access path.** NPC-side `n.server` may differ in shape from `n.server.gamemap`. Plan-author runs `controller_preflight` grep to confirm receiver name (`s` vs `srv` vs `n.server` etc.) and exact field path before codifying.
- **R6 — Hidden free-function callers of `inOperableDistance`.** The signature changes from `(int, int, int, int)` to `(*Player, entity)`. Plan-author runs `rg "inOperableDistance\(" modules/` and updates all sites, including any test fixtures.

## 10. Out of scope (explicit deferrals)

- Obj-side `reachedObj` port (NAI-91-D-OPERABLE-CHEB-FALLBACK residual).
- Entity-shape `reachedEntity` port for `*Player` / `*Npc` targets (NAI-91-D-OPERABLE-CHEB-FALLBACK residual).
- Survival Expert NPC pathing across walls (NAI-92 candidate; different mechanism — pathfinder shape-blindness, not reach-gate shape-blindness).
- Full TS `Player.pathToTarget` shape-aware port (NAI-90 H1; not required for this user-symptom because the proc handles throughwalk once dispatched).
- NPC's own `inApproachDistance` shape-awareness (separate gate; carries the actual S6l-D4 LOS deviation forward unchanged plus a future-scope shape-port adjacent to NAI-91-D-OPERABLE-CHEB-FALLBACK).
- `forceapproach` semantic-correctness audit (the field plumbs through but a deep audit of LocType data correctness is out of scope; we trust the cache load).

## 11. Test strategy summary

- TDD per task: write RED tests first, implement, run GREEN.
- Player-side: matrix-driven table tests in `interaction_test.go` covering wall_straight × 4 angles + wall_l + wall_diagonal. Plus the on-tile pin test naming the user's symptom.
- NPC-side: mirror matrix in `npc_interaction_test.go`, parameterized by `srcSize`.
- No new fixtures outside `modules/world/`; rely on `pkg/pathfinder/reach/strategy_test.go` for the underlying reach predicate coverage.
- End-of-bundle race build (`go test -race ./...`) and binary build to confirm no compile/test regression elsewhere.
- Smoke is binding for "does the door exit work" per `cascade_theory_smoke_binding`.
