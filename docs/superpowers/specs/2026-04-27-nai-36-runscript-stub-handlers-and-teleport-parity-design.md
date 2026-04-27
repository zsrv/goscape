# NAI-36 — runscript stub handlers (NPC_WALK, NPC_GETMODE, NPC_SETMODE, SPOTANIM_MAP, MAP_BLOCKED) + PathingEntity.teleport partial parity

## Motivation

Five script-VM opcodes are declared in `pkg/script/opcode.go` but have no handler registered in `pkg/script/handlers.go` at HEAD `c9e777d` (NAI-35 close). All surfaced as `runner.go:71` runtime errors during the post-NAI-35 smoke run. Classic `protocol_stub_not_completed.md` shape — declared opcodes without dispatch wiring, tests pass against absent registrations because no test exercises the script-VM dispatch path for those opcode numbers.

Smoke-surfaced stubs:

- `NPC_WALK` (opcode 2544, declared at opcode.go:281) — sibling of NPC_TELE (NAI-34); dispatches `n.queueWaypoint(x, z)` with level discarded TS-faithfully (TS NpcOps.ts:451-455 uses `coord.x, coord.z` only, drops `coord.level`).
- `NPC_GETMODE` (opcode 2522, declared at opcode.go:259) — single-field push of `n.targetOp` (TS NpcOps.ts:473-475).
- `NPC_SETMODE` (opcode 2535, declared at opcode.go:272) — 3-branch dispatch (TS NpcOps.ts:188-249): clear-target modes (NONE/WANDER/PATROL), NULL→resetDefaults, target-binding modes (OPNPC1+/OPOBJ1+/OPLOC1+/OPPLAYER1+) routing through `n.SetInteraction(SCRIPT, target, mode)`.
- `SPOTANIM_MAP` (opcode 1020, declared at opcode.go:94) — pops 4 ints (spotanim, coord, height, delay), validates, delegates to `World.AnimMap(level, x, z, spotanim, height, delay)` (TS ServerOps.ts:84-90).
- `MAP_BLOCKED` (opcode 1007, declared at opcode.go:81) — F2P-world short-circuit (push 1 if not-F2P-tile on F2P world) + `IsMapBlocked(level, x, z)` push (TS ServerOps.ts:129-138).

Plus pre-existing divergences surfaced by NAI-34 review:

- **PatrolMode level discard** at `modules/world/npc_interaction.go:121` — `n.Teleport(dest.X, dest.Z, 0)` should be `n.Teleport(dest.X, dest.Z, dest.Level)` per TS Npc.ts:729. Pre-existing divergence (NOT introduced by NAI-34); pre-tagged in `nai_followups.md` as "could co-ship with the parity sub-spec."
- **NAI-34-D1/D2 closure** for both `Npc.Teleport` and `Player.Teleport` — level clamp to [0,3] and unallocated-zone rejection per TS PathingEntity.teleport (PathingEntity.ts:267-298). Plus `Player.Teleport` body-order alignment (refresh-then-flag) and level-change INSTANT/jump branch (D5-Player). NAI-34-D3 (focus orientation), NAI-34-D4 (lastStepX/Z adjust), and NAI-34-D5-NPC (jump field for NPC) remain DEFERRED — closure requires NPC-side infrastructure that has no current consumers (`dead_api_polish.md` foot-gun).

Pre-NAI-36 behavior: 5 opcodes abort with `Aborted` execution state and log `script %q: no handler for %s (opcode %d) at pc=%d` (runner.go:71); PatrolMode silently teleports patrol NPCs to level 0 ignoring `dest.level`; level/zone divergences cause subtle desync vs TS for any teleport that hits clamp/reject conditions.

Post-NAI-36 behavior: all 5 opcodes execute TS-faithfully; PatrolMode preserves `dest.level`; Player.Teleport + Npc.Teleport gain TS-matching level-clamp + unallocated-zone reject (and Player.Teleport additionally gains the level-change INSTANT/jump branch). Three NAI-34 deviations close (D1, D2 for both entities; D5 for Player). Three remain residual (D3, D4, D5-NPC) tracked for a future "pathing-entity-focus-and-step-tracking" sub-spec.

## Tech stack

