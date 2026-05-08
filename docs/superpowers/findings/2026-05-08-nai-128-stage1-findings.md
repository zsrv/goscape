# NAI-128 — Stage 1 findings

**Date:** 2026-05-08
**Stage 1 plan:** `docs/superpowers/plans/2026-05-08-nai-128-rat-loot-cascade-investigation.md` (`07f71e2`)
**Spec:** `docs/superpowers/specs/2026-05-08-nai-128-rat-loot-cascade-investigation-design.md` (`45be85c`)
**Probe:** `modules/world/nai128_rat_loot_test.go::TestNAI128_RatLootCascade`
**HEAD after T5:** `8ae0dae`

## Outcome: BINDING FOUND

The synthetic cascade probe drives the rat death-loot end-to-end against the real Lumbridge cache, asserting at each link. T1–T4 PASS; **T5 FAILS** at the final `obj_add` link.

### Subtest results at HEAD `8ae0dae`

| Subtest | Result | Evidence |
|---|---|---|
| `FixtureLoaded` | PASS | scriptProvider + npcTypes + objTypes loaded non-empty |
| `BaselineState` | PASS | rat uid=62259201 typeId=950 HP=3 at (3094,3106,0); player coord (3094,3107,0) |
| `Preconditions` | PASS | `s.addPlayer(p)` → `uid=1 slot=1`; `heroPoints.AddHero(1, 5)`; `EnqueueScriptForTrigger(TriggerAiQueue2, 0, 5)`; HP forced to 1 |
| `AiQueueCascade` | PASS | one `processNpcQueue` call → HP 1→0 + queue drained (ai_queue3 phase-collapsed via for-len-grows pattern as predicted by spec §4.4) |
| `GroundObjs` | **FAIL** | `z.Objs` length = **0** at rat coord post-cascade (want 2: death_drop + raw_rat_meat) |

### Verbatim FAIL output

```
=== RUN   TestNAI128_RatLootCascade/GroundObjs
    nai128_rat_loot_test.go:252: ground obj count at rat coord = 0; want 2
        (binding candidate E: OBJ_ADD not registering OR npc_findhero=false skipping the if-block)
    nai128_rat_loot_test.go:253:   observed types at rat coord: []
    nai128_rat_loot_test.go:254:   zone.Objs full: 0 entries
--- FAIL: TestNAI128_RatLootCascade/GroundObjs (0.00s)
```

### Pre-cascade param resolution (sanity)

Resolved successfully *before* the failing assertion:

- `s.objTypes.ConfigNames["raw_rat_meat"]` → present (T5 implementer confirmed)
- `s.paramTypes.ConfigNames["death_drop"]` → present
- `ratType.Params[uint32(dropParamID)]` → `uint32(2530)` (sign-extends to dropObjID=2530)
- `dropObjID >= 0` (no -1 sentinel returned by death_drop param)

So binding candidates D (`npc_param(death_drop)` returns null/-1) and E1 (raw_rat_meat ObjType cache gap) are **refuted**: both ID lookups succeed.

## Bound candidate per spec §4.2

**Candidate E (broad)**: ai_queue3 fires (queue drained, cascade ran), but `obj_add` registers ZERO entries in `z.Objs`. Two sub-candidates remain — disambiguation is Stage-2 brainstorm work:

### Sub-candidate E2a — NPC_FINDHERO returns false

`[ai_queue3,newbiegiantrat]` (`tut_giant_rat.rs2:6`) gates both `obj_add` calls behind `if (npc_findhero = ^true) { … }`. The generic fallthrough `[ai_queue3,_]` → `[proc,npc_default_death]` (`Content/scripts/skill_combat/scripts/npc/npc_combat.rs2:line of npc_default_death`) ALSO gates its single `obj_add` behind `if ($drop ! null & npc_findhero = ^true)`. So if `NPC_FINDHERO` returns 0 along whichever ai_queue3 path dispatched, both paths produce zero objs — exactly matching observed behavior.

`handleNpcFindHero` at `pkg/script/handlers_npc.go:1105` reads `s.ActiveNpc.TopContributor()`. Path through `LookupPlayerByUID(uid)` at `modules/world/server.go:791` iterates `s.playerLoop`. The probe registered the player via `s.addPlayer(p)` (Preconditions log confirms `uid=1 slot=1` post-`addPlayer` — i.e. `s.playerLoop` is non-empty and `p.uid=1`). `heroPoints.AddHero(1, 5)` was called *after* `addPlayer`, so the ledger keys on `uid=1`. Preconditions assertion `top != p.UID()` passed at HEAD, confirming `TopContributor()` returns 1.

So nominally NPC_FINDHERO *should* succeed. Stage-2 brainstorm needs to verify:

