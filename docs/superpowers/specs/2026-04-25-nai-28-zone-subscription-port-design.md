# NAI-28 — Zone PathingEntity subscription primitive port + first-consumer migrations (huntNpcs + huntPlayers)

- **Sub-spec**: NAI-28
- **Date**: 2026-04-25
- **Scope label**: B (Subscription primitive port closing NAI-19-D1 + first observer-pass consumer migration; touches `pkg/zone/` (new `list.go`, additions to `zone.go`) + `modules/world/{server.go,npc_registry.go,movement.go,npc_interaction.go,npc_ai.go,player.go,player_script.go,npc.go,npc_hunt.go,npc_hunt_entities.go}` + tests; ~280-400 LOC production + tests across 3 bundles; retires deviation tag NAI-19-D1; introduces 0 new deviation tags; net deviation count 14 → 13)
- **Predecessors**: NAI-27 (player timer family + VARARG opcode port + NPC queue audit memo) — last on `main` as `ef9ed20`
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

NAI-19 closed `revertType`'s structural alignment with TS `Npc.ts:1083-1085` by porting `s.removeNpc(n, -1); s.addNpc(n, -1, false)` into goscape's `(*Server).removeNpc`/`(*Server).addNpc`. Two TS-side calls inside those helpers were deferred at NAI-19 close as a tracked deviation:

```
World.gameMap.getZone(npc.x, npc.z, npc.level).enter(npc);
zone.leave(npc);
```

The deviation was named **NAI-19-D1** and inlined at four sites (`modules/world/npc_registry.go:64, 149` + `modules/world/npc.go:259, 283`) with rationale "Zone abstraction not ported." That rationale was correct at NAI-19 brainstorm time but is now stale: goscape's `pkg/zone/` package has grown to ~1091 LOC across `event.go`, `grid.go`, `map.go`, and `zone.go`, mirroring TS's `~1164` LOC across `ZoneEvent.ts`, `ZoneGrid.ts`, `ZoneMap.ts`, and `Zone.ts`. The actual missing piece is narrower than "Zone abstraction" — it is the single PathingEntity subscription primitive: per-zone `Player`/`Npc` doubly-linked lists with `enter`/`leave` methods and `ZoneGrid` flag/unflag wiring on first-player-enter / last-player-leave.

Brainstorm-time recon (against `Engine-TS/src/engine/zone/Zone.ts:79-100`, `Engine-TS/src/engine/entity/PathingEntity.ts:170-190`, and `Engine-TS/src/engine/World.ts:941, 1268-1269, 1297-1299, 1587-1612`) confirmed:

1. **TS Zone.players + Zone.npcs are `DoublyLinkList<T>` subscription lists**, not collections of locs/objs. Goscape's `Zone` has `Locs []*entity.Loc` and `Objs []*entity.Obj` but no PathingEntity counterpart.
2. **TS Zone.enter / Zone.leave** are called from six sites: NPC spawn (`World.ts:1268-1269`), NPC despawn (`World.ts:1297-1299`), per-step cross-zone movement (`PathingEntity.ts:182-183`, fires for both player and NPC via the shared base class), player login (`World.ts:941`), and player logout (`World.ts:1598`). Plus several teleport sites that bypass `stepOnce` and need explicit `leave`/`enter` calls.
3. **TS Zone.players list flags ZoneGrid** at `Zone.ts:83` (on first-player-enter, via `World.gameMap.getZoneGrid(level).flag(x, z)`) and unflags at `Zone.ts:94-96` (on last-player-leave). Goscape's `ZoneGrid` already exposes `Flag`/`Unflag` (`pkg/zone/grid.go:33-39`); the integration is missing.
4. **Goscape currently maintains a parallel-track spatial index in `pkg/grid/` (104 LOC)** with `Add`/`Remove`/`AddNpc`/`RemoveNpc`/`NearbyPlayers`/`NearbyNpcs`. This index has TS no counterpart — TS uses Zone subscription as its spatial index. Goscape's existing wire-through at `tick.go:320-322` (player movement) and `tick.go:356-357` (NPC movement) updates `pkg/grid` per-tick. Bundle 3's consumer migrations (huntNpcs at `npc_hunt_entities.go:23` + huntPlayers at `npc_hunt.go:108`) shift the **read-side** to `pkg/zone` subscription; **write-side** `pkg/grid` calls remain in place until a future NAI sub-spec retires `pkg/grid` entirely.

