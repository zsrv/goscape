# NAI-190 — `World.reload()` port + `::reload` cheat wiring

**Date:** 2026-05-13
**Status:** Spec (awaiting plan)
**Tech stack:** Go 1.26+
**TS source:** `LostCityRS/Engine-TS/src/engine/World.ts:206-292` (`World.reload`), plus the `::reload` cheat callsite at `LostCityRS/Engine-TS/src/network/game/client/handler/ClientCheatHandler.ts:149-150`.
**Predecessors:** NAI-187 (admin spawn cohort + ByName cluster), NAI-188 (::speed + tick-rate field promotion), NAI-189 (DEBUGPROC dispatch path; closes ClientCheatHandler.ts except `reload`/`rebuild`).
**HEAD at spec-write:** `9213c52`

## §1 Goal

Port `World.reload(clearInvs: boolean = true)` from TS `World.ts:206-292` into a new method `(*Server).Reload(clearInvs bool) error` on the goscape world module. Wire the `::reload` cheat in the dev-block at `modules/world/handlers_game.go:589` (the fixed-cmd switch following the debugproc prefix branch) to call `s.Reload(true)`.

`World.reload()` is the cache & script hot-swap path: re-invokes all `*Type.Load(...)` loaders into new registries, atomically swaps the registry fields on `Server`, reconciles `Server.vars`/`Server.varsStrings` if the `VarSharedType` count changed, reconciles world-shared and per-player inventories when `clearInvs=true`, reloads the script provider, regenerates `cache.MakeCRCs()`, and re-runs `cache.PreloadClient(...)`.

This is the first sub-spec in a planned NAI-190 → NAI-204 arc that eventually closes the second carryforward item (`::rebuild`). The arc decomposition is documented in §11; NAI-190 itself is independent of the compiler / file-watcher work and closes the `::reload` cheat in isolation.

## §2 Out of scope

| Concern | TS line | Why deferred |
|---|---|---|
| `::rebuild` cheat | `ClientCheatHandler.ts:151-153` | Calls `World.rebuild()` which posts `'world_rebuild'` to a `DevThread` worker. Requires the cache-compiler arc (NAI-191 .. NAI-203) to land first; the worker wiring is NAI-204. |
| `tools/pack/*` source-compile pipeline (~8200 LOC of TS) | `tools/pack/PackAll.ts` and friends | Multi-sub-spec arc (NAI-191 .. NAI-203). See §11. |
| `DevThread` analog (fsnotify + goroutine) | `src/cache/DevThread.ts` | NAI-204. |
| `// todo: detect and reload static data (like maps)` | TS L290 | Inherited TS limitation — TS itself does NOT reload maps. Goscape inherits the carryforward verbatim. |
| GameMap re-build, NPC respawn, static-loc/obj zone repopulation | n/a in TS reload | TS reload does NOT touch these. Goscape must not either. |
| `FontType.load` / `WordEnc.load` | TS L297-298 (in `start()`, not `reload()`) | TS does not include these in `reload()`. Goscape mirrors. |
| Friends-server inbound `RELAY_RELOAD` caller | TS `World.ts:2036` | Goscape has no friends-server inbound receiver yet (`bridges.go` covers outbound only). The `Reload(clearInvs)` signature preserves the parameter for the future caller; no dead code is added in NAI-190. |

## §3 Pre-flight audit

Per memory `controller_preflight` and `risk_register_premise_grep`, every premise below was re-verified against HEAD `9213c52`.

### §3.1 TS `World.reload` body (`World.ts:206-292`)

The exact TS flow is captured in §4 as 11 ordered steps. Key bindings:

- `clearInvs` defaults to `true`. The `::reload` cheat (TS `ClientCheatHandler.ts:149-150`) calls `world.reload()` with no argument → `clearInvs=true`.
- `World.start()` (TS L300) also calls `this.reload()` at bootstrap — same `clearInvs=true` default. The bootstrap call runs on empty registries; goscape's existing startup path at `modules/world/server.go:218-361` is functionally equivalent and is NOT replaced by `Reload`. Reload is a runtime path, not a bootstrap path.
- The friends-server relay at `World.ts:2036` (`FriendsServerOpcodes.RELAY_RELOAD`) is the only `reload(false)` caller. Goscape has no inbound friends-server channel yet, so this caller is absent. We keep the `clearInvs bool` parameter rather than hardcoding `true`, with a doc-comment naming the future caller.

