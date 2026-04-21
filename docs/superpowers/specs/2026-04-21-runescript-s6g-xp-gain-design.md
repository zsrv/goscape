# Sub-spec RuneScript S6g: XP Gain + Level-Up Math (TS-Faithful) — Design

**Status:** Draft → ready for plan
**Scope:** Port TS `Player.getLevelByExp` (Player.ts:87-95) to `pkg/objtype/playerstat.go` alongside S6f's `GetExpByLevel`. Export `MaxSkillXP = 130344310` (the level-99 XP cap). Replace `Player.AddXP`'s body to match TS's advance-stat math (Player.ts:1758-1772): clamp at XP cap, recompute `baseLevels[id]` from `GetLevelByExp(stats[id])`, replenish drained `levels[id]` by the level delta on level-up. Closes the explicit TODO at `modules/world/player_script.go:187`.
**Out of scope:** Firing `ChangeStat` triggers (script dispatcher exists via `GetByTrigger(TriggerChangeStat=165, ...)` but no cache-script consumer yet — wiring deferred). Combat-level recomputation. Save-file load. Boost / drain ops (`Player.boostStat`, `Player.drainStat`). XP gain from real player actions (skill sub-specs own those).

---

## Rationale

S6f shipped default-player skill init + the `GetExpByLevel` helper. `Player.AddXP` still has an explicit TODO at `player_script.go:187`: "recompute baseLevels from getLevelByExp table and clamp at XP cap." Today `AddXP` does `p.stats[id] += int32(xp)` — no cap, no level recomputation, no drain replenish. Any script or future combat-code call to `AddXP` leaves `baseLevels` stale.

TS `Player.advanceStat` (Player.ts:1758-1772) is the canonical advance-stat math:

```typescript
const before = this.baseLevels[stat];
this.stats[stat] = Math.min(this.stats[stat] + exp, maxXp);
this.baseLevels[stat] = getLevelByExp(this.stats[stat]);
if (this.baseLevels[stat] > before) {
    if (this.levels[stat] < before) {
        // replenish stat
        this.levels[stat] += this.baseLevels[stat] - before;
    }
    this.changeStat(stat);
}
```

S6g ports the math (cap clamp, base recompute, drain replenish). The `changeStat` trigger fire is deferred — no cache-script consumer exists today.

## Architecture

```
pkg/objtype/
├── playerstat.go               (modify) — add GetLevelByExp + MaxSkillXP const
└── playerstat_test.go          (modify) — add GetLevelByExp + MaxSkillXP tests

modules/world/
├── player_script.go            (modify) — rewrite Player.AddXP body; delete TODO
└── player_script_test.go       (NEW)    — 7 AddXP tests covering all edge cases
```

Total **~280 LOC** (production + tests).

## Components

### 1. `pkg/objtype/playerstat.go` — add `GetLevelByExp` + `MaxSkillXP`

Append after the existing `GetExpByLevel`:

```go
// MaxSkillXP is the XP threshold to reach level 99 (the game's max level).
// Equal to levelExperience[98] = GetExpByLevel(99) = 130344310. XP is
// stored as fixed-point tenths (×10), so this represents 13,034,431 "real"
// XP — the canonical RS2 level-99 XP value.
const MaxSkillXP = 130344310

// GetLevelByExp returns the highest level whose XP threshold is <= xp, or 1
// if xp is below any threshold. Clamped at level 99. Matches TS
// Player.getLevelByExp (Player.ts:87-95). xp is the fixed-point tenths
// value (scaled ×10), consistent with GetExpByLevel.
//
// Negative xp returns 1 (defensive — no threshold is negative, so the loop
// falls through to the `return 1` tail).
func GetLevelByExp(xp int) int {
	for i := 98; i >= 0; i-- {
		if xp >= levelExperience[i] {
			level := i + 2
			if level > 99 {
				level = 99
			}
			return level
		}
	}
	return 1
}
```

The `level > 99` guard matches TS's `Math.min(i + 2, 99)` — when `i == 98`, raw `i + 2 == 100` must clamp to 99.

### 2. `pkg/objtype/playerstat_test.go` — extend with new tests

Append:

```go
func TestGetLevelByExpKnownValues(t *testing.T) {
	cases := []struct {
		xp, want int
	}{
		{0, 1},          // below any threshold
		{82, 1},         // just below level-2 (830) threshold
		{830, 2},        // exactly at level-2 threshold
		{831, 2},        // just above
		{11539, 9},      // just below level-10
		{11540, 10},     // exactly at level-10
		{1013329, 49},   // just below level-50
		{1013330, 50},   // exactly at level-50
		{130344309, 98}, // just below level-99
		{130344310, 99}, // exactly at level-99 (cap)
		{999999999, 99}, // way above cap → still 99
	}
	for _, tc := range cases {
		if got := GetLevelByExp(tc.xp); got != tc.want {
			t.Errorf("GetLevelByExp(%d): got %d, want %d", tc.xp, got, tc.want)
		}
	}
}

func TestGetLevelByExpNegativeClampsToOne(t *testing.T) {
	for _, xp := range []int{-1, -100, -999999} {
		if got := GetLevelByExp(xp); got != 1 {
			t.Errorf("GetLevelByExp(%d): got %d, want 1", xp, got)
		}
	}
}

func TestGetLevelByExpInverseOfGetExpByLevel(t *testing.T) {
	// Round-trip: for every valid level, GetLevelByExp(GetExpByLevel(level)) == level.
	for level := 2; level <= 99; level++ {
		xp := GetExpByLevel(level)
		if got := GetLevelByExp(xp); got != level {
			t.Errorf("roundtrip level=%d xp=%d GetLevelByExp=%d", level, xp, got)
		}
	}
}

func TestMaxSkillXP(t *testing.T) {
	if MaxSkillXP != 130344310 {
		t.Errorf("MaxSkillXP: got %d, want 130344310", MaxSkillXP)
	}
	if MaxSkillXP != GetExpByLevel(99) {
		t.Errorf("MaxSkillXP (%d) must equal GetExpByLevel(99) (%d)", MaxSkillXP, GetExpByLevel(99))
	}
}
```

### 3. `modules/world/player_script.go` — rewrite `Player.AddXP`

Find the method at lines 185-193 (includes the TODO comment). Replace the entire comment + function:

```go
// AddXP adds xp (scaled ×10) to the player's stored XP for skill id and
// recomputes baseLevels from the XP curve. On level-up (baseLevels
// increases), if levels[id] was drained below the previous base, replenish
// by the level delta — matches TS Player.advanceStat (Player.ts:1758-1772).
// XP is clamped at objtype.MaxSkillXP (level-99 cap). OOB id drops silently.
//
// Negative xp is clamped to keep stats[id] >= 0 defensively — deviation
// from TS where a bug could reduce stored XP. Matches the convention from
// Player.Damage / *Npc.Damage negative-amount clamps.
//
// Does NOT fire the ChangeStat trigger (S-future sub-spec — no cache-script
// consumer yet). Does NOT recompute combat level (future combat sub-spec).
func (p *Player) AddXP(id int, xp int) {
	if !statBounds(id) {
		return
	}
	next := int64(p.stats[id]) + int64(xp)
	if next > int64(objtype.MaxSkillXP) {
		next = int64(objtype.MaxSkillXP)
	}
	if next < 0 {
		next = 0
	}
	beforeBase := int(p.baseLevels[id])
	p.stats[id] = int32(next)
	p.baseLevels[id] = uint8(objtype.GetLevelByExp(int(p.stats[id])))
	afterBase := int(p.baseLevels[id])

	if afterBase > beforeBase && int(p.levels[id]) < beforeBase {
		// Level-up while drained: replenish by the level delta.
		// Matches TS Player.ts:1767-1770.
		newLevel := int(p.levels[id]) + (afterBase - beforeBase)
		if newLevel > 255 {
			newLevel = 255
		}
		p.levels[id] = uint8(newLevel)
	}
}
```

`int64` intermediate prevents overflow: `stats[id]` is `int32` (max ~2.1B); `xp` caller-controlled. Naive `int32(xp) + int32(p.stats[id])` could overflow. Compute in int64, clamp, cast.

The combined `if afterBase > beforeBase && levels[id] < beforeBase` short-circuits cleanly — no replenish if level didn't go up OR if player wasn't drained.

Import `objtype` if not already present in `player_script.go` (it's likely imported already from S6f).

### 4. `modules/world/player_script_test.go` — new test file

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestAddXPNormalGainNoLevelUp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 200
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2
	p.AddXP(objtype.PlayerStatAttack, 100)
	if p.stats[objtype.PlayerStatAttack] != 300 {
		t.Errorf("stats: got %d, want 300", p.stats[objtype.PlayerStatAttack])
	}
	if p.baseLevels[objtype.PlayerStatAttack] != 2 {
		t.Errorf("baseLevels: got %d, want 2", p.baseLevels[objtype.PlayerStatAttack])
	}
	if p.levels[objtype.PlayerStatAttack] != 2 {
		t.Errorf("levels: got %d, want 2 (no replenish)", p.levels[objtype.PlayerStatAttack])
	}
}

