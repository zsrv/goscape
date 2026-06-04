# TS-Parity Fresh Audit (2026-05-28) — Deviation Ledger

**Subject:** goscape `146e759d`
**Reference (source of truth):** Engine-TS `e1dea19f`

This is a fresh, from-scratch TS→Go parity audit. **TypeScript is the source of truth.** This audit *distrusted every prior "fixed", "confirmed-exception", and "by design" claim* and re-verified each against the frozen TS reference. Findings carrying an adversarial verdict (`verdict.real === false`, or a `corrected_severity`) have had that verdict applied: refuted findings are moved to the **Refuted / false-positive** appendix; corrected severities override the original walk severity.

~58 audit units contributed, plus three coverage critics (TS-file coverage, deferred-marker sweep, extensions inventory).

---

## Running summary

### By severity (main ledger, survived verification)

| Severity | Count |
|---|---:|
| CRITICAL | 1 |
| HIGH | 6 |
| MEDIUM | 39 |
| LOW | 119 |
| **Refuted / false-positive** | 4 |
| Confirmed-exception (LOW, in-ledger) | (subset of LOW) |

> Note: Many findings carry `class = CONFIRMED-EXCEPTION` / `N-A` at LOW severity — these are verified-faithful re-checks kept in the ledger for traceability. The headline regression count (CRITICAL + HIGH + MEDIUM `DEVIATION`/`MISSING`/`COMMENT-LIE`) is what warrants action.

### By class (main ledger)

| Class | Count (approx) |
|---|---:|
| DEVIATION | ~120 |
| MISSING | ~25 |
| COMMENT-LIE | ~20 |
| CONFIRMED-EXCEPTION | ~90 |
| EXTENSION | ~25 |
| N-A | ~15 |
| STALE-DEFER | ~8 |
| WRONG-PATH | 4 |

### Severity corrections applied from adversarial verdicts

- `npc-ai-2` HIGH→MEDIUM (test pins wrong-subject contract; downgraded — the live engine bug is `npc-ai-1`).
- `world-tick-1` HIGH→MEDIUM (shutdown force-removal hang).
- `player-script-1` HIGH→MEDIUM (StopAction omits unsetMapFlag).
- `script-core-1` HIGH→MEDIUM (script error path dropped).
- `interaction-1` HIGH→HIGH (player validateTarget — held).
- `pathing-1` HIGH→HIGH (MoveRestrict enum — held).
- `rsbuf-player-1` HIGH→HIGH (writePlayers fits() — held).
- `h-loc-1` HIGH→MEDIUM (LOC_FIND no isActive filter).
- `h-player-1` HIGH→MEDIUM (operand-independent pointer gate).
- `h-npc-1` (npc-ai-1) HIGH→CRITICAL (AI trigger by wrong subject).
- `login-server-1` HIGH→HIGH (same-node duplicate login admitted — held).
- `pack-media-compiler-1` HIGH→MEDIUM (scriptVarTypeName missing 7 cases).
- `inventory-1` HIGH→LOW (Transfer absent — refuted-severity, see note).
- `h-obj-1` HIGH→LOW (OBJ_TAKEITEM overflow — refuted-severity).
- `rsbuf-player-1` confirmed HIGH (fits gate).
- Numerous LOW `corrected_severity` confirmations (cosmetic comment-lies etc.).

---

## CRITICAL

### Subsystem: NPC AI / scripting

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| npc-ai-1 / h-npc-1(cross) | `modules/world/npc_interaction_trigger.go:55,116,178,225` | `src/engine/entity/Npc.ts:865-866,989-995` | DEVIATION | **AI op/ap trigger scripts looked up by TARGET type/category, not the acting NPC's own type** | TS `tryInteract` resolves the AI trigger ONCE via `getTrigger(type)` where `type = NpcType.get(this.type)` — the **acting NPC's own type id and category** for ALL targets (`getByTrigger(trigger, this.type, type.category)`). Go's `fireAi*` helpers resolve per-target-type using the **TARGET's** type/category (`GetByTrigger(trigger, target.typeId, target.typ.category)`). Verdict CONFIRMED, severity raised to CRITICAL: every NPC-vs-NPC/loc/obj AI interaction in goscape dispatches the wrong script subject. `this.type` = acting NPC (ctor :82, changetype :431). Quote: TS `:992 return ScriptProvider.getByTrigger(trigger, this.type, type.category)`. |

---

## HIGH

### Subsystem: Pathing / movement

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| pathing-1 | `modules/world/movement_consts.go:17-24`, `modules/world/npc.go:187` | `src/engine/entity/MoveRestrict.ts:2-10`, `PathingEntity.ts:558-575` | DEVIATION | **MoveRestrict enum omits BLOCKED_NORMAL, shifting every NPC moverestrict cache value to the wrong restriction** | TS enum: NORMAL=0, BLOCKED=1, **BLOCKED_NORMAL=2**, INDOORS=3, OUTDOORS=4, NOMOVE=5, PASSTHRU=6. Go world enum drops BLOCKED_NORMAL → Normal=0,Blocked=1,Indoors=2,Outdoors=3,NoMove=4,Passthru=5. `NpcType.moverestrict` is parsed as a raw byte under *TS* enum semantics (`npctype.go` correctly mirrors BlockedNormal=2..Passthru=6), but `npc.go:187 moveRestrict: MoveRestrict(typ.MoveRestrict)` reinterprets the raw value through the *shifted* world enum → indoors-restricted NPCs become outdoors, NOMOVE becomes Passthru, etc. Verdict CONFIRMED HIGH. |

### Subsystem: Interaction

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| interaction-1 | `modules/world/interaction.go:252-264` (processInteractionPreMove) | `src/engine/entity/Player.ts:1186-1198` (validateTarget @1212) | MISSING | **Player interaction path omits validateTarget's changetype + isValid checks (only level ported)** | TS `validateTarget()` runs EVERY tick before pathing: (1) level mismatch, (2) Npc/Loc changetype (`targetSubject.type !== target.type`), (3) `target.isValid(hash64)`; any failure → `clearInteraction()+unsetMapFlag()+return`. Go `processInteractionPreMove` ports ONLY (1) (inline `tlevel != p.level`); (2) and (3) are entirely absent (no `(*Player).validateTarget`, no isValid). A player can keep interacting with a changed-type or removed target. Verdict CONFIRMED HIGH. |

### Subsystem: Login server

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| login-server-1 | `modules/login/handler.go:148-189` | `src/server/login/LoginServer.ts:271,318-327` | DEVIATION | **Same-node non-reconnect login while already logged in is admitted** | TS keys reconnect on `reconnecting && logged_in===nodeId`; any other already-logged-in case (incl. same node, `reconnecting=false`) falls to the `else if (logged_in!==null && logged_in!==0)` branch → response 3 (already logged in → client opcode 5). Go step 7: returns ALREADY_LOGGED_IN ONLY when `account.NodeID != req.NodeId`; same-node + `!Reconnecting` fires neither inner branch (`reconnect` stays false), falling through to the full-login tx → OK/NEW_PLAYER. A second concurrent same-node login is admitted where TS rejects. Verdict CONFIRMED HIGH. |

### Subsystem: rsbuf (PlayerInfo / NpcInfo wire encoder)

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| rsbuf-player-1 | `pkg/rsbuf/playerinfo.go:235-273` | rsbuf `info.rs:118-127` (+`fits` 404-408) | MISSING | **writePlayers omits the per-tracked-other byte-budget fits() gate** | Rust `write_players` gates each tracked-other's extended-info on the byte budget: run/walk emit `extend` only when `len>0 && self.fits(bytes+2, BITS_RUN/WALK, len)`; extend-only arm calls `extend()` only when `len>0 && fits(...)` else `idle()`, threading a running `bytes += len+2` accumulator. Go `writePlayers` has NO fits check and NO accumulator: it unconditionally sets `extend=1` whenever `hdLen>0` and always appends the full high-def block. Under a crowded view this can overflow the PlayerInfo byte budget (the wire packet the Java client decodes). Verdict CONFIRMED HIGH; source of truth verified against @2004scape/rsbuf v225.1.7. |

### Subsystem: Pack / compiler

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| pack-media-compiler-1 | `pkg/pack/compiler/symbols.go:175-216` | `src/cache/config/ScriptVarType.ts:28-83` | MISSING | **scriptVarTypeName omits 7 ScriptVarType cases (autoint/varp/player_uid/npc_uid/npc_stat/idkit/dbrow)** — *severity corrected HIGH→MEDIUM by verdict* | Listed here for visibility; **the verdict downgrades this to MEDIUM** (see MEDIUM table). Go's `scriptVarTypeName` has 18 cases (default→"unknown") while TS `getType` has 25; the 4 varp/varn/vars/dbcolumn enrichment passes feed through it. Real content (`antimacro` varn uses PLAYER_UID) exercises a missing case → the *compiler symbol table* gets `"unknown"`. |

---

## MEDIUM

### Subsystem: Asset module

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| entry-1 | `modules/asset/rs2cgi.go:86-95` | `src/web.ts:89-90` + `src/util/TryParse.ts:11-26` | DEVIATION | rs2.cgi query-param parsing: Go `strconv.Atoi` strict, TS `parseInt` lenient | `'1x'→TS 1/Go 0`; `'10abc'→TS 10/Go 0`; `'3.5'→TS 3/Go 0`. plugin/lowmem differ on trailing garbage / floats / 0x / leading whitespace. TS `parseInt(value)` parses leading numeric prefix; Go `strconv.Atoi` rejects any non-canonical input → default 0. |

### Subsystem: Config types (objtype / fonttype)

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| cfg-onl-2 | `pkg/objtype/objtype.go:112-115` | `src/cache/config/ObjType.ts:69-73` | DEVIATION | F2P param-autodisable fixup can panic on out-of-range/nil ParamType where TS optional-chains gracefully | TS `ParamType.get(key)?.autodisable` short-circuits on missing key (no crash, param kept). Go `if ptc.Configs[k].AutoDisable` — unguarded slice index by uint32 key + unguarded pointer deref (no bound check, no nil check). |
| cfg-var-9 | `pkg/script/handlers_npc.go:159-164` (no `categorytype.go`) | `src/cache/config/CategoryType.ts:12-66`; `ScriptValidators.ts:123` | MISSING | No runtime CategoryType decoder in Go; checkCategoryType drops the count-bound guard | TS CategoryType is a full runtime ConfigType (load/parse/get/getId/getByName/count) used by `CategoryTypeValid` (count-bound). Go has NO runtime loader — `checkCategoryType` only rejects -1, dropping `category < CategoryType.count`. An out-of-range positive id passes. (Dup-reported as `h-npc-3`.) |

### Subsystem: Media-config types

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| cfg-media-1 | `pkg/objtype/seqtype.go:65-74` | `src/cache/config/SeqType.ts:95-102` | DEVIATION | SeqType opcode-1 delay fallback adds a bounds guard TS lacks | When per-frame delay reads 0, TS unconditionally derefs `SeqFrame.instances[frames[i]].delay` (TypeError crash on OOB/empty registry → aborts whole config parse). Go wraps in `if t.frames != nil && idx>=0 && idx<len(Instances)` → silently uses delay=1 on OOB. Error-vs-silent divergence. |

### Subsystem: Data structures / DB

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| datastruct-db-1 | `pkg/zone/list.go:68-84` | `src/datastruct/DoublyLinkList.ts:73-87`, `LinkList.ts:97-111` | DEVIATION | Go `DoublyLinkList.All()` does not replicate TS `all()`'s cursor-save-before-yield removal safety | TS advances the cursor to `node.next2` BEFORE yielding and saves/restores cursor around the yield body, so a node that `unlink2()`s itself during iteration still continues from the captured successor. Go walks `for n := l.head; n != nil; n = n.next` advancing AFTER the yield; `Element.Unlink()` nils `n.next` → iteration can terminate early or skip nodes if the body unlinks the current node. |

### Subsystem: Game map

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| gamemap-1 | `pkg/gamemap/load.go:266-277` (+`modules/world/server.go:558-571`, `npc_registry.go:48`) | `src/engine/GameMap.ts:125-134` | MISSING | loadNPCs omits the NpcType-members gate — **member-only NPCs spawn on an F2P world** | TS applies TWO filters: (1) tile F2P gate (ported), AND (2) `if ((npcType.members && this.members) || !npcType.members)` — a member-only NPC only added when world is members. Go implements only the tile gate; no `Members` check on the spawned type. |

### Subsystem: Inventory

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| inventory-2 | `modules/world/inv_update.go:26-36` | `src/network/game/server/codec/UpdateInvFullEncoder.ts:13-16` | DEVIATION | UpdateInvFull size clamp diverges from TS `Math.min(capacity, width*height)` | TS UNCONDITIONALLY computes `size = min(capacity, w*h)`. Go only clamps when `grid>0 && grid<size`: when component grid is 0, TS sends size=0 but Go sends FULL capacity; when the component is not found / client/server nil, Go sends full capacity but TS calls `Component.get` unconditionally. (Encoder-side dup: `net-server-enc-2`.) |

### Subsystem: World tick

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| world-tick-1 | `modules/world/reboot.go:66-72` + `tick.go:431-453` | `src/engine/World.ts:1206-1214` | MISSING | Shutdown force-removal does not bypass CanAccess/queue gate — *severity corrected HIGH→MEDIUM* | TS `processShutdown` force-removes stuck players after `duration>=1024` via `this.removePlayer(player)` DIRECTLY (no canAccess gate). Go only sets `p.forceRemove=true` and relies on `processLogouts`' force-branch, which only bypasses the OUTER gate — a player still blocked by `!CanAccess() || engineQueue` is never force-removed, so a busy player can hang shutdown forever. Verdict: real, downgraded to MEDIUM (depends on `tickRate=0` interaction). |
| world-tick-2 | `modules/world/player_script.go:400-411` | `src/engine/entity/Player.ts:805-812` | DEVIATION | CanAccess() omits the `World.shutdown→true` relaxation | TS `canAccess()` returns true unconditionally when `World.shutdown`. Go has no shutdown branch — queues/timers/logout do not force-fire during shutdown. Doc-comment claims "goscape has no global shutdown flag" but `reboot.go:34 isPendingShutdown` / `tick.go:67` contradict it. (Dup: `player-script-12`/`interaction-14` mark the same omission CONFIRMED-EXCEPTION; this unit flags it as a live deviation.) |
| world-tick-3 | `modules/world/tick.go:211-368` + `server.go:948-1009` | `src/engine/World.ts:202-204,882-889` | MISSING | shutdownSoon login-rejection window (final 50 ticks) not implemented | TS `get shutdownSoon` (`shutdownTick != -1 && currentTick >= shutdownTick - 50`) rejects any login via `forceLogout(player, 14)`. Go has no `shutdownSoon` concept; logins are admitted during pre-shutdown. |
| world-tick-4 | `modules/world/reboot.go:48-86` | `src/engine/World.ts:1222-1225` | MISSING | Shutdown tick-rate acceleration (`tickRate=0` after duration>2) is missing | TS sets `tickRate=0` once `duration>2` so remaining shutdown ticks run as fast as possible (reach 1024-tick force-removal in seconds). Go never mutates `s.tickRate` in `processShutdown` → shutdown drain runs at normal rate. |
| world-tick-5 | `modules/world/tick.go:912-916` | `src/engine/World.ts:681-690` | MISSING | processNpcs lacks per-NPC panic recovery (TS catches and removeNpc) | TS wraps each `npc.turn()` in try/catch → `removeNpc(npc,-1)` on error, isolating one bad NPC. Go calls `n.turn(s)` with NO recover, so a panic propagates up the tick goroutine. (Dup-asserted by `npc-ai-8` comment-lie: the "tick_recovery covers it" claim is false.) |

### Subsystem: World ops

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| world-ops-1 | `modules/world/server_broadcast.go:8-17` | `src/engine/World.ts:1803-1811` | MISSING | BroadcastMes does not word-wrap or newline-split; TS uses wrappedMessageGame | TS splits on `\n` and feeds each segment to `wrappedMessageGame` → `FontType.get(1).split(mes, 456)` (one MESSAGE_GAME per wrapped line). Go calls `MessageGame(msg)` directly — no split, no wrap. No `WrappedMessageGame` exists in goscape. (Test `world-ops-11` pins the divergent non-wrapping contract.) |
| world-ops-2 | `modules/world/server.go:1218-1247` (removePlayerInternal) | `src/engine/World.ts:1601` | MISSING | Player NPC-collision footprint not cleared on removePlayer | TS `removePlayer` unconditionally calls `changeNpcCollision(width,x,z,level,false)` on logout. Go's `removePlayerInternal` does zone-leave + rsbuf-remove + slot cleanup but never clears the NPC-block collision flag set by `SetVisibility` → stale collision left at the logout tile. |
| world-ops-3 | `modules/world/npc_registry.go:72-74,199-201` | `src/engine/World.ts:1258-1262,1312-1315` | DEVIATION | rsbuf.AddNpc/RemoveNpc not gated on firstSpawn/DESPAWN — re-runs on every RESPAWN cycle | TS calls `rsbuf.addNpc` ONLY inside `if (firstSpawn)` and `rsbuf.removeNpc` ONLY in the DESPAWN branch (so a RESPAWN npc keeps its rsbuf registration across death/respawn). Go places `AddNpc` outside firstSpawn (runs on every addNpc incl. revertType) and `RemoveNpc` unconditionally. |

### Subsystem: Player core

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| player-core-1 | `modules/world/player_script.go:654-658` | `src/engine/entity/Player.ts:1898-1900`; `PathingEntity.ts:321-333` | DEVIATION | FaceSquare omits the faceAngle update that TS focus(client=true) performs | TS `faceSquare` → `focus(...,true)` ALWAYS sets `faceAngleX/Z` (persistent resting facing) AND (client=true) `faceSquareX/Z`+coordmask. Go `FaceSquare` sets ONLY `faceSquareX/Z`+MaskFaceCoord, never `faceAngle`. (Dup: `pathing-4` reports the same on the live P_FACESQUARE path.) |
| player-core-2 | `modules/world/player_run.go:38-41` | `src/engine/entity/Player.ts:690-693` | DEVIATION | updateEnergy drain truncates kg before float math (integer vs float weightKg) | TS `weightKg = this.runweight / 1000` (FLOAT) → clamp → `loss = (67 + 67*clampWeight/64)|0`. Go `weightKg := p.runweight / 1000` (INTEGER division, runweight is int) discards fractional kilogram before the loss formula → Go drains slightly less energy for any non-whole-kg weight. |
| player-core-3 | `modules/world/player_script.go:865-882` | `src/engine/entity/Player.ts:1741-1749` | DEVIATION | AddXP silently reduces XP on negative input where TS throws | TS `addXp` begins `if (xp<0) throw`. Go omits the throw: negative xp → `next := min(stats+xp, MaxXP)` (REDUCED), clamped ≥0. Go silently lowers stored XP where TS aborts before mutation. (`player-core-4` is the matching COMMENT-LIE.) |

### Subsystem: Player scripting

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| player-script-1 | `modules/world/player_script.go:1258-1264` | `src/engine/entity/Player.ts:944-947,2169-2172` | MISSING | StopAction omits unsetMapFlag() — walk queue NOT cleared, no UnsetMapFlag packet (contradicts its own comment) — *severity corrected HIGH→MEDIUM* | TS `stopAction()` = `clearPendingAction()` THEN `unsetMapFlag()` (= `clearWaypoints()` + write `UnsetMapFlag`). Go `StopAction()` = `ClearInteraction()` + `ClearPendingAction()` and never clears waypoints or sends UnsetMapFlag, despite a docstring claiming the walk queue is "preserved" by design. P_STOPACTION and OPLOC/OPNPC/OPOBJ terminals call it expecting the in-flight walk to stop. |
| player-script-2 | `modules/world/tick.go:572-643` | `src/engine/entity/Player.ts:854-906` | DEVIATION | Weak queue no longer a separate second pass — interleaved by insertion order | TS `processQueues()` runs the primary queue (NORMAL/STRONG/LONG) to completion THEN `processWeakQueue()` (all primary before any weak). Go unifies WEAK/NORMAL/STRONG/LONG into one `p.queue` slice drained in insertion order → a weak entry queued before a normal one fires first, reversing TS ordering. (Dup: `world-tick-9`.) |
| player-script-3 | `modules/world/tick.go:705-766` | `src/engine/World.ts:718-723` + `Player.ts:925-941` | DEVIATION | Timers fired in one id-sorted pass instead of TS's NORMAL-pass-then-SOFT-pass | TS calls `processTimers(NORMAL)` then `processTimers(SOFT)` (all ready NORMAL before any SOFT). Go iterates all timer ids ascending in ONE pass → a SOFT timer with a lower scriptID fires before a NORMAL with a higher id; even within one type Go uses id-sort vs TS Map-insertion order. |
| player-script-7 | `modules/world/player_script.go:1274-1282` | `src/engine/entity/Player.ts:950-953` + `PathingEntity.ts:550-556` | DEVIATION | ClearPendingAction does a partial interaction-clear (no apRange/apRangeCalled/targetSubject reset) | TS `clearPendingAction()` = `clearInteraction()` + `closeModal()`; `clearInteraction()` resets target/targetOp/targetSubject/apRange=10/apRangeCalled=false. Go inlines only interactionKind/target/targetOp + `CloseModal(true)` — does NOT reset apRange, apRangeCalled, targetSubject. (See `interaction-4` for the dedicated ClearInteraction divergence.) |

