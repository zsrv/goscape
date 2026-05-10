# NAI-152 — static-obj pickup-respawn wiring (closes NAI-151 R3)

## 1. Scope

Investigation+fix sub-spec for static-obj pickup. Closes NAI-151 §9 R3 ("Pickup-respawn cycling").

**Symptom (user-reported, 2026-05-10):** static ground items spawned by NAI-151 (`f7d5554..56a29c7`) render in the Java client, but right-clicking "Take" produces only an `OPOBJ3` server-log line — **no** "Nothing interesting happens." chat message in client, **no** inventory change, **obj stays in zone**. Wholly silent on the client.

The "client wholly silent" data point is binding: it rules out the trigger-fire path in `interaction_trigger.go:707-712`, which would emit `MessageGame("Nothing interesting happens.")` when no script is found. The handler short-circuits **before** reaching the trigger fire.

Per `investigation_subspec_cadence.md`: Stage 1 risk-weighted audit (done — see §5), Stage 2 fix in 4 bundles (B1.5 → B1 → B2 → B3 → B4) with smoke gate between each, conditional Stage 3 if smoke surfaces adjacent issues.

## 2. Tech stack

Go 1.26+. Standard goscape packages: `modules/world`, `pkg/zone`, `pkg/entity`, `pkg/objtype`, `pkg/script`. No new dependencies. NodeDebug gateway probe pattern per `nodedebug_gateway_probe_pattern.md` for B1.5.

## 3. TS source

Bundle-keyed; full quotes inlined in §6 per bundle.

- **B1 / B1.5:** `Engine-TS/src/network/game/client/handler/OpObjHandler.ts:36-50` — handler that constructs the OPOBJ trigger and calls `setInteraction` **unconditionally** (no objType-op gate). The presence/absence of analogous gating in goscape's `handleOpObj` is the load-bearing question for B1.
- **B2:** `Engine-TS/src/engine/entity/Player.ts:1113-1184` `tryInteract` + `:1095` `defaultOp` + `:997` op→trigger offset; engine-side pickup behavior (whether TS has a hardcoded `obj_take` path or 100% script-driven).
- **B3:** `Engine-TS/src/engine/zone/Zone.ts:334-351` `removeObj` — Respawn vs Despawn branching: `isActive=false`, `objsCount--` only for Despawn, queue ENCLOSED OBJ_DEL for Respawn or NO_RECEIVER, FOLLOWS for private Despawn.
- **B4:** `Engine-TS/src/engine/entity/Obj.ts:27-50` `turn` + `Engine-TS/src/engine/World.ts:1501-1518` `removeObj` — `setLifeCycle(respawnDuration)` schedules respawn; `Obj.turn` re-adds the obj on the scheduled tick.

## 4. Existing surface

- `modules/world/handler_opobj.go:8-90` `handleOpObj(p, payload, op)` — shared OPOBJ1-5 impl. Per Stage 1 P1, contains 7 enumerated short-circuits (see §5.1). All silent (`sendUnsetMapFlag(p) + return nil` — no `MessageGame`).
- `modules/world/handlers_game.go:55` — `gameHandlers[200] = handleOpObj3`. Dispatch verified.
- `modules/world/interaction_trigger.go:674-712` `fireOpTriggerObj` — apObjTriggerForOp(targetOp) → `+7` → `GetByTrigger(TriggerOpObj{N}, objType, _)`. No-script path (line 707-712) emits `MessageGame("Nothing interesting happens.")`. Confirmed identical to TS (P3).
- `pkg/zone/zone.go:301-329` `RemoveObj` — slice removal gated on `Lifecycle == LifecycleDespawn` (line 303-310). Respawn-lifecycle path: clears queued events, queues OBJ_DEL ENCLOSED only if `LastLifecycleTick != currentTick`. **Does NOT set `IsActive=false`** (TS does at Zone.ts:344). **Does NOT call `obj.SetLifeCycle(respawnDuration, currentTick)`** (TS World.ts:1510 does).
- `modules/world/tick.go:607-609` — Obj-turn case is **stubbed**: `// TODO(NAI-86 D-N86-3): Obj.Turn ports later. _ = p`. Static-obj respawn cannot work without this.
- `pkg/entity/lifecycle.go:8-11` — `LifecycleForever`, `LifecycleRespawn`, `LifecycleDespawn` constants. `Lifecycle` field is currently set only at construction (`entity.NewObj`); there are NO mutation sites in the codebase (P4).
- `pkg/entity/entity.go:50` `Entity.SetLifeCycle(transitionTick, currentTick)` — sets `LastLifecycleTick = currentTick`, `LifecycleTick = currentTick + transitionTick`. Tracker registration TBC at plan-author time.
- `pkg/objtype/objtype.go` — `ObjType.RespawnRate` (TBC field name; plan-author preflight). TS `objType.respawnrate` provides per-type respawn duration; goscape mirror needed for B4.
- `pkg/script/handlers_player.go` (likely) — `obj_take` / `inv_add` opcode handlers. Existence and correctness TBC by B2 audit.

