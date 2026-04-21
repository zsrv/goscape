# RuneScript S6g: XP Gain + Level-Up Math Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `Player.getLevelByExp` as `objtype.GetLevelByExp`, export `objtype.MaxSkillXP = 130344310`, and rewrite `Player.AddXP` to match TS advance-stat math (cap clamp + baseLevels recompute + drain replenish). Closes the explicit TODO at `modules/world/player_script.go:187`.

**Architecture:** Two tasks. Task 1 extends `pkg/objtype/playerstat.go` with the new helper and const plus tests — pure infrastructure with no consumer impact; `Player.AddXP` doesn't call them yet. Build stays green. Task 2 rewrites `Player.AddXP` using the new helpers and ships a dedicated `modules/world/player_script_test.go` with 7 scenario tests covering cap, level-up, drain replenish, and OOB.

**Tech Stack:** Go; `pkg/objtype` helpers; `modules/world/player_script.go` XP mutation.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s6g-xp-gain-design.md`](../specs/2026-04-21-runescript-s6g-xp-gain-design.md) (commit `48099be`)

---

## Task 1: `GetLevelByExp` + `MaxSkillXP` in `pkg/objtype/playerstat.go` + tests

**Files:**
- Modify: `pkg/objtype/playerstat.go` (append `MaxSkillXP` const + `GetLevelByExp` function)
- Modify: `pkg/objtype/playerstat_test.go` (append 4 new tests)

This task is pure infrastructure — `Player.AddXP` still uses the old body, no `modules/world` changes. Task 2 wires the new helpers.

- [ ] **Step 1: Write the failing tests FIRST.** Open `pkg/objtype/playerstat_test.go`. Append at the end:

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

- [ ] **Step 2: Run tests to verify they fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run "TestGetLevelByExp|TestMaxSkillXP" -v
```

Expected: FAIL at build with `undefined: GetLevelByExp` and `undefined: MaxSkillXP`.

- [ ] **Step 3: Add `MaxSkillXP` const + `GetLevelByExp` function to `pkg/objtype/playerstat.go`.** Open the file. Find the existing `GetExpByLevel` function. Append immediately after it:

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

- [ ] **Step 4: Run tests to verify they pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run "TestGetLevelByExp|TestMaxSkillXP" -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
```

All 4 new tests PASS; full package green.

- [ ] **Step 5: Build full repo + quality checks.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/objtype/
```

All clean. `gofmt -l` empty for `pkg/objtype/playerstat.go` and `pkg/objtype/playerstat_test.go`. Pre-existing drift in other files is not your concern.

- [ ] **Step 6: Commit.**

```bash
git add pkg/objtype/playerstat.go pkg/objtype/playerstat_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): GetLevelByExp + MaxSkillXP const

Ports TS Player.getLevelByExp (Player.ts:87-95) as the inverse of
S6f's GetExpByLevel. Adds MaxSkillXP = 130344310 (level-99 cap,
== GetExpByLevel(99)) for consumers that need to clamp XP.

Round-trip test verifies GetLevelByExp(GetExpByLevel(L)) == L for
every valid level 2..99 — catches off-by-one errors in either
direction. Known-value table covers level boundaries including
the 99-cap clamp.

Pure infrastructure for S6g Task 2, which will use both to fix
Player.AddXP's cap + baseLevels recompute + drain replenish math.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA + green build/tests + gofmt clean.

---

## Task 2: Rewrite `Player.AddXP` + new `player_script_test.go` with 7 AddXP tests

**Files:**
- Modify: `modules/world/player_script.go` (rewrite `Player.AddXP` body; delete TODO; add `objtype` import if missing)
- Create: `modules/world/player_script_test.go` (7 AddXP tests)

- [ ] **Step 1: Write the failing tests FIRST.** Create `modules/world/player_script_test.go`:

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
	// Still level 2 (level 3 threshold is 1740).
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
	// levels[Attack] == beforeBase (1), NOT less — so no replenish.
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
	p.AddXP(objtype.PlayerStatAttack, -100) // would go negative
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

- [ ] **Step 2: Run tests to verify they fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestAddXP" -v
```

Expected: FAIL — assertions don't match current behavior. Specifically:
- `TestAddXPNormalGainNoLevelUp`: current `AddXP` doesn't recompute `baseLevels`, so the assertion `baseLevels == 2` may pass (it stays at its pre-set value of 2). But the test as written pre-sets `baseLevels = 2` and the happy path doesn't change it, so this particular test might coincidentally pass. The others don't — they assert level-up recompute or cap-clamp that the current code doesn't implement.
- `TestAddXPClampsAtCap`: current `AddXP` does `p.stats[id] += int32(xp)`, which with `stats = MaxSkillXP - 10 = 130344300` and `xp = 1000` produces `130345300` — past the cap. Assertion fails.
- `TestAddXPLevelUpNotDrained`: current code leaves `baseLevels` at 2; assertion wants 3.
- Similar for `TestAddXPLevelUpWhileDrained`, `TestAddXPMultiLevelUpNotDrained`, `TestAddXPNegativeClampsAtZero`.
- `TestAddXPOOBIsNoop` passes already (OOB guard via `statBounds` is pre-existing).

