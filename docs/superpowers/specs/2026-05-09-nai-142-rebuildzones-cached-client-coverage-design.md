# NAI-142 — Retire `rebuildScenery` activeZones dual-write + port per-zone-change `rebuildZones` (TS parity)

**Date:** 2026-05-09
**Status:** Brainstorm-approved spec.
**Cadence:** Mid (~50–75 LOC including tests) — spec + plan + combined single review per `compressed_cadence.md` 15-100 LOC band.
**Tech stack:** Go 1.26+ (`go_version.md`).
**Origin:** TS-fidelity cleanup. Closes deferred follow-ups NAI-84-D-R-D2 and NAI-84-D-R-D3 from `nai_followups.md:89-140`. No user-reported symptom — bundle is parity-driven; today's behavior is benign-broader (rebuildScenery's 13×13 level-0 preset is a superset of the correct 7×7 level-`p.level` set), never benign-narrower.
**TS source:**
- `LostCityRS/Engine-TS/src/engine/entity/BuildArea.ts` (`Rebuild` does not touch `activeZones`)
- `LostCityRS/Engine-TS/src/engine/entity/NetworkPlayer.ts:269-271` (per-zone-change `rebuildZones()` site)
- `LostCityRS/Engine-TS/src/engine/entity/Player.ts:380` (`lastZone: number = -1` field declaration)

---

## §1 Acceptance criteria

### §1.1 PRIMARY (regression-fence smoke)

User-driven smoke at HEAD post-impl must observe:

1. **Login + walk through 2-3 zones streams Loc deltas correctly.** Steps:
   1. Spawn at any populated tutorial/varrock area.
   2. Walk continuously through ≥3 zone boundaries.
   3. **Pin:** observed scenery (locs with shared per-tick events: opened doors, npcs interacting with locs, objs on the ground from `obj_add`) renders without duplication, dropout, or stale-state artifacts in destination zones.
2. **REBUILD_GETMAPS round-trip still works.** Steps:
   1. Tele across the 13×13 build-area window (e.g. `::tele` to a far coord).
   2. **Pin:** map loads, locs/objs in the new window render correctly, no client-side AIOOBE.
3. **No regression in NAI-141 §1 criteria 1-2** (kitchen-door plane round-trip). The bundle does not touch `writeFullFollows`; smoke re-witnesses the green NAI-141 path as a regression-fence.

### §1.2 SECONDARY (unit-test pins)

Locked into automated tests (§3):

- T1: First tick after login fires `rebuildZones()` (lastZone = -1 sentinel; first updateBuildArea call → mismatch → fire).
- T2: Second tick on the same zone does NOT fire `rebuildZones()` (lastZone equals currentZone → no-op).
- T3: Cross-zone walk (x or z crosses an 8-tile boundary) fires `rebuildZones()`.
- T4: Cross-level move (e.g. trapdoor down) fires `rebuildZones()`.
- T5: `rebuildScenery` no longer pre-populates `activeZones` (post-`rebuildScenery`, activeZones is empty before the next `rebuildZones()` call).
- T6: Cached-client cross-zone path: simulate two in-window zone crossings without REBUILD; assert `activeZones` updates each crossing (this is the value of the bundle).

## §2 Architecture

### §2.1 Current state (HEAD)

- `(*Player).rebuildScenery` at `modules/world/player.go:647-682` resets `loadedZones`/`activeZones`/`mapsquares`, computes a 13×13 zone window centered on `(p.x, p.z)`, and writes `activeZones[ZoneIndex(zx<<3, zz<<3, 0)] = true` at line 667 — **hardcoded `level=0`**, regardless of `p.level`.
- `(*Player).rebuildZones` at `modules/world/player.go:701-722` resets `activeZones` and writes the correct 7×7 zone window keyed at `p.level`. Called only from `handleRebuildGetMaps` (`modules/world/data_map.go:153`) — i.e., only after a REBUILD_NORMAL → client-asks-for-maps round-trip.
- `(*Player).processOut` at `modules/world/player.go:855-867` runs `updatePlayers → updateNpcs → updateZones → updateInvs → updateStats → updateAfkZones`. **Missing the entire TS `NetworkPlayer.updateMap` step that should sit at the top.**
- `Player` struct has no `lastZone` field.

