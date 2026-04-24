# NAI-18 — Size-Aware inApproachDistance LoS Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the NAI-12 "size-aware `inApproachDistance` LoS" deferral by threading per-concrete-entity sizes through `(*Npc).inApproachDistance`'s `HasLineOfSight` call, retiring the tracked DEVIATION.

**Architecture:** One new package-private helper `approachEntitySize(e entity) (width, length int)` in `modules/world/npc_interaction.go` type-switches on `*Player` → (1,1) and `*Npc` → (typ.Size, typ.Size). `(*Npc).inApproachDistance` passes `targetSize` (from the helper) as `srcSize` and `int(n.typ.Size)` as both `destWidth`/`destLength` of `HasLineOfSight`. Two shared test-fixture builders gain `Size: 1` so the existing NAI-12 LoS tests keep the same effective semantics (production `NewNpcType` default is `Size: 1`; fixtures previously inherited the `uint8` zero and silently relied on NAI-12's hard-coded literal).

**Tech Stack:** Go 1.26+. No new packages. Touches `modules/world/npc_interaction.go`, `modules/world/npc_event_queue_test.go`, `modules/world/npc_interaction_test.go`. Spec: `docs/superpowers/specs/2026-04-24-nai-18-sized-approach-los-design.md`.

---

## Task 1: Fixture hygiene — `Size: 1` on shared NPC builders

**Files:**
- Modify: `modules/world/npc_event_queue_test.go:17-25`
- Modify: `modules/world/npc_interaction_test.go:197-203`

Lands first so subsequent tasks can read `int(n.typ.Size)` and get `1` (matching NAI-12's literal behavior) without disturbing existing NAI-12 LoS tests.

- [ ] **Step 1: Add `Size: 1` to `newNpcForLifecycleTest`**

File: `modules/world/npc_event_queue_test.go` — update the `NpcType` literal:

```go
func newNpcForLifecycleTest(t *testing.T) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		Size:       1, // match production NewNpcType default (npctype.go:310);
		// NAI-18: fixture was silently Size=0 (uint8 zero value), which will
		// collide with HasLineOfSight's lineCoordinate(a, b, 0) → a-1 off-by-one
		// once inApproachDistance threads int(n.typ.Size) in Task 3.
		Stats:    []uint16{0, 0, 0, 10, 0, 0}, // HP=10 at NpcStatHitpoints (3)
		Category: -1,
	}
	return NewNpc(1, 0, 3094, 3106, 0, typ)
}
```

- [ ] **Step 2: Add `Size: 1` to `newNpcAt100`**

Read `modules/world/npc_interaction_test.go:197-203` first (the exact body may have minor differences from the snippet below). Apply this edit to set `Size: 1` on the builder's `NpcType` literal. If `newNpcAt100` does not currently construct its own `NpcType` and instead composes an existing helper, skip this step and document in the commit message.

Expected current shape (verify before editing):

```go
func newNpcAt100(t *testing.T) *Npc {
	t.Helper()
	typ := &objtype.NpcType{} // or whatever is there
	n := NewNpc(1, 0, 100, 100, 0, typ)
	n.x, n.z, n.level = 100, 100, 0
	n.startX, n.startZ, n.startLevel = 100, 100, 0
	return n
}
```

Target:

```go
func newNpcAt100(t *testing.T) *Npc {
	t.Helper()
	typ := &objtype.NpcType{Size: 1} // match production NewNpcType default (NAI-18).
	n := NewNpc(1, 0, 100, 100, 0, typ)
	n.x, n.z, n.level = 100, 100, 0
	n.startX, n.startZ, n.startLevel = 100, 100, 0
	return n
}
```

- [ ] **Step 3: Run existing tests to verify no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS. The fixture change does not alter any current test behavior — no NAI-12-era test reads `typ.Size` through a code path that cares about its value.

- [ ] **Step 4: Commit**

```bash
git add modules/world/npc_event_queue_test.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(world): NAI-18 Task 1 fixture Size:1 for shared NPC builders

Add Size: 1 to newNpcForLifecycleTest and newNpcAt100 NpcType literals
to match production NewNpcType default (npctype.go:310). Prep for
Task 3's switch from srcSize=1 literal to int(n.typ.Size) in
(*Npc).inApproachDistance — zero-sized fixtures would feed
lineCoordinate(a, b, 0) → a-1 and silently break NAI-12 LoS tests.
EOF
)"
```

---

## Task 2: `approachEntitySize` helper + table-driven unit test

**Files:**
- Modify: `modules/world/npc_interaction.go` (add helper immediately above `(*Npc).inApproachDistance`)
- Modify: `modules/world/npc_interaction_test.go` (add table-driven test alongside existing `inApproachDistance` tests)

- [ ] **Step 1: Write the failing table-driven helper test**

Add this test to `modules/world/npc_interaction_test.go`. Place it adjacent to the existing `TestNpcInApproachDistance*` tests (near line 930 in the current file).

```go
// TestApproachEntitySize verifies the type-switch returns TS-equivalent
// width/length pairs per concrete entity type. Mirrors TS
// PathingEntity.width / .length semantics (NAI-18).
func TestApproachEntitySize(t *testing.T) {
	tests := []struct {
		name       string
		build      func() entity
		wantWidth  int
		wantLength int
	}{
		{
			name:       "player",
			build:      func() entity { return newActivePlayer(1) },
			wantWidth:  1,
			wantLength: 1,
		},
		{
			name: "npc_size_1",
			build: func() entity {
				return NewNpc(1, 0, 3094, 3106, 0, &objtype.NpcType{Size: 1})
			},
			wantWidth:  1,
			wantLength: 1,
		},
		{
			name: "npc_size_2",
			build: func() entity {
				return NewNpc(1, 0, 3094, 3106, 0, &objtype.NpcType{Size: 2})
			},
			wantWidth:  2,
			wantLength: 2,
		},
		{
			name: "npc_size_3",
			build: func() entity {
				return NewNpc(1, 0, 3094, 3106, 0, &objtype.NpcType{Size: 3})
			},
			wantWidth:  3,
			wantLength: 3,
		},
		{
			name:       "default_fake_entity",
			build:      func() entity { return fakeEntity{} },
			wantWidth:  1,
			wantLength: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, l := approachEntitySize(tc.build())
			if w != tc.wantWidth || l != tc.wantLength {
				t.Errorf("approachEntitySize: got (%d, %d), want (%d, %d)",
					w, l, tc.wantWidth, tc.wantLength)
			}
		})
	}
}
```

If `fakeEntity` has required fields (e.g. in `interaction_test.go:449` it is `func (f fakeEntity) Coords() (x, z, level int) { return f.x, f.z, f.level }`), use `fakeEntity{}` with zero values since the test does not call `Coords()`. Verify by reading `interaction_test.go:449-451` before running.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestApproachEntitySize -v`
Expected: FAIL with `approachEntitySize` undefined compile error.

- [ ] **Step 3: Implement the helper**

Add this function to `modules/world/npc_interaction.go` immediately ABOVE the `inApproachDistance` function (which currently begins at line 508 with its doc comment block).

```go
// approachEntitySize returns target (width, length) for the NPC-side
// LoS sizing call in inApproachDistance. Mirrors TS PathingEntity.width
// and .length per concrete entity type:
//
//	*Player → (1, 1)           players are always square size-1
//	*Npc    → (typ.Size, typ.Size)  NPCs are square; typ.Size is side length
//	default → (1, 1)           test doubles / future non-pathing entities
//
// Length is returned for API symmetry with TS; current callers consume
// only width because Go's HasLineOfSight collapses src to scalar srcSize
// (see FIDELITY note on inApproachDistance).
func approachEntitySize(e entity) (width, length int) {
	switch t := e.(type) {
	case *Player:
		return 1, 1
	case *Npc:
		size := int(t.typ.Size)
		return size, size
	default:
		return 1, 1
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestApproachEntitySize -v`
Expected: PASS all five subtests (`player`, `npc_size_1`, `npc_size_2`, `npc_size_3`, `default_fake_entity`).

- [ ] **Step 5: Run full world package tests as a safety net**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`
Expected: PASS. Adding an unreferenced helper does not affect other tests.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-18 Task 2 approachEntitySize helper

Type-switches target entity on *Player → (1,1), *Npc → (typ.Size,
typ.Size), default → (1,1). Mirrors TS PathingEntity.width/.length per
concrete type. Width and length returned symmetrically though current
callers consume only width because Go's HasLineOfSight collapses src
to scalar srcSize (consumer lands in Task 3).
EOF
)"
```

---

## Task 3: `(*Npc).inApproachDistance` call-site rewire + integration tests

**Files:**
- Modify: `modules/world/npc_interaction.go:526-554` (the `(*Npc).inApproachDistance` function body)
- Modify: `modules/world/npc_interaction_test.go` (add two new integration tests)

TDD: integration tests written first — they assert size=2 outcomes that the current NAI-12 `srcSize=1` literal cannot produce. Then rewire the call site. Then add size=1 companion sub-tests as regression anchors.

- [ ] **Step 1: Write failing integration test — multi-tile target shifts LoS start tile**

Add to `modules/world/npc_interaction_test.go`, next to the existing `TestNpcInApproachDistance*` tests (after line 938).

```go
// TestNpcInApproachDistanceMultiTileTargetShiftsLoSStartTile guards the
// target-size flow through approachEntitySize → HasLineOfSight's srcSize
// arg (NAI-18). Fixture exploits lineCoordinate's size-2 start-tile
// shift: target-as-src at srcZ=3106 with srcSize=2 starts the ray at
// startZ=3107 (target's N-edge); with srcSize=1, start=3106.
//
// FlagLoc placed at (3094, 3107) — this flag is checked only at the
// ray start tile (linevalidator.go:54), NOT in traversal masks. So
// size=2 fails immediately (start tile flagged) and size=1 passes
// (ray walks through 3107 without a FlagLoc check).
func TestNpcInApproachDistanceMultiTileTargetShiftsLoSStartTile(t *testing.T) {
	build := func(t *testing.T, targetSize uint8) (*Npc, *Npc) {
		t.Helper()
		s := newServerForScriptTest(t)
		s.gamemap = gamemap.New(discardLogger())
		s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3094, 3106, 0)
		s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3094, 3107, 0)
		s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3094, 3108, 0)
		s.gamemap.Pathfinder.Flags.Add(3094, 3107, 0, collision.FlagLoc)

		self := NewNpc(1, 0, 3094, 3108, 0, &objtype.NpcType{Size: 1})
		self.server = s

		target := NewNpc(2, 0, 3094, 3106, 0, &objtype.NpcType{Size: targetSize})
		return self, target
	}

	t.Run("size2_start_tile_flagged", func(t *testing.T) {
		self, target := build(t, 2)
		if self.inApproachDistance(5, target) {
			t.Error("inApproachDistance: got true, want false — target Size=2 " +
				"should shift ray start to FlagLoc'd tile (3094, 3107)")
		}
	})

	t.Run("size1_start_tile_clear", func(t *testing.T) {
		self, target := build(t, 1)
		if !self.inApproachDistance(5, target) {
			t.Error("inApproachDistance: got false, want true — target Size=1 " +
				"should start ray at (3094, 3106); FlagLoc at 3107 is not " +
				"in traversal masks")
		}
	})
}
```

- [ ] **Step 2: Write failing integration test — multi-tile self shifts LoS end tile**

Also add to `modules/world/npc_interaction_test.go`:

```go
// TestNpcInApproachDistanceMultiTileSelfShiftsLoSEndTile guards the
// self-size flow through int(n.typ.Size) → HasLineOfSight's destWidth
// AND destLength args (NAI-18). Fixture exploits lineCoordinate's
// size-2 end-tile shift: self-as-dest at destZ=3106 with destLength=2
// ends the ray at endZ=3107; with destLength=1, end=3106.
//
// FlagWallNorthProjBlocker placed at (3094, 3106). Travelling south
// (dest is south of src), the zFlags mask is LineSightBlockedNorth =
// FlagLocProjBlocker | FlagWallNorthProjBlocker. Only FlagLocProjBlocker
// is cleared at the end tile (linevalidator.go:112), so
// FlagWallNorthProjBlocker blocks traversal when the ray enters 3106.
// Size=2 ray stops at 3107 → passes. Size=1 ray enters 3106 → blocked.
func TestNpcInApproachDistanceMultiTileSelfShiftsLoSEndTile(t *testing.T) {
	build := func(t *testing.T, selfSize uint8) (*Npc, *Player) {
		t.Helper()
		s := newServerForScriptTest(t)
		s.gamemap = gamemap.New(discardLogger())
		s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3094, 3106, 0)
		s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3094, 3107, 0)
		s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3094, 3108, 0)
		s.gamemap.Pathfinder.Flags.Add(3094, 3106, 0, collision.FlagWallNorthProjBlocker)

		self := NewNpc(1, 0, 3094, 3106, 0, &objtype.NpcType{Size: selfSize})
		self.server = s

		target := addPlayerToServer(t, s, 1, 3094, 3108, 0)
		return self, target
	}

	t.Run("size2_end_tile_clear", func(t *testing.T) {
		self, target := build(t, 2)
		if !self.inApproachDistance(5, target) {
			t.Error("inApproachDistance: got false, want true — self Size=2 " +
				"should terminate ray at (3094, 3107), not reach " +
				"FlagWallNorthProjBlocker at (3094, 3106)")
		}
	})

	t.Run("size1_end_tile_blocked", func(t *testing.T) {
		self, target := build(t, 1)
		if self.inApproachDistance(5, target) {
			t.Error("inApproachDistance: got true, want false — self Size=1 " +
				"should terminate ray at (3094, 3106), where " +
				"FlagWallNorthProjBlocker blocks entry from the north")
		}
	})
}
```

- [ ] **Step 3: Run the new integration tests to verify the size=2 sub-tests fail under current code**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ \
  -run 'TestNpcInApproachDistanceMultiTile' -v
```

