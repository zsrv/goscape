# NAI-173 — Port TS reachedEntity to PathingEntity arm of inOperableDistance

**Date:** 2026-05-11
**Status:** Design
**Tracker:** Retires `NAI-91-D-OPERABLE-CHEB-FALLBACK`
**Predecessors:** NAI-91 (Loc shape-aware reach), NAI-152 B2 (Obj OR-chain on player; Obj-only on npc)
**HEAD at design:** `5d16e9f` (top of main, post-NAI-172 close)

## 1. Problem

`modules/world/interaction.go:663` and `modules/world/npc_interaction.go:720`
fall through to `inOperableDistanceCheb` (Chebyshev≤1, excludes same-tile)
when the operable target is another `*Player` or `*Npc`. TS dispatches that
arm through `reachedEntity` — `reach.Reached(..., locAngle=0, locShape=-2,
blockAccessFlags=0)` — i.e. the `rectangleExclusiveStrategy` with the target's
own width/length and the source's own size.

The Chebyshev fallback diverges from TS in two ways:

1. **No collision-flag check.** Chebyshev passes any 1-tile orthogonal /
   diagonal pair regardless of intervening WALK_BLOCKED flags; TS
   `reachedEntity` consults the flag map via `reachRectangle1`.
2. **No shape awareness.** Multi-tile NPC targets (size > 1) collapse to
   center-coord Chebyshev under the fallback; TS reaches against the full
   target rectangle.

Today both Player and Npc widths are 1, but NPC `size` can exceed 1 per
`npctype` cache (`Npc.size` field at `modules/world/npc.go:131`), so the
shape gap is reachable in production.

## 2. Goal

Port the PathingEntity arm of `inOperableDistance` to call
`pkg/pathfinder/reach.Reached(..., 0, -2, 0)` on both Player and Npc sides,
matching TS `PathingEntity.inOperableDistance` (PathingEntity.ts:378-390)
and TS `Player.inOperableDistance` (Player.ts:1099-1111). Retire the
`NAI-91-D-OPERABLE-CHEB-FALLBACK` deviation tag.

## 3. Out of scope

- The Obj OR-chain (Player.ts:1110 — `reachedEntity || reachedObj`) is
  already mirrored at `interaction.go:656-661` (NAI-152 B2 T2). No change.
- The Npc Obj branch (PathingEntity.ts:389 base — `reachedObj` only) is
  already mirrored at `npc_interaction.go:705-716` (NAI-152 B2 T3). No
  change.
- `inApproachDistance` (the AP-side Chebyshev predicate) is governed by a
  separate deviation (S6l-D4 LOS) and is out of scope for this sub-spec.

## 4. TS source of truth

**Base class — Npc inherits this** (PathingEntity.ts:378-390):

```ts
inOperableDistance(target: Entity): boolean {
    if (target.level !== this.level) return false;
    if (target instanceof PathingEntity) {
        return reachedEntity(this.level, this.x, this.z, target.x, target.z, target.width, target.length, this.width);
    } else if (target instanceof Loc) {
        const forceapproach = LocType.get(target.type).forceapproach;
        return reachedLoc(...);
    }
    // instanceof Obj
    return reachedObj(this.level, this.x, this.z, target.x, target.z, target.width, target.length, this.width);
}
```

**Player override** (Player.ts:1099-1111): identical, except the Obj arm is
`reachedEntity || reachedObj` (already mirrored, see §3).

**reachedEntity** (GameMap.ts:394-396):

```ts
export function reachedEntity(level, srcX, srcZ, destX, destZ, destWidth, destHeight, srcSize): boolean {
    return rsmod.reached(level, srcX, srcZ, destX, destZ, destWidth, destHeight, srcSize, 0, -2, 0);
}
```

i.e. `locAngle=0`, `locShape=-2`, `blockAccessFlags=0`. `locShape=-2` selects
`rectangleExclusiveStrategy` in goscape's port (`pkg/pathfinder/reach/strategy.go:14`),
which on a 1×1 same-tile case returns false (Collides → !collides=false) and
on adjacent open terrain returns true via `reachRectangle1`'s walk-flag check.