### Subsystem: Player net

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| player-net-1 / net-client-prot-2(dup) | `modules/world/player.go:1163-1169` + `player_post_decode.go` | `src/engine/entity/NetworkPlayer.ts:55-57` | MISSING | processIn does not reset userPath at decode start — stale move-path leaks into post-decode | TS `decodeIn()` unconditionally `this.userPath = []` at the top of every tick, so post-decode `userPath.length>0` is true ONLY if a MoveClick arrived this tick. Go resets `opcalled`/`decodedThisTick` but NEVER `userPath`; a compensating `if !p.decodedThisTick { return }` guard exists but the stale path remains a structural divergence. |
| player-net-2 | `modules/world/login_resync.go:57-60` | `src/engine/entity/Player.ts:543 + 741` | DEVIATION | onReconnect calls CloseModal(false) but TS onReconnect calls closeModal()=closeModal(true) — weak queue not cleared on reconnect (+ comment-lie about the parameter) | TS `Player.onReconnect` calls `this.closeModal()` (default `clearWeakQueue=true`) → weak queue cleared. Go calls `p.CloseModal(false)` → weak queue NOT cleared; the param is the exact clearWeakQueue equivalent so passing false is a behavioral change, and the comment misdescribes it. |
| player-net-3 | `modules/world/login_resync.go:91` | `src/engine/entity/Player.ts:554-555` | MISSING | onReconnect omits `masks |= APPEARANCE` resync | TS onReconnect sets BOTH `masks |= entitymask` (face_entity) AND `masks |= APPEARANCE`. Go only does `p.masks |= p.entitymask`; the APPEARANCE bit is missing → appearance not re-flagged for tracked observers on reconnect. |
| player-net-5 | `modules/world/player.go:994` | `src/engine/entity/NetworkPlayer.ts:330` | DEVIATION | updateStats run-energy change-detection: Go `floor(runenergy/100)` vs TS `floor(runenergy)/100` — Go emits UpdateRunEnergy far less often | TS operator precedence makes the guard `(floor(runenergy))/100` (float divide) — likely a TS bug, but it makes TS emit on essentially every integral energy change. Go uses integer `runenergy/100 != lastRunEnergy/100` → only emits per 100-unit bucket boundary. Observable wire-frequency divergence. |
| player-net-7 | `modules/world/player.go:1187-1194` | `src/engine/entity/NetworkPlayer.ts:143-152` | DEVIATION | Inbound USER_EVENT rate-limit counting ignores handler success | TS: `success = handler.handle() ?? false; if (success && USER_EVENT) userLimit++; else if (RESTRICTED) restrictedLimit++; else clientLimit++`. A USER_EVENT whose handler returns false counts against the larger CLIENT limit (20), NOT the USER limit (5). Go always counts a USER_EVENT against userLimit regardless of handler outcome. (Dup: `net-client-prot-1`.) |

### Subsystem: NPC core / AI

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| npc-core-1 | `modules/world/npc_registry.go:121-179` | `src/engine/entity/Npc.ts:307` (resetEntity(true) 280-317) | MISSING | resetEntityForRespawn omits resetDefaults() — target/targetOp/faceEntity/apRange/timerInterval not reset on respawn | TS `resetEntity(true)` calls `resetDefaults()` (clearInteraction + targetOp=defaultmode + faceEntity=-1 + hunt fields + timerInterval=type.timer). Go's `resetEntityForRespawn` (invoked from addNpc on first-spawn AND revertType respawn) never calls resetDefaults/clearInteraction and never assigns target/targetOp/faceEntity/apRange/timerInterval. Prior audit reported this MISSING; still present. |
| npc-core-2 | `modules/world/npc_interaction.go:281,85` | `src/engine/entity/Npc.ts:338-341,701` | DEVIATION | updateMovement / wanderMode read frozen n.moveRestrict; TS reads live NpcType.get(this.type).moverestrict | TS reads the CURRENT type's moverestrict every call. Go stores `moveRestrict` once in `NewNpc` and never updates on changeType/revertType → after a changeType between NOMOVE and a moving type, movement uses the stale value. (Compounds with `pathing-1` enum shift.) |
| npc-ai-3 | `modules/world/npc.go:159-228`, `npc_interaction.go:123` | `src/engine/entity/Npc.ts:69,728` | DEVIATION | nextPatrolTick not initialised to -1 → patrol NPC force-teleports to first waypoint on first patrol tick | TS `nextPatrolTick = -1` default makes the stuck-teleport guard `nextPatrolTick > -1` never fire on the first patrol cycle (NPC WALKS). Go never sets nextPatrolTick (zero-value 0) → the guard can fire on tick 0 and teleport the NPC to its first patrol point. |
| npc-ai-4 | `modules/world/npc_interaction.go:85-92` | `src/engine/entity/Npc.ts:701-703,682-691` | DEVIATION | wanderMode gated on WanderRange>0 — TS calls randomWalk unconditionally | TS rolls 1/8 and calls `randomWalk(type.wanderrange)` whenever `moverestrict!==NOMOVE` (no `>0` guard); with wanderrange=0 it re-paths an off-spawn NPC back home. Go adds `n.typ.WanderRange > 0` → a wanderrange-0 NPC pushed off spawn never re-paths home. |
| npc-ai-5 | `modules/world/npc_interaction.go:895-900` | `src/engine/entity/PathingEntity.ts:396-400` | DEVIATION | inApproachDistance footprint-overlap bail applies to ALL target types; TS bails only for PathingEntity targets | TS bails on intersect ONLY when target is a PathingEntity. Go runs `coordgrid.Intersects` for all target types incl. Loc → a player/NPC standing on a Loc footprint fails AP where TS passes. (Dup: `interaction-5`, `pathing-5`.) |

### Subsystem: NPC hunt

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| npc-hunt-1 | `modules/world/npc_hunt_entities.go:33` → `pkg/zone/zone.go:483` → `npc.go:458` | `ScriptIterators.ts:99` → `Zone.ts:399-405` → `Npc.ts:370-375` | DEVIATION | huntNpcs does not exclude DELAYED candidate NPCs | TS `getAllNpcsSafe(true)` yields only `Npc.isValid()` npcs; `Npc.isValid()` returns false when `this.delayed`. Go's `(*Npc).IsValid()==!n.dead` intentionally omits `delayed`; `huntNpcs` adds no delayed filter → a delayed neighbour NPC can be selected as a hunt target. |
| npc-hunt-2 | `modules/world/npc_hunt_entities.go:107-110` | `ScriptIterators.ts:123` → `Zone.ts:411-417` → `Obj.ts:52-62` | MISSING | huntObjs omits the obj.isValid() gate (count≥1 && isActive) — depleted/removed objs can be returned as hunt targets | TS iterates `getAllObjsSafe(true)` (count≥1 && isActive). Go iterates `zn.Objs` with ONLY a nil check; inactive/depleted objs linger in `z.Objs`. Contrast `obj_lookup.go:20-28` which DOES filter. |
| npc-hunt-3 | `modules/world/npc_hunt_entities.go:182-185` | `ScriptIterators.ts:146` → `Zone.ts:459-465` → `Entity.ts:32-34` | MISSING | huntLocs omits the loc.isValid() gate (isActive) — removed static locs can be returned as hunt targets | TS `getAllLocsSafe(true)` yields only active locs. Go iterates `zn.Locs` with ONLY a nil check; a removed static (Respawn) loc stays in `z.Locs` with `IsActive=false` → can be acquired as a HuntModeScenery target. |

### Subsystem: Pathing / movement (cont.)

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| pathing-2 | `modules/world/movement.go:41-118` | `src/engine/entity/PathingEntity.ts:134-151`, `Player.ts:655-680` | DEVIATION | resolveMovement lacks the moveSpeed==INSTANT/STATIONARY early-return | TS `processMovement` first guard returns false (no step) when `moveSpeed===STATIONARY||INSTANT`. Go `resolveMovement` has no such guard → an INSTANT player with queued waypoints (e.g. after Teleport/TeleJump set moveSpeed Instant) still steps. |
| pathing-4 | `modules/world/player_script.go:654-658` (dead dup `player_masks.go:39-43`) | `src/engine/entity/Player.ts:1898-1900`, `PathingEntity.ts:321-333` | DEVIATION | FaceSquare (live P_FACESQUARE handler) sets only faceSquareX/Z; TS faceSquare→focus(...,true) ALSO writes faceAngleX/Z | Same root as `player-core-1`; the live handler at `handlers_player.go:662` calls this. faceAngle resting-facing never updated. |
| pathing-5 | `modules/world/interaction.go:760-773` | `src/engine/entity/PathingEntity.ts:392-406` | DEVIATION | inApproachDistance applies the intersect 'underneath' exclusion to ALL target types | Dup of `interaction-5`/`npc-ai-5` on the player path. TS gates the rejection on `target instanceof PathingEntity`; Go applies it to Loc/Obj too. |

### Subsystem: Interaction (cont.)

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| interaction-2 | `modules/world/interaction.go:91-144` (SetInteraction) | `src/engine/entity/PathingEntity.ts:521-526` | MISSING | SetInteraction does not set targetSubject.typ for any target | TS sets `targetSubject.type = target.type` for Npc/Loc/Obj (else -1) before focus. Go sets only `targetSubject.com`, never `targetSubject.typ`. Loc/Obj are masked (handlers set typ after), but Npc and Player targets retain a leftover typ from a prior interaction. |
| interaction-3 | `modules/world/interaction.go:255` | `src/engine/entity/Player.ts:1214` + `2169-2172` | DEVIATION | Level-mismatch clear uses sendUnsetMapFlag (no waypoint clear) instead of bundled unsetMapFlag | TS calls `unsetMapFlag()` = `clearWaypoints()` + write. Go's level-mismatch branch calls write-only `sendUnsetMapFlag(p)` → when a target moves to another level, waypoints are NOT cleared. |
| interaction-4 | `modules/world/interaction.go:147-154` (ClearInteraction) | `src/engine/entity/PathingEntity.ts:550-556` | DEVIATION | ClearInteraction omits targetSubject reset; adds non-TS interacted/repathed resets | TS resets target/targetOp/targetSubject={-1,-1}/apRange=10/apRangeCalled=false. Go resets target/targetOp/apRange/apRangeCalled but NOT targetSubject (stale typ/com survives), and ADDS `interacted=false`/`repathed=false`. |
| interaction-5 | `modules/world/interaction.go:760-773 + 447-448` | `src/engine/entity/PathingEntity.ts:392-406` | DEVIATION | inApproachDistance applies the intersects 'underneath' exclusion to ALL target types, not just PathingEntity | Canonical dedup target for `npc-ai-5`/`pathing-5`. A source on a Loc/Obj footprint (distanceTo==0) can satisfy AP in TS but is rejected by Go. |
| interaction-6 | `modules/world/player_masks.go:96-143` + `interaction.go:336` | `src/engine/entity/PathingEntity.ts:587-588` via `Player.ts:458` at `World.ts:1138` | DEVIATION | apRangeCalled (and interacted) not reset per-tick; can stay stuck-true | TS `resetPathingEntity` (every tick) sets `interacted=false`/`apRangeCalled=false`. Go `ResetMasks` does NOT; AP-fire pre-resets apRangeCalled but a branch-1 OP fire never touches it → a stale-true apRangeCalled can suppress a later AP fire. |
| interaction-7 | `modules/world/interaction.go:231-233` | `src/engine/entity/Player.ts:1227-1240 + 1039-1042` | DEVIATION | Delayed-player short-circuit skips post-step HEAD (followOp chase recompute + followOp-exhaustion clear) TS still runs | TS gates only the interact arms on canAccess; the `if (!interacted)` block (followOp chase + queueWaypoint + followOp-exhaustion clearInteraction) runs even when delayed. Go's delayed short-circuit returns early, skipping it. |

### Subsystem: Entity base

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| entity-base-3 | `modules/world/loc_lookup.go:14-25` | `src/engine/zone/Zone.ts:259-266` (getLocsSafe :471-477), `Entity.ts:32-34` | MISSING | Server.GetLoc does not filter inactive locs — TS Zone.getLoc uses getLocsSafe (isValid==isActive) | Canonical dedup target with `h-loc-1`/`zone-sub-5`. Go `GetLoc` matches Level/X/Z/Type only; a removed RESPAWN/FOREVER loc stays in `z.Locs` with `IsActive=false` and is returned. Verdict downgraded to MEDIUM (RESPAWN+loc_del window). |
| entity-base-5 | `modules/world/obj_turn_test.go:140-157`, `loc_turn_test.go:50-62/111-135` | `Obj.ts:33-35`, `Loc.ts:55-57` | DEVIATION | loc_turn/obj_turn tests pin the off-by-one absolute-tick contract (fire at T+duration, not T+duration-1) | Tests assert despawn fires at `T+duration`; per `entity-base-1` TS fires at `T+duration-1` due to the same-tick decrement. The tests lock in the divergent behavior as the intended contract. |

### Subsystem: Script core

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| script-core-1 | `pkg/script/runner.go:76-79` | `src/engine/script/ScriptRunner.ts:170-229` | MISSING | Execute drops the entire player/NPC script-error handling path — *severity corrected HIGH→MEDIUM* | TS `execute` catch prefixes err.message with the opcode name (+'.' for protected VARP/VARN), sends the Player `wrappedMessageGame` "script error:"/"file:"/"stack backtrace:" lines walking debugFrames, and in NODE_PRODUCTION calls `logout()`+`loggingOut=true` (Player) or `removeNpc(state.self,0)` (Npc). Go does `if err := h(s); err != nil { s.Execution=Aborted; return err }` — no in-game message, no backtrace, no production logout/removeNpc. Verdict: real, MEDIUM. |
| script-core-2 | `pkg/script/state.go:509-511` via `handlers_core.go:16-27` | `src/engine/script/handlers/CoreOps.ts:194-214` | DEVIATION | GOSUB frame-stack overflow panics in Go instead of TS's graceful throw→Aborted | TS checks `if (state.fp >= 50) throw 'stack overflow'` at the TOP of the handler (caught → Aborted). Go's `GosubCall` `panic()`s on `FrameSP >= FrameCapacity`; `Execute` has no recover. (Dup: `h-core-5`.) |
| script-core-3 | `pkg/script/state.go:463-489` | `src/engine/script/ScriptState.ts:333-351` | DEVIATION | PushInt/PushString panic at StackCapacity=1024 where TS arrays grow without bound | A pathological/miscompiled script that pushes >1024 unconsumed values crashes the Go goroutine vs runs-to-completion in TS. |
| script-core-7 | `pkg/script/handlers_player.go:44-49` (requireActivePlayer) | `src/engine/script/ScriptPointer.ts:47-56`, `ScriptState.ts:187-194` | DEVIATION | require* pointer gates check only the primary slot while operand-aware activePlayer() may return the secondary | TS `checkedHandler` calls `pointerCheck(ActivePlayer[state.intOperand])` (operand 1 → ActivePlayer2). Go's gate checks only `PtrActivePlayer`/`Self` (operand-blind) while access via `s.activePlayer()` returns Self2 for operand 1 → operand-1 paths can deref a nil Self2. (Dup: `h-player-1`.) |
| h-player-1 | `pkg/script/handlers_player.go:44-74` | `src/engine/script/ScriptPointer.ts:47-56`; PlayerOps checkedHandler | DEVIATION | Player-op pointer gates are operand-INDEPENDENT but TS checkedHandler is operand-aware — *severity corrected HIGH→MEDIUM* | Canonical dedup with `script-core-7`/`h-inv-3`/`h-inv-8`. Go `requireActivePlayer`/`requireProtectedActivePlayer` check the LITERAL primary slot; handler bodies operate on `s.activePlayer()` (operand1→Self2). Verdict: real, MEDIUM. |

### Subsystem: Script handlers — player

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| h-player-2 | `pkg/script/handlers_player.go:1712-1728` | `src/engine/script/handlers/PlayerOps.ts:768-779` | MISSING | DAMAGE omits NumberNotNull validation on amount and uid | TS validates amount and uid via `check(..., NumberNotNull)`. Go validates only hitType; `amount=-1` (script null sentinel) and `uid=-1` are forwarded raw where TS aborts. |
| h-player-3 | `pkg/script/handlers_player.go:667-711,657-664` | `src/engine/script/handlers/PlayerOps.ts:440-451,239-243` | MISSING | P_TELEPORT / P_TELEJUMP / FACESQUARE skip the CoordValid range check that TS applies | TS `check(popInt, CoordValid)` (range [0, 2147483647]) throws on negative/oversized packed coord. Go calls `unpackCoord(...)` directly → a negative packed value is silently bit-masked into a garbage tile. Sibling P_WALK does call checkCoord (internal inconsistency). |
| h-player-4 | `pkg/script/handlers_player.go:609-628` | `src/engine/script/handlers/PlayerOps.ts:578-586` | DEVIATION | STAT_RANDOM adds a checkStatID abort TS lacks and uses trunc-toward-zero where TS uses Math.floor | TS does NO stat validation (OOB → NaN, continues) and uses `Math.floor` per term. Go adds `checkStatID` abort and integer division (trunc toward zero) → diverges when a numerator is negative. |

### Subsystem: Script handlers — interface

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| h-interface-1 | `modules/world/player_script.go:1123-1175` (OpenMain/Chat/Side/MainSide) | `src/engine/entity/Player.ts:1929-1941,1954-1964,1977-1987,2006-2010` | MISSING | Modal-open methods do not emit the eager IF_CLOSE wire packet TS writes when replacing an open modal | TS writes `new IfClose()` eagerly for every previously-open modal slot replaced. Go's open methods only flip bits + set refreshModal=true; encodeOut emits one IF_OPEN_* per change and never the preceding IF_CLOSE for the displaced modal. (Dup: `player-script-8`. `h-interface-2` was the comment-lie, REFUTED.) |

### Subsystem: Script handlers — npc

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| h-npc-3 | `pkg/script/handlers_npc.go:159-164,800` | `ScriptValidators.ts:123`; `NpcOps.ts:373` | DEVIATION | NPC_FINDCAT category validation incomplete — only rejects -1, no range/count bound (S7f-D3) | Dup of `cfg-var-9`. TS `CategoryTypeValid` is a full config-type validator `[0,count)`. Go `checkCategoryType` only rejects exactly -1. No CategoryType loader exists. |

### Subsystem: Script handlers — inventory

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| h-inv-1 | `pkg/script/handlers_inv.go:1401-1404` | `src/engine/script/handlers/InvOps.ts:592-596` + `Player.ts:1496` | DEVIATION | INV_MOVEITEM_UNCERT uses assureFullInsertion=false where TS uses default true | TS `invAdd` omits the 4th arg → `assureFullInsertion=true` (all-or-nothing: a non-stackable add where count>freeSlots adds NOTHING). Go passes `AddOpts{...Stackable}` → AssureFullInsertion=false (partial fill). Diverges on a near-full inventory. |
| h-inv-2 | `pkg/script/handlers_inv.go:351-365` | `src/engine/script/handlers/InvOps.ts:634-640` | MISSING | INV_TOTALCAT omits CategoryTypeValid check on category | TS does `check(category, CategoryTypeValid)` before summing (rejects -1 and OOB). Go never calls checkCategoryType → goes straight to the loop. |
| h-inv-3 | `pkg/script/handlers_inv.go:19-24,471,2157` | `InvOps.ts:57,72` + `ScriptPointer.ts:47-54` | DEVIATION | Single-player INV handlers resolve inv for Self but require/operate on operand-resolved player — split on intOperand=1 | Dup of the operand-pointer cluster (`h-player-1`/`script-core-7`/`h-inv-8`). `resolveInv` uses `s.Self` only while the gate/access can resolve Self2 on operand 1. |

### Subsystem: Script handlers — obj

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| h-obj-2 | `pkg/script/handlers_obj.go:238-246` | `ObjOps.ts:147`; `Player.ts:1496` | COMMENT-LIE | NAI-153-D3 comment understates the OBJ_TAKEITEM/performInvAdd divergence as gate-only no-ops | Verdict CONFIRMED MEDIUM. The comment claims the performInvAdd routing only adds no-op gates, but the material divergence is `assureFullInsertion=true`-vs-`false` PLUS performInvAdd's overflow-to-ground re-drop that TS bare `invAdd` does NOT do. (The behavioral finding `h-obj-1` was REFUTED to LOW — see appendix.) |
| h-obj-3 | `pkg/script/handlers_config.go:45-49` (OBJ_PARAM @`handlers_obj.go:466`, OC_PARAM @`handlers_config.go:480`) | `ObjOps.ts:99-103`; `ObjConfigOps.ts:20-24`; `ParamHelper.ts getStringParam` | DEVIATION | OBJ_PARAM / OC_PARAM push empty string for an absent string param with no default; TS pushes literal "null" | TS `getStringParam` returns `defaultValue ?? 'null'`. Go's paramLookup pushes `pt.DefaultString` (Go zero "") → pushes "" not "null". (Dup of the string-default cluster `h-config-4`/`h-loc-10`.) |

### Subsystem: Script handlers — loc

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| h-loc-1 | `modules/world/loc_lookup.go:14-25`; `script_loc_ops.go:102-108` | `Zone.ts:259-266,471-477`; `Entity.ts:32-34` | DEVIATION | LOC_FIND (GetLoc) does not filter on isActive; TS getLocsSafe does — *severity corrected HIGH→MEDIUM* | Dup of `entity-base-3`/`zone-sub-5`. Clicking the tile of a temporarily-removed static loc could pass LOC_FIND validation in Go. |
| h-loc-2 | `pkg/script/loc_ops.go:20-23`; `script_loc_ops.go:94-96` | `Zone.ts:259-266,471-477` | COMMENT-LIE | loc_ops.go GetLoc comment claims it mirrors Zone.getLoc but omits the isValid filter | Verdict CONFIRMED MEDIUM. `Zone.getLoc` iterates `getLocsSafe` which filters `isValid()`; Go's "mirrors TS Zone.getLoc" comment describes getLocsUnsafe-style matching. |
| h-loc-3 | `pkg/script/handlers_loc.go:423-449` | `LocOps.ts:19-21`; `ScriptValidators.ts:109` | DEVIATION | LOC_ADD skips the CoordValid range check that TS performs | TS `check(coord, CoordValid)` throws on out-of-range packed coord. Go goes straight to `UnpackCoord` (silent bit-mask). Sibling LOC_FIND/LOC_FINDALLZONE do call checkCoord (inconsistent). |
| h-loc-4 | `pkg/script/loc_iterator.go:57-70`; `script_loc_ops.go:85-92` | `ScriptIterators.ts:377-385`; `Zone.ts:459-465` | DEVIATION | LocIterator yields inactive locs in forward order; TS getAllLocsSafe(true) filters isValid and reverses | TS iterates reverse, isValid-filtered. Go returns every loc in append order, unfiltered → LOC_FINDNEXT returns inactive locs in a different sequence. |
| h-loc-6 | `modules/world/loc_lookup_test.go:10-21` | `Zone.ts:471-477`; `Entity.ts:32-34` | WRONG-PATH | loc_lookup_test pins the buggy GetLoc-returns-inactive-loc contract | The test appends an `IsActive=false` loc to `z.Locs` and asserts GetLoc returns it — pinning the `h-loc-1` deviation as a contract. TS would filter it. |