Expected:
- `TestNpcInApproachDistanceMultiTileTargetShiftsLoSStartTile/size2_start_tile_flagged` — FAIL (current code passes `srcSize=1`, ray starts at 3106, not the flagged 3107; returns true; assertion wants false).
- `TestNpcInApproachDistanceMultiTileTargetShiftsLoSStartTile/size1_start_tile_clear` — PASS (current code effectively passes srcSize=1 already).
- `TestNpcInApproachDistanceMultiTileSelfShiftsLoSEndTile/size2_end_tile_clear` — FAIL (current code uses destLength=1, so ray reaches 3106 and is blocked; returns false; assertion wants true).
- `TestNpcInApproachDistanceMultiTileSelfShiftsLoSEndTile/size1_end_tile_blocked` — PASS (current code already effectively uses destLength=1).

This is the RED anchor: exactly the two size=2 cases fail. If a size=1 case also fails, the fixture setup has a bug — fix the fixture, not the production code.

- [ ] **Step 4: Rewire the call site in `(*Npc).inApproachDistance`**

Modify `modules/world/npc_interaction.go:548-552`. The current code is:

```go
	// LoS gate — TS PathingEntity.ts:402-405. Target-as-source + self-as-dest
	// (NPC-backward quirk); FlagBlockPlayers as extraFlag (GameMap.ts:433-435).
	// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
	if n.server != nil && n.server.gamemap != nil &&
		!n.server.gamemap.Pathfinder.LineValidator.HasLineOfSight(
			n.level, tx, tz, n.x, n.z, 1, 1, 1, collision.FlagBlockPlayers) {
		return false
	}
```

