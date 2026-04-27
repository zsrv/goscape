# NAI-34 — `npc_tele` opcode 2541 + `Npc.Teleport` extraction

## Motivation

NPC_TELE (opcode 2541) is declared at `pkg/script/opcode.go:278` and stringified at `:955`, but no handler is registered in the dispatch table at `pkg/script/handlers.go`. Classic stub-not-completed shape (see `protocol_stub_not_completed.md`): tests pass against the missing entry because no test exercises the script-VM dispatch path for opcode 2541.

The visible smoke gate is `fishing_movement.rs2:10` (LostCity Content):

```
[label,move_fishing_spot]
def_coord $rand_coord = ~fishing_spot_random_coord(npc_type);
npc_delay(2);
npc_tele($rand_coord);    // ← opcode 2541, currently aborts the script
npc_settimer(calc(280 + random(250)));
```

Pre-NAI-34 behavior: the ai_timer fires every ~280-530 ticks, the script enters `move_fishing_spot`, succeeds at `npc_delay(2)`, then aborts with `Aborted` execution state at `npc_tele($rand_coord)` (per `runner_test.go:8`). Fishing NPCs never relocate; `npc_settimer` never reached. Each abort logs `no handler for opcode 2541`.

Post-NAI-34 behavior: NPCs visibly relocate within their `fishing_movement_enum` coord set; error log goes silent. Closes NAI-33 spec item 4 (the visible fishing-spot-relocate smoke gate that NAI-33 left blocked one opcode downstream).

NAI-34 is the highest-priority follow-up from `nai_followups.md`'s "From NAI-33" section per the user's prompt.

## Tech stack

- Go 1.26+
- Existing helpers: `checkCoord` (handlers_npc.go:8 — mirrors TS CoordValid), `requireActiveNpc` (handlers_npc.go:75 — mirrors TS `checkedHandler(ActiveNpc, ...)`), `unpackCoord` (handlers_player.go:18), `refreshNpcZone` (zone_refresh.go:33 — has built-in same-zone short-circuit at :39).
- Reference shape: `Player.Teleport(x, z, level int)` at `modules/world/player_script.go:226` — established 5-line goscape teleport convention.
- TS source: `Engine-TS/src/engine/script/handlers/NpcOps.ts:443` (handler), `Engine-TS/src/engine/entity/PathingEntity.ts:267` (base method).

## Scope

**In scope:**

1. New `(n *Npc) Teleport(x, z, level int)` method in `modules/world/npc_script.go` mirroring `Player.Teleport` shape exactly.
2. Refactor 2 existing inline NPC-teleport sites in `modules/world/npc_interaction.go` (wanderMode home-tele at ~`:97`, patrolMode waypoint-tele at ~`:126`) to call `n.Teleport(...)`. Behavior-preserving — no semantics change.
3. New `Teleport(x, z, level int)` method on `pkg/script/active.go` `ActiveNpc` interface.
4. New `handleNpcTele` function in `pkg/script/handlers_npc.go` — pop 1 packed coord, validate via `checkCoord`, delegate to `s.ActiveNpc.Teleport(x, z, level)`.
5. Register `OpNpcTele: handleNpcTele` in `pkg/script/handlers.go` dispatch table.
6. Drive-by docstring fix in `modules/world/zone_refresh.go:28-29` — the "3 NPC teleport sites" comment is stale; enumerate the actual call sites accurately.

**Out of scope (with rationale):**