- **Go 1.26+** (per `go_version.md`; use modern Go syntax via the `use-modern-go` skill).
- TS source: `Engine-TS` only per `ts_source_canonical_path.md`.
  - `src/engine/script/handlers/NpcOps.ts:451-455` (NPC_WALK)
  - `src/engine/script/handlers/NpcOps.ts:473-475` (NPC_GETMODE)
  - `src/engine/script/handlers/NpcOps.ts:188-249` (NPC_SETMODE)
  - `src/engine/script/handlers/ServerOps.ts:84-90` (SPOTANIM_MAP)
  - `src/engine/script/handlers/ServerOps.ts:129-138` (MAP_BLOCKED)
  - `src/engine/entity/PathingEntity.ts:267-298` (PathingEntity.teleport canonical body)
  - `src/engine/entity/Npc.ts:729` (PatrolMode `dest.level` reference)
  - `src/engine/entity/Npc.ts:377-379` (clearPatrol)
- Existing infrastructure (verified at HEAD c9e777d):
  - `(n *Npc) queueWaypoint(x, z int)` at `modules/world/npc_ai.go:84` — to be exported as `QueueWaypoint`.
  - `(n *Npc) Teleport(x, z, level int)` at `modules/world/npc_script.go:109` (NAI-34 extraction).
  - `(p *Player) Teleport(x, z, level int)` at `modules/world/player_script.go:226`.
  - `n.targetOp` field at `modules/world/npc.go:82`; full `NPCMode*` constants at `pkg/objtype/npctype.go:43+`.
  - `n.resetDefaults()`, `n.clearInteraction()`, `n.SetInteraction(...)` from NAI-13.
  - `(n *Npc) defaultMode()` at `modules/world/npc_interaction.go:696`.
  - `PtrActiveNpc2` + `OtherActiveNpc` ScriptState field from NAI-11 (`pkg/script/pointer.go:11`, `state.go:201-203`).
  - `Script.IntOperands []int32` accessed via `s.Script.IntOperands[s.PC]` (pattern at `handlers_number.go:50`).
  - `(s *Server) AnimMap(level, x, zc, spotanim, height, delay int)` at `modules/world/world_zone.go:76`.
  - `s.World.IsMapBlocked(level, x, z)`, `s.World.IsFreeToPlay(x, z)`, `s.World.MapMembers()` from NAI-35-T6 (`pkg/script/state.go:64-74`).
  - `(m *FlagMap) IsZoneAllocated(absX, absZ, level)` at `pkg/pathfinder/collision/flagmap.go:142`.
  - `MoveSpeedInstant` at `modules/world/movement_consts.go:11`; `p.moveSpeed`, `p.jump`, `p.lastStepX/Z` fields on Player.
  - `(p *Player) FaceCoord(x, z int)` and `(n *Npc) FaceCoord(x, z int)` mask-emit methods.
  - `requireActiveNpc(s, opName)` at `handlers_npc.go:72`, `checkCoord(coord, opName)` at `handlers_npc.go:8`, `checkNotNull(...)` helpers (per `plan_grep_helper_patterns.md` — reuse, don't reinvent).
- Validators are inline `checkXxx(value, opName) error` helpers (NOT exported `XxxValid` predicates per goscape convention).
- Spec/plan reference: NAI-34 spec at `docs/superpowers/specs/2026-04-26-nai-34-npc-tele-handler-design.md`, NAI-35 spec at `docs/superpowers/specs/2026-04-27-nai-35-deferred-opcode-stubs-design.md`.

## Scope

**In scope:**

1. **T1 — Foundation seams.**
   - Rename `(n *Npc) queueWaypoint → QueueWaypoint` at `modules/world/npc_ai.go:84`. Update all 7 in-package call sites: `npc_interaction.go:87`, `:118`, `:135`, `:351`; `npc_player_modes.go:167`, `:174`, `:176`. (`enumerate_all_sites.md` — list every site in the plan.)
   - Add `QueueWaypoint(x, z int)` and `TargetOp() int` to the `ActiveNpc` interface in `pkg/script/active.go`.
   - Add `AnimMap(level, x, z, spotanim, height, delay int)` to the `State.World` interface in `pkg/script/state.go`.
   - Wire methods on the script-side ActiveNpc adapter and World adapter in `modules/world/`.

2. **T2 — NPC_WALK (opcode 2544).** `handleNpcWalk` in `pkg/script/handlers_npc.go` mirroring `handleNpcTele` shape: `requireActiveNpc` + `checkCoord` + `s.ActiveNpc.QueueWaypoint(x, z)` (level dropped TS-faithfully). Register `OpNpcWalk: handleNpcWalk` in `pkg/script/handlers.go`.

3. **T3 — NPC_GETMODE (opcode 2522).** `handleNpcGetMode` in `pkg/script/handlers_npc.go`: `requireActiveNpc` + `s.PushInt(s.ActiveNpc.TargetOp())`. Register in `handlers.go`.

4. **T4 — MAP_BLOCKED (opcode 1007).** `handleMapBlocked` in `pkg/script/handlers_server.go` (or `handlers_map.go` extension): `checkCoord` (unpack to level/x/z) + F2P-world gate (`s.World.MapMembers() == 0 && !s.World.IsFreeToPlay(x, z) → push 1; return`) + `if s.World.IsMapBlocked(level, x, z) { push 1 } else { push 0 }`. Register in `handlers.go`.

5. **T5 — SPOTANIM_MAP (opcode 1020).** `checkSpotAnimType(id, opName) error` helper — validates spotanim id against `s.Configs.SpotAnimType(id) != nil` (verify config-view accessor at plan-write). `handleSpotAnimMap`: pops 4 ints `(spotanim, coord, height, delay)`, validates coord + spotanim, delegates `s.World.AnimMap(level, x, z, spotanimID, height, delay)`. Register in `handlers.go`.

6. **T6 — NPC_SETMODE (opcode 2535).**
   - `checkNpcMode(mode, opName) error` helper — validates against full `NPCMode*` enum at `pkg/objtype/npctype.go:43+`.
   - Add `clearPatrol()` method to `Npc`: single-field write `n.nextPatrolTick = -1` (mirrors TS Npc.ts:377-379).
   - `handleNpcSetMode` with 3-branch dispatch (TS NpcOps.ts:188-249):
     - `mode == NPCModeNone || NPCModeWander || NPCModePatrol`: `n.clearInteraction()`; `n.targetOp = mode`; if PATROL also `n.clearPatrol()`; return.
     - `mode == NPCModeNull`: `n.resetDefaults()`; return.
     - else: `n.targetOp = mode`; resolve target via mode-range:
       - `mode >= NPCModeOpNpc1`: `intOperand = s.Script.IntOperands[s.PC]; target = (intOperand == 0 ? OtherActiveNpc : ActiveNpc)`.
       - `mode >= NPCModeOpObj1`: `target = ActiveObj`.
       - `mode >= NPCModeOpLoc1`: `target = ActiveLoc`.
       - else: `target = ActivePlayer (Self)`.
       - if `target == nil`: `n.resetDefaults()`.
       - else: `n.SetInteraction(InteractionScript, target, mode)`.
   - Add `SetInteractionScript(target any, mode int)` method on the `ActiveNpc` interface in `pkg/script/active.go`, plus the matching adapter in `modules/world/` that calls into `n.SetInteraction(...)` with the existing world-side signature. The plan-author resolves the world-side adapter argument shape against the existing `n.SetInteraction(...)` callers at `modules/world/handler_oploc.go:170,287` and `interaction.go`; if the existing signature can't accept `target any` cleanly, track as deviation NAI-36-D3 and adjust the adapter to wrap an internal helper.
   - Register in `handlers.go`.

7. **T7 — Partial PathingEntity.teleport parity sweep.**
   - **Player.Teleport** (player_script.go:226-233) — close 4 divergences:
     - **D1 (level clamp):** clamp `level` to [0, 3] before assignment. TS PathingEntity.ts:269.
     - **D2 (unallocated-zone reject):** if `!s.World.IsZoneAllocated(x, z, level)` (or equivalent accessor), early-return with no state change. TS PathingEntity.ts:271.
     - **Order alignment:** swap `tele = true` and `refreshPlayerZone(...)` so refresh runs FIRST, then tele=true. TS PathingEntity.ts:290-293.
     - **D5 (level-change INSTANT/jump):** `if previousLevel != level { p.moveSpeed = MoveSpeedInstant; p.jump = true }`. TS PathingEntity.ts:294-297.
   - **Npc.Teleport** (npc_script.go:109-114) — close 2 divergences:
     - **D1 (level clamp):** same as Player.
     - **D2 (unallocated-zone reject):** same as Player.
     - **D3 (focus), D4 (lastStepX/Z), D5-NPC (jump):** REMAIN RESIDUAL — see Out of Scope.
   - **PatrolMode fix** at npc_interaction.go:121: `n.Teleport(dest.X, dest.Z, 0)` → `n.Teleport(dest.X, dest.Z, dest.Level)`.
   - **Update DEVIATION comments** at `modules/world/npc_script.go:95-97` and `pkg/script/active.go:501` to reflect partial-closure framing (D1+D2 closed; D3+D4+D5-NPC residual).

8. **T8 — Close polish.**
   - Update `nai_followups.md` retired-deviations section: NAI-34-D1, NAI-34-D2 (both entities), NAI-34-D5 (Player only) closed; NAI-34-D3, NAI-34-D4, NAI-34-D5-NPC remain residual under "future pathing-entity-focus-and-step-tracking sub-spec."
   - Add `Closes memory:` trailer to close commit per `close_commit_memory_trailer.md`.
   - Run `rg "NAI-34-D" pkg/ modules/ cmd/` per `retire_deviation_grep_all_comments.md` and update every comment site (not just production touch points) for partial-closure framing.
   - Smoke-gate handoff to user (per `smoke_test_server_handoff.md`).

**Out of scope (with rationale):**

- **NAI-34-D3 (focus orientation in Npc.Teleport):** TS calls `this.focus(CoordGrid.fine(moveX, this.width), CoordGrid.fine(moveZ, this.length), false)` (PathingEntity.ts:286). Goscape has both `(n *Npc) focus(fx, fz int, instant bool)` (npc_interaction.go:686) AND `(n *Npc) FaceCoord(x, z int)` (npc_masks.go:120) — choosing between them and porting the fine-coord conversion is its own design question. Tracked for a future "pathing-entity-focus-and-step-tracking" sub-spec.
- **NAI-34-D4 (lastStepX/Z adjust in Npc.Teleport):** TS sets `this.lastStepX = this.x - 1; this.lastStepZ = this.z` (PathingEntity.ts:289-290). Npc has no `lastStepX/Z` fields. Adding fields with zero current consumers is a `dead_api_polish.md` foot-gun. Lands when NPC-side stride/path-tracking gets a consumer.
- **NAI-34-D5-NPC (jump field for NPC):** Player has `jump`; Npc does not. NPC client-side has no jump animation consumer. Adding Npc.jump is dead-API. Player branch (which has `jump`) closes cleanly in T7.
- **NPC_WALKTRIGGER (opcode 2545):** separate stub, not in NAI-34 follow-up scope. Leaves no smoke-driven blocker; defer to a future audit.
- **PLAYER_FINDALL / PLAYER_FINDALLZONE family:** NAI-35-D2 deferral. PlayerIterator currently HuntAll-only; no NAI-36 expansion.
- **Members/F2P-world full configuration:** NAI-35-D3 already resolved via `MapMembers() != 0`. T4 reuses that pattern; no new accessor.
- **`SpotAnimType` config-port (if not already present):** if `s.Configs.SpotAnimType(...)` is missing at plan-write, T5 falls back to range-validation `if id < 0 { error }` and tracks as NAI-36-D4. Verify at plan-write.
- **NAI-35-T3-D1 (op[1] operability gate audit):** independent residual; revisit when HUNTALL smoke surfaces a real-content miss.

## Architecture

NAI-36 is a multi-task feature-port + parity-sweep sub-spec, full cadence per `runescript_cadence.md` (size > 100 LOC, beyond `compressed_cadence.md` threshold). Cross-package surface: `pkg/script` ↔ `modules/world`. Three new ActiveNpc methods (`QueueWaypoint`, `TargetOp`, `SetInteractionScript`) + one new World method (`AnimMap`). No new packages, no mandatory new files (handlers extend existing `handlers_npc.go`/`handlers_server.go` or `handlers_map.go`).

### Two-theme structure

- **Theme A — Smoke-driven stub-not-completed ports (T1-T6):** narrative is "close the dispatch gaps surfaced by post-NAI-35 smoke." All `protocol_stub_not_completed.md` instances. Test infrastructure: handler unit tests at `pkg/script` + ActiveNpc/World interface seams + adapter wiring at `modules/world`.
- **Theme B — Pre-existing parity sweep (T7):** narrative is "close NAI-34-D1+D2 (both entities), Player.Teleport order divergence, and Player.Teleport D5 in one sweep." All deviation-thread closures. Test infrastructure: entity-level body-order pinning at `modules/world/*_script_test.go`.

The two themes share one file (`modules/world/npc_interaction.go`) at line 121:
- T1 renames `n.queueWaypoint(...)` callers (lines 87, 118, 135, 351) to `n.QueueWaypoint(...)`.
- T7 changes line 121 from `n.Teleport(dest.X, dest.Z, 0)` to `n.Teleport(dest.X, dest.Z, dest.Level)`.

Sequencing: T1 first → cleanly land all renames → T2-T6 add new dispatch → T7 modifies Teleport call sites + body. No file conflicts within the task ordering.

### File layout

| Path | Change | Net production lines |
|---|---|---|
| `pkg/script/handlers.go` | + 5 dispatch entries (`OpNpcWalk`, `OpNpcGetMode`, `OpNpcSetMode`, `OpSpotAnimMap`, `OpMapBlocked`) | +5 |
| `pkg/script/handlers_npc.go` | + handleNpcWalk, handleNpcGetMode, handleNpcSetMode | +90 |
| `pkg/script/handlers_server.go` (or extend `handlers_map.go`) | + handleSpotAnimMap, handleMapBlocked | +35 |
| `pkg/script/active.go` | + ActiveNpc.QueueWaypoint, ActiveNpc.TargetOp, ActiveNpc.SetInteractionScript; + State.World.AnimMap | +30 |
| `pkg/script/checks.go` (or inline at handlers_*) | + checkNpcMode, + checkSpotAnimType helpers | +20 |
| `modules/world/npc_ai.go` | rename queueWaypoint → QueueWaypoint (definition) | 0 |
| `modules/world/npc_interaction.go` | rename callers (lines 87/118/135/351); fix PatrolMode level (line 121); +clearPatrol method | +5 |
| `modules/world/npc_player_modes.go` | rename callers (lines 167/174/176) | 0 |
| `modules/world/npc_script.go` | align Npc.Teleport body (D1+D2 close); update DEVIATION comment for partial closure | +15 |
| `modules/world/player_script.go` | align Player.Teleport body (D1+D2+order+D5 close) | +15 |
| `modules/world/<adapter file>` | + QueueWaypoint, TargetOp, target-binding methods on the script-side ActiveNpc adapter | +25 |
| **Total prod** | | **~240** |

### Test layout (~34 new tests)

| Path | New test count | Coverage |
|---|---|---|
| `pkg/script/handlers_npc_test.go` | +12 | NPC_WALK ×3 (pop+validate+delegate, level-discarded TS-asymmetry pin, no-active-npc), NPC_GETMODE ×2 (pop+delegate, no-active-npc), NPC_SETMODE ×7 (one per dispatch branch — see Test strategy) |
| `pkg/script/handlers_server_test.go` (or `_map_test.go`) | +8 | MAP_BLOCKED ×4 (members-world clear, members-world blocked, F2P-world non-F2P-tile, F2P-world F2P-tile), SPOTANIM_MAP ×4 (pop+validate+delegate, invalid coord, invalid spotanim, World.AnimMap delegation pin) |
| `modules/world/npc_script_test.go` | +6 | Npc.Teleport: level-clamp ×2 (level=-1, level=4), zone-reject ×1, order pin ×1 (already TS-correct, regression guard), PatrolMode level fix ×2 (level=0 path, level=1 path) |
| `modules/world/player_script_test.go` | +8 | Player.Teleport: level-clamp ×2, zone-reject ×1, order pin ×1, level-change-INSTANT ×2, jump-set ×1, regression existing callers ×1 |

## Test strategy

(per `plan_test_coverage_crosscheck.md` — every test below maps to a specific task code block in the plan)

- **Layer 1 (handler dispatch tests, `pkg/script/handlers_*_test.go`):** mock ActiveNpc/World fixtures, verify pop order, validation error paths, delegation invocation. Pattern from NAI-34's `TestNpcTele_PopsCoordValidatesAndDelegates`.
  - **NPC_WALK TS-asymmetry pin** (per `ts_asymmetry_dual_pin.md`): pin both presence (`QueueWaypoint(x, z)` called with the unpacked x/z) AND conspicuous absence (level discarded — assert no Teleport-style 3-arg call, no separate level path). The presence test escalates if a future TS upstream fix changes WALK to honor level.
  - **NPC_SETMODE branch coverage gate** — ALL 5 dispatch branches exercised:
    1. `mode == NPCModeNone` → `clearInteraction` called, `targetOp = NPCModeNone`, `clearPatrol` NOT called.
    2. `mode == NPCModeWander` → `clearInteraction` called, `targetOp = NPCModeWander`, `clearPatrol` NOT called.
    3. `mode == NPCModePatrol` → `clearInteraction` called, `targetOp = NPCModePatrol`, `clearPatrol` IS called.
    4. `mode == NPCModeNull` → `resetDefaults` called.
    5. `mode == NPCModeOpPlayer1` (target ActivePlayer Self) → `targetOp` set, `SetInteraction` called with player.
    6. `mode == NPCModeOpNpc1` × `intOperand == 0` → target = OtherActiveNpc (PtrActiveNpc2 path).
    7. `mode == NPCModeOpNpc1` × `intOperand != 0` → target = ActiveNpc (PtrActiveNpc path).
    8. `mode == NPCModeOpObj1` → target = ActiveObj.
    9. `mode == NPCModeOpLoc1` → target = ActiveLoc.
    10. `mode == NPCModeOpPlayer1` × no Self → `resetDefaults` called (no-target fallthrough).
- **Layer 2 (entity-method tests, `modules/world/*_script_test.go`):**
  - **Player.Teleport order pin** (per `ts_asymmetry_dual_pin.md`): use a recording fixture that captures `tele=true` write-time and `refreshPlayerZone` call-time; assert refresh-time precedes flag-write-time.
  - **Player.Teleport level-clamp**: assert level=-1 clamps to 0, level=4 clamps to 3.
  - **Player.Teleport zone-reject**: stub `IsZoneAllocated → false`; assert no state change (x/z/level/tele/jump/moveSpeed unchanged from pre-call values).
  - **Player.Teleport level-change INSTANT/jump**: `previousLevel=0, level=1` → assert `moveSpeed == MoveSpeedInstant && jump == true`. `previousLevel=0, level=0` → assert `moveSpeed`/`jump` unchanged.
  - **Npc.Teleport level-clamp + zone-reject**: same shape as Player.
  - **PatrolMode level fix**: drive `patrolMode(s)` against a multi-level PatrolCoord (e.g. `dest.Level=1`) past the `nextPatrolTick` threshold; pin that the resulting `n.Teleport` call observes `level=1` (not 0). Pattern from `npc_player_modes_test.go` fixture style.
- **Test fixture discipline** (per `plan_runnable_test_fixtures.md`): every direct-call ScriptState fixture allocates `IntStack: make([]int, StackCapacity), StringStack: make([]string, StackCapacity)`; for tests requiring `IntOperand`, wire `Script.IntOperands` and `s.PC` to a known value before dispatch. Test fixtures are mentally compiled against current Npc/Player struct shape before plan-author dispatch.

## Expected deviations

(per `audit_full_method_against_ts.md` — every entry HEAD-verified at spec-write)

**New deviations potentially introduced by NAI-36:**

- **NAI-36-D1 (probable, finalize at plan-write):** `intOperand` access pattern in NPC_SETMODE. TS reads `state.intOperand` directly; goscape reads `s.Script.IntOperands[s.PC]`. If a `s.CurrentIntOperand()` helper emerges in the plan, track as deviation; if direct-array-access matches existing handlers (e.g. `handlers_number.go:50` pattern), no deviation.
- **NAI-36-D2 (conditional, T5):** if `s.Configs.SpotAnimType(...)` accessor doesn't exist at plan-write, T5 falls back to range-validation `if id < 0 { error }` and tracks divergence vs TS `SpotAnimTypeValid`. Verify at plan-write.
- **NAI-36-D3 (conditional, T6):** if the world-side `n.SetInteraction(...)` signature can't accept the `target any` adapter cleanly, track shape divergence vs TS `setInteraction(Interaction.SCRIPT, target, mode)`. The plan-author resolves at T6 task entry.

**T7 closes (existing deviations retired, no new D# assignment needed):**

- NAI-34-D1 (level clamp) — closed for both `Npc.Teleport` and `Player.Teleport`.
- NAI-34-D2 (unallocated-zone reject) — closed for both entities.
- Player.Teleport order divergence (`tele=true` before `refreshPlayerZone`) — pre-existing, never given a D#; retired from the `nai_followups.md` parity section in T8.
- Player.Teleport level-change INSTANT/jump branch — pre-existing; retired in T8.

**Existing deviations remain residual (carryover, original D# preserved per `retire_deviation_grep_all_comments.md`):**

- **NAI-34-D3** — `Npc.Teleport` doesn't call `focus()`. Reason: NPC-side fine-coord conversion + instant-flag semantics need design; no current consumer.
- **NAI-34-D4** — `Npc.Teleport` doesn't adjust `lastStepX/Z`. Reason: Npc has no `lastStepX/Z` fields; adding without consumer is `dead_api_polish.md` foot-gun.
- **NAI-34-D5** (NPC half) — `Npc.Teleport` doesn't set `jump = true` on level change. Reason: Npc has no `jump` field; no NPC client-side render consumer. (Player half closes in T7.)

Closure plan for the three carryovers: future "pathing-entity-focus-and-step-tracking" sub-spec, conditional on a real consumer materializing for NPC stride-tracking / focus / jump.

**Net deviation tally (running):**

- Pre-NAI-36 (post-NAI-35): 16 active deviations.
- T7 closes: NAI-34-D1, NAI-34-D2 → -2.
- Carryovers retained: NAI-34-D3, D4, D5 (NPC half).
- Possible additions: NAI-36-D1 (+0 or +1), NAI-36-D2 (+0 or +1), NAI-36-D3 (+0 or +1).
- Post-NAI-36 expected range: 14 (best case, no new deviations) to 17 (worst case, all three NAI-36-D# materialize).
- T8 finalizes the tally after observing which NAI-36-D# entries actually land.

## Cadence

Standard cadence per `runescript_cadence.md` (~240 prod LOC + ~34 tests; well above `compressed_cadence.md` threshold). Subagent-driven TDD per `execution_mode_default.md`. **Two-stage review** at the T1-T5 / T6-T7 boundary:

- **Stage 1** after T1-T5: foundation seams + 4 simpler dispatch ports (NPC_WALK, NPC_GETMODE, MAP_BLOCKED, SPOTANIM_MAP). Reviewer focus: dispatch correctness, interface seam quality, validator helper hygiene, queueWaypoint rename completeness.
- **Stage 2** after T6-T8: NPC_SETMODE 3-branch dispatch + parity sweep + PatrolMode fix + close. Reviewer focus: branch coverage gate, body-order TS-fidelity, deviation-tracker accuracy (`retire_deviation_grep_all_comments.md`), `Closes memory:` trailer.

## Sequencing rationale

- **T1 first** — lands all interface seams + queueWaypoint→QueueWaypoint rename in one step; T2-T6 depend on the exported methods.
- **T2-T4 are tight** (~50 LOC combined) — good progress checkpoint after T1 lands; can be implemented in parallel by independent subagents (per `superpowers:dispatching-parallel-agents.md`) since they touch disjoint handler functions.
- **T5 SPOTANIM_MAP before T6** — checkSpotAnimType is a useful warmup for the more complex checkNpcMode in T6.
- **T6 NPC_SETMODE** — heaviest dispatch task; isolated from T7's body-order math so each stage's review focuses on one concern.
- **T7 parity sweep last** — touches `npc_interaction.go:121` which T1 just renamed and T6 may have referenced. Ordering avoids merge conflicts in the same hunk and lets T7 inherit a stable Teleport interface from earlier tasks.
- **T8 polish** — wraps memory + tracker updates + smoke-gate handoff.

## Risk + mitigations

- **NPC_SETMODE branch fan-out** (highest risk): 3-branch + 4-target-pointer dispatch has 8+ conjugate paths. **Mitigation**: explicit test-per-branch gate in plan (10 listed branches above), fixture allocation per `plan_runnable_test_fixtures.md`, controller pre-flight verification per `controller_preflight.md` re-grep at HEAD before T6 dispatch.
- **Player.Teleport regression** (medium risk): body-order change AND new clamp/reject branches may affect callers that depend on `tele=true` being set BEFORE refresh, or callers passing out-of-range level/coord. **Mitigation**: pre-flight grep `Player.Teleport` and `p.Teleport` callers at plan-write per `enumerate_all_sites.md`; verify each behaves correctly under refresh-then-flag order + clamp + reject. Track any caller-site changes needed.
- **queueWaypoint rename completeness** (low risk, easy to miss): ~7 in-package call sites + the definition. **Mitigation**: `enumerate_all_sites.md` discipline; plan lists every call site with file:line; post-T1 `rg "queueWaypoint" modules/world/` should show zero hits to lowercase.
- **Stale tracker entries** (medium risk): NAI-34-D1..D5 doc-comments at `pkg/script/active.go:501` and `modules/world/npc_script.go:95-97` reference all 5 deviations together. T7 partially closes (D1+D2 only). **Mitigation**: per `retire_deviation_grep_all_comments.md`, T7 must `rg "NAI-34-D" pkg/ modules/ cmd/` at task close and update each comment site to reflect partial-closure framing — not just production touch points.
- **Smoke-gate timing**: NAI-35 smoke gate items (Lumbridge NPC_PARAM, Al-Kharid HUNTALL, Barbarian Village NPC_HUNTALL, Wizards' Tower MAP_FINDSQUARE) are still pending from NAI-35 close. T8 hands off NAI-36 smokes additively, not as a block. NAI-35 + NAI-36 smokes should be run in one session per `smoke_test_server_handoff.md`.
- **`Closes memory:` trailer discipline** (per `close_commit_memory_trailer.md`): T8 close commit must enumerate the closed `nai_followups.md` items so the partial-closure of NAI-34-D1..D5 is grep-discoverable from `git log` (e.g. "Closes memory: NAI-34 follow-up #2 PatrolMode + #1 NPC_WALK + parity D1/D2/D5-Player").
- **Plan-author premise drift**: spec was authored with all premises HEAD-verified at `c9e777d`. Plan-author MUST re-verify each "Existing infrastructure" line in Tech stack against HEAD before dispatch (per `controller_preflight.md`).

## Smoke handoff (T8)

Per `smoke_test_server_handoff.md`, all server-launched smokes are user-driven. T8 hands off the following list additively to NAI-35's pending smokes:

**NAI-36 smokes (new):**
1. **NPC_WALK** — content script triggering `npc_walk` on a known NPC (audit `LostCityRS/Content/scripts/` for `npc_walk` consumers; pick one with deterministic destination).
2. **NPC_GETMODE / NPC_SETMODE** — patrol clear-then-set path; verify NPC switches mode and emits faceEntity mask correctly.
3. **MAP_BLOCKED** — F2P-world non-F2P tile (push 1 expected) AND members-world tile (collision-driven push reflecting actual blockmap state).
4. **SPOTANIM_MAP** — visual graphic emission at a coord (e.g. Falador square or Wizards' Tower); verify zone-broadcast reach.
5. **PatrolMode multi-level** — patrol NPC with `dest.Level != 0` (e.g. multi-level dungeon patrol or Lumbridge cellar patrol path); verify NPC arrives at the correct level after `nextPatrolTick` fires.
6. **Player.Teleport level-change** — `::tele` cheat across level boundaries (e.g. ground floor → upper floor); verify smooth transition with `moveSpeed=INSTANT + jump=true`.

**Carryover from NAI-35 smoke gate:**
7. Lumbridge NPC_PARAM (NAI-35-T1 verification).
8. Al-Kharid HUNTALL (NAI-35-T4 verification).
9. Barbarian Village NPC_HUNTALL (NAI-35-T3 verification).
10. Wizards' Tower MAP_FINDSQUARE (NAI-35-T6 verification).

Smoke results inform whether any deviation tally needs revision and whether residual NAI-34-D3/D4/D5-NPC need elevated priority for the next sub-spec.
