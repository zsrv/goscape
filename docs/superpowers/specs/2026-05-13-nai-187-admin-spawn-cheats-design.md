# NAI-187 — Admin-block spawn / interface cheats (`locadd` / `npcadd` / `openmain`)

**Date:** 2026-05-13
**Status:** Spec (awaiting plan)
**Tech stack:** Go 1.26+
**TS source:** `LostCityRS/Engine-TS/src/network/game/client/handler/ClientCheatHandler.ts`
**Predecessors:** NAI-184 (cheat infra), NAI-185 (admin-block non-spawn cohort), NAI-186 (super-mod cohort + carryforward rewrite).

## §1 Goal

Port three TS admin-tier (`staffModLevel >= 3`, no `NODE_PRODUCTION` requirement) cheats into goscape's existing admin block in `modules/world/handlers_game.go`:

- `::locadd <name>` — spawn a despawning `Loc` at the player's tile (TS L441-452)
- `::npcadd <name>` — spawn a despawning `Npc` at the player's tile (TS L453-463)
- `::openmain <name>` — open a `ComponentType` as the main modal (TS L464-476)

Closes the admin-tier portion of `DEVIATION-NAI-186-D2-CARRYFORWARD` at `modules/world/handlers_game.go:368-377`.

## §2 Out of scope

| Cheat | TS line | Gate | Why deferred |
|---|---|---|---|
| `reload` | L149-150 | `>=4 + !NP` | Calls `World.reload()` — full cache hot-reload pipeline; goscape has zero equivalent. Genuine infra gap. |
| `rebuild` | L151-153 | `>=4 + !NP` | Calls `World.rebuild()` — script-provider hot-reload; same infra gap. |
| `speed` | L154-167 | `>=4 + !NP` | Trivial code (~10 LOC) but mutates `tickRate`, currently a package-level `const` at `modules/world/tick.go:15`. Touches load-bearing tick-loop infra. Right size for its own one-shot follow-up sub-spec, not a bundling target. |
| `snapshot` | L477-480 | `>=3` | Already ported in NAI-185 (uses `runtime/pprof` analog at `handlers_game.go:563-574`). |

NAI-187 close MUST rewrite `DEVIATION-NAI-186-D2-CARRYFORWARD` to remove the admin cluster and keep the dev cluster (with the corrected `speed = trivial code / infra-blocked status differs per-cheat` framing).

## §3 Pre-flight audit (vs. NAI-186 carryforward framing)

The NAI-186 carryforward described the admin cluster as "blocked on dynamic Loc/Npc spawn + interface routing." Per memory `tracker_entry_framing_can_be_incomplete` and `risk_register_premise_grep`, I re-derived from primary sources at HEAD `5644450`. The framing was stale: every dynamic-spawn / interface-routing primitive already exists.

| Cheat | Required primitives | Status at HEAD |
|---|---|---|
| `locadd` | `s.AddLoc(*entity.Loc, dur)` | ✓ `modules/world/world_zone.go:17` |
|  | `entity.NewLoc(level, x, z, w, l, lc, typ, shape, angle)` | ✓ `pkg/entity/loc.go:23` |
|  | `loc.ShapeCentrepieceStraight` constant | ✓ `pkg/pathfinder/loc/shape.go:16` |
|  | `entity.LifecycleDespawn` | ✓ `pkg/entity/lifecycle.go` |
|  | `LocAngle.WEST == 0` | ✓ literal `0` (validated `[0,3]` per `pkg/script/handlers_player.go:91`) |
|  | `LocType.ByName(name)` | ✗ **gap** — must add (mirrors `VarpTypeConfigs.ByName`) |
| `npcadd` | `s.addNpc(n *Npc, dur int, firstSpawn bool)` | ✓ `modules/world/npc_registry.go:48` |
|  | `world.NewNpc(nid, typeId, x, z, level, *NpcType)` | ✓ reference impl at `modules/world/server_varp.go:174` (`worldVarsView.AddNpcAt`) |
|  | `NpcLifecycleDespawn` | ✓ |
|  | `NpcType.ByName(name)` | ✗ **gap** — must add |
| `openmain` | `p.OpenMain(com int)` | ✓ `modules/world/player_script.go:943` |
|  | `ComponentType.RootLayer` field | ✓ `pkg/objtype/componenttype.go:49` |
|  | `ComponentType.ByName(name)` | ✗ **gap** — must add |