- The 5 TS divergences in `PathingEntity.teleport` (level clamp, unallocated-zone rejection, `focus()` orientation, `lastStepX/Z` adjust, `previousLevel != level` branch) — tracked as deviations NAI-34-D1..D5; closure plan = future `pathing-entity-teleport-parity` sub-spec that closes both Player and Npc divergences in one sitting (matching the existing `Player.Teleport` reduced shape preserves consistency until that parity sub-spec lands).
- The patrolMode `level = 0` literal at `npc_interaction.go:126` (TS Npc.ts:729 uses `dest.level`). Pre-existing divergence that NAI-34's refactor surfaces but does NOT introduce — tracked as a separate `nai_followups.md` entry, not as a NAI-34 deviation.
- NPC_TELEJUMP (no corresponding TS opcode for NPCs).
- NPC_WALK (opcode 2542; TS NpcOps.ts:451-455 — `checkedHandler(ActiveNpc) + CoordValid + queueWaypoint(x, z)`). Same shape as NPC_TELE but calls `n.queueWaypoint(x, z)` instead of `n.Teleport`. Tracked as a separate `nai_followups.md` entry.
- The other 4 deferred NAI-33 stubs: NPC_PARAM (2529), MAP_PLAYERCOUNT (1015), HUNTALL (2031), MAP_FINDSQUARE (1009) — independent of fishing-spots, deferred to per-opcode sub-specs.
- Player.Teleport closure of its 5 mirror divergences (would dramatically expand scope).

## Architecture

NAI-34 is a single-bundle sub-spec. The work is structurally a **stub fill** plus a **3-callsite extraction refactor** (1 new caller + 2 existing callers reuse). Cross-package surface: `pkg/script` ↔ `modules/world`, with one new interface method on `ActiveNpc`.

**File layout:**

| File | Change | Production lines |
|---|---|---|
| `pkg/script/active.go` | `ActiveNpc.Teleport(x, z, level int)` interface method | +5 |
| `pkg/script/handlers_npc.go` | `handleNpcTele` function | +15 |
| `pkg/script/handlers.go` | `OpNpcTele: handleNpcTele,` dispatch entry | +1 |
| `modules/world/npc_script.go` | `(n *Npc) Teleport(x, z, level int)` extraction | +12 |
| `modules/world/npc_interaction.go` | Refactor wanderMode + patrolMode inline sites | -8 / +2 |
| `modules/world/zone_refresh.go` | Doc-comment fix (drive-by) | +3 / -1 |
| **Total** | | **~22 net lines added, ~32 lines touched** |

Cross-cutting symmetry: `Player.Teleport(x, z, level)` and `Npc.Teleport(x, z, level)` share an identical 5-line body modulo the receiver type. Keep them visibly symmetric so a reviewer reads "yes, this is the established pattern" instantly.

## Implementation specifics

### Task 1 — `Npc.Teleport` world-side method

In `modules/world/npc_script.go`, append after the existing read methods (which end around line 71 `SetNpcVarN`):

```go
// Teleport moves the NPC to (x, z, level), refreshes its zone
// subscription if the zone changed, and flags the client for a tele
// transition (no walk-anim interpolation). Mirrors Player.Teleport at
// player_script.go:226.
//
// Used by NPC_TELE script handler (pkg/script/handlers_npc.go) and by
// AI teleport sites — wanderMode home-tele (npc_interaction.go ~:97)
// and patrolMode waypoint-tele (~:126).
//
// DEVIATION NAI-34-D1..D5 vs TS PathingEntity.teleport (PathingEntity.ts:267):
// no level clamp, no unallocated-zone rejection, no focus(), no
// lastStepX/Z adjust, no previousLevel != level branch. Mirrors the
// established Player.Teleport reduced shape. Closure plan: future
// pathing-entity-teleport-parity sub-spec aligns both Player + Npc.
func (n *Npc) Teleport(x, z, level int) {
    prevX, prevZ, prevLevel := n.x, n.z, n.level
    n.x, n.z, n.level = x, z, level
    refreshNpcZone(n.server, n, prevX, prevZ, prevLevel)
    n.tele = true
}
```

`n.server` is set by `Server.addNpc` (already used by `SetDelayed` at npc.go:219 and `revertType` at :294). Test fixtures must wire `n.server = s` before calling `n.Teleport` (per the established pattern at `npc_ai_test.go:51-52`).

### Task 2 — Refactor wanderMode + patrolMode inline sites

In `modules/world/npc_interaction.go` around line 97 (wanderMode home-tele), replace:

```go
prevX, prevZ, prevLevel := n.x, n.z, n.level
n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
refreshNpcZone(s, n, prevX, prevZ, prevLevel)
n.tele = true
```

with:

```go
n.Teleport(n.startX, n.startZ, n.startLevel)
```

