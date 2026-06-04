# Hero-points lifecycle clear-site sweep — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the four observably-meaningful TS `heroPoints.clear()` lifecycle call sites that goscape currently omits (NPC respawn + STAT_ADD/BOOST/HEAL HP-full branches) + document the fifth site (Player.cleanup) as an observable no-op deferral.

**Architecture:** Strict TDD, one site per task, five tasks total. Task 1 widens the `ActivePlayer` script interface with `HeroPointsClear()` and lands the real-type + mock implementations (pre-stub: must compile, no observable behavior yet). Tasks 2-4 add HP-full clear tails to `handleStatAdd` / `handleStatBoost` / `handleStatHeal`. Task 5 wires the NPC respawn clear and lands the Player.cleanup doc-comment deferral.

**Tech Stack:** Go 1.26.3; goscape `pkg/script/` (RuneScript interpreter), `modules/world/` (game world / entities). All `go` invocations must be prefixed with `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` (per CLAUDE.md + slice-close memos — the `/home/owner/go/current` symlink points to nonexistent `go1.26.2`).

**Spec:** `docs/superpowers/specs/2026-05-21-hero-points-lifecycle-clear-design.md` (committed at `376ffa68`).

---

## File Structure

**Modified files (5):**
- `pkg/script/active.go` — widen `ActivePlayer` interface with `HeroPointsClear()`
- `pkg/script/runner_test.go` — extend `mockPlayer` with `heroPointsClearCalls` field + method
- `pkg/script/handlers_player.go` — add HP-full clear tails to `handleStatAdd`, `handleStatBoost`, `handleStatHeal`
- `pkg/script/handlers_player_test.go` — 9 new tests (3 per handler)
- `modules/world/player_script.go` — add real `(p *Player) HeroPointsClear()` method
- `modules/world/npc_registry.go` — call `n.heroPoints.Clear()` in `resetEntityForRespawn`
- `modules/world/npc_registry_test.go` — 1 new test for respawn clear
- `modules/world/server.go` — doc-comment-only on `removePlayerInternal` for Player.cleanup deferral

**Sanity test:** added to `modules/world/heropoints_test.go` (Task 1) — 1 test for `(*Player).HeroPointsClear`.

---

## Task 1: Widen `ActivePlayer` interface with `HeroPointsClear()` (pre-stub + impls)

**Files:**
- Modify: `pkg/script/active.go:711-720`
- Modify: `pkg/script/runner_test.go:350-358` and `:782-786`
- Modify: `modules/world/player_script.go:1336-1338`
- Modify: `modules/world/heropoints_test.go` (append new test)

This task is a **pre-stub widening**: the interface gains a new method, both implementations (real `*Player` + mock `*mockPlayer`) gain bodies, and the new mock counter field is added. No production behavior changes — no handler calls the new method yet. The whole repo must still compile + tests pass. Then we add a sanity unit test for the real `(*Player).HeroPointsClear()`.

- [ ] **Step 1: Add `HeroPointsClear()` to `ActivePlayer` interface**

Edit `pkg/script/active.go`. Find the block at lines 717-720:

```go
	// TopContributor returns the playerUID with the largest HeroPoints
	// credit on this player's ledger, or 0 if the ledger is empty. Used
	// by FINDHERO (PlayerOps.ts:1138-1154). NAI-127 Bundle 1.
	TopContributor() int
```

Insert immediately after (still inside the `ActivePlayer` interface block):

```go

	// HeroPointsClear resets the player's hero-point contributor ledger.
	// Called by STAT_ADD / STAT_BOOST / STAT_HEAL on the HP-full branch
	// (PlayerOps.ts:513-515, :552-554, :609-611). Parallel to
	// ActiveNpc.HeroPointsClear (used by NPC_STATHEAL HP-full branch).
	// NAI-120 Bundle 2D follow-up.
	HeroPointsClear()
```

- [ ] **Step 2: Add real impl on `*Player`**

Edit `modules/world/player_script.go`. Find the block at lines 1332-1338:

```go
// TopContributor implements script.ActivePlayer. Returns the playerUID
// with the largest HeroPoints credit, or 0 if the ledger is empty.
// Used by FINDHERO. Mirrors TS state.activePlayer.heroPoints.findHero()
// at PlayerOps.ts:1139.
func (p *Player) TopContributor() int {
	return p.heroPoints.TopContributor()
}
```

Insert immediately after:

```go

// HeroPointsClear implements script.ActivePlayer. Resets the player's
// hero-point contributor ledger. Mirrors TS Player.heroPoints.clear() at
// PlayerOps.ts:513-515, :552-554, :609-611 (HP-full branches of STAT_ADD
// / STAT_BOOST / STAT_HEAL). NAI-120 Bundle 2D follow-up.
func (p *Player) HeroPointsClear() {
	p.heroPoints.Clear()
}
```

- [ ] **Step 3: Add mock counter field**

Edit `pkg/script/runner_test.go`. Find the `topContributor` block at lines 353-358:

```go
	// NAI-127 Bundle 1: FINDHERO ledger-top getter.
	topContributor int

	// NAI-127 Bundle 1: BOTH_HEROPOINTS recipient recorder. Mirrors
	// mockNpc.addHeroPointsCalls.
	addHeroPointsCalls []struct{ playerUID, amount int }
```

Replace with (preserves existing fields, adds new one in-bundle):

```go
	// NAI-127 Bundle 1: FINDHERO ledger-top getter.
	topContributor int

	// NAI-127 Bundle 1: BOTH_HEROPOINTS recipient recorder. Mirrors
	// mockNpc.addHeroPointsCalls.
	addHeroPointsCalls []struct{ playerUID, amount int }

	// NAI-120 Bundle 2D follow-up: HeroPointsClear() call counter.
	// Mirrors mockNpc.heroPointsClearCalls.
	heroPointsClearCalls int
```

- [ ] **Step 4: Add mock method**

Edit `pkg/script/runner_test.go`. Find the `TopContributor` + `AddHeroPoints` block at lines 782-786:

```go
func (m *mockPlayer) TopContributor() int { return m.topContributor }

func (m *mockPlayer) AddHeroPoints(playerUID, amount int) {
	m.addHeroPointsCalls = append(m.addHeroPointsCalls, struct{ playerUID, amount int }{playerUID, amount})
}
```

Replace with (preserves existing methods, adds new one immediately after):

```go
func (m *mockPlayer) TopContributor() int { return m.topContributor }

func (m *mockPlayer) AddHeroPoints(playerUID, amount int) {
	m.addHeroPointsCalls = append(m.addHeroPointsCalls, struct{ playerUID, amount int }{playerUID, amount})
}

// HeroPointsClear increments the call counter. NAI-120 Bundle 2D follow-up.
func (m *mockPlayer) HeroPointsClear() { m.heroPointsClearCalls++ }
```

- [ ] **Step 5: Verify the repo compiles (pre-stub gate)**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache build ./...`

Expected: zero output (success). If there are compile errors mentioning some other type doesn't implement `ActivePlayer`, that means another mock exists somewhere — find it via `grep -rn "TopContributor\|AddHeroPoints" pkg/script/ modules/world/ --include="*.go"` and add `HeroPointsClear() {}` (empty body) on that type too.

- [ ] **Step 6: Verify all tests still pass (pre-stub gate)**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race ./...`

Expected: PASS across all 57+ packages. No new test added yet; this verifies the interface widening is non-breaking.

- [ ] **Step 7: Write the failing sanity test for `(*Player).HeroPointsClear`**

Edit `modules/world/heropoints_test.go`. Append at end of file:

```go

// TestPlayerHeroPointsClear pins the real (*Player).HeroPointsClear()
// method: clears the player's heroPoints ledger so TopContributor()
// returns 0 after the call. Implements script.ActivePlayer.HeroPointsClear.
// NAI-120 Bundle 2D follow-up.
func TestPlayerHeroPointsClear(t *testing.T) {
	p := &Player{heroPoints: NewHeroPoints(16)}
	p.heroPoints.AddHero(1, 5)
	p.heroPoints.AddHero(2, 3)
	if got := p.heroPoints.TopContributor(); got != 1 {
		t.Fatalf("setup: TopContributor() = %d, want 1", got)
	}
	p.HeroPointsClear()
	if got := p.heroPoints.TopContributor(); got != 0 {
		t.Errorf("after HeroPointsClear: TopContributor() = %d, want 0", got)
	}
}
```

If `testing` isn't already imported in this test file, add it. Check the top of the file: `grep -n "^import\|^package" modules/world/heropoints_test.go` — if there's no import block yet (header-only test file), add:

```go
import "testing"
```

- [ ] **Step 8: Run the new test and verify it passes**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race -run TestPlayerHeroPointsClear ./modules/world/`

Expected: `--- PASS: TestPlayerHeroPointsClear` and `ok  ...`. Since the implementation was added in Step 2, this is a GREEN-on-first-run sanity test (the impl already exists). This is fine — Task 1 is a pre-stub widening, not a behavior-change RED→GREEN cycle.

- [ ] **Step 9: Commit**

```bash
git add pkg/script/active.go pkg/script/runner_test.go modules/world/player_script.go modules/world/heropoints_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): expose ActivePlayer.HeroPointsClear() for STAT-handler HP-full clears

Widens the ActivePlayer script interface with HeroPointsClear() and
lands the real *Player implementation + mockPlayer counter. Mirrors
the existing ActiveNpc.HeroPointsClear surface used by NPC_STATHEAL.
Tasks 2-4 will wire this into STAT_ADD / STAT_BOOST / STAT_HEAL.

