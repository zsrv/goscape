# NAI-12 — CheckVis / Line-of-Sight Unification

Wire `CheckVis` (Line-of-Sight / Line-of-Walk) gating across the four hunt
variants (PLAYER / NPC / OBJ / SCENERY) and the NPC-side `inApproachDistance`
range check. Closes five known fidelity deferrals from NAI-8 and NAI-11 in one
cohesive slice, using the existing `s.gamemap.Pathfinder.LineValidator`
infrastructure — no new types, no new packages.

Part of the NPC AI tick decomposition roadmap
(`docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`).
Blockers: NAI-8 (huntPlayers filter chain — TODO breadcrumb at
`modules/world/npc_hunt.go:138`), NAI-9 (huntNpcs/huntObjs/huntLocs — TODO
breadcrumbs at `modules/world/npc_hunt_entities.go:61,119,183`), NAI-11
(`inApproachDistance` range-only posture at
`modules/world/npc_interaction.go`). Roadmap fidelity risk: **Medium** — the
two TS argument-order quirks (huntPlayers src/dest swap;
`inApproachDistance` NPC-backward LoS) must be preserved exactly. See
`nai_followups.md` entries "Deferred: CheckVis (LoS/LoW) gate on all four
hunt variants" and "Deferred: LoS gating in inApproachDistance".

**Tech Stack:** Go 1.26+, existing `pkg/pathfinder/routefinder.LineValidator`
(with `HasLineOfSight` / `HasLineOfWalk` methods), existing
`pkg/pathfinder/collision` (`FlagMap`, `FlagLoc`, `FlagBlockPlayers`,
`FlagLocProjBlocker`, directional wall blocker flags), existing
`pkg/gamemap.GameMap` wrapping a `*routefinder.PathFinderAPI`, existing
`pkg/objtype` hunt constants (`HuntVisOff = 0`, `HuntVisLineOfSight = 1`,
`HuntVisLineOfWalk = 2`).

## Goal

After NAI-12 ships:

1. All four hunt variants (`huntPlayers`, `huntNpcs`, `huntObjs`, `huntLocs`)
   apply the `CheckVis` gate per TS `ScriptIterators.ts:88-94`, `113-118`,
   `137-142`, `160-165` — replacing the four `// TODO: CheckVis gate`
   breadcrumbs at `modules/world/npc_hunt.go:138` and
   `modules/world/npc_hunt_entities.go:61,119,183`.
2. `huntPlayers` preserves the TS argument-order quirk: the LoS/LoW call
   passes player-as-source (`p.x, p.z, n.x, n.z`) — opposite of the other
   three variants' NPC-as-source convention. A fidelity comment at the call
   site documents the asymmetry.
3. `(*Npc).inApproachDistance` applies the TS-native NPC-backward LoS gate
   mirroring `PathingEntity.ts:402-405`. Source and dest are swapped (target
   as source, self as dest) and the extra flag is
   `collision.FlagBlockPlayers`, matching TS `isApproached`'s use of
   `CollisionFlag.PLAYER` at `GameMap.ts:433-435`.
4. All five sites short-circuit to gate-pass when `s.gamemap == nil`,
   preserving existing test fixtures that don't wire a gamemap. Test
   fixtures that *do* exercise the gate construct `s.gamemap = gamemap.New(...)`
   and mutate collision flags directly.
5. 13 new tests assert pass + block per site and guard the two
   argument-order quirks against future accidental reversal.
6. Two memory entries in `nai_followups.md` are updated to "Resolved
   2026-04-23 (NAI-12)": the NAI-8 CheckVis deferral and the NAI-11
   `inApproachDistance` LoS deferral.

## Scope — what's IN

### Hunt-variant gates (4 sites)

1. **`huntPlayers` CheckVis gate** at `modules/world/npc_hunt.go` —
   replaces TODO at L138. Call shape:
   `HasLineOfSight(n.level, p.x, p.z, n.x, n.z, 1, 1, 1, 0)` for
   `HuntVisLineOfSight`; `HasLineOfWalk` with identical args for
   `HuntVisLineOfWalk`. Preserves TS src/dest swap
   (`ScriptIterators.ts:88-94`).

