# NAI-184 Combat-Level Recompute Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `Player.getCombatLevel()` and wire the guarded-recompute pattern into the three sites that TS recomputes on (SetStat, AddXP level-up, LoadSave), retiring `DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD`, `NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD`, and one informal AddXP deferral.

**Architecture:** Two new private methods on `*Player` in `modules/world/player_script.go`: a pure formula `calcCombatLevel() int` and a guarded-side-effect `recomputeCombatLevel(triggerRebuild bool)` that flips `MaskAppearance` via `SetAppearanceInv` when the value changes. Three call-site one-liners. No new interfaces, no new infra; goscape's `p.combatLevel` field and `SetAppearanceInv` (the goscape equivalent of TS `buildAppearance`) already exist.

**Tech Stack:** Go 1.26 (`math.Floor`, `math.Max`, integer division on `int`); existing `pkg/objtype/playerstat.go` constants (`PlayerStatAttack`…`PlayerStatMagic`); existing `rsbuf.MaskAppearance`; `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...` invocation prefix per CLAUDE.md.

**Spec:** `docs/superpowers/specs/2026-05-20-nai184-combat-level-recompute-design.md` (commit `fcb3242f`).

**HEAD at start:** `fcb3242f` on top of `578bf55b` (handlePWalk port).

---

## File map

| File | What changes |
| --- | --- |
| `modules/world/player_script.go` | **Modify**: add `calcCombatLevel()` + `recomputeCombatLevel(triggerRebuild bool)`; hook into `SetStat` (line ~682); hook into `AddXP` level-up branch (line ~786-791); remove `DEVIATION-NAI-184-D1` paragraph from `SetStat` doc; remove "Does NOT recompute combat level" sentence from `AddXP` doc. |
| `modules/world/player_load.go` | **Modify**: hook `p.recomputeCombatLevel(false)` into `LoadSave` before the final `return nil` (line ~243); replace `NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD` comment with a one-liner. |
| `modules/world/player_script_test.go` | **Modify**: add 7 formula tests + 3 recompute-method tests + 2 SetStat integration tests + 2 AddXP integration tests. Append at end-of-file (after current line ~1935). |
| `modules/world/player_load_integration_test.go` | **Modify**: add 1 LoadSave integration test alongside existing `validSAVBytes` / `runProcessLogins` fixtures. |

No new files. No new packages. No new imports outside `math` (in `player_script.go`).

---

## Conventions (apply to every task)

- All `go` invocations: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`
- All commits: `git commit --no-gpg-sign -m "..."`
- Pre-commit: run `git status` and confirm only the expected files are staged. Stage explicitly by name — never `git add -A` or `git add .` (project has standing `config.yaml` drift and standing untracked noise).
- Post-commit: run `git show --stat HEAD` and confirm the file list matches expectations.
- Never `--amend`. Always create new commits.
- Existing baseline gates that must stay green at every commit: `-race ./...` (57+ pkgs / 0 FAIL) and smoke-pack (12 OK / 0 ERR). Per-task instructions specify the narrow test slice to run during the task; the final task runs the full gate.
- The "RED" step (run-test-and-see-it-fail) is required by TDD — do NOT skip it. The diagnostic value of seeing the exact failure message before writing the fix is what makes TDD work.

---

## Task 1: Pure `calcCombatLevel()` formula port

**Goal:** Land a unit-testable, side-effect-free combat-level formula that matches TS `Player.getCombatLevel` (Player.ts:1302-1308). Seven table tests pin the boundary cases.

**Files:**
- Modify: `modules/world/player_script.go` (add method near `SetStat`, ~line 695-696)
- Test: `modules/world/player_script_test.go` (append after current EOF)

- [ ] **Step 1.1: Write the 7 failing formula tests**

Append to `modules/world/player_script_test.go` at end-of-file:

```go
// TestCalcCombatLevel_* pin the goscape port of TS Player.getCombatLevel
// (Engine-TS/.../Player.ts:1302-1308). The formula uses baseLevels[]
// (not levels[]) — buffs/drains don't move combat level. NAI-184 T1.
//
// "Fresh stats" convention: baseLevels = all 1 except HP = 10, mirroring
// the empty-save bootstrap at player_load.go:79-85.

func TestCalcCombatLevel_FreshStats(t *testing.T) {
	p := &Player{}
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	if got := p.calcCombatLevel(); got != 3 {
		t.Errorf("calcCombatLevel(fresh): got %d, want 3", got)
	}
}

func TestCalcCombatLevel_Maxed(t *testing.T) {
	p := &Player{}
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 99
	}
	if got := p.calcCombatLevel(); got != 126 {
		t.Errorf("calcCombatLevel(maxed): got %d, want 126", got)
	}
}

func TestCalcCombatLevel_PureMelee99(t *testing.T) {
	p := &Player{}
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.baseLevels[objtype.PlayerStatAttack] = 99
	p.baseLevels[objtype.PlayerStatStrength] = 99
	if got := p.calcCombatLevel(); got != 67 {
		t.Errorf("calcCombatLevel(att=str=99, rest fresh): got %d, want 67", got)
	}
}

func TestCalcCombatLevel_PureRanged99(t *testing.T) {
	p := &Player{}
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.baseLevels[objtype.PlayerStatRanged] = 99
	if got := p.calcCombatLevel(); got != 50 {
		t.Errorf("calcCombatLevel(range=99, rest fresh): got %d, want 50", got)
	}
}

