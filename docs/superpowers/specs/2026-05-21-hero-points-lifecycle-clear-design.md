# Hero-points lifecycle clear-site sweep — spec

**Status:** brainstormed 2026-05-21
**Predecessor slice:** `[[hunt-huntvis-filter-close]]` (HEAD `74ea0fe5`)
**Origin:** carry-forward item #3 from predecessor — "Hero-points consumption (NAI-120 Bundle 2D)". Resume framing ("wire `HeroPoints.TopContributor()` into NPC death drop assignment") was inaccurate; brainstorming caught it. See §2.

## 1. Goal

Port the four observably-meaningful TS `heroPoints.clear()` lifecycle call sites that goscape currently omits. Document the fifth TS call site (Player.cleanup) as an observable no-op deferral. Strict TDD; one site per commit.

## 2. Premise correction (brainstorming finding)

The carry-forward menu line "Wire `HeroPoints.TopContributor()` into NPC death drop assignment" is **not** the gap. Verification:

- `HeroPoints` struct + `AddHero`/`Clear`/`TopContributor` operations: already shipped (`modules/world/heropoints.go`).
- `NPC_HEROPOINTS` write opcode: already wired (`pkg/script/handlers_npc.go:1235` `handleNpcHeroPoints` → `s.ActiveNpc.AddHeroPoints`).
- `NPC_FINDHERO` read opcode: already wired (`pkg/script/handlers_npc.go:1262` `handleNpcFindHero` → `s.ActiveNpc.TopContributor` → `s.World.LookupPlayerByUID`).
- `FINDHERO` read opcode: already wired (`pkg/script/handlers_player.go:1479` `handleFindHero`).
- `BOTH_HEROPOINTS` write opcode: already wired (`pkg/script/handlers_player.go:1512` `handleBothHeroPoints`).
- `NPC_STATHEAL` HP-full branch clear: already wired (`pkg/script/handlers_npc.go:1368` → `s.ActiveNpc.HeroPointsClear()`).

TS engine has **no engine-side NPC death drop assignment** that consumes `heroPoints`. Loot routing on NPC death is driven entirely by content scripts (e.g., `chompy_bird` bird drops via NPC_FINDHERO → OBJADDSLOT). This matches goscape's already-shipped surface.

**Actual gap:** five TS `heroPoints.clear()` lifecycle sites; goscape ports one (NPC_STATHEAL) and omits four. Of those four, one is observably equivalent to GC in goscape (Player.cleanup, since goscape freshly allocates Player per login). The remaining **four** sites are real ports.

## 3. Sites in scope

| # | TS source | TS site | goscape target | Real port? |
|---|---|---|---|---|
| 1 | `Engine-TS/src/engine/entity/Npc.ts:292` | `Npc.resetEntity(respawn=true)` | `modules/world/npc_registry.go:121` `resetEntityForRespawn` | **YES** — NPC struct reused across respawn cycles |
| 2 | `Engine-TS/src/engine/entity/Player.ts:446` | `Player.cleanup()` (logout) | `modules/world/server.go:977` `removePlayerInternal` | **NO** — fresh `*Player` per login (`newPlayer` at `player.go:506` line 544 initialises `heroPoints: NewHeroPoints(16)`), so clearing a soon-to-be-GC'd ledger has no observable effect. Document with informal English doc-comment citing TS line, no formal `NAI-XXX-D-*` tag (precedent: `[[combat-sub-spec-framing-doc-cleanup-close]]`). |
| 3 | `Engine-TS/src/engine/script/handlers/PlayerOps.ts:513-515` | `STAT_ADD` HP-full branch | `pkg/script/handlers_player.go:342` `handleStatAdd` | **YES** — observable to FINDHERO between PvP encounters |
| 4 | `Engine-TS/src/engine/script/handlers/PlayerOps.ts:552-554` | `STAT_BOOST` HP-full branch | `pkg/script/handlers_player.go:408` `handleStatBoost` | **YES** — same |
| 5 | `Engine-TS/src/engine/script/handlers/PlayerOps.ts:609-611` | `STAT_HEAL` HP-full branch | `pkg/script/handlers_player.go:481` `handleStatHeal` | **YES** — same |