NAI-120 Bundle 2D follow-up.
EOF
)"
```

---

## Task 2: Wire HP-full clear into `handleStatAdd`

**Files:**
- Modify: `pkg/script/handlers_player.go:342-366` (`handleStatAdd`)
- Modify: `pkg/script/handlers_player_test.go` (append 3 new tests)

Note on test setup: `mockPlayer.SetCurLevel` only **records** the call into `setCurLevelCalls` — it does NOT mutate `m.levels`. So the predicate `s.Self.Stat(PlayerStatHitpoints) >= s.Self.StatBase(PlayerStatHitpoints)` reads from whatever was pre-seeded into `m.levels[3]` and `m.baseLevels[3]`. Pre-seed these to the values the predicate should observe; the formula's computed `added` is captured in `setCurLevelCalls` but does not feed back into the predicate read. This is the same pattern existing tests use (see `TestStatAddFormula` at `handlers_player_test.go:165`).

- [ ] **Step 1: Write failing test — STAT_ADD on HITPOINTS at full HP clears heroPoints**

Edit `pkg/script/handlers_player_test.go`. Find a stable insertion point — append after the existing STAT_ADD tests (after `TestHandleStatAddNullRejected` at `:2627`, but actually adjacent to the formula tests is more idiomatic — search for `TestStatAddCapsAt255` ending and append). To be precise: find the end of `TestStatAddCapsAt255` (closing `}` around line 220) and insert immediately after that closing brace.

```go

// TestStatAddOnHitpointsAtFullClearsHeroPoints pins STAT_ADD's HP-full
// heroPoints.clear() tail (TS PlayerOps.ts:513-515): when stat ==
// HITPOINTS and post-update levels[HITPOINTS] >= baseLevels[HITPOINTS],
// the player's hero-point ledger is cleared. The mock's
// SetCurLevel does not mutate m.levels, so we pre-seed m.levels[3] to
// the value the predicate should observe.
//
// NAI-120 Bundle 2D follow-up.
func TestStatAddOnHitpointsAtFullClearsHeroPoints(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 15     // PlayerStatHitpoints; predicate sees this as post-update
	mp.baseLevels[3] = 15 // HP-full: 15 >= 15

	sf := &ScriptFile{
		Name: "stat_add_hp_full",
		Opcodes: []Opcode{
			OpPushConstantInt, // stat id
			OpPushConstantInt, // constant
			OpPushConstantInt, // percent (top)
			OpStatAdd,
			OpReturn,
		},
		IntOperands:      []int32{3, 1, 0, 0, 0}, // stat=HITPOINTS, const=1, pct=0
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 1 {
		t.Errorf("STAT_ADD HP-full: heroPointsClearCalls = %d, want 1", got)
	}
}
```

- [ ] **Step 2: Write failing test — STAT_ADD on HITPOINTS NOT at full does NOT clear**

Append immediately after the previous test:

```go

// TestStatAddOnHitpointsNotFullSkipsClear pins the HP-not-full negative
// branch: predicate sees levels[3] < baseLevels[3] → no clear.
//
// NAI-120 Bundle 2D follow-up.
func TestStatAddOnHitpointsNotFullSkipsClear(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 10     // post-update HP per predicate read
	mp.baseLevels[3] = 15 // not full: 10 < 15

	sf := &ScriptFile{
		Name: "stat_add_hp_not_full",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatAdd, OpReturn,
		},
		IntOperands:      []int32{3, 1, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 0 {
		t.Errorf("STAT_ADD HP-not-full: heroPointsClearCalls = %d, want 0", got)
	}
}
```

- [ ] **Step 3: Write failing test — STAT_ADD on non-HP stat at "full" HP does NOT clear**

Append immediately after:

```go

// TestStatAddOnNonHitpointsStatSkipsClear pins the stat-gate: when
// stat != HITPOINTS, the clear NEVER fires even if HP happens to be
// full. Mirrors TS PlayerOps.ts:513 gate `stat === PlayerStat.HITPOINTS &&`.
//
// NAI-120 Bundle 2D follow-up.
func TestStatAddOnNonHitpointsStatSkipsClear(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 15     // HP IS full
	mp.baseLevels[3] = 15
	mp.levels[0] = 50     // Attack; the stat being mutated
	mp.baseLevels[0] = 80

	sf := &ScriptFile{
		Name: "stat_add_non_hp",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatAdd, OpReturn,
		},
		IntOperands:      []int32{0, 10, 25, 0, 0}, // stat=ATTACK, const=10, pct=25
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 0 {
		t.Errorf("STAT_ADD non-HP stat: heroPointsClearCalls = %d, want 0", got)
	}
}
```

- [ ] **Step 4: Run the three new tests and verify they all FAIL**

Run:

```
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race -run 'TestStatAddOnHitpointsAtFullClearsHeroPoints|TestStatAddOnHitpointsNotFullSkipsClear|TestStatAddOnNonHitpointsStatSkipsClear' ./pkg/script/
```

Expected:
- `TestStatAddOnHitpointsAtFullClearsHeroPoints`: FAIL — `heroPointsClearCalls = 0, want 1`
- `TestStatAddOnHitpointsNotFullSkipsClear`: PASS — clear isn't wired, so calls=0 matches want=0
- `TestStatAddOnNonHitpointsStatSkipsClear`: PASS — same reason

Only the positive test should fail. The two negatives are GREEN-on-first-run (the absence of the feature trivially satisfies "no clear"). This is fine — they pin the negative behavior so a future regression that fires the clear unconditionally would catch.

- [ ] **Step 5: Add HP-full clear tail to `handleStatAdd`**

Edit `pkg/script/handlers_player.go`. Find `handleStatAdd` at line 342, and update the tail. The current body ends with:

```go
	base := s.Self.StatBase(id)
	cur := s.Self.Stat(id)
	added := cur + (constant + (base*percent)/100)
	if added > 255 {
		added = 255
	}
	s.Self.SetCurLevel(id, added)
	return nil
}
```

Replace the tail starting from `s.Self.SetCurLevel(id, added)` to the closing brace with:

```go
	s.Self.SetCurLevel(id, added)
	// TS PlayerOps.ts:513-515 — when STAT_ADD targets HITPOINTS and the
	// post-update HP meets or exceeds base, clear the heroPoints ledger.
	// Predicate matches TS exactly (>= comparison, stat-gate). NAI-120
	// Bundle 2D follow-up.
	if id == objtype.PlayerStatHitpoints && s.Self.Stat(objtype.PlayerStatHitpoints) >= s.Self.StatBase(objtype.PlayerStatHitpoints) {
		s.Self.HeroPointsClear()
	}
	return nil
}
```

Verify the `objtype` import is already present. Check the top of the file: `grep -n "objtype" pkg/script/handlers_player.go | head`. If not imported, add `"github.com/zsrv/goscape/pkg/objtype"` to the import block (the file likely already imports it — `checkStatID` and other handlers use it). If a stray `imported and not used` error appears, the import is already there and goimports/gofmt will tidy on save.

- [ ] **Step 6: Run the three tests and verify they ALL pass**

Run:

```
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race -run 'TestStatAddOnHitpointsAtFullClearsHeroPoints|TestStatAddOnHitpointsNotFullSkipsClear|TestStatAddOnNonHitpointsStatSkipsClear' ./pkg/script/
```

Expected: all three PASS.

- [ ] **Step 7: Run the full pkg/script suite to verify no regression**

Run:

```
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race ./pkg/script/
```

Expected: PASS across the package, including `TestStatAddFormula` and `TestStatAddCapsAt255` (those don't touch HITPOINTS as `stat`, so the new tail is gated off for them).

- [ ] **Step 8: Refresh `handleStatAdd` doc-comment**

Edit `pkg/script/handlers_player.go` around line 337. The current doc-comment is:

```go
// handleStatAdd implements STAT_ADD.
// TS formula (PlayerOps.ts:501-519):
//
//	added = current + ((constant + (base*percent)/100) | 0)
//	levels[stat] = min(added, 255)
```

Replace with:

```go
// handleStatAdd implements STAT_ADD.
// TS formula (PlayerOps.ts:501-519):
//
//	added = current + ((constant + (base*percent)/100) | 0)
//	levels[stat] = min(added, 255)
//
// Tail (TS PlayerOps.ts:513-515): when stat == HITPOINTS and the
// post-update HP meets or exceeds base, the heroPoints contributor
// ledger is cleared. NAI-120 Bundle 2D follow-up.
```

- [ ] **Step 9: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): port STAT_ADD HP-full heroPoints.clear tail (PlayerOps.ts:513-515)

When STAT_ADD targets HITPOINTS and post-update HP >= base, clear the
active player's hero-point contributor ledger. Three tests pin the
gate (HP-full clears, HP-not-full skips, non-HP-stat skips).

NAI-120 Bundle 2D follow-up.
EOF
)"
```