func TestCalcCombatLevel_PureMagic99(t *testing.T) {
	p := &Player{}
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.baseLevels[objtype.PlayerStatMagic] = 99
	if got := p.calcCombatLevel(); got != 50 {
		t.Errorf("calcCombatLevel(mage=99, rest fresh): got %d, want 50", got)
	}
}

func TestCalcCombatLevel_PrayerLeveraged(t *testing.T) {
	p := &Player{}
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatDefence] = 99
	p.baseLevels[objtype.PlayerStatHitpoints] = 99
	p.baseLevels[objtype.PlayerStatPrayer] = 99
	if got := p.calcCombatLevel(); got != 62 {
		t.Errorf("calcCombatLevel(def=hp=prayer=99, rest=1): got %d, want 62", got)
	}
}

func TestCalcCombatLevel_UsesBaseLevelsNotLevels(t *testing.T) {
	// Critical regression guard: drinking a strength potion does NOT
	// change combat level. baseLevels is fresh; levels[STR] is boosted.
	p := &Player{}
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 1
		p.levels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.levels[objtype.PlayerStatHitpoints] = 10
	p.levels[objtype.PlayerStatStrength] = 99 // boosted ONLY in levels, not baseLevels
	if got := p.calcCombatLevel(); got != 3 {
		t.Errorf("calcCombatLevel(potion-boosted): got %d, want 3 (must ignore levels[])", got)
	}
}
```

- [ ] **Step 1.2: Run tests to verify they fail (RED)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestCalcCombatLevel_' ./modules/world/... -v`

Expected: 7 compile errors of the form `p.calcCombatLevel undefined (type *Player has no field or method calcCombatLevel)`.

If they compile and pass, something is very wrong — STOP and investigate.

- [ ] **Step 1.3: Implement `calcCombatLevel()`**

In `modules/world/player_script.go`, find the `SetStat` function (currently at ~line 682). Insert the following method **immediately AFTER `SetStat` ends** (after the closing `}` at ~line 695, before the `changeStat` doc-block):

```go
// calcCombatLevel ports TS Player.getCombatLevel (Player.ts:1302-1308).
// Pure formula, no side effects. Uses baseLevels[] (NOT levels[]) so
// buffs/drains don't move combat level. Result is bounded: at fresh
// stats (all=1, hp=10) CL=3; at all-99 maxed stats CL=126.
//
// Integer division (Go) on non-negative operands floors exactly like
// TS Math.floor — prayer/2, rng/2, mag/2 don't need an explicit floor.
// math.Floor on the final float64 mirrors the outer TS Math.floor.
//
// NAI-184.
func (p *Player) calcCombatLevel() int {
	def := int(p.baseLevels[objtype.PlayerStatDefence])
	hp := int(p.baseLevels[objtype.PlayerStatHitpoints])
	prayer := int(p.baseLevels[objtype.PlayerStatPrayer])
	att := int(p.baseLevels[objtype.PlayerStatAttack])
	str := int(p.baseLevels[objtype.PlayerStatStrength])
	rng := int(p.baseLevels[objtype.PlayerStatRanged])
	mag := int(p.baseLevels[objtype.PlayerStatMagic])

	base := 0.25 * float64(def+hp+prayer/2)
	melee := 0.325 * float64(att+str)
	rangd := 0.325 * float64(rng/2+rng)
	magic := 0.325 * float64(mag/2+mag)

	return int(math.Floor(base + math.Max(melee, math.Max(rangd, magic))))
}
```

Ensure `math` is in the import block at the top of `player_script.go`. If it isn't, add it. (Run `goimports` mentally — if `math` was already imported it'll be unchanged; if not, add a line `"math"` in the appropriate stdlib group.)

- [ ] **Step 1.4: Run tests to verify they pass (GREEN)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestCalcCombatLevel_' ./modules/world/... -v`

Expected: 7 PASS.

If any FAIL, the formula is wrong. Re-check the arithmetic against the spec §5.3 table (each row's "Arithmetic" column shows the hand computation). Do NOT modify the test's expected value — it's the ground truth.

- [ ] **Step 1.5: Run a wider sanity check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`

Expected: no errors.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1`

Expected: all green. No existing test regressions.

- [ ] **Step 1.6: Pre-commit `git status` check**

Run: `git status`

Expected: exactly two modified files — `modules/world/player_script.go` and `modules/world/player_script_test.go`. Standing untracked noise (`config.yaml` drift, `.claude/`, etc.) is fine — leave it.

- [ ] **Step 1.7: Commit**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): port *Player.calcCombatLevel (NAI-184 T1)

Pure formula from TS Player.getCombatLevel (Player.ts:1302-1308).
Seven table tests pin boundary cases: fresh CL=3, maxed CL=126,
prayer-leveraged CL=62, pure-melee/ranged/magic-99, and the
critical "potions don't change CL" regression guard.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 1.8: Verify commit**

Run: `git show --stat HEAD`

Expected: two files changed, ~120 lines added (formula ~25 lines + 7 tests ~95 lines).

---

## Task 2: `recomputeCombatLevel(triggerRebuild bool)` method

