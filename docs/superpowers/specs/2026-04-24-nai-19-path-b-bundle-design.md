# NAI-19 — PATH B follow-up bundle (cache relocation + typ snapshot + cosmetic + CRC source unification + revertType respawn alignment)

- **Sub-spec**: NAI-19
- **Date**: 2026-04-24
- **Scope label**: C (cross-package bundle — `pkg/gamemap`, `pkg/cache`, `modules/asset`, `modules/world`; ~270-370 LOC production + ~120-180 LOC tests; revised down ~10 LOC after plan-time discovery that `gm.ChangeNPCCollision` already exists)
- **Predecessors**: NAI-16 (MIDI encoders + PRELOADED registry) — last on `main` as `caea569`
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

Five tracked PATH B follow-ups have accumulated across the NAI series. Each is small enough that a dedicated brainstorm-spec-plan-review cycle would dwarf the work, but together they cluster into one bundle that reuses one cadence pass and one final review.

The five items, in TDD-friendly order:

1. **B3** (cosmetic) — `NewNpc` stats-seeding loop modernization. Flagged by NAI-17 final review (in `nai_followups.md` § "From NAI-17 → Cosmetic"). One-liner; warm-up task.
2. **B1** (relocation) — `cache.MakeCRCs()` belongs in `world.startingFn` next to `cache.PreloadClient` (which NAI-16 just wired). The pre-existing `modules/asset/handler.go:24` placement (inside `/crc` HTTP handler) carries the comment `// TEST - belongs in world` and was explicitly out of scope for NAI-16.
3. **B2** (active fidelity gap) — `*Npc.changeTypeImpl` updates `n.typeId` but does NOT refresh `n.typ`. NAI-18 introduced the first geometry-relevant `n.typ.Size` read in `inApproachDistance` LoS, making this stale-snapshot bug newly observable. Tracked under `nai_followups.md` § "From NAI-18 → Stale `*Npc.typ` snapshot after changetype" with explicit "NAI-19-size sub-spec" pointer.
4. **B4** (CRC source unification) — TS `RebuildNormalEncoder.ts:18-19` reads `PRELOADED_CRC.get('m{x}_{z}'/'l{x}_{z}')`. Goscape's `sendRebuildNormal` reads from a parallel CRC table populated at `gamemap.Init` time (`gm.mapCRC` / `gm.locCRC`). NAI-16 unblocked the registry-side fix; this sub-spec switches the consumer to PRELOADED_CRC and deletes the dead-mirror gamemap CRC table.
5. **B5** (revertType respawn alignment, structural refactor α′) — NAI-17-D1 documents that goscape's `revertType` heavy path is an INLINE reset, while TS `Npc.ts:1083-1085` calls `World.removeNpc(this, -1); World.addNpc(this, -1, false)`. NAI-19 ports the structural refactor: extends `s.removeNpc` / `s.addNpc` to take TS's parameter shape, refactors all 3 callers, and rewrites `revertType` heavy path to the 2-line TS form. Defers Zone state and AI_SPAWN trigger dispatch as new tracked deviations (NAI-19-D1 / NAI-19-D2).

**Bundle rationale**: B1+B2+B3 sum to ~15 LOC (compressed_cadence threshold) but B4+B5 push the total to ~280-380 LOC, requiring formal cadence anyway. Bundling B1-B3 with B4-B5 amortizes the cadence overhead. Each task is independently testable and commits independently.

## Tech stack

- Go 1.26+
- Existing packages: `pkg/cache/` (no new files; consumed differently by `modules/world/rebuildmap.go`), `pkg/gamemap/` (deletes `mapCRC`/`locCRC` fields + `MapsquareCRC` method; adds `ChangeNpcCollision`), `modules/asset/` (one-line removal), `modules/world/` (touches `world.go`, `npc.go`, `npc_masks.go`, `npc_registry.go`, `rebuildmap.go`, `npc_ai.go`, `server.go`).

## Scope (C)