- [ ] **Step 3: Rewrite `Player.AddXP` in `modules/world/player_script.go`.** Open the file. Find the method at lines 185-193 (note: the exact line numbers may drift by a couple lines; search for `func (p *Player) AddXP`). The current body:

```go
// AddXP adds xp (scaled * 10) to the player's stored XP for skill id.
// OOB ids are dropped silently.
// TODO: recompute baseLevels from getLevelByExp table and clamp at XP cap.
func (p *Player) AddXP(id int, xp int) {
	if !statBounds(id) {
		return
	}
	p.stats[id] += int32(xp)
}
```

Replace the entire comment + function with:

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

Ensure `objtype` is imported. Open the imports block at the top of `player_script.go`:

```go
import (
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)
```

Replace with:

```go
import (
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)
```

(Keep imports alphabetized within the group — `objtype` sits between `protocol/game/server` and `rsbuf`.)

- [ ] **Step 4: Run the new AddXP tests to verify they pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestAddXP" -v
```

All 7 tests PASS.

- [ ] **Step 5: Verify `TestStatAdvanceViaScript` still passes.** This pre-existing test (`modules/world/script_test.go:511-542`) uses `p.stats[3] = 100` then adds 50 XP via the `OpStatAdvance` script opcode. It asserts only `p.stats[3] == 150` (not baseLevels/levels). Our new AddXP behavior: XP 150 is below the level-2 threshold (830), so `GetLevelByExp(150) = 1`; `baseLevels[3]` becomes 1 (up from the zero-init default of 0). The test's sole assertion `stats[3] == 150` still holds.

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestStatAdvanceViaScript -v
```

PASS. If it fails unexpectedly, investigate — don't modify the test without understanding why.

- [ ] **Step 6: Run full repo + quality checks.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l modules/world/
```

All PASS / clean. `gofmt -l` must NOT flag `modules/world/player_script.go` or `modules/world/player_script_test.go`. Pre-existing drift on other files is fine — don't sweep.

- [ ] **Step 7: Commit.**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): S6g Player.AddXP cap + baseLevels + drain replenish

Closes the TODO at player_script.go:187. AddXP now matches TS
Player.advanceStat (Player.ts:1758-1772):
- Clamps at objtype.MaxSkillXP (level-99 cap) using int64
  intermediate to avoid int32 overflow on adversarial inputs
- Recomputes baseLevels[id] via objtype.GetLevelByExp
- On level-up with drained levels[id] < beforeBase, replenishes
  by the level delta. Non-drained level-ups leave levels[id]
  alone (strict < test, matches TS line 1767)
- Defensive negative-xp clamp at 0 (matches S6c/S6e Damage
  convention; deviation from TS which would reduce stored XP)

ChangeStat trigger fire deferred — no cache-script consumer yet.

New player_script_test.go ships 7 scenario tests: normal gain,
level-up not drained, level-up while drained, multi-level-up,
cap clamp, negative clamp, OOB noop.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA + full-repo green + race + vet + gofmt clean + confirm `TestStatAdvanceViaScript` still passes.

---

## Self-Review Checklist

After both tasks complete:

- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` green
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/objtype/ modules/world/` empty (or only flags pre-existing drift you didn't touch)
- [ ] Two commits on main: T1 `feat(objtype): GetLevelByExp + MaxSkillXP const`, T2 `feat(world): S6g Player.AddXP cap + baseLevels + drain replenish`
- [ ] `player_script.go:187` TODO comment is GONE (deleted as part of T2)
- [ ] `grep -n 'TODO' modules/world/player_script.go` shows no remaining TODO related to AddXP
- [ ] Spec coverage:
  - [ ] `GetLevelByExp` in `pkg/objtype/playerstat.go` with TS-faithful curve + level-99 clamp → T1
  - [ ] `MaxSkillXP = 130344310` exported const → T1
  - [ ] Round-trip test `GetLevelByExp(GetExpByLevel(L)) == L` → T1
  - [ ] Known-values table for XP curve boundaries → T1
  - [ ] `Player.AddXP` clamps at XP cap with int64 intermediate → T2
  - [ ] `Player.AddXP` recomputes `baseLevels[id]` → T2
  - [ ] `Player.AddXP` replenishes drained `levels[id]` on level-up → T2
  - [ ] `Player.AddXP` defensive negative-xp clamp at 0 → T2
  - [ ] `Player.AddXP` OOB id is noop → T2 (existing behavior preserved)
  - [ ] 7 AddXP scenario tests in new `player_script_test.go` → T2
  - [ ] `TestStatAdvanceViaScript` still passes unchanged → T2