## 5. Stage 1 findings (informational; informs Stage 2)

Four parallel probes (P1-P4) executed pre-spec. Convergent findings:

### 5.1. Handler short-circuits (P1)

`modules/world/handler_opobj.go` `handleOpObj` enumerates 7 silent short-circuits, all returning via `sendUnsetMapFlag(p) + return nil`:

1. **L18-20:** `p.client == nil || p.client.server == nil` — nil-client guard
2. **L22-25:** `p.delayed && s.currentTick < p.delayedUntil` — delayed-player guard
3. **L27-29:** `len(payload) < 6` — payload underflow
4. **L35-40:** `dx > 52 || dz > 52` — viewport boundary
5. **L42-44:** `obj := s.GetObj(...); obj == nil` — zone obj lookup
6. **L46-50:** ObjType chain (`s.objTypes == nil || objId < 0 || objId >= len(Configs) || Configs[objId] == nil`)
7. **L51-53:** `len(objType.Op) < op || objType.Op[op-1] == ""` — op slot empty in cache config

**Strong prior on #7** (cross-checked against P3's TS quote of `OpObjHandler.ts:44-46`):

```ts
// TS — OpObjHandler.ts:36-50
const trigger: ServerTriggerType = ServerTriggerType.APOBJ1 + (message.op - 1);
player.setInteraction(Interaction.ENGINE, obj, trigger);
```

TS has **no** objType-op gate at handler entry — it sets the interaction unconditionally. In RS clients, ground-item "Take" is **client-side hardcoded** independent of cache `op[2]`; many obj types have empty `op[2]` in the cache. If goscape's #7 gate fires and TS's doesn't, every static-obj pickup attempt is silently dropped — exactly matching the symptom.

**Verification deferred to B1.5** — must re-read `handler_opobj.go` and `OpObjHandler.ts` end-to-end before code change (memory `audit_subagent_fabrication.md` — verify before code).

### 5.2. Field parity (P2)

Static-obj field initialization (`populateStaticObjsIntoZones` → `entity.NewObj` → `Zone.AddStaticObj`) matches dynamic-obj initialization on every lookup-relevant field: `X`, `Z`, `Type`, `ReceiverID=-1` (PublicReceiver), `IsActive=true`. **No field divergence explains GetObj returning nil.** Conclusion: short-circuit #5 is unlikely; #7 remains the leading hypothesis.

### 5.3. Trigger-dispatch chain (P3)

goscape's dispatch is TS-equivalent. `handleOpObj` → `SetInteraction(InteractionEngine, obj, op=3)` → `fireOpTriggerObj` → `apObjTriggerForOp(3)` (TriggerApObj3=33) → `+7` (TriggerOpObj3=40) → `GetByTrigger(TriggerOpObj3, objType, category)`. No-script fallback emits `MessageGame("Nothing interesting happens.")` + `ClearInteraction()` — **identical to TS**.

**Critical observation:** neither TS nor goscape has an engine-default pickup. ALL pickup is script-driven. If LostCity content lacks an `[opobj3, _]` template script (or per-type bindings), even a fixed handler will produce the "Nothing interesting happens." chat message and no inventory transfer. This is B2's domain.