### Subsystem: Script handlers — server / core / config

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| h-server-1 | `pkg/script/handlers_server.go:96` | `ServerOps.ts:106` (+`CoordGrid.ts:136-138`) | DEVIATION | MOVECOORD packs the result without the 0x3fff/0x3 masks that TS CoordGrid.packCoord applies | TS uses `CoordGrid.packCoord` (masks every field). Go does raw `(level<<28)|(cx<<14)|cz` with NO masking → when a delta drives a component out of its bit-field, TS truncates within the field while Go bleeds bits into adjacent fields. `coordgrid.PackCoord` DOES mask but the handler bypasses it. |
| h-core-1 / h-config-1(dup) | `pkg/script/handlers_config.go:109-132` | `src/engine/script/handlers/EnumOps.ts:17-22` | DEVIATION | ENUM missing-key on a STRING-output enum pushes DefaultString to the string stack; TS pushes defaultInt to the INT stack | TS dispatches on the RUNTIME type of the retrieved value: a missing key → `undefined` → `typeof !== 'string'` → `pushInt(undefined ?? defaultInt)` (INT to int stack). Go dispatches on `et.OutputType==String` → pushes DefaultString to the string stack. Wrong value AND wrong stack. (`h-core-2`/`h-config-2` are the tests pinning it.) |
| h-core-3 | `pkg/script/handlers_vars.go:108-125` | `src/engine/script/handlers/CoreOps.ts:257-275` | DEVIATION | PUSH_VARS / POP_VARS always treat shared vars as INT (no string dispatch) and skip VarSharedValid | TS dispatches on `varsType.type` (STRING → World.varsString) and validates via `VarSharedValid`. Go always uses VarsInt/SetVarsInt, no type branch, no validation. (Dup: `h-config-5`.) |
| h-config-3 | `pkg/script/handlers_config.go:23-43` | `src/cache/config/ParamHelper.ts:10-24` | DEVIATION | paramLookup errors (aborts script) on param value/type mismatch where TS silently returns the default | TS `getStringParam`/`getIntParam` return the default on a runtime-type mismatch. Go returns a hard error → aborts the script. |
| h-config-4 | `pkg/script/handlers_config.go:44-49` | `ParamHelper.ts:10-16` + `ParamType.ts:62` | DEVIATION | Unset string-param default pushes "" where TS pushes "null" (string sibling of the M21 int-default fix) | TS `defaultString` defaults to null → `getStringParam` returns "null". Go leaves `DefaultString=""` (only DefaultInt was normalized to -1) → pushes "" for an unset string param. (Canonical dedup for `h-obj-3`/`h-loc-10`.) |

### Subsystem: Net (client/server protocol & encoders)

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| net-client-h-social-1 | `modules/world/handlers_game.go:1359` (parseIntOr) | `TryParse.ts:11-26`; `ClientCheatHandler.ts:160,217,...` | DEVIATION | parseIntOr rejects leading-digit-with-junk; TS tryParseInt/parseInt parses the leading digits | `::ban bob 30abc` → 60min (Go) vs 30 (TS); `::speed 25x`, `::tele 0a,50,50`, `::setvar v 5x`, `::give obj 5x`, `::slowreboot 30s` all diverge on staff cheat paths. |
| net-client-h-social-2 | `modules/world/handlers_game.go:1005,1042,1119,857,1283,1302` | `ClientCheatHandler.ts:200,217,...` | DEVIATION | SplitN lumps trailing tokens into the value slot; TS indexes the exact positional arg | `::setvar name 5 extra` → Go `sub[1]='5 extra'`→parseIntOr fails→0; TS `args[1]='5'`→5. Same for `::give`, `::ban`, `::setvarother`, `::giveother`. |
| net-client-h-social-5 | `modules/world/handler_interface.go:101` | `IdkSaveDesignHandler.ts:18-35` | DEVIATION | IdkSaveDesign skips ALL idk validation when s.idkTypes is nil (applies invalid design) | Go wraps the idk-validation loop in `if s.idkTypes != nil`; with nil registry the disable/type validation is skipped and the design applied unconditionally. TS always validates. |
| net-server-enc-3 (cross) | `pkg/io/packet/packet.go:396-406` (PJStr via WriteString) | `src/io/Packet.ts:330-337` | DEVIATION | PJStr string encoders write UTF-8 bytes whereas TS pjstr writes one byte per UTF-16 code unit | ASCII identical; for code points >0x7F (`'£'`) TS writes 1 byte (low byte of charCode) vs Go's 2 UTF-8 bytes. Affects MessageGame/IfSetText/MidiSong name. (Cross-cutting JagString issue; dup of `io-packet-5` GJStr read-side.) |

### Subsystem: I/O packet

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| io-packet-4 | `pkg/io/packet/packet.go:236-252` | `src/io/Packet.ts:267-276` | DEVIATION | GJStr no-terminator fallback keeps the final byte; TS gjstr drops it | TS loop `(b=getUint8(pos++)) !== terminator && this.pos < length`: the `&&` requires `pos<length` to enter the body, so the final byte (read when pos reaches length) is never appended. Go's not-found branch returns `Data[start:len]` including the last byte. Diverges on a non-terminated buffer. |

### Subsystem: Login server (cont.)

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| login-server-2 | `modules/login/handler.go:322-333` (setLoggedOut), `db.go:228-260` | `LoginServer.ts:477-487` | DEVIATION | PlayerForceLogout stamps account.logout_time; TS force-logout does not (arms spurious M25 safety reject) | TS `player_force_logout` sets ONLY `{logged_in:0, login_time:null}`. Go reuses `setLoggedOut` which stamps `logout_time=now` → a force-logout (e.g. p2p-on-f2p kick) before any save can cause the NEXT login to be rejected for safety. |
| login-server-3 | `modules/login/db.go:240-245` | `LoginServer.ts:438-439,484-485` | DEVIATION | setLoggedOut filters on node_id; TS clears login row by (account_id, profile) only | If the persisted `account_login.node_id` differs from the issuing node, Go's `AND node_id=?` matches zero rows and `logged_in` is NOT cleared → account stuck logged-in. TS always clears. |
| login-server-4 | `modules/login/handler.go:100,117` | `LoginServer.ts:213,233` | DEVIATION | Passwords are case-sensitive; TS lowercases before hash and compare | TS hashes/compares `password.toLowerCase()` (case-insensitive). Go hashes/compares `req.Password` verbatim. Internally consistent but diverges from TS auth and breaks cross-server account parity. |
| login-server-5 | `modules/login/handler.go:181,184,224,236` + `modules/world/server.go:1027` | `LoginServer.ts:115-124,287-290,346-347,364-367` + `World.ts:1857-1861` | DEVIATION | Safety-reject paths return gRPC DataLoss → world emits opcode 8 (offline), TS sends opcode 11 (rejected/try again) | TS `rejectLoginForSafety` → response 7 → client opcode 11. Go returns `codes.DataLoss` → world maps any RPC error to `OpLoginServerOffline` (opcode 8). The reject DECISION is faithful; the wire code surface differs. |
| login-server-6 | `modules/login/save.go:48-75` | `LoginServer.ts:126-141` (PlayerLoading.load throws) | DEVIATION | wouldResetSaveFile reads playtime by length only; TS aborts the write when the EXISTING save is corrupt | TS `PlayerLoading.load` THROWS on bad magic/version/CRC of the existing save → the new save is NOT written (fail-safe). Go's `savePlaytime` only checks length and reads int32@24 → a corrupt-but-long existing save yields garbage playtime and may allow overwriting. |
| login-server-7 | `modules/login/db.go:228-260`, `migrations/000001` | `LoginServer.ts:345` + account_login.logout_time migration | DEVIATION | M25 logout_time is per-account (global); TS logout_time is per-profile | TS stores logout_time on `account_login` (per profile+account). Go stores it on `account` (global). A player who logged out on profile 'main' then logs into a new profile 'beta' (no beta save) would be rejected for safety in Go; TS's beta row has NULL logout_time. |

### Subsystem: Friend server

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| friend-server-1 | `modules/friends/handler.go:53` → `repository.go:68-72` | `FriendServerRepository.ts:48-54` | DEVIATION | WorldConnect unconditionally resets playerCount to 0; TS repository.initializeWorld is a no-op guard on an already-initialized world | TS `initializeWorld` early-returns `if (this.playersByWorld[world]) return;` (preserves the existing player array/count). Go's `InitializeWorld` unconditionally creates a fresh `worldState{playerCount:0}`. A re-WORLD_CONNECT zeroes the count in Go. (`friend-server-2` was the related COMMENT-LIE — REFUTED.) |
| friend-server-5 | `modules/friends/handler.go:166-181` + `repository.go:543-553` | `FriendServer.ts:270-285` | DEVIATION | PrivateMessage does not abort on missing from/to account; TS executeTakeFirstOrThrow throws (PM dropped, not delivered) | TS resolves both accounts via `executeTakeFirstOrThrow` → throws on a missing account → caught → no insert, no delivery. Go stores raw username37 ints with no account existence check → logs/delivers a PM that TS would drop. |

### Subsystem: Logger / transport

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| logger-transport-3 | `modules/world/server.go:804-809`, `config.go:77` | `src/server/tcp/TcpServer.ts:19` | DEVIATION | Idle socket timeout is 5s (TCPServerReadTimeout) in Go vs 30s (setTimeout(30000)) in TS | TS sets a 30s socket idle timeout. Go applies a per-read deadline of 5s reset before every `bufr.Read` → a momentarily-stalled/slow client is dropped at 5s instead of 30s. Active clients (~600ms/tick) unaffected. |
| logger-transport-4 | `modules/world/server.go:766-779`, `server.go:1218-1247` | `TcpServer.ts:48,55,63`; `web.ts:159`; `World.ts:1606` | MISSING | Transport-lifecycle ENGINE session logs ('TCP/WS socket closed/error/timeout', 'Logged out') not emitted on disconnect | TS emits ENGINE session logs on socket close/error/timeout/WS-close and a MODERATOR 'Logged out' in `removePlayer`. Go's disconnect path emits NONE → session audit trail is missing the disconnect/logout reason. |
| logger-transport-5 | `modules/world/player_script.go:1566-1568` (AddWealthEvent) | `World.ts:2233-2263`; `WealthEventType.ts:15-24` | DEVIATION | World.addWealthEvent filtering (DROP/PICKUP min-value) and per-tick grouping (DEATH/PVP/PARTY_ROOM) not ported | TS drops filteredEventTypes below `NODE_MINIMUM_WEALTH_VALUE_EVENT` and coalesces groupedEventTypes per tick. Go's `AddWealthEvent` appends to in-memory `p.wealthLog` with no filtering/grouping. (Dup-asserted by `world-tick-8`, `tracking-6`; the NAI-162 "in-memory only" defer is STALE.) |

### Subsystem: util

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| util-1 | `modules/world/heropoints.go:66-77` (TopContributor) | `HeroPoints.ts:37-47` + `QuickSort.ts:9-36` | DEVIATION | FINDHERO loot-recipient selection uses linear first-max instead of TS RS-authentic quicksort tiebreak | TS clones the ledger and runs the RS-authentic quicksort (`b.points-a.points`, with the parity tiebreak `compare(...) < (loop_index & 1)`), returning `clone[0].hash64`. Go's `TopContributor` linear-scans taking the first strict-maximum. On ties in points, the quicksort can select a different equal-points contributor. (Dup-clean: `player-core-9`, `entity-base-8`.) |

### Subsystem: rsbuf (NPC)

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| rsbuf-npc-1 | `pkg/rsbuf/npcinfo.go:139-177` | rsbuf `info.rs:483-491` | DEVIATION | writeNpcs tracked loop omits the byte-budget fits() gate on the extend bit | Rust gates the high-def 'extend' decision on the byte budget (`len>0 && fits(bytes+1, BITS_RUN/WALK/EXTEND, len)` else idle). Go sets `extend=1` whenever `hdLen>0` with NO fits() call in writeNpcs. (NpcInfo-side sibling of `rsbuf-player-1`.) |
| rsbuf-player-2 | `pkg/rsbuf/renderer.go:50,56` + buildPayload:153-156 | `info.rs:296-346` (lowdefinition) vs `282-293` | DEVIATION | Low-def add block strips CHAT; Rust lowdefinition never strips CHAT | Rust strips CHAT only in highdefinition (self-echo). Go builds both lowDef payloads with `suppressChat=true` → a player becoming visible the same tick they chat omits CHAT from the add block. |

### Subsystem: Zone subsystem

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| zone-sub-1 | `pkg/zone/zone.go:158-172` | `src/engine/zone/Zone.ts:219-228` | MISSING | Zone.AddLoc does not call loc.Revert() before queuing LOC_ADD_CHANGE | TS `addLoc` calls `loc.revert()` so a re-added loc uses BASE render fields. Go never reverts → queues LocAddChange with (possibly still-changed) CurrentInfo. Reachable via the RESPAWN re-add path (`turnLoc` RESPAWN → `AddLoc`). (Dup: `h-loc-8`.) |
| zone-sub-4 | `modules/world/obj_lookup.go:46-54` | `Zone.ts:362-369,423-429` | DEVIATION | getObjOfReceiver lacks the isValid (count≥1 && isActive) filter that TS Zone.getObjOfReceiver applies | Used by the stack-merge path (`world_zone.go:146`). A depleted/de-activated obj lingering in `z.Objs` could be merged into. (Dup: `world-ops-7`.) |
| zone-sub-5 | `modules/world/loc_lookup.go:14-25` | `Zone.ts:259-266,471-477` | DEVIATION | GetLoc lacks the isValid/IsActive filter that TS Zone.getLoc applies | Dup of `h-loc-1`/`entity-base-3`. Reachable from the oploc handlers. |

### Subsystem: Pathfinder

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| pathfinder-1 | `pkg/pathfinder/routefinder/routefinder.go:497,925` | rsmod `pathfinder.rs` find_path_n south-to-north loop | DEVIATION | routeFindBig + routeBlockerFindBig south-to-north loop bound is srcSize+1, canonical uses srcSize-1 | Go loops `for index := 1; index < srcSize+1` for big (srcSize≥3) sources; canonical uses `1..src_size-1` (verbatim across the four cardinals). Go tests two extra tiles with FlagBlockSouthEastAndWest → can spuriously reject a legal south step for size≥3 actors. |
| pathfinder-2 | `pkg/pathfinder/routefinder/stepvalidator.go:198-208` | rsmod `step_validator.rs` is_blocked_south_east default arm | MISSING | StepValidator.isBlockedSouthEast size>2 default case omits the leading corner check | Canonical default arm starts with `if !can_move(get(x+size, z-1), BLOCK_SOUTH_EAST) { return true; }` BEFORE the loop. Go goes straight into the loop with no leading corner check; the three sibling diagonals DO have theirs → south-east is the lone omission for size≥3 actors. |
| pathfinder-3 | `pkg/pathfinder/collision/flagmap.go:131-133` | rsmod `collision.rs` is_flagged | DEVIATION | FlagMap.IsFlagged reports off-map/unallocated tiles as flagged; canonical is_flagged returns false there | Canonical returns false for an unallocated zone. Go `(Get(x,z,level) & flags) != FlagOpen` with `Get` returning `FlagNull=0x7FFFFFFF` for unallocated zones → off-map tiles report blocked (inverse of canonical). Affects RayCast/LineValidator LoS/LoW. |

### Subsystem: Pack / compiler (cont.)

| id | go_loc | ts_loc | class | title | detail/evidence |
|---|---|---|---|---|---|
| pack-media-compiler-12 | `pkg/pack/clientinterface/pack.go:63-71` | `tools/pack/interface/PackClient.ts:16-18` | DEVIATION | clientinterface BuildVerify CRC mismatch downgraded to a stderr log instead of TS's hard throw | TS gates on `Environment.BUILD_VERIFY` and THROWS ('.if checksum mismatch!') on mismatch (aborts pack). Go logs to stderr and continues writing → a corrupt interface payload would be written rather than rejected. (Deferred-marker NAI-213-D-BUILDVERIFY-INTERFACE-MAY-DIVERGE assessed UNVERIFIED.) |

---

## LOW

> The LOW table aggregates the remaining `DEVIATION` / `MISSING` / `COMMENT-LIE` / `EXTENSION` / `N-A` / `CONFIRMED-EXCEPTION` findings whose severity (after verdict) is LOW. Grouped by subsystem. Confirmed-exceptions and EXTENSIONs are included for traceability but are verified-faithful or net-additive.

### Asset module
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| entry-2 | `modules/asset/rs2cgi.go:84-85` | `TryParse.ts:11-26` | COMMENT-LIE | Comment claims tryParseIntDefault 'mirrors TS tryParseInt' but parse semantics differ (verdict CONFIRMED LOW) |
| entry-3 | `modules/asset/websocket.go:72-74` | `web.ts:124-125` | DEVIATION | WS inbound read-limit disabled when MaxPayloadBytes≤0; TS always caps at 2000 |
| entry-4 | `cmd/goscape/app/modules.go:142-155` | `web.ts:167-181`+`app.ts:42-45` | MISSING | startManagementWeb /prometheus endpoint has no port-faithful counterpart (OTel substitution) |
| entry-5 | `cmd/goscape/main.go:17-55` | `app.ts:12-28` | CONFIRMED-EXCEPTION | packAll() startup cache-pack check not reproduced (intentional restructure) |
| entry-6 | `cmd/goscape/app/app.go:126-149` | `app.ts:48-67` | DEVIATION | Shutdown maps SIGINT/SIGTERM to dskit StopAsync(), not World.rebootTimer(0); global crash-loggers absent |
| entry-7 | `modules/asset/websocket.go:43-47,81-94` | `web.ts:126-130` | EXTENSION | WS origin allowlist generalized single-string→[]string, enforced pre-upgrade |
| entry-8 | `modules/asset/handler.go:55-56` | `web.ts:63-69` | DEVIATION | .mid handler sets Content-Type octet-stream before ServeFile; TS sets none + bare 404 |
| entry-9 | `modules/asset/handler.go:36-57` | `web.ts:63-69` | EXTENSION | .mid path-traversal blocked in Go (ServeFile guard); latent TS traversal faithfully NOT reproduced |
| entry-10 | `modules/asset/rs2cgi.go:40-44` + templates | `view/client.ejs`,`view/java.ejs` | DEVIATION | rs2.cgi numeric/boolean substitutions rendered with surrounding whitespace (byte-different, parses identically) |
| logger-transport-1 | `modules/asset/websocket.go:21` | `Environment.ts:13`,`web.ts:132` | COMMENT-LIE | Doc comment cites non-existent env var WEB_CORS_ALLOWED_ORIGINS; TS gate is WEB_ALLOWED_ORIGIN (verdict CONFIRMED LOW) |
| logger-transport-2 | `modules/asset/websocket.go:84-94` | `web.ts:132` | EXTENSION | WS origin gate is a multi-entry allowlist; TS is a single string (strict superset) |

### Config types (objtype / fonttype / paramtype)
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| cfg-onl-1 | `pkg/objtype/objtype.go:222-223` | `ObjType.ts:204-205` | DEVIATION | ObjType cost (code 12) decoded unsigned `int(G4)` where TS uses signed `g4s()` (bit-31-set costs diverge) |
| cfg-onl-3 | `pkg/objtype/objtype.go:183-193` | `ObjType.ts:294-304` | DEVIATION | toCertificate renders empty name (not literal "null") when link.name absent |
| cfg-onl-4 | `pkg/objtype/objtype.go:333-334` | `ObjType.ts:151-152` | CONFIRMED-EXCEPTION | op/iop nulls represented as empty strings (project-wide convention) |
| cfg-onl-5 | `pkg/objtype/objtype.go:299-300` | `ObjType.ts:276-277` | CONFIRMED-EXCEPTION | Unrecognized config code fatal-via-error-return vs printFatalError/process.exit |
| cfg-onl-6 | `pkg/objtype/paramtype.go:12-25` | `ParamHelper.ts:26-40` | CONFIRMED-EXCEPTION | ObjType params stored uint32 but sign-extended at read (byte-identical net) |
| cfg-media-2 | `pkg/objtype/flotype.go:39-49` | `FloType.ts:12-20` | DEVIATION | FloType.load silent-on-missing in TS but LoadFloTypes returns an error when server/flo.dat absent |
| cfg-media-3 | `pkg/objtype/flotype.go:11-31` | `FloType.ts:44-101` | EXTENSION | Go FloType is a minimal worldmap-packer view (drops rgb/texture/overlay/occlude/loadJag); stream parsed identically |
| cfg-var-1 | `pkg/objtype/paramtype.go:161` | `ParamType.ts:62` | CONFIRMED-EXCEPTION | M21 ParamType.DefaultInt default = -1 (prior regression now FIXED) |
| cfg-var-2 | `varptype.go:40-50`,`varntype.go:21-28`,`varstype.go:21-28` | `VarPlayerType.ts:101-103` etc | CONFIRMED-EXCEPTION | L33 varp/varn/vars unknown-opcode = log+continue (matches TS printError) |
| cfg-var-3 | `componenttype.go:334` | `Component.ts:243` | CONFIRMED-EXCEPTION | L34 Component server overlay read via gbool (==1) not !=0 |
| cfg-var-4 | `npcmode.go:37-86` | `NpcMode.ts:147-167` | CONFIRMED-EXCEPTION | NpcMode QUEUE1..20 omitted from NpcModeMap — TS still commented out |
| cfg-var-5 | `componenttype.go:335-340` | `Component.ts:245-248` | DEVIATION | decodeExtra unknown server-stream id silently skipped (TS would TypeError-crash) |
| cfg-var-6 | `dbtableindex.go:142-167` | `DbTableIndex.ts:75-89` | DEVIATION | DbTableIndex.Find: non-indexed-column lookup returns nil silently (TS printWarning) |
| cfg-var-7 | `dbrowtype.go:67-77` | `DbRowType.ts:95-102` | DEVIATION | DbRowType.GetValue returns default on partial trailing tuple where TS returns the partial slice |
| cfg-var-8 | `fonttype.go:168-178` | `FontType.ts:123-138` | DEVIATION | FontType.StringWidth indexes drawWidth by byte vs TS by UTF-16 charCode (>255→NaN in TS) |

