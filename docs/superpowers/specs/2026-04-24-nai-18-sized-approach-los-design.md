# NAI-18 — Size-Aware `inApproachDistance` LoS

Close the NAI-12 "Deferred: size-aware inApproachDistance LoS" follow-up by
threading per-entity-type sizes into `(*Npc).inApproachDistance`'s
`HasLineOfSight` call. Replaces the hard-coded `srcSize=destW=destL=1`
approximation with `approachEntitySize(target)` (target width) and
`int(n.typ.Size)` (self width = self length, NPCs are square). Also
retires the matching DEVIATION comment block and the `nai_followups.md`
tracking entry.

**Scope items:**

1. **Item A — Helper `approachEntitySize(e entity) (width, length int)`.**
   New package-private function in `modules/world/npc_interaction.go`,
   placed adjacent to `(*Npc).inApproachDistance`. Type-switches on
   `*Player` → (1, 1), `*Npc` → (int(typ.Size), int(typ.Size)), default
   → (1, 1). Returns both dimensions for API symmetry with TS's
   `target.width`/`target.length` pair even though current callers
   only consume width (see Item B).

2. **Item B — `(*Npc).inApproachDistance` call-site update.**
   Replaces the `HasLineOfSight(..., 1, 1, 1, ...)` literals at
   `modules/world/npc_interaction.go:548-552` with:
   - `targetSize, _ := approachEntitySize(n.target)` (target-as-src
     width; length dropped because Go's `HasLineOfSight` collapses src
     to scalar).
   - `selfSize := int(n.typ.Size)` (self-as-dest width and length; NPCs
     are square per `NpcType.Size`).
   - Call: `HasLineOfSight(n.level, tx, tz, n.x, n.z, targetSize,
     selfSize, selfSize, collision.FlagBlockPlayers)`.

3. **Item C — Test-fixture hygiene: `Size: 1` on shared NPC builders.**
   The shared test helpers `newNpcForLifecycleTest`
   (`npc_event_queue_test.go:17-25`) and `newNpcAt100`
   (`npc_interaction_test.go:197-203`) construct their `NpcType` literal
   without setting `Size`, silently inheriting the `uint8` zero value.
   Production's `objtype.NewNpcType` defaults `Size: 1` at
   `npctype.go:310`; NAI-12 never surfaced this divergence because
   it passed literal `1`s. Once NAI-18 reads `int(n.typ.Size)`, a
   zero-size NPC feeds `lineCoordinate(a, b, size=0)` which returns
   `a-1` (off-by-one start/end tile) and would silently break NAI-12's
   four `TestNpcInApproachDistance*` tests. Fix: add `Size: 1` to both
   shared builders. No defensive clamp in production code — the
   production invariant is `typ.Size ≥ 1` (NewNpcType default), and the
   fixtures now match that.

4. **Item D — New tests: one direct helper + two sizing-quirk guards.**
   One table-driven test of `approachEntitySize` covering `*Player`,
   `*Npc` with `typ.Size ∈ {1, 2, 3}`, and the default branch. Two
   integration tests against `(*Npc).inApproachDistance` that each
   construct a fixture where size=2 vs. size=1 produces a measurably
   different LoS outcome via `lineCoordinate`'s tile-anchor shift —
   proving the sizing actually flows from helper → LoS call.

5. **Item E — DEVIATION retirement.** Replace the six-line
   `DEVIATION: TS passes target.width+target.length ...` comment block
   at `npc_interaction.go:521-525` with a concise `FIDELITY:` note
   pointing at `approachEntitySize` + noting Go's lossless-for-square
   src-size collapse.

6. **Item F — nai_followups.md entry retirement.** Annotate the
   "From NAI-12 / Deferred: size-aware inApproachDistance LoS" entry
   (at `nai_followups.md:542`) with the established `**Resolved
   2026-04-24 (NAI-18)**` prefix, preserve the original body per the
   NAI-followups convention.

**Roadmap:** NAI-18 is a contained behavioral fidelity fix, ~15 prod
LOC + fixture hygiene. No subsystems added, no new packages, no
interface changes. Fidelity risk: **Low**. The LoS RayCast path is
unchanged; only arguments shift from constant `1`s to per-type reads.
The cost of getting sizing wrong is symmetric for all current entity
types (Player=1 → identical to NAI-12 behavior; Npc=1 identical;
Npc=N>1 now correctly exercises the tile-anchor shift).

**Tech Stack:** Go 1.26+. No new packages or file creations. Touches
`modules/world/npc_interaction.go` (helper + call-site + comment);
`modules/world/npc_event_queue_test.go` (fixture); `modules/world/
npc_interaction_test.go` (fixture + 3 new tests).

## Goal

After NAI-18 ships:

