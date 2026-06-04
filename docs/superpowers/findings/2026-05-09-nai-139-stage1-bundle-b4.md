# NAI-139 Stage 1 — Bundle B4: ~update_all proc subtree audit

**Date:** 2026-05-09  
**TS source:** `LostCityRS/Content/scripts/player/scripts/appearance.rs2:98-119`  
**Depth:** top-level opcodes (direct) + depth-1 procs (one level down only)

---

## Confirmed proc body (appearance.rs2:98-119)

```
[proc,update_all](obj $previous_weapon)
if (staffmodlevel >= 3 & inv_getobj(worn, ^wearpos_rhand) = poisoned_dagger_p) {
    stat_add(hitpoints, calc(255 - stat(hitpoints)), 0);
    … [× 7 skills]
}
if (p_finduid(uid) = true) {
    p_animprotect(^false);
}
~update_weight_equipment;
~update_bas;
~update_bonuses;
~update_weight;
if (%tutorial > ^newbie_combat_instructor_unequipping_items) {
    ~update_weapon_category($previous_weapon);
}
~player_combat_stat;
```

---

## Audit Table

| token | kind | ts_ref | goscape_dispatch | status | evidence |
|-------|------|--------|------------------|--------|----------|
| staffmodlevel | opcode | appearance.rs2:100 | handlers.go:134 → handlers_dialog.go:104 | WIRED | `OpStaffModLevel: handleStaffModLevel,` at handlers.go:134; impl at handlers_dialog.go:104 `s.PushInt(int(s.Self.StaffModLevel()))` |
| inv_getobj | opcode | appearance.rs2:100 | handlers.go:300 → handlers_inv.go:44 | WIRED | `OpInvGetObj: handleInvGetObj,` at handlers.go:300; cross-bundle B1+B3 overlap; `func handleInvGetObj(s *ScriptState)` at handlers_inv.go:44 |
| stat_add | opcode | appearance.rs2:101-107 | handlers.go:218 → handlers_player.go:305 | WIRED | `OpStatAdd: handleStatAdd,` at handlers.go:218; cross-bundle B2 overlap; `func handleStatAdd(s *ScriptState)` at handlers_player.go:305 |
| p_finduid | opcode | appearance.rs2:109 | handlers.go:467 → handlers_player.go:959 | WIRED | `OpPFindUID: handlePFindUID,` at handlers.go:467; NAI-111 lifecycle verified; handler pushes 1 on self-reacquire fast-path (uid match + protected slot set), 0 on miss |
| p_animprotect | opcode | appearance.rs2:110 | handlers.go:470 → handlers_player.go:153 | WIRED | `OpPAnimProtect: handlePAnimProtect,` at handlers.go:470; gate is `requireProtectedActivePlayer`; calls `s.Self.SetAnimProtect(v)` at handlers_player.go:161 |
| %tutorial (PUSH_VARP) | opcode | appearance.rs2:116 | handlers.go:206 → handlers_vars.go:42 | WIRED | `OpPushVarp: handlePushVarp,` at handlers.go:206; `func handlePushVarp(s *ScriptState)` at handlers_vars.go:42 |
| ^newbie_combat_instructor_unequipping_items | const | tutorial.constant:50 | n/a — compiler constant | WIRED | value = **400**; `^newbie_combat_instructor_unequipping_items = 400` at tutorial/configs/tutorial.constant:50; baked into bytecode by runescript compiler |
| ^tutorial_complete | const | quest.constant:1 | n/a — compiler constant | WIRED | value = **1000**; `^tutorial_complete = 1000` at general/configs/quest.constant:1; 1000 > 400 ⇒ `~update_weapon_category` FIRES on tutorial completion |
| GOSUB (proc dispatch) | opcode | appearance.rs2:112-119 | handlers.go:83 | WIRED | `OpGosub: handleGosub,` at handlers.go:83; `OpGosubWithParams: handleGosubWithParams,` at handlers.go:30 |

---

## Depth-1 proc audit

### ~update_weight_equipment (equip.rs2:398-411)

Top-level opcodes: `inv_total`, `map_members`, `inv_del`, `inv_setslot`, `inv_add`

| token | goscape_dispatch | status | evidence |
|-------|-----------------|--------|----------|
| inv_total | handlers.go:299 | WIRED | `OpInvTotal: handleInvTotal,` |
| map_members | handlers.go:89 | WIRED | `OpMapMembers: handleMapMembers,` |
| inv_del | handlers.go:310 | WIRED | `OpInvDel: handleInvDel,` |
| inv_setslot | handlers.go:312 | WIRED | `OpInvSetSlot: handleInvSetSlot,` |
| inv_add | handlers.go:309 | WIRED | `OpInvAdd: handleInvAdd,` |