### Cache graphics / pixpack / sprites / packfile
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| cache-graphics-1 | `pkg/pack/graphics/pack.go:1` | `AnimBase/AnimFrame/Model/Pix.ts` | N-A | Assigned TS files are CLIENT cache-reader classes with NO Go counterpart; audited Go ports the inverse tools/pack packer |
| cache-graphics-2 | `pkg/pack/graphics/pack.go:26-32` | `tools/pack/graphics/pack.ts:12` | DEVIATION | graphics.Pack drops the shouldBuildFile(pack.ts source) self-watch arm |
| cache-graphics-3 | `pkg/pack/graphics/pack.go:26-28` | `tools/pack/graphics/pack.ts:11-34` | DEVIATION | graphics.Pack adds a no-src no-op guard TS lacks (net behavior matches) |
| cache-graphics-4 | `pkg/pixpack/convert.go:46-52` | `tools/pack/PixPack.ts:186-190` | DEVIATION (was STALE-DEFER) | ConvertImage errors on palette >255 colors instead of TS's img.quantize re-quantization (verdict CONFIRMED, LOW — TS genuinely implements quantize) |
| cache-graphics-5 | `pkg/pack/sprites/sprites.go:135-145` | `tools/pack/sprite/textures.ts:11-13` | DEVIATION | PackTexture skips missing texture IDs where TS passes undefined and reads 'undefined.png' (TS throws) |
| cache-graphics-6 | `pkg/pack/packfile.go:188-189` | `tools/pack/graphics/pack.ts:55-60` | DEVIATION | PackFile.GetByID doc-comment 'or "" if absent' inaccurate for out-of-range ids (panics) |

### Wordenc
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| wordenc-1 | `pkg/pack/wordenc/pack.go:1` | `tools/pack/chat/pack.ts:1` | WRONG-PATH | Audit-scope path pkg/pack/wordenc/wordenc.go does not exist; actual packer is pack.go |
| wordenc-2 | `encfilter.go:196` | `WordEnc.ts:191` | DEVIATION | Decode count uses unsigned G4() where TS uses signed g4s() (diverges only on corrupt cache) |
| wordenc-3 | `encfilter.go:212` | `WordEnc.ts:202` | DEVIATION | decodeBadEnc always stores an (empty) badCombos slice; TS leaves undefined when comboCount==0 |
| wordenc-4 | `encfilter.go:70` | `WordEnc.ts:37` | DEVIATION | Load/readAll error semantics differ (TS silent all-or-nothing no-op; Go partial-populate then abort) |
| wordenc-5 | `fragments.go:68` | `WordEncFragments.ts:55` | DEVIATION | isBadFragment narrows getInteger to uint16 (lossless for all reachable ≤3-char inputs) |
| wordenc-6 | `wordpack.go:73` | `WordPack.ts:44` | DEVIATION | WordPack.Pack lowercases-then-truncates; TS truncates-then-lowercases (Unicode boundary edge) |

### Data structures / DB
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| datastruct-db-2 | `repository.go:240-249` | `FriendServerRepository.ts:230-235` | DEVIATION | Friend/ignore 100-cap is profile-scoped in Go but account-wide in TS (dup: friend-server-4) |
| datastruct-db-3 | `db.go:178-219` | `LoginServer.ts:162-169,385-402` | CONFIRMED-EXCEPTION | logged_in overloaded-as-nodeId split into logged_in(bool)+node_id |
| datastruct-db-4 | `db.go:221-260` | `LoginServer.ts:429-441` | CONFIRMED-EXCEPTION | setLoggedOut stamps logout_time on account (Go) vs account_login (TS) |
| datastruct-db-5 | `login/db.go`,`friends/db.go` | `src/db/dialect/*` | CONFIRMED-EXCEPTION | DB transport differs (Kysely+Bun-SQLite vs database/sql+modernc); busy-retry/fail-soft not reproduced |
| datastruct-db-6 | `db.go:149-162`,`migrations/000001` | `db/types.ts:7-17` | DEVIATION | account.registration_date present in TS schema/insert, absent in Go (write-only in TS, never read) |
| datastruct-db-7 | `obj_delayed_queue.go:10-11,48-67` | `World.ts:157,563-573` | CONFIRMED-EXCEPTION | TS LinkList<ObjDelayedRequest> ported to a Go slice (behavior matches) |
| datastruct-db-8 | `pkg/zone/zone.go:424-431`,`grid.go:32-39` | `Zone.ts:79-100`,`ZoneGrid.ts:18-24` | EXTENSION | Zone player-enter flags grid only on 0→1 (identical idempotent bitmap) |

### Game map
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| gamemap-2 | `server.go:559-565` | `GameMap.ts:125-129` | DEVIATION | loadNPCs silently skips unknown NPC type id; TS calls printFatalError |
| gamemap-3 | `gamemap.go:160-169` | `GameMap.ts:48-61` | DEVIATION | CSV map files loaded from cacheDir/server/maps/ rather than TS BUILD_SRC_DIR/maps/ |
| gamemap-4 | `coordgrid.go:209-214` | `ZoneMap.ts:10-15` | DEVIATION | UnpackZoneIndex masks level with &0x3; TS does not (harmless) (dup: zone-sub-10) |
| gamemap-5 | `handlers_npc.go:1030` | `CoordGrid.ts:83-88` | EXTENSION | CoordGrid.euclideanSquaredDistance not ported to coordgrid (inlined at NPC-hunt site) |
| gamemap-6 | `coordgrid.go:33-35` | `CoordGrid.ts:21` | DEVIATION | coordgrid.MapSquare narrows to uint16; TS operates on number |

### Inventory
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| inventory-3 | `inv_update.go:33-35` | `UpdateInvFullEncoder.ts:15-21` | DEVIATION | UpdateInvFull adds an extra size>0xff byte clamp not present in TS (dup: net-server-enc-2) |
| inventory-4 | `inventory.go:220-222,333-335` | `Inventory.ts:158-241,243-323` | DEVIATION | Add/Remove early-return on count≤0 (TS has no such guard; count==0 TS dirties+re-writes) |
| inventory-5 | `inventory.go:80-85,182-188,200-206` | `Inventory.ts:330-334,350-357` | EXTENSION | Get/Set/Swap add bounds checks; TS get/set/swap are unguarded (validSlot folded in) |
| inventory-6 | `inventory.go` (none) | `Inventory.ts:104-134,...` | MISSING | TS helpers hasSpace/hasAny/occupiedSlotCount/itemsFiltered/validSlot/shift have no Go equivalent (itemsFiltered IS used by Player.ts:1665) |

### World tick / ops
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| world-tick-6 / world-ops-6 | `server.go:1082,1162-1170` | `World.ts:1645-1653,1703-1713` | DEVIATION | Player slot capacity off-by-one: Go allows/counts slot 2047; TS caps at 2046 |
| world-tick-7 | `tick.go` (none) | `WorldStat.ts`,`World.ts:459-499` | MISSING | WorldStat / cycleStats per-tick telemetry entirely absent |
| world-tick-8 | `player_script.go:1566-1568` | `World.ts:444-457,2233-2263` | MISSING | Per-tick wealth-transaction grouping + dispatch not ported (dup: logger-transport-5) |
| world-tick-9 | `tick.go:572-643` | `Player.ts:854-906` | DEVIATION | Unified queue fires weak entries interleaved with primary (dup: player-script-2) |
| world-tick-10 | `tick.go:128-158` | `World.ts:703-746` | DEVIATION | Per-pass (vs TS per-entity) player processing changes intra-tick cross-player ordering |
| world-tick-11 | `tick.go:67-72,81-85` | `World.ts:419-426` | CONFIRMED-EXCEPTION | L3/L4 processShutdown/savePlayers hoisted to top-of-loop (placement benign) |
| world-tick-12 / world-ops-10 / world-tick-L1 | `tick.go:138`,`obj_delayed_queue.go:48-68` | `World.ts:562-574`+`InvOps.ts:208` | CONFIRMED-EXCEPTION/DEVIATION | objDelayedQueue drained after processNpcs (1-tick HuntModeObj latency, documented) |
| world-tick-13 | `tick.go:49-58,184-193` | n/a | EXTENSION | Top-of-loop drains (rebuildResult, relayActions) + saveAllOnShutdown have no TS counterpart |
| world-tick-14 | `reboot.go:82-85` | `World.ts:1216-1220` | DEVIATION | Graceful-exit condition omits logoutRequests-empty term (architectural; goscape has no queue) |
| world-tick-15 | `tick.go:24-27` | `World.ts:131-132` | DEVIATION | TIMEOUT debug-socket override (NODE_DEBUG_SOCKET=60000) not implemented (dev-only) |
| world-ops-4 | `world_state_ops.go:149-153` | `World.ts:2035-2036` | DEVIATION | RELAY_RELOAD triggers a full source-rebuild instead of TS reload(false) |
| world-ops-5 | `world_state_ops.go:155-165` | `World.ts:2037-2038,141,143` | COMMENT-LIE | ClearLogins comment claims it mirrors loginRequests.clear() but drains newPlayers (verdict CONFIRMED LOW) |
| world-ops-8 | `server_pmid.go:25-34` | `World.ts:1638` | DEVIATION | nextPmId masks NodeID to 0xff + bitwise-OR where TS uses unmasked NodeID + addition |
| world-ops-9 | `server.go:908-914` | `World.ts:2129-2136` | DEVIATION | Login CRC check compares 9 archive CRCs element-wise rather than TS's CRC-of-CRCs (stricter) |
| world-ops-11 | `server_broadcast_test.go:8-32` | `World.ts:1803-1811` | COMMENT-LIE | BroadcastMes test pins the non-wrapping contract that contradicts TS broadcastMes |

### Player core / scripting / net
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| player-core-4 | `player_script.go:841-844` | `Player.ts:1742-1743` | COMMENT-LIE | AddXP doc falsely claims TS reduces XP on negative input (TS throws) (verdict CONFIRMED LOW) |
| player-core-5 | `player_load.go:53,111` | `PlayerLoading.ts:21-24,59-62` | DEVIATION | VerifySave/LoadSave reject version 0 where TS verify/load accept it |
| player-core-6 | `player_save.go:77-110` | `Player.ts:227-253` | DEVIATION | Save iterates inventories in sorted typeId order vs TS Map-insertion order (CRC differs from TS save) |
| player-core-7 | `player_load.go:168-182` | `PlayerLoading.ts:98-101` | DEVIATION | LoadSave applies read-side non-PERM varp scrub TS load does not |
| player-core-8 | `player.go:249-250` | `Player.ts:295-296` | EXTENSION | Player struct carries dead vars/varsString fields parallel to canonical varps |
| player-core-9 | `heropoints.go:66-77` | `HeroPoints.ts:37-47` | DEVIATION | HeroPoints findHero/TopContributor tie-break ordering differs (dup: util-1) |
| player-core-10 | `packet.go:22-24` | `Packet.ts:54-60` | DEVIATION | packet.GetCRC treats length as count not end-index (all callers offset=0; coincides) |
| player-core-11 | `player_script.go:936` | `Player.ts:1810-1813` | DEVIATION | AddXP combat-level recompute level-up-gated; TS recomputes every call (equivalent) |
| player-core-12 | `player_script_test.go:45-46,56` | `Player.ts:97-99` | COMMENT-LIE | Test comment misstates GetExpByLevel(2) as 820 (actual 830); test body correct (verdict CONFIRMED LOW) |
| player-script-4 | `pkg/script/active.go:814-819` | `PlayerOps.ts:903-918` | COMMENT-LIE | QueueCount comment claims 'non-Weak' but impl & TS count weak too (verdict CONFIRMED LOW; behavior correct) |
| player-script-5 | `player_script.go:1330-1333` | `IdkSaveDesignHandler.ts:10` | STALE-DEFER | SetAllowDesign comment 'Reader path unported' is stale — reader IS implemented (verdict CONFIRMED LOW) |
| player-script-6 | `player_script.go:1097-1103` | `Player.ts:762,...`+`ScriptProvider.ts:124-154` | DEVIATION | IF_CLOSE lookup uses GetByTriggerSpecific (no global [if_close] fallback) (content has none) |
| player-script-8 | `player_script.go:1123-1175` | `Player.ts:1928-2022` | DEVIATION | Modal open methods do not emit IF_CLOSE for the displaced modal (dup: h-interface-1) |
| player-script-9 | `player_script.go:84-91` | `Player.ts:821-830` | DEVIATION | EnqueueScriptFile does not force Delay=0 for QueueEngine entries (all callers pass 0) |
| player-script-10 | `player_interface.go:140-162` | `Player.ts:2047-2049,1953-1997` | DEVIATION | IsComponentVisible bit-gates modal-slot reads, masking TS's stale-field bug |
| player-script-11 | `player_script.go:1044-1088` | `Player.ts:741-794` | CONFIRMED-EXCEPTION | CloseModal omits TS 'if(!delayed) protect=false' clear (modeled via activeScript pointer) |
| player-script-12 | `player_script.go:400-411` | `Player.ts:805-812` | CONFIRMED-EXCEPTION | CanAccess omits World.shutdown force-true branch (no global flag) (cf. world-tick-2 which flags it live) |
| player-script-13 | `player_script.go:1215-1226` | `Player.ts:716-726` | DEVIATION | CloseTutorial clears modalStateTut bit; TS leaves it set (inert) |
| player-net-4 | `login_resync.go:64-68` | `Player.ts:545-547` | DEVIATION | onReconnect tabs resync skips tab value 0; TS writes IfSetTab for ALL 14 unconditionally (defaults -1 match) |
| player-net-6 | `player_zone.go:33-89,98-115` | `Zone.ts:141,148-164,184-195` | DEVIATION | Zone full/partial-follows header placement diverges (idempotent) (dup: zone-sub-2/-3) |
| player-net-8 | `player.go:1196-1199,505-520` | `NetworkPlayer.ts:79-82,231` | DEVIATION | WorldStat cycleStats BANDWIDTH_IN/OUT accounting omitted |
| player-net-9 | `player.go:505-520,1141` | `NetworkPlayer.ts:230` | CONFIRMED-EXCEPTION | Per-packet immediate send replaced by buffered-write + once-per-tick flush (byte-identical stream) |

