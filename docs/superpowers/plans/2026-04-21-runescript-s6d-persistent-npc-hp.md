# RuneScript S6d: Persistent NPC HP Storage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Promote NPC `curHP` and `baseHP` from per-tick ephemeral fields (sentinel `-1`, cleared by `ResetMasks`) to persistent NPC state seeded from `NpcType.Stats[NpcStatHitpoints]` at construction. Drop the lazy baseHP init from `*Npc.Damage`. Add public `*Npc.ResetHP()` for future respawn paths. Export the stat-index constants from `pkg/objtype`.

**Architecture:** Two tasks. Task 1 is a rename — export `NpcStat*` constants from `pkg/objtype/npctype.go` and update the one internal use. Task 2 is the behavior change — `NewNpc` seeds HP, `ResetMasks` stops clearing HP, `Damage` simplifies, `ResetHP` helper added, tests updated. Task 1 doesn't affect behavior; Task 2 is where real semantics shift.

**Tech Stack:** Go; `pkg/objtype` config loader; `modules/world` NPC entity.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s6d-persistent-npc-hp-design.md`](../specs/2026-04-21-runescript-s6d-persistent-npc-hp-design.md) (commit `2a9f53b`)

---

## Task 1: Export `NpcStat*` constants from `pkg/objtype`

**Files:**
- Modify: `pkg/objtype/npctype.go` (capitalize 6 const names; update 1 internal reference)

- [ ] **Step 1: Rename the six constants.** Open `pkg/objtype/npctype.go`. Find the const block at lines 11-19:

```go
// NPC stat indices (attack, defence, strength, hitpoints, ranged, magic).
const (
	npcStatAttack    = 0
	npcStatDefence   = 1
	npcStatStrength  = 2
	npcStatHitpoints = 3
	npcStatRanged    = 4
	npcStatMagic     = 5
)
```

Replace with:

```go
// NpcStat* are indices into NpcType.Stats for combat-relevant attributes
// (attack, defence, strength, hitpoints, ranged, magic). Exported so that
// modules/world and other callers can reference stat slots by name rather
// than magic index.
const (
	NpcStatAttack    = 0
	NpcStatDefence   = 1
	NpcStatStrength  = 2
	NpcStatHitpoints = 3
	NpcStatRanged    = 4
	NpcStatMagic     = 5
)
```

- [ ] **Step 2: Update the one internal use site.** In the same file, find line 155:

```go
		t.Stats[npcStatHitpoints] = dat.G2()
```

Replace with:

```go
		t.Stats[NpcStatHitpoints] = dat.G2()
```

- [ ] **Step 3: Confirm no other internal references.** Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache grep -n "npcStat" pkg/objtype/npctype.go
```

Expected: empty output. If any remaining lowercase references surface, capitalize them the same way.

- [ ] **Step 4: Build + test.** The rename is behavior-neutral; existing tests should pass unchanged.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/objtype/
```

All clean. `gofmt -l` output empty.

- [ ] **Step 5: Commit.**

```bash
git add pkg/objtype/npctype.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(objtype): export NpcStat* indices

Promotes npcStatAttack/Defence/Strength/Hitpoints/Ranged/Magic from
unexported package-local constants to exported names so modules/world
and future sub-specs can reference stat slots without duplicating the
magic index 3. Zero behavior change.

Part of S6d (persistent NPC HP); retires the S6c follow-up for magic
index 3 at all call sites.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA + green build/test.

---

## Task 2: NewNpc seeds HP + ResetMasks + Damage simplification + ResetHP + tests

**Files:**
- Modify: `modules/world/npc.go` (NewNpc seeds HP; add `initialHP` helper)
- Modify: `modules/world/npc_masks.go` (drop lazy baseHP from Damage; slim ResetMasks; add ResetHP)
- Modify: `modules/world/npc_masks_test.go` (update helper comment; add 6 new tests)

- [ ] **Step 1: Write the failing tests FIRST.** Open `modules/world/npc_masks_test.go`. Update the helper's comment first (no behavior change):

```go
// npcWithHP builds an Npc whose NpcType.Stats[NpcStatHitpoints] = maxHP,
// then overrides curHP if needed. NewNpc seeds both curHP and baseHP from
// Stats[NpcStatHitpoints] as of S6d, so the override is only meaningful
// when the caller wants a starting curHP distinct from max.
func npcWithHP(t *testing.T, maxHP, curHP int) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "rat"},
	}
	typ.Stats = []uint16{0, 0, 0, uint16(maxHP), 0, 0}
	npc := NewNpc(0, 7, 3222, 3218, 0, typ)
	npc.curHP = curHP
	return npc
}
```

Then append the 6 new tests at the end of the file:

```go
func TestNewNpcSeedsHPFromStats(t *testing.T) {
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	typ.Stats = []uint16{0, 0, 0, 20, 0, 0} // NpcStatHitpoints = 3
	npc := NewNpc(0, 7, 3222, 3218, 0, typ)
	if npc.curHP != 20 {
		t.Errorf("curHP: got %d, want 20", npc.curHP)
	}
	if npc.baseHP != 20 {
		t.Errorf("baseHP: got %d, want 20", npc.baseHP)
	}
}

func TestNewNpcWithEmptyStatsSeedsZeroHP(t *testing.T) {
	// &NpcType{} has nil Stats, so initialHP returns 0.
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	npc := NewNpc(0, 7, 3222, 3218, 0, typ)
	if npc.curHP != 0 || npc.baseHP != 0 {
		t.Errorf("curHP/baseHP: got %d/%d, want 0/0", npc.curHP, npc.baseHP)
	}
}

func TestNpcDamagePersistsAcrossResetMasks(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(3, 1)
	if npc.curHP != 7 {
		t.Fatalf("pre-reset curHP: got %d, want 7", npc.curHP)
	}
	npc.ResetMasks()
	if npc.curHP != 7 {
		t.Errorf("post-reset curHP: got %d, want 7 (persistent)", npc.curHP)
	}
	if npc.damageAmt != -1 {
		t.Errorf("damageAmt: got %d, want -1 (per-tick cleared)", npc.damageAmt)
	}
}

func TestNpcBaseHPPersistsAcrossResetMasks(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(3, 1)
	npc.ResetMasks()
	if npc.baseHP != 10 {
		t.Errorf("post-reset baseHP: got %d, want 10 (persistent)", npc.baseHP)
	}
}

func TestNpcResetHP(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(7, 1)
	if npc.curHP != 3 {
		t.Fatalf("pre-reset curHP: got %d, want 3", npc.curHP)
	}
	npc.ResetHP()
	if npc.curHP != 10 {
		t.Errorf("curHP after ResetHP: got %d, want 10", npc.curHP)
	}
	if npc.baseHP != 10 {
		t.Errorf("baseHP after ResetHP: got %d, want 10", npc.baseHP)
	}
}

func TestNpcResetHPWithNilTypDirectConstruction(t *testing.T) {
	// NewNpc would panic on nil typ (PatrolCoord / WanderRange / etc.), so
	// build *Npc directly to exercise the initialHP nil-guard path via
	// ResetHP. Any future caller that manually constructs an Npc must
	// survive ResetHP cleanly.
	npc := &Npc{}
	npc.ResetHP()
	if npc.curHP != 0 || npc.baseHP != 0 {
		t.Errorf("after ResetHP on nil-typ npc: got %d/%d, want 0/0", npc.curHP, npc.baseHP)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNewNpcSeedsHPFromStats|TestNewNpcWithEmptyStatsSeedsZeroHP|TestNpcDamagePersistsAcrossResetMasks|TestNpcBaseHPPersistsAcrossResetMasks|TestNpcResetHP|TestNpcResetHPWithNilTypDirectConstruction" -v
```

Expected: builds fail with `npc.ResetHP undefined` plus the HP-seeding tests fail because `NewNpc` still initializes to `-1`.

- [ ] **Step 3: Seed HP at construction in `modules/world/npc.go`.** Find the `NewNpc` struct literal around lines 88-125. Replace:

```go
		curHP:           -1,
		baseHP:          -1,
```

with:

```go
		curHP:           initialHP(typ),
		baseHP:          initialHP(typ),
```

Then add the helper below `NewNpc` (before the next function definition):

```go
// initialHP returns the max HP stored in an NpcType, defaulting to 0 when
// typ is nil or Stats doesn't cover the Hitpoints slot. Called from NewNpc
// (to seed curHP + baseHP) and from *Npc.ResetHP.
func initialHP(typ *objtype.NpcType) int {
	if typ == nil || len(typ.Stats) <= objtype.NpcStatHitpoints {
		return 0
	}
	hp := int(typ.Stats[objtype.NpcStatHitpoints])
	if hp < 0 {
		return 0
	}
	return hp
}
```

- [ ] **Step 4: Slim `ResetMasks` and simplify `Damage` in `modules/world/npc_masks.go`.** First, find and replace the existing `Damage` method:

```go
// Damage applies `amount` damage of `dmgType` to the NPC this tick, flagging
// NpcMaskDamage so the NPC-info encoder emits the hitsplat. curHP decrements
// by amount (clamped at 0). On overkill (amount > curHP), the emitted
// damageAmt is clamped to the pre-hit curHP so the client shows only damage
// actually dealt — matches TS Npc.applyDamage (Npc.ts:472-485). Negative
// amount is coerced to 0 defensively so a script bug cannot heal the NPC.
//
// baseHP is seeded at NPC construction (NewNpc) and refilled by ResetHP;
// Damage no longer touches it. curHP is persistent state (S6d); scripts
// calling NPC_STAT(0) on later ticks see real decremented HP.
//
// This method is a pure output op — no death / auto-retaliate / aggro logic.
// Scripts that need death handling should check NPC_STAT(0) and fire their
// own despawn flow. The AI sub-spec will later ship a real combat loop.
func (n *Npc) Damage(amount, dmgType int) {
	if amount < 0 {
		amount = 0
	}
	prevHP := n.curHP
	if amount > prevHP {
		n.damageAmt = prevHP
	} else {
		n.damageAmt = amount
	}
	n.damageType = dmgType
	n.curHP -= amount
	if n.curHP < 0 {
		n.curHP = 0
	}
	n.masks |= rsbuf.NpcMaskDamage
}
```

