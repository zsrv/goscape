# NAI-184 — Port `getCombatLevel` + recompute-on-stat-change

**Predecessors:** NAI-184 cheat cohort (close commit 3351153 — opened `DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD`); P_WALK pathfinder port (HEAD `45f34867`).

**Status:** drafted 2026-05-20.

## 1. Goal

Retire three related deferrals by porting the TS `Player.getCombatLevel()` formula and wiring it into the three call sites that TS already calls it from:

- `DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD` — `*Player.SetStat` ignores combat-level recompute (player_script.go:678-681).
- `NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD` — `LoadAccount` leaves `p.combatLevel` at the constructor default of `3` (player_load.go:237-242).
- AddXP comment "Does NOT recompute combat level (future combat sub-spec)" — informal English deferral on the level-up branch (player_script.go:758-760).

After this slice, `p.combatLevel` is computed correctly at load, and re-computed on every stat-mutating path that TS recomputes it on.

## 2. Why now

Two production read sites are silently consuming the stale value:

1. **Appearance packet** (`appearance.go:101`) — every player anyone sees through the appearance buffer has their combat level encoded into the packet body. Today every player advertises CL=3, regardless of actual stats.
2. **NPC hunt "too strong to attack" gate** (`npc_hunt.go:172-179`) — `p.combatLevel > n.typ.VisLevel*2` skip. Today every player passes the gate as if CL=3.

Both bugs auto-fix the moment `p.combatLevel` becomes live; no separate fix is needed at either read site.

Additionally, the script + ActivePlayer/Npc interface layer is otherwise end-to-end complete (post-P_WALK port; world-module gap audit returned zero stubs). This is the largest remaining single-call-site behavioral gap that lives entirely within a small, well-bounded surface.

## 3. TS reference

### 3.1 Formula (`Engine-TS/src/engine/entity/Player.ts:1302-1308`)

```ts
getCombatLevel() {
    const base  = 0.25  * (this.baseLevels[PlayerStat.DEFENCE] + this.baseLevels[PlayerStat.HITPOINTS] + Math.floor(this.baseLevels[PlayerStat.PRAYER] / 2));
    const melee = 0.325 * (this.baseLevels[PlayerStat.ATTACK] + this.baseLevels[PlayerStat.STRENGTH]);
    const range = 0.325 * (Math.floor(this.baseLevels[PlayerStat.RANGED] / 2) + this.baseLevels[PlayerStat.RANGED]);
    const magic = 0.325 * (Math.floor(this.baseLevels[PlayerStat.MAGIC]  / 2) + this.baseLevels[PlayerStat.MAGIC]);
    return Math.floor(base + Math.max(melee, range, magic));
}
```

Uses `baseLevels[]` (not `levels[]`) — drinking a strength potion does NOT raise combat level.

### 3.2 Call sites in TS

All three use the same guarded-rebuild pattern, except the load site (which has no client to inform):

| TS site                                  | Code shape                                                                                                                                                                       |
| ---                                      | ---                                                                                                                                                                              |
| `Player.setLevel` (Player.ts:1830-1833)  | `if (this.combatLevel != this.getCombatLevel()) { this.combatLevel = this.getCombatLevel(); this.buildAppearance(this.appearanceInv); }`                                         |
| `Player.advanceStat` (Player.ts:1810-1813) | Identical to setLevel.                                                                                                                                                          |
| `PlayerLoading.ts:156`                  | `player.combatLevel = player.getCombatLevel();`  (unconditional, no buildAppearance — load happens before first appearance generation).                                          |

`buildAppearance(inv)` (Player.ts:1836-1839) is a literal two-liner: `this.appearanceInv = inv; this.masks |= PlayerInfoProt.APPEARANCE;` — already mirrored in goscape as `(p *Player) SetAppearanceInv(id int)` (player_script.go:827).

## 4. Goscape baseline