**Sole infrastructure gap:** three `ByName` helpers on `LocTypeConfigs` / `NPCTypeConfigs` / `ComponentTypeConfigs`. Each is a 12-line wrapper over the already-populated `ConfigNames map[string]int` index. Pattern is identical to `(*VarpTypeConfigs).ByName` at `pkg/objtype/varptype.go:120` and `(*ObjTypeConfigs).ByName` at `pkg/objtype/objtype.go:76`.

## §4 Architecture

### §4.1 Layer 1: `pkg/objtype/` — three `ByName` helpers

Add as methods on the existing `*Configs` types. Each mirrors `VarpTypeConfigs.ByName`'s shape exactly:

```go
func (c *LocTypeConfigs) ByName(name string) *LocType {
    if c == nil {
        return nil
    }
    if id, ok := c.ConfigNames[name]; ok {
        if id >= 0 && id < len(c.Configs) {
            return c.Configs[id]
        }
    }
    for _, t := range c.Configs {
        if t != nil && t.DebugName == name {
            return t
        }
    }
    return nil
}
```

Identical bodies for `NPCTypeConfigs.ByName(name) *NpcType` and `ComponentTypeConfigs.ByName(name) *ComponentType`.

**Rationale for the linear-scan fallback:** matches the established pattern. Test fixtures sometimes construct a `*Configs` value without populating `ConfigNames` (e.g. NPC fixtures in `npc_*_test.go` build configs slices directly). The fallback keeps `ByName` usable from those fixtures without forcing every test to wire the name index. (`VarpTypeConfigs.ByName` was reviewed and accepted with this same fallback in NAI-185 — see commit `32dd969`.)

**Files added/touched:**
- `pkg/objtype/loctype.go` — append `ByName` method.
- `pkg/objtype/npctype.go` — append `ByName` method.
- `pkg/objtype/componenttype.go` — append `ByName` method.
- New test files (one each): `pkg/objtype/loctype_byname_test.go`, `npctype_byname_test.go`, `componenttype_byname_test.go` — OR append to existing `_test.go` files; final placement decided at plan time based on which existing file already holds `ByName`-adjacent tests.

### §4.2 Layer 2: `modules/world/handlers_game.go` — three new switch arms

The existing admin switch starts at `handlers_game.go:428` (`if p.staffModLevel >= 3 { switch parts[0] { ... } }`). Add three new `case` arms inside it. No new files in `modules/world/`. Matches NAI-185's pattern (`setvar`, `getvar`, `give`, etc. all live inline).

**Placement order within the admin switch:** alphabetical-by-cmd, matching the existing layout — `locadd` between `give*` block and `minme`; `npcadd` after `minme`; `openmain` after `npcadd`. (Final placement may be adjusted at plan time to keep TS-line-comments monotonic; preserves grep-friendliness.)

## §5 Per-cheat implementation

Arg-parsing convention follows NAI-185 (`setvar` at `handlers_game.go:575`): `if args == "" { return nil }`, then `strings.SplitN(args, " ", 2)` if multi-arg, or `strings.Fields(args)[0]` for single-arg-name. All three NAI-187 cheats are single-arg-name; `strings.Fields(args)[0]` after the empty guard is the canonical form.

### §5.1 `::locadd <name>` (TS L441-452)

```go
case "locadd":
    // TS L441-452 — admin spawn. Resolves LocType by debugname, spawns
    // a CENTREPIECE_STRAIGHT loc with angle=west=0, duration=500 ticks.
    // Mirrors TS World.addLoc(new Loc(...), 500).
    if args == "" {
        return nil
    }
    name := strings.Fields(args)[0]
    lt := p.client.server.locTypes.ByName(name)
    if lt == nil {
        return nil
    }
    l := entitypkg.NewLoc(
        p.level, p.x, p.z,
        lt.Width, lt.Length,
        entitypkg.LifecycleDespawn,
        lt.ID,
        int(loc.ShapeCentrepieceStraight),
        0, // LocAngle.WEST
    )
    p.client.server.AddLoc(l, 500)
    p.MessageGame(fmt.Sprintf("Loc Added: %s (ID: %d)", name, lt.ID))
```

Notes:
- `int(loc.ShapeCentrepieceStraight)` — explicit cast because `entity.NewLoc` signature is `int`, but `loc.Shape` is a named type. Existing call sites (e.g. `pkg/gamemap/load_test.go:414`) use the same cast.
- `LocAngle.WEST == 0` per `pkg/script/handlers_player.go:91-100` validator range. Inlining `0` matches the lack of a named `LocAngleWest` constant in goscape.
- Duration `500` matches TS literal.