## 5. Design

### 5.1 Player-side change (`modules/world/interaction.go`)

Insert a PathingEntity arm before the final fallthrough at line 663:

```go
if t, ok := target.(pathingEntity); ok {
    srv := p.client.server
    if srv.gamemap == nil {
        return inOperableDistanceCheb(p.x, p.z, tx, tz)
    }
    flags := srv.gamemap.Pathfinder.Flags
    return reach.Reached(flags, p.level, p.x, p.z, tx, tz,
        t.Width(), t.Length(), p.Width(), 0, -2, 0)
}
return inOperableDistanceCheb(p.x, p.z, tx, tz) // unreachable in production
```

The trailing `inOperableDistanceCheb` call survives only as a guard for
non-`pathingEntity`, non-`*Loc`, non-`*Obj` test doubles. Production
`p.target` is always one of these four types.

### 5.2 Npc-side change (`modules/world/npc_interaction.go`)

Insert a PathingEntity arm before the final fallthrough at line 720:

```go
if t, ok := target.(pathingEntity); ok && n.server != nil && n.server.gamemap != nil {
    flags := n.server.gamemap.Pathfinder.Flags
    srcSize := n.size
    if srcSize <= 0 {
        srcSize = 1
    }
    return reach.Reached(flags, n.level, n.x, n.z, tx, tz,
        t.Width(), t.Length(), srcSize, 0, -2, 0)
}
return inOperableDistanceCheb(n.x, n.z, tx, tz) // defensive: nil server / gamemap
```

Mirrors the existing Loc/Obj guard pattern at npc_interaction.go:692,705.

### 5.3 Doc-comment + tag retirement

Sites enumerated via `rg "NAI-91-D-OPERABLE-CHEB-FALLBACK" pkg/ modules/ cmd/`
(per `retire_deviation_grep_all_comments.md`):

- `interaction.go:613-615` — swap "PathingEntity… fall through to
  inOperableDistanceCheb… (DEVIATION NAI-91-D-OPERABLE-CHEB-FALLBACK)" bullet
  for "PathingEntity targets dispatch to reach.Reached with locShape=-2
  (reachedEntity) (NAI-173)."
- `interaction.go:666-669` — re-label `inOperableDistanceCheb` doc-comment
  as defensive-only: "Goscape-defensive Chebyshev≤1 fallback for the
  nil-gamemap test path. Production never reaches this since NAI-91 (Loc),
  NAI-152 B2 (Obj), and NAI-173 (PathingEntity) cover all production target
  types via reach.Reached."
- `npc_interaction.go:680-682` — same bullet swap as `interaction.go:613-615`.
- `npc_interaction.go:718-720` — drop the "Chebyshev fallback
  (NAI-91-D-OPERABLE-CHEB-FALLBACK)" comment; replace with "Defensive
  fallback for nil server / nil gamemap (test fixtures)."