func TestAddXPLevelUpNotDrained(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2
	p.AddXP(objtype.PlayerStatAttack, 1000) // → 1800, crosses 1740 = level 3
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3", p.baseLevels[objtype.PlayerStatAttack])
	}
	if p.levels[objtype.PlayerStatAttack] != 2 {
		t.Errorf("levels: got %d, want 2 (not drained, no replenish)", p.levels[objtype.PlayerStatAttack])
	}
}

func TestAddXPLevelUpWhileDrained(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 1 // drained below base
	p.AddXP(objtype.PlayerStatAttack, 1000) // → level 3
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3", p.baseLevels[objtype.PlayerStatAttack])
	}
	// Replenish: levels + (afterBase - beforeBase) = 1 + (3 - 2) = 2.
	if p.levels[objtype.PlayerStatAttack] != 2 {
		t.Errorf("levels: got %d, want 2 (drained, replenished by 1)", p.levels[objtype.PlayerStatAttack])
	}
}

func TestAddXPMultiLevelUpNotDrained(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 0
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1
	p.AddXP(objtype.PlayerStatAttack, 11540) // GetExpByLevel(10)
	if p.baseLevels[objtype.PlayerStatAttack] != 10 {
		t.Errorf("baseLevels: got %d, want 10", p.baseLevels[objtype.PlayerStatAttack])
	}
	// levels[Attack] == beforeBase (1), NOT less than — so no replenish.
	if p.levels[objtype.PlayerStatAttack] != 1 {
		t.Errorf("levels: got %d, want 1 (equal, not less — no replenish)", p.levels[objtype.PlayerStatAttack])
	}
}

func TestAddXPClampsAtCap(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = int32(objtype.MaxSkillXP - 10)
	p.baseLevels[objtype.PlayerStatAttack] = 99
	p.levels[objtype.PlayerStatAttack] = 99
	p.AddXP(objtype.PlayerStatAttack, 1000)
	if int(p.stats[objtype.PlayerStatAttack]) != objtype.MaxSkillXP {
		t.Errorf("stats: got %d, want MaxSkillXP %d",
			p.stats[objtype.PlayerStatAttack], objtype.MaxSkillXP)
	}
	if p.baseLevels[objtype.PlayerStatAttack] != 99 {
		t.Errorf("baseLevels: got %d, want 99 (capped)", p.baseLevels[objtype.PlayerStatAttack])
	}
}

func TestAddXPNegativeClampsAtZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 50
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1
	p.AddXP(objtype.PlayerStatAttack, -100) // would underflow to negative
	if p.stats[objtype.PlayerStatAttack] != 0 {
		t.Errorf("stats: got %d, want 0 (negative clamped)", p.stats[objtype.PlayerStatAttack])
	}
	if p.baseLevels[objtype.PlayerStatAttack] != 1 {
		t.Errorf("baseLevels: got %d, want 1 (from 0 XP)", p.baseLevels[objtype.PlayerStatAttack])
	}
}

