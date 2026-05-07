# NAI-121 Bundle 2 — V-PARTIAL Stage 1 audit findings

**Date:** 2026-05-07
**Scope:** Read-only investigation, no production code.
**Author:** Sonnet Explore audit subagent + controller verification.

---

## 1. Symptom recap

`%npc_combat_xp_multiplier` (declared in `LostCityRS/Content/scripts/npc/configs/ai_spawn.varn`, INT default) reads as `0` after Tutorial Island giant-rat attack lands. Effect: combat XP multiplies to 0; no XP awarded on hit.

The `[ai_spawn,_]` global trigger at `Content/scripts/npc/scripts/ai_spawn.rs2:1-3` is supposed to populate it from `npc_param(combat_xp_multiplier)` on every NPC spawn.

This was the V-PARTIAL parked at NAI-120 close and re-surfaced at NAI-121 spec-write per `nai_followups.md` NAI-120 entry.

---

## 2. Step-by-step audit

### Step 1 — Script-pack inclusion: HOLDS

`pkg/script/provider.go:42-106` — `Provider.Load` reads `script.dat`/`script.idx`, decodes per-blob, indexes via `LookupKey` at `provider.go:100-101`. `[ai_spawn,_]` registers as a global trigger (no type/category bits) at `byKey[uint32(TriggerAiSpawn)] = byKey[166]`.

`script.TriggerAiSpawn = 166` confirmed at `pkg/script/trigger.go:171`.

### Step 2 — Provider lookup: HOLDS

`Provider.GetByTrigger` (`pkg/script/provider.go:114-126`) probes 3 keys in order: type-specific, category, global. The global probe at line 125 — `p.byKey[uint32(166)]` — matches the global registration of `[ai_spawn,_]`. Lookup succeeds.

Confirmed via dispatch site `modules/world/npc_registry.go:88-99` enqueues a non-nil `*ScriptFile`.

### Step 3 — Dispatch path: HOLDS, with a tick-ordering caveat

`processNpcEventQueue` (`modules/world/npc_event_queue.go:37-49`) iterates `s.npcEventQueue`, skips `req.Npc.delayed == true`, then calls `s.runNpcScript(req.Script, req.Npc, nil, nil, nil)`. No `n.dead` gate. Pointers + ActiveNpc are set correctly via `buildNpcScriptState`.

**Tick-phase ordering (verified at HEAD `23d1a4b`, `modules/world/tick.go:35-50`):**

```
35  s.processClientsIn()
36  s.processWorldQueue()
37  s.processActiveScripts()
38  s.processPlayerTimers()
39  s.processPathing()
40  s.processInteractions()        ← combat scripts read %npc_combat_xp_multiplier here
41  s.processWalkTriggerFallbacks()
42  s.processNpcEventQueue()       ← AI_SPAWN dispatched here ("NAI-5: matches TS World.ts:356")
43  s.processNpcs()
```

**Critical observation:** `processInteractions` runs BEFORE `processNpcEventQueue`. On the very first tick after an NPC spawns:
- Server boot enqueues AI_SPAWN at NewServer time (before tick loop starts).
- Tick 1 begins with `processInteractions` — if a player's combat tick lands on the freshly spawned NPC, the combat scripts read `%npc_combat_xp_multiplier` BEFORE the AI_SPAWN script has executed.
- `processNpcEventQueue` fires AFTER, so the value is correctly written, but the read already returned `0` on the kill tick.

This is the structural lag. For long-lived NPCs, AI_SPAWN runs on tick 1 and the value is correct from tick 2 onward. For Tutorial Island giant rats killed instantly on first encounter, the read precedes the write.

### Step 4 — `npc_param` opcode handler: HOLDS

`handleNpcParam` (`pkg/script/handlers_config.go:298-312`):
```go
paramID := s.PopInt()
nt := s.Configs.NpcType(s.ActiveNpc.NpcType())
return paramLookup(s, nt.Params, paramID)
```

`paramLookup` (`pkg/script/handlers_config.go:17-49`) looks up `params[uint32(paramID)]` and falls back to `pt.DefaultInt` if absent. Mechanics work correctly.

**Open question (Scenario B):** Whether `combat_xp_multiplier` is actually present in the giant-rat's `NpcType.Params` map. If absent, `paramLookup` returns `0` regardless of dispatch ordering — i.e. the AI_SPAWN script writes `0`. Cannot disambiguate without a runtime probe or offline `npc.dat` inspection.