### NPC core / AI / hunt
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| npc-core-3 | `npc.go:169` | `Npc.ts:62,505-525` | DEVIATION | NPC regenInterval initialized to type.RegenRate, not 0 — TS fires first regen on spawn tick (1-tick phase offset) |
| npc-core-4 | `npc_masks.go:32-35`,`handlers_npc.go:338-346` | `Npc.ts:487-494` | COMMENT-LIE | Npc.Say + handleNpcSay drop TS's empty-string guard; comment claims empty 'clears the bubble' (verdict CONFIRMED LOW) |
| npc-core-5 / npc-ai-10 | `npc_interaction.go:85-92` | `Npc.ts:682-691` | DEVIATION | wanderMode random-walk distribution differs (discrete uniform vs Math.round of continuous) |
| npc-core-6 | `npc_masks.go:169-181` | `Npc.ts:472-485` | DEVIATION | Damage coerces negative amount to 0; TS applyDamage would heal on negative |
| npc-core-7 | `npc_masks.go:210-218` | `World.ts:1152`,`PathingEntity.ts:611-614` | COMMENT-LIE | ResetMasks comment falsely claims TS fires faceEntity trailing-clear 'at tick start' (TS fires at tick end, same as Go) (verdict CONFIRMED LOW) |
| npc-core-8 | `npc.go:428-447` | `Npc.ts:1082-1091,427-449` | COMMENT-LIE | revertType re-arm comment claims 'TS gets this for free via ctor rerun'; TS never reruns ctor on revert (verdict CONFIRMED LOW) |
| npc-core-9 | `npc_ai.go:36` | `World.ts:1262-1264` | DEVIATION | Dead-respawn branch resets n.level to startLevel; TS World.addNpc does not reset level |
| npc-core-10 | `npc_ai.go:31-45` | `Npc.ts:124-128`,`World.ts:1258-1294` | DEVIATION | Dead-respawn uses revertType (removeNpc+addNpc) instead of single addNpc (net-equivalent) |
| npc-core-11 / npc-ai-11 / pathing-10 | `npc.go:185`,`npc_masks.go:259` | `Npc.ts:343-345`,`PathingEntity.ts:578` | CONFIRMED-EXCEPTION/STALE-DEFER | NPC moveSpeed never set to WALK (stays INSTANT); net-equivalent (Go updateMovement doesn't gate on moveSpeed) — pathing-10 verdict NONE |
| npc-core-12 | `npc.go:449-460` | `Npc.ts:370-375` | CONFIRMED-EXCEPTION | Npc.IsValid only !dead; TS adds !delayed (enforced externally) (cf. npc-hunt-1) |
| npc-core-13 | `npc_ai.go:49-65` | `Npc.ts:135-143` | DEVIATION | DESPAWN Events branch adds a `!n.dead` guard TS lacks (always-true) |
| npc-ai-6 | `npc_interaction_trigger.go:43-60,...` | `Npc.ts:869-882` | DEVIATION | tryInteract no-script/target-invalid paths clearInteraction; TS leaves interaction intact (returns true) |
| npc-ai-7 | `npc_ai.go:30-66` | `Npc.ts:122-185` | DEVIATION | turn() returns early after RESPAWN/revert; TS continues to hunt/regen/timer/queue/movement the same tick |
| npc-ai-8 | `npc_ai.go:46-48`,`tick.go:912-916` | `World.ts:683-690` | COMMENT-LIE | despawn comment claims 'tick_recovery covers the panic case' but processNpcs has no recover() (verdict CONFIRMED LOW; cf. world-tick-5) |
| npc-ai-9 | `npc.go:168` | `Npc.ts:96,210-214` | DEVIATION | NewNpc seeds timerInterval=int(typ.Timer) directly; TS setTimer ignores -1 (cosmetic; both ≤0 disable) |
| npc-ai-12 | `pkg/script/queue.go:55-59` | `NpcQueueRequest.ts:10-17`,`NpcOps.ts:149` | CONFIRMED-EXCEPTION | NpcQueueRequest collapses args[]+lastInt to single LastInt (args always []) |
| npc-ai-13 | `npc_player_modes.go:29-...` | `Npc.ts:748,804,817,823` | CONFIRMED-EXCEPTION | PLAYER* mode type-guards log+return instead of throw (validateTarget pre-guarantees) |
| npc-ai-14 | `npc_interaction.go:1083-1088`,`npc.go:217` | `Npc.ts:100,414` | N-A | Prior cluster D (H6): defaultMode reads stored typ.DefaultMode; comment honest — confirmed FIXED |
| npc-ai-15 | `npc_event_queue.go:49-53` | `World.ts:664-672`,`Npc.ts:139-141` | DEVIATION | processNpcEventQueue re-derives trigger from event type rather than the captured script (benign) |
| npc-hunt-4 | `npc_hunt_entities.go:88-92` | `ScriptIterators.ts:122-123`,`Zone.ts:411-417`,`zone.go:254-256` | COMMENT-LIE | huntObjs comment claims 'Zone.Objs contains only dynamic objs' — both claims false (AddStaticObj appends statics) (verdict CONFIRMED LOW) |
| npc-hunt-5 | `pkg/zone/map.go:62-80`,... | `ScriptIterators.ts:73-76` | CONFIRMED-EXCEPTION | Zone iteration order reversed vs TS HuntIterator (distribution-neutral — uniform random pick) |
| npc-hunt-6 | huntNpcs/Objs/Locs (none) | `ScriptIterators.ts:80-82,...` | CONFIRMED-EXCEPTION | TS stale-iterator throw has no Go equivalent (unreachable in synchronous model) |
| npc-hunt-7 | `inventory.go:103-111` | `Inventory.ts:136-147` | DEVIATION | checkInv quantity (GetItemCount) lacks the TS STACK_LIMIT clamp (theoretical overflow only) |
| npc-hunt-8 | `npc_hunt.go:31-39,88-92` | `Npc.ts:159-162`,`World.ts:581-585` | EXTENSION | processNpcHunt/processNpcHuntPlayers add bounds/nil-hunt defensiveness beyond TS |
| npc-hunt-9 | `npc_hunt.go:232-233` | `Npc.ts:942` | N-A | Combat-guard nil-gamemap branch (test-only; production gamemap always set) |
| npc-hunt-10 | `npc_script.go:123-128`,`player_script.go:427-432` | `Npc.ts:195-198`,`Player.ts:1706-1713` | DEVIATION | NpcVarN/Varp read raw id; TS getVar resolves via VarType.get(id).id remap (equivalent for declared ids) |
| npc-hunt-11 | `npc_hunt.go:77-96`,`tick.go:126-127` | `World.ts:577-589` | CONFIRMED-EXCEPTION | processNpcHuntPlayers wired and correctly ordered before processNpcs (prior cluster-D confirmed fixed) |
| npc-hunt-12 | `npc_hunt.go:373-388`,`npc_interaction.go:933-995` | `Npc.ts:896-907`,`PathingEntity.ts:510-548` | CONFIRMED-EXCEPTION | HuntType.FindNewMode (incl default NONE) correctly feeds SetInteraction post-hunt |

### Pathing (cont.) / interaction (cont.)
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| pathing-3 | `movement.go:91-98` | `PathingEntity.ts:134-151`,`Player.ts:670-673` | COMMENT-LIE | resolveMovement resets tempRun on a blocked step; TS preserves it (verdict CONFIRMED LOW) |
| pathing-6 | `player_script.go:589-594` | `Player.ts:1201-1202,1040`,`PathingEntity.ts:446` | COMMENT-LIE | Teleport comment calls lastStepX/Z a 'dead-write' but it IS read via followX/Z (verdict CONFIRMED LOW) |
| pathing-7 | `player.go:557-558` | `PathingEntity.ts:108-109`,`Player.ts:511-512` | DEVIATION | Player ctor seeds lastStepX/Z = -1 instead of TS's (x-1, z) (corrected at login) |
| pathing-8 | `movement.go:10-13` | `PathingEntity.ts:239-242` | DEVIATION | queueWaypoint packs with p.level; TS hardcodes level 0 (inert — takeStep ignores level) |
| pathing-9 | `player_masks.go:165-178` | `Player.ts:1860-1873` | CONFIRMED-EXCEPTION | (*Player).Damage coerces negative amount to 0 (documented intentional) |
| interaction-8 | `interaction_trigger.go:310-317` vs `interaction.go:834-836` | `Player.ts:1139` | COMMENT-LIE | fireApTriggerNpc doc claims effectiveApRange reads npc.typ.AttackRange — code returns p.apRange (verdict CONFIRMED LOW) |
| interaction-9 | `interaction.go:113-116`,`npc_interaction.go:999-1004` | `PathingEntity.ts:528` | DEVIATION | SetInteraction focus uses 1x1 footprint for NPC targets (masked by faceEntity centering) |
| interaction-10 | `interaction.go:91-144` | `PathingEntity.ts:510-513` | MISSING | SetInteraction omits TS setInteraction's target.isValid early-return guard (no caller reads return) |
| interaction-11 | `interaction.go:575,587` | `Player.ts:1087,1089` | DEVIATION | defaultOp debug-name fallback returns strconv(typ/com) instead of TS '[object Object]' (debug-only) |
| interaction-12 | `interaction.go:462,471` | `Player.ts:1113-1184` | EXTENSION | tryInteract sets the PathingEntity.interacted field (unread; no TS counterpart) |
| interaction-13 | `interaction_trigger.go:118-121` | `Player.ts:1139-1170` | DEVIATION | Stale DEVIATION S6j-D2 comment on fireOpTriggerLoc ('no APLOC fallback') predates the AP branch |
| interaction-14 | `player_script.go:400-411` | `Player.ts:805-812` | CONFIRMED-EXCEPTION | CanAccess omits TS World.shutdown short-circuit (documented; cf. world-tick-2) |

### Entity base
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| entity-base-1 | `loc_turn.go:16-18`,`obj_turn.go:37-39`,`nonpathing.go:55`,`entity.go:37-40` | `Loc.ts:54-57`,`Obj.ts:33-35`,`World.ts:376+388` | DEVIATION | Loc/Obj lifecycle fires one tick LATER than TS — relative→absolute re-model off-by-one (verdict CONFIRMED, downgraded LOW) |
| entity-base-2 | `loc_turn.go:11-14`,`obj_turn.go:18-22` | `Loc.ts:54-57`,`World.ts:376+388` | COMMENT-LIE | turnLoc/turnObj + spec D-N86-4 claim 'observably equivalent' contradicted by TS same-tick decrement (verdict CONFIRMED LOW) |
| entity-base-4 | `pkg/entity/obj.go:93-101` | `Obj.ts:52-62` | DEVIATION | Obj.IsValidFor omits the super.isValid() (isActive) check present in TS Obj.isValid (dup: h-obj-6) |
| entity-base-6 | `obj_turn.go:45-50` | `Obj.ts:40-44` | DEVIATION | Obj.turn default-arm side-effect order swapped (setLifeCycle then error — independent, identical) |
| entity-base-7 | `pkg/entity/loc.go:39-46` | `Loc.ts:20-24` | DEVIATION | packLocInfo computes layer from masked shape (shape&0x1F); TS locShapeLayer uses raw shape (diverge only shape≥32) |
| entity-base-8 | `heropoints.go:66-77` | `HeroPoints.ts:37-47` | DEVIATION | HeroPoints empty-ledger sentinel differs (TS -1n; Go 0) (dup: util-1) |
| entity-base-9 | `nonpathing.go:47-54` | `NonPathingEntity.ts:11-25` | EXTENSION | NonPathing.SetLifeCycle adds a defensive nil-tracker guard absent in TS |
| entity-base-10 | `tick.go:926-946`,`loc_tracker.go:34-48` | `World.ts:961-986`,`LocObjEvent.ts:14-16` | DEVIATION | processZones snapshots the tracker; TS iterates live with per-event check() validity guard |

### Tracking
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| tracking-1 | `pkg/rsbuf/buildarea.go:1-255` | `BuildArea.ts:1-94` | WRONG-PATH | Listed Go file pkg/rsbuf/buildarea.go is NOT the counterpart of TS BuildArea.ts (real port in player.go/data_map.go/rebuildmap.go) |
| tracking-2 | `player.go:804-811` | `BuildArea.ts:78-84` | DEVIATION | rebuildScenery adds defensive negative-zone and >0xff-mapsquare skips absent from TS rebuildNormal |
| tracking-3 | `player.go:860-862` | `BuildArea.ts:46-54` | DEVIATION | rebuildZones adds defensive negative-zone skip absent from TS |
| tracking-4 | `player.go:646` | `Player.ts:375,2051-2063` | DEVIATION | afkZones[0] pre-seeded at construction; TS leaves it [0,0] until first updateAfkZones (lastAfkZone 1 vs 0) |
| tracking-5 | `logger_bridge.go:41-48` | `World.ts:2314-2321` | DEVIATION | SubmitInputTracking logger sink omits the timestamp field |
| tracking-6 | `pkg/script/active.go:8-20` | `WealthEvent.ts:7-25` | MISSING | WealthEvent struct omits TS recipient_items/recipient_value (PVP) + coord/session_uuid; nullability differs |
| tracking-7 | `player.go:1421-1439` | `World.ts:2222-2231` | MISSING | AddSessionLog omits the trackSessionEventsPublished metric increment |
| tracking-8 | `handler_event_tracking.go:52-69` | `EventTrackingHandler.ts:7-28` | EXTENSION | EVENT_TRACKING handler emits a zero-coordinate MouseMove telemetry event with no TS precedent |
| tracking-9 | `input_tracking.go:172-175` | `InputTracking.ts:141-145` | N-A | submitEvents recordedBlobs[0] empty-slice panic edge faithfully mirrors TS Buffer.from(undefined) throw |

### Script core / iter / opcodes
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| script-core-4 | `file.go:88`,`provider.go:105-107` | `ScriptFile.ts:99`,`ScriptProvider.ts:73-75` | DEVIATION | Decode reads lookupKey as unsigned u32, fixing a latent TS map-pollution bug (Go more correct) |
| script-core-5 | `file.go:96-102` | `ScriptFile.ts:141-149` | MISSING | ScriptFile has no lineNumber(pc); PCs/Lines decoded but never read (consequence of script-core-1) |
| script-core-6 | `file.go:104-124` | `ScriptFile.ts:76,111-124` | DEVIATION | Decode pre-sizes opcode arrays to trailer count and writes without re-bounding (panics if declared < actual) |
| script-core-8 | `runner.go:53-66` | `ScriptRunner.ts:140-148` | DEVIATION | Execute reorders the PC-bounds and opcount-cap checks relative to TS (different error when both trigger) |
| script-iter-1 | `npc_iterator.go:181,233`,`player_iterator.go:88` | `ScriptIterators.ts:57,...` | DEVIATION | radius arithmetic diverges for negative distance in [-7,-2] (unobservable; distance always ≥0) |
| script-iter-2 | `npc_iterator.go:99-108` | `ScriptIterators.ts:274-280` | CONFIRMED-EXCEPTION | NpcIterator HuntAll op[1] gate pessimistically allows when Configs nil; TS throws |
| script-iter-3 | `npc_iterator.go:140-...`,`player_iterator.go:63-78` | `ScriptIterators.ts:113-...` | CONFIRMED-EXCEPTION | Iterator LoS/LoW pessimistically allow when lineValidator nil; TS always invokes rsmod |
| script-iter-4 | `npc_iterator.go:13-17` | `NpcIteratorType.ts:1-4` | N-A | NpcIteratorMode enum values differ from TS NpcIteratorType (internal-only, never serialized) |
| script-opcodes-1 | `opcode.go:303` | n/a | EXTENSION | Go-only opcode OpLocOp = 3014 with no TS ScriptOpcode counterpart (dup: h-loc-11) |
| script-opcodes-2 | `trigger.go:361-366` | `ServerTriggerType.ts:166-170` | DEVIATION | ServerTriggerType.String() returns 'trigger_<N>' for unknown; TS toString throws |
| script-opcodes-3 | `opcode.go:1283-1285` | `ScriptOpcode.ts:859-861` | DEVIATION | Opcode.String() returns 'opcode_<n>' for undefined; TS reverse map yields undefined |

### Script handlers (LOW remainder)
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| h-player-5 | `handlers_player.go:1388-...` | `PlayerOps.ts:972-974,...` | DEVIATION | HINT_NPC/P_OPNPC/P_OPNPCT read raw primary ActiveNpc where TS uses operand-resolved (gate/read slot mismatch) |
| h-player-6 | `handlers_player.go:2065-2086` | `PlayerOps.ts:1191-1202` | DEVIATION | WEALTH_EVENT uses obj id=-1 for an unresolved name where TS passes undefined |
| h-player-7 | `handlers_player.go:1426-1437` | `PlayerOps.ts:976-978` | DEVIATION | HINT_PL operand=1 gate validates raw Self2 while read uses operand-resolved activePlayer2() |
| h-player-8 | `handlers_player.go:2000-2014` | `PlayerOps.ts:1127-1135` | DEVIATION | P_OPPLAYERT adds a PtrActivePlayer2 pointer-flag gate TS lacks (sibling P_OPPLAYER matches) |
| h-player-9 | `handlers_player.go:1085-1090,1127-1132` | `PlayerOps.ts:391-394,408-411` | DEVIATION | P_OPLOC/P_OPNPC fire unconditionally when Configs nil instead of TS default empty-op skip |
| h-interface-3 | `player_script.go:1044-1088` | `Player.ts:741-794` | DEVIATION | CloseModal does not clear protect the way TS closeModal does (modeled via activeScript pointer) |
| h-interface-4 | `player_script.go:1137-1160` | `Player.ts:1957,1963,1980` | DEVIATION | OpenChat/OpenSide reset modal-slot fields to -1, not replicating TS's copy-paste field bug |
| h-interface-5 | `player_script.go:1215-1226` | `Player.ts:716-726` | DEVIATION | CloseTutorial clears the modalState TUT bit; TS leaves it set (inert) (dup: player-script-13) |
| h-interface-6 | `handlers_dialog.go:64-146`,`handlers_interface.go:109,132` | `PlayerOps.ts:249-340,...` | EXTENSION | LAST_*/bare-handler ops add requireActivePlayer guard absent in TS; trigger-check ordering differs |
| h-npc-1 (M14) | `handlers_npc.go:137-142` | `ScriptValidators.ts:114` | CONFIRMED-EXCEPTION | checkQueue [1,20] vs TS QueueValid [0,19] is a genuine correct deviation |
| h-npc-2 | `handlers_npc.go:193-198,...` | `ScriptPointer.ts:47-56`,`NpcOps.ts:191-233` | N-A (verdict REFUTED→no-defect) | Cluster J operand resolution confirmed-fixed (verdict real=false; kept as verified-clean) |
| h-npc-4 | `handlers_npc.go:1224,1256,1436` | `NpcOps.ts:251,502,516` | DEVIATION | STATHEAL/STATADD/STATSUB truncation point differs for negative base*percent (content non-negative) |
| h-npc-5 | `handlers_config.go:296-...`,`handlers_npc.go:274-277` | `NpcConfigOps.ts:12,...` | DEVIATION | NC_NAME/NPC_NAME/NC_DESC/NC_DEBUGNAME use empty-string fallthrough vs TS nullish (??) |
| h-npc-6 | `handlers_config.go:324-337` | `NpcOps.ts:132-135` | DEVIATION | NPC_PARAM validates npcType before paramType — reversed vs TS |
| h-npc-7 | `npc_script_lookup.go:25-...` | `NpcOps.ts:347-358,...` | DEVIATION | NPC_FIND/FINDCAT/FINDEXACT tie-break iteration order differs (slot vs zone/distance-iterator order) |
| h-npc-8 | `handlers_npc.go:462-...` | `NpcOps.ts:78-80,...` | CONFIRMED-EXCEPTION | Defensive nil-World guards in NPC_DEL/ARRIVEDELAY/FINDHERO (TS has none; unreachable) |
| h-npc-9 | `handlers_npc.go:1146-1194` | `NpcOps.ts:152-168`,`CoordGrid.ts:59-72` | N-A | NPC_RANGE size-aware Chebyshev confirmed byte-faithful |
| h-inv-4 | `handlers_inv.go:2246-2253`,`player_script.go:1612-1644` | `InvOps.ts:792-796`,`Player.ts:1668-1672` | DEVIATION | INV_TOTALPARAM_STACK silently returns 0 on invalid inv/param where TS throws |
| h-inv-5 | `handlers_inv.go:309-323` | `InvOps.ts:786-790`,`Player.ts:1668-1694` | DEVIATION | INV_TOTALPARAM eagerly validates param where TS validates lazily (empty-inv edge) |
| h-inv-6 | `handlers_inv.go:1142-1169` | `Player.ts:1597-1629` | DEVIATION | INV_MOVETOSLOT silently no-ops on out-of-range slot where TS throws |
| h-inv-7 | `handlers_inv.go:2171,2221-2225` | `InvOps.ts:744,776-783` | DEVIATION | INV_DROPALL Death-event item ordering is non-deterministic (Go map) vs TS insertion order |
| h-inv-8 | `handlers_inv.go:63,86,...` | `InvOps.ts:263,...` | DEVIATION | INV read opcodes lack explicit ActivePlayer gate (operand-1 read also resolves Self) (operand cluster) |
| h-inv-9 | `handlers_inv.go:733,...` | `InvOps.ts:333,...` | CONFIRMED-EXCEPTION | NAI-131-D1 to-protect gate evaluates fromInvType.Scope — faithfully pins a real TS quirk |
| h-inv-10 | `handlers_inv.go:1476-1523` | n/a | EXTENSION | INV_DROPITEM emits goscape ItemDroppedEvent telemetry (only on INV_DROPITEM, internally inconsistent) |
| h-inv-11 | `handlers_inv.go:1926-1937` | `InvOps.ts:34-38` | DEVIATION | INV_DEBUGNAME maps empty-string debugname to 'null' (TS ?? only catches null) |
| h-obj-4 | `handlers_obj.go:453-466` | `ObjOps.ts:95-104` | DEVIATION | OBJ_PARAM validates ObjType before ParamType + gates null-obj before popping; TS validates ParamType first |
| h-obj-5 | `handlers_obj.go:326-332` | `ObjOps.ts:156-160` | DEVIATION | OBJ_TAKEITEM lifecycle neither RESPAWN nor DESPAWN: TS skips removeObj, Go always RemoveObj(obj,0) |
| h-obj-6 | `pkg/entity/obj.go:93-101` | `Obj.ts:52-62`,`Entity.ts:32-34` | DEVIATION | Obj.IsValidFor omits the `&& hash64` short-circuit and the Entity.isActive final gate (dup: entity-base-4) |
| h-obj-7 | `handlers_obj.go:439-445,...` | `ObjOps.ts:109`,`ObjConfigOps.ts:...` | DEVIATION | Name/Desc/Debugname fallbacks treat empty-string as null; TS ?? only on actual null |
| h-obj-8 | `handlers_obj.go:147-168` | `ObjOps.ts:112-119` | CONFIRMED-EXCEPTION | OBJ_DEL collapses TS's identical-arm pointerGet branch into one unconditional RemoveObj |
| h-loc-5 | `loc_iterator.go:14-18` | `ScriptIterators.ts:377-385`,`Zone.ts:459-465` | COMMENT-LIE | loc_iterator.go calls forward-unfiltered snapshot 'equivalent' to TS getAllLocsSafe(true) (verdict CONFIRMED LOW) |
| h-loc-7 | `script_loc_ops.go:30-46` | `LocOps.ts:39` | DEVIATION | AddLoc adapter coerces width/length 0→1; TS passes loctype values verbatim |
| h-loc-8 | `pkg/zone/zone.go:158-172`,`loc_turn.go:24-25` | `Zone.ts:217-228` | DEVIATION | Go Zone.AddLoc does not loc.revert() on (re-)add; TS Zone.addLoc does (dup: zone-sub-1) |
| h-loc-9 | `handlers_loc.go:360-374` | `LocOps.ts:114-122` | DEVIATION | LOC_PARAM validates LocType before ParamType; TS validates ParamType first |
| h-loc-10 | `handlers_config.go:45-46,...`,`handlers_loc.go:276-282` | `LocConfigOps.ts:12,...` | DEVIATION | String name/desc/param defaults push "" for explicit-empty values; TS ?? keeps "" (dup: h-config-4) |
| h-loc-11 | `opcode.go:303`,`handlers_loc.go:156-177` | n/a | EXTENSION | Go adds LOC_OP opcode (3014) with no TS counterpart (dup: script-opcodes-1) |
| h-loc-12 | `loc_turn.go:15-33` | `Loc.ts:54-74` | N-A | turnLoc has no explicit counterpart for TS Loc.turn's lifecycleTick<0 failsafe (unreachable in absolute model) |
| h-server-2 | `handlers_map.go:233,256,427,298` | `ServerOps.ts:65-162` | DEVIATION | Inconsistent nil-World handling: MAP_BLOCKED/LINEOFSIGHT/LINEOFWALK/SPOTANIM_MAP panic while siblings return error |
| h-server-3 | `handlers_map.go:101-102,132` | `ServerOps.ts:264-...` | EXTENSION | MAP_FINDSQUARE uses math/rand/v2 instead of TS Math.random (distribution-equivalent) |
| h-number-1 | `handlers_number.go:272-286` | `NumberOps.ts:154-163` | DEVIATION | SETBIT_RANGE_TOINT masks the value to the field width instead of clamping (saturating) to maxValue (verdict CONFIRMED, downgraded MEDIUM→LOW... see note) |
| h-number-2 | `handlers_number.go:107-112` | `NumberOps.ts:49-52` | DEVIATION | ADDPERCENT truncates the percent quotient before adding (integer div) (diverges only negative percent) |
| h-number-3 | `handlers_number.go:114-128` | `NumberOps.ts:124-127` | DEVIATION | SCALE uses exact int64 (a*c)/b where TS computes in lossy float64 above 2^53 |
| h-number-4 | `handlers_number.go:215-241` | `NumberOps.ts:54-67,...` | DEVIATION | Bit ops (SET/CLEAR/TOGGLE/TESTBIT) do not mask the shift count to 5 bits; negative bit panics in Go |
| h-number-5 | `handlers_number.go:290-308` | `NumberOps.ts:32-40` | DEVIATION | RANDOM/RANDOMINC clamp negative bounds to 0 and use a non-deterministic PRNG (no JavaRandom port) |
| h-number-6 | `handlers_number.go:376-442` | `ServerOps.ts:93-123` | N-A | COORD*/DISTANCE handlers belong to ServerOps.ts (out of NumberOps audit scope) |
| h-number-7 | `handlers_number.go:59-...` | `NumberOps.ts:20-...` | CONFIRMED-EXCEPTION | M15-M18/L24/L25 prior-cluster-L truncation/float semantics re-verified correct |
| h-string-1 | `handlers_string.go:298-304` | `StringOps.ts:121` | DEVIATION | SPLIT_GETANIM pushes -1 when page line-count exceeds MesanimType.Len bounds; TS pushes 0 |
| h-string-2 | `handlers_string.go:233-235` | `StringOps.ts:93-95` | COMMENT-LIE | SPLIT_INIT comment claims TS divide-by-zero on splice(0,0); TS actually infinite-loops (verdict CONFIRMED LOW) |
| h-string-3 | `handlers_string.go:233-238` | `StringOps.ts:93-95` | DEVIATION | SPLIT_INIT linesPerPage<0 folds to single page in Go; TS hangs |
| h-db-1 | `handlers_db.go:9-18,...` | `ScriptOpcodePointers.ts:966-982`,`Compiler.ts:115-138` | DEVIATION | find_db gate is a RUNTIME pointer in Go but COMPILE-TIME-only in TS (benign for valid blobs) |
| h-db-2 | `handlers_db.go:168 vs 190` | `DbOps.ts:83 vs 153` | DEVIATION | DB_FINDNEXT gates on PtrFindDb while sibling DB_FINDBYINDEX gates on DbTable==nil; TS uses same gate |
| h-db-3 | `handlers_db.go:269-271,311-313` | `DbOps.ts:42-63` | DEVIATION | DB_FIND_REFINE has NO runtime gate in TS but Go errors when PtrFindDb unset (silent-empty vs error) |
| h-db-4 | `state.go:317-324` | `ScriptState.ts:122`,`DebugOps.ts:13-26` | COMMENT-LIE | Timespent comment claims TS uses undefined→NaN when unset; TS initializes timespent to 0 (verdict CONFIRMED LOW) |
| h-db-5 | `handlers_debug.go:11-14` | `DebugOps.ts:5-7` | DEVIATION | ERROR opcode prepends 'ERROR: ' to the scripted message; TS throws the raw popped string |
| h-db-6 | `dbtableindex.go:142-167` | `DbTableIndex.ts:75-89` | DEVIATION | Not-indexed-column find drops TS printWarning (dup: cfg-var-6) |
| h-db-7 | `handlers_db.go:67,95`,`runner.go:52-83` | `ScriptRunner.ts:137-228` | DEVIATION | Go DB handlers can panic on OOB column/table index where TS try/catch gracefully aborts |
| h-db-8 | `handlers_debug.go:28-37` | `DebugOps.ts:17-26` | DEVIATION | GETTIMESPENT value can differ under int32 overflow on huge elapsed (debug-only) |
| h-db-9 | `handlers_db.go:237-238` | `DbOps.ts:16-18` | DEVIATION | db_find sets DbRow=-1 AFTER the index lookup; TS sets dbRow=-1 BEFORE find (no observable effect) |
| h-core-2 | `handlers_config_test.go:484-495` | `EnumOps.ts:17-22` | DEVIATION | Test pins the divergent ENUM contract (asserts DefaultString, contradicting TS) (dup: h-config-2) |
| h-core-4 | `handlers_array.go:11-47` | `CoreOps.ts:232-242` | EXTENSION | DEFINE_ARRAY/PUSH_ARRAY_INT/POP_ARRAY_INT fully implemented in Go; TS throws 'unimplemented' |
| h-core-5 | `state.go:509-511`,`handlers_core.go:16-28` | `CoreOps.ts:194-214` | DEVIATION | GOSUB frame-overflow is a panic (recovered at tick level) instead of script-scoped throw (dup: script-core-2) |
| h-core-6 | `handlers_core.go:682-692` | `CoreOps.ts:167-173` | DEVIATION | POP_INT/STRING_DISCARD clamp at empty stack instead of decrementing the pointer below zero like TS |
| h-core-7 | `handlers_vars.go:20-35`,`configs.go:39-47` | `CoreOps.ts:33,...` | DEVIATION | VARP/VARN id validation degraded: out-of-range id silently treated as INT-typed (dup: h-config-6) |
| h-core-8 | `handlers.go:9-11` | n/a | COMMENT-LIE | handlers.go header comment claims 'Only the 19 S1 MVP opcodes registered' — false (~394 registered) (verdict CONFIRMED LOW) |
| h-core-9 | `handlers_core.go:17-...` | `CoreOps.ts:194-230` | EXTENSION | Defensive nil-Provider guard in GOSUB/JUMP variants (singleton; unreachable) |
| h-config-5 | `handlers_vars.go:108-125` | `CoreOps.ts:257-275` | MISSING | PUSH_VARS/POP_VARS hardcode int dispatch — string-typed shared vars unsupported (dup: h-core-3) |
| h-config-6 | `handlers_vars.go:20-35`,`server_configs.go:173-192` | `CoreOps.ts:33,...` | DEVIATION | Var-id/config-id OOB validation degraded (no throw) vs TS check() — NAI-121-D3 (dup: h-core-7) |
| h-config-7 | `handlers_config.go:395-414` | `NpcConfigOps.ts:39-50` | DEVIATION | NC_OP validates op before npc; TS validates npc first |
| h-config-8 | `handlers_config.go:180-...` | `ObjConfigOps.ts:12,...` | DEVIATION | name/desc/debugname use empty-string test (!="") instead of TS ?? (dup: h-npc-5/h-obj-7) |
| h-config-9 | `handlers_config.go:110` | `EnumOps.ts:17` | DEVIATION | ENUM key truncated to int32 for map lookup — keys outside int32 range could falsely match |
| h-config-10 | `handlers_vars.go:78-106`,`handlers_timer.go:13-66` | `CoreOps.ts:41-91`,`PlayerOps.ts:817-864` | CONFIRMED-EXCEPTION | VARN/VARP secondary-bit + protect-gate + timer semantics verified TS-faithful |

### Net protocol (LOW remainder)
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| net-client-prot-1 | `player.go:1187-1194` | `NetworkPlayer.ts:146-152` | DEVIATION | USER_EVENT limit increment ignores handler success (dup: player-net-7) |
| net-client-prot-3 | `player.go:1187-1194` | `NetworkPlayer.ts:138-153` | DEVIATION | Unbound opcodes (15 anticheat + EVENT_CAMERA_POSITION) increment clientLimit in Go but no counter in TS |
| net-client-prot-4 | `handlers_game.go:36,208-210` | `ClientGameProtRepository.ts:124` | EXTENSION | NO_TIMEOUT (108) handled in Go (no-op) but left unbound/commented-out in TS |
| net-client-prot-5 | `handler_reportabuse.go:43` | `Packet.ts:263-265` | DEVIATION | ReportAbuse moderatorMute decodes as `G1() != 0`, TS gbool() is `g1() === 1` |
| net-client-prot-6 | `handlers_game.go:633,674` | `ClientCheatHandler.ts:43-53` | DEVIATION | ClientCheat 'Ran cheat' session log records lowercased text; TS logs original-case input |
| net-client-prot-7 | `handler_opheld.go:73-79 (+oploc/opobj/opnpc)` | `OpHeldHandler.ts:46-50 (+...)` | CONFIRMED-EXCEPTION | Op* handlers add defensive ObjType/LocType/NpcType nil-guards where TS would throw |
| net-client-prot-8 | `handler_opheld.go:80 (+inv_button/...)` | `OpHeldHandler.ts:47 (+...)` | CONFIRMED-EXCEPTION | Go tests iop/op slot == empty-string where TS tests === null (faithful convention) |
| net-client-h-interact-1 | `handler_opheld.go:320-340` | `OpHeldUHandler.ts:51-76` | DEVIATION | OpHeldU: out-of-bounds slot triggers cleanup that TS reserves for valid-slot/wrong-item only |
| net-client-h-interact-2 | `handler_opheld.go:210-...`,`handler_inv_button.go:65,128` | `OpHeldHandler.ts:71-73` | MISSING | Environment.NODE_DEBUG 'No trigger for [...]' developer messages omitted across all handlers |
| net-client-h-interact-3 | `handler_interface.go:52-54` | `IfButtonHandler.ts:26-29`,`ScriptRunner.ts:127-130` | DEVIATION | IfButton resume path bypasses ScriptRunner executionHistory push |
| net-client-h-interact-4 | `handler_opheld.go:77-79,...` | `OpHeldHandler.ts:45,...` | CONFIRMED-EXCEPTION | Defensive ObjType/LocType/NpcType nil guards Go adds where TS would throw |
| net-client-h-interact-5 | `handler_oploc.go:98-119`,`handler_opobj.go:81-84` | n/a | EXTENSION | Go adds targetSubject lifecycle snapshot + emitOpLocGate/NODE_DEBUG telemetry not in TS |
| net-client-h-social-3 | `handlers_game.go:824,842` | `ClientCheatHandler.ts:414,431` | DEVIATION | setstat/advancestat non-numeric level: Go clamps to 1; TS produces NaN |
| net-client-h-social-4 | `handlers_game.go:508` | `ClientCheatHandler.ts:77-80` | DEVIATION | debugproc INT arg lacks 32-bit truncation (TS `| 0`) |
| net-client-h-social-6 | `handlers_game.go:533-539` | `ClientCheatHandler.ts:103-107` | DEVIATION | debugproc STAT-arg unknown key: Go yields -1, TS yields undefined (documented, dev-only) |
| net-client-h-social-7 | `handlers_game.go:1336-1341` | `ClientCheatHandler.ts:606-612` | CONFIRMED-EXCEPTION | kick defers teardown one tick (NAI-186-D1) vs TS synchronous logout()+close() |
| net-client-h-social-8 | `handlers_game.go:1202` | `ClientCheatHandler.ts:499-518` | DEVIATION | tele uses whole-args split-on-comma so a trailing whitespace token corrupts the final coord field |
| net-client-h-social-9 | `handlers_game.go:401-413`,`handler_event_tracking.go:60-69` | n/a | EXTENSION | MESSAGE_PUBLIC and EVENT_TRACKING emit NAI-Phase2 telemetry events not present in TS |
| net-client-h-social-10 | `handlers_game.go:1347-1352` | n/a | EXTENSION | say cheat command is a goscape extension with no ClientCheatHandler counterpart |
| net-client-h-social-11 | `handlers_game.go:1149-1162` | `ClientCheatHandler.ts:327-338` | DEVIATION | givecrap adds an extra obj==nil continue guard absent in TS |
| net-server-prot-1 | `prot.go:9-10` | `ServerGameProt.ts:1-93` | COMMENT-LIE | Stale header comment claims only sub-spec-1 opcodes present; file actually contains all 71 (verdict CONFIRMED LOW) |
| net-server-enc-1 | `zone_encoders.go:31-39,122-149` | `ObjAddEncoder.ts:11`,... | DEVIATION | ObjAdd/ObjCount/ObjReveal count clamp differs for negative counts (clampU16→0 vs JS p2 wrap) (dup: zone-sub-1/-8) |
| net-server-enc-2 | `inv_update.go:33-37` | `UpdateInvFullEncoder.ts:14-16,19-20` | DEVIATION | UpdateInvFull size-byte clamp to 0xff differs when component grid >255 (dup: inventory-3) |
| net-server-enc-3 | `packet.go:396-406` | `Packet.ts:330-337` | DEVIATION | PJStr writes UTF-8 bytes whereas TS pjstr writes one byte per UTF-16 code unit (cross-cutting; dup) |
| net-server-enc-4 | `stat_update.go:28-36` | `Packet.ts:295-298` | COMMENT-LIE | sendUpdateRunWeight comment calls TS p2 'signed'; TS p2 is setUint16 unsigned (verdict CONFIRMED LOW; bytes identical) |

### I/O packet / isaac / jagfile / bzip2
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| io-packet-1 | `packet.go` (none) | `Packet.ts:251-255` | MISSING | g4s() (signed 4-byte reader) absent in Go (cache-config parsers substitute) |
| io-packet-2 | `packet.go` (none) | `Packet.ts:237-243` | MISSING | g3s() (signed 3-byte reader) absent in Go (no TS caller) |
| io-packet-3 | `packet.go:222-224` | `Packet.ts:257-261` | DEVIATION | G8 returns uint64; TS g8 returns signed bigint (bytes identical) |
| io-packet-5 | `packet.go:247,251` | `Packet.ts:273` | DEVIATION | GJStr builds string from raw bytes (UTF-8) vs TS String.fromCharCode (Latin-1 code points) |
| io-packet-6 | `packet.go:80-90` | `Packet.ts:98-140` | DEVIATION | Alloc is size-based (rounds up to tier); TS alloc is type-based (0-5 fixed sizes) |
| io-packet-7 | `packet.go:108-113` | `Packet.ts:142-164` | DEVIATION | Release/pool: sync.Pool capacity-tier, no count cap; TS exact-length match w/ per-tier caps |
| io-packet-8 | `packet.go:475-491` | `Packet.ts:423-436` | DEVIATION | RSAEnc length byte uses minimal big.Int.Bytes length vs TS Java-BigInteger toByteArray length |
| io-packet-9 | `load.go:10-19` | `Packet.ts:166-200` | MISSING | Load() omits seekToEnd; saveGz() and fetch() not ported (no callers) |
| io-packet-10 | `packet.go:414-419` | `Packet.ts:339-342` | DEVIATION | PData ignores TS offset/length args and writes whole slice (all callers offset=0) |
| io-packet-11 | `packet.go:211-219`,`alt.go:6-34` | n/a | EXTENSION | Go adds IG4 reader and P1Alt/P2Alt/P4Alt/PDataAlt writers (rsbuf branch 225; forward-looking) |
| io-packet-12 | `packetbit.go:36-55` | `Packet.ts:384-402` | CONFIRMED-EXCEPTION | GBit 32-bit-read sign behavior differs but Go more correct (no live caller) |
| io-packet-13 | `packetbit.go:57-90`,`packet.go:151-...` | `Packet.ts:404-421,...` | EXTENSION | PBit grows buffer; readers panic(io.EOF); GJStr handles empty buffer — safer failure modes |
| io-packet-14 | `packet.go:14-24` | `Packet.ts:54-60` | CONFIRMED-EXCEPTION | GetCRC uses correct offset+length span; TS getcrc has latent end-index bug (all callers offset=0) |
| io-packet-15 | `packet.go:271-295` | `Packet.ts:278-284` | CONFIRMED-EXCEPTION | GSmart/GSmartS sign-fix verified correct |
| io-packet-16 | `packet.go:265-268` | `Packet.ts:286-289` | DEVIATION | GData drops TS's dest offset parameter (always dest[0]; all callers offset=0) |
| io-isaac-1 | `isaac.go:211` | `Isaac.ts:121` | DEVIATION | Second mem lookup index rewritten ((y>>>8)>>>2)&0xff → (y>>10)&0xFF (proven equivalent) |
| io-isaac-2 | `isaac.go:11,190` | `Isaac.ts:7,96` | DEVIATION | State counter `c` is uint32 (wraps at 2^32) vs TS float64 (exact to 2^53) — unreachable edge |
| io-isaac-3 | `server.go:926-930` | `World.ts:2150` | DEVIATION | Seed read with unsigned G4 vs signed g4s (bit-identical for the cipher) |
| io-jagfile-1 | `jagfile.go:82` | `Jagfile.ts:95-97` | MISSING | Jagfile.Get drops TS's explicit out-of-bounds guard; corrupt jagfile panics instead of clean error |
| io-jagfile-2 | `crctable.go:58-77` | `CrcTable.ts:9-35` | DEVIATION | MakeCRCs always emits a 9-slot table (0 for missing); TS makeCrc SKIPS missing files, compacting later slots |
| io-jagfile-3 | `jagfile.go:235-243` | `Jagfile.ts:144,237-240` | DEVIATION | Save re-compresses FileWrite in place; a second Save call double-bzip2s per-file entries |
| io-jagfile-4 | `jagfile.go:251-255` | `Jagfile.ts:237-240` | EXTENSION | Save falls back to original packed bytes for un-rewritten loaded entries; TS would throw |
| io-jagfile-5 | `jagfile.go:34-41` | `Jagfile.ts:4-11` | DEVIATION | genHash iterates UTF-8 runes/ToUpper; TS UTF-16 code units/toUpperCase (ASCII identical) |
| io-jagfile-6 | `preloaded.go:65-67` | `PreloadedPacks.ts:9-18` | DEVIATION | PreloadClient explicitly skips subdirectories; TS would throw (EISDIR) on a subdir entry |
| io-jagfile-7 | `crctable.go:25-28` | `CrcTable.ts:7,34`,`World.ts:2129-2136` | CONFIRMED-EXCEPTION | CrcBuffer32 (whole-table CRC) dropped; world login validates per-slot (equivalent accept/reject) |
| io-jagfile-8 | `content_watcher.go` (out of unit) | `DevThread.ts:1-112` | CONFIRMED-EXCEPTION | DevThread.ts logic lives in modules/world/content_watcher.go |
| io-bzip2-1 | `pkg/io/jagfile/bzip2.go` | `BZip2.ts:1-7` | WRONG-PATH | Listed pkg/io/bzip2/bzip2.go does not exist; framing quirk lives in pkg/io/jagfile/bzip2.go |
| io-bzip2-2 | `bzip2.go:88-98` | `bzip2-wasm.js:120-135` | DEVIATION | decompressedLength does not bound actual output in Go; TS WASM rejects (BZ_OUTBUFF_FULL) |
| io-bzip2-3 | `bzip2.go:18,70-72,92-98` | `bzip2-wasm.js:94-136` | EXTENSION | Go adds MaxBZip2DecompressedSize (64 MiB) range guard absent in TS |
| io-bzip2-4 | `bzip2.go:30-35`,`writer.go:47-57` | `bzip2-wasm.js:149-151` | DEVIATION | blockSize/Level=0: TS throws RangeError, Go defaults Level 0 to 6 (callers always pass 1) |
| io-bzip2-5 | `bzip2.go:21-29` | `bzip2-wasm.js:141-171` | DEVIATION | compressedLength has no functional effect in Go (capacity hint); TS dest size can BZ_OUTBUFF_FULL |
| io-bzip2-6 | `pkg/io/bzip2/*` | `BZip2.ts:1-7` | EXTENSION | pkg/io/bzip2 is a native dsnet/compress reimpl (Case-B); TS uses WASM/libbzip2 |
| io-bzip2-7 | `bzip2.go:43-54` | `bzip2-wasm.js:175-186` | CONFIRMED-EXCEPTION | prefixLength then removeHeader makes prefixLength a no-op (faithfully preserved) |
| io-bzip2-8 | `bzip2.go:63-66,74-78` | `bzip2-wasm.js:97-111` | CONFIRMED-EXCEPTION | Hardcoded 'BZh1' matches TS verbatim (assumes blockSize==1 streams) |

### Login / friend / logger / util (LOW remainder)
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| login-server-8 | `handler.go:284-290` | `LoginServer.ts:420-448` | DEVIATION | PlayerLogout errors NotFound when account missing; TS logout is lenient (success even if gone) |
| login-server-9 | `handler.go:295-304` | `LoginServer.ts:19-109,450` | STALE-DEFER | updateHiscores not ported (TS exports per-stat XP/levels on logout) (verdict CONFIRMED LOW) |
| login-server-10 | `save.go:46-54` | `PlayerLoading.ts:85-90` | DEVIATION | savePlaytime assumes int32@offset24 for all versions; TS reads g2 playtime for version<2 |
| login-server-11 | `handler.go:144,152,...` | `LoginServer.ts:350-358,...` | DEVIATION | NEW_PLAYER and rejection responses carry members/muted_until that TS omits |
| login-server-12 | `handler.go:410-416` | `LoginServer.ts:298,...`,`World.ts:1897` | DEVIATION | muted_until expired-filter at login vs TS raw passthrough (world re-checks at chat time) |
| login-server-14 | `handler.go:90,104`,`server.go:948,976` | `LoginServer.ts:173,203,213` | DEVIATION | DB stores safe-name username; TS account table stores the raw username |
| friend-server-2 | `repository.go:64-67` | `FriendServer.ts:412-418`,`FriendServerRepository.ts:48-54` | (REFUTED — see appendix) | InitializeWorld doc 'Mirrors TS FriendServer initializeWorld' |
| friend-server-3 | `repository.go:357-370` | `FriendServerRepository.ts:332-355` | DEVIATION | IsVisibleTo returns false for an unregistered 'other' BEFORE the staff bypass; TS checks staff first |
| friend-server-4 | `repository.go:240-248` | `FriendServerRepository.ts:230-235` | DEVIATION | AddFriend/AddIgnore cap count is profile-scoped in Go; TS counts across ALL profiles (dup: datastruct-db-2) |
| friend-server-6 | `repository.go:318-341` | `FriendServerRepository.ts:177-181` | DEVIATION | GetFollowers queries ALL owners from DB; TS getFollowers returns only online (in-memory) followers (net result identical) |
| friend-server-7 | `subscriptions.go:16,81-94`,`world_subscriptions.go` | `FriendServer.ts:471-477,302-308` | EXTENSION | Buffered per-subscriber channel with drop-on-full; TS uses unbounded WS send queue |
| friend-server-8 | `handler.go:48-55` | `FriendServer.ts:89-93` | DEVIATION | No 'WORLD_CONNECT after already connected' duplicate-suppression at the RPC layer (compounds friend-server-1) |
| friend-server-9 | `repository.go:177-201` | `FriendServerRepository.ts:152-153` | CONFIRMED-EXCEPTION | Friend list NOT capped to 100 on the read path (both TS and Go omit the cap) |
| friend-server-10 | `repository.go:543-571`,`migrations/000002,000003` | `FriendServer.ts:279,293` | DEVIATION | PublicMessage/PrivateMessage timestamp uses DB CURRENT_TIMESTAMP, not the world-supplied nodeTime |
| logger-transport-6 | `player.go:1421-1439` | `Metrics.ts:1-74`,`World.ts:2230` | MISSING | Prometheus metrics (Metrics.ts) not ported; trackSessionEventsPublished.inc() has no Go counterpart (dup: tracking-7) |
| logger-transport-7 | `server.go:766-779` | `TcpClientSocket.ts:19-23`,`WSClientSocket.ts:22-31` | DEVIATION | TcpClientSocket.close() 1-second graceful drain not modeled (Go closes immediately after flush) |
| logger-transport-8 | `server.go:786-788` | `TcpServer.ts:24-27`,`web.ts:134-137` | DEVIATION | Seed-word upper bound differs from TS by one value (Go can emit 0xffffff/0xffffffff TS cannot) |
| logger-transport-9 | `client.go:30`,`server.go:825-851` | `ClientSocket.ts:14`,`TcpServer.ts:31` | DEVIATION | ClientStateClosed (-1) sentinel defined but never assigned (teardown via goroutine-return) |
| util-2 | `heropoints.go:64-66` | `NpcOps.ts:114-115` | COMMENT-LIE | HeroPoints.TopContributor comment calls itself a 'Stub' but it is the live FINDHERO impl (verdict CONFIRMED LOW) |
| util-3 | `jstring.go:62-64` | `JString.ts:57-59` | DEVIATION | ToTitleCase via strings.Title does not lowercase the tail of each token (pre-normalized input) |
| util-4 | `jstring.go:18-19` | `JString.ts:5-6` | DEVIATION | ToBase37 iterates bytes vs TS UTF-16 units (ASCII names identical) |
| util-5 | `handlers_number.go:347-353` | `Trig.ts:17-19` | DEVIATION | Atan2Deg uses Go math.Round (half-away-from-zero) vs JS Math.round (half-toward-+Inf) at exact .5 negatives |
| util-6 | n/a (no pkg port) | `QuickSort.ts:9-36` | CONFIRMED-EXCEPTION | QuickSort.ts has no standalone Go port (only consumer HeroPoints reimplements as linear scan; cf. util-1) |

### rsbuf / zone / pathfinder / pack (LOW remainder)
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| rsbuf-player-3 | `playerinfo.go:308-311` | `info.rs:155-160` | DEVIATION | write_new_players budget uses len(lowDef) only, not lowdefinitions+highdefinitions |
| rsbuf-player-4 | `playerinfo.go:9` | `build.rs:76` | DEVIATION | Exported PreferredPlayers=255 contradicts upstream PREFERRED_PLAYERS=250 (unused, misleading) |
| rsbuf-player-5 | `playerinfo.go:314-...` | `info.rs:161,179-181` | DEVIATION | new-player dx/dz clamped to [-15,15] then masked; Rust writes raw delta (no-op on reachable input) |
| rsbuf-player-6 | `playerinfo.go:228-231,303-305` | `info.rs:114,154` | EXTENSION | Go adds 7th SOFT-visibility+staff-mod reject (NAI-9), absent from Rust (admin-invisibility semantics) |
| rsbuf-player-7 | `player_script.go:635-650`,`mask_payload.go:82-85` | `info.rs:321-341`,`coord.rs:42-44` | DEVIATION | FACE_COORD low-def fallback collapsed 3-way→2-way via faceAngle, with z-1 south orientation |
| rsbuf-player-8 | `doc.go:7-40` | `PlayerInfo.ts:1-9`,`PlayerInfoEncoder.ts:9-15` | CONFIRMED-EXCEPTION | doc.go NAI-30/31 'TS-parity exception' claim verified accurate (bare TS containers delegate to rsbuf crate) |
| rsbuf-player-9 | `mask_payload.go:24-111` | `info.rs:362-401`,`message.rs` | N-A | Mask-payload field order + widths byte-verified against message.rs |
| rsbuf-npc-2 | `npcinfo.go:255-259,210-249` | `info.rs:454-455,...` | DEVIATION | Running byte total `bytes` not threaded; fits() in writeNewNpcs measures differently (usually more accurate) |
| rsbuf-npc-3 | `renderer.go:125-133` | `info.rs:625-666` | DEVIATION | Renderer lowdef never conditionally adds FACE_ENTITY from a persistent face_entity |
| rsbuf-npc-4 | `npcinfo.go:221-224` | `info.rs:517-527` | DEVIATION | writeNewNpcs adds a redundant !other.Active gate absent from Rust (unreachable-redundant) |
| rsbuf-npc-5 | `npcinfo.go:233-240` | `info.rs:524,542-545` | EXTENSION | writeNewNpcs add path clamps dx/dz to [-15,15] (no-op on reachable input) |
| rsbuf-npc-6 | `npcinfo.go:59-98` | `NpcInfoEncoder.ts:9-15`,`NpcInfo.ts:3-9` | N-A | TS NpcInfoEncoder/NpcInfo are pass-through stubs; Go natively reimplements rsbuf |
| rsbuf-zone-1 | `zone_encoders.go:31-39` | `ObjAddEncoder.ts:12` | DEVIATION | clampU16 clamps negative counts to 0; TS Math.min lets negatives wrap (dup: net-server-enc-1/zone-sub-8) |
| rsbuf-zone-2 | `zonemap.go:1-74` | `ZoneMap.ts:5-72` | N-A | rsbuf zonemap is the player/npc visibility index, NOT a port of TS engine ZoneMap (loc/obj+grid) |
| rsbuf-zone-3 | `zonemap.go:4-5,...` | `ZoneMap.ts:6-8,25-33` | DEVIATION | zonemap.go doc-comments cite Rust grid.rs lineage, not the TS reference (packing byte-identical) |
| zone-sub-2 | `player_zone.go:32-88` | `Zone.ts:133-165` | DEVIATION | writeFullFollows emits a single shared PartialFollows header instead of one-per-obj (dup: player-net-6) |
| zone-sub-3 | `player_zone.go:98-115` | `Zone.ts:184-195` | DEVIATION | writePartialFollows skips the header when no FOLLOWS event matches; TS emits a lone header when events.size>0 |
| zone-sub-6 | `pkg/zone/zone.go:299-307` | `Zone.ts:297-301` | DEVIATION | AddObj event-type branch differs for the (theoretical) RESPAWN-with-private-receiver case |
| zone-sub-7 | `pkg/zone/zone.go:176-211` | `Zone.ts:230-257` | DEVIATION | RemoveLoc/ChangeLoc do not move RESPAWN/changed locs to the list tail (iteration-order divergence) |
| zone-sub-8 | `zone_encoders.go:30-39,122-148` | `ObjAddEncoder.ts:11` | DEVIATION | clampU16 clamps negative counts to 0 (dup: zone-sub-1/net-server-enc-1) |
| zone-sub-9 | `pkg/zone/grid.go:42-69` | `ZoneGrid.ts:26-52` | DEVIATION | ZoneGrid.IsFlagged uses arithmetic >> on int32 where TS uses logical >>> (boolean output equivalent) |
| zone-sub-10 | `coordgrid.go:209-213` | `ZoneMap.ts:10-15` | N-A | ZoneMap.UnpackZoneIndex masks level with &0x3 where TS uses bare >>22 (no-op; dup: gamemap-4) |
| pathfinder-4 | `reach/strategy.go:470` | rsmod `reach_strategy.rs` | DEVIATION | reachWallDecoN WallDecorDiagonalBoth first branch compares against 0 not FlagOpen (cosmetic) |
| pathfinder-5 | `linevalidator.go:111-115` vs `lineroutefinder.go:131-144` | rsmod `line_validator.rs` vs `line_pathfinder.rs` | CONFIRMED-EXCEPTION | Two RayCast variants strip projectile flags at different depths — faithful to two distinct canonical sources |
| pathfinder-6 | `api.go:351-354` | n/a | N-A | Deprecated LocShapeLayer stub always returns layer 0 (real mapping in loc.LayerOf) |
| pack-config-1 | `pkg/pack/mesanim.go:48-52` | `MesAnimConfig.ts:69-77` | DEVIATION | mesanim bare `len` key (no digit): TS emits opcode 1, Go skips (Number('')===0 vs Atoi error) |
| pack-config-2 | `pkg/pack/obj.go:255-259` | `ObjConfig.ts:108-130` | DEVIATION | obj weight truncated to int at parse; TS keeps fractional grams until p2 emit |
| pack-config-3 | `pkg/pack/enum.go:129-132` | `EnumConfig.ts:113-118` | DEVIATION | enum val-key with no comma: TS lookups empty string, Go errors (both fail-closed) |
| pack-config-4 | `pkg/pack/loc.go:456-463` | `LocConfig.ts:313-316` | DEVIATION | loc recol_d trailer: TS p2(NaN)→0 for missing slot, Go writes 0 (byte-identical) |
| pack-media-compiler-2 | `pkg/pack/compiler/symbols_test.go:153-186` | `ScriptVarType.ts:28-83` | COMMENT-LIE | TestScriptVarTypeName comment claims it mirrors getType but tests only 18 of 25 cases (verdict CONFIRMED LOW) |
| pack-media-compiler-3 | `pkg/pack/compiler/symbols.go:608-629` | `tools/pack/Compiler.ts:219-227` | DEVIATION | populateInterfaceOverlay emits a symbol when Component.get(id) is nil; TS skips it |
| pack-media-compiler-4 | `pkg/pack/audio/midi.go:28-78` | `tools/pack/midi/pack.ts:7-35` | DEVIATION | PackMidi drops TS shouldBuild(jingles) whole-function gate; gates per-subdir |
| pack-media-compiler-5 | `pkg/pack/maps/pack.go:255-292` | `tools/pack/map/Pack.js:273-300` | DEVIATION | Map NPC/OBJ server streams emit zone entries ascending-key vs TS Map insertion order (decoded data identical) |
| pack-media-compiler-6 | `pkg/pack/graphics/pack.go:30-32` | `tools/pack/graphics/pack.ts:12` | DEVIATION | graphics/clientinterface/maps drop TS's second shouldBuildFile(packer-source) freshness gate |
| pack-media-compiler-7 | `pkg/pack/worldmap/csv.go:99-118` | `tools/pack/map/Worldmap.ts:639-653` | DEVIATION | parseLabels skips rows with !=4 fields; TS destructure keeps first 4 / NaN-writes <4 |
| pack-media-compiler-8 | `pkg/pack/clientinterface/pack.go:240-...` | `tools/pack/interface/PackShared.ts:286-...` | DEVIATION | comparator/slot-offset parts-length guards emit 0 where TS emits NaN-derived bytes |
| pack-media-compiler-9 | `pkg/pack/sprites/sprites.go:126-161` | `tools/pack/sprite/textures.ts:7-21` | CONFIRMED-EXCEPTION | PackTexture skips missing texture IDs; TS always writes 50 entries (production has all 50) |
| pack-media-compiler-10 | `pkg/pack/compiler/*` | n/a | N-A | RuneScript compiler is external @lostcityrs/runescript port — no Engine-TS counterpart |
| pack-media-compiler-11 | `pkg/pack/compiler/codegen/*` | n/a | CONFIRMED-EXCEPTION | Prior cluster S compiler 'stubs' are fully implemented external-port codegen |

### Extensions (net-new; LOW EXTENSION)
| id | go_loc | ts_loc | class | title |
|---|---|---|---|---|
| extensions-3 | `pkg/eventspb/`,`proto/events/v1/` | n/a | EXTENSION | Protobuf event envelopes (wealth/player_input partially echo TS WealthEventType/InputTracking) |
| extensions-4 | `pkg/friendspb/`,`proto/friends/friends.proto` | `FriendServer.ts` | EXTENSION | gRPC schema replacing TS in-process WS FriendServer (routing semantics ported) |
| extensions-5 | `pkg/loginpb/`,`proto/login/login.proto` | `LoginServer.ts` | EXTENSION | gRPC schema replacing TS in-process WS LoginServer (login/ban/mute semantics ported) |
| extensions-6 | `pkg/telemetry/` | `Metrics.ts`,`web.ts:167-181` | EXTENSION | OTel observability seam (PARTIAL overlap with TS prom-client; metric names not ported) |
| extensions-7 | `internal/dskit/` | n/a | EXTENSION | Grafana dskit port (service lifecycle) — not a TS port |

---

## Refuted / false-positive

Findings the adversarial verdict marked `real === false` (high/medium confidence). Retained with the refuter's reasoning; NOT in the main ledger.

| id | go_loc | ts_loc | original class/sev | refuter's reasoning |
|---|---|---|---|---|
| inventory-1 | `pkg/inventory/inventory.go` (no Transfer) | `src/engine/Inventory.ts:363-389` | MISSING / HIGH → **refuted to LOW** | Quotes verified accurate — TS `transfer()` exists, Go has no `Transfer` method (only the `transaction.go:7` doc comment). But the verdict reasoning corrects severity to LOW: the note/unnote inter-inventory move is not wired to any live goscape caller and `Items`/`Transaction` plumbing is unused; it is an unported helper, not a live behavioral bug. (Listed here because severity was downgraded from HIGH below the main-table threshold; the gap is real but inert.) |
| h-obj-1 | `handlers_obj.go:272` → `handlers_inv.go:480-499` | `ObjOps.ts:147`; `Player.ts:1496-1504` | DEVIATION / HIGH → **refuted to LOW** | Verdict: quotes accurate, but reasoning re-examined the reachability — TS `OBJ_TAKEITEM` uses `invAdd` default `assureFullInsertion=true` (all-or-nothing) then unconditionally removes the obj; Go routes through `performInvAdd` (`AssureFullInsertion:false` + overflow-to-ground). The behavioral divergence is real but the verdict downgrades it to LOW (the related COMMENT-LIE `h-obj-2` survives at MEDIUM as the accurate framing of the same code). |
| h-interface-2 | `modules/world/player_script.go:1119-1132` | `src/engine/entity/Player.ts:1928-1951` | COMMENT-LIE / LOW → **REFUTED** | The verdict marks this `real=false`: the cited WIRE deviation (Go OpenMain emits no eager IfClose) is real but is the SEPARATE finding `h-interface-1` (MISSING). The comment-lie framing here is not itself a distinct defect — the comment accurately describes the modal-bit + clear-suspended-script behavior it claims to match; only the wire-side IfClose is the (separately-tracked) gap. |
| friend-server-2 | `modules/friends/repository.go:64-67` | `FriendServer.ts:412-418`,`FriendServerRepository.ts:48-54` | COMMENT-LIE / LOW → **REFUTED** | The finding misreads which TS method the comment cites: `repository.go:64-67` cites `FriendServer.ts:412-418` (the socket-managing wrapper `FriendServer.initializeWorld`), which DOES drop the prior socket binding on re-init (`socketByWorld[world].terminate()`). The comment is accurate about the cited method. (The unconditional-reset behavior is the separate, surviving finding `friend-server-1`.) |
| script-opcodes-4 | `pkg/script/opcode_map.go:14-16` | `tools/pack/Compiler.ts:110-111` | COMMENT-LIE / LOW → **REFUTED** | Verdict `real=false`: the Go comment ("runServerCompiler sorts by opcode value before iteration (Compiler.ts:111)") is fully accurate at Engine-TS HEAD — `runServerCompiler` exists at `Compiler.ts:109`, and `:110-111` is `Array.from(ScriptOpcodeMap.entries()).sort((a,b)=>a[1]-b[1])`. Not a comment-lie; the citation and behavioral claim both check out. |

---

## Verified-clean / byte-faithful (aggregated byte_faithful notes)

- **Asset dispatch** (entry): handler.go archive dispatch reproduces every TS startsWith branch in order; `/crc` serves fresh CRC each request; MIME map + text/plain fallback exactly match web.ts; rs2.cgi `Debug && plugin==1` gate mirrors TS.
- **Config opcodes** (cfg-onl): opcode SETS for Obj/Npc/Loc/Inv match TS exactly (no missing/extra cases); NPC code 16=hasanim quirk faithfully mirrored; all NewXxxType defaults match.
- **Media-config** (cfg-media): HuntType M22 default FindNewMode=NONE=0 confirmed-fixed; HuntType opcodes 1-20/250 incl. g4s sign handling faithful.
- **Var-config** (cfg-var): all parse loops + g2 count headers + ConfigNames indexing match; ParamType DefaultInt=-1/AutoDisable=true; EnumType/DbRow/DbTable decode faithful.
- **Cache graphics** (cache-graphics): packModelStreams byte-faithful to pack.ts (18-byte trailer, ob_head field order, 13-segment copy order).
- **Wordenc** (wordenc): leet table getEmulatedBadCharLen byte-identical across all branches; both binary searches faithful; symbol-masking faithful.
- **Datastruct/DB** (datastruct-db): DoublyLinkList AddTail/Unlink/Size; GetFriends/GetIgnores ORDER BY created ASC; GetFollowers; IsVisibleTo staff→ignore→mode order.
- **Game map** (gamemap): CSV underscore format wired; multimap zoneIndex byte-identical; F2P-tile gates present; boot collision faithful.
- **Inventory** (inventory): Add stack predicate; previousCount==StackLimit short-circuit; AssureFullInsertion both branches; non-stack fill; stack find-or-allocate.
- **World tick** (world-tick): queue/engine/world-queue post-decrement delay; worldScriptQueue delay+1 distinction; logout removal sequence; timers-while-loggingOut early-return.
- **World ops** (world-ops): L37 login order rev→CRC→rsadec; L9 GetObj isValid filter; H2 obj cap eviction.
- **Player core** (player-core): XP table byte-matches; calcCombatLevel float64+floor+max; SAV format byte-for-byte vs PlayerLoading.ts.
- **Player scripting** (player-script): writeVarp threshold encoders; GetTimer absolute clock; SetTimer pop order; processPlayerQueue !CanAccess gate; post-decrement delay.
- **Player net** (player-net): generateAppearance byte-faithful; updateBuildArea camera drain + zone-trigger; processIn opcalled reset.
- **NPC core** (npc-core): NpcStat enum; changeType boost-drain formula; damage overkill clamp; processNpcRegen 6-slot convergence; processNpcTimer.
- **NPC AI** (npc-ai): NPCMode constants; checkOp/ApTrigger band ranges; aiMode 1:1; turn() phase order; validateTarget 4 gates.
- **NPC hunt** (npc-hunt): Hunt enums exact; processNpcHunt nobodyNear gate; huntAll rate-throttle/random pick; Chebyshev distance; zone span.
- **Pathing** (pathing): M2 per-step focus; M1 player AP LoS; M3 validateDistanceWalked edge-aware.
- **Interaction** (interaction): M1 approachHasLineOfSight; +7 AP→OP offset byte-exact.
- **Entity base** (entity-base): EntityLifeCycle/WalkTriggerSetting enums; Loc bit-packing; Obj REVEAL=100; turnObj reveal countdown.
- **Tracking** (tracking): InputTracking RATE/TIME/LEEWAY/jitter; IsActive; OnCycle order; submitEvents 4 branches; EVENT_TRACKING gate order.
- **Script core** (script-core): isLargeOperand; opcode values; activePlayer()/activePlayer2() getters; popArgsForTarget.
- **Script iter** (script-iter): zone-walk order; bounds math; distanceToSW; filter ORDER (DISTANCE/HuntAll/ZONE); LoS/LoW asymmetry.
- **Script opcodes** (script-opcodes): all 393 opcode ids match; ScriptOpcodeMap 393 keys identical; pointer-requirement table; trigger enum.
- **h-player/h-interface/h-npc/h-inv/h-obj/h-loc/h-server/h-number/h-string/h-db/h-core/h-config**: STAT arithmetic; pop orders; validator bounds; ENUM dispatch (present-value); STRUCT/OC/NC/LC_PARAM routing; bit-ops 0-31; ADD/SUB/MUL/DIV/MOD/POW/INVPOW math; Trig table (0 mismatches over 16384 entries); ATAN2 const bit-identical; all DB opcodes; javaStringCompare/SUBSTRING.
- **Net protocol** (net-client-prot / net-server-prot / net-server-enc): all 78 client + 71 server opcode (id,size,name) triples byte-identical; all 36 decoders + 72 encoders byte-faithful field order/width; rgb24to15; CamLookAt/MoveTo field order; ISAAC writeOut.
- **I/O** (io-packet/io-isaac/io-jagfile/io-bzip2): G1-G8/P1-P8 byte orders; PSmart/PSmartS; GBit/PBit; ISAAC BIT-IDENTICAL over 4 seeds incl. 1M-draw checkpoint; CRC32; genHash; bzip2 framing (BZh1 strip/prepend, length prefix).
- **Login/friend** (login-server/friend-server): verifySave magic/version/CRC; response-code→wire mapping; H14 staff bypass; H15 ignore-suppression; M29 100-cap (add path).
- **Pathfinder** (pathfinder): all CollisionFlag constants incl FlagNull; LocShape/Angle/Layer; CanMove all 5 types incl LineOfSight shuffle; routeFindSize1/2 all 8 dirs (H9 NE-SW fixed); lineScaleDown signed shift.
- **rsbuf** (rsbuf-player/npc/zone): local-player leaves byte-pinned; appearance-dedup; mask-payload field order vs message.rs; all 10 zone opcodes + payload widths.
- **Pack** (pack-config/pack-media-compiler): npc opcode 214 emit+decode; full emit/decode cross-check for every config packer; maps land/loc encoders; worldmap land decode; BZip2Compress defaults.

---

## Deferred / PORTING-EXCEPTION markers

Merged per-unit `deferred_markers` + the deferred-marker-sweep critic. Assessment column reflects the sweep verdict.

| go_loc | marker | assessment | TS status |
|---|---|---|---|
| `modules/asset/handler.go:36-57` | M28 panic guard / L47 .mid-matched-first | CONFIRMED-EXCEPTION | Both re-verified correct; `if us<1` guard prevents panic; TS substring also 404s |
| `modules/world/server.go:782-788` | L48 WS open-seed first word masked 0x00ffffff | CONFIRMED-EXCEPTION | Logic in WORLD module; asset delegates to world.HandleConn matching TS |
| `cmd/goscape/app/config.go:77-82,91` | CheckConfig TODO / ConfigWarnings | CONFIRMED-EXCEPTION | No TS counterpart (goscape scaffolding) |
| `modules/asset/asset.go:14,27,...` | TODO tracer/mine/unused; dead subservices | CONFIRMED-EXCEPTION | Internal dskit scaffolding; TS asset is a single fetch handler |
| `modules/asset/handler.go:68 (8 sites)` | check http.Dir.Open path sanitization | CONFIRMED-EXCEPTION | Aspirational hardening; fixed paths via ServeFile, matches TS |
| `cmd/goscape/app/modules.go` | idle-service for disabled modules | CONFIRMED-EXCEPTION | Module-enable gating is dskit-only; no 1:1 TS equiv |
| `pkg/objtype/objtype.go:245-248`,`npctype.go:207-216`,`loctype.go:104-113` | Op codes 30-34/30-39 stored verbatim incl 'hidden' | CONFIRMED-EXCEPTION | VERIFIED TRUE vs ObjType/NpcType/LocType; 'hidden' gated at handler |
| `pkg/pack/npc.go:334-335,491` | huntrange p1 (TS oversight) mirrored | CONFIRMED-EXCEPTION | VERIFIED TRUE; self-consistent byte-faithful |
| `pkg/pack/obj.go:537-543` | cert reverse-lookup !isCert guard | CONFIRMED-EXCEPTION | Plausible-and-consistent; identical output |
| `pkg/objtype/seqtype.go:176,68` | ByName linear-scan fallback / L97 delay fallback | CONFIRMED-EXCEPTION | Additive robustness; comment doesn't disclose bounds guard (cfg-media-1) |
| `pkg/objtype/spotanimtype.go:60-67`,`idktype.go:49-71` | OOB recol_s/recol_d silent-discard | CONFIRMED-EXCEPTION | Accurate; matches JS TypedArray silent-discard |
| `pkg/objtype/flotype.go:11-14` | minimal binary view scope | CONFIRMED-EXCEPTION | Accurate; full-field TS FloType reduced by design |
| `pkg/objtype/hunttype.go:213` | CheckInv forward-looking note | CONFIRMED-EXCEPTION | About a consumer, not HuntType decode |
| `pkg/script/handlers_npc.go:154-164` | S7f-D3 no CategoryType loader | ✅ **FIXED** `fix/categorytype-port` (`9968b923`) | TS CategoryType.ts:12-66 IS a runtime loader; real gap (cfg-var-9/h-npc-3). CLOSED 2026-06-01 — loader + Provider + reload arm + full-bound validator. See docs/PORTING-CLOSED.md. |
| `pkg/objtype/npcmode.go:14-36` | NAI-201 QUEUE1..20 omitted | CONFIRMED-EXCEPTION | TS keeps them commented out; Go matches |
| `pkg/objtype/varptype.go etc` | printError log+continue | CONFIRMED-EXCEPTION | Verified vs VarPlayer/Npc/Shared; matches printError |
| `pkg/objtype/dbtabletype.go:11-13` | per-column flag constants future-use | CONFIRMED-EXCEPTION | Informational; consumed by DbTableIndex (ported) |
| `pkg/pixpack/convert.go:48-51` | NAI-213 quantize missing | **STALE-DEFER** | TS ACTUALLY implements quantize; real gap (cache-graphics-4) |
| `pkg/pack/sprites/sprites.go:122-125` | PACKTEXTURE missing-id skip | CONFIRMED-EXCEPTION | TS crashes on missing ids; Go robustness |
| `pkg/pack/graphics/pack.go:21`,`sprites.go:27` | NO-SRC-NO-OP mirror | CONFIRMED-EXCEPTION | TS freshness gate yields same no-output |
| `pkg/wordenc/encfilter/tlds.go:10`,`domains.go:76`,`badwords.go:121` | TS-faithful dead stores/back-refs | CONFIRMED-EXCEPTION | Verified TRUE vs TS dead stores/redundant checks |
| `pkg/wordenc/wordpack/wordpack.go:1`,`pkg/pack/wordenc/pack.go:23` | NAI-158 / NO-SRC-NO-OP | CONFIRMED-EXCEPTION | Internal tag / faithful no-op |
| `modules/world/obj_delayed_queue.go:10` | NAI-134-D1 LinkList→slice | CONFIRMED-EXCEPTION | Behavior identical vs World.ts:563-573 |
| `modules/friends/db.go:21` | Arc 18 DB-2 schema-decoupling | CONFIRMED-EXCEPTION | Deliberate federation (username37, no FK) |
| `pkg/gamemap/gamemap.go:107,126` | LayerWallDecor no-op | CONFIRMED-EXCEPTION | Verified vs changeLocCollision (no WALL_DECOR branch) |
| `pkg/gamemap/load.go:211,232,237` | defensive / printFatalError on missing LocType | CONFIRMED-EXCEPTION | Verified vs TS |
| `pkg/gamemap/load.go:71` | op≥82 no-op | CONFIRMED-EXCEPTION | Verified vs loadGround pass-1 |
| `modules/world/player.go:1093-1098` | lastZone=-1 first-tick → isMulti false | CONFIRMED-EXCEPTION | Verified isMulti(-1)→false |
| `pkg/inventory/inventory.go:57-63,310-...` | L10-L13/M10/M11 stock-seed/clamp/wrap | CONFIRMED-EXCEPTION | ACCURATE (L12 load-bearing BeginSlot -1 sentinel) |
| `modules/world/tick_recovery.go:54-58` | ARCH-1 world-script panic swallowed | CONFIRMED-EXCEPTION | TS console.error+continue (no retry); matches |
| `modules/world/tick.go:316-319` | NAI-182-D4/D5 login emit | UNVERIFIED | onLogin-emit details outside tick-loop scope |
| `modules/world/player_script.go:1562-1568` | NAI-162 WEALTHEVENT in-memory only | **STALE-DEFER** | TS World.addWealthEvent groups+dispatches; real gap (logger-transport-5) |
| `modules/world/world_state_ops.go:167-179` | NAI-S5B no-goscape-queue (ClearLogouts) | CONFIRMED-EXCEPTION | TS logoutRequests is real; goscape uses saveWg |
| `modules/world/tick.go:656-660`,`player_script.go:389-390` | NAI-144-D4 CanAccess no shutdown branch | **STALE-DEFER** | 'no shutdown flag' claim contradicted by reboot.go:34/tick.go:67 (world-tick-2) |
| `modules/world/world_state_ops.go:87-103` | NAI-186-D1 KickPlayer defers teardown | CONFIRMED-EXCEPTION | TS inline logout+close; deferred to processLogouts |
| `modules/world/server.go:1211-1217` | heroPoints.clear omission | CONFIRMED-EXCEPTION | Fresh *Player per login; unobservable |
| `modules/world/loc_turn.go:11-14`,`obj_turn.go:19-22` | D-N86-4 absolute-tick | CONFIRMED-EXCEPTION (sweep) / DEVIATION (entity-base-1/2) | TS decrements per-tick; off-by-one is the live finding entity-base-1 |
| `modules/world/server_varp.go:166-172` | AddNpcAt unknown-typeID error | CONFIRMED-EXCEPTION | Defensive extension |
| `modules/world/player_load.go:40-62` | VerifySave no world caller | CONFIRMED-EXCEPTION | Account/login-server layer owns verify |
| n/a wouldResetSaveFile | unported playtime-regression guard | CONFIRMED-EXCEPTION | LoginServer.ts (account-server), out of player-core scope |
| `modules/world/heropoints.go:64-77` | TopContributor 'Stub' | **STALE-DEFER** | TS findHero implemented + consumed; method is live FINDHERO (util-2) |
| `modules/world/player_gender.go:59-66` | NAI-SETGENDER unmapped idkit→-1 | CONFIRMED-EXCEPTION | TS literally `Map.get ?? -1`; byte-faithful |
| `modules/world/player_script.go:1032-1043` | NAI-111-D1 CloseModal no protect clear | CONFIRMED-EXCEPTION | Modeled via activeScript pointer |
| `modules/world/player_inv_cheat.go:16-18` | NAI-184-D2 invAdd nil→0 | CONFIRMED-EXCEPTION | Admin-cheat-only; benign |
| `modules/world/player_script.go:589-594` | NAI-65 D4 lastStep dead-write | CONFIRMED-EXCEPTION (sweep) / COMMENT-LIE (pathing-6) | lastStep IS read via followX/Z; comment-lie pathing-6 |
| `modules/world/npc_ai.go:46-48,85-92` | ARCH-1 sync despawn / M3 validateDistanceWalked | STALE-DEFER (despawn) / CONFIRMED-EXCEPTION (validateDistanceWalked) | 'tick_recovery covers panic' false (npc-ai-8); jump no-op verified |
| `modules/world/npc_masks.go:259,261` | moveSpeed=WALK / interacted+exactMove deferred | STALE-DEFER / CONFIRMED-EXCEPTION | moveSpeed near-inert (pathing-10); interacted never read on NPC |
| `modules/world/npc_interaction.go:43-45` | resetDefaults stripped-flat | **STALE-DEFER** | Real partial deviation (npc-core-1) |
| `modules/world/npc_interaction.go:404-411`,`movement.go:167-172` | NAI-176 Fly bypass | CONFIRMED-EXCEPTION | TS implements Fly; Go ports it (dead in content) |
| `modules/world/npc_player_modes.go:60-61` | playerFollowMode SMART deferred | **STALE-DEFER (UNVERIFIED)** | Go DOES implement pathToTargetSmart; wording stale |
| `pkg/script/queue.go:47-51` | NAI-123 args[]→LastInt | CONFIRMED-EXCEPTION | args always [] at sole enqueue |
| `modules/world/npc_script.go:198-209` | Teleport D4/D5 dead-API | CONFIRMED-EXCEPTION | No NPC reader consumes lastStep/jump |
| `modules/world/player_masks.go:87-94,139-142` | NAI-91 mask-reset 1-tick-lag | CONFIRMED-EXCEPTION | TS also resets at tick-end |
| `modules/world/interaction.go:600-606` | NAI-148 OPFIRE fallback | CONFIRMED-EXCEPTION | NodeDebug-only path |
| `modules/world/interaction.go:941` | NAI-11/92 SMART branch | CONFIRMED-EXCEPTION | Now fully implemented |
| `modules/world/interaction_trigger.go:298-317` | apRangeCalled no-op for NPC | **STALE-DEFER** | Contradicted by same file + effectiveApRange (interaction-8) |
| `pkg/script/handlers_b0_stubs.go:11-41` | PUSH/POP_VARBIT, LC_OP, OC_IOP, OC_OP unimplemented | CONFIRMED-EXCEPTION | TS declares but registers no handler; both abort |
| `pkg/script/configs.go:39-47` | NAI-121-D3 degraded var-type lookup | UNVERIFIED (script-core) / DEVIATION (h-config-6) | TS check() throws; live finding h-config-6/h-core-7 |
| `pkg/script/state.go:421-427` | DB_FIND* deferred comment | UNVERIFIED | Out of script-core scope (db-handler audit) |
| `pkg/script/handlers_player.go:1988-1993` | P_OPHELD unimplemented stub | CONFIRMED-EXCEPTION | TS also throws unimplemented |
| `pkg/script/handlers_player.go:608` | StatRandom JavaRandom vs rand/v2 | CONFIRMED-EXCEPTION | No JavaRandom port; range-equivalent |
| `pkg/script/handlers_player.go:111-136` | checkLocAngle/Shape retained unused | CONFIRMED-EXCEPTION | LocOps concern, not PlayerOps |
| `pkg/script/handlers_npc.go:132-164` | M14 queue-range / S7f-D3 categorytype | CONFIRMED-EXCEPTION / ✅ **FIXED** `fix/categorytype-port` (`9968b923`) | M14 correct; categorytype real gap (h-npc-3). CLOSED 2026-06-01 — full TS CategoryTypeValid bound check at checkCategoryType. See docs/PORTING-CLOSED.md. |
| `pkg/script/handlers_npc.go:453-...` | NAI-126/125/127 nil-World guards | CONFIRMED-EXCEPTION | Unreachable defensive |
| `pkg/script/handlers_npc.go:1397-1398` | NAI-160 attackrange widen | CONFIRMED-EXCEPTION | Value-identical |
| `modules/world/npc_script_lookup.go:111-121` | S7f-D2 FINDEXACT linear scan | CONFIRMED-EXCEPTION | First-match modulo iteration order (h-npc-7) |
| `pkg/script/handlers_inv.go:1698,...` | NAI-162 RecipientSession deferred | **STALE-DEFER** | TS DOES implement recipient_session (STAKE/TRADE/PVP) |
| `pkg/script/handlers_inv.go:401,...,1179-1181` | NAI-130-D2/D3 nil-World/Configs; replaceCount unvalidated | CONFIRMED-EXCEPTION | Defensive / faithfully omitted |
| `pkg/script/handlers_obj.go:78,235-...,215,344` | NAI-115/153 retired markers; UID-vs-hash64 | CONFIRMED-EXCEPTION / **STALE-DEFER (153-D3)** | invAdd mechanism differs (h-obj-1/-2) |
| `pkg/script/handlers_b0_stubs.go:21-35` (loc) | LC_OP stub | CONFIRMED-EXCEPTION | TS declares, no handler |
| `modules/world/loc_turn.go:13-14` | D-N86-4 absolute-tick equivalent | CONFIRMED-EXCEPTION (sweep) / DEVIATION | Live off-by-one (entity-base-1) |
| `pkg/script/loc_iterator.go:14-18` | getAllLocsSafe equivalent | **STALE-DEFER** | Reverse + isValid-filtered diverge (h-loc-4/-5) |
| `pkg/script/handlers_string.go:14-30,195,...` | checkMesanim/checkFont/SPLIT light-fidelity | CONFIRMED-EXCEPTION | Fully ported; 'stub' label stale-but-cosmetic |
| `pkg/script/handlers_db.go:9-18`,`tick_recovery.go:55-58` | find_db asymmetry / ARCH-1 | CONFIRMED-EXCEPTION / UNVERIFIED | Compile-time-only equivalent for valid blobs |
| `pkg/script/handlers_b0_stubs.go` (core) | PUSH/POP_VARBIT, LC_OP, OC_IOP/OC_OP | CONFIRMED-EXCEPTION | TS declares but no handler |
| `pkg/script/handlers_array.go:11-47` | array ops implemented (TS stubs) | CONFIRMED-EXCEPTION | Inverse: Go implements what TS throws on (h-core-4) |
| `pkg/script/handlers_vars.go:112-115`,`handlers.go:236-237` | MVP int-only VARS / 'stub until S6' | **STALE-DEFER** | TS dispatches by type; handlePushVarn fully implemented |
| `pkg/io/protocol login/server.go:1002,1004` | TODO save var / reconnecting check | UNVERIFIED | reconnecting captured; additional vars partial |
| `pkg/io/packet/load.go:16` | compressed=true not supported | CONFIRMED-EXCEPTION | Go-only extension; TS has no compression |
| `pkg/io/packet/packet.go:457,468` | PSmart/PSmartS out-of-range panic | CONFIRMED-EXCEPTION | TS throws; parity |
| `pkg/io/packet/buffer.go:233` | Go stdlib TODO | CONFIRMED-EXCEPTION | Inherited from Go SDK; no TS counterpart |
| `pkg/io/bzip2/bwt.go:33` | dsnet SA-IS TODO | CONFIRMED-EXCEPTION | Upstream library TODO |
| `pkg/cache/crctable.go:12-19`,`jagfile.go:109-...,158-...,217,339-...` | atomic-swap / TS-faithful array-grow / compressWhole / ordered-scan | CONFIRMED-EXCEPTION | Verified faithful |
| `modules/login/handler.go:298-307,154-160`,`db.go:221-227` | hiscores no-op / M27 / setLoggedOut placement | STALE-DEFER (hiscores) / CONFIRMED-EXCEPTION | Hiscores real gap (login-server-9) |
| `modules/friends/handler.go:30,39-...,357-...,464-...`,`subscriptions.go`,`world_subscriptions.go` | NAI-S1/S4A/S5A markers | CONFIRMED-EXCEPTION | Lazy world-init / drop-on-full / dumb-relay verified |
| `modules/world/logger_bridge.go:10-...`,`handler_event_tracking.go`,`input_tracking.go` | NAI-72/73/74/Phase2 | CONFIRMED-EXCEPTION / **STALE-DEFER (wealth)** | Logger sink by design; wealth real gap (logger-transport-5) |
| `modules/asset/websocket.go:50-58` | InsecureSkipVerify / pre-upgrade reject | CONFIRMED-EXCEPTION | Strict superset of TS |
| `modules/world/heropoints.go:64-66` (util) | 'Stub for future loot-routing' | **STALE-DEFER** | Live FINDHERO impl (util-1/util-2) |
| `pkg/util/jstring/jstring.go:44-46`,`handlers_player.go:598-608` | NAI-72 %37 invalid / JavaRandom | CONFIRMED-EXCEPTION | Verified faithful |
| `pkg/rsbuf/doc.go:7-40`,`buf.go:93-...`,`buildarea.go:30-38` | NAI-30/31/32 | CONFIRMED-EXCEPTION / UNVERIFIED | doc verified; buildarea resize affects only >250-track (rsbuf-player-1 budget) |
| `pkg/rsbuf/npcinfo.go:52-56`,`npc.go:5`,`renderer.go:110-116` | T3.2 skeleton / NAI-30/116 | STALE-DEFER (skeleton) / CONFIRMED-EXCEPTION | Code fully implemented; comment stale |
| `pkg/pathfinder/routefinder/*.go:38,66,90`,`nai94_repro_test.go` | error/panic TODOs; NAI-94 skip | CONFIRMED-EXCEPTION | Range-checked panics mirror TS throws; degenerate-case skip |
| `pkg/pack/*` (pack-config + pack-media-compiler markers) | NAI-191/194/213/192 build-verify/freshness/order/no-src | CONFIRMED-EXCEPTION / **UNVERIFIED (BUILDVERIFY-INTERFACE)** | BUILD_VERIFY gated off by default; interface-CRC relaxation is pack-media-compiler-12 |
| `internal/dskit/*`,`proto/friends`, goscape extension stubs | extension stubs/TODOs | CONFIRMED-EXCEPTION | No TS counterpart (goscape-only features) |

**STALE-DEFER markers needing comment cleanup (live findings exist):** `handlers_npc.go` S7f-D3 (h-npc-3), `pixpack/convert.go` quantize (cache-graphics-4), `player_script.go` WEALTHEVENT (logger-transport-5), `tick.go`/`player_script.go` NAI-144-D4 shutdown flag (world-tick-2), `heropoints.go` 'Stub' (util-1/util-2), `npc_interaction.go` resetDefaults (npc-core-1), `npc_player_modes.go` SMART (now implemented), `interaction_trigger.go` apRangeCalled no-op (interaction-8), `loc_iterator.go` 'equivalent' (h-loc-4/-5), `handlers_inv.go` RecipientSession, `handlers_obj.go` NAI-153-D3 (h-obj-1/-2), `handlers_vars.go`/`handlers.go` 'stub until S6'/'MVP int-only', `login/handler.go` hiscores (login-server-9), and the various `handlers.go` stale 'stub' headers (h-core-8 + sweep findings on P_WALK/SPLIT_*/Camera).

✅ CLOSED 2026-06-01 — `fix/categorytype-port` (`9968b923`) ports the loader + Provider + reload arm + full-bound validator for the gap-world-reload-events-8 / cfg-var-9 / h-npc-3 cluster. See docs/PORTING-CLOSED.md.

---

## Coverage

Gaps flagged by the TS-file coverage critic (TS files / Go packages with no assigned audit unit, plus per-unit 'uncovered' notes):

### TS files / Go packages flagged uncovered (coverage critic)

1. **`src/network/game/client/codec/*Decoder.ts` (36 files)** — the entire client codec decoder directory is UNASSIGNED (the server codec dir IS assigned to net-server-enc; the client one is not). These hold RS2 wire-decode logic; goscape inlines decoding into `modules/world/handler_*.go` (plausibly covered) but decode-field offset/G1/G2-order parity is unaudited. **Single largest TS-side coverage hole.**
2. **`src/network/game/client/model/*.ts` (39 files)** — all client message MODEL carriers unassigned; most trivial, but `AnticheatOpLogic.ts`/`AnticheatCycleLogic.ts`/`EventTracking.ts`/`MoveClick.ts` carry semantic anti-cheat/movement fields only partially traced.
3. **`src/network/game/server/model/*.ts` (69 files)** — net-server-enc assigns the codec dir but NOT the parallel server/model payload structs; ~50 encoders (Cam*/IfSet*/Data*/Tut*) have no individually named Go file.
4. **`src/network/{ClientMessage,ClientMessageHandler,ServerMessage}.ts`, `client/ClientGameMessage.ts`** — four network base-abstraction files unassigned (root interfaces the whole net layer extends).
5. **`src/engine/entity/Player.ts` runScript guard (~line 2094) → `modules/world/script.go`** — the load-bearing dupe-prevention `!force && protect && this.protect` guard is in `script.go` (245L), in NO unit's Go list. Parity-critical, dupe-preventing, unaudited.
6. **`src/engine/World.ts` reload()/hot-reload → `modules/world/reload.go` (283L), `content_watcher.go` (303L)** and **world-event pub/sub → `world_events_dispatcher.go` (70L), `world_events_subscriber.go` (135L)** — unreferenced slices of a 2333-line file.
7. **`src/engine/World.ts` friend-thread plumbing + `NetworkPlayer.ts` social → `friends_client.go` (316L), `friends_subscriber.go` (132L), `friends_emit.go` (66L), `social.go` (55L)** — the in-world side of the friend/social bridge is unaudited (friend-server unit covers only `modules/friends/`).
8. **`src/server/login/LoginClient.ts` login handshake → `pkg/io/protocol/login/req/req.go` (133L) + `resp/resp.go` (125L)** — the RS2 login wire packets (GameLogin struct, archive checksums, RSA block) are referenced by NO unit.
9. **`src/io/Packet.ts` framing → `pkg/io/protocol/protocol.go` + `rsakey.go`** — `Operation` + `CheckPacketLength` (the partial-TCP-read / dynamic -1/-2 size handling, foundational per CLAUDE.md) and RSA key loading are in no unit's Go list.
10. **`src/cache/DevThread.ts` → `content_watcher.go` + `reload.go`** — assigned to io-jagfile but mapped only to `cache.go`; actual file-watch/dev-reload behavior unreferenced.
11. **`src/engine/World.ts` config snapshot/broadcast → `config.go`, `configs_snapshot.go` (91L, DEVIATION-NAI-C atomic config swap), `data_map.go`, `admin_bridge.go`** — `configs_snapshot.go` encodes a documented deviation no unit reviews.
12. **`src/db/*` + `src/datastruct/*`** — collapsed onto two thin Go db files; most of the rich TS surface (Bun SQLite dialect/driver/query-builder, DoublyLinkList/HashTable/LinkList) has no real Go parity target (Go uses pgx + native slices/maps); coverage nominal not actual.
13. **`src/engine/script/handlers/PlayerOps.ts` (1265 lines)** — split across h-player/h-interface/h-config; mid-file op groups (design/stat/sound ops) risk falling between the seams. Flagged for verification, not a confirmed miss.

> Scope: 435 TS src files, **217 unassigned** (dominated by network model/codec/decoder/encoder files). Go side: 109 .go files in named parity pkgs; ~14 `modules/world` + ~6 `pkg/io` files unreferenced. Treated as intentionally covered at dir granularity: `pkg/io/bzip2/internal/**`, `pkg/pathfinder/*`, `pkg/pack/*`, the extensions list.

### Per-unit 'uncovered' notes (selected, highest-value)

- **cfg-onl/cfg-media:** downstream op-click 'hidden'/null gating handlers (OpObj/Npc/LocHandler) only confirmed to EXIST; their full gating logic is a separate unit. The verbatim-store decision is correct ONLY if those handlers honor `""==null` and the 'hidden' gate.
- **world-tick:** RPC-based login codec + onReconnect replace large parts of TS processLogins; the reconnect-replaces-session swap, logoutRequests-mid-flush rejection only spot-checked.
- **player-net:** `pkg/rsbuf` PlayerInfo/NpcInfo internal bit-stream NOT byte-verified here (separate rsbuf unit — which DID find rsbuf-player-1/rsbuf-npc-1 HIGH/MEDIUM).
- **npc-core/npc-ai/npc-hunt/pathing:** pathfinder reach/LoS internals traced only at the call boundary (pathfinder unit covers internals — found pathfinder-1/-2/-3 MEDIUM).
- **player-core:** account/login-server gate (PlayerLoading.verify on read+write, wouldResetSaveFile playtime-regression) spot-checked (login-server unit owns — found login-server-6 corrupt-existing-save divergence).
- **h-player:** entity-layer ActivePlayer impls (Teleport/ExactMove/AddXP bounds/SetGender/PlaySynth) only spot-checked (player-core unit owns — found player-core-3 negative-xp).
- **io-packet:** RSAEnc output bytes could not be diffed against a real node-forge run (io-packet-8 reasoned from big.Int semantics + Go-internal roundtrip).
- **extensions:** goscape-only extension internals NOT re-audited (no TS counterpart; functions_compared=0 by design).

---

## Net-new extensions (no TS counterpart)

From the extensions inventory. "Partial" rows mirror a ported TS concept over a different transport/mechanism.

| Go package | TS counterpart | Note |
|---|---|---|
| `pkg/eventspb` + `proto/events/v1` | none (generated) | 7 Kafka protobuf envelope schemas. wealth/player_input partially echo TS WealthEventType/InputTracking but TS transports as ad-hoc JSON over WS. |
| `pkg/friendspb` + `proto/friends/friends.proto` | `src/server/friend/FriendServer.ts` | PARTIAL: gRPC schema replacing the TS in-process WS FriendServer; RPCs map 1:1 to TS opcodes (routing semantics ported, transport+schema net-new). |
| `pkg/loginpb` + `proto/login/login.proto` | `src/server/login/LoginServer.ts` | PARTIAL: gRPC LoginService replacing the TS WS LoginServer; login/ban/mute semantics ported, transport+schema net-new. |
| `proto/` | partial | friends/login mirror TS microservice protocols; the events/v1 envelopes are net-new goscape schemas. TS uses zero protobuf. |
| `cmd/goscape-cli` | partial: `tools/pack/*`, `tools/server/setup.ts` | Multi-verb CLI: pack/jag/compile/worldmap verbs correspond to tools/pack; the remaining verbs are goscape-only. |
| `internal/dskit` | none (port of grafana/dskit) | Service lifecycle state machine, modules manager, server, signals, middleware, tracing. A Grafana port, NOT TS-derived. |
| `pkg/telemetry` | partial: `Metrics.ts` (prom-client) | OTel logs/metrics/traces exporters, ring buffer, Kafka shipper. TS counterpart is metrics-only Prometheus; no traces/logs/shipper. |

---

*End of ledger.*