2. **`huntNpcs` CheckVis gate** at
   `modules/world/npc_hunt_entities.go` — replaces TODO at L61. Call
   shape: `HasLineOfSight(n.level, n.x, n.z, other.x, other.z, 1, 1, 1, 0)`.
   TS NPC-as-source convention (`ScriptIterators.ts:113-118`).

3. **`huntObjs` CheckVis gate** at
   `modules/world/npc_hunt_entities.go` — replaces TODO at L119. Call
   shape: `HasLineOfSight(n.level, n.x, n.z, o.X, o.Z, 1, 1, 1, 0)`.
   Per `ScriptIterators.ts:137-142`.

4. **`huntLocs` CheckVis gate** at
   `modules/world/npc_hunt_entities.go` — replaces TODO at L183. Call
   shape: `HasLineOfSight(n.level, n.x, n.z, l.X, l.Z, 1, 1, 1, 0)`.
   Loc SW-corner is used per TS convention (TS passes `{x: loc.x, z: loc.z}`
   — `ScriptIterators.ts:160-165`). Multi-tile locs are NOT treated as
   multi-tile by TS here; goscape preserves that quirk.

### `inApproachDistance` LoS gate (1 site)

5. **`(*Npc).inApproachDistance` LoS gate** at
   `modules/world/npc_interaction.go`. Extends the existing
   Chebyshev-only range check (`inApproachDistance` at lines 482-502)
   with an AND'd LoS call matching TS `PathingEntity.ts:402-405`. The
   gate lands **between** the Chebyshev range check and the existing
   final `return !(dx == 0 && dz == 0)`:

   ```go
   // LoS gate — TS PathingEntity.ts:402-405.
   // FIDELITY: "Los for Npcs is always calculated backwards for all
   // Entity types" — source is target, dest is self. TS's isApproached
   // (GameMap.ts:433-435) dispatches to hasLineOfSight with
   // CollisionFlag.PLAYER as extraFlag — Go equivalent
   // collision.FlagBlockPlayers.
   // gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
   // DEVIATION: TS passes target.width+target.length and this.width+this.length
   // (four size args). Go's HasLineOfSight collapses src to scalar srcSize;
   // NAI-12 approximates with srcSize=1, destWidth=1, destLength=1 matching
   // the hunt-variant convention. Tracks as future size-aware follow-up.
   if s != nil && s.gamemap != nil &&
       !s.gamemap.Pathfinder.LineValidator.HasLineOfSight(
           n.level, tx, tz, n.x, n.z, 1, 1, 1, collision.FlagBlockPlayers) {
       return false
   }
   ```

   Preserves the TS NPC-backward src/dest swap AND the
   `CollisionFlag.PLAYER` extra flag.

### Inline gate implementation pattern (hunt variants)

Single pattern applied to all four hunt sites:

```go
// CheckVis gate — TS ScriptIterators.ts:<lines>.
// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
if hunt.CheckVis == objtype.HuntVisLineOfSight && s.gamemap != nil &&
    !s.gamemap.Pathfinder.LineValidator.HasLineOfSight(
        n.level, srcX, srcZ, destX, destZ, 1, 1, 1, 0) {
    continue
}
if hunt.CheckVis == objtype.HuntVisLineOfWalk && s.gamemap != nil &&
    !s.gamemap.Pathfinder.LineValidator.HasLineOfWalk(
        n.level, srcX, srcZ, destX, destZ, 1, 1, 1, 0) {
    continue
}
```

For `huntPlayers`, `srcX/srcZ` are `p.x/p.z` and `destX/destZ` are
`n.x/n.z` — with a FIDELITY comment calling out the asymmetry vs the
other three variants.

### Memory updates

6. **`nai_followups.md`** — mark the NAI-8 CheckVis deferral and the
   NAI-11 `inApproachDistance` deferral as "Resolved 2026-04-23 (NAI-12)"
   with pointer to this spec.

## Scope — what's OUT (non-goals)

1. **Reach helpers (`reachedEntity`/`reachedLoc`/`reachedObj`)** —
   `inOperableDistance` stays on Chebyshev-only. The NAI-11 deferral
   "Deferred: reach helpers" remains open. Those helpers are structurally
   independent from LoS (they handle shape/angle/forceapproach semantics,
   not visibility).