---

## Task 3: Wire HP-full clear into `handleStatBoost`

**Files:**
- Modify: `pkg/script/handlers_player.go:408-439` (`handleStatBoost`)
- Modify: `pkg/script/handlers_player_test.go` (append 3 new tests)

- [ ] **Step 1: Write failing test — STAT_BOOST on HITPOINTS at full HP clears heroPoints**

Edit `pkg/script/handlers_player_test.go`. Append after the three STAT_ADD HP-full tests added in Task 2:

```go

// TestStatBoostOnHitpointsAtFullClearsHeroPoints pins STAT_BOOST's
// HP-full heroPoints.clear() tail (TS PlayerOps.ts:552-554).
//
// NAI-120 Bundle 2D follow-up.
func TestStatBoostOnHitpointsAtFullClearsHeroPoints(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 15
	mp.baseLevels[3] = 15

	sf := &ScriptFile{
		Name: "stat_boost_hp_full",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatBoost, OpReturn,
		},
		IntOperands:      []int32{3, 0, 0, 0, 0}, // stat=HITPOINTS, const=0, pct=0
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 1 {
		t.Errorf("STAT_BOOST HP-full: heroPointsClearCalls = %d, want 1", got)
	}
}
```

- [ ] **Step 2: Write failing test — STAT_BOOST on HITPOINTS NOT at full does NOT clear**

Append:

```go

// TestStatBoostOnHitpointsNotFullSkipsClear pins the HP-not-full
// negative branch for STAT_BOOST.
//
// NAI-120 Bundle 2D follow-up.
func TestStatBoostOnHitpointsNotFullSkipsClear(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 10
	mp.baseLevels[3] = 15

	sf := &ScriptFile{
		Name: "stat_boost_hp_not_full",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatBoost, OpReturn,
		},
		IntOperands:      []int32{3, 1, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 0 {
		t.Errorf("STAT_BOOST HP-not-full: heroPointsClearCalls = %d, want 0", got)
	}
}
```

- [ ] **Step 3: Write failing test — STAT_BOOST on non-HP stat does NOT clear**

Append:

```go

// TestStatBoostOnNonHitpointsStatSkipsClear pins the stat-gate for
// STAT_BOOST. Mirrors TS PlayerOps.ts:552 gate.
//
// NAI-120 Bundle 2D follow-up.
func TestStatBoostOnNonHitpointsStatSkipsClear(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 15
	mp.baseLevels[3] = 15
	mp.levels[0] = 50
	mp.baseLevels[0] = 80

	sf := &ScriptFile{
		Name: "stat_boost_non_hp",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatBoost, OpReturn,
		},
		IntOperands:      []int32{0, 5, 0, 0, 0}, // stat=ATTACK
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 0 {
		t.Errorf("STAT_BOOST non-HP stat: heroPointsClearCalls = %d, want 0", got)
	}
}
```

- [ ] **Step 4: Run the three new tests and verify positive case FAILS**

Run:

```
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race -run 'TestStatBoostOnHitpointsAtFullClearsHeroPoints|TestStatBoostOnHitpointsNotFullSkipsClear|TestStatBoostOnNonHitpointsStatSkipsClear' ./pkg/script/
```

Expected: positive test FAIL (`want 1`), two negatives PASS (clear isn't wired anywhere new yet).

- [ ] **Step 5: Add HP-full clear tail to `handleStatBoost`**

Edit `pkg/script/handlers_player.go`. Find `handleStatBoost` at line 408. The current body ends with:

```go
	base := s.Self.StatBase(id)
	cur := s.Self.Stat(id)
	boost := constant + (base*percent)/100
	boosted := cur + boost
	if ceiling := base + boost; boosted > ceiling {
		boosted = ceiling
	}
	if boosted < cur {
		boosted = cur
	}
	if boosted > 255 {
		boosted = 255
	}
	s.Self.SetCurLevel(id, boosted)
	return nil
}
```

Replace the tail starting from `s.Self.SetCurLevel(id, boosted)` to the closing brace with:

```go
	s.Self.SetCurLevel(id, boosted)
	// TS PlayerOps.ts:552-554 — same HP-full clear tail as STAT_ADD.
	// NAI-120 Bundle 2D follow-up.
	if id == objtype.PlayerStatHitpoints && s.Self.Stat(objtype.PlayerStatHitpoints) >= s.Self.StatBase(objtype.PlayerStatHitpoints) {
		s.Self.HeroPointsClear()
	}
	return nil
}
```

- [ ] **Step 6: Run the three tests and verify they ALL pass**

Run:

```
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race -run 'TestStatBoostOnHitpointsAtFullClearsHeroPoints|TestStatBoostOnHitpointsNotFullSkipsClear|TestStatBoostOnNonHitpointsStatSkipsClear' ./pkg/script/
```

Expected: all three PASS.

- [ ] **Step 7: Run the full pkg/script suite**

Run:

```
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race ./pkg/script/
```

Expected: PASS across the package.

- [ ] **Step 8: Refresh `handleStatBoost` doc-comment**

Edit `pkg/script/handlers_player.go` around line 399. The current doc-comment is:

```go
// handleStatBoost implements STAT_BOOST.
// TS formula (PlayerOps.ts:538-558):
//
//	boost = (constant + (base*percent)/100) | 0
//	boosted = max(min(current + boost, base + boost), current)
//	levels[stat] = min(boosted, 255)
//
// The max(..., current) clamp means a boost never lowers the stat —
// useful when the stat is already boosted above base + boost.
```

Replace with:

```go
// handleStatBoost implements STAT_BOOST.
// TS formula (PlayerOps.ts:538-558):
//
//	boost = (constant + (base*percent)/100) | 0
//	boosted = max(min(current + boost, base + boost), current)
//	levels[stat] = min(boosted, 255)
//
// The max(..., current) clamp means a boost never lowers the stat —
// useful when the stat is already boosted above base + boost.
//
// Tail (TS PlayerOps.ts:552-554): when stat == HITPOINTS and the
// post-update HP meets or exceeds base, the heroPoints contributor
// ledger is cleared. NAI-120 Bundle 2D follow-up.
```

- [ ] **Step 9: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): port STAT_BOOST HP-full heroPoints.clear tail (PlayerOps.ts:552-554)

Same gate shape as STAT_ADD: stat == HITPOINTS and post-update HP >=
base. Three tests pin the gate.

