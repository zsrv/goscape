# TS-Parity Fix Tracker

Implementation checklist for the findings in `2026-05-23-ts-parity-audit.md`.
Work top→bottom (most→least critical). Mark `- [x]` + append `(SHA)` when landed.
Rules & batching: see `docs/superpowers/handoffs/2026-05-23-ts-parity-fix-all-resume.md`.

Legend: `⚠TEST` = a green test pins the buggy contract, update it as part of the fix ·
`⚠LIE` = fix/delete the false comment too · `⚠TRACE` = investigate before fixing ·
`(dep: X)` = do together with / after X · cluster ref = ledger section.

## Progress

| Severity | Done / Total |
|---|---|
| CRITICAL | 3 / 3 ✓ |
| HIGH | 17 / 17 ✓ (+1 disputed, tracked separately) |
| MEDIUM | 21 / 30 |
| LOW | 0 / 50 |
| **Total** | **0 / 100** (+1 disputed, +~11 do-not-fix) |

---

## CRITICAL

- [x] **C1** NpcType decoder missing opcode 214 (regenrate) → config-load crash; `RegenRate` frozen at 100 — `pkg/objtype/npctype.go:294` — add `case 214: t.RegenRate = int(dat.G2())`; ⚠LIE comment at `:331`. [cluster M/S] **(82da4025)** — note: no "lie comment" exists at :331 (it's the Stats initializer); added pin test `TestNpcTypeDecodeRegenRate214`.
- [x] **C2** `gamemap.loadCsvMap` parses comma `level,x,z` not underscore `level_mx_mz_lx_lz` → multimap/freemap empty, multi-combat + F2P-gating dead — `pkg/gamemap/multimap.go:37` — wire existing `pkg/pack/worldmap/csv.go:processCsv`; ⚠TEST `TestInitLoadsCsvMaps`; **(dep: H10 — fix together)**. [cluster Q] **(5ca96311)** — did NOT use processCsv (that's the worldmap range-expander keyed per-tile, incompatible w/ zone-index lookup); ported TS GameMap.loadCsvMap instead. Also added packer CSV-copy into server/maps so Init can find them at runtime.
- [x] **C3** CLIENT_CHEAT reads phantom leading `G1()` → eats 1st char of every `::` command — `modules/world/handlers_game.go:594` — ⚠TRACE live repro first; then remove `G1()` + ⚠TEST `handler_cheats_supermod_test.go:45` fabricated `P1(0)`; ⚠LIE comment. [cluster G] **(0f895a04)** — confirmed via wire-trace (TS decoder gjstr-only + Java client p1=length-prefix + size=-1 framing strips it), stronger than live repro. Removed G1(), fixed both masking helpers (dispatchCheat + dispatchTeleCheat) + lying comment.

## HIGH

- [x] **H1** NPC operand gap: ~37 read/mutate ops read `s.ActiveNpc` not operand-resolved — `handlers_npc.go:184` + sites — mirror `c58cac51`: add `s.activeNpcResolved()` + operand-aware `requireActiveNpc` + ~35 swaps (opcode list in cluster J). [cluster J] **(8e27bd8f)** — added `s.activeNpc()`, operand-aware `requireActiveNpc`, swapped all listed sites + NPC_PARAM; updated 2 bug-pinning tests + added 2 pin tests. SETMODE target & write-side left raw per TS.
- [x] **H2** Per-zone obj cap (129) eviction missing — `pkg/zone/zone.go:263` (TODO-marked) — evict first DESPAWN obj when `totalObjs>=129`. [cluster E] **(f65b8491)** — added zone.MaxObjs + TotalObjs/FirstDespawnObj; eviction in Server.AddObj via s.RemoveObj.
- [x] **H3** Shop restock/decay never ported — `modules/world/tick.go:866` processCleanup — port TS World.ts:1159-1190; **(dep: M10, M11 first)**. [cluster A/F] **(425e843c)** — restock/decay/allstock pass in processCleanup w/ modulo-by-zero guard; +pin tests.
- [x] **H4** Queue/engine-queue delay off-by-one (fires 1 tick early) — `tick.go:508` — pre-decrement gating like TS Player.ts:883; ⚠TEST `player_engine_queue_test.go:86`. [cluster C] **(5f64edd1)** — post-decrement (read old, gate on old) in both drains; fixed 3 pinning tests (engine-queue + TestQueueFiresAtDelayExpiry).
- [x] **H5** Logout pipeline incomplete: no `closeModal()` / `canAccess()`+queue-drain gate / LOGOUT trigger — `tick.go:403` (+`server.go:1242`); `TriggerLogout` dispatched nowhere. [cluster C] **(f9f5b52c)** — closeModal + queue-discardable + canAccess/engineQueue gate + LOGOUT trigger dispatch; DEVIATION: degrades to removal (not TS never-remove) when no [logout] script. +2 pin tests.
- [x] **H6** NPC `defaultMode` re-derived, ignores `typ.DefaultMode` — `npc_interaction.go:1073` — read stored field like TS; ⚠TEST `npc_interaction_test.go:1186`; ⚠LIE comment `:1076`. [cluster D] **(e5e4aeec)** — reads n.typ.DefaultMode; rewrote pin test + fixed 4 wander fixtures (newWanderNpc + WanderRange:5 literals).
- [x] **H7** INV_MOVEITEM item-loss: discards `toInv.Add` overflow TS drops to floor — `handlers_inv.go:761`. [cluster K] **(bfcb3a72)** — overflow→floor via new dropOverflowToFloor helper; +pin test.
- [x] **H8** INV_MOVEFROMSLOT item-loss: same — `handlers_inv.go:828`. [cluster K] **(bfcb3a72)** — same helper; +pin test.
- [x] **H9** `routeFindSize2` typo `baseZ,baseZ`→`baseX,baseZ` (size-2 entity BFS, live path) — `pkg/pathfinder/routefinder/routefinder.go:325`. [cluster P] **(b260913d)** — confirmed vs rsmod PathFinder.ts:320.
- [x] **H10** gamemap multimap keys per-tile not 8×8-zone (missing `>>3`) — `pkg/gamemap/multimap.go:12` — **(dep: C2 — fix together)**. [cluster Q] **(5ca96311)** — `packZoneCoord`→`zoneIndex` w/ TS ZoneMap.zoneIndex packing; updated TestSetMulti (pinned per-tile).
- [x] **H11** `loadGround` missing `!members && !isFreeToPlay && !bordersFreeToPlay` gate — `pkg/gamemap/load.go:79` — port `bordersFreeToPlay` from TS. [cluster Q] **(052d783e)** — gate added + bordersFreeToPlay ported.
- [x] **H12** `loadLocs` missing same F2P/members gate — `pkg/gamemap/load.go:159` — batch with H11/H13. [cluster Q] **(052d783e)**
- [x] **H13** `loadNPCs` missing F2P/members gate — `pkg/gamemap/load.go:242` — batch with H11/H12. [cluster Q] **(052d783e)** — no borders (TS:122-124); typeID read before gate to keep stream aligned. +pin tests.
- [x] **H14** Friend staff online-status bypass absent (`staffLvl` stored, never consulted) — `modules/friends/repository.go:319` (IsVisibleTo/Many). [cluster R] **(53ad6ba5)** — staffLvl>1 bypass (TS playerStaff) in scalar + batched paths.
- [x] **H15** Friend ignore-list online-suppression absent — `modules/friends/repository.go:319` — batch with H14. [cluster R] **(53ad6ba5)** — isIgnoredBy/targetsAmong; staff-then-ignore-then-mode order; +4 pin tests.
- [x] **H16** Login `PlayerLoading.verify` absent on read+write (corrupt save served/persisted) — `modules/login/handler.go:185,294`. [cluster R] **(d90b3422)** — verifySave (magic/version/CRC); read rejects (DataLoss), write skips.
- [x] **H17** Login `wouldResetSaveFile` anti-rollback absent (stale save clobbers newer progress) — `modules/login/handler.go:226`. [cluster R] **(d90b3422)** — playtime-based rollback check; new modules/login/save.go + unit tests.

## DISPUTED (resolve separately — do NOT fix until traced)

- [ ] **D1** `faceSquareX/Z` never reset per tick — `player_masks.go:63` / `npc_masks.go:195` — ⚠TRACE: HIGH stale-leak vs intentional Arc-30 #202 force-emit; may share root with M2. Trace live render path first, then fix-or-document; update ledger + tracker with the verdict. [clusters C/D + adjudication note]

## MEDIUM

- [x] **M1** Player `inApproachDistance` missing line-of-sight gate (can fire through walls; NPC side has it) — `modules/world/interaction.go:750`. [B] **(5c4be889)** — kept free inApproachDistance as the range half; added `(*Player).approachHasLineOfSight` (forward LoS, FlagBlockPlayers) AND'd at the tryInteract call site; closes DEVIATION S6l-D4. +pin test.
- [ ] **M2** Per-step `focus()` not called → stale walk-facing — `movement.go:182`, `npc_interaction.go:439` — **(consider with D1)**. [B]
- [x] **M3** `validateDistanceWalked` not ported (no jump-snap on >2-tile move) — `tick.go:140`. [A/B] **(122f1148)** — ported player side: `(*Player).validateDistanceWalked` + `processValidateDistanceWalked` pass after processEnergy (EXACT_MOVE-gated, TS World.ts:733). NPC side intentionally omitted (PORTING-EXCEPTION at npc_ai.go) — TS Npc.ts:184 sets jump but computeNpc never passes it; NpcInfo has no jump bit, so it's a wire no-op + Npc has no jump field. +pin tests.
- [x] **M4** Primary queue gates on `p.delayed` not full `canAccess()` — `tick.go:514`. [C] **(7291272f)** — gate now `!p.CanAccess()` (delayed||modal||protected). With M5.
- [x] **M5** STRONG queue fires while `p.delayed` (TS has no such exception) — `tick.go:513`. [C] **(7291272f)** — dropped QueueStrong gate exception; STRONG closes modal (pre-pass) but still waits for CanAccess. ⚠TEST TestStrongQueueFiresWhileDelayed rewritten to TS-correct contract.
- [x] **M6** NORMAL timer gate uses `p.delayed` not `canAccess()` (+ missing shutdown force-fire) — `tick.go:625`. [C] **(7291272f)** — NORMAL gate now `!p.CanAccess()`; shutdown force-fire is the same DEVIATION-NAI-144-D4 (no shutdown flag). +pin test.
- [x] **M7** `addXp` NODE_XPRATE multiplier not applied → `node_xp_rate` config dead — `player_script.go:852` — ⚠LIE comment `handlers_player.go:570`. [C] **(116f9922)** — added allowMulti param + xpRate() helper; STAT_ADVANCE=true, setlevel cheat=false; fixed lie comment. +pin test (rate=3).
- [x] **M8** Modal-open clobbers full bitmap (wipes TUT bit) + skips suspended-script clear — `player_script.go:1086`. [C] **(1e3767c3)** — OpenMain/Chat/Side/MainSide use bit clear/OR (TUT survives) + clearSuspendedDialogScript (CountDialog/PauseButton→nil). +pin test.
- [x] **M9** Obj-reveal tradeable/members gating missing (private drop becomes public) — `pkg/zone/zone.go:343` (TODO-marked). [E] **(8e4756ca)** — gate added at (*Server).RevealObj (objTypeFor + cfg.NodeMembers); pkg/zone TODO retired. +pin test (non-tradeable/members-f2p stay private).
- [x] **M10** INV `assureFullRemoval` short-circuit absent — `pkg/inventory/inventory.go:291` — **(needed by H3)**. [F] **(425e843c)** — honor existing AssureFullRemoval opt (all-or-nothing).
- [x] **M11** INV stockObj count-0 slot retention missing — `pkg/inventory/inventory.go:306` — **(needed by H3)**. [F] **(425e843c)** — RemoveOpts.StockObj retains slot at count 0.
- [x] **M12** FINDHERO reads `s.Self` not operand-aware (player) — `handlers_player.go:1644` — ⚠TEST `TestFindHero_PopulatedAlwaysSetsSelf2`; ⚠LIE comment. [I] **(c45ed279)** — ledger read → `s.activePlayer()`; write stays raw `s.Self2` (TS); fixed lie comment; test rewritten to operand-selects-ledger.
- [x] **M13** P_PREVENTLOGOUT missing StringNotNull/NumberNotNull validation — `handlers_player.go:1759`. [I] **(c45ed279)** — added checkStringNotNull(msg)+checkNotNull(ticks) in TS order. +pin test.
- [x] **M14** `checkQueue` range [1,20] vs TS [0,19] — `handlers_npc.go:131` — verify Content for queueid 0/20 first. [J] **(492d7f2f)** — VERIFIED-DEVIATION (no code change): Content uses queueid {1-7,10-12}+walktrigger{8}, never 0/20; TS [0,19]+`+queueId-1` leaves AiQueue20 unreachable & admits garbage AiQueue0 → goscape [1,20] is correct. Added PORTING-EXCEPTION marker; bounds already pinned by RejectsZero/AcceptsTwenty tests.
- [x] **M15** DIVIDE floor-toward-−∞ vs TS truncate-toward-zero — `handlers_number.go:92` — batch M15–M18. [L] **(bd4638cf)** — Go native `/` (trunc); ⚠TEST -7/2 -4→-3.
- [x] **M16** MODULO Euclidean-positive vs TS truncated remainder — `handlers_number.go:102`. [L] **(bd4638cf)** — Go native `%`; removed posMod; ⚠TEST -7%3 2→-1.
- [x] **M17** SCALE floor vs truncate — `handlers_number.go:128`. [L] **(bd4638cf)** — Go native `(a*c)/b` trunc. (floorDiv kept for INTERPOLATE's Math.floor.)
- [x] **M18** SIN_DEG/COS_DEG round vs TS table-truncate — `handlers_number.go:336`. [L] **(bd4638cf)** — int() trunc + TS verbatim size literal 3.834951969714103e-4 (bit-identical to TS table). atan2 already correct.
- [x] **M19** COMPARE returns sign only vs TS char-code magnitude — `handlers_string.go:73` — ⚠TEST `handlers_string_test.go:213` (incomplete contract). [L] **(27de25c5)** — added javaStringCompare (magnitude/len-diff, byte-wise); strengthened test with magnitude cases.
- [x] **M20** SUBSTRING negative-end panic / no start>end swap — `handlers_string.go:85`. [L] **(27de25c5)** — JS substring semantics: clamp each idx to [0,len], swap if start>end. +edge-case tests.
- [x] **M21** ParamType.DefaultInt default 0 vs TS -1 — `pkg/objtype/paramtype.go:152`. [M] **(9fa8879a)** — NewParamType inits DefaultInt: -1 (TS ParamType.ts:62).
- [x] **M22** HuntType.FindNewMode default -1 vs TS NONE(0) — `pkg/objtype/hunttype.go:83`. [M] **(9fa8879a)** — NewHuntType FindNewMode → NPCModeNone(0); matches existing npc_hunt fixtures; updated hunttype_test assertion.
- [ ] **M23** IDLE_TIMER (opcode 70) routed to no-op vs TS `requestIdleLogout` — `handlers_game.go:37`. [G]
- [ ] **M24** MoveClick passes start tile not dest in non-default routefinder config — `handlers_game.go:300`. [G]
- [ ] **M25** Login "save missing + logout_time set" safety reject absent (silently resets char) — `modules/login/handler.go:186` — **(dep: M26)**. [R]
- [ ] **M26** Login `setLoggedOut` doesn't record logout_time/logged_out — `modules/login/db.go:221`. [R]
- [ ] **M27** Login reconnect can't re-serve save when client lost it — `modules/login/handler.go:148`. [R]
- [ ] **M28** Asset `.mid` path with no `_` panics (slice `[1:-1]`) vs TS clean 404 — `modules/asset/handler.go:84`. [R]
- [ ] **M29** Friend 100-entry friend/ignore cap not enforced — `modules/friends/repository.go:150,196`. [R]
- [ ] **M30** Obj full-follows gates on `CheckLifecycle` not `obj.isActive` (loc branch is correct) — `modules/world/player_zone.go:50`. [E/Q]

## LOW

- [ ] **L1** processObjDelayedQueue after processNpcs (TS before) — `tick.go:127` — ⚠LIE comment. [A]
- [ ] **L2** processInfo before processZones (documented NAI-93; 1-tick facing artifact) — `tick.go:146`. [A]
- [ ] **L3** processShutdown at top of tick vs TS end — `tick.go:60` — ⚠LIE comment. [A]
- [ ] **L4** autosavePlayers at top of tick vs TS end — `tick.go:71`. [A]
- [ ] **L5** intra-cleanup reset order differs (benign) — `tick.go:872`. [A]
- [ ] **L6** Timers fire while `loggingOut` (TS guards) — `tick.go:601`. [C]
- [ ] **L7** `preventLogoutMessage` never emitted — `tick.go:395`. [C]
- [ ] **L8** NPC `apRangeCalled` not reset per-tick (event-driven instead) — `npc_masks.go:250`. [D]
- [ ] **L9** `GetObj` lacks `isValid()` (count≥1) filter — `modules/world/obj_lookup.go:18`. [E]
- [ ] **L10** INV `beginSlot` skipped-indices second pass missing (latent; needed once restock uses beginSlot) — `inventory.go:296`. [F]
- [ ] **L11** INV `FromType` stockobj seeding diverges (skip id==0, count fallback) — `inventory.go:42`. [F]
- [ ] **L12** INV `AddOpts.BeginSlot` zero-value footgun (0 vs -1) — `inventory.go:250`. [F]
- [ ] **L13** INV stack-overflow basis uses GetItemCount not per-slot stackCount — `inventory.go:269`. [F]
- [ ] **L14** HINT_PL uses `s.Self2` not operand-aware — `handlers_player.go:1430`. [I]
- [ ] **L15** P_LOCMERGE uses `s.Self` not operand-aware — `handlers_player.go:2036`. [I]
- [ ] **L16** STAT_ADVANCE extra `checkStatID` (TS validates ticks only) — `handlers_player.go:588`. [I]
- [ ] **L17** P_OPOBJ errors on nil-Configs vs TS silent-skip — `handlers_player.go:1488`. [I]
- [ ] **L18** COORDX/Y/Z + INZONE/MOVECOORD/DISTANCE skip CoordValid check — `handlers_number.go:388`, `handlers_server.go:48`. [J]
- [ ] **L19** LOC2/OBJ2 read ops not operand-aware (documented NAI-119-D/154-D; no consumers) — `handlers_loc.go`/`handlers_obj.go`. [K]
- [ ] **L20** ActiveObj write-side not operand-aware in spawn handlers — `handlers_obj.go:116`, `handlers_inv.go:1048,1444`. [K]
- [ ] **L21** INV_SIZE requires resolvable player inv vs TS pure config read — `handlers_inv.go:131`. [K]
- [ ] **L22** OBJ_ADDALL lacks nil-World guard (twin OBJ_ADD has it) — `handlers_obj.go:178`. [K]
- [ ] **L23** OBJ_FIND validates coord before objType (TS reversed) — `handlers_obj.go:343`. [K]
- [ ] **L24** INTERPOLATE x1==x0 fallback y0 vs TS de-facto 0 — `handlers_number.go:368`. [L]
- [ ] **L25** MULTIPLY/POW int32-wrap vs TS float64→toInt32 (>2^53 edge) — `handlers_number.go:84,155`. [L]
- [ ] **L26** STRING_LENGTH/APPEND_CHAR UTF-8 byte vs TS UTF-16 unit — `handlers_string.go:80,46`. [L]
- [ ] **L27** Plain GOSUB/JUMP don't pop callee declared args — `handlers_core.go:8`. [H]
- [ ] **L28** SWITCH branches on key-presence vs TS truthy-offset — `handlers_array.go:59`. [H]
- [ ] **L29** Pointer bit positions differ from TS (internal-only) — `pkg/script/pointer.go:7` — ⚠LIE comment citing matching values. [H]
- [ ] **L30** Opcount cap off-by-one (`>=` vs TS `>`) — `pkg/script/state.go:54`. [H]
- [ ] **L31** NpcType accepts op 30-39 but Op is 5-slot → panic on 35-39 (latent, foreign caches) — `npctype.go:207`. [M]
- [ ] **L32** ObjType extra opcode 200 + `"hidden"→""` coercion (undocumented for obj/npc) — `objtype.go:292,246`, `npctype.go:213`. [M]
- [ ] **L33** varp/varn/vars fatal-error vs TS printError+continue on unknown opcode — `varptype.go:39` etc. [M]
- [ ] **L34** Component overlay `G1()!=0` vs TS gbool(`==1`) — `componenttype.go:331`. [M]
- [ ] **L35** login `lowMemory` decoded `==1` vs TS `& 0x1` (client only sends 0/1) — `pkg/io/protocol/login/req/req.go:88`. [N]
- [ ] **L36** `GBit(n)` returns uint8, truncates >8-bit reads (latent, no callers) — `pkg/io/packet/packetbit.go:32`. [N]
- [ ] **L37** Login validation RSA-decrypts before rev/CRC checks (TS gates first) — `modules/world/server.go:886`. [N]
- [ ] **L38** `GetCRC` slices `offset:offset+length` vs TS `i<length` end-index (latent, offset always 0) — `pkg/io/packet/packet.go:15`. [O]
- [ ] **L39** jagfile `Deconstruct` dead code, independently buggy — `pkg/io/jagfile/jagfile.go:318` — consider deleting. [O]
- [ ] **L40** jagfile name-label lookup non-deterministic map order — `jagfile.go:372`. [O]
- [ ] **L41** jagfile `knownNames` ordering differs (zero effect) — `jagfile.go:401`. [O]
- [ ] **L42** `lineScaleDown` arithmetic `>>16` vs logical `>>>16` (latent) — `pkg/pathfinder/routefinder/line.go:24`. [P]
- [ ] **L43** `FlagNull=-1` vs TS NULL=0x7FFFFFFF (Indoors strategy on off-map tiles) — `pkg/pathfinder/collision/flag.go:4`. [P]
- [ ] **L44** Friend list has no `ORDER BY created` (display order undefined) — `modules/friends/repository.go:169,255`. [R]
- [ ] **L45** Friend re-WORLD_CONNECT doesn't terminate prior socket (per-player-stream arch) — `modules/friends/handler.go:38`. [R]
- [ ] **L46** Login `updateHiscores` on logout is a bare TODO — `modules/login/handler.go:241`. [R]
- [ ] **L47** Asset path dispatch order: archives before `.mid` (TS reverse) — `modules/asset/handler.go:29`. [R]
- [ ] **L48** Asset WS open seed first word not masked to 24 bits — `modules/asset/server.go:779`. [R]
- [ ] **L49** entity `CheckLifecycle` is a Go-only predicate substituting TS `isActive` — `pkg/entity/entity.go:33` — resolve with M30. [Q]
- [ ] **L50** `gamemap` collision deferred to `populateStaticLocsIntoZones` (verify still matches TS blockwalk gate after H11-13) — `modules/world/server.go:592`. [Q]

---

## DO NOT FIX — CONFIRMED-EXCEPTIONS (verify against ledger before reclassifying)

- NpcMode QUEUE1..20 machinery — TS NpcMode.ts:147-167 commented out; Go forward-compat is correct. [D]
- `Inventory.transfer()` not a method — TS has zero callers; cert logic reimplemented in script layer. [F]
- Wealth-flush in-memory only — NAI-162-D, deferred to goscape telemetry. [A]
- NO_TIMEOUT + anticheat opcodes accept-and-discard — TS binds no handlers. [G]
- DEFINE_ARRAY/PUSH/POP_ARRAY_INT implemented — TS throws 'unimplemented'; Go extension. [L]
- DB find_db pointer-asymmetry — verified equivalent via ScriptOpcodePointers gating. [L]
- CategoryType resolve-at-pack (no runtime decoder) — deliberate architecture. [M]
- pixpack palette>255 quantize missing — NAI-213-D; TS itself flags CRC-divergent. [O]
- RANDOM/STAT_RANDOM PRNG sequence — RS randomness not wire-reproduced; range matches. [L]
- MoveSpeed.CRAWL branch — TS never assigns CRAWL (dead branch in reference). [B]
- friend/login gRPC+SQLite transport (vs TS WebSocket/Kysely) — deliberate goscape architecture. [R]
- Compiler stubs (PushConstantLong, long-arith/branch, queue-arg TODO) — TS throws/TODOs identically. [S]