In the same file around line 126 (patrolMode waypoint-tele), replace:

```go
prevX, prevZ, prevLevel := n.x, n.z, n.level
n.x, n.z, n.level = dest.X, dest.Z, 0
refreshNpcZone(s, n, prevX, prevZ, prevLevel)
n.tele = true
```

with:

```go
n.Teleport(dest.X, dest.Z, 0)
```

**Note:** the literal `0` for level is preserved as-is. TS Npc.ts:729 uses `dest.level`, but the existing goscape inline code uses `0`. This is a pre-existing divergence that NAI-34's refactor surfaces but does NOT introduce. Logged as a separate `nai_followups.md` entry.

In `modules/world/zone_refresh.go:28-30`, replace the stale doc comment:

```go
// refreshNpcZone is the NPC-side analogue of refreshPlayerZone. Called from
// (*Npc).stepOnce, (*Npc).Teleport (used by wanderMode home-tele,
// patrolMode waypoint-tele, and the NPC_TELE script handler), and the
// respawn lifecycle path in (*Npc).turn (npc_ai.go ~:37).
//
// NPC enter/leave do NOT touch ZoneGrid (only player branch flags).
```

### Task 3 — `ActiveNpc.Teleport` interface method + `handleNpcTele` script handler

These two land in a single commit because the handler references `s.ActiveNpc.Teleport(...)` — the interface method must exist for the package to compile.

In `pkg/script/active.go`, in the `ActiveNpc` interface (currently ending around line 470 `SetHuntMode`), add:

```go
// Teleport moves the active NPC to (x, z, level) and flags the client
// for a tele transition. Mirrors (n *Npc).Teleport on the world side.
// Called by NPC_TELE handler (handlers_npc.go) after checkCoord
// validates and unpacks the packed coord.
//
// DEVIATION NAI-34-D1..D5 — see Npc.Teleport doc comment for the
// full divergence list and closure plan.
Teleport(x, z, level int)
```

In `pkg/script/handlers_npc.go`, add (sibling shape to `handleNpcDelay` at ~:296):

```go
// handleNpcTele (NPC_TELE, opcode 2541) teleports the active NPC to
// the packed coord. Pop order: coord (single int). Mirrors TS
// NpcOps.ts:443 — checkedHandler(ActiveNpc) + CoordValid +
// activeNpc.teleport(x, z, level).
func handleNpcTele(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_TELE"); err != nil {
        return err
    }
    coord := s.PopInt()
    level, x, z, err := checkCoord(coord, "NPC_TELE")
    if err != nil {
        return err
    }
    s.ActiveNpc.Teleport(x, z, level)
    return nil
}
```

### Task 4 — Dispatch wiring

In `pkg/script/handlers.go`, add `OpNpcTele: handleNpcTele,` in the existing NPC mutator block. Suggested position: between `OpNpcSetTimer` (~line 347) and the FIND family (~line 350-358) so it sits with sibling mutators.

## Data flow

```
.rs2 source: npc_tele($rand_coord);
   ↓
bytecode: PUSH_CONST_INT (packed coord) ; OP 2541 (NPC_TELE)
   ↓
ScriptVM.dispatch[OpNpcTele] = handleNpcTele
   ↓
handleNpcTele(s):
   1. requireActiveNpc("NPC_TELE")     → guards no-active-NPC bytecode
   2. coord := s.PopInt()               → consumes the packed-coord push
   3. checkCoord(coord, "NPC_TELE")     → CoordValid: validates [0, 2147483647]
                                         → unpacks (level, x, z)
   4. s.ActiveNpc.Teleport(x, z, level) → delegates to interface impl
   ↓ (interface bridge)
(*Npc).Teleport(x, z, level int):
   5. snapshot prev coords
   6. assign new coords
   7. refreshNpcZone(n.server, n, prevX, prevZ, prevLevel)
       7a. same-zone short-circuit (zone_refresh.go:39) — if (prevX>>3,
           prevZ>>3, prevLevel) == (n.x>>3, n.z>>3, n.level), return
           without re-subscribing
       7b. otherwise: prevZone.LeaveNpc(...) + newZone.EnterNpc(...)
   8. n.tele = true (NPC mask flag — picked up next NPC info encode)
   ↓ (next-tick NPC info encode)
NPC info packet emits NpcMaskTele flag → Java client renders teleport,
no walk-anim interpolation
```