### 5.4. Lifecycle transition wiring (P4)

**Two structural gaps confirmed:**

- **No `Lifecycle` mutation sites anywhere in goscape.** Field is set only at construction. TS `Obj.lifecycle` is also immutable; the lifecycle-state-machine effect comes from `IsActive` toggling + `SetLifeCycle(N)` rescheduling.
- **`tick.go:607-609` Obj-turn case is stubbed** (`TODO(NAI-86 D-N86-3): Obj.Turn ports later`). Static-obj respawn-after-N-ticks is wholly absent.
- **`pkg/zone/zone.go:301-329` `RemoveObj`** does not set `IsActive=false`, does not call `SetLifeCycle`, and only removes from `z.Objs` for Despawn — diverges from TS `Zone.removeObj` (Zone.ts:334-351).

These gaps are the B3 + B4 scope. They become observable **only after B1 unblocks the handler short-circuit** — without B1, RemoveObj is never called.

### 5.5. NAI-151 deferral

NAI-151 spec §9 R3 explicitly deferred pickup-respawn to NAI-152. This sub-spec closes that.

## 6. Stage 2 — bundle-by-bundle design

Sequenced execution. **Smoke gate between each bundle.** Bundle scope may compress (B1.5+B1) or expand (B2 may spawn child sub-specs) based on what the preceding smoke reveals.

### 6.1. B1.5 — handler short-circuit probe

**Goal:** confirm exactly which of the 7 `handleOpObj` short-circuits fires for static-obj OPOBJ3, and confirm what (if any) gating TS performs.

**Approach:** NodeDebug gateway probe pattern per `nodedebug_gateway_probe_pattern.md`. Add `s.log.Info` gateways at each short-circuit, each gated on `s.cfg.NodeDebug` (TBC config-flag name). User runs server, performs pickup, greps log for which gateway fired.

**Parallel:** read `Engine-TS/src/network/game/client/handler/OpObjHandler.ts` end-to-end (not just lines 36-50) to confirm there's no upstream guard we missed.

**Acceptance:**
- Server smoke produces a single gateway log identifying the firing short-circuit.
- TS-source read confirms presence/absence of the analogous gate.
- B1 design is locked based on the reveal.

**LOC:** ~25 (8 gateway lines + log-format + flag plumbing if needed).

**Plan-author preflight:**
- Confirm `s.cfg.NodeDebug` (or equivalent debug-flag) exists; mirror existing NodeDebug gateway sites via `rg "NodeDebug" modules/world/`.
- Confirm `s.log.Info` is the right logger reference inside `handleOpObj` (likely accessible via `p.client.server.log`).

### 6.2. B1 — handler fix

**Conditional on B1.5 result.** Most-likely path (~95% prior): short-circuit #7 (`objType.Op[op-1] == ""`) fires. Per TS `OpObjHandler.ts:36-50`, TS does NOT gate at the handler. Fix shape:

**Remove the `objType.Op[op-1] == ""` gate at `handler_opobj.go:51-53`.** The trigger fire downstream (`interaction_trigger.go`) already handles the no-script case correctly via `MessageGame("Nothing interesting happens.")`.

**Pin tests** (`modules/world/handler_opobj_test.go` extension):
1. `TestHandleOpObj_EmptyOpSlotStillFiresInteraction` — construct an obj whose `objType.Op[2] == ""`. Call `handleOpObj3`. Assert `p.targetSubject.obj == obj`, `p.targetOp == 3`, **interaction is set** (not short-circuited).
2. `TestHandleOpObj_NilObjType` (regression preserve) — `objType` index out of range still short-circuits.
3. `TestHandleOpObj_GetObjNil` (regression preserve) — `s.GetObj` returns nil still short-circuits.

**Acceptance gate (post-impl smoke):** user repeats pickup; **client now displays "Nothing interesting happens."** chat message (or pickup actually works if a script exists). The transition from "wholly silent" to "chat message visible" is the binding signal.

**LOC:** ~5 production + ~60 test.

