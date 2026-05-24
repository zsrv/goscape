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
| HIGH | 17 / 17 ✓ |
| DISPUTED | D1 RESOLVED ✓ (real bug, fixed with M2) |
| MEDIUM | 30 / 30 ✓ |
| LOW | 23 / 50 |
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

## DISPUTED — RESOLVED

- [x] **D1** `faceSquareX/Z` never reset per tick — `player_masks.go` / `npc_masks.go` — **(ca81da68)** **VERDICT: real bug (Player agent correct), unified with M2.** Traced vs TS PathingEntity.ts: faceSquare is a ONE-TICK event (reset L608-609); faceAngle is the persistent orientation fed to rsbuf, refreshed per-step by takeStep focus (L216-220). goscape conflated them (effectiveFaceCoord prefers faceSquare + no reset + no per-step focus) → stale square leaked to newly-visible observers via the forced low-def FACE_COORD. Fix = D1 reset + M2 focus together. Arc-30 #202 PRESERVED: unfocus()-on-spawn keeps faceAngle a valid south fallback. Fixed both lying comments. +2 pin tests.

## MEDIUM

- [x] **M1** Player `inApproachDistance` missing line-of-sight gate (can fire through walls; NPC side has it) — `modules/world/interaction.go:750`. [B] **(5c4be889)** — kept free inApproachDistance as the range half; added `(*Player).approachHasLineOfSight` (forward LoS, FlagBlockPlayers) AND'd at the tryInteract call site; closes DEVIATION S6l-D4. +pin test.
- [x] **M2** Per-step `focus()` not called → stale walk-facing — `movement.go:182`, `npc_interaction.go:439` — **(ca81da68)** — applyStep (player+npc) now calls focus(one tile ahead, client=false) per TS PathingEntity.ts:216-220; faceAngle tracks walk direction. Landed with D1 (same gap). +pin test.
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
- [x] **M23** IDLE_TIMER (opcode 70) routed to no-op vs TS `requestIdleLogout` — `handlers_game.go:37`. [G] **(abff5127)** — new handleIdleTimer sets requestIdleLogout unless NodeDebug (TS IdleTimerHandler.ts:8-12); flag already drained by processLogouts. +pin test (production-kicks / node_debug-keeps).
- [x] **M24** MoveClick passes start tile not dest in non-default routefinder config — `handlers_game.go:300`. [G] **(abff5127)** — pathToMoveClick now gets p.userPath (==[dest] in non-routefinder) not raw packed (whose [0] is START); TS MoveClickHandler.ts:40. Fixed mislabeled dest_packed log. +pin test; adjusted minimap trailer test to routefinder mode (full-path preservation is routefinder-only in TS; it had relied on the old always-pass-packed bug).
- [x] **M25** Login "save missing + logout_time set" safety reject absent (silently resets char) — `modules/login/handler.go:186` — **(dep: M26)**. [R] **(6bc61282)** — ErrNotExist branch now rejects (codes.DataLoss) when account.LogoutTime.Valid; NULL logout_time still → NEW_PLAYER. TS LoginServer.ts:342-348. +2 pin tests (reject + new-player complement).
- [x] **M26** Login `setLoggedOut` doesn't record logout_time/logged_out — `modules/login/db.go:221`. [R] **(6bc61282)** — stamps account.logout_time + clears logged_in in a tx (TS LoginServer.ts:429-440). DEVIATION: logout_time on `account` table (not account_login); no `logged_out` column (TS bookkeeping, read nowhere). +pin test.
- [x] **M27** Login reconnect can't re-serve save when client lost it — `modules/login/handler.go:148`. [R] **(6bc61282)** — reconnect now keys on Reconnecting+same-node (not req.HasSave, which the world never sets); inserts session, reads+verifies+re-serves save on !HasSave, returns RECONNECT_OK (DataLoss reject if unreadable). TS LoginServer.ts:271-317. Was falling through to full-login → OK not RECONNECT_OK. +2 pin tests.
- [x] **M28** Asset `.mid` path with no `_` panics (slice `[1:-1]`) vs TS clean 404 — `modules/asset/handler.go:84`. [R] **(ba3d795c)** — explicit 404 when LastIndex("_")<1 (TS web.ts:68 clamps to a non-existent file → 404). +pin test (3 malformed paths).
- [x] **M29** Friend 100-entry friend/ignore cap not enforced — `modules/friends/repository.go:150,196`. [R] **(ba3d795c)** — atomicUpsertList counts owner's entries before a NEW insert; silent no-op at friendListLimit=100 (TS FriendServerRepository). +2 pin tests (friend + ignore).
- [x] **M30** Obj full-follows gates on `CheckLifecycle` not `obj.isActive` (loc branch is correct) — `modules/world/player_zone.go:50`. [E/Q] **(cd271589)** — gate swapped to stored `obj.IsActive` (TS Zone.ts:142-146: both Despawn+isActive & Respawn+isActive emit ObjAdd). Updated 2 direct-inject tests to set IsActive=true (bypass AddObj). L49 (CheckLifecycle now only test-referenced) is the LOW-tier follow-up.

