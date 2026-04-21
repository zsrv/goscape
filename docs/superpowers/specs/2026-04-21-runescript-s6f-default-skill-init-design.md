# Sub-spec RuneScript S6f: Default Player Skill Init at Login — Design

**Status:** Draft → ready for plan
**Scope:** Replace the S6e Task 1 ad-hoc Hitpoints-only seed in `processLogins` with the full default-player skill init matching TS `PlayerLoading.ts:41-53`. Adds the remaining 20 `PlayerStat*` constants in a new `pkg/objtype/playerstat.go` file (moves `PlayerStatHitpoints` there from `npctype.go`). Adds a `GetExpByLevel(level int) int` helper using the canonical RuneScape XP curve, ×10 scaling matching TS `levelExperience` (Player.ts:77-85). Closes the `stats[Hitpoints]=0` TS-infidelity flagged by the S6e final review.
**Out of scope:** Save-file load + restore (the TS `PlayerLoading.ts.load()` post-line-53 path; future sub-spec). Combat formulas. Skill-advancement (XP gain on action). Skill restore on tick (idle regen). The `OpDamage` script handler. `Npc.ShowHit` retirement.

---

## Rationale

S6e Task 1 added a deliberate stop-gap in `processLogins`: seed `levels[Hitpoints]=10` and `baseLevels[Hitpoints]=10`, leaving the other 20 skills at zero. The S6e final review (M5) flagged this as the next TS-infidelity gap: a level-10 Hitpoints player with `stats[3]=0` represents a state impossible through normal XP grinding. The wire byte sequence is technically valid, but any combat or skill-advance code reading XP would see a mismatch.

TS `PlayerLoading.ts:41-53` is the canonical "no save file" path:

```typescript
if (sav.data.length < 2) {
    for (let i = 0; i < 21; i++) {
        player.stats[i] = 0;
        player.baseLevels[i] = 1;
        player.levels[i] = 1;
    }
    // hitpoints starts at level 10
    player.stats[PlayerStat.HITPOINTS] = getExpByLevel(10);
    player.baseLevels[PlayerStat.HITPOINTS] = 10;
    player.levels[PlayerStat.HITPOINTS] = 10;
    return player;
}
```

S6f ports this faithfully and supersedes the S6e Task 1 stop-gap. When future save-restore sub-spec lands, this default-init path becomes the no-save fallback (mirroring the TS `if` branch).

The work also lands the deferred PlayerStat* constant set (S6e final-review M2 and S6e spec §Key design calls explicitly noted "wait until first consumer arrives — that consumer is the loop"). And it adds `GetExpByLevel`, which combat / skill-advance sub-specs will need.

## Architecture

```
pkg/objtype/
├── npctype.go                  (modify) — DELETE PlayerStatHitpoints (moves to playerstat.go).
│                                            NpcStat* untouched.
├── playerstat.go               (NEW)    — All 21 PlayerStat* constants + PlayerStatCount +
│                                            levelExperience table (init()) + GetExpByLevel
└── playerstat_test.go          (NEW)    — XP curve correctness tests + boundary clamps

modules/world/
├── tick.go                     (modify) — replace S6e Task 1 ad-hoc Hitpoints seed with full
│                                            default-player init (loop 21 + Hitpoints override)
└── tick_logins_test.go         (modify) — replace TestProcessLoginsSeedsHitpoints with broader
                                            TestProcessLoginsSeedsAllSkillsToDefaults
```

Total **~155 LOC**.

## Components

### 1. `pkg/objtype/playerstat.go` — new file

Owns all player-skill stat indices and the XP curve helper.