## 4. TS source pin (canonical predicate for sites 3-5)

```ts
if (stat === PlayerStat.HITPOINTS && player.levels[PlayerStat.HITPOINTS] >= player.baseLevels[PlayerStat.HITPOINTS]) {
    player.heroPoints.clear();
}
```

Three properties to mirror exactly:

1. **Predicate is checked AFTER the level mutation** (clear sees the post-update HP).
2. **`>=` comparison** (TS uses non-strict greater-or-equal; both equal and over-cap-then-clamped cases clear).
3. **Gated on `stat === HITPOINTS`** — clear NEVER fires when a non-HP stat is mutated, even if HP happens to be full.

Goscape predicate (using `pkg/objtype/playerstat.go:12` `PlayerStatHitpoints = 3`):

```go
if id == objtype.PlayerStatHitpoints && s.Self.Stat(objtype.PlayerStatHitpoints) >= s.Self.StatBase(objtype.PlayerStatHitpoints) {
    s.Self.HeroPointsClear()
}
```

Order: must run AFTER `s.Self.SetCurLevel(id, ...)` to read the updated value.

## 5. Interface surface change

`ActivePlayer` in `pkg/script/active.go` currently exposes `AddHeroPoints` + `TopContributor` (lines 711-720) but NOT `HeroPointsClear`. Add it after `TopContributor`:

```go
// HeroPointsClear resets the player's hero-point contributor ledger.
// Called by STAT_ADD, STAT_BOOST, STAT_HEAL on the HP-full branch
// (PlayerOps.ts:513-515, :552-554, :609-611). Mirrors the parallel
// ActiveNpc.HeroPointsClear surface used by NPC_STATHEAL. NAI-120
// Bundle 2D follow-up.
HeroPointsClear()
```

Real implementation in `modules/world/player_script.go:1338` (right after `TopContributor`):

```go
// HeroPointsClear implements script.ActivePlayer. Resets the player's
// hero-point contributor ledger. Mirrors TS Player.heroPoints.clear()
// at PlayerOps.ts:513-515, :552-554, :609-611 (HP-full branches of
// STAT_ADD / STAT_BOOST / STAT_HEAL).
func (p *Player) HeroPointsClear() {
    p.heroPoints.Clear()
}
```

Mock implementation: extend `mockPlayer` in `pkg/script/runner_test.go:99` (the only `ActivePlayer`-implementing mock in `pkg/script/`) with a `heroPointsClearCalls int` field and a `HeroPointsClear()` method that increments it. Pattern mirrors `mockNpc.heroPointsClearCalls` at `handlers_npc_test.go:308-309`.

Note: existing `pkg/script/handlers_player_test.go:84` has `func (m *mockActiveNpc) HeroPointsClear()` (empty body) for the `ActiveNpc` surface — unrelated to this interface widening; leave it alone. No other mocks in `pkg/script/` need updating.

## 6. Task breakdown (5 commits, strict TDD)

### Task 1 — Add `HeroPointsClear()` to `ActivePlayer` interface + real impl + mock

Files: `pkg/script/active.go`, `modules/world/player_script.go`, `pkg/script/handlers_player_test.go` (mock extension).

Sub-steps:
1. Pre-stub: extend `ActivePlayer` interface with `HeroPointsClear()`, add real impl on `*Player`, extend `mockActivePlayer` (must compile + existing tests green — interface add is widening for the real type but not for mocks; the mock must implement the new method).
2. RED: add `TestPlayerHeroPointsClear` to `modules/world/heropoints_test.go` (or a new `player_script_test.go` test if appropriate) that:
   - Constructs a Player with `heroPoints` populated via `AddHeroPoints(uid=1, amount=5)` and `AddHeroPoints(uid=2, amount=3)`.
   - Asserts `TopContributor() == 1` (sanity).
   - Calls `HeroPointsClear()`.
   - Asserts `TopContributor() == 0` (ledger empty).