Note: the old method had an additional `n.baseHP < 0 && ... { ... baseHP = hp ... }` lazy-seed block after the curHP clamp — that block is removed entirely.

Second, find and replace `ResetMasks`. The current shape (before this edit):

```go
func (n *Npc) ResetMasks() {
	n.masks = 0
	n.sayText = nil
	n.damageAmt = -1
	n.damageType = -1
	n.curHP = -1
	n.baseHP = -1
	n.spotanimID = -1
	n.spotanimHeight = -1
	n.spotanimDelay = -1
}
```

Replace with:

```go
// ResetMasks clears mask bits + ephemeral per-tick state. Persistent fields
// (animID, faceEntity, faceSquareX/Z, changeTypeID, curHP, baseHP) are
// retained across ticks — S6d promoted curHP/baseHP from ephemeral to
// persistent. damageAmt / damageType remain per-tick hitsplat payload.
func (n *Npc) ResetMasks() {
	n.masks = 0
	n.sayText = nil
	n.damageAmt = -1
	n.damageType = -1
	n.spotanimID = -1
	n.spotanimHeight = -1
	n.spotanimDelay = -1
}
```

Third, add the new `ResetHP` method at the end of the file:

```go
// ResetHP re-seeds curHP + baseHP from the NPC's current NpcType.Stats
// Hitpoints slot. Called by respawn paths (on NPC death-and-respawn) and by
// AI sub-spec code that needs to restore max HP on some trigger. Safe on
// nil typ (leaves both at 0).
func (n *Npc) ResetHP() {
	hp := initialHP(n.typ)
	n.curHP = hp
	n.baseHP = hp
}
```

- [ ] **Step 5: Run tests to verify they pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNewNpcSeedsHPFromStats|TestNewNpcWithEmptyStatsSeedsZeroHP|TestNpcDamagePersistsAcrossResetMasks|TestNpcBaseHPPersistsAcrossResetMasks|TestNpcResetHP|TestNpcResetHPWithNilTypDirectConstruction" -v
```

All 6 PASS.

- [ ] **Step 6: Confirm S6c HP tests still pass (they should, given `npcWithHP` still works identically).**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNpcDamage" -v
```

Expected: `TestNpcDamageDecrementsHPAndSetsMask`, `TestNpcDamageClampsAtZero`, `TestNpcDamageNegativeAmountClampsToZero` all still PASS. The S6c tests don't assert specific post-ResetMasks curHP values, so the semantic change is invisible to them.

- [ ] **Step 7: Run the full repo suite and quality checks.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l modules/world/
```

All PASS / empty. If `gofmt -l` flags a file you touched (`npc.go` or `npc_masks.go` or `npc_masks_test.go`), run `gofmt -w` on just that file. Do NOT sweep pre-existing drift elsewhere.

- [ ] **Step 8: Commit.**

```bash
git add modules/world/npc.go modules/world/npc_masks.go modules/world/npc_masks_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): S6d persistent NPC HP — seed at ctor, survive ResetMasks

Promotes *Npc.curHP and *Npc.baseHP from per-tick ephemeral (-1
sentinel, cleared each tick) to persistent state. NewNpc seeds both
from NpcType.Stats[NpcStatHitpoints] via a new initialHP helper;
ResetMasks no longer touches them. *Npc.Damage drops the lazy baseHP
init it used to need. New *Npc.ResetHP public method for future
respawn / AI paths.

Scripts calling NPC_STAT(0) on tick N+1 after damage on tick N now
see real persistent current HP.

Closes the S6c final-review "biggest owed debt."

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
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/objtype/ modules/world/` empty
- [ ] `grep -rn 'npcStat' pkg/objtype/ modules/world/` returns nothing (all lowercase refs gone)
- [ ] Two commits on main: refactor(objtype) export + feat(world) persistent HP
- [ ] Spec requirements covered:
  - [ ] NpcStat* constants exported → Task 1
  - [ ] NewNpc seeds curHP + baseHP from Stats[NpcStatHitpoints] → Task 2
  - [ ] ResetMasks stops clearing curHP / baseHP → Task 2
  - [ ] *Npc.Damage drops lazy baseHP init → Task 2
  - [ ] *Npc.ResetHP public method → Task 2
  - [ ] Persistence tests across ResetMasks → Task 2
  - [ ] Nil-typ safety for initialHP + ResetHP → Task 2
