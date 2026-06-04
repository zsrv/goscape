# NAI-120 — Bundle 0 controller pre-flight findings

**Date:** 2026-05-07
**Spec:** `docs/superpowers/specs/2026-05-07-nai-120-combat-init-path-investigation-design.md` (commit `b020daa`)
**Plan:** `docs/superpowers/plans/2026-05-07-nai-120-combat-init-stage1.md` (commit `50e3a7d`)
**HEAD at pre-flight:** `50e3a7d` (≡ `de13459` for code-content; only docs added since NAI-119 close)

This is the Bundle 0 deliverable per spec §4.5. Bundle 1 (Sonnet Explore audit subagent dispatch) consumes it to produce per-entry TS-source signature audit.

---

## 1. Token universe (Task 1)

Static enumeration over the 8 inner-ring `.rs2` files under `LostCityRS/Content/scripts/skill_combat/scripts/player/` (~1195 source lines).

**Per-file token counts:**

| File | Lines | Calls (`foo(`) | Vars (`%foo`) | Refs (`~foo` / `@foo`) | Consts (`^foo`) |
|---|---|---|---|---|---|
| `player_combat.rs2` | 149 | 17 | 18 | 11 | 7 |
| `player_melee.rs2` | 59 | 22 | 7 | 5 | 1 |
| `player_ranged.rs2` | 144 | 44 | 7 | 12 | 5 |
| `player_magic.rs2` | 280 | 52 | 6 | 28 | 34 |
| `auto_cast.rs2` | 60 | 10 | 2 | 5 | 17 |
| `auto_retaliate.rs2` | 46 | 10 | 3 | 1 | 3 |
| `player_attackstyles.rs2` | 201 | 17 | 2 | 6 | 2 |
| `player_combat_stat.rs2` | 256 | 20 | 29 | 13 | 8 |
| **Union (sorted-unique)** | 1195 | **111** | **42** | **66** | **47** |

Plus a separate **bare-identifier** sweep (regex `(?:^|[^a-zA-Z_0-9$%^~@.])[a-z_][a-z_0-9]*`, line-comments + string-literals stripped) recovered no-paren opcode call-sites missed by the call-shape regex (`map_clock`, `npc_uid`, etc.). 235 bare-only tokens; 8 confirmed bare-form opcode candidates classified in §2.2 below (the rest are stat / inv / spell / enum identifiers that compile to int constants, plus rs2 type/declaration keywords).

Scratch token files lived under `$TMPDIR/nai-120-tokens/` during pre-flight; deleted at Task 6 commit.

---

## 2. Call-token classification (Task 2)