NAI-120 Bundle 2D follow-up.
EOF
)"
```

---

## Task 4: Wire HP-full clear into `handleStatHeal`

**Files:**
- Modify: `pkg/script/handlers_player.go:481-508` (`handleStatHeal`)
- Modify: `pkg/script/handlers_player_test.go` (append 3 new tests)

- [ ] **Step 1: Write failing test — STAT_HEAL on HITPOINTS at full HP clears heroPoints**

Edit `pkg/script/handlers_player_test.go`. Append after the three STAT_BOOST tests added in Task 3:

```go

// TestStatHealOnHitpointsAtFullClearsHeroPoints pins STAT_HEAL's
// HP-full heroPoints.clear() tail (TS PlayerOps.ts:609-611).
//
// NAI-120 Bundle 2D follow-up.
func TestStatHealOnHitpointsAtFullClearsHeroPoints(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 15
	mp.baseLevels[3] = 15

	sf := &ScriptFile{
		Name: "stat_heal_hp_full",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatHeal, OpReturn,
		},
		IntOperands:      []int32{3, 5, 0, 0, 0}, // stat=HITPOINTS, const=5, pct=0
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 1 {
		t.Errorf("STAT_HEAL HP-full: heroPointsClearCalls = %d, want 1", got)
	}
}
```

- [ ] **Step 2: Write failing test — STAT_HEAL on HITPOINTS NOT at full does NOT clear**

Append:

```go

// TestStatHealOnHitpointsNotFullSkipsClear pins the HP-not-full
// negative branch for STAT_HEAL.
//
// NAI-120 Bundle 2D follow-up.
func TestStatHealOnHitpointsNotFullSkipsClear(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 10
	mp.baseLevels[3] = 15

	sf := &ScriptFile{
		Name: "stat_heal_hp_not_full",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatHeal, OpReturn,
		},
		IntOperands:      []int32{3, 1, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 0 {
		t.Errorf("STAT_HEAL HP-not-full: heroPointsClearCalls = %d, want 0", got)
	}
}
```

- [ ] **Step 3: Write failing test — STAT_HEAL on non-HP stat does NOT clear**

Append:

```go

// TestStatHealOnNonHitpointsStatSkipsClear pins the stat-gate for
// STAT_HEAL. Mirrors TS PlayerOps.ts:609 gate.
//
// NAI-120 Bundle 2D follow-up.
func TestStatHealOnNonHitpointsStatSkipsClear(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 15
	mp.baseLevels[3] = 15
	mp.levels[0] = 50
	mp.baseLevels[0] = 80

	sf := &ScriptFile{
		Name: "stat_heal_non_hp",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatHeal, OpReturn,
		},
		IntOperands:      []int32{0, 5, 0, 0, 0}, // stat=ATTACK
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.heroPointsClearCalls; got != 0 {
		t.Errorf("STAT_HEAL non-HP stat: heroPointsClearCalls = %d, want 0", got)
	}
}
```

- [ ] **Step 4: Run the three new tests and verify positive case FAILS**

Run:

```
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race -run 'TestStatHealOnHitpointsAtFullClearsHeroPoints|TestStatHealOnHitpointsNotFullSkipsClear|TestStatHealOnNonHitpointsStatSkipsClear' ./pkg/script/
```

Expected: positive FAIL, two negatives PASS.

- [ ] **Step 5: Add HP-full clear tail to `handleStatHeal`**

Edit `pkg/script/handlers_player.go`. Find `handleStatHeal` at line 481. The current body ends with:

```go
	base := s.Self.StatBase(id)
	cur := s.Self.Stat(id)
	healed := cur + (constant + (base*percent)/100)
	if healed > base {
		healed = base
	}
	if healed < cur {
		healed = cur
	}
	s.Self.SetCurLevel(id, healed)
	return nil
}
```

Replace the tail starting from `s.Self.SetCurLevel(id, healed)` to the closing brace with:

```go
	s.Self.SetCurLevel(id, healed)
	// TS PlayerOps.ts:609-611 — same HP-full clear tail as STAT_ADD / STAT_BOOST.
	// NAI-120 Bundle 2D follow-up.
	if id == objtype.PlayerStatHitpoints && s.Self.Stat(objtype.PlayerStatHitpoints) >= s.Self.StatBase(objtype.PlayerStatHitpoints) {
		s.Self.HeroPointsClear()
	}
	return nil
}
```

- [ ] **Step 6: Run the three tests and verify they ALL pass**

Run:

```
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race -run 'TestStatHealOnHitpointsAtFullClearsHeroPoints|TestStatHealOnHitpointsNotFullSkipsClear|TestStatHealOnNonHitpointsStatSkipsClear' ./pkg/script/
```

Expected: all three PASS.

- [ ] **Step 7: Run the full pkg/script suite**

Run:

```
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race ./pkg/script/
```

Expected: PASS across the package.

- [ ] **Step 8: Refresh `handleStatHeal` doc-comment**

Edit `pkg/script/handlers_player.go` around line 473. The current doc-comment is:

```go
// handleStatHeal implements STAT_HEAL.
// TS formula (PlayerOps.ts:596-616):
//
//	healed = current + ((constant + (base*percent)/100) | 0)
//	levels[stat] = max(min(healed, base), current)
//
// The max(..., current) clamp means healing never drops the stat below
// its current (boosted) value; min(..., base) caps at base.
```

Replace with:

```go
// handleStatHeal implements STAT_HEAL.
// TS formula (PlayerOps.ts:596-616):
//
//	healed = current + ((constant + (base*percent)/100) | 0)
//	levels[stat] = max(min(healed, base), current)
//
// The max(..., current) clamp means healing never drops the stat below
// its current (boosted) value; min(..., base) caps at base.
//
// Tail (TS PlayerOps.ts:609-611): when stat == HITPOINTS and the
// post-update HP meets or exceeds base, the heroPoints contributor
// ledger is cleared. NAI-120 Bundle 2D follow-up.
```

- [ ] **Step 9: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): port STAT_HEAL HP-full heroPoints.clear tail (PlayerOps.ts:609-611)

Same gate shape as STAT_ADD / STAT_BOOST: stat == HITPOINTS and
post-update HP >= base. Three tests pin the gate.

NAI-120 Bundle 2D follow-up.
EOF
)"
```

