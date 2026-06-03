# TS-Parity Full Codebase Audit — Deviation Ledger

**Started:** 2026-05-23
**Subject (Go):** goscape HEAD `6589f9d1`
**Reference (TS):** Engine-TS HEAD `e1dea19f` (2026-02-23 "Synced with latest engine work") — pinned, no upstream drift vs handoff baseline.
**Wire oracle:** Client-Java (wire-format adjudication only).

Mission & rules: see `docs/superpowers/handoffs/2026-05-23-ts-parity-full-audit-resume.md`.

## Classification legend

- **MISSING** — TS branch/function not ported.
- **DEVIATION** — ported but computes differently.
- **WRONG-PATH / DEAD-CODE** — ported into a path that doesn't reach the wire/DB.
- **STALE-DEFER** — marked deferred but TS actually does it (real gap).
- **CONFIRMED-EXCEPTION** — Go differs but TS genuinely also stubs/TODOs it (cite TS SHA+line; fine).
- **COMMENT-LIE** — comment's claim contradicted by the cited TS.
- **N/A** — no TS counterpart (dskit, goscape extensions).

## Running summary

| Metric | Count |
|---|---|
| Packages audited | **AUDIT COMPLETE** — all in-scope clusters (Waves 1–4); dskit + goscape extensions + rsbuf-vs-Rust marked N/A per handoff |
| Functions compared | ~990 |
| Open: CRITICAL | 3 (NpcType opcode-214 VERIFIED-latent; gamemap CSV-format broken VERIFIED; CLIENT_CHEAT phantom byte — VERIFY) |
| Open: HIGH | 17 firm + 1 DISPUTED (faceSquare) |
| Open: MEDIUM | ~30 |
| Open: LOW | ~50 |
| CONFIRMED-EXCEPTION (closed) | ~11 |

### Byte-faithful subsystems (verified clean, high-value negatives)
ISAAC cipher · RSA login block · client+server opcode tables/sizes · bzip2 RS-framing quirk · CRC32 table · **wordenc chat-censor algorithm (0 deviations, security-correct)** · collision-flag constants + direction encoding · entity lifecycle re-model (relative→absolute, no off-by-one) · player-ops operand-awareness (`c58cac51` not regressed) · INVPOW nth-root (prior bug fixed).

### Top findings to triage first