All 5 opcodes WIRED. Proc body is engine-contained; no unresolved calls.

### ~update_bas (appearance.rs2:1-24)

Top-level opcodes/calls: `~inzone_coord_pair_table` (GOSUB), `buildappearance`, `inv_getobj`, `readyanim`, `turnanim`, `walkanim`, `walkanim_b`, `walkanim_l`, `walkanim_r`, `runanim`, `oc_param`

| token | goscape_dispatch | status | evidence |
|-------|-----------------|--------|----------|
| ~inzone_coord_pair_table | GOSUB → handleGosub | WIRED | proc defined at coord_procs.rs2:49; dispatched via OpGosub at handlers.go:83 |
| buildappearance | handlers.go:473 | WIRED | `OpBuildAppearance: handleBuildAppearance,` |
| inv_getobj | handlers.go:300 | WIRED | same as top-level |
| readyanim | handlers.go:237 | WIRED | `OpReadyAnim: handleReadyAnim,` |
| turnanim | handlers.go:238 | WIRED | `OpTurnAnim: handleTurnAnim,` |
| walkanim | handlers.go:239 | WIRED | `OpWalkAnim: handleWalkAnim,` |
| walkanim_b | handlers.go:240 | WIRED | `OpWalkAnimB: handleWalkAnimB,` |
| walkanim_l | handlers.go:241 | WIRED | `OpWalkAnimL: handleWalkAnimL,` |
| walkanim_r | handlers.go:242 | WIRED | `OpWalkAnimR: handleWalkAnimR,` |
| runanim | handlers.go:243 | WIRED | `OpRunAnim: handleRunAnim,` |
| oc_param | handlers.go:280 | WIRED | `OpOcParam: handleOcParam,` |

All 11 WIRED.

### ~update_bonuses (equip.rs2:107-148)

Top-level opcodes/calls: `~equip_get_bonuses` (GOSUB), `%prayer_drain_resistance` (POP_VARP), `add`, `multiply`, `~update_bonus_text` (GOSUB)

| token | goscape_dispatch | status | evidence |
|-------|-----------------|--------|----------|
| ~equip_get_bonuses | GOSUB → handleGosub | WIRED | proc at equip.rs2:195; dispatched via OpGosub/OpGosubWithParams |
| %prayer_drain_resistance (POP_VARP) | handlers.go:207 | WIRED | `OpPopVarp: handlePopVarp,`; varp defined at prayer.varp:70; POP_VARP requires protected access check at handlers_vars.go:70 |
| add | handlers.go:27 | WIRED | `OpAdd: handleAdd,` |
| multiply | handlers.go:44 | WIRED | `OpMultiply: handleMultiply,` |
| ~update_bonus_text | GOSUB → handleGosub | WIRED | dispatched via OpGosub |

All 5 WIRED.

### ~update_weight (equip.rs2:296-319)

**Body is empty** — lines 297-319 are developer requirement comments only; no RuneScript statements. The proc returns immediately when called via GOSUB.

Engine-side weight computation fires independently via `(*Player).updateInvs` inv-listener loop at `modules/world/player.go:832-837`: `runWeightChanged` flag → `p.calculateRunWeight()` → `sendUpdateRunWeight`. This is the NAI-136 wiring (commit 82b7dbf).

| token | goscape_dispatch | status | evidence |
|-------|-----------------|--------|----------|
| ~update_weight (empty proc body) | GOSUB → immediate return | WIRED | proc body is comments only (equip.rs2:297-319); real work in updateInvs at player.go:832-837 via calculateRunWeight() (player_runweight.go:16) |
| OpUpdateRunWeight (game prot) | modules/world via sendUpdateRunWeight | WIRED | NAI-136 commit 82b7dbf: "Adds OpUpdateRunWeight (opcode 22, payload 2) to game server prot." |

NAI-136 alignment confirmed: `calculateRunWeight` exists at `modules/world/player_runweight.go:16`; `runWeightChanged` triggers at `player.go:832-835`.

### ~update_weapon_category (player_attackstyles.rs2:93-131)

Top-level opcodes/calls: `oc_wearpos`, `inv_getobj`, `~player_autocast_reset` (GOSUB), `~inzone_coord_pair_table` (GOSUB), `if_settab`, `map_members`, `oc_members`, `switch_category` (OpSwitch), `oc_category`, `~weapon_category_tab_attack` (GOSUB), `~weapon_category_tab_attack_unarmed` (GOSUB), `if_setobject`, `if_settext`, `oc_name`