---

## Task 5: Clear heroPoints on NPC respawn + document Player.cleanup deferral

**Files:**
- Modify: `modules/world/npc_registry.go:121-174` (`resetEntityForRespawn`)
- Modify: `modules/world/npc_registry_test.go` (append 1 new test)
- Modify: `modules/world/server.go:977-1006` (`removePlayerInternal` — doc-comment only)

- [ ] **Step 1: Write failing test — `resetEntityForRespawn` clears heroPoints**

Edit `modules/world/npc_registry_test.go`. Find a stable insertion point — append after the existing `TestResetEntityForRespawnRevertRaisesChangeTypeMask` block (the closing `}` around line 254). Insert:

```go

// TestResetEntityForRespawnClearsHeroPoints pins the TS Npc.ts:292
// heroPoints.clear() call on the respawn=true branch. The NPC struct
// is reused across respawn cycles, so contributors accumulated before
// death must NOT linger into the next life. NAI-120 Bundle 2D follow-up.
func TestResetEntityForRespawnClearsHeroPoints(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNPC}
	n := newRegisteredNpc(t, s, typ, true)

	n.heroPoints.AddHero(42, 7)
	n.heroPoints.AddHero(99, 3)
	if got := n.heroPoints.TopContributor(); got != 42 {
		t.Fatalf("setup: TopContributor() = %d, want 42", got)
	}

	s.resetEntityForRespawn(n)

	if got := n.heroPoints.TopContributor(); got != 0 {
		t.Errorf("after resetEntityForRespawn: TopContributor() = %d, want 0 (empty ledger)", got)
	}
}
```

- [ ] **Step 2: Run the new test and verify it FAILS**

Run:

```
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race -run TestResetEntityForRespawnClearsHeroPoints ./modules/world/
```

Expected: FAIL — `TopContributor() = 42, want 0`. `resetEntityForRespawn` does not yet clear heroPoints.

- [ ] **Step 3: Add `n.heroPoints.Clear()` call to `resetEntityForRespawn`**

Edit `modules/world/npc_registry.go`. Find the lifecycle-zero cluster at lines 142-145:

```go
	n.queue = nil
	n.waypointIndex = -1
	n.tele = true
	n.huntClock = 0
	n.huntTarget = nil
```

Replace with (adds heroPoints clear adjacent to queue clear, mirroring TS Npc.ts:292-293 which has `this.heroPoints.clear(); this.queue.clear();` adjacent):

```go
	// TS Npc.ts:292 — clear heroPoints contributor ledger on respawn.
	// The Npc struct is reused across respawn cycles; old contributors
	// would otherwise linger into the next life. NAI-120 Bundle 2D
	// follow-up.
	n.heroPoints.Clear()
	n.queue = nil
	n.waypointIndex = -1
	n.tele = true
	n.huntClock = 0
	n.huntTarget = nil
```

- [ ] **Step 4: Run the test and verify it PASSES**

Run:

```
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race -run TestResetEntityForRespawnClearsHeroPoints ./modules/world/
```

Expected: PASS.

- [ ] **Step 5: Run the full modules/world suite**

Run:

```
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race ./modules/world/
```

Expected: PASS. This package is the long pole (~150s); be patient.

- [ ] **Step 6: Refresh `resetEntityForRespawn` doc-comment to mention heroPoints clear**

Edit `modules/world/npc_registry.go`. Find the doc-comment at lines 112-120:

```go
// resetEntityForRespawn applies the TS Npc.resetEntity(true) reseed
// (TS Npc.ts:280-317, respawn=true branch) factored out so addNpc and
// the future revertType refactor (Task 5e) share one definition.
//
// Resets typeId/uid to baseType (with fresh n.typ lookup), reseeds
// all 6 stats from n.typ.Stats, clears queue/waypoints, sets tele +
// CHANGE_TYPE mask, resets hunt fields. Does NOT touch n.x/n.z (the
// caller handles position) or collision flags (the caller handles
// those via gamemap).
```