2. **SMART pathfinding** — `pathToTarget` stays naive-only. NAI-11
   deferral "Deferred: SMART pathfinding branch in pathToTarget" remains
   open.
3. **PLAYER\* modes** — out of scope; stays deferred. The flipped
   `TestNpcTurnHuntAndConsumeSetsTarget` assertion from NAI-11 stays
   flipped.
4. **Hunt filter backfill (5 remaining filters)** — the NAI-8 deferral
   "Deferred filters in huntPlayers (future audit)" for checkNotBusy,
   checkNotTooStrong, checkNotCombat, checkNotCombatSelf, checkVars,
   checkInv remains open; each has an independent Go-side infra
   prerequisite.
5. **NumberNotNull opcode gates** — orthogonal audit track, remains open.
6. **Multi-tile loc LoS using real width/length** — TS uses `{loc.x, loc.z}`
   (1×1) at `ScriptIterators.ts:160-165`. goscape preserves that exactly.
7. **Size-aware `inApproachDistance` LoS** — TS passes
   `target.width, target.length, this.width, this.length` (four
   separate size dimensions); Go's `HasLineOfSight` collapses src to a
   scalar `srcSize`. NAI-12 approximates with all-1s. A future
   size-aware port would add `Width()`/`Length()` to the `entity`
   interface (or a helper `approachTargetSize(target)`) and thread the
   NPC's `typ.Size` through. Deferred — impact is low because the
   upstream Chebyshev range check already filters most mismatches, and
   multi-tile NPCs are rare in this era. Will live as a new entry in
   `nai_followups.md` post-NAI-12.

8. **Size-aware hunt LoS** — TS `isLineOfSight` at
   `GameMap.ts:429-431` takes no size args (defaults to 1). NAI-12
   preserves the 1,1,1 convention. Not a deviation — TS itself is 1s
   at the hunt-variant layer.

## Architecture

No structural additions. The existing `pkg/gamemap.GameMap` already
exposes `*routefinder.PathFinderAPI` (wired at
`modules/world/server.go:136`); the existing `LineValidator` type
already has `HasLineOfSight` and `HasLineOfWalk` with identical
nine-argument signatures. The sub-spec is purely insertion of
conditional calls at five identified sites.

### Argument-order fidelity table

| Site | TS source line | Source coord | Dest coord | srcSize | destW | destL | extraFlag |
|---|---|---|---|---|---|---|---|
| huntPlayers | `ScriptIterators.ts:88-94` | `p.x, p.z` | `n.x, n.z` | 1 | 1 | 1 | 0 |
| huntNpcs | `ScriptIterators.ts:113-118` | `n.x, n.z` | `other.x, other.z` | 1 | 1 | 1 | 0 |
| huntObjs | `ScriptIterators.ts:137-142` | `n.x, n.z` | `o.X, o.Z` | 1 | 1 | 1 | 0 |
| huntLocs | `ScriptIterators.ts:160-165` | `n.x, n.z` | `l.X, l.Z` | 1 | 1 | 1 | 0 |
| inApproachDistance (NPC branch) | `PathingEntity.ts:402-403` | `target.x, target.z` | `n.x, n.z` | 1† | 1† | 1† | `FlagBlockPlayers` |

† TS passes real sizes (`target.width, target.length, this.width, this.length`).
NAI-12 approximates with 1s — tracked deviation, see § Scope OUT #7.

**Two quirks worth repeating:**

- huntPlayers is the only hunt variant with **player-as-source** — every
  other variant uses NPC-as-source. TS comment at `ScriptIterators.ts:88`
  does not explain why; preserve verbatim.
- inApproachDistance is the only site that uses a non-zero
  `extraFlag`. TS `isApproached` at `GameMap.ts:433-435` dispatches to
  `rsmod.hasLineOfSight(..., CollisionFlag.PLAYER)` — Go equivalent is
  `collision.FlagBlockPlayers`.

## Error handling

### `s.gamemap == nil`

Gate short-circuits to **pass** (treat as LoS clear). This preserves
existing test fixtures that construct `Server` without wiring
`s.gamemap`. Matches the existing pattern at
`modules/world/npc_interaction.go:312` where `CanTravel` is
inline-guarded: `if s != nil && s.gamemap != nil && !s.gamemap.CanTravel(...)`.

