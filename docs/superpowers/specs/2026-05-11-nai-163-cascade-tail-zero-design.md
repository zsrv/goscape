---
status: brainstorm-approved
date: 2026-05-11
ts_source:
  # B0 — Busy
  - LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:893-895 (BUSY)
  # B1 — LineOfSight
  - LostCityRS/Engine-TS/src/engine/script/handlers/ServerOps.ts:144-162 (LINE_OF_SIGHT)
  - LostCityRS/Engine-TS/src/engine/GameMap.ts:429-431 (isLineOfSight delegate)
  # B2 — NpcHunt
  - LostCityRS/Engine-TS/src/engine/script/handlers/NpcOps.ts:290-321 (NPC_HUNT)
  - LostCityRS/Engine-TS/src/engine/script/ScriptIterators.ts:234-295 (NpcHuntAllCommandIterator)
  # B3 — NpcAdd
  - LostCityRS/Engine-TS/src/engine/script/handlers/NpcOps.ts:42-53 (NPC_ADD)
  - LostCityRS/Engine-TS/src/engine/World.ts:1258-1294 (World.addNpc)
---

# NAI-163 — Cascade-tail to zero (4 ops; 4 bundles)

**Cadence:** ~70 LOC handlers + ~30 LOC interface adapter + ~200 LOC tests = ~300 LOC across four bundles. Below NAI-162's ~620 LOC envelope; runs as one sub-spec with per-bundle close commits (B0 → B1 → B2 → B3) and a final NAI-163 roll-up close that cites the tightened-regex recount.
**Tech stack:** Go 1.26+ (`go_version.md`).
**Cascade-tail context:** missing-handler audit at HEAD `0027628`, **using the tightened regex** `Op[A-Za-z][A-Za-z0-9]*` (per `missing_handler_audit_regex_flaw.md` — the original `Op[A-Za-z]+` regex collapsed `OpBusy` into the dispatched `OpBusy2`), reports **4 unhandled opcodes**:

```
OpBusy
OpLineOfSight
OpNpcAdd
OpNpcHunt
```

This sub-spec ports all four. Post-close cascade-tail: **0**. The NAI-162 handoff framed LineOfSight/NpcAdd/NpcHunt as "each is a discrete new subsystem"; exploration revealed all three sit on existing goscape infrastructure (LineValidator + IsFreeToPlay + HuntAll iterator + `(*Server).addNpc` allocator), reducing the binding cost to handler-only + one interface bridge for NpcAdd.

---

## §1 Symptom / motivation

NAI-162 closed sweep #4 (15 ops; 18 → 3 by the original audit, 4 by the tightened audit). The remaining 4 ops split cleanly:

| Class | Ops | Count | TS body | New infra |
|---|---|---:|---|---|
| **Player gate** | OpBusy 2005 | 1 | 1 line | none |
| **World geometry gate** | OpLineOfSight 1005 | 1 | ~10 lines | none — `s.LineValidator`, `s.World.IsFreeToPlay`, `s.World.MapMembers` all exist |
| **NPC iterator scan** | OpNpcHunt 2525 | 1 | ~30 lines | none — `NewHuntAllNpcIterator` already exists (NAI-35-T3, used by `OpNpcHuntAll`) |
| **NPC entity create** | OpNpcAdd 2500 | 1 | ~10 lines | new `World.AddNpcAt` interface method + `modules/world` adapter |

**Re-confirmed at HEAD `0027628` via direct re-read of TS source per `spec_ts_source_read.md`:**