**Plan-author preflight:**
- Re-read `handler_opobj.go` lines 8-90 to confirm exact line numbers for the gate (P1's enumeration is high-confidence but uncached at plan time).
- Re-grep `OpObjHandler.ts` and `OpObjUHandler.ts` and `OpObjTHandler.ts` in TS for any analogous gate before declaring "TS doesn't gate".
- Check whether `handleOpObjT` (opcode 138, op=6) and `handleOpObjU` (opcode 239, op=7) share the same gate via `handleOpObj` — if so, the fix benefits OPOBJT/OPOBJU too; pin those with their own smoke if observable.

### 6.3. B2 — pickup chain

**Conditional on B1 smoke.** Three observable outcomes:

- **B2-α: smoke shows pickup actually works** (item moves to inventory, clears from zone). Skip B2; advance to B3 to fix the visual-clear path.
- **B2-β: smoke shows "Nothing interesting happens." chat msg.** Pickup script chain isn't wired or content is absent. Bisect:
  - Audit `pkg/script/` for whether `[opobj3, _]` template scripts exist in the loaded script provider.
  - Cross-check LostCity content (`LostCityRS/Content/scripts/`) for the canonical pickup template — is it `[opobj3, _]`, `[opobj3, obj]`, or per-type?
  - If content registers it but goscape's script-loader skips it: fix loader.
  - If TS has an engine-default pickup we missed: read `Engine-TS/src/engine/entity/Player.ts:1113-1184` `tryInteract` end-to-end (P3 read this but may have missed an engine fallback).
- **B2-γ: smoke shows item moves to inventory but stays visually in zone.** This is B3's domain (RemoveObj for Respawn doesn't visually clear). Skip to B3.

**Design depth deferred to post-B1 smoke.** B2 may be a one-line script registration, a multi-task script-loader fix, or its own child sub-spec. Re-brainstorm if B2 exceeds ~50 LOC or surfaces architectural changes.

**Plan-author preflight (when activated):**
- `rg "TriggerOpObj3" pkg/script/` and read script-provider registration sites.
- Read `LostCityRS/Content/scripts/engine/obj/` (or canonical pickup script location) to identify the expected script header.
- Read TS `Player.ts` `tryInteract` end-to-end — flag any engine-fallback path P3 may have missed.

### 6.4. B3 — RemoveObj Respawn semantics

**Conditional on B2 producing inventory transfer.** Goal: port TS `Zone.removeObj` (Zone.ts:334-351) for Respawn-lifecycle objs.

**TS reference:**
```ts
// Zone.ts:334-351
removeObj(obj: Obj): void {
    const coord: number = CoordGrid.packZoneCoord(obj.x, obj.z);
    if (obj.lifecycle === EntityLifeCycle.DESPAWN) {
        obj.unlink();
        this.objsCount--;
    }
    this.clearQueuedEvents(obj);
    obj.isActive = false;  // ← critical: applies to BOTH lifecycles
    if (obj.lastLifecycleTick !== World.currentTick) {
        if (obj.lifecycle === EntityLifeCycle.RESPAWN || obj.receiver64 === Obj.NO_RECEIVER) {
            this.queueEvent(obj, new ZoneEvent(ZoneEventType.ENCLOSED, Obj.NO_RECEIVER, ...));
        } else if (obj.lifecycle === EntityLifeCycle.DESPAWN) {
            this.queueEvent(obj, new ZoneEvent(ZoneEventType.FOLLOWS, obj.receiver64, ...));
        }
    }
}
```

**Fix shape (`pkg/zone/zone.go:301-329`):**
- Set `obj.IsActive = false` for BOTH lifecycles after the slice-removal block (currently only happens implicitly via never being set; explicit set required).
- Keep slice-removal gated on `Lifecycle == LifecycleDespawn` (current behavior is correct: Respawn objs survive in `z.Objs` for re-add at respawn tick).
- Preserve existing OBJ_DEL queue (already correct shape).