## Validation & error paths

- **No active NPC** (script reached NPC_TELE without binding): handler returns error `"NPC_TELE: no active npc"`. Logged via existing handler-error → log-warn path. Script transitions to `Aborted`.
- **Coord out of range** (negative or > `2147483647`): handler returns error formatted as `"NPC_TELE: coord out of range (%d)"` with the offending value substituted by `checkCoord` (handlers_npc.go:8). Same abort path.
- **All other inputs** (any valid packed coord, any active NPC): succeed silently. The 5 TS divergences (level clamp, unallocated zone rejection) are NOT enforced in NAI-34 (per Out-of-scope).

## Testing

### Layer 1 — handler unit tests (`pkg/script/handlers_npc_test.go`)

```
TestNpcTele_PopsCoordValidatesAndDelegates
  — happy path: bytecode pushes coordgrid.PackCoord(2, 3200, 3200);
    OpNpcTele dispatches; verify mockActiveNpc.teleportCalls ==
    [{x: 3200, z: 3200, level: 2}]

TestNpcTele_NoActiveNpcErrors
  — s.ActiveNpc = nil; expect err containing "NPC_TELE: no active npc"

TestNpcTele_InvalidCoordErrors
  — push -1 (invalid coord); expect err containing
    "NPC_TELE: coord out of range (-1)"

TestNpcTele_PopOrderIsSinglePopInt
  — push sentinel value; verify exactly 1 popInt is consumed
    (stack depth before-after delta = -1)
```

`mockActiveNpc` already exists in `handlers_npc_test.go` — extend with a `teleportCalls []mockTeleport` recorder field per the established `mockEnqueue` pattern. **Plan-author MUST grep the actual mock struct for field names before referencing** (per `mock_recorder_field_naming_check.md` memory).

### Layer 2 — world-side `Npc.Teleport` direct tests (`modules/world/npc_script_test.go`)

```
TestNpcTeleport_SetsFieldsAndTeleFlag
  — n at (5000, 5000, 0); n.Teleport(3200, 3200, 1)
  — assert n.x==3200, n.z==3200, n.level==1, n.tele==true

TestNpcTeleport_CrossZoneRefreshSubscription
  — mirrors player_script_test.go:548 TestPlayerTeleportCrossZoneRefreshSubscription
  — n in zone A; n.Teleport into zone B
  — assert zone A NpcsCount() drops by 1, zone B NpcsCount() rises by 1

TestNpcTeleport_SameZoneNoRefresh
  — mirrors player_script_test.go:588 TestPlayerTeleportSameZoneNoRefresh
  — n at (5000, 5000, 0); n.Teleport(5001, 5001, 0) (same 8x8 zone)
  — assert zone NpcsCount() unchanged, n.zoneListElement pointer unchanged

TestNpcTeleport_NilServerNoOp
  — n.server == nil (test fixture); n.Teleport(...) does not panic
    (refreshNpcZone has the nil-guard at zone_refresh.go:34)
```

### Layer 3 — refactor regression coverage

- `modules/world/npc_ai_test.go:31 TestTeleportHomeAfterStuck` already exists, exercises wanderMode home-tele. **Verify still green post-refactor** (no test changes needed). Add a `n.tele == true` assertion if not already pinned.
- `modules/world/npc_interaction_test.go:1403 TestNpcStuckTeleportRefreshSubscription` already exists. **Verify still green post-refactor.**
- Patrol tests at `npc_interaction_test.go:817-849` exercise mode selection only, not the waypoint-tele branch. No change needed.

### Smoke gate (closes NAI-33 spec item 4)

User-launched server (per `smoke_test_server_handoff.md`):

1. `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml`
2. Java client login.
3. Walk to a fishing-spot zone (e.g., shrimp/anchovies at Lumbridge Swamp) where fishing NPCs run the `move_fishing_spot` ai_timer.
4. Wait for the ai_timer to fire (~280-530 tick period).