### Step 5 — Var write path: HOLDS

`handlePopVarn` (`pkg/script/handlers_vars.go:120-132`) post-NAI-121-T7. INT branch fires for INT-typed varn, writes via `SetNpcVarN`. No type-mismatch.

---

## 3. Root cause

Two scenarios remain on the table; the audit cannot disambiguate without a runtime probe or content-pack inspection:

**Scenario A — tick-phase ordering bug (most likely given V-PARTIAL framing):**
- `tick.go:40` (`processInteractions`) runs before `tick.go:42` (`processNpcEventQueue`).
- A player attacking the giant rat on the same tick the NPC spawned reads `%npc_combat_xp_multiplier == 0` because AI_SPAWN dispatches AFTER combat.
- TS contrast: TS `World.addNpc` (`World.ts:1284-1289` per audit recall) runs AI_SPAWN inline within `addNpc`, not via deferred queue. The varn is set before any player can interact.
- Goscape's queue-deferred dispatch is a TS-divergence (un-tracked deviation as of NAI-121 close).

**Scenario B — content/data:**
- `combat_xp_multiplier` may not be set as a param on the giant-rat's `NpcType.Params` in the compiled `npc.dat`. `paramLookup` falls back to `DefaultInt = 0`. AI_SPAWN writes `0` correctly because the source is `0`.
- Disambiguator: a single boot-time log line at `npc_registry.go:88` printing the lookup result for `paramLookup(n.typ.Params, key("combat_xp_multiplier"))` would resolve A vs B.

---

## 4. Sized recommendation for Bundle 3

**Routing: (c) carry forward to NAI-122. NAI-121 closes on Bundle 1 PRIMARY only.**

Rationale:
- The audit's Scenario A fix sketch (synchronous AI_SPAWN dispatch in `addNpc`, ~8 LOC) is technically small but is an **architecture-level policy change** (sync-vs-async dispatch). It removes the AI_SPAWN producer from `s.npcEventQueue`, which currently shares plumbing with AI_DESPAWN (`modules/world/npc_ai.go:47-58`). The asymmetry warrants a brainstorm.
- Boot-time synchronous dispatch has runtime implications: world-spawn iterates thousands of NPCs at NewServer; running their AI_SPAWN scripts synchronously before tick loop starts may surface ordering issues that the queue defers.
- Alternative fixes — e.g. reorder `tick.go` so `processNpcEventQueue` runs BEFORE `processInteractions` — also need brainstorm-level analysis (the current order has a documented `// NAI-5: matches TS World.ts:356` rationale).
- Scenario A vs B disambiguation needs a runtime probe before any code change to avoid fixing the wrong layer.

**NAI-122 brainstorm should:**
1. Add a one-shot boot probe to disambiguate Scenario A vs B (log `paramLookup` result for giant-rat `combat_xp_multiplier`).
2. If Scenario A confirmed: choose between (a) sync dispatch in `addNpc` (TS fidelity), (b) tick-phase reorder, (c) addNpc enqueues at front-of-queue + flush-before-interactions guard, etc.
3. If Scenario B confirmed: route to content-pack rebuild.

**NAI-121 close criteria (per plan §12):**
- Bundle 1 cross-package green at HEAD `23d1a4b` ✓
- Bundle 2 findings doc committed (this doc)
- User-launched smoke confirms "It's not after you." gate no longer fires on Tutorial Island giant rat (PRIMARY)
- Combat XP V-PARTIAL re-parked in `nai_followups.md` with NAI-122 reference

---

## 5. References

- TS source canonical: `LostCityRS/Engine-TS` (per `ts_source_canonical_path` memory). Audit-claimed TS line `World.ts:1284-1289` (sync AI_SPAWN dispatch) — not independently verified by controller; flag for NAI-122 brainstorm verification.
- Goscape commits in scope: 5cc1432 (plan) → cbf0d50 (T9) → 23d1a4b (Bundle 1 close-gate review fixes).
- Memory entries applied: `audit_subagent_fabrication` (controller verified tick-ordering claim at HEAD), `dispatch_correct_reach_blocked` (PRIMARY closes on smoke-bind even when SECONDARY routes forward).