Replace with:

```go
	// LoS gate — TS PathingEntity.ts:402-405. Target-as-source + self-as-dest
	// (NPC-backward quirk); FlagBlockPlayers as extraFlag (GameMap.ts:433-435).
	// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
	targetSize, _ := approachEntitySize(target)
	selfSize := int(n.typ.Size)
	if n.server != nil && n.server.gamemap != nil &&
		!n.server.gamemap.Pathfinder.LineValidator.HasLineOfSight(
			n.level, tx, tz, n.x, n.z, targetSize, selfSize, selfSize,
			collision.FlagBlockPlayers) {
		return false
	}
```

Do NOT touch the six-line `DEVIATION:` comment block at lines 521-525 in this task — Task 4 retires it separately for commit isolation.

- [ ] **Step 5: Run the new integration tests to verify GREEN**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ \
  -run 'TestNpcInApproachDistanceMultiTile' -v
```

Expected: all four subtests PASS.

- [ ] **Step 6: Run all existing inApproachDistance tests to catch regressions**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ \
  -run 'TestNpcInApproachDistance' -v
```

Expected: all pre-NAI-18 tests (`TestNpcInApproachDistanceLosPasses`, `TestNpcInApproachDistanceLosBlocks`, `TestNpcInApproachDistanceNpcBackwardArgsQuirk`, `TestNpcInApproachDistancePlayerFlagIsRespected`, plus the four new subtests from Steps 1-2) PASS. This validates that Task 1's fixture hygiene (`Size: 1` on `newNpcForLifecycleTest`) preserves NAI-12 semantics.

