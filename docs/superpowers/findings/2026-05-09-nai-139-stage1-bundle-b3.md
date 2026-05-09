# NAI-139 Stage 1 — Bundle B3: `~initalltabs` subtree audit

**Date:** 2026-05-09  
**TS ref:** `LostCityRS/Content/scripts/login_logout/login.rs2:62-87`  
**Scope:** `~initalltabs` body + transitive procs `~update_weapon_category`, `~update_questlist`

---

## 1. Confirmed TS proc bodies

### `[proc,initalltabs]` — login.rs2:62-87 (Read-verified)

```
~update_weapon_category(inv_getobj(worn, ^wearpos_rhand));
if_settab(stats, ^tab_skills);
if_settab(questlist, ^tab_quest_journal);
~update_questlist;
inv_transmit(inv, inventory:inv);
if_settab(inventory, ^tab_inventory);
inv_transmit(worn, wornitems:wear);
if_settab(wornitems, ^tab_wornitems);
if_settab(prayer, ^tab_prayer);
if_settab(magic, ^tab_magic);
if_settab(friends, ^tab_friends);
if_settab(ignore, ^tab_ignore);
if_settab(logout, ^tab_logout);
if_settab(controls, ^tab_player_controls);
if (lowmem = true) {
    if_settab(options_ld, ^tab_game_options);
    if_settab(music_ld, ^tab_musicplayer);
} else {
    if_settab(options, ^tab_game_options);
    if_settab(music, ^tab_musicplayer);
}
```

### `[proc,update_weapon_category]` — player_attackstyles.rs2:93-131 (Read-verified)

Key opcodes used: `inv_getobj(worn, ^wearpos_rhand)`, `oc_wearpos`, `oc_category`, `map_members`, `switch_category` (→ SWITCH), `if_settab`. Calls sub-procs `~player_autocast_reset`, `~inzone_coord_pair_table` (proc, GOSUB), `~weapon_category_tab_attack`, `~weapon_category_tab_attack_unarmed`.

### `[proc,update_questlist]` — quests.rs2:217-256 (Read-verified)

Key opcodes used: `if_setcolour` (via `~send_quest_progress_colour`). Calls sub-procs `~update_questpoints`, `~send_quest_progress_colour` ×N. No additional primitive opcodes beyond `if_setcolour`.

### `[proc,send_quest_progress_colour]` — quests.rs2:5-13 (Read-verified)

Uses: `if_setcolour`. No new opcodes.

---

## 2. Opcode / proc / interface / constant audit table