A DoublyLinkList-faithful primitive ships in Bundle 1 — intrusive, O(1) add/unlink, identical iteration semantics. Per `true_to_ts_gate` discipline, the Go-idiom translations (Element-based intrusive list with `Unlink()` on the element rather than TS's `DoublyLinkable` abstract-base `unlink2()` on the value; split `EnterPlayer`/`EnterNpc` methods rather than unified `enter()` with TS `instanceof` dispatch) are externally invisible — same O(1) cost, same iteration order, same visible state — and therefore introduce **no new deviation tags**.

The disciplines `audit_full_method_against_ts`, `enumerate_all_sites`, `controller_preflight`, `spec_followup_tracker_freshness`, and `file_scoped_audits_miss_cross_file_ts` (all freshly active across NAI-25/26/27) drove this brainstorm — re-derivation from primary TS sources reframed candidate (a) from "design Zone abstraction infrastructure" to "port one missing subscription primitive into the existing 1091-LOC `pkg/zone/` surface."

## Tech stack

- Go 1.26+
- Existing packages touched:
  - `pkg/zone/zone.go` — `Zone` struct gains `players DoublyLinkList[PlayerLike]` and `npcs DoublyLinkList[NpcLike]` fields; new methods `EnterPlayer`/`LeavePlayer`/`EnterNpc`/`LeaveNpc`/`PlayersSafe`/`NpcsSafe`/`PlayersCount`/`NpcsCount`; new package-level interfaces `PlayerLike` and `NpcLike` for the cyclic-import boundary
  - `modules/world/server.go` (`(*Server).addPlayer` at `:599`; `(*Server).removePlayer` at `:643`)
  - `modules/world/npc_registry.go` (`(*Server).addNpc` at `:64`; `(*Server).removeNpc` at `:149` — both DEVIATION NAI-19-D1 sites retired)
  - `modules/world/movement.go` (`(*Player).stepOnce` at `:64-94` — per-step cross-zone refresh)
  - `modules/world/npc_interaction.go` (`(*Npc).stepOnce` at `:314-337` — per-step cross-zone refresh; NPC teleport sites at `:95` (stuck-teleport) and `:122` (patrol teleport))
  - `modules/world/npc_ai.go` (`:35` — NPC respawn-to-startCoord teleport)
  - `modules/world/player_script.go` (`(*Player).TeleJump` at `:214` and `(*Player).Teleport` at `:224` — explicit leave-old/enter-new)
  - `modules/world/player.go` (Player struct gains `zoneListElement *zone.Element[zone.PlayerLike]`; doc-comment around `:392` already reads `Slot() int` interface contract; no method additions needed)
  - `modules/world/npc.go` (Npc struct gains `zoneListElement *zone.Element[zone.NpcLike]`; doc-comment retirement at `:259, 283` for the four "no zone state" narration lines)
  - `modules/world/npc_hunt_entities.go` (`huntNpcs` at `:23-76` migrates from `s.grid.NearbyNpcs` to `s.zoneMap.NearbyZones` + `zn.NpcsSafe`)
  - `modules/world/npc_hunt.go` (`huntPlayers` at `:108-180` migrates from `s.grid.NearbyPlayers` to `s.zoneMap.NearbyZones` + `zn.PlayersSafe`)
- New files in production packages:
  - `pkg/zone/list.go` (~90 LOC — generic intrusive `DoublyLinkList[T]` + `Element[T]` mirroring TS `DoublyLinkList<T>` semantics)
- Test files touched:
  - `pkg/zone/list_test.go` (new — list ops, idempotency, iteration order)
  - `pkg/zone/zone_test.go` (extension — Enter/Leave grid wiring, count tracking, IsValid filter)
  - `modules/world/server_zone_subscription_test.go` (new — login/logout enter/leave end-to-end)
  - `modules/world/npc_registry_test.go` (extension — addNpc/removeNpc enter/leave; revertType heavy-path leaves+enters cycle)
  - `modules/world/movement_test.go` (extension — per-step cross-zone refresh; intra-zone no-op; cross-level)
  - `modules/world/player_script_test.go` (extension — Teleport/TeleJump leave-old/enter-new; explicit not via stepOnce)
  - `modules/world/npc_interaction_test.go` + `modules/world/npc_ai_test.go` (extensions — NPC teleport sites refresh subscription)
  - `modules/world/npc_hunt_entities_test.go` (existing tests must still pass under Zone-backed huntNpcs; new tests pin "no fallback to grid" via dual-pin per `ts_asymmetry_dual_pin`)
  - `modules/world/npc_hunt_test.go` (analogous for huntPlayers)
- Memory files:
  - `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (NAI-19-D1 entry marked Resolved with NAI-28 close hash; new "From NAI-28" entry for the `pkg/grid` retirement followup; possible new memory entry on the parallel-spatial-index migration pattern)

## Scope

### Bundle 1 — `pkg/zone` subscription primitive (foundation, ~120 LOC + ~90 LOC tests)

**Goal**: Land the generic intrusive `DoublyLinkList[T]` + `Element[T]` types in a new `pkg/zone/list.go`, then extend `Zone` with `players`/`npcs` lists, `EnterPlayer`/`LeavePlayer`/`EnterNpc`/`LeaveNpc` methods, iterator helpers, and ZoneGrid flag/unflag wiring. No `modules/world/` consumers in this bundle — Zone subscription is exercised entirely through pkg/zone-internal tests with stub implementations of `PlayerLike`/`NpcLike`.

**Source mappings**:

- `pkg/zone/list.go` (new file):
  ```go
  type Element[T any] struct {
      next, prev *Element[T]
      list       *DoublyLinkList[T]
      Value      T
  }

  // Unlink removes the node from its list. Idempotent — second call is a no-op.
  // Mirrors TS DoublyLinkable.unlink2 (Engine-TS datastruct/DoublyLinkList.ts).
  func (e *Element[T]) Unlink()

  type DoublyLinkList[T any] struct {
      head, tail *Element[T]
      size       int
  }

  func (l *DoublyLinkList[T]) AddTail(v T) *Element[T]
  func (l *DoublyLinkList[T]) Size() int
  func (l *DoublyLinkList[T]) All(reverse bool) iter.Seq[T]
  ```
  Uses Go 1.23+ `iter.Seq` for iteration.
- `pkg/zone/zone.go` additions:
  - Package-level interfaces near the top of the file:
    ```go
    type PlayerLike interface {
        IsValid() bool
        Slot() int
    }

    type NpcLike interface {
        IsValid() bool
        Nid() int
    }
    ```
  - `Zone` struct gains two unexported fields:
    ```go
    players DoublyLinkList[PlayerLike]
    npcs    DoublyLinkList[NpcLike]
    ```
  - `New` constructor extension: zero-initialise — list zero values are valid empty lists.
  - New methods (mirror TS `Zone.enter`/`Zone.leave` per `Zone.ts:79-100`):
    ```go
    // EnterPlayer adds p to z.players. If z's player count transitions 0→1,
    // grid.Flag(z.X, z.Z) is called. Returns the Element pointer for caller storage.
    // Mirrors TS Zone.enter (Player branch) at Zone.ts:80-83.
    func (z *Zone) EnterPlayer(p PlayerLike, grid *ZoneGrid) *Element[PlayerLike]

    // LeavePlayer removes p (via e.Unlink()) from z.players. If z's player
    // count transitions 1→0, grid.Unflag(z.X, z.Z) is called. Caller must
    // null its zoneListElement field after this call.
    // Mirrors TS Zone.leave (Player branch) at Zone.ts:90-96.
    func (z *Zone) LeavePlayer(p PlayerLike, e *Element[PlayerLike], grid *ZoneGrid)

    // EnterNpc / LeaveNpc analogous; NPC branch does NOT touch ZoneGrid.
    // Mirrors TS Zone.enter / Zone.leave (Npc branch) at Zone.ts:84-87, 97-99.
    func (z *Zone) EnterNpc(n NpcLike) *Element[NpcLike]
    func (z *Zone) LeaveNpc(n NpcLike, e *Element[NpcLike])

    // PlayersSafe / NpcsSafe yield only IsValid() == true entities.
    // Mirrors TS getAllPlayersSafe / getAllNpcsSafe at Zone.ts:387-405.
    func (z *Zone) PlayersSafe(reverse bool) iter.Seq[PlayerLike]
    func (z *Zone) NpcsSafe(reverse bool) iter.Seq[NpcLike]

    // PlayersCount / NpcsCount return list size. Mirrors TS playersCount /
    // npcsCount fields at Zone.ts:51-52.
    func (z *Zone) PlayersCount() int
    func (z *Zone) NpcsCount() int
    ```
  - `Reset()` method (currently at `pkg/zone/zone.go:43-47`): leaves the new lists ALONE per TS — `Zone.reset()` clears events but NOT players/npcs (Zone.ts:197-201). No change required if the new fields are not added to Reset's clear list.

**Plan-author premise verification (per `controller_preflight`)**: re-grep at HEAD for:
- `iter.Seq` import already present in `pkg/zone/` (likely yes since `iter.Seq[*Zone]` patterns exist)
- `Zone.Reset()` body for any clear that would need to spare the new fields
- `pkg/zone/zone_test.go` test scaffolding patterns (existing `newZone` helper or equivalent)

**Acceptance criteria**:
1. `go build ./pkg/zone/...` and `go test ./pkg/zone/...` both green at HEAD on the bundle-close commit.
2. `Zone.PlayersCount` and `Zone.NpcsCount` return 0 on a freshly-constructed zone.
3. `EnterPlayer` on an empty zone calls `grid.Flag(z.X, z.Z)` exactly once; `EnterPlayer` on a non-empty zone does NOT touch grid.
4. `LeavePlayer` reducing count to 0 calls `grid.Unflag(z.X, z.Z)` exactly once; `LeavePlayer` reducing count from N>1 to N-1 does NOT touch grid.
5. `EnterNpc`/`LeaveNpc` never touch the grid (TS Zone.enter/leave NPC branches only update count, not grid).
6. `Element.Unlink()` is idempotent — calling twice does not panic, decrements size only on the first call.
7. `PlayersSafe(false)` yields entities in insertion order; `PlayersSafe(true)` yields in reverse insertion order. Same for `NpcsSafe`.
8. `PlayersSafe`/`NpcsSafe` skip entities whose `IsValid()` returns false.

**Tests** (in `pkg/zone/list_test.go` and `pkg/zone/zone_test.go`):

- **`list_test.go`** (new file):
  - `TestDoublyLinkList_AddTailIncrementsSize`
  - `TestDoublyLinkList_UnlinkRemovesAndDecrements`
  - `TestDoublyLinkList_UnlinkIdempotent` — second call is no-op, size unchanged
  - `TestDoublyLinkList_AllForwardOrderMatchesInsertion`
  - `TestDoublyLinkList_AllReverseOrderMatchesInsertion`
  - `TestDoublyLinkList_EmptyAllYieldsNothing`
  - `TestElement_UnlinkClearsListPointer` — defensive (`e.list == nil` after Unlink)
- **`zone_test.go`** (extension; uses small stub structs implementing `PlayerLike`/`NpcLike`):
  - `TestZoneEnterPlayerFlagsGridOnFirstEntry`
  - `TestZoneEnterPlayerSecondPlayerDoesNotReFlag`
  - `TestZoneLeaveLastPlayerUnflagsGrid`
  - `TestZoneLeavePlayerNonLastDoesNotUnflag`
  - `TestZoneEnterNpcDoesNotFlagGrid` — **dual-pin per `ts_asymmetry_dual_pin`**: NPC enter does NOT touch grid even on first NPC.
  - `TestZoneLeaveNpcDoesNotUnflagGrid` — symmetric dual-pin.
  - `TestZoneEnterIncrementsCount`
  - `TestZoneLeaveDecrementsCount`
  - `TestZonePlayersSafeFiltersInvalid`
  - `TestZoneNpcsSafeFiltersInvalid`

Per `plan_runnable_test_fixtures` memory: each test fixture is mentally executed at plan-write to verify field shapes and assertion structure.

**Deviation impact**: 0. Element-vs-abstract-base and split-method-vs-instanceof are externally invisible Go-idiom translations per `true_to_ts_gate`.

### Bundle 2 — `modules/world` wire-through (~140 LOC + ~120 LOC tests; **retires NAI-19-D1**)

**Goal**: Wire the production sites that mutate Player or Npc position to call `Zone.EnterX`/`Zone.LeaveX`. Retire the 4 NAI-19-D1 inline comments and the 2 doc-comment narration lines in `npc.go`. Bundle 2 ships parallel to (does NOT replace) the existing `pkg/grid` write-side updates in `tick.go`; both indexes update on every event until Bundle 3 migrates the read-side and a future sub-spec retires `pkg/grid` entirely.

**Source mappings** (8 logical wire-through groups → 11 individual code edits):

1. **`(*Server).addPlayer`** (`modules/world/server.go:599-613`):
   - After `s.players[i] = p` (`:606`) and before `return nil` (`:609`):
     ```go
     z := s.zoneMap.Get(p.level, p.x, p.z)
     p.zoneListElement = z.EnterPlayer(p, s.zoneMap.Grid(p.level))
     ```

2. **`(*Server).removePlayer`** (`modules/world/server.go:643-659`):
   - Before `s.players[p.slot] = nil` (`:651`):
     ```go
     if p.zoneListElement != nil {
         z := s.zoneMap.Get(p.level, p.x, p.z)
         z.LeavePlayer(p, p.zoneListElement, s.zoneMap.Grid(p.level))
         p.zoneListElement = nil
     }
     ```

3. **`(*Server).addNpc`** (`modules/world/npc_registry.go` around `:64`):
   - **Replace** the four-line `// DEVIATION NAI-19-D1: zone.enter omitted — Zone abstraction not ported. See spec § Tracked deviations.` comment with:
     ```go
     z := s.zoneMap.Get(n.level, n.x, n.z)
     n.zoneListElement = z.EnterNpc(n)
     ```
   - Doc-comment update on the addNpc method header: drop the "DEFERRED per NAI-19-D1" mention.

4. **`(*Server).removeNpc`** (`modules/world/npc_registry.go` around `:149`):
   - **Replace** the four-line `// DEVIATION NAI-19-D1: zone.leave omitted — Zone abstraction not ported.` comment with:
     ```go
     if n.zoneListElement != nil {
         z := s.zoneMap.Get(n.level, n.x, n.z)
         z.LeaveNpc(n, n.zoneListElement)
         n.zoneListElement = nil
     }
     ```
   - Doc-comment update on the removeNpc method header (`:138-147`): drop the "(DEFERRED per NAI-19-D1)" mention.

5. **`(*Player).stepOnce`** (`modules/world/movement.go:64-94`):
   - Insert per-step refreshZone immediately AFTER `p.x += dx; p.z += dz; p.stepsTaken++` (`:86-88`) and BEFORE the destination check at `:90`:
     ```go
     prevZX, prevZZ := p.lastStepX>>3, p.lastStepZ>>3
     newZX, newZZ := p.x>>3, p.z>>3
     if prevZX != newZX || prevZZ != newZZ {
         if p.client != nil && p.client.server != nil && p.client.server.zoneMap != nil {
             s := p.client.server
             grid := s.zoneMap.Grid(p.level)
             prevZone := s.zoneMap.Get(p.level, p.lastStepX, p.lastStepZ)
             newZone := s.zoneMap.Get(p.level, p.x, p.z)
             prevZone.LeavePlayer(p, p.zoneListElement, grid)
             p.zoneListElement = newZone.EnterPlayer(p, grid)
         }
     }
     ```
   - **Note on level**: `stepOnce` cannot change level — single-tile delta. Level changes only via Teleport/TeleJump.

6. **`(*Npc).stepOnce`** (`modules/world/npc_interaction.go:314-337`):
   - Symmetric per-step refreshZone immediately after `n.x += dx; n.z += dz; n.stepsTaken++` (`:330-332`) and before the destination check (`:333`). Uses `s` parameter already passed in.

7. **`(*Player).Teleport` and `(*Player).TeleJump`** (`modules/world/player_script.go:214-220, 222-229`):
   - Both methods mutate `p.x`/`p.z`/`p.level` and currently bypass any zone refresh. Capture previous coord BEFORE mutation; after mutation, leave-old/enter-new (cross-zone OR cross-level):
     ```go
     prevX, prevZCoord, prevLevel := p.x, p.z, p.level
     p.x = x; p.z = z; p.level = level
     p.tele = true
     // (TeleJump also sets p.jump = true)
     if (prevX>>3) != (x>>3) || (prevZCoord>>3) != (z>>3) || prevLevel != level {
         if p.client != nil && p.client.server != nil && p.client.server.zoneMap != nil {
             s := p.client.server
             prevGrid := s.zoneMap.Grid(prevLevel)
             newGrid := s.zoneMap.Grid(level)
             prevZone := s.zoneMap.Get(prevLevel, prevX, prevZCoord)
             newZone := s.zoneMap.Get(level, x, z)
             prevZone.LeavePlayer(p, p.zoneListElement, prevGrid)
             p.zoneListElement = newZone.EnterPlayer(p, newGrid)
         }
     }
     ```
     **Plan-author note**: factor a helper `refreshPlayerZone(p, prevX, prevZ, prevLevel)` per `plan_grep_helper_patterns` discipline — three production sites use the same shape (TeleJump + Teleport + future TELE script handler), making a shared helper appropriate.

8. **NPC teleport sites** (3 sites identified at brainstorm):
   - `modules/world/npc_interaction.go:95` — stuck-teleport-to-spawn after 30-tick stuck horizon
   - `modules/world/npc_interaction.go:122` — patrol-teleport-to-waypoint
   - `modules/world/npc_ai.go:35` — respawn-to-startCoord
   
   Each site captures `prevX, prevZ, prevLevel` before the assignment, then calls a shared helper `refreshNpcZone(s, n, prevX, prevZ, prevLevel)` after.

**Plan-author premise verification (per `controller_preflight`)**: re-grep at HEAD for:
- All `n.x = `, `n.z = `, `n.level = ` assignments outside `stepOnce` and `addNpc` (in case a new NPC teleport site has landed since brainstorm)
- All `p.x = `, `p.z = `, `p.level = ` assignments outside `stepOnce`, `addPlayer`, and the two `player_script.go` teleport methods
- `(*Server).removePlayer` flow — confirm no early-return path that would skip the LeavePlayer call before `s.players[p.slot] = nil`

**Plan-author re-greps** (per `enumerate_all_sites`):
```bash
rg -n 'n\.x = |n\.z = |n\.level = ' modules/world/*.go | grep -v _test | grep -v '+= '
rg -n 'p\.x = |p\.z = |p\.level = ' modules/world/*.go | grep -v _test | grep -v '+= '
```
Implementer re-greps post-commit per `enumerate_all_sites` and adds any newly-found assignment site to the wire-through list (or flags as out-of-scope with rationale).

**Acceptance criteria**:
1. `go build ./...` and `go test ./...` both green at HEAD on the bundle-close commit.
2. Zero remaining occurrences of `NAI-19-D1` in `modules/world/npc.go` and `modules/world/npc_registry.go` — verified by `rg -n 'NAI-19-D1' modules/world/`.
3. `addPlayer(p)` followed by `s.zoneMap.Get(p.level, p.x, p.z).PlayersCount()` returns 1 (when p was the only player at that coord).
4. After `removePlayer(p)`, the same `PlayersCount()` returns 0 and `s.zoneMap.Grid(p.level).IsFlagged(p.x>>3, p.z>>3, 0)` returns false.
5. `addNpc(n)` followed by `Zone.NpcsCount()` returns 1.
6. NPC `revertType` heavy path (`removeNpc(n, -1); addNpc(n, -1, false)`) returns `Zone.NpcsCount` to its pre-call value (1 in, 1 out, 1 in again).
7. Player step that crosses zone boundary (e.g., from (3199, 3200) to (3200, 3200)) leaves the (399, 400) zone and enters (400, 400).
8. Player step within a zone (e.g., from (3200, 3200) to (3201, 3201)) does NOT call enter/leave (zone is unchanged).
9. Player Teleport from (3200, 3200, 0) to (4000, 4000, 1) leaves the (400, 400, 0) zone and enters (500, 500, 1) — both zone AND level changed.
10. Calling `removePlayer(p)` twice (defensive double-call) does not panic; second call's LeavePlayer is a no-op via the `zoneListElement == nil` guard.

**Tests** (~12-15 tests across `server_zone_subscription_test.go` (new), `npc_registry_test.go` (extension), `movement_test.go` (extension), `player_script_test.go` (extension), `npc_interaction_test.go` (extension), `npc_ai_test.go` (extension)):

- `TestAddPlayerEntersZoneAndFlagsGrid`
- `TestRemovePlayerLeavesZoneAndUnflagsGrid`
- `TestRemovePlayerDoubleCallIsNoop`
- `TestAddNpcEntersZoneNoGridFlag` — dual-pin (NPC enter does NOT flag grid)
- `TestRemoveNpcLeavesZone`
- `TestNpcRevertTypeHeavyPathLeavesAndReentersZone`
- `TestPlayerStepCrossZoneRefreshSubscription`
- `TestPlayerStepIntraZoneNoSubscriptionChange`
- `TestPlayerTeleportCrossZoneRefreshSubscription`
- `TestPlayerTeleJumpCrossLevelRefreshSubscription`
- `TestPlayerTeleportSameZoneNoRefresh` — same coord, no leave/enter
- `TestNpcStepCrossZoneRefreshSubscription`
- `TestNpcStuckTeleportRefreshSubscription` — `npc_interaction.go:95` site
- `TestNpcPatrolTeleportRefreshSubscription` — `npc_interaction.go:122` site
- `TestNpcRespawnTeleportRefreshSubscription` — `npc_ai.go:35` site

Per `plan_runnable_test_fixtures` memory: each fixture mentally executed at plan-write — verify the test player has a non-nil `client.server.zoneMap`, that `lastStepX/lastStepZ` are seeded to a known value at fixture setup, and that the helper for "step from A to B" actually invokes `stepOnce` (not just sets coords).

Per `plan_helper_coverage` memory: existing `newRegisteredNpc` and `newTestServer` helpers must be cross-checked — the helpers initialise `s.grid` (per `npc_hunt_entities_test.go:34, 172`) but do they initialise `s.zoneMap`? If not, Bundle 2 must extend the helpers OR the new tests must construct `zoneMap` explicitly. Re-grep at plan-write:
```bash
rg -n 'zoneMap = |s\.zoneMap = ' modules/world/*_test.go
```
Plan author records the per-fixture initialisation requirements.

**Deviation impact**: 1 retired (NAI-19-D1) → net -1. No new deviation tags introduced.

### Bundle 3 — Consumer migrations: `huntNpcs` + `huntPlayers` (~90 LOC + ~80 LOC tests)

**Goal**: Migrate the two read-side consumers of `pkg/grid` to use `pkg/zone` subscription instead. Validates the Bundle 1 primitive end-to-end with real consumers; demonstrates the migration pattern for future NAI sub-specs that retire `pkg/grid` entirely.

**Source mappings**:

- **`(*Npc).huntNpcs`** (`modules/world/npc_hunt_entities.go:23-76`):
  - Old: `nids := s.grid.NearbyNpcs(n.x, n.z, n.level, zoneRadius)` then loop over `nids` with `s.npcs[nid]` lookup.
  - New: `for _, zn := range s.zoneMap.NearbyZones(n.level, n.x, n.z, zoneRadius)` then `for other := range zn.NpcsSafe(false)` (or use `for _, e := range zn.NpcsRaw()` if a raw iterator is preferred for type-safety; plan author chooses based on whether the existing nil-check pattern (`if other == nil { continue }`) maps cleanly to `IsValid()` semantics).
  - The TS-faithful semantic is **Zone.npcs is the canonical NPC index**; iteration via `getAllNpcsSafe` is what TS HuntIterator uses internally. The level filter (TS HuntIterator skips wrong-level NPCs) is handled implicitly because NPCs only ever subscribe to one zone (their current `(level, x, z)`).
  - Cast at iteration: `zn.NpcsSafe` yields `NpcLike`. The huntNpcs body needs `(*Npc).typeId`, `.x`, `.z`, etc. — so the iterator value must be downcast back to `*Npc` via type assertion: `other, ok := nl.(*Npc); if !ok { continue }`.
- **`(*Npc).huntPlayers`** (`modules/world/npc_hunt.go:108-180`):
  - Old: `slots := s.grid.NearbyPlayers(n.x, n.z, n.level, zoneRadius)` then loop over `slots` with `s.players[slot]` lookup.
  - New: `for _, zn := range s.zoneMap.NearbyZones(n.level, n.x, n.z, zoneRadius)` then `for pl := range zn.PlayersSafe(false)` with the same `*Player` type assertion at iteration.
  - The level filter (`if p.level != n.level { continue }` at `:125`) becomes redundant — NearbyZones is already level-filtered. Plan author drops this defensive filter and pins the new behavior with a test (negative-pin per `ts_asymmetry_dual_pin`).

**Plan-author premise verification (per `controller_preflight`)**:
- Re-grep `s.grid.` consumers AT HEAD to confirm only `NearbyPlayers` and `NearbyNpcs` are used by these two functions (and no other consumers will be left orphaned by the migration). Expected at brainstorm:
  ```
  modules/world/npc_hunt.go:115  s.grid.NearbyPlayers(...)
  modules/world/npc_hunt_entities.go:28  s.grid.NearbyNpcs(...)
  modules/world/npc_script_lookup.go:10  // future: route via s.grid.NearbyNpcs (comment only — no actual call)
  ```
  Other `s.grid.` references at HEAD are write-side (`Add`/`Remove`/`AddNpc`/`RemoveNpc` in `tick.go` + `server.go` + test scaffolding `_test.go`), which Bundle 3 leaves untouched.
- Re-grep at plan-write to confirm no NEW `s.grid.NearbyPlayers`/`s.grid.NearbyNpcs` consumer has landed in any sub-spec since brainstorm.
- Validate that `s.zoneMap.NearbyZones` returns zones in the same order the existing tests assume; `ZoneMap.NearbyZones` at `pkg/zone/map.go:62-80` documents iteration order as "dx ascending then dz ascending" and the existing huntObjs/huntLocs (`npc_hunt_entities.go:101, 176`) already use this iterator without issue, so the pattern is proven.

**Acceptance criteria**:
1. `go build ./...` and `go test ./...` both green at HEAD.
2. All existing `TestHuntNpcs*` and `TestHuntPlayers*` tests continue to pass with no test-side changes (behavioral parity).
3. New negative-pin tests confirm that NPCs/players subscribed ONLY to `s.grid` (test fixtures that bypass `addPlayer`/`addNpc`) are NOT returned by huntNpcs/huntPlayers — proves the migration to Zone subscription is exclusive (no fallback to grid).
4. `npc_hunt.go` no longer references `s.grid.NearbyPlayers`. `npc_hunt_entities.go` no longer references `s.grid.NearbyNpcs`. Verified by `rg -n 's\.grid\.Nearby' modules/world/`.
5. `pkg/grid` write-side calls (in `tick.go`, `server.go`, and test scaffolding) remain untouched — Bundle 3 does NOT retire `pkg/grid`.

**Tests** (additions to `npc_hunt_entities_test.go` and `npc_hunt_test.go`):

- `TestHuntNpcsUsesZoneSubscriptionExclusive` — NPC registered in `s.grid` only (skipping `addNpc`'s zone enter) is NOT returned. Dual-pin per `ts_asymmetry_dual_pin`.
- `TestHuntPlayersUsesZoneSubscriptionExclusive` — same pattern for players.
- `TestHuntNpcsRespectsIsValidFilter` — NPC with `IsValid()==false` (e.g., `n.dead = true`) is filtered by `NpcsSafe`. Mirrors TS `getAllNpcsSafe` semantics.
- `TestHuntPlayersRespectsIsValidFilter` — same for players.

Per `plan_helper_coverage` memory: `npc_hunt_entities_test.go` test scaffolding currently calls `s.grid.AddNpc(nid, x, z, level)` directly (`:34, 172`). After Bundle 3 migration, these test helpers MUST also call `EnterNpc` — otherwise existing tests break because the migrated huntNpcs reads from Zone, not grid. Plan author updates the helper at the same commit; mentally executes each existing test against the updated helper to verify pass.

**Deviation impact**: 0. Pure consumer migration; no behavior change beyond which spatial index is queried.

### Polish commit (between Bundle 3 close and NAI-28 close)

Standard cadence: one polish commit absorbs minor review feedback from all three bundles. Per `dead_api_polish` memory, polish commit catches any helpers shipped with zero consumers — but in NAI-28's case, `pkg/zone` Subscription primitive has 2 guaranteed read-side consumers (huntNpcs, huntPlayers) ALREADY in Bundle 3, plus 11 write-side wire-through edits in Bundle 2. No `dead_api_polish` risk.

Polish-eligible items expected (re-evaluate at close):
- Doc-comment polish on `EnterPlayer`/`EnterNpc` if reviewer surfaces unclear narration
- Helper extraction if the per-step refreshZone block in `(*Player).stepOnce` and `(*Npc).stepOnce` evolved into duplication (already factored in Bundle 2 as `refreshPlayerZone` / `refreshNpcZone` per `plan_grep_helper_patterns`)
- Memory entries pre-flagged at brainstorm time:
  - `parallel_spatial_index_migration_pattern` — when migrating consumers from one spatial index to another, both indexes update on writes during the migration window; only retire the old index when ALL consumers migrate. Provenance: pkg/grid retention through NAI-28. Likely save.
  - `interface_at_cyclic_import_boundary` — when a primitive in `pkg/zone` needs to track types defined in `modules/world`, define minimal interfaces in `pkg/zone` rather than restructuring imports. Provenance: PlayerLike/NpcLike. Maybe save — depends on whether this pattern is uncommon enough to warrant memory.

## Tracked deviations

NAI-28 close removes one deviation tag and introduces zero:

- **Retired NAI-19-D1** (zone enter/leave omitted from `s.removeNpc` / `s.addNpc`): all 4 inline sites and 2 doc-comment sites in `npc.go` retired in Bundle 2.

**Inherited unchanged (no NAI-28 work):**

- NAI-11-D* (PLAYER* mode SMART pathfinding, reach helpers, etc. — all separate sub-spec tracks)
- NAI-19-D2 (AI_SPAWN trigger dispatch — already wired by NAI-19 itself, modulo any latent gap)
- All other deviations in `nai_followups.md`

**Active deviation count math**: 14 (post-NAI-27) − 1 (NAI-19-D1 retired) = **13**. Net **−1**.

## Plan-author verification gates (per `controller_preflight` + `spec_followup_tracker_freshness`)

Before each implementer dispatch, plan author / controller re-runs:

1. **Bundle 1 prereqs**:
   - `pkg/zone/` files at HEAD — confirm no new `Players`/`Npcs` field has been added in an intervening sub-spec
   - `iter.Seq` import availability
2. **Bundle 2 prereqs**:
   - `(*Player).IsValid` at `player.go:691` and `(*Player).Slot` at `:392` still exist with the documented signatures
   - `(*Npc).IsValid` at `npc.go:299` and `(*Npc).Nid` at `npc_source.go:10` still exist
   - All 11 wire-through call sites at the documented line numbers (the 8 logical groups expand to 11 edits: groups 1-6 are 1 edit each, group 7 is 2 edits, group 8 is 3 edits) — re-grep at HEAD
   - The 4 NAI-19-D1 inline comment sites grep-confirmable: `rg -n 'NAI-19-D1' modules/world/`
3. **Bundle 3 prereqs**:
   - `s.grid.NearbyPlayers` consumer count at HEAD is exactly 1 (`npc_hunt.go:115`) and `s.grid.NearbyNpcs` consumer count is exactly 1 (`npc_hunt_entities.go:28`)
   - `s.zoneMap.NearbyZones` semantics unchanged at `pkg/zone/map.go:62-80`
   - Test scaffolding helpers (`newRegisteredNpc`, `newTestServer`) — re-grep their bodies for `s.zoneMap = ` initialisation pattern; if absent, Bundle 3's tests must construct zoneMap explicitly OR Bundle 2's test scaffolding extension is a prerequisite

## Risks & mitigations

- **Bundle 1 risk: `iter.Seq` import not yet used in `pkg/zone/`.** Mitigation: pre-flight grep confirms; if absent, Bundle 1 introduces it (Go 1.23+ stdlib). The codebase elsewhere uses `iter.Seq` (per `use-modern-go` skill discipline).

- **Bundle 1 risk: `Zone.Reset()` side-effect change.** Per TS Zone.reset (`Zone.ts:197-201`), reset clears `events` + `entityEvents` + `shared` but NOT players/npcs lists. Goscape's existing `Reset()` at `pkg/zone/zone.go:43-47` clears events/entityEvents/shared and matches. Adding the new lists must NOT extend Reset's clear list — they persist across ticks. Mitigation: explicit test `TestZoneResetPreservesSubscription` in Bundle 1.

- **Bundle 2 risk: per-step refreshZone in `stepOnce` requires reading `s.zoneMap` from inside `(*Player).stepOnce`.** Player has `p.client.server.zoneMap` access pattern (per the existing `p.client.server.gamemap` access at `movement.go:78`). NPC stepOnce already has `s` parameter. Mitigation: factor `refreshPlayerZone(p, prevX, prevZ, prevLevel)` helper that handles the nil-guard chain.

- **Bundle 2 risk: teleport sites bypass `lastStepX`/`lastStepZ` snapshot.** `(*Player).Teleport`/`TeleJump` and the 3 NPC teleport sites all directly assign `entity.x = ...` without going through `stepOnce` — `lastStepX`/`lastStepZ` are NOT updated. Mitigation: each teleport site explicitly captures `prevX, prevZ, prevLevel` BEFORE the assignment and passes them to the refresh helper, NOT reading from `lastStepX`/`lastStepZ` (which would be stale).

- **Bundle 2 risk: cross-tick zone consistency under concurrent reads.** `pkg/zone` is single-tick-goroutine owned (per `server.go:669` `TrackZone` discipline). Subscription mutations happen inside `addPlayer` (under `playersMu.Lock()`) and `removePlayer` (same), and inside the per-tick goroutine for movement. Mitigation: NO mutex on the new subscription lists; explicit comment on `EnterPlayer`/`LeavePlayer` documenting "must be called from the tick goroutine OR under playersMu", mirroring the existing pattern.

- **Bundle 3 risk: type assertion overhead at huntNpcs/huntPlayers iteration.** `NpcsSafe` yields `NpcLike` (interface); the consumer downcasts to `*Npc`. Type assertion is O(1) and JIT-friendly in modern Go but is one extra check per iteration. Mitigation: this is acceptable overhead for the parallel-spatial-index migration pattern; benchmarks not required (existing tests pin behavior, not performance). If the overhead is reviewer-flagged at Bundle 3 close, polish commit can introduce typed iterator wrappers `Zone.NpcsTyped() iter.Seq[*Npc]` in pkg/zone — but this requires the cyclic-import constraint to be re-evaluated (would need pkg/entity to gain Player/Npc types).

- **Bundle 3 risk: hidden `s.grid.NearbyPlayers`/`NearbyNpcs` consumer.** Per `enumerate_all_sites`: implementer re-greps post-commit and confirms the brainstorm-stated 2-consumer count is still correct. If a third consumer landed in an intervening commit, Bundle 3 either migrates it (small) or flags it for follow-up tracker.

- **`controller_preflight` discipline at task dispatch.** Per memory: 30-second grep+Read pass against HEAD before each implementer dispatch to verify file paths, line numbers, signatures, helper init state. Applied per-bundle.

- **`spec_followup_tracker_freshness` discipline.** All assertions in this brainstorm (file paths, line numbers, method signatures, NAI-19-D1 inline comment sites, current `pkg/grid` consumer count) verified at HEAD `ef9ed20` at brainstorm-write. Plan author re-verifies at plan-write; controller re-verifies before each dispatch.

- **`audit_full_method_against_ts` discipline.** Bundle 1 plan author re-reads TS `Zone.ts:79-100` (enter/leave) and `Zone.ts:387-405` (getAllPlayersSafe/getAllNpcsSafe) line-by-line against the proposed Go shape. Bundle 2 plan author re-reads `PathingEntity.ts:170-190` (refreshZone) and `World.ts:941, 1268-1269, 1297-1299, 1587-1612` (login/spawn/despawn/logout) line-by-line.

## Review structure

Per `runescript_cadence` memory: two-stage review per bundle (spec compliance → code quality, both via opus). Final whole-impl review after all bundles.

- **Bundle 1**: Stage 1 spec compliance (DoublyLinkList API matches plan; PlayerLike/NpcLike interfaces match plan; ZoneGrid flag/unflag wiring matches TS Zone.ts L83/L94-96; NPC branch does NOT touch grid; Element.Unlink idempotent) + Stage 2 code-quality review (test naming, doc-comment narration matches plan, no unused fields/methods, generic constraint clarity).
- **Bundle 2**: Stage 1 spec compliance (all 8 logical wire-through groups land per plan, totalling 11 individual edits; NAI-19-D1 retirement complete in `npc.go` AND `npc_registry.go`; per-step refreshZone is INSIDE `stepOnce` not at `resolveMovement` tail; teleport sites use captured-before-mutation prev coords; double-leave defensive guard present) + Stage 2 code-quality review (helper factoring `refreshPlayerZone`/`refreshNpcZone`, test fixture realism, no shipped-with-zero-consumers helpers).
- **Bundle 3**: Stage 1 spec compliance (huntNpcs + huntPlayers both migrated; `s.grid.NearbyPlayers`/`s.grid.NearbyNpcs` both removed from production code; existing test pass; new negative-pin tests assert "no fallback to grid"; type assertion to `*Npc`/`*Player` is the canonical iteration pattern) + Stage 2 code-quality review (test scaffolding helpers updated to subscribe via Zone, level-filter redundancy correctly removed in huntPlayers, doc-comment migration narration).
- **Whole-impl review**: validates that NAI-28 retires NAI-19-D1 (zero matches in `rg -n 'NAI-19-D1' modules/`), the 14→13 deviation count is consistent across all bundle commit bodies, and the parallel-spatial-index pattern is correctly applied (write-side `pkg/grid` calls preserved; only read-side migrated).

Polish commits land if final whole-impl review surfaces remediable findings, per NAI-23 / NAI-24 / NAI-25 / NAI-26 / NAI-27 precedent.

## NAI-28 close

The close commit:

- Updates `nai_followups.md`: marks the From-NAI-19 NAI-19-D1 entry Resolved with NAI-28 close hash + per-bundle commit summary. Adds a new "From NAI-28" entry tracking the `pkg/grid` retirement followup ("Retire `pkg/grid` once write-side consumers (tick.go:320-322, 356-357; server.go:238) are migrated to Zone subscription writes; the read-side already migrated by NAI-28 Bundle 3").
- Per `close_commit_memory_trailer` memory: includes the standard `Co-Authored-By` trailer; carries `Closes memory: nai_followups.md` for the NAI-19-D1 retirement.
- Per `post_task_handoff` memory: at NAI-28 close, save non-derivable info to memory AND give the user a paste-ready resume prompt for NAI-29 (with HEAD hash, deviation count 13, and the most actionable next-NAI candidates including `pkg/grid` retirement).
- Memory entry candidates pre-flagged at brainstorm time (re-evaluate at close):
  - `parallel_spatial_index_migration_pattern` — when migrating consumers from one spatial index to another, both indexes update on writes during the migration window. Likely save.
  - `interface_at_cyclic_import_boundary` — pkg-internal interface for cross-package types is preferable to restructuring import graph. Maybe save.
  - `framing_drift_in_followup_tracker` — when a deferral entry says "X not ported," re-derive against HEAD: the actual gap may be narrower (NAI-19-D1's "Zone abstraction not ported" was actually "subscription primitive missing in an otherwise-90%-ported package"). Likely save — pairs with `tracker_entry_framing_can_be_incomplete`.

## Out-of-scope (explicitly deferred to future NAI sub-specs)

1. **`pkg/grid` retirement.** Write-side calls remain after NAI-28. A future sub-spec migrates `tick.go:320-322` (player movement) and `tick.go:356-357` (NPC movement) and `server.go:238` (NPC seeding) to use `Zone.EnterPlayer`/`LeavePlayer`/`EnterNpc`/`LeaveNpc` exclusively; once migrated, `pkg/grid/` is deletable. Tracked under new "From NAI-28" entry.

2. **NAI-11 deferred items.** SMART pathfinding, reach helpers, focus() instant flag — each its own substantial sub-spec. Inherited deferral; NAI-28 does not touch them.

3. **Observer-pass culling using subscription lists.** Player info pass / NPC info pass currently use `s.players`/`s.npcs` global slices. Migrating to per-zone subscription iteration is a TS-faithful follow-up that benefits from NAI-28's primitive but is its own sub-spec (touches PlayerInfo encoder, NpcInfo encoder).

4. **Zone-bounded loc/obj iteration migrations.** `huntObjs`/`huntLocs` (`npc_hunt_entities.go:95, 170`) already use `s.zoneMap.NearbyZones` — they don't need migration. Other potential consumers (script handlers that walk zones) are out-of-scope.

5. **`Zone.entityEvents` extension to PathingEntity.** Currently `entityEvents` is keyed by `*entity.NonPathing` (Loc + Obj). TS keys by `NonPathingEntity` (same). NAI-28 does not extend the events tombstoning path to PathingEntity; that's a separate concern from subscription tracking.

6. **`DoublyLinkable` abstract base port.** TS uses inheritance; goscape uses Element-based composition. Not a behavioral divergence (per `true_to_ts_gate` analysis); no follow-up tracker entry needed.