| Piece                                 | Where                                                | State                                              |
| ---                                   | ---                                                  | ---                                                |
| `p.combatLevel int` field             | player.go:210                                        | Exists. Init to 3 at construction (player.go:537). |
| Appearance read site                  | appearance.go:101 — `buf.P1(uint8(p.combatLevel))`   | Live; reads the field on every appearance regen.   |
| NPC-hunt read site                    | npc_hunt.go:172-179 — `p.combatLevel > n.typ.VisLevel*2` | Live; reads the field on every hunt cycle.     |
| `SetAppearanceInv(id)`                | player_script.go:827-830                             | Goscape equivalent of TS `buildAppearance`.        |
| Stat slot constants                   | `pkg/objtype/playerstat.go` (`PlayerStatAttack`…`PlayerStatMagic`) | All seven combat stats defined.    |
| `*Player.SetStat`                     | player_script.go:682-695                             | Sets baseLevels/levels/stats; no CL recompute.     |
| `*Player.AddXP` level-up branch       | player_script.go:786-791                             | Fires changeStat + advanceStat triggers; no CL.    |
| `LoadAccount`                         | player_load.go:243 (returns nil)                     | Loads baseLevels from SAV; CL stays at default 3.  |
| TS `getCombatLevel` body              | n/a                                                  | Not ported anywhere.                               |

Net gap: one new method (with a pure formula helper), three hook sites.

## 5. Design

### 5.1 New methods (in `modules/world/player_script.go`, near `SetStat`)

```go
// calcCombatLevel ports TS Player.getCombatLevel (Player.ts:1302-1308).
// Uses baseLevels (NOT levels) — buffs/drains don't move combat level.
// Result is bounded by the formula: at level-99 across all combat stats,
// CL = 126; at fresh-player stats, CL = 3.
func (p *Player) calcCombatLevel() int {
    def    := int(p.baseLevels[objtype.PlayerStatDefence])
    hp     := int(p.baseLevels[objtype.PlayerStatHitpoints])
    prayer := int(p.baseLevels[objtype.PlayerStatPrayer])
    att    := int(p.baseLevels[objtype.PlayerStatAttack])
    str    := int(p.baseLevels[objtype.PlayerStatStrength])
    rng    := int(p.baseLevels[objtype.PlayerStatRanged])
    mag    := int(p.baseLevels[objtype.PlayerStatMagic])

    base  := 0.25  * float64(def + hp + prayer/2)
    melee := 0.325 * float64(att + str)
    rangd := 0.325 * float64(rng/2 + rng)
    magic := 0.325 * float64(mag/2 + mag)

    return int(math.Floor(base + math.Max(melee, math.Max(rangd, magic))))
}

// recomputeCombatLevel updates p.combatLevel if the formula now yields a
// different value. When triggerRebuild is true, also flips MaskAppearance
// (via SetAppearanceInv) so the next encodeOut emits a fresh appearance.
// SetStat and AddXP pass true; player_load passes false (no client yet).
//
// Mirrors the guarded-rebuild blocks at TS Player.ts:1810-1813 and
// 1830-1833; the load-time variant matches PlayerLoading.ts:156.
func (p *Player) recomputeCombatLevel(triggerRebuild bool) {
    newCL := p.calcCombatLevel()
    if newCL == p.combatLevel {
        return
    }
    p.combatLevel = newCL
    if triggerRebuild {
        p.SetAppearanceInv(p.appearanceInv)
    }
}
```

Notes:
- `prayer/2`, `rng/2`, `mag/2` are integer divisions on `int` — Go integer division on non-negative operands floors exactly like `Math.floor`. The TS-side `Math.floor` calls are redundant on integer inputs but harmless. We rely on the same property.
- `math.Floor` on the final `float64` matches the outer TS `Math.floor`.
- The intermediate float arithmetic is single-precision-safe for the input range (max baseLevel = 99 ⇒ max float intermediate ≪ 2^24).

### 5.2 Three hook sites

1. **`*Player.SetStat`** (player_script.go:682) — append `p.recomputeCombatLevel(true)` as the last statement. Remove the `DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD` paragraph from the doc-block; replace with a one-line cross-ref to `recomputeCombatLevel`.

2. **`*Player.AddXP` level-up branch** (player_script.go:786-791) — append `p.recomputeCombatLevel(true)` after the `advanceStat(id)` call, inside the `afterBase > beforeBase` block. Update the doc-comment to remove "Does NOT recompute combat level (future combat sub-spec)"; replace with a cross-ref.

