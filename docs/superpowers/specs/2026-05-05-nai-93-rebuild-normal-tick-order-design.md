# NAI-93: RebuildNormal tick-order fix + tele cheat tightening

**Status:** Draft (brainstorm output)
**Date:** 2026-05-05
**Cadence:** Two-bundle fix sub-spec. Stage 1 root-cause confirmed during brainstorm via TS source comment (`Engine-TS/src/engine/World.ts:996`); no Stage 1 instrumentation needed. Bundle 2 is the deferred-deviation closure flagged at `handler_game.go` cheat-handler `tele` arm (the existing comment at `handlers_game.go:380-387` schedules it as a follow-up).
**Predecessor:** NAI-92 (full SMART pathfinding port; closed `[smoke partial]` 2026-05-05 with three residuals tracked in `nai_followups.md` "From NAI-92"). NAI-93 picks up residual #3 — the Java client AIOOBE on tele to Ardougne mapsquares (mx=41 mz=51 and mx=40 mz=51).
**Tech stack:** Go 1.26+ / TS reference at `LostCityRS/Engine-TS` / Java client reference at `LostCityRS/Client-Java` branch `225` / Rust `pkg/rsbuf` reference at `2004scape/rsbuf` branch 225 (per `rust_source_canonical_path` memory).

---

## 1. Goal

Fix the post-tele Java-client `ArrayIndexOutOfBoundsException` in `getHeightmapY` (and the cascading post-corner-tele-and-click crash in `getTopLevel`) by reordering goscape's per-tick `rebuildNormal` to run **before** `rsbuf.ComputePlayer`, matching TS `World.processInfo` (`Engine-TS/src/engine/World.ts:992-1056`). Same pass tightens the `::tele` cheat handler to add the TS pre-tele cleanup chain (`closeModal` → `canAccess` gate → `clearInteraction` → `unsetMapFlag`), closing the deviation tagged at `handlers_game.go:380-387`.

## 2. Stage 1 finding (brainstorm-confirmed; root cause)

**Reproducer (user-launched smoke 2026-05-05, deterministic):**

| # | Cheat command | Outcome |
|---|---|---|
| 1 | `::tele 0,41,51,37,42` (Ardougne center) | Crash: `AIOOBE: 122` at `client.java:2640` (`getHeightmapY`) → `updateOrbitCamera` (`client.java:6394`) |
| 2 | `::tele 0,40,51,37,42` (one square west) | Same crash, same trace |
| 3 | `::tele 0,41,51,0,0` (Ardougne SW corner) | No crash on tele; clicking NW after the tele crashes: `AIOOBE: 104` at `client.java:1935` (`getTopLevel`) → `drawScene` |

**Diagnosis chain (verified at brainstorm-time):**