| Op | Re-confirmed status |
|---|---|
| OpBusy | TS body present at `PlayerOps.ts:893-895`: `pushInt(busy() || loggingOut ? 1 : 0)`. Distinct from OpBusy2 (which uses `hasInteraction() || hasWaypoints()`); `loggingOut` is the gating accessor. Goscape has `OpBusy2` dispatched at `handlers.go:425`; `OpBusy` declared at `opcode.go:105` but not dispatched. Regex-flaw (per `missing_handler_audit_regex_flaw.md`) hid this until NAI-163 brainstorm tightened the audit. |
| OpLineOfSight | TS body present at `ServerOps.ts:144-162`: pops `[from, to]`, early-return 0 on `from.level !== to.level`, F2P gate on `!NODE_MEMBERS && !World.gameMap.isFreeToPlay(to.x, to.z)`, then pushes `isLineOfSight(from.level, from.x, from.z, to.x, to.z) ? 1 : 0`. Goscape's `s.LineValidator.HasLineOfSight` already exists at `pkg/pathfinder/routefinder/linevalidator.go:19`; `s.LineValidator` is exposed on ScriptState at `pkg/script/state.go:268`; existing private helper `isLineOfSight(s, ...)` lives at `pkg/script/handlers_map.go:178-185`. Handler-only port. |
| OpNpcHunt | TS body present at `NpcOps.ts:290-321`: constructs `NpcHuntAllCommandIterator` (= goscape's `NewHuntAllNpcIterator` at `npc_iterator.go:209`), iterates, picks smallest euclidean² with `<=` tie-break (later iterator yield wins), sets ActiveNpc + pushes 1 (or 0 if none). Iterator reused as-is. |
| OpNpcAdd | TS body present at `NpcOps.ts:42-53`: pops `[coord, id, duration]`, validates via CoordValid/NpcTypeValid/DurationValid, constructs `new Npc(level, x, z, size, size, EntityLifeCycle.DESPAWN, World.getNextNid(), typeId, moverestrict, blockwalk)`, calls `World.addNpc(npc, duration)`, sets ActiveNpc. Goscape's `(*Server).addNpc(n, duration, firstSpawn=true)` at `modules/world/npc_registry.go:48` is the equivalent; `allocNpcSlot()` stands in for `getNextNid()`. Cyclic-import boundary requires a new interface method on the World contract. |

**Cohort definition (cascade-tail-zero):**
1. **Cascade-tail closure** — drain audit from 4 → 0 in one sub-spec close.
2. **No new subsystems** — all ports use existing infra. NpcAdd's `AddNpcAt` is a thin factory wrapper around existing `addNpc`, not a new subsystem.
3. **TS body ≤30 lines per handler** — same envelope as NAI-160/161/162 sweeps.

**Bundle ordering: B0 → B1 → B2 → B3.** Only B3 has a real prerequisite (interface method must land before handler can dispatch); B0–B2 are independent. Ordering chosen for ascending complexity. Each bundle has its own close commit; final NAI-163 close rolls them up.

---

## §2 Architecture

### §2.1 Bundle table

| Bundle | Op | TS source | New infra | Est. LOC |
|---|---|---|---|---:|
| **B0** | `OpBusy` 2005 | `PlayerOps.ts:893-895` | none (or `(*Player).LoggingOut` getter if absent at HEAD) | ~5 + ~15 test |
| **B1** | `OpLineOfSight` 1005 | `ServerOps.ts:144-162` | none | ~20 + ~40 test |
| **B2** | `OpNpcHunt` 2525 | `NpcOps.ts:290-321` | none | ~25 + ~50 test |
| **B3** | `OpNpcAdd` 2500 | `NpcOps.ts:42-53` | `World.AddNpcAt(level, x, z, typeID, duration int) (Npc, error)` factory + `modules/world` adapter | ~20 handler + ~30 adapter + ~50 test |

**Cascade-tail recount in each close commit:** 4 → 3 → 2 → 1 → 0.

### §2.2 TS-fidelity gates per op

- **OpBusy:** the `loggingOut` term distinguishes BUSY from BUSY2. Pin with a `loggingOut=true, busy()=false` fixture.
- **OpLineOfSight:** level-mismatch early return must fire **before** the F2P gate, which must fire **before** the LineValidator call. Order pinned by recording-stub tests asserting zero calls when an earlier gate trips.
- **OpNpcHunt:** TS uses `<=` (`NpcOps.ts:307`), so equidistant ties prefer the later iterator yield. Observable; pin per `ts_asymmetry_dual_pin.md`.
- **OpNpcAdd:** TS constructs `EntityLifeCycle.DESPAWN`. The `AddNpcAt` adapter must hard-set despawn-lifecycle and route through `firstSpawn=true`. Duration handling already correct in goscape's `addNpc` (`npc_registry.go:106` writes `lifecycleTick = duration` if `duration > -1`).

---

## §3 Components

### §3.1 B0 — `handleBusy`

- **`pkg/script/handlers_player.go`:** new ~5-line `handleBusy(s)`, mirrors `handleBusy2` shape (`handlers_player.go:1326+`).
- **`pkg/script/handlers.go`:** dispatch entry `OpBusy: handleBusy,` next to the existing `OpBusy2: handleBusy2,` line.
- If `(*ActivePlayer).LoggingOut()` accessor is missing at HEAD, B0 lands it on the Player interface in `pkg/script/state.go` + the `modules/world/script.go` adapter (re-grep per `plan_sibling_site_guard_audit.md`).

### §3.2 B1 — `handleLineOfSight`

- **`pkg/script/handlers_map.go`:** new ~20-line `handleLineOfSight(s)`. Pop order: bottom-first `[from, to]` per TS `state.popInts(2)` — plan-author replicates the existing pop-2 idiom (check whether `s.PopInts(2)` exists or two sequential `s.PopInt()` mirror).
- Pre-checks: `checkCoord(from)`, `checkCoord(to)`. Level-mismatch → push 0. F2P gate on **dest only** via `s.World.MapMembers() == 0 && !s.World.IsFreeToPlay(to.x, to.z)` → push 0.
- Raycast via existing helper `isLineOfSight(s, from.level, from.x, from.z, to.x, to.z)` at `handlers_map.go:178`; push `1`/`0`.
- **`pkg/script/handlers.go`:** dispatch entry next to `OpLineOfWalk`.

### §3.3 B2 — `handleNpcHunt`

- **`pkg/script/handlers_npc.go`:** new ~25-line `handleNpcHunt(s)`. Pop order: `huntvis`, `distance`, `coord` (top first). Validate via `checkCoord` + `checkNotNull(distance)` + `checkHuntVis`.
- Nil-`s.Npcs` → push 0 + `return nil` (precedent: `handleNpcFindAll` at `handlers_npc.go` ~795).
- Construct local iterator `it := NewHuntAllNpcIterator(s.Npcs, s.LineValidator, s.World.CurrentTick(), level, x, z, distance, huntvis)`. **Local-only** — do NOT stash in `s.npcIterator`; the iterator's lifetime is bounded by this handler invocation.
- Iterate via `it.Next()` + `it.Stale(...)` (mirror `handleNpcFindNext` at `handlers_npc.go:863`). Track closest by `(dx*dx + dz*dz)` with `<=` (later yield wins on tie).
- On hit: `setActiveNpcSlot(s, closest)` + push 1. On miss: push 0.
- **`pkg/script/handlers.go`:** dispatch entry between `OpNpcHuntAll` and `OpNpcInRange`.

### §3.4 B3 — `handleNpcAdd` + `World.AddNpcAt`

**Interface (B3 prerequisite, lands first within the bundle):**

- **`pkg/script/state.go`:** add `AddNpcAt(level, x, z, typeID, duration int) (Npc, error)` to the `World` interface. The return `Npc` is the existing `pkg/script.Npc` interface (concrete `*world.Npc` satisfies it).
- **`modules/world/script.go`:** adapter impl. NpcType lookup via the existing field (plan-author re-greps `s.npcTypes` or equivalent per `plan_grep_helper_patterns.md`). Construct:

  ```go
  // Illustrative — plan-author re-greps modules/world/npc.go for the
  // actual Npc struct field names (per plan_type_name_grep.md) and the
  // goscape-side despawn-lifecycle constant (likely
  // entity.LifeCycleDespawn or similar — TS calls it
  // EntityLifeCycle.DESPAWN). Cross-reference test_helper_bypass_masks_
  // production_path.md for the production spawn-site shape.
  n := &Npc{
      level:        level,
      startX:       x,
      startZ:       z,
      baseType:     typeID,
      typeId:       typeID,
      size:         int(typ.Size),
      blockWalk:    typ.BlockWalk,
      moveRestrict: typ.MoveRestrict,
      typ:          typ,
      lifecycle:    <goscape-despawn-constant>,
  }
  ```

  Then call `s.addNpc(n, duration, true)`. Return `(n, nil)` on success; bubble `errNpcsFull` on full.

**Handler:**

- **`pkg/script/handlers_npc.go`:** new ~20-line `handleNpcAdd(s)`. Pop order: `duration`, `id`, `coord` (top first). Validate via `checkCoord` + `checkNpcType(id)` + `checkDuration`.
- Call `npc, err := s.World.AddNpcAt(level, x, z, id, duration)`. On error → return error (existing log-warn + ClearActiveScript path handles it). On success → `setActiveNpcSlot(s, npc)`. **No push** (TS handler doesn't push).
- **`pkg/script/handlers.go`:** dispatch entry next to `OpNpcAnim`.

---

## §4 Data flow

### B0 OpBusy

```
script → handleBusy
       → push (s.ActivePlayer.Busy() || s.ActivePlayer.LoggingOut()) ? 1 : 0
```

### B1 OpLineOfSight

```
script → pop [from, to] → checkCoord×2
       → if from.level != to.level: push 0; return
       → if !s.World.MapMembers() && !s.World.IsFreeToPlay(to.x, to.z): push 0; return
       → push isLineOfSight(s, from.level, from.x, from.z, to.x, to.z) ? 1 : 0
```

Note ordering: level-mismatch fires before F2P gate, which fires before LineValidator call. Each test asserts the relevant skip via recording stubs.

### B2 OpNpcHunt

```
script → pop [coord, distance, huntvis] → checkCoord, checkNotNull, checkHuntVis
       → if s.Npcs == nil: push 0; return
       → tick := s.World.CurrentTick()
       → it := NewHuntAllNpcIterator(s.Npcs, s.LineValidator, tick, level, x, z, distance, huntvis)
       → loop:
            if it.Stale(tick): return error
            npc, ok := it.Next(); if !ok break
            dx, dz := npc.NpcX()-x, npc.NpcZ()-z
            d := dx*dx + dz*dz
            if d <= closestDist: closest = npc; closestDist = d
       → if closest == nil: push 0; return
       → setActiveNpcSlot(s, closest); push 1
```

### B3 OpNpcAdd

```
script → pop [coord, id, duration] → checkCoord, checkNpcType, checkDuration
       → npc, err := s.World.AddNpcAt(level, x, z, id, duration)
       → if err != nil: return err
       → setActiveNpcSlot(s, npc)

modules/world.AddNpcAt:
       → typ := s.npcTypes.Get(typeID)
       → if typ == nil: return nil, errNpcTypeUnknown   // defensive; TS checkNpcType already rejects
       → n := &Npc{... lifecycle: DESPAWN, ...}
       → if err := s.addNpc(n, duration, true); err != nil: return nil, err
       → return n, nil
```

---

## §5 Error handling

| Path | Behavior |
|---|---|
| Stack-underflow (popInt below empty) | `s.PopInt()` returns 0 via existing impl; validators (`checkCoord`, `checkNpcType`, `checkDuration`, `checkHuntVis`, `checkNotNull`) reject the resulting out-of-range value → return error from handler. Dispatch loop logs + ClearActiveScript. |
| nil-`s.ActivePlayer` (B0) | Mirror existing `handleBusy2` pattern — plan-author re-greps the exact guard (`requireActivePlayer` helper or inline nil-check) and reproduces. |
| nil-`s.LineValidator` (B1) | Existing private helper `isLineOfSight` at `handlers_map.go:181-184` returns `true` on nil (pessimistic-allow); B1 inherits that behavior. Matches TS shape (TS calls unconditionally; goscape's nil-guard is a goscape-defensive addition, labeled "(goscape defensive; TS skips this check)" per `defensive_gate_doc_comment_label.md`). |
| nil-`s.Npcs` (B2) | Push 0 + `return nil`. Matches `handleNpcFindAll` / `handleNpcFindAllZone` precedent. |
| Registry-full (B3) | `errNpcsFull` bubbles `s.addNpc` → `AddNpcAt` → handler → dispatch loop. |
| Stale iterator (B2) | Return error matching `handleNpcFindNext` (`handlers_npc.go:869-871`). |

**Deviations tracked at spec time:** none. Three "anticipated-if-needed" tags:

- **NAI-163-D-LOS-ARG-SHAPE-FIX** — opens iff R1 trips (LineValidator arg-shape divergence between goscape wrapper and TS `rsmod` call).
- **NAI-163-D-LOS-LEVEL-SHORTCIRCUIT** — opens iff plan-author finds an existing goscape helper that does the gates in opposite order (per `spec_diagram_order_divergence.md`).
- **NAI-163-D-NPCHUNT-TIE-BREAK-PIN** — opens iff B2 T4 (tie-break test) needs to pin a deviation rather than match TS shape.

---

## §6 Testing

### §6.1 B0 — `handleBusy`

1. `TestHandleBusy_NotBusy_NotLoggingOut_PushZero` — both arms false → push 0.
2. `TestHandleBusy_Busy_PushOne` — `busy()=true` → push 1.
3. `TestHandleBusy_LoggingOut_PushOne` — `busy()=false, loggingOut=true` → push 1. **Pins the loggingOut arm** (distinguishes BUSY from BUSY2).

### §6.2 B1 — `handleLineOfSight`

1. `TestHandleLineOfSight_LevelMismatch_PushZero` — different levels → push 0; recording-LineValidator-stub asserts zero calls.
2. `TestHandleLineOfSight_F2PGate_NonMembersWorld_PushZero` — `MapMembers=0`, dest in P2P zone → push 0; recording stub asserts zero LineValidator calls.
3. `TestHandleLineOfSight_F2PGate_MembersWorld_Bypasses` — `MapMembers=1`, dest in P2P zone → LineValidator IS called.
4. `TestHandleLineOfSight_RayClear_PushOne` — stub returns true → push 1.
5. `TestHandleLineOfSight_RayBlocked_PushZero` — stub returns false → push 0.
6. `TestHandleLineOfSight_ArgShape` — recording stub captures args; assert `(level=from.level, srcX=from.x, srcZ=from.z, destX=to.x, destZ=to.z, srcSize=1, destWidth=0, destLength=0, extraFlag=0)`. **See R1 — verify expected shape against routefinder docs before pinning** per `audit_subagent_fabrication.md` and `tracker_expected_value_premise_pretrace.md`.

### §6.3 B2 — `handleNpcHunt`

1. `TestHandleNpcHunt_NoNpcsInRange_PushZero` — empty iterator → push 0, no ActiveNpc set.
2. `TestHandleNpcHunt_SingleNpc_PicksIt` — one NPC in range → ActiveNpc set, push 1.
3. `TestHandleNpcHunt_PicksClosest_ByEuclideanSquared` — three NPCs at varying distances; closest selected.
4. `TestHandleNpcHunt_TieBreak_PrefersLaterYield` — two equidistant NPCs; pin `<=` tie-break (later yield wins).
5. `TestHandleNpcHunt_HuntVisLineOfSight_FiltersBlocked` — recording LineValidator returns false for one NPC; assert filtered out.
6. `TestHandleNpcHunt_NilNpcs_PushZero` — nil-`s.Npcs` → push 0 + nil return.
7. `TestHandleNpcHunt_StaleIterator_ReturnsError` — tick-drift mid-handler → error (matches `handleNpcFindNext`).

### §6.4 B3 — `handleNpcAdd` + `AddNpcAt`

**pkg/script handler tests** (mock World):

1. `TestHandleNpcAdd_Success_SetsActiveNpc` — mock `AddNpcAt` returns `(npc, nil)` → ActiveNpc set + pointer added; no push.
2. `TestHandleNpcAdd_RegistryFull_ReturnsError` — mock returns `errNpcsFull` → error bubbles.
3. `TestHandleNpcAdd_InvalidCoord_ReturnsError` — `checkCoord` error.
4. `TestHandleNpcAdd_InvalidNpcType_ReturnsError` — `checkNpcType` error.
5. `TestHandleNpcAdd_InvalidDuration_ReturnsError` — `checkDuration` error.

**modules/world adapter tests** (real `AddNpcAt`, mirror `npc_registry_test.go`):

6. `TestAddNpcAt_AllocsNidAndRegisters` — assert npc in `s.npcs[nid]` + `s.npcLoop` after call.
7. `TestAddNpcAt_SetsDespawnLifecycle` — pin `EntityLifeCycle.DESPAWN`.
8. `TestAddNpcAt_WritesLifecycleTick` — `duration > -1` → `n.lifecycleTick = duration`.
9. `TestAddNpcAt_RegistryFull_ReturnsErrNpcsFull` — fill `s.npcs`, assert error.
10. `TestAddNpcAt_PopulatesSizeBlockWalkMoveRestrict` — assert `n.size = typ.Size`, `n.blockWalk = typ.BlockWalk`, `n.moveRestrict = typ.MoveRestrict`.

### §6.5 Smoke handoffs (post-merge, user-launched)

Per `smoke_test_server_handoff.md` and `cascade_theory_smoke_binding.md`:

- **B0**: trigger a content script that gates on `BUSY` (plan-author greps `Content/scripts/` for `~busy` / `busy` callers).
- **B1**: spawn two coord points with a known LOS-blocking loc between them; verify LineOfSight true/false flip matches OSRS reference.
- **B2**: trigger a content script calling `npc_hunt` with 2+ NPCs at varying distances; verify closest selected.
- **B3**: trigger a content script calling `npc_add`; verify NPC appears and despawns after duration ticks.

Plan-author re-greps `Content/scripts/` for actual callers at plan-write time; smokes bind only if real content exercises the path.

---

## §7 Risk register

| # | Risk | Mitigation | Trip-wire |
|---|---|---|---|
| R1 | LineValidator arg shape divergence — existing goscape wrapper passes `(1, 0, 0, 0)` (handlers_map.go:184); TS `rsmod.hasLineOfSight` is called `(1, 1, 1, 1, 0)`. The `0, 0, 0` triplet may collapse `destWidth`/`destLength` semantics. | Plan-author re-reads `pkg/pathfinder/routefinder/linevalidator.go` impl and Rust-side `2004scape/rsmod` reference per `rust_source_canonical_path.md`. If existing wrapper is wrong, fix as B1 prerequisite (1-line patch) + open `NAI-163-D-LOS-ARG-SHAPE-FIX`. | B1 T6 (arg-shape test) fails against real LineValidator. |
| R2 | `OpBusy`'s `loggingOut` term — goscape may lack an exposed accessor on Player. | Plan-author re-greps `(*Player).Lo*` for the existing field; expose getter on Player interface if absent. | B0 plan-author fails to find field; flags + escalates before coding. |
| R3 | NpcAdd `EntityLifeCycle.DESPAWN` vs `RESPAWN` — wrong constant means script-spawned NPCs survive shutdown / never despawn. | B3 T7 pins despawn-lifecycle explicitly. | Smoke handoff: `npc_add` with `duration=10` — NPC must vanish at tick 10. |
| R4 | TS `<=` vs `<` in NpcHunt closest-selection tie-break (`NpcOps.ts:307`) — observable behavior, easy to mis-port as `<`. | Plan-author re-reads `NpcOps.ts:303-312` verbatim and codifies in plan per `spec_ts_source_read.md`. | B2 T4 fails if `<` was ported. |
| R5 | NpcAdd Npc-construction may need fields beyond size/blockWalk/moveRestrict (huntRange, huntMode, starting stats). | `resetEntityForRespawn` (`npc_registry.go:121`) already reseeds stats + hunt fields when called from `addNpc`. Plan-author confirms reseed runs unconditionally on first-spawn path. | B3 adapter test asserts non-zero level-0 stat after `AddNpcAt` (plan-author uses goscape's actual stat-field name; `n.levels[]` is the placeholder name pending re-grep). |
| R6 | Audit regex flaw recurs at NAI-164 — `missing_handler_audit.md` itself is not updated (declined in scoping vote). | NAI-163 close commit body documents the tightened regex (`Op[A-Za-z][A-Za-z0-9]*`) in the recount section. Future cohorts grep the close commit. | Future sweep mis-counts; controller pre-flight catches it. |

---

## §8 Out of scope

- **`OpHuntAll` re-port** — already dispatched at HEAD (`handlers.go:497`); not in this cascade-tail.
- **`missing_handler_audit.md` regex update** — declined in scoping vote. Tightened regex is documented in NAI-163 close commit body only.
- **NpcAdd cache-driven respawn path** — `EntityLifeCycle.RESPAWN` NPCs are the world-boot path; NPC_ADD is script-spawned only.
- **`World.removeNpc` / `OpNpcDel` audit** — `OpNpcDel` already dispatched at HEAD; not a cascade-tail member.
- **Iterator pattern refactor** — `iterator_state_pattern.md` template already implemented in `NpcIterator`; B2 reuses, does not refactor.
- **NAI-162 carry-forwards** — WealthEvent struct fields (RecipientItems/RecipientValue), ActivePlayer.Session(), OPHELD trigger plumbing, B0-stub re-ports (PUSH_VARBIT 25, POP_VARBIT 27, SET_GENDER 2099, LC_OP 4105, OC_IOP 4205, OC_OP 4208): all explicitly out of NAI-163 scope per NAI-162 §8 handoff.
- **OPHELD / OcOp / LcOp / OcIop content-trigger cohort** — still pending; future NAI-N.