**Pass criterion (binary):**

- ✅ PASS: fishing NPC visibly relocates within `fishing_movement_enum` coord set when ai_timer fires; server log shows no `OpNpcTele` error.
- ❌ FAIL: NPC stays put OR server log shows error containing "NPC_TELE" or "opcode 2541".

**Smoke FAIL escalation:** if smoke fails despite all unit/integration tests green, suspect: (a) the script is calling NPC_TELE through an indirect dispatch path the refactor missed; (b) the `n.tele` flag isn't being picked up by the NPC info encode (separate stub-not-completed bug); (c) `n.server` is nil at the script-handler bridge point because of an unrealized init-order assumption.

## Deviations introduced

| ID | Description | Rationale | Closure plan |
|---|---|---|---|
| NAI-34-D1 | `Npc.Teleport` does not clamp `level` to [0,3] (TS PathingEntity.ts:269) | Mirrors established `Player.Teleport` shape (player_script.go:226) which also lacks the clamp; preserves existing inline-site behavior | Future `pathing-entity-teleport-parity` sub-spec aligns both Player + Npc with TS |
| NAI-34-D2 | `Npc.Teleport` does not reject teleports to unallocated zones (TS PathingEntity.ts:271 `!isZoneAllocated` check) | Same as D1 | Same as D1 |
| NAI-34-D3 | `Npc.Teleport` does not call `focus()` to orient toward the teleport vector (TS PathingEntity.ts:286) | Same as D1; goscape has no NPC `focus` plumbing yet | Same as D1 |
| NAI-34-D4 | `Npc.Teleport` does not adjust `lastStepX = x-1; lastStepZ = z` (TS PathingEntity.ts:289-290) | Same as D1; goscape NPCs have no `lastStepX/Z` field yet | Same as D1 |
| NAI-34-D5 | `Npc.Teleport` does not branch on `previousLevel != level` (TS PathingEntity.ts:292+) | Same as D1; off-screen handling not yet ported | Same as D1 |

## `nai_followups.md` updates

**Add to "From NAI-34" section (new):**

1. **`pathing-entity-teleport-parity` sub-spec** — closes NAI-34-D1..D5 + the analogous 5 `Player.Teleport` divergences in one sitting. Estimated ~80 LOC + tests; medium sub-spec.
2. **PatrolMode level discard** — `npc_interaction.go:126` hardcodes `level = 0`; TS Npc.ts:729 uses `dest.level`. Pre-existing divergence surfaced (NOT introduced) by NAI-34's refactor read-through. Estimated 1-line fix + 1 test. Could co-ship with the parity sub-spec.
3. **NPC_WALK opcode 2542** — sibling of NPC_TELE in TS NpcOps.ts:451-455 (`checkedHandler(ActiveNpc) + CoordValid + queueWaypoint(x, z)`). Same shape as NPC_TELE, calls `n.queueWaypoint(x, z)` instead of `n.Teleport`. Tiny sub-spec (~20 LOC).

**Remove from "From NAI-33" section:**

- `NPC_TELE 2541` (closed by NAI-34).

## Plan-author pre-flight checklist

Verify each claim against current HEAD before authoring the plan doc (per `controller_preflight.md`):