**Pin tests** (`pkg/zone/zone_test.go` extension):
1. `TestRemoveObjRespawnSetsIsActiveFalse` — `obj := newRespawnObj`, `z.AddStaticObj(obj)`, `z.RemoveObj(obj, currentTick=10)`. Assert `obj.IsActive == false`, `len(z.Objs) == 1` (still in slice), exactly 1 OBJ_DEL ENCLOSED event queued.
2. `TestRemoveObjRespawnSameTickNoEvent` — `obj.LastLifecycleTick = 10`, `z.RemoveObj(obj, 10)`. Assert no event (preserves existing same-tick guard).
3. `TestRemoveObjDespawnUnchanged` — regression-pin existing Despawn behavior unchanged.

**Acceptance:** smoke shows item visually clears from client after pickup.

**LOC:** ~3 production + ~60 test.

**Plan-author preflight:**
- Re-read `zone.go:301-329` at HEAD to confirm exact line numbers and the current `IsActive` mutation sites (if any).
- Confirm `Zone.Events()` / `z.queuedEvents` accessor name from existing tests (likely `pkg/zone/zone_test.go:193-207` per the locs precedent).

### 6.5. B4 — Obj.turn respawn scheduling

**Conditional on B3 producing visual clear.** Goal: port TS `Obj.turn` (Obj.ts:27-50) into `tick.go:607-609`, closing the NAI-86 D-N86-3 stub. Wire `SetLifeCycle(respawnRate, currentTick)` on pickup.

**TS reference:**
```ts
// Obj.ts:27-50 (paraphrased)
turn(): void {
    if (this.lifecycleTick !== World.currentTick) return;
    if (this.lifecycle === EntityLifeCycle.RESPAWN && !this.isActive) {
        World.addObj(this, Obj.NO_RECEIVER, 0);  // respawn
    } else if (this.lifecycle === EntityLifeCycle.DESPAWN && this.isActive) {
        World.removeObj(this, 0);  // despawn
    }
}
// World.ts:1501-1518
removeObj(obj: Obj, duration: number): void {
    if (!obj.isActive) return;
    const zone: Zone = this.gameMap.getZone(obj.x, obj.z, obj.level);
    zone.removeObj(obj);
    if (duration > 0 && obj.lifecycle === EntityLifeCycle.RESPAWN) {
        obj.setLifeCycle(adjustedDuration);  // schedule respawn
    } else {
        obj.setLifeCycle(-1);
    }
}
```

**Fix shape:**

1. **`tick.go:607-609`** — replace stub with:
   ```go
   case *entitypkg.Obj:
       turnObj(s, p, s.currentTick)
   ```

2. **New `modules/world/obj_turn.go`** — port `Obj.turn` semantics:
   ```go
   func turnObj(s *Server, o *entity.Obj, now int) {
       if o.LifecycleTick != now { return }
       switch {
       case o.Lifecycle == entity.LifecycleRespawn && !o.IsActive:
           s.AddObj(o, zone.PublicReceiver)  // re-add at respawn tick
       case o.Lifecycle == entity.LifecycleDespawn && o.IsActive:
           s.RemoveObj(o, 0)
       default:
           o.SetLifeCycle(-1, now)  // untrack
       }
   }
   ```

3. **`Server.RemoveObj`** (TBC location — likely `world_zone.go`) — wire `SetLifeCycle(respawnRate, currentTick)` for Respawn objs after `zone.RemoveObj`. Read `objType.RespawnRate` (TBC field name).

4. **Pickup script-handler entry point** — when `obj_take` (or equivalent) handler runs, ensure it calls `s.RemoveObj(obj, objType.RespawnRate)` (or whatever the analogue is).

**Pin tests** (`modules/world/obj_turn_test.go` new file):
1. `TestTurnObjRespawnAddsBack` — Respawn obj with `LifecycleTick == currentTick`, `IsActive==false`. Call `turnObj`. Assert obj re-added to zone, OBJ_ADD event queued, `IsActive == true`.
2. `TestTurnObjDespawnRemoves` — Despawn obj, scheduled tick. Assert obj removed.
3. `TestTurnObjWrongTickNoOp` — `LifecycleTick != currentTick`. Assert no state change.
4. `TestServerRemoveObjSchedulesRespawn` — call `s.RemoveObj` on Respawn obj. Assert `obj.LifecycleTick == currentTick + respawnRate`.

**Acceptance:** smoke shows item reappears in zone after `objType.respawnrate` ticks.