3. **`player_load.go` LoadAccount** (around line 243) — append `p.recomputeCombatLevel(false)` before the final `return nil`. Replace the `NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD` comment with a one-liner pointing at the call.

### 5.3 Tests

#### Unit — formula (`player_script_test.go`, new test block)

**Baseline convention for these tests:** "fresh stats" means `baseLevels[]` matches what `LoadAccount` writes for a brand-new account (player_load.go:79-85): all stats = 1, except **HP = 10**. Tests set `baseLevels[]` directly on a test-fixture `*Player`; they do not exercise the `LoadAccount` path.

| Test | Input (baseLevels) | Expected CL | Arithmetic |
| --- | --- | --- | --- |
| `TestCalcCombatLevel_FreshStats` | all=1, hp=10 | **3** | base = 0.25·(1+10+0) = 2.75; melee = 0.325·2 = 0.65; range = 0.325·1 = 0.325; magic = 0.325·1 = 0.325; max = 0.65; floor(2.75+0.65) = **3**. |
| `TestCalcCombatLevel_Maxed` | all=99 | **126** | base = 0.25·(99+99+49) = 61.75; melee = 0.325·198 = 64.35; range = magic = 0.325·148 = 48.1; max = 64.35; floor(61.75+64.35) = **126**. |
| `TestCalcCombatLevel_PureMelee99` | att=str=99; def=prayer=range=mage=1; hp=10 | **67** | base = 2.75; melee = 64.35; range = magic ≈ 0.325; max = 64.35; floor(2.75+64.35) = **67**. |
| `TestCalcCombatLevel_PureRanged99` | range=99; rest fresh (att=str=def=prayer=mage=1, hp=10) | **50** | base = 2.75; melee ≈ 0.65; range = 0.325·(49+99) = 48.1; magic ≈ 0.325; max = 48.1; floor(2.75+48.1) = **50**. |
| `TestCalcCombatLevel_PureMagic99` | mage=99; rest fresh | **50** | symmetric to ranged. |
| `TestCalcCombatLevel_PrayerLeveraged` | def=hp=prayer=99; att=str=range=mage=1 | **62** | base = 0.25·(99+99+49) = 61.75; melee = 0.65; range/magic ≈ 0.325; max = 0.65; floor(61.75+0.65) = **62**. |
| `TestCalcCombatLevel_UsesBaseLevelsNotLevels` | baseLevels: all=1, hp=10; **levels[STR]=99** (boosted) | **3** | Critical regression guard: drinking a strength potion does NOT change combat level. |

Each row's "Arithmetic" column is the spec-side hand-computation. The implementer should not re-derive these; they should treat the table as ground-truth and only flag a row if Go arithmetic produces a different value (which would indicate a porting bug).

#### Unit — recompute method

| Test | Setup | Action | Expected |
| --- | --- | --- | --- |
| `TestRecomputeCombatLevel_NoChange_NoMaskFlip` | fresh player (CL=3); pre-set `p.combatLevel=3` | call with `triggerRebuild=true` | `p.masks & MaskAppearance == 0` (early-return on guard) |
| `TestRecomputeCombatLevel_Change_RebuildTrue_FlipsMask` | fresh player; bump `baseLevels[STRENGTH]=99` | call with `triggerRebuild=true` | `p.combatLevel` updated; `p.masks & MaskAppearance != 0`; `p.appearanceInv` unchanged |
| `TestRecomputeCombatLevel_Change_RebuildFalse_NoMaskFlip` | fresh player; bump `baseLevels[STRENGTH]=99` | call with `triggerRebuild=false` | `p.combatLevel` updated; `p.masks & MaskAppearance == 0` |

#### Integration — hook sites