**Goal:** Land the guarded-rebuild helper that wraps `calcCombatLevel`. Three unit tests pin: no-change-no-flip, change-with-rebuild-flips-mask, change-without-rebuild-keeps-mask-clean.

**Files:**
- Modify: `modules/world/player_script.go` (add method after `calcCombatLevel`)
- Test: `modules/world/player_script_test.go` (append after T1 tests)

- [ ] **Step 2.1: Write the 3 failing tests**

Append to `modules/world/player_script_test.go` after the T1 tests:

```go
// TestRecomputeCombatLevel_* pin the guarded-rebuild semantics.
// Mirrors the inline `if (combatLevel != getCombatLevel()) { ...
// buildAppearance(appearanceInv); }` blocks at TS Player.ts:1810-1813
// and 1830-1833. NAI-184 T2.

func TestRecomputeCombatLevel_NoChange_NoMaskFlip(t *testing.T) {
	p := &Player{}
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.combatLevel = 3 // matches calcCombatLevel() for these stats
	p.masks = 0
	p.recomputeCombatLevel(true)
	if p.combatLevel != 3 {
		t.Errorf("combatLevel: got %d, want 3 (no-op when value unchanged)", p.combatLevel)
	}
	if p.masks&rsbuf.MaskAppearance != 0 {
		t.Errorf("masks: MaskAppearance unexpectedly set when CL didn't change")
	}
}

func TestRecomputeCombatLevel_Change_RebuildTrue_FlipsMask(t *testing.T) {
	p := &Player{}
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.baseLevels[objtype.PlayerStatStrength] = 99 // CL now 67
	p.combatLevel = 3                              // stale
	p.appearanceInv = 42                           // arbitrary, must remain unchanged
	p.masks = 0
	p.recomputeCombatLevel(true)
	if p.combatLevel != 67 {
		t.Errorf("combatLevel: got %d, want 67", p.combatLevel)
	}
	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Errorf("masks: MaskAppearance not set after CL change with triggerRebuild=true")
	}
	if p.appearanceInv != 42 {
		t.Errorf("appearanceInv: got %d, want 42 (must not be reset)", p.appearanceInv)
	}
}

func TestRecomputeCombatLevel_Change_RebuildFalse_NoMaskFlip(t *testing.T) {
	p := &Player{}
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.baseLevels[objtype.PlayerStatStrength] = 99 // CL now 67
	p.combatLevel = 3
	p.masks = 0
	p.recomputeCombatLevel(false)
	if p.combatLevel != 67 {
		t.Errorf("combatLevel: got %d, want 67 (field still updates)", p.combatLevel)
	}
	if p.masks&rsbuf.MaskAppearance != 0 {
		t.Errorf("masks: MaskAppearance unexpectedly set when triggerRebuild=false")
	}
}
```

Ensure `rsbuf` is in the test file's import block. If it isn't (it's used by other tests in this file so likely already imported), add `"github.com/zsrv/goscape/pkg/io/rsbuf"` — but VERIFY first by grepping the file's imports.

- [ ] **Step 2.2: Run tests to verify they fail (RED)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestRecomputeCombatLevel_' ./modules/world/... -v`

Expected: 3 compile errors of the form `p.recomputeCombatLevel undefined`.

- [ ] **Step 2.3: Implement `recomputeCombatLevel`**

In `modules/world/player_script.go`, immediately AFTER `calcCombatLevel` (added in T1), insert:

```go
// recomputeCombatLevel updates p.combatLevel if calcCombatLevel now
// yields a different value. When triggerRebuild is true, also flips
// MaskAppearance (via SetAppearanceInv) so the next encodeOut emits a
// fresh appearance — required after stat-changing operations that
// happen post-login. When triggerRebuild is false, only updates the
// field — used at LoadSave time, before the client has any appearance.
//
// SetStat and AddXP pass true; LoadSave passes false. Mirrors the
// guarded-rebuild blocks at TS Player.ts:1810-1813 and 1830-1833;
// the false-variant matches PlayerLoading.ts:156.
//
// NAI-184.
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

- [ ] **Step 2.4: Run tests to verify they pass (GREEN)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestRecomputeCombatLevel_' ./modules/world/... -v`

Expected: 3 PASS.

- [ ] **Step 2.5: Sanity sweep**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1`

Expected: all green.

- [ ] **Step 2.6: Pre-commit `git status` check**

Run: `git status`

Expected: exactly two modified files (`modules/world/player_script.go`, `modules/world/player_script_test.go`).

- [ ] **Step 2.7: Commit**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): add *Player.recomputeCombatLevel guarded-rebuild helper (NAI-184 T2)

Wraps calcCombatLevel with the guarded-rebuild semantics from TS
Player.ts:1810-1813 / 1830-1833. triggerRebuild=true flips
MaskAppearance via SetAppearanceInv; triggerRebuild=false (LoadSave
path) only updates the field.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 2.8: Verify commit**

Run: `git show --stat HEAD`

Expected: two files changed, ~70 lines added (helper ~15 lines + 3 tests ~55 lines).

---

## Task 3: Hook into `SetStat` + retire `DEVIATION-NAI-184-D1`

**Goal:** Wire `recomputeCombatLevel(true)` into `SetStat` and retire `DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD`. Two integration tests pin: combat-stat bump flips the mask; non-combat-stat bump does not.

**Files:**
- Modify: `modules/world/player_script.go` (`SetStat` body + doc-block at ~line 678-695)
- Test: `modules/world/player_script_test.go` (append after T2 tests)

- [ ] **Step 3.1: Write the 2 failing integration tests**

Append to `modules/world/player_script_test.go` after the T2 tests:

```go
// TestSetStat_RecomputesCombatLevel* pin the SetStat hook into the
// guarded combat-level rebuild. Retires DEVIATION-NAI-184-D1-SETSTAT-
// NO-COMBAT-REBUILD. NAI-184 T3.