| token | goscape_dispatch | status | evidence |
|-------|-----------------|--------|----------|
| oc_wearpos | handlers.go:285 | WIRED | `OpOcWearPos: handleOcWearPos,` at handlers.go:285 |
| inv_getobj | handlers.go:300 | WIRED | same as top-level |
| ~player_autocast_reset | GOSUB → handleGosub | WIRED | proc at auto_cast.rs2:54; dispatched via OpGosub |
| ~inzone_coord_pair_table | GOSUB → handleGosub | WIRED | same as ~update_bas |
| if_settab | handlers.go:344 | WIRED | `OpIfSetTab: handleIfSetTab,` |
| map_members | handlers.go:89 | WIRED | same as ~update_weight_equipment |
| oc_members | handlers.go:283 | WIRED | `OpOcMembers: handleOcMembers,` |
| switch_category (OpSwitch) | handlers.go:203 | WIRED | `OpSwitch: handleSwitch,` at handlers.go:203 |
| oc_category | handlers.go:281 | WIRED | `OpOcCategory: handleOcCategory,` |
| if_setobject | handlers.go:345 | WIRED | `OpIfSetObject: handleIfSetObject,` |
| if_settext | handlers.go:338 | WIRED | `OpIfSetText: handleIfSetText,` |
| oc_name | — | WIRED | `OpOcName` at opcode.go:355; dispatched via handleOcName |

All 12 WIRED.

### ~player_combat_stat (player_combat_stat.rs2:1-50+)

Top-level opcodes/calls: `~equip_get_bonuses` (GOSUB), `stat`, `inv_getobj`, `~combat_get_weapon_style_data` (GOSUB), `%com_mode` (PUSH_VARP + POP_VARP), `db_getfieldcount`, `min`, `sub`, `~combat_get_damagetype` (GOSUB), `~combat_get_damagestyle` (GOSUB)

| token | goscape_dispatch | status | evidence |
|-------|-----------------|--------|----------|
| ~equip_get_bonuses | GOSUB → handleGosub | WIRED | same as ~update_bonuses |
| stat | handlers.go:215 | WIRED | `OpStat: handleStat,` |
| inv_getobj | handlers.go:300 | WIRED | same as top-level |
| db_getfieldcount | handlers.go:164 | WIRED | `OpDbGetFieldCount: handleDbGetFieldCount,` |
| %com_mode (PUSH_VARP/POP_VARP) | handlers.go:206-207 | WIRED | `OpPushVarp/OpPopVarp` registered |
| min | handlers.go:50 | WIRED | `OpMin: handleMin,` |
| sub | handlers.go:28 | WIRED | `OpSub: handleSub,` |
| ~combat_get_weapon_style_data | GOSUB → handleGosub | WIRED | depth-1 proc call; dispatched via OpGosub |
| ~combat_get_damagetype | GOSUB → handleGosub | WIRED | depth-1 proc call; dispatched via OpGosub |
| ~combat_get_damagestyle | GOSUB → handleGosub | WIRED | depth-1 proc call; dispatched via OpGosub |

All 10 WIRED.

---

## Staff cheat branch analysis

**Gate:** `staffmodlevel >= 3 & inv_getobj(worn, ^wearpos_rhand) = poisoned_dagger_p`

For a tutorial-completion player: `staffModLevel` is sourced from the gRPC login response (`resp.GetStaffModLevel()`) at `modules/world/client.go:51-54`. Default zero value for regular players. `handleStaffModLevel` at handlers_dialog.go:104 pushes `int(s.Self.StaffModLevel())` = **0**. Since `0 >= 3` is false, the entire staff cheat block is **dead on the smoke path**. Even if `staffmodlevel` were unwired, the branch would never execute for a normal player. Classification: WIRED but branch dormant for smoke.

---

## Tutorial gate analysis

`^newbie_combat_instructor_unequipping_items` = **400** (tutorial.constant:50)  
`^tutorial_complete` = **1000** (quest.constant:1)

At tutorial completion `%tutorial` reaches 1000. The gate `%tutorial > 400` evaluates **true**, so `~update_weapon_category($previous_weapon)` **fires** during tutorial-complete `~update_all` execution. All ~update_weapon_category opcodes are WIRED (see depth-1 table above).

---

## Summary

**All tokens audited: WIRED.** No STUB or MISSING rows found in B4 scope.

- 8 direct opcodes in ~update_all body: all WIRED  
- 6 depth-1 procs, 40+ opcode/proc-call surface points: all WIRED  
- ~update_weight empty body + NAI-136 engine-side wiring: confirmed correct  
- Staff cheat branch: WIRED but dormant (staffModLevel = 0 for normal players)  
- ~update_weapon_category fires at tutorial completion: confirmed (1000 > 400)
