# RuneScript S6e: Persistent Player HP (TS-Faithful) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the duplicated `Player.curHP` / `Player.baseHP` fields. Route current and base HP through the existing skill arrays `levels[PlayerStatHitpoints]` and `baseLevels[PlayerStatHitpoints]` — matching TS `Player.applyDamage` and the wire encoder. Add `Player.Damage` and `Player.ResetHP` methods. Delete test-only `Player.ShowHit`. Seed Hitpoints to 10 at login (closes a pre-existing missing-init bug).

**Architecture:** Two tasks. Task 1 ships the prerequisites: export `PlayerStatHitpoints` constant from `pkg/objtype` and seed `baseLevels[Hitpoints]=10, levels[Hitpoints]=10` in `processLogins`. The build stays passing throughout — no behavior change yet. Task 2 is the field-deletion sweep + new methods + test rewrites in one cohesive commit (delete fields → rewrite getters → delete ShowHit → add Damage + ResetHP → slim ResetMasks → rewrite ShowHit-using tests → add 7 new tests).

**Tech Stack:** Go; `pkg/objtype` config constants; `modules/world` Player entity.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s6e-persistent-player-hp-design.md`](../specs/2026-04-21-runescript-s6e-persistent-player-hp-design.md) (commit `40fb1c8`)

---

## Task 1: Export `PlayerStatHitpoints` + seed Hitpoints in `processLogins`

**Files:**
- Modify: `pkg/objtype/npctype.go` (add `PlayerStatHitpoints` constant)
- Modify: `modules/world/tick.go` (seed `levels[3]` and `baseLevels[3]` in `processLogins`)
- Modify: `modules/world/tick_logins_test.go` (or wherever existing login tests live; create `tick_test.go` if no suitable file exists) — new test asserting login seeds Hitpoints to 10

- [ ] **Step 1: Write the failing login-seed test.** Find an existing login-related test file under `modules/world/`. Search:

```bash
grep -lE 'processLogins|TestProcessLogin' modules/world/*_test.go
```

If a `tick_logins_test.go` or similar exists, append the new test there. Otherwise, create `modules/world/tick_logins_test.go` with package header:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)
```

Then in the chosen file, append:

```go
func TestProcessLoginsSeedsHitpoints(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	s.newPlayers = []*Player{p}
	s.processLogins()
	if p.baseLevels[objtype.PlayerStatHitpoints] != 10 {
		t.Errorf("baseLevels[Hitpoints]: got %d, want 10",
			p.baseLevels[objtype.PlayerStatHitpoints])
	}
	if p.levels[objtype.PlayerStatHitpoints] != 10 {
		t.Errorf("levels[Hitpoints]: got %d, want 10",
			p.levels[objtype.PlayerStatHitpoints])
	}
}
```

- [ ] **Step 2: Run the test to verify it fails.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessLoginsSeedsHitpoints -v
```

Expected: FAIL — either at build (`undefined: objtype.PlayerStatHitpoints`) or at runtime (the assertions fail because `processLogins` doesn't seed HP yet, so `levels[3]` and `baseLevels[3]` are both 0).

- [ ] **Step 3: Add `PlayerStatHitpoints` to `pkg/objtype/npctype.go`.** Open the file. Find the existing `NpcStat*` block (lines 11-19, after Task S6d Task 1's export). Append immediately after it:

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

- [ ] **Step 4: Seed Hitpoints in `processLogins`.** Open `modules/world/tick.go`. Find the `processLogins` per-player loop (around lines 79-120). After `p.invs = map[int]*inventory.Inventory{}` (around line 94) and before the `p.masks |= MaskAppearance` line (around line 106), insert:

```go
		// Seed Hitpoints to 10 (RS2 default starting HP) before any code
		// reads p.levels[PlayerStatHitpoints]. Matches TS PlayerLoading.ts:49-51.
		// Full skill initialization (all 21 skills with persisted XP) is a
		// future sub-spec; S6e covers Hitpoints only because the persistent-HP
		// design requires it.
		p.baseLevels[objtype.PlayerStatHitpoints] = 10
		p.levels[objtype.PlayerStatHitpoints] = 10
```

If `objtype` is not already imported in `tick.go`, add `"github.com/zsrv/goscape/pkg/objtype"` to the imports block.

- [ ] **Step 5: Run the test to verify it passes.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessLoginsSeedsHitpoints -v
```

PASS.

- [ ] **Step 6: Run full repo + quality checks.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/objtype/ modules/world/
```

All clean. `gofmt -l` empty for `pkg/objtype/npctype.go` and `modules/world/tick.go` and the test file. (Pre-existing drift in other files is not your concern here.)

- [ ] **Step 7: Commit.**

```bash
git add pkg/objtype/npctype.go modules/world/tick.go modules/world/tick_logins_test.go
# (replace the third path with the actual test file you wrote to)
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world,objtype): seed Player Hitpoints at login + export constant

Adds objtype.PlayerStatHitpoints = 3 and seeds both
p.baseLevels[Hitpoints] and p.levels[Hitpoints] to 10 in
processLogins, matching TS PlayerLoading.ts:49-51. Pre-existing
missing-init bug — invisible until S6e removes the duplicated
curHP/baseHP fields and Player.CurHP() starts reading from the
skill array.

Prerequisite for S6e Task 2 which deletes the duplicated state.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA + green test status.

---

## Task 2: Delete duplicated HP fields + add Damage/ResetHP + rewrite tests

**Files:**
- Modify: `modules/world/player.go` (delete `curHP, baseHP` fields and their `-1` init)
- Modify: `modules/world/player_source.go` (rewrite `CurHP()` + `BaseHP()` getters to derive)
- Modify: `modules/world/player_masks.go` (delete `ShowHit`; slim `ResetMasks`; add `Damage` + `ResetHP`)
- Modify: `modules/world/player_masks_test.go` (rewrite `TestShowHitSetsMask` + `TestResetMasksClearsEphemerals`; add 7 new tests)

This is a coordinated sweep — partial application breaks the build (deleting curHP fields without also deleting `ShowHit` produces a compile error). All edits land in one commit.

- [ ] **Step 1: Write the failing tests FIRST.** Open `modules/world/player_masks_test.go`. Replace the existing `TestShowHitSetsMask` (lines 42-51) with a `Damage`-based version, and update `TestResetMasksClearsEphemerals` (lines 72-91) to use `Damage` instead of `ShowHit`. Apply the following edits:

Replace `TestShowHitSetsMask` body with:

```go
func TestPlayerDamageDecrementsHitpointsAndSetsMask(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 50
	p.Damage(10, 1)
	if p.masks&rsbuf.MaskDamage == 0 {
		t.Error("MaskDamage bit should be set")
	}
	if p.damageAmt != 10 {
		t.Errorf("damageAmt: got %d, want 10", p.damageAmt)
	}
	if p.damageType != 1 {
		t.Errorf("damageType: got %d, want 1", p.damageType)
	}
	if p.levels[3] != 40 {
		t.Errorf("levels[3]: got %d, want 40", p.levels[3])
	}
	if p.baseLevels[3] != 50 {
		t.Errorf("baseLevels[3]: got %d, want 50 (unchanged)", p.baseLevels[3])
	}
}
```

Replace `TestResetMasksClearsEphemerals` body with:

```go
func TestResetMasksClearsEphemerals(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 50
	p.Say([]byte("hi"))
	p.Animate(123, 5)
	p.Damage(10, 1)
	p.ResetMasks()
	if p.masks != 0 {
		t.Errorf("masks: got %d, want 0", p.masks)
	}
	if p.sayText != nil {
		t.Error("sayText should be nil after reset")
	}
	if p.damageAmt != -1 {
		t.Errorf("damageAmt: got %d, want -1", p.damageAmt)
	}
	// Persistent (animID, faceEntity, levels[3], baseLevels[3]) should stay.
	if p.animID != 123 {
		t.Errorf("animID should persist: got %d", p.animID)
	}
	if p.levels[3] != 40 {
		t.Errorf("levels[3] should persist after ResetMasks (S6e): got %d, want 40", p.levels[3])
	}
}
```

Then append the 7 new tests at the end of the file:

```go
func TestPlayerDamageClampsAtZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 10
	p.levels[3] = 2
	p.Damage(5, 1)
	if p.levels[3] != 0 {
		t.Errorf("levels[3]: got %d, want 0 (clamped)", p.levels[3])
	}
	// damageAmt clamps to pre-hit current — matches TS Player.applyDamage
	// (Player.ts:1865-1867: hitmarkDamage = current on overkill).
	if p.damageAmt != 2 {
		t.Errorf("damageAmt: got %d, want 2 (clamped to pre-hit current)", p.damageAmt)
	}
}

func TestPlayerDamageNegativeAmountClampsToZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 50
	p.Damage(-3, 1)
	if p.levels[3] != 50 {
		t.Errorf("levels[3]: got %d, want 50 (negative amount must not heal)", p.levels[3])
	}
	if p.damageAmt != 0 {
		t.Errorf("damageAmt: got %d, want 0 (negative clamped)", p.damageAmt)
	}
	if p.masks&rsbuf.MaskDamage == 0 {
		t.Error("MaskDamage bit should still be set on zero damage (debug signal)")
	}
}

func TestPlayerHPPersistsAcrossResetMasks(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 50
	p.Damage(3, 1)
	if p.levels[3] != 47 {
		t.Fatalf("pre-reset levels[3]: got %d, want 47", p.levels[3])
	}
	p.ResetMasks()
	if p.levels[3] != 47 {
		t.Errorf("post-reset levels[3]: got %d, want 47 (persistent)", p.levels[3])
	}
	if p.damageAmt != -1 {
		t.Errorf("damageAmt: got %d, want -1 (per-tick cleared)", p.damageAmt)
	}
}

func TestPlayerResetHP(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 30
	p.ResetHP()
	if p.levels[3] != 50 {
		t.Errorf("levels[3] after ResetHP: got %d, want 50", p.levels[3])
	}
	if p.baseLevels[3] != 50 {
		t.Errorf("baseLevels[3]: got %d, want 50 (unchanged)", p.baseLevels[3])
	}
}

func TestPlayerCurHPAndBaseHPDeriveFromLevels(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 35
	if got := p.CurHP(); got != 35 {
		t.Errorf("CurHP(): got %d, want 35", got)
	}
	if got := p.BaseHP(); got != 50 {
		t.Errorf("BaseHP(): got %d, want 50", got)
	}
}

func TestPlayerDamageWithBoostedHitpoints(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 10
	p.levels[3] = 14 // boosted via brew/etc
	p.Damage(3, 1)
	if p.levels[3] != 11 {
		t.Errorf("boosted levels[3] after Damage: got %d, want 11", p.levels[3])
	}
	if p.damageAmt != 3 {
		t.Errorf("damageAmt: got %d, want 3", p.damageAmt)
	}
	if p.baseLevels[3] != 10 {
		t.Errorf("baseLevels[3]: got %d, want 10 (boost doesn't touch base)", p.baseLevels[3])
	}
	// ResetHP restores to base, not boosted-max — RS2 respawn semantics.
	p.ResetHP()
	if p.levels[3] != 10 {
		t.Errorf("levels[3] after ResetHP: got %d, want 10 (restored to base)", p.levels[3])
	}
}

func TestPlayerDamageOnZeroHP(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 10
	p.levels[3] = 0
	p.Damage(5, 1)
	if p.levels[3] != 0 {
		t.Errorf("levels[3]: got %d, want 0 (already dead, stays dead)", p.levels[3])
	}
	if p.damageAmt != 0 {
		t.Errorf("damageAmt: got %d, want 0 (clamped to current=0)", p.damageAmt)
	}
	if p.masks&rsbuf.MaskDamage == 0 {
		t.Error("MaskDamage bit should still flip on zero damage")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPlayerDamage|TestPlayerHPPersists|TestPlayerResetHP|TestPlayerCurHPAndBaseHP|TestResetMasksClears" -v
```

Expected: FAIL at build (`p.Damage undefined (type *Player has no field or method Damage)` plus `p.ResetHP undefined`). The `TestShowHitSetsMask` rename also breaks any test runner that was looking for the old name — fine, we're replacing it.

- [ ] **Step 3: Delete `Player.curHP` and `Player.baseHP` fields.** Open `modules/world/player.go`. Find line 194:

```go
	curHP, baseHP         int
```

Delete this entire line. Then find lines 330-331 in `newPlayer`:

```go
		curHP:          -1,
		baseHP:         -1,
```

Delete both lines from the struct literal.

- [ ] **Step 4: Rewrite `CurHP()` and `BaseHP()` getters.** Open `modules/world/player_source.go`. Find lines 32-33:

```go
func (p *Player) CurHP() int          { return p.curHP }
func (p *Player) BaseHP() int         { return p.baseHP }
```

Replace with:

```go
func (p *Player) CurHP() int          { return int(p.levels[objtype.PlayerStatHitpoints]) }
func (p *Player) BaseHP() int         { return int(p.baseLevels[objtype.PlayerStatHitpoints]) }
```

If `objtype` is not already imported in `player_source.go`, add `"github.com/zsrv/goscape/pkg/objtype"` to the imports block.

- [ ] **Step 5: Update `Player.ResetMasks` and `Player.ShowHit` in `player_masks.go`.** Open `modules/world/player_masks.go`.

First, **delete** the entire `ShowHit` method (lines 24-30):

```go
func (p *Player) ShowHit(amount, dmgType, cur, base int) {
	p.damageAmt = amount
	p.damageType = dmgType
	p.curHP = cur
	p.baseHP = base
	p.masks |= rsbuf.MaskDamage
}
```

Second, in `ResetMasks` (lines 65-85), **delete** these two lines:

```go
	p.curHP = -1
	p.baseHP = -1
```

Update `ResetMasks`'s doc-comment to acknowledge the change. The full method should end up as:

```go
// ResetMasks clears mask bits and ephemeral mask state for the next tick.
// Persistent fields (animID, faceEntity, faceSquareX/Z, levels[Hitpoints],
// baseLevels[Hitpoints]) retained — S6e promoted Player HP from per-tick
// ephemeral to persistent, routed through the skill arrays. Also clears
// one-shot movement intents (tele, jump) so a single-tick teleport
// emission doesn't repeat next tick.
func (p *Player) ResetMasks() {
	p.masks = 0
	p.tele = false
	p.jump = false
	p.sayText = nil
	p.chatBytes = nil
	p.damageAmt = -1
	p.damageType = -1
	p.spotanimID = -1
	p.spotanimHeight = -1
	p.spotanimDelay = -1
	p.exactStartX = -1
	p.exactStartZ = -1
	p.exactEndX = -1
	p.exactEndZ = -1
	p.exactBegin = -1
	p.exactFinish = -1
	p.exactDir = -1
}
```

- [ ] **Step 6: Add `Player.Damage` and `Player.ResetHP` to `player_masks.go`.** In the same file, append after the (now-modified) `ResetMasks`:

```go
// Damage applies `amount` damage of `dmgType` to the player this tick,
// flagging MaskDamage so the player-info encoder emits the hitsplat. HP
// decrements via levels[Hitpoints] (the single source of truth — no
// separate curHP field as of S6e). On overkill (amount > current HP),
// damageAmt clamps to the pre-hit HP so the wire shows only damage
// actually dealt — matches TS Player.applyDamage (Player.ts:1860-1873).
//
// Negative amount coerces to 0 defensively. This deviates from TS where
// negative amount would heal the player (current - (-3) = current + 3
// passes the overkill check and writes back). The TS path is almost
// certainly an unintended consequence of unsigned-input assumptions; we
// match the *Npc.Damage convention from S6c instead.
//
// This is a pure output op — no death / auto-retaliate / aggro logic.
// Death/respawn/regen belong in a future combat sub-spec.
func (p *Player) Damage(amount, dmgType int) {
	if amount < 0 {
		amount = 0
	}
	current := int(p.levels[objtype.PlayerStatHitpoints])
	p.damageAmt = min(amount, current)
	p.damageType = dmgType
	next := current - amount
	if next < 0 {
		next = 0
	}
	p.levels[objtype.PlayerStatHitpoints] = uint8(next)
	p.masks |= rsbuf.MaskDamage
}

// ResetHP restores levels[Hitpoints] to baseLevels[Hitpoints] — the
// player's "full HP" state. Called by respawn paths and certain script
// triggers in future sub-specs. Boost/drain effects on Hitpoints are
// wiped (RS2 convention: respawn fills to base, not boosted-max).
//
// No direct TS counterpart — TS performs HP refill inline within death
// handling. This Go-side helper makes the intent reusable.
func (p *Player) ResetHP() {
	p.levels[objtype.PlayerStatHitpoints] = p.baseLevels[objtype.PlayerStatHitpoints]
}
```

If `objtype` is not already imported in `player_masks.go`, add `"github.com/zsrv/goscape/pkg/objtype"` to the imports block.

- [ ] **Step 7: Run the new tests + previously-rewritten tests.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPlayerDamage|TestPlayerHPPersists|TestPlayerResetHP|TestPlayerCurHPAndBaseHP|TestResetMasksClears" -v
```

All 9 tests pass (7 new + 2 rewrites).

- [ ] **Step 8: Sweep for other consumers of the deleted fields.** The deletion may have broken something outside the files touched. Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache grep -rn 'p\.curHP\|p\.baseHP' modules/ pkg/
```

Both should return no errors / no matches. If `grep` finds straggling references to `p.curHP` or `p.baseHP` (e.g., another test file or a script handler), each must be rewritten to `p.levels[objtype.PlayerStatHitpoints]` / `p.baseLevels[objtype.PlayerStatHitpoints]` before the build will succeed.

Also check for `ShowHit` callers outside `player_masks_test.go`:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache grep -rn '\.ShowHit(' modules/ pkg/
```

Should return nothing (or only doc comments referencing the deleted method). If a real caller surfaces, rewrite it to use `Damage` with appropriate level-array setup.

- [ ] **Step 9: Run full repo + quality checks.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l modules/world/
```

All PASS / clean. `gofmt -l` empty for files you touched (`player.go`, `player_source.go`, `player_masks.go`, `player_masks_test.go`). Don't sweep pre-existing drift in other files.

- [ ] **Step 10: Commit.**

```bash
git add modules/world/player.go modules/world/player_source.go modules/world/player_masks.go modules/world/player_masks_test.go
# If Step 8 turned up other files that needed updates, add them too.
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): S6e persistent Player HP via skill array (TS-faithful)

Eliminates the duplicated Player.curHP / Player.baseHP fields by
routing current and base HP through levels[PlayerStatHitpoints] and
baseLevels[PlayerStatHitpoints] — matching TS Player.applyDamage
(Player.ts:1860-1873) and the wire encoder (World.ts:1023-1024).

Player.CurHP() and Player.BaseHP() now derive from the skill arrays.
ResetMasks no longer touches HP. New Player.Damage(amount, dmgType)
mirrors TS applyDamage (with the same defensive negative-clamp
deviation as *Npc.Damage from S6c). New Player.ResetHP() helper for
future respawn paths. Test-only Player.ShowHit deleted; tests
rewritten to use Damage with the same wire-byte intent.

Healing potions, regen, stat boost/drain now affect HP correctly
without any extra wiring — they already write p.levels[3].

Closes the parallel anti-pattern flagged by S6d's review.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA + full-repo green + race + vet + gofmt clean. Note any straggling-consumer fixes from Step 8.

---

## Self-Review Checklist

After both tasks complete:

- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` green
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/objtype/ modules/world/` empty (or only flags pre-existing drift you didn't touch)
- [ ] `grep -rn 'p\.curHP\|p\.baseHP' modules/ pkg/` returns no matches (the duplicated fields are fully retired)
- [ ] `grep -rn '\.ShowHit(' modules/ pkg/` returns no matches (legacy API gone)
- [ ] Two commits on main: T1 prerequisite (constant + login seed), T2 main behavior (deletion sweep + new methods + tests)
- [ ] Spec coverage:
  - [ ] PlayerStatHitpoints exported → T1
  - [ ] processLogins seeds Hitpoints to 10 → T1
  - [ ] Player.curHP/baseHP fields deleted → T2
  - [ ] CurHP()/BaseHP() derive from skill arrays → T2
  - [ ] Player.ShowHit deleted → T2
  - [ ] Player.Damage method added (TS-faithful overkill clamp + defensive negative-clamp) → T2
  - [ ] Player.ResetHP helper added → T2
  - [ ] ResetMasks no longer touches HP → T2
  - [ ] Existing ShowHit-using tests rewritten → T2
  - [ ] 7 new behavior tests → T2
  - [ ] Login seed test → T1