- [ ] **Step 7: Run full world package tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`
Expected: PASS. If any `tryInteract`-exercising test fails because its inline `NpcType{}` literal leaves `Size=0`, add `Size: 1` to that literal and document the ripple in the commit message. Candidate hot-spots to scan if failures appear: `npc_interaction_test.go:123`, `:144`, `:165`, `:212`, `:234`, `:256`, `:278`, `:425`, `:496`; `npc_player_modes_test.go:25`, `:50`, `:76`, `:98`, `:122`, `:149`; `interaction_test.go:461`, `:513`; `interaction_trigger_test.go:778`.

- [ ] **Step 8: Run full module test suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: PASS. No cross-package breakage expected — the change is local to `modules/world` internals.

- [ ] **Step 9: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-18 Task 3 size-aware inApproachDistance LoS

Thread approachEntitySize(target) (target-as-src width) and
int(n.typ.Size) (self-as-dest width+length; NPCs are square) through
HasLineOfSight, replacing NAI-12's triple-1 approximation. Mirrors
TS PathingEntity.ts:402-403 NPC branch per-type sizing while staying
within Go's HasLineOfSight scalar-src-size signature (lossless for
all current square pathing entities).

Two new integration tests exercise lineCoordinate's size-2 tile-anchor
shift via concrete collision-flag fixtures:
- Target Size=2 shifts ray start to N-edge tile (FlagLoc probe).
- Self Size=2 shifts ray end by one tile short of destZ
  (FlagWallNorthProjBlocker probe).
Both tests also assert size=1 regression anchors.
EOF
)"
```