TS side, by contrast:
- `BuildArea.Rebuild` (TS equivalent of goscape's `rebuildScenery`, but goscape mis-named it `updateMap`) does NOT touch `activeZones`.
- TS `NetworkPlayer.updateMap` runs first in `processClientsOut` (TS `World.ts:1097`) and includes the `lastZone !== zone → buildArea.rebuildZones()` block at lines 269-271.
- `Player.lastZone: number = -1` is the sentinel.

### §2.2 What changes

**R-D2 — drop dual-write in `rebuildScenery`:**

- `modules/world/player.go:667` — remove the `p.activeZones[coordgrid.ZoneIndex(zx<<3, zz<<3, 0)] = true` line. The surrounding loop continues to populate `mapsquares`.
- `modules/world/player.go:649` — keep the `p.activeZones = map[int]bool{}` reset. Defensive: `rebuildScenery` is logically resetting all scenery-window state, and rebuildZones() resets activeZones again at its top, so this is idempotent. Removal not required; keep for state-reset clarity.
- `modules/world/player.go:694-700` — update the `rebuildZones` doc-comment to remove the §6 R-D2 reference and the "rebuildScenery currently also writes activeZones" paragraph.
- `modules/world/player.go:691` — update the `rebuildZones` doc-comment that says "Not called per-zone-change because goscape has not yet ported NetworkPlayer.ts:269-271 lastZone-transition tracking; deferred follow-up in nai84_rebuildzones_per_zone_change.md." Replace with reference to the new per-zone-change call site.

**R-D3 — add per-zone-change call site:**

- `Player` struct (`modules/world/player.go:316` area, near `activeZones`): add `lastZone int` field. Doc-comment: "Mirrors TS Player.ts:380 `lastZone: number = -1`. Tracks the previously-witnessed packed (level, zoneX<<3, zoneZ<<3) so per-tick `updateBuildArea` can fire `rebuildZones()` on transition."
- Player constructor (`modules/world/player.go:518-521` area): initialize `lastZone: -1`. Sentinel: first updateBuildArea call always fires.
- New method `(p *Player).updateBuildArea()` placed just before `processOut` (player.go:855 area):

  ```go
  // updateBuildArea fires rebuildZones() on per-tick zone transitions.
  // Mirrors the lastZone slice of TS NetworkPlayer.updateMap (NetworkPlayer.ts:269-271):
  //
  //     const zone = CoordGrid.packCoord(this.level, (this.x >> 3) << 3, (this.z >> 3) << 3);
  //     if (this.lastZone !== zone) {
  //         this.buildArea.rebuildZones();
  //         // ... triggerZone/triggerZoneExit/SetMultiway (NAI-N>142)
  //         this.lastZone = zone;
  //     }
  //
  // Camera packets (NetworkPlayer.ts:243-253), lastMapZone tracking
  // (NetworkPlayer.ts:256-266), and triggerZone/SetMultiway
  // (NetworkPlayer.ts:274-285) are deferred to follow-up sub-specs;
  // see nai_followups.md NAI-142-D-R-D{1,2,3}.
  func (p *Player) updateBuildArea() {
      zone := coordgrid.PackCoord(p.level, (p.x>>3)<<3, (p.z>>3)<<3)
      if p.lastZone != zone {
          p.rebuildZones()
          p.lastZone = zone
      }
  }
  ```

- `processOut` (`modules/world/player.go:855`): insert `p.updateBuildArea()` as the first statement (mirroring TS `processClientsOut` ordering at `World.ts:1097`):

  ```go
  func (p *Player) processOut() {
      p.updateBuildArea() // NAI-142: TS NetworkPlayer.updateMap slot in processClientsOut
      p.updatePlayers()
      // ...
  }
  ```

- Update the existing comment block at `modules/world/player.go:856-858` (currently: "// NAI-93: updateMap moved to Server.processInfo per TS World.ts:996 // ordering. processOut now starts with PlayerInfo encode against the // already-fresh rsbuf state.") to acknowledge that the new top-of-`processOut` call is the TS NetworkPlayer.updateMap slot, distinct from the goscape-misnamed `updateMap` (=TS `BuildArea.rebuildNormal`) called from `processInfo`.

### §2.3 Why bundling matters

Per `nai_followups.md:124-129`: pre-bundle, on cached clients (no REBUILD round-trip after a teleport across zones, e.g. in-window `::tele`), `activeZones` stays at the 13×13 superset from `rebuildScenery` until the next forced REBUILD. This is benign-broader (the stale set is broader than needed but never narrower), so loc/obj deltas in the destination zone DO flow — they're just being delivered for an over-set of zones that includes some pre-tele zones. The wasteful side effect: per-tick `updateZones` iterates the 169-entry 13×13 set instead of the canonical 49-entry 7×7. Bundling fixes both:

- R-D2 alone (without R-D3) would break the cached path: post-rebuildScenery `activeZones` would be empty until next REBUILD, dropping all delta delivery.
- R-D3 alone (without R-D2) is safe but redundant: rebuildZones gets called per-zone-change AND rebuildScenery's preset still pre-populates the same map until rebuildZones overwrites it.

The PRIMARY value: cached-client cross-zone delta delivery now uses the correct level-aware 7×7 set, and `updateZones`'s iteration cost drops ~3.4×.

### §2.4 Call-order verification (first-tick coverage)

Concern: with R-D2 retired, on a first login tick:
1. `processInfo` runs `p.updateMap()` (=goscape `rebuildScenery` slot) → no longer pre-populates `activeZones`.
2. `processInfo` proceeds to `ComputePlayer` etc.
3. `processOut` runs `p.updateBuildArea()` → `lastZone (-1) != currentZone` → fires `rebuildZones()` → `activeZones` populated with correct 7×7 level-aware set.
4. `processOut` continues to `updateZones()` → reads fresh `activeZones`, delivers Loc/Obj/MapAnim per zone.

**Verified safe.** No tick has `updateZones` running against an empty `activeZones`.

### §2.5 Call-order verification (REBUILD path)

REBUILD_NORMAL → client asks for maps → `handleRebuildGetMaps` → `rebuildZones()` (existing, data_map.go:153). On the same tick, processOut may also run, hitting `updateBuildArea`:
- If lastZone equals current packed-coord (typical, since rebuildZones was just called from `handleRebuildGetMaps` AFTER lastZone was last set), no-op.
- If lastZone differs (e.g., the client sent the request from a different zone than its current one), `updateBuildArea` fires rebuildZones() again. **Idempotent** — rebuildZones starts with `activeZones = {}`. **Acceptable; matches TS** (TS also has both call sites: `handleRebuildGetMaps`-equivalent and the per-zone-change site, and accepts the redundant call).

### §2.6 Out of scope (deviation flags)

The following TS lines in `NetworkPlayer.updateMap` are NOT ported in this sub-spec. Each gets a tracker entry (§7) for future routing:

- **NAI-142-D-R-D1:** Camera packet processing (`NetworkPlayer.ts:243-253`). Goscape has no `cameraPackets` accumulator yet; CamMoveTo/CamLookAt opcodes are not implemented. Standalone sub-spec.
- **NAI-142-D-R-D2:** `lastMapZone` tracking + `triggerMapzone`/`triggerMapzoneExit` (`NetworkPlayer.ts:256-266`). Mapzone (64-tile-grid) script triggers; standalone sub-spec.
- **NAI-142-D-R-D3:** `triggerZone`/`triggerZoneExit` + `SetMultiway` write (`NetworkPlayer.ts:274-285`). Zone (8-tile-grid) script triggers + multi-combat overlay; goscape has no `World.gameMap.isMulti` and no `[zone,…]` script trigger surface. Standalone sub-spec.
- **NAI-142-D-R-D4:** Renaming goscape's `updateMap` (=TS `BuildArea.rebuildNormal` slot) to `rebuildNormal` to match TS naming and free the `updateMap` name for the future TS NetworkPlayer.updateMap port. Cosmetic; defer until D-R-D{1,2,3} are scheduled and cumulatively justify the rename.

## §3 Test plan

All tests live in `modules/world/player_zone_test.go` (or a new `modules/world/player_buildarea_test.go` file if line count justifies). Use existing `newTestPlayer` fixture pattern.

### §3.1 R-D3 unit tests

**T1 — first-tick fires:** new player; assert `lastZone == -1` after construction; call `updateBuildArea`; assert `lastZone == coordgrid.PackCoord(0, ...)` of the spawn coord; assert `activeZones` non-empty (rebuildZones populated it).

**T2 — same-zone no-fire:** call `updateBuildArea` once; capture `len(activeZones)`; mutate `activeZones` to a sentinel single-entry map; call `updateBuildArea` again; assert `activeZones` is the sentinel (not overwritten — proves rebuildZones did NOT run).

**T3 — cross-zone fires:** call `updateBuildArea` once at (50<<3, 50<<3); shift `p.x` to (51<<3, 50<<3) (cross x-zone boundary); call `updateBuildArea`; assert `lastZone` updated and `activeZones` recentered.

**T4 — cross-level fires:** call `updateBuildArea` at level=0; set `p.level = 1`; call `updateBuildArea`; assert `lastZone` updated and `activeZones` keys are level-1 keys (not level-0).

### §3.2 R-D2 unit tests

**T5 — rebuildScenery no longer pre-populates activeZones:** new player; pre-set `activeZones` to a sentinel; call `rebuildScenery(0)`; assert `activeZones` is empty (the line-649 reset still runs; the line-667 write no longer runs).

### §3.3 Bundle-value test

**T6 — cached-client cross-zone delivers fresh activeZones:** new player at (50<<3, 50<<3); call `updateBuildArea` (first-tick fire); capture `activeZones` set A; move to (52<<3, 50<<3) (in-window cross-zone, no REBUILD); call `updateBuildArea`; assert `activeZones` set B differs from A and contains the new center zone (52, 50). Pre-bundle, B would equal the post-rebuildScenery 13×13 set or the pre-tick stale set; post-bundle, B is the fresh 7×7 centered on (52,50).

### §3.4 Existing tests

- `TestRebuildZonesIntersectsBuildArea` (data_map_test.go:322) — unaffected, still passes.
- `TestHandleRebuildGetMapsCallsRebuildZones` (data_map_test.go:296) — unaffected.
- `TestRebuildZonesHonorsPlayerLevel` (data_map_test.go:354) — unaffected.
- `TestUpdateZonesUnloadsBogusIndex` (player_zone_test.go:116-area) — unaffected.

No existing test pins `rebuildScenery → activeZones write`, so no test deletions/edits required for R-D2.

### §3.5 Regression-fence smoke

Per §1.1: user runs the game post-impl, walks through ≥3 zones, teleports cross-window, climbs trapdoor, observes no scenery dropout/duplication. **No new content-side reproducer required** — this is a parity cleanup, and existing in-game behavior is the regression fence.

## §4 Sequencing

Single-task implementation. R-D2 and R-D3 must land in the same commit (as a unit) because:
- R-D2 alone (without R-D3) breaks cached-client delta delivery.
- R-D3 alone (without R-D2) is harmless but un-finished.
- Splitting makes intermediate-state review impossible (would require re-running the entire smoke at two intermediate HEADs that exhibit known-suboptimal behavior).

Order within the commit:
1. Add `lastZone int` field + init `-1`.
2. Add `updateBuildArea` method.
3. Insert `p.updateBuildArea()` at top of `processOut`.
4. Delete `p.activeZones[...] = true` write at player.go:667.
5. Update doc-comment at player.go:691, 694-700.
6. Update doc-comment at player.go:856-858.
7. Add tests T1-T6.

## §5 Risk register

| # | Risk | Mitigation |
|---|---|---|
| 1 | First-tick coverage on REBUILD path: rebuildScenery fires in `processInfo`, no longer pre-populates activeZones; updateZones runs in `processOut` after updateBuildArea fires rebuildZones. | Verified via call-order trace (§2.4). T1 + smoke (§1.1) re-witness. |
| 2 | REBUILD_GETMAPS double-call: `handleRebuildGetMaps` fires rebuildZones AND processOut.updateBuildArea may also fire it on the same tick. | Idempotent (rebuildZones resets activeZones first); matches TS. §2.5. |
| 3 | Encoding mismatch with future triggerZone/SetMultiway port: lastZone field uses `coordgrid.PackCoord` (TS-compatible, decodable via `UnpackCoord`). The future D-R-D3 port will need to unpack this for `triggerZoneExit(level, x, z)`. | Spec mandates `coordgrid.PackCoord(p.level, (p.x>>3)<<3, (p.z>>3)<<3)` — exact TS shape (`NetworkPlayer.ts:269`). NOT `coordgrid.ZoneIndex` (which uses a different bit layout). Pinned in the doc-comment of `lastZone` field. |
| 4 | `delayed` players: TS condition for skipping updateBuildArea? | TS `processClientsOut` runs `updateMap` unconditionally for connected players (only `isClientConnected` gates). Goscape's `processOut` is also unconditional in this sense. **No additional gate needed.** updateBuildArea runs every tick for every player with a client. |
| 5 | Per-tick cost: rebuildZones is now called every tick on every cross-zone movement, not just on REBUILD. | rebuildZones is O(49) — tight 7×7 nested loop with a build-area window clip. Fires only on zone transitions (lastZone != zone), so cost is bounded by zone-crossing frequency (~1 per ≥4 ticks at run speed). Negligible. |
| 6 | Test fixture parity: tests must call `updateBuildArea` for first-tick state to match production behavior. | Tests construct `newTestPlayer` then explicitly call `updateBuildArea`. Existing tests that call `rebuildScenery` directly are unaffected (T5 verifies). |

## §6 Memory entries to retire / update

After commit, update `nai_followups.md`:
- **Retire** entries `NAI-84-D-R-D2` (lines 89-116) and `NAI-84-D-R-D3` (lines 118-140) — both close.
- **Add** entries `NAI-142-D-R-D1` (camera packets), `NAI-142-D-R-D2` (lastMapZone+triggerMapzone), `NAI-142-D-R-D3` (triggerZone+triggerZoneExit+SetMultiway), `NAI-142-D-R-D4` (rename `updateMap` → `rebuildNormal` for naming parity).
- **Update** the cross-reference at `nai_followups.md:6577` (NAI-141 follow-ups note about R-D2/R-D3) — close the sentence by noting they retired in NAI-142.

## §7 Close-commit memory trailer

Per `close_commit_memory_trailer.md`, the close commit body ends with:

```
Closes memory: nai_followups.md NAI-84-D-R-D2 NAI-84-D-R-D3
```

so the retirement is grep-discoverable from `git log`.