### §5.2 `::npcadd <name>` (TS L453-463)

```go
case "npcadd":
    // TS L453-463 — admin spawn. Resolves NpcType by debugname, constructs
    // a DESPAWN npc at the player's tile with duration=500 ticks. nid is
    // allocated inside addNpc (firstSpawn=true). TS has no messageGame.
    if args == "" {
        return nil
    }
    name := strings.Fields(args)[0]
    nt := p.client.server.npcTypes.ByName(name)
    if nt == nil {
        return nil
    }
    n := NewNpc(0 /* placeholder; allocated inside addNpc */, nt.ID, p.x, p.z, p.level, nt)
    n.lifecycle = NpcLifecycleDespawn
    _ = p.client.server.addNpc(n, 500, true)
```

Notes:
- Pattern mirrors `worldVarsView.AddNpcAt` at `server_varp.go:160-180`. We construct directly rather than routing through `AddNpcAt` because `AddNpcAt` returns a `script.ActiveNpc` wrapper the cheat doesn't need, and the inline form keeps the cheat readable side-by-side with TS.
- `_ = ... addNpc(...)` — TS doesn't check the void return; goscape's `errNpcsFull` is rare enough that silent drop matches TS observable behavior. Acceptable for a staff cheat.
- **No `MessageGame` follow-up** — TS L463 has none. Faithfully omitted.

### §5.3 `::openmain <name>` (TS L464-476)

```go
case "openmain":
    // TS L464-476 — admin interface routing. Resolves ComponentType by
    // debugname, gates on rootLayer == id (only root layers can be main
    // modals), and routes through p.OpenMain (which closes chat + side
    // modals per TS Player.openMainModal modal-mutex).
    if args == "" {
        return nil
    }
    name := strings.Fields(args)[0]
    ct := p.client.server.componentTypes.ByName(name)
    if ct == nil || ct.RootLayer != ct.ID {
        return nil
    }
    p.OpenMain(ct.ID)
```

Notes:
- `p.OpenMain` at `player_script.go:943` already handles modal-mutex (closes chat / side, sets `refreshModal`). The chat / side closure is invisible to TS direct comparison because TS `openMainModal` does the same via separate side-effects — goscape consolidated that into one method.
- No `MessageGame` follow-up — TS L476 has none.

## §6 Test strategy

### §6.1 `ByName` helper tests (`pkg/objtype/`)

For each of `LocTypeConfigs`, `NPCTypeConfigs`, `ComponentTypeConfigs`, the test set mirrors `VarpTypeConfigs.ByName`'s five tests (`pkg/objtype/varptype_test.go:204-264`):

1. `TestXByName_HitViaConfigNames` — populated `ConfigNames` → O(1) hit returns the right `*X` (verify by ID).
2. `TestXByName_MissReturnsNil` — unknown name → nil.
3. `TestXByName_NilReceiverReturnsNil` — `var c *XConfigs = nil; c.ByName(...)` → nil (no panic).
4. `TestXByName_StaleIndexFallsThroughToLinearScan` — `ConfigNames` references an out-of-range id → fallback finds the correct entry by `DebugName` linear scan.
5. `TestXByName_LinearScanWhenConfigNamesEmpty` — `ConfigNames == nil` (test-fixture path) → linear scan succeeds on `DebugName`.

15 tests total. Mechanical / fast.

### §6.2 Cheat dispatch tests (`modules/world/handlers_game_test.go`, or new `handlers_game_admin_spawn_test.go`)

Final filename decided at plan time. NAI-185 added cheats inline to `handlers_game_test.go`; NAI-187 likely follows.

**For each cheat, three tests:**

1. **Happy path** — `p.staffModLevel = 3`, server has `s.locTypes` / `s.npcTypes` / `s.componentTypes` populated with a known-by-name fixture, `processCheat` invoked with `::<cmd> <name>`. Assertions:
   - `locadd`: `s.locObjTracker` contains a new `NonPathing` whose Parent is the spawned `*Loc` at `(p.level, p.x, p.z)` with the expected type/shape/angle. `MessageGame` body `"Loc Added: <name> (ID: <id>)"` appears in the emitted writeOut stream (capture pattern matches NAI-185 — see `handlers_game_test.go:446,1170`).
   - `npcadd`: `s.npcLoop` contains the new `*Npc` with the expected `typeId`. No `MessageGame`.
   - `openmain`: `p.modalMain == ct.ID`, `p.modalState == modalStateMain`, `p.refreshModal == true`. No `MessageGame`.