| token | kind | ts_ref | goscape_dispatch | status | evidence |
|-------|------|--------|------------------|--------|----------|
| `if_settab` | opcode | login.rs2:64 | pkg/script/handlers.go:344 | WIRED | `OpIfSetTab: handleIfSetTab,` — handler at handlers_interface.go:262; emits `gameserver.OpIfSetTab` (wire opcode 167, 3-byte payload) via `modules/world/player_interface.go:79` |
| `inv_transmit` | opcode | login.rs2:68 | pkg/script/handlers.go:323 | WIRED | `OpInvTransmit: handleInvTransmit,` — handler at handlers_inv.go:671 |
| `inv_getobj` | opcode | login.rs2:63 | pkg/script/handlers.go:300 | WIRED | `OpInvGetObj: handleInvGetObj,` — handler at handlers_inv.go:42 |
| `lowmem` (LOWMEM) | opcode | login.rs2:80 | pkg/script/handlers.go:120 | WIRED | `OpLowMem: handleLowMem,` — handler at handlers_player.go:1272; reads `s.Self.LowMemory()` from login-request bit |
| `if_setcolour` | opcode | quests.rs2:8 | pkg/script/handlers.go:346 | WIRED | `OpIfSetColour: handleIfSetColour,` — handler at handlers_interface.go:302 |
| `oc_wearpos` | opcode | player_attackstyles.rs2:95 | pkg/script/handlers.go:285 | WIRED | `OpOcWearPos: handleOcWearPos,` — handler at handlers_config.go:526 |
| `oc_category` | opcode | player_attackstyles.rs2:99 | pkg/script/handlers.go:281 | WIRED | `OpOcCategory: handleOcCategory,` — handler at handlers_config.go:462 |
| `map_members` | opcode | player_attackstyles.rs2:108 | pkg/script/handlers.go:89 | WIRED | `OpMapMembers: handleMapMembers,` — handler at handlers_server.go:27 |
| `switch_category` (→ SWITCH) | opcode | player_attackstyles.rs2:112 | pkg/script/handlers.go:203 | WIRED | `OpSwitch: handleSwitch,` — handler at handlers_array.go:52 |
| `~update_weapon_category` | proc | player_attackstyles.rs2:93 | N/A — GOSUB bytecode | WIRED | RuneScript proc; dispatched via GOSUB (core opcode). No separate goscape handler needed. |
| `~update_questlist` | proc | quests.rs2:217 | N/A — GOSUB bytecode | WIRED | RuneScript proc; dispatched via GOSUB (core opcode). No separate goscape handler needed. |
| `~inzone_coord_pair_table` | proc | coord_procs.rs2:49 | N/A — GOSUB bytecode | WIRED | RuneScript proc call; dispatched via GOSUB. |
| `~player_autocast_reset` | proc | (referenced player_attackstyles.rs2:102) | N/A — GOSUB bytecode | WIRED | RuneScript proc call; dispatched via GOSUB. |
| `~send_quest_progress_colour` | proc | quests.rs2:5 | N/A — GOSUB bytecode | WIRED | RuneScript proc call; dispatched via GOSUB. |
| `~update_questpoints` | proc | quests.rs2:50 | N/A — GOSUB bytecode | WIRED | RuneScript proc call; dispatched via GOSUB. |
| `stats` (interface) | iface | login.rs2:64 | pkg/objtype/componenttype.go:336 | WIRED | `configs[id].ComName = debugName; configNames[debugName] = id` — source file `player/interfaces/stats.if` confirmed present |
| `questlist` (interface) | iface | login.rs2:65 | pkg/objtype/componenttype.go:336 | WIRED | Source file `player/interfaces/questlist.if` confirmed present |
| `inventory` (interface) | iface | login.rs2:69 | pkg/objtype/componenttype.go:336 | WIRED | Source file `player/interfaces/inventory.if` confirmed present |
| `wornitems` (interface) | iface | login.rs2:72 | pkg/objtype/componenttype.go:336 | WIRED | Source file `player/interfaces/wornitems.if` confirmed present |
| `prayer` (interface) | iface | login.rs2:74 | pkg/objtype/componenttype.go:336 | WIRED | Source file `skill_prayer/interfaces/prayer.if` confirmed present |
| `magic` (interface) | iface | login.rs2:75 | pkg/objtype/componenttype.go:336 | WIRED | Source file `skill_magic/interfaces/magic.if` confirmed present |
| `friends` (interface) | iface | login.rs2:76 | pkg/objtype/componenttype.go:336 | WIRED | Source file `player/interfaces/friends.if` confirmed present |
| `ignore` (interface) | iface | login.rs2:77 | pkg/objtype/componenttype.go:336 | WIRED | Source file `player/interfaces/ignore.if` confirmed present |
| `logout` (interface) | iface | login.rs2:78 | pkg/objtype/componenttype.go:336 | WIRED | Source file `player/interfaces/logout.if` confirmed present |
| `controls` (interface) | iface | login.rs2:79 | pkg/objtype/componenttype.go:336 | WIRED | Source file `interface_controls/interfaces/controls.if` confirmed present |
| `options` (interface) | iface | login.rs2:84 | pkg/objtype/componenttype.go:336 | WIRED | Source file `interface_options/interfaces/options.if` confirmed present |
| `options_ld` (interface) | iface | login.rs2:81 | pkg/objtype/componenttype.go:336 | WIRED | Source file `interface_options/interfaces/options_ld.if` confirmed present |
| `music` (interface) | iface | login.rs2:85 | pkg/objtype/componenttype.go:336 | WIRED | Source file `music/interfaces/music.if` confirmed present |
| `music_ld` (interface) | iface | login.rs2:82 | pkg/objtype/componenttype.go:336 | WIRED | Source file `music/interfaces/music_ld.if` confirmed present |
| `^tab_combat_options` | const | tabs.constant:1 | N/A — bytecode literal | WIRED | `^tab_combat_options = 0` (not used in initalltabs; present for completeness) |
| `^tab_skills` | const | tabs.constant:2 | N/A — bytecode literal | WIRED | `^tab_skills = 1` — `if_settab(stats, 1)` |
| `^tab_quest_journal` | const | tabs.constant:3 | N/A — bytecode literal | WIRED | `^tab_quest_journal = 2` — `if_settab(questlist, 2)` |
| `^tab_inventory` | const | tabs.constant:4 | N/A — bytecode literal | WIRED | `^tab_inventory = 3` — `if_settab(inventory, 3)` |
| `^tab_wornitems` | const | tabs.constant:5 | N/A — bytecode literal | WIRED | `^tab_wornitems = 4` — `if_settab(wornitems, 4)` |
| `^tab_prayer` | const | tabs.constant:6 | N/A — bytecode literal | WIRED | `^tab_prayer = 5` — `if_settab(prayer, 5)` |
| `^tab_magic` | const | tabs.constant:7 | N/A — bytecode literal | WIRED | `^tab_magic = 6` — `if_settab(magic, 6)` |
| `^tab_friends` | const | tabs.constant:8 | N/A — bytecode literal | WIRED | `^tab_friends = 8` — NOTE: slot 7 absent from tabs.constant |
| `^tab_ignore` | const | tabs.constant:9 | N/A — bytecode literal | WIRED | `^tab_ignore = 9` |
| `^tab_logout` | const | tabs.constant:10 | N/A — bytecode literal | WIRED | `^tab_logout = 10` |
| `^tab_game_options` | const | tabs.constant:11 | N/A — bytecode literal | WIRED | `^tab_game_options = 11` |
| `^tab_player_controls` | const | tabs.constant:12 | N/A — bytecode literal | WIRED | `^tab_player_controls = 12` |
| `^tab_musicplayer` | const | tabs.constant:13 | N/A — bytecode literal | WIRED | `^tab_musicplayer = 13` |