```go
package objtype

import "math"

// PlayerStat* are indices into Player.levels, Player.baseLevels, and
// Player.stats[XP] for player-skill slots. Index values match TS PlayerStat
// enum (PlayerStat.ts).
const (
	PlayerStatAttack      = 0
	PlayerStatDefence     = 1
	PlayerStatStrength    = 2
	PlayerStatHitpoints   = 3
	PlayerStatRanged      = 4
	PlayerStatPrayer      = 5
	PlayerStatMagic       = 6
	PlayerStatCooking     = 7
	PlayerStatWoodcutting = 8
	PlayerStatFletching   = 9
	PlayerStatFishing     = 10
	PlayerStatFiremaking  = 11
	PlayerStatCrafting    = 12
	PlayerStatSmithing    = 13
	PlayerStatMining      = 14
	PlayerStatHerblore    = 15
	PlayerStatAgility     = 16
	PlayerStatThieving    = 17
	PlayerStat18          = 18 // unused in RS2-225 era; kept for index parity with TS
	PlayerStat19          = 19 // unused in RS2-225 era; kept for index parity with TS
	PlayerStatRunecraft   = 20

	PlayerStatCount = 21
)

// levelExperience holds the XP threshold to reach level (i+2) at index i.
// Built once at package init from the canonical RS XP formula. Matches TS
// levelExperience (Player.ts:77-85). XP is stored as fixed-point tenths
// (×10) so increments can be fractional (e.g. 0.1 XP from a half-cooked food).
var levelExperience [99]int

func init() {
	acc := 0
	for i := 0; i < 99; i++ {
		level := i + 1
		delta := int(math.Floor(float64(level) + math.Pow(2.0, float64(level)/7.0)*300.0))
		acc += delta
		levelExperience[i] = (acc / 4) * 10
	}
}

// GetExpByLevel returns the XP threshold required to reach `level`. Matches
// TS Player.getExpByLevel (Player.ts:97-99).
//
// Boundary handling diverges from TS for safety:
//   - level <= 1 returns 0 (TS returns undefined → NaN-cascade)
//   - level > 99 clamps to level-99 XP (TS returns undefined)
//
// These defensive clamps match the same convention as Player.Damage (S6e)
// and *Npc.Damage (S6c) negative-amount clamps.
func GetExpByLevel(level int) int {
	if level <= 1 {
		return 0
	}
	if level > 99 {
		level = 99
	}
	return levelExperience[level-2]
}
```

### 2. `pkg/objtype/npctype.go` — delete moved constant

Find the `PlayerStat*` block added in S6e Task 1 (the single-entry block immediately after the `NpcStat*` block):

```go
// PlayerStat* are indices into Player.levels and Player.baseLevels for
// player-skill slots. Only Hitpoints is exported here; other stats
// (Attack, Defence, Strength, Ranged, Prayer, Magic, Cooking, ...) get
// added as their first consumer ships. Index values match TS PlayerStat
// enum (PlayerStat.ts) — Hitpoints is 3, sharing the slot with
// NpcStatHitpoints since both represent the same skill index.
const (
	PlayerStatHitpoints = 3
)
```

DELETE this entire block — `PlayerStatHitpoints` now lives in `playerstat.go` alongside its 20 siblings.

The `NpcStat*` block above it stays untouched.

### 3. `modules/world/tick.go` — replace ad-hoc seed with full init

Find the S6e Task 1 seeding block in `processLogins` (placed between `p.invs = ...` and `p.masks |= MaskAppearance`):

```go
		// Seed Hitpoints to 10 (RS2 default starting HP) before any code
		// reads p.levels[PlayerStatHitpoints]. Matches TS PlayerLoading.ts:49-51.
		// Full skill initialization (all 21 skills with persisted XP) is a
		// future sub-spec; S6e covers Hitpoints only because the persistent-HP
		// design requires it.
		p.baseLevels[objtype.PlayerStatHitpoints] = 10
		p.levels[objtype.PlayerStatHitpoints] = 10
```

Replace with the full default-player init (matches TS PlayerLoading.ts:41-53 — the "no save data" branch):

```go
		// Default-player skill init — 21 skills at level 1 with 0 XP, then
		// Hitpoints overridden to level 10 with the matching XP. Matches TS
		// PlayerLoading.ts:41-53 (the "no save data" branch). Save-file load
		// + restore is a future sub-spec; this default becomes the no-save
		// fallback when that lands.
		for i := 0; i < objtype.PlayerStatCount; i++ {
			p.stats[i] = 0
			p.baseLevels[i] = 1
			p.levels[i] = 1
		}
		p.stats[objtype.PlayerStatHitpoints] = int32(objtype.GetExpByLevel(10))
		p.baseLevels[objtype.PlayerStatHitpoints] = 10
		p.levels[objtype.PlayerStatHitpoints] = 10
```