### Task 1 — B3 cosmetic warm-up (~3 LOC)

Modernize `NewNpc` stats-seeding loop at `modules/world/npc.go:163-168` from C-style `for i := 0; i < objtype.NpcStatCount && i < len(typ.Stats); i++` to `for i := range min(objtype.NpcStatCount, len(typ.Stats))`. Behaviorally identical. Pattern matches three sibling sites already modernized in NAI-17:

- `modules/world/npc.go:288` (revertType heavy-path stats reseed)
- `modules/world/npc_masks.go:98` (`resetStatsForType`)
- `modules/world/npc_script.go:244` (regen loop)

### Task 2 — B1 cache.MakeCRCs relocation (~5 LOC)

Move `cache.MakeCRCs()` call from `modules/asset/handler.go:24` to `modules/world/world.go:83` (right after `cache.PreloadClient("data/pack/client")`). The asset module's `RootHandler` no longer mutates global CRC state; world's `startingFn` owns the one-time write at startup. Asset is dependency-ordered after world in the modules graph (`cmd/goscape/app/modules.go`), so by the time the asset HTTP server accepts a `/crc` request, world has already populated `cache.CrcBuffer`.

### Task 3 — B2 changeTypeImpl typ re-lookup (~7-10 LOC + tests)

Refactor `(*Npc).changeTypeImpl` at `modules/world/npc_masks.go:53-74`:

- Lift the `n.lookupType(newType)` call **outside** the `if reset { ... }` block.
- Assign the result to `n.typ` (always — both CHANGETYPE and KEEPALL paths now refresh the snapshot).
- Inside `if reset { ... }`, retain the `resetStatsForType(newTyp)` call using the lifted `newTyp`.
- Preserve the existing tolerance: when `lookupType` returns nil (server/registry unavailable), skip both the `n.typ` assign and the stats reset.

Per TS `Npc.ts`: TS fetches type fresh on every access via `NpcType.get(this.type)` — there is no snapshot to invalidate. Goscape's `n.typ` snapshot model is a deviation already; this task closes the *staleness* surface without changing the snapshot architecture.

### Task 4 — B4 CRC source unification (~30-50 LOC)

Switch the consumer side of map CRCs from `gamemap` to `cache.PreloadedCRC` (single source of truth, mirroring TS layering):