**LOC:** ~50 production + ~120 test.

**Plan-author preflight:**
- Confirm `objType.RespawnRate` field name on `pkg/objtype/objtype.go` `ObjType`. If absent, port loader first (likely already loaded — TS `objType.respawnrate`).
- Confirm tracker registration model — does `SetLifeCycle` register the obj with the tick-loop's lifecycle tracker, or is registration manual? Mirror loc precedent at `modules/world/loc_turn.go:15-33`.
- Confirm `Server.RemoveObj` signature — does it take a duration arg today? (Likely no; B4 may need to add one.)
- Closes `TODO(NAI-86 D-N86-3)` per `retire_deviation_grep_all_comments.md` — `rg "NAI-86 D-N86-3" pkg/ modules/` to enumerate all references; retire all in B4's close commit.

## 7. Test strategy summary

Per-bundle pin tests inlined in §6. Cross-bundle regression: full `go test ./...` after each bundle. End-to-end smoke (user-driven, per `smoke_test_server_handoff.md`) is the binding gate between bundles — listed below.

| Bundle | Smoke acceptance |
|---|---|
| B1.5 | Server log identifies short-circuit by gateway name |
| B1 | "Nothing interesting happens." appears in client chat (or pickup works) — distinguishes handler-fix from script-chain |
| B2 | Item moves to inventory + cleared from `z.Objs` slice (verify via `/dump-zone` or log) |
| B3 | Item visually disappears from client after pickup |
| B4 | Item reappears in zone after `objType.respawnrate` ticks |

## 8. Cadence

Per `runescript_cadence.md` mid-band, multi-bundle. Total LOC estimate (production + test):