---

## Task 4: Retire DEVIATION comment block

**Files:**
- Modify: `modules/world/npc_interaction.go:521-525`

Cosmetic + documentation-hygiene commit isolating the comment swap from the behavioral change in Task 3. Makes `git log` tell a clearer story.

- [ ] **Step 1: Replace the six-line DEVIATION block with a four-line FIDELITY note**

Current text in `modules/world/npc_interaction.go`, as part of the `(*Npc).inApproachDistance` doc comment block (lines 521-525):

```go
// DEVIATION: TS passes target.width+target.length and this.width+this.length
// (four size args). Go's HasLineOfSight collapses src to scalar srcSize;
// NAI-12 approximates with srcSize=1, destWidth=1, destLength=1 matching
// the hunt-variant convention. Tracked as size-aware follow-up in
// nai_followups.md.
```

Replace with:

```go
// FIDELITY: LoS sizing uses approachEntitySize per target concrete
// type (*Player → 1, *Npc → typ.Size; all current pathing entities
// are square). Go's HasLineOfSight collapses src to scalar srcSize
// (linevalidator.go:21 forces srcLength = srcWidth in the underlying
// RayCast), which is lossless for square entities. NAI-18 closed the
// NAI-12 tracked size-aware deferral.
```

- [ ] **Step 2: Run inApproachDistance tests to confirm no semantic drift**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ \
  -run 'TestNpcInApproachDistance|TestApproachEntitySize' -v
```

Expected: all tests still PASS (comment change is non-functional).

- [ ] **Step 3: Commit**

```bash
git add modules/world/npc_interaction.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
polish(world): NAI-18 Task 4 retire size-aware DEVIATION comment

NAI-12's DEVIATION block above inApproachDistance pointed at the
size-aware follow-up in nai_followups.md. Task 3 closed that
follow-up, so swap the block for a concise FIDELITY note explaining
why Go's scalar-src-size HasLineOfSight signature is lossless for
the current all-square pathing-entity population.
EOF
)"
```

---

## Task 5: NAI close — annotate `nai_followups.md` and write close commit

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (the NAI-12 "Deferred: size-aware inApproachDistance LoS" entry starting at line 542)

This is the NAI close commit. Follows the established chore(nai) pattern (see `082bcd9` NAI-17 close, `29eb8f8` NAI-16 close) and carries the `Closes memory:` trailer per the `close_commit_memory_trailer.md` memory entry.

- [ ] **Step 1: Annotate the NAI-12 follow-up entry as resolved**

Read `nai_followups.md` around line 542 first to confirm the current text. The entry begins:

```
### Deferred: size-aware inApproachDistance LoS