2. **Unknown name** — `::<cmd> nonexistent_name` → no world mutation (no tracker register / no npcLoop append / no modal set). No `MessageGame`.

3. **Empty args** — `::<cmd>` alone → no-op.

**One combined gate test:** `p.staffModLevel = 2` → none of the three cheats fire. (NAI-185 has a pattern for this; reuse.)

### §6.3 Test fixture audit (controller pre-flight)

Per memory `plan_runnable_test_fixtures` + `test_fixture_view_parity`: at plan time, controller MUST grep `newTestServer` / `newTestPlayer` for `locTypes`, `npcTypes`, `componentTypes` field initialisation, and verify each NAI-187 dispatch test either uses the existing seed or adds a per-test seed. The `ByName` fallback handles the empty-`ConfigNames` case, so populating `Configs` slice with a `DebugName`-tagged entry is sufficient — `ConfigNames` map population is optional for test paths.

## §7 Deviations (anticipated)

None at spec time. All three cheats have direct goscape primitives; arg-parsing convention follows established NAI-185 inline pattern; `OpenMain` modal-mutex side effects are TS-faithful (already audited at NAI-30 / NAI-53 close).

**Potential D-tags at implementation time** (none yet committed):
- If any test path constructs `*XConfigs` without `ConfigNames`, the linear-scan fallback covers it — not a deviation.
- TS `args.shift()` vs goscape's pre-split `args string` is handled by `strings.Fields(args)[0]` — pattern matches NAI-185, not a deviation.

## §8 Risks & mitigations

| Risk | Mitigation |
|---|---|
| `addNpc` returns `errNpcsFull` on a saturated world; silent drop differs from any imagined goscape-side error-message convention. | TS suppresses identically. Acceptable for staff-cheat. Future surfacing can be a separate sub-spec if a developer complains. |
| `LocType.Width` / `Length` defaults to 1 when `code 14/15` are absent in cache data (`pkg/objtype/loctype.go:181-182`). A `locadd` of a no-footprint type spawns a 1×1 loc — matches TS LocType defaults. | No mitigation needed; spec-codified. |
| `ComponentType.RootLayer == -1` by default (`pkg/objtype/componenttype.go:106`). A name resolving to a non-root component fails the gate. | TS-faithful: `type.rootLayer !== type.id` rejects the same set. |
| Plan-time controller pre-flight (memory `controller_preflight`): re-grep `s.locTypes` / `s.npcTypes` / `s.componentTypes` access patterns in `handlers_game.go` to confirm field name + nil-guard convention matches NAI-185. | Plan task. |

## §9 Iteration order / plan shape

Anticipated plan task breakdown (final shape in NAI-187 plan doc):

1. **T1** — `LocTypeConfigs.ByName` + 5 tests. Code-review (Sonnet, per memory `superpowers_code_reviewer_model`).
2. **T2** — `NPCTypeConfigs.ByName` + 5 tests.
3. **T3** — `ComponentTypeConfigs.ByName` + 5 tests.
4. **T4** — `::locadd` dispatch + 3 tests (happy / unknown / empty).
5. **T5** — `::npcadd` dispatch + 3 tests.
6. **T6** — `::openmain` dispatch + 3 tests.
7. **T7** — Combined `staffModLevel < 3` gate test (one test, three sub-asserts).
8. **CLOSE** — Rewrite `DEVIATION-NAI-186-D2-CARRYFORWARD` block: drop admin cluster, retain dev cluster (`reload`, `rebuild`, `speed`) with re-derived per-cheat status framing. Final commit `chore(close): NAI-187 — admin spawn/interface cheats complete`.

T1-T3 are independent (parallel-dispatchable per memory `dispatching-parallel-agents`). T4-T6 each depend on the corresponding ByName from T1-T3. T7 + CLOSE are sequential at the end.

## §10 No-deviation declaration

This spec prescribes no behavioral divergences from TS. All goscape-side variations are mechanical (arg parsing, modal-mutex consolidation in `OpenMain`) and already converged at the cited line numbers in earlier sub-specs. If any divergence surfaces during implementation, plan-author + implementer must declare it as a `DEVIATION-NAI-187-D<N>-...` doc-comment in production (memory `true_to_ts_gate`) before merging.