**Wave 4:**
-3. **CRITICAL (VERIFIED) — `gamemap.loadCsvMap` broken vs real CSV format** (`multimap.go:37`): parses comma `level,x,z`; real format is underscore-packed `level_mx_mz_lx_lz` (two coords/line = a rect range). Every real line → `len(row)==2` → skipped. multimap+freemap load EMPTY → multi-combat detection + ALL F2P/members map-gating non-functional server-wide. Correct expander exists (`pkg/pack/worldmap/csv.go`) but never wired to `gamemap.Init`. Green test pins the fabricated format.
-2. **HIGH — `routeFindSize2` copy-paste typo** (`routefinder.go:325`): 2nd collisionFlag call passes `baseZ,baseZ` not `baseX,baseZ`; corrupts BFS neighbor test for size-2 (medium) entities moving diagonally, on the LIVE MOVE_CLICK/SMART path.
-2b. **HIGH — missing F2P/members gates** in `gamemap` `loadGround`/`loadLocs`/`loadNPCs`: collision/static-locs/NPC-spawns for member-only content recorded on an F2P server (`bordersFreeToPlay` doesn't exist in goscape). `loadObjs` correctly has the gate.
-2c. **HIGH — login save-integrity** (`modules/login/handler.go`): no `PlayerLoading.verify` (corrupt save served to client / garbage blob persisted) + no `wouldResetSaveFile` anti-rollback (stale/reordered save clobbers newer progress).
-2d. **HIGH — friend online-visibility** (`repository.go` IsVisibleTo): staff online-status bypass + ignore-list online-suppression both absent (`staffLvl` stored, never consulted).

**Wave 3:**
-1. **CRITICAL (VERIFIED, latent) — NpcType decoder missing opcode 214 (regenrate)** (`npctype.go:294`): goscape's OWN packer emits `P1(214)+P2(value)` for any `regenrate=` line (`pkg/pack/npc.go:525`), but the decoder has cases 200-213 then jumps to 249 — opcode 214 hits `default → fmt.Errorf("unrecognized npc config code 214")`, aborting the entire NPC config load. `RegenRate` (field exists, default 100) can never be set from cache. **Verified:** no shipped Content `.npc` sets regenrate (grep=0), so latent today; the comment at `npctype.go:331` *lists* "214 (regenrate)" as handled = comment-lie. One-case fix.
0a. **HIGH→CRITICAL-blast — NPC operand gap fully scoped: ~37 opcodes** (`handlers_npc.go`): every NPC read/mutate handler reads `s.ActiveNpc` directly; none resolve operand 0/1. `.npc2` ops in NPC→NPC AI target SELF not the secondary NPC. Exact twin of player fix `c58cac51`. Fix = one `s.activeNpcResolved()` accessor + operand-aware `requireActiveNpc` + ~35 call-site swaps. Affected opcodes listed in cluster J.
0b. **HIGH — INV_MOVEITEM + INV_MOVEFROMSLOT silently destroy overflow** (`handlers_inv.go:761,828`): both discard the `toInv.Add(...)` tx instead of dropping unplaced units to the floor like TS. Item-loss whenever destination inv is full. Sibling INV_MOVEITEM_CERT handles it correctly — inconsistent omission, not by design.

**Wave 2:**
0. **CRITICAL (VERIFY) — CLIENT_CHEAT phantom leading byte** (`handlers_game.go:594`): handler reads a `G1()` before `GJStrLF()`, but TS `ClientCheatDecoder` reads only `gjstr()` and the Java client sends no body-leading byte → Go eats the first character of every `::` command (`::reload`→parses `eload`). Comment "unused ctrlHeld-style byte per TS" is a LIE; companion test (`handler_cheats_supermod_test.go:45`) fabricates the byte to stay green. Wire-oracle-backed but "all cheats broken" warrants a live repro before fixing.
1. **HIGH — NPC operand gap** (`handlers_npc.go:184` + 52 read sites): NPC read/mutate ops not operand-aware; `.npc2`-prefixed ops in NPC→NPC AI read/write SELF not target. Unfixed twin of shipped player fix `c58cac51` (Arc 30 #5). Untracked.
2. **HIGH — per-zone obj cap eviction missing** (`zone.go:263`, TODO-marked): >129 dynamic objs/zone diverge (Go keeps all, TS evicts oldest DESPAWN).
3. **HIGH — shop restock/decay never ported** (`tick.go:866`): cross-confirmed by 3 agents; InvType stock fields loaded, never consumed; shops never replenish/decay.

**Wave 1:**
4. **HIGH — queue/engine-queue delay off-by-one** (`tick.go:508`): post-decrement gating fires QUEUE/WEAK/LONG/STRONG scripts one tick early vs TS pre-decrement. Test pins the buggy contract.
5. **HIGH — logout pipeline incomplete** (`tick.go:403`): no `closeModal()`, no `canAccess()`/queue-drain gate, LOGOUT content-trigger (`TriggerLogout`) dispatched nowhere.
6. **HIGH + COMMENT-LIE — NPC `defaultMode` re-derived** (`npc_interaction.go:1073`): ignores parsed `typ.DefaultMode`; comment "Matches TS NpcType.defaultmode" false; test pins it.
7. **DISPUTED HIGH/LOW — `faceSquareX/Z` never reset per tick**: Player agent → stale-square leak (HIGH); Npc agent → intentional Arc-30 #202 architecture (LOW). **Adjudicate before any fix** — cluster D + adjudication note.

## Audit progress by package

| Package | TS reference | Status |
|---|---|---|
| `modules/world` — tick loop | `World.ts` main loop ↔ `tick.go` | ✅ DONE (cluster A) |
| `modules/world` — movement/interaction interleave | `PathingEntity.ts`, Player/Npc interaction ↔ `movement.go`,`interaction.go` | ✅ DONE (cluster B) |
| `modules/world` — Player | `Player.ts` ↔ `player*.go` | ✅ DONE (cluster C) |
| `modules/world` — Npc + hunt/AI | `Npc.ts`,hunt ↔ `npc*.go` | ✅ DONE (cluster D) |
| `modules/world` — zone/build-area | `zone/`,`BuildArea.ts` ↔ `*_zone.go`,build | ✅ DONE (cluster E) |
| `modules/world` — inventory | `Inventory.ts` ↔ `inv_*.go` | ✅ DONE (cluster F) |
| `modules/world` — network handlers | `src/network/game/client/*` ↔ `handler_*.go` | ✅ DONE (cluster G) |
| `pkg/script` — runner core | `ScriptRunner/State/File/Provider/Iterators` | ✅ DONE (cluster H) |
| `pkg/script` — PlayerOps | `PlayerOps.ts` ↔ `handlers_player*.go` | ✅ DONE (cluster I) |
| `pkg/script` — Npc/Server ops | `NpcOps/ServerOps.ts` ↔ `handlers_npc/server.go` | ✅ DONE (cluster J) |
| `pkg/script` — Inv/Obj/Loc ops | `InvOps/ObjOps/LocOps/config` ↔ `handlers_inv/obj/loc/config.go` | ✅ DONE (cluster K) |
| `pkg/script` — Core/Number/String/Db | `CoreOps/NumberOps/StringOps/DbOps` etc. | ✅ DONE (cluster L) |
| `pkg/objtype` | `src/cache/config/*Type.ts` | ✅ DONE (cluster M) |
| `pkg/pack` | RuneScript compiler/packers | ✅ DONE spot-check (cluster S; Arc-26 byte-parity not re-litigated) |
| `pkg/pathfinder` | movement/collision/routefinder | ✅ DONE (cluster P) |
| `pkg/io/packet`,`protocol`,`login`,`isaac` | `src/io/`,`src/network/` | ✅ DONE (cluster N) |
| `pkg/io/jagfile`,`bzip2`,`cache`,`pixpack` | `src/cache/`,`src/io/` | ✅ DONE (cluster O) |
| `pkg/gamemap`,`entity`,`wordenc` | zone/GameMap/wordenc | ✅ DONE (cluster Q) |
| `modules/friends`,`login`,`asset` | `friend.ts`,`login.ts`,`web.ts` | ✅ DONE (cluster R) |
| `pkg/rsbuf` | Rust crate (spot-check only) | ✅ PARITY (Arc-15 #12; out of deep TS-scope per handoff) |
| `internal/dskit/*` | N/A (dskit port) | N/A |
| goscape extension packages | N/A (no TS counterpart) | N/A |

---

## Findings

<!-- One section per package/cluster. Columns: Go loc | TS loc | Class | Severity | Finding | Evidence -->

### Cluster A — Per-tick pass ordering (`World.cycle()` ↔ `tick.go`)

TS phase order: processWorld(script-queue, objDelayed, huntPlayers) → processClientsIn → processNpcEventQueue → processNpcs → processPlayers(resume→queues→timers→engineQueue→interaction[movement interleaved]→energy→validateDistanceWalked) → processLogouts → processLogins → processZones → processInfo → processClientsOut → processCleanup → [shutdown] → [savePlayers t%1500] → coordLog → sessionLogs → wealth-flush → tick++.

Go order: [shutdown@top] → [autosave@top t%1500] → processClientsIn → processWorldQueue → processNpcEventQueue → processNpcHuntPlayers → processNpcs → processActiveScripts → processObjDelayedQueue → processPlayerTimers → processPlayerEngineQueues → processInteractionsPreMove → processPathing → processInteractionsPostMove → processEnergy → processLogouts → processLogins → processInfo → processZones → processClientsOut → processCleanup → processSessionLogs → tick++.

Per-player sub-pass order and npc-before-player ordering are faithful. The Arc-29 movement/interaction pre/post split verifies correct vs Player.ts:1200-1268.

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `tick.go:140-143` (none) | `Player.ts:733-735`,`PathingEntity.ts:304-315` | MISSING | MEDIUM | `validateDistanceWalked` not ported: TS forces `jump=true` when player moved >2 tiles and EXACT_MOVE unset, so client teleport-snaps vs gliding an impossible step. Go records lastTickX/Z but never checks. **Cross-confirmed by cluster B.** | `grep validateDistanceWalked` → 0 hits in modules/world; movement.go:79-80 sets lastTickX/Z only. |
| `tick.go:902-906` (none) | `World.ts:1160-1190` | MISSING | MEDIUM | Shop restock/decay per-tick pass not ported. TS restocks toward base stock (stockcount/stockrate) inside processCleanup world-inv loop every tick; Go loop only clears `inv.Update`. InvType carries Restock/AllStock/StockCount/StockRate (loaded, never consumed). Shops never restock/decay. | tick.go:902-906 world-inv loop has no restock; stockrate keywords only in pkg/pack/inv.go parser. |
| `tick.go:84` | `World.ts:346,353` | DEVIATION + COMMENT-LIE | MEDIUM | World script queue runs AFTER processClientsIn; TS runs processWorld (incl. queue) BEFORE processClientsIn. Comment claims "matches TS start-of-cycle ordering" — it is not start-of-cycle in Go. WORLD_DELAY'd script resuming same tick mutates state one pass later than TS. | tick.go:83 processClientsIn then :84 processWorldQueue; World.ts:346 processWorld then :353 processClientsIn. |
| `tick.go:127` | `World.ts:346/563` vs `:365` | DEVIATION + COMMENT-LIE | LOW-MED | processObjDelayedQueue runs AFTER processNpcs; in TS objDelayed (inside processWorld) runs BEFORE processNpcs. Comment describes TS doing the opposite of Go. NPC hunting a delay-0 dropped obj reacts one tick late. | tick.go:117 processNpcs then :127 objDelayed; World.ts:563 objDelayed precedes processNpcs L365. |
| `tick.go:146-147` | `World.ts:388,395` | DEVIATION | LOW | processInfo runs BEFORE processZones (TS reverse). Loc/obj despawn/respawn turns happen after reorient → 1-tick facing artifact (face a loc/obj the tick it despawns). Wire delivery unaffected (computeShared still precedes processClientsOut). | tick.go:146 processInfo then :147 processZones; World.ts:388/395 reverse. |
| `tick.go:60-65` | `World.ts:419-421` | DEVIATION + COMMENT-LIE | LOW-MED | processShutdown at TOP of tick body; TS at END (post-cleanup). Comment cites World.ts:419-420 (which is post-cleanup) but states inverse intent. TS lets doomed players run one final tick; Go differs by ~1 tick. | tick.go:60 top-of-body; World.ts:419 post-cleanup. |
| `tick.go:71-75` | `World.ts:423-426` | DEVIATION | LOW | autosavePlayers at TOP of tick body; TS savePlayers at END. Same cadence (t%1500), but saves pre-pass vs TS post-pass state. | tick.go:71 top gate vs World.ts:423 post-cleanup gate. |
| `tick.go:872-914` | `World.ts:1133-1156` | DEVIATION (benign) | LOW | processCleanup resets players→invs→npcs→zones; TS resets zones→players→npcs→invs. All four groups independent + complete before next tick → no wire effect. Pure intra-cleanup reorder. | tick.go:872/907/911 vs World.ts:1133/1137/1151/1156. |
| `player_script.go:1510` (in-mem) | `World.ts:444-457` | CONFIRMED-EXCEPTION | LOW | End-of-cycle wealth flush to loggerThread not in Go tick = documented NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY; analytics RPC deferred to goscape telemetry. Not a bug. | player.go:420 wealthLog; no wealth flush in tick loop. |

### Cluster B — Movement + movement/interaction interleave (`PathingEntity.ts` ↔ `movement.go`,`interaction.go`)

Pre/post-move interaction split **present and correct for BOTH players (split world passes) and NPCs (in-`aiMode` interleave, npc_interaction.go:215-239 ↔ Npc.ts:832-859)**. Fork-N-ways risk does not apply (NPC path never used the broken ordering). Verified-correct: waypoint queue, validateAndAdvanceStep recursion, run=2/walk=1 steps, MoveRestrict→collision mapping, width>1 axis-split, reorient, inOperableDistance branches, edge-aware distance.

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `movement.go:182-193` (`applyStep`); `npc_interaction.go:439-449` | `PathingEntity.ts:218-220` | MISSING | MEDIUM | Per-step `focus()` not called. TS `validateAndAdvanceStep` calls `focus(fine(moveX,w),fine(moveZ,l),false)` after every step → faceAngle points one tile ahead. Go applyStep updates pos+zone but never refreshes faceAngle → walking entity with no target shows stale walk-facing to newly-revealed observers (renderer falls back to faceAngle when faceSquare==-1). Adjacent to Arc-30 spawn-orientation. | Go applyStep has no focus call; TS L218-220 does; effectiveFaceCoord reads faceAngleX/Z fallback. |
| `interaction.go:750-763` (`inApproachDistance`) | `PathingEntity.ts:405` | DEVIATION | MEDIUM | Player-side inApproachDistance omits the `isApproached` (line-of-sight) gate that TS applies on the Player branch. NPC side correctly includes LOS (npc_interaction.go:907-912). Player can fire AP/ranged/magic trigger through a wall where TS blocks. Documented "DEVIATION S6l-D4" but live + asymmetric with NPC path. | interaction.go:762 distance-only; npc_interaction.go:907 has HasLineOfSight; TS L405 ANDs isApproached. |
| (none) | `Player.ts:733-735` | MISSING | LOW | `validateDistanceWalked` — same as cluster A (cross-confirmed). Player-only in TS. | grep finds only test/comment hits. |
| `movement_consts.go:8` | `PathingEntity.ts:138-142` | CONFIRMED-EXCEPTION | LOW | MoveSpeed.CRAWL 2-tick toggle (`lastCrawl`) not handled, but TS never assigns moveSpeed=CRAWL anywhere → unreachable dead branch in reference. Non-issue. | grep MoveSpeed.CRAWL in TS → only the read site, never a write. |

### Cluster C — Player entity state machinery (`Player.ts` + helpers ↔ `player*.go`)

Verified-correct (no row): SAV save/load round-trip, composeUID, combat-level calc, AddXP buffed/drained branches, setLevel clamp, updateEnergy (agility recovery + weight-loss formula), calculateRunWeight, VARP small/large threshold + transmit gating, CloseModal cascade + per-slot IF_CLOSE + COUNTDIALOG/PAUSEBUTTON clear, LONG-queue accelerate, Damage overkill clamp, stat/runenergy/runweight wire encoders, processPostDecode.

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `tick.go:508-509`,`:576-577` | `Player.ts:883-884`,`:643-644`,`:898-899` | DEVIATION (off-by-one) | HIGH | **Queue/engine-queue delay off-by-one — queued scripts fire one tick early.** TS `const delay = request.delay--` (pre-dec) fires when `delay<=0`; Go does `req.Delay--` then gates `if req.Delay>0 continue` (post-dec), firing at original delay `<=1`. delay≥1 fires one tick sooner. Engine queue masked (TS forces delay=0) but primary queue exposed. No `+1` compensation at enqueue. | tick.go:508 vs Player.ts:883; buggy contract pinned by player_engine_queue_test.go:86-96. |
| `tick.go:403-417`,`server.go:1242` | `World.ts:773-799` | MISSING | HIGH | **Logout pipeline omits `closeModal()`, `queueDiscardable`+engineQueue-empty+`canAccess()` gating, and the LOGOUT content-trigger.** TS removes player only when `canAccess() && engineQueue empty && queueDiscardable`, after closeModal, running `[logout,_]` via getByTriggerSpecific(LOGOUT). Go unconditionally writes OpLogout + closes socket once loggingOut && (force||tick>=preventLogoutUntil). `TriggerLogout`(158) defined but dispatched nowhere. | grep TriggerLogout modules/ → 0 consumers; World.ts:788-797. |
| `player_masks.go:63-92`,`player_source.go:32-33` | `PathingEntity.ts:608-609` | COMMENT-LIE / WRONG-PATH | **DISPUTED HIGH** | **`faceSquareX/Z` never reset per tick.** TS resets to -1 each tick. Go preserves them; comment claims "non-symptomatic; encoder gates on MaskFaceCoord which IS cleared". Player agent: claim false — low-def renderer FORCES MaskFaceCoord (renderer.go:49/55/129) and reads FaceSquareX()→effectiveFaceCoord()→persisted field → stale square renders to every newly-visible observer. **CONFLICTS with cluster D + memory #202 which says this persistence is the INTENTIONAL Arc-30 spawn-orientation fix.** See adjudication note below. | player_masks.go:64-66 comment vs PathingEntity.ts:608-609; renderer.go:49 forced; mask_payload.go:83-84 reads accessor. |
| `tick.go:514` | `Player.ts:884` | DEVIATION | MEDIUM | Primary queue gates on `p.delayed` only, not full `canAccess()` (`!protect && !busy()`, busy=delayed||modal). NORMAL/WEAK/LONG entries fire while a modal is open or protected script active — TS suppresses. (Engine-queue path correctly uses CanAccess.) | tick.go:514 vs Player.ts:884; CanAccess() exists at player_script.go:400. |
| `tick.go:513-517` | `Player.ts:877-893` | DEVIATION | MEDIUM | STRONG queue fires while `p.delayed`; TS has no STRONG firing exception — gated by canAccess() like all types (STRONG-specific behavior is only the modal-close, which Go does replicate at tick.go:488). | tick.go:514 QueueStrong carve-out absent in Player.ts:884. |
| `tick.go:625-630` | `Player.ts:933` | DEVIATION | MEDIUM | NORMAL timer gate uses `p.delayed` not `canAccess()`; misses modal-open + protected-script suppression and the `World.shutdown→force-fire` override. SOFT correctly bypasses in both. | tick.go:628 vs Player.ts:933. |
| `player_script.go:852-856`,`handlers_player.go:570` | `Player.ts:1740-1752` | COMMENT-LIE / DEVIATION | MEDIUM | **addXp `NODE_XPRATE` multiplier not applied — `node_xp_rate` config silently dead.** TS: `stats[stat] += xp * (allowMulti?NODE_XPRATE:1)`. Go AddXP adds raw xp. Comment "scaling is the Player implementation's responsibility" is false (AddXP never scales). NodeXPRate parsed (config.go:34) but read nowhere. Invisible at rate=1. | player_script.go:856 no *rate vs Player.ts:1752; grep NodeXPRate → declared, never consumed. |
| `player_script.go:1086-1122` | `Player.ts:1928-1973` | DEVIATION | MEDIUM | Modal-open methods do `p.modalState = modalStateMain` (assignment) wiping the TUT bit; TS does `|= ModalState.MAIN` and selectively clears only CHAT/SIDE. Also never clears a suspended COUNTDIALOG/PAUSEBUTTON activeScript (TS Player.ts:1947-1950). | player_script.go:1090 `=` vs Player.ts:1943 `|=`; no activeScript=nil clear. |
| `tick.go:601-648` | `World.ts:718-723` | MISSING | LOW | Timers fire while `loggingOut`; TS wraps both NORMAL+SOFT processTimers in `if (!player.loggingOut)`. | processPlayerTimers has no loggingOut guard; World.ts:718. |
| `tick.go:395-401` | `World.ts:765-768` | MISSING | LOW | `preventLogoutMessage` never emitted; TS messages player then nulls it when requestLogout && message set. Field set by SETPREVENTLOGOUT (player_script.go:1497) but never read on this path. | tick.go:395-401 resets flags only vs World.ts:765-767. |

### Cluster D — Npc entity state machine + hunt/AI (`Npc.ts` + hunt ↔ `npc*.go`)

Verified-correct: `processNpcHuntPlayers` now ported + wired (prior "never ported" bug fixed), `wanderCounter=0` on-move reset present (prior teleport-home bug fixed), `animID/animDelay` per-tick reset present (prior "persistent-by-design" lie fixed), huntClock gate, hunt range Chebyshev math, turn() ordering, processQueue/Timer/Regen, validateTarget, PLAYER* modes, spawn/despawn queue. **NpcMode QUEUE1..20 = CONFIRMED-EXCEPTION** (TS NpcMode.ts:147-167 still commented `// TODO: not used?`; Go forward-compat machinery is fine).

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `npc_interaction.go:1073-1088` (`defaultMode`) | `NpcType.ts:106,208`,`Npc.ts:100,414` | DEVIATION + COMMENT-LIE | HIGH | **NPC default mode re-derived at runtime from patrol/wander presence; TS reads stored `defaultmode` config field (default WANDER, opcode 210).** Parsed `typ.DefaultMode` (npctype.go:279) is ignored. Consequences: (1) NPC with patrol pts but implicit `defaultmode=wander` is force-PATROL in Go, WANDER in TS; (2) `wanderrange=0` non-patrol NPC is NONE in Go (no teleport-home recovery), WANDER in TS. Comment "Matches TS NpcType.defaultmode" is a LIE; TestNpcDefaultMode pins the wrong contract. | Go switches on len(PatrolCoord)/WanderRange, never reads typ.DefaultMode; npc_interaction_test.go:1186. |
| `npc_registry.go:121-179` (`resetEntityForRespawn`) | `Npc.ts:280-317` (`resetDefaults` :307) | MISSING | MEDIUM | On respawn TS `resetEntity(true)`→`resetDefaults()` sets targetOp=defaultmode + clearInteraction. Go resets stats/varns/hunt/queue/waypoints/heroPoints+unfocus but never resets target/targetOp/faceEntity/apRange/apRangeCalled/targetSubject. Self-heals after 1 tick (validateTarget fails on stale target) → 1-tick deviation. | grep targetOp/resetDefaults in npc_registry.go → none; Npc.ts:307. |
| `npc_masks.go:250-258` (`resetPathingEntity`) | `PathingEntity.ts:577-588` (:588) | DEVIATION | LOW | `apRangeCalled` not reset per-tick (TS resets every NPC every tick); Go relies on event-driven resets at SetInteraction/clearInteraction. Real per-tick-vs-event divergence for AP-range NPC scripts. Documented out-of-scope. | npc_masks.go:249 admits "AP-range deferred"; TS L588. |
| `npc_masks.go:195-234` | `PathingEntity.ts:608-609` | DEVIATION | LOW | `faceSquareX/Z` not reset per tick (NPC side of the disputed cluster-C finding). Npc agent assesses this as the **intentional** Arc-30 force-emit+fallback architecture, non-symptomatic. Recorded for completeness; see adjudication note. | npc_masks.go:196 doc "retained across ticks" vs TS L608-609. |

### ⚠ Adjudication note — `faceSquareX/Z` per-tick reset (clusters C & D conflict)

Two agents reached opposite conclusions on the same mechanism:
- **Cluster C (Player agent): HIGH bug / COMMENT-LIE.** Argues the "non-symptomatic" comment is false because the low-def renderer force-emits `MaskFaceCoord` and reads the persisted field, so a stale face-square renders to every newly-visible observer indefinitely.
- **Cluster D (Npc agent): LOW / by-design.** Argues this is exactly the Arc-30 (`69b1f11b`, memory #202) spawn-orientation architecture — TS resets to -1 each tick, but goscape's rsbuf renderer force-emits FACE_COORD on low-def and *requires* a persistent value, with `effectiveFaceCoord()`→`faceAngle` fallback.

**Memory #202 supports the Npc agent's "intentional" reading** but does NOT settle whether a *stale* square (set by a script-driven FaceSquare/FaceCoord that the entity then walks away from, without a new face) leaks to late-joining observers. Per cluster B's `focus()`-missing finding, a walking entity's faceAngle also goes stale — these two may be the same underlying gap (movement should refresh facing; without it the persisted faceSquare is the only signal and it's stale). **Do not fix until the live render path for a walked-away-from face target is traced against the Java client.** This is the #1 thing to resolve next session before any masks change.

---

### Cluster E — Zone system + build-area + map rebuild (`src/engine/zone/*`, `BuildArea.ts` ↔ `pkg/zone/*`, `*_zone.go`)

Verified-correct: ZoneGrid bit layout, ZoneMap pack/unpack, BuildArea rebuildZones (7×7 ∩ 13×13) + rebuildNormal reload window, login-enter/logout-leave/cross-step refreshZone, grid flag/unflag on 0↔1 player-count, updateZones unload-then-deliver order, LOC_MERGE coordinate plumbing (traced 4 hops, byte-faithful), obj-merge lifecycle equivalence (verified, comment NOT a lie), grid `>>` arithmetic-vs-logical (boolean outcome unaffected). No comment-lies found; the two real gaps are honestly TODO-marked.

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `pkg/zone/zone.go:263-287` (`AddObj`) | `Zone.ts:280-293` | MISSING | HIGH | Per-zone obj cap (`OBJS=129`) eviction not implemented. TS evicts first DESPAWN obj via `World.removeObj` when `totalObjs>=129`; Go appends unconditionally. >129 dynamic objs/zone diverge. Honestly marked `TODO(beyond-4b)`. | zone.go:261 TODO comment; TS Zone.ts:281. |
| `pkg/zone/zone.go:343-356` (`RevealObj`); `obj_turn.go:25` | `Zone.ts:304-323` | MISSING | MEDIUM | Obj-reveal tradeable/members gating absent. TS early-returns (no OBJ_REVEAL) when `\!tradeable || (members && \!NODE_MEMBERS)`; Go always reveals after 100 ticks. Untradeable/f2p-members private drop becomes public in Go, stays receiver-only in TS. Marked `TODO(beyond-4b)`. | zone.go:340 TODO; TS Zone.ts:310. |
| `tick.go:146-147` | `World.ts:388,395` | DEVIATION | LOW | processInfo before processZones (TS reverse) = documented NAI-93. Zone agent confirms benign for zone subsystem (turnLoc/turnObj don't read origin; ComputeShared completes before processClientsOut delivery). **Consolidated with cluster-A row — single documented deviation.** | tick.go:146-147; World.ts:384-395. |
| `player_zone.go:43-57` | `Zone.ts:142-146` | DEVIATION | LOW | Obj replay gates on tick-derived `CheckLifecycle(currentTick)` vs TS stored `obj.isActive`. Agree in all reachable states (despawn-tick edge masked by guard above). Reads derived state where TS reads stored flag. | player_zone.go:50; Zone.ts:142. |
| `obj_lookup.go:18-27` (`GetObj`) | `Zone.ts:353-360` | DEVIATION | LOW | GetObj lacks `isValid()` filter (count≥1) TS applies via getObjsSafe; a depleted obj could be returned by Go. Low impact (depleted objs removed promptly). | obj_lookup.go:20; Zone.ts:354/425. |

### Cluster F — Inventory (`Inventory.ts` ↔ `pkg/inventory/*`, `inv_*.go`)

Verified-correct: UpdateInvFull slot clamp to `min(capacity,width*height)` (prior Java-crash bug fixed + present), UpdateInvPartial/StopTransmit encoders, Add happy paths + partial-fill order + StackLimit short-circuits, Swap/Delete/Get/Set/Contains/IsFull/GetItemCount/NextFreeSlot. Most findings stem from the missing restock feature.

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `tick.go:866-930` (`processCleanup`) | `World.ts:1159-1190` | MISSING | HIGH | **Shop stock-restock/decay entirely unimplemented** (cross-confirmed by cluster A). Per-slot restock toward stockcount at tick%stockrate, over-stock decay, allstock general-store decrement — no Go counterpart. InvType.Restock/StockCount/StockRate/AllStock parsed but never consumed. Comment "Mirrors TS World.ts:1140-1147" is scoped to end exactly before the restock block (1159-1190), masking the gap. | grep stockrate modules/world → none; tick.go:894 comment. |
| `inventory.go:291-315` (`Remove`) | `Inventory.ts:243-251` | MISSING | MEDIUM | `assureFullRemoval` short-circuit absent: TS returns {completed:0} when `assureFullRemoval && hasCount<count`; Go always partial-removes. `RemoveOpts.AssureFullRemoval` field exists, never read. TS restock path calls with true. | inventory.go:139 field declared, body never reads it. |
| `inventory.go:306-308` (`Remove`) | `Inventory.ts:280,304` | DEVIATION | MEDIUM | stockObj count-0 retention missing: TS keeps exhausted slot live for stockobj; Go unconditionally nils slot at count 0. Combined with missing restock, drained shop slot vanishes instead of persisting at 0. | Go `if it.Count==0 { Items[i]=nil }` no stockObj guard. |
| `inventory.go:296-309` (`Remove`) | `Inventory.ts:256-316` | DEVIATION | LOW | beginSlot skipped-indices second pass missing. Latent (all live callers use BeginSlot:-1); required once restock ported. | single-pass loop vs TS skippedIndices 293-316. |
| `inventory.go:42-52` (`FromType`) | `Inventory.ts:66-73` | DEVIATION | LOW | StockObj seeding diverges: Go skips id==0 + falls back count=1 when StockCount==0; TS sets every index with literal stockcount. Cosmetic for normal data, not byte-faithful. | Go 43-49 vs TS 66-72. |
| `inventory.go:250-266` (`Add`) | `Inventory.ts:215-227` | DEVIATION | LOW | Latent BeginSlot zero-value footgun: stack branch keys on `BeginSlot==-1`; AddOpts zero value is 0. All 7 callers set -1 explicitly; future caller could silently diverge. | no default-guard in struct. |
| `inventory.go:269,284` (`Add`) | `Inventory.ts:229-237` | DEVIATION | LOW | Stack-overflow basis differs: TS clamps using per-slot `stackCount`; Go uses `GetItemCount(id)` (sum over all slots). Diverges only with duplicate stacks of one id in a stack-typed inv. | Go uses previousCount; TS uses per-slot. |
| `pkg/inventory/*` | `Inventory.ts:363-389` (`transfer`) | CONFIRMED-EXCEPTION | LOW | `Inventory.transfer()` not ported as method, but note/unnote cert logic correctly reimplemented in script layer (handlers_inv.go:1278/1369) and TS has ZERO callers of transfer(). Functionally covered. | grep '.transfer(' src → none. |

### Cluster G — Incoming network packet handlers (`src/network/game/client/*` ↔ `handler_*.go`)

All 78 ClientGameProt opcodes present in Go `Ops[]` with identical IDs/sizes/categories. Verified-correct: obj-receiver identity now consistent (p.uid both read+write — prior slot/uid bug fixed), MoveClick fork unified into `moveClickInner` ×3 (prior modal-close fork bug fixed), all decode field orders/widths, gate ordering (delayed/component-visible/inv-listener/HasAt/members/UnsetMapFlag), socialProtect/reportAbuse/muted gates, anticheat+NO_TIMEOUT accept-and-discard (matches TS unbound).

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `handlers_game.go:594` (`handleClientCheat`) | `ClientCheatDecoder.ts:11` | COMMENT-LIE + WRONG-PATH | **CRITICAL (VERIFY)** | Reads `_ = r.G1()` before `GJStrLF()`, but TS decoder reads ONLY `gjstr()` and Java client sends no body-leading byte (client.java:2840-2842, Packet.pjstr = bytes+LF). Go consumes the first char of every `::` command (`::reload`→`eload`). Comment "unused ctrlHeld-style byte per TS" is false. **VERIFY with live repro before fixing** — "all cheats broken" is a strong claim. | Java client.java:2840-2842; packet.go:248 GJStrLF; framing player.go:1251/1269 strips len-prefix. |
| `handler_cheats_supermod_test.go:45` | — | TEST-PINS-BUG | — | `dispatchCheat` helper prepends `P1(0)` "ctrlHeld byte (unused)" the real client never sends — the suite passes *because* it fabricates the phantom byte. Fix requires removing both handler `G1()` and test `P1(0)`. | handler_cheats_supermod_test.go:42-50. |
| `handlers_game.go:37` (opcode 70 → `handleNoTimeout`) | `IdleTimerHandler.ts:8-12` | MISSING | MEDIUM | IDLE_TIMER routed to no-op, but TS sets `requestIdleLogout=true` (unless NODE_DEBUG). Java client sends opcode 70 after 4500 idle cycles. Go ignores → idle clients never logged out via client's explicit idle signal. | Java client.java:7500-7503; IdleTimerHandler.ts:9; Go no-op handlers_game.go:208. |
| `handlers_game.go:300-312` (MoveClick) | `MoveClickHandler.ts:34-41` | DEVIATION | MEDIUM (latent) | Passes full `packed` slice to pathToMoveClick whose `[0]` is the START tile, not dest; TS passes `userPath` (==[dest] in non-routefinder mode). When `node_client_routefinder=false`, Go pathfinds to start tile not dest. Default config (routefinder=true) unaffected. | handlers_game.go:312; movement.go:241-249; MoveClickHandler.ts:41. |

### Cluster H — Script runner core (`ScriptRunner/State/File/Provider/Iterators` ↔ `pkg/script/runner.go` etc.)

Verified-correct (and several "by design" comments verified NOT lies): operand-aware `activePlayer()/activePlayer2()` (ScriptState.ts:214-229, 117 sites), `requireActivePlayer` operand-independence sound for the PLAYER path (init binds both self+target), VARP/VARN bit-16 secondary selector + protect gate, ScriptFile.Decode, GetByTrigger/Specific + lookup-key composition, ServerTriggerType enum, all 5 iterators (radius/zone-walk/filter order/LoS swap), ScriptOpcodePointers (237 entries), Gosub/Return reverse-pop, branch/array/join_string.

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `handlers_npc.go:184` (`requireActiveNpc`) + 52 read sites | `ScriptState.ts:246-252`; `NpcOps.ts:65,69,74,235` | DEVIATION | HIGH | NPC read/mutate ops NOT operand-aware. `requireActiveNpc` checks only `s.ActiveNpc`; handlers read it unconditionally. TS `activeNpc` getter resolves operand 0→_activeNpc, 1→_activeNpc2. `.npc2`-prefixed ops in NPC→NPC AI triggers read/write SELF instead of target. **Unfixed NPC twin of player fix `c58cac51` (Arc 30 #5).** Write-side + VARN + FINDHERO ARE operand-aware; bulk of read ops are not. Untracked in PORTING.md. | TS NpcOps reads operand-aware getter (65/69/74/235); Go handleNpcStat reads s.ActiveNpc directly. |
| `handlers_loc.go:13`/`handlers_obj.go:17` + read ops | `ScriptState.ts:269-275,289-295` | DEVIATION | LOW | LOC_*/OBJ_* read ops read only primary slot, ignore intOperand `.loc2`/`.obj2`. Write-side operand-aware but no read consumer. Documented `NAI-119-D`/`NAI-154-D` (state.go:363) — deferred until a content consumer surfaces. Lower impact than NPC row (no known consumers). | state.go:363; handlers_loc.go:14. |
| `handlers_core.go:8-19` (GOSUB/JUMP) | `CoreOps.ts:194-222`; `ScriptState.ts:388-405` | DEVIATION | LOW | Plain GOSUB/JUMP (computed id) pass nil args, popping none of callee's declared args; TS setupNewScript always pops intArgCount/stringArgCount. Latent: compiler emits GOSUB_WITH_PARAMS for static calls (handled correctly). | Go GosubCall(target,nil,nil); TS gosubFrame→setupNewScript. |
| `handlers_array.go:59-61` (SWITCH) | `CoreOps.ts:243-256` | DEVIATION | LOW | TS branches only when `if(result)` truthy (offset 0 doesn't branch); Go branches on map-key presence regardless of offset value. Benign: compiled offsets never 0, and 0+PC++ ≡ fall-through. | TS `if(result)`; Go `if ok`. |
| `pointer.go:7-28` | `ScriptPointer.ts:7-19` | DEVIATION | LOW | Pointer bit positions differ from TS (Go PtrActiveNpc=bit2, protected flags at 9-10, goscape PtrFindDb=bit8). Self-consistent, never serialized/compared cross-engine. Comment at pointer.go:20 cites ScriptPointer.ts:10 as if values match — they don't. | Go PtrActiveNpc=1<<2; TS ProtectedActivePlayer=2. |
| `state.go:54` (opcount guard) | `ScriptRunner.ts:144-148` | DEVIATION | LOW | Opcount cap off-by-one: Go aborts at `>=500_000`, TS at `>500_000`. Both ~500k, cosmetic, no content reaches it. | Go `>=`; TS `>`. |

---

### Cluster I — PlayerOps script handlers (`PlayerOps.ts` ↔ `handlers_player*.go`) — 122 opcodes

All 122 opcodes use operand-aware `s.activePlayer()/activePlayer2()` (fix `c58cac51` not regressed); raw `s.Self/Self2` sites verified correct against TS raw-field usage (P_OPPLAYER, BOTH_HEROPOINTS, FINDHERO-write).

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `handlers_player.go:1644-1648` (FINDHERO) | `PlayerOps.ts:1139` | DEVIATION (operand) + COMMENT-LIE | MEDIUM | FINDHERO reads ledger from `s.Self` (hard primary); TS reads operand-aware `state.activePlayer.heroPoints`. For `.findhero` (operand=1) TS sources from _activePlayer2. Comment "keep s.Self rather than the operand-aware accessor" + test `TestFindHero_PopulatedAlwaysSetsSelf2` pin the buggy contract; comment conflates the read (operand-aware in TS) with the write (raw _activePlayer2). | TS:1139 getter; Go reads s.Self. |
| `handlers_player.go:1759-1770` (P_PREVENTLOGOUT) | `PlayerOps.ts:628-629` | MISSING (validation) | MEDIUM | No `check(popString, StringNotNull)` on message + `check(popInt, NumberNotNull)` on ticks. Go accepts empty message / ticks=-1 where TS aborts the script. | Go pops with no check; TS:628-629. |
| `handlers_player.go:1430` (HINT_PL) | `PlayerOps.ts:977` | DEVIATION (operand) | LOW | Points arrow at `s.Self2.Slot()`; TS uses operand-aware `activePlayer2`. operand=1 differs. | TS:977 getter. |
| `handlers_player.go:2036` (P_LOCMERGE) | `PlayerOps.ts:928` | DEVIATION (operand) | LOW | Passes `s.Self` as merge-owner; TS passes operand-aware `activePlayer`. operand=1 differs. | TS:928. |
| `handlers_player.go:588-590` (STAT_ADVANCE) | `PlayerOps.ts:760-763` | DEVIATION | LOW | Go adds `checkStatID(id<21)` after NumberNotNull; TS validates ticks with NumberNotNull only, forwards out-of-range to addXp. Go aborts where TS doesn't. | Go extra checkStatID. |
| `handlers_player.go:1488-1494` (P_OPOBJ) | `PlayerOps.ts:996-998` | DEVIATION | LOW | On nil Configs/unregistered ObjType Go errors; TS ObjType.get returns default + silently returns. Reachable only without a registry. | Go returns error vs TS silent-skip. |

### Cluster J — NpcOps + ServerOps script handlers (`NpcOps.ts`/`ServerOps.ts` ↔ `handlers_npc.go`/`handlers_server.go`) — ~74 opcodes

**THE NPC OPERAND GAP (HIGH; agent assessed CRITICAL by blast radius).** TS `state.activeNpc` is operand-aware (0→_activeNpc, 1→_activeNpc2); Go stores `s.ActiveNpc`/`s.OtherActiveNpc` separately and every NpcOps read/mutate handler dereferences `s.ActiveNpc` directly, plus `requireActiveNpc` (`handlers_npc.go:184`) only nil-checks the primary. `.npc2`-prefixed ops (operand=1) in NPC→NPC AI scripts read/mutate the WRONG npc (or wrongly abort). Write/find side (`setActiveNpcSlot`) and VARN are operand-correct. **Fix mirrors `c58cac51`: add `s.activeNpcResolved()` + operand-aware `requireActiveNpc` + ~35 call-site swaps.** Affected opcodes (all DEVIATION, operand=1 wrong-path): NPC_TYPE, NPC_COORD, NPC_STAT, NPC_BASESTAT, NPC_NAME, NPC_HASOP, NPC_UID, NPC_CATEGORY, NPC_SAY, NPC_ANIM, NPC_FACESQUARE, NPC_CHANGETYPE, NPC_CHANGETYPE_KEEPALL, NPC_DAMAGE, NPC_DEL, NPC_DELAY, NPC_ARRIVEDELAY, NPC_QUEUE, NPC_SETTIMER, NPC_TELE, NPC_WALK, NPC_WALKTRIGGER, NPC_GETMODE, NPC_SETMODE (subject half), NPC_SETHUNT, NPC_SETHUNTMODE, NPC_RANGE, NPC_STATADD, NPC_STATSUB, SPOTANIM_NPC, NPC_HEROPOINTS (npc half), NPC_FINDHERO (read half), NPC_INRANGE, NPC_ATTACKRANGE, NPC_STATHEAL, NPC_PARAM (`handlers_config.go:328`). (~37 sites.)

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `handlers_npc.go:184` + ~37 read/mutate sites (listed above) | `ScriptState.ts:246-252`; `NpcOps.ts` various | DEVIATION (WRONG-PATH operand=1) | HIGH | NPC read/mutate ops not operand-aware (full list above). Untracked in PORTING.md. | Go reads s.ActiveNpc; TS operand-aware getter. |
| `handlers_npc.go:131-136` (`checkQueue`) | `ScriptValidators.ts:114` (QueueValid 0..19) | DEVIATION (documented) | MEDIUM | NPC_QUEUE/WALKTRIGGER queue-id range [1,20] vs TS [0,19]. Doc claims no script uses 0/20 — **unverified against Content**. | Go `if v<1||v>20`. |
| `handlers_number.go:388-432`, `handlers_server.go:48-85` (COORDX/Y/Z, INZONE, MOVECOORD, DISTANCE) | `ServerOps.ts:47-123` | MISSING (validation) | LOW | Skip `check(_, CoordValid)`; TS aborts on out-of-range coord, Go silently bit-masks. | Go raw bit-unpack; TS check(). |
| `handlers_server.go:108`, `handlers_map.go:448` (MAP_INDOORS, MAP_MULTIWAY) | `ServerOps.ts:139,376` | CONFIRMED-EXCEPTION | LOW | nil-World defensive guards Go adds; TS World always live. Behavior-neutral. | Go `if s.World==nil`. |

### Cluster K — Inv/Obj/Loc + config-lookup script handlers (`InvOps/ObjOps/LocOps` ↔ `handlers_inv/obj/loc/config.go`) — 91 opcodes

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `handlers_inv.go:761` (INV_MOVEITEM) | `InvOps.ts:521-530` | DEVIATION | HIGH | **Item-loss:** ignores `toInv.Add(...)` return; TS drops overflow units to the world floor (duration 200). Units vanish when dest inv full. Sibling INV_MOVEITEM_CERT (Go:1288) handles overflow → inconsistent omission. | Go discards tx, no AddObj; TS:521-530. |
| `handlers_inv.go:828-834` (INV_MOVEFROMSLOT) | `InvOps.ts:323-349`,`Player.ts:1651-1656` | DEVIATION | HIGH | **Item-loss:** Delete(fromSlot)+Add(...) discards Add tx with no world drop; TS drops overflow. Moving a slot into a full inv destroys spillover. | Go discards tx; TS:339-348. |
| LOC_*/OBJ_* read handlers (`handlers_obj.go`:160-472; `handlers_loc.go` LOC_ANGLE/ANIM/etc.) | `ObjOps.ts`/`LocOps.ts`; getters `ScriptState.ts:269-295` | STALE-DEFER | LOW | Read handlers not operand-aware for `.obj2`/`.loc2`. Documented NAI-154-D/NAI-119-D ("no downstream consumers"); confirmed still accurate at HEAD. Lower impact than NPC (no known consumers). | state.go:359-381; writers ARE operand-aware. |
| `handlers_obj.go:116-124` (OBJ_ADD/ADDALL), `handlers_inv.go:1048,1444` (INV_DROPSLOT/DROPITEM) | `ObjOps.ts:45-91`,`InvOps.ts:184,250` | DEVIATION | LOW | ActiveObj **write-side** not operand-aware in spawn handlers (writes slot-0 directly vs TS operand-aware setter). operand=1 would write wrong slot; practically unreachable. Not covered by NAI-154-D read-note. | Go direct field write vs setActiveObjSlot. |
| `handlers_inv.go:131-144` (INV_SIZE) | `InvOps.ts:27-31` | DEVIATION | LOW | Requires resolvable player inv (returns Capacity); TS is a pure config read (`invType.size`, no player). Value matches when allocated; diverges for player lacking inv. | Go resolveInv vs TS config read. |
| `handlers_obj.go:178-180` (OBJ_ADDALL) | `ObjOps.ts:58-93` | DEVIATION | LOW | Lacks nil-World guard that twin OBJ_ADD has; nil World → panic on members obj. Defensive-only asymmetry. | objAddCommon:105 derefs s.World. |
| `handlers_obj.go:343-351` (OBJ_FIND) | `ObjOps.ts:168-173` | DEVIATION | LOW | Validates coord before objType; TS validates objType first. Observable only as which error fires when both invalid. | order reversed. |

### Cluster L — Core/Number/String/Db + misc script handlers — ~70 opcodes

**INVPOW prior logarithm bug confirmed FIXED** (nth-root correct, comment accurate). The sign/rounding group below changes script arithmetic for any negative-operand path.

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `handlers_number.go:92-100` (DIVIDE) | `NumberOps.ts:26-30` | DEVIATION | MEDIUM | Rounds toward −∞ (`floorDiv`); TS truncates toward zero. `-5/2`→Go -3, TS -2. | TS pushInt=toInt32 truncate. |
| `handlers_number.go:102-110` (MODULO) | `NumberOps.ts:69-72` | DEVIATION | MEDIUM | Euclidean-positive `posMod`; TS truncated `%` keeps dividend sign. `posMod(-5,3)`→Go 1, TS -2. (Brief specifically flagged modulo sign.) | TS `n1 % n2`. |
| `handlers_number.go:128-139` (SCALE) | `NumberOps.ts:124-127` | DEVIATION | MEDIUM | `floorDiv(a*c,b)`; TS truncates `(a*c)/b`. Diverges when a*c negative. | same toInt32 truncation. |
| `handlers_number.go:336-349` (SIN_DEG/COS_DEG) | `NumberOps.ts:165-171`→`Trig.ts:5-27` | DEVIATION | MEDIUM | `math.Round(sin·16384)`; TS precomputed table built with `|0` (truncate). Off-by-1 (e.g. 100.7→TS 100, Go 101). ATAN2 matches (both round). | Trig._sin uses `|0`. |
| `handlers_string.go:73-78` (COMPARE) | `StringOps.ts:37-40,125-140` | DEVIATION | MEDIUM | Returns only −1/0/1; TS `javaStringCompare` returns full char-code diff / len diff. compare("apple","apply")→Go -1, TS -20. Test pins only sign-matching cases (incomplete contract). | strings.Compare vs javaStringCompare. |
| `handlers_string.go:85-99` (SUBSTRING) | `StringOps.ts:58-62` | DEVIATION | MEDIUM | Doesn't match JS substring: negative end not clamped (slice-panic→tick-recover script abort); no start>end swap (Go collapses to empty, JS swaps). | JS substring(3,1)→"el", Go→"". |
| `handlers_number.go:368-380` (INTERPOLATE) | `NumberOps.ts:42-47` | DEVIATION | LOW | Guards x1==x0→pushes y0; TS unguarded → Inf/NaN→toInt32→0. Comment honest but fallback (y0) ≠ TS de-facto 0. | handlers_number.go:366 comment. |
| `handlers_number.go:84-90,155-169` (MULTIPLY, POW) | `NumberOps.ts:20-24,74-77` | DEVIATION | LOW | int32 wraparound vs TS float64 then toInt32 — differ only on products/results > 2^53. Go arguably more correct. | edge-only. |
| `handlers_string.go:80-83,46-51` (STRING_LENGTH, APPEND_CHAR) | `StringOps.ts:54,48-52` | DEVIATION | LOW | UTF-8 byte length / UTF-8 emit vs TS UTF-16 code-unit. "é"→Go len 2, JS 1. Rare (RS strings mostly ASCII). | len(string) vs .length. |
| `handlers_array.go:11-47` (DEFINE_ARRAY/PUSH/POP_ARRAY_INT) | `CoreOps.ts:232-242` | CONFIRMED-EXCEPTION | LOW | Fully implemented in Go; TS throws 'unimplemented'. Go-only extension (no TS behavior to diverge from). | TS all three throw. |
| `handlers_number.go:304-322` (RANDOM/RANDOMINC) | `NumberOps.ts:32-40`→JavaRandom | DEVIATION | LOW | math/rand/v2 vs JavaRandom port — range matches, sequence not reproducible. Acceptable (RS randomness not wire-reproduced). | range OK. |

### Cluster M — Config type decoders (`src/cache/config/*Type.ts` ↔ `pkg/objtype/*.go`) — 24 decoders, ~230 opcode cases

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `npctype.go:294` (cases 213→249, no 214) | `NpcType.ts:223-224` | MISSING + COMMENT-LIE | **CRITICAL (VERIFIED, latent)** | No opcode 214 (regenrate) case. Packer emits `P1(214)+P2(value)` for `regenrate=` (pkg/pack/npc.go:525); decoder `default→fmt.Errorf("unrecognized npc config code 214")` aborts entire NPC config load. `RegenRate` frozen at default 100. Comment at npctype.go:331 *lists* "214 (regenrate)" as handled = lie. Latent: 0 Content `.npc` set regenrate (verified). | Verified by grep + read; TS:223-224 `regenrate=dat.g2()`. |
| `paramtype.go:152-159` (DefaultInt=0) | `ParamType.ts:62` (defaultInt=-1) | DEVIATION | MEDIUM | ParamType DefaultInt default 0 (Go zero-value) vs TS -1. Packer omits opcode 2 when no `default=`; param read with no key falls to DefaultInt → Go pushes 0, TS -1. The exact -1-vs-0 default bug class. EnumType correctly matches (0). | handlers_config.go:48 fallback; TS:62. |
| `hunttype.go:83` (FindNewMode=NPCModeNull -1) | `HuntType.ts:83` (findNewMode=NONE 0) | DEVIATION | MEDIUM | HuntType FindNewMode default -1 vs TS NONE=0. Packer omits opcode 6 when NONE; hunt without explicit findnewmode → SetInteraction(...,-1) sets different NpcMode post-hunt than TS. | npc_hunt.go:387; TS Npc.ts:906. |
| `npctype.go:207-215` (case 30..39, Op=make([]string,5)) | `NpcType.ts:141-146` (Array(5)) | DEVIATION | LOW | Accepts opcodes 30-39 but Op is 5-slot; 35-39 → index 5-9 → **Go panics**; TS Array grows silently. Unreachable from goscape packer (op1-5 only); latent crash on foreign caches. | npctype.go:207. |
| `objtype.go:292`,`246`; `npctype.go:213`; `loctype.go:113` (op 200 extra; "hidden"→"") | `ObjType.ts:181-279`; OpObj/Npc/LocHandler.ts | DEVIATION | LOW | (a) ObjType handles extra opcode 200 (Tradeable=true) TS rejects (harmless forward-compat). (b) Coerces op-slot "hidden"→"" at decode; TS keeps literal + checks at handler. Documented for loc (NAI-80-D1); undocumented for obj/npc. Consumer reading raw op string would diverge. | loctype.go:17 doc; obj/npc undocumented. |
| `varptype.go:39`,`varntype.go:21`,`varstype.go:21` (default→error) | `VarPlayerType.ts:101`,`VarNpcType.ts:71`,`VarSharedType.ts:73` (printError+continue) | DEVIATION | LOW | varp/varn/vars: TS non-fatal printError+continue on unknown opcode; Go returns error aborting config load. Opposite of obj/npc/loc (where TS itself throws + Go matches). Malformed-cache only; Go arguably safer. | varptype.go:39. |
| `componenttype.go:331` (overlay=G1()\!=0) | `Component.ts:243` (gbool) | DEVIATION | LOW | overlay `G1()\!=0` vs gbool (`==1`); byte≥2 → Go true, TS false. Packer writes pbool 0/1, unobservable. | packet.go:221. |
| (no Go CategoryType decoder) | `CategoryType.ts` | CONFIRMED-EXCEPTION | LOW | No runtime CategoryType decoder; category name→id resolved at pack time, runtime keeps raw int. No runtime consumer needs name registry. Deliberate architecture. | pack_configs.go:147. |

---

### Cluster N — Wire protocol: Packet I/O + ISAAC + RSA login + opcode tables (`pkg/io/*` ↔ `src/io`/`src/network`)

**ISAAC, RSA, and client+server opcode tables are BYTE-FAITHFUL** (verified against TS + Java client): ISAAC seed-init/mix/extraction/GetNext + the encryptor-seed offset; RSA modulus/exponent + 1-byte length prefix + BigInteger sign-byte strip/pad + magic 10; all 78 client + 71 server opcode IDs/sizes/categories incl. -1/-2 dynamic. GSmart sign-fix comment verified accurate (not a lie). ~40 accessors + handshake compared.

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `login/req/req.go:88` | `World.ts:2126-2127` | DEVIATION | LOW | `lowMemory` decoded `GBool()` (`==1`) vs TS `(info & 0x1)`. Java client only ever sends 0/1, so no live-wire break; diverges only for non-conforming clients. | Java writes p1(lowMemory?1:0). |
| `packet/packetbit.go:32` | `Packet.ts:384` | DEVIATION | LOW | `GBit(n)` returns `uint8`, truncating reads >8 bits; TS/Java return int. Latent — zero non-test callers (server writes bits via PBit/rsbuf, never reads multi-bit). | grep no non-test callers. |
| `modules/world/server.go:886-907` | `World.ts:2119-2138` | DEVIATION | LOW | Login validation order: Go RSA-decrypts before revision/CRC checks; TS gates rev+CRC before rsadec. Success path byte-identical; only error-path CPU/ordering differs. | Go UnmarshalBinary(RSA) then rev/CRC. |

### Cluster O — Cache/compression/sprite formats (`pkg/io/jagfile`,`bzip2`,`pkg/cache`,`pixpack` ↔ `src/cache`/`src/io`)

**bzip2 RS-framing quirk + CRC32 table are BYTE-FAITHFUL** (writer emits BZh0 4-byte header, compress strips first 4, decompress prepends/overwrites "BZh1"; CRC IEEE poly 0xedb88320). ~15 functions compared. All 5 deviations LOW, none affect serialized bytes for real caches.

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `packet/packet.go:15-17` (GetCRC) | `Packet.ts:54-60` | DEVIATION | LOW | Slices `src[offset:offset+length]`; TS loops `i<length` (treats length as END index). Identical only at offset=0 — all production callers pass offset=0 (verified). | latent. |
| `jagfile/jagfile.go:318-319` (Deconstruct) | (none) | DEVIATION (dead code) | LOW | Go-only dead code, independently buggy (passes running-total offset as start index). Zero callers. | grep no callers. |
| `jagfile/jagfile.go:372-377` | `Jagfile.ts:68-71` | DEVIATION | LOW | Name-label lookup iterates Go map (non-deterministic) + breaks on first match; TS findIndex deterministic. FileName is a debug label, never serialized; no collisions in 212-name set. | order-independent in practice. |
| `jagfile/jagfile.go:401-640` | `Jagfile.ts:266-506` | DEVIATION | LOW | knownNames ordering differs from TS; zero effect (Go keys by name-map; name SETS byte-identical, 212 each). | set-diff empty. |
| `pixpack/convert.go:46-52` | `PixPack.ts:186-190` | CONFIRMED-EXCEPTION | LOW | Palette >255: TS quantizes, Go errors (no stdlib quantizer). Documented NAI-213-D; TS itself flags this path CRC-divergent. | convert.go:48 marker. |

### Cluster P — Pathfinding (`pkg/pathfinder/*` ↔ rsmod-pathfinder v5.0.4 / Java)

NOTE: TS delegates pathfinding to the `@2004scape/rsmod-pathfinder` v5.0.4 WASM crate; audited against the crate's canonical TS source + enum dump (the proper reference, like rsbuf). **Collision-flag constants + direction encoding are FAITHFUL** (full 1<<iota chain, composite masks, Direction N/E/S/W=1/2/4/8, all loc Angle/Shape/Layer enums). ~40 functions compared. `isBlockedSouthEast` asymmetry confirmed PRESENT IN TS TOO (correct parity, not a Go omission).

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `routefinder/routefinder.go:325` | `rsmod PathFinder.ts:319-320` | DEVIATION (typo) | HIGH | `routeFindSize2` west→east diagonal: 2nd collisionFlag call passes `baseZ, baseZ` instead of `baseX, baseZ` → reads wrong tile column when baseX≠baseZ (almost always). Corrupts BFS neighbor test for size-2 (medium) entities. On LIVE path (MOVE_CLICK/SMART). Line 324 directly above correctly uses baseX,baseZ. | Go `collisionFlag(baseZ,baseZ,...)` vs TS `collisionFlag(baseX,baseZ,...)`. |
| `routefinder/line.go:24` (lineScaleDown) | `rsmod Line.ts:24` | DEVIATION | LOW | Arithmetic `>>16` vs logical `>>>16`. Identical for non-negative on-map coords (the only reachable values); latent if a scaled intermediate goes negative. | edge-only. |
| `collision/flag.go:4`,`flagmap.go:35` (FlagNull=-1) | `CollisionFlagMap.ts` (NULL=0x7FFFFFFF) | DEVIATION | LOW | Unallocated-tile sentinel -1 (all bits) vs TS 0x7FFFFFFF (ROOF bit clear). Diverges only for Indoors strategy on off-map/unallocated tiles (Go=roofed, TS=un-roofed). Internal-only sentinel, never wire-compared; BFS clips inside allocated regions. | `-1 & ROOF` ≠ `0x7FFFFFFF & ROOF`. |

### Cluster Q — wordenc + gamemap + entity (`src/wordenc`/`GameMap.ts`/entity base)

**wordenc: 0 deviations** — faithful, security-correct chat-censor (pipeline order, full leet-substitution table, binary searches, symbol-masking, whitelist restoration, format/uppercase). entity lifecycle relative→absolute re-model verified behaviorally equivalent (no off-by-one). ~44 functions compared.

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `gamemap/multimap.go:37-67` (loadCsvMap) | `GameMap.ts:269-282` | WRONG-PATH | **CRITICAL** | Parses comma `level,x,z`; real TS CSV is underscore-packed `level_mx_mz_lx_lz`, two coords/line (rect range). Every real line → len==2 → skipped by `len(row)<3` guard. **multimap+freemap load EMPTY** → multi-combat detection + ALL F2P gating dead server-wide. Correct expander exists `pkg/pack/worldmap/csv.go:processCsv` but never wired to gamemap.Init. | empirically every real line SKIPs. |
| `gamemap/multimap.go:12-14` (packZoneCoord) | `ZoneMap.ts:6-8` (zoneIndex) | DEVIATION | HIGH | Keys per-TILE (`z&0x3fff | x<<14 | level<<28`); TS keys by 8×8-ZONE (`(x>>3)&0x7ff | (z>>3)<<11 | level<<22`, with `>>3`). Even with correct CSV, Go matches only exact tiles not whole zones. | no `>>3` in Go. |
| `gamemap/load.go:79-127` (loadGround p2) | `GameMap.ts:189-191` | MISSING | HIGH | No `\!members && \!isFreeToPlay && \!bordersFreeToPlay` gate; writes collision for member-only tiles on F2P server. `bordersFreeToPlay` absent in goscape. | no gate. |
| `gamemap/load.go:159-235` (loadLocs) | `GameMap.ts:238-240` | MISSING | HIGH | Same missing F2P/members gate for static-loc parsing. | no gate. |
| `gamemap/load.go:242-259` (loadNPCs) | `GameMap.ts:122-124` | MISSING | HIGH | No F2P/members gate for NPC spawns (loadObjs DOES have it — inconsistent). | no gate. |
| `gamemap/gamemap_test.go:45-69` | (n/a) | TEST-PINS-BUG | MEDIUM | `TestInitLoadsCsvMaps` feeds fabricated `"0,1000,2000"` comma format the toolchain never emits + asserts exact tile (not zone) → hides both the CSV bug AND the tile/zone deviation. | passing test ≠ correct contract. |
| `entity.go:33-44` / `player_zone.go:50` (obj CheckLifecycle) | `Zone.ts:142-146` | DEVIATION | MEDIUM | obj full-follows gates on Go-invented `CheckLifecycle(tick)` vs TS maintained `obj.isActive` (loc branch correctly uses IsActive). Can diverge at tick boundaries. (= cluster E row, entity-side.) | inconsistent with loc sibling + TS. |

### Cluster R — Friend / Login / Asset servers (`friend.ts`/`login.ts`/`web.ts`)

Transport (gRPC/SQLite vs WebSocket/Kysely) = CONFIRMED-EXCEPTION. Client-facing login wire response codes verified PARITY. ~37 handlers compared.

| Go loc | TS loc | Class | Severity | Finding | Evidence |
|---|---|---|---|---|---|
| `friends/repository.go:319-432` (IsVisibleTo/Many) | `FriendServerRepository.ts:332-355` | MISSING | HIGH | Staff online-status bypass absent: TS returns true for staff viewers; Go stores `staffLvl` but never consults it → staff see FRIENDS/OFF players as offline. | staffLvl written (repository.go:101), read by nothing in visibility. |
| `friends/repository.go:319-432` | `FriendServerRepository.ts:340-342` | MISSING | HIGH | Ignore-list online-suppression absent: TS returns false if `other` ignores `viewer`; Go switches on privateChat mode only, never queries ignorelist. Ignored viewer still sees you online. | no ignorelist SELECT in visibility. |
| `friends/repository.go:150-198` (AddFriend/AddIgnore) | `FriendServerRepository.ts:230-270` | MISSING | MEDIUM | 100-entry friend/ignore cap not enforced; Go inserts with no COUNT ceiling. | TS `if count>=100 return`. |
| `friends/repository.go:169-278` (GetFriends/Ignores) | `FriendServerRepository.ts:357-386` | DEVIATION | LOW | No `ORDER BY created ASC`; list display order undefined/non-stable vs TS. | no ORDER BY. |
| `friends/handler.go:38-45` (WorldConnect) | `FriendServer.ts:413-415` | DEVIATION | LOW | Re-WORLD_CONNECT doesn't terminate prior socket (per-player-stream arch NAI-S4A); stale streams not force-dropped. | counter reset only. |
| `login/handler.go:185-333` (save read/writeSave) | `LoginServer.ts:287,364,410,455` | MISSING | HIGH | `PlayerLoading.verify` absent on read AND write: corrupt save served to client; garbage blob persisted unconditionally. TS rejects (resp 7) / skips write on invalid. | no verify call. |
| `login/handler.go:226-256` (Logout/Autosave) | `LoginServer.ts:410,455` (wouldResetSaveFile) | MISSING | HIGH | Anti-rollback absent: TS refuses a save whose playtime went backward; Go always overwrites. Reordered autosave/logout rolls progress back. | writeSave unconditional. |
| `login/handler.go:186-196` (NEW_PLAYER) | `LoginServer.ts:342-348` | MISSING | MEDIUM | "save missing but logout_time set" safety reject absent: a vanished save silently resets a real character to fresh instead of rejecting (resp 7). | no logout_time consult. |
| `login/db.go:221-230` (setLoggedOut) | `LoginServer.ts:429-440` | DEVIATION | MEDIUM | Logout records only `logged_in=0`; misses logout_time/logged_out (also disables the safety branch above). | TS sets 4 fields. |
| `login/handler.go:148-163` (reconnect) | `LoginServer.ts:271-317` | DEVIATION | MEDIUM | Reconnect requires `req.HasSave` + returns nil save; TS re-serves the save when client lost it (`\!hasSave` → read+send). | reconnect short-circuits nil save. |
| `login/handler.go:241` | `LoginServer.ts:450` (updateHiscores) | STALE-DEFER | LOW | Hiscore update on logout is a bare TODO; never updates. | TODO. |
| `asset/handler.go:84-97` (.mid) | `web.ts:63-69` | DEVIATION | MEDIUM | `.mid` path with no `_` → `LastIndex` returns -1 → slice `[1:-1]` panics (recovered per-request, aborts response no body); TS `substring(1,-1)`→'' → clean 404. Crafted `GET /x.mid` aborts. | handler.go:92. |
| `asset/handler.go:29-97` (dispatch order) | `web.ts:63-87` | DEVIATION | LOW | Archive prefixes checked before `.mid`; TS checks `.mid` first. `/config_123.mid` serves config archive in Go vs song in TS. | order differs. |
| `asset/server.go:779-783` (WS seed) | `web.ts:134-137` | DEVIATION | LOW | First seed word not masked to 24 bits (TS `rand & 0x00ffffff`); session entropy, functionally inert. | two full Uint32. |

### Cluster S — pkg/pack spot-check (emit-vs-decode cross-check; Arc-26 byte-parity NOT re-litigated)

Cross-checked all 15 config packers' emitted opcodes against the `pkg/objtype` decoder cases. **The npc opcode-214 (regenrate) gap is the ONLY emit-without-decode gap** (corroborates cluster M CRITICAL). Default-value parity spot-check: 0 deviations (obj/seq/npc defaults all match TS). Compiler stubs (PushConstantLong, long-arith/branch, queue-arg TODO) all verified TS-faithful CONFIRMED-EXCEPTIONS (TS throws/TODOs identically). No SHIPPED Arc-26 item flagged.

| Config | Opcode | Packer emits | Decoder | Class | Severity |
|---|---|---|---|---|---|
| npc | 214 (regenrate) | `pkg/pack/npc.go:526` | **MISSING** (`npctype.go` 213→249) | MISSING | CRITICAL (= cluster M; decoder-only fix: `case 214: t.RegenRate = int(dat.G2())`) |

---

## Audit complete — closing notes

All in-scope packages audited across 4 waves (~990 functions/opcodes, 19 finding clusters A–S). Out of scope per handoff and confirmed N/A: `internal/dskit/*` (dskit port), goscape extension packages (no TS counterpart), `pkg/rsbuf` (reimplements the Rust crate, PARITY since Arc-15 — spot-checked, not deep-walked vs TS stubs).

**3 CRITICAL** (2 verified by direct grep/read, 1 needs live repro), **17 firm HIGH + 1 disputed**, **~30 MEDIUM**, **~50 LOW**, **~11 CONFIRMED-EXCEPTION**. Recurring theme confirmed at scale: the operand-awareness bug class (player fixed, NPC unfixed), default-value drift (ParamType/HuntType/regenrate), and several confidently-wrong "by design"/"matches TS" comments (faceSquare, NPC defaultMode, FINDHERO read, regenrate-214, gamemap CSV test). Byte-level wire/cache/pathfinding constants are clean. **No fixes applied — this is a report-only ledger.**

Suggested fix-priority order (cheap+verified first): regenrate-214 decoder case (1 line) → gamemap CSV wiring (expander exists) → routeFindSize2 typo (1 token) → NPC operand accessor (mirror `c58cac51`) → INV overflow item-loss → login save-verify/anti-rollback → arithmetic sign/rounding group. Resolve the faceSquare dispute by tracing the live render path before touching masks.