1. A package-private `approachEntitySize(e entity) (width, length int)`
   exists in `modules/world/npc_interaction.go`, returns per-type widths
   mirroring TS `PathingEntity.width`/`.length` semantics, with a default
   branch for test doubles / future non-pathing entities.
2. `(*Npc).inApproachDistance` passes actual target size and self size
   into `HasLineOfSight` instead of the NAI-12-era triple-`1`
   approximation. Current NAI-12 test outcomes are preserved when all
   entities are size 1 (lossless fallback).
3. Multi-tile NPCs (e.g. large monsters with `NpcType.Size > 1`) correctly
   exercise the size-adjusted LoS ray-start (target-as-src) and ray-end
   (self-as-dest) tiles per `lineCoordinate(a, b, size)`.
4. The shared test builders `newNpcForLifecycleTest` and `newNpcAt100`
   set `NpcType.Size: 1` explicitly, matching production's
   `NewNpcType` default. No stray zero-sized NPCs feed the LoS
   pipeline.
5. Three new tests ship: one table-driven helper test, two integration
   tests that each prove a size-2 fixture produces a measurably
   different `inApproachDistance` outcome than the same fixture at
   size 1 (one test exercises target-size, the other self-size).
6. The former six-line `DEVIATION:` comment block above
   `(*Npc).inApproachDistance` is replaced by a four-line `FIDELITY:`
   note; the `nai_followups.md` "From NAI-12" entry is annotated as
   resolved.

## Non-Goals

1. **`HasLineOfSight` API extension.** Go's signature
   `HasLineOfSight(..., srcSize, destW, destL, flag)` collapses src to
   a single scalar (forces `srcWidth = srcLength = srcSize` in the
   underlying `RayCast` call at `linevalidator.go:21`). For all current
   concrete entity types this is lossless — `*Player` and `*Npc` are
   both square (width = length). TS's four-size-arg
   `hasLineOfSight(..., srcW, srcH, destW, destH, ...)` has no
   observable divergence from Go's three-size form as long as sources
   are square, which they always are in this era. Rectangular LoS
   sources would arrive only if Loc-as-source becomes a thing, and
   `(*Npc).inApproachDistance` never takes a Loc as source (`this`
   is always an `*Npc`, square).

2. **`entity` interface extension.** Adding `Width() int` / `Length()
   int` to the `movement_consts.go:45` interface would force every
   concrete entity plus every test double (`fakeEntity`,
   `nonNpcEntity`, `mockTargetEntity`, etc.) to declare both methods
   for exactly one consumer. Punted until a second consumer emerges
   (most likely the NAI-11 reach-helper sub-spec, which will naturally
   land when Loc/Obj concretes arrive).

3. **Hunt-variant LoS calls.** The four `huntPlayers` /
   `huntScenery` / `huntObjs` / `huntNpcs` LoS calls at
   `modules/world/npc_hunt.go` use the same `srcSize=destW=destL=1`
   approximation that NAI-18 fixes for `inApproachDistance`. They are
   deferred to a future broader "entity-geometry-aware LoS" pass — the
   hunt-side fix requires target size for every iterated entity (not
   just `n.target`) and may benefit from the interface extension
   deferred in non-goal 2.

4. **Reach helpers (`reachedEntity` / `reachedLoc` / `reachedObj`).**
   Separate NAI-11 deferral line, geometry-aware but operating on the
   OP-trigger branch. Their scope overlaps the Item B call-site update
   only in that both read `n.typ.Size`; NAI-18 stays strictly on the
   AP-trigger branch.

5. **`*Npc.typ.Size` defensive clamp.** NAI-18 trusts the
   production invariant that `NewNpcType` defaults `Size: 1`. No
   production-code clamp; the fixture hygiene fix (Item C) closes the
   single observed divergence (test builders that neglected to set
   `Size`). Defensive clamping would mask data-loading bugs.

## Tracked Deviations

No new deviations introduced. NAI-18 closes one (the "size-aware"
NAI-12 deferral). The NAI-12-era quirks preserved:

- **NPC-backward LoS arg order** — target-as-src, self-as-dest.
  Preserved exactly; the size args reorder too: `srcSize=targetSize`,
  `destWidth/Length=selfSize`. Test
  `TestNpcInApproachDistanceNpcBackwardArgsQuirk` continues to assert
  this.
- **`CollisionFlag.PLAYER` extraFlag** — preserved as
  `collision.FlagBlockPlayers`. Test
  `TestNpcInApproachDistancePlayerFlagIsRespected` continues to assert
  this.
- **`n.server == nil || n.server.gamemap == nil` short-circuits to
  gate-pass** — preserved. No change to the guard. Scripted tests
  without gamemap wired continue to skip LoS evaluation.

## Architecture

### Helper placement and signature