- `interaction_test.go:268-271` — rewrite `TestInOperableDistanceCheb_PathingEntityFallback`
  doc-comment per §7.3 (replace "Lives under NAI-91-D-OPERABLE-CHEB-FALLBACK
  pending entity-shape port" with defensive-fallback framing).
- `interaction_test.go:2553` — historical reference inside NAI-152 B2
  doc-block ("Retires the Obj clause of…"). Leave as-is — accurately
  describes B2's scope at the time.

### 5.4 Affected call sites (audit complete)

Production callers of `inOperableDistance`:
- `modules/world/interaction.go:410` (player tick) — now sees shape-aware result.
- `modules/world/npc_interaction.go:256` (npc tick) — now sees shape-aware result.

Test callers (must continue to pass / get new fixtures):
- `modules/world/interaction_test.go:319` — Loc/Obj tests, unaffected.
- `modules/world/interaction_test.go:1813,1907,1926,1940,1965,2589,2608,2626,2640` — existing
  Loc/Obj/Npc-target tests. Line 1965's npc-target test is the existing
  Chebyshev pin and MAY shift; verify in T1.
- `modules/world/npc_interaction_test.go:751` — npc-side existing pin; verify.

Tracker entry to retire: `nai_followups.md` line ~5149 — the
`NAI-91-D-OPERABLE-CHEB-FALLBACK` line referencing "Chebyshev≤1 fallback
retained for *Player / *Npc / *Obj targets… pending TS reachedEntity /
reachedObj ports." The Obj remainder was already closed by NAI-152 B2; this
sub-spec closes the Player + Npc remainder, retiring the entry entirely.

## 6. Behavioral diff vs Chebyshev

For the production case (`srv.gamemap != nil`, 1×1 entities, open terrain
with walk flags allocated to 0):

| Geometry          | Chebyshev | reach.Reached(...,-2,...) | Match? |
|-------------------|-----------|---------------------------|--------|
| Same-tile         | false     | false (Collides → !collides=false) | ✓      |
| Orth ±1, flag=0   | true      | true (rect1 perimeter arm hit, wall-bit clear) | ✓      |
| **Diag ±1**       | **true**  | **false** (rect1 has NO diagonal arm) | **✗ TS-faithful** |
| Distance ≥2       | false     | false                     | ✓      |

Two real semantic differences from Chebyshev — both TS-faithful:

1. **Diagonals reject.** `reachRectangle1` (rectangularbounds.go:15-48) has
   four orthogonal-perimeter arms only. A 1×1 source diagonally adjacent to a
   1×1 destination matches none of them and falls through to false. Players
   diagonally next to an NPC are NOT in operable distance per TS reachedEntity.
2. **Walk-flags consulted.** `reachRectangle1` reads
   `flags.Get(srcX, srcZ, level) & FlagWallSouth/North/East/West`. With an
   empty FlagMap (`FlagNull=-1`, all bits set), every wall-bit appears set
   and every reach attempt returns false (`empty_flagmap_degenerate_routefinder.md`).
   Test fixtures MUST `AllocateIfAbsent(srcX, srcZ, level)` to clear flags
   to 0, mirroring the existing Obj fixture at `interaction_test.go:2601`.

Multi-tile NPC targets (npc.size > 1) additionally exercise the
`destWidth/destLength` rectangle math — center-coord Chebyshev underestimates
reach against a 2×2 NPC's far edge.

## 7. Test plan

### 7.1 Player-side pins (`modules/world/interaction_test.go`)

New table `TestPlayer_InOperableDistance_PathingEntity_Reach`. Use the
existing `newInOperableTestServer(t)` fixture (initialized gamemap) AND
`s.gamemap.Pathfinder.Flags.AllocateIfAbsent(srcX, srcZ, level)` for each
src tile so wall-flags are 0 (per `empty_flagmap_degenerate_routefinder.md`).

1. **player→npc same-tile** → false. Pins ReachExclusiveRectangle Collides.
2. **player→npc adjacent N (orth)** → true. flags allocated at src.
3. **player→npc adjacent E (orth)** → true.
4. **player→npc adjacent NE (diag)** → **false** (TS-faithful — `reachRectangle1` has no diagonal arm). PINS the new semantic divergence from Chebyshev.
5. **player→player adjacent N** → true. Covers `*Player` arm of pathingEntity assertion.
6. **player→npc distance 2 east** → false.
7. **player→npc cross-level** → false (level guard).
8. **player→npc multi-tile target (npc.size=2, src 1 tile west of west edge)** → true. Pins shape-aware reach against the far edge.

### 7.2 Npc-side pins (`modules/world/npc_interaction_test.go`)

New table `TestNpc_InOperableDistance_PathingEntity_Reach`. Mirror the same
8-row set with `n.server` set to a fixture-initialized gamemap and
`AllocateIfAbsent` for the src tile. Cover npc→player and npc→npc adjacent
+ same-tile + diagonal-rejects + multi-tile (set `n.size=2` for the source-side
multi-tile row to exercise the `srcSize` divergence).

### 7.3 Existing pin updates

- **`interaction_test.go:1947 TestPlayer_InOperableDistance_NpcTarget_UsesCheb`**:
  This test uses `newInOperableTestServer` (gamemap initialized but flags
  unallocated). After NAI-173, the "adjacent" row will FAIL — `reachRectangle1`
  reads `flags.Get(100,101,0)` which returns `FlagNull=-1` (all bits set,
  including FlagWallSouth) → false. Action:
  - **Rename** to `TestPlayer_InOperableDistance_NpcTarget_DefensiveFallback`
    OR replace it entirely; keeping the "UsesCheb" name is misleading post-NAI-173.
  - **Replace coverage** by the new §7.1 table (which exercises the production
    reach path with flags allocated). Delete this test once §7.1 lands.

- **`npc_interaction_test.go:734 TestNpcInOperableDistance`**:
  Uses `NewNpc(1, 42, ...)` with no server attached. After NAI-173, n.server
  is still nil → defensive Chebyshev arm → all four existing rows
  (same-tile, adjacent N, diagonal NE, two tiles away) PASS UNCHANGED.
  - **Action**: rename to `TestNpcInOperableDistance_DefensiveFallback`,
    update the doc-comment to make the defensive role explicit per
    `defensive_gate_doc_comment_label.md`. Test body unchanged.

- **`interaction_test.go:268 TestInOperableDistanceCheb_PathingEntityFallback`**:
  Tests the standalone `inOperableDistanceCheb` function. Function survives
  as defensive-only post-NAI-173. Update doc-comment to drop "pending
  entity-shape port" framing; replace with "exercises the goscape-defensive
  nil-gamemap fallback retained post-NAI-173."

### 7.4 Smoke

User-driven Java client (per `smoke_test_server_handoff.md`):
1. Tutorial Island chicken combat — player attacks adjacent chicken; verify
   damage XP (4 per damage point per `combat_xp_per_damage.md`).
2. Player follow: two clients, one selects /follow on the other; verify
   chase converges to adjacent.
3. NPC aggression: walk near an aggressive NPC; verify it engages on
   adjacency, not on same-tile.

If any smoke regresses, escalate per `dispatch_correct_reach_blocked.md` —
this sub-spec is the dispatch fix; pathfinding is upstream.

## 8. Risks + premise audit

| Risk                                                             | Mitigation                                                                                          |
|------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------|
| `pathingEntity` interface excludes a future test double          | Trailing `return inOperableDistanceCheb(...)` survives as defensive fallthrough (re-purposed).      |
| Existing test rows shift when migrated to reach-based check       | T1 audits `interaction_test.go:1965` + `npc_interaction_test.go:751` line-by-line per `latent_bug_at_migration_boundary.md`. |
| `inOperableDistanceCheb` becomes dead production code             | Retain (re-labeled defensive-only). Pre-existing test at `interaction_test.go:289` still exercises it directly. |
| Shape-aware reach changes for multi-tile NPC targets               | TS-faithful change. Smoke step 3 (NPC aggression) covers it.                                        |
| Doc-comment audit misses a NAI-91-D reference                      | T4 runs `rg "NAI-91-D-OPERABLE-CHEB-FALLBACK" pkg/ modules/ cmd/ docs/` per `retire_deviation_grep_all_comments.md`. |

## 9. Cadence

`runescript_cadence` — full spec → plan → subagent-driven impl → two-stage
review → close. Not compressed (~30-50 production LOC + 10-12 test pins +
doc-comment edits + smoke handoff). Stage 1 audit already happened in the
NAI-91 close-doc; this is the Stage 2 fix.

## 10. Deviations

None planned.

- Goscape-defensive `inOperableDistanceCheb` fallback survives solely for
  the nil-gamemap test path, labeled per `defensive_gate_doc_comment_label.md`.
- The diagonal-rejection behavior (§6 row 3) is TS-faithful and not a
  deviation; it is a pre-existing latent semantic difference exposed by the
  port (per `latent_bug_at_migration_boundary.md`). Smoke step 3 (NPC
  aggression on diagonal approach) covers it.

## 11. Tech stack

Go 1.26+ per `go_version.md`. No new dependencies. `pkg/pathfinder/reach.Reached`
already imported at `interaction.go` (NAI-91) and `npc_interaction.go` (NAI-91).