Replace with:

```go
// resetEntityForRespawn applies the TS Npc.resetEntity(true) reseed
// (TS Npc.ts:280-317, respawn=true branch) factored out so addNpc and
// the future revertType refactor (Task 5e) share one definition.
//
// Resets typeId/uid to baseType (with fresh n.typ lookup), reseeds
// all 6 stats from n.typ.Stats, clears heroPoints (TS Npc.ts:292) +
// queue/waypoints, sets tele + CHANGE_TYPE mask, resets hunt fields.
// Does NOT touch n.x/n.z (the caller handles position) or collision
// flags (the caller handles those via gamemap).
```

- [ ] **Step 7: Add Player.cleanup deferral doc-comment to `removePlayerInternal`**

Edit `modules/world/server.go`. Find `removePlayerInternal` at line 977:

```go
// removePlayerInternal performs the slot/zone/playerLoop cleanup for p.
// Must only be called from the tick goroutine.
//
// Callers should use removePlayerOnTick or removePlayerOnDisconnect,
// which add the appropriate gRPC-side cleanup before invoking this.
func (s *Server) removePlayerInternal(p *Player) {
```

Replace with:

```go
// removePlayerInternal performs the slot/zone/playerLoop cleanup for p.
// Must only be called from the tick goroutine.
//
// Callers should use removePlayerOnTick or removePlayerOnDisconnect,
// which add the appropriate gRPC-side cleanup before invoking this.
//
// TS Player.cleanup at Engine-TS/src/engine/entity/Player.ts:446 calls
// player.heroPoints.clear() as part of cleanup. goscape omits the
// call: newPlayer (player.go:506) allocates a fresh *Player per login
// with a fresh NewHeroPoints(16) (player.go:544), so clearing the
// about-to-be-GC'd ledger has no observable effect. Informal English
// deferral (no NAI-XXX-D pin); precedent set by combat sub-spec
// framing cleanup (2026-05-20). NAI-120 Bundle 2D follow-up.
func (s *Server) removePlayerInternal(p *Player) {
```

- [ ] **Step 8: Verify the full repo builds and all tests pass**

Run:

```
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race ./...
```

Expected: PASS across all 57+ packages. ~155s total runtime is normal.

- [ ] **Step 9: Verify TestPackAll_TwelveStageSmoke**

Run:

```
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/go GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache test -race -run TestPackAll_TwelveStageSmoke ./...
```

Expected: PASS.

- [ ] **Step 10: Audit-grep sweep — verify TS citations land where expected**

Run:

```
grep -rn "heroPoints\.clear\|heroPoints.Clear" pkg/script/ modules/world/ --include="*.go"
```

Expected hits (production-side, ignore _test.go):
- `modules/world/heropoints.go` — existing `Clear()` method definition
- `modules/world/npc_script.go` — existing `(n *Npc) HeroPointsClear()` calls `n.heroPoints.Clear()`
- `modules/world/npc_registry.go` — NEW `n.heroPoints.Clear()` call in `resetEntityForRespawn` (Task 5)
- `modules/world/player_script.go` — NEW `(p *Player) HeroPointsClear()` calls `p.heroPoints.Clear()` (Task 1)
- `modules/world/server.go` — NEW doc-comment reference in `removePlayerInternal` (Task 5)

Then run:

```
grep -rn "PlayerOps\.ts:513\|PlayerOps\.ts:552\|PlayerOps\.ts:609\|Npc\.ts:292\|Player\.ts:446" pkg/script/ modules/world/ --include="*.go"
```

Expected: at least 1 hit per TS line citation (production-side doc-comments from Tasks 2-5). Three for PlayerOps citations (one per handler + one in active.go interface doc), one for Npc.ts:292 (npc_registry.go doc-comment + resetEntityForRespawn body), one for Player.ts:446 (server.go deferral).

- [ ] **Step 11: Commit**

```bash
git add modules/world/npc_registry.go modules/world/npc_registry_test.go modules/world/server.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): clear heroPoints on NPC respawn (Npc.ts:292) + document Player.cleanup no-op deferral

NPC struct is reused across respawn cycles, so contributor ledger must
clear or stale entries linger into the next life. Player.cleanup site
(TS Player.ts:446) is observably a no-op in goscape since *Player is
allocated fresh per login; informal English deferral on
removePlayerInternal.

NAI-120 Bundle 2D follow-up.
EOF
)"
```

---

## Slice Close

After Task 5 commits cleanly, the slice is logically complete. Carry-forward bookkeeping (memory update, MEMORY.md prepend, resume memo) is handled by the finishing-a-development-branch skill at slice close — not by this plan.

Final gate state expected:
- `-race ./...` 57+ pkgs, 0 FAIL (~155s)
- `TestPackAll_TwelveStageSmoke` PASS
- 5 commits on top of spec commit `376ffa68` (Tasks 1-5, one commit each)
- 0 deviation pins opened or retired (informal English deferral on Player.cleanup is not a formal pin)
- Carry-forward menu loses "Hero-points consumption"; gains nothing new