```go
// approachEntitySize returns target (width, length) for the NPC-side
// LoS sizing call. Mirrors TS PathingEntity.width/length per concrete
// entity type:
//
//   *Player → (1, 1)   players are always square size-1
//   *Npc    → (typ.Size, typ.Size)  NPCs are square; typ.Size is side length
//   default → (1, 1)   test doubles / future non-pathing entities
//
// Length is returned for API symmetry with TS; all current callers
// only consume width because Go's HasLineOfSight collapses src to a
// scalar srcSize. See NAI-18 spec § "Signature collapse is lossless
// for square entities".
func approachEntitySize(e entity) (width, length int) {
    switch t := e.(type) {
    case *Player:
        return 1, 1
    case *Npc:
        size := int(t.typ.Size)
        return size, size
    default:
        return 1, 1
    }
}
```

Placed in `modules/world/npc_interaction.go` immediately above
`(*Npc).inApproachDistance`.

### `(*Npc).inApproachDistance` diff

Before (`npc_interaction.go:548-552`, NAI-12):

```go
if n.server != nil && n.server.gamemap != nil &&
    !n.server.gamemap.Pathfinder.LineValidator.HasLineOfSight(
        n.level, tx, tz, n.x, n.z, 1, 1, 1, collision.FlagBlockPlayers) {
    return false
}
```

After (NAI-18):

```go
targetSize, _ := approachEntitySize(n.target)
selfSize := int(n.typ.Size)
if n.server != nil && n.server.gamemap != nil &&
    !n.server.gamemap.Pathfinder.LineValidator.HasLineOfSight(
        n.level, tx, tz, n.x, n.z, targetSize, selfSize, selfSize,
        collision.FlagBlockPlayers) {
    return false
}
```

### Comment block retirement

The existing `DEVIATION` block at `npc_interaction.go:521-525`:

```go
// DEVIATION: TS passes target.width+target.length and this.width+this.length
// (four size args). Go's HasLineOfSight collapses src to scalar srcSize;
// NAI-12 approximates with srcSize=1, destWidth=1, destLength=1 matching
// the hunt-variant convention. Tracked as size-aware follow-up in
// nai_followups.md.
```

Is replaced by:

```go
// FIDELITY: LoS sizing uses approachEntitySize per target concrete
// type (*Player → 1, *Npc → typ.Size; all current pathing entities
// are square). Go's HasLineOfSight collapses src to a scalar srcSize
// (linevalidator.go:21 forces srcLength = srcWidth in the underlying
// RayCast), which is lossless for square entities. NAI-18 closed the
// NAI-12 tracked size-aware deferral.
```

### Signature collapse is lossless for square entities