1. Whether `processNpcQueue` actually invokes the rat-specific `[ai_queue3,newbiegiantrat]` script (vs. the generic, vs. neither). Read `s.scriptProvider.GetByTrigger(TriggerAiQueue3, ratTypeID, ratType.Category)` return value directly.
2. Whether the script execution completes successfully or errors silently. `resumeOrFinishNpc` may swallow errors. Add slog.Info instrumentation to the script runner around the OBJ_ADD invocation.
3. Whether `s.Self` is set inside the script state when OBJ_ADD runs (handleObjAdd:103 requires `s.Self != nil` and returns error otherwise). NPC_FINDHERO sets `s.Self = player; s.Pointers |= PtrActivePlayer` on success — but only if `LookupPlayerByUID` returns non-nil.

### Sub-candidate E2b — OBJ_ADD path failure post-FINDHERO

`objAddCommon` at `pkg/script/handlers_obj.go:48` calls `s.World.AddObj(level, x, z, objId, count, duration, receiverID)`. If this returns nil (no error path back to test) or if the World adapter's AddObj implementation has a bug, the script completes with no zone state mutation. Possible failures:

- `objType.Members && s.World.MapMembers() == 0` short-circuit (line 75–77) — rat_meat is unlikely Members-only but verify.
- `checkDuration` / `checkCoord` validation failures returning silently.
- `s.World.AddObj` adapter on the Server type writing to the wrong zone or failing the write.

Stage-2 disambiguation: a single targeted t.Logf on the OBJ_ADD entry point + return value would bind E2a vs E2b in one cycle.

## Refuted hypotheses

- **NAI-127 §6 cascade-via-unhandled-opcode**: REFUTED at brainstorm via static disasm of the 148-script call graph rooted at `[label,player_melee_attack]` finding 0 unhandled opcodes. Probe-side confirms: ai_queue2 + ai_queue3 fired without panic.
- **B (NPC_DAMAGE handler bug)**: REFUTED — T4 AiQueueCascade PASSes the HP=0 assertion. NPC_DAMAGE works.
- **C (`[ai_queue2,_]` / `~npc_default_damage` not dispatching)**: REFUTED — same as B (HP=0 only happens if ai_queue2 → ~npc_default_damage → NPC_DAMAGE all run).
- **D (`npc_param(death_drop)` returns null/-1)**: REFUTED — `ratType.Params[death_drop]` resolves to a positive obj id (2530).
- **E1 (raw_rat_meat ObjType cache gap)**: REFUTED — `s.objTypes.ConfigNames["raw_rat_meat"]` resolves successfully.
- **Phase-collapse in `processNpcQueue`**: CONFIRMED — single call drains both ai_queue2 and the re-entered ai_queue3 (spec §4.4 prediction held).

## Stage-2 routing

The synthetic probe binds to "candidate E" but not narrowly enough to prescribe a fix. Stage-2 brainstorm should:

1. **Disambiguate E2a vs E2b** with one of:
   - Direct invocation of `handleNpcFindHero` on a fresh `ScriptState` seeded from `rat` + `s` to check whether it pushes 0 or 1.
   - slog.Info instrumentation in `handleNpcFindHero`, `handleObjAdd`, `objAddCommon`, and `Server.AddObj` adapter.
   - A unit-level scriptstate test exercising `[ai_queue3,newbiegiantrat]` against the same fixture, with PC-level tracing.
2. **If E2a (NPC_FINDHERO=false)**: verify whether `s.ActiveNpc` is set correctly at script-start time, and whether `LookupPlayerByUID(1)` returns the registered player when invoked from inside the script-running goroutine.
3. **If E2b (OBJ_ADD fails post-FINDHERO)**: trace `s.World.AddObj` from the goscape adapter into `pkg/zone` to find where the obj is dropped or fails to register.

The existing probe (`TestNAI128_RatLootCascade`) becomes the regression gate for Stage 2: turning the failed `GroundObjs` assertion green with the production fix is the close criterion.

## Resume prompt for next session

```
NAI-128 Stage 2 brainstorm. Stage 1 probe at
modules/world/nai128_rat_loot_test.go ran on 2026-05-08; HEAD 8ae0dae.

Outcome: BINDING FOUND. T1–T4 PASS, T5 FAIL: zone.Objs has 0 entries
at rat coord post-cascade. Bound candidate per spec §4.2: E (ai_queue3
fires + cascade drains, but obj_add registers nothing).

Disambiguation needed between E2a (NPC_FINDHERO returns false despite
TopContributor()=1 + addPlayer-registered player) and E2b (OBJ_ADD or
World.AddObj path fails post-FINDHERO). See findings doc:
docs/superpowers/findings/2026-05-08-nai-128-stage1-findings.md

Brainstorm should propose ONE disambiguation probe (direct
handleNpcFindHero invocation, scriptstate trace, or slog.Info
instrumentation) and bind in one cycle. Stage-2 plan TDDs against
the existing probe — turn GroundObjs green.

Memory entries to grep at start: cascade_theory_smoke_binding,
disasm_reframes_inferred_binding, controller_preflight,
verify_implementer_claims, no_rng_seam_cascade_probe_bypass.
```