func TestSetStat_RecomputesCombatLevelAndFlipsAppearance(t *testing.T) {
	p := &Player{}
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.combatLevel = 3 // fresh
	p.masks = 0
	p.SetStat(objtype.PlayerStatStrength, 99)
	if p.combatLevel <= 3 {
		t.Errorf("combatLevel: got %d, want > 3 after STR→99", p.combatLevel)
	}
	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Errorf("masks: MaskAppearance not set after combat-stat SetStat")
	}
}

func TestSetStat_NonCombatStat_NoMaskFlip(t *testing.T) {
	// Cooking is not a combat stat; SetStat(cooking, 50) must NOT change
	// combatLevel and must NOT flip MaskAppearance.
	p := &Player{}
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.combatLevel = 3
	p.masks = 0
	p.SetStat(objtype.PlayerStatCooking, 50)
	if p.combatLevel != 3 {
		t.Errorf("combatLevel: got %d, want 3 (non-combat stat must not move CL)", p.combatLevel)
	}
	if p.masks&rsbuf.MaskAppearance != 0 {
		t.Errorf("masks: MaskAppearance unexpectedly set after non-combat-stat SetStat")
	}
}
```

- [ ] **Step 3.2: Run tests to verify they fail (RED)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestSetStat_RecomputesCombatLevelAndFlipsAppearance|TestSetStat_NonCombatStat_NoMaskFlip' ./modules/world/... -v`