## LOW

- [x] **L1** processObjDelayedQueue after processNpcs (TS before) — `tick.go:127` — ⚠LIE comment. [A] **(4053cbb2)** — comment corrected: real deviation = delayed objs visible to NPC obj-hunt 1 tick later; placement gives delay=0 player drops same-tick visibility. Accepted LOW (no reorder — load-bearing NAI-134/217).
- [x] **L2** processInfo before processZones (documented NAI-93; 1-tick facing artifact) — `tick.go:146`. [A] **(4053cbb2)** — accepted deviation, already documented NAI-93; added call-site breadcrumb.
- [x] **L3** processShutdown at top of tick vs TS end — `tick.go:60` — ⚠LIE comment. [A] **(4053cbb2)** — "Mirrors TS" corrected; top-placement is deliberate (doomed conn cut 1 tick earlier). Accepted LOW.
- [x] **L4** autosavePlayers at top of tick vs TS end — `tick.go:71`. [A] **(4053cbb2)** — added deviation note; identical cadence, snapshot consistent either way. Accepted LOW.
- [x] **L5** intra-cleanup reset order differs (benign) — `tick.go:872`. [A] **(4053cbb2)** — added doc note; no cross-step dependency makes order observable. Accepted LOW.
- [x] **L6** Timers fire while `loggingOut` (TS guards) — `tick.go:601`. [C] **(6520ece9)** — processPlayerTimers now skips logging-out players (both NORMAL+SOFT), TS World.ts:717-722. +pin test (soft timer suppressed).
- [x] **L7** `preventLogoutMessage` never emitted — `tick.go:395`. [C] **(6520ece9)** — processLogouts emits + consumes preventLogoutMessage inside the prevent window (requestLogout only), TS World.ts:765-767. +pin test.
- [x] **L8** NPC `apRangeCalled` not reset per-tick (event-driven instead) — `npc_masks.go:250`. [D] **(842e03a8)** — added per-tick reset in npc resetPathingEntity (TS L588); field is write-only on NPC side today (no reader), so TS-faithful + future-proof, zero behavior change. npc has no `interacted` field (TS L587 N/A).
- [x] **L9** `GetObj` lacks `isValid()` (count≥1) filter — `modules/world/obj_lookup.go:18`. [E] **(842e03a8)** — GetObj skips count<1 || !IsActive (TS getObjsSafe→isValid). Meaningful: static RESPAWN objs linger inactive after being taken → prevents re-take before respawn. +pin test; set IsActive=true in 4 test injection sites (bypass AddObj, hard-rule #2; TS Entity ctor is isActive=false too so NewObj is correct).
- [x] **L10** INV `beginSlot` skipped-indices second pass missing — `inventory.go:296`. [F] **(7a810f52)** — Remove now does TS second pass (Inventory.ts:256-316): BeginSlot>=1 scans [begin,cap) then wraps to prefix [0,begin). Premise "all callers use -1" is STALE — restock decay passes BeginSlot=index (tick.go:1056,1060), though that caller keeps id at start slot so wrap not exercised today. +pin test.
- [x] **L11** INV `FromType` stockobj seeding diverges (skip id==0, count fallback) — `inventory.go:42`. [F] **(7a810f52)** — now seeds literal {stockobj[i], stockcount[i]} for every index (TS Inventory.ts:66-73). Load-bearing post-H3: count-0 stock slot must seed at 0 (restocks up) not 1; obj id 0 is valid. +pin test; world suite green (shop restock is live consumer).
- [x] **L12** INV `AddOpts.BeginSlot` zero-value footgun (0 vs -1) — `inventory.go:250`. [F] **(7a810f52)** — documented on AddOpts/RemoveOpts: TS default is -1 sentinel ("append from first free"), Go zero value 0 is a real slot index; all 7 callers pass -1, doc warns future callers must too. No behavior change.
- [x] **L13** INV stack-overflow basis uses GetItemCount not per-slot stackCount — `inventory.go:269`. [F] **(7a810f52)** — stack write now clamps by PER-SLOT count + SETs slot to total (TS Inventory.ts:229-237); sum (previousCount) still gates entry. Diverges only w/ duplicate stacks of one id. +pin test.
- [x] **L14** HINT_PL uses `s.Self2` not operand-aware — `handlers_player.go:1430`. [I] **(fd5335d9)** — now uses operand-aware `s.activePlayer2()` (swaps to Self at operand 1) per TS PlayerOps.ts:976-978; guards already bind both slots.
- [x] **L15** P_LOCMERGE uses `s.Self` not operand-aware — `handlers_player.go:2036`. [I] **(fd5335d9)** — merge-owner now `s.activePlayer()` (TS:929); was lone protected-op outlier vs P_TELEPORT/P_WALK.
- [x] **L16** STAT_ADVANCE extra `checkStatID` (TS validates ticks only) — `handlers_player.go:588`. [I] **(fd5335d9)** — dropped checkStatID; TS forwards OOB stat to addXp (TypedArray no-op), Go AddXP already bounds-guards (statBounds→return) so OOB now no-ops + script continues vs aborts. -1 still rejected by NumberNotNull. ⚠TEST: updated TestStatOpsRejectOOBStatID (STAT_ADVANCE id=21 → Finished not Aborted).
- [x] **L17** P_OPOBJ errors on nil-Configs vs TS silent-skip — `handlers_player.go:1488`. [I] **(fd5335d9)** — nil Configs / unregistered ObjType now treated as empty-op default type → silent skip (TS ObjType.get returns default, never null; PlayerOps.ts:996-998). +pin test.
- [x] **L18** COORDX/Y/Z + INZONE/MOVECOORD/DISTANCE skip CoordValid check — `handlers_number.go:388`, `handlers_server.go:48`. [J] **(89d89a2d)** — all 6 now route through existing checkCoord (TS CoordValid [0,2^31-1]); abort on negative coord vs prior silent bit-mask. TS validation order preserved (DISTANCE c1→c2, INZONE from→to→pos, MOVECOORD base-coord only). +pin test.
- [x] **L19** LOC2/OBJ2 read ops not operand-aware — **(c95d6cd7, doc-confirm)** CONFIRMED accurate at HEAD as documented deferral NAI-154-D/119-D (state.go): no OBJ_*/LOC_* read handler reads the secondary slot; no `.obj2`/`.loc2` read consumers in content. NAI-154-D comment refreshed to cite the L20 spawn writers + still-deferred read accessor. No code change. [K]
- [x] **L20** ActiveObj write-side not operand-aware in spawn handlers — **(c95d6cd7)** OBJ_ADD/OBJ_ADDALL (objAddCommon), INV_DROPSLOT, INV_DROPITEM now route through operand-aware setActiveObjSlot (matches OBJ_FIND/FINDNEXT + TS ObjOps.ts:50-53/82-85, InvOps.ts:184-185/250-258). Operand-0 unchanged. [K]
- [x] **L21** INV_SIZE requires resolvable player inv vs TS pure config read — **(c95d6cd7)** now reads `s.Configs.InvType(id).Size` directly (TS InvOps.ts:27-31), works without a bound inv. +pin TestInvSize_NoResolvableInv_L21. [K]
- [x] **L22** OBJ_ADDALL lacks nil-World guard — **(c95d6cd7)** added matching twin guard (no Self guard — broadcast path). +pin TestHandleObjAddAllNilWorldGuard. [K]
- [x] **L23** OBJ_FIND validates coord before objType — **(c95d6cd7)** order swapped to objType-then-coord per TS ObjOps.ts:172-173. +pin TestObjFindValidatesObjTypeBeforeCoord. [K]
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
