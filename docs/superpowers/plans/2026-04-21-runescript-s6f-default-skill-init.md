# RuneScript S6f: Default Player Skill Init at Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the S6e Task 1 ad-hoc Hitpoints-only seed in `processLogins` with the full TS-faithful default-player init from `PlayerLoading.ts:41-53`. Add the remaining 20 `PlayerStat*` constants and a `GetExpByLevel` helper.

**Architecture:** Two tasks. Task 1 ships pure infrastructure: a new `pkg/objtype/playerstat.go` with all 21 PlayerStat constants and the XP curve helper; the existing single-entry `PlayerStatHitpoints` is moved out of `npctype.go` into the new file (same package-qualified name; consumers don't notice). Build stays green throughout — no behaviour change. Task 2 flips the read path: `processLogins` replaces the ad-hoc Hitpoints-only seed with the full default-player init loop, and the existing login-seed test is broadened to assert all 21 skills.

**Tech Stack:** Go; `pkg/objtype` constants & helpers; `modules/world` tick loop.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s6f-default-skill-init-design.md`](../specs/2026-04-21-runescript-s6f-default-skill-init-design.md) (commit `65e004d`)

---

## Task 1: New `pkg/objtype/playerstat.go` + move PlayerStatHitpoints + XP curve helper

**Files:**
- Create: `pkg/objtype/playerstat.go` (21 constants + `PlayerStatCount` + `levelExperience` table + `GetExpByLevel`)
- Create: `pkg/objtype/playerstat_test.go` (5 tests)
- Modify: `pkg/objtype/npctype.go` (delete the moved `PlayerStatHitpoints` block from S6e Task 1)

This task is pure infrastructure. No `modules/world` changes; no behavior change in any consumer. The `objtype.PlayerStatHitpoints` qualified reference resolves identically before and after the move.

- [ ] **Step 1: Write the failing tests FIRST.** Create `pkg/objtype/playerstat_test.go`:

```go
package objtype

import "testing"

func TestGetExpByLevelKnownValues(t *testing.T) {
	cases := []struct {
		level, want int
	}{
		{1, 0},          // base case (TS returns undefined; we return 0)
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

- [ ] **Step 2: Run tests to verify they fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run "TestGetExpByLevel|TestPlayerStatCount|TestPlayerStatHitpoints" -v
```

Expected: FAIL at build with `undefined: GetExpByLevel`, `undefined: PlayerStatCount`, etc. (`PlayerStatHitpoints` IS defined in `npctype.go` from S6e Task 1, so that test alone would pass — but compile fails because the others don't exist, so nothing runs.)

- [ ] **Step 3: Create `pkg/objtype/playerstat.go`.**

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

- [ ] **Step 4: Delete the moved `PlayerStatHitpoints` block from `pkg/objtype/npctype.go`.** Open the file. Find the `PlayerStat*` block (added in S6e Task 1, immediately after the `NpcStat*` block):

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

DELETE this entire block (the const block plus its preceding comment). The `NpcStat*` block above it stays untouched.

If the deletion leaves a stray blank line at the end of the file or between blocks, leave it — `gofmt` will normalize.

- [ ] **Step 5: Run tests to verify they pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run "TestGetExpByLevel|TestPlayerStatCount|TestPlayerStatHitpoints" -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
```

All 5 tests pass; full package green.

- [ ] **Step 6: Build full repo + quality checks.** The constant moved files; consumers in `modules/world` (the S6e Task 1 seed line, `Player.CurHP()`/`BaseHP()` getters from S6e Task 2) reference `objtype.PlayerStatHitpoints` and resolve identically — build should be clean.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/objtype/
```

All clean. `gofmt -l` empty for `pkg/objtype/npctype.go`, `pkg/objtype/playerstat.go`, and `pkg/objtype/playerstat_test.go`. (Pre-existing drift in other files is not your concern.)

- [ ] **Step 7: Commit.**

```bash
git add pkg/objtype/playerstat.go pkg/objtype/playerstat_test.go pkg/objtype/npctype.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): all 21 PlayerStat constants + GetExpByLevel helper

New pkg/objtype/playerstat.go owns all PlayerStat* constants
(Attack..Runecraft, including unused STAT18/19 for index parity
with TS PlayerStat.ts) plus GetExpByLevel — a TS-faithful XP
curve helper using levelExperience[i] = floor(acc/4) * 10
matching Player.ts:77-85. Defensive clamps at level <=1 (returns 0)
and level >99 (clamps to level-99 XP) match the convention from
Player.Damage / *Npc.Damage.

Moves PlayerStatHitpoints out of npctype.go (where S6e Task 1
parked it as a single-entry block) into playerstat.go alongside
its 20 siblings. No call-site changes — same package-qualified name.

Pure infrastructure for S6f Task 2, which will use the loop +
helper to do full default-player skill init at login.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA + green build/tests + gofmt clean.

---

## Task 2: Full default-player skill init in `processLogins` + extended login-seed test

**Files:**
- Modify: `modules/world/tick.go` (replace S6e Task 1 ad-hoc seed with full init loop)
- Modify: `modules/world/tick_logins_test.go` (replace `TestProcessLoginsSeedsHitpoints` with `TestProcessLoginsSeedsAllSkillsToDefaults`)

- [ ] **Step 1: Write the failing test FIRST.** Open `modules/world/tick_logins_test.go`. Find the existing `TestProcessLoginsSeedsHitpoints` function (added in S6e Task 1). Replace its entire body — function name and all — with:

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

The test file's existing imports already include `"github.com/zsrv/goscape/pkg/objtype"` from S6e Task 1, so no new import line is needed.

- [ ] **Step 2: Run the test to verify it fails.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessLoginsSeeds" -v
```

Expected: FAIL — the assertions for non-Hitpoints skills (`levels[0..2,4..20] == 1`) fail because the current `processLogins` only seeds Hitpoints; other slots stay at zero. The Hitpoints `stats` assertion also fails (currently 0, test wants 11540).

- [ ] **Step 3: Replace the ad-hoc seed in `processLogins`.** Open `modules/world/tick.go`. Find the seeding block added in S6e Task 1 (placed between `p.invs = ...` and `p.masks |= MaskAppearance`):

```go
		// Seed Hitpoints to 10 (RS2 default starting HP) before any code
		// reads p.levels[PlayerStatHitpoints]. Matches TS PlayerLoading.ts:49-51.
		// Full skill initialization (all 21 skills with persisted XP) is a
		// future sub-spec; S6e covers Hitpoints only because the persistent-HP
		// design requires it.
		p.baseLevels[objtype.PlayerStatHitpoints] = 10
		p.levels[objtype.PlayerStatHitpoints] = 10
```

Replace it with the full default-player init:

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

The cast `int32(objtype.GetExpByLevel(10))` is required: `Player.stats` is `[21]int32`, `GetExpByLevel` returns `int`. The value `11540` fits comfortably in `int32`.

- [ ] **Step 4: Run the test to verify it passes.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessLoginsSeeds" -v
```

PASS.

- [ ] **Step 5: Run full repo + quality checks.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l modules/world/
```

All PASS / clean. `gofmt -l` empty for `modules/world/tick.go` and `modules/world/tick_logins_test.go`. (Pre-existing drift in other files is not your concern.)

- [ ] **Step 6: Sweep for residual ad-hoc-seed references.** The S6e Task 1 commit body and other documentation may reference the ad-hoc Hitpoints seed. The replacement is documentary — code is now the full init. No additional code changes expected, but spot-check that no test other than `TestProcessLoginsSeeds*` was specifically relying on the post-S6e-T1 partial-init state:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache grep -rn 'Hitpoints.*=.*10\b' modules/world/
```

Hits should be: the production seed in `tick.go`, the test assertions in `tick_logins_test.go`. Any other hit (e.g., a test asserting `levels[0..2] == 0` post-login) is a real regression to address.

- [ ] **Step 7: Commit.**

```bash
git add modules/world/tick.go modules/world/tick_logins_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): S6f full default-player skill init at login

Replaces S6e Task 1's ad-hoc Hitpoints-only seed in processLogins
with the full TS-faithful default-player init from PlayerLoading.ts:
41-53 (the "no save data" branch). All 21 skills now initialised
to level 1 / 0 XP; Hitpoints overridden to level 10 / 11540 XP via
the new objtype.GetExpByLevel helper.

Closes the stats[Hitpoints]=0 TS-infidelity flagged by the S6e
final review (M5). Save-file load + restore deferred; this default-
init becomes the no-save fallback when that lands.

Test broadened: TestProcessLoginsSeedsHitpoints renamed to
TestProcessLoginsSeedsAllSkillsToDefaults and now asserts every
slot's seeded value, including Hitpoints' XP.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA + full-repo green + race + vet + gofmt clean.

---

## Self-Review Checklist

After both tasks complete:

- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` green
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/objtype/ modules/world/` empty (or only flags pre-existing drift you didn't touch)
- [ ] Two commits on main: T1 infrastructure (playerstat.go + helper + move), T2 behaviour (full init + broader test)
- [ ] `pkg/objtype/npctype.go` no longer contains `PlayerStat*` block
- [ ] `pkg/objtype/playerstat.go` exists with 21 constants + `PlayerStatCount` + helper
- [ ] Spec coverage:
  - [ ] All 21 PlayerStat constants exported → T1
  - [ ] `GetExpByLevel` helper with TS-faithful curve + defensive clamps → T1
  - [ ] PlayerStatHitpoints moved out of npctype.go → T1
  - [ ] processLogins replaces ad-hoc seed with full default init → T2
  - [ ] Hitpoints override matches PlayerLoading.ts:49-51 → T2
  - [ ] Login-seed test broadened to cover all 21 skills → T2
  - [ ] XP curve correctness tests against canonical values → T1
  - [ ] Boundary clamp tests (low + high) → T1