Inline comment at each site:

```go
// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
```

### Same-tile src == dest

No explicit guard. Relies on `LineValidator.RayCast` at
`pkg/pathfinder/routefinder/linevalidator.go:50-52` which returns `true`
when `startX == endX && startZ == endZ`. Matches TS behavior.

### What is NOT handled

- Unallocated zones: `FlagMap.IsFlagged` returns 0 for unallocated
  coords → rays through unloaded squares pass freely. Correct behavior.
- Cross-level hunts: already filtered upstream in every variant; the
  LoS call cannot receive a cross-level target.
- Route-blocker flags: `HasLineOfSight` already incorporates
  `FlagLocProjBlocker` via `LineSightBlocked*` constants
  (`pkg/pathfinder/routefinder/linevalidator.go:22-28`). No extra
  wiring needed.

## Data flow

Existing hunt-tick flow (NAI-7, NAI-8, NAI-9) is unchanged at every
layer except the filter chain. The CheckVis gate is a pure filter step
with no state mutation and no cross-tick effect:

```
Npc.turn(s)
  → s.processNpcHunt(n)
    → n.huntNpcs(s, hunt)  [or huntPlayers / huntObjs / huntLocs]
      → loop candidates:
          ├── type filter
          ├── category filter
          ├── Chebyshev range filter
          ├── NEW: CheckVis gate — LineValidator.HasLineOfSight/HasLineOfWalk
          └── append to hunted[]
```

For `inApproachDistance`:

```
Npc.turn(s)
  → n.processMovementInteraction(s)
    → n.aiMode(s) or similar
      → n.inApproachDistance(range, target):
          1. level check
          2. same-tile intersect check (existing)
          3. Chebyshev range check
          4. NEW: LoS gate via HasLineOfSight(backward args, FlagBlockPlayers)
          5. return
```

## Testing strategy

### Shared helper (new)

`modules/world/npc_hunt_entities_test.go`:

```go
// withBlockingWall installs a projectile-blocker flag at (level, x, z)
// on the given Server's gamemap. Used to prove the CheckVis gate blocks
// when the straight-line ray traverses that tile.
//
// Pre-condition: s.gamemap has been constructed via gamemap.New(...).
func withBlockingWall(s *Server, level, x, z int) {
    s.gamemap.Pathfinder.Flags.Add(x, z, level, collision.FlagLoc)
}
```

Existing tests for hunt variants (NAI-7/8/9) that set `hunt.CheckVis = 0`
remain green via the nil-gamemap gate-pass from § Error handling.

### The 13 tests

**`modules/world/npc_hunt_test.go` — 3 tests:**

1. **`TestHuntPlayersCheckVisLineOfSightPasses`** — NPC at (3094, 3106),
   player at (3094, 3108), `hunt.CheckVis = HuntVisLineOfSight`, empty
   flagmap. Assert player is hunted.

2. **`TestHuntPlayersCheckVisLineOfSightBlocks`** — same fixture +
   `withBlockingWall(s, 0, 3094, 3107)`. Assert player is filtered out.

3. **`TestHuntPlayersCheckVisArgumentOrderSwapQuirk`** —
   asymmetric-wall fixture designed so `HasLineOfSight(p.x, p.z, n.x, n.z, ...)`
   returns false but the swapped order
   `HasLineOfSight(n.x, n.z, p.x, p.z, ...)` would return true. Assert
   player is filtered. If implementer reverts the TS swap, this test flips
   red. Concrete shape: use `FlagWallNorthProjBlocker` at a single tile
   such that the ray traversal direction matters.

**`modules/world/npc_hunt_entities_test.go` — 6 tests:**

4. **`TestHuntNpcsCheckVisLineOfSightPasses`** — NPC-type hunt, clear LoS.
5. **`TestHuntNpcsCheckVisLineOfWalkBlocks`** — uses
   `HuntVisLineOfWalk` to prove the LoW dispatch branch (net +0 tests;
   reassigns what would have been a LoS-block test). Blocker via
   `withBlockingWall`.
6. **`TestHuntObjsCheckVisLineOfSightPasses`** — dynamic obj in zone,
   clear LoS.