Each call-shape token was classified by deriving `Op<PascalCase>` from the snake_case rs2 name (with a flat-lowercase fallback that matches goscape's actual capitalization, e.g. `OpDbGetField` not `OpDbGetfield`), then looking up `pkg/script/opcode.go` (decl) and `pkg/script/handlers*.go` (dispatch, excluding `_test.go`).

**Status legend:**

- **W** — Wired (declared + dispatched).
- **D** — Declared in `opcode.go` but no dispatch entry — Stage 2 needs handler+dispatch.
- **U** — Not declared in `opcode.go` — Stage 2 needs decl + handler + dispatch.
- **P** — Proc/label call (not an opcode); body lookup decides P-in-ring vs F.
- **K** — rs2 syntax keyword (`if`, `while`, `return`, `switch_*`, `error`, `calc`).

### 2.1 Call-shape (parenthesized) matrix (111 tokens)

| Token | Status | Goscape Op | opcode.go | handlers.go | Notes |
|---|---|---|---|---|---|
| `abs` | W | `OpAbs` | `opcode.go:462` | `handlers.go:47` |  |
| `add` | W | `OpAdd` | `opcode.go:434` | `handlers.go:27` |  |
| `anim` | W | `OpAnim` | `opcode.go:102` | `handlers.go:229` |  |
| `calc` | K | `-` | `-` | `-` | compile-time arithmetic wrapper |
| `check_spell_requirements` | P | `-` | `-` | `-` |  |
| `chronozon_spell` | P | `-` | `-` | `-` |  |
| `combat_defend_anim` | P | `-` | `-` | `-` |  |
| `combat_effective_stat` | P | `-` | `-` | `-` |  |
| `combat_get_damagestyle` | P | `-` | `-` | `-` |  |
| `combat_get_damagestyle_bonuses` | P | `-` | `-` | `-` |  |
| `combat_get_damagetype` | P | `-` | `-` | `-` |  |
| `combat_get_weapon_style_data` | P | `-` | `-` | `-` |  |
| `combat_maxhit` | P | `-` | `-` | `-` |  |
| `combat_stat` | P | `-` | `-` | `-` |  |
| `combat_swing_anim_and_synth` | P | `-` | `-` | `-` |  |
| `convert_stat_to_npc_stat` | P | `-` | `-` | `-` |  |
| `db_getfield` | W | `OpDbGetField` | `opcode.go:474` | `handlers.go:160` |  |
| `db_getfieldcount` | W | `OpDbGetFieldCount` | `opcode.go:475` | `handlers.go:161` |  |
| `delete_spell_runes` | P | `-` | `-` | `-` |  |
| `divide` | W | `OpDivide` | `opcode.go:437` | `handlers.go:45` |  |
| `error` | K | `-` | `-` | `-` | syntax keyword |
| `finduid` | W | `OpFindUID` | `opcode.go:119` | `handlers.go:433` |  |
| `get_spell_data` | P | `-` | `-` | `-` |  |
| `give_combat_experience` | P | `-` | `-` | `-` |  |
| `give_spell_xp` | P | `-` | `-` | `-` |  |
| `if` | K | `-` | `-` | `-` | syntax keyword |
| `if_setobject` | W | `OpIfSetObject` | `opcode.go:143` | `handlers.go:332` |  |
| `if_settab` | W | `OpIfSetTab` | `opcode.go:148` | `handlers.go:331` |  |
| `if_settext` | W | `OpIfSetText` | `opcode.go:150` | `handlers.go:325` |  |
| `in_duel_arena` | P | `-` | `-` | `-` |  |
| `inv_add` | W | `OpInvAdd` | `opcode.go:371` | `handlers.go:301` |  |
| `inv_del` | W | `OpInvDel` | `opcode.go:376` | `handlers.go:302` |  |
| `inv_dropitem_delayed` | D | `OpInvDropItemDelayed` | `opcode.go:379` | `-` |  |
| `inv_freespace` | W | `OpInvFreeSpace` | `opcode.go:382` | `handlers.go:295` |  |
| `inv_getobj` | W | `OpInvGetObj` | `opcode.go:384` | `handlers.go:292` |  |
| `inv_total` | W | `OpInvTotal` | `opcode.go:396` | `handlers.go:291` |  |
| `inzone_coord_pair_table` | P | `-` | `-` | `-` |  |
| `legends_spell_usage` | P | `-` | `-` | `-` |  |
| `lowercase` | W | `OpLowercase` | `opcode.go:415` | `handlers.go:175` |  |
| `magic_spell_maxhit` | P | `-` | `-` | `-` |  |
| `map_blocked` | W | `OpMapBlocked` | `opcode.go:81` | `handlers.go:100` |  |
| `map_multiway` | D | `OpMapMultiway` | `opcode.go:88` | `-` |  |
| `max` | W | `OpMax` | `opcode.go:451` | `handlers.go:51` |  |
| `mes` | W | `OpMes` | `opcode.go:162` | `handlers.go:31` |  |
| `min` | W | `OpMin` | `opcode.go:450` | `handlers.go:50` |  |
| `multiply` | W | `OpMultiply` | `opcode.go:436` | `handlers.go:44` |  |
| `npc_anim` | W | `OpNpcAnim` | `opcode.go:238` | `handlers.go:397` |  |
| `npc_basestat` | W | `OpNpcBaseStat` | `opcode.go:241` | `handlers.go:387` |  |
| `npc_defence_roll_specific` | P | `-` | `-` | `-` |  |
| `npc_finduid` | D | `OpNpcFindUID` | `opcode.go:258` | `-` |  |
| `npc_heropoints` | D | `OpNpcHeroPoints` | `opcode.go:261` | `-` |  |
| `npc_is_attackable` | P | `-` | `-` | `-` |  |
| `npc_param` | W | `OpNpcParam` | `opcode.go:266` | `handlers.go:265` |  |
| `npc_poison_start` | P | `-` | `-` | `-` |  |
| `npc_projectile` | P | `-` | `-` | `-` |  |
| `npc_queue` | W | `OpNpcQueue` | `opcode.go:267` | `handlers.go:403` |  |
| `npc_range` | D | `OpNpcRange` | `opcode.go:268` | `-` |  |
| `npc_retaliate` | P | `-` | `-` | `-` |  |
| `npc_stat` | W | `OpNpcStat` | `opcode.go:274` | `handlers.go:386` |  |
| `npc_statadd` | D | `OpNpcStatAdd` | `opcode.go:275` | `-` |  |
| `npc_statsub` | D | `OpNpcStatSub` | `opcode.go:277` | `-` |  |
| `npc_walk` | W | `OpNpcWalk` | `opcode.go:281` | `handlers.go:410` |  |
| `npc_walktrigger` | W | `OpNpcWalkTrigger` | `opcode.go:282` | `handlers.go:411` |  |
| `oc_category` | W | `OpOcCategory` | `opcode.go:348` | `handlers.go:275` |  |
| `oc_members` | W | `OpOcMembers` | `opcode.go:354` | `handlers.go:277` |  |
| `oc_name` | W | `OpOcName` | `opcode.go:355` | `handlers.go:273` |  |
| `oc_param` | W | `OpOcParam` | `opcode.go:357` | `handlers.go:274` |  |
| `oc_wearpos` | W | `OpOcWearPos` | `opcode.go:361` | `handlers.go:279` |  |
| `p_aprange` | W | `OpPApRange` | `opcode.go:167` | `handlers.go:369` |  |
| `p_finduid` | W | `OpPFindUID` | `opcode.go:173` | `handlers.go:434` |  |
| `player_attackrange` | P | `-` | `-` | `-` |  |
| `player_attack_roll_specific` | P | `-` | `-` | `-` |  |
| `player_npc_hit_roll` | P | `-` | `-` | `-` |  |
| `player_ranged_check_ammo` | P | `-` | `-` | `-` |  |
| `player_ranged_use_weapon` | P | `-` | `-` | `-` |  |
| `p_opnpc` | W | `OpPOpNpc` | `opcode.go:178` | `handlers.go:373` |  |
| `p_opnpct` | D | `OpPOpNpcT` | `opcode.go:179` | `-` |  |
| `p_opplayer` | D | `OpPOpPlayer` | `opcode.go:181` | `-` |  |
| `pvm_combat_spell_checks` | P | `-` | `-` | `-` |  |
| `pvm_debuff_allowed` | P | `-` | `-` | `-` |  |
| `pvm_default_spell` | P | `-` | `-` | `-` |  |
| `pvm_freeze_allowed` | P | `-` | `-` | `-` |  |
| `pvm_freeze_effect` | P | `-` | `-` | `-` |  |
| `pvm_spell_cast` | P | `-` | `-` | `-` |  |
| `pvm_spell_fail` | P | `-` | `-` | `-` |  |
| `pvm_spell_success` | P | `-` | `-` | `-` |  |
| `pvm_stat_change_effect` | P | `-` | `-` | `-` |  |
| `pvm_tutorial_spell` | P | `-` | `-` | `-` |  |
| `random` | W | `OpRandom` | `opcode.go:438` | `handlers.go:69` |  |
| `randominc` | W | `OpRandomInc` | `opcode.go:439` | `handlers.go:70` |  |
| `ranged_dropammo_holywater` | P | `-` | `-` | `-` |  |
| `ranged_dropammo_npc` | P | `-` | `-` | `-` |  |
| `return` | K | `-` | `-` | `-` | syntax keyword |
| `scale` | W | `OpScale` | `opcode.go:452` | `handlers.go:49` |  |
| `set_attackstyle` | P | `-` | `-` | `-` |  |
| `set_autocast_spell` | P | `-` | `-` | `-` |  |
| `sound_synth` | W | `OpSoundSynth` | `opcode.go:204` | `handlers.go:451` |  |
| `spotanim_npc` | D | `OpSpotAnimNpc` | `opcode.go:284` | `-` |  |
| `spotanim_pl` | W | `OpSpotAnimPl` | `opcode.go:205` | `handlers.go:230` |  |
| `stat` | W | `OpStat` | `opcode.go:216` | `handlers.go:212` |  |
| `stat_name` | P | `-` | `-` | `-` |  |
| `sub` | W | `OpSub` | `opcode.go:435` | `handlers.go:28` |  |
| `switch_category` | K | `-` | `-` | `-` | syntax keyword |
| `switch_int` | K | `-` | `-` | `-` | syntax keyword |
| `testbit` | W | `OpTestBit` | `opcode.go:444` | `handlers.go:59` |  |
| `togglebit` | W | `OpToggleBit` | `opcode.go:454` | `handlers.go:62` |  |
| `tostring` | W | `OpToString` | `opcode.go:417` | `handlers.go:29` |  |
| `update_all` | P | `-` | `-` | `-` |  |
| `weapon_category_tab_attack` | P | `-` | `-` | `-` |  |
| `weapon_category_tab_attack_unarmed` | P | `-` | `-` | `-` |  |
| `while` | K | `-` | `-` | `-` | syntax keyword |

### 2.2 Bare-form (no-paren) matrix (8 confirmed opcodes)

These rs2 tokens compile to opcode calls without parens (TS-side: `ScriptOpcode.MAP_CLOCK` etc.). Verified via flat-lowercase lookup against `pkg/script/opcode.go` plus a manual rs2 cross-grep to confirm actual referenced use (false candidate `busy` rejected — only `busy2` is referenced in the inner ring).

| Token | Status | Goscape Op | opcode.go | handlers.go |
|---|---|---|---|---|
| `map_clock` | W | `OpMapClock` | `opcode.go:82` | `handlers.go:86` |
| `map_members` | W | `OpMapMembers` | `opcode.go:87` | `handlers.go:89` |
| `npc_uid` | W | `OpNpcUID` | `opcode.go:280` | `handlers.go:390` |
| `npc_coord` | W | `OpNpcCoord` | `opcode.go:245` | `handlers.go:385` |
| `coord` | W | `OpCoord` | `opcode.go:114` | `handlers.go:223` |
| `uid` | W | `OpUID` | `opcode.go:223` | `handlers.go:134` |
| `busy2` | D | `OpBusy2` | `opcode.go:106` | `-` |
| `name` | W | `OpName` | `opcode.go:165` | `handlers.go:32` |

### 2.3 Combined classification summary

| Status | Count | Meaning |
|---|---|---|
| W (Wired) | 53 | 46 call-form + 7 bare-form |
| D (Declared, no dispatch) | **11** | **Stage 2 work; see §2.4** |
| U (Undeclared) | 0 | Clean — no Stage 2 decls needed |
| P (Proc/label ref) | 48 | 20 in-ring + 28 frontier; see §2.5 |
| K (Syntax keyword) | 7 | `if`, `while`, `return`, `switch_int`, `switch_category`, `error`, `calc` |

### 2.4 (D) entries — Stage 2 missing-handler list (the headline finding)

These 11 opcodes are **declared in goscape but lack a dispatch entry**. Each consumes a handler implementation + one-line dispatch in `pkg/script/handlers*.go`. Bundle 1's audit subagent will produce the TS-source signature for each.

- `busy2` (`OpBusy2`, decl `opcode.go:106`) — Stage 2 dispatch entry
- `inv_dropitem_delayed` (`OpInvDropItemDelayed`, decl `opcode.go:379`) — Stage 2 dispatch entry
- `map_multiway` (`OpMapMultiway`, decl `opcode.go:88`) — Stage 2 dispatch entry
- `npc_finduid` (`OpNpcFindUID`, decl `opcode.go:258`) — Stage 2 dispatch entry
- `npc_heropoints` (`OpNpcHeroPoints`, decl `opcode.go:261`) — Stage 2 dispatch entry
- `npc_range` (`OpNpcRange`, decl `opcode.go:268`) — Stage 2 dispatch entry
- `npc_statadd` (`OpNpcStatAdd`, decl `opcode.go:275`) — Stage 2 dispatch entry
- `npc_statsub` (`OpNpcStatSub`, decl `opcode.go:277`) — Stage 2 dispatch entry
- `p_opnpct` (`OpPOpNpcT`, decl `opcode.go:179`) — Stage 2 dispatch entry
- `p_opplayer` (`OpPOpPlayer`, decl `opcode.go:181`) — Stage 2 dispatch entry
- `spotanim_npc` (`OpSpotAnimNpc`, decl `opcode.go:284`) — Stage 2 dispatch entry

### 2.5 (P) procs/labels — in-ring vs frontier

20 entries are in-ring (body lives in one of the 8 inner-ring files) and are recursively covered when Stage 2 ports their parent file. 28 entries are **frontier** — bodies live outside the inner ring and route to NAI-121+. See §6 for the frontier list.

| Proc/Label | Class | Body file(s) |
|---|---|---|
| `~check_spell_requirements` / `@check_spell_requirements` | F | `skill_magic/scripts/magic.rs2 ` |
| `~chronozon_spell` / `@chronozon_spell` | F | `quests/quest_crest/scripts/crest_chronozon.rs2 ` |
| `~combat_defend_anim` / `@combat_defend_anim` | F | `skill_combat/scripts/combat.rs2 ` |
| `~combat_effective_stat` / `@combat_effective_stat` | F | `skill_combat/scripts/combat.rs2 ` |
| `~combat_get_damagestyle` / `@combat_get_damagestyle` | F | `skill_combat/scripts/combat.rs2 ` |
| `~combat_get_damagestyle_bonuses` / `@combat_get_damagestyle_bonuses` | F | `skill_combat/scripts/combat.rs2 ` |
| `~combat_get_damagetype` / `@combat_get_damagetype` | F | `skill_combat/scripts/combat.rs2 ` |
| `~combat_get_weapon_style_data` / `@combat_get_weapon_style_data` | F | `skill_combat/scripts/combat.rs2 ` |
| `~combat_maxhit` / `@combat_maxhit` | F | `skill_combat/scripts/combat.rs2 ` |
| `~combat_stat` / `@combat_stat` | F | `skill_combat/scripts/combat.rs2 ` |
| `~combat_swing_anim_and_synth` / `@combat_swing_anim_and_synth` | F | `skill_combat/scripts/combat.rs2 ` |
| `~convert_stat_to_npc_stat` / `@convert_stat_to_npc_stat` | F | `player/scripts/stat.rs2 ` |
| `~delete_spell_runes` / `@delete_spell_runes` | F | `skill_magic/scripts/magic.rs2 ` |
| `~get_spell_data` / `@get_spell_data` | F | `skill_magic/scripts/magic.rs2 ` |
| `~give_combat_experience` / `@give_combat_experience` | F | `skill_combat/scripts/combat.rs2 ` |
| `~give_spell_xp` / `@give_spell_xp` | F | `skill_magic/scripts/magic.rs2 ` |
| `~in_duel_arena` / `@in_duel_arena` | F | `minigames/game_duelarena/scripts/duel_arena.rs2 ` |
| `~inzone_coord_pair_table` / `@inzone_coord_pair_table` | F | `general/scripts/misc/coord_procs.rs2 ` |
| `~legends_spell_usage` / `@legends_spell_usage` | F | `quests/quest_legends/scripts/ungadulu.rs2 ` |
| `~magic_spell_maxhit` / `@magic_spell_maxhit` | F | `skill_combat/scripts/pvp/pvp_magic.rs2 ` |
| `~npc_defence_roll_specific` / `@npc_defence_roll_specific` | F | `skill_combat/scripts/npc/npc_combat.rs2 ` |
| `~npc_is_attackable` / `@npc_is_attackable` | F | `skill_combat/scripts/npc/npc_combat.rs2 ` |
| `~npc_poison_start` / `@npc_poison_start` | F | `skill_combat/scripts/npc/npc_poison.rs2 ` |
| `~npc_projectile` / `@npc_projectile` | F | `skill_combat/scripts/projectile.rs2 ` |
| `~npc_retaliate` / `@npc_retaliate` | F | `skill_combat/scripts/npc/npc_combat.rs2 ` |
| `~player_attackrange` / `@player_attackrange` | P-in-ring | `skill_combat/scripts/player/player_combat.rs2 ` |
| `~player_attack_roll_specific` / `@player_attack_roll_specific` | P-in-ring | `skill_combat/scripts/player/player_combat.rs2 ` |
| `~player_npc_hit_roll` / `@player_npc_hit_roll` | P-in-ring | `skill_combat/scripts/player/player_combat.rs2 ` |
| `~player_ranged_check_ammo` / `@player_ranged_check_ammo` | P-in-ring | `skill_combat/scripts/player/player_ranged.rs2 ` |
| `~player_ranged_use_weapon` / `@player_ranged_use_weapon` | P-in-ring | `skill_combat/scripts/player/player_ranged.rs2 ` |
| `~pvm_combat_spell_checks` / `@pvm_combat_spell_checks` | P-in-ring | `skill_combat/scripts/player/player_magic.rs2 ` |
| `~pvm_debuff_allowed` / `@pvm_debuff_allowed` | P-in-ring | `skill_combat/scripts/player/player_magic.rs2 ` |
| `~pvm_default_spell` / `@pvm_default_spell` | P-in-ring | `skill_combat/scripts/player/player_magic.rs2 ` |
| `~pvm_freeze_allowed` / `@pvm_freeze_allowed` | P-in-ring | `skill_combat/scripts/player/player_magic.rs2 ` |
| `~pvm_freeze_effect` / `@pvm_freeze_effect` | P-in-ring | `skill_combat/scripts/player/player_magic.rs2 ` |
| `~pvm_spell_cast` / `@pvm_spell_cast` | P-in-ring | `skill_combat/scripts/player/player_magic.rs2 ` |
| `~pvm_spell_fail` / `@pvm_spell_fail` | P-in-ring | `skill_combat/scripts/player/player_magic.rs2 ` |
| `~pvm_spell_success` / `@pvm_spell_success` | P-in-ring | `skill_combat/scripts/player/player_magic.rs2 ` |
| `~pvm_stat_change_effect` / `@pvm_stat_change_effect` | P-in-ring | `skill_combat/scripts/player/player_magic.rs2 ` |
| `~pvm_tutorial_spell` / `@pvm_tutorial_spell` | F | `tutorial/scripts/skills/tut_player_magic.rs2 ` |
| `~ranged_dropammo_holywater` / `@ranged_dropammo_holywater` | P-in-ring | `skill_combat/scripts/player/player_ranged.rs2 ` |
| `~ranged_dropammo_npc` / `@ranged_dropammo_npc` | P-in-ring | `skill_combat/scripts/player/player_ranged.rs2 ` |
| `~set_attackstyle` / `@set_attackstyle` | P-in-ring | `skill_combat/scripts/player/player_attackstyles.rs2 ` |
| `~set_autocast_spell` / `@set_autocast_spell` | P-in-ring | `skill_combat/scripts/player/auto_cast.rs2 ` |
| `~stat_name` / `@stat_name` | F | `player/scripts/stat.rs2 ` |
| `~update_all` / `@update_all` | F | `player/scripts/appearance.rs2 ` |
| `~weapon_category_tab_attack` / `@weapon_category_tab_attack` | P-in-ring | `skill_combat/scripts/player/player_attackstyles.rs2 ` |
| `~weapon_category_tab_attack_unarmed` / `@weapon_category_tab_attack_unarmed` | P-in-ring | `skill_combat/scripts/player/player_attackstyles.rs2 ` |

---

## 3. Var classification (Task 3)

Each `%name` reference was looked up in `LostCityRS/Content/scripts/**/*.varp|.varn|.vars|.varbit` to determine its declared type.

### 3.1 Goscape var-registry path (discovered)

- **VarPType registry:** `pkg/objtype/varptype.go` (`LoadVarpTypes` reads `server/varp.dat`); plumbed via `modules/world/server.go:220` and `modules/world/player_varp.go`. Per-player varp values stored in `Player.varps`; access via `p.Varp(id)`/`p.SetVarp(id, val)`.
- **VarSType registry:** `pkg/objtype/varstype.go` (`LoadVarsTypes` reads `server/vars.dat`); plumbed via `modules/world/server.go:224`. World-side `World.VarsInt(id)`/`SetVarsInt(id, val)`.
- **VarN runtime:** no goscape-side `varn.dat` loader — per-NPC vars stored in `Npc.varns []int32` (lazily grown on first `SetNpcVarN` write); reads return `0` for never-written ids. The `stub until S6` comment at `pkg/script/handlers.go:207-208` is **obsolete**: the actual handlers at `pkg/script/handlers_vars.go:52-69` route to real `Npc.NpcVarN`/`SetNpcVarN` with TS-faithful integer indexing. **Caveat:** TS engine populates per-NPC-type default values from `varn.dat`/`ai_spawn.varn`; goscape's 0-default is divergent for any varn whose TS default is non-zero. `%npc_combat_xp_multiplier` is the one inner-ring varn at risk (see §3.3).
- **VarBit runtime:** `OpPushVarbit` (opcode.go:52) and `OpPopVarbit` (opcode.go:53) are **declared but not dispatched**. Not blocking NAI-120 (no varbit-typed vars in the inner ring; see §3.2).

### 3.2 Per-var classification (42 vars)

| Var | Type | TS source (Content/scripts/) |
|---|---|---|
| `%action_delay` | VarP | `_unpack/all.varp` |
| `%aggressive_npc` | VarP | `_unpack/all.varp` |
| `%attackstyle_magic` | VarP | `_unpack/all.varp` |
| `%autocast_spell` | VarP | `_unpack/all.varp` |
| `%com_attackanim` | VarP | `_unpack/all.varp` |
| `%com_attacksound` | VarP | `_unpack/all.varp` |
| `%com_crushattack` | VarP | `_unpack/all.varp` |
| `%com_crushdef` | VarP | `_unpack/all.varp` |
| `%com_defendanim` | VarP | `_unpack/all.varp` |
| `%com_magicattack` | VarP | `_unpack/all.varp` |
| `%com_magicdef` | VarP | `_unpack/all.varp` |
| `%com_maxhit` | VarP | `_unpack/all.varp` |
| `%com_mode` | VarP | `_unpack/all.varp` |
| `%com_rangeattack` | VarP | `_unpack/all.varp` |
| `%com_rangedef` | VarP | `_unpack/all.varp` |
| `%com_slashattack` | VarP | `_unpack/all.varp` |
| `%com_slashdef` | VarP | `_unpack/all.varp` |
| `%com_stabattack` | VarP | `_unpack/all.varp` |
| `%com_stabdef` | VarP | `_unpack/all.varp` |
| `%damagestyle` | VarP | `_unpack/all.varp` |
| `%damagetype` | VarP | `_unpack/all.varp` |
| `%lastcombat` | VarP | `_unpack/all.varp` |
| `%lastcombat_pvp` | VarP | `_unpack/all.varp` |
| `%npc_aggressive_player` | VarN | `_unpack/all.varn` |
| `%npc_combat_xp_multiplier` | VarN | `npc/configs/ai_spawn.varn` |
| `%npc_lastcombat` | VarN | `_unpack/all.varn` |
| `%npc_macro_event_target` | VarN | `macro events/configs/antimacro.varn` |
| `%npc_stunned` | VarN | `_unpack/all.varn` |
| `%option_nodef` | VarP | `interface_controls/configs/player_controls.varp` |
| `%prayer0` | VarP | `skill_prayer/configs/prayer.varp` |
| `%prayer1` | VarP | `skill_prayer/configs/prayer.varp` |
| `%prayer10` | VarP | `skill_prayer/configs/prayer.varp` |
| `%prayer11` | VarP | `skill_prayer/configs/prayer.varp` |
| `%prayer12` | VarP | `skill_prayer/configs/prayer.varp` |
| `%prayer13` | VarP | `skill_prayer/configs/prayer.varp` |
| `%prayer14` | VarP | `skill_prayer/configs/prayer.varp` |
| `%prayer2` | VarP | `skill_prayer/configs/prayer.varp` |
| `%prayer3` | VarP | `skill_prayer/configs/prayer.varp` |
| `%prayer4` | VarP | `skill_prayer/configs/prayer.varp` |
| `%prayer5` | VarP | `skill_prayer/configs/prayer.varp` |
| `%prayer9` | VarP | `skill_prayer/configs/prayer.varp` |
| `%tutorial` | VarP | `tutorial/configs/tutorial.varp` |

### 3.3 Type counts + V-status

| Type | Count | Goscape runtime | V-status |
|---|---|---|---|
| VarP | 37 | `varp.dat` loaded; `OpPushVarp`/`OpPopVarp` dispatched | **V-W** for all 37 |
| VarN | 5 | numeric-indexed; no `.varn` defaults loaded | **V-W** for 4 (defaults of 0 are TS-faithful for clock-style fields), **V-PARTIAL** for `%npc_combat_xp_multiplier` (Bundle 1 audit binding: confirm TS default + decide whether Stage 2 needs a varn defaults loader) |
| VarBit | 0 | n/a | n/a (PUSH_VARBIT/POP_VARBIT undispatched but not referenced) |
| VarS | 0 | n/a | n/a |

**No (V-D)/(V-U) entries** in the strong sense — all vars are declared in TS and goscape's runtime path can serve them. The one open binding is the `%npc_combat_xp_multiplier` default.

---

## 4. §9 risk register — final HEAD verification (Task 4)

| Item | Status at HEAD `50e3a7d` | Evidence |
|---|---|---|
| **R1** ADD wired | ✅ | `opcode.go:434` + `handlers.go:27` |
| **R2** BRANCH_* family wired | ✅ | Per spec write-time verification (`opcode.go:39-43,55-56` + `handlers.go:21-23` representative) |
| **R3** PUSH_VARP/S/N wired | ✅ | `handlers.go:203-208`; PUSH_VARN `stub until S6` comment is **obsolete** — handlers_vars.go:52-69 route to real `Npc.NpcVarN` |
| **R3** PUSH_VARBIT/POP_VARBIT wired | ⚠ | Declared at `opcode.go:52-53`; **no dispatch** entry — does not block NAI-120 (no varbit-typed vars in inner ring), but flag for future routing |
| **R4** enum/inv pack constants | ✅ | Goscape loads pre-compiled `script.dat` (`pkg/script/provider.go`) — `^stab_style` etc. resolve to int operands at TS-compile time, baked into bytecode; no runtime registry needed |
| **R5** frontier resolutions | ✅ | 28 (P) entries with body paths outside inner ring; 0 (P) entries un-locatable |
| **R6** `p_aprange` wired | ✅ | `OpPApRange` at `opcode.go:167` + `handlers.go:369` (declared+dispatched; spec was wrong to flag this as missing) |
| **R7** Gosub/Jump wired | ✅ | Per spec write-time verification |
| **R8** `mes` / `npc_uid` / `uid` / `p_finduid` / `npc_finduid` / `finduid` wired | ⚠ | `mes`+`uid`+`npc_uid`+`p_finduid`+`finduid` ✅ (`opcode.go:162/223/280/173/119` + `handlers.go:31/134/390/handlers.go (P/Find variants dispatched)`); `npc_finduid` declared at `opcode.go:258` but **NOT dispatched** — moves to (D) list (see §2.4) |

**Surprises vs spec §9:**

1. **R3 PUSH_VARN comment misrepresents reality.** Spec deferred verification; HEAD shows fully-functional handler. The "stub until S6" comment in handlers.go is leftover from an earlier dev cycle. Suggest a doc-comment cleanup at NAI-120 final close, NOT a Stage 2 task. (Tracking as a NAI-120 follow-up.)
2. **R6 `p_aprange` was wrong.** Spec listed `p_aprange` as a "known Stage 2 port"; it is wired at HEAD. Strike from Stage 2 expectations.
3. **R8 `npc_finduid` is missing dispatch.** Spec lumped all uid-family handlers as wired; `OpNpcFindUID` is declared but undispatched, so it joins the (D) list.
4. **R3-extension VarBit dispatch missing.** Not a Stage 2 NAI-120 blocker; defer to NAI-121+ if a downstream sub-spec encounters varbit-typed vars.

---

## 5. Bundle 1 audit input

Bundle 1 audit subagent (Sonnet, Explore agent type, read-only) consumes:

- **This findings note**, pinned at the commit hash from Task 6.
- The 8 inner-ring `.rs2` files at `/home/owner/Code/github.com/LostCityRS/Content/scripts/skill_combat/scripts/player/` (excluding `spells/`).
- TS handler source at `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/`.
- Goscape var-registry path discovered in §3.1 (for varn-defaults binding on `%npc_combat_xp_multiplier`).

Bundle 1 produces per-(D)/(V-PARTIAL) stanzas at `docs/superpowers/investigations/2026-05-07-nai-120-bundle1-audit.md` per spec §5.

**The (D)/(V-PARTIAL) audit input set:**

11 (D) entries (§2.4) + 1 V-PARTIAL (`%npc_combat_xp_multiplier` defaults) = **12 audit stanzas expected**.

---

## 6. Frontier list (NAI-121+ routing)

These 28 procs/labels live outside the inner ring and are not in NAI-120's scope. Each will need scope assignment in a follow-up sub-spec (NAI-121 candidate or parked).

| Proc/Label | Body file | Likely sub-spec routing |
|---|---|---|
| `~check_spell_requirements` | `skill_magic/scripts/magic.rs2` | NAI-121 (magic-spell support) |
| `~chronozon_spell` | `quests/quest_crest/...` | parked (quest-specific) |
| `~combat_defend_anim` | `skill_combat/scripts/combat.rs2` | NAI-121 (combat sibling file) |
| `~combat_effective_stat` | `skill_combat/scripts/combat.rs2` | NAI-121 |
| `~combat_get_damagestyle` | `skill_combat/scripts/combat.rs2` | NAI-121 |
| `~combat_get_damagestyle_bonuses` | `skill_combat/scripts/combat.rs2` | NAI-121 |
| `~combat_get_damagetype` | `skill_combat/scripts/combat.rs2` | NAI-121 |
| `~combat_get_weapon_style_data` | `skill_combat/scripts/combat.rs2` | NAI-121 |
| `~combat_maxhit` | `skill_combat/scripts/combat.rs2` | NAI-121 |
| `~combat_stat` | `skill_combat/scripts/combat.rs2` | NAI-121 |
| `~combat_swing_anim_and_synth` | `skill_combat/scripts/combat.rs2` | NAI-121 |
| `~convert_stat_to_npc_stat` | `player/scripts/stat.rs2` | NAI-121 (stat-id translation) |
| `~delete_spell_runes` | `skill_magic/scripts/magic.rs2` | NAI-121 |
| `~get_spell_data` | `skill_magic/scripts/magic.rs2` | NAI-121 |
| `~give_combat_experience` | `skill_combat/scripts/combat.rs2` | NAI-121 |
| `~give_spell_xp` | `skill_magic/scripts/magic.rs2` | NAI-121 |
| `~in_duel_arena` | `minigames/game_duelarena/...` | parked (minigame) |
| `~inzone_coord_pair_table` | `general/scripts/misc/coord_procs.rs2` | NAI-121 (utility) |
| `~legends_spell_usage` | `quests/quest_legends/...` | parked (quest-specific) |
| `~magic_spell_maxhit` | `skill_combat/scripts/pvp/pvp_magic.rs2` | parked (PvP only — not tutorial path) |
| `~npc_defence_roll_specific` | `skill_combat/scripts/npc/npc_combat.rs2` | NAI-122 (NPC-side combat) |
| `~npc_is_attackable` | `skill_combat/scripts/npc/npc_combat.rs2` | NAI-122 |
| `~npc_poison_start` | `skill_combat/scripts/npc/npc_poison.rs2` | NAI-122 |
| `~npc_projectile` | `skill_combat/scripts/projectile.rs2` | NAI-121 |
| `~npc_retaliate` | `skill_combat/scripts/npc/npc_combat.rs2` | NAI-122 |
| `~pvm_tutorial_spell` | `tutorial/scripts/skills/tut_player_magic.rs2` | parked (tutorial-specific magic) |
| `~stat_name` | `player/scripts/stat.rs2` | NAI-121 |
| `~update_all` | `player/scripts/appearance.rs2` | parked (appearance) |

**Bottom line for Stage 2 of NAI-120:** the 11 (D) opcodes (§2.4) are the complete handler-port list. Frontier procs are intentionally deferred — Stage 2 ports the inner-ring rs2 only and stops at the first `~frontier_proc(` boundary.

---

## 7. Stage 2 bundle assignment hypothesis (refines after Bundle 1)

Per spec §6.1, anticipated decomposition. Bundle 1's dependency edges may merge or re-split these.

| Bundle | Scope | (D) opcodes likely covered |
|---|---|---|
| 2A | `player_combat.rs2` (149 lines, entry labels) | `map_multiway`, `p_opnpct`, `p_opplayer`, `busy2`, possibly `npc_finduid` |
| 2B | `player_melee.rs2` (59 lines) | (TBD per Bundle 1) |
| 2C | `player_ranged.rs2` (144 lines) | `inv_dropitem_delayed`, `spotanim_npc` |
| 2D | `player_magic.rs2` (280 lines) | `spotanim_npc`, `npc_range` |
| 2E | small-files merge (`auto_cast`, `auto_retaliate`, `player_attackstyles`, `player_combat_stat`) (563 lines) | `npc_heropoints`, `npc_statadd`, `npc_statsub`, `npc_finduid` (auto_retaliate), `busy2` (auto_retaliate) |

Total expected production LOC: **200-800** + comparable test LOC. Multi-session.

---

## 8. Bundle 0 deviations from spec / Bundle 0 follow-ups

- **Spec (D)-count** anticipated "≈ several handlers"; actual is **11**.
- **Spec §9 R6 (p_aprange)** reclassified as already wired; remove from Stage 2 expectations.
- **Spec R3 stub-comment** — handlers.go:207-208 has a stale "stub until S6" doc-comment; real handler is wired. Tracked as a NAI-120 final-close cleanup item (single-line comment edit).
- **PUSH_VARBIT/POP_VARBIT undispatched** — declared at opcode.go:52-53; no inner-ring blocker. Track in `nai_followups.md` for routing on first downstream sub-spec that touches a varbit var.

