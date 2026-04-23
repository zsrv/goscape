# NAI-15 — `checkVars` + `checkNotCombat` + Outer Combat Guard — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close two of the three deferred varp-dependent filters in `huntPlayers` (TS `Npc.ts:942-957`) by porting the outer multi-zone/target guard, `checkNotCombat`, `checkVars`, and the supporting `HuntType.CheckHuntCondition` operator evaluator.

**Architecture:** Single-package surgery in `modules/world/npc_hunt.go` for the filter chain; single helper method added to `pkg/objtype/hunttype.go`; one small test-only setter added to `pkg/gamemap/multimap.go` to enable multi-zone fixture construction from cross-package tests. No new files. `checkNotCombatSelf` stays deferred pending NPC-vars infra (recorded as Task 5's memory-update deliverable).

**Tech Stack:** Go 1.26+. No new packages. Existing `pkg/objtype` (`HuntType`, `HuntCheckVar`), `pkg/gamemap` (`IsMulti`), `modules/world` (`Player.Varp`, `Server.currentTick`, `Npc.target`).

**Spec**: `docs/superpowers/specs/2026-04-23-nai-15-varp-filter-bundle-design.md`.

---

## File Structure

| File | Role | Task |
|------|------|------|
| `pkg/objtype/hunttype.go` | + `CheckHuntCondition` method on `*HuntType` | 1 |
| `pkg/objtype/hunttype_test.go` | + `TestHuntTypeCheckHuntCondition` (table-driven, 5 rows) | 1 |
| `modules/world/npc_hunt.go` | Insert `checkVars` filter between CheckVis and the final append | 2 |
| `modules/world/npc_hunt_test.go` | + `TestHuntPlayersCheckVars` (6 sub-cases) | 2 |
| `pkg/gamemap/multimap.go` | + `SetMulti(x, z, level int, multi bool)` test-infra setter | 3 |
| `pkg/gamemap/gamemap_test.go` | + `TestSetMulti` (1 sanity assertion) | 3 |
| `modules/world/npc_hunt.go` | Insert outer combat guard + `checkNotCombat` filter; rewrite deferred-filter comment block | 4 |
| `modules/world/npc_hunt_test.go` | + `TestHuntPlayersCheckNotCombat` (5 sub-cases) + `TestHuntPlayersCombatGuard` (5 sub-cases) | 4 |
| `~/.claude/projects/.../memory/nai_followups.md` | Add resolution preamble to "From NAI-8 → Deferred filters" entry | 5 |

No new files. No packages added.

---

## Task 1: Port `HuntType.CheckHuntCondition`

**Files:**
- Modify: `pkg/objtype/hunttype.go` (+ one method at end of file)
- Test: `pkg/objtype/hunttype_test.go` (+ one test function)

Rationale: port TS `HuntType.checkHuntCondition` at `Engine-TS/src/cache/config/HuntType.ts:63-75`. Prerequisite for Task 2 (`checkVars` calls it). Method on `*HuntType` matches TS class shape.

- [ ] **Step 1: Write the failing test**

Append to `pkg/objtype/hunttype_test.go`:

```go
func TestHuntTypeCheckHuntCondition(t *testing.T) {
	ht := &HuntType{}

	cases := []struct {
		name      string
		value     int
		condition string
		check     int
		want      bool
	}{
		{name: "greater-than-true", value: 5, condition: ">", check: 3, want: true},
		{name: "less-than-false", value: 5, condition: "<", check: 3, want: false},
		{name: "equal-true", value: 7, condition: "=", check: 7, want: true},
		{name: "not-equal-true", value: 7, condition: "!", check: 8, want: true},
		{name: "unknown-operator-false", value: 5, condition: "??", check: 5, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ht.CheckHuntCondition(tc.value, tc.condition, tc.check); got != tc.want {
				t.Errorf("CheckHuntCondition(%d, %q, %d): got %v, want %v",
					tc.value, tc.condition, tc.check, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestHuntTypeCheckHuntCondition -v`

Expected: BUILD FAIL with `ht.CheckHuntCondition undefined (type *HuntType has no field or method CheckHuntCondition)`.

- [ ] **Step 3: Implement the method**

Append to `pkg/objtype/hunttype.go` (after the `LoadHuntTypes` / `parseHuntTypes` functions):

```go
// CheckHuntCondition evaluates condition against (value, checkValue) using the
// hunt-config operator string. Mirrors TS HuntType.checkHuntCondition at
// Engine-TS/src/cache/config/HuntType.ts:63-75. Unknown operators return
// false (TS default-case behavior — fail-closed for malformed hunt data).
//
// Used by huntPlayers's CheckVars filter (TS Npc.ts:950-957) and, once
// inventory infra lands, CheckInv (TS Npc.ts:959-969).
func (t *HuntType) CheckHuntCondition(value int, condition string, checkValue int) bool {
	switch condition {
	case ">":
		return value > checkValue
	case "<":
		return value < checkValue
	case "=":
		return value == checkValue
	case "!":
		return value != checkValue
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestHuntTypeCheckHuntCondition -v`

Expected: `--- PASS` for all 5 sub-tests (`greater-than-true`, `less-than-false`, `equal-true`, `not-equal-true`, `unknown-operator-false`).

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/hunttype.go pkg/objtype/hunttype_test.go
git commit --no-gpg-sign -m "feat(objtype): NAI-15 Task 1 add HuntType.CheckHuntCondition

Port TS HuntType.checkHuntCondition at
Engine-TS/src/cache/config/HuntType.ts:63-75. Operator switch on
>, <, =, ! with fail-closed default-case for unknown operators.
Consumed by huntPlayers's CheckVars filter in NAI-15 Task 2; future
CheckInv port reuses the same helper.

Table-driven TestHuntTypeCheckHuntCondition covers all 4 operators
plus the unknown-operator default case."
```

---

## Task 2: Port `checkVars` filter in `huntPlayers`

**Files:**
- Modify: `modules/world/npc_hunt.go:107-161` (insert before the final `hunted = append` at the current line 158)
- Test: `modules/world/npc_hunt_test.go` (+ `TestHuntPlayersCheckVars` after the existing `TestHuntPlayersCheckVisArgumentOrderSwapQuirk` at line 421)

Rationale: port TS `Npc.ts:950-957` — the `CheckVars` AND-chain. Each entry either short-circuits to pass (`VarID == -1`) or evaluates `CheckHuntCondition(Varp(VarID), Condition, Val)`. Nil/empty CheckVars → no-op (zero iterations match TS `[].every() === true`).

- [ ] **Step 1: Write the failing test**

Append to `modules/world/npc_hunt_test.go`:

```go
// TestHuntPlayersCheckVars guards the CheckVars AND-chain filter at
// TS Npc.ts:950-957. Each entry passes if VarID==-1 OR
// CheckHuntCondition(p.Varp(VarID), Condition, Val). Any failing entry
// excludes the player. Nil/empty CheckVars → no-op.
func TestHuntPlayersCheckVars(t *testing.T) {
	setup := func(t *testing.T, varps []int32) (*Server, *Npc, *Player) {
		t.Helper()
		s := newServerForScriptTest(t)
		n := newNpcForLifecycleTest(t)
		n.server = s
		n.x, n.z, n.level = 3094, 3106, 0
		n.huntRange = 10
		p := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
		p.varps = varps
		return s, n, p
	}

	t.Run("nil-checkvars-no-filter", func(t *testing.T) {
		_, n, _ := setup(t, []int32{0})
		hunt := &objtype.HuntType{} // CheckVars nil
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (nil CheckVars → no filter)", len(hunted))
		}
	})

	t.Run("single-entry-passes", func(t *testing.T) {
		_, n, _ := setup(t, []int32{5})
		hunt := &objtype.HuntType{CheckVars: []objtype.HuntCheckVar{
			{VarID: 0, Condition: ">", Val: 3},
		}}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (5 > 3 → pass)", len(hunted))
		}
	})

	t.Run("single-entry-fails", func(t *testing.T) {
		_, n, _ := setup(t, []int32{5})
		hunt := &objtype.HuntType{CheckVars: []objtype.HuntCheckVar{
			{VarID: 0, Condition: ">", Val: 10},
		}}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (5 > 10 → fail)", len(hunted))
		}
	})

	t.Run("two-entries-both-pass", func(t *testing.T) {
		_, n, _ := setup(t, []int32{5, 7})
		hunt := &objtype.HuntType{CheckVars: []objtype.HuntCheckVar{
			{VarID: 0, Condition: ">", Val: 3},
			{VarID: 1, Condition: "=", Val: 7},
		}}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (both pass)", len(hunted))
		}
	})

	t.Run("two-entries-second-fails", func(t *testing.T) {
		_, n, _ := setup(t, []int32{5, 7})
		hunt := &objtype.HuntType{CheckVars: []objtype.HuntCheckVar{
			{VarID: 0, Condition: ">", Val: 3}, // pass
			{VarID: 1, Condition: "=", Val: 9}, // fail
		}}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (AND-fail: first passes, second fails)", len(hunted))
		}
	})

	t.Run("varid-minus-one-short-circuits", func(t *testing.T) {
		_, n, _ := setup(t, []int32{5})
		hunt := &objtype.HuntType{CheckVars: []objtype.HuntCheckVar{
			// VarID == -1 must pass without reading any varp, regardless of
			// the Condition/Val. TS Npc.ts:953 `checkVar.varId === -1 ||` short-circuit.
			{VarID: -1, Condition: ">", Val: 999},
			{VarID: 0, Condition: ">", Val: 3}, // real gate: 5 > 3 → pass
		}}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (VarID=-1 entry skipped, second passes)", len(hunted))
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHuntPlayersCheckVars -v`

Expected: 4 of 6 sub-tests FAIL (`single-entry-fails`, `two-entries-second-fails`, and wherever else the filter is silently ignored — `single-entry-passes` + `varid-minus-one-short-circuits` + `two-entries-both-pass` + `nil-checkvars-no-filter` currently pass coincidentally because the filter is absent and the player is always hunted). Confirm at least one sub-test fails with `got 1, want 0`.

- [ ] **Step 3: Implement the filter in `huntPlayers`**

Edit `modules/world/npc_hunt.go`. Locate the CheckVis Line-of-Walk block ending at line 157 (`continue` of the LoW branch) and the `hunted = append(hunted, p)` at line 158. Insert the new filter between them:

```go
		if hunt.CheckVis == objtype.HuntVisLineOfWalk && s.gamemap != nil &&
			!s.gamemap.Pathfinder.LineValidator.HasLineOfWalk(
				n.level, p.x, p.z, n.x, n.z, 1, 1, 1, 0) {
			continue
		}

		// checkVars (TS:950-957): AND-chain of varp/operator/value predicates.
		// Nil/empty CheckVars → no-op (ranging nil slice yields zero iterations,
		// matching TS empty-`every` → true semantics).
		passCheckVars := true
		for _, cv := range hunt.CheckVars {
			if cv.VarID == -1 {
				// TS:953 `checkVar.varId === -1 ||` short-circuit.
				continue
			}
			if !hunt.CheckHuntCondition(int(p.Varp(cv.VarID)), cv.Condition, cv.Val) {
				passCheckVars = false
				break
			}
		}
		if !passCheckVars {
			continue
		}

		hunted = append(hunted, p)
	}
	return hunted
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHuntPlayersCheckVars -v`

Expected: `--- PASS` for all 6 sub-tests. Then run the broader hunt-test group to verify no regression: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHuntPlayers -v`. Expected: all existing tests still pass (including the NAI-8 and NAI-12 tests in `npc_event_queue_test.go` and `npc_hunt_test.go`).

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_hunt.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "feat(world): NAI-15 Task 2 checkVars filter in huntPlayers

Port TS Npc.ts:950-957 — AND-chain over hunt.CheckVars entries.
Each entry passes when VarID==-1 (TS short-circuit) OR
CheckHuntCondition(int(p.Varp(VarID)), Condition, Val). First
failing entry excludes the player. Nil/empty CheckVars ranges to
zero iterations, matching TS empty-every-true semantics without
an explicit nil guard.

TestHuntPlayersCheckVars covers nil-slice default, single-pass,
single-fail, two-pass, AND-fail (second entry fails), and
VarID=-1 short-circuit."
```

---

## Task 3: Add `SetMulti` test-infra setter to `pkg/gamemap/multimap.go`

**Files:**
- Modify: `pkg/gamemap/multimap.go` (+ one method)
- Test: `pkg/gamemap/gamemap_test.go` (+ `TestSetMulti` sanity check)

Rationale: `multimap` is an unexported `map[int]bool` populated by `loadCsvMap` via `Init`. Task 4's `TestHuntPlayersCombatGuard` needs to set multi-zone state at a specific coord without invoking the full `Init` pipeline (which also loads mapsquares, CRCs, NPC spawns, etc. — heavy and fragile for a unit test). A small exported setter keeps the test fixture minimal and honest.

- [ ] **Step 1: Write the failing test**

Append to `pkg/gamemap/gamemap_test.go`:

```go
func TestSetMulti(t *testing.T) {
	gm := New(discardLogger())
	if gm.IsMulti(3094, 3107, 0) {
		t.Fatalf("pre-set: IsMulti(3094,3107,0) = true, want false (fresh GameMap)")
	}
	gm.SetMulti(3094, 3107, 0, true)
	if !gm.IsMulti(3094, 3107, 0) {
		t.Errorf("post-set: IsMulti(3094,3107,0) = false, want true")
	}
	// Different coord must remain unaffected.
	if gm.IsMulti(3094, 3108, 0) {
		t.Errorf("adjacent coord: IsMulti(3094,3108,0) = true, want false")
	}
	// Clearing works.
	gm.SetMulti(3094, 3107, 0, false)
	if gm.IsMulti(3094, 3107, 0) {
		t.Errorf("post-clear: IsMulti(3094,3107,0) = true, want false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run TestSetMulti -v`

Expected: BUILD FAIL with `gm.SetMulti undefined (type *GameMap has no field or method SetMulti)`.

- [ ] **Step 3: Implement the setter**

Append to `pkg/gamemap/multimap.go`:

```go
// SetMulti marks (or clears) the given tile as multi-combat. Intended for
// tests — production data flows from multiway.csv via Init. Exposing a
// setter avoids having to stand up a tempdir + CSV + Init for every
// cross-package test that needs a single multi-combat coord.
func (gm *GameMap) SetMulti(x, z, level int, multi bool) {
	gm.multimap[packZoneCoord(x, z, level)] = multi
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run TestSetMulti -v`

Expected: `--- PASS: TestSetMulti`. Then verify no regressions in the package: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/`. Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/gamemap/multimap.go pkg/gamemap/gamemap_test.go
git commit --no-gpg-sign -m "feat(gamemap): NAI-15 Task 3 add SetMulti test-infra setter

Expose a small setter on *GameMap for cross-package tests that need
multi-combat state at a specific coord. Production data still flows
from multiway.csv via Init; the setter only exists so NAI-15 Task 4
(TestHuntPlayersCombatGuard) can wire IsMulti=true at one coord
without spinning up the full Init pipeline.

TestSetMulti asserts pre-set false, post-set true, adjacent-coord
isolation, and post-clear false."
```

---

## Task 4: Port outer combat guard + `checkNotCombat` filter + rewrite deferred-filter comment

**Files:**
- Modify: `modules/world/npc_hunt.go:85-106` (rewrite the deferred-filter doc comment) + insert new filter block between CheckVis LoW (line 157 post-Task-2) and the `checkVars` block (which Task 2 placed ahead of the `append`)
- Test: `modules/world/npc_hunt_test.go` (+ `TestHuntPlayersCheckNotCombat` and `TestHuntPlayersCombatGuard`)

Rationale: ports TS `Npc.ts:942-945` — the outer multi-zone/target-equality guard plus `checkNotCombat`. `checkNotCombatSelf` (TS:946-948) remains deferred because it reads an NPC-side varp via `Npc.getVar` / `VarNpcType`, which has no Go analogue.

The filter ORDER in the filter chain after this task, bottom-up, will be: CheckVis → **outer combat guard → checkNotCombat** → **checkVars** → `append`. Outer-guard precedes checkVars in TS order (TS:942 before TS:950).

- [ ] **Step 1: Write `TestHuntPlayersCheckNotCombat`**

Append to `modules/world/npc_hunt_test.go`:

```go
// TestHuntPlayersCheckNotCombat guards the 8-tick combat-window filter
// at TS Npc.ts:943-945. When the outer guard applies (see
// TestHuntPlayersCombatGuard), a player whose last-combat varp was
// written within [currentTick-7, currentTick] is filtered; at
// currentTick-8 and earlier, they pass.
func TestHuntPlayersCheckNotCombat(t *testing.T) {
	// Helper: build a Server/Npc/Player with gamemap+non-multi guard so
	// the outer combat guard APPLIES (i.e., the checkNotCombat filter
	// actually runs). varpVal seeds p.varps[0].
	setup := func(t *testing.T, currentTick int, varpVal int32) (*Server, *Npc, *Player) {
		t.Helper()
		s := newServerForScriptTest(t)
		s.gamemap = gamemap.New(discardLogger())
		s.currentTick = currentTick
		n := newNpcForLifecycleTest(t)
		n.server = s
		n.x, n.z, n.level = 3094, 3106, 0
		n.huntRange = 10
		n.target = nil // guard applies (target != p) → filter fires
		p := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
		p.varps = []int32{varpVal}
		return s, n, p
	}

	t.Run("default-minus-one-disables", func(t *testing.T) {
		_, n, _ := setup(t, 100, 100) // varp written this tick
		hunt := &objtype.HuntType{CheckNotCombat: -1}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (CheckNotCombat=-1 disables filter)", len(hunted))
		}
	})

	t.Run("varp-this-tick-excluded", func(t *testing.T) {
		_, n, _ := setup(t, 100, 100) // 100+8 > 100 → fire
		hunt := &objtype.HuntType{CheckNotCombat: 0}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (varp==currentTick → within 8-tick window)", len(hunted))
		}
	})

	t.Run("varp-minus-seven-excluded", func(t *testing.T) {
		_, n, _ := setup(t, 100, 93) // 93+8 = 101 > 100 → fire
		hunt := &objtype.HuntType{CheckNotCombat: 0}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (varp==currentTick-7 → window-inclusive, filter fires)", len(hunted))
		}
	})

	t.Run("varp-minus-eight-included", func(t *testing.T) {
		_, n, _ := setup(t, 100, 92) // 92+8 = 100, 100 > 100 is false → pass
		hunt := &objtype.HuntType{CheckNotCombat: 0}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (varp==currentTick-8 → exclusive boundary, filter passes)", len(hunted))
		}
	})

	t.Run("varp-zero-well-past-window-included", func(t *testing.T) {
		_, n, _ := setup(t, 100, 0) // fresh player, no combat recorded
		hunt := &objtype.HuntType{CheckNotCombat: 0}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (varp==0, currentTick=100 → well past window)", len(hunted))
		}
	})
}
```

- [ ] **Step 2: Write `TestHuntPlayersCombatGuard`**

Append to `modules/world/npc_hunt_test.go` (immediately after the previous test):

```go
// TestHuntPlayersCombatGuard guards the outer multi-zone/target-equality
// guard at TS Npc.ts:942. When the guard is SKIPPED (target==p OR
// IsMulti(p) returns true), the inner combat filters (checkNotCombat
// and the deferred checkNotCombatSelf) do NOT run — even if they would
// otherwise fire. When the guard APPLIES, the combat filters run.
//
// All sub-cases use a CheckNotCombat setup that WOULD filter the player
// (varp written at currentTick) to make the guard's effect observable.
func TestHuntPlayersCombatGuard(t *testing.T) {
	// Helper: setup with gamemap but WITHOUT multi-combat set; Test mutates.
	setup := func(t *testing.T) (*Server, *Npc, *Player) {
		t.Helper()
		s := newServerForScriptTest(t)
		s.gamemap = gamemap.New(discardLogger())
		s.currentTick = 100
		n := newNpcForLifecycleTest(t)
		n.server = s
		n.x, n.z, n.level = 3094, 3106, 0
		n.huntRange = 10
		n.target = nil
		p := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
		p.varps = []int32{100} // varp==currentTick → filter would fire if guard applies
		return s, n, p
	}

	hunt := &objtype.HuntType{CheckNotCombat: 0}

	t.Run("target-equals-player-skips-guard", func(t *testing.T) {
		_, n, p := setup(t)
		n.target = p // guard SKIPPED (target == p)
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (target==p → guard skipped, filter does not fire)", len(hunted))
		}
	})

	t.Run("ismulti-true-skips-guard", func(t *testing.T) {
		s, n, p := setup(t)
		s.gamemap.SetMulti(p.x, p.z, p.level, true) // guard SKIPPED (multi-combat zone)
		hunted := n.huntPlayers(s, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (IsMulti=true → guard skipped, filter does not fire)", len(hunted))
		}
	})

	t.Run("target-nil-applies-guard", func(t *testing.T) {
		_, n, _ := setup(t)
		n.target = nil
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (target==nil → guard applies, filter fires)", len(hunted))
		}
	})

	t.Run("target-is-different-player-applies-guard", func(t *testing.T) {
		s, n, _ := setup(t)
		other := addPlayerToServer(t, s, 2, n.x+5, n.z+5, n.level)
		n.target = other
		hunted := n.huntPlayers(s, hunt)
		// The candidate (slot 1) is filtered; the other (slot 2) is the
		// NPC's current target AND passes the combat guard-skip, so it
		// bypasses the combat filter. Expected: exactly slot 2 returned.
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (target=other → candidate filtered, other passes)", len(hunted))
		}
		if hunted[0].Slot() != other.slot {
			t.Errorf("hunted[0]: got slot %d, want slot %d (the target-player)", hunted[0].Slot(), other.slot)
		}
	})

	t.Run("gamemap-nil-applies-guard", func(t *testing.T) {
		_, n, _ := setup(t)
		n.server.gamemap = nil // fidelity: nil gamemap treats as not-multi
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (gamemap==nil → guard applies, filter fires)", len(hunted))
		}
	})
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHuntPlayersCheckNotCombat|TestHuntPlayersCombatGuard' -v`

Expected: FAIL in the varp-within-window and target-nil / gamemap-nil sub-tests, because `huntPlayers` does not yet filter on `CheckNotCombat`. Sub-tests that expect "player included" pass coincidentally (no filter → always included). Confirm at least 4 sub-test failures across the two test functions.

- [ ] **Step 4: Rewrite the deferred-filter comment block**

Edit `modules/world/npc_hunt.go`. Replace the block at lines 83-106 (from `// huntPlayers iterates ...` through `// NAI-8 dispatches NO scripts. ...`) with:

```go
// huntPlayers iterates the player grid in huntRange and returns
// players passing the filter chain. Matches TS Npc.huntPlayers at
// Engine-TS/.../Npc.ts:921-973.
//
// Filter coverage:
//   - Range + level match:     always
//   - checkAfk                 (NAI-8,  TS:935-937)
//   - CheckVis LoS/LoW         (NAI-12, TS per ScriptIterators.ts:88-94)
//   - Outer combat guard       (NAI-15, TS:942)
//   - checkNotCombat           (NAI-15, TS:943-945)
//   - checkVars                (NAI-15, TS:950-957)
//
// CheckVis (NAI-12) preserves the TS player-as-source / NPC-as-dest
// argument swap quirk — see FIDELITY note at the gate below.
//
// Filters DEFERRED (infra missing; each TS line cited):
//   - checkNotBusy             (TS:931-933)       — no Player.Busy()
//   - checkNotTooStrong        (TS:939-941)       — wilderness + combat-level
//   - checkNotCombatSelf       (TS:946-948)       — needs NPC-vars infra
//                                                   (VarNpcType, Npc.vars, Npc.Varp)
//   - checkInv                 (TS:959-969)       — inventory queries
//
// NAI-8 dispatches NO scripts. TS huntPlayers is a config-driven
// filter pipeline, not a script runner.
```

- [ ] **Step 5: Implement the outer guard + `checkNotCombat` filter**

Edit `modules/world/npc_hunt.go`. Locate the CheckVis LoW block (ending with `continue` after the `HasLineOfWalk` check) and the `checkVars` block Task 2 added (starts with `// checkVars (TS:950-957)`). Insert the combat guard + `checkNotCombat` BETWEEN them, so the final post-CheckVis sequence reads:

```go
		if hunt.CheckVis == objtype.HuntVisLineOfWalk && s.gamemap != nil &&
			!s.gamemap.Pathfinder.LineValidator.HasLineOfWalk(
				n.level, p.x, p.z, n.x, n.z, 1, 1, 1, 0) {
			continue
		}

		// Outer combat guard — TS:942. Only when the candidate is not the
		// NPC's current target AND not in a multi-combat zone.
		// FIDELITY: gamemap == nil mirrors the CheckVis short-circuit above —
		// treat as not-multi so the guard applies and combat filters fire.
		applyCombatGuard := entity(p) != n.target &&
			(s.gamemap == nil || !s.gamemap.IsMulti(p.x, p.z, p.level))
		if applyCombatGuard {
			// checkNotCombat (TS:943-945): skip players whose last-combat
			// varp was written within the past 8 ticks.
			if hunt.CheckNotCombat != -1 &&
				int(p.Varp(hunt.CheckNotCombat))+8 > s.currentTick {
				continue
			}
			// checkNotCombatSelf (TS:946-948) — DEFERRED: requires NPC-vars
			// infra (VarNpcType, Npc.vars, Npc.Varp). See nai_followups.md.
		}

		// checkVars (TS:950-957): AND-chain of varp/operator/value predicates.
		// Nil/empty CheckVars → no-op (ranging nil slice yields zero iterations,
		// matching TS empty-`every` → true semantics).
		passCheckVars := true
		for _, cv := range hunt.CheckVars {
			if cv.VarID == -1 {
				// TS:953 `checkVar.varId === -1 ||` short-circuit.
				continue
			}
			if !hunt.CheckHuntCondition(int(p.Varp(cv.VarID)), cv.Condition, cv.Val) {
				passCheckVars = false
				break
			}
		}
		if !passCheckVars {
			continue
		}

		hunted = append(hunted, p)
	}
	return hunted
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHuntPlayersCheckNotCombat|TestHuntPlayersCombatGuard' -v`

Expected: all 10 sub-tests PASS (5 per test function).

Then run the full hunt-test group to verify no regression: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHuntPlayers -v`. Expected: all existing tests still pass — notably `TestHuntPlayersInRange`, `TestHuntPlayersFiltersByLevel`, `TestHuntPlayersSkipsAfkZonedPlayers`, `TestHuntPlayersCheckVisLineOfSight*`, and `TestHuntPlayersCheckVars` from Task 2.

Finally, run the full repository test suite: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`. Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc_hunt.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "feat(world): NAI-15 Task 4 combat guard + checkNotCombat filter

Port TS Npc.ts:942-945 — outer multi-zone/target-equality guard
gating the 8-tick combat-window filter. Guard applies when
candidate is not the NPC's current target AND not in a multi-combat
zone (nil gamemap treats as not-multi, matching the CheckVis
short-circuit convention in the same file).

checkNotCombatSelf (TS:946-948) stays deferred inside the guarded
block with an inline comment citing the NPC-vars-infra blocker.

Deferred-filter comment block rewritten: moves checkNotCombat,
checkVars, and the combat guard into the 'covered' list; the
surviving deferred list cites each filter's missing-infra blocker.

Tests: TestHuntPlayersCheckNotCombat exercises 8-tick window
semantics (varp==currentTick, currentTick-7, currentTick-8, 0,
and -1 disable). TestHuntPlayersCombatGuard exercises all four
guard-SKIP and guard-APPLY paths plus the nil-gamemap fidelity
note."
```

---

## Task 5: Close commit — `nai_followups.md` update + `Closes memory:` trailer

**Files:**
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — add a **Resolved 2026-04-23 (NAI-15)** preamble to the "From NAI-8 → Deferred filters in huntPlayers (future audit)" entry, listing which sub-items closed and which remain. Also add an NPC-vars-infrastructure follow-up section keyed for a future sub-spec.
- No code changes in this task. No test changes.

Rationale: this is the NAI-15 close commit. Applies the `Closes memory:` trailer convention (first NAI to use it, per the `close_commit_memory_trailer.md` memory entry — "apply NAI-15 onward").

- [ ] **Step 1: Update `nai_followups.md`**

Open the file (path above). Locate the section `### Deferred filters in huntPlayers (future audit)` under `## From NAI-8`. Prepend a Resolved preamble exactly as follows (preserving the existing body by moving it into the `_Original deferral body_` collapsed section, matching the style of the `### Deferred: CheckVis` entry in the same file):

```markdown
### Deferred filters in huntPlayers (future audit)

**Partial resolution 2026-04-23 (NAI-15)** — items 3 (checkNotCombat)
and 5 (checkVars) from the deferred list are now ported, along with
the shared outer multi-zone/target-equality guard at TS `Npc.ts:942`
and the supporting `HuntType.CheckHuntCondition` evaluator
(`pkg/objtype/hunttype.go`). Filter order matches TS; deferred-filter
comment block in `modules/world/npc_hunt.go` rewritten to reflect the
new coverage. See `docs/superpowers/specs/2026-04-23-nai-15-varp-filter-bundle-design.md`.

**Remaining deferrals** (each still blocked on missing Go infrastructure):

1. **checkNotBusy (TS:931-933)** — still needs `Player.Busy()` equivalent.
2. **checkNotTooStrong (TS:939-941)** — still needs wilderness detection
   + `NpcType.VisLevel` access at filter-evaluation time.
4. **checkNotCombatSelf (TS:946-948)** — NAI-15 wired the shared outer
   guard but left this filter as an inline DEFERRED comment inside the
   guarded block. **Blocker:** no NPC-vars infrastructure yet.
   Required scope for a future sub-spec: `VarNpcType` config registry
   (parallels `VarPlayerType`), `Npc.vars []int32` field + allocation
   site, `Npc.Varp(id int) int32` method (matching `Player.Varp`
   signature). TS ref: `Npc.ts:195-198` (NPC `getVar`), `Npc.ts:946-948`
   (consumer). Suggested sub-spec: NAI-NN "NPC vars +
   checkNotCombatSelf".
6. **checkInv (TS:959-969)** — still needs `Player.InvTotal(inv, obj)`
   and `Player.InvTotalParam(inv, param)` methods.

---

_Original deferral body (preserved for historical context):_

[leave the existing body here — everything that was under the
"Deferred filters in huntPlayers (future audit)" heading before this
edit, starting from "NAI-8 shipped huntPlayers with 3 of 8 TS
filters..." and ending at "...checkVars → checkInv)."]
```

Keep the existing `### Deferred: CheckVis (LoS/LoW)` entry above untouched (already resolved by NAI-12).

- [ ] **Step 2: Verify working tree before committing**

Run: `git status --short`

Expected: working tree clean (memory lives outside the repo — no repo files should be staged/unstaged).

Run: `git log --oneline -5`

Expected: the top four commits are Tasks 1-4 (CheckHuntCondition, checkVars, SetMulti, combat guard + checkNotCombat), above the NAI-14 close commit `0044ee8`.

- [ ] **Step 3: Full test sweep before close**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS across the entire module. This is the last gate before the close commit.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no output (clean).

- [ ] **Step 4: Create the close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(nai): NAI-15 closed — checkVars + checkNotCombat + combat guard

Closes 2 of 3 deferred varp-dependent filters in huntPlayers by
porting TS Npc.ts:942-957 (outer multi-zone/target-equality guard,
checkNotCombat, checkVars) plus supporting HuntType.CheckHuntCondition.
checkNotCombatSelf stays deferred inside the guarded block pending
NPC-vars infrastructure (VarNpcType, Npc.vars, Npc.Varp), tracked in
memory for a future sub-spec.

Adds pkg/gamemap.SetMulti(x,z,level,multi) test-infra helper to
enable multi-combat coord injection from cross-package tests without
running the full Init pipeline.

Tests: 10 new test cases across TestHuntTypeCheckHuntCondition (5
rows), TestHuntPlayersCheckVars (6 sub-cases), TestHuntPlayersCheckNotCombat
(5 sub-cases), TestHuntPlayersCombatGuard (5 sub-cases), plus
TestSetMulti sanity check.

Closes memory: nai_followups.md § "From NAI-8 → Deferred filters in huntPlayers (future audit)" (items 3 + 5 + shared outer guard)
EOF
)"
```

Note: `--allow-empty` is used because Task 5 contains no in-repo file changes — the memory update is outside the repo. If preferred, fold the close-commit trailer into Task 4's commit instead. The agent should default to `--allow-empty` to keep the close commit as a distinct, grep-discoverable marker.

- [ ] **Step 5: Verify the `Closes memory:` trailer is grep-discoverable**

Run: `git log --grep='Closes memory:' --oneline`

Expected: at least one line showing the new close commit. Confirms the convention is now queryable via git log.

---

## Summary of deliverables

| Artifact | Location | Source |
|---|---|---|
| `HuntType.CheckHuntCondition` method | `pkg/objtype/hunttype.go` | Task 1 |
| `TestHuntTypeCheckHuntCondition` (5 rows) | `pkg/objtype/hunttype_test.go` | Task 1 |
| `checkVars` filter | `modules/world/npc_hunt.go` huntPlayers | Task 2 |
| `TestHuntPlayersCheckVars` (6 sub-cases) | `modules/world/npc_hunt_test.go` | Task 2 |
| `GameMap.SetMulti` test setter | `pkg/gamemap/multimap.go` | Task 3 |
| `TestSetMulti` | `pkg/gamemap/gamemap_test.go` | Task 3 |
| Outer combat guard + `checkNotCombat` filter | `modules/world/npc_hunt.go` huntPlayers | Task 4 |
| Rewritten deferred-filter doc comment | `modules/world/npc_hunt.go:85-106` | Task 4 |
| `TestHuntPlayersCheckNotCombat` (5 sub-cases) | `modules/world/npc_hunt_test.go` | Task 4 |
| `TestHuntPlayersCombatGuard` (5 sub-cases) | `modules/world/npc_hunt_test.go` | Task 4 |
| NAI-15 resolution entry | `~/.claude/.../memory/nai_followups.md` | Task 5 |
| Close commit with `Closes memory:` trailer | git log | Task 5 |