`Player.stats` is `[21]int32`; `GetExpByLevel(10)` returns `int` (= 11540, fits comfortably). The cast is necessary.

### 4. `modules/world/tick_logins_test.go` — broaden the existing test

Replace `TestProcessLoginsSeedsHitpoints` (added in S6e Task 1) with a broader test covering all 21 skills:

```go
func TestProcessLoginsSeedsAllSkillsToDefaults(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	s.newPlayers = []*Player{p}
	s.processLogins()

	// All non-Hitpoints skills: level 1, base level 1, XP 0.
	for i := 0; i < objtype.PlayerStatCount; i++ {
		if i == objtype.PlayerStatHitpoints {
			continue
		}
		if p.levels[i] != 1 {
			t.Errorf("levels[%d]: got %d, want 1", i, p.levels[i])
		}
		if p.baseLevels[i] != 1 {
			t.Errorf("baseLevels[%d]: got %d, want 1", i, p.baseLevels[i])
		}
		if p.stats[i] != 0 {
			t.Errorf("stats[%d]: got %d, want 0", i, p.stats[i])
		}
	}

	// Hitpoints overridden to level 10 with matching XP.
	if p.levels[objtype.PlayerStatHitpoints] != 10 {
		t.Errorf("levels[Hitpoints]: got %d, want 10",
			p.levels[objtype.PlayerStatHitpoints])
	}
	if p.baseLevels[objtype.PlayerStatHitpoints] != 10 {
		t.Errorf("baseLevels[Hitpoints]: got %d, want 10",
			p.baseLevels[objtype.PlayerStatHitpoints])
	}
	if int(p.stats[objtype.PlayerStatHitpoints]) != objtype.GetExpByLevel(10) {
		t.Errorf("stats[Hitpoints]: got %d, want %d (XP for level 10)",
			p.stats[objtype.PlayerStatHitpoints], objtype.GetExpByLevel(10))
	}
}
```

### 5. `pkg/objtype/playerstat_test.go` — new file

```go
package objtype

import "testing"

func TestGetExpByLevelKnownValues(t *testing.T) {
	cases := []struct {
		level, want int
	}{
		{1, 0},          // base case
		{2, 830},        // first table entry: 83 × 10
		{3, 1740},       // 174 × 10
		{10, 11540},     // 1154 × 10 — RS2 canonical level-10 XP
		{50, 1013330},   // 101333 × 10 — mid-curve sanity
		{99, 130344310}, // 13034431 × 10 — top of curve
	}
	for _, tc := range cases {
		if got := GetExpByLevel(tc.level); got != tc.want {
			t.Errorf("GetExpByLevel(%d): got %d, want %d", tc.level, got, tc.want)
		}
	}
}

func TestGetExpByLevelClampsLow(t *testing.T) {
	for _, lvl := range []int{0, -1, -100} {
		if got := GetExpByLevel(lvl); got != 0 {
			t.Errorf("GetExpByLevel(%d): got %d, want 0 (low-clamp)", lvl, got)
		}
	}
}

func TestGetExpByLevelClampsHigh(t *testing.T) {
	want := GetExpByLevel(99)
	for _, lvl := range []int{100, 200, 1000} {
		if got := GetExpByLevel(lvl); got != want {
			t.Errorf("GetExpByLevel(%d): got %d, want %d (clamp to level-99)", lvl, got, want)
		}
	}
}

func TestPlayerStatCount(t *testing.T) {
	if PlayerStatCount != 21 {
		t.Errorf("PlayerStatCount: got %d, want 21 (matches TS PlayerStat enum)", PlayerStatCount)
	}
}

func TestPlayerStatHitpointsIsThree(t *testing.T) {
	if PlayerStatHitpoints != 3 {
		t.Errorf("PlayerStatHitpoints: got %d, want 3", PlayerStatHitpoints)
	}
}
```

## Data flow

**Login (post-S6f):**