- `modules/world/rebuildmap.go:11-29`: rewrite the per-mapsquare loop body. Read `cache.PreloadedCRC[fmt.Sprintf("m%d_%d", mx, mz)]` and `cache.PreloadedCRC[fmt.Sprintf("l%d_%d", mx, mz)]` (default 0 on miss, matching TS `?? 0`). Per TS `RebuildNormalEncoder.ts:18-19`.
- `pkg/gamemap/gamemap.go`: delete the `mapCRC` / `locCRC` map fields; delete the `crc32.ChecksumIEEE(...)` populate calls at lines 134-135 (and the matching `m{X}_{Z}` populate near 130); delete the `MapsquareCRC(mapX, mapZ int)` method at lines 154-159.
- `pkg/gamemap/gamemap_test.go:108,116`: delete `TestMapsquareCRCReturnsZeroForMissing` and `TestMapsquareCRCCachedFromInit` (their subject no longer exists).
- `modules/world/rebuildmap_test.go` and `modules/world/login_map_test.go`: rewrite the CRC-seeding setup. Add a shared test helper `seedCachedMapCRC(t *testing.T, mx, mz int, mCRC, lCRC uint32)` at a sensible test-shared location (likely `modules/world/rebuildmap_test.go` so both files in the same package use it; pattern mirrors NAI-16's `seedCachedMidi` helper). The helper writes to `cache.PreloadedCRC` and registers a `t.Cleanup` to delete on test end.
- Verify wire-format unchanged: `TestSendRebuildNormalWireFormat` (`rebuildmap_test.go:11`) and `TestLoginSendsRebuildNormal` (`login_map_test.go:13`) MUST pass with byte-identical output (only the CRC-seed source changes).

### Task 5 — B5 α′ revertType respawn alignment (~140-190 LOC + tests)

Four sub-pieces, sequenced for safe TDD (each is one commit). Original 5-piece structure dropped sub-piece 5a after plan-time discovery: `gm.ChangeNPCCollision(size, x, z, level int, add bool)` **already exists** at `pkg/gamemap/gamemap.go:69`. Note the actual name uses uppercase `NPC` (not `Npc`); use the existing name in 5c/5d bodies.

#### 5b — `(*Server).scaleByPlayerCount` helper (~10 LOC)

Port TS `World.scaleByPlayerCount(rate int) int` formula at TS `World.ts:1715` (parameter name is `rate`, semantically a duration/tick rate). Read the TS body and port it directly; the formula scales respawn duration by current player count. Add as a method on `*Server` near other lifecycle helpers in `modules/world/server.go`. Read player count from the existing `s.playersMu`-guarded player slice (or whatever live-player-count primitive exists; verify at impl time).

#### 5c — Extend `(*Server).removeNpc(n *Npc) → removeNpc(n *Npc, duration int)` (~30 LOC)

New body, per TS `World.ts:1296-1319`:

```
func (s *Server) removeNpc(n *Npc, duration int) {
    adjustedDuration := s.scaleByPlayerCount(duration)
    // DEVIATION NAI-19-D1: zone.leave omitted — Zone abstraction not ported.
    n.dead = true
    if n.typ != nil {
        switch n.typ.BlockWalk {
        case objtype.BlockWalkNPC:
            s.gamemap.ChangeNPCCollision(int(n.typ.Size), n.x, n.z, n.level, false)
        case objtype.BlockWalkAll:
            s.gamemap.ChangeNPCCollision(int(n.typ.Size), n.x, n.z, n.level, false)
            s.gamemap.ChangePlayerCollision(int(n.typ.Size), n.x, n.z, n.level, false)
        }
    }
    if n.lifecycle == NpcLifecycleDespawn {
        // TODO(NAI-19): rsbuf.RemoveNpc(n.nid) when rsbuf API surface lands.
        // TODO(NAI-19): full registry cleanup (delete from s.npcs[], splice s.npcLoop)
        // remains deferred per pre-existing dead-bool model — see npc_registry.go header.
    } else if n.lifecycle == NpcLifecycleRespawn && duration > -1 {
        n.lifecycleTick = adjustedDuration
    }
}
```

`n.typ.BlockWalk` is an untyped int populated from `pkg/objtype/npctype.go:38-40` constants (`BlockWalkNone=0, BlockWalkNPC=1, BlockWalkAll=2`). NOT the typed `world.BlockWalk` enum from `modules/world/movement_consts.go` (those are different constants for `Player.blockWalk`). Use `objtype.BlockWalkNPC` / `objtype.BlockWalkAll` in the switch.

NAI-19-D1 inline at the omitted zone.leave site.

#### 5d — Extend `(*Server).addNpc(n *Npc) error → addNpc(n *Npc, duration int, firstSpawn bool) error` (~60 LOC)

New body, per TS `World.ts:1258-1294`:

```
func (s *Server) addNpc(n *Npc, duration int, firstSpawn bool) error {
    if firstSpawn {
        nid := s.allocNpcSlot()
        if nid < 0 {
            return errNpcsFull
        }
        n.nid = nid
        n.server = s
        s.npcs[nid] = n
        s.npcLoop = append(s.npcLoop, n)
        // TODO(NAI-19): rsbuf.AddNpc(n.nid, n.typeId) when rsbuf API surface lands.
    }
    n.x = n.startX
    n.z = n.startZ
    n.dead = false
    // DEVIATION NAI-19-D1: zone.enter omitted — Zone abstraction not ported.
    if n.typ != nil {
        switch n.typ.BlockWalk {
        case objtype.BlockWalkNPC:
            s.gamemap.ChangeNPCCollision(int(n.typ.Size), n.x, n.z, n.level, true)
        case objtype.BlockWalkAll:
            s.gamemap.ChangeNPCCollision(int(n.typ.Size), n.x, n.z, n.level, true)
            s.gamemap.ChangePlayerCollision(int(n.typ.Size), n.x, n.z, n.level, true)
        }
    }
    s.resetEntityForRespawn(n)  // helper introduced in 5d alongside addNpc — see body below
    n.animID = -1
    n.animDelay = 0
    // DEVIATION NAI-19-D2: AI_SPAWN trigger queue omitted — TriggerAiSpawn (script/trigger.go:171)
    // declared but no spawn-flow consumer wiring exists. Activating here would change first-spawn
    // behavior across all existing NPCs. Tracked for closure in a future "AI_SPAWN dispatch wiring" sub-spec.
    if duration > -1 {
        n.lifecycleTick = duration
    }
    return nil
}
```

NAI-19-D1 inline at the omitted zone.enter site. NAI-19-D2 inline at the omitted AI_SPAWN trigger site.

**`resetEntityForRespawn` helper introduction** — add as a private `*Server` method (or free function in `npc_registry.go` taking `*Npc`) in this same commit. Body factored out of the existing `revertType` heavy path stats-reseed logic so 5d's `addNpc` and 5e's revertType refactor share one definition:

```
func (s *Server) resetEntityForRespawn(n *Npc) {
    if n.typeId != n.baseType {
        n.typeId = n.baseType
        n.uid = (n.typeId << 16) | n.nid
        if newTyp := n.lookupType(n.baseType); newTyp != nil {
            n.typ = newTyp
        }
    }
    if n.typ != nil {
        for i := range min(objtype.NpcStatCount, len(n.typ.Stats)) {
            v := int(n.typ.Stats[i])
            n.levels[i] = v
            n.baseLevels[i] = v
        }
    }
    n.queue = nil
    n.waypointIndex = -1
    n.tele = true
    n.masks |= rsbuf.NpcMaskChangeType
    n.huntClock = 0
    n.huntTarget = nil
    if n.typ != nil {
        n.huntRange = int(n.typ.HuntRange)
        n.huntMode = n.typ.HuntMode
    }
}
```

#### 5e — Refactor `revertType` heavy path + caller updates (~50 LOC including test churn)

In `modules/world/npc.go`:

- Replace lines 276-307 (the entire heavy-path body) with the two-line TS port:
  ```
  // DEVIATION NAI-17-D1 retired: heavy path is now the structural TS port.
  s.removeNpc(n, -1)
  s.addNpc(n, -1, false)
  n.resetOnRevert = true // re-arm default (TS gets this for free via ctor rerun)
  ```
- Delete the NAI-17-D1 comment block at lines 257-259 (the deviation is retired).
- Light path (`!n.resetOnRevert` branch at lines 265-273) unchanged.
- Note: `revertType` will need a `*Server` reference. `n.server` already exists (back-reference set by `addNpc` at `npc_registry.go:36`). Use `n.server.removeNpc(n, -1)` etc. Guard nil server defensively to keep test isolation simple.

The `resetEntityForRespawn` helper (introduced in 5d) replaces the existing inline reset body — the helper definition lives in 5d's commit; 5e's commit just deletes the now-redundant inline code.

Update the 3 production/test callers to the new signatures:

- `modules/world/server.go:234`: `s.addNpc(n)` → `s.addNpc(n, -1, true)`. Existing NewServer NPC seeding flow.
- `modules/world/npc_ai.go:46`: `s.removeNpc(n)` → `s.removeNpc(n, -1)`. DESPAWN-lifecycle NPC removal.
- `modules/world/player_npc_test.go:41`: `s.addNpc(n)` → `s.addNpc(n, -1, true)`. Test fixture.

## Explicitly out of scope

- **Zone abstraction port.** TS `gameMap.getZone(x, z, level).enter(npc)` / `.leave(npc)` requires porting a full Zone state machine (zones own player/NPC subscription lists, loc state, obj state, info-pass culling, etc.). Multi-hundred-LOC sub-spec on its own. Tracked under NAI-19-D1.
- **AI_SPAWN trigger dispatch wiring.** Activating it inside `addNpc(firstSpawn=true)` would fire `ai_spawn` for every NPC at server boot — semantically invasive. Belongs in its own sub-spec where the AI_SPAWN dispatch path can be designed (event queue vs immediate fire, ordering relative to `processNpcEventQueue`, etc.). Tracked under NAI-19-D2.
- **rsbuf.AddNpc / rsbuf.RemoveNpc.** Placeholder TODO breadcrumbs in the new addNpc/removeNpc bodies. The rsbuf API surface for entity registration is a separate port; until then, NPC visibility is handled via `EncodeNpc`'s observer-counter side (NAI-9).
- **DESPAWN-lifecycle full registry cleanup.** Existing pre-NAI-19 dead-bool model (`npc_registry.go:42-48` header comment) is preserved. The TODO breadcrumb in 5c's removeNpc DESPAWN branch flags the gap; the registry-manipulation surgery itself (delete from `s.npcs[]`, splice `s.npcLoop` mid-iteration-safely) is orthogonal to revertType respawn alignment.
- **NPC `varns` / `varsString` reset.** TS `resetEntity:296-306` clears varns and varsString on respawn. Goscape's `varns` infrastructure exists from S6a; varsString does not. Current scripts don't depend on revert-time varn reset (the only ChangeType consumers don't call SetNpcVarN), so deferred. Add when a script-content need surfaces.
- **`heroPoints.clear()`.** Goscape has no heroPoints concept. Not relevant.
- **B4 follow-on for `RebuildGetMapsHandler`.** TS uses PRELOADED_CRC at `RebuildGetMapsHandler.ts:44,54` for the get-maps handshake. That handler isn't ported in goscape yet. When it lands, it should consume `cache.PreloadedCRC` directly — no extra registry work needed.

## Architecture

### Files modified (production)

- `modules/asset/handler.go` — delete line 24 (`cache.MakeCRCs()` + comment).
- `modules/world/world.go` — add `cache.MakeCRCs()` immediately after the existing `cache.PreloadClient` call in `startingFn`.
- `modules/world/npc.go` — Task 1 (loop modernization, lines 163-168); Task 5e (heavy-path body replacement at 276-307; comment deletion at 257-259).
- `modules/world/npc_masks.go` — Task 3 (changeTypeImpl refactor, lines 53-74).
- `modules/world/rebuildmap.go` — Task 4 (CRC read source switch).
- `pkg/gamemap/gamemap.go` — Task 4 (delete mapCRC/locCRC fields, populate sites, MapsquareCRC method). [Plan-time correction: sub-piece 5a dropped — `ChangeNPCCollision` already exists at line 69.]
- `modules/world/npc_registry.go` — Task 5c+5d (extend addNpc / removeNpc signatures + bodies).
- `modules/world/npc_ai.go` — Task 5e (caller update at line 46).
- `modules/world/server.go` — Task 5e (caller update at line 234); Task 5b (scaleByPlayerCount helper).

### Files modified (tests)

- `modules/world/npc_masks_test.go` (or wherever changeTypeImpl tests live) — Task 3 (add `TestChangeTypeRefreshesTypSnapshot` + `TestChangeTypeKeepAllRefreshesTypSnapshot`).
- `modules/world/rebuildmap_test.go` — Task 4 (rewrite seed, add `seedCachedMapCRC` helper).
- `modules/world/login_map_test.go` — Task 4 (rewrite seed using shared helper).
- `pkg/gamemap/gamemap_test.go` — Task 4 (delete two MapsquareCRC tests).
- `modules/world/player_npc_test.go` — Task 5e (signature update).
- `modules/world/world_test.go` (new or existing) — Task 2 (assert `cache.CrcBuffer` populated after world startingFn).
- `modules/world/npc_test.go` (or `npc_revert_test.go`) — Task 5 (new tests):
  - `TestRevertTypeHeavyPathTeles` — pin `n.x == n.startX`, `n.z == n.startZ` post-revert.
  - `TestRevertTypeHeavyPathRunsCollisionToggles` — fixture gamemap, assert ChangeNPCCollision off-then-on (name confirmed at `pkg/gamemap/gamemap.go:69`).
  - `TestRevertTypeHeavyPathReseedsStats` — preserve existing coverage.
  - `TestRevertTypeHeavyPathClearsQueueWaypoints` — preserve existing coverage.
  - `TestRevertTypeLightPathUnchanged` — regression pin (KEEPALL revert path body byte-identical).
  - `TestRevertTypeUsesScaledRespawnDuration` — assert `s.scaleByPlayerCount` is consulted.

### Touch-site map

```
Asset HTTP                     World startingFn          Script handlers
   │                                  │                        │
   ▼                                  ▼                        ▼
RootHandler              cache.PreloadClient (NAI-16)    handleNpcChangeType
   /crc → io.Copy(...)   cache.MakeCRCs() (NAI-19 B1)       │
                                                            ▼
                                                    *Npc.ChangeType
                                                            │
                                                            ▼
                                              changeTypeImpl (NAI-19 B2: refresh n.typ)
                                                            │
                                                            ▼
                                                  (later, lifecycleTick=0)
                                                            │
                                                            ▼
                                                   revertType heavy path
                                                            │
                                                            ▼
                                              s.removeNpc(n, -1)  ← NAI-19 B5 (5c, 5e)
                                                            │
                                                            ▼
                                              s.addNpc(n, -1, false)  ← NAI-19 B5 (5d, 5e)
                                                  └─ resetEntityForRespawn(n)


World map encoding                                NPC tick state
   │                                                    │
   ▼                                                    ▼
sendRebuildNormal              ──────────►   inApproachDistance LoS
   reads cache.PreloadedCRC          (NAI-19 B2: now reads fresh n.typ.Size)
   (NAI-19 B4: was gamemap)
```

## Test strategy

**Per-task TDD discipline** — each plan task starts with failing tests, then minimal implementation, then verify-pass + commit. Reviewer Stage 1 checks code quality + spec compliance per task; Stage 2 (whole-impl) at bundle close validates true-to-TS fidelity + deviation count.

**Test fidelity gates per task**:

- Task 1: existing `TestNewNpcSeedsStatsFromType` (`modules/world/npc_test.go:219`) keeps passing; no new test (style-only change).
- Task 2: new test asserts `cache.CrcBuffer != nil` and `len(cache.CrcTable) > 0` after a synthetic call to world's `startingFn` (or via integration-test shape).
- Task 3: TWO new tests in `modules/world/npc_masks_test.go` (alongside existing changeTypeImpl coverage) — `TestChangeTypeRefreshesTypSnapshot` and `TestChangeTypeKeepAllRefreshesTypSnapshot`. Each: register two NpcTypes with distinct `Size` values via `s.npcTypes.Configs[id]`, ChangeType (or ChangeTypeKeepAll) the NPC from one to the other, assert `n.typ.Size` reflects the new value.
- Task 4: byte-for-byte equivalence — `TestSendRebuildNormalWireFormat` and `TestLoginSendsRebuildNormal` must produce IDENTICAL bytes pre/post-refactor (only the CRC source plumbing changes; the wire output is unchanged because both PRELOADED_CRC and the dropped gamemap table compute CRC32/IEEE on the same files). Add a positive-witness test seeding `cache.PreloadedCRC["m30_72"]=0xDEADBEEF, ["l30_72"]=0xCAFEBABE` and asserting those bytes appear in the encoded packet at the right offsets.
- Task 5: six new tests enumerated above, all in `modules/world/npc_test.go` (no `npc_revert_test.go` exists at HEAD). Plus retain all existing revert-related tests at HEAD (`TestNpcTurnRespawnAliveMorphReverts` from NAI-16 Task 3) — they must still pass post-refactor.

**Cross-task regression suite**: full `go test -race ./...` green at every task close.

## Tracked deviations (this spec creates)

- **NAI-19-D1**: `zone.leave` / `zone.enter` omitted from `s.removeNpc` and `s.addNpc`. Reason: Zone abstraction is unported in goscape (no `Zone` type exists in `pkg/gamemap`). Inline comment at both omission sites in `npc_registry.go`. Tracked-for-closure when a Zone-state-port sub-spec lands.
- **NAI-19-D2**: AI_SPAWN trigger queue omitted from `s.addNpc(firstSpawn=true)` body. Reason: `script.TriggerAiSpawn` (declared at `pkg/script/trigger.go:171`) has no spawn-flow consumer wiring. Activating here would change first-spawn behavior across every NPC at server boot. Inline comment in `npc_registry.go` addNpc body. Tracked-for-closure when an AI_SPAWN dispatch sub-spec lands.

## Tracked closures (this spec retires)

- **NAI-17-D1** (revertType inline reset vs TS despawn+respawn): heavy-path body replaced by the structural `s.removeNpc(n, -1); s.addNpc(n, -1, false)` two-line TS port (per TS `Npc.ts:1083-1085`). The structural deviation is retired; the remaining TS divergences (no zone state, no AI_SPAWN re-trigger) are now narrower and tracked separately as NAI-19-D1 / NAI-19-D2.

**Active deviation count math**: 15 (post-NAI-16) − 1 (NAI-17-D1 retired) + 2 (NAI-19-D1, NAI-19-D2 added) = **16**. Net +1, but each new deviation is more honest than the old NAI-17-D1 was — D1 said "we don't do despawn+respawn"; the new pair each names a specific missing primitive (Zone state, AI_SPAWN dispatch) with a concrete future trigger.

## Acceptance gates

1. `go test ./...` green with race detector at HEAD.
2. NAI-19-D1 inline comment present at exactly the two omission sites in `npc_registry.go` (zone.leave site in removeNpc, zone.enter site in addNpc).
3. NAI-19-D2 inline comment present at exactly one site in `npc_registry.go` (addNpc body, AI_SPAWN omission).
4. NAI-17-D1 doc-comment block at `npc.go:257-259` removed (along with the `// DEVIATION NAI-17-D1` line at `npc.go:276`).
5. Bundle delta within envelope: ~280-380 LOC production + ~120-180 LOC tests.
6. Two-stage final review:
   - **Stage 1** (code quality + spec compliance per task): reviewer reads each commit's diff against this spec's Task description and acceptance bullets.
   - **Stage 2** (whole-impl, true-to-TS + deviation count): reviewer cross-checks the 16-active-deviation count, validates that `revertType` heavy path matches TS `Npc.ts:1083-1085` minus NAI-19-D1/D2 omissions, validates that `sendRebuildNormal` matches TS `RebuildNormalEncoder.ts:10-21` byte-for-byte.

## Memory closures (close commit trailer)

Per `close_commit_memory_trailer.md`, the close commit message should include `Closes memory:` trailer pointing at:

- `nai_followups.md` § "From NAI-17 → Cosmetic" (B3)
- `nai_followups.md` § "From NAI-17 → NAI-17-D1 closure track" (B5)
- `nai_followups.md` § "From NAI-18 → Stale `*Npc.typ` snapshot after changetype" (B2)
- The NAI-16 § "Out of scope → cache.MakeCRCs() relocation" pointer (B1, indirectly — surfaced via `nai_followups.md`'s out-of-scope pattern)
- The NAI-16 § "Out of scope → RebuildNormalEncoder port" pointer (B4, narrowed-and-resolved version)