1. ☐ `OpNpcTele = 2541` declared at `pkg/script/opcode.go:278`.
2. ☐ `OpNpcTele` stringified at `pkg/script/opcode.go:955`.
3. ☐ `OpNpcTele` NOT in dispatch table at `pkg/script/handlers.go` (currently around lines 325-358).
4. ☐ `checkCoord(v, op)` exists at `pkg/script/handlers_npc.go:8`.
5. ☐ `requireActiveNpc(s, op)` exists at `pkg/script/handlers_npc.go:75`.
6. ☐ `unpackCoord(v) (level, x, z int)` exists at `pkg/script/handlers_player.go:18`.
7. ☐ `ActiveNpc` interface at `pkg/script/active.go` ends around line 470 with `SetHuntMode`.
8. ☐ `Player.Teleport(x, z, level int)` at `modules/world/player_script.go:226` is the 5-line shape this spec mirrors.
9. ☐ `(n *Npc).server *Server` field at `modules/world/npc.go:67` (back-reference).
10. ☐ Inline NPC teleport sites at `modules/world/npc_interaction.go:97` (wanderMode) and `:126` (patrolMode), 4-line pattern each.
11. ☐ `refreshNpcZone(s, n, prevX, prevZ, prevLevel)` at `modules/world/zone_refresh.go:33`, with same-zone short-circuit at `:39`.
12. ☐ `(n *Npc).tele bool` field at `modules/world/npc.go:63`.
13. ☐ `mockActiveNpc` struct exists in `pkg/script/handlers_npc_test.go` — grep for field names before adding `teleportCalls`.
14. ☐ `TestTeleportHomeAfterStuck` at `modules/world/npc_ai_test.go:31` exercises the wanderMode site that this spec refactors.
15. ☐ `TestNpcStuckTeleportRefreshSubscription` at `modules/world/npc_interaction_test.go:1403` exercises the wanderMode site too.
16. ☐ `coordgrid.UnpackCoord` returns a struct with `Level int` field at `pkg/coordgrid/coordgrid.go:145` (used by patrolMode site).
17. ☐ `nai_followups.md` "From NAI-33" section currently contains an `NPC_TELE 2541` entry to remove.

## Cadence (per `compressed_cadence.md` 15-100 LOC bucket)

- Spec doc + plan doc as separate artifacts (this doc + `docs/superpowers/plans/2026-04-26-runescript-nai-34.md`).
- Subagent-driven execution (per `execution_mode_default.md`); fresh implementer per task.
- **Single combined review at the end** (after Task 4); no per-task review pairs.
- Optional polish commit if the code-quality reviewer flags minor issues.

## Commit plan

| # | Commit | Type | Files |
|---|---|---|---|
| 1 | `docs(spec): NAI-34 brainstorm — npc_tele handler + Npc.Teleport extraction` | spec | this doc |
| 2 | `docs(plan): NAI-34 — npc_tele handler + Npc.Teleport extraction` | plan | the plan doc |
| 3 | `feat(world): NAI-34 Task 1 — Npc.Teleport extraction (mirrors Player.Teleport)` | impl | `modules/world/npc_script.go` + tests in `npc_script_test.go` |
| 4 | `refactor(world): NAI-34 Task 2 — wanderMode + patrolMode call Npc.Teleport` | refactor | `modules/world/npc_interaction.go` + `zone_refresh.go` doc-fix |
| 5 | `feat(script): NAI-34 Task 3 — ActiveNpc.Teleport interface + handleNpcTele` | impl | `pkg/script/active.go` + `handlers_npc.go` + tests |
| 6 | `feat(script): NAI-34 Task 4 — register handleNpcTele in dispatch` | impl | `pkg/script/handlers.go` |
| 7 (optional) | `polish(script,world): NAI-34 final-review polish` | polish | per code-quality reviewer findings |
| 8 | `chore(script,world): NAI-34 closed — npc_tele handler + Npc.Teleport extraction` | close | `nai_followups.md` updates summarized in commit body; `Closes memory:` trailer per `close_commit_memory_trailer.md` |

## Close criteria

NAI-34 closes when:

1. ✅ All Layer 1 + Layer 2 + Layer 3 tests pass via `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`.
2. ✅ `go vet ./...` clean.
3. ✅ Combined code-review pass identifies no critical issues.
4. ✅ Smoke gate observed: fishing NPC visibly relocates after `ai_timer` fires; server log silent on `OpNpcTele`.
5. ✅ `nai_followups.md` updated: 3 follow-ups added under "From NAI-34"; `NPC_TELE 2541` removed from "From NAI-33".
6. ✅ Close commit `chore(script,world): NAI-34 closed — ...` lands with `Closes memory:` trailer.

After NAI-34 closes, the visible chain `move_fishing_spot → check_fishing_spot_empty (NAI-33) → npc_tele (NAI-34) → npc_settimer` is end-to-end functional for fishing-spot relocation.