Go's `HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize,
destWidth, destLength, extraFlag)` expands internally to
`RayCast(..., srcSize, srcSize, destWidth, destLength, ...)` — i.e.
the src is forced square. TS's `hasLineOfSight(level, srcX, srcZ,
destX, destZ, srcWidth, srcHeight, destWidth, destHeight, ...)` takes
four size args. For all current concrete entity types in this engine:

| Entity  | width | length | square? |
|---------|-------|--------|---------|
| `*Player` | 1   | 1      | yes     |
| `*Npc`    | typ.Size | typ.Size | yes (NPCs are always square) |

No concrete Loc or Obj entity type exists yet in `modules/world/` (see
NAI-11 deferrals). When they do arrive, TS's `target.width !=
target.length` case becomes reachable — but only for OP-trigger reach
helpers (NAI-11), not for AP-trigger `inApproachDistance` where
`this` (the LoS source) is always `*Npc` (square).

## Test-fixture hygiene

### Shared builders needing `Size: 1`

| Fixture | File | Line |
|---------|------|------|
| `newNpcForLifecycleTest` | `npc_event_queue_test.go` | 17-25 |
| `newNpcAt100` | `npc_interaction_test.go` | 197-203 |

Both construct `NpcType{}` without a `Size` field. Fix: add `Size: 1`.

### Why not a defensive clamp?

A `if size < 1 { size = 1 }` in production code would paper over any
future `NpcType` load path that failed to default `Size`. The current
`NewNpcType` at `npctype.go:310` defaults `Size: 1`; the dat-load path
at `npctype.go:193` reads `dat.G1()` which can produce zero for malformed
data. A production clamp would silently hide malformed type data;
better to trust the invariant and let test fixtures match it.

### Scope of fixture audit

Only tests that (a) wire `s.gamemap` AND (b) reach
`(*Npc).inApproachDistance` AND (c) use an inline `NpcType{}` literal
outside the two shared builders are at risk. A spot-audit at plan-TDD
time covers the `TestNpcTryInteractApBranch*` tests and any AP-branch
coverage in `interaction_test.go` / `interaction_trigger_test.go`. The
RED → GREEN TDD cycle surfaces any stragglers immediately (LoS ray
starts one tile off, existing assertion fails).

## Testing strategy

Three new tests added to `modules/world/npc_interaction_test.go`:

### 1. `TestApproachEntitySize` (table-driven, helper unit)

| Case | Input | Expected (width, length) |
|------|-------|--------------------------|
| player | `*Player` | (1, 1) |
| npc size=1 | `*Npc{typ: &NpcType{Size: 1}}` | (1, 1) |
| npc size=2 | `*Npc{typ: &NpcType{Size: 2}}` | (2, 2) |
| npc size=3 | `*Npc{typ: &NpcType{Size: 3}}` | (3, 3) |
| fake entity | existing `fakeEntity` double | (1, 1) |

Proves: type-switch correctness; NPC width always equals length; the
default branch falls through to (1, 1) for test doubles.

### 2. `TestNpcInApproachDistanceMultiTileTargetShiftsLoSStartTile`

**Fixture (target-size quirk guard):**

- Self NPC at (3094, 3108), `typ.Size = 1`.
- Target NPC at (3094, 3106), `typ.Size = 2`.
  - Target-as-src: `lineCoordinate(srcZ=3106, destZ=3108, srcSize=2)`
    = 3107 (start tile shifted to target's N-edge). With size=1, start
    = 3106.
- `FlagLoc` placed at (3094, 3107). `FlagLoc` is checked only at the
  ray start tile (`linevalidator.go:54`), NOT in traversal masks.

**Assertions:**

- With target Size=2: `inApproachDistance(5, target)` returns `false`
  (ray start at 3107 hits FlagLoc).
- Companion sub-test: same fixture but flip target Size to 1 →
  returns `true` (ray starts at 3106, walks through 3107 without
  FlagLoc check in the traversal xFlags/zFlags masks, arrives at
  3108).

Proves: target size actually threads through `approachEntitySize` →
`HasLineOfSight` → ray start tile.

### 3. `TestNpcInApproachDistanceMultiTileSelfShiftsLoSEndTile`

**Fixture (self-size quirk guard):**

- Self NPC at (3094, 3106), `typ.Size = 2`.
- Target Player at (3094, 3108), Size=1.
  - Self-as-dest: `lineCoordinate(destZ=3106, srcZ=3108,
    destLength=2)` = 3107 (end tile shifted by one toward src). With
    Size=1, end = 3106.
- `FlagWallNorthProjBlocker` placed at (3094, 3106). The travelSouth
  zFlags mask (`LineSightBlockedNorth = FlagLocProjBlocker |
  FlagWallNorthProjBlocker`) includes this flag; it's checked on tile
  entry during traversal and is NOT cleared at end-tile (only
  `FlagLocProjBlocker` is end-tile-cleared per `linevalidator.go:112`).

**Assertions:**

- With self Size=2: `inApproachDistance(5, target)` returns `true`
  (ray terminates at endZ=3107, never enters 3106).
- Companion sub-test: same fixture but flip self Size to 1 →
  returns `false` (ray walks 3108 → 3107 → 3106, FlagWallNorth
  triggers on entry to 3106).

Proves: self size actually threads through `int(n.typ.Size)` → both
`destWidth` and `destLength` args of `HasLineOfSight` → ray end tile.

### Existing tests (preserved)

The four NAI-12-era `TestNpcInApproachDistance*` tests
(`LosPasses`, `LosBlocks`, `NpcBackwardArgsQuirk`,
`PlayerFlagIsRespected`) continue to pass unchanged — their implicit
all-size-1 fixture now explicitly sets `Size: 1` via Item C fixture
hygiene, producing identical LoS semantics to the NAI-12 literal
`srcSize=destW=destL=1` call.

## Error Handling

No change from NAI-12. The LoS branch preserves:

- `n.server == nil` → skip LoS, gate passes (test-construction mode).
- `n.server.gamemap == nil` → skip LoS, gate passes
  (asset-module-only runs, partial integration setups).
- Range check (`dx > rng || dz > rng`) precedes LoS; out-of-range
  paths never invoke `HasLineOfSight`.

The NAI-12 commit message called this "gamemap==nil short-circuits to
gate-pass; see NAI-12 spec § error handling". NAI-18 inherits the same
rationale.

## Out of Scope

1. Hunt-variant LoS sizing (see Non-Goals 3).
2. Reach helpers / `inOperableDistance` (NAI-11 deferral).
3. `entity` interface extension (Non-Goal 2).
4. Loc/Obj concrete entity types (NAI-11 deferrals).
5. Any changes to `pkg/pathfinder/routefinder` (Non-Goal 1).
6. Cosmetic modernization of `NewNpc` stats-seeding loop
   (`modules/world/npc.go:164`) — the NAI-17-era flagged follow-up.
   Explicitly left for a future drive-by sweep per the NAI-17 close
   note's "not worth a dedicated polish commit" assessment.