---

## 3. Notes

### if_settab wire-format verification

`handleIfSetTab` (handlers_interface.go:262) calls `s.Self.IfSetTab(com, tab)`.  
`Player.IfSetTab` (modules/world/player_interface.go:72-79):
- Writes `p.tabs[tab] = com` (server-side state for `IsComponentVisible`).
- Encodes `P2(com) + P1(tab)` → emits `gameserver.OpIfSetTab` (wire opcode 167, 3-byte payload, prot.go:32).  
This is the RS2 `IF_SETTAB` / `OPENSUB` equivalent for this era. Confirmed production packet emission, not a stub.

### lowmem property

`LowMemory()` is declared in `pkg/script/active.go:548-552` and implemented on `*Player` in `modules/world/`. The value is carried from the RS2 login request's LowMemory bit (established at login handshake). Handler at handlers_player.go:1272 pushes `1` or `0` accordingly.

### Interface config loading

Interface names are resolved at cache-load time via `LoadComponentTypes` → `parseComponentTypes` (componenttype.go:158-342). Server-side names come from `server/interface.dat`; the `configNames` map is keyed by `debugName` string. All 14 `.if` source files confirmed present in `LostCityRS/Content/scripts/`. The compiled cache will emit their names into `server/interface.dat`, making all 14 lookable. No MISSING interfaces.

### Tab constants — gap at slot 7

`tabs.constant` defines slots 0–6 and 8–13 (slot 7 absent). This is an intentional TS gap — the RS2 sidebar has no slot 7 in this era. `initalltabs` never emits `if_settab(..., 7)`, so no behavioral impact.

### Cross-bundle overlaps

- `inv_getobj`: also appears in B1+B4 (declared overlap — no coordination needed).
- `~update_weapon_category`: also appears in B4 (declared overlap — no coordination needed).

---

## 4. Summary

**All tokens WIRED.** No MISSING or STUB rows in the B3 surface area.  
The `~initalltabs` subtree relies on: `IF_SETTAB` (opcode+wire fully wired), `INV_TRANSMIT` (wired), `INV_GETOBJ` (wired), `LOWMEM` (wired), `IF_SETCOLOUR` (wired), `OC_WEARPOS` (wired), `OC_CATEGORY` (wired), `MAP_MEMBERS` (wired), `SWITCH` (wired). All 14 interface configs exist in content source. All 12 tab constants resolve to expected sidebar slot indices.