1. **TS canonical order** (`World.ts:996`): inside `processInfo`'s per-player loop, the comment is verbatim *"set origin before compute player is why this is above."* The call order is `reorient()` → `buildArea.rebuildNormal()` → `rsbuf.computePlayer(...)`.
2. **goscape inverts it.** `tick.go:46` calls `processInfo()` (which invokes `s.rsbuf.ComputePlayer(... p.originX, p.originZ ...)` at `tick.go:396-419`). `tick.go:48` calls `processClientsOut()` → `processOut()` → `updateMap()` (`player.go:815-816`), and `updateMap` calls `rebuildScenery` (`player.go:639-678`) which mutates `p.originX = p.x; p.originZ = p.z`, then `sendRebuildNormal` (`rebuildmap.go:18-34`).
3. **Stale origin in rsbuf cache.** When a tele crosses the rebuild window:
   - `processClientsIn` runs the cheat handler; `TeleJump` writes `p.x, p.z` to the new tile and sets `p.tele = true`. `p.originX, p.originZ` retain the **old** values (rebuildScenery hasn't run yet).
   - `processInfo` calls `s.rsbuf.ComputePlayer` with `(p.x = NEW, p.originX = OLD, p.tele = true, ...)`. `pkg/rsbuf/buf.go:204` packs `p.Origin = PackCoord(level, OLD originX, OLD originZ)` into rsbuf's cached `Player` struct.
   - `processClientsOut` → `updateMap` runs `shouldRebuild()` which returns `true` (player crossed the window), then `rebuildScenery` updates `p.originX, p.originZ` to NEW. `sendRebuildNormal` emits with `zoneX = p.x >> 3` (NEW). Wire packet ordering is correct (RebuildNormal first, PlayerInfo after).
   - `updatePlayers` → `s.rsbuf.PlayerInfo.Encode` reads the cached `self.Origin` (still OLD). The tele-leaf branch at `pkg/rsbuf/playerinfo.go:120-142` computes:
     ```
     localX := pos.X - (((originPos.X >> 3) - 6) << 3)   // pos.X = NEW, originPos.X = OLD
     localZ := pos.Z - (((originPos.Z >> 3) - 6) << 3)
     ```
     For reproducer #1 (login spawn `(3094, 3106)` → `::tele 0,48,50,21,28` → drift-landed `(3094, 3251)` (NAI-92 residual #2, separate bug) → `::tele 0,41,51,37,42`):
     - `pos = (2661, 3306)`, `origin = (3094, 3251)`.
     - `localX = 2661 - ((386 - 6) << 3) = 2661 - 3040 = -379`. `PBit(7, -379)` writes the low 7 bits of `uint32(-379) = 0xFFFFFE85` → `0x05` = **5**.
     - `localZ = 3306 - ((406 - 6) << 3) = 3306 - 3200 = 106`. Fits in 7 bits → encoded as **106**.
   - Client `client.java:9204-9213`: tele case (`var5 == 3`) reads `gBit(7), gBit(7)`, calls `localPlayer.teleport(jump=1, var6=5, var7=106)`. `PathingEntity.teleport` at `PathingEntity.java:182-193` snaps `pathTileX[0] = 5; pathTileZ[0] = 106; this.x = 5*128 + 64 = 704; this.z = 106*128 + 64 = 13632`.
   - Render frame: `updateOrbitCamera` (`client.java:6356-6394`) snaps `orbitCameraX/Z` to `localPlayer.x/z` (large divergence triggers the ±500 snap at `client.java:6359`). Calls `getHeightmapY(level=0, orbitCameraX=704, orbitCameraZ=13632)`. `getHeightmapY` (`client.java:2636-2648`): `var5 = 704 >> 7 = 5`; `var6 = 13632 >> 7 = 106`. First read at `levelTileFlags[1][5][106]`. The active-window arrays are sized to the 13×13 zone scene = 104+1 tiles per axis (allocated in REBUILD_NORMAL handler at `client.java:9460-9482`). **Index 106 OOB.** Reproducer #2 differs in the prior origin and produces the observed `122`-class index in the same crash site (the exact wrap value depends on which prior tele set the now-stale origin).
4. **Reproducer #3 is the same bug, milder.** `::tele 0,41,51,0,0` from the just-prior `::tele 0,40,51,37,42` (state per Q3 reply): `pos = (2624, 3264)`, stale `origin = (2597, 3306)`. `localX = 2624 - 2544 = 80` (in range); `localZ = 3264 - 3256 = 8` (in range). Tele itself succeeds with corrupted-but-in-range local coords. Clicking NW asks the server to walk one tile further northwest; the client's camera-tile-flag lookup (`getTopLevel` at `client.java:1935`, indexing `levelTileFlags[currentLevel][var3][var4]` with `var3 = cameraX >> 7`) eventually walks past array bound `104`.
5. **Bail-out (β / client-side) ruled out.** TS source explicitly documents the placement; the Java client decode path is correct given correctly-encoded inputs. Fix is server-side.

**Conclusion:** The bug is a TS-fidelity tick-order violation that produces stale-origin tele leaves on cross-window teles. The fix is one literal-port of TS `processInfo`'s call order.

## 3. Cadence

Two bundles, smoke at end of Bundle 2.

**Bundle 1 — Tick-order fix.** TS-literal port of `processInfo`'s call order. Three tasks (T1 test, T2 move, T3 doc-comment cleanup). One commit each per `runescript_cadence`.

**Bundle 2 — `::tele` cheat handler tightening.** Adds the TS pre-tele cleanup chain that the existing `handlers_game.go:380-387` DEVIATION block schedules as a follow-up. Three tasks (T4 test, T5 implement, T6 doc-comment update). One commit each.

**Smoke** — user-launched after T6 lands. Three teles + click per §7.

**Close commit** — `chore(close): NAI-93 — RebuildNormal tick-order + tele cheat tightening [smoke conditional]` with the `Closes memory:` trailer per `close_commit_memory_trailer` memory.

Per `compressed_cadence` memory: production diff is ~30-50 LOC across two bundles; full cadence (no compression) because the surface spans engine ordering + cheat-handler cleanup + tests + doc-comment audits across three files.

## 4. Architecture

### 4.1 Bundle 1 — Tick-order fix

**Site (move):** `modules/world/tick.go` `processInfo()` at lines 340-419, `processOut()` at `player.go:815-825`, and the `updateMap` function at `player.go:720-733`.

**TS reference body** (`Engine-TS/src/engine/World.ts:992-1056`, abridged):

```ts
private processInfo(): void {
    for (const player of this.playerLoop.all()) {
        player.reorient();
        player.buildArea.rebuildNormal(); // set origin before compute player is why this is above.
        const appearance = player.masks & PlayerInfoProt.APPEARANCE
            ? player.generateAppearance()
            : (player.appearanceBuf ?? player.generateAppearance());
        rsbuf.computePlayer(player.x, player.level, player.z, player.originX, player.originZ, /* … */);
    }
    // … npcs …
}
```

**goscape new body** (`tick.go:340-365` region):

```go
func (s *Server) processInfo() {
    s.playersMu.RLock()
    players := make([]*Player, len(s.playerLoop))
    copy(players, s.playerLoop)
    s.playersMu.RUnlock()

    // NAI-66: TS World.ts:995 — per-tick refocus before rsbuf compute.
    // NAI-93: TS World.ts:996 — rebuildNormal before ComputePlayer so
    // the rsbuf-cached Origin matches the just-emitted RebuildNormal
    // packet's zoneX/zoneZ. Inverting this order produces stale-origin
    // tele leaves on cross-window teles → client-side AIOOBE.
    for _, p := range players {
        p.reorient()
        p.updateMap()
    }

    // Regenerate appearance buffer for any player whose MaskAppearance is set
    // … (existing block, unchanged)

    // … existing s.renderer.ComputePlayers + ComputeNpcs + per-player ComputePlayer push …
}
```

**`processOut` change** (`player.go:815-825`):

```go
func (p *Player) processOut() {
    // NAI-93: updateMap moved to Server.processInfo (TS World.ts:996 ordering).
    p.updatePlayers()
    p.updateNpcs()
    p.updateZones()
    p.updateInvs()
    p.updateStats()
    p.updateAfkZones()
    p.encodeOut()
    p.client.flushWrite()
}
```

`updateMap` itself is unchanged — only its call site moves.

**Wire-order invariant preserved.** `sendRebuildNormal` writes to `p.client.bufw` via `p.writeOut`. `flushWrite` only fires at the end of `processOut`. So:
- Old order: bufw appended as `[RebuildNormal, PlayerInfo, NpcInfo, Zone*, Inv*, Stat*, …]`, single `flushWrite`.
- New order: same — `processInfo` calls `updateMap` which appends `RebuildNormal` to bufw; `processOut` then appends `PlayerInfo, NpcInfo, …` and flushes. Wire packet sequence is identical.

**Login-tick safety.** `processLogins` runs at `tick.go:45`, BEFORE `processInfo` (line 46). New players get `p.originX = p.x` (line 97-98) and `p.tele = true` (line 135) before `processInfo` runs `updateMap` on them. `shouldRebuild()` returns `true` (`!p.rebuiltOnce`); `rebuildScenery` confirms origin (no-op write) and emits the first RebuildNormal. `ComputePlayer` then reads the freshly-set origin. First-tick login flow unchanged in behavior.

**No new state coupling.** `updateMap`'s outputs (`p.originX, p.originZ, p.activeZones, p.loadedZones, p.mapsquares, p.lastBuild, p.rebuiltOnce`) feed downstream consumers: `updateZones` (which is now AFTER `updateMap` since both have moved earlier vs each other — `updateZones` is in `processOut` and `updateMap` is in `processInfo`, still ordered correctly), `handleRebuildGetMaps` (next-tick `processClientsIn`), and `rebuildZones` (called from `handleRebuildGetMaps`). All consumers see the new origin/mapsquares before they read.

### 4.2 Bundle 2 — `::tele` cheat handler tightening

**Site:** `modules/world/handlers_game.go:371-410` (the `case "tele":` arm of `handleClientCheat` / similar; the exact function is the cheat-dispatch arm).

**TS reference** (`Engine-TS/src/network/game/client/handler/ClientCheatHandler.ts:491-524`):

```ts
} else if (cmd === 'tele') {
    if (args.length < 1) { return false; }
    const coord = args[0].split(',');
    if (coord.length < 3) { return false; }

    player.closeModal();

    if (!player.canAccess()) {
        player.messageGame('Please finish what you are doing first.');
        return false;
    }

    player.clearInteraction();
    player.unsetMapFlag();

    const level = tryParseInt(coord[0], 0);
    /* … parse mx, mz, lx, lz … */
    if (level < 0 || level > 3 || mx < 0 || mx > 255 || /* … */) { return false; }

    player.teleJump((mx << 6) + lx, (mz << 6) + lz, level);
}
```

**goscape new body** (replaces lines 371-411):

```go
case "tele":
    // staffModLevel >= 2 gate mirrors TS ClientCheatHandler.ts:483.
    if p.staffModLevel < 2 {
        return nil
    }
    // Mirrors TS `::tele level,mapX,mapZ[,localX,localZ]` at
    // ClientCheatHandler.ts:491-524.
    if args == "" {
        return nil
    }
    coord := strings.Split(args, ",")
    if len(coord) < 3 {
        return nil
    }

    p.CloseModal(true) // TS closeModal() default arg.
    if !p.CanAccess() {
        p.MessageGame("Please finish what you are doing first.")
        return nil
    }
    p.ClearInteraction()
    sendUnsetMapFlag(p)
    p.waypointIndex = -1 // TS Player.unsetMapFlag → clearWaypoints (Player.ts:2169-2172).

    level := parseIntOr(coord[0], 0)
    mx := parseIntOr(coord[1], 50)
    mz := parseIntOr(coord[2], 50)
    lx := 32
    if len(coord) > 3 {
        lx = parseIntOr(coord[3], 32)
    }
    lz := 32
    if len(coord) > 4 {
        lz = parseIntOr(coord[4], 32)
    }
    if level < 0 || level > 3 || mx < 0 || mx > 255 || mz < 0 || mz > 255 || lx < 0 || lx > 63 || lz < 0 || lz > 63 {
        return nil
    }
    p.TeleJump((mx<<6)+lx, (mz<<6)+lz, level)
```

**Helper-bundle pattern note.** `sendUnsetMapFlag(p) + p.waypointIndex = -1` matches the existing TS-helper-bundle pattern at `handlers_game.go:256-257` (NAI-77 / `ts_helper_method_bundles` memory). No new helper introduced (YAGNI — only two call sites in production after this lands).

**TS-fidelity ordering.** The TS sequence is `closeModal → canAccess gate → clearInteraction → unsetMapFlag → parse → teleJump`. The bounds-check on parsed integers in TS comes AFTER `unsetMapFlag`; goscape mirrors this. (Side effect: an invalid coord string still triggers `closeModal/clearInteraction/unsetMapFlag`. This matches TS exactly, including the awkwardness of those side effects on a bad input — staff-only debug op, behavior parity wins.)

## 5. Tests

### 5.1 Bundle 1 — Tick-order

**T1 — failing test BEFORE T2:** `modules/world/tick_test.go` (new file or extension of existing).

```go
func TestProcessInfo_RebuildNormalRunsBeforeComputePlayer_TeleCrossesWindow(t *testing.T) {
    // Setup: server with one player at Tutorial Island spawn (3094, 3106).
    // Run one tick to settle origin.
    // TeleJump player to Ardougne (2661, 3306).
    // Run one tick.
    // Assert: the OpPlayerInfo packet emitted in that tick decodes to a
    // tele-leaf with localX, localZ both in [0, 104].
    s := newTestServer(t)
    p := newTestPlayerAt(t, s, 3094, 3106, 0)
    s.tickOnce(t) // settle: rebuiltOnce flips, origin pinned.
    p.TeleJump(2661, 3306, 0)
    payload := s.tickOnceCapturePlayerInfo(t, p)
    leaf := decodeTeleLeaf(t, payload)
    if leaf.LocalX < 0 || leaf.LocalX > 104 {
        t.Errorf("localX = %d, want [0, 104]", leaf.LocalX)
    }
    if leaf.LocalZ < 0 || leaf.LocalZ > 104 {
        t.Errorf("localZ = %d, want [0, 104]", leaf.LocalZ)
    }
    // Pin specific values: post-rebuild origin = (2661, 3306).
    // localX = 2661 - ((332-6)*8) = 2661 - 2608 = 53.
    // localZ = 3306 - ((413-6)*8) = 3306 - 3256 = 50.
    if leaf.LocalX != 53 || leaf.LocalZ != 50 {
        t.Errorf("localX, localZ = %d, %d, want 53, 50", leaf.LocalX, leaf.LocalZ)
    }
}
```

Auxiliary helpers (`newTestServer`, `newTestPlayerAt`, `tickOnce`, `tickOnceCapturePlayerInfo`, `decodeTeleLeaf`) — plan-author reuses or extends existing test infra at `modules/world/*_test.go` (e.g., `appearance_test.go` and the existing `newTestPlayer(t)` helper). If `tickOnce` doesn't exist, plan adds a thin synchronous helper that runs the same per-stage sequence as `runTickLoopWithRate`'s body without the timer.

**Pre-T2 behavior:** test must FAIL (localZ=106 or similar OOB-when-translated value).
**Post-T2 behavior:** test must PASS with the pinned (53, 50) values.

**T1 secondary assertion** — the SW-corner subcase from reproducer #3: tele to (2624, 3264) from a prior (2597, 3306) origin; assert localX=48, localZ=48 post-fix. (Without the fix: localX=80, localZ=8 — in-range but wrong; the click-NW symptom is downstream of the engine fix and is covered by the smoke step §7, not the unit test.)

### 5.2 Bundle 2 — Cheat handler

**T4 — failing tests BEFORE T5:** add to `modules/world/handlers_game_test.go` (existing).

- `TestTeleCheat_CallsCloseModalAndClearsModalState` — set `p.modalState = modalStateMain`, send `::tele 0,50,50,32,32`, assert `p.modalState == modalStateNone` post-handler.
- `TestTeleCheat_CanAccessGate_RejectsWithMessageGame` — set `p.delayed = true` (forces `CanAccess() = false`), send `::tele 0,50,50,32,32`, assert: position unchanged; one game message in outbox containing `"Please finish what you are doing first."`.
- `TestTeleCheat_UnsetMapFlag_ClearsWaypointAndEmitsPacket` — set `p.waypointIndex = 5` and waypoints[0..5], send `::tele 0,50,50,32,32`, assert: `p.waypointIndex == -1`; outbox contains one `OpUnsetMapFlag` packet.
- `TestTeleCheat_BoundsCheck_StillRejectsAfterCleanup` — send `::tele 9,50,50,32,32` (level OOB), assert: position unchanged BUT modal-close + waypoint-clear side effects still observed (TS-faithful ordering).

### 5.3 Test-coverage cross-check

Per `plan_test_coverage_crosscheck`: the spec's test list must cover every code branch added or moved. Bundle 1 has one new branch (the rebuild fires inside processInfo); the T1 single test exercises it. Bundle 2 has four new branches (closeModal call, canAccess fail-path, unsetMapFlag bundle, ordering of cleanup-vs-bounds-check); T4's four named tests exercise each. Plan-author re-runs this cross-check pre-dispatch.

## 6. Smoke (Stage 2 close)

User-launched per `smoke_test_server_handoff`. Steps the user runs after Bundle 2's T6 lands:

1. Build + start server: `CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml`.
2. Connect Java client (`Client-Java` branch 225), log in.
3. Run reproducer #1: `::tele 0,41,51,37,42`. **Expect:** no client crash; client renders Ardougne market square.
4. Run reproducer #2 from the new state: `::tele 0,40,51,37,42`. **Expect:** no crash.
5. Run reproducer #3: `::tele 0,41,51,0,0`, then click any tile northwest. **Expect:** no crash; player walks one step NW.
6. Tutorial Island regression smoke: `::tele 0,48,50,21,28` and verify the player can walk around the spawn area without crash. (NAI-92 residual #2, the first-tele-after-region-load drift, is OUT OF SCOPE for this sub-spec — see §8.)

If any step still crashes:
- Step 3-5 still crashing → bail to β: file Client-Java repo issue. Investigation continues in NAI-94.
- Step 6 drift (the +23-tile north landing) — expected; tracked in `nai_followups.md` as separate residual. Not a NAI-93 bail trigger.

## 7. Risk register

| ID | Risk | Mitigation | Verification |
|---|---|---|---|
| R1 | `updateMap` in `processInfo` writes RebuildNormal before `processClientsOut` flushes; if any new packet writer added between processInfo and flushWrite reorders before RebuildNormal, wire order breaks. | Wire ordering remains: `processInfo` only appends RebuildNormal to `bufw`; subsequent processOut steps append after. `flushWrite` is single per tick. | Inspect `processOut`'s ordered list; T1's tickOnce captures the actual emitted byte stream and can assert `OpRebuildNormal` precedes `OpPlayerInfo`. |
| R2 | A separate code path (e.g., reconnect) reaches `updateMap` outside the new placement and produces stale-origin PlayerInfo. | Per `risk_register_premise_grep`: grep all `updateMap()` call sites at plan-author time. Currently the only call is `processOut`; after move, only `processInfo`. | Plan-author runs `rg "\.updateMap\(\)" modules/world/` and lists all sites. |
| R3 | Test helper `tickOnce` semantics drift from `runTickLoopWithRate` body. | Plan-author authors `tickOnce` as a thin call-the-same-stages function and uses `s.currentTick++` to step the clock; documents the drift surface in a comment. | Code review: spot-check that the synchronous helper matches the loop body's stage list. |
| R4 | `p.CloseModal(true)` has IF_CLOSE-trigger side effects (`player_script.go:692-718`); calling it on a player without a modal but with an `activeScript` may suspend or null `activeScript`. | TS does the same; this is intended. The `if p.modalState == modalStateNone { return }` early-exit at `player_script.go:686-688` covers the no-modal case. | T4's `CanAccessGate` test asserts the `delayed` (script-active) path is rejected by `CanAccess()` BEFORE we reach `ClearInteraction`/`unsetMapFlag` in those tests with explicit modal state. |
| R5 | `p.MessageGame` requires non-nil server; on a disconnecting player the call may panic. | Existing `MessageGame` impl handles nil-server defensively; plan-author verifies before T5. | grep `func .*MessageGame` and read defensive-guard pattern. |

Per `risk_register_premise_grep` memory: the plan-author re-greps every "verified" claim in this spec at plan-write time and re-greps at controller-pre-flight per `controller_preflight`.

## 8. Out of scope (deferred)

- **NAI-92 residual #1** — pathfinder reach for through-door routes (Survival Expert + Hans). Tracked separately; not blocked by NAI-93.
- **NAI-92 residual #2** — first-tele-after-region-load coord drift (`tele 0,48,50,21,28` lands ~23 tiles north). The smoke step 6 may still exhibit this; that's expected and tracked. NAI-94 candidate.
- **`::teleto` cheat command** (TS `ClientCheatHandler.ts:525+`) — not currently in goscape. Out of scope; future port.
- **`p.unsetMapFlag()` extraction into a Player method.** Two call sites (handlers_game.go:256-257 OPCOORD path and the new tele cheat) currently inline the bundle. YAGNI; if a third site appears, extract then. Tracked in `nai_followups.md` as future polish.
- **NAI-91 residual** (`NAI-91-D-OPERABLE-CHEB-FALLBACK`, entity-shape `reachedEntity` and Obj-shape `reachedObj` ports) — orthogonal.

## 9. TS-fidelity ledger

**Closes deviation:** the inline `DEVIATION` comment at `handlers_game.go:380-387` (TS pre-tele cleanup chain not yet wired). Bundle 2 closes it. T6 deletes the DEVIATION block and replaces with a one-line "Mirrors TS ClientCheatHandler.ts:491-524" reference. Per `retire_deviation_grep_all_comments` memory: T6 also runs `rg "DEVIATION" modules/world/handlers_game.go` to confirm no other tele-related deviation comments need updating.

**Closes implicit divergence:** the tick-order divergence at `tick.go:46-48` was not explicitly tagged before this sub-spec; Bundle 1 closes it without a tag (it predates the deviation-ledger discipline). T2's commit body is the provenance.

**No new deviations introduced.** All Bundle 1 and Bundle 2 changes are TS-literal ports.

## 10. Memory updates (post-close)

Per `post_task_handoff`: the close commit's `Closes memory:` trailer should reference (and the post-close memory pass should update):

- `nai_followups.md` "From NAI-92" — strike residual #3 (the AIOOBE crash); leave residuals #1 and #2.
- `nai_followups.md` add a new "From NAI-93" section if any residuals surface from smoke (e.g., new symptoms after the fix).
- A new memory entry on **TS comments-as-load-bearing-spec** if the brainstorm pattern (TS source-comment "X before Y is why this is above" → spec-justification chain) is reusable. Provisional title: `ts_comment_as_ordering_spec.md`. Decide at close-commit time based on whether NAI-92 + NAI-93 form a pattern of multi-tick-ordering bugs surfacing as wire-encode failures.

---

## Appendix A — Why the bug specifically OOBs at index `122` for reproducer #1

Reproducer #1 stack:
```
java.lang.ArrayIndexOutOfBoundsException: 122
    at deob.client.getHeightmapY(client.java:2640)
    at deob.client.updateOrbitCamera(client.java:6394)
```

The smoke session preceding the crash had executed multiple teles. The exact prior origin in rsbuf's cache at the moment of the crashing tele's `ComputePlayer` is what determines which OOB index the client reports. The mechanism is unchanged regardless of the precise number — `localZ = pos.Z - (((staleOriginZ >> 3) - 6) << 3)` produces a value between 105 and 127 for prior origins ~16-176 tiles south of the new pos.Z. The 7-bit field carries the value verbatim (no wrap into [0, 104]); the client array bound is 105 (indices 0..104). Indices 105..127 all crash.

For reproducer #2 (`tele 0,40,51,37,42` from the post-#1 state), the prior origin would have been the first tele's intended landing zone (plus the drift); recomputing yields a localZ in the 105-127 band, with `122` as the specific reported index. The plan-author does not need to reproduce this exact arithmetic — pinning the post-fix values (localX=53, localZ=50 for the canonical Ardougne center coord) in T1 is sufficient.