- B1.5: ~25 (probe gateways, removed in B1's close commit)
- B1: ~5 + ~60
- B2: TBD (1 line registration up to its own sub-spec)
- B3: ~3 + ~60
- B4: ~50 + ~120

**Total estimate: ~325-400 LOC, ranging up if B2 spawns a child sub-spec.**

**Workflow per bundle:** spec section → plan → subagent-driven impl per `execution_mode_default.md` → reviewer subagent on Sonnet (`superpowers_code_reviewer_model.md`) → `/clear` between plan and impl per `superpowers_clear_between_spec_and_impl.md` → smoke handoff per `smoke_test_server_handoff.md` → bundle close commit with `Closes memory:` trailer per `close_commit_memory_trailer.md`.

**Bundle close gate:** each bundle merges only after its smoke acceptance passes. B1 reveal may compress B1.5+B1 into one commit; B2's reveal may expand it into its own sub-spec (NAI-153). B3+B4 are predictable; B4 also closes NAI-86 D-N86-3.

## 9. TS-fidelity deviations

Bundle-tagged. Filled at impl time per `true_to_ts_gate.md`.

- **NAI-152-D1 (B1) [predicted]:** removal of `objType.Op[op-1] == ""` gate at `handler_opobj.go:51-53`. **Why:** TS `OpObjHandler.ts:36-50` sets interaction unconditionally; goscape's gate diverges and silently drops every pickup attempt for obj types with empty `op[2]`. **Risk:** OPOBJ1/2/4/5 may also be affected — confirm symmetric behavior or scope-narrow to op==3 only.
- **NAI-152-D2 (B3) [predicted]:** explicit `obj.IsActive = false` set in `Zone.RemoveObj`. **Why:** TS `Zone.ts:344` sets it for BOTH lifecycles; goscape currently never sets it, leaving stale `IsActive=true` after RemoveObj. **Risk:** any current reader of `IsActive` on a "removed" obj would have observed wrong-but-harmless value pre-fix; audit consumers via `rg "\.IsActive" pkg/ modules/` at impl time.
- **NAI-152-D3 (B4) [predicted]:** closes NAI-86 D-N86-3 stub at `tick.go:607-609`. **Why:** stub was deferred at NAI-86; now load-bearing for static-obj respawn.
- **NAI-152-D-X:** any further divergence surfaced at impl time — open before close commit.

## 10. Risk register

- **R1 (high):** B1.5 short-circuit reveal contradicts the prior (#7 `objType.Op[op-1] == ""`). **Mitigation:** B1.5 is cheap (~25 LOC, removable). If a different short-circuit fires, B1 design re-derives from the actual reveal — do not commit a fix until the smoke log is in hand. Per `audit_subagent_fabrication.md`: do not skip the probe.
- **R2 (med):** B2 reveals no `[opobj3, _]` script in LostCity content — pickup is genuinely engine-side in TS but P3 missed it. **Mitigation:** B2 plan-author re-reads `Engine-TS/src/engine/entity/Player.ts:1113-1184` end-to-end before designing the fix. May spawn child sub-spec.
- **R3 (med):** B4's `objType.RespawnRate` field is unloaded in goscape. **Mitigation:** plan-author preflight confirms field load via `rg "RespawnRate\|respawnrate" pkg/objtype/`; if unloaded, B4 includes loader port (~10 LOC).
- **R4 (low):** OPOBJT (opcode 138, op=6) and OPOBJU (opcode 239, op=7) share `handleOpObj`'s short-circuit set; B1 fix may broaden to those handlers. **Mitigation:** smoke acceptance for B1 explicitly probes whether OPOBJT/OPOBJU display analogous symptoms (per `smoke_surfaces_adjacent_divergences.md`).
- **R5 (low):** B4's `Obj.turn` port may surface NAI-86's deferred adjacent stubs (e.g., reveal-counter, lastLifecycleTick guard semantics). **Mitigation:** scope-gate at impl time per `scope_gate_prerequisite_chain.md`; route ≤30 LOC into B4, larger to NAI-152-stretch or NAI-153.
- **R6 (low):** B3's `IsActive=false` set may break a current consumer that depends on `IsActive` remaining `true` after RemoveObj. **Mitigation:** plan-author preflight grep all `\.IsActive` reader sites for objs (per `enumerate_all_sites.md`).
- **R7 (low):** Smoke environment unavailable — Java client at `LostCityRS/Client-Java` not launchable from sandbox per `smoke_test_server_handoff.md`. **Mitigation:** smoke handoff to user at each bundle gate; do not advance bundles without human confirmation.

## 11. Out of scope / non-goals

- **OPLOC pickup parity** (locs are not picked up; "interact" is the relevant verb, already wired).
- **NPC-on-obj interaction (AI_OPOBJ chain).** Different code path; unaffected by this fix.
- **Reveal-counter / tradeability gating on respawn** (`pkg/zone/zone.go:323` `TODO(beyond-4b)` — separate sub-spec).
- **Per-zone obj cap (TS `OBJS = 129`)** — `pkg/zone/zone.go:251` `TODO(beyond-4b)`.
- **Private-receiver pickup semantics** — current scope is public-receiver (NAI-151's only spawn type). If B2/B3 surface private-pickup divergences, route to NAI-152-stretch only if ≤30 LOC.
- **Inventory UI refresh post-pickup** — assumed already wired by existing OPHELD/inv listener infrastructure; B2 surfaces if not.

## 12. Cascade attribution

Closes the user-reported "static-obj pickup wholly silent" symptom (2026-05-10 conversation). NAI-151 §9 R3 explicitly opened this sub-spec slot.

Symptom-cause chain (predicted, B1.5 confirms):

```
NAI-151 spawns static obj into z.Objs (LifecycleRespawn, IsActive=true)
  → client renders via writeFullFollows (works)
  → user right-clicks "Take" → client sends OPOBJ3
  → handleOpObj receives, short-circuits at L51-53 (objType.Op[2] == "")
  → no MessageGame, no SetInteraction, no trigger fire
  → client wholly silent
```

**Predicted fix dependency graph:**
```
B1 (handler short-circuit) — load-bearing; unblocks all downstream
  └── B2 (script chain or engine fallback) — load-bearing for inventory transfer
       └── B3 (RemoveObj Respawn semantics) — load-bearing for visual clear
            └── B4 (Obj.turn respawn) — load-bearing for re-spawn cycling
```

Each downstream bundle is observable only after its predecessor's smoke. Bundle-by-bundle smoke is the binding cascade-attribution mechanism per `cascade_theory_smoke_binding.md`.