### §3.2 goscape registry shape at HEAD `9213c52`

Per `modules/world/server.go:93-121`, the following fields hold registry pointers loaded once during `NewServer`:

| Field | Loader | TS counterpart |
|---|---|---|
| `s.paramTypes` | `objtype.LoadParams(cfg.CachePath)` | `ParamType.load` |
| `s.objTypes` | `objtype.LoadObjTypes(cfg.CachePath, params)` | `ObjType.load` (depends on params) |
| `s.locTypes` | `objtype.LoadLocTypes(cfg.CachePath)` | `LocType.load` |
| `s.npcTypes` | `objtype.LoadNPCTypes(cfg.CachePath)` | `NpcType.load` |
| `s.idkTypes` | `objtype.LoadIdkTypes(cfg.CachePath)` | `IdkType.load` |
| `s.seqTypes` | `objtype.LoadSeqTypes(cfg.CachePath, seqFrames)` | `SeqFrame.load` + `SeqType.load` |
| `s.spotanimTypes` | `objtype.LoadSpotanimTypes(cfg.CachePath)` | `SpotanimType.load` |
| `s.varpTypes` | `objtype.LoadVarpTypes(cfg.CachePath)` | `VarPlayerType.load` |
| `s.enumTypes` | `objtype.LoadEnumTypes(cfg.CachePath)` | `EnumType.load` |
| `s.structTypes` | `objtype.LoadStructTypes(cfg.CachePath)` | `StructType.load` |
| `s.invTypes` | `objtype.LoadInvTypes(cfg.CachePath)` | `InvType.load` |
| `s.mesanimTypes` | `objtype.LoadMesanimTypes(cfg.CachePath)` | `MesanimType.load` |
| `s.dbTableTypes` | `objtype.LoadDbTableTypes(cfg.CachePath)` | `DbTableType.load` |
| `s.dbRowTypes` | `objtype.LoadDbRowTypes(cfg.CachePath)` | `DbRowType.load` |
| `s.dbTableIndex` | `objtype.BuildDbTableIndex(dbTable, dbRow)` | `DbTableIndex.init` |
| `s.huntTypes` | `objtype.LoadHuntTypes(cfg.CachePath)` | `HuntType.load` |
| `s.varnTypes` | `objtype.LoadVarnTypes(cfg.CachePath)` | `VarNpcType.load` |
| `s.varsTypes` | `objtype.LoadVarsTypes(cfg.CachePath)` | `VarSharedType.load` |
| `s.componentTypes` | `objtype.LoadComponentTypes(cfg.CachePath)` | `Component.load` |

All loaders are top-level functions returning fresh registry pointers — no globals to reset. Re-invocation produces independent instances. Verified by reading each `LoadXxx` signature in `pkg/objtype/`.

### §3.3 Script provider

`pkg/script/provider.go` exposes `(*Provider).Load(dir string) error` at line ~? (verify at plan-write). Currently returns `error` only; loaded-script count is not surfaced. TS `ScriptProvider.load` returns the count (or `-1` on failure) — required for the broadcast message ("Loaded N scripts." vs "There was an issue while reloading scripts.").

**Required signature change:** `(*Provider).Load(dir string) (int, error)`. Single-call-site impact: `server.go:358` ignores the count. Confirmed low blast radius at spec-write; plan task will re-grep at plan-write to verify no test-helper or fixture site exists.

### §3.4 Cache primitives

| Primitive | Location | Re-callable? |
|---|---|---|
| `cache.MakeCRCs()` | `pkg/cache/crctable.go:47` | ✓ Resets `CrcTable`, `CrcBuffer`, `CrcBuffer32`, `CrcBytes` at the top of the function. |
| `cache.PreloadClient(baseDir)` | `pkg/cache/preloaded.go:42` | ◐ Does NOT clear `Preloaded` / `PreloadedCRC` maps before population — stale entries leak (TS `preloadClient()` has the same posture, so this is parity, not a deviation). New entries overwrite same-name old ones via map assignment. |

