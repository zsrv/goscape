# NAI-122 Bundle 0 — Static disambiguation findings

**Date:** 2026-05-07
**Scope:** Controller pre-flight (no production code change). Disambiguates Scenario A vs B for the V-PARTIAL parked at NAI-120 / NAI-121 and verifies the TS dispatch shape claimed by the NAI-121 audit.

---

## 1. Static probe output (Task B0.1)

Throwaway probe ran against `data/pack/server/` (loaded via `objtype.LoadParams` and `objtype.LoadNPCTypes`). Verbatim `t.Logf` output:

```
ParamType combat_xp_multiplier: id=103 Type=105 DefaultInt=1000
--- NPCs with 'rat' in name ---
pirate2 (id=183) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
giantrat1 (id=87) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
rat (id=47) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
pirate_aggressive (id=184) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
giantrat (id=86) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
blessed_giantrat (id=978) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
curator (id=646) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
dungeon_rat (id=88) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
lady_pirate (id=185) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
dragonslayer_giantrat (id=748) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
newbiegiantrat (id=950) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
clocktower_rat (id=224) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
pirate1 (id=182) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
witchrat (id=901) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
pirate_guard (id=799) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=1000
--- distribution across all 1017 NpcTypes: combat_xp_multiplier PRESENT=40 ABSENT=977 ---
[sample] macro_shade5 (id=429) Params[combat_xp_multiplier]: PRESENT value=25
```