func TestAddXPOOBIsNoop(t *testing.T) {
	p, _ := newTestPlayer(t)
	var before [21]int32
	copy(before[:], p.stats[:])
	p.AddXP(-1, 100)
	p.AddXP(21, 100)
	p.AddXP(100, 100)
	for i := 0; i < 21; i++ {
		if p.stats[i] != before[i] {
			t.Errorf("OOB AddXP mutated stats[%d]: got %d, want %d", i, p.stats[i], before[i])
		}
	}
}
```

## Data flow (two-level-up while drained)

1. Player before: `stats[Attack]=0, baseLevels[Attack]=1, levels[Attack]=1`.
2. Combat script (future) deals damage; player is poisoned, `levels[Attack]` drained to 0 via separate drain op. Now: `levels[Attack]=0, baseLevels[Attack]=1`.
3. Meanwhile, player gains 1800 XP from an attack action. `AddXP(PlayerStatAttack, 1800)` called.
4. In `AddXP`: `beforeBase = 1`. `next = 0 + 1800 = 1800`. Not capped. Not negative.
5. `stats[Attack] = 1800`. `baseLevels[Attack] = GetLevelByExp(1800) = 3` (crossed 830 → 2, then 1740 → 3).
6. `afterBase = 3 > beforeBase = 1` AND `levels[Attack] = 0 < beforeBase = 1`.
7. Replenish: `newLevel = 0 + (3 - 1) = 2`. `levels[Attack] = 2`.
8. Next tick, `updateStats()` differ sees `stats[Attack]` and `levels[Attack]` both changed — fires `UpdateStat(Attack, 1800, 2)` wire op.
9. Client shows level 2 Attack with 1800 XP.

## Edge cases

| # | Case | Handling |
|---|---|---|
| 1 | Normal XP gain, no level-up | stats updated; baseLevels/levels unchanged. |
| 2 | Level-up, not drained (`levels == beforeBase`) | baseLevels updated; levels stays (strict `<` test). |
| 3 | Level-up while drained | baseLevels updated; levels replenished by (afterBase − beforeBase). |
| 4 | XP cap | int64 intermediate clamps at MaxSkillXP; baseLevels stays ≤ 99. |
| 5 | Multi-level-up in one call | GetLevelByExp naturally returns final level; replenish is total delta. |
| 6 | Negative XP | int64 intermediate clamps at 0. baseLevels recomputes from clamped XP. |
| 7 | OOB id | statBounds early return. |
| 8 | Level-up past 255 on replenished `levels` | `newLevel > 255` clamped. Practically unreachable (level-up deltas are small; levels are 1..99) but defensive. |
| 9 | `afterBase == beforeBase` with levels drained | No replenish — the guard requires `afterBase > beforeBase`. Pure XP gain without crossing a boundary. |
| 10 | Stats cross multiple level thresholds when levels is drained | Replenish by full delta in one call. Matches TS line 1769. |

## Key design calls

- **int64 intermediate** for XP addition. `stats[id]` is `int32`, `xp` is caller-controlled `int`. Adding directly risks overflow; int64 guarantees safe arithmetic before the cap clamp.
- **Strict `<` for drain test.** TS does `if (this.levels[stat] < before)` — equal means "at full base level," not drained. Our Go port matches exactly. Test #11 in Section 2's matrix verifies this.
- **`MaxSkillXP` exported as package const.** Alternative would be `objtype.levelExperience[98] * 10` — but that's unexported. Exporting the const means combat / skill-action sub-specs can reference the cap without duplicating the magic number.
- **`GetLevelByExp`'s `level > 99` clamp** matches TS's `Math.min(i + 2, 99)`. Without it, raw `i+2` at `i == 98` would return 100 — a real bug, not just a stylistic quibble.
- **Negative-XP defensive clamp.** Consistent with S6c `*Npc.Damage` and S6e `Player.Damage` negative-amount handling. TS wouldn't clamp (XP would just decrement); we break from TS for bug-safety.
- **No ChangeStat trigger fire.** The TS path has `this.changeStat(stat)` after replenish. Our scope defers this because no cache-script consumer exists today. When that consumer arrives, the fire goes immediately after the replenish branch: `if afterBase > beforeBase { ...; fireChangeStat(p, id) }`.
- **`updateStats` differ picks up changes automatically.** The existing `player.go:442` loop diffs `stats[i]` and `levels[i]` against `lastStats[i]` / `lastLevels[i]`. AddXP mutates both (sometimes just stats); the differ fires `sendUpdateStat` on the next tick. No wire work in S6g.

## Gotchas

- **`uint8` cast for levels.** `levels[id]` is `[21]uint8`. Post-replenish clamp at 255 before casting. Practically the clamp never fires (levels are 1..99 and deltas are small), but required for type safety.
- **The `MaxSkillXP` constant is `int`, not `int32`.** Matches `GetLevelByExp`'s int argument. At the clamp site (`next > int64(objtype.MaxSkillXP)`), the int→int64 promotion is explicit.
- **`TestStatAdvanceViaScript` in script_test.go:511 uses `AddXP`** via the `OpStatAdvance` script handler. The test's existing XP values may need updating — the implementer must read the test and verify the new cap/recompute behavior doesn't break the assertions. If the test uses a small XP value (e.g., 150) with explicit post-`p.stats[3]=150` check, it still passes because small XP doesn't trigger any new behavior. If it expected the old no-cap / no-recompute semantics, it needs updating.
- **`objtype` import** in `player_script.go` is almost certainly present already (file uses `objtype.PlayerStatHitpoints` in other methods). If grep confirms absence, add it.
- **Boost / drain sub-spec interaction.** A future `Player.boostStat` would write `levels[id] > baseLevels[id]` (buffed state). AddXP's replenish clause uses `beforeBase`, not `baseLevels[id]`, so a boosted player who levels up doesn't lose the boost on replenish — the replenish branch only fires if `levels < beforeBase` (i.e., drained). Correct TS-faithful behavior; worth a mental note for the boost/drain sub-spec.