Both are global package state; both are re-invocable. The "stale-entry leak" is TS-parity and explicitly NOT a goscape deviation — note in the reload doc-comment but do NOT tag.

### §3.5 GameMap type-binding

`modules/world/server.go:233-240` injects loc/obj types into the GameMap via `gm.SetLocTypes` / `gm.SetObjTypes`. The GameMap retains these refs internally; collision queries and loc-iteration paths read from those refs (not from `s.locTypes` / `s.objTypes`).

If reload swaps `s.locTypes` without re-injecting into `gm`, GameMap reads will use stale type configs. This is **glue-only divergence from TS** — TS reads `LocType.get(id)` from a package singleton, so re-load implicitly propagates. Mark **DEVIATION-NAI-190-D1-GAMEMAP-RE-INJECT** at the Reload call-site. Pin via test.

### §3.6 Concurrency

Per memory `plan_race_tag_for_cross_goroutine_test` (NAI-188): goscape's production world module is single-goroutine on the tick. `handleClientCheat` runs synchronously on the tick goroutine (driven by `processClientsIn` in `tick.go`). Therefore:

- `(*Server).Reload` runs inline from the cheat handler. No locks needed.
- Tick latency spike is acceptable and matches TS (where `reload` blocks Node's main thread).
- No deferred-to-tick-boundary queue is required.

### §3.7 World-shared and per-player inv data model

| State | Location | Notes |
|---|---|---|
| `s.invs` (world-shared) | `modules/world/server.go:91` `map[int]*inventory.Inventory` | Empty at boot; lazily populated by world content. `inventory.FromType(invType *InvType)` exists at `server_invs.go:44`. |
| `p.invs` (per-player) | `modules/world/player.go:335` `map[int]*inventory.Inventory` | Lazily populated; nil-initialised. Tick.go:147 re-initialises to empty for fresh logins. |
| `Server.players` shape | `modules/world/server.go:?` | Plan-author must verify the iteration shape (slice vs map; sparse vs dense) before codifying the SCOPE_TEMP delete loop. |

### §3.8 VarsType.Type tagging

Per `pkg/objtype/varstype.go:12` and `pkg/objtype/paramtype.go:30-38`, every `VarSharedType` has a `Type ScriptVarType` field tagged with one of `ScriptVarTypeInt | ScriptVarTypeString | ScriptVarTypeObj | ScriptVarTypeLoc | …`. The TS reload's L259-267 re-init loop dispatches on this tag:

```
if (varsh.type === ScriptVarType.STRING) { continue; }
else                                     { this.vars[i] = varsh.type === ScriptVarType.INT ? 0 : -1; }
```

Goscape mirror: `objtype.ScriptVarTypeString` and `objtype.ScriptVarTypeInt`. Plan task threads these through verbatim.

## §4 Reload pipeline (TS-parity, 11 steps)

The pipeline implementation lives in a new file `modules/world/reload.go` exposing one public method:

```go
func (s *Server) Reload(clearInvs bool) error
```

Steps, mirroring TS L207-292:

1. **Load pre-inv config registries into local vars** (TS L207-219, minus `InvType`):
   `varpTypes'`, `params'`, `objTypes'`, `locTypes'`, `npcTypes'`, `idkTypes'`, `seqFrames'`, `seqTypes'`, `spotanim'`, `categoryTypes'` (if present in goscape — see §9 risk), `enumTypes'`, `structTypes'`. Twelve locals. Any loader error → return without mutating `s.*`.
2. **Load InvType** (TS L219): `invTypes' := LoadInvTypes(...)`. Error → return.
3. **Atomic swap** of pre-inv registry fields: `s.paramTypes=params'; s.objTypes=objTypes'; …; s.invTypes=invTypes'`.
4. **`clearInvs` reconcile branch** (TS L221-236): see §5.
5. **Load post-inv configs** (TS L238-244): `mesanim'`, `dbTable'`, `dbRow'`, `huntTypes'`, `varnTypes'`, `varsTypes'`. Build new `dbTableIndex' := BuildDbTableIndex(dbTable', dbRow')`. Errors mid-pipeline → see §8.
6. **Swap post-inv registries**: `s.mesanimTypes=mesanim'; s.dbTableTypes=dbTable'; …; s.varsTypes=varsTypes'; s.dbTableIndex=dbTableIndex'`.
7. **VarShared resize** (TS L246-268): see §6.
8. **Load Component types** (TS L270): `componentTypes' := LoadComponentTypes(...)`; swap.
9. **Reload scripts** (TS L272-285):
   - `count, err := s.scriptProvider.Load(filepath.Join(s.cfg.CachePath, "server"))`.
   - If `s.cfg.NodeDebug`:
       `count == -1` → `s.broadcastMes("There was an issue while reloading scripts.")`
       `count >= 0`  → `s.broadcastMes(fmt.Sprintf("Loaded %d scripts.", count))`
   - Else:
       `count == -1` → `s.log.Error("script reload failed", "err", err)`
       `count >= 0`  → `s.log.Debug("scripts reloaded", "count", count)`
10. **CRC regen + client preload** (TS L288, L291):
    `cache.MakeCRCs()`; `cache.PreloadClient(filepath.Join(s.cfg.CachePath, "client"))`.
11. **GameMap re-injection** (DEVIATION-NAI-190-D1-GAMEMAP-RE-INJECT):
    `s.gamemap.SetLocTypes(s.locTypes); s.gamemap.SetObjTypes(s.objTypes); s.gamemap.SetMembers(s.cfg.NodeMembers)`.
    Required because GameMap holds its own refs (TS reads package singletons; goscape goes through these setters).

Return `nil` on success; first error encountered after step 3 is returned (post-swap mutations may have already taken effect — see §8 / DEVIATION-NAI-190-D2-HALF-SWAP).

## §5 Inv reconcile (`clearInvs=true`, TS L221-236)

```go
if clearInvs {
    s.invs = make(map[int]*inventory.Inventory)
    for id := 0; id < len(s.invTypes.Configs); id++ {
        inv := s.invTypes.Configs[id]
        if inv == nil {
            continue // goscape-defensive; TS InvType.get(id) returns a sentinel
        }
        switch inv.Scope {
        case objtype.InvTypeScopeShared:
            s.invs[id] = inventory.FromType(inv)
        case objtype.InvTypeScopeTemp:
            // Plan-author: verify s.players iteration shape (slice vs map)
            // before codifying. Pattern mirrors §3.7.
            for _, p := range s.players {
                if p == nil || p.invs == nil {
                    continue
                }
                if _, ok := p.invs[id]; ok {
                    delete(p.invs, id)
                }
            }
        case objtype.InvTypeScopePerm:
            // TS does not reconcile SCOPE_PERM; goscape mirrors.
        }
    }
}
```

The `inv == nil` guard is a goscape-defensive check labelled per memory `defensive_gate_doc_comment_label`. TS uses `InvType.get(id)` which always returns a sentinel.

## §6 VarShared resize (TS L246-268, mirror verbatim)

```go
if len(s.vars) != len(s.varsTypes.Configs) {
    oldVars := s.vars
    oldStrs := s.varsStrings
    s.vars = make([]int32, len(s.varsTypes.Configs))
    s.varsStrings = make([]string, len(s.varsTypes.Configs))
    n := min(len(oldVars), len(s.vars))
    copy(s.vars, oldVars[:n])
    copy(s.varsStrings, oldStrs[:n])

    // TS L259-267: iterates ALL indices (not just net-new), unconditionally
    // overwriting non-string slots to the type default. Looks bug-shaped
    // but is in TS; mirrored verbatim per the true-to-TS gate.
    // DEVIATION-NAI-190-D3-CANDIDATE-VARSHARED-CLOBBER documents this if
    // it turns out to be unintentional in TS (file an upstream issue).
    for i := 0; i < len(s.vars); i++ {
        varsh := s.varsTypes.Configs[i]
        if varsh == nil {
            continue // goscape-defensive
        }
        if varsh.Type == objtype.ScriptVarTypeString {
            continue
        }
        if varsh.Type == objtype.ScriptVarTypeInt {
            s.vars[i] = 0
        } else {
            s.vars[i] = -1
        }
    }
}
```

The clobber-after-copy is verbatim TS. Two readings:

1. **TS bug** — copy is then unconditionally clobbered. Tagged as DEVIATION candidate; mirror verbatim regardless per `true_to_ts_gate`.
2. **Intentional** — when `varsTypes` count changes, the type-tag at index `i` may have also changed (e.g., a `vars.cfg` edit shuffled type assignments). Reset is safer than carrying a stale-typed value.

The implementer-side test `TestReload_VarSharedCountGrew_CopiesOverlapAndResetsNew` pins the clobber-after-copy behaviour exactly.

## §7 Cheat wiring

Replace the 2-line CARRYFORWARD comment for `reload`/`rebuild` at `modules/world/handlers_game.go:539-547`. Add a `"reload"` case to the dev-block fixed-cmd switch following the debugproc prefix branch (around `handlers_game.go:589`):

```go
case "reload":
    // TS ClientCheatHandler.ts:149-150 — World.reload() (default clearInvs=true).
    if err := p.client.server.Reload(true); err != nil {
        // TS dispatches via try/catch on uncaught throws; goscape
        // surfaces explicitly. DEVIATION-NAI-190-D2-HALF-SWAP documents
        // the half-mutated state risk.
        p.client.server.log.Error("reload cheat failed", "err", err)
        p.MessageGame("Reload failed: see server log.")
    }
    return nil
```

The carryforward block (`modules/world/handlers_game.go:539-547`) is rewritten on the NAI-190 close commit per memory `tracker_carryforward_listings_compound`. The new tally is **1** (only `::rebuild` remains, attributed to NAI-191+ in the close commit body).

## §8 Failure-mode contract (DEVIATION-NAI-190-D2-HALF-SWAP)

The pipeline mirrors TS step-by-step. TS does NOT roll back on partial failure — uncaught throws bubble out via Node's exception handling. Goscape's `error` return surfaces the failure to the caller (the cheat handler), but the swap-after-load ordering in §4 means **steps 5-11 may have already mutated `s.*` fields when step 9 (scripts) returns an error**.

**DEVIATION-NAI-190-D2-HALF-SWAP** documents this. The contract:
- Pre-step-3 errors leave `s.*` unmutated. Verified by the `TestReload_LoaderError_NoMutation` test.
- Post-step-3 errors leave the registry fields in a "newer-config + older-or-failed-scripts" state. Acceptable per TS-parity; users are expected to retry `::reload` after fixing source.
- No rollback path is added in NAI-190.

A "two-phase commit" alternative (load everything into locals, then swap atomically) was considered and rejected because:
- TS does not do this.
- It separates loader errors from CRC/preload/script errors (which mutate global package state via `cache.MakeCRCs`, `cache.PreloadClient`, `scriptProvider.Load`). Two-phase commit would only protect the registry-pointer swaps; the global-state mutations would still be half-applied.

## §9 Risks & open questions

| Risk | Mitigation | Owner |
|---|---|---|
| `s.players` iteration shape not yet verified at spec-write. | Plan task §1 must re-grep `s\.players\b` and `range s.players` and codify the shape (slice vs map) before drafting the inv-reconcile fixture. | Plan author |
| `categoryTypes` may or may not have a goscape loader (TS L216 has `CategoryType.load`). | Plan task §1 must verify `pkg/objtype/categorytype.go` existence; if absent, document as **stretch** or **DEVIATION-NAI-190-D4-NO-CATEGORYTYPES**. | Plan author |
| `s.scriptProvider.Load` signature change to `(int, error)` has unknown blast radius outside `server.go:358`. | Re-grep at plan-write for all callers including test fixtures. Memory `parallel_adapter_init_duplication` reminds us to enumerate all sibling sites. | Plan author |
| `cache.PreloadClient` stale-entry leak (no map clear) is TS-parity, not a deviation. | Note in `Reload` doc-comment; do NOT tag. | Implementer |
| Reload runs on tick goroutine; tick spike during cache reload could be measurable. | Match TS (which has the same spike). Acceptable for a dev-tier cheat. | Implementer |
| `inventory.FromType` takes `*InvType` not `int` id (goscape divergence from TS's `Inventory.fromType(id)`). | Verified at `server_invs.go:44`. Plan code block must thread the type pointer, not the id. | Plan author |
| VarShared clobber-after-copy may be a TS bug. | Mirror verbatim per `true_to_ts_gate`. Tag as DEVIATION-NAI-190-D3-CANDIDATE. File an upstream TS issue post-close if reviewer concurs. | Reviewer |

## §10 Deviations summary

| Tag | Scope | Rationale |
|---|---|---|
| **DEVIATION-NAI-190-D1-GAMEMAP-RE-INJECT** | Step 11 of pipeline calls `gm.SetLocTypes`/`SetObjTypes`/`SetMembers` to propagate swapped registries into the GameMap's internal refs. TS reads package singletons so no propagation needed. | Glue-only — required for goscape's separated GameMap type-binding. Not a behavioral divergence. |
| **DEVIATION-NAI-190-D2-HALF-SWAP** | On mid-pipeline error (post step 3), `s.*` fields are partially mutated. | TS-parity: TS does not roll back. Documented; no rollback path added. |
| **DEVIATION-NAI-190-D3-CANDIDATE-VARSHARED-CLOBBER** | TS L259-267 unconditionally re-initialises every non-string vars slot after the copy, clobbering copied values. | Mirrored verbatim. CANDIDATE status: promote to confirmed DEVIATION if reviewer concludes the TS behaviour is buggy; else retire the tag. |

## §11 Out-of-spec context — NAI-190 arc decomposition

NAI-190 stands alone but is the first sub-spec in an arc that eventually closes the second carryforward item (`::rebuild`). The arc is not committed in NAI-190; this section exists so the reviewer understands where NAI-190 fits.

| Sub-spec | Scope | Independence |
|---|---|---|
| **NAI-190** (this) | `World.reload()` port + `::reload` cheat. | Independent of compiler. |
| **NAI-191** | Pack-pipeline foundation: `tools/pack/{PackFileBase,PackFile,FsCache,Parse,NameMap}.ts` ported. | Prerequisite for per-config compilers. |
| **NAI-192 .. NAI-197** (~6 sub-specs) | Per-config compilers grouped 3-4 per sub-spec: {Param,Obj,Loc,Npc}, {Idk,Seq,SpotAnim,Inv}, {Enum,Struct,Mesanim,Hunt}, {DbTable,DbRow,Flo,Category}, {Varp,Varn,Vars,Component}. | Each cluster independent. |
| **NAI-198 .. NAI-202** (~5 sub-specs) | RuneScript .rs2 compiler: lexer, parser, semantic analysis, bytecode emit, proc/label resolution. | Independent of config packers. |
| **NAI-203** | `packAll` orchestrator. | Depends on NAI-191 + per-config + script compiler. |
| **NAI-204** | fsnotify file-watcher + DevThread analog goroutine + `::rebuild` cheat wiring + dev-message broadcasts. | Final piece; closes the second carryforward item. |

NAI-190 closes 1 of 2 carryforward items. The remaining `::rebuild` is attributed to NAI-191 .. NAI-204 in the close commit body.

## §12 Test strategy

Tests live in a new `modules/world/reload_test.go`. The implementer-side TDD plan in NAI-190 unrolls these into RED → GREEN → REVIEW tasks.

### Unit tests

| Test | Pins |
|---|---|
| `TestReload_FreshLoad_PopulatesAllFields` | After `s.Reload(true)` on a fixture cache: each `s.*Types` field is non-nil and `len(Configs) > 0`. Positive control. |
| `TestReload_PreservesIdentitySwap` | Capture `s.objTypes` ptr pre-reload; post-reload assert ptr differs. Confirms genuine reassignment, not in-place mutation. |
| `TestReload_ClearInvsTrue_EmptiesAndRebuildsSharedInvs` | Pre-populate `s.invs[shareID]` with sentinel; reload(true); assert SCOPE_SHARED id has a fresh inv. |
| `TestReload_ClearInvsTrue_DeletesTempScopeFromPlayers` | Pre-populate `p.invs[tempID]` with sentinel; reload(true); assert `p.invs[tempID] == nil`. |
| `TestReload_ClearInvsFalse_LeavesInvsUntouched` | Pre-populate; reload(false); assert sentinels still present. Pins future friends-relay caller's behaviour. |
| `TestReload_VarSharedCountUnchanged_PreservesValues` | `s.vars[0]=42`; reload (no count change); assert `s.vars[0]==42`. |
| `TestReload_VarSharedCountGrew_CopiesOverlapAndResetsNew` | Pre-state 3 vars `[10,20,30]`; reload with new count 5 and mixed types; assert `s.vars` matches the verbatim TS clobber-after-copy outcome. |
| `TestReload_VarSharedStringType_Untouched` | A STRING-typed slot retains its prior string value through reload. |
| `TestReload_ScriptProviderLoadCount_DriveSuccessBroadcast` | scriptProvider.Load → (42, nil); NodeDebug=true; assert broadcast == "Loaded 42 scripts." |
| `TestReload_ScriptProviderLoadCount_DriveFailureBroadcast` | scriptProvider.Load → (-1, err); NodeDebug=true; assert broadcast == "There was an issue while reloading scripts." |
| `TestReload_NotNodeDebug_DoesNotBroadcast` | NodeDebug=false; assert no broadcast, logger called. |
| `TestReload_GameMapTypesReInjected` | Capture `gm.locTypes` ptr pre-reload; post-reload assert `gm.locTypes == s.locTypes` (and differs from pre-reload). |
| `TestReload_CRCRegen` | `cache.CrcBuffer32 = 0xDEAD` pre-reload; post-reload assert overwritten. |
| `TestReload_LoaderError_NoMutation` | Inject fixture w/ bad `obj.dat`; reload returns error; assert `s.objTypes` unchanged (pre-step-3 error → no mutation). |

### Integration tests (cheat-level)

| Test | Pins |
|---|---|
| `TestHandleClientCheat_Reload_DispatchesAndBroadcasts` | Staff-level player, NodeProduction=false; send `::reload`; assert `s.broadcastMes` invoked. |
| `TestHandleClientCheat_Reload_ErrorPath_LogsAndSendsPrivate` | Force scriptProvider.Load failure; send `::reload`; assert player got `MessageGame("Reload failed: ...")` AND logger saw Error level entry. |
| `TestHandleClientCheat_Reload_DefaultsClearInvsTrue` | Sentinel SCOPE_TEMP inv pre-state; send `::reload`; assert sentinel cleared. Pins the `Reload(true)` default at the cheat call-site. |

### Skip-with-pin (DEVIATION-NAI-190-D2-HALF-SWAP)

`TestReload_LoaderErrorMidPipeline_LeavesHalfSwapped` — captures the documented half-swap. Per memory `skip_pin_full_struct_capture`, the implementer captures the actual observed `%+v` of `s.objTypes` post-error (not inferred). Marked `t.Skip` with a `t.Logf` snapshot so future re-investigation has a verbatim reference.

### Test infrastructure

- **`broadcastMes` capture**: refactor `s.broadcastMes` to a function-field on `Server` (overridable in tests) OR add a `BroadcasterBridge` interface to `bridges.go` mirroring the LoggerBridge pattern. Plan task picks the lighter touch.
- **Fixture cache directory**: reuse existing `testdata/` fixtures if present; else build a minimal cache via `testdata/cache_min/` with hand-crafted `.dat` files for one InvType (SHARED), one InvType (TEMP), 3 VarSharedTypes, and 0 scripts. Plan task §1 surveys.
- **scriptProvider.Load signature change** (`error` → `(int, error)`): single non-test caller at `server.go:358` (verified at spec-write). Plan task §1 re-greps to confirm no test-helper site.

## §13 Acceptance criteria

NAI-190 closes when:

1. `(*Server).Reload(clearInvs bool) error` exists in `modules/world/reload.go` with the 11-step pipeline from §4.
2. The `::reload` cheat is wired in the dev-block at `handlers_game.go` per §7.
3. `s.broadcastMes` exists on `Server` and is reusable (will be re-used in NAI-204 for DevThread broadcasts).
4. `(*Provider).Load(dir) (int, error)` signature change is propagated to all callers.
5. All unit tests + integration tests + skip-with-pin from §12 are GREEN.
6. CARRYFORWARD comment at `handlers_game.go:539-547` rewritten on close commit; new tally = 1 (`::rebuild` only, attributed to NAI-191..204).
7. DEVIATION tags D1, D2 land in code with the framing from §10. D3-CANDIDATE either confirmed-and-promoted or retired-with-rationale by the close commit.
8. `go test ./...` passes with `-race` (single-goroutine, no new races).
9. Close commit message includes `Closes memory:` trailer per memory `close_commit_memory_trailer`.