NAI-12 wires LoS into `inApproachDistance` but approximates sizes with
`srcSize=destWidth=destLength=1` ...
```

Add a resolution preamble immediately under the header, following the pattern used for other resolved entries in the same file (e.g. the `### Deferred: LoS gating in inApproachDistance` entry earlier in the file). The result should be:

```
### Deferred: size-aware inApproachDistance LoS

**Resolved 2026-04-24 (NAI-18)** in the Task 2 + Task 3 commits of
`docs/superpowers/plans/2026-04-24-nai-18-sized-approach-los.md`.
`(*Npc).inApproachDistance` now threads `approachEntitySize(target)`
(target-as-src width) and `int(n.typ.Size)` (self-as-dest width+length)
through `HasLineOfSight`, replacing the NAI-12 triple-1 approximation.
Per-type sizing mirrors TS `PathingEntity.width`/`.length` for the NPC
branch at TS `PathingEntity.ts:402-403`. Go's scalar-src-size
`HasLineOfSight` signature is lossless for all current concrete entity
types (all square pathing entities). Two new integration tests
(`TestNpcInApproachDistanceMultiTileTargetShiftsLoSStartTile` and
`...MultiTileSelfShiftsLoSEndTile`) exercise the size-2 ray-start
and ray-end tile-anchor shifts via concrete collision-flag fixtures.
See `docs/superpowers/specs/2026-04-24-nai-18-sized-approach-los-design.md`.

---

_Original deferral body (preserved for historical context):_

NAI-12 wires LoS into `inApproachDistance` but approximates sizes with
...
```

Preserve the original body verbatim below the `---` separator per the existing NAI-followups convention (see how other resolved entries in the same file handle the preamble + horizontal-rule + verbatim-body pattern).

- [ ] **Step 2: Verify the memory file is still valid markdown**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcInApproachDistance|TestApproachEntitySize' -v`
Expected: PASS (smoke check; memory file edit is doc-only).

Visual-scan the edited file to confirm:
- The resolution preamble appears under the `### Deferred: size-aware inApproachDistance LoS` header.
- The `---` separator divides the new preamble from the preserved body.
- No stray characters or broken markdown.

- [ ] **Step 3: Run the full world-module test suite one last time**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`
Expected: PASS. This is the gate for the NAI close commit.

- [ ] **Step 4: Write the NAI close commit**

Note: the memory file is outside the goscape worktree, so this commit is goscape-repo-only (matching the `chore(nai)` pattern for prior closes). The memory file edit does not appear in `git status` because the memory dir is tracked as a separate `.claude/` concern.

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(nai): NAI-18 closed — size-aware inApproachDistance LoS

(*Npc).inApproachDistance now threads approachEntitySize(target) and
int(n.typ.Size) through HasLineOfSight, closing the NAI-12 "Deferred:
size-aware inApproachDistance LoS" follow-up. Two new integration
tests guard the size-2 ray-start and ray-end tile-anchor shifts; two
shared test-fixture builders (newNpcForLifecycleTest, newNpcAt100)
gained explicit Size: 1 to match production NewNpcType default.

Spec: docs/superpowers/specs/2026-04-24-nai-18-sized-approach-los-design.md
Plan: docs/superpowers/plans/2026-04-24-nai-18-sized-approach-los.md

Closes memory: From NAI-12 / Deferred: size-aware inApproachDistance LoS
EOF
)"
```

If any code changes were made after Task 4 (e.g. stray fixture fixes flushed out in Task 3 Step 7), stage those in this commit instead of `--allow-empty`.

- [ ] **Step 5: Verify the commit log tells the NAI-18 story**

Run: `git log --oneline -10`
Expected output shape:
```
<sha> chore(nai): NAI-18 closed — size-aware inApproachDistance LoS
<sha> polish(world): NAI-18 Task 4 retire size-aware DEVIATION comment
<sha> feat(world): NAI-18 Task 3 size-aware inApproachDistance LoS
<sha> feat(world): NAI-18 Task 2 approachEntitySize helper
<sha> chore(world): NAI-18 Task 1 fixture Size:1 for shared NPC builders
<sha> docs(plan): NAI-18 implementation plan — size-aware inApproachDistance LoS
<sha> docs(spec): NAI-18 size-aware inApproachDistance LoS
<sha> chore(nai): NAI-17 closed — NPC stats-array + NPC_CHANGETYPE_KEEPALL
...
```

All NAI-18 commits present and in the expected order.