Expected:
- `TestSetStat_RecomputesCombatLevelAndFlipsAppearance` FAILS — `combatLevel <= 3` (still 3 because SetStat doesn't recompute).
- `TestSetStat_NonCombatStat_NoMaskFlip` PASSES vacuously (today nothing changes either, since SetStat doesn't touch CL/mask at all).

The plan is that after the hook lands, BOTH pass — the second test pins that the guard-no-op path correctly leaves the mask alone.

- [ ] **Step 3.3: Modify `SetStat` to call `recomputeCombatLevel(true)` and retire `DEVIATION-NAI-184-D1`**

In `modules/world/player_script.go`, the current `SetStat` is at ~line 674-695. Replace it with:

```go
// SetStat clamps level to [1, 99] and writes baseLevels, levels, and
// stats (XP) for the given stat slot. Mirrors TS Player.setLevel
// (Player.ts:1823-1834). Used by ::setstat and ::minme cheats (NAI-184).
//
// On any change, calls recomputeCombatLevel(true) — TS guards the
// rebuild on (combatLevel != getCombatLevel()) so non-combat-stat
// changes and no-op cases don't flip MaskAppearance.
func (p *Player) SetStat(stat, level int) {
	if !statBounds(stat) {
		return
	}
	if level < 1 {
		level = 1
	}
	if level > 99 {
		level = 99
	}
	p.baseLevels[stat] = uint8(level)
	p.levels[stat] = uint8(level)
	p.stats[stat] = int32(objtype.GetExpByLevel(level))
	p.recomputeCombatLevel(true)
}
```

Note the changes vs the original:
- Removed the 4-line `DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD` paragraph (was lines 678-681).
- Added the `p.recomputeCombatLevel(true)` call as the final body line.
- Updated the doc-block to describe the new behavior (one short paragraph instead of the deviation note).

- [ ] **Step 3.4: Run tests to verify they pass (GREEN)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestSetStat_' ./modules/world/... -v`

Expected: ALL `TestSetStat_*` pass (the new two plus the existing `TestSetStat_WritesBaseCurAndXPClamped` and `TestSetStat_OOBStatDropsSilently`).

- [ ] **Step 3.5: Verify no `DEVIATION-NAI-184-D1` references remain in non-test Go**

Run: `grep -rnE 'NAI-184-D1' modules/ pkg/ cmd/ internal/ --include='*.go' | grep -v _test.go`

Expected: empty output. (Spec docs under `docs/superpowers/` may still reference the tag — those are point-in-time records and should NOT be touched.)

- [ ] **Step 3.6: Sanity sweep**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1`

Expected: all green.

- [ ] **Step 3.7: Pre-commit `git status` check**

Run: `git status`

Expected: exactly two modified files (`modules/world/player_script.go`, `modules/world/player_script_test.go`).

- [ ] **Step 3.8: Commit**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): SetStat recomputes combat level (NAI-184 T3)

Wires recomputeCombatLevel(true) into *Player.SetStat. Two tests pin
the combat-stat-bump-flips-mask and non-combat-stat-no-flip branches.

Retires DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3.9: Verify commit**

Run: `git show --stat HEAD`

Expected: two files changed, ~50 lines diff (one-line hook + doc-block trim + 2 tests).

---

## Task 4: Hook into `AddXP` level-up branch + retire informal AddXP deferral

**Goal:** Wire `recomputeCombatLevel(true)` into the level-up branch of `AddXP` and remove the "Does NOT recompute combat level (future combat sub-spec)" sentence from the doc-block. Two integration tests pin: level-up recomputes; no-level-up does not.

**Files:**
- Modify: `modules/world/player_script.go` (`AddXP` body + doc at ~line 739-792)
- Test: `modules/world/player_script_test.go` (append after T3 tests)

- [ ] **Step 4.1: Write the 2 failing integration tests**

Append to `modules/world/player_script_test.go` after the T3 tests:

```go
// TestAddXP_*CombatLevel pin the AddXP hook into the guarded combat-
// level rebuild. Retires the informal "Does NOT recompute combat
// level (future combat sub-spec)" deferral in AddXP's doc-block.
// NAI-184 T4.

func TestAddXP_LevelUp_RecomputesCombatLevel(t *testing.T) {
	p, _ := newTestPlayer(t)
	// Pre-load fresh baseLevels (newTestPlayer leaves them at zero).
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 1
		p.levels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.levels[objtype.PlayerStatHitpoints] = 10
	// Start STR at level 1 with 0 XP; add enough XP to reach level 99.
	p.stats[objtype.PlayerStatStrength] = 0
	p.baseLevels[objtype.PlayerStatStrength] = 1
	p.levels[objtype.PlayerStatStrength] = 1
	p.combatLevel = 3
	p.masks = 0
	p.AddXP(objtype.PlayerStatStrength, objtype.GetExpByLevel(99))
	if p.baseLevels[objtype.PlayerStatStrength] != 99 {
		t.Fatalf("baseLevels[STR]: got %d, want 99 (precondition for CL recompute)",
			p.baseLevels[objtype.PlayerStatStrength])
	}
	if p.combatLevel <= 3 {
		t.Errorf("combatLevel: got %d, want > 3 after STR level-up to 99", p.combatLevel)
	}
	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Errorf("masks: MaskAppearance not set after level-up combat-stat AddXP")
	}
}

func TestAddXP_NoLevelUp_NoRecompute(t *testing.T) {
	// Adding XP without crossing a level threshold must NOT trigger
	// recomputeCombatLevel — the guard short-circuits on no-change.
	// More importantly, the AddXP code only calls recompute inside the
	// afterBase > beforeBase branch.
	p, _ := newTestPlayer(t)
	for i := 0; i < objtype.PlayerStatCount; i++ {
		p.baseLevels[i] = 1
		p.levels[i] = 1
	}
	p.baseLevels[objtype.PlayerStatHitpoints] = 10
	p.levels[objtype.PlayerStatHitpoints] = 10
	// Start STR exactly at level 2; add a small amount that stays in [820, 1740).
	p.stats[objtype.PlayerStatStrength] = int32(objtype.GetExpByLevel(2))
	p.baseLevels[objtype.PlayerStatStrength] = 2
	p.levels[objtype.PlayerStatStrength] = 2
	p.combatLevel = 3
	p.masks = 0
	p.AddXP(objtype.PlayerStatStrength, 100) // → 920 XP, still level 2
	if p.baseLevels[objtype.PlayerStatStrength] != 2 {
		t.Fatalf("baseLevels[STR]: got %d, want 2 (precondition: no level-up)",
			p.baseLevels[objtype.PlayerStatStrength])
	}
	if p.combatLevel != 3 {
		t.Errorf("combatLevel: got %d, want 3 (no level-up → no recompute)", p.combatLevel)
	}
	if p.masks&rsbuf.MaskAppearance != 0 {
		t.Errorf("masks: MaskAppearance unexpectedly set without level-up")
	}
}
```

- [ ] **Step 4.2: Run tests to verify they fail (RED)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestAddXP_LevelUp_RecomputesCombatLevel|TestAddXP_NoLevelUp_NoRecompute' ./modules/world/... -v`

Expected:
- `TestAddXP_LevelUp_RecomputesCombatLevel` FAILS — `combatLevel <= 3` (still 3 because AddXP doesn't recompute today).
- `TestAddXP_NoLevelUp_NoRecompute` PASSES (today's AddXP doesn't touch combatLevel/mask in any branch).

- [ ] **Step 4.3: Modify `AddXP` to hook `recomputeCombatLevel(true)` into the level-up branch**

In `modules/world/player_script.go`, the current `AddXP` is at ~line 761-792. The doc-block at lines 758-760 reads "Does NOT recompute combat level (future combat sub-spec) or emit session-log / milestone events". Update to keep the session-log half and drop the combat-level half. Replace the function with:

```go
// AddXP adds xp (scaled ×10) to the player's stored XP for skill id and
// recomputes baseLevels from the XP curve. Matches TS Player.advanceStat
// (Player.ts:1752-1772) in three branches:
//
//   - Un-buffed (levels[id] == baseLevels[id]): advance BOTH levels and
//     baseLevels together. This is the common case — every fresh-player
//     training session. TS line 1760-1763.
//   - Buffed (levels[id] > baseLevels[id]): update baseLevels only;
//     preserve the buff on levels. Level-ups don't strip active potions.
//   - Drained (levels[id] < baseLevels[id]): update baseLevels; on
//     level-up replenish levels by the level delta. TS line 1767-1770.
//
// XP is clamped at objtype.MaxXP (200m real, stored as 2B ×10). Negative
// xp is clamped to keep stats[id] >= 0 defensively — deviation from TS
// where a bug could reduce stored XP. Matches the convention from
// Player.Damage / *Npc.Damage negative-amount clamps.
//
// On level-up (baseLevels increases), fires the [changestat,<skill>] trigger
// via changeStat (TS Player.ts:1772) then the [advancestat,<skill>] trigger
// via advanceStat (TS Player.ts:1804-1807), then calls recomputeCombatLevel
// to mirror TS Player.ts:1810-1813. Does NOT emit session-log / milestone
// events (TS Player.ts:1773-1803; session-log infrastructure not yet ported).
func (p *Player) AddXP(id int, xp int) {
	if !statBounds(id) {
		return
	}
	next := min(int64(p.stats[id])+int64(xp), int64(objtype.MaxXP))
	if next < 0 {
		next = 0
	}
	beforeBase := int(p.baseLevels[id])
	p.stats[id] = int32(next)
	newBase := objtype.GetLevelByExp(int(p.stats[id]))

	// Un-buffed branch: advance levels in lockstep with baseLevels so a
	// fresh-player level-up is visible on the stat display. TS Player.ts:1760-1763.
	if int(p.levels[id]) == beforeBase {
		p.levels[id] = uint8(newBase)
	}
	p.baseLevels[id] = uint8(newBase)
	afterBase := newBase

	if afterBase > beforeBase && int(p.levels[id]) < beforeBase {
		// Drained + level-up: replenish levels by the level delta.
		// Matches TS Player.ts:1767-1770.
		p.levels[id] = uint8(min(int(p.levels[id])+(afterBase-beforeBase), 255))
	}
	if afterBase > beforeBase {
		// Level-up: fire [changestat,<skill>] then [advancestat,<skill>]
		// triggers if registered. Matches TS Player.ts:1772, 1804-1807.
		p.changeStat(id)
		p.advanceStat(id)
		p.recomputeCombatLevel(true) // TS Player.ts:1810-1813
	}
}
```

Note the two changes vs the original:
- Doc-block: replaced the "Does NOT recompute combat level (future combat sub-spec) or emit session-log / milestone events" sentence with "calls recomputeCombatLevel to mirror TS Player.ts:1810-1813. Does NOT emit session-log / milestone events (TS Player.ts:1773-1803; session-log infrastructure not yet ported)." The session-log half is still deferred and stays documented.
- Body: inside the `afterBase > beforeBase` block, added `p.recomputeCombatLevel(true)` after `p.advanceStat(id)`.

- [ ] **Step 4.4: Run tests to verify they pass (GREEN)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestAddXP_' ./modules/world/... -v`

Expected: ALL `TestAddXP_*` pass — the new two plus existing `TestAddXPNormalGainNoLevelUp`, `TestAddXPLevelUpUnbuffedAdvancesLevels`, `TestAddXPLevelUpWhileDrained`, `TestAddXPMultiLevelUpUnbuffed`, `TestAddXPClampsAtMaxXP`, `TestAddXPAccumulatesPastLevel99ThresholdUpToMaxXP`, and the changestat/advancestat trigger tests at lines 393-625.

- [ ] **Step 4.5: Sanity sweep**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1`

Expected: all green.

- [ ] **Step 4.6: Pre-commit `git status` check**

Run: `git status`

Expected: exactly two modified files.

- [ ] **Step 4.7: Commit**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): AddXP level-up branch recomputes combat level (NAI-184 T4)

Wires recomputeCombatLevel(true) into the afterBase > beforeBase
branch of *Player.AddXP, after changeStat/advanceStat triggers fire.
Mirrors TS Player.ts:1810-1813. Two tests pin: level-up recomputes
and flips MaskAppearance; no-level-up leaves both untouched.

Retires the informal "Does NOT recompute combat level (future combat
sub-spec)" deferral in the AddXP doc-block. Session-log half of that
sentence stays — it remains deferred independently.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4.8: Verify commit**

Run: `git show --stat HEAD`

Expected: two files changed, ~80 lines diff.

---

## Task 5: Hook into `LoadSave` + retire `NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD` + final gates

**Goal:** Wire `recomputeCombatLevel(false)` into `LoadSave` and retire the load-time deferral. One integration test pins: a loaded SAV with non-fresh baseLevels yields a non-default combat level with no mask flip. Final gates run at end.

**Files:**
- Modify: `modules/world/player_load.go` (`LoadSave` near line 237-243)
- Test: `modules/world/player_load_integration_test.go` (add test alongside existing fixtures)

- [ ] **Step 5.1: Write the failing integration test**

First, inspect the existing test fixtures to use the right helper for producing a SAV with chosen baseLevels:

Run: `grep -nE 'func newTestPlayerForLoadSave|func validSAVBytes' modules/world/player_load_integration_test.go`

Expected output: two function definitions. Both used by the existing tests in this file.

Append to `modules/world/player_load_integration_test.go`:

```go
// TestLoadSave_PopulatesCombatLevel pins that after LoadSave, the
// player's combatLevel is computed from the loaded baseLevels — NOT
// the constructor default of 3. Retires NAI-PLAYERLOADING-D-COMBAT-
// LEVEL-NOT-RECOMPUTED-ON-LOAD. NAI-184 T5.
//
// Uses a SAV produced by Player.Save with all combat stats at level 99
// (CL=126 per the formula). The load-time recompute is the no-rebuild
// variant: MaskAppearance must NOT be flipped (no client yet).
func TestLoadSave_PopulatesCombatLevel(t *testing.T) {
	src, invTypes := newTestPlayerForLoadSave(t)
	// Override all seven combat-stat XPs to level-99 thresholds; LoadSave
	// derives baseLevels from stats[i] via GetLevelByExp.
	for _, stat := range []int{
		objtype.PlayerStatAttack,
		objtype.PlayerStatDefence,
		objtype.PlayerStatStrength,
		objtype.PlayerStatHitpoints,
		objtype.PlayerStatRanged,
		objtype.PlayerStatPrayer,
		objtype.PlayerStatMagic,
	} {
		src.stats[stat] = int32(objtype.GetExpByLevel(99))
	}
	sav := src.Save(invTypes)

	dst := &Player{}
	dst.combatLevel = 3 // constructor default; LoadSave must overwrite it
	dst.masks = 0
	if err := LoadSave(dst, sav, invTypes); err != nil {
		t.Fatalf("LoadSave: %v", err)
	}
	if dst.baseLevels[objtype.PlayerStatStrength] != 99 {
		t.Fatalf("precondition: baseLevels[STR]: got %d, want 99",
			dst.baseLevels[objtype.PlayerStatStrength])
	}
	if dst.combatLevel != 126 {
		t.Errorf("combatLevel: got %d, want 126 (maxed combat stats)", dst.combatLevel)
	}
	if dst.masks&rsbuf.MaskAppearance != 0 {
		t.Errorf("masks: MaskAppearance unexpectedly set by LoadSave (load uses triggerRebuild=false)")
	}
}
```

Ensure the test file's imports include `"github.com/zsrv/goscape/pkg/io/rsbuf"` (grep the existing imports first; add only if absent). `objtype` is already imported per the existing fixtures.

- [ ] **Step 5.2: Run test to verify it fails (RED)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestLoadSave_PopulatesCombatLevel' ./modules/world/... -v`

Expected: FAIL — `combatLevel: got 3, want 126` (LoadSave doesn't recompute today).

- [ ] **Step 5.3: Hook `recomputeCombatLevel(false)` into `LoadSave` and replace the deferral comment**

In `modules/world/player_load.go`, the current code at ~lines 237-243 reads:

```go
	// NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD: TS
	// PlayerLoading.ts:156 recomputes player.combatLevel via
	// getCombatLevel(). Goscape has no equivalent method on Player;
	// combatLevel is set at appearance-rebuild time elsewhere in the
	// tick. Loaded baseLevels propagate to combat level on the next
	// appearance refresh.
	return nil
}
```

Replace with:

```go
	// Recompute combat level from loaded baseLevels — mirrors TS
	// PlayerLoading.ts:156 (player.combatLevel = player.getCombatLevel()).
	// triggerRebuild=false because the client has no appearance state
	// yet; first appearance generation post-login picks up the value.
	p.recomputeCombatLevel(false)
	return nil
}
```

Note: the placement is BEFORE the existing final `return nil`, replacing the multi-line deferral comment.

- [ ] **Step 5.4: Run test to verify it passes (GREEN)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestLoadSave_' ./modules/world/... -v`

Expected: all `TestLoadSave_*` pass.

- [ ] **Step 5.5: Verify the `NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD` tag is no longer referenced in non-test Go**

Run: `grep -rnE 'NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD' modules/ pkg/ cmd/ internal/ --include='*.go' | grep -v _test.go`

Expected: empty.

Run the same grep against `docs/superpowers/` to confirm point-in-time records still mention the tag (they should — don't edit them):

Run: `grep -rnE 'NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD' docs/superpowers/`

Expected: at least one hit in the spec doc — that's fine.

- [ ] **Step 5.6: Full sanity sweep**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1`

Expected: all green.

- [ ] **Step 5.7: Full repo race gate**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

Expected: 57+ packages, 0 FAIL.

If any unrelated package fails, investigate before committing — do NOT mask flakes with retries.

- [ ] **Step 5.8: Smoke-pack gate**

Run: `make smoke-pack` (the project's standard pack-validation gate; see `Makefile`).

Expected: `12 OK / 0 ERR / 0 SKIP`.

If the project's Makefile target name differs, check `Makefile` for `smoke` or `pack` targets and run the equivalent.

- [ ] **Step 5.9: Pre-commit `git status` check**

Run: `git status`

Expected: exactly two modified files (`modules/world/player_load.go`, `modules/world/player_load_integration_test.go`).

- [ ] **Step 5.10: Commit**

```bash
git add modules/world/player_load.go modules/world/player_load_integration_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): LoadSave recomputes combat level (NAI-184 T5 — close)

Wires recomputeCombatLevel(false) into LoadSave before final return.
Loaded baseLevels now propagate to p.combatLevel at load time instead
of leaving the constructor default (3) in place until the next
stat-changing call. Mirrors TS PlayerLoading.ts:156. One integration
test pins: loaded SAV with all-99 combat stats yields CL=126 with
MaskAppearance unset (no client yet).

Retires NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD.

Closes the NAI-184 combat-level recompute slice. Net effect:
- *Player.calcCombatLevel formula (T1)
- *Player.recomputeCombatLevel guarded helper (T2)
- SetStat hook (T3) — retires DEVIATION-NAI-184-D1
- AddXP level-up hook (T4) — retires informal AddXP deferral
- LoadSave hook (T5) — retires NAI-PLAYERLOADING-D-COMBAT-LEVEL-...

Side effects: appearance.go:101 and npc_hunt.go:172-179 now see live
values instead of the stale constructor default of 3 — no code change
at either read site.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5.11: Verify commit + final state**

Run: `git show --stat HEAD`

Expected: two files changed, ~50 lines diff.

Run: `git log --oneline fcb3242f..HEAD`

Expected: exactly 5 new commits (T1, T2, T3, T4, T5).

Run: `git log --oneline 578bf55b..HEAD`

Expected: 6 commits (the spec commit `fcb3242f` + the 5 implementation commits).

---

## Self-Review (run after writing the plan)

**Spec coverage check** — each section of the spec maps to a task:

| Spec section | Task |
| --- | --- |
| §5.1 `calcCombatLevel` formula | T1 |
| §5.1 `recomputeCombatLevel` method | T2 |
| §5.2 SetStat hook | T3 |
| §5.2 AddXP hook | T4 |
| §5.2 LoadSave hook (called `LoadAccount` in spec but actually `LoadSave`) | T5 |
| §5.3 7 formula tests | T1 |
| §5.3 3 recompute method tests | T2 |
| §5.3 SetStat integration (2 tests) | T3 |
| §5.3 AddXP integration (2 tests) | T4 |
| §5.3 LoadSave integration (1 test) | T5 |
| §5.4 retire DEVIATION-NAI-184-D1 | T3 |
| §5.4 retire NAI-PLAYERLOADING-D-... | T5 |
| §5.4 retire informal AddXP deferral | T4 |
| §8 gate plan: -race, smoke-pack, grep | T5 (5.7, 5.8, 5.5) |

Every spec item maps to a step. Spec §7 (out-of-scope items) require no work — they're deliberate non-actions.

**Placeholder scan:** zero "TBD"/"TODO"/"implement later" in the plan body. Every step has either a code block or a literal command with expected output.

**Type/signature consistency:** `calcCombatLevel()` (T1) signature `func (p *Player) calcCombatLevel() int` matches the call in `recomputeCombatLevel` (T2) `newCL := p.calcCombatLevel()`. `recomputeCombatLevel(triggerRebuild bool)` (T2) signature matches the call sites in T3 (`p.recomputeCombatLevel(true)`), T4 (`p.recomputeCombatLevel(true)`), T5 (`p.recomputeCombatLevel(false)`). `SetAppearanceInv(id int)` referenced in T2 — confirmed in player_script.go:827. `rsbuf.MaskAppearance` referenced in T2/T3/T4/T5 — confirmed in T2.3 to match the existing usage at player_script.go:829.

**Spec naming bug surfaced and fixed:** spec used `LoadAccount` for the function, but the real function is `LoadSave`. Plan uses `LoadSave` everywhere. No spec-edit needed; the plan supersedes that detail.

---

## Risks & contingencies

- **Float precision** — the formula uses `float64` intermediate values. All inputs are bounded by 99 so the largest possible intermediate is well under 2^24 (float32 boundary), and float64 has 53 bits of mantissa. No precision concern. If a test ever fails by exactly 1 (off-by-one), check whether the formula is being applied to non-stat values that exceed 99 (would indicate a separate bug).

- **AddXP "no-level-up" branch coverage** — T4's `TestAddXP_NoLevelUp_NoRecompute` would PASS today (no recompute happens in any branch). After T4 lands, the test still PASSES, but its meaning changes: it now pins that the level-up GUARD correctly scopes the recompute to the level-up branch. This is intentional. If a future refactor moves the recompute outside the `afterBase > beforeBase` block, this test catches it.

- **LoadSave test sensitivity to Save format** — T5's `TestLoadSave_PopulatesCombatLevel` round-trips through `Player.Save`. If anyone changes the Save format incompatibly, the test breaks at the precondition assertion (`baseLevels[STR] != 99`) rather than at the assertion under test, which is the right ordering — a Save-format bug surfaces as a precondition failure, not a false NAI-184 regression.

- **Pre-commit safety per `git-pre-commit-status-check`** — every commit step pairs `git status` (pre) with `git show --stat HEAD` (post). If concurrent shell activity stages unexpected files, the post-commit check catches it; recover via `git reset --mixed HEAD~1` (NOT `--amend`).

---

## What this plan does NOT do (explicit non-goals)

Restated from spec §7:
- No combat AI / weapon / attack-calc work.
- No `npc_hunt.go` or `appearance.go` code change (their read sites already exist; the field value just becomes live).
- No change to `newPlayer`'s `combatLevel: 3` constructor default (still load-bearing for test fixtures that bypass LoadSave).
- No `*Npc` combat-level work.
- No session-log / milestone event work (still deferred in the AddXP doc-block as a separate concern).
- No edits to `docs/superpowers/specs/` or `docs/superpowers/plans/` referencing the retired tags — those are point-in-time records.