7. **`TestHuntObjsCheckVisLineOfSightBlocks`** — blocked by wall.
8. **`TestHuntLocsCheckVisLineOfSightPasses`** — loc in zone, clear LoS.
9. **`TestHuntLocsCheckVisLineOfSightBlocks`** — blocked by wall.

**`modules/world/npc_interaction_test.go` — 4 tests:**

10. **`TestNpcInApproachDistanceLosPasses`** — in range + clear LoS →
    true.
11. **`TestNpcInApproachDistanceLosBlocks`** — in range + wall between →
    false.
12. **`TestNpcInApproachDistanceNpcBackwardArgsQuirk`** —
    asymmetric-wall fixture proving `target.x, target.z, n.x, n.z` is
    the call order (TS `PathingEntity.ts:403`). If implementer "fixes"
    to forward order, test fails.
13. **`TestNpcInApproachDistancePlayerFlagIsRespected`** — separate
    NPC from target only by a `FlagBlockPlayers`-only tile (no wall, no
    proj-blocker). Assert `inApproachDistance` returns false. Proves
    `extraFlag = FlagBlockPlayers` is wired — distinguishes
    `isApproached` from plain `isLineOfSight`.

### Test-count justification

| Site | Pass | Block | Quirk | Other | Total |
|---|---|---|---|---|---|
| huntPlayers | 1 | 1 | 1 (swap) | — | 3 |
| huntNpcs | 1 | 1 (LoW) | — | — | 2 |
| huntObjs | 1 | 1 | — | — | 2 |
| huntLocs | 1 | 1 | — | — | 2 |
| inApproachDistance | 1 | 1 | 1 (backward) | 1 (player-flag) | 4 |
| **Total** | **5** | **5** | **2** | **1** | **13** |

LineOfWalk coverage is proven once (test 5) — the underlying
`RayCast` is already exhaustively tested in
`pkg/pathfinder/routefinder/linevalidator_test.go` (20+ tests). No
per-variant LoW duplication.

## Files changed

| File | Change |
|---|---|
| `modules/world/npc_hunt.go` | Replace TODO at L138 with huntPlayers gate (swapped args + fidelity comment) |
| `modules/world/npc_hunt_entities.go` | Replace TODOs at L61, L119, L183 with NPC/OBJ/SCENERY gates |
| `modules/world/npc_interaction.go` | Extend `inApproachDistance` with NPC-backward LoS gate |
| `modules/world/npc_hunt_test.go` | +3 tests (huntPlayers) |
| `modules/world/npc_hunt_entities_test.go` | +6 tests + `withBlockingWall` helper |
| `modules/world/npc_interaction_test.go` | +4 tests |
| `docs/superpowers/specs/2026-04-23-nai-12-checkvis-unified-design.md` | This doc |
| `docs/superpowers/plans/2026-04-23-nai-12-checkvis-unified.md` | Plan (NAI-12 writing-plans handoff) |
| `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` | Mark NAI-8 CheckVis + NAI-11 inApproachDistance as Resolved |

No package-level additions. No mocks added. `pkg/objtype/hunttype.go`
already defines `HuntVisOff`/`HuntVisLineOfSight`/`HuntVisLineOfWalk`.
`pkg/pathfinder/collision` already exports `FlagBlockPlayers`, `FlagLoc`,
and all directional wall blockers.

## References

- TS source (LoS/LoW helpers):
  `LostCityRS/Engine-TS/src/engine/GameMap.ts:425-435` (`isLineOfSight`,
  `isLineOfWalk`, `isApproached`)
- TS source (hunt iteration): `LostCityRS/Engine-TS/src/engine/script/ScriptIterators.ts:72-171`
- TS source (NPC approach): `LostCityRS/Engine-TS/src/engine/entity/PathingEntity.ts:392-406`
- Go LoS API: `pkg/pathfinder/routefinder/linevalidator.go:19-40`
- Go FlagMap: `pkg/pathfinder/collision/flagmap.go` + `pkg/pathfinder/collision/type.go`
- Memory: `nai_followups.md` entries "Deferred: CheckVis (LoS/LoW) gate on
  all four hunt variants" (NAI-8) and "Deferred: LoS gating in
  inApproachDistance" (NAI-11)