3. GREEN: real impl already in step 1 satisfies the test. Adjust if needed.
4. Commit: `feat(world): expose Player.HeroPointsClear for STAT handler HP-full clears`

### Task 2 — Wire HP-full clear into `handleStatAdd`

File: `pkg/script/handlers_player.go:342-366`.

Sub-steps:
1. RED: add `TestHandleStatAdd_HPFullClearsHeroPoints` and `TestHandleStatAdd_HPNotFullSkipsClear` and `TestHandleStatAdd_NonHPStatSkipsClear` to `handlers_player_test.go`.
   - HP-full test: prime ledger via `mockActivePlayer.AddHeroPoints` calls, set HP=current=10 (base=15), call `handleStatAdd` with constant=10 percent=0 → post-update HP=20→clamped 255 but predicate checks against base=15; with constant=10 percent=0 added=cur+10=20 which is >=15 so clears. Use simpler: HP=14 base=15, constant=1 percent=0 → added=15 → predicate hits → clear. Assert `mockActivePlayer.heroPointsClearCalls == 1`.
   - HP-not-full test: HP=10 base=15, constant=1 percent=0 → added=11 → predicate misses → assert `heroPointsClearCalls == 0`.
   - Non-HP-stat test: stat=0 (attack), HP currently=15 base=15 (full), constant=1 → added attack=N+1, but stat != HITPOINTS so no clear → assert `heroPointsClearCalls == 0`.
2. GREEN: add tail block after `s.Self.SetCurLevel(id, added)`:

   ```go
   if id == objtype.PlayerStatHitpoints && s.Self.Stat(objtype.PlayerStatHitpoints) >= s.Self.StatBase(objtype.PlayerStatHitpoints) {
       s.Self.HeroPointsClear()
   }
   ```
3. Doc-comment refresh on `handleStatAdd` to mention the TS:513-515 HP-full clear tail.
4. Commit: `feat(script): port STAT_ADD HP-full heroPoints.clear tail (PlayerOps.ts:513-515)`

### Task 3 — Wire HP-full clear into `handleStatBoost`

Same shape as Task 2 against `pkg/script/handlers_player.go:408-439`. Tests:
- `TestHandleStatBoost_HPFullClearsHeroPoints`
- `TestHandleStatBoost_HPNotFullSkipsClear`
- `TestHandleStatBoost_NonHPStatSkipsClear`

Note STAT_BOOST formula: `boosted = max(min(current + boost, base + boost), current)`. Pick fixture so post-update HP ≥ base. E.g., HP=15 base=15 (already full), constant=0 percent=0 → boost=0 → boosted=cur=15 → ≥ base → clears.

Commit: `feat(script): port STAT_BOOST HP-full heroPoints.clear tail (PlayerOps.ts:552-554)`

### Task 4 — Wire HP-full clear into `handleStatHeal`

Same shape against `pkg/script/handlers_player.go:481-508`. Tests:
- `TestHandleStatHeal_HPFullClearsHeroPoints`
- `TestHandleStatHeal_HPNotFullSkipsClear`
- `TestHandleStatHeal_NonHPStatSkipsClear`

STAT_HEAL formula: `healed = max(min(cur + (constant + base*percent/100), base), cur)` — caps at base, never lowers. Fixture: HP=10 base=15, constant=5 percent=0 → healed=15 → ≥ base → clears.

Commit: `feat(script): port STAT_HEAL HP-full heroPoints.clear tail (PlayerOps.ts:609-611)`

### Task 5 — Wire NPC respawn clear + document Player.cleanup deferral

Files: `modules/world/npc_registry.go:121`, `modules/world/server.go:977` (doc-comment only).