(Plan-spec'd name `giant_rat` was not registered — actual content names use `giantrat`, `giantrat1`, `dungeon_rat`, etc. Probe broadened to enumerate every "rat" name.)

### Probe interpretation

- ParamType `combat_xp_multiplier` IS registered (Scenario B variant 1 ruled out).
- `DefaultInt = 1000` — non-zero. When AI_SPAWN's `npc_param(combat_xp_multiplier)` runs against any rat NPC, `paramLookup` returns 1000 (the default), and `[ai_spawn,_]` writes 1000 into the `%npc_combat_xp_multiplier` varn.
- The observed runtime symptom is the varn reading **0** in combat. 0 ≠ 1000 (the value the AI_SPAWN script would write if it ran).
- Therefore the AI_SPAWN script has not run before combat reads the varn. The varn's zero-init value (0) is what combat sees. Scenario B variant 2 ruled out.
- **Scenario A confirmed:** engine has a dispatch-ordering bug. The AI_SPAWN dispatch is structurally lagged behind the first combat read.

---

## 2. TS source verification (Task B0.2)

### World.addNpc neighborhood

File: `LostCityRS/Engine-TS/src/engine/World.ts:1258-1294`

```
1258    addNpc(npc: Npc, duration: number, firstSpawn: boolean = true): void {
1259        if (firstSpawn) {
1260            rsbuf.addNpc(npc.nid, npc.type);
1261            this.npcs.set(npc.nid, npc);
1262        }
1263
1264        npc.x = npc.startX;
1265        npc.z = npc.startZ;
1266        npc.isActive = true;
1267
1268        const zone = this.gameMap.getZone(npc.x, npc.z, npc.level);
1269        zone.enter(npc);
1270
1271        switch (npc.blockWalk) {
1272            case BlockWalk.NPC:
1273                changeNpcCollision(npc.width, npc.x, npc.z, npc.level, true);
1274                break;
1275            case BlockWalk.ALL:
1276                changeNpcCollision(npc.width, npc.x, npc.z, npc.level, true);
1277                changePlayerCollision(npc.width, npc.x, npc.z, npc.level, true);
1278                break;
1279        }
1280
1281        npc.resetEntity(true);
1282        npc.playAnimation(-1, 0);
1283
1284        // Queue spawn trigger
1285        const type = NpcType.get(npc.type);
1286        const script = ScriptProvider.getByTrigger(ServerTriggerType.AI_SPAWN, type.id, type.category);
1287        if (script) {
1288            this.npcEventQueue.addTail(new NpcEventRequest(NpcEventType.SPAWN, script, npc));
1289        }
1290
1291        if (duration > -1) {
1292            npc.setLifeCycle(duration);
1293            }
1294    }
```

### npcEventQueue field declaration

File: `LostCityRS/Engine-TS/src/engine/World.ts:156`

```
156    readonly npcEventQueue: LinkList<NpcEventRequest> = new LinkList();
```

Single unified queue — both `NpcEventType.SPAWN` and `NpcEventType.DESPAWN` share it. **No split-queue in TS.**

### npcEventQueue drain site (TS tick phase)

File: `LostCityRS/Engine-TS/src/engine/World.ts:340-376` (relevant lines):

```
340            const start: number = Date.now();
341            const drift: number = Math.max(0, start - this.nextTick);
342
343            // world processing
344            // - world queue
345            // - npc hunt
346            this.processWorld();
347
348            // client input
349            // - calculate afk event readiness
350            // - process packets
351            // - process pathfinding/following request
352            // - client input tracking
353            this.processClientsIn();
354
355            // Spawn triggers, despawn triggers
356            this.processNpcEventQueue();
357
358            // npc processing (if npc is not busy)
359            ...
365            this.processNpcs();
366
367            // player processing
368            // - primary queue
369            // - weak queue
370            // - timers
371            // - soft timers
372            // - engine queue
373            // - interactions          ← combat reads npc varns HERE
374            // - movement
375            // - close interface if attempting to logout
376            this.processPlayers();
```

### TS shape determination

TS uses **unified queue** + **drain BEFORE player interactions**:
- Single `npcEventQueue` carries both SPAWN and DESPAWN events.
- `processNpcEventQueue()` runs at tick line 356, immediately after `processClientsIn()`.
- `processPlayers()` (which dispatches combat / interaction scripts) runs at line 376 — AFTER the queue drain.
- Net: AI_SPAWN scripts execute and populate npc varns BEFORE any same-tick combat read.

Neither plan path (a) "sync-inline" nor plan path (c) "split-queue + pre-flush" matches TS. The TS shape is **(b): unified queue, ordered EARLIER in the tick than goscape's**.

---

## 3. Outcome lock

Selected fix shape:
- [ ] (a) Sync dispatch in `addNpc` — TS-(a) sync-inline. **REJECTED**: TS does not do this.
- [ ] (c) Split-queue + pre-flush — TS-(c) deferred-but-pre-flush with split. **REJECTED**: TS uses a unified queue.
- [x] **(b) Tick-phase reorder of the unified queue — TS-faithful, NOT enumerated in the plan as a path option but matches TS exactly.** **DEVIATION-NAI-122-D3 declared (see §4).**

### Justification

TS's `World.addNpc` (line 1284-1289) queues AI_SPAWN to `npcEventQueue` exactly like goscape's `addNpc` (npc_registry.go:88-99). The producer side is already TS-faithful. TS's drain-side, however, runs **before** player interactions (TS line 356 vs line 376), whereas goscape's drain runs **after** player interactions (tick.go:42 after tick.go:40 `processInteractions`). The minimum-diff TS-faithful fix is to move `s.processNpcEventQueue()` earlier in the tick — before `s.processInteractions()` — so AI_SPAWN scripts populate npc varns before combat reads them.

### Despawn-timing correctness check

Despawn enqueues happen inside `Npc.turn()` (called by `processNpcs()` at tick.go:43) per `modules/world/npc_ai.go:47-58`. Since `processNpcs` runs AFTER `processNpcEventQueue` either way, despawn requests enqueued at tick N are processed at tick N+1's drain. The same is true in TS (despawn enqueues at line 365 `processNpcs` are drained at line 356 of the next tick). The reorder preserves despawn timing identically.

### AI_SPAWN reentrancy + boot-storm pre-flight (controller, supplements Bundle 1 audit B1.1)

- `s.addNpc` call sites grepped via `rg -n "s\.addNpc\(|\.addNpc\(" modules/world/`:
  - Production: `modules/world/server.go:312` (boot init), `modules/world/npc.go:412` (revertType heavy path called from `Npc.turn()`).
  - Tests only otherwise.
- `OpNpcAdd` opcode (constant `pkg/script/opcode.go:237`) has NO handler implementation — only the opcode constant + String() name exist. No script reentry path.
- Path (b) does not run AI_SPAWN scripts during `addNpc` at all (still queues), so boot-time storm risk is moot for path (b). Boot's queued AI_SPAWN events drain at tick 1's reordered `processNpcEventQueue`.

### Content-script hazard pre-flight

Per plan B1.1 Step 3, content `[ai_spawn,_]` scripts shouldn't depend on tick/phase state. Path (b) runs them at the same tick-phase TS does, so even if a content script had hidden phase dependencies, goscape would now mirror TS. No hazards apply.

---

## 4. Refutations of NAI-121 audit claims

NAI-121 Bundle 2 audit (`docs/superpowers/investigations/2026-05-07-nai-121-vpartial-findings.md` §3) cited `World.ts:1284-1289` as TS sync-inline AI_SPAWN dispatch. Verified at HEAD:

- [x] Line range matches: **YES** — `World.ts:1284-1289` is the AI_SPAWN block.
- [ ] Shape matches: **NO** — TS uses `npcEventQueue.addTail(...)` at line 1288, not synchronous `runScript`/`executeScript`.

**DEVIATION-NAI-122-D3 declared:** NAI-121 audit claim refuted. TS shape is unified-queue with earlier drain phase (path (b)), NOT sync-inline (path (a)). Plan path (a) and path (c) both rejected; path (b) introduced as the actual TS-faithful fix.

Per `audit_subagent_fabrication`: this is the controller catching a fabricated/misframed shape claim before code change. Memory entry validates the controller pre-flight protocol.

---

## 5. Bundle 1 routing (path (b))

Plan's Bundle 1 path (a) and path (c) are both inapplicable. Replacement Bundle 1 (path (b)):

- **B1.1b: Failing test** — append `TestAddNpc_FreshSpawn_AiSpawnRunsBeforeInteractions` to `modules/world/npc_event_queue_test.go` (or `npc_registry_test.go`). Test seeds an NPC with a queued AI_SPAWN that mutates a varn, then drives one tick of `runTickLoopWithRate` (or just calls the relevant phases in order), and asserts the varn is populated before any `processInteractions` invocation observes it. (Implementation: a queue-state pin — assert `npcEventQueue` is empty and the varn has been written by the time `processInteractions` would run.)
- **B1.2b: Implementation** — single-line move of `s.processNpcEventQueue()` from `tick.go:42` to between `s.processWorldQueue()` (line 36) and `s.processActiveScripts()` (line 37). Update inline comments at both old and new sites to cite TS line 356 and DEVIATION-NAI-122-D3.
- **B1.3b: Cross-package green** — `go test ./...`, `go vet ./...`, `go build ./...`.
- **B1.4b: Sonnet code-reviewer pass** per `superpowers_code_reviewer_model`.
- **No NpcEventType retirement** (path (a)'s B1.4 is moot — `NpcEventSpawn` keeps its producer in path (b)).

LOC budget: ~5 LOC of code (+ test). This stays well within the in-scope-stretch threshold even if smoke surfaces additional residuals.

---

## 6. References

- TS canonical: `LostCityRS/Engine-TS/src/engine/World.ts`. Lines verified: 156 (queue field), 340-376 (tick phase order), 1258-1294 (addNpc).
- Goscape commits in scope: `a17ed5d` (NAI-121 close) → `6321659` (NAI-122 spec) → `35c85b0` (NAI-122 plan).
- Memory entries applied: `controller_preflight` (30-second grep+Read pass), `audit_subagent_fabrication` (TS-shape claim refuted), `verify_implementer_claims` (probe + TS source independently re-verified at HEAD), `dispatch_order_audit_blind_spot` (per-writer correctness ≠ wire dispatch order — generalized here to per-phase queue dispatch order).