| Test | Setup | Action | Expected |
| --- | --- | --- | --- |
| `TestSetStat_RecomputesCombatLevelAndFlipsAppearance` | fresh player (CL=3) | `SetStat(PlayerStatStrength, 99)` | `p.combatLevel` > 3; `p.masks & MaskAppearance != 0` |
| `TestSetStat_NoChange_NoMaskFlip` | fresh player (CL=3) | `SetStat(PlayerStatCooking, 50)` (non-combat stat) | `p.combatLevel == 3`; `p.masks & MaskAppearance == 0` |
| `TestAddXP_LevelUp_RecomputesCombatLevel` | fresh player; baseLevels[STR]=1 | `AddXP(PlayerStatStrength, enough_to_reach_99)` | `p.combatLevel` reflects the new CL; `p.masks & MaskAppearance != 0` |
| `TestAddXP_NoLevelUp_NoRecompute` | fresh player; baseLevels[STR]=98 with enough XP for 98 but not 99 | `AddXP(PlayerStatStrength, 1)` (no level-up) | `p.combatLevel` unchanged; mask not flipped |
| `TestLoadAccount_PopulatesCombatLevel` | SAV bytes with non-fresh baseLevels (e.g. all=50) | run LoadAccount | `p.combatLevel == calcCombatLevel()` for the loaded baseLevels; `p.masks & MaskAppearance == 0` (no rebuild on load) |

### 5.4 Tag retirements

Three retirements in this slice. The retirement scheme follows the established goscape pattern (remove the deviation block; add a one-line "Retires DEVIATION-..." note in the commit message; do NOT edit point-in-time spec docs under `docs/superpowers/`).

| Tag | Site | Action |
| --- | --- | --- |
| `DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD` | player_script.go:678-681 (SetStat doc-block) | Delete the 4-line deviation paragraph. Replace with one-line cross-ref to recomputeCombatLevel. |
| `NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD` | player_load.go:237-242 | Delete the 6-line deferral comment. Replace with a one-liner ("Recomputes combat level from loaded baseLevels — see recomputeCombatLevel."). |
| AddXP "Does NOT recompute combat level (future combat sub-spec)" | player_script.go:758-760 (informal, no tag) | Delete the deferral sentence. The remaining doc-block already explains the level-up branches faithfully. |

No new deviation tags opened.

## 6. Spec → plan deviations (pre-acknowledged)

None expected. The design is a direct port of three TS call sites that already share a guarded-rebuild idiom; the goscape `recomputeCombatLevel(triggerRebuild bool)` parameterization is the only structural choice and is explicit in the design above.

## 7. Out of scope (explicit non-goals)

- Combat AI, weapon interactions, attack/defence rolls, hit calcs — the broader "future combat sub-spec".
- Refactoring or relocating `appearance.go` encoding — `p.combatLevel` is already read at line 101; the field just needs to be live.
- `npc_hunt.go:172-179` "too strong" gate code change — the read site already exists; once the field is correct, the gate self-corrects. **No code change at the npc-hunt read site; no test added there** (user declined the optional "All three + npc_hunt fix" variant during brainstorm).
- Changing `newPlayer`'s init `combatLevel: 3` — the formula confirms 3 for fresh stats, so the init is load-bearing for the no-mutation path (the `TestCalcCombatLevel_FreshPlayer` test pins this). The first `recomputeCombatLevel` either confirms 3 (early-return) or updates it.
- `*Npc` combat-level recompute — NPCs in TS don't have a getCombatLevel; their combat level comes from NpcType config.
- Session-log / milestone events on level-up (TS Player.ts:1773-1803) — separately deferred in the AddXP doc-block as "session-log infrastructure not yet ported"; not retired by this slice.

## 8. Gate plan

After all hooks land and tests pass:

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` — 57+ pkgs, 0 FAIL.
- Smoke-pack 12 OK / 0 ERR.
- Confirm no `DEVIATION-NAI-184-D1` or `NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD` references remain in non-test Go (greps must return empty outside `docs/superpowers/`).

## 9. Memory pointers

- Predecessor close: `[[handlepwalk-pathfinder-port-close]]`.
- Pre-decessor cluster: NAI-184 cheat cohort (close commit `3351153`).
- Will retire: `DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD`, `NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD`, informal AddXP combat-level deferral.
- Net effect on `~145 live NAI-XXX-D-*` board: −2 formal pins.