Sub-steps:
1. RED: add `TestResetEntityForRespawn_ClearsHeroPoints` to `modules/world/npc_registry_test.go`:
   - Construct registered NPC, call `n.heroPoints.AddHero(uid=42, amount=7)`, assert `TopContributor() == 42`.
   - Call `s.resetEntityForRespawn(n)`.
   - Assert `n.heroPoints.TopContributor() == 0`.
2. GREEN: in `resetEntityForRespawn` add `n.heroPoints.Clear()` adjacent to other reset operations. Placement: alongside `n.queue = nil` and `n.waypointIndex = -1` (line 142-143 area — the "lifecycle field zeros" cluster). Adjust doc-comment range mention if it enumerates resets.
3. Doc-comment on `removePlayerInternal` (server.go:977): add one line of provenance:

   ```go
   // TS Player.cleanup at Engine-TS/src/engine/entity/Player.ts:446 calls
   // player.heroPoints.clear() here. goscape omits the call — newPlayer
   // (player.go:506) allocates a fresh *Player per login with a fresh
   // NewHeroPoints(16) (player.go:544), so clearing the about-to-be-GC'd
   // ledger has no observable effect. Informal deferral (no NAI-XXX-D
   // pin); precedent [[combat-sub-spec-framing-doc-cleanup-close]].
   ```
4. Commit: `feat(world): clear heroPoints on NPC respawn (Npc.ts:292) + document Player.cleanup no-op deferral`

## 7. Test plan

New tests (11 total):
- `pkg/script/handlers_player_test.go`: 9 new tests across Tasks 2-4 (3 per handler × 3 handlers).
- `modules/world/npc_registry_test.go`: 1 new test (Task 5).
- `modules/world/heropoints_test.go` or a new `player_script_test.go`: 1 sanity test for `(*Player).HeroPointsClear` (Task 1).

Mock surface change: `mockPlayer` in `pkg/script/runner_test.go:99` gains `heroPointsClearCalls int` field + `HeroPointsClear()` method.

Existing tests: zero modifications expected. The interface widening on `ActivePlayer` is a Go-compile-error trigger for any mock implementation not updated; only `mockPlayer` in `pkg/script/runner_test.go` and `*Player` in `modules/world/` implement it.

## 8. Gates

- `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go test -race ./...` — 57+ pkgs, 0 FAIL.
- `TestPackAll_TwelveStageSmoke` PASS.
- Audit-grep `heroPoints.clear()` post-slice — should find production-side citations at all 4 real-port sites (TS line refs) + 1 deferral site in `removePlayerInternal` doc-comment.
- Audit-grep `NAI-120 Bundle 2D` — preserves existing 13 hits unchanged (no retirements; this is a Bundle 2D follow-up, the bundle stays open as the umbrella).

## 9. Deviation pins

- **Opened:** 0 (informal English deferral at Player.cleanup, no `NAI-XXX-D-*` tag).
- **Retired:** 0.
- Net board change: 0.

## 10. Out of scope

- Wiring `HeroPoints.TopContributor()` into engine-side death drop assignment — **phantom gap**; TS engine has no such code path. Loot routing is script-driven via existing NPC_FINDHERO/FINDHERO opcodes.
- Player.cleanup heroPoints.clear() port — documented as observable no-op; no port commits.
- Any other Bundle 2D handler work (NPC_HEROPOINTS, NPC_STATHEAL clear, NPC_FINDHERO, FINDHERO, BOTH_HEROPOINTS all already shipped).

## 11. Carry-forward (post-slice)

Remove "Hero-points consumption (NAI-120 Bundle 2D)" from predecessor menu. Remaining:
1. LoW arg-shape pin at FindClosestNpc* (XS, ~hours).
2. Hit-splat multi-hit queue (NAI-127 Bundle 2) (M, ~1 week).
3. General world/runescript engine work.
4. NAI-162 analytics RPC.
5. Combat-level read-site verification.
6. Deviation audit refresh.
7. InvType/NpcType/ObjType registry-presence validator family.

## 12. Open questions

None — design fully specified; brainstorming caught the only major premise issue (TopContributor consumption framing).