1. Client connects, login handshake completes, `Server.processLogins` runs.
2. Per-player loop: `addPlayer`, then default-skill-init runs (the new block).
3. Loop seeds 21 skills: `stats[i]=0, baseLevels[i]=1, levels[i]=1` for i in 0..20.
4. Hitpoints override: `stats[3]=11540, baseLevels[3]=10, levels[3]=10`.
5. `MaskAppearance` flag set; LOGIN trigger fires.
6. LOGIN script can call `STAT(3)` → returns 10 (real, not stale-zero).
7. Tick 1: `updateStats` diff fires `UpdateStat(i, 0/11540, 1/10)` for every skill — client receives initial skills.

## Edge cases

| # | Case | Handling |
|---|---|---|
| 1 | Fresh-login player | All 21 skills at level 1 / 0 XP; Hitpoints override → level 10 / 11540 XP. |
| 2 | `GetExpByLevel(level <= 1)` | Returns 0 (defensive). |
| 3 | `GetExpByLevel(level > 99)` | Clamps to level-99 value (130344310). |
| 4 | Package init ordering | Standard Go init runs before main; no race with tick loop. |
| 5 | Tests using `&Player{}` direct construction | Bypass processLogins — see all-zero arrays. Existing pattern; tests must seed manually as before. |
| 6 | First-tick UpdateStat diff | `lastLevels[i]=0, lastStats[i]=0` (zero-init). New init values differ → all 21 UpdateStat ops fire on tick 1. Client gets initial skill data correctly. No extra work. |
| 7 | Save-data load (future sub-spec) | This default-init path becomes the "no save" fallback. Save-restore overwrites these defaults. |

## Key design calls

- **Move `PlayerStatHitpoints` to a dedicated file.** S6e final-review M2 flagged this as deferred-but-warranted. S6f is the natural moment because the constant gains 20 siblings and the original "next to NpcStat for adjacency" justification weakens once there's a real `PlayerStat*` set.
- **Pre-compute the XP table once via `init()`.** The formula is deterministic and the table is small (99 ints = ~800 bytes). Lazy computation via `sync.Once` would be over-engineered; per-call computation would be wasteful. Standard Go `init()` is the right tool.
- **Defensive clamps on `GetExpByLevel`.** TS returns `undefined` for out-of-range; Go's typed return forces a concrete value. Returning 0 for low-clamp matches "level 1 = 0 XP" semantics; clamping high to level-99 value avoids a panic on cache-script bugs. Same defensive philosophy as our Damage methods.
- **Loop iterates `< PlayerStatCount`, not literal `21`.** Future-proofs against the count changing (it won't, but the constant exists so use it).
- **Replaces (not augments) S6e Task 1's seeding.** The ad-hoc block deletes entirely; the new block lives in the same place. No vestigial code.
- **No new `Player` field changes.** Stats arrays already exist (`stats [21]int32`, `levels [21]uint8`, `baseLevels [21]uint8`). S6f just seeds them properly.

## Gotchas

- **`Player.stats[i]` is `int32`**; `GetExpByLevel` returns `int`. The cast `int32(objtype.GetExpByLevel(10))` is required. Value 11540 is well within int32 range (max ~2.1B; level-99 XP scaled is ~130M).
- **`Player.levels[i]` and `Player.baseLevels[i]` are `uint8`**. Loop assigns literal `1` which compiles fine. Hitpoints override to `10` likewise.
- **Float64 precision in the XP formula** is comfortable for level 1..99. `2^(99/7) ≈ 18043` — well within float64's 15-17 digit precision. Floor truncations dominate any float rounding. Bit-identical to TS.
- **`PlayerStat18` and `PlayerStat19` keep their numeric names** (mirrors TS `STAT18, STAT19`) — they're unused slots in the RS2-225 era enum. Renaming would break index parity with TS PlayerStat.ts.
- **The `init()` runs at package load time**, before any test or main. If the formula were ever to fail (e.g., division by zero), it'd manifest as a package-load panic — desirable failure mode.
- **No changes to `NpcStat*` constants.** They live in `npctype.go` and stay there. NPCs and players have separate enum identities even though the first 4 indices coincide.
